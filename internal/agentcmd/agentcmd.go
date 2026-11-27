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
	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/isolate"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/valores"
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
	// BudgetSet: BudgetUSD was asked (0 included), so hoom.yaml's
	// agents.<rol>.budget_usd does not apply.
	BudgetSet bool
	// Effort is the reasoning effort, in the provider's vocabulary; "" =
	// agents.<rol>.effort of the project's hoom.yaml, or the provider's.
	Effort string
	// Timeout bounds the role's run; 0 = agents.<rol>.timeout of the
	// project's hoom.yaml, or hoom's 30 minutes.
	Timeout time.Duration
	// EnvelopeID presets the id of the envelope's record ("" = a new one), so
	// a caller that runs the envelope in the background can name it at once.
	EnvelopeID string
	// Started is called once, when the envelope's first record is on disk.
	// An error before that record arrives without Started being called.
	Started func()
	// Pilot marks an envelope the cabin's belt launched: its record says so.
	Pilot bool

	// autonomia-entre-firmas: what the envelope's record keeps for the
	// pilot. Columna is the card's column at the launch (lanzar sets it);
	// with it the record carries `columna` and `pedido` (Prompt, the text
	// the role received). Regla, Intento, Ronda, Hallazgos and Bloqueantes
	// go to the record's keys of the same name (envelope.Record) as given;
	// only the pilot sets them. None of them changes what the envelope does.
	Columna     string
	Regla       string
	Intento     int
	Ronda       int
	Hallazgos   []string
	Bloqueantes int

	// ValoresImpresos (proporcion-por-riesgo) says the verb that launched
	// the envelope already printed its line valores: (lanzar sets it from
	// lanzar.Request.ValoresImpresos): Run does not print it. false (`hoom
	// agent` itself, the Studio) = Run prints it once, before its steps.
	ValoresImpresos bool
}

// NotaDeRiesgo (proporcion-por-riesgo) is the note hoom adds at the end of
// the SystemPrompt of a role, after a blank line (for the test-writer,
// after the paragraph of the blind tree), and nothing else after it; r is
// the DECLARED riesgo of the card of --task (item.RiesgoDeLanzamiento: alto
// when it cannot be read, normal without card) and t the tope its spec is
// measured with (the card's own, else spec.topes.<r> of hoom.yaml):
//
//   - test-writer: `Riesgo declarado de la tarjeta: <r>.`;
//   - arquitecto:
//     `Riesgo declarado de la tarjeta: <r>. Tope del spec: <t> palabras y <c> criterios.`,
//     with `palabras sin tope` or `criterios sin tope` in place of
//     `<t> palabras` or `<c> criterios` when that tope is 0;
//   - every other role: "" (no note).
//
// rol is the role's slug. The contract in .hoom/agents/ does not change
// (CA-182).
func NotaDeRiesgo(rol, r string, t spec.Tope) string {
	medida := func(n int, que string) string {
		if n == 0 {
			return que + " sin tope"
		}
		return fmt.Sprintf("%d %s", n, que)
	}
	switch rol {
	case "test-writer":
		return fmt.Sprintf("Riesgo declarado de la tarjeta: %s.", r)
	case "arquitecto":
		return fmt.Sprintf("Riesgo declarado de la tarjeta: %s. Tope del spec: %s y %s.", r,
			medida(t.Palabras, "palabras"), medida(t.Criterios, "criterios"))
	}
	return ""
}

