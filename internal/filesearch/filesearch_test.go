// Tests adversariales del spec .hoom/specs/arroba-archivos.md
// (CA-47, CA-48, CA-50, CA-51): git es el indice — ignorados jamas,
// untracked si, ranking estable, sin git no hay drama.
package filesearch

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

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "t@hoom.dev")
	git(t, root, "config", "user.name", "t")
	return root
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// CA-47: los archivos ignorados por .gitignore JAMAS aparecen en la lista.
func TestCA47_IgnoradosNuncaAparecen(t *testing.T) {
	root := newRepo(t)
	write(t, root, ".gitignore", "node_modules/\nsecreto.env\n")
	write(t, root, "app/main.go", "package main\n")
	write(t, root, "node_modules/lib/index.js", "x\n")
	write(t, root, "secreto.env", "TOKEN=x\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")

	files := List(root)
	if !contains(files, "app/main.go") {
		t.Fatalf("CA-47: falta un archivo trackeado: %v", files)
	}
	if contains(files, "node_modules/lib/index.js") || contains(files, "secreto.env") {
		t.Fatalf("CA-47: un ignorado se colo en las sugerencias: %v", files)
	}
}

// CA-48: prefijo del nombre > subcadena en el nombre > subcadena en el
// directorio; empates alfabeticos estables.
func TestCA48_Ranking(t *testing.T) {
	files := []string{
		"docs/test-runner.md",      // subcadena en el nombre (no prefijo)
		"internal/gates/runner.go", // prefijo del nombre
		"runner-tools/config.yaml", // subcadena solo en el directorio
	}
	got := Match(files, "runn", 10)
	want := []string{"internal/gates/runner.go", "docs/test-runner.md", "runner-tools/config.yaml"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("CA-48: ranking incorrecto:\ngot:  %v\nwant: %v", got, want)
	}
	// query con caracteres de regex se trata literal, no explota
	if r := Match(files, "runner(", 10); len(r) != 0 {
		t.Fatalf("CA-48: query con parentesis debe tratarse literal: %v", r)
	}
}

// CA-49 (parte de funcion): query vacia => primeros N en orden estable;
// el limite se respeta.
func TestCA49_QueryVaciaYLimite(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go"}
	got := Match(files, "", 2)
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Fatalf("CA-49: query vacia debe dar los primeros N estables: %v", got)
	}
	if got := Match(files, "go", 3); len(got) != 3 {
		t.Fatalf("CA-49: el limite debe respetarse tambien con query: %v", got)
	}
}

// CA-50: sin git no hay lista ni error — degradacion silenciosa.
func TestCA50_SinGitListaVacia(t *testing.T) {
	root := t.TempDir()
	write(t, root, "archivo.txt", "hola\n")
	if files := List(root); len(files) != 0 {
		t.Fatalf("CA-50: sin git la lista es vacia, obtuve %v", files)
	}
}

// CA-51: los archivos nuevos sin commitear (no ignorados) tambien se
// sugieren.
func TestCA51_UntrackedAparecen(t *testing.T) {
	root := newRepo(t)
	write(t, root, "viejo.go", "package x\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	write(t, root, "nuevo/recien-creado.go", "package y\n")

	files := List(root)
	if !contains(files, "viejo.go") || !contains(files, "nuevo/recien-creado.go") {
		t.Fatalf("CA-51: deben aparecer trackeados Y untracked: %v", files)
	}
}
