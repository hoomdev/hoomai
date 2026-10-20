// Tests de regresion de la septima vuelta de la review de la cabina C4 (la
// delta 3) sobre .hoom/specs/historia-doctor-y-cinta.md, en boardcmd: la
// historia de la tarjeta (CA-362). Escritos desde el spec (enmienda 5,
// seccion "Fuente telemetria", y CA-373 del normalizador de Claude) y la
// descripcion de los hallazgos, sin leer la implementacion; repos git reales
// en t.TempDir(). Los helpers compartidos (hi*, bd*) estan en
// historia_test.go, historia_telemetria_test.go y gather_test.go; los de este
// archivo llevan el prefijo c4v7.
//
//   - 20261019T213022_b8d4a1 y 20261019T213459_733ac3: con la enmienda 4 hoom
//     recortaba al escribirlo el detail de un agent_end cuya linea pasaba de
//     1 MiB; con un subagente de nombre largo que falla se perdia la marca
//     " fallo" y la historia publicaba "<agente> termino su parte".
//   - 20261019T213041_de5a5a: los logs de un hoom anterior a la enmienda 4
//     tienen el evento entero, tambien delegaciones de mas de 1 MiB.
//
// El contrato es el de la enmienda 5 (decision de Henry, reemplaza a la 4):
// hoom no recorta las delegaciones, y la historia lee entera cada linea de
// delegacion de hasta 1 MiB y salta una mas larga. Una delegacion de mas de
// 1 MiB, en un log nuevo o viejo, queda fuera de contrato: no tapa a sus
// vecinas y nunca se publica distinta de lo que fue (un fallo, como exito).
//
// La narracion sale de dos fuentes: un run de verdad (runcmd.Manager con un
// claude falso en el PATH, el de CA-374), y el log que escribia un hoom
// anterior a la enmienda 4 (cada evento del normalizador entero, con
// json.Marshal), que es el mismo que exige la enmienda 5.
package boardcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

const (
	c4v7MiB    = 1 << 20
	c4v7Sesion = "s-c4v7"
)

// c4v7Agente es un subagent_type de casi n bytes (sin partir un caracter)
// que empieza con prefijo, con ASCII y caracteres de 2 y 3 bytes.
func c4v7Agente(prefijo string, n int) string {
	s := prefijo + strings.Repeat("falla-ñ-漢-", n/13+1)
	for n > 0 && n < len(s) && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}

// La salida de un claude falso (claude -p --output-format stream-json
// --verbose de Claude Code 2.1.281, como en CA-374).
func c4v7ToolUse(id, agente string, fondo bool) string {
	in, _ := json.Marshal(map[string]any{"description": "mapea el modulo", "prompt": "mapea", "subagent_type": agente, "run_in_background": fondo})
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id +
		`","name":"Agent","input":` + string(in) + `}]},"parent_tool_use_id":null,"session_id":"` + c4v7Sesion + `"}`
}

func c4v7Resultado(id string, fallo bool, texto string) string {
	e := ""
	if fallo {
		e = `,"is_error":true`
	}
	txt, _ := json.Marshal(texto)
	return `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"` + id + `","type":"tool_result","content":` +
		string(txt) + e + `}]},"parent_tool_use_id":null,"session_id":"` + c4v7Sesion + `"}`
}

func c4v7Notificacion(id, status string) string {
	return `{"type":"system","subtype":"task_notification","task_id":"t-` + id + `","tool_use_id":"` + id +
		`","status":"` + status + `","summary":"aviso","session_id":"` + c4v7Sesion + `"}`
}

// c4v7Fallo es una delegacion a un subagente de nombre largo que falla.
type c4v7Fallo struct {
	nombre       string
	agente, tool string
	notificacion bool // falla con una task_notification failed (en segundo plano), no con is_error
	// si la linea de su agent y la de su agent_end, como las arma el
	// normalizador, entran en 1 MiB
	entraAgent, entraFin bool
}

// c4v7Fallos: agentes de 200 KiB (las dos lineas entran), de 600 KiB (el
// agent entra y el agent_end, con el agente dos veces, no) y de 1100 KiB (no
// entra ninguna), por cada forma de fallar.
func c4v7Fallos() []c4v7Fallo {
	var fs []c4v7Fallo
	for _, n := range []int{200 << 10, 600 << 10, 1100 << 10} {
		for _, notif := range []bool{false, true} {
			como, prefijo := "tool_result con is_error", "hoom-error-"
			if notif {
				como, prefijo = "task_notification failed en segundo plano", "hoom-fondo-"
			}
			fs = append(fs, c4v7Fallo{
				nombre:       fmt.Sprintf("agente de %d KiB, %s", n>>10, como),
				agente:       c4v7Agente(prefijo, n),
				tool:         fmt.Sprintf("toolu_FALLA_%d_%v", n>>10, notif),
				notificacion: notif,
				entraAgent:   n < c4v7MiB,
				entraFin:     2*n < c4v7MiB,
			})
		}
	}
	return fs
}

