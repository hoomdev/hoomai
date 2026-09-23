// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-374, CA-375): runcmd pide un normalizador por run al provider que
// implementa providers.Correlating, asi el .jsonl de un run de Claude guarda
// el agent_end de cada delegacion; y el escenario usa esos cierres para saber
// cuando un subagente sale de escena (Actor.Open). Un `claude` falso en el
// PATH: ningun CLI real.
package runcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// coDelegacion imita una invocacion de Claude 2.1.281 que delega en
// hoom-scout (tool_use `Agent` con id) y recibe su resultado, en el orden
// real: task_started, task_notification y despues el tool_result.
const coDelegacion = `cat <<'EOF'
{"type":"system","subtype":"init","session_id":"s-co"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_co_run1","name":"Agent","input":{"description":"mapea el modulo de precios","prompt":"mapea","subagent_type":"hoom-scout","run_in_background":false}}]},"parent_tool_use_id":null,"session_id":"s-co"}
{"type":"system","subtype":"task_started","task_id":"t-co","tool_use_id":"toolu_co_run1","subagent_type":"hoom-scout","session_id":"s-co"}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"mapea"}]},"parent_tool_use_id":"toolu_co_run1","session_id":"s-co"}
{"type":"system","subtype":"task_notification","task_id":"t-co","tool_use_id":"toolu_co_run1","status":"completed","summary":"listo","session_id":"s-co"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_co_run1","type":"tool_result","content":[{"type":"text","text":"el mapa del modulo"}]}]},"parent_tool_use_id":null,"session_id":"s-co"}
{"type":"result","subtype":"success","result":"ok","num_turns":1,"total_cost_usd":0.01}
EOF
exit 0
`

// coSoloDelega delega (id toolu_co_run2) y termina sin que llegue el
// resultado de la delegacion.
const coSoloDelega = `cat <<'EOF'
{"type":"system","subtype":"init","session_id":"s-co2"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_co_run2","name":"Agent","input":{"description":"escribi los tests","subagent_type":"hoom-test-writer"}}]},"session_id":"s-co2"}
{"type":"result","subtype":"success","result":"ok","num_turns":1}
EOF
exit 0
`

// coSoloCierra trae SOLO el tool_result y la task_notification del id que
// delego OTRO run: para su propio normalizador ese id no es una delegacion.
const coSoloCierra = `cat <<'EOF'
{"type":"system","subtype":"init","session_id":"s-co3"}
{"type":"system","subtype":"task_notification","task_id":"t-co2","tool_use_id":"toolu_co_run2","status":"completed","session_id":"s-co3"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_co_run2","type":"tool_result","content":"tests escritos"}]},"session_id":"s-co3"}
{"type":"result","subtype":"success","result":"ok","num_turns":1}
EOF
exit 0
`

func coDel(agent, toolID, detail string) Event {
	return Event{TS: time.Now().UTC(), Kind: "agent", Agent: agent, ToolID: toolID, Detail: detail}
}

func coFin(agent, toolID string) Event {
	return Event{TS: time.Now().UTC(), Kind: "agent_end", Agent: agent, ToolID: toolID, Detail: agent + " termino"}
}

