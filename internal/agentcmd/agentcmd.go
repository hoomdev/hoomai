package agentcmd

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
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
	Role      string      `json:"role"`
	Provider  string      `json:"provider"`
	Dir       string      `json:"dir"`
	Spec      string      `json:"spec,omitempty"`
	Approval  string      `json:"approval,omitempty"`
	RunID     string      `json:"run_id,omitempty"`
	RunStatus string      `json:"run_status,omitempty"`
	SessionID string      `json:"provider_session_id,omitempty"`
	Scope     ScopeResult `json:"scope"`
	VerdictID string      `json:"verdict_id,omitempty"`
	Verdict   string      `json:"verdict,omitempty"`
	// EnvelopeID names the durable record of THIS envelope in
	// .hoom/envelopes/, which is what `hoom status` and the Studio read.
	EnvelopeID string           `json:"envelope_id,omitempty"`
	Usage      *providers.Usage `json:"usage,omitempty"`
	Check      *checkcmd.Result `json:"check,omitempty"`
	Isolation  *Isolation       `json:"isolation,omitempty"`
	Stage      string           `json:"stage"`  // spec | aislar | run | scope | verify | check | ok
	Status     string           `json:"status"` // entregable | no-entregable | sin-entrega
	ExitCode   int              `json:"exit_code"`
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
	// Input validation, like an empty pedido: no record, no run, no token.
	if IsPlaceholder(opt.Prompt) {
		return Result{}, fmt.Errorf("el pedido %q no dice nada: escribi lo que el rol tiene que hacer", opt.Prompt)
	}
	if role.Scope == agents.ScopeSpecs && strings.TrimSpace(opt.Spec) != "" {
		return Result{}, fmt.Errorf("el rol %s escribe specs: --spec es el spec desde el que un rol trabaja y contra el que se verifica, "+
			"y un autor no se verifica contra el suyo. Nombra el spec en el pedido", role.Slug)
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
	// El registro del sobre: lo que status y el Studio pueden mirar MIENTRAS
	// esto corre. Se escribe en cada transicion de paso; escribirlo jamas
	// puede romper el sobre que describe (Write es best-effort).
	rec := envelope.Record{
		ID: envelope.NewID(), Role: role.Slug, Provider: prov.Name(), Task: opt.Task,
		Dir: dir, Stage: "spec", Step: 1, Steps: steps,
		Status: envelope.StatusRunning, ExitCode: -1, StartedAt: time.Now().UTC(),
	}
	res.EnvelopeID = rec.ID
	// Una sola transicion mueve los dos estados que el sobre mantiene: el
	// resultado que devuelve y el registro que status y el Studio leen. Dos
	// asignaciones gemelas repartidas por el flujo ya se desviaron una vez.
	advance := func(stage string, step int) {
		res.Stage = stage
		rec.Stage, rec.Step = stage, step
		envelope.Write(root, rec)
	}
	advance("spec", 1)
	fmt.Fprintf(w, "hoom agent: rol %s (%s) en %s\n", role.Slug, prov.Name(), displayDir(opt.Task))
	// Lo local queda fuera de Git ANTES de la primera foto: una regla que
	// hoom agrega durante el run se le imputaria al rol, y una que falta deja
	// telemetria sin trackear bloqueando el cierre de la tarea.
	if added, _ := hoomfs.EnsureIgnored(dir); len(added) > 0 {
		fmt.Fprintf(w, "  hoom completo .hoom/.gitignore (%s): commitealo junto con el cambio\n", strings.Join(added, ", "))
	}

	// [1/N] spec: no burn tokens on work the human has not authorized.
	specPath, err := specGate(w, dir, role, opt.Spec, &res, steps)
	if err != nil {
		return res, roto(root, &rec, err)
	}
	rec.Spec, rec.Approval = res.Spec, res.Approval
	if res.ExitCode != 0 {
		return cerrar(w, root, &rec, res, "spec", 1, "el spec no tiene aprobacion vigente y el rol escribe"), nil
	}
	advance("spec", 1)

	// [2/N] aislar: el rol ciego no obedece la regla de oro, la habita.
	mgr := runcmd.NewManager(root)
	runDir := dir
	var tree *isolate.Tree
	var beforeReal Snapshot
	if role.Isolated {
		advance("aislar", 2)
		if why := blindPrechecks(mgr, dir, specPath, PolicyFor(m, role)); why != "" {
			return cerrar(w, root, &rec, res, "aislar", 1, why), nil
		}
		markers, _ := profiles.Markers(m.Profile)
		pol := PolicyFor(m, role)
		pats := isolate.Patterns(pol.Allow, pol.Deny, markers)
		tree, err = isolate.Open(dir, role.Slug+"_"+newID(), pats)
		if err != nil {
			return cerrar(w, root, &rec, res, "aislar", 1, err.Error()), nil
		}
		res.Isolation = &Isolation{Dir: tree.Dir, Commit: tree.Commit, Patterns: tree.Patterns, Hidden: len(tree.Hidden)}
		rec.Isolated, rec.IsolatedFrom = true, tree.Commit
		advance("aislar", 2)
		runDir = tree.Dir
		beforeReal = Take(dir, base)
		contract += blindNote(tree)
		fmt.Fprintf(w, "  %s aislar  arbol ciego %s\n", step(2, steps), display(dir, tree.Dir))
		fmt.Fprintf(w, "                desde %s - %s\n", shortSHA(tree.Commit),
			plural(len(tree.Hidden), "archivo rastreado quedo fuera", "archivos rastreados quedaron fuera"))
	}

	// [N-3/N] run
	advance("run", steps-3)
	so, warn := startOptions(prov, role, contract, opt)
	so.Dir = runDir
	if warn {
		fmt.Fprintf(w, "  aviso: %s no puede imponer un rol de solo lectura; el limite se verifica solo despues del run\n", prov.Name())
	}
	if _, sinShell := NoExecFor(prov, role); sinShell {
		fmt.Fprintf(w, "  aviso: %s no puede quitarle el shell a un rol que escribe; su escritura se verifica solo despues del run\n", prov.Name())
	}
	before := Take(runDir, base)
	info, err := mgr.Start(so)
	if err != nil {
		keepQuarantine(w, tree, res.Isolation, "el run no arranco")
		return res, roto(root, &rec, err)
	}
	res.RunID = info.ID
	rec.RunID = info.ID
	advance("run", steps-3)
	fmt.Fprintf(w, "  %s run     %s - narracion en .hoom/runs/%s.jsonl\n", step(steps-3, steps), info.ID, info.ID)
	// latido: mientras el run narra no hay transiciones, y un sobre sin
	// movimiento es indistinguible de uno muerto. Como mucho un archivo cada
	// 5 segundos, nunca uno por evento.
	ultimo := time.Now()
	st := stream(mgr, info.ID, w, func() {
		if time.Since(ultimo) < 5*time.Second {
			return
		}
		ultimo = time.Now()
		envelope.Write(root, rec)
	})
	res.RunStatus, res.SessionID = st.Status, st.ProviderSessionID
	res.Usage, rec.Usage = st.Usage, st.Usage
	if !st.Usage.Empty() {
		fmt.Fprintf(w, "    gasto: %s\n", st.Usage.Summary())
	}
	advance("run", steps-3)
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
		return cerrar(w, root, &rec, res, "run", code, "el run fallo: no hay arbol confiable que medir"), nil
	}

	// [N-2/N] scope: the question no prompt can answer, plus the one the
	// blind tree lets us ask — is the blindfold still on?
	advance("scope", steps-2)
	var blind *Blind
	if tree != nil {
		blind = &Blind{Restored: tree.Breaches(), Leaked: delta(beforeReal.Touched, Take(dir, base).Touched)}
	}
	res.Scope = Gate(dir, base, opt.Task, role, before, Take(runDir, base), PolicyFor(m, role), blind)
	printScope(w, res.Scope, role, step(steps-2, steps))
	if res.Scope.Cuts() {
		note := "manipulacion de la evidencia: no se emite veredicto sobre este arbol"
		if res.Scope.Broken() {
			note = "el aislamiento se rompio: no se emite veredicto sobre este arbol"
		}
		keepQuarantine(w, tree, res.Isolation, "no se trasplanto nada")
		return cerrar(w, root, &rec, res, "scope", 1, note), nil
	}
	// Solo viaja lo que el gate aprobo: la cuarentena es la unica vez que el
	// gate llega ANTES de que el arbol certificable reciba la escritura.
	if tree != nil {
		if err := transplant(w, tree, dir, res.Scope, res.Isolation); err != nil {
			keepQuarantine(w, tree, res.Isolation, "el trasplante fallo a mitad de camino")
			return res, roto(root, &rec, err)
		}
	}
	// A role that writes and delivered nothing leaves no new tree to certify.
	// Checked AFTER the cut (tampering always wins) and after the transplant
	// (a finding the blind role created does not die with the quarantine).
	if !role.ReadOnly && len(res.Scope.Delivered()) == 0 {
		return sinEntrega(w, root, &rec, res, role), nil
	}

	// [N-1/N] verify
	advance("verify", steps-1)
	v, _, err := verifycmd.Run(m, verifycmd.Options{Spec: specPath})
	if err != nil {
		return res, roto(root, &rec, err)
	}
	res.VerdictID, res.Verdict = v.ID, v.Verdict
	rec.VerdictID, rec.Verdict = v.ID, v.Verdict
	fmt.Fprintf(w, "  %s verify  %s (veredicto %s)\n", step(steps-1, steps), color(v.Verdict == "green"), v.ID)

	// [N/N] check
	advance("check", steps)
	cr, err := checkcmd.Run(dir, base)
	if err != nil {
		return res, roto(root, &rec, err)
	}
	res.Check = &cr
	if cr.OK {
		fmt.Fprintf(w, "  %s check   VERDE (huella %s)\n", step(steps, steps), cr.FingerprintNow)
	} else {
		fmt.Fprintf(w, "  %s check   ROJO - %s. Accion: %s\n", step(steps, steps), cr.Reason, cr.Action)
	}

	switch {
	case !res.Scope.OK:
		return cerrar(w, root, &rec, res, "scope", 1, "el rol escribio fuera de su territorio"), nil
	case v.Verdict != "green":
		return cerrar(w, root, &rec, res, "verify", 1, "veredicto rojo"), nil
	case !cr.OK:
		return cerrar(w, root, &rec, res, "check", 1, cr.Reason), nil
	}
	return cerrar(w, root, &rec, res, "ok", 0, ""), nil
}

