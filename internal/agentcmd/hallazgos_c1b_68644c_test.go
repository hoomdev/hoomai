// Tests adversariales del hallazgo 20261021T145451_68644c (high, risk) de la
// review 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md,
// CA-280: el trabajo de la tarea vive en su worktree .hoom/worktrees/<slug>),
// sobre `hoom agent --task`, con la ampliacion del contrato tras el
// refutador. Escritos desde el contrato, nunca desde la implementacion:
//
// .hoom/worktrees/<slug> es la tarea solo si su .git es un ARCHIVO regular
// hacia <common-dir de la raiz>/worktrees/<id>, el registro <ese
// gitdir>/gitdir nombra exactamente <la carpeta>/.git, git responde desde
// ahi esa carpeta como top y el common-dir de la raiz, su HEAD esta en
// refs/heads/hoom/<slug>, y ninguna variable GIT_* del entorno cambia la
// respuesta. Con cualquier carpeta de worktreetest.Casos (o una carpeta plana con
// GIT_DIR de la raiz en el entorno), el sobre con --task se niega con un
// error, el provider no corre, no quedan runs y legit (rama, indice,
// archivos), la raiz y la carpeta quedan igual. Guarda: el sobre de legit
// corre en legit.
//
// La matriz (worktreetest.Casos) y su armado viven en internal/worktreetest
// (hallazgo 20261021T174702_00e2fc): aca queda solo la operacion propia.
package agentcmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// t68Entorno: el proyecto con legit (commit propio y app.go sin commitear),
// la carpeta del caso slug (o la plana, que devuelve su entorno GIT_*; o
// todas las de worktreetest.Casos con slug "") y un provider falso que anota su cwd
// fisico en la marca.
func t68Entorno(t *testing.T, slug string) (root, marca string, vars map[string]string) {
	t.Helper()
	root, legit := worktreetest.Proyecto(t, taskcmd.Start)
	worktreetest.Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
	marca = filepath.Join(filepath.Dir(root), "provider-corrio-en.txt")
	fakeProvider(t, "claude", "pwd -P >> '"+marca+"'\nexit 0\n")
	switch slug {
	case "":
		for _, c := range worktreetest.Casos {
			worktreetest.Armar(t, root, c.Slug, true)
		}
	case "plana":
		vars = worktreetest.Plana(t, root)
	default:
		worktreetest.Armar(t, root, slug, true)
	}
	return root, marca, vars
}

// Hallazgo 68644c (high). CA-280: `hoom agent --role scout --task <slug>`
// con cualquier carpeta de worktreetest.Casos, o con una carpeta plana y GIT_DIR de
// la raiz en el entorno, se niega con un error, el provider no corre, no
// deja runs y no toca legit, la raiz ni la carpeta.
func TestHallazgo68644c_CA280_AgentConLoQueNoEsLaTareaNoCorre(t *testing.T) {
	for _, slug := range worktreetest.SlugsConPlana() {
		slug := slug
		t.Run(slug, func(t *testing.T) {
			root, marca, vars := t68Entorno(t, slug)
			antes := worktreetest.Foto(t, root)
			var res Result
			var err error
			worktreetest.ConEntorno(vars, func() {
				res, err = Run(root, "main", Options{Role: "scout", Provider: "claude", Task: slug, Prompt: "explora"}, io.Discard)
			})
			if err == nil {
				t.Errorf("CA-280: hallazgo 68644c: el sobre con Task %q se niega con un error (la carpeta no es la tarea), corrio: %+v", slug, res)
			}
			if raw, errLeer := os.ReadFile(marca); errLeer == nil {
				t.Errorf("CA-280: hallazgo 68644c: con Task %q el provider no corre, corrio en:\n%s", slug, raw)
			}
			if n := runsCount(t, root); n != 0 {
				t.Errorf("CA-280: hallazgo 68644c: el sobre con Task %q no deja runs (%d en .hoom/runs)", slug, n)
			}
			if despues := worktreetest.Foto(t, root); despues != antes {
				t.Errorf("CA-280: hallazgo 68644c: el sobre con Task %q toco legit, la raiz o la carpeta:\n%s", slug, worktreetest.Cambios(antes, despues))
			}
		})
	}
}

// Guarda del hallazgo 68644c. CA-280: con todas las carpetas de worktreetest.Casos al
// lado, el sobre de legit corre en su worktree.
func TestHallazgo68644c_CA280_GuardaAgentEnLegitSigueAndando(t *testing.T) {
	root, marca, _ := t68Entorno(t, "")
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	if _, err := Run(root, "main", Options{Role: "scout", Provider: "claude", Task: "legit", Prompt: "explora"}, io.Discard); err != nil {
		t.Fatalf("CA-280: guarda 68644c: el sobre de legit corre: %v", err)
	}
	raw, err := os.ReadFile(marca)
	if err != nil {
		t.Fatalf("CA-280: guarda 68644c: el provider de legit corrio: %v", err)
	}
	real, _ := filepath.EvalSymlinks(legit)
	if got := strings.TrimSpace(string(raw)); got != real {
		t.Fatalf("CA-280: guarda 68644c: el sobre de legit corre en su worktree (%s), corrio en %s", real, got)
	}
}
