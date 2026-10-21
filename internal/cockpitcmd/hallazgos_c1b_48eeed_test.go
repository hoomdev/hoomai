// Tests adversariales de los hallazgos 20261021T174140_48eeed (medium,
// reliability) y 20261021T174455_44ed99 (medium, resilience) de la delta 1
// de fixes-cabina-c1b, sobre `hoom cockpit --task` (CA-85 de
// .hoom/specs/cockpit-tmux-zellij.md; CA-280 de
// .hoom/specs/items-y-columna-derivada.md), en el camino que vio el
// refutador en f12cbb0. Escritos desde el contrato, nunca desde la
// implementacion:
//
// Con .hoom/worktrees cerrada (0600: se listan los nombres pero no se
// alcanzan; o 0000), no se puede saber si legit es la tarea: Open (tmux) y
// Run (tmux y zellij) fallan sin lanzar nada, y su error envuelve
// gitx.ErrSinComprobar (errors.Is) y no dice que la tarea "no existe" ni
// manda a crearla (`hoom task start`, "crea su worktree"). Guarda: abierta
// otra vez, Open monta la sesion en el worktree.
//
// Reusa c28Intentos de hallazgos_c1b_c284d0_test.go, t28Proyecto y
// t28NuevaSesionEn de hallazgos_c1b_28c350_test.go, el tmux falso h3Tmux y
// worktreetest.Cerrar.
package cockpitcmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// Hallazgos 48eeed y 44ed99. CA-85 y CA-280: con .hoom/worktrees cerrada,
// el cockpit de legit devuelve el error de no poder comprobarlo.
func TestHallazgo48eeed_CA85_CockpitConWorktreesCerradaEsSinComprobar(t *testing.T) {
	for _, modo := range worktreetest.ModosCerrados {
		modo := modo
		t.Run(fmt.Sprintf("%04o", modo), func(t *testing.T) {
			e := t28Proyecto(t)
			abrir := worktreetest.Cerrar(t, filepath.Join(e.root, ".hoom", "worktrees"), "legit", modo)
			for _, in := range c28Intentos(t, e) {
				if in.err == nil {
					t.Errorf("CA-280: hallazgo 48eeed (%04o): %s sin poder comprobar la tarea debe fallar", modo, in.como)
					continue
				}
				if !errors.Is(in.err, gitx.ErrSinComprobar) {
					t.Errorf("CA-85: hallazgo 48eeed (%04o): el error de %s envuelve gitx.ErrSinComprobar: %v", modo, in.como, in.err)
				}
				if m := in.err.Error(); strings.Contains(m, "no existe") || strings.Contains(m, "task start") || strings.Contains(m, "crea su worktree") {
					t.Errorf("CA-85: hallazgo 48eeed (%04o): no se sabe si la tarea existe: el error de %s no dice que no existe ni manda a crearla: %v", modo, in.como, in.err)
				}
			}

			abrir()
			f := &h3Tmux{}
			if _, err := Open(e.root, "demo", Options{Provider: "claude", Task: "legit", Mux: "tmux"}, f.deps()); err != nil {
				t.Fatalf("CA-85: guarda 48eeed (%04o): abierta otra vez, Open funciona: %v\n%s", modo, err, f.todas())
			}
			if !t28NuevaSesionEn(f, e.legit) {
				t.Errorf("CA-85: guarda 48eeed (%04o): la sesion vive en el worktree (%s):\n%s", modo, e.legit, f.todas())
			}
		})
	}
}
