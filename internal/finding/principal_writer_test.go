package finding

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

// Henry's decision: run from the task's worktree, the findings_open gate
// judges with the riesgo of the main checkout's item. There it is alto, so
// a medium of the task's completa review blocks; the worktree's item says
// bajo and would let it pass.
func TestGateLeeElRiesgoDelPrincipal(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	f, err := Register(wt, "main", Draft{Severity: "medium", Lens: "reliability", File: "app.go",
		Description: "medium de la review completa", Author: "Ana", Task: "legit"})
	if err != nil {
		t.Fatal(err)
	}
	id := "20260501T120000_aaaaaa"
	rec := `{"id":"` + id + `","created_at":"` + time.Now().UTC().Format(time.RFC3339) +
		`","task":"legit","findings":["` + f.ID + `"],"cobertura":"completa"}`
	if err := os.MkdirAll(filepath.Join(wt, ".hoom", "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".hoom", "reviews", id+".json"), []byte(rec), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := Gate(wt, "main", "high", ""); res.Status != "pass" {
		t.Fatalf("fixture: con bajo en los dos arboles el medium no bloquea: %+v", res)
	}
	if _, _, err := item.SetRiesgo(principal, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	res := Gate(wt, "main", "high", "")
	if res.Status != "fail" || !strings.Contains(res.OutputTail, "riesgo alto") {
		t.Fatalf("el gate desde el worktree juzga con el alto del principal: %+v", res)
	}
	c, err := ContextoDe(wt, wt, "high")
	if err != nil || c.Riesgos["legit"] != "alto" {
		t.Fatalf("ContextoDe desde el worktree: %+v %v", c.Riesgos, err)
	}
}
