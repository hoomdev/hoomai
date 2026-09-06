package agentcmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

// Options is one envelope invocation.
type Options struct {
	Role      string // slug ("writer") or native name ("hoom-writer"); required
	Provider  string // "" = first installed provider that can carry a contract
	Task      string // task slug: run inside its worktree
	Spec      string // spec path: approval gate + verify --spec
	Prompt    string
	Model     string
	ResumeID  string
	MaxTurns  int
	BudgetUSD float64
}

// Result is the envelope's answer, identical in text and in JSON.
type Result struct {
	Role      string           `json:"role"`
	Provider  string           `json:"provider"`
	Dir       string           `json:"dir"`
	Spec      string           `json:"spec,omitempty"`
	Approval  string           `json:"approval,omitempty"`
	RunID     string           `json:"run_id,omitempty"`
	RunStatus string           `json:"run_status,omitempty"`
	SessionID string           `json:"provider_session_id,omitempty"`
	Scope     ScopeResult      `json:"scope"`
	VerdictID string           `json:"verdict_id,omitempty"`
	Verdict   string           `json:"verdict,omitempty"`
	Check     *checkcmd.Result `json:"check,omitempty"`
	Stage     string           `json:"stage"`  // spec | run | scope | verify | check | ok
	Status    string           `json:"status"` // entregable | no-entregable
	ExitCode  int              `json:"exit_code"`
}

// Run executes the five steps. Failures the envelope is MEANT to report (an
// unapproved spec, a failing run, a violated scope, a red verdict) come back
// as a Result with an exit code; only a broken setup returns an error.
func Run(root, base string, opt Options, w io.Writer) (Result, error) {
	role, err := agents.Lookup(opt.Role)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(opt.Prompt) == "" {
		return Result{}, fmt.Errorf("falta el pedido: hoom agent --role %s \"<pedido>\"", role.Slug)
	}
	dir, err := runcmd.TaskDir(root, opt.Task)
	if err != nil {
		return Result{}, err
	}
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		return Result{}, err
	}
	if m.BaseBranch != "" {
		base = m.BaseBranch
	}
	contract, err := agents.Contract(dir, role)
	if err != nil {
		return Result{}, err
	}
	prov, err := pickProvider(opt.Provider)
	if err != nil {
		return Result{}, err
	}
	res := Result{Role: role.Slug, Provider: prov.Name(), Dir: dir, Stage: "spec"}
	fmt.Fprintf(w, "hoom agent: rol %s (%s) en %s\n", role.Slug, prov.Name(), displayDir(opt.Task))

	// [1/5] spec: no burn tokens on work the human has not authorized.
	specPath, err := specGate(w, dir, role, opt.Spec, &res)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		return finish(w, res, "spec", 1, "el spec no tiene aprobacion vigente y el rol escribe"), nil
	}

	// [2/5] run
	res.Stage = "run"
	so, warn := startOptions(prov, role, contract, opt)
	if warn {
		fmt.Fprintf(w, "  aviso: %s no puede imponer un rol de solo lectura; el limite se verifica solo despues del run\n", prov.Name())
	}
	before := Take(dir, base)
	mgr := runcmd.NewManager(root)
	info, err := mgr.Start(so)
	if err != nil {
		return res, err
	}
	res.RunID = info.ID
	fmt.Fprintf(w, "  [2/5] run     %s - narracion en .hoom/runs/%s.jsonl\n", info.ID, info.ID)
	st := stream(mgr, info.ID, w)
	res.RunStatus, res.SessionID = st.Status, st.ProviderSessionID
	if st.ProviderSessionID != "" {
		fmt.Fprintf(w, "    sesion %s - reanudar: hoom agent --role %s --provider %s --resume %s \"<pedido>\"\n",
			st.ProviderSessionID, role.Slug, prov.Name(), st.ProviderSessionID)
	}
	if st.Status != runcmd.StatusDone || st.ExitCode != 0 {
		code := st.ExitCode
		if code == 0 {
			code = 1
		}
		fmt.Fprintf(w, "    run %s (exit %d)\n", st.Status, st.ExitCode)
		return finish(w, res, "run", code, "el run fallo: no hay arbol confiable que medir"), nil
	}

	// [3/5] scope: the question no prompt can answer.
	res.Stage = "scope"
	res.Scope = Gate(dir, base, role, before, Take(dir, base), PolicyFor(m, role))
	printScope(w, res.Scope, role)
	if res.Scope.Tampering {
		return finish(w, res, "scope", 1,
			"manipulacion de la evidencia: no se emite veredicto sobre este arbol"), nil
	}

	// [4/5] verify
	res.Stage = "verify"
	v, _, err := verifycmd.Run(m, verifycmd.Options{Spec: specPath})
	if err != nil {
		return res, err
	}
	res.VerdictID, res.Verdict = v.ID, v.Verdict
	fmt.Fprintf(w, "  [4/5] verify  %s (veredicto %s)\n", color(v.Verdict == "green"), v.ID)

	// [5/5] check
	res.Stage = "check"
	cr, err := checkcmd.Run(dir, base)
	if err != nil {
		return res, err
	}
	res.Check = &cr
	if cr.OK {
		fmt.Fprintf(w, "  [5/5] check   VERDE (huella %s)\n", cr.FingerprintNow)
	} else {
		fmt.Fprintf(w, "  [5/5] check   ROJO - %s. Accion: %s\n", cr.Reason, cr.Action)
	}

	switch {
	case !res.Scope.OK:
		return finish(w, res, "scope", 1, "el rol escribio fuera de su territorio"), nil
	case v.Verdict != "green":
		return finish(w, res, "verify", 1, "veredicto rojo"), nil
	case !cr.OK:
		return finish(w, res, "check", 1, cr.Reason), nil
	}
	return finish(w, res, "ok", 0, ""), nil
}

