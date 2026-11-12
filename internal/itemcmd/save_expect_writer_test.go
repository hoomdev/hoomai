package itemcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
)

// wVista is card legit's unsynced as the board of dir shows it.
func wVista(t *testing.T, dir string) []string {
	t.Helper()
	c, err := boardcmd.CardFor(dir, "main", "high", "legit", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return c.Unsynced
}

// wCambios leaves unsaved changes in both trees, from the worktree: the
// task's spec there and the riesgo set with `hoom item set`, which writes the
// main checkout's item.
func wCambios(t *testing.T, wt string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(wt, ".hoom", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".hoom", "specs", "legit.md"), []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wSet(t, wt, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
}

// Findings d26397 and f4d55b (Henry's decision): from the worktree, Save with
// the unsynced the worktree's board shows saves exactly that, in the
// worktree; the main checkout's item stays for `hoom item save`, which still
// commits it there (finding 42ffc0).
func TestSaveConExpectDesdeElWorktreeGuardaLoQueSeVio(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	if err := os.WriteFile(filepath.Join(wt, "trabajo.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wCambios(t, wt)
	vista := wVista(t, wt)
	if !sameSet(vista, []string{".hoom/specs/legit.md"}) {
		t.Fatalf("fixture: el tablero del worktree muestra %q", vista)
	}
	headP := wSalida(t, principal, "rev-parse", "HEAD")
	res, err := Save(wt, "main", "high", "legit", vista)
	if err != nil {
		t.Fatalf("guardar con lo que se vio: %v", err)
	}
	if len(res.Commits) != 1 || res.Commits[0].Dir != "." || !sameSet(res.Commits[0].Paths, vista) {
		t.Fatalf("un solo commit, en el worktree, con lo que se vio: %+v", res.Commits)
	}
	if wSalida(t, wt, "rev-parse", "HEAD") != res.Commits[0].SHA || wSalida(t, wt, "rev-parse", "--abbrev-ref", "HEAD") != "hoom/legit" {
		t.Fatal("el commit queda en la rama de la tarea")
	}
	if files := wSalida(t, wt, "show", "--name-only", "--format=", "HEAD"); files != ".hoom/specs/legit.md" {
		t.Fatalf("el commit lleva solo el spec: %q", files)
	}
	if st := wSalida(t, wt, "status", "--porcelain", "--", "trabajo.go"); st != "?? trabajo.go" {
		t.Fatalf("lo que el tablero no muestra sigue sin commitear: %q", st)
	}
	if wSalida(t, principal, "rev-parse", "HEAD") != headP {
		t.Fatal("el principal no gana un commit")
	}
	if st := wSalida(t, principal, "status", "--porcelain", "--", ".hoom/items/legit.yaml"); st == "" {
		t.Fatal("el item del principal sigue sin guardar")
	}
	if it, err := item.Load(principal, "legit"); err != nil || it.Riesgo != "alto" {
		t.Fatalf("el item del principal sigue alto en el disco: %+v %v", it, err)
	}
	// `hoom item save` from the worktree commits the main checkout's item
	if out, err := wSave(t, wt, "legit"); err != nil || !strings.Contains(out, "tarjeta legit guardada") {
		t.Fatalf("hoom item save desde el worktree: %q %v", out, err)
	}
	if body := wSalida(t, principal, "show", "HEAD:.hoom/items/legit.yaml"); !strings.Contains(body, "riesgo: alto") {
		t.Fatalf("el item del principal queda commiteado con riesgo: alto:\n%s", body)
	}
	if st := wSalida(t, principal, "status", "--porcelain", "--", ".hoom/items"); st != "" {
		t.Fatalf("el item del principal queda limpio: %q", st)
	}
}

// Another set than the one shown, or a path that appears in the worktree's
// view between viewing and saving, is ErrChanged, and neither tree gets a
// commit (finding ac2feb: compared under the locks).
func TestSaveConExpectDesdeElWorktreeOtroConjuntoNoCommitea(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	wCambios(t, wt)
	vista := wVista(t, wt)
	headP, headWT := wSalida(t, principal, "rev-parse", "HEAD"), wSalida(t, wt, "rev-parse", "HEAD")
	otros := map[string][]string{
		"las rutas del principal": wVista(t, principal),
		"una de mas":              append(append([]string{}, vista...), "trabajo.go"),
		"ninguna":                 {},
	}
	for nombre, expect := range otros {
		if _, err := Save(wt, "main", "high", "legit", expect); err == nil || err.Error() != ErrChanged {
			t.Fatalf("%s: %v", nombre, err)
		}
	}
	// the worktree's own copy of the item changes after the person looked
	if _, _, err := item.SetRiesgo(wt, "legit", "alto"); err != nil {
		t.Fatal(err)
	}
	if v := wVista(t, wt); sameSet(v, vista) {
		t.Fatalf("fixture: la vista del worktree gana una ruta: %q", v)
	}
	if _, err := Save(wt, "main", "high", "legit", vista); err == nil || err.Error() != ErrChanged {
		t.Fatalf("cambio entre ver y guardar: %v", err)
	}
	if wSalida(t, principal, "rev-parse", "HEAD") != headP || wSalida(t, wt, "rev-parse", "HEAD") != headWT {
		t.Fatal("ningun arbol gana un commit")
	}
}

// From the main checkout, and with the item only in the task's tree, expect
// is that tree's unsynced as before.
func TestSaveConExpectDesdeElPrincipalYSoloEnLaRama(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	wCambios(t, wt)
	res, err := Save(principal, "main", "high", "legit", wVista(t, principal))
	if err != nil || len(res.Commits) != 2 || res.Commits[0].Dir != ".hoom/worktrees/legit" || res.Commits[1].Dir != "." {
		t.Fatalf("desde el principal, un commit por arbol: %+v %v", res, err)
	}

	principal, wt = wProyecto(t, false, "")
	if _, err := item.Add(wt, item.Draft{Titulo: "Legit", Slug: "legit"}); err != nil {
		t.Fatal(err)
	}
	headP := wSalida(t, principal, "rev-parse", "HEAD")
	vista := wVista(t, wt)
	if res, err := Save(wt, "main", "high", "legit", vista); err != nil || len(res.Commits) != 1 || res.Commits[0].Dir != "." || !sameSet(res.Commits[0].Paths, vista) {
		t.Fatalf("solo en la rama: %+v %v", res, err)
	}
	if wSalida(t, principal, "rev-parse", "HEAD") != headP {
		t.Fatal("el principal no se toca")
	}
}
