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
//   - 20261014T194026_186524 (Guardar, del refutador): despues de `git mv
//     app.go movido.go` en el espacio de trabajo, la tarjeta lista como sin
//     guardar el nombre nuevo Y el viejo, y Guardar commitea el rename
//     entero: en el commit app.go ya no esta y movido.go si, con su
//     contenido; el SaveResult nombra las dos rutas, y despues unsynced queda
//     vacio y el espacio de trabajo limpio. Antes, unsynced solo traia
//     movido.go y el borrado de app.go quedaba en el indice. Caso limite: un
//     borrado que ya esta en el indice sin rename (`git rm viejo.go`) se
//     guarda igual.
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

// h3GuardaLiteral corre Save (ca es el criterio y el hallazgo que se
// prueban, para los mensajes) y exige que el espacio de trabajo quede guardado
// con EXACTAMENTE enWt (nombres relativos al espacio de trabajo) y el arbol
// raiz con solo el item; lo ajeno del indice del raiz sigue ahi y la tarjeta
// queda sin nada sin guardar.
func h3GuardaLiteral(t *testing.T, ca, caso, root, wt string, enWt []string, expect []string) {
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
		t.Fatalf("%s: %s: Save guarda tal cual las rutas sin guardar (nombres, no patrones de git): %v", ca, caso, err)
	}
	porDir := map[string]SaveCommit{}
	for _, c := range res.Commits {
		porDir[c.Dir] = c
	}
	cw, okW := porDir[".hoom/worktrees/"+cbSlug]
	cr, okR := porDir["."]
	if len(res.Commits) != 2 || !okW || !okR {
		t.Fatalf("%s: %s: un commit por arbol: %+v", ca, caso, res.Commits)
	}

	// el SaveResult nombra exactamente las rutas literales
	if !cbMismos(cw.Paths, esperadas[1:]) {
		t.Fatalf("%s: %s: el SaveResult del espacio de trabajo nombra %q, nombro %q", ca, caso, esperadas[1:], cw.Paths)
	}
	if !cbMismos(cr.Paths, []string{h3Item}) {
		t.Fatalf("%s: %s: en el arbol raiz se guarda solo el item: %q", ca, caso, cr.Paths)
	}

	// el commit del espacio de trabajo es uno, nuevo, y lleva exactamente esas rutas
	if cw.SHA != cbHead(t, wt) || cbGit(t, wt, "rev-parse", "HEAD~1") != headWt {
		t.Fatalf("%s: %s: un commit nuevo en el espacio de trabajo (%s sobre %s): %+v", ca, caso, cbHead(t, wt), headWt, cw)
	}
	if got := h3DelCommit(t, wt); !cbMismos(got, enWt) {
		t.Fatalf("%s: %s: el commit del espacio de trabajo toca exactamente %q, toco %q", ca, caso, enWt, got)
	}

	// el del raiz, solo el item; lo ajeno sigue en el indice
	if cr.SHA != cbHead(t, root) || cbGit(t, root, "rev-parse", "HEAD~1") != headRoot {
		t.Fatalf("%s: %s: un commit nuevo en el arbol raiz: %+v", ca, caso, cr)
	}
	if got := h3DelCommit(t, root); !cbMismos(got, []string{h3Item}) {
		t.Fatalf("%s: %s: el commit del raiz lleva solo el item: %q", ca, caso, got)
	}
	if got := cbEnIndice(t, root); !cbMismos(got, []string{"ajeno.txt"}) {
		t.Fatalf("%s: %s: el archivo ajeno sigue en el indice del raiz sin commitear: %q", ca, caso, got)
	}

	// y no queda nada sin guardar
	if got := cbUnsynced(t, root); len(got) != 0 {
		t.Fatalf("%s: %s: despues de guardar, unsynced esta vacio (la tarjeta no sigue sin guardar): %q", ca, caso, got)
	}
	if st := cbGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("%s: %s: el espacio de trabajo queda limpio:\n%s", ca, caso, st)
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
			h3GuardaLiteral(t, "CA-344 (hallazgo a26f9d)", c.caso, root, wt, c.archivo, nil)
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
	h3GuardaLiteral(t, "CA-344 (hallazgo a26f9d)", "juntos", root, wt, enWt, expect)
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

