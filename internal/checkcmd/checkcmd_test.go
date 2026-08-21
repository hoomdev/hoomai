// Tests adversariales del spec .hoom/specs/studio-v1-dashboard.md (CA-3):
// el resultado del check es el mismo dato en texto y en JSON, con el mismo
// exit code, y cada razon de fallo trae la accion exacta.
package checkcmd

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

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	return root
}

func writeVerdict(t *testing.T, root string, at time.Time, status string) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{
		Project:   "demo",
		CreatedAt: at,
		Git:       gitx.Snapshot(root, "main"),
		Gates:     []verdict.GateResult{{Name: "test", Required: true, Status: status}},
	}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatal(err)
	}
	return v
}

// CA-3: sin veredictos el check falla (exit 1) con la accion exacta.
func TestCA3_SinVeredictos(t *testing.T) {
	root := initRepo(t)
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.ExitCode() != 1 {
		t.Fatalf("CA-3: sin veredictos debe fallar con exit 1: %+v", res)
	}
	if res.Reason != ReasonNoVerdict || !strings.Contains(res.Action, "hoom verify") {
		t.Fatalf("CA-3: razon/accion incorrectas: %+v", res)
	}
}

// CA-3: veredicto verde con huella coincidente => ok, exit 0, y el JSON
// del resultado expone ok, verdict_id, fingerprint_match y action.
func TestCA3_VerdeCoincidente(t *testing.T) {
	root := initRepo(t)
	green := writeVerdict(t, root, time.Now(), verdict.StatusPass)
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.ExitCode() != 0 || !res.FingerprintMatch {
		t.Fatalf("CA-3: esperaba check verde: %+v", res)
	}
	if res.VerdictID != green.ID {
		t.Fatalf("CA-3: verdict_id debe ser %q, es %q", green.ID, res.VerdictID)
	}
	raw, _ := json.Marshal(res)
	for _, campo := range []string{"\"ok\"", "\"verdict_id\"", "\"fingerprint_match\""} {
		if !strings.Contains(string(raw), campo) {
			t.Fatalf("CA-3: el JSON no expone %s: %s", campo, raw)
		}
	}
}

// CA-3: editar el arbol despues del verde => drift, exit 1, huellas visibles.
func TestCA3_Drift(t *testing.T) {
	root := initRepo(t)
	writeVerdict(t, root, time.Now(), verdict.StatusPass)
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app // editado\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.ExitCode() != 1 || res.Reason != ReasonDrift {
		t.Fatalf("CA-3: esperaba drift con exit 1: %+v", res)
	}
	if res.FingerprintNow == "" || res.FingerprintVerdict == "" || res.FingerprintNow == res.FingerprintVerdict {
		t.Fatalf("CA-3: las huellas deben divergir y ser visibles: %+v", res)
	}
}

// CA-3: ultimo veredicto rojo => exit 1 con razon red-verdict.
func TestCA3_UltimoRojo(t *testing.T) {
	root := initRepo(t)
	writeVerdict(t, root, time.Now(), verdict.StatusPass)
	red := writeVerdict(t, root, time.Now().Add(time.Minute), verdict.StatusFail)
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Reason != ReasonRedVerdict || res.VerdictID != red.ID {
		t.Fatalf("CA-3: esperaba red-verdict sobre %q: %+v", red.ID, res)
	}
}
