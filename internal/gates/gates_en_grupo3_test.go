//go:build !windows

// Ronda 3 de gates-en-grupo, hallazgo 20261027T170436_6599fe (low): la
// salida de cada gate iba a un archivo con nombre en TMPDIR, que quedaba ahi
// cuando hoom terminaba por una senal (o por SIGKILL) con el gate corriendo:
// cada Ctrl-C a hoom verify dejaba un archivo con la salida del gate, que
// puede traer lo que una herramienta imprima.
//
// El hoom de estos tests es este mismo binario corriendo
// TestAyudanteGatesEnGrupo3Hoom en un subproceso con un TMPDIR propio y
// vacio: TMPDIR no se toca en el proceso de los tests. Escritos desde el
// contrato y el hallazgo, sin leer la implementacion. Reusan los ayudantes
// de las rondas 1 y 2 (gegLeerPID, gegEscribir, gegMuere, geg2Proc).
package gates

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	// geg3EnvDir y geg3EnvCmd llevan al hoom ayudante su carpeta y el
	// comando de su gate; su TMPDIR va aparte, en la variable de siempre.
	geg3EnvDir = "HOOM_TEST_GATES_EN_GRUPO3_DIR"
	geg3EnvCmd = "HOOM_TEST_GATES_EN_GRUPO3_CMD"
	// geg3Marca esta en la salida del gate: un archivo que la contiene es
	// un archivo con esa salida.
	geg3Marca = "SALIDA-DEL-GATE-6599fe"
)

// TestAyudanteGatesEnGrupo3Hoom no es un test: es el hoom verify de los tests
// de 6599fe. Corre el gate test con el comando que le pasan (plazo de 5
// minutos, cola de 1 MiB) y, si Run vuelve, deja los resultados en
// <carpeta>/resultado.json, fuera de su TMPDIR.
func TestAyudanteGatesEnGrupo3Hoom(t *testing.T) {
	d, cmd := os.Getenv(geg3EnvDir), os.Getenv(geg3EnvCmd)
	if d == "" || cmd == "" {
		t.Skip("solo corre como proceso ayudante de los tests de 6599fe")
	}
	req := true
	m := &manifest.Manifest{Dir: d, Gates: map[string]manifest.Gate{"test": {Cmd: &cmd, Required: &req}}}
	res := Run(m, gitx.Info{}, Options{Timeout: 5 * time.Minute, TailSize: 1 << 20})
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(d, "resultado.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(d, "resultado.json")); err != nil {
		t.Fatal(err)
	}
}

// geg3Hoom arranca el hoom ayudante en un grupo propio, con el gate cmd y
// TMPDIR=tmp.
func geg3Hoom(t *testing.T, d, tmp, cmd string) *geg2Proc {
	t.Helper()
	hoom := exec.Command(os.Args[0], "-test.run=^TestAyudanteGatesEnGrupo3Hoom$", "-test.count=1")
	// el ultimo valor de una variable repetida es el que vale
	hoom.Env = append(os.Environ(), geg3EnvDir+"="+d, geg3EnvCmd+"="+cmd, "TMPDIR="+tmp)
	hoom.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log := filepath.Join(t.TempDir(), "hoom.log")
	salida, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	defer salida.Close()
	hoom.Stdout, hoom.Stderr = salida, salida
	if err := hoom.Start(); err != nil {
		t.Fatal(err)
	}
	p := &geg2Proc{cmd: hoom, log: log, fin: make(chan struct{})}
	go func() { hoom.Wait(); close(p.fin) }()
	t.Cleanup(func() {
		syscall.Kill(-hoom.Process.Pid, syscall.SIGKILL)
		<-p.fin
		if raw, _ := os.ReadFile(log); strings.Contains(string(raw), "DATA RACE") {
			t.Errorf("hallazgo 6599fe: el hoom ayudante tuvo una carrera de datos:\n%s", raw)
		}
	})
	return p
}

