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
//     que recibe la review, ni en un rango simple ni con la base integrada;
//     tampoco un reemplazo plantado en otra base de refs que hoom hereda en
//     GIT_REPLACE_REF_BASE (ronda 7).
//
// Los fixtures estan en evidencia_de_la_review_helpers_test.go y
// evidencia_de_la_review_delta_test.go. Las guardas de los fixtures corren
// con los git de siempre (rdGit): TestMain aisla el git del binario
// (gittest.AislarGit: quita las variables de git que cambian lo que el
// fixture planta o lee, hallazgo 688441, y fija la config global y la del
// sistema, hallazgo 20261029T172636_8f2253). Los helpers de entorno que la
// ronda 6 habia dejado aca ya no estan, y lo que comparten este archivo y
// evidencia_de_la_review_hallazgos5_test.go (edrGitConEntorno) esta en el
// de los fixtures (hallazgo cd162c).
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

// edrReemplazo es donde un test planta el reemplazo de un commit: en
// refs/replace/ (git replace), o en otra base de refs que hoom hereda en
// GIT_REPLACE_REF_BASE.
type edrReemplazo struct {
	nombre string
	base   string // "" = refs/replace/
}

// edrReemplazos son los dos lugares. TestMain quito del entorno un
// GIT_REPLACE_REF_BASE heredado (hallazgo 688441): el test que lo quiere
// para hoom lo pone con t.Setenv (heredar) despues de plantar y guardar.
var edrReemplazos = []edrReemplazo{
	{"refs-replace", ""},
	{"GIT_REPLACE_REF_BASE-heredado", "refs/prueba-externa/"},
}

// entorno es lo que git necesita en el entorno para leer el reemplazo.
func (r edrReemplazo) entorno() []string {
	if r.base == "" {
		return nil
	}
	return []string{"GIT_REPLACE_REF_BASE=" + r.base}
}

// heredar pone en el entorno del proceso del test, el de hoom, lo que git
// necesita para leer el reemplazo.
func (r edrReemplazo) heredar(t *testing.T) {
	if r.base != "" {
		t.Setenv("GIT_REPLACE_REF_BASE", r.base)
	}
}

// edrReemplazar planta, en el lugar de r, el reemplazo de viejo: un commit
// con el arbol arbol y los padres padres, que git lee en lugar de viejo.
// Guarda: git lo honra con r.entorno(); y si r no es refs/replace/, sin esa
// variable no lo honra (la variable es lo que trae el reemplazo).
func edrReemplazar(t *testing.T, dir string, r edrReemplazo, viejo, arbol string, padres ...string) {
	t.Helper()
	args := []string{"commit-tree", arbol + "^{tree}", "-m", "commit falso que reemplaza a " + viejo[:12]}
	for _, p := range padres {
		args = append(args, "-p", p)
	}
	falso := rdGit(t, dir, args...)
	quiero := rdGit(t, dir, "rev-parse", arbol+"^{tree}")
	if r.base == "" {
		git(t, dir, "replace", "-f", viejo, falso)
	} else {
		git(t, dir, "update-ref", r.base+viejo, falso)
		if got := rdGit(t, dir, "rev-parse", viejo+"^{tree}"); got == quiero {
			t.Fatalf("4acfc8: fixture: sin GIT_REPLACE_REF_BASE=%s, git no honra %s%s", r.base, r.base, viejo[:12])
		}
	}
	if got := edrGitConEntorno(t, dir, r.entorno(), "rev-parse", viejo+"^{tree}"); got != quiero {
		t.Fatalf("4acfc8: fixture: git honra el reemplazo de %s en %s (su arbol es el de %s): %s", viejo[:12], r.nombre, arbol[:12], got)
	}
}

