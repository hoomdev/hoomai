// Package manifest defines and loads hoom.yaml: the per-project contract
// that maps abstract gate capabilities to concrete stack commands.
// The hoomAI core never knows what Laravel, Kotlin or Go are; it only
// executes what the manifest declares and evaluates the results.
package manifest

import (
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"weak"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/modelos"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/riesgo"
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
	// AplicaEn (gates-por-ruta) is gates.<g>.aplica_en: the paths the gate
	// applies to, a non-empty list of patterns relative to the root of the
	// repository with the syntax of `rutas:` (riesgo.Coincide,
	// riesgo.PatronValido; CA-777):
	//
	//	gates:
	//	  mutation:
	//	    aplica_en: [app/**, src/**]
	//
	// nil = the key is not written: the gate applies always, as today. In a
	// verify de trabajo (gates.Options.Cambio, without Full, Cierre or Only)
	// a gate whose AplicaEn matches no file of the change does not run its
	// command and stays verdict.StatusNoAplica (gates.Aplica, gates.Run).
	//
	// The project's list replaces the profile's whole (Load); a gate of the
	// project without the key keeps the profile's. The patterns are kept as
	// written, in their order. Load and LoadWithoutBaseCheck refuse, with
	// "hoom.yaml invalido: " in front:
	//
	//   - an aplica_en that is not a non-empty list of strings (an empty
	//     list, null, a scalar, a map, or a list with an element that is not a
	//     string): fmt.Sprintf(ErrAplicaEnLista, <g>);
	//   - an element that is not a valid pattern (riesgo.PatronValido):
	//     fmt.Sprintf(ErrAplicaEnPatron, <g>, <p>).
	AplicaEn []string `yaml:"aplica_en,omitempty"`
}

// GateKeys (gates-por-ruta) are the keys gates.<g> accepts, in the order its
// error names them: required, cmd, diff_cmd, notes, partial, aplica_en. Any
// other key of a gate, in hoom.yaml or in an embedded profile, is the error
// ErrGateClaveDesconocida: never a silence.
var GateKeys = []string{"required", "cmd", "diff_cmd", "notes", "partial", "aplica_en"}

// The errors of a gate of hoom.yaml (gates-por-ruta), exactly. Load and
// LoadWithoutBaseCheck give them with "hoom.yaml invalido: " in front, and
// then `hoom verify` runs no gate and writes no verdict. <g> (%s) is the
// gate's name as written; the key and the pattern (%q) are quoted as %q
// quotes them.
const (
	// ErrAplicaEnLista is an aplica_en that is not a non-empty list of
	// patterns; %s is the gate.
	ErrAplicaEnLista = "gates.%s.aplica_en: tiene que ser una lista no vacia de patrones"
	// ErrAplicaEnPatron is an element of aplica_en that is not a valid
	// pattern (riesgo.PatronValido); %s is the gate and %q the element.
	ErrAplicaEnPatron = "gates.%s.aplica_en: patron invalido %q"
	// ErrGateClaveDesconocida is a key of gates.<g> outside GateKeys; %s is
	// the gate and %q the key. The list in parentheses is GateKeys joined by
	// ", ". In an embedded profile it is the same text, after the prefix
	// that profiles gives an embedded profile it cannot read.
	ErrGateClaveDesconocida = "gates.%s: clave desconocida %q (validas: required, cmd, diff_cmd, notes, partial, aplica_en)"
)

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
	// PorRiesgo (proporcion-por-riesgo) are agents.<rol>.bajo, .normal and
	// .alto, keyed by riesgo: each one's model, effort, budget_usd and
	// timeout, with the same rules as in agents.<rol> (Provider is always
	// ""; the provider does not vary by riesgo). A riesgo whose map is null
	// or empty has no entry, and nil means none declared.
	PorRiesgo map[string]RoleDefaults `yaml:"-"`
}

// AgentKeys are the keys agents.<rol> accepts, in the order the error names
// them. proporcion-por-riesgo: the three riesgos go after write, so the
// error is `agents.<rol>: clave desconocida "<k>" (validas: provider, model,
// effort, budget_usd, timeout, write, bajo, normal, alto)`.
var AgentKeys = []string{"provider", "model", "effort", "budget_usd", "timeout", "write", riesgo.Bajo, riesgo.Normal, riesgo.Alto}

