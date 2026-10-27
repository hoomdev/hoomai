// Tests de regresion de la tercera ronda de endurecimiento-2 en boardcmd,
// sobre el doctor y los avisos del tablero (CA-369, CA-442). Escritos desde
// el texto del hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Reusan e1bProyecto, e1bAvisos, e1Ahora y bdEscribir de los
// tests de endurecimiento-1; los helpers de este archivo llevan el prefijo
// e2c.
//
//   - 20261026T21*_2a04f7 (medium, risk): un sidecar de run con un campo de
//     otro tipo hacia que MetasErr devolviera el error crudo de la
//     decodificacion, con el valor plantado entero, y el doctor y los avisos
//     del tablero lo copiaban y exponian. Ahora ni el error del doctor ni los
//     avisos del tablero traen el valor; el doctor sigue fallando.
package boardcmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/runcmd"
)

// e2cMarca es la marca reconocible dentro del valor plantado.
const e2cMarca = "MARCA-PLANTADA-2a04f7-tablero"

// CA-369, CA-442 (hallazgo 2a04f7): en un proyecto con un item, un sidecar
// .hoom/runs/<id>.meta.json con el id de su archivo y un created_at de ~100
// KiB que no es una fecha, con una marca reconocible: el doctor sigue
// devolviendo error (no certifica lo que no leyo), pero su error no trae la
// marca; y el tablero se arma con avisos que tampoco la traen (ni pesan como
// el valor plantado). Guarda: sin el sidecar plantado, el doctor da un
// reporte sin error.
func TestHallazgo_2a04f7_CA369_ElDoctorYElTableroNoCopianElValorPlantado(t *testing.T) {
	const ca = "CA-369, CA-442 (hallazgo 2a04f7, valor plantado en un sidecar, tablero y doctor)"
	root := e1bProyecto(t)
	if _, err := Doctor(root, "main", "high", e1Ahora); err != nil {
		t.Fatalf("%s: guarda: sin el sidecar plantado, Doctor da un reporte sin error: %v", ca, err)
	}
	antes := e1bAvisos(t, ca, root)

	const id = "20261026T210000_e2c0a1"
	raw, err := json.MarshalIndent(runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: "precios", Dir: root,
		CreatedAt: e1Ahora.Add(-time.Hour), Status: runcmd.StatusDone}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	plantado, _ := json.Marshal(strings.Repeat("x", 50<<10) + e2cMarca + strings.Repeat("y", 50<<10))
	m["created_at"] = plantado
	raw, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	bdEscribir(t, root, filepath.Join(".hoom", "runs", id+".meta.json"), string(raw)+"\n")

	if r, err := Doctor(root, "main", "high", e1Ahora); err == nil {
		t.Errorf("%s: con un sidecar que no se puede leer como meta, Doctor sigue devolviendo error; dio un reporte de %d problemas", ca, len(r.Problems))
	} else if msg := err.Error(); strings.Contains(msg, e2cMarca) || len(msg) > 4096 {
		t.Errorf("%s: el error del doctor no copia el valor plantado: contiene la marca: %v, pesa %d bytes", ca, strings.Contains(msg, e2cMarca), len(msg))
	}

	b, err := Build(root, "main", "high", e1Ahora)
	if err != nil {
		t.Fatalf("%s: el tablero se arma igual: %v", ca, err)
	}
	total := 0
	for _, w := range e1Nuevos(antes, b.Warnings) {
		total += len(w)
		if strings.Contains(w, e2cMarca) {
			t.Errorf("%s: los avisos del tablero no copian el valor plantado: uno trae la marca (%d bytes)", ca, len(w))
		}
	}
	if total > 4096 {
		t.Errorf("%s: los avisos nuevos del tablero no crecen con el valor plantado: pesan %d bytes", ca, total)
	}
}
