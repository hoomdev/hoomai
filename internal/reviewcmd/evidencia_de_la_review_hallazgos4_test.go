// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 4: los hallazgos de la delta 1 de la review de la correccion, escritos
// desde el spec y la decision de Henry (2026-10-28): lo que el merge
// sintetico no puede llevar de un repositorio cae a la review completa o a
// un error, nunca a una delta mas chica; lo barato si se lleva.
//
//   - 6f228c (CA-484): en un clon shallow donde D e I comparten un commit
//     de la frontera shallow, la delta que separa la base anda: delta con
//     base_integrada, como en un clon entero.
//   - 4acfc8 (CA-481, CA-484): la evidencia y la medida salen de los
//     objetos reales: un refs/replace plantado en el repositorio no cambia lo
//     que recibe la review, ni en un rango simple ni con la base integrada.
//
// Los fixtures estan en evidencia_de_la_review_helpers_test.go y
// evidencia_de_la_review_delta_test.go.
package reviewcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gittest"
)

// edrVariablesDeLectura son las variables del entorno con las que git deja
// de leer lo que un test planta en el repositorio: los reemplazos
// (refs/replace) y los grafts (hallazgo ff30ce). Las lecturas del fixture
// (sus guardas) las quitan del entorno que heredan, venga de donde venga el
// go test; hoom, que corre en el proceso del test, las hereda como esten y
// tiene que fijar las suyas.
var edrVariablesDeLectura = []string{"GIT_GRAFT_FILE", "GIT_NO_REPLACE_OBJECTS", "GIT_REPLACE_REF_BASE"}

// edrEntornoDelFixture es el entorno de este proceso sin edrVariablesDeLectura,
// con extra al final.
func edrEntornoDelFixture(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		quitar := false
		for _, v := range edrVariablesDeLectura {
			if strings.HasPrefix(kv, v+"=") {
				quitar = true
				break
			}
		}
		if !quitar {
			env = append(env, kv)
		}
	}
	return append(env, extra...)
}

// edrGitDelFixture corre git en dir con edrEntornoDelFixture y devuelve su
// stdout sin espacios alrededor (git puede avisar por stderr).
func edrGitDelFixture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, edrEntornoDelFixture()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fixture: git %v en %s: %v", args, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------- 6f228c

// edrClonShallowConBaseIntegrada arma el caso del hallazgo 6f228c. En el
// origen: main con M0 y S; la rama feature desde S con D (el spec x y
// codigo); main con I; la rama integra main sin conflicto (H) y commitea Z.
// El clon es de feature sola con --depth=4 (Z, H, D e I, S): S queda en la
// frontera shallow, y es el merge-base de D con I. En el clon, main apunta a
// I y la tarea x tiene una review completa (a mano) que llega a D. Devuelve el
// clon, la review, D, I y Z.
func edrClonShallowConBaseIntegrada(t *testing.T) (clon string, r1 Record, d, i, z string) {
	t.Helper()
	origen := raRepo(t, "")
	rdCommit(t, origen, "S", map[string]string{"s.go": edrCodigo("MARCA-S")})
	git(t, origen, "checkout", "-q", "main")
	git(t, origen, "merge", "-q", "--ff-only", "feature")
	git(t, origen, "checkout", "-q", "feature")
	s := rdSha(t, origen, "HEAD")
	d = rdCommit(t, origen, "D", map[string]string{edrSpec: "# Spec x\n\n- CA-1: algo.\n", "antes.go": edrCodigo("MARCA-DE-ANTES")})
	i = edrMainAvanza(t, origen, "I", map[string]string{"y.go": edrCodigo("MARCA-Y")})
	edrIntegrar(t, origen, nil)
	z = rdCommit(t, origen, "Z", map[string]string{"z.go": edrCodigo("MARCA-Z")})
	if mb := rdGit(t, origen, "merge-base", d, i); mb != s {
		t.Fatalf("6f228c: fixture: el merge-base de D con I es S: %s", mb)
	}

	padre, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clon = filepath.Join(padre, "clon")
	git(t, padre, "clone", "-q", "--depth=4", "--single-branch", "--branch", "feature", "file://"+origen, clon)
	gittest.ApagarMantenimiento(t, clon) // un clon no hereda el .git/config de su origen
	git(t, clon, "config", "user.email", "test@hoom.dev")
	git(t, clon, "config", "user.name", "hoom test")
	git(t, clon, "branch", "-q", "main", i)
	if raw, err := os.ReadFile(rdGit(t, clon, "rev-parse", "--path-format=absolute", "--git-path", "shallow")); err != nil || !strings.Contains(string(raw), s) {
		t.Fatalf("6f228c: fixture: S (%s) esta en la frontera shallow del clon", s[:12])
	}
	if mb := rdGit(t, clon, "merge-base", d, i); mb != s {
		t.Fatalf("6f228c: fixture: en el clon, git encuentra el merge-base de D con I (S): %s", mb)
	}
	r1 = Record{ID: "20261028T120000_5a11ab", CreatedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC), Task: "x", Spec: edrSpec,
		Lenses: []string{LenteDominante}, Provider: "codex", Writer: "claude", Cross: CrossYes, WritersDeclared: []string{},
		Findings: []string{}, Usage: []LensUsage{}, Isolated: true, Desde: s, Hasta: d, Cobertura: CoberturaCompleta, Base: "main"}
	rdRegistroAMano(t, clon, r1)
	return clon, r1, d, i, z
}

