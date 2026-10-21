// Tests adversariales de los hallazgos 20261021T174140_48eeed (medium,
// reliability) y 20261021T174455_44ed99 (medium, resilience) de la delta 1
// de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md, CA-280).
// Escritos desde el contrato, nunca desde la implementacion:
//
// Si la puerta no puede leer <carpeta>/.git (la carpeta de la tarea sin
// permiso de busqueda, o su .git sin permiso de lectura), no sabe si es la
// tarea: runcmd.TaskDir devuelve un error que envuelve gitx.ErrSinComprobar y
// que no dice "no existe". Solo un .git que no existe es un "no". Guarda: un
// .git que no existe (una carpeta comun, o un worktree registrado al que se
// le borro el .git) sigue dando "no existe" sin ErrSinComprobar, y con los
// permisos devueltos legit vuelve a ser su tarea.
//
// Lo mismo un nivel mas arriba (el camino que vio el refutador en f12cbb0):
// con .hoom/worktrees cerrada (0600: se listan los nombres pero no se
// alcanzan; o 0000), TaskDir de legit y de una carpeta comun da
// ErrSinComprobar y no "no existe".
//
// Usa internal/worktreetest (Cerrar, ModosCerrados), t6Proyecto, t6SinPermisos, t6SinComprobar y t6NoEsSinComprobar del
// archivo de 6aeb17, y t68Acepta del de 68644c.
package runcmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// t48SinLectura deja el archivo p en 000 y lo devuelve a 0644 al terminar.
// Saltea si corre como root o si el sistema deja leerlo igual.
func t48SinLectura(t *testing.T, p string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita la lectura, el test no probaria nada")
	}
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := os.ReadFile(p); err == nil {
		t.Skip("este sistema deja leer un archivo sin permiso de lectura")
	}
}

// Hallazgos 48eeed y 44ed99. CA-280: con la carpeta de la tarea en 000
// (legit, un worktree registrado, y comun, una carpeta cualquiera) o con el
// .git de legit en 000, TaskDir no puede decir "no existe": devuelve
// ErrSinComprobar. Con los permisos devueltos, legit vuelve a ser su tarea.
func TestHallazgo48eeed_CA280_TaskDirSinPoderLeerElGitDeLaCarpetaEsSinComprobar(t *testing.T) {
	t.Run("la carpeta de legit en 000", func(t *testing.T) {
		root, legit := t6Proyecto(t)
		t6SinPermisos(t, legit)
		t6SinComprobar(t, ".hoom/worktrees/legit sin permiso de busqueda", root, "legit")
		if err := os.Chmod(legit, 0o755); err != nil {
			t.Fatal(err)
		}
		t68Acepta(t, "carpeta legible otra vez", root, "legit", legit)
	})
	t.Run("una carpeta comun en 000", func(t *testing.T) {
		root, _ := t6Proyecto(t)
		t6SinPermisos(t, filepath.Join(root, ".hoom", "worktrees", "comun"))
		t6SinComprobar(t, ".hoom/worktrees/comun sin permiso de busqueda", root, "comun")
	})
	t.Run("el .git de legit en 000", func(t *testing.T) {
		root, legit := t6Proyecto(t)
		gitArchivo := filepath.Join(legit, ".git")
		t48SinLectura(t, gitArchivo)
		t6SinComprobar(t, ".hoom/worktrees/legit/.git ilegible", root, "legit")
		if err := os.Chmod(gitArchivo, 0o644); err != nil {
			t.Fatal(err)
		}
		t68Acepta(t, ".git legible otra vez", root, "legit", legit)
	})
}

// Guarda de los hallazgos 48eeed y 44ed99. CA-280: un .git que no existe es
// un "no": una carpeta comun y un worktree registrado sin su .git dicen "no
// existe" sin ErrSinComprobar.
func TestHallazgo48eeed_CA280_GuardaSinGitSigueDiciendoNoExiste(t *testing.T) {
	root, legit := t6Proyecto(t)
	t6NoEsSinComprobar(t, "carpeta comun sin .git", root, "comun", true)
	if err := os.Remove(filepath.Join(legit, ".git")); err != nil {
		t.Fatal(err)
	}
	t6NoEsSinComprobar(t, "worktree registrado sin su .git", root, "legit", true)
}

// Hallazgos 48eeed y 44ed99. CA-280: con .hoom/worktrees cerrada (0600 y
// 0000), la puerta no puede leer el .git de ninguna carpeta: TaskDir de legit
// y de comun da ErrSinComprobar, no "no existe". Abierta otra vez, legit
// vuelve a ser su tarea.
func TestHallazgo48eeed_CA280_TaskDirConWorktreesCerradaEsSinComprobar(t *testing.T) {
	for _, modo := range worktreetest.ModosCerrados {
		modo := modo
		t.Run(fmt.Sprintf("%04o", modo), func(t *testing.T) {
			root, legit := t6Proyecto(t)
			abrir := worktreetest.Cerrar(t, filepath.Join(root, ".hoom", "worktrees"), "legit", modo)
			t6SinComprobar(t, fmt.Sprintf(".hoom/worktrees en %04o, legit", modo), root, "legit")
			t6SinComprobar(t, fmt.Sprintf(".hoom/worktrees en %04o, carpeta comun", modo), root, "comun")
			abrir()
			t68Acepta(t, ".hoom/worktrees abierta otra vez", root, "legit", legit)
		})
	}
}
