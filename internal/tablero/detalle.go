package tablero

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
)

// clave0Actual says what the detail shows, to know when it changes.
func (m *Modelo) clave0Actual() string {
	switch {
	case m.verProyecto:
		return "p"
	case m.vista == VistaHallazgos:
		if h, ok := m.hallazgoElegido(); ok {
			return "h:" + h.ID
		}
	default:
		if s := m.elegidaDeLista(); s != "" {
			return "t:" + s
		}
	}
	return ""
}

// documentoDetalle is the detail's title, the right part of its top border
// and its document, for its box r.
func (m *Modelo) documentoDetalle(r rect) (linea, linea, []docLinea) {
	wi, hi := max(1, r.w-2), max(1, r.h-2)
	d := &doc{wi: wi}
	switch {
	case m.verProyecto:
		if m.proyecto != nil {
			for _, l := range m.contenido1() {
				d.poner(l)
			}
			d.vacia()
		}
		m.docProyecto(d)
		return lin("Proyecto"), nil, d.lineas
	case m.vista == VistaHallazgos:
		if h, ok := m.hallazgoElegido(); ok {
			m.docHallazgo(d, h, hi)
			return lin("hallazgo " + h.ID), nil, d.lineas
		}
	default:
		if c := m.tarjeta(m.elegidaDeLista()); c != nil {
			m.docTarjeta(d, c)
			t, der := m.bordeTarjeta(c, r.w)
			return lin(t), lin(der), d.lineas
		}
	}
	return nil, nil, nil
}

// bordeTarjeta is the title and the right part of the top border of a card's
// detail of w cells (the package doc, "The detail").
func (m *Modelo) bordeTarjeta(c *boardcmd.Card, w int) (string, string) {
	titulo := limpio(tituloDe(c))
	partes := []string{c.Slug}
	if r := c.Item.Riesgo; r != "" && r != "normal" {
		partes = append(partes, " · riesgo "+r)
	}
	if c.Item.Auto == item.AutoHastaHumano {
		partes = append(partes, " · piloto encendido")
	}
	for k := len(partes); k >= 1; k-- {
		der := strings.Join(partes[:k], "")
		if celdas(titulo)+1+celdas(limpio(der))+1 <= w-2 {
			return titulo, der
		}
	}
	if n := w - 2 - celdas(limpio(c.Slug)) - 2; n >= 2 {
		return cortar(titulo, n, m.s.corte), c.Slug
	}
	return cortar(titulo, w-2, m.s.corte), ""
}

// conDerecha is izq, spaces and der with one space after it, in wi cells
// (at least one space between them; the box cuts what does not fit).
func conDerecha(izq, der linea, wi int) linea {
	n := max(1, wi-celdas(izq.limpia().plano())-celdas(der.limpia().plano())-1)
	return append(append(izq.mas(strings.Repeat(" ", n)), der...), tramo{texto: " "})
}

// ---------------------------------------------------------------- a card

// The styles of the detail's labels.
const (
	estiloEtiqueta = negrita | gris
	estiloTeToca   = negrita | amarillo
)

func (m *Modelo) docTarjeta(d *doc, c *boardcmd.Card) {
	g := m.grupoDe(c)
	p := m.pasoDe(c)
	if m.ancha() {
		m.docTarjetaAncha(d, c, g, p)
	} else {
		m.docTarjetaAngosta(d, c, g, p)
	}
	d.vacia()
	m.docResto(d, c)
}

// fraseRol is AHORA's phrase of a card whose role runs.
func fraseRol(rol string) string {
	switch rol {
	case "arquitecto":
		return "El arquitecto está escribiendo el spec."
	case "test-writer":
		return "El test-writer está escribiendo los tests."
	case "writer":
		return "El writer está escribiendo el código."
	case "reviewer":
		return "El reviewer está revisando el cambio."
	}
	return "El " + rolDe(rol) + " está trabajando."
}