// cerrar cierra el sobre y su registro con la misma verdad: el paso donde
// paro, el exit y por que. El registro se escribe DESPUES de imprimir, porque
// lo que se guarda es el resultado, no la intencion.
func cerrar(w io.Writer, root string, rec *envelope.Record, res Result, stage string, code int, note string) Result {
	return registrar(root, rec, finish(w, res, stage, code, note), note)
}

// sinEntrega closes an envelope whose writing role changed nothing but the
// evidence hoom itself generates. It is not a red verdict — there is no
// verdict at all, because there is no new tree to certify — so it has its
// own status, and the step where it stopped is scope: that is where the delta
// was measured.
func sinEntrega(w io.Writer, root string, rec *envelope.Record, res Result, role agents.Role) Result {
	note := fmt.Sprintf("el rol %s escribe y el run no dejo ningun archivo: no hay arbol nuevo que certificar", role.Slug)
	res.Stage, res.ExitCode, res.Status = "scope", 1, envelope.StatusNoDelivery
	fmt.Fprintf(w, "hoom agent: SIN ENTREGA - %s\n", note)
	return registrar(root, rec, res, note)
}

// registrar writes the closing record from the Result it closes with.
func registrar(root string, rec *envelope.Record, res Result, note string) Result {
	rec.Stage, rec.Status, rec.ExitCode, rec.Note = res.Stage, res.Status, res.ExitCode, note
	rec.EndedAt = time.Now().UTC()
	envelope.Write(root, *rec)
	return res
}

