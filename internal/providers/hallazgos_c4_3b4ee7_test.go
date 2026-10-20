// Tests de regresion del hallazgo 20261020T151253_3b4ee7 (review de la cabina
// C4, lente resilience) sobre .hoom/specs/historia-doctor-y-cinta.md: CA-373
// y la enmienda 5 ("#### Fuente telemetria"). Escritos desde el spec, la
// descripcion del hallazgo y los tests existentes, sin leer la
// implementacion. Los helpers de este archivo llevan el prefijo h3b4.
//
// El normalizador con memoria de Claude (Correlating) cierra una delegacion
// con una task_notification cuyo status no es completed y emite un agent_end
// con detail "<agente> fallo (<status>)". La enmienda 5 dice que el detail de
// un agent_end lo arma el normalizador "con el agente y textos que recorta a
// 300 bytes (la descripcion de la delegacion y el texto del fallo)": el
// status es el texto de ESE fallo, y se recorta con la misma regla que el
// texto de un tool_result con is_error ("<agente> fallo: <texto
// recortado>"). Hoy el status va entero: uno de 2 MiB (la linea del provider
// admite hasta 4 MiB) queda en el detail entero.
package providers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/quick"
)

const h3b4Limite = 300 // bytes del texto del fallo que el normalizador conserva (enmienda 5)

// h3b4Marca es la marca de recorte mas larga que se le tolera al texto de un
// fallo recortado (la que pone hoy el recorte de un tool_result es "…", 3
// bytes); solo acota el largo, la forma exacta sale de h3b4RecorteDeToolResult.
const h3b4Marca = 8

// h3b4RecorteDeToolResult es como el normalizador recorta el texto de un
// fallo en un tool_result con is_error: el detail "<agente> fallo: <texto
// recortado>" de una delegacion normal, sin el "<agente> fallo: ". Es la
// regla que la enmienda 5 aplica al texto de cualquier fallo, y la referencia
// del status recortado.
func h3b4RecorteDeToolResult(t *testing.T, texto string) string {
	t.Helper()
	const agente = "hoom-referencia"
	n := coNorm(t)
	n(coToolUse("toolu_h3b4_ref", "Agent", agente, false))
	txt, err := json.Marshal(texto)
	if err != nil {
		t.Fatal(err)
	}
	ends := coEnds(n(coToolResult("toolu_h3b4_ref", true, string(txt))))
	if len(ends) != 1 || !strings.HasPrefix(ends[0].Detail, agente+" fallo: ") {
		t.Fatalf("fixture: un tool_result con is_error cierra la delegacion con UN agent_end \"<agente> fallo: <texto recortado>\" (CA-373): %+v", ends)
	}
	return strings.TrimPrefix(ends[0].Detail, agente+" fallo: ")
}

// h3b4Notificacion es la task_notification de la delegacion id con el
// status dado, escrito como JSON valido (comillas, barras y unicode
// incluidos).
func h3b4Notificacion(t *testing.T, id, status string) string {
	t.Helper()
	st, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	return `{"type":"system","subtype":"task_notification","task_id":"t-h3b4","tool_use_id":"` + id +
		`","status":` + string(st) + `,"summary":"se corto a mitad de camino","session_id":"s-h3b4"}`
}

// h3b4Mezcla es un texto de exactamente n bytes, sin espacios en los bordes,
// con ASCII, comillas, barras y caracteres de 2, 3 y 4 bytes (puede partir
// el ultimo caracter si n cae en medio: solo se usa con n multiplo de su
// ciclo de 50 bytes o con n enorme, donde el corte a 300 cae en el borde de
// un ciclo).
func h3b4Mezcla(n int) string {
	piezas := []string{"relleno ", `"comillas" `, "ñandú ", "漢字 ", "🙂 ", `\barra\ `, "é", "x"}
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		b.WriteString(piezas[i%len(piezas)])
	}
	return b.String()[:n]
}