// AgentRiesgoKeys (proporcion-por-riesgo) are the keys agents.<rol>.<r>
// accepts, in the order its error names them:
// `agents.<rol>.<r>: clave desconocida "<k>" (validas: model, effort, budget_usd, timeout)`.
// provider is not one of them: `alto: { provider: codex }` is that error.
var AgentRiesgoKeys = []string{"model", "effort", "budget_usd", "timeout"}

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
	if esNulo(n) {
		*c = nil
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("models debe ser un mapa por provider (ej. models: { codex: [ { name: gpt-7-sol } ] })")
	}
	validos := modelos.ConModelo()
	out := ModelCatalog{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		p := n.Content[i].Value
		if _, visto := out[p]; visto {
			return fmt.Errorf("models: el provider %q esta repetido (linea %d)", p, n.Content[i].Line)
		}
		if !containsKey(validos, p) {
			if containsKey(providerNames(), p) {
				return fmt.Errorf("models.%s: hoom no le pasa el modelo a %s", p, p)
			}
			return fmt.Errorf("models: provider desconocido %q (validos: %s)", p, strings.Join(validos, ", "))
		}
		lista := n.Content[i+1]
		entradas := []ModelEntry{}
		if !esNulo(lista) {
			if lista.Kind != yaml.SequenceNode {
				return fmt.Errorf("models.%s debe ser una lista de modelos (ej. - name: gpt-7-sol)", p)
			}
			for j, e := range lista.Content {
				m, err := decodeModel(p, j, e)
				if err != nil {
					return err
				}
				entradas = append(entradas, m)
			}
		}
		out[p] = entradas
	}
	if len(out) == 0 {
		out = nil
	}
	*c = out
	return nil
}

// decodeModel is the entry j of models.<p>, with its rules.
func decodeModel(p string, j int, n *yaml.Node) (ModelEntry, error) {
	var m ModelEntry
	donde := fmt.Sprintf("models.%s[%d]", p, j)
	if n.Kind != yaml.MappingNode {
		return m, fmt.Errorf("%s debe ser un mapa con name y description", donde)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		if !containsKey(ModelKeys, k) {
			return m, fmt.Errorf("%s: clave desconocida %q (validas: %s)", donde, k, strings.Join(ModelKeys, ", "))
		}
		v := n.Content[i+1]
		if esNulo(v) {
			continue
		}
		if v.Kind != yaml.ScalarNode {
			return m, fmt.Errorf("%s.%s debe ser un texto", donde, k)
		}
		switch k {
		case "name":
			m.Name = strings.TrimSpace(v.Value)
		case "description":
			m.Description = strings.TrimSpace(v.Value)
		}
	}
	if m.Name == "" {
		return m, fmt.Errorf("%s: falta name", donde)
	}
	if strings.HasPrefix(m.Name, "-") {
		return m, fmt.Errorf("%s: el nombre no puede empezar con '-'", donde)
	}
	return m, nil
}

// AgentPolicies are the agents: section, keyed by role slug. It decodes
// itself so every error names the role: a key hoom does not know is an
// error, never a silence (an ignored `efort: low` is the very failure the
// launch values must not have).
//
// proporcion-por-riesgo: a role hoom does not know (`writr`, `test_writer`,
// a native name like `hoom-writer`) is an error too:
// `agents: rol "<x>" desconocido (validos: <the slugs of agents.Roles, in
// its order, joined by ", ">)`. Each agents.<rol> also accepts the keys
// bajo, normal and alto (AgentPolicy.PorRiesgo), each one a map with
// AgentRiesgoKeys; a null or empty one changes nothing. The errors, with
// "hoom.yaml invalido: " in front from Load, LoadWithoutBaseCheck and
// LoadRoles:
//
//   - `agents.<rol>.<r>: clave desconocida "<k>" (validas: model, effort, budget_usd, timeout)`;
//   - `agents.<rol>.<r> debe ser un mapa (ej. alto: { model: fuerte })`;
//   - their values follow agents.<rol>'s rules and texts with the place
//     `agents.<rol>.<r>` (`agents.<rol>.<r>.budget_usd: ...`,
//     `agents.<rol>.<r>.timeout: ...`);
//   - agents.reviewer keeps refusing every launch key, bajo, normal and
//     alto included, with today's error.
//
// A hoom.yaml without these keys loads as today.
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
		if !esRol(role) {
			var slugs []string
			for _, r := range agents.Roles() {
				slugs = append(slugs, r.Slug)
			}
			return fmt.Errorf("agents: rol %q desconocido (validos: %s)", role, strings.Join(slugs, ", "))
		}
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
	porRiesgo := map[string]RoleDefaults{}
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
		if containsKey(riesgo.Vocabulario, key) {
			d, hay, err := decodeAgentRiesgo(role, key, n.Content[i+1])
			if err != nil {
				return p, err
			}
			if hay {
				porRiesgo[key] = d
			}
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
	if len(porRiesgo) > 0 {
		p.PorRiesgo = porRiesgo
	}
	return p, nil
}

// esRol says whether role is the slug of one of agents.Roles, exactly as
// written.
func esRol(role string) bool {
	for _, r := range agents.Roles() {
		if r.Slug == role {
			return true
		}
	}
	return false
}

