package tablero

import (
	"io"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// UsageText is the usage block of `hoom tablero`.
const UsageText = `Uso: hoom tablero

Abre el tablero en la terminal, a pantalla completa: donde esta cada
tarjeta, que le falta, que esta corriendo y que se puede hacer. Una tecla
por accion (? muestra las teclas) y cada accion ejecuta un verbo de la
tarjeta (hoom pedir, siguiente, aprobar, guardar, integrar, reanudar, usar).
Necesita una terminal; sin terminal usa hoom board.
'hoom tablero' no tiene flags ni argumentos.`

// MsgSinTerminal is the exit-2 message without a terminal on stdin and
// stdout.
const MsgSinTerminal = "hoom tablero necesita una terminal; sin terminal usa hoom board"

// The sequences the TUI writes to take the terminal and to give it back:
// SecEntrar enters the alternate screen and hides the cursor; SecSalir
// clears the screen (a terminal without an alternate screen is left clean),
// shows the cursor and leaves the alternate screen.
const (
	SecEntrar = "\x1b[?1049h\x1b[?25l"
	SecSalir  = "\x1b[2J\x1b[H\x1b[?25h\x1b[?1049l"
)

// Intervalo is how often the board is read again, as the Studio polls it.
const Intervalo = 2 * time.Second

// ParseArgs applies cliargs' discipline: a flag or an operand is a
// *cliargs.UsageError (exit 2); -h and --help are cliargs.ErrHelp.
func ParseArgs(args []string) error {
	return nil
}

// Terminal is the terminal the TUI takes. The real one is the process's
// stdin and stdout (golang.org/x/term); tests give their own.
type Terminal interface {
	io.Reader // the keys, as raw bytes
	io.Writer // the screen
	// Crudo puts the terminal in raw mode and returns how to put it back.
	Crudo() (restaurar func() error, err error)
	// Tamano is its size in cells.
	Tamano() (ancho, alto int, err error)
}

// Datos is where the TUI reads the board and the open card. The real one
// reads the project of hoom tablero's directory with the same functions as
// hoom board --json and the Studio.
type Datos interface {
	Tablero() (boardcmd.Board, error)
	Fijada() string // the card hoom usar fixed, "" = none
	// Detalle reads the open card; the Historia only when historia.
	Detalle(slug string, historia bool) Detalle
}

// Lanzador starts the processes the TUI launches. Lanzar never waits: it
// starts the process and calls fin, once, from another goroutine, when the
// process ends. An error means it could not start (fin is not called).
type Lanzador interface {
	Lanzar(o Lanzar, fin func(FinProceso)) error
}

// Editor opens the person's editor. Run releases the screen while it is
// open.
type Editor interface {
	// Abrir opens archivo (absolute) at linea (0 = no line).
	Abrir(archivo string, linea int) error
	// Editar opens texto in a temporary file and returns what it holds when
	// the editor ends.
	Editar(texto string) (string, error)
}

// Entorno is the process boundary of hoom tablero. A nil field is the real
// one; tests replace what they need.
type Entorno struct {
	Dir    string    // where hoom tablero runs (and the verbs it launches)
	Out    io.Writer // the usage (-h) and the exit message
	Err    io.Writer // errors
	Getenv func(string) string
	GOOS   string // "" = the one hoom runs on
	// Terminal is nil when stdin or stdout is not a terminal.
	Terminal Terminal
	Datos    Datos    // nil = the project of Dir
	Lanzador Lanzador // nil = hoom itself, detached
	Editor   Editor   // nil = $VISUAL, $EDITOR or vi
	Ahora    func() time.Time
	// Intervalo between reads of the board; 0 = Intervalo.
	Intervalo time.Duration
}

// Real is the Entorno of the hoom process.
func Real() Entorno {
	return Entorno{}
}

// Run is hoom tablero, and it returns its exit code. In order: the args
// (ParseArgs: 2 with the usage block on Err; -h prints UsageText on Out
// and is 0), the terminal (without one: 2 and MsgSinTerminal on Err, and
// nothing written anywhere else), the project's hoom.yaml
// (manifest.Load(Dir): an error is 1 with "hoom: <error>" on Err, before
// the terminal is touched). Then it takes the terminal (Crudo, SecEntrar),
// draws, reads the board every Intervalo, and gives the terminal back
// (SecSalir, restaurar) when it ends: with q or ctrl-c (0), with an error
// of the terminal (1, the error on Err after giving it back) and with a
// panic (given back, then the panic goes on). After giving it back it
// prints MensajeSalida on Out when it is not empty.
func Run(args []string, e Entorno) int {
	return 0
}

// ConColor says whether the screen has color: not when NO_COLOR is set to a
// value that is not empty (no-color.org).
func ConColor(getenv func(string) string) bool {
	return false
}

// ConUTF8 says whether the screen uses the UTF-8 signs: the locale (the
// first of LC_ALL, LC_CTYPE and LANG that is not empty) names UTF-8 ("UTF-8"
// or "utf8", any case), and HOOM_ASCII is not "1".
func ConUTF8(getenv func(string) string) bool {
	return false
}

// ComandoEditor is the person's editor: $VISUAL, then $EDITOR, then vi
// (notepad on Windows), as the capa-1 verbs choose it.
func ComandoEditor(getenv func(string) string, goos string) string {
	return ""
}

// ArgumentosEditor are the arguments that open archivo at linea, after the
// editor's command: "+<linea>" and archivo when the editor (the base name
// of the first word of editor, without ".exe") is vi, vim, nvim, nano,
// emacs, micro or hx; "-g" and "<archivo>:<linea>" when it is code or
// cursor; archivo alone with any other editor or with linea 0.
func ArgumentosEditor(editor, archivo string, linea int) []string {
	return nil
}

// LanzadorDe is the real Lanzador: it runs the binary hoom with the order's
// Args in dir, detached (a process group and a session of its own on Unix;
// a new process group without a console on Windows), with stdin empty and
// stdout and stderr to two new files in the directory logs (created when
// missing). Its FinProceso.Ultima comes from those files.
func LanzadorDe(hoom, dir, logs string) Lanzador {
	return nil
}

// DatosDe is the real Datos of the project at root: the board of
// boardcmd.Build, the card fixed in tarjetacmd.ArchivoFijada, the detail of
// boardcmd.DetailFor (without a diff), the timeline of
// boardcmd.TimelineFor and the last MaxSobre bytes of the narration of the
// card's newest work row (runcmd.UltimosEventos, from root or else from the
// card's evidence dir).
func DatosDe(root, base, blockOn string) Datos {
	return nil
}

// EditorDe is the real Editor of the editor command (ComandoEditor): it
// runs through the shell like the capa-1 verbs (sh -c on Unix, cmd /C on
// Windows), with ArgumentosEditor, on the process's stdin, stdout and
// stderr.
func EditorDe(comando, goos string) Editor {
	return nil
}
