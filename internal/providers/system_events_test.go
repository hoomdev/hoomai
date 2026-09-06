// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-192, CA-193, CA-194) y para el hallazgo 20260904T183018_5103a7 sobre el
// spec .hoom/specs/providers-v2-interfaz-y-claude.md (CA-112, CA-117): solo
// system/init abre la sesion; todo lo demas que la CLI dice DE SI MISMA es
// ruido con nombre —kind system— y ya no JSON crudo disfrazado de narracion.
package providers

import (
	"strings"
	"testing"
)

// CA-117: system/init es el UNICO start. CA-112 (re-expresado por CA-192): el
// resto de los system ya NO degrada a text; sale como UN evento system, que es
// el kind que existe justamente para esto.
func TestCA117_SoloInitAbreLaSesion(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"hook_started", "hook_response", "compact_boundary", "task_started", "status"} {
		line := `{"type":"system","subtype":"` + sub + `","session_id":"s1"}`
		evs := p.Normalize(line)
		if len(evs) != 1 || evs[0].Kind != "system" {
			t.Fatalf("CA-112/CA-192: system/%s debe ser UN evento system, fue %+v", sub, evs)
		}
		if evs[0].SessionID != "" {
			t.Fatalf("CA-192: un system que no es init no abre sesion: %+v", evs[0])
		}
	}
	evs := p.Normalize(`{"type":"system","subtype":"init","session_id":"s1"}`)
	if len(evs) != 1 || evs[0].Kind != "start" || evs[0].SessionID != "s1" {
		t.Fatalf("CA-117: system/init debe ser start con SessionID, fue %+v", evs)
	}
}

// CA-193: lo que hoom sabe leer se lee; lo que no, conserva la linea integra.
// El exit_code de un hook viene como STRING ("0") en Claude Code 2.1.263:
// verificado a mano, y por eso no se decodifica como entero.
func TestCA193_DetalleLegibleDelRuido(t *testing.T) {
	p, _ := Lookup("claude")
	uno := func(line string) Event {
		t.Helper()
		evs := p.Normalize(line)
		if len(evs) != 1 {
			t.Fatalf("CA-193: se esperaba UN evento de %s: %+v", line, evs)
		}
		return evs[0]
	}

	ev := uno(`{"type":"system","subtype":"hook_started","hook_name":"SessionStart:startup","session_id":"s1"}`)
	if ev.Kind != "system" || !strings.Contains(ev.Detail, "SessionStart:startup") || strings.Contains(ev.Detail, "{") {
		t.Fatalf("CA-193: hook_started nombra el hook y no lleva JSON: %+v", ev)
	}

	ev = uno(`{"type":"system","subtype":"hook_response","hook_name":"SessionStart:startup","outcome":"success","exit_code":"0","session_id":"s1"}`)
	if ev.Kind != "system" || !strings.Contains(ev.Detail, "SessionStart:startup") ||
		!strings.Contains(ev.Detail, "success") || !strings.Contains(ev.Detail, "exit 0") {
		t.Fatalf("CA-193: hook_response nombra hook, outcome y exit leido como string: %+v", ev)
	}
	// el mismo campo como numero tampoco puede romper el parseo
	if ev := uno(`{"type":"system","subtype":"hook_response","hook_name":"h","exit_code":1}`); !strings.Contains(ev.Detail, "exit 1") {
		t.Fatalf("CA-193: exit_code numerico tambien se lee: %+v", ev)
	}

	// subtype desconocido: kind system, pero la linea NO se pierde
	linea := `{"type":"system","subtype":"telemetria_nueva","dato":42}`
	if ev := uno(linea); ev.Kind != "system" || !strings.Contains(ev.Detail, `"dato":42`) {
		t.Fatalf("CA-193: un subtype desconocido conserva la linea integra: %+v", ev)
	}
	// system sin subtype: mismo trato, nada se pierde
	linea = `{"type":"system","dato":7}`
	if ev := uno(linea); ev.Kind != "system" || !strings.Contains(ev.Detail, `"dato":7`) {
		t.Fatalf("CA-193: un system sin subtype conserva la linea integra: %+v", ev)
	}
}

// CA-194: rate_limit_event deja de ser JSON crudo, nombra las ventanas que
// TRAE (sin inventar las que faltan) y no abre sesion pese a su session_id.
func TestCA194_RateLimitEsRuidoConNombre(t *testing.T) {
	p, _ := Lookup("claude")
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","rateLimitType":"five_hour",` +
		`"unifiedWindows":{"seven_day":{"utilization":0.08},"five_hour":{"utilization":0.07}}},"session_id":"s1"}`
	evs := p.Normalize(line)
	if len(evs) != 1 || evs[0].Kind != "system" {
		t.Fatalf("CA-194: rate_limit_event debe ser UN evento system: %+v", evs)
	}
	ev := evs[0]
	if ev.SessionID != "" {
		t.Fatalf("CA-194: rate_limit_event no abre sesion: %+v", ev)
	}
	if strings.Contains(ev.Detail, "{") {
		t.Fatalf("CA-194: el detalle no puede ser JSON crudo: %q", ev.Detail)
	}
	// orden estable (mapa de Go) y ambas ventanas nombradas
	if i, j := strings.Index(ev.Detail, "five_hour"), strings.Index(ev.Detail, "seven_day"); i < 0 || j < 0 || i > j {
		t.Fatalf("CA-194: las ventanas se nombran en orden estable: %q", ev.Detail)
	}
	if !strings.Contains(ev.Detail, "7.0%") || !strings.Contains(ev.Detail, "8.0%") {
		t.Fatalf("CA-194: el detalle dice la utilizacion de cada ventana: %q", ev.Detail)
	}

	// sin ventanas no se inventa ninguna
	ev = p.Normalize(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","rateLimitType":"five_hour"}}`)[0]
	if strings.Contains(ev.Detail, "%") {
		t.Fatalf("CA-194: sin ventanas no se inventan porcentajes: %q", ev.Detail)
	}
	if !strings.Contains(ev.Detail, "five_hour") {
		t.Fatalf("CA-194: sin ventanas queda el tipo que si vino: %q", ev.Detail)
	}

	// un type de primer nivel DESCONOCIDO sigue siendo text con la linea
	// integra: solo baja a system lo que hoom reconoce como plomeria
	otro := `{"type":"telemetria_nueva","dato":1}`
	if ev := p.Normalize(otro)[0]; ev.Kind != "text" || ev.Detail != otro {
		t.Fatalf("CA-194: un type desconocido sigue visible como text: %+v", ev)
	}
}
