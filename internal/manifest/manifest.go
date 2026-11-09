// Package manifest defines and loads hoom.yaml: the per-project contract
// that maps abstract gate capabilities to concrete stack commands.
// The hoomAI core never knows what Laravel, Kotlin or Go are; it only
// executes what the manifest declares and evaluates the results.
package manifest

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/providers"
)

const (
	Schema       = "hoom/v1"
	FileName     = "hoom.yaml"
	DefaultBase  = "main"
	PolicyStrict = "strict"
)

// Gate is one verifiable capability (test, static, lint, arch, mutation,
// build, ...). Required, Cmd and DiffCmd are pointers so a project override
// can distinguish "not specified" (inherit from profile) from an explicit
// value — in particular `cmd: ""` explicitly declares the gate ABSENT
// (even when the profile ships a diff_cmd), and `diff_cmd: ""` explicitly
// clears the profile's diff command.
type Gate struct {
	Required *bool   `yaml:"required"`
	Cmd      *string `yaml:"cmd"`
	DiffCmd  *string `yaml:"diff_cmd,omitempty"`
	Notes    string  `yaml:"notes,omitempty"`
	// Partial declares known partial coverage (e.g. Pitest only mutates the
	// JVM target in KMP). Declared degradation is a feature: reported loudly,
	// never hidden.
	Partial string `yaml:"partial,omitempty"`
}

// IsRequired resolves the pointer (nil => false).
func (g Gate) IsRequired() bool { return g.Required != nil && *g.Required }

// CmdStr resolves the pointer (nil => "" => absent).
func (g Gate) CmdStr() string {
	if g.Cmd == nil {
		return ""
	}
	return *g.Cmd
}

// DiffCmdStr resolves the pointer (nil => "" => no diff scoping).
func (g Gate) DiffCmdStr() string {
	if g.DiffCmd == nil {
		return ""
	}
	return *g.DiffCmd
}

// AgentPolicy is one role's project-level policy: where it may write, and
// the launch values the project chooses for it (verbos-de-la-tarjeta). Every
// launch value is optional; model and effort are the provider's vocabulary
// and are never validated.
type AgentPolicy struct {
	Provider  string        `yaml:"provider,omitempty"`
	Model     string        `yaml:"model,omitempty"`
	Effort    string        `yaml:"effort,omitempty"`
	BudgetUSD *float64      `yaml:"budget_usd,omitempty"` // nil = not declared; 0 = no cap
	Timeout   time.Duration `yaml:"-"`                    // agents.<rol>.timeout; 0 = not declared
	Write     WriteScope    `yaml:"write,omitempty"`
}

// AgentKeys are the keys agents.<rol> accepts, in the order the error names
// them.
var AgentKeys = []string{"provider", "model", "effort", "budget_usd", "timeout", "write"}

// MinTimeout and MaxTimeout bound every timeout hoom.yaml or a flag declares:
// under a minute no CLI gets to start, over 12 hours is a forgotten run.
const (
	MinTimeout = time.Minute
	MaxTimeout = 12 * time.Hour
)

// ParseTimeout reads a timeout as hoom.yaml and the flags write it: a Go
// duration ("45m", "1h30m") between MinTimeout and MaxTimeout. campo names
// where it was written ("agents.writer.timeout", "--timeout"); the error is
// `<campo>: "<v>" no es una duracion entre 1m y 12h (ej. 45m, 1h30m)`.
func ParseTimeout(campo, v string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d < MinTimeout || d > MaxTimeout {
		return 0, fmt.Errorf("%s: %q no es una duracion entre 1m y 12h (ej. 45m, 1h30m)", campo, v)
	}
	return d, nil
}

// ModelEntry is one entry of models.<provider> in hoom.yaml: a model the
// project adds to hoom's catalog (modelos.Propio), with an optional
// description that becomes its line in the Modelo selector.
type ModelEntry struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// ModelKeys are the keys an entry of models.<provider> accepts, in the
// order the error names them.
var ModelKeys = []string{"name", "description"}

