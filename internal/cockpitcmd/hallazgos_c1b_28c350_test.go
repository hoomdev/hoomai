// Tests adversariales del hallazgo 20261020T201503_28c350 (high, risk) de la
// review de los fixes de C1 (.hoom/specs/items-y-columna-derivada.md, CA-280:
// el trabajo de la tarea vive en su worktree .hoom/worktrees/<slug>), sobre
// `hoom cockpit --task` (CA-85 de .hoom/specs/cockpit-tmux-zellij.md).
// Escritos desde el contrato "puerta completa", nunca desde la
// implementacion:
//
// cockpitcmd.Open y cockpitcmd.Run con Task pasan por runcmd.TaskDir, la
// unica puerta que decide la carpeta de una tarea. Con un slug que escapa
// (../../../victima, ..), con .hoom/worktrees/<slug> como symlink a otro
// repo, a la raiz o a otra tarea, o con una carpeta comun que no es un
// worktree registrado de este repo, fallan sin lanzar nada: ni
// new-session/split-window/attach de tmux, ni la sesion de zellij, ni un
// solo comando dentro de la victima. Guarda: con el worktree real de
// taskcmd.Start la sesion se monta en el. Todo contra un tmux falso con
// estado (h3Tmux): ningun multiplexor real.
package cockpitcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const t28NombreVictima = "victima-28c350"

// t28Lanzan son los verbos de tmux que crean o muestran algo: ninguno puede
// aparecer cuando la tarea se rechaza.
var t28Lanzan = []string{"tmux new-session", "tmux split-window", "tmux attach", "tmux switch-client", "tmux new-window", "tmux send-keys", "tmux respawn"}

type t28Entorno struct {
	base, root, legit, victima string
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
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@hoom.dev")
	gitRun(t, root, "config", "user.name", "hoom test")
	for rel, body := range map[string]string{
		"hoom.yaml":        "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n",
		".hoom/.gitignore": hoomfs.GitignoreBody(),
		"app.go":           "package app\n",
	} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "inicial")
	if err := taskcmd.Start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}
	victima := filepath.Join(base, t28NombreVictima)
	if err := os.MkdirAll(victima, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, victima, "init", "-q", "-b", "main")
	return t28Entorno{base: base, root: root, legit: filepath.Join(root, ".hoom", "worktrees", "legit"), victima: victima}
}

// t28SinLanzar exige que el tmux falso no haya visto ningun verbo que lance
// algo ni ningun comando que nombre la victima.
func t28SinLanzar(t *testing.T, como, slug string, f *h3Tmux) {
	t.Helper()
	todas := f.todas()
	for _, l := range t28Lanzan {
		if f.indice(l, 0) >= 0 {
			t.Errorf("CA-280: hallazgo 28c350: %s con Task %q no lanza nada, y corrio %q:\n%s", como, slug, l, todas)
			return
		}
	}
	if f.indice("zellij", 0) >= 0 && strings.Contains(todas, "--session") {
		t.Errorf("CA-280: hallazgo 28c350: %s con Task %q no lanza la sesion de zellij:\n%s", como, slug, todas)
		return
	}
	if strings.Contains(todas, t28NombreVictima) {
		t.Errorf("CA-280: hallazgo 28c350: %s con Task %q no corre nada que nombre la victima:\n%s", como, slug, todas)
	}
}

// t28CockpitRechazado prueba Open (tmux) y Run (tmux y zellij) con slug.
func t28CockpitRechazado(t *testing.T, e t28Entorno, slug string) {
	t.Helper()
	f := &h3Tmux{}
	if _, err := Open(e.root, "demo", Options{Provider: "claude", Task: slug, Mux: "tmux"}, f.deps()); err == nil {
		t.Errorf("CA-280: hallazgo 28c350: Open con Task %q debe fallar:\n%s", slug, f.todas())
	}
	t28SinLanzar(t, "Open", slug, f)

	for _, mux := range []string{"tmux", "zellij"} {
		f := &h3Tmux{}
		if err := Run(e.root, "demo", Options{Provider: "claude", Task: slug, Mux: mux}, f.deps()); err == nil {
			t.Errorf("CA-280: hallazgo 28c350: Run (%s) con Task %q debe fallar:\n%s", mux, slug, f.todas())
		}
		t28SinLanzar(t, "Run ("+mux+")", slug, f)
	}
	if entradas, _ := os.ReadDir(e.victima); len(entradas) != 1 { // solo .git
		var nombres []string
		for _, en := range entradas {
			nombres = append(nombres, en.Name())
		}
		t.Errorf("CA-280: hallazgo 28c350: el cockpit con Task %q no escribe en la victima: %v", slug, nombres)
	}
}

