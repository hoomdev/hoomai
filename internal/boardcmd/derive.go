package boardcmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// pedidoHint is the placeholder the commands print where the request goes:
// the same one the envelope refuses on purpose, so copying it verbatim can
// never burn a run.
const pedidoHint = `"<pedido>"`

// Derive computes the card from its evidence. Pure: no disk, no clock of its
// own (ev.Now), no git — the same Evidence always gives the same Card.
func Derive(ev Evidence) Card {
	c := Card{
		Slug:     ev.Item.Slug,
		Item:     ev.Item,
		Evidence: meter(ev),
		Unsynced: sortedCopy(ev.Uncommitted),
		Spend:    spend(ev),
		Red:      red(ev),
	}
	c.Running, c.Interrupted = liveness(ev)
	c.Column, c.Missing, c.Next, c.Plain = column(ev)
	if c.Missing == nil {
		c.Missing = []string{}
	}
	for _, col := range Columns {
		if col.ID == c.Column {
			c.ColumnName = col.Name
			c.WaitingHuman = col.Human
		}
	}
	c.NeedsDecision = c.WaitingHuman || c.Interrupted != nil
	c.Meter = meterOf(ev, c.Column, c.Evidence)
	c.Providers = crewOf(ev, c.Evidence.ReviewID)
	return c
}

// column walks the requirements in order and stops at the FIRST one the
// evidence does not meet: that is the station where the work is pending. The
// last value is the same main reason in the normal mode's words.
func column(ev Evidence) (string, []string, string, string) {
	s := ev.Item.Slug
	spec := specOf(ev)
	withTask := hasTask(ev)
	task := ""
	if withTask {
		task = " --task " + s
	}

	if ev.Item.HechoEn != nil {
		return ColHecho, []string{}, "", "terminada: su cierre quedo registrado"
	}
	arquitecto := "hoom agent --role arquitecto" + task + " " + pedidoHint
	if !ev.SpecExists {
		next := "hoom task start " + s
		if withTask {
			next = arquitecto
		}
		return ColBacklog, []string{"no hay spec: " + spec}, next, "falta el spec de la tarjeta"
	}
	if len(ev.LintIssues) > 0 {
		return ColArquitecto, append([]string{}, ev.LintIssues...), arquitecto,
			"el spec no esta completo (" + count(len(ev.LintIssues), "problema", "problemas") + " de formato)"
	}
	if ev.Approval != approval.StatusApproved {
		motivo, plain := "el spec no tiene aprobacion humana", "el spec espera tu aprobacion"
		if ev.Approval == approval.StatusInvalidated {
			motivo = "el spec cambio despues de tu aprobacion"
			plain = "el spec cambio despues de tu aprobacion: hay que aprobarlo de nuevo"
		}
		return ColTuAprobacion, []string{motivo}, "hoom spec approve " + spec, plain
	}
	if len(ev.Untraced) > 0 {
		return ColTestWriter, []string{"criterios sin test: " + strings.Join(ev.Untraced, ", ")},
			"hoom agent --role test-writer" + task + " --spec " + spec + " " + pedidoHint,
			fmt.Sprintf("faltan pruebas para %d de %d criterios", len(ev.Untraced), len(ev.Criteria))
	}
	writer := "hoom agent --role writer" + task + " --spec " + spec + " " + pedidoHint
	v := ev.Verdict
	switch {
	case v == nil:
		return ColWriter, []string{"no hay veredicto de la tarjeta: hoom verify --spec " + spec}, writer,
			"falta verificar el trabajo contra el spec"
	case v.Verdict != "green":
		return ColWriter, []string{redReason(v)}, writer, plainVerdict(v)
	case ev.Fingerprint == "" || v.Git.ChangeFingerprint != ev.Fingerprint:
		return ColWriter, []string{"el codigo cambio despues del ultimo verde"}, writer,
			"el codigo cambio despues del ultimo verde"
	}

	// Green with the current fingerprint: the acceptance conditions, all of
	// them, in the order the spec fixes. Every one that fails is listed.
	var missing, plains []string
	next := ""
	fail := func(motivo, plain, cmd string) {
		missing = append(missing, motivo)
		plains = append(plains, plain)
		if next == "" {
			next = cmd
		}
	}
	if lines, required := reviewRequired(v); required && reviewOf(ev) == "" {
		fail(fmt.Sprintf("la review exige las 4 lentes (%d lineas > %d) y no hay registro de review", lines, reviewcmd.UmbralLineas),
			fmt.Sprintf("falta la revision de 4 lentes (%d lineas)", lines),
			"hoom review"+task+" --spec "+spec)
	}
	if ids := blocking(ev); len(ids) > 0 {
		plain := fmt.Sprintf("hay %d hallazgos que bloquean", len(ids))
		if len(ids) == 1 {
			plain = "hay 1 hallazgo que bloquea"
		}
		fail("hallazgos abiertos que bloquean: "+strings.Join(ids, ", "), plain,
			"hoom finding resolve "+ids[0]+` --as corregido|refutado --evidence "..."`)
	}
	if gateStatus(v, "spec_approved") != verdict.StatusPass {
		fail("el veredicto no trae spec_approved en pass",
			"hay que verificar de nuevo: el ultimo verde no incluye la aprobacion del spec",
			"hoom verify --spec "+spec)
	}
	switch gateStatus(v, finding.GateName) {
	case verdict.StatusPass:
	case "":
		fail(`falta el gate findings_open: agrega "findings: { block_on: high }" a hoom.yaml y vuelve a verificar`,
			"el proyecto no declara que hallazgos bloquean",
			`agrega "findings: { block_on: high }" a hoom.yaml y corre hoom verify --spec `+spec)
	default:
		fail("el veredicto no trae findings_open en pass",
			"hay que verificar de nuevo: el control de hallazgos no paso",
			"hoom verify --spec "+spec)
	}
	switch {
	case !withTask:
		fail("cerrar exige la tarea: hoom task start "+s, "falta el espacio de trabajo de la tarjeta", "hoom task start "+s)
	case ev.ReadyErr != "":
		fail(ev.ReadyErr, plainReady(ev.ReadyKind), readyAction(ev.ReadyErr, s))
	}
	if len(missing) > 0 {
		plain := plains[0]
		if rest := len(plains) - 1; rest > 0 {
			plain += " (y " + count(rest, "pendiente", "pendientes") + " mas)"
		}
		return ColReview, missing, next, plain
	}
	return ColTuAceptacion, []string{"falta tu aceptacion: hoom task done " + s}, "hoom task done " + s,
		"espera tu aceptacion para integrar"
}

