// Tests de los hallazgos de la review a82af1 (4 lentes) de
// studio-tu-siguiente-paso, corroborados por el refutador. Son estaticos,
// como los de studio_ahora_ui_test.go: leen ui/ahora.js y ui/index.html
// embebidos y afirman la propiedad que pide el spec
// (.hoom/specs/studio-tu-siguiente-paso.md), no una linea exacta. Donde el
// hallazgo nombra la funcion (pedirFicha, haceAh, chipsAh, lineaAh) el test
// la busca por su nombre.
//
// Para no confundir codigo con texto, los tests trabajan sobre ahora.js sin
// comentarios (stCodigo) y con una mascara de lo que es codigo: lo que esta
// dentro de una cadena o del texto de una plantilla no abre ni cierra
// llaves; lo que esta dentro de ${...} si.
package servecmd

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// hzUI lee index.html, ahora.js y acciones.js embebidos; hz es el hallazgo.
func hzUI(t *testing.T, hz string) (html, ahora, acciones string) {
	t.Helper()
	html, _, ahora = stLeerUI(t, hz)
	raw, err := uiFS.ReadFile("ui/acciones.js")
	if err != nil {
		t.Fatal(err)
	}
	return html, ahora, string(raw)
}

// hzMascara marca con true cada byte de js que es codigo: no lo es lo que
// esta dentro de una cadena ("...", '...') ni el texto de una plantilla
// (`...`), ni las llaves de ${ y su cierre; si lo es lo de adentro de
// ${...}. Las expresiones regulares literales no se distinguen.
func hzMascara(js string) []bool {
	m := make([]bool, len(js))
	type marco struct {
		plantilla bool
		llaves    int
	}
	pila := []marco{{}}
	for i := 0; i < len(js); i++ {
		top := &pila[len(pila)-1]
		c := js[i]
		if top.plantilla {
			switch {
			case c == '\\':
				i++
			case c == '`':
				pila = pila[:len(pila)-1]
			case c == '$' && i+1 < len(js) && js[i+1] == '{':
				i++
				pila = append(pila, marco{})
			}
			continue
		}
		switch c {
		case '"', '\'':
			for i++; i < len(js) && js[i] != c && js[i] != '\n'; i++ {
				if js[i] == '\\' {
					i++
				}
			}
		case '`':
			pila = append(pila, marco{plantilla: true})
		case '{':
			top.llaves++
			m[i] = true
		case '}':
			if top.llaves == 0 && len(pila) > 1 {
				pila = pila[:len(pila)-1] // el cierre de ${...}
			} else {
				top.llaves--
				m[i] = true
			}
		default:
			m[i] = true
		}
	}
	return m
}

// hzCierre es la posicion del (, [ o { que cierra el que abre en abre (-1 si
// no cierra).
func hzCierre(js string, m []bool, abre int) int {
	nivel := 0
	for k := abre; k < len(js); k++ {
		if !m[k] {
			continue
		}
		switch js[k] {
		case '(', '[', '{':
			nivel++
		case ')', ']', '}':
			nivel--
			if nivel == 0 {
				return k
			}
		}
	}
	return -1
}

// hzAbiertos son las posiciones de los (, [ y { que, contando desde desde,
// siguen abiertos en p, de afuera hacia adentro.
func hzAbiertos(js string, m []bool, desde, p int) []int {
	var pila []int
	for k := desde; k < p && k < len(js); k++ {
		if !m[k] {
			continue
		}
		switch js[k] {
		case '(', '[', '{':
			pila = append(pila, k)
		case ')', ']', '}':
			if len(pila) > 0 {
				pila = pila[:len(pila)-1]
			}
		}
	}
	return pila
}

var hzReSigueDecl = regexp.MustCompile(`^\n\s*(?:async\s+)?(?:function|const|let|var)\b`)

// hzCuerpo da los limites [ini, fin) del cuerpo de la funcion declarada con
// ese nombre (function nombre(...) {...}, o const nombre = (...) => {...} o
// => expresion).
func hzCuerpo(js string, m []bool, nombre string) (int, int, bool) {
	for _, d := range atDeclaracion.FindAllStringSubmatchIndex(js, -1) {
		n := ""
		if d[2] >= 0 {
			n = js[d[2]:d[3]]
		} else {
			n = js[d[4]:d[5]]
		}
		if n != nombre || !m[d[0]] {
			continue
		}
		i := d[1]
		if d[2] >= 0 { // function nombre( : saltar los parametros
			c := hzCierre(js, m, d[1]-1)
			if c < 0 {
				return 0, 0, false
			}
			i = c + 1
		}
		for i < len(js) && strings.ContainsRune(" \t\r\n", rune(js[i])) {
			i++
		}
		if i < len(js) && js[i] == '{' && m[i] {
			c := hzCierre(js, m, i)
			if c < 0 {
				return 0, 0, false
			}
			return i + 1, c, true
		}
		// una flecha con cuerpo de expresion: hasta su ; o el cierre que la
		// termina, o una declaracion en la linea siguiente
		nivel := 0
		for k := i; k < len(js); k++ {
			if !m[k] {
				continue
			}
			switch js[k] {
			case '(', '[', '{':
				nivel++
			case ')', ']', '}':
				if nivel == 0 {
					return i, k, true
				}
				nivel--
			case ';':
				if nivel == 0 {
					return i, k, true
				}
			case '\n':
				if nivel == 0 && hzReSigueDecl.MatchString(js[k:]) {
					return i, k, true
				}
			}
		}
		return i, len(js), true
	}
	return 0, 0, false
}

// hzCuerpos: el cuerpo exacto de cada funcion con nombre de js.
func hzCuerpos(js string) map[string]string {
	m := hzMascara(js)
	out := map[string]string{}
	for _, d := range atDeclaracion.FindAllStringSubmatch(js, -1) {
		n := d[1] + d[2]
		if _, ya := out[n]; ya {
			continue
		}
		if ini, fin, ok := hzCuerpo(js, m, n); ok {
			out[n] = js[ini:fin]
		}
	}
	return out
}

