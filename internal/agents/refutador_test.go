// Tests adversariales del spec .hoom/specs/hallazgos-y-refutador.md
// (CA-57, CA-58): los contratos del Refutador y del Reviewer viven
// embebidos en el binario — la disciplina no depende de conversaciones.
package agents

import (
	"io/fs"
	"strings"
	"testing"
)

// CA-57: el contrato del Refutador exige evidencia deterministica, prohibe
// editar codigo, fija el tope de 2 ciclos y escala al humano.
func TestCA57_ContratoRefutador(t *testing.T) {
	raw, err := fs.ReadFile(assetsFS, "assets/agents/09-refutador.md")
	if err != nil {
		t.Fatal(err)
	}
	contrato := string(raw)
	for _, marca := range []string{
		"DETERMINISTICA",
		"PROHIBIDO refutar por opinion",
		"JAMAS edita",
		"maximo 2 ciclos",
		"ESCALA a Hoom",
		"hoom finding resolve",
		"hoom finding list --open",
	} {
		if !strings.Contains(contrato, marca) {
			t.Fatalf("CA-57: el contrato del refutador no contiene %q", marca)
		}
	}
}

// CA-58: el contrato del Reviewer manda registrar los hallazgos con
// hoom finding add — el chat no es registro.
func TestCA58_ReviewerRegistraHallazgos(t *testing.T) {
	raw, err := fs.ReadFile(assetsFS, "assets/agents/06-reviewer.md")
	if err != nil {
		t.Fatal(err)
	}
	contrato := string(raw)
	for _, marca := range []string{"hoom finding add", "chat NO es registro", ".hoom/findings/"} {
		if !strings.Contains(contrato, marca) {
			t.Fatalf("CA-58: el contrato del reviewer no contiene %q", marca)
		}
	}
}
