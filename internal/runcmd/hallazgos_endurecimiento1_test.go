// Tests de regresion de la tarea endurecimiento-1 en runcmd. Escritos desde
// el texto de cada hallazgo, las specs y el contrato que promete la
// correccion, SIN LEER LA IMPLEMENTACION: runs de verdad (Manager.Start) con
// un provider falso registrado en providers.Default y un binario falso en el
// PATH, y telemetria escrita a mano en .hoom/runs. Los helpers de este
// archivo llevan el prefijo e1. Los casos que necesitan unix (un limite de
// tamano de archivo en un proceso ayudante, la carrera de carreratest) estan
// en hallazgos_endurecimiento1_unix_test.go.
//
//   - 20261019T154924_e8e42c (resilience): una linea del provider de mas de
//     MaxLinea (4 MiB) cortaba la lectura de stdout (o de stderr) sin mirar
//     el error: el provider quedaba bloqueado escribiendo hasta el timeout y
//     la linea se perdia en silencio. Ahora la lectura sigue, las lineas de
//     despues se procesan y la narracion trae un aviso con el tamano de la
//     linea descartada (CA-362, CA-24: degradacion declarada).
//   - 20261020T150815_a4f371 (reliability): con stdout y stderr produciendo
//     a la vez, el orden de los eventos en memoria (Manager.Events) y el del
//     .jsonl podian diferir. Lo que el run tiene en memoria es exactamente lo
//     que escribio, en el mismo orden (CA-362).
//   - 20261019T153725_305cbc (risk): con .hoom/runs (o .hoom) enlazado al de
//     otro proyecto, Metas importaba sus sidecars. Ningun run de otro
//     proyecto se lista ni se lee, tampoco por un sidecar que es un enlace;
//     MetasErr dice cuando no pudo listar (CA-159, CA-361, CA-362).
//   - 20261019T153725_31a00d (risk): el caso estatico (enlace ya puesto) va
//     aca junto con 305cbc; la carrera, en el archivo _unix.
//   - 20261019T213853_186b1a (resilience): en el archivo _unix.
package runcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// ---------------------------------------------------------------- provider falso

// e1Proveedor: cada linea de su salida que es un evento en JSON sale tal
// cual (ts incluido); una que empieza con e1Relleno no da ningun evento (es
// solo volumen); cualquier otra sale como text con la linea.
type e1Proveedor struct{}

const (
	e1Nombre  = "e1-falso"
	e1Relleno = "e1-relleno "
)

func (e1Proveedor) Name() string { return e1Nombre }
func (e1Proveedor) Bin() string  { return e1Nombre }
func (e1Proveedor) Capabilities() providers.Capabilities {
	return providers.Capabilities{Structured: true}
}
func (e1Proveedor) Command(providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: e1Nombre}, nil
}
func (e1Proveedor) Normalize(line string) []providers.Event {
	if strings.HasPrefix(line, e1Relleno) {
		return nil
	}
	var e providers.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil || e.Kind == "" {
		return []providers.Event{{TS: time.Now().UTC(), Kind: "text", Detail: line}}
	}
	return []providers.Event{e}
}

var (
	e1Registro    sync.Once
	e1ErrRegistro error
)

func e1Registrar(t *testing.T) {
	t.Helper()
	e1Registro.Do(func() { e1ErrRegistro = providers.Default.Register(e1Proveedor{}) })
	if e1ErrRegistro != nil {
		t.Fatalf("fixture: registrar el provider falso: %v", e1ErrRegistro)
	}
}