// geg3Archivos lista lo que hay en tmp (recursivo) y cuales de esos archivos
// tienen la salida del gate.
func geg3Archivos(t *testing.T, tmp string) (todos, conSalida []string) {
	t.Helper()
	err := filepath.WalkDir(tmp, func(ruta string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ruta == tmp {
			return nil
		}
		todos = append(todos, ruta)
		if e.Type().IsRegular() {
			if raw, err := os.ReadFile(ruta); err == nil && strings.Contains(string(raw), geg3Marca) {
				conSalida = append(conSalida, ruta)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fixture: no se pudo recorrer el TMPDIR del hoom ayudante %s: %v", tmp, err)
	}
	return todos, conSalida
}

// Hallazgo 6599fe: hoom que muere con un gate corriendo, que ya escribio
// salida (por stdout y por stderr), no deja en su TMPDIR ningun archivo con
// esa salida: ni cuando termina por SIGINT (el Ctrl-C, despues de terminar
// el gate) ni cuando lo matan con SIGKILL (sin poder hacer nada).
func TestHallazgo6599fe_HoomQueMuereNoDejaLaSalidaDelGateEnTMPDIR(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGKILL} {
		t.Run(sig.String(), func(t *testing.T) {
			d, tmp := t.TempDir(), t.TempDir()
			hoom := geg3Hoom(t, d, tmp, "echo "+geg3Marca+"-stdout\n"+
				"echo "+geg3Marca+"-stderr >&2\n"+
				// los PID despues de la salida: cuando el test los ve, la
				// salida ya esta escrita
				gegEscribir(d, "lider.pid", "$$")+
				"sleep 300 &\n"+
				gegEscribir(d, "hijo.pid", "$!")+
				"wait\n")
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
			todos, conSalida := geg3Archivos(t, tmp)
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
	hoom := geg3Hoom(t, d, tmp, "echo "+geg3Marca+"-inicio\n"+
		"i=1; while [ $i -le 5000 ]; do echo linea-$i; i=$((i+1)); done\n"+
		"echo "+geg3Marca+"-stderr >&2\n"+
		"echo "+geg3Marca+"-fin\n"+
		"exit 0\n")
	if !hoom.termino(gegTope) {
		t.Fatalf("hallazgo 6599fe: una corrida normal de un gate que sale enseguida termina enseguida")
	}
	raw, err := os.ReadFile(filepath.Join(d, "resultado.json"))
	if err != nil {
		log, _ := os.ReadFile(hoom.log)
		t.Fatalf("fixture: el hoom ayudante no dejo sus resultados: %v\n%s", err, log)
	}
	var res []verdict.GateResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("fixture: resultados del hoom ayudante: %v\n%s", err, raw)
	}
	var r *verdict.GateResult
	for i := range res {
		if res[i].Name == "test" {
			r = &res[i]
		}
	}
	if r == nil {
		t.Fatalf("fixture: el hoom ayudante no devolvio el gate test: %s", raw)
	}
	if r.Status != verdict.StatusPass || r.ExitCode != 0 {
		t.Fatalf("hallazgo 6599fe: el gate salio con exit 0: pasa: %+v", *r)
	}
	quiere := []string{geg3Marca + "-inicio"}
	for i := 1; i <= 5000; i++ {
		quiere = append(quiere, "linea-"+strconv.Itoa(i))
	}
	quiere = append(quiere, geg3Marca+"-stderr", geg3Marca+"-fin")
	if got := strings.TrimSpace(r.OutputTail); got != strings.Join(quiere, "\n") {
		n := len(got)
		if n > 300 {
			got = got[:150] + " ... " + got[n-150:]
		}
		t.Fatalf("hallazgo 6599fe: el gate devuelve su salida completa y en orden (%d bytes): %q", n, got)
	}
	if todos, _ := geg3Archivos(t, tmp); len(todos) > 0 {
		t.Fatalf("hallazgo 6599fe: una corrida normal no deja nada en el TMPDIR de hoom: %v", todos)
	}
}
