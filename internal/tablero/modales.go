package tablero

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
	"github.com/hoomdev/hoomai/internal/valores"
)

// lineaModal is one text line of a modal, with what a click on it hits.
type lineaModal struct {
	l       linea
	marca   bool
	campo   int  // the Pedir field it is the line of (-1: none)
	botones bool // the buttons' line
	accion  int  // Acciones: the index of its action (-1: none)
}

func lm(l linea) lineaModal { return lineaModal{l: l, campo: -1, accion: -1} }

// modalAncho is the modals' width.
func (m *Modelo) modalAncho() int { return min(72, m.c.Ancho-4) }

// botones is the buttons' line in c cells: the chosen one between brackets,
// centered; and where each button starts and ends in the text.
func botones(etiquetas [2]string, elegido, c int) (linea, [2][2]int) {
	var b [2]string
	for i, e := range etiquetas {
		if i == elegido {
			b[i] = "[ " + e + " ]"
		} else {
			b[i] = "  " + e + "  "
		}
	}
	t := b[0] + "   " + b[1]
	pad := max(0, (c-celdas(t))/2)
	var rangos [2][2]int
	rangos[0] = [2]int{pad, pad + celdas(b[0])}
	rangos[1] = [2]int{pad + celdas(b[0]) + 3, pad + celdas(t)}
	return lin(strings.Repeat(" ", pad) + t), rangos
}

// ---------------------------------------------------------------- Pedir

// origenTexto are the words of an origin of the action.
func origenTexto(o string) string {
	switch o {
	case valores.OrigenHoomYAML:
		return "de hoom.yaml"
	case valores.OrigenTarjeta:
		return "de la tarjeta"
	case valores.OrigenProvider:
		return "del provider"
	}
	return ""
}

// selector is what a selector's line shows.
type selector struct {
	etiqueta string
	valor    string
	origen   string
	gris     bool
}

func (m *Modelo) selectoresPedir(p *pedir) []selector {
	etiquetas := map[int]string{campoProvider: "Provider", campoModelo: "Modelo", campoEsfuerzo: "Esfuerzo",
		campoTope: "Tope", campoTiempo: m.s.tiempoLimite}
	origenes := map[int]string{campoProvider: p.accion.Origen.Provider, campoModelo: p.accion.Origen.Model,
		campoEsfuerzo: p.accion.Origen.Effort, campoTope: p.accion.Origen.BudgetUSD, campoTiempo: p.accion.Origen.Timeout}
	var out []selector
	for _, c := range selectores {
		s := selector{etiqueta: etiquetas[c]}
		if g, why, v := m.gris(p, c); g {
			s.gris, s.origen = true, why
			s.valor = m.s.flechaIzq + " " + v + " " + m.s.flechaDer
			if c == campoProvider {
				s.valor = v
			}
			out = append(out, s)
			continue
		}
		if p.otro != nil && p.otroCampo == c {
			t := p.otro
			s.valor = string(t.r[:t.cursor]) + m.s.cursor + string(t.r[t.cursor:])
		} else {
			ops := m.opciones(p, c)
			nombre := p.valor[c]
			if i := m.elegida(p, c, ops); i >= 0 {
				nombre = ops[i].nombre
			}
			s.valor = m.s.flechaIzq + " " + nombre + " " + m.s.flechaDer
		}
		cambiado := p.valor[c] != p.propuesto[c]
		if c == campoProvider {
			cambiado = p.prov != p.provPropuesto
		}
		if cambiado {
			s.origen = "elegido"
		} else {
			s.origen = origenTexto(origenes[c])
		}
		out = append(out, s)
	}
	return out
}

