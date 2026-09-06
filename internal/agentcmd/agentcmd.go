package agentcmd

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/isolate"
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
	Isolation *Isolation       `json:"isolation,omitempty"`
	Stage     string           `json:"stage"`  // spec | aislar | run | scope | verify | check | ok
	Status    string           `json:"status"` // entregable | no-entregable
	ExitCode  int              `json:"exit_code"`
}

// Isolation is what the envelope did with the blind tree, and what came back
// out of it. It travels in the Result so the guarantee is a datum and not a
// line of terminal output.
type Isolation struct {
	Dir      string   `json:"dir"`
	Commit   string   `json:"commit"`
	Patterns []string `json:"patterns"`
	Hidden   int      `json:"hidden"`  // archivos rastreados fuera del arbol
	Applied  []string `json:"applied"` // rutas trasplantadas al arbol real
	Removed  []string `json:"removed"` // rutas que el trasplante borro
	Kept     bool     `json:"kept"`    // la cuarentena quedo en disco
}

// Run executes the five steps — six when the role runs blind. Failures the
// envelope is MEANT to report (an unapproved spec, a failing run, a violated
// scope, a red verdict) come back as a Result with an exit code; only a
// broken setup returns an error.
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
	steps := 5
	if role.Isolated {
		steps = 6 // el arbol ciego se arma y se prueba: eso es un paso, no una nota al pie
	}
	res := Result{Role: role.Slug, Provider: prov.Name(), Dir: dir, Stage: "spec"}
	fmt.Fprintf(w, "hoom agent: rol %s (%s) en %s\n", role.Slug, prov.Name(), displayDir(opt.Task))

	// [1/N] spec: no burn tokens on work the human has not authorized.
	specPath, err := specGate(w, dir, role, opt.Spec, &res, steps)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		return finish(w, res, "spec", 1, "el spec no tiene aprobacion vigente y el rol escribe"), nil
	}

	// [2/N] aislar: el rol ciego no obedece la regla de oro, la habita.
	mgr := runcmd.NewManager(root)
	runDir := dir
	var tree *isolate.Tree
	var beforeReal Snapshot
	if role.Isolated {
		res.Stage = "aislar"
		if why := blindPrechecks(mgr, dir, specPath); why != "" {
			return finish(w, res, "aislar", 1, why), nil
		}
		markers, _ := profiles.Markers(m.Profile)
		pol := PolicyFor(m, role)
		pats := isolate.Patterns(pol.Allow, pol.Deny, markers)
		tree, err = isolate.Open(dir, role.Slug+"_"+newID(), pats)
		if err != nil {
			return finish(w, res, "aislar", 1, err.Error()), nil
		}
		res.Isolation = &Isolation{Dir: tree.Dir, Commit: tree.Commit, Patterns: tree.Patterns, Hidden: len(tree.Hidden)}
		runDir = tree.Dir
		beforeReal = Take(dir, base)
		contract += blindNote(tree)
		fmt.Fprintf(w, "  %s aislar  arbol ciego %s\n", step(2, steps), display(dir, tree.Dir))
		fmt.Fprintf(w, "                desde %s - %s\n", shortSHA(tree.Commit),
			plural(len(tree.Hidden), "archivo rastreado quedo fuera", "archivos rastreados quedaron fuera"))
	}

	// [N-3/N] run
	res.Stage = "run"
	so, warn := startOptions(prov, role, contract, opt)
	so.Dir = runDir
	if warn {
		fmt.Fprintf(w, "  aviso: %s no puede imponer un rol de solo lectura; el limite se verifica solo despues del run\n", prov.Name())
	}
	before := Take(runDir, base)
	info, err := mgr.Start(so)
	if err != nil {
		keepQuarantine(w, tree, res.Isolation, "el run no arranco")
		return res, err
	}
	res.RunID = info.ID
	fmt.Fprintf(w, "  %s run     %s - narracion en .hoom/runs/%s.jsonl\n", step(steps-3, steps), info.ID, info.ID)
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
		keepQuarantine(w, tree, res.Isolation, "el run fallo")
		return finish(w, res, "run", code, "el run fallo: no hay arbol confiable que medir"), nil
	}

	// [N-2/N] scope: the question no prompt can answer, plus the one the
	// blind tree lets us ask — is the blindfold still on?
	res.Stage = "scope"
	var blind *Blind
	if tree != nil {
		blind = &Blind{Restored: tree.Breaches(), Leaked: delta(beforeReal.Touched, Take(dir, base).Touched)}
	}
	res.Scope = Gate(dir, base, role, before, Take(runDir, base), PolicyFor(m, role), blind)
	printScope(w, res.Scope, role, step(steps-2, steps))
	if res.Scope.Cuts() {
		note := "manipulacion de la evidencia: no se emite veredicto sobre este arbol"
		if res.Scope.Broken() {
			note = "el aislamiento se rompio: no se emite veredicto sobre este arbol"
		}
		keepQuarantine(w, tree, res.Isolation, "no se trasplanto nada")
		return finish(w, res, "scope", 1, note), nil
	}
	// Solo viaja lo que el gate aprobo: la cuarentena es la unica vez que el
	// gate llega ANTES de que el arbol certificable reciba la escritura.
	if tree != nil {
		if err := transplant(w, tree, dir, res.Scope, res.Isolation); err != nil {
			keepQuarantine(w, tree, res.Isolation, "el trasplante fallo a mitad de camino")
			return res, err
		}
	}

	// [N-1/N] verify
	res.Stage = "verify"
	v, _, err := verifycmd.Run(m, verifycmd.Options{Spec: specPath})
	if err != nil {
		return res, err
	}
	res.VerdictID, res.Verdict = v.ID, v.Verdict
	fmt.Fprintf(w, "  %s verify  %s (veredicto %s)\n", step(steps-1, steps), color(v.Verdict == "green"), v.ID)

	// [N/N] check
	res.Stage = "check"
	cr, err := checkcmd.Run(dir, base)
	if err != nil {
		return res, err
	}
	res.Check = &cr
	if cr.OK {
		fmt.Fprintf(w, "  %s check   VERDE (huella %s)\n", step(steps, steps), cr.FingerprintNow)
	} else {
		fmt.Fprintf(w, "  %s check   ROJO - %s. Accion: %s\n", step(steps, steps), cr.Reason, cr.Action)
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
func specGate(w io.Writer, dir string, role agents.Role, spec string, res *Result, steps int) (string, error) {
	if strings.TrimSpace(spec) == "" {
		fmt.Fprintf(w, "  %s spec    sin --spec: no se exige aprobacion ni trazabilidad\n", step(1, steps))
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
	fmt.Fprintf(w, "  %s spec    %s: %s\n", step(1, steps), display(dir, specPath), approval.Describe(state, rec))
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

func printScope(w io.Writer, sc ScopeResult, role agents.Role, prefix string) {
	if sc.OK {
		fmt.Fprintf(w, "  %s scope   %s, 0 fuera de scope%s\n", prefix,
			plural(len(sc.Touched), "archivo tocado", "archivos tocados"), intacto(role))
		return
	}
	fmt.Fprintf(w, "  %s scope   ROJO - %s, %s (rol %s, scope %s):\n", prefix,
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

// step renders the "[n/N]" prefix. The envelope has five steps, and six when
// the role runs blind: the blind tree is armed and PROVED, and a proof is not
// a footnote of another step.
func step(n, total int) string { return fmt.Sprintf("[%d/%d]", n, total) }

func intacto(role agents.Role) string {
	if role.Isolated {
		return ", aislamiento intacto"
	}
	return ""
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func newID() string {
	raw := make([]byte, 3)
	rand.Read(raw)
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(raw)
}

// blindPrechecks refuses, BEFORE creating anything, the three situations in
// which a blind tree would be a lie: no git to build it from, another run
// already moving the tree it will transplant into, and a spec the role would
// work from without it being in the commit the tree is built from.
func blindPrechecks(mgr *runcmd.Manager, dir, specPath string) string {
	if err := isolate.Ready(dir); err != nil {
		return err.Error() + ".\n  Un test-writer sin venda no es un test-writer; si querés correr sin la garantía, eso es 'hoom run'"
	}
	if id, busy := mgr.Busy(dir); busy {
		return fmt.Sprintf("ya hay un run activo (%s) en el arbol de trabajo, y el trasplante pisaria ediciones en curso. Accion: espera a que termine", id)
	}
	if specPath != "" {
		if err := specInHead(dir, specPath); err != nil {
			return err.Error()
		}
	}
	return ""
}

// specInHead demands the spec be part of the commit the blind tree is built
// from. The alternative — hoom copying the file in — would make the tree's
// content a decision of hoom instead of a fact of git, and the whole point is
// that anyone can rebuild it from the sha and check what the role saw.
func specInHead(dir, specPath string) error {
	rel, err := filepath.Rel(dir, specPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("el spec %s vive fuera del arbol de trabajo y no puede viajar al arbol ciego", specPath)
	}
	rel = filepath.ToSlash(rel)
	cmd := exec.Command("git", "show", "HEAD:"+rel)
	cmd.Dir = dir
	head, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("el spec %s no esta en el commit desde el que se arma el arbol ciego, asi que el rol no lo veria.\n  Accion: git add %s && git commit", rel, rel)
	}
	disk, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("no se pudo leer el spec %s: %v", rel, err)
	}
	if !bytes.Equal(head, disk) {
		return fmt.Errorf("el spec %s difiere del commiteado y el rol veria la version vieja.\n  Accion: git add %s && git commit", rel, rel)
	}
	return nil
}

// blindNote tells the role what is already true. It replaces nothing: a model
// that does not know WHY files are missing burns turns looking for them or
// decides the repo is broken.
func blindNote(t *isolate.Tree) string {
	return fmt.Sprintf(`

Este arbol es CIEGO: hoom lo armo desde el commit %s con git sparse-checkout
y dejo fuera %d archivos rastreados, entre ellos toda la implementacion. Los
tests que escribas salen del spec, no del codigo, y por eso el codigo no esta.
No falta nada ni el repo esta roto. Buscar la implementacion por otra via
(historia de git, rutas absolutas fuera de este arbol) viola el contrato del
rol y hoom lo trata como tal.
`, shortSHA(t.Commit), len(t.Hidden))
}

// transplant moves out of the quarantine exactly what the gate approved. A
// path with a violation stays inside: the certifiable tree never sees it.
func transplant(w io.Writer, t *isolate.Tree, dest string, sc ScopeResult, iso *Isolation) error {
	bad := map[string]bool{}
	for _, v := range sc.Violations {
		bad[v.Path] = true
	}
	var travel []string
	for _, p := range sc.Touched {
		if !bad[p] {
			travel = append(travel, p)
		}
	}
	applied, removed, err := t.Apply(dest, travel)
	if err != nil {
		return err
	}
	iso.Applied, iso.Removed = applied, removed
	switch {
	case len(applied)+len(removed) == 0:
		fmt.Fprintln(w, "                0 archivos trasplantados al arbol real")
	default:
		if len(applied) > 0 {
			fmt.Fprintf(w, "                trasplantados: %s\n", strings.Join(applied, ", "))
		}
		if len(removed) > 0 {
			fmt.Fprintf(w, "                borrados por el rol: %s\n", strings.Join(removed, ", "))
		}
	}
	if sc.OK {
		if cerr := t.Close(); cerr != nil {
			fmt.Fprintf(w, "                aviso: %v\n", cerr)
			iso.Kept = true
		}
		return nil
	}
	keepQuarantine(w, t, iso, "lo que violo el scope se quedo adentro")
	return nil
}

// keepQuarantine leaves the blind tree on disk and says so. A quarantine with
// something wrong inside is evidence, and evidence nobody can look at is not
// evidence.
func keepQuarantine(w io.Writer, t *isolate.Tree, iso *Isolation, why string) {
	if t == nil || iso == nil {
		return
	}
	iso.Kept = true
	fmt.Fprintf(w, "                cuarentena conservada en %s (%s)\n", t.Dir, why)
	fmt.Fprintf(w, "                para descartarla: git worktree remove --force %s\n", t.Dir)
}
