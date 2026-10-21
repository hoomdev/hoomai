// Tests adversariales del hallazgo 20261021T145451_68644c (high, risk) de la
// review 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md:
// CA-280, la carpeta de la tarea es el worktree .hoom/worktrees/<slug>;
// CA-290, task done --force), sobre los verbos que descartan y cierran
// (CA-346 de .hoom/specs/acciones-desde-la-tarjeta.md), con la ampliacion
// del contrato tras el refutador. Escritos desde el contrato, nunca desde la
// implementacion:
//
// .hoom/worktrees/<slug> es la tarea solo si su .git es un ARCHIVO regular
// hacia <common-dir de la raiz>/worktrees/<id>, el registro <ese
// gitdir>/gitdir nombra exactamente <la carpeta>/.git, git responde desde
// ahi esa carpeta como top y el common-dir de la raiz, su HEAD esta en
// refs/heads/hoom/<slug>, y ninguna variable GIT_* del entorno cambia la
// respuesta. Con cualquier carpeta de worktreetest.Casos (o una carpeta plana con
// GIT_DIR de la raiz en el entorno), Discardable, RunDiscard (con y sin
// --yes), Discard y Done (con --force primero, y sin) fallan y no tocan
// nada: ni legit (su rama, su indice, sus archivos con su trabajo sin
// commitear), ni la raiz (main, su indice, su trabajo sin commitear), ni la
// carpeta. Guarda: legit sigue andando.
//
// Cada caso se prueba con un entorno NUEVO para descartar y otro para
// cerrar. Repos git reales bajo t.TempDir.
//
// La matriz (worktreetest.Casos) y su armado viven en internal/worktreetest
// (hallazgo 20261021T174702_00e2fc): aca queda solo la operacion propia.
package taskcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// t68Entorno: el proyecto con legit (commit propio, app.go modificado y
// wip-legit.txt sin commitear), trabajo sin commitear en la raiz, y la
// carpeta del caso slug (o la plana, que devuelve su entorno GIT_*).
func t68Entorno(t *testing.T, slug string) (root string, vars map[string]string) {
	t.Helper()
	root, legit := worktreetest.Proyecto(t, Start)
	worktreetest.Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
	worktreetest.Escribir(t, legit, "wip-legit.txt", "sin guardar\n")
	worktreetest.Escribir(t, root, "app.go", "package app\n\n// trabajo sin guardar de la raiz\n")
	worktreetest.Escribir(t, root, "raiz-nuevo.go", "package app\n")
	if slug == "plana" {
		return root, worktreetest.Plana(t, root)
	}
	worktreetest.Armar(t, root, slug, true)
	return root, nil
}

// t68Rechazado exige que el verbo haya fallado sin tocar nada y sin listar
// el trabajo de legit ni el de la raiz. Devuelve false si el verbo rompio
// algo: los siguientes ya no tendrian un entorno intacto.
func t68Rechazado(t *testing.T, ca, verbo, slug string, err error, antes, despues, salida string) bool {
	t.Helper()
	sano := true
	if despues != antes {
		t.Errorf("%s: hallazgo 68644c: %s con %q toco legit, la raiz o la carpeta:\n%s\nsalida:\n%s", ca, verbo, slug, worktreetest.Cambios(antes, despues), salida)
		sano = false
	}
	if err == nil {
		t.Errorf("%s: hallazgo 68644c: %s con %q debe fallar (la carpeta no es la tarea), fue nil\nsalida:\n%s", ca, verbo, slug, salida)
	}
	for _, ajeno := range []string{"wip-legit.txt", "legit.go", "raiz-nuevo.go"} {
		if strings.Contains(salida, ajeno) {
			t.Errorf("%s: hallazgo 68644c: %s con %q no lista archivos de legit ni de la raiz (%s):\n%s", ca, verbo, slug, ajeno, salida)
			break
		}
	}
	return sano
}

func t68Descartar(t *testing.T, root, slug string, vars map[string]string) {
	t.Helper()
	antes := worktreetest.Foto(t, root)
	var lista []string
	var err error
	worktreetest.ConEntorno(vars, func() { lista, err = Discardable(root, slug) })
	if !t68Rechazado(t, "CA-346", "Discardable", slug, err, antes, worktreetest.Foto(t, root), strings.Join(lista, "\n")) {
		return
	}
	var out bytes.Buffer
	worktreetest.ConEntorno(vars, func() { err = RunDiscard(root, slug, false, false, &out) })
	if !t68Rechazado(t, "CA-346", "RunDiscard sin --yes", slug, err, antes, worktreetest.Foto(t, root), out.String()) {
		return
	}
	pref := ".hoom/worktrees/" + slug + "/"
	for _, expect := range [][]string{lista, nil, {pref + "app.go", pref + slug + ".go"}} {
		var res DiscardResult
		worktreetest.ConEntorno(vars, func() { res, err = Discard(root, slug, expect) })
		if !t68Rechazado(t, "CA-346", fmt.Sprintf("Discard (expect %v)", expect), slug, err, antes, worktreetest.Foto(t, root), fmt.Sprintf("%+v", res)) {
			return
		}
	}
	for _, asJSON := range []bool{false, true} {
		out.Reset()
		worktreetest.ConEntorno(vars, func() { err = RunDiscard(root, slug, true, asJSON, &out) })
		if !t68Rechazado(t, "CA-346", fmt.Sprintf("RunDiscard --yes (json=%v)", asJSON), slug, err, antes, worktreetest.Foto(t, root), out.String()) {
			return
		}
	}
}

