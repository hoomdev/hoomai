// Tests adversariales del spec .hoom/specs/studio-v3-cockpit.md
// (CA-25..CA-27, CA-29, CA-30): el cockpit despacha runs con token, un
// writer por arbol, la sesion continua, y la narracion jamas toca la
// evidencia.
package servecmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// fakeProvider instala un `claude` falso al frente del PATH.
func fakeProvider(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

type runResp struct {
	Run    runcmd.Run     `json:"run"`
	Events []runcmd.Event `json:"events"`
	Next   int            `json:"next"`
}

func pollRun(t *testing.T, s *Server, id string) (runcmd.Run, []runcmd.Event) {
	t.Helper()
	var all []runcmd.Event
	after := 0
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/runs/"+id+"?after="+strconv.Itoa(after), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET run: %d %s", rec.Code, rec.Body.String())
		}
		var resp runResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		all = append(all, resp.Events...)
		after = resp.Next
		if resp.Run.Status != runcmd.StatusRunning {
			return resp.Run, all
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("el run nunca termino")
	return runcmd.Run{}, nil
}

func detailsOf(evs []runcmd.Event) string {
	var b strings.Builder
	for _, e := range evs {
		b.WriteString(e.Detail + "\n")
	}
	return b.String()
}

// CA-25: POST /api/runs con token crea el run y el polling incremental
// entrega estado + eventos con los que la UI pinta el feed.
func TestCA25_RunConFeedIncremental(t *testing.T) {
	fakeProvider(t, "echo uno\necho dos\nexit 0\n")
	dir := newProject(t)
	s := newServer(t, dir)

	rec := doPOST(t, s, "/api/runs", s.Token(), []byte(`{"provider":"claude","prompt":"hola"}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-25: esperaba 200, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	var info runcmd.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	final, evs := pollRun(t, s, info.ID)
	if final.Status != runcmd.StatusDone || final.ExitCode != 0 {
		t.Fatalf("CA-25: esperaba done/0: %+v", final)
	}
	out := detailsOf(evs)
	if !strings.Contains(out, "uno") || !strings.Contains(out, "dos") {
		t.Fatalf("CA-25: el feed debe traer la narracion del provider:\n%s", out)
	}
}

// CA-26: mismo arbol => el segundo run recibe 409 con el run en curso;
// worktrees distintos corren en paralelo.
func TestCA26_UnRunPorArbol(t *testing.T) {
	fakeProvider(t, "sleep 2\nexit 0\n")
	dir := newGitProject(t)
	if err := taskcmd.Start(dir, "paralela", "main"); err != nil {
		t.Fatal(err)
	}
	s := newServer(t, dir)

	first := doPOST(t, s, "/api/runs", s.Token(), []byte(`{"provider":"claude","prompt":"uno"}`), "application/json")
	if first.Code != http.StatusOK {
		t.Fatalf("primer run: %d %s", first.Code, first.Body.String())
	}
	second := doPOST(t, s, "/api/runs", s.Token(), []byte(`{"provider":"claude","prompt":"dos"}`), "application/json")
	if second.Code != http.StatusConflict {
		t.Fatalf("CA-26: mismo arbol debe ser 409, fue %d: %s", second.Code, second.Body.String())
	}
	enTarea := doPOST(t, s, "/api/runs", s.Token(), []byte(`{"provider":"claude","prompt":"tres","task":"paralela"}`), "application/json")
	if enTarea.Code != http.StatusOK {
		t.Fatalf("CA-26: otro worktree debe correr en paralelo, fue %d: %s", enTarea.Code, enTarea.Body.String())
	}
}

// CA-27: input continua la MISMA sesion del run usando el mecanismo nativo
// del provider (claude: --continue), sin crear un run nuevo.
func TestCA27_InputContinuaLaSesion(t *testing.T) {
	fakeProvider(t, "echo \"args: $@\"\nexit 0\n")
	dir := newProject(t)
	s := newServer(t, dir)

	rec := doPOST(t, s, "/api/runs", s.Token(), []byte(`{"provider":"claude","prompt":"primer prompt"}`), "application/json")
	var info runcmd.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	pollRun(t, s, info.ID)

	in := doPOST(t, s, "/api/runs/"+info.ID+"/input", s.Token(), []byte(`{"prompt":"spec aprobado, adelante"}`), "application/json")
	if in.Code != http.StatusOK {
		t.Fatalf("CA-27: esperaba 200, obtuve %d: %s", in.Code, in.Body.String())
	}
	final, evs := pollRun(t, s, info.ID)
	if final.ID != info.ID {
		t.Fatalf("CA-27: la continuacion vive en el MISMO run (%s vs %s)", final.ID, info.ID)
	}
	out := detailsOf(evs)
	if !strings.Contains(out, "--continue") || !strings.Contains(out, "spec aprobado, adelante") {
		t.Fatalf("CA-27: la segunda invocacion debe usar --continue con el nuevo prompt:\n%s", out)
	}
}

// CA-31: /api/runs/{id}/stage computa el escenario desde los MISMOS eventos
// del run que alimenta el feed; run inexistente responde 404 JSON.
func TestCA31_StageDesdeElMismoStream(t *testing.T) {
	fakeProvider(t, `echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Task","input":{"subagent_type":"hoom-scout","prompt":"mapea"}}]}}'`+"\nexit 0\n")
	dir := newProject(t)
	s := newServer(t, dir)

	rec := doPOST(t, s, "/api/runs", s.Token(), []byte(`{"provider":"claude","prompt":"hola"}`), "application/json")
	var info runcmd.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	_, evs := pollRun(t, s, info.ID)

	stageRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(stageRec, httptest.NewRequest(http.MethodGet, "/api/runs/"+info.ID+"/stage", nil))
	if stageRec.Code != http.StatusOK {
		t.Fatalf("CA-31: esperaba 200, obtuve %d", stageRec.Code)
	}
	var sv runcmd.StageView
	if err := json.Unmarshal(stageRec.Body.Bytes(), &sv); err != nil {
		t.Fatal(err)
	}
	finalInfo, allEvs, err := s.runs.Events(info.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	expected := runcmd.Stage(finalInfo, allEvs)
	if len(sv.Actors) != len(expected.Actors) || sv.Status != expected.Status {
		t.Fatalf("CA-31: el stage del endpoint debe salir de los mismos eventos del run:\napi: %+v\nesperado: %+v", sv, expected)
	}
	found := false
	for _, a := range sv.Actors {
		if a.Role == "scout" && a.Acts == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("CA-31: la delegacion del feed (%d eventos) debe verse en el escenario: %+v", len(evs), sv.Actors)
	}

	notFound := httptest.NewRecorder()
	s.Handler().ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/api/runs/no-existe/stage", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("CA-31: run inexistente debe ser 404, fue %d", notFound.Code)
	}
}

// CA-29: la narracion jamas toca la evidencia: con basura en .hoom/runs/,
// verify sigue emitiendo su veredicto y check sigue verde.
func TestCA29_NarracionNoTocaEvidencia(t *testing.T) {
	dir := newProject(t)
	s := newServer(t, dir)

	runsDir := filepath.Join(dir, ".hoom", "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "basura.jsonl"), []byte("{esto no es json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-29: verify no puede verse afectado por .hoom/runs/: %d %s", rec.Code, rec.Body.String())
	}
	var v struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Verdict != "green" {
		t.Fatalf("CA-29: el veredicto (gates ausentes) debe ser green, es %q", v.Verdict)
	}
	if res, err := checkcmd.Run(dir, "main"); err != nil || !res.OK {
		t.Fatalf("CA-29: el check no puede leer .hoom/runs/: %+v err=%v", res, err)
	}
}

// CA-30: sin token valido, 401 y CERO efectos: el provider ni se ejecuta.
func TestCA30_SinTokenNoSeLanzaNada(t *testing.T) {
	dir := newProject(t)
	sentinel := filepath.Join(t.TempDir(), "ejecutado")
	fakeProvider(t, "touch "+sentinel+"\nexit 0\n")
	s := newServer(t, dir)

	casos := []string{"/api/runs", "/api/runs/xxx/input", "/api/runs/xxx/cancel"}
	for _, path := range casos {
		rec := doPOST(t, s, path, "token-falso", []byte(`{"provider":"claude","prompt":"hola"}`), "application/json")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("CA-30: POST %s sin token valido debe ser 401, fue %d", path, rec.Code)
		}
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("CA-30: el provider se ejecuto pese al 401")
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom", "runs")); err == nil {
		t.Fatal("CA-30: el 401 no puede haber escrito logs de runs")
	}
}
