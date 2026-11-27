package gitx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSubdirectorio_HuellaYTouchedCubrenCambiosSinCommit reproduce el caso de
// un proyecto cuyo hoom.yaml vive en un subdirectorio del repositorio Git
// (ej. dte_electronic_invoices/electronic_invoices): cambiar un byte en un
// archivo trackeado debe mover la huella inmediatamente, commitear ese mismo
// contenido la preserva, escribir veredictos bajo <sub/>/.hoom/ no la mueve,
// y TouchedErr devuelve hashes reales en lugar de "-".
func TestSubdirectorio_HuellaYTouchedCubrenCambiosSinCommit(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.email", "test@hoom.dev")
	git(t, repo, "config", "user.name", "hoom test")

	sub := filepath.Join(repo, "electronic_invoices")
	write(t, sub, "hoom.yaml", "schema: hoom/v1\nproject: dte\n")
	write(t, sub, "app/Services/CustomerService.php", "<?php\nclass CustomerService { public int $v = 1; }\n")
	write(t, sub, "app/Services/Borrame.php", "<?php\nclass Borrame {}\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "inicial")

	s0 := Snapshot(sub, "main")
	h0, err := HuellaConTope(sub, 30*time.Second)
	if err != nil {
		t.Fatalf("HuellaConTope inicial: %v", err)
	}
	if s0.ChangeFingerprint == "" || s0.ChangeFingerprint != h0 {
		t.Fatalf("huella inicial inconsistente: Snapshot=%q HuellaConTope=%q", s0.ChangeFingerprint, h0)
	}

	// 1. Editar un byte en un archivo trackeado del subdirectorio debe cambiar la huella.
	write(t, sub, "app/Services/CustomerService.php", "<?php\nclass CustomerService { public int $v = 2; }\n")
	s1 := Snapshot(sub, "main")
	h1, err := HuellaConTope(sub, 30*time.Second)
	if err != nil {
		t.Fatalf("HuellaConTope tras edicion: %v", err)
	}
	if s1.ChangeFingerprint == s0.ChangeFingerprint {
		t.Fatalf("cambiar 1 byte en subdirectorio NO movio la huella: %q", s1.ChangeFingerprint)
	}
	if s1.ChangeFingerprint != h1 {
		t.Fatalf("Snapshot=%q != HuellaConTope=%q tras edicion", s1.ChangeFingerprint, h1)
	}

	// TouchedErr debe devolver un hash real (no "-") para el archivo editado.
	touched, err := TouchedErr(sub, "main")
	if err != nil {
		t.Fatalf("TouchedErr: %v", err)
	}
	const keyEditado = "electronic_invoices/app/Services/CustomerService.php"
	if got := touched[keyEditado]; got == "" || got == "-" {
		t.Fatalf("TouchedErr[%q] = %q; esperaba hash real de contenido", keyEditado, got)
	}

	// 2. Escribir un veredicto en <sub/>/.hoom/verdicts/ NO debe mover la huella ni entrar en ChangedFiles.
	write(t, sub, ".hoom/verdicts/2026-11-27T00-00-00Z_deadbeef.json", `{"verdict":"green"}`+"\n")
	s1v := Snapshot(sub, "main")
	if s1v.ChangeFingerprint != s1.ChangeFingerprint {
		t.Fatalf("escribir veredicto en subdirectorio movio la huella: antes=%q despues=%q", s1.ChangeFingerprint, s1v.ChangeFingerprint)
	}
	for _, f := range s1v.ChangedFiles {
		if f == "electronic_invoices/.hoom/verdicts/2026-11-27T00-00-00Z_deadbeef.json" {
			t.Fatalf("veredicto en subdirectorio no debe aparecer en ChangedFiles: %v", s1v.ChangedFiles)
		}
	}

	// 3. Commitear el mismo contenido (incluido el veredicto) debe conservar exactamente la huella.
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "cambio v2")
	s2 := Snapshot(sub, "main")
	if s2.ChangeFingerprint != s1.ChangeFingerprint {
		t.Fatalf("commitear contenido identico cambio la huella: antes=%q despues=%q", s1.ChangeFingerprint, s2.ChangeFingerprint)
	}

	// 4. Crear un archivo nuevo sin seguimiento en el subdirectorio debe cambiar la huella y aparecer en TouchedErr.
	write(t, sub, "app/Services/Nuevo.php", "<?php\nclass Nuevo {}\n")
	s3 := Snapshot(sub, "main")
	if s3.ChangeFingerprint == s2.ChangeFingerprint {
		t.Fatalf("archivo nuevo sin seguimiento en subdirectorio no movio la huella: %q", s3.ChangeFingerprint)
	}
	touched, err = TouchedErr(sub, "main")
	if err != nil {
		t.Fatalf("TouchedErr con archivo nuevo: %v", err)
	}
	const keyNuevo = "electronic_invoices/app/Services/Nuevo.php"
	if got := touched[keyNuevo]; got == "" || got == "-" {
		t.Fatalf("TouchedErr[%q] = %q; esperaba hash real", keyNuevo, got)
	}

	// 5. Borrar un archivo trackeado en el subdirectorio debe cambiar la huella y marcar "-" en TouchedErr.
	if err := os.Remove(filepath.Join(sub, "app/Services/Borrame.php")); err != nil {
		t.Fatal(err)
	}
	s4 := Snapshot(sub, "main")
	if s4.ChangeFingerprint == s3.ChangeFingerprint {
		t.Fatalf("borrar archivo trackeado en subdirectorio no movio la huella: %q", s4.ChangeFingerprint)
	}
	touched, err = TouchedErr(sub, "main")
	if err != nil {
		t.Fatalf("TouchedErr con archivo borrado: %v", err)
	}
	const keyBorrado = "electronic_invoices/app/Services/Borrame.php"
	if got := touched[keyBorrado]; got != "-" {
		t.Fatalf("TouchedErr[%q] = %q; esperaba \"-\" para borrado", keyBorrado, got)
	}
}
