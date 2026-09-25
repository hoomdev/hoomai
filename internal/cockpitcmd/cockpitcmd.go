// Package cockpitcmd assembles the operator's post in ONE command: the
// user's AI CLI in a real terminal pane and `hoom status --watch` beside it.
// hoom does not emulate terminals nor reinvent multiplexers — it detects
// tmux or zellij (same PATH-detection pattern as providers) and composes the
// session; the emulation is theirs, which is why ANY AI CLI runs intact.
// The cockpit launches and shows; it never directs the AI: orchestration
// stays in the CLI under its role contracts, and hoom stays the referee.
package cockpitcmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
)

// Options mirror the cockpit verb's flags.
type Options struct {
	Provider string // AI CLI from the providers registry; "" = autodetect
	Task     string // task slug: mount the cockpit inside its worktree
	Mux      string // tmux | zellij; "" = autodetect (tmux first)
}

// Deps is the process boundary, injected so tests verify the exact plan
// against recorders and fake binaries instead of a real multiplexer.
type Deps struct {
	LookPath func(file string) (string, error)
	// RunCmd runs a step in foreground with the terminal's streams.
	RunCmd func(dir, name string, args ...string) error
	// QuietCmd runs a probe discarding output (e.g. tmux has-session).
	QuietCmd func(dir, name string, args ...string) error
	Getenv   func(key string) string
	// Output runs a probe and returns its stdout (tmux list-panes,
	// capture-pane): the terminal mirror reads through it.
	Output  func(dir, name string, args ...string) ([]byte, error)
	HoomBin string // absolute path of the RUNNING hoom binary (CA-88)
}

// DefaultDeps wires the real process boundary.
func DefaultDeps() Deps {
	bin, err := os.Executable()
	if err != nil {
		bin = "hoom" // degraded: PATH resolution, better than nothing
	}
	return Deps{
		LookPath: exec.LookPath,
		RunCmd: func(dir, name string, args ...string) error {
			cmd := exec.Command(name, args...)
			cmd.Dir = dir
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return cmd.Run()
		},
		QuietCmd: func(dir, name string, args ...string) error {
			cmd := exec.Command(name, args...)
			cmd.Dir = dir
			return cmd.Run()
		},
		Output: func(dir, name string, args ...string) ([]byte, error) {
			cmd := exec.Command(name, args...)
			cmd.Dir = dir
			return cmd.Output()
		},
		Getenv:  os.Getenv,
		HoomBin: bin,
	}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = slugRe.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-")
}

// Run assembles and attaches the cockpit for the project rooted at root.
// With tmux it is Open plus the attach; zellij composes and attaches in one
// command, so it does not go through Open.
func Run(root, project string, opt Options, deps Deps) error {
	mux, err := resolveMux(opt.Mux, deps)
	if err != nil {
		return err
	}
	if mux == "zellij" {
		_, bin, err := resolveProvider(opt.Provider, deps)
		if err != nil {
			return err
		}
		dir, session, _, err := placeOf(root, project, opt.Task)
		if err != nil {
			return err
		}
		return runZellij(root, dir, session, bin, deps)
	}
	s, err := Open(root, project, Options{Provider: opt.Provider, Task: opt.Task, Mux: "tmux"}, deps)
	if err != nil {
		return err
	}
	dir, _, _, err := placeOf(root, project, opt.Task)
	if err != nil {
		return err
	}
	if deps.Getenv("TMUX") != "" {
		return deps.RunCmd(dir, "tmux", "switch-client", "-t", s.Name)
	}
	return deps.RunCmd(dir, "tmux", "attach-session", "-t", s.Name)
}

// placeOf is where the cockpit of a project (or of one of its tasks) lives:
// its directory, its session name and the directory relative to root.
func placeOf(root, project, task string) (dir, session, rel string, err error) {
	dir, rel = root, "."
	if task != "" {
		wt := filepath.Join(root, ".hoom", "worktrees", task)
		if st, err := os.Stat(wt); err != nil || !st.IsDir() {
			return "", "", "", fmt.Errorf("la tarea %q no existe. Accion: crea su worktree con 'hoom task start %s'", task, task)
		}
		dir, rel = wt, ".hoom/worktrees/"+task
	}
	return dir, SessionName(project, task), rel, nil
}

// resolveMux picks the multiplexer: an explicit --mux that cannot be honored
// is an error, never a silent fallback.
func resolveMux(mux string, deps Deps) (string, error) {
	switch mux {
	case "":
		if _, err := deps.LookPath("tmux"); err == nil {
			return "tmux", nil
		}
		if _, err := deps.LookPath("zellij"); err == nil {
			return "zellij", nil
		}
		return "", fmt.Errorf("no se encontro tmux ni zellij en PATH.\n  Accion: instala uno (ej. brew install tmux) o abre una segunda terminal con 'hoom status --watch'")
	case "tmux", "zellij":
		if _, err := deps.LookPath(mux); err != nil {
			return "", fmt.Errorf("--mux %s: no esta instalado (no se encontro %q en PATH); instalalo o quita el flag", mux, mux)
		}
		return mux, nil
	default:
		return "", fmt.Errorf("--mux invalido %q (validos: tmux, zellij)", mux)
	}
}

