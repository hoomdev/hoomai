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

// writePartialVerdict emite un veredicto de corrida --gate: un gate corrido,
// el resto skipped y la marca partial (spec gates-ausentes-parciales-verifica).
func writePartialVerdict(t *testing.T, root string, at time.Time, status string) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{
		Project:   "demo",
		CreatedAt: at,
		Partial:   true,
		Git:       gitx.Snapshot(root, "main"),
		Gates: []verdict.GateResult{
			{Name: "test", Required: true, Status: status},
			{Name: "build", Required: true, Status: verdict.StatusSkipped},
		},
	}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatal(err)
	}
	return v
}

// CA-64: un parcial ROJO posterior no pisa al completo verde: check sigue OK.
func TestCA64_ParcialRojoNoPisaVerdeCompleto(t *testing.T) {
	root := initRepo(t)
	green := writeVerdict(t, root, time.Now(), verdict.StatusPass)
	writePartialVerdict(t, root, time.Now().Add(time.Minute), verdict.StatusFail)
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.VerdictID != green.ID {
		t.Fatalf("CA-64: la referencia debe ser el completo verde %q: %+v", green.ID, res)
	}
}

// CA-64: un parcial VERDE posterior tampoco pisa al completo rojo: check ROJO.
func TestCA64_ParcialVerdeNoPisaRojoCompleto(t *testing.T) {
	root := initRepo(t)
	red := writeVerdict(t, root, time.Now(), verdict.StatusFail)
	writePartialVerdict(t, root, time.Now().Add(time.Minute), verdict.StatusPass)
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Reason != ReasonRedVerdict || res.VerdictID != red.ID {
		t.Fatalf("CA-64: la referencia debe ser el completo rojo %q: %+v", red.ID, res)
	}
}

// CA-65: con solo veredictos parciales, check es ROJO con razon
// solo-parciales y la accion exacta (verify completo).
func TestCA65_SoloParciales(t *testing.T) {
	root := initRepo(t)
	writePartialVerdict(t, root, time.Now(), verdict.StatusPass)
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.ExitCode() != 1 || res.Reason != ReasonOnlyPartial {
		t.Fatalf("CA-65: esperaba rojo con razon solo-parciales: %+v", res)
	}
	if !strings.Contains(res.Action, "hoom verify") {
		t.Fatalf("CA-65: la accion debe nombrar el comando exacto: %+v", res)
	}
}

// CA-67: un veredicto viejo SIN campo partial pero con un gate skipped ajeno
// a spec_lint/spec_trace se trata como parcial (heuristica retrocompatible).
func TestCA67_HeuristicaLegacy(t *testing.T) {
	root := initRepo(t)
	legacy := &verdict.Verdict{
		Project:   "demo",
		CreatedAt: time.Now(),
		Git:       gitx.Snapshot(root, "main"),
		Gates: []verdict.GateResult{
			{Name: "test", Required: true, Status: verdict.StatusPass},
			{Name: "mutation", Required: true, Status: verdict.StatusSkipped},
		},
	}
	legacy.Finalize()
	if legacy.Partial {
		t.Fatalf("CA-67: el veredicto simulado debe carecer del campo partial")
	}
	if _, err := verdict.Write(root, legacy); err != nil {
		t.Fatal(err)
	}
	res, err := Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != ReasonOnlyPartial {
		t.Fatalf("CA-67: el legacy con skipped debe tratarse como parcial: %+v", res)
	}
	// Un skip de spec_trace NO dispara la heuristica (corridas completas
	// con spec sin criterios lo emiten legitimamente).
	spec := &verdict.Verdict{
		Project:   "demo",
		CreatedAt: time.Now().Add(time.Minute),
		Git:       gitx.Snapshot(root, "main"),
		Gates: []verdict.GateResult{
			{Name: "spec_trace", Required: true, Status: verdict.StatusSkipped},
			{Name: "test", Required: true, Status: verdict.StatusPass},
		},
	}
	spec.Finalize()
	if _, err := verdict.Write(root, spec); err != nil {
		t.Fatal(err)
	}
	res, err = Run(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.VerdictID != spec.ID {
		t.Fatalf("CA-67: spec_trace skipped no debe marcar parcial: %+v", res)
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
