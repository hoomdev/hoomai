package boardcmd

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// DiffMaxBytes caps the patch of the detail's diff.
const DiffMaxBytes = 256 << 10

// How a criterion is covered.
const (
	TracedTest    = "test"
	TracedComando = "comando"
)

// Kinds of WorkRow.
const (
	WorkSobre = "sobre"
	WorkRun   = "run"
)

// Detail is everything the Studio shows about one card, read-only.
type Detail struct {
	Card        Card               `json:"card"`
	Spec        SpecDetail         `json:"spec"`
	Criteria    []CriterionTrace   `json:"criteria"`
	Verdict     *verdict.Verdict   `json:"verdict"`
	Fingerprint string             `json:"fingerprint"`
	Findings    []finding.Item     `json:"findings"`
	Reviews     []reviewcmd.Record `json:"reviews"`
	Work        []WorkRow          `json:"work"`
	Paths       Paths              `json:"paths"`
	Diff        *gitx.Diff         `json:"diff,omitempty"`
}

// SpecDetail is the card's spec as it is in the evidence tree.
type SpecDetail struct {
	Path     string           `json:"path"`
	Exists   bool             `json:"exists"`
	Markdown string           `json:"markdown"`
	Approval *approval.Record `json:"approval"`
}

// CriterionTrace is one criterion with how it is covered.
type CriterionTrace struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	TracedBy string   `json:"traced_by"` // TracedTest | TracedComando | ""
	Files    []string `json:"files"`
}

// WorkRow is one envelope of the card, or one run of the card that no
// envelope references.
type WorkRow struct {
	Kind       string           `json:"kind"` // WorkSobre | WorkRun
	ID         string           `json:"id"`
	RunID      string           `json:"run_id,omitempty"`
	Role       string           `json:"role"`
	Provider   string           `json:"provider"`
	Status     string           `json:"status"`
	Stage      string           `json:"stage,omitempty"`
	Step       int              `json:"step,omitempty"`
	Steps      int              `json:"steps,omitempty"`
	Isolated   bool             `json:"isolated"`
	StartedAt  time.Time        `json:"started_at"`
	EndedAt    *time.Time       `json:"ended_at,omitempty"`
	DurationMS *int64           `json:"duration_ms"`
	Alive      bool             `json:"alive"`
	Usage      *providers.Usage `json:"usage"`
	Note       string           `json:"note,omitempty"`
}

// Paths are the card's artifacts, relative to root ("" / empty when absent).
type Paths struct {
	Item     string   `json:"item"`
	Dir      string   `json:"dir"`
	Spec     string   `json:"spec"`
	Approval string   `json:"approval"`
	Verdict  string   `json:"verdict"`
	Reviews  []string `json:"reviews"`
	Findings []string `json:"findings"`
}

// NoDiffNote is what the detail says when there is no task worktree to
// diff.
const NoDiffNote = "sin espacio de trabajo de la tarea: no hay diff base...HEAD que mostrar"

// DetailFor reads the detail of one card. It fails like CardFor (missing or
// invalid item) and, like Gather, it creates and modifies nothing.
func DetailFor(root, base, blockOn, slug string, now time.Time, withDiff bool) (Detail, error) {
	it, err := loadItem(root, slug)
	if err != nil {
		return Detail{}, err
	}
	g := newGatherer(root, base, blockOn, now)
	ev, dir, t := g.gatherIn(it)
	d := Detail{
		Card:        Derive(ev),
		Spec:        SpecDetail{Path: ev.SpecPath, Exists: ev.SpecExists},
		Criteria:    []CriterionTrace{},
		Verdict:     ev.Verdict,
		Fingerprint: ev.Fingerprint,
		Findings:    []finding.Item{},
		Reviews:     append([]reviewcmd.Record{}, ev.Reviews...),
		Work:        workOf(ev),
	}
	join := func(parts ...string) string { return path.Join(append([]string{ev.Dir}, parts...)...) }
	d.Paths = Paths{Item: item.RelPath(slug), Dir: ev.Dir, Reviews: []string{}, Findings: []string{}}

	specAbs := filepath.Join(dir, filepath.FromSlash(ev.SpecPath))
	if ev.SpecExists {
		d.Paths.Spec = join(ev.SpecPath)
		if raw, err := os.ReadFile(specAbs); err == nil {
			d.Spec.Markdown = string(raw)
		}
		if state, rec, err := approval.Status(dir, specAbs); err == nil && state == approval.StatusApproved && rec != nil {
			d.Spec.Approval = rec
			d.Paths.Approval = join(".hoom", "approvals", slug+"_"+rec.SHA256[:8]+".json")
		}
	}
	if ev.SpecExists && len(ev.LintIssues) == 0 {
		d.Criteria = criteriaOf(specAbs, ev, t)
	}
	if ev.Verdict != nil {
		d.Paths.Verdict = join(".hoom", "verdicts", ev.Verdict.ID+".json")
	}
	for _, r := range d.Reviews {
		d.Paths.Reviews = append(d.Paths.Reviews, join(".hoom", reviewcmd.RecordsDir, r.ID+".json"))
	}
	for _, f := range t.allFindings(base) {
		if f.Task != slug {
			continue
		}
		d.Findings = append(d.Findings, f)
		d.Paths.Findings = append(d.Paths.Findings, join(".hoom", "findings", f.ID+".json"))
		if f.Resolution != nil {
			d.Paths.Findings = append(d.Paths.Findings, join(".hoom", "findings", f.ID+".res.json"))
		}
	}
	if withDiff {
		diff := gitx.Diff{Base: base, Files: []gitx.DiffFile{}, Note: NoDiffNote}
		if ev.Source == SourceWorktree {
			diff, _ = gitx.BranchDiff(dir, base, DiffMaxBytes)
		}
		d.Diff = &diff
	}
	return d, nil
}

