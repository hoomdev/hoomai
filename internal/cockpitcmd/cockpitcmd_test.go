// Tests adversariales del spec .hoom/specs/cockpit-tmux-zellij.md
// (CA-81..CA-88): el cockpit compone tmux/zellij sin adivinar nada — plan
// verificado contra recorders, sin multiplexor real.
package cockpitcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
)

type call struct {
	dir, name, args string
}

func recorder(hasSession bool, env map[string]string) (*[]call, Deps) {
	calls := &[]call{}
	rec := func(dir, name string, args ...string) error {
		*calls = append(*calls, call{dir, name, strings.Join(args, " ")})
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" && !hasSession {
			return fmt.Errorf("no session")
		}
		return nil
	}
	return calls, Deps{
		LookPath: func(f string) (string, error) { return "/usr/bin/" + f, nil },
		RunCmd:   rec,
		QuietCmd: rec,
		Getenv:   func(k string) string { return env[k] },
		HoomBin:  "/opt/fake/bin/hoom",
	}
}

func hasCall(calls []call, name, argsPart string) bool {
	for _, c := range calls {
		if c.name == name && strings.Contains(c.args, argsPart) {
			return true
		}
	}
	return false
}

// fakeBin deja un ejecutable falso en dir para que providers.Detect lo vea.
func fakeBin(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// CA-81: sin tmux ni zellij, error con la accion exacta y cero procesos.
func TestCA81_SinMultiplexor(t *testing.T) {
	calls, deps := recorder(false, nil)
	deps.LookPath = func(string) (string, error) { return "", fmt.Errorf("not found") }
	err := Run(t.TempDir(), "demo", Options{Provider: "claude"}, deps)
	if err == nil || !strings.Contains(err.Error(), "status --watch") {
		t.Fatalf("CA-81: esperaba error accionable con el watch manual: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("CA-81: no debe lanzarse ningun proceso: %+v", *calls)
	}
}

// CA-82: sesion tmux nueva = new-session con la CLI + split con el watch +
// attach, todos sobre el directorio del proyecto.
func TestCA82_LayoutTmux(t *testing.T) {
	root := t.TempDir()
	calls, deps := recorder(false, nil)
	if err := Run(root, "demo", Options{Provider: "claude"}, deps); err != nil {
		t.Fatal(err)
	}
	if !hasCall(*calls, "tmux", "new-session -d -s hoom-demo -c "+root+" claude") {
		t.Fatalf("CA-82: falta new-session con la CLI en el proyecto: %+v", *calls)
	}
	if !hasCall(*calls, "tmux", "split-window") || !hasCall(*calls, "tmux", "status --watch") {
		t.Fatalf("CA-82: falta el pane del watch: %+v", *calls)
	}
	if !hasCall(*calls, "tmux", "attach-session -t hoom-demo") {
		t.Fatalf("CA-82: falta el attach final: %+v", *calls)
	}
}

// CA-83: sin --provider jamas se adivina: cero CLIs = error, varias = error
// listandolas, exactamente una = se usa esa.
func TestCA83_ProviderSinAdivinar(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	_, deps := recorder(false, nil)
	err := Run(t.TempDir(), "demo", Options{}, deps)
	if err == nil || !strings.Contains(err.Error(), "ninguna CLI") {
		t.Fatalf("CA-83: cero CLIs debe ser error accionable: %v", err)
	}

	fakeBin(t, binDir, "claude")
	fakeBin(t, binDir, "opencode")
	err = Run(t.TempDir(), "demo", Options{}, deps)
	if err == nil || !strings.Contains(err.Error(), "claude") || !strings.Contains(err.Error(), "opencode") {
		t.Fatalf("CA-83: varias CLIs debe listar lo detectado: %v", err)
	}

	if err := os.Remove(filepath.Join(binDir, "opencode")); err != nil {
		t.Fatal(err)
	}
	calls, deps := recorder(false, nil)
	if err := Run(t.TempDir(), "demo", Options{}, deps); err != nil {
		t.Fatalf("CA-83: una sola CLI instalada debe usarse sola: %v", err)
	}
	if !hasCall(*calls, "tmux", " claude") {
		t.Fatalf("CA-83: debia lanzar claude: %+v", *calls)
	}
}

// CA-84: sesion existente => re-attach sin panes nuevos; dentro de tmux =>
// switch-client, jamas anidar.
func TestCA84_SesionEstable(t *testing.T) {
	calls, deps := recorder(true, nil)
	if err := Run(t.TempDir(), "Mi Proyecto", Options{Provider: "claude"}, deps); err != nil {
		t.Fatal(err)
	}
	if hasCall(*calls, "tmux", "new-session") || hasCall(*calls, "tmux", "split-window") {
		t.Fatalf("CA-84: con sesion existente no se crean panes: %+v", *calls)
	}
	if !hasCall(*calls, "tmux", "attach-session -t hoom-mi-proyecto") {
		t.Fatalf("CA-84: esperaba re-attach al nombre estable: %+v", *calls)
	}

	calls, deps = recorder(true, map[string]string{"TMUX": "/tmp/sock,1,0"})
	if err := Run(t.TempDir(), "demo", Options{Provider: "claude"}, deps); err != nil {
		t.Fatal(err)
	}
	if !hasCall(*calls, "tmux", "switch-client -t hoom-demo") || hasCall(*calls, "tmux", "attach-session") {
		t.Fatalf("CA-84: dentro de tmux debe usarse switch-client: %+v", *calls)
	}
}

// CA-85: --task monta el cockpit en el worktree y nombra la sesion con el
// slug; tarea inexistente = error que nombra hoom task start.
func TestCA85_TaskWorktree(t *testing.T) {
	root := t.TempDir()
	_, deps := recorder(false, nil)
	err := Run(root, "demo", Options{Provider: "claude", Task: "facturas"}, deps)
	if err == nil || !strings.Contains(err.Error(), "hoom task start facturas") {
		t.Fatalf("CA-85: tarea inexistente debe nombrar la accion: %v", err)
	}

	wt := filepath.Join(root, ".hoom", "worktrees", "facturas")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	calls, deps := recorder(false, nil)
	if err := Run(root, "demo", Options{Provider: "claude", Task: "facturas"}, deps); err != nil {
		t.Fatal(err)
	}
	if !hasCall(*calls, "tmux", "new-session -d -s hoom-demo-facturas -c "+wt+" claude") {
		t.Fatalf("CA-85: los panes deben vivir en el worktree: %+v", *calls)
	}
}

// CA-86: zellij genera el layout KDL en .hoom/cache y lanza la sesion.
func TestCA86_ZellijLayout(t *testing.T) {
	root := t.TempDir()
	calls, deps := recorder(false, nil)
	if err := Run(root, "demo", Options{Provider: "claude", Mux: "zellij"}, deps); err != nil {
		t.Fatal(err)
	}
	layout := filepath.Join(root, ".hoom", "cache", "cockpit-hoom-demo.kdl")
	raw, err := os.ReadFile(layout)
	if err != nil {
		t.Fatalf("CA-86: falta el layout KDL en .hoom/cache: %v", err)
	}
	for _, want := range []string{`command="claude"`, `"status" "--watch"`, "/opt/fake/bin/hoom"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("CA-86: el layout debe declarar %q:\n%s", want, raw)
		}
	}
	if !hasCall(*calls, "zellij", "--session hoom-demo") {
		t.Fatalf("CA-86: falta el lanzamiento de la sesion zellij: %+v", *calls)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// CA-87: el cockpit no escribe fuera de .hoom/cache y no altera la huella.
func TestCA87_HuellaIntacta(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@hoom.dev")
	gitRun(t, root, "config", "user.name", "hoom test")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "inicial")

	before := gitx.Snapshot(root, "main").ChangeFingerprint
	_, deps := recorder(false, nil)
	if err := Run(root, "demo", Options{Provider: "claude"}, deps); err != nil {
		t.Fatal(err)
	}
	if err := Run(root, "demo", Options{Provider: "claude", Mux: "zellij"}, deps); err != nil {
		t.Fatal(err)
	}
	if after := gitx.Snapshot(root, "main").ChangeFingerprint; after != before {
		t.Fatalf("CA-87: la huella cambio por armar el cockpit: %q vs %q", before, after)
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return werr
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, ".git"+string(filepath.Separator)) || rel == "app.go" {
			return nil
		}
		if !strings.HasPrefix(rel, filepath.Join(".hoom", "cache")+string(filepath.Separator)) {
			return fmt.Errorf("archivo fuera de .hoom/cache: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CA-87: %v", err)
	}
}

// CA-88: el pane del watch usa la ruta ABSOLUTA del binario hoom en
// ejecucion, no un hoom del PATH de la sesion.
func TestCA88_BinarioAbsoluto(t *testing.T) {
	calls, deps := recorder(false, nil)
	if err := Run(t.TempDir(), "demo", Options{Provider: "claude"}, deps); err != nil {
		t.Fatal(err)
	}
	if !hasCall(*calls, "tmux", "'/opt/fake/bin/hoom' status --watch") {
		t.Fatalf("CA-88: el watch debe invocar la ruta absoluta del binario: %+v", *calls)
	}
}
