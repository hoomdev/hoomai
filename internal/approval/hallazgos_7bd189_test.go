// Regresion del hallazgo 7bd189 (ronda 2 de proporcion-por-riesgo): un
// registro guardado con la ruta como se dio desde la raiz
// (`./.hoom/specs/precios.md`, lo que escribia `hoom spec approve
// ./.hoom/specs/precios.md` antes) sigue aprobando ese contenido: Status y
// CalibreDe comparan su grafia canonica.
package approval_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
)

// h7bRegistro escribe a mano el registro de la aprobacion del contenido de
// sp con el nombre nombre.
func h7bRegistro(t *testing.T, root, sp, nombre string) {
	t.Helper()
	raw, err := os.ReadFile(sp)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	h := hex.EncodeToString(sum[:])
	rec, err := json.MarshalIndent(approval.Record{Spec: nombre, SHA256: h, ApprovedBy: "hoom test <test@hoom.dev>",
		ApprovedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pprEscribir(t, root, filepath.Join(".hoom", "approvals", "precios_"+h[:8]+".json"), string(rec))
}

func TestHallazgo_7bd189_RegistroConPuntoBarraSigueAprobado(t *testing.T) {
	root, sp, m := pprProyecto(t, "", 100, 2, "-")
	h7bRegistro(t, root, sp, "./.hoom/specs/precios.md")
	for _, ruta := range []string{sp, filepath.Join(root, ".", ".hoom", "specs", "precios.md")} {
		st, rec, err := approval.Status(root, ruta)
		if err != nil || st != approval.StatusApproved || rec == nil || rec.Spec != "./.hoom/specs/precios.md" {
			t.Fatalf("hallazgo 7bd189: el registro ./.hoom/specs/precios.md aprueba %s: %s %+v %v", ruta, st, rec, err)
		}
	}
	if c := approval.CalibreDe(m, root, ".hoom/specs/precios.md"); !c.Aprobado {
		t.Fatalf("hallazgo 7bd189: CalibreDe lo ve aprobado: %+v", c)
	}
}