// criteriaOf is every criterion with its statement and how it is covered,
// read from the same token index the column used.
func criteriaOf(specAbs string, ev Evidence, t *tree) []CriterionTrace {
	out := []CriterionTrace{}
	cs, err := spec.Criteria(specAbs)
	if err != nil {
		return out
	}
	idx, _ := t.tokenIndex()
	byCmd := set(ev.ByCommand)
	for _, c := range cs {
		ct := CriterionTrace{ID: c.ID, Text: c.Text, Files: idx.Files(c.ID)}
		switch {
		case byCmd[c.ID]:
			ct.TracedBy = TracedComando
		case len(ct.Files) > 0:
			ct.TracedBy = TracedTest
		}
		out = append(out, ct)
	}
	return out
}

// workOf lists what ran for the card: one row per envelope, and one per run
// no envelope references. The usage of each row follows the rule of spend,
// so the rows add up to exactly the card's spend.
func workOf(ev Evidence) []WorkRow {
	sidecars := map[string]runcmd.Meta{}
	for _, r := range ev.Runs {
		sidecars[r.Meta.ID] = r.Meta
	}
	since := func(from, to time.Time) *int64 {
		ms := to.Sub(from).Milliseconds()
		if ms < 0 {
			ms = 0
		}
		return &ms
	}
	rows := []WorkRow{}
	referenced := map[string]bool{}
	for _, e := range ev.Envelopes {
		rec := e.Record
		row := WorkRow{Kind: WorkSobre, ID: rec.ID, RunID: rec.RunID, Role: rec.Role, Provider: rec.Provider,
			Status: rec.Status, Stage: rec.Stage, Step: rec.Step, Steps: rec.Steps, Isolated: rec.Isolated,
			StartedAt: rec.StartedAt, Alive: e.Alive, Note: rec.Note}
		if rec.RunID != "" && !referenced[rec.RunID] {
			referenced[rec.RunID] = true
			if m, ok := sidecars[rec.RunID]; ok {
				row.Usage = m.Usage
			} else {
				row.Usage = rec.Usage
			}
		}
		switch {
		case rec.Done():
			if !rec.EndedAt.IsZero() {
				end := rec.EndedAt
				row.EndedAt = &end
			}
			row.DurationMS = since(rec.StartedAt, endOf(rec))
		case e.Alive:
			row.DurationMS = since(rec.StartedAt, ev.Now)
		default:
			row.DurationMS = since(rec.StartedAt, rec.UpdatedAt)
		}
		rows = append(rows, row)
	}
	for _, r := range ev.Runs {
		m := r.Meta
		if referenced[m.ID] {
			continue
		}
		row := WorkRow{Kind: WorkRun, ID: m.ID, Role: m.Role, Provider: m.Provider, Status: m.Status,
			Isolated: m.Isolated, StartedAt: m.CreatedAt, Alive: r.Alive, Usage: m.Usage}
		switch {
		case !m.EndedAt.IsZero():
			end := m.EndedAt
			row.EndedAt = &end
			row.DurationMS = since(m.CreatedAt, m.EndedAt)
		case r.Alive:
			row.DurationMS = since(m.CreatedAt, ev.Now)
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].StartedAt.After(rows[j].StartedAt) })
	return rows
}
