// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-206, CA-207, CA-208): el sobre se ve desde afuera, la identidad del run
// sale del sidecar y no de una frase, y el JSON dice lo mismo que el texto.
package statuscmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// CA-206: los sobres en curso se ven con rol, provider y paso n/N; el ultimo
// cerrado se ve con su resultado; y uno que dejo de moverse se ETIQUETA
// "posible huerfano" —la palabra y el umbral que ya usa verify— sin que hoom
// lo declare muerto.
func TestCA206_LosSobresSeVen(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	now := time.Now().UTC()

	envelope.Write(dir, envelope.Record{
		ID: "20260906T230000_aaa", Role: "test-writer", Provider: "claude", Isolated: true,
		Stage: "run", Step: 3, Steps: 6, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: now.Add(-2 * time.Minute), RunID: "20260906T230001_run",
	})
	envelope.Write(dir, envelope.Record{
		ID: "20260906T220000_bbb", Role: "writer", Provider: "codex",
		Stage: "verify", Step: 4, Steps: 5, Status: envelope.StatusNotDeliverable, ExitCode: 1,
		Note: "veredicto rojo", VerdictID: "v-1", Verdict: "red",
		StartedAt: now.Add(-30 * time.Minute), EndedAt: now.Add(-25 * time.Minute),
	})

	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Envelopes) != 2 {
		t.Fatalf("CA-206: el que corre y el ultimo cerrado: %+v", s.Envelopes)
	}
	if s.Envelopes[0].ID != "20260906T230000_aaa" || s.Envelopes[0].Done() {
		t.Fatalf("CA-206: primero el que esta corriendo: %+v", s.Envelopes[0])
	}
	if s.Envelopes[0].PossibleOrphan {
		t.Fatalf("CA-206: un sobre que se movio hace dos minutos no es huerfano: %+v", s.Envelopes[0])
	}

	var buf bytes.Buffer
	Render(&buf, s, false)
	out := buf.String()
	for _, quiero := range []string{"sobres:", "1 en curso", "rol test-writer", "(ciego)", "paso 3/6", "run",
		"ultimo:", "NO ENTREGABLE", "verify", "veredicto rojo"} {
		if !strings.Contains(out, quiero) {
			t.Fatalf("CA-206: el render debe decir %q:\n%s", quiero, out)
		}
	}

	// el mismo sobre, callado desde hace mas del umbral de verify: se etiqueta
	envelope.Write(dir, envelope.Record{
		ID: "20260906T230000_aaa", Role: "test-writer", Provider: "claude",
		Stage: "run", Step: 3, Steps: 6, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: now.Add(-2 * time.Hour),
	})
	viejo, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	// Write pone UpdatedAt en ahora, asi que el huerfano se simula moviendo el reloj
	viejo.Now = now.Add(live.OrphanAfter + time.Minute)
	viejo.Envelopes = envelopes(dir, viejo.Now)
	var abierto EnvelopeView
	for _, e := range viejo.Envelopes {
		if e.ID == "20260906T230000_aaa" {
			abierto = e
		}
	}
	if !abierto.PossibleOrphan {
		t.Fatalf("CA-206: sin actividad pasado el umbral se etiqueta: %+v", abierto)
	}
	if abierto.Status != envelope.StatusRunning {
		t.Fatalf("CA-206: etiquetar no es cerrar: hoom no declara muerto lo que no vio morir: %+v", abierto)
	}
	buf.Reset()
	Render(&buf, viejo, false)
	if !strings.Contains(buf.String(), "posible huerfano") {
		t.Fatalf("CA-206: el render dice posible huerfano:\n%s", buf.String())
	}
}