// ahoraDe is AHORA's phrase and gray line ("" without one), as the mode
// writes them (the package doc, "The detail of a card").
func (m *Modelo) ahoraDe(c *boardcmd.Card) (string, string) {
	plain := unaLinea(c.Plain)
	switch {
	case c.Running != nil:
		r := c.Running
		var partes []string
		if r.Provider != "" {
			partes = append(partes, r.Provider)
		}
		partes = append(partes, "empezo "+r.StartedAt.In(m.ahora.Location()).Format("15:04"))
		if e := etapa(r.Stage, r.Step, r.Steps); e != "" {
			partes = append(partes, e)
		}
		return m.fijo(fraseRol(r.Role)), strings.Join(partes, " · ")
	case m.pilotoVivo(c.Slug):
		return m.fijo("El piloto está decidiendo el siguiente paso."), "hoom piloto " + c.Slug
	case c.Interrupted != nil:
		i := c.Interrupted
		return m.fijo(fmt.Sprintf("El trabajo del %s se cortó en el paso %d de %d.", rolDe(i.Role), i.Step, i.Steps)),
			"ya no corre: R lo reanuda"
	}
	if pa, ok := m.paradaDe(c); ok {
		return m.fijo("El piloto se detuvo."), pa.motivo
	}
	switch {
	case c.Red != nil && !c.Red.Esperado:
		gr := ""
		if m.experto {
			gr = unaLinea(c.Red.Reason)
		}
		return unaLinea(c.Red.Plain), gr
	case c.Red != nil:
		return m.fijo("Los tests están escritos y fallan, como se espera."), m.fijo("falta el código")
	case c.Column == boardcmd.ColTuAprobacion:
		return m.fijo("El spec está listo y espera tu firma."), plain
	case c.Column == boardcmd.ColTuAceptacion:
		return m.fijo("Está lista para que la pruebes y la aceptes."), plain
	case c.Column == boardcmd.ColBacklog:
		return m.fijo("Todavía no empezó."), plain
	case c.Column == boardcmd.ColHecho:
		return m.fijo("Terminada."), plain
	}
	return plain, ""
}

// ayudaPaso is TE TOCA's help of the step p.
func ayudaPaso(p paso) string {
	if p.seguir {
		return "el piloto la lleva hasta tu aceptacion o hasta que algo lo frene"
	}
	switch p.act.ID {
	case boardcmd.ActAprobar:
		return "hoom no firma por vos"
	case boardcmd.ActIntegrar:
		return "hoom no acepta por vos"
	case boardcmd.ActReanudar:
		return "retoma la sesion que se corto"
	case boardcmd.ActGuardar:
		return "guarda el trabajo de la tarjeta en su rama"
	}
	return "se abre el pedido para revisarlo antes de lanzar"
}

// sigueDe is SIGUE's phrase and, without the pilot where it would carry the
// card, its gray line (CA-710).
func (m *Modelo) sigueDe(c *boardcmd.Card) (string, string) {
	on := c.Item.Auto == item.AutoHastaHumano
	sugerir := ""
	if !on {
		sugerir = "con el piloto seguiria sola: hoom item set " + c.Slug + " --auto hasta-humano"
	}
	var f string
	switch c.Column {
	case boardcmd.ColBacklog:
		return m.fijo("el arquitecto escribe el spec; despues te toca firmarlo"), ""
	case boardcmd.ColArquitecto:
		return m.fijo("te toca firmar el spec"), ""
	case boardcmd.ColTuAprobacion:
		f = "al firmar, te toca pedir los tests al test-writer"
		if on {
			f = "al firmar, el piloto pide los tests y sigue solo hasta tu aceptación"
		}
	case boardcmd.ColTestWriter:
		f = "despues, te toca pedir el código al writer"
		if on {
			f = "el piloto pide el código al writer y sigue hasta tu aceptación"
		}
	case boardcmd.ColWriter:
		f = "despues, te toca pedir la review al reviewer"
		if on {
			f = "el piloto pide la review; si algo bloquea, hace una ronda de hallazgos"
		}
	case boardcmd.ColReview:
		f = "si nada bloquea, te toca aceptar"
		if on {
			f = "si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos"
		}
	case boardcmd.ColTuAceptacion:
		return m.fijo("al aceptar, hoom integra y la tarjeta pasa a Hecho"), ""
	case boardcmd.ColHecho:
		return m.fijo("nada: la tarjeta esta terminada"), ""
	default:
		return "", ""
	}
	return m.fijo(f), sugerir
}

// nombresCamino are the steps of the path, one per column.
var nombresCamino = []string{"Pedido", "Spec", "Tu firma", "Tests", "Código", "Review", "Tu aceptación", "Hecho"}

// nombreCamino is the step's name of the column i, as the mode writes it.
func (m *Modelo) nombreCamino(i int) string {
	if i < 0 || i >= len(nombresCamino) {
		return ""
	}
	return m.fijo(nombresCamino[i])
}

// estadoCamino is the state the path's line of the card's column says.
func (m *Modelo) estadoCamino(c *boardcmd.Card, g Grupo) string {
	switch g {
	case GrupoTeToca:
		return "te toca a vos"
	case GrupoCorriendo:
		if c.Running == nil {
			return "piloto, ahora"
		}
		return rolDe(c.Running.Role) + ", ahora"
	case GrupoDetenida:
		return "se detuvo"
	case GrupoEnCola:
		return "sin empezar"
	}
	return ""
}

// lineaTeToca is TE TOCA's line after its label: "enter <label> · <help>"
// or "nada: <text>".
func lineaTeToca(p paso) linea {
	if !p.hay() {
		return lin("nada: " + unaLinea(p.texto))
	}
	return lin("enter "+strings.ToLower(p.etiqueta())).masCon(" · "+ayudaPaso(p), gris)
}

func estiloDeTeToca(p paso) estilo {
	if p.hay() {
		return estiloTeToca
	}
	return estiloEtiqueta
}

