package reviewcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
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
		if _, err := item.Add(principal, item.Draft{Titulo: "Legit", Slug: "legit", Riesgo: r}); err != nil {
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

// Henry's decision: `hoom review` run from the task's worktree takes the
// declared riesgo from the main checkout's item.
func TestReviewLeeElRiesgoDelPrincipal(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	if r, err := riesgoDeclarado(wt, "legit"); err != nil || r != "bajo" {
		t.Fatalf("fixture: %q %v", r, err)
	}
	if _, _, err := item.SetRiesgo(principal, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	if r, err := riesgoDeclarado(wt, "legit"); err != nil || r != "alto" {
		t.Fatalf("la review desde el worktree lee el alto del principal: %q %v", r, err)
	}
}
