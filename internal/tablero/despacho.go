package tablero

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// Tecla takes one key at ahora. What a key does is the row of Atajos that
// has it in the current Lugar (Modelo.Lugar); a key with no row there does
// nothing: the screen and the orders stay the same. The package doc says
// what each row does in each Lugar ("The keys"); in short:
//
//   - enter on a card (in the list or in its detail) and n (the box's card)
//     open the modal of the card's step, whatever its kind; without a step
//     the message line says why for DuracionAviso (CA-704, CA-705). enter
//     on HECHO's line opens or closes HECHO.
//   - The keys of an action (a, t, w, r, R, s, A and I) never order Lanzar:
//     when the card has the action and it is enabled, they open its modal
//     (Pedir for a, t, w, r and R; Firma for A and I; Guardar for s) and
//     return no order; when the card does not have it, the message line
//     says for DuracionAviso "en <column_name> no hay <Hace>"; when it is
//     disabled, its why. P opens Seguir solo when it is available, and else
//     says why (see the package doc, "Seguir solo").
//   - u orders Lanzar{Rol: "", Args: {"usar", <slug>}} at once (no modal)
//     and adds its Registro entry ("Fijar para la terminal").
//   - Only enter on a modal's action button orders Lanzar (Pedir, Firma,
//     Guardar, Seguir solo), with the Args the modal's "Se va a ejecutar"
//     shows, adds the verb's Registro entry and closes the modal. Pedir, as
//     the v1's form, also puts "lanzando hoom <verb>" in the message line
//     for DuracionAviso (the verb's words, without -m and the flags:
//     "lanzando hoom pedir writer <slug>").
//   - e orders AbrirEditor (and adds the editor's Registro entry), ctrl-e
//     in the Pedir modal orders EditarPedido, q and ctrl-c order Salir.
//
// A key that opens a modal returns no order: Modal says which one opened.
func (m *Modelo) Tecla(t Tecla, ahora time.Time) []Orden {
	if !ahora.IsZero() {
		m.ahora = ahora
	}
	if m.pequena() {
		if t == "q" || t == TeclaCtrlC {
			return []Orden{Salir{}}
		}
		return nil
	}
	l := m.Lugar()
	// ]c and [c: two strokes
	if l == LugarVerTarjeta {
		if m.prefijo != "" {
			junta := m.prefijo + t
			m.prefijo = ""
			if junta == "]c" || junta == "[c" {
				t = junta
			}
		}
		if t == "]" || t == "[" {
			m.prefijo = t
			return nil
		}
	} else {
		m.prefijo = ""
	}
	a, ok := atajoEn(m.filas(l), t)
	if !ok {
		return nil
	}
	ords := m.hacer(l, a, t)
	m.acomodar()
	return ords
}

// filas are the rows of l that work now: AtajosDe(l), without ctrl-e in
// the reviewer's Pedir modal, which has no Pedido (CA-564).
func (m *Modelo) filas(l Lugar) []Atajo {
	out := AtajosDe(l)
	if m.modal != ModalPedir || m.pedir == nil || !m.pedir.revisor {
		return out
	}
	var sin []Atajo
	for _, a := range out {
		if !(len(a.Teclas) == 1 && a.Teclas[0] == TeclaCtrlE) {
			sin = append(sin, a)
		}
	}
	return sin
}

// atajoEn is the row of filas that t has: the one that names it or, for a
// printable key that no row names, the row of TeclaTexto.
func atajoEn(filas []Atajo, t Tecla) (Atajo, bool) {
	for _, a := range filas {
		for _, k := range a.Teclas {
			if k == t {
				return a, true
			}
		}
	}
	if !imprimible(t) {
		return Atajo{}, false
	}
	for _, a := range filas {
		for _, k := range a.Teclas {
			if k == TeclaTexto {
				return a, true
			}
		}
	}
	return Atajo{}, false
}

// imprimible: a key that is one printable rune (the space included).
func imprimible(t Tecla) bool {
	r, n := utf8.DecodeRuneInString(string(t))
	return n == len(t) && n > 0 && r != utf8.RuneError && unicode.IsPrint(r)
}

// AtajosDe are the rows of Atajos that work in l, in table order.
func AtajosDe(l Lugar) []Atajo {
	var out []Atajo
	for _, a := range Atajos {
		if contieneLugar(a.Lugares, l) {
			out = append(out, a)
		}
	}
	return out
}

func contieneLugar(ls []Lugar, l Lugar) bool {
	for _, x := range ls {
		if x == l {
			return true
		}
	}
	return false
}

