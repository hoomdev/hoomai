package tablero

import (
	"strconv"
	"strings"
	"time"
)

// The kinds of entries of the Registro.
const (
	asientoVerbo = iota
	asientoEditor
	asientoFin
)

// The estados of a verb's entry (lanzando… is written with the mode's sign).
const (
	estadoLanzando  = "lanzando"
	estadoCorriendo = "corriendo"
	estadoHecho     = `hecho` // the Registro's state (CA-563), no column id (CA-517)
)

// asiento is one entry of the Registro.
type asiento struct {
	hora     time.Time
	tipo     int
	etiqueta string // a verb's: the action's label
	slug     string
	estado   string // a verb's: estadoLanzando, estadoCorriendo, estadoHecho, "fallo: <...>" or "se detuvo: <...>"
	// estadoEstilo is the color of a "se detuvo: <...>" (green or red)
	estadoEstilo estilo
	archivo      string // the editor's
	linea        int
	texto        string // the end of a job's
	estilo       estilo // the end of a job's: green or red
	comando      string // its second line ("" for the end of a job)
}

// primera is its first line after "<HH:MM> ", in pieces.
func (a *asiento) primeraLinea(s signos) linea {
	switch a.tipo {
	case asientoEditor:
		t := "Abrir en el editor · " + a.archivo
		if a.linea > 0 {
			t += ":" + strconv.Itoa(a.linea)
		}
		return lin(t)
	case asientoFin:
		return linCon(a.texto, a.estilo)
	}
	l := lin(a.etiqueta + " · " + a.slug + " · ")
	switch {
	case a.estado == estadoLanzando:
		return l.mas("lanzando" + s.puntos)
	case strings.HasPrefix(a.estado, "fallo: "):
		return l.masCon(a.estado, rojo)
	case a.estadoEstilo != normal:
		return l.masCon(a.estado, a.estadoEstilo)
	}
	return l.mas(a.estado)
}

func (a *asiento) primera(s signos) string { return a.primeraLinea(s).plano() }

// lineas are the entry's lines in the Registro modal: "<HH:MM> <first>"
// and "      <comando>".
func (a *asiento) lineas(s signos) []linea {
	uno := append(lin(a.hora.Format("15:04")+" "), a.primeraLinea(s)...)
	if a.tipo == asientoFin {
		return []linea{uno.todaCon(a.estilo)}
	}
	return []linea{uno, lin("      " + s.enPantalla(unaLinea(a.comando)))}
}

// tituloRegistro is the folded Registro's title, with its newest entry.
func (m *Modelo) tituloRegistro() linea {
	if len(m.registro) == 0 {
		return lin("Registro")
	}
	a := m.registro[len(m.registro)-1]
	l := lin("Registro · " + a.hora.Format("15:04") + " ")
	if a.tipo == asientoFin {
		return append(l, linCon(a.texto, a.estilo)...)
	}
	return append(l, a.primeraLinea(m.s)...)
}

// registrarVerbo adds a verb's entry for the launch la.
func (m *Modelo) registrarVerbo(la *lanzado, etiqueta, comando string) {
	a := &asiento{hora: m.ahora, tipo: asientoVerbo, etiqueta: etiqueta, slug: la.o.Slug,
		estado: estadoLanzando, comando: comando}
	la.asiento = a
	m.registro = append(m.registro, a)
}

// registrarEditor adds the editor's entry.
func (m *Modelo) registrarEditor(archivo string, linea int) {
	partes := append([]string{m.c.Editor}, ArgumentosEditor(m.c.Editor, archivo, linea)...)
	m.registro = append(m.registro, &asiento{hora: m.ahora, tipo: asientoEditor, archivo: archivo, linea: linea,
		comando: strings.TrimSpace(strings.Join(partes, " "))})
}
