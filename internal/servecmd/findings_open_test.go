// Tests adversariales del spec .hoom/specs/gate-findings-open.md (CA-256,
// CA-260): el Studio muestra findings_open con el mismo render que cualquier
// gate, expone el umbral en /api/status, no recalcula quien bloquea y deja de
// afirmar que los hallazgos jamas tocan un veredicto.
package servecmd

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func foProyectoStudio(t *testing.T, gatesExtra, findingsYAML string) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@hoom.dev")
	gitRun(t, dir, "config", "user.name", "hoom test")
	body := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" + gatesExtra + findingsYAML
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "inicial")
	return dir
}

func foGET(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// CA-260: GET /api/status expone findings_block_on: el umbral normalizado del
// manifiesto, o "" (la clave existe) cuando los hallazgos no bloquean.
func TestCA260_StatusExponeElUmbral(t *testing.T) {
	for yml, quiero := range map[string]string{
		"findings:\n  block_on: high\n":   "high",
		"findings:\n  block_on: MEDIUM\n": "medium",
		"":                                "",
		"findings: {}\n":                  "",
	} {
		s := newServer(t, foProyectoStudio(t, "", yml))
		rec := foGET(t, s, "/api/status")
		if rec.Code != http.StatusOK {
			t.Fatalf("CA-260: /api/status responde 200, respondio %d: %s", rec.Code, rec.Body.String())
		}
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		v, ok := m["findings_block_on"]
		if !ok || v != quiero {
			t.Fatalf("CA-260: con %q findings_block_on debe ser %q (clave presente): %v", yml, quiero, m)
		}
	}
}

// CA-260: POST /api/verify corre el gate (misma funcion que el CLI), y el
// detalle /api/verdicts/{id} y la tendencia /api/report lo muestran con sus
// notas y su output_tail.
func TestCA260_DetalleYTendenciaTraenElGate(t *testing.T) {
	dir := foProyectoStudio(t, "", "findings:\n  block_on: high\n")
	f, err := finding.Register(dir, "main", finding.Draft{Severity: "high", Lens: "risk", File: "hoom.yaml",
		Description: "high abierto visto desde el Studio", Author: "reviewer@test"})
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(t, dir)

	rec := doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-260: POST /api/verify responde 200, respondio %d: %s", rec.Code, rec.Body.String())
	}
	var v verdict.Verdict
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Verdict != "red" {
		t.Fatalf("CA-260: el verify del Studio es el del CLI: ROJO con un high abierto: %+v", v.Summary)
	}

	rec = foGET(t, s, "/api/verdicts/"+v.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-260: el detalle responde 200, respondio %d", rec.Code)
	}
	var detalle verdict.Verdict
	if err := json.Unmarshal(rec.Body.Bytes(), &detalle); err != nil {
		t.Fatal(err)
	}
	var g *verdict.GateResult
	for i := range detalle.Gates {
		if detalle.Gates[i].Name == "findings_open" {
			g = &detalle.Gates[i]
		}
	}
	if g == nil || g.Status != verdict.StatusFail {
		t.Fatalf("CA-260: /api/verdicts/{id} devuelve findings_open FAIL: %+v", detalle.Gates)
	}
	if !strings.Contains(g.Notes, f.ID) || !strings.Contains(g.OutputTail, "hoom finding resolve "+f.ID) {
		t.Fatalf("CA-260: el detalle trae notas y output_tail del gate: %+v", g)
	}

	rec = foGET(t, s, "/api/report")
	var rep struct {
		Gates map[string]struct {
			Total      int    `json:"total"`
			LastStatus string `json:"last_status"`
		} `json:"gates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if tr, ok := rep.Gates["findings_open"]; !ok || tr.Total != 1 || tr.LastStatus != verdict.StatusFail {
		t.Fatalf("CA-260: la tendencia de /api/report incluye findings_open: %+v", rep.Gates)
	}
}

// CA-260: la UI embebida ya no afirma que los hallazgos "jamas tocan un
// veredicto" y lee findings_block_on para decir si bloquean.
func TestCA260_LaUIYaNoMiente(t *testing.T) {
	raw, err := fs.ReadFile(uiFS, "ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, frase := range []string{"jamás tocan un veredicto", "jamas tocan un veredicto", "jamás tocan el veredicto"} {
		if strings.Contains(html, frase) {
			t.Fatalf("CA-260: la UI no puede seguir diciendo %q", frase)
		}
	}
	for _, marca := range []string{"findings_block_on", "findings_open"} {
		if !strings.Contains(html, marca) {
			t.Fatalf("CA-260: la nota del cajon de hallazgos se arma con %q", marca)
		}
	}
}

// CA-256: POST /api/verify con gates:["findings_open"] es 400 (el mismo
// UsageError que el CLI), aunque hoom.yaml declare un gate con ese nombre, y
// no escribe veredicto ni eventos.
func TestCA256_StudioNoSeleccionaFindingsOpen(t *testing.T) {
	extra := "  findings_open:\n    required: true\n    cmd: \"true\"\n"
	for _, gatesExtra := range []string{"", extra} {
		dir := foProyectoStudio(t, gatesExtra, "findings:\n  block_on: high\n")
		s := newServer(t, dir)
		for _, cuerpo := range []string{`{"gates":["findings_open"]}`, `{"gates":["test","findings_open"]}`} {
			rec := doPOST(t, s, "/api/verify", s.Token(), []byte(cuerpo), "application/json")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("CA-256: %s (declarado=%v) esperaba 400, obtuve %d: %s", cuerpo, gatesExtra != "", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "gate desconocido") || !strings.Contains(rec.Body.String(), "findings_open") {
				t.Fatalf("CA-256: el 400 trae la razon del CLI y nombra el gate: %s", rec.Body.String())
			}
			sinArtefactos(t, "CA-256", dir)
		}
	}
}
