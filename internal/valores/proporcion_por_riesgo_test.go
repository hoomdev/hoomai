// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// valores: la regla suma agents.<rol>.<riesgo> entre el comando y
// agents.<rol> (CA-804, que re-expresa CA-491), traduce los niveles con el
// provider del lanzamiento (CA-805) y la linea valores: muestra el nivel
// (CA-806). Escritos desde el spec, las aclaraciones de la ronda 0 y las
// firmas publicas, sin leer la implementacion.
package valores_test

import (
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/valores"
)

func pprF(v float64) *float64 { return &v }

// pprYAML es un writer con modelo medio en agents.writer y fuerte (y mas
// valores) en agents.writer.alto.
func pprYAML() manifest.RoleDefaults {
	return manifest.RoleDefaults{
		Provider: "claude",
		Model:    "medio",
		Effort:   "low",
		Timeout:  45 * time.Minute,
		PorRiesgo: map[string]manifest.RoleDefaults{
			"alto": {Model: "fuerte", Effort: "high", BudgetUSD: pprF(9), Timeout: 2 * time.Hour},
			"bajo": {Model: "barato"},
		},
	}
}

// CA-804: un writer en una tarjeta alto con agents.writer.alto.model lanza
// con ese modelo y origen `hoom.yaml, riesgo alto`; en una normal, con
// agents.writer.model y origen hoom.yaml.
func TestCA804_AltoUsaSuNivelYNormalElDelRol(t *testing.T) {
	const ca = "CA-804"
	if valores.OrigenRiesgo("alto") != "hoom.yaml, riesgo alto" || valores.OrigenRiesgo("bajo") != "hoom.yaml, riesgo bajo" {
		t.Fatalf("%s: OrigenRiesgo: %q", ca, valores.OrigenRiesgo("alto"))
	}
	e := valores.ResolverConRiesgo(valores.Pedido{}, "alto", pprYAML(), valores.Lugar{Provider: "codex"})
	if e.Provider != "claude" || e.Origen.Provider != valores.OrigenHoomYAML {
		t.Fatalf("%s: el provider sale de agents.writer: %+v", ca, e)
	}
	if e.Model != "opus" || e.Nivel != "fuerte" || e.Origen.Model != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: alto: modelo opus (nivel fuerte, hoom.yaml, riesgo alto): %+v", ca, e)
	}
	if e.Effort != "high" || e.Origen.Effort != "hoom.yaml, riesgo alto" || e.Timeout != 2*time.Hour || e.Origen.Timeout != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: alto: esfuerzo y tiempo del riesgo: %+v", ca, e)
	}
	if !vdtIgualPresu(e.BudgetUSD, pprF(9)) || e.Origen.BudgetUSD != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: alto: presupuesto del riesgo sin tope en la tarjeta: %v %q", ca, e.BudgetUSD, e.Origen.BudgetUSD)
	}
	e = valores.ResolverConRiesgo(valores.Pedido{}, "normal", pprYAML(), valores.Lugar{Provider: "codex"})
	if e.Model != "sonnet" || e.Nivel != "medio" || e.Origen.Model != valores.OrigenHoomYAML ||
		e.Effort != "low" || e.Origen.Effort != valores.OrigenHoomYAML || e.Timeout != 45*time.Minute {
		t.Fatalf("%s: normal: los de agents.writer: %+v", ca, e)
	}
	// bajo solo cambia el modelo: el resto sale de agents.writer
	e = valores.ResolverConRiesgo(valores.Pedido{}, "bajo", pprYAML(), valores.Lugar{})
	if e.Model != "haiku" || e.Origen.Model != "hoom.yaml, riesgo bajo" || e.Effort != "low" || e.Origen.Effort != valores.OrigenHoomYAML {
		t.Fatalf("%s: bajo: modelo del riesgo, esfuerzo de agents.writer: %+v", ca, e)
	}
	// un nombre por riesgo pasa tal cual
	y := manifest.RoleDefaults{Model: "m-yaml", PorRiesgo: map[string]manifest.RoleDefaults{"alto": {Model: "m-alto"}}}
	if e := valores.ResolverConRiesgo(valores.Pedido{}, "alto", y, valores.Lugar{Provider: "claude"}); e.Model != "m-alto" || e.Nivel != "" || e.Origen.Model != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: un nombre por riesgo: %+v", ca, e)
	}
}

