//go:build !windows

// Ronda 4 de gates-en-grupo: tests de los hallazgos de la review de 4 lentes
// (registro 20261027T172238_9473c1).
//
//   - 47764f, fb2f6d, e55193: la salida del gate iba a un archivo
//     hoom-gate-*.log de TMPDIR; en Windows quedaba en cada corrida, y en
//     Unix se creaba y se borraba enseguida.
//   - 5a3741: ese archivo no tenia tope, y la cola se cortaba despues de
//     leerlo entero en memoria (io.ReadAll): un gate verboso llenaba el disco
//     temporal y la memoria de hoom.
//   - 2f4af2, f0dfd4, cd9f87: TerminarGrupo deja un SIGKILL para despues de
//     la gracia que se dispara sobre el numero de grupo aunque el grupo ya
//     haya terminado y hoom lo haya liberado: si el numero se reuso, mata a
//     otro.
//
// Escritos desde el contrato (go doc ./internal/gates y ./internal/procesos)
// y los hallazgos, sin leer la implementacion. Lo que necesita un TMPDIR
// propio corre en un hoom ayudante (gates_en_grupo_ayudantes_test.go): el
// TMPDIR de este proceso no se toca.
package gates

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/procesos"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// gegMarcaTruncado es la marca con la que empieza la cola de una salida que
// no entro entera (la que se ve en OutputTail).
const gegMarcaTruncado = "...[truncado]..."

// gegFechaVieja es la fecha que se le pone al TMPDIR del hoom ayudante antes
// de arrancarlo: crear o borrar cualquier cosa adentro, aunque dure un
// instante, la cambia.
var gegFechaVieja = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// gegTMPDIRVacio devuelve un directorio vacio para TMPDIR con la fecha vieja.
func gegTMPDIRVacio(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	if err := os.Chtimes(tmp, gegFechaVieja, gegFechaVieja); err != nil {
		t.Fatal(err)
	}
	return tmp
}

// gegTMPDIRIntacto falla si en tmp se creo o se borro algo (su fecha cambio)
// o si quedo algo adentro. vistos es lo que se llego a ver mientras corria.
func gegTMPDIRIntacto(t *testing.T, tmp, etiqueta string, vistos map[string]bool) {
	t.Helper()
	todos, _ := gegArchivos(t, tmp, "\x00")
	st, err := os.Stat(tmp)
	if err != nil {
		t.Fatal(err)
	}
	var nombres []string
	for n := range vistos {
		nombres = append(nombres, n)
	}
	if !st.ModTime().Equal(gegFechaVieja) || len(todos) > 0 || len(vistos) > 0 {
		t.Fatalf("%s: una corrida de un gate no crea ningun archivo en TMPDIR, ni siquiera mientras corre: la fecha de TMPDIR paso de %v a %v (algo se creo o se borro adentro); visto mientras corria: %v; quedo: %v", etiqueta, gegFechaVieja, st.ModTime(), nombres, todos)
	}
}

// gegMirarMientrasCorre mira tmp cada pocos milisegundos hasta que h termine
// (a lo sumo plazo) y devuelve los nombres que vio.
func gegMirarMientrasCorre(t *testing.T, h *gegHoom, tmp string, plazo time.Duration) map[string]bool {
	t.Helper()
	vistos := map[string]bool{}
	hasta := time.Now().Add(plazo)
	for {
		if entradas, err := os.ReadDir(tmp); err == nil {
			for _, e := range entradas {
				vistos[e.Name()] = true
			}
		}
		if h.termino(2 * time.Millisecond) {
			return vistos
		}
		if time.Now().After(hasta) {
			t.Fatalf("%s: el hoom ayudante no termino en %v\n%s", h.etiqueta, plazo, h.salida())
		}
	}
}

// Hallazgos 47764f, fb2f6d y e55193: una corrida normal de un gate no crea
// NINGUN archivo en TMPDIR, ni siquiera mientras corre (en Unix el
// hoom-gate-*.log se creaba y se borraba enseguida; en Windows quedaba). El
// gate sigue devolviendo su salida completa.
func TestHallazgo47764f_CorridaNormalNoCreaNingunArchivoEnTMPDIR(t *testing.T) {
	const etiqueta = "hallazgos 47764f, fb2f6d, e55193"
	d, tmp := t.TempDir(), gegTMPDIRVacio(t)
	h := gegArrancarHoom(t, gegConUnGate(d, "echo uno\necho dos >&2\nsleep 1\necho tres\nexit 0\n", etiqueta, "TMPDIR="+tmp))
	vistos := gegMirarMientrasCorre(t, h, tmp, gegTope)
	gegTMPDIRIntacto(t, tmp, etiqueta, vistos)
	r := h.resultado(t, d)
	if r.Status != verdict.StatusPass || strings.TrimSpace(r.OutputTail) != "uno\ndos\ntres" {
		t.Fatalf("%s: el gate pasa y devuelve su salida completa: %+v", etiqueta, r)
	}
}