// decodeAgentRiesgo is one agents.<rol>.<r> (proporcion-por-riesgo): its
// keys (AgentRiesgoKeys) and values, with agents.<rol>'s rules and the place
// agents.<rol>.<r> in every error. hay is false for a null or empty map, or
// one that declares nothing: it changes nothing.
func decodeAgentRiesgo(role, r string, n *yaml.Node) (d RoleDefaults, hay bool, err error) {
	donde := "agents." + role + "." + r
	if esNulo(n) {
		return d, false, nil
	}
	if n.Kind != yaml.MappingNode {
		return d, false, fmt.Errorf("%s debe ser un mapa (ej. alto: { model: fuerte })", donde)
	}
	var timeout *yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if !containsKey(AgentRiesgoKeys, key) {
			return d, false, fmt.Errorf("%s: clave desconocida %q (validas: %s)", donde, key, strings.Join(AgentRiesgoKeys, ", "))
		}
		if key == "timeout" {
			timeout = n.Content[i+1]
		}
	}
	var plain struct {
		Model     string   `yaml:"model"`
		Effort    string   `yaml:"effort"`
		BudgetUSD *float64 `yaml:"budget_usd"`
	}
	if err := n.Decode(&plain); err != nil {
		return d, false, fmt.Errorf("%s: %v", donde, err)
	}
	d = RoleDefaults{Model: strings.TrimSpace(plain.Model), Effort: strings.TrimSpace(plain.Effort), BudgetUSD: plain.BudgetUSD}
	if b := d.BudgetUSD; b != nil && !(*b >= 0) || b != nil && math.IsInf(*b, 0) {
		return d, false, fmt.Errorf("%s.budget_usd: %v no es un numero mayor o igual que 0 (0 = sin tope)", donde, *b)
	}
	if timeout != nil && !esNulo(timeout) {
		t, err := ParseTimeout(donde+".timeout", timeout.Value)
		if err != nil {
			return d, false, err
		}
		d.Timeout = t
	}
	hay = d.Model != "" || d.Effort != "" || d.BudgetUSD != nil || d.Timeout != 0
	return d, hay, nil
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
//
// proporcion-por-riesgo: Model may be a level (modelos.Fuerte, Medio,
// Barato), which the launch translates with its provider.
type RoleDefaults struct {
	Provider  string
	Model     string
	Effort    string
	BudgetUSD *float64      // nil = not declared; 0 = declared without a cap
	Timeout   time.Duration // 0 = not declared
	// PorRiesgo (proporcion-por-riesgo) are the values of
	// agents.<rol>.<r>, keyed by riesgo (AgentPolicy.PorRiesgo): level 2 of
	// the rule (valores.ResolverConRiesgo), over the fields above. nil when
	// the role declares none, and always for the reviewer (review: has no
	// values by riesgo). Their Provider is always "".
	PorRiesgo map[string]RoleDefaults
}

// RoleDefaults is the ONE place that says what hoom.yaml declares for a role:
// `hoom agent`, the card verbs and the board ask here. The reviewer's come
// from review: (it has no budget there); every other role's from
// agents.<rol>. A nil manifest declares nothing.
//
// proporcion-por-riesgo: the result carries agents.<rol>.<r> in PorRiesgo
// (a copy: changing it changes nothing in m); the reviewer's has none.
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
	if len(p.PorRiesgo) > 0 {
		d.PorRiesgo = make(map[string]RoleDefaults, len(p.PorRiesgo))
		for r, v := range p.PorRiesgo {
			c := RoleDefaults{Model: v.Model, Effort: v.Effort, Timeout: v.Timeout}
			if v.BudgetUSD != nil {
				b := *v.BudgetUSD
				c.BudgetUSD = &b
			}
			d.PorRiesgo[r] = c
		}
	}
	return d
}