// h3b4Resumen describe un texto sin volcarlo entero.
func h3b4Resumen(s string) string {
	if len(s) <= 80 {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf("%.40q...%q (%d bytes)", s, s[len(s)-20:], len(s))
}

// h3b4Status es un status de una task_notification no completed.
type h3b4Status struct {
	nombre string
	status string
	// si el status pasa de 300 bytes y se tiene que recortar
	recorta bool
}

func h3b4Largos() []h3b4Status {
	return []h3b4Status{
		{"status de 2 MiB en ASCII", strings.Repeat("x", 2<<20), true},
		{"status de 2 MiB con comillas, barras y unicode", h3b4Mezcla(2 << 20), true},
		{"status de 301 bytes", strings.Repeat("x", 301), true},
		{"status de 301 bytes con el byte 300 en medio de una ñ", "a" + strings.Repeat("ñ", 150), true},
		{"status de 300 bytes exactos", strings.Repeat("x", 300), false},
		{"status de 300 bytes exactos con unicode", h3b4Mezcla(300), false},
	}
}

// h3b4ExigirRecortado exige, para el detail de un agent_end cerrado por una
// task_notification con status, la forma "<agente> fallo (<status
// recortado>)", con el status recortado igual que el texto de un tool_result
// con is_error; y, aparte de la referencia, que el recorte deje a lo sumo 300
// bytes del status (mas su marca) y sea su principio.
func h3b4ExigirRecortado(t *testing.T, ca, agente string, s h3b4Status, detail string) {
	t.Helper()
	ref := h3b4RecorteDeToolResult(t, s.status)
	if want := agente + " fallo (" + ref + ")"; detail != want {
		t.Errorf("%s: %s: el agent_end es \"<agente> fallo (<status recortado>)\", con el status recortado como el texto de un tool_result con is_error (enmienda 5: el normalizador recorta a 300 bytes el texto del fallo):\n  quiere %s\n  fue    %s",
			ca, s.nombre, h3b4Resumen(want), h3b4Resumen(detail))
	}
	pre, suf := agente+" fallo (", ")"
	if !strings.HasPrefix(detail, pre) || !strings.HasSuffix(detail, suf) || len(detail) < len(pre)+len(suf) {
		t.Errorf("%s: %s: el agent_end conserva la marca \"<agente> fallo (\" y cierra con \")\": %s", ca, s.nombre, h3b4Resumen(detail))
		return
	}
	got := detail[len(pre) : len(detail)-len(suf)]
	if !s.recorta {
		if got != s.status {
			t.Errorf("%s: %s: un status de hasta 300 bytes sale entero: quiere %s, fue %s", ca, s.nombre, h3b4Resumen(s.status), h3b4Resumen(got))
		}
		return
	}
	if got == s.status {
		t.Errorf("%s: %s: un status de %d bytes no entra entero en el detail: se recorta a 300 bytes", ca, s.nombre, len(s.status))
	}
	if len(got) > h3b4Limite+h3b4Marca {
		t.Errorf("%s: %s: el status recortado mide a lo sumo 300 bytes mas su marca, midio %d", ca, s.nombre, len(got))
	}
	if cuerpo := strings.TrimRight(got, "….· "); !strings.HasPrefix(s.status, cuerpo) {
		t.Errorf("%s: %s: el status recortado es el principio del status: %s", ca, s.nombre, h3b4Resumen(got))
	}
}

// CA-373 (hallazgo 3b4ee7, enmienda 5): una delegacion en segundo plano
// (run_in_background: true) que cierra con una task_notification de status
// largo (2 MiB en ASCII y con unicode, 301 bytes, 301 con el corte en medio
// de un caracter) da su system y despues UN agent_end "<agente> fallo
// (<status recortado>)", con el status recortado como el texto de un
// tool_result con is_error. Uno de 300 bytes exactos no se recorta.
func TestHallazgo_3b4ee7_CA373_SegundoPlanoConStatusLargoSaleRecortado(t *testing.T) {
	const ca = "CA-373 (hallazgo 3b4ee7)"
	const agente = "hoom-test-writer"
	for i, s := range h3b4Largos() {
		id := fmt.Sprintf("toolu_h3b4_fondo_%d", i)
		n := coNorm(t)
		if evs := n(coToolUse(id, "Agent", agente, true)); len(evs) != 1 || evs[0].Kind != "agent" || evs[0].ToolID != id {
			t.Fatalf("%s: %s: la delegacion en segundo plano es un agent con su tool_id: %+v", ca, s.nombre, evs)
		}
		if ends := coEnds(n(coToolResult(id, false, `"Async agent launched successfully"`))); len(ends) != 0 {
			t.Fatalf("%s: %s: el tool_result inmediato de una delegacion en segundo plano no la cierra: %+v", ca, s.nombre, ends)
		}
		evs := n(h3b4Notificacion(t, id, s.status))
		if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
			t.Errorf("%s: %s: la task_notification emite su system y despues UN agent_end: %v", ca, s.nombre, coKinds(evs))
			continue
		}
		e := evs[1]
		if e.Agent != agente || e.ToolID != id {
			t.Errorf("%s: %s: el agent_end lleva agent y tool_id de la delegacion: agent=%s tool_id=%q", ca, s.nombre, h3b4Resumen(e.Agent), e.ToolID)
		}
		h3b4ExigirRecortado(t, ca, agente, s, e.Detail)
		if ends := coEnds(n(h3b4Notificacion(t, id, s.status))); len(ends) != 0 {
			t.Errorf("%s: %s: un solo agent_end por delegacion; otra notificacion no emite otro: %d", ca, s.nombre, len(ends))
		}
	}
}

