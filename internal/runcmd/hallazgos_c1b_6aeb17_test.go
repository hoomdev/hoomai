// Tests adversariales del hallazgo 20261021T150458_6aeb17 (medium,
// resilience) de la review 1 de fixes-cabina-c1b
// (.hoom/specs/items-y-columna-derivada.md, CA-280). Escritos desde el
// contrato, nunca desde la implementacion:
//
// Cuando git no puede responder sobre la raiz (git fuera del PATH, la raiz
// que no es un repositorio, su .git ilegible) o la puerta no puede leer el
// registro del worktree (<root>/.git/worktrees/legit en chmod 000), no puede
// decir que la tarea "no existe": runcmd.TaskDir devuelve un error que
// envuelve gitx.ErrSinComprobar (errors.Is) y que no dice "no existe".
// Con git sano, ninguna carpeta de t68Casos da ErrSinComprobar, y la tarea
// registrada en otra rama (otro) dice que no esta en su rama, sin "no
// existe" ni ErrSinComprobar. Guarda: con git sano, una carpeta que no es un
// worktree (o que no esta) sigue dando "no existe" sin ErrSinComprobar.
//
// Se prueba la puerta por runcmd.TaskDir, no por la firma de gitx.WorktreeDe.
// OJO: gitx.ErrSinComprobar no existe en b72de8f: este archivo no compila
// ahi (falla en la base). Usa las utilidades del archivo de 68644c.
package runcmd_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// t6SinComprobar exige que TaskDir(root, slug) diga "no pude comprobarlo":
// un error que envuelve ErrSinComprobar y no dice "no existe".
func t6SinComprobar(t *testing.T, caso, root, slug string) {
	t.Helper()
	got, err := runcmd.TaskDir(root, slug)
	if err == nil {
		t.Errorf("CA-280: hallazgo 6aeb17 (%s): TaskDir(%q) sin poder comprobar debe fallar, devolvio %q", caso, slug, got)
		return
	}
	if !errors.Is(err, gitx.ErrSinComprobar) {
		t.Errorf("CA-280: hallazgo 6aeb17 (%s): el error de TaskDir(%q) debe envolver gitx.ErrSinComprobar: %v", caso, slug, err)
	}
	if strings.Contains(err.Error(), "no existe") {
		t.Errorf("CA-280: hallazgo 6aeb17 (%s): TaskDir(%q) no sabe si la tarea existe, no puede decir \"no existe\": %v", caso, slug, err)
	}
}

// t6NoEsSinComprobar exige que, con git sano, TaskDir(root, slug) falle sin
// ErrSinComprobar; conNoExiste ademas exige que diga "no existe".
func t6NoEsSinComprobar(t *testing.T, caso, root, slug string, conNoExiste bool) error {
	t.Helper()
	got, err := runcmd.TaskDir(root, slug)
	if err == nil {
		t.Errorf("CA-280: guarda 6aeb17 (%s): TaskDir(%q) debe fallar, devolvio %q", caso, slug, got)
		return nil
	}
	if errors.Is(err, gitx.ErrSinComprobar) {
		t.Errorf("CA-280: guarda 6aeb17 (%s): con git sano TaskDir(%q) responde, no es ErrSinComprobar: %v", caso, slug, err)
	}
	if conNoExiste && !strings.Contains(err.Error(), "no existe") {
		t.Errorf("CA-280: guarda 6aeb17 (%s): con git sano TaskDir(%q) sigue diciendo \"no existe\": %v", caso, slug, err)
	}
	return err
}

// t6Proyecto: el proyecto de 68644c con una carpeta comun al lado de legit,
// y git sin subir a buscar repos por encima del directorio del test.
func t6Proyecto(t *testing.T) (root, legit string) {
	t.Helper()
	root, legit = t68Proyecto(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	t68Escribir(t, filepath.Join(root, ".hoom", "worktrees", "comun"), "x.go", "package x\n")
	t68Acepta(t, "precondicion con git", root, "legit", legit)
	return root, legit
}

// t6SinPermisos deja dir en 000 y lo devuelve a 0755 al terminar el test
// (antes de que t.TempDir lo borre). Saltea si corre como root o si el
// sistema deja leerlo igual: el test no probaria nada.
func t6SinPermisos(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita la lectura, el test no probaria nada")
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("este sistema deja listar un directorio sin permiso de lectura")
	}
}

