package boardcmd

// Regresion de los hallazgos 85c48e y d4c535 (review 26c0ba), lado tablero:
// el veredicto guarda changed_files de gitx.Snapshot (--diff-filter=ACMR),
// sin lo que la rama borro en un commit; el tablero suma esos borrados
// (Evidence.ArchivosDelCambio) cuando el veredicto certifica la huella
// actual.

import (
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// h26Tarjeta arma un proyecto con rel (cuerpo body), el spec aprobado de
// la tarjeta y su item (riesgo r, "" = sin la clave) commiteados en main,
// y la tarea bdSlug cuya rama solo borra rel en un commit. Escribe en el
// worktree el veredicto verde que dejaria verify (con el riesgo ev y el
// tamano de producto p) y devuelve Gather + Derive.
func h26Tarjeta(t *testing.T, rel, body, r string, ev riesgo.Evaluacion, p int) (Evidence, Card) {
	t.Helper()
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdEscribir(t, root, rel, body)
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdAprobar(t, root, bdSlug)
	extra := ""
	if r != "" {
		extra = "riesgo: " + r + "\n"
	}
	bdItemArchivo(t, root, bdSlug, "Precios", extra)
	bdCommitear(t, root, "base de la tarjeta")
	wt := bdWorktree(t, root, bdSlug)
	bdGit(t, wt, "rm", "-q", "--", rel)
	bdGit(t, wt, "commit", "-q", "-m", "borra "+rel)

	snap := gitx.Snapshot(wt, "main")
	if len(snap.ChangedFiles) != 0 || snap.Deletions == 0 {
		t.Fatalf("premisa: el snapshot de una rama que solo borra %s no lo lista: %+v", rel, snap)
	}
	v := &verdict.Verdict{Project: "demo", CreatedAt: bdT0.Add(time.Minute), Spec: bdSpec, Git: snap,
		Gates: bdGatesVerdes(), Riesgo: &ev, Tamano: &riesgo.Tamano{Producto: p}}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	// commiteado, como lo deja el writer: no mueve la huella
	bdCommitear(t, wt, "veredicto")
	it := bdItem(bdSlug)
	it.Riesgo = r
	e := Gather(root, "main", "high", it, time.Now().UTC())
	if e.Verdict == nil || e.Verdict.ID != v.ID || e.Fingerprint != v.Git.ChangeFingerprint {
		t.Fatalf("premisa: la tarjeta lee el veredicto del worktree con la huella actual: %+v", e.Verdict)
	}
	return e, Derive(e)
}

// d4c535: borrar internal/auth/token.go (ruta de riesgo) con un verde da
// riesgo alto, exige la review y deja la tarjeta en Review.
func TestHallazgoD4c535_BorrarUnaRutaDeRiesgoExigeLaReview(t *testing.T) {
	const ca = "hallazgo d4c535"
	alto := riesgo.Evaluacion{Declarado: riesgo.Normal, Efectivo: riesgo.Alto,
		Motivo: "el cambio toca internal/auth/token.go: riesgo alto"}
	var b strings.Builder
	b.WriteString("package auth\n\n")
	for i := 0; i < 28; i++ {
		b.WriteString("var T = 1\n")
	}
	ev, c := h26Tarjeta(t, "internal/auth/token.go", b.String(), "", alto, 30)
	if !containsStr(ev.ArchivosDelCambio, "internal/auth/token.go") {
		t.Fatalf("%s: los archivos del cambio traen el borrado: %q", ca, ev.ArchivosDelCambio)
	}
	if c.Evidence.Riesgo != riesgo.Alto || !c.Evidence.ReviewRequired {
		t.Fatalf("%s: riesgo alto y review exigida: %+v", ca, c.Evidence)
	}
	bdCol(t, ca, c, ColReview)
	if len(c.Missing) != 1 || c.Missing[0] != "la review exige las 4 lentes (riesgo alto) y no hay registro de review" {
		t.Fatalf("%s: lo unico que falta es la review: %q", ca, c.Missing)
	}
}

// d4c535: borrar solo README.md sigue siendo solo documentacion: con riesgo
// alto declarado, no exige review.
func TestHallazgoD4c535_BorrarSoloElREADMENoExigeReview(t *testing.T) {
	const ca = "hallazgo d4c535"
	alto := riesgo.Evaluacion{Declarado: riesgo.Alto, Efectivo: riesgo.Alto, Motivo: "declarado alto"}
	ev, c := h26Tarjeta(t, "README.md", "# demo\n\nuna linea\n", "alto", alto, 3)
	if !containsStr(ev.ArchivosDelCambio, "README.md") {
		t.Fatalf("%s: los archivos del cambio traen el borrado: %q", ca, ev.ArchivosDelCambio)
	}
	if c.Evidence.Riesgo != riesgo.Alto || c.Evidence.ReviewRequired {
		t.Fatalf("%s: riesgo alto y solo documentacion: no exige review: %+v", ca, c.Evidence)
	}
	bdCol(t, ca, c, ColTuAceptacion)
}

// d4c535: los borrados del worktree cuentan solo para el veredicto de la
// huella actual; uno de otra huella lee sus changed_files.
func TestHallazgoD4c535_LosBorradosSonDeLaHuellaActual(t *testing.T) {
	const ca = "hallazgo d4c535"
	v := &verdict.Verdict{Git: gitx.Info{ChangeFingerprint: "otra", Insertions: 0, Deletions: 30},
		Riesgo: &riesgo.Evaluacion{Declarado: riesgo.Normal, Efectivo: riesgo.Alto, Motivo: "x"},
		Tamano: &riesgo.Tamano{Producto: 30}}
	ev := Evidence{Fingerprint: "actual", ArchivosDelCambio: []string{"internal/auth/token.go"}}
	if _, req := reviewRequired(ev, v, riesgo.Alto); req {
		t.Fatalf("%s: un veredicto de otra huella no toma los borrados del worktree", ca)
	}
	ev.Fingerprint = "otra"
	if _, req := reviewRequired(ev, v, riesgo.Alto); !req {
		t.Fatalf("%s: el veredicto de la huella actual suma los borrados y exige review", ca)
	}
}