func specOf(ev Evidence) string {
	if ev.SpecPath != "" {
		return ev.SpecPath
	}
	return ".hoom/specs/" + ev.Item.Slug + ".md"
}

func hasTask(ev Evidence) bool { return ev.Worktree || ev.Source == SourceWorktree }

// reviewRequired is contract 06's size rule read from the verdict itself:
// more than UmbralLineas changed lines is exactly when the verdict already
// prints "la review exige las 4 lentes".
func reviewRequired(v *verdict.Verdict) (int, bool) {
	if v == nil {
		return 0, false
	}
	n := v.Git.Insertions + v.Git.Deletions
	return n, n > reviewcmd.UmbralLineas
}

// reviewOf returns the newest review record that satisfies the card: its
// task, the four lenses, and a verdict_id that is a complete green verdict of
// the card (a review of a tree that never went green reviewed nothing).
func reviewOf(ev Evidence) string {
	green := map[string]bool{}
	for _, id := range ev.GreenVerdicts {
		green[id] = true
	}
	best := ""
	var bestAt int64
	for _, r := range ev.Reviews {
		if r.Task != ev.Item.Slug || !green[r.VerdictID] || !containsAll(r.Lenses, reviewcmd.Lentes) {
			continue
		}
		if at := r.CreatedAt.UnixNano(); best == "" || at >= bestAt {
			best, bestAt = r.ID, at
		}
	}
	return best
}

// blocking lists the open findings of the card that block under the
// threshold, with the ONE definition of blocking (finding.Blocks).
func blocking(ev Evidence) []string {
	out := []string{}
	threshold := ev.BlockOn
	if threshold == "" {
		threshold = "high"
	}
	for _, f := range ev.Findings {
		if f.Status != finding.StatusOpen || f.Task != ev.Item.Slug {
			continue
		}
		if finding.Blocks(f.Severity, threshold) {
			out = append(out, f.ID)
		}
	}
	return out
}

func gateStatus(v *verdict.Verdict, name string) string {
	for _, g := range v.Gates {
		if g.Name == name {
			return g.Status
		}
	}
	return ""
}

// failedGates are the required gates of v that failed, in its order.
func failedGates(v *verdict.Verdict) []string {
	var failed []string
	for _, g := range v.Gates {
		if g.Required && (g.Status == verdict.StatusFail || g.Status == verdict.StatusError) {
			failed = append(failed, g.Name)
		}
	}
	return failed
}

// redReason names the required gates that failed, in the verdict's order.
func redReason(v *verdict.Verdict) string {
	failed := failedGates(v)
	switch len(failed) {
	case 0:
		return "veredicto rojo"
	case 1:
		return "veredicto rojo: fallo el gate " + failed[0]
	default:
		return "veredicto rojo: fallaron los gates " + strings.Join(failed, ", ")
	}
}

// readyAction takes the action a taskcmd.Ready message already carries.
func readyAction(msg, slug string) string {
	if i := strings.Index(msg, "Accion: "); i >= 0 {
		return strings.TrimSpace(msg[i+len("Accion: "):])
	}
	return "hoom task done " + slug
}

