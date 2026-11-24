package verifycmd

// Regresion del hallazgo 22a836 (review 26c0ba), con lo que decidio la
// persona el 2026-11-23: verify en .hoom/worktrees/<slug> fuera de su rama
// (ErrOtraRama) sigue siendo esa tarea, con su tarjeta; si git o un Lstat
// no responden (gitx.ErrSinComprobar), la tarea es el slug de la ruta y la
// tarjeta cuenta como ilegible (regla 1: alto); una carpeta que no es
// worktree sigue sin tarea (aclaracion 12).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// h22Riesgo exige la tarea y el riesgo del veredicto.
func h22Riesgo(t *testing.T, ca, caso, dir, tarea, efectivo, motivo string) {
	t.Helper()
	v, _ := retyVerify(t, ca, dir, Options{})
	if v.Tarea != tarea || v.Riesgo == nil || v.Riesgo.Efectivo != efectivo || v.Riesgo.Motivo != motivo {
		t.Fatalf("%s: %s: tarea %q y riesgo %s (%s); fue tarea %q y riesgo %+v", ca, caso, tarea, efectivo, motivo, v.Tarea, v.Riesgo)
	}
}

// 22a836: una tarjeta de tipo seguridad con su worktree en HEAD separado (o
// en otra rama), en el mismo commit: la tarea sigue siendo precios y el
// riesgo, alto por el tipo.
func TestHallazgo22a836_FueraDeSuRamaSigueSiendoLaTarea(t *testing.T) {
	const ca = "hallazgo 22a836"
	root, wt := retyProyecto(t, "", "seguridad", "")
	cdwEscribir(t, filepath.Join(wt, "a.go"), "package app\n\nfunc A() {}\n")
	cdwGit(t, wt, "add", "a.go")
	cdwGit(t, wt, "commit", "-q", "-m", "a")
	h22Riesgo(t, ca, "en hoom/precios", wt, retySlug, riesgo.Alto, riesgo.MotivoSeguridad)

	cdwGit(t, wt, "checkout", "-q", "--detach")
	if _, err := runcmd.TaskDir(root, retySlug); !errors.Is(err, runcmd.ErrOtraRama) {
		t.Fatalf("%s: premisa: con HEAD separado TaskDir da ErrOtraRama: %v", ca, err)
	}
	if slug, sin := runcmd.TareaDe(wt); slug != retySlug || sin {
		t.Fatalf("%s: TareaDe con HEAD separado es la tarea, comprobada: %q %v", ca, slug, sin)
	}
	h22Riesgo(t, ca, "con HEAD separado", wt, retySlug, riesgo.Alto, riesgo.MotivoSeguridad)

	cdwGit(t, wt, "checkout", "-q", "-b", "otra")
	h22Riesgo(t, ca, "en la rama otra", wt, retySlug, riesgo.Alto, riesgo.MotivoSeguridad)
}

// 22a836: si git no puede comprobar el worktree (su registro en
// .git/worktrees/precios/gitdir sin permiso de lectura: TaskDir da
// gitx.ErrSinComprobar), la tarea es precios y la tarjeta, ilegible: alto
// con MotivoIlegible, aunque el item declare bajo.
func TestHallazgo22a836_SinComprobarEsTarjetaIlegible(t *testing.T) {
	const ca = "hallazgo 22a836"
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita la lectura, el test no probaria nada")
	}
	root, wt := retyProyecto(t, "", "feature", "bajo")
	cdwEscribir(t, filepath.Join(wt, "README.md"), "# precios\n")
	cdwGit(t, wt, "add", "README.md")
	cdwGit(t, wt, "commit", "-q", "-m", "docs")
	h22Riesgo(t, ca, "comprobada", wt, retySlug, riesgo.Bajo, "declarado bajo")

	registro := filepath.Join(cdwGit(t, wt, "rev-parse", "--absolute-git-dir"), "gitdir")
	if err := os.Chmod(registro, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(registro, 0o644) })
	if _, err := os.ReadFile(registro); err == nil {
		t.Skip("este sistema deja leer un archivo sin permiso de lectura")
	}
	if _, err := runcmd.TaskDir(root, retySlug); !errors.Is(err, gitx.ErrSinComprobar) {
		t.Fatalf("%s: premisa: con el registro ilegible TaskDir da ErrSinComprobar: %v", ca, err)
	}
	if slug, sin := runcmd.TareaDe(wt); slug != retySlug || !sin {
		t.Fatalf("%s: TareaDe sin poder comprobar es la tarea, sin comprobar: %q %v", ca, slug, sin)
	}
	h22Riesgo(t, ca, "sin poder comprobar", wt, retySlug, riesgo.Alto, riesgo.MotivoIlegible)
}

// Guarda (aclaracion 12): una carpeta comun bajo .hoom/worktrees/ no es una
// tarea, ni con el item de su nombre.
func TestHallazgo22a836_UnaCarpetaQueNoEsWorktreeNoTieneTarea(t *testing.T) {
	const ca = "hallazgo 22a836"
	root, _ := retyProyecto(t, "", "seguridad", "")
	comun := filepath.Join(root, ".hoom", "worktrees", "comun")
	cdwEscribir(t, filepath.Join(comun, "x.go"), "package x\n")
	if slug, sin := runcmd.TareaDe(comun); slug != "" || sin {
		t.Fatalf("%s: una carpeta comun no tiene tarea: %q %v", ca, slug, sin)
	}
	if slug, sin := runcmd.TareaDe(filepath.Join(root, ".hoom", "worktrees", "no-existe")); slug != "" || sin {
		t.Fatalf("%s: una ruta que no existe no tiene tarea: %q %v", ca, slug, sin)
	}
}