// e1Archivo escribe contenido en dir/nombre y devuelve la ruta.
func e1Archivo(t *testing.T, dir, nombre string, contenido []byte) string {
	t.Helper()
	p := filepath.Join(dir, nombre)
	if err := os.WriteFile(p, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// e1Lineas une las lineas, cada una con su salto.
func e1Lineas(ls ...string) []byte {
	return []byte(strings.Join(ls, "\n") + "\n")
}

// e1Texto es la linea JSON de un evento text del provider.
func e1Texto(t *testing.T, detail string) string {
	t.Helper()
	raw, err := json.Marshal(Event{TS: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC), Kind: "text", Detail: detail})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// e1Correr hace un run de verdad del provider falso con script como binario
// y el timeout dado, y devuelve root, el id, el estado final, cuanto tardo
// desde Start hasta cerrar y lo que el run tiene en memoria.
func e1Correr(t *testing.T, script string, timeout time.Duration) (root, id string, fin Run, tarda time.Duration, mem []Event) {
	t.Helper()
	e1Registrar(t)
	installFakeNamed(t, e1Nombre, script)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(root)
	m.Timeout = timeout
	t0 := time.Now()
	info, err := m.Start(StartOptions{Provider: e1Nombre, Prompt: "endurecimiento"})
	if err != nil {
		t.Fatalf("fixture: el run arranca: %v", err)
	}
	listo := make(chan Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	select {
	case fin = <-listo:
	case <-time.After(timeout + time.Minute):
		t.Fatalf("fixture: el run no cerro ni con su timeout de %s", timeout)
	}
	tarda = time.Since(t0)
	_, mem, err = m.Events(info.ID, 0)
	if err != nil {
		t.Fatalf("fixture: Events del run: %v", err)
	}
	return root, info.ID, fin, tarda, mem
}

// e1Indice es el indice del primer evento cuyo detail contiene s, o -1.
func e1Indice(evs []Event, s string) int {
	for i, e := range evs {
		if strings.Contains(e.Detail, s) {
			return i
		}
	}
	return -1
}

// e1Resumen describe los eventos sin volcar details enormes.
func e1Resumen(evs []Event) string {
	var b strings.Builder
	for i, e := range evs {
		if i >= 40 {
			fmt.Fprintf(&b, "  ... (%d eventos mas)\n", len(evs)-i)
			break
		}
		b.WriteString("  " + dlgResumen(e) + "\n")
	}
	return b.String()
}

// ---------------------------------------------------------------- e8e42c

const (
	// e1Timeout es el timeout del run en los tests de e8e42c: chico, para que
	// un run trabado lo agote enseguida.
	e1Timeout = 15 * time.Second
	// e1Holgura es lo mas que puede tardar el run: mucho menos que el
	// timeout. Un provider que se queda bloqueado escribiendo lo pasa.
	e1Holgura = 7 * time.Second
	// e1Aviso es el comienzo del detail del aviso de una linea descartada.
	e1Aviso = "aviso: hoom descarto una linea"
)

// e1Gigante es una linea de mas de MaxLinea bytes (sin el salto) que no es
// JSON: si llegara al normalizador saldria entera como text.
func e1Gigante() string {
	return "e1-gigante " + strings.Repeat("g", MaxLinea+64<<10)
}

// e1RellenoPesado son 8 lineas de 128 KiB (1 MiB en total, mucho mas que el
// buffer de un pipe) que el provider escribe despues de la gigante: si hoom
// deja de leer, el provider se bloquea escribiendolas.
func e1RellenoPesado() []string {
	var ls []string
	for i := 0; i < 8; i++ {
		ls = append(ls, fmt.Sprintf("%s%d %s", e1Relleno, i, strings.Repeat("r", 128<<10)))
	}
	return ls
}

// e1SinLaGigante exige que ningun evento traiga la linea gigante (ni un
// detail de mas de MaxLinea bytes): se descarta, no se carga.
func e1SinLaGigante(t *testing.T, ca string, evs []Event) {
	t.Helper()
	for i, e := range evs {
		if len(e.Detail) > MaxLinea || strings.Contains(e.Detail, "e1-gigante "+strings.Repeat("g", 64)) {
			t.Errorf("%s: la linea de mas de MaxLinea no llega a la narracion: el evento %d trae %d bytes (%.80q)",
				ca, i, len(e.Detail), e.Detail)
		}
	}
}

// CA-362, CA-24 (hallazgo e8e42c): una linea de stdout del provider de mas
// de MaxLinea (4 MiB) no detiene la lectura. Despues de la linea gigante el
// provider escribe 1 MiB mas (mas que el buffer de un pipe) y dos eventos:
//   - el run cierra mucho antes de su timeout (el provider no queda
//     bloqueado escribiendo);
//   - los eventos de despues de la gigante SI se procesan, en orden;
//   - la narracion trae, en el lugar de la linea, un evento text cuyo detail
//     empieza con "aviso: hoom descarto una linea" y dice el tamano de la
//     linea en bytes (con o sin el salto), en memoria y en el .jsonl;
//   - ningun evento trae la linea descartada;
//   - si el run cerro done, el detail de su evento de cierre menciona
//     "descarto" (el cierre no calla que se perdio una linea).
func TestHallazgo_e8e42c_CA362_LineaDeMasDe4MiBEnStdoutNoFrenaLaLectura(t *testing.T) {
	const ca = "CA-362 (hallazgo e8e42c, stdout)"
	dir := t.TempDir()
	gigante := e1Gigante()
	lineas := []string{e1Texto(t, "e1 antes de la gigante"), gigante}
	lineas = append(lineas, e1RellenoPesado()...)
	lineas = append(lineas, e1Texto(t, "e1 despues de la gigante 1"), e1Texto(t, "e1 despues de la gigante 2"))
	salida := e1Archivo(t, dir, "salida.txt", e1Lineas(lineas...))

	root, id, fin, tarda, mem := e1Correr(t, "cat '"+salida+"'\nexit 0\n", e1Timeout)

	if tarda > e1Holgura || fin.Status == StatusRunning {
		t.Fatalf("%s: una linea de mas de MaxLinea (%d bytes) no frena la lectura: el run cierra mucho antes de su timeout de %s; tardo %s y cerro %q (el provider quedo bloqueado escribiendo):\n%s",
			ca, len(gigante), e1Timeout, tarda.Round(time.Millisecond), fin.Status, e1Resumen(mem))
	}
	antes, d1, d2 := e1Indice(mem, "e1 antes de la gigante"), e1Indice(mem, "e1 despues de la gigante 1"), e1Indice(mem, "e1 despues de la gigante 2")
	if antes < 0 || d1 < 0 || d2 < 0 || !(antes < d1 && d1 < d2) {
		t.Errorf("%s: las lineas de despues de la gigante se procesan, en orden: antes=%d despues1=%d despues2=%d:\n%s",
			ca, antes, d1, d2, e1Resumen(mem))
	}
	e1SinLaGigante(t, ca, mem)

	var avisos []int
	for i, e := range mem {
		if e.Kind == "text" && strings.HasPrefix(e.Detail, e1Aviso) {
			avisos = append(avisos, i)
		}
	}
	if len(avisos) != 1 {
		t.Fatalf("%s: la narracion trae UN evento text que empieza con %q por la linea descartada; hay %d:\n%s",
			ca, e1Aviso, len(avisos), e1Resumen(mem))
	}
	aviso := mem[avisos[0]]
	if !strings.Contains(aviso.Detail, strconv.Itoa(len(gigante))) && !strings.Contains(aviso.Detail, strconv.Itoa(len(gigante)+1)) {
		t.Errorf("%s: el aviso dice el tamano de la linea descartada (%d bytes, %d con el salto): %q",
			ca, len(gigante), len(gigante)+1, aviso.Detail)
	}
	if antes >= 0 && d1 >= 0 && !(antes < avisos[0] && avisos[0] < d1) {
		t.Errorf("%s: el aviso va en el lugar de la linea descartada, entre lo de antes (%d) y lo de despues (%d); esta en %d",
			ca, antes, d1, avisos[0])
	}
	if fin.Status == StatusDone {
		ultimo := mem[len(mem)-1]
		if !strings.Contains(ultimo.Detail, "descarto") {
			t.Errorf("%s: el run cerro done: el detail de su evento de cierre menciona que se descarto una linea: %s", ca, dlgResumen(ultimo))
		}
	}

	disco, err := ReadEvents(root, id)
	if err != nil {
		t.Fatalf("%s: ReadEvents lee la narracion del run: %v", ca, err)
	}
	if e1Indice(disco, e1Aviso) < 0 {
		t.Errorf("%s: el aviso tambien queda en el .jsonl del run:\n%s", ca, e1Resumen(disco))
	}
	e1SinLaGigante(t, ca+", en el .jsonl", disco)
}

// CA-362, CA-24 (hallazgo e8e42c): lo mismo por stderr, que hoom tambien
// narra. El provider escribe por stdout un evento, por stderr la linea
// gigante y 1 MiB mas, y otra vez por stdout dos eventos: el run cierra mucho
// antes de su timeout, los eventos de stdout de despues y la linea de stderr
// de despues de la gigante se procesan, y ningun evento trae la gigante.
func TestHallazgo_e8e42c_CA362_LineaDeMasDe4MiBEnStderrNoFrenaLaLectura(t *testing.T) {
	const ca = "CA-362 (hallazgo e8e42c, stderr)"
	dir := t.TempDir()
	gigante := e1Gigante()
	errs := append([]string{gigante}, e1RellenoPesado()...)
	errs = append(errs, "e1 despues de la gigante en stderr")
	antes := e1Archivo(t, dir, "antes.txt", e1Lineas(e1Texto(t, "e1 antes de la gigante")))
	porErr := e1Archivo(t, dir, "stderr.txt", e1Lineas(errs...))
	despues := e1Archivo(t, dir, "despues.txt", e1Lineas(e1Texto(t, "e1 despues de la gigante 1"), e1Texto(t, "e1 despues de la gigante 2")))

	_, _, fin, tarda, mem := e1Correr(t, "cat '"+antes+"'\ncat '"+porErr+"' >&2\ncat '"+despues+"'\nexit 0\n", e1Timeout)

	if tarda > e1Holgura || fin.Status == StatusRunning {
		t.Fatalf("%s: una linea de mas de MaxLinea (%d bytes) por stderr no frena la lectura: el run cierra mucho antes de su timeout de %s; tardo %s y cerro %q:\n%s",
			ca, len(gigante), e1Timeout, tarda.Round(time.Millisecond), fin.Status, e1Resumen(mem))
	}
	a, d1, d2 := e1Indice(mem, "e1 antes de la gigante"), e1Indice(mem, "e1 despues de la gigante 1"), e1Indice(mem, "e1 despues de la gigante 2")
	if a < 0 || d1 < 0 || d2 < 0 || !(a < d1 && d1 < d2) {
		t.Errorf("%s: los eventos de stdout de despues de la gigante se procesan, en orden: antes=%d despues1=%d despues2=%d:\n%s",
			ca, a, d1, d2, e1Resumen(mem))
	}
	if e1Indice(mem, "e1 despues de la gigante en stderr") < 0 {
		t.Errorf("%s: la linea de stderr de despues de la gigante se procesa:\n%s", ca, e1Resumen(mem))
	}
	e1SinLaGigante(t, ca, mem)
}

// ---------------------------------------------------------------- a4f371

// e1Grande es la linea JSON de una delegacion con un detail de unos 200 KiB
// de texto mezclado (ASCII, comillas, barras y caracteres de 2 a 4 bytes):
// serializarla cuesta mucho mas que una linea corta.
func e1Grande(t *testing.T, i int) string {
	t.Helper()
	raw, err := json.Marshal(Event{
		TS:     time.Date(2026, 10, 22, 9, 0, i, 0, time.UTC),
		Kind:   "agent",
		Agent:  "e1-sub",
		ToolID: fmt.Sprintf("toolu_e1_%03d", i),
		Detail: fmt.Sprintf("Agent: grande %03d ", i) + dlgMezcla(200<<10),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// CA-362 (hallazgo a4f371): con stdout y stderr produciendo A LA VEZ eventos
// de tamanos muy distintos (40 delegaciones de unos 200 KiB por stdout y
// 10000 lineas cortas por stderr), lo que el run tiene en memoria
// (Manager.Events) es exactamente lo que escribio en .hoom/runs/<id>.jsonl,
// evento por evento y en el MISMO orden: un consumidor que relee el .jsonl
// despues de un reinicio ve la misma historia que vio el run en vivo. Corre
// el run dos veces (la intercalacion depende del scheduler; con la
// correccion no hay ninguna que rompa la igualdad, y sobre el arbol viejo la
// primera vuelta ya la rompe).
func TestHallazgo_a4f371_CA362_MemoriaYDiscoEnElMismoOrdenConStdoutYStderr(t *testing.T) {
	const ca = "CA-362 (hallazgo a4f371)"
	const grandes, cortas = 40, 10000
	dir := t.TempDir()
	var gl []string
	for i := 0; i < grandes; i++ {
		gl = append(gl, e1Grande(t, i))
	}
	var cl []string
	for i := 0; i < cortas; i++ {
		cl = append(cl, fmt.Sprintf("e1 corta %05d", i))
	}
	grande := e1Archivo(t, dir, "grande.jsonl", e1Lineas(gl...))
	corta := e1Archivo(t, dir, "corta.txt", e1Lineas(cl...))
	script := "cat '" + grande + "' &\ncat '" + corta + "' >&2\nwait\nexit 0\n"

	for vuelta := 1; vuelta <= 2; vuelta++ {
		root, id, fin, _, mem := e1Correr(t, script, 2*time.Minute)
		if fin.Status != StatusDone {
			t.Fatalf("%s: fixture: el run del provider falso cierra done: %+v", ca, fin)
		}
		nG, nC := 0, 0
		for _, e := range mem {
			if e.Kind == "agent" && e.Agent == "e1-sub" {
				nG++
			}
			if strings.Contains(e.Detail, "e1 corta ") {
				nC++
			}
		}
		if nG != grandes || nC != cortas {
			t.Fatalf("%s: fixture: el run narra las %d delegaciones de stdout y las %d lineas de stderr; narro %d y %d",
				ca, grandes, cortas, nG, nC)
		}
		disco := dlgDisco(t, root, id)
		if len(disco) != len(mem) {
			t.Fatalf("%s (vuelta %d): el run guarda en memoria exactamente lo que escribe: %d eventos en memoria, %d lineas en el .jsonl",
				ca, vuelta, len(mem), len(disco))
		}
		for i := range mem {
			if !disco[i].ok {
				t.Fatalf("%s (vuelta %d): la linea %d del .jsonl es un evento JSON valido: %.120q", ca, vuelta, i, disco[i].raw)
			}
			if !dlgMismoEvento(mem[i], disco[i].ev) {
				t.Fatalf("%s (vuelta %d): el .jsonl tiene los eventos en el mismo orden que la memoria; el evento %d difiere:\n  memoria %s\n  disco   %s",
					ca, vuelta, i, dlgResumen(mem[i]), dlgResumen(disco[i].ev))
			}
		}
	}
}

// ---------------------------------------------------------------- 305cbc / 31a00d (estatico)

const (
	e1IDPropio = "20261022T100000_e1a001" // el run de este proyecto
	e1IDAjeno  = "20261022T100500_e1b002" // el run que solo existe en el otro proyecto
	e1Secreto  = "secreto del otro proyecto"
)

// e1Run escribe en runs el sidecar y la narracion de un run (con una
// delegacion) del provider prov.
func e1Run(t *testing.T, runs, id, prov, quien string) {
	t.Helper()
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	costo := 7.5
	meta := Meta{ID: id, Provider: prov, Task: "precios", Dir: filepath.Dir(filepath.Dir(runs)), CreatedAt: t0,
		Status: StatusDone, EndedAt: t0.Add(time.Minute), Usage: &providers.Usage{CostUSD: &costo}}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runs, id+".meta.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range []Event{
		{TS: t0, Kind: "start", Detail: "run " + id},
		{TS: t0.Add(time.Second), Kind: "agent", Agent: "e1-espia", ToolID: "toolu_E1", Detail: "Agent: " + quien},
		{TS: t0.Add(2 * time.Second), Kind: "agent_end", Agent: "e1-espia", ToolID: "toolu_E1", Detail: "e1-espia termino"},
		{TS: t0.Add(time.Minute), Kind: "end", Detail: "listo"},
	} {
		l, _ := json.Marshal(e)
		b.Write(l)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(runs, id+".jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// e1Enlace crea enlace -> destino; saltea donde no hay enlaces simbolicos.
func e1Enlace(t *testing.T, destino, enlace string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
}

// e1IDs son los ids de los metas.
func e1IDs(ms []Meta) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.ID+"("+m.Provider+")")
	}
	return out
}

// e1NadaAjeno exige que ningun camino de lectura de runcmd traiga el run que
// solo existe en el otro proyecto: ni Metas ni MetasErr lo listan (ni nada
// del provider ajeno), ReadMeta no lo devuelve, y ReadEvents y
// ReadDelegations son un error sin eventos.
func e1NadaAjeno(t *testing.T, ca, root, como string) {
	t.Helper()
	for _, m := range Metas(root) {
		if m.ID == e1IDAjeno || m.Provider == "ajeno" {
			t.Errorf("%s: con %s, Metas no lista runs de otro proyecto; listo %v", ca, como, e1IDs(Metas(root)))
			break
		}
	}
	ms, _ := MetasErr(root)
	for _, m := range ms {
		if m.ID == e1IDAjeno || m.Provider == "ajeno" {
			t.Errorf("%s: con %s, MetasErr no lista runs de otro proyecto; listo %v", ca, como, e1IDs(ms))
			break
		}
	}
	if m, ok := ReadMeta(root, e1IDAjeno); ok {
		t.Errorf("%s: con %s, ReadMeta no devuelve el sidecar de otro proyecto: %+v", ca, como, m)
	}
	if evs, err := ReadEvents(root, e1IDAjeno); err == nil || len(evs) != 0 {
		t.Errorf("%s: con %s, ReadEvents del run ajeno es un error sin eventos; fue err=%v y %d eventos", ca, como, err, len(evs))
	}
	if ds, err := ReadDelegations(root, e1IDAjeno); err == nil || len(ds) != 0 {
		t.Errorf("%s: con %s, ReadDelegations del run ajeno es un error sin delegaciones; fue err=%v y %d", ca, como, err, len(ds))
	}
}

// CA-159, CA-361, CA-362 (hallazgos 305cbc y 31a00d): si .hoom/runs entero
// (por ruta absoluta o relativa) o .hoom entero es un enlace simbolico al del
// proyecto vecino, ningun run del vecino entra a este proyecto: Metas y
// MetasErr no lo listan (antes Metas importaba sus sidecars validos, que
// Gather publicaba como runs de la tarjeta con su PID y su gasto), ReadMeta
// no lo devuelve, ReadEvents y ReadDelegations son un error, y MetasErr dice
// que no pudo listar (error). Guarda en el mismo fixture: con .hoom/runs real,
// el run propio se lista y se lee.
func TestHallazgo_305cbc_31a00d_CA159_RunsDeOtroProyectoNoSeListanNiSeLeen(t *testing.T) {
	const ca = "CA-159 (hallazgos 305cbc y 31a00d)"
	for _, cs := range []struct{ nombre, que, destino string }{
		{".hoom/runs enlace absoluto al del vecino", "runs", ""},
		{".hoom/runs enlace relativo al del vecino", "runs", filepath.Join("..", "..", "otro", ".hoom", "runs")},
		{".hoom enlace absoluto al del vecino", "hoom", ""},
		{".hoom enlace relativo al del vecino", "hoom", filepath.Join("..", "otro", ".hoom")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(base, "proyecto")
			otroRuns := filepath.Join(base, "otro", ".hoom", "runs")
			e1Run(t, otroRuns, e1IDAjeno, "ajeno", e1Secreto)

			// guarda: el run propio con .hoom/runs real
			runs := filepath.Join(root, ".hoom", "runs")
			e1Run(t, runs, e1IDPropio, "propio", "trabajo propio")
			ms, err := MetasErr(root)
			if err != nil || len(ms) != 1 || ms[0].ID != e1IDPropio {
				t.Fatalf("%s: guarda: con .hoom/runs real, MetasErr lista el run propio sin error: %v %v", ca, e1IDs(ms), err)
			}
			if ms := Metas(root); len(ms) != 1 || ms[0].ID != e1IDPropio {
				t.Fatalf("%s: guarda: con .hoom/runs real, Metas lista el run propio: %v", ca, e1IDs(ms))
			}
			if m, ok := ReadMeta(root, e1IDPropio); !ok || m.Provider != "propio" {
				t.Fatalf("%s: guarda: ReadMeta del run propio: %+v %v", ca, m, ok)
			}
			if evs, err := ReadEvents(root, e1IDPropio); err != nil || len(evs) != 4 {
				t.Fatalf("%s: guarda: ReadEvents del run propio: %v %d", ca, err, len(evs))
			}

			// el ataque: el enlace al vecino
			enlace, destino := runs, otroRuns
			if cs.que == "hoom" {
				enlace, destino = filepath.Join(root, ".hoom"), filepath.Join(base, "otro", ".hoom")
			}
			if err := os.RemoveAll(enlace); err != nil {
				t.Fatal(err)
			}
			if cs.destino != "" {
				destino = cs.destino
			}
			e1Enlace(t, destino, enlace)
			if fi, err := os.Stat(filepath.Join(runs, e1IDAjeno+".meta.json")); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("fixture: a traves del enlace, el sidecar del vecino es un archivo regular: %v %v", fi, err)
			}

			e1NadaAjeno(t, ca, root, cs.nombre)
			if _, err := MetasErr(root); err == nil {
				t.Errorf("%s: con %s, MetasErr dice que no pudo listar los runs de este proyecto (error), no una lista", ca, cs.nombre)
			}
		})
	}
}

// CA-159, CA-361 (hallazgo 305cbc): con .hoom/runs real, un sidecar
// <id>.meta.json que es un enlace simbolico al sidecar valido de otro
// proyecto (y su <id>.jsonl, otro enlace) no se lista ni lo devuelve
// ReadMeta, y su narracion no se lee. El run propio, al lado, sigue listado.
func TestHallazgo_305cbc_CA159_SidecarEnlazadoNoSeLista(t *testing.T) {
	const ca = "CA-159 (hallazgo 305cbc, sidecar enlazado)"
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	runs := filepath.Join(root, ".hoom", "runs")
	otroRuns := filepath.Join(base, "otro", ".hoom", "runs")
	e1Run(t, otroRuns, e1IDAjeno, "ajeno", e1Secreto)
	e1Run(t, runs, e1IDPropio, "propio", "trabajo propio")
	for _, suf := range []string{".meta.json", ".jsonl"} {
		e1Enlace(t, filepath.Join(otroRuns, e1IDAjeno+suf), filepath.Join(runs, e1IDAjeno+suf))
	}

	e1NadaAjeno(t, ca, root, "el sidecar y la narracion como enlaces al vecino")
	ms, err := MetasErr(root)
	if len(ms) != 1 || ms[0].ID != e1IDPropio {
		t.Errorf("%s: el run propio sigue listado y el enlazado no: MetasErr dio %v (err=%v)", ca, e1IDs(ms), err)
	}
	if ms := Metas(root); len(ms) != 1 || ms[0].ID != e1IDPropio {
		t.Errorf("%s: el run propio sigue listado y el enlazado no: Metas dio %v", ca, e1IDs(ms))
	}
}

// CA-159 (hallazgo 305cbc, API nueva MetasErr): sin .hoom/runs, MetasErr da
// una lista vacia y ningun error (un proyecto que nunca corrio un run no es
// un problema); con .hoom/runs que existe y no se puede listar (000), da un
// error en vez de una lista vacia que calla lo que no vio. Metas sigue siendo
// la lectura best-effort de siempre.
func TestHallazgo_305cbc_CA159_MetasErrDistingueNadaDeIlegible(t *testing.T) {
	const ca = "CA-159 (hallazgo 305cbc, MetasErr)"
	root := t.TempDir()
	if ms, err := MetasErr(root); err != nil || len(ms) != 0 {
		t.Fatalf("%s: sin .hoom/runs, MetasErr no da nada y ningun error: %v %v", ca, e1IDs(ms), err)
	}
	if ms := Metas(root); len(ms) != 0 {
		t.Fatalf("%s: sin .hoom/runs, Metas no da nada: %v", ca, e1IDs(ms))
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("sin permisos de unix (o como root) un chmod no quita la lectura")
	}
	runs := filepath.Join(root, ".hoom", "runs")
	e1Run(t, runs, e1IDPropio, "propio", "trabajo propio")
	if ms, err := MetasErr(root); err != nil || len(ms) != 1 {
		t.Fatalf("%s: guarda: legible, MetasErr lista el run: %v %v", ca, e1IDs(ms), err)
	}
	if err := os.Chmod(runs, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(runs, 0o755) })
	if _, err := os.ReadDir(runs); err == nil {
		t.Skip("este sistema deja listar un directorio sin permiso de lectura")
	}
	if ms, err := MetasErr(root); err == nil {
		t.Errorf("%s: con .hoom/runs ilegible, MetasErr da un error y no una lista (%v) que calla lo que no pudo ver", ca, e1IDs(ms))
	}
}
