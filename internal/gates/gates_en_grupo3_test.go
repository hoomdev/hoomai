//go:build !windows

// Ronda 3 de gates-en-grupo, hallazgo 20261027T170436_6599fe (low): la
// salida de cada gate iba a un archivo con nombre en TMPDIR, que quedaba ahi
// cuando hoom terminaba por una senal (o por SIGKILL) con el gate corriendo:
// cada Ctrl-C a hoom verify dejaba un archivo con la salida del gate, que
// puede traer lo que una herramienta imprima.
//
// El hoom de estos tests es este mismo binario corriendo
// TestAyudanteHoomConUnGate (gates_en_grupo_ayudantes_test.go) en un
// subproceso con un TMPDIR propio y vacio: TMPDIR no se toca en el proceso de
// los tests. Escritos desde el contrato y el hallazgo, sin leer la
// implementacion.
package gates

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// gegMarcaDeSalida esta en la salida del gate: un archivo que la contiene es
// un archivo con esa salida.
const gegMarcaDeSalida = "SALIDA-DEL-GATE-6599fe"

// gegHoomConTMPDIR es el hoom ayudante de la carpeta d con el gate cmd,
// TMPDIR=tmp y una cola de 1 MiB.
func gegHoomConTMPDIR(t *testing.T, d, tmp, cmd, etiqueta string) *gegHoom {
	t.Helper()
	return gegArrancarHoom(t, gegConUnGate(d, cmd, etiqueta, "TMPDIR="+tmp, gegEnvAyudanteCola+"=1048576"))
}

// Hallazgo 6599fe: hoom que muere con un gate corriendo, que ya escribio
// salida (por stdout y por stderr), no deja en su TMPDIR ningun archivo con
// esa salida: ni cuando termina por SIGINT (el Ctrl-C, despues de terminar
// el gate) ni cuando lo matan con SIGKILL (sin poder hacer nada).
func TestHallazgo6599fe_HoomQueMuereNoDejaLaSalidaDelGateEnTMPDIR(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGKILL} {
		t.Run(sig.String(), func(t *testing.T) {
			d, tmp := t.TempDir(), t.TempDir()
			hoom := gegHoomConTMPDIR(t, d, tmp, "echo "+gegMarcaDeSalida+"-stdout\n"+
				"echo "+gegMarcaDeSalida+"-stderr >&2\n"+
				// los PID despues de la salida: cuando el test los ve, la
				// salida ya esta escrita
				gegEscribir(d, "lider.pid", "$$")+
				"sleep 300 &\n"+
				gegEscribir(d, "hijo.pid", "$!")+
				"wait\n", "hallazgo 6599fe")
			lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

			if err := syscall.Kill(hoom.cmd.Process.Pid, sig); err != nil {
				t.Fatal(err)
			}
			if !hoom.termino(gegGracia + gegMargen) {
				t.Fatalf("hallazgo 6599fe: hoom que recibe %v termina en a lo sumo %v", sig, gegGracia+gegMargen)
			}
			if sig == syscall.SIGINT {
				// con SIGINT hoom termina el gate antes de terminar
				for _, pid := range []int{lider, hijo} {
					if !gegMuere(pid, 2*time.Second) {
						t.Fatalf("hallazgo 6599fe: hoom que recibe SIGINT termina su gate: %d sigue vivo", pid)
					}
				}
			}
			// con SIGKILL el gate puede seguir vivo (nadie lo termino): su
			// salida tampoco puede quedar a la vista en TMPDIR
			todos, conSalida := gegArchivos(t, tmp, gegMarcaDeSalida)
			if len(conSalida) > 0 {
				t.Fatalf("hallazgo 6599fe: hoom que muere por %v con un gate corriendo no deja en su TMPDIR un archivo con la salida del gate; quedaron %v (todo lo que hay: %v)", sig, conSalida, todos)
			}
			if len(todos) > 0 {
				t.Logf("hallazgo 6599fe: en el TMPDIR del hoom ayudante quedaron archivos sin la salida del gate: %v", todos)
			}
		})
	}
}

// Guardas de 6599fe: en una corrida normal, sin senal, el gate sigue
// devolviendo su salida completa (stdout y stderr en orden, 5000 lineas,
// con la cola de 1 MiB) y en el TMPDIR de hoom no queda nada.
func TestGatesEnGrupo3_6599fe_CorridaNormalDevuelveLaSalidaYNoDejaNada(t *testing.T) {
	d, tmp := t.TempDir(), t.TempDir()
	hoom := gegHoomConTMPDIR(t, d, tmp, "echo "+gegMarcaDeSalida+"-inicio\n"+
		"i=1; while [ $i -le 5000 ]; do echo linea-$i; i=$((i+1)); done\n"+
		"echo "+gegMarcaDeSalida+"-stderr >&2\n"+
		"echo "+gegMarcaDeSalida+"-fin\n"+
		"exit 0\n", "hallazgo 6599fe")
	if !hoom.termino(gegTope) {
		t.Fatalf("hallazgo 6599fe: una corrida normal de un gate que sale enseguida termina enseguida")
	}
	r := hoom.resultado(t, d)
	if r.Status != verdict.StatusPass || r.ExitCode != 0 {
		t.Fatalf("hallazgo 6599fe: el gate salio con exit 0: pasa: %+v", r)
	}
	quiere := []string{gegMarcaDeSalida + "-inicio"}
	for i := 1; i <= 5000; i++ {
		quiere = append(quiere, "linea-"+strconv.Itoa(i))
	}
	quiere = append(quiere, gegMarcaDeSalida+"-stderr", gegMarcaDeSalida+"-fin")
	if got := strings.TrimSpace(r.OutputTail); got != strings.Join(quiere, "\n") {
		n := len(got)
		if n > 300 {
			got = got[:150] + " ... " + got[n-150:]
		}
		t.Fatalf("hallazgo 6599fe: el gate devuelve su salida completa y en orden (%d bytes): %q", n, got)
	}
	if todos, _ := gegArchivos(t, tmp, gegMarcaDeSalida); len(todos) > 0 {
		t.Fatalf("hallazgo 6599fe: una corrida normal no deja nada en el TMPDIR de hoom: %v", todos)
	}
}
