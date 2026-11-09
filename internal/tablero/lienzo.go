package tablero

import "strings"

// celda is one cell of the screen: its text (one rune, or "" for the second
// cell of a wide one) and its style.
type celda struct {
	s string
	e estilo
}

// lienzo is the screen being drawn, cell by cell: the panels, then the
// modals over them.
type lienzo struct {
	w, h  int
	corte string
	c     [][]celda
}

func nuevoLienzo(w, h int, corte string) *lienzo {
	lz := &lienzo{w: w, h: h, corte: corte, c: make([][]celda, h)}
	for y := range lz.c {
		fila := make([]celda, w)
		for x := range fila {
			fila[x] = celda{s: " "}
		}
		lz.c[y] = fila
	}
	return lz
}

// fijar puts the rune r (its text) at (x, y) and, when it is wide, its
// second cell; a wide rune or half of one it breaks becomes a space.
func (lz *lienzo) fijar(x, y int, r string, ancho int, e estilo) {
	if y < 0 || y >= lz.h || x < 0 || x+ancho > lz.w {
		return
	}
	fila := lz.c[y]
	romper := func(i int) {
		if i < 0 || i >= lz.w {
			return
		}
		if fila[i].s == "" && i > 0 {
			fila[i-1] = celda{s: " ", e: fila[i-1].e}
			fila[i] = celda{s: " ", e: fila[i].e}
		} else if i+1 < lz.w && fila[i+1].s == "" {
			fila[i+1] = celda{s: " ", e: fila[i+1].e}
		}
	}
	for i := x; i < x+ancho; i++ {
		romper(i)
	}
	fila[x] = celda{s: r, e: e}
	if ancho == 2 {
		fila[x+1] = celda{s: "", e: e}
	}
}

// poner writes the line l at (x, y) in w cells: neutralized, cut with the
// cut sign when it does not fit, and, with fondo, padded to w with spaces
// in the style fondo (its padding is drawn: the bar of the chosen row).
// It returns the cells written.
func (lz *lienzo) poner(x, y, w int, l linea, rellenoEstilo estilo, rellenar bool) int {
	if w <= 0 {
		return 0
	}
	l = l.limpia().cortada(w, lz.corte)
	n := 0
	for _, t := range l {
		for _, r := range t.texto {
			a := AnchoDe(r)
			if a == 0 {
				continue
			}
			if n+a > w {
				break
			}
			lz.fijar(x+n, y, string(r), a, t.estilo)
			n += a
		}
	}
	if rellenar {
		for ; n < w; n++ {
			lz.fijar(x+n, y, " ", 1, rellenoEstilo)
		}
	}
	return n
}

// texto writes s at (x, y) in style e, cut to w cells.
func (lz *lienzo) texto(x, y, w int, s string, e estilo) int {
	return lz.poner(x, y, w, linCon(s, e), normal, false)
}

// repetir writes the sign s n times from (x, y) in style e.
func (lz *lienzo) repetir(x, y, n int, s string, e estilo) {
	for i := 0; i < n; i++ {
		lz.fijar(x+i, y, s, 1, e)
	}
}

// enGris draws every cell outside the rectangles fuera in gray alone.
func (lz *lienzo) enGris(fuera ...rect) {
	for y := 0; y < lz.h; y++ {
		for x := 0; x < lz.w; x++ {
			dentro := false
			for _, r := range fuera {
				if r.contiene(x, y) {
					dentro = true
					break
				}
			}
			if !dentro {
				lz.c[y][x].e = gris
			}
		}
	}
}

// lineas are the screen's lines: trailing spaces off, with SGR sequences
// when color (adjacent pieces of one style are one piece).
func (lz *lienzo) lineas(color bool) []string {
	out := make([]string, lz.h)
	for y, fila := range lz.c {
		fin := len(fila)
		for fin > 0 && fila[fin-1].s == " " {
			fin--
		}
		var b strings.Builder
		var tramoTexto strings.Builder
		actual := estilo(255)
		volcar := func() {
			if tramoTexto.Len() == 0 {
				return
			}
			if color && actual.sgr() != "" {
				b.WriteString(actual.sgr() + tramoTexto.String() + "\x1b[0m")
			} else {
				b.WriteString(tramoTexto.String())
			}
			tramoTexto.Reset()
		}
		for _, c := range fila[:fin] {
			if c.s == "" {
				continue
			}
			e := c.e
			if !color {
				e = normal
			}
			if e != actual {
				volcar()
				actual = e
			}
			tramoTexto.WriteString(c.s)
		}
		volcar()
		out[y] = b.String()
	}
	return out
}

// rect is a rectangle of cells: x, y its first cell, w and h its size.
type rect struct{ x, y, w, h int }

func (r rect) contiene(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}
