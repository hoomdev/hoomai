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

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gitajeno"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/materialgit"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/ratchet"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/runcmd"
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
//     role Rol in the manifest's tree (for the test-writer that tree is
//     read-only and only its blind tree Ciego is writable) and the cache
//     `verify`
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

// LineaEntorno (verify-en-el-entorno) is the line `hoom verify` prints under
// its first line when the verdict has Entorno; %s is the template
// (verdict.Entorno.Ejecutar): `entorno: <plantilla> (de .hoom/local.yaml)`.
// With --json it goes, like that first line, to stderr: stdout stays the
// verdict alone.
const LineaEntorno = "entorno: %s (de .hoom/local.yaml)"

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

// GateEntornoLocal (verify-en-el-entorno, CA-773) is the gate EVERY verify
// adds to its verdict when the main checkout's .hoom/local.yaml was created,
// changed or removed while its gates ran: required, status fail, scope full,
// exit_code 1, its note (Notes) exactly NotaEntornoLocal and its output
// (OutputTail) the path entorno.Ruta. The verdict is then red, like one with
// GateMaterialDeGit, and no reader of verdicts takes it as green. A verify
// whose .hoom/local.yaml did not change has no such gate. It holds for
// `hoom verify`, the verify of an envelope, the pilot's cerrar-hallazgos and
// POST /api/verify, with Options.Sobre or without it, with ejecutar or
// without it, and in a --gate run too: a gate that runs unenclosed, with the
// template or without it, can write in the main checkout.
const GateEntornoLocal = "entorno_local"

// NotaEntornoLocal is the note of the gate GateEntornoLocal, exactly.
const NotaEntornoLocal = "manipulacion: .hoom/local.yaml cambio mientras corrian los gates"

