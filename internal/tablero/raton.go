package tablero

import (
	"strings"
	"time"
)

// Raton takes one mouse event at ahora (see the package doc, "The mouse").
// It moves the focus, chooses rows, options and buttons and scrolls; it
// never orders Lanzar nor opens a modal, so it returns no order (CA-575).
func (m *Modelo) Raton(r Raton, ahora time.Time) []Orden {
	if !ahora.IsZero() {
		m.ahora = ahora
	}
	if m.pequena() {
		return nil
	}
	if m.modal != SinModal {
		m.ratonEnModal(r)
	} else {
		m.ratonEnPaneles(r)
	}
	m.acomodar()
	return nil
}

// ratonEnPaneles is a mouse event without a modal.
func (m *Modelo) ratonEnPaneles(r Raton) {
	var c cajaPanel
	hay := false
	for _, x := range m.disponer() {
		if x.r.contiene(r.X, r.Y) {
			c, hay = x, true
		}
	}
	if !hay || c.p == panelRegistro {
		return
	}
	switch r.Accion {
	case RatonClic:
		m.ponerFoco(c.p)
		if c.plegado || c.p == PanelPrincipal || c.p == PanelProyecto {
			return
		}
		// the row under the click
		dentro := r.Y - c.r.y - 1
		if dentro < 0 || dentro >= c.r.h-2 || r.X <= c.r.x || r.X >= c.r.x+c.r.w-1 {
			return
		}
		k := *m.scrollDe(c.p) + dentro
		switch c.p {
		case PanelTarjetas:
			_, filas := m.contenido2(c.r.w - 2)
			if k < len(filas) && filas[k] >= 0 {
				m.elegir2(filas[k])
			}
		case PanelCorriendo:
			if k < len(m.filas3()) {
				m.elegir3(k)
			}
		case PanelHallazgos:
			if k < len(m.hallazgos) {
				m.elegir4(k)
			}
		}
	case RatonRuedaArriba, RatonRuedaAbajo:
		if c.plegado {
			return
		}
		d := 1
		if r.Accion == RatonRuedaArriba {
			d = -1
		}
		switch c.p {
		case PanelTarjetas:
			m.elegir2(m.sel2.idx + d)
		case PanelCorriendo:
			m.elegir3(m.idx3 + d)
		case PanelHallazgos:
			m.elegir4(m.idx4 + d)
		case PanelPrincipal:
			m.desplazar0(3 * d)
		}
	}
}

// ratonEnModal is a mouse event with a modal open.
func (m *Modelo) ratonEnModal(r Raton) {
	g, ok := m.geometria()
	if !ok {
		return
	}
	if r.Accion != RatonClic {
		if m.modal == ModalAyuda && g.r.contiene(r.X, r.Y) {
			d := 3
			if r.Accion == RatonRuedaArriba {
				d = -3
			}
			m.desplazarAyuda(d)
		}
		return
	}
	// an open list: only its options
	if m.modal == ModalPedir && m.pedir.lista != nil {
		if g.lista.contiene(r.X, r.Y) {
			k := g.listaDesde + r.Y - g.lista.y - 1
			if r.Y > g.lista.y && r.Y < g.lista.y+g.lista.h-1 && k < len(g.listaOps) && !g.listaOps[k].gris {
				m.elegirDeLista(m.pedir, k)
			}
		}
		return
	}
	if !g.r.contiene(r.X, r.Y) {
		return
	}
	i := r.Y - g.r.y - 1
	if i < 0 || i >= len(g.lineas) {
		return
	}
	l := g.lineas[i]
	switch m.modal {
	case ModalPedir:
		p := m.pedir
		if p.otro != nil {
			return
		}
		switch {
		case l.botones:
			if b := m.botonBajo(r.X-g.r.x-4, [2]string{p.accion.Label, "Cancelar"}, g.r.w-6); b >= 0 {
				p.foco, p.boton = campoBotones, b
				m.decirEnter([2]string{p.accion.Label, "Cancelar"}[b])
			}
		case l.campo >= 0:
			p.foco = l.campo
		}
	case ModalFirma, ModalGuardar:
		if l.botones {
			e := m.conf.etiquetas()
			if b := m.botonBajo(r.X-g.r.x-4, e, g.r.w-6); b >= 0 {
				m.conf.boton = b
				m.decirEnter(e[b])
			}
		}
	case ModalAcciones:
		if c := m.tarjeta(m.acciones.slug); c != nil && l.accion >= 0 {
			as := m.accionesVisibles(c)
			if l.accion < len(as) && m.elegible(as[l.accion]) {
				m.acciones.sel = l.accion
			}
		}
	}
}

// botonBajo is the button under the column x of the text (-1: none).
func (m *Modelo) botonBajo(x int, etiquetas [2]string, c int) int {
	_, rangos := botones(etiquetas, 0, c)
	for i, r := range rangos {
		if x >= r[0] && x < r[1] {
			return i
		}
	}
	return -1
}

// decirEnter says what enter does on the chosen button.
func (m *Modelo) decirEnter(etiqueta string) {
	m.avisar("enter para "+strings.ToLower(etiqueta), normal, DuracionAviso)
}