// ayudaPedir is the selectors' help line.
func (m *Modelo) ayudaPedir(p *pedir) string {
	nota := func(c int) string {
		if g, _, _ := m.gris(p, c); g {
			return ""
		}
		ops := m.opciones(p, c)
		if i := m.elegida(p, c, ops); i >= 0 && ops[i].nota {
			return ops[i].linea
		}
		return ""
	}
	switch p.foco {
	case campoPedido, campoBotones:
		return ""
	case campoProvider:
		if n := nota(campoModelo); n != "" {
			return n
		}
		return nota(campoEsfuerzo)
	}
	ops := m.opciones(p, p.foco)
	i := m.elegida(p, p.foco, ops)
	if i < 0 {
		return ""
	}
	o := ops[i]
	switch {
	case o.gris:
		return o.why
	case o.nota:
		return o.linea
	case o.linea != "":
		return o.nombre + ": " + o.linea
	}
	return ""
}

// lineasPedir are the Pedir modal's text lines in c cells at the step paso
// of what does not fit (0: everything).
func (m *Modelo) lineasPedir(p *pedir, c, paso int) []lineaModal {
	var out []lineaModal
	vacia := func() {
		if paso < 2 {
			out = append(out, lm(lin("")))
		}
	}
	if !p.revisor {
		n := 4
		switch {
		case paso >= 5:
			n = 1
		case paso >= 3:
			n = 4 - (paso - 2)
		}
		etiqueta := linCon("Pedido", normal)
		if p.pedidoVacio {
			etiqueta = linCon("Pedido", rojo)
		}
		foco := p.foco == campoPedido && p.otro == nil && p.lista == nil
		arriba := lin(m.s.esqSI + strings.Repeat(m.s.linea, max(0, c-2)) + m.s.esqSD)
		if paso >= 6 {
			arriba = append(append(lin(m.s.esqSI+" "), etiqueta...), tramo{texto: " " + strings.Repeat(m.s.linea, max(0, c-10)) + m.s.esqSD})
			out = append(out, lineaModal{l: arriba, marca: foco, campo: campoPedido, accion: -1})
		} else {
			out = append(out, lineaModal{l: etiqueta, marca: foco, campo: campoPedido, accion: -1})
			out = append(out, lineaModal{l: arriba, campo: campoPedido, accion: -1})
		}
		wb := max(1, c-4)
		var ls []string
		if foco {
			t := &p.pedido
			ls = envolverDuro(string(t.r[:t.cursor])+m.s.cursor+string(t.r[t.cursor:]), wb)
			fin := len(envolverDuro(string(t.r[:t.cursor])+m.s.cursor, wb))
			ini := max(0, fin-n)
			ls = ls[ini:min(fin, len(ls))]
		} else {
			ls = envolverDuro(p.pedido.valor(), wb)
			if len(ls) > n {
				ls = ls[:n]
			}
		}
		for len(ls) < n {
			ls = append(ls, "")
		}
		for _, t := range ls {
			out = append(out, lineaModal{l: lin(m.s.lado + " " + rellenar(cortar(limpio(t), wb, m.s.corte), wb) + " " + m.s.lado),
				campo: campoPedido, accion: -1})
		}
		out = append(out, lineaModal{l: lin(m.s.esqII + strings.Repeat(m.s.linea, max(0, c-2)) + m.s.esqID),
			campo: campoPedido, accion: -1})
	}
	sels := m.selectoresPedir(p)
	v := 26
	for _, s := range sels {
		v = max(v, celdas(s.valor)+2)
	}
	// the origins of the enabled selectors go all or none; a gray one's why
	// that does not fit goes on its own line under it
	conOrigen := true
	for _, s := range sels {
		if !s.gris && s.origen != "" && 15+v+celdas(s.origen) > c {
			conOrigen = false
		}
	}
	for i, s := range sels {
		campo := selectores[i]
		e := normal
		if s.gris {
			e = gris
		}
		l := linCon(rellenar(s.etiqueta, 15)+s.valor, e)
		cabe := 15+v+celdas(s.origen) <= c
		if s.origen != "" && (s.gris && cabe || !s.gris && conOrigen) {
			l = l.mas(strings.Repeat(" ", 15+v-celdas(rellenar(s.etiqueta, 15)+s.valor))).masCon(s.origen, gris)
		}
		marca := p.foco == campo && p.lista == nil
		if s.gris {
			out = append(out, lineaModal{l: l, campo: -1, accion: -1})
			if s.origen != "" && !cabe {
				out = append(out, lm(linCon(strings.Repeat(" ", 15)+s.origen, gris)))
			}
		} else {
			out = append(out, lineaModal{l: l, marca: marca, campo: campo, accion: -1})
		}
	}
	if paso < 1 {
		out = append(out, lm(linCon(strings.Repeat(" ", 15)+m.ayudaPedir(p), gris)))
	}
	vacia()
	out = append(out, m.lineasEjecutar(m.argsPedir(p), c, paso >= 7)...)
	vacia()
	bl, _ := botones([2]string{p.accion.Label, "Cancelar"}, p.boton, c)
	out = append(out, lineaModal{l: bl, marca: p.foco == campoBotones, botones: true, campo: campoBotones, accion: -1})
	return out
}

