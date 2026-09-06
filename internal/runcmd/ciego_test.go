// Tests adversariales del spec .hoom/specs/test-writer-en-arbol-ciego.md:
// runcmd solo necesita saber DONDE corre; el sidecar convierte la garantia en
// un dato durable en vez de una linea de terminal.
package runcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// CA-178: Dir manda sobre Task, y un run dentro de un arbol ciego queda
// marcado con el commit del que se armo.
func TestCA178_DirYSidecarDelRunCiego(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@hoom.dev")
	gitRun(t, root, "config", "user.name", "hoom test")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "inicial")

	// una tarea, para probar que Dir le gana
	wt := filepath.Join(root, ".hoom", "worktrees", "tarea")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	ciego := filepath.Join(root, ".hoom", "isolated", "test-writer_x")
	gitRun(t, root, "worktree", "add", "--detach", ciego, "HEAD")

	m := NewManager(root)
	dir, err := m.dirFor(StartOptions{Task: "tarea", Dir: ciego})
	if err != nil {
		t.Fatal(err)
	}
	if dir != ciego {
		t.Fatalf("CA-178: Dir explicito manda sobre Task: %q", dir)
	}
	if dir, err = m.dirFor(StartOptions{Task: "tarea"}); err != nil || dir != wt {
		t.Fatalf("CA-178: sin Dir se resuelve desde Task: %q %v", dir, err)
	}
	if _, err = m.dirFor(StartOptions{Dir: filepath.Join(root, "no-existe")}); err == nil {
		t.Fatal("CA-178: un Dir inexistente es error")
	}

	// el sidecar reconoce el arbol ciego y guarda su commit
	commit := strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD"))
	from, blind := blindFrom(ciego)
	if !blind || from != commit {
		t.Fatalf("CA-178: un run bajo .hoom/isolated/ es ciego y recuerda su commit: %q %v", from, blind)
	}
	if _, blind := blindFrom(wt); blind {
		t.Fatal("CA-178: el worktree de una tarea NO es un arbol ciego")
	}
	if _, blind := blindFrom(root); blind {
		t.Fatal("CA-178: el proyecto tampoco")
	}

	fakeCLI(t, "claude", "exit 0\n")
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Dir: ciego, Role: "test-writer"})
	if err != nil {
		t.Fatal(err)
	}
	m.Wait(info.ID)
	var meta Meta
	for _, x := range Metas(root) {
		if x.ID == info.ID {
			meta = x
		}
	}
	if meta.Dir != ciego {
		t.Fatalf("CA-178: el sidecar guarda donde corrio: %q", meta.Dir)
	}
	if !meta.Isolated || meta.IsolatedFrom != commit {
		t.Fatalf("CA-178: el sidecar prueba que se escribio a ciegas y desde donde: %+v", meta)
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func fakeCLI(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}
