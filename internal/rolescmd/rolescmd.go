// Package rolescmd implements `hoom roles`: the enforcement matrix. For every
// role and every provider it says what the role may read, write and execute,
// with which mechanism, and how much of that hoom can actually stand behind.
// It is DERIVED — from the roles table, the project's write policy and the
// capabilities each provider declares, through the same functions the
// envelope uses to decide (agentcmd.PolicyFor, ReadOnlyFor, NoExecFor) — so
// the matrix cannot promise what the envelope does not do.
package rolescmd

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/cliargs"
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
	rows := make([]Row, 0, len(roles)*len(provs))
	for _, r := range roles {
		for _, p := range provs {
			rows = append(rows, row(m, r, p))
		}
	}
	return rows
}

// Gaps, one sentence each: what the category leaves uncovered.
const (
	gapPost    = "nada lo impide durante el run: el gate de scope lo ve despues (hallazgo high o corte)"
	gapNoShell = "el provider no puede quitarle el shell: nada lo impide y ningun gate ve lo que ejecuto"
	gapGitShow = "el object DB de git es compartido: 'git show HEAD:<ruta>' lee la implementacion"
	floor      = "nunca el piso: aprobaciones, hoom.yaml, evidencia existente, items, reviews"
)

func row(m *manifest.Manifest, r agents.Role, p providers.Provider) Row {
	out := Row{Role: r.Slug, Provider: p.Name()}
	caps := p.Capabilities()
	if !caps.SystemPrompt {
		// 'hoom agent' and 'hoom review' refuse it before any run
		cell := Cell{Allows: "no corre el rol", Mechanisms: []string{MechNoSystemPrompt}, Category: Unsupported,
			Gap: p.Name() + " no soporta system_prompt: 'hoom agent' no puede darle el contrato del rol"}
		out.Read, out.Write, out.Exec = cell, cell, cell
		return out
	}
	readOnly, exec, _ := agentcmd.ReadOnlyFor(p, r)
	noExec, _ := agentcmd.NoExecFor(p, r)
	// how THIS provider imposes read-only: by tool name if it names tools,
	// with its sandbox otherwise (CA-297 pins it to the adapter's argv)
	byTools := caps.Tools
	shellGone := (readOnly && !exec && byTools) || noExec

	// read
	switch {
	case r.Isolated && shellGone:
		out.Read = Cell{Allows: "el spec y los tests, sin la implementacion en disco",
			Mechanisms: []string{MechBlind, MechGateIsolation}, Category: Enforced}
	case r.Isolated:
		out.Read = Cell{Allows: "el spec y los tests, sin la implementacion en disco",
			Mechanisms: []string{MechBlind, MechGateIsolation}, Category: BestEffort, Gap: gapGitShow}
	default:
		out.Read = Cell{Allows: "todo el arbol", Mechanisms: []string{MechNone}, Category: Enforced}
	}

	// write
	pol := agentcmd.PolicyFor(m, r)
	allows := strings.Join(pol.Allow, ", ")
	if len(pol.Deny) > 0 {
		allows += " menos " + strings.Join(pol.Deny, ", ")
	}
	allows += "; " + floor
	switch {
	case readOnly && !exec:
		mech := MechSandbox
		if byTools {
			mech = MechToolDeny
		}
		out.Write = Cell{Allows: "nada", Mechanisms: []string{mech, MechGate}, Category: Enforced}
	case readOnly && byTools:
		out.Write = Cell{Allows: allows, Mechanisms: []string{MechToolDenyShell, MechGate}, Category: PostVerified, Gap: gapPost}
	case r.Isolated:
		out.Write = Cell{Allows: allows, Mechanisms: []string{MechQuarantine, MechGate}, Category: PostVerified, Gap: gapPost}
	default:
		out.Write = Cell{Allows: allows, Mechanisms: []string{MechGate}, Category: PostVerified, Gap: gapPost}
	}

	// exec
	switch {
	case r.Exec:
		out.Exec = Cell{Allows: "comandos", Mechanisms: []string{MechNone}, Category: Enforced}
	case shellGone:
		out.Exec = Cell{Allows: "ningun comando", Mechanisms: []string{MechToolDeny}, Category: Enforced}
	default:
		// a read-only sandbox still runs commands, and a sandbox cannot take
		// the shell from a role that writes: only the contract asks
		out.Exec = Cell{Allows: "ningun comando (lo pide el contrato)", Mechanisms: []string{MechContract},
			Category: BestEffort, Gap: gapNoShell}
	}
	return out
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
	fs := flag.NewFlagSet("roles", flag.ContinueOnError)
	role := fs.String("role", "", "solo ese rol")
	provider := fs.String("provider", "", "solo ese provider")
	asJSON := fs.Bool("json", false, "emitir la matriz como JSON en stdout")
	if err := cliargs.Strict(fs, args, "roles", UsageText); err != nil {
		return Options{}, err
	}
	if vacio := cliargs.FirstEmptyValue(fs, args); vacio != "" {
		return Options{}, cliargs.NewUsageError("roles", "--"+vacio+" vacio: escribir un flag y no darle valor es un typo", UsageText)
	}
	return Options{Role: *role, Provider: *provider, JSON: *asJSON}, nil
}

