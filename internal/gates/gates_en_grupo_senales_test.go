//go:build !windows

// Hallazgo 20261023T140133_862449 (medium), la otra cara del grupo propio:
// un gate en su propio grupo de procesos no recibe el Ctrl-C de la terminal
// ni el SIGHUP de una terminal cerrada, asi que hoom, cuando recibe una de
// esas senales (o SIGTERM), mata los grupos de sus gates antes de terminar.
//
// El hoom de estos tests es este mismo binario corriendo
// TestAyudanteGatesEnGrupoVerify, como en los tests de b5e187 de runcmd.
// Escritos desde el contrato (go doc ./internal/gates, go doc
// ./internal/procesos: Senales y Terminar) sin leer la implementacion.
package gates

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// gegEnvDir lleva al proceso ayudante la carpeta donde su gate deja sus PID.
const gegEnvDir = "HOOM_TEST_GATES_EN_GRUPO_DIR"

// TestAyudanteGatesEnGrupoVerify no es un test: es el hoom verify de
// TestHallazgo862449_SenalAHoomMataSusGates. Corre un gate largo (un sh que
// espera a un sleep de 300 s) con un plazo de 5 minutos y se queda ahi.
func TestAyudanteGatesEnGrupoVerify(t *testing.T) {
	d := os.Getenv(gegEnvDir)
	if d == "" {
		t.Skip("solo corre como proceso ayudante de TestHallazgo862449_SenalAHoomMataSusGates")
	}
	req := true
	m := &manifest.Manifest{Dir: d, Gates: map[string]manifest.Gate{"test": {
		Cmd: strPtr(gegEscribir(d, "lider.pid", "$$") +
			"sleep 300 &\n" +
			gegEscribir(d, "hijo.pid", "$!") +
			"wait\n"),
		Required: &req,
	}}}
	Run(m, gitx.Info{}, Options{Timeout: 5 * time.Minute, TailSize: 4096})
}

// Hallazgo 862449: hoom que recibe SIGINT (el Ctrl-C), SIGTERM o SIGHUP
// mientras corre un gate termina, y el gate (el sh y su hijo) muere con el:
// estando en un grupo propio, nadie mas le manda la senal. hoom termina como
// terminaria siempre con esa senal. "terminal" es el Ctrl-C de verdad: la
// senal le llega al grupo de hoom entero, no solo a hoom.
func TestHallazgo862449_SenalAHoomMataSusGates(t *testing.T) {
	casos := []struct {
		nombre   string
		senal    syscall.Signal
		terminal bool
	}{
		{"SIGINT", syscall.SIGINT, false},
		{"SIGTERM", syscall.SIGTERM, false},
		{"SIGHUP", syscall.SIGHUP, false},
		{"SIGINT terminal", syscall.SIGINT, true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			d := t.TempDir()
			hoom := exec.Command(os.Args[0], "-test.run=^TestAyudanteGatesEnGrupoVerify$", "-test.count=1")
			hoom.Env = append(os.Environ(), gegEnvDir+"="+d)
			// su propio grupo, como el de una terminal: el Ctrl-C de
			// "terminal" no le llega al test
			hoom.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			salida, err := os.Create(filepath.Join(t.TempDir(), "hoom.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer salida.Close()
			hoom.Stdout, hoom.Stderr = salida, salida
			if err := hoom.Start(); err != nil {
				t.Fatal(err)
			}
			fin := make(chan struct{})
			go func() { hoom.Wait(); close(fin) }()
			t.Cleanup(func() {
				syscall.Kill(-hoom.Process.Pid, syscall.SIGKILL)
				<-fin
			})
			lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

			destino := hoom.Process.Pid
			if caso.terminal {
				destino = -destino
			}
			if err := syscall.Kill(destino, caso.senal); err != nil {
				t.Fatal(err)
			}
			select {
			case <-fin:
			case <-time.After(gegGracia + gegMargen):
				raw, _ := os.ReadFile(salida.Name())
				t.Fatalf("hallazgo 862449: hoom que recibe %v termina en a lo sumo %v\n%s", caso.senal, gegGracia+gegMargen, raw)
			}
			for _, pid := range []int{lider, hijo} {
				if !gegMuere(pid, 5*time.Second) {
					t.Fatalf("hallazgo 862449: hoom que recibe %v mata el grupo de su gate antes de terminar: %d sigue vivo (sh %d vivo=%v, hijo %d vivo=%v)", caso.senal, pid, lider, gegVivo(lider), hijo, gegVivo(hijo))
				}
			}
			ws, _ := hoom.ProcessState.Sys().(syscall.WaitStatus)
			porLaSenal := ws.Signaled() && ws.Signal() == caso.senal
			comoShell := ws.Exited() && ws.ExitStatus() == 128+int(caso.senal)
			if !porLaSenal && !comoShell {
				t.Fatalf("hallazgo 862449: despues de matar sus gates hoom termina como terminaria siempre con %v: %v", caso.senal, hoom.ProcessState)
			}
		})
	}
}
