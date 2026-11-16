package tablero

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
)

// How long the message line keeps a message.
const (
	DuracionAviso = 5 * time.Second // a why, a key with nothing to do, a card that no longer exists
	DuracionFin   = 8 * time.Second // "<rol> termino en <slug>: <plain>"
)

// The smallest screen the TUI draws.
const (
	anchoMinimo = 60
	altoMinimo  = 15
)

const msgAgrandar = "agranda la terminal (minimo 60x15)"

// Modelo is the TUI without a terminal: keys, mouse events and reads in, a
// screen and orders out (the package doc is the contract of its screen and
// of what every key does). It is not safe for concurrent use: Run calls it
// from one goroutine.
type Modelo struct {
	c     Config
	s     signos
	ahora time.Time

	// the board
	leido      bool
	tablero    boardcmd.Board
	fijada     string
	errTablero string
	experto    bool

	// the project and its findings: the last reads that worked
	proyecto    *Proyecto
	errProyecto string
	hallazgos   []finding.Item // high, medium, low, the rest
	hallLeidos  bool
	errHall     string

	detalle *Detalle // of Elegida
	archivo *Archivo // of ArchivoElegido

	zona        Zona
	vista       Vista
	verProyecto bool // the detail shows the project (v) while it has the focus

	// the list of cards: the selection is a card (slug) or HECHO's line
	// (hecho); idx is its place among the rows
	sel          elegido
	hechoAbierto bool
	scrollL      int
	buscando     bool
	filtro       []rune

	// the Hallazgos view: the chosen finding by id, and its index
	sel4    string
	idx4    int
	scroll4 int

	// the detail
	scroll0 int
	clave0  string // what the detail showed when scroll0 was set
	sigue0  bool   // the detail follows its end
	prefijo Tecla  // "]" or "[" waiting for its second stroke

	// the modal
	modal    Modal
	pedir    *pedir
	conf     *confirmacion // Firma, Guardar and Seguir solo
	acciones *acciones
	ayuda    *ayuda
	reg      *verRegistro

	barra barra

	registro []*asiento

	sigID    int
	lanzados map[int]*lanzado
	vistos   map[string]visto
	paradas  map[string]parada // the pilots' stops that stand, by card
}

// elegido is the list's selection.
type elegido struct {
	slug  string
	hecho bool
	idx   int
	nada  bool // nothing chosen yet: the first card row at the next read
}

// lanzado is a launch the model ordered that did not end yet.
type lanzado struct {
	o          Lanzar
	antes      string // the running id its card had when it was ordered
	registrado bool
	asiento    *asiento
	encadenar  string // an aprobar whose Firma said "y despues: hoom piloto <slug>": that slug
}

// visto is what a card the model saw with running was running.
type visto struct {
	rol, envelope string
}

// parada is the stop of a pilot this board launched (see the package doc,
// "Seguir solo").
type parada struct {
	code    int
	motivo  string
	columna string
	running string // the running id the card had when it stopped
}

// Nuevo is the model before the first read of the board: the focus on the
// list (which says "leyendo el tablero..."), the cards view, no modal, the
// Registro empty, the normal mode.
func Nuevo(c Config) *Modelo {
	m := &Modelo{
		c:        c,
		s:        signosASCII,
		zona:     ZonaLista,
		vista:    VistaTarjetas,
		sel:      elegido{nada: true},
		lanzados: map[int]*lanzado{},
		vistos:   map[string]visto{},
		paradas:  map[string]parada{},
	}
	if c.UTF8 {
		m.s = signosUTF8
	}
	return m
}

