package tablero

import (
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
	// job, which takes one line. It keeps tarjetacmd.CitarPedido's "…" in
	// both modes: in ASCII the screen writes it "~".
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