// lineasEjecutar are "Se va a ejecutar:" and the command's lines; junto
// puts the label on the command's first line.
func (m *Modelo) lineasEjecutar(args []string, c int, junto bool) []lineaModal {
	if junto {
		var out []lineaModal
		for _, l := range lineasComando(args, c, "Se va a ejecutar: ", m.c.Windows, m.s.corte) {
			out = append(out, lm(lin(l)))
		}
		return out
	}
	out := []lineaModal{lm(lin("Se va a ejecutar:"))}
	for _, l := range lineasComando(args, c, "", m.c.Windows, m.s.corte) {
		out = append(out, lm(lin(l)))
	}
	return out
}

// pasosPedir is how many steps of what does not fit Pedir has.
const pasosPedir = 8

// ---------------------------------------------------------------- Firma and Guardar

// confirmacion is the Firma or the Guardar modal.
type confirmacion struct {
	slug   string
	accion boardcmd.Action
	firma  bool     // aprobar or integrar; false: guardar
	verbo  []string // what the action's button runs
	boton  int      // the chosen button
	texto  string
	paths  []string
	spec   string // sha256 of the spec the card's detail showed; "" = not read
}

// etiquetas are its buttons: Firma's Cancelar first, Guardar's the action.
func (cf *confirmacion) etiquetas() [2]string {
	if cf.firma {
		return [2]string{"Cancelar", cf.accion.Label}
	}
	return [2]string{cf.accion.Label, "Cancelar"}
}

// ejecuta says whether the chosen button is the action's.
func (cf *confirmacion) ejecuta() bool { return cf.firma == (cf.boton == 1) }

func (m *Modelo) abrirConfirmacion(c *boardcmd.Card, a boardcmd.Action, firma bool, verbo []string) {
	cf := &confirmacion{slug: c.Slug, accion: a, firma: firma, verbo: verbo}
	if firma {
		cf.texto = tarjetacmd.TextoFirma(*c, a)
		cf.spec = m.huellaSpec(c.Slug)
	} else {
		cf.paths = rutasSinGuardar(c, a)
	}
	m.conf = cf
	m.modal = ModalGuardar
	if firma {
		m.modal = ModalFirma
	}
}

func (m *Modelo) lineasConfirmacion(cf *confirmacion, c, paso int, paths []string) []lineaModal {
	var out []lineaModal
	vacia := func() {
		if paso < 1 {
			out = append(out, lm(lin("")))
		}
	}
	if cf.firma {
		for _, l := range envolverPalabras(cf.texto, c) {
			out = append(out, lm(lin(l)))
		}
	} else {
		out = append(out, lm(lin("Se guarda en git:")))
		for _, p := range paths {
			out = append(out, lm(lin(p)))
		}
	}
	vacia()
	out = append(out, m.lineasEjecutar(cf.verbo, c, paso >= 2)...)
	vacia()
	bl, _ := botones(cf.etiquetas(), cf.boton, c)
	out = append(out, lineaModal{l: bl, marca: true, botones: true, campo: campoBotones, accion: -1})
	return out
}

