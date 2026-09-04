// Tests adversariales del spec .hoom/specs/studio-v3-cockpit.md
// (CA-22, CA-23, CA-28): el run ejecuta la CLI del usuario como subproceso
// con el cwd correcto, su narracion queda fuera de la huella, y cancelar
// conserva el log completo.
package runcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// installFake pone un `claude` falso al frente del PATH: un script que
// imprime sus argumentos y su cwd, y termina con el exit pedido.
func installFake(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

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

func allDetails(evs []Event) string {
	var b strings.Builder
	for _, e := range evs {
		b.WriteString(e.Detail + "\n")
	}
	return b.String()
}

// CA-22: el run ejecuta el comando headless documentado del provider como
// subproceso, con cwd en el proyecto, y propaga su exit code.
func TestCA22_SubprocesoCwdYExitCode(t *testing.T) {
	installFake(t, "echo \"args: $@\"\npwd\nexit 7\n")
	root := t.TempDir()
	m := NewManager(root)

	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola agente"})
	if err != nil {
		t.Fatal(err)
	}
	final := m.Wait(info.ID)
	if final.ExitCode != 7 {
		t.Fatalf("CA-22: el exit del provider debe propagarse: %+v", final)
	}
	_, evs, err := m.Events(info.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := allDetails(evs)
	for _, want := range []string{"-p", "--output-format stream-json", "--verbose", "hola agente"} {
		if !strings.Contains(out, want) {
			t.Fatalf("CA-22: falta %q en la invocacion headless documentada:\n%s", want, out)
		}
	}
	rootReal, _ := filepath.EvalSymlinks(root)
	if !strings.Contains(out, rootReal) && !strings.Contains(out, root) {
		t.Fatalf("CA-22: el cwd del subproceso debe ser el proyecto (%s):\n%s", root, out)
	}
}

// CA-22: con --task, el cwd es el worktree de la tarea; tarea inexistente
// falla con la accion exacta.
func TestCA22_TareaInexistente(t *testing.T) {
	installFake(t, "exit 0\n")
	m := NewManager(t.TempDir())
	if _, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Task: "no-existe"}); err == nil || !strings.Contains(err.Error(), "no existe") {
		t.Fatalf("CA-22: tarea inexistente debe fallar con mensaje claro: %v", err)
	}
}

// CA-23: la narracion queda en .hoom/runs/<id>.jsonl y ese directorio esta
// fuera de la huella: correr un run NO rompe un check verde.
func TestCA23_NarracionFueraDeLaHuella(t *testing.T) {
	installFake(t, "echo narrando\nexit 0\n")
	root := initRepo(t)

	before := gitx.Snapshot(root, "main").ChangeFingerprint
	v := &verdict.Verdict{
		Project: "demo",
		Git:     gitx.Snapshot(root, "main"),
		Gates:   []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass}},
	}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatal(err)
	}
	if res, _ := checkcmd.Run(root, "main"); !res.OK {
		t.Fatalf("fixture: el check debia arrancar verde: %+v", res)
	}

	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	m.Wait(info.ID)

	logPath := filepath.Join(root, ".hoom", "runs", info.ID+".jsonl")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("CA-23: falta el log del run: %v", err)
	}
	if after := gitx.Snapshot(root, "main").ChangeFingerprint; after != before {
		t.Fatalf("CA-23: la narracion cambio la huella (%s -> %s)", before, after)
	}
	if res, _ := checkcmd.Run(root, "main"); !res.OK {
		t.Fatalf("CA-23: el check verde se rompio por correr un run: %+v", res)
	}
}

// CA-28: cancelar termina el subproceso; el run queda canceled y su log
// completo sobrevive.
func TestCA28_CancelarConservaElLog(t *testing.T) {
	installFake(t, "echo arranco\nsleep 30\necho nunca-llego\n")
	root := t.TempDir()
	m := NewManager(root)

	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo"})
	if err != nil {
		t.Fatal(err)
	}
	// esperar a que el subproceso arranque de verdad
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, evs, _ := m.Events(info.ID, 0); strings.Contains(allDetails(evs), "arranco") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := m.Cancel(info.ID); err != nil {
		t.Fatal(err)
	}
	final := m.Wait(info.ID)
	if final.Status != StatusCanceled {
		t.Fatalf("CA-28: esperaba %q, es %q", StatusCanceled, final.Status)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", info.ID+".jsonl"))
	if err != nil {
		t.Fatalf("CA-28: el log debe sobrevivir a la cancelacion: %v", err)
	}
	if !strings.Contains(string(raw), "arranco") || !strings.Contains(string(raw), "cancelado") {
		t.Fatalf("CA-28: el log debe tener la narracion y el cierre de cancelacion:\n%s", raw)
	}
}
