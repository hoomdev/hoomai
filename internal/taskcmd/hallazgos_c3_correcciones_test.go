// Test de regresion del hallazgo 20261015T163642_0c71ae (medium) de la review
// de las correcciones de C3 (.hoom/specs/acciones-desde-la-tarjeta.md,
// CA-346), escrito desde el contrato y el hallazgo, sin leer la
// implementacion: un rename en el indice (git mv) tiene dos rutas sin
// guardar, el origen y el destino. Discardable lista las dos, y Discard con
// esas dos como expect (lo que manda el Studio) descarta el rename sin
// ErrChanged.
//
// Reusa h3Tarea, h3Rel, h3Descartable, h3Evidencia (hallazgos_c3_test.go) y
// cbGit, cbLeer, cbMismos (discard_test.go).
package taskcmd

import (
	"strings"
	"testing"
)

// Hallazgo 20261015T163642_0c71ae (medium), CA-346: despues de
// `git mv app.go movido.go`, Discardable lista app.go y movido.go, y Discard
// con esas dos rutas como expect descarta el rename: app.go vuelve como esta
// en HEAD, movido.go desaparece y no queda nada en el indice fuera de .hoom/.
func TestHallazgo_0c71ae_CA346_DescartarUnRenameConLoQueSeVio(t *testing.T) {
	root, wt := h3Tarea(t, nil, nil)
	cbGit(t, wt, "mv", "app.go", "movido.go")
	vio := []string{h3Rel("app.go"), h3Rel("movido.go")}
	// Discardable lista las dos (Errorf: igual se prueba lo que manda el
	// Studio, que son las dos rutas de la tarjeta).
	if got, err := Discardable(root, cbSlug); err != nil || !cbMismos(got, vio) {
		t.Errorf("hallazgo 0c71ae (CA-346): Discardable lista las dos rutas del rename, el origen y el destino:\n got  %q (%v)\n want %q", got, err, vio)
	}

	res, err := Discard(root, cbSlug, vio)
	if err != nil {
		t.Fatalf("hallazgo 0c71ae (CA-346): Discard con las dos rutas del rename que se vieron descarta, sin %q: %v", ErrChanged, err)
	}
	if got, ok := cbLeer(t, wt, "app.go"); !ok || got != "package app\n" {
		t.Fatalf("hallazgo 0c71ae (CA-346): app.go vuelve a como esta en HEAD: %q (existe: %v)", got, ok)
	}
	if _, ok := cbLeer(t, wt, "movido.go"); ok {
		t.Fatal("hallazgo 0c71ae (CA-346): movido.go no esta en HEAD y desaparece")
	}
	for _, l := range strings.Split(cbGit(t, wt, "status", "--porcelain", "--untracked-files=all"), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(strings.TrimSpace(l[2:]), ".hoom/") {
			t.Fatalf("hallazgo 0c71ae (CA-346): despues de descartar el rename, lo unico sin guardar es la evidencia: %q", l)
		}
	}
	h3Evidencia(t, "0c71ae", wt)
	if !cbMismos(res.Removed, []string{h3Rel("movido.go")}) {
		t.Fatalf("hallazgo 0c71ae (CA-346): movido.go (no esta en HEAD) es lo que se borro: removed %q", res.Removed)
	}
}

// Hallazgo 20261015T163642_0c71ae, CA-346 (el resultado): DiscardResult es lo
// que hizo Descartar ({"restored", "removed"}, relativas a la raiz). Con un
// rename, app.go, que HEAD tiene y que la persona vio entre lo descartable,
// vuelve a HEAD: el resultado lo nombra en restored, y movido.go en removed.
func TestHallazgo_0c71ae_CA346_ElResultadoNombraLasDosRutasDelRename(t *testing.T) {
	root, wt := h3Tarea(t, nil, nil)
	cbGit(t, wt, "mv", "app.go", "movido.go")
	vio := []string{h3Rel("app.go"), h3Rel("movido.go")}
	h3Descartable(t, "0c71ae", root, vio)
	res, err := Discard(root, cbSlug, vio)
	if err != nil {
		t.Fatalf("hallazgo 0c71ae (CA-346): %v", err)
	}
	if !cbMismos(res.Restored, []string{h3Rel("app.go")}) || !cbMismos(res.Removed, []string{h3Rel("movido.go")}) {
		t.Fatalf("hallazgo 0c71ae (CA-346): el resultado dice lo que hizo: app.go (en HEAD) en restored y movido.go (no esta en HEAD) en removed: restored %q removed %q", res.Restored, res.Removed)
	}
}