func coPorKind(evs []Event, kind string) []Event {
	var out []Event
	for _, e := range evs {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// coCorrer lanza un run del claude falso y devuelve sus eventos leidos del
// .jsonl, como los lee el Studio.
func coCorrer(t *testing.T, m *Manager, root string) (string, []Event) {
	t.Helper()
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "delega en el scout"})
	if err != nil {
		t.Fatal(err)
	}
	if fin := waitRun(t, m, info.ID); fin.Status != StatusDone {
		t.Fatalf("fixture: el run del claude falso termina bien: %+v", fin)
	}
	evs, err := ReadEvents(root, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	return info.ID, evs
}

// CA-374: el .jsonl de un run de Claude con una delegacion trae su agent_end:
// runcmd uso el normalizador de NewNormalizer, no el Normalize sin memoria.
func TestCA374_ElRunDeClaudeGuardaElAgentEnd(t *testing.T) {
	installFake(t, coDelegacion)
	root := t.TempDir()
	m := NewManager(root)
	id, evs := coCorrer(t, m, root)

	agents := coPorKind(evs, "agent")
	if len(agents) != 1 || agents[0].Agent != "hoom-scout" || agents[0].ToolID != "toolu_co_run1" {
		t.Fatalf("CA-374: el run guarda la delegacion (name Agent) con su tool_id: %+v", agents)
	}
	ends := coPorKind(evs, "agent_end")
	if len(ends) != 1 {
		t.Fatalf("CA-374: el .jsonl del run trae UN agent_end de la delegacion: %+v", evs)
	}
	if ends[0].Agent != "hoom-scout" || ends[0].ToolID != "toolu_co_run1" || ends[0].Detail != "hoom-scout termino" {
		t.Fatalf("CA-374: el agent_end guardado lleva agent, tool_id y detail: %+v", ends[0])
	}

	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"kind":"agent_end"`) || !strings.Contains(string(raw), `"tool_id":"toolu_co_run1"`) {
		t.Fatalf("CA-374: el agent_end y su tool_id estan en el .jsonl del run:\n%s", raw)
	}

	// el escenario del mismo log: la delegacion cerro, el scout ya no esta
	// en escena aunque el run se viera en curso
	sv := Stage(Run{Status: StatusRunning}, evs)
	if scout := actorByRole(t, sv, "scout"); scout.Open != 0 || scout.Active || scout.Acts != 1 {
		t.Fatalf("CA-374: con su agent_end en el log, el scout tiene open 0 y sale de escena: %+v", scout)
	}
}

// CA-374: un normalizador POR RUN: el id que delego un run no lo cierra el
// tool_result de otro run del mismo Manager, y una delegacion sin resultado
// queda abierta en su propio log.
func TestCA374_UnNormalizadorPorRun(t *testing.T) {
	root := t.TempDir()
	m := NewManager(root)

	installFake(t, coSoloDelega)
	_, evs := coCorrer(t, m, root)
	if agents := coPorKind(evs, "agent"); len(agents) != 1 || agents[0].ToolID != "toolu_co_run2" {
		t.Fatalf("CA-374: el primer run guarda su delegacion con tool_id: %+v", agents)
	}
	if ends := coPorKind(evs, "agent_end"); len(ends) != 0 {
		t.Fatalf("CA-374: sin resultado la delegacion no cierra: %+v", ends)
	}
	if tw := actorByRole(t, Stage(Run{Status: StatusRunning}, evs), "test-writer"); tw.Open != 1 || !tw.Active {
		t.Fatalf("CA-374: la delegacion sin cierre deja al test-writer con open 1 y en escena: %+v", tw)
	}

	installFake(t, coSoloCierra)
	_, evs = coCorrer(t, m, root)
	if ends := coPorKind(evs, "agent_end"); len(ends) != 0 {
		t.Fatalf("CA-374: el normalizador de otro run no conoce la delegacion del primero: %+v", ends)
	}
}

// CA-375: open cuenta las delegaciones con tool_id sin agent_end, y con el
// run en curso un subagente esta activo si y solo si open > 0: con la
// delegacion del writer cerrada sigue en escena el scout, aunque el writer
// haya sido el ultimo delegado.
func TestCA375_ActivoSiYSoloSiOpen(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		coDel("hoom-scout", "toolu_a", "Agent: mapea"),
		coDel("hoom-writer", "toolu_b", "Agent: implementa"),
		coFin("hoom-writer", "toolu_b"),
	})
	scout, writer := actorByRole(t, sv, "scout"), actorByRole(t, sv, "writer")
	if scout.Open != 1 || !scout.Active {
		t.Fatalf("CA-375: el scout tiene su delegacion abierta: open 1 y activo: %+v", scout)
	}
	if writer.Open != 0 || writer.Active {
		t.Fatalf("CA-375: el writer ya recibio su agent_end: open 0 y fuera de escena: %+v", writer)
	}
	if !actorByRole(t, sv, "orquestador").Active {
		t.Fatal("CA-375: con el run en curso el orquestador siempre esta activo")
	}
	for _, a := range sv.Actors {
		if a.Role != "orquestador" && a.Role != "scout" && a.Active {
			t.Fatalf("CA-375: nadie mas esta en escena: %+v", a)
		}
	}
}

// CA-375: con todas las delegaciones cerradas y el run en curso, solo el
// orquestador esta en escena.
func TestCA375_TodoCerradoSoloElOrquestador(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		coDel("hoom-scout", "toolu_a", "Agent: mapea"),
		ev("tool", "", "Read: app.go"),
		coFin("hoom-scout", "toolu_a"),
		ev("text", "", "sigo yo"),
	})
	if !actorByRole(t, sv, "orquestador").Active {
		t.Fatal("CA-375: el orquestador siempre esta activo con el run en curso")
	}
	for _, a := range sv.Actors[1:] {
		if a.Active || a.Open != 0 {
			t.Fatalf("CA-375: sin delegaciones abiertas ningun subagente esta en escena: %+v", a)
		}
	}

	// mixto: si ALGUN agent del run trae tool_id rige open; un agent sin id
	// no tiene cierre que esperar y no queda en escena
	sv = Stage(Run{Status: StatusRunning}, []Event{
		coDel("hoom-writer", "toolu_w", "Agent: implementa"),
		coFin("hoom-writer", "toolu_w"),
		ev("agent", "hoom-scout", "Task: sin id"),
	})
	if scout := actorByRole(t, sv, "scout"); scout.Active || scout.Open != 0 || scout.Acts != 1 {
		t.Fatalf("CA-375: con tool_id en el run, un agent sin id no suma a open ni queda activo: %+v", scout)
	}
	if actorByRole(t, sv, "writer").Active {
		t.Fatal("CA-375: el writer cerro su delegacion")
	}
}

// CA-375: si ningun agent del run trae tool_id (logs viejos, otro provider)
// rige la regla de siempre: el ultimo delegado esta activo mientras el run
// corre, aunque aparezca un agent_end suelto.
func TestCA375_SinToolIDRigeLaReglaDelUltimoDelegado(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		ev("agent", "hoom-scout", "Task: mapea"),
		ev("agent", "hoom-arquitecto", "Task: escribi el spec"),
		coFin("hoom-arquitecto", "toolu_suelto"),
	})
	if !actorByRole(t, sv, "arquitecto").Active || actorByRole(t, sv, "scout").Active {
		t.Fatalf("CA-375: sin tool_id el ultimo delegado sigue en escena: %+v", sv.Actors)
	}
	if !actorByRole(t, sv, "orquestador").Active {
		t.Fatal("CA-375: el orquestador sigue activo")
	}
	for _, a := range sv.Actors {
		if a.Open != 0 {
			t.Fatalf("CA-375: sin tool_id no hay delegaciones abiertas que contar: %+v", a)
		}
	}
}

// CA-375: agent_end no suma actos ni cambia last_detail (ni del subagente ni
// del orquestador), y uno sin su agent se ignora: no baja open, no crea
// tarjetas, no deja open negativo.
func TestCA375_AgentEndNoActuaYElHuerfanoSeIgnora(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		coDel("hoom-scout", "toolu_a", "Agent: mapea precios"),
		coDel("hoom-scout", "toolu_b", "Agent: mapea stock"),
		coFin("hoom-scout", "toolu_a"),
		coFin("hoom-scout", "toolu_a"),     // repetido: ya estaba cerrada
		coFin("hoom-scout", "toolu_nunca"), // de ninguna delegacion
		coFin("custom-fantasma", "toolu_z"),
	})
	scout := actorByRole(t, sv, "scout")
	if scout.Acts != 2 || scout.LastDetail != "Agent: mapea stock" {
		t.Fatalf("CA-375: agent_end no suma actos ni pisa last_detail: %+v", scout)
	}
	if scout.Open != 1 || !scout.Active {
		t.Fatalf("CA-375: un agent_end repetido o sin su agent no cierra la otra delegacion: open 1 y activo: %+v", scout)
	}
	if orq := actorByRole(t, sv, "orquestador"); orq.Acts != 0 || orq.LastDetail != "" {
		t.Fatalf("CA-375: agent_end tampoco es un acto del orquestador: %+v", orq)
	}
	if len(sv.Actors) != len(FixedCast) {
		t.Fatalf("CA-375: un agent_end sin su agent no crea tarjetas: %+v", sv.Actors)
	}
	for _, a := range sv.Actors {
		if a.Open < 0 {
			t.Fatalf("CA-375: open nunca es negativo: %+v", a)
		}
	}
}

// CA-375: con el run terminado nadie esta activo, aunque quede una
// delegacion abierta (open 1).
func TestCA375_RunTerminadoNadieActivo(t *testing.T) {
	events := []Event{
		coDel("hoom-scout", "toolu_a", "Agent: mapea"),
		coDel("hoom-writer", "toolu_b", "Agent: implementa"),
		coFin("hoom-writer", "toolu_b"),
	}
	for _, status := range []string{StatusDone, StatusError, StatusCanceled} {
		sv := Stage(Run{Status: status}, events)
		for _, a := range sv.Actors {
			if a.Active {
				t.Fatalf("CA-375: con el run %s nadie esta activo: %+v", status, a)
			}
		}
		if scout := actorByRole(t, sv, "scout"); scout.Open != 1 {
			t.Fatalf("CA-375: el run %s termino con la delegacion del scout abierta: open 1: %+v", status, scout)
		}
		if writer := actorByRole(t, sv, "writer"); writer.Open != 0 || writer.Acts != 1 {
			t.Fatalf("CA-375: run %s: el writer cerro y conserva su acto: %+v", status, writer)
		}
	}
}

// CA-375: dos delegaciones en paralelo al mismo subagente dan open 2, y el
// subagente sigue en escena hasta que cierran las dos. Un subagente
// desconocido lleva su open en su tarjeta extra.
func TestCA375_DosDelegacionesParalelasAlMismoSubagente(t *testing.T) {
	base := []Event{
		coDel("hoom-scout", "toolu_a", "Agent: mapea precios"),
		coDel("hoom-scout", "toolu_b", "Agent: mapea stock"),
	}
	sv := Stage(Run{Status: StatusRunning}, base)
	if scout := actorByRole(t, sv, "scout"); scout.Open != 2 || !scout.Active || scout.Acts != 2 {
		t.Fatalf("CA-375: dos delegaciones abiertas al scout: open 2 y activo: %+v", scout)
	}

	sv = Stage(Run{Status: StatusRunning}, append(append([]Event{}, base...), coFin("hoom-scout", "toolu_b")))
	if scout := actorByRole(t, sv, "scout"); scout.Open != 1 || !scout.Active {
		t.Fatalf("CA-375: cerro una de dos: open 1 y sigue en escena: %+v", scout)
	}

	sv = Stage(Run{Status: StatusRunning}, append(append([]Event{}, base...),
		coFin("hoom-scout", "toolu_b"), coFin("hoom-scout", "toolu_a")))
	if scout := actorByRole(t, sv, "scout"); scout.Open != 0 || scout.Active {
		t.Fatalf("CA-375: cerraron las dos: open 0 y fuera de escena: %+v", scout)
	}

	sv = Stage(Run{Status: StatusRunning}, []Event{coDel("custom-refactorer", "toolu_c", "Agent: algo raro")})
	extra := sv.Actors[len(sv.Actors)-1]
	if extra.Role != "custom-refactorer" || extra.Known || extra.Open != 1 || !extra.Active {
		t.Fatalf("CA-375: el desconocido con su delegacion abierta: open 1 y activo: %+v", extra)
	}
}