// lineasConfirmacionQueEntran are the lines of the confirmation for at
// most alto lines: the empty lines go first, then the label joins the
// command, then guardar's paths that do not fit say how many they are.
func (m *Modelo) lineasConfirmacionQueEntran(cf *confirmacion, c, alto int) []lineaModal {
	var ls []lineaModal
	for paso := 0; paso <= 2; paso++ {
		if ls = m.lineasConfirmacion(cf, c, paso, cf.paths); len(ls) <= alto {
			return ls
		}
	}
	if !cf.firma {
		fijas := len(ls) - len(cf.paths)
		if n := alto - fijas; n >= 1 && len(cf.paths) > n {
			paths := append(cf.paths[:n-1:n-1], fmt.Sprintf("y %d rutas mas", len(cf.paths)-(n-1)))
			return m.lineasConfirmacion(cf, c, 2, paths)
		}
	}
	return ls
}

func (m *Modelo) enConfirmacion(t Tecla) []Orden {
	cf := m.conf
	switch t {
	case TeclaIzq, TeclaDer, TeclaTab, TeclaShiftTab:
		cf.boton = 1 - cf.boton
	case TeclaEsc:
		m.cerrarModal()
	case TeclaEnter:
		if !cf.ejecuta() {
			m.cerrarModal()
			return nil
		}
		if why, ok := m.vigente(cf); !ok {
			m.cerrarModal()
			m.avisar(why, rojo, DuracionAviso)
			return nil
		}
		if cf.firma && !m.firmaCabe() {
			m.avisar("agranda la terminal: la firma no entra entera", rojo, DuracionAviso)
			return nil
		}
		la := m.lanzar(cf.slug, "", cf.verbo)
		m.registrarVerbo(la, cf.accion.Label, lineaVerbo(cf.verbo, m.c.Windows))
		m.cerrarModal()
		return []Orden{la.o}
	}
	return nil
}

// firmaCabe says whether the open signature fits whole on the screen.
func (m *Modelo) firmaCabe() bool {
	c := m.modalAncho() - 6
	alto := m.c.Alto - 2 - 2
	return len(m.lineasConfirmacionQueEntran(m.conf, c, alto)) <= alto
}

// rutasSinGuardar are the paths guardar would save: the action's, else the
// card's unsynced ones.
func rutasSinGuardar(c *boardcmd.Card, a boardcmd.Action) []string {
	if len(a.Paths) > 0 {
		return a.Paths
	}
	return c.Unsynced
}

// huellaSpec is the sha256 of slug's spec as the card's detail has it, ""
// when that detail is not read.
func (m *Modelo) huellaSpec(slug string) string {
	d := m.detalle
	if d == nil || d.Slug != slug || d.Err != nil || !d.Detalle.Spec.Exists {
		return ""
	}
	sum := sha256.Sum256([]byte(d.Detalle.Spec.Markdown))
	return hex.EncodeToString(sum[:])
}

// vigente says whether the confirmation still shows what the data says now.
func (m *Modelo) vigente(cf *confirmacion) (string, bool) {
	cambio := fmt.Sprintf("cambio lo que ibas a confirmar en %s: vuelve a abrirlo", cf.slug)
	c := m.tarjeta(cf.slug)
	if c == nil {
		return fmt.Sprintf("la tarjeta %s ya no existe", cf.slug), false
	}
	a, ok := c.Action(cf.accion.ID)
	switch {
	case !ok:
		return cambio, false
	case !a.Enabled:
		return a.Why, false
	case cf.firma:
		if tarjetacmd.TextoFirma(*c, a) != cf.texto {
			return cambio, false
		}
		if cf.spec != "" && m.huellaSpec(cf.slug) != cf.spec {
			return cambio, false
		}
	case !slices.Equal(rutasSinGuardar(c, a), cf.paths):
		return cambio, false
	}
	return "", true
}

