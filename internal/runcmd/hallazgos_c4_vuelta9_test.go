// Tests de regresion de la novena vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd: los hallazgos
// 20261020T184842_aa41d6 (lente reliability), 20261020T185039_d60369
// (resilience) y 20261020T184551_f94377 (risk) de la delta 5. Escritos desde
// el spec (CA-362, CA-373, la enmienda 6 y la "#### Fuente telemetria"), la
// descripcion de los hallazgos y los tests existentes, sin leer la
// implementacion. Los helpers de este archivo llevan el prefijo c4v9; los
// compartidos (prefijo dlg) estan en delegaciones_helpers_test.go.
//
// Los hallazgos: el normalizador recorta a 300 bytes cortando por bytes, y
// puede partir un caracter. Con un status "a" + 150 "ñ" (301 bytes) el
// detail del agent_end queda con UTF-8 invalido en memoria (Manager.Events)
// y, escrito como JSON, con U+FFFD en el .jsonl: memoria y disco no
// coinciden, y la historia (ReadDelegations) lee un texto alterado.
//
// El contrato que exigen estos tests: el normalizador recorta esos textos a
// 300 bytes SIN partir un caracter (UTF-8 valido, un prefijo del original de
// 297 a 300 bytes mas la marca de recorte), y el run guarda en memoria lo
// mismo que escribe.
package runcmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	c4v9Limite  = 300
	c4v9Minimo  = 297 // un caracter mide a lo sumo 4 bytes
	c4v9MarcaMx = 8   // largo maximo tolerado de la marca de recorte (no la fija el spec)
)

// c4v9Resumen describe un texto sin volcarlo entero, con su cola en hex.
func c4v9Resumen(s string) string {
	if len(s) <= 40 {
		return fmt.Sprintf("%q (%d bytes)", s, len(s))
	}
	return fmt.Sprintf("%.24q...cola % x (%d bytes)", s, s[len(s)-8:], len(s))
}

// c4v9Falta es el oraculo propio (no pasa por el normalizador): dice que
// incumple cuerpo, el texto como quedo sin la envoltura del detail, para el
// original de mas de 300 bytes y la marca de recorte; vacio si cumple.
func c4v9Falta(orig, cuerpo, marca string) []string {
	var mal []string
	if !utf8.ValidString(cuerpo) {
		mal = append(mal, "no es UTF-8 valido: el recorte parte un caracter")
	}
	if strings.Contains(cuerpo, "\uFFFD") {
		mal = append(mal, "trae U+FFFD, que el original no tiene")
	}
	if !strings.HasSuffix(cuerpo, marca) {
		return append(mal, fmt.Sprintf("no termina con la marca de recorte %q", marca))
	}
	p := strings.TrimSuffix(cuerpo, marca)
	if !strings.HasPrefix(orig, p) {
		mal = append(mal, "lo que conserva no es un prefijo del original")
	}
	if len(p) > c4v9Limite || len(p) < c4v9Minimo {
		mal = append(mal, fmt.Sprintf("conserva %d bytes del original; son de 297 a 300 (el borde de caracter es %d)", len(p), len(dlgCabeza(orig, c4v9Limite))))
	}
	return mal
}

// c4v9Envuelto saca el cuerpo de un detail "<pre><cuerpo><suf>".
func c4v9Envuelto(detail, pre, suf string) (string, bool) {
	if !strings.HasPrefix(detail, pre) || !strings.HasSuffix(detail, suf) || len(detail) < len(pre)+len(suf) {
		return "", false
	}
	return detail[len(pre) : len(detail)-len(suf)], true
}

// c4v9Notificacion es la task_notification de la delegacion id con el
// status dado, escrito como JSON valido.
func c4v9Notificacion(id, status string) string {
	st, _ := json.Marshal(status)
	return `{"type":"system","subtype":"task_notification","task_id":"t-` + id + `","tool_use_id":"` + id +
		`","status":` + string(st) + `,"summary":"aviso","session_id":"` + dlgSesion + `"}`
}

