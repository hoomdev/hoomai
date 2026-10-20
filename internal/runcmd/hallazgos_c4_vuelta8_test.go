// Tests de regresion de la octava vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd: el hallazgo
// 20261020T151253_3b4ee7 (lente resilience). Escritos desde el spec (CA-362,
// CA-373, CA-374 y la enmienda 5, "#### Fuente telemetria"), la descripcion
// del hallazgo y los tests existentes, sin leer la implementacion. Los
// helpers de este archivo llevan el prefijo c4v8; los compartidos (prefijo
// dlg) estan en delegaciones_helpers_test.go.
//
// El hallazgo: runcmd acepta lineas del provider de hasta 4 MiB y, desde que
// hoom no recorta las delegaciones (enmienda 5), guarda en memoria y escribe
// entero en el .jsonl el agent_end que arma el normalizador. Una delegacion
// en segundo plano que cierra con una task_notification de status de varios
// MiB deja un agent_end de varios MiB en memoria y en el disco.
//
// El contrato que exigen estos tests: el normalizador arma el detail de un
// agent_end con el agente y el texto del fallo recortado a 300 bytes
// (enmienda 5), y el status de una task_notification no completed es el
// texto de ese fallo: el detail es "<agente> fallo (<status recortado>)",
// con el status recortado igual que el texto de un tool_result con is_error.
// Entonces la linea de ese agent_end mide unos cientos de bytes, lo que el
// run tiene en memoria es lo que escribio, y la historia (ReadDelegations) lo
// lee como un fallo.
package runcmd

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/providers"
)

// c4v8LineaMax es lo que puede medir, como mucho, la linea del agent_end de
// un fallo por status con un agente y un tool_id cortos: unos pocos KiB (el
// detail lleva a lo sumo 300 bytes del status, escapados).
const c4v8LineaMax = 4 << 10

// c4v8OtraLineaMax acota cualquier linea del .jsonl del run: ningun evento
// carga el status de 2 MiB.
const c4v8OtraLineaMax = 64 << 10

// c4v8RecorteDeToolResult es como el normalizador de Claude recorta el texto
// de un fallo en un tool_result con is_error ("<agente> fallo: <texto
// recortado>", CA-373), sin el "<agente> fallo: ": la regla con la que se
// recorta el status.
func c4v8RecorteDeToolResult(t *testing.T, texto string) string {
	t.Helper()
	p, err := providers.Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := p.(providers.Correlating)
	if !ok {
		t.Fatal("fixture: el provider claude implementa providers.Correlating (CA-373)")
	}
	const agente = "hoom-referencia"
	norm := c.NewNormalizer()
	norm(dlgClaudeToolUse("toolu_C4V8_REF", agente, false))
	var ends []Event
	for _, e := range norm(dlgClaudeResultado("toolu_C4V8_REF", true, texto)) {
		if e.Kind == "agent_end" {
			ends = append(ends, e)
		}
	}
	if len(ends) != 1 || !strings.HasPrefix(ends[0].Detail, agente+" fallo: ") {
		t.Fatalf("fixture: un tool_result con is_error cierra con UN agent_end \"<agente> fallo: <texto recortado>\" (CA-373):\n%s", dlgListado(ends))
	}
	return strings.TrimPrefix(ends[0].Detail, agente+" fallo: ")
}

// c4v8Notificacion es la task_notification de la delegacion id con el
// status dado, escrito como JSON valido.
func c4v8Notificacion(id, status string) string {
	st, _ := json.Marshal(status)
	return `{"type":"system","subtype":"task_notification","task_id":"t-` + id + `","tool_use_id":"` + id +
		`","status":` + string(st) + `,"summary":"aviso","session_id":"` + dlgSesion + `"}`
}

// c4v8Fondo es una delegacion en segundo plano que cierra con una
// task_notification no completed.
type c4v8Fondo struct {
	nombre, agente, tool, status string
	// el detail que tiene que tener su agent_end
	detail string
}