// revisarConfirmacion closes the open confirmation when the data it was
// opened on changed.
func (m *Modelo) revisarConfirmacion() {
	if m.conf == nil {
		return
	}
	if why, ok := m.vigente(m.conf); !ok {
		m.cerrarModal()
		m.avisar(why, rojo, DuracionAviso)
	}
}

// ---------------------------------------------------------------- Acciones

// acciones is the Acciones modal.
type acciones struct {
	slug string
	sel  int // index into the shown actions
}

func (m *Modelo) abrirAcciones(c *boardcmd.Card) {
	m.cerrarModal()
	m.acciones = &acciones{slug: c.Slug}
	m.modal = ModalAcciones
	for i, a := range m.accionesVisibles(c) {
		if m.elegible(a) {
			m.acciones.sel = i
			break
		}
	}
}

// accionesVisibles are the card's actions the modal lists.
func (m *Modelo) accionesVisibles(c *boardcmd.Card) []boardcmd.Action {
	var out []boardcmd.Action
	for _, a := range c.Actions {
		if a.Expert && !m.experto {
			continue
		}
		out = append(out, a)
	}
	return out
}

// elegible: an action the modal's selection can be on.
func (m *Modelo) elegible(a boardcmd.Action) bool {
	_, ok := filaDeAccion(a.ID)
	return a.Enabled && ok
}

func (m *Modelo) lineasAcciones() []lineaModal {
	c := m.tarjeta(m.acciones.slug)
	if c == nil {
		return nil
	}
	as := m.accionesVisibles(c)
	if len(as) == 0 {
		return []lineaModal{lm(linCon("no hay acciones en esta tarjeta", gris))}
	}
	var out []lineaModal
	for i, a := range as {
		tecla, verbo := " ", "sin verbo en la terminal"
		fila, conVerbo := filaDeAccion(a.ID)
		if conVerbo {
			tecla = fila.Muestra
			verbo = strings.ReplaceAll(fila.Verbo, "<slug>", c.Slug)
		}
		t := tecla + " " + a.Label + "  " + verbo
		if !a.Enabled {
			t += "  (" + a.Why + ")"
		}
		e := normal
		if !a.Enabled || !conVerbo {
			e = gris
		}
		out = append(out, lineaModal{l: linCon(unaLinea(t), e), marca: i == m.acciones.sel && m.elegible(a), campo: -1, accion: i})
	}
	return out
}

func (m *Modelo) enAcciones(t Tecla) []Orden {
	c := m.tarjeta(m.acciones.slug)
	if c == nil {
		m.cerrarModal()
		return nil
	}
	as := m.accionesVisibles(c)
	mover := func(d int) {
		for i := m.acciones.sel + d; i >= 0 && i < len(as); i += d {
			if m.elegible(as[i]) {
				m.acciones.sel = i
				return
			}
		}
	}
	switch t {
	case "j", TeclaAbajo:
		mover(1)
	case "k", TeclaArriba:
		mover(-1)
	case TeclaEsc:
		m.cerrarModal()
	case TeclaEnter:
		if i := m.acciones.sel; i >= 0 && i < len(as) && m.elegible(as[i]) {
			fila, _ := filaDeAccion(as[i].ID)
			return m.accion(c, fila)
		}
	}
	return nil
}

// ---------------------------------------------------------------- Ayuda

// ayuda is the Ayuda modal.
type ayuda struct {
	desde  Lugar
	scroll int
}

func (m *Modelo) abrirAyuda(l Lugar) {
	m.cerrarModal()
	m.ayuda = &ayuda{desde: l}
	m.modal = ModalAyuda
}