// hacer runs the row a in the Lugar l for the key t.
func (m *Modelo) hacer(l Lugar, a Atajo, t Tecla) []Orden {
	if t == TeclaCtrlC || t == "q" && contieneLugar(LugaresPaneles, l) {
		return []Orden{Salir{}}
	}
	switch a.Accion {
	case "":
	case AccionPaso:
		if l == LugarTarjetas && !m.sel.nada && m.sel.hecho && len(m.filasLista()) > 0 {
			m.hechoAbierto = !m.hechoAbierto
			return nil
		}
		if c := m.tarjetaDeLugar(l); c != nil {
			m.abrirPaso(c)
		}
		return nil
	case AccionSiguiente:
		c, texto := m.tarjetaDeN()
		switch {
		case c != nil:
			m.abrirPaso(c)
		case texto != "":
			m.avisar(texto, normal, DuracionAviso)
		}
		return nil
	default:
		if c := m.tarjetaDeLugar(l); c != nil {
			return m.accion(c, a)
		}
		return nil
	}
	switch l {
	case LugarPedirSelector, LugarPedirPedido, LugarPedirTexto, LugarPedirLista, LugarPedirBotones:
		return m.enPedir(l, t)
	case LugarFirma, LugarGuardar, LugarSeguir:
		return m.enConfirmacion(t)
	case LugarAcciones:
		return m.enAcciones(t)
	case LugarAyuda:
		m.enAyuda(t)
		return nil
	case LugarRegistro:
		m.enRegistro(t)
		return nil
	case LugarBusqueda:
		m.enBusqueda(t)
		return nil
	}
	// the zones
	switch t {
	case TeclaTab, TeclaShiftTab:
		if m.zona == ZonaLista {
			m.ponerZona(ZonaDetalle)
		} else {
			m.ponerZona(ZonaLista)
		}
		return nil
	case "/":
		m.abrirBusqueda()
		return nil
	case "x":
		m.experto = !m.experto
		return nil
	case "?":
		m.abrirAyuda(l)
		return nil
	case "L":
		m.abrirRegistro()
		return nil
	case "H":
		if m.vista == VistaHallazgos {
			m.volverATarjetas()
		} else {
			m.vista = VistaHallazgos
			m.ponerZona(ZonaLista)
		}
		return nil
	case "v":
		m.ponerZona(ZonaDetalle)
		m.verProyecto = true
		return nil
	case TeclaEspacio:
		if c := m.tarjetaDeLugar(l); c != nil {
			m.abrirAcciones(c)
		}
		return nil
	case "e":
		return m.abrirEnEditor(l)
	}
	if contieneLugar(LugaresVer, l) {
		m.enDetalle(t)
		return nil
	}
	m.enLista(l, t)
	return nil
}

// tarjetaDeLugar is the card the card's keys act on in l.
func (m *Modelo) tarjetaDeLugar(l Lugar) *boardcmd.Card {
	switch l {
	case LugarTarjetas, LugarVerTarjeta:
		return m.tarjeta(m.elegidaDeLista())
	case LugarAcciones:
		if m.acciones != nil {
			return m.tarjeta(m.acciones.slug)
		}
	}
	return nil
}

// enLista is a key of the list (of cards or of findings).
func (m *Modelo) enLista(l Lugar, t Tecla) {
	switch t {
	case "j", TeclaAbajo:
		m.moverLista(1, false)
	case "k", TeclaArriba:
		m.moverLista(-1, false)
	case "g":
		m.moverLista(-1, true)
	case "G":
		m.moverLista(1, true)
	case TeclaEnter, "l", TeclaDer:
		m.ponerZona(ZonaDetalle)
	case TeclaEsc:
		if l == LugarHallazgos {
			m.volverATarjetas()
		}
	}
}

// enDetalle is a key of the detail.
func (m *Modelo) enDetalle(t Tecla) {
	switch t {
	case "j", TeclaAbajo:
		m.desplazar0(1)
	case "k", TeclaArriba:
		m.desplazar0(-1)
	case TeclaPgAbajo:
		m.desplazar0(max(1, m.alto0()))
	case TeclaPgArriba:
		m.desplazar0(-max(1, m.alto0()))
	case "g":
		m.desplazar0(-1 << 30)
	case "G":
		m.desplazar0(1 << 30)
	case TeclaEsc, TeclaIzq, "h":
		m.ponerZona(ZonaLista)
	case "]c", "[c":
		m.saltarCriterio(t == "]c")
	}
}

// desplazar0 scrolls the detail d lines (clamped); a card with running
// follows its end when the view is at it.
func (m *Modelo) desplazar0(d int) {
	docu, hi := m.doc0()
	maximo := max(0, len(docu)-hi)
	s := m.scroll0 + d
	if d == 1<<30 || s > maximo {
		s = maximo
	}
	if s < 0 {
		s = 0
	}
	m.scroll0 = s
	m.sigue0 = m.sigueElFinal(s, maximo)
}

