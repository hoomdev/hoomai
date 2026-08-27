// Package finding turns review findings into ARTIFACTS: append-only
// records under .hoom/findings/, bound to the tree fingerprint at the
// moment of discovery, with an explicit lifecycle (abierto -> corregido |
// refutado). Two files per lifecycle, both immutable: the finding itself
// and, if closed, one terminal resolution carrying mandatory evidence —
// nobody closes a finding without saying why, and nobody edits history.
// Findings travel in Git like approvals, but they are narration (qualified
// and auditable) — never gate evidence: verify and check do not read them,
// and recording one never changes the candidate fingerprint.
package finding

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// Severities and terminal states.
var (
	validSeverities = map[string]bool{"low": true, "medium": true, "high": true}
	validStates     = map[string]bool{"corregido": true, "refutado": true}
)

// Derived status values.
const (
	StatusOpen      = "abierto"
	StatusCorrected = "corregido"
	StatusRefuted   = "refutado"
)

// Finding is the immutable record of one review finding.
type Finding struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	Severity    string    `json:"severity"` // low | medium | high
	Lens        string    `json:"lens"`
	File        string    `json:"file,omitempty"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	Fingerprint string    `json:"fingerprint,omitempty"` // huella del arbol al encontrarlo
}

// Resolution is the single terminal transition of a finding.
type Resolution struct {
	FindingID  string    `json:"finding_id"`
	As         string    `json:"as"` // corregido | refutado
	Evidence   string    `json:"evidence"`
	Author     string    `json:"author"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// Item is the derived view: finding + state, for list/API.
type Item struct {
	Finding
	Status      string      `json:"status"` // abierto | corregido | refutado
	Resolution  *Resolution `json:"resolution,omitempty"`
	CodeChanged bool        `json:"code_changed"` // la huella ya no coincide con el arbol
}

func dir(root string) string { return filepath.Join(root, ".hoom", "findings") }

func findingPath(root, id string) string { return filepath.Join(dir(root), id+".json") }
func resPath(root, id string) string     { return filepath.Join(dir(root), id+".res.json") }

func gitUser(root string) string {
	get := func(key string) string {
		cmd := exec.Command("git", "config", key)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	name, email := get("user.name"), get("user.email")
	switch {
	case name != "" && email != "":
		return name + " <" + email + ">"
	case name != "":
		return name
	case email != "":
		return email
	}
	return "desconocido"
}

func newID() string {
	raw := make([]byte, 3)
	rand.Read(raw)
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(raw)
}

// Add records a new immutable finding bound to the current tree fingerprint.
func Add(root, base, severity, lens, file, description, author string) (Finding, error) {
	severity = strings.ToLower(strings.TrimSpace(severity))
	if !validSeverities[severity] {
		return Finding{}, fmt.Errorf("severidad invalida %q (low|medium|high)", severity)
	}
	if strings.TrimSpace(description) == "" {
		return Finding{}, fmt.Errorf("la descripcion del hallazgo no puede ser vacia")
	}
	if strings.TrimSpace(author) == "" {
		author = gitUser(root)
	}
	f := Finding{
		ID:          newID(),
		CreatedAt:   time.Now().UTC(),
		Severity:    severity,
		Lens:        strings.TrimSpace(lens),
		File:        strings.TrimSpace(file),
		Description: strings.TrimSpace(description),
		Author:      author,
		Fingerprint: gitx.Snapshot(root, base).ChangeFingerprint,
	}
	if err := os.MkdirAll(dir(root), 0o755); err != nil {
		return Finding{}, err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return Finding{}, err
	}
	if err := os.WriteFile(findingPath(root, f.ID), raw, 0o644); err != nil {
		return Finding{}, err
	}
	return f, nil
}

// Resolve writes the single terminal transition. Mandatory evidence: the
// binary refuses to close a finding without it. A resolved finding admits
// no second transition — reopening is a NEW finding citing the old one.
func Resolve(root, id, as, evidence, author string) (Resolution, error) {
	as = strings.ToLower(strings.TrimSpace(as))
	if !validStates[as] {
		return Resolution{}, fmt.Errorf("estado invalido %q (corregido|refutado)", as)
	}
	if strings.TrimSpace(evidence) == "" {
		return Resolution{}, fmt.Errorf("falta --evidence: nadie cierra un hallazgo sin decir por que")
	}
	if _, err := os.Stat(findingPath(root, id)); err != nil {
		return Resolution{}, fmt.Errorf("hallazgo no encontrado: %s", id)
	}
	if raw, err := os.ReadFile(resPath(root, id)); err == nil {
		var prev Resolution
		_ = json.Unmarshal(raw, &prev)
		return Resolution{}, fmt.Errorf("el hallazgo %s ya esta resuelto como %q; reabrir = un hallazgo NUEVO que cite a este", id, prev.As)
	}
	if strings.TrimSpace(author) == "" {
		author = gitUser(root)
	}
	r := Resolution{
		FindingID:  id,
		As:         as,
		Evidence:   strings.TrimSpace(evidence),
		Author:     author,
		ResolvedAt: time.Now().UTC(),
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Resolution{}, err
	}
	if err := os.WriteFile(resPath(root, id), raw, 0o644); err != nil {
		return Resolution{}, err
	}
	return r, nil
}

// List derives the current state of every finding, oldest first. Corrupt
// files are skipped and reported as warnings — never fatal, like verdicts.
func List(root, base string, openOnly bool) ([]Item, []string, error) {
	entries, err := os.ReadDir(dir(root))
	if os.IsNotExist(err) {
		return []Item{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	current := gitx.Snapshot(root, base).ChangeFingerprint

	resolutions := map[string]*Resolution{}
	var warnings []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".res.json") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir(root), e.Name()))
		if rerr != nil {
			warnings = append(warnings, fmt.Sprintf("resolucion ilegible %s: %v", e.Name(), rerr))
			continue
		}
		var r Resolution
		if json.Unmarshal(raw, &r) != nil || r.FindingID == "" {
			warnings = append(warnings, "resolucion ilegible "+e.Name())
			continue
		}
		res := r
		resolutions[r.FindingID] = &res
	}

	items := []Item{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".res.json") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir(root), name))
		if rerr != nil {
			warnings = append(warnings, fmt.Sprintf("hallazgo ilegible %s: %v", name, rerr))
			continue
		}
		var f Finding
		if json.Unmarshal(raw, &f) != nil || f.ID == "" {
			warnings = append(warnings, "hallazgo ilegible "+name)
			continue
		}
		it := Item{Finding: f, Status: StatusOpen}
		if r := resolutions[f.ID]; r != nil {
			it.Status = r.As
			it.Resolution = r
		}
		if f.Fingerprint != "" && current != "" && f.Fingerprint != current {
			it.CodeChanged = true // a re-verificar, no a asumir
		}
		if openOnly && it.Status != StatusOpen {
			continue
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, warnings, nil
}

// ListJSON renders exactly what both the CLI and the Studio emit.
type ListView struct {
	Warnings []string `json:"warnings"`
	Findings []Item   `json:"findings"`
}

func JSONBytes(root, base string, openOnly bool) ([]byte, error) {
	items, warnings, err := List(root, base, openOnly)
	if err != nil {
		return nil, err
	}
	if warnings == nil {
		warnings = []string{}
	}
	return json.MarshalIndent(ListView{Warnings: warnings, Findings: items}, "", "  ")
}