// grupos are the help's panel groups, in order, with their title.
var grupos = []struct {
	l      Lugar
	titulo string
}{
	{LugarProyecto, "[1] Proyecto"}, {LugarTarjetas, "[2] Tarjetas"}, {LugarCorriendo, "[3] Corriendo"},
	{LugarHallazgos, "[4] Hallazgos"}, {LugarVerProyecto, "[0] el proyecto"}, {LugarVerTarjeta, "[0] una tarjeta"},
	{LugarVerCorriendo, "[0] lo que corre"}, {LugarVerHallazgo, "[0] un hallazgo"},
}

// enTodasPartes says whether the row works in every panel Lugar.
func enTodasPartes(a Atajo) bool {
	for _, l := range LugaresPaneles {
		if !contieneLugar(a.Lugares, l) {
			return false
		}
	}
	return true
}

func (m *Modelo) filaAyuda(a Atajo) linea {
	t := "  " + rellenar(m.s.conFlechas(a.Muestra), 18) + a.Hace
	if a.Verbo != "" {
		t += "  " + a.Verbo
	}
	return lin(t)
}

func (m *Modelo) lineasAyuda() []lineaModal {
	var out []lineaModal
	grupo := func(titulo string, filas []Atajo) {
		if len(out) > 0 {
			out = append(out, lm(lin("")))
		}
		out = append(out, lm(lin(titulo)))
		for _, a := range filas {
			out = append(out, lm(m.filaAyuda(a)))
		}
	}
	delPanel := func(l Lugar) []Atajo {
		var out []Atajo
		for _, a := range AtajosDe(l) {
			if !enTodasPartes(a) {
				out = append(out, a)
			}
		}
		if l == LugarTarjetas {
			out = append(out, AtajosDe(LugarBusqueda)...)
		}
		return out
	}
	desde := m.ayuda.desde
	titulo := "[0]"
	for _, g := range grupos {
		if g.l == desde {
			titulo = g.titulo
		}
	}
	grupo(titulo, delPanel(desde))
	var todas []Atajo
	for _, a := range Atajos {
		if enTodasPartes(a) {
			todas = append(todas, a)
		}
	}
	grupo("En todas partes", todas)
	for _, g := range grupos {
		if g.l != desde {
			grupo(g.titulo, delPanel(g.l))
		}
	}
	var modales []Atajo
	for _, a := range Atajos {
		for _, l := range LugaresModales {
			if contieneLugar(a.Lugares, l) {
				modales = append(modales, a)
				break
			}
		}
	}
	grupo("Modales", modales)
	out = append(out, lm(lin("")), lm(lin("Mouse")))
	if m.c.Mouse {
		for _, l := range []string{
			"  clic              foco al panel; en una fila, la elige",
			"  rueda             mueve la seleccion; en [0] y en la ayuda, desplaza",
			"  clic en un modal  enfoca un campo, elige una opcion o un boton",
			"  para seleccionar texto con el mouse, manten Option (macOS) o Shift",
		} {
			out = append(out, lm(lin(l)))
		}
	} else {
		out = append(out, lm(lin("  apagado: HOOM_MOUSE=0")))
	}
	return out
}

func (m *Modelo) enAyuda(t Tecla) {
	switch t {
	case "j", TeclaAbajo:
		m.desplazarAyuda(1)
	case "k", TeclaArriba:
		m.desplazarAyuda(-1)
	case TeclaPgAbajo:
		m.desplazarAyuda(max(1, m.c.Alto-4))
	case TeclaPgArriba:
		m.desplazarAyuda(-max(1, m.c.Alto-4))
	case TeclaEsc, "?":
		m.cerrarModal()
	}
}

func (m *Modelo) desplazarAyuda(d int) {
	ls := m.lineasAyuda()
	alto := min(len(ls)+2, m.c.Alto-2) - 2
	m.ayuda.scroll = max(0, min(m.ayuda.scroll+d, len(ls)-alto))
}

