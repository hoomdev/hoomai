// Tests adversariales de los hallazgos 20261021T174140_966474 y
// 20261021T174140_c284d0 (medium, reliability) de la delta 1 de
// fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md, CA-280), sobre
// runcmd.ErrOtraRama. Escritos desde el contrato, nunca desde la
// implementacion:
//
// TaskDir de un worktree registrado en su lugar que no esta en hoom/<slug>
// devuelve un error que envuelve runcmd.ErrOtraRama (errors.Is): es un
// worktree a arreglar, no una tarea a crear. Guarda: solo eso es
// ErrOtraRama. Ninguna carpeta de worktreetest.Casos que no sea ese worktree
// (copias, movidos, .git hacia el gitdir de otra tarea...), ni una carpeta
// comun, ni una que no esta, ni la tarea sin git en el PATH
// (ErrSinComprobar) dan ErrOtraRama; otro (registrado en su lugar, en la
// rama de legit) si.
//
// OJO: runcmd.ErrOtraRama no existe en 37b6ecb: este archivo no compila ahi
// (falla en la base). Usa t96Demo de hallazgos_c1b_966474_test.go y
// t6Proyecto del archivo de 6aeb17.
package runcmd_test

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// Hallazgos 966474 y c284d0. CA-280: TaskDir de demo fuera de su rama
// envuelve runcmd.ErrOtraRama.
func TestHallazgo966474_CA280_TaskDirFueraDeSuRamaEsErrOtraRama(t *testing.T) {
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			root, _ := t96Demo(t, v)
			if _, err := runcmd.TaskDir(root, "demo"); !errors.Is(err, runcmd.ErrOtraRama) {
				t.Errorf("CA-280: hallazgo 966474 (%s): TaskDir(demo) fuera de su rama envuelve runcmd.ErrOtraRama: %v", v.Nombre, err)
			}
		})
	}
}

// Guarda de los hallazgos 966474 y c284d0. CA-280: ErrOtraRama es solo el
// worktree registrado en su lugar y fuera de su rama.
func TestHallazgo966474_CA280_GuardaSoloElWorktreeFueraDeSuRamaEsErrOtraRama(t *testing.T) {
	root, legit := t6Proyecto(t)
	for _, c := range worktreetest.Casos {
		worktreetest.Armar(t, root, c.Slug, true)
		_, err := runcmd.TaskDir(root, c.Slug)
		if c.Slug == "otro" {
			if !errors.Is(err, runcmd.ErrOtraRama) {
				t.Errorf("CA-280: hallazgo 966474 (%s): otro esta registrado en su lugar y en la rama de legit: envuelve runcmd.ErrOtraRama: %v", c.Que, err)
			}
			continue
		}
		if err == nil || errors.Is(err, runcmd.ErrOtraRama) {
			t.Errorf("CA-280: guarda 966474 (%s): %s no es un worktree de la tarea: falla sin runcmd.ErrOtraRama: %v", c.Que, c.Slug, err)
		}
	}
	for _, slug := range []string{"comun", "nada"} {
		if _, err := runcmd.TaskDir(root, slug); err == nil || errors.Is(err, runcmd.ErrOtraRama) {
			t.Errorf("CA-280: guarda 966474: %s no es un worktree: falla sin runcmd.ErrOtraRama: %v", slug, err)
		}
	}
	t68Acepta(t, "legit en su rama", root, "legit", legit)

	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("git"); err == nil {
		t.Fatal("fixture: git no esta en el PATH")
	}
	if _, err := runcmd.TaskDir(root, "legit"); !errors.Is(err, gitx.ErrSinComprobar) || errors.Is(err, runcmd.ErrOtraRama) {
		t.Errorf("CA-280: guarda 966474: sin git TaskDir(legit) no sabe la rama: ErrSinComprobar, no runcmd.ErrOtraRama: %v", err)
	}
}