// Hallazgo 28c350 (high). CA-280 y CA-85: `hoom cockpit --task
// ../../../victima` (y ..) no monta la sesion fuera del proyecto.
func TestHallazgo28c350_CA280_CockpitConSlugQueEscapaNoLanza(t *testing.T) {
	for _, slug := range []string{"../../../" + t28NombreVictima, ".."} {
		t.Run(strings.ReplaceAll(slug, "/", "_"), func(t *testing.T) {
			t28CockpitRechazado(t, t28Proyecto(t), slug)
		})
	}
}

// Hallazgo 28c350 (high). CA-280 y CA-85: con .hoom/worktrees/<slug> como
// symlink a otro repo, a la raiz o a otra tarea, o como una carpeta comun,
// el cockpit no lanza nada.
func TestHallazgo28c350_CA280_CockpitConCarpetaFalsaNoLanza(t *testing.T) {
	casos := []struct {
		slug    string
		destino func(e t28Entorno) string // "" = carpeta comun
	}{
		{"a-otro-repo", func(e t28Entorno) string { return e.victima }},
		{"a-la-raiz", func(t28Entorno) string { return "../.." }},
		{"a-otra-tarea", func(t28Entorno) string { return "legit" }},
		{"comun", func(t28Entorno) string { return "" }},
	}
	for _, c := range casos {
		t.Run(c.slug, func(t *testing.T) {
			e := t28Proyecto(t)
			if d := c.destino(e); d != "" {
				if err := os.Symlink(d, e.wt(c.slug)); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(e.wt(c.slug), 0o755); err != nil {
				t.Fatal(err)
			}
			t28CockpitRechazado(t, e, c.slug)
		})
	}
}

// t28NuevaSesionEn: hubo un `tmux new-session` de hoom-demo-legit con
// `-c dir` (sin depender del orden del resto de los flags).
func t28NuevaSesionEn(f *h3Tmux, dir string) bool {
	for _, l := range strings.Split(f.todas(), "\n") {
		if strings.HasPrefix(l, "tmux new-session") && strings.Contains(l, "hoom-demo-legit") &&
			(strings.Contains(l, "-c "+dir+" ") || strings.HasSuffix(l, "-c "+dir)) {
			return true
		}
	}
	return false
}

// Guarda del hallazgo 28c350. CA-85: con el worktree real de taskcmd.Start
// (y un symlink hostil al lado) Open y Run montan la sesion en el worktree.
func TestHallazgo28c350_CA85_CockpitEnElWorktreeRealSigueAndando(t *testing.T) {
	e := t28Proyecto(t)
	if err := os.Symlink(e.victima, e.wt("a-otro-repo")); err != nil {
		t.Fatal(err)
	}
	f := &h3Tmux{}
	s, err := Open(e.root, "demo", Options{Provider: "claude", Task: "legit", Mux: "tmux"}, f.deps())
	if err != nil {
		t.Fatalf("CA-85: guarda 28c350: Open de la tarea real funciona: %v\n%s", err, f.todas())
	}
	if !s.Created || s.Name != "hoom-demo-legit" {
		t.Fatalf("CA-85: guarda 28c350: Open crea la sesion de la tarea real: %+v", s)
	}
	if !t28NuevaSesionEn(f, e.legit) {
		t.Fatalf("CA-85: guarda 28c350: la sesion vive en el worktree real (%s):\n%s", e.legit, f.todas())
	}

	f = &h3Tmux{}
	if err := Run(e.root, "demo", Options{Provider: "claude", Task: "legit", Mux: "tmux"}, f.deps()); err != nil {
		t.Fatalf("CA-85: guarda 28c350: Run de la tarea real funciona: %v\n%s", err, f.todas())
	}
	if !t28NuevaSesionEn(f, e.legit) {
		t.Fatalf("CA-85: guarda 28c350: Run monta la sesion en el worktree real (%s):\n%s", e.legit, f.todas())
	}
}
