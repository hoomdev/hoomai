// Package verifycmd composes one verification run end to end: git snapshot,
// optional spec gates, deterministic gates, verdict finalized and persisted.
// It exists so the CLI verb and the Studio's POST /api/verify execute the
// EXACT same function — the Studio never grows a second brain.
package verifycmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gitajeno"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/materialgit"
	"github.com/hoomdev/hoomai/internal/ratchet"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Options mirror the verify verb's flags.
type Options struct {
	Full  bool
	Gates []string // empty = all
	Spec  string   // spec path, relative to the manifest's Dir or absolute; adds spec_lint + spec_trace
	JSON  bool     // emit the verdict as JSON (rendering is the caller's job)
	// Sobre (contencion-del-writer) is the option only the verify of the
	// envelope of a role that executes (agentcmd.Run) and the pilot's
	// cerrar-hallazgos use; `hoom verify` and POST /api/verify never set it
	// (ParseArgs never does). nil = as today.
	Sobre *Sobre
}

// Sobre is the verify of an envelope of a role that executes, or of the
// pilot's cerrar-hallazgos (Options.Sobre). With it:
//   - in a project whose hoom.yaml (Root's, manifest.LoadRoles) has the
//     section `encierro`, each gate runs enclosed (encierro), with the view of
//     role Rol in the manifest's tree and the cache `verify`
//     (encierro.Cache(Root, Task, encierro.QuienVerify)), nested user
//     namespaces allowed and the gate's gracia kept; the gates of
//     encierro.gates_sin_encierro run as today. Without the section the gates
//     run as today;
//   - with the section or without it, the git material of the manifest's tree
//     (materialgit.TomarConTope, the photograph of agentcmd.FotoMaterialGit)
//     is photographed before the first gate and after the last,
//     gates_sin_encierro included. When it changed, the verdict carries one
//     more gate, GateMaterialDeGit; git-ajeno too (ronda 1, aclaracion 20),
//     with the gate GateGitAjeno.
//
// The gates' view adds no provider's state: they have no provider. Their
// level is the system's (encierro.Nivel without a provider): ninguno without
// Task. It does not judge encierro.exigir: whoever calls it already did.
type Sobre struct {
	// Root is the project's main checkout: its hoom.yaml's policy, and where
	// the cache lives.
	Root string
	// Task is the task's slug.
	Task string
	// Rol is the role of the envelope whose view the gates get: "writer" in
	// cerrar-hallazgos.
	Rol string
	// Ciego (ronda 1, a8ef36) is the test-writer's blind tree, when Rol is
	// the test-writer: its gates write only there, when it still exists, and
	// in the cache `verify`; the manifest's tree (the implementation) stays
	// read-only. "" with the test-writer = no blind tree left.
	Ciego string
}

// GateGitAjeno (ronda 1, aclaracion 20) is the gate a verify with
// Options.Sobre adds to its verdict when git changed outside the task's
// branch during its gates (agentcmd.RuleGitAjeno, photographed before the
// first gate and after the last, like the git material): required, status
// fail, scope full, its output the paths agentcmd.CambiosGitAjeno names, one
// per line. The verdict is then red, and no reader of verdicts takes it as
// green.
const GateGitAjeno = "git_ajeno"

// GateMaterialDeGit is the gate a verify with Options.Sobre adds to its
// verdict when the git material of its tree changed during its gates:
// required, status fail, scope full, its output (OutputTail) the paths that
// changed, one per line, as agentcmd.RuleMaterialDeGit names them. The
// verdict is then red, and no reader of verdicts (verdict.LatestComplete,
// hoom task done, check, the review) takes it as green. A verify whose
// material did not change has no such gate.
const GateMaterialDeGit = "material_de_git"

