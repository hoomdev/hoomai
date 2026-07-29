// Package profiles ships the built-in stack profiles (laravel, kmp,
// kmp-compose, go) embedded in the binary, resolves profile inheritance
// (kmp-compose extends kmp) and detects the stack of a project directory.
// A profile is only a mapping from abstract capabilities to concrete
// ecosystem commands; all logic lives in the agnostic core.
package profiles

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/manifest"
)

//go:embed assets/profiles/*.yaml
var assetsFS embed.FS

// Profile mirrors assets/profiles/*.yaml.
type Profile struct {
	Name        string                   `yaml:"name"`
	Description string                   `yaml:"description"`
	Extends     string                   `yaml:"extends,omitempty"`
	Detect      Detect                   `yaml:"detect"`
	Gates       map[string]manifest.Gate `yaml:"gates"`
}

// Detect declares how `hoom init` recognizes a stack.
type Detect struct {
	Files      []string        `yaml:"files"`
	AnyContent []ContentSignal `yaml:"any_content,omitempty"`
}

type ContentSignal struct {
	Path     string `yaml:"path"`
	Contains string `yaml:"contains"`
}

func loadAll() (map[string]*Profile, error) {
	entries, err := assetsFS.ReadDir("assets/profiles")
	if err != nil {
		return nil, err
	}
	out := map[string]*Profile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		raw, err := assetsFS.ReadFile("assets/profiles/" + e.Name())
		if err != nil {
			return nil, err
		}
		var p Profile
		if err := yaml.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("perfil embebido %s invalido: %w", e.Name(), err)
		}
		out[p.Name] = &p
	}
	return out, nil
}

// Names lists available profile names, sorted.
func Names() []string {
	all, err := loadAll()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Describe returns "name - description" lines for `hoom profiles`.
func Describe() []string {
	all, err := loadAll()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range Names() {
		p := all[n]
		line := fmt.Sprintf("%-12s %s", p.Name, p.Description)
		if p.Extends != "" {
			line += fmt.Sprintf(" (hereda de %s)", p.Extends)
		}
		out = append(out, line)
	}
	return out
}

// Resolve returns the fully-merged gate map for a profile, walking the
// extends chain (child gates override parent gates). Returns gates and
// the human description.
func Resolve(name string) (map[string]manifest.Gate, string, error) {
	all, err := loadAll()
	if err != nil {
		return nil, "", err
	}
	var chain []*Profile
	seen := map[string]bool{}
	cur := name
	for cur != "" {
		p, ok := all[cur]
		if !ok {
			return nil, "", fmt.Errorf("perfil desconocido %q (disponibles: %s)", cur, strings.Join(Names(), ", "))
		}
		if seen[cur] {
			return nil, "", fmt.Errorf("herencia circular en perfil %q", name)
		}
		seen[cur] = true
		chain = append([]*Profile{p}, chain...) // parents first
		cur = p.Extends
	}
	merged := map[string]manifest.Gate{}
	for _, p := range chain {
		for gname, g := range p.Gates {
			merged[gname] = g
		}
	}
	return merged, all[name].Description, nil
}

// DetectStack inspects dir and returns the best-matching profile name.
// Content signals score higher than bare file presence so kmp-compose
// beats kmp, and deeper (more specific) profiles win ties.
func DetectStack(dir string) (string, error) {
	all, err := loadAll()
	if err != nil {
		return "", err
	}
	type match struct {
		name  string
		score int
		depth int
	}
	var best *match
	for _, p := range all {
		score := 0
		for _, f := range p.Detect.Files {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				score++
			}
		}
		if len(p.Detect.Files) > 0 && score == 0 {
			continue // required marker files absent
		}
		contentHit := len(p.Detect.AnyContent) == 0
		for _, sig := range p.Detect.AnyContent {
			raw, err := os.ReadFile(filepath.Join(dir, sig.Path))
			if err == nil && strings.Contains(string(raw), sig.Contains) {
				contentHit = true
				score += 2
			}
		}
		if !contentHit {
			continue
		}
		depth := 0
		for cur := p; cur.Extends != ""; cur = all[cur.Extends] {
			depth++
		}
		m := match{p.Name, score, depth}
		if best == nil || m.score > best.score || (m.score == best.score && m.depth > best.depth) {
			best = &m
		}
	}
	if best == nil {
		return "", fmt.Errorf("no se pudo detectar el stack en %s (perfiles: %s); usa 'hoom init --profile <nombre>'", dir, strings.Join(Names(), ", "))
	}
	return best.name, nil
}