// riesgoYTope are the declared riesgo of the card task as a launch reads it
// (item.RiesgoDeLanzamiento) and the tope its spec is measured with: the
// card's tope_palabras and tope_criterios over spec.topes.<r> of the
// project's hoom.yaml (roles).
func riesgoYTope(roles *manifest.Manifest, root, task string) (string, spec.Tope) {
	r := item.RiesgoDeLanzamiento(root, task)
	t := roles.TopeDelSpec(r)
	if it, hay, err := item.TarjetaDe(root, task); err == nil && hay {
		if it.TopePalabras != nil {
			t.Palabras = *it.TopePalabras
		}
		if it.TopeCriterios != nil {
			t.Criterios = *it.TopeCriterios
		}
	}
	return r, t
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
	// Note is why the envelope closed where it did: the same text its record
	// keeps, so a --json consumer reads the cause and the action without
	// parsing the terminal (CA-592, finding b5bf6b). registrar sets it on
	// every close that answers with a Result; roto answers with the error.
	Note string `json:"note,omitempty"`
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
//
// proporcion-por-riesgo: the role's values are valores.ResolverConRiesgo
// with the DECLARED riesgo of the card of opt.Task
// (item.RiesgoDeLanzamiento of root: alto when its item cannot be read,
// normal without --task or without item), and a model level is translated
// with the provider Run takes, also the first installed one
// (valores.Elegidos.ConProvider): no level reaches the provider. Unless
// opt.ValoresImpresos, Run prints the line valores: (valores.Linea) once,
// before its steps and outside them. The SystemPrompt of the test-writer and
// of the arquitecto ends in NotaDeRiesgo, after a blank line (the
// test-writer's after the paragraph of the blind tree); the arquitecto's
// tope is the card's own, else spec.topes.<riesgo> of the hoom.yaml Run
// loads. The other roles get no note, and the contract on disk does not
// change.
//
// autonomia-entre-firmas: for a role of shape codigo (today only the
// writer), Run reads the task's protected tests (TestsProtegidos of root)
// and photographs them in the task's tree (FotoProtegidos) BEFORE the run,
// and the scope gate judges them through Policy.Protegidos (see
// RuleTestsDelTestWriter); a test-writer envelope never protects against
// itself. For a blind role the record keeps the paths its transplant
// applied (envelope.Record.Trasplantados), also when it then closes red in
// verify or check, or in scope for fuera-de-scope. A run that died by its
// timeout leaves `corte: "tiempo"` in the record (envelope.CorteTiempo), and
// the envelope closes as always (stage run, no-entregable).
//
// contencion-del-writer: in a project whose hoom.yaml (root's,
// manifest.LoadRoles) has the section `encierro`, the envelope of a role that
// executes (agents.Role.Exec) computes its level (encierro.Nivel, with its
// provider; ninguno with the motive `sin tarea` without opt.Task) and keeps
// it in its record (envelope.Record.Encierro). Inside its step run, before
// launching, it refuses with exit 1, no run in .hoom/runs/, no verdict and
// the usual steps (five, or six with aislar) when encierro.Validar breaks,
// or encierro.ValidarEscribe with the Escribe of its provider (their error is
// the note), or the level does not meet encierro.exigir
// (encierro.NotaExigir is the note). Otherwise its log carries
// encierro.Linea and, under completo, encierro.Aviso with the role, and with
// a task the run starts enclosed (runcmd.StartOptions.Encierro): its run tree
// is the task's tree (the blind tree for the test-writer), its cache the
// role's (encierro.Cache), plus its provider's Escribe and
// encierro.escribibles. When the level the run really had (its sidecar's:
// on Windows, ninguno without the job, and runcmd already killed its tree)
// does not meet encierro.exigir, the envelope closes in its step run with
// exit 1 and encierro.NotaExigir, its territory measured like a failed run's
// (ronda 1, decision 22). Without the section, or for a role that does not
// execute, none of this happens and the record has no `encierro`.
//
// In any project, after the run of a role that executes, the envelope
// compares the agents' config (FotoConfigAgentes, RuleConfigDeAgentes) and,
// for the writer, the git material (RuleMaterialDeGit), BEFORE Take or any
// other git of its own on the tree: when they changed it cuts in scope
// without running git status. It also compares git-ajeno (FotoGitAjeno,
// RuleGitAjeno), which cuts the same way. The note of a cut is the first that
// applies of: the isolation note, the tampering note, NotaTestsDelTestWriter,
// NotaMaterialDeGit, NotaConfigDeAgentes(rol), NotaGitAjeno(rol). Every git
// of hoom on the tree of a role that executes carries core.fsmonitor=false.
// Its verify runs with verifycmd.Options.Sobre (the role's view, the cache
// `verify`, the git material and git-ajeno photographed around the gates):
// when that verdict has verifycmd.GateMaterialDeGit or verifycmd.GateGitAjeno
// in fail, the writer's envelope also registers the high finding of
// RuleMaterialDeGit or RuleGitAjeno with each path and closes in verify,
// no-entregable, exit 1, with NotaMaterialDeGit or NotaGitAjeno; another
// role's closes as any red verdict.
//
// verify-en-el-entorno: the envelope of every role photographs the
// .hoom/local.yaml of root's main checkout, the one verify reads
// (FotoLocal(entorno.Principal(root)) into Snapshot.Local, the folder found
// once before the run; ProtegidoAusente when it is not there) before and
// after the run, also when root is a subdirectory of its repository. A run that created, changed or
// removed it is tampering (RuleTampering, path entorno.Ruta, DetalleLocal):
// the envelope is cut in scope, no-entregable, exit 1, with the tampering
// note and no verify, and its high finding names .hoom/local.yaml. Its verify
// then runs the gates with root's .hoom/local.yaml, like every verify
// (verifycmd.Run).
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
	// the role's values from the PROJECT's hoom.yaml, never the branch's:
	// a branch does not raise its own budget or timeout
	roles, err := manifest.LoadRoles(root)
	if err != nil {
		return Result{}, err
	}
	// review: configures hoom review's passes (isolated, with its effort);
	// an envelope never carries them, whatever the role (CA-400)
	var def manifest.RoleDefaults
	if role.Slug != "reviewer" {
		def = roles.RoleDefaults(role.Slug)
	}
	// the one rule (valores): the command, then hoom.yaml for the declared
	// riesgo of the card of --task, then hoom.yaml, then the provider's own;
	// hoom agent's place proposes no value of its own
	riesgoDeLaTarjeta, tope := riesgoYTope(roles, root, opt.Task)
	elegidos := valores.ResolverConRiesgo(pedidoDe(opt), riesgoDeLaTarjeta, def, valores.Lugar{})
	opt.Provider = elegidos.Provider
	prov, err := pickProvider(opt.Provider)
	if err != nil {
		return Result{}, err
	}
	// a level is translated with the provider the envelope takes, also the
	// one it takes for being installed
	elegidos = elegidos.ConProvider(prov.Name())
	avisos := aplicarValores(prov, elegidos, &opt)
	steps := 5
	if role.Isolated {
		steps = 6 // el arbol ciego se arma y se prueba: eso es un paso, no una nota al pie
	}
	res := Result{Role: role.Slug, Provider: prov.Name(), Dir: dir, Stage: "spec"}
	// El registro del sobre: lo que status y el Studio pueden mirar MIENTRAS
	// esto corre. Se escribe en cada transicion de paso, best-effort: no
	// escribirlo no rompe el sobre que describe. La unica excepcion es el
	// primer registro cuando alguien espera Started (el Studio): sin el no
	// hay sobre que nombrar (CA-335).
	id := strings.TrimSpace(opt.EnvelopeID)
	if id == "" {
		id = envelope.NewID()
	}
	rec := envelope.Record{
		ID: id, Role: role.Slug, Provider: prov.Name(), Task: opt.Task,
		Dir: dir, Stage: "spec", Step: 1, Steps: steps,
		Status: envelope.StatusRunning, ExitCode: -1, StartedAt: time.Now().UTC(),
		PID: os.Getpid(), Pilot: opt.Pilot,
		Columna: opt.Columna, Regla: opt.Regla, Intento: opt.Intento, Ronda: opt.Ronda,
		Hallazgos: opt.Hallazgos, Bloqueantes: opt.Bloqueantes,
	}
	if opt.Columna != "" {
		rec.Pedido = opt.Prompt
	}
	res.EnvelopeID = rec.ID
	// Una sola transicion mueve los dos estados que el sobre mantiene: el
	// resultado que devuelve y el registro que status y el Studio leen. Dos
	// asignaciones gemelas repartidas por el flujo ya se desviaron una vez.
	advance := func(stage string, step int) error {
		res.Stage = stage
		rec.Stage, rec.Step = stage, step
		return envelope.Write(root, rec)
	}
	// seguir es advance para el resto de las transiciones: su registro es
	// best-effort a proposito (CA-202), el error se descarta aca y no en
	// cada llamada.
	seguir := func(stage string, step int) { _ = advance(stage, step) }
	if err := advance("spec", 1); err != nil && opt.Started != nil {
		// quien espera el registro para nombrarlo (el Studio) no recibe un
		// sobre que no existe (CA-335); sin nadie esperando, sigue best-effort
		return res, fmt.Errorf("no pude escribir el registro del sobre: %v", err)
	}
	if opt.Started != nil {
		opt.Started() // el primer registro ya esta en disco: quien espera puede nombrarlo
	}
	fmt.Fprintf(w, "hoom agent: rol %s (%s) en %s\n", role.Slug, prov.Name(), displayDir(opt.Task))
	for _, a := range avisos {
		fmt.Fprintf(w, "  aviso: %s\n", a)
	}
	if !opt.ValoresImpresos {
		// the line valores: of this launch, once, before the steps and not
		// one of them (proporcion-por-riesgo)
		fmt.Fprintf(w, "  %s\n", valores.Linea(elegidos, !prov.Capabilities().Budget && !opt.BudgetSet))
	}
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
	seguir("spec", 1)

	// [2/N] aislar: el rol ciego no obedece la regla de oro, la habita.
	mgr := runcmd.NewManager(root)
	runDir := dir
	var tree *isolate.Tree
	var beforeReal Snapshot
	if role.Isolated {
		seguir("aislar", 2)
		// first of the blind tree's checks, before blindPrechecks, the
		// quarantine (isolate.Open) and the run: a provider that cannot drop
		// the user's config would read what the blind tree hides (CA-592).
		// The envelope's own record is already on disk, as in any refusal.
		if err := IsolationFor(prov, role); err != nil {
			return cerrar(w, root, &rec, res, "aislar", 1, err.Error()), nil
		}
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
		seguir("aislar", 2)
		runDir = tree.Dir
		beforeReal = Take(dir, base)
		contract += blindNote(tree)
		fmt.Fprintf(w, "  %s aislar  arbol ciego %s\n", step(2, steps), display(dir, tree.Dir))
		fmt.Fprintf(w, "                desde %s - %s\n", shortSHA(tree.Commit),
			plural(len(tree.Hidden), "archivo rastreado quedo fuera", "archivos rastreados quedaron fuera"))
	}

	// [N-3/N] run
	seguir("run", steps-3)
	// the encierro (contencion-del-writer): only a role that executes, in a
	// project whose hoom.yaml asks for it; its refusals are inside this step,
	// before launching
	var plan *encierro.Plan
	if role.Exec && roles.Encierro != nil {
		info := encierro.Nivel(opt.Task != "", prov)
		rec.Encierro = &info
		if why := encierroNiega(root, dir, m, *roles.Encierro, info, role.Slug, prov); why != "" {
			keepQuarantine(w, tree, res.Isolation, "el encierro no dejo correr el rol")
			return cerrar(w, root, &rec, res, "run", 1, why), nil
		}
		fmt.Fprintf(w, "    %s\n", encierro.Linea(info))
		if aviso := encierro.Aviso(info, role.Slug); aviso != "" {
			fmt.Fprintf(w, "    %s\n", aviso)
		}
		// with ninguno the plan only carries the level to the run's sidecar
		plan = &encierro.Plan{Info: info, Exigir: roles.Encierro.Exigir}
		if opt.Task != "" && info.Nivel != encierro.NivelNinguno {
			vista, err := encierro.VistaDe(encierro.Lugar{Principal: root, Tarea: opt.Task, Arbol: runDir, Quien: role.Slug,
				Estado: EstadoDe(prov), Escribibles: roles.Encierro.Escribibles, Perfil: m.Profile})
			if err != nil {
				keepQuarantine(w, tree, res.Isolation, "el encierro no se pudo armar")
				return res, roto(root, &rec, err)
			}
			plan.Vista = vista
		}
	}
	// the note of riesgo goes after the contract (and the paragraph of the
	// blind tree), after a blank line, and nothing after it: added here, not
	// in startOptions, which hands the contract on as it gets it
	if nota := NotaDeRiesgo(role.Slug, riesgoDeLaTarjeta, tope); nota != "" {
		contract = strings.TrimRight(contract, "\n") + "\n\n" + nota
	}
	so, warn := startOptions(prov, role, contract, opt)
	so.Dir = runDir
	so.Encierro = plan
	if warn {
		fmt.Fprintf(w, "  aviso: %s no puede imponer un rol de solo lectura; el limite se verifica solo despues del run\n", prov.Name())
	}
	if _, sinShell := NoExecFor(prov, role); sinShell {
		fmt.Fprintf(w, "  aviso: %s no puede quitarle el shell a un rol que escribe; su escritura se verifica solo despues del run\n", prov.Name())
	}
	// the protected tests are read and photographed BEFORE the run: what
	// the gate compares is what the test-writer left, not what the writer
	// says it left (autonomia-entre-firmas)
	pol := PolicyFor(m, role)
	if role.Scope == agents.ScopeCodigo {
		rutas, err := TestsProtegidos(root, opt.Task)
		if err != nil {
			return res, roto(root, &rec, err)
		}
		pol.Protegidos = FotoProtegidos(dir, rutas)
		pol.ProtegidosGit = FotoGitProtegidos(dir, rutas, pol.Protegidos)
		// and what the pilot's save runs of git: the writer does not change
		// it (aclaracion 25)
		pol.MaterialGit = FotoMaterialGit(dir)
	}
	// what runs code in the next session, and git outside the task's
	// branch: photographed for every role that executes, compared after the
	// run before any git of hoom on the tree (contencion-del-writer)
	// the main checkout's .hoom/local.yaml decides what the gates of verify
	// run: photographed around every role's run (verify-en-el-entorno), at
	// the folder verify reads it from, found once before the run
	principal := entorno.Principal(root)
	var previo *fotosPrevias
	if role.Exec {
		previo = tomarPrevias(runDir, dir, principal)
	}
	tomar := func() Snapshot {
		s := Take(runDir, base)
		s.Local = FotoLocal(principal)
		return s
	}
	before := tomar()
	info, err := mgr.Start(so)
	if err != nil {
		keepQuarantine(w, tree, res.Isolation, "el run no arranco")
		return res, roto(root, &rec, err)
	}
	res.RunID = info.ID
	rec.RunID = info.ID
	seguir("run", steps-3)
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
		_ = envelope.Write(root, rec) // best-effort (CA-202)
	})
	res.RunStatus, res.SessionID = st.Status, st.ProviderSessionID
	res.Usage, rec.Usage = st.Usage, st.Usage
	rec.Corte = st.Corte
	if !st.Usage.Empty() {
		fmt.Fprintf(w, "    gasto: %s\n", st.Usage.Summary())
	}
	seguir("run", steps-3)
	if st.ProviderSessionID != "" {
		fmt.Fprintf(w, "    sesion %s - reanudar: hoom agent --role %s --provider %s --resume %s \"<pedido>\"\n",
			st.ProviderSessionID, role.Slug, prov.Name(), st.ProviderSessionID)
	}
	if rec.Encierro != nil && plan != nil {
		// the level the run really had (on Windows, without the job: ninguno)
		if meta, ok := runcmd.ReadMeta(root, info.ID); ok && meta.Encierro != nil {
			rec.Encierro = meta.Encierro
		}
	}
	// BEFORE Take or any other git of hoom on the tree: a fsmonitor or a
	// filter the run left does not run inside hoom before the gate sees it
	if previo != nil {
		if sc, corta := previo.comparar(dir, opt.Task, role, before, beforeReal, tree != nil, pol); corta {
			seguir("scope", steps-2)
			res.Scope = sc
			printScope(w, res.Scope, role, step(steps-2, steps))
			keepQuarantine(w, tree, res.Isolation, "no se trasplanto nada")
			return cerrar(w, root, &rec, res, "scope", 1, notaDelCorte(sc, role.Slug)), nil
		}
	}
	// the level the run really had does not meet exigir (ronda 1, decision
	// 22: Windows did not let Adoptar create the job, and runcmd killed its
	// tree on start): a refusal of exigir, in run, with its territory measured
	// like a failed run's
	niegaExigir := ""
	if plan != nil && rec.Encierro != nil && !encierro.Alcanza(rec.Encierro.Nivel, plan.Exigir) {
		niegaExigir = encierro.NotaExigir(*rec.Encierro, plan.Exigir, role.Slug)
	}
	if niegaExigir != "" || st.Status != runcmd.StatusDone || st.ExitCode != 0 {
		code := st.ExitCode
		if code == 0 {
			code = 1
		}
		fmt.Fprintf(w, "    run %s (exit %d)\n", st.Status, st.ExitCode)
		// A run that failed may have written anyway: its territory is measured
		// exactly like a finished one's — blind or not, with the real tree's
		// photograph — because a resolution closes a finding whether the run
		// ended well or not (CA-438). What it cannot give is a tree to verify.
		res.Scope = Gate(dir, base, opt.Task, role, before, tomar(), pol, cegado(tree, beforeReal, dir, base))
		printScope(w, res.Scope, role, step(steps-2, steps))
		if niegaExigir != "" {
			keepQuarantine(w, tree, res.Isolation, "el encierro no dejo correr el rol")
			return cerrar(w, root, &rec, res, "run", 1, niegaExigir), nil
		}
		keepQuarantine(w, tree, res.Isolation, "el run fallo")
		return cerrar(w, root, &rec, res, "run", code, "el run fallo: su territorio se midio; no se emite veredicto"), nil
	}

	// [N-2/N] scope: the question no prompt can answer, plus the one the
	// blind tree lets us ask — is the blindfold still on?
	seguir("scope", steps-2)
	res.Scope = Gate(dir, base, opt.Task, role, before, tomar(), pol, cegado(tree, beforeReal, dir, base))
	printScope(w, res.Scope, role, step(steps-2, steps))
	if res.Scope.Cuts() {
		keepQuarantine(w, tree, res.Isolation, "no se trasplanto nada")
		return cerrar(w, root, &rec, res, "scope", 1, notaDelCorte(res.Scope, role.Slug)), nil
	}
	// Solo viaja lo que el gate aprobo: la cuarentena es la unica vez que el
	// gate llega ANTES de que el arbol certificable reciba la escritura.
	if tree != nil {
		if err := transplant(w, tree, dir, res.Scope, res.Isolation); err != nil {
			keepQuarantine(w, tree, res.Isolation, "el trasplante fallo a mitad de camino")
			return res, roto(root, &rec, err)
		}
		rec.Trasplantados = trasplantados(res.Isolation.Applied)
	}
	// A role that writes and delivered nothing leaves no new tree to certify.
	// Checked AFTER the cut (tampering always wins) and after the transplant
	// (a finding the blind role created does not die with the quarantine).
	if !role.ReadOnly && len(res.Scope.Delivered()) == 0 {
		return sinEntrega(w, root, &rec, res, role), nil
	}

	// [N-1/N] verify
	seguir("verify", steps-1)
	// a role that executes: its verify's gates run with its view (enclosed
	// when the project asks) and the git material is photographed around
	// them (contencion-del-writer)
	var sobre *verifycmd.Sobre
	if role.Exec {
		sobre = &verifycmd.Sobre{Root: root, Task: opt.Task, Rol: role.Slug}
		if tree != nil {
			sobre.Ciego = tree.Dir // the test-writer's gates write only there
		}
	}
	v, _, err := verifycmd.Run(m, verifycmd.Options{Spec: specPath, Sobre: sobre})
	if err != nil {
		return res, roto(root, &rec, err)
	}
	res.VerdictID, res.Verdict = v.ID, v.Verdict
	rec.VerdictID, rec.Verdict = v.ID, v.Verdict
	fmt.Fprintf(w, "  %s verify  %s (veredicto %s)\n", step(steps-1, steps), color(v.Verdict == "green"), v.ID)
	if role.Scope == agents.ScopeCodigo {
		// the writer's code changed the git material, or git outside its
		// branch (aclaracion 20), during its own verify: the finding of
		// always, and the envelope stops here. Another role's envelope goes
		// on like any red verdict
		material, ajeno := MaterialDelVeredicto(v), GitAjenoDelVeredicto(v)
		if len(material) > 0 {
			res.Scope = registrarCorte(dir, opt.Task, role, res.Scope, material, RuleMaterialDeGit, DetalleMaterialDeGit)
		}
		if len(ajeno) > 0 {
			res.Scope = registrarCorte(dir, opt.Task, role, res.Scope, ajeno, RuleGitAjeno, DetalleGitAjeno)
		}
		switch {
		case len(material) > 0:
			return cerrar(w, root, &rec, res, "verify", 1, NotaMaterialDeGit), nil
		case len(ajeno) > 0:
			return cerrar(w, root, &rec, res, "verify", 1, NotaGitAjeno(role.Slug)), nil
		}
	}

	// [N/N] check
	seguir("check", steps)
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