// ModelCatalog is the models: section of hoom.yaml (tablero-con-paneles),
// keyed by provider, each with its entries in the file's order:
//
//	models:
//	  codex:
//	    - name: gpt-7-sol
//	      description: el nuevo de todos los dias
//
// It decodes itself so every error names where it is, and Load (and
// LoadWithoutBaseCheck) refuse the file with "hoom.yaml invalido: " and the
// error. The texts, with <p> the provider as written, <i> the entry's
// index in its list counting from 0, and <k> the key:
//
//   - a provider that is not in hoom's registry:
//     `models: provider desconocido "<p>" (validos: claude, codex)`, the
//     valid ones being modelos.ConModelo joined by ", ";
//   - a provider of hoom's registry that takes no model (gemini, opencode):
//     `models.<p>: hoom no le pasa el modelo a <p>`;
//   - an entry with a key other than name and description:
//     `models.<p>[<i>]: clave desconocida "<k>" (validas: name, description)`;
//   - an entry without name, or whose name is empty or only spaces:
//     `models.<p>[<i>]: falta name`;
//   - a name that starts with "-" (after trimming its spaces):
//     `models.<p>[<i>]: el nombre no puede empezar con '-'`.
//
// Shapes the spec does not name are refused too: models: that is not a
// map is "models debe ser un mapa por provider (ej. models: { codex: [ {
// name: gpt-7-sol } ] })"; models.<p> that is not a list is
// "models.<p> debe ser una lista de modelos (ej. - name: gpt-7-sol)"; an
// entry that is not a map is "models.<p>[<i>] debe ser un mapa con name y
// description"; a provider written twice is `models: el provider "<p>"
// esta repetido (linea <n>)`. The checks go in the file's order, provider
// by provider and entry by entry, and within an entry its keys first, then
// name: the first error found is the one given. Name and description are
// kept trimmed. A models: that is empty or null declares nothing (nil).
type ModelCatalog map[string][]ModelEntry

// UnmarshalYAML decodes models: with the rules of ModelCatalog.
func (c *ModelCatalog) UnmarshalYAML(n *yaml.Node) error {
	return nil
}

// AgentPolicies are the agents: section, keyed by role slug. It decodes
// itself so every error names the role: a key hoom does not know is an
// error, never a silence (an ignored `efort: low` is the very failure the
// launch values must not have).
type AgentPolicies map[string]AgentPolicy

// UnmarshalYAML decodes agents: role by role, with the contract's rules.
func (a *AgentPolicies) UnmarshalYAML(n *yaml.Node) error {
	if esNulo(n) {
		*a = nil
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("agents debe ser un mapa por rol (ej. agents: { writer: { effort: high } })")
	}
	out := AgentPolicies{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		role := n.Content[i].Value
		// decoding by hand skips yaml.v3's unique-key check: a second
		// agents.<rol> would silently replace the first (its write: too)
		if _, visto := out[role]; visto {
			return fmt.Errorf("agents: el rol %q esta repetido (linea %d)", role, n.Content[i].Line)
		}
		p, err := decodeAgent(role, n.Content[i+1])
		if err != nil {
			return err
		}
		out[role] = p
	}
	*a = out
	return nil
}

// decodeAgent is one agents.<rol>: its keys, its provider, its budget and
// its timeout, each refused with the role in the message.
func decodeAgent(role string, n *yaml.Node) (AgentPolicy, error) {
	var p AgentPolicy
	if esNulo(n) {
		return p, nil
	}
	if n.Kind != yaml.MappingNode {
		return p, fmt.Errorf("agents.%s debe ser un mapa (ej. agents: { %s: { effort: high } })", role, role)
	}
	var timeout *yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if !containsKey(AgentKeys, key) {
			return p, fmt.Errorf("agents.%s: clave desconocida %q (validas: %s)", role, key, strings.Join(AgentKeys, ", "))
		}
		// one place per role: the reviewer launches from review:
		if role == "reviewer" && key != "write" {
			return p, fmt.Errorf("agents.reviewer: la review se configura en review: (provider, model, effort, timeout)")
		}
		if key == "timeout" {
			timeout = n.Content[i+1]
		}
	}
	var plain struct {
		Provider  string     `yaml:"provider"`
		Model     string     `yaml:"model"`
		Effort    string     `yaml:"effort"`
		BudgetUSD *float64   `yaml:"budget_usd"`
		Write     WriteScope `yaml:"write"`
	}
	if err := n.Decode(&plain); err != nil {
		return p, fmt.Errorf("agents.%s: %v", role, err)
	}
	p = AgentPolicy{Provider: strings.TrimSpace(plain.Provider), Model: strings.TrimSpace(plain.Model),
		Effort: strings.TrimSpace(plain.Effort), BudgetUSD: plain.BudgetUSD, Write: plain.Write}
	if p.Provider != "" && !containsKey(providerNames(), p.Provider) {
		return p, fmt.Errorf("agents.%s.provider: %q no es un provider de hoom (%s)", role, plain.Provider, strings.Join(providerNames(), ", "))
	}
	if b := p.BudgetUSD; b != nil && !(*b >= 0) || b != nil && math.IsInf(*b, 0) {
		return p, fmt.Errorf("agents.%s.budget_usd: %v no es un numero mayor o igual que 0 (0 = sin tope)", role, *b)
	}
	if timeout != nil && !esNulo(timeout) {
		d, err := ParseTimeout("agents."+role+".timeout", timeout.Value)
		if err != nil {
			return p, err
		}
		p.Timeout = d
	}
	return p, nil
}

