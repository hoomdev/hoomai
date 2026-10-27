// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md
// (CA-483, CA-485, CA-486, enmienda 1) en el tablero, con la evidencia EN
// DISCO (repos git reales, CardFor) y `hoom review` de verdad
// (reviewcmd.Run con un codex falso):
//
//   - "codigo" despues de una review es lo que cambio en las rutas de la
//     evidencia, que ahora son todo menos el estado de hoom: un commit que
//     solo toca .hoom/semgrep/gate.sh devuelve la tarjeta a Review (y su
//     --delta lo revisa con lentes); uno que solo toca el estado de hoom no.
//   - CambioDespues gana la base: lo que trajo un merge limpio de la base
//     despues de la review no cuenta (CA-485); un merge que no se separa
//     (conflicto) si (CA-486).
//
// Los fixtures (rdGrande, rdTarjeta, rdCuenta, rdNoCuenta, r1Codex,
// tj.review, ...) estan en review_por_diferencia_test.go y
// review_por_diferencia_ronda1_test.go.
package boardcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

// edrCodex es r1Codex con un codex falso que ademas guarda el pedido de cada
// invocacion (lo que le llega por stdin) en dir/stdin.<n>.
func edrCodex(t *testing.T) (dir string) {
	t.Helper()
	bin, dir := t.TempDir(), t.TempDir()
	s := "#!/bin/sh\nd='" + dir + "'\n" +
		"n=$(cat \"$d/n\" 2>/dev/null || echo 0); n=$((n+1)); echo $n > \"$d/n\"\n" +
		"cat > \"$d/stdin.$n\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return dir
}

// edrPedido es el pedido que recibio la invocacion n del codex de edrCodex.
func edrPedido(t *testing.T, dir string, n int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "stdin."+strconv.Itoa(n)))
	if err != nil {
		t.Fatalf("la invocacion %d del codex falso nunca corrio: %v", n, err)
	}
	return string(raw)
}

// edrMotivoDelta es el motivo de CA-428 para la review r.
func edrMotivoDelta(id, hasta string) string {
	return "la review " + id + " cubre hasta " + hasta[:12] + " y el codigo cambio despues"
}

// edrPideElDelta exige la tarjeta en Review con el motivo y el siguiente
// paso del delta para la review id que cubre hasta hasta.
func edrPideElDelta(t *testing.T, ca, caso string, c Card, id, hasta string) {
	t.Helper()
	bdCol(t, ca+" ("+caso+")", c, ColReview)
	bdPrimero(t, ca+" ("+caso+")", c, edrMotivoDelta(id, hasta))
	if c.Next != rdNextDelta {
		t.Fatalf("%s (%s): el siguiente paso es %q, fue %q", ca, caso, rdNextDelta, c.Next)
	}
	if c.Evidence.ReviewID != "" {
		t.Fatalf("%s (%s): la review de antes ya no es la review de la tarjeta: %+v", ca, caso, c.Evidence)
	}
}

// edrMainAvanza commitea en main (el checkout del proyecto) los archivos y
// devuelve el sha de main.
func edrMainAvanza(t *testing.T, tj *rdTarjeta, msg string, archivos map[string]string) string {
	t.Helper()
	var rutas []string
	for ruta, cuerpo := range archivos {
		bdEscribir(t, tj.root, ruta, cuerpo)
		rutas = append(rutas, ruta)
	}
	bdGit(t, tj.root, append([]string{"add", "--"}, rutas...)...)
	bdGit(t, tj.root, append([]string{"commit", "-q", "-m", msg, "--"}, rutas...)...)
	return bdGit(t, tj.root, "rev-parse", "HEAD")
}

// edrIntegrar integra main en el worktree de la tarea con un merge real sin
// conflicto.
func edrIntegrar(t *testing.T, tj *rdTarjeta) {
	t.Helper()
	bdGit(t, tj.wt, "merge", "-q", "--no-ff", "--no-edit", "main")
}

// ---------------------------------------------------------------- CA-483

