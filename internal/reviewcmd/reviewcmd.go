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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Lentes are the 4 lenses of contract 06, in the fixed order they run: risk
// first, so that when the provider's quota cuts a review the lens that gave
// the highest findings has already run; reliability keeps the second place.
var Lentes = []string{"risk", "reliability", "resilience", "readability"}

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
	// CrossDeclared: nobody saw the writer write (no run sidecar), but the
	// card declares interactive sessions and the reviewer is none of them. It
	// is never promoted to CrossYes.
	CrossDeclared = "cruzada-declarada"
)

// Options is one review invocation.
type Options struct {
	Provider     string // "" = the first installed one that carries the contract and is not the writer's
	Lens         string // "" = deterministic
	Task         string
	Spec         string
	Model        string
	Effort       string // reasoning effort, provider vocabulary; "" = review.effort of hoom.yaml
	SameProvider bool   // allows reviewing with the same provider that wrote
	MaxTurns     int
	BudgetUSD    float64
	// EnvelopeID and Started: the same contract as agentcmd.Options.
	EnvelopeID string
	Started    func()
	Pilot      bool // same contract as agentcmd.Options.Pilot
	// HoomBin is the hoom the reviewer must call to register its findings:
	// "" = this very binary (os.Executable). Only when that cannot be
	// resolved, or on Windows, does the pedido fall back to plain `hoom`,
	// which the reviewer's shell resolves by PATH.
	HoomBin string
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
	// Usage is what the provider reported for this pass, with its own
	// semantics (CA-197, CA-198); nil when it reported nothing.
	Usage *providers.Usage `json:"usage,omitempty"`
}

// Evidencia is the frozen evidence every lens receives: the candidate's diff
// and the spec text, built once per review.
type Evidencia struct {
	Diff   []byte // unified patch of the candidate
	Spec   []byte // spec text; empty without --spec
	Bytes  int    // len(Diff) + len(Spec)
	SHA256 string // hex sha256 of Diff followed by Spec
}