// c4v9ToolUse es el tool_use Agent de una delegacion normal con la
// descripcion dada.
func c4v9ToolUse(id, agente, descripcion string) string {
	in, _ := json.Marshal(map[string]any{"description": descripcion, "prompt": "mapea", "subagent_type": agente, "run_in_background": false})
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id +
		`","name":"Agent","input":` + string(in) + `}]},"parent_tool_use_id":null,"session_id":"` + dlgSesion + `"}`
}

// c4v9Texto es la linea assistant con un bloque text.
func c4v9Texto(texto string) string {
	txt, _ := json.Marshal(texto)
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":` + string(txt) +
		`}]},"parent_tool_use_id":null,"session_id":"` + dlgSesion + `"}`
}

// El texto de los hallazgos: 301 bytes, y el byte 300 es el segundo de la
// ultima ñ. Con 漢 (3 bytes) y 🙂 (4 bytes) el corte tambien cae adentro de
// un caracter.
var (
	c4v9Enie  = "a" + strings.Repeat("ñ", 150)
	c4v9Kanji = "a" + strings.Repeat("漢", 100)
	c4v9Cara  = "a" + strings.Repeat("🙂", 75)
	c4v9ASCII = strings.Repeat("x", 301)
)

// c4v9Fallo es una delegacion del run que cierra con un fallo.
type c4v9Fallo struct {
	nombre, agente, tool, texto string
	// pre y suf envuelven el texto recortado en el detail del agent_end
	pre, suf string
}

func c4v9Fallos() []c4v9Fallo {
	porStatus := func(nombre, agente, tool, texto string) c4v9Fallo {
		return c4v9Fallo{nombre, agente, tool, texto, agente + " fallo (", ")"}
	}
	return []c4v9Fallo{
		porStatus("status ASCII de 301 bytes (da la marca de recorte)", "hoom-ascii", "toolu_V9_ASCII", c4v9ASCII),
		porStatus("status \"a\" + 150 \"ñ\" en segundo plano", "hoom-fondo", "toolu_V9_STATUS", c4v9Enie),
		{"tool_result con is_error y texto \"a\" + 150 \"ñ\"", "hoom-scout", "toolu_V9_RESULT", c4v9Enie, "hoom-scout fallo: ", ""},
		porStatus("status \"a\" + 100 \"漢\" de una delegacion normal", "hoom-kanji", "toolu_V9_KANJI", c4v9Kanji),
		porStatus("status \"a\" + 75 \"🙂\" en segundo plano", "hoom-cara", "toolu_V9_CARA", c4v9Cara),
	}
}

// c4v9Salida es la salida del claude falso (CA-374): las delegaciones que
// fallan (las de status en segundo plano con su tool_result inmediato), una
// delegacion normal con la descripcion "a" + 150 "ñ" que termina, y un text
// "a" + 150 "ñ".
func c4v9Salida() []byte {
	var lineas []string
	for _, f := range c4v9Fallos() {
		switch f.tool {
		case "toolu_V9_RESULT":
			lineas = append(lineas, dlgClaudeToolUse(f.tool, f.agente, false), dlgClaudeResultado(f.tool, true, f.texto))
		case "toolu_V9_KANJI":
			lineas = append(lineas, dlgClaudeToolUse(f.tool, f.agente, false), c4v9Notificacion(f.tool, f.texto))
		default:
			lineas = append(lineas,
				dlgClaudeToolUse(f.tool, f.agente, true),
				dlgClaudeResultado(f.tool, false, "Async agent launched successfully"),
				c4v9Notificacion(f.tool, f.texto))
		}
	}
	lineas = append(lineas,
		c4v9ToolUse("toolu_V9_DESC", "hoom-desc", c4v9Enie),
		dlgClaudeResultado("toolu_V9_DESC", false, "listo"),
		c4v9Texto(c4v9Enie))
	return dlgClaudeSalida(lineas...)
}

