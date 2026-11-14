//go:build linux

// Ronda 1 del test-writer de contencion-del-writer: el guardado del piloto
// con Windows sin job simulado en Linux con ReemplazarAdopcion (ids c5790e,
// 5380e1, 15f87d, c1983c, 185514; aclaracion 22). Sin bwrap falla
// (aclaracion 1).
package pilotocmd_test

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/itemcmd"
)

// CA-676 y CA-677 (id c5790e; aclaracion 22): con la seccion exigiendo
// parcial (que este Linux alcanza al empezar) y sin job, el primer git
// encerrado del guardado muere enseguida y SaveTarea falla con
// ErrExigirDelGuardado del nivel real (ninguno), sin commitear: HEAD queda
// donde estaba y lo sin guardar sigue sin guardar. Con exigir no, guarda.
func TestR1_c5790e_CA676_CA677_SinJobElGuardadoSeNiegaConElNivelReal(t *testing.T) {
	const ca = "CA-676"
	cdwlEncierro(t, ca)
	restaurar := encierro.ReemplazarAdopcion("windows", func(int) bool { return false })
	defer restaurar()
	apPATH(t)
	cdwHome(t)
	root, wt := cdwTarjeta(t, "encierro:\n  exigir: parcial\n", "sinjob")
	head := apHead(t, wt)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "sinjob")
	want := itemcmd.ErrExigirDelGuardado(encierro.NivelNinguno, "parcial")
	if err == nil || !strings.HasPrefix(err.Error(), want) || len(res.Commits) != 0 || apHead(t, wt) != head {
		t.Fatalf("%s: sin job el guardado se niega con %q sin commitear: %+v %v (HEAD %s -> %s)", ca, want, res, err, head, apHead(t, wt))
	}
	if !strings.Contains(apGit(t, wt, "status", "--porcelain"), "app.go") {
		t.Fatalf("%s: lo que habia sin guardar sigue sin guardar", ca)
	}

	root, _ = cdwTarjeta(t, cdwlSeccion, "sinjobno")
	if res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "sinjobno"); err != nil || len(res.Commits) != 1 {
		t.Fatalf("CA-677: sin job y con exigir no, el guardado guarda: %+v %v", res, err)
	}
}
