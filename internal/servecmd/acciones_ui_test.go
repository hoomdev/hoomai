// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-353..CA-355): la UI de las acciones de la tarjeta. Son estaticos, como
// los de C2: afirman lo que esta y lo que no esta en tablero.js (el pintor
// sin POST de C2, que ahora envuelve la tarjeta, pinta el fantasma y el
// espejo de la terminal) y en acciones.js (todo POST de la cabina, el
// arrastre y los dialogos). Que se vean y funcionen se comprueba a mano.
package servecmd

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// atUI lee los tres archivos embebidos de la UI.
func atUI(t *testing.T) (html, tablero, acciones string) {
	t.Helper()
	leer := func(p string) string {
		raw, err := fs.ReadFile(uiFS, p)
		if err != nil {
			t.Fatalf("CA-353: %s esta embebido en el binario: %v", p, err)
		}
		return string(raw)
	}
	return leer("ui/index.html"), leer("ui/tablero.js"), leer("ui/acciones.js")
}

// Los ids de columna que la pagina nunca decide.
var atIDsDeColumna = []string{"backlog", "arquitecto", "tu-aprobacion", "test-writer", "writer", "review", "tu-aceptacion", "hecho"}

var atPalabrasDeGit = regexp.MustCompile(`(?i)\b(git|commit|worktree|merge|rama|branch|diff|head|huella)\b`)

// Una declaracion de funcion con nombre: function f(, async function f(, o
// const f = (...) => / const f = async x =>.
var atDeclaracion = regexp.MustCompile(`(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(|(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>`)

// atFuncionQueEncierra es el nombre de la ultima funcion declarada antes de
// pos ("" si no hay ninguna).
func atFuncionQueEncierra(js string, pos int) string {
	decl := atDeclaracion.FindAllStringSubmatchIndex(js[:pos], -1)
	if len(decl) == 0 {
		return ""
	}
	u := decl[len(decl)-1]
	if u[2] >= 0 {
		return js[u[2]:u[3]]
	}
	return js[u[4]:u[5]]
}

// atEncadenada: la funcion se vuelve a programar con setTimeout(..., 1000)
// desde su propio cuerpo (el que va hasta la declaracion siguiente).
func atEncadenada(js, nombre string) bool {
	for _, d := range atDeclaracion.FindAllStringSubmatchIndex(js, -1) {
		n := ""
		if d[2] >= 0 {
			n = js[d[2]:d[3]]
		} else {
			n = js[d[4]:d[5]]
		}
		if n != nombre {
			continue
		}
		fin := len(js)
		if sig := atDeclaracion.FindStringIndex(js[d[1]:]); sig != nil {
			fin = d[1] + sig[0]
		}
		cuerpo := js[d[1]:fin]
		if regexp.MustCompile(`setTimeout\(\s*(?:\([^)]*\)\s*=>\s*)?` + regexp.QuoteMeta(nombre) + `\b[\s\S]{0,200}?,\s*1000\s*\)`).MatchString(cuerpo) {
			return true
		}
	}
	return false
}

// CA-353: tablero.js sigue cumpliendo CA-320 (sin POST, sin token, sin
// arrastre, solo lee /api/board, /api/board/{slug} y /api/runs/{id}), pinta
// cada tarjeta dentro de un .tcard-wrap con data-slug y un lugar .tacc, pinta
// el fantasma desde ghost, y avisa con document.dispatchEvent(new
// Event("tablero:pintado")) cada vez que pinta.
func TestCA353_TableroEnvuelveLaTarjetaYPintaElFantasma(t *testing.T) {
	html, js, _ := atUI(t)
	tbRevisarNoMuta(t, html, js)
	tbRevisarURLs(t, js)

	// el contenedor de la tarjeta, con su slug, y el lugar de las acciones:
	// la etiqueta que abre el .tcard-wrap lleva data-slug (o se lo pone
	// dataset.slug, si se arma con el DOM)
	conSlug := false
	for idx := 0; ; {
		k := strings.Index(js[idx:], "tcard-wrap")
		if k < 0 {
			break
		}
		p := idx + k
		ini := strings.LastIndex(js[:p], "<")
		fin := strings.Index(js[p:], ">")
		if ini >= 0 && fin >= 0 && !strings.Contains(js[ini:p], ">") && strings.Contains(js[ini:p+fin], "data-slug=") {
			conSlug = true
		}
		idx = p + len("tcard-wrap")
	}
	if !conSlug && !(strings.Contains(js, "tcard-wrap") && strings.Contains(js, "dataset.slug")) {
		t.Fatal("CA-353: tablero.js pinta cada tarjeta dentro de un .tcard-wrap con data-slug")
	}
	if !regexp.MustCompile(`\btacc\b`).MatchString(js) {
		t.Fatal("CA-353: tablero.js deja en cada .tcard-wrap un lugar vacio .tacc para las acciones")
	}

	// el fantasma sale de ghost, que calcula el binario
	if !regexp.MustCompile(`\bghost\b`).MatchString(js) {
		t.Fatal("CA-353: tablero.js pinta el fantasma desde el campo ghost de la tarjeta")
	}

	// el aviso a acciones.js
	if !regexp.MustCompile(`document\.dispatchEvent\(\s*new Event\(\s*["']tablero:pintado["']\s*\)\s*\)`).MatchString(js) {
		t.Fatal(`CA-353: tablero.js avisa que pinto con document.dispatchEvent(new Event("tablero:pintado"))`)
	}
}

