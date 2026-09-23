// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-366, CA-371 y CA-377): la UI de la historia, del doctor y del
// trinquete. Son estaticos, como los de C2 y C3: afirman lo que esta y lo que
// no esta en index.html y en tablero.js. Que el replay, las insignias y la
// curva se vean bien se comprueba a mano con hoom serve antes del PR.
package servecmd

import (
	"regexp"
	"strings"
	"testing"
)

// huPalabra: la palabra entera, como identificador de JS o campo del JSON.
func huPalabra(src, w string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(w) + `\b`).MatchString(src)
}

// huCuerpos devuelve el cuerpo de cada funcion con nombre de js: desde su
// declaracion hasta la declaracion siguiente (la regla de atEncadenada).
func huCuerpos(js string) map[string]string {
	out := map[string]string{}
	decl := atDeclaracion.FindAllStringSubmatchIndex(js, -1)
	for i, d := range decl {
		n := ""
		if d[2] >= 0 {
			n = js[d[2]:d[3]]
		} else {
			n = js[d[4]:d[5]]
		}
		fin := len(js)
		if i+1 < len(decl) {
			fin = decl[i+1][0]
		}
		out[n] += js[d[1]:fin]
	}
	return out
}

// huUsa: la funcion nombre menciona campo en su cuerpo, o llama a una
// funcion de js que lo menciona (un nivel de ayudante).
func huUsa(js, nombre, campo string) bool {
	cuerpos := huCuerpos(js)
	cuerpo, ok := cuerpos[nombre]
	if !ok {
		return false
	}
	if huPalabra(cuerpo, campo) {
		return true
	}
	for _, m := range regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\(`).FindAllStringSubmatch(cuerpo, -1) {
		if otro, ok := cuerpos[m[1]]; ok && m[1] != nombre && huPalabra(otro, campo) {
			return true
		}
	}
	return false
}

// huFueraDeComentarios son las posiciones de aguja en js que no caen en un
// comentario: ni dentro de un bloque /* ... */ ni despues de un // de su
// linea que no esta dentro de una cadena (comillas pares antes del //).
func huFueraDeComentarios(js, aguja string) []int {
	var out []int
	for idx := 0; ; {
		k := strings.Index(js[idx:], aguja)
		if k < 0 {
			return out
		}
		p := idx + k
		enBloque := strings.LastIndex(js[:p], "/*") > strings.LastIndex(js[:p], "*/")
		linea := js[strings.LastIndex(js[:p], "\n")+1 : p]
		enLinea := false
		for i := strings.Index(linea, "//"); i >= 0 && !enLinea; {
			antes := linea[:i]
			if strings.Count(antes, `"`)%2 == 0 && strings.Count(antes, "'")%2 == 0 && strings.Count(antes, "`")%2 == 0 {
				enLinea = true
			}
			sig := strings.Index(linea[i+2:], "//")
			if sig < 0 {
				break
			}
			i += 2 + sig
		}
		if !enBloque && !enLinea {
			out = append(out, p)
		}
		idx = p + len(aguja)
	}
}

// huRegion es el marcado del elemento <div id="id"> hasta su </div>,
// contando los div anidados.
func huRegion(t *testing.T, html, id string) string {
	t.Helper()
	ini := strings.Index(html, `<div id="`+id+`"`)
	if ini < 0 {
		t.Fatalf("no encontre <div id=%q> en index.html", id)
	}
	re := regexp.MustCompile(`<div\b|</div>`)
	prof := 0
	for _, m := range re.FindAllStringIndex(html[ini:], -1) {
		if html[ini+m[0]:ini+m[1]] == "</div>" {
			prof--
		} else {
			prof++
		}
		if prof == 0 {
			return html[ini : ini+m[1]]
		}
	}
	t.Fatalf("el <div id=%q> de index.html no cierra", id)
	return ""
}

// ---------------------------------------------------------------------------

