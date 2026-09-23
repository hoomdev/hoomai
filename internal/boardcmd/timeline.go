package boardcmd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Timeline sources: where an entry was read. The page labels them, so a
// person on another machine knows why the agents' work is missing.
const (
	SourceGit        = "git"
	SourceTelemetria = "telemetria"
)

// Timeline entry kinds, in their tie-break order.
const (
	KindItem         = "item"
	KindSpec         = "spec"
	KindAprobacion   = "aprobacion"
	KindCommit       = "commit"
	KindVeredicto    = "veredicto"
	KindHallazgo     = "hallazgo"
	KindResolucion   = "resolucion"
	KindReview       = "review"
	KindSobre        = "sobre"
	KindRun          = "run"
	KindSubagente    = "subagente"
	KindSubagenteFin = "subagente-fin"
)

var kindOrder = []string{KindItem, KindSpec, KindAprobacion, KindCommit, KindVeredicto, KindHallazgo,
	KindResolucion, KindReview, KindSobre, KindRun, KindSubagente, KindSubagenteFin}

// TimelinePatchMax caps the patch of the task's commits read to light the
// criteria.
const TimelinePatchMax = 8 << 20

// The notes of a timeline, verbatim.
const (
	NoteSinTelemetria = "sin telemetria en esta computadora: el trabajo de los agentes solo se ve donde corrio"
	NoteFastForward   = "no se pueden separar los commits de la tarea: se integro sin commit de merge"
	NoteSinEspacio    = "la tarjeta no tiene espacio de trabajo propio: la historia muestra solo su evidencia"
	NoteTestsCortados = "la historia de los tests se corto en 8 MiB: los criterios se encienden con el veredicto"
	NoteSinGitPrefijo = "sin historial de git: "
)

// Timeline is the story of one card: how it got where it is.
type Timeline struct {
	Slug      string          `json:"slug"`
	Card      Card            `json:"card"`  // the card now: the replay's last frame
	Meter     []MeterSlot     `json:"meter"` // the replay's meter skeleton (card.meter's ids and labels)
	Entries   []TimelineEntry `json:"entries"`
	Telemetry TelemetryCount  `json:"telemetry"`
	Notes     []string        `json:"notes"`
}

// MeterSlot is one segment of the replay's meter.
type MeterSlot struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// MeterEffect is what one entry does to one segment of the replay's meter.
type MeterEffect struct {
	ID    string `json:"id"`
	State string `json:"state"` // SegHecho | SegFalta | SegNoAplica
}

// TimelineEntry is one moment of the card's story.
type TimelineEntry struct {
	At       time.Time     `json:"at"`
	EndedAt  *time.Time    `json:"ended_at"`
	Source   string        `json:"source"` // SourceGit | SourceTelemetria
	Kind     string        `json:"kind"`
	Who      string        `json:"who"` // git identity, or "<rol> (<provider>)"
	Role     string        `json:"role"`
	Provider string        `json:"provider"`
	Artifact string        `json:"artifact"` // relative to root; "" for a code commit
	Ref      string        `json:"ref"`      // sha, envelope id, run id
	Summary  string        `json:"summary"`  // expert mode
	Plain    string        `json:"plain"`    // normal mode
	CostUSD  *float64      `json:"cost_usd"`
	Tokens   int           `json:"tokens"`
	Piloto   bool          `json:"piloto"`
	Meter    []MeterEffect `json:"meter"`
}

// TelemetryCount is how many envelopes and runs of the card this machine has.
type TelemetryCount struct {
	Sobres int `json:"sobres"`
	Runs   int `json:"runs"`
}

