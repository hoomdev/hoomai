package tablero

import (
	"strconv"
	"strings"
)

// cajaPanel is a panel's place on the screen.
type cajaPanel struct {
	p       Panel
	r       rect
	plegado bool
}

// disponer lays the panels out (the package doc, "The layout").
func (m *Modelo) disponer() []cajaPanel {
	w, a := m.c.Ancho, m.c.Alto
	b := a - 2
	h3n := min(7, max(3, max(1, len(m.filas3()))+2))
	if w >= 80 {
		l := w / 3
		pleg := map[Panel]bool{}
		var h1, h2, h3, h4 int
		for {
			h1, h3 = 6, h3n
			if pleg[PanelProyecto] {
				h1 = 1
			}
			if pleg[PanelCorriendo] {
				h3 = 1
			}
			h4 = max(3, (b-h1-h3)/3)
			if pleg[PanelHallazgos] {
				h4 = 1
			}
			h2 = b - h1 - h3 - h4
			if h2 >= 3 {
				break
			}
			plego := false
			for _, p := range []Panel{PanelProyecto, PanelHallazgos, PanelCorriendo} {
				if !pleg[p] && p != m.foco {
					pleg[p], plego = true, true
					break
				}
			}
			if !plego {
				break
			}
		}
		var out []cajaPanel
		y := 0
		for _, x := range []struct {
			p Panel
			h int
		}{{PanelProyecto, h1}, {PanelTarjetas, h2}, {PanelCorriendo, h3}, {PanelHallazgos, h4}} {
			out = append(out, cajaPanel{p: x.p, r: rect{0, y, l, x.h}, plegado: x.h == 1})
			y += x.h
		}
		hr := 4
		if a >= 24 {
			hr = 6
		}
		h0 := b - hr
		plegR := false
		if h0 < 3 {
			hr, h0, plegR = 1, b-1, true
		}
		out = append(out, cajaPanel{p: PanelPrincipal, r: rect{l, 0, w - l, h0}},
			cajaPanel{p: panelRegistro, r: rect{l, h0, w - l, hr}, plegado: plegR})
		return out
	}
	// one column
	natural := map[Panel]int{
		PanelProyecto:  6,
		PanelCorriendo: h3n,
		PanelHallazgos: max(3, max(1, len(m.hallazgos))+2),
	}
	l2, _ := m.contenido2(w - 2)
	natural[PanelTarjetas] = len(l2) + 2
	alturas := map[Panel]int{PanelProyecto: 1, PanelTarjetas: 1, PanelCorriendo: 1, PanelHallazgos: 1}
	if m.foco != PanelPrincipal {
		alturas[m.foco] = max(3, min(natural[m.foco], a/3))
	}
	h0 := b - 1
	for _, h := range alturas {
		h0 -= h
	}
	var out []cajaPanel
	y := 0
	for _, p := range []Panel{PanelProyecto, PanelTarjetas, PanelCorriendo, PanelHallazgos} {
		out = append(out, cajaPanel{p: p, r: rect{0, y, w, alturas[p]}, plegado: alturas[p] == 1})
		y += alturas[p]
	}
	out = append(out, cajaPanel{p: PanelPrincipal, r: rect{0, y, w, h0}},
		cajaPanel{p: panelRegistro, r: rect{0, y + h0, w, 1}, plegado: true})
	return out
}

// caja is the place of panel p.
func (m *Modelo) caja(p Panel) cajaPanel {
	for _, c := range m.disponer() {
		if c.p == p {
			return c
		}
	}
	return cajaPanel{}
}

// doc0 is [0]'s title, its document and its inner height.
func (m *Modelo) doc0() (string, []docLinea, int) {
	c := m.caja(PanelPrincipal)
	wi, hi := max(1, c.r.w-2), max(1, c.r.h-2)
	t, d := m.documento0(wi, hi)
	return t, d, hi
}

// alto0 is [0]'s inner height.
func (m *Modelo) alto0() int { return max(1, m.caja(PanelPrincipal).r.h-2) }

