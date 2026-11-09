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
	ReasonNoVerdict   = "no-verdict"
	ReasonOnlyPartial = "solo-parciales" // only --gate diagnostics exist: not a reference
	ReasonRedVerdict  = "red-verdict"
	ReasonDrift       = "drift"
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

// Motivo is why a failing check is red, in the words of `hoom check` (what
// its line says after "hoom check: ROJO - ", without the period that ends
// it and what follows): one source for the CLI and for hoom tablero's [1]
// ("check ROJO: <motivo>"). By Reason:
//
//   - ReasonNoVerdict: "no existe ningun veredicto";
//   - ReasonOnlyPartial: "solo hay veredictos PARCIALES (--gate), que no
//     son referencia";
//   - ReasonRedVerdict: "el ultimo veredicto (<VerdictID>) es rojo";
//   - ReasonDrift: "el arbol cambio despues del ultimo veredicto verde
//     (<VerdictID>)";
//
// and "" for a check that is OK or a Reason it does not know.
func Motivo(r Result) string {
	if r.OK {
		return ""
	}
	switch r.Reason {
	case ReasonNoVerdict:
		return "no existe ningun veredicto"
	case ReasonOnlyPartial:
		return "solo hay veredictos PARCIALES (--gate), que no son referencia"
	case ReasonRedVerdict:
		return "el ultimo veredicto (" + r.VerdictID + ") es rojo"
	case ReasonDrift:
		return "el arbol cambio despues del ultimo veredicto verde (" + r.VerdictID + ")"
	}
	return ""
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
	// The reference is the newest COMPLETE verdict: a --gate diagnostic can
	// never become the reference, neither for a false green nor a false red.
	last := verdict.LatestComplete(all)
	if last == nil {
		res.Reason = ReasonOnlyPartial
		res.Action = "ejecuta 'hoom verify' completo (sin --gate): los veredictos parciales no son referencia"
		return res, nil
	}
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
