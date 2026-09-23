// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md sobre
// la fuente telemetria de la historia: sobres y runs (CA-361) y las
// delegaciones a subagentes emparejadas por tool_id (CA-362). Registros y
// sidecars escritos a mano en la raiz y en el espacio de trabajo, eventos en
// .hoom/runs/<id>.jsonl; sin ningun CLI de IA. Los helpers compartidos (hi*)
// estan en historia_test.go.
package boardcmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Los registros del fixture de telemetria.
const (
	hiE1 = "20260921T090900_sobre1" // entregable, con run y sidecar, lanzado por la cinta
	hiE2 = "20260921T094000_sobre2" // no-entregable, su run no tiene sidecar
	hiE3 = "20260921T095000_sobre3" // sin-entrega, sin run
	hiE4 = "20260921T100000_sobre4" // en curso con dueno vivo
	hiE5 = "20260921T101000_sobre5" // en curso sin dueno
	hiE6 = "20260921T101500_sobre6" // de otra tarea

	hiR1      = "20260921T091000_run001" // el run de E1, con delegaciones
	hiRPerdid = "20260921T094000_perdid" // el run de E2: sin sidecar
	hiR3      = "20260921T102000_run003" // suelto, sin costo
	hiR4      = "20260921T104000_run004" // suelto, sin rol, con error, en el espacio de trabajo
	hiR5      = "20260921T104500_run005" // de otra tarea, con una delegacion
	hiR6      = "20260921T105000_run006" // suelto, viejo: delegaciones sin tool_id
)

var hiTelD0 = time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)

func hiMin(n int) time.Time { return hiTelD0.Add(time.Duration(n) * time.Minute) }

func hiEv(n int, kind, agente, tool, detalle string) providers.Event {
	return providers.Event{TS: hiMin(n), Kind: kind, Agent: agente, ToolID: tool, Detail: detalle}
}

