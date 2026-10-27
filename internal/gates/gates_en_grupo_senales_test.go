//go:build !windows

// Hallazgo 20261023T140133_862449 (medium), la otra cara del grupo propio:
// un gate en su propio grupo de procesos no recibe el Ctrl-C de la terminal
// ni el SIGHUP de una terminal cerrada, asi que hoom, cuando recibe una de
// esas senales (o SIGTERM), mata los grupos de sus gates antes de terminar.
//
// El hoom de estos tests es este mismo binario corriendo
// TestAyudanteHoomConUnGate (gates_en_grupo_ayudantes_test.go), como en los
// tests de b5e187 de runcmd. Escritos desde el contrato (go doc
// ./internal/gates, go doc ./internal/procesos: Senales y Terminar) sin leer
// la implementacion.
package gates

import (
	"syscall"
	"testing"
	"time"
)

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
			// un gate largo: un sh que espera a un sleep de 300 s
			hoom := gegArrancarHoom(t, gegConUnGate(d, gegEscribir(d, "lider.pid", "$$")+
				"sleep 300 &\n"+
				gegEscribir(d, "hijo.pid", "$!")+
				"wait\n", "hallazgo 862449"))
			lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

			hoom.senal(t, caso.senal, caso.terminal)
			if !hoom.termino(gegGracia + gegMargen) {
				t.Fatalf("hallazgo 862449: hoom que recibe %v termina en a lo sumo %v\n%s", caso.senal, gegGracia+gegMargen, hoom.salida())
			}
			for _, pid := range []int{lider, hijo} {
				if !gegMuere(pid, 5*time.Second) {
					t.Fatalf("hallazgo 862449: hoom que recibe %v mata el grupo de su gate antes de terminar: %d sigue vivo (sh %d vivo=%v, hijo %d vivo=%v)", caso.senal, pid, lider, gegVivo(lider), hijo, gegVivo(hijo))
				}
			}
			if !hoom.porLaSenal(caso.senal) {
				t.Fatalf("hallazgo 862449: despues de matar sus gates hoom termina como terminaria siempre con %v: %v", caso.senal, hoom.cmd.ProcessState)
			}
		})
	}
}