// c4v7Salida: una delegacion normal a Explore que termina, las que fallan y
// una normal a general-purpose que termina.
func c4v7Salida(fs []c4v7Fallo) []byte {
	lineas := []string{
		`{"type":"system","subtype":"init","session_id":"` + c4v7Sesion + `"}`,
		c4v7ToolUse("toolu_ANTES", "Explore", false),
		c4v7Resultado("toolu_ANTES", false, "los precios"),
	}
	for _, f := range fs {
		if f.notificacion {
			lineas = append(lineas, c4v7ToolUse(f.tool, f.agente, true),
				c4v7Resultado(f.tool, false, "Async agent launched successfully"),
				c4v7Notificacion(f.tool, "failed"))
			continue
		}
		lineas = append(lineas, c4v7ToolUse(f.tool, f.agente, false),
			c4v7Resultado(f.tool, true, "el subagente se quedo sin turnos"))
	}
	lineas = append(lineas,
		c4v7ToolUse("toolu_DESPUES", "general-purpose", false),
		c4v7Resultado("toolu_DESPUES", false, "listo"),
		`{"type":"result","subtype":"success","result":"ok","num_turns":1,"session_id":"`+c4v7Sesion+`"}`)
	return []byte(strings.Join(lineas, "\n") + "\n")
}

// c4v7Normalizar es lo que da el normalizador de Claude de un run
// (providers.Correlating, CA-373) para la salida: todos los eventos, y
// exige la forma del fixture (cada fallo con su agent y un agent_end
// "<agente> fallo...", con el agente entero, y lineas del tamano que dice).
func c4v7Normalizar(t *testing.T, salida []byte, fs []c4v7Fallo) []providers.Event {
	t.Helper()
	p, err := providers.Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := p.(providers.Correlating)
	if !ok {
		t.Fatal("fixture: el provider claude implementa providers.Correlating (CA-373)")
	}
	norm := c.NewNormalizer()
	var todos []providers.Event
	porClave := map[string]providers.Event{}
	for _, l := range strings.Split(strings.TrimSuffix(string(salida), "\n"), "\n") {
		for _, e := range norm(l) {
			todos = append(todos, e)
			if e.Kind == "agent" || e.Kind == "agent_end" {
				porClave[e.Kind+" "+e.ToolID] = e
			}
		}
	}
	for _, f := range fs {
		a, okA := porClave["agent "+f.tool]
		fin, okF := porClave["agent_end "+f.tool]
		if !okA || !okF || a.Agent != f.agente || fin.Agent != f.agente || !strings.HasPrefix(fin.Detail, f.agente+" fallo") {
			t.Fatalf("fixture: %s: el normalizador da su agent y un agent_end \"<agente> fallo...\" con el agente entero (CA-373): %v %v %q",
				f.nombre, okA, okF, fmt.Sprintf("%.80s", fin.Detail))
		}
		for _, x := range []struct {
			e     providers.Event
			entra bool
		}{{a, f.entraAgent}, {fin, f.entraFin}} {
			raw, _ := json.Marshal(x.e)
			if (x.entra && len(raw)+64 > c4v7MiB) || (!x.entra && len(raw)-64 <= c4v7MiB) {
				t.Fatalf("fixture: %s: la linea de su %s entra en 1 MiB = %v, lejos del borde: mide %d", f.nombre, x.e.Kind, x.entra, len(raw))
			}
		}
	}
	for _, tool := range []string{"toolu_ANTES", "toolu_DESPUES"} {
		if _, ok := porClave["agent_end "+tool]; !ok {
			t.Fatalf("fixture: el normalizador cierra la delegacion vecina %s", tool)
		}
	}
	return todos
}

// c4v7Tarjeta arma la tarjeta "precios" (como bdRepo) sin telemetria.
func c4v7Tarjeta(t *testing.T) string {
	t.Helper()
	root := bdRepo(t, "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")
	return root
}

