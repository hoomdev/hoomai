// Package gates executes the deterministic verification layer. No LLM is
// ever involved here: trust lives in tools with exit codes, not in agent
// claims. Missing tools fail closed (error => red under strict policy);
// declared absence (no command) is reported loudly but is not red.
package gates

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/oceansbits/hoomai/internal/gitx"
	"github.com/oceansbits/hoomai/internal/manifest"
	"github.com/oceansbits/hoomai/internal/verdict"
)

// Options control a verify run.
type Options struct {
	Full     bool            // ignore diff scoping, run full commands
	Only     map[string]bool // if non-empty, run only these gates (others => skipped)
	Timeout  time.Duration   // per-gate timeout (0 = default)
	TailSize int             // bytes of output tail kept in the verdict
}

const (
	defaultTimeout = 30 * time.Minute
	defaultTail    = 4000
)

// Run executes every gate in canonical order and returns results.
func Run(m *manifest.Manifest, git gitx.Info, opt Options) []verdict.GateResult {
	if opt.Timeout == 0 {
		opt.Timeout = defaultTimeout
	}
	if opt.TailSize == 0 {
		opt.TailSize = defaultTail
	}
	var results []verdict.GateResult
	for _, name := range m.SortedGateNames() {
		results = append(results, runOne(name, m.Gates[name], m, git, opt))
	}
	return results
}

func runOne(name string, g manifest.Gate, m *manifest.Manifest, git gitx.Info, opt Options) verdict.GateResult {
	res := verdict.GateResult{
		Name:     name,
		Required: g.IsRequired(),
		Partial:  g.Partial,
		Scope:    "none",
	}
	if len(opt.Only) > 0 && !opt.Only[name] {
		res.Status = verdict.StatusSkipped
		return res
	}
	cmdStr, scope := selectCommand(g, git, opt)
	if strings.TrimSpace(cmdStr) == "" {
		res.Status = verdict.StatusAbsent // declared degradation
		res.Notes = g.Notes
		return res
	}
	res.Command = cmdStr
	res.Scope = scope

	ctx, cancel := context.WithTimeout(context.Background(), opt.Timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = m.Dir
	out, err := cmd.CombinedOutput()
	res.DurationMS = time.Since(start).Milliseconds()
	res.OutputTail = tail(string(out), opt.TailSize)

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res.Status = verdict.StatusError
		res.ExitCode = -1
		res.Notes = fmt.Sprintf("timeout tras %s", opt.Timeout)
	case err == nil:
		res.Status = verdict.StatusPass
	default:
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			res.Status = verdict.StatusFail
		} else {
			res.ExitCode = -1
			res.Status = verdict.StatusError // could not even start — fail closed
			res.Notes = err.Error()
		}
	}
	return res
}

// selectCommand picks diff_cmd when a diff scope exists and --full was not
// requested, then substitutes template variables:
//
//	{base}     base branch
//	{files}    space-separated changed files
//	{packages} ./dir style package list derived from changed .go files
func selectCommand(g manifest.Gate, git gitx.Info, opt Options) (string, string) {
	cmdStr, scope := g.CmdStr(), "full"
	if !opt.Full && g.DiffCmd != "" && git.IsRepo && len(git.ChangedFiles) > 0 {
		cmdStr, scope = g.DiffCmd, "diff"
	}
	if cmdStr == "" {
		return "", "none"
	}
	pkgs := gitx.GoPackages(git.ChangedFiles)
	if scope == "diff" {
		// A diff command whose variables expand to nothing falls back to the
		// full command rather than executing a broken line.
		if strings.Contains(cmdStr, "{packages}") && len(pkgs) == 0 {
			cmdStr, scope = g.CmdStr(), "full"
		}
		if strings.Contains(cmdStr, "{files}") && len(git.ChangedFiles) == 0 {
			cmdStr, scope = g.CmdStr(), "full"
		}
	}
	cmdStr = strings.ReplaceAll(cmdStr, "{base}", git.Base)
	cmdStr = strings.ReplaceAll(cmdStr, "{files}", strings.Join(git.ChangedFiles, " "))
	cmdStr = strings.ReplaceAll(cmdStr, "{packages}", strings.Join(pkgs, " "))
	return cmdStr, scope
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "...[truncado]...\n" + s[len(s)-n:]
}
