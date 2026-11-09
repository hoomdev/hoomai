package tablero

import (
	"strings"
	"unicode"
)

// signos are the signs of the screen and the TUI's own words that change
// between UTF-8 and ASCII (see the package doc).
type signos struct {
	hecho, falta, noAplica, decision string
	conPrueba, sinPrueba, porComando string
	sel, hechoCerrado, hechoAbierto  string
	// the simple box and the focus box: corners, side, line
	esqSI, esqSD, esqII, esqID, lado, linea       string
	fEsqSI, fEsqSD, fEsqII, fEsqID, fLado, fLinea string
	flechaIzq, flechaDer, cursor, corte, puntos   string
	// the arrows of the keys line and the help, in that order: ↑ ↓ ← →
	flechas [4]string
	// the TUI's own words with accents
	decisionTitulo, queFalta, tiempoLimite, narracion string
	escribio, reviso, narro, ultimo                   string
}

var signosUTF8 = signos{
	hecho: "●", falta: "○", noAplica: "·", decision: "✋",
	conPrueba: "✓", sinPrueba: "✗", porComando: "⚑",
	sel: "▸", hechoCerrado: "▸", hechoAbierto: "▾",
	esqSI: "┌", esqSD: "┐", esqII: "└", esqID: "┘", lado: "│", linea: "─",
	fEsqSI: "╔", fEsqSD: "╗", fEsqII: "╚", fEsqID: "╝", fLado: "║", fLinea: "═",
	flechaIzq: "‹", flechaDer: "›", cursor: "▌", corte: "…", puntos: "…",
	flechas:        [4]string{"↑", "↓", "←", "→"},
	decisionTitulo: "NECESITA TU DECISIÓN", queFalta: "¿Qué falta?", tiempoLimite: "Tiempo límite",
	narracion: "Narración", escribio: "escribió", reviso: "revisó", narro: "narró", ultimo: "último",
}

var signosASCII = signos{
	hecho: "#", falta: "-", noAplica: ".", decision: "!",
	conPrueba: "+", sinPrueba: "x", porComando: "v",
	sel: ">", hechoCerrado: ">", hechoAbierto: "v",
	esqSI: "+", esqSD: "+", esqII: "+", esqID: "+", lado: "|", linea: "-",
	fEsqSI: "#", fEsqSD: "#", fEsqII: "#", fEsqID: "#", fLado: "#", fLinea: "=",
	flechaIzq: "<", flechaDer: ">", cursor: "_", corte: "~", puntos: "...",
	flechas:        [4]string{"^", "v", "<", ">"},
	decisionTitulo: "NECESITA TU DECISION", queFalta: "Que falta?", tiempoLimite: "Tiempo limite",
	narracion: "Narracion", escribio: "escribio", reviso: "reviso", narro: "narro", ultimo: "ultimo",
}

// conFlechas is s (a Muestra or a Barra of the table, written with UTF-8
// arrows) with the arrows of the mode.
func (s signos) conFlechas(t string) string {
	return strings.NewReplacer("↑", s.flechas[0], "↓", s.flechas[1], "←", s.flechas[2], "→", s.flechas[3]).Replace(t)
}

// enPantalla is a command as the screen writes it: in ASCII the "…" of
// tarjetacmd.CitarPedido's cut is the ASCII cut sign ("~"); with UTF-8 it
// stays. The order's Args and the Registro's Comando keep it.
func (s signos) enPantalla(comando string) string {
	return strings.ReplaceAll(comando, "…", s.corte)
}

// estilo is how a piece of a line is drawn when the screen has color: bold
// and reverse, and one color.
type estilo uint8

const (
	normal   estilo = 0
	negrita  estilo = 1
	inverso  estilo = 2
	rojo     estilo = 1 << 2
	verde    estilo = 2 << 2
	amarillo estilo = 3 << 2
	cian     estilo = 4 << 2
	gris     estilo = 5 << 2
	colores  estilo = 7 << 2

	estiloFoco = negrita | verde
)

// sgr is the sequence of the style: its codes joined by ";" (1, 7, then
// the color), "" for normal.
func (e estilo) sgr() string {
	var c []string
	if e&negrita != 0 {
		c = append(c, "1")
	}
	if e&inverso != 0 {
		c = append(c, "7")
	}
	switch e & colores {
	case rojo:
		c = append(c, "31")
	case verde:
		c = append(c, "32")
	case amarillo:
		c = append(c, "33")
	case cian:
		c = append(c, "36")
	case gris:
		c = append(c, "90")
	}
	if len(c) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(c, ";") + "m"
}

