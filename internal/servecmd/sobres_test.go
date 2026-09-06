// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-196, CA-209, CA-210, CA-214): el Studio ve los sobres y los runs de
// OTROS procesos, los mira sin manejarlos, y distingue el ruido de la CLI.
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
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// runAjeno deja en disco lo que otro proceso habria dejado: el sidecar del run
// y su narracion.
func runAjeno(t *testing.T, dir, id string, meta runcmd.Meta, evs []providers.Event) {
	t.Helper()
	runs := filepath.Join(dir, ".hoom", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(runs, id+".meta.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, ev := range evs {
		line, _ := json.Marshal(ev)
		b.Write(append(line, '\n'))
	}
	if err := os.WriteFile(filepath.Join(runs, id+".jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// CA-209: los sobres se sirven; los runs mezclan memoria y disco; un run ajeno
// se abre con sus eventos leidos del jsonl y NO acepta input ni cancel.
func TestCA209_ElStudioVeLoQueCorreOtroProceso(t *testing.T) {
	dir := newGitProject(t)
	srv := newServer(t, dir)
	now := time.Now().UTC()
	cost := 0.5
	runAjeno(t, dir, "20260906T230000_zzz",
		runcmd.Meta{ID: "20260906T230000_zzz", Provider: "codex", Role: "reviewer", Dir: dir,
			CreatedAt: now, Status: runcmd.StatusRunning, ExitCode: -1, Isolated: true,
			Usage: &providers.Usage{CostUSD: &cost, Turns: 2}},
		[]providers.Event{
			{TS: now, Kind: "start", Detail: "run"},
			{TS: now, Kind: "system", Detail: "hook SessionStart:startup arranco"},
			{TS: now, Kind: "text", Detail: "revisando"},
		})
	envelope.Write(dir, envelope.Record{ID: "20260906T225959_yyy", Role: "reviewer", Provider: "codex",
		Stage: "run", Step: 2, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: now, RunID: "20260906T230000_zzz"})

	// los sobres
	var sobres []envelope.Record
	get(t, srv, "/api/envelopes", &sobres)
	if len(sobres) != 1 || sobres[0].Role != "reviewer" || sobres[0].Step != 2 {
		t.Fatalf("CA-209: /api/envelopes devuelve los sobres: %+v", sobres)
	}

	// los runs: el de otro proceso aparece, con su rol y su gasto
	var runs []RunRow
	get(t, srv, "/api/runs", &runs)
	if len(runs) != 1 {
		t.Fatalf("CA-209: el run de otro proceso aparece: %+v", runs)
	}
	r := runs[0]
	if !r.Foreign || r.Provider != "codex" || r.Role != "reviewer" || !r.Isolated {
		t.Fatalf("CA-209: con su identidad y marcado como ajeno: %+v", r)
	}
	if r.Usage == nil || r.Usage.CostUSD == nil || *r.Usage.CostUSD != 0.5 {
		t.Fatalf("CA-210: y con su gasto: %+v", r.Usage)
	}

	// sus eventos salen del jsonl
	var detalle struct {
		Run    RunRow            `json:"run"`
		Events []providers.Event `json:"events"`
		Next   int               `json:"next"`
	}
	get(t, srv, "/api/runs/20260906T230000_zzz", &detalle)
	if len(detalle.Events) != 3 || detalle.Events[1].Kind != "system" {
		t.Fatalf("CA-209: los eventos de un run ajeno se leen del disco: %+v", detalle.Events)
	}
	if detalle.Next != 3 || !detalle.Run.Foreign {
		t.Fatalf("CA-209: el detalle dice que es ajeno y cuantos eventos van: %+v", detalle)
	}

	// el escenario tambien
	var stage runcmd.StageView
	get(t, srv, "/api/runs/20260906T230000_zzz/stage", &stage)
	if stage.Status != runcmd.StatusRunning || stage.Actors[0].Acts != 1 {
		t.Fatalf("CA-195/CA-209: el escenario de un run ajeno se computa igual (y el ruido no actua): %+v", stage.Actors[0])
	}

	// pero no se maneja
	for _, ruta := range []string{"/api/runs/20260906T230000_zzz/input", "/api/runs/20260906T230000_zzz/cancel"} {
		rec := doPOST(t, srv, ruta, srv.token, []byte(`{"prompt":"seguimos"}`), "application/json")
		if rec.Code != http.StatusConflict {
			t.Fatalf("CA-209: %s sobre un run ajeno debe ser 409, fue %d: %s", ruta, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), "solo lectura") {
			t.Fatalf("CA-209: y decir por que: %s", rec.Body)
		}
	}
}

// CA-214: la negativa de Input bajo strict es 400 (el run existe y esta
// perfecto; lo que no se puede es continuarlo) y el run sigue consultable.
func TestCA214_LaNegativaDeInputEs400(t *testing.T) {
	if code := runErrCode(runcmd.ErrNoContinuation{Provider: "gemini", RunID: "r1"}); code != http.StatusBadRequest {
		t.Fatalf("CA-214: ErrNoContinuation es 400, fue %d", code)
	}
	if code := runErrCode(runcmd.ErrBusy{RunID: "r1"}); code != http.StatusConflict {
		t.Fatalf("CA-214: y no se pisa con el 409 de ErrBusy, fue %d", code)
	}

	dir := newGitProject(t)
	srv := newServer(t, dir)
	fakeProviderNamed(t, "gemini", "printf 'hola\\n'\nexit 0\n")
	rec := doPOST(t, srv, "/api/runs", srv.token, []byte(`{"provider":"gemini","prompt":"hola"}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-214: el run tiene que arrancar: %d %s", rec.Code, rec.Body)
	}
	var run runcmd.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	pollRun(t, srv, run.ID)

	// el Studio no arranca runs con strict, asi que la negativa se prueba en
	// su traduccion HTTP; lo que si se comprueba aca es que el run sobrevive
	// consultable a cualquier rechazo
	var detalle map[string]any
	get(t, srv, "/api/runs/"+run.ID, &detalle)
	if detalle["run"] == nil {
		t.Fatalf("CA-214: el run sigue consultable despues del rechazo: %v", detalle)
	}
	_ = dir
}

// CA-196: la UI embebida distingue el ruido de la CLI y deja ocultarlo, sin
// pedirle nada a la red.
func TestCA196_LaUIDistingueElRuido(t *testing.T) {
	raw, err := fs.ReadFile(uiFS, "ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, quiero := range []string{".fev.k-system", "sin-ruido", "ocultar ruido de la CLI", "chk-ruido"} {
		if !strings.Contains(html, quiero) {
			t.Fatalf("CA-196: la UI debe distinguir y poder ocultar el ruido (%q)", quiero)
		}
	}
	// CA-210: y pintar los sobres con su paso y su estado
	for _, quiero := range []string{"/api/envelopes", "refreshEnvelopes", "paso ${e.step}/${e.steps}", "ENTREGABLE"} {
		if !strings.Contains(html, quiero) {
			t.Fatalf("CA-210: la UI debe pintar los sobres (%q)", quiero)
		}
	}
	// y el meta del run con rol y gasto
	for _, quiero := range []string{"rol <b>", "gasto(r.usage)", "solo lectura"} {
		if !strings.Contains(html, quiero) {
			t.Fatalf("CA-210: el meta del run muestra rol, gasto y si es ajeno (%q)", quiero)
		}
	}
}

// --- helpers ---

// fakeProviderNamed instala un CLI falso con el nombre que haga falta.
func fakeProviderNamed(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func get(t *testing.T, srv *Server, path string, into any) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("GET %s: %v (%s)", path, err, rec.Body)
	}
}
