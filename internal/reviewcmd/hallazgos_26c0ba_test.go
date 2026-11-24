package reviewcmd

// Regresion de los hallazgos 85c48e y d4c535 (review 26c0ba): la lista de
// archivos de gitx.Snapshot es --diff-filter=ACMR y no trae lo que la rama
// borro en un commit; «sin cambios» y «solo documentacion» se deciden sobre
// esa lista mas los borrados (gitx.ArchivosConBorrados).

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// h26BorradoEnLaRama arma raRepo con rel commiteado en main (cuerpo body),
// la tarea retySlug cuya rama solo borra rel en un commit, su item (tipo,
// r) y el codex falso.
func h26BorradoEnLaRama(t *testing.T, rel, body, tipo, r string) (root, wt string, cx *raCLI) {
	t.Helper()
	bin := raPATH(t)
	root = raRepo(t, "")
	write(t, root, rel, body)
	git(t, root, "add", "--", rel)
	git(t, root, "commit", "-q", "-m", "agrega "+rel)
	wt = retyTarea(t, root, retySlug, nil)
	git(t, wt, "rm", "-q", "--", rel)
	git(t, wt, "commit", "-q", "-m", "borra "+rel)
	retyItem(t, root, retySlug, tipo, r)
	cx = raInstalar(t, bin, "codex", "")
	// la premisa del hallazgo: el snapshot no trae el borrado
	if snap := gitx.Snapshot(wt, "main"); len(snap.ChangedFiles) != 0 || snap.Deletions == 0 {
		t.Fatalf("premisa: el snapshot de una rama que solo borra %s no lo lista: %+v", rel, snap)
	}
	return root, wt, cx
}

// 85c48e: una rama que solo borra un archivo de una ruta de riesgo es alto
// (regla 4) y corre las 4 lentes con MotivoRiesgoAlto, no «no hay cambios».
func TestHallazgo85c48e_BorrarUnaRutaDeRiesgoCorreLasCuatro(t *testing.T) {
	const ca = "hallazgo 85c48e"
	root, wt, cx := h26BorradoEnLaRama(t, "internal/auth/token.go", rdCodigo(30, "Token"), "feature", "")
	res, out := retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "borrar internal/auth/token.go", res, out, cx, 0, Lentes, MotivoRiesgoAlto)
	retyMotivos(t, ca, wt, res, out, "alto", "normal", "el cambio toca internal/auth/token.go: riesgo alto")
	if res.RecordID == "" {
		t.Fatalf("%s: la review revisada deja su registro: %+v\n%s", ca, res, out)
	}
}

// 85c48e: una rama que solo borra README.md sigue siendo solo documentacion:
// 0 lentes con MotivoSoloDocs, ni con riesgo alto declarado.
func TestHallazgo85c48e_BorrarSoloElREADMENoCorreLentes(t *testing.T) {
	const ca = "hallazgo 85c48e"
	root, _, cx := h26BorradoEnLaRama(t, "README.md", "# demo\n\nuna linea\n", "feature", "alto")
	res, err, out := rdCorrer(t, ca, root, Options{Provider: "codex", Task: retySlug})
	if err != nil || len(res.Lenses) != 0 || len(res.Passes) != 0 || cx.veces() != 0 || res.Reason != MotivoSoloDocs {
		t.Fatalf("%s: borrar solo README.md no corre lentes (%q): %+v %v\n%s", ca, MotivoSoloDocs, res, err, out)
	}
}