// Leer takes a read of the board.
//
//   - A read with Err keeps the screen of the last board and puts the
//     message line's read error (see Lectura).
//   - The list's selection stays on its row (a card, HECHO's line). When
//     the chosen card is no longer on the board, the selection moves to the
//     row that now has its index (the last one if there are fewer) and the
//     detail shows it; when the focus was on the detail showing that card,
//     or a modal of that card was open, the modal closes, the focus goes
//     back to the list and the message line says "la tarjeta <slug> ya no
//     existe" for DuracionAviso (CA-578).
//   - A card the model saw with running, read now without it, adds to the
//     Registro the entry "<rol> termino en <slug>: <plain>" (its plain of
//     now) and puts the same text in the message line for DuracionFin,
//     green when its envelope closed entregable and red when not, by the
//     v1 rule: red is the card's red being the envelope's (red.source ==
//     boardcmd.RedEnvelope and red.id == the envelope id it saw running),
//     the card showing interrupted for that envelope, or the work having
//     had no envelope at all. It does so whoever launched it. Several cards
//     that stopped in one read show one after another in the message line,
//     in the board's order, each for DuracionFin; an end notice takes the
//     line from a message given before it, and a message given while it
//     shows takes the line for its time and the end notice comes back
//     after it, whole: none is lost.
//   - A launch the model is waiting for (a role's or the pilot's) is
//     "registered" when its card shows a running whose envelope id (or run
//     id when it has no envelope) the card did not have when the order was
//     given: its Registro entry says "corriendo".
//   - A pilot's stop stops standing when its card leaves the column it
//     stopped in, shows a running with another id, or is gone.
//   - A Firma, Guardar or Seguir solo modal open on a card is checked
//     against the read (the v1 rule that ties a confirmation to what it
//     shows; see the package doc, "The modals" and "Seguir solo").
func (m *Modelo) Leer(l Lectura) []Orden {
	m.fijada = l.Fijada
	if l.Err != nil {
		m.errTablero = "no se pudo leer el tablero: " + l.Err.Error()
		m.acomodar()
		return nil
	}
	if !l.Ahora.IsZero() {
		m.ahora = l.Ahora
	}
	m.errTablero = ""
	enDetalle := m.zona == ZonaDetalle && m.vista == VistaTarjetas && !m.verProyecto
	antes := m.elegidaDeLista()
	m.tablero = l.Tablero
	m.leido = true

	// the cards that stopped running
	for _, c := range m.todas() {
		v, ok := m.vistos[c.Slug]
		if !ok || c.Running != nil {
			continue
		}
		e := verde
		if fallo(c, v) {
			e = rojo
		}
		texto := fmt.Sprintf("%s termino en %s: %s", v.rol, c.Slug, c.Plain)
		m.barra.encolarFin(texto, e)
		m.registro = append(m.registro, &asiento{hora: m.ahora, tipo: asientoFin, texto: texto, estilo: e})
	}
	m.barra.avanzar(m.ahora)
	m.vistos = map[string]visto{}
	for _, c := range m.todas() {
		if r := c.Running; r != nil {
			m.vistos[c.Slug] = visto{rol: rolDe(r.Role), envelope: r.EnvelopeID}
		}
	}
	// the launches its card now shows running
	for _, la := range m.lanzados {
		if la.o.Rol == "" || la.registrado {
			continue
		}
		if c := m.tarjeta(la.o.Slug); c != nil && c.Running != nil {
			if id := idDe(c.Running); id != "" && id != la.antes {
				la.registrado = true
				if la.asiento != nil && la.asiento.estado == estadoLanzando {
					la.asiento.estado = estadoCorriendo
				}
			}
		}
	}
	// the pilots' stops that no longer stand
	for slug, p := range m.paradas {
		c := m.tarjeta(slug)
		if c == nil || c.Column != p.columna || c.Running != nil && idDe(c.Running) != p.running {
			delete(m.paradas, slug)
		}
	}
	// the card of a modal, or the one the detail showed with the focus,
	// that is no longer on the board
	if slug := m.slugDelModal(); slug != "" && m.tarjeta(slug) == nil {
		m.cerrarModal()
		m.ponerZona(ZonaLista)
		m.avisar(fmt.Sprintf("la tarjeta %s ya no existe", slug), normal, DuracionAviso)
	} else if enDetalle && antes != "" && m.tarjeta(antes) == nil {
		m.ponerZona(ZonaLista)
		m.avisar(fmt.Sprintf("la tarjeta %s ya no existe", antes), normal, DuracionAviso)
	}
	m.revisarConfirmacion()
	m.acomodar()
	return nil
}

// LeerDetalle takes the read of the card [0] shows (Elegida); one of
// another card is ignored. It checks an open Firma modal again (its spec).
func (m *Modelo) LeerDetalle(d Detalle) {
	if d.Slug == "" || d.Slug != m.Elegida() {
		return
	}
	m.detalle = &d
	m.revisarConfirmacion()
	m.acomodar()
}

// LeerProyecto takes a read of the project ([1] and its [0]). A read with
// Err keeps what the last one showed and puts the message line's
// "no se pudo leer el proyecto: <Err>" until a read works (CA-562).
func (m *Modelo) LeerProyecto(p Proyecto) {
	if p.Err != nil {
		m.errProyecto = "no se pudo leer el proyecto: " + p.Err.Error()
		return
	}
	m.errProyecto = ""
	m.proyecto = &p
	m.acomodar()
}

