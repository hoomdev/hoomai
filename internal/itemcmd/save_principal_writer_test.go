package itemcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
)

// wSalida runs git in dir and returns its output, trimmed.
func wSalida(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func wSave(t *testing.T, root, slug string) (string, error) {
	t.Helper()
	req, err := Parse([]string{"save", slug})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOOM_ROLE", "")
	var out, errb bytes.Buffer
	err = Execute(root, "main", "high", req, &out, &errb)
	return out.String(), err
}

// Finding 42ffc0 (Henry's decision): from the task's worktree, set and then
// save commit the main checkout's item, the one set wrote, and leave the
// worktree's copy and branch alone.
func TestSaveDesdeElWorktreeGuardaElItemDelPrincipal(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	headWT := wSalida(t, wt, "rev-parse", "HEAD")
	headP := wSalida(t, principal, "rev-parse", "HEAD")
	if _, err := wSet(t, wt, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	out, err := wSave(t, wt, "legit")
	if err != nil || !strings.Contains(out, "tarjeta legit guardada") {
		t.Fatalf("save desde el worktree: %q %v", out, err)
	}
	if h := wSalida(t, principal, "rev-parse", "HEAD"); h == headP {
		t.Fatal("hay un commit nuevo en el principal")
	}
	if msg := wSalida(t, principal, "log", "-1", "--format=%s"); msg != SaveMessage("legit") {
		t.Fatalf("el commit es el de save: %q", msg)
	}
	if body := wSalida(t, principal, "show", "HEAD:.hoom/items/legit.yaml"); !strings.Contains(body, "riesgo: alto") {
		t.Fatalf("el commit lleva riesgo: alto:\n%s", body)
	}
	if st := wSalida(t, principal, "status", "--porcelain", "--", ".hoom/items/legit.yaml"); st != "" {
		t.Fatalf("el item del principal queda limpio: %q", st)
	}
	if h := wSalida(t, wt, "rev-parse", "HEAD"); h != headWT {
		t.Fatal("la rama del worktree no se toca")
	}
	if it, err := item.Load(wt, "legit"); err != nil || it.Riesgo != "bajo" {
		t.Fatalf("la copia del worktree no se toca: %+v %v", it, err)
	}
}

// From the main checkout, save is what it was: it commits the item there.
func TestSaveDesdeElPrincipalComoHoy(t *testing.T) {
	principal, _ := wProyecto(t, true, "bajo")
	if _, err := wSet(t, principal, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	if out, err := wSave(t, principal, "legit"); err != nil || !strings.Contains(out, "tarjeta legit guardada") {
		t.Fatalf("%q %v", out, err)
	}
	if st := wSalida(t, principal, "status", "--porcelain", "--", ".hoom/items/legit.yaml"); st != "" {
		t.Fatalf("limpio: %q", st)
	}
}

// With the item only in the task's tree, save keeps saving there.
func TestSaveItemSoloEnLaRama(t *testing.T) {
	principal, wt := wProyecto(t, false, "")
	if _, err := item.Add(wt, item.Draft{Titulo: "Legit", Slug: "legit"}); err != nil {
		t.Fatal(err)
	}
	headP := wSalida(t, principal, "rev-parse", "HEAD")
	if out, err := wSave(t, wt, "legit"); err != nil || !strings.Contains(out, "tarjeta legit guardada") {
		t.Fatalf("%q %v", out, err)
	}
	if body := wSalida(t, wt, "show", "HEAD:.hoom/items/legit.yaml"); !strings.Contains(body, "Legit") {
		t.Fatalf("el item queda commiteado en la rama:\n%s", body)
	}
	if h := wSalida(t, principal, "rev-parse", "HEAD"); h != headP {
		t.Fatal("el principal no se toca")
	}
}

// Unsaved changes in both trees: from the worktree, save does what it does
// from the project, one commit per tree: the workspace's work in the task's
// branch, the item in the main checkout.
func TestSaveDesdeElWorktreeConCambiosEnLosDosArboles(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	if err := os.WriteFile(filepath.Join(wt, "trabajo.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wSet(t, wt, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	if out, err := wSave(t, wt, "legit"); err != nil || !strings.Contains(out, "tarjeta legit guardada") {
		t.Fatalf("%q %v", out, err)
	}
	if st := wSalida(t, wt, "status", "--porcelain", "--", "trabajo.go"); st != "" {
		t.Fatalf("el trabajo del worktree queda commiteado: %q", st)
	}
	if msg := wSalida(t, wt, "log", "-1", "--format=%s"); msg != SaveMessage("legit") {
		t.Fatalf("en la rama de la tarea: %q", msg)
	}
	if body := wSalida(t, principal, "show", "HEAD:.hoom/items/legit.yaml"); !strings.Contains(body, "riesgo: alto") {
		t.Fatalf("el item, en el principal:\n%s", body)
	}
	if st := wSalida(t, principal, "status", "--porcelain", "--", ".hoom/items"); st != "" {
		t.Fatalf("los items del principal quedan limpios: %q", st)
	}
}
