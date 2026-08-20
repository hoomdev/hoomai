// Tests adversariales del spec .hoom/specs/studio-v1-dashboard.md
// (CA-4..CA-10): el Studio es un espejo del harness — mismos datos que el
// CLI, loopback por default, UI autocontenida, degradacion visible.
package servecmd

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/report"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const manifestYAML = `schema: hoom/v1
project: demo
gates:
  test:
    required: false
    cmd: ""
`

func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeVerdict(t *testing.T, dir string, at time.Time, status string) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{
		Project:   "demo",
		CreatedAt: at,
		Gates:     []verdict.GateResult{{Name: "test", Required: true, Status: status, OutputTail: "linea 1\nlinea 2"}},
	}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func serveGET(t *testing.T, dir, path string) *httptest.ResponseRecorder {
	t.Helper()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// CA-4: el default es SOLO loopback y / sirve la UI embebida desde el binario.
func TestCA4_LoopbackYUIEmbebida(t *testing.T) {
	if DefaultAddr != "127.0.0.1:4666" {
		t.Fatalf("CA-4: el default debe ser 127.0.0.1:4666, es %q", DefaultAddr)
	}
	rec := serveGET(t, newProject(t), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-4: GET / debe responder 200, respondio %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") || !strings.Contains(body, "HoomAI Studio") {
		t.Fatalf("CA-4: / debe servir la UI embebida (HTML), obtuve %q...", body[:min(120, len(body))])
	}
}

// CA-5: veredictos del mas nuevo al mas viejo, con paginacion ?n=.
func TestCA5_OrdenYPaginacion(t *testing.T) {
	dir := newProject(t)
	t0 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	writeVerdict(t, dir, t0, verdict.StatusPass)
	writeVerdict(t, dir, t0.Add(time.Hour), verdict.StatusFail)
	nuevo := writeVerdict(t, dir, t0.Add(2*time.Hour), verdict.StatusPass)

	var resp verdictsResp
	if err := json.Unmarshal(serveGET(t, dir, "/api/verdicts").Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 3 || len(resp.Verdicts) != 3 {
		t.Fatalf("CA-5: esperaba 3 veredictos, total=%d len=%d", resp.Total, len(resp.Verdicts))
	}
	if resp.Verdicts[0].ID != nuevo.ID {
		t.Fatalf("CA-5: el primero debe ser el mas nuevo (%s), es %s", nuevo.ID, resp.Verdicts[0].ID)
	}

	if err := json.Unmarshal(serveGET(t, dir, "/api/verdicts?n=2").Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Verdicts) != 2 || resp.Total != 3 {
		t.Fatalf("CA-5: con n=2 esperaba 2 de 3, obtuve %d de %d", len(resp.Verdicts), resp.Total)
	}
}

// CA-6: el detalle trae el output_tail por gate; un id inexistente es 404 JSON.
func TestCA6_DetalleY404(t *testing.T) {
	dir := newProject(t)
	v := writeVerdict(t, dir, time.Now(), verdict.StatusFail)

	rec := serveGET(t, dir, "/api/verdicts/"+v.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-6: detalle debe ser 200, es %d", rec.Code)
	}
	var got verdict.Verdict
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Gates) != 1 || got.Gates[0].OutputTail != "linea 1\nlinea 2" {
		t.Fatalf("CA-6: el detalle debe incluir output_tail: %+v", got.Gates)
	}

	rec = serveGET(t, dir, "/api/verdicts/no-existe")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("CA-6: id inexistente debe ser 404, es %d", rec.Code)
	}
	var e map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e["error"] == "" {
		t.Fatalf("CA-6: el 404 debe tener cuerpo JSON con error, obtuve %q", rec.Body.String())
	}
}

// CA-7: /api/tasks y /api/report emiten EXACTAMENTE los mismos bytes que
// los verbos CLI equivalentes — una sola representacion, dos skins.
func TestCA7_ParidadConCLI(t *testing.T) {
	dir := newProject(t)
	writeVerdict(t, dir, time.Now(), verdict.StatusPass)

	cliTasks, err := taskcmd.JSONBytes(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := serveGET(t, dir, "/api/tasks").Body.Bytes(); !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(cliTasks)) {
		t.Fatalf("CA-7: /api/tasks difiere del CLI:\napi: %s\ncli: %s", got, cliTasks)
	}

	all, _, err := verdict.LoadAllWithWarnings(dir)
	if err != nil {
		t.Fatal(err)
	}
	cliReport, err := report.JSONBytes(all, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := serveGET(t, dir, "/api/report").Body.Bytes(); !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(cliReport)) {
		t.Fatalf("CA-7: /api/report difiere del CLI:\napi: %s\ncli: %s", got, cliReport)
	}
}

// CA-8: un veredicto ilegible no tumba el listado: se omite y se reporta
// en el campo de advertencias.
func TestCA8_VeredictoIlegible(t *testing.T) {
	dir := newProject(t)
	writeVerdict(t, dir, time.Now(), verdict.StatusPass)
	corrupto := filepath.Join(dir, ".hoom", "verdicts", "zzz-corrupto.json")
	if err := os.WriteFile(corrupto, []byte("{esto no es json"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := serveGET(t, dir, "/api/verdicts")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-8: el listado no debe caerse, respondio %d", rec.Code)
	}
	var resp verdictsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Verdicts) != 1 {
		t.Fatalf("CA-8: el veredicto legible debe seguir listado, hay %d", len(resp.Verdicts))
	}
	if len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "zzz-corrupto.json") {
		t.Fatalf("CA-8: la advertencia debe nombrar el archivo ilegible: %v", resp.Warnings)
	}
}

// CA-9: fuera de un proyecto hoom, serve falla con la accion exacta (hoom init).
func TestCA9_FueraDeProyecto(t *testing.T) {
	_, err := New(t.TempDir())
	if err == nil {
		t.Fatal("CA-9: fuera de un proyecto debe fallar")
	}
	if !strings.Contains(err.Error(), "hoom init") {
		t.Fatalf("CA-9: el error debe indicar la accion exacta (hoom init): %v", err)
	}
}

// CA-10: la UI embebida no referencia NINGUN asset externo: el dashboard
// carga con la maquina offline.
func TestCA10_UISinRed(t *testing.T) {
	err := fs.WalkDir(uiFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(uiFS, path)
		if err != nil {
			return err
		}
		for _, marca := range []string{"http:", "https:", "//cdn", "@import"} {
			if bytes.Contains(raw, []byte(marca)) {
				t.Fatalf("CA-10: %s contiene una referencia externa (%q)", path, marca)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
