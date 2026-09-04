// Package providers knows the supported AI CLIs and how to drive them
// headless. hoomAI never talks to a model API: it executes the USER'S own
// CLI as a subprocess — same auth, same config, same subagents as when the
// user types it in a terminal. Every CLI is an adapter behind one Provider
// interface: it TRANSLATES a Request into an Invocation and PARSES stdout
// lines into normalized events; it never executes anything (runcmd is the
// single executor). What a CLI cannot honor is declared in Capabilities and
// reported per request as Ignored — declared degradation, never silent —
// or refused outright when the caller asks for Strict.
package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Event is the normalized unit of a run's narration. One schema for every
// provider so the UI (and the theater) is written once.
// Kind: start | text | tool | agent | end | error.
type Event struct {
	TS     time.Time `json:"ts"`
	Kind   string    `json:"kind"`
	Agent  string    `json:"agent,omitempty"`
	Detail string    `json:"detail,omitempty"`
	// SessionID travels only on start/end/error events, when the provider
	// reports its own session id; the run keeps the last non-empty one.
	SessionID string `json:"session_id,omitempty"`
}

// Capabilities declares what a provider can honor natively. A false flag
// never errors by itself: the field is dropped and reported in Ignored.
type Capabilities struct {
	Structured   bool `json:"structured"`    // stdout parseable line by line
	Continue     bool `json:"continue"`      // continue the LAST session of the directory
	Resume       bool `json:"resume"`        // resume ONE session by id
	SessionID    bool `json:"session_id"`    // reports its session id in the stream
	Model        bool `json:"model"`         // model selection
	SystemPrompt bool `json:"system_prompt"` // appends text to its own system prompt
	Tools        bool `json:"tools"`         // allow/deny tools by name
	MaxTurns     bool `json:"max_turns"`     // hard cap on agentic turns
	Budget       bool `json:"budget"`        // hard cap on spend (USD)
}

// Names lists the supported capabilities by their JSON name, in stable
// order — the vocabulary `hoom providers` prints.
func (c Capabilities) Names() []string {
	var out []string
	for _, f := range []struct {
		name string
		on   bool
	}{
		{"structured", c.Structured}, {"continue", c.Continue}, {"resume", c.Resume},
		{"session_id", c.SessionID}, {"model", c.Model}, {"system_prompt", c.SystemPrompt},
		{"tools", c.Tools}, {"max_turns", c.MaxTurns}, {"budget", c.Budget},
	} {
		if f.on {
			out = append(out, f.name)
		}
	}
	return out
}

// Summary renders the capability list for humans; no capability at all is
// said out loud instead of printing nothing.
func (c Capabilities) Summary() string {
	names := c.Names()
	if len(names) == 0 {
		return "texto plano (sin sesion ni stream)"
	}
	return strings.Join(names, ", ")
}

// Request is what the caller wants, in hoom's vocabulary. Translating it
// to provider-specific flags is the adapter's job.
type Request struct {
	Prompt       string
	ResumeID     string // provider session id to resume; "" = none
	Continue     bool   // continue the last session of the directory (weaker than ResumeID)
	Model        string
	SystemPrompt string   // APPENDED to the provider's own system prompt, never replaces it
	AllowTools   []string // provider tool names/patterns, verbatim
	DenyTools    []string
	MaxTurns     int     // 0 = unbounded
	BudgetUSD    float64 // 0 = unbounded
	Strict       bool    // unsupported field = error instead of Ignored
}

// Invocation is the materialized headless command. Ignored lists the
// request fields the provider could not honor, by canonical name.
type Invocation struct {
	Bin     string
	Args    []string
	Ignored []string
}

// Provider is one AI CLI as hoom sees it: a translator and a parser.
type Provider interface {
	Name() string
	Bin() string
	Capabilities() Capabilities
	Command(req Request) (Invocation, error) // translates; NEVER executes
	Normalize(line string) []Event           // one stdout line -> events
}