var hzReIdent = regexp.MustCompile(`[A-Za-z_$][\w$]*`)

// hzAlcance es texto mas el cuerpo de cada funcion de decl que nombra, y asi
// hasta niveles de ayudantes.
func hzAlcance(texto string, decl map[string]string, niveles int) string {
	var b strings.Builder
	b.WriteString(texto)
	vistos := map[string]bool{}
	frente := []string{texto}
	for n := 0; n < niveles; n++ {
		var sig []string
		for _, f := range frente {
			for _, tok := range hzReIdent.FindAllString(f, -1) {
				if c, ok := decl[tok]; ok && !vistos[tok] {
					vistos[tok] = true
					b.WriteString("\n")
					b.WriteString(c)
					sig = append(sig, c)
				}
			}
		}
		frente = sig
	}
	return b.String()
}

// hzCorte es un pedazo corto de js alrededor de p, para los mensajes.
func hzCorte(js string, p int) string {
	return js[max(0, p-80):min(len(js), p+120)]
}

// ---------------------------------------------------------------------------
// Hallazgo 20261117T203125_95d04e (high). CA-354 de
// acciones-desde-la-tarjeta.md ("Las acciones con expert solo se muestran en
// modo experto"), que el spec nuevo mantiene ("Los demas criterios del
// Studio siguen igual"): los botones de Detenidas ("un boton secundario por
// cada accion habilitada", CA-739) y de la columna de botones de la Tarjeta
// ("un secundario por cada otra accion habilitada", CA-740) no muestran una
// accion expert fuera del modo experto.

var (
	hzReUsoActions  = regexp.MustCompile(`\bactions\b`)
	hzReColaRecorre = regexp.MustCompile(`^\s*(?:(?:\|\||\?\?)\s*\[\s*\]\s*)?\)?\s*\??\.\s*(?:filter|map|forEach|slice|flatMap|reduce)\s*\(`)
	hzReForOf       = regexp.MustCompile(`for\s*\(\s*(?:const|let|var)\s+[^;)]*?\bof\s+[^;)]*$`)
	hzReAlias       = regexp.MustCompile(`(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*\(?\s*(?:[A-Za-z_$][\w$]*\s*\??\.\s*)*$`)
	hzReColaAlias   = regexp.MustCompile(`^\s*(?:(?:\|\||\?\?)\s*\[\s*\]\s*)?\)?\s*[;,\n]`)
	hzReArmaBoton   = regexp.MustCompile("<button|createElement\\(\\s*[\"'`]button|\\blabel\\b|\\babrirAccion\\b|\\betiquetaAccion\\b")
	hzRePredicado   = regexp.MustCompile(`\.\s*(?:filter|some|every)\(\s*([A-Za-z_$][\w$]*)\s*\)`)
	hzReExpert      = regexp.MustCompile(`\bexpert\b`)
)

// hzRecorridosDeAcciones son los recorridos de actions de cod (filter, map,
// forEach, slice, flatMap, reduce o for...of, directos o por un alias como
// const acts = c.actions || []): [inicio, fin) de la ventana de cada uno,
// que va desde el uso de actions hasta 400 bytes despues del recorrido.
func hzRecorridosDeAcciones(cod string) [][2]int {
	var out [][2]int
	for _, u := range hzReUsoActions.FindAllStringIndex(cod, -1) {
		antes, cola := cod[max(0, u[0]-80):u[0]], cod[u[1]:]
		if hzReColaRecorre.MatchString(cola) || hzReForOf.MatchString(antes) {
			out = append(out, [2]int{u[0], min(len(cod), u[1]+400)})
			continue
		}
		a := hzReAlias.FindStringSubmatch(antes)
		if a == nil || !hzReColaAlias.MatchString(cola) {
			continue
		}
		hasta := min(len(cod), u[1]+1500)
		q := regexp.QuoteMeta(a[1])
		reRec := regexp.MustCompile(`\b` + q + `\s*\??\.\s*(?:filter|map|forEach|slice|flatMap|reduce)\s*\(|for\s*\(\s*(?:const|let|var)\s+[\w$]+\s+of\s+\(?\s*` + q + `\b`)
		for _, r := range reRec.FindAllStringIndex(cod[u[1]:hasta], -1) {
			out = append(out, [2]int{u[0], min(len(cod), u[1]+r[1]+400)})
		}
	}
	return out
}

// Hallazgo 20261117T203125_95d04e, CA-354 (acciones-desde-la-tarjeta) y
// CA-739/CA-740: cada recorrido de card.actions de ahora.js que arma
// botones (los de Detenidas y la columna de botones de la Tarjeta) mira
// expert: en su misma expresion, en las lineas que siguen o en un ayudante o
// predicado que llama (por ejemplo .filter(a => !a.expert || experto())).
// Un .find de la accion del paso no es un recorrido: el paso nunca es una
// accion expert (descartar, sesion, terminal).
func TestHallazgo_95d04e_CA354_AccionesExpertSoloEnModoExperto(t *testing.T) {
	html, ahora, acciones := hzUI(t, "95d04e")
	_, tablero := tbUI(t)
	otros := stCodigo(stScriptPropio(html)) + "\n" + stCodigo(tablero) + "\n" + stCodigo(acciones)
	if msg := hzRevisar95d04e(otros, ahora); msg != "" {
		t.Fatal(msg)
	}
}

