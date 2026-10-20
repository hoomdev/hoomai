// Tests de regresion de los hallazgos 20261020T184842_aa41d6 (lente
// reliability), 20261020T185039_d60369 (resilience) y 20261020T184551_f94377
// (risk) de la delta 5 de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md: CA-373 y la enmienda 6 (el status
// de una task_notification fallida se recorta a 300 bytes como el texto de un
// tool_result fallido), con la "#### Fuente telemetria" (el normalizador arma
// el detail con textos que recorta a 300 bytes: la descripcion de la
// delegacion, el texto del fallo y el status). Escritos desde el spec, la
// descripcion de los hallazgos y los tests existentes, sin leer la
// implementacion. Los helpers de este archivo llevan el prefijo hu8.
//
// Los tres hallazgos son una sola causa: el recorte a 300 bytes corta por
// bytes y puede partir un caracter. "a" + 150 veces "ñ" mide 301 bytes y su
// byte 300 es el segundo de la ultima ñ: el detail queda con UTF-8 invalido
// en memoria y, escrito como JSON, con U+FFFD en el disco. Los tests de
// 3b4ee7 no lo vieron porque usaban el mismo recorte como oraculo.
//
// El contrato que exigen estos tests, con un oraculo PROPIO que no pasa por el
// normalizador: el normalizador recorta esos textos a 300 bytes SIN partir un
// caracter. Un texto de hasta 300 bytes sale entero; uno mas largo sale como
// un prefijo del original de a lo sumo 300 bytes y de por lo menos 297 (un
// caracter mide como mucho 4), en UTF-8 valido, sin U+FFFD, mas la marca de
// recorte. Y ningun evento que el normalizador emite de una linea en UTF-8
// valido trae un detail en UTF-8 invalido.
package providers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/quick"
	"unicode/utf8"
)

const (
	hu8Limite = 300 // bytes del texto que el normalizador conserva, como mucho
	hu8Minimo = 297 // 300 menos 3: el corte retrocede a lo sumo un caracter de 4 bytes
	// hu8MarcaMax es el largo maximo que se le tolera a la marca de recorte
	// (no la fija el spec; la de hoy es de 3 bytes).
	hu8MarcaMax = 8
)

// hu8Caracteres son los caracteres multibyte de los textos: de 2, 3 y 4 bytes.
// € empieza con 0xE2, el mismo primer byte que "…".
var hu8Caracteres = []struct{ nombre, c string }{
	{"ñ", "ñ"},
	{"漢", "漢"},
	{"€", "€"},
	{"🙂", "🙂"},
}

// hu8Texto es un texto del caso: el original y como se lo nombra.
type hu8Texto struct {
	nombre, texto string
}

func hu8Rep(s string, n int) string { return strings.Repeat(s, n) }

// hu8Largos son textos de mas de 300 bytes armados para que el byte 300 caiga
// en CADA posicion posible de un caracter de 2, 3 y 4 bytes (y en su borde):
// j letras "a" y despues caracteres multibyte, con j de 0 a w-1; un solo
// caracter que pisa el byte 300 seguido de ASCII; y uno de 64 KiB por
// caracter. Entre ellos esta el ejemplo de los hallazgos, "a" + 150 "ñ".
func hu8Largos() []hu8Texto {
	var out []hu8Texto
	for _, ch := range hu8Caracteres {
		w := len(ch.c)
		for j := 0; j < w; j++ {
			for _, extra := range []int{1, 2, 40} {
				c := (hu8Limite-j)/w + extra
				s := hu8Rep("a", j) + hu8Rep(ch.c, c)
				out = append(out, hu8Texto{fmt.Sprintf("%d \"a\" y %d %q (%d bytes, %s)", j, c, ch.c, len(s), hu8Donde((hu8Limite-j)%w, w)), s})
			}
		}
		for j := hu8Limite - w + 1; j <= hu8Limite; j++ {
			s := hu8Rep("a", j) + ch.c + hu8Rep("b", 8)
			out = append(out, hu8Texto{fmt.Sprintf("%d \"a\", un %q y 8 \"b\" (%d bytes, %s)", j, ch.c, len(s), hu8Donde(hu8Limite-j, w)), s})
		}
		s := "a" + hu8Rep(ch.c, (64<<10)/w)
		out = append(out, hu8Texto{fmt.Sprintf("\"a\" y %q hasta %d bytes", ch.c, len(s)), s})
	}
	return out
}

