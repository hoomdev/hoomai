// Tests adversariales del hallazgo 20261020T201503_28c350 (high, risk) de la
// review de los fixes de C1 (.hoom/specs/items-y-columna-derivada.md, CA-280:
// el trabajo de la tarea vive en su worktree .hoom/worktrees/<slug>), sobre
// `hoom agent --task`. Escritos desde el contrato "puerta completa", nunca
// desde la implementacion:
//
// agentcmd.Run con Task pasa por runcmd.TaskDir, la unica puerta que decide
// la carpeta de una tarea. Con un slug que escapa (../../../victima), con
// .hoom/worktrees/<slug> como symlink a otro repo, o con una carpeta comun
// que no es un worktree registrado de este repo, el sobre se niega con un
// error (como con una tarea que no existe, CA-335) y el provider no corre:
// nada se ejecuta ni se escribe afuera. Guarda: con el worktree real de
// taskcmd.Start el sobre corre en el.
package agentcmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const t28NombreVictima = "victima-28c350"

// t28Entorno: base/proyecto (proyecto hoom con la tarea legit de
// taskcmd.Start) y base/victima-28c350 (otro repo, con trabajo sin
// commitear). marca es donde el provider falso anota su cwd fisico.
type t28Entorno struct {
	base, root, legit, victima, marca string
}

func (e t28Entorno) wt(slug string) string { return filepath.Join(e.root, ".hoom", "worktrees", slug) }

func t28Proyecto(t *testing.T) t28Entorno {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	if err := taskcmd.Start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}

	victima := filepath.Join(base, t28NombreVictima)
	if err := os.MkdirAll(victima, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, victima, "init", "-b", "main")
	git(t, victima, "config", "user.email", "victima@hoom.dev")
	git(t, victima, "config", "user.name", "victima")
	write(t, victima, "hoom.yaml", "schema: hoom/v1\nproject: victima\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, victima, "tracked.go", "package victima\n")
	git(t, victima, "add", "-A")
	git(t, victima, "commit", "-m", "de la victima")
	write(t, victima, "tracked.go", "package victima\n\n// trabajo sin guardar de otra persona\n")

	marca := filepath.Join(base, "provider-corrio-en.txt")
	fakeProvider(t, "claude", "pwd -P >> '"+marca+"'\nexit 0\n")
	return t28Entorno{base: base, root: root, legit: filepath.Join(root, ".hoom", "worktrees", "legit"), victima: victima, marca: marca}
}

// t28ArbolDe lista todo lo que hay bajo dir (fuera de .git) con su contenido.
func t28ArbolDe(t *testing.T, dir string) string {
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

// t28SobreRechazado corre `hoom agent --role scout --task slug` y exige un
// error, que el provider no haya corrido y que la victima siga igual.
func t28SobreRechazado(t *testing.T, e t28Entorno, slug string) {
	t.Helper()
	antes := t28ArbolDe(t, e.victima)
	res, err := Run(e.root, "main", Options{Role: "scout", Provider: "claude", Task: slug, Prompt: "explora"}, io.Discard)
	if err == nil {
		t.Errorf("CA-280: hallazgo 28c350: el sobre con Task %q se niega con un error (como una tarea que no existe), corrio: %+v", slug, res)
	}
	if raw, errLeer := os.ReadFile(e.marca); errLeer == nil {
		t.Errorf("CA-280: hallazgo 28c350: con Task %q el provider no corre, corrio en:\n%s", slug, raw)
		os.Remove(e.marca)
	}
	if despues := t28ArbolDe(t, e.victima); despues != antes {
		t.Errorf("CA-280: hallazgo 28c350: el sobre con Task %q escribio en la victima:\nantes:\n%s\ndespues:\n%s", slug, antes, despues)
	}
	if n := runsCount(t, e.root); n != 0 {
		t.Errorf("CA-280: hallazgo 28c350: el sobre con Task %q no deja runs (%d en .hoom/runs)", slug, n)
	}
}

// Hallazgo 28c350 (high). CA-280: `hoom agent --task ../../../victima` no
// lanza el provider fuera del proyecto (ni en .hoom con ..).
func TestHallazgo28c350_CA280_AgentConSlugQueEscapaNoCorre(t *testing.T) {
	for _, slug := range []string{"../../../" + t28NombreVictima, ".."} {
		t.Run(strings.ReplaceAll(slug, "/", "_"), func(t *testing.T) {
			t28SobreRechazado(t, t28Proyecto(t), slug)
		})
	}
}

// Hallazgo 28c350 (high). CA-280: con .hoom/worktrees/<slug> como symlink a
// otro repo, a la raiz o a otra tarea, o como una carpeta comun, el sobre no
// corre el provider.
func TestHallazgo28c350_CA280_AgentConCarpetaFalsaNoCorre(t *testing.T) {
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
			t28SobreRechazado(t, e, c.slug)
		})
	}
}

// Guarda del hallazgo 28c350. CA-280: con el worktree real de
// taskcmd.Start (y un symlink hostil al lado) el sobre corre en el worktree.
func TestHallazgo28c350_CA280_AgentEnElWorktreeRealSigueAndando(t *testing.T) {
	e := t28Proyecto(t)
	if err := os.Symlink(e.victima, e.wt("a-otro-repo")); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(e.root, "main", Options{Role: "scout", Provider: "claude", Task: "legit", Prompt: "explora"}, io.Discard); err != nil {
		t.Fatalf("CA-280: guarda 28c350: el sobre de la tarea real corre: %v", err)
	}
	raw, err := os.ReadFile(e.marca)
	if err != nil {
		t.Fatalf("CA-280: guarda 28c350: el provider de la tarea real corrio: %v", err)
	}
	real, _ := filepath.EvalSymlinks(e.legit)
	if got := strings.TrimSpace(string(raw)); got != real {
		t.Fatalf("CA-280: guarda 28c350: el sobre de la tarea real corre en su worktree (%s), corrio en %s", real, got)
	}
}