// TimelineFor reads the story of one card. It fails like CardFor and, like
// Gather, creates and modifies nothing: it only runs git reads.
func TimelineFor(root, base, blockOn, slug string, now time.Time) (Timeline, error) {
	it, err := loadItem(root, slug)
	if err != nil {
		return Timeline{}, err
	}
	g := newGatherer(root, base, blockOn, now)
	ev, dir, t := g.gatherIn(it)
	c := Derive(ev)
	tl := Timeline{Slug: slug, Card: c, Meter: []MeterSlot{}, Entries: []TimelineEntry{}, Notes: []string{},
		Telemetry: TelemetryCount{Sobres: len(ev.Envelopes), Runs: len(ev.Runs)}}
	for _, s := range c.Meter {
		tl.Meter = append(tl.Meter, MeterSlot{ID: s.ID, Label: s.Label})
	}

	h := &history{root: root, base: base, dir: dir, ev: ev, t: t, extra: map[int]*entryData{}}
	if err := h.gitEntries(); err != nil {
		tl.Notes = append(tl.Notes, NoteSinGitPrefijo+err.Error())
	}
	h.telemetry()
	tl.Notes = append(tl.Notes, h.notes...)
	if len(ev.Envelopes) == 0 && len(ev.Runs) == 0 {
		tl.Notes = append(tl.Notes, NoteSinTelemetria)
	}
	tl.Entries = h.ordered()
	return tl, nil
}

// entryData is what the meter pass needs about a git entry, kept out of
// the JSON.
type entryData struct {
	letter       string
	specHash     string   // spec entries: SHA-256 of the spec at that commit
	approvalHash string   // aprobacion entries: the hash it signs
	criteria     []string // commit entries: CA tokens its test lines add
	verdict      *verdict.Verdict
	lenses       []string
}

type history struct {
	root, base, dir string
	ev              Evidence
	t               *tree
	entries         []TimelineEntry
	extra           map[int]*entryData // by index in entries
	notes           []string
	approvalsAt     map[string][]string // sha -> hashes approved in that commit
}

func (h *history) add(e TimelineEntry, d *entryData) {
	if e.Meter == nil {
		e.Meter = []MeterEffect{}
	}
	h.entries = append(h.entries, e)
	if d != nil {
		h.extra[len(h.entries)-1] = d
	}
}

// rel is a path of the evidence tree, relative to root.
func (h *history) rel(p string) string { return path.Join(h.ev.Dir, p) }

// --- git ---

type gitCommit struct {
	sha, who, subject string
	at                time.Time
	files             []gitFile
}

type gitFile struct{ letter, path string }

func (h *history) gitEntries() error {
	s := h.ev.Item.Slug
	h.approvalsAt = map[string][]string{}
	specPath := specOf(h.ev)

	// the card's evidence files in its evidence tree
	byVerdict := map[string]*verdict.Verdict{}
	for _, v := range h.t.allVerdicts() {
		if specMatches(v.Spec, h.dir, h.ev.SpecPath) {
			byVerdict[".hoom/verdicts/"+v.ID+".json"] = v
		}
	}
	findings := map[string]finding.Item{}
	resolutions := map[string]finding.Item{}
	for _, f := range h.t.allFindings(h.base) {
		if f.Task == s {
			findings[".hoom/findings/"+f.ID+".json"] = f
			resolutions[".hoom/findings/"+f.ID+".res.json"] = f
		}
	}
	reviews := map[string]reviewcmd.Record{}
	for _, r := range h.ev.Reviews {
		reviews[".hoom/"+reviewcmd.RecordsDir+"/"+r.ID+".json"] = r
	}
	approvalsGlob := ".hoom/approvals/" + s + "_*.json"
	specs := []string{specPath, ":(glob)" + approvalsGlob}
	for p := range byVerdict {
		specs = append(specs, p)
	}
	for p := range findings {
		specs = append(specs, p)
	}
	for p := range resolutions {
		specs = append(specs, p)
	}
	for p := range reviews {
		specs = append(specs, p)
	}
	itemPath := ".hoom/items/" + s + ".yaml"
	sameTree := filepath.Clean(h.dir) == filepath.Clean(h.root)
	if sameTree {
		specs = append(specs, itemPath)
	}
	commits, err := logFiles(h.dir, specs)
	if err != nil {
		return err
	}
	if !sameTree {
		itemCommits, err := logFiles(h.root, []string{itemPath})
		if err != nil {
			return err
		}
		for _, c := range itemCommits {
			h.fileEntries(c, "", func(string) string { return KindItem }, byVerdict, findings, resolutions, reviews)
		}
	}
	kindOf := func(p string) string {
		switch {
		case p == itemPath && sameTree:
			return KindItem
		case p == specPath:
			return KindSpec
		case strings.HasPrefix(p, ".hoom/approvals/"):
			return KindAprobacion
		case byVerdict[p] != nil:
			return KindVeredicto
		case strings.HasSuffix(p, ".res.json"):
			if _, ok := resolutions[p]; ok {
				return KindResolucion
			}
		case strings.HasPrefix(p, ".hoom/findings/"):
			if _, ok := findings[p]; ok {
				return KindHallazgo
			}
		case strings.HasPrefix(p, ".hoom/"+reviewcmd.RecordsDir+"/"):
			return KindReview
		}
		return ""
	}
	for _, c := range commits {
		h.fileEntries(c, h.ev.Dir, kindOf, byVerdict, findings, resolutions, reviews)
	}
	return h.taskCommits()
}