func t68Cerrar(t *testing.T, root, slug string, vars map[string]string) {
	t.Helper()
	antes := worktreetest.Foto(t, root)
	for _, force := range []bool{true, false} {
		var err error
		worktreetest.ConEntorno(vars, func() { err = Done(root, slug, "main", force) })
		if !t68Rechazado(t, "CA-290", fmt.Sprintf("Done(force=%v)", force), slug, err, antes, worktreetest.Foto(t, root), "") {
			return
		}
	}
}

// Hallazgo 68644c (high). CA-346 y CA-290: con cualquier carpeta de
// worktreetest.Casos, y con una carpeta plana con GIT_DIR de la raiz en el entorno,
// task discard y task done (--force incluido) fallan sin tocar legit, la
// raiz ni la carpeta.
func TestHallazgo68644c_CA346_LoQueNoEsLaTareaNoSeDescartaNiSeCierra(t *testing.T) {
	for _, slug := range worktreetest.SlugsConPlana() {
		slug := slug
		t.Run(slug+"/descartar", func(t *testing.T) {
			root, vars := t68Entorno(t, slug)
			t68Descartar(t, root, slug, vars)
		})
		t.Run(slug+"/cerrar", func(t *testing.T) {
			root, vars := t68Entorno(t, slug)
			t68Cerrar(t, root, slug, vars)
		})
	}
}

// Guarda del hallazgo 68644c. CA-346 y CA-290: con todas las carpetas de
// worktreetest.Casos al lado, legit sigue andando: Discardable lista su trabajo sin
// guardar, Discard lo vuelve a HEAD sin tocar la raiz ni las carpetas de al
// lado, y Done --force lo quita (la rama queda, con su commit).
func TestHallazgo68644c_CA346_GuardaLegitSigueAndando(t *testing.T) {
	root, _ := t68Entorno(t, "evil")
	for _, c := range worktreetest.Casos[1:] {
		worktreetest.Armar(t, root, c.Slug, true)
	}
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	raiz := worktreetest.Git(t, root, "status", "--porcelain", "--untracked-files=all")
	vecinas := func() string {
		var b strings.Builder
		for _, c := range worktreetest.Casos {
			b.WriteString(worktreetest.Archivos(t, filepath.Join(root, ".hoom", "worktrees", c.Slug)) + "\n")
		}
		return b.String()
	}
	antesVecinas := vecinas()
	want := []string{".hoom/worktrees/legit/app.go", ".hoom/worktrees/legit/wip-legit.txt"}
	lista, err := Discardable(root, "legit")
	if err != nil || !cbMismos(lista, want) {
		t.Fatalf("CA-346: guarda 68644c: Discardable de legit lista su trabajo %v, dio %v, %v", want, lista, err)
	}
	if _, err := Discard(root, "legit", lista); err != nil {
		t.Fatalf("CA-346: guarda 68644c: Discard de legit funciona: %v", err)
	}
	if st := worktreetest.Git(t, legit, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("CA-346: guarda 68644c: despues de Discard legit queda en HEAD:\n%s", st)
	}
	if ahora := worktreetest.Git(t, root, "status", "--porcelain", "--untracked-files=all"); ahora != raiz {
		t.Fatalf("CA-346: guarda 68644c: Discard de legit no toca la raiz:\nantes:\n%s\ndespues:\n%s", raiz, ahora)
	}
	if vecinas() != antesVecinas {
		t.Fatalf("CA-346: guarda 68644c: Discard de legit no toca las carpetas de al lado")
	}
	head := worktreetest.Git(t, legit, "rev-parse", "HEAD")
	if err := Done(root, "legit", "main", true); err != nil {
		t.Fatalf("CA-290: guarda 68644c: Done --force de legit funciona: %v", err)
	}
	if _, err := os.Stat(legit); !os.IsNotExist(err) {
		t.Fatalf("CA-290: guarda 68644c: Done --force quita el worktree de legit: %v", err)
	}
	if got := worktreetest.Git(t, root, "rev-parse", "refs/heads/hoom/legit"); got != head {
		t.Fatalf("CA-290: guarda 68644c: Done --force deja la rama hoom/legit en su commit %s, esta en %s", head, got)
	}
}
