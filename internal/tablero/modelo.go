package tablero

import (
	"fmt"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/modelos"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/statuscmd"
)

// Config is what the model knows from the start.
type Config struct {
	Proyecto string // the project's name (hoom.yaml's project, else the root's base name), for [1]
	Ancho    int    // the terminal's size, in cells
	Alto     int
	Color    bool // false with NO_COLOR (see ConColor)
	UTF8     bool // false without a UTF-8 locale or with HOOM_ASCII=1 (see ConUTF8)
	// Windows makes the commands of the modals and of the Registro quote
	// for Windows, as the capa-1 verbs do there (tarjetacmd.Citar).
	Windows bool
	// Mouse says that Run asked the terminal for the mouse (ConMouse): the
	// help's "Mouse" group says what it does, or that it is off.
	Mouse bool
	// Editor is the editor's command (ComandoEditor): the Registro writes
	// what e opens with it (see the package doc, "The Registro").
	Editor string
	// Modelos are hoom.yaml's models: (manifest.Manifest.Models), by
	// provider, as modelos.Propio: the Modelo selector offers them after the
	// catalog's (modelos.Modelos). nil = none.
	Modelos map[string][]modelos.Propio
}

// Lectura is one read of the board: Run reads it every Intervalo.
type Lectura struct {
	Ahora   time.Time
	Tablero boardcmd.Board // the same as hoom board --json
	// Err: the read failed. The model keeps the last board and the message
	// line says "no se pudo leer el tablero: <Err>" until a read works.
	Err    error
	Fijada string // the card hoom usar fixed ("" = none): [1]'s fourth line
}

// Detalle is the read of the card [0] shows (Modelo.Elegida): Run reads it
// when Elegida changes and with every Lectura while it is not "".
type Detalle struct {
	Slug     string
	Detalle  boardcmd.Detail   // spec, criteria, findings, work
	Historia boardcmd.Timeline // the card's timeline (boardcmd.TimelineFor)
	Sobre    Sobre
	Err      error // the read failed: [0] says so and keeps nothing stale
}

// Sobre is the narration of the card's envelope at work or, without one,
// of its last one: the last 64 KB of its run's narration (MaxSobre).
type Sobre struct {
	EnvelopeID string
	RunID      string
	Eventos    []runcmd.Event
	Err        error
}

// MaxSobre is how much of a narration [0] reads: its last bytes.
const MaxSobre = 64 << 10

// Proyecto is one read of what [1] and its [0] show (spec, "Los datos"):
// Run reads it at the start, every IntervaloProyecto and when a process the
// TUI launched ends.
type Proyecto struct {
	// Git says whether the root is in a git repository; without one [1]
	// says "sin git".
	Git bool
	// Rama is the branch gitx reads (the one a verdict keeps).
	Rama string
	// Check is hoom check's result (checkcmd.Run, the function of hoom
	// status).
	Check checkcmd.Result
	// Veredicto is the project's newest verdict, as hoom status reads it
	// (statuscmd's last_verdict); nil without one.
	Veredicto *statuscmd.LastVerdict
	// Hoy is what today's runs cost (costcmd.HoyEn, the function of hoom
	// costo).
	Hoy costcmd.Hoy
	// Problemas are hoom board doctor's (boardcmd.Doctor).
	Problemas []boardcmd.Problem
	// Err: the read failed. The model keeps the last Proyecto and the
	// message line says "no se pudo leer el proyecto: <Err>" until a read
	// works.
	Err error
}

// Hallazgos is one read of the project's open findings ([4] and its [0]),
// from the function of hoom finding list --open (finding.List with
// openOnly), oldest first as it gives them. Run reads it with Proyecto.
type Hallazgos struct {
	Items []finding.Item
	// Err: the read failed. The model keeps the last list and the message
	// line says "no se pudieron leer los hallazgos: <Err>" until a read
	// works.
	Err error
}

// ContextoArchivo is how many lines Datos.Archivo reads before and after a
// finding's line.
const ContextoArchivo = 10

// Archivo is the read of the file a finding names, for its [0]
// (Modelo.ArchivoElegido): Run reads it, through the project's os.Root,
// when ArchivoElegido changes.
type Archivo struct {
	Ruta  string // as ArchivoElegido gave it: relative to the project root, with /
	Linea int    // as ArchivoElegido gave it (0 = the finding names no line)
	// Desde is the number (from 1) of the first line of Lineas.
	Desde int
	// Lineas are the file's lines from max(1, Linea-ContextoArchivo) to
	// Linea+ContextoArchivo (fewer at the end of the file), without their
	// line ends; with Linea 0, its first 2*ContextoArchivo+1 lines.
	Lineas []string
	// Fuera: a symlink of the project takes the path out of it; nothing
	// was read.
	Fuera bool
	// Err: it could not be read ([0] says "no se pudo leer <Ruta>: <Err>").
	Err error
}