// fileEntries turns one commit into one entry per card file it touched.
func (h *history) fileEntries(c gitCommit, prefix string, kindOf func(string) string,
	verdicts map[string]*verdict.Verdict, findings, resolutions map[string]finding.Item, reviews map[string]reviewcmd.Record) {
	gitDir := h.dir
	if prefix == "" && filepath.Clean(h.dir) != filepath.Clean(h.root) {
		gitDir = h.root
	}
	for _, f := range c.files {
		kind := kindOf(f.path)
		if kind == "" {
			continue
		}
		d := &entryData{letter: f.letter}
		e := TimelineEntry{At: c.at, Source: SourceGit, Kind: kind, Who: c.who, Ref: c.sha,
			Artifact: path.Join(orDot(prefix), f.path)}
		e.Summary = fmt.Sprintf("%s %s %s: %s", short(c.sha), f.letter, f.path, c.subject)
		deleted := f.letter == "D"
		switch kind {
		case KindItem:
			e.Plain = pick(f.letter, "se creo la tarjeta", "cambio la tarjeta", "se borro la tarjeta")
		case KindSpec:
			e.Plain = pick(f.letter, "se escribio el spec", "cambio el spec", "se borro el spec")
			if !deleted {
				if raw, err := blob(gitDir, c.sha, f.path); err == nil {
					sum := sha256.Sum256(raw)
					d.specHash = hex.EncodeToString(sum[:])
				}
			}
		case KindAprobacion:
			e.Plain = "se aprobo el spec"
			if deleted {
				e.Plain = "se borro una aprobacion del spec"
				break
			}
			if raw, err := blob(gitDir, c.sha, f.path); err == nil {
				var rec approval.Record
				if json.Unmarshal(raw, &rec) == nil && rec.SHA256 != "" {
					d.approvalHash = rec.SHA256
					h.approvalsAt[c.sha] = append(h.approvalsAt[c.sha], rec.SHA256)
					if rec.ApprovedBy != "" {
						e.Summary += " (firmada por " + rec.ApprovedBy + ")"
					}
				}
			}
		case KindVeredicto:
			v := verdicts[f.path]
			d.verdict = v
			switch {
			case deleted || v == nil:
				e.Plain = "se borro un veredicto"
			case v.IsPartial():
				e.Plain = "una verificacion parcial dio " + color(v.Verdict)
			case v.Verdict == "green":
				e.Plain = "la verificacion dio verde"
			default:
				e.Plain = plainVerdict(v)
			}
		case KindHallazgo:
			e.Plain = "se borro un problema de la revision"
			if fi, ok := findings[f.path]; ok && !deleted {
				e.Plain = "la revision encontro un problema (" + fi.Severity + ")"
			}
		case KindResolucion:
			e.Plain = "se resolvio un problema de la revision"
			if fi, ok := resolutions[f.path]; ok && !deleted && fi.Resolution != nil && fi.Resolution.As != "" {
				e.Plain += " (" + fi.Resolution.As + ")"
			}
		case KindReview:
			e.Plain = "quedo registrada una revision"
			if r, ok := reviews[f.path]; ok {
				d.lenses = r.Lenses
				if containsAll(r.Lenses, reviewcmd.Lentes) {
					e.Plain = "quedo registrada la revision de 4 lentes"
				}
			}
			if deleted {
				e.Plain = "se borro un registro de revision"
				d.lenses = nil
			}
		}
		h.add(e, d)
	}
}

