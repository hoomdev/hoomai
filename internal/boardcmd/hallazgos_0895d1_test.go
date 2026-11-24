package boardcmd

// Regresion del hallazgo a4379f (con 48f385 y ff65c0; review delta 0895d1):
// si gitx.LineasDelCambio falla al leer los borrados del veredicto de la
// huella actual, el tablero no toma el cambio como vacio: sus archivos son
// desconocidos (aclaracion 25), ni vacio ni solo documentacion.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// h08Tarjeta es el fixture de h26Tarjeta para la rama que solo borra
// internal/auth/token.go, con el veredicto verde (alto por la ruta)
// commiteado en el worktree; devuelve root y el veredicto.
func h08Tarjeta(t *testing.T) (string, *verdict.Verdict) {
	t.Helper()
	rel := "internal/auth/token.go"
	var b strings.Builder
	b.WriteString("package auth\n\n")
	for i := 0; i < 28; i++ {
		b.WriteString("var T = 1\n")
	}
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdEscribir(t, root, rel, b.String())
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdAprobar(t, root, bdSlug)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdCommitear(t, root, "base de la tarjeta")
	wt := bdWorktree(t, root, bdSlug)
	bdGit(t, wt, "rm", "-q", "--", rel)
	bdGit(t, wt, "commit", "-q", "-m", "borra "+rel)
	ev := riesgo.Evaluacion{Declarado: riesgo.Normal, Efectivo: riesgo.Alto,
		Motivo: "el cambio toca internal/auth/token.go: riesgo alto"}
	v := &verdict.Verdict{Project: "demo", CreatedAt: bdT0.Add(time.Minute), Spec: bdSpec, Git: gitx.Snapshot(wt, "main"),
		Gates: bdGatesVerdes(), Riesgo: &ev, Tamano: &riesgo.Tamano{Producto: 30}}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	bdCommitear(t, wt, "veredicto")
	return root, v
}

// h08Exige exige que la tarjeta lea el veredicto vigente sin poder medir sus
// borrados, y que con alto exija la review y quede en Review.
func h08Exige(t *testing.T, ca, caso, root string, v *verdict.Verdict) {
	t.Helper()
	if _, err := gitx.LineasDelCambio(filepath.Join(root, bdWtDir), "main"); err == nil {
		t.Fatalf("%s: %s: premisa: LineasDelCambio falla", ca, caso)
	}
	e := Gather(root, "main", "high", bdItem(bdSlug), time.Now().UTC())
	if e.Verdict == nil || e.Verdict.ID != v.ID || e.Fingerprint != v.Git.ChangeFingerprint {
		t.Fatalf("%s: %s: premisa: el veredicto sigue vigente: %+v %q", ca, caso, e.Verdict, e.Fingerprint)
	}
	if !e.ArchivosDesconocidos || len(e.ArchivosDelCambio) != 0 {
		t.Fatalf("%s: %s: sin poder medir, los archivos son desconocidos: %v %q", ca, caso, e.ArchivosDesconocidos, e.ArchivosDelCambio)
	}
	c := Derive(e)
	if c.Evidence.Riesgo != riesgo.Alto || !c.Evidence.ReviewRequired {
		t.Fatalf("%s: %s: riesgo alto con archivos desconocidos exige review: %+v", ca, caso, c.Evidence)
	}
	bdCol(t, ca+" ("+caso+")", c, ColReview)
	falta := false
	for _, m := range c.Missing {
		falta = falta || m == "la review exige las 4 lentes (riesgo alto) y no hay registro de review"
	}
	if !falta {
		t.Fatalf("%s: %s: falta la review: %q", ca, caso, c.Missing)
	}
}

// a4379f: la base del veredicto (main) se renombra despues del veredicto; la
// huella no depende de la base y sigue vigente.
func TestHallazgoA4379f_BaseQueYaNoResuelveExigeLaReview(t *testing.T) {
	const ca = "hallazgo a4379f"
	root, v := h08Tarjeta(t)
	bdGit(t, root, "branch", "-m", "main", "trunk")
	h08Exige(t, ca, "main renombrada", root, v)
}

// a4379f: un git que solo falla `diff --numstat` (el de LineasDelCambio).
func TestHallazgoA4379f_DiffNumstatQueFallaExigeLaReview(t *testing.T) {
	const ca = "hallazgo a4379f"
	root, v := h08Tarjeta(t)
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do\n  if [ \"$a\" = \"--numstat\" ]; then echo 'fatal: simulado' >&2; exit 1; fi\ndone\nexec " + real + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h08Exige(t, ca, "git sin --numstat", root, v)
}

// a4379f: archivos desconocidos no son solo documentacion, pero tampoco
// suben el umbral: con normal o bajo, la review se exige solo con mas de 400
// lineas de producto; y cuentan solo para el veredicto de la huella actual.
func TestHallazgoA4379f_DesconocidosConNormalYBajo(t *testing.T) {
	const ca = "hallazgo a4379f"
	v := func(p int) *verdict.Verdict {
		return &verdict.Verdict{Git: gitx.Info{ChangeFingerprint: "actual", ChangedFiles: []string{"README.md"}},
			Riesgo: &riesgo.Evaluacion{Declarado: riesgo.Normal, Efectivo: riesgo.Normal, Motivo: "declarado normal"},
			Tamano: &riesgo.Tamano{Producto: p}}
	}
	ev := Evidence{Fingerprint: "actual", ArchivosDesconocidos: true}
	for _, k := range []struct {
		r     string
		p     int
		exige bool
	}{{riesgo.Normal, 400, false}, {riesgo.Normal, 401, true}, {riesgo.Bajo, 400, false}, {riesgo.Bajo, 401, true}, {riesgo.Alto, 3, true}} {
		if _, req := reviewRequired(ev, v(k.p), k.r); req != k.exige {
			t.Fatalf("%s: %s con %d de producto y archivos desconocidos: exige %v, fue %v", ca, k.r, k.p, k.exige, req)
		}
	}
	otra := Evidence{Fingerprint: "otra", ArchivosDesconocidos: true}
	if _, req := reviewRequired(otra, v(3), riesgo.Alto); req {
		t.Fatalf("%s: un veredicto de otra huella lee sus changed_files (solo documentacion)", ca)
	}
}