// Hallazgo 6f228c (CA-484): en el clon shallow, `hoom review --delta` (y
// --desde D) separa la base como en un clon entero: revisa, delta con
// desde_review y base_integrada = I, y la evidencia es Z sin lo que trajo
// main. Nunca un error de git por un padre que el clon no tiene.
func TestHallazgo6f228c_CA484_EnUnClonShallowLaDeltaSeparaLaBase(t *testing.T) {
	for _, delta := range []bool{true, false} {
		caso := "--desde"
		if delta {
			caso = "--delta"
		}
		t.Run(caso, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			clon, r1, d, i, z := edrClonShallowConBaseIntegrada(t)
			opt := Options{Provider: "codex", Spec: edrSpec, Delta: delta}
			if !delta {
				opt.Desde = d
			}
			edrLimpio(t, "6f228c", clon)
			res, err, out := rdCorrer(t, "6f228c", clon, opt)
			if err != nil || res.Status != "revisado" || cx.veces() != 1 || res.Desde != d || res.Hasta != z {
				t.Fatalf("6f228c/CA-484: %s en un clon shallow revisa de %s a %s: %+v %v\n%s", caso, d[:12], z[:12], res, err, out)
			}
			if delta && (res.Cobertura != CoberturaDelta || res.DesdeReview != r1.ID) {
				t.Fatalf("6f228c/CA-484: --delta es un delta de %s: %+v", r1.ID, res)
			}
			if m := edrJSON(t, res); m["base_integrada"] != i {
				t.Fatalf("6f228c/CA-484: %s separa la base: base_integrada %s: %v", caso, i[:12], m["base_integrada"])
			}
			if strings.Contains(out, "no se separa") {
				t.Fatalf("6f228c/CA-484: %s: el clon shallow no es un motivo para la review completa:\n%s", caso, out)
			}
			diff, _ := edrDiffDelPedido(t, "6f228c", cx.pedido(t, 1))
			if got := edrCabeceras(diff); !reflect.DeepEqual(got, []string{"z.go"}) {
				t.Fatalf("6f228c/CA-484: %s: la evidencia es Z, sin lo que trajo main: %q\n%s", caso, got, diff)
			}
		})
	}
}

// ---------------------------------------------------------------- 4acfc8

// edrReemplazar planta refs/replace/<viejo>: un commit con el arbol arbol y
// los padres padres, que git (sin GIT_NO_REPLACE_OBJECTS) lee en lugar de
// viejo. Guarda: git lo honra, aunque el go test herede
// GIT_NO_REPLACE_OBJECTS (la guarda corre con edrEntornoDelFixture).
func edrReemplazar(t *testing.T, dir, viejo, arbol string, padres ...string) {
	t.Helper()
	args := []string{"commit-tree", arbol + "^{tree}", "-m", "commit falso que reemplaza a " + viejo[:12]}
	for _, p := range padres {
		args = append(args, "-p", p)
	}
	falso := rdGit(t, dir, args...)
	git(t, dir, "replace", "-f", viejo, falso)
	if got := edrGitDelFixture(t, dir, "rev-parse", viejo+"^{tree}"); got != edrGitDelFixture(t, dir, "rev-parse", arbol+"^{tree}") {
		t.Fatalf("4acfc8: fixture: git honra refs/replace/%s (su arbol es el de %s): %s", viejo[:12], arbol[:12], got)
	}
}