// LeerHallazgos takes a read of the project's open findings ([4] and its
// [0]). A read with Err keeps the last list and puts the message line's
// "no se pudieron leer los hallazgos: <Err>" until a read works (CA-562).
// [4]'s selection stays on its finding (by id); when that finding is gone,
// it moves to the row that now has its index (the last one if there are
// fewer).
func (m *Modelo) LeerHallazgos(h Hallazgos) {
	if h.Err != nil {
		m.errHall = "no se pudieron leer los hallazgos: " + h.Err.Error()
		return
	}
	m.errHall = ""
	m.hallazgos = ordenarHallazgos(h.Items)
	m.hallLeidos = true
	m.acomodar()
}

// ordenarHallazgos are the findings with the high ones first, then the
// medium ones, then the low ones, then any other severity, each group in
// the order of the read.
func ordenarHallazgos(items []finding.Item) []finding.Item {
	out := append([]finding.Item(nil), items...)
	rango := func(s string) int {
		switch s {
		case "high":
			return 0
		case "medium":
			return 1
		case "low":
			return 2
		}
		return 3
	}
	sort.SliceStable(out, func(i, j int) bool { return rango(out[i].Severity) < rango(out[j].Severity) })
	return out
}

// LeerArchivo takes the read of the file of the finding [0] shows; one
// whose Ruta and Linea are not ArchivoElegido's now is ignored.
func (m *Modelo) LeerArchivo(a Archivo) {
	ruta, linea := m.ArchivoElegido()
	if ruta == "" || a.Ruta != ruta || a.Linea != linea {
		return
	}
	m.archivo = &a
	m.acomodar()
}

// Termino takes the end of a process the model ordered, and returns the
// orders that end brings. Its Registro entry says `hecho` when Code is 0
// and `fallo: <Ultima>` otherwise (Ultima empty: `hoom <verb> salio con
// <Code>`, the verb being the first of the order's Args); a pilot's (Seguir
// solo) says `se detuvo: <rest>` when its Ultima starts with "piloto
// automatico: se detiene: " (see the package doc, "Seguir solo"). As in the
// v1, the message line also says it: a role launch that ends before it was
// registered (see Leer) puts its Ultima there for DuracionAviso, red when
// Code is not 0; one that ends after leaves the line to Leer's notice;
// usar, guardar, aprobar, integrar and the pilot put their Ultima there,
// green with Code 0 and red otherwise.
//
// The only orders it returns: when the process is an aprobar whose Firma
// modal said "y despues: hoom piloto <slug>" and Code is 0, the
// Lanzar{Rol: "piloto", Args: {"piloto", <slug>}} of the pilot, with its
// own Registro entry (CA-717). Otherwise none.
func (m *Modelo) Termino(f FinProceso, ahora time.Time) []Orden {
	if !ahora.IsZero() {
		m.ahora = ahora
	}
	la, ok := m.lanzados[f.ID]
	if !ok {
		return nil
	}
	delete(m.lanzados, f.ID)
	texto := f.Ultima
	if texto == "" && f.Code != 0 {
		verbo := ""
		if len(la.o.Args) > 0 {
			verbo = la.o.Args[0]
		}
		texto = fmt.Sprintf("hoom %s salio con %d", verbo, f.Code)
	}
	resultado := verde
	if f.Code != 0 {
		resultado = rojo
	}
	if la.o.Rol == rolPiloto {
		m.terminoPiloto(la, f, texto, resultado)
		m.acomodar()
		return nil
	}
	if a := la.asiento; a != nil {
		if f.Code == 0 {
			a.estado = estadoHecho
		} else {
			a.estado = "fallo: " + unaLinea(texto)
		}
	}
	switch {
	case la.o.Rol != "" && la.registrado:
		// Leer says how it ended
	case la.o.Rol != "":
		e := normal
		if f.Code != 0 {
			e = rojo
		}
		if texto != "" {
			m.avisar(texto, e, DuracionAviso)
		}
	default:
		if texto != "" {
			m.avisar(texto, resultado, DuracionAviso)
		}
	}
	var ords []Orden
	if la.encadenar != "" && f.Code == 0 {
		ords = append(ords, m.lanzarPiloto(la.encadenar).o)
	}
	m.acomodar()
	return ords
}

// prefijoParada is how hoom piloto says why it stops.
const prefijoParada = "piloto automatico: se detiene: "

// rolPiloto is the Rol of the pilot's Lanzar.
const rolPiloto = "piloto"