// hiTelemetria arma la telemetria de "precios" (en la raiz y en su espacio
// de trabajo) mas la de otra tarea.
func hiTelemetria(t *testing.T) string {
	t.Helper()
	root := bdRepo(t, "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")
	wt := bdWorktree(t, root, bdSlug)
	muerto := bdPIDMuerto(t)

	// E1 + R1: el sidecar manda sobre el usage del sobre
	bdRunDisco(t, root, runcmd.Meta{ID: hiR1, Provider: "claude", Role: "writer", Task: bdSlug, Dir: wt,
		CreatedAt: hiMin(10), EndedAt: hiMin(30), Status: runcmd.StatusDone,
		Usage: &providers.Usage{CostUSD: bdF(0.8), InputTokens: 1000, OutputTokens: 100}})
	hiEventos(t, root, hiR1,
		hiEv(10, "start", "", "", "init"),
		hiEv(12, "agent", "general-purpose", "toolu_A", "Agent: buscar precios"),
		hiEv(13, "agent", "Explore", "toolu_B", "Agent: explorar"),
		hiEv(14, "agent", "general-purpose", "toolu_C", "Agent: otra busqueda en paralelo"),
		hiEv(15, "tool", "", "", "Read: precios.go"),
		hiEv(16, "agent_end", "general-purpose", "toolu_C", "general-purpose termino"),
		hiEv(17, "agent_end", "Explore", "toolu_B", "Explore fallo: git merge conflicto en HEAD"),
		hiEv(18, "agent_end", "fantasma", "toolu_Z", "fantasma termino"),
		hiEv(20, "agent_end", "general-purpose", "toolu_A", "general-purpose termino"),
		hiEv(21, "agent", "Plan", "toolu_D", "Agent: planear"),
		hiEv(23, "agent_end", "Plan", "toolu_D", "Plan fallo (killed)"),
		hiEv(24, "agent", "Explore", "toolu_E", "Agent: explorar de nuevo"),
		hiEv(29, "end", "", "", "listo"))
	bdSobreDisco(t, root, envelope.Record{ID: hiE1, Role: "writer", Provider: "claude", Task: bdSlug, Dir: wt,
		RunID: hiR1, Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable, Pilot: true,
		Usage:     &providers.Usage{CostUSD: bdF(0.1), InputTokens: 1, OutputTokens: 1},
		StartedAt: hiMin(9), UpdatedAt: hiMin(31), EndedAt: hiMin(31)})
	// E2: su run no tiene sidecar: cuenta su propio usage
	bdSobreDisco(t, root, envelope.Record{ID: hiE2, Role: "test-writer", Provider: "claude", Task: bdSlug, Dir: wt,
		RunID: hiRPerdid, Stage: "verify", Step: 5, Steps: 7, Status: envelope.StatusNotDeliverable,
		Note:      "git diff en .hoom/worktrees/precios: hoom verify fallo",
		Usage:     &providers.Usage{CostUSD: bdF(0.5), InputTokens: 10, OutputTokens: 1},
		StartedAt: hiMin(40), UpdatedAt: hiMin(45), EndedAt: hiMin(45)})
	bdSobreDisco(t, root, envelope.Record{ID: hiE3, Role: "writer", Provider: "codex", Task: bdSlug, Dir: wt,
		Stage: "scope", Step: 4, Steps: 7, Status: envelope.StatusNoDelivery,
		StartedAt: hiMin(50), UpdatedAt: hiMin(55), EndedAt: hiMin(55)})
	bdSobreDisco(t, root, envelope.Record{ID: hiE4, Role: "arquitecto", Provider: "claude", Task: bdSlug, Dir: wt,
		Stage: "run", Step: 3, Steps: 7, Status: envelope.StatusRunning, PID: os.Getpid(),
		StartedAt: hiMin(60), UpdatedAt: hiMin(61)})
	bdSobreDisco(t, root, envelope.Record{ID: hiE5, Role: "reviewer", Provider: "codex", Task: bdSlug, Dir: wt,
		Stage: "run", Step: 2, Steps: 5, Status: envelope.StatusRunning, PID: muerto,
		StartedAt: hiMin(70), UpdatedAt: hiMin(71)})
	bdSobreDisco(t, root, envelope.Record{ID: hiE6, Role: "writer", Provider: "claude", Task: "otra", Dir: root,
		Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable,
		StartedAt: hiMin(75), UpdatedAt: hiMin(76), EndedAt: hiMin(76)})

	// runs sueltos
	bdRunDisco(t, root, runcmd.Meta{ID: hiR3, Provider: "codex", Role: "reviewer", Task: bdSlug, Dir: wt,
		CreatedAt: hiMin(80), EndedAt: hiMin(90), Status: runcmd.StatusDone,
		Usage: &providers.Usage{InputTokens: 50000, OutputTokens: 2900}})
	hiEventos(t, root, hiR3, hiEv(80, "start", "", "", "thread"), hiEv(90, "end", "", "", "turn completado"))
	bdRunDisco(t, wt, runcmd.Meta{ID: hiR4, Provider: "claude", Task: bdSlug, Dir: wt,
		CreatedAt: hiMin(100), EndedAt: hiMin(101), Status: runcmd.StatusError,
		Usage: &providers.Usage{CostUSD: bdF(0.2), InputTokens: 7, OutputTokens: 7}})
	hiEventos(t, wt, hiR4, hiEv(100, "start", "", "", ""), hiEv(101, "error", "", "", "se corto"))
	bdRunDisco(t, root, runcmd.Meta{ID: hiR5, Provider: "claude", Role: "writer", Task: "otra", Dir: root,
		CreatedAt: hiMin(102), EndedAt: hiMin(104), Status: runcmd.StatusDone,
		Usage: &providers.Usage{CostUSD: bdF(9), InputTokens: 9, OutputTokens: 9}})
	hiEventos(t, root, hiR5, hiEv(103, "agent", "general-purpose", "toolu_O", "Agent: de otra tarea"))
	bdRunDisco(t, root, runcmd.Meta{ID: hiR6, Provider: "claude", Role: "writer", Task: bdSlug, Dir: wt,
		CreatedAt: hiMin(110), EndedAt: hiMin(115), Status: runcmd.StatusDone})
	hiEventos(t, root, hiR6,
		hiEv(110, "start", "", "", "init"),
		hiEv(112, "agent", "general-purpose", "", "Task: buscar"),
		hiEv(113, "agent", "Explore", "", "Task: explorar"),
		hiEv(115, "end", "", "", "listo"))
	return root
}

