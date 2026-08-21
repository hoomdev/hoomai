// Tests adversariales del spec .hoom/specs/studio-v2-acciones.md
// (CA-11, CA-12): la aprobacion es de un CONTENIDO exacto, no de un nombre
// de archivo — editar el spec la invalida por construccion.
package approval

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func newProject(t *testing.T) (root, specPath string) {
	t.Helper()
	root = t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "ana@hoom.dev")
	git(t, root, "config", "user.name", "Ana")
	specPath = filepath.Join(root, ".hoom", "specs", "demo.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# Spec demo\n\ncontenido v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, specPath
}

func countRecords(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".hoom", "approvals"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// CA-11: approve registra sha256 del contenido + autor de git en
// .hoom/approvals/, y re-aprobar el MISMO contenido no duplica el registro.
func TestCA11_AprobarRegistraYEsIdempotente(t *testing.T) {
	root, specPath := newProject(t)

	rec, already, err := Approve(root, specPath)
	if err != nil {
		t.Fatal(err)
	}
	if already {
		t.Fatal("CA-11: la primera aprobacion no puede ser un no-op")
	}
	if len(rec.SHA256) != 64 {
		t.Fatalf("CA-11: sha256 invalido: %q", rec.SHA256)
	}
	if !strings.Contains(rec.ApprovedBy, "Ana") || !strings.Contains(rec.ApprovedBy, "ana@hoom.dev") {
		t.Fatalf("CA-11: el aprobador debe salir del git config: %q", rec.ApprovedBy)
	}
	if rec.Spec != ".hoom/specs/demo.md" {
		t.Fatalf("CA-11: el registro debe referir el spec relativo al proyecto: %q", rec.Spec)
	}
	if countRecords(t, root) != 1 {
		t.Fatalf("CA-11: esperaba 1 registro, hay %d", countRecords(t, root))
	}

	rec2, already, err := Approve(root, specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !already || rec2.SHA256 != rec.SHA256 {
		t.Fatalf("CA-11: re-aprobar el mismo contenido debe ser no-op informado: already=%v", already)
	}
	if countRecords(t, root) != 1 {
		t.Fatalf("CA-11: el no-op no puede duplicar registros, hay %d", countRecords(t, root))
	}
}

// CA-12: status distingue aprobado (vigente) / no-aprobado / invalidado, y
// solo el estado aprobado corresponde a exit 0 en el CLI.
func TestCA12_EstadosDelStatus(t *testing.T) {
	root, specPath := newProject(t)

	state, rec, err := Status(root, specPath)
	if err != nil {
		t.Fatal(err)
	}
	if state != StatusNotApproved || rec != nil {
		t.Fatalf("CA-12: sin registros el estado es no-aprobado, obtuve %q", state)
	}

	if _, _, err := Approve(root, specPath); err != nil {
		t.Fatal(err)
	}
	state, rec, err = Status(root, specPath)
	if err != nil {
		t.Fatal(err)
	}
	if state != StatusApproved || rec == nil {
		t.Fatalf("CA-12: tras aprobar el estado es aprobado, obtuve %q", state)
	}

	// editar el spec despues de aprobar => la aprobacion queda invalidada.
	if err := os.WriteFile(specPath, []byte("# Spec demo\n\ncontenido v2 EDITADO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, rec, err = Status(root, specPath)
	if err != nil {
		t.Fatal(err)
	}
	if state != StatusInvalidated || rec == nil {
		t.Fatalf("CA-12: tras editar el estado es invalidado, obtuve %q", state)
	}

	// re-aprobar el contenido nuevo => vigente otra vez, con historial (2 registros).
	if _, _, err := Approve(root, specPath); err != nil {
		t.Fatal(err)
	}
	if state, _, _ = Status(root, specPath); state != StatusApproved {
		t.Fatalf("CA-12: re-aprobado debe quedar vigente, obtuve %q", state)
	}
	if countRecords(t, root) != 2 {
		t.Fatalf("CA-12: el historial es append-only, esperaba 2 registros, hay %d", countRecords(t, root))
	}
}