// terminoPiloto is the end of a pilot this board launched: its entry's
// estado, the message line and its stop.
func (m *Modelo) terminoPiloto(la *lanzado, f FinProceso, texto string, e estilo) {
	resto, parado := strings.CutPrefix(f.Ultima, prefijoParada)
	if a := la.asiento; a != nil {
		switch {
		case parado:
			a.estado, a.estadoEstilo = "se detuvo: "+unaLinea(resto), e
		case f.Code == 0:
			a.estado = estadoHecho
		default:
			a.estado = "fallo: " + unaLinea(texto)
		}
	}
	if texto != "" {
		m.avisar(texto, e, DuracionAviso)
	}
	motivo := f.Ultima
	switch {
	case parado:
		motivo = resto
	case motivo == "":
		motivo = fmt.Sprintf("hoom piloto salio con %d", f.Code)
	}
	if c := m.tarjeta(la.o.Slug); c != nil {
		p := parada{code: f.Code, motivo: unaLinea(motivo), columna: c.Column}
		if c.Running != nil {
			p.running = idDe(c.Running)
		}
		m.paradas[c.Slug] = p
	}
}

// lanzarPiloto orders the pilot of slug, with its Registro entry.
func (m *Modelo) lanzarPiloto(slug string) *lanzado {
	args := []string{"piloto", slug}
	la := m.lanzar(slug, rolPiloto, args)
	m.registrarVerbo(la, etiquetaSeguir, lineaVerbo(args, m.c.Windows))
	return la
}

// PedidoEditado takes what the editor gave back for the pedido of the open
// Pedir modal: with err, the pedido stays as it was and the message line
// says "el editor salio con error" in red for DuracionAviso; without it,
// the text is the pedido, its cursor at its end.
func (m *Modelo) PedidoEditado(texto string, err error) {
	if m.pedir == nil {
		return
	}
	if err != nil {
		m.avisar("el editor salio con error", rojo, DuracionAviso)
		return
	}
	m.pedir.pedido.poner(texto)
	m.pedir.pedidoVacio = false
	m.acomodar()
}

// Tamano takes a new size of the terminal: the panels are laid out again
// and an open modal is centered again, with the rule of what does not fit.
func (m *Modelo) Tamano(ancho, alto int) {
	m.c.Ancho, m.c.Alto = ancho, alto
	m.acomodar()
}

// Tic moves the clock: the message line's messages expire and the times on
// screen ("hace <n>s", "12:41 de 45:00") move.
func (m *Modelo) Tic(ahora time.Time) {
	m.ahora = ahora
	m.barra.avanzar(m.ahora)
}

// Foco is the zone with the focus: ZonaLista at the start. There is always
// one, also while a modal is open (the zone under it).
func (m *Modelo) Foco() Zona {
	return m.zona
}

// Vista is what the list shows: VistaTarjetas at the start, VistaHallazgos
// after H (CA-712).
func (m *Modelo) Vista() Vista {
	return m.vista
}

// Grupo is the group of the card slug of the last read of the board, by
// the rule of the package doc ("The groups"), whatever the view and the
// search show; "" when the board has no such card or before the first
// read.
func (m *Modelo) Grupo(slug string) Grupo {
	if !m.leido {
		return ""
	}
	c := m.tarjeta(slug)
	if c == nil {
		return ""
	}
	return m.grupoDe(c)
}

// Caja is the slug of the box's card (TU SIGUIENTE PASO): the first card of
// TE TOCA in the list's order, else the first of DETENIDA; "" without one
// (and before the first read), also when the box offers the first card of
// EN COLA (CA-698, CA-699).
func (m *Modelo) Caja() string {
	if c, _ := m.tarjetaDeCaja(); c != nil {
		return c.Slug
	}
	return ""
}

// Modal is the open modal (SinModal when none).
func (m *Modelo) Modal() Modal {
	return m.modal
}

// Lugar is where the keys are now: the Lugar whose rows of Atajos the
// next key is looked up in.
func (m *Modelo) Lugar() Lugar {
	switch m.modal {
	case ModalPedir:
		return m.pedir.lugar()
	case ModalFirma:
		return LugarFirma
	case ModalGuardar:
		return LugarGuardar
	case ModalSeguir:
		return LugarSeguir
	case ModalAcciones:
		return LugarAcciones
	case ModalAyuda:
		return LugarAyuda
	case ModalRegistro:
		return LugarRegistro
	}
	if m.buscando {
		return LugarBusqueda
	}
	if m.zona == ZonaLista {
		if m.vista == VistaHallazgos {
			return LugarHallazgos
		}
		return LugarTarjetas
	}
	switch {
	case m.verProyecto:
		return LugarVerProyecto
	case m.vista == VistaHallazgos:
		if _, ok := m.hallazgoElegido(); ok {
			return LugarVerHallazgo
		}
	case m.elegidaDeLista() != "":
		return LugarVerTarjeta
	}
	return LugarVerNada
}