// LoadRoles reads only the agents: and review: sections of root/hoom.yaml,
// with the strictness Load applies to them: the values of the PROJECT, which
// the board and the launches read whatever the card's branch says. A root
// without hoom.yaml declares nothing (an empty manifest, no error).
//
// It reads autonomia: the same way (autonomia-entre-firmas): the pilot's
// policy is the project's, never the branch's. The result's Autonomia is the
// declared section (nil without it), and an invalid one is the error of
// AutonomiaPolicy's contract, wrapped as `hoom.yaml invalido: ...`; the rule
// that rounds need findings.block_on is judged against this same file.
//
// It reads encierro: the same way (contencion-del-writer), without running
// git: the result's Encierro is the declared section (nil without it, and
// then nothing is enclosed), with Exigir ExigirNo when the section does not
// say it, and an invalid one is the error of EncierroPolicy's contract,
// wrapped as `hoom.yaml invalido: ...`.
//
// It reads spec: the same way (proporcion-por-riesgo): the result's Spec is
// the declared section (nil without it), and an invalid one is the error of
// SpecPolicy's contract, wrapped as `hoom.yaml invalido: ...`; the board
// measures a card's spec with its TopeDelSpec.
func LoadRoles(root string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(root, FileName))
	if os.IsNotExist(err) {
		return &Manifest{Dir: root}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Agents    AgentPolicies    `yaml:"agents"`
		Review    *ReviewPolicy    `yaml:"review"`
		Autonomia *AutonomiaPolicy `yaml:"autonomia"`
		Encierro  *EncierroPolicy  `yaml:"encierro"`
		Spec      *SpecPolicy      `yaml:"spec"`
		Findings  yaml.Node        `yaml:"findings"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if err := doc.Review.validate(); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if err := validarAutonomia(doc.Autonomia, blockOnDeclarado(doc.Findings), maxRondasDeclarado(doc.Findings)); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	roles := &Manifest{Agents: doc.Agents, Review: doc.Review, Autonomia: doc.Autonomia, Encierro: doc.Encierro, Spec: doc.Spec, Dir: root}
	registrarHuella(roles, raw)
	return roles, nil
}

// AutonomiaPolicy is the optional `autonomia:` section of the PROJECT's
// hoom.yaml (autonomia-entre-firmas): how far the pilot goes alone between a
// person's signature and a person's acceptance.
//
//	autonomia:
//	  reintentos_ante_rojo: 1   # 0 o 1
//	  rondas_de_hallazgos: 2    # de 0 a 5
//	  ventana_max: 80           # de 1 a 100: porcentaje de la ventana de 5 h
//
// Without the section, or without a key, both counters are 0 and VentanaMax
// is nil (not declared). The section is strict; the rest of the top level
// stays as lenient as always. Load (and LoadWithoutBaseCheck) and LoadRoles
// reject an invalid section, wrapped as `hoom.yaml invalido: <error>`, and
// the errors are exactly:
//
//   - `autonomia: clave desconocida "<k>" (validas: reintentos_ante_rojo, rondas_de_hallazgos, ventana_max)`;
//   - `autonomia: reintentos_ante_rojo tiene que ser 0 o 1`;
//   - `autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5`;
//   - `autonomia: ventana_max tiene que ser un entero de 1 a 100`;
//   - `autonomia: rondas_de_hallazgos necesita findings.block_on`, when the
//     rounds are more than 0 and the same hoom.yaml has no findings.block_on
//     (without it findings_open never runs and no round could close);
//   - `autonomia: rondas_de_hallazgos (<a>) pasa de findings.max_rondas (<m>)`,
//     when the same hoom.yaml declares a findings.max_rondas below the
//     rounds (ciclo-lo-decide-hoom): the pilot never goes past the card's
//     cap (Manifest.FindingsMaxRondas).
type AutonomiaPolicy struct {
	// ReintentosAnteRojo: 0 or 1 retries of the writer after a red verdict.
	ReintentosAnteRojo int `yaml:"reintentos_ante_rojo"`
	// RondasDeHallazgos: 0 to 5 rounds of findings after a review.
	RondasDeHallazgos int `yaml:"rondas_de_hallazgos"`
	// VentanaMax: 1 to 100, the percent of the 5 h window at which the pilot
	// stops launching on a provider; nil = not declared.
	VentanaMax *int `yaml:"ventana_max"`
}

// AutonomiaKeys are the keys `autonomia:` accepts, in the order its error
// lists them.
var AutonomiaKeys = []string{"reintentos_ante_rojo", "rondas_de_hallazgos", "ventana_max"}

// UnmarshalYAML decodes `autonomia:` strictly: an unknown key or a value out
// of range is the error of AutonomiaPolicy's contract. The rule that needs
// findings.block_on is checked by the loaders, which see the whole file.
func (p *AutonomiaPolicy) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("autonomia debe ser un mapa (ej. autonomia: { reintentos_ante_rojo: 1 })")
	}
	var out AutonomiaPolicy
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, v := n.Content[i].Value, n.Content[i+1]
		if !containsKey(AutonomiaKeys, key) {
			return fmt.Errorf("autonomia: clave desconocida %q (validas: %s)", key, strings.Join(AutonomiaKeys, ", "))
		}
		if esNulo(v) {
			continue // como en findings: nulo = sin la clave
		}
		x, ok := enteroYAML(v)
		switch key {
		case "reintentos_ante_rojo":
			if !ok || x < 0 || x > 1 {
				return fmt.Errorf("autonomia: reintentos_ante_rojo tiene que ser 0 o 1")
			}
			out.ReintentosAnteRojo = x
		case "rondas_de_hallazgos":
			if !ok || x < 0 || x > 5 {
				return fmt.Errorf("autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5")
			}
			out.RondasDeHallazgos = x
		case "ventana_max":
			if !ok || x < 1 || x > 100 {
				return fmt.Errorf("autonomia: ventana_max tiene que ser un entero de 1 a 100")
			}
			out.VentanaMax = &x
		}
	}
	*p = out
	return nil
}

// enteroYAML is the integer a scalar node holds, when it holds one (an int
// tag: "1.0", "uno" or true are not).
func enteroYAML(v *yaml.Node) (int, bool) {
	var x int
	if v.Kind != yaml.ScalarNode || v.Tag != "!!int" || v.Decode(&x) != nil {
		return 0, false
	}
	return x, true
}

// validarAutonomia is the rule of autonomia that needs the rest of the file:
// rounds of findings close only through findings_open, which runs only with
// findings.block_on, and the pilot's rounds never pass a declared
// findings.max_rondas (maxRondas nil = not declared, or not valid).
func validarAutonomia(a *AutonomiaPolicy, blockOn bool, maxRondas *int) error {
	if a != nil && a.RondasDeHallazgos > 0 && !blockOn {
		return fmt.Errorf("autonomia: rondas_de_hallazgos necesita findings.block_on")
	}
	if a != nil && maxRondas != nil && a.RondasDeHallazgos > *maxRondas {
		return fmt.Errorf("autonomia: rondas_de_hallazgos (%d) pasa de findings.max_rondas (%d)", a.RondasDeHallazgos, *maxRondas)
	}
	return nil
}

// blockOnDeclarado says whether a findings: node declares a block_on, valid
// or not (Load judges its value).
func blockOnDeclarado(n yaml.Node) bool {
	if n.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == "block_on" && !esNulo(n.Content[i+1]) {
			return true
		}
	}
	return false
}

// EncierroPolicy is the optional `encierro:` section of the PROJECT's
// hoom.yaml (contencion-del-writer, "La politica encierro"). With the
// section, what runs code of the writer under an envelope of a role that
// executes, the review or the pilot runs enclosed; without it, nothing does.
//
//	encierro:
//	  exigir: completo          # completo, parcial o no (no por defecto)
//	  escribibles:              # rutas fuera del arbol que el encierro deja escribir
//	    - ~/.semgrep
//	  gates_sin_encierro:       # gates del verify que corren como hoy
//	    - e2e
//
// The section is strict. Load (and LoadWithoutBaseCheck) and LoadRoles
// reject an invalid one without running git, wrapped as
// `hoom.yaml invalido: <error>`, and the errors are exactly:
//
//   - `encierro: clave desconocida "<k>" (validas: exigir, escribibles, gates_sin_encierro)`;
//   - `encierro: exigir tiene que ser completo, parcial o no`;
//   - `encierro: escribibles: "<ruta>" no es una ruta absoluta ni empieza con ~/`,
//     for a relative, empty or null entry (`- ~` is null in YAML), "<ruta>"
//     being the entry as the file writes it;
//   - `encierro: escribibles: "<ruta>" es todo el disco o todo el directorio personal`,
//     for what is, after filepath.Clean and expanding "~/" with this
//     process's $HOME, `/` or the $HOME (`/`, `~/`, the $HOME written
//     absolute, also with extra "/" or "." components).
//
// The two rules that need the tree and the repository (a writable path that
// contains or is inside the main checkout or the git common directory, a
// gate that is not among the tree's gates) are judged when the envelope or
// the review starts (encierro.Validar), not here.
type EncierroPolicy struct {
	// Exigir is ExigirCompleto, ExigirParcial or ExigirNo; ExigirNo when the
	// section does not say it (or says it null).
	Exigir string `yaml:"exigir"`
	// Escribibles are the paths outside the tree the encierro lets write, as
	// the file writes them: absolute or starting with "~/" (the $HOME of
	// whoever runs hoom).
	Escribibles []string `yaml:"escribibles"`
	// GatesSinEncierro are names of gates of verify that run as today
	// (unenclosed), also in the verify of the envelope; the photograph of the
	// git material before and after the gates still includes them.
	GatesSinEncierro []string `yaml:"gates_sin_encierro"`
}

// The values of encierro.exigir.
const (
	ExigirCompleto = "completo"
	ExigirParcial  = "parcial"
	ExigirNo       = "no"
)

// EncierroKeys are the keys `encierro:` accepts, in the order its error
// lists them.
var EncierroKeys = []string{"exigir", "escribibles", "gates_sin_encierro"}

// UnmarshalYAML decodes `encierro:` strictly: an unknown key, an exigir out
// of its list or a writable path that is not absolute nor starts with "~/",
// or that is the whole disk or the whole home, is the error of
// EncierroPolicy's contract.
func (p *EncierroPolicy) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("encierro debe ser un mapa (ej. encierro: { exigir: completo })")
	}
	out := EncierroPolicy{Exigir: ExigirNo}
	home, _ := os.UserHomeDir()
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, v := n.Content[i].Value, n.Content[i+1]
		if !containsKey(EncierroKeys, key) {
			return fmt.Errorf("encierro: clave desconocida %q (validas: %s)", key, strings.Join(EncierroKeys, ", "))
		}
		if esNulo(v) {
			continue // como en autonomia: nulo = sin la clave
		}
		switch key {
		case "exigir":
			if v.Kind != yaml.ScalarNode || (v.Value != ExigirCompleto && v.Value != ExigirParcial && v.Value != ExigirNo) {
				return fmt.Errorf("encierro: exigir tiene que ser completo, parcial o no")
			}
			out.Exigir = v.Value
		case "escribibles":
			if v.Kind != yaml.SequenceNode {
				return fmt.Errorf("encierro: escribibles tiene que ser una lista de rutas")
			}
			for _, e := range v.Content {
				ruta := e.Value
				if e.Kind != yaml.ScalarNode || esNulo(e) || !rutaEscribible(ruta) {
					return fmt.Errorf("encierro: escribibles: %q no es una ruta absoluta ni empieza con ~/", ruta)
				}
				if todoElDisco(ruta, home) {
					return fmt.Errorf("encierro: escribibles: %q es todo el disco o todo el directorio personal", ruta)
				}
				out.Escribibles = append(out.Escribibles, ruta)
			}
		case "gates_sin_encierro":
			if v.Kind != yaml.SequenceNode {
				return fmt.Errorf("encierro: gates_sin_encierro tiene que ser una lista de nombres de gates")
			}
			for _, e := range v.Content {
				if e.Kind != yaml.ScalarNode || esNulo(e) || strings.TrimSpace(e.Value) == "" {
					return fmt.Errorf("encierro: gates_sin_encierro tiene que ser una lista de nombres de gates")
				}
				out.GatesSinEncierro = append(out.GatesSinEncierro, e.Value)
			}
		}
	}
	*p = out
	return nil
}

// rutaEscribible says whether ruta has the shape encierro.escribibles takes:
// absolute, or starting with "~/".
func rutaEscribible(ruta string) bool {
	return strings.HasPrefix(ruta, "~/") || strings.HasPrefix(ruta, "/") || filepath.IsAbs(ruta)
}

// todoElDisco says whether ruta, cleaned and with "~/" expanded with home,
// is the whole disk or the whole home: making it writable would leave
// nothing enclosed. An ancestor of the home is not judged here: when the
// project lives under it, encierro.Validar refuses it at the start because it
// contains the main checkout. It runs no git.
func todoElDisco(ruta, home string) bool {
	if rest, ok := strings.CutPrefix(ruta, "~/"); ok {
		if filepath.Clean("/"+rest) == "/" && !strings.HasPrefix(filepath.Clean(rest), "..") {
			return true // ~/, ~/. and the like, whatever the home is
		}
		if home == "" {
			return false
		}
		ruta = filepath.Join(home, rest)
	}
	c := filepath.Clean(ruta)
	if c == "/" || c == filepath.VolumeName(c)+string(filepath.Separator) {
		return true
	}
	return home != "" && c == filepath.Clean(home)
}

// AutonomiaOrDefault is the project's autonomia policy: the declared section,
// or the zero policy (0 retries, 0 rounds, VentanaMax nil) without it. A nil
// manifest declares nothing.
func (m *Manifest) AutonomiaOrDefault() AutonomiaPolicy {
	if m == nil || m.Autonomia == nil {
		return AutonomiaPolicy{}
	}
	a := *m.Autonomia
	if a.VentanaMax != nil {
		v := *a.VentanaMax
		a.VentanaMax = &v
	}
	return a
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
	// MaxPendientes (findings.max_pendientes) is the cap on open mediums a
	// task may leave in the project when it is accepted
	// (ciclo-de-review-acotado): an integer >= 0; nil = no cap. Any other
	// value makes the manifest invalid with
	// `findings: max_pendientes tiene que ser un entero mayor o igual a 0`,
	// and an unknown key under findings is
	// `findings: clave desconocida "<k>" (validas: block_on, max_pendientes)`.
	// Since ciclo-lo-decide-hoom that error is
	// `findings: clave desconocida "<k>" (validas: block_on, max_pendientes, max_rondas)`.
	MaxPendientes *int `yaml:"max_pendientes"`
	// MaxRondas (findings.max_rondas, ciclo-lo-decide-hoom) is the cap on the
	// rounds of findings of a card, by any path: the pilot, `hoom siguiente`,
	// `hoom pedir` or the orchestrator. An integer from 1 to 5; nil = not
	// declared (FindingsMaxRondas gives the default then). A null value is
	// no key, like block_on and max_pendientes. Out of range or not an
	// integer (0, 6, 2.5, "dos", true) makes the manifest invalid with
	// `findings: max_rondas tiene que ser un entero de 1 a 5`.
	//
	// It is read from the same hoom.yaml the board reads findings.block_on
	// from (the project's). A declared max_rondas under the same file's
	// autonomia.rondas_de_hallazgos is the error
	// `autonomia: rondas_de_hallazgos (<a>) pasa de findings.max_rondas (<m>)`,
	// in Load, LoadWithoutBaseCheck and LoadRoles, wrapped as
	// `hoom.yaml invalido: <error>` like every other error of the section.
	MaxRondas *int `yaml:"max_rondas"`
}

// DefaultMaxRondas is the cap on the rounds of findings of a hoom.yaml
// without findings.max_rondas (ciclo-lo-decide-hoom): the «segunda y ultima
// ronda» of CAL-4.
const DefaultMaxRondas = 2

// ParseFindings reads only the `findings:` section of a hoom.yaml, with the
// same strictness Load applies to it (block_on normalized, max_pendientes
// checked). taskcmd.Ready uses it on the hoom.yaml of the task's tree. nil =
// no section.
func ParseFindings(raw []byte) (*FindingsPolicy, error) {
	var doc struct {
		Findings *FindingsPolicy `yaml:"findings"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	m := Manifest{Findings: doc.Findings}
	if err := m.validateFindings(); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	return m.Findings, nil
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
	// Autonomia is the pilot's policy (autonomia-entre-firmas): nil = no
	// section. Read it with AutonomiaOrDefault.
	Autonomia *AutonomiaPolicy `yaml:"autonomia,omitempty"`
	// Encierro is the containment policy (contencion-del-writer): nil = no
	// section, and then nothing runs enclosed. What counts is the PROJECT's
	// (LoadRoles of the main checkout), never the branch's.
	Encierro *EncierroPolicy `yaml:"encierro,omitempty"`
	// Rutas is the section `rutas` (riesgo-efectivo-y-tamano): nil = no
	// section, and then every key takes the profile's default. Resolve it
	// with profiles.Clasificador.
	Rutas *Rutas `yaml:"rutas,omitempty"`
	// Spec is the section `spec` (proporcion-por-riesgo): the topes of a
	// spec per riesgo. nil = no section, and then every tope is the
	// default (TopesPorDefecto). Read it with TopeDelSpec.
	Spec *SpecPolicy `yaml:"spec,omitempty"`

	// Dir is the project root where hoom.yaml lives (not serialized).
	Dir string `yaml:"-"`
}

