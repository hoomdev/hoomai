// Package reviewcmd implements `hoom review`: the cross review as a property
// hoom VERIFIES, not a convention someone has to remember. Three things stop
// being narration here: the lens comes from the evidence (contract 06's rule,
// with the verdict's own line count), the reviewing provider is chosen
// DIFFERENT from the one that wrote, and the result of the review is the
// findings hoom SEES appear under .hoom/findings/ — never what the CLI says
// it recorded.
//
// It is not a second envelope: it composes the parts `hoom agent` exports and
// deliberately skips the two steps that do not belong to a reviewer. Verify
// and check certify a tree that CHANGED, and the reviewer does not change it.
package reviewcmd

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Lentes are the 4 lenses of contract 06, in fixed order.
var Lentes = []string{"readability", "reliability", "resilience", "risk"}

// LenteDominante is the lens of a standard change. It is FIXED instead of
// guessed from the content: guessing would be the model's judgement wearing a
// deterministic costume. Whoever knows their change is of another nature says
// so with --lens.
const LenteDominante = "reliability"

// UmbralLineas is contract 06's threshold, and it comes from the verdict
// (insertions+deletions), never from an eyeball estimate.
const UmbralLineas = 400

// Cross states of a review.
const (
	CrossYes     = "cruzada"
	CrossNo      = "no-cruzada"
	CrossUnknown = "desconocida"
)

// Options is one review invocation.
type Options struct {
	Provider     string // "" = the first installed one that carries the contract and is not the writer's
	Lens         string // "" = deterministic
	Task         string
	Spec         string
	Model        string
	SameProvider bool // allows reviewing with the same provider that wrote
	MaxTurns     int
	BudgetUSD    float64
}

// Pass is one lens: one session, its scope gate and the findings hoom saw
// appear while it ran.
type Pass struct {
	Lens      string               `json:"lens"`
	RunID     string               `json:"run_id,omitempty"`
	RunStatus string               `json:"run_status,omitempty"`
	SessionID string               `json:"provider_session_id,omitempty"`
	Scope     agentcmd.ScopeResult `json:"scope"`
	Findings  []string             `json:"findings"`
}

// Result is the review's answer, identical in text and in JSON.
type Result struct {
	Provider string   `json:"provider,omitempty"`
	Writer   string   `json:"writer,omitempty"` // provider of the last run that WROTE
	Cross    string   `json:"cross"`            // cruzada | no-cruzada | desconocida
	Reason   string   `json:"reason"`           // why those lenses
	Lenses   []string `json:"lenses"`
	Passes   []Pass   `json:"passes"`
	Findings []string `json:"findings"` // union of the passes
	Status   string   `json:"status"`   // revisado | sin-revisar | no-entregable
	ExitCode int      `json:"exit_code"`
}

// Lenses applies contract 06's rule over EVIDENCE, not over judgement. The
// list is computed BEFORE the first run and never changes with what the
// reviewer says.
func Lenses(git gitx.Info, explicit string) ([]string, string, error) {
	if e := strings.ToLower(strings.TrimSpace(explicit)); e != "" {
		if !contains(Lentes, e) {
			return nil, "", fmt.Errorf("lente desconocida %q (validas: %s)", explicit, strings.Join(Lentes, ", "))
		}
		return []string{e}, "lente pedida a mano", nil
	}
	if len(git.ChangedFiles) == 0 {
		return nil, "no hay cambios contra la base: no hay nada que revisar", nil
	}
	if soloDocs(git.ChangedFiles) {
		return nil, "el cambio es solo documentacion: no se invoca review", nil
	}
	if p := rutaDeRiesgo(git.ChangedFiles); p != "" {
		return Lentes, fmt.Sprintf("el cambio toca %s: las 4 lentes", p), nil
	}
	if n := git.Insertions + git.Deletions; n > UmbralLineas {
		return Lentes, fmt.Sprintf("%d lineas cambiadas (>%d): las 4 lentes", n, UmbralLineas), nil
	}
	return []string{LenteDominante}, "cambio estandar: la lente dominante", nil
}

// docExts and docDirs decide what "only documentation" means. A change that
// only touches prose has 0 lenses: the contract says the reviewer is not
// invoked, and not burning a session is part of respecting that.
var docExts = map[string]bool{".md": true, ".txt": true, ".rst": true, ".adoc": true}

