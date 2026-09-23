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
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event is the normalized unit of a run's narration. One schema for every
// provider so the UI (and the theater) is written once.
// Kind: start | text | tool | agent | system | end | error.
//
// `system` means ONE thing: the CLI talking about ITSELF — hooks, compaction,
// rate limits — and not the agent working. It exists so that plumbing is
// filed as plumbing instead of landing in the log as raw JSON inside a text
// event, indistinguishable from what the agent said.
type Event struct {
	TS     time.Time `json:"ts"`
	Kind   string    `json:"kind"`
	Agent  string    `json:"agent,omitempty"`
	Detail string    `json:"detail,omitempty"`
	// SessionID travels only on start/end/error events, when the provider
	// reports its own session id; the run keeps the last non-empty one.
	SessionID string `json:"session_id,omitempty"`
	// Usage travels on the closing events, when the provider measures what
	// the invocation spent. The run ACCUMULATES it (see runcmd): every CLI
	// verified reports per invocation, never per session.
	Usage *Usage `json:"usage,omitempty"`
	// ToolID pairs a delegation (`agent`) with its end (`agent_end`): the id
	// the provider gave the tool call.
	ToolID string `json:"tool_id,omitempty"`
}

// Correlating is implemented by an adapter whose events need memory of the
// run: pairing a delegation with its result. runcmd asks for one normalizer
// per run; Normalize stays the stateless fallback.
type Correlating interface {
	NewNormalizer() func(line string) []Event
}

// Usage is what one invocation cost, in the only units every CLI can be read
// in. CostUSD is a POINTER on purpose: "spent 0" and "does not report cost"
// are different facts, and Codex — which reports tokens and no price at all —
// makes the second one the normal case, not the edge.
type Usage struct {
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	Turns        int      `json:"turns,omitempty"`
	InputTokens  int      `json:"input_tokens,omitempty"`
	CachedTokens int      `json:"cached_input_tokens,omitempty"`
	OutputTokens int      `json:"output_tokens,omitempty"`
	DurationMS   int      `json:"duration_ms,omitempty"`
}

// Empty reports a usage that measured nothing: an adapter attaches it only
// when the provider actually said something.
func (u *Usage) Empty() bool {
	return u == nil || (u.CostUSD == nil && u.Turns == 0 &&
		u.InputTokens == 0 && u.CachedTokens == 0 && u.OutputTokens == 0 && u.DurationMS == 0)
}

// Summary renders usage for humans, in Spanish and in one line. What the
// provider did not report is said out loud ("sin dato de costo"), never
// printed as a zero.
func (u *Usage) Summary() string {
	if u.Empty() {
		return "sin datos de uso"
	}
	var parts []string
	if u.CostUSD != nil {
		parts = append(parts, fmt.Sprintf("costo %s USD", trimFloat(*u.CostUSD)))
	} else {
		parts = append(parts, "sin dato de costo")
	}
	if u.Turns > 0 {
		parts = append(parts, plural(u.Turns, "turno", "turnos"))
	}
	if tok := u.InputTokens + u.OutputTokens; tok > 0 {
		entry := fmt.Sprintf("%s tokens", compactInt(tok))
		if u.CachedTokens > 0 {
			entry += fmt.Sprintf(" (%s de cache)", compactInt(u.CachedTokens))
		}
		parts = append(parts, entry)
	}
	if u.DurationMS > 0 {
		parts = append(parts, (time.Duration(u.DurationMS) * time.Millisecond).Round(time.Millisecond).String())
	}
	return strings.Join(parts, " · ")
}

// trimFloat prints a cost without scientific notation and without a tail of
// meaningless decimals: 0.111583 -> "0.1116", 2 -> "2".
func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// compactInt shortens token counts the way a human reads them: 17408 -> 17.4k.
func compactInt(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return strings.TrimSuffix(strconv.FormatFloat(float64(n)/1000, 'f', 1, 64), ".0") + "k"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
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
	Tools        bool `json:"tools"`         // allow/deny tools by NAME
	ReadOnly     bool `json:"read_only"`     // can impose a role that does NOT write
	NoExec       bool `json:"no_exec"`       // can take the shell away from a role that WRITES
	Unattended   bool `json:"unattended"`    // can run with nobody answering prompts
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
		{"tools", c.Tools}, {"read_only", c.ReadOnly}, {"no_exec", c.NoExec}, {"unattended", c.Unattended},
		{"max_turns", c.MaxTurns}, {"budget", c.Budget},
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
	// ReadOnly is the role's limit as an INTENTION, not as one CLI's
	// vocabulary: the role does not write, and each adapter imposes it with
	// whatever it has (Claude denies tools by name, Codex sets a sandbox).
	ReadOnly bool
	Exec     bool // ...but it DOES run commands (hoom finding, tests). Alone it means nothing.
	// NoExec: the role writes but runs NO commands (the spec authors). The
	// sibling of Exec for a role that writes; under ReadOnly it says nothing,
	// because there the absence of Exec already says it.
	NoExec bool
	// Unattended: nobody will answer a permission prompt. The adapter grants
	// up front the tools the role needs (the read set under ReadOnly, read +
	// write + shell otherwise) instead of letting the CLI deny them one by
	// one in silence, which is what a headless CLI does by default.
	Unattended bool
	MaxTurns   int     // 0 = unbounded
	BudgetUSD  float64 // 0 = unbounded
	Strict     bool    // unsupported field = error instead of Ignored
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
	FieldTools        = "tools"     // covers AllowTools and DenyTools
	FieldReadOnly     = "read_only" // covers ReadOnly and Exec
	FieldNoExec       = "no_exec"
	FieldUnattended   = "unattended"
	FieldMaxTurns     = "max_turns"
	FieldBudget       = "budget"
)

// ErrUnsupported is what Command returns under Strict when the provider
// cannot honor one or more request fields. Value type: errors.As works
// with a value target.
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
	readOnly     bool
	exec         bool
	noExec       bool
	unattended   bool
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
	if req.ReadOnly {
		if caps.ReadOnly {
			p.readOnly, p.exec = true, req.Exec
		} else {
			p.ignored = append(p.ignored, FieldReadOnly)
		}
	}
	if req.NoExec && !req.ReadOnly {
		if caps.NoExec {
			p.noExec = true
		} else {
			p.ignored = append(p.ignored, FieldNoExec)
		}
	}
	if req.Unattended {
		if caps.Unattended {
			p.unattended = true
		} else {
			p.ignored = append(p.ignored, FieldUnattended)
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

// dedup keeps the first appearance of every entry: merging the role's tool
// vocabulary with the caller's must not repeat names nor reorder them.
func dedup(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
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
	// MinBudgetUSD is the least budget a run of this provider can do useful
	// work with (0 = no minimum: the provider takes no cap).
	MinBudgetUSD float64 `json:"min_budget_usd"`
}

// BudgetFloor is implemented by a provider that takes a USD cap and declares
// the least one worth launching with: under it the CLI stops before
// delivering anything and the money is lost.
type BudgetFloor interface {
	MinBudgetUSD() float64
}

// MinBudgetUSD is p's declared minimum budget, or 0 when it declares none.
func MinBudgetUSD(p Provider) float64 {
	if f, ok := p.(BudgetFloor); ok {
		return f.MinBudgetUSD()
	}
	return 0
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
		info := Info{Name: p.Name(), Capabilities: p.Capabilities(), MinBudgetUSD: MinBudgetUSD(p)}
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