// huellas is the sha256 ([sha256.Size]byte) of the bytes of Dir's hoom.yaml
// that LoadWithoutBaseCheck (and so Load) or LoadRoles read to build each
// manifest they return (gates-por-ruta, findings a6ad52 and 94b657), keyed by
// a weak.Pointer to it: the table never keeps a manifest alive, and the
// cleanup registrarHuella attaches deletes its entry once the manifest is
// unreachable. It lives outside Manifest so that a Manifest is only what its
// hoom.yaml says: two manifests with the same content are equal
// (reflect.DeepEqual) whatever bytes they came from. Only registrarHuella
// writes it; HoomYAMLSigueIgual reads it.
var huellas sync.Map

// registrarHuella records in huellas the fingerprint of raw, the bytes of
// hoom.yaml m was built from, until m is unreachable.
func registrarHuella(m *Manifest, raw []byte) {
	k := weak.Make(m)
	huellas.Store(k, sha256.Sum256(raw))
	runtime.AddCleanup(m, func(k weak.Pointer[Manifest]) { huellas.Delete(k) }, k)
}

// HoomYAMLSigueIgual says whether Dir's hoom.yaml still has the bytes this
// manifest was built from (gates-por-ruta, findings a6ad52 and 94b657): a
// manifest loaded once and reused, as the Studio's, may be judging with a
// hoom.yaml that is no longer the project's. It is false when the file cannot
// be read (gone, a directory, unreadable), when its bytes differ from the
// ones read, and for a manifest without a fingerprint: one not returned by
// Load, LoadWithoutBaseCheck or LoadRoles from the file, such as a literal or
// a copy by value (*m does not carry the fingerprint: it is m's, not its
// content's). Ante la duda, it is not the same. Like Load, it reads through a
// symbolic link; it only compares bytes, so a file rewritten with the same
// ones is still the same.
func (m *Manifest) HoomYAMLSigueIgual() bool {
	if m == nil {
		return false
	}
	h, ok := huellas.Load(weak.Make(m))
	if !ok {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(m.Dir, FileName))
	if err != nil {
		return false
	}
	return h == sha256.Sum256(raw)
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

// FindingsMaxPendientes is findings.max_pendientes: (n, true) when the
// project declares the cap, (0, false) when it does not (no cap).
func (m *Manifest) FindingsMaxPendientes() (n int, ok bool) {
	if m == nil || m.Findings == nil || m.Findings.MaxPendientes == nil {
		return 0, false
	}
	return *m.Findings.MaxPendientes, true
}

// FindingsMaxRondas is the effective cap on the rounds of findings
// (ciclo-lo-decide-hoom): findings.max_rondas when the project declares it;
// without it, DefaultMaxRondas, or autonomia.rondas_de_hallazgos when that is
// greater, so a hoom.yaml of before this spec does not change behavior (a
// project with `autonomia.rondas_de_hallazgos: 3` and no findings.max_rondas
// has cap 3). A nil manifest declares nothing: DefaultMaxRondas.
//
// A card is AT THE CAP when reviewcmd.Rondas of its task is at or above this
// cap and it has at least one blocking finding (finding.Bloquea).
func (m *Manifest) FindingsMaxRondas() int {
	if m == nil {
		return DefaultMaxRondas
	}
	if m.Findings != nil && m.Findings.MaxRondas != nil {
		return *m.Findings.MaxRondas
	}
	if m.Autonomia != nil && m.Autonomia.RondasDeHallazgos > DefaultMaxRondas {
		return m.Autonomia.RondasDeHallazgos
	}
	return DefaultMaxRondas
}

// Severities are the values block_on accepts, mildest first. They are the
// same three a finding can carry.
var Severities = []string{"low", "medium", "high"}

// FindingsKeys are the keys `findings:` accepts, in the order its error
// lists them.
var FindingsKeys = []string{"block_on", "max_pendientes", "max_rondas"}

// maxRondasValido is the cap a findings.max_rondas node holds when it is a
// valid one: an integer from 1 to 5.
func maxRondasValido(v *yaml.Node) (int, bool) {
	x, ok := enteroYAML(v)
	if !ok || x < 1 || x > 5 {
		return 0, false
	}
	return x, true
}

// maxRondasDeclarado is the findings.max_rondas of a findings: node when it
// declares a valid one, nil otherwise (LoadRoles compares autonomia with it
// only then; Load judges an invalid value).
func maxRondasDeclarado(n yaml.Node) *int {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value != "max_rondas" {
			continue
		}
		if x, ok := maxRondasValido(n.Content[i+1]); ok {
			return &x
		}
		return nil
	}
	return nil
}

