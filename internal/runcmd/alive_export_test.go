// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-282, CA-283): runcmd.Alive es el `alive` de siempre, exportado, para
// que el tablero resuelva la vida del dueño de un run con la MISMA regla que
// usa el manager.
package runcmd

import (
	"os"
	"os/exec"
	"testing"
)

// alPIDMuerto devuelve el pid de un proceso que ya termino y fue recogido.
func alPIDMuerto(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("no pude correr 'true': %v", err)
	}
	return cmd.ProcessState.Pid()
}

// CA-282: el PID de un proceso vivo (este mismo test) esta vivo.
// CA-283: el de un proceso terminado no; tampoco 0 ni negativos (sidecars
// viejos sin PID).
func TestCA282_AliveExportado(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatalf("CA-282: el proceso del test esta vivo (pid %d)", os.Getpid())
	}
	muerto := alPIDMuerto(t)
	if Alive(muerto) {
		t.Fatalf("CA-283: un proceso terminado no esta vivo (pid %d)", muerto)
	}
	for _, pid := range []int{0, -1} {
		if Alive(pid) {
			t.Fatalf("CA-283: el pid %d no es de nadie", pid)
		}
	}
	// la misma respuesta que la funcion interna: no pueden divergir
	for _, pid := range []int{os.Getpid(), muerto, 0} {
		if Alive(pid) != alive(pid) {
			t.Fatalf("CA-282: Alive(%d) difiere de alive(%d)", pid, pid)
		}
	}
}
