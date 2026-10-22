// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// gitx, sobre .hoom/specs/evidencia-en-disco.md (CA-442) y el gate de scope
// de .hoom/specs/hoom-agent-sobre-determinista.md (CA-133). Escritos desde el
// spec, el texto del hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Reusan git y write de touched_test.go; los helpers de este
// archivo llevan el prefijo e1b.
//
//   - 20261022T202056_6d4918 (high, risk): TouchedErr solo propagaba el
//     timeout de un git del listado; un git que fallaba rapido (un
//     .git/index corrupto hace fallar `git diff HEAD` y `git ls-files`
//     enseguida, mientras base...HEAD sigue listando) caia en continue y
//     TouchedErr devolvia una lista parcial sin error: archivos locales
//     fuera de scope quedaban escondidos del gate. Ahora TouchedErr devuelve
//     error ante cualquier fallo de los git que necesita para listar.
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// e1bRepoConIndiceRoto arma un repo con un commit permitido en la rama
// hoom/tarea (base...HEAD lo lista), un archivo sin seguimiento y otro
// modificado fuera de commits, y devuelve el directorio. corromper deja el
// .git/index con basura.
func e1bRepoConIndiceRoto(t *testing.T) (dir string, corromper func()) {
	t.Helper()
	dir = t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@hoom.dev")
	git(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "app.go", "package app\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "inicial")
	git(t, dir, "checkout", "-q", "-b", "hoom/tarea")
	write(t, dir, "internal/permitido.go", "package internal\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "cambio permitido")
	write(t, dir, ".hoom/specs/fuera.md", "# fuera de scope\n")
	write(t, dir, "app.go", "package app // tocado\n")
	return dir, func() {
		if err := os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("basura de la prueba e1b"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// e1bGitFalla exige que, con el indice roto, git falle de verdad (no
// timeout) en el listado de lo que cambio sin commitear, y que base...HEAD
// siga listando.
func e1bGitFalla(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"diff", "--name-only", "HEAD"}, {"ls-files", "--others", "--exclude-standard"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err == nil {
			t.Skipf("fixture: con el .git/index corrupto, git %v no falla en este sistema: %s", args, out)
		}
	}
	c := exec.Command("git", "diff", "--name-only", "main...HEAD")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil || !strings.Contains(string(out), "internal/permitido.go") {
		t.Skipf("fixture: con el .git/index corrupto, git diff main...HEAD ya no lista el cambio permitido: %v %s", err, out)
	}
}

// CA-442, CA-133 (hallazgo 6d4918): con el .git/index corrupto, `git diff
// HEAD` y `git ls-files --others` fallan enseguida (exit 128) mientras
// main...HEAD sigue listando el cambio permitido. TouchedErr no devuelve esa
// lista parcial como si fuera todo lo que cambio: devuelve error (el gate
// de scope tiene que fallar cerrado ante cualquier fallo de los git que
// necesita para listar, no solo ante un timeout). Y vuelve enseguida (no
// espera TopeListado: el fallo es rapido). Guarda: con el indice sano,
// TouchedErr lista los tres cambios sin error.
func TestHallazgo_6d4918_CA442_GitDelListadoQueFallaRapidoEsUnErrorDeTouchedErr(t *testing.T) {
	const ca = "CA-442 (hallazgo 6d4918, git del listado que falla rapido)"
	dir, corromper := e1bRepoConIndiceRoto(t)
	sano, err := TouchedErr(dir, "main")
	if err != nil {
		t.Fatalf("%s: guarda: con el indice sano, TouchedErr lista sin error: %v", ca, err)
	}
	for _, p := range []string{"internal/permitido.go", ".hoom/specs/fuera.md", "app.go"} {
		if _, ok := sano[p]; !ok {
			t.Fatalf("%s: guarda: con el indice sano, TouchedErr ve %s: %v", ca, p, sano)
		}
	}

	corromper()
	e1bGitFalla(t, dir)
	t0 := time.Now()
	got, err := TouchedErr(dir, "main")
	tarda := time.Since(t0)
	if err == nil {
		t.Errorf("%s: con el .git/index corrupto, git diff HEAD y git ls-files fallan: TouchedErr devuelve error, no la lista parcial %v sin error",
			ca, got)
	}
	if tarda > 30*time.Second {
		t.Errorf("%s: un git que falla rapido no espera el tope: TouchedErr tardo %s", ca, tarda.Round(time.Millisecond))
	}
}