// CA-361: una entrada sobre por registro de la tarjeta (at = started_at,
// ended_at null si no cerro, el plain de su estado, who "<rol> (<provider>)",
// role y provider en sus campos, piloto copiado del registro, artifact la
// ruta del registro) y una run por cada run que ningun sobre referencia (la
// del espacio de trabajo con su prefijo). El costo de cada run se cuenta una
// vez: el del sidecar en su sobre (no el del sobre, y no otra vez en una
// entrada run), el propio del sobre si su run no tiene sidecar, y null sin
// dato; la suma es la de card.spend. telemetry cuenta sobres y runs.
func TestCA361_SobresYRunsDeLaTarjeta(t *testing.T) {
	root := hiTelemetria(t)
	tl := hiTimeline(t, "CA-361", root)
	fin := func(n int) *time.Time { x := hiMin(n); return &x }

	for _, q := range []struct {
		id, who, role, provider, plain string
		at                             time.Time
		ended                          *time.Time
		cost                           *float64
		tokens                         int
		piloto                         bool
	}{
		{hiE1, "writer (claude)", "writer", "claude", "el writer entrego su trabajo", hiMin(9), fin(31), bdF(0.8), 1100, true},
		{hiE2, "test-writer (claude)", "test-writer", "claude", "el test-writer no entrego: la verificacion dio rojo", hiMin(40), fin(45), bdF(0.5), 11, false},
		{hiE3, "writer (codex)", "writer", "codex", "el writer no entrego: no dejo ningun archivo", hiMin(50), fin(55), nil, 0, false},
		{hiE4, "arquitecto (claude)", "arquitecto", "claude", "el arquitecto esta trabajando", hiMin(60), nil, nil, 0, false},
		{hiE5, "reviewer (codex)", "reviewer", "codex", "el trabajo del reviewer quedo interrumpido", hiMin(70), nil, nil, 0, false},
	} {
		e := hiUna(t, "CA-361", tl, KindSobre, q.id, ".hoom/envelopes/"+q.id+".json")
		hiTelEs(t, "CA-361", e, q.who, q.role, q.provider, q.plain, q.at, q.ended, q.cost, q.tokens, q.piloto)
	}
	for _, q := range []struct {
		id, artifact, who, role, plain string
		at                             time.Time
		ended                          *time.Time
		cost                           *float64
		tokens                         int
	}{
		{hiR3, ".hoom/runs/" + hiR3 + ".jsonl", "reviewer (codex)", "reviewer", "trabajo el reviewer", hiMin(80), fin(90), nil, 52900},
		{hiR4, hiWtp + ".hoom/runs/" + hiR4 + ".jsonl", "claude", "", "el trabajo del claude fallo", hiMin(100), fin(101), bdF(0.2), 14},
		{hiR6, ".hoom/runs/" + hiR6 + ".jsonl", "writer (claude)", "writer", "trabajo el writer", hiMin(110), fin(115), nil, 0},
	} {
		e := hiUna(t, "CA-361", tl, KindRun, q.id, q.artifact)
		prov := "claude"
		if q.id == hiR3 {
			prov = "codex"
		}
		hiTelEs(t, "CA-361", e, q.who, q.role, prov, q.plain, q.at, q.ended, q.cost, q.tokens, false)
	}

	// ni el run de un sobre como entrada run, ni lo de otra tarea
	if n := len(hiDeKind(tl, KindSobre)); n != 5 {
		t.Fatalf("CA-361: una entrada sobre por registro de la tarjeta (5), hubo %d:\n%s", n, hiListado(tl))
	}
	if n := len(hiDeKind(tl, KindRun)); n != 3 {
		t.Fatalf("CA-361: una entrada run por run que ningun sobre referencia (3: no el de E1), hubo %d:\n%s", n, hiListado(tl))
	}
	for _, e := range tl.Entries {
		if e.Ref == hiE6 || e.Ref == hiR5 || (e.Kind == KindRun && e.Ref == hiR1) {
			t.Fatalf("CA-361: ni el run de un sobre ni la telemetria de otra tarea son entradas: %+v", e)
		}
	}

	// el costo de cada run una sola vez: la suma de la historia es card.spend
	var costo float64
	tokens := 0
	for _, e := range tl.Entries {
		if e.CostUSD != nil {
			costo += *e.CostUSD
		}
		tokens += e.Tokens
	}
	s := tl.Card.Spend
	if s.CostUSD == nil || !hiCasi(costo, *s.CostUSD) || !hiCasi(costo, 1.5) {
		t.Fatalf("CA-361: la suma de cost_usd de la historia (%v) es card.spend.cost_usd (%v) = 1.5: cada run una sola vez", costo, s.CostUSD)
	}
	if tokens != s.InputTokens+s.OutputTokens || tokens != 54025 {
		t.Fatalf("CA-361: la suma de tokens de la historia (%d) es la entrada mas la salida de card.spend (%d)", tokens, s.InputTokens+s.OutputTokens)
	}
	if tl.Telemetry != (TelemetryCount{Sobres: 5, Runs: 4}) {
		t.Fatalf("CA-361: telemetry cuenta los 5 sobres y los 4 runs (sidecars) de la tarjeta en esta computadora: %+v", tl.Telemetry)
	}
	if hiTieneNota(tl, NoteSinTelemetria) {
		t.Fatalf("CA-361: con telemetria no va la nota %q: %q", NoteSinTelemetria, tl.Notes)
	}

	// en JSON: ended_at y cost_usd null cuando no hay dato
	for _, e := range tl.Entries {
		if e.Ref != hiE4 {
			continue
		}
		m, raw := tbJSONMap(t, e)
		if v, ok := m["ended_at"]; !ok || v != nil {
			t.Fatalf("CA-361: un sobre que no cerro tiene ended_at null en JSON: %s", raw)
		}
		if v, ok := m["cost_usd"]; !ok || v != nil {
			t.Fatalf("CA-361: sin dato de costo, cost_usd es null en JSON: %s", raw)
		}
	}
}

