package boardcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
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
// Read-only. A task is judged on its own trees (root and its worktree), the
// same ones the board looks at to hold the card afterwards. An error means
// it could not decide (a spec or git that could not answer): backfill then
// writes nothing.
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
	propio := map[string]string{} // the worktree of each task that has one
	for _, p := range places(root, worktrees)[1:] {
		propio[strings.TrimPrefix(p.rel, ".hoom/worktrees/")] = p.dir
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
		if items[s] {
			out = append(out, Candidata{Slug: s, Tipo: "feature", ConItem: true})
			continue
		}
		dirs := []string{root}
		if wt, ok := propio[s]; ok {
			dirs = append(dirs, wt)
		}
		spec, err := specDe(s, dirs)
		if err != nil {
			return nil, err
		}
		titulo, err := tituloDeSpec(spec, s)
		if err != nil {
			return nil, err
		}
		c := Candidata{Slug: s, Titulo: titulo, Tipo: "feature"}
		// what takes the item's place without being one (a directory: the
		// doctor does not count it as an item) is not "already has an item":
		// backfill could not write there either (finding 752455)
		if fi, err := os.Lstat(item.Path(root, s)); err == nil {
			c.Motivo = fmt.Sprintf("en %s hay algo que no es un item (%s)", item.RelPath(s), fi.Mode().Type())
			out = append(out, c)
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no se pudo mirar %s: %w", item.RelPath(s), err)
		}
		// a title nobody typed (it comes from the spec, through git) never
		// carries what a terminal would obey (finding 1499b7)
		if conControl(titulo) {
			c.Motivo = fmt.Sprintf("el titulo del spec trae caracteres de control: %q", titulo)
			out = append(out, c)
			continue
		}
		// a task with verdicts of its spec, or a spec on its trees, is a task
		// with spec: only a verdict qualifies it, never its findings (a
		// spec that only lives in the worktree too, finding 939eec)
		if len(verdicts[s]) > 0 || spec != "" {
			v, err := g.veredictoQueSostiene(s, dirs)
			if err != nil {
				return nil, err
			}
			if v != nil {
				c.Historico = item.HistoricoVeredicto(v.ID)
			} else {
				c.Motivo = motivoSinVeredicto
			}
			out = append(out, c)
			continue
		}
		c.Tipo = "bug"
		if motivo := g.porQueNoSostienenHallazgos(s, dirs); motivo != "" {
			c.Motivo = motivo
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

// specDe is the spec of task s on its trees dirs (the first one that has
// it), or "" when none does. An entry that is there counts whatever it is:
// a directory in its place still makes it a task with spec, the stricter
// side. Anything but "not there" is an error.
func specDe(s string, dirs []string) (string, error) {
	for _, d := range dirs {
		p := filepath.Join(d, ".hoom", "specs", s+".md")
		_, err := os.Lstat(p)
		switch {
		case err == nil:
			return p, nil
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("no se pudo mirar el spec de %s: %w", s, err)
		}
	}
	return "", nil
}

// veredictoQueSostiene is the newest verdict of task s on its trees dirs
// that can hold its card (porQueNoSostiene): the same trees sostenHistorica
// looks at afterwards, so what backfill picks the board finds (finding
// 552a4e). nil = none. An error is git that could not say whether a commit
// is in the base: it is not "not integrated" (finding 3098f2).
func (g *gatherer) veredictoQueSostiene(s string, dirs []string) (*verdict.Verdict, error) {
	var mejor *verdict.Verdict
	for _, d := range dirs {
		for _, v := range g.tree(d).allVerdicts() {
			if specTask(v, d) != s {
				continue
			}
			motivo, err := g.porQueNoSostiene(v)
			if err != nil {
				return nil, err
			}
			if motivo != "" {
				continue
			}
			if mejor == nil || v.CreatedAt.After(mejor.CreatedAt) {
				mejor = v
			}
		}
	}
	return mejor, nil
}

// porQueNoSostiene says why verdict v cannot hold a historical card, or ""
// when it can. The error is git failing to answer, not a reason.
func (g *gatherer) porQueNoSostiene(v *verdict.Verdict) (string, error) {
	switch {
	case v.IsPartial():
		return fmt.Sprintf("el veredicto %s es parcial", v.ID), nil
	case v.Verdict != "green":
		return fmt.Sprintf("el veredicto %s no es verde", v.ID), nil
	case !specAprobado(v):
		return fmt.Sprintf("el veredicto %s no tiene spec_approved en pass", v.ID), nil
	}
	// the commit comes from a file: only hex reaches git, never an option
	if v.Git.Commit == "" || strings.Trim(strings.ToLower(v.Git.Commit), "0123456789abcdef") != "" {
		return fmt.Sprintf("el veredicto %s no dice un commit valido", v.ID), nil
	}
	// a commit this repository does not have is not in its base: that is an
	// answer, not git failing (a branch that was never pushed, a rebase)
	_, existe, err := gitx.ResolverCommit(g.root, v.Git.Commit)
	switch {
	case err != nil:
		return "", fmt.Errorf("git no pudo resolver el commit %s del veredicto %s: %w", v.Git.Commit, v.ID, err)
	case !existe:
		return fmt.Sprintf("el commit %s del veredicto %s no esta en este repositorio", v.Git.Commit, v.ID), nil
	}
	en, err := gitx.EsAncestro(g.root, v.Git.Commit, g.base)
	switch {
	case err != nil:
		return "", fmt.Errorf("git no pudo decir si el commit %s del veredicto %s esta en %s: %w", v.Git.Commit, v.ID, g.base, err)
	case !en:
		return fmt.Sprintf("el commit %s del veredicto %s no esta integrado en %s", v.Git.Commit, v.ID, g.base), nil
	}
	return "", nil
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

// porQueNoSostienenHallazgos says why the findings of task s on its trees
// dirs do not hold a card without spec, or "" when they do: at least one
// finding of the task is there (findings a28967, 6fdcc5), none of them
// open and blocking at the board's threshold on any of those trees
// (finding 6d0cce), and no finding there is unreadable, whose state nobody
// knows (finding 1d6b30). It is the rule backfill qualifies by and the one
// the board holds the card by.
func (g *gatherer) porQueNoSostienenHallazgos(s string, dirs []string) string {
	umbral := g.blockOn
	if umbral == "" {
		umbral = "high"
	}
	testigo := false
	for _, d := range dirs {
		t := g.tree(d)
		items := t.allFindings(g.base)
		if len(t.unreadable) > 0 {
			return fmt.Sprintf("hay hallazgos ilegibles en %s: %s", relDe(g.root, d), strings.Join(t.unreadable, "; "))
		}
		for _, f := range items {
			if f.Task != s {
				continue
			}
			testigo = true
			if f.Status == finding.StatusOpen && bloquea(f.Severity, umbral) {
				return fmt.Sprintf("el hallazgo %s (%s) sigue abierto", f.ID, f.Severity)
			}
		}
	}
	if !testigo {
		return "la tarea ya no tiene ningun hallazgo"
	}
	return ""
}

// bloquea is finding.Blocks failing closed: a severity outside the
// vocabulary blocks, as the findings_open gate refuses it (finding eda92d).
// Blocks(sev, "low") is true exactly for the severities it knows.
func bloquea(sev, umbral string) bool {
	return !finding.Blocks(sev, "low") || finding.Blocks(sev, umbral)
}

// relDe is dir relative to root, for a message; dir itself when it is not
// under root.
func relDe(root, dir string) string {
	if rel, err := filepath.Rel(root, dir); err == nil {
		return filepath.ToSlash(rel)
	}
	return dir
}

// sostenHistorica fills in whether the item's historico still holds its card
// and why not when it does not, on the card's trees: root and dir, the one
// its evidence is read from. It is the same rule backfill qualified it by.
// An item without historico is left alone.
func (g *gatherer) sostenHistorica(ev *Evidence, dir string) {
	h := ev.Item.Historico
	if h == "" {
		return
	}
	dirs := []string{g.root}
	if dir != g.root {
		dirs = append(dirs, dir)
	}
	s := ev.Item.Slug
	id, hallazgos, ok := item.LeerHistorico(h)
	switch {
	case !ok:
		ev.HistoricaMotivo = fmt.Sprintf("historico %q no es 'veredicto <id>' ni '%s'", h, item.HistoricoHallazgos)
	case hallazgos:
		if motivo := g.porQueNoSostienenHallazgos(s, dirs); motivo != "" {
			ev.HistoricaMotivo = motivo
			return
		}
		ev.HistoricaSostenida = true
	default:
		var v *verdict.Verdict
		ajeno := ""
		for _, d := range dirs {
			for _, cand := range g.tree(d).allVerdicts() {
				if cand.ID != id {
					continue
				}
				// the verdict has to be this card's: one of another task's
				// spec never holds it (finding 639517)
				if task := specTask(cand, d); task != s {
					ajeno = task
					continue
				}
				v = cand
			}
		}
		switch {
		case v == nil && ajeno != "":
			ev.HistoricaMotivo = fmt.Sprintf("el veredicto %s no es de esta tarea (es del spec de %q)", id, ajeno)
			return
		case v == nil:
			ev.HistoricaMotivo = fmt.Sprintf("el veredicto %s que la sostiene ya no esta", id)
			return
		}
		motivo, err := g.porQueNoSostiene(v)
		if err != nil {
			motivo = err.Error()
		}
		if motivo != "" {
			ev.HistoricaMotivo = motivo
			return
		}
		ev.HistoricaSostenida = true
	}
}

// conControl says whether s has a control character or one that changes
// the direction of the text (U+202E and the like).
func conControl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)
	}) >= 0
}

// tituloDeSpec is the title of task s's card: the first heading of its spec
// at path without "Spec: ", or the slug when there is no spec (path "") or
// no heading. A spec that is there and cannot be read is an error, never a
// title degraded to the slug that no later backfill would fix (finding
// b16c61); it is read the way hoom reads what it does not own, never
// following a symlink nor blocking on a FIFO.
func tituloDeSpec(path, s string) (string, error) {
	if path == "" {
		return s, nil
	}
	f, err := hoomfs.AbrirRegular(path)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer el spec de %s: %w", s, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if titulo, ok := strings.CutPrefix(l, "# "); ok {
			titulo = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(titulo), "Spec:"))
			if titulo != "" {
				return titulo, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("no se pudo leer el spec de %s: %w", s, err)
	}
	return s, nil
}