// tramo is a piece of a line with one style.
type tramo struct {
	texto  string
	estilo estilo
}

// linea is one line of a panel or a modal, in pieces.
type linea []tramo

func lin(s string) linea              { return linea{{texto: s}} }
func linCon(s string, e estilo) linea { return linea{{texto: s, estilo: e}} }
func (l linea) mas(s string) linea    { return append(l, tramo{texto: s}) }
func (l linea) masCon(s string, e estilo) linea {
	return append(l, tramo{texto: s, estilo: e})
}

// plano is the line's text without styles.
func (l linea) plano() string {
	var b strings.Builder
	for _, t := range l {
		b.WriteString(t.texto)
	}
	return b.String()
}

// todaCon is the line with every piece in style e.
func (l linea) todaCon(e estilo) linea {
	out := make(linea, len(l))
	for i, t := range l {
		out[i] = tramo{texto: t.texto, estilo: e}
	}
	return out
}

// limpia is the line with no character that the terminal would take as a
// command: the texts come from the project and from the agents' narration
// (titles, specs, findings, events), and an ESC, a C1 control or a bidi
// override there would run as a sequence or reorder the line. A tab is a
// space; any other control, a ?.
func (l linea) limpia() linea {
	out := make(linea, len(l))
	for i, t := range l {
		out[i] = tramo{texto: limpio(t.texto), estilo: t.estilo}
	}
	return out
}

func limpio(s string) string { return strings.Map(neutro, s) }

func neutro(r rune) rune {
	switch {
	case r == '\t':
		return ' '
	case unicode.IsControl(r), unicode.Is(unicode.Bidi_Control, r):
		return '?'
	}
	return r
}

// celdas is how many cells s takes (Celdas).
func celdas(s string) int { return Celdas(s) }

// prefijo is the longest start of s that takes at most w cells.
func prefijo(s string, w int) string {
	n := 0
	for i, r := range s {
		a := AnchoDe(r)
		if n+a > w {
			return s[:i]
		}
		n += a
	}
	return s
}

// cortar cuts s to w cells: when it does not fit, the longest start that
// fits in w less the cut sign's cells, then the cut sign.
func cortar(s string, w int, corte string) string {
	if w <= 0 {
		return ""
	}
	if celdas(s) <= w {
		return s
	}
	n := w - celdas(corte)
	if n < 0 {
		return prefijo(corte, w)
	}
	return rellenar(prefijo(s, n)+corte, w)
}

// cortada is the line cut to w cells (its last cell the cut sign when it
// did not fit), the cut sign in the style of the last piece kept.
func (l linea) cortada(w int, corte string) linea {
	if w <= 0 {
		return nil
	}
	total := 0
	for _, t := range l {
		total += celdas(t.texto)
	}
	if total <= w {
		return l
	}
	resto := w - celdas(corte)
	var out linea
	for _, t := range l {
		if resto <= 0 {
			break
		}
		p := prefijo(t.texto, resto)
		if p == "" {
			break
		}
		out = append(out, tramo{texto: p, estilo: t.estilo})
		resto -= celdas(p)
		if p != t.texto {
			break
		}
	}
	e := normal
	if len(out) > 0 {
		e = out[len(out)-1].estilo
	} else if len(l) > 0 {
		e = l[0].estilo
	}
	out = append(out, tramo{texto: corte, estilo: e})
	// a wide character that did not fit leaves its cell as a space
	if n := w - celdas(out.plano()); n > 0 {
		out = append(out, tramo{texto: strings.Repeat(" ", n), estilo: e})
	}
	return out
}