// hiTelEs exige los campos de una entrada de telemetria.
func hiTelEs(t *testing.T, ca string, e TimelineEntry, who, role, provider, plain string, at time.Time,
	ended *time.Time, cost *float64, tokens int, piloto bool) {
	t.Helper()
	if e.Source != SourceTelemetria || e.Who != who || e.Role != role || e.Provider != provider || e.Plain != plain {
		t.Fatalf("%s: %s %s: source telemetria, who %q, role %q, provider %q, plain %q; fue %s, %q, %q, %q, %q",
			ca, e.Kind, e.Ref, who, role, provider, plain, e.Source, e.Who, e.Role, e.Provider, e.Plain)
	}
	if !e.At.Equal(at) {
		t.Fatalf("%s: %s %s: at %s, fue %s", ca, e.Kind, e.Ref, at.Format(time.RFC3339), e.At.Format(time.RFC3339))
	}
	switch {
	case ended == nil && e.EndedAt != nil:
		t.Fatalf("%s: %s %s: ended_at null (no cerro), fue %s", ca, e.Kind, e.Ref, e.EndedAt.Format(time.RFC3339))
	case ended != nil && (e.EndedAt == nil || !e.EndedAt.Equal(*ended)):
		t.Fatalf("%s: %s %s: ended_at %s, fue %v", ca, e.Kind, e.Ref, ended.Format(time.RFC3339), e.EndedAt)
	}
	switch {
	case cost == nil && e.CostUSD != nil:
		t.Fatalf("%s: %s %s: cost_usd null (nadie reporto costo), fue %v", ca, e.Kind, e.Ref, *e.CostUSD)
	case cost != nil && (e.CostUSD == nil || !hiCasi(*e.CostUSD, *cost)):
		t.Fatalf("%s: %s %s: cost_usd %v, fue %v", ca, e.Kind, e.Ref, *cost, e.CostUSD)
	}
	if e.Tokens != tokens {
		t.Fatalf("%s: %s %s: tokens = entrada + salida = %d, fue %d", ca, e.Kind, e.Ref, tokens, e.Tokens)
	}
	if e.Piloto != piloto {
		t.Fatalf("%s: %s %s: piloto copia el del registro (%v), fue %v", ca, e.Kind, e.Ref, piloto, e.Piloto)
	}
}

