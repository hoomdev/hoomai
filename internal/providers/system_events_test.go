// Tests del writer para el hallazgo 20260904T183018_5103a7 sobre el spec
// .hoom/specs/providers-v2-interfaz-y-claude.md (CA-112, CA-117): solo
// system/init abre la sesion; cualquier otro subtype de system degrada a
// text con la linea integra, sin perderse y sin fabricar un segundo start.
package providers

import "testing"

// CA-117: system/init es el UNICO start; CA-112: el resto de los system
// (hooks, compactacion, tareas, status) es una linea no reconocida y por
// eso sale como UN evento text con la linea integra.
func TestCA117_SoloInitAbreLaSesion(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"hook_started", "hook_response", "compact_boundary", "task_started", "status"} {
		line := `{"type":"system","subtype":"` + sub + `","session_id":"s1"}`
		evs := p.Normalize(line)
		if len(evs) != 1 || evs[0].Kind != "text" || evs[0].Detail != line {
			t.Fatalf("CA-112: system/%s debe degradar a UN text con la linea integra, fue %+v", sub, evs)
		}
	}
	evs := p.Normalize(`{"type":"system","subtype":"init","session_id":"s1"}`)
	if len(evs) != 1 || evs[0].Kind != "start" || evs[0].SessionID != "s1" {
		t.Fatalf("CA-117: system/init debe ser start con SessionID, fue %+v", evs)
	}
}