// taskCommits adds the commits of the card's task, in the range the spec
// fixes, with the CA tokens their test lines add.
func (h *history) taskCommits() error {
	s := h.ev.Item.Slug
	gitDir, rng := h.root, ""
	switch {
	case h.ev.Item.CommitFinal != "":
		cf := h.ev.Item.CommitFinal
		if !gitOK(h.root, "merge-base", "--is-ancestor", cf, h.base) {
			rng = h.base + ".." + cf
			break
		}
		out, err := gitRun(h.root, "rev-list", "--first-parent", "--ancestry-path", cf+".."+h.base)
		lines := strings.Fields(out)
		if err != nil || len(lines) == 0 {
			h.notes = append(h.notes, NoteFastForward)
			return nil
		}
		m := lines[len(lines)-1]
		parents, _ := gitRun(h.root, "rev-list", "--parents", "-n", "1", m)
		if len(strings.Fields(parents)) < 3 {
			h.notes = append(h.notes, NoteFastForward)
			return nil
		}
		rng = m + "^1.." + cf
	case h.ev.Worktree:
		gitDir, rng = h.dir, h.base+"..HEAD"
	case gitOK(h.root, "rev-parse", "--verify", "--quiet", "refs/heads/hoom/"+s):
		rng = h.base + "..hoom/" + s
	default:
		h.notes = append(h.notes, NoteSinEspacio)
		return nil
	}
	commits, err := logNames(gitDir, rng)
	if err != nil {
		return err
	}
	cites, truncated := testTokens(gitDir, rng)
	if truncated {
		h.notes = append(h.notes, NoteTestsCortados)
	}
	for _, c := range commits {
		n := len(c.files)
		e := TimelineEntry{At: c.at, Source: SourceGit, Kind: KindCommit, Who: c.who, Ref: c.sha,
			Summary: fmt.Sprintf("%s: %s (%s)", short(c.sha), c.subject, count(n, "archivo", "archivos")),
			Plain:   "se guardaron cambios en " + count(n, "archivo", "archivos")}
		h.add(e, &entryData{criteria: cites[c.sha]})
	}
	return nil
}

// --- telemetria ---

func (h *history) telemetry() {
	ev := h.ev
	records := map[string]envelope.Record{}
	for _, e := range ev.Envelopes {
		records[e.Record.ID] = e.Record
	}
	for _, w := range workOf(ev) {
		e := TimelineEntry{At: w.StartedAt, EndedAt: w.EndedAt, Source: SourceTelemetria, Role: w.Role,
			Provider: w.Provider, Ref: w.ID, Who: whoOf(w.Role, w.Provider)}
		if w.Usage != nil {
			if w.Usage.CostUSD != nil {
				v := *w.Usage.CostUSD
				e.CostUSD = &v
			}
			e.Tokens = w.Usage.InputTokens + w.Usage.OutputTokens
		}
		role := orRol(w.Role)
		if w.Kind == WorkSobre {
			rec := records[w.ID]
			e.Kind, e.Piloto = KindSobre, rec.Pilot
			e.Artifact = h.where(".hoom/" + envelope.DirName + "/" + w.ID + ".json")
			e.Summary = fmt.Sprintf("sobre %s: %s en %s (paso %d de %d)", w.ID, w.Status, w.Stage, w.Step, w.Steps)
			if n := strings.TrimSpace(w.Note); n != "" {
				e.Summary += ": " + n
			}
			switch {
			case w.Status == envelope.StatusDeliverable:
				e.Plain = "el " + role + " entrego su trabajo"
			case w.Status == envelope.StatusNotDeliverable || w.Status == envelope.StatusNoDelivery:
				e.Plain = plainEnvelope(rec)
			case w.Alive:
				e.Plain = "el " + role + " esta trabajando"
			default:
				e.Plain = "el trabajo del " + role + " quedo interrumpido"
			}
		} else {
			who := w.Role
			if who == "" {
				who = w.Provider
			}
			e.Kind = KindRun
			e.Artifact = h.where(".hoom/runs/" + w.ID + ".jsonl")
			e.Summary = fmt.Sprintf("run %s: %s", w.ID, w.Status)
			e.Plain = "trabajo el " + who
			if w.Status == runcmd.StatusError {
				e.Plain = "el trabajo del " + who + " fallo"
			}
		}
		h.add(e, nil)
	}
	h.delegations()
}

