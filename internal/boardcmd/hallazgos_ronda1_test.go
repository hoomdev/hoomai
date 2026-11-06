// Tests adversariales de la ronda 1 de la review de verbos-de-la-tarjeta en
// el tablero: hallazgos 4a1c25, 810954 y 42550b (CA-492, CA-496). La accion
// emite en su JSON de donde salio cada valor que propone (clave origen), para
// que el Studio diga "(el de hoom)" solo cuando nada declaro el tiempo limite
// y nombre el valor de hoom.yaml en los campos vacios. Es el mismo JSON de
// hoom board --json, hoom item show --json y /api/board (CA-316).
package boardcmd

import (
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/valores"
)

var r1Claves = []string{"provider", "model", "effort", "budget_usd", "timeout"}

var r1Origenes = map[string]bool{valores.OrigenComando: true, valores.OrigenHoomYAML: true,
	valores.OrigenTarjeta: true, valores.OrigenProvider: true}

// r1Origen devuelve el objeto origen de la accion id en el JSON de la
// tarjeta, exigiendo sus cinco claves con un origen del contrato.
func r1Origen(t *testing.T, ca string, c Card, id string) (map[string]any, map[string]any) {
	t.Helper()
	m, raw := acJSON(t, c)
	acts, _ := m["actions"].([]any)
	for _, x := range acts {
		a, _ := x.(map[string]any)
		if a["id"] != id {
			continue
		}
		o, ok := a["origen"].(map[string]any)
		if !ok {
			t.Fatalf("%s: la accion %s trae en su JSON la clave origen (un objeto): %v\n%s", ca, id, a["origen"], raw)
		}
		for _, k := range r1Claves {
			v, _ := o[k].(string)
			if !r1Origenes[v] {
				t.Fatalf("%s: origen.%s de %s es uno de comando, hoom.yaml, tarjeta o provider: %v", ca, k, id, o)
			}
		}
		return a, o
	}
	t.Fatalf("%s: la tarjeta (%s) no tiene %s:\n%s", ca, c.Column, id, raw)
	return nil, nil
}

// hallazgos 4a1c25, 810954 y 42550b, CA-492: toda accion de rol, en toda
// columna, trae origen con sus cinco claves.
func TestHallazgo_4a1c25_CA492_TodaAccionDeRolTraeSuOrigen(t *testing.T) {
	for _, c := range acColumnas() {
		card := Derive(c.ev)
		for _, a := range card.Actions {
			if acDeRol[a.ID] {
				r1Origen(t, "hallazgo 4a1c25, CA-492 ("+c.nombre+")", card, a.ID)
			}
		}
	}
}

// hallazgos 4a1c25 y 42550b, CA-496: un timeout de 30m declarado da
// timeout_s 1800 con origen hoom.yaml (el Studio no lo llama "el de hoom");
// sin nada declarado, 1800 con origen provider.
func TestHallazgo_42550b_CA496_TimeoutDeclaradoDe30mNoEsElDeHoom(t *testing.T) {
	a, o := r1Origen(t, "hallazgo 42550b", Derive(vdtCon(acWriter(), "writer", manifest.RoleDefaults{Timeout: 30 * time.Minute})), ActPedirWriter)
	if a["timeout_s"] != float64(1800) || o["timeout"] != valores.OrigenHoomYAML {
		t.Fatalf("hallazgo 42550b, CA-496: agents.writer.timeout: 30m da timeout_s 1800 con origen.timeout hoom.yaml: %v %v", a["timeout_s"], o)
	}
	a, o = r1Origen(t, "hallazgo 42550b", Derive(acWriter()), ActPedirWriter)
	if a["timeout_s"] != float64(1800) || o["timeout"] != valores.OrigenProvider {
		t.Fatalf("hallazgo 42550b, CA-496: sin nada declarado, timeout_s 1800 con origen.timeout provider: %v %v", a["timeout_s"], o)
	}
}

// hallazgo 810954, CA-496: el origen del modelo y del esfuerzo dice si el
// valor viene de hoom.yaml (y el del provider y el presupuesto, de donde
// salieron), igual que Action.Origen.
func TestHallazgo_810954_CA496_OrigenDelModeloYElEsfuerzo(t *testing.T) {
	d := manifest.RoleDefaults{Provider: "claude", Model: "m-yaml", Effort: "high", BudgetUSD: bdF(3)}
	card := Derive(vdtCon(acWriter(), "writer", d))
	a, o := r1Origen(t, "hallazgo 810954", card, ActPedirWriter)
	if a["model"] != "m-yaml" || a["effort"] != "high" || o["model"] != valores.OrigenHoomYAML || o["effort"] != valores.OrigenHoomYAML ||
		o["provider"] != valores.OrigenHoomYAML || o["budget_usd"] != valores.OrigenHoomYAML {
		t.Fatalf("hallazgo 810954, CA-496: modelo, esfuerzo, provider y presupuesto de agents.writer con origen hoom.yaml: %v %v", a, o)
	}
	acc := acAccion(t, "hallazgo 810954", card, ActPedirWriter)
	if o["provider"] != acc.Origen.Provider || o["model"] != acc.Origen.Model || o["effort"] != acc.Origen.Effort ||
		o["budget_usd"] != acc.Origen.BudgetUSD || o["timeout"] != acc.Origen.Timeout {
		t.Fatalf("hallazgo 810954: el JSON emite Action.Origen tal cual: %v vs %+v", o, acc.Origen)
	}

	_, o = r1Origen(t, "hallazgo 810954", Derive(acWriter()), ActPedirWriter)
	if o["model"] != valores.OrigenProvider || o["effort"] != valores.OrigenProvider || o["provider"] != valores.OrigenTarjeta {
		t.Fatalf("hallazgo 810954, CA-496: sin nada declarado, modelo y esfuerzo del provider y el provider de la tarjeta: %v", o)
	}
}