// Hallazgo 5a3741: un gate que imprime 50 MiB devuelve solo la cola: a lo
// sumo el tamano de cola configurado mas la marca de truncado, empezando por
// esa marca y terminando en lo ultimo que imprimio. La salida no va a disco
// (ni a TMPDIR ni a la carpeta del proyecto) y la memoria de hoom no crece
// con los 50 MiB: la memoria residente maxima del hoom ayudante se compara
// con la de una corrida igual que imprime 1 KiB.
func TestHallazgo5a3741_SalidaEnormeSoloDejaLaColaSinDiscoNiMemoria(t *testing.T) {
	const etiqueta = "hallazgo 5a3741"
	const cola = 4096
	const enorme = 50 << 20
	correr := func(bytes int) (int64, func()) {
		d, tmp := t.TempDir(), gegTMPDIRVacio(t)
		h := gegArrancarHoom(t, gegConUnGate(d,
			fmt.Sprintf("yes relleno-de-la-salida-0123456789 | head -c %d\necho\necho FIN-DE-LA-SALIDA\n", bytes),
			etiqueta, "TMPDIR="+tmp, gegEnvAyudanteCola+"="+strconv.Itoa(cola)))
		vistos := gegMirarMientrasCorre(t, h, tmp, 2*gegTope)
		r := h.resultado(t, d)
		mem := h.memoriaPico()
		if bytes == enorme {
			if r.Status != verdict.StatusPass {
				t.Fatalf("%s: el gate que imprime %d bytes pasa: %+v", etiqueta, bytes, r)
			}
			if !strings.HasPrefix(r.OutputTail, gegMarcaTruncado) || len(r.OutputTail) > cola+len(gegMarcaTruncado)+1 {
				n := len(r.OutputTail)
				t.Fatalf("%s: de una salida de %d bytes queda la cola: a lo sumo %d bytes mas la marca %q, empezando por ella; quedaron %d bytes que empiezan con %q", etiqueta, bytes, cola, gegMarcaTruncado, n, r.OutputTail[:min(n, 40)])
			}
			if !strings.HasSuffix(strings.TrimSpace(r.OutputTail), "FIN-DE-LA-SALIDA") {
				t.Fatalf("%s: la cola termina en lo ultimo que imprimio el gate: ...%q", etiqueta, r.OutputTail[max(0, len(r.OutputTail)-60):])
			}
		}
		// ni en el proyecto: ningun archivo de mas de 1 MiB
		todos, _ := gegArchivos(t, d, "\x00")
		for _, ruta := range todos {
			if st, err := os.Stat(ruta); err == nil && st.Size() > 1<<20 {
				t.Fatalf("%s: la salida del gate no va a disco; en el proyecto quedo %s con %d bytes", etiqueta, ruta, st.Size())
			}
		}
		// TMPDIR se mira al final: en Unix eso era 47764f, y 5a3741 es el tope
		return mem, func() { gegTMPDIRIntacto(t, tmp, etiqueta, vistos) }
	}
	memChica, _ := correr(1 << 10)
	memEnorme, tmpIntacto := correr(enorme)
	if memChica <= 0 || memEnorme <= 0 {
		t.Fatalf("fixture: no se pudo medir la memoria del hoom ayudante: %d, %d", memChica, memEnorme)
	}
	if crecio := memEnorme - memChica; crecio > enorme/2 {
		t.Fatalf("%s: la memoria de hoom no crece con la salida del gate: con %d MiB de salida su memoria residente maxima fue %d MiB, con 1 KiB %d MiB (crecio %d MiB; tope %d MiB)", etiqueta, enorme>>20, memEnorme>>20, memChica>>20, crecio>>20, enorme>>21)
	}
	t.Logf("%s: memoria residente maxima del hoom ayudante: %d MiB con 1 KiB de salida, %d MiB con %d MiB", etiqueta, memChica>>20, memEnorme>>20, enorme>>20)
	// la salida tampoco paso por TMPDIR, ni un instante
	tmpIntacto()
}