// CA-353: la seccion Terminal pide /api/board/{slug}/terminal con un
// setTimeout(..., 1000) encadenado (la funcion que pide se vuelve a llamar a
// si misma, nunca setInterval) y es solo de lectura: ni tablero.js ni el
// panel Quien de index.html tienen <input o <textarea, y nada en la UI manda
// teclas (send-keys).
func TestCA353_TerminalSondeaYEsDeSoloLectura(t *testing.T) {
	html, js, acc := atUI(t)
	i := strings.Index(js, "/terminal")
	if i < 0 {
		t.Fatal("CA-353: tablero.js pide /api/board/{slug}/terminal")
	}
	if !regexp.MustCompile("/api/board/[^\\n]{0,80}/terminal").MatchString(js) {
		t.Fatal("CA-353: la terminal se pide a /api/board/{slug}/terminal")
	}
	// la funcion que pide /terminal (o, si el pedido vive en un ayudante, la
	// que llama al ayudante) se vuelve a programar a si misma con
	// setTimeout(..., 1000)
	nombre := atFuncionQueEncierra(js, i)
	if nombre == "" {
		t.Fatal("CA-353: el pedido de /terminal vive en una funcion con nombre de tablero.js")
	}
	encadenada := atEncadenada(js, nombre)
	if !encadenada {
		llamada := regexp.MustCompile(`\b` + regexp.QuoteMeta(nombre) + `\(`)
		esDeclaracion := regexp.MustCompile(`function\s*$`)
		for _, m := range llamada.FindAllStringIndex(js, -1) {
			if esDeclaracion.MatchString(js[:m[0]]) {
				continue // la declaracion del ayudante no es una llamada
			}
			if quien := atFuncionQueEncierra(js, m[0]); quien != "" && quien != nombre && atEncadenada(js, quien) {
				encadenada = true
			}
		}
	}
	if !encadenada {
		t.Fatalf("CA-353: %s, que pide /terminal, se vuelve a llamar con setTimeout(..., 1000) despues de cada respuesta (sondeo encadenado de C2)", nombre)
	}
	if strings.Contains(js, "setInterval") {
		t.Fatal("CA-353: el sondeo de la terminal no usa setInterval")
	}

	for _, prohibido := range []string{"<input", "<textarea", "send-keys", "contenteditable"} {
		if strings.Contains(js, prohibido) {
			t.Fatalf("CA-353: la seccion Terminal es de solo lectura: tablero.js no contiene %q", prohibido)
		}
	}
	a := strings.Index(html, `id="tb-p-quien"`)
	b := strings.Index(html, `id="tb-p-pruebas"`)
	if a < 0 || b < a {
		t.Fatal("CA-353: no encontre el panel Quien del detalle en index.html")
	}
	for _, prohibido := range []string{"<input", "<textarea", "contenteditable"} {
		if strings.Contains(html[a:b], prohibido) {
			t.Fatalf("CA-353: la seccion Terminal (panel Quien) no tiene %q", prohibido)
		}
	}
	for nombre, src := range map[string]string{"index.html": html, "tablero.js": js, "acciones.js": acc} {
		if strings.Contains(src, "send-keys") || strings.Contains(src, "sendKeys") {
			t.Fatalf("CA-353: %s no manda teclas a la terminal (send-keys)", nombre)
		}
	}
}

