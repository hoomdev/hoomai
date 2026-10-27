// Tests de regresion de la tercera ronda de endurecimiento-2 en runcmd,
// sobre .hoom/specs/evidencia-en-disco.md (CA-442) y el doctor del tablero.
// Escritos desde el texto del hallazgo y el contrato de la correccion, SIN
// LEER LA IMPLEMENTACION. Reusan e2Valido, e2Roto, e2Sidecar, e2MetaJSON y
// e2IDs (hallazgos_endurecimiento2_test.go) y e2bConCampo
// (hallazgos_endurecimiento2b_test.go); los helpers de este archivo llevan el
// prefijo e2c.
//
//   - 20261026T21*_2a04f7 (medium, risk): por un sidecar con un campo de
//     otro tipo, MetasErr devolvia el error crudo de la decodificacion, que
//     trae el valor plantado entero (un created_at de hasta 1 MiB), y los
//     juntaba todos: el doctor y los avisos del tablero copiaban y exponian
//     ese contenido y crecian con el. Ahora el error dice que sidecar y que
//     falla, sin el valor; y sigue siendo un error.
package runcmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const (
	// e2cMarca es la marca reconocible dentro del valor plantado.
	e2cMarca = "MARCA-PLANTADA-2a04f7"
	// e2cTopeError es lo mas que puede pesar el error por cada sidecar
	// plantado: el nombre del archivo y una razon, no el valor.
	e2cTopeError = 1024
)

// e2cPlantado es un string JSON de ~100 KiB con e2cMarca en el medio: no es
// una fecha.
func e2cPlantado() string {
	v := strings.Repeat("x", 50<<10) + e2cMarca + strings.Repeat("y", 50<<10)
	raw, _ := json.Marshal(v)
	return string(raw)
}

// CA-442 (hallazgo 2a04f7): con un sidecar legible y, al lado, 1 o 20
// sidecars con el id de su archivo y un created_at (o ended_at) plantado de
// ~100 KiB que no es una fecha, con una marca reconocible, MetasErr sigue
// dando error (fail-closed) y devolviendo el legible, pero el error no trae
// el valor plantado: no contiene la marca ni pesa mas de 1 KiB por sidecar
// plantado. Guarda: el error nombra los sidecars que no pudo leer.
func TestHallazgo_2a04f7_CA442_ElErrorDeMetasErrNoCopiaElValorPlantado(t *testing.T) {
	const ca = "CA-442 (hallazgo 2a04f7, valor plantado en un sidecar)"
	for _, campo := range []string{"created_at", "ended_at"} {
		for _, n := range []int{1, 20} {
			t.Run(fmt.Sprintf("%s, %d sidecars", campo, n), func(t *testing.T) {
				root := t.TempDir()
				e2Sidecar(t, root, e2Valido+".meta.json", e2MetaJSON(t, e2Valido, "done"))
				plantado := e2cPlantado()
				var ids []string
				for i := 0; i < n; i++ {
					id := fmt.Sprintf("20261026T2100%02d_e2c%03d", i%60, i)
					raw := e2bConCampo(t, campo, plantado)
					raw = []byte(strings.Replace(string(raw), e2Roto, id, 1))
					e2Sidecar(t, root, id+".meta.json", raw)
					ids = append(ids, id)
				}

				ms, err := MetasErr(root)
				if err == nil {
					t.Fatalf("%s: con %d sidecars con un %s plantado que no es una fecha, MetasErr sigue dando error: dio %v sin error", ca, n, campo, e2IDs(ms))
				}
				if got := e2IDs(ms); len(got) != 1 || got[0] != e2Valido {
					t.Errorf("%s: MetasErr devuelve igual el meta legible y solo ese: %v", ca, got)
				}
				msg := err.Error()
				if strings.Contains(msg, e2cMarca) {
					t.Errorf("%s: el error de MetasErr no copia el valor plantado en %s: contiene la marca %q (%d bytes de error)", ca, campo, e2cMarca, len(msg))
				}
				if len(msg) > n*e2cTopeError {
					t.Errorf("%s: el error de MetasErr no crece con el valor plantado: pesa %d bytes con %d sidecars (tope %d)", ca, len(msg), n, n*e2cTopeError)
				}
				if !strings.Contains(msg, ids[0]) {
					t.Errorf("%s: guarda: el error nombra el sidecar que no pudo leer (%s): %.300s", ca, ids[0], msg)
				}
			})
		}
	}
}