// sigueElFinal says whether the detail follows its end at the scroll s of
// maximo: a card with running when it is at its end; another card's
// document only when it is longer than the view and it is at its end (its
// Narración in view). The project and a finding never follow.
func (m *Modelo) sigueElFinal(s, maximo int) bool {
	if !strings.HasPrefix(m.clave0, "t:") {
		return false
	}
	if c := m.tarjeta(strings.TrimPrefix(m.clave0, "t:")); c != nil && c.Running != nil {
		return s >= maximo
	}
	return maximo > 0 && s >= maximo
}

// saltarCriterio makes the next (or previous) criterion without a test the
// first line in view.
func (m *Modelo) saltarCriterio(adelante bool) {
	docu, hi := m.doc0()
	actual := m.scroll0
	var sin []int
	for i, l := range docu {
		// the first line of each ✗ criterion
		if l.sin && (i == 0 || docu[i-1].crit != l.crit) {
			sin = append(sin, i)
		}
	}
	maximo := max(0, len(docu)-hi)
	if adelante {
		for _, i := range sin {
			if i > actual {
				m.scroll0 = min(i, maximo)
				return
			}
		}
	} else {
		for j := len(sin) - 1; j >= 0; j-- {
			if sin[j] < actual {
				m.scroll0 = min(sin[j], maximo)
				return
			}
		}
	}
	m.avisar("no hay mas criterios sin prueba", normal, DuracionAviso)
}

// abrirEnEditor is e.
func (m *Modelo) abrirEnEditor(l Lugar) []Orden {
	switch l {
	case LugarVerTarjeta:
		c := m.tarjeta(m.elegidaDeLista())
		det := m.detalle
		if c == nil || det == nil || det.Slug != c.Slug || det.Err != nil {
			return nil
		}
		if !tieneSpec(det) {
			m.avisar("sin spec: "+rutaSpec(c, det), normal, DuracionAviso)
			return nil
		}
		return m.abrirArchivo(rutaSpec(c, det), m.lineaCriterioEnVista(det))
	case LugarHallazgos, LugarVerHallazgo:
		h, ok := m.hallazgoElegido()
		if !ok {
			return nil
		}
		archivo, n := m.rutaHallazgo(h)
		if archivo == "" {
			m.avisar("el hallazgo no nombra un archivo", normal, DuracionAviso)
			return nil
		}
		return m.abrirArchivo(archivo, n)
	}
	return nil
}

// lineaCriterioEnVista is the spec's line of the first criterion with a
// line in the detail's view, or 1.
func (m *Modelo) lineaCriterioEnVista(det *Detalle) int {
	docu, hi := m.doc0()
	for i := m.scroll0; i < len(docu) && i < m.scroll0+hi; i++ {
		id := docu[i].crit
		if id == "" {
			continue
		}
		for n, l := range strings.Split(det.Detalle.Spec.Markdown, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "- "+id+":") {
				return n + 1
			}
		}
		return 1
	}
	return 1
}

// abrirArchivo is the order to open archivo (a path of the project, with /)
// at linea, or a message when the path leaves the project.
func (m *Modelo) abrirArchivo(archivo string, linea int) []Orden {
	if fueraPorLaForma(archivo) {
		m.avisar("el archivo "+strconv.Quote(archivo)+" esta fuera del proyecto", rojo, DuracionAviso)
		return nil
	}
	m.registrarEditor(archivo, linea)
	return []Orden{AbrirEditor{Archivo: archivo, Linea: linea}}
}

// enBusqueda is a key of the search.
func (m *Modelo) enBusqueda(t Tecla) {
	switch t {
	case TeclaAbajo:
		m.moverLista(1, false)
	case TeclaArriba:
		m.moverLista(-1, false)
	case TeclaBorrar:
		if len(m.filtro) > 0 {
			m.filtro = m.filtro[:len(m.filtro)-1]
		}
		m.trasFiltrar()
	case TeclaEsc, TeclaEnter:
		m.cerrarBusqueda()
	default:
		m.filtro = append(m.filtro, []rune(string(t))...)
		m.trasFiltrar()
	}
}

// trasFiltrar keeps the chosen card while it stays, and else goes to the
// first row that does.
func (m *Modelo) trasFiltrar() {
	if m.sel.hecho {
		m.acomodarLista()
		return
	}
	if c := m.tarjeta(m.sel.slug); c != nil && m.pasaFiltro(c) {
		m.acomodarLista()
		return
	}
	m.elegirL(0)
}

// ---------------------------------------------------------------- the actions

