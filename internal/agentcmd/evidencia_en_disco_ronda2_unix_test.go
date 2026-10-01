//go:build !windows

// Carreras del hallazgo 98faf2 de la segunda ronda de review de
// .hoom/specs/evidencia-en-disco.md (CA-442: un symlink o un FIFO "nunca se
// sigue ni se lee"): la foto decide sobre el descriptor que abre, no sobre
// una mirada anterior a la ruta. Un proceso que la corrida dejo vivo puede
// cambiar la entrada entre la mirada y la apertura; la foto no se cuelga, no
// lee un destino de afuera ni el FIFO, y no le pone a la ruta la huella de
// otra cosa.
package agentcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// edr2Carrera es cuanto dura cada carrera.
const edr2Carrera = 1500 * time.Millisecond

// edr2RelojPorFoto es cuanto puede tardar UNA foto durante la carrera: mas es
// que siguio un symlink a un FIFO o abrio un FIFO.
const edr2RelojPorFoto = 5 * time.Second

// edr2Intercambiador cambia destino, con rename atomico y sin parar, entre
// un archivo regular con contenido regular, un symlink a fifo y el mismo
// fifo (un enlace duro, asi cualquier lectura colgada se suelta por la ruta
// de fifo). Devuelve la funcion que lo para y dice cuantos cambios hizo.
func edr2Intercambiador(t *testing.T, destino, fifo string, regular []byte) func() int {
	t.Helper()
	escenario := t.TempDir() // mismo disco que destino: rename atomico
	var parar atomic.Bool
	var cambios atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !parar.Load(); i++ {
			p := filepath.Join(escenario, fmt.Sprintf("e%d", i))
			var err error
			switch i % 4 {
			case 0, 2:
				err = os.WriteFile(p, regular, 0o644)
			case 1:
				err = os.Symlink(fifo, p)
			case 3:
				err = os.Link(fifo, p)
			}
			if err == nil {
				err = os.Rename(p, destino)
			}
			if err != nil {
				t.Errorf("fixture: el intercambiador no pudo cambiar %s: %v", destino, err)
				return
			}
			cambios.Add(1)
		}
	}()
	var una sync.Once
	stop := func() int {
		una.Do(func() {
			parar.Store(true)
			wg.Wait()
		})
		return int(cambios.Load())
	}
	t.Cleanup(func() { stop() })
	return stop
}

// CA-442 (hallazgo 98faf2, propiedad de carrera): mientras otro proceso
// cambia una evidencia con nombre valido de hallazgo, con rename atomico y
// sin parar, entre un archivo regular, un symlink a un FIFO de afuera y el
// FIFO mismo (que nadie escribe), CADA Take vuelve (con reloj por foto: si
// una se cuelga, el test falla en vez de colgarse) y la huella de esa ruta,
// cuando la trae, es la del archivo regular o una de HuellaNoRegular /
// HuellaIlegible: nunca la del contenido del FIFO ni la de otra cosa. Lo
// vea git o no (con git mirando, la foto tambien pasa por gitx).
func TestCA442_TakeNoSeCuelgaNiSigueLaEvidenciaQueCambiaEntreRegularYFIFO(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, c := range []struct {
		caso   string
		oculto bool
	}{
		{"git la ignora", true},
		{"git la ve", false},
	} {
		t.Run(c.caso, func(t *testing.T) {
			t.Parallel()
			root := repo(t)
			if c.oculto {
				qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
			}
			const rel = ".hoom/findings/20261001T060606_ca2e2a.json"
			regular := edr1HallazgoJSON("20261001T060606_ca2e2a", "high")
			write(t, root, rel, string(regular))
			if c.oculto != qcr1Ignorado(t, root, rel) {
				t.Fatalf("CA-442: fixture: %s: git ignora %s = %v", c.caso, rel, c.oculto)
			}
			fifo := filepath.Join(t.TempDir(), "fifo")
			edr1Fifo(t, fifo)

			parar := edr2Intercambiador(t, filepath.Join(root, rel), fifo, regular)
			fin := time.Now().Add(edr2Carrera)
			fotos := 0
			vistas := map[string]int{}
			for time.Now().Before(fin) {
				ch := make(chan Snapshot, 1)
				go func() { ch <- Take(root, "main") }()
				var s Snapshot
				select {
				case s = <-ch:
				case <-time.After(edr2RelojPorFoto):
					parar()
					t.Fatalf("CA-442: %s: Take no volvio en %s mientras %s cambiaba entre un archivo regular y un FIFO: siguio o abrio el FIFO",
						c.caso, edr2RelojPorFoto, rel)
				}
				fotos++
				h, ok := s.Huellas[rel]
				if !ok {
					continue
				}
				switch {
				case strings.ToLower(h) == edSHA(regular):
					vistas["regular"]++
				case strings.HasPrefix(h, HuellaNoRegular):
					vistas["no regular"]++
				case strings.HasPrefix(h, HuellaIlegible):
					vistas["ilegible"]++
				default:
					parar()
					t.Fatalf("CA-442: %s: la huella de %s es la del archivo regular, %q o %q: %q (el sha256 de un FIFO vacio es %s)",
						c.caso, rel, HuellaNoRegular, HuellaIlegible, h, edSHA(nil))
				}
			}
			cambios := parar()
			if cambios < 4 {
				t.Fatalf("CA-442: fixture: el intercambiador cambio %s %d veces", rel, cambios)
			}
			t.Logf("CA-442: %s: %d fotos, %d cambios, huellas vistas %v", c.caso, fotos, cambios, vistas)
		})
	}
}
