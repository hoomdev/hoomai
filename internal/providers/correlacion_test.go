// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-372, CA-373): la correlacion tool_use/tool_result en el parser de
// Claude. Claude Code 2.1.281 llama `Agent` a la delegacion (hasta 2.1.263 era
// `Task`); el Normalize sin memoria la reconoce con los dos nombres y le pone
// el id, y el normalizador con memoria de un run (Correlating) empareja cada
// delegacion con su resultado y emite UN solo agent_end por delegacion.
package providers

import (
	"strings"
	"testing"
)

// Lineas REALES de Claude Code 2.1.281 (`claude -p --output-format
// stream-json --verbose`) de una delegacion, recortadas en su contenido largo
// pero con su forma y sus claves. La session_id y la ruta de output_file son
// ficticias. El orden real: tool_use Agent, task_started, la linea interna
// del subagente (con parent_tool_use_id), task_updated, task_notification y
// recien despues el tool_result con su tool_use_result.
const (
	coRealToolUse = `{"type":"assistant","message":{"model":"claude-opus-5-5","id":"msg_011CfM3RvrbJDiMvBf3HXRvY","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_012yCCfYAudQxXBj46xM7v8v","name":"Agent","input":{"description":"Answer simple arithmetic","prompt":"What is 2+2? Reply with only the numeric answer.","subagent_type":"general-purpose","run_in_background":false},"caller":{"type":"direct"}}],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":2,"cache_creation_input_tokens":15949,"cache_read_input_tokens":10234,"output_tokens":16}},"parent_tool_use_id":null,"session_id":"00000000-0000-4000-8000-000000000281","uuid":"a7b039f6-7612-4e7f-9873-cd8edf953dbb","timestamp":"2026-09-23T20:59:12.148Z","request_id":"req_011CfM3RvNKckJSp9ywNk3Xz","wire_tool_inputs":{"toolu_012yCCfYAudQxXBj46xM7v8v":{"description":"Answer simple arithmetic","prompt":"What is 2+2? Reply with only the numeric answer.","subagent_type":"general-purpose","run_in_background":false}}}`

	coRealTaskStarted = `{"type":"system","subtype":"task_started","task_id":"ab2410a173565f232","tool_use_id":"toolu_012yCCfYAudQxXBj46xM7v8v","description":"Answer simple arithmetic","subagent_type":"general-purpose","is_backgrounded":false,"spawn_depth":1,"task_type":"local_agent","prompt":"What is 2+2? Reply with only the numeric answer.","uuid":"a7305ec9-d296-49fe-be61-ce16507139b4","session_id":"00000000-0000-4000-8000-000000000281"}`

	coRealInterna = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"What is 2+2? Reply with only the numeric answer."}]},"parent_tool_use_id":"toolu_012yCCfYAudQxXBj46xM7v8v","session_id":"00000000-0000-4000-8000-000000000281","uuid":"e91856eb-7a06-40ca-a408-6f8670f88365","timestamp":"2026-09-23T20:59:12.191Z","subagent_type":"general-purpose","task_description":"Answer simple arithmetic"}`

	coRealTaskUpdated = `{"type":"system","subtype":"task_updated","task_id":"ab2410a173565f232","patch":{"status":"completed","end_time":1790197153568},"uuid":"f62fa0b6-ad9b-430c-866f-98788182019e","session_id":"00000000-0000-4000-8000-000000000281"}`

	coRealNotification = `{"type":"system","subtype":"task_notification","task_id":"ab2410a173565f232","tool_use_id":"toolu_012yCCfYAudQxXBj46xM7v8v","status":"completed","output_file":"/tmp/hoom-demo/tasks/ab2410a173565f232.output","summary":"4","usage":{"total_tokens":23039,"tool_uses":0,"duration_ms":1378},"uuid":"c7c44947-d3d4-4cd8-8f28-a5984b228925","session_id":"00000000-0000-4000-8000-000000000281"}`

	coRealToolResult = `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_012yCCfYAudQxXBj46xM7v8v","type":"tool_result","content":[{"type":"text","text":"[Subagent hand-back] The text below is the final report of a subagent this session delegated to. The report follows:\n  4\nagentId: ab2410a173565f232\n<usage>subagent_tokens: 23039\ntool_uses: 0\nduration_ms: 1379</usage>"}]}]},"parent_tool_use_id":null,"session_id":"00000000-0000-4000-8000-000000000281","uuid":"06ec7b12-f580-443c-a822-37a6481e264b","timestamp":"2026-09-23T20:59:13.600Z","tool_use_result":{"status":"completed","prompt":"What is 2+2? Reply with only the numeric answer.","agentId":"ab2410a173565f232","agentType":"general-purpose","content":[{"type":"text","text":"4"}],"resolvedModel":"claude-opus-5-5","totalDurationMs":1379,"totalTokens":23039,"totalToolUseCount":0,"usage":{"input_tokens":2,"cache_creation_input_tokens":23034,"cache_read_input_tokens":0,"output_tokens":3}}}`

	coRealID = "toolu_012yCCfYAudQxXBj46xM7v8v"
)