// CA-804: el comando gana a los dos niveles de hoom.yaml, tambien con un
// nivel (traducido, origen comando).
func TestCA804_ElComandoGana(t *testing.T) {
	const ca = "CA-804"
	for _, r := range []string{"alto", "normal", "bajo", ""} {
		e := valores.ResolverConRiesgo(valores.Pedido{Model: "m-cmd", Effort: "max", Timeout: time.Hour, BudgetUSD: pprF(1.5)}, r, pprYAML(), valores.Lugar{Provider: "claude"})
		if e.Model != "m-cmd" || e.Nivel != "" || e.Origen.Model != valores.OrigenComando || e.Effort != "max" || e.Origen.Effort != valores.OrigenComando ||
			e.Timeout != time.Hour || e.Origen.Timeout != valores.OrigenComando || !vdtIgualPresu(e.BudgetUSD, pprF(1.5)) || e.Origen.BudgetUSD != valores.OrigenComando {
			t.Fatalf("%s: con riesgo %q el comando gana: %+v", ca, r, e)
		}
		e = valores.ResolverConRiesgo(valores.Pedido{Model: "barato"}, r, pprYAML(), valores.Lugar{Provider: "claude"})
		if e.Model != "haiku" || e.Nivel != "barato" || e.Origen.Model != valores.OrigenComando {
			t.Fatalf("%s: --model barato con riesgo %q: %+v", ca, r, e)
		}
	}
}

// CA-804: el provider nunca sale del nivel del riesgo; sin riesgo ("")
// PorRiesgo no cuenta (Resolver de hoy).
func TestCA804_ProviderNoVariaYSinRiesgoNoCuenta(t *testing.T) {
	const ca = "CA-804"
	y := pprYAML()
	y.PorRiesgo["alto"] = manifest.RoleDefaults{Provider: "codex", Model: "fuerte"}
	e := valores.ResolverConRiesgo(valores.Pedido{}, "alto", y, valores.Lugar{Provider: "claude"})
	if e.Provider != "claude" || e.Model != "opus" {
		t.Fatalf("%s: el provider no sale del riesgo; el modelo se traduce con claude: %+v", ca, e)
	}
	for _, e := range []valores.Elegidos{
		valores.ResolverConRiesgo(valores.Pedido{}, "", pprYAML(), valores.Lugar{Provider: "claude"}),
		valores.Resolver(valores.Pedido{}, pprYAML(), valores.Lugar{Provider: "claude"}),
	} {
		if e.Model != "sonnet" || e.Origen.Model != valores.OrigenHoomYAML || e.Effort != "low" {
			t.Fatalf("%s: sin riesgo, agents.writer (Resolver de hoy): %+v", ca, e)
		}
	}
	// un riesgo sin entrada es agents.writer
	if e := valores.ResolverConRiesgo(valores.Pedido{}, "normal", manifest.RoleDefaults{Model: "m"}, valores.Lugar{}); e.Model != "m" || e.Origen.Model != valores.OrigenHoomYAML {
		t.Fatalf("%s: sin PorRiesgo: %+v", ca, e)
	}
}

// CA-804: el presupuesto por riesgo no pasa lo que le queda a la tarjeta
// (propiedad, como CA-491 con el de hoom.yaml); 0 por riesgo toma lo que
// queda.
func TestCA804_PresupuestoPorRiesgoAcotado(t *testing.T) {
	const ca = "CA-804"
	e := valores.ResolverConRiesgo(valores.Pedido{}, "alto", pprYAML(), valores.Lugar{Provider: "claude", BudgetUSD: pprF(4)})
	if !vdtIgualPresu(e.BudgetUSD, pprF(4)) {
		t.Fatalf("%s: budget_usd 9 del riesgo con 4 restantes es 4: %v", ca, e.BudgetUSD)
	}
	y := manifest.RoleDefaults{BudgetUSD: pprF(1), PorRiesgo: map[string]manifest.RoleDefaults{"alto": {BudgetUSD: pprF(0)}}}
	if e := valores.ResolverConRiesgo(valores.Pedido{}, "alto", y, valores.Lugar{Provider: "claude", BudgetUSD: pprF(3)}); !vdtIgualPresu(e.BudgetUSD, pprF(3)) || e.Origen.BudgetUSD != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: budget_usd 0 del riesgo toma lo que queda: %v %q", ca, e.BudgetUSD, e.Origen.BudgetUSD)
	}
	prop := func(riesgoCents, quedaCents uint16) bool {
		r := float64(riesgoCents%5000)/100 + 0.01
		q := float64(quedaCents%5000)/100 + 0.01
		y := manifest.RoleDefaults{PorRiesgo: map[string]manifest.RoleDefaults{"alto": {BudgetUSD: pprF(r)}}}
		e := valores.ResolverConRiesgo(valores.Pedido{}, "alto", y, valores.Lugar{Provider: "claude", BudgetUSD: pprF(q)})
		return e.BudgetUSD != nil && *e.BudgetUSD <= q+1e-9
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatalf("%s: el presupuesto del riesgo nunca pasa lo que queda: %v", ca, err)
	}
}