// rellenar pads s with spaces to w cells (a longer s goes whole).
func rellenar(s string, w int) string {
	if n := w - celdas(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// envolverPalabras wraps s to lines of at most w cells at the spaces
// (greedy); a word longer than the line is cut hard. Its newlines are line
// breaks.
func envolverPalabras(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	var out []string
	for _, parrafo := range strings.Split(s, "\n") {
		actual := ""
		for _, palabra := range strings.Fields(parrafo) {
			for celdas(palabra) > w {
				if actual != "" {
					out = append(out, actual)
					actual = ""
				}
				p := prefijo(palabra, w)
				if p == "" {
					p = string([]rune(palabra)[:1])
				}
				out = append(out, p)
				palabra = palabra[len(p):]
			}
			switch {
			case palabra == "":
			case actual == "":
				actual = palabra
			case celdas(actual)+1+celdas(palabra) <= w:
				actual += " " + palabra
			default:
				out = append(out, actual)
				actual = palabra
			}
		}
		out = append(out, actual)
	}
	return out
}

// envolverPalabrasEspacios is envolverPalabras keeping the spaces between
// the words of each line.
func envolverPalabrasEspacios(s string, w int) []string {
	var out []string
	for _, l := range envolverLinea(lin(s), w) {
		out = append(out, l.plano())
	}
	return out
}

// envolverDuro wraps s every w cells, its newlines kept as line breaks.
func envolverDuro(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	var out []string
	for _, parrafo := range strings.Split(s, "\n") {
		if parrafo == "" {
			out = append(out, "")
			continue
		}
		for celdas(parrafo) > w {
			p := prefijo(parrafo, w)
			if p == "" {
				p = string([]rune(parrafo)[:1])
			}
			out = append(out, p)
			parrafo = parrafo[len(p):]
		}
		out = append(out, parrafo)
	}
	return out
}

// envolverLinea wraps a line of [0] (in pieces) to w cells at its spaces
// (greedy), each continuation starting with the line's leading spaces; the
// spaces between the words of one line stay as they were, and a word longer
// than the line is cut hard. Each rune keeps its piece's style.
func envolverLinea(l linea, w int) []linea {
	if w <= 0 {
		return nil
	}
	if celdas(l.plano()) <= w {
		return []linea{l}
	}
	type runa struct {
		s string
		e estilo
	}
	var rs []runa
	for _, t := range l {
		for _, r := range t.texto {
			rs = append(rs, runa{string(r), t.estilo})
		}
	}
	i := 0
	for i < len(rs) && rs[i].s == " " {
		i++
	}
	sangria := strings.Repeat(" ", i)
	if i >= w/2 {
		sangria = ""
	}
	type token struct{ esp, pal []runa }
	var toks []token
	for i < len(rs) {
		var t token
		for i < len(rs) && rs[i].s == " " {
			t.esp = append(t.esp, rs[i])
			i++
		}
		for i < len(rs) && rs[i].s != " " {
			t.pal = append(t.pal, rs[i])
			i++
		}
		if len(t.pal) > 0 {
			toks = append(toks, t)
		}
	}
	ancho := func(x []runa) int {
		n := 0
		for _, r := range x {
			n += celdas(r.s)
		}
		return n
	}
	var out []linea
	var actual linea
	usado := 0
	empezada := false
	poner := func(x []runa) {
		for _, r := range x {
			if n := len(actual); n > 0 && actual[n-1].estilo == r.e {
				actual[n-1].texto += r.s
			} else {
				actual = append(actual, tramo{texto: r.s, estilo: r.e})
			}
			usado += celdas(r.s)
		}
	}
	empezar := func() {
		actual = linea{{texto: sangria}}
		usado = celdas(sangria)
		empezada = true
	}
	cerrar := func() {
		out = append(out, actual)
		actual, usado, empezada = nil, 0, false
	}
	for _, t := range toks {
		if empezada && usado+ancho(t.esp)+ancho(t.pal) > w {
			cerrar()
		}
		if empezada {
			poner(t.esp)
			poner(t.pal)
			continue
		}
		empezar()
		pal := t.pal
		for ancho(pal) > w-usado {
			k := 0
			for k < len(pal) && usado+celdas(pal[k].s) <= w {
				poner(pal[k : k+1])
				k++
			}
			if k == 0 {
				poner(pal[:1])
				k = 1
			}
			pal = pal[k:]
			cerrar()
			empezar()
		}
		poner(pal)
	}
	if empezada {
		cerrar()
	}
	return out
}

// plegar is s in lower case with the accents folded, for the search.
func plegar(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if p, ok := pliegues[r]; ok {
			r = p
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

var pliegues = func() map[rune]rune {
	m := map[rune]rune{}
	for base, con := range map[rune]string{
		'a': "áàâäã", 'e': "éèêë", 'i': "íìîï", 'o': "óòôöõ", 'u': "úùûü", 'n': "ñ", 'c': "ç",
	} {
		for _, r := range con {
			m[r] = base
			m[unicode.ToUpper(r)] = base
		}
	}
	return m
}()

// unaLinea is s with its newlines ("\n", "\r\n") as spaces; a lone "\r" is
// a control, which the screen shows as "?".
func unaLinea(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ").Replace(s)
}
