package tablero

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/modelos"
	"github.com/hoomdev/hoomai/internal/profiles"
)

// UsageText is the usage block of `hoom tablero`.
const UsageText = `Uso: hoom tablero

Abre el tablero en la terminal, a pantalla completa y con paneles: el
proyecto, las tarjetas, lo que corre y los hallazgos a la izquierda, y a la
derecha el detalle de lo que elegis y el registro de lo que se ejecuto. Una
tecla por accion (? muestra todas) y cada accion ejecuta un verbo de la
tarjeta (hoom pedir, aprobar, guardar, integrar, reanudar, usar) despues de
mostrar en un modal el comando exacto. HOOM_MOUSE=0 apaga el mouse.
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

// SecMouseSi asks the terminal for the mouse: clicks and the wheel (1000),
// in the SGR format (1006). SecMouseNo gives it back, in the reverse order.
// Run writes them unless HOOM_MOUSE is "0" (ConMouse): see Run.
const (
	SecMouseSi = "\x1b[?1000h\x1b[?1006h"
	SecMouseNo = "\x1b[?1006l\x1b[?1000l"
)

// Intervalo is how often the board is read again, as the Studio polls it.
const Intervalo = 2 * time.Second

// IntervaloProyecto is how often [1] and [4] are read again (Proyecto and
// Hallazgos): hoom check computes the tree's fingerprint, which weighs in a
// big project.
const IntervaloProyecto = 10 * time.Second

// ParseArgs applies cliargs' discipline: a flag or an operand is a
// *cliargs.UsageError (exit 2); -h and --help are cliargs.ErrHelp.
func ParseArgs(args []string) error {
	fs := flag.NewFlagSet("tablero", flag.ContinueOnError)
	return cliargs.Strict(fs, args, "tablero", UsageText)
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

// Datos is where the TUI reads what it shows. The real one (DatosDe) reads
// the project of hoom tablero's directory with the functions of the CLI:
// hoom board --json, hoom status, hoom costo, hoom finding list --open and
// hoom board doctor.
type Datos interface {
	Tablero() (boardcmd.Board, error)
	Fijada() string // the card hoom usar fixed, "" = none
	// Detalle reads a card: its detail, its timeline and its narration.
	Detalle(slug string) Detalle
	// Proyecto reads what [1] and its [0] show.
	Proyecto() Proyecto
	// Hallazgos reads the project's open findings.
	Hallazgos() Hallazgos
	// Archivo reads the lines of ruta (relative to the project root, with
	// /) around linea, through the project's os.Root (see Archivo).
	Archivo(ruta string, linea int) Archivo
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
	// IntervaloProyecto between reads of Proyecto and Hallazgos; 0 =
	// IntervaloProyecto.
	IntervaloProyecto time.Duration
}

// Real is the Entorno of the hoom process.
func Real() Entorno {
	return Entorno{
		Dir:      ".",
		Out:      os.Stdout,
		Err:      os.Stderr,
		Getenv:   os.Getenv,
		GOOS:     runtime.GOOS,
		Terminal: terminalDelProceso(),
		Ahora:    time.Now,
	}
}

// Run is hoom tablero, and it returns its exit code. In order: the args
// (ParseArgs: 2 with the usage block on Err; -h prints UsageText on Out
// and is 0), the terminal (without one: 2 and MsgSinTerminal on Err, and
// nothing written anywhere else), the project's hoom.yaml
// (manifest.Load(Dir): an error, its models: included, is 1 with
// "hoom: <error>" on Err, before the terminal is touched).
//
// Then it takes the terminal: Crudo, and one write of SecEntrar followed by
// SecMouseSi (SecEntrar alone when ConMouse says no). When that write
// fails, the terminal is given back before Run returns 1 with
// "hoom: <error>" on Err. It gives the terminal back with one write of
// SecMouseNo followed by SecSalir (SecSalir alone without the mouse) and
// restaurar: when it ends with q or ctrl-c (0), with an error reading or
// writing the terminal, its end of file included (1, "hoom: <error>" on Err
// after giving it back), and with a panic, in Run or in what it calls
// (given back, then the panic goes on in Run's goroutine). After giving it
// back it prints MensajeSalida on Out when it is not empty. While the
// editor is open (AbrirEditor, EditarPedido) the terminal is given back the
// same way (the mouse off) and taken again after (the mouse asked again).
//
// The model gets Config{Proyecto, Ancho, Alto, Color: ConColor, UTF8:
// ConUTF8, Windows, Mouse: ConMouse, Editor: ComandoEditor, Modelos: the
// manifest's Models as modelos.Propio}. Run reads, each in another
// goroutine and never two of a kind at once:
//
//   - the board (Datos.Tablero and Fijada) at once and then every
//     Intervalo after the last read ended;
//   - Proyecto and Hallazgos at once, then every IntervaloProyecto after
//     the last read ended, and again when a process the TUI launched ends
//     (each FinProceso it gets);
//   - the Detalle of Modelo.Elegida when it changes and with every read of
//     the board, while it is not "";
//   - the Archivo of Modelo.ArchivoElegido when it changes, while it is
//     not "".
//
// It asks the terminal's size at least every 250 ms (and on SIGWINCH) and
// draws again when it changes. The bytes it reads go through
// DecodificarEntradas: the keys to Modelo.Tecla and the mouse events to
// Modelo.Raton. The bytes a read brings that end inside an escape sequence
// or inside a UTF-8 character wait a moment for the next read before they
// are decoded: a read may cut a key, a mouse event or a pasted text
// anywhere.
//
// An Abrir that fails puts "no se pudo abrir el editor: <error>" in red in
// the message line for DuracionAviso. An AbrirEditor whose Archivo,
// followed through its symlinks inside os.Root, leaves the project root (a
// symlink of the project to a file or a directory outside it) opens no
// editor: the message line says in red "el archivo <Archivo quoted as
// strconv.Quote> esta fuera del proyecto" for DuracionAviso. The check
// fails closed: when it cannot be made (the root does not open, a component
// cannot be read) no editor opens either, and the message line says in red
// "no se pudo abrir el editor: <error>"; a file that is not there yet,
// under components inside the root, opens. Nothing outside os.Root is
// looked at to decide.
func Run(args []string, e Entorno) int {
	if e.Out == nil {
		e.Out = io.Discard
	}
	if e.Err == nil {
		e.Err = io.Discard
	}
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if e.GOOS == "" {
		e.GOOS = runtime.GOOS
	}
	if e.Dir == "" {
		e.Dir = "."
	}
	if e.Ahora == nil {
		e.Ahora = time.Now
	}
	if e.Intervalo <= 0 {
		e.Intervalo = Intervalo
	}
	if e.IntervaloProyecto <= 0 {
		e.IntervaloProyecto = IntervaloProyecto
	}
	if err := ParseArgs(args); err != nil {
		if errors.Is(err, cliargs.ErrHelp) {
			fmt.Fprintln(e.Out, UsageText)
			return 0
		}
		var ue *cliargs.UsageError
		if errors.As(err, &ue) {
			fmt.Fprintln(e.Err, ue.Error())
			return ue.ExitCode()
		}
		fmt.Fprintln(e.Err, "hoom:", err)
		return 2
	}
	if e.Terminal == nil {
		fmt.Fprintln(e.Err, MsgSinTerminal)
		return 2
	}
	m, err := manifest.Load(e.Dir, profiles.Resolve)
	if err != nil {
		fmt.Fprintln(e.Err, "hoom:", err)
		return 1
	}
	if e.Datos == nil {
		e.Datos = DatosDe(m.Dir, m.BaseBranch, m.FindingsBlockOn())
	}
	if e.Lanzador == nil {
		hoom, err := os.Executable()
		if err != nil {
			fmt.Fprintln(e.Err, "hoom:", err)
			return 1
		}
		e.Lanzador = LanzadorDe(hoom, e.Dir, filepath.Join(m.Dir, ".hoom", "cache", "tablero"))
	}
	editor := ComandoEditor(e.Getenv, e.GOOS)
	if e.Editor == nil {
		e.Editor = EditorDe(editor, e.GOOS)
	}
	proyecto := m.Project
	if proyecto == "" {
		proyecto = filepath.Base(m.Dir)
	}
	var propios map[string][]modelos.Propio
	for p, es := range m.Models {
		if propios == nil {
			propios = map[string][]modelos.Propio{}
		}
		for _, x := range es {
			propios[p] = append(propios[p], modelos.Propio{Nombre: x.Name, Descripcion: x.Description})
		}
	}
	mouse := ConMouse(e.Getenv)
	b := &bucle{e: e, root: m.Dir, mouse: mouse, cfg: Config{
		Proyecto: proyecto,
		Color:    ConColor(e.Getenv),
		UTF8:     ConUTF8(e.Getenv),
		Windows:  e.GOOS == "windows",
		Mouse:    mouse,
		Editor:   editor,
		Modelos:  propios,
	}}
	return b.correr()
}

// ConColor says whether the screen has color: not when NO_COLOR is set to a
// value that is not empty (no-color.org).
func ConColor(getenv func(string) string) bool {
	return getenv("NO_COLOR") == ""
}

// ConMouse says whether Run asks the terminal for the mouse: not when
// HOOM_MOUSE is "0" (any other value, or none, is yes).
func ConMouse(getenv func(string) string) bool {
	return getenv("HOOM_MOUSE") != "0"
}

// ConUTF8 says whether the screen uses the UTF-8 signs: the locale (the
// first of LC_ALL, LC_CTYPE and LANG that is not empty) names UTF-8 ("UTF-8"
// or "utf8", any case), and HOOM_ASCII is not "1".
func ConUTF8(getenv func(string) string) bool {
	if getenv("HOOM_ASCII") == "1" {
		return false
	}
	for _, v := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if l := getenv(v); l != "" {
			l = strings.ToLower(l)
			return strings.Contains(l, "utf-8") || strings.Contains(l, "utf8")
		}
	}
	return false
}

// ComandoEditor is the person's editor: $VISUAL, then $EDITOR, then vi
// (notepad on Windows), as the capa-1 verbs choose it.
func ComandoEditor(getenv func(string) string, goos string) string {
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if ed := strings.TrimSpace(getenv(v)); ed != "" {
			return ed
		}
	}
	if goos == "windows" {
		return "notepad"
	}
	return "vi"
}

// ArgumentosEditor are the arguments that open archivo at linea, after the
// editor's command: "+<linea>" and archivo when the editor (the base name
// of the first word of editor, without ".exe") is vi, vim, nvim, nano,
// emacs, micro or hx; "-g" and "<archivo>:<linea>" when it is code or
// cursor; archivo alone with any other editor or with linea 0.
func ArgumentosEditor(editor, archivo string, linea int) []string {
	if linea <= 0 {
		return []string{archivo}
	}
	primera := editor
	if f := strings.Fields(editor); len(f) > 0 {
		primera = f[0]
	}
	// the base name on either separator: a Windows path read on Unix too
	if i := strings.LastIndexAny(primera, `/\`); i >= 0 {
		primera = primera[i+1:]
	}
	nombre := strings.TrimSuffix(strings.ToLower(primera), ".exe")
	switch nombre {
	case "vi", "vim", "nvim", "nano", "emacs", "micro", "hx":
		return []string{"+" + strconv.Itoa(linea), archivo}
	case "code", "cursor":
		return []string{"-g", archivo + ":" + strconv.Itoa(linea)}
	}
	return []string{archivo}
}

// LanzadorDe is the real Lanzador: it runs the binary hoom with the order's
// Args in dir, detached (a process group and a session of its own on Unix;
// a new process group without a console on Windows), with stdin empty and
// stdout and stderr to two new files in the directory logs (created when
// missing). Its FinProceso.Ultima comes from those files.
func LanzadorDe(hoom, dir, logs string) Lanzador {
	return &lanzadorReal{hoom: hoom, dir: dir, logs: logs}
}

// DatosDe is the real Datos of the project at root (base the base branch,
// blockOn the findings_open threshold):
//
//   - Tablero: boardcmd.Build; Fijada: the card in tarjetacmd.ArchivoFijada;
//   - Detalle: boardcmd.DetailFor (without a diff), boardcmd.TimelineFor and
//     the last MaxSobre bytes of the narration of the card's newest work row
//     (runcmd.UltimosEventos, from root or else from the card's evidence
//     dir); a narration that is in neither (a run of another machine) leaves
//     Sobre without events and without Err, any other failure to read it is
//     Sobre.Err;
//   - Proyecto: the branch of gitx, the check and the newest verdict of hoom
//     status's function, costcmd.HoyEn with the local clock, and
//     boardcmd.Doctor's problems; the first error is Err;
//   - Hallazgos: finding.List(root, base, true);
//   - Archivo: the file through os.OpenRoot(root), so a path or a symlink
//     that leaves the root reads nothing (Fuera); at most its first 4 MiB
//     are read.
func DatosDe(root, base, blockOn string) Datos {
	return datosReales{root: root, base: base, blockOn: blockOn}
}

// EditorDe is the real Editor of the editor command (ComandoEditor): it
// runs through the shell like the capa-1 verbs (sh -c on Unix, cmd /S /C
// on Windows), with ArgumentosEditor, on the process's stdin, stdout and
// stderr. On Windows (goos "windows") each argument goes between double
// quotes, and one with a double quote, a %, a ! or a control character is
// refused before anything runs: the error says "cmd no puede citar
// <argument quoted>".
func EditorDe(comando, goos string) Editor {
	return editorReal{comando: comando, goos: goos}
}