// c4v8Fondos son las delegaciones en segundo plano del run: dos con un
// status de 2 MiB (en ASCII y con comillas, barras y unicode) y una con un
// status corto que sale entero.
func c4v8Fondos(t *testing.T) []c4v8Fondo {
	t.Helper()
	fs := []c4v8Fondo{
		{nombre: "status de 2 MiB en ASCII", agente: "hoom-fondo", tool: "toolu_FONDO_ASCII", status: strings.Repeat("x", 2<<20)},
		{nombre: "status de 2 MiB con comillas, barras y unicode", agente: "hoom-mezcla", tool: "toolu_FONDO_MEZCLA", status: dlgMezcla(2 << 20)},
		{nombre: "status corto", agente: "hoom-corto", tool: "toolu_FONDO_CORTO", status: "failed"},
	}
	for i := range fs {
		ref := c4v8RecorteDeToolResult(t, fs[i].status)
		if len(fs[i].status) > 300 {
			if ref == fs[i].status || len(ref) > 300+8 || !utf8.ValidString(ref) {
				t.Fatalf("fixture: %s: el texto de un tool_result fallido de %d bytes sale recortado a 300 bytes y en UTF-8 valido: %d bytes, valido %v",
					fs[i].nombre, len(fs[i].status), len(ref), utf8.ValidString(ref))
			}
		} else if ref != fs[i].status {
			t.Fatalf("fixture: %s: el texto corto de un tool_result fallido sale entero: %q", fs[i].nombre, ref)
		}
		fs[i].detail = fs[i].agente + " fallo (" + ref + ")"
	}
	return fs
}

// c4v8Salida es la salida del claude falso (CA-374): una delegacion normal a
// Explore que termina, las delegaciones en segundo plano (su tool_result
// inmediato y despues su task_notification) y una normal a general-purpose
// que termina.
func c4v8Salida(fs []c4v8Fondo) []byte {
	lineas := []string{
		dlgClaudeToolUse("toolu_ANTES", "Explore", false),
		dlgClaudeResultado("toolu_ANTES", false, "los precios"),
	}
	for _, f := range fs {
		lineas = append(lineas,
			dlgClaudeToolUse(f.tool, f.agente, true),
			dlgClaudeResultado(f.tool, false, "Async agent launched successfully"),
			c4v8Notificacion(f.tool, f.status))
	}
	lineas = append(lineas,
		dlgClaudeToolUse("toolu_DESPUES", "general-purpose", false),
		dlgClaudeResultado("toolu_DESPUES", false, "listo"))
	return dlgClaudeSalida(lineas...)
}

