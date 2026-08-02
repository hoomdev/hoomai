// Package manifest defines and loads hoom.yaml: the per-project contract
// that maps abstract gate capabilities to concrete stack commands.
// The hoomAI core never knows what Laravel, Kotlin or Go are; it only
// executes what the manifest declares and evaluates the results.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	Schema       = "hoom/v1"
	FileName     = "hoom.yaml"
	DefaultBase  = "main"
	PolicyStrict = "strict"
)

// Gate is one verifiable capability (test, static, lint, arch, mutation,
// build, ...). Required and Cmd are pointers so a project override can
// distinguish "not specified" (inherit from profile) from an explicit
// value — in particular `cmd: ""` explicitly declares the gate ABSENT,
// overriding the profile default.
type Gate struct {
	Required *bool   `yaml:"required"`
	Cmd      *string `yaml:"cmd"`
	DiffCmd  string  `yaml:"diff_cmd,omitempty"`
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

// Manifest is the parsed hoom.yaml after profile inheritance is resolved.
type Manifest struct {
	Schema     string          `yaml:"schema"`
	Project    string          `yaml:"project"`
	Profile    string          `yaml:"profile"`
	BaseBranch string          `yaml:"base_branch,omitempty"`
	Policy     string          `yaml:"policy,omitempty"`
	Gates      map[string]Gate `yaml:"gates"`

	// Dir is the project root where hoom.yaml lives (not serialized).
	Dir string `yaml:"-"`
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
// provided resolver (injected to avoid an import cycle with profiles).
func Load(dir string, resolveProfile func(name string) (map[string]Gate, string, error)) (*Manifest, error) {
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
// and `required:` only changes when the project actually writes it.
func overlay(base, over Gate) Gate {
	out := base
	if over.Required != nil {
		out.Required = over.Required
	}
	if over.Cmd != nil {
		out.Cmd = over.Cmd
	}
	if over.DiffCmd != "" {
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
