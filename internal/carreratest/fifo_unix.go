//go:build !windows

package carreratest

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Fifo crea un FIFO en p (y los directorios que falten arriba) que nadie
// escribe: abrirlo para leer sin O_NONBLOCK se cuelga. Al final del test lo
// suelta (Soltar).
func Fifo(t testing.TB, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("fixture: crear el directorio del FIFO %s: %v", p, err)
	}
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Fatalf("fixture: mkfifo %s: %v", p, err)
	}
	Soltar(t, p)
}

// Soltar registra, al final del test, abrir para escribir (sin bloquear)
// cada FIFO unas veces: suelta una lectura que se haya quedado colgada en el
// (de la foto, o de un git hijo), asi el test termina aunque lo que mide se
// haya colgado. Si nadie lo esta leyendo, la apertura falla y no hace nada.
func Soltar(t testing.TB, fifos ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, p := range fifos {
			for i := 0; i < 16; i++ {
				f, err := os.OpenFile(p, os.O_WRONLY|syscall.O_NONBLOCK, 0)
				if err != nil {
					break
				}
				f.Close()
				time.Sleep(10 * time.Millisecond)
			}
		}
	})
}
