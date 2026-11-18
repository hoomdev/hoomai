package tablero

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// pasaFiltro: the card stays while the search filters.
func (m *Modelo) pasaFiltro(c *boardcmd.Card) bool {
	if !m.buscando || len(m.filtro) == 0 {
		return true
	}
	q := plegar(string(m.filtro))
	return strings.Contains(plegar(c.Slug), q) || strings.Contains(plegar(c.Item.Titulo), q)
}

// filaL is a row of the list of cards: a card or HECHO's line.
type filaL struct {
	slug  string
	hecho bool
}

// filasLista are the rows of the list of cards, in order: the cards of each
// group but HECHO, then HECHO's line and, when it is open, its cards.
func (m *Modelo) filasLista() []filaL {
	if !m.leido || len(m.todas()) == 0 {
		return nil
	}
	gs := m.grupos(true)
	var out []filaL
	for _, g := range ordenGrupos {
		if g == GrupoHecho {
			out = append(out, filaL{hecho: true})
			if !m.hechoAbierto {
				continue
			}
		}
		for _, c := range gs[g] {
			out = append(out, filaL{slug: c.Slug})
		}
	}
	return out
}

// elegidaDeLista is the card chosen in the list of cards ("" on HECHO's
// line or none).
func (m *Modelo) elegidaDeLista() string {
	if m.sel.nada || m.sel.hecho {
		return ""
	}
	c := m.tarjeta(m.sel.slug)
	if c == nil || !m.pasaFiltro(c) {
		return ""
	}
	return m.sel.slug
}

// elegirL puts the list's selection on its i-th row (clamped).
func (m *Modelo) elegirL(i int) {
	fs := m.filasLista()
	if len(fs) == 0 {
		return
	}
	i = max(0, min(i, len(fs)-1))
	m.sel = elegido{slug: fs[i].slug, hecho: fs[i].hecho, idx: i}
}

// elegirTarjeta puts the list's selection on the card slug, when it has a
// row.
func (m *Modelo) elegirTarjeta(slug string) {
	for i, f := range m.filasLista() {
		if !f.hecho && f.slug == slug {
			m.elegirL(i)
			return
		}
	}
}

// acomodarLista keeps the list's selection on its card or line; when it is
// no longer there, it takes the row with its index (the last one when there
// are fewer). At the start it is the first card row.
func (m *Modelo) acomodarLista() {
	fs := m.filasLista()
	if len(fs) == 0 {
		return
	}
	if m.sel.nada {
		for i, f := range fs {
			if !f.hecho {
				m.elegirL(i)
				return
			}
		}
		m.elegirL(0)
		return
	}
	for i, f := range fs {
		if f.hecho == m.sel.hecho && (f.hecho || f.slug == m.sel.slug) {
			m.sel.idx = i
			return
		}
	}
	m.elegirL(m.sel.idx)
}

func (m *Modelo) abrirBusqueda() {
	m.vista = VistaTarjetas
	m.ponerZona(ZonaLista)
	m.buscando = true
	m.filtro = nil
}

func (m *Modelo) cerrarBusqueda() {
	m.buscando = false
	m.filtro = nil
	m.acomodarLista()
}

// volverATarjetas is the cards view again, with the card that was chosen and
// the focus on the list.
func (m *Modelo) volverATarjetas() {
	m.vista = VistaTarjetas
	m.ponerZona(ZonaLista)
}

// ---------------------------------------------------------------- the rows

// ancha says whether the body has two zones (W >= 100).
func (m *Modelo) ancha() bool { return m.c.Ancho >= 100 }

// estiloGrupo is the color of a group's rows and states.
func estiloGrupo(g Grupo) estilo {
	switch g {
	case GrupoTeToca:
		return amarillo
	case GrupoCorriendo:
		return cian
	case GrupoDetenida:
		return rojo
	}
	return gris
}

// estiloNombre is the style of a group's name in its line.
func estiloNombre(g Grupo) estilo {
	switch g {
	case GrupoTeToca, GrupoCorriendo, GrupoDetenida:
		return inverso | estiloGrupo(g)
	}
	return gris
}

// rolCorriendo is the role and the time of a card that runs: "<rol> ·
// <tiempo>[ de <limite>]".
func (m *Modelo) rolCorriendo(r *boardcmd.Running) string {
	t := rolDe(r.Role) + " · " + reloj(m.ahora.Sub(r.StartedAt))
	if limite, ok := limiteDe(r); ok {
		t += " de " + reloj(limite)
	}
	return t
}

