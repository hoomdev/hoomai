//go:build !windows

// Tests de regresion de la tarea endurecimiento-2 en hoomfs. Escritos desde
// el texto del hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Los helpers de este archivo llevan el prefijo e2.
//
//   - 20261023T132601_c6c300 (medium, resilience): EnsureIgnored leia
//     .hoom/.gitignore con os.ReadFile: seguia un enlace y se bloqueaba si
//     era un FIFO, y con el cada funcion que lo llama (envelope.Write, el
//     primer intento de cada Cerrar). Ahora EnsureIgnored vuelve sin colgarse
//     y no dice que completo algo que no es un archivo comun.
package hoomfs

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// CA-333 (hallazgo c6c300): con .hoom/.gitignore como un FIFO que nadie
// escribe, EnsureIgnored (pidiendo todas las entradas o solo "envelopes")
// vuelve en menos de 5 s. Y no dice que completo el archivo si no lo hizo: o
// devuelve error, o despues .hoom/.gitignore es un archivo comun con las
// entradas pedidas. Si no vuelve, el test suelta el FIFO (EOF) para no dejar
// nada colgado.
func TestHallazgo_c6c300_CA333_EnsureIgnoredConUnFIFONoSeCuelga(t *testing.T) {
	const ca = "CA-333 (hallazgo c6c300, EnsureIgnored con .hoom/.gitignore como FIFO)"
	for _, cs := range []struct {
		nombre string
		pide   []string
	}{
		{"todas las entradas", nil},
		{"solo envelopes", []string{"envelopes"}},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			root := t.TempDir()
			gi := filepath.Join(root, ".hoom", ".gitignore")
			if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(gi, 0o644); err != nil {
				t.Skipf("este sistema no deja crear un FIFO: %v", err)
			}
			enlace := filepath.Join(root, "e2-fifo")
			if err := os.Link(gi, enlace); err != nil {
				t.Fatal(err)
			}

			var err error
			listo := make(chan struct{})
			go func() {
				defer close(listo)
				_, err = EnsureIgnored(root, cs.pide...)
			}()
			select {
			case <-listo:
			case <-time.After(5 * time.Second):
				t.Errorf("%s: con .hoom/.gitignore como un FIFO que nadie escribe, EnsureIgnored(%v) vuelve sin colgarse: 5 s despues sigue colgado", ca, cs.pide)
				fin := time.Now().Add(15 * time.Second)
				for time.Now().Before(fin) {
					if f, e := os.OpenFile(enlace, os.O_WRONLY|syscall.O_NONBLOCK, 0); e == nil {
						f.Close()
					}
					select {
					case <-listo:
						return
					case <-time.After(50 * time.Millisecond):
					}
				}
				t.Fatalf("%s: fixture: 15 s despues de soltar el FIFO, EnsureIgnored sigue colgado", ca)
			}
			if err != nil {
				return
			}
			fi, e := os.Lstat(gi)
			if e != nil || !fi.Mode().IsRegular() {
				t.Fatalf("%s: EnsureIgnored(%v) dio nil con .hoom/.gitignore como FIFO: entonces lo dejo como un archivo comun con las entradas; es %v (%v)", ca, cs.pide, fi.Mode(), e)
			}
			b, e := os.ReadFile(gi)
			if e != nil {
				t.Fatal(e)
			}
			var quiero []string
			for _, l := range strings.Split(GitignoreBody(), "\n") {
				if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
					quiero = append(quiero, l)
				}
			}
			if cs.pide != nil {
				quiero = []string{"envelopes"}
			}
			for _, q := range quiero {
				if !strings.Contains(string(b), q) {
					t.Errorf("%s: EnsureIgnored(%v) dio nil: .hoom/.gitignore tiene %q; tiene %q", ca, cs.pide, q, b)
				}
			}
		})
	}
}
