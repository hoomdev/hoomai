// Tests de regresion de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, seccion 4 (CA-373): el normalizador
// con memoria de Claude (Correlating) solo cierra una delegacion con una
// task_notification de estado TERMINAL. Los estados del ciclo de vida de una
// tarea de Claude Code antes de terminar (pending, running, started) no son
// terminales: emiten su system de siempre y ningun agent_end, y la delegacion
// sigue abierta. completed (o vacio) cierra con "<agente> termino"; cualquier
// otro status (failed, killed, stopped, desconocido) con "<agente> fallo
// (<status>)". Siempre un solo agent_end por delegacion.
//
// Las lineas son las REALES de Claude Code 2.1.281 de correlacion_test.go
// (coRealToolUse, coRealTaskStarted, coRealNotification, coRealToolResult):
// las task_notification en running, failed, started y pending salen de la
// real cambiando SOLO su campo status, y la delegacion en segundo plano sale
// del tool_use real cambiando SOLO run_in_background a true.
package providers

import (
	"strings"
	"testing"
)

// hcNotifReal es la task_notification real de Claude Code 2.1.281 con otro
// status: la misma linea, cambiando solo el valor de "status".
func hcNotifReal(t *testing.T, status string) string {
	t.Helper()
	const viejo = `"status":"completed"`
	if strings.Count(coRealNotification, viejo) != 1 {
		t.Fatalf("fixture: la task_notification real trae un solo %s", viejo)
	}
	return strings.Replace(coRealNotification, viejo, `"status":"`+status+`"`, 1)
}

// hcToolUseRealSegundoPlano es el tool_use real de Claude Code 2.1.281 con
// run_in_background true (en su input y en wire_tool_inputs, que lo repite).
func hcToolUseRealSegundoPlano(t *testing.T) string {
	t.Helper()
	const viejo = `"run_in_background":false`
	if strings.Count(coRealToolUse, viejo) != 2 {
		t.Fatalf("fixture: el tool_use real trae %s en input y en wire_tool_inputs", viejo)
	}
	return strings.ReplaceAll(coRealToolUse, viejo, `"run_in_background":true`)
}

// hcSoloSystem exige que la task_notification no terminal emita exactamente
// lo que emite el Normalize sin memoria: UN system, y ningun agent_end.
func hcSoloSystem(t *testing.T, que, linea string, evs []Event) {
	t.Helper()
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	if ends := coEnds(evs); len(ends) != 0 {
		t.Fatalf("538306 CA-373: %s: una task_notification no terminal NO cierra la delegacion (ningun agent_end), fue %v %+v",
			que, coKinds(evs), ends)
	}
	if len(evs) != 1 || evs[0].Kind != "system" {
		t.Fatalf("538306 CA-373: %s: la task_notification no terminal sigue emitiendo su system de siempre, y nada mas: %v %+v",
			que, coKinds(evs), evs)
	}
	if want := p.Normalize(linea); !coMismo(evs, want) {
		t.Fatalf("538306 CA-373: %s: el system es el de siempre (el del Normalize sin memoria):\ncorrelado   %+v\nsin memoria %+v",
			que, evs, want)
	}
}

// hcCierre exige que la linea emita su system y DESPUES un unico agent_end de
// la delegacion real, con el detail pedido.
func hcCierre(t *testing.T, que string, evs []Event, detail string) {
	t.Helper()
	if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
		t.Fatalf("538306 CA-373: %s: la task_notification terminal emite su system y despues el agent_end: %v %+v",
			que, coKinds(evs), evs)
	}
	e := evs[1]
	if e.Agent != "general-purpose" || e.ToolID != coRealID || e.Detail != detail {
		t.Fatalf("538306 CA-373: %s: el agent_end lleva agent general-purpose, tool_id %s y detail %q: %+v",
			que, coRealID, detail, e)
	}
}

// hcContarEnds cuenta los agent_end de una secuencia entera.
func hcContarEnds(evs ...[]Event) int {
	n := 0
	for _, l := range evs {
		n += len(coEnds(l))
	}
	return n
}

