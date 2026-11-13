package itemcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Hallazgo 20261112T204029_1d9e67: un refs/replace que hace que HEAD parezca
// tener ya lo que esta staged no esconde ese cambio al guardado de la
// tarjeta: sinGuardar, con el git del guardado, lo lista igual.
func TestWriter1d9e67ElGuardadoNoSeEnganaConUnReemplazo(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	escribir := func(s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.name", "t")
	git("config", "user.email", "t@example.com")
	escribir("v1\n")
	git("add", "a.txt")
	git("commit", "-q", "-m", "v1")
	escribir("v2\n")
	git("add", "a.txt")
	// HEAD reemplazado por un commit cuyo arbol es el del indice
	falso := git("commit-tree", git("write-tree"), "-m", "falso")
	git("replace", git("rev-parse", "HEAD"), falso)
	if st := git("status", "--porcelain"); st != "" {
		t.Fatalf("fixture: con el reemplazo, git status ve limpio el cambio staged: %q", st)
	}
	got, err := sinGuardar(dir, delGuardado())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("1d9e67: el guardado ve el cambio staged de a.txt aunque un refs/replace lo esconda: %q", got)
	}
}
