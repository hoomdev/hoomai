package boardcmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Candidata is one task with evidence on the board's tree, as `hoom item
// backfill` sees it: the item it would create, or why not.
type Candidata struct {
	Slug   string `json:"slug"`
	Titulo string `json:"titulo"`
	Tipo   string `json:"tipo"`
	// Historico is what the item would carry; "" when it does not qualify.
	Historico string `json:"historico,omitempty"`
	// Motivo is why it does not qualify; "" when it does.
	Motivo string `json:"motivo,omitempty"`
	// ConItem: the task already has an item, which backfill never touches.
	ConItem bool `json:"con_item,omitempty"`
}

// motivoSinVeredicto is why a task with spec does not qualify.
const motivoSinVeredicto = "sin veredicto verde aprobado en la base"

// Candidatas lists every task with verdicts or findings on root's trees
// (the ones the doctor's evidencia-sin-item looks at, plus those that
// already have an item), in slug order, each with what backfill would do.
// Read-only.
func Candidatas(root, base, blockOn string, now time.Time) ([]Candidata, error) {
	g := newGatherer(root, base, blockOn, now)
	items, err := itemFiles(root)
	if err != nil {
		return nil, err
	}
	worktrees, err := taskWorktrees(root)
	if err != nil {
		return nil, err
	}
	verdicts, findings := tareasConEvidencia(g, worktrees)
	tareas := map[string]bool{}
	for t := range verdicts {
		tareas[t] = true
	}
	for t := range findings {
		tareas[t] = true
	}
	slugs := make([]string, 0, len(tareas))
	for t := range tareas {
		slugs = append(slugs, t)
	}
	sort.Strings(slugs)

	out := make([]Candidata, 0, len(slugs))
	for _, s := range slugs {
		c := Candidata{Slug: s, Titulo: tituloDeSpec(root, s), Tipo: "feature"}
		if items[s] {
			c.ConItem = true
			out = append(out, c)
			continue
		}
		specPath := filepath.Join(root, ".hoom", "specs", s+".md")
		_, errSpec := os.Stat(specPath)
		if len(verdicts[s]) > 0 || errSpec == nil {
			if v := g.veredictoQueSostiene(s, worktrees); v != nil {
				c.Historico = item.HistoricoVeredicto(v.ID)
			} else {
				c.Motivo = motivoSinVeredicto
			}
			out = append(out, c)
			continue
		}
		c.Tipo = "bug"
		if abierto := g.bloqueanteDe(s, worktrees); abierto != nil {
			c.Motivo = fmt.Sprintf("el hallazgo %s (%s) sigue abierto", abierto.ID, abierto.Severity)
		} else {
			c.Historico = item.HistoricoHallazgos
		}
		out = append(out, c)
	}
	return out, nil
}

// tareasConEvidencia is every task with a verdict bound to its spec or a
// finding of its own, on root and on the task worktrees, with the ids seen:
// what evidencia-sin-item and backfill both look at. A verdict or finding
// seen in several trees counts once.
func tareasConEvidencia(g *gatherer, worktrees []string) (verdicts, findings map[string]map[string]bool) {
	verdicts, findings = map[string]map[string]bool{}, map[string]map[string]bool{}
	note := func(m map[string]map[string]bool, task, id string) {
		if m[task] == nil {
			m[task] = map[string]bool{}
		}
		m[task][id] = true
	}
	for _, p := range places(g.root, worktrees) {
		t := g.tree(p.dir)
		for _, v := range t.allVerdicts() {
			if task := specTask(v, p.dir); task != "" {
				note(verdicts, task, v.ID)
			}
		}
		for _, f := range t.allFindings(g.base) {
			// a task that is not a slug is not a card's, and never goes
			// into the command the doctor proposes (finding 5435b7)
			if item.ValidSlug(f.Task) {
				note(findings, f.Task, f.ID)
			}
		}
	}
	return verdicts, findings
}

// veredictoQueSostiene is the newest verdict of task s, on root or its
// worktree, that can hold its card: complete, green, bound to its spec,
// with spec_approved in pass, and with its commit in the base. nil = none.
func (g *gatherer) veredictoQueSostiene(s string, worktrees []string) *verdict.Verdict {
	var mejor *verdict.Verdict
	for _, p := range places(g.root, worktrees) {
		for _, v := range g.tree(p.dir).allVerdicts() {
			if specTask(v, p.dir) != s || g.porQueNoSostiene(v) != "" {
				continue
			}
			if mejor == nil || v.CreatedAt.After(mejor.CreatedAt) {
				mejor = v
			}
		}
	}
	return mejor
}

