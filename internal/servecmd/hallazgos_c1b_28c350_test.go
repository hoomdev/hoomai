// Tests adversariales del hallazgo 20261020T201503_28c350 (high, risk) de la
// review de los fixes de C1 (.hoom/specs/items-y-columna-derivada.md, CA-280:
// el trabajo de la tarea vive en su worktree .hoom/worktrees/<slug>), sobre
// POST /api/runs del Studio con `task` (CA-26 de
// .hoom/specs/studio-v3-cockpit.md). Escritos desde el contrato "puerta
// completa", nunca desde la implementacion:
//
// POST /api/runs pasa por runcmd.TaskDir, la unica puerta que decide la
// carpeta de una tarea. Con el token y un task que escapa (../../x,
// ../../../victima), que es un symlink a otro repo o a la raiz, o que es una
// carpeta comun que no es un worktree registrado de este repo, responde 4xx,
// no arranca ningun run y el provider no corre. Guarda: con el worktree real
// de taskcmd.Start el run corre en el.
package servecmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const t28NombreVictima = "victima-28c350"

type t28Entorno struct {
	base, root, legit, victima, marca string
}

func (e t28Entorno) wt(slug string) string { return filepath.Join(e.root, ".hoom", "worktrees", slug) }

// t28Proyecto: base/proyecto (con la tarea legit de taskcmd.Start y una
// carpeta x en la raiz, el destino de ../../x), base/victima-28c350 (otro
// repo) y un `claude` falso que anota su cwd fisico en marca.
func t28Proyecto(t *testing.T) t28Entorno {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	if err := os.MkdirAll(filepath.Join(root, ".hoom"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@hoom.dev")
	gitRun(t, root, "config", "user.name", "hoom test")
	if err := os.WriteFile(filepath.Join(root, "hoom.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hoom", ".gitignore"), []byte(hoomfs.GitignoreBody()), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "inicial")
	if err := taskcmd.Start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	victima := filepath.Join(base, t28NombreVictima)
	if err := os.MkdirAll(victima, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, victima, "init", "-b", "main")

	marca := filepath.Join(base, "provider-corrio-en.txt")
	fakeProvider(t, "pwd -P >> '"+marca+"'\nexit 0\n")
	return t28Entorno{base: base, root: root, legit: filepath.Join(root, ".hoom", "worktrees", "legit"), victima: victima, marca: marca}
}

func t28Post(t *testing.T, s *Server, task string) (int, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"provider": "claude", "prompt": "hola", "task": task})
	if err != nil {
		t.Fatal(err)
	}
	rec := doPOST(t, s, "/api/runs", s.Token(), body, "application/json")
	return rec.Code, rec.Body.String()
}

// t28RunRechazado exige 4xx, ningun run en /api/runs y el provider sin
// correr.
func t28RunRechazado(t *testing.T, e t28Entorno, task string) {
	t.Helper()
	s := newServer(t, e.root)
	code, cuerpo := t28Post(t, s, task)
	if code < 400 || code > 499 {
		t.Errorf("CA-280: hallazgo 28c350: POST /api/runs con task %q responde 4xx, respondio %d: %s", task, code, cuerpo)
		var run runcmd.Run
		if json.Unmarshal([]byte(cuerpo), &run) == nil && run.ID != "" {
			pollRun(t, s, run.ID)
		}
	}
	var runs []RunRow
	get(t, s, "/api/runs", &runs)
	if len(runs) != 0 {
		t.Errorf("CA-280: hallazgo 28c350: POST /api/runs con task %q no arranca ningun run, hay %d: %+v", task, len(runs), runs)
	}
	if raw, err := os.ReadFile(e.marca); err == nil {
		t.Errorf("CA-280: hallazgo 28c350: con task %q el provider no corre, corrio en:\n%s", task, raw)
		os.Remove(e.marca)
	}
}

// Hallazgo 28c350 (high). CA-280: POST /api/runs con un task que escapa del
// proyecto (../../x llega a una carpeta de la raiz, ../../../victima a otro
// repo, .. a .hoom) responde 4xx y no lanza nada.
func TestHallazgo28c350_CA280_ApiRunsConTaskQueEscapa(t *testing.T) {
	for _, task := range []string{"../../x", "../../../" + t28NombreVictima, ".."} {
		t.Run(strings.ReplaceAll(task, "/", "_"), func(t *testing.T) {
			t28RunRechazado(t, t28Proyecto(t), task)
		})
	}
}

// Hallazgo 28c350 (high). CA-280: POST /api/runs con un task cuya carpeta es
// un symlink a otro repo, a la raiz o a otra tarea, o una carpeta comun,
// responde 4xx y no lanza nada.
func TestHallazgo28c350_CA280_ApiRunsConCarpetaFalsa(t *testing.T) {
	casos := []struct {
		task    string
		destino func(e t28Entorno) string // "" = carpeta comun
	}{
		{"a-otro-repo", func(e t28Entorno) string { return e.victima }},
		{"a-la-raiz", func(t28Entorno) string { return "../.." }},
		{"a-otra-tarea", func(t28Entorno) string { return "legit" }},
		{"comun", func(t28Entorno) string { return "" }},
	}
	for _, c := range casos {
		t.Run(c.task, func(t *testing.T) {
			e := t28Proyecto(t)
			if d := c.destino(e); d != "" {
				if err := os.Symlink(d, e.wt(c.task)); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(e.wt(c.task), 0o755); err != nil {
				t.Fatal(err)
			}
			t28RunRechazado(t, e, c.task)
		})
	}
}

// Guarda del hallazgo 28c350. CA-26: con el worktree real de
// taskcmd.Start (y un symlink hostil al lado) POST /api/runs con task
// arranca el run y el provider corre en el worktree.
func TestHallazgo28c350_CA26_ApiRunsEnElWorktreeRealSigueAndando(t *testing.T) {
	e := t28Proyecto(t)
	if err := os.Symlink(e.victima, e.wt("a-otro-repo")); err != nil {
		t.Fatal(err)
	}
	s := newServer(t, e.root)
	code, cuerpo := t28Post(t, s, "legit")
	if code != http.StatusOK {
		t.Fatalf("CA-26: guarda 28c350: el run de la tarea real arranca: %d %s", code, cuerpo)
	}
	var run runcmd.Run
	if err := json.Unmarshal([]byte(cuerpo), &run); err != nil {
		t.Fatal(err)
	}
	pollRun(t, s, run.ID)
	raw, err := os.ReadFile(e.marca)
	if err != nil {
		t.Fatalf("CA-26: guarda 28c350: el provider de la tarea real corrio: %v", err)
	}
	real, _ := filepath.EvalSymlinks(e.legit)
	if got := strings.TrimSpace(string(raw)); got != real {
		t.Fatalf("CA-26: guarda 28c350: el run de la tarea real corre en su worktree (%s), corrio en %s", real, got)
	}
}