// limiteDe is the limit of the run r and whether it has one: its timeout_s
// when it is above 0 and fits a time.Duration. A damaged sidecar's
// negative or huge timeout_s is no limit, never a crash.
func limiteDe(r *boardcmd.Running) (time.Duration, bool) {
	if r.TimeoutS <= 0 || int64(r.TimeoutS) > math.MaxInt64/int64(time.Second) {
		return 0, false
	}
	return time.Duration(r.TimeoutS) * time.Second, true
}

// estadoDe is the state of a card's row (its second line, or the end of a
// narrow row).
func (m *Modelo) estadoDe(c *boardcmd.Card, g Grupo) string {
	switch g {
	case GrupoCorriendo:
		if c.Running == nil {
			return "el piloto la lleva · decidiendo"
		}
		t := m.rolCorriendo(c.Running)
		if m.pilotoVivo(c.Slug) {
			t = "el piloto la lleva · " + t
		}
		return t
	case GrupoDetenida:
		return m.s.marcaDetenida + " " + motivoDetenida(c)
	}
	return unaLinea(c.Plain)
}

// motivoDetenida is why a card of DETENIDA stopped: "se corto el trabajo
// del <rol>" with interrupted, else its red's plain.
func motivoDetenida(c *boardcmd.Card) string {
	if i := c.Interrupted; i != nil {
		return "se corto el trabajo del " + rolDe(i.Role)
	}
	if c.Red != nil {
		return unaLinea(c.Red.Plain)
	}
	return ""
}

// indiceColumna is the place (from 0) of the column id in boardcmd.Columns.
func indiceColumna(id string) int {
	for i, col := range boardcmd.Columns {
		if col.ID == id {
			return i
		}
	}
	return -1
}

// signoCamino is the sign of the path for the column j of a card of group
// g in the column i, and its color.
func (m *Modelo) signoCamino(i, j int, g Grupo) (string, estilo) {
	switch {
	case g == GrupoHecho || j < i:
		return m.s.antes, verde
	case j > i:
		return m.s.despues, gris
	}
	switch g {
	case GrupoTeToca:
		return m.s.teToca, amarillo
	case GrupoCorriendo:
		return m.s.corriendo, cian
	case GrupoDetenida:
		return m.s.detenida, rojo
	}
	return m.s.despues, gris
}

// camino is the card's path: one sign per column, in its color.
func (m *Modelo) camino(c *boardcmd.Card, g Grupo) linea {
	i := indiceColumna(c.Column)
	var l linea
	for j := range boardcmd.Columns {
		s, e := m.signoCamino(i, j, g)
		l = l.masCon(s, e)
	}
	return l
}

// lineasTarjeta are a card's lines in the list, w the list's width.
func (m *Modelo) lineasTarjeta(c *boardcmd.Card, g Grupo, sel bool, w, t int) []linea {
	marca := " "
	if sel {
		marca = m.s.sel
	}
	titulo := boardcmd.TituloDe(c)
	estado := m.estadoDe(c, g)
	if !m.ancha() {
		l := lin(" " + marca + " " + rellenar(cortar(titulo, t, m.s.corte), t) + " ")
		l = append(l, m.camino(c, g)...)
		return []linea{l.mas(" ").masCon(estado, estiloGrupo(g))}
	}
	lugar := w - 2 - 3
	var uno linea
	if lugar-9 >= 8 {
		uno = append(lin(" "+marca+" "+rellenar(cortar(titulo, lugar-9, m.s.corte), lugar-9)+" "), m.camino(c, g)...)
	} else {
		uno = lin(" " + marca + " " + cortar(titulo, lugar, m.s.corte))
	}
	return []linea{uno, linCon("   "+estado, estiloGrupo(g))}
}

// anchoTitulos is the width of the titles of a narrow list: max(8, min(the
// widest title shown, (w-2-12)/2)).
func (m *Modelo) anchoTitulos(gs map[Grupo][]*boardcmd.Card, w int) int {
	mayor := 0
	for _, g := range ordenGrupos {
		if g == GrupoHecho && !m.hechoAbierto {
			continue
		}
		for _, c := range gs[g] {
			mayor = max(mayor, celdas(limpio(boardcmd.TituloDe(c))))
		}
	}
	return max(8, min(mayor, (w-2-12)/2))
}