// ---------------------------------------------------------------- the modal on screen

// modalEnPantalla is the open modal as it is drawn: its box, its title, its
// text lines (those in view) and its position.
type modalEnPantalla struct {
	r      rect
	titulo string
	lineas []lineaModal
	pos    string
	// the list over it
	lista      rect
	listaOps   []opcion
	listaDesde int
}

// geometria is the open modal on screen (ok false without one).
func (m *Modelo) geometria() (modalEnPantalla, bool) {
	var g modalEnPantalla
	if m.modal == SinModal {
		return g, false
	}
	b := m.c.Alto - 2
	wm := m.modalAncho()
	c := wm - 6
	var ls []lineaModal
	switch m.modal {
	case ModalPedir:
		p := m.pedir
		g.titulo = p.accion.Label + " · " + p.slug
		for paso := 0; paso < pasosPedir; paso++ {
			if ls = m.lineasPedir(p, c, paso); len(ls)+2 <= b {
				break
			}
		}
	case ModalFirma, ModalGuardar:
		g.titulo = m.conf.accion.Label + " · " + m.conf.slug
		ls = m.lineasConfirmacionQueEntran(m.conf, c, b-2)
	case ModalAcciones:
		g.titulo = "Acciones · " + m.acciones.slug
		ls = m.lineasAcciones()
	case ModalAyuda:
		g.titulo = "Todas las teclas"
		ls = m.lineasAyuda()
		alto := min(len(ls)+2, b) - 2
		m.ayuda.scroll = max(0, min(m.ayuda.scroll, len(ls)-alto))
		if len(ls) > alto {
			g.pos = fmt.Sprintf("%d de %d", m.ayuda.scroll+alto, len(ls))
			ls = ls[m.ayuda.scroll : m.ayuda.scroll+alto]
		}
	}
	hm := min(len(ls)+2, b)
	if len(ls) > hm-2 {
		ls = ls[:max(0, hm-2)]
	}
	g.r = rect{x: (m.c.Ancho - wm) / 2, y: max(0, (b-hm)/2), w: wm, h: hm}
	g.lineas = ls
	if m.modal == ModalPedir && m.pedir.lista != nil {
		m.geometriaLista(&g)
	}
	return g, true
}

// geometriaLista places the open list over the Pedir modal.
func (m *Modelo) geometriaLista(g *modalEnPantalla) {
	p := m.pedir
	ops := m.opciones(p, p.lista.campo)
	fila := -1
	for i, l := range g.lineas {
		if l.campo == p.lista.campo {
			fila = i
		}
	}
	if fila < 0 {
		return
	}
	ySel := g.r.y + 1 + fila
	ultima := g.r.y + g.r.h - 2 // the modal's last text line
	x := g.r.x + 4 + 15
	w := g.r.x + g.r.w - 2 - x
	alto := min(len(ops)+2, ultima-ySel)
	y := ySel + 1
	if alto < 3 {
		alto = min(len(ops)+2, g.r.h-2)
		y = ultima - alto + 1
	}
	g.lista = rect{x: x, y: y, w: w, h: alto}
	g.listaOps = ops
	dentro := alto - 2
	if p.lista.idx < p.lista.scroll {
		p.lista.scroll = p.lista.idx
	}
	if p.lista.idx >= p.lista.scroll+dentro {
		p.lista.scroll = p.lista.idx - dentro + 1
	}
	p.lista.scroll = max(0, min(p.lista.scroll, len(ops)-dentro))
	g.listaDesde = p.lista.scroll
}

