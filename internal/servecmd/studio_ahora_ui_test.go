// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md
// (CA-734..CA-740 y CA-742, la parte de Ahora): la portada Ahora, las
// pestañas, el hash y ui/ahora.js como pintor sin logica. Son estaticos,
// como los de C2, C3 y C4: afirman lo que esta y lo que no esta en
// index.html, tablero.js y ahora.js. Que se vea como el lienzo lo mira
// Henry (Riesgos: "la UI se prueba sin navegador").
//
// Dos prohibiciones del spec se escriben de forma compatible con el resto:
//   - CA-737 "no contiene los ids de columna": tu-aprobacion, tu-aceptacion,
//     test-writer y backlog no aparecen en ahora.js; hecho no aparece como
//     cadena ("hecho", 'hecho', `hecho`), porque la fila de Hecho lee
//     item.hecho_en y el titulo es "Hecho — lo que ya aceptaste".
//   - CA-737 "no lee esperado": "Como se prueba" lo acota a elegir la
//     seccion, y la Tarjeta tiene que decir "FAIL esperado" con red.esperado
//     (CA-741). Cada lectura de esperado tiene que estar para decir FAIL
//     esperado: en la funcion que lo dice o cerca de ese texto.
package servecmd

import (
	"io/fs"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
)

// stLeerUI lee index.html, tablero.js y ahora.js embebidos; ca es el
// criterio que pide leerlos.
func stLeerUI(t *testing.T, ca string) (html, tablero, ahora string) {
	t.Helper()
	html, tablero = tbUI(t)
	raw, err := uiFS.ReadFile("ui/ahora.js")
	if err != nil {
		t.Fatalf("%s: las pantallas Ahora y Tarjeta viven en ui/ahora.js, embebido en el binario: %v", ca, err)
	}
	return html, tablero, string(raw)
}

// stScriptPropio es el codigo de los <script> de index.html que no tienen src.
func stScriptPropio(html string) string {
	var b strings.Builder
	for _, m := range regexp.MustCompile(`(?s)<script\b([^>]*)>(.*?)</script>`).FindAllStringSubmatch(html, -1) {
		if !strings.Contains(m[1], "src=") {
			b.WriteString(m[2])
			b.WriteString("\n")
		}
	}
	return b.String()
}

// stCodigo es js sin comentarios: un texto que solo esta en un comentario no
// es algo que la pagina muestre ni lea.
func stCodigo(js string) string { return c2SinComentarios(js) }

// stTiene exige cada aguja en src.
func stTiene(t *testing.T, ca, donde, src string, agujas ...string) {
	t.Helper()
	for _, a := range agujas {
		if !strings.Contains(src, a) {
			t.Fatalf("%s: %s tiene %q", ca, donde, a)
		}
	}
}

// stUsa exige cada campo del JSON como palabra en el codigo.
func stUsa(t *testing.T, ca, donde, src string, campos ...string) {
	t.Helper()
	for _, c := range campos {
		if !huPalabra(src, c) {
			t.Fatalf("%s: %s lee el campo %q del JSON", ca, donde, c)
		}
	}
}

// stVar exige que index.html o ahora.js usen cada variable CSS.
func stVar(t *testing.T, ca, html, ahora string, vars ...string) {
	t.Helper()
	for _, v := range vars {
		if !regexp.MustCompile(`var\(\s*` + regexp.QuoteMeta(v) + `\s*[,)]`).MatchString(html + ahora) {
			t.Fatalf("%s: la UI usa var(%s)", ca, v)
		}
	}
}

// stLlegaA: algun identificador de texto es meta, o es una funcion de
// cuerpos que llega a meta (hasta niveles de ayudantes).
func stLlegaA(cuerpos map[string]string, texto, meta string, niveles int) bool {
	for _, tok := range regexp.MustCompile(`[A-Za-z_$][\w$]*`).FindAllString(texto, -1) {
		if tok == meta {
			return true
		}
		if c, ok := cuerpos[tok]; ok && niveles > 0 && stLlegaA(cuerpos, c, meta, niveles-1) {
			return true
		}
	}
	return false
}

// stPestanas son los elementos con data-tab de index.html, en orden: id,
// texto y atributos.
type stPestana struct{ id, texto, attrs string }

func stPestanas(html string) []stPestana {
	var out []stPestana
	re := regexp.MustCompile(`<([a-z]+)\b([^>]*)\bdata-tab="([^"]*)"([^>]*)>([^<]*)<`)
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		out = append(out, stPestana{id: m[3], texto: strings.TrimSpace(m[5]), attrs: m[2] + " " + m[4]})
	}
	return out
}

