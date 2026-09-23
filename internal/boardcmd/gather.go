package boardcmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Gather reads the evidence of one item. Read-only: it creates and modifies
// nothing, and the only commands it runs are git reads.
func Gather(root, base, blockOn string, it item.Item, now time.Time) Evidence {
	return newGatherer(root, base, blockOn, now).gather(it)
}

// Build derives the card of every valid item of root: the eight columns
// always, in order, and the warnings of the items that could not be read.
func Build(root, base, blockOn string, now time.Time) (Board, error) {
	items, warnings, err := item.List(root)
	if err != nil {
		return Board{}, err
	}
	b := Board{Columns: make([]BoardColumn, 0, len(Columns)), Warnings: append([]string{}, warnings...)}
	for _, col := range Columns {
		b.Columns = append(b.Columns, BoardColumn{ID: col.ID, Name: col.Name, Human: col.Human, Cards: []Card{}})
	}
	g := newGatherer(root, base, blockOn, now)
	for _, it := range items {
		c := Derive(g.gather(it))
		for i := range b.Columns {
			if b.Columns[i].ID == c.Column {
				b.Columns[i].Cards = append(b.Columns[i].Cards, c)
			}
		}
	}
	return b, nil
}

// CardFor derives the card of one item.
func CardFor(root, base, blockOn, slug string, now time.Time) (Card, error) {
	it, err := item.Load(root, slug)
	if os.IsNotExist(err) {
		return Card{}, fmt.Errorf("no existe el item %q (%s). Accion: crealo con 'hoom item add \"<titulo>\" --slug %s'", slug, item.RelPath(slug), slug)
	}
	if err != nil {
		return Card{}, fmt.Errorf("el item %s es invalido: %v", item.RelPath(slug), err)
	}
	return Derive(Gather(root, base, blockOn, it, now)), nil
}

// gatherer reads each evidence tree ONCE: every card of the current tree
// shares its verdicts, findings, reviews, status and fingerprint.
type gatherer struct {
	root, base, blockOn string
	now                 time.Time
	trees               map[string]*tree
	envelopes           map[string][]envelope.Record
	metas               map[string][]runcmd.Meta
}

// tree is what one evidence directory holds, read lazily.
type tree struct {
	dir         string
	verdicts    []*verdict.Verdict
	findings    []finding.Item
	reviews     []reviewcmd.Record
	status      []string // uncommitted paths, relative to dir
	fingerprint string
	read        map[string]bool
}

func newGatherer(root, base, blockOn string, now time.Time) *gatherer {
	return &gatherer{root: root, base: base, blockOn: blockOn, now: now,
		trees: map[string]*tree{}, envelopes: map[string][]envelope.Record{}, metas: map[string][]runcmd.Meta{}}
}

func (g *gatherer) tree(dir string) *tree {
	t, ok := g.trees[dir]
	if !ok {
		t = &tree{dir: dir, read: map[string]bool{}}
		g.trees[dir] = t
	}
	return t
}

func (t *tree) once(what string, f func()) {
	if !t.read[what] {
		t.read[what] = true
		f()
	}
}

func (t *tree) allVerdicts() []*verdict.Verdict {
	t.once("verdicts", func() { t.verdicts, _, _ = verdict.LoadAllWithWarnings(t.dir) })
	return t.verdicts
}

func (t *tree) allFindings(base string) []finding.Item {
	t.once("findings", func() { t.findings, _, _ = finding.List(t.dir, base, false) })
	return t.findings
}

func (t *tree) allReviews() []reviewcmd.Record {
	t.once("reviews", func() { t.reviews, _ = reviewcmd.Records(t.dir) })
	return t.reviews
}

func (t *tree) uncommitted() []string {
	t.once("status", func() { t.status = porcelain(t.dir) })
	return t.status
}

func (t *tree) currentFingerprint(base string) string {
	t.once("fingerprint", func() { t.fingerprint = gitx.Snapshot(t.dir, base).ChangeFingerprint })
	return t.fingerprint
}

