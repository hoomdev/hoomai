//go:build !windows

// Ronda 1 del test-writer, solo Unix: las senales que terminan Run (CA-575,
// CA-533, hallazgo 3ab10c) y Datos.Archivo sobre un FIFO (CA-559, hallazgo
// a6a051). Con la terminal y los datos falsos de tt_run_test.go, y los Datos
// reales de tt_datos_test.go; cada espera con su tiempo maximo.
package tablero_test

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-575, CA-533, hallazgo 3ab10c: SIGTERM, SIGHUP y SIGINT terminan Run como
// Salir: devuelve la terminal (restaura el modo crudo y lo ultimo que escribe
// es SecMouseNo y SecSalir) y sale con 128 + el numero de la senal.
func TestCA575_CA533_RunTerminaConUnaSenal(t *testing.T) {
	for _, s := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT} {
		// una suscripcion propia antes de mandar la senal: sin ella, si Run no
		// la atendiera, la senal terminaria el proceso de los tests
		mia := make(chan os.Signal, 1)
		signal.Notify(mia, s)
		w := ttNuevoMundo(t, ttTableroPrincipal())
		fin := w.correr(nil)
		ttEsperar(t, "CA-575", "el primer cuadro", w, func() bool { return w.marcoCon("[2] Tarjetas") })
		time.Sleep(50 * time.Millisecond)
		if err := syscall.Kill(os.Getpid(), s); err != nil {
			t.Fatal(err)
		}
		code, pan := fin(t)
		signal.Stop(mia)
		if pan != nil {
			t.Fatalf("CA-575 (hallazgo 3ab10c): %v no es un panic: %v", s, pan)
		}
		if code != 128+int(s) {
			t.Fatalf("CA-575 (hallazgo 3ab10c): %v termina Run con 128+%d: %d", s, int(s), code)
		}
		if salida := w.r.texto("escribe"); !strings.HasSuffix(salida, tablero.SecMouseNo+tablero.SecSalir) {
			t.Fatalf("CA-533 (hallazgo 3ab10c): con %v la terminal se devuelve (el mouse apagado, la pantalla normal): %q", s, ttCola(salida, 200))
		}
		if w.r.indice(0, func(e ttSuceso) bool { return e.que == "restaurar" }) < 0 {
			t.Fatalf("CA-533 (hallazgo 3ab10c): con %v se restaura el modo crudo", s)
		}
	}
}

// CA-559, hallazgo a6a051: Datos.Archivo sobre algo que no es un archivo
// regular (un FIFO) vuelve enseguida, sin bloquearse, con el error "no es un
// archivo regular" y ninguna linea.
func TestCA559_DatosDeArchivoSobreUnFIFO(t *testing.T) {
	root, _ := ttProyecto(t)
	tubo := filepath.Join(root, "tubo")
	if err := syscall.Mkfifo(tubo, 0o644); err != nil {
		t.Fatalf("no pude crear el FIFO: %v", err)
	}
	d := ttDatosDe(t, root)
	ch := make(chan tablero.Archivo, 1)
	go func() { ch <- d.Archivo("tubo", 1) }()
	select {
	case a := <-ch:
		if a.Err == nil || !strings.Contains(a.Err.Error(), "no es un archivo regular") || len(a.Lineas) != 0 || a.Fuera {
			t.Fatalf("CA-559 (hallazgo a6a051): un FIFO da el error 'no es un archivo regular' y ninguna linea: %+v", a)
		}
	case <-time.After(3 * time.Second):
		// destrabar la lectura (un escritor que abre y cierra) antes de fallar
		if f, err := os.OpenFile(tubo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			f.Close()
		}
		select {
		case <-ch:
		case <-time.After(time.Second):
		}
		t.Fatalf("CA-559 (hallazgo a6a051): Datos.Archivo sobre un FIFO no se bloquea (no volvio en 3s)")
	}
}
