// Package cockpitcmd assembles the operator's post in ONE command: the
// user's AI CLI in a real terminal pane and `hoom status --watch` beside it.
// hoom does not emulate terminals nor reinvent multiplexers — it detects
// tmux or zellij (same PATH-detection pattern as providers) and composes the
// session; the emulation is theirs, which is why ANY AI CLI runs intact.
// The cockpit launches and shows; it never directs the AI: orchestration
// stays in the CLI under its role contracts, and hoom stays the referee.
package cockpitcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

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
	HoomBin  string // absolute path of the RUNNING hoom binary (CA-88)
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
func Run(root, project string, opt Options, deps Deps) error {
	mux, err := resolveMux(opt.Mux, deps)
	if err != nil {
		return err
	}
	bin, err := resolveProvider(opt.Provider, deps)
	if err != nil {
		return err
	}

	dir, session := root, "hoom-"+slugify(project)
	if opt.Task != "" {
		wt := filepath.Join(root, ".hoom", "worktrees", opt.Task)
		if st, err := os.Stat(wt); err != nil || !st.IsDir() {
			return fmt.Errorf("la tarea %q no existe. Accion: crea su worktree con 'hoom task start %s'", opt.Task, opt.Task)
		}
		dir = wt
		session += "-" + slugify(opt.Task)
	}

	if mux == "zellij" {
		return runZellij(root, dir, session, bin, deps)
	}
	return runTmux(dir, session, bin, deps)
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

// resolveProvider returns the AI CLI binary to launch. Without --provider,
// exactly one installed CLI decides; zero or several NEVER guess.
func resolveProvider(name string, deps Deps) (string, error) {
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
			return "", fmt.Errorf("ninguna CLI de IA instalada (mira 'hoom providers').\n  Accion: instala una, o abre tu herramienta a mano junto a 'hoom status --watch'")
		case 1:
			name = installed[0]
		default:
			return "", fmt.Errorf("varias CLIs instaladas (%s): elegi una con --provider", strings.Join(installed, ", "))
		}
	}
	p, err := providers.Lookup(name)
	if err != nil {
		return "", err
	}
	if _, err := deps.LookPath(p.Bin()); err != nil {
		return "", fmt.Errorf("el provider %q no esta instalado (no se encontro %q en PATH); instala su CLI primero", name, p.Bin())
	}
	return p.Bin(), nil
}

// runTmux composes (or re-attaches) the tmux session: AI pane ~70%, watch
// pane beside it, both rooted at dir. Idempotent by session name.
func runTmux(dir, session, aiBin string, deps Deps) error {
	exists := deps.QuietCmd(dir, "tmux", "has-session", "-t", session) == nil
	if !exists {
		if err := deps.RunCmd(dir, "tmux", "new-session", "-d", "-s", session, "-c", dir, aiBin); err != nil {
			return err
		}
		if err := deps.RunCmd(dir, "tmux", "split-window", "-h", "-l", "30%", "-t", session, "-c", dir, watchCommand(deps)); err != nil {
			return err
		}
		// foco inicial en el pane de la IA; cosmetico, jamas fatal
		_ = deps.QuietCmd(dir, "tmux", "select-pane", "-t", session, "-L")
	}
	if deps.Getenv("TMUX") != "" {
		return deps.RunCmd(dir, "tmux", "switch-client", "-t", session)
	}
	return deps.RunCmd(dir, "tmux", "attach-session", "-t", session)
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