// CA-354: acciones.js hace cada POST con act( a /launch, /approve (el de
// siempre, con {card}), /done, /save, /discard y /session; usa actions,
// enabled, why, drops, draggable, dragstart y drop; se engancha a
// tablero:pintado y a los .tcard-wrap / .tacc que pinta tablero.js; no
// contiene setInterval, fetch(, XMLHttpRequest, sendBeacon ni ningun id de
// columna como cadena; su dialogo de rol tiene provider, modelo, presupuesto
// y pedido y dice "no tiene tope"; y las acciones con expert solo se
// muestran en modo experto.
func TestCA354_AccionesJSActuaConActYNoDecide(t *testing.T) {
	_, _, acc := atUI(t)
	if !regexp.MustCompile(`\bact\(`).MatchString(acc) {
		t.Fatal("CA-354: acciones.js hace sus POST con act( (el token de siempre)")
	}
	for _, sufijo := range []string{"/launch", "/approve", "/done", "/save", "/discard", "/session"} {
		if !regexp.MustCompile(regexp.QuoteMeta(sufijo) + `\b`).MatchString(acc) {
			t.Fatalf("CA-354: acciones.js hace el POST a %s", sufijo)
		}
	}
	for _, base := range []string{"/api/board/", "/api/specs/", "/api/tasks/"} {
		if !strings.Contains(acc, base) {
			t.Fatalf("CA-354: acciones.js nombra %s (launch/save/discard/session, approve, done)", base)
		}
	}
	if !regexp.MustCompile(`\{\s*card\b`).MatchString(acc) {
		t.Fatal(`CA-354: aprobar desde la tarjeta manda {card: <slug>} a /api/specs/<slug>/approve`)
	}
	for _, campo := range []string{"actions", "enabled", "why", "drops", "draggable", "dragstart", "drop", "expert"} {
		if !regexp.MustCompile(`\b` + campo + `\b`).MatchString(acc) {
			t.Fatalf("CA-354: acciones.js usa %q", campo)
		}
	}
	for _, gancho := range []string{"tablero:pintado", "tcard-wrap", "tacc"} {
		if !strings.Contains(acc, gancho) {
			t.Fatalf("CA-354: acciones.js se engancha a lo que pinta tablero.js (%q)", gancho)
		}
	}
	for _, prohibido := range []string{"setInterval", "fetch(", "XMLHttpRequest", "sendBeacon"} {
		if strings.Contains(acc, prohibido) {
			t.Fatalf("CA-354: todo POST de acciones.js pasa por act(: no contiene %q", prohibido)
		}
	}
	for _, id := range atIDsDeColumna {
		for _, q := range []string{`"`, `'`, "`"} {
			if strings.Contains(acc, q+id+q) {
				t.Fatalf("CA-354: acciones.js no decide columnas: contiene el id %s como cadena", q+id+q)
			}
		}
	}

	// el dialogo de rol
	bajo := strings.ToLower(acc)
	for _, campo := range []string{"provider", "modelo", "presupuesto", "pedido"} {
		if !strings.Contains(bajo, campo) {
			t.Fatalf("CA-354: el dialogo de rol tiene %s", campo)
		}
	}
	if !strings.Contains(acc, "no tiene tope") {
		t.Fatal(`CA-354: con un provider sin budget el dialogo dice que el trabajo "no tiene tope" en USD`)
	}

	// las acciones de experto, solo en modo experto
	if !regexp.MustCompile(`\bexperto\b`).MatchString(acc) {
		t.Fatal("CA-354: acciones.js mira expert de cada accion y el modo experto antes de mostrarla")
	}
}

// CA-355: acciones.js tiene su bloque /* vocabulario normal */ ... /* fin
// vocabulario normal */, que contiene "Guardar" e "Integrar" y no habla de
// git; las etiquetas "Abrir sesión", "Aprobar spec", "Volver a lanzar" y
// "Guardar en git" estan con sus acentos, y "Guardar en git" solo fuera del
// bloque normal.
func TestCA355_VocabularioYEtiquetasDeLasAcciones(t *testing.T) {
	_, _, acc := atUI(t)
	const ini, fin = "/* vocabulario normal */", "/* fin vocabulario normal */"
	a, b := strings.Index(acc, ini), strings.Index(acc, fin)
	if a < 0 || b < 0 || b < a {
		t.Fatalf("CA-355: el vocabulario normal de acciones.js vive entre %q y %q", ini, fin)
	}
	if strings.Count(acc, ini) != 1 || strings.Count(acc, fin) != 1 {
		t.Fatal("CA-355: acciones.js tiene UN bloque de vocabulario normal")
	}
	vocab := acc[a+len(ini) : b]
	for _, quiero := range []string{"Guardar", "Integrar"} {
		if !strings.Contains(vocab, quiero) {
			t.Fatalf("CA-355: el vocabulario normal dice %q", quiero)
		}
	}
	if w := atPalabrasDeGit.FindString(vocab); w != "" {
		t.Fatalf("CA-355: el vocabulario normal no dice %q", w)
	}
	for _, etiqueta := range []string{"Abrir sesión", "Aprobar spec", "Volver a lanzar", "Guardar en git"} {
		if !strings.Contains(acc, etiqueta) {
			t.Fatalf("CA-355: acciones.js tiene la etiqueta %q con sus acentos", etiqueta)
		}
	}
	if strings.Contains(vocab, "Guardar en git") {
		t.Fatal(`CA-355: "Guardar en git" es del modo experto: no esta en el bloque normal`)
	}
	fuera := acc[:a] + acc[b+len(fin):]
	if !strings.Contains(fuera, "Guardar en git") {
		t.Fatal(`CA-355: "Guardar en git" esta fuera del bloque normal`)
	}
}