// coNorm pide al registro el provider claude y, por la interfaz opcional
// Correlating, un normalizador nuevo: con memoria de UN run.
func coNorm(t *testing.T) func(string) []Event {
	t.Helper()
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := p.(Correlating)
	if !ok {
		t.Fatal("CA-373: el provider claude implementa providers.Correlating")
	}
	n := c.NewNormalizer()
	if n == nil {
		t.Fatal("CA-373: NewNormalizer devuelve un normalizador, no nil")
	}
	return n
}

// coToolUse es la linea assistant de un tool_use con id; subagent vacio =
// sin subagent_type (una herramienta comun).
func coToolUse(id, name, subagent string, background bool) string {
	input := `{"description":"encargo de prueba","prompt":"hace tu parte"`
	if subagent != "" {
		input += `,"subagent_type":"` + subagent + `"`
	}
	if background {
		input += `,"run_in_background":true`
	} else {
		input += `,"run_in_background":false`
	}
	input += `}`
	blockID := ""
	if id != "" {
		blockID = `"id":"` + id + `",`
	}
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use",` + blockID +
		`"name":"` + name + `","input":` + input + `}]},"parent_tool_use_id":null,"session_id":"s-co"}`
}

// coToolResult es la linea user con el tool_result de id. content va tal
// cual (un string JSON o una lista de bloques).
func coToolResult(id string, isError bool, content string) string {
	errKey := ""
	if isError {
		errKey = `,"is_error":true`
	}
	return `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"` + id +
		`","type":"tool_result","content":` + content + errKey + `}]},"parent_tool_use_id":null,"session_id":"s-co"}`
}

// coNotification es la linea system task_notification de id con su status.
func coNotification(id, status string) string {
	return `{"type":"system","subtype":"task_notification","task_id":"t-co","tool_use_id":"` + id +
		`","status":"` + status + `","summary":"se corto a mitad de camino","session_id":"s-co"}`
}

func coEnds(evs []Event) []Event {
	var out []Event
	for _, e := range evs {
		if e.Kind == "agent_end" {
			out = append(out, e)
		}
	}
	return out
}

func coKinds(evs []Event) []string {
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

// coMismo compara dos listas de eventos sin mirar ts: kind, agent, detail y
// tool_id.
func coMismo(a, b []Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].Agent != b[i].Agent || a[i].Detail != b[i].Detail || a[i].ToolID != b[i].ToolID {
			return false
		}
	}
	return true
}

