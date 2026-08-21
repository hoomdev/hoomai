// Tests adversariales del spec .hoom/specs/studio-v1-dashboard.md (CA-2):
// el JSON del report lleva la misma informacion que la vista texto —
// historial reciente y pass-rate por gate sobre el historial completo.
package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

func synth(at time.Time, id string, gates ...verdict.GateResult) *verdict.Verdict {
	v := &verdict.Verdict{ID: id, CreatedAt: at, Gates: gates}
	v.Finalize()
	return v
}

// CA-2: pass-rate por gate sobre TODO el historial, runs recientes primero,
// y los estados skipped/absent no cuentan contra la tasa (igual que el texto).
func TestCA2_ReportJSON(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	all := []*verdict.Verdict{
		synth(t0, "v-viejo",
			verdict.GateResult{Name: "test", Required: true, Status: verdict.StatusPass},
			verdict.GateResult{Name: "static", Required: true, Status: verdict.StatusPass},
			verdict.GateResult{Name: "lint", Status: verdict.StatusAbsent},
		),
		synth(t0.Add(time.Hour), "v-nuevo",
			verdict.GateResult{Name: "test", Required: true, Status: verdict.StatusFail},
			verdict.GateResult{Name: "static", Status: verdict.StatusSkipped},
		),
	}

	rep := Build(all, 1)
	if rep.Total != 2 {
		t.Fatalf("CA-2: total debe ser 2, es %d", rep.Total)
	}
	if len(rep.Runs) != 1 || rep.Runs[0].ID != "v-nuevo" {
		t.Fatalf("CA-2: con n=1 el run debe ser el mas reciente: %+v", rep.Runs)
	}
	if rep.Runs[0].Verdict != "red" {
		t.Fatalf("CA-2: el run reciente es rojo, obtuve %q", rep.Runs[0].Verdict)
	}

	test := rep.Gates["test"]
	if test.Pass != 1 || test.Total != 2 || test.PassRate != 50 || test.LastStatus != verdict.StatusFail {
		t.Fatalf("CA-2: tendencia de 'test' incorrecta: %+v", test)
	}
	static := rep.Gates["static"]
	if static.Pass != 1 || static.Total != 1 || static.PassRate != 100 {
		t.Fatalf("CA-2: skipped no debe contar contra la tasa de 'static': %+v", static)
	}
	if _, ok := rep.Gates["lint"]; ok {
		t.Fatalf("CA-2: un gate solo-ausente no aparece en la tendencia (igual que el texto)")
	}
}

// CA-2: sin veredictos el JSON sigue siendo valido y vacio, nunca null.
func TestCA2_ReportJSONVacio(t *testing.T) {
	raw, err := JSONBytes(nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	var rep JSON
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("CA-2: JSON invalido: %v", err)
	}
	if rep.Total != 0 || rep.Runs == nil || len(rep.Runs) != 0 {
		t.Fatalf("CA-2: vacio pero valido, obtuve %s", raw)
	}
	if strings.Contains(string(raw), "\"runs\": null") {
		t.Fatalf("CA-2: runs no puede ser null: %s", raw)
	}
}
