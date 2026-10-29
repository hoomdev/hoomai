// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 3: los hallazgos de la review de la correccion, escritos desde el spec.
//
//   - 686b61 (CA-481, CA-482): cada entrada del estado de hoom tiene una
//     FORMA: .hoom/ratchet.json y .hoom/.gitignore son archivos;
//     .hoom/intake/ y los locales (.hoom/cache/, ...) son directorios. Un
//     directorio .hoom/ratchet.json/ o .hoom/.gitignore/, o un ARCHIVO
//     .hoom/intake o .hoom/cache, no son estado: entran a la evidencia (la
//     medida del rango y el parche) y, sin commitear, ensucian el arbol.
package gitx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// edrFormasAjenas son rutas con el nombre de una entrada del estado de hoom
// pero la otra forma (686b61): ninguna es estado.
var edrFormasAjenas = []string{
	".hoom/ratchet.json/x",
	".hoom/.gitignore/x",
	".hoom/intake",
	".hoom/cache",
}

// edrRepoConFormasAjenas arma main con app.go y una rama "tarea" que
// commitea las formas ajenas y app.go. Devuelve el repo y el merge-base.
func edrRepoConFormasAjenas(t *testing.T) (dir, mb string) {
	t.Helper()
	idAislarGit(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "init", "-b", "main")
	dfGit(t, dir, "config", "user.email", "test@hoom.dev")
	dfGit(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "app.go", "package app\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "inicial")
	mb = dfGit(t, dir, "rev-parse", "HEAD")
	dfGit(t, dir, "checkout", "-q", "-b", "tarea")
	for _, r := range append([]string{"app.go"}, edrFormasAjenas...) {
		write(t, dir, r, "MARCA-RAMA "+r+"\n")
	}
	dfGit(t, dir, "add", "-A", "-f")
	dfGit(t, dir, "commit", "-q", "-m", "la rama commitea las formas ajenas")
	if s := dfGit(t, dir, "status", "--porcelain", "-uall"); s != "" {
		t.Fatalf("fixture: el arbol queda limpio despues del commit: %q", s)
	}
	return dir, mb
}

// Hallazgo 686b61 (CA-481): un directorio .hoom/ratchet.json/, un directorio
// .hoom/.gitignore/, un archivo .hoom/intake y un archivo .hoom/cache,
// commiteados, estan en la medida del rango (CambiosEntre) y en el parche de
// la evidencia (CandidatePatch), con su linea.
func TestHallazgo686b61_CA481_LasFormasAjenasDelEstadoEntranALaEvidencia(t *testing.T) {
	dir, mb := edrRepoConFormasAjenas(t)
	files, _, _, err := CambiosEntre(dir, mb, dfGit(t, dir, "rev-parse", "HEAD"))
	if err != nil {
		t.Fatalf("686b61/CA-481: CambiosEntre: %v", err)
	}
	quiero := append([]string{"app.go"}, edrFormasAjenas...)
	sort.Strings(quiero)
	got := append([]string(nil), files...)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(quiero, "\n") {
		t.Fatalf("686b61/CA-481: la medida del rango trae las formas que no son estado de hoom: %q, quiero %q", got, quiero)
	}
	var buf bytes.Buffer
	if over, err := CandidatePatch(dir, "main", &buf, 16<<20); err != nil || over {
		t.Fatalf("686b61/CA-481: CandidatePatch: over %v, %v", over, err)
	}
	for _, r := range edrFormasAjenas {
		if !edrEnParche(buf.String(), r) || !strings.Contains(buf.String(), "\n+MARCA-RAMA "+r+"\n") {
			t.Fatalf("686b61/CA-481: %s no es estado de hoom: el parche de la evidencia lo trae:\n%s", r, buf.String())
		}
	}
}

// Hallazgo 686b61 (CA-482): con una de esas formas sin commitear (una
// edicion, un borrado o un hermano nuevo), el arbol esta sucio y
// CambioSinCommitear la nombra; el parche se niega con ArbolSucio. Control
// (la otra forma, la del estado): un .hoom/ratchet.json o un .hoom/.gitignore
// archivos, y lo de adentro de .hoom/intake/ y .hoom/cache/, no ensucian.
func TestHallazgo686b61_CA482_LasFormasAjenasDelEstadoEnsucian(t *testing.T) {
	dir, _ := edrRepoConFormasAjenas(t)
	sucio := func(caso, ruta string) {
		t.Helper()
		got, err := CambioSinCommitear(dir)
		if err != nil || !edrNombra(got, ruta) {
			t.Fatalf("686b61/CA-482: %s %s (no es estado de hoom) ensucia el arbol y CambioSinCommitear la nombra: %q, %v", caso, ruta, got, err)
		}
		var buf bytes.Buffer
		_, perr := CandidatePatch(dir, "main", &buf, 16<<20)
		var as ArbolSucio
		if !errors.As(perr, &as) {
			t.Fatalf("686b61/CA-482: %s %s: el parche de la evidencia se niega por arbol sucio: %v", caso, ruta, perr)
		}
	}
	limpio := func(t *testing.T) {
		t.Helper()
		if s := dfGit(t, dir, "status", "--porcelain", "-uall"); s != "" {
			t.Fatalf("fixture: el arbol vuelve a quedar limpio: %q", s)
		}
	}
	for _, r := range edrFormasAjenas {
		write(t, dir, r, "EDICION SIN COMMITEAR\n")
		sucio("edicion de", r)
		dfGit(t, dir, "checkout", "-q", "--", r)
		if err := os.Remove(filepath.Join(dir, r)); err != nil {
			t.Fatal(err)
		}
		sucio("borrado de", r)
		dfGit(t, dir, "checkout", "-q", "--", r)
		limpio(t)
	}
	for _, r := range []string{".hoom/ratchet.json/y", ".hoom/.gitignore/y"} {
		write(t, dir, r, "SIN RASTREAR\n")
		sucio("sin rastrear", r)
		if err := os.Remove(filepath.Join(dir, r)); err != nil {
			t.Fatal(err)
		}
		limpio(t)
	}

	// control: la forma del estado no ensucia. Un repo nuevo con
	// .hoom/ratchet.json y .hoom/.gitignore archivos, y .hoom/intake/ y
	// .hoom/cache/ directorios, todo commiteado y despues tocado.
	otro, _ := edrRepo(t)
	for _, r := range []string{".hoom/ratchet.json", ".hoom/.gitignore", ".hoom/intake/contrato.pdf", ".hoom/cache/c.txt"} {
		write(t, otro, r, "EDICION SIN COMMITEAR\n")
		if got, err := CambioSinCommitear(otro); err != nil || got != "" {
			t.Fatalf("686b61/CA-482: %s es estado de hoom: no ensucia el arbol: %q, %v", r, got, err)
		}
		dfGit(t, otro, "checkout", "-q", "--", r)
	}
	for _, r := range []string{".hoom/intake/nuevo/doc.pdf", ".hoom/cache/nuevo.txt"} {
		write(t, otro, r, "SIN RASTREAR\n")
		if got, err := CambioSinCommitear(otro); err != nil || got != "" {
			t.Fatalf("686b61/CA-482: %s (dentro de un directorio de estado) no ensucia el arbol: %q, %v", r, got, err)
		}
		if err := os.RemoveAll(filepath.Join(otro, filepath.Dir(r))); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		dfGit(t, otro, "checkout", "-q", "--", ".")
	}
}
