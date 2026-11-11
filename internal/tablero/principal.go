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

// docLinea is one line of [0]'s document: the line, and, on part D of a
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

// cortada adds l without wrapping (the panel cuts it).
func (d *doc) cortada(l linea) { d.lineas = append(d.lineas, docLinea{l: l}) }

func (d *doc) vacia() { d.lineas = append(d.lineas, docLinea{l: lin("")}) }

// clave0 says what [0] shows, to know when it changes.
func (m *Modelo) clave0Actual() string {
	switch m.izq {
	case PanelProyecto:
		return "p"
	case PanelTarjetas:
		if s := m.elegidaDe2(); s != "" {
			return "t:" + s
		}
	case PanelCorriendo:
		if s := m.elegidaDe3(); s != "" {
			return "c:" + s
		}
	case PanelHallazgos:
		if h, ok := m.hallazgoElegido(); ok {
			return "h:" + h.ID
		}
	}
	return ""
}

// documento0 is [0]'s title and document for an inner width wi and an
// inner height hi.
func (m *Modelo) documento0(wi, hi int) (string, []docLinea) {
	d := &doc{wi: wi}
	switch m.izq {
	case PanelProyecto:
		m.docProyecto(d)
		return "[0] Proyecto", d.lineas
	case PanelTarjetas:
		if c := m.tarjeta(m.elegidaDe2()); c != nil {
			m.docTarjeta(d, c)
			return "[0] " + c.Slug + " · " + c.Item.Titulo, d.lineas
		}
	case PanelCorriendo:
		if c := m.tarjeta(m.elegidaDe3()); c != nil {
			rol := ""
			if c.Running != nil {
				rol = rolDe(c.Running.Role)
			} else if c.Interrupted != nil {
				rol = rolDe(c.Interrupted.Role)
			}
			m.docCorriendo(d, c)
			return "[0] " + c.Slug + " · " + rol, d.lineas
		}
	case PanelHallazgos:
		if h, ok := m.hallazgoElegido(); ok {
			m.docHallazgo(d, h, hi)
			return "[0] hallazgo " + h.ID, d.lineas
		}
	}
	return "[0]", nil
}

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

func (m *Modelo) docTarjeta(d *doc, c *boardcmd.Card) {
	// A
	d.poner(lin(" " + m.s.queFalta + "  " + unaLinea(c.Plain)))
	if m.experto {
		d.poner(lin(" siguiente: " + unaLinea(c.Next)))
		for _, x := range c.Missing {
			d.poner(lin(" missing: " + unaLinea(x)))
		}
	}
	if act, fila, texto := m.siguientePaso(c); act != nil {
		d.poner(lin(" Siguiente paso  " + act.Label + "  [" + m.s.conFlechas(fila.Muestra) + "]"))
	} else {
		d.poner(lin(" Siguiente paso  " + unaLinea(texto)))
	}
	// what does not block, in sight when accepting (ciclo-de-review-acotado)
	if c.Pendientes.Texto != "" {
		d.poner(lin(" " + unaLinea(c.Pendientes.Texto)))
	}
	// B
	d.vacia()
	g := gasto(c.Spend)
	if c.Spend.BudgetUSD != nil {
		g += fmt.Sprintf(" de %.2f USD", *c.Spend.BudgetUSD)
	}
	b := " " + c.ColumnName + " · gasto " + g
	if c.Providers.Writer != "" {
		b += " · " + m.s.escribio + " " + c.Providers.Writer
	}
	if c.Providers.Reviewer != "" {
		b += " · " + m.s.reviso + " " + c.Providers.Reviewer
	}
	d.poner(lin(b))
	if c.Red != nil {
		motivo := c.Red.Plain
		if m.experto {
			motivo = c.Red.Reason
		}
		d.poner(linCon(" rojo: "+unaLinea(motivo), rojo))
	}
	for _, p := range c.Doctor {
		que := p.Plain
		if que == "" {
			que = p.What
		}
		d.poner(lin(" doctor: " + unaLinea(que) + ": " + unaLinea(p.Action)))
	}
	// C
	if len(c.Meter) > 0 {
		d.vacia()
		for i, s := range c.Meter {
			t := "  " + m.signo(c, i) + " " + s.Label
			if s.Detail != "" {
				t += ": " + unaLinea(s.Detail)
			}
			d.poner(lin(t))
		}
	}
	d.vacia()
	det := m.detalle
	switch {
	case det == nil || det.Slug != c.Slug:
		d.poner(lin(" leyendo la tarjeta..."))
		return
	case det.Err != nil:
		d.poner(linCon(" no se pudo leer la tarjeta: "+unaLinea(det.Err.Error()), rojo))
		return
	}
	// D
	m.docSpec(d, c, det)
	// E
	d.vacia()
	m.docHallazgosTarea(d, det)
	// F
	d.vacia()
	d.poner(lin(" Historia"))
	if len(det.Historia.Entries) == 0 {
		d.poner(lin("  sin historia todavia"))
	}
	for _, e := range det.Historia.Entries {
		que := e.Plain
		if m.experto {
			que = e.Summary
		}
		d.poner(lin("  " + e.At.UTC().Format("2006-01-02 15:04") + " " + e.Who + " " + unaLinea(que)))
	}
	// G
	d.vacia()
	d.poner(lin(" " + m.s.narracion))
	m.docSobre(d, c, det)
}

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
		if p := paso(r.Stage, r.Step, r.Steps); p != "" {
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
		if p := paso(w.Stage, w.Step, w.Steps); p != "" {
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

// ---------------------------------------------------------------- [3]'s card

func (m *Modelo) docCorriendo(d *doc, c *boardcmd.Card) {
	if r := c.Running; r != nil {
		partes := []string{rolDe(r.Role)}
		if p := paso(r.Stage, r.Step, r.Steps); p != "" {
			partes = append(partes, p)
		}
		t := reloj(m.ahora.Sub(r.StartedAt))
		if r.TimeoutS != 0 {
			t += " de " + reloj(time.Duration(r.TimeoutS)*time.Second)
		}
		partes = append(partes, t)
		if r.LastEventAt != nil {
			partes = append(partes, fmt.Sprintf("%s hace %ds", m.s.narro, segundos(m.ahora.Sub(*r.LastEventAt))))
		}
		partes = append(partes, gasto(c.Spend))
		d.poner(lin(" " + strings.Join(partes, " · ")))
	} else if i := c.Interrupted; i != nil {
		d.poner(linCon(fmt.Sprintf(" interrumpido en el paso %d de %d (%s): el %s ya no corre",
			i.Step, i.Steps, i.Stage, rolDe(i.Role)), rojo))
	}
	det := m.detalle
	if det == nil || det.Slug != c.Slug {
		d.poner(lin(" leyendo la tarjeta..."))
		return
	}
	if det.Err != nil {
		d.poner(linCon(" no se pudo leer la tarjeta: "+unaLinea(det.Err.Error()), rojo))
		return
	}
	if det.Sobre.Err != nil {
		d.poner(linCon(" no se pudo leer el sobre: "+unaLinea(det.Sobre.Err.Error()), rojo))
	}
	if len(det.Sobre.Eventos) == 0 {
		d.poner(lin(" sin eventos todavia"))
	}
	for _, e := range det.Sobre.Eventos {
		l := " " + e.TS.UTC().Format("15:04:05") + " " + e.Kind
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