// hzRevisar95d04e devuelve la falla ("" si no hay); otros es el codigo de
// index.html, tablero.js y acciones.js, donde pueden vivir los ayudantes.
func hzRevisar95d04e(otros, ahora string) string {
	cod := stCodigo(ahora)
	declAhora := hzCuerpos(cod)
	declTodos := hzCuerpos(otros)
	for n, c := range declAhora {
		declTodos[n] = c
	}

	revisados := 0
	for _, r := range hzRecorridosDeAcciones(cod) {
		ventana := cod[r[0]:r[1]]
		if !hzReArmaBoton.MatchString(hzAlcance(ventana, declAhora, 2)) {
			continue // no arma botones (por ejemplo, cuenta)
		}
		revisados++
		// expert se mira en la ventana, en los ayudantes que nombra (un
		// nivel) y en los predicados que le pasa a filter/some/every
		al := hzAlcance(ventana, declAhora, 1)
		for _, p := range hzRePredicado.FindAllStringSubmatch(ventana, -1) {
			al += "\n" + declTodos[p[1]]
		}
		if !hzReExpert.MatchString(al) {
			return fmt.Sprintf("95d04e CA-354: este recorrido de card.actions arma botones con cada accion habilitada sin mirar expert: una accion expert (Descartar cambios, Abrir sesion, Ver terminal) solo se muestra en modo experto (filtrar con !a.expert || experto(), como acciones.js):\n...%s...",
				hzCorte(cod, r[0]))
		}
	}
	if revisados > 0 {
		return ""
	}
	// sin recorridos propios: los botones salen de un ayudante de otro
	// archivo que recorre actions y mira expert
	for _, tok := range hzReLlamada.FindAllStringSubmatch(cod, -1) {
		if _, propia := declAhora[tok[1]]; propia {
			continue
		}
		if c, ok := declTodos[tok[1]]; ok && hzReUsoActions.MatchString(c) && hzReExpert.MatchString(c) {
			return ""
		}
	}
	return "95d04e CA-354: ahora.js arma los botones de Detenidas y de la Tarjeta con card.actions, y solo muestra una accion expert en modo experto: no encontre ningun recorrido de actions que arme botones ni un ayudante que filtre expert"
}

// ---------------------------------------------------------------------------
// Hallazgo 20261117T203622_1f6f83 (high). Spec, "Una fila de En curso" (CA-739)
// y "El titulo y los chips" de la Tarjeta (CA-741): el chip "riesgo <riesgo>"
// va "cuando la evidencia trae riesgo", y evidence.riesgo siempre viene (por
// defecto normal). El chip sale tambien con "riesgo normal": ahora.js no
// compara el riesgo con "normal".

// Hallazgo 20261117T203622_1f6f83, CA-739 y CA-741: ninguna lectura del
// riesgo en ahora.js esta junto a un literal "normal" (ni r !== "normal" ni
// ["normal"].includes(r)), y chipsAh, si existe, no lo nombra.
func TestHallazgo_1f6f83_CA739_CA741_ChipRiesgoTambienNormal(t *testing.T) {
	_, ahora, _ := hzUI(t, "1f6f83")
	if msg := hzRevisar1f6f83(ahora); msg != "" {
		t.Fatal(msg)
	}
}

