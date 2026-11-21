package tablero

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
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
	cortada bool // a line of "y despues: hoom piloto <slug>" cut in the width (Firma never signs with one)
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
			// the text being written has no origin yet
			t := p.otro
			s.valor = string(t.r[:t.cursor]) + m.s.cursor + string(t.r[t.cursor:])
			out = append(out, s)
			continue
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
func (m *Modelo) lineasPedir(p *pedir, c int, k compacto) []lineaModal {
	var out []lineaModal
	vacia := func() {
		if !k.sinVacias {
			out = append(out, lm(lin("")))
		}
	}
	if !p.sinPedido {
		n := k.pedido
		etiqueta := linCon("Pedido", normal)
		if p.pedidoVacio {
			etiqueta = linCon("Pedido", rojo)
		}
		foco := p.foco == campoPedido && p.otro == nil && p.lista == nil
		arriba := lin(m.s.esqSI + strings.Repeat(m.s.linea, max(0, c-2)) + m.s.esqSD)
		if k.etiquetaEnBorde {
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
	conOrigen := !k.sinOrigen
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
		if s.origen != "" && !k.sinOrigen && (s.gris && cabe || !s.gris && conOrigen) {
			l = l.mas(strings.Repeat(" ", 15+v-celdas(rellenar(s.etiqueta, 15)+s.valor))).masCon(s.origen, gris)
		}
		marca := p.foco == campo && p.lista == nil
		if s.gris {
			out = append(out, lineaModal{l: l, campo: -1, accion: -1})
			if s.origen != "" && !cabe && !k.sinWhyLinea {
				out = append(out, lm(linCon(strings.Repeat(" ", 15)+s.origen, gris)))
			}
		} else {
			out = append(out, lineaModal{l: l, marca: marca, campo: campo, accion: -1})
		}
	}
	if !k.sinAyuda {
		out = append(out, lm(linCon(lineaDeAyuda(m.ayudaPedir(p), c), gris)))
	}
	vacia()
	out = append(out, m.lineasEjecutar(m.argsPedir(p), c, k.junto)...)
	vacia()
	bl, _ := botones([2]string{p.accion.Label, "Cancelar"}, p.boton, c)
	out = append(out, lineaModal{l: bl, marca: p.foco == campoBotones, botones: true, campo: campoBotones, accion: -1})
	return out
}

// lineaDeAyuda is the selectors' help line t in c cells: from column 15
// when it fits there, else from the text's first column, and cut only when
// it does not fit even there (it never loses text otherwise).
func lineaDeAyuda(t string, c int) string {
	if 15+celdas(t) <= c {
		return strings.Repeat(" ", 15) + t
	}
	return t
}

// lineasEjecutar are "Se va a ejecutar:" and the command's lines; junto
// puts the label on the command's first line.
func (m *Modelo) lineasEjecutar(args []string, c int, junto bool) []lineaModal {
	if junto {
		ls, _ := m.lineasDeComando(args, c, "Se va a ejecutar: ")
		return ls
	}
	ls, _ := m.lineasDeComando(args, c, "")
	return append([]lineaModal{lm(lin("Se va a ejecutar:"))}, ls...)
}

// lineasDeComando are the lines of the command args after prefijo, laid out
// by words in at most two lines (cut when it does not fit even so), and
// whether it fits whole.
func (m *Modelo) lineasDeComando(args []string, c int, prefijo string) ([]lineaModal, bool) {
	ls, entero := lineasComando(args, c, prefijo, m.c.Windows, m.s.corte)
	out := make([]lineaModal, 0, len(ls))
	for _, l := range ls {
		out = append(out, lm(lin(m.s.enPantalla(l))))
	}
	return out, entero
}

// compacto is what the Pedir modal gives up to fit in B lines.
type compacto struct {
	sinAyuda        bool // the selectors' help line
	sinWhyLinea     bool // the own lines of the gray selectors' whys
	sinOrigen       bool // the origin words (and the whys beside their selector)
	sinVacias       bool // the empty lines
	pedido          int  // the Pedido box's text lines
	etiquetaEnBorde bool // "Pedido" in the box's top border
	junto           bool // "Se va a ejecutar:" on the command's first line
}

// pasosPedir are the steps of what does not fit, in order: "Se va a
// ejecutar", the command and the buttons never go.
var pasosPedir = []compacto{
	{pedido: 4},
	{sinAyuda: true, pedido: 4},
	{sinAyuda: true, sinWhyLinea: true, pedido: 4},
	{sinAyuda: true, sinWhyLinea: true, sinOrigen: true, pedido: 4},
	{sinAyuda: true, sinWhyLinea: true, sinOrigen: true, sinVacias: true, pedido: 4},
	{sinAyuda: true, sinWhyLinea: true, sinOrigen: true, sinVacias: true, pedido: 3},
	{sinAyuda: true, sinWhyLinea: true, sinOrigen: true, sinVacias: true, pedido: 2},
	{sinAyuda: true, sinWhyLinea: true, sinOrigen: true, sinVacias: true, pedido: 1},
	{sinAyuda: true, sinWhyLinea: true, sinOrigen: true, sinVacias: true, pedido: 1, etiquetaEnBorde: true},
	{sinAyuda: true, sinWhyLinea: true, sinOrigen: true, sinVacias: true, pedido: 1, etiquetaEnBorde: true, junto: true},
}

// pasoPedir is the first step whose lines fit in the modal's B lines, and
// whether one does.
func (m *Modelo) pasoPedir(p *pedir) (compacto, bool) {
	c := m.modalAncho() - 6
	b := m.c.Alto - 2
	for _, k := range pasosPedir {
		if len(m.lineasPedir(p, c, k))+2 <= b {
			return k, true
		}
	}
	return pasosPedir[len(pasosPedir)-1], false
}

// pedirEntero says whether the Pedir modal shows its command whole and its
// buttons: its lines fit at some step and its command fits in its two
// lines. Only then does its action's button run.
func (m *Modelo) pedirEntero(p *pedir) bool {
	k, ok := m.pasoPedir(p)
	if !ok {
		return false
	}
	prefijo := ""
	if k.junto {
		prefijo = "Se va a ejecutar: "
	}
	_, entero := lineasComando(m.argsPedir(p), m.modalAncho()-6, prefijo, m.c.Windows, m.s.corte)
	return entero
}

// ---------------------------------------------------------------- Firma, Guardar and Seguir solo

// confirmacion is the Firma, the Guardar or the Seguir solo modal.
type confirmacion struct {
	slug   string
	accion boardcmd.Action
	firma  bool     // aprobar or integrar
	seguir bool     // Seguir solo; neither: guardar
	verbo  []string // what the action's button runs
	boton  int      // the chosen button
	texto  string
	paths  []string
	spec   string // sha256 of the spec the card's detail showed; "" = not read
	piloto bool   // aprobar on a card with the pilot on: "y despues: hoom piloto <slug>"
	titulo string // Seguir solo's: the card's title
}

// etiqueta is the action's label.
func (cf *confirmacion) etiqueta() string {
	if cf.seguir {
		return etiquetaSeguir
	}
	return cf.accion.Label
}

// etiquetas are its buttons: Firma's Cancelar first, Guardar's and Seguir
// solo's the action.
func (cf *confirmacion) etiquetas() [2]string {
	if cf.firma {
		return [2]string{"Cancelar", cf.accion.Label}
	}
	return [2]string{cf.etiqueta(), "Cancelar"}
}

// textoSeguir is the text of the Seguir solo modal.
const textoSeguir = "El piloto lleva la tarjeta sola: tests, codigo, verify, review y rondas de hallazgos, " +
	"hasta tu aceptacion o hasta que algo lo frene. Nunca firma ni acepta."

// abrirSeguir opens the Seguir solo modal of c.
func (m *Modelo) abrirSeguir(c *boardcmd.Card) {
	m.cerrarModal()
	m.conf = &confirmacion{slug: c.Slug, seguir: true, verbo: []string{"piloto", c.Slug}, texto: textoSeguir,
		titulo: boardcmd.TituloDe(c)}
	m.modal = ModalSeguir
}

// conPiloto says whether the Firma of the action a on c chains the pilot.
func conPiloto(c *boardcmd.Card, a boardcmd.Action) bool {
	return a.ID == boardcmd.ActAprobar && c.Item.Auto == item.AutoHastaHumano
}

// ejecuta says whether the chosen button is the action's.
func (cf *confirmacion) ejecuta() bool { return cf.firma == (cf.boton == 1) }

func (m *Modelo) abrirConfirmacion(c *boardcmd.Card, a boardcmd.Action, firma bool, verbo []string) {
	cf := &confirmacion{slug: c.Slug, accion: a, firma: firma, verbo: verbo}
	if firma {
		cf.texto = tarjetacmd.TextoFirma(*c, a)
		cf.spec = m.huellaSpec(c.Slug)
		cf.piloto = conPiloto(c, a)
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
	if cf.firma || cf.seguir {
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
	if cf.piloto {
		ls, entero := m.lineasDeComando([]string{"piloto", cf.slug}, c, "y despues: ")
		for i := range ls {
			ls[i].cortada = !entero
		}
		out = append(out, ls...)
	}
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
	if !cf.firma && !cf.seguir {
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
		if cf.seguir {
			la := m.lanzarPiloto(cf.slug)
			m.cerrarModal()
			return []Orden{la.o}
		}
		la := m.lanzar(cf.slug, "", cf.verbo)
		m.registrarVerbo(la, cf.accion.Label, lineaVerbo(cf.verbo, m.c.Windows))
		if cf.piloto {
			la.encadenar = cf.slug
		}
		m.cerrarModal()
		return []Orden{la.o}
	}
	return nil
}

// firmaCabe says whether the open signature fits whole on the screen: every
// line in the modal's height, and "y despues: hoom piloto <slug>" (the
// process it launches by itself) not cut in the width.
func (m *Modelo) firmaCabe() bool {
	c := m.modalAncho() - 6
	alto := m.c.Alto - 2 - 2
	ls := m.lineasConfirmacionQueEntran(m.conf, c, alto)
	if len(ls) > alto {
		return false
	}
	for _, l := range ls {
		if l.cortada {
			return false
		}
	}
	return true
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
	if cf.seguir {
		if why := m.motivoSinSeguir(c); why != "" {
			return why, false
		}
		return "", true
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
		if conPiloto(c, a) != cf.piloto {
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

// gruposAyuda are the help's first three groups, with their title and the
// Lugares of their rows.
var gruposAyuda = []struct {
	titulo  string
	lugares []Lugar
}{
	{"Tarjetas", []Lugar{LugarTarjetas, LugarBusqueda}},
	{"Detalle", LugaresVer},
	{"Hallazgos", []Lugar{LugarHallazgos}},
}

// trabajaEn says whether the row works in one of ls.
func trabajaEn(a Atajo, ls []Lugar) bool {
	for _, l := range ls {
		if contieneLugar(a.Lugares, l) {
			return true
		}
	}
	return false
}

// filasAyuda are a row's lines in the help, c cells wide: wrapped at the
// words, each continuation from the Hace column (20), so no text is lost.
func (m *Modelo) filasAyuda(a Atajo, c int) []lineaModal {
	t := a.Hace
	if a.Verbo != "" {
		t += "  " + a.Verbo
	}
	cabeza := "  " + rellenar(m.s.conFlechas(a.Muestra), 18)
	ancho := max(10, c-celdas(cabeza))
	var out []lineaModal
	for i, l := range envolverPalabrasEspacios(t, ancho) {
		if i == 0 {
			out = append(out, lm(lin(cabeza+l)))
		} else {
			out = append(out, lm(lin(strings.Repeat(" ", celdas(cabeza))+l)))
		}
	}
	return out
}

func (m *Modelo) lineasAyuda() []lineaModal {
	var out []lineaModal
	c := m.modalAncho() - 6
	type grupo struct {
		titulo  string
		lugares []Lugar
	}
	// the group of where the focus is, then the other two, then Modales
	primero := 0
	switch {
	case contieneLugar(LugaresVer, m.ayuda.desde):
		primero = 1
	case m.ayuda.desde == LugarHallazgos:
		primero = 2
	}
	var orden []grupo
	orden = append(orden, grupo(gruposAyuda[primero]))
	for i, g := range gruposAyuda {
		if i != primero {
			orden = append(orden, grupo(g))
		}
	}
	orden = append(orden, grupo{"Modales", LugaresModales})
	listadas := map[int]bool{}
	for _, g := range orden {
		if len(out) > 0 {
			out = append(out, lm(lin("")))
		}
		out = append(out, lm(lin(g.titulo)))
		for i, a := range Atajos {
			if listadas[i] || !trabajaEn(a, g.lugares) {
				continue
			}
			listadas[i] = true
			out = append(out, m.filasAyuda(a, c)...)
		}
	}
	out = append(out, lm(lin("")), lm(lin("Mouse")))
	if m.c.Mouse {
		for _, l := range []string{
			"  clic              foco a la lista o al detalle; en una tarjeta, la elige",
			"  clic en la caja   elige la tarjeta de tu siguiente paso",
			"  rueda             mueve la seleccion de la lista; en el detalle y en la ayuda, desplaza",
			"  clic en un modal  enfoca un campo, elige una opcion o un boton",
			"  para seleccionar texto con el mouse, manten Option (macOS) o Shift",
		} {
			// a line that does not fit goes on from the column of Hace, 20
			cabeza, resto := "  ", strings.TrimPrefix(l, "  ")
			if celdas(l) > 20 && strings.TrimSpace(l[:20]) != "" && l[19] == ' ' {
				cabeza, resto = l[:20], l[20:]
			}
			for i, p := range envolverDos(resto, max(10, c-celdas(cabeza)), max(10, c-20)) {
				if i == 0 {
					out = append(out, lm(lin(cabeza+p)))
				} else {
					out = append(out, lm(lin(strings.Repeat(" ", 20)+p)))
				}
			}
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

// ---------------------------------------------------------------- Registro

// verRegistro is the Registro modal.
type verRegistro struct {
	scroll int
}

// abrirRegistro opens the Registro modal at its end.
func (m *Modelo) abrirRegistro() {
	m.cerrarModal()
	m.reg = &verRegistro{scroll: 1 << 30}
	m.modal = ModalRegistro
}

// lineasRegistro are the Registro modal's text lines: every entry's lines,
// the oldest on top.
func (m *Modelo) lineasRegistro() []lineaModal {
	if len(m.registro) == 0 {
		return []lineaModal{lm(lin("todavia no se ejecuto nada"))}
	}
	var out []lineaModal
	for _, a := range m.registro {
		for _, l := range a.lineas(m.s) {
			out = append(out, lm(l))
		}
	}
	return out
}

func (m *Modelo) enRegistro(t Tecla) {
	switch t {
	case "j", TeclaAbajo:
		m.desplazarRegistro(1)
	case "k", TeclaArriba:
		m.desplazarRegistro(-1)
	case TeclaPgAbajo:
		m.desplazarRegistro(max(1, m.c.Alto-4))
	case TeclaPgArriba:
		m.desplazarRegistro(-max(1, m.c.Alto-4))
	case TeclaEsc, "L":
		m.cerrarModal()
	}
}

func (m *Modelo) desplazarRegistro(d int) {
	ls := m.lineasRegistro()
	alto := min(len(ls)+2, m.c.Alto-2) - 2
	m.reg.scroll = max(0, min(m.reg.scroll, len(ls)-alto))
	m.reg.scroll = max(0, min(m.reg.scroll+d, len(ls)-alto))
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
		k, _ := m.pasoPedir(p)
		ls = m.lineasPedir(p, c, k)
	case ModalFirma, ModalGuardar:
		g.titulo = m.conf.accion.Label + " · " + m.conf.slug
		ls = m.lineasConfirmacionQueEntran(m.conf, c, b-2)
	case ModalSeguir:
		g.titulo = etiquetaSeguir + " · " + m.conf.titulo
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
	case ModalRegistro:
		g.titulo = "Registro"
		ls = m.lineasRegistro()
		alto := min(len(ls)+2, b) - 2
		m.reg.scroll = max(0, min(m.reg.scroll, len(ls)-alto))
		if len(ls) > alto {
			g.pos = fmt.Sprintf("%d de %d", m.reg.scroll+alto, len(ls))
			ls = ls[m.reg.scroll : m.reg.scroll+alto]
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

// geometriaLista places the open list over the Pedir modal: a box of its
// own, as wide as its longest row, its margin and its borders (at most
// Ancho-2), its left border on the modal's first text cell (further left
// when it would leave the screen); its top border on the selector's line,
// going up as much as it needs to fit in the modal's height; it scrolls
// only when it has more options than the modal has text lines.
func (m *Modelo) geometriaLista(g *modalEnPantalla) {
	p := m.pedir
	ops := m.opciones(p, p.lista.campo)
	fila := -1
	for i, l := range g.lineas {
		if l.campo == p.lista.campo {
			fila = i
		}
	}
	if fila < 0 || len(ops) == 0 {
		return
	}
	ySel := g.r.y + 1 + fila
	ancho := 0
	for i, o := range ops {
		ancho = max(ancho, celdas(m.filaLista(ops, i, o).plano()))
	}
	w := min(ancho+3, m.c.Ancho-2)
	x := g.r.x + 4
	if x+w > m.c.Ancho {
		x = m.c.Ancho - w
	}
	x = max(0, x)
	texto := g.r.h - 2
	h := min(len(ops), texto) + 2
	y := ySel
	if y+h > g.r.y+g.r.h {
		y = g.r.y + g.r.h - h
	}
	y = max(g.r.y, y)
	g.lista = rect{x: x, y: y, w: w, h: h}
	g.listaOps = ops
	dentro := h - 2
	if p.lista.idx < p.lista.scroll {
		p.lista.scroll = p.lista.idx
	}
	if p.lista.idx >= p.lista.scroll+dentro {
		p.lista.scroll = p.lista.idx - dentro + 1
	}
	p.lista.scroll = max(0, min(p.lista.scroll, len(ops)-dentro))
	g.listaDesde = p.lista.scroll
}

// filaLista is the row of the option o (the i-th of ops) in the open list:
// " " + its mark + " " + its name padded to the longest name + "  " + its
// line (a gray one, all gray, with its why as its line).
func (m *Modelo) filaLista(ops []opcion, i int, o opcion) linea {
	ancho := 0
	for _, x := range ops {
		ancho = max(ancho, celdas(x.nombre))
	}
	marca := " "
	if i == m.pedir.lista.idx {
		marca = m.s.sel
	}
	texto := o.linea
	if o.gris {
		texto = o.why
	}
	t := " " + marca + " " + rellenar(o.nombre, ancho)
	if texto != "" {
		t += "  " + texto
	}
	if o.gris {
		return linCon(t, gris)
	}
	return lin(strings.TrimRight(t, " "))
}

// dibujarModal draws the open modal (and its list) over the screen; with
// color the lines behind it go gray.
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
	for i := 0; i < r.h-2; i++ {
		y := r.y + 1 + i
		k := g.listaDesde + i
		lz.fijar(r.x, y, s.lado, 1, normal)
		var l linea
		if k < len(g.listaOps) {
			l = m.filaLista(g.listaOps, k, g.listaOps[k])
		}
		lz.poner(r.x+1, y, r.w-3, l, normal, true)
		lz.texto(r.x+r.w-2, y, 1, " ", normal)
		lz.fijar(r.x+r.w-1, y, s.lado, 1, normal)
	}
	pos := ""
	if len(g.listaOps) > r.h-2 {
		pos = strconv.Itoa(p.lista.idx+1) + " de " + strconv.Itoa(len(g.listaOps))
	}
	m.borde(lz, r.x, r.y+r.h-1, r.w, s.esqII, s.linea, s.esqID, pos, normal)
}

// envolverDos wraps s at its words: the first line in w1 cells, the others
// in w2 (a word longer than its line is cut hard).
func envolverDos(s string, w1, w2 int) []string {
	primera := envolverPalabrasEspacios(s, w1)
	if len(primera) <= 1 {
		return primera
	}
	resto := strings.TrimLeft(strings.TrimPrefix(s, primera[0]), " ")
	return append([]string{primera[0]}, envolverPalabrasEspacios(resto, w2)...)
}
