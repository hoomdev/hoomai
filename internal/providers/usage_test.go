// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-197, CA-198): lo que costo una invocacion es un dato del evento de
// cierre, no prosa dentro de un detalle. Las lineas de este archivo son
// RECORTES DE CORRIDAS REALES medidas a mano el 2026-09-06 contra Claude Code
// 2.1.263 y Codex CLI 0.151.0.
package providers

import (
	"strings"
	"testing"
)

// CA-197: el result de Claude llena costo, turnos, tokens y duracion; el
// costo ausente es nil y jamas 0.
func TestCA197_UsageDeClaude(t *testing.T) {
	p, _ := Lookup("claude")

	// linea real: 1 turno, 0.111583 USD, 2 tokens de entrada + 10641 de
	// creacion de cache, 10126 leidos de cache, 4 de salida
	line := `{"type":"result","subtype":"success","is_error":false,"result":"hola","num_turns":1,` +
		`"duration_ms":1562,"duration_api_ms":1456,"total_cost_usd":0.111583,"session_id":"e2e40cc0",` +
		`"usage":{"input_tokens":2,"cache_creation_input_tokens":10641,"cache_read_input_tokens":10126,"output_tokens":4}}`
	evs := p.Normalize(line)
	if len(evs) != 1 || evs[0].Kind != "end" {
		t.Fatalf("CA-197: el result exitoso cierra el run: %+v", evs)
	}
	u := evs[0].Usage
	if u == nil {
		t.Fatal("CA-197: el result trae gasto y tiene que viajar como dato")
	}
	if u.CostUSD == nil || *u.CostUSD != 0.111583 {
		t.Fatalf("CA-197: costo mal leido: %+v", u)
	}
	if u.Turns != 1 || u.DurationMS != 1562 {
		t.Fatalf("CA-197: turnos/duracion mal leidos: %+v", u)
	}
	// entrada = lo nuevo (input + creacion de cache); cache = lo leido
	if u.InputTokens != 10643 || u.CachedTokens != 10126 || u.OutputTokens != 4 {
		t.Fatalf("CA-197: tokens mal leidos: %+v", u)
	}

	// el costo se gasto igual cuando la invocacion FALLA: un run que fallo
	// caro tiene que poder decir cuanto costo fallar
	bad := `{"type":"result","subtype":"error_max_budget_usd","is_error":true,"num_turns":2,"total_cost_usd":0.05,"session_id":"s1"}`
	evs = p.Normalize(bad)
	if len(evs) != 1 || evs[0].Kind != "error" {
		t.Fatalf("CA-197: un result con error es error: %+v", evs)
	}
	if evs[0].Usage == nil || evs[0].Usage.CostUSD == nil || *evs[0].Usage.CostUSD != 0.05 || evs[0].Usage.Turns != 2 {
		t.Fatalf("CA-197: el gasto de una invocacion fallida se cuenta igual: %+v", evs[0].Usage)
	}

	// un result sin ninguna medicion no inventa un usage vacio
	if ev := p.Normalize(`{"type":"result","subtype":"success","result":"ok","session_id":"s1"}`)[0]; ev.Usage != nil {
		t.Fatalf("CA-197: sin medicion no hay usage: %+v", ev.Usage)
	}
}

// CA-198: Codex llena tokens y turnos, NO costo (no lo reporta en ninguna
// linea), y el detalle deja de llevar los numeros en prosa.
func TestCA198_UsageDeCodex(t *testing.T) {
	p, _ := Lookup("codex")

	// linea real de una corrida trivial
	line := `{"type":"turn.completed","usage":{"input_tokens":17408,"cached_input_tokens":11264,` +
		`"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0}}`
	evs := p.Normalize(line)
	if len(evs) != 1 || evs[0].Kind != "end" {
		t.Fatalf("CA-198: turn.completed cierra: %+v", evs)
	}
	ev := evs[0]
	if strings.ContainsAny(ev.Detail, "0123456789") {
		t.Fatalf("CA-198: el detalle ya no lleva los numeros en prosa: %q", ev.Detail)
	}
	u := ev.Usage
	if u == nil {
		t.Fatal("CA-198: turn.completed trae gasto y tiene que viajar como dato")
	}
	if u.CostUSD != nil {
		t.Fatalf("CA-198: Codex no reporta costo; el campo queda ausente, no en 0: %+v", u)
	}
	if u.Turns != 1 || u.InputTokens != 17408 || u.CachedTokens != 11264 || u.OutputTokens != 5 {
		t.Fatalf("CA-198: tokens/turnos mal leidos: %+v", u)
	}

	// un turn.completed sin usage no fabrica ninguno
	if ev := p.Normalize(`{"type":"turn.completed"}`)[0]; ev.Usage != nil {
		t.Fatalf("CA-198: sin usage no se inventa: %+v", ev.Usage)
	}
}

// CA-197: Summary dice en voz alta lo que el provider no midio, en vez de
// imprimir un cero que seria mentira.
func TestCA197_SummaryDiceLoQueNoSabe(t *testing.T) {
	cost := 0.111583
	conCosto := (&Usage{CostUSD: &cost, Turns: 1, InputTokens: 10643, CachedTokens: 10126, OutputTokens: 4}).Summary()
	if !strings.Contains(conCosto, "0.1116") || !strings.Contains(conCosto, "1 turno") {
		t.Fatalf("CA-197: el resumen dice costo y turnos: %q", conCosto)
	}
	if strings.Contains(conCosto, "e-") || strings.Contains(conCosto, "e+") {
		t.Fatalf("CA-197: el costo nunca sale en notacion cientifica: %q", conCosto)
	}
	sinCosto := (&Usage{Turns: 1, InputTokens: 17408, OutputTokens: 5}).Summary()
	if !strings.Contains(sinCosto, "sin dato de costo") {
		t.Fatalf("CA-197: sin costo se dice, no se pone 0: %q", sinCosto)
	}
	var nada *Usage
	if nada.Summary() != "sin datos de uso" || !nada.Empty() {
		t.Fatalf("CA-197: un usage ausente se nombra: %q", nada.Summary())
	}
}
