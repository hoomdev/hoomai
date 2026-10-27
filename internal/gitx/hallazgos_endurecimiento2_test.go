// Tests de regresion de la tarea endurecimiento-2 en gitx, sobre el gate de
// scope de .hoom/specs/hoom-agent-sobre-determinista.md (CA-133) y
// .hoom/specs/evidencia-en-disco.md (CA-442). Escritos desde el texto del
// hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION. Reusan
// git y write de touched_test.go; los helpers de este archivo llevan el
// prefijo e2.
//
//   - 20261024T141938_9ad39d (high): TouchedErr y HuellaConTope tomaban
//     CUALQUIER fallo del primer `git rev-parse` por "fuera de un repo" y
//     devolvian una lista vacia (o ninguna huella) sin error. Con un repo
//     roto (HEAD o config corruptos), el gate de scope veia "nada cambio" y
//     no fallaba cerrado. Ahora: un arbol que TIENE .git (en el o arriba) y
//     donde git falla es un error; sin .git en el ni arriba (fuera de un
//     repo), lista vacia y ninguna huella, sin error, como antes.
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// e2Repo arma un repo con un commit en main, un archivo modificado y uno sin
// seguimiento, y un subdirectorio sub/ con un archivo.
func e2Repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@hoom.dev")
	git(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "app.go", "package app\n")
	write(t, dir, "sub/sub.go", "package sub\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "inicial")
	write(t, dir, "app.go", "package app // tocado\n")
	write(t, dir, "nuevo.txt", "sin seguimiento\n")
	return dir
}

// e2Roturas son las formas de romper un repo que tiene .git: git falla
// enseguida en cualquier comando, empezando por rev-parse.
var e2Roturas = []struct {
	nombre string
	romper func(t *testing.T, dir string)
}{
	{"HEAD corrupto", func(t *testing.T, dir string) {
		if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("basura de la prueba e2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}},
	{"config corrupto", func(t *testing.T, dir string) {
		if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core\nbasura de la prueba e2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}},
	{".git archivo de un worktree cuyo gitdir ya no existe", func(t *testing.T, dir string) {
		if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
			t.Fatal(err)
		}
		gitdir := filepath.Join(t.TempDir(), "no-existe", "worktrees", "tarea")
		if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}},
	{".git directorio vacio", func(t *testing.T, dir string) {
		if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}},
}

// e2GitFallaEn exige que git de verdad falle en dir (rev-parse), para que el
// caso este inyectado; si no, saltea.
func e2GitFallaEn(t *testing.T, dir string) {
	t.Helper()
	c := exec.Command("git", "rev-parse", "--git-dir")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err == nil {
		t.Skipf("fixture: con el repo roto, git rev-parse no falla en este sistema: %s", out)
	}
}

// e2ConReloj corre f con un reloj de 60 s: un listado que no vuelve es un
// fallo del test, no un test colgado.
func e2ConReloj(t *testing.T, ca, que string, f func()) {
	t.Helper()
	listo := make(chan struct{})
	go func() {
		defer close(listo)
		f()
	}()
	select {
	case <-listo:
	case <-time.After(60 * time.Second):
		t.Fatalf("%s: %s no volvio en 60 s", ca, que)
	}
}

// CA-133, CA-442 (hallazgo 9ad39d): en un repo que TIENE .git y esta roto
// (HEAD corrupto, config corrupto, .git de un worktree cuyo gitdir ya no
// existe, .git vacio), git falla desde el primer rev-parse: TouchedErr
// devuelve error, no una lista vacia que el gate de scope toma por "nada
// cambio"; y HuellaConTope devuelve error y ninguna huella. Lo mismo
// pidiendolo desde un subdirectorio del repo (.git arriba). Guardas: con el
// repo sano, TouchedErr lista lo que cambio y HuellaConTope da una huella;
// y git de verdad falla en cada repo roto (si no, Skip).
func TestHallazgo_9ad39d_CA133_UnRepoRotoEsUnErrorNoUnaListaVacia(t *testing.T) {
	const ca = "CA-133, CA-442 (hallazgo 9ad39d, repo roto)"
	sano := e2Repo(t)
	m, err := TouchedErr(sano, "main")
	if err != nil || m["app.go"] == "" || m["nuevo.txt"] == "" {
		t.Fatalf("%s: guarda: con el repo sano, TouchedErr lista app.go y nuevo.txt sin error: %v %v", ca, m, err)
	}
	if h, err := HuellaConTope(sano, 30*time.Second); err != nil || h == "" {
		t.Fatalf("%s: guarda: con el repo sano, HuellaConTope da una huella sin error: %q %v", ca, h, err)
	}

	for _, r := range e2Roturas {
		for _, desde := range []string{"la raiz", "un subdirectorio"} {
			t.Run(r.nombre+" desde "+desde, func(t *testing.T) {
				dir := e2Repo(t)
				r.romper(t, dir)
				if desde == "un subdirectorio" {
					dir = filepath.Join(dir, "sub")
				}
				e2GitFallaEn(t, dir)

				var m map[string]string
				var err error
				e2ConReloj(t, ca, "TouchedErr", func() { m, err = TouchedErr(dir, "main") })
				if err == nil {
					t.Errorf("%s: con %s (pedido desde %s), git falla: TouchedErr devuelve error, no la lista %v sin error (el gate de scope la toma por \"nada cambio\")",
						ca, r.nombre, desde, m)
				}
				var h string
				e2ConReloj(t, ca, "HuellaConTope", func() { h, err = HuellaConTope(dir, 30*time.Second) })
				if err == nil || h != "" {
					t.Errorf("%s: con %s (pedido desde %s), git falla: HuellaConTope devuelve error y ninguna huella, no %q con error %v",
						ca, r.nombre, desde, h, err)
				}
			})
		}
	}
}

// CA-133 (hallazgo 9ad39d, guarda del contrato de siempre): en un
// directorio sin .git en el ni arriba (fuera de un repo) no hay nada que
// listar: TouchedErr da una lista vacia sin error, y HuellaConTope ninguna
// huella sin error. Lo mismo en un subdirectorio de ese directorio.
func TestHallazgo_9ad39d_CA133_FueraDeUnRepoListaVaciaSinError(t *testing.T) {
	const ca = "CA-133 (hallazgo 9ad39d, fuera de un repo)"
	afuera := t.TempDir()
	write(t, afuera, "sub/archivo.txt", "fuera de un repo\n")
	c := exec.Command("git", "rev-parse", "--git-dir")
	c.Dir = afuera
	if out, err := c.CombinedOutput(); err == nil {
		t.Skipf("fixture: el directorio temporal de este sistema esta dentro de un repo git: %s", out)
	}
	for _, dir := range []string{afuera, filepath.Join(afuera, "sub")} {
		var m map[string]string
		var err error
		e2ConReloj(t, ca, "TouchedErr", func() { m, err = TouchedErr(dir, "main") })
		if err != nil || len(m) != 0 {
			t.Errorf("%s: fuera de un repo (%s), TouchedErr da una lista vacia sin error: %v %v", ca, dir, m, err)
		}
		var h string
		e2ConReloj(t, ca, "HuellaConTope", func() { h, err = HuellaConTope(dir, 30*time.Second) })
		if err != nil || h != "" {
			t.Errorf("%s: fuera de un repo (%s), HuellaConTope no da huella ni error: %q %v", ca, dir, h, err)
		}
	}
}