// Hallazgo 4acfc8 (CA-481, rango simple): despues de la review R1 (hasta D)
// la rama commitea X. Un refs/replace/<D> plantado hace que git lea D con
// el arbol de X: el diff de D a HEAD que da git queda vacio. `hoom review
// --delta` revisa igual lo real: la evidencia es la de hoy (Evidence contra
// una rama en D, armada antes de plantar el reemplazo), la medida es la de
// x.go, y nunca SIN REVISAR.
func TestHallazgo4acfc8_CA481_UnReplaceNoEscondeElCambioDeUnRango(t *testing.T) {
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root, r1, d := edrConReview(t, "4acfc8", cx)
	x := rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X-REAL")})
	ev := rdOraculo(t, "4acfc8", root, d, edrSpec)
	edrReemplazar(t, root, d, x, rdSha(t, root, d+"^"))
	if out := edrGitDelFixture(t, root, "diff", "--stat", d, x); out != "" {
		t.Fatalf("4acfc8: fixture: con el reemplazo, git no ve cambios de D a X: %q", out)
	}
	antes := cx.veces()
	res, out := edrRevisar(t, "4acfc8", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	if res.Status != "revisado" || res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID || cx.veces() != antes+1 {
		t.Fatalf("4acfc8/CA-481: un refs/replace no esconde X: --delta revisa: %+v\n%s", res, out)
	}
	if !strings.Contains(cx.pedido(t, antes+1), raBloque(t, ev, edrSpec, true)) || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("4acfc8/CA-481: la review recibe la evidencia real de D a X:\n%s", cx.pedido(t, antes+1))
	}
	if primera := fmt.Sprintf("hoom review: 1 archivo cambiado, +%d/-0 lineas desde %s", edrLineasDe(edrCodigo("MARCA-X-REAL")), d[:12]); raLineas(out)[0] != primera {
		t.Fatalf("4acfc8/CA-481: la medida es la real (x.go):\n%q\nfue\n%q", primera, raLineas(out)[0])
	}
}

// Hallazgo 4acfc8 (CA-484, con la base integrada): el caso de CA-484 (X, un
// merge limpio de main con un cambio propio, Z) y un refs/replace/<D> que
// hace que git lea D con el arbol de HEAD y con I de padre: con el
// reemplazo, D "ya tiene" la base y todo el cambio. `hoom review --delta`
// mira los objetos reales: separa la base (base_integrada = I) y la
// evidencia y la medida son X, el commit de merge y Z, como sin reemplazo.
func TestHallazgo4acfc8_CA484_UnReplaceNoEscondeElCambioConLaBaseIntegrada(t *testing.T) {
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root, r1, d, i, h := edrConBaseIntegrada(t, "4acfc8", cx)
	edrReemplazar(t, root, d, h, i)
	if out := edrGitDelFixture(t, root, "diff", "--stat", d, h); out != "" {
		t.Fatalf("4acfc8: fixture: con el reemplazo, git no ve cambios de D a HEAD: %q", out)
	}
	esAncestro := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", i, d)
	esAncestro.Env = edrEntornoDelFixture()
	if err := esAncestro.Run(); err != nil {
		t.Fatalf("4acfc8: fixture: con el reemplazo, git ve a I como ancestro de D: %v", err)
	}
	antes := cx.veces()
	res, out := edrRevisar(t, "4acfc8", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	if res.Status != "revisado" || res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID || cx.veces() != antes+1 {
		t.Fatalf("4acfc8/CA-484: un refs/replace no esconde el cambio: --delta revisa: %+v\n%s", res, out)
	}
	if m := edrJSON(t, res); m["base_integrada"] != i {
		t.Fatalf("4acfc8/CA-484: con los objetos reales la base se separa: base_integrada %s: %v", i[:12], m["base_integrada"])
	}
	diff, _ := edrDiffDelPedido(t, "4acfc8", cx.pedido(t, antes+1))
	edrSoloLoNuevo(t, "4acfc8/CA-484", "con refs/replace", diff)
	n, ins, del := edrTamanoXZ()
	if primera := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas desde %s", n, ins, del, d[:12]); raLineas(out)[0] != primera {
		t.Fatalf("4acfc8/CA-484: la medida es la real:\n%q\nfue\n%q", primera, raLineas(out)[0])
	}
}
