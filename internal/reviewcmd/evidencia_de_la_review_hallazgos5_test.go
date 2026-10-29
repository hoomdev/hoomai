// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 5: el hallazgo 8d0272 de la delta 2, escrito desde el spec y la decision
// de Henry (2026-10-28): la base integrada que se descuenta sale de la
// historia real, y nada plantado en el repositorio achica la delta.
//
//   - 8d0272 (CA-484): un .git/info/grafts que declara que el merge H de la
//     rama tiene de padres a X y a una punta J de la base posterior a la
//     que H integro de verdad (I) hace que git elija J como merge-base de la
//     base con HEAD. Si J trae un cambio que la tarea tambien hizo, el merge
//     sintetico de D con J ya lo tiene y la delta lo pierde. La review mira
//     la historia real: base_integrada = I, y el cambio de la tarea sigue en
//     la evidencia y en la medida. Sin refs/replace: falla solo por el graft.
//     El graft esta en .git/info/grafts o en un archivo de afuera que hoom
//     hereda en GIT_GRAFT_FILE (ronda 7). TestMain quita del entorno
//     heredado las variables de git que cambian lo que el fixture planta o
//     lee (edrGitHeredado: un GIT_GRAFT_FILE heredado, hallazgos ff30ce y
//     d88bae), asi que el fixture y sus guardas ven el entorno limpio, y el
//     test le pone a hoom el GIT_GRAFT_FILE con t.Setenv; con un git que ya
//     no lee grafts el test se salta (hallazgo a21de0).
//
// Los fixtures estan en evidencia_de_la_review_helpers_test.go.
package reviewcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// edrMergeBaseCon es `git merge-base main HEAD` en dir con extra agregado al
// entorno (edrGitConEntorno).
func edrMergeBaseCon(t *testing.T, dir string, extra ...string) string {
	t.Helper()
	return edrGitConEntorno(t, dir, extra, "merge-base", "main", "HEAD")
}

// edrGraft es donde un test planta un graft: en el info/grafts del
// repositorio, o en un archivo de afuera que hoom hereda en GIT_GRAFT_FILE.
type edrGraft struct {
	nombre  string
	externo bool
}

// edrGrafts son los dos lugares. TestMain quito del entorno un
// GIT_GRAFT_FILE heredado (hallazgo d88bae): el test que lo quiere para hoom
// lo pone con t.Setenv despues de plantar y guardar.
var edrGrafts = []edrGraft{{"info-grafts", false}, {"GIT_GRAFT_FILE-heredado", true}}