// trasplantados is the record's copy of what a transplant applied: sorted,
// with "/", nil when nothing was applied (the record then has no key).
func trasplantados(applied []string) []string {
	if len(applied) == 0 {
		return nil
	}
	out := make([]string, len(applied))
	for i, p := range applied {
		out[i] = filepath.ToSlash(p)
	}
	sort.Strings(out)
	return out
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
	res.Note = note
	rec.Stage, rec.Status, rec.ExitCode, rec.Note = res.Stage, res.Status, res.ExitCode, note
	rec.EndedAt = time.Now().UTC()
	_ = envelope.Cerrar(root, *rec) // best-effort (CA-202), retried until it lands (finding de4843)
	return res
}

// roto cierra el registro cuando lo que falla no es el trabajo del rol sino
// el setup (verify que no arranca, un trasplante a medias). El sobre termino
// igual, y un registro que se queda "en curso" para siempre seria una mentira
// que despues alguien tiene que interpretar.
func roto(root string, rec *envelope.Record, err error) error {
	rec.Status, rec.ExitCode, rec.Note = envelope.StatusNotDeliverable, 1, err.Error()
	rec.EndedAt = time.Now().UTC()
	_ = envelope.Cerrar(root, *rec) // best-effort (CA-202), retried until it lands (finding de4843)
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

// pedidoDe is what the command asked: the first level of the rule.
func pedidoDe(o Options) valores.Pedido {
	p := valores.Pedido{Provider: strings.TrimSpace(o.Provider), Model: strings.TrimSpace(o.Model), Effort: strings.TrimSpace(o.Effort)}
	if o.Timeout > 0 {
		p.Timeout = o.Timeout
	}
	if o.BudgetSet {
		b := o.BudgetUSD
		p.BudgetUSD = &b
	}
	return p
}

// aplicarValores puts the values the rule chose in opt, and then, as a
// phase of its own, drops what the provider cannot take. The envelope is
// strict, so an effort the provider ignores would refuse the run: it is
// dropped with the usual warning instead, before the run exists; so is a
// hoom.yaml budget for a provider without a cap. It returns the warnings to
// print.
func aplicarValores(prov providers.Provider, e valores.Elegidos, opt *Options) []string {
	opt.Model, opt.Effort = e.Model, e.Effort
	if e.Origen.Timeout != valores.OrigenProvider {
		// the provider's own is the run's default: opt leaves it to the run
		opt.Timeout = e.Timeout
	}
	caps := prov.Capabilities()
	var avisos []string
	if strings.HasPrefix(e.Origen.BudgetUSD, valores.OrigenHoomYAML) && e.BudgetUSD != nil {
		if caps.Budget {
			opt.BudgetUSD = *e.BudgetUSD
		} else {
			avisos = append(avisos, fmt.Sprintf("%s no acepta tope: agents.%s.budget_usd no se aplica", prov.Name(), opt.roleSlug()))
		}
	}
	if opt.Effort != "" && !caps.Effort {
		avisos = append(avisos, fmt.Sprintf("%s no soporta %s; se ignora", prov.Name(), providers.FieldEffort))
		opt.Effort = ""
	}
	return avisos
}

// roleSlug is the role of opt as hoom.yaml names it.
func (o Options) roleSlug() string {
	if r, err := agents.Lookup(o.Role); err == nil {
		return r.Slug
	}
	return o.Role
}

// startOptions translates the envelope's decision into a run request. Strict
// is not a flag here: without its contract as system prompt there is no role
// to verify, and a silent degradation would be a lie told with evidence.
func startOptions(prov providers.Provider, role agents.Role, contract string, opt Options) (runcmd.StartOptions, bool) {
	so := runcmd.StartOptions{
		Provider: prov.Name(), Prompt: opt.Prompt, Task: opt.Task, Role: role.Slug,
		ResumeID: opt.ResumeID, Model: opt.Model, SystemPrompt: contract,
		MaxTurns: opt.MaxTurns, BudgetUSD: opt.BudgetUSD, Strict: true,
		Effort: opt.Effort, Timeout: opt.Timeout,
		// The envelope is unattended by definition: no human answers the
		// CLI's prompts, so the role gets its tools up front and the scope
		// gate, not a denied Write, is what bounds it.
		Unattended: true,
		// A blind role runs without the user's personal provider config:
		// the blind tree only hides the disk (CA-588). Every other role
		// keeps the CLI the way its user knows it (CA-400).
		Isolated: role.Isolated,
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

// IsolationFor says whether provider p can run role r. A blind role needs a
// provider that runs without the user's personal config: the blind tree hides
// the implementation on disk, but the user's MCP servers (Engram) and the
// CLI's auto memory would hand it back through the context. Unlike
// ReadOnlyFor there is no warn: a blind role that is not blind is not the
// role, so the envelope refuses (spec ciego-sin-config-personal).
func IsolationFor(p providers.Provider, r agents.Role) error {
	if !r.Isolated || p.Capabilities().Isolation {
		return nil
	}
	var aislables []string
	for _, q := range providers.All() {
		if q.Capabilities().Isolation {
			aislables = append(aislables, q.Name())
		}
	}
	return fmt.Errorf("%s no puede correr sin tu configuracion personal (MCP, memoria, hooks): un rol ciego no corre ahi.\n  Accion: usa un provider que pueda aislarse: %s",
		p.Name(), strings.Join(aislables, ", "))
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
	cambios, err := gitx.Cambios(dir)
	if err != nil {
		return nil
	}
	prefix, _ := gitx.Prefix(dir)
	var dirty []string
	for _, c := range cambios {
		p := gitx.RelAlProyecto(filepath.ToSlash(c.Ruta), prefix)
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
	prefix, _ := gitx.Prefix(dir)
	cmd := exec.Command("git", "-c", "core.fsmonitor=false", "show", "HEAD:"+prefix+rel)
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
	fmt.Fprintf(w, "                para descartarla: git worktree remove --force %s\n", t.Worktree())
}

// cegado is what the blind tree lets the gate ask after a blind run: the
// witnesses that came back, the real tree's changes as git lists them, and
// the real tree's two photographs, so a resolution the role wrote OUT of its
// quarantine — even one git ignores — is still a closing. nil = not blind.
func cegado(tree *isolate.Tree, beforeReal Snapshot, dir, base string) *Blind {
	if tree == nil {
		return nil
	}
	realAfter := Take(dir, base)
	return &Blind{Restored: tree.Breaches(), Leaked: delta(beforeReal.Touched, realAfter.Touched),
		Real: &Fotos{Antes: beforeReal, Despues: realAfter}}
}
