package tablero

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
)

// docLinea is one line of the detail's document: the line, and, on part D of a
// card's, the criterion it is of and whether it is ✗.
type docLinea struct {
	l    linea
	crit string
	sin  bool
}

// doc is a document being built: lines wrapped to the inner width.
type doc struct {
	wi     int
	lineas []docLinea
}

// poner adds l wrapped.
func (d *doc) poner(l linea) { d.ponerCrit(l, "", false) }

func (d *doc) ponerCrit(l linea, crit string, sin bool) {
	for _, p := range envolverLinea(l.limpia(), d.wi) {
		d.lineas = append(d.lineas, docLinea{l: p, crit: crit, sin: sin})
	}
}

// cortada adds l without wrapping (the box cuts it).
func (d *doc) cortada(l linea) { d.lineas = append(d.lineas, docLinea{l: l}) }

func (d *doc) vacia() { d.lineas = append(d.lineas, docLinea{l: lin("")}) }

// ---------------------------------------------------------------- Proyecto

func (m *Modelo) docProyecto(d *doc) {
	p := m.proyecto
	if p == nil {
		d.poner(lin(" leyendo el proyecto..."))
		return
	}
	d.poner(lin(" Veredicto"))
	if v := p.Veredicto; v != nil {
		l := lin("  " + v.ID + " · hace " + hace(m.ahora.Sub(v.CreatedAt)) + " · ")
		if v.Verdict == "green" {
			l = l.masCon("VERDE", verde)
		} else {
			l = l.masCon("ROJO", rojo)
		}
		if v.Partial {
			l = l.mas(" · parcial")
		}
		d.poner(l)
		s := v.Summary
		d.poner(lin(fmt.Sprintf("  pass %d · fail %d · error %d · absent %d", s.Pass, s.Fail, s.Error, s.Absent)))
		d.poner(lin("  .hoom/verdicts/" + v.ID + ".json"))
	} else {
		d.poner(lin("  sin veredicto todavia"))
	}
	d.vacia()
	d.poner(lin(" Check"))
	d.poner(append(lin("  "), m.lineaCheck()...))
	if len(p.Avisos) > 0 {
		d.vacia()
		d.poner(lin(" Avisos"))
		for _, a := range p.Avisos {
			d.poner(lin("  " + unaLinea(a)))
		}
	}
	d.vacia()
	d.poner(lin(" Doctor"))
	if len(p.Problemas) == 0 {
		d.poner(lin("  sin problemas"))
	}
	for _, pr := range p.Problemas {
		que := pr.Plain
		if que == "" {
			que = pr.What
		}
		t := que + ": " + pr.Action
		if pr.Slug != "" {
			t = pr.Slug + " · " + t
		}
		d.poner(lin("  " + unaLinea(t)))
	}
}

// ---------------------------------------------------------------- a card

func tieneSpec(d *Detalle) bool { return d.Detalle.Spec.Exists || d.Detalle.Spec.Markdown != "" }

// rutaSpec is the card's spec, relative to the project root.
func rutaSpec(c *boardcmd.Card, det *Detalle) string {
	p := det.Detalle.Paths.Spec
	if p != "" {
		return p
	}
	p = det.Detalle.Spec.Path
	if p == "" {
		p = c.Evidence.Spec
	}
	if absoluta(p) {
		return p
	}
	dir := det.Detalle.Card.Evidence.Dir
	if dir == "" {
		dir = c.Evidence.Dir
	}
	return path.Join(dir, p)
}

// sinPruebaDe are the card's criteria without a test.
func sinPruebaDe(c *boardcmd.Card, det *Detalle) map[string]bool {
	out := map[string]bool{}
	for _, u := range c.Evidence.Untraced {
		out[u] = true
	}
	for _, u := range det.Detalle.Card.Evidence.Untraced {
		out[u] = true
	}
	return out
}

func (m *Modelo) docSpec(d *doc, c *boardcmd.Card, det *Detalle) {
	if !tieneSpec(det) {
		d.poner(lin(" sin spec: " + rutaSpec(c, det)))
		return
	}
	sin := sinPruebaDe(c, det)
	crits := det.Detalle.Criteria
	k := 0
	for _, ct := range crits {
		if sin[ct.ID] {
			k++
		}
	}
	n := fmt.Sprintf("%d criterios", len(crits))
	if len(crits) == 1 {
		n = "1 criterio"
	}
	d.poner(lin(fmt.Sprintf(" Spec · %s · %d sin prueba", n, k)))
	for _, ct := range crits {
		signo := m.s.conPrueba
		switch {
		case sin[ct.ID]:
			signo = m.s.sinPrueba
		case ct.TracedBy == boardcmd.TracedComando:
			signo = m.s.porComando
		}
		t := "  " + signo + " "
		if m.experto {
			t += ct.ID + " "
		}
		t += unaLinea(ct.Text)
		d.ponerCrit(lin(strings.TrimRight(t, " ")), ct.ID, sin[ct.ID])
	}
}