func (g *gatherer) gather(it item.Item) Evidence {
	s := it.Slug
	ev := Evidence{Item: it, Now: g.now, Source: SourceArbol, Dir: ".",
		SpecPath: ".hoom/specs/" + s + ".md", BlockOn: g.blockOn}
	if ev.BlockOn == "" {
		ev.BlockOn = "high"
	}
	dir := g.root
	if wt, err := runcmd.TaskDir(g.root, s); err == nil && s != "" {
		dir, ev.Source, ev.Worktree = wt, SourceWorktree, true
		ev.Dir = ".hoom/worktrees/" + s
	}
	t := g.tree(dir)
	specAbs := filepath.Join(dir, filepath.FromSlash(ev.SpecPath))

	// spec, lint, trace by token (verifica commands never run here) and the
	// approval of its current content
	if st, err := os.Stat(specAbs); err == nil && !st.IsDir() {
		ev.SpecExists = true
		ids, cmds, issues, err := spec.Lint(specAbs)
		switch {
		case err != nil:
			ev.LintIssues = []string{"no se pudo leer el spec: " + err.Error()}
		case len(issues) > 0:
			ev.LintIssues = issues
			ev.Criteria = ids
		default:
			ev.Criteria = ids
			var needTest []string
			for _, id := range ids {
				if len(cmds[id]) == 0 {
					needTest = append(needTest, id)
				}
			}
			missing, _, terr := spec.Tokens(dir, needTest)
			if terr != nil {
				missing = needTest // sin poder leer los tests, nada esta trazado
			}
			ev.Untraced = missing
		}
		if state, _, err := approval.Status(dir, specAbs); err == nil {
			ev.Approval = state
		} else {
			ev.Approval = approval.StatusNotApproved
		}
	}

	// the card's verdicts: complete, bound to its spec
	var cardVerdicts []*verdict.Verdict
	for _, v := range t.allVerdicts() {
		if specMatches(v.Spec, dir, ev.SpecPath) {
			cardVerdicts = append(cardVerdicts, v)
		}
	}
	for _, v := range cardVerdicts {
		if v.IsPartial() {
			continue
		}
		ev.Verdict = v // oldest first: the last one is the newest
		if v.Verdict == "green" {
			ev.GreenVerdicts = append(ev.GreenVerdicts, v.ID)
		}
	}
	if ev.Verdict != nil {
		ev.Fingerprint = t.currentFingerprint(g.base)
	}

	for _, r := range t.allReviews() {
		if r.Task == s {
			ev.Reviews = append(ev.Reviews, r)
		}
	}
	var cardFindings []finding.Item
	for _, f := range t.allFindings(g.base) {
		if f.Task != s {
			continue
		}
		cardFindings = append(cardFindings, f)
		if f.Status == finding.StatusOpen {
			ev.Findings = append(ev.Findings, f)
		}
	}

	if ev.Worktree {
		if err := taskcmd.Ready(g.root, s, g.base); err != nil {
			ev.ReadyErr = err.Error()
		}
	}
	ev.Uncommitted = g.uncommitted(ev, t, dir, cardVerdicts, cardFindings)
	ev.Envelopes, ev.Runs = g.telemetry(s, dir)
	return ev
}