// CA-483: "despues de una review de 4 lentes, un commit que solo toca
// .hoom/semgrep/gate.sh devuelve la tarjeta a Review con el siguiente paso
// --delta, y ese --delta lo revisa con lentes, no con SIN REVISAR". De punta
// a punta: R1 de 4 lentes con hoom review; un commit solo de gate.sh (y su
// verde): la tarjeta vuelve a Review con el motivo del delta; el --delta
// revisa (las 4 lentes de la regla sobre el cambio entero) y su evidencia es
// el diff de gate.sh; despues, la tarjeta pasa a Tu aceptacion.
func TestCA483_UnCommitSoloDeSemgrepDevuelveLaTarjetaAReview(t *testing.T) {
	pedidos := edrCodex(t)
	tj := rdGrande(t)
	bdEscribir(t, tj.wt, ".hoom/semgrep/gate.sh", "set -e\nsemgrep --config .hoom/semgrep/reglas.yaml\n")
	bdCommitear(t, tj.wt, "el gate de semgrep")
	tj.verde(t)
	r1, out := tj.review(t, "CA-483", false)
	r1Completa(t, "CA-483", "450 lineas", r1, out, reviewcmd.Lentes)
	rdCuenta(t, "CA-483 (fixture)", "R1 de 4 lentes", tj.card(t), r1.RecordID)

	r1Esperar()
	bdEscribir(t, tj.wt, ".hoom/semgrep/gate.sh", "set -e\n# GATE-QUE-YA-NO-CORRE-NADA\nexit 0\n")
	bdCommitear(t, tj.wt, "solo el gate")
	tj.verde(t)
	edrPideElDelta(t, "CA-483", "commit solo de .hoom/semgrep/gate.sh", tj.card(t), r1.RecordID, r1.Hasta)

	r1Esperar()
	r2, out := tj.review(t, "CA-483", true)
	if r2.Status != "revisado" || r2.Cobertura != reviewcmd.CoberturaDelta || r2.DesdeReview != r1.RecordID || len(r2.Lenses) != 4 || len(r2.Passes) != 4 {
		t.Fatalf("CA-483: el --delta revisa el commit de gate.sh con lentes (las 4 de la regla sobre el cambio entero), no SIN REVISAR: %+v\n%s", r2, out)
	}
	p := edrPedido(t, pedidos, 5)
	if !strings.Contains(p, "diff --git a/.hoom/semgrep/gate.sh b/.hoom/semgrep/gate.sh") || !strings.Contains(p, "+# GATE-QUE-YA-NO-CORRE-NADA") {
		t.Fatalf("CA-483: la evidencia del --delta trae el diff de gate.sh:\n%s", p)
	}
	rdCuenta(t, "CA-483", "despues del --delta", tj.card(t), r2.RecordID)
}

