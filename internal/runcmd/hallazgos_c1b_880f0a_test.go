// Tests adversariales del hallazgo 20261021T173532_880f0a (high, risk) de la
// delta 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md,
// CA-280: la carpeta de la tarea es el worktree .hoom/worktrees/<slug>).
// Escritos desde el contrato, nunca desde la implementacion:
//
// El registro del worktree (<common-dir>/worktrees/<id>/gitdir) tiene que
// nombrar EXACTAMENTE <la carpeta>/.git. Un registro que nombra otro archivo
// que existe (otro nombre en la misma carpeta, el .git de una subcarpeta, o
// .GIT) no es el de esa carpeta: runcmd.TaskDir falla, sin tocar nada, y es
// un "no" con git sano (no ErrSinComprobar). Guarda: antes de editarlo y con
// el registro original devuelto, la carpeta es su tarea.
//
// Usa internal/worktreetest y t68Acepta del archivo de 68644c.
package runcmd_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// Hallazgo 880f0a (high). CA-280: con el registro de la tarea demo editado
// para nombrar otro archivo que existe, TaskDir(demo) falla con un "no" y no
// toca legit, la raiz ni la carpeta. Con el registro original, vuelve a ser
// la tarea.
func TestHallazgo880f0a_CA280_TaskDirExigeQueElRegistroNombreSuPropioGit(t *testing.T) {
	for _, v := range worktreetest.RegistrosAjenos {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			root, _ := t68Entorno(t, false)
			if err := taskcmd.Start(root, "demo", "main"); err != nil {
				t.Fatalf("fixture: task start demo: %v", err)
			}
			dir := filepath.Join(root, ".hoom", "worktrees", "demo")
			worktreetest.Escribir(t, dir, "app.go", "package app\n\n// trabajo sin guardar de demo\n")
			worktreetest.Escribir(t, dir, "demo.go", "package app\n")
			t68Acepta(t, "precondicion: el registro de git sin editar", root, "demo", dir)

			reg := worktreetest.Registro(t, dir)
			original, err := os.ReadFile(reg)
			if err != nil {
				t.Fatal(err)
			}
			ajeno := v.Armar(t, dir)
			if err := os.WriteFile(reg, []byte(ajeno+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			antes := worktreetest.Foto(t, root)
			got, err := runcmd.TaskDir(root, "demo")
			if err == nil {
				t.Errorf("CA-280: hallazgo 880f0a (%s): el registro nombra %s, no %s: TaskDir(demo) debe fallar, devolvio %q",
					v.Nombre, ajeno, filepath.Join(dir, ".git"), got)
			} else if errors.Is(err, gitx.ErrSinComprobar) {
				t.Errorf("CA-280: hallazgo 880f0a (%s): con git sano el registro se lee: es un \"no\", no ErrSinComprobar: %v", v.Nombre, err)
			}
			if despues := worktreetest.Foto(t, root); despues != antes {
				t.Errorf("CA-280: hallazgo 880f0a (%s): TaskDir no toca nada, y cambio:\n%s", v.Nombre, worktreetest.Cambios(antes, despues))
			}

			if err := os.WriteFile(reg, original, 0o644); err != nil {
				t.Fatal(err)
			}
			t68Acepta(t, v.Nombre+", con el registro original devuelto", root, "demo", dir)
		})
	}
}
