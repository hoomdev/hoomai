// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// hoomfs (CA-770, aclaracion 15): GitignoreBody incluye local.yaml despues
// de los directorios; EnsureIgnoredFiles completa .hoom/.gitignore con la
// linea local.yaml (creandolo si no hay), nunca reescribe uno que ya la
// tiene y devuelve lo que agrego; EnsureIgnored sin nombres sigue agregando
// solo directorios. Con ese .gitignore, git ignora .hoom/local.yaml.
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package hoomfs

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func veeGitignore(t *testing.T, root, body string) string {
	t.Helper()
	gi := filepath.Join(root, ".hoom", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(gi, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return gi
}

func veeLineas(body string) []string {
	var out []string
	for _, l := range strings.Split(body, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// CA-770: GitignoreBody es una linea <d>/ por directorio de Local, en su
// orden, y despues local.yaml; cada linea con su salto.
func TestCA770_GitignoreBodyIncluyeLocalYaml(t *testing.T) {
	const ca = "CA-770"
	if !reflect.DeepEqual(LocalFiles, []string{"local.yaml"}) {
		t.Fatalf("%s: LocalFiles: %q", ca, LocalFiles)
	}
	if want := "cache/\nworktrees/\nruns/\nisolated/\nenvelopes/\nlocal.yaml\n"; GitignoreBody() != want {
		t.Fatalf("%s: GitignoreBody es %q; fue %q", ca, want, GitignoreBody())
	}
	var want strings.Builder
	for _, d := range Local {
		want.WriteString(d + "/\n")
	}
	for _, f := range LocalFiles {
		want.WriteString(f + "\n")
	}
	if GitignoreBody() != want.String() {
		t.Fatalf("%s: GitignoreBody sale de Local y LocalFiles: %q", ca, GitignoreBody())
	}
}

// CA-770: EnsureIgnoredFiles crea .hoom/.gitignore si no hay, agrega la
// linea local.yaml a uno que no la tiene sin tocar el resto, y devuelve lo
// que agrego. Una linea cuenta si, sin blancos al borde, es local.yaml: ni
// local.yaml/, ni /local.yaml, ni un comentario.
func TestCA770_EnsureIgnoredFilesAgregaLocalYaml(t *testing.T) {
	const ca = "CA-770"
	for _, c := range []struct {
		nombre, antes string
		lineas        []string
	}{
		{"sin .gitignore", "", []string{"local.yaml"}},
		{"con directorios", "cache/\nworktrees/\n", []string{"cache/", "worktrees/", "local.yaml"}},
		{"sin salto final", "cache/", []string{"cache/", "local.yaml"}},
		{"con local.yaml/", "local.yaml/\n", []string{"local.yaml/", "local.yaml"}},
		{"con /local.yaml", "/local.yaml\n", []string{"/local.yaml", "local.yaml"}},
		{"con un comentario", "# local.yaml\n", []string{"# local.yaml", "local.yaml"}},
		{"con otro archivo", "local.yml\nmi-local.yaml\n", []string{"local.yml", "mi-local.yaml", "local.yaml"}},
	} {
		for _, nombres := range [][]string{nil, {"local.yaml"}} {
			root := t.TempDir()
			gi := veeGitignore(t, root, c.antes)
			added, err := EnsureIgnoredFiles(root, nombres...)
			if err != nil || !reflect.DeepEqual(added, []string{"local.yaml"}) {
				t.Fatalf("%s: %s (%q): agrega local.yaml: %q %v", ca, c.nombre, nombres, added, err)
			}
			raw, err := os.ReadFile(gi)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(veeLineas(string(raw)), c.lineas) || !strings.HasSuffix(string(raw), "local.yaml\n") {
				t.Fatalf("%s: %s: el .gitignore queda con %q: %q", ca, c.nombre, c.lineas, raw)
			}
			if c.antes != "" && strings.HasSuffix(c.antes, "\n") && !strings.HasPrefix(string(raw), c.antes) {
				t.Fatalf("%s: %s: lo que habia no cambia: %q -> %q", ca, c.nombre, c.antes, raw)
			}
		}
	}
}

// CA-770: un .gitignore que ya tiene la linea (con blancos al borde
// tambien) no se reescribe, ni byte a byte: los bytes y la fecha quedan.
func TestCA770_EnsureIgnoredFilesNoReescribe(t *testing.T) {
	const ca = "CA-770"
	for _, antes := range []string{"local.yaml\n", "cache/\n  local.yaml \t\nruns/\n", GitignoreBody(), "local.yaml"} {
		root := t.TempDir()
		gi := veeGitignore(t, root, antes)
		viejo := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(gi, viejo, viejo); err != nil {
			t.Fatal(err)
		}
		added, err := EnsureIgnoredFiles(root)
		if err != nil || len(added) != 0 {
			t.Fatalf("%s: %q: no agrega nada: %q %v", ca, antes, added, err)
		}
		raw, _ := os.ReadFile(gi)
		st, _ := os.Stat(gi)
		if string(raw) != antes || !st.ModTime().Equal(viejo) {
			t.Fatalf("%s: %q: no se reescribe: %q, %v", ca, antes, raw, st.ModTime())
		}
	}
}

// CA-770 (aclaracion 15): EnsureIgnored sin nombres sigue agregando solo
// directorios: nunca local.yaml.
func TestCA770_EnsureIgnoredSigueSiendoDeDirectorios(t *testing.T) {
	const ca = "CA-770"
	root := t.TempDir()
	gi := veeGitignore(t, root, "worktrees/\n")
	added, err := EnsureIgnored(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range added {
		if !strings.HasSuffix(a, "/") || strings.Contains(a, "local.yaml") {
			t.Fatalf("%s: EnsureIgnored agrega solo directorios: %q", ca, added)
		}
	}
	if raw, _ := os.ReadFile(gi); strings.Contains(string(raw), "local.yaml") {
		t.Fatalf("%s: EnsureIgnored no agrega local.yaml: %q", ca, raw)
	}
	if added, err := EnsureIgnoredFiles(root); err != nil || !reflect.DeepEqual(added, []string{"local.yaml"}) {
		t.Fatalf("%s: despues, EnsureIgnoredFiles agrega local.yaml: %q %v", ca, added, err)
	}
	raw, _ := os.ReadFile(gi)
	if want := "worktrees/\ncache/\nruns/\nisolated/\nenvelopes/\nlocal.yaml\n"; string(raw) != want {
		t.Fatalf("%s: queda %q; fue %q", ca, want, raw)
	}
}

// CA-770: con el .gitignore canonico, git ignora .hoom/local.yaml (y git
// status no lo muestra).
func TestCA770_ConElGitignoreCanonicoGitIgnoraLocalYaml(t *testing.T) {
	const ca = "CA-770"
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	veeGitignore(t, root, GitignoreBody())
	if err := os.WriteFile(filepath.Join(root, ".hoom", "local.yaml"), []byte("ejecutar: \"ddev {cmd}\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "check-ignore", "-q", ".hoom/local.yaml").Run(); err != nil {
		t.Fatalf("%s: git ignora .hoom/local.yaml con GitignoreBody: %v", ca, err)
	}
	out, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil || strings.Contains(string(out), "local.yaml") {
		t.Fatalf("%s: git status no muestra .hoom/local.yaml: %q %v", ca, out, err)
	}
}