// Hallazgo 20260924T053338_538306 (HIGH). CA-373: una delegacion normal no
// cierra con una task_notification "running" (lineas reales de Claude Code
// 2.1.281): emite su system y nada mas. Despues la task_notification
// "completed" real la cierra UNA vez con "general-purpose termino", y el
// tool_result real que llega despues no emite otro agent_end.
func TestHallazgo_538306_RunningNoCierraYCompletedCierraUnaVez(t *testing.T) {
	n := coNorm(t)
	evUso := n(coRealToolUse)
	if len(evUso) != 1 || evUso[0].Kind != "agent" || evUso[0].ToolID != coRealID {
		t.Fatalf("538306 CA-373: el tool_use real es un agent con su tool_id: %+v", evUso)
	}
	evStart := n(coRealTaskStarted)
	evInterna := n(coRealInterna)

	running := hcNotifReal(t, "running")
	evRunning := n(running)
	hcSoloSystem(t, "delegacion normal, status running", running, evRunning)

	evFin := n(coRealNotification)
	hcCierre(t, "delegacion normal, running y despues completed", evFin, "general-purpose termino")

	evRes := n(coRealToolResult)
	if ends := coEnds(evRes); len(ends) != 0 {
		t.Fatalf("538306 CA-373: el tool_result real despues de la notificacion completed no emite otro agent_end: %+v", ends)
	}
	if total := hcContarEnds(evUso, evStart, evInterna, evRunning, evFin, evRes); total != 1 {
		t.Fatalf("538306 CA-373: un solo agent_end por delegacion, hubo %d", total)
	}
}

// Hallazgo 20260924T053338_538306 (HIGH). CA-373: running no cierra; la
// task_notification "failed" real que llega despues la cierra UNA vez con
// "general-purpose fallo (failed)".
func TestHallazgo_538306_RunningNoCierraYFailedCierraConFallo(t *testing.T) {
	n := coNorm(t)
	evUso := n(coRealToolUse)
	evStart := n(coRealTaskStarted)

	running := hcNotifReal(t, "running")
	evRunning := n(running)
	hcSoloSystem(t, "delegacion normal, status running antes de failed", running, evRunning)

	evFin := n(hcNotifReal(t, "failed"))
	hcCierre(t, "delegacion normal, running y despues failed", evFin, "general-purpose fallo (failed)")

	evRes := n(coRealToolResult)
	if ends := coEnds(evRes); len(ends) != 0 {
		t.Fatalf("538306 CA-373: el tool_result real despues del fallo no emite otro agent_end: %+v", ends)
	}
	if total := hcContarEnds(evUso, evStart, evRunning, evFin, evRes); total != 1 {
		t.Fatalf("538306 CA-373: un solo agent_end por delegacion, hubo %d", total)
	}
}

// Hallazgo 20260924T053338_538306 (HIGH). CA-373: una delegacion normal cierra
// con el primero de su tool_result o su task_notification TERMINAL. Si
// despues de un "running" llega el tool_result real, cierra ese tool_result
// (termino), y la task_notification "completed" posterior ya no emite otro.
func TestHallazgo_538306_RunningYDespuesToolResultCierraConElResultado(t *testing.T) {
	n := coNorm(t)
	evUso := n(coRealToolUse)
	running := hcNotifReal(t, "running")
	evRunning := n(running)
	hcSoloSystem(t, "delegacion normal, running antes del tool_result", running, evRunning)

	evRes := n(coRealToolResult)
	ends := coEnds(evRes)
	if len(ends) != 1 {
		t.Fatalf("538306 CA-373: la delegacion sigue abierta despues de running y su tool_result la cierra con UN agent_end: %v %+v",
			coKinds(evRes), evRes)
	}
	if e := ends[0]; e.Agent != "general-purpose" || e.ToolID != coRealID || e.Detail != "general-purpose termino" {
		t.Fatalf("538306 CA-373: el tool_result cierra con 'general-purpose termino': %+v", e)
	}

	evFin := n(coRealNotification)
	if len(coEnds(evFin)) != 0 || len(evFin) != 1 || evFin[0].Kind != "system" {
		t.Fatalf("538306 CA-373: la notificacion completed posterior emite solo su system: %v %+v", coKinds(evFin), evFin)
	}
	if total := hcContarEnds(evUso, evRunning, evRes, evFin); total != 1 {
		t.Fatalf("538306 CA-373: un solo agent_end por delegacion, hubo %d", total)
	}
}

