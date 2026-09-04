// Tests adversariales del spec .hoom/specs/hoom-agent-sobre-determinista.md:
// el sobre necesita ver EXACTAMENTE lo que la huella esconde.
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// CA-133: Touched ve lo que Snapshot excluye del candidato, cubre
// modificados/sin seguimiento/borrados, y devuelve vacio fuera de un repo.
func TestCA133_TouchedVeLoQueLaHuellaEsconde(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "app.go", "package app\n")
	write(t, root, "viejo.go", "package viejo\n")
	write(t, root, ".hoom/verdicts/v1.json", "{}\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")

	// modificado (evidencia que la huella excluye), sin seguimiento, borrado
	write(t, root, ".hoom/verdicts/v1.json", "{\"tocado\":true}\n")
	write(t, root, ".hoom/findings/f1.json", "{}\n")
	write(t, root, ".hoom/ratchet.json", "{\"schema\":\"hoom.ratchet/v1\"}\n")
	if err := os.Remove(filepath.Join(root, "viejo.go")); err != nil {
		t.Fatal(err)
	}

	got := Touched(root, "main")
	for _, p := range []string{".hoom/verdicts/v1.json", ".hoom/findings/f1.json", ".hoom/ratchet.json"} {
		if got[p] == "" {
			t.Fatalf("CA-133: Touched debe ver %s (la huella lo excluye a proposito): %v", p, got)
		}
	}
	if got["viejo.go"] != "-" {
		t.Fatalf("CA-133: un archivo borrado debe valer \"-\": %q", got["viejo.go"])
	}
	if _, ok := got["app.go"]; ok {
		t.Fatalf("CA-133: app.go no cambio y no deberia aparecer: %v", got)
	}
	// la huella oficial NO ve la evidencia: son contratos distintos a proposito
	snap := Snapshot(root, "main")
	for _, f := range snap.ChangedFiles {
		if f == ".hoom/verdicts/v1.json" {
			t.Fatal("CA-133: Snapshot no debe incluir veredictos en el candidato")
		}
	}

	if len(Touched(t.TempDir(), "main")) != 0 {
		t.Fatal("CA-133: fuera de un repo git el mapa debe venir vacio")
	}
}
