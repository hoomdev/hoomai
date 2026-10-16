// Tests de regresion de la review de 4 lentes de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos desde el spec
// y la descripcion de cada hallazgo, sin leer la implementacion; un `claude`
// falso en el PATH, ningun CLI real. Los helpers de este archivo llevan el
// prefijo c4.
//
//   - 20261016T195646_f47433 (risk): ReadEvents/ReadEventsFrom arman la ruta
//     con el id del run sin validarlo: un id que sube de directorio lee un
//     .jsonl fuera de .hoom/runs. La narracion de un run es
//     .hoom/runs/<id>.jsonl (CA-362): un id asi es un error y no se lee nada.
//   - 20261016T200503_3bdc20 (resilience): el normalizador con memoria se
//     crea por invocacion y no por run, asi que una delegacion que queda
//     abierta al terminar una invocacion no cierra cuando su resultado llega
//     en la continuacion (Manager.Input). CA-374 pide un normalizador POR RUN:
//     el .jsonl del run trae el agent_end y el escenario (CA-375) ya no la
//     cuenta abierta.
package runcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- f47433

// c4Narracion escribe eventos como lineas JSON en p.
func c4Narracion(t *testing.T, p string, evs ...Event) {
	t.Helper()
	var b strings.Builder
	for _, e := range evs {
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// CA-362 (hallazgo f47433): la narracion de un run es .hoom/runs/<id>.jsonl
// de la raiz. ReadEvents y ReadEventsFrom con un id que sube de directorio
// (y llega al .jsonl de un proyecto vecino, con delegaciones de verdad)
// devuelven un error sin leer nada: ningun evento, y ReadEventsFrom no avanza
// el offset. Un id legitimo se sigue leyendo (el fixture no pasa por fallar
// siempre).
func TestHallazgo_f47433_CA362_ReadEventsNoSaleDeRuns(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	t0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	c4Narracion(t, filepath.Join(root, ".hoom", "runs", "20260921T090000_propio.jsonl"),
		Event{TS: t0, Kind: "start", Detail: "init"},
		Event{TS: t0.Add(time.Minute), Kind: "agent", Agent: "hoom-scout", ToolID: "toolu_ok", Detail: "Agent: mapea"})
	c4Narracion(t, filepath.Join(base, "otro", ".hoom", "runs", "ajeno.jsonl"),
		Event{TS: t0, Kind: "start", Detail: "init"},
		Event{TS: t0.Add(time.Minute), Kind: "agent", Agent: "espia-ajeno", ToolID: "toolu_aj", Detail: "Agent: secreto del otro proyecto"},
		Event{TS: t0.Add(2 * time.Minute), Kind: "agent_end", Agent: "espia-ajeno", ToolID: "toolu_aj", Detail: "espia-ajeno termino"})

	if evs, err := ReadEvents(root, "20260921T090000_propio"); err != nil || len(evs) != 2 {
		t.Fatalf("fixture: un id legitimo se lee: %v %+v", err, evs)
	}

	for _, id := range []string{
		"../../../otro/.hoom/runs/ajeno",
		"x/../../../../otro/.hoom/runs/ajeno",
	} {
		p := filepath.Join(root, ".hoom", "runs", id+".jsonl")
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("fixture: el id %q llega al .jsonl del proyecto vecino (%s): %v", id, p, err)
		}
		evs, err := ReadEvents(root, id)
		if err == nil || len(evs) != 0 {
			t.Fatalf("CA-362 (hallazgo f47433): ReadEvents con el id %q (sale de .hoom/runs) es un error y no lee nada; fue err=%v y %d eventos: %+v",
				id, err, len(evs), evs)
		}
		for _, desde := range []int64{0, 10} {
			evs, next, err := ReadEventsFrom(root, id, desde)
			if err == nil || len(evs) != 0 || next != desde {
				t.Fatalf("CA-362 (hallazgo f47433): ReadEventsFrom(%q, %d) es un error, no lee nada y no avanza el offset; fue err=%v, %d eventos, offset %d",
					id, desde, err, len(evs), next)
			}
		}
	}
}

// ---------------------------------------------------------------- 3bdc20

// c4ClaudeEnDosTurnos instala un `claude` falso que en la primera invocacion
// imprime primero y en las siguientes (la continuacion de Input) imprime
// despues. Un archivo de estado propio distingue las invocaciones.
func c4ClaudeEnDosTurnos(t *testing.T, primero, despues string) {
	t.Helper()
	estado := filepath.Join(t.TempDir(), "ya-corrio")
	installFake(t, "if [ -f '"+estado+"' ]; then\ncat <<'EOF'\n"+despues+"EOF\nelse\n: > '"+estado+"'\ncat <<'EOF'\n"+primero+"EOF\nfi\nexit 0\n")
}

// CA-374, CA-375 (hallazgo 3bdc20): runcmd pide un normalizador POR RUN, no
// por invocacion. Una delegacion de Claude que queda abierta al terminar la
// primera invocacion cierra cuando su resultado llega en la continuacion del
// mismo run (Manager.Input): el .jsonl del run trae UN agent_end de esa
// delegacion (con su agent, su tool_id y su detail), y el escenario ya no la
// cuenta abierta (open 0, fuera de escena aunque el run siga en curso).
//
//   - tool_result: una delegacion normal; la continuacion trae su
//     tool_result y despues su task_notification (cierra con el primero: un
//     solo agent_end).
//   - segundo-plano: una delegacion con run_in_background; su tool_result
//     inmediato (en la primera invocacion) no la cierra, y la cierra la
//     task_notification que llega en la continuacion.
func TestHallazgo_3bdc20_CA374_LaContinuacionCierraLaDelegacion(t *testing.T) {
	for _, caso := range []struct {
		nombre, agente, rol, toolID, primero, despues string
	}{
		{
			nombre: "tool_result", agente: "hoom-test-writer", rol: "test-writer", toolID: "toolu_c4_cont",
			primero: `{"type":"system","subtype":"init","session_id":"s-c4a"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_c4_cont","name":"Agent","input":{"description":"escribi los tests","prompt":"escribi","subagent_type":"hoom-test-writer","run_in_background":false}}]},"parent_tool_use_id":null,"session_id":"s-c4a"}
{"type":"system","subtype":"task_started","task_id":"t-c4a","tool_use_id":"toolu_c4_cont","subagent_type":"hoom-test-writer","session_id":"s-c4a"}
{"type":"result","subtype":"success","result":"sigo despues","num_turns":1}
`,
			despues: `{"type":"system","subtype":"init","session_id":"s-c4a"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_c4_cont","type":"tool_result","content":[{"type":"text","text":"tests escritos"}]}]},"parent_tool_use_id":null,"session_id":"s-c4a"}
{"type":"system","subtype":"task_notification","task_id":"t-c4a","tool_use_id":"toolu_c4_cont","status":"completed","summary":"listo","session_id":"s-c4a"}
{"type":"result","subtype":"success","result":"ok","num_turns":1}
`,
		},
		{
			nombre: "segundo-plano", agente: "hoom-scout", rol: "scout", toolID: "toolu_c4_fondo",
			primero: `{"type":"system","subtype":"init","session_id":"s-c4b"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_c4_fondo","name":"Agent","input":{"description":"mapea en segundo plano","prompt":"mapea","subagent_type":"hoom-scout","run_in_background":true}}]},"parent_tool_use_id":null,"session_id":"s-c4b"}
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_c4_fondo","type":"tool_result","content":[{"type":"text","text":"Async agent launched successfully"}]}]},"parent_tool_use_id":null,"session_id":"s-c4b"}
{"type":"system","subtype":"task_started","task_id":"t-c4b","tool_use_id":"toolu_c4_fondo","subagent_type":"hoom-scout","session_id":"s-c4b"}
{"type":"result","subtype":"success","result":"sigo despues","num_turns":1}
`,
			despues: `{"type":"system","subtype":"init","session_id":"s-c4b"}
{"type":"system","subtype":"task_notification","task_id":"t-c4b","tool_use_id":"toolu_c4_fondo","status":"completed","summary":"mapa listo","session_id":"s-c4b"}
{"type":"result","subtype":"success","result":"ok","num_turns":1}
`,
		},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			c4ClaudeEnDosTurnos(t, caso.primero, caso.despues)
			root := t.TempDir()
			m := NewManager(root)
			info, err := m.Start(StartOptions{Provider: "claude", Prompt: "delega"})
			if err != nil {
				t.Fatal(err)
			}
			if fin := waitRun(t, m, info.ID); fin.Status != StatusDone {
				t.Fatalf("fixture: la primera invocacion termina bien: %+v", fin)
			}
			evs, err := ReadEvents(root, info.ID)
			if err != nil {
				t.Fatal(err)
			}
			if a := coPorKind(evs, "agent"); len(a) != 1 || a[0].Agent != caso.agente || a[0].ToolID != caso.toolID {
				t.Fatalf("fixture: la primera invocacion guarda la delegacion a %s con tool_id %s: %+v", caso.agente, caso.toolID, a)
			}
			if e := coPorKind(evs, "agent_end"); len(e) != 0 {
				t.Fatalf("fixture: la delegacion queda abierta al terminar la primera invocacion: %+v", e)
			}
			if a := actorByRole(t, Stage(Run{Status: StatusRunning}, evs), caso.rol); a.Open != 1 {
				t.Fatalf("fixture: despues de la primera invocacion el %s tiene open 1: %+v", caso.rol, a)
			}

			if _, err := m.Input(info.ID, "segui"); err != nil {
				t.Fatalf("Input: %v", err)
			}
			if fin := waitRun(t, m, info.ID); fin.Status != StatusDone {
				t.Fatalf("fixture: la continuacion termina bien: %+v", fin)
			}
			evs, err = ReadEvents(root, info.ID)
			if err != nil {
				t.Fatal(err)
			}
			ends := coPorKind(evs, "agent_end")
			if len(ends) != 1 {
				t.Fatalf("CA-374 (hallazgo 3bdc20): el normalizador es del run, no de la invocacion: el .jsonl trae UN agent_end de la delegacion %s que cerro en la continuacion; hubo %d: %+v",
					caso.toolID, len(ends), evs)
			}
			if e := ends[0]; e.Agent != caso.agente || e.ToolID != caso.toolID || e.Detail != caso.agente+" termino" {
				t.Fatalf("CA-374 (hallazgo 3bdc20): el agent_end lleva agent %s, tool_id %s y detail %q: %+v", caso.agente, caso.toolID, caso.agente+" termino", e)
			}
			if a := actorByRole(t, Stage(Run{Status: StatusRunning}, evs), caso.rol); a.Open != 0 || a.Active || a.Acts != 1 {
				t.Fatalf("CA-375 (hallazgo 3bdc20): con el agent_end en el log, el %s tiene open 0 y sale de escena (con su acto): %+v", caso.rol, a)
			}
		})
	}
}