// accion runs the row a (one with Accion) on the card c.
func (m *Modelo) accion(c *boardcmd.Card, a Atajo) []Orden {
	switch a.Accion {
	case AccionUsar:
		args := verboDe(a, c.Slug)
		la := m.lanzar(c.Slug, "", args)
		m.registrarVerbo(la, "Fijar para la terminal", lineaVerbo(args, m.c.Windows))
		if m.modal == ModalAcciones {
			m.cerrarModal()
		}
		return []Orden{la.o}
	case AccionSeguir:
		if why := m.motivoSinSeguir(c); why != "" {
			m.avisar(why, normal, DuracionAviso)
			return nil
		}
		m.abrirSeguir(c)
		return nil
	}
	act, ok := c.Action(a.Accion)
	if !ok {
		m.avisar(fmt.Sprintf("en %s no hay %s", c.ColumnName, a.Hace), normal, DuracionAviso)
		return nil
	}
	if !act.Enabled {
		m.avisar(act.Why, normal, DuracionAviso)
		return nil
	}
	m.abrirModalDe(c, act, a)
	return nil
}

// abrirPaso opens the modal of the card's step, or says why it has none.
func (m *Modelo) abrirPaso(c *boardcmd.Card) {
	p := m.pasoDe(c)
	switch {
	case p.seguir:
		m.abrirSeguir(c)
	case p.act != nil:
		m.abrirModalDe(c, *p.act, p.fila)
	default:
		m.avisar(p.texto, normal, DuracionAviso)
	}
}

// abrirModalDe opens the modal of the action act of the card c, whose key's
// row is fila.
func (m *Modelo) abrirModalDe(c *boardcmd.Card, act boardcmd.Action, fila Atajo) {
	m.cerrarModal()
	verbo := verboDe(fila, c.Slug)
	switch act.ID {
	case boardcmd.ActAprobar, boardcmd.ActIntegrar:
		m.abrirConfirmacion(c, act, true, verbo)
	case boardcmd.ActGuardar:
		m.abrirConfirmacion(c, act, false, verbo)
	default:
		m.abrirPedir(c, act, verbo)
	}
}

// cerrarModal closes the open modal.
func (m *Modelo) cerrarModal() {
	m.modal = SinModal
	m.pedir, m.conf, m.acciones, m.ayuda, m.reg = nil, nil, nil, nil, nil
}

// slugDelModal is the card of the open modal ("" without one).
func (m *Modelo) slugDelModal() string {
	switch {
	case m.pedir != nil:
		return m.pedir.slug
	case m.conf != nil:
		return m.conf.slug
	case m.acciones != nil:
		return m.acciones.slug
	}
	return ""
}

// verboDe is the capa-1 verb that row a runs on slug: its Verbo without
// "hoom" and with <slug> as slug ("hoom pedir writer <slug>" is {"pedir",
// "writer", slug}). What is shown and what runs are one text.
func verboDe(a Atajo, slug string) []string {
	f := strings.Fields(a.Verbo)
	if len(f) < 2 || f[0] != "hoom" {
		return nil
	}
	out := make([]string, 0, len(f)-1)
	for _, w := range f[1:] {
		if w == "<slug>" {
			w = slug
		}
		out = append(out, w)
	}
	return out
}

// lanzar is the order of a launch, remembered until it ends.
func (m *Modelo) lanzar(slug, rol string, args []string) *lanzado {
	m.sigID++
	o := Lanzar{ID: m.sigID, Slug: slug, Rol: rol, Args: args}
	antes := ""
	if c := m.tarjeta(slug); c != nil && c.Running != nil {
		antes = idDe(c.Running)
	}
	la := &lanzado{o: o, antes: antes}
	m.lanzados[o.ID] = la
	return la
}

// palabrasVerbo are the verb's words of a launch's Args: up to the first
// flag.
func palabrasVerbo(args []string) string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			break
		}
		out = append(out, a)
	}
	return strings.Join(out, " ")
}

// marcadores are the markers tarjetacmd.Citar writes on Windows for a value
// it cannot quote.
var marcadores = map[string]string{
	"--provider": "<provider>", "--model": "<modelo>", "--effort": "<esfuerzo>",
	"--budget-usd": "<presupuesto>", "--timeout": "<tiempo>",
}

// palabrasComando are the command's words: "hoom" and the Args, each value
// quoted as the capa-1 verbs do it (the pedido cut to 60 characters).
func palabrasComando(args []string, windows bool) []string {
	parts := []string{"hoom"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-m" && i+1 < len(args):
			parts = append(parts, a, tarjetacmd.CitarPedido(args[i+1], windows))
			i++
		case strings.HasPrefix(a, "--") && i+1 < len(args) && marcadores[a] != "":
			parts = append(parts, a, tarjetacmd.Citar(args[i+1], marcadores[a], windows))
			i++
		default:
			parts = append(parts, a)
		}
	}
	return parts
}

// lineaVerbo is the command on one line.
func lineaVerbo(args []string, windows bool) string {
	return strings.Join(palabrasComando(args, windows), " ")
}