// stRecorrerUI llama a fn con cada archivo embebido bajo ui/.
func stRecorrerUI(fn func(p string, raw []byte)) error {
	return fs.WalkDir(uiFS, "ui", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(uiFS, p)
		if err != nil {
			return err
		}
		fn(p, raw)
		return nil
	})
}

// ---------------------------------------------------------------------------
// CA-734: la portada y los archivos

// CA-734: index.html tiene las pestañas Ahora (data-tab="ahora"), Tablero y
// Cockpit, en ese orden, y Ahora es la activa al cargar: ninguna otra viene
// marcada "on".
func TestCA734_PestanasAhoraTableroCockpit(t *testing.T) {
	html, _ := tbUI(t)
	ps := stPestanas(html)
	var ids []string
	for _, p := range ps {
		ids = append(ids, p.id)
	}
	if strings.Join(ids, ",") != "ahora,tablero,cockpit" {
		t.Fatalf("CA-734: las pestañas son ahora, tablero y cockpit, en ese orden: %v", ids)
	}
	textos := map[string]string{"ahora": "Ahora", "tablero": "Tablero", "cockpit": "Cockpit"}
	reClase := regexp.MustCompile(`\bclass="([^"]*)"`)
	for _, p := range ps {
		if p.texto != textos[p.id] {
			t.Fatalf("CA-734: la pestaña data-tab=%q dice %q, dice %q", p.id, textos[p.id], p.texto)
		}
		activa := false
		if m := reClase.FindStringSubmatch(p.attrs); m != nil {
			for _, c := range strings.Fields(m[1]) {
				activa = activa || c == "on"
			}
		}
		if activa && p.id != "ahora" {
			t.Fatalf("CA-734: Ahora es la pestaña activa al cargar: %q viene marcada on", p.id)
		}
	}
}

