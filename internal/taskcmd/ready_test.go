// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-277): taskcmd.Ready es EXACTAMENTE el chequeo que hace 'hoom task
// done' sin --force, con los mismos mensajes. El tablero no puede decir
// "listo para cerrar" de algo que task done rechaza.
package taskcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func rdGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func rdEscribir(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rdTarea arma un proyecto con la tarea slug creada como la crea 'task
// start' (rama hoom/<slug>, worktree en .hoom/worktrees/<slug>).
func rdTarea(t *testing.T, slug string) (root, wt string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rdGit(t, root, "init", "-b", "main")
	rdGit(t, root, "config", "user.email", "test@hoom.dev")
	rdGit(t, root, "config", "user.name", "hoom test")
	rdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	rdEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	rdEscribir(t, root, "app.go", "package app\n")
	rdGit(t, root, "add", "-A")
	rdGit(t, root, "commit", "-q", "-m", "inicial")
	wt = filepath.Join(root, ".hoom", "worktrees", slug)
	rdGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	return root, wt
}

func rdVeredicto(t *testing.T, wt string, at time.Time, status string, partial bool, huella string) *verdict.Verdict {
	t.Helper()
	g := gitx.Snapshot(wt, "main")
	if huella != "" {
		g.ChangeFingerprint = huella
	}
	gates := []verdict.GateResult{{Name: "test", Required: true, Status: status}}
	v := &verdict.Verdict{Project: "demo", CreatedAt: at, Partial: partial, Git: g, Gates: gates}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func rdCommit(t *testing.T, wt, msg string) {
	t.Helper()
	rdGit(t, wt, "add", "-A")
	rdGit(t, wt, "commit", "-q", "-m", msg)
}

// rdIgual exige que Ready y Done (sin --force) den el MISMO error, y que ese
// error diga lo esperado.
func rdIgual(t *testing.T, root, slug, contiene string) {
	t.Helper()
	rerr := Ready(root, slug, "main")
	if rerr == nil {
		t.Fatalf("CA-277: Ready debe rechazar (%q), fue nil", contiene)
	}
	if !strings.Contains(rerr.Error(), contiene) {
		t.Fatalf("CA-277: el mensaje de Ready debe contener %q: %v", contiene, rerr)
	}
	derr := Done(root, slug, "main", false)
	if derr == nil {
		t.Fatalf("CA-277: task done debe rechazar lo que Ready rechaza (%q)", contiene)
	}
	if derr.Error() != rerr.Error() {
		t.Fatalf("CA-277: Ready y task done dan el mismo mensaje:\nReady: %v\nDone:  %v", rerr, derr)
	}
}

// CA-277: cada condicion de cierre, con el mensaje de siempre, en Ready y en
// Done por igual; y con todo en orden, Ready da nil y Done cierra.
func TestCA277_ReadyEsElChequeoDeTaskDone(t *testing.T) {
	const slug = "precios"
	root, wt := rdTarea(t, slug)

	rdIgual(t, root, "no-existe", `la tarea "no-existe" no existe (mira 'hoom task list')`)

	rdEscribir(t, wt, "precios.go", "package app\n")
	rdIgual(t, root, slug, "tiene cambios sin commitear")
	rdCommit(t, wt, "codigo")

	rdIgual(t, root, slug, "no tiene veredictos")

	rdVeredicto(t, wt, time.Now().UTC(), verdict.StatusPass, true, "")
	rdCommit(t, wt, "parcial")
	rdIgual(t, root, slug, "solo tiene veredictos PARCIALES")

	rojo := rdVeredicto(t, wt, time.Now().UTC().Add(time.Second), verdict.StatusFail, false, "")
	rdCommit(t, wt, "rojo")
	rdIgual(t, root, slug, "es ROJO ("+rojo.ID+")")

	rdVeredicto(t, wt, time.Now().UTC().Add(2*time.Second), verdict.StatusPass, false, "otra-huella")
	rdCommit(t, wt, "verde de otra huella")
	rdIgual(t, root, slug, "cambio despues del ultimo veredicto verde")

	rdVeredicto(t, wt, time.Now().UTC().Add(3*time.Second), verdict.StatusPass, false, "")
	rdCommit(t, wt, "verde")
	if err := Ready(root, slug, "main"); err != nil {
		t.Fatalf("CA-277: limpio, verde y con la huella: Ready es nil, fue %v", err)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("CA-277: Ready no toca el worktree: %v", err)
	}
	if err := Done(root, slug, "main", false); err != nil {
		t.Fatalf("CA-277: lo que Ready aprueba, task done lo cierra: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("CA-277: task done quito el worktree (err=%v)", err)
	}
}

// CA-277: Ready es de solo lectura: no toca el worktree ni el arbol.
func TestCA277_ReadySoloLee(t *testing.T) {
	root, wt := rdTarea(t, "precios")
	rdEscribir(t, wt, "sucio.go", "package app\n")
	antes := rdGit(t, wt, "status", "--porcelain", "--untracked-files=all")
	antesRaiz := rdGit(t, root, "status", "--porcelain", "--untracked-files=all", "--ignored")
	for i := 0; i < 3; i++ {
		_ = Ready(root, "precios", "main")
	}
	if got := rdGit(t, wt, "status", "--porcelain", "--untracked-files=all"); got != antes {
		t.Fatalf("CA-277: Ready no cambia el worktree:\nantes: %s\nahora: %s", antes, got)
	}
	if got := rdGit(t, root, "status", "--porcelain", "--untracked-files=all", "--ignored"); got != antesRaiz {
		t.Fatalf("CA-277: Ready no cambia el arbol:\nantes: %s\nahora: %s", antesRaiz, got)
	}
	if Ready(root, "precios", "main") == nil {
		t.Fatal("CA-277: con el worktree sucio Ready rechaza")
	}
}
