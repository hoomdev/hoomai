package isolate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSubdirectorio_ArbolCiegoYTrasplante verifica el arbol ciego cuando el
// proyecto (hoom.yaml) vive en un subdirectorio del repositorio Git (como
// dte_electronic_invoices/electronic_invoices): sparse-checkout acotado al
// subdirectorio, deteccion de brechas y trasplante al arbol real sin duplicar
// el prefijo.
func TestSubdirectorio_ArbolCiegoYTrasplante(t *testing.T) {
	repo := t.TempDir()
	gitc(t, repo, "init", "-b", "main")
	gitc(t, repo, "config", "user.email", "test@hoom.dev")
	gitc(t, repo, "config", "user.name", "hoom test")

	write(t, repo, ".gitignore", "*.log\n")
	write(t, repo, "otro/Secret.php", "<?php // fuera del proyecto\n")

	sub := filepath.Join(repo, "electronic_invoices")
	write(t, sub, "composer.json", `{"name":"demo/dte"}`+"\n")
	write(t, sub, ".gitignore", "vendor/\n")
	write(t, sub, "app/Services/DteService.php", "<?php\nclass DteService {}\n")
	write(t, sub, "tests/Feature/HuellaTest.php", "<?php\nclass HuellaTest {}\n")
	write(t, sub, "tests/fixtures/inv_01.json", `{"v":1}`+"\n")
	write(t, sub, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\n")
	write(t, sub, ".hoom/specs/b20.md", "# spec B20\nCA-1: invalidacion\n")
	write(t, sub, ".hoom/verdicts/v.json", `{"verdict":"red","gate":"DteService.php:2"}`)
	write(t, sub, ".hoom/findings/f.json", `{"desc":"cita DteService"}`)
	gitc(t, repo, "add", "-A")
	gitc(t, repo, "commit", "-m", "inicial")

	pats := Patterns(
		[]string{".hoom/**", "tests/**", "**/*Test.php"},
		nil,
		[]string{"composer.json"},
	)
	tree, err := Open(sub, "test-writer_sub", pats)
	if err != nil {
		t.Fatalf("Open en subdirectorio: %v", err)
	}
	t.Cleanup(func() { _ = tree.Close() })

	if filepath.Base(tree.Dir) != "electronic_invoices" {
		t.Fatalf("tree.Dir debe apuntar al subdirectorio del proyecto dentro del worktree: %s", tree.Dir)
	}
	if tree.Worktree() == tree.Dir {
		t.Fatalf("tree.Worktree() debe ser la raiz del worktree, distinto de tree.Dir (%s)", tree.Dir)
	}

	presentes := []string{
		"composer.json",
		"tests/Feature/HuellaTest.php",
		"tests/fixtures/inv_01.json",
		".hoom/specs/b20.md",
	}
	for _, p := range presentes {
		if !existe(t, filepath.Join(tree.Dir, p)) {
			t.Fatalf("%s deberia estar presente en el arbol ciego (%s)", p, tree.Dir)
		}
	}
	ausentes := []string{
		"app/Services/DteService.php",
		".hoom/verdicts/v.json",
		".hoom/findings/f.json",
	}
	for _, p := range ausentes {
		if existe(t, filepath.Join(tree.Dir, p)) {
			t.Fatalf("%s NO deberia estar presente en el arbol ciego", p)
		}
	}
	if existe(t, filepath.Join(tree.Worktree(), "otro/Secret.php")) {
		t.Fatalf("otro/Secret.php (fuera del subdirectorio) NO deberia estar presente en el arbol ciego")
	}

	if b := tree.Breaches(); len(b) != 0 {
		t.Fatalf("aislamiento recien abierto deberia estar intacto: %v", b)
	}

	// Restaurar un testigo dentro del subdirectorio debe reportarse relativo al proyecto.
	write(t, tree.Dir, "app/Services/DteService.php", "<?php\nclass DteService {}\n")
	if b := tree.Breaches(); len(b) != 1 || b[0] != "app/Services/DteService.php" {
		t.Fatalf("Breaches tras restaurar testigo = %v; esperaba [app/Services/DteService.php]", b)
	}
	if err := os.Remove(filepath.Join(tree.Dir, "app/Services/DteService.php")); err != nil {
		t.Fatal(err)
	}
	if b := tree.Breaches(); len(b) != 0 {
		t.Fatalf("Breaches tras quitar testigo = %v; esperaba []", b)
	}

	// El test-writer crea tests nuevos y edita uno existente en tree.Dir;
	// Apply los trasplanta a sub/ sin duplicar electronic_invoices/.
	write(t, tree.Dir, "tests/Feature/B20Test.php", "<?php\n// CA-1\nclass B20Test {}\n")
	write(t, tree.Dir, "tests/Feature/HuellaTest.php", "<?php\nclass HuellaTest { /* v2 */ }\n")
	applied, removed, err := tree.Apply(sub, []string{"tests/Feature/B20Test.php", "tests/Feature/HuellaTest.php"})
	if err != nil {
		t.Fatalf("Apply en subdirectorio: %v", err)
	}
	if len(removed) != 0 || len(applied) != 2 {
		t.Fatalf("Apply = applied:%v removed:%v; esperaba 2 aplicados y 0 borrados", applied, removed)
	}
	if !existe(t, filepath.Join(sub, "tests/Feature/B20Test.php")) {
		t.Fatalf("el test nuevo no llego a sub/tests/Feature/B20Test.php")
	}
	if existe(t, filepath.Join(sub, "electronic_invoices")) {
		t.Fatalf("Apply duplico el prefijo del subdirectorio bajo %s", sub)
	}
	raw, err := os.ReadFile(filepath.Join(sub, "tests/Feature/HuellaTest.php"))
	if err != nil || !strings.Contains(string(raw), "v2") {
		t.Fatalf("HuellaTest.php no recibio la edicion: err=%v contenido=%q", err, string(raw))
	}

	if err := tree.Close(); err != nil {
		t.Fatalf("Close en subdirectorio: %v", err)
	}
	if existe(t, tree.Worktree()) {
		t.Fatalf("Close dejo el worktree en pie: %s", tree.Worktree())
	}
	if lst := gitc(t, repo, "worktree", "list"); strings.Contains(lst, "test-writer_sub") {
		t.Fatalf("Close dejo el worktree registrado en git: %s", lst)
	}
}