// CA-366: el detalle de la tarjeta gana una cuarta pestaña, Historia,
// despues de Pruebas (data-pane="historia"), y su panel tb-p-historia en el
// cuerpo del detalle.
func TestCA366_PestanaHistoriaEnElDetalle(t *testing.T) {
	html, _ := tbUI(t)
	a := strings.Index(html, `id="tb-dtabs"`)
	b := strings.Index(html, `id="tb-dbody"`)
	if a < 0 || b < a {
		t.Fatal("CA-366: no encontre las pestañas del detalle (tb-dtabs) ni su cuerpo (tb-dbody) en index.html")
	}
	pestanas := html[a:b]
	if n := strings.Count(pestanas, `data-pane="`); n != 4 {
		t.Fatalf("CA-366: el detalle tiene cuatro pestañas (Qué, Quién, Pruebas, Historia), tiene %d", n)
	}
	p, h := strings.Index(pestanas, `data-pane="pruebas"`), strings.Index(pestanas, `data-pane="historia"`)
	if h < 0 {
		t.Fatal(`CA-366: el detalle tiene la pestaña data-pane="historia"`)
	}
	if p < 0 || h < p {
		t.Fatal("CA-366: la pestaña Historia va despues de Pruebas")
	}
	if !regexp.MustCompile(`data-pane="historia"[^>]*>\s*Historia\s*<`).MatchString(pestanas) {
		t.Fatal(`CA-366: la pestaña data-pane="historia" dice "Historia"`)
	}

	fin := strings.Index(html[b:], "</aside>")
	if fin < 0 {
		t.Fatal("CA-366: no encontre el cierre del detalle (</aside>)")
	}
	cuerpo := html[b : b+fin]
	if strings.Count(html, `id="tb-p-historia"`) != 1 {
		t.Fatal(`CA-366: index.html tiene un solo panel id="tb-p-historia"`)
	}
	i := strings.Index(cuerpo, `id="tb-p-historia"`)
	if i < 0 {
		t.Fatal("CA-366: el panel tb-p-historia esta en el cuerpo del detalle (tb-dbody)")
	}
	tag := ""
	if ini := strings.LastIndex(cuerpo[:i], "<"); ini >= 0 {
		if fin := strings.Index(cuerpo[ini:], ">"); fin >= 0 {
			tag = cuerpo[ini : ini+fin+1]
		}
	}
	if !strings.HasPrefix(tag, "<div") || !regexp.MustCompile(`class="[^"]*\bdpane\b`).MatchString(tag) {
		t.Fatalf("CA-366: el panel de la historia es un div.dpane como los otros tres: %s", tag)
	}
}

// CA-366: tablero.js pide /api/board/ + slug + /timeline (una vez al abrir la
// pestaña: esa funcion no se reprograma con setTimeout(..., 1000)); pinta
// desde entries, meter, source, plain, summary, piloto, notes y card; rotula
// las fuentes "guardado" y "esta computadora" (normal) y "git" y "telemetría
// local" (experto); tiene Actualizar, Reproducir, Pausa, Reiniciar, "Así
// está hoy", "sin dato" y "Todavía no hay historia para esta tarjeta.";
// espera 800 ms (divididos por la velocidad) con setTimeout y nunca
// setInterval; y sigue cumpliendo CA-320 (sin POST, solo lee /api/board,
// /api/board/{slug} y /api/runs/{id}) y CA-322 (sondeo y acentos).
func TestCA366_TableroPideLaHistoriaYLaReproduce(t *testing.T) {
	html, js := tbUI(t)
	if !regexp.MustCompile(`/api/board/[^\n]{0,80}/timeline`).MatchString(js) {
		t.Fatal("CA-366: tablero.js pide /api/board/ + slug + /timeline")
	}
	pedidos := 0
	for _, i := range huFueraDeComentarios(js, "/timeline") {
		pedidos++
		if nombre := atFuncionQueEncierra(js, i); nombre == "" {
			t.Fatal("CA-366: el pedido de /timeline vive en una funcion con nombre de tablero.js")
		} else if atEncadenada(js, nombre) {
			t.Fatalf("CA-366: la historia no sondea: %s, que pide /timeline, no se reprograma con setTimeout(..., 1000)", nombre)
		}
	}
	if pedidos == 0 {
		t.Fatal("CA-366: tablero.js pide /timeline en su codigo, no solo en un comentario")
	}

	for _, campo := range []string{"entries", "meter", "source", "plain", "summary", "piloto", "notes", "card"} {
		if !huPalabra(js, campo) {
			t.Fatalf("CA-366: tablero.js usa el campo %q de la historia", campo)
		}
	}
	for _, rotulo := range []string{"guardado", "esta computadora", "telemetría local"} {
		if !strings.Contains(js, rotulo) {
			t.Fatalf("CA-366: tablero.js rotula la fuente con %q", rotulo)
		}
	}
	if !regexp.MustCompile("[\"'`]git[\"'`]").MatchString(js) {
		t.Fatal(`CA-366: tablero.js rotula la fuente git con "git" en modo experto`)
	}
	for _, etiqueta := range []string{"Actualizar", "Reproducir", "Pausa", "Reiniciar", "Así está hoy", "sin dato",
		"Todavía no hay historia para esta tarjeta."} {
		if !strings.Contains(js, etiqueta) {
			t.Fatalf("CA-366: tablero.js tiene %q", etiqueta)
		}
	}
	if !strings.Contains(js, "setTimeout(") || !strings.Contains(js, "800") {
		t.Fatal("CA-366: el replay espera 800 ms divididos por la velocidad con setTimeout")
	}
	tbRevisarNoMuta(t, html, js)
	tbRevisarURLs(t, js)
	tbRevisarSondeo(t, html, js) // incluye: nunca setInterval
}