// c4v7RunDeVerdad corre en root un run de verdad (runcmd.Manager.Start) del
// provider claude con un claude falso en el PATH que imprime salida, y deja
// su sidecar como el de un run del writer de la tarjeta. Devuelve su id.
func c4v7RunDeVerdad(t *testing.T, root string, salida []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "salida.jsonl")
	if err := os.WriteFile(p, salida, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\ncat '"+p+"'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m := runcmd.NewManager(root)
	info, err := m.Start(runcmd.StartOptions{Provider: "claude", Prompt: "delega"})
	if err != nil {
		t.Fatalf("fixture: el run arranca: %v", err)
	}
	listo := make(chan runcmd.Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	select {
	case fin := <-listo:
		if fin.Status != runcmd.StatusDone {
			t.Fatalf("fixture: el run del claude falso termina bien: %+v", fin)
		}
	case <-time.After(2 * time.Minute):
		t.Fatal("fixture: el run del claude falso termina en 2 minutos")
	}
	var meta runcmd.Meta
	for plazo := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		mt, ok := runcmd.ReadMeta(root, info.ID)
		if ok && mt.Status != runcmd.StatusRunning {
			meta = mt
			break
		}
		if time.Now().After(plazo) {
			t.Fatalf("fixture: el sidecar del run queda cerrado: %+v %v", mt, ok)
		}
	}
	meta.Task, meta.Role = bdSlug, "writer"
	bdRunDisco(t, root, meta)
	return info.ID
}

// c4v7LogViejo escribe en root la narracion de un run del writer de la
// tarjeta como la escribia un hoom anterior a la enmienda 4: cada evento del
// normalizador entero, una linea con json.Marshal. Devuelve su id.
func c4v7LogViejo(t *testing.T, root string, evs []providers.Event) string {
	t.Helper()
	const id = "20261019T213041_viejo0"
	desde, hasta := evs[0].TS, evs[len(evs)-1].TS
	bdRunDisco(t, root, runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
		CreatedAt: desde.Add(-time.Second), EndedAt: hasta.Add(time.Second), Status: runcmd.StatusDone})
	hiEventos(t, root, id, evs...)
	return id
}

// c4v7Who corta un who o un plain enorme para los mensajes: su comienzo, su
// final (donde dice "fallo" o "termino su parte") y su largo.
func c4v7Who(s string) string {
	if len(s) <= 60 {
		return s
	}
	i := len(s) - 24
	for i < len(s) && (s[i]&0xC0) == 0x80 {
		i++
	}
	return fmt.Sprintf("%.36s...%s (%d bytes)", s, s[i:], len(s))
}

func c4v7Listado(es []TimelineEntry) string {
	var b strings.Builder
	for _, e := range es {
		fmt.Fprintf(&b, "  %s %s who=%q plain=%q ended_at=%v\n", e.At.UTC().Format(time.RFC3339Nano), e.Kind, c4v7Who(e.Who), c4v7Who(e.Plain), e.EndedAt)
	}
	return b.String()
}

