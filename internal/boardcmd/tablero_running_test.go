// Running gana timeout_s y last_event_at (spec tablero-en-la-terminal,
// CA-519): boardcmd.Build, DetailFor y JSONBytes los traen, desde el disco:
// el timeout_s del sidecar del run (el del sobre, o el del run suelto) y la
// hora del ultimo evento completo de su narracion (null sin eventos).
package boardcmd_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// tltProyecto es un proyecto hoom en disco (git con gittest.EntornoSolo,
// ningun provider) con tres tarjetas en curso, vivas por el PID de este test:
//   - con-sobre: un sobre en run 3/6 cuyo run tiene timeout_s 2700 y una
//     narracion con eventos a now-10s y now-2s y una ultima linea a medio
//     escribir;
//   - suelto: un run suelto (sin sobre) de reviewer con timeout_s 1800 y un
//     evento a now-7s;
//   - sin-run: un sobre en spec 1/6 que todavia no tiene run.
func tltProyecto(t *testing.T) (string, time.Time) {
	t.Helper()
	// git solo con su configuracion, sin tocar el entorno del proceso
	entorno := gittest.EntornoSolo(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = entorno
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	escribir := func(rel, contenido string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(contenido), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	jsonDe := func(v any) string {
		t.Helper()
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return string(raw) + "\n"
	}
	git("init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	git("config", "user.email", "test@hoom.dev")
	git("config", "user.name", "hoom test")
	escribir("hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	escribir(".hoom/.gitignore", hoomfs.GitignoreBody())
	escribir("app.go", "package app\n")
	git("add", "-A")
	git("commit", "-q", "-m", "inicial")
	now := time.Now().UTC().Truncate(time.Second)
	for _, slug := range []string{"con-sobre", "suelto", "sin-run"} {
		escribir(".hoom/items/"+slug+".yaml", "titulo: Tarjeta "+slug+"\ntipo: feature\nprioridad: media\n"+
			"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n")
	}
	yo := os.Getpid()
	evento := func(ts time.Time, detalle string) string {
		return `{"ts":"` + ts.Format(time.RFC3339) + `","kind":"text","detail":"` + detalle + `"}` + "\n"
	}
	escribir(".hoom/envelopes/20261108T120000_sobre1.json", jsonDe(envelope.Record{ID: "20261108T120000_sobre1",
		Role: "writer", Provider: "claude", Task: "con-sobre", Dir: root, RunID: "20261108T120000_run001",
		Stage: "run", Step: 3, Steps: 6, Status: envelope.StatusRunning, PID: yo,
		StartedAt: now.Add(-12 * time.Minute), UpdatedAt: now}))
	escribir(".hoom/runs/20261108T120000_run001.meta.json", jsonDe(runcmd.Meta{ID: "20261108T120000_run001",
		Provider: "claude", Role: "writer", Task: "con-sobre", Dir: root, CreatedAt: now.Add(-12 * time.Minute),
		Status: runcmd.StatusRunning, PID: yo, TimeoutS: 2700}))
	escribir(".hoom/runs/20261108T120000_run001.jsonl", evento(now.Add(-10*time.Second), "uno")+
		evento(now.Add(-2*time.Second), "dos")+`{"ts":"`+now.Format(time.RFC3339)+`","kind":"te`)
	escribir(".hoom/runs/20261108T120000_run002.meta.json", jsonDe(runcmd.Meta{ID: "20261108T120000_run002",
		Provider: "codex", Role: "reviewer", Task: "suelto", Dir: root, CreatedAt: now.Add(-5 * time.Minute),
		Status: runcmd.StatusRunning, PID: yo, TimeoutS: 1800}))
	escribir(".hoom/runs/20261108T120000_run002.jsonl", evento(now.Add(-7*time.Second), "revisando"))
	escribir(".hoom/envelopes/20261108T120000_sobre3.json", jsonDe(envelope.Record{ID: "20261108T120000_sobre3",
		Role: "arquitecto", Provider: "claude", Task: "sin-run", Dir: root,
		Stage: "spec", Step: 1, Steps: 6, Status: envelope.StatusRunning, PID: yo,
		StartedAt: now.Add(-time.Minute), UpdatedAt: now}))
	return root, now
}

// tltRunning busca, en un JSON cualquiera, el objeto running de la tarjeta
// slug: un objeto con "slug": slug que tiene "running" (o el running de su
// "card").
func tltRunning(v any, slug string) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		if s, _ := x["slug"].(string); s == slug {
			if r, ok := x["running"].(map[string]any); ok {
				return r, true
			}
		}
		for _, k := range []string{"card"} {
			if c, ok := x[k].(map[string]any); ok {
				if r, ok := tltRunning(c, slug); ok {
					return r, true
				}
			}
		}
		for _, w := range x {
			if r, ok := tltRunning(w, slug); ok {
				return r, true
			}
		}
	case []any:
		for _, w := range x {
			if r, ok := tltRunning(w, slug); ok {
				return r, true
			}
		}
	}
	return nil, false
}

// tltRevisarJSON exige, en el JSON raw, que el running de cada tarjeta
// traiga timeout_s y last_event_at con sus valores (null sin eventos).
func tltRevisarJSON(t *testing.T, ca, que string, raw []byte, now time.Time, slugs ...string) {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("%s: %s no es JSON: %v\n%s", ca, que, err, raw)
	}
	quiere := map[string][2]any{
		"con-sobre": {2700.0, now.Add(-2 * time.Second).Format(time.RFC3339)},
		"suelto":    {1800.0, now.Add(-7 * time.Second).Format(time.RFC3339)},
		"sin-run":   {0.0, nil},
	}
	for _, slug := range slugs {
		r, ok := tltRunning(v, slug)
		if !ok {
			t.Fatalf("%s: %s trae el running de %s:\n%s", ca, que, slug, raw)
		}
		q := quiere[slug]
		ts, ok1 := r["timeout_s"]
		le, ok2 := r["last_event_at"]
		if !ok1 || !ok2 {
			t.Fatalf("%s: el running de %s en %s trae las claves timeout_s y last_event_at: %v", ca, slug, que, r)
		}
		if ts != q[0] {
			t.Fatalf("%s: en %s el timeout_s de %s es %v, es %v", ca, que, slug, q[0], ts)
		}
		if q[1] == nil {
			if le != nil {
				t.Fatalf("%s: en %s el last_event_at de %s sin eventos es null, es %v", ca, que, slug, le)
			}
			continue
		}
		s, _ := le.(string)
		got, err := time.Parse(time.RFC3339Nano, s)
		w, _ := time.Parse(time.RFC3339, q[1].(string))
		if err != nil || !got.Equal(w) {
			t.Fatalf("%s: en %s el last_event_at de %s es %v (el ultimo evento completo), es %v", ca, que, slug, q[1], le)
		}
	}
}

func tltCard(t *testing.T, b boardcmd.Board, slug string) boardcmd.Card {
	t.Helper()
	for _, c := range b.Columns {
		for _, k := range c.Cards {
			if k.Slug == slug {
				return k
			}
		}
	}
	t.Fatalf("el tablero trae la tarjeta %s", slug)
	return boardcmd.Card{}
}

// CA-519: Build trae en Running el timeout_s del run y la hora del ultimo
// evento completo de su narracion (la linea a medio escribir no cuenta); sin
// run, 0 y nil.
func TestCA519_BuildTraeTimeoutYUltimoEvento(t *testing.T) {
	root, now := tltProyecto(t)
	b, err := boardcmd.Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-519: Build: %v", err)
	}
	casos := []struct {
		slug    string
		timeout int
		ultimo  *time.Time
	}{
		{"con-sobre", 2700, ptr(now.Add(-2 * time.Second))},
		{"suelto", 1800, ptr(now.Add(-7 * time.Second))},
		{"sin-run", 0, nil},
	}
	for _, c := range casos {
		k := tltCard(t, b, c.slug)
		if k.Running == nil {
			t.Fatalf("CA-519: %s esta en curso (su dueno es este proceso)", c.slug)
		}
		if k.Running.TimeoutS != c.timeout {
			t.Fatalf("CA-519: el timeout_s de %s es %d, es %d", c.slug, c.timeout, k.Running.TimeoutS)
		}
		switch {
		case c.ultimo == nil && k.Running.LastEventAt != nil:
			t.Fatalf("CA-519: %s sin narracion: last_event_at nil, es %v", c.slug, k.Running.LastEventAt)
		case c.ultimo != nil && (k.Running.LastEventAt == nil || !k.Running.LastEventAt.Equal(*c.ultimo)):
			t.Fatalf("CA-519: el last_event_at de %s es %v, es %v", c.slug, *c.ultimo, k.Running.LastEventAt)
		}
	}
	d, err := boardcmd.DetailFor(root, "main", "high", "con-sobre", now, false)
	if err != nil {
		t.Fatalf("CA-519: DetailFor: %v", err)
	}
	if r := d.Card.Running; r == nil || r.TimeoutS != 2700 || r.LastEventAt == nil || !r.LastEventAt.Equal(now.Add(-2*time.Second)) {
		t.Fatalf("CA-519: la tarjeta del detalle trae lo mismo: %+v", r)
	}
}

func ptr(t time.Time) *time.Time { return &t }

// CA-519: hoom board --json (JSONBytes) emite timeout_s y last_event_at en
// cada running, con null sin eventos.
func TestCA519_JSONBytesTraeLasClaves(t *testing.T) {
	root, now := tltProyecto(t)
	b, err := boardcmd.Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-519: Build: %v", err)
	}
	raw, err := boardcmd.JSONBytes(b)
	if err != nil {
		t.Fatalf("CA-519: JSONBytes: %v", err)
	}
	tltRevisarJSON(t, "CA-519", "hoom board --json", raw, now, "con-sobre", "suelto", "sin-run")
}