// Asiento is one entry of the Registro, as the screen writes it (see the
// package doc, "The Registro").
type Asiento struct {
	// Hora is when it was added: the model's clock then.
	Hora time.Time
	// Texto is its first line after "<HH:MM> ": "<etiqueta> · <slug> ·
	// <estado>" for a verb, "Abrir en el editor · <archivo>[:<linea>]" for
	// the editor, "<rol> termino en <slug>: <plain>" for the end of a job.
	// A verb's entry changes its estado in place.
	Texto string
	// Comando is its second line after the six spaces: the verb's command
	// as "Se va a ejecutar" showed it, or the editor's; "" for the end of a
	// job, which takes one line.
	Comando string
}

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

// How long the message line keeps a message.
const (
	DuracionAviso = 5 * time.Second // a why, a key with nothing to do, a card that no longer exists
	DuracionFin   = 8 * time.Second // "<rol> termino en <slug>: <plain>"
)

// Modelo is the TUI without a terminal: keys, mouse events and reads in, a
// screen and orders out (the package doc is the contract of its screen and
// of what every key does). It is not safe for concurrent use: Run calls it
// from one goroutine.
type Modelo struct {
	c     Config
	s     signos
	ahora time.Time

	leido      bool
	tablero    boardcmd.Board
	fijada     string
	errLectura string
	experto    bool

	vista vistaV1

	// the list: the selection is a card (selSlug) or Hecho's line
	// (selHecho); selIdx is its place among the lines that can be chosen
	sinSeleccion bool
	selSlug      string
	selHecho     bool
	selIdx       int
	hechoAbierto bool
	scrollLista  int
	filtro       []rune // the search's text, while vistaBusqueda

	// the open card
	abierta  string
	pestana  vistaV1
	detalle  *Detalle
	scrolls  map[vistaV1]int
	alFinal  bool // Sobre follows its end
	hallazgo int
	prefijo  Tecla // "]" or "[" waiting for its second stroke

	// what is over the list or the card
	menu    *menu
	form    *formulario
	conf    *confirmacion
	ayudaDe vistaV1

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

// Nuevo is the model before the first read of the board: the focus on [2]
// (which says "leyendo el tablero..."), no modal, the Registro empty, the
// normal mode.
func Nuevo(c Config) *Modelo {
	m := &Modelo{
		c:            c,
		s:            signosASCII,
		vista:        vistaLista,
		sinSeleccion: true,
		scrolls:      map[vistaV1]int{},
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
//   - A read with Err keeps the screen of the last board and puts the
//     message line's read error (see Lectura).
//   - Each left panel's selection stays on its row (a card, Hecho's line).
//     When the card chosen in [2] is no longer on the board, the selection
//     moves to the row that now has its index (the last one if there are
//     fewer) and [0] shows it; when the focus was on [0] showing that card,
//     or a modal of that card was open, the modal closes, the focus goes
//     back to [2] and the message line says "la tarjeta <slug> ya no
//     existe" for DuracionAviso (CA-578). [3]'s selection follows the same
//     index rule without a message: a card that stops running leaves [3].
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
//   - A launch the model is waiting for is "registered" when its card
//     shows a running whose envelope id (or run id when it has no envelope)
//     the card did not have when the order was given: its Registro entry
//     says "corriendo".
//   - A Firma or Guardar modal open on a card is checked against the read
//     (the v1 rule that ties a confirmation to what it shows; see the
//     package doc, "The Firma modal").
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

// LeerDetalle takes the read of the card [0] shows (Elegida); one of
// another card is ignored. It checks an open Firma modal again (its spec).
func (m *Modelo) LeerDetalle(d Detalle) {
	if m.abierta == "" || d.Slug != m.abierta {
		return
	}
	m.detalle = &d
	m.revisarConfirmacion()
	m.acomodar()
}

// Tecla takes one key at ahora. What a key does is the row of Atajos that
// has it in the current Lugar (Modelo.Lugar); a key with no row there does
// nothing: the screen and the orders stay the same. The package doc says
// what each row does in each Lugar ("The keys"); in short:
//
//   - The keys of an action (a, t, w, r, R, s, A, I and n) never order
//     Lanzar: when the card has the action and it is enabled, they open its
//     modal (Pedir for a, t, w, r and R; Firma for A and I; Guardar for s;
//     n the modal of the next step's action, whatever its kind) and return
//     no order; when the card does not have it, the message line says for
//     DuracionAviso "en <column_name> no hay <Hace>"; when it is disabled,
//     its why. n without a next step says what hoom siguiente would say
//     (see the package doc, "The next step"). Without a card (Hecho's line,
//     an empty [3]) they do nothing.
//   - u orders Lanzar{Rol: "", Args: {"usar", <slug>}} at once (no modal)
//     and adds its Registro entry ("Fijar para la terminal").
//   - Only enter on a modal's action button orders Lanzar (Pedir, Firma,
//     Guardar), with the Args the modal's "Se va a ejecutar" shows, adds
//     the verb's Registro entry and closes the modal. Pedir, as the v1's
//     form, also puts "lanzando hoom <verb>" in the message line for
//     DuracionAviso (the verb's words, without -m and the flags:
//     "lanzando hoom pedir writer <slug>").
//   - e orders AbrirEditor (and adds the editor's Registro entry), ctrl-e
//     in the Pedir modal orders EditarPedido, q and ctrl-c order Salir.
//
// A key that opens a modal returns no order: Modal says which one opened.
func (m *Modelo) Tecla(t Tecla, ahora time.Time) []Orden {
	if !ahora.IsZero() {
		m.ahora = ahora
	}
	v := m.vistaV1()
	// ]c and [c: two strokes
	if v == vistaSpec {
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

// Termino takes the end of a process the model ordered. Its Registro entry
// says "hecho" when Code is 0 and "fallo: <Ultima>" otherwise (Ultima
// empty: "hoom <verb> salio con <Code>"). As in the v1, the message line
// also says it: a role launch that ends before it was registered (see
// Leer) puts its Ultima there for DuracionAviso, red when Code is not 0;
// one that ends after leaves the line to Leer's notice; usar, guardar,
// aprobar and integrar put their Ultima there, green with Code 0 and red
// otherwise.
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

// PedidoEditado takes what the editor gave back for the pedido of the open
// Pedir modal: with err, the pedido stays as it was and the message line
// says "el editor salio con error" in red for DuracionAviso; without it,
// the text is the pedido, its cursor at its end.
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

// LeerProyecto takes a read of the project ([1] and its [0]). A read with
// Err keeps what the last one showed and puts the message line's
// "no se pudo leer el proyecto: <Err>" until a read works (CA-562).
func (m *Modelo) LeerProyecto(p Proyecto) {}

// LeerHallazgos takes a read of the project's open findings ([4] and its
// [0]). A read with Err keeps the last list and puts the message line's
// "no se pudieron leer los hallazgos: <Err>" until a read works (CA-562).
// [4]'s selection stays on its finding (by id); when that finding is gone,
// it moves to the row that now has its index (the last one if there are
// fewer).
func (m *Modelo) LeerHallazgos(h Hallazgos) {}

// LeerArchivo takes the read of the file of the finding [0] shows; one
// whose Ruta and Linea are not ArchivoElegido's now is ignored.
func (m *Modelo) LeerArchivo(a Archivo) {}

// Raton takes one mouse event at ahora (see the package doc, "The mouse").
// It moves the focus, chooses rows, options and buttons and scrolls; it
// never orders Lanzar nor opens a modal, so it returns no order (CA-575).
func (m *Modelo) Raton(r Raton, ahora time.Time) []Orden {
	return nil
}

// Foco is the panel with the focus: PanelTarjetas at the start. There is
// always one, also while a modal is open (the panel under it).
func (m *Modelo) Foco() Panel {
	return 0
}

// Modal is the open modal (SinModal when none).
func (m *Modelo) Modal() Modal {
	return SinModal
}

// Lugar is where the keys are now: the Lugar whose rows of Atajos the
// next key is looked up in.
func (m *Modelo) Lugar() Lugar {
	return ""
}

// Elegida is the slug of the card [0] shows, the one whose Detalle Run
// reads: the card chosen in [2] or in [3] when that panel is the last left
// panel that had the focus; "" when [0] shows Proyecto, a finding or
// nothing.
func (m *Modelo) Elegida() string {
	return ""
}

// ArchivoElegido is the file of the finding [0] shows, relative to the
// project root with /, and its line (0 when the finding names none), the
// one whose Archivo Run reads; "" when [0] shows no finding, the finding
// names no file, or its path leaves the project by its form (see the
// package doc, "Paths").
func (m *Modelo) ArchivoElegido() (ruta string, linea int) {
	return "", 0
}

// Registro is the Registro's entries, oldest first: every verb the TUI
// ran, every e, and every end of a job (see the package doc, "The
// Registro"). It lives while the model does.
func (m *Modelo) Registro() []Asiento {
	return nil
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

// vistaV1 is the current view.
func (m *Modelo) vistaV1() vistaV1 {
	return m.vista
}

// Abierta is the slug of the open card and whether its Historia tab is the
// active one ("" in the list).
func (m *Modelo) abiertaV1() (slug string, historia bool) {
	if m.abierta == "" {
		return "", false
	}
	return m.abierta, m.pestana == vistaHistoria
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