// roto cierra el registro cuando lo que falla no es el trabajo del rol sino
// el setup (verify que no arranca, un trasplante a medias). El sobre termino
// igual, y un registro que se queda "en curso" para siempre seria una mentira
// que despues alguien tiene que interpretar.
func roto(root string, rec *envelope.Record, err error) error {
	rec.Status, rec.ExitCode, rec.Note = envelope.StatusNotDeliverable, 1, err.Error()
	rec.EndedAt = time.Now().UTC()
	envelope.Write(root, *rec)
	return err
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
		// The envelope is unattended by definition: no human answers the
		// CLI's prompts, so the role gets its tools up front and the scope
		// gate, not a denied Write, is what bounds it.
		Unattended: true,
	}
	var warn bool
	so.ReadOnly, so.Exec, warn = ReadOnlyFor(prov, role)
	so.NoExec, _ = NoExecFor(prov, role) // su aviso lo imprime Run
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

// NoExecFor resolves the "no shell" of a role that writes without Exec
// against what the provider DECLARES, exactly like ReadOnlyFor does with the
// read-only limit: one that cannot impose it returns warn, the run happens
// anyway and the scope gate is the net. Read-only roles and roles that do run
// commands ask for nothing here.
func NoExecFor(p providers.Provider, role agents.Role) (noExec, warn bool) {
	if role.ReadOnly || role.Exec {
		return false, false
	}
	if !p.Capabilities().NoExec {
		return false, true
	}
	return true, false
}

