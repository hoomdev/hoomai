package tablero

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// signos are the signs of the screen and the TUI's own words that change
// between UTF-8 and ASCII (see the package doc).
type signos struct {
	hecho, falta, noAplica, decision           string
	conPrueba, sinPrueba, porComando           string
	sel, hechoCerrado, hechoAbierto            string
	separador                                  string
	esqSupIzq, esqSupDer, esqInfIzq, esqInfDer string
	lado, horizontal                           string
	flechaIzq, flechaDer, cursor, flecha       string
	corte                                      string
	// the TUI's own words with accents
	decisionTitulo, narro, ultimo string
}

var signosUTF8 = signos{
	hecho: "●", falta: "○", noAplica: "·", decision: "✋",
	conPrueba: "✓", sinPrueba: "✗", porComando: "⚑",
	sel: "▸", hechoCerrado: "▸", hechoAbierto: "▾",
	separador: "─",
	esqSupIzq: "┌", esqSupDer: "┐", esqInfIzq: "└", esqInfDer: "┘",
	lado: "│", horizontal: "─",
	flechaIzq: "‹", flechaDer: "›", cursor: "▌", flecha: "→",
	corte:          "…",
	decisionTitulo: "NECESITA TU DECISIÓN", narro: "narró", ultimo: "último",
}

var signosASCII = signos{
	hecho: "#", falta: "-", noAplica: ".", decision: "!",
	conPrueba: "+", sinPrueba: "x", porComando: "v",
	sel: ">", hechoCerrado: ">", hechoAbierto: "v",
	separador: "-",
	esqSupIzq: "+", esqSupDer: "+", esqInfIzq: "+", esqInfDer: "+",
	lado: "|", horizontal: "-",
	flechaIzq: "<", flechaDer: ">", cursor: "_", flecha: "->",
	corte:          "~",
	decisionTitulo: "NECESITA TU DECISION", narro: "narro", ultimo: "ultimo",
}

// estilo is how a piece of a line is drawn when the screen has color.
type estilo int

const (
	normal estilo = iota
	negrita
	rojo
	verde
	gris
)

// sgr is the SGR sequence of each style (the 16 colors of the terminal).
var sgr = map[estilo]string{negrita: "\x1b[1m", rojo: "\x1b[31m", verde: "\x1b[32m", gris: "\x1b[90m"}

// tramo is a piece of a line with one style.
type tramo struct {
	texto  string
	estilo estilo
}

// linea is one line of the screen, in pieces.
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

// cortada is the line cut to w cells (its last cell corte when it did not
// fit) and without its trailing spaces.
func (l linea) cortada(w int, corte string) linea {
	if w <= 0 {
		return nil
	}
	total := 0
	for _, t := range l {
		total += celdas(t.texto)
	}
	var out linea
	if total <= w {
		out = append(out, l...)
	} else {
		resto := w - celdas(corte)
		for _, t := range l {
			if resto <= 0 {
				break
			}
			r := []rune(t.texto)
			if len(r) > resto {
				r = r[:resto]
			}
			out = append(out, tramo{texto: string(r), estilo: t.estilo})
			resto -= len(r)
		}
		e := normal
		if len(out) > 0 {
			e = out[len(out)-1].estilo
		}
		out = append(out, tramo{texto: corte, estilo: e})
	}
	// trailing spaces off
	for len(out) > 0 {
		t := &out[len(out)-1]
		t.texto = strings.TrimRight(t.texto, " ")
		if t.texto != "" {
			break
		}
		out = out[:len(out)-1]
	}
	return out
}

// dibujar is the line as Pantalla returns it: cut to w, with SGR sequences
// when color.
func (l linea) dibujar(w int, corte string, color bool) string {
	c := l.cortada(w, corte)
	if !color {
		return c.plano()
	}
	var b strings.Builder
	for _, t := range c {
		if s, ok := sgr[t.estilo]; ok && t.texto != "" {
			b.WriteString(s + t.texto + "\x1b[0m")
			continue
		}
		b.WriteString(t.texto)
	}
	return b.String()
}

// celdas is how many cells s takes: one per rune (spec, Riesgos).
func celdas(s string) int { return utf8.RuneCountInString(s) }

// cortar cuts s to w cells, its last cell corte when it did not fit.
func cortar(s string, w int, corte string) string {
	if w <= 0 {
		return ""
	}
	if celdas(s) <= w {
		return s
	}
	r := []rune(s)
	n := w - celdas(corte)
	if n < 0 {
		n = 0
	}
	return string(r[:n]) + corte
}

// rellenar pads s with spaces to w cells (a longer s goes whole).
func rellenar(s string, w int) string {
	if n := w - celdas(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// envolverPalabras wraps s to lines of at most w cells at the spaces
// (greedy); a word longer than the line is cut hard.
func envolverPalabras(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	var out []string
	for _, parrafo := range strings.Split(s, "\n") {
		var actual []rune
		for _, palabra := range strings.Fields(parrafo) {
			p := []rune(palabra)
			for len(p) > w {
				if len(actual) > 0 {
					out = append(out, string(actual))
					actual = nil
				}
				out = append(out, string(p[:w]))
				p = p[w:]
			}
			switch {
			case len(p) == 0:
			case len(actual) == 0:
				actual = p
			case len(actual)+1+len(p) <= w:
				actual = append(append(actual, ' '), p...)
			default:
				out = append(out, string(actual))
				actual = p
			}
		}
		out = append(out, string(actual))
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
		r := []rune(parrafo)
		if len(r) == 0 {
			out = append(out, "")
			continue
		}
		for len(r) > w {
			out = append(out, string(r[:w]))
			r = r[w:]
		}
		out = append(out, string(r))
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

// unaLinea is s with its newlines as spaces.
func unaLinea(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}