// Run executes the verification and writes the verdict. It returns the
// finalized verdict and the path of its artifact.
//
// It validates the gate selection against the manifest BEFORE anything else:
// a request hoom does not understand produces no verdict, no live events and
// no ratchet measurement — it produces a *cliargs.UsageError.
func Run(m *manifest.Manifest, opt Options) (*verdict.Verdict, string, error) {
	if err := validarGates(m, opt.Gates); err != nil {
		return nil, "", err
	}
	git := gitx.Snapshot(m.Dir, m.BaseBranch)
	// a relative spec is the project's (the verdict keeps it as given): read
	// from m.Dir, so a caller in another directory verifies the same file
	specFile := opt.Spec
	if specFile != "" && !filepath.IsAbs(specFile) {
		specFile = filepath.Join(m.Dir, specFile)
	}

	// Live narration: best-effort by contract — a broken cache never touches
	// the run. The file is truncated per run; artifacts remain the history.
	lw := live.NewWriter(m.Dir)
	defer lw.Close()

	gopt := gates.Options{Full: opt.Full, Emit: lw.Emit}
	var antes materialgit.Foto
	if opt.Sobre != nil {
		plan, sin, err := planDelSobre(m, *opt.Sobre)
		if err != nil {
			return nil, "", err
		}
		gopt.Encierro, gopt.SinEncierro = plan, sin
	}
	if len(opt.Gates) > 0 {
		gopt.Only = map[string]bool{}
		for _, g := range opt.Gates {
			gopt.Only[g] = true // ya validado: el nombre existe tal cual en el manifiesto
		}
	}

	// The ratchet only lives in COMPLETE --full runs: partial runs never
	// touch the baseline, and scoped measurements are not comparable to it.
	var rf *ratchet.File
	var rerr error
	includeRatchet := false
	if opt.Full && len(gopt.Only) == 0 {
		rf, rerr = ratchet.Load(m.Dir)
		includeRatchet = rerr != nil || (rf != nil && len(rf.Metrics) > 0)
	}

	// findings_open runs when the project opted in, and — like the ratchet,
	// the other gate that configuration (not a flag) turns on — never in a
	// --gate diagnostic.
	blockOn := m.FindingsBlockOn()
	includeFindings := blockOn != "" && len(gopt.Only) == 0

	total := len(m.Gates)
	if opt.Spec != "" {
		total += 3 // spec_lint + spec_trace + spec_approved
	}
	if includeFindings {
		total++
	}
	if includeRatchet {
		total++
	}
	lw.Emit(live.Event{Kind: live.KindVerifyStart, Gates: total, Spec: opt.Spec})

	var results []verdict.GateResult
	if opt.Spec != "" {
		for _, name := range []string{"spec_lint", "spec_trace", "spec_approved"} {
			lw.Emit(live.Event{Kind: live.KindGateStart, Gate: name, Scope: "spec"})
		}
		specResults := append(spec.Gates(m.Dir, specFile), approvedGate(m.Dir, specFile))
		for _, r := range specResults {
			lw.Emit(live.Event{Kind: live.KindGateEnd, Gate: r.Name, Status: r.Status,
				DurationMS: r.DurationMS, ExitCode: r.ExitCode})
		}
		results = append(results, specResults...)
	}
	// Human and review state first (instant), then the tree, then the ratchet.
	if includeFindings {
		scope := "full"
		if opt.Spec != "" {
			scope = "spec"
		}
		lw.Emit(live.Event{Kind: live.KindGateStart, Gate: finding.GateName, Scope: scope})
		fres := finding.Gate(m.Dir, m.BaseBranch, blockOn, specFile)
		lw.Emit(live.Event{Kind: live.KindGateEnd, Gate: fres.Name, Status: fres.Status,
			DurationMS: fres.DurationMS, ExitCode: fres.ExitCode})
		results = append(results, fres)
	}
	var ajeno gitajeno.Foto
	if opt.Sobre != nil {
		antes = materialgit.TomarConTope(m.Dir, topeFotoMaterial)
		ajeno = gitajeno.Tomar(m.Dir, gitajeno.Tope, gitajeno.MaxLeido)
	}
	results = append(results, gates.Run(m, git, gopt)...)
	if opt.Sobre != nil {
		if cambios := materialgit.Cambios(antes, materialgit.TomarConTope(m.Dir, topeFotoMaterial)); len(cambios) > 0 {
			results = append(results, verdict.GateResult{Name: GateMaterialDeGit, Required: true, Status: verdict.StatusFail,
				Scope: "full", ExitCode: 1, OutputTail: strings.Join(cambios, "\n"),
				Notes: "el codigo que corrieron los gates cambio el material de git de la tarea"})
		}
		// git outside the task's branch, around the same gates (aclaracion 20)
		if cambios := gitajeno.Cambios(ajeno, gitajeno.Tope, gitajeno.MaxLeido); len(cambios) > 0 {
			results = append(results, verdict.GateResult{Name: GateGitAjeno, Required: true, Status: verdict.StatusFail,
				Scope: "full", ExitCode: 1, OutputTail: strings.Join(cambios, "\n"),
				Notes: "el codigo que corrieron los gates cambio git fuera de la rama de la tarea"})
		}
	}

	if includeRatchet {
		lw.Emit(live.Event{Kind: live.KindGateStart, Gate: "ratchet", Scope: "full"})
		var rres verdict.GateResult
		if rerr != nil {
			// configured but broken (corrupt baseline) = fail-closed
			rres = verdict.GateResult{Name: "ratchet", Required: true, Scope: "full",
				Status: verdict.StatusError, Notes: rerr.Error()}
		} else {
			rres = ratchet.Gate(m.Dir, rf)
		}
		lw.Emit(live.Event{Kind: live.KindGateEnd, Gate: "ratchet", Status: rres.Status,
			DurationMS: rres.DurationMS, ExitCode: rres.ExitCode})
		results = append(results, rres)
	}

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

// topeFotoMaterial bounds each photograph of the git material of a verify
// with Options.Sobre (aclaracion 28): one that does not end counts as
// changed.
const topeFotoMaterial = 5 * time.Second

// planDelSobre is the encierro of the gates of a verify with Sobre s: nil
// (as today) without the section `encierro` in s.Root's hoom.yaml, or with
// a level ninguno; otherwise the view of role s.Rol in the manifest's tree
// with the cache `verify`. sin are encierro.gates_sin_encierro.
func planDelSobre(m *manifest.Manifest, s Sobre) (*encierro.Plan, map[string]bool, error) {
	roles, err := manifest.LoadRoles(s.Root)
	if err != nil {
		return nil, nil, err
	}
	if roles.Encierro == nil {
		return nil, nil, nil
	}
	sin := map[string]bool{}
	for _, g := range roles.Encierro.GatesSinEncierro {
		sin[g] = true
	}
	info := encierro.Nivel(s.Task != "", nil)
	if info.Mecanismo != encierro.MecanismoBwrap {
		return nil, sin, nil
	}
	vista, err := encierro.VistaDe(encierro.Lugar{Principal: s.Root, Tarea: s.Task, Arbol: m.Dir, Quien: encierro.QuienVerify,
		Escribibles: roles.Encierro.Escribibles, Perfil: m.Profile})
	if err != nil {
		return nil, nil, err
	}
	return &encierro.Plan{Info: info, Vista: vista}, sin, nil
}

// approvedGate enforces the human control point: a spec bound to a verify
// run must carry a CURRENT approval (content hash). The contract always
// demanded it; now the binary does too.
func approvedGate(root, specPath string) verdict.GateResult {
	start := time.Now()
	res := verdict.GateResult{Name: "spec_approved", Required: true, Scope: "spec"}
	state, rec, err := approval.Status(root, specPath)
	res.DurationMS = time.Since(start).Milliseconds()
	switch {
	case err != nil:
		res.Status = verdict.StatusError // fail-closed
		res.Notes = err.Error()
	case state == approval.StatusApproved:
		res.Status = verdict.StatusPass
		res.Notes = fmt.Sprintf("aprobado por %s (%s, sha %s)",
			rec.ApprovedBy, rec.ApprovedAt.Format("2006-01-02"), rec.SHA256[:8])
	case state == approval.StatusInvalidated:
		res.Status = verdict.StatusFail
		res.OutputTail = fmt.Sprintf("el spec fue EDITADO despues de aprobarse: la aprobacion quedo invalidada por el hash de contenido.\nAccion: revisa el cambio y re-aprueba con 'hoom spec approve %s'", specPath)
	default:
		res.Status = verdict.StatusFail
		res.OutputTail = fmt.Sprintf("el spec no tiene aprobacion humana registrada.\nAccion: 'hoom spec approve %s' antes de verificar contra el", specPath)
	}
	return res
}