// Hallazgo 6aeb17. CA-280: con git fuera del PATH, la tarea legit (un
// worktree registrado de verdad) no "no existe": TaskDir devuelve
// ErrSinComprobar. Lo mismo con una carpeta comun: tampoco se puede saber.
func TestHallazgo6aeb17_CA280_TaskDirSinGitEsSinComprobar(t *testing.T) {
	root, _ := t6Proyecto(t)
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("git"); err == nil {
		t.Fatal("fixture: git no esta en el PATH")
	}
	t6SinComprobar(t, "git fuera del PATH, legit", root, "legit")
	t6SinComprobar(t, "git fuera del PATH, carpeta comun", root, "comun")
}

// Hallazgo 6aeb17. CA-280: si git no puede responder sobre la raiz o la
// puerta no puede leer el registro del worktree, TaskDir de legit devuelve
// ErrSinComprobar, no "no existe": con el .git de la raiz movido, con una
// raiz que nunca fue un repo, con el .git de la raiz en 000 y con
// <root>/.git/worktrees/legit en 000. Todo queda como estaba al terminar.
func TestHallazgo6aeb17_CA280_TaskDirSinPoderLeerEsSinComprobar(t *testing.T) {
	t.Run("el .git de la raiz movido", func(t *testing.T) {
		root, _ := t6Proyecto(t)
		if err := os.Rename(filepath.Join(root, ".git"), filepath.Join(filepath.Dir(root), "git-movido")); err != nil {
			t.Fatal(err)
		}
		t6SinComprobar(t, "raiz sin .git", root, "legit")
	})
	t.Run("la raiz nunca fue un repo", func(t *testing.T) {
		base, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIT_CEILING_DIRECTORIES", base)
		root := filepath.Join(base, "carpeta")
		t68Escribir(t, filepath.Join(root, ".hoom", "worktrees", "x"), "x.go", "package x\n")
		t6SinComprobar(t, "raiz que no es un repo", root, "x")
	})
	t.Run("el .git de la raiz en 000", func(t *testing.T) {
		root, _ := t6Proyecto(t)
		t6SinPermisos(t, filepath.Join(root, ".git"))
		t6SinComprobar(t, ".git de la raiz ilegible", root, "legit")
	})
	t.Run("el registro de legit en 000", func(t *testing.T) {
		root, legit := t6Proyecto(t)
		registro := filepath.Join(root, ".git", "worktrees", "legit")
		t6SinPermisos(t, registro)
		t6SinComprobar(t, "<root>/.git/worktrees/legit ilegible", root, "legit")
		if err := os.Chmod(registro, 0o755); err != nil {
			t.Fatal(err)
		}
		t68Acepta(t, "registro legible otra vez", root, "legit", legit)
	})
}

// Guarda del hallazgo 6aeb17. CA-280: con git sano, lo que no es un
// worktree de la raiz sigue siendo "no existe" sin ErrSinComprobar: una
// carpeta comun, una que no esta y un clon. legit sigue andando.
func TestHallazgo6aeb17_CA280_GuardaConGitSanoSigueDiciendoNoExiste(t *testing.T) {
	root, legit := t6Proyecto(t)
	t68Git(t, filepath.Dir(root), "clone", "-q", root, filepath.Join(root, ".hoom", "worktrees", "clon"))
	t6NoEsSinComprobar(t, "carpeta comun", root, "comun", true)
	t6NoEsSinComprobar(t, "carpeta que no esta", root, "nada", true)
	t6NoEsSinComprobar(t, "clon", root, "clon", true)
	t68Acepta(t, "legit con git sano", root, "legit", legit)
}

// Hallazgo 6aeb17 (y 68644c). CA-280: con git sano ninguna carpeta de
// t68Casos es ErrSinComprobar: git responde, y es un "no", no un "no pude"
// (evil-inexistente tambien: su gitdir no existe, pero el registro de la
// raiz se lee). otro, la tarea registrada en la rama de legit, dice que no
// esta en su rama hoom/otro, sin "no existe" ni ErrSinComprobar.
func TestHallazgo6aeb17_CA280_LoQueNoEsLaTareaEsUnNoConGitSano(t *testing.T) {
	root, legit := t6Proyecto(t)
	for _, c := range t68Casos {
		t68Armar(t, root, c.slug, true)
		err := t6NoEsSinComprobar(t, c.que, root, c.slug, false)
		if c.slug == "otro" {
			t68OtroDiceLaRama(t, err)
		}
	}
	t68Acepta(t, "legit con git sano", root, "legit", legit)
}