// specGate resolves the spec path against the WORK directory and enforces a
// current approval for roles that write. A read-only role never needs one:
// the architect writes the spec that is not approved yet.
func specGate(w io.Writer, dir string, role agents.Role, spec string, res *Result) (string, error) {
	if strings.TrimSpace(spec) == "" {
		fmt.Fprintln(w, "  [1/5] spec    sin --spec: no se exige aprobacion ni trazabilidad")
		return "", nil
	}
	specPath := spec
	if !filepath.IsAbs(specPath) {
		specPath = filepath.Join(dir, specPath)
	}
	state, rec, err := approval.Status(dir, specPath)
	if err != nil {
		return "", err
	}
	res.Spec, res.Approval = specPath, state
	fmt.Fprintf(w, "  [1/5] spec    %s: %s\n", display(dir, specPath), approval.Describe(state, rec))
	if !role.ReadOnly && state != approval.StatusApproved {
		res.ExitCode = 1
	}
	return specPath, nil
}

// pickProvider honors an explicit choice and otherwise takes the first
// installed provider that can carry the role's contract. Refusing by naming
// the missing capability beats degrading in silence.
func pickProvider(name string) (providers.Provider, error) {
	if n := strings.TrimSpace(name); n != "" {
		p, err := providers.Lookup(n)
		if err != nil {
			return nil, err
		}
		if !p.Capabilities().SystemPrompt {
			return nil, providers.ErrUnsupported{Provider: p.Name(), Fields: []string{"system_prompt"}}
		}
		return p, nil
	}
	for _, info := range providers.Detect() {
		if info.Installed && info.Capabilities.SystemPrompt {
			return providers.Lookup(info.Name)
		}
	}
	return nil, fmt.Errorf("ningun provider instalado soporta system_prompt, que 'hoom agent' exige para dar el contrato del rol (mira 'hoom providers')")
}