// delegations reads the events of every run of the card: each delegation
// enters the scene, and leaves it when its agent_end arrives.
func (h *history) delegations() {
	type runInfo struct{ role, provider string }
	runs := map[string]runInfo{}
	var ids []string
	for _, r := range h.ev.Runs {
		if _, ok := runs[r.Meta.ID]; !ok {
			ids = append(ids, r.Meta.ID)
		}
		runs[r.Meta.ID] = runInfo{role: r.Meta.Role, provider: r.Meta.Provider}
	}
	for _, e := range h.ev.Envelopes {
		id := e.Record.RunID
		if id == "" {
			continue
		}
		info, ok := runs[id]
		if !ok {
			ids = append(ids, id)
		}
		if info.role == "" {
			info.role = e.Record.Role
		}
		if info.provider == "" {
			info.provider = e.Record.Provider
		}
		runs[id] = info
	}
	sort.Strings(ids)
	for _, id := range ids {
		info := runs[id]
		events, artifact := h.runEvents(id)
		open := map[string]int{} // tool id -> index of its entry
		for _, ev := range events {
			switch ev.Kind {
			case "agent":
				who := info.role
				if who == "" {
					who = info.provider
				}
				ref := ev.ToolID
				if ref == "" {
					ref = id
				}
				h.add(TimelineEntry{At: ev.TS, Source: SourceTelemetria, Kind: KindSubagente,
					Who: whoOf(ev.Agent, info.provider), Role: ev.Agent, Provider: info.provider,
					Artifact: artifact, Ref: ref,
					Summary: fmt.Sprintf("run %s: delego en %s: %s", id, ev.Agent, ev.Detail),
					Plain:   "el " + orRol(who) + " le paso trabajo a " + ev.Agent}, nil)
				if ev.ToolID != "" {
					if _, dup := open[ev.ToolID]; !dup {
						open[ev.ToolID] = len(h.entries) - 1
					}
				}
			case "agent_end":
				i, ok := open[ev.ToolID]
				if !ok || ev.ToolID == "" {
					continue
				}
				delete(open, ev.ToolID)
				ts := ev.TS
				h.entries[i].EndedAt = &ts
				plain := ev.Agent + " termino su parte"
				if strings.Contains(ev.Detail, " fallo") {
					plain = ev.Agent + " fallo"
				}
				h.add(TimelineEntry{At: ev.TS, Source: SourceTelemetria, Kind: KindSubagenteFin,
					Who: whoOf(ev.Agent, info.provider), Role: ev.Agent, Provider: info.provider,
					Artifact: artifact, Ref: ev.ToolID,
					Summary: fmt.Sprintf("run %s: %s", id, ev.Detail), Plain: plain}, nil)
			}
		}
	}
}

// runEvents reads a run's narration where it ran: the root, or the card's
// worktree.
func (h *history) runEvents(id string) ([]runcmd.Event, string) {
	if evs, err := runcmd.ReadEvents(h.root, id); err == nil {
		return evs, ".hoom/runs/" + id + ".jsonl"
	}
	if h.dir != h.root {
		if evs, err := runcmd.ReadEvents(h.dir, id); err == nil {
			return evs, h.rel(".hoom/runs/" + id + ".jsonl")
		}
	}
	return nil, ".hoom/runs/" + id + ".jsonl"
}

