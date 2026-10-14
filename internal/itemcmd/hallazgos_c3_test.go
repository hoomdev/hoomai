// Tests de regresion de la review de 4 lentes de la cabina C3 sobre
// .hoom/specs/acciones-desde-la-tarjeta.md, en itemcmd (CA-344: `hoom item
// save <slug>` hace un commit por arbol con exactamente lo que la tarjeta
// tiene sin guardar):
//
//   - 20261014T180517_a26f9d (Guardar): las rutas sin guardar son nombres de
//     archivo, no patrones de git. Un archivo del espacio de trabajo llamado
//     `:nuevo.go`, `:!app.go` o `:(foo)x` se guarda tal cual: Save no falla,
//     el commit lleva exactamente esas rutas (ni una de menos ni una de mas),
//     el SaveResult las nombra y despues la tarjeta no tiene nada sin
//     guardar. Hoy git las lee como pathspecs con magia (`:` = desde la
//     raiz, `:!` = excluir, `:(foo)` = magia desconocida).
//   - 20261014T180437_42fa10 y su duplicado 20261014T180753_f2a6d3
//     (Guardar): Save commitea primero el espacio de trabajo y despues el
//     item en el arbol raiz. Si el segundo commit falla (un pre-commit del
//     repo que rechaza los commits con `.hoom/items/`), el error dice que el
//     espacio de trabajo YA quedo guardado y nombra ese commit: contiene los
//     primeros 12 caracteres de su sha. Hoy solo trae la salida del hook.
//
// Reusa los fixtures de save_test.go (cbRepoSave, cbTareaSave, cbUnsynced,
// cbHead, cbEnIndice, cbMismos) e itemcmd_test.go (icEscribir, icGit).
package itemcmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// h3Item es la ruta del item de la tarjeta, relativa a la raiz.
const h3Item = ".hoom/items/" + cbSlug + ".yaml"

// h3EnWt es la ruta (relativa a la raiz) de un archivo del espacio de
// trabajo de la tarjeta.
func h3EnWt(nombre string) string { return ".hoom/worktrees/" + cbSlug + "/" + nombre }