// meter is the evidence the card shows, segment by segment.
func meter(ev Evidence) CardEvidence {
	m := CardEvidence{
		Source: ev.Source, Dir: ev.Dir, Spec: specOf(ev), SpecExists: ev.SpecExists,
		LintIssues: nonNil(ev.LintIssues), Criteria: len(ev.Criteria),
		Untraced: nonNil(ev.Untraced), Approval: ev.Approval,
		BlockingFindings: blocking(ev),
	}
	if ev.SpecExists && len(ev.LintIssues) == 0 {
		m.Traced = len(ev.Criteria) - len(ev.Untraced)
	}
	if v := ev.Verdict; v != nil {
		m.VerdictID, m.Verdict = v.ID, v.Verdict
		m.FingerprintMatch = ev.Fingerprint != "" && v.Git.ChangeFingerprint == ev.Fingerprint
		_, m.ReviewRequired = reviewRequired(v)
	}
	m.ReviewID = reviewOf(ev)
	return m
}

// liveness answers "who is working on this card right now" from the
// envelopes and runs whose owner Gather found alive — and labels the open
// envelopes whose owner is gone, without closing them.
func liveness(ev Evidence) (*Running, *Interrupted) {
	var run *Running
	var intr *Interrupted
	for _, e := range ev.Envelopes {
		rec := e.Record
		if rec.Done() {
			continue
		}
		if e.Alive {
			cand := &Running{EnvelopeID: rec.ID, RunID: rec.RunID, Role: rec.Role, Provider: rec.Provider,
				Stage: rec.Stage, Step: rec.Step, Steps: rec.Steps, StartedAt: rec.StartedAt}
			if run == nil || cand.StartedAt.After(run.StartedAt) {
				run = cand
			}
			continue
		}
		if intr == nil || rec.UpdatedAt.After(intr.UpdatedAt) {
			intr = &Interrupted{EnvelopeID: rec.ID, RunID: rec.RunID, Role: rec.Role, Provider: rec.Provider,
				Stage: rec.Stage, Step: rec.Step, Steps: rec.Steps, UpdatedAt: rec.UpdatedAt}
		}
	}
	own := map[string]bool{} // runs an alive envelope already describes, with its step
	for _, e := range ev.Envelopes {
		if e.Alive && !e.Record.Done() && e.Record.RunID != "" {
			own[e.Record.RunID] = true
		}
	}
	for _, r := range ev.Runs {
		m := r.Meta
		if m.Status != runcmd.StatusRunning || !r.Alive || own[m.ID] {
			continue
		}
		cand := &Running{RunID: m.ID, Role: m.Role, Provider: m.Provider, Stage: "run", StartedAt: m.CreatedAt}
		if run == nil || cand.StartedAt.After(run.StartedAt) {
			run = cand
		}
	}
	return run, intr
}

// red compares the card's verdict with its last CLOSED envelope and reports
// the newer one if it failed. A later green turns the red off.
func red(ev Evidence) *Red {
	var last *envelope.Record
	for i := range ev.Envelopes {
		rec := ev.Envelopes[i].Record
		if !rec.Done() {
			continue
		}
		if last == nil || endOf(rec).After(endOf(*last)) {
			r := rec
			last = &r
		}
	}
	v := ev.Verdict
	if v != nil && (last == nil || v.CreatedAt.After(endOf(*last))) {
		if v.Verdict != "green" {
			return &Red{Source: RedVerdict, ID: v.ID, Reason: redReason(v), Plain: plainVerdict(v)}
		}
		return nil
	}
	if last == nil {
		return nil
	}
	if last.Status != envelope.StatusNotDeliverable && last.Status != envelope.StatusNoDelivery {
		return nil
	}
	reason := fmt.Sprintf("el sobre de %s corto en %s", last.Role, last.Stage)
	if n := strings.TrimSpace(last.Note); n != "" {
		reason += ": " + n
	}
	return &Red{Source: RedEnvelope, ID: last.ID, Reason: reason, Plain: plainEnvelope(*last)}
}

func endOf(r envelope.Record) time.Time {
	if !r.EndedAt.IsZero() {
		return r.EndedAt
	}
	return r.UpdatedAt
}

// spend adds up the card's runs, plus every envelope whose run has no sidecar
// (so nothing is counted twice). A cost nobody reported stays nil.
func spend(ev Evidence) Spend {
	s := Spend{BudgetUSD: ev.Item.PresupuestoUSD}
	var total float64
	reported := false
	add := func(u *providers.Usage) {
		s.Runs++
		if u == nil || u.CostUSD == nil {
			s.RunsWithoutCost++
		} else {
			total += *u.CostUSD
			reported = true
		}
		if u != nil {
			s.InputTokens += u.InputTokens
			s.OutputTokens += u.OutputTokens
		}
	}
	seen := map[string]bool{}
	for _, r := range ev.Runs {
		seen[r.Meta.ID] = true
		add(r.Meta.Usage)
	}
	for _, e := range ev.Envelopes {
		if id := e.Record.RunID; id != "" && !seen[id] {
			seen[id] = true
			add(e.Record.Usage)
		}
	}
	if reported {
		s.CostUSD = &total
	}
	return s
}

func containsAll(have, want []string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return append([]string{}, in...)
}

func sortedCopy(in []string) []string {
	out := nonNil(in)
	sort.Strings(out)
	return out
}
