// Package taskcmd implements parallel task isolation via git worktrees:
// hoomAI's adaptation of SwarmForge's worktree-per-role idea, reshaped to our
// unit of work. One task = one branch (hoom/<slug>) = one worktree under
// .hoom/worktrees/<slug> = one writer = its own verdict history. Two tasks
// can run in parallel with hard filesystem isolation, and a task can only be
// closed when its own `hoom check` is green: verdict + fingerprint, per
// worktree, no exceptions.
package taskcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/verdict"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func worktreeDir(root, slug string) string {
	return filepath.Join(root, ".hoom", "worktrees", slug)
}

// ensureIgnored guarantees .hoom/.gitignore hides worktrees/ so parallel
// checkouts never pollute the change candidate or the fingerprint.
func ensureIgnored(root string) error {
	_, err := hoomfs.EnsureIgnored(root, "worktrees")
	return err
}

// Start creates the branch hoom/<slug> and its isolated worktree.
func Start(root, slug, base string) error {
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug invalido %q: usa minusculas, numeros y guiones (ej: precios-por-region)", slug)
	}
	if err := ensureIgnored(root); err != nil {
		return err
	}
	wt := worktreeDir(root, slug)
	if _, err := os.Stat(wt); err == nil {
		return fmt.Errorf("la tarea %q ya existe (%s); usa 'hoom task list' o elegi otro slug", slug, wt)
	}
	branch := "hoom/" + slug
	ref := base
	if _, err := run(root, "rev-parse", "--verify", ref); err != nil {
		ref = "HEAD" // base branch may not exist yet in young repos
	}
	if out, err := run(root, "worktree", "add", "-b", branch, wt, ref); err != nil {
		return fmt.Errorf("git worktree add fallo: %s", out)
	}
	fmt.Printf("hoom: tarea %q creada\n", slug)
	fmt.Printf("  rama:     %s (desde %s)\n", branch, ref)
	fmt.Printf("  worktree: %s\n", wt)
	fmt.Println("  siguiente paso:")
	fmt.Printf("    cd %s\n", wt)
	fmt.Println("    (abri tu CLI ahi; UN writer por tarea; hoom verify corre aislado en este worktree)")
	fmt.Println("  cierre: commitea TODO (codigo + veredictos) y ejecuta 'hoom task done " + slug + "' desde el proyecto principal")
	return nil
}

// Task states, machine-readable. The human rendering derives from these.
const (
	StateGreen     = "green"
	StateDrift     = "drift"
	StateRed       = "red"
	StateNoVerdict = "no-verdict"
)

// TaskInfo is one task's state, shared verbatim by `hoom task list --json`
// and by the Studio's /api/tasks (one brain, several skins).
type TaskInfo struct {
	Slug      string `json:"slug"`
	Branch    string `json:"branch"`
	State     string `json:"state"` // green | drift | red | no-verdict
	VerdictID string `json:"verdict_id,omitempty"`
	Dirty     bool   `json:"dirty"`
}

// Snapshot collects every task worktree with its verdict state. No tasks
// yields an empty (non-nil) slice so JSON renders as [] and never null.
func Snapshot(root, base string) ([]TaskInfo, error) {
	out := []TaskInfo{}
	dir := filepath.Join(root, ".hoom", "worktrees")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wt := filepath.Join(dir, e.Name())
		ti := TaskInfo{Slug: e.Name()}
		ti.Branch, _ = run(wt, "rev-parse", "--abbrev-ref", "HEAD")
		if st, _ := run(wt, "status", "--porcelain"); st != "" {
			ti.Dirty = true
		}
		ti.State, ti.VerdictID = taskState(wt, base)
		out = append(out, ti)
	}
	return out, nil
}

// JSONBytes renders the snapshot exactly as both the CLI and the Studio
// emit it, so the two representations cannot diverge.
func JSONBytes(root, base string) ([]byte, error) {
	tasks, err := Snapshot(root, base)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(tasks, "", "  ")
}

// List shows every task worktree with its verdict state.
func List(root, base string) error {
	tasks, err := Snapshot(root, base)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("hoom: sin tareas activas (crea una con 'hoom task start <slug>')")
		return nil
	}
	fmt.Println("hoom: tareas activas")
	for _, t := range tasks {
		fmt.Printf("  %-24s %-12s %s\n", t.Slug, t.Branch, humanState(t))
	}
	return nil
}

func humanState(t TaskInfo) string {
	switch t.State {
	case StateRed:
		return "ROJO (" + t.VerdictID + ")"
	case StateDrift:
		return "VERDE con drift (re-ejecuta hoom verify)"
	case StateGreen:
		return "VERDE listo para cerrar (" + t.VerdictID + ")"
	default:
		return "SIN-VEREDICTO (corre hoom verify dentro del worktree)"
	}
}

func taskState(wt, base string) (state, verdictID string) {
	all, err := verdict.LoadAll(wt)
	if err != nil || len(all) == 0 {
		return StateNoVerdict, ""
	}
	// Same reference rule as hoom check: --gate diagnostics never count.
	last := verdict.LatestComplete(all)
	if last == nil {
		return StateNoVerdict, ""
	}
	if last.Verdict != "green" {
		return StateRed, last.ID
	}
	g := gitx.Snapshot(wt, base)
	if g.ChangeFingerprint != last.Git.ChangeFingerprint {
		return StateDrift, last.ID
	}
	return StateGreen, last.ID
}