// porQueNoSostiene says why verdict v cannot hold a historical card, or ""
// when it can.
func (g *gatherer) porQueNoSostiene(v *verdict.Verdict) string {
	switch {
	case v.IsPartial():
		return fmt.Sprintf("el veredicto %s es parcial", v.ID)
	case v.Verdict != "green":
		return fmt.Sprintf("el veredicto %s no es verde", v.ID)
	case !specAprobado(v):
		return fmt.Sprintf("el veredicto %s no tiene spec_approved en pass", v.ID)
	}
	// the commit comes from a file: only hex reaches git, never an option
	if v.Git.Commit == "" || strings.Trim(strings.ToLower(v.Git.Commit), "0123456789abcdef") != "" {
		return fmt.Sprintf("el veredicto %s no dice un commit valido", v.ID)
	}
	en, err := gitx.EsAncestro(g.root, v.Git.Commit, g.base)
	switch {
	case err != nil:
		return fmt.Sprintf("git no pudo decir si el commit %s del veredicto %s esta en la base: %v", v.Git.Commit, v.ID, err)
	case !en:
		return fmt.Sprintf("el commit %s del veredicto %s no esta integrado en %s", v.Git.Commit, v.ID, g.base)
	}
	return ""
}

// specAprobado says whether v's spec_approved gate passed.
func specAprobado(v *verdict.Verdict) bool {
	for _, gr := range v.Gates {
		if gr.Name == "spec_approved" {
			return gr.Status == verdict.StatusPass
		}
	}
	return false
}

// bloqueanteDe is the first open finding of task s, on root or its
// worktrees, that blocks at the board's threshold; nil = none.
func (g *gatherer) bloqueanteDe(s string, worktrees []string) *finding.Item {
	umbral := g.blockOn
	if umbral == "" {
		umbral = "high"
	}
	for _, p := range places(g.root, worktrees) {
		for _, f := range g.tree(p.dir).allFindings(g.base) {
			if f.Task == s && f.Status == finding.StatusOpen && finding.Blocks(f.Severity, umbral) {
				f := f
				return &f
			}
		}
	}
	return nil
}

// sostenHistorica fills in whether the item's historico still holds its card
// on the evidence tree t, and why not when it does not. An item without
// historico is left alone.
func (g *gatherer) sostenHistorica(ev *Evidence, t *tree) {
	h := ev.Item.Historico
	if h == "" {
		return
	}
	id, hallazgos, ok := item.LeerHistorico(h)
	switch {
	case !ok:
		ev.HistoricaMotivo = fmt.Sprintf("historico %q no es 'veredicto <id>' ni '%s'", h, item.HistoricoHallazgos)
	case hallazgos:
		for _, f := range ev.Findings {
			if f.Status == finding.StatusOpen && finding.Blocks(f.Severity, ev.BlockOn) {
				ev.HistoricaMotivo = fmt.Sprintf("el hallazgo %s (%s) de la tarea esta abierto", f.ID, f.Severity)
				return
			}
		}
		ev.HistoricaSostenida = true
	default:
		var v *verdict.Verdict
		for _, cand := range t.allVerdicts() {
			if cand.ID == id {
				v = cand
			}
		}
		if v == nil && t.dir != g.root {
			for _, cand := range g.tree(g.root).allVerdicts() {
				if cand.ID == id {
					v = cand
				}
			}
		}
		if v == nil {
			ev.HistoricaMotivo = fmt.Sprintf("el veredicto %s que la sostiene ya no esta", id)
			return
		}
		if motivo := g.porQueNoSostiene(v); motivo != "" {
			ev.HistoricaMotivo = motivo
			return
		}
		ev.HistoricaSostenida = true
	}
}

// tituloDeSpec is the title of task s's card: the first heading of its spec
// without "Spec: ", or the slug when there is no spec or no heading.
func tituloDeSpec(root, s string) string {
	f, err := os.Open(filepath.Join(root, ".hoom", "specs", s+".md"))
	if err != nil {
		return s
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if titulo, ok := strings.CutPrefix(l, "# "); ok {
			titulo = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(titulo), "Spec:"))
			if titulo != "" {
				return titulo
			}
		}
	}
	return s
}
