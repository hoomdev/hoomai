// Package taskcmd implements parallel task isolation via git worktrees:
// hoomAI's adaptation of SwarmForge's worktree-per-role idea, reshaped to our
// unit of work. One task = one branch (hoom/<slug>) = one worktree under
// .hoom/worktrees/<slug> = one writer = its own verdict history. Two tasks
// can run in parallel with hard filesystem isolation, and a task can only be
// closed when its own `hoom check` is green: verdict + fingerprint, per
// worktree, no exceptions.
package taskcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hoomdev/hoomai/internal/gitx"
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
	gi := filepath.Join(root, ".hoom", ".gitignore")
	raw, _ := os.ReadFile(gi)
	if strings.Contains(string(raw), "worktrees/") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		return err
	}
	content := string(raw)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(gi, []byte(content+"worktrees/\n"), 0o644)
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

// List shows every task worktree with its verdict state.
func List(root, base string) error {
	dir := filepath.Join(root, ".hoom", "worktrees")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) || len(entries) == 0 {
		fmt.Println("hoom: sin tareas activas (crea una con 'hoom task start <slug>')")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Println("hoom: tareas activas")
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wt := filepath.Join(dir, e.Name())
		state := taskState(wt, base)
		branch, _ := run(wt, "rev-parse", "--abbrev-ref", "HEAD")
		fmt.Printf("  %-24s %-12s %s\n", e.Name(), branch, state)
	}
	return nil
}

func taskState(wt, base string) string {
	all, err := verdict.LoadAll(wt)
	if err != nil || len(all) == 0 {
		return "SIN-VEREDICTO (corre hoom verify dentro del worktree)"
	}
	last := all[len(all)-1]
	if last.Verdict != "green" {
		return "ROJO (" + last.ID + ")"
	}
	g := gitx.Snapshot(wt, base)
	if g.ChangeFingerprint != last.Git.ChangeFingerprint {
		return "VERDE con drift (re-ejecuta hoom verify)"
	}
	return "VERDE listo para cerrar (" + last.ID + ")"
}

// Done closes a task: requires a clean tree (code AND verdicts committed) and
// a green verdict whose fingerprint matches, then removes the worktree and
// leaves the branch ready to merge. --force skips the checks and force-removes.
func Done(root, slug, base string, force bool) error {
	wt := worktreeDir(root, slug)
	if _, err := os.Stat(wt); err != nil {
		return fmt.Errorf("la tarea %q no existe (mira 'hoom task list')", slug)
	}
	branch := "hoom/" + slug
	if !force {
		if st, _ := run(wt, "status", "--porcelain"); st != "" {
			return fmt.Errorf("la tarea %q tiene cambios sin commitear (incluidos posibles veredictos).\n  Accion: commitea todo dentro de %s y repite 'hoom task done %s'", slug, wt, slug)
		}
		all, err := verdict.LoadAll(wt)
		if err != nil || len(all) == 0 {
			return fmt.Errorf("la tarea %q no tiene veredictos. Accion: ejecuta 'hoom verify' dentro del worktree", slug)
		}
		last := all[len(all)-1]
		if last.Verdict != "green" {
			return fmt.Errorf("el ultimo veredicto de %q es ROJO (%s). Accion: corrige y re-ejecuta 'hoom verify' en el worktree", slug, last.ID)
		}
		g := gitx.Snapshot(wt, base)
		if g.ChangeFingerprint != last.Git.ChangeFingerprint {
			return fmt.Errorf("el arbol de %q cambio despues del ultimo veredicto verde (huella %s vs %s).\n  Accion: re-ejecuta 'hoom verify' dentro del worktree y commitea", slug, g.ChangeFingerprint, last.Git.ChangeFingerprint)
		}
	}
	args := []string{"worktree", "remove", wt}
	if force {
		args = []string{"worktree", "remove", "--force", wt}
	}
	if out, err := run(root, args...); err != nil {
		return fmt.Errorf("git worktree remove fallo: %s", out)
	}
	fmt.Printf("hoom: tarea %q cerrada con veredicto verde y huella coincidente\n", slug)
	fmt.Printf("  la rama %s queda lista para integrar:\n", branch)
	fmt.Printf("    git merge --no-ff %s\n", branch)
	fmt.Printf("    git branch -d %s   (despues del merge)\n", branch)
	return nil
}