// Elegida is the slug of the card the detail shows, the one whose Detalle
// Run reads: the card chosen in the list of cards; "" when the detail shows
// the project, a finding or nothing (HECHO's line, an empty list).
func (m *Modelo) Elegida() string {
	if m.vista != VistaTarjetas || m.verProyecto {
		return ""
	}
	return m.elegidaDeLista()
}

// ArchivoElegido is the file of the finding the detail shows, relative to
// the project root with /, and its line (0 when the finding names none),
// the one whose Archivo Run reads; "" when the detail shows no finding, the
// finding names no file, or its path leaves the project by its form (see
// the package doc, "Paths").
func (m *Modelo) ArchivoElegido() (ruta string, linea int) {
	if m.vista != VistaHallazgos || m.verProyecto {
		return "", 0
	}
	h, ok := m.hallazgoElegido()
	if !ok {
		return "", 0
	}
	archivo, n := m.rutaHallazgo(h)
	if archivo == "" || fueraPorLaForma(archivo) {
		return "", 0
	}
	return archivo, n
}

// rutaHallazgo is the file of the finding h relative to the project root,
// and its line: as the v1, the file is under the evidence dir of the card
// of its task (evidence.dir + "/" + file, or the file alone when the dir is
// "" or ".", or when the file is absolute: the rules of the paths judge
// it).
func (m *Modelo) rutaHallazgo(h finding.Item) (string, int) {
	archivo, n := archivoYLinea(h)
	if archivo == "" || absoluta(archivo) {
		return archivo, n
	}
	if c := m.tarjeta(h.Task); c != nil {
		if dir := c.Evidence.Dir; dir != "" && dir != "." {
			archivo = dir + "/" + archivo
		}
	}
	return archivo, n
}

// Registro is the Registro's entries, oldest first: every verb the TUI
// ran, every e, and every end of a job (see the package doc, "The
// Registro"). It lives while the model does.
func (m *Modelo) Registro() []Asiento {
	out := make([]Asiento, 0, len(m.registro))
	for _, a := range m.registro {
		out = append(out, Asiento{Hora: a.hora, Texto: a.primera(m.s), Comando: a.comando})
	}
	return out
}

// Corriendo is how many role jobs (Lanzar with Rol) the model ordered that
// did not end yet (Termino).
func (m *Modelo) Corriendo() int {
	n := 0
	for _, la := range m.lanzados {
		if la.o.Rol != "" {
			n++
		}
	}
	return n
}

// MensajeSalida is what hoom tablero prints when it gives the terminal
// back: "quedan <n> trabajos corriendo: hoom status" ("queda 1 trabajo
// corriendo: hoom status" for one), or "" when none is left.
func MensajeSalida(corriendo int) string {
	switch {
	case corriendo <= 0:
		return ""
	case corriendo == 1:
		return "queda 1 trabajo corriendo: hoom status"
	}
	return fmt.Sprintf("quedan %d trabajos corriendo: hoom status", corriendo)
}

// fallo says whether the work the model saw running on c ended without
// closing entregable: it had no envelope (a run of hoom run by hand closes
// none), the card's red is its envelope's, or the card shows its envelope
// interrupted (its owner died).
func fallo(c *boardcmd.Card, v visto) bool {
	if v.envelope == "" {
		return true
	}
	if c.Red != nil && c.Red.Source == boardcmd.RedEnvelope && c.Red.ID == v.envelope {
		return true
	}
	return c.Interrupted != nil && c.Interrupted.EnvelopeID == v.envelope
}

// ---------------------------------------------------------------- helpers

// avisar puts a message in the message line for d.
func (m *Modelo) avisar(texto string, e estilo, d time.Duration) {
	m.barra.dar(texto, e, m.ahora, d)
}

// mensaje is the message line's message now: the last one while it lasts,
// else the read errors.
func (m *Modelo) mensaje() (string, estilo) {
	if m.barra.mostrando(m.ahora) {
		return m.barra.actual.texto, m.barra.actual.estilo
	}
	for _, e := range []string{m.errTablero, m.errProyecto, m.errHall} {
		if e != "" {
			return e, rojo
		}
	}
	return "", normal
}

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

// pequena says whether the screen is under the smallest one.
func (m *Modelo) pequena() bool {
	return m.c.Ancho < anchoMinimo || m.c.Alto < altoMinimo
}

// ponerZona gives the focus to z. Leaving the detail ends the project's
// view (v) and leaving the list closes the search.
func (m *Modelo) ponerZona(z Zona) {
	if z != m.zona {
		m.prefijo = ""
	}
	m.zona = z
	if z == ZonaLista {
		m.verProyecto = false
	}
	if z != ZonaLista && m.buscando {
		m.cerrarBusqueda()
	}
}