// coIncluido dice si cada evento de got tambien lo emite el Normalize sin
// memoria (want): nada nuevo, sin exigir que lo repita.
func coIncluido(got, want []Event) bool {
	for _, g := range got {
		ok := false
		for _, w := range want {
			if coMismo([]Event{g}, []Event{w}) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// CA-372: el Normalize sin memoria reconoce la delegacion con name `Agent`
// (Claude 2.1.281) y con `Task` (hasta 2.1.263), y el evento agent lleva el
// tool_id cuando el tool_use trae id.
func TestCA372_NormalizeReconoceAgentYTask(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Agent", "Task"} {
		evs := p.Normalize(coToolUse("toolu_co_"+name, name, "hoom-scout", false))
		if len(evs) != 1 || evs[0].Kind != "agent" || evs[0].Agent != "hoom-scout" {
			t.Fatalf("CA-372: un tool_use %q con subagent_type es UN evento agent con el subagente: %+v", name, evs)
		}
		if evs[0].ToolID != "toolu_co_"+name {
			t.Fatalf("CA-372: el evento agent de %q lleva el tool_id del tool_use: %+v", name, evs[0])
		}
	}

	// sin id en el tool_use no hay tool_id que inventar
	evs := p.Normalize(coToolUse("", "Task", "hoom-scout", false))
	if len(evs) != 1 || evs[0].Kind != "agent" || evs[0].ToolID != "" {
		t.Fatalf("CA-372: sin id el agent sale sin tool_id: %+v", evs)
	}

	// la linea real de 2.1.281: hoy se veia como una herramienta mas
	evs = p.Normalize(coRealToolUse)
	if len(evs) != 1 || evs[0].Kind != "agent" || evs[0].Agent != "general-purpose" || evs[0].ToolID != coRealID {
		t.Fatalf("CA-372: la delegacion real de Claude 2.1.281 (name Agent) es un agent con su tool_id: %+v", evs)
	}

	// una herramienta que no delega sigue siendo tool, y el tool_id no es de
	// ella: solo el agent lo lleva
	evs = p.Normalize(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_co_read","name":"Read","input":{"file_path":"app.go"}}]}}`)
	if len(evs) != 1 || evs[0].Kind != "tool" || evs[0].Detail != "Read: app.go" {
		t.Fatalf("CA-372: un Read sigue siendo tool con su detalle de siempre: %+v", evs)
	}
}

// CA-372: el Normalize sin memoria nunca emite agent_end, ni con la
// secuencia real completa, ni con un tool_result o una task_notification que
// cierran la delegacion. La task_notification sigue siendo un system.
func TestCA372_NormalizeNuncaEmiteAgentEnd(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	lineas := []string{
		coRealToolUse, coRealTaskStarted, coRealInterna, coRealTaskUpdated, coRealNotification, coRealToolResult,
		coToolUse("toolu_co_a", "Agent", "hoom-scout", false),
		coToolResult("toolu_co_a", false, `"listo"`),
		coToolResult("toolu_co_a", true, `"se rompio"`),
		coNotification("toolu_co_a", "failed"),
		coNotification("toolu_co_a", "completed"),
	}
	for _, l := range lineas {
		for _, e := range p.Normalize(l) {
			if e.Kind == "agent_end" {
				t.Fatalf("CA-372: Normalize no tiene memoria y nunca emite agent_end: %+v de %s", e, l)
			}
		}
	}
	evs := p.Normalize(coRealNotification)
	if len(evs) != 1 || evs[0].Kind != "system" {
		t.Fatalf("CA-372: la task_notification sigue siendo UN system en Normalize: %+v", evs)
	}
}

// CA-373: una delegacion normal cierra con su tool_result si llega primero:
// UN agent_end con agent, tool_id y "<agente> termino". La task_notification
// que llega despues ya no emite otro, pero sigue emitiendo su system.
func TestCA373_DelegacionNormalCierraConElToolResult(t *testing.T) {
	n := coNorm(t)
	evs := n(coToolUse("toolu_co_1", "Agent", "hoom-scout", false))
	if len(evs) != 1 || evs[0].Kind != "agent" || evs[0].ToolID != "toolu_co_1" {
		t.Fatalf("CA-373: la delegacion es un agent con su tool_id: %+v", evs)
	}
	if ends := coEnds(evs); len(ends) != 0 {
		t.Fatalf("CA-373: delegar no cierra nada: %+v", ends)
	}

	evs = n(coToolResult("toolu_co_1", false, `"el scout termino de mapear"`))
	ends := coEnds(evs)
	if len(ends) != 1 {
		t.Fatalf("CA-373: el tool_result de la delegacion emite UN agent_end: %+v", evs)
	}
	e := ends[0]
	if e.Agent != "hoom-scout" || e.ToolID != "toolu_co_1" || e.Detail != "hoom-scout termino" {
		t.Fatalf("CA-373: agent_end lleva agent, tool_id y 'hoom-scout termino': %+v", e)
	}
	if e.TS.IsZero() {
		t.Fatalf("CA-373: el agent_end lleva su ts (la historia lo usa como ended_at): %+v", e)
	}

	evs = n(coNotification("toolu_co_1", "completed"))
	if ends := coEnds(evs); len(ends) != 0 {
		t.Fatalf("CA-373: un solo agent_end por delegacion; la task_notification posterior no emite otro: %+v", evs)
	}
	if len(evs) != 1 || evs[0].Kind != "system" {
		t.Fatalf("CA-373: la task_notification sigue emitiendo su system: %+v", evs)
	}

	// un segundo tool_result del mismo id tampoco
	if ends := coEnds(n(coToolResult("toolu_co_1", false, `"otra vez"`))); len(ends) != 0 {
		t.Fatalf("CA-373: un solo agent_end por delegacion: %+v", ends)
	}
}

// CA-373: si la task_notification completed llega primero (el orden real de
// 2.1.281), ella cierra: su system y DESPUES el agent_end. El tool_result
// posterior no emite nada nuevo.
func TestCA373_DelegacionNormalCierraConLaNotificacion(t *testing.T) {
	n := coNorm(t)
	n(coToolUse("toolu_co_2", "Task", "hoom-writer", false))

	evs := n(coNotification("toolu_co_2", "completed"))
	if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
		t.Fatalf("CA-373: la task_notification emite su system y despues el agent_end: %v %+v", coKinds(evs), evs)
	}
	if evs[1].Agent != "hoom-writer" || evs[1].ToolID != "toolu_co_2" || evs[1].Detail != "hoom-writer termino" {
		t.Fatalf("CA-373: agent_end de una notificacion completed: %+v", evs[1])
	}

	evs = n(coToolResult("toolu_co_2", false, `"listo"`))
	if ends := coEnds(evs); len(ends) != 0 {
		t.Fatalf("CA-373: el tool_result de una delegacion ya cerrada no emite otro agent_end: %+v", evs)
	}
}

// CA-373: un tool_result con is_error: true cierra la delegacion como
// fallida: "<agente> fallo: <texto recortado>".
func TestCA373_ToolResultConErrorFalla(t *testing.T) {
	n := coNorm(t)
	n(coToolUse("toolu_co_3", "Agent", "hoom-scout", false))
	evs := n(coToolResult("toolu_co_3", true, `"no pude leer el modulo de precios"`))
	ends := coEnds(evs)
	if len(ends) != 1 {
		t.Fatalf("CA-373: un tool_result con error tambien cierra: UN agent_end: %+v", evs)
	}
	if ends[0].Detail != "hoom-scout fallo: no pude leer el modulo de precios" {
		t.Fatalf("CA-373: detail de un fallo: '<agente> fallo: <texto>', fue %q", ends[0].Detail)
	}
	if ends[0].Agent != "hoom-scout" || ends[0].ToolID != "toolu_co_3" {
		t.Fatalf("CA-373: el agent_end fallido lleva agent y tool_id: %+v", ends[0])
	}

	// el texto va recortado: un error enorme no entra entero al log
	n = coNorm(t)
	n(coToolUse("toolu_co_4", "Agent", "hoom-scout", false))
	largo := strings.Repeat("x", 2000)
	ends = coEnds(n(coToolResult("toolu_co_4", true, `"`+largo+`"`)))
	if len(ends) != 1 || !strings.HasPrefix(ends[0].Detail, "hoom-scout fallo: x") {
		t.Fatalf("CA-373: un error largo cierra con '<agente> fallo: ...': %+v", ends)
	}
	if strings.Contains(ends[0].Detail, largo) {
		t.Fatalf("CA-373: el texto del fallo va recortado (%d bytes)", len(ends[0].Detail))
	}

	// el contenido en bloques (la forma real de un tool_result de Agent)
	// tambien cierra como fallo
	n = coNorm(t)
	n(coToolUse("toolu_co_5", "Agent", "hoom-scout", false))
	ends = coEnds(n(coToolResult("toolu_co_5", true, `[{"type":"text","text":"el subagente se quedo sin turnos"}]`)))
	if len(ends) != 1 || !strings.HasPrefix(ends[0].Detail, "hoom-scout fallo") {
		t.Fatalf("CA-373: un tool_result con is_error y contenido en bloques cierra como fallo: %+v", ends)
	}
}

// CA-373: una task_notification con un status distinto de completed cierra
// como fallo, sin texto: "<agente> fallo (<status>)". Su system sigue
// saliendo primero.
func TestCA373_NotificacionNoCompletedFalla(t *testing.T) {
	for _, status := range []string{"failed", "killed"} {
		n := coNorm(t)
		n(coToolUse("toolu_co_6", "Agent", "hoom-test-writer", false))
		evs := n(coNotification("toolu_co_6", status))
		if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
			t.Fatalf("CA-373: status %s: system y despues agent_end: %v %+v", status, coKinds(evs), evs)
		}
		if want := "hoom-test-writer fallo (" + status + ")"; evs[1].Detail != want {
			t.Fatalf("CA-373: status %s: detail %q, esperaba %q", status, evs[1].Detail, want)
		}
		if evs[1].Agent != "hoom-test-writer" || evs[1].ToolID != "toolu_co_6" {
			t.Fatalf("CA-373: status %s: el agent_end lleva agent y tool_id: %+v", status, evs[1])
		}
		if ends := coEnds(n(coToolResult("toolu_co_6", false, `"tarde"`))); len(ends) != 0 {
			t.Fatalf("CA-373: status %s: el tool_result posterior no emite otro agent_end: %+v", status, ends)
		}
	}
}

// CA-373: una delegacion en segundo plano (run_in_background: true) no cierra
// con su tool_result, que llega enseguida y solo dice que arranco: cierra con
// su task_notification.
func TestCA373_SegundoPlanoSoloConLaNotificacion(t *testing.T) {
	n := coNorm(t)
	evs := n(coToolUse("toolu_co_7", "Agent", "hoom-scout", true))
	if len(evs) != 1 || evs[0].Kind != "agent" || evs[0].ToolID != "toolu_co_7" {
		t.Fatalf("CA-373: la delegacion en segundo plano tambien es un agent con tool_id: %+v", evs)
	}
	if ends := coEnds(n(coToolResult("toolu_co_7", false, `"Async agent launched successfully"`))); len(ends) != 0 {
		t.Fatalf("CA-373: el tool_result inmediato de una delegacion en segundo plano NO la cierra: %+v", ends)
	}
	// ni siquiera un tool_result con error cierra la de segundo plano
	if ends := coEnds(n(coToolResult("toolu_co_7", true, `"ruido"`))); len(ends) != 0 {
		t.Fatalf("CA-373: en segundo plano solo cierra la task_notification: %+v", ends)
	}
	evs = n(coNotification("toolu_co_7", "completed"))
	if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
		t.Fatalf("CA-373: la task_notification cierra la de segundo plano (system y agent_end): %v %+v", coKinds(evs), evs)
	}
	if evs[1].Agent != "hoom-scout" || evs[1].ToolID != "toolu_co_7" || evs[1].Detail != "hoom-scout termino" {
		t.Fatalf("CA-373: agent_end de la delegacion en segundo plano: %+v", evs[1])
	}
	if ends := coEnds(n(coNotification("toolu_co_7", "completed"))); len(ends) != 0 {
		t.Fatalf("CA-373: una segunda notificacion no emite otro agent_end: %+v", ends)
	}
}