// where is a telemetry file relative to root: in root when it is there,
// in the card's worktree otherwise.
func (h *history) where(rel string) string {
	if _, err := os.Stat(filepath.Join(h.root, filepath.FromSlash(rel))); err == nil || h.dir == h.root {
		return rel
	}
	return h.rel(rel)
}

// --- orden y medidor ---

// ordered sorts the entries and computes their meter effects in that order.
func (h *history) ordered() []TimelineEntry {
	idx := make([]int, len(h.entries))
	for i := range idx {
		idx[i] = i
	}
	rank := func(kind string) int {
		for i, k := range kindOrder {
			if k == kind {
				return i
			}
		}
		return len(kindOrder)
	}
	src := func(s string) int {
		if s == SourceGit {
			return 0
		}
		return 1
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := h.entries[idx[a]], h.entries[idx[b]]
		switch {
		case !x.At.Equal(y.At):
			return x.At.Before(y.At)
		case src(x.Source) != src(y.Source):
			return src(x.Source) < src(y.Source)
		case rank(x.Kind) != rank(y.Kind):
			return rank(x.Kind) < rank(y.Kind)
		case x.Ref != y.Ref:
			return x.Ref < y.Ref
		}
		return x.Artifact < y.Artifact
	})

	criteria := h.ev.Criteria
	isCriterion := set(criteria)
	approved := map[string]bool{}
	specHash := ""
	cited := map[string]bool{} // tokens a task commit already brought to a test
	out := make([]TimelineEntry, 0, len(idx))
	for _, i := range idx {
		e := h.entries[i]
		d := h.extra[i]
		if d != nil {
			e.Meter = h.effects(e, d, criteria, isCriterion, approved, &specHash, cited)
		}
		out = append(out, e)
	}
	return out
}

func (h *history) effects(e TimelineEntry, d *entryData, criteria []string, isCriterion, approved map[string]bool,
	specHash *string, cited map[string]bool) []MeterEffect {
	fx := []MeterEffect{}
	put := func(id, state string) { fx = append(fx, MeterEffect{ID: id, State: state}) }
	switch e.Kind {
	case KindSpec:
		for _, hsh := range h.approvalsAt[e.Ref] {
			approved[hsh] = true
		}
		if d.letter == "D" {
			*specHash = ""
			put("spec", SegFalta)
			break
		}
		*specHash = d.specHash
		if d.specHash != "" && approved[d.specHash] {
			put("spec", SegHecho)
		} else {
			put("spec", SegFalta)
		}
	case KindAprobacion:
		if d.approvalHash == "" {
			break
		}
		approved[d.approvalHash] = true
		if d.approvalHash == *specHash {
			put("spec", SegHecho)
		}
	case KindCommit:
		for _, tok := range d.criteria {
			// la primera vez que un commit de la tarea lo cita en un test,
			// aunque un veredicto ya lo haya encendido antes
			if isCriterion[tok] && !cited[tok] {
				cited[tok] = true
				put(tok, SegHecho)
			}
		}
	case KindVeredicto:
		v := d.verdict
		if v == nil || v.IsPartial() || d.letter == "D" {
			break
		}
		if gateStatus(v, "spec_trace") == verdict.StatusPass {
			for _, id := range criteria {
				put(id, SegHecho)
			}
		}
		for _, g := range meterGates {
			switch st := gateStatus(v, g); {
			case st == "":
				put(g, SegNoAplica)
			case st == verdict.StatusPass:
				put(g, SegHecho)
			default:
				put(g, SegFalta)
			}
		}
		if _, required := reviewRequired(v); !required {
			put("review", SegNoAplica)
		}
	case KindReview:
		if containsAll(d.lenses, reviewcmd.Lentes) {
			put("review", SegHecho)
		}
	}
	return fx
}

// --- git helpers ---

const logFormat = "--format=%x1e%H%x1f%aI%x1f%an <%ae>%x1f%s"

func gitRun(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.quotePath=false"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return string(out), nil
}