// CA-362 y CA-373 (hallazgo 3b4ee7, enmienda 5), con el claude falso de
// CA-374: un run de Claude con delegaciones en segundo plano que cierran con
// una task_notification de status de 2 MiB. El agent_end de cada una se
// escribe con "<agente> fallo (<status recortado>)" en una linea de unos
// pocos KiB (ninguna linea del .jsonl carga el status de 2 MiB), lo que el
// run tiene en memoria (Manager.Events) es lo que escribio, y ReadDelegations
// lo devuelve como un fallo, con su marca, entre sus vecinas. Un status corto
// sale entero: "<agente> fallo (failed)".
func TestHallazgo_3b4ee7_CA362_CA373_StatusDe2MiBEnSegundoPlanoSeEscribeRecortado(t *testing.T) {
	const ca = "CA-362/CA-373 (hallazgo 3b4ee7, enmienda 5)"
	fs := c4v8Fondos(t)
	salida := c4v8Salida(fs)
	root, id, mem := dlgCorrerSalida(t, "claude", salida)
	disco := dlgDisco(t, root, id)

	// el .jsonl: eventos JSON validos, ninguno con el status de 2 MiB, y el
	// agent_end de cada fallo una vez, recortado, en una linea de pocos KiB
	fins := map[string][]dlgLinea{}
	for i, l := range disco {
		if !l.ok {
			t.Errorf("%s: la linea %d del .jsonl es un evento JSON valido: %.120q", ca, i, l.raw)
			continue
		}
		if len(l.raw) > c4v8OtraLineaMax {
			t.Errorf("%s: ninguna linea del .jsonl carga el status de 2 MiB: la linea %d mide %d bytes: %s", ca, i, len(l.raw), dlgResumen(l.ev))
		}
		if l.ev.Kind == "agent_end" {
			fins[l.ev.ToolID] = append(fins[l.ev.ToolID], l)
		}
	}
	for _, f := range fs {
		cc := ca + ", " + f.nombre
		got := fins[f.tool]
		if len(got) != 1 {
			t.Errorf("%s: su agent_end se escribe exactamente una vez; hay %d", cc, len(got))
			continue
		}
		l := got[0]
		if len(l.raw) > c4v8LineaMax {
			t.Errorf("%s: la linea de su agent_end mide como mucho %d bytes (el status va recortado a 300 bytes); mide %d", cc, c4v8LineaMax, len(l.raw))
		}
		if l.ev.Agent != f.agente || l.ev.Detail != f.detail || l.ev.TS.IsZero() {
			t.Errorf("%s: su agent_end se escribe con su agente, su ts y el detail \"<agente> fallo (<status recortado>)\":\n  quiere agent=%q detail=%.80q... (%d bytes)\n  fue    %s",
				cc, f.agente, f.detail, len(f.detail), dlgResumen(l.ev))
		}
	}

	// lo que el run tiene en memoria es lo que escribio
	if len(mem) != len(disco) {
		t.Errorf("%s: el run guarda en memoria los mismos eventos que escribe: %d en memoria, %d lineas en el .jsonl", ca, len(mem), len(disco))
	} else {
		for i := range mem {
			if !dlgMismoEvento(mem[i], disco[i].ev) {
				t.Errorf("%s: el run guarda en memoria el mismo evento que escribe (evento %d):\n  memoria %s\n  disco   %s",
					ca, i, dlgResumen(mem[i]), dlgResumen(disco[i].ev))
			}
		}
	}
	for i, e := range mem {
		if len(e.Detail) > c4v8LineaMax {
			t.Errorf("%s: el run no retiene en memoria el status de 2 MiB: el evento %d tiene un detail de %d bytes: %s", ca, i, len(e.Detail), dlgResumen(e))
		}
	}

	// la historia lee cada fallo como un fallo, entero, entre sus vecinas
	ds, err := ReadDelegations(root, id)
	if err != nil {
		t.Fatalf("%s: ReadDelegations lee el run: %v", ca, err)
	}
	var orden, quiere []string
	for _, d := range ds {
		orden = append(orden, d.Kind+" "+d.ToolID)
	}
	quiere = append(quiere, "agent toolu_ANTES", "agent_end toolu_ANTES")
	for _, f := range fs {
		quiere = append(quiere, "agent "+f.tool, "agent_end "+f.tool)
	}
	quiere = append(quiere, "agent toolu_DESPUES", "agent_end toolu_DESPUES")
	if g, w := strings.Join(orden, ", "), strings.Join(quiere, ", "); g != w {
		t.Errorf("%s: ReadDelegations trae todas las delegaciones, una vez y en orden (la linea de cada agent_end entra en 1 MiB):\n  trajo  %s\n  quiere %s", ca, g, w)
	}
	for _, f := range fs {
		cc := ca + ", " + f.nombre
		var leidos []Event
		for _, d := range ds {
			if d.Kind == "agent_end" && d.ToolID == f.tool {
				leidos = append(leidos, d)
			}
		}
		if len(leidos) != 1 {
			t.Errorf("%s: la historia lee su agent_end una vez; ReadDelegations trajo %d:\n%s", cc, len(leidos), dlgListado(leidos))
			continue
		}
		d := leidos[0]
		if !strings.HasPrefix(d.Detail, f.agente+" fallo (") || d.Agent != f.agente {
			t.Errorf("%s: la historia publica el fallo como fallo: ReadDelegations da el agent_end con la marca \"<agente> fallo (\": %s", cc, dlgResumen(d))
		}
		if d.Detail != f.detail {
			t.Errorf("%s: ReadDelegations da el agent_end \"<agente> fallo (<status recortado>)\":\n  quiere %.80q... (%d bytes)\n  fue    %s", cc, f.detail, len(f.detail), dlgResumen(d))
		}
		for _, l := range fins[f.tool] {
			if !dlgMismoEvento(d, l.ev) {
				t.Errorf("%s: ReadDelegations da el agent_end tal como se escribio:\n  leido   %s\n  escrito %s", cc, dlgResumen(d), dlgResumen(l.ev))
			}
		}
	}
}