// Hallazgo 4acfc8 (CA-481, rango simple): despues de la review R1 (hasta D)
// la rama commitea X. Un reemplazo de D plantado hace que git lea D con el
// arbol de X: el diff de D a HEAD que da git queda vacio. `hoom review
// --delta` revisa igual lo real: la evidencia es la de hoy (Evidence contra
// una rama en D, armada antes de plantar el reemplazo), la medida es la de
// x.go, y nunca SIN REVISAR. El reemplazo esta en refs/replace/ o en otra
// base que hoom hereda en GIT_REPLACE_REF_BASE (edrReemplazos).
func TestHallazgo4acfc8_CA481_UnReplaceNoEscondeElCambioDeUnRango(t *testing.T) {
	for _, r := range edrReemplazos {
		t.Run(r.nombre, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, r1, d := edrConReview(t, "4acfc8", cx)
			x := rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X-REAL")})
			ev := rdOraculo(t, "4acfc8", root, d, edrSpec)
			edrReemplazar(t, root, r, d, x, rdSha(t, root, d+"^"))
			if out := edrGitConEntorno(t, root, r.entorno(), "diff", "--stat", d, x); out != "" {
				t.Fatalf("4acfc8: fixture: con el reemplazo, git no ve cambios de D a X: %q", out)
			}
			r.heredar(t)
			antes := cx.veces()
			res, out := edrRevisar(t, "4acfc8", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
			if res.Status != "revisado" || res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID || cx.veces() != antes+1 {
				t.Fatalf("4acfc8/CA-481 (%s): un reemplazo no esconde X: --delta revisa: %+v\n%s", r.nombre, res, out)
			}
			if !strings.Contains(cx.pedido(t, antes+1), raBloque(t, ev, edrSpec, true)) || res.EvidenceSHA256 != ev.SHA256 {
				t.Fatalf("4acfc8/CA-481 (%s): la review recibe la evidencia real de D a X:\n%s", r.nombre, cx.pedido(t, antes+1))
			}
			if primera := fmt.Sprintf("hoom review: 1 archivo cambiado, +%d/-0 lineas desde %s", edrLineasDe(edrCodigo("MARCA-X-REAL")), d[:12]); raLineas(out)[0] != primera {
				t.Fatalf("4acfc8/CA-481 (%s): la medida es la real (x.go):\n%q\nfue\n%q", r.nombre, primera, raLineas(out)[0])
			}
		})
	}
}

// Hallazgo 4acfc8 (CA-484, con la base integrada): el caso de CA-484 (X, un
// merge limpio de main con un cambio propio, Z) y un reemplazo de D que hace
// que git lea D con el arbol de HEAD y con I de padre: con el reemplazo, D
// "ya tiene" la base y todo el cambio. `hoom review --delta` mira los
// objetos reales: separa la base (base_integrada = I) y la evidencia y la
// medida son X, el commit de merge y Z, como sin reemplazo. El reemplazo
// esta en refs/replace/ o en otra base que hoom hereda en
// GIT_REPLACE_REF_BASE (edrReemplazos).
func TestHallazgo4acfc8_CA484_UnReplaceNoEscondeElCambioConLaBaseIntegrada(t *testing.T) {
	for _, r := range edrReemplazos {
		t.Run(r.nombre, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, r1, d, i, h := edrConBaseIntegrada(t, "4acfc8", cx)
			edrReemplazar(t, root, r, d, h, i)
			if out := edrGitConEntorno(t, root, r.entorno(), "diff", "--stat", d, h); out != "" {
				t.Fatalf("4acfc8: fixture: con el reemplazo, git no ve cambios de D a HEAD: %q", out)
			}
			esAncestro := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", i, d)
			esAncestro.Env = append(os.Environ(), r.entorno()...)
			if err := esAncestro.Run(); err != nil {
				t.Fatalf("4acfc8: fixture: con el reemplazo, git ve a I como ancestro de D: %v", err)
			}
			r.heredar(t)
			antes := cx.veces()
			res, out := edrRevisar(t, "4acfc8", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
			if res.Status != "revisado" || res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID || cx.veces() != antes+1 {
				t.Fatalf("4acfc8/CA-484 (%s): un reemplazo no esconde el cambio: --delta revisa: %+v\n%s", r.nombre, res, out)
			}
			if m := edrJSON(t, res); m["base_integrada"] != i {
				t.Fatalf("4acfc8/CA-484 (%s): con los objetos reales la base se separa: base_integrada %s: %v", r.nombre, i[:12], m["base_integrada"])
			}
			diff, _ := edrDiffDelPedido(t, "4acfc8", cx.pedido(t, antes+1))
			edrSoloLoNuevo(t, "4acfc8/CA-484", "con un reemplazo en "+r.nombre, diff)
			n, ins, del := edrTamanoXZ()
			if primera := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas desde %s", n, ins, del, d[:12]); raLineas(out)[0] != primera {
				t.Fatalf("4acfc8/CA-484 (%s): la medida es la real:\n%q\nfue\n%q", r.nombre, primera, raLineas(out)[0])
			}
		})
	}
}