// Select resolves the filters against the roles table and the provider
// registry. An unknown value is a *cliargs.UsageError listing the valid ones.
func Select(opt Options) ([]agents.Role, []providers.Provider, error) {
	roles, provs := agents.Roles(), providers.All()
	if want := strings.TrimSpace(opt.Role); want != "" {
		r, err := agents.Lookup(want)
		if err != nil {
			var slugs []string
			for _, r := range roles {
				slugs = append(slugs, r.Slug)
			}
			return nil, nil, cliargs.NewUsageError("roles", fmt.Sprintf("rol desconocido %q (validos: %s)", want, strings.Join(slugs, ", ")), UsageText)
		}
		roles = []agents.Role{r}
	}
	if want := strings.TrimSpace(opt.Provider); want != "" {
		p, err := providers.Lookup(want)
		if err != nil {
			var names []string
			for _, p := range provs {
				names = append(names, p.Name())
			}
			return nil, nil, cliargs.NewUsageError("roles", fmt.Sprintf("provider desconocido %q (validos: %s)", want, strings.Join(names, ", ")), UsageText)
		}
		provs = []providers.Provider{p}
	}
	return roles, provs, nil
}

// Render prints the matrix for humans, ending with the legend.
func Render(w io.Writer, rows []Row) {
	fmt.Fprintln(w, "hoom roles: que puede cada rol con cada provider")
	fmt.Fprintln(w, "  (derivado de la tabla de roles, la politica de escritura de hoom.yaml y las capacidades de cada provider)")
	for _, r := range rows {
		fmt.Fprintf(w, "\n%s · %s\n", r.Role, r.Provider)
		for _, c := range []struct {
			name string
			cell Cell
		}{{"lee", r.Read}, {"escribe", r.Write}, {"ejecuta", r.Exec}} {
			fmt.Fprintf(w, "  %-8s %-14s %s\n", c.name, c.cell.Category, strings.Join(c.cell.Mechanisms, " + "))
			fmt.Fprintf(w, "  %-8s %s\n", "", c.cell.Allows)
			if c.cell.Gap != "" {
				fmt.Fprintf(w, "  %-8s hueco: %s\n", "", c.cell.Gap)
			}
		}
	}
	fmt.Fprintln(w, "\ncategorias:")
	fmt.Fprintf(w, "  %-14s el provider o el sistema de archivos impide cualquier otra cosa\n", Enforced)
	fmt.Fprintf(w, "  %-14s nada lo impide durante el run; el gate de scope lo ve despues\n", PostVerified)
	fmt.Fprintf(w, "  %-14s solo lo pide el contrato; nada lo impide y ningun gate lo ve\n", BestEffort)
	fmt.Fprintf(w, "  %-14s el provider no puede llevar el rol (sin system_prompt)\n", Unsupported)
}