// resolveProvider returns the AI CLI to launch, by name and binary. Without
// --provider, exactly one installed CLI decides; zero or several NEVER guess.
func resolveProvider(name string, deps Deps) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		var installed []string
		for _, p := range providers.Detect() {
			if p.Installed {
				installed = append(installed, p.Name)
			}
		}
		switch len(installed) {
		case 0:
			return "", "", fmt.Errorf("ninguna CLI de IA instalada (mira 'hoom providers').\n  Accion: instala una, o abre tu herramienta a mano junto a 'hoom status --watch'")
		case 1:
			name = installed[0]
		default:
			return "", "", fmt.Errorf("varias CLIs instaladas (%s): elegi una con --provider", strings.Join(installed, ", "))
		}
	}
	p, err := providers.Lookup(name)
	if err != nil {
		return "", "", err
	}
	if _, err := deps.LookPath(p.Bin()); err != nil {
		return "", "", fmt.Errorf("el provider %q no esta instalado (no se encontro %q en PATH); instala su CLI primero", name, p.Bin())
	}
	return p.Name(), p.Bin(), nil
}

// composeTmux composes the tmux session if it does not exist yet: AI pane
// ~70%, watch pane beside it, both rooted at dir. Idempotent by session name;
// it reports whether it created the session. It never attaches.
func composeTmux(dir, session, aiBin string, deps Deps) (bool, error) {
	if deps.QuietCmd(dir, "tmux", "has-session", "-t", session) == nil {
		return false, nil
	}
	if err := deps.RunCmd(dir, "tmux", "new-session", "-d", "-s", session, "-c", dir, aiBin); err != nil {
		return false, err
	}
	if err := deps.RunCmd(dir, "tmux", "split-window", "-h", "-l", "30%", "-t", session, "-c", dir, watchCommand(deps)); err != nil {
		// a half-built cockpit would be "found" by the next try and never
		// completed: it goes, and the next try builds it whole
		return false, killSession(dir, session, deps, err)
	}
	// foco inicial en el pane de la IA; cosmetico, jamas fatal
	_ = deps.QuietCmd(dir, "tmux", "select-pane", "-t", session, "-L")
	return true, nil
}

// killSession closes exactly session (= : no prefix match on another one)
// after cause went wrong, and returns cause saying what happened to the
// session: closed, so the next try starts over, or still open, with the
// command that closes it (a promise it cannot keep would hide a cockpit the
// next try never completes).
func killSession(dir, session string, deps Deps, cause error) error {
	if err := deps.QuietCmd(dir, "tmux", "kill-session", "-t", "="+session); err != nil {
		return fmt.Errorf("%w; la sesion %s quedo abierta y no pude cerrarla (%v): cerrala con 'tmux kill-session -t =%s' y reintenta", cause, session, err, session)
	}
	return fmt.Errorf("%w; cerre la sesion %s para que el proximo intento la arme entera", cause, session)
}