// CA-207: la identidad del run sale del SIDECAR. La prueba mas dura: un log
// cuya frase de arranque dice un provider y un sidecar que dice otro — gana el
// sidecar, porque la frase nunca fue un dato.
func TestCA207_LaIdentidadSaleDelSidecar(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	now := time.Now().UTC()
	cost := 0.42
	writeRun(t, dir, "20260906T235959_ccc", []providers.Event{
		{TS: now.Add(-time.Minute), Kind: "start", Detail: "run 20260906T235959_ccc: claude en el proyecto"},
		{TS: now.Add(-time.Second), Kind: "text", Detail: "trabajando"},
	})
	writeRunMeta(t, dir, runcmd.Meta{ID: "20260906T235959_ccc", Provider: "codex", Role: "reviewer",
		Task: "tarea-x", Dir: dir, CreatedAt: now.Add(-time.Minute), Status: runcmd.StatusRunning, ExitCode: -1,
		Isolated: true, Usage: &providers.Usage{CostUSD: &cost, Turns: 3, InputTokens: 1000}})

	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Runs) != 1 {
		t.Fatalf("CA-207: un run activo: %+v", s.Runs)
	}
	r := s.Runs[0]
	if r.Provider != "codex" {
		t.Fatalf("CA-207: el provider sale del sidecar, no de la frase del log: %+v", r)
	}
	if r.Role != "reviewer" || r.Task != "tarea-x" || !r.Isolated {
		t.Fatalf("CA-207: rol, tarea y aislamiento salen del sidecar: %+v", r)
	}
	if r.Usage == nil || r.Usage.CostUSD == nil || *r.Usage.CostUSD != 0.42 {
		t.Fatalf("CA-208: el gasto del run viaja en el snapshot: %+v", r.Usage)
	}

	var buf bytes.Buffer
	Render(&buf, s, false)
	if !strings.Contains(buf.String(), "codex") || strings.Contains(buf.String(), "claude") {
		t.Fatalf("CA-207: el render muestra el provider del sidecar:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "0.42") {
		t.Fatalf("CA-208: el render muestra el gasto del run:\n%s", buf.String())
	}
}

// CA-208: --json trae envelopes y el usage de cada run, y dice lo mismo que el
// texto: una lectura, dos pieles.
func TestCA208_JSONYTextoDicenLoMismo(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	now := time.Now().UTC()
	cost := 1.25
	writeRun(t, dir, "20260906T235959_ddd", []providers.Event{
		{TS: now.Add(-time.Minute), Kind: "start", Detail: "arranque"},
		{TS: now, Kind: "system", Detail: "hook SessionStart:startup arranco"},
	})
	writeRunMeta(t, dir, runcmd.Meta{ID: "20260906T235959_ddd", Provider: "claude", Role: "writer",
		Dir: dir, CreatedAt: now.Add(-time.Minute), Status: runcmd.StatusRunning, ExitCode: -1,
		Usage: &providers.Usage{CostUSD: &cost, Turns: 2}})
	envelope.Write(dir, envelope.Record{
		ID: "20260906T235000_eee", Role: "writer", Provider: "claude", Stage: "scope", Step: 3, Steps: 5,
		Status: envelope.StatusRunning, ExitCode: -1, StartedAt: now.Add(-time.Minute),
		RunID: "20260906T235959_ddd", Spec: ".hoom/specs/x.md", Approval: "approved",
	})

	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := JSONBytes(s)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("CA-208: el JSON debe ser legible: %v", err)
	}
	sobres, ok := back["envelopes"].([]any)
	if !ok || len(sobres) != 1 {
		t.Fatalf("CA-208: el JSON incluye envelopes: %s", raw)
	}
	sobre := sobres[0].(map[string]any)
	for _, campo := range []string{"id", "role", "provider", "stage", "step", "steps", "status", "run_id", "spec"} {
		if _, ok := sobre[campo]; !ok {
			t.Fatalf("CA-204/CA-208: al sobre le falta %q: %v", campo, sobre)
		}
	}
	runs := back["runs"].([]any)
	usage, ok := runs[0].(map[string]any)["usage"].(map[string]any)
	if !ok || usage["cost_usd"] != 1.25 {
		t.Fatalf("CA-208: el JSON incluye el usage de cada run: %v", runs[0])
	}

	// y el texto no dice nada distinto
	var buf bytes.Buffer
	Render(&buf, s, false)
	out := buf.String()
	for _, quiero := range []string{"rol writer", "paso 3/5", "scope", "1.25"} {
		if !strings.Contains(out, quiero) {
			t.Fatalf("CA-208: el texto dice lo mismo que el JSON (%q):\n%s", quiero, out)
		}
	}
}
