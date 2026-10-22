//go:build !windows

// Tests de regresion de la tarea endurecimiento-1 en runcmd que necesitan
// unix, escritos desde el texto de cada hallazgo y el contrato de la
// correccion, sin leer la implementacion. Usan los helpers e1 de
// hallazgos_endurecimiento1_test.go.
//
//   - 20261019T213853_186b1a (resilience): el run publicaba el evento en
//     memoria antes de escribirlo e ignoraba el error de la escritura del
//     .jsonl; ante un disco lleno o un write corto, Events exponia eventos
//     que no estaban en el disco, el run cerraba done, y si una escritura
//     posterior volvia a funcionar sus bytes se pegaban a la linea partida.
//     La falla se provoca de forma deterministica con RLIMIT_FSIZE (el
//     limite de tamano de archivo del proceso: un write que lo cruza escribe
//     hasta el limite y el siguiente da EFBIG; Go ignora SIGXFSZ) en un
//     proceso AYUDANTE (el binario de test relanzado), para que el limite no
//     toque ningun otro archivo del proceso de tests (ni el log de la cache de
//     go test). Contrato: lo que el run tiene en memoria es lo que llego al
//     disco, despues de la falla no se escribe nada mas (nunca una linea
//     partida seguida de otra), y el run termina con status error y un
//     detail que dice "la narracion del run no se pudo escribir" (CA-362).
//   - 20261019T153725_31a00d (risk), la carrera: .hoom/runs se comprobaba
//     con Lstat y despues se abria el descendiente por ruta completa; un
//     proceso que cambia .hoom/runs por un enlace al de otro proyecto entre
//     las dos cosas hacia leer la narracion o el sidecar ajeno. Con
//     carreratest.IntercambiarDir cambiando .hoom/runs sin parar, ninguna
//     lectura trae nada del otro proyecto (CA-362, CA-159). La deteccion de
//     la carrera vieja es probabilistica; la correccion no puede fallar este
//     test: no hay intercalacion en la que leer lo ajeno sea correcto.
package runcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// ---------------------------------------------------------------- 186b1a

const (
	// e1EnvNarracion lleva al proceso ayudante el directorio del fixture.
	e1EnvNarracion = "HOOM_TEST_E1_NARRACION"
	// e1Limite es el RLIMIT_FSIZE del ayudante mientras el run escribe la
	// primera parte: el .jsonl no puede pasar de aca.
	e1Limite = 256 << 10
	// e1Parte1 y e1Parte2 son los eventos del provider antes y despues de
	// que el ayudante devuelve el limite: la primera parte pasa de e1Limite.
	e1Parte1, e1Parte2 = 400, 50
	// e1FraseFalla es lo que dice el cierre de un run cuya narracion no se
	// pudo escribir.
	e1FraseFalla = "la narracion del run no se pudo escribir"
)

// e1Informe es lo que el ayudante deja al terminar, ya sin limite.
type e1Informe struct {
	ID      string  `json:"id"`
	Fin     Run     `json:"fin"`
	Eventos []Event `json:"eventos"`
	Fallo   string  `json:"fallo,omitempty"`
}

// TestAyudanteE1NarracionSinDisco no es un test: es el proceso ayudante de
// TestHallazgo_186b1a_CA362_NarracionQueNoSePuedeEscribir. Corre un run del
// provider falso con RLIMIT_FSIZE en e1Limite, espera a que la escritura del
// .jsonl haya chocado con el limite, devuelve el limite, deja seguir al
// provider, espera el cierre y escribe su informe.
func TestAyudanteE1NarracionSinDisco(t *testing.T) {
	dir := os.Getenv(e1EnvNarracion)
	if dir == "" {
		t.Skip("solo corre como proceso ayudante de TestHallazgo_186b1a_CA362_NarracionQueNoSePuedeEscribir")
	}
	inf := e1Informe{}
	escribir := func() {
		raw, _ := json.Marshal(inf)
		if err := os.WriteFile(filepath.Join(dir, "informe.json"), raw, 0o644); err != nil {
			fmt.Printf("informe: %v\n", err)
		}
	}
	e1Registrar(t)
	root := filepath.Join(dir, "proyecto")
	m := NewManager(root)
	m.Timeout = time.Minute

	var viejo syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &viejo); err != nil {
		inf.Fallo = "getrlimit: " + err.Error()
		escribir()
		return
	}
	nuevo := viejo
	nuevo.Cur = e1Limite
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &nuevo); err != nil {
		inf.Fallo = "setrlimit: " + err.Error()
		escribir()
		return
	}
	restaurar := func() { _ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &viejo) }
	defer restaurar()

	info, err := m.Start(StartOptions{Provider: e1Nombre, Prompt: "endurecimiento"})
	if err != nil {
		restaurar()
		inf.Fallo = "start: " + err.Error()
		escribir()
		return
	}
	inf.ID = info.ID
	log := filepath.Join(root, ".hoom", "runs", info.ID+".jsonl")

	// la escritura ya choco con el limite: el .jsonl llego hasta el (o el
	// run ya cerro) y el .jsonl y la memoria dejaron de moverse 300 ms
	limite := time.Now().Add(30 * time.Second)
	var tam int64 = -1
	n, quieto := -1, 0
	for time.Now().Before(limite) && quieto < 15 {
		r, _ := m.Get(info.ID)
		var t2 int64
		if fi, err := os.Stat(log); err == nil {
			t2 = fi.Size()
		}
		llego := t2 >= e1Limite-4096 || r.Status != StatusRunning
		if llego && t2 == tam && r.NumEvents == n {
			quieto++
		} else {
			quieto = 0
		}
		tam, n = t2, r.NumEvents
		time.Sleep(20 * time.Millisecond)
	}
	restaurar()
	if err := os.WriteFile(filepath.Join(dir, "seguir"), nil, 0o644); err != nil {
		inf.Fallo = "seguir: " + err.Error()
	}

	listo := make(chan Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	select {
	case inf.Fin = <-listo:
	case <-time.After(90 * time.Second):
		inf.Fallo = "el run no cerro en 90 s"
		inf.Fin, _ = m.Get(info.ID)
	}
	_, inf.Eventos, _ = m.Events(info.ID, 0)
	escribir()
}

