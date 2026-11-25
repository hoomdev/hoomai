// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre el
// tablero (CA-815): con el error de taskcmd.Ready de kind sin-cierre, la
// tarjeta dice `falta la verificacion de cierre del espacio de trabajo` y su
// siguiente paso es la accion que el mensaje trae despues de "Accion: ", como
// en TestCA633 con el tope de pendientes; no esta en Tu aceptacion ni ofrece
// integrar (aclaracion 19). Derive con Evidence literales (bdEv).
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package boardcmd

import (
	"fmt"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// CA-815: el tablero muestra el texto de sin-cierre: Plain el del spec, Next
// lo que sigue a "Accion: " en el mensaje, fuera de Tu aceptacion y sin la
// accion integrar.
func TestCA815_ElTableroMuestraSinCierre(t *testing.T) {
	const ca = "CA-815"
	ev := bdEv()
	const motivo = "ningun archivo del cambio coincide con aplica_en (app/**)"
	gs := append(bdGatesVerdes(),
		verdict.GateResult{Name: "static", Required: true, Status: verdict.StatusNoAplica, Scope: "none", Notes: motivo},
		verdict.GateResult{Name: "mutation", Required: true, Status: verdict.StatusNoAplica, Scope: "none", Notes: motivo})
	v := bdVeredicto("2026-09-22T15-30-00Z_bbbb2222", bdT0.Add(30*time.Minute), "huella-1", 50, 10, gs)
	ev.Verdict, ev.GreenVerdicts = v, []string{v.ID}
	msg := fmt.Sprintf(taskcmd.FormatoSinCierre, bdSlug, "static, mutation")
	ev.ReadyErr, ev.ReadyKind = msg, taskcmd.ReadySinCierre

	c := Derive(ev)
	if c.Plain != "falta la verificacion de cierre del espacio de trabajo" {
		t.Fatalf("%s: el texto llano de sin-cierre: %q (missing=%q)", ca, c.Plain, c.Missing)
	}
	if c.Next != "ejecuta 'hoom verify --cierre' dentro del worktree y commitea" {
		t.Fatalf("%s: el siguiente paso es la accion del mensaje: %q", ca, c.Next)
	}
	if c.Column == ColTuAceptacion || c.Column == ColHecho {
		t.Fatalf("%s: con sin-cierre la tarjeta no esta en Tu aceptacion: %s", ca, c.Column)
	}
	if _, ok := c.Action(ActIntegrar); ok {
		t.Fatalf("%s: con sin-cierre no se ofrece integrar: %+v", ca, c.Actions)
	}
}
