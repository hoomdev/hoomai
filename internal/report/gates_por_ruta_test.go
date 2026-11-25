// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre el
// reporte (CA-815): report.Render imprime NO APLICA con el motivo en la
// linea de abajo y suma no_aplica:<n> a la linea de totales solo cuando hay
// (aclaracion 12); History, Build y JSONBytes descartan no_aplica del
// pass-rate y de la tendencia, como a absent (aclaracion 13).
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package report_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/report"
	"github.com/hoomdev/hoomai/internal/verdict"
)

var gprT0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

var gprMotivo = fmt.Sprintf(gates.MotivoNoAplica, "app/**")

func gprV(at time.Time, id string, gs ...verdict.GateResult) *verdict.Verdict {
	v := &verdict.Verdict{Schema: verdict.SchemaID, ID: id, CreatedAt: at, Project: "demo", Gates: gs}
	v.Finalize()
	return v
}

func gprGate(nombre, status string) verdict.GateResult {
	g := verdict.GateResult{Name: nombre, Required: true, Status: status, Scope: "full", Command: "true"}
	if status == verdict.StatusNoAplica {
		g.Scope, g.Command, g.Notes = "none", "", gprMotivo
	}
	if status == verdict.StatusFail {
		g.ExitCode = 1
	}
	return g
}

func gprLineas(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }

func gprRender(v *verdict.Verdict) string {
	var b bytes.Buffer
	report.Render(&b, v, ".hoom/verdicts/"+v.ID+".json")
	return b.String()
}

// gprHistoria son tres corridas de static: pass, fail y, la mas nueva,
// no_aplica; test pasa en las tres.
func gprHistoria() []*verdict.Verdict {
	return []*verdict.Verdict{
		gprV(gprT0, "20261125T100000_aaaaaa", gprGate("test", verdict.StatusPass), gprGate("static", verdict.StatusPass)),
		gprV(gprT0.Add(time.Hour), "20261125T110000_bbbbbb", gprGate("test", verdict.StatusPass), gprGate("static", verdict.StatusFail)),
		gprV(gprT0.Add(2*time.Hour), "20261125T120000_cccccc", gprGate("test", verdict.StatusPass), gprGate("static", verdict.StatusNoAplica)),
	}
}

// CA-815 y aclaracion 12: el renglon de un gate en no_aplica lleva NO APLICA
// y debajo su motivo; la linea de totales (la de skip:) trae no_aplica:2.
func TestCA815_RenderImprimeNoAplicaConSuMotivo(t *testing.T) {
	const ca = "CA-815"
	v := gprV(gprT0, "20261125T100000_abcdef",
		gprGate("test", verdict.StatusPass), gprGate("static", verdict.StatusNoAplica), gprGate("mutation", verdict.StatusNoAplica))
	out := gprRender(v)
	lineas := gprLineas(out)
	for _, n := range []string{"static", "mutation"} {
		i := -1
		for k, l := range lineas {
			if strings.Contains(l, "NO APLICA") && strings.Contains(l, n) {
				i = k
				break
			}
		}
		if i < 0 {
			t.Fatalf("%s: el renglon de %s lleva NO APLICA:\n%s", ca, n, out)
		}
		if i+1 >= len(lineas) || !strings.Contains(lineas[i+1], gprMotivo) {
			t.Fatalf("%s: debajo del renglon de %s va su motivo %q:\n%s", ca, n, gprMotivo, out)
		}
	}
	total := ""
	for _, l := range lineas {
		if strings.Contains(l, "skip:") {
			total = l
		}
	}
	if total == "" || !strings.Contains(total, "no_aplica:2") {
		t.Fatalf("%s: la linea de totales trae no_aplica:2: %q\n%s", ca, total, out)
	}
}

// CA-815 y aclaracion 12: sin gates en no_aplica nada cambia: ni NO APLICA ni
// no_aplica en ninguna linea.
func TestCA815_RenderSinNoAplicaComoHoy(t *testing.T) {
	const ca = "CA-815"
	v := gprV(gprT0, "20261125T100000_fedcba", gprGate("test", verdict.StatusPass),
		verdict.GateResult{Name: "lint", Status: verdict.StatusAbsent, Scope: "none"})
	out := gprRender(v)
	if strings.Contains(out, "no_aplica") || strings.Contains(out, "NO APLICA") {
		t.Fatalf("%s: un veredicto sin no_aplica se imprime como hoy:\n%s", ca, out)
	}
}

// CA-815 y aclaracion 13: la tendencia de History no cuenta la corrida en
// no_aplica: static queda (1/2), no (1/3); test, (3/3).
func TestCA815_HistoryDescartaNoAplica(t *testing.T) {
	const ca = "CA-815"
	var b bytes.Buffer
	report.History(&b, gprHistoria(), 3)
	out := b.String()
	tiene := func(gate, cuenta string) bool {
		for _, l := range gprLineas(out) {
			if strings.Contains(l, gate) && strings.Contains(l, cuenta) {
				return true
			}
		}
		return false
	}
	if !tiene("static", "(1/2)") || tiene("static", "(1/3)") {
		t.Fatalf("%s: la tendencia de static descarta el no_aplica: (1/2):\n%s", ca, out)
	}
	if !tiene("test", "(3/3)") {
		t.Fatalf("%s: la tendencia de test es (3/3):\n%s", ca, out)
	}
}

// CA-815 y aclaracion 13: Build y JSONBytes no cuentan el no_aplica ni en el
// total ni en los pass, y nunca es el last_status; la corrida que lo tiene
// trae summary.no_aplica.
func TestCA815_BuildYJSONDescartanNoAplica(t *testing.T) {
	const ca = "CA-815"
	all := gprHistoria()
	want := report.GateTrend{Pass: 1, Total: 2, PassRate: 50, LastStatus: verdict.StatusFail}
	if got := report.Build(all, 3).Gates["static"]; got != want {
		t.Fatalf("%s: Build: la tendencia de static es %+v, fue %+v", ca, want, got)
	}
	if got := report.Build(all, 3).Gates["test"]; got.Pass != 3 || got.Total != 3 || got.PassRate != 100 {
		t.Fatalf("%s: Build: test 3/3: %+v", ca, got)
	}

	raw, err := report.JSONBytes(all, 3)
	if err != nil {
		t.Fatal(err)
	}
	var rep report.JSON
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("%s: el JSON es valido: %v\n%s", ca, err, raw)
	}
	if got := rep.Gates["static"]; got != want {
		t.Fatalf("%s: JSONBytes: la tendencia de static es %+v, fue %+v", ca, want, got)
	}
	if len(rep.Runs) != 3 || rep.Runs[0].ID != "20261125T120000_cccccc" || rep.Runs[0].Summary.NoAplica != 1 {
		t.Fatalf("%s: la corrida mas nueva trae summary.no_aplica 1: %+v", ca, rep.Runs)
	}
	var doc struct {
		Runs []struct {
			Summary map[string]int `json:"summary"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Runs[1].Summary["no_aplica"]; ok {
		t.Fatalf("%s: una corrida sin no_aplica no trae la clave: %+v", ca, doc.Runs[1].Summary)
	}
}
