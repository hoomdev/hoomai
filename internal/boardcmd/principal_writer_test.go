package boardcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// Henry's decision: the board run from the task's worktree shows the riesgo
// of the main checkout's item, and requires the review it asks for.
func TestTableroLeeElRiesgoDelPrincipal(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	if _, _, err := item.SetRiesgo(principal, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	it, err := item.Load(wt, "legit")
	if err != nil || it.Riesgo != "bajo" {
		t.Fatalf("fixture: el item del worktree: %+v %v", it, err)
	}
	ev := Gather(wt, "main", "high", it, time.Now())
	if ev.Item.Riesgo != "alto" || declaradoDe(ev.Item) != "alto" {
		t.Fatalf("el tablero desde el worktree declara el alto del principal: %q", ev.Item.Riesgo)
	}
}
