package tablero

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// zonas are the places of the body: the list and the detail (the package
// doc, "The layout of the body").
type zonas struct {
	lista, detalle rect
	plegada        bool // the list folded to one line
}

// The fixed lines of the screen (from 0).
const (
	yCabecera = 0
	yCaja     = 1
	altoCaja  = 4
	yCuerpo   = 5
)

// cuerpo is the body's height: C = Alto-8.
func (m *Modelo) cuerpo() int { return max(0, m.c.Alto-8) }

// disponer lays the body out.
func (m *Modelo) disponer() zonas {
	w, cu := m.c.Ancho, m.cuerpo()
	if m.ancha() {
		l := w * 2 / 5
		return zonas{lista: rect{0, yCuerpo, l, cu}, detalle: rect{l, yCuerpo, w - l, cu}}
	}
	if m.zona == ZonaDetalle {
		return zonas{lista: rect{0, yCuerpo, w, 1}, detalle: rect{0, yCuerpo + 1, w, cu - 1}, plegada: true}
	}
	tope := max(3, (cu+1)/2)
	n := m.altoNatural(w, tope-2)
	h := max(3, min(n, (cu+1)/2))
	return zonas{lista: rect{0, yCuerpo, w, h}, detalle: rect{0, yCuerpo + h, w, cu - h}}
}

// altoNatural is the list's natural height in one column of w cells: its
// content lines + 2 (the text of an empty list wrapped when it fits in hi
// lines).
func (m *Modelo) altoNatural(w, hi int) int {
	ls, _, _ := m.contenidoLista(rect{0, 0, w, hi + 2})
	return len(ls) + 2
}

// avisoEnvuelto is the text of an empty box, or of one that is reading, for
// an inner width wi and an inner height hi: wrapped at its words when it
// does not fit in one line and the box has the height for it; else one line
// (cut by the box).
func avisoEnvuelto(l linea, wi, hi int) []linea {
	ls := envolverLinea(l, wi)
	if len(ls) <= 1 || len(ls) > hi {
		return []linea{l}
	}
	return ls
}

// ---------------------------------------------------------------- boxes

// marco is a box to draw.
type marco struct {
	r               rect
	foco            bool
	estilo          estilo // the border's style when it has no focus
	titulo, derecha linea
	lineas          []linea
	sel, scroll     int // the chosen line (-1: none) and the first line in view
	pos             string
}

// dibujarMarco draws a box (the package doc, "The boxes").
func (m *Modelo) dibujarMarco(lz *lienzo, b marco) {
	s := m.s
	r := b.r
	if r.w < 2 || r.h < 1 {
		return
	}
	esqSI, esqSD, esqII, esqID, lado, hor := s.esqSI, s.esqSD, s.esqII, s.esqID, s.lado, s.linea
	borde := b.estilo
	if b.foco {
		esqSI, esqSD, esqII, esqID, lado, hor = s.fEsqSI, s.fEsqSD, s.fEsqII, s.fEsqID, s.fLado, s.fLinea
		borde = estiloFoco
	}
	conBorde := func(l linea) linea {
		out := make(linea, len(l))
		for i, t := range l {
			out[i] = t
			if t.estilo == normal {
				out[i].estilo = borde
			}
		}
		return out
	}
	titulo := conBorde(b.titulo).limpia().cortada(r.w-2, s.corte)
	lz.fijar(r.x, r.y, esqSI, 1, borde)
	n := lz.poner(r.x+1, r.y, r.w-2, titulo, normal, false)
	der := conBorde(b.derecha).limpia()
	dw := celdas(der.plano())
	if dw > 0 && n+1+dw+1 <= r.w-2 {
		lz.repetir(r.x+1+n, r.y, r.w-2-n-dw-1, hor, borde)
		lz.poner(r.x+r.w-2-dw, r.y, dw, der, normal, false)
		lz.fijar(r.x+r.w-2, r.y, hor, 1, borde)
	} else {
		lz.repetir(r.x+1+n, r.y, r.w-2-n, hor, borde)
	}
	lz.fijar(r.x+r.w-1, r.y, esqSD, 1, borde)
	hi := r.h - 2
	for i := 0; i < hi; i++ {
		y := r.y + 1 + i
		lz.fijar(r.x, y, lado, 1, borde)
		k := b.scroll + i
		var l linea
		if k >= 0 && k < len(b.lineas) {
			l = b.lineas[k]
		}
		if k == b.sel && b.foco {
			barra := make(linea, len(l))
			for j, t := range l {
				barra[j] = tramo{texto: t.texto, estilo: t.estilo | inverso}
			}
			lz.poner(r.x+1, y, r.w-2, barra, inverso, true)
		} else {
			lz.poner(r.x+1, y, r.w-2, l, normal, true)
		}
		lz.fijar(r.x+r.w-1, y, lado, 1, borde)
	}
	if r.h >= 2 {
		m.borde(lz, r.x, r.y+r.h-1, r.w, esqII, hor, esqID, b.pos, borde)
	}
}