// CA-483: "un commit que solo toca el estado de hoom (un veredicto,
// hallazgos, un registro de review) no la devuelve". Despues de una review
// completa de 4 lentes: un veredicto nuevo, un hallazgo y un registro de
// review de otra tarea, commiteados: la tarjeta sigue con su review.
// Guarda: verde tambien antes de esta spec.
func TestCA483_UnCommitSoloDelEstadoDeHoomNoLaDevuelve(t *testing.T) {
	tj := rdGrande(t)
	r1 := tj.completa(t, "20261027T160000_c0mp1a")
	rdCuenta(t, "CA-483 (fixture)", "completa", tj.card(t), r1.ID)
	tj.verde(t)
	if _, err := finding.Register(tj.wt, "main", finding.Draft{Severity: "low", Lens: "risk",
		Description: "nota sin tarea", Author: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	otra := tj.registro("20261027T170000_0tr4a1", tj.mb, tj.head(t), reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes)
	otra.Task, otra.Spec = "otra", ".hoom/specs/otra.md"
	bdReviewDisco(t, tj.wt, otra)
	bdCommitear(t, tj.wt, "solo estado de hoom: un hallazgo y un registro de otra tarea")
	tj.verde(t)
	rdCuenta(t, "CA-483", "commits solo del estado de hoom", tj.card(t), r1.ID)
}

// ---------------------------------------------------------------- CA-485

// CA-485: "con un verde para el HEAD nuevo, el tablero no devuelve la
// tarjeta a Review por ese merge": despues de la review de la tarjeta, main
// avanza con codigo (450 lineas, una ruta de riesgo) o con documentacion, y
// la rama solo lo integra sin conflicto (y se da un verde): la tarjeta sigue
// con su review. Con dos merges seguidos, lo mismo.
func TestCA485_SoloIntegrarLaBaseNoDevuelveLaTarjetaAReview(t *testing.T) {
	casos := []struct {
		nombre string
		traer  func(t *testing.T, tj *rdTarjeta)
	}{
		{"codigo", func(t *testing.T, tj *rdTarjeta) {
			edrMainAvanza(t, tj, "main avanza con codigo", map[string]string{"internal/auth/main.go": rdCodigo(450, "Main")})
			edrIntegrar(t, tj)
		}},
		{"documentacion", func(t *testing.T, tj *rdTarjeta) {
			edrMainAvanza(t, tj, "main avanza con docs", map[string]string{"README.md": "# demo\n\nprosa de main\n"})
			edrIntegrar(t, tj)
		}},
		{"dos-merges", func(t *testing.T, tj *rdTarjeta) {
			edrMainAvanza(t, tj, "main 1", map[string]string{"main1.go": "package app\n\nfunc Main1() {}\n"})
			edrIntegrar(t, tj)
			edrMainAvanza(t, tj, "main 2", map[string]string{"main2.go": "package app\n\nfunc Main2() {}\n"})
			edrIntegrar(t, tj)
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tj := rdGrande(t)
			r1 := tj.completa(t, "20261027T160000_c0mp1a")
			rdCuenta(t, "CA-485 (fixture)", "completa", tj.card(t), r1.ID)
			c.traer(t, tj)
			tj.verde(t)
			rdCuenta(t, "CA-485", "solo integro la base: "+c.nombre, tj.card(t), r1.ID)
		})
	}
}

// ---------------------------------------------------------------- CA-486

// CA-486: "para el tablero, el codigo cambio despues de la review" cuando la
// rama integro la base resolviendo un conflicto, aunque lo que choco sea
// documentacion: la tarjeta vuelve a Review con el motivo y el siguiente paso
// del delta. Seguir ese paso (hoom review --delta, que no se separa y hace
// la review completa) la devuelve a Tu aceptacion.
func TestCA486_ConflictoAlIntegrarLaBaseDevuelveLaTarjetaAReview(t *testing.T) {
	r1Codex(t)
	tj := rdGrande(t)
	edrMainAvanza(t, tj, "main trae el README", map[string]string{"README.md": "# demo\n\nlinea base\n"})
	edrIntegrar(t, tj)
	bdEscribir(t, tj.wt, "README.md", "# demo\n\nlinea de la tarea\n")
	bdCommitear(t, tj.wt, "la tarea cambia el README")
	tj.verde(t)
	r1 := tj.completa(t, "20261027T160000_c0mp1a")
	rdCuenta(t, "CA-486 (fixture)", "completa", tj.card(t), r1.ID)

	edrMainAvanza(t, tj, "main cambia la misma linea", map[string]string{"README.md": "# demo\n\nlinea de main\n"})
	if _, err := edrGitErr(tj.wt, "merge", "-q", "--no-ff", "--no-edit", "main"); err == nil {
		t.Fatal("CA-486: fixture: el merge de main tiene conflicto en README.md")
	}
	bdEscribir(t, tj.wt, "README.md", "# demo\n\nlinea de la tarea\nlinea de main\n")
	bdGit(t, tj.wt, "add", "README.md")
	bdGit(t, tj.wt, "commit", "-q", "--no-edit")
	tj.verde(t)
	edrPideElDelta(t, "CA-486", "conflicto en la documentacion", tj.card(t), r1.ID, r1.Hasta)

	r1Esperar()
	res, out := tj.review(t, "CA-486", true)
	if res.Status != "revisado" || res.Cobertura != reviewcmd.CoberturaCompleta || res.DesdeReview != "" || len(res.Lenses) != 4 {
		t.Fatalf("CA-486: el --delta que pide la tarjeta no se separa: es la review completa de 4 lentes: %+v\n%s", res, out)
	}
	rdCuenta(t, "CA-486", "despues de la review completa", tj.card(t), res.RecordID)
}

// edrGitErr corre git en dir y devuelve su salida y su error, sin cortar el
// test.
func edrGitErr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
