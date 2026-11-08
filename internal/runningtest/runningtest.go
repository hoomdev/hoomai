// Package runningtest arma, SOLO para tests, el proyecto en disco con
// tarjetas en curso de CA-519 (.hoom/specs/tablero-en-la-terminal.md:
// Running gana timeout_s y last_event_at) y lo que se exige del JSON que lo
// emite: hoom board --json (boardcmd), hoom item show --json (itemcmd) y
// /api/board (servecmd) prueban lo mismo sobre el mismo proyecto.
//
// Existe solo para tests, como net/http/httptest: ningun codigo de
// produccion lo importa. Es un paquete y no un _test.go porque lo usan los
// tests de tres paquetes, y con una copia en cada uno el proyecto, la
// busqueda en el JSON y lo que se exige eran tres fuentes de verdad casi
// iguales que habia que mover juntas (hallazgo 20261108T230948_50d437). No
// importa ninguno de los tres paquetes que lo usan (boardcmd, itemcmd,
// servecmd): solo lo que el proyecto necesita para escribirse.
package runningtest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Proyecto es un proyecto hoom en disco (git con gittest.EntornoSolo, ningun
// provider) con tres tarjetas en curso, vivas por el PID del test:
//   - con-sobre: un sobre en run 3/6 cuyo run tiene timeout_s 2700 y una
//     narracion con eventos a now-10s y now-2s y una ultima linea a medio
//     escribir;
//   - suelto: un run suelto (sin sobre) de reviewer con timeout_s 1800 y un
//     evento a now-7s;
//   - sin-run: un sobre en spec 1/6 que todavia no tiene run.
//
// Devuelve la raiz (sin symlinks) y el now de los registros (UTC, al
// segundo).
func Proyecto(t testing.TB) (string, time.Time) {
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

// Running busca, en un JSON cualquiera, el objeto running de la tarjeta
// slug: un objeto con "slug": slug que tiene "running" (o el running de su
// "card").
func Running(v any, slug string) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		if s, _ := x["slug"].(string); s == slug {
			if r, ok := x["running"].(map[string]any); ok {
				return r, true
			}
		}
		for _, k := range []string{"card"} {
			if c, ok := x[k].(map[string]any); ok {
				if r, ok := Running(c, slug); ok {
					return r, true
				}
			}
		}
		for _, w := range x {
			if r, ok := Running(w, slug); ok {
				return r, true
			}
		}
	case []any:
		for _, w := range x {
			if r, ok := Running(w, slug); ok {
				return r, true
			}
		}
	}
	return nil, false
}

// RevisarJSON exige, en el JSON raw, que el running de cada tarjeta de slugs
// traiga las claves timeout_s y last_event_at con los valores de Proyecto
// (now es el suyo): con-sobre 2700 y now-2s (el ultimo evento completo),
// suelto 1800 y now-7s, sin-run 0 y null (sin eventos). ca y que nombran el
// criterio y la salida en los mensajes.
func RevisarJSON(t testing.TB, ca, que string, raw []byte, now time.Time, slugs ...string) {
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
		r, ok := Running(v, slug)
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
