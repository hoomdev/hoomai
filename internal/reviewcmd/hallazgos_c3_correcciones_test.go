// Tests de regresion de la review de las correcciones de C3
// (.hoom/specs/acciones-desde-la-tarjeta.md) sobre `hoom review`, escritos
// desde el contrato y los hallazgos, sin leer la implementacion:
//
//   - 20261015T163244_0aade6 (medium), CA-351: un item de la tarea que es un
//     symlink colgante EXISTE como entrada aunque leerlo de "no existe": es un
//     item ilegible, no "sin item". La review falla antes de la primera
//     pasada, sin correr el provider y sin registros (como 9e2b00/5d64ca).
//   - 20261015T164614_9a93be (low), guarda: el catalogo de lentes es una sola
//     fuente: ninguna lente de Lentes es vacia ni se repite, y
//     CantidadDeLentes es len(Lentes).
//
// Reusa m2Revision, m2Corrio y m2SinRegistros (hallazgos_c3_mitad2_test.go)
// y cbMetasEn y cbStarted (sobre_review_test.go).
package reviewcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Hallazgo 20261015T163244_0aade6 (medium), CA-351: con el item de la tarea
// como symlink colgante (la entrada existe, lo que apunta no), revisar con
// codex falla con error antes de la primera pasada: el provider no corre,
// ningun run se registra, Started no se llama y no queda registro de review
// ni de sobre.
func TestHallazgo_0aade6_CA351_ItemSymlinkColganteFrenaLaReview(t *testing.T) {
	root, wt, marca := m2Revision(t)
	items := filepath.Join(root, ".hoom", "items")
	if err := os.MkdirAll(items, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "no-existe.yaml"), filepath.Join(items, "precios.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(items, "precios.yaml")); err != nil {
		t.Fatalf("CA-351: fixture: la entrada del item existe: %v", err)
	}
	metas := cbMetasEn(root)
	s := &cbStarted{root: root, id: "20261015T163244_ccrev1"}

	var out strings.Builder
	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "codex",
		EnvelopeID: s.id, Started: s.fn}, &out)
	if m2Corrio(marca) {
		t.Fatalf("hallazgo 0aade6 (CA-351): con el item como symlink colgante el provider no corre: %+v %v\n%s", res, err, out.String())
	}
	if err == nil {
		t.Fatalf("hallazgo 0aade6 (CA-351): con el item como symlink colgante la review falla con error antes de la primera pasada (es un item ilegible, no la falta de item); devolvio %+v\n%s", res, out.String())
	}
	if len(res.Passes) != 0 || cbMetasEn(root) != metas {
		t.Fatalf("hallazgo 0aade6 (CA-351): sin pasadas ni runs: %+v (runs %d -> %d)", res.Passes, metas, cbMetasEn(root))
	}
	if calls, _, _, _ := s.snapshot(); calls != 0 {
		t.Fatalf("hallazgo 0aade6 (CA-351): Started no se llama (llamadas: %d)", calls)
	}
	m2SinRegistros(t, "0aade6 (CA-351)", root, wt)
}

// Hallazgo 20261015T164614_9a93be (low), guarda: ninguna lente de Lentes es
// vacia ni se repite, y CantidadDeLentes es len(Lentes).
func TestHallazgo_9a93be_GuardaLentesSinVaciasNiRepetidas(t *testing.T) {
	if CantidadDeLentes != len(Lentes) {
		t.Fatalf("hallazgo 9a93be: CantidadDeLentes (%d) es len(Lentes) (%d): %q", CantidadDeLentes, len(Lentes), Lentes)
	}
	vistas := map[string]int{}
	for i, l := range Lentes {
		if strings.TrimSpace(l) == "" {
			t.Fatalf("hallazgo 9a93be: ninguna lente es vacia: la %d es %q en %q", i, l, Lentes)
		}
		if j, ok := vistas[l]; ok {
			t.Fatalf("hallazgo 9a93be: ninguna lente se repite: %q esta en %d y en %d de %q", l, j, i, Lentes)
		}
		vistas[l] = i
	}
}
