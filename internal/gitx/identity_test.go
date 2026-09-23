// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md:
// CA-263 (la identidad git vive en un solo lugar: gitx.Identity) y CA-291
// (.hoom/items/ y .hoom/reviews/ quedan fuera del candidato: crear un item o
// un registro de review no mueve la huella).
package gitx

import (
	"path/filepath"
	"testing"
)

// idAislarGit corta la configuracion global y de sistema: la identidad sale
// solo de lo que el test configura en el repo.
func idAislarGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig-vacio"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func idRepo(t *testing.T, name, email string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	if name != "" {
		git(t, dir, "config", "user.name", name)
	}
	if email != "" {
		git(t, dir, "config", "user.email", email)
	}
	return dir
}

// CA-263: "Nombre <email>", o lo que haya, o "desconocido".
func TestCA263_IdentityDelProyecto(t *testing.T) {
	idAislarGit(t)
	casos := []struct{ name, email, want string }{
		{"Henry Orellana", "henry@example.com", "Henry Orellana <henry@example.com>"},
		{"Henry Orellana", "", "Henry Orellana"},
		{"", "henry@example.com", "henry@example.com"},
		{"", "", "desconocido"},
	}
	for _, c := range casos {
		dir := idRepo(t, c.name, c.email)
		if got := Identity(dir); got != c.want {
			t.Fatalf("CA-263: Identity con name=%q email=%q debe ser %q, fue %q", c.name, c.email, c.want, got)
		}
	}
	if got := Identity(t.TempDir()); got != "desconocido" {
		t.Fatalf("CA-263: fuera de un repo y sin config global la identidad es \"desconocido\", fue %q", got)
	}
}

// CA-291: crear un item o un registro de review (sin commitear o
// commiteado) no cambia ChangeFingerprint ni entra en ChangedFiles. Un
// archivo de codigo si la cambia (el test no es ciego).
func TestCA291_ItemsYReviewsFueraDeLaHuella(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	h0 := Snapshot(root, "main").ChangeFingerprint
	if h0 == "" {
		t.Fatal("CA-291: la huella del arbol limpio no puede ser vacia")
	}

	write(t, root, ".hoom/items/precios.yaml", "titulo: Precios\ntipo: feature\nprioridad: media\n")
	write(t, root, ".hoom/reviews/20260922T150405_ab12cd.json", "{\"id\":\"20260922T150405_ab12cd\",\"task\":\"precios\"}\n")
	s := Snapshot(root, "main")
	if s.ChangeFingerprint != h0 {
		t.Fatalf("CA-291: un item y un registro de review SIN commitear no mueven la huella: %s -> %s", h0, s.ChangeFingerprint)
	}
	for _, f := range s.ChangedFiles {
		if f == ".hoom/items/precios.yaml" || f == ".hoom/reviews/20260922T150405_ab12cd.json" {
			t.Fatalf("CA-291: %s no es parte del candidato: %v", f, s.ChangedFiles)
		}
	}

	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "item y review")
	if h := Snapshot(root, "main").ChangeFingerprint; h != h0 {
		t.Fatalf("CA-291: commitearlos tampoco mueve la huella: %s -> %s", h0, h)
	}

	// editar el item commiteado (lo que hace 'hoom task done') tampoco
	write(t, root, ".hoom/items/precios.yaml", "titulo: Precios\ntipo: feature\nprioridad: media\nhecho_en: 2026-09-25T10:00:00Z\n")
	if h := Snapshot(root, "main").ChangeFingerprint; h != h0 {
		t.Fatalf("CA-291: registrar el cierre en el item no rompe la huella: %s -> %s", h0, h)
	}

	// control: el codigo SI mueve la huella
	write(t, root, "nuevo.go", "package app\n")
	if h := Snapshot(root, "main").ChangeFingerprint; h == h0 {
		t.Fatal("CA-291: un archivo de codigo nuevo debe mover la huella (el test veria cualquier cosa)")
	}
}