// h3DelCommit lista las rutas (relativas a dir) que toco el commit HEAD de
// dir, con -z para que ningun nombre llegue entre comillas.
func h3DelCommit(t *testing.T, dir string) []string {
	t.Helper()
	out := []string{}
	for _, p := range strings.Split(cbGit(t, dir, "diff-tree", "-z", "--no-commit-id", "--name-only", "-r", "HEAD"), "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// h3TareaCon arma una tarjeta con su tarea (como `hoom task start`), el item
// sin commitear y un archivo AJENO en el indice del arbol raiz, y en el
// espacio de trabajo SOLO los archivos que se pidan (app.go y viejo.go ya
// estan commiteados: escribirlos es modificarlos; cualquier otro es nuevo).
func h3TareaCon(t *testing.T, nombres ...string) (root, wt string) {
	t.Helper()
	root = cbRepoSave(t)
	if err := taskcmd.Start(root, cbSlug, "main"); err != nil {
		t.Fatalf("CA-344: fixture: task start: %v", err)
	}
	wt = filepath.Join(root, ".hoom", "worktrees", cbSlug)
	icEscribir(t, root, h3Item, cbItemYAML())
	icEscribir(t, root, "ajeno.txt", "esto es de la persona\n")
	icGit(t, root, "add", "ajeno.txt")
	for _, n := range nombres {
		icEscribir(t, wt, n, "package app // "+n+"\n")
	}
	return root, wt
}

// h3GuardaLiteral corre Save y exige que el espacio de trabajo quede guardado
// con EXACTAMENTE enWt (nombres relativos al espacio de trabajo) y el arbol
// raiz con solo el item; lo ajeno del indice del raiz sigue ahi y la tarjeta
// queda sin nada sin guardar.
func h3GuardaLiteral(t *testing.T, caso, root, wt string, enWt []string, expect []string) {
	t.Helper()
	esperadas := []string{h3Item}
	for _, n := range enWt {
		esperadas = append(esperadas, h3EnWt(n))
	}
	if got := cbUnsynced(t, root); !cbMismos(got, esperadas) {
		t.Fatalf("CA-344: %s: fixture: la tarjeta tiene sin guardar %q, no %q", caso, got, esperadas)
	}
	headRoot, headWt := cbHead(t, root), cbHead(t, wt)

	res, err := Save(root, "main", "high", cbSlug, expect)
	if err != nil {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: Save guarda los archivos por su nombre literal, no como patrones de git: %v", caso, err)
	}
	porDir := map[string]SaveCommit{}
	for _, c := range res.Commits {
		porDir[c.Dir] = c
	}
	cw, okW := porDir[".hoom/worktrees/"+cbSlug]
	cr, okR := porDir["."]
	if len(res.Commits) != 2 || !okW || !okR {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: un commit por arbol: %+v", caso, res.Commits)
	}

	// el SaveResult nombra exactamente las rutas literales
	if !cbMismos(cw.Paths, esperadas[1:]) {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: el SaveResult del espacio de trabajo nombra %q, nombro %q", caso, esperadas[1:], cw.Paths)
	}
	if !cbMismos(cr.Paths, []string{h3Item}) {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: en el arbol raiz se guarda solo el item: %q", caso, cr.Paths)
	}

	// el commit del espacio de trabajo es uno, nuevo, y lleva exactamente esas rutas
	if cw.SHA != cbHead(t, wt) || cbGit(t, wt, "rev-parse", "HEAD~1") != headWt {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: un commit nuevo en el espacio de trabajo (%s sobre %s): %+v", caso, cbHead(t, wt), headWt, cw)
	}
	if got := h3DelCommit(t, wt); !cbMismos(got, enWt) {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: el commit del espacio de trabajo toca exactamente %q, toco %q", caso, enWt, got)
	}

	// el del raiz, solo el item; lo ajeno sigue en el indice
	if cr.SHA != cbHead(t, root) || cbGit(t, root, "rev-parse", "HEAD~1") != headRoot {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: un commit nuevo en el arbol raiz: %+v", caso, cr)
	}
	if got := h3DelCommit(t, root); !cbMismos(got, []string{h3Item}) {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: el commit del raiz lleva solo el item: %q", caso, got)
	}
	if got := cbEnIndice(t, root); !cbMismos(got, []string{"ajeno.txt"}) {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: el archivo ajeno sigue en el indice del raiz sin commitear: %q", caso, got)
	}

	// y no queda nada sin guardar
	if got := cbUnsynced(t, root); len(got) != 0 {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: despues de guardar, unsynced esta vacio (la tarjeta no sigue sin guardar): %q", caso, got)
	}
	if st := cbGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("CA-344 (hallazgo a26f9d): %s: el espacio de trabajo queda limpio:\n%s", caso, st)
	}
}

// h3Nombres son nombres de archivo validos que git leeria como pathspecs con
// magia: `:` (desde la raiz: `:nuevo.go` es nuevo.go), `:!` (excluir) y
// `:(foo)` (magia desconocida: git aborta).
var h3Nombres = []string{":nuevo.go", ":!app.go", ":(foo)x"}

// CA-344 (hallazgo a26f9d): cada nombre por separado, como lo pide la CLI
// (expect nil): Save lo guarda tal cual. Cada uno solo y, cuando el patron
// nombraria a otro archivo, junto a ese archivo cambiado: `:nuevo.go` no se
// confunde con nuevo.go, y `:!app.go` no deja afuera a app.go. (`:!app.go`
// solo no se prueba: "todo menos app.go" es justo ese archivo.)
func TestHallazgo_a26f9d_GuardarTomaCadaNombreLiteral(t *testing.T) {
	for _, c := range []struct {
		caso    string
		archivo []string
	}{
		{":nuevo.go solo", []string{":nuevo.go"}},
		{":nuevo.go con nuevo.go nuevo", []string{":nuevo.go", "nuevo.go"}},
		{":!app.go con app.go modificado", []string{":!app.go", "app.go"}},
		{":(foo)x solo", []string{":(foo)x"}},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, wt := h3TareaCon(t, c.archivo...)
			h3GuardaLiteral(t, c.caso, root, wt, c.archivo, nil)
		})
	}
}

