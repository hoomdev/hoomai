// Tests adversariales del hallazgo 20261021T173532_880f0a (high, risk) de la
// delta 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md,
// CA-280; descartar desde la tarjeta, CA-346 de
// .hoom/specs/acciones-desde-la-tarjeta.md). Escritos desde el contrato,
// nunca desde la implementacion:
//
// El registro del worktree (<common-dir>/worktrees/<id>/gitdir) tiene que
// nombrar EXACTAMENTE <la carpeta>/.git. Con el registro de la tarea demo
// editado para nombrar otro archivo que existe (otro nombre en la misma
// carpeta, el .git de una subcarpeta, .GIT, o <carpeta>/s/../.git con s un
// symlink hacia afuera), Discardable, RunDiscard (con y sin --yes) y
// Discard fallan y no tocan nada: ni demo (su trabajo sin commitear), ni
// legit, ni la raiz. Guarda: con el registro sin editar, y con
// el original devuelto, Discardable de demo lista su trabajo.
//
// Usa internal/worktreetest y t68Descartar del archivo de 68644c (sus
// mensajes citan 68644c: es la misma regla de la puerta, la del registro).
package taskcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// Hallazgo 880f0a (high). CA-346: con el registro de demo nombrando otro
// archivo que existe, descartar demo falla sin tocar nada.
func TestHallazgo880f0a_CA346_DiscardExigeQueElRegistroNombreSuPropioGit(t *testing.T) {
	for _, v := range worktreetest.RegistrosAjenos {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			root, legit := worktreetest.Proyecto(t, Start)
			worktreetest.Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
			worktreetest.Escribir(t, legit, "wip-legit.txt", "sin guardar\n")
			worktreetest.Escribir(t, root, "app.go", "package app\n\n// trabajo sin guardar de la raiz\n")
			worktreetest.Escribir(t, root, "raiz-nuevo.go", "package app\n")
			if err := Start(root, "demo", "main"); err != nil {
				t.Fatalf("fixture: task start demo: %v", err)
			}
			dir := filepath.Join(root, ".hoom", "worktrees", "demo")
			worktreetest.Escribir(t, dir, "app.go", "package app\n\n// escrito desde demo\n")
			worktreetest.Escribir(t, dir, "demo.go", "package app\n\n// archivo de demo\n")
			want := []string{".hoom/worktrees/demo/app.go", ".hoom/worktrees/demo/demo.go"}
			if lista, err := Discardable(root, "demo"); err != nil || !cbMismos(lista, want) {
				t.Fatalf("CA-346: precondicion 880f0a: con el registro sin editar Discardable(demo) lista %v, dio %v, %v", want, lista, err)
			}

			reg := worktreetest.Registro(t, dir)
			original, err := os.ReadFile(reg)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(reg, []byte(v.Armar(t, dir)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			t68Descartar(t, root, "demo", nil)

			if err := os.WriteFile(reg, original, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Discardable(root, "demo"); err != nil {
				t.Errorf("CA-346: guarda 880f0a (%s): con el registro original devuelto, demo vuelve a ser la tarea: %v", v.Nombre, err)
			}
		})
	}
}