// CA-361: sin sobres ni runs de la tarjeta en esta computadora (los de otra
// tarea no cuentan) no hay entradas de telemetria, telemetry es {0, 0} y
// notes lo dice.
func TestCA361_SinTelemetriaEnEstaComputadora(t *testing.T) {
	root := bdRepo(t, "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0, "")
	bdSobreDisco(t, root, envelope.Record{ID: hiE6, Role: "writer", Provider: "claude", Task: "otra", Dir: root,
		Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable,
		StartedAt: hiMin(75), UpdatedAt: hiMin(76), EndedAt: hiMin(76)})
	bdRunDisco(t, root, runcmd.Meta{ID: hiR5, Provider: "claude", Role: "writer", Task: "otra", Dir: root,
		CreatedAt: hiMin(102), EndedAt: hiMin(104), Status: runcmd.StatusDone})
	hiEventos(t, root, hiR5, hiEv(103, "agent", "general-purpose", "toolu_O", "Agent: de otra tarea"))

	tl := hiTimeline(t, "CA-361", root)
	for _, e := range tl.Entries {
		if e.Source == SourceTelemetria {
			t.Fatalf("CA-361: sin telemetria de la tarjeta no hay entradas de telemetria: %+v", e)
		}
	}
	if len(tl.Entries) != 1 || tl.Entries[0].Kind != KindItem {
		t.Fatalf("CA-361: la historia de otra computadora trae su evidencia de git (el item):\n%s", hiListado(tl))
	}
	if !hiTieneNota(tl, NoteSinTelemetria) {
		t.Fatalf("CA-361: notes dice %q: %q", NoteSinTelemetria, tl.Notes)
	}
	if tl.Telemetry != (TelemetryCount{}) {
		t.Fatalf("CA-361: telemetry es {0, 0}: %+v", tl.Telemetry)
	}
	raw, _ := tbJSONMap(t, tl)
	if tel, ok := raw["telemetry"].(map[string]any); !ok || tel["sobres"] != float64(0) || tel["runs"] != float64(0) {
		t.Fatalf("CA-361: telemetry en JSON es {\"sobres\": 0, \"runs\": 0}: %v", raw["telemetry"])
	}
}

// CA-362: cada evento agent de los runs de la tarjeta (tambien el de un run
// que un sobre referencia) da una entrada subagente, y cada agent_end
// emparejado por tool_id una subagente-fin. Dos delegaciones al mismo agente
// en paralelo se emparejan por su tool_id, no por el nombre ni por orden de
// llegada. ended_at de la delegacion es el ts de su agent_end, o null si no
// llego (tambien en un run viejo sin tool_id, que no tiene subagente-fin). Un
// agent_end sin su agent no es una entrada. Las delegaciones no tienen costo.
func TestCA362_DelegacionesEmparejadasPorToolID(t *testing.T) {
	root := hiTelemetria(t)
	tl := hiTimeline(t, "CA-362", root)
	r1 := ".hoom/runs/" + hiR1 + ".jsonl"
	r6 := ".hoom/runs/" + hiR6 + ".jsonl"
	fin := func(n int) *time.Time { x := hiMin(n); return &x }

	en := func(kind string, n int, who string) TimelineEntry {
		t.Helper()
		var hits []TimelineEntry
		for _, e := range hiDeKind(tl, kind) {
			if e.At.Equal(hiMin(n)) && e.Who == who {
				hits = append(hits, e)
			}
		}
		if len(hits) != 1 {
			t.Fatalf("CA-362: se esperaba una entrada %s de %q en el minuto %d, hubo %d:\n%s", kind, who, n, len(hits), hiListado(tl))
		}
		return hits[0]
	}

	for _, q := range []struct {
		n        int
		agente   string
		ended    *time.Time
		artifact string
	}{
		{12, "general-purpose", fin(20), r1}, // toolu_A cierra despues que toolu_C
		{13, "Explore", fin(17), r1},
		{14, "general-purpose", fin(16), r1}, // toolu_C, en paralelo con toolu_A
		{21, "Plan", fin(23), r1},
		{24, "Explore", nil, r1},          // sin agent_end
		{112, "general-purpose", nil, r6}, // run viejo, sin tool_id
		{113, "Explore", nil, r6},
	} {
		e := en(KindSubagente, q.n, q.agente+" (claude)")
		plain := "el writer le paso trabajo a " + q.agente
		if e.Plain != plain || e.Artifact != q.artifact || e.Provider != "claude" || e.Source != SourceTelemetria {
			t.Fatalf("CA-362: la delegacion a %s del minuto %d: plain %q, artifact %q, provider claude; fue %q %q %q %s",
				q.agente, q.n, plain, q.artifact, e.Plain, e.Artifact, e.Provider, e.Source)
		}
		switch {
		case q.ended == nil && e.EndedAt != nil:
			t.Fatalf("CA-362: la delegacion a %s del minuto %d no tiene agent_end emparejado: ended_at null, fue %s", q.agente, q.n, e.EndedAt.Format(time.RFC3339))
		case q.ended != nil && (e.EndedAt == nil || !e.EndedAt.Equal(*q.ended)):
			t.Fatalf("CA-362: ended_at de la delegacion a %s del minuto %d es el ts de SU agent_end (%s, por tool_id), fue %v",
				q.agente, q.n, q.ended.Format(time.RFC3339), e.EndedAt)
		}
	}
	for _, q := range []struct {
		n             int
		agente, plain string
	}{
		{16, "general-purpose", "general-purpose termino su parte"},
		{17, "Explore", "Explore fallo"},
		{20, "general-purpose", "general-purpose termino su parte"},
		{23, "Plan", "Plan fallo"},
	} {
		e := en(KindSubagenteFin, q.n, q.agente+" (claude)")
		if e.Plain != q.plain || e.EndedAt != nil || e.Artifact != r1 || e.Provider != "claude" {
			t.Fatalf("CA-362: la salida de %s del minuto %d: plain %q, ended_at null, artifact %q; fue %q %v %q",
				q.agente, q.n, q.plain, r1, e.Plain, e.EndedAt, e.Artifact)
		}
	}
	if n := len(hiDeKind(tl, KindSubagente)); n != 7 {
		t.Fatalf("CA-362: 7 delegaciones de la tarjeta (5 del run del sobre, 2 del run viejo; ninguna de otra tarea), hubo %d:\n%s", n, hiListado(tl))
	}
	if n := len(hiDeKind(tl, KindSubagenteFin)); n != 4 {
		t.Fatalf("CA-362: 4 salidas emparejadas (el agent_end sin su agent no cuenta, el run viejo no tiene), hubo %d:\n%s", n, hiListado(tl))
	}
	for _, e := range tl.Entries {
		if e.Kind != KindSubagente && e.Kind != KindSubagenteFin {
			continue
		}
		if e.CostUSD != nil || e.Tokens != 0 {
			t.Fatalf("CA-362: las delegaciones no tienen costo propio (cost_usd null, tokens 0): %+v", e)
		}
		if strings.Contains(e.Who, "fantasma") || e.At.Equal(hiMin(18)) || e.At.Equal(hiMin(103)) {
			t.Fatalf("CA-362: ni un agent_end sin su agent ni una delegacion de otra tarea son entradas: %+v", e)
		}
	}
}