// Canonical field names used in Invocation.Ignored and ErrUnsupported.
const (
	FieldResume       = "resume"
	FieldContinue     = "continue"
	FieldModel        = "model"
	FieldSystemPrompt = "system_prompt"
	FieldTools        = "tools" // covers AllowTools and DenyTools
	FieldMaxTurns     = "max_turns"
	FieldBudget       = "budget"
)

// ErrUnsupported is what Command returns under Strict when the provider
// cannot honor one or more request fields. Value type: errors.As works
// with a value target.
// ToolNamer is implemented by the providers that NAME their tools. It is an
// optional interface, resolved with a type assertion: the Provider contract
// stays exactly as the providers-v2 spec fixed it, and the vocabulary stays
// in the adapter that actually knows it. `exec` marks a read-only role that
// still runs commands (hoom finding, tests).
type ToolNamer interface {
	ReadOnlyTools(exec bool) (allow, deny []string)
}

type ErrUnsupported struct {
	Provider string
	Fields   []string
}

func (e ErrUnsupported) Error() string {
	return fmt.Sprintf("el provider %q no soporta: %s (mira 'hoom providers')", e.Provider, strings.Join(e.Fields, ", "))
}

// plan is a request after the common rules: validated, tools cleaned,
// session precedence resolved and unsupported fields collected. Adapters
// build argv ONLY from what the plan kept, so every CLI obeys the same
// contract without repeating it.
type plan struct {
	prompt       string
	resumeID     string // "" = not used
	cont         bool
	model        string
	systemPrompt string
	allow, deny  []string
	maxTurns     int
	budgetUSD    float64
	ignored      []string
}

// resolve applies the common Command rules against a provider's
// capabilities. Ignored comes out in canonical order.
func resolve(name string, caps Capabilities, req Request) (plan, error) {
	var p plan
	if strings.TrimSpace(req.Prompt) == "" {
		return p, fmt.Errorf("prompt vacio")
	}
	if strings.HasPrefix(req.Prompt, "-") {
		return p, fmt.Errorf("el prompt no puede empezar con '-' (la CLI lo leeria como flag)")
	}
	resumeID := strings.TrimSpace(req.ResumeID)
	if strings.HasPrefix(resumeID, "-") {
		return p, fmt.Errorf("id de sesion invalido %q: no puede empezar con '-'", resumeID)
	}
	if req.MaxTurns < 0 {
		return p, fmt.Errorf("max_turns negativo (%d)", req.MaxTurns)
	}
	if req.BudgetUSD < 0 {
		return p, fmt.Errorf("presupuesto negativo (%v)", req.BudgetUSD)
	}
	p.prompt = req.Prompt

	// session: the strongest supported mechanism wins; a weaker one that
	// was also requested is superseded (not ignored); an unsupported one
	// is ignored.
	switch {
	case resumeID != "" && caps.Resume:
		p.resumeID = resumeID
	case resumeID != "":
		p.ignored = append(p.ignored, FieldResume)
		if req.Continue {
			if caps.Continue {
				p.cont = true
			} else {
				p.ignored = append(p.ignored, FieldContinue)
			}
		}
	case req.Continue:
		if caps.Continue {
			p.cont = true
		} else {
			p.ignored = append(p.ignored, FieldContinue)
		}
	}
	if m := strings.TrimSpace(req.Model); m != "" {
		if caps.Model {
			p.model = m
		} else {
			p.ignored = append(p.ignored, FieldModel)
		}
	}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		if caps.SystemPrompt {
			p.systemPrompt = req.SystemPrompt
		} else {
			p.ignored = append(p.ignored, FieldSystemPrompt)
		}
	}
	allow, deny := cleanTools(req.AllowTools), cleanTools(req.DenyTools)
	if len(allow) > 0 || len(deny) > 0 {
		if caps.Tools {
			p.allow, p.deny = allow, deny
		} else {
			p.ignored = append(p.ignored, FieldTools)
		}
	}
	if req.MaxTurns > 0 {
		if caps.MaxTurns {
			p.maxTurns = req.MaxTurns
		} else {
			p.ignored = append(p.ignored, FieldMaxTurns)
		}
	}
	if req.BudgetUSD > 0 {
		if caps.Budget {
			p.budgetUSD = req.BudgetUSD
		} else {
			p.ignored = append(p.ignored, FieldBudget)
		}
	}
	if req.Strict && len(p.ignored) > 0 {
		return plan{}, ErrUnsupported{Provider: name, Fields: append([]string(nil), p.ignored...)}
	}
	return p, nil
}

