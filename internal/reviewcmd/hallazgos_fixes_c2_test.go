// Tests de regresion de la review delta 3 de la tarea fixes-cabina-c2 sobre
// .hoom/specs/base-de-la-review.md:
//
//   - 20261014T164649_173dde (low, resilience; CA-430 y CA-414): la review
//     con --task revisa contra la base del proyecto, y el base_branch que
//     declara el hoom.yaml de la rama se avisa y nunca se obedece; uno que
//     empieza con "-" no rompe la review (lo fija
//     TestCA430_LaBaseDeLaRamaQueEmpiezaConGuionNoLlegaAGit). Pero eso no
//     perdona el resto del hoom.yaml de la rama: si hoom no lo puede cargar
//     por OTRA razon (un perfil que no existe), la review falla sin lanzar
//     ninguna pasada ni escribir registro, haya o no un base_branch con
//     guion.
//
// Reusa los fixtures de base_de_la_review_test.go (bdProyecto, bdTarea,
// bdAtaque, bdYAML) y de review_aislada_helpers_test.go (raPATH, raInstalar,
// raLimpio, raRunBaseConReloj).
package reviewcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// c2PerfilQueNoExiste es el perfil que la rama de la tarea declara en su
// hoom.yaml: ningun perfil de hoom se llama asi.
const c2PerfilQueNoExiste = "perfil-que-no-existe-173dde"

// c2ReviewDeRamaConPerfilRoto arma el proyecto de CA-430 y una tarea cuya
// rama (con el ataque de bdAtaque) commitea un hoom.yaml con base_branch
// declara y profile: c2PerfilQueNoExiste; corre la review con --task contra
// la base del proyecto (main), como la CLI y el Studio, y exige que falle sin
// lanzar ninguna pasada de codex ni escribir un registro de review, con un
// error que nombra el perfil.
func c2ReviewDeRamaConPerfilRoto(t *testing.T, ca, declara string) {
	t.Helper()
	bin := raPATH(t)
	root := bdProyecto(t, "")
	wt := bdTarea(t, root, bdSlug, "main")
	bdAtaque(t, wt, declara, "profile: "+c2PerfilQueNoExiste+"\n", false)
	cx := raInstalar(t, bin, "codex", "")

	raLimpio(t, ca, wt)
	res, err, out := raRunBaseConReloj(t, ca, 60*time.Second, root, "main", Options{Task: bdSlug, Provider: "codex", Lens: "risk"})
	if err == nil {
		t.Errorf("%s: el hoom.yaml de la rama declara un perfil que no existe (%s) y base_branch %s: la review con --task falla; termino %q (codex corrio %d veces)\n%s",
			ca, c2PerfilQueNoExiste, declara, res.Status, cx.veces(), out)
	} else if !strings.Contains(err.Error(), c2PerfilQueNoExiste) {
		t.Errorf("%s: el error de la review nombra el perfil %q, fue: %v", ca, c2PerfilQueNoExiste, err)
	}
	if n := cx.veces(); n != 0 {
		t.Errorf("%s: con el hoom.yaml de la rama que hoom no puede cargar, codex no corre ninguna vez: corrio %d", ca, n)
	}
	for _, dir := range []string{root, wt} {
		recs, avisos := Records(dir)
		if len(recs) != 0 || len(avisos) != 0 {
			t.Errorf("%s: la review que falla no escribe ningun registro de review en %s: %d registros, avisos %v", ca, filepath.Base(dir), len(recs), avisos)
		}
	}
}

// Hallazgo 20261014T164649_173dde. CA-430 (el base_branch de la rama se
// avisa y no se obedece) y CA-414 (la review que no puede seguir falla sin
// pasadas ni registro): la rama commitea un hoom.yaml con base_branch
// "--output=<tmp>/pwned-173dde" (el valor hostil de CA-430) y un profile que
// no existe. Que la review perdone ese base_branch no puede perdonar el
// perfil: hoom review --task falla, el error nombra el perfil, codex no corre
// y no queda registro. Y el valor hostil sigue sin llegar a git: el archivo
// no se crea.
func TestHallazgo_173dde_UnPerfilQueNoExisteEnLaRamaFallaAunqueSuBaseEmpieceConGuion(t *testing.T) {
	pwned := filepath.Join(t.TempDir(), "pwned-173dde")
	c2ReviewDeRamaConPerfilRoto(t, "173dde CA-430", `"--output=`+pwned+`"`)
	if _, err := os.Stat(pwned); err == nil {
		t.Fatalf("173dde CA-430: el base_branch de la rama no llega a git: se creo %s", pwned)
	}
}

// Guarda del hallazgo 20261014T164649_173dde. CA-430 y CA-414: la misma rama
// con el mismo perfil que no existe y base_branch: main (una base valida)
// falla igual: el error nombra el perfil, codex no corre y no queda registro.
func TestHallazgo_173dde_GuardaUnPerfilQueNoExisteEnLaRamaFallaConBaseValida(t *testing.T) {
	c2ReviewDeRamaConPerfilRoto(t, "173dde CA-430", "main")
}
