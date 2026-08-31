// Tests adversariales del spec .hoom/specs/gates-ausentes-parciales-verifica.md
// (CA-66): el estado de una tarea usa la misma regla de referencia que
// hoom check — el ultimo veredicto COMPLETO del worktree; los parciales de
// --gate no cuentan.
package taskcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func initWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@hoom.dev")
	gitRun(t, root, "config", "user.name", "hoom test")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "inicial")
	return root
}

func writeTaskVerdict(t *testing.T, wt string, at time.Time, status string, partial bool) *verdict.Verdict {
	t.Helper()
	gates := []verdict.GateResult{{Name: "test", Required: true, Status: status}}
	if partial {
		gates = append(gates, verdict.GateResult{Name: "build", Required: true, Status: verdict.StatusSkipped})
	}
	v := &verdict.Verdict{
		Project:   "demo",
		CreatedAt: at,
		Partial:   partial,
		Git:       gitx.Snapshot(wt, "main"),
		Gates:     gates,
	}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	return v
}

// CA-66: con solo un veredicto parcial la tarea sigue SIN-VEREDICTO.
func TestCA66_SoloParcialEsSinVeredicto(t *testing.T) {
	wt := initWorktree(t)
	writeTaskVerdict(t, wt, time.Now(), verdict.StatusPass, true)
	state, id := taskState(wt, "main")
	if state != StateNoVerdict || id != "" {
		t.Fatalf("CA-66: un parcial no es referencia de tarea: state=%q id=%q", state, id)
	}
}

// CA-66: un parcial rojo posterior no pisa al completo verde de la tarea.
func TestCA66_ParcialNoPisaCompletoVerde(t *testing.T) {
	wt := initWorktree(t)
	green := writeTaskVerdict(t, wt, time.Now(), verdict.StatusPass, false)
	writeTaskVerdict(t, wt, time.Now().Add(time.Minute), verdict.StatusFail, true)
	state, id := taskState(wt, "main")
	if state != StateGreen || id != green.ID {
		t.Fatalf("CA-66: la referencia debe ser el completo verde %q: state=%q id=%q", green.ID, state, id)
	}
}