// plantar escribe el graft de dir en el lugar de g (el commit c con esos
// padres) y devuelve lo que git necesita en el entorno para leerlo.
func (g edrGraft) plantar(t *testing.T, dir, c string, padres ...string) (entorno []string) {
	t.Helper()
	linea := c + " " + strings.Join(padres, " ") + "\n"
	if !g.externo {
		comun := rdGit(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
		write(t, comun, filepath.Join("info", "grafts"), linea)
		return nil
	}
	archivo := filepath.Join(t.TempDir(), "grafts")
	if err := os.WriteFile(archivo, []byte(linea), 0o644); err != nil {
		t.Fatal(err)
	}
	return []string{"GIT_GRAFT_FILE=" + archivo}
}

// Hallazgo 8d0272 (CA-484): despues de la review R1 (hasta D) la rama
// commitea X (x.go, codigo de la tarea), integra main en I (H, un merge
// limpio) y main sigue con J, que trae el mismo x.go. Un graft declara H con
// padres X y J: para git, el merge-base de main con HEAD pasa de I a J (las
// guardas lo comprueban, y que sin el graft es I). `hoom review --delta` y
// `--desde D` miran la historia real: delta con base_integrada = I, y la
// evidencia y la medida son x.go; con J como base integrada el merge
// sintetico ya traeria x.go y la delta quedaria vacia. El graft esta en
// .git/info/grafts o en un archivo que hoom hereda en GIT_GRAFT_FILE
// (edrGrafts).
func TestHallazgo8d0272_CA484_UnGraftNoAchicaLaDelta(t *testing.T) {
	for _, g := range edrGrafts {
		t.Run(g.nombre, func(t *testing.T) {
			for _, delta := range []bool{true, false} {
				caso := "--desde"
				if delta {
					caso = "--delta"
				}
				t.Run(caso, func(t *testing.T) {
					bin := raPATH(t)
					cx := raInstalar(t, bin, "codex", "")
					root, r1, d := edrConReview(t, "8d0272", cx)
					x := rdCommit(t, root, "X: codigo de la tarea", map[string]string{"x.go": edrCodigo("MARCA-X-TAREA")})
					i := edrMainAvanza(t, root, "I", map[string]string{"y.go": edrCodigo("MARCA-Y")})
					h := edrIntegrar(t, root, nil)
					j := edrMainAvanza(t, root, "J: main trae el mismo x.go", map[string]string{"x.go": edrCodigo("MARCA-X-TAREA")})
					if mb := edrMergeBaseCon(t, root); mb != i {
						t.Fatalf("8d0272: fixture: sin el graft, el merge-base de main con HEAD es I: %s", mb)
					}
					// quien planta el graft tambien apaga el aviso de desuso de git
					// (por stderr en cada comando): solo cambia la historia que git cree
					git(t, root, "config", "advice.graftFileDeprecated", "false")
					hostil := g.plantar(t, root, h, x, j)
					switch mb := edrMergeBaseCon(t, root, hostil...); mb {
					case j:
					case i:
						// hallazgo a21de0: git 3.0 (o uno compilado con
						// WITH_BREAKING_CHANGES) ya no lee info/grafts
						t.Skip("8d0272: el git instalado no lee grafts (el graft no cambia el merge-base): ese vector no existe con este git")
					default:
						t.Fatalf("8d0272: fixture: con el graft, git ve a J como merge-base de main con HEAD: %s", mb)
					}
					if mb := edrMergeBaseCon(t, root, "GIT_GRAFT_FILE=/dev/null"); mb != i {
						t.Fatalf("8d0272: fixture: sin leer el graft, el merge-base sigue siendo I: %s", mb)
					}
					for _, kv := range hostil {
						k, v, _ := strings.Cut(kv, "=")
						t.Setenv(k, v)
					}

					opt := Options{Provider: "codex", Spec: edrSpec, Delta: delta}
					if !delta {
						opt.Desde = d
					}
					antes := cx.veces()
					res, out := edrRevisar(t, "8d0272", root, opt)
					if res.Status != "revisado" || cx.veces() != antes+1 || res.Desde != d || res.Hasta != h {
						t.Fatalf("8d0272/CA-484: %s (%s): con el graft la delta no queda vacia: revisa de %s a %s: %+v\n%s", caso, g.nombre, d[:12], h[:12], res, out)
					}
					if delta && (res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID) {
						t.Fatalf("8d0272/CA-484 (%s): --delta es un delta de %s: %+v", g.nombre, r1.RecordID, res)
					}
					if m := edrJSON(t, res); m["base_integrada"] != i {
						t.Fatalf("8d0272/CA-484: %s (%s): base_integrada es el merge-base real (I %s), no la punta del graft (J %s): %v", caso, g.nombre, i[:12], j[:12], m["base_integrada"])
					}
					diff, _ := edrDiffDelPedido(t, "8d0272", cx.pedido(t, antes+1))
					if got := edrCabeceras(diff); !reflect.DeepEqual(got, []string{"x.go"}) || !strings.Contains(diff, "+// MARCA-X-TAREA") {
						t.Fatalf("8d0272/CA-484: %s (%s): el cambio de la tarea (x.go) sigue en la evidencia: %q\n%s", caso, g.nombre, got, diff)
					}
					if primera := fmt.Sprintf("hoom review: 1 archivo cambiado, +%d/-0 lineas desde %s", edrLineasDe(edrCodigo("MARCA-X-TAREA")), d[:12]); raLineas(out)[0] != primera {
						t.Fatalf("8d0272/CA-484: %s (%s): la medida es la de x.go:\n%q\nfue\n%q", caso, g.nombre, primera, raLineas(out)[0])
					}
				})
			}
		})
	}
}
