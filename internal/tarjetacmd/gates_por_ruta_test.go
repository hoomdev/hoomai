// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre `hoom
// integrar` y la tarjeta en disco (CA-814, CA-815; aclaracion 19): con un
// ultimo veredicto completo y verde con no_aplica, la tarjeta no esta en Tu
// aceptacion, su texto llano es el de sin-cierre, e `integrar <slug> --si`
// sale con 1 y `en <columna> no hay nada que integrar: <plain>`, sin
// efectos; tras un verde sin no_aplica (el de un --cierre) commiteado, la
// tarjeta vuelve a Tu aceptacion e integrar la pasa a Hecho.
//
// Usa los fixtures de tarjetacmd_test.go (vdtProyecto, vdtItem, vdtTarea,
// vdtCorrer, vdtFoto...). Escritos desde el spec, las aclaraciones de la
// ronda 0 y el go doc, sin leer la implementacion.
package tarjetacmd_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// gprPlain es el texto llano del spec para sin-cierre.
const gprPlain = "falta la verificacion de cierre del espacio de trabajo"

// gprVeredicto escribe en dir el veredicto verde de la tarjeta con los gates
// de vdtGatesVerdes y static y mutation: en no_aplica (un verify de trabajo)
// o en pass (un --cierre).
func gprVeredicto(t *testing.T, dir, slug string, at time.Time, noAplica bool) *verdict.Verdict {
	t.Helper()
	gs := vdtGatesVerdes()
	for _, n := range []string{"static", "mutation"} {
		g := verdict.GateResult{Name: n, Required: true, Status: verdict.StatusPass, Scope: "diff", Command: "true"}
		if noAplica {
			g = verdict.GateResult{Name: n, Required: true, Status: verdict.StatusNoAplica, Scope: "none",
				Notes: fmt.Sprintf(gates.MotivoNoAplica, "app/**")}
		}
		gs = append(gs, g)
	}
	v := &verdict.Verdict{Project: "demo", CreatedAt: at, Spec: vdtSpecRel(slug), Git: gitx.Snapshot(dir, vdtBase), Gates: gs}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

// CA-814 y CA-815: la tarjeta con un verde de trabajo con no_aplica dice
// sin-cierre, integrar se niega sin efectos, y tras el verde de cierre
// integra.
func TestCA814_IntegrarSeNiegaSinCierreYCierraTrasElCierre(t *testing.T) {
	const ca = "CA-814"
	const slug = "precios"
	root := vdtProyecto(t, "")
	vdtItem(t, root, slug, "")
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto("- CA-970: citado por un test."))
	vdtEscribir(t, wt, vdtTestDe(slug), "package app\n\n// CA-970\n")
	vdtEscribir(t, wt, "lista.go", "package app\n\nfunc Lista() int { return 1 }\n")
	vdtAprobar(t, wt, slug)
	t0 := time.Now().UTC().Add(-2 * time.Minute)
	v := gprVeredicto(t, wt, slug, t0, true)
	if v.Verdict != "green" || v.IsPartial() {
		t.Fatalf("%s: fixture: el verde de trabajo con no_aplica es verde y completo: %s", ca, v.Verdict)
	}
	vdtCommit(t, wt, "verde de trabajo")

	// CA-815: el tablero en disco muestra el texto de sin-cierre
	c := vdtCarta(t, root, slug)
	if c.Column == boardcmd.ColTuAceptacion || c.Column == boardcmd.ColHecho {
		t.Fatalf("%s: con sin-cierre la tarjeta no esta en Tu aceptacion: %s (%s)", ca, c.Column, c.Plain)
	}
	if c.Plain != gprPlain {
		t.Fatalf("CA-815: el tablero dice %q, fue %q (missing=%q)", gprPlain, c.Plain, c.Missing)
	}

	antes := vdtFoto(t, root)
	s := vdtCorrer(t, ca, vdtOpts{dir: root}, "integrar", slug, "--si")
	vdtExit(t, ca, s, 1)
	vdtDiceErr(t, ca, s, "en "+c.ColumnName+" no hay nada que integrar: "+c.Plain)
	vdtSinEfectos(t, ca, "integrar sin cierre", antes, vdtFoto(t, root))
	if it, err := item.Load(root, slug); err != nil || it.HechoEn != nil {
		t.Fatalf("%s: integrar no cierra: el item sigue sin hecho_en: %+v %v", ca, it, err)
	}

	gprVeredicto(t, wt, slug, t0.Add(time.Minute), false)
	vdtCommit(t, wt, "verde de cierre")
	vdtColumna(t, ca, root, slug, boardcmd.ColTuAceptacion)
	s = vdtCorrer(t, ca, vdtOpts{dir: root}, "integrar", slug, "--si")
	vdtExit(t, ca, s, 0)
	vdtDiceOut(t, ca, s, "listo: la tarjeta pasa a Hecho")
	vdtColumna(t, ca, root, slug, boardcmd.ColHecho)
}
