// Tests adversariales del hallazgo 20261020T201503_28c350 (high, risk) de la
// review de los fixes de C1 (.hoom/specs/items-y-columna-derivada.md: CA-280,
// el trabajo de la tarea vive en su worktree .hoom/worktrees/<slug>; CA-293,
// la review con --task), sobre `hoom review --task`. Escritos desde el
// contrato "puerta completa", nunca desde la implementacion:
//
// reviewcmd.Run con Task pasa por runcmd.TaskDir, la unica puerta que decide
// la carpeta de una tarea. Con un slug que escapa (../../../victima), con
// .hoom/worktrees/<slug> como symlink a otro proyecto hoom, a la raiz o a
// otra tarea, o con una carpeta comun que no es un worktree registrado de
// este repo, la review falla (error, o un Result que no es "revisado" con
// exit distinto de 0), el reviewer no corre y no se escribe nada afuera (ni
// registro de review ni hallazgos en la victima). Guarda: con el worktree
// real de taskcmd.Start la review corre en el.
package reviewcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const t28NombreVictima = "victima-28c350"

// t28Entorno: base/proyecto (proyecto hoom con la tarea legit de
// taskcmd.Start) y base/victima-28c350, OTRO proyecto hoom limpio con un
// cambio de codigo commiteado en su rama: algo que revisar.
type t28Entorno struct {
	base, root, legit, victima, log string
}

func (e t28Entorno) wt(slug string) string { return filepath.Join(e.root, ".hoom", "worktrees", slug) }

func t28ProyectoHoom(t *testing.T, dir, nombre string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@hoom.dev")
	git(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "hoom.yaml", "schema: hoom/v1\nproject: "+nombre+"\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, dir, ".hoom/.gitignore", hoomfs.GitignoreBody())
	write(t, dir, "app.go", "package app\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "inicial")
}

func t28Proyecto(t *testing.T) t28Entorno {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	t28ProyectoHoom(t, root, "demo")
	if err := taskcmd.Start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}

	victima := filepath.Join(base, t28NombreVictima)
	t28ProyectoHoom(t, victima, "victima")
	git(t, victima, "checkout", "-q", "-b", "feature")
	write(t, victima, "app.go", "package app\n\nfunc DeLaVictima() {}\n")
	git(t, victima, "commit", "-q", "-am", "cambio de la victima")
	// limpia a proposito: una victima con cambios sin commitear la
	// protegeria por otra razon (la review revisa solo lo commiteado)

	log := htReviewer(t, "")
	return t28Entorno{base: base, root: root, legit: filepath.Join(root, ".hoom", "worktrees", "legit"), victima: victima, log: log}
}

// t28Arbol lista todo lo que hay bajo dir (fuera de .git) con su contenido.
func t28Arbol(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		raw, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		b.WriteString(rel + "=" + string(raw) + "\n")
		return nil
	})
	return b.String()
}

// t28ReviewRechazada corre `hoom review --task slug` y exige que falle, que
// el reviewer no haya corrido y que la victima siga igual.
func t28ReviewRechazada(t *testing.T, e t28Entorno, slug string) {
	t.Helper()
	antes := t28Arbol(t, e.victima)
	var out bytes.Buffer
	res, err := Run(e.root, "main", Options{Lens: "risk", Provider: "codex", Task: slug}, &out)
	if err == nil && (res.Status == "revisado" || res.ExitCode == 0) {
		t.Errorf("CA-293: hallazgo 28c350: la review con Task %q debe fallar (la carpeta no es un worktree de la tarea), termino %q exit %d:\n%s",
			slug, res.Status, res.ExitCode, out.String())
	}
	if raw, errLeer := os.ReadFile(e.log); errLeer == nil {
		t.Errorf("CA-280: hallazgo 28c350: con Task %q el reviewer no corre, corrio:\n%s", slug, raw)
		os.Remove(e.log)
	}
	if despues := t28Arbol(t, e.victima); despues != antes {
		t.Errorf("CA-280: hallazgo 28c350: la review con Task %q escribio en la victima:\nantes:\n%s\ndespues:\n%s", slug, antes, despues)
	}
}

// Hallazgo 28c350 (high). CA-280 y CA-293: `hoom review --task
// ../../../victima` no revisa otro proyecto (ni .hoom con ..).
func TestHallazgo28c350_CA293_ReviewConSlugQueEscapaNoCorre(t *testing.T) {
	for _, slug := range []string{"../../../" + t28NombreVictima, ".."} {
		t.Run(strings.ReplaceAll(slug, "/", "_"), func(t *testing.T) {
			t28ReviewRechazada(t, t28Proyecto(t), slug)
		})
	}
}

// Hallazgo 28c350 (high). CA-280 y CA-293: con .hoom/worktrees/<slug> como
// symlink a otro proyecto hoom, a la raiz o a otra tarea, o como una
// carpeta comun, la review no corre el reviewer.
func TestHallazgo28c350_CA293_ReviewConCarpetaFalsaNoCorre(t *testing.T) {
	casos := []struct {
		slug  string
		armar func(t *testing.T, e t28Entorno)
	}{
		{"a-otro-repo", func(t *testing.T, e t28Entorno) {
			if err := os.Symlink(e.victima, e.wt("a-otro-repo")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a-la-raiz", func(t *testing.T, e t28Entorno) {
			if err := os.Symlink("../..", e.wt("a-la-raiz")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a-otra-tarea", func(t *testing.T, e t28Entorno) {
			write(t, e.legit, "app.go", "package app\n\nfunc DeLegit() {}\n")
			raCommit(t, e.legit, "cambio en legit")
			if err := os.Symlink("legit", e.wt("a-otra-tarea")); err != nil {
				t.Fatal(err)
			}
		}},
		{"comun", func(t *testing.T, e t28Entorno) {
			write(t, e.wt("comun"), "x.go", "package x\n")
		}},
	}
	for _, c := range casos {
		t.Run(c.slug, func(t *testing.T) {
			e := t28Proyecto(t)
			c.armar(t, e)
			t28ReviewRechazada(t, e, c.slug)
		})
	}
}

// Guarda del hallazgo 28c350. CA-293: con el worktree real de
// taskcmd.Start (y un symlink hostil al lado) la review de la tarea corre en
// el worktree y termina revisado.
func TestHallazgo28c350_CA293_ReviewDelWorktreeRealSigueAndando(t *testing.T) {
	e := t28Proyecto(t)
	if err := os.Symlink(e.victima, e.wt("a-otro-repo")); err != nil {
		t.Fatal(err)
	}
	write(t, e.legit, "app.go", "package app\n\nfunc Nuevo() {}\n")
	raCommit(t, e.legit, "cambio en la tarea")

	htRevisar(t, e.root, Options{Lens: "risk", Provider: "codex", Task: "legit"})
	htTodas(t, htPasadas(t, e.log), 1, "legit", htReal(t, e.legit))
}