// CA-805: un nivel se traduce con el provider elegido; sin provider todavia
// queda el nivel sin modelo y ConProvider lo traduce (tambien a nada en
// gemini); un nombre no cambia.
func TestCA805_ConProviderTraduceElNivel(t *testing.T) {
	const ca = "CA-805"
	e := valores.ResolverConRiesgo(valores.Pedido{}, "normal", manifest.RoleDefaults{Model: "medio"}, valores.Lugar{})
	if e.Provider != "" || e.Model != "" || e.Nivel != "medio" {
		t.Fatalf("%s: sin provider el nivel espera: %+v", ca, e)
	}
	c := e.ConProvider("codex")
	if c.Provider != "codex" || c.Model != "gpt-6.1-sol" || c.Nivel != "medio" || c.Origen.Provider != e.Origen.Provider || c.Origen.Model != e.Origen.Model {
		t.Fatalf("%s: ConProvider(codex) traduce medio y deja los origenes: %+v", ca, c)
	}
	if g := c.ConProvider("gemini"); g.Model != "" || g.Nivel != "medio" || g.Provider != "gemini" {
		t.Fatalf("%s: gemini no recibe modelo: %+v", ca, g)
	}
	if cl := c.ConProvider("claude"); cl.Model != "sonnet" {
		t.Fatalf("%s: de codex a claude se re-traduce: %+v", ca, cl)
	}
	nombre := valores.Elegidos{Provider: "claude", Model: "m-x", Origen: valores.Origenes{Model: valores.OrigenHoomYAML}}
	if n := nombre.ConProvider("codex"); n.Model != "m-x" || n.Nivel != "" {
		t.Fatalf("%s: un nombre queda como esta: %+v", ca, n)
	}
	// gemini elegido por hoom.yaml con un nivel: ningun modelo
	if g := valores.ResolverConRiesgo(valores.Pedido{}, "alto", manifest.RoleDefaults{Provider: "gemini", Model: "fuerte"}, valores.Lugar{}); g.Model != "" || g.Nivel != "fuerte" {
		t.Fatalf("%s: a gemini no le llega modelo: %+v", ca, g)
	}
	// Fuerte no es nivel
	if f := valores.ResolverConRiesgo(valores.Pedido{}, "normal", manifest.RoleDefaults{Model: "Fuerte"}, valores.Lugar{Provider: "claude"}); f.Model != "Fuerte" || f.Nivel != "" {
		t.Fatalf("%s: Fuerte pasa como nombre: %+v", ca, f)
	}
}

// CA-805: Sobre traduce un nivel del comando con el provider del
// lanzamiento, y re-traduce un modelo de nivel de la base cuando el comando
// pide otro provider; un nombre de la base queda.
func TestCA805_SobreTraduce(t *testing.T) {
	const ca = "CA-805"
	base := valores.Elegidos{Provider: "codex", Model: "gpt-6-astra", Nivel: "fuerte", Timeout: time.Hour,
		Origen: valores.Origenes{Provider: valores.OrigenTarjeta, Model: "hoom.yaml, riesgo alto", Timeout: valores.OrigenProvider}}
	s := valores.Sobre(valores.Pedido{Model: "medio"}, base)
	if s.Model != "gpt-6.1-sol" || s.Nivel != "medio" || s.Origen.Model != valores.OrigenComando || s.Provider != "codex" {
		t.Fatalf("%s: --model medio sobre codex: %+v", ca, s)
	}
	s = valores.Sobre(valores.Pedido{Provider: "claude"}, base)
	if s.Provider != "claude" || s.Model != "opus" || s.Nivel != "fuerte" || s.Origen.Model != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: otro provider re-traduce el nivel de la base con su origen: %+v", ca, s)
	}
	s = valores.Sobre(valores.Pedido{Provider: "claude", Model: "barato"}, base)
	if s.Model != "haiku" || s.Nivel != "barato" {
		t.Fatalf("%s: --provider claude --model barato: %+v", ca, s)
	}
	nombre := base
	nombre.Model, nombre.Nivel = "m-x", ""
	if s := valores.Sobre(valores.Pedido{Provider: "claude"}, nombre); s.Model != "m-x" {
		t.Fatalf("%s: un nombre de la base queda: %+v", ca, s)
	}
	if s := valores.Sobre(valores.Pedido{Provider: "gemini"}, base); s.Model != "" || s.Nivel != "fuerte" {
		t.Fatalf("%s: a gemini no le llega el nivel de la base: %+v", ca, s)
	}
}

