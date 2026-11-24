package verifycmd

// Regresion de los hallazgos 64221a y eaf157 (con 5c7763; review delta
// 0895d1): si git no responde `rev-parse --git-common-dir`,
// gitx.CheckoutPrincipal falla y runcmd.TareaDe no llegaba a la regla 1. En
// .hoom/worktrees/<slug> con un <P>/.git presente, la tarea es el slug y la
// tarjeta, ilegible (alto); fuera de un repositorio sigue sin tarea.

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// h64GitFalso pone primero en el PATH un git que falla (exit 1, sin
// respuesta) cuando sus argumentos traen --git-common-dir y, si conFormato,
// tambien un --path-format=...; lo demas va al git real.
func h64GitFalso(t *testing.T, conFormato bool) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	pide := "1"
	if conFormato {
		pide = "$p"
	}
	script := "#!/bin/sh\nc=0; p=0\nfor a in \"$@\"; do [ \"$a\" = \"--git-common-dir\" ] && c=1; case \"$a\" in --path-format=*) p=1;; esac; done\n" +
		"if [ $c = 1 ] && [ " + pide + " = 1 ]; then echo 'fatal: git simulado sin respuesta' >&2; exit 1; fi\nexec " + real + " \"$@\"\n"
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// 64221a / eaf157: una tarjeta de tipo seguridad en su worktree, con un git
// que no responde --git-common-dir (el de CheckoutPrincipal, con
// --path-format, o cualquiera): TareaDe da (precios, true) y el veredicto
// sale con la tarea precios, alto y MotivoIlegible.
func TestHallazgo64221a_GitSinCommonDirEsTarjetaIlegible(t *testing.T) {
	const ca = "hallazgo 64221a"
	for _, k := range []struct {
		caso       string
		conFormato bool
	}{{"falla --path-format=absolute --git-common-dir", true}, {"falla todo --git-common-dir", false}} {
		t.Run(k.caso, func(t *testing.T) {
			_, wt := retyProyecto(t, "", "seguridad", "")
			cdwEscribir(t, filepath.Join(wt, "a.go"), "package app\n\nfunc A() {}\n")
			cdwGit(t, wt, "add", "a.go")
			cdwGit(t, wt, "commit", "-q", "-m", "a")
			h64GitFalso(t, k.conFormato)
			if _, ok := gitx.CheckoutPrincipal(wt); ok {
				t.Fatalf("%s: %s: premisa: CheckoutPrincipal falla", ca, k.caso)
			}
			if slug, sin := runcmd.TareaDe(wt); slug != retySlug || !sin {
				t.Fatalf("%s: %s: TareaDe es (%q, true), fue (%q, %v)", ca, k.caso, retySlug, slug, sin)
			}
			h22Riesgo(t, ca, k.caso, wt, retySlug, riesgo.Alto, riesgo.MotivoIlegible)
		})
	}
}

// Guardas de 64221a: fuera de un repositorio (sin <P>/.git), una ruta con
// la forma de un worktree no es una tarea; tampoco un slug invalido con git
// sin responder.
func TestHallazgo64221a_SinRepoOSlugInvalidoNoTienenTarea(t *testing.T) {
	const ca = "hallazgo 64221a"
	afuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(afuera))
	y := filepath.Join(afuera, "x", ".hoom", "worktrees", "y")
	cdwEscribir(t, filepath.Join(y, "a.go"), "package app\n")
	if slug, sin := runcmd.TareaDe(y); slug != "" || sin {
		t.Fatalf("%s: x/.hoom/worktrees/y fuera de un repo no tiene tarea: (%q, %v)", ca, slug, sin)
	}

	root, _ := retyProyecto(t, "", "seguridad", "")
	malo := filepath.Join(root, ".hoom", "worktrees", "No_Valido")
	cdwEscribir(t, filepath.Join(malo, "a.go"), "package app\n")
	h64GitFalso(t, false)
	if slug, sin := runcmd.TareaDe(malo); slug != "" || sin {
		t.Fatalf("%s: un slug invalido no es una tarea: (%q, %v)", ca, slug, sin)
	}
	if slug, sin := runcmd.TareaDe(y); slug != "" || sin {
		t.Fatalf("%s: fuera de un repo, con git sin responder, no tiene tarea: (%q, %v)", ca, slug, sin)
	}
}