// e1ProviderConPausa escribe en dir las dos partes de la salida y devuelve
// el script del provider: la primera parte, la espera de "seguir" (a lo sumo
// 30 s) y la segunda parte.
func e1ProviderConPausa(t *testing.T, dir string) string {
	t.Helper()
	parte := func(desde, hasta int) []byte {
		var ls []string
		for i := desde; i < hasta; i++ {
			ls = append(ls, e1Texto(t, fmt.Sprintf("e1 evento %03d %s", i, strings.Repeat("n", 1000))))
		}
		return e1Lineas(ls...)
	}
	p1 := e1Archivo(t, dir, "parte1.jsonl", parte(0, e1Parte1))
	p2 := e1Archivo(t, dir, "parte2.jsonl", parte(e1Parte1, e1Parte1+e1Parte2))
	return "cat '" + p1 + "'\n" +
		"i=0\nwhile [ ! -f '" + filepath.Join(dir, "seguir") + "' ] && [ $i -lt 600 ]; do sleep 0.05; i=$((i+1)); done\n" +
		"cat '" + p2 + "'\nexit 0\n"
}

// CA-362 (hallazgo 186b1a): si la escritura del .jsonl falla a mitad del run
// (el limite de tamano de archivo del proceso, como un disco lleno: un write
// corto y despues EFBIG), y despues las escrituras vuelven a poder hacerse:
//   - el run termina con status error (en Run y en su sidecar) y su ultimo
//     evento dice "la narracion del run no se pudo escribir";
//   - el run no publica en memoria eventos que no llegaron al disco: sin ese
//     ultimo evento, lo que tiene en memoria es exactamente lo que el .jsonl
//     tiene en lineas completas, en el mismo orden;
//   - despues de la falla no escribe nada mas: el .jsonl no pasa del limite
//     aunque el limite ya no este, y nunca hay una linea partida seguida de
//     otra (a lo sumo, un pedazo sin salto al final).
func TestHallazgo_186b1a_CA362_NarracionQueNoSePuedeEscribir(t *testing.T) {
	const ca = "CA-362 (hallazgo 186b1a)"
	e1Registrar(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "proyecto")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	installFakeNamed(t, e1Nombre, e1ProviderConPausa(t, dir))

	cmd := exec.Command(os.Args[0], "-test.run=^TestAyudanteE1NarracionSinDisco$", "-test.count=1")
	cmd.Env = append(os.Environ(), e1EnvNarracion+"="+dir)
	salida := make(chan []byte, 1)
	go func() {
		out, _ := cmd.CombinedOutput()
		salida <- out
	}()
	var out []byte
	select {
	case out = <-salida:
	case <-time.After(3 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatalf("%s: fixture: el proceso ayudante no termino en 3 minutos", ca)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "informe.json"))
	if err != nil {
		t.Fatalf("%s: fixture: el ayudante deja su informe: %v\n%s", ca, err, out)
	}
	var inf e1Informe
	if err := json.Unmarshal(raw, &inf); err != nil || inf.Fallo != "" || inf.ID == "" {
		t.Fatalf("%s: fixture: el ayudante corrio el run: %v %q\n%s", ca, err, inf.Fallo, out)
	}

	disco, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", inf.ID+".jsonl"))
	if err != nil {
		t.Fatalf("%s: el .jsonl del run existe: %v", ca, err)
	}
	completas := strings.Split(string(disco), "\n")
	pedazo := completas[len(completas)-1] // lo que hay despues del ultimo salto
	completas = completas[:len(completas)-1]
	if n := strings.Count(string(disco), "e1 evento "); n >= e1Parte1+e1Parte2 {
		t.Fatalf("%s: fixture: el limite corto la narracion (hay %d eventos del provider en el .jsonl)", ca, n)
	}

	if inf.Fin.Status != StatusError {
		t.Errorf("%s: un run cuya narracion no se pudo escribir termina con status error, no %q", ca, inf.Fin.Status)
	}
	if m, ok := ReadMeta(root, inf.ID); !ok || m.Status != StatusError {
		t.Errorf("%s: el sidecar del run dice status error: %+v %v", ca, m, ok)
	}
	if len(inf.Eventos) == 0 || !strings.Contains(inf.Eventos[len(inf.Eventos)-1].Detail, e1FraseFalla) {
		ult := "(ninguno)"
		if len(inf.Eventos) > 0 {
			ult = dlgResumen(inf.Eventos[len(inf.Eventos)-1])
		}
		t.Errorf("%s: el ultimo evento del run dice %q; fue %s", ca, e1FraseFalla, ult)
	}

	if len(disco) > e1Limite {
		t.Errorf("%s: despues de la falla el run no escribe nada mas: el .jsonl mide %d bytes, mas que el limite de %d con que fallo",
			ca, len(disco), e1Limite)
	}
	var enDisco []Event
	for i, l := range completas {
		var e Event
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("%s: nunca una linea partida seguida de otra: la linea %d del .jsonl (de %d, %d bytes) no es un evento: %.80q ... %.80q",
				ca, i, len(completas), len(l), l, l[max(0, len(l)-80):])
		}
		enDisco = append(enDisco, e)
	}
	mem := inf.Eventos
	if len(mem) > 0 && strings.Contains(mem[len(mem)-1].Detail, e1FraseFalla) {
		mem = mem[:len(mem)-1]
	}
	if len(mem) != len(enDisco) {
		t.Fatalf("%s: el run no publica en memoria eventos que no llegaron al disco: %d en memoria (sin el cierre), %d lineas completas en el .jsonl (y un pedazo de %d bytes)",
			ca, len(mem), len(enDisco), len(pedazo))
	}
	for i := range mem {
		if !dlgMismoEvento(mem[i], enDisco[i]) {
			t.Fatalf("%s: lo que el run tiene en memoria es lo que esta en el disco, en el mismo orden; el evento %d difiere:\n  memoria %s\n  disco   %s",
				ca, i, dlgResumen(mem[i]), dlgResumen(enDisco[i]))
		}
	}
}

