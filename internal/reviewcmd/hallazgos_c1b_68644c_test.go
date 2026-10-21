// Tests adversariales del hallazgo 20261021T145451_68644c (high, risk) de la
// review 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md:
// CA-280, el trabajo de la tarea vive en su worktree .hoom/worktrees/<slug>;
// CA-293, la review con --task), sobre `hoom review --task`, con la
// ampliacion del contrato tras el refutador. Escritos desde el contrato,
// nunca desde la implementacion:
//
// .hoom/worktrees/<slug> es la tarea solo si su .git es un ARCHIVO regular
// hacia <common-dir de la raiz>/worktrees/<id>, el registro <ese
// gitdir>/gitdir nombra exactamente <la carpeta>/.git, git responde desde
// ahi esa carpeta como top y el common-dir de la raiz, su HEAD esta en
// refs/heads/hoom/<slug>, y ninguna variable GIT_* del entorno cambia la
// respuesta. Con cualquier carpeta de worktreetest.Casos (o una carpeta plana con
// GIT_DIR en el entorno), la review con --task falla (error, o un Result que
// no es "revisado" con exit distinto de 0), el reviewer no corre y legit
// (rama, indice, archivos), la raiz y la carpeta quedan igual. Guarda: la
// review de legit corre en legit.
//
// Cada carpeta queda LIMPIA vista por git desde adentro (una copia del arbol
// al que apunta): una carpeta sucia la protegeria por otra razon (la review
// revisa solo lo commiteado).
//
// La matriz (worktreetest.Casos) y su armado viven en internal/worktreetest
// (hallazgo 20261021T174702_00e2fc): aca queda solo la operacion propia.
package reviewcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// t68Entorno: el proyecto con legit (con su commit: algo que revisar), un
// reviewer falso que deja rastro en log, y la carpeta limpia del caso slug
// (o todas las de worktreetest.Casos con slug ""). La plana es una copia de legit sin
// .git, con GIT_DIR = el gitdir de legit en el entorno que devuelve.
func t68Entorno(t *testing.T, slug string) (root, log string, vars map[string]string) {
	t.Helper()
	root, legit := worktreetest.Proyecto(t, taskcmd.Start)
	log = htReviewer(t, "")
	switch slug {
	case "":
		for _, c := range worktreetest.Casos {
			worktreetest.Armar(t, root, c.Slug, false)
		}
	case "plana":
		worktreetest.Copiar(t, legit, filepath.Join(root, ".hoom", "worktrees", "plana"))
		vars = map[string]string{"GIT_DIR": filepath.Join(root, ".git", "worktrees", "legit")}
	default:
		worktreetest.Armar(t, root, slug, false)
		if slug != "evil-inexistente" {
			if st := worktreetest.Git(t, filepath.Join(root, ".hoom", "worktrees", slug), "status", "--porcelain", "--untracked-files=all"); st != "" {
				t.Fatalf("fixture: desde %s git ve el arbol limpio, ve:\n%s", slug, st)
			}
		}
	}
	return root, log, vars
}

// Hallazgo 68644c (high). CA-280 y CA-293: `hoom review --task <slug>` con
// cualquier carpeta de worktreetest.Casos, o con una carpeta plana y GIT_DIR en el
// entorno, falla, el reviewer no corre y no se toca legit, la raiz ni la
// carpeta.
func TestHallazgo68644c_CA293_ReviewDeLoQueNoEsLaTareaNoCorre(t *testing.T) {
	for _, slug := range worktreetest.SlugsConPlana() {
		slug := slug
		t.Run(slug, func(t *testing.T) {
			root, log, vars := t68Entorno(t, slug)
			antes := worktreetest.Foto(t, root)
			var out bytes.Buffer
			var res Result
			var err error
			worktreetest.ConEntorno(vars, func() {
				res, err = Run(root, "main", Options{Lens: "risk", Provider: "codex", Task: slug}, &out)
			})
			if err == nil && (res.Status == "revisado" || res.ExitCode == 0) {
				t.Errorf("CA-293: hallazgo 68644c: la review con Task %q debe fallar (la carpeta no es la tarea), termino %q exit %d:\n%s",
					slug, res.Status, res.ExitCode, out.String())
			}
			if raw, errLeer := os.ReadFile(log); errLeer == nil {
				t.Errorf("CA-280: hallazgo 68644c: con Task %q el reviewer no corre, corrio:\n%s", slug, raw)
			}
			if despues := worktreetest.Foto(t, root); despues != antes {
				t.Errorf("CA-280: hallazgo 68644c: la review con Task %q toco legit, la raiz o la carpeta:\n%s", slug, worktreetest.Cambios(antes, despues))
			}
		})
	}
}

// Guarda del hallazgo 68644c. CA-293: con todas las carpetas de worktreetest.Casos al
// lado, la review de legit corre en su worktree y termina revisado.
func TestHallazgo68644c_CA293_GuardaReviewDeLegitSigueAndando(t *testing.T) {
	root, log, _ := t68Entorno(t, "")
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	htRevisar(t, root, Options{Lens: "risk", Provider: "codex", Task: "legit"})
	htTodas(t, htPasadas(t, log), 1, "legit", htReal(t, legit))
}