// Hallazgos 2f4af2, f0dfd4 y cd9f87, lo observable sin reusar un numero de
// grupo: un gate que obedece SIGTERM al vencer el plazo termina enseguida;
// en el mismo proceso, antes de que pase su gracia, se arranca un grupo
// nuevo con procesos.Arrancar (como el gate siguiente o un provider). Pasada
// la gracia del primero, el grupo nuevo sigue vivo: el SIGKILL tardio del
// primero no alcanza a otro grupo de hoom. No prueba el caso del hallazgo
// (que el grupo nuevo tenga el MISMO numero): eso pide reusar un PID, que no
// se puede forzar.
func TestHallazgo2f4af2_GrupoNuevoSobreviveLaGraciaDelGateAnterior(t *testing.T) {
	d := t.TempDir()
	c := gegLanzar(t, map[string]string{"test": "sleep 300 &\n" + gegEscribir(d, "hijo.pid", "$!") + "wait\n"})
	gegLeerPID(t, d, "hijo.pid")
	if !c.volvio(gegPlazo + gegMargen) {
		t.Fatalf("hallazgos 2f4af2, f0dfd4, cd9f87: un gate que obedece SIGTERM vuelve enseguida al vencer el plazo")
	}
	if r := c.gate(t, "test"); !gegEsTimeout(r) {
		t.Fatalf("hallazgos 2f4af2, f0dfd4, cd9f87: el gate vencido termina como timeout: %+v", r)
	}

	ctx, cancelar := context.WithCancel(context.Background())
	nuevo := exec.CommandContext(ctx, "sh", "-c", "exec sleep 300")
	if err := procesos.Arrancar(nuevo); err != nil {
		cancelar()
		t.Fatal(err)
	}
	fin := make(chan struct{})
	go func() { nuevo.Wait(); close(fin) }()
	t.Cleanup(func() {
		cancelar() // terminar su contexto mata su grupo
		<-fin
		procesos.Liberar(nuevo.Process.Pid)
	})
	if pasada := time.Since(c.inicio); pasada > gegPlazo+gegGracia/2 {
		t.Fatalf("fixture: el grupo nuevo se arranca antes de que pase la gracia del gate (%v despues del inicio)", pasada)
	}
	time.Sleep(time.Until(c.inicio.Add(gegPlazo + gegGracia + 2*time.Second)))
	if !gegVivo(nuevo.Process.Pid) {
		t.Fatalf("hallazgos 2f4af2, f0dfd4, cd9f87: pasada la gracia del gate vencido, el SIGKILL que quedo para despues no alcanza al grupo nuevo %d", nuevo.Process.Pid)
	}
}

// Hallazgos 2f4af2, f0dfd4 y cd9f87, lo que el arreglo promete (go doc
// TerminarGrupo: "the later SIGKILL checks the registry"): el SIGKILL de
// despues de la gracia solo le llega a un grupo que hoom tiene registrado.
// Un grupo vivo que no es de hoom (como el que heredo el numero de uno ya
// liberado) y que ignora SIGTERM sigue vivo pasada la gracia. Es la
// condicion que protege del numero reusado; no prueba que se mire en el
// momento del disparo y no al llamar (para eso haria falta el mismo numero
// registrado al llamar y ajeno al disparar).
func TestHallazgo2f4af2_SIGKILLTardioSoloAUnGrupoDeHoom(t *testing.T) {
	d := t.TempDir()
	ajeno := exec.Command("sh", "-c", "trap '' TERM\n"+gegEscribir(d, "ajeno.pid", "$$")+"exec sleep 300\n")
	ajeno.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := ajeno.Start(); err != nil {
		t.Fatal(err)
	}
	fin := make(chan struct{})
	go func() { ajeno.Wait(); close(fin) }()
	t.Cleanup(func() {
		syscall.Kill(-ajeno.Process.Pid, syscall.SIGKILL)
		<-fin
	})
	pid := gegLeerPID(t, d, "ajeno.pid") // con el trap ya puesto

	const gracia = time.Second
	if err := procesos.TerminarGrupo(pid, gracia); err != nil {
		t.Fatalf("fixture: TerminarGrupo: %v", err)
	}
	select {
	case <-fin:
		t.Fatalf("hallazgos 2f4af2, f0dfd4, cd9f87: el SIGKILL de despues de la gracia solo le llega a un grupo que hoom tiene registrado; mato al grupo ajeno %d (%v)", pid, ajeno.ProcessState)
	case <-time.After(gracia + 2*time.Second):
	}
}
