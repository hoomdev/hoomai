// Package checkcmd implements the deterministic drift check as a reusable
// result: does the CURRENT working tree match what the latest verdict
// verified, and is that verdict green? The CLI renders it as text or JSON
// and hoom serve embeds it in /api/status — one brain, three skins.
package checkcmd

import (
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Reason values for a failing check.
const (
	ReasonNoVerdict  = "no-verdict"
	ReasonRedVerdict = "red-verdict"
	ReasonDrift      = "drift"
)

// Result is the outcome of one check, JSON-ready for `hoom check --json`
// and for the Studio.
type Result struct {
	OK                 bool   `json:"ok"`
	VerdictID          string `json:"verdict_id,omitempty"`
	LastVerdict        string `json:"last_verdict,omitempty"` // green | red
	FingerprintMatch   bool   `json:"fingerprint_match"`
	FingerprintNow     string `json:"fingerprint_now,omitempty"`
	FingerprintVerdict string `json:"fingerprint_verdict,omitempty"`
	Reason             string `json:"reason,omitempty"` // no-verdict | red-verdict | drift
	Action             string `json:"action,omitempty"` // exact next command, in-band
}

// ExitCode mirrors the strict policy: a failing check exits 1 in every mode.
func (r Result) ExitCode() int {
	if r.OK {
		return 0
	}
	return 1
}

// Run evaluates the check for the project rooted at root against base.
func Run(root, base string) (Result, error) {
	git := gitx.Snapshot(root, base)
	all, err := verdict.LoadAll(root)
	if err != nil {
		return Result{}, err
	}
	res := Result{FingerprintNow: git.ChangeFingerprint}
	if len(all) == 0 {
		res.Reason = ReasonNoVerdict
		res.Action = "ejecuta 'hoom verify'"
		return res, nil
	}
	last := all[len(all)-1]
	res.VerdictID = last.ID
	res.LastVerdict = last.Verdict
	res.FingerprintVerdict = last.Git.ChangeFingerprint
	if last.Verdict != "green" {
		res.Reason = ReasonRedVerdict
		res.Action = "corrige y ejecuta 'hoom verify'"
		return res, nil
	}
	if last.Git.ChangeFingerprint != git.ChangeFingerprint {
		res.Reason = ReasonDrift
		res.Action = "ejecuta 'hoom verify' de nuevo sobre el estado actual"
		return res, nil
	}
	res.OK = true
	res.FingerprintMatch = true
	return res, nil
}