// IsPlaceholder reports a pedido that says nothing: empty once every blank is
// gone, made only of '.' and '…', or exactly "<pedido>" — the placeholder the
// envelope itself prints in its resume hint. Burning a run on it is paying a
// model to guess.
func IsPlaceholder(pedido string) bool {
	var b strings.Builder
	for _, r := range pedido {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if strings.EqualFold(s, "<pedido>") {
		return true
	}
	for _, r := range s {
		if r != '.' && r != '…' {
			return false
		}
	}
	return true
}

// stream mirrors the run narration while it happens, exactly like `hoom run`.
// beat is the envelope's heartbeat: it fires while the run talks, and the
// caller decides how often that is worth writing down.
func stream(mgr *runcmd.Manager, id string, w io.Writer, beat func()) runcmd.Run {
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
			fmt.Fprintf(w, "    %-6s %s%s\n", ev.Kind, agent, ev.Detail)
		}
		seen += len(evs)
		if beat != nil {
			beat()
		}
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
func blindPrechecks(mgr *runcmd.Manager, dir, specPath string, pol Policy) string {
	if err := isolate.Ready(dir); err != nil {
		return err.Error() + ".\n  Un test-writer sin venda no es un test-writer; si querés correr sin la garantía, eso es 'hoom run'"
	}
	if id, busy := mgr.Busy(dir); busy {
		return fmt.Sprintf("ya hay un run activo (%s) en el arbol de trabajo, y el trasplante pisaria ediciones en curso. Accion: espera a que termine", id)
	}
	if dirty := localChanges(dir, pol); len(dirty) > 0 {
		return fmt.Sprintf("el arbol real tiene cambios sin commitear donde el rol escribe (%s) y el trasplante no los pisa. Accion: commitealos o descartalos y repeti el run", strings.Join(dirty, ", "))
	}
	if specPath != "" {
		if err := specInHead(dir, specPath); err != nil {
			return err.Error()
		}
	}
	return ""
}

// localChanges lists the uncommitted paths (modified or untracked) that fall
// where the role may write: a transplant would have to overwrite them, and it
// refuses to. hoom's own local dirs and its ignore file never count.
func localChanges(dir string, pol Policy) []string {
	cmd := exec.Command("git", "status", "--porcelain", "-z", "--untracked-files=all")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var dirty []string
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		e := fields[i]
		if len(e) < 4 {
			continue
		}
		st, p := e[:2], e[3:]
		if st[0] == 'R' || st[0] == 'C' {
			i++ // el origen del rename viaja en el campo siguiente
		}
		p = filepath.ToSlash(p)
		if hoomfs.IsLocal(p) || strings.HasPrefix(p, ".hoom/") {
			continue
		}
		if ok, _ := allowedBy(p, pol); ok {
			dirty = append(dirty, p)
		}
	}
	sort.Strings(dirty)
	return dirty
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
