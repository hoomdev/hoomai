package tablero

import (
	"fmt"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Config is what the model knows from the start.
type Config struct {
	Proyecto string // the project's name (hoom.yaml's project), for the title
	Ancho    int    // the terminal's size, in cells
	Alto     int
	Color    bool // false with NO_COLOR (see ConColor)
	UTF8     bool // false without a UTF-8 locale or with HOOM_ASCII=1 (see ConUTF8)
	// Windows makes the → line quote for Windows, as the capa-1 verbs do
	// there (tarjetacmd.Citar).
	Windows bool
}

// Lectura is one read of the board: Run reads it every 2 seconds.
type Lectura struct {
	Ahora   time.Time
	Tablero boardcmd.Board // the same as hoom board --json
	// Err: the read failed. The model keeps the last board and the bar says
	// "no se pudo leer el tablero: <Err>" until a read works.
	Err    error
	Fijada string // the card hoom usar fixed ("" = none)
}

// Detalle is the open card as Run reads it with every Lectura while a card
// is open (Modelo.Abierta).
type Detalle struct {
	Slug     string
	Detalle  boardcmd.Detail   // spec, criteria, findings, work
	Historia boardcmd.Timeline // only read while the Historia tab is active
	Sobre    Sobre
	Err      error // the read failed: the tab says so and keeps nothing stale
}

// Sobre is the narration of the card's envelope at work or, without one,
// of its last one: the last 64 KB of its run's narration (MaxSobre).
type Sobre struct {
	EnvelopeID string
	RunID      string
	Eventos    []runcmd.Event
	Err        error
}

// MaxSobre is how much of a narration the Sobre tab reads: its last bytes.
const MaxSobre = 64 << 10

// FinProceso is a process the TUI launched that ended (or could not start).
type FinProceso struct {
	ID   int // Lanzar.ID of the order that started it
	Code int // its exit code; -1 when it could not start
	// Ultima is the last line of what it said that is not empty: of its
	// stderr, or of its stdout when stderr was empty (or the start error),
	// without a leading "hoom: ".
	Ultima string
}

// Orden is something the model asks Run to do.
type Orden interface{ orden() }

// Lanzar runs `hoom <Args...>` as a detached process of its own (its own
// process group), in the directory hoom tablero was started from, with
// stdin empty and its output to files: leaving the TUI does not cut it.
type Lanzar struct {
	ID   int    // the model's number for it, from 1, one per order
	Slug string // the card
	// Rol is the role it launches ("" for usar, guardar, aprobar and
	// integrar): the ones with a role are the jobs the exit counts.
	Rol  string
	Args []string // after "hoom", e.g. {"pedir", "writer", "<slug>", "-m", "<pedido>", "--model", "x"}
}

// AbrirEditor opens a file in the editor (ArgumentosEditor); Run releases the
// screen while it is open and takes it back after.
type AbrirEditor struct {
	Archivo string // relative to the project root
	Linea   int    // 1-based; 0 = no line
}

// EditarPedido opens the pedido in the editor; Run gives the result back
// with Modelo.PedidoEditado.
type EditarPedido struct {
	Texto string
}

// Salir ends the TUI.
type Salir struct{}

func (Lanzar) orden()       {}
func (AbrirEditor) orden()  {}
func (EditarPedido) orden() {}
func (Salir) orden()        {}

// How long the bar keeps a message.
const (
	DuracionAviso = 5 * time.Second // a why, a key with nothing to do, a card that no longer exists
	DuracionFin   = 8 * time.Second // "<rol> termino en <slug>: <plain>"
)

// Modelo is the TUI without a terminal: keys and reads in, a screen and
// orders out. It is not safe for concurrent use: Run calls it from one
// goroutine.
type Modelo struct {
	c     Config
	s     signos
	ahora time.Time

	leido      bool
	tablero    boardcmd.Board
	fijada     string
	errLectura string
	experto    bool

	vista Vista

	// the list: the selection is a card (selSlug) or Hecho's line
	// (selHecho); selIdx is its place among the lines that can be chosen
	sinSeleccion bool
	selSlug      string
	selHecho     bool
	selIdx       int
	hechoAbierto bool
	scrollLista  int
	filtro       []rune // the search's text, while VistaBusqueda

	// the open card
	abierta  string
	pestana  Vista
	detalle  *Detalle
	scrolls  map[Vista]int
	alFinal  bool // Sobre follows its end
	hallazgo int
	prefijo  Tecla // "]" or "[" waiting for its second stroke

	// what is over the list or the card
	menu    *menu
	form    *formulario
	conf    *confirmacion
	ayudaDe Vista

	// the bar's message and the end notices waiting for it
	barra barra

	// the launches
	sigID    int
	lanzados map[int]*lanzado
	vistos   map[string]visto
}

// lanzado is a launch the model ordered that did not end yet.
type lanzado struct {
	o          Lanzar
	antes      string // the running id its card had when it was ordered
	registrado bool
}

// visto is what a card the model saw with running was running.
type visto struct {
	rol, envelope string
}

// Nuevo is the model before the first read of the board.
func Nuevo(c Config) *Modelo {
	m := &Modelo{
		c:            c,
		s:            signosASCII,
		vista:        VistaLista,
		sinSeleccion: true,
		scrolls:      map[Vista]int{},
		lanzados:     map[int]*lanzado{},
		vistos:       map[string]visto{},
	}
	if c.UTF8 {
		m.s = signosUTF8
	}
	return m
}

// Leer takes a read of the board.
//
//   - A read with Err keeps the screen of the last board and puts the bar's
//     read error (see Lectura).
//   - The selection stays on its card. When that card is no longer on the
//     board, it moves to the line that now has its index (the last one if
//     there are fewer); when the card was open, the view goes back to the
//     list and the bar says "la tarjeta <slug> ya no existe" for
//     DuracionAviso.
//   - A card the model saw with running, read now without it, puts in the
//     bar for DuracionFin "<rol> termino en <slug>: <plain>" (its plain of
//     now): green when its envelope closed entregable, red when not (the
//     spec: "en verde si el sobre cerró entregable y en rojo si no"). Red
//     is: the card's red is the envelope's (red.source ==
//     boardcmd.RedEnvelope and red.id == the envelope id it saw running),
//     the card shows interrupted for that envelope (the work died), or the
//     work had no envelope at all (a run of hoom run by hand: the roles and
//     the review always have one; without an envelope nothing closed
//     entregable). Green otherwise. It does so whoever launched it. Several cards that
//     stopped in one read show one after another, in the board's order,
//     each for DuracionFin. An end notice takes the bar from a message
//     given before it; a message given while it shows (this read's "ya no
//     existe", a closed confirmation, a key) takes the bar for its time and
//     the end notice comes back after it, whole: none is lost.
//   - A launch the model is waiting for is "registered" when its card shows
//     a running whose envelope id (or run id when it has no envelope) the
//     card did not have when the order was given.
func (m *Modelo) Leer(l Lectura) []Orden {
	m.fijada = l.Fijada
	if l.Err != nil {
		// the screen stays the last board's, its times included: the clock
		// moves with Tic and with the reads that work
		m.errLectura = "no se pudo leer el tablero: " + l.Err.Error()
		m.acomodar()
		return nil
	}
	if !l.Ahora.IsZero() {
		m.ahora = l.Ahora
	}
	m.errLectura = ""
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
		m.barra.encolarFin(fmt.Sprintf("%s termino en %s: %s", v.rol, c.Slug, c.Plain), e)
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
			}
		}
	}
	// the open card (or the one under the menu, the form or the
	// confirmation) that is no longer on the board
	if slug := m.slugEnUso(); slug != "" && m.tarjeta(slug) == nil {
		m.cerrarTodo()
		m.avisar(fmt.Sprintf("la tarjeta %s ya no existe", slug), normal, DuracionAviso)
	}
	m.revisarConfirmacion()
	m.acomodar()
	return nil
}