// UnmarshalYAML rejects unknown keys inside `findings`: a typo like
// `blockon` would otherwise switch a BLOCKING gate off in silence, which is
// exactly the failure an opt-in gate must not have.
func (p *FindingsPolicy) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("findings debe ser un mapa (ej. findings: { block_on: high })")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch key, v := n.Content[i].Value, n.Content[i+1]; key {
		case "block_on":
		case "max_pendientes":
			if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
				continue // como block_on: nulo = sin la clave
			}
			var x int
			if v.Kind != yaml.ScalarNode || v.Tag != "!!int" || v.Decode(&x) != nil || x < 0 {
				return fmt.Errorf("findings: max_pendientes tiene que ser un entero mayor o igual a 0")
			}
		case "max_rondas":
			if esNulo(v) {
				continue // como block_on: nulo = sin la clave
			}
			if _, ok := maxRondasValido(v); !ok {
				return fmt.Errorf("findings: max_rondas tiene que ser un entero de 1 a 5")
			}
		default:
			return fmt.Errorf("findings: clave desconocida %q (validas: %s)", key, strings.Join(FindingsKeys, ", "))
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
//
// gates-por-ruta: each gate of the project is decoded strictly, with the
// keys of GateKeys and the errors ErrGateClaveDesconocida, ErrAplicaEnLista
// and ErrAplicaEnPatron (with "hoom.yaml invalido: " in front). The merge
// with the profile keeps today's rules (nil inherits, non-nil replaces), and
// so does AplicaEn: a project gate with aplica_en replaces the profile's
// list whole; one without the key keeps the profile's. A hoom.yaml without
// aplica_en loads as today.
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
	// the gates first, strictly (gates-por-ruta): an unknown key of a gate
	// would otherwise be silence, and a bad aplica_en yaml.v3's own text
	if err := ValidarGates(raw); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
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
	var maxRondas *int
	if m.Findings != nil {
		maxRondas = m.Findings.MaxRondas
	}
	if err := validarAutonomia(m.Autonomia, m.FindingsBlockOn() != "", maxRondas); err != nil {
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
	// the bytes it is built from, to tell later whether Dir's hoom.yaml is
	// still the one (HoomYAMLSigueIgual)
	registrarHuella(&m, raw)
	return &m, nil
}

// overlay applies project-level overrides on top of a profile default gate.
// nil pointer fields inherit; non-nil fields replace — so `cmd: ""` in the
// project explicitly disables (declares absent) a gate the profile ships,
// `diff_cmd: ""` clears the profile's diff command, and `required:` only
// changes when the project actually writes it. aplica_en (gates-por-ruta)
// follows the same rule: the project's list replaces the profile's whole,
// and a project gate without the key keeps the profile's.
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
	if over.AplicaEn != nil {
		out.AplicaEn = append([]string(nil), over.AplicaEn...)
	}
	return out
}
