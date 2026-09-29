// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (enmienda 1: CA-406, CA-407): el gasto por lente sale en la salida, en el
// Result y en el registro, que tambien dice modelo, esfuerzo, aislamiento y
// evidencia; un registro viejo se sigue leyendo. Los fixtures estan en
// review_aislada_helpers_test.go.
package reviewcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// ---------------------------------------------------------------- CA-406

// CA-406: cada pasada imprime su gasto con los numeros de providers.Usage,
// despues de su linea de hallazgos; el resumen imprime el total de las 4
// lentes antes de 'registro'; Result.passes[].usage los trae; el sobre de la
// review sigue sin gasto (CA-334).
func TestCA406_GastoPorPasadaYTotal(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	raInstalar(t, bin, "codex", raUsoCodex)

	res, out := raRevisar(t, "CA-406", root, Options{Provider: "codex"})
	if res.Status != "revisado" || len(res.Passes) != 4 {
		t.Fatalf("CA-406: fixture: las 4 lentes corren: %+v\n%s", res, out)
	}
	lineas := raLineas(out)
	for i := 1; i <= 4; i++ {
		ini := raLinea(lineas, 0, fmt.Sprintf("  [%d/4] ", i))
		fin := raLinea(lineas, ini+1, "  [")
		if fin < 0 {
			fin = len(lineas)
		}
		h := raLinea(lineas, ini, "    hallazgos ")
		g := raLinea(lineas, ini, "    gasto     ")
		quiero := fmt.Sprintf("    gasto     entrada %d - cache %d - salida %d - turnos 1", i*1000, i*100, i*10)
		if ini < 0 || h < 0 || g < 0 || !(h < g && g < fin) || lineas[g] != quiero {
			t.Fatalf("CA-406: la pasada %d imprime %q despues de sus hallazgos:\n%s", i, quiero, out)
		}
		u := res.Passes[i-1].Usage
		if u == nil || u.InputTokens != i*1000 || u.CachedTokens != i*100 || u.OutputTokens != i*10 || u.Turns != 1 {
			t.Fatalf("CA-406: Result.passes[%d].usage trae lo que informo el provider: %+v", i-1, u)
		}
	}
	total := "  gasto       4 lentes: entrada 10000 - cache 1000 - salida 100"
	gt := raLinea(lineas, 0, "  gasto       ")
	reg := raLinea(lineas, 0, "  registro    ")
	if gt < 0 || lineas[gt] != total || reg < 0 || gt > reg || gt < raLinea(lineas, 0, "  [4/4] ") {
		t.Fatalf("CA-406: el resumen imprime %q antes de registro:\n%s", total, out)
	}
	raw, _ := json.Marshal(res)
	var m struct {
		Passes []map[string]json.RawMessage `json:"passes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || len(m.Passes) != 4 {
		t.Fatal(err)
	}
	for i, p := range m.Passes {
		if _, ok := p["usage"]; !ok {
			t.Fatalf("CA-406: el JSON de la pasada %d trae usage: %s", i+1, raw)
		}
	}
	recs := envelope.List(root)
	if len(recs) != 1 || !recs[0].Usage.Empty() {
		t.Fatalf("CA-406/CA-334: el sobre de la review sigue sin gasto: %+v", recs)
	}
}

// CA-406: con una lente el resumen dice '1 lente:'; una pasada sin consumo
// informado lo dice, y su usage queda omitido.
func TestCA406_UnaLenteYSinConsumo(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	raInstalar(t, bin, "codex", raUsoCodex)
	res, out := raRevisar(t, "CA-406", root, Options{Provider: "codex", Lens: "risk"})
	if !strings.Contains(out, "\n    gasto     entrada 1000 - cache 100 - salida 10 - turnos 1\n") ||
		!strings.Contains(out, "\n  gasto       1 lente: entrada 1000 - cache 100 - salida 10\n") {
		t.Fatalf("CA-406: una lente: gasto de la pasada y '1 lente:' en el resumen:\n%s", out)
	}
	if res.Passes[0].Usage == nil {
		t.Fatalf("CA-406: la pasada trae su usage: %+v", res.Passes[0])
	}

	root2 := raRepo(t, "")
	raCambio(t, root2)
	bin2 := raPATH(t)
	raInstalar(t, bin2, "codex", "")
	res, out = raRevisar(t, "CA-406", root2, Options{Provider: "codex", Lens: "risk"})
	if !strings.Contains(out, "\n    gasto     el provider no informo consumo\n") {
		t.Fatalf("CA-406: sin consumo informado la pasada lo dice:\n%s", out)
	}
	if res.Passes[0].Usage != nil {
		t.Fatalf("CA-406: sin datos Pass.Usage es nil: %+v", res.Passes[0].Usage)
	}
	raw, _ := json.Marshal(res.Passes[0])
	if strings.Contains(string(raw), `"usage"`) {
		t.Fatalf("CA-406: sin datos el JSON de la pasada omite usage: %s", raw)
	}
}

// ---------------------------------------------------------------- CA-407

// CA-407: el registro trae model, effort, isolated, evidence_bytes,
// evidence_sha256 y usage con una entrada por pasada CON datos (aca, las
// pares: reliability y readability); el total solo suma lo informado.
func TestCA407_RegistroConModeloEsfuerzoEvidenciaYGasto(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	raInstalar(t, bin, "codex", raUsoPares)
	ev := raEvidenciaCruda(t, "CA-407", root, "")

	res, out := raRevisar(t, "CA-407", root, Options{Provider: "codex", Model: "m1", Effort: "e1"})
	if res.Status != "revisado" || res.RecordID == "" {
		t.Fatalf("CA-407: fixture: la review termina revisada: %+v\n%s", res, out)
	}
	m := raRegistroCrudo(t, root, res.RecordID)
	if m["model"] != "m1" || m["effort"] != "e1" || m["isolated"] != true {
		t.Fatalf("CA-407: el registro trae model, effort e isolated: %v", m)
	}
	if m["evidence_bytes"] != float64(ev.Bytes) || m["evidence_sha256"] != ev.SHA256 {
		t.Fatalf("CA-407: el registro trae evidence_bytes %d y evidence_sha256 %s: %v", ev.Bytes, ev.SHA256, m)
	}
	usos, ok := m["usage"].([]any)
	if !ok || len(usos) != 2 {
		t.Fatalf("CA-407: usage es una lista con una entrada por pasada con datos (2): %v", m["usage"])
	}
	quiero := []map[string]any{
		{"lens": "reliability", "input_tokens": 2000.0, "cached_input_tokens": 200.0, "output_tokens": 20.0, "turns": 1.0},
		{"lens": "readability", "input_tokens": 4000.0, "cached_input_tokens": 400.0, "output_tokens": 40.0, "turns": 1.0},
	}
	for i, u := range usos {
		e, _ := u.(map[string]any)
		if !reflect.DeepEqual(e, quiero[i]) {
			t.Fatalf("CA-407: la entrada %d de usage es %v, fue %v", i, quiero[i], e)
		}
	}
	recs := raRegistros(t, root)
	if len(recs) != 1 || len(recs[0].Usage) != 2 || recs[0].Usage[0].Lens != "reliability" || recs[0].Usage[1].InputTokens != 4000 {
		t.Fatalf("CA-407: Records lee el gasto por lente: %+v", recs)
	}
	// CA-406 (caso hostil): el total suma solo las pasadas que informaron, y
	// las que no, lo dicen
	if !strings.Contains(out, "\n  gasto       4 lentes: entrada 6000 - cache 600 - salida 60\n") ||
		strings.Count(out, "\n    gasto     el provider no informo consumo\n") != 2 {
		t.Fatalf("CA-406: el total suma solo las pasadas con datos (2 y 4) y las otras dos dicen que no informaron:\n%s", out)
	}
}

// CA-407: sin consumo informado usage es [] (nunca null), y el JSON del
// Result trae model, effort, isolated, evidence_bytes y evidence_sha256.
func TestCA407_SinConsumoUsageEsListaVacia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-407", root, Options{Provider: "codex", Lens: "risk"})
	if res.RecordID == "" {
		t.Fatalf("CA-407: fixture: la review deja registro: %+v\n%s", res, out)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", RecordsDir, res.RecordID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if u, ok := m["usage"]; !ok || strings.TrimSpace(string(u)) != "[]" {
		t.Fatalf("CA-407: sin datos usage es [] y no null: %s", raw)
	}
	for _, k := range []string{"model", "effort", "isolated", "evidence_bytes", "evidence_sha256"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("CA-407: al registro le falta %q: %s", k, raw)
		}
	}
	rj, _ := json.Marshal(res)
	var r map[string]json.RawMessage
	json.Unmarshal(rj, &r)
	for _, k := range []string{"model", "effort", "isolated", "evidence_bytes", "evidence_sha256"} {
		if _, ok := r[k]; !ok {
			t.Fatalf("CA-407: al JSON del Result le falta %q: %s", k, rj)
		}
	}
	if string(r["isolated"]) != "true" || string(r["evidence_bytes"]) == "0" {
		t.Fatalf("CA-407: el Result dice isolated true y los bytes de la evidencia: %s", rj)
	}
}

// CA-407: un registro viejo, sin model/effort/isolated/evidence/usage, se
// sigue leyendo sin avisos y con los campos en cero. Guarda: los lectores de
// hoy ya lo leen; la implementacion no puede volverlos estrictos.
func TestCA407_RegistroViejoSeSigueLeyendo(t *testing.T) {
	dir := t.TempDir()
	viejo := `{
  "id": "20260920T100000_v1ej0a",
  "created_at": "2026-09-20T10:00:00Z",
  "task": "precios",
  "spec": ".hoom/specs/precios.md",
  "fingerprint": "abc",
  "verdict_id": "v1",
  "verdict": "green",
  "lenses": ["readability", "reliability", "resilience", "risk"],
  "provider": "codex",
  "writer": "claude",
  "cross": "cruzada",
  "writers_declared": [],
  "findings": []
}
`
	write(t, dir, filepath.Join(".hoom", RecordsDir, "20260920T100000_v1ej0a.json"), viejo)
	recs, avisos := Records(dir)
	if len(avisos) != 0 || len(recs) != 1 {
		t.Fatalf("CA-407: un registro viejo se lee sin avisos: %v %+v", avisos, recs)
	}
	r := recs[0]
	if r.Model != "" || r.Effort != "" || r.Isolated || r.EvidenceBytes != 0 || r.EvidenceSHA256 != "" || len(r.Usage) != 0 {
		t.Fatalf("CA-407: lo que el registro viejo no dice queda en cero: %+v", r)
	}
	if r.Provider != "codex" || r.Cross != CrossYes || len(r.Lenses) != 4 {
		t.Fatalf("CA-407: lo que el registro viejo dice se conserva: %+v", r)
	}
}