// uncommitted lists the card's evidence that is not in Git yet, relative to
// root. In the task's worktree everything counts (it is the card's tree, and
// it is what `task done` demands); in the shared tree only the card's own
// paths do.
func (g *gatherer) uncommitted(ev Evidence, t *tree, dir string, verdicts []*verdict.Verdict, findings []finding.Item) []string {
	out := []string{}
	s := ev.Item.Slug
	rootItem := item.RelPath(s)
	if ev.Worktree {
		for _, p := range t.uncommitted() {
			out = append(out, ev.Dir+"/"+p)
		}
		for _, p := range g.tree(g.root).uncommitted() {
			if p == rootItem {
				out = append(out, p)
			}
		}
		sort.Strings(out)
		return out
	}
	own := map[string]bool{rootItem: true, ev.SpecPath: true}
	for _, v := range verdicts {
		own[".hoom/verdicts/"+v.ID+".json"] = true
	}
	for _, f := range findings {
		own[".hoom/findings/"+f.ID+".json"] = true
		own[".hoom/findings/"+f.ID+".res.json"] = true
	}
	for _, r := range ev.Reviews {
		own[".hoom/"+reviewcmd.RecordsDir+"/"+r.ID+".json"] = true
	}
	for _, p := range t.uncommitted() {
		if own[p] || strings.HasPrefix(p, ".hoom/approvals/"+s+"_") {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// telemetry reads the card's envelopes and runs (local, never in Git) from
// the project root and, when the evidence lives in a worktree, from there
// too — and resolves whether each owner is alive, which Derive cannot do.
func (g *gatherer) telemetry(slug, dir string) ([]EnvelopeState, []RunState) {
	places := []string{g.root}
	if dir != g.root {
		places = append(places, dir)
	}
	seenEnv, seenRun := map[string]bool{}, map[string]bool{}
	var envs []EnvelopeState
	var runs []RunState
	metaOf := map[string]runcmd.Meta{}
	for _, place := range places {
		for _, m := range g.metasOf(place) {
			if !seenRun[m.ID] {
				metaOf[m.ID] = m
			}
			if m.Task != slug || seenRun[m.ID] {
				continue
			}
			seenRun[m.ID] = true
			runs = append(runs, RunState{Meta: m,
				Alive: m.Status == runcmd.StatusRunning && m.PID > 0 && runcmd.Alive(m.PID)})
		}
	}
	for _, place := range places {
		for _, rec := range g.envelopesOf(place) {
			if rec.Task != slug || seenEnv[rec.ID] {
				continue
			}
			seenEnv[rec.ID] = true
			envs = append(envs, EnvelopeState{Record: rec, Alive: g.envelopeAlive(rec, metaOf)})
		}
	}
	sort.SliceStable(envs, func(i, j int) bool { return envs[i].Record.StartedAt.After(envs[j].Record.StartedAt) })
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].Meta.CreatedAt.After(runs[j].Meta.CreatedAt) })
	return envs, runs
}

// envelopeAlive: the owner of an open envelope is alive when the sidecar of
// its run says running with a live PID; before the run, after it, or with a
// sidecar without PID, only a fresh heartbeat says so.
func (g *gatherer) envelopeAlive(rec envelope.Record, metas map[string]runcmd.Meta) bool {
	if rec.Done() {
		return false
	}
	if m, ok := metas[rec.RunID]; ok && rec.RunID != "" && m.Status == runcmd.StatusRunning && m.PID > 0 {
		return runcmd.Alive(m.PID)
	}
	return g.now.Sub(rec.UpdatedAt) <= live.OrphanAfter
}

func (g *gatherer) envelopesOf(dir string) []envelope.Record {
	if _, ok := g.envelopes[dir]; !ok {
		g.envelopes[dir] = envelope.List(dir)
	}
	return g.envelopes[dir]
}

func (g *gatherer) metasOf(dir string) []runcmd.Meta {
	if _, ok := g.metas[dir]; !ok {
		g.metas[dir] = runcmd.Metas(dir)
	}
	return g.metas[dir]
}

// specMatches reports whether a verdict's spec field names the card's spec:
// cleaned, and relative to the evidence tree when it was written absolute.
func specMatches(vspec, dir, want string) bool {
	vspec = strings.TrimSpace(vspec)
	if vspec == "" {
		return false
	}
	if filepath.IsAbs(vspec) {
		rel, err := filepath.Rel(dir, vspec)
		if err != nil {
			return false
		}
		vspec = rel
	}
	return filepath.ToSlash(filepath.Clean(vspec)) == want
}

// porcelain lists every uncommitted path of dir, file by file (untracked
// directories expanded), with git's own quoting turned off.
func porcelain(dir string) []string {
	cmd := exec.Command("git", "-c", "core.quotePath=false", "status", "--porcelain", "--untracked-files=all", "-z")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		f := string(fields[i])
		if len(f) < 4 {
			continue
		}
		paths = append(paths, f[3:])
		if f[0] == 'R' || f[0] == 'C' {
			i++ // the origin of a rename travels in the next field
		}
	}
	return paths
}
