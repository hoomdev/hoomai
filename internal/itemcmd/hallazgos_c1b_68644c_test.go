// Tests adversariales del hallazgo 20261021T145451_68644c (high, risk) de la
// review 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md:
// CA-280, el trabajo de la tarjeta esta en el worktree .hoom/worktrees/<slug>;
// CA-286, lo sin sincronizar del worktree), sobre `hoom item save` (CA-344 de
// .hoom/specs/acciones-desde-la-tarjeta.md, el "Guardar" del Studio), con la
// ampliacion del contrato tras el refutador. Escritos desde el contrato,
// nunca desde la implementacion:
//
// .hoom/worktrees/<slug> es la tarea solo si su .git es un ARCHIVO regular
// hacia <common-dir de la raiz>/worktrees/<id>, el registro <ese
// gitdir>/gitdir nombra exactamente <la carpeta>/.git, git responde desde
// ahi esa carpeta como top y el common-dir de la raiz, su HEAD esta en
// refs/heads/hoom/<slug>, y ninguna variable GIT_* del entorno cambia la
// respuesta. Con cualquier carpeta de worktreetest.Casos (o una carpeta plana con
// GIT_DIR de la raiz en el entorno), guardar la tarjeta (Save y `hoom item
// save`) falla, no commitea nada y no toca legit (su rama, su indice y sus
// archivos) ni la raiz (main, su indice y su trabajo sin commitear) ni la
// carpeta. Guarda: guardar legit sigue andando.
//
// La matriz (worktreetest.Casos) y su armado viven en internal/worktreetest
// (hallazgo 20261021T174702_00e2fc): aca queda solo la operacion propia.
package itemcmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// t68Entorno: el proyecto con legit (commit propio, app.go modificado y
// wip-legit.txt sin commitear), trabajo sin commitear en la raiz, la carpeta
// del caso slug (o la plana, que devuelve su entorno GIT_*) y el item de
// slug en la raiz: la tarjeta existe. Con slug "", todas las carpetas de
// worktreetest.Casos y ningun item.
func t68Entorno(t *testing.T, slug string) (root string, vars map[string]string) {
	t.Helper()
	root, legit := worktreetest.Proyecto(t, taskcmd.Start)
	worktreetest.Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
	worktreetest.Escribir(t, legit, "wip-legit.txt", "sin guardar\n")
	worktreetest.Escribir(t, root, "app.go", "package app\n\n// trabajo sin guardar de la raiz\n")
	worktreetest.Escribir(t, root, "raiz-nuevo.go", "package app\n")
	if slug == "" {
		for _, c := range worktreetest.Casos {
			worktreetest.Armar(t, root, c.Slug, true)
		}
		return root, nil
	}
	t28Item(t, root, slug)
	if slug == "plana" {
		return root, worktreetest.Plana(t, root)
	}
	worktreetest.Armar(t, root, slug, true)
	return root, nil
}

// Hallazgo 68644c (high). CA-280 y CA-286: guardar la tarjeta de cualquier
// carpeta de worktreetest.Casos, o de una carpeta plana con GIT_DIR de la raiz en el
// entorno (Save y `hoom item save`, cada uno en un entorno nuevo), falla, no
// commitea nada y no toca legit, la raiz ni la carpeta.
func TestHallazgo68644c_CA280_SaveDeLoQueNoEsLaTareaNoTocaNada(t *testing.T) {
	for _, slug := range worktreetest.SlugsConPlana() {
		slug := slug
		t.Run(slug+"/Save", func(t *testing.T) {
			root, vars := t68Entorno(t, slug)
			antes := worktreetest.Foto(t, root)
			var res SaveResult
			var err error
			worktreetest.ConEntorno(vars, func() { res, err = Save(root, "main", "high", slug, nil) })
			if err == nil {
				t.Errorf("CA-280: hallazgo 68644c: Save de %q debe fallar (la carpeta no es la tarea), guardo: %+v", slug, res)
			}
			if despues := worktreetest.Foto(t, root); despues != antes {
				t.Errorf("CA-280: hallazgo 68644c: Save de %q toco legit, la raiz o la carpeta:\n%s", slug, worktreetest.Cambios(antes, despues))
			}
		})
		t.Run(slug+"/item-save", func(t *testing.T) {
			root, vars := t68Entorno(t, slug)
			antes := worktreetest.Foto(t, root)
			req, perr := Parse([]string{"save", slug})
			if perr != nil {
				t.Fatalf("fixture: Parse(save %s): %v", slug, perr)
			}
			var out, errb strings.Builder
			var err error
			worktreetest.ConEntorno(vars, func() { err = Execute(root, "main", "high", req, &out, &errb) })
			if err == nil {
				t.Errorf("CA-280: hallazgo 68644c: `hoom item save %s` debe fallar, salio bien:\n%s", slug, out.String())
			}
			for _, ajeno := range []string{"wip-legit.txt", "legit.go", "raiz-nuevo.go"} {
				if strings.Contains(out.String()+errb.String(), ajeno) {
					t.Errorf("CA-280: hallazgo 68644c: `hoom item save %s` no nombra archivos de legit ni de la raiz (%s):\n%s%s", slug, ajeno, out.String(), errb.String())
					break
				}
			}
			if despues := worktreetest.Foto(t, root); despues != antes {
				t.Errorf("CA-280: hallazgo 68644c: `hoom item save %s` toco legit, la raiz o la carpeta:\n%s", slug, worktreetest.Cambios(antes, despues))
			}
		})
	}
}

// Guarda del hallazgo 68644c. CA-280: con todas las carpetas de worktreetest.Casos al
// lado, guardar legit sigue andando: un commit en su worktree con su
// trabajo, y las carpetas de al lado y el trabajo ajeno de la raiz quedan
// igual.
func TestHallazgo68644c_CA280_GuardaSaveDeLegitSigueAndando(t *testing.T) {
	root, _ := t68Entorno(t, "")
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	t28Item(t, root, "legit")
	vecinas := func() string {
		var b strings.Builder
		for _, c := range worktreetest.Casos {
			b.WriteString(worktreetest.Archivos(t, filepath.Join(root, ".hoom", "worktrees", c.Slug)) + "\n")
		}
		return b.String()
	}
	antesVecinas := vecinas()
	head := cbHead(t, legit)
	res, err := Save(root, "main", "high", "legit", nil)
	if err != nil {
		t.Fatalf("CA-280: guarda 68644c: Save de legit funciona: %v", err)
	}
	if cbHead(t, legit) == head || len(res.Commits) == 0 {
		t.Fatalf("CA-280: guarda 68644c: Save commitea en el worktree de legit: %+v", res)
	}
	if got := cbDelCommit(t, legit); !cbMismos(got, []string{"app.go", "wip-legit.txt"}) {
		t.Fatalf("CA-280: guarda 68644c: el commit de legit lleva su trabajo: %v", got)
	}
	if st := cbGit(t, legit, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("CA-280: guarda 68644c: legit queda guardado:\n%s", st)
	}
	if vecinas() != antesVecinas {
		t.Fatalf("CA-280: guarda 68644c: guardar legit no toca las carpetas de al lado")
	}
	st := cbGit(t, root, "status", "--porcelain", "--untracked-files=all")
	if !strings.Contains(st, "app.go") || !strings.Contains(st, "raiz-nuevo.go") {
		t.Fatalf("CA-280: guarda 68644c: el trabajo ajeno de la raiz sigue sin commitear:\n%s", st)
	}
}
