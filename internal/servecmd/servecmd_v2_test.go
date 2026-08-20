// Tests adversariales del spec .hoom/specs/studio-v2-acciones.md
// (CA-13..CA-20): las acciones del Studio ejecutan el mismo codigo que los
// verbos CLI, exigen token, y ningun nombre malicioso escapa del harness.
package servecmd

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newGitProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@hoom.dev")
	gitRun(t, dir, "config", "user.name", "hoom test")
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "inicial")
	return dir
}

func newServer(t *testing.T, dir string) *Server {
	t.Helper()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func doPOST(t *testing.T, s *Server, path, token string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set(TokenHeader, token)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func writeSpecFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, ".hoom", "specs", name+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// CA-13: POST /api/verify produce un veredicto con el mismo esquema que
// hoom verify --json y lo persiste en .hoom/verdicts/ como una corrida CLI.
func TestCA13_VerifyDesdeElStudio(t *testing.T) {
	dir := newProject(t)
	s := newServer(t, dir)

	rec := doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-13: esperaba 200, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	var v verdict.Verdict
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Schema != verdict.SchemaID {
		t.Fatalf("CA-13: el esquema debe ser %q, es %q", verdict.SchemaID, v.Schema)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("CA-13: el veredicto debe persistirse en .hoom/verdicts/ (err=%v, n=%d)", err, len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), v.ID+".json") {
		t.Fatalf("CA-13: el artefacto %q no corresponde al veredicto %q", entries[0].Name(), v.ID)
	}
}

// CA-14: un done que el CLI rechazaria responde error con el MISMO mensaje
// del CLI y la tarea queda abierta.
func TestCA14_DoneRechazadoVerbatim(t *testing.T) {
	dir := newGitProject(t)
	s := newServer(t, dir)

	if rec := doPOST(t, s, "/api/tasks", s.Token(), []byte(`{"slug":"demo"}`), "application/json"); rec.Code != http.StatusOK {
		t.Fatalf("CA-14: task start via Studio fallo: %d %s", rec.Code, rec.Body.String())
	}
	wt := filepath.Join(dir, ".hoom", "worktrees", "demo")
	if err := os.WriteFile(filepath.Join(wt, "sucio.txt"), []byte("sin commitear\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// El rechazo del CLI (sin efectos) es la referencia exacta del mensaje.
	expected := taskcmd.Done(dir, "demo", "main", false)
	if expected == nil {
		t.Fatal("CA-14: el fixture debia ser rechazado por el CLI")
	}

	rec := doPOST(t, s, "/api/tasks/demo/done", s.Token(), nil, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("CA-14: esperaba 409, obtuve %d", rec.Code)
	}
	var e map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e["error"] != expected.Error() {
		t.Fatalf("CA-14: el mensaje debe ser el del CLI, verbatim.\napi: %q\ncli: %q", e["error"], expected.Error())
	}
	tasks, err := taskcmd.Snapshot(dir, "main")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("CA-14: la tarea debe seguir abierta (err=%v, n=%d)", err, len(tasks))
	}
}

// CA-15: nunca corren dos verify a la vez sobre el mismo arbol: el segundo
// recibe 409 mientras el primero esta en curso.
func TestCA15_VerifyConcurrente409(t *testing.T) {
	dir := t.TempDir()
	slowManifest := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: false\n    cmd: \"sleep 1\"\n"
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(slowManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newServer(t, dir)

	var wg sync.WaitGroup
	codes := make(chan int, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		codes <- doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json").Code
	}()
	// esperar a que el primero tome el lock (su gate duerme 1s)
	deadline := time.Now().Add(2 * time.Second)
	for !isVerifyLocked(s) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	codes <- doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json").Code
	wg.Wait()
	close(codes)

	var got []int
	for c := range codes {
		got = append(got, c)
	}
	if !((got[0] == 200 && got[1] == 409) || (got[0] == 409 && got[1] == 200)) {
		t.Fatalf("CA-15: esperaba exactamente un 200 y un 409, obtuve %v", got)
	}
}

func isVerifyLocked(s *Server) bool {
	if s.verifyMu.TryLock() {
		s.verifyMu.Unlock()
		return false
	}
	return true
}

// CA-16: un upload con nombre malicioso queda SOLO bajo .hoom/intake/.
func TestCA16_IntakeSaneaElNombre(t *testing.T) {
	dir := newProject(t)
	s := newServer(t, dir)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "../../../pwn.md")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("# doc del cliente\n"))
	mw.Close()

	rec := doPOST(t, s, "/api/intake", s.Token(), buf.Bytes(), mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-16: esperaba 200, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp["path"], ".hoom/intake/") || strings.Contains(resp["path"], "..") {
		t.Fatalf("CA-16: la ruta debe quedar bajo .hoom/intake/: %q", resp["path"])
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(resp["path"]))); err != nil {
		t.Fatalf("CA-16: el archivo no quedo donde se declaro: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "pwn.md")); err == nil {
		t.Fatal("CA-16: el nombre malicioso escapo del directorio de intake")
	}
}

// CA-17: los criterios del detalle salen de la MISMA regex que spec_lint.
func TestCA17_CriteriosMismaRegex(t *testing.T) {
	dir := newProject(t)
	content := "# Spec fixture\n\n- CA-1: algo verificable\n- CA-2: otra cosa\n"
	writeSpecFixture(t, dir, "demo-spec", content)
	s := newServer(t, dir)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/specs/demo-spec", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-17: esperaba 200, obtuve %d", rec.Code)
	}
	var d struct {
		Markdown string   `json:"markdown"`
		Criteria []string `json:"criteria"`
		State    string   `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Markdown != content {
		t.Fatalf("CA-17: el markdown debe viajar crudo")
	}
	if len(d.Criteria) != 2 || d.Criteria[0] != "CA-1" || d.Criteria[1] != "CA-2" {
		t.Fatalf("CA-17: criterios extraidos incorrectos: %v", d.Criteria)
	}
	if d.State != approval.StatusNotApproved {
		t.Fatalf("CA-17: sin aprobar el estado es no-aprobado, es %q", d.State)
	}
}

// CA-18: aprobar desde la UI produce el mismo registro que el CLI, y editar
// el spec despues lo reporta invalidado en ambas caras.
func TestCA18_AprobacionParidadEInvalidacion(t *testing.T) {
	dir := newGitProject(t)
	specPath := writeSpecFixture(t, dir, "demo-spec", "# Spec\n\nCA-1 contenido aprobable\n")
	s := newServer(t, dir)

	rec := doPOST(t, s, "/api/specs/demo-spec/approve", s.Token(), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-18: esperaba 200, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	// el CLI ve la aprobacion del Studio como propia: mismo registro.
	if _, already, err := approval.Approve(dir, specPath); err != nil || !already {
		t.Fatalf("CA-18: el CLI debe reconocer el registro del Studio (already=%v, err=%v)", already, err)
	}
	if state, _, _ := approval.Status(dir, specPath); state != approval.StatusApproved {
		t.Fatalf("CA-18: hoom spec status debe decir aprobado, dice %q", state)
	}

	if err := os.WriteFile(specPath, []byte("# Spec\n\nCA-1 contenido EDITADO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := approval.Status(dir, specPath); state != approval.StatusInvalidated {
		t.Fatalf("CA-18: tras editar, el CLI debe decir invalidado, dice %q", state)
	}
	getRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/specs/demo-spec", nil))
	var d struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.State != approval.StatusInvalidated {
		t.Fatalf("CA-18: la API debe decir invalidado, dice %q", d.State)
	}
}

// CA-19: sin token valido, 401 y CERO efectos secundarios.
func TestCA19_SinTokenNoHayEfectos(t *testing.T) {
	dir := newGitProject(t)
	writeSpecFixture(t, dir, "demo-spec", "# Spec\n\nCA-1\n")
	s := newServer(t, dir)

	casos := []struct{ path, token string }{
		{"/api/tasks", ""},
		{"/api/tasks", "token-falso"},
		{"/api/specs/demo-spec/approve", ""},
		{"/api/verify", "token-falso"},
	}
	for _, c := range casos {
		rec := doPOST(t, s, c.path, c.token, []byte(`{"slug":"demo"}`), "application/json")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("CA-19: POST %s con token %q debe ser 401, fue %d", c.path, c.token, rec.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom", "worktrees", "demo")); err == nil {
		t.Fatal("CA-19: el 401 no puede haber creado la tarea")
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom", "approvals")); err == nil {
		t.Fatal("CA-19: el 401 no puede haber registrado aprobaciones")
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom", "verdicts")); err == nil {
		t.Fatal("CA-19: el 401 no puede haber corrido un verify")
	}
}

// CA-20: los comentarios de review quedan en .hoom/specs/<name>.review.md.
func TestCA20_ReviewPersistida(t *testing.T) {
	dir := newProject(t)
	writeSpecFixture(t, dir, "demo-spec", "# Spec\n\nCA-1\n")
	s := newServer(t, dir)

	body := []byte(`{"comments":"faltan casos de borde en el criterio 1"}`)
	rec := doPOST(t, s, "/api/specs/demo-spec/review", s.Token(), body, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-20: esperaba 200, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", "specs", "demo-spec.review.md"))
	if err != nil {
		t.Fatalf("CA-20: falta el sidecar de review: %v", err)
	}
	if !strings.Contains(string(raw), "faltan casos de borde en el criterio 1") {
		t.Fatalf("CA-20: el sidecar no contiene los comentarios: %s", raw)
	}
}