// hu8Donde dice donde cae el corte a 300 bytes: en el borde de un caracter de
// w bytes (pos 0) o adentro, despues de pos de sus bytes.
func hu8Donde(pos, w int) string {
	if pos == 0 {
		return "el corte a 300 cae en un borde de caracter"
	}
	return fmt.Sprintf("el corte a 300 parte un caracter de %d bytes despues de su byte %d", w, pos)
}

// hu8Cortos son textos de hasta 300 bytes que salen enteros, con caracteres
// de 2, 3 y 4 bytes que terminan justo en el byte 300 o antes.
func hu8Cortos() []hu8Texto {
	out := []hu8Texto{
		{"300 \"a\"", hu8Rep("a", 300)},
		{"un \"a\"", "a"},
	}
	for _, ch := range hu8Caracteres {
		w := len(ch.c)
		for j := 0; j < w; j++ {
			c := (hu8Limite - j) / w
			s := hu8Rep("a", j) + hu8Rep(ch.c, c)
			out = append(out, hu8Texto{fmt.Sprintf("%d \"a\" y %d %q (%d bytes)", j, c, ch.c, len(s)), s})
		}
	}
	return out
}

// hu8Borde es el oraculo propio del largo esperado: el prefijo mas largo de s
// de a lo sumo 300 bytes que no parte un caracter. Solo se usa para describir
// la falla; el contrato admite de 297 a 300 bytes.
func hu8Borde(s string) int {
	n := hu8Limite
	if len(s) <= n {
		return len(s)
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return n
}

// hu8Falta dice que incumple cuerpo (el texto como lo dejo el normalizador,
// sin la envoltura del detail) para el original, con la marca de recorte del
// canal; vacio si cumple el contrato. No pasa por el normalizador.
func hu8Falta(orig, cuerpo, marca string) []string {
	var mal []string
	if !utf8.ValidString(cuerpo) {
		mal = append(mal, "no es UTF-8 valido: el recorte parte un caracter")
	}
	if strings.Contains(cuerpo, "\uFFFD") && !strings.Contains(orig, "\uFFFD") {
		mal = append(mal, "trae U+FFFD, que el original no tiene")
	}
	if len(orig) <= hu8Limite {
		if cuerpo != orig {
			mal = append(mal, "un texto de hasta 300 bytes sale entero")
		}
		return mal
	}
	if !strings.HasSuffix(cuerpo, marca) {
		return append(mal, fmt.Sprintf("no termina con la marca de recorte %q", marca))
	}
	p := strings.TrimSuffix(cuerpo, marca)
	if !strings.HasPrefix(orig, p) {
		mal = append(mal, "lo que conserva no es un prefijo del original")
	}
	if len(p) > hu8Limite || len(p) < hu8Minimo {
		mal = append(mal, fmt.Sprintf("conserva %d bytes del original; son de 297 a 300 (el borde de caracter es %d)", len(p), hu8Borde(orig)))
	}
	if !utf8.ValidString(p) {
		mal = append(mal, fmt.Sprintf("lo que conserva (%d bytes) termina en un caracter partido; el borde de caracter es %d", len(p), hu8Borde(orig)))
	}
	return mal
}

// hu8Resumen describe un texto sin volcarlo entero, con su cola en hex.
func hu8Resumen(s string) string {
	if len(s) <= 40 {
		return fmt.Sprintf("%q (%d bytes)", s, len(s))
	}
	return fmt.Sprintf("%.24q...cola % x (%d bytes)", s, s[len(s)-8:], len(s))
}

// hu8DetailsValidos exige que ningun evento emitido traiga un detail (ni un
// agente) en UTF-8 invalido o con U+FFFD: la linea del provider es UTF-8
// valido y sin U+FFFD.
func hu8DetailsValidos(t *testing.T, ca, que string, evs []Event) bool {
	t.Helper()
	ok := true
	for _, e := range evs {
		for campo, v := range map[string]string{"detail": e.Detail, "agent": e.Agent} {
			if !utf8.ValidString(v) || strings.Contains(v, "\uFFFD") {
				t.Errorf("%s: %s: el evento %s sale con su %s en UTF-8 valido y sin U+FFFD (lo que el run guarda en memoria es lo que escribe): %s",
					ca, que, e.Kind, campo, hu8Resumen(v))
				ok = false
			}
		}
	}
	return ok
}

// hu8Canal es un texto que el normalizador de Claude recorta: emitir corre el
// texto por un normalizador nuevo y devuelve el cuerpo (el texto como quedo,
// sin la envoltura del detail) y todos los eventos emitidos en el camino.
type hu8Canal struct {
	nombre string
	emitir func(t *testing.T, texto string) (cuerpo string, evs []Event, err error)
}

func hu8JSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// hu8Notif es una task_notification de la delegacion id con el status dado,
// escrito como JSON valido.
func hu8Notif(t *testing.T, id, status string) string {
	return `{"type":"system","subtype":"task_notification","task_id":"t-hu8","tool_use_id":"` + id +
		`","status":` + hu8JSON(t, status) + `,"summary":"se corto","session_id":"s-hu8"}`
}

// hu8NotifReal es la task_notification REAL de Claude Code 2.1.281 con otro
// status (solo cambia el valor de "status", escrito como JSON).
func hu8NotifReal(t *testing.T, status string) string {
	t.Helper()
	const viejo = `"status":"completed"`
	if strings.Count(coRealNotification, viejo) != 1 {
		t.Fatalf("fixture: la task_notification real trae un solo %s", viejo)
	}
	return strings.Replace(coRealNotification, viejo, `"status":`+hu8JSON(t, status), 1)
}

// hu8ToolUse es un tool_use Agent con la descripcion dada.
func hu8ToolUse(t *testing.T, id, agente, descripcion string) string {
	in, err := json.Marshal(map[string]any{"description": descripcion, "prompt": "hace tu parte", "subagent_type": agente, "run_in_background": false})
	if err != nil {
		t.Fatal(err)
	}
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id +
		`","name":"Agent","input":` + string(in) + `}]},"parent_tool_use_id":null,"session_id":"s-hu8"}`
}

// hu8LineaTexto es la linea assistant con un bloque text.
func hu8LineaTexto(t *testing.T, texto string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":` + hu8JSON(t, texto) +
		`}]},"parent_tool_use_id":null,"session_id":"s-hu8"}`
}

// hu8Envuelto saca el cuerpo de un detail "<pre><cuerpo><suf>".
func hu8Envuelto(detail, pre, suf string) (string, error) {
	if !strings.HasPrefix(detail, pre) || !strings.HasSuffix(detail, suf) || len(detail) < len(pre)+len(suf) {
		return "", fmt.Errorf("el detail tiene la forma %q<texto>%q: %s", pre, suf, hu8Resumen(detail))
	}
	return detail[len(pre) : len(detail)-len(suf)], nil
}

// hu8UnEnd exige UN agent_end en evs y devuelve su detail.
func hu8UnEnd(evs []Event, agente, id string) (string, error) {
	ends := coEnds(evs)
	if len(ends) != 1 {
		return "", fmt.Errorf("cierra la delegacion con UN agent_end; hubo %d (%v)", len(ends), coKinds(evs))
	}
	if ends[0].Agent != agente || ends[0].ToolID != id {
		return "", fmt.Errorf("el agent_end lleva el agente y el tool_id de la delegacion: %q %q", ends[0].Agent, ends[0].ToolID)
	}
	return ends[0].Detail, nil
}

// hu8Uno exige UN evento del kind en evs y devuelve su detail.
func hu8Uno(evs []Event, kind string) (string, error) {
	var ds []string
	for _, e := range evs {
		if e.Kind == kind {
			ds = append(ds, e.Detail)
		}
	}
	if len(ds) != 1 {
		return "", fmt.Errorf("emite UN evento %s; hubo %d (%v)", kind, len(ds), coKinds(evs))
	}
	return ds[0], nil
}

func hu8Canales() []hu8Canal {
	return []hu8Canal{
		{"status de la task_notification que cierra con un fallo una delegacion en segundo plano", func(t *testing.T, texto string) (string, []Event, error) {
			const agente, id = "hoom-fondo", "toolu_hu8_fondo"
			n := coNorm(t)
			evs := n(coToolUse(id, "Agent", agente, true))
			evs = append(evs, n(coToolResult(id, false, `"Async agent launched successfully"`))...)
			ult := n(hu8Notif(t, id, texto))
			evs = append(evs, ult...)
			d, err := hu8UnEnd(ult, agente, id)
			if err != nil {
				return "", evs, err
			}
			c, err := hu8Envuelto(d, agente+" fallo (", ")")
			return c, evs, err
		}},
		{"status de la task_notification que cierra con un fallo una delegacion normal", func(t *testing.T, texto string) (string, []Event, error) {
			const agente, id = "hoom-scout", "toolu_hu8_normal"
			n := coNorm(t)
			evs := n(coToolUse(id, "Agent", agente, false))
			ult := n(hu8Notif(t, id, texto))
			evs = append(evs, ult...)
			d, err := hu8UnEnd(ult, agente, id)
			if err != nil {
				return "", evs, err
			}
			c, err := hu8Envuelto(d, agente+" fallo (", ")")
			return c, evs, err
		}},
		{"status de la task_notification real de Claude Code 2.1.281", func(t *testing.T, texto string) (string, []Event, error) {
			n := coNorm(t)
			evs := n(hcToolUseRealSegundoPlano(t))
			evs = append(evs, n(coRealTaskStarted)...)
			evs = append(evs, n(coRealInterna)...)
			ult := n(hu8NotifReal(t, texto))
			evs = append(evs, ult...)
			d, err := hu8UnEnd(ult, "general-purpose", coRealID)
			if err != nil {
				return "", evs, err
			}
			c, err := hu8Envuelto(d, "general-purpose fallo (", ")")
			return c, evs, err
		}},
		{"texto de un tool_result con is_error", func(t *testing.T, texto string) (string, []Event, error) {
			const agente, id = "hoom-writer", "toolu_hu8_result"
			n := coNorm(t)
			evs := n(coToolUse(id, "Agent", agente, false))
			ult := n(coToolResult(id, true, hu8JSON(t, texto)))
			evs = append(evs, ult...)
			d, err := hu8UnEnd(ult, agente, id)
			if err != nil {
				return "", evs, err
			}
			c, err := hu8Envuelto(d, agente+" fallo: ", "")
			return c, evs, err
		}},
		{"texto de un tool_result con is_error en un bloque text", func(t *testing.T, texto string) (string, []Event, error) {
			const agente, id = "hoom-writer", "toolu_hu8_bloque"
			n := coNorm(t)
			evs := n(coToolUse(id, "Agent", agente, false))
			ult := n(coToolResult(id, true, `[{"type":"text","text":`+hu8JSON(t, texto)+`}]`))
			evs = append(evs, ult...)
			d, err := hu8UnEnd(ult, agente, id)
			if err != nil {
				return "", evs, err
			}
			c, err := hu8Envuelto(d, agente+" fallo: ", "")
			return c, evs, err
		}},
		{"texto de un evento text (normalizador del run)", func(t *testing.T, texto string) (string, []Event, error) {
			evs := coNorm(t)(hu8LineaTexto(t, texto))
			d, err := hu8Uno(evs, "text")
			return d, evs, err
		}},
		{"texto de un evento text (Normalize sin memoria)", func(t *testing.T, texto string) (string, []Event, error) {
			p, err := Lookup("claude")
			if err != nil {
				t.Fatal(err)
			}
			evs := p.Normalize(hu8LineaTexto(t, texto))
			d, err := hu8Uno(evs, "text")
			return d, evs, err
		}},
		{"descripcion de una delegacion (evento agent)", func(t *testing.T, texto string) (string, []Event, error) {
			evs := coNorm(t)(hu8ToolUse(t, "toolu_hu8_desc", "hoom-scout", texto))
			d, err := hu8Uno(evs, "agent")
			if err != nil {
				return "", evs, err
			}
			c, err := hu8Envuelto(d, "Agent: ", "")
			return c, evs, err
		}},
	}
}

// hu8Marca es la marca de recorte del canal: lo que sigue a los 300 bytes de
// un texto ASCII de 301 (en ASCII cada byte es un borde de caracter, asi que
// el recorte conserva exactamente 300). Una marca no vacia, de pocos bytes,
// en UTF-8 valido.
func hu8Marca(t *testing.T, ca string, c hu8Canal) string {
	t.Helper()
	orig := hu8Rep("x", hu8Limite+1)
	cuerpo, _, err := c.emitir(t, orig)
	if err != nil {
		t.Fatalf("%s: %s: fixture, texto ASCII de 301 bytes: %v", ca, c.nombre, err)
	}
	marca := strings.TrimPrefix(cuerpo, hu8Rep("x", hu8Limite))
	if !strings.HasPrefix(cuerpo, hu8Rep("x", hu8Limite)) || marca == "" || strings.HasPrefix(marca, "x") ||
		len(marca) > hu8MarcaMax || !utf8.ValidString(marca) {
		t.Fatalf("%s: %s: fixture: un texto ASCII de 301 bytes sale como sus primeros 300 bytes mas una marca de recorte de 1 a %d bytes en UTF-8 valido: %s",
			ca, c.nombre, hu8MarcaMax, hu8Resumen(cuerpo))
	}
	return marca
}

// CA-373 (enmienda 6) y fuente telemetria (hallazgos aa41d6, d60369 y
// f94377): para textos de caracteres de 2, 3 y 4 bytes con el byte 300 en
// cada posicion posible de un caracter, el status de una task_notification
// fallida (delegacion en segundo plano, normal y la linea real de 2.1.281),
// el texto de un tool_result con is_error (string y bloque text), el texto de
// un evento text (con y sin memoria) y la descripcion de una delegacion salen
// recortados SIN partir un caracter: UTF-8 valido, sin U+FFFD, un prefijo del
// original de 297 a 300 bytes mas la marca de recorte.
func TestHallazgos_aa41d6_d60369_f94377_CA373_RecorteSinPartirCaracter(t *testing.T) {
	const ca = "CA-373 (enmienda 6; hallazgos aa41d6, d60369, f94377)"
	for _, c := range hu8Canales() {
		t.Run(c.nombre, func(t *testing.T) {
			marca := hu8Marca(t, ca, c)
			fallas := 0
			for _, tx := range hu8Largos() {
				cuerpo, _, err := c.emitir(t, tx.texto)
				if err != nil {
					t.Errorf("%s: %s: %s: %v", ca, c.nombre, tx.nombre, err)
					continue
				}
				if mal := hu8Falta(tx.texto, cuerpo, marca); len(mal) > 0 {
					fallas++
					t.Errorf("%s: %s: %s: el texto sale recortado a 300 bytes sin partir un caracter: %s; salio %s",
						ca, c.nombre, tx.nombre, strings.Join(mal, "; "), hu8Resumen(cuerpo))
				}
			}
			if fallas > 0 {
				t.Logf("%s: %s: %d de %d textos salen con un caracter partido o fuera de contrato", ca, c.nombre, fallas, len(hu8Largos()))
			}
		})
	}
}

// CA-373 (enmienda 6; hallazgos aa41d6, d60369, f94377), guarda: un texto de
// hasta 300 bytes exactos (con caracteres de 2, 3 y 4 bytes que terminan
// justo en el byte 300 o antes) sale entero, sin marca, en todos los canales.
func TestHallazgos_aa41d6_d60369_f94377_CA373_HastaTrescientosBytesSaleEntero(t *testing.T) {
	const ca = "CA-373 (enmienda 6; hallazgos aa41d6, d60369, f94377, guarda)"
	for _, c := range hu8Canales() {
		for _, tx := range hu8Cortos() {
			cuerpo, _, err := c.emitir(t, tx.texto)
			if err != nil {
				t.Errorf("%s: %s: %s: %v", ca, c.nombre, tx.nombre, err)
				continue
			}
			if cuerpo != tx.texto {
				t.Errorf("%s: %s: %s: un texto de hasta 300 bytes sale entero: quiere %s, salio %s",
					ca, c.nombre, tx.nombre, hu8Resumen(tx.texto), hu8Resumen(cuerpo))
			}
		}
	}
}

// CA-373 (enmienda 6; hallazgos aa41d6, d60369, f94377), como propiedad: para
// cualquier texto armado con ASCII, comillas, barras, <, & y caracteres de 2,
// 3 y 4 bytes, de unos pocos bytes a unos KiB y con frecuencia alrededor del
// byte 300, cada canal deja el texto entero si mide hasta 300 bytes y, si no,
// un prefijo del original de 297 a 300 bytes en UTF-8 valido mas la marca.
func TestHallazgos_aa41d6_d60369_f94377_CA373_PropiedadRecorteEnUTF8Valido(t *testing.T) {
	const ca = "CA-373 (enmienda 6; hallazgos aa41d6, d60369, f94377, propiedad)"
	canales := hu8Canales()
	marcas := make([]string, len(canales))
	for i, c := range canales {
		marcas[i] = hu8Marca(t, ca, c)
	}
	alfabeto := []string{"a", "ñ", "漢", "€", "🙂", `"`, `\`, "<", "&", "é"}
	prop := func(relleno uint16, bs []uint8) bool {
		var b strings.Builder
		b.WriteString(hu8Rep("a", int(relleno)%(hu8Limite+1)))
		for _, v := range bs {
			b.WriteString(hu8Rep(alfabeto[int(v)%len(alfabeto)], int(v)/len(alfabeto)%8+1))
		}
		if b.Len() == 0 {
			b.WriteString("a")
		}
		orig := b.String()
		for i, c := range canales {
			cuerpo, _, err := c.emitir(t, orig)
			if err != nil {
				t.Errorf("%s: %s: texto %s: %v", ca, c.nombre, hu8Resumen(orig), err)
				return false
			}
			if mal := hu8Falta(orig, cuerpo, marcas[i]); len(mal) > 0 {
				t.Errorf("%s: %s: texto %s (borde de caracter en %d): %s; salio %s",
					ca, c.nombre, hu8Resumen(orig), hu8Borde(orig), strings.Join(mal, "; "), hu8Resumen(cuerpo))
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		t.Errorf("%s: %v", ca, err)
	}
}

// CA-373 y CA-362 (enmienda 6; hallazgos aa41d6, d60369, f94377): el run
// guarda en memoria lo que escribe, y un detail en UTF-8 invalido no se
// escribe igual (el JSON lo cambia por U+FFFD). Entonces ningun evento que el
// normalizador emite en el camino de cada canal, con textos largos y cortos de
// caracteres de 2, 3 y 4 bytes, sale con un detail o un agente en UTF-8
// invalido o con U+FFFD: tampoco el system que la task_notification sigue
// emitiendo (CA-373), cuyo detail tambien es un texto recortado, ni el text
// de la linea del tool_result.
func TestHallazgos_aa41d6_d60369_f94377_CA373_CA362_NingunEventoDelCaminoSaleConUTF8Invalido(t *testing.T) {
	const ca = "CA-373/CA-362 (enmienda 6; hallazgos aa41d6, d60369, f94377, todos los eventos)"
	textos := append(hu8Largos(), hu8Cortos()...)
	for _, c := range hu8Canales() {
		t.Run(c.nombre, func(t *testing.T) {
			malos := 0
			for _, tx := range textos {
				_, evs, err := c.emitir(t, tx.texto)
				if err != nil {
					t.Errorf("%s: %s: %s: %v", ca, c.nombre, tx.nombre, err)
					continue
				}
				if !hu8DetailsValidos(t, ca, c.nombre+": "+tx.nombre, evs) {
					malos++
				}
			}
			if malos > 0 {
				t.Logf("%s: %s: %d de %d textos dejan algun evento con UTF-8 invalido", ca, c.nombre, malos, len(textos))
			}
		})
	}
}