// Hallazgo 20260924T053338_538306 (HIGH). CA-373: pending, running y started
// no son terminales. Cada uno emite solo su system (tambien repetido), la
// delegacion sigue abierta, y la task_notification "completed" real la cierra
// una vez.
func TestHallazgo_538306_EstadosNoTerminalesNoCierran(t *testing.T) {
	for _, status := range []string{"pending", "running", "started"} {
		n := coNorm(t)
		evUso := n(coRealToolUse)
		evStart := n(coRealTaskStarted)
		linea := hcNotifReal(t, status)
		var todos [][]Event
		todos = append(todos, evUso, evStart)
		for i := 0; i < 2; i++ { // dos veces: la segunda tampoco cierra
			evs := n(linea)
			hcSoloSystem(t, "status "+status, linea, evs)
			todos = append(todos, evs)
		}
		evFin := n(coRealNotification)
		hcCierre(t, "status "+status+" y despues completed", evFin, "general-purpose termino")
		todos = append(todos, evFin, n(coRealToolResult))
		if total := hcContarEnds(todos...); total != 1 {
			t.Fatalf("538306 CA-373: status %s: un solo agent_end por delegacion, hubo %d", status, total)
		}
	}
}

// Hallazgo 20260924T053338_538306 (HIGH). CA-373: una delegacion en segundo
// plano (run_in_background: true) solo cierra con su task_notification
// terminal: ni la "running", ni su tool_result la cierran; la "completed"
// real si, una vez.
func TestHallazgo_538306_SegundoPlanoRunningNoCierra(t *testing.T) {
	n := coNorm(t)
	evUso := n(hcToolUseRealSegundoPlano(t))
	if len(evUso) != 1 || evUso[0].Kind != "agent" || evUso[0].ToolID != coRealID || evUso[0].Agent != "general-purpose" {
		t.Fatalf("538306 CA-373: la delegacion real en segundo plano es un agent con su tool_id: %+v", evUso)
	}
	evStart := n(coRealTaskStarted)

	running := hcNotifReal(t, "running")
	evRunning := n(running)
	hcSoloSystem(t, "segundo plano, status running", running, evRunning)

	evRes := n(coRealToolResult)
	if ends := coEnds(evRes); len(ends) != 0 {
		t.Fatalf("538306 CA-373: en segundo plano el tool_result no cierra: %+v", ends)
	}

	evFin := n(coRealNotification)
	hcCierre(t, "segundo plano, running, tool_result y despues completed", evFin, "general-purpose termino")
	if total := hcContarEnds(evUso, evStart, evRunning, evRes, evFin); total != 1 {
		t.Fatalf("538306 CA-373: un solo agent_end por delegacion en segundo plano, hubo %d", total)
	}
}

// Hallazgo 20260924T053338_538306 (HIGH). CA-373: en segundo plano, running
// (dos veces, antes y despues del tool_result) no cierra, y la
// task_notification "failed" real cierra una vez con "fallo (failed)".
func TestHallazgo_538306_SegundoPlanoRunningYDespuesFailed(t *testing.T) {
	n := coNorm(t)
	evUso := n(hcToolUseRealSegundoPlano(t))
	running := hcNotifReal(t, "running")
	evR1 := n(running)
	hcSoloSystem(t, "segundo plano, running antes del tool_result", running, evR1)
	evRes := n(coRealToolResult)
	evR2 := n(running)
	hcSoloSystem(t, "segundo plano, running despues del tool_result", running, evR2)

	evFin := n(hcNotifReal(t, "failed"))
	hcCierre(t, "segundo plano, running y despues failed", evFin, "general-purpose fallo (failed)")
	if total := hcContarEnds(evUso, evR1, evRes, evR2, evFin); total != 1 {
		t.Fatalf("538306 CA-373: un solo agent_end por delegacion en segundo plano, hubo %d", total)
	}
}

// Hallazgo 20260924T053338_538306, test del writer. CA-373: paused es el
// otro estado no terminal que el SDK de Claude declara para una tarea
// (task_updated.patch.status: pending, running, paused), y stopped es uno de
// los tres terminales de SDKTaskNotificationMessage.status junto a completed
// y failed: paused no cierra, stopped cierra como fallo, una sola vez.
func TestHallazgo_538306_PausedNoCierraYStoppedCierra(t *testing.T) {
	n := coNorm(t)
	evUso := n(coRealToolUse)
	evStart := n(coRealTaskStarted)
	paused := hcNotifReal(t, "paused")
	evPaused := n(paused)
	hcSoloSystem(t, "status paused", paused, evPaused)
	evStopped := n(hcNotifReal(t, "stopped"))
	hcCierre(t, "status paused y despues stopped", evStopped, "general-purpose fallo (stopped)")
	if total := hcContarEnds(evUso, evStart, evPaused, evStopped, n(coRealToolResult)); total != 1 {
		t.Fatalf("538306 CA-373: un solo agent_end por delegacion, hubo %d", total)
	}
}
