// Package rolescmd implements `hoom roles`: the enforcement matrix. For every
// role and every provider it says what the role may read, write and execute,
// with which mechanism, and how much of that hoom can actually stand behind.
// It is DERIVED — from the roles table, the project's write policy and the
// capabilities each provider declares, through the same functions the
// envelope uses to decide (agentcmd.PolicyFor, ReadOnlyFor, NoExecFor) — so
// the matrix cannot promise what the envelope does not do.
package rolescmd

import (
	"io"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
)

// Categories, in the order the legend prints them.
const (
	Enforced     = "ENFORCED"      // the provider or the filesystem makes anything else impossible
	PostVerified = "POST-VERIFIED" // nothing prevents it; the scope gate sees it after the run
	BestEffort   = "BEST-EFFORT"   // only the contract asks; nothing prevents it, no gate sees it
	Unsupported  = "UNSUPPORTED"   // the provider cannot carry the role
)

// Categories lists the four, in legend order.
var Categories = []string{Enforced, PostVerified, BestEffort, Unsupported}

// Mechanisms, by name.
const (
	MechNone           = "sin restriccion"
	MechToolDeny       = "deny de tools"
	MechToolDenyShell  = "deny de tools (sin Edit/Write; el shell puede escribir)"
	MechSandbox        = "sandbox"
	MechGate           = "gate post-run"
	MechGateIsolation  = "gate post-run (aislamiento)"
	MechBlind          = "arbol ciego"
	MechQuarantine     = "arbol ciego (cuarentena)"
	MechContract       = "contrato"
	MechNoSystemPrompt = "sin system_prompt"
)

// Cell is one dimension (read, write or exec) of one role on one provider.
type Cell struct {
	Allows     string   `json:"allows"`
	Mechanisms []string `json:"mechanisms"`
	Category   string   `json:"category"`
	Gap        string   `json:"gap"` // empty in ENFORCED, mandatory otherwise
}

// Row is one role on one provider.
type Row struct {
	Role     string `json:"role"`
	Provider string `json:"provider"`
	Read     Cell   `json:"read"`
	Write    Cell   `json:"write"`
	Exec     Cell   `json:"exec"`
}

// Matrix derives one row per role × provider, in the order given.
func Matrix(m *manifest.Manifest, roles []agents.Role, provs []providers.Provider) []Row {
	return nil
}

// UsageText is the exact usage block of `hoom roles`.
const UsageText = `Uso: hoom roles [--role r] [--provider p] [--json]

  --role r       Solo ese rol (writer, test-writer, reviewer, ...)
  --provider p   Solo ese provider (claude|opencode|codex|gemini)
  --json         Emite la matriz como JSON en stdout

Que puede leer, escribir y ejecutar cada rol con cada provider, con que
mecanismo, y en que categoria cae: ENFORCED, POST-VERIFIED, BEST-EFFORT o
UNSUPPORTED. 'hoom roles' no tiene argumentos posicionales.`

// Options mirror the roles verb's flags.
type Options struct {
	Role     string
	Provider string
	JSON     bool
}

// ParseArgs is pure: a request hoom does not understand is a
// *cliargs.UsageError; -h/--help is cliargs.ErrHelp.
func ParseArgs(args []string) (Options, error) {
	return Options{}, nil
}

// Select resolves the filters against the roles table and the provider
// registry. An unknown value is a *cliargs.UsageError listing the valid ones.
func Select(opt Options) ([]agents.Role, []providers.Provider, error) {
	return nil, nil, nil
}

// Render prints the matrix for humans, ending with the legend.
func Render(w io.Writer, rows []Row) {}
