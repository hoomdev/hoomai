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
	panic("sin implementar")
}

// Duracion renders a timeout the way hoom prints it: 45m, 1h30m, 2h, 30s.
func Duracion(d time.Duration) string {
	panic("sin implementar")
}