// contenido is panel p's content for its box: its lines, the line of its
// chosen row (-1: none) and its "<n> de <m>" when its lines do not fit.
func (m *Modelo) contenido(c cajaPanel) ([]linea, int) {
	wi := c.r.w - 2
	switch c.p {
	case PanelProyecto:
		return m.contenido1(), -1
	case PanelTarjetas:
		ls, filas := m.contenido2(wi)
		sel := -1
		if !m.sel2.nada {
			for i, f := range filas {
				if f == m.sel2.idx {
					sel = i
				}
			}
		}
		return ls, sel
	case PanelCorriendo:
		sel := -1
		if len(m.filas3()) > 0 {
			sel = m.idx3
		}
		return m.contenido3(wi), sel
	case PanelHallazgos:
		sel := -1
		if len(m.hallazgos) > 0 {
			sel = m.idx4
		}
		return m.contenido4(wi), sel
	case panelRegistro:
		return m.contenidoRegistro(max(1, c.r.h-2)), -1
	}
	return nil, -1
}

// scrollDe is a pointer to panel p's scroll.
func (m *Modelo) scrollDe(p Panel) *int {
	switch p {
	case PanelTarjetas:
		return &m.scroll2
	case PanelCorriendo:
		return &m.scroll3
	case PanelHallazgos:
		return &m.scroll4
	case PanelPrincipal:
		return &m.scroll0
	}
	return nil
}

// titulo is panel p's title.
func (m *Modelo) titulo(c cajaPanel) linea {
	switch c.p {
	case PanelProyecto:
		t := "[1] Proyecto"
		if c.plegado {
			t += m.estadoCheck()
		}
		return lin(t)
	case PanelTarjetas:
		return lin("[2] Tarjetas")
	case PanelCorriendo:
		if n := len(m.filas3()); n > 0 {
			return lin("[3] Corriendo " + strconv.Itoa(n))
		}
		return lin("[3] Corriendo")
	case PanelHallazgos:
		n := len(m.hallazgos)
		if n == 0 {
			return lin("[4] Hallazgos")
		}
		l := lin("[4] Hallazgos " + strconv.Itoa(n))
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
	case PanelPrincipal:
		t, _, _ := m.doc0()
		return lin(t)
	}
	return m.tituloRegistro(c.plegado)
}