// dibujarPlegada draws a folded box: the line sign, its title and line
// signs up to its width.
func (m *Modelo) dibujarPlegada(lz *lienzo, r rect, titulo linea) {
	lz.fijar(r.x, r.y, m.s.linea, 1, normal)
	n := lz.poner(r.x+1, r.y, r.w-2, titulo, normal, false)
	lz.repetir(r.x+1+n, r.y, r.w-1-n, m.s.linea, normal)
}

// ---------------------------------------------------------------- the header

// cabecera is the header's line (the package doc, "The header").
func (m *Modelo) cabecera() linea {
	w := m.c.Ancho
	izq := linCon("hoom tablero", estiloFoco).mas(" · " + m.c.Proyecto)
	var partes []linea
	if p := m.proyecto; p != nil {
		if p.Git {
			izq = izq.mas(" · rama " + p.Rama)
		} else {
			izq = izq.mas(" · sin git")
		}
		partes = append(partes, m.lineaCheck())
		if p.Hoy.Runs > 0 {
			partes = append(partes, lin(fmt.Sprintf(" · hoy %.2f USD", p.Hoy.CostoUSD)))
		} else {
			partes = append(partes, lin(" · hoy sin trabajos"))
		}
		if m.fijada != "" {
			partes = append(partes, lin(" · fijada: "+m.fijada))
		}
	} else {
		partes = append(partes, lin("leyendo el proyecto..."))
	}
	ancho := func(l linea) int { return celdas(l.limpia().plano()) }
	unir := func() linea {
		var l linea
		for _, p := range partes {
			l = append(l, p...)
		}
		return l
	}
	cabe := func(der linea) bool { return 1+ancho(izq)+2+ancho(der)+1 <= w }
	for !cabe(unir()) && len(partes) > 1 {
		partes = partes[:len(partes)-1]
	}
	der := unir()
	if !cabe(der) {
		if n := w - 4 - ancho(izq); n >= 1 {
			der = der.limpia().cortada(n, m.s.corte)
		} else {
			return append(lin(" "), izq.limpia().cortada(w-2, m.s.corte)...)
		}
	}
	return append(lin(" "), conDerecha(izq, der, w-1)...)
}

// ---------------------------------------------------------------- the box TU SIGUIENTE PASO