func hzRevisar1f6f83(ahora string) string {
	cod := stCodigo(ahora)
	reNormal := regexp.MustCompile("\"normal\"|'normal'|`normal`")
	usos := regexp.MustCompile(`\briesgo\b`).FindAllStringIndex(cod, -1)
	if len(usos) == 0 {
		return "1f6f83 CA-739: fixture: ahora.js lee evidence.riesgo para el chip riesgo <riesgo>"
	}
	for _, u := range usos {
		v := cod[max(0, u[0]-200):min(len(cod), u[1]+200)]
		if w := reNormal.FindString(v); w != "" {
			return fmt.Sprintf("1f6f83 CA-739/CA-741: el chip riesgo <riesgo> sale cuando la evidencia trae riesgo, tambien riesgo normal: ahora.js compara el riesgo con %s:\n...%s...", w, v)
		}
	}
	if ini, fin, ok := hzCuerpo(cod, hzMascara(cod), "chipsAh"); ok {
		if w := reNormal.FindString(cod[ini:fin]); w != "" {
			return fmt.Sprintf("1f6f83 CA-739/CA-741: chipsAh no esconde el chip con riesgo normal: nombra %s:\n%s", w, cod[ini:fin])
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Hallazgo 20261117T203622_af3462 (high). Spec, casos limite: "Un titulo largo
// se corta con … en la banda (despues:) y en las filas de una linea. En las
// tarjetas se parte en varias lineas." Banda: CA-738; filas de En cola y
// Hecho: CA-739.

var (
	hzReAtributoClase = regexp.MustCompile("class(?:Name)?\\s*=\\s*[\"'`]([^\"'`]*)|classList\\.add\\(\\s*[\"'`]([^\"'`]*)")
	hzReNombreClase   = regexp.MustCompile(`[A-Za-z_][\w-]*`)
)

// hzClasesQueCortan son las clases del CSS con white-space: nowrap,
// overflow (u overflow-x) hidden o clip y text-overflow: ellipsis en la
// misma regla. De cada selector vale la clase de su ultimo compuesto; si el
// ultimo es una etiqueta sola (.ah-linea a), la del compuesto anterior.
func hzClasesQueCortan(css string) map[string]bool {
	out := map[string]bool{}
	reClase := regexp.MustCompile(`\.([A-Za-z_][\w-]*)`)
	reComb := regexp.MustCompile(`\s*[>+~]\s*`)
	for _, r := range regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`).FindAllStringSubmatch(css, -1) {
		d := stDeclaraciones(r[2])
		ov := d["overflow"]
		if ov == "" {
			ov = d["overflow-x"]
		}
		if !strings.HasPrefix(d["white-space"], "nowrap") || !strings.Contains(d["text-overflow"], "ellipsis") ||
			!(strings.HasPrefix(ov, "hidden") || strings.HasPrefix(ov, "clip")) {
			continue
		}
		for _, sel := range strings.Split(r[1], ",") {
			partes := strings.Fields(reComb.ReplaceAllString(strings.TrimSpace(sel), " "))
			for i := len(partes) - 1; i >= 0; i-- {
				cs := reClase.FindAllStringSubmatch(partes[i], -1)
				if len(cs) == 0 {
					continue
				}
				for _, c := range cs {
					out[c[1]] = true
				}
				break
			}
		}
	}
	return out
}

// hzClasesCerca son las clases del atributo class mas cercano antes de pos
// (hasta 300 bytes, sin salir de desde) y del primero despues (hasta 300),
// y, si el texto de pos se guarda en una variable, las de los atributos
// cercanos a cada ${variable}.
func hzClasesCerca(cod string, desde, pos int) []string {
	var out []string
	tomar := func(m []string) {
		for _, g := range m[1:] {
			out = append(out, hzReNombreClase.FindAllString(g, -1)...)
		}
	}
	antes := cod[max(desde, pos-300):pos]
	if ms := hzReAtributoClase.FindAllStringSubmatch(antes, -1); len(ms) > 0 {
		tomar(ms[len(ms)-1])
	}
	if m := hzReAtributoClase.FindStringSubmatch(cod[pos:min(len(cod), pos+300)]); m != nil {
		tomar(m)
	}
	linea := cod[strings.LastIndex(cod[:pos], "\n")+1 : pos]
	if v := regexp.MustCompile(`(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=`).FindStringSubmatch(linea); v != nil {
		for _, u := range regexp.MustCompile(`\$\{\s*`+regexp.QuoteMeta(v[1])+`\b`).FindAllStringIndex(cod[pos:], -1) {
			p := pos + u[0]
			if ms := hzReAtributoClase.FindAllStringSubmatch(cod[max(pos, p-300):p], -1); len(ms) > 0 {
				tomar(ms[len(ms)-1])
			}
		}
	}
	return out
}

func hzAlgunaCorta(clases []string, cortan map[string]bool) bool {
	for _, c := range clases {
		if cortan[c] {
			return true
		}
	}
	return false
}

// Hallazgo 20261117T203622_af3462, caso limite "un titulo largo" (CA-738 y
// CA-739): index.html tiene una regla que corta con … (white-space: nowrap,
// overflow: hidden, text-overflow: ellipsis); el elemento de "después:" de
// la banda lleva una de sus clases, y el titulo de las filas de una linea
// (lineaAh, En cola y Hecho) tambien (en el enlace, en lo que lo envuelve o
// en la fila, con un selector como .ah-linea a).
func TestHallazgo_af3462_CA738_CA739_TituloLargoSeCortaConElipsis(t *testing.T) {
	html, ahora, _ := hzUI(t, "af3462")
	if msg := hzRevisaraf3462(stEstilo(t, html), ahora); msg != "" {
		t.Fatal(msg)
	}
}

// hzRevisaraf3462: css es el de los <style> de index.html.
func hzRevisaraf3462(css, ahora string) string {
	cortan := hzClasesQueCortan(css)
	if len(cortan) == 0 {
		return "af3462 CA-738/CA-739: un titulo largo se corta con … en la banda (después:) y en las filas de una linea: el CSS de index.html no tiene ninguna regla con white-space: nowrap, overflow: hidden y text-overflow: ellipsis juntos"
	}
	cod := stCodigo(ahora)

	usos := regexp.MustCompile(`después:`).FindAllStringIndex(cod, -1)
	if len(usos) == 0 {
		return "af3462 CA-738: fixture: la banda dice después: «<titulo>» · «<titulo>»"
	}
	for _, u := range usos {
		if cl := hzClasesCerca(cod, 0, u[0]); !hzAlgunaCorta(cl, cortan) {
			return fmt.Sprintf("af3462 CA-738: el elemento de después: «…» · «…» de la banda lleva una clase que corta con … (nowrap, overflow hidden, text-overflow ellipsis); sus clases cercanas %q no estan entre las que cortan %v:\n...%s...",
				cl, cortan, hzCorte(cod, u[0]))
		}
	}

	m := hzMascara(cod)
	ini, fin, ok := hzCuerpo(cod, m, "lineaAh")
	if !ok {
		k := strings.Index(cod, "aceptada ")
		if k < 0 {
			return "af3462 CA-739: fixture: las filas de una linea las arma lineaAh (o la funcion que dice aceptada <fecha>)"
		}
		if ini, fin, ok = hzCuerpo(cod, m, atFuncionQueEncierra(cod, k)); !ok {
			return "af3462 CA-739: fixture: no encontre la funcion que arma las filas de una linea"
		}
	}
	k := strings.Index(cod[ini:fin], "#tarjeta/")
	if k < 0 {
		return fmt.Sprintf("af3462 CA-739: fixture: la fila de una linea lleva el titulo como enlace a #tarjeta/<slug>:\n%s", cod[ini:fin])
	}
	if cl := hzClasesCerca(cod, ini, ini+k); !hzAlgunaCorta(cl, cortan) {
		return fmt.Sprintf("af3462 CA-739: el titulo de las filas de una linea (En cola y Hecho) se corta con …: sus clases cercanas %q no estan entre las que cortan %v:\n%s",
			cl, cortan, cod[ini:fin])
	}
	return ""
}

// ---------------------------------------------------------------------------
// Hallazgo 20261117T203726_ab487d (high). Spec, casos limite: "Un running con
// started_at en el futuro (reloj corrido) dice hace 0 min". Lo dicen la
// banda (CA-738), En curso (CA-739) y la Tarjeta (CA-741), los tres con
// haceAh.

// Hallazgo 20261117T203726_ab487d, caso limite "started_at en el futuro"
// (CA-738, CA-739 y CA-741): haceAh (con sus ayudantes) dice minutos, y con
// un started_at en el futuro dice "0 min", no "0 s": o tiene el literal
// "0 min" para ese caso, o no tiene unidad de segundos y topea la
// diferencia en 0 (Math.max(0, ...), < 0).
func TestHallazgo_ab487d_HaceConStartedAtEnElFuturoDiceCeroMin(t *testing.T) {
	_, ahora, _ := hzUI(t, "ab487d")
	if msg := hzRevisarab487d(ahora); msg != "" {
		t.Fatal(msg)
	}
}

func hzRevisarab487d(ahora string) string {
	cod := stCodigo(ahora)
	ini, fin, ok := hzCuerpo(cod, hzMascara(cod), "haceAh")
	if !ok {
		return "ab487d CA-739: fixture: ahora.js declara haceAh, el que dice el <tiempo> de hace <tiempo>"
	}
	al := hzAlcance(cod[ini:fin], hzCuerpos(cod), 1)
	if !regexp.MustCompile("\\smin\\b|[\"'`]min\\b").MatchString(al) {
		return fmt.Sprintf("ab487d CA-739: haceAh dice minutos (hace <n> min):\n%s", al)
	}
	cero := regexp.MustCompile("[\"'`]0\\s?min\\b").MatchString(al)
	segundos := regexp.MustCompile("\\}\\s?s`|[\"'`]\\s?s[\"'`]|\\}\\s?seg\\b|[\"'`]\\s?seg\\.?[\"'`]").MatchString(al)
	tope := regexp.MustCompile(`Math\.max\(\s*0\s*,|Math\.max\([^()]*,\s*0\s*\)|<=?\s*0\b|\b0\s*>`).MatchString(al)
	if cero || (!segundos && tope) {
		return ""
	}
	return fmt.Sprintf("ab487d: un running con started_at en el futuro (reloj corrido) dice hace 0 min, en la banda, En curso y la Tarjeta: haceAh lo dice en segundos (hace 0 s) o no topea la diferencia:\n%s", al)
}

// ---------------------------------------------------------------------------
// Hallazgos 20261117T203622_2c8a7a y 20261117T204141_815a53 (medium, el mismo
// defecto). CA-742 y el spec, "La pantalla Tarjeta": "Cualquier otro error
// muestra no se pudo leer la tarjeta: <error> y reintenta". pedirFicha pide
// /api/board/<slug> y /api/board/<slug>/timeline; un error del timeline se
// muestra como error de la tarjeta, igual que uno del detalle.

// hzTraga: el texto h de un manejador de error (el argumento de .catch o
// el segundo de .then) no usa el error ni lo relanza.
func hzTraga(h string) bool {
	h = strings.TrimSpace(h)
	if h == "" {
		return true
	}
	var param, cuerpo string
	if mm := regexp.MustCompile(`^(?:async\s*)?\(\s*\)\s*=>([\s\S]*)$`).FindStringSubmatch(h); mm != nil {
		cuerpo = mm[1]
	} else if mm := regexp.MustCompile(`^(?:async\s*)?\(?\s*([A-Za-z_$][\w$]*)\s*\)?\s*=>([\s\S]*)$`).FindStringSubmatch(h); mm != nil {
		param, cuerpo = mm[1], mm[2]
	} else if mm := regexp.MustCompile(`^(?:async\s+)?function\b\s*[\w$]*\s*\(\s*([\w$]*)\s*\)([\s\S]*)$`).FindStringSubmatch(h); mm != nil {
		param, cuerpo = mm[1], mm[2]
	} else {
		return false // una funcion con nombre u otra forma: recibe el error
	}
	if huPalabra(cuerpo, "throw") {
		return false
	}
	return param == "" || !huPalabra(cuerpo, param)
}

// hzArgumentos parte el contenido de una llamada (entre abre y su cierre) en
// sus argumentos de primer nivel.
func hzArgumentos(js string, m []bool, abre int) []string {
	cierre := hzCierre(js, m, abre)
	if cierre < 0 {
		return nil
	}
	var out []string
	ini, nivel := abre+1, 0
	for k := abre + 1; k < cierre; k++ {
		if !m[k] {
			continue
		}
		switch js[k] {
		case '(', '[', '{':
			nivel++
		case ')', ']', '}':
			nivel--
		case ',':
			if nivel == 0 {
				out = append(out, js[ini:k])
				ini = k + 1
			}
		}
	}
	return append(out, js[ini:cierre])
}

var (
	hzRePromCatch = regexp.MustCompile(`\.\s*catch\s*\(`)
	hzRePromThen  = regexp.MustCompile(`\.\s*then\s*\(`)
	hzReTryCatch  = regexp.MustCompile(`(?:^|[^.\w$])catch\s*(?:\(\s*([A-Za-z_$][\w$]*)\s*\))?\s*\{`)
)

// hzCatchTraga: el bloque catch (param) { cuerpo } no usa el error ni lo
// relanza.
func hzCatchTraga(param, cuerpo string) bool {
	if huPalabra(cuerpo, "throw") {
		return false
	}
	return param == "" || !huPalabra(cuerpo, param)
}

// hzInicioSentencia: donde empieza la sentencia en la que esta c (despues
// del ; o del bloque anterior del mismo nivel, o del que la encierra).
func hzInicioSentencia(js string, m []bool, c int) int {
	pila := hzAbiertos(js, m, 0, c)
	o := -1
	if len(pila) > 0 {
		o = pila[len(pila)-1]
	}
	ini, nivel := o+1, 0
	for k := o + 1; k < c; k++ {
		if !m[k] {
			continue
		}
		switch js[k] {
		case '(', '[', '{':
			nivel++
		case ')', ']', '}':
			nivel--
			if nivel == 0 && js[k] == '}' {
				ini = k + 1
			}
		case ';':
			if nivel == 0 {
				ini = k + 1
			}
		}
	}
	return ini
}

var (
	hzReAllSettled     = regexp.MustCompile(`\bPromise\s*\.\s*allSettled\s*\(\s*\[`)
	hzReLadoAwait      = regexp.MustCompile(`(\[[^\]]*\]|[A-Za-z_$][\w$]*)\s*=\s*await\s*$`)
	hzReThenAllSettled = regexp.MustCompile(`^\s*\)\s*\.\s*then\s*\(\s*(?:async\s*)?(?:\(\s*(\[[^\]]*\])\s*\)|\(?\s*([A-Za-z_$][\w$]*)\s*\)?)\s*=>`)
)

// hzFuncionQueContiene: el cuerpo [ini, fin) de la funcion con nombre mas
// interna cuyo cuerpo contiene p.
func hzFuncionQueContiene(js string, m []bool, p int) (int, int, bool) {
	mejor, mIni, mFin := false, 0, 0
	for _, d := range atDeclaracion.FindAllStringSubmatch(js, -1) {
		if ini, fin, ok := hzCuerpo(js, m, d[1]+d[2]); ok && ini <= p && p < fin && (!mejor || ini > mIni) {
			mejor, mIni, mFin = true, ini, fin
		}
	}
	return mIni, mFin, mejor
}

// hzManejadoPorAllSettled: el pedido de tl va dentro de un
// Promise.allSettled([...]) y su resultado, en esa posicion, se lee con
// .reason dentro de la misma funcion. El resultado se toma de
// const [d, h] = await Promise.allSettled(...), de const rs = await ... (con
// rs[i].reason o recorriendo rs con .reason) o del parametro del .then que
// sigue (([d, h]) => ... o rs => ...).
func hzManejadoPorAllSettled(js string, m []bool, tl int) bool {
	for _, a := range hzReAllSettled.FindAllStringIndex(js, -1) {
		corchete := a[1] - 1
		cierre := hzCierre(js, m, corchete)
		if !m[corchete] || cierre < 0 || tl < corchete || tl > cierre {
			continue
		}
		// la posicion de tl en el arreglo: las comas de primer nivel antes
		pos, nivel := 0, 0
		for k := corchete + 1; k < tl; k++ {
			if !m[k] {
				continue
			}
			switch js[k] {
			case '(', '[', '{':
				nivel++
			case ')', ']', '}':
				nivel--
			case ',':
				if nivel == 0 {
					pos++
				}
			}
		}
		var lado string
		if l := hzReLadoAwait.FindStringSubmatch(strings.TrimRight(js[max(0, a[0]-200):a[0]], " \t\r\n")); l != nil {
			lado = l[1]
		} else if th := hzReThenAllSettled.FindStringSubmatch(js[cierre+1:]); th != nil {
			lado = th[1] + th[2]
		}
		if lado == "" {
			continue
		}
		_, fin, ok := hzFuncionQueContiene(js, m, a[0])
		if !ok {
			continue
		}
		despues := js[a[0]:fin]
		if strings.HasPrefix(lado, "[") {
			nombres := strings.Split(strings.Trim(lado, "[]"), ",")
			if pos >= len(nombres) {
				continue
			}
			nombre := hzReIdent.FindString(strings.TrimSpace(nombres[pos]))
			if nombre == "" || strings.HasPrefix(strings.TrimSpace(nombres[pos]), "...") {
				continue
			}
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(nombre) + `\s*\??\.\s*reason\b`).MatchString(despues) {
				return true
			}
			continue
		}
		q := regexp.QuoteMeta(lado)
		indice := regexp.MustCompile(`\b` + q + `\s*\[\s*` + fmt.Sprint(pos) + `\s*\]\s*\??\.\s*reason\b`)
		recorre := regexp.MustCompile(`\b` + q + `\s*\??\.\s*(?:find|filter|some|map|forEach|reduce)\s*\(|for\s*\(\s*(?:const|let|var)\s+[^;)]*\bof\s+` + q + `\b`)
		if indice.MatchString(despues) || (recorre.MatchString(despues) && regexp.MustCompile(`\.\s*reason\b`).MatchString(despues)) {
			return true
		}
	}
	return false
}

// Hallazgos 20261117T203622_2c8a7a y 20261117T204141_815a53, CA-742: en
// pedirFicha (con sus ayudantes y la funcion que pide el timeline) ningun
// error se traga (.catch(() => null), .catch(e => ({...})) sin usar e, un
// segundo argumento de .then que no lo usa, ni un catch {} vacio), y el
// pedido de /timeline esta cubierto por un manejador que usa el error: un
// .catch de su misma cadena (Promise.all([...]).then(...).catch(e => ...)),
// el catch de un try que lo encierra, un .catch en quien llama a pedirFicha,
// o un Promise.allSettled([...]) cuyo resultado en la posicion del timeline
// se lee con .reason en la misma funcion.
func TestHallazgo_2c8a7a_815a53_CA742_ErrorDelTimelineEsErrorDeLaTarjeta(t *testing.T) {
	_, ahora, _ := hzUI(t, "2c8a7a")
	if msg := hzRevisar2c8a7a(ahora); msg != "" {
		t.Fatal(msg)
	}
}

func hzRevisar2c8a7a(ahora string) string {
	cod := stCodigo(ahora)
	m := hzMascara(cod)
	ini, fin, ok := hzCuerpo(cod, m, "pedirFicha")
	if !ok {
		return "2c8a7a CA-742: fixture: ahora.js declara pedirFicha, la que pide /api/board/<slug> y su /timeline"
	}
	tls := regexp.MustCompile(`/timeline`).FindAllStringIndex(cod, -1)
	if len(tls) == 0 {
		return "2c8a7a CA-742: fixture: la Tarjeta pide /api/board/<slug>/timeline"
	}

	// donde se mira: pedirFicha, sus ayudantes de ahora.js y la funcion que
	// pide el timeline
	type tramo struct{ ini, fin int }
	tramos := []tramo{{ini, fin}}
	conTry := []tramo{{ini, fin}}
	declAhora := hzCuerpos(cod)
	for _, mm := range regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\s*\(`).FindAllStringSubmatch(cod[ini:fin], -1) {
		if _, ok := declAhora[mm[1]]; ok && mm[1] != "pedirFicha" {
			if a, b, ok := hzCuerpo(cod, m, mm[1]); ok {
				tramos = append(tramos, tramo{a, b})
			}
		}
	}
	for _, u := range tls {
		if a, b, ok := hzFuncionQueContiene(cod, m, u[0]); ok {
			tramos = append(tramos, tramo{a, b})
			conTry = append(conTry, tramo{a, b})
		}
	}

	for _, tr := range tramos {
		for _, c := range hzRePromCatch.FindAllStringIndex(cod[tr.ini:tr.fin], -1) {
			abre := tr.ini + c[1] - 1
			if !m[abre] {
				continue
			}
			if args := hzArgumentos(cod, m, abre); len(args) > 0 && hzTraga(args[0]) {
				return fmt.Sprintf("2c8a7a/815a53 CA-742: un error de /api/board/<slug> o de su /timeline muestra no se pudo leer la tarjeta: <error> y reintenta: este .catch se lo traga (no usa el error ni lo relanza):\n...%s...",
					hzCorte(cod, tr.ini+c[0]))
			}
		}
		for _, c := range hzRePromThen.FindAllStringIndex(cod[tr.ini:tr.fin], -1) {
			abre := tr.ini + c[1] - 1
			if !m[abre] {
				continue
			}
			if args := hzArgumentos(cod, m, abre); len(args) >= 2 && hzTraga(args[1]) {
				return fmt.Sprintf("2c8a7a/815a53 CA-742: el segundo argumento de este .then se traga el error (no lo usa ni lo relanza):\n...%s...",
					hzCorte(cod, tr.ini+c[0]))
			}
		}
		if regexp.MustCompile(`\ballSettled\b`).MatchString(cod[tr.ini:tr.fin]) && !regexp.MustCompile(`\breason\b`).MatchString(cod[tr.ini:tr.fin]) {
			return ("2c8a7a/815a53 CA-742: con Promise.allSettled, un pedido rechazado (su reason) se muestra como no se pudo leer la tarjeta: <error>")
		}
	}
	for _, tr := range conTry {
		for _, c := range hzReTryCatch.FindAllStringSubmatchIndex(cod[tr.ini:tr.fin], -1) {
			llave := tr.ini + c[1] - 1
			if !m[llave] {
				continue
			}
			param := ""
			if c[2] >= 0 {
				param = cod[tr.ini+c[2] : tr.ini+c[3]]
			}
			if cierre := hzCierre(cod, m, llave); cierre > 0 && hzCatchTraga(param, cod[llave+1:cierre]) {
				return fmt.Sprintf("2c8a7a/815a53 CA-742: este catch se traga el error de la Tarjeta (no lo usa ni lo relanza):\n...%s...",
					hzCorte(cod, llave))
			}
		}
	}

	// cada pedido del timeline llega a un manejador que usa el error
	cubierto := func(tl int) bool {
		for _, c := range hzRePromCatch.FindAllStringIndex(cod, -1) {
			abre := c[1] - 1
			if c[0] <= tl || !m[abre] {
				continue
			}
			if args := hzArgumentos(cod, m, abre); len(args) > 0 && !hzTraga(args[0]) && hzInicioSentencia(cod, m, c[0]) <= tl {
				return true
			}
		}
		for _, tr := range regexp.MustCompile(`\btry\s*\{`).FindAllStringIndex(cod, -1) {
			llave := tr[1] - 1
			cierre := hzCierre(cod, m, llave)
			if !m[llave] || cierre < 0 || tl < llave || tl > cierre {
				continue
			}
			if c := regexp.MustCompile(`^\s*catch\s*(?:\(\s*([A-Za-z_$][\w$]*)\s*\))?\s*\{`).FindStringSubmatchIndex(cod[cierre+1:]); c != nil {
				param, l2 := "", cierre+1+c[1]-1
				if c[2] >= 0 {
					param = cod[cierre+1+c[2] : cierre+1+c[3]]
				}
				if c2 := hzCierre(cod, m, l2); c2 > 0 && !hzCatchTraga(param, cod[l2+1:c2]) {
					return true
				}
			}
		}
		for _, c := range regexp.MustCompile(`\bpedirFicha\s*\([^()]*\)\s*\.\s*catch\s*\(`).FindAllStringIndex(cod, -1) {
			if args := hzArgumentos(cod, m, c[1]-1); len(args) > 0 && !hzTraga(args[0]) {
				return true
			}
		}
		return hzManejadoPorAllSettled(cod, m, tl)
	}
	for _, u := range tls {
		if !cubierto(u[0]) {
			return fmt.Sprintf("2c8a7a/815a53 CA-742: un error de /api/board/<slug>/timeline (un 500) muestra no se pudo leer la tarjeta: <error>, igual que uno del detalle: este pedido no llega a ningun manejador que use el error (un .catch de su cadena o el catch de su try):\n...%s...",
				hzCorte(cod, u[0]))
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Hallazgo 20261117T204141_ed5369 (medium). CA-741 y CA-736 (la Tarjeta de
// <slug> pinta la tarjeta de su hash, con el sondeo encadenado): al pasar de
// A a B con A lento, la respuesta tardia de A no repuebla el estado de la
// Tarjeta. En pedirFicha la comprobacion de generacion (ah.genF) va antes de
// cualquier asignacion a ah.det, ah.hist, ah.ferr o ah.no, en el camino
// bueno y en el del error.

var (
	hzReAsignaFicha = regexp.MustCompile(`\bah\s*\.\s*(?:det|hist|ferr|no)\s*(?:=[^=>]|\|\|=|\?\?=|&&=)|\bObject\.assign\(\s*ah\s*,`)
	hzReGeneracion  = regexp.MustCompile(`(?:===|!==|==|!=|<=|>=|[^=<>]<|[^=<>]>)\s*(?:ah\s*\.\s*)?genF\b|\bgenF\s*(?:===|!==|==|!=|<=|>=|<|>)`)
	hzReDeFuncion   = regexp.MustCompile(`\bfunction\b\s*[\w$]*\s*\([^()]*\)$`)
	hzReDeCatch     = regexp.MustCompile(`(?:\bcatch\s*\(\s*[\w$]*\s*\)|\bcatch|\bfinally)$`)
	hzReDePromesa   = regexp.MustCompile(`\.\s*(?:then|catch|finally)$`)
	hzReAwait       = regexp.MustCompile(`\bawait\b`)
	hzReLlamada     = regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\s*\(`)
	hzReReferencia  = regexp.MustCompile(`\.\s*(?:then|catch|finally)\(\s*([A-Za-z_$][\w$]*)\s*[,)]`)
)

// hzComprueba: texto compara la generacion (genF), directo o con un
// ayudante de decl que la compara.
func hzComprueba(texto string, decl map[string]string) bool {
	if hzReGeneracion.MatchString(texto) {
		return true
	}
	for _, mm := range hzReLlamada.FindAllStringSubmatch(texto, -1) {
		if c, ok := decl[mm[1]]; ok && hzReGeneracion.MatchString(c) {
			return true
		}
	}
	return false
}

// hzUltimoAwait: el ultimo await de codigo en [desde, hasta)
// (-1 si no hay).
func hzUltimoAwait(js string, m []bool, desde, hasta int) int {
	u := -1
	for _, a := range hzReAwait.FindAllStringIndex(js[desde:hasta], -1) {
		if m[desde+a[0]] {
			u = desde + a[0]
		}
	}
	return u
}

// hzInicioManejador: donde empieza el manejador mas interno que encierra p
// dentro del cuerpo [ini, ...): el cuerpo de una flecha o de una funcion, el
// argumento de .then/.catch/.finally, o un bloque catch/finally despues de
// un await. Si no hay ninguno, el cuerpo mismo (cuerpo = true).
func hzInicioManejador(js string, m []bool, ini, p int) (int, bool) {
	pila := hzAbiertos(js, m, ini, p)
	for i := len(pila) - 1; i >= 0; i-- {
		b := pila[i]
		antes := strings.TrimRight(js[max(ini, b-200):b], " \t\r\n")
		switch js[b] {
		case '{':
			if strings.HasSuffix(antes, "=>") || hzReDeFuncion.MatchString(antes) {
				return b + 1, false
			}
			if hzReDeCatch.MatchString(antes) && hzUltimoAwait(js, m, ini, b) >= 0 {
				return b + 1, false
			}
		case '(':
			if strings.HasSuffix(antes, "=>") || hzReDePromesa.MatchString(antes) {
				return b + 1, false
			}
		}
	}
	return ini, true
}

// Hallazgo 20261117T204141_ed5369, CA-741 y CA-736: cada asignacion a ah.det,
// ah.hist, ah.ferr o ah.no que hace pedirFicha (o un ayudante al que le pasa
// la respuesta) con una respuesta en la mano, en el .then, en el .catch o
// despues de un await, tiene antes, en el mismo manejador, una comparacion
// de la generacion (g !== ah.genF, o un ayudante que la hace). Las que hace
// antes de pedir, sin esperar nada, no la necesitan.
func TestHallazgo_ed5369_CA741_RespuestaTardiaNoRepueblaLaTarjeta(t *testing.T) {
	_, ahora, _ := hzUI(t, "ed5369")
	if msg := hzRevisared5369(ahora); msg != "" {
		t.Fatal(msg)
	}
}

func hzRevisared5369(ahora string) string {
	cod := stCodigo(ahora)
	m := hzMascara(cod)
	ini, fin, ok := hzCuerpo(cod, m, "pedirFicha")
	if !ok {
		return "ed5369 CA-741: fixture: ahora.js declara pedirFicha, la que pide /api/board/<slug> y su /timeline"
	}
	decl := hzCuerpos(cod)

	asignaciones := 0
	// revisar mira las asignaciones de [a, b); conRespuesta: todas llegan con
	// una respuesta (un ayudante llamado desde un manejador sin comprobar).
	revisar := func(a, b int, donde string, conRespuesta bool) string {
		for _, as := range hzReAsignaFicha.FindAllStringIndex(cod[a:b], -1) {
			p := a + as[0]
			if !m[p] {
				continue
			}
			asignaciones++
			s, esCuerpo := hzInicioManejador(cod, m, a, p)
			w := hzUltimoAwait(cod, m, s, p)
			if esCuerpo && w < 0 && !conRespuesta {
				continue // antes de pedir: todavia no hay respuesta
			}
			desde := s
			if w >= 0 {
				desde = w
			}
			if !hzComprueba(cod[desde:p], decl) {
				return fmt.Sprintf("ed5369 CA-741: al pasar de A a B con A lento, la respuesta tardia de A no repuebla la Tarjeta B: en %s, esta asignacion llega con una respuesta y antes, en su manejador, no se comprueba que sigue siendo de la generacion visible (ah.genF):\n...%s...",
					donde, hzCorte(cod, p))
			}
		}
		return ""
	}
	if msg := revisar(ini, fin, "pedirFicha", false); msg != "" {
		return msg
	}

	// los ayudantes a los que pedirFicha les pasa la respuesta sin haber
	// comprobado antes la generacion
	contados := map[string]bool{}
	var sinComprobar []string
	llamadas := append(hzReLlamada.FindAllStringSubmatchIndex(cod[ini:fin], -1), hzReReferencia.FindAllStringSubmatchIndex(cod[ini:fin], -1)...)
	for _, ll := range llamadas {
		n := cod[ini+ll[2] : ini+ll[3]]
		if _, ok := decl[n]; !ok || n == "pedirFicha" {
			continue
		}
		a, b, ok := hzCuerpo(cod, m, n)
		if !ok {
			continue
		}
		if !contados[n] {
			contados[n] = true
			asignaciones += len(hzReAsignaFicha.FindAllStringIndex(cod[a:b], -1))
		}
		q := ini + ll[2]
		s, esCuerpo := hzInicioManejador(cod, m, ini, q)
		w := hzUltimoAwait(cod, m, s, q)
		if esCuerpo && w < 0 {
			continue // la llama antes de pedir
		}
		desde := s
		if w >= 0 {
			desde = w
		}
		if hzComprueba(cod[desde:q], decl) {
			continue // ya comprobo antes de llamarla
		}
		sinComprobar = append(sinComprobar, n)
	}
	revisados := map[string]bool{}
	for _, n := range sinComprobar {
		if revisados[n] {
			continue
		}
		revisados[n] = true
		a, b, _ := hzCuerpo(cod, m, n)
		if msg := revisar(a, b, n+" (llamada desde pedirFicha con la respuesta)", true); msg != "" {
			return msg
		}
	}
	if asignaciones == 0 {
		return "ed5369 CA-741: fixture: pedirFicha (o un ayudante suyo) guarda la respuesta en ah.det, ah.hist, ah.ferr o ah.no"
	}
	return ""
}