// Evidence builds the review evidence of the change candidate in dir: the
// patch of every changed file outside .hoom/ against the merge-base of base
// (working tree included), plus the spec text. spec is the path as the user
// gave it, relative to dir; "" = none. A spec that does not exist in the
// tree leaves Spec empty (the dossier says so); any other read error is one.
func Evidence(dir, base string, git gitx.Info, spec string) (Evidencia, error) {
	var files []string
	for _, f := range git.ChangedFiles {
		if !strings.HasPrefix(filepath.ToSlash(f), ".hoom/") {
			files = append(files, f)
		}
	}
	diff, err := gitx.CandidatePatch(dir, base, files)
	if err != nil {
		return Evidencia{}, fmt.Errorf("no pude armar el diff de la evidencia: %v", err)
	}
	var texto []byte
	if s := strings.TrimSpace(spec); s != "" {
		path := s
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		texto, err = os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return Evidencia{}, fmt.Errorf("no pude leer el spec %s: %v", s, err)
		}
	}
	h := sha256.New()
	h.Write(diff)
	h.Write(texto)
	return Evidencia{Diff: diff, Spec: texto, Bytes: len(diff) + len(texto), SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// kib rounds bytes up to KiB, the unit the review prints.
func kib(n int) int { return (n + 1023) / 1024 }

// resolveOptions fills what the caller left empty from the `review:` section
// of hoom.yaml: explicit option > hoom.yaml > empty. same_provider only ever
// permits: it never turns a cross review into a non-cross one.
func resolveOptions(opt Options, m *manifest.Manifest) Options {
	opt.Provider, opt.Model, opt.Effort = strings.TrimSpace(opt.Provider), strings.TrimSpace(opt.Model), strings.TrimSpace(opt.Effort)
	if m == nil || m.Review == nil {
		return opt
	}
	r := m.Review
	if opt.Provider == "" {
		opt.Provider = strings.TrimSpace(r.Provider)
	}
	if opt.Model == "" {
		opt.Model = strings.TrimSpace(r.Model)
	}
	if opt.Effort == "" {
		opt.Effort = strings.TrimSpace(r.Effort)
	}
	if r.SameProvider != nil && *r.SameProvider {
		opt.SameProvider = true
	}
	return opt
}

// elegido renders a model or effort that may not have been chosen.
func elegido(v string) string {
	if v == "" {
		return "por defecto del provider (no elegido)"
	}
	return v
}

// Result is the review's answer, identical in text and in JSON.
type Result struct {
	Provider string `json:"provider,omitempty"`
	Writer   string `json:"writer,omitempty"` // provider of the last run that WROTE
	Cross    string `json:"cross"`            // cruzada | no-cruzada | cruzada-declarada | desconocida
	// WritersDeclared are the providers of the item's interactive sessions:
	// declared, never observed.
	WritersDeclared []string `json:"writers_declared"`
	Reason          string   `json:"reason"` // why those lenses
	Lenses          []string `json:"lenses"`
	Passes          []Pass   `json:"passes"`
	Findings        []string `json:"findings"` // union of the passes
	Status          string   `json:"status"`   // revisado | sin-revisar | no-entregable
	ExitCode        int      `json:"exit_code"`
	// RecordID names the review record written in .hoom/reviews/ when the
	// review ended revisado; empty otherwise.
	RecordID string `json:"record_id,omitempty"`
	// Notes: the same notes the record keeps, for --json.
	Notes []string `json:"notes,omitempty"`
	// Model and Effort as requested ("" = the provider's default); Isolated
	// and the evidence the lenses received.
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	Isolated       bool   `json:"isolated"`
	EvidenceBytes  int    `json:"evidence_bytes"`
	EvidenceSHA256 string `json:"evidence_sha256"`
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
	opt = resolveOptions(opt, m)
	git := gitx.Snapshot(dir, base)
	lentes, motivo, err := Lenses(git, opt.Lens)
	if err != nil {
		return Result{}, err
	}
	res := Result{Cross: CrossUnknown, Reason: motivo, Lenses: lentes, Passes: []Pass{}, Findings: []string{},
		WritersDeclared: declaredWriters(root, taskOf(opt))}

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

	// cruzada: quien escribio sale del meta del run, no de la memoria de nadie.
	// Las sesiones interactivas que declara el item suman writers DECLARADOS:
	// pueden volver una review no cruzada, nunca cruzada.
	writer, hayWriter := writerOf(root, dir)
	if hayWriter {
		res.Writer = writer.Provider
	}
	evitar := append([]string{}, res.WritersDeclared...)
	if res.Writer != "" {
		evitar = append(evitar, res.Writer)
	}
	prov, err := pickProvider(opt.Provider, evitar)
	if err != nil {
		return res, err
	}
	res.Provider = prov.Name()
	declarado := contains(res.WritersDeclared, prov.Name())
	switch {
	case hayWriter && writer.Provider == prov.Name():
		res.Cross = CrossNo
		fmt.Fprintf(w, "  reviewer    %s - NO seria cruzada: el writer corrio en %s (run %s)\n",
			prov.Name(), writer.Provider, writer.ID)
	case declarado:
		res.Cross = CrossNo
		fmt.Fprintf(w, "  reviewer    %s - NO seria cruzada: la tarea declara una sesion interactiva de %s (writer declarado)\n",
			prov.Name(), prov.Name())
	case hayWriter:
		res.Cross = CrossYes
		fmt.Fprintf(w, "  reviewer    %s - cruzada SI (el writer corrio en %s, run %s)\n",
			prov.Name(), writer.Provider, writer.ID)
	case len(res.WritersDeclared) > 0:
		res.Cross = CrossDeclared
		fmt.Fprintf(w, "  reviewer    %s - cruzada DECLARADA: ningun run registro al writer; la tarea declara sesiones de %s\n",
			prov.Name(), strings.Join(res.WritersDeclared, ", "))
	default:
		res.Cross = CrossUnknown
		fmt.Fprintf(w, "  reviewer    %s - cruzada DESCONOCIDA (no hay run previo registrado en este arbol)\n", prov.Name())
	}
	if res.Cross == CrossNo && !opt.SameProvider {
		fmt.Fprintf(w, "  el mismo modelo que escribio no puede ser el que revisa: elegi otro provider\n"+
			"  (mira 'hoom providers') o asumilo con: hoom review --provider %s --same-provider\n"+
			"  o con review.same_provider: true en hoom.yaml\n", prov.Name())
		return finish(w, res, "no-entregable", 1, "la review no seria cruzada"), nil
	}

	// La evidencia se congela UNA vez: las 4 lentes reciben los mismos bytes,
	// enteros. Si no entra en el tope, no se corta: no corre ninguna lente.
	ev, err := Evidence(dir, base, git, opt.Spec)
	if err != nil {
		return res, err
	}
	isolated, tope := m.ReviewIsolated(), m.ReviewMaxEvidenceKiB()
	res.Model, res.Effort, res.Isolated = opt.Model, opt.Effort, isolated
	res.EvidenceBytes, res.EvidenceSHA256 = ev.Bytes, ev.SHA256
	fmt.Fprintf(w, "  modelo      %s\n", elegido(opt.Model))
	fmt.Fprintf(w, "  esfuerzo    %s\n", elegido(opt.Effort))
	if isolated {
		fmt.Fprintln(w, "  aislado     si - sin la config personal del provider")
	} else {
		fmt.Fprintln(w, "  aislado     no - review.isolated: false en hoom.yaml")
	}
	fmt.Fprintf(w, "  evidencia   %d KiB (diff %d + spec %d), tope %d KiB - sha256 %s\n",
		kib(ev.Bytes), kib(len(ev.Diff)), kib(len(ev.Spec)), tope, ev.SHA256[:12])
	if ev.Bytes > tope*1024 {
		return finish(w, res, "no-entregable", 1, fmt.Sprintf(
			"la evidencia (%d KiB) pasa el tope (%d KiB): hoom no la corta; parti el cambio o subi review.max_evidence_kib si el modelo del reviewer la aguanta",
			kib(ev.Bytes), tope)), nil
	}

	readOnly, exec, warn := agentcmd.ReadOnlyFor(prov, role)
	if warn {
		fmt.Fprintf(w, "  aviso: %s no puede imponer un rol de solo lectura; el limite se verifica solo despues del run\n", prov.Name())
	}
	v := ultimoVeredicto(dir)
	pol := agentcmd.PolicyFor(m, role)
	mgr := runcmd.NewManager(root)
	bin, err := hoomBin(opt)
	if err != nil {
		fmt.Fprintf(w, "  aviso: no pude resolver este binario (%v): el reviewer usara el hoom de su PATH\n", err)
	}
	var notas []string

	// El registro del sobre de la review: lo mismo que deja `hoom agent`, asi
	// el tablero ve la review en curso, cortada o fallida como a cualquier
	// rol. Nace ANTES de la primera pasada: lo que se decidio antes (sin
	// lentes, no cruzada) no corrio nada y no deja registro.
	id := strings.TrimSpace(opt.EnvelopeID)
	if id == "" {
		id = envelope.NewID()
	}
	rec := envelope.Record{
		ID: id, Role: role.Slug, Provider: prov.Name(), Task: taskOf(opt), Dir: dir, Spec: opt.Spec,
		Stage: "run", Step: 1, Steps: len(lentes), Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: time.Now().UTC(), PID: os.Getpid(), Pilot: opt.Pilot,
	}
	if err := envelope.Write(root, rec); err != nil && opt.Started != nil {
		// el Studio espera el registro para responder: sin el, no hay sobre
		// que nombrar (CA-335); desde la terminal sigue best-effort
		return res, fmt.Errorf("no pude escribir el registro del sobre: %v", err)
	}
	if opt.Started != nil {
		opt.Started()
	}
	// cerrarSobre settles the envelope record; terminar is the one terminal
	// transition of a review that ends in a Result: the Result (finish) and
	// the record move together, so they never tell two stories.
	cerrarSobre := func(stage string, code int, note string) {
		rec.Stage, rec.ExitCode, rec.Note = stage, code, note
		rec.Status = envelope.StatusDeliverable
		if code != 0 {
			rec.Status = envelope.StatusNotDeliverable
		}
		rec.EndedAt = time.Now().UTC()
		_ = envelope.Write(root, rec) // best-effort (CA-202)
	}
	terminar := func(status string, code int, stage, note string) Result {
		res = finish(w, res, status, code, note)
		cerrarSobre(stage, code, note)
		return res
	}

	comun := pedidoComun(base, git, opt.Spec, v, ev)
	var usos []LensUsage
	var total providers.Usage
	for i, lens := range lentes {
		fmt.Fprintf(w, "  [%d/%d] %s\n", i+1, len(lentes), lens)
		pass := Pass{Lens: lens, Findings: []string{}}
		before := agentcmd.Take(dir, base)
		antes := idsDeHallazgos(dir, base)
		rec.Stage, rec.Step, rec.RunID = "run", i+1, ""
		_ = envelope.Write(root, rec) // best-effort (CA-202)

		info, err := mgr.Start(runcmd.StartOptions{
			Provider: prov.Name(), Prompt: pedido(comun, lens, role, prov.Name(), bin),
			Task: opt.Task, FindingTask: findingTask(opt), Role: role.Slug, SystemPrompt: contract,
			Model: opt.Model, Effort: opt.Effort, Isolated: isolated, ReadOnly: readOnly, Exec: exec,
			MaxTurns: opt.MaxTurns, BudgetUSD: opt.BudgetUSD, Strict: true,
		})
		if err != nil {
			cerrarSobre("run", 1, err.Error())
			return res, err
		}
		pass.RunID = info.ID
		rec.RunID = info.ID
		_ = envelope.Write(root, rec) // best-effort (CA-202)
		fmt.Fprintf(w, "    run       %s - narracion en .hoom/runs/%s.jsonl\n", info.ID, info.ID)
		st := stream(mgr, info.ID, w)
		pass.RunStatus, pass.SessionID = st.Status, st.ProviderSessionID
		if !st.Usage.Empty() {
			u := *st.Usage
			pass.Usage = &u
			usos = append(usos, LensUsage{Lens: lens, InputTokens: u.InputTokens,
				CachedTokens: u.CachedTokens, OutputTokens: u.OutputTokens, Turns: u.Turns})
			total.InputTokens += u.InputTokens
			total.CachedTokens += u.CachedTokens
			total.OutputTokens += u.OutputTokens
		}

		if st.Status != runcmd.StatusDone || st.ExitCode != 0 {
			fmt.Fprintf(w, "    run %s (exit %d)\n", st.Status, st.ExitCode)
			printGasto(w, pass.Usage)
			res.Passes = append(res.Passes, pass)
			return terminar("no-entregable", 1, "run", "el run del reviewer fallo"), nil
		}

		// Los hallazgos del reviewer se cuentan ANTES de que hoom escriba los
		// suyos por violaciones: el arbitro no se cuenta como jugador.
		pass.Findings = nuevos(antes, idsDeHallazgos(dir, base))
		pass.Scope = agentcmd.Gate(dir, base, opt.Task, role, before, agentcmd.Take(dir, base), pol, nil)
		printScope(w, pass.Scope, role)
		printFindings(w, pass.Findings)
		printGasto(w, pass.Usage)
		if t := findingTask(opt); t != "" {
			if fuera := fueraDeLaTarea(dir, base, pass.Findings, t); len(fuera) > 0 {
				nota := fmt.Sprintf("hallazgos fuera de la tarea %s de la review (sin tarea o con otra): %s",
					t, strings.Join(fuera, ", "))
				fmt.Fprintf(w, "    aviso: %s\n", nota)
				notas = append(notas, nota)
			}
		}
		res.Passes = append(res.Passes, pass)
		res.Findings = append(res.Findings, pass.Findings...)
		if !pass.Scope.OK {
			return terminar("no-entregable", 1, "scope", "el reviewer escribio fuera de su territorio"), nil
		}
	}
	// The trace of the review, clean or not: without it a review that found
	// nothing would leave nothing on disk, and the board could not tell it
	// from a review that never happened.
	registro, err := WriteRecord(dir, Record{
		Task: taskOf(opt), Spec: opt.Spec, Fingerprint: git.ChangeFingerprint,
		VerdictID: verdictID(v), Verdict: verdictColor(v),
		Lenses: append([]string(nil), lentes...), Provider: res.Provider, Writer: res.Writer,
		Cross: res.Cross, Findings: append([]string{}, res.Findings...),
		WritersDeclared: append([]string{}, res.WritersDeclared...), Notes: notas,
		Model: opt.Model, Effort: opt.Effort, Isolated: isolated,
		EvidenceBytes: ev.Bytes, EvidenceSHA256: ev.SHA256, Usage: usos,
	})
	res.Notes = notas
	fmt.Fprintf(w, "  gasto       %s: entrada %d - cache %d - salida %d\n",
		plural(len(res.Passes), "lente", "lentes"), total.InputTokens, total.CachedTokens, total.OutputTokens)
	if err != nil {
		// without its record the review did not happen for the board
		// (CA-293): it cannot end revisado
		return terminar("no-entregable", 1, "registro", "no pude escribir el registro de review: "+err.Error()), nil
	}
	res.RecordID = registro.ID
	fmt.Fprintf(w, "  registro    .hoom/%s/%s.json (commitealo: es el rastro de esta review)\n", RecordsDir, registro.ID)
	return terminar("revisado", 0, "ok", ""), nil
}

// declaredWriters are the providers of the interactive sessions the task's
// item declares in root: distinct and sorted. A missing or unreadable item
// declares none.
func declaredWriters(root, task string) []string {
	out := []string{}
	if strings.TrimSpace(task) == "" || !item.ValidSlug(task) {
		return out
	}
	it, err := item.Load(root, task)
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	for _, s := range it.Sesiones {
		if !seen[s.Provider] {
			seen[s.Provider] = true
			out = append(out, s.Provider)
		}
	}
	sort.Strings(out)
	return out
}

// taskOf is the task a review belongs to: --task, or the task of its spec.
func taskOf(opt Options) string {
	if t := strings.TrimSpace(opt.Task); t != "" {
		return t
	}
	if s := strings.TrimSpace(opt.Spec); s != "" {
		return finding.TaskOfSpec(s)
	}
	return ""
}

// findingTask is the task the reviewer's own findings carry (HOOM_TASK):
// the review's task, the same one its record gets (CA-293), so a review of
// a spec without --task still ties them to the card. A spec whose name is
// not an item slug gives none: no card can carry it.
func findingTask(opt Options) string {
	if t := taskOf(opt); item.ValidSlug(t) {
		return t
	}
	return ""
}

func verdictID(v *verdict.Verdict) string {
	if v == nil {
		return ""
	}
	return v.ID
}

func verdictColor(v *verdict.Verdict) string {
	if v == nil {
		return ""
	}
	return v.Verdict
}

// writerOf answers "who wrote this tree?" with the run metas: the most recent
// run of THIS directory whose role is not read-only. A previous review is not
// a writer, and neither is a spec author: it wrote the spec, not the code
// under review. A run without a role (`hoom run`) counts as one, because
// nothing says it did not write.
func writerOf(root, dir string) (runcmd.Meta, bool) {
	for _, meta := range runcmd.Metas(root) {
		if meta.Dir != dir {
			continue
		}
		if !agents.WritesCode(meta.Role) {
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
func pickProvider(name string, writers []string) (providers.Provider, error) {
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
		if !contains(writers, info.Name) {
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

// pedidoComun is the part of the reviewer's dossier every lens shares:
// deterministic and built from evidence. hoom froze the evidence, so the
// reviewer does not take the diff itself; its shell is for context.
func pedidoComun(base string, git gitx.Info, spec string, v *verdict.Verdict, ev Evidencia) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Revisa el cambio de esta rama. La evidencia completa esta abajo, congelada por hoom (sha256 %s, %d KiB): "+
		"no vuelvas a sacar el diff; lee otros archivos solo por rangos y solo si hace falta.\n", ev.SHA256, kib(ev.Bytes))
	fmt.Fprintf(&b, "Base: %s. Tamano: %d archivos, +%d/-%d lineas.\n", base, len(git.ChangedFiles), git.Insertions, git.Deletions)
	if v != nil {
		fmt.Fprintf(&b, "Veredicto vigente: %s (%s).\n", v.ID, v.Verdict)
	} else {
		b.WriteString("No hay veredicto vigente: la review no reemplaza a 'hoom verify'.\n")
	}
	if s := strings.TrimSpace(spec); s != "" {
		if len(ev.Spec) == 0 {
			fmt.Fprintf(&b, "Spec: %s (no existe en este arbol)\n", s)
		} else {
			fmt.Fprintf(&b, "Spec: %s\n", s)
			fmt.Fprintf(&b, "=== spec %s ===\n", s)
			writeBlock(&b, ev.Spec)
		}
	}
	b.WriteString("=== diff ===\n")
	writeBlock(&b, ev.Diff)
	b.WriteString("=== fin de la evidencia ===\n")
	return b.String()
}

// writeBlock writes a block of the evidence ending in exactly one newline
// of its own, so the next marker always starts its own line.
func writeBlock(b *strings.Builder, raw []byte) {
	b.Write(raw)
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		b.WriteString("\n")
	}
}

// pedido is the dossier of ONE lens: the shared evidence first, byte for
// byte the same for every lens (so the provider can reuse the prefix), and
// only then what changes per lens.
func pedido(comun, lens string, role agents.Role, provider, bin string) string {
	var b strings.Builder
	b.WriteString(comun)
	fmt.Fprintf(&b, "Revisalo con la lente %s. Solo esa lente.\n", lens)
	cmd := registerCmd(runtime.GOOS, bin)
	b.WriteString("Registra cada hallazgo que sobreviva su propia lectura con hoom finding add")
	if cmd != "hoom" {
		b.WriteString(", usando ESTE hoom (el del PATH puede ser otra version)")
	}
	fmt.Fprintf(&b, ":\n  %s finding add --sev low|medium|high --lens %s --file <ruta> --author %s@%s \"<descripcion con archivo:linea>\"\n",
		cmd, lens, role.Slug, provider)
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

// fueraDeLaTarea are the ids among ids whose finding does not carry task:
// none (a hoom that does not know tasks, first in the PATH of the reviewer's
// login shell) or another one (--task in the reviewer's command).
func fueraDeLaTarea(dir, base string, ids []string, task string) []string {
	items, _, err := finding.List(dir, base, false)
	if err != nil {
		return nil
	}
	tareas := map[string]string{}
	for _, it := range items {
		tareas[it.Finding.ID] = it.Finding.Task
	}
	var out []string
	for _, id := range ids {
		if tareas[id] != task {
			out = append(out, id)
		}
	}
	return out
}

// hoomBin is the hoom the reviewer must call: this very binary unless the
// caller says otherwise, like the cockpit's status pane (CA-88). "" and
// the error when this binary cannot be resolved.
func hoomBin(opt Options) (string, error) {
	if opt.HoomBin != "" {
		return opt.HoomBin, nil
	}
	return executable()
}

// executable is os.Executable, a variable so a test can make it fail.
var executable = os.Executable

// registerCmd is how the pedido names the hoom the reviewer must call: its
// absolute path quoted for a POSIX shell. On Windows the reviewer's shell
// may be PowerShell, cmd or bash and no quoting runs in all three, so there
// (and without a path) it is plain `hoom`, resolved by PATH as before.
func registerCmd(goos, bin string) string {
	if goos == "windows" || bin == "" {
		return "hoom"
	}
	return shellQuote(bin)
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

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
			fmt.Fprintf(w, "    %-6s %s%s\n", ev.Kind, agent, ev.Detail)
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

// printGasto prints what one pass cost, in the numbers its provider reported
// (each with its own semantics, CA-197/198); silence is said out loud.
func printGasto(w io.Writer, u *providers.Usage) {
	if u.Empty() {
		fmt.Fprintln(w, "    gasto     el provider no informo consumo")
		return
	}
	fmt.Fprintf(w, "    gasto     entrada %d - cache %d - salida %d - turnos %d\n",
		u.InputTokens, u.CachedTokens, u.OutputTokens, u.Turns)
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
