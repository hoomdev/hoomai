// Regresion del hallazgo 6dcf5b (ronda 3 de proporcion-por-riesgo): el
// registro de una aprobacion es de SU spec. Dos specs con el mismo nombre de
// archivo (o el mismo slug: a_b.md y a-b.md) y el mismo contenido, o un spec
// movido de ruta, no comparten registro: si Approve o Aprobar dicen exito o
// already, Status de esa misma ruta dice aprobado, y los registros de los
// otros specs no se tocan.
package approval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// h6dAprobado: Aprobar y despues Approve de ruta dicen exito o already, y
// tras cada uno Status de ruta es aprobado por un registro llamado spec.
func h6dAprobado(t *testing.T, hallazgo string, m *manifest.Manifest, root, ruta, spec string) {
	t.Helper()
	for i, aprobar := range []func() (approval.Record, bool, error){
		func() (approval.Record, bool, error) { return approval.Aprobar(m, root, ruta) },
		func() (approval.Record, bool, error) { return approval.Approve(root, ruta) },
	} {
		rec, already, err := aprobar()
		if err != nil {
			t.Fatalf("hallazgo %s: %s se aprueba (llamada %d): %v", hallazgo, ruta, i, err)
		}
		st, vigente, err := approval.Status(root, ruta)
		if err != nil || st != approval.StatusApproved || vigente == nil || vigente.SHA256 != rec.SHA256 {
			t.Fatalf("hallazgo %s: Aprobar/Approve de %s dijo exito (already=%v): Status de la misma ruta es aprobado, fue %s %+v %v",
				hallazgo, ruta, already, st, vigente, err)
		}
		if rec.Spec != spec || i == 1 && !already {
			t.Fatalf("hallazgo %s: el registro de %s se llama %s y la segunda vez es already: %+v already=%v", hallazgo, ruta, spec, rec, already)
		}
	}
}

// h6dRegistros son los registros de root, por nombre y contenido.
func h6dRegistros(t *testing.T, root string) map[string]string {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(root, ".hoom", "approvals"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range es {
		raw, err := os.ReadFile(filepath.Join(root, ".hoom", "approvals", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(raw)
	}
	return out
}

// h6dIntactos: los registros de antes siguen ahi, byte a byte.
func h6dIntactos(t *testing.T, hallazgo string, antes, ahora map[string]string) {
	t.Helper()
	for n, raw := range antes {
		if ahora[n] != raw {
			t.Fatalf("hallazgo %s: el registro %s de otro spec no se toca", hallazgo, n)
		}
	}
}

func TestHallazgo_6dcf5b_MismoNombreYContenidoNoCompartenRegistro(t *testing.T) {
	const h = "6dcf5b"
	root, _, m := pprProyecto(t, "", 100, 2, "-")
	texto := pprTexto(t, 120, 2)
	for _, par := range [][2]string{
		{"docs/a/precios.md", "docs/b/precios.md"}, // el mismo nombre de archivo
		{"docs/a_b.md", "docs/a-b.md"},             // el mismo slug
	} {
		a := pprEscribir(t, root, par[0], texto)
		b := pprEscribir(t, root, par[1], texto)
		h6dAprobado(t, h, m, root, a, par[0])
		antes := h6dRegistros(t, root)
		h6dAprobado(t, h, m, root, b, par[1])
		h6dIntactos(t, h, antes, h6dRegistros(t, root))
		// el primero sigue aprobado
		if st, _, err := approval.Status(root, a); err != nil || st != approval.StatusApproved {
			t.Fatalf("hallazgo %s: %s sigue aprobado: %s %v", h, par[0], st, err)
		}
	}
}

func TestHallazgo_6dcf5b_SpecMovidoSeApruebaEnSuRutaNueva(t *testing.T) {
	const h = "6dcf5b"
	root, _, m := pprProyecto(t, "", 100, 2, "-")
	borrador := pprEscribir(t, root, "docs/borradores/cobro.md", pprTexto(t, 120, 2))
	h6dAprobado(t, h, m, root, borrador, "docs/borradores/cobro.md")
	antes := h6dRegistros(t, root)
	// git mv del borrador a .hoom/specs/: el mismo contenido en otra ruta
	movido := filepath.Join(root, ".hoom", "specs", "cobro.md")
	if err := os.Rename(borrador, movido); err != nil {
		t.Fatal(err)
	}
	if st, _, err := approval.Status(root, movido); err != nil || st == approval.StatusApproved {
		t.Fatalf("fixture: el spec movido no hereda la aprobacion del borrador: %s %v", st, err)
	}
	h6dAprobado(t, h, m, root, movido, ".hoom/specs/cobro.md")
	h6dIntactos(t, h, antes, h6dRegistros(t, root))
}