// contenidoTarjetas is the list of cards for a width w: its lines, and for
// each line the row it is of (-1 for a group line and for the texts of an
// empty list).
func (m *Modelo) contenidoTarjetas(w int) ([]linea, []int) {
	if !m.leido {
		return []linea{lin(" leyendo el tablero...")}, []int{-1}
	}
	if len(m.todas()) == 0 {
		return []linea{lin(` no hay tarjetas: hoom item add "<titulo>"`)}, []int{-1}
	}
	gs := m.grupos(true)
	t := m.anchoTitulos(gs, w)
	var out []linea
	var filas []int
	idx := 0
	elegida := func(i int) bool { return !m.sel.nada && m.sel.idx == i }
	for _, g := range ordenGrupos {
		cs := gs[g]
		if g == GrupoHecho {
			marca := ""
			if elegida(idx) {
				marca = m.s.sel + " "
			}
			signo := m.s.hechoCerrado
			if m.hechoAbierto {
				signo = m.s.hechoAbierto
			}
			out = append(out, lin(" "+marca).masCon(string(g), estiloNombre(g)).mas(" "+strconv.Itoa(len(cs))+" "+signo))
			filas = append(filas, idx)
			idx++
			if !m.hechoAbierto {
				continue
			}
		} else {
			if len(cs) == 0 {
				continue
			}
			out = append(out, lin(" ").masCon(string(g), estiloNombre(g)).mas(" "+strconv.Itoa(len(cs))))
			filas = append(filas, -1)
		}
		for _, c := range cs {
			for _, l := range m.lineasTarjeta(c, g, elegida(idx), w, t) {
				out = append(out, l)
				filas = append(filas, idx)
			}
			idx++
		}
	}
	return out, filas
}

// ---------------------------------------------------------------- the list's box

// tituloLista is the list's title: "Tarjetas", or the Hallazgos view's.
func (m *Modelo) tituloLista() linea {
	if m.vista == VistaTarjetas {
		return lin("Tarjetas")
	}
	n := len(m.hallazgos)
	if n == 0 {
		return lin("Hallazgos")
	}
	l := lin("Hallazgos " + strconv.Itoa(n))
	altos := 0
	for _, h := range m.hallazgos {
		if h.Severity == "high" {
			altos++
		}
	}
	if m.c.Color && altos > 0 {
		l = l.masCon(" · "+strconv.Itoa(altos)+" high", rojo)
	}
	return l
}

// esAvisoLista says whether the list shows its text of empty or of reading.
func (m *Modelo) esAvisoLista() bool {
	if m.vista == VistaHallazgos {
		return !m.hallLeidos || len(m.hallazgos) == 0
	}
	return !m.leido || len(m.todas()) == 0
}

// contenidoLista is the list's content for its box (r): its lines, the line
// of each one's row (-1: none) and the first line of the chosen row (-1:
// none).
func (m *Modelo) contenidoLista(r rect) ([]linea, []int, int) {
	var ls []linea
	var filas []int
	elegida := -1
	if m.vista == VistaHallazgos {
		ls = m.contenido4(r.w - 2)
		if len(m.hallazgos) > 0 {
			for i := range ls {
				filas = append(filas, i)
			}
			elegida = m.idx4
		}
	} else {
		ls, filas = m.contenidoTarjetas(r.w)
		if !m.sel.nada {
			for i, f := range filas {
				if f == m.sel.idx {
					elegida = i
					break
				}
			}
		}
	}
	if m.esAvisoLista() && len(ls) == 1 {
		ls = avisoEnvuelto(ls[0], r.w-2, max(1, r.h-2))
		filas = make([]int, len(ls))
		for i := range filas {
			filas[i] = -1
		}
		elegida = -1
	}
	return ls, filas, elegida
}

// posicionLista is "<n> de <m>" of the list's chosen row ("" without one).
func (m *Modelo) posicionLista() string {
	if m.vista == VistaHallazgos {
		if len(m.hallazgos) == 0 {
			return ""
		}
		return strconv.Itoa(m.idx4+1) + " de " + strconv.Itoa(len(m.hallazgos))
	}
	fs := m.filasLista()
	if len(fs) == 0 || m.sel.nada {
		return ""
	}
	return strconv.Itoa(m.sel.idx+1) + " de " + strconv.Itoa(len(fs))
}

// scrollLista is a pointer to the list's scroll in the current view.
func (m *Modelo) scrollLista() *int {
	if m.vista == VistaHallazgos {
		return &m.scroll4
	}
	return &m.scrollL
}

// moverLista moves the list's selection d rows (0: the first, 2: the
// last).
func (m *Modelo) moverLista(d int, extremo bool) {
	if m.vista == VistaHallazgos {
		switch {
		case extremo && d < 0:
			m.elegir4(0)
		case extremo:
			m.elegir4(len(m.hallazgos) - 1)
		default:
			m.elegir4(m.idx4 + d)
		}
		return
	}
	if m.sel.nada {
		return
	}
	switch {
	case extremo && d < 0:
		m.elegirL(0)
	case extremo:
		m.elegirL(len(m.filasLista()) - 1)
	default:
		m.elegirL(m.sel.idx + d)
	}
}
