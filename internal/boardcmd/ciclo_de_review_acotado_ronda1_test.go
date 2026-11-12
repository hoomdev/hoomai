// Tests de regresion de la ronda 1 de ciclo-de-review-acotado sobre el
// tablero: los hallazgos high de la review de codex que el refutador
// corroboro. Cada uno falla con la implementacion de a42af11. Escritos desde
// el spec, las firmas publicas y el texto de los hallazgos, sin leer la
// implementacion.
package boardcmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	crR1Autor = "Ana \x1b]0;PWN\x07\x1b[31m‮roja <ana@hoom.dev>"
	crR1File  = "internal/auth/\x1b[32mG‮.go"
)

// crR1Controles devuelve el primer control (C0 salvo \n y \t, DEL, C1 o un
// formato bidi) de s, o -1.
func crR1Controles(s string) rune {
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f,
			r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			return r
		}
	}
	return -1
}

// hallazgo 20261111T203520_04b283 (CA-627, CA-629): el autor de una subida
// valida y el file de un medium en ruta de riesgo llegan a los motivos de la
// tarjeta. Esperado: Card.Missing y RenderCard (lo que imprime `hoom item
// show`) no traen ESC, BEL ni U+202E literales: salen escapados, como
// cualquier otro valor de la evidencia; el motivo sigue diciendo de que se
// trata (subido a mano por / medium en ruta de riesgo) y deja ver el texto.
func TestHallazgo_04b283_MotivosConControlesSalenEscapados(t *testing.T) {
	ev := crEv("normal", 50, 10, "a.go") // verde vigente, spec aprobado (bdEv)
	ev.Findings = []finding.Item{crHallazgo("f-low", "low", "app.go"), crHallazgo("f-ruta", "medium", crR1File)}
	ev.Subidas = map[string]finding.Subida{"f-low": {FindingID: "f-low", Reason: "rompe", Author: crR1Autor}}
	c := Derive(ev)
	bdCol(t, "04b283", c, ColReview)
	if len(c.Evidence.BlockingFindings) != 2 {
		t.Fatalf("04b283 (fixture): bloquean el low subido y el medium en ruta: %v", c.Evidence.BlockingFindings)
	}
	subida, ruta := false, false
	for _, m := range c.Missing {
		if r := crR1Controles(m); r >= 0 {
			t.Fatalf("hallazgo 04b283 (CA-627): un motivo de la tarjeta lleva el control %U crudo: %q", r, m)
		}
		subida = subida || (strings.Contains(m, "subido a mano por ") && strings.Contains(m, "PWN"))
		// sin fijar la forma del escape (con o sin comillas): despues de
		// "medium en ruta de riesgo: " viene la ruta
		if i := strings.Index(m, "medium en ruta de riesgo: "); i >= 0 {
			resto := m[i+len("medium en ruta de riesgo: "):]
			ruta = ruta || (strings.Contains(resto, "internal/auth/") && strings.Contains(resto, ".go"))
		}
	}
	if !subida || !ruta {
		t.Fatalf("04b283: los motivos siguen nombrando la subida y la ruta, escapadas: %q", c.Missing)
	}
	var b bytes.Buffer
	RenderCard(&b, c)
	if r := crR1Controles(b.String()); r >= 0 {
		t.Fatalf("hallazgo 04b283 (CA-627/CA-629): RenderCard imprime el control %U crudo:\n%q", r, b.String())
	}
}

// hallazgo 20261111T203520_3e88e6 (CA-627): el item de la tarea en el
// checkout principal declara riesgo alto pero es invalido por otro campo; la
// copia del worktree no tiene la clave. Un item que existe en el principal y
// no se puede leer no cae al del arbol ni a normal: RiesgoDe da error y el
// gate findings_open corrido desde el worktree no da verde (un medium de la
// tarea figura en una review completa).
func TestHallazgo_3e88e6_ItemInvalidoEnElPrincipalNoCaeANormal(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdCommitear(t, root, "item sin riesgo")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdEscribir(t, wt, "precios.go", "package app\n")
	bdCommitear(t, wt, "spec y codigo")
	// la persona sube el riesgo en el principal, pero el item queda invalido
	bdEscribir(t, root, ".hoom/items/"+bdSlug+".yaml",
		"titulo: Precios\ntipo: feature\nprioridad: urgentisima\n"+
			"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\nriesgo: alto\n")
	bdCommitear(t, root, "riesgo alto (item invalido)")

	m, err := finding.Register(wt, "main", finding.Draft{Severity: "medium", Lens: "risk", File: "app.go",
		Description: "medium de la completa", Author: "reviewer@test", Task: bdSlug})
	if err != nil {
		t.Fatal(err)
	}
	rec := reviewcmd.Record{ID: "20261111T100000_c0mp1a", CreatedAt: bdT0, Task: bdSlug, Spec: bdSpec,
		Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes, Provider: "codex",
		Writer: "claude", Cross: reviewcmd.CrossYes, WritersDeclared: []string{}, Findings: []string{m.ID},
		Usage: []reviewcmd.LensUsage{}, Cobertura: reviewcmd.CoberturaCompleta, Desde: bdDesde, Hasta: bdHasta}
	bdReviewDisco(t, wt, rec)
	bdCommitear(t, wt, "hallazgo y registro")

	for _, dir := range []string{wt, root} {
		if r, err := item.RiesgoDe(dir, bdSlug); err == nil {
			t.Fatalf("hallazgo 3e88e6: RiesgoDe(%s) de un item invalido en el principal es un error, no %q", dir, r)
		}
	}
	g := finding.Gate(wt, "main", "high", bdSpec)
	if g.Status == verdict.StatusPass {
		t.Fatalf("hallazgo 3e88e6 (CA-627): con el item del principal ilegible el gate desde el worktree no da verde (ERROR, o FAIL por %s): %s %q\n%s",
			m.ID, g.Status, g.Notes, g.OutputTail)
	}
}