// CA-373 (hallazgo 3b4ee7, enmienda 5): una delegacion normal que cierra con
// una task_notification de status largo antes de su tool_result tambien da
// "<agente> fallo (<status recortado>)", con la misma regla.
func TestHallazgo_3b4ee7_CA373_DelegacionNormalConStatusLargoSaleRecortado(t *testing.T) {
	const ca = "CA-373 (hallazgo 3b4ee7)"
	const agente = "hoom-scout"
	for i, s := range h3b4Largos() {
		id := fmt.Sprintf("toolu_h3b4_normal_%d", i)
		n := coNorm(t)
		n(coToolUse(id, "Agent", agente, false))
		evs := n(h3b4Notificacion(t, id, s.status))
		if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
			t.Errorf("%s: %s: la task_notification terminal cierra la delegacion normal: system y despues agent_end: %v", ca, s.nombre, coKinds(evs))
			continue
		}
		if evs[1].Agent != agente || evs[1].ToolID != id {
			t.Errorf("%s: %s: el agent_end lleva agent y tool_id de la delegacion: %q %q", ca, s.nombre, evs[1].Agent, evs[1].ToolID)
		}
		h3b4ExigirRecortado(t, ca, agente, s, evs[1].Detail)
		if ends := coEnds(n(coToolResult(id, false, `"tarde"`))); len(ends) != 0 {
			t.Errorf("%s: %s: el tool_result posterior no emite otro agent_end: %d", ca, s.nombre, len(ends))
		}
	}
}

// CA-373 (hallazgo 3b4ee7, enmienda 5), con las lineas REALES de Claude Code
// 2.1.281: el tool_use real en segundo plano y la task_notification real con
// un status de 2 MiB en lugar de "completed" cierran con "general-purpose
// fallo (<status recortado>)".
func TestHallazgo_3b4ee7_CA373_LineasRealesConStatusDe2MiB(t *testing.T) {
	const ca = "CA-373 (hallazgo 3b4ee7, lineas reales de 2.1.281)"
	s := h3b4Status{"status de 2 MiB en la task_notification real", strings.Repeat("y", 2<<20), true}
	n := coNorm(t)
	if evs := n(hcToolUseRealSegundoPlano(t)); len(evs) != 1 || evs[0].Kind != "agent" || evs[0].ToolID != coRealID {
		t.Fatalf("%s: el tool_use real en segundo plano es un agent con su tool_id: %+v", ca, evs)
	}
	n(coRealTaskStarted)
	n(coRealInterna)
	evs := n(hcNotifReal(t, s.status))
	if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
		t.Fatalf("%s: la task_notification real emite su system y despues el agent_end: %v", ca, coKinds(evs))
	}
	if evs[1].Agent != "general-purpose" || evs[1].ToolID != coRealID {
		t.Fatalf("%s: el agent_end lleva agent general-purpose y el tool_id real: %q %q", ca, evs[1].Agent, evs[1].ToolID)
	}
	h3b4ExigirRecortado(t, ca, "general-purpose", s, evs[1].Detail)
	if ends := coEnds(n(coRealToolResult)); len(ends) != 0 {
		t.Fatalf("%s: el tool_result real posterior no emite otro agent_end: %+v", ca, ends)
	}
}