// cajaPaso is the box TU SIGUIENTE PASO: its right part, its two lines and
// whether its border is yellow.
func (m *Modelo) cajaPaso() (linea, [2]linea, bool) {
	var ls [2]linea
	if !m.leido {
		ls[0] = lin(" leyendo el tablero...")
		return nil, ls, false
	}
	c, gs := m.tarjetaDeCaja()
	der := lin(fmt.Sprintf("te toca %d · corriendo %d · detenida %d",
		len(gs[GrupoTeToca]), len(gs[GrupoCorriendo]), len(gs[GrupoDetenida])))
	amarilla := len(gs[GrupoTeToca])+len(gs[GrupoDetenida]) > 0
	wi := m.c.Ancho - 2
	switch {
	case c != nil:
		g := m.grupoDe(c)
		p := m.pasoDe(c)
		plain := unaLinea(c.Plain)
		if g == GrupoDetenida {
			plain = motivoDetenida(c)
		}
		ls[0] = lin(" ").masCon(m.fraseCaja(c, g, p), negrita).mas("  ").masCon(plain, gris)
		var siguen []*boardcmd.Card
		for _, x := range append(append([]*boardcmd.Card(nil), gs[GrupoTeToca]...), gs[GrupoDetenida]...) {
			if x != c {
				siguen = append(siguen, x)
			}
		}
		ls[1] = m.segundaDeCaja(p, siguen, wi)
	case len(gs[GrupoCorriendo]) > 0:
		cs := gs[GrupoCorriendo]
		n := "1 tarjeta"
		if len(cs) > 1 {
			n = strconv.Itoa(len(cs)) + " tarjetas"
		}
		ls[0] = lin(" ").masCon("Nada te espera. hoom trabaja en "+n+".", negrita)
		primera := cs[0]
		que := "piloto, decidiendo"
		if r := primera.Running; r != nil {
			que = rolDe(r.Role) + ", " + reloj(m.ahora.Sub(r.StartedAt))
		}
		t := "corriendo: " + m.citar(boardcmd.TituloDe(primera)) + " (" + que + ")"
		if k := len(cs) - 1; k > 0 {
			t += m.fijo(fmt.Sprintf(" y %d más", k))
		}
		ls[1] = lin(" " + t)
	case len(gs[GrupoEnCola]) > 0:
		e := gs[GrupoEnCola][0]
		ls[0] = lin(" ").masCon(m.frase("Nada en curso. Podés empezar ", boardcmd.TituloDe(e), "."), negrita)
		ls[1] = m.segundaDeCaja(m.pasoDe(e), nil, wi)
	default:
		ls[0] = lin(" ").masCon("No hay nada pendiente.", negrita)
		ls[1] = lin(` para sumar trabajo: hoom item add "<titulo>"`)
	}
	return der, ls, amarilla
}

// fraseCaja is the box's phrase for its card c of group g with step p:
// boardcmd's, and Seguir solo's, that only the board offers.
func (m *Modelo) fraseCaja(c *boardcmd.Card, g Grupo, p paso) string {
	if p.seguir && g != GrupoDetenida {
		return m.frase("Dejá que ", boardcmd.TituloDe(c), " siga sola.")
	}
	id := ""
	if p.act != nil {
		id = p.act.ID
	}
	return boardcmd.FraseDe(c, string(g), id, m.piel())
}

// segundaDeCaja is the box's second line: "n <label>" (or the rule's
// text) and, gray on the right, "después:" and the titles that follow.
func (m *Modelo) segundaDeCaja(p paso, siguen []*boardcmd.Card, wi int) linea {
	izq := p.texto
	if p.hay() {
		izq = "n " + strings.ToLower(p.etiqueta())
	}
	izq = unaLinea(izq)
	l := lin(" " + izq)
	if len(siguen) == 0 {
		return l
	}
	lugar := wi - 1 - celdas(limpio(izq)) - 2 - 1
	der := m.fijo("después: ")
	puestos := 0
	for _, c := range siguen {
		sep := ""
		if puestos > 0 {
			sep = " · "
		}
		t := limpio(boardcmd.TituloDe(c))
		pieza := m.citar(t)
		if celdas(der+sep+pieza) <= lugar {
			der += sep + pieza
			puestos++
			continue
		}
		if resto := lugar - celdas(der+sep); resto >= 2 {
			der += sep + m.citar(cortar(t, resto-celdas(m.s.comillaAbre+m.s.comillaCierra), m.s.corte))
			puestos++
		}
		break
	}
	if puestos == 0 {
		return l
	}
	return lin(" ").mas(izq).mas(strings.Repeat(" ", max(1, wi-1-celdas(limpio(izq))-celdas(der)-1))).masCon(der, gris).mas(" ")
}

// ---------------------------------------------------------------- the screen