// CA-362 y CA-373 (enmienda 6; hallazgos aa41d6, d60369 y f94377), con el
// claude falso de CA-374: un run de Claude con una delegacion que falla con
// status "a" + 150 "ñ", otra con un tool_result is_error del mismo texto
// (y status de 3 y 4 bytes por caracter, una descripcion y un text con el
// mismo texto). Lo que el run tiene en memoria (Manager.Events) es igual,
// evento por evento, a lo que escribio en el .jsonl; cada detail es UTF-8
// valido y sin U+FFFD; el texto de cada fallo es un prefijo del original de
// 297 a 300 bytes mas la marca; y ReadDelegations devuelve lo mismo que la
// memoria y el disco.
func TestHallazgos_aa41d6_d60369_f94377_CA362_CA373_RecorteUTF8MemoriaIgualADisco(t *testing.T) {
	const ca = "CA-362/CA-373 (enmienda 6; hallazgos aa41d6, d60369, f94377)"
	root, id, mem := dlgCorrerSalida(t, "claude", c4v9Salida())
	disco := dlgDisco(t, root, id)

	// lo que el run tiene en memoria es lo que escribio
	if len(mem) != len(disco) {
		t.Errorf("%s: el run guarda en memoria los mismos eventos que escribe: %d en memoria, %d lineas en el .jsonl", ca, len(mem), len(disco))
	} else {
		for i := range mem {
			if !disco[i].ok {
				t.Errorf("%s: la linea %d del .jsonl es un evento JSON valido: %.120q", ca, i, disco[i].raw)
				continue
			}
			if !dlgMismoEvento(mem[i], disco[i].ev) {
				t.Errorf("%s: el run guarda en memoria el mismo evento que escribe (evento %d, %s):\n  memoria %s detail %s\n  disco   %s detail %s",
					ca, i, mem[i].Kind, dlgResumen(mem[i]), c4v9Resumen(mem[i].Detail), dlgResumen(disco[i].ev), c4v9Resumen(disco[i].ev.Detail))
			}
		}
	}
	for i, e := range mem {
		if !utf8.ValidString(e.Detail) || strings.Contains(e.Detail, "\uFFFD") {
			t.Errorf("%s: el evento %d (%s) esta en memoria con su detail en UTF-8 valido y sin U+FFFD: %s", ca, i, e.Kind, c4v9Resumen(e.Detail))
		}
	}
	for i, l := range disco {
		if l.ok && strings.Contains(l.ev.Detail, "\uFFFD") {
			t.Errorf("%s: la linea %d (%s) del .jsonl no trae U+FFFD en su detail: el texto se recorto partiendo un caracter: %s", ca, i, l.ev.Kind, c4v9Resumen(l.ev.Detail))
		}
	}

	// la marca de recorte: lo que sigue a los 300 bytes del status ASCII
	var marca string
	fallos := c4v9Fallos()
	for _, e := range mem {
		if e.Kind == "agent_end" && e.ToolID == fallos[0].tool {
			c, ok := c4v9Envuelto(e.Detail, fallos[0].pre, fallos[0].suf)
			marca = strings.TrimPrefix(c, c4v9ASCII[:c4v9Limite])
			if !ok || !strings.HasPrefix(c, c4v9ASCII[:c4v9Limite]) || marca == "" || len(marca) > c4v9MarcaMx || !utf8.ValidString(marca) || strings.HasPrefix(marca, "x") {
				t.Fatalf("%s: fixture: un status ASCII de 301 bytes sale como sus primeros 300 bytes mas una marca de recorte de 1 a %d bytes: %s",
					ca, c4v9MarcaMx, c4v9Resumen(e.Detail))
			}
		}
	}
	if marca == "" {
		t.Fatalf("%s: fixture: el run tiene en memoria el agent_end de %s", ca, fallos[0].tool)
	}

	// el texto de cada fallo, en memoria, sale recortado sin partir un caracter
	for _, f := range fallos[1:] {
		cc := ca + ", " + f.nombre
		var ends []Event
		for _, e := range mem {
			if e.Kind == "agent_end" && e.ToolID == f.tool {
				ends = append(ends, e)
			}
		}
		if len(ends) != 1 {
			t.Errorf("%s: el run tiene en memoria UN agent_end de la delegacion; hay %d", cc, len(ends))
			continue
		}
		c, ok := c4v9Envuelto(ends[0].Detail, f.pre, f.suf)
		if !ok || ends[0].Agent != f.agente {
			t.Errorf("%s: el agent_end es %q<texto recortado>%q con el agente %q: %s", cc, f.pre, f.suf, f.agente, c4v9Resumen(ends[0].Detail))
			continue
		}
		if mal := c4v9Falta(f.texto, c, marca); len(mal) > 0 {
			t.Errorf("%s: el texto del fallo sale recortado a 300 bytes sin partir un caracter: %s; en memoria quedo %s",
				cc, strings.Join(mal, "; "), c4v9Resumen(c))
		}
	}
	// la descripcion de la delegacion y el text
	vistos := map[string]int{}
	for _, e := range mem {
		var que, c string
		var ok bool
		switch {
		case e.Kind == "agent" && e.ToolID == "toolu_V9_DESC":
			que = "la descripcion \"a\" + 150 \"ñ\" de una delegacion (evento agent)"
			c, ok = c4v9Envuelto(e.Detail, "Agent: ", "")
		case e.Kind == "text" && strings.HasPrefix(e.Detail, "añ"):
			que = "el texto \"a\" + 150 \"ñ\" de un evento text"
			c, ok = e.Detail, true
		default:
			continue
		}
		vistos[e.Kind]++
		if !ok {
			t.Errorf("%s, %s: el detail lleva el texto recortado: %s", ca, que, c4v9Resumen(e.Detail))
			continue
		}
		if mal := c4v9Falta(c4v9Enie, c, marca); len(mal) > 0 {
			t.Errorf("%s, %s: el texto sale recortado a 300 bytes sin partir un caracter: %s; en memoria quedo %s",
				ca, que, strings.Join(mal, "; "), c4v9Resumen(c))
		}
	}

	if vistos["agent"] != 1 || vistos["text"] != 1 {
		t.Errorf("%s: fixture: el run tiene en memoria UN agent de toolu_V9_DESC y UN text \"a\" + 150 \"ñ\": %v", ca, vistos)
	}

	// la historia lee lo mismo que el run tiene en memoria y escribio
	ds, err := ReadDelegations(root, id)
	if err != nil {
		t.Fatalf("%s: ReadDelegations lee el run: %v", ca, err)
	}
	var enMem, enDisco []Event
	for _, e := range mem {
		if e.Kind == "agent" || e.Kind == "agent_end" {
			enMem = append(enMem, e)
		}
	}
	for _, l := range disco {
		if l.ok && (l.ev.Kind == "agent" || l.ev.Kind == "agent_end") {
			enDisco = append(enDisco, l.ev)
		}
	}
	for nombre, quiere := range map[string][]Event{"la memoria del run (Manager.Events)": enMem, "el .jsonl": enDisco} {
		if len(ds) != len(quiere) {
			t.Errorf("%s: ReadDelegations devuelve las mismas %d delegaciones que %s; trajo %d:\n%squiere:\n%s",
				ca, len(quiere), nombre, len(ds), dlgListado(ds), dlgListado(quiere))
			continue
		}
		for i := range ds {
			if !dlgMismoEvento(ds[i], quiere[i]) {
				t.Errorf("%s: ReadDelegations devuelve la delegacion %d igual que %s:\n  leida  %s detail %s\n  quiere %s detail %s",
					ca, i, nombre, dlgResumen(ds[i]), c4v9Resumen(ds[i].Detail), dlgResumen(quiere[i]), c4v9Resumen(quiere[i].Detail))
			}
		}
	}
}