// Run executes the verification and writes the verdict. It returns the
// finalized verdict and the path of its artifact.
//
// It validates the gate selection against the manifest BEFORE anything else:
// a request hoom does not understand produces no verdict, no live events and
// no ratchet measurement — it produces a *cliargs.UsageError.
//
// Right after that, and before anything else (no git snapshot, no live
// events, no gate), it reads the local adjustment of the machine
// (verify-en-el-entorno) with entorno.Leer(m.Dir, the names of m.Gates): its
// error is Run's error, exactly, and no gate runs and no verdict is written,
// as with an invalid hoom.yaml. This holds for every caller — `hoom verify`,
// the envelope of `hoom agent`, the pilot's cerrar-hallazgos and
// POST /api/verify — since all of them run this function. With ejecutar:
//   - the gates run with its template (gates.Options.Local);
//   - the verdict carries Entorno, with Ejecutar and SHA256 of the file, and
//     every GateResult its Entorno (verdict.EntornoPrefijo or
//     verdict.EntornoHost);
//   - with Options.Sobre and the section `encierro` in Sobre.Root's
//     hoom.yaml, gates.Options.ExigirEncierro is its exigir, whether or not
//     this system can enclose.
//
// Without ejecutar (no file, or ejecutar with no value), the same commands,
// the same states and the same JSON as before.
//
// With ejecutar or without it, and with Options.Sobre or without it, every
// run photographs the main checkout's .hoom/local.yaml (entorno.Ruta under
// entorno.Ajuste.Principal): the sha256 of its bytes, or that it does not
// exist, right before the first gate the run executes and right after the
// last (CA-773), counting hoom's own: the spec's (spec_lint, spec_trace with
// its [verifica:] commands, spec_approved), findings_open, the manifest's
// gates and the ratchet with its metric commands. When the two differ, the
// verdict carries one more gate, GateEntornoLocal, in fail with
// NotaEntornoLocal, and is red; when they do not, it has no such gate and the
// JSON is as before.
//
// riesgo-efectivo-y-tamano: every run computes, before its gates, the
// riesgo and the size of the change, and the verdict carries them
// (verdict.Verdict Riesgo, Tamano and Tarea):
//   - the task is Sobre.Task with Options.Sobre, else runcmd.TareaDe(m.Dir)
//     (the worktree .hoom/worktrees/<slug> where it runs, on its branch or
//     not); in any other tree there is none;
//   - its card is item.TarjetaDeRiesgo(m.Dir, task) (the main checkout's
//     item, as on the board), or one that cannot be read (alto, rule 1)
//     when TareaDe could not check the worktree (gitx.ErrSinComprobar);
//   - the classifier is profiles.Clasificador(m): the rutas and the
//     test-writer's territory of the hoom.yaml it verifies with, like the
//     gates;
//   - the files and lines are gitx.LineasDelCambio(m.Dir, m.BaseBranch);
//     outside a git repository there is no Tamano and the riesgo is computed
//     without files.
//
// Its findings_open gate is finding.GateDelVerify with that classifier, the
// task and the effective riesgo it computed. Neither key moves the
// fingerprint. An error of the section `rutas` is an error of the manifest
// (manifest.Rutas): Load already refuses it, so no gate runs and no verdict
// is written.
func Run(m *manifest.Manifest, opt Options) (*verdict.Verdict, string, error) {
	if err := validarGates(m, opt.Gates); err != nil {
		return nil, "", err
	}
	local, err := entorno.Leer(m.Dir, m.SortedGateNames())
	if err != nil {
		return nil, "", err
	}
	git := gitx.Snapshot(m.Dir, m.BaseBranch)
	// the riesgo and the size of the change, before the gates: the
	// findings_open gate judges with them
	med, err := medirElCambio(m, opt, git)
	if err != nil {
		return nil, "", err
	}
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

	gopt := gates.Options{Full: opt.Full, Emit: lw.Emit, Local: local}
	var antes materialgit.Foto
	if opt.Sobre != nil {
		plan, sin, exigir, err := planDelSobre(m, *opt.Sobre)
		if err != nil {
			return nil, "", err
		}
		gopt.Encierro, gopt.SinEncierro, gopt.ExigirEncierro = plan, sin, exigir
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

	// the photograph of .hoom/local.yaml spans every command the run
	// executes, hoom's own gates included (CA-773)
	localAntes := fotoLocal(local.Principal)
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
		fres := finding.GateDelVerify(m.Dir, m.BaseBranch, blockOn, specFile,
			finding.DelVerify{Clasificador: med.clasificador, Tarea: med.tarea, Efectivo: med.riesgo.Efectivo})
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
	if fotoLocal(local.Principal) != localAntes {
		results = append(results, verdict.GateResult{Name: GateEntornoLocal, Required: true, Status: verdict.StatusFail,
			Scope: "full", ExitCode: 1, OutputTail: entorno.Ruta, Notes: NotaEntornoLocal})
	}

	v := &verdict.Verdict{
		Project: m.Project,
		Profile: m.Profile,
		Policy:  m.Policy,
		Git:     git,
		Gates:   results,
		Spec:    opt.Spec,
		Riesgo:  &med.riesgo,
		Tamano:  med.tamano,
		Tarea:   med.tarea,
	}
	if local.Ejecutar != "" {
		v.Entorno = &verdict.Entorno{Ejecutar: local.Ejecutar, SHA256: local.SHA256}
		for i := range v.Gates {
			if v.Gates[i].Entorno == "" {
				// hoom's own gates (spec_*, findings_open, ratchet, the
				// photographs) run in hoom, on the host
				v.Gates[i].Entorno = verdict.EntornoHost
			}
		}
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

// medida is what a verify measures of its change (riesgo-efectivo-y-tamano).
type medida struct {
	tarea        string
	clasificador riesgo.Clasificador
	riesgo       riesgo.Evaluacion
	tamano       *riesgo.Tamano // nil outside a repository or when git cannot measure
}

// medirElCambio measures the change m verifies: its task (Sobre.Task, or the
// worktree's: runcmd.TareaDe), its card as the board reads it
// (item.TarjetaDeRiesgo: the main checkout's item first; one that cannot be
// read when TareaDe could not check the worktree), the classifier of
// the hoom.yaml it verifies with (profiles.Clasificador) and the files and
// lines of gitx.LineasDelCambio. When git cannot give those (outside a
// repository, a base without a merge-base, a diff that fails) there is no
// tamano and the riesgo looks at the snapshot's changed files (aclaracion 3):
// none outside a repository.
func medirElCambio(m *manifest.Manifest, opt Options, git gitx.Info) (medida, error) {
	cl, err := profiles.Clasificador(m)
	if err != nil {
		return medida{}, err
	}
	md := medida{clasificador: cl}
	if opt.Sobre != nil {
		md.tarea = strings.TrimSpace(opt.Sobre.Task)
	}
	sinComprobar := false
	if md.tarea == "" {
		md.tarea, sinComprobar = runcmd.TareaDe(m.Dir)
	}
	var t riesgo.Tarjeta
	switch {
	case sinComprobar:
		// git could not say whether this is the task's worktree: its card
		// counts as one that cannot be read, and fails closed (rule 1)
		t = riesgo.Tarjeta{Tarea: md.tarea, Ilegible: true}
	case md.tarea != "":
		t = item.TarjetaDeRiesgo(m.Dir, md.tarea)
	}
	if git.IsRepo {
		if lineas, err := gitx.LineasDelCambio(m.Dir, m.BaseBranch); err == nil {
			tam := cl.Medir(lineas)
			md.tamano = &tam
			archivos := make([]string, 0, len(lineas))
			for f := range lineas {
				archivos = append(archivos, f)
			}
			md.riesgo = cl.Evaluar(t, archivos)
			return md, nil
		}
	}
	md.riesgo = cl.Evaluar(t, git.ChangedFiles)
	return md, nil
}

// topeFotoMaterial bounds each photograph of the git material of a verify
// with Options.Sobre (aclaracion 28): one that does not end counts as
// changed.
const topeFotoMaterial = 5 * time.Second

// fotoLocal is the photograph of the .hoom/local.yaml of the main checkout
// principal around the gates (CA-773): its Huella (the sha256 of its bytes,
// or that it is not there, not a regular file or not readable), read from
// the disk without following it.
func fotoLocal(principal string) string {
	return materialgit.Huella(filepath.Join(principal, filepath.FromSlash(entorno.Ruta)))
}

// planDelSobre is the encierro of the gates of a verify with Sobre s: nil
// (as today) without the section `encierro` in s.Root's hoom.yaml, or with
// a level ninguno; with the mechanism job (Windows) a plan with no view,
// each gate in its job and, when Windows leaves it out with exigir above
// ninguno, in error on start (encierro.Comprobar, ronda 1, decision 22);
// with bwrap the view of role s.Rol with the cache `verify`:
// the manifest's tree writable, but for the test-writer (agents.Role.Isolated),
// whose gates run in the manifest's tree read-only and write only in its
// blind tree s.Ciego, when it is still there. sin are
// encierro.gates_sin_encierro, and exigir is encierro.exigir when there is
// the section ("" without it), whether or not this system can enclose
// (verify-en-el-entorno, aclaracion 10).
func planDelSobre(m *manifest.Manifest, s Sobre) (plan *encierro.Plan, sin map[string]bool, exigir string, err error) {
	roles, err := manifest.LoadRoles(s.Root)
	if err != nil {
		return nil, nil, "", err
	}
	if roles.Encierro == nil {
		return nil, nil, "", nil
	}
	exigir = roles.Encierro.Exigir
	sin = map[string]bool{}
	for _, g := range roles.Encierro.GatesSinEncierro {
		sin[g] = true
	}
	info := encierro.Nivel(s.Task != "", nil)
	switch info.Mecanismo {
	case encierro.MecanismoBwrap:
	case encierro.MecanismoJob:
		return &encierro.Plan{Info: info, Exigir: roles.Encierro.Exigir}, sin, exigir, nil
	default:
		return nil, sin, exigir, nil
	}
	lugar := encierro.Lugar{Principal: s.Root, Tarea: s.Task, Arbol: m.Dir, Quien: encierro.QuienVerify,
		Escribibles: roles.Encierro.Escribibles, Perfil: m.Profile}
	if rol, err := agents.Lookup(s.Rol); err == nil && rol.Isolated {
		// the view of the test-writer (ronda 1, a8ef36): the implementation
		// stays read-only; only its blind tree, if it is still there, and
		// the cache are writable
		lugar.Dir, lugar.Arbol = m.Dir, s.Ciego
	}
	vista, err := encierro.VistaDe(lugar)
	if err != nil {
		return nil, nil, "", err
	}
	return &encierro.Plan{Info: info, Vista: vista, Exigir: roles.Encierro.Exigir}, sin, exigir, nil
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
