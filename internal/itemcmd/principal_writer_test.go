package itemcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
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

func wSet(t *testing.T, root, slug, r string) (string, error) {
	t.Helper()
	req, err := Parse([]string{"set", slug, "--riesgo", r})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOOM_ROLE", "")
	var out, errb bytes.Buffer
	err = Execute(root, "main", "high", req, &out, &errb)
	return out.String(), err
}

// Findings aee796 and f91dcf: `hoom item set` run from the task's worktree
// writes the item the riesgo is read from, the main checkout's, and says
// where to save it.
func TestSetEscribeElItemDelPrincipal(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	out, err := wSet(t, wt, "legit", "alto")
	if err != nil {
		t.Fatal(err)
	}
	if it, err := item.Load(principal, "legit"); err != nil || it.Riesgo != "alto" {
		t.Fatalf("el item del principal queda alto: %+v %v", it, err)
	}
	if it, err := item.Load(wt, "legit"); err != nil || it.Riesgo != "bajo" {
		t.Fatalf("el del worktree no se toca: %+v %v", it, err)
	}
	if r, _ := item.RiesgoDe(wt, "legit"); r != "alto" {
		t.Fatalf("lo que set escribe es lo que se lee: %q", r)
	}
	if !strings.Contains(out, "guardalo con: hoom item save legit") || !strings.Contains(out, principal) {
		t.Fatalf("la linea dice desde donde guardarlo: %q", out)
	}
	// from the main checkout, the line is the plain one
	out, err = wSet(t, principal, "legit", "bajo")
	if err != nil || strings.TrimSpace(out) != "riesgo de legit: alto -> bajo - guardalo con: hoom item save legit" {
		t.Fatalf("desde el principal: %q %v", out, err)
	}
}

// A card created only on the task's branch is written in the task's tree.
func TestSetItemSoloEnLaRama(t *testing.T) {
	principal, wt := wProyecto(t, false, "")
	if _, err := item.Add(wt, item.Draft{Titulo: "Legit", Slug: "legit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := wSet(t, wt, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	if it, err := item.Load(wt, "legit"); err != nil || it.Riesgo != "alto" {
		t.Fatalf("el item de la rama queda alto: %+v %v", it, err)
	}
	if _, err := os.Lstat(item.Path(principal, "legit")); !os.IsNotExist(err) {
		t.Fatalf("no se crea en el principal: %v", err)
	}
}

// wConHecho writes hecho_en into the item slug of root, as task done does.
func wConHecho(t *testing.T, root, slug string) {
	t.Helper()
	raw, err := os.ReadFile(item.Path(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("hecho_en: 2026-01-01T00:00:00Z\n")...)
	if err := os.WriteFile(item.Path(root, slug), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Findings 822b51 and c55dde: Set judges Hecho on the card of the tree it
// writes in. The main checkout's item is closed and the worktree's copy is
// not: from the worktree, set refuses and writes nothing.
func TestSetHechoEnElArbolDeDestino(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	wConHecho(t, principal, "legit")
	antes, _ := os.ReadFile(item.Path(principal, "legit"))
	if _, err := wSet(t, wt, "legit", "alto"); err == nil || !strings.Contains(err.Error(), "la tarjeta ya esta en Hecho") {
		t.Fatalf("el item cerrado del principal no cambia: %v", err)
	}
	if despues, _ := os.ReadFile(item.Path(principal, "legit")); !bytes.Equal(antes, despues) {
		t.Fatal("no escribe el item cerrado")
	}
}

// The mirror case: the worktree's copy says hecho_en, the main checkout's
// item (the one set writes) is open: set changes it.
func TestSetHechoSoloEnLaCopiaDelWorktree(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	wConHecho(t, wt, "legit")
	if _, err := wSet(t, wt, "legit", "alto"); err != nil {
		t.Fatalf("la tarjeta del principal esta abierta: %v", err)
	}
	if it, err := item.Load(principal, "legit"); err != nil || it.Riesgo != "alto" {
		t.Fatalf("%+v %v", it, err)
	}
}

// A historic card of the main checkout (historico: hallazgos, held by a
// finding of its task that does not block) still counts as Hecho, also run
// from the worktree, whose copy has no historico.
func TestSetHistoricaDelPrincipalSigueEnHecho(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	raw, err := os.ReadFile(item.Path(principal, "legit"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(item.Path(principal, "legit"), append(raw, []byte("historico: hallazgos\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := finding.Register(principal, "main", finding.Draft{Severity: "low", Lens: "readability",
		File: "app.go", Description: "low", Author: "Ana", Task: "legit"}); err != nil {
		t.Fatal(err)
	}
	for _, arbol := range []string{principal, wt} {
		if _, err := wSet(t, arbol, "legit", "alto"); err == nil || !strings.Contains(err.Error(), "la tarjeta ya esta en Hecho") {
			t.Fatalf("desde %s la historica sigue en Hecho: %v", arbol, err)
		}
	}
}