// CA-734: index.html carga tablero.js, acciones.js y ahora.js, una vez cada
// uno, en ese orden y despues de su propio script; GET de cada uno responde
// 200 con el archivo embebido, como javascript.
func TestCA734_CargaLosTresScriptsEnOrden(t *testing.T) {
	html, tablero, ahora := stLeerUI(t, "CA-734")
	var srcs []string
	finPropio, primerSrc := -1, -1
	re := regexp.MustCompile(`<script\b([^>]*)>`)
	for _, m := range re.FindAllStringSubmatchIndex(html, -1) {
		attrs := html[m[2]:m[3]]
		if s := regexp.MustCompile(`\bsrc="([^"]*)"`).FindStringSubmatch(attrs); s != nil {
			srcs = append(srcs, s[1])
			if primerSrc < 0 {
				primerSrc = m[0]
			}
			continue
		}
		if fin := strings.Index(html[m[1]:], "</script>"); fin >= 0 && strings.Contains(html[m[1]:m[1]+fin], "async function j(") {
			finPropio = m[1] + fin
		}
	}
	if strings.Join(srcs, ",") != "tablero.js,acciones.js,ahora.js" {
		t.Fatalf("CA-734: index.html carga tablero.js, acciones.js y ahora.js, en ese orden: %v", srcs)
	}
	for _, s := range srcs {
		if strings.Count(html, `<script src="`+s+`"></script>`) != 1 {
			t.Fatalf(`CA-734: index.html carga <script src="%s"></script> una vez`, s)
		}
	}
	if finPropio < 0 || primerSrc < finPropio {
		t.Fatal("CA-734: los tres se cargan DESPUES del script de index.html (el que define j)")
	}

	raw, err := uiFS.ReadFile("ui/acciones.js")
	if err != nil {
		t.Fatal(err)
	}
	dir := newProject(t)
	for _, f := range []struct{ path, cuerpo string }{{"/tablero.js", tablero}, {"/acciones.js", string(raw)}, {"/ahora.js", ahora}} {
		rec := serveGET(t, dir, f.path)
		if rec.Code != http.StatusOK {
			t.Fatalf("CA-734: GET %s responde 200, respondio %d", f.path, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
			t.Fatalf("CA-734: %s se sirve como javascript: %q", f.path, rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != f.cuerpo {
			t.Fatalf("CA-734: %s es el archivo embebido", f.path)
		}
	}
}

// CA-734: la UI embebida son exactamente index.html, tablero.js, acciones.js,
// ahora.js y los 5 .ttf de fuentes/; ningun archivo embebido (los .ttf
// incluidos) contiene http:, https:, //cdn ni @import; y el CSS nuevo va en
// el <style> de index.html, no en ahora.js.
func TestCA734_UIEmbebidaExacta(t *testing.T) {
	var archivos []string
	err := stRecorrerUI(func(p string, raw []byte) {
		archivos = append(archivos, p)
		for _, marca := range []string{"http:", "https:", "//cdn", "@import"} {
			if strings.Contains(string(raw), marca) {
				t.Fatalf("CA-734: %s contiene una referencia externa (%q)", p, marca)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(archivos)
	quiere := "ui/acciones.js,ui/ahora.js," +
		"ui/fuentes/JetBrainsMono-400.ttf,ui/fuentes/JetBrainsMono-500.ttf," +
		"ui/fuentes/SpaceGrotesk-400.ttf,ui/fuentes/SpaceGrotesk-500.ttf,ui/fuentes/SpaceGrotesk-700.ttf," +
		"ui/index.html,ui/tablero.js"
	if strings.Join(archivos, ",") != quiere {
		t.Fatalf("CA-734: la UI embebida son exactamente index.html, tablero.js, acciones.js, ahora.js y los 5 .ttf de fuentes/: %v", archivos)
	}

	_, _, ahora := stLeerUI(t, "CA-734")
	for _, css := range []string{"<style", "insertRule", "CSSStyleSheet"} {
		if strings.Contains(ahora, css) {
			t.Fatalf("CA-734: el CSS nuevo va en el <style> de index.html: ahora.js contiene %q", css)
		}
	}
}

// CA-734: index.html sigue definiendo --human y teniendo las pestañas Cockpit
// y Tablero, y tablero.js sigue cumpliendo lo demas de CA-319 (pinta del JSON
// y reusa renderStage y appendFeed).
func TestCA734_SigueCumpliendoCA319(t *testing.T) {
	html, tablero := tbUI(t)
	tbRevisarPestanas(t, html)
	tbRevisarPintores(t, html, tablero)
	if !strings.Contains(html, "--human:") {
		t.Fatal("CA-734: index.html define --human")
	}
}

// ---------------------------------------------------------------------------
// CA-735: el hash dice la pantalla

// CA-735: la pagina lee location.hash; un hashchange cambia de pestaña (su
// manejador llega a mostrarPestana); cambiar de pestaña escribe el hash; y
// los hashes del contrato estan: #tarjeta/<slug> (los enlaces de Ahora a la
// Tarjeta) y #ahora (el enlace "← Ahora").
func TestCA735_ElHashEligeLaPantalla(t *testing.T) {
	html, tablero, ahora := stLeerUI(t, "CA-735")
	codigo := stCodigo(stScriptPropio(html)) + "\n" + stCodigo(tablero) + "\n" + stCodigo(ahora)
	if !regexp.MustCompile(`location\.hash`).MatchString(codigo) {
		t.Fatal("CA-735: la pagina lee location.hash para elegir la pantalla")
	}
	k := regexp.MustCompile(`["']hashchange["']|onhashchange`).FindStringIndex(codigo)
	if k == nil {
		t.Fatal("CA-735: la pagina escucha hashchange")
	}
	ventana := codigo[k[0]:min(len(codigo), k[0]+300)]
	if !stLlegaA(huCuerpos(codigo), ventana, "mostrarPestana", 2) {
		t.Fatalf("CA-735: un hashchange cambia de pestaña con mostrarPestana:\n%s", ventana)
	}

	escribe := regexp.MustCompile(`location\.hash\s*=[^=]|history\.pushState\(|location\.assign\(|location\.href\s*=[^=]`).MatchString(codigo)
	if !escribe {
		// o cada pestaña es un enlace a su hash
		escribe = true
		for _, p := range stPestanas(html) {
			if !strings.Contains(p.attrs, `href="#`+p.id+`"`) {
				escribe = false
			}
		}
	}
	if !escribe {
		t.Fatal("CA-735: cambiar de pestaña escribe el hash (location.hash = ..., history.pushState o un enlace href=\"#<pestaña>\")")
	}

	ah := stCodigo(ahora)
	if !strings.Contains(ah, "#tarjeta/") {
		t.Fatal("CA-735: los enlaces de Ahora a una tarjeta van a #tarjeta/<slug>")
	}
	if !strings.Contains(ah+html, "#ahora") {
		t.Fatal("CA-735: el enlace ← Ahora va a #ahora")
	}
}

// CA-735: mostrarPestana sigue siendo la unica que muestra y oculta las
// pestañas: se declara una sola vez, nadie la reemplaza, suma Ahora y
// Tarjeta, y ahora.js no toca las pestañas ni los contenedores que ella
// muestra.
func TestCA735_MostrarPestanaEsLaUnica(t *testing.T) {
	html, tablero, ahora := stLeerUI(t, "CA-735")
	fuentes := map[string]string{"index.html": stCodigo(stScriptPropio(html)), "tablero.js": stCodigo(tablero), "ahora.js": stCodigo(ahora)}
	decls, donde := 0, ""
	for n, src := range fuentes {
		if c := len(regexp.MustCompile(`function\s+mostrarPestana\s*\(`).FindAllString(src, -1)); c > 0 {
			decls += c
			donde = n
		}
		if m := regexp.MustCompile(`\bmostrarPestana\s*=[^=]`).FindString(src); m != "" {
			t.Fatalf("CA-735: nadie reemplaza mostrarPestana: %s tiene %q", n, m)
		}
	}
	if decls != 1 {
		t.Fatalf("CA-735: mostrarPestana se declara una sola vez, se declara %d", decls)
	}
	cuerpo, ok := h3CuerpoDe(fuentes[donde], "mostrarPestana")
	if !ok {
		t.Fatal("CA-735: no encontre el cuerpo de mostrarPestana")
	}
	todo := strings.Join([]string{fuentes["index.html"], fuentes["tablero.js"], fuentes["ahora.js"]}, "\n")
	cuerpos := huCuerpos(todo)
	alcance := cuerpo
	for _, m := range regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\(`).FindAllStringSubmatch(cuerpo, -1) {
		if c, ok := cuerpos[m[1]]; ok && m[1] != "mostrarPestana" {
			alcance += "\n" + c
		}
	}
	bajo := strings.ToLower(alcance)
	for _, p := range []string{"ahora", "tarjeta"} {
		if !strings.Contains(bajo, p) {
			t.Fatalf("CA-735: mostrarPestana suma %s (la muestra, la oculta y corta su sondeo):\n%s", p, cuerpo)
		}
	}

	ah := fuentes["ahora.js"]
	for _, re := range []string{`\.tabs\s+\.tab`, `querySelectorAll\(\s*["']\.tab`,
		`(?:\$|getElementById)\(\s*["'](?:tablero|cockpit|runview)["']\s*\)`} {
		if m := regexp.MustCompile(re).FindString(ah); m != "" {
			t.Fatalf("CA-735: solo mostrarPestana muestra y oculta las pestañas: ahora.js tiene %q", m)
		}
	}
	// los contenedores que mostrarPestana muestra y oculta, ahora.js no los
	// muestra ni los oculta por su cuenta
	for _, m := range regexp.MustCompile(`(?:\$|getElementById)\(\s*["']([\w-]+)["']\s*\)\s*\.(?:style\.display|hidden|classList)`).FindAllStringSubmatch(alcance, -1) {
		re := regexp.MustCompile(`(?:\$|getElementById)\(\s*["']` + regexp.QuoteMeta(m[1]) + `["']\s*\)\s*\.(?:style\.display|hidden)`)
		if w := re.FindString(ah); w != "" {
			t.Fatalf("CA-735: solo mostrarPestana muestra y oculta #%s: ahora.js tiene %q", m[1], w)
		}
	}
}

// CA-735 (contrato: "un slug que no es valido para item.ValidSlug lleva a
// Ahora sin pedir nada"): la pagina valida el slug de #tarjeta/<slug> con la
// misma forma que item.ValidSlug. Propiedad: alguna expresion regular de la
// pagina coincide con item.ValidSlug en cada muestra de hasta 64 bytes
// (fijas y al azar), y el largo maximo (64) se respeta, en la expresion o
// con un .length.
func TestCA735_SlugInvalidoVaAAhora(t *testing.T) {
	html, tablero, ahora := stLeerUI(t, "CA-735")
	codigo := stCodigo(stScriptPropio(html)) + "\n" + stCodigo(tablero) + "\n" + stCodigo(ahora)

	muestras := []string{"", "a", "0", "abc", "a-b", "a--b", "a-", "9-", "-a", "--", "A", "aB", "Precios",
		"a_b", "a b", "con%20espacio", "a.b", "a/b", "..", "á", "ñandu", "a\n", "tarjeta/x",
		strings.Repeat("a", 63), strings.Repeat("a", 64), "x" + strings.Repeat("-", 63)}
	azar := rand.New(rand.NewSource(735))
	alfabeto := []rune("abz09-_A .%/ñ")
	for i := 0; i < 3000; i++ {
		r := make([]rune, azar.Intn(70))
		for k := range r {
			r[k] = alfabeto[azar.Intn(len(alfabeto))]
		}
		muestras = append(muestras, string(r))
	}

	largo := regexp.MustCompile(`\.length\b[^;\n]{0,20}\b6[45]\b|\b6[45]\b[^;\n]{0,20}\.length\b`).MatchString(codigo)
	reLit := regexp.MustCompile(`/(\^(?:\\.|\[(?:\\.|[^\]\\\n])*\]|[^/\\\[\n])*\$)/[a-z]*`)
	var vistas []string
	for _, m := range reLit.FindAllStringSubmatch(codigo, -1) {
		re, err := regexp.Compile(m[1])
		if err != nil {
			continue
		}
		vistas = append(vistas, m[1])
		igual := true
		for _, s := range muestras {
			if len(s) <= item.MaxSlug && re.MatchString(s) != item.ValidSlug(s) {
				igual = false
				break
			}
		}
		if igual && (largo || !re.MatchString(strings.Repeat("a", item.MaxSlug+1))) {
			return
		}
	}
	t.Fatalf("CA-735: la pagina valida el slug de #tarjeta/ como item.ValidSlug (minusculas, numeros y guiones, sin guion al principio, hasta %d): ninguna de sus expresiones coincide: %q",
		item.MaxSlug, vistas)
}

// ---------------------------------------------------------------------------
// CA-736 y CA-737: ahora.js es un pintor de solo lectura y sin logica

// CA-736: ahora.js no contiene act(, POST, method, fetch(, XMLHttpRequest,
// sendBeacon, X-Hoom-Token ni setInterval; sus literales /api/ son solo
// /api/board y /api/board/ (mas /timeline), y no pide ninguna otra ruta de
// la tarjeta (terminal, launch, save...).
func TestCA736_AhoraSoloLee(t *testing.T) {
	_, _, ahora := stLeerUI(t, "CA-736")
	for _, p := range []string{"act(", "POST", "method", "fetch(", "XMLHttpRequest", "sendBeacon", "X-Hoom-Token", "setInterval"} {
		if strings.Contains(ahora, p) {
			t.Fatalf("CA-736: ahora.js no contiene %q", p)
		}
	}
	cod := stCodigo(ahora)
	vistas := map[string]bool{}
	for _, lit := range regexp.MustCompile(`/api/[a-z/]*`).FindAllString(cod, -1) {
		if lit != "/api/board" && lit != "/api/board/" {
			t.Fatalf("CA-736: los literales /api/ de ahora.js son solo /api/board y /api/board/: aparece %q", lit)
		}
		vistas[lit] = true
	}
	if len(vistas) == 0 {
		t.Fatal("CA-736: ahora.js lee /api/board y /api/board/<slug>")
	}
	if !strings.Contains(cod, "/timeline") {
		t.Fatal("CA-736: la Tarjeta pide /api/board/<slug>/timeline")
	}
	for _, otra := range []string{"/terminal", "/launch", "/save", "/discard", "/session", "/approve", "/done", "/stage", "/runs"} {
		if strings.Contains(cod, otra) {
			t.Fatalf("CA-736: ahora.js pide solo /api/board, /api/board/<slug> y su /timeline: aparece %q", otra)
		}
	}
}

// stSondeoDesde: la funcion que se vuelve a programar con setTimeout(...,
// 1000) y lee lo que esta en pos: la que lo encierra, la que la llama, o la
// que recibe la lectura (un sondeo generico); tambien si el literal vive en
// una constante, donde se usa la constante.
func stSondeoDesde(js string, pos int) (string, bool) {
	var nombres []string
	if n := atFuncionQueEncierra(js, pos); n != "" {
		nombres = append(nombres, n)
	}
	ini := strings.LastIndex(js[:pos], "\n") + 1
	if m := regexp.MustCompile(`^\s*(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=[^=>]*$`).FindStringSubmatch(js[ini:pos]); m != nil {
		for _, u := range regexp.MustCompile(`\b`+regexp.QuoteMeta(m[1])+`\b`).FindAllStringIndex(js, -1) {
			if u[0] > pos {
				if n := atFuncionQueEncierra(js, u[0]); n != "" {
					nombres = append(nombres, n)
				}
			}
		}
	}
	cuerpos := huCuerpos(js)
	esDecl := regexp.MustCompile(`function\s*$`)
	for _, n := range nombres {
		if atEncadenada(js, n) {
			return n, true
		}
		for _, m := range regexp.MustCompile(`\b`+regexp.QuoteMeta(n)+`\(`).FindAllStringIndex(js, -1) {
			if esDecl.MatchString(js[:m[0]]) {
				continue
			}
			if q := atFuncionQueEncierra(js, m[0]); q != "" && q != n && atEncadenada(js, q) {
				return q, true
			}
		}
		for _, m := range regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\(`).FindAllStringSubmatch(cuerpos[n], -1) {
			if q := m[1]; q != n && atEncadenada(js, q) {
				return q, true
			}
		}
	}
	return "", false
}

// CA-736: Ahora y Tarjeta sondean con setTimeout(..., 1000) encadenado a cada
// respuesta (cada lectura de /api/board vive en una funcion que se vuelve a
// programar), esa funcion mira que la pagina este visible, y al volver
// visible la pagina el sondeo se retoma (visibilitychange, en ahora.js o en
// index.html: tablero.js no cambia salvo mostrarPestana, CA-743).
func TestCA736_SondeoEncadenadoYVisible(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-736")
	cod := stCodigo(ahora)
	if !regexp.MustCompile(`setTimeout\([\s\S]{1,300}?,\s*1000\s*\)`).MatchString(cod) {
		t.Fatal("CA-736: ahora.js sondea con setTimeout(..., 1000)")
	}
	cuerpos := huCuerpos(cod)
	visible := regexp.MustCompile(`visibilityState|\bvisible\(\)|document\.hidden`)
	usos := regexp.MustCompile(`/api/board`).FindAllStringIndex(cod, -1)
	if len(usos) == 0 {
		t.Fatal("CA-736: ahora.js lee /api/board")
	}
	for _, u := range usos {
		nombre, ok := stSondeoDesde(cod, u[0])
		if !ok {
			t.Fatalf("CA-736: la lectura de ...%s... se vuelve a pedir con setTimeout(..., 1000) despues de cada respuesta (nunca setInterval)",
				cod[u[0]:min(len(cod), u[0]+40)])
		}
		alcance := cuerpos[nombre]
		for _, m := range regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\(`).FindAllStringSubmatch(alcance, -1) {
			if c, ok := cuerpos[m[1]]; ok && m[1] != nombre {
				alcance += "\n" + c
			}
		}
		if !visible.MatchString(alcance) {
			t.Fatalf("CA-736: %s, que sondea, solo pide con la pagina visible (document.visibilityState)", nombre)
		}
	}
	if !strings.Contains(cod, "visibilitychange") && !strings.Contains(stCodigo(stScriptPropio(html)), "visibilitychange") {
		t.Fatal("CA-736: al volver visible la pagina, Ahora y Tarjeta retoman su sondeo (visibilitychange)")
	}
}

// CA-737: ahora.js no decide grupos: no contiene los ids de columna
// tu-aprobacion, tu-aceptacion, test-writer ni backlog, ni "hecho" como
// cadena; solo lee esperado para decir "FAIL esperado" (CA-741), nunca para
// elegir seccion; y elige la seccion por pelota.grupo y el orden por
// pelota.orden.
func TestCA737_AhoraNoDecideGrupos(t *testing.T) {
	_, _, ahora := stLeerUI(t, "CA-737")
	for _, id := range []string{"tu-aprobacion", "tu-aceptacion", "test-writer", "backlog"} {
		if strings.Contains(ahora, id) {
			t.Fatalf("CA-737: ahora.js no decide grupos: no contiene el id de columna %q", id)
		}
	}
	for _, q := range []string{`"`, `'`, "`"} {
		if strings.Contains(ahora, q+"hecho"+q) {
			t.Fatalf("CA-737: ahora.js no decide grupos: no contiene el id de columna %s", q+"hecho"+q)
		}
	}

	cod := stCodigo(ahora)
	cuerpos := huCuerpos(cod)
	dice := regexp.MustCompile(`FAIL esperado`).FindAllStringIndex(cod, -1)
	for _, m := range regexp.MustCompile(`\besperado\b`).FindAllStringIndex(cod, -1) {
		if m[0] >= 5 && cod[m[0]-5:m[0]] == "FAIL " {
			continue // el texto que se muestra
		}
		ok := false
		if n := atFuncionQueEncierra(cod, m[0]); n != "" && strings.Contains(cuerpos[n], "FAIL esperado") {
			ok = true
		}
		for _, d := range dice {
			if d[0]-m[0] <= 400 && m[0]-d[0] <= 400 {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("CA-737: ahora.js lee esperado solo para decir \"FAIL esperado\", nunca para elegir seccion: ...%s...",
				cod[max(0, m[0]-60):min(len(cod), m[1]+40)])
		}
	}

	stUsa(t, "CA-737", "ahora.js", cod, "pelota", "grupo", "orden")
	for _, g := range stGrupos {
		if !strings.Contains(cod, g) {
			t.Fatalf("CA-737: ahora.js elige la seccion por pelota.grupo: nombra el grupo %q", g)
		}
	}
}

// ---------------------------------------------------------------------------
// CA-738 y CA-739: la banda y las secciones

// CA-738: la banda TU SIGUIENTE PASO lleva su etiqueta, la cuenta (te toca,
// corriendo y detenida con sus numeros), la frase, el plain y el boton de la
// tarjeta de caja, "Ver la tarjeta" y "después:"; sin caja, los tres textos
// del contrato; y su borde es --hd-amarillo o --hd-borde.
func TestCA738_BandaTuSiguientePaso(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-738")
	cod := stCodigo(ahora)
	stTiene(t, "CA-738", "la UI (index.html o ahora.js)", html+cod, "TU SIGUIENTE PASO")
	stTiene(t, "CA-738", "ahora.js", cod,
		"te toca ", "corriendo ", "detenida ", " · ",
		"Ver la tarjeta", "después: ", "«", "»",
		"Nada te espera. hoom trabaja en", "corriendo: «", " más",
		"Nada en curso. Podés empezar",
		"No hay nada pendiente.", "para sumar trabajo: hoom item add",
		"se cortó el trabajo del")
	stUsa(t, "CA-738", "ahora.js", cod, "caja", "cuenta", "te_toca", "corriendo", "detenida", "frase", "plain", "etiqueta", "texto")
	stVar(t, "CA-738", html, ahora, "--hd-amarillo", "--hd-borde", "--hd-grafito", "--mono")
}

// CA-738 (caso limite "un item invalido"): en Ahora, los warnings son una
// linea en --hd-naranja arriba de la banda: "<n> item(s) no se pudieron
// leer: mirá el Tablero".
func TestCA738_AvisoDeItemsIlegibles(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-738")
	cod := stCodigo(ahora)
	stTiene(t, "CA-738", "ahora.js", cod, "no se pudieron leer: mirá el Tablero")
	stUsa(t, "CA-738", "ahora.js", cod, "warnings")
	stVar(t, "CA-738", html, ahora, "--hd-naranja")
}

// CA-739: Ahora tiene los cinco titulos de seccion del contrato, el subtitulo
// de Te toca, "y <k> más en el Tablero" para Hecho, y el pie.
func TestCA739_SeccionesYPie(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-739")
	cod := stCodigo(ahora)
	stTiene(t, "CA-739", "ahora.js", cod,
		"Te toca — ", "tuya", "hoom no firma ni acepta por vos",
		"En curso — lo que hoom hace ahora", "trabajando",
		"Detenidas — hoom paró y dice por qué",
		"En cola — todavía no empezó",
		"Hecho — lo que ya aceptaste", "más en el Tablero")
	stTiene(t, "CA-739", "la UI (index.html o ahora.js)", html+cod, "acá ves lo que hoom comprobó, no lo que el modelo cuenta")
}

// CA-739: las filas de cada seccion: la tarjeta de Te toca (badge del paso
// del camino en MAYUSCULA, slug, plain, pendientes, VERDE), la fila de En
// curso (enlace a #tarjeta/, riesgo, piloto encendido, la barra del camino,
// "paso <i> de 8 — ...", AHORA con CORRIENDO y "<provider> · hace
// <tiempo>[ · tope <limite>]", SIGUE), la de Detenidas (DETENIDA, la caja
// hundida, las acciones habilitadas) y las de En cola y Hecho ("aceptada
// <fecha>" con item.hecho_en).
func TestCA739_FilasDeLasSecciones(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-739")
	cod := stCodigo(ahora)
	stTiene(t, "CA-739", "ahora.js", cod,
		"toUpperCase(", "VERDE", "Ver la tarjeta",
		"#tarjeta/", "riesgo ", "piloto encendido", "hasta-humano",
		"de 8 — ", "toLowerCase(", "CORRIENDO", " · hace ", " · tope ",
		"DETENIDA", "aceptada ")
	stUsa(t, "CA-739", "ahora.js", cod,
		"slug", "titulo", "plain", "pendientes", "texto", "evidence", "verdict", "riesgo", "auto",
		"camino", "estado", "nombre", "ahora", "sigue", "gris", "running", "provider", "started_at", "timeout_s",
		"actions", "enabled", "label", "hecho_en")
	stVar(t, "CA-739", html, ahora, "--hd-verde", "--hd-azul", "--hd-borde", "--hd-carbon-900", "--hd-gris", "--hd-amarillo", "--hd-rojo")
}

// CA-739 (caso limite "una tarjeta sin titulo"): la pagina usa el slug cuando
// el titulo esta vacio (titulo || slug, o su ternario).
func TestCA739_TarjetaSinTituloUsaElSlug(t *testing.T) {
	_, _, ahora := stLeerUI(t, "CA-739")
	cod := stCodigo(ahora)
	re := regexp.MustCompile(`\btitulo\b[^;\n]{0,80}(?:\|\||\?\?|\?)[^;\n]{0,80}\bslug\b|\bslug\b[^;\n]{0,80}(?:\|\||\?\?|\?|:)[^;\n]{0,80}\btitulo\b`)
	if !re.MatchString(cod) {
		t.Fatal("CA-739: una tarjeta sin titulo usa su slug (titulo || slug) en la seccion y en el detalle")
	}
}

// ---------------------------------------------------------------------------
// CA-740: los botones van por abrirAccion

// CA-740: cada boton de paso o de accion llama a abrirAccion(card, action)
// de acciones.js con la tarjeta y la accion del JSON (paso.accion buscada en
// card.actions), y ahora.js no define su propio dialogo ni su propia
// confirmacion.
func TestCA740_BotonesPorAbrirAccion(t *testing.T) {
	_, _, ahora := stLeerUI(t, "CA-740")
	cod := stCodigo(ahora)
	if !regexp.MustCompile(`\babrirAccion\([^,()]*(?:\([^()]*\)[^,()]*)*,`).MatchString(cod) {
		t.Fatal("CA-740: ahora.js llama a abrirAccion(card, action) de acciones.js")
	}
	stUsa(t, "CA-740", "ahora.js", cod, "actions", "accion", "etiqueta", "enabled", "label")
	for _, re := range []string{`function\s+abrirAccion\b`, `\babrirAccion\s*=[^=]`, `\bconfirm\(`, `\bprompt\(`, `<dialog`, `showModal`} {
		if m := regexp.MustCompile(re).FindString(cod); m != "" {
			t.Fatalf("CA-740: ahora.js no define su propio dialogo ni su propia confirmacion: tiene %q", m)
		}
	}
	for _, d := range atDeclaracion.FindAllStringSubmatch(cod, -1) {
		n := d[1] + d[2]
		bajo := strings.ToLower(n)
		if strings.HasPrefix(bajo, "dialogo") || strings.HasPrefix(bajo, "confirmar") || n == "cerrarDialogo" {
			t.Fatalf("CA-740: ahora.js no define su propio dialogo ni su propia confirmacion: declara %s", n)
		}
	}
}

// CA-740 (casos limite "un paso deshabilitado" y "sin token de acciones"): el
// boton sale de paso.accion (un paso deshabilitado o que no hay tiene accion
// "" y se muestra su texto en --hd-gris), y Ahora y Tarjeta no piden el token
// por su cuenta: eso es del dialogo de acciones.js.
func TestCA740_PasoDeshabilitadoYSinToken(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-740")
	cod := stCodigo(ahora)
	if !regexp.MustCompile(`\.accion\b`).MatchString(cod) || !regexp.MustCompile(`\.texto\b`).MatchString(cod) {
		t.Fatal("CA-740: el boton del paso sale de paso.accion, y sin accion se muestra paso.texto")
	}
	stVar(t, "CA-740", html, ahora, "--hd-gris")
	for _, p := range []string{"hoom-token", `$("token")`, `$('token')`, `getElementById("token")`, `getElementById('token')`, "X-Hoom-Token"} {
		if strings.Contains(ahora, p) {
			t.Fatalf("CA-740: Ahora y Tarjeta no piden el token por su cuenta: ahora.js contiene %q", p)
		}
	}
}

// ---------------------------------------------------------------------------
// CA-742: los errores de Ahora

// CA-742: Ahora dice "leyendo el tablero..." antes de la primera respuesta y
// "no se pudo leer el tablero: <error>" si la lectura falla.
func TestCA742_ErroresDeAhora(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-742")
	cod := stCodigo(ahora)
	stTiene(t, "CA-742", "la UI (index.html o ahora.js)", html+cod, "leyendo el tablero...")
	stTiene(t, "CA-742", "ahora.js", cod, "no se pudo leer el tablero: ")
}