func soloDocs(files []string) bool {
	for _, f := range files {
		l := strings.ToLower(f)
		switch {
		case docExts[filepath.Ext(l)]:
		case strings.HasPrefix(l, "docs/"), strings.Contains(l, "/docs/"):
		case filepath.Base(l) == "license", filepath.Base(l) == "notice":
		default:
			return false
		}
	}
	return true
}

// marcasDeRiesgo is the deterministic reading of "security, auth, payments":
// a coarse list of path substrings. It errs on the side of MORE review, and
// the noise is honest — the alternative is letting the model pick its own
// lens, which contract 06 forbids.
var marcasDeRiesgo = []string{
	"auth", "login", "password", "passwd", "secret", "token", "cred",
	"crypt", "sign", "pago", "payment", "billing", "invoice", "dte",
	"permission", "permiso", "sandbox", "sudo",
}

func rutaDeRiesgo(files []string) string {
	for _, f := range files {
		l := strings.ToLower(f)
		for _, mark := range marcasDeRiesgo {
			if strings.Contains(l, mark) {
				return f
			}
		}
	}
	return ""
}

// Run executes the review. Failures the review is MEANT to report (a refusal
// for not being cross, a failed run, a violated scope) come back as a Result
// with an exit code; only a broken setup returns an error.
func Run(root, base string, opt Options, w io.Writer) (Result, error) {
	role, err := agents.Lookup("reviewer")
	if err != nil {
		return Result{}, err
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
	git := gitx.Snapshot(dir, base)
	lentes, motivo, err := Lenses(git, opt.Lens)
	if err != nil {
		return Result{}, err
	}
	res := Result{Cross: CrossUnknown, Reason: motivo, Lenses: lentes, Passes: []Pass{}, Findings: []string{}}

	fmt.Fprintf(w, "hoom review: %s, +%d/-%d lineas contra %s\n",
		plural(len(git.ChangedFiles), "archivo cambiado", "archivos cambiados"),
		git.Insertions, git.Deletions, base)
	if len(lentes) == 0 {
		fmt.Fprintf(w, "  lentes      ninguna - %s\n", motivo)
		return finish(w, res, "sin-revisar", 0, motivo), nil
	}
	fmt.Fprintf(w, "  lentes      %s (%s)\n", strings.Join(lentes, ", "), motivo)

	contract, err := agents.Contract(dir, role)
	if err != nil {
		return res, err
	}

	// cruzada: quien escribio sale del meta del run, no de la memoria de nadie
	writer, hayWriter := writerOf(root, dir)
	if hayWriter {
		res.Writer = writer.Provider
	}
	prov, err := pickProvider(opt.Provider, res.Writer)
	if err != nil {
		return res, err
	}
	res.Provider = prov.Name()
	switch {
	case !hayWriter:
		res.Cross = CrossUnknown
		fmt.Fprintf(w, "  reviewer    %s - cruzada DESCONOCIDA (no hay run previo registrado en este arbol)\n", prov.Name())
	case writer.Provider == prov.Name():
		res.Cross = CrossNo
		fmt.Fprintf(w, "  reviewer    %s - NO seria cruzada: el writer corrio en %s (run %s)\n",
			prov.Name(), writer.Provider, writer.ID)
	default:
		res.Cross = CrossYes
		fmt.Fprintf(w, "  reviewer    %s - cruzada SI (el writer corrio en %s, run %s)\n",
			prov.Name(), writer.Provider, writer.ID)
	}
	if res.Cross == CrossNo && !opt.SameProvider {
		fmt.Fprintf(w, "  el mismo modelo que escribio no puede ser el que revisa: elegi otro provider\n"+
			"  (mira 'hoom providers') o asumilo con: hoom review --provider %s --same-provider\n", prov.Name())
		return finish(w, res, "no-entregable", 1, "la review no seria cruzada"), nil
	}

	readOnly, exec, warn := agentcmd.ReadOnlyFor(prov, role)
	if warn {
		fmt.Fprintf(w, "  aviso: %s no puede imponer un rol de solo lectura; el limite se verifica solo despues del run\n", prov.Name())
	}
	v := ultimoVeredicto(dir)
	pol := agentcmd.PolicyFor(m, role)
	mgr := runcmd.NewManager(root)

	for i, lens := range lentes {
		fmt.Fprintf(w, "  [%d/%d] %s\n", i+1, len(lentes), lens)
		pass := Pass{Lens: lens, Findings: []string{}}
		before := agentcmd.Take(dir, base)
		antes := idsDeHallazgos(dir, base)

		info, err := mgr.Start(runcmd.StartOptions{
			Provider: prov.Name(), Prompt: pedido(base, lens, git, opt.Spec, v, role, prov.Name()),
			Task: opt.Task, Role: role.Slug, SystemPrompt: contract,
			Model: opt.Model, ReadOnly: readOnly, Exec: exec,
			MaxTurns: opt.MaxTurns, BudgetUSD: opt.BudgetUSD, Strict: true,
		})
		if err != nil {
			return res, err
		}
		pass.RunID = info.ID
		fmt.Fprintf(w, "    run       %s - narracion en .hoom/runs/%s.jsonl\n", info.ID, info.ID)
		st := stream(mgr, info.ID, w)
		pass.RunStatus, pass.SessionID = st.Status, st.ProviderSessionID

		if st.Status != runcmd.StatusDone || st.ExitCode != 0 {
			fmt.Fprintf(w, "    run %s (exit %d)\n", st.Status, st.ExitCode)
			res.Passes = append(res.Passes, pass)
			return finish(w, res, "no-entregable", 1, "el run del reviewer fallo"), nil
		}

		// Los hallazgos del reviewer se cuentan ANTES de que hoom escriba los
		// suyos por violaciones: el arbitro no se cuenta como jugador.
		pass.Findings = nuevos(antes, idsDeHallazgos(dir, base))
		pass.Scope = agentcmd.Gate(dir, base, role, before, agentcmd.Take(dir, base), pol)
		printScope(w, pass.Scope, role)
		printFindings(w, pass.Findings)
		res.Passes = append(res.Passes, pass)
		res.Findings = append(res.Findings, pass.Findings...)
		if !pass.Scope.OK {
			return finish(w, res, "no-entregable", 1, "el reviewer escribio fuera de su territorio"), nil
		}
	}
	return finish(w, res, "revisado", 0, ""), nil
}

// writerOf answers "who wrote this tree?" with the run metas: the most recent
// run of THIS directory whose role is not read-only. A previous review is not
// a writer; a run without a role (`hoom run`) counts as one, because nothing
// says it did not write.
func writerOf(root, dir string) (runcmd.Meta, bool) {
	for _, meta := range runcmd.Metas(root) {
		if meta.Dir != dir {
			continue
		}
		if r, err := agents.Lookup(meta.Role); err == nil && r.ReadOnly {
			continue
		}
		return meta, true
	}
	return runcmd.Meta{}, false
}

// pickProvider honors an explicit choice and otherwise takes the first
// installed provider that carries the contract AND is not the writer's — so
// the review is cross by construction. When the only candidate is the
// writer's own, it comes back anyway: refusing is the caller's decision, and
// it says so out loud.
func pickProvider(name, writer string) (providers.Provider, error) {
	if n := strings.TrimSpace(name); n != "" {
		p, err := providers.Lookup(n)
		if err != nil {
			return nil, err
		}
		if !p.Capabilities().SystemPrompt {
			return nil, providers.ErrUnsupported{Provider: p.Name(), Fields: []string{providers.FieldSystemPrompt}}
		}
		return p, nil
	}
	var mismo providers.Provider
	for _, info := range providers.Detect() {
		if !info.Installed || !info.Capabilities.SystemPrompt {
			continue
		}
		p, err := providers.Lookup(info.Name)
		if err != nil {
			continue
		}
		if info.Name != writer {
			return p, nil
		}
		if mismo == nil {
			mismo = p
		}
	}
	if mismo != nil {
		return mismo, nil
	}
	return nil, fmt.Errorf("ningun provider instalado soporta system_prompt, que 'hoom review' exige para dar el contrato del rol (mira 'hoom providers')")
}

// pedido is the reviewer's dossier: deterministic, short, and built from
// evidence. The diff it takes itself — it has a shell.
func pedido(base, lens string, git gitx.Info, spec string, v *verdict.Verdict, role agents.Role, provider string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Revisa el cambio de esta rama con la lente %s. Solo esa lente.\n", lens)
	fmt.Fprintf(&b, "Base: %s. El diff lo sacas vos: git diff %s...\n", base, base)
	fmt.Fprintf(&b, "Tamano: %d archivos, +%d/-%d lineas.\n", len(git.ChangedFiles), git.Insertions, git.Deletions)
	if n := len(git.ChangedFiles); n > 0 {
		lista := git.ChangedFiles
		if n > 40 {
			lista = lista[:40]
		}
		fmt.Fprintf(&b, "Archivos: %s", strings.Join(lista, ", "))
		if n > 40 {
			fmt.Fprintf(&b, " (+%d mas)", n-40)
		}
		b.WriteString("\n")
	}
	if v != nil {
		fmt.Fprintf(&b, "Veredicto vigente: %s (%s).\n", v.ID, v.Verdict)
	} else {
		b.WriteString("No hay veredicto vigente: la review no reemplaza a 'hoom verify'.\n")
	}
	if strings.TrimSpace(spec) != "" {
		fmt.Fprintf(&b, "Spec: %s\n", spec)
	}
	fmt.Fprintf(&b, "Registra cada hallazgo que sobreviva su propia lectura con:\n"+
		"  hoom finding add --sev low|medium|high --lens %s --file <ruta> --author %s@%s \"<descripcion con archivo:linea>\"\n",
		lens, role.Slug, provider)
	b.WriteString("El chat no es registro: lo que no quede como hallazgo, no paso.\n")
	b.WriteString("No edites codigo: este arbol es de solo lectura para vos.\n")
	return b.String()
}

// ultimoVeredicto reads the current verdict for the dossier. Its absence is
// information too, so it is never an error.
func ultimoVeredicto(dir string) *verdict.Verdict {
	all, err := verdict.LoadAll(dir)
	if err != nil {
		return nil
	}
	return verdict.LatestComplete(all)
}

// idsDeHallazgos photographs the findings of the tree. The review's result is
// the difference between two of these — hoom's own observation, not the
// narration of the CLI.
func idsDeHallazgos(dir, base string) map[string]bool {
	out := map[string]bool{}
	items, _, err := finding.List(dir, base, false)
	if err != nil {
		return out
	}
	for _, it := range items {
		out[it.Finding.ID] = true
	}
	return out
}

func nuevos(antes, despues map[string]bool) []string {
	var out []string
	for id := range despues {
		if !antes[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
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

func printScope(w io.Writer, sc agentcmd.ScopeResult, role agents.Role) {
	if sc.OK {
		fmt.Fprintf(w, "    scope     %s, 0 fuera de scope\n",
			plural(len(sc.Touched), "archivo tocado", "archivos tocados"))
		return
	}
	fmt.Fprintf(w, "    scope     ROJO - %s (rol %s, scope %s):\n",
		plural(len(sc.Violations), "violacion", "violaciones"), role.Slug, role.Scope)
	for _, v := range sc.Violations {
		id := ""
		if v.FindingID != "" {
			id = " [" + v.FindingID + "]"
		}
		fmt.Fprintf(w, "                %s (%s): %s%s\n", v.Path, v.Rule, v.Detail, id)
	}
}

func printFindings(w io.Writer, ids []string) {
	if len(ids) == 0 {
		fmt.Fprintln(w, "    hallazgos 0 nuevos (una review sin hallazgos es informacion, no un error)")
		return
	}
	fmt.Fprintf(w, "    hallazgos %d nuevos: %s\n", len(ids), strings.Join(ids, ", "))
}

func finish(w io.Writer, res Result, status string, code int, note string) Result {
	res.Status, res.ExitCode = status, code
	switch {
	case code != 0:
		line := "hoom review: NO ENTREGABLE"
		if note != "" {
			line += " - " + note
		}
		fmt.Fprintln(w, line)
	case status == "sin-revisar":
		fmt.Fprintf(w, "hoom review: SIN REVISAR - %s\n", note)
	case len(res.Findings) == 0:
		fmt.Fprintln(w, "hoom review: REVISADO - 0 hallazgos nuevos")
	default:
		fmt.Fprintf(w, "hoom review: REVISADO - %s (hoom finding list --open)\n",
			plural(len(res.Findings), "hallazgo nuevo", "hallazgos nuevos"))
	}
	return res
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func plural(n int, uno, varios string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, uno)
	}
	return fmt.Sprintf("%d %s", n, varios)
}