// ---------------------------------------------------------------- 31a00d (carrera)

// CA-362, CA-159 (hallazgo 31a00d): mientras otro proceso cambia sin parar
// .hoom/runs entre el directorio del proyecto y un enlace simbolico al
// .hoom/runs del vecino (carreratest.IntercambiarDir), y el MISMO id existe
// en los dos con contenido distinto, ninguna lectura trae nada del vecino:
// ReadEvents, ReadEventsFrom y ReadDelegations no traen su delegacion, y
// Metas y ReadMeta no traen su sidecar. Una lectura puede fallar mientras
// .hoom/runs falta o es el enlace; lo que nunca puede es leer lo ajeno.
func TestHallazgo_31a00d_CA362_RunsQueCambiaPorUnEnlaceNoTraeLoAjeno(t *testing.T) {
	const ca = "CA-362 (hallazgo 31a00d)"
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	runs := filepath.Join(root, ".hoom", "runs")
	otroRuns := filepath.Join(base, "otro", ".hoom", "runs")
	e1Run(t, runs, e1IDPropio, "propio", "trabajo propio")
	e1Run(t, otroRuns, e1IDPropio, "ajeno", e1Secreto)
	if evs, err := ReadEvents(root, e1IDPropio); err != nil || len(evs) != 4 || strings.Contains(evs[1].Detail, e1Secreto) {
		t.Fatalf("%s: fixture: sin la carrera se lee el run propio: %v %+v", ca, err, evs)
	}

	parar := carreratest.IntercambiarDir(t, runs, carreratest.Symlink(otroRuns))
	ajenas := map[string]int{}
	lecturas := 0
	for fin := time.Now().Add(1500 * time.Millisecond); time.Now().Before(fin); {
		lecturas++
		if evs, _ := ReadEvents(root, e1IDPropio); e1Indice(evs, e1Secreto) >= 0 {
			ajenas["ReadEvents"]++
		}
		if evs, _, _ := ReadEventsFrom(root, e1IDPropio, 0); e1Indice(evs, e1Secreto) >= 0 {
			ajenas["ReadEventsFrom"]++
		}
		if ds, _ := ReadDelegations(root, e1IDPropio); e1Indice(ds, e1Secreto) >= 0 {
			ajenas["ReadDelegations"]++
		}
		for _, m := range Metas(root) {
			if m.Provider == "ajeno" {
				ajenas["Metas"]++
			}
		}
		if m, ok := ReadMeta(root, e1IDPropio); ok && m.Provider == "ajeno" {
			ajenas["ReadMeta"]++
		}
	}
	vueltas := parar()
	if vueltas == 0 {
		t.Fatalf("%s: fixture: el intercambiador cambio .hoom/runs al menos una vez", ca)
	}
	if len(ajenas) > 0 {
		t.Errorf("%s: con .hoom/runs cambiando por un enlace al vecino (%d vueltas, %d rondas de lectura), ninguna lectura trae lo del vecino; lo trajeron: %v",
			ca, vueltas, lecturas, ajenas)
	}
}