// providerNames are the names of hoom's registry, sorted: the providers a
// hoom.yaml may name. Whether one is installed is checked at launch.
func providerNames() []string {
	var out []string
	for _, p := range providers.All() {
		out = append(out, p.Name())
	}
	sort.Strings(out)
	return out
}

func esNulo(n *yaml.Node) bool {
	return n == nil || n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

func containsKey(list []string, k string) bool {
	for _, v := range list {
		if v == k {
			return true
		}
	}
	return false
}

// RoleDefaults are the launch values hoom.yaml declares for one role:
// agents.<rol>, or review: for the reviewer. An empty field was not declared.
type RoleDefaults struct {
	Provider  string
	Model     string
	Effort    string
	BudgetUSD *float64      // nil = not declared; 0 = declared without a cap
	Timeout   time.Duration // 0 = not declared
}

// RoleDefaults is the ONE place that says what hoom.yaml declares for a role:
// `hoom agent`, the card verbs and the board ask here. The reviewer's come
// from review: (it has no budget there); every other role's from
// agents.<rol>. A nil manifest declares nothing.
func (m *Manifest) RoleDefaults(role string) RoleDefaults {
	if m == nil {
		return RoleDefaults{}
	}
	if role == "reviewer" {
		r := m.Review
		if r == nil {
			return RoleDefaults{}
		}
		return RoleDefaults{Provider: strings.TrimSpace(r.Provider), Model: strings.TrimSpace(r.Model),
			Effort: strings.TrimSpace(r.Effort), Timeout: r.Timeout}
	}
	p, ok := m.Agents[role]
	if !ok {
		return RoleDefaults{}
	}
	d := RoleDefaults{Provider: p.Provider, Model: p.Model, Effort: p.Effort, Timeout: p.Timeout}
	if p.BudgetUSD != nil {
		v := *p.BudgetUSD
		d.BudgetUSD = &v
	}
	return d
}

// LoadRoles reads only the agents: and review: sections of root/hoom.yaml,
// with the strictness Load applies to them: the values of the PROJECT, which
// the board and the launches read whatever the card's branch says. A root
// without hoom.yaml declares nothing (an empty manifest, no error).
func LoadRoles(root string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(root, FileName))
	if os.IsNotExist(err) {
		return &Manifest{Dir: root}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Agents AgentPolicies `yaml:"agents"`
		Review *ReviewPolicy `yaml:"review"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if err := doc.Review.validate(); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	return &Manifest{Agents: doc.Agents, Review: doc.Review, Dir: root}, nil
}

// WriteScope re-aims where a role may write. A declared Allow REPLACES the
// role's default globs (a project knows where its tests live better than a
// default does); Deny is ADDED to them.
type WriteScope struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// FindingsPolicy is the project's opt-in to let open review findings block
// verify (synthetic gate findings_open). Absent = the gate does not run.
type FindingsPolicy struct {
	// BlockOn is a severity threshold: that severity or worse blocks.
	BlockOn *string `yaml:"block_on"`
}

// Manifest is the parsed hoom.yaml after profile inheritance is resolved.
type Manifest struct {
	Schema     string          `yaml:"schema"`
	Project    string          `yaml:"project"`
	Profile    string          `yaml:"profile"`
	BaseBranch string          `yaml:"base_branch,omitempty"`
	Policy     string          `yaml:"policy,omitempty"`
	Gates      map[string]Gate `yaml:"gates"`
	// Agents overrides the write scope of a role for THIS project, keyed by
	// role slug ("test-writer"). It can only re-aim a role's territory: the
	// universal append-only floor that `hoom agent` enforces is not
	// reachable from here, because a floor you can lower is not a floor.
	Agents AgentPolicies `yaml:"agents,omitempty"`
	// Findings lets open findings of a severity or worse turn verify red.
	// Project-level only: blocking on findings is a team decision, not a
	// stack default, so profiles never carry it.
	Findings *FindingsPolicy `yaml:"findings,omitempty"`
	// Review is the reviewer the project chooses for `hoom review`.
	Review *ReviewPolicy `yaml:"review,omitempty"`
	// Models widens hoom's catalog of models (internal/modelos) for the
	// providers hoom passes a model to (tablero-con-paneles): hoom tablero's
	// Modelo selector offers them after the catalog's.
	Models ModelCatalog `yaml:"models,omitempty"`

	// Dir is the project root where hoom.yaml lives (not serialized).
	Dir string `yaml:"-"`
}

// DefaultMaxEvidenceKiB is the evidence cap of `hoom review` when hoom.yaml
// does not set review.max_evidence_kib.
const DefaultMaxEvidenceKiB = 320

// MaxEvidenceKiB is the highest review.max_evidence_kib accepted: no model
// holds more than a few MiB of text, and the cap times 1024 must never
// overflow.
const MaxEvidenceKiB = 16384

// ReviewPolicy is the `review:` section of hoom.yaml. Every key is optional;
// model and effort are the provider's vocabulary and are never validated.
type ReviewPolicy struct {
	Provider       string `yaml:"provider"`
	Model          string `yaml:"model"`
	Effort         string `yaml:"effort"`
	SameProvider   *bool  `yaml:"same_provider"`
	Isolated       *bool  `yaml:"isolated"`
	MaxEvidenceKiB *int   `yaml:"max_evidence_kib"`
	// Timeout is review.timeout, with the rules of agents.<rol>.timeout;
	// 0 = not declared.
	Timeout time.Duration `yaml:"-"`
}

// reviewKeys are the keys `review:` accepts, in the order the error names them:
// the six of review-aislada (CA-394 fixes that text) and then timeout, which
// verbos-de-la-tarjeta added after them.
var (
	reviewKeys        = []string{"provider", "model", "effort", "same_provider", "isolated", "max_evidence_kib"}
	reviewKeysTimeout = "timeout"
)

// UnmarshalYAML rejects unknown keys inside `review`: a typo like `isolate`
// would otherwise hand the reviewer the user's personal config in silence.
func (p *ReviewPolicy) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("review debe ser un mapa (ej. review: { model: gpt-5.6-sol, effort: high })")
	}
	var timeout *yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if !containsKey(reviewKeys, key) && key != reviewKeysTimeout {
			return fmt.Errorf("review: clave desconocida %q (validas: %s) y %s", key, strings.Join(reviewKeys, ", "), reviewKeysTimeout)
		}
		if key == reviewKeysTimeout {
			timeout = n.Content[i+1]
		}
	}
	type plain ReviewPolicy // sin el metodo: evita la recursion
	if err := n.Decode((*plain)(p)); err != nil {
		return err
	}
	if timeout != nil && !esNulo(timeout) {
		d, err := ParseTimeout("review.timeout", timeout.Value)
		if err != nil {
			return err
		}
		p.Timeout = d
	}
	return nil
}

// validateReview rejects a cap that could never admit any evidence.
func (m *Manifest) validateReview() error {
	return m.Review.validate()
}

// validate rejects a cap that could never admit any evidence, or one so
// large that the cap in bytes would overflow.
func (p *ReviewPolicy) validate() error {
	switch {
	case p == nil || p.MaxEvidenceKiB == nil:
		return nil
	case *p.MaxEvidenceKiB <= 0:
		return fmt.Errorf("review: max_evidence_kib debe ser mayor que 0")
	case *p.MaxEvidenceKiB > MaxEvidenceKiB:
		return fmt.Errorf("review: max_evidence_kib no puede pasar de %d", MaxEvidenceKiB)
	}
	return nil
}

// ParseReview reads only the `review:` section of a hoom.yaml, with the same
// strictness Load applies to it. `hoom review` uses it on the hoom.yaml of
// the BASE: a change does not pick its own reviewer. nil = no section.
func ParseReview(raw []byte) (*ReviewPolicy, error) {
	var doc struct {
		Review *ReviewPolicy `yaml:"review"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if err := doc.Review.validate(); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	return doc.Review, nil
}

// ParseAgents reads only the `agents:` section of a hoom.yaml — the one the
// review reads from its merge-base, never from the tree it reviews (like
// ParseReview). nil = no section.
func ParseAgents(raw []byte) (map[string]AgentPolicy, error) {
	var doc struct {
		Agents AgentPolicies `yaml:"agents"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	return doc.Agents, nil
}

// ReviewIsolated reports whether the reviewer runs without the user's
// personal provider config: true unless review.isolated is false.
func (m *Manifest) ReviewIsolated() bool {
	if m == nil {
		return true
	}
	return m.Review.IsolatedOrDefault()
}

// ReviewMaxEvidenceKiB is the evidence cap: review.max_evidence_kib, or
// DefaultMaxEvidenceKiB when absent.
func (m *Manifest) ReviewMaxEvidenceKiB() int {
	if m == nil {
		return DefaultMaxEvidenceKiB
	}
	return m.Review.MaxEvidenceKiBOrDefault()
}

// IsolatedOrDefault is review.isolated, true when absent (nil policy too).
func (p *ReviewPolicy) IsolatedOrDefault() bool {
	if p == nil || p.Isolated == nil {
		return true
	}
	return *p.Isolated
}

// MaxEvidenceKiBOrDefault is review.max_evidence_kib, DefaultMaxEvidenceKiB
// when absent (nil policy too).
func (p *ReviewPolicy) MaxEvidenceKiBOrDefault() int {
	if p == nil || p.MaxEvidenceKiB == nil {
		return DefaultMaxEvidenceKiB
	}
	return *p.MaxEvidenceKiB
}

// FindingsBlockOn is the validated threshold of the findings_open gate:
// low | medium | high, or "" when the project did not opt in.
func (m *Manifest) FindingsBlockOn() string {
	if m == nil || m.Findings == nil || m.Findings.BlockOn == nil {
		return ""
	}
	return *m.Findings.BlockOn
}

// Severities are the values block_on accepts, mildest first. They are the
// same three a finding can carry.
var Severities = []string{"low", "medium", "high"}

// UnmarshalYAML rejects unknown keys inside `findings`: a typo like
// `blockon` would otherwise switch a BLOCKING gate off in silence, which is
// exactly the failure an opt-in gate must not have.
func (p *FindingsPolicy) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("findings debe ser un mapa (ej. findings: { block_on: high })")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if key := n.Content[i].Value; key != "block_on" {
			return fmt.Errorf("findings: clave desconocida %q (la unica valida es block_on)", key)
		}
	}
	type plain FindingsPolicy // sin el metodo: evita la recursion
	return n.Decode((*plain)(p))
}

// validateFindings normalizes block_on the way `hoom finding add` normalizes
// a severity. Absent or null = the gate does not run; any other value that is
// not a severity — "" included — is an invalid manifest.
func (m *Manifest) validateFindings() error {
	if m.Findings == nil || m.Findings.BlockOn == nil {
		return nil
	}
	v := strings.ToLower(strings.TrimSpace(*m.Findings.BlockOn))
	for _, s := range Severities {
		if v == s {
			m.Findings.BlockOn = &v
			return nil
		}
	}
	return fmt.Errorf("findings.block_on invalido %q (valores: %s)",
		*m.Findings.BlockOn, strings.Join(Severities, "|"))
}

// validateBaseBranch refuses a base_branch that git would read as an option.
// The value ends up as a revision argument of git (`<base>...HEAD`), and a
// name starting with "-" is never a branch: `--output=<path>` made a plain
// read create or truncate that path (finding bb986a). The same rule
// gitx.ResolverCommit applies to --base.
func (m *Manifest) validateBaseBranch() error {
	if strings.HasPrefix(m.BaseBranch, "-") {
		return fmt.Errorf("base_branch invalido %q: el nombre de una rama no empieza con \"-\"", m.BaseBranch)
	}
	return nil
}

// canonical execution order for well-known gates; unknown gates go last, alphabetical.
var gateOrder = []string{"build", "lint", "static", "security", "arch", "compose_lint", "compose_metrics", "test", "mutation"}

// SortedGateNames returns gate names in canonical execution order.
func (m *Manifest) SortedGateNames() []string {
	rank := map[string]int{}
	for i, n := range gateOrder {
		rank[n] = i
	}
	names := make([]string, 0, len(m.Gates))
	for n := range m.Gates {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, oki := rank[names[i]]
		rj, okj := rank[names[j]]
		switch {
		case oki && okj:
			return ri < rj
		case oki:
			return true
		case okj:
			return false
		default:
			return names[i] < names[j]
		}
	})
	return names
}

// Find walks up from dir looking for hoom.yaml. Returns the containing dir.
func Find(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, FileName)); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("no se encontro %s en %s ni en sus directorios padres (ejecuta 'hoom init')", FileName, dir)
		}
		abs = parent
	}
}

// Load reads hoom.yaml from dir and resolves profile inheritance using the
// provided resolver (injected to avoid an import cycle with profiles). Its
// base_branch is the base hoom hands to git, so one that git would read as an
// option makes the file invalid (finding bb986a).
func Load(dir string, resolveProfile func(name string) (map[string]Gate, string, error)) (*Manifest, error) {
	m, err := LoadWithoutBaseCheck(dir, resolveProfile)
	if err != nil {
		return nil, err
	}
	if err := m.validateBaseBranch(); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	return m, nil
}

// LoadWithoutBaseCheck is Load for a hoom.yaml whose base_branch hoom only
// shows and never hands to git: the task branch's, which the review reads to
// say it does not obey it (CA-430). Every other rule of Load applies
// (findings f3e283, 173dde and 461d05).
func LoadWithoutBaseCheck(dir string, resolveProfile func(name string) (map[string]Gate, string, error)) (*Manifest, error) {
	root, err := Find(dir)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if m.Schema != Schema {
		return nil, fmt.Errorf("schema no soportado %q (esperado %q)", m.Schema, Schema)
	}
	if err := m.validateFindings(); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if err := m.validateReview(); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if m.BaseBranch == "" {
		m.BaseBranch = DefaultBase
	}
	if m.Policy == "" {
		m.Policy = PolicyStrict
	}
	m.Dir = root

	// Merge: profile defaults first, project overrides on top.
	if m.Profile != "" && resolveProfile != nil {
		defaults, _, err := resolveProfile(m.Profile)
		if err != nil {
			return nil, fmt.Errorf("perfil %q: %w", m.Profile, err)
		}
		merged := map[string]Gate{}
		for name, g := range defaults {
			merged[name] = g
		}
		for name, g := range m.Gates {
			merged[name] = overlay(merged[name], g)
		}
		m.Gates = merged
	}
	if len(m.Gates) == 0 {
		return nil, fmt.Errorf("hoom.yaml no declara gates y el perfil %q no aporta ninguno", m.Profile)
	}
	return &m, nil
}

// overlay applies project-level overrides on top of a profile default gate.
// nil pointer fields inherit; non-nil fields replace — so `cmd: ""` in the
// project explicitly disables (declares absent) a gate the profile ships,
// `diff_cmd: ""` clears the profile's diff command, and `required:` only
// changes when the project actually writes it.
func overlay(base, over Gate) Gate {
	out := base
	if over.Required != nil {
		out.Required = over.Required
	}
	if over.Cmd != nil {
		out.Cmd = over.Cmd
	}
	if over.DiffCmd != nil {
		out.DiffCmd = over.DiffCmd
	}
	if over.Notes != "" {
		out.Notes = over.Notes
	}
	if over.Partial != "" {
		out.Partial = over.Partial
	}
	return out
}
