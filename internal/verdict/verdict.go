// Package verdict defines hoomAI's core artifact: the verification verdict.
// Verdicts are append-only JSON files under .hoom/verdicts/ — plain,
// diffable, merge-conflict-free by construction (one file per run, unique
// name), versioned in Git so history travels with the project across
// machines and CI. Any local index (SQLite) is a regenerable cache derived
// from these files, never the source of truth.
package verdict

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oceansbits/hoomai/internal/gitx"
)

const SchemaID = "hoom.verdict/v1"

// Gate status values.
const (
	StatusPass    = "pass"    // gate ran, exit 0
	StatusFail    = "fail"    // gate ran, non-zero exit
	StatusError   = "error"   // configured but could not run (missing tool, timeout) — fail closed
	StatusAbsent  = "absent"  // no command configured — declared degradation, reported loudly
	StatusSkipped = "skipped" // excluded by --gate selection
)

// GateResult is the outcome of one gate execution.
type GateResult struct {
	Name       string `json:"name"`
	Required   bool   `json:"required"`
	Status     string `json:"status"`
	Scope      string `json:"scope"` // full | diff | none
	Command    string `json:"command,omitempty"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	OutputTail string `json:"output_tail,omitempty"`
	Partial    string `json:"partial,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// Verdict is one immutable verification run.
type Verdict struct {
	Schema    string       `json:"schema"`
	ID        string       `json:"id"`
	CreatedAt time.Time    `json:"created_at"`
	Project   string       `json:"project"`
	Profile   string       `json:"profile"`
	Policy    string       `json:"policy"`
	Git       gitx.Info    `json:"git"`
	Gates     []GateResult `json:"gates"`
	Summary   Summary      `json:"summary"`
	Verdict   string       `json:"verdict"` // green | red
	Spec      string       `json:"spec,omitempty"`
	Notes     []string     `json:"notes,omitempty"`
}

type Summary struct {
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Error   int `json:"error"`
	Absent  int `json:"absent"`
	Skipped int `json:"skipped"`
}

// Finalize computes the summary and the strict verdict: any fail or error
// on a required gate turns it red. Declared absence is never silent (it is
// counted and rendered loudly) but is not red; configured-but-broken is.
func (v *Verdict) Finalize() {
	s := Summary{}
	red := false
	for _, g := range v.Gates {
		switch g.Status {
		case StatusPass:
			s.Pass++
		case StatusFail:
			s.Fail++
			if g.Required {
				red = true
			}
		case StatusError:
			s.Error++
			if g.Required {
				red = true
			}
		case StatusAbsent:
			s.Absent++
		case StatusSkipped:
			s.Skipped++
		}
	}
	v.Summary = s
	v.Verdict = "green"
	if red {
		v.Verdict = "red"
	}
}

// Write persists the verdict as .hoom/verdicts/<timestamp>_<hash>.json.
// Timestamp + content hash guarantees unique, collision-free, append-only
// files even across machines and CI running concurrently.
func Write(projectDir string, v *Verdict) (string, error) {
	v.Schema = SchemaID
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	v.ID = v.CreatedAt.Format("2006-01-02T15-04-05Z") + "_" + hex.EncodeToString(sum[:])[:8]
	raw, _ = json.MarshalIndent(v, "", "  ")

	dir := filepath.Join(projectDir, ".hoom", "verdicts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, v.ID+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// LoadAll reads every verdict in the project, oldest first. This scan IS
// the regenerable index for v1; a SQLite cache can be layered later
// without changing the source of truth.
func LoadAll(projectDir string) ([]*Verdict, error) {
	dir := filepath.Join(projectDir, ".hoom", "verdicts")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Verdict
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var v Verdict
		if err := json.Unmarshal(raw, &v); err != nil {
			fmt.Fprintf(os.Stderr, "hoom: veredicto ilegible %s: %v\n", e.Name(), err)
			continue
		}
		out = append(out, &v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
