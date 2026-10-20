// Helpers compartidos por los tests de las delegaciones de la narracion de un
// run (CA-362 de .hoom/specs/historia-doctor-y-cinta.md, fuente
// "telemetria") de las vueltas de la review de la cabina C4
// (hallazgos_c4_vuelta4..7_test.go). Llevan el prefijo dlg y no dependen de
// ningun archivo de vuelta: antes las vueltas 4, 5 y 6 dependian unas de
// otras (hallazgo 20261019T214037_cca299). Escritos desde el spec y los tests
// existentes, sin leer la implementacion.
//
// El contrato es el de la enmienda 5 (decision de Henry, reemplaza a la 4):
// hoom no recorta las delegaciones, escribe cada agent y agent_end tal como
// lo arma el normalizador; la historia lee entera, en cualquier orden de
// claves, cada linea de delegacion de hasta 1 MiB sin contar el salto, y
// salta una mas larga, como salta una mal formada o a medio escribir. Una
// delegacion "escrita como hoom" es, en estos tests, una linea de hasta 1 MiB.
package runcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/providers"
)

const dlgMiB = 1 << 20

// dlgMismoEvento compara todos los campos de un evento de la narracion.
func dlgMismoEvento(a, b Event) bool {
	return a.TS.Equal(b.TS) && a.Kind == b.Kind && a.Agent == b.Agent && a.ToolID == b.ToolID &&
		a.Detail == b.Detail && a.SessionID == b.SessionID && reflect.DeepEqual(a.Usage, b.Usage)
}

// dlgCabeza es el principio de s de a lo sumo n bytes que no parte un
// caracter (s es UTF-8 valido).
func dlgCabeza(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// dlgMezcla arma un texto de casi n bytes (sin pasarse, sin partir un
// caracter) con ASCII, comillas, barras y caracteres de 2, 3 y 4 bytes. Sin
// <, > ni &: cualquier codificador de JSON lo escribe igual. dlgMezcla(n) es
// un prefijo de dlgMezcla(m) si n <= m.
func dlgMezcla(n int) string {
	piezas := []string{"relleno ", `"comillas" `, "ñandú ", "漢字 ", "🙂 ", `\barra\ `, "é", "x"}
	var b strings.Builder
	for i := 0; b.Len() < n+8; i++ {
		b.WriteString(piezas[i%len(piezas)])
	}
	return dlgCabeza(b.String(), n)
}

// dlgDeLinea devuelve e con su detail alargado con dlgMezcla para que su
// linea, como la escribe json.Marshal (o un codificador que no escapa <, > y
// &), mida lo mas cerca posible de largo bytes sin pasarse (a lo sumo 3 bytes
// menos). Es una delegacion larga escrita como hoom si largo <= 1 MiB.
func dlgDeLinea(e Event, largo int) Event {
	raw, err := json.Marshal(e)
	if err != nil {
		panic(err)
	}
	resto := largo - len(raw)
	if resto <= 0 {
		return e
	}
	m := dlgMezcla(resto)
	n, ocupa := 0, 0
	for n < len(m) {
		_, w := utf8.DecodeRuneInString(m[n:])
		c := w
		if m[n] == '"' || m[n] == '\\' {
			c = 2
		}
		if ocupa+c > resto {
			break
		}
		ocupa += c
		n += w
	}
	e.Detail += m[:n]
	return e
}

// dlgLargoJSON es el largo de la linea del evento con el codificador de
// json.Marshal y con uno que no escapa <, > y &: el menor y el mayor.
func dlgLargoJSON(t *testing.T, e Event) (menor, mayor int) {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		t.Fatal(err)
	}
	sin := b.Len() - 1 // sin el salto de Encode
	return min(len(raw), sin), max(len(raw), sin)
}

// dlgResumen describe un evento sin volcar un detail ni un agente enormes.
func dlgResumen(e Event) string {
	d := e.Detail
	if len(d) > 60 {
		d = fmt.Sprintf("%.60s... (%d bytes)", d, len(e.Detail))
	}
	a := e.Agent
	if len(a) > 40 {
		a = fmt.Sprintf("%.40s... (%d bytes)", a, len(e.Agent))
	}
	return fmt.Sprintf("%s %s agent=%q tool_id=%q detail=%q", e.TS.UTC().Format(time.RFC3339Nano), e.Kind, a, e.ToolID, d)
}

func dlgListado(ds []Event) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString("  " + dlgResumen(d) + "\n")
	}
	return b.String()
}