// Pantalla is the screen now, as the package's doc describes it.
func (m *Modelo) Pantalla() []string {
	w, h := m.c.Ancho, m.c.Alto
	if h <= 0 {
		return nil
	}
	out := make([]string, h)
	if m.pequena() {
		if w > 0 {
			out[0] = cortar(msgAgrandar, w, m.s.corte)
		}
		return out
	}
	lz := nuevoLienzo(w, h, m.s.corte)
	lz.poner(0, yCabecera, w, m.cabecera(), normal, false)
	der, ls, amarilla := m.cajaPaso()
	e := gris
	if amarilla {
		e = amarillo
	}
	m.dibujarMarco(lz, marco{r: rect{0, yCaja, w, altoCaja}, estilo: e, titulo: lin("TU SIGUIENTE PASO"),
		derecha: der, lineas: ls[:], sel: -1})
	z := m.disponer()
	m.dibujarListaZona(lz, z)
	m.dibujarDetalle(lz, z)
	m.dibujarPlegada(lz, rect{0, h - 3, w, 1}, m.tituloRegistro())
	lz.poner(0, h-2, w, m.lineaMensajes(), normal, false)
	lz.poner(0, h-1, w, m.lineaTeclas(), normal, false)
	m.dibujarModal(lz)
	return lz.lineas(m.c.Color)
}

func (m *Modelo) dibujarListaZona(lz *lienzo, z zonas) {
	titulo := m.tituloLista()
	if z.plegada {
		if pos := m.posicionLista(); pos != "" {
			titulo = titulo.mas(" · " + pos)
		}
		m.dibujarPlegada(lz, z.lista, titulo)
		return
	}
	ls, _, sel := m.contenidoLista(z.lista)
	pos := ""
	if len(ls) > z.lista.h-2 && sel >= 0 {
		pos = m.posicionLista()
	}
	m.dibujarMarco(lz, marco{r: z.lista, foco: m.zona == ZonaLista, titulo: titulo, lineas: ls, sel: sel,
		scroll: *m.scrollLista(), pos: pos})
}

func (m *Modelo) dibujarDetalle(lz *lienzo, z zonas) {
	titulo, der, d := m.documentoDetalle(z.detalle)
	ls := make([]linea, len(d))
	for i, x := range d {
		ls[i] = x.l
	}
	m.dibujarMarco(lz, marco{r: z.detalle, foco: m.zona == ZonaDetalle, titulo: titulo, derecha: der, lineas: ls,
		sel: -1, scroll: m.scroll0})
}

// doc0 is the detail's document and its inner height.
func (m *Modelo) doc0() ([]docLinea, int) {
	r := m.disponer().detalle
	_, _, d := m.documentoDetalle(r)
	return d, max(1, r.h-2)
}

// alto0 is the detail's inner height.
func (m *Modelo) alto0() int { return max(1, m.disponer().detalle.h-2) }

// lineaMensajes is the message line.
func (m *Modelo) lineaMensajes() linea {
	l := lin(" ")
	if m.experto {
		l = l.mas("EXPERTO")
	}
	if m.buscando {
		if m.experto {
			l = l.mas(" · ")
		}
		return l.mas("/" + string(m.filtro) + m.s.cursor)
	}
	if msg, e := m.mensaje(); msg != "" {
		if m.experto {
			l = l.mas(" · ")
		}
		l = l.masCon(unaLinea(msg), e)
	}
	return l
}

// pista is a hint of the keys line; enter's goes bold.
type pista struct {
	texto string
	enter bool
}

// pistas are the hints of the keys line now.
func (m *Modelo) pistas() []pista {
	l := m.Lugar()
	c := m.tarjetaDeLugar(l)
	var out []pista
	for _, a := range m.filas(l) {
		if a.Barra == "" || a.DeTarjeta && c == nil {
			continue
		}
		if a.Accion == AccionPaso {
			if t := m.pistaPaso(l, c); t != "" {
				out = append(out, pista{texto: t, enter: true})
			}
			continue
		}
		// "enter ver" of the Hallazgos view is enter's hint too (CA-718)
		out = append(out, pista{texto: m.s.conFlechas(a.Barra), enter: contieneTecla(a.Teclas, TeclaEnter) && l == LugarHallazgos})
	}
	return out
}