func (m *Modelo) docHallazgosTarea(d *doc, det *Detalle) {
	var abiertos, otros []finding.Item
	for _, f := range det.Detalle.Findings {
		if f.Status == finding.StatusOpen {
			abiertos = append(abiertos, f)
		} else {
			otros = append(otros, f)
		}
	}
	t := " Hallazgos"
	switch k := len(abiertos); {
	case k == 1:
		t += " · 1 abierto"
	case k > 1:
		t += fmt.Sprintf(" · %d abiertos", k)
	}
	d.poner(lin(t))
	if len(abiertos)+len(otros) == 0 {
		d.poner(lin("  sin hallazgos"))
	}
	for _, f := range abiertos {
		d.poner(lin("  ").masCon(f.Severity, estiloSeveridad(f.Severity)).mas(" " + donde(f) + " " + unaLinea(f.Description)))
	}
	for _, f := range otros {
		d.poner(linCon("  "+f.Severity+" "+donde(f)+" "+unaLinea(f.Description), gris))
	}
}

func (m *Modelo) docSobre(d *doc, c *boardcmd.Card, det *Detalle) {
	var partes []string
	switch {
	case c.Running != nil:
		r := c.Running
		partes = append(partes, rolDe(r.Role))
		if p := etapa(r.Stage, r.Step, r.Steps); p != "" {
			partes = append(partes, p)
		}
		t := reloj(m.ahora.Sub(r.StartedAt))
		if r.TimeoutS != 0 {
			t += " de " + reloj(time.Duration(r.TimeoutS)*time.Second)
		}
		partes = append(partes, t)
	case len(det.Detalle.Work) > 0:
		w := det.Detalle.Work[0]
		partes = append(partes, rolDe(w.Role))
		if p := etapa(w.Stage, w.Step, w.Steps); p != "" {
			partes = append(partes, p)
		}
		if w.DurationMS != nil {
			partes = append(partes, reloj(time.Duration(*w.DurationMS)*time.Millisecond))
		}
		if w.Status != "" {
			partes = append(partes, w.Status)
		}
	default:
		d.poner(lin("  sin sobres todavia"))
		return
	}
	ev := det.Sobre.Eventos
	if n := len(ev); n > 0 {
		partes = append(partes, fmt.Sprintf("%s evento hace %ds", m.s.ultimo, segundos(m.ahora.Sub(ev[n-1].TS))))
	}
	d.poner(lin("  " + strings.Join(partes, " · ")))
	if det.Sobre.Err != nil {
		d.poner(linCon("  no se pudo leer el sobre: "+unaLinea(det.Sobre.Err.Error()), rojo))
	}
	for _, e := range ev {
		l := "  " + e.TS.UTC().Format("15:04:05") + " " + e.Kind
		if e.Detail != "" {
			l += " " + unaLinea(e.Detail)
		}
		d.cortada(lin(l))
	}
}

// ---------------------------------------------------------------- a finding

func (m *Modelo) docHallazgo(d *doc, h finding.Item, hi int) {
	tarea := h.Task
	if tarea == "" {
		tarea = "sin tarea"
	}
	d.poner(lin(" ").masCon(h.Severity, estiloSeveridad(h.Severity)).mas(" · lente " + h.Lens + " · tarea " + tarea))
	for _, p := range strings.Split(strings.ReplaceAll(h.Description, "\r\n", "\n"), "\n") {
		d.poner(lin(" " + p))
	}
	archivo, n := archivoYLinea(h)
	cerrar := lin(fmt.Sprintf(` para cerrarlo: hoom finding resolve %s --as corregido|refutado --evidence "<por que>"`, h.ID))
	if archivo == "" {
		d.vacia()
		d.poner(cerrar)
		return
	}
	d.vacia()
	cabeza := " " + archivo
	if n > 0 {
		cabeza += ":" + strconv.Itoa(n)
	}
	d.poner(lin(cabeza))
	var ctx []linea
	a := m.archivo
	ruta, nl := m.ArchivoElegido()
	resuelto, _ := m.rutaHallazgo(h)
	switch {
	case fueraPorLaForma(resuelto):
		ctx = []linea{linCon(" el archivo "+strconv.Quote(archivo)+" esta fuera del proyecto", rojo)}
	case a == nil || a.Ruta != ruta || a.Linea != nl:
		ctx = []linea{lin(" leyendo " + archivo + "...")}
	case a.Fuera:
		ctx = []linea{linCon(" el archivo "+strconv.Quote(archivo)+" esta fuera del proyecto", rojo)}
	case a.Err != nil:
		ctx = []linea{linCon(" no se pudo leer "+archivo+": "+unaLinea(a.Err.Error()), rojo)}
	}
	// the document's other lines: what is there, the empty line and the
	// closing line (wrapped)
	resto := &doc{wi: d.wi}
	resto.vacia()
	resto.poner(cerrar)
	if ctx == nil {
		otras := len(d.lineas) + len(resto.lineas)
		k := min(ContextoArchivo, max(0, (hi-otras-1)/2))
		desde, hasta := n-k, n+k
		if n == 0 {
			desde, hasta = 1, 2*k+1
		}
		var mostrar []int
		for i, l := range a.Lineas {
			num := a.Desde + i
			if num >= desde && num <= hasta {
				mostrar = append(mostrar, i)
				_ = l
			}
		}
		ancho := 0
		for _, i := range mostrar {
			ancho = max(ancho, len(strconv.Itoa(a.Desde+i)))
		}
		for _, i := range mostrar {
			num := a.Desde + i
			marca := " "
			if n > 0 && num == n {
				marca = m.s.sel
			}
			numTxt := strconv.Itoa(num)
			ctx = append(ctx, lin(" "+marca+" "+strings.Repeat(" ", ancho-len(numTxt))+numTxt+" "+a.Lineas[i]))
		}
	}
	for _, l := range ctx {
		d.cortada(l)
	}
	d.lineas = append(d.lineas, resto.lineas...)
}