// Done closes a task: requires a clean tree (code AND verdicts committed) and
// a green verdict whose fingerprint matches, then records the close on the
// task's item (when this tree has one) and removes the worktree, leaving the
// branch ready to merge. --force skips the checks and force-removes, and
// never marks the item done: a card does not earn Hecho without evidence.
func Done(root, slug, base string, force bool) error {
	wt := worktreeDir(root, slug)
	if _, err := os.Stat(wt); err != nil {
		return fmt.Errorf("la tarea %q no existe (mira 'hoom task list')", slug)
	}
	branch := "hoom/" + slug
	if !force {
		if err := Ready(root, slug, base); err != nil {
			return err
		}
	}

	// The item is read BEFORE touching the worktree: an invalid item stops
	// the close with everything still in place.
	var (
		itemPath  = item.Path(root, slug)
		original  []byte
		itemNote  string
		wroteItem bool
	)
	switch raw, err := os.ReadFile(itemPath); {
	case os.IsNotExist(err):
		itemNote = fmt.Sprintf("hoom: sin item %s en este arbol: no hay tarjeta que cerrar", item.RelPath(slug))
	case err != nil:
		return fmt.Errorf("no pude leer el item %s: %v", item.RelPath(slug), err)
	default:
		it, perr := item.Parse(slug, raw)
		switch {
		case perr != nil && !force:
			return fmt.Errorf("el item %s es invalido: %v.\n  Accion: reparalo y repite 'hoom task done %s' (o cerra con --force, que no lo marca hecho)", item.RelPath(slug), perr, slug)
		case force:
			itemNote = fmt.Sprintf("hoom: con --force el item %s no se marca hecho", slug)
		case it.HechoEn != nil:
			itemNote = fmt.Sprintf("hoom: el item %s ya estaba hecho (%s): no se reescribe", slug, it.HechoEn.Format("2006-01-02 15:04 UTC"))
		default:
			commit, cerr := run(root, "rev-parse", branch)
			if cerr != nil {
				return fmt.Errorf("no pude leer la punta de %s: %s", branch, commit)
			}
			original = raw
			if _, err := item.MarkDone(root, slug, commit, time.Now()); err != nil {
				return fmt.Errorf("no pude registrar el cierre en %s: %v", item.RelPath(slug), err)
			}
			wroteItem = true
			itemNote = fmt.Sprintf("hoom: item %s hecho (commit_final %s) - commitea %s", slug, short(commit), item.RelPath(slug))
		}
	}

	args := []string{"worktree", "remove", wt}
	if force {
		args = []string{"worktree", "remove", "--force", wt}
	}
	if out, err := run(root, args...); err != nil {
		if wroteItem {
			// the task did not close, so the card did not either
			hoomfs.AtomicWrite(itemPath, original, 0o644)
		}
		return fmt.Errorf("git worktree remove fallo: %s", out)
	}
	if force {
		fmt.Printf("hoom: tarea %q cerrada con --force (sin verificar)\n", slug)
	} else {
		fmt.Printf("hoom: tarea %q cerrada con veredicto verde y huella coincidente\n", slug)
	}
	fmt.Println(itemNote)
	fmt.Printf("  la rama %s queda lista para integrar:\n", branch)
	fmt.Printf("    git merge --no-ff %s\n", branch)
	fmt.Printf("    git branch -d %s   (despues del merge)\n", branch)
	return nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// Ready is the check `hoom task done` makes without --force — the task
// exists, its tree is clean, it has a complete verdict, the latest complete
// one is green and its fingerprint matches — with the same messages. nil
// means task done would close it.
func Ready(root, slug, base string) error {
	wt := worktreeDir(root, slug)
	if _, err := os.Stat(wt); err != nil {
		return fmt.Errorf("la tarea %q no existe (mira 'hoom task list')", slug)
	}
	if st, _ := run(wt, "status", "--porcelain"); st != "" {
		return fmt.Errorf("la tarea %q tiene cambios sin commitear (incluidos posibles veredictos).\n  Accion: commitea todo dentro de %s y repite 'hoom task done %s'", slug, wt, slug)
	}
	all, err := verdict.LoadAll(wt)
	if err != nil || len(all) == 0 {
		return fmt.Errorf("la tarea %q no tiene veredictos. Accion: ejecuta 'hoom verify' dentro del worktree", slug)
	}
	last := verdict.LatestComplete(all)
	if last == nil {
		return fmt.Errorf("la tarea %q solo tiene veredictos PARCIALES (--gate), que no son referencia. Accion: ejecuta 'hoom verify' completo dentro del worktree", slug)
	}
	if last.Verdict != "green" {
		return fmt.Errorf("el ultimo veredicto de %q es ROJO (%s). Accion: corrige y re-ejecuta 'hoom verify' en el worktree", slug, last.ID)
	}
	g := gitx.Snapshot(wt, base)
	if g.ChangeFingerprint != last.Git.ChangeFingerprint {
		return fmt.Errorf("el arbol de %q cambio despues del ultimo veredicto verde (huella %s vs %s).\n  Accion: re-ejecuta 'hoom verify' dentro del worktree y commitea", slug, g.ChangeFingerprint, last.Git.ChangeFingerprint)
	}
	return nil
}
