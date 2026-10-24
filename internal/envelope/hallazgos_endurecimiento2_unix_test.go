//go:build !windows

// Tests de regresion de la tarea endurecimiento-2 en envelope que se arman
// con un FIFO (solo en unix). Escritos desde el texto del hallazgo y el
// contrato de la correccion, SIN LEER LA IMPLEMENTACION. Los helpers de este
// archivo llevan el prefijo e2.
//
//   - 20261023T132601_c6c300 (medium, resilience): EnsureIgnored leia
//     .hoom/.gitignore siguiendo enlaces y bloqueandose en un FIFO, y
//     envelope.Write lo llama antes de escribir el registro: un FIFO ahi
//     colgaba para siempre cada Write y el primer intento de cada Cerrar.
//     Ahora Write y Cerrar vuelven sin colgarse.
package envelope

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// e2FIFO deja .hoom/.gitignore de root como un FIFO que nadie escribe y
// devuelve otro nombre del mismo FIFO (fuera de .hoom), para soltar despues
// cualquier lectura colgada.
func e2FIFO(t *testing.T, root string) string {
	t.Helper()
	gi := filepath.Join(root, ".hoom", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gi); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(gi, 0o644); err != nil {
		t.Skipf("este sistema no deja crear un FIFO: %v", err)
	}
	enlace := filepath.Join(root, "e2-fifo")
	if err := os.Link(gi, enlace); err != nil {
		t.Fatal(err)
	}
	return enlace
}

// e2Tocar abre el FIFO para escribir sin bloquear y lo cierra: una lectura
// colgada en el recibe EOF. Dice si habia alguien leyendo.
func e2Tocar(fifo string) bool {
	f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// e2SinColgarse corre f y espera hasta 5 s a que vuelva. Si no vuelve, falla
// diciendo que se colgo, suelta el FIFO (EOF, tocandolo hasta que f vuelva,
// a lo sumo 15 s) y devuelve false.
func e2SinColgarse(t *testing.T, ca, que, fifo string, f func()) bool {
	t.Helper()
	listo := make(chan struct{})
	go func() {
		defer close(listo)
		f()
	}()
	select {
	case <-listo:
		return true
	case <-time.After(5 * time.Second):
	}
	t.Errorf("%s: con .hoom/.gitignore como un FIFO que nadie escribe, %s vuelve sin colgarse: 5 s despues sigue colgado", ca, que)
	fin := time.Now().Add(15 * time.Second)
	for time.Now().Before(fin) {
		e2Tocar(fifo)
		select {
		case <-listo:
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Errorf("%s: fixture: 15 s despues de soltar el FIFO, %s sigue colgado", ca, que)
	return false
}

// CA-333, CA-202 (hallazgo c6c300): con .hoom/.gitignore como un FIFO que
// nadie escribe, envelope.Write (de un registro nuevo y de uno que ya
// existe) y Cerrar vuelven en menos de 5 s. Si Write o Cerrar dan nil, el
// registro que escribieron esta en disco (su contrato). Si no vuelven, el
// test suelta el FIFO para no dejar nada colgado.
func TestHallazgo_c6c300_CA333_UnFIFOEnGitignoreNoCuelgaWriteNiCerrar(t *testing.T) {
	const ca = "CA-333 (hallazgo c6c300, .hoom/.gitignore como FIFO)"
	for _, cs := range []string{"Write de un registro nuevo", "Write de un registro que ya existe", "Cerrar"} {
		t.Run(cs, func(t *testing.T) {
			root := t.TempDir()
			rec := e1Rec(e1IDPropio, "propio")
			if cs != "Write de un registro nuevo" {
				e1Escribir(t, root, rec)
			}
			fifo := e2FIFO(t, root)
			quiero := rec
			var err error
			ok := e2SinColgarse(t, ca, cs, fifo, func() {
				if cs == "Cerrar" {
					quiero = e1Cerrado(rec)
					err = Cerrar(root, quiero)
					return
				}
				quiero.Note = "escrito con el FIFO en .hoom/.gitignore"
				err = Write(root, quiero)
			})
			if !ok || err != nil {
				t.Logf("%s: %s devolvio %v", ca, cs, err)
				return
			}
			if r, enDisco := e1EnDisco(root, rec.ID); !enDisco || r.Status != quiero.Status || r.Note != quiero.Note {
				t.Errorf("%s: %s dio nil: el registro que escribio esta en disco: %+v %v", ca, cs, r, enDisco)
			}
		})
	}
}
