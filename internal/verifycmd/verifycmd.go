// Package verifycmd composes one verification run end to end: git snapshot,
// optional spec gates, deterministic gates, verdict finalized and persisted.
// It exists so the CLI verb and the Studio's POST /api/verify execute the
// EXACT same function — the Studio never grows a second brain.
package verifycmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Options mirror the verify verb's flags.
type Options struct {
	Full  bool
	Gates []string // empty = all
	Spec  string   // spec path; adds spec_lint + spec_trace
}

// Run executes the verification and writes the verdict. It returns the
// finalized verdict and the path of its artifact.
func Run(m *manifest.Manifest, opt Options) (*verdict.Verdict, string, error) {
	git := gitx.Snapshot(m.Dir, m.BaseBranch)

	// Live narration: best-effort by contract — a broken cache never touches
	// the run. The file is truncated per run; artifacts remain the history.
	lw := live.NewWriter(m.Dir)
	defer lw.Close()

	gopt := gates.Options{Full: opt.Full, Emit: lw.Emit}
	if len(opt.Gates) > 0 {
		gopt.Only = map[string]bool{}
		for _, g := range opt.Gates {
			if g = strings.TrimSpace(g); g != "" {
				gopt.Only[g] = true
			}
		}
	}

	total := len(m.Gates)
	if opt.Spec != "" {
		total += 2 // spec_lint + spec_trace
	}
	lw.Emit(live.Event{Kind: live.KindVerifyStart, Gates: total, Spec: opt.Spec})

	var results []verdict.GateResult
	if opt.Spec != "" {
		for _, name := range []string{"spec_lint", "spec_trace"} {
			lw.Emit(live.Event{Kind: live.KindGateStart, Gate: name, Scope: "spec"})
		}
		specResults := spec.Gates(m.Dir, opt.Spec)
		for _, r := range specResults {
			lw.Emit(live.Event{Kind: live.KindGateEnd, Gate: r.Name, Status: r.Status,
				DurationMS: r.DurationMS, ExitCode: r.ExitCode})
		}
		results = append(results, specResults...)
	}
	results = append(results, gates.Run(m, git, gopt)...)

	v := &verdict.Verdict{
		Project: m.Project,
		Profile: m.Profile,
		Policy:  m.Policy,
		Git:     git,
		Gates:   results,
		Spec:    opt.Spec,
	}
	if len(gopt.Only) > 0 {
		// A --gate scoped run is a diagnostic, never a reference: mark it so
		// check/task done skip it by construction (no verdict fraud).
		only := make([]string, 0, len(gopt.Only))
		for g := range gopt.Only {
			only = append(only, g)
		}
		sort.Strings(only)
		v.Partial = true
		v.Notes = append(v.Notes, fmt.Sprintf(
			"veredicto PARCIAL (--gate %s): diagnostico; 'hoom check' y 'hoom task done' no lo usan como referencia",
			strings.Join(only, ",")))
	}
	v.Finalize()
	path, err := verdict.Write(m.Dir, v)
	if err != nil {
		return nil, "", fmt.Errorf("no se pudo escribir el veredicto: %w", err)
	}
	lw.Emit(live.Event{Kind: live.KindVerifyEnd, Verdict: v.Verdict, VerdictID: v.ID, Partial: v.Partial})
	return v, path, nil
}