func (m *Modelo) docTarjetaAncha(d *doc, c *boardcmd.Card, g Grupo, p paso) {
	wi := d.wi
	// AHORA
	d.cortada(conDerecha(lin(" ").masCon("AHORA", estiloEtiqueta), linCon(string(g), estiloNombre(g)), wi))
	frase, grisL := m.ahoraDe(c)
	d.poner(lin(" ").masCon(frase, negrita))
	if grisL != "" {
		d.poner(lin(" ").masCon(grisL, gris))
	}
	if r := c.Running; r != nil {
		if limite, ok := limiteDe(r); ok {
			m.barraDeTiempo(d, m.ahora.Sub(r.StartedAt), limite)
		}
	}
	if m.experto {
		d.poner(lin(" siguiente: " + unaLinea(c.Next)))
		for _, x := range c.Missing {
			d.poner(lin(" missing: " + unaLinea(x)))
		}
	}
	// TE TOCA
	d.vacia()
	d.poner(lin(" ").masCon("TE TOCA", estiloDeTeToca(p)))
	d.poner(append(lin(" "), lineaTeToca(p)...))
	// SIGUE
	d.vacia()
	d.poner(lin(" ").masCon("SIGUE", estiloEtiqueta))
	sigue, sugerir := m.sigueDe(c)
	d.poner(lin(" " + sigue))
	if sugerir != "" {
		d.poner(lin(" ").masCon(sugerir, gris))
	}
	// EL CAMINO
	d.vacia()
	i := indiceColumna(c.Column)
	d.cortada(conDerecha(lin(" ").masCon("EL CAMINO", estiloEtiqueta), lin(fmt.Sprintf("paso %d de 8", i+1)), wi))
	for j := range boardcmd.Columns {
		s, e := m.signoCamino(i, j, g)
		l := lin("  ").masCon(s, e).mas(" ")
		if j != i {
			d.cortada(l.mas(m.nombreCamino(j)))
			continue
		}
		l = l.mas(rellenar(m.nombreCamino(j), 15))
		if est := m.estadoCamino(c, g); est != "" {
			l = l.masCon(est, estiloGrupo(g))
		}
		d.cortada(conDerecha(l, linCon(m.fijo("← acá"), estiloGrupo(g)), wi))
	}
}

func (m *Modelo) docTarjetaAngosta(d *doc, c *boardcmd.Card, g Grupo, p paso) {
	wi := d.wi
	// AHORA: the phrase cut so the group fits with two spaces before it
	frase, _ := m.ahoraDe(c)
	nombre := linCon(string(g), estiloNombre(g))
	cabeza := lin(" ").masCon("AHORA", estiloEtiqueta).mas("   ")
	lugar := wi - 9 - 2 - celdas(string(g)) - 1
	l := cabeza
	if lugar >= 1 {
		l = l.masCon(cortar(limpio(frase), lugar, m.s.corte), negrita)
	}
	d.cortada(conDerecha(l, nombre, wi))
	d.cortada(append(lin(" ").masCon("TE TOCA", estiloDeTeToca(p)).mas(" "), lineaTeToca(p)...))
	sigue, _ := m.sigueDe(c)
	d.cortada(lin(" ").masCon("SIGUE", estiloEtiqueta).mas("   " + sigue))
	i := indiceColumna(c.Column)
	cam := append(lin(" ").masCon("CAMINO", estiloEtiqueta).mas("  "), m.camino(c, g)...)
	d.cortada(cam.mas(" paso " + strconv.Itoa(i+1) + " de 8: " + m.nombreCamino(i)))
}

// docResto are the v2's parts B to G of a card.
func (m *Modelo) docResto(d *doc, c *boardcmd.Card) {
	// B
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
	// what does not block, in sight when accepting (ciclo-de-review-acotado)
	if c.Pendientes.Texto != "" {
		d.poner(lin(" " + unaLinea(c.Pendientes.Texto)))
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

// barraDeTiempo puts AHORA's " tiempo " line: a bar of 20 cells, f =
// 20 * trans / limite of them run (the exact quotient rounded down, at least
// 0 and at most 20), then the time and the limit. limite is above 0
// (limiteDe).
func (m *Modelo) barraDeTiempo(d *doc, trans, limite time.Duration) {
	f := 0
	switch {
	case trans >= limite:
		f = 20
	case trans > 0:
		// 20 * trans in 128 bits, so it neither overflows nor rounds; its
		// high word is at most 19 and below limite, as Div64 needs
		hi, lo := bits.Mul64(20, uint64(trans))
		q, _ := bits.Div64(hi, lo, uint64(limite))
		f = int(q)
	}
	l := lin(" tiempo ").masCon(strings.Repeat(m.s.barraHecha, f), cian).masCon(strings.Repeat(m.s.barraFalta, 20-f), gris)
	d.poner(l.mas(" " + reloj(trans) + " de " + reloj(limite)))
}
