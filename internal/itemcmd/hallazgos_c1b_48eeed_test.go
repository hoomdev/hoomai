// Tests adversariales de los hallazgos 20261021T174140_48eeed (medium,
// reliability) y 20261021T174455_44ed99 (medium, resilience) de la delta 1
// de fixes-cabina-c1b, sobre guardar la tarjeta (CA-344 de
// .hoom/specs/acciones-desde-la-tarjeta.md; CA-280 y CA-286 de
// .hoom/specs/items-y-columna-derivada.md), en el camino que vio el
// refutador en f12cbb0. Escritos desde el contrato, nunca desde la
// implementacion:
//
// Con .hoom/worktrees cerrada (0600: se listan los nombres pero no se
// alcanzan; o 0000), no se puede saber si legit es la tarea: Save de la
// tarjeta legit falla y no commitea nada, ni en legit ni en la raiz (no la
// trata como una tarjeta sin espacio de trabajo cuyo trabajo esta en la
// raiz). La foto se compara con la carpeta abierta otra vez.
//
// Reusa worktreetest (Proyecto, Foto, Cerrar, ModosCerrados) y t28Item de
// hallazgos_c1b_28c350_test.go.
package itemcmd

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// Hallazgos 48eeed y 44ed99. CA-344: con .hoom/worktrees cerrada, Save de
// legit falla sin commitear nada; legit, la raiz y sus trabajos sin guardar
// quedan igual.
func TestHallazgo48eeed_CA344_SaveConWorktreesCerradaNoCommiteaNada(t *testing.T) {
	for _, modo := range worktreetest.ModosCerrados {
		modo := modo
		t.Run(fmt.Sprintf("%04o", modo), func(t *testing.T) {
			root, legit := worktreetest.Proyecto(t, taskcmd.Start)
			worktreetest.Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
			worktreetest.Escribir(t, legit, "wip-legit.txt", "sin guardar\n")
			worktreetest.Escribir(t, root, "app.go", "package app\n\n// trabajo sin guardar de la raiz\n")
			worktreetest.Escribir(t, root, "raiz-nuevo.go", "package app\n")
			t28Item(t, root, "legit")
			antes := worktreetest.Foto(t, root)

			abrir := worktreetest.Cerrar(t, filepath.Join(root, ".hoom", "worktrees"), "legit", modo)
			res, err := Save(root, "main", "high", "legit", nil)
			abrir()
			if err == nil {
				t.Errorf("CA-344: hallazgo 48eeed (.hoom/worktrees en %04o): Save de legit sin poder comprobar su espacio de trabajo debe fallar, guardo: %+v", modo, res)
			}
			if despues := worktreetest.Foto(t, root); despues != antes {
				t.Errorf("CA-344: hallazgo 48eeed (.hoom/worktrees en %04o): Save no commitea nada, y cambio:\n%s", modo, worktreetest.Cambios(antes, despues))
			}
		})
	}
}
