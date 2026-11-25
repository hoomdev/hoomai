// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre `hoom
// check` (CA-814, aclaracion 21): check no cambia y acepta no_aplica: con un
// ultimo veredicto completo, verde, con no_aplica y la huella actual, el
// check es OK con exit 0.
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package checkcmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func gprGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// CA-814: hoom check acepta un ultimo veredicto completo y verde con
// no_aplica y la huella actual: OK, exit 0, sin razon de fallo.
func TestCA814_CheckAceptaNoAplica(t *testing.T) {
	const ca = "CA-814"
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gprGit(t, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	gprGit(t, root, "config", "user.email", "test@hoom.dev")
	gprGit(t, root, "config", "user.name", "hoom test")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gprGit(t, root, "add", "-A")
	gprGit(t, root, "commit", "-q", "-m", "inicial")

	motivo := fmt.Sprintf(gates.MotivoNoAplica, "app/**")
	v := &verdict.Verdict{Schema: verdict.SchemaID, Project: "demo", CreatedAt: time.Now().UTC(), Git: gitx.Snapshot(root, "main"),
		Gates: []verdict.GateResult{
			{Name: "test", Required: true, Status: verdict.StatusPass, Scope: "full", Command: "true"},
			{Name: "static", Required: true, Status: verdict.StatusNoAplica, Scope: "none", Notes: motivo},
			{Name: "mutation", Required: true, Status: verdict.StatusNoAplica, Scope: "none", Notes: motivo},
		}}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatal(err)
	}

	res, err := checkcmd.Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.ExitCode() != 0 || res.Reason != "" || res.LastVerdict != "green" || !res.FingerprintMatch || res.VerdictID != v.ID {
		t.Fatalf("%s: check acepta el veredicto con no_aplica: OK, exit 0, verde y con la huella: %+v", ca, res)
	}
}
