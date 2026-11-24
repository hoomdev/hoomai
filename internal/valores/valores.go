// Package valores is the ONE rule that decides the launch values of a role
// (spec verbos-de-la-tarjeta): provider, model, effort, budget and timeout.
// For each value the first that exists wins: what the command (or the
// Studio's dialog) asked, then the project's hoom.yaml (manifest.RoleDefaults),
// then what the place proposes today (the card, or `hoom agent`), then the
// provider's own (and, for the timeout, hoom's 30 minutes). Every value comes
// out with where it came from: no value is chosen in silence.
//
// proporcion-por-riesgo adds a level between the command and agents.<rol>:
// agents.<rol>.<riesgo> for the declared riesgo of the launch's card
// (ResolverConRiesgo), and translates a model level with the launch's
// provider. It holds for every launch: `hoom agent`, pedir, siguiente,
// reanudar, the board, the Studio and the pilot.
package valores

import (
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Origins of a value, in the words the card verbs print.
const (
	OrigenComando  = "comando"
	OrigenHoomYAML = "hoom.yaml"
	OrigenTarjeta  = "tarjeta"
	OrigenProvider = "provider"
)

// DefaultTimeout is the last level of the timeout: hoom's own.
const DefaultTimeout = runcmd.DefaultTimeout

// Pedido is what the command asked for: "" / nil / 0 = not asked.
type Pedido struct {
	Provider  string
	Model     string
	Effort    string
	BudgetUSD *float64
	Timeout   time.Duration
}

// Lugar is what the place proposes today (level 3): on the card, the first
// provider that can take the action and what is left of its budget (per lens
// in the review); in `hoom agent`, the first installed provider that carries
// the contract.
type Lugar struct {
	Provider string
	// Puede says whether a provider is an option with ok here: hoom.yaml's
	// provider only wins when it is. nil = any provider can be taken.
	Puede func(provider string) bool
	// BudgetUSD is what is left to spend (nil = no cap): hoom.yaml's budget
	// never goes over it.
	BudgetUSD *float64
}

// Origenes says where each chosen value came from.
type Origenes struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	BudgetUSD string `json:"budget_usd"`
	Timeout   string `json:"timeout"`
}

// Elegidos are the values of one launch. "" / nil = none (the provider's
// own; a nil budget is no cap).
type Elegidos struct {
	Provider string
	// Model is the model the provider receives (proporcion-por-riesgo):
	// a level is already translated with Provider (modelos.Traducir), and
	// it is "" when Provider receives no model or is still "" (ConProvider
	// translates it once the launch picks its provider). A name goes as
	// written.
	Model string
	// Nivel (proporcion-por-riesgo) is the level Model was translated
	// from (modelos.Fuerte, Medio, Barato); "" when the model is a name or
	// there is none. Its origin is Origen.Model.
	Nivel     string
	Effort    string
	BudgetUSD *float64
	Timeout   time.Duration // never 0: the last level is DefaultTimeout
	Origen    Origenes
}

// OrigenRiesgo (proporcion-por-riesgo) is the origin of a value that
// agents.<rol>.<r> of the project's hoom.yaml gave: `hoom.yaml, riesgo <r>`.
func OrigenRiesgo(r string) string {
	return ""
}

// Resolver applies the rule: command, hoom.yaml of the project, the place's
// proposal, the provider. hoom.yaml's budget is capped by what the place has
// left; a budget_usd of 0 (no cap) takes what the place has left, or no cap.
//
// proporcion-por-riesgo: Resolver is ResolverConRiesgo without a riesgo
// (r ""): yaml.PorRiesgo does not count.
func Resolver(p Pedido, yaml manifest.RoleDefaults, l Lugar) Elegidos {
	return Sobre(p, propuesta(yaml, l))
}

// ResolverConRiesgo (proporcion-por-riesgo) applies the rule with the level
// of the riesgo: the command, agents.<rol>.<r> of the project's hoom.yaml
// (yaml.PorRiesgo[r], origin OrigenRiesgo(r)), agents.<rol> (yaml's own
// fields, origin hoom.yaml), the place's proposal, the provider. r is the
// DECLARED riesgo of the launch's card (item.RiesgoDeLanzamiento: alto when
// it cannot be read, normal without card); "" = no level of riesgo. For each
// value the first that exists wins; the provider never comes from the level
// of the riesgo. A budget_usd of the riesgo never goes over what the place
// has left (l.BudgetUSD), like hoom.yaml's. A model that is a level is
// translated with the chosen provider (Elegidos.Model and Nivel).
func ResolverConRiesgo(p Pedido, r string, yaml manifest.RoleDefaults, l Lugar) Elegidos {
	return Elegidos{}
}

