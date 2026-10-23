//go:build !windows

// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// verifycmd que necesitan un FIFO (el de internal/carreratest, que nadie
// escribe y que se suelta al final del test). Escritos desde el spec, el
// texto del hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Reusan foConUmbral y foAlta de findings_open_test.go; los
// helpers de este archivo llevan el prefijo e1b.
//
//   - 20261022T202055_53c2af (high, risk): el tablero, el doctor y verify
//     leen los directorios de evidencia; un FIFO con nombre de registro en
//     .hoom/reviews, .hoom/verdicts o .hoom/findings no puede colgar a
//     ninguno (CA-442: un FIFO "nunca se sigue ni se lee"). Aca, verify:
//     vuelve en tiempo acotado.
package verifycmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// e1bReloj es lo mas que puede tardar verify con el FIFO: sus gates son
// "true" y sin el FIFO tarda menos de un segundo.
const e1bReloj = 20 * time.Second

// CA-442 (hallazgo 53c2af, verify): con findings.block_on (verify cuenta los
// hallazgos con findings_open), un FIFO que nadie escribe en el lugar de un
// registro de .hoom/findings, de .hoom/verdicts, o llamado x.json en
// .hoom/reviews, no cuelga verify: Run vuelve en tiempo acotado. Guarda:
// sin el FIFO, Run da un veredicto.
func TestHallazgo_53c2af_CA442_FIFOEnLaEvidenciaNoCuelgaVerify(t *testing.T) {
	for _, dir := range []string{"findings", "verdicts", "reviews"} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			ca := "CA-442 (hallazgo 53c2af, verify con un FIFO en .hoom/" + dir + ")"
			m := foConUmbral(t, "high")
			f := foAlta(t, m, "low", "risk", "precios")
			v, _, err := Run(m, Options{})
			if err != nil || v == nil {
				t.Fatalf("%s: guarda: sin el FIFO, verify da un veredicto: %v", ca, err)
			}

			var fifo string
			switch dir {
			case "findings":
				fifo = filepath.Join(m.Dir, ".hoom", "findings", f.ID+".json")
			case "verdicts":
				entradas, err := os.ReadDir(filepath.Join(m.Dir, ".hoom", "verdicts"))
				if err != nil || len(entradas) == 0 {
					t.Fatalf("fixture: verify dejo su veredicto en .hoom/verdicts: %v %v", entradas, err)
				}
				fifo = filepath.Join(m.Dir, ".hoom", "verdicts", entradas[0].Name())
			case "reviews":
				fifo = filepath.Join(m.Dir, ".hoom", "reviews", "x.json")
			}
			if err := os.Remove(fifo); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			carreratest.Fifo(t, fifo)

			ch := make(chan error, 1)
			t0 := time.Now()
			go func() { _, _, err := Run(m, Options{}); ch <- err }()
			select {
			case err := <-ch:
				t.Logf("%s: verify volvio en %s (err=%v)", ca, time.Since(t0).Round(time.Millisecond), err)
			case <-time.After(e1bReloj):
				t.Fatalf("%s: verify no volvio en %s: se quedo abriendo el FIFO %s", ca, e1bReloj, filepath.Base(fifo))
			}
		})
	}
}
