// Tests adversariales de los hallazgos 20261021T174140_966474 y
// 20261021T174140_c284d0 (medium, reliability) de la delta 1 de
// fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md, CA-280), en la
// puerta. Escritos desde el contrato, nunca desde la implementacion:
//
// Un worktree registrado en su lugar que quedo con HEAD separado o en otra
// rama que no es hoom/<slug> no es "una tarea por crear": runcmd.TaskDir
// falla con un error que dice que no esta en su rama hoom/<slug>, sin "no
// existe" y sin ErrSinComprobar (git respondio), y no toca nada. Guarda: de
// vuelta en hoom/<slug>, la carpeta es su tarea.
//
// Este archivo compila en 37b6ecb. Lo que exige runcmd.ErrOtraRama (que no
// existe ahi) esta en hallazgos_c1b_966474_errotrarama_test.go.
//
// Usa internal/worktreetest, t68Entorno y t68Acepta del archivo de 68644c.
package runcmd_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// t96Demo: el proyecto de 68644c con la tarea demo (creada con
// taskcmd.Start, con trabajo sin commitear) dejada fuera de su rama por v.
func t96Demo(t *testing.T, v worktreetest.RamaAjena) (root, dir string) {
	t.Helper()
	root, _ = t68Entorno(t, false)
	if err := taskcmd.Start(root, "demo", "main"); err != nil {
		t.Fatalf("fixture: task start demo: %v", err)
	}
	dir = filepath.Join(root, ".hoom", "worktrees", "demo")
	worktreetest.Escribir(t, dir, "app.go", "package app\n\n// trabajo sin guardar de demo\n")
	worktreetest.Escribir(t, dir, "demo.go", "package app\n")
	t68Acepta(t, "precondicion: demo en su rama", root, "demo", dir)
	v.Armar(t, dir, "demo")
	return root, dir
}

// Hallazgos 966474 y c284d0. CA-280: TaskDir de demo fuera de su rama dice
// que no esta en su rama hoom/demo, no que no existe, y no toca nada.
func TestHallazgo966474_CA280_TaskDirFueraDeSuRamaDiceLaRama(t *testing.T) {
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			root, dir := t96Demo(t, v)
			antes := worktreetest.Foto(t, root)
			got, err := runcmd.TaskDir(root, "demo")
			switch {
			case err == nil:
				t.Errorf("CA-280: hallazgo 966474 (%s): TaskDir(demo) fuera de su rama debe fallar, devolvio %q", v.Nombre, got)
			case errors.Is(err, gitx.ErrSinComprobar):
				t.Errorf("CA-280: hallazgo 966474 (%s): git respondio: no es ErrSinComprobar: %v", v.Nombre, err)
			default:
				if m := err.Error(); !strings.Contains(m, "hoom/demo") || !strings.Contains(m, "rama") || strings.Contains(m, "no existe") {
					t.Errorf("CA-280: hallazgo 966474 (%s): el error dice que demo no esta en su rama hoom/demo (y no \"no existe\"): %v", v.Nombre, err)
				}
			}
			if despues := worktreetest.Foto(t, root); despues != antes {
				t.Errorf("CA-280: hallazgo 966474 (%s): TaskDir no toca nada, y cambio:\n%s", v.Nombre, worktreetest.Cambios(antes, despues))
			}
			worktreetest.VolverASuRama(t, dir, "demo")
			t68Acepta(t, v.Nombre+", de vuelta en hoom/demo", root, "demo", dir)
		})
	}
}
