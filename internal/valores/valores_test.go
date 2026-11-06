// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md
// (CA-491): la regla de los valores. Para cada valor gana el primero que
// existe: el comando, el hoom.yaml del proyecto, lo que propone el lugar, lo
// del provider (y, para el tiempo limite, los 30 minutos de hoom). El
// presupuesto de hoom.yaml nunca pasa lo que le queda a la tarjeta. Cada
// valor sale con su origen: ninguno se elige en silencio.
package valores_test

import (
	"math"
	"math/rand"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/valores"
)

func vdtF(v float64) *float64 { return &v }

// vdtIgualPresu compara dos presupuestos (nil = sin tope).
func vdtIgualPresu(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Abs(*a-*b) < 1e-9
}

func vdtTodos(p string) bool { return true }

// CA-491: con todo pedido en el comando, gana el comando en cada valor, con
// origen comando, aunque hoom.yaml y el lugar digan otra cosa.
func TestCA491_ElComandoGanaEnCadaValor(t *testing.T) {
	yaml := manifest.RoleDefaults{Provider: "codex", Model: "m-yaml", Effort: "e-yaml", BudgetUSD: vdtF(4), Timeout: 45 * time.Minute}
	lugar := valores.Lugar{Provider: "claude", Puede: vdtTodos, BudgetUSD: vdtF(9)}
	e := valores.Resolver(valores.Pedido{Provider: "gemini", Model: "m-cmd", Effort: "e-cmd", BudgetUSD: vdtF(3), Timeout: 10 * time.Minute}, yaml, lugar)
	if e.Provider != "gemini" || e.Model != "m-cmd" || e.Effort != "e-cmd" || !vdtIgualPresu(e.BudgetUSD, vdtF(3)) || e.Timeout != 10*time.Minute {
		t.Fatalf("CA-491: lo del comando gana: %+v (presupuesto %v)", e, e.BudgetUSD)
	}
	o := e.Origen
	for nombre, got := range map[string]string{"provider": o.Provider, "model": o.Model, "effort": o.Effort, "budget": o.BudgetUSD, "timeout": o.Timeout} {
		if got != valores.OrigenComando {
			t.Fatalf("CA-491: el origen de %s es %q, fue %q", nombre, valores.OrigenComando, got)
		}
	}
}

// CA-491: el presupuesto del comando no se recorta en silencio: si pasa lo
// que queda, lo dice el lanzamiento (409), no la regla.
func TestCA491_ElPresupuestoDelComandoNoSeRecorta(t *testing.T) {
	e := valores.Resolver(valores.Pedido{BudgetUSD: vdtF(3)}, manifest.RoleDefaults{}, valores.Lugar{Provider: "claude", BudgetUSD: vdtF(2)})
	if !vdtIgualPresu(e.BudgetUSD, vdtF(3)) || e.Origen.BudgetUSD != valores.OrigenComando {
		t.Fatalf("CA-491: el presupuesto pedido viaja tal cual (3, comando): %v %q", e.BudgetUSD, e.Origen.BudgetUSD)
	}
}

// CA-491: sin comando, hoom.yaml del proyecto gana a lo que propone el lugar.
func TestCA491_HoomYamlGanaAlLugar(t *testing.T) {
	yaml := manifest.RoleDefaults{Provider: "codex", Model: "m-yaml", Effort: "e-yaml", BudgetUSD: vdtF(1.5), Timeout: 45 * time.Minute}
	e := valores.Resolver(valores.Pedido{}, yaml, valores.Lugar{Provider: "claude", Puede: vdtTodos, BudgetUSD: vdtF(5)})
	if e.Provider != "codex" || e.Model != "m-yaml" || e.Effort != "e-yaml" || !vdtIgualPresu(e.BudgetUSD, vdtF(1.5)) || e.Timeout != 45*time.Minute {
		t.Fatalf("CA-491: sin comando gana hoom.yaml: %+v (presupuesto %v)", e, e.BudgetUSD)
	}
	o := e.Origen
	for nombre, got := range map[string]string{"provider": o.Provider, "model": o.Model, "effort": o.Effort, "budget": o.BudgetUSD, "timeout": o.Timeout} {
		if got != valores.OrigenHoomYAML {
			t.Fatalf("CA-491: el origen de %s es %q, fue %q", nombre, valores.OrigenHoomYAML, got)
		}
	}
	// Puede nil: cualquier provider se puede tomar
	e = valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{Provider: "codex"}, valores.Lugar{Provider: "claude"})
	if e.Provider != "codex" || e.Origen.Provider != valores.OrigenHoomYAML {
		t.Fatalf("CA-491: con Puede nil el provider de hoom.yaml gana: %+v", e)
	}
}