// CA-344 (hallazgo a26f9d): los tres nombres juntos con cambios normales que
// se les parecen (app.go modificado, nuevo.go nuevo, viejo.go borrado), con
// expect = unsynced como lo pide el Studio: el commit lleva las seis rutas, ni
// `:!app.go` deja afuera a app.go ni `:nuevo.go` se confunde con nuevo.go.
func TestHallazgo_a26f9d_GuardarNombresRarosJuntoAArchivosNormales(t *testing.T) {
	root, wt := cbTareaSave(t)
	for _, n := range h3Nombres {
		icEscribir(t, wt, n, "package app // "+n+"\n")
	}
	enWt := append([]string{"app.go", "nuevo.go", "viejo.go"}, h3Nombres...)
	expect := []string{h3Item}
	for _, n := range enWt {
		expect = append(expect, h3EnWt(n))
	}
	sort.Strings(expect)
	h3GuardaLiteral(t, "juntos", root, wt, enWt, expect)
	if _, err := os.Stat(filepath.Join(wt, "viejo.go")); !os.IsNotExist(err) {
		t.Fatalf("CA-344 (hallazgo a26f9d): viejo.go sigue borrado: %v", err)
	}
}

// h3HookQueRechazaItems instala en el repo un pre-commit que rechaza SOLO los
// commits que traen algo bajo .hoom/items/ (el del arbol raiz); el del
// espacio de trabajo pasa. Solo usa git y builtins de sh.
func h3HookQueRechazaItems(t *testing.T, root string) {
	t.Helper()
	hooks := filepath.Join(root, ".git", "hooks")
	icGit(t, root, "config", "core.hooksPath", hooks)
	icEscribir(t, root, ".git/hooks/pre-commit", `#!/bin/sh
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
}

// CA-344 (hallazgos 42fa10 y f2a6d3): el commit del espacio de trabajo queda
// hecho y el del raiz lo rechaza un hook. Save devuelve error, y ese error
// nombra el commit que SI quedo: los primeros 12 caracteres de su sha.
func TestHallazgo_42fa10_SiFallaElCommitDelRaizElErrorNombraElDelEspacio(t *testing.T) {
	root, wt := cbTareaSave(t)
	if got := cbUnsynced(t, root); !cbMismos(got, cbEsperadas) {
		t.Fatalf("CA-344: fixture: la tarjeta tiene sin guardar %v, no %v", got, cbEsperadas)
	}
	h3HookQueRechazaItems(t, root)
	headRoot, headWt := cbHead(t, root), cbHead(t, wt)

	_, err := Save(root, "main", "high", cbSlug, nil)
	if err == nil {
		t.Fatal("CA-344 (hallazgo 42fa10): con el commit del raiz rechazado por el hook, Save devuelve error")
	}
	if h := cbHead(t, root); h != headRoot {
		t.Fatalf("CA-344: fixture: el hook rechaza el commit del raiz (HEAD %s -> %s)", headRoot, h)
	}
	hecho := cbHead(t, wt)
	if hecho == headWt {
		t.Fatalf("CA-344 (hallazgo 42fa10): el espacio de trabajo se guarda primero y su commit queda hecho aunque falle el del raiz (HEAD sigue en %s); error: %v", headWt, err)
	}
	if !strings.Contains(err.Error(), hecho[:12]) {
		t.Fatalf("CA-344 (hallazgo 42fa10): el error dice que el espacio de trabajo ya quedo guardado y nombra ese commit (%s): %v", hecho[:12], err)
	}
}