// ConProvider (proporcion-por-riesgo) is e on provider p, the one the launch
// takes after the rule (hoom agent takes the first installed provider when
// nothing chose one): Provider becomes p with its origin as it was, and a
// model that came from a level (Nivel) is translated for p
// (modelos.Traducir): "" when p receives no model. A name stays as it is.
func (e Elegidos) ConProvider(p string) Elegidos {
	return Elegidos{}
}

// Linea (proporcion-por-riesgo) is the line `valores:` (CA-503) of a launch
// with e, the ONE text the card verbs, `hoom agent` and the pilot print
// (the pilot with `piloto automatico: ` in front):
// `valores: provider <p> (<o>) · modelo <m> (<o>) · esfuerzo <e> (<o>) ·
// presupuesto <x> (<o>) · tiempo limite <d> (<o>)`, each with its origin,
// as the card verbs print it today. A model that came from a level is
// `modelo <m> (nivel <n>, <origen>)`, and on a provider that receives no
// model `modelo del provider (nivel <n>: <p> no recibe modelo)`. sinTope
// says e.Provider takes no cap in USD and the command asked none: the
// budget is then `presupuesto sin tope (<p> no acepta tope)`.
func Linea(e Elegidos, sinTope bool) string {
	return ""
}

// Sobre puts what the command asked (level 1) over values already chosen by
// the other levels: what the card's action proposes, which applied hoom.yaml
// and the place when the card was derived. Each value the command asks wins,
// with origin comando; the rest keep their value and origin.
//
// proporcion-por-riesgo: a model the command asks that is a level is
// translated with the launch's provider (base's, or the one the command
// asks), and so is a base model that came from a level (base.Nivel) when
// the command asks another provider.
func Sobre(p Pedido, base Elegidos) Elegidos {
	e := base
	e.BudgetUSD = copia(base.BudgetUSD)
	if p.Provider != "" {
		e.Provider, e.Origen.Provider = p.Provider, OrigenComando
	}
	if p.Model != "" {
		e.Model, e.Origen.Model = p.Model, OrigenComando
	}
	if p.Effort != "" {
		e.Effort, e.Origen.Effort = p.Effort, OrigenComando
	}
	if p.BudgetUSD != nil {
		e.BudgetUSD, e.Origen.BudgetUSD = copia(p.BudgetUSD), OrigenComando
	}
	if p.Timeout > 0 {
		e.Timeout, e.Origen.Timeout = p.Timeout, OrigenComando
	}
	return e
}

// propuesta is levels 2 to 4: the project's hoom.yaml, the place's
// proposal, the provider's own.
func propuesta(yaml manifest.RoleDefaults, l Lugar) Elegidos {
	var e Elegidos
	switch {
	case yaml.Provider != "" && (l.Puede == nil || l.Puede(yaml.Provider)):
		e.Provider, e.Origen.Provider = yaml.Provider, OrigenHoomYAML
	case l.Provider != "":
		e.Provider, e.Origen.Provider = l.Provider, OrigenTarjeta
	default:
		e.Origen.Provider = OrigenProvider
	}
	e.Model, e.Origen.Model = pick(yaml.Model)
	e.Effort, e.Origen.Effort = pick(yaml.Effort)

	switch {
	case yaml.BudgetUSD != nil && *yaml.BudgetUSD > 0:
		e.BudgetUSD, e.Origen.BudgetUSD = copia(yaml.BudgetUSD), OrigenHoomYAML
		if l.BudgetUSD != nil && *l.BudgetUSD < *yaml.BudgetUSD {
			// never over what the card has left
			e.BudgetUSD, e.Origen.BudgetUSD = copia(l.BudgetUSD), OrigenTarjeta
		}
	case yaml.BudgetUSD != nil: // 0 = no cap: what is left, or none
		if l.BudgetUSD != nil {
			e.BudgetUSD, e.Origen.BudgetUSD = copia(l.BudgetUSD), OrigenTarjeta
		} else {
			e.Origen.BudgetUSD = OrigenHoomYAML
		}
	case l.BudgetUSD != nil:
		e.BudgetUSD, e.Origen.BudgetUSD = copia(l.BudgetUSD), OrigenTarjeta
	default:
		e.Origen.BudgetUSD = OrigenProvider
	}

	if yaml.Timeout > 0 {
		e.Timeout, e.Origen.Timeout = yaml.Timeout, OrigenHoomYAML
	} else {
		e.Timeout, e.Origen.Timeout = DefaultTimeout, OrigenProvider
	}
	return e
}

// pick is the order of a value the place does not propose (model, effort)
// under the command: hoom.yaml, the provider's own ("").
func pick(yaml string) (string, string) {
	if yaml != "" {
		return yaml, OrigenHoomYAML
	}
	return "", OrigenProvider
}

func copia(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// Duracion renders a timeout the way hoom prints it: 45m, 1h30m, 2h, 30s.
func Duracion(d time.Duration) string {
	return runcmd.Duracion(d)
}
