// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// `hoom status` (CA-627): cuenta como que bloquean los mismos hallazgos que
// finding.Bloquea (via ContextoDe): una subida valida, un medium en ruta de
// riesgo; un medium o un low comunes no. Escritos desde el spec y las firmas
// publicas, sin leer la implementacion.
package statuscmd

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
)

func crAltaEn(t *testing.T, dir, sev, file, task string) finding.Finding {
	t.Helper()
	f, err := finding.Register(dir, "main", finding.Draft{Severity: sev, Lens: "risk", File: file,
		Description: "hallazgo " + sev, Author: "reviewer@test", Task: task})
	if err != nil {
		t.Fatalf("fixture: Register: %v", err)
	}
	return f
}

// CA-627: status cuenta los mismos que bloquean bajo Bloquea.
func TestCA627_StatusCuentaLosQueBloquean(t *testing.T) {
	dir := initProject(t)
	crAltaEn(t, dir, "high", "app.go", "")
	subido := crAltaEn(t, dir, "low", "app.go", "otra")
	crAltaEn(t, dir, "medium", "internal/pagos/cobro.go", "")
	crAltaEn(t, dir, "medium", "app.go", "")
	crAltaEn(t, dir, "low", "app.go", "")
	if _, _, err := finding.Raise(dir, subido.ID, "toca el cobro"); err != nil {
		t.Fatalf("fixture: Raise: %v", err)
	}
	// una subida invalida no sube nada ni rompe la cuenta
	foCrudo(t, dir, "20260101T000000_cccccc.raise.json", `{"finding_id":"20260101T000000_cccccc","reason":"x","author":"Ana","raised_at":"2026-11-01T00:00:00Z"}`)

	s, err := BuildFor(dir, "main", "high")
	if err != nil {
		t.Fatal(err)
	}
	if s.Findings.Blocking != 3 {
		t.Fatalf("CA-627: bloquean el high, el low subido y el medium en ruta de riesgo (3): %+v", s.Findings)
	}
	if s.Findings.Open != 5 {
		t.Fatalf("CA-627: una subida no es un hallazgo: 5 abiertos: %+v", s.Findings)
	}
	items, _, err := finding.ListConUmbral(dir, "main", "high", true)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, it := range items {
		if it.Bloquea {
			n++
		}
	}
	if n != s.Findings.Blocking {
		t.Fatalf("CA-627: status y la lista juzgada cuentan lo mismo: %d vs %d", s.Findings.Blocking, n)
	}
	// con block_on medium bloquean ademas el medium comun
	s, err = BuildFor(dir, "main", "medium")
	if err != nil || s.Findings.Blocking != 4 {
		t.Fatalf("CA-627: con block_on medium bloquean 4: %+v %v", s.Findings, err)
	}
}