// CA-491 y CA-492: el provider de hoom.yaml gana solo cuando es una opcion
// con ok; si no, el del lugar (el primero que puede), con origen tarjeta.
func TestCA491_ProviderDeHoomYamlSoloSiPuede(t *testing.T) {
	puede := func(p string) bool { return p == "claude" }
	e := valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{Provider: "opencode"}, valores.Lugar{Provider: "claude", Puede: puede})
	if e.Provider != "claude" || e.Origen.Provider != valores.OrigenTarjeta {
		t.Fatalf("CA-491: un provider de hoom.yaml que no puede cede al del lugar (claude, tarjeta): %+v", e)
	}
	e = valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{Provider: "claude"}, valores.Lugar{Provider: "codex", Puede: puede})
	if e.Provider != "claude" || e.Origen.Provider != valores.OrigenHoomYAML {
		t.Fatalf("CA-491: un provider de hoom.yaml que puede gana: %+v", e)
	}
}

// CA-491: sin comando ni hoom.yaml, lo del lugar (origen tarjeta); sin nada,
// lo del provider: modelo y esfuerzo vacios, el tiempo limite de hoom.
func TestCA491_LugarYDespuesProvider(t *testing.T) {
	e := valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{}, valores.Lugar{Provider: "claude", BudgetUSD: vdtF(2)})
	if e.Provider != "claude" || e.Origen.Provider != valores.OrigenTarjeta {
		t.Fatalf("CA-491: el provider del lugar con origen tarjeta: %+v", e)
	}
	if !vdtIgualPresu(e.BudgetUSD, vdtF(2)) || e.Origen.BudgetUSD != valores.OrigenTarjeta {
		t.Fatalf("CA-491: el presupuesto que le queda a la tarjeta con origen tarjeta: %v %q", e.BudgetUSD, e.Origen.BudgetUSD)
	}
	if e.Model != "" || e.Effort != "" || e.Origen.Model != valores.OrigenProvider || e.Origen.Effort != valores.OrigenProvider {
		t.Fatalf("CA-491: sin modelo ni esfuerzo, los del provider: %+v", e)
	}
	if valores.DefaultTimeout != 30*time.Minute {
		t.Fatalf("CA-491: el ultimo nivel del tiempo limite son los 30 minutos de hoom: %s", valores.DefaultTimeout)
	}
	if e.Timeout != valores.DefaultTimeout || e.Origen.Timeout != valores.OrigenProvider {
		t.Fatalf("CA-491: sin tiempo limite, los 30 minutos de hoom con origen provider: %s %q", e.Timeout, e.Origen.Timeout)
	}
}

// CA-491: el presupuesto de hoom.yaml nunca pasa lo que le queda a la
// tarjeta (budget_usd 5 y 2 restantes: 2); por debajo, el de hoom.yaml.
func TestCA491_PresupuestoDeHoomYamlTopeadoPorLoQueQueda(t *testing.T) {
	e := valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{BudgetUSD: vdtF(5)}, valores.Lugar{Provider: "claude", BudgetUSD: vdtF(2)})
	if !vdtIgualPresu(e.BudgetUSD, vdtF(2)) {
		t.Fatalf("CA-491: con budget_usd 5 y 2 USD restantes la propuesta es 2: %v", e.BudgetUSD)
	}
	e = valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{BudgetUSD: vdtF(1)}, valores.Lugar{Provider: "claude", BudgetUSD: vdtF(2)})
	if !vdtIgualPresu(e.BudgetUSD, vdtF(1)) || e.Origen.BudgetUSD != valores.OrigenHoomYAML {
		t.Fatalf("CA-491: con budget_usd 1 y 2 restantes la propuesta es 1, de hoom.yaml: %v %q", e.BudgetUSD, e.Origen.BudgetUSD)
	}
	// sin presupuesto en la tarjeta, el de hoom.yaml
	e = valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{BudgetUSD: vdtF(3)}, valores.Lugar{Provider: "claude"})
	if !vdtIgualPresu(e.BudgetUSD, vdtF(3)) || e.Origen.BudgetUSD != valores.OrigenHoomYAML {
		t.Fatalf("CA-491: sin tope en la tarjeta, el presupuesto de hoom.yaml: %v %q", e.BudgetUSD, e.Origen.BudgetUSD)
	}
}