// pistaPaso is enter's hint: the step of the chosen card, or HECHO's line.
func (m *Modelo) pistaPaso(l Lugar, c *boardcmd.Card) string {
	if l == LugarTarjetas && !m.sel.nada && m.sel.hecho && len(m.filasLista()) > 0 {
		if m.hechoAbierto {
			return "enter cerrar"
		}
		return "enter abrir"
	}
	if c == nil {
		return ""
	}
	if p := m.pasoDe(c); p.hay() {
		return "enter " + strings.ToLower(p.etiqueta())
	}
	return ""
}

// lineaTeclas is the keys line: the hints that fit (the package doc, "The
// two last lines").
func (m *Modelo) lineaTeclas() linea {
	ps := m.pistas()
	w := m.c.Ancho
	ancho := func() int {
		n := 1
		for i, p := range ps {
			if i > 0 {
				n += 3
			}
			n += celdas(p.texto)
		}
		return n
	}
	if len(ps) > 0 && ancho() > w {
		if ps[len(ps)-1].texto == "? todas las teclas" {
			ps[len(ps)-1].texto = "? todas"
		}
		for ancho() > w && len(ps) > 1 {
			i := len(ps) - 2
			if i == 0 && ps[0].enter {
				break
			}
			ps = append(ps[:i], ps[i+1:]...)
		}
	}
	l := lin(" ")
	for i, p := range ps {
		if i > 0 {
			l = l.mas(" · ")
		}
		if p.enter {
			l = l.masCon(p.texto, negrita)
		} else {
			l = l.mas(p.texto)
		}
	}
	return l
}

// ---------------------------------------------------------------- after any change

// acomodar keeps the selections, the scrolls and the detail where the
// contract says after any change.
// detalleLeido says whether the detail shows a document that is read: not
// a card's while its detail is still being read (" leyendo la tarjeta...").
func (m *Modelo) detalleLeido() bool {
	clave := m.clave0Actual()
	if !strings.HasPrefix(clave, "t:") {
		return true
	}
	return m.detalle != nil && m.detalle.Slug == strings.TrimPrefix(clave, "t:")
}

func (m *Modelo) acomodar() {
	if m.c.Ancho <= 0 || m.c.Alto <= 0 {
		return
	}
	m.acomodarLista()
	m.acomodar4()
	if m.buscando && (m.zona != ZonaLista || m.vista != VistaTarjetas) {
		m.cerrarBusqueda()
	}
	// what the detail shows changed: its document starts again
	if clave := m.clave0Actual(); clave != m.clave0 {
		m.clave0 = clave
		m.scroll0 = 0
		m.sigue0 = false
		if c := m.tarjeta(strings.TrimPrefix(clave, "t:")); strings.HasPrefix(clave, "t:") && c != nil && c.Running != nil {
			m.sigue0 = true
		}
	}
	if m.pequena() {
		return
	}
	z := m.disponer()
	d, hi := m.doc0()
	maximo := max(0, len(d)-hi)
	if m.sigue0 && m.detalleLeido() {
		m.scroll0 = maximo
	}
	m.scroll0 = max(0, min(m.scroll0, maximo))
	if z.plegada {
		return
	}
	ls, filas, sel := m.contenidoLista(z.lista)
	hiL := max(1, z.lista.h-2)
	p := m.scrollLista()
	if sel >= 0 {
		ultima := sel
		for ultima+1 < len(filas) && filas[ultima+1] == filas[sel] {
			ultima++
		}
		if ultima >= *p+hiL {
			*p = ultima - hiL + 1
		}
		if sel < *p {
			*p = sel
		}
	}
	*p = max(0, min(*p, len(ls)-hiL))
}

// contieneTecla says whether ts has t.
func contieneTecla(ts []Tecla, t Tecla) bool {
	for _, x := range ts {
		if x == t {
			return true
		}
	}
	return false
}