// c4v7Exigir exige la historia de la tarjeta con el run id:
//   - nunca hay una subagente-fin "<agente> termino su parte" de una
//     delegacion que fallo: solo las vecinas (Explore y general-purpose)
//     terminan su parte;
//   - de cada fallo cuya linea de agent_end entra en 1 MiB, una subagente-fin
//     "<agente> fallo", con el agente entero; de uno cuya linea no entra,
//     ninguna (la historia salta la linea);
//   - de cada fallo cuya linea de agent entra, una entrada subagente "el
//     writer le paso trabajo a <agente>", cerrada (ended_at) con el ts de su
//     subagente-fin si su agent_end entra, y abierta si no;
//   - las vecinas dan su subagente cerrada y su subagente-fin "termino su
//     parte".
func c4v7Exigir(t *testing.T, ca, root, id string, fs []c4v7Fallo) {
	t.Helper()
	tl, err := TimelineFor(root, "main", "high", bdSlug, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("%s: TimelineFor(%s): %v", ca, bdSlug, err)
	}
	hiBienFormada(t, ca, tl)
	artefacto := ".hoom/runs/" + id + ".jsonl"
	sub, fins := hiDeKind(tl, KindSubagente), hiDeKind(tl, KindSubagenteFin)
	vecinas := map[string]bool{"Explore": true, "general-purpose": true}
	for _, e := range fins {
		if a, ok := strings.CutSuffix(e.Plain, " termino su parte"); ok && !vecinas[a] {
			t.Errorf("%s: la historia nunca publica un fallo como exito (hallazgos b8d4a1 y 733ac3): hay una subagente-fin %q de %s, y esa delegacion fallo\n%s",
				ca, c4v7Who(e.Plain), c4v7Who(e.Who), c4v7Listado(fins))
		}
	}
	for _, f := range fs {
		cc := ca + ", " + f.nombre
		who := f.agente + " (claude)"
		var misFins, misSub []TimelineEntry
		for _, e := range fins {
			if e.Who == who {
				misFins = append(misFins, e)
			}
		}
		for _, e := range sub {
			if e.Who == who {
				misSub = append(misSub, e)
			}
		}
		switch {
		case f.entraFin && (len(misFins) != 1 || misFins[0].Plain != f.agente+" fallo"):
			t.Errorf("%s: la linea de su agent_end entra en 1 MiB: la historia da una subagente-fin \"<agente> fallo\", con el agente entero; dio %d:\n%s",
				cc, len(misFins), c4v7Listado(misFins))
		case !f.entraFin && len(misFins) != 0:
			t.Errorf("%s: la linea de su agent_end pasa de 1 MiB: la historia la salta, sin subagente-fin; dio %d:\n%s",
				cc, len(misFins), c4v7Listado(misFins))
		}
		for _, e := range misFins {
			if e.Source != SourceTelemetria || e.Artifact != artefacto || e.CostUSD != nil || e.EndedAt != nil {
				t.Errorf("%s: la subagente-fin sale del run %s, sin costo y con ended_at null: %+v", cc, artefacto, c4v7Listado([]TimelineEntry{e}))
			}
		}
		if !f.entraAgent {
			continue
		}
		if len(misSub) != 1 || misSub[0].Plain != "el writer le paso trabajo a "+f.agente || misSub[0].Artifact != artefacto {
			t.Errorf("%s: la linea de su agent entra en 1 MiB: la historia da una entrada subagente del writer a %s; dio %d:\n%s",
				cc, c4v7Who(f.agente), len(misSub), c4v7Listado(misSub))
			continue
		}
		e := misSub[0]
		switch {
		case len(misFins) == 1 && (e.EndedAt == nil || !e.EndedAt.Equal(misFins[0].At)):
			t.Errorf("%s: la delegacion queda cerrada con el ts de su agent_end (%s); ended_at fue %v", cc, misFins[0].At, e.EndedAt)
		case !f.entraFin && e.EndedAt != nil:
			t.Errorf("%s: su agent_end esta en una linea de mas de 1 MiB, que se salta: la delegacion queda abierta (ended_at null); fue %s", cc, e.EndedAt)
		}
	}
	for _, v := range []string{"Explore", "general-purpose"} {
		var vs, vf []TimelineEntry
		for _, e := range sub {
			if e.Who == v+" (claude)" {
				vs = append(vs, e)
			}
		}
		for _, e := range fins {
			if e.Who == v+" (claude)" {
				vf = append(vf, e)
			}
		}
		if len(vs) != 1 || len(vf) != 1 || vf[0].Plain != v+" termino su parte" || vs[0].EndedAt == nil || !vs[0].EndedAt.Equal(vf[0].At) {
			t.Errorf("%s: la delegacion vecina a %s da su subagente cerrada y su subagente-fin \"termino su parte\":\n%s%s",
				ca, v, c4v7Listado(vs), c4v7Listado(vf))
		}
	}
}

// CA-362 (hallazgos b8d4a1 y 733ac3, y de5a5a; enmienda 5): la historia de
// una tarjeta cuyo writer delego en subagentes de nombre largo (200 KiB,
// 600 KiB y 1100 KiB) que FALLARON, con un tool_result con is_error y, en
// segundo plano, con una task_notification failed, entre dos delegaciones
// normales que terminaron. Nunca aparece una subagente-fin "<agente> termino
// su parte" de un fallo: si la linea del agent_end entra en 1 MiB, la
// subagente-fin es "<agente> fallo"; si no entra (600 KiB y 1100 KiB), no hay
// subagente-fin. Las vecinas no cambian.
//
// Dos fuentes de la narracion: un run de verdad de hoom de hoy con el claude
// falso (con la enmienda 4 el agent_end de 600 KiB se escribia recortado, sin
// " fallo", y la historia lo publicaba como exito), y el log de un hoom
// anterior a la enmienda 4, con los eventos enteros (de5a5a: una delegacion
// de mas de 1 MiB de un log viejo queda fuera de contrato, pero nunca se
// publica distinta de lo que fue).
func TestHallazgo_b8d4a1_733ac3_CA362_LaHistoriaNuncaPublicaUnFalloComoExito(t *testing.T) {
	fs := c4v7Fallos()
	salida := c4v7Salida(fs)
	evs := c4v7Normalizar(t, salida, fs)
	t.Run("run de verdad de hoom", func(t *testing.T) {
		root := c4v7Tarjeta(t)
		id := c4v7RunDeVerdad(t, root, salida)
		c4v7Exigir(t, "CA-362 (hallazgos b8d4a1 y 733ac3, enmienda 5, run de verdad)", root, id, fs)
	})
	t.Run("log de un hoom anterior a la enmienda 4", func(t *testing.T) {
		root := c4v7Tarjeta(t)
		id := c4v7LogViejo(t, root, evs)
		c4v7Exigir(t, "CA-362 (hallazgos b8d4a1, 733ac3 y de5a5a, enmienda 5, log viejo)", root, id, fs)
	})
}