// CA-491: budget_usd 0 es sin tope: toma lo que le queda a la tarjeta, o
// sin tope si la tarjeta no tiene.
func TestCA491_BudgetCeroTomaLoQueQuedaOSinTope(t *testing.T) {
	e := valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{BudgetUSD: vdtF(0)}, valores.Lugar{Provider: "claude", BudgetUSD: vdtF(2)})
	if !vdtIgualPresu(e.BudgetUSD, vdtF(2)) {
		t.Fatalf("CA-491: budget_usd 0 con 2 restantes propone 2: %v", e.BudgetUSD)
	}
	e = valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{BudgetUSD: vdtF(0)}, valores.Lugar{Provider: "claude"})
	if e.BudgetUSD != nil {
		t.Fatalf("CA-491: budget_usd 0 sin tope en la tarjeta es sin tope: %v", *e.BudgetUSD)
	}
	if e.Origen.BudgetUSD != valores.OrigenHoomYAML {
		t.Fatalf("CA-491: el sin tope sale de hoom.yaml: %q", e.Origen.BudgetUSD)
	}
}

// CA-491 (propiedad): sin presupuesto en el comando, la propuesta nunca pasa
// lo que le queda a la tarjeta, sea cual sea el de hoom.yaml.
func TestCA491_PropiedadNuncaMasDeLoQueQueda(t *testing.T) {
	prop := func(yamlCents, quedaCents uint16, declarado bool) bool {
		queda := float64(uint32(quedaCents)+1) / 100 // lo que queda nunca es 0 aca: con 0 no hay lanzamiento
		var y *float64
		if declarado {
			y = vdtF(float64(yamlCents) / 100)
		}
		e := valores.Resolver(valores.Pedido{}, manifest.RoleDefaults{BudgetUSD: y}, valores.Lugar{Provider: "claude", BudgetUSD: vdtF(queda)})
		return e.BudgetUSD != nil && *e.BudgetUSD <= queda+1e-9
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500, Rand: rand.New(rand.NewSource(4910))}); err != nil {
		t.Fatalf("CA-491: la propuesta nunca pasa lo que le queda a la tarjeta: %v", err)
	}
}

// CA-491 (propiedad): el tiempo limite nunca es 0, y el origen de cada valor
// es uno de los cuatro del contrato.
func TestCA491_PropiedadTimeoutNuncaCeroYOrigenesDelContrato(t *testing.T) {
	validos := map[string]bool{valores.OrigenComando: true, valores.OrigenHoomYAML: true, valores.OrigenTarjeta: true, valores.OrigenProvider: true}
	prop := func(cmd, yaml uint8, conModelo, conProvider bool) bool {
		p := valores.Pedido{Timeout: time.Duration(cmd%3) * 20 * time.Minute}
		y := manifest.RoleDefaults{Timeout: time.Duration(yaml%3) * 15 * time.Minute}
		if conModelo {
			y.Model = "m"
		}
		l := valores.Lugar{}
		if conProvider {
			l.Provider = "claude"
		}
		e := valores.Resolver(p, y, l)
		if e.Timeout <= 0 {
			return false
		}
		if p.Timeout > 0 && e.Timeout != p.Timeout {
			return false
		}
		if p.Timeout == 0 && y.Timeout > 0 && e.Timeout != y.Timeout {
			return false
		}
		o := e.Origen
		return validos[o.Model] && validos[o.Effort] && validos[o.Timeout] && validos[o.BudgetUSD] && (validos[o.Provider] || !conProvider)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(4911))}); err != nil {
		t.Fatalf("CA-491: timeout nunca 0 y origenes del contrato: %v", err)
	}
}

// CA-503 (la forma del tiempo limite en la linea valores: y en --timeout):
// 45m, 1h30m, 2h, 30s, 30m.
func TestCA503_DuracionComoLaImprimeHoom(t *testing.T) {
	for d, want := range map[time.Duration]string{
		45 * time.Minute: "45m", 90 * time.Minute: "1h30m", 2 * time.Hour: "2h", 30 * time.Second: "30s",
		30 * time.Minute: "30m", time.Minute: "1m", 12 * time.Hour: "12h", 2 * time.Second: "2s",
		61 * time.Second: "1m1s", 3*time.Hour + 5*time.Minute: "3h5m",
	} {
		if got := valores.Duracion(d); got != want {
			t.Fatalf("CA-503: Duracion(%s) es %q, fue %q", d, want, got)
		}
	}
}