// dibujarPanel draws one panel.
func (m *Modelo) dibujarPanel(lz *lienzo, c cajaPanel) {
	s := m.s
	r := c.r
	titulo := m.titulo(c)
	if c.plegado {
		lz.fijar(r.x, r.y, s.linea, 1, normal)
		n := lz.poner(r.x+1, r.y, r.w-2, titulo, normal, false)
		lz.repetir(r.x+1+n, r.y, r.w-1-n, s.linea, normal)
		return
	}
	foco := c.p == m.foco
	esqSI, esqSD, esqII, esqID, lado, hor := s.esqSI, s.esqSD, s.esqII, s.esqID, s.lado, s.linea
	borde := normal
	if foco {
		esqSI, esqSD, esqII, esqID, lado, hor = s.fEsqSI, s.fEsqSD, s.fEsqII, s.fEsqID, s.fLado, s.fLinea
		borde = estiloFoco
		for i := range titulo {
			if titulo[i].estilo == normal {
				titulo[i].estilo = estiloFoco
			}
		}
	}
	lz.fijar(r.x, r.y, esqSI, 1, borde)
	n := lz.poner(r.x+1, r.y, r.w-2, titulo, normal, false)
	lz.repetir(r.x+1+n, r.y, r.w-2-n, hor, borde)
	lz.fijar(r.x+r.w-1, r.y, esqSD, 1, borde)

	var lineas []linea
	sel := -1
	scroll := 0
	if c.p == PanelPrincipal {
		_, d, _ := m.doc0()
		for _, x := range d {
			lineas = append(lineas, x.l)
		}
		scroll = m.scroll0
	} else {
		lineas, sel = m.contenido(c)
		if p := m.scrollDe(c.p); p != nil {
			scroll = *p
		}
	}
	hi := r.h - 2
	for i := 0; i < hi; i++ {
		y := r.y + 1 + i
		lz.fijar(r.x, y, lado, 1, borde)
		k := scroll + i
		var l linea
		if k >= 0 && k < len(lineas) {
			l = lineas[k]
		}
		if k == sel && foco {
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
	pos := ""
	if len(lineas) > hi && sel >= 0 {
		switch c.p {
		case PanelTarjetas:
			pos = strconv.Itoa(m.sel2.idx+1) + " de " + strconv.Itoa(len(m.filas2()))
		case PanelCorriendo:
			pos = strconv.Itoa(m.idx3+1) + " de " + strconv.Itoa(len(m.filas3()))
		case PanelHallazgos:
			pos = strconv.Itoa(m.idx4+1) + " de " + strconv.Itoa(len(m.hallazgos))
		}
	}
	m.borde(lz, r.x, r.y+r.h-1, r.w, esqII, hor, esqID, pos, borde)
}

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
	for _, c := range m.disponer() {
		m.dibujarPanel(lz, c)
	}
	lz.poner(0, h-2, w, m.lineaMensajes(), normal, false)
	lz.poner(0, h-1, w, lin(m.lineaTeclas()), normal, false)
	m.dibujarModal(lz)
	return lz.lineas(m.c.Color)
}

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

// pistas are the hints of the keys line now.
func (m *Modelo) pistas() []string {
	l := m.Lugar()
	c := m.tarjetaDeLugar(l)
	var out []string
	for _, a := range AtajosDe(l) {
		if a.Barra == "" {
			continue
		}
		deTarjeta := a.Accion != "" || a.Barra == "espacio acciones"
		if deTarjeta && c == nil {
			continue
		}
		out = append(out, m.s.conFlechas(a.Barra))
		if a.Accion == AccionSiguiente {
			if act, fila, _ := m.siguientePaso(c); act != nil {
				out = append(out, m.s.conFlechas(fila.Muestra)+" "+strings.ToLower(act.Label))
			}
		}
	}
	return out
}

// lineaTeclas is the keys line: the hints that fit (the package doc, "The
// two last lines").
func (m *Modelo) lineaTeclas() string {
	ps := m.pistas()
	w := m.c.Ancho
	unir := func() string { return " " + strings.Join(ps, " · ") }
	if len(ps) == 0 || celdas(unir()) <= w {
		return unir()
	}
	if ps[len(ps)-1] == "? todas las teclas" {
		ps[len(ps)-1] = "? todas"
	}
	for celdas(unir()) > w && len(ps) > 1 {
		ps = append(ps[:len(ps)-2], ps[len(ps)-1])
	}
	return unir()
}

// acomodar keeps the selections, the scrolls and [0] where the contract
// says after any change.
func (m *Modelo) acomodar() {
	if m.c.Ancho <= 0 || m.c.Alto <= 0 {
		return
	}
	m.acomodar2()
	m.acomodar3()
	m.acomodar4()
	if m.buscando && m.foco != PanelTarjetas {
		m.cerrarBusqueda()
	}
	// what [0] shows changed: its document starts again
	if clave := m.clave0Actual(); clave != m.clave0 {
		m.clave0 = clave
		m.scroll0 = 0
		m.sigue0 = strings.HasPrefix(clave, "c:")
		if !strings.HasPrefix(clave, "t:") && !strings.HasPrefix(clave, "c:") {
			m.detalle = nil
		} else if m.detalle != nil && m.detalle.Slug != m.Elegida() {
			m.detalle = nil
		}
		if !strings.HasPrefix(clave, "h:") {
			m.archivo = nil
		}
	}
	if m.pequena() {
		return
	}
	for _, c := range m.disponer() {
		p := m.scrollDe(c.p)
		if p == nil || c.plegado {
			continue
		}
		hi := max(1, c.r.h-2)
		if c.p == PanelPrincipal {
			_, d, _ := m.doc0()
			maximo := max(0, len(d)-hi)
			if m.sigue0 {
				m.scroll0 = maximo
			}
			m.scroll0 = max(0, min(m.scroll0, maximo))
			continue
		}
		ls, sel := m.contenido(c)
		if sel >= 0 {
			if sel < *p {
				*p = sel
			}
			if sel >= *p+hi {
				*p = sel - hi + 1
			}
		}
		*p = max(0, min(*p, len(ls)-hi))
	}
}
