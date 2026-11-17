package tablero

import (
	"strings"
	"time"
)

// Raton takes one mouse event at ahora (see the package doc, "The mouse").
// It moves the focus, chooses rows, options and buttons and scrolls; it
// never orders Lanzar nor opens a modal, so it returns no order (CA-719).
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
	h := m.c.Alto
	if r.Y <= yCabecera || r.Y >= h-3 {
		return // the header, the Registro's line and the two last lines
	}
	caja := rect{0, yCaja, m.c.Ancho, altoCaja}
	z := m.disponer()
	rueda := r.Accion == RatonRuedaArriba || r.Accion == RatonRuedaAbajo
	d := 1
	if r.Accion == RatonRuedaArriba {
		d = -1
	}
	switch {
	case caja.contiene(r.X, r.Y):
		if r.Accion != RatonClic {
			return
		}
		c, _ := m.tarjetaDeCaja()
		if c == nil {
			return
		}
		m.buscando, m.filtro = false, nil
		m.vista = VistaTarjetas
		m.ponerZona(ZonaLista)
		m.elegirTarjeta(c.Slug)
	case z.lista.contiene(r.X, r.Y):
		if rueda {
			if !z.plegada {
				m.moverLista(d, false)
			}
			return
		}
		if r.Accion != RatonClic {
			return
		}
		m.ponerZona(ZonaLista)
		if z.plegada {
			return
		}
		l := z.lista
		dentro := r.Y - l.y - 1
		if dentro < 0 || dentro >= l.h-2 || r.X <= l.x || r.X >= l.x+l.w-1 {
			return
		}
		_, filas, _ := m.contenidoLista(l)
		k := *m.scrollLista() + dentro
		if k >= len(filas) || filas[k] < 0 {
			return
		}
		if m.vista == VistaHallazgos {
			m.elegir4(filas[k])
		} else {
			m.elegirL(filas[k])
		}
	case z.detalle.contiene(r.X, r.Y):
		if rueda {
			m.desplazar0(3 * d)
			return
		}
		if r.Accion == RatonClic {
			m.ponerZona(ZonaDetalle)
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
		if !g.r.contiene(r.X, r.Y) {
			return
		}
		d := 3
		if r.Accion == RatonRuedaArriba {
			d = -3
		}
		switch m.modal {
		case ModalAyuda:
			m.desplazarAyuda(d)
		case ModalRegistro:
			m.desplazarRegistro(d)
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
	case ModalFirma, ModalGuardar, ModalSeguir:
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
