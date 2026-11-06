// Package valores is the ONE rule that decides the launch values of a role
// (spec verbos-de-la-tarjeta): provider, model, effort, budget and timeout.
// For each value the first that exists wins: what the command (or the
// Studio's dialog) asked, then the project's hoom.yaml (manifest.RoleDefaults),
// then what the place proposes today (the card, or `hoom agent`), then the
// provider's own (and, for the timeout, hoom's 30 minutes). Every value comes
// out with where it came from: no value is chosen in silence.
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
	Provider  string
	Model     string
	Effort    string
	BudgetUSD string
	Timeout   string
}

// Elegidos are the values of one launch. "" / nil = none (the provider's
// own; a nil budget is no cap).
type Elegidos struct {
	Provider  string
	Model     string
	Effort    string
	BudgetUSD *float64
	Timeout   time.Duration // never 0: the last level is DefaultTimeout
	Origen    Origenes
}

// Resolver applies the rule: command, hoom.yaml of the project, the place's
// proposal, the provider. hoom.yaml's budget is capped by what the place has
// left; a budget_usd of 0 (no cap) takes what the place has left, or no cap.
func Resolver(p Pedido, yaml manifest.RoleDefaults, l Lugar) Elegidos {
	var e Elegidos
	switch {
	case p.Provider != "":
		e.Provider, e.Origen.Provider = p.Provider, OrigenComando
	case yaml.Provider != "" && (l.Puede == nil || l.Puede(yaml.Provider)):
		e.Provider, e.Origen.Provider = yaml.Provider, OrigenHoomYAML
	case l.Provider != "":
		e.Provider, e.Origen.Provider = l.Provider, OrigenTarjeta
	default:
		e.Origen.Provider = OrigenProvider
	}
	e.Model, e.Origen.Model = pick(p.Model, yaml.Model)
	e.Effort, e.Origen.Effort = pick(p.Effort, yaml.Effort)

	switch {
	case p.BudgetUSD != nil:
		e.BudgetUSD, e.Origen.BudgetUSD = copia(p.BudgetUSD), OrigenComando
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

	switch {
	case p.Timeout > 0:
		e.Timeout, e.Origen.Timeout = p.Timeout, OrigenComando
	case yaml.Timeout > 0:
		e.Timeout, e.Origen.Timeout = yaml.Timeout, OrigenHoomYAML
	default:
		e.Timeout, e.Origen.Timeout = DefaultTimeout, OrigenProvider
	}
	return e
}

// pick is the order of a value the place does not propose (model, effort):
// the command, hoom.yaml, the provider's own ("").
func pick(cmd, yaml string) (string, string) {
	switch {
	case cmd != "":
		return cmd, OrigenComando
	case yaml != "":
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
