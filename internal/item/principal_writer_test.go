package item

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// wGit runs git in dir for a fixture, without a hook's GIT_* variables.
func wGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// wProyecto is a project at main with a linked worktree of the task legit
// at .hoom/worktrees/legit; conItem adds legit's item (riesgo r) before the
// worktree is created, so both trees have it.
func wProyecto(t *testing.T, conItem bool, r string) (principal, wt string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	principal = filepath.Join(base, "proyecto")
	wGit(t, base, "init", "-q", "-b", "main", principal)
	wGit(t, principal, "config", "user.email", "test@hoom.dev")
	wGit(t, principal, "config", "user.name", "hoom test")
	if conItem {
		if _, err := Add(principal, Draft{Titulo: "Legit", Slug: "legit", Riesgo: r}); err != nil {
			t.Fatal(err)
		}
	}
	wGit(t, principal, "commit", "-q", "--allow-empty", "-m", "inicial")
	wGit(t, principal, "add", "-A")
	wGit(t, principal, "commit", "-q", "--allow-empty", "-m", "item")
	wt = filepath.Join(principal, ".hoom", "worktrees", "legit")
	wGit(t, principal, "worktree", "add", "-q", "-b", "hoom/legit", wt)
	return principal, wt
}

// Henry's decision on ciclo-de-review-acotado: the riesgo is the item of the
// main checkout; from the task's worktree too.
func TestRiesgoDeGanaElPrincipal(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	if _, _, err := SetRiesgo(principal, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	for _, arbol := range []string{wt, principal} {
		if r, err := RiesgoDe(arbol, "legit"); err != nil || r != "alto" {
			t.Fatalf("desde %s: RiesgoDe = %q %v, quiero alto (el del principal)", arbol, r, err)
		}
		if r := RiesgosDe(arbol)["legit"]; r != "alto" {
			t.Fatalf("desde %s: RiesgosDe = %q, quiero alto", arbol, r)
		}
	}
	// the worktree's own item still says bajo: only the reading changed
	if it, err := Load(wt, "legit"); err != nil || it.Riesgo != "bajo" {
		t.Fatalf("fixture: el item del worktree: %+v %v", it, err)
	}
}

// A card created only on the task's branch is read from the task's tree;
// with no item anywhere, normal.
func TestRiesgoDeSinItemEnElPrincipal(t *testing.T) {
	principal, wt := wProyecto(t, false, "")
	if r, err := RiesgoDe(wt, "legit"); err != nil || r != "normal" {
		t.Fatalf("sin item: %q %v, quiero normal", r, err)
	}
	if _, err := Add(wt, Draft{Titulo: "Legit", Slug: "legit", Riesgo: "alto"}); err != nil {
		t.Fatal(err)
	}
	if r, err := RiesgoDe(wt, "legit"); err != nil || r != "alto" {
		t.Fatalf("item solo en la rama: %q %v, quiero alto", r, err)
	}
	if r := RiesgosDe(wt)["legit"]; r != "alto" {
		t.Fatalf("RiesgosDe con item solo en la rama: %q", r)
	}
	if r, err := RiesgoDe(principal, "legit"); err != nil || r != "normal" {
		t.Fatalf("desde el principal no se mira la rama: %q %v", r, err)
	}
}
