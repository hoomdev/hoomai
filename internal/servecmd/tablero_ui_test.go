// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-319..CA-322): la pestaña Tablero de la UI embebida. Son estaticos,
// como los del Studio de hoy: afirman lo que esta y lo que no esta en
// index.html y en tablero.js. La pestaña vive en su propio archivo para que
// "no emite ningun POST" se pueda afirmar leyendo exactamente la pestaña.
package servecmd

import (
	"bytes"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// tbUI lee los dos archivos embebidos de la UI.
func tbUI(t *testing.T) (html, js string) {
	t.Helper()
	raw, err := fs.ReadFile(uiFS, "ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	rawJS, err := fs.ReadFile(uiFS, "ui/tablero.js")
	if err != nil {
		t.Fatalf("CA-319: la pestaña vive en ui/tablero.js, embebido en el binario: %v", err)
	}
	return string(raw), string(rawJS)
}

// CA-319: index.html tiene las pestañas Cockpit y Tablero, define --human y
// carga tablero.js despues de su script; GET /tablero.js lo sirve embebido.
func TestCA319_PestanasYTableroJSEmbebido(t *testing.T) {
	html, js := tbUI(t)
	tbRevisarPestanas(t, html)

	rec := serveGET(t, newProject(t), "/tablero.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-319: GET /tablero.js responde 200, respondio %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("CA-319: /tablero.js se sirve como javascript: %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != js {
		t.Fatal("CA-319: /tablero.js es el archivo embebido")
	}
}

func tbRevisarPestanas(t *testing.T, html string) {
	t.Helper()
	for _, tab := range []string{"Cockpit", "Tablero"} {
		if !regexp.MustCompile(`>[^<]*\b` + tab + `\b[^<]*<`).MatchString(html) {
			t.Fatalf("CA-319: index.html tiene la pestaña %q como texto de un elemento", tab)
		}
	}
	if !strings.Contains(html, "--human:") {
		t.Fatal("CA-319: index.html define la variable --human (el morado de las columnas humanas)")
	}
	const carga = `<script src="tablero.js"></script>`
	i := strings.Index(html, carga)
	if i < 0 {
		t.Fatalf("CA-319: index.html carga %s", carga)
	}
	j := strings.Index(html, "async function j(")
	if j < 0 || j > i || strings.LastIndex(html[:i], "</script>") < j {
		t.Fatal("CA-319: tablero.js se carga DESPUES del script de index.html (que define j, renderStage y appendFeed)")
	}
}

// CA-319: el test de UI sin assets de red (CA-10) pasa sobre los dos
// archivos: ninguna referencia externa en index.html ni en tablero.js.
func TestCA319_SinAssetsDeRed(t *testing.T) {
	html, js := tbUI(t)
	for nombre, src := range map[string]string{"index.html": html, "tablero.js": js} {
		for _, marca := range []string{"http:", "https:", "//cdn", "@import"} {
			if strings.Contains(src, marca) {
				t.Fatalf("CA-319: %s contiene una referencia externa (%q)", nombre, marca)
			}
		}
	}
	var archivos []string
	_ = fs.WalkDir(uiFS, "ui", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			archivos = append(archivos, p)
		}
		return nil
	})
	sort.Strings(archivos)
	if strings.Join(archivos, ",") != "ui/index.html,ui/tablero.js" {
		t.Fatalf("CA-319: la UI embebida son index.html y tablero.js, sin dependencias: %v", archivos)
	}
}

// CA-319: tablero.js pinta desde el JSON del binario (columns, human,
// needs_decision, meter, plain, providers) y reusa renderStage y appendFeed
// del cockpit, que ahora reciben su contenedor, sin definir los suyos.
func TestCA319_TableroPintaDelJSONYReusaElEscenario(t *testing.T) {
	html, js := tbUI(t)
	tbRevisarPintores(t, html, js)
}

func tbRevisarPintores(t *testing.T, html, js string) {
	t.Helper()
	for _, campo := range []string{"columns", "human", "needs_decision", "meter", "plain", "providers"} {
		if !regexp.MustCompile(`\b` + campo + `\b`).MatchString(js) {
			t.Fatalf("CA-319: tablero.js usa el campo %q del JSON", campo)
		}
	}
	for _, firma := range []string{"function renderStage(sv, el)", "function appendFeed(evs, el)"} {
		if !strings.Contains(html, firma) {
			t.Fatalf("CA-319: index.html declara %q: el pintor recibe su contenedor", firma)
		}
	}
	for _, vieja := range []string{"function renderStage(sv)", "function appendFeed(evs)"} {
		if strings.Contains(html, vieja) {
			t.Fatalf("CA-319: la firma vieja %q ya no existe", vieja)
		}
	}
	for _, uso := range []string{"renderStage(", "appendFeed("} {
		if !strings.Contains(js, uso) {
			t.Fatalf("CA-319: el panel En vivo de tablero.js reusa %s", uso)
		}
	}
	for _, propia := range []string{"function renderStage", "function appendFeed", "renderStage =", "appendFeed =", "renderStage=", "appendFeed="} {
		if strings.Contains(js, propia) {
			t.Fatalf("CA-319: tablero.js no define su propio pintor (%q)", propia)
		}
	}
}

// CA-320: la pestaña Tablero no emite ningun POST ni arrastra: tablero.js no
// llama a act(, no nombra POST ni method, no usa fetch directo, ni
// XMLHttpRequest, ni sendBeacon, ni el token, ni drag and drop, ni forms.
func TestCA320_TableroNoMuta(t *testing.T) {
	html, js := tbUI(t)
	tbRevisarNoMuta(t, html, js)
}

func tbRevisarNoMuta(t *testing.T, html, js string) {
	t.Helper()
	for _, prohibido := range []string{"POST", "method", "fetch(", "XMLHttpRequest", "sendBeacon", "X-Hoom-Token",
		"draggable", "dragstart", "ondrop", "<form", "hoom-token", `$("token")`} {
		if strings.Contains(js, prohibido) {
			t.Fatalf("CA-320: tablero.js no contiene %q", prohibido)
		}
	}
	for _, re := range []string{`\bact\(`, `\bdrop\b`} {
		if m := regexp.MustCompile(re).FindString(js); m != "" {
			t.Fatalf("CA-320: tablero.js no contiene %q", m)
		}
	}

	// j de index.html: un GET, fetch sin opciones
	i := strings.Index(html, "async function j(url) {")
	if i < 0 {
		t.Fatal("CA-320: index.html define async function j(url)")
	}
	fin := strings.Index(html[i:], "\n}")
	if fin < 0 {
		t.Fatal("CA-320: no encontre el cierre de j")
	}
	cuerpo := html[i : i+fin]
	if !strings.Contains(cuerpo, "fetch(url)") || strings.Contains(cuerpo, "fetch(url,") || strings.Contains(cuerpo, "method") {
		t.Fatalf("CA-320: j llama a fetch(url) sin opciones:\n%s", cuerpo)
	}
	if !regexp.MustCompile(`\bj\(`).MatchString(js) {
		t.Fatal("CA-320: tablero.js lee con j()")
	}
}

// CA-320: cada literal /api/ de tablero.js es /api/board, /api/board/ o
// /api/runs/, y un GET a cada uno no es 405. El item "una" no tiene spec:
// backlog por CA-269.
func TestCA320_TableroSoloLeeSusURLs(t *testing.T) {
	_, js := tbUI(t)
	vistas := tbRevisarURLs(t, js)

	dir := tbProyecto(t)
	tbItem(t, dir, "una", "")
	s := newServer(t, dir)
	for lit := range vistas {
		path := lit
		switch lit {
		case "/api/board/":
			path += "una"
		case "/api/runs/":
			path += "x"
		}
		if rec := tbGET(t, s, path); rec.Code == http.StatusMethodNotAllowed {
			t.Fatalf("CA-320: GET %s no es 405", path)
		}
	}
	if rec := tbGET(t, s, "/api/board"); rec.Code != http.StatusOK {
		t.Fatalf("CA-320: GET /api/board responde 200: %d", rec.Code)
	}
	if rec := tbGET(t, s, "/api/board/una"); rec.Code != http.StatusOK {
		t.Fatalf("CA-320: GET /api/board/una responde 200: %d", rec.Code)
	}
}

func tbRevisarURLs(t *testing.T, js string) map[string]bool {
	t.Helper()
	permitidas := map[string]bool{"/api/board": true, "/api/board/": true, "/api/runs/": true}
	vistas := map[string]bool{}
	for _, lit := range regexp.MustCompile(`/api/[a-z/]*`).FindAllString(js, -1) {
		if !permitidas[lit] {
			t.Fatalf("CA-320: tablero.js solo lee /api/board, /api/board/{slug} y /api/runs/{id}: aparece %q", lit)
		}
		vistas[lit] = true
	}
	for lit := range permitidas {
		if !vistas[lit] {
			t.Fatalf("CA-320: tablero.js lee %s (el tablero, el detalle y el panel En vivo)", lit)
		}
	}
	return vistas
}

// CA-321: el modo se guarda en localStorage con la clave hoom-tablero-modo y
// los valores normal y experto, siempre dentro de try/catch; el vocabulario
// del modo normal vive en un bloque y no habla de git; el filtro se llama
// "Necesitan tu decisión" y filtra por needs_decision.
func TestCA321_ModoVocabularioYFiltro(t *testing.T) {
	html, js := tbUI(t)
	tbRevisarModo(t, html, js)
}

func tbRevisarModo(t *testing.T, html, js string) {
	t.Helper()
	for _, quiero := range []string{"hoom-tablero-modo", "localStorage", "needs_decision", "Necesitan tu decisión"} {
		if !strings.Contains(js, quiero) {
			t.Fatalf("CA-321: tablero.js contiene %q", quiero)
		}
	}
	for _, valor := range []string{"normal", "experto"} {
		if !strings.Contains(js, `"`+valor+`"`) && !strings.Contains(js, `'`+valor+`'`) {
			t.Fatalf("CA-321: el modo toma el valor %q", valor)
		}
	}
	todo := html + js
	for _, etiqueta := range []string{"Normal", "Experto"} {
		if !strings.Contains(todo, etiqueta) {
			t.Fatalf("CA-321: el interruptor dice %q", etiqueta)
		}
	}
	// cada uso de localStorage esta dentro de un try con su catch
	ultimo := func(re *regexp.Regexp, s string) int {
		pos := -1
		for _, m := range re.FindAllStringIndex(s, -1) {
			pos = m[0]
		}
		return pos
	}
	reTry := regexp.MustCompile(`\btry\s*\{`)
	reCatch := regexp.MustCompile(`(^|[^.\w])catch\s*[({]`)
	for idx := 0; ; {
		k := strings.Index(js[idx:], "localStorage")
		if k < 0 {
			break
		}
		p := idx + k
		try := ultimo(reTry, js[:p])
		catch := ultimo(reCatch, js[:p])
		if try < 0 || try < catch || p-try > 600 || !reCatch.MatchString(js[p:]) {
			t.Fatalf("CA-321: localStorage se usa dentro de try/catch (ventana privada): ...%s...", js[max(0, p-80):min(len(js), p+40)])
		}
		idx = p + len("localStorage")
	}

	const ini, fin = "/* vocabulario normal */", "/* fin vocabulario normal */"
	a, b := strings.Index(js, ini), strings.Index(js, fin)
	if a < 0 || b < 0 || b < a {
		t.Fatalf("CA-321: el vocabulario normal vive entre %q y %q", ini, fin)
	}
	vocab := js[a+len(ini) : b]
	for _, quiero := range []string{"espacio de trabajo", "integrar", "guardar"} {
		if !strings.Contains(vocab, quiero) {
			t.Fatalf("CA-321: el vocabulario normal dice %q", quiero)
		}
	}
	if w := regexp.MustCompile(`(?i)\b(git|commit|worktree|merge|rama|branch|diff|head|huella)\b`).FindString(vocab); w != "" {
		t.Fatalf("CA-321: el vocabulario normal no dice %q", w)
	}
}

// CA-322: el sondeo es un setTimeout de 1000 ms encadenado (nunca
// setInterval) y solo con la pagina visible; las etiquetas van con sus
// acentos.
func TestCA322_SondeoYAcentos(t *testing.T) {
	html, js := tbUI(t)
	tbRevisarSondeo(t, html, js)
}

func tbRevisarSondeo(t *testing.T, html, js string) {
	t.Helper()
	if !regexp.MustCompile(`setTimeout\([\s\S]{1,300}?,\s*1000\s*\)`).MatchString(js) {
		t.Fatal("CA-322: tablero.js sondea con setTimeout(..., 1000)")
	}
	if strings.Contains(js, "setInterval") {
		t.Fatal("CA-322: tablero.js no usa setInterval: dos pedidos nunca se pisan")
	}
	if !strings.Contains(js, "visibilityState") {
		t.Fatal("CA-322: el sondeo mira document.visibilityState")
	}
	todo := html + js
	for _, etiqueta := range []string{"Qué", "Quién", "Pruebas", "Tu aprobación", "Tu aceptación", "Necesitan tu decisión"} {
		if !strings.Contains(todo, etiqueta) {
			t.Fatalf("CA-322: la UI tiene la etiqueta %q con sus acentos", etiqueta)
		}
	}
	// el ciclo de 5 s del Studio no cambia
	if !bytes.Contains([]byte(html), []byte("setInterval(tick, 5000)")) {
		t.Fatal("CA-322: el ciclo de 5 s del Studio sigue igual")
	}
}
