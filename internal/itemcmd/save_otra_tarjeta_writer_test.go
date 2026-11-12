package itemcmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// wOtraTarjeta adds card precios, unsaved, to the worktree of task legit
// (and to the main checkout too when enPrincipal), and returns its unsynced
// as the worktree's board shows it.
func wOtraTarjeta(t *testing.T, principal, wt string, enPrincipal bool) []string {
	t.Helper()
	dirs := []string{wt}
	if enPrincipal {
		dirs = append(dirs, principal)
	}
	for _, d := range dirs {
		if _, err := item.Add(d, item.Draft{Titulo: "Precios", Slug: "precios"}); err != nil {
			t.Fatal(err)
		}
	}
	c, err := boardcmd.CardFor(wt, "main", "high", "precios", time.Now().UTC())
	if err != nil || len(c.Unsynced) == 0 {
		t.Fatalf("fixture: la tarjeta precios tiene algo sin guardar en el worktree: %+v %v", c.Unsynced, err)
	}
	return c.Unsynced
}

// Finding 65fbb7: from task legit's worktree off its branch, saving another
// card (precios) also goes through legit's gate: ErrOtraRama naming legit,
// with expect and without, and no tree gets a commit.
func TestSaveOtraTarjetaDesdeElWorktreeFueraDeSuRamaSeNiega(t *testing.T) {
	for _, separado := range []bool{false, true} {
		for _, enPrincipal := range []bool{false, true} {
			principal, wt := wProyecto(t, true, "bajo")
			vista := wOtraTarjeta(t, principal, wt, enPrincipal)
			dice := wFueraDeRama(t, wt, separado)
			headP, headWT := wSalida(t, principal, "rev-parse", "HEAD"), wSalida(t, wt, "rev-parse", "HEAD")
			for _, expect := range [][]string{vista, nil} {
				_, err := Save(wt, "main", "high", "precios", expect)
				if !errors.Is(err, runcmd.ErrOtraRama) || !strings.Contains(err.Error(), dice) || !strings.Contains(err.Error(), `"legit"`) {
					t.Fatalf("separado=%v enPrincipal=%v expect=%q: %v", separado, enPrincipal, expect, err)
				}
			}
			if wSalida(t, principal, "rev-parse", "HEAD") != headP || wSalida(t, wt, "rev-parse", "HEAD") != headWT {
				t.Fatalf("separado=%v enPrincipal=%v: ningun arbol gana un commit", separado, enPrincipal)
			}
		}
	}
}

// With legit's worktree on hoom/legit, saving card precios from there does
// what it did: what the worktree's board shows, in the worktree.
func TestSaveOtraTarjetaDesdeElWorktreeEnSuRamaSigueIgual(t *testing.T) {
	principal, wt := wProyecto(t, true, "bajo")
	vista := wOtraTarjeta(t, principal, wt, false)
	headP := wSalida(t, principal, "rev-parse", "HEAD")
	res, err := Save(wt, "main", "high", "precios", vista)
	if err != nil || len(res.Commits) != 1 || res.Commits[0].Dir != "." || !sameSet(res.Commits[0].Paths, vista) {
		t.Fatalf("en hoom/legit: %+v %v", res, err)
	}
	if wSalida(t, wt, "rev-parse", "HEAD") != res.Commits[0].SHA || wSalida(t, principal, "rev-parse", "HEAD") != headP {
		t.Fatal("el commit queda en el worktree y el principal no se toca")
	}
}
