// Tests de regresion de la review de 4 lentes de la cabina C3 sobre
// .hoom/specs/acciones-desde-la-tarjeta.md, por el Studio (CA-345):
//
//   - 20261014T180437_42fa10 y su duplicado 20261014T180753_f2a6d3
//     (Guardar): POST /api/board/{slug}/save llama a la misma funcion que
//     `hoom item save`, que commitea primero el espacio de trabajo y despues
//     el item en el arbol raiz. Si el segundo commit falla (un pre-commit del
//     repo que rechaza los commits con `.hoom/items/`), la respuesta es 409 y
//     su error nombra el commit del espacio de trabajo que SI quedo hecho
//     (los primeros 12 caracteres de su sha), para que la persona sepa que esa
//     parte ya esta guardada. Hoy el 409 solo trae la salida del hook.
//
// Fixtures de acciones_tarjeta_test.go (atPATH, atConEspacio, atTarjeta,
// atPedir, atCuerpoPaths, atGit) y tablero_test.go (tbProyecto, tbEscribir,
// tbErrorJSON). El PATH tiene solo git y sh de verdad.
package servecmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CA-345 (hallazgos 42fa10 y f2a6d3): con las rutas de unsynced y un hook que
// rechaza solo el commit del arbol raiz, el save responde 409 y el error trae
// el sha corto del commit que quedo en el espacio de trabajo.
func TestHallazgo_42fa10_GuardarDesdeLaTarjetaConElCommitDelRaizRechazado(t *testing.T) {
	atPATH(t, nil)
	dir := tbProyecto(t)
	const slug = "a-medias"
	const ca = "CA-345 (hallazgo 42fa10)"
	wt := atConEspacio(t, dir, slug)
	tbEscribir(t, wt, "app.go", "package app\n\nfunc Cambio() int { return 2 }\n")
	tbEscribir(t, wt, "nuevo.go", "package app\n\nfunc Nuevo() {}\n")
	unsynced := atOrdenadas([]string{
		".hoom/items/" + slug + ".yaml",
		".hoom/worktrees/" + slug + "/app.go",
		".hoom/worktrees/" + slug + "/nuevo.go",
	})
	if got := atTarjeta(t, dir, slug).Unsynced; !atMismas(got, unsynced) {
		t.Fatalf("%s: fixture: unsynced de la tarjeta es %v, se esperaba %v", ca, got, unsynced)
	}

	// un pre-commit que rechaza SOLO los commits que traen .hoom/items/ (git y
	// builtins de sh: el PATH no tiene nada mas)
	hooks := filepath.Join(dir, ".git", "hooks")
	gitRun(t, dir, "config", "core.hooksPath", hooks)
	tbEscribir(t, dir, ".git/hooks/pre-commit", `#!/bin/sh
for f in $(git diff --cached --name-only); do
  case "$f" in
  .hoom/items/*)
    echo 'el hook de la casa no deja commitear items' >&2
    exit 1 ;;
  esac
done
exit 0
`)
	if err := os.Chmod(filepath.Join(hooks, "pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	headRaiz := atGit(t, dir, "rev-parse", "HEAD")
	headWt := atGit(t, wt, "rev-parse", "HEAD")

	s := newServer(t, dir)
	rec := atPedir(t, ca, s, http.MethodPost, "/api/board/"+slug+"/save", s.Token(), atCuerpoPaths(unsynced))
	msg := tbErrorJSON(t, ca, rec, http.StatusConflict)

	if h := atGit(t, dir, "rev-parse", "HEAD"); h != headRaiz {
		t.Fatalf("%s: fixture: el hook rechaza el commit del arbol raiz (HEAD %s -> %s)", ca, headRaiz, h)
	}
	hecho := atGit(t, wt, "rev-parse", "HEAD")
	if hecho == headWt {
		t.Fatalf("%s: el espacio de trabajo se guarda primero y su commit queda hecho aunque falle el del raiz (HEAD sigue en %s); 409: %q", ca, headWt, msg)
	}
	if !strings.Contains(msg, hecho[:12]) {
		t.Fatalf("%s: el 409 dice que el espacio de trabajo ya quedo guardado y nombra ese commit (%s): %q", ca, hecho[:12], msg)
	}
}
