package itemcmd

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// wFueraDeRama leaves the worktree on another branch or with a detached
// HEAD, its unsaved changes with it, and returns the words of the refusal.
func wFueraDeRama(t *testing.T, wt string, separado bool) string {
	t.Helper()
	if separado {
		wGit(t, wt, "checkout", "-q", "--detach")
		return "con HEAD separado"
	}
	wGit(t, wt, "checkout", "-q", "-b", "otra")
	return "en la rama otra"
}

// Finding 13bba6: from the task's worktree on another branch or with a
// detached HEAD, Save refuses with ErrOtraRama, as from the main checkout,
// with expect (the Studio) and without (hoom item save), the item in the
// main checkout or only on the branch, and no tree gets a commit.
func TestSaveDesdeElWorktreeFueraDeSuRamaSeNiega(t *testing.T) {
	for _, separado := range []bool{false, true} {
		for _, soloEnLaRama := range []bool{false, true} {
			principal, wt := wProyecto(t, !soloEnLaRama, "bajo")
			if soloEnLaRama {
				if _, err := item.Add(wt, item.Draft{Titulo: "Legit", Slug: "legit"}); err != nil {
					t.Fatal(err)
				}
			}
			wCambios(t, wt)
			vista := wVista(t, wt)
			dice := wFueraDeRama(t, wt, separado)
			headP, headWT := wSalida(t, principal, "rev-parse", "HEAD"), wSalida(t, wt, "rev-parse", "HEAD")
			for _, expect := range [][]string{vista, nil} {
				_, err := Save(wt, "main", "high", "legit", expect)
				if !errors.Is(err, runcmd.ErrOtraRama) || !strings.Contains(err.Error(), dice) {
					t.Fatalf("separado=%v soloEnLaRama=%v expect=%q: %v", separado, soloEnLaRama, expect, err)
				}
			}
			if wSalida(t, principal, "rev-parse", "HEAD") != headP || wSalida(t, wt, "rev-parse", "HEAD") != headWT {
				t.Fatalf("separado=%v soloEnLaRama=%v: ningun arbol gana un commit", separado, soloEnLaRama)
			}
		}
	}
}

// On hoom/<slug>, from the main checkout and in a project with no task
// worktree, the gate lets Save through as before.
func TestSaveEnSuRamaYSinEspacioSigueIgual(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	wCambios(t, wt)
	if res, err := Save(wt, "main", "high", "legit", wVista(t, wt)); err != nil || len(res.Commits) != 1 {
		t.Fatalf("en hoom/legit, con expect: %+v %v", res, err)
	}
	if res, err := Save(wt, "main", "high", "legit", nil); err != nil || len(res.Commits) != 1 || res.Commits[0].Dir != "." {
		t.Fatalf("en hoom/legit, sin expect: %+v %v", res, err)
	}
	if st := wSalida(t, principal, "status", "--porcelain", "--", ".hoom/items"); st != "" {
		t.Fatalf("el item del principal queda guardado: %q", st)
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	solo := filepath.Join(base, "solo")
	wGit(t, base, "init", "-q", "-b", "main", solo)
	wGit(t, solo, "config", "user.email", "test@hoom.dev")
	wGit(t, solo, "config", "user.name", "hoom test")
	wGit(t, solo, "commit", "-q", "--allow-empty", "-m", "inicial")
	if _, err := item.Add(solo, item.Draft{Titulo: "Legit", Slug: "legit"}); err != nil {
		t.Fatal(err)
	}
	vista := wVista(t, solo)
	if res, err := Save(solo, "main", "high", "legit", vista); err != nil || len(res.Commits) != 1 || !sameSet(res.Commits[0].Paths, vista) {
		t.Fatalf("sin espacio de trabajo: %+v %v", res, err)
	}
}