// CA-373 (hallazgo 3b4ee7, guarda): el recorte no toca los estados de
// siempre. Un status corto sale entero ("<agente> fallo (failed)"), y
// completed sigue dando "<agente> termino", en segundo plano y en una
// delegacion normal.
func TestHallazgo_3b4ee7_CA373_StatusCortoEnteroYCompletedTermina(t *testing.T) {
	const ca = "CA-373 (hallazgo 3b4ee7, guarda)"
	const agente = "hoom-scout"
	for _, fondo := range []bool{true, false} {
		for _, c := range []struct{ status, detail string }{
			{"failed", agente + " fallo (failed)"},
			{"killed", agente + " fallo (killed)"},
			{"completed", agente + " termino"},
		} {
			id := fmt.Sprintf("toolu_h3b4_corto_%s_%v", c.status, fondo)
			n := coNorm(t)
			n(coToolUse(id, "Agent", agente, fondo))
			if fondo {
				n(coToolResult(id, false, `"Async agent launched successfully"`))
			}
			evs := n(h3b4Notificacion(t, id, c.status))
			if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
				t.Errorf("%s: status %s (segundo plano %v): system y despues agent_end: %v", ca, c.status, fondo, coKinds(evs))
				continue
			}
			if evs[1].Detail != c.detail || evs[1].Agent != agente || evs[1].ToolID != id {
				t.Errorf("%s: status %s (segundo plano %v): agent_end %q (agent %q, tool_id %q), quiere %q",
					ca, c.status, fondo, evs[1].Detail, evs[1].Agent, evs[1].ToolID, c.detail)
			}
		}
	}
}

// CA-373 (hallazgo 3b4ee7, enmienda 5), como propiedad: para cualquier status
// no completed armado con ASCII, comillas, barras y caracteres de 2, 3 y 4
// bytes (de 1 byte a unos KiB, alrededor del borde de 300), la delegacion en
// segundo plano cierra con "<agente> fallo (<status recortado>)", recortado
// como el texto de un tool_result con is_error: entero hasta 300 bytes, y
// recortado a 300 bytes mas su marca si pasa.
func TestHallazgo_3b4ee7_CA373_PropiedadStatusRecortadoComoToolResult(t *testing.T) {
	const ca = "CA-373 (hallazgo 3b4ee7, propiedad)"
	const agente = "hoom-prop"
	alfabeto := []string{"x", "ñ", "漢", "🙂", `"`, `\`, "-", "é"}
	prop := func(bs []uint8) bool {
		var b strings.Builder
		b.WriteString("x")
		for _, v := range bs {
			b.WriteString(strings.Repeat(alfabeto[int(v)%len(alfabeto)], int(v)/len(alfabeto)%16+1))
		}
		s := h3b4Status{nombre: fmt.Sprintf("status de %d bytes", b.Len()), status: b.String()}
		s.recorta = len(s.status) > h3b4Limite
		n := coNorm(t)
		n(coToolUse("toolu_h3b4_prop", "Agent", agente, true))
		evs := n(h3b4Notificacion(t, "toolu_h3b4_prop", s.status))
		if len(evs) != 2 || evs[1].Kind != "agent_end" {
			t.Errorf("%s: %s: system y despues agent_end: %v", ca, s.nombre, coKinds(evs))
			return false
		}
		antes := t.Failed()
		h3b4ExigirRecortado(t, ca, agente, s, evs[1].Detail)
		return antes || !t.Failed()
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("%s: %v", ca, err)
	}
}