// cleanTools trims entries and drops the empty ones; nil when nothing
// survives, so an all-empty list counts as "not sent".
func cleanTools(in []string) []string {
	var out []string
	for _, t := range in {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// textEvents is the normalization of a provider without structured
// output: the line as-is, never lost, never interpreted.
func textEvents(line string) []Event {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return nil
	}
	return []Event{{TS: time.Now().UTC(), Kind: "text", Detail: line}}
}

// Info is a provider's availability on this machine plus what it can do.
type Info struct {
	Name         string       `json:"name"`
	Installed    bool         `json:"installed"`
	Bin          string       `json:"bin,omitempty"`
	Capabilities Capabilities `json:"capabilities"`
}

// Registry holds providers in insertion order — the order every listing
// shows — and refuses duplicates. Default carries the built-ins; tests
// build their own with NewRegistry.
type Registry struct {
	mu     sync.Mutex
	order  []Provider
	byName map[string]Provider
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]Provider{}}
}

// Register adds a provider. Empty name or duplicate name is an error.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("provider nulo")
	}
	name := strings.TrimSpace(p.Name())
	if name == "" {
		return fmt.Errorf("provider sin nombre")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("provider duplicado %q", name)
	}
	r.byName[name] = p
	r.order = append(r.order, p)
	return nil
}

func (r *Registry) names() []string {
	out := make([]string, 0, len(r.order))
	for _, p := range r.order {
		out = append(out, p.Name())
	}
	return out
}

// Lookup returns the provider registered under name.
func (r *Registry) Lookup(name string) (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.byName[strings.TrimSpace(name)]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("provider desconocido %q (soportados: %s)", name, strings.Join(r.names(), ", "))
}

// All returns every provider in registration order.
func (r *Registry) All() []Provider {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Provider(nil), r.order...)
}

// Detect reports every provider, whether its binary is in PATH right now
// and its declared capabilities, in registration order. It never claims
// more than PATH proves (no authentication guesses).
func (r *Registry) Detect() []Info {
	all := r.All()
	out := make([]Info, 0, len(all))
	for _, p := range all {
		info := Info{Name: p.Name(), Capabilities: p.Capabilities()}
		if path, err := exec.LookPath(p.Bin()); err == nil {
			info.Installed = true
			info.Bin = path
		}
		out = append(out, info)
	}
	return out
}

// Default is the registry of built-in providers, populated at init in the
// historic order: claude, opencode, codex, gemini.
var Default = NewRegistry()

func init() {
	for _, p := range []Provider{claude{}, opencode{}, codex{}, gemini{}} {
		if err := Default.Register(p); err != nil {
			panic("providers: " + err.Error()) // a binary bug, not a project state
		}
	}
}

// Lookup returns a built-in provider by name.
func Lookup(name string) (Provider, error) { return Default.Lookup(name) }

// All returns the built-in providers in order.
func All() []Provider { return Default.All() }

// Detect reports every built-in provider's availability and capabilities.
func Detect() []Info { return Default.Detect() }

// JSONBytes renders Detect exactly as both the CLI and the Studio emit it.
func JSONBytes() ([]byte, error) {
	return json.MarshalIndent(Detect(), "", "  ")
}

// RenderText prints the human listing of `hoom providers`: availability by
// PATH and, under each provider, the capabilities it declares.
func RenderText(w io.Writer, infos []Info) {
	fmt.Fprintln(w, "hoom: providers de IA soportados (deteccion por PATH)")
	for _, p := range infos {
		state := "NO instalado"
		if p.Installed {
			state = "instalado (" + p.Bin + ")"
		}
		fmt.Fprintf(w, "  %-10s %s\n", p.Name, state)
		fmt.Fprintf(w, "  %-10s capacidades: %s\n", "", p.Capabilities.Summary())
	}
}
