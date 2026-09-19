// Package finding turns review findings into ARTIFACTS: append-only
// records under .hoom/findings/, bound to the tree fingerprint at the
// moment of discovery, with an explicit lifecycle (abierto -> corregido |
// refutado). Two files per lifecycle, both immutable: the finding itself
// and, if closed, one terminal resolution carrying mandatory evidence —
// nobody closes a finding without saying why, and nobody edits history.
// Findings travel in Git like approvals and they are narration (qualified
// and auditable): recording one never changes the candidate fingerprint, and
// check never reads them. A project may opt in (hoom.yaml findings.block_on)
// to let verify COUNT them through the synthetic gate findings_open — it
// counts states, it never judges the evidence of a resolution.
package finding

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	// Task: the task the finding belongs to. The cockpit uses it to put a
	// finding on its card, and `verify --spec` uses it to scope the
	// findings_open gate to the spec's task; check never reads it.
	Task string `json:"task,omitempty"`
}

// Draft is what a new finding says before hoom stamps it (id, time,
// fingerprint).
type Draft struct {
	Severity, Lens, File, Description, Author string
	Task                                      string // slug de la tarea; "" = sin tarea
}

// taskRe is the slug shape `hoom task start` accepts. A finding validates the
// SHAPE only: from inside a task's worktree the task registry lives in
// another directory, and a finding outlives the worktree `hoom task done`
// removes.
var taskRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

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

// Add records a new immutable finding bound to the current tree fingerprint,
// with no task.
func Add(root, base, severity, lens, file, description, author string) (Finding, error) {
	return Register(root, base, Draft{Severity: severity, Lens: lens, File: file,
		Description: description, Author: author})
}

// Register records a new immutable finding bound to the current tree
// fingerprint and, when the draft names one, to its task. An invalid task is
// an error and writes nothing.
func Register(root, base string, d Draft) (Finding, error) {
	severity, lens, file, description, author := d.Severity, d.Lens, d.File, d.Description, d.Author
	task := strings.TrimSpace(d.Task)
	if task != "" && !taskRe.MatchString(task) {
		return Finding{}, fmt.Errorf("tarea invalida %q: usa el slug de la tarea (minusculas, numeros y guiones)", d.Task)
	}
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
		Task:        task,
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
		prev, motivo := parseResolution(raw)
		if motivo != "" {
			// The binary never rewrites append-only evidence: repairing or
			// deleting a broken record is a human act, visible in Git.
			return Resolution{}, fmt.Errorf("la resolucion existente de %s es invalida (%s): el hallazgo sigue ABIERTO.\nAccion: repara o borra a mano %s (el cambio queda en el diff de Git) y vuelve a resolver",
				id, motivo, resPath(root, id))
		}
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

// parseResolution reads a resolution record and says why it does not close
// its finding ("" = it does). This is the single definition of "closed": a
// terminal state hoom knows and evidence that is not blank. A record written
// by hand without evidence, or with a state nobody defined, closes nothing.
func parseResolution(raw []byte) (Resolution, string) {
	var r Resolution
	if json.Unmarshal(raw, &r) != nil || r.FindingID == "" {
		return r, "ilegible"
	}
	r.As = strings.ToLower(strings.TrimSpace(r.As))
	if !validStates[r.As] {
		return r, fmt.Sprintf("estado %q (corregido|refutado)", r.As)
	}
	if strings.TrimSpace(r.Evidence) == "" {
		return r, "sin evidencia"
	}
	return r, ""
}

// scanned is one read of .hoom/findings/: the findings that parsed, the
// finding files that did not (a gate cannot know what they say, so it fails
// closed on them) and the warnings about resolutions that close nothing.
type scanned struct {
	items    []Item
	broken   []string // "<archivo>: <motivo>" de hallazgos ilegibles
	warnings []string // resoluciones ilegibles o invalidas
}

func scan(root string) (scanned, error) {
	var out scanned
	entries, err := os.ReadDir(dir(root))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}

	resolutions := map[string]*Resolution{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".res.json") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir(root), e.Name()))
		if rerr != nil {
			out.warnings = append(out.warnings, fmt.Sprintf("resolucion ilegible %s: %v", e.Name(), rerr))
			continue
		}
		r, motivo := parseResolution(raw)
		switch {
		case motivo == "ilegible":
			out.warnings = append(out.warnings, "resolucion ilegible "+e.Name())
		case motivo != "":
			out.warnings = append(out.warnings, fmt.Sprintf("resolucion invalida %s: %s; el hallazgo %s sigue abierto",
				e.Name(), motivo, r.FindingID))
		default:
			res := r
			resolutions[r.FindingID] = &res
		}
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".res.json") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir(root), name))
		if rerr != nil {
			out.broken = append(out.broken, fmt.Sprintf("%s: %v", name, rerr))
			continue
		}
		var f Finding
		if json.Unmarshal(raw, &f) != nil || f.ID == "" {
			out.broken = append(out.broken, name+": JSON ilegible o sin id")
			continue
		}
		it := Item{Finding: f, Status: StatusOpen}
		if r := resolutions[f.ID]; r != nil {
			it.Status = r.As
			it.Resolution = r
		}
		out.items = append(out.items, it)
	}
	sort.Slice(out.items, func(i, j int) bool { return out.items[i].CreatedAt.Before(out.items[j].CreatedAt) })
	return out, nil
}

// List derives the current state of every finding, oldest first. Corrupt
// files are skipped and reported as warnings — never fatal, like verdicts.
// A resolution that closes nothing (unreadable, no evidence, unknown state)
// leaves its finding OPEN and says so in a warning.
func List(root, base string, openOnly bool) ([]Item, []string, error) {
	sc, err := scan(root)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	for _, b := range sc.broken {
		warnings = append(warnings, "hallazgo ilegible "+b)
	}
	warnings = append(warnings, sc.warnings...)
	if len(sc.items) == 0 {
		return []Item{}, warnings, nil
	}

	current := gitx.Snapshot(root, base).ChangeFingerprint
	items := []Item{}
	for _, it := range sc.items {
		if it.Fingerprint != "" && current != "" && it.Fingerprint != current {
			it.CodeChanged = true // a re-verificar, no a asumir
		}
		if openOnly && it.Status != StatusOpen {
			continue
		}
		items = append(items, it)
	}
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