// dlgCorrerSalida hace un run de verdad (Manager.Start) del provider nombre
// con un binario falso en el PATH que imprime salida, y devuelve root, el id
// del run y lo que el run tiene en memoria (Manager.Events).
func dlgCorrerSalida(t *testing.T, nombre string, salida []byte) (root, id string, mem []Event) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "salida.jsonl")
	if err := os.WriteFile(p, salida, 0o644); err != nil {
		t.Fatal(err)
	}
	installFakeNamed(t, nombre, "cat '"+p+"'\nexit 0\n")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: nombre, Prompt: "delega"})
	if err != nil {
		t.Fatalf("fixture: el run arranca: %v", err)
	}
	// la salida pesa decenas de MiB: con -race el run tarda unos segundos, y
	// el plazo de waitRun (10 s) queda justo; Wait con un plazo holgado
	listo := make(chan Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	select {
	case fin := <-listo:
		if fin.Status != StatusDone {
			t.Fatalf("fixture: el run del provider falso termina bien: %+v", fin)
		}
	case <-time.After(2 * time.Minute):
		t.Fatalf("fixture: el run del provider falso termina en 2 minutos")
	}
	_, mem, err = m.Events(info.ID, 0)
	if err != nil {
		t.Fatalf("fixture: Events del run: %v", err)
	}
	return root, info.ID, mem
}

// dlgLinea es una linea del .jsonl del run tal como esta en el disco, sin el
// salto, con el evento que trae.
type dlgLinea struct {
	raw []byte
	ev  Event
	ok  bool
}

// dlgDisco lee el .jsonl del run linea por linea, sin pasar por runcmd.
func dlgDisco(t *testing.T, root, id string) []dlgLinea {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []dlgLinea
	for _, l := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		var e Event
		err := json.Unmarshal(l, &e)
		out = append(out, dlgLinea{raw: l, ev: e, ok: err == nil})
	}
	return out
}

// La salida de un claude falso (el formato de CA-374: claude -p
// --output-format stream-json --verbose de Claude Code 2.1.281).
const dlgSesion = "s-dlg"

// dlgClaudeToolUse es la linea assistant con el tool_use Agent de una
// delegacion en el subagente agente (en segundo plano si fondo).
func dlgClaudeToolUse(id, agente string, fondo bool) string {
	in, _ := json.Marshal(map[string]any{"description": "mapea el modulo", "prompt": "mapea", "subagent_type": agente, "run_in_background": fondo})
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id +
		`","name":"Agent","input":` + string(in) + `}]},"parent_tool_use_id":null,"session_id":"` + dlgSesion + `"}`
}

// dlgClaudeResultado es la linea user con el tool_result de la delegacion id
// (con is_error si fallo).
func dlgClaudeResultado(id string, fallo bool, texto string) string {
	e := ""
	if fallo {
		e = `,"is_error":true`
	}
	txt, _ := json.Marshal(texto)
	return `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"` + id + `","type":"tool_result","content":` +
		string(txt) + e + `}]},"parent_tool_use_id":null,"session_id":"` + dlgSesion + `"}`
}

// dlgClaudeNotificacion es la linea system task_notification de la
// delegacion id con su estado.
func dlgClaudeNotificacion(id, status string) string {
	return `{"type":"system","subtype":"task_notification","task_id":"t-` + id + `","tool_use_id":"` + id +
		`","status":"` + status + `","summary":"aviso","session_id":"` + dlgSesion + `"}`
}

// dlgClaudeSalida es la salida entera: init, las lineas dadas y el result.
func dlgClaudeSalida(lineas ...string) []byte {
	todas := append([]string{`{"type":"system","subtype":"init","session_id":"` + dlgSesion + `"}`}, lineas...)
	todas = append(todas, `{"type":"result","subtype":"success","result":"ok","num_turns":1,"session_id":"`+dlgSesion+`"}`)
	return []byte(strings.Join(todas, "\n") + "\n")
}

// dlgNormalizarClaude son las delegaciones (agent y agent_end) que da el
// normalizador de Claude de un run (providers.Correlating, CA-373) para la
// salida: la referencia de lo que hoom tiene que escribir.
func dlgNormalizarClaude(t *testing.T, salida []byte) []Event {
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
	var ref []Event
	for _, l := range strings.Split(strings.TrimSuffix(string(salida), "\n"), "\n") {
		for _, e := range norm(l) {
			if e.Kind == "agent" || e.Kind == "agent_end" {
				ref = append(ref, e)
			}
		}
	}
	return ref
}

// dlgCaso es una delegacion que el provider emite (como la arma el
// normalizador) y que hoom tiene que escribir tal cual.
type dlgCaso struct {
	nombre string
	ev     Event
}

// dlgTSDelDisco copia en cada caso el ts con que hoom escribio esa
// delegacion (la de su kind y su tool_id): el normalizador de la referencia
// corrio en otro momento que el del run. Exige que no sea cero.
func dlgTSDelDisco(t *testing.T, ca, root, id string, casos []dlgCaso) {
	t.Helper()
	disco := dlgDisco(t, root, id)
	for i := range casos {
		for _, l := range disco {
			if l.ok && l.ev.Kind == casos[i].ev.Kind && l.ev.ToolID == casos[i].ev.ToolID {
				if l.ev.TS.IsZero() {
					t.Errorf("%s: la delegacion %s se escribe con su ts", ca, dlgResumen(l.ev))
				}
				casos[i].ev.TS = l.ev.TS
			}
		}
	}
}