// CA-373: un tool_result de otra herramienta, o de un id que nunca fue una
// delegacion, y una task_notification de un id desconocido no emiten nada
// nuevo: lo mismo que el Normalize sin memoria.
func TestCA373_IdsAjenosNoEmitenNadaNuevo(t *testing.T) {
	p, _ := Lookup("claude")
	n := coNorm(t)
	// un Read y un Bash con id, y una delegacion abierta que no se toca
	n(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_co_read","name":"Read","input":{"file_path":"app.go"}}]}}`)
	n(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_co_bash","name":"Bash","input":{"command":"go test ./..."}}]}}`)
	n(coToolUse("toolu_co_8", "Agent", "hoom-scout", false))

	// tool_result ajenos: ningun evento que el Normalize sin memoria no emita
	for _, l := range []string{
		coToolResult("toolu_co_read", false, `"package app"`),
		coToolResult("toolu_co_bash", true, `"exit 1"`),
		coToolResult("toolu_co_nunca", false, `"de nadie"`),
	} {
		evs := n(l)
		if ends := coEnds(evs); len(ends) != 0 {
			t.Fatalf("CA-373: un id que no es una delegacion no emite agent_end: %+v de %s", ends, l)
		}
		if want := p.Normalize(l); !coIncluido(evs, want) {
			t.Fatalf("CA-373: un tool_result ajeno no emite nada nuevo:\nlinea %s\ncorrelado %+v\nsin memoria %+v", l, evs, want)
		}
	}
	// task_notification ajenas: su system de siempre, y nada mas
	for _, l := range []string{
		coNotification("toolu_co_nunca", "completed"),
		coNotification("toolu_co_read", "failed"),
	} {
		evs := n(l)
		if want := p.Normalize(l); !coMismo(evs, want) || len(evs) != 1 || evs[0].Kind != "system" {
			t.Fatalf("CA-373: una task_notification ajena emite solo su system:\nlinea %s\ncorrelado %+v\nsin memoria %+v", l, evs, want)
		}
	}

	// la delegacion abierta sigue esperando su resultado y cierra una vez
	if ends := coEnds(n(coToolResult("toolu_co_8", false, `"listo"`))); len(ends) != 1 || ends[0].ToolID != "toolu_co_8" {
		t.Fatalf("CA-373: los ids ajenos no tocan la delegacion abierta, que cierra con su tool_result: %+v", ends)
	}
}

// CA-373: dos delegaciones al mismo subagente en el mismo mensaje (en
// paralelo) son dos agent con su id, y cada una cierra con su propio
// agent_end, aunque los dos tool_result lleguen en un solo mensaje.
func TestCA373_DosDelegacionesEnParalelo(t *testing.T) {
	n := coNorm(t)
	evs := n(`{"type":"assistant","message":{"content":[` +
		`{"type":"tool_use","id":"toolu_co_pa","name":"Agent","input":{"description":"mapea precios","subagent_type":"hoom-scout"}},` +
		`{"type":"tool_use","id":"toolu_co_pb","name":"Agent","input":{"description":"mapea stock","subagent_type":"hoom-scout"}}]}}`)
	if len(evs) != 2 || evs[0].Kind != "agent" || evs[1].Kind != "agent" ||
		evs[0].ToolID != "toolu_co_pa" || evs[1].ToolID != "toolu_co_pb" {
		t.Fatalf("CA-373: dos delegaciones en un mensaje son dos agent con su id: %+v", evs)
	}
	evs = n(`{"type":"user","message":{"role":"user","content":[` +
		`{"tool_use_id":"toolu_co_pb","type":"tool_result","content":"stock listo"},` +
		`{"tool_use_id":"toolu_co_pa","type":"tool_result","content":"precios listo"}]}}`)
	ends := coEnds(evs)
	if len(ends) != 2 {
		t.Fatalf("CA-373: cada delegacion cierra con su agent_end: %+v", evs)
	}
	ids := map[string]bool{}
	for _, e := range ends {
		if e.Agent != "hoom-scout" || e.Detail != "hoom-scout termino" {
			t.Fatalf("CA-373: agent_end de una delegacion en paralelo: %+v", e)
		}
		ids[e.ToolID] = true
	}
	if !ids["toolu_co_pa"] || !ids["toolu_co_pb"] {
		t.Fatalf("CA-373: un agent_end por id: %+v", ends)
	}
}

// CA-373: cada normalizador nuevo empieza sin memoria: el id que delego un
// run no existe para el normalizador de otro, y el de cada run sigue
// recordando lo suyo.
func TestCA373_CadaNormalizadorEmpiezaSinMemoria(t *testing.T) {
	uno := coNorm(t)
	uno(coToolUse("toolu_co_9", "Agent", "hoom-scout", false))

	otro := coNorm(t)
	if ends := coEnds(otro(coToolResult("toolu_co_9", false, `"listo"`))); len(ends) != 0 {
		t.Fatalf("CA-373: un normalizador nuevo no conoce las delegaciones de otro: %+v", ends)
	}
	if ends := coEnds(otro(coNotification("toolu_co_9", "completed"))); len(ends) != 0 {
		t.Fatalf("CA-373: tampoco por task_notification: %+v", ends)
	}

	ends := coEnds(uno(coToolResult("toolu_co_9", false, `"listo"`)))
	if len(ends) != 1 || ends[0].ToolID != "toolu_co_9" || ends[0].Agent != "hoom-scout" {
		t.Fatalf("CA-373: el normalizador que vio la delegacion la cierra: %+v", ends)
	}
}

// CA-373: el fixture con las lineas reales de Claude Code 2.1.281 da el agent
// con su tool_id y despues UN unico agent_end con el mismo tool_id. En el
// orden real la task_notification llega antes que el tool_result: cierra
// ella, despues de su system.
func TestCA373_FixtureRealClaude2_1_281(t *testing.T) {
	n := coNorm(t)
	var todos []Event
	porLinea := map[string][]Event{}
	for _, l := range []string{coRealToolUse, coRealTaskStarted, coRealInterna, coRealTaskUpdated, coRealNotification, coRealToolResult} {
		evs := n(l)
		porLinea[l] = evs
		todos = append(todos, evs...)
	}

	agentAt, endAt := -1, -1
	agents, ends := 0, 0
	for i, e := range todos {
		switch e.Kind {
		case "agent":
			agents++
			agentAt = i
		case "agent_end":
			ends++
			endAt = i
		}
	}
	if agents != 1 || ends != 1 {
		t.Fatalf("CA-373: la delegacion real da UN agent y UN agent_end, fueron %d y %d: %v", agents, ends, coKinds(todos))
	}
	if agentAt > endAt {
		t.Fatalf("CA-373: el agent va antes que su agent_end: %v", coKinds(todos))
	}
	a, e := todos[agentAt], todos[endAt]
	if a.Agent != "general-purpose" || a.ToolID != coRealID {
		t.Fatalf("CA-373: el agent real lleva el subagente y su tool_id: %+v", a)
	}
	if e.Agent != "general-purpose" || e.ToolID != coRealID || e.Detail != "general-purpose termino" {
		t.Fatalf("CA-373: el agent_end real lleva el mismo agent y tool_id y 'general-purpose termino': %+v", e)
	}

	notif := porLinea[coRealNotification]
	if len(notif) != 2 || notif[0].Kind != "system" || notif[1].Kind != "agent_end" {
		t.Fatalf("CA-373: la task_notification real emite su system y despues el agent_end: %v", coKinds(notif))
	}
	if coEnds(porLinea[coRealToolResult]) != nil {
		t.Fatalf("CA-373: el tool_result real que llega despues no emite otro agent_end: %+v", porLinea[coRealToolResult])
	}
	for _, l := range []string{coRealTaskStarted, coRealTaskUpdated} {
		if evs := porLinea[l]; len(evs) != 1 || evs[0].Kind != "system" {
			t.Fatalf("CA-373: task_started y task_updated siguen siendo UN system: %+v", evs)
		}
	}
}
