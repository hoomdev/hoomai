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
	estadoHecho     = "hecho"
)

// asiento is one entry of the Registro.
type asiento struct {
	hora     time.Time
	tipo     int
	etiqueta string // a verb's: the action's label
	slug     string
	estado   string // a verb's: estadoLanzando, estadoCorriendo, estadoHecho or "fallo: <...>"
	archivo  string // the editor's
	linea    int
	texto    string // the end of a job's
	estilo   estilo // the end of a job's: green or red
	comando  string // its second line ("" for the end of a job)
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
	}
	return l.mas(a.estado)
}

func (a *asiento) primera(s signos) string { return a.primeraLinea(s).plano() }

// lineas are the entry's lines in the panel, with their margin.
func (a *asiento) lineas(s signos) []linea {
	uno := append(lin(" "+a.hora.Format("15:04")+" "), a.primeraLinea(s)...)
	if a.tipo == asientoFin {
		return []linea{uno.todaCon(a.estilo)}
	}
	return []linea{uno, lin("       " + unaLinea(a.comando))}
}

// contenidoRegistro is the Registro's content for h inner lines: the newest
// whole entries that fit, the oldest of them on top.
func (m *Modelo) contenidoRegistro(h int) []linea {
	if len(m.registro) == 0 {
		return []linea{lin(" todavia no se ejecuto nada")}
	}
	var bloques [][]linea
	n := 0
	for i := len(m.registro) - 1; i >= 0; i-- {
		ls := m.registro[i].lineas(m.s)
		if n+len(ls) > h && len(bloques) > 0 {
			break
		}
		bloques = append(bloques, ls)
		n += len(ls)
	}
	var out []linea
	for i := len(bloques) - 1; i >= 0; i-- {
		out = append(out, bloques[i]...)
	}
	return out
}

// tituloRegistro is the Registro's title; folded, with its newest entry.
func (m *Modelo) tituloRegistro(plegado bool) linea {
	if !plegado || len(m.registro) == 0 {
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