func gitOK(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// logFiles is `git log --name-status` over pathspecs: one commit per record.
func logFiles(dir string, pathspecs []string) ([]gitCommit, error) {
	args := append([]string{"log", "--no-renames", logFormat, "--name-status", "--"}, pathspecs...)
	out, err := gitRun(dir, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(out, true), nil
}

// logNames is `git log --no-merges --name-only` over a range.
func logNames(dir, rng string) ([]gitCommit, error) {
	out, err := gitRun(dir, "log", "--no-merges", "--no-renames", logFormat, "--name-only", rng, "--")
	if err != nil {
		return nil, err
	}
	return parseLog(out, false), nil
}

func parseLog(out string, status bool) []gitCommit {
	var commits []gitCommit
	for _, rec := range strings.Split(out, "\x1e") {
		if strings.TrimSpace(rec) == "" {
			continue
		}
		lines := strings.Split(rec, "\n")
		head := strings.SplitN(lines[0], "\x1f", 4)
		if len(head) < 4 {
			continue
		}
		at, _ := time.Parse(time.RFC3339, head[1])
		c := gitCommit{sha: head[0], at: at.UTC(), who: head[2], subject: head[3]}
		for _, l := range lines[1:] {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if !status {
				c.files = append(c.files, gitFile{path: l})
				continue
			}
			parts := strings.SplitN(l, "\t", 2)
			if len(parts) == 2 {
				c.files = append(c.files, gitFile{letter: parts[0][:1], path: parts[1]})
			}
		}
		commits = append(commits, c)
	}
	return commits
}

// testTokens reads the patch of a range and returns, per commit, the CA
// tokens its added lines bring to test files (spec_trace's filter). The
// patch is read up to TimelinePatchMax.
func testTokens(dir, rng string) (map[string][]string, bool) {
	out := map[string][]string{}
	cmd := exec.Command("git", "-c", "core.quotePath=false", "log", "--no-merges", "--no-renames",
		"--format=%x1e%H", "-p", "--unified=0", "--no-color", "--no-ext-diff", rng, "--")
	cmd.Dir = dir
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return out, false
	}
	if err := cmd.Start(); err != nil {
		return out, false
	}
	limited := &countingReader{r: pipe, max: TimelinePatchMax}
	sc := bufio.NewScanner(limited)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	sha, file := "", ""
	seen := map[string]bool{}
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "\x1e"):
			sha, file = strings.TrimSpace(l[1:]), ""
			seen = map[string]bool{}
		case strings.HasPrefix(l, "diff --git "):
			file = ""
		case strings.HasPrefix(l, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(l, "+++ "), "b/")
		case strings.HasPrefix(l, "+") && file != "" && sha != "" && spec.IsTestPath(file):
			for _, tok := range spec.TokensIn(l[1:]) {
				if !seen[tok] {
					seen[tok] = true
					out[sha] = append(out[sha], tok)
				}
			}
		}
	}
	truncated := limited.hit
	if truncated && cmd.Process != nil {
		cmd.Process.Kill()
	}
	io.Copy(io.Discard, pipe)
	cmd.Wait()
	return out, truncated
}

// countingReader stops at max bytes and says it did.
type countingReader struct {
	r   io.Reader
	n   int
	max int
	hit bool
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n >= c.max {
		c.hit = true
		return 0, io.EOF
	}
	if rest := c.max - c.n; len(p) > rest {
		p = p[:rest]
	}
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// blob is a file's bytes at a commit, exactly as git stores them.
func blob(dir, sha, p string) ([]byte, error) {
	cmd := exec.Command("git", "cat-file", "blob", sha+":"+p)
	cmd.Dir = dir
	return cmd.Output()
}

// --- small helpers ---

func pick(letter, added, modified, deleted string) string {
	switch letter {
	case "A":
		return added
	case "D":
		return deleted
	}
	return modified
}

func color(v string) string {
	if v == "green" {
		return "verde"
	}
	return "rojo"
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func orDot(p string) string {
	if p == "" {
		return "."
	}
	return p
}

func orRol(role string) string {
	if role == "" {
		return "rol"
	}
	return role
}

func whoOf(role, provider string) string {
	switch {
	case role != "" && provider != "":
		return role + " (" + provider + ")"
	case role != "":
		return role
	}
	return provider
}