// CA-806: la linea valores: muestra `modelo <m> (nivel <n>, <origen>)`; en
// un provider sin modelo, `modelo del provider (nivel <n>: <p> no recibe
// modelo)`; un nombre como hoy.
func TestCA806_LineaConNivel(t *testing.T) {
	const ca = "CA-806"
	e := valores.Elegidos{Provider: "claude", Model: "opus", Nivel: "fuerte", Effort: "high", BudgetUSD: pprF(5), Timeout: 45 * time.Minute,
		Origen: valores.Origenes{Provider: valores.OrigenHoomYAML, Model: "hoom.yaml, riesgo alto", Effort: valores.OrigenHoomYAML, BudgetUSD: valores.OrigenTarjeta, Timeout: valores.OrigenHoomYAML}}
	want := "valores: provider claude (hoom.yaml) · modelo opus (nivel fuerte, hoom.yaml, riesgo alto) · esfuerzo high (hoom.yaml) · presupuesto 5 (tarjeta) · tiempo limite 45m (hoom.yaml)"
	if got := valores.Linea(e, false); got != want {
		t.Fatalf("%s: la linea con nivel\n%q\nfue\n%q", ca, want, got)
	}
	e.Model, e.Nivel, e.Origen.Model = "m-yaml", "", valores.OrigenHoomYAML
	if got := valores.Linea(e, false); !strings.Contains(got, " · modelo m-yaml (hoom.yaml) · ") || strings.Contains(got, "nivel") {
		t.Fatalf("%s: un nombre como hoy: %q", ca, got)
	}
	g := valores.Elegidos{Provider: "gemini", Model: "", Nivel: "medio", Timeout: 30 * time.Minute,
		Origen: valores.Origenes{Provider: valores.OrigenHoomYAML, Model: valores.OrigenHoomYAML, Effort: valores.OrigenProvider, BudgetUSD: valores.OrigenProvider, Timeout: valores.OrigenProvider}}
	got := valores.Linea(g, true)
	if !strings.HasPrefix(got, "valores: provider gemini (hoom.yaml) · modelo del provider (nivel medio: gemini no recibe modelo) · ") {
		t.Fatalf("%s: en gemini, modelo del provider con el nivel: %q", ca, got)
	}
	if !strings.Contains(got, "presupuesto sin tope (gemini no acepta tope)") {
		t.Fatalf("%s: sinTope sigue como hoy: %q", ca, got)
	}
	// ningun nivel sin traducir llega a la linea
	for _, n := range []string{"modelo fuerte", "modelo medio", "modelo barato"} {
		if strings.Contains(valores.Linea(e, false), n) || strings.Contains(got, n) {
			t.Fatalf("%s: ningun nivel llega sin traducir: %q", ca, n)
		}
	}
}

// CA-491 (re-expresado por CA-804): la regla suma el nivel del riesgo entre
// el comando y agents.<rol>; Elegidos gana Nivel, vacio cuando el modelo no
// viene de un nivel.
func TestCA491_LaReglaSumaElNivelDelRiesgo(t *testing.T) {
	const ca = "CA-491"
	y := manifest.RoleDefaults{Model: "m-rol", PorRiesgo: map[string]manifest.RoleDefaults{"alto": {Model: "m-alto"}}}
	if e := valores.ResolverConRiesgo(valores.Pedido{Model: "m-cmd"}, "alto", y, valores.Lugar{Provider: "claude"}); e.Model != "m-cmd" || e.Origen.Model != valores.OrigenComando || e.Nivel != "" {
		t.Fatalf("%s: 1, el comando: %+v", ca, e)
	}
	if e := valores.ResolverConRiesgo(valores.Pedido{}, "alto", y, valores.Lugar{Provider: "claude"}); e.Model != "m-alto" || e.Origen.Model != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: 2, agents.<rol>.<riesgo>: %+v", ca, e)
	}
	if e := valores.ResolverConRiesgo(valores.Pedido{}, "bajo", y, valores.Lugar{Provider: "claude"}); e.Model != "m-rol" || e.Origen.Model != valores.OrigenHoomYAML {
		t.Fatalf("%s: 3, agents.<rol>: %+v", ca, e)
	}
	if e := valores.ResolverConRiesgo(valores.Pedido{}, "bajo", manifest.RoleDefaults{}, valores.Lugar{Provider: "claude"}); e.Model != "" || e.Nivel != "" || e.Origen.Model != valores.OrigenProvider || e.Provider != "claude" {
		t.Fatalf("%s: 4 y 5, el lugar y el provider: %+v", ca, e)
	}
}