// CA-371: tablero.js pinta en cada tarjeta con doctor no vacio la insignia
// .tdoc con la cantidad; su titulo son los plain en modo normal y what con
// "Accion: <action>" en experto; y la pestaña Qué (el pintor de tb-que) lista
// los problemas de la tarjeta. tablero.js sigue sin POST (CA-320).
func TestCA371_InsigniasDelDoctor(t *testing.T) {
	html, js := tbUI(t)
	if !huPalabra(js, "tdoc") {
		t.Fatal("CA-371: tablero.js pinta la insignia .tdoc")
	}
	for _, campo := range []string{"doctor", "plain", "what", "action"} {
		if !huPalabra(js, campo) {
			t.Fatalf("CA-371: tablero.js usa el campo %q de los problemas", campo)
		}
	}
	if !strings.Contains(js, "Accion") {
		t.Fatal(`CA-371: el titulo de la insignia en modo experto dice what y "Accion: <action>"`)
	}
	// la insignia lleva titulo: en su etiqueta (title=) o por el DOM (.title)
	conTitulo := strings.Contains(js, ".title")
	for idx := 0; ; {
		k := strings.Index(js[idx:], "tdoc")
		if k < 0 {
			break
		}
		p := idx + k
		ini := strings.LastIndex(js[:p], "<")
		fin := strings.Index(js[p:], ">")
		if ini >= 0 && fin >= 0 && !strings.Contains(js[ini:p], ">") && strings.Contains(js[ini:p+fin], "title=") {
			conTitulo = true
		}
		idx = p + len("tdoc")
	}
	if !conTitulo {
		t.Fatal("CA-371: la insignia .tdoc lleva titulo (title) con los problemas")
	}

	// la pestaña Qué: lo que se asigna a tb-que (o una funcion que llama)
	// lista los problemas
	m := regexp.MustCompile(`\$\(\s*["']tb-que["']\s*\)\.innerHTML\s*=([^;\n]*)`).FindStringSubmatch(js)
	if m == nil {
		t.Fatal(`CA-371: no encontre el pintor de la pestaña Qué ($("tb-que").innerHTML = ...)`)
	}
	lista := huPalabra(m[1], "doctor")
	for _, llamada := range regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\(`).FindAllStringSubmatch(m[1], -1) {
		lista = lista || huUsa(js, llamada[1], "doctor")
	}
	if !lista {
		t.Fatalf("CA-371: la pestaña Qué (%s) lista los problemas de la tarjeta (doctor)", strings.TrimSpace(m[1]))
	}
	tbRevisarNoMuta(t, html, js)
	tbRevisarURLs(t, js)
}

// CA-377: la pestaña Cockpit (el div #cockpit de index.html) gana la seccion
// "Trinquete", que index.html pinta desde /api/ratchet con j( en el refresco
// de siempre (tick), con un <svg en linea desde history, los loosened
// distinguidos, "sin congelar" para la metrica declarada sin base y "Sin
// línea base" cuando no hay archivo. Sin assets de red (CA-10/CA-319 sigue
// pasando: nada de http:, ni siquiera el namespace de SVG).
func TestCA377_SeccionTrinqueteEnElCockpit(t *testing.T) {
	html, _ := tbUI(t)
	cockpit := huRegion(t, html, "cockpit")
	if !regexp.MustCompile(`>[^<]*\bTrinquete\b[^<]*<`).MatchString(cockpit) {
		t.Fatal(`CA-377: la pestaña Cockpit (#cockpit) tiene la seccion "Trinquete"`)
	}
	loc := regexp.MustCompile("\\bj\\(\\s*[\"'`]/api/ratchet[\"'`]").FindStringIndex(html)
	if loc == nil {
		t.Fatal(`CA-377: index.html pide /api/ratchet con j(`)
	}
	quien := atFuncionQueEncierra(html, loc[0])
	cuerpos := huCuerpos(html)
	if quien == "" || (quien != "tick" && !huPalabra(cuerpos["tick"], quien)) {
		t.Fatalf("CA-377: el trinquete se pide en el refresco de siempre: tick llama a %q", quien)
	}
	if !strings.Contains(html, "<svg") {
		t.Fatal("CA-377: la curva es un <svg en linea")
	}
	for _, w := range []string{"history", "loosened"} {
		if !huPalabra(html, w) {
			t.Fatalf("CA-377: index.html usa %q para la curva", w)
		}
	}
	for _, texto := range []string{"sin congelar", "Sin línea base"} {
		if !strings.Contains(html, texto) {
			t.Fatalf("CA-377: la seccion Trinquete dice %q", texto)
		}
	}
	TestCA319_SinAssetsDeRed(t)
}
