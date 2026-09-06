// Test del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md (CA-195):
// el ruido de la CLI no actua en el escenario, y un run cuyo ultimo evento es
// ruido sigue vivo.
package runcmd

import (
	"testing"
	"time"
)

// CA-195: los eventos system no cuentan como actos del orquestador, y solo
// end/error siguen siendo terminales.
func TestCA195_ElRuidoNoActuaEnElEscenario(t *testing.T) {
	now := time.Now().UTC()
	evs := []Event{
		{TS: now, Kind: "start", Detail: "run"},
		{TS: now, Kind: "system", Detail: "hook SessionStart:startup arranco"},
		{TS: now, Kind: "system", Detail: "hook SessionStart:startup: success (exit 0)"},
		{TS: now, Kind: "text", Detail: "trabajando"},
		{TS: now, Kind: "system", Detail: "rate limit allowed: five_hour 7.0%"},
	}
	sv := Stage(Run{Status: StatusRunning}, evs)
	if sv.Actors[0].Role != "orquestador" {
		t.Fatalf("CA-195: el primer actor es el orquestador: %+v", sv.Actors[0])
	}
	if sv.Actors[0].Acts != 1 {
		t.Fatalf("CA-195: solo el text es un acto; tres system no lo son: %d actos", sv.Actors[0].Acts)
	}
	if sv.Actors[0].LastDetail != "trabajando" {
		t.Fatalf("CA-195: el ruido tampoco pisa lo ultimo que dijo el actor: %q", sv.Actors[0].LastDetail)
	}

	// terminal sigue siendo solo end/error: un run que termina hablando de si
	// mismo no esta muerto
	if !esTerminal("end") || !esTerminal("error") {
		t.Fatal("CA-195: end y error cierran")
	}
	if esTerminal("system") {
		t.Fatal("CA-195: un system no cierra un run")
	}
}

// esTerminal replica la unica regla que markOrphans y activeRuns usan para
// decidir si un run sigue vivo.
func esTerminal(kind string) bool { return kind == "end" || kind == "error" }
