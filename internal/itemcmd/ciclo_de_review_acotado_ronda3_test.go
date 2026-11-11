// Tests de regresion de la ronda 3 de ciclo-de-review-acotado (CA-617):
// `hoom item save <slug>` resuelve el arbol igual que `hoom item set`. Desde
// el worktree de la tarea, si el item existe en el checkout principal, save
// commitea el item en el principal; si solo existe en el arbol de la tarea,
// lo guarda ahi como hoy. La linea de set no se mira aca (la fija 321462).
// Escritos desde el spec, las firmas publicas y el texto del hallazgo, sin
// leer la implementacion.
package itemcmd

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func crR3Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v en %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// crR3Guardado exige que el item de dir este limpio en git status y que el
// commit nuevo de dir (HEAD distinto de antes) contenga riesgo: alto.
func crR3Guardado(t *testing.T, id, donde, dir, antes, slug string) {
	t.Helper()
	rel := ".hoom/items/" + slug + ".yaml"
	if st := crR3Git(t, dir, "status", "--porcelain", "--", rel); strings.TrimSpace(st) != "" {
		t.Fatalf("hallazgo %s (CA-617): %s queda con %s limpio en git status: %q", id, donde, rel, st)
	}
	if crHead(t, dir) == antes {
		t.Fatalf("hallazgo %s (CA-617): save hace un commit nuevo en %s", id, donde)
	}
	if !strings.Contains(crR3Git(t, dir, "show", "HEAD:"+rel), "riesgo: alto") {
		t.Fatalf("hallazgo %s (CA-617): el commit nuevo de %s contiene riesgo: alto", id, donde)
	}
}

func crR3Contenido(t *testing.T, dir, slug string) string {
	t.Helper()
	return crR2Leer(t, dir, slug)
}

// hallazgo 20261111T220511_42ffc0 (CA-617): por itemcmd, desde el worktree,
// Set y despues save: guarda la tarjeta en el checkout principal; la copia
// del worktree no se toca y el worktree no gana commit.
func TestHallazgo_42ffc0_SaveDesdeElWorktreeGuardaEnElPrincipal(t *testing.T) {
	root, wt := crR1Tarea(t, "precios")
	headP, headW := crHead(t, root), crHead(t, wt)
	copia := crR3Contenido(t, wt, "precios")
	if _, err := Set(wt, "main", "high", "precios", "alto", ""); err != nil {
		t.Fatalf("42ffc0 (fixture): Set desde el worktree: %v", err)
	}
	out, errb, err := cbExec(t, wt, "save", "precios")
	if err != nil || !strings.Contains(out, "tarjeta precios guardada") {
		t.Fatalf("hallazgo 42ffc0 (CA-617): save desde el worktree informa `tarjeta precios guardada`: %v\n%s%s", err, out, errb)
	}
	crR3Guardado(t, "42ffc0", "el principal", root, headP, "precios")
	if crHead(t, wt) != headW {
		t.Fatal("hallazgo 42ffc0 (CA-617): el worktree no gana commit")
	}
	if crR3Contenido(t, wt, "precios") != copia {
		t.Fatal("hallazgo 42ffc0 (CA-617): la copia del worktree no se toca")
	}
}

// hallazgo 20261111T220511_42ffc0 (CA-617), por el binario: `hoom item set`
// y `hoom item save` desde el worktree.
func TestHallazgo_42ffc0_SaveDesdeElWorktreePorElBinario(t *testing.T) {
	bin := ixBinario(t)
	root, wt := crR1Tarea(t, "precios")
	headP, headW := crHead(t, root), crHead(t, wt)
	copia := crR3Contenido(t, wt, "precios")
	if r := crCorrer(t, bin, wt, "", "item", "set", "precios", "--riesgo", "alto"); r.exit != 0 {
		t.Fatalf("42ffc0 (fixture): set sale 0: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	r := crCorrer(t, bin, wt, "", "item", "save", "precios")
	if r.exit != 0 || !strings.Contains(r.stdout, "tarjeta precios guardada") {
		t.Fatalf("hallazgo 42ffc0 (CA-617): save desde el worktree sale 0 con `tarjeta precios guardada`: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	crR3Guardado(t, "42ffc0", "el principal", root, headP, "precios")
	if crHead(t, wt) != headW || crR3Contenido(t, wt, "precios") != copia {
		t.Fatal("hallazgo 42ffc0 (CA-617): el worktree no gana commit y su copia no se toca")
	}
}

// CA-617 (guarda de 42ffc0): desde el principal, set + save sigue igual.
func TestHallazgo_42ffc0_GuardaDesdeElPrincipal(t *testing.T) {
	root, wt := crR1Tarea(t, "precios")
	headP, headW := crHead(t, root), crHead(t, wt)
	if _, err := Set(root, "main", "high", "precios", "alto", ""); err != nil {
		t.Fatal(err)
	}
	out, errb, err := cbExec(t, root, "save", "precios")
	if err != nil || !strings.Contains(out, "tarjeta precios guardada") {
		t.Fatalf("CA-617 (42ffc0, guarda): save desde el principal: %v\n%s%s", err, out, errb)
	}
	crR3Guardado(t, "42ffc0", "el principal", root, headP, "precios")
	if crHead(t, wt) != headW {
		t.Fatal("CA-617 (42ffc0, guarda): el worktree no gana commit")
	}
}

// CA-617 (guarda de 42ffc0, fallback de la ronda 1): un item que solo existe
// en el arbol de la tarea se cambia y se guarda en el worktree, como hoy.
func TestHallazgo_42ffc0_ItemSoloEnLaTareaSeGuardaEnElWorktree(t *testing.T) {
	root := icRepo(t)
	icEscribir(t, root, ".hoom/.gitignore", "worktrees/\n")
	crCommitearItems(t, root)
	wt := filepath.Join(root, ".hoom", "worktrees", "precios")
	icGit(t, root, "worktree", "add", "-q", "-b", "hoom/precios", wt, "main")
	icEscribir(t, wt, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	crCommitearItems(t, wt)
	headP, headW := crHead(t, root), crHead(t, wt)

	if _, err := Set(wt, "main", "high", "precios", "alto", ""); err != nil {
		t.Fatalf("42ffc0 (fixture): Set del item solo en la tarea: %v", err)
	}
	out, errb, err := cbExec(t, wt, "save", "precios")
	if err != nil || !strings.Contains(out, "tarjeta precios guardada") {
		t.Fatalf("CA-617 (42ffc0, fallback): save desde el worktree: %v\n%s%s", err, out, errb)
	}
	crR3Guardado(t, "42ffc0", "el worktree", wt, headW, "precios")
	if crHead(t, root) != headP {
		t.Fatal("CA-617 (42ffc0, fallback): el principal no gana commit")
	}
}