// startOptions translates the envelope's decision into a run request. Strict
// is not a flag here: without its contract as system prompt there is no role
// to verify, and a silent degradation would be a lie told with evidence.
func startOptions(prov providers.Provider, role agents.Role, contract string, opt Options) (runcmd.StartOptions, bool) {
	so := runcmd.StartOptions{
		Provider: prov.Name(), Prompt: opt.Prompt, Task: opt.Task, Role: role.Slug,
		ResumeID: opt.ResumeID, Model: opt.Model, SystemPrompt: contract,
		MaxTurns: opt.MaxTurns, BudgetUSD: opt.BudgetUSD, Strict: true,
	}
	var warn bool
	so.ReadOnly, so.Exec, warn = ReadOnlyFor(prov, role)
	return so, warn
}

// ReadOnlyFor resolves the role's limit against what the provider DECLARES,
// not against who it is: a provider that can impose it receives the
// intention, and one that cannot returns warn — the run still happens and the
// scope gate, which depends on no CLI, remains the net. A writing role limits
// nothing.
func ReadOnlyFor(p providers.Provider, role agents.Role) (readOnly, exec, warn bool) {
	if !role.ReadOnly {
		return false, false, false
	}
	if !p.Capabilities().ReadOnly {
		return false, false, true
	}
	return true, role.Exec, false
}

// stream mirrors the run narration while it happens, exactly like `hoom run`.
func stream(mgr *runcmd.Manager, id string, w io.Writer) runcmd.Run {
	seen := 0
	for {
		st, evs, err := mgr.Events(id, seen)
		if err != nil {
			return st
		}
		for _, ev := range evs {
			agent := ""
			if ev.Agent != "" {
				agent = "[" + ev.Agent + "] "
			}
			fmt.Fprintf(w, "    %-5s %s%s\n", ev.Kind, agent, ev.Detail)
		}
		seen += len(evs)
		if st.Status != runcmd.StatusRunning {
			return st
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func printScope(w io.Writer, sc ScopeResult, role agents.Role) {
	if sc.OK {
		fmt.Fprintf(w, "  [3/5] scope   %s, 0 fuera de scope\n", plural(len(sc.Touched), "archivo tocado", "archivos tocados"))
		return
	}
	fmt.Fprintf(w, "  [3/5] scope   ROJO - %s, %s (rol %s, scope %s):\n",
		plural(len(sc.Touched), "archivo tocado", "archivos tocados"),
		plural(len(sc.Violations), "violacion", "violaciones"), role.Slug, role.Scope)
	fueraDelAllow := false
	for _, v := range sc.Violations {
		id := ""
		if v.FindingID != "" {
			id = " [" + v.FindingID + "]"
		}
		fmt.Fprintf(w, "                  %s (%s): %s%s\n", v.Path, v.Rule, v.Detail, id)
		if strings.HasPrefix(v.Detail, detalleFueraDelAllow) {
			fueraDelAllow = true
		}
	}
	// La pista solo sirve cuando lo que falto fue territorio: una ruta
	// prohibida a proposito no se arregla ensanchando el allow.
	if fueraDelAllow {
		fmt.Fprintf(w, "                  si el layout del proyecto es otro, declaralo en hoom.yaml: agents.%s.write.allow\n", role.Slug)
	}
}

func finish(w io.Writer, res Result, stage string, code int, note string) Result {
	res.Stage, res.ExitCode = stage, code
	res.Status = "entregable"
	if code != 0 {
		res.Status = "no-entregable"
	}
	if code == 0 {
		fmt.Fprintln(w, "hoom agent: ENTREGABLE")
		return res
	}
	line := fmt.Sprintf("hoom agent: NO ENTREGABLE (%s)", stage)
	if note != "" {
		line += " - " + note
	}
	fmt.Fprintln(w, line)
	return res
}

// displayDir names where the envelope worked the same way the run log does.
func displayDir(task string) string {
	if task == "" {
		return "el proyecto"
	}
	return filepath.Join(".hoom", "worktrees", task)
}

// display shortens a path against the work directory when it lives inside it.
func display(dir, p string) string {
	if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

func plural(n int, uno, varios string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, uno)
	}
	return fmt.Sprintf("%d %s", n, varios)
}

func color(green bool) string {
	if green {
		return "VERDE"
	}
	return "ROJO"
}