// dibujarModal draws the open modal (and its list) over the panels; with
// color the panels behind it go gray.
func (m *Modelo) dibujarModal(lz *lienzo) {
	g, ok := m.geometria()
	if !ok {
		return
	}
	fuera := []rect{g.r, {x: 0, y: m.c.Alto - 2, w: m.c.Ancho, h: 2}}
	if g.lista.w > 0 {
		fuera = append(fuera, g.lista)
	}
	lz.enGris(fuera...)
	r := g.r
	s := m.s
	lz.fijar(r.x, r.y, s.fEsqSI, 1, estiloFoco)
	n := 1 + lz.texto(r.x+1, r.y, r.w-2, " "+cortar(g.titulo, r.w-4, s.corte)+" ", estiloFoco)
	lz.repetir(r.x+n, r.y, r.w-1-n, s.fLinea, estiloFoco)
	lz.fijar(r.x+r.w-1, r.y, s.fEsqSD, 1, estiloFoco)
	for i := 0; i < r.h-2; i++ {
		y := r.y + 1 + i
		lz.fijar(r.x, y, s.fLado, 1, estiloFoco)
		marca := "  "
		var l linea
		if i < len(g.lineas) {
			l = g.lineas[i].l
			if g.lineas[i].marca {
				marca = s.sel + " "
			}
		}
		lz.texto(r.x+1, y, 3, " "+marca, normal)
		lz.poner(r.x+4, y, r.w-6, l, normal, true)
		lz.texto(r.x+r.w-2, y, 1, " ", normal)
		lz.fijar(r.x+r.w-1, y, s.fLado, 1, estiloFoco)
	}
	m.borde(lz, r.x, r.y+r.h-1, r.w, s.fEsqII, s.fLinea, s.fEsqID, g.pos, estiloFoco)
	if g.lista.w > 0 {
		m.dibujarLista(lz, g)
	}
}

// borde draws a bottom border, with "<n> de <m>" before its corner when it
// fits.
func (m *Modelo) borde(lz *lienzo, x, y, w int, izq, hor, der, pos string, e estilo) {
	lz.fijar(x, y, izq, 1, e)
	relleno := w - 2
	if pos != "" && celdas(pos)+1 <= w-2 {
		relleno = w - 2 - celdas(pos) - 1
		lz.repetir(x+1, y, relleno, hor, e)
		lz.texto(x+1+relleno, y, celdas(pos), pos, e)
		lz.fijar(x+w-2, y, hor, 1, e)
	} else {
		lz.repetir(x+1, y, relleno, hor, e)
	}
	lz.fijar(x+w-1, y, der, 1, e)
}

func (m *Modelo) dibujarLista(lz *lienzo, g modalEnPantalla) {
	r := g.lista
	s := m.s
	p := m.pedir
	lz.fijar(r.x, r.y, s.esqSI, 1, normal)
	lz.repetir(r.x+1, r.y, r.w-2, s.linea, normal)
	lz.fijar(r.x+r.w-1, r.y, s.esqSD, 1, normal)
	ancho := 0
	for _, o := range g.listaOps {
		ancho = max(ancho, celdas(o.nombre))
	}
	for i := 0; i < r.h-2; i++ {
		y := r.y + 1 + i
		k := g.listaDesde + i
		lz.fijar(r.x, y, s.lado, 1, normal)
		var l linea
		if k < len(g.listaOps) {
			o := g.listaOps[k]
			marca := " "
			if k == p.lista.idx {
				marca = s.sel
			}
			texto := o.linea
			if o.gris {
				texto = o.why
			}
			t := " " + marca + " " + rellenar(o.nombre, ancho)
			if texto != "" {
				t += "  " + texto
			}
			e := normal
			if o.gris {
				e = gris
			}
			l = linCon(t, e)
		}
		lz.poner(r.x+1, y, r.w-2, l, normal, true)
		lz.fijar(r.x+r.w-1, y, s.lado, 1, normal)
	}
	pos := ""
	if len(g.listaOps) > r.h-2 {
		pos = strconv.Itoa(p.lista.idx+1) + " de " + strconv.Itoa(len(g.listaOps))
	}
	m.borde(lz, r.x, r.y+r.h-1, r.w, s.esqII, s.linea, s.esqID, pos, normal)
}
