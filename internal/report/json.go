// JSON view of the verdict history: the same information History renders as
// text, structured for `hoom report --json` and the Studio's /api/report.
// Both consumers call JSONBytes so the representations cannot diverge.
package report

import (
	"encoding/json"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// RunSummary is one verdict, without per-gate evidence (that lives in the
// verdict file itself, served by /api/verdicts/{id}).
type RunSummary struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Verdict   string          `json:"verdict"`
	Branch    string          `json:"branch,omitempty"`
	Commit    string          `json:"commit,omitempty"`
	Spec      string          `json:"spec,omitempty"`
	Summary   verdict.Summary `json:"summary"`
}

// GateTrend is the full-history pass-rate of one gate. Skipped and absent
// runs do not count against the rate, matching the text trend.
type GateTrend struct {
	Pass       int    `json:"pass"`
	Total      int    `json:"total"`
	PassRate   int    `json:"pass_rate"` // percent, 0-100
	LastStatus string `json:"last_status"`
}

// JSON is the machine-readable report.
type JSON struct {
	Total int                  `json:"total"`
	Runs  []RunSummary         `json:"runs"`  // newest first, capped at n
	Gates map[string]GateTrend `json:"gates"` // trend over the FULL history
}

// Build assembles the report from all verdicts (oldest first, as LoadAll
// returns them), keeping the n most recent runs.
func Build(all []*verdict.Verdict, n int) JSON {
	out := JSON{Total: len(all), Runs: []RunSummary{}, Gates: map[string]GateTrend{}}
	if n > len(all) {
		n = len(all)
	}
	if n < 0 {
		n = 0
	}
	for i := len(all) - 1; i >= len(all)-n; i-- {
		v := all[i]
		out.Runs = append(out.Runs, RunSummary{
			ID:        v.ID,
			CreatedAt: v.CreatedAt,
			Verdict:   v.Verdict,
			Branch:    v.Git.Branch,
			Commit:    v.Git.Commit,
			Spec:      v.Spec,
			Summary:   v.Summary,
		})
	}
	for _, v := range all {
		for _, g := range v.Gates {
			if g.Status == verdict.StatusSkipped || g.Status == verdict.StatusAbsent {
				continue
			}
			t := out.Gates[g.Name]
			t.Total++
			if g.Status == verdict.StatusPass {
				t.Pass++
			}
			t.LastStatus = g.Status // all is oldest-first: the last write wins
			if t.Total > 0 {
				t.PassRate = 100 * t.Pass / t.Total
			}
			out.Gates[g.Name] = t
		}
	}
	return out
}

// JSONBytes renders the report exactly as both the CLI and the Studio emit it.
func JSONBytes(all []*verdict.Verdict, n int) ([]byte, error) {
	return json.MarshalIndent(Build(all, n), "", "  ")
}
