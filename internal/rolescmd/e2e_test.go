// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-295, CA-298) contra el BINARIO: 'hoom roles' lee la politica del
// hoom.yaml del arbol actual, filtra, y un valor desconocido es exit 2.
package rolescmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
)

var (
	rxOnce sync.Once
	rxBin  string
	rxDir  string
	rxErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if rxDir != "" {
		os.RemoveAll(rxDir)
	}
	os.Exit(code)
}

func rxBinario(t *testing.T) string {
	t.Helper()
	rxOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			rxErr = err
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			padre := filepath.Dir(dir)
			if padre == dir {
				rxErr = fmt.Errorf("no encontre go.mod hacia arriba")
				return
			}
			dir = padre
		}
		rxDir, err = os.MkdirTemp("", "hoom-rolescmd-bin-")
		if err != nil {
			rxErr = err
			return
		}
		bin := filepath.Join(rxDir, "hoom")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/hoom")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			rxErr = fmt.Errorf("go build ./cmd/hoom: %v\n%s", err, out)
			return
		}
		rxBin = bin
	})
	if rxErr != nil {
		t.Fatal(rxErr)
	}
	return rxBin
}

func rxCorrer(t *testing.T, bin, dir string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOOM_TASK=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), errb.String()
}

// CA-298: la politica sale del hoom.yaml del arbol actual; --role y
// --provider filtran; los valores desconocidos son exit 2 con los validos;
// el texto cierra con la leyenda. CA-295: --json es el arreglo de Row.
func TestCA298_E2ERoles(t *testing.T) {
	bin := rxBinario(t)
	dir := t.TempDir()
	yml := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" +
		"agents:\n  writer:\n    write:\n      allow: [\"src/**\"]\n"
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := rxCorrer(t, bin, dir, "roles", "--json")
	if code != 0 {
		t.Fatalf("CA-295: 'hoom roles --json' sale 0, salio %d\n%s", code, errOut)
	}
	var rows []Row
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 40 {
		t.Fatalf("CA-295: --json es el arreglo de 40 filas: %v (%d)\n%s", err, len(rows), out)
	}
	if rows[0].Role != "orquestador" || rows[0].Provider != "claude" || rows[39].Role != "refutador" || rows[39].Provider != "gemini" {
		t.Fatalf("CA-295: orden agents.Roles() y despues providers.All(): %s/%s ... %s/%s",
			rows[0].Role, rows[0].Provider, rows[39].Role, rows[39].Provider)
	}

	code, out, _ = rxCorrer(t, bin, dir, "roles", "--role", "writer", "--provider", "claude", "--json")
	rows = nil
	if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 1 {
		t.Fatalf("CA-298: --role writer --provider claude da una fila: exit %d\n%s", code, out)
	}
	if !strings.Contains(rows[0].Write.Allows, "src/**") {
		t.Fatalf("CA-298: el agents.writer.write.allow de hoom.yaml se ve en la matriz: %+v", rows[0].Write)
	}

	for _, c := range []struct {
		args   []string
		nombra string
	}{
		{[]string{"roles", "--role", "bogus"}, "test-writer"},
		{[]string{"roles", "--provider", "bogus"}, "codex"},
		{[]string{"roles", "--role"}, ""},
		{[]string{"roles", "x"}, ""},
		{[]string{"roles", "--role", ""}, ""},
	} {
		code, _, errOut := rxCorrer(t, bin, dir, c.args...)
		if code != 2 {
			t.Fatalf("CA-298: 'hoom %s' es error de uso (exit 2), salio %d\n%s", strings.Join(c.args, " "), code, errOut)
		}
		if c.nombra != "" && !strings.Contains(errOut, c.nombra) {
			t.Fatalf("CA-298: 'hoom %s' lista los valores validos (%s):\n%s", strings.Join(c.args, " "), c.nombra, errOut)
		}
	}

	code, out, _ = rxCorrer(t, bin, dir, "roles", "--provider", "codex")
	if code != 0 {
		t.Fatalf("CA-298: 'hoom roles --provider codex' sale 0, salio %d", code)
	}
	var lineas []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lineas = append(lineas, l)
		}
	}
	if len(lineas) < 4 {
		t.Fatalf("CA-298: el texto trae bloques y leyenda:\n%s", out)
	}
	for i, cat := range Categories {
		if !strings.Contains(lineas[len(lineas)-4+i], cat) {
			t.Fatalf("CA-298: el texto termina con la leyenda de las cuatro categorias:\n%s", out)
		}
	}
	code, out, _ = rxCorrer(t, bin, dir, "roles", "--provider", "codex", "--json")
	rows = nil
	if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 10 {
		t.Fatalf("CA-298: --provider codex da las 10 filas de codex: exit %d\n%s", code, out)
	}
	for _, r := range rows {
		if r.Provider != "codex" {
			t.Fatalf("CA-298: --provider codex filtra los otros providers: %+v", r)
		}
	}
}
