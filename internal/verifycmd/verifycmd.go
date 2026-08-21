// Package verifycmd composes one verification run end to end: git snapshot,
// optional spec gates, deterministic gates, verdict finalized and persisted.
// It exists so the CLI verb and the Studio's POST /api/verify execute the
// EXACT same function — the Studio never grows a second brain.
package verifycmd

import (
	"fmt"
	"strings"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gitx"
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

	gopt := gates.Options{Full: opt.Full}
	if len(opt.Gates) > 0 {
		gopt.Only = map[string]bool{}
		for _, g := range opt.Gates {
			if g = strings.TrimSpace(g); g != "" {
				gopt.Only[g] = true
			}
		}
	}

	var results []verdict.GateResult
	if opt.Spec != "" {
		results = append(results, spec.Gates(m.Dir, opt.Spec)...)
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
	v.Finalize()
	path, err := verdict.Write(m.Dir, v)
	if err != nil {
		return nil, "", fmt.Errorf("no se pudo escribir el veredicto: %w", err)
	}
	return v, path, nil
}
