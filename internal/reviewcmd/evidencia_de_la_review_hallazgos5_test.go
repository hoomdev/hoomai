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
//     Las guardas del fixture no heredan GIT_GRAFT_FILE ni
//     GIT_NO_REPLACE_OBJECTS (hallazgo ff30ce), y con un git que ya no lee
//     grafts el test se salta (hallazgo a21de0).
//
// Los fixtures estan en evidencia_de_la_review_helpers_test.go.
package reviewcmd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// edrMergeBaseCon es `git merge-base main HEAD` en dir con el entorno del
// fixture (sin un GIT_GRAFT_FILE heredado: hallazgo ff30ce) y extra,
// leyendo solo su salida (git puede avisar por stderr que info/grafts esta
// en desuso).
func edrMergeBaseCon(t *testing.T, dir string, extra ...string) string {
	t.Helper()
	cmd := exec.Command("git", "merge-base", "main", "HEAD")
	cmd.Dir, cmd.Env = dir, edrEntornoDelFixture(extra...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fixture: git merge-base main HEAD en %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// edrPlantarGraft escribe el info/grafts del repositorio de dir: el commit c
// con esos padres.
func edrPlantarGraft(t *testing.T, dir, c string, padres ...string) {
	t.Helper()
	comun := rdGit(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	write(t, comun, filepath.Join("info", "grafts"), c+" "+strings.Join(padres, " ")+"\n")
}

// Hallazgo 8d0272 (CA-484): despues de la review R1 (hasta D) la rama
// commitea X (x.go, codigo de la tarea), integra main en I (H, un merge
// limpio) y main sigue con J, que trae el mismo x.go. Un graft declara H con
// padres X y J: para git, el merge-base de main con HEAD pasa de I a J (las
// guardas lo comprueban, y que sin el graft es I). `hoom review --delta` y
// `--desde D` miran la historia real: delta con base_integrada = I, y la
// evidencia y la medida son x.go; con J como base integrada el merge
// sintetico ya traeria x.go y la delta quedaria vacia.
func TestHallazgo8d0272_CA484_UnGraftNoAchicaLaDelta(t *testing.T) {
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
			edrPlantarGraft(t, root, h, x, j)
			switch mb := edrMergeBaseCon(t, root); mb {
			case j:
			case i:
				// hallazgo a21de0: git 3.0 (o uno compilado con
				// WITH_BREAKING_CHANGES) ya no lee info/grafts
				t.Skip("8d0272: el git instalado no lee .git/info/grafts (el graft no cambia el merge-base): ese vector no existe con este git")
			default:
				t.Fatalf("8d0272: fixture: con el graft, git ve a J como merge-base de main con HEAD: %s", mb)
			}
			if mb := edrMergeBaseCon(t, root, "GIT_GRAFT_FILE=/dev/null"); mb != i {
				t.Fatalf("8d0272: fixture: sin leer el graft, el merge-base sigue siendo I: %s", mb)
			}

			opt := Options{Provider: "codex", Spec: edrSpec, Delta: delta}
			if !delta {
				opt.Desde = d
			}
			antes := cx.veces()
			res, out := edrRevisar(t, "8d0272", root, opt)
			if res.Status != "revisado" || cx.veces() != antes+1 || res.Desde != d || res.Hasta != h {
				t.Fatalf("8d0272/CA-484: %s: con el graft la delta no queda vacia: revisa de %s a %s: %+v\n%s", caso, d[:12], h[:12], res, out)
			}
			if delta && (res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID) {
				t.Fatalf("8d0272/CA-484: --delta es un delta de %s: %+v", r1.RecordID, res)
			}
			if m := edrJSON(t, res); m["base_integrada"] != i {
				t.Fatalf("8d0272/CA-484: %s: base_integrada es el merge-base real (I %s), no la punta del graft (J %s): %v", caso, i[:12], j[:12], m["base_integrada"])
			}
			diff, _ := edrDiffDelPedido(t, "8d0272", cx.pedido(t, antes+1))
			if got := edrCabeceras(diff); !reflect.DeepEqual(got, []string{"x.go"}) || !strings.Contains(diff, "+// MARCA-X-TAREA") {
				t.Fatalf("8d0272/CA-484: %s: el cambio de la tarea (x.go) sigue en la evidencia: %q\n%s", caso, got, diff)
			}
			if primera := fmt.Sprintf("hoom review: 1 archivo cambiado, +%d/-0 lineas desde %s", edrLineasDe(edrCodigo("MARCA-X-TAREA")), d[:12]); raLineas(out)[0] != primera {
				t.Fatalf("8d0272/CA-484: %s: la medida es la de x.go:\n%q\nfue\n%q", caso, primera, raLineas(out)[0])
			}
		})
	}
}