// dlgExigirTalCual exige, para un run cuya salida trajo las delegaciones de
// casos (mas otros eventos), lo que dice la enmienda 5:
//   - cada linea del .jsonl es un evento JSON valido;
//   - hoom escribe cada delegacion de casos exactamente una vez y TAL CUAL la
//     armo el normalizador: ts, kind, agente, tool_id, session_id y detail
//     iguales, sin recortar nada ni agregar un aviso, mida lo que mida su
//     linea;
//   - lo que el run tiene en memoria es lo que escribio, evento por evento;
//   - ReadDelegations trae enteras, una vez y en orden, las delegaciones
//     escritas cuya linea mide hasta 1 MiB sin el salto, y salta las mas
//     largas sin tapar a las vecinas.
//
// Devuelve las lineas del .jsonl.
func dlgExigirTalCual(t *testing.T, ca, root, id string, mem []Event, casos []dlgCaso) []dlgLinea {
	t.Helper()
	disco := dlgDisco(t, root, id)
	var escritas, entran []Event
	porClave := map[string][]Event{}
	for i, l := range disco {
		if !l.ok {
			t.Errorf("%s: la linea %d del .jsonl es un evento JSON valido: %.120q", ca, i, l.raw)
			continue
		}
		if l.ev.Kind != "agent" && l.ev.Kind != "agent_end" {
			continue
		}
		escritas = append(escritas, l.ev)
		porClave[l.ev.Kind+" "+l.ev.ToolID] = append(porClave[l.ev.Kind+" "+l.ev.ToolID], l.ev)
		if len(l.raw) <= dlgMiB {
			entran = append(entran, l.ev)
		}
	}
	for _, c := range casos {
		got := porClave[c.ev.Kind+" "+c.ev.ToolID]
		cc := ca + ", " + c.nombre
		if len(got) != 1 {
			t.Errorf("%s: la delegacion %s se escribe exactamente una vez; hay %d:\n%s", cc, dlgResumen(c.ev), len(got), dlgListado(got))
			continue
		}
		g := got[0]
		if !g.TS.Equal(c.ev.TS) || g.Kind != c.ev.Kind || g.Agent != c.ev.Agent || g.ToolID != c.ev.ToolID || g.SessionID != c.ev.SessionID {
			t.Errorf("%s: hoom escribe la delegacion tal cual, con su ts, su agente y su tool_id enteros: quiere %s, se escribio %s",
				cc, dlgResumen(c.ev), dlgResumen(g))
		}
		if g.Detail != c.ev.Detail {
			cola := g.Detail
			if len(cola) > 120 {
				cola = cola[len(cola)-120:]
			}
			t.Errorf("%s: hoom no recorta las delegaciones (enmienda 5): escribe el detail tal como lo arma el normalizador (%d bytes, empieza %.60q); se escribio con %d bytes, termina %q",
				cc, len(c.ev.Detail), c.ev.Detail, len(g.Detail), cola)
			if marca := c.ev.Agent + " fallo"; c.ev.Kind == "agent_end" && strings.HasPrefix(c.ev.Detail, marca) && !strings.HasPrefix(g.Detail, marca) {
				t.Errorf("%s: el agent_end de un fallo se escribe con su marca %q (%d bytes de agente); se perdio, y la historia publicaria el fallo como exito (hallazgos b8d4a1 y 733ac3)",
					cc, "<agente> fallo", len(c.ev.Agent))
			}
		}
	}

	// lo que el run tiene en memoria es lo que escribio
	if len(mem) != len(disco) {
		t.Errorf("%s: el run guarda en memoria los mismos eventos que escribe: %d en memoria, %d lineas en el .jsonl",
			ca, len(mem), len(disco))
	} else {
		for i := range mem {
			if !dlgMismoEvento(mem[i], disco[i].ev) {
				t.Errorf("%s: el run guarda en memoria el mismo evento que escribe (evento %d):\n  memoria %s\n  disco   %s",
					ca, i, dlgResumen(mem[i]), dlgResumen(disco[i].ev))
			}
		}
	}

	// la historia lee entera cada linea de hasta 1 MiB y salta las demas
	ds, err := ReadDelegations(root, id)
	if err != nil {
		t.Errorf("%s: ReadDelegations lee el run: %v", ca, err)
		return disco
	}
	ok := len(ds) == len(entran)
	for i := 0; ok && i < len(ds); i++ {
		ok = dlgMismoEvento(ds[i], entran[i])
	}
	if !ok {
		t.Errorf("%s: ReadDelegations trae enteras y en orden las %d delegaciones escritas en lineas de hasta 1 MiB, y salta las %d mas largas sin tapar a sus vecinas; trajo %d:\n%squiere:\n%s",
			ca, len(entran), len(escritas)-len(entran), len(ds), dlgListado(ds), dlgListado(entran))
	}
	return disco
}