// LeerDetalle takes the read of the open card; one of another card is
// ignored.
func (m *Modelo) LeerDetalle(d Detalle) {
	if m.abierta == "" || d.Slug != m.abierta {
		return
	}
	m.detalle = &d
	m.revisarConfirmacion()
	m.acomodar()
}

// Tecla takes one key at ahora. What a key does is the Atajos row of the
// current Vista that has it; a key with no row there does nothing (the
// screen and the orders stay the same). In the views where the card's
// action keys work (the list, the open card, the menu), on the selected or
// open card:
//
//   - a, t, w, r, R, s, A, I: when the card has the Accion of the row and
//     it is enabled, they open its form (a, t, w, r, R), its confirmation
//     (A, I, s); when it does not have it, the bar says for DuracionAviso
//     "en <column_name> no hay <hace>" (Hace of the row); when it is
//     disabled, its why. Nothing runs.
//   - n: what hoom siguiente does. When the card is in Hecho, the bar says
//     "la tarjeta ya esta en Hecho"; with running, "el <rol> esta
//     trabajando: espera a que termine"; with interrupted, "el trabajo se
//     corto: R reanuda o pide de nuevo"; in Tu aprobacion, "te toca: A
//     aprobar"; in Tu aceptacion, "te toca: I integrar"; without actions,
//     "en <column_name> no hay nada que hacer ahora: <plain>". Otherwise,
//     when the first action is a role action: its form if it is enabled,
//     its why if not; when the first action is guardar: "te toca: s
//     guardar". The form of n runs `hoom siguiente <slug>`.
//   - u: orders Lanzar{Args: {"usar", <slug>}}.
//   - Without a selected card (the selection on Hecho's line), these keys
//     do nothing.
//
// The form (VistaFormulario) takes the action's values: the provider
// options, the default one chosen (the first that can when none is
// default), model, effort, budget_usd (empty without one), timeout_s
// (valores.Duracion) and pedido, the focus on Pedido (on Provider for the
// reviewer) with the cursor at its end. enter with an empty pedido marks it
// and the bar says "el pedido esta vacio"; with a provider that cannot,
// its why; with a Tiempo limite that is not a duration between 1m and 12h,
// manifest.ParseTimeout's error; with a Presupuesto that is not a number,
// "el presupuesto no es un numero"; otherwise it orders Lanzar and goes
// back to where it was opened from, with "lanzando hoom <verb>" in the bar
// for DuracionAviso (e.g. "lanzando hoom pedir writer <slug>", "lanzando
// hoom siguiente <slug>": the verb's words, without -m and the flags). The order's Args are the verb ({"pedir", <rol>, <slug>}, {"siguiente",
// <slug>} or {"reanudar", <slug>}), "-m" and the trimmed pedido (not for the
// reviewer, nor its --timeout), then, in this order and only when its value differs from the
// action's proposal: "--provider" <name>, "--model" <text>, "--effort"
// <text>, "--budget-usd" <text> (an empty Presupuesto is "0", no cap),
// "--timeout" <text>. The → line is "→ hoom " and those Args, each value
// quoted as tarjetacmd.Citar does it (the pedido as tarjetacmd.CitarPedido:
// cut to 60 characters), cut to the box.
//
// A confirmation of A or I orders Lanzar{Args: {"aprobar"|"integrar",
// <slug>, "--si"}} only with enter on Firmar; enter on Cancelar or esc
// close it. The one of s orders Lanzar{Args: {"guardar", <slug>}} with
// enter.
//
// q and ctrl-c order Salir.
func (m *Modelo) Tecla(t Tecla, ahora time.Time) []Orden {
	if !ahora.IsZero() {
		m.ahora = ahora
	}
	v := m.Vista()
	// ]c and [c: two strokes
	if v == VistaSpec {
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
	a, ok := atajo(v, t)
	if !ok {
		return nil
	}
	ordenes := m.hacer(v, a, t)
	m.acomodar()
	return ordenes
}

// Termino takes the end of a process the model ordered. A launch that ends
// before it was registered (see Leer) puts its Ultima in the bar for
// DuracionAviso: red when Code is not 0. One that ends after leaves the bar
// to Leer's notice. usar, guardar, aprobar and integrar put their Ultima in
// the bar, green with Code 0 and red otherwise.
func (m *Modelo) Termino(f FinProceso, ahora time.Time) {
	if !ahora.IsZero() {
		m.ahora = ahora
	}
	la, ok := m.lanzados[f.ID]
	if !ok {
		return
	}
	delete(m.lanzados, f.ID)
	texto := f.Ultima
	if texto == "" && f.Code != 0 {
		texto = fmt.Sprintf("hoom %s salio con %d", palabrasVerbo(la.o.Args), f.Code)
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
		e := verde
		if f.Code != 0 {
			e = rojo
		}
		if texto != "" {
			m.avisar(texto, e, DuracionAviso)
		}
	}
	m.acomodar()
}

// PedidoEditado takes what the editor gave back for the pedido: with err,
// the pedido stays as it was and the bar says "el editor salio con error";
// without it, the text is the pedido, cursor at its end.
func (m *Modelo) PedidoEditado(texto string, err error) {
	if m.form == nil {
		return
	}
	if err != nil {
		m.avisar("el editor salio con error", rojo, DuracionAviso)
		return
	}
	if c := m.form.campo(campoPedido); c != nil {
		c.poner(texto)
		m.form.pedidoVacio = false
	}
}

// Tamano takes a new size of the terminal.
func (m *Modelo) Tamano(ancho, alto int) {
	m.c.Ancho, m.c.Alto = ancho, alto
	m.acomodar()
}

// Tic moves the clock: the bar's messages expire and the times on screen
// ("hace <n>s") move.
func (m *Modelo) Tic(ahora time.Time) {
	m.ahora = ahora
	m.barra.avanzar(m.ahora)
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

// Vista is the current view.
func (m *Modelo) Vista() Vista {
	return m.vista
}

// Abierta is the slug of the open card and whether its Historia tab is the
// active one ("" in the list).
func (m *Modelo) Abierta() (slug string, historia bool) {
	if m.abierta == "" {
		return "", false
	}
	return m.abierta, m.pestana == VistaHistoria
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
