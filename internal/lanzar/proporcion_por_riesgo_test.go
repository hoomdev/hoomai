// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// lanzar.Valores (CA-805): un nivel de la accion o del comando se traduce con
// el provider que toma el lanzamiento; Model nunca es un nivel; un nombre
// pasa igual. Escritos desde el spec, las aclaraciones de la ronda 0 y las
// firmas publicas, sin leer la implementacion.
package lanzar_test

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/valores"
)

// CA-805: la accion trae fuerte traducido para codex; el lanzamiento en codex
// lo deja, en claude lo re-traduce, en gemini no lleva modelo; Nivel se
// conserva con su origen.
func TestCA805_ValoresTraduceElNivelDeLaAccion(t *testing.T) {
	const ca = "CA-805"
	a := vdtAccion()
	a.Model, a.Nivel = "gpt-6-astra", "fuerte"
	a.Origen.Model = "hoom.yaml, riesgo alto"
	e := lanzar.Valores(a, lanzar.Request{Action: a.ID, Pedido: "x"})
	if e.Provider != "codex" || e.Model != "gpt-6-astra" || e.Nivel != "fuerte" || e.Origen.Model != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: en codex, el de la accion: %+v", ca, e)
	}
	e = lanzar.Valores(a, lanzar.Request{Action: a.ID, Provider: "claude", Pedido: "x"})
	if e.Provider != "claude" || e.Model != "opus" || e.Nivel != "fuerte" || e.Origen.Model != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: en claude se re-traduce fuerte a opus: %+v", ca, e)
	}
	e = lanzar.Valores(a, lanzar.Request{Action: a.ID, Provider: "gemini", Pedido: "x"})
	if e.Model != "" || e.Nivel != "fuerte" {
		t.Fatalf("%s: a gemini no le llega modelo: %+v", ca, e)
	}
}

// CA-805: --model medio se traduce con el provider del lanzamiento (el
// default de la accion o el pedido), con origen comando; Fuerte pasa como
// nombre; ningun nivel queda en Model.
func TestCA805_ValoresConModelMedio(t *testing.T) {
	const ca = "CA-805"
	a := vdtAccion()
	e := lanzar.Valores(a, lanzar.Request{Action: a.ID, Model: "medio", Pedido: "x"})
	if e.Provider != "codex" || e.Model != "gpt-6.1-sol" || e.Nivel != "medio" || e.Origen.Model != valores.OrigenComando {
		t.Fatalf("%s: --model medio en codex (default): %+v", ca, e)
	}
	e = lanzar.Valores(a, lanzar.Request{Action: a.ID, Provider: "claude", Model: "medio", Pedido: "x"})
	if e.Model != "sonnet" || e.Nivel != "medio" {
		t.Fatalf("%s: --provider claude --model medio: %+v", ca, e)
	}
	e = lanzar.Valores(a, lanzar.Request{Action: a.ID, Provider: "claude", Model: "Fuerte", Pedido: "x"})
	if e.Model != "Fuerte" || e.Nivel != "" {
		t.Fatalf("%s: Fuerte pasa como nombre: %+v", ca, e)
	}
	// la accion con un nombre: queda en cualquier provider
	e = lanzar.Valores(a, lanzar.Request{Action: a.ID, Provider: "claude", Pedido: "x"})
	if e.Model != "m-accion" || e.Nivel != "" {
		t.Fatalf("%s: un nombre de la accion pasa igual: %+v", ca, e)
	}
	for _, r := range []lanzar.Request{
		{Action: a.ID, Model: "fuerte"}, {Action: a.ID, Model: "barato", Provider: "claude"}, {Action: a.ID, Model: "medio", Provider: "codex"},
	} {
		if e := lanzar.Valores(a, r); e.Model == "fuerte" || e.Model == "medio" || e.Model == "barato" {
			t.Fatalf("%s: Model nunca es un nivel: %+v", ca, e)
		}
	}
}
