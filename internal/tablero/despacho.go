package tablero

import (
	"fmt"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// hacer runs the row a of the view v for the key t.
func (m *Modelo) hacer(v Vista, a Atajo, t Tecla) []Orden {
	// the keys that mean the same wherever they work
	switch {
	case t == "q" && v != VistaBusqueda && v != VistaFormulario, t == TeclaCtrlC:
		return []Orden{Salir{}}
	case t == "x" && esTarjetaVista(v):
		m.experto = !m.experto
		return nil
	case t == "?" && v != VistaAyuda && esTarjetaVista(v):
		m.ayudaDe = v
		m.vista = VistaAyuda
		return nil
	case a.Accion != "":
		if c := m.tarjetaDe(v); c != nil {
			return m.accion(c, a)
		}
		return nil
	case t == TeclaEspacio && v != VistaBusqueda && v != VistaFormulario:
		if c := m.tarjetaDe(v); c != nil {
			m.abrirMenu(c, v)
		}
		return nil
	}
	switch v {
	case VistaLista:
		return m.enLista(t)
	case VistaBusqueda:
		return m.enBusqueda(t)
	case VistaResumen, VistaSpec, VistaSobre, VistaHallazgos, VistaHistoria:
		return m.enPestana(v, t)
	case VistaMenu:
		return m.enMenu(t)
	case VistaFormulario:
		return m.enFormulario(t)
	case VistaConfirmacion:
		return m.enConfirmacion(t)
	case VistaAyuda:
		m.vista = m.ayudaDe
		return nil
	}
	return nil
}

func esTarjetaVista(v Vista) bool { return contieneVista(tarjetaVistas, v) }

func esPestana(v Vista) bool { return contieneVista(pestanaVistas, v) }

// tarjetaDe is the card the action keys act on in v: the selected one in
// the list, the open one in its tabs, the menu's in the menu.
func (m *Modelo) tarjetaDe(v Vista) *boardcmd.Card {
	switch {
	case v == VistaLista:
		if m.selHecho || m.selSlug == "" {
			return nil
		}
		return m.tarjeta(m.selSlug)
	case esPestana(v):
		return m.tarjeta(m.abierta)
	case v == VistaMenu && m.menu != nil:
		return m.tarjeta(m.menu.slug)
	}
	return nil
}

// ---------------------------------------------------------------- the list

func (m *Modelo) enLista(t Tecla) []Orden {
	switch t {
	case "j", TeclaAbajo:
		m.mover(1)
	case "k", TeclaArriba:
		m.mover(-1)
	case "g":
		m.elegir(0)
	case "G":
		m.elegir(len(m.elegibles()) - 1)
	case TeclaEnter, "l", TeclaDer:
		m.entrar()
	case "/":
		m.vista = VistaBusqueda
		m.filtro = nil
	}
	return nil
}

func (m *Modelo) enBusqueda(t Tecla) []Orden {
	switch t {
	case TeclaAbajo:
		m.mover(1)
	case TeclaArriba:
		m.mover(-1)
	case TeclaBorrar:
		if len(m.filtro) > 0 {
			m.filtro = m.filtro[:len(m.filtro)-1]
		}
		m.alFiltrar()
	case TeclaEsc:
		m.cerrarBusqueda()
	case TeclaEnter:
		abrir := !m.selHecho && m.selSlug != "" && !m.sinSeleccion
		m.cerrarBusqueda()
		if abrir {
			m.abrir(m.selSlug)
		}
	default:
		m.filtro = append(m.filtro, []rune(string(t))...)
		m.alFiltrar()
	}
	return nil
}

func (m *Modelo) cerrarBusqueda() {
	m.vista = VistaLista
	m.filtro = nil
	m.acomodar()
}

// mover moves the selection d lines over the ones that can be chosen.
func (m *Modelo) mover(d int) {
	els := m.elegibles()
	if len(els) == 0 {
		return
	}
	m.elegir(m.selIdx + d)
}

// elegir puts the selection on the i-th line that can be chosen (clamped).
func (m *Modelo) elegir(i int) {
	els := m.elegibles()
	if len(els) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(els) {
		i = len(els) - 1
	}
	m.selIdx = i
	m.selSlug, m.selHecho = els[i].slug, els[i].hecho
	m.sinSeleccion = false
}

// entrar is enter on the list: it opens the card, or opens and closes
// Hecho.
func (m *Modelo) entrar() {
	if m.sinSeleccion {
		return
	}
	if m.selHecho {
		m.hechoAbierto = !m.hechoAbierto
		return
	}
	if m.selSlug != "" {
		m.abrir(m.selSlug)
	}
}

// abrir opens a card on its Resumen.
func (m *Modelo) abrir(slug string) {
	if m.tarjeta(slug) == nil {
		return
	}
	if m.abierta != slug {
		m.detalle = nil
	}
	m.abierta = slug
	m.pestana = VistaResumen
	m.vista = VistaResumen
	m.scrolls = map[Vista]int{}
	m.hallazgo = 0
	m.alFinal = true
}

// volver closes the open card: back to the list, the selection on it.
func (m *Modelo) volver() {
	if m.abierta != "" {
		m.selSlug, m.selHecho, m.sinSeleccion = m.abierta, false, false
	}
	m.abierta = ""
	m.detalle = nil
	m.vista = VistaLista
}

// cerrarTodo goes back to the list from wherever it is.
func (m *Modelo) cerrarTodo() {
	m.menu, m.form, m.conf = nil, nil, nil
	m.abierta = ""
	m.detalle = nil
	m.filtro = nil
	m.vista = VistaLista
}

// slugEnUso is the card the screen is about beyond the list: the open one,
// or the one of the menu, the form or the confirmation ("" in the list).
func (m *Modelo) slugEnUso() string {
	switch {
	case m.form != nil && (m.vista == VistaFormulario || m.vista == VistaAyuda):
		return m.form.slug
	case m.conf != nil && m.vista == VistaConfirmacion:
		return m.conf.slug
	case m.menu != nil && (m.vista == VistaMenu || m.ayudaDe == VistaMenu && m.vista == VistaAyuda):
		return m.menu.slug
	}
	return m.abierta
}

// ---------------------------------------------------------------- the tabs

func (m *Modelo) enPestana(v Vista, t Tecla) []Orden {
	switch t {
	case "1", "2", "3", "4", "5":
		m.elegirPestana(Pestanas[int(t[0]-'1')])
	case TeclaTab:
		for i, p := range Pestanas {
			if p == v {
				m.elegirPestana(Pestanas[(i+1)%len(Pestanas)])
				break
			}
		}
	case TeclaEsc, "h", TeclaIzq:
		m.volver()
	case "j", TeclaAbajo:
		m.desplazar(v, 1)
	case "k", TeclaArriba:
		m.desplazar(v, -1)
	case "g":
		if v == VistaHallazgos {
			m.hallazgo = 0
		} else {
			m.scrolls[v] = 0
			m.alFinal = v != VistaSobre
		}
	case "G":
		if v == VistaHallazgos {
			m.hallazgo = len(m.hallazgos()) - 1
		} else {
			m.scrolls[v] = m.maxScroll(v)
			m.alFinal = true
		}
	case "]c", "[c":
		m.saltarCriterio(t == "]c")
	case "o":
		return m.abrirSpec()
	case TeclaEnter:
		return m.abrirHallazgo()
	}
	return nil
}

func (m *Modelo) elegirPestana(p Vista) {
	m.pestana = p
	m.vista = p
	if p == VistaSobre {
		m.alFinal = true
		m.scrolls[p] = m.maxScroll(p)
	}
}

// alto is how many lines of a tab's content are in view.
func (m *Modelo) alto() int {
	if h := m.c.Alto - 5; h > 0 {
		return h
	}
	return 1
}

func (m *Modelo) maxScroll(v Vista) int {
	if n := len(m.contenido(v)) - m.alto(); n > 0 {
		return n
	}
	return 0
}

func (m *Modelo) desplazar(v Vista, d int) {
	if v == VistaHallazgos {
		m.hallazgo += d
		return
	}
	s := m.scrolls[v]
	switch {
	case d > 0 && s < m.maxScroll(v):
		s++
	case d < 0 && s > 0:
		s--
	}
	m.scrolls[v] = s
	if v == VistaSobre {
		m.alFinal = s >= m.maxScroll(v)
	}
}

// saltarCriterio makes the next (or previous) criterion without a test the
// first line in view.
func (m *Modelo) saltarCriterio(adelante bool) {
	actual := m.scrolls[VistaSpec]
	sin := m.sinPrueba()
	if adelante {
		for _, i := range sin {
			if i > actual {
				m.scrolls[VistaSpec] = i
				return
			}
		}
	} else {
		for j := len(sin) - 1; j >= 0; j-- {
			if sin[j] < actual {
				m.scrolls[VistaSpec] = sin[j]
				return
			}
		}
	}
	m.avisar("no hay mas criterios sin prueba", normal, DuracionAviso)
}

func (m *Modelo) abrirSpec() []Orden {
	if m.detalle == nil || !tieneSpec(m.detalle) {
		return nil
	}
	return []Orden{AbrirEditor{Archivo: m.rutaSpec(), Linea: m.scrolls[VistaSpec] + 1}}
}

func (m *Modelo) abrirHallazgo() []Orden {
	hs := m.hallazgos()
	if len(hs) == 0 {
		return nil
	}
	i := m.hallazgo
	if i < 0 || i >= len(hs) {
		return nil
	}
	archivo, n := archivoYLinea(hs[i])
	if archivo == "" {
		m.avisar("el hallazgo no nombra un archivo", normal, DuracionAviso)
		return nil
	}
	dir := m.dirEvidencia()
	if dir != "" && dir != "." {
		archivo = dir + "/" + archivo
	}
	return []Orden{AbrirEditor{Archivo: archivo, Linea: n}}
}

// ---------------------------------------------------------------- the menu

// menu is the action menu of a card.
type menu struct {
	slug  string
	desde Vista // the list or a tab
	sel   int   // index into the card's shown actions
}

func (m *Modelo) abrirMenu(c *boardcmd.Card, desde Vista) {
	m.menu = &menu{slug: c.Slug, desde: desde, sel: -1}
	m.vista = VistaMenu
	m.acomodarMenu()
}

// accionesMenu are the card's actions the menu shows: the expert ones only
// in expert mode.
func (m *Modelo) accionesMenu(c *boardcmd.Card) []boardcmd.Action {
	var out []boardcmd.Action
	for _, a := range c.Actions {
		if a.Expert && !m.experto {
			continue
		}
		out = append(out, a)
	}
	return out
}

// elegibleMenu: an enabled action with a capa-1 verb.
func elegibleMenu(a boardcmd.Action) bool {
	_, ok := filaDeAccion(a.ID)
	return a.Enabled && ok
}

// filaDeAccion is the row of Atajos that runs the action id.
func filaDeAccion(id string) (Atajo, bool) {
	for _, a := range Atajos {
		if a.Accion == id && a.Accion != "" {
			return a, true
		}
	}
	return Atajo{}, false
}

func (m *Modelo) acomodarMenu() {
	if m.menu == nil {
		return
	}
	c := m.tarjeta(m.menu.slug)
	if c == nil {
		return
	}
	as := m.accionesMenu(c)
	if m.menu.sel >= 0 && m.menu.sel < len(as) && elegibleMenu(as[m.menu.sel]) {
		return
	}
	m.menu.sel = -1
	for i, a := range as {
		if elegibleMenu(a) {
			m.menu.sel = i
			return
		}
	}
}

func (m *Modelo) enMenu(t Tecla) []Orden {
	c := m.tarjeta(m.menu.slug)
	if c == nil {
		return nil
	}
	as := m.accionesMenu(c)
	switch t {
	case "j", TeclaAbajo, "k", TeclaArriba:
		d := 1
		if t == "k" || t == TeclaArriba {
			d = -1
		}
		for i := m.menu.sel + d; i >= 0 && i < len(as); i += d {
			if elegibleMenu(as[i]) {
				m.menu.sel = i
				break
			}
		}
	case TeclaEsc:
		m.cerrarMenu()
	case TeclaEnter:
		if m.menu.sel < 0 || m.menu.sel >= len(as) {
			return nil
		}
		fila, ok := filaDeAccion(as[m.menu.sel].ID)
		if !ok {
			return nil
		}
		return m.accion(c, fila)
	}
	return nil
}

// cerrarMenu goes back to where the menu was opened from.
func (m *Modelo) cerrarMenu() {
	if m.menu != nil {
		m.vista = m.menu.desde
	}
	m.menu = nil
}

// origen is the view a form or a confirmation goes back to: the one under
// the menu when it came from the menu.
func (m *Modelo) origen() Vista {
	if m.vista == VistaMenu && m.menu != nil {
		return m.menu.desde
	}
	return m.vista
}

// ---------------------------------------------------------------- the actions

// accion runs the row a (one with Accion) on the card c.
func (m *Modelo) accion(c *boardcmd.Card, a Atajo) []Orden {
	switch a.Accion {
	case AccionUsar:
		o := m.lanzar(c.Slug, "", []string{"usar", c.Slug})
		m.dejarMenu()
		return []Orden{o}
	case AccionSiguiente:
		return m.siguiente(c)
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
	switch act.ID {
	case boardcmd.ActAprobar, boardcmd.ActIntegrar:
		m.abrirConfirmacion(c, act, true)
	case boardcmd.ActGuardar:
		m.abrirConfirmacion(c, act, false)
	case boardcmd.ActReanudar:
		m.abrirFormulario(c, act, []string{"reanudar", c.Slug})
	default:
		m.abrirFormulario(c, act, []string{"pedir", act.Role, c.Slug})
	}
	return nil
}

// siguiente is n: what hoom siguiente does.
func (m *Modelo) siguiente(c *boardcmd.Card) []Orden {
	switch {
	case c.Column == boardcmd.ColHecho:
		m.avisar("la tarjeta ya esta en Hecho", normal, DuracionAviso)
	case c.Running != nil:
		m.avisar(fmt.Sprintf("el %s esta trabajando: espera a que termine", rolDe(c.Running.Role)), normal, DuracionAviso)
	case c.Interrupted != nil:
		m.avisar("el trabajo se corto: R reanuda o pide de nuevo", normal, DuracionAviso)
	case c.Column == boardcmd.ColTuAprobacion:
		m.avisar("te toca: A aprobar", normal, DuracionAviso)
	case c.Column == boardcmd.ColTuAceptacion:
		m.avisar("te toca: I integrar", normal, DuracionAviso)
	case len(c.Actions) == 0:
		m.avisar(fmt.Sprintf("en %s no hay nada que hacer ahora: %s", c.ColumnName, c.Plain), normal, DuracionAviso)
	default:
		a := c.Actions[0]
		switch {
		case boardcmd.IsRoleAction(a.ID) && !a.Enabled:
			m.avisar(a.Why, normal, DuracionAviso)
		case boardcmd.IsRoleAction(a.ID):
			m.abrirFormulario(c, a, []string{"siguiente", c.Slug})
		case a.ID == boardcmd.ActGuardar:
			m.avisar("te toca: s guardar", normal, DuracionAviso)
		default:
			m.avisar(fmt.Sprintf("en %s no hay nada que hacer ahora: %s", c.ColumnName, c.Plain), normal, DuracionAviso)
		}
	}
	return nil
}

// dejarMenu closes the menu after it ran an action that opens nothing.
func (m *Modelo) dejarMenu() {
	if m.vista == VistaMenu {
		m.cerrarMenu()
	}
}

// lanzar is the order of a launch, remembered until it ends.
func (m *Modelo) lanzar(slug, rol string, args []string) Lanzar {
	m.sigID++
	o := Lanzar{ID: m.sigID, Slug: slug, Rol: rol, Args: args}
	antes := ""
	if c := m.tarjeta(slug); c != nil && c.Running != nil {
		antes = idDe(c.Running)
	}
	m.lanzados[o.ID] = &lanzado{o: o, antes: antes}
	return o
}

// ---------------------------------------------------------------- the confirmations

// confirmacion is the box of aprobar, integrar or guardar.
type confirmacion struct {
	slug   string
	accion boardcmd.Action
	desde  Vista
	firma  bool // aprobar or integrar; false: guardar
	firmar bool // Firmar is the chosen button
	texto  string
	paths  []string
}

func (m *Modelo) abrirConfirmacion(c *boardcmd.Card, a boardcmd.Action, firma bool) {
	cf := &confirmacion{slug: c.Slug, accion: a, desde: m.origen(), firma: firma}
	if firma {
		cf.texto = tarjetacmd.TextoFirma(*c, a)
	} else {
		cf.paths = a.Paths
		if len(cf.paths) == 0 {
			cf.paths = c.Unsynced
		}
	}
	m.menu = nil
	m.conf = cf
	m.vista = VistaConfirmacion
}

func (m *Modelo) enConfirmacion(t Tecla) []Orden {
	cf := m.conf
	switch t {
	case TeclaIzq, TeclaDer, TeclaTab:
		cf.firmar = !cf.firmar
	case TeclaEsc:
		m.cerrarConfirmacion()
	case TeclaEnter:
		if cf.firma && !cf.firmar {
			m.cerrarConfirmacion()
			return nil
		}
		args := []string{"guardar", cf.slug}
		if cf.firma {
			verbo := "aprobar"
			if cf.accion.ID == boardcmd.ActIntegrar {
				verbo = "integrar"
			}
			args = []string{verbo, cf.slug, "--si"}
		}
		o := m.lanzar(cf.slug, "", args)
		m.cerrarConfirmacion()
		return []Orden{o}
	}
	return nil
}

func (m *Modelo) cerrarConfirmacion() {
	if m.conf != nil {
		m.vista = m.conf.desde
	}
	m.conf = nil
}

// ---------------------------------------------------------------- the bar

// avisar puts a message in the bar for d.
func (m *Modelo) avisar(texto string, e estilo, d time.Duration) {
	m.msg, m.msgEstilo, m.msgHasta = texto, e, m.ahora.Add(d)
}

// mensaje is the bar's message now: the last one while it lasts, else the
// read error.
func (m *Modelo) mensaje() (string, estilo) {
	if m.msg != "" && m.ahora.Before(m.msgHasta) {
		return m.msg, m.msgEstilo
	}
	if m.errLectura != "" {
		return m.errLectura, rojo
	}
	return "", normal
}

// ---------------------------------------------------------------- the board

// todas are the board's cards, in its order.
func (m *Modelo) todas() []*boardcmd.Card {
	var out []*boardcmd.Card
	for i := range m.tablero.Columns {
		col := &m.tablero.Columns[i]
		for j := range col.Cards {
			out = append(out, &col.Cards[j])
		}
	}
	return out
}

// tarjeta is the card slug of the board (nil when it is not there).
func (m *Modelo) tarjeta(slug string) *boardcmd.Card {
	if slug == "" {
		return nil
	}
	for _, c := range m.todas() {
		if c.Slug == slug {
			return c
		}
	}
	return nil
}

// rolDe is a running's role, "agente" when it has none.
func rolDe(r string) string {
	if r == "" {
		return "agente"
	}
	return r
}

// idDe is what tells one running from another: its envelope, else its run.
func idDe(r *boardcmd.Running) string {
	if r.EnvelopeID != "" {
		return r.EnvelopeID
	}
	return r.RunID
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

// ---------------------------------------------------------------- keeping it in order

// acomodar keeps the selection, the scroll and the cursors where the
// contract says after any change.
func (m *Modelo) acomodar() {
	m.acomodarSeleccion()
	m.acomodarScrollLista()
	m.acomodarMenu()
	if m.abierta != "" {
		n := len(m.hallazgos())
		if m.hallazgo >= n {
			m.hallazgo = n - 1
		}
		if m.hallazgo < 0 {
			m.hallazgo = 0
		}
		for _, p := range Pestanas {
			s, max := m.scrolls[p], len(m.contenido(p))-1
			if p == VistaSobre && m.alFinal {
				s = m.maxScroll(p)
			}
			if p == VistaHallazgos {
				h := m.alto()
				if m.hallazgo < s {
					s = m.hallazgo
				}
				if m.hallazgo >= s+h {
					s = m.hallazgo - h + 1
				}
			}
			if s > max {
				s = max
			}
			if s < 0 {
				s = 0
			}
			m.scrolls[p] = s
		}
	}
}

// acomodarSeleccion keeps the selection on its card or line; when it is no
// longer there, it takes the line with its index (the last one when there
// are fewer). At the start it is the first card line.
func (m *Modelo) acomodarSeleccion() {
	els := m.elegibles()
	if len(els) == 0 {
		m.selIdx = 0
		return
	}
	if m.sinSeleccion {
		for i, e := range els {
			if !e.hecho {
				m.elegir(i)
				return
			}
		}
		m.elegir(0)
		return
	}
	for i, e := range els {
		if e.hecho == m.selHecho && (e.hecho || e.slug == m.selSlug) {
			m.selIdx = i
			return
		}
	}
	m.elegir(m.selIdx)
}

// alFiltrar keeps the selection when its card stays, else it goes to the
// first card line.
func (m *Modelo) alFiltrar() {
	els := m.elegibles()
	for i, e := range els {
		if !m.sinSeleccion && e.hecho == m.selHecho && (e.hecho || e.slug == m.selSlug) {
			m.selIdx = i
			return
		}
	}
	for i, e := range els {
		if !e.hecho {
			m.elegir(i)
			return
		}
	}
	m.elegir(0)
}

// acomodarScrollLista scrolls the list the least needed to show the
// selected line and its second line.
func (m *Modelo) acomodarScrollLista() {
	lineas, pos := m.cuerpoLista()
	h := m.c.Alto - 4
	if h <= 0 {
		return
	}
	if m.selIdx >= 0 && m.selIdx < len(pos) {
		ini, fin := pos[m.selIdx].ini, pos[m.selIdx].fin
		if ini < m.scrollLista {
			m.scrollLista = ini
		}
		if fin >= m.scrollLista+h {
			m.scrollLista = fin - h + 1
		}
	}
	if max := len(lineas) - h; m.scrollLista > max {
		m.scrollLista = max
	}
	if m.scrollLista < 0 {
		m.scrollLista = 0
	}
}