// runZellij writes the KDL layout under .hoom/cache/ (the only place the
// cockpit writes) and launches the session; an existing session re-attaches.
func runZellij(root, dir, session, aiBin string, deps Deps) error {
	layout := filepath.Join(root, ".hoom", "cache", "cockpit-"+session+".kdl")
	if err := os.MkdirAll(filepath.Dir(layout), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(layout, []byte(kdlLayout(aiBin, deps.HoomBin, dir)), 0o644); err != nil {
		return err
	}
	if err := deps.RunCmd(dir, "zellij", "--session", session, "--new-session-with-layout", layout); err != nil {
		return deps.RunCmd(dir, "zellij", "attach", session)
	}
	return nil
}

// watchCommand builds the watch pane's shell command with the ABSOLUTE path
// of the running hoom binary: the session's PATH must not decide (CA-88).
func watchCommand(deps Deps) string {
	return "'" + deps.HoomBin + "' status --watch"
}

func kdlLayout(aiBin, hoomBin, dir string) string {
	return fmt.Sprintf(`layout {
    pane split_direction="vertical" {
        pane command=%q size="70%%" cwd=%q
        pane command=%q cwd=%q {
            args "status" "--watch"
        }
    }
}
`, aiBin, dir, hoomBin, dir)
}

// TerminalMaxBytes caps the text of the terminal mirror.
const TerminalMaxBytes = 256 << 10

// Session is the cockpit session Open composed or found.
type Session struct {
	Name     string `json:"session"`
	Created  bool   `json:"created"`
	Provider string `json:"provider"`
	Dir      string `json:"dir"`    // relative to root ("." = the project)
	Attach   string `json:"attach"` // the command a person runs to attach
}

// Terminal is the read-only mirror of the AI pane of a card's session.
type Terminal struct {
	Available bool   `json:"available"`
	Session   string `json:"session"`
	Pane      string `json:"pane"`
	Text      string `json:"text"`
	Note      string `json:"note"`
}

// SessionName is the cockpit session of a project, or of one of its tasks.
func SessionName(project, task string) string {
	name := "hoom-" + slugify(project)
	if task != "" {
		name += "-" + slugify(task)
	}
	return name
}

// Open composes the tmux cockpit (the plan of Run) WITHOUT attaching, and
// says whether it created the session or found it. Creating the session of
// a task that has an item in root appends the session to the item.
func Open(root, project string, opt Options, deps Deps) (Session, error) {
	if opt.Mux != "" && opt.Mux != "tmux" {
		return Session{}, fmt.Errorf("--mux %s: abrir la sesion sin adjuntarse usa tmux; con %s usa 'hoom cockpit --mux %s' desde una terminal", opt.Mux, opt.Mux, opt.Mux)
	}
	if _, err := deps.LookPath("tmux"); err != nil {
		return Session{}, fmt.Errorf("no se encontro tmux en PATH.\n  Accion: instala tmux (ej. brew install tmux) o abre una segunda terminal con 'hoom status --watch'")
	}
	name, bin, err := resolveProvider(opt.Provider, deps)
	if err != nil {
		return Session{}, err
	}
	dir, session, rel, err := placeOf(root, project, opt.Task)
	if err != nil {
		return Session{}, err
	}
	created, err := composeTmux(dir, session, bin, deps)
	s := Session{Name: session, Created: created, Provider: name, Dir: rel, Attach: "tmux attach -t " + session}
	if err != nil {
		return s, err
	}
	// El writer DECLARADO: quien abre una sesion interactiva en la tarea lo
	// deja dicho en su item. Solo al crearla: reabrir no es otra sesion.
	if created && opt.Task != "" {
		if _, err := item.AddSession(root, opt.Task, name, time.Now()); err != nil {
			// a session nobody declared would be "found" by the next try,
			// which never registers it: it goes, and the next try does both
			return Session{Name: session, Provider: name, Dir: rel}, killSession(dir, session, deps,
				fmt.Errorf("no pude registrar la sesion %s en %s: %v", session, item.RelPath(opt.Task), err))
		}
	}
	return s, nil
}

// Capture reads the AI pane of the card's session: what tmux already
// painted, colors included. It never writes and never sends keys.
func Capture(root, project, slug string, deps Deps) (Terminal, error) {
	t := Terminal{Session: SessionName(project, slug)}
	if _, err := deps.LookPath("tmux"); err != nil {
		t.Note = "tmux no esta instalado"
		return t, nil
	}
	if deps.QuietCmd(root, "tmux", "has-session", "-t", "="+t.Session) != nil {
		t.Note = "no hay una sesion abierta para esta tarjeta"
		return t, nil
	}
	out, err := deps.Output(root, "tmux", "list-panes", "-t", "="+t.Session, "-F", "#{pane_id}")
	if err != nil {
		t.Note = "no hay una sesion abierta para esta tarjeta"
		return t, nil
	}
	panes := strings.Fields(string(out))
	if len(panes) == 0 {
		t.Note = "no hay una sesion abierta para esta tarjeta"
		return t, nil
	}
	t.Pane = panes[0] // el del CLI de la IA: el cockpit lo crea primero
	text, err := deps.Output(root, "tmux", "capture-pane", "-p", "-e", "-t", t.Pane)
	if err != nil {
		return t, fmt.Errorf("tmux capture-pane fallo: %v", err)
	}
	t.Text, t.Available = string(clipTerminal(text, TerminalMaxBytes)), true
	return t, nil
}

// clipTerminal cuts what tmux painted to at most max bytes without leaving
// half a character or half an escape sequence at the end: the mirror only
// keeps what it can draw whole.
func clipTerminal(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	b = b[:max]
	i := len(b) - 1
	for i > 0 && len(b)-i < utf8.UTFMax && !utf8.RuneStart(b[i]) {
		i--
	}
	if !utf8.FullRune(b[i:]) {
		b = b[:i]
	}
	// the last sequence still open goes whole, and again if cutting it
	// opened the one before (the ESC of an ESC \ that ends an OSC)
	for {
		i := bytes.LastIndexByte(b, 0x1b)
		if i < 0 || escapeEnds(b[i:]) {
			return b
		}
		b = b[:i]
	}
}

// escapeEnds says whether the escape sequence seq starts with is whole: a
// CSI (ESC [) ends on a byte 0x40-0x7E, a string (OSC, DCS, APC, PM, SOS) on
// BEL (its ESC \ is an escape of its own), any other on a byte 0x30-0x7E.
func escapeEnds(seq []byte) bool {
	if len(seq) < 2 {
		return false
	}
	switch seq[1] {
	case '[':
		return bytes.IndexFunc(seq[2:], func(r rune) bool { return r >= 0x40 && r <= 0x7e }) >= 0
	case ']', 'P', '_', '^', 'X':
		return bytes.IndexByte(seq[2:], 0x07) >= 0
	}
	return bytes.IndexFunc(seq[1:], func(r rune) bool { return r >= 0x30 && r <= 0x7e }) >= 0
}