// h3EnHEAD lista los archivos que tiene el commit HEAD de dir (con -z: ningun
// nombre entre comillas).
func h3EnHEAD(t *testing.T, dir string) []string {
	t.Helper()
	out := []string{}
	for _, p := range strings.Split(cbGit(t, dir, "ls-tree", "-z", "-r", "--name-only", "HEAD"), "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func h3Tiene(lista []string, x string) bool {
	for _, y := range lista {
		if y == x {
			return true
		}
	}
	return false
}

// CA-344 (hallazgo 186524): un rename en el indice del espacio de trabajo
// (`git mv app.go movido.go`) junto a un archivo modificado (viejo.go). La
// tarjeta lista como sin guardar el nombre nuevo y el viejo; Guardar commitea
// el rename entero (app.go ya no esta en el commit, movido.go si, con su
// contenido), el SaveResult nombra las dos rutas, y despues unsynced queda
// vacio y el espacio de trabajo limpio. Como la CLI (expect nil) y como el
// Studio (expect = unsynced), con el rename puro y con movido.go cambiado
// despues del mv.
func TestHallazgo_186524_GuardarCommiteaUnRenameEntero(t *testing.T) {
	const cambiado = "package app\n\nfunc Movido() {}\n"
	for _, c := range []struct {
		caso        string
		cambiaNuevo bool
		studio      bool
	}{
		{"rename puro, como la CLI", false, false},
		{"rename puro, como el Studio", false, true},
		{"rename y movido.go cambiado, como la CLI", true, false},
		{"rename y movido.go cambiado, como el Studio", true, true},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, wt := h3TareaCon(t)
			icGit(t, wt, "mv", "app.go", "movido.go")
			if c.cambiaNuevo {
				icEscribir(t, wt, "movido.go", cambiado)
			}
			icEscribir(t, wt, "viejo.go", "package app\n\nfunc Viejo() int { return 2 }\n")
			if st := cbGit(t, wt, "diff", "--cached", "--name-status", "-M"); !strings.Contains(st, "app.go\tmovido.go") || !strings.HasPrefix(st, "R") {
				t.Fatalf("CA-344: %s: fixture: el indice del espacio de trabajo tiene el rename app.go -> movido.go:\n%s", c.caso, st)
			}
			contenido, err := os.ReadFile(filepath.Join(wt, "movido.go"))
			if err != nil {
				t.Fatal(err)
			}

			enWt := []string{"app.go", "movido.go", "viejo.go"}
			got := cbUnsynced(t, root)
			if !h3Tiene(got, h3EnWt("app.go")) || !h3Tiene(got, h3EnWt("movido.go")) {
				t.Fatalf("CA-344 (hallazgo 186524): %s: con un rename en el indice la tarjeta lista como sin guardar el nombre viejo y el nuevo: %q", c.caso, got)
			}
			var expect []string
			if c.studio {
				expect = got
			}

			// un commit por arbol con las rutas exactas (las dos del rename), el
			// raiz con solo el item, lo ajeno en el indice, unsynced vacio y el
			// espacio de trabajo limpio
			h3GuardaLiteral(t, "CA-344 (hallazgo 186524)", c.caso, root, wt, enWt, expect)

			// en el commit del espacio de trabajo el viejo ya no esta y el nuevo
			// si, con el contenido que tenia el archivo
			arbol := h3EnHEAD(t, wt)
			if h3Tiene(arbol, "app.go") {
				t.Fatalf("CA-344 (hallazgo 186524): %s: en el commit del espacio de trabajo app.go ya no esta: %q", c.caso, arbol)
			}
			if !h3Tiene(arbol, "movido.go") {
				t.Fatalf("CA-344 (hallazgo 186524): %s: en el commit del espacio de trabajo esta movido.go: %q", c.caso, arbol)
			}
			if en := cbGit(t, wt, "show", "HEAD:movido.go"); en != strings.TrimSpace(string(contenido)) {
				t.Fatalf("CA-344 (hallazgo 186524): %s: movido.go se commitea con su contenido %q, quedo %q", c.caso, contenido, en)
			}
			if _, err := os.Stat(filepath.Join(wt, "app.go")); !os.IsNotExist(err) {
				t.Fatalf("CA-344 (hallazgo 186524): %s: app.go sigue sin existir en el espacio de trabajo: %v", c.caso, err)
			}
		})
	}
}

// CA-344 (hallazgo 186524, caso limite): la mitad "borrado" de un rename sin
// el rename. `git rm viejo.go` deja el borrado YA en el indice (ni el indice
// ni el disco tienen viejo.go), junto a app.go modificado. La tarjeta lo lista
// como sin guardar, Guardar commitea el borrado (viejo.go ya no esta en el
// commit), y despues unsynced queda vacio y el espacio de trabajo limpio.
func TestHallazgo_186524_GuardarCommiteaUnBorradoYaEnElIndice(t *testing.T) {
	for _, studio := range []bool{false, true} {
		caso := "como la CLI"
		if studio {
			caso = "como el Studio"
		}
		t.Run(caso, func(t *testing.T) {
			root, wt := h3TareaCon(t)
			icGit(t, wt, "rm", "-q", "viejo.go")
			icEscribir(t, wt, "app.go", "package app\n\nfunc Precio() int { return 1 }\n")
			if st := cbGit(t, wt, "diff", "--cached", "--name-status"); st != "D\tviejo.go" {
				t.Fatalf("CA-344: %s: fixture: el indice del espacio de trabajo tiene solo el borrado de viejo.go: %q", caso, st)
			}
			got := cbUnsynced(t, root)
			if !h3Tiene(got, h3EnWt("viejo.go")) {
				t.Fatalf("CA-344 (hallazgo 186524): %s: la tarjeta lista como sin guardar el borrado ya en el indice: %q", caso, got)
			}
			var expect []string
			if studio {
				expect = got
			}
			h3GuardaLiteral(t, "CA-344 (hallazgo 186524)", caso, root, wt, []string{"app.go", "viejo.go"}, expect)
			if h3Tiene(h3EnHEAD(t, wt), "viejo.go") {
				t.Fatalf("CA-344 (hallazgo 186524): %s: en el commit del espacio de trabajo viejo.go ya no esta: %q", caso, h3EnHEAD(t, wt))
			}
		})
	}
}
