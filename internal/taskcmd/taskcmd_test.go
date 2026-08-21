// Tests adversariales del spec .hoom/specs/studio-v1-dashboard.md.
// Escritos desde los criterios de aceptacion (CA-n), no desde la
// implementacion: cada test referencia el criterio que ata.
package taskcmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	return root
}

func writeVerdict(t *testing.T, dir string, at time.Time, status string, base string) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{
		Project:   "demo",
		CreatedAt: at,
		Git:       gitx.Snapshot(dir, base),
		Gates: []verdict.GateResult{
			{Name: "test", Required: true, Status: status, OutputTail: "tail del gate test"},
		},
	}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

// CA-1: sin tareas, el JSON en stdout es exactamente [] (nunca null).
func TestCA1_JSONVacioSinTareas(t *testing.T) {
	root := initRepo(t)
	raw, err := JSONBytes(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("CA-1: sin tareas debe emitir [], obtuve %q", raw)
	}
}

// CA-1: el snapshot expone slug, rama y estado por tarea, y el estado
// recorre no-verdict -> green -> drift -> red segun la evidencia real.
func TestCA1_EstadosDeTarea(t *testing.T) {
	root := initRepo(t)
	if err := Start(root, "demo", "main"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", "demo")

	snap := func() TaskInfo {
		tasks, err := Snapshot(root, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 {
			t.Fatalf("esperaba 1 tarea, hay %d", len(tasks))
		}
		return tasks[0]
	}

	ti := snap()
	if ti.Slug != "demo" || ti.Branch != "hoom/demo" {
		t.Fatalf("CA-1: slug/branch incorrectos: %+v", ti)
	}
	if ti.State != StateNoVerdict {
		t.Fatalf("CA-1: sin veredicto el estado debe ser %q, es %q", StateNoVerdict, ti.State)
	}
	if ti.Dirty {
		t.Fatalf("CA-1: worktree recien creado no puede estar dirty")
	}

	// arbol sucio: un archivo sin commitear se declara.
	if err := os.WriteFile(filepath.Join(wt, "foo.txt"), []byte("uno\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ti = snap(); !ti.Dirty {
		t.Fatalf("CA-1: con cambios sin commitear, dirty debe ser true")
	}

	// veredicto verde con la huella actual => green.
	green := writeVerdict(t, wt, time.Now(), verdict.StatusPass, "main")
	if ti = snap(); ti.State != StateGreen {
		t.Fatalf("CA-1: con veredicto verde y huella coincidente esperaba %q, es %q", StateGreen, ti.State)
	}
	if ti.VerdictID != green.ID {
		t.Fatalf("CA-1: verdict_id debe ser %q, es %q", green.ID, ti.VerdictID)
	}

	// editar despues del verde => drift.
	if err := os.WriteFile(filepath.Join(wt, "foo.txt"), []byte("dos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ti = snap(); ti.State != StateDrift {
		t.Fatalf("CA-1: tras editar esperaba %q, es %q", StateDrift, ti.State)
	}

	// ultimo veredicto rojo => red, con su id.
	red := writeVerdict(t, wt, time.Now().Add(time.Minute), verdict.StatusFail, "main")
	if ti = snap(); ti.State != StateRed || ti.VerdictID != red.ID {
		t.Fatalf("CA-1: esperaba %q con id %q, obtuve %q/%q", StateRed, red.ID, ti.State, ti.VerdictID)
	}
}

// CA-1: la salida JSON decodifica a la estructura contractual
// {slug, branch, state, verdict_id, dirty}.
func TestCA1_EstructuraJSON(t *testing.T) {
	root := initRepo(t)
	if err := Start(root, "otra", "main"); err != nil {
		t.Fatal(err)
	}
	raw, err := JSONBytes(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("CA-1: el JSON no decodifica: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("esperaba 1 tarea, hay %d", len(decoded))
	}
	for _, key := range []string{"slug", "branch", "state", "dirty"} {
		if _, ok := decoded[0][key]; !ok {
			t.Fatalf("CA-1: falta el campo %q en %v", key, decoded[0])
		}
	}
}
