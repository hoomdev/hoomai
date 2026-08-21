// Test adversarial del spec .hoom/specs/contexto-salud.md (CA-39): los
// tres modos de arranque del Analista viven en su CONTRATO embebido, no en
// conversaciones — si Fredy manana tiene el intake vacio, su CLI sabe sola
// que debe proponer la reconstruccion o la entrevista fundacional.
package agents

import (
	"io/fs"
	"strings"
	"testing"
)

// CA-39: el contrato embebido del analista contiene los tres modos:
// documento, reconstruccion desde codigo y entrevista fundacional con sus
// 6 preguntas.
func TestCA39_ContratoAnalistaConTresModos(t *testing.T) {
	raw, err := fs.ReadFile(assetsFS, "assets/agents/08-analista.md")
	if err != nil {
		t.Fatal(err)
	}
	contrato := string(raw)

	requeridos := []string{
		"Modos de arranque",
		"Modo A",
		"Modo B",
		"RECONSTRUIDA DESDE CODIGO",
		"Modo C",
		"entrevista",
		"6 preguntas",
		"prohibido inventarla",
	}
	for _, marca := range requeridos {
		if !strings.Contains(contrato, marca) {
			t.Fatalf("CA-39: el contrato del analista no contiene %q", marca)
		}
	}
	// las 6 preguntas de la entrevista fundacional, por su contenido
	for _, pregunta := range []string{"para quien", "modulos", "roles", "innegociables", "NO", "prioridades"} {
		if !strings.Contains(contrato, pregunta) {
			t.Fatalf("CA-39: falta la pregunta sobre %q en la entrevista fundacional", pregunta)
		}
	}
}
