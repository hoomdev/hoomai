package boardcmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Problem ids, in the order the doctor checks a card.
const (
	ProbSpecEditado      = "spec-editado"
	ProbSinVeredicto     = "sin-veredicto"
	ProbVerdeVencido     = "verde-vencido"
	ProbBugDerivacion    = "bug-derivacion"
	ProbSobreHuerfano    = "sobre-huerfano"
	ProbSinSpec          = "sin-spec"
	ProbEvidenciaSinItem = "evidencia-sin-item"
)

// DoctorSinSpecDias: an item older than this without a spec is a problem.
const DoctorSinSpecDias = 14

// Problem is one place where the evidence does not add up, with the exact
// action that fixes it.
type Problem struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`   // the card, or the task; "" when none
	What   string `json:"what"`   // expert words
	Plain  string `json:"plain"`  // normal words; "" for a project problem
	Action string `json:"action"` // the exact command (or instruction)
	Ref    string `json:"ref"`    // the artifact it talks about
}

// DoctorReport is what `hoom board doctor` prints.
type DoctorReport struct {
	Problems []Problem `json:"problems"`
}

// DoctorOptions mirror the doctor verb's flags.
type DoctorOptions struct {
	JSON bool
}

// DoctorUsageText is the exact usage block of `hoom board doctor`.
const DoctorUsageText = `Uso: hoom board doctor [--json]

  --json   Emite los problemas como JSON en stdout

Lista cada lugar donde la evidencia de las tarjetas no es coherente, con su
accion exacta. Solo lee: no arregla nada. Sale con 0 siempre que entendio el
pedido; los que bloquean son 'hoom verify' y 'hoom check'.`

// DoctorOf is the card's own problems, in the fixed order. Pure: Derive calls
// it with the evidence and the card it just derived. A card in Hecho is
// terminal: only an orphan envelope is still worth saying.
func DoctorOf(ev Evidence, c Card) []Problem {
	out := []Problem{}
	s := ev.Item.Slug
	spec := specOf(ev)
	cd := ""
	if ev.Dir != "" && ev.Dir != "." {
		cd = "cd " + ev.Dir + " && "
	}
	at := func(parts ...string) string { return path.Join(append([]string{ev.Dir}, parts...)...) }
	// problema fills in what every problem of the card shares; each rule
	// below says only its own fields, by name (findings 1bbb70, c26bb0)
	problema := func(id string, p Problem) Problem {
		p.ID, p.Slug = id, s
		return p
	}
	hecho := c.Column == ColHecho
	v := ev.Verdict
	green := v != nil && v.Verdict == "green"

	if !hecho && ev.Approval == approval.StatusInvalidated {
		out = append(out, problema(ProbSpecEditado, Problem{
			What:   "el spec cambio despues de su aprobacion: revisalo y aprobalo de nuevo",
			Plain:  "el spec cambio despues de tu aprobacion",
			Action: cd + "hoom spec approve " + spec,
			Ref:    at(spec)}))
	}
	if !hecho && ev.Worktree && len(ev.TreeChanges) > 0 && !ev.Certified && !green {
		action := cd + "hoom verify"
		if ev.SpecExists {
			action += " --spec " + spec
		}
		out = append(out, problema(ProbSinVeredicto, Problem{
			What:   fmt.Sprintf("el espacio de trabajo tiene cambios que ningun veredicto certifica (huella %s)", ev.TreeFingerprint),
			Plain:  "hay cambios sin verificar",
			Action: action,
			Ref:    ev.Dir}))
	}
	if !hecho && green && ev.TreeFingerprint != "" && v.Git.ChangeFingerprint != ev.TreeFingerprint {
		out = append(out, problema(ProbVerdeVencido, Problem{
			What:   fmt.Sprintf("el veredicto verde %s certifica la huella %s y el arbol tiene %s", v.ID, v.Git.ChangeFingerprint, ev.TreeFingerprint),
			Plain:  "el codigo cambio despues del ultimo verde",
			Action: cd + "hoom verify --spec " + spec,
			Ref:    at(".hoom", "verdicts", v.ID+".json")}))
	}
	if !hecho && c.Column == ColTuAceptacion {
		for _, f := range ev.Findings {
			if f.Status != finding.StatusOpen || f.Task != s || f.Severity != "high" {
				continue
			}
			out = append(out, problema(ProbBugDerivacion, Problem{
				What:   fmt.Sprintf("bug de hoom: la tarjeta esta en Tu aceptacion con el hallazgo high abierto %s, y la derivacion nunca deberia permitirlo", f.ID),
				Plain:  "hay un error de hoom en esta tarjeta: no la integres",
				Action: "reporta este bug de hoom con la salida de 'hoom board doctor --json' y no integres la tarjeta hasta entonces",
				Ref:    at(".hoom", "findings", f.ID+".json")}))
		}
	}
	if in := c.Interrupted; in != nil {
		role := in.Role
		if role == "" {
			role = "rol"
		}
		out = append(out, problema(ProbSobreHuerfano, Problem{
			What:   fmt.Sprintf("el sobre %s del %s quedo abierto en el paso %d de %d (%s) y su proceso ya no vive", in.EnvelopeID, role, in.Step, in.Steps, in.Stage),
			Plain:  "el trabajo del " + role + " quedo interrumpido",
			Action: orphanAction(c, in, spec),
			Ref:    ".hoom/" + envelope.DirName + "/" + in.EnvelopeID + ".json"}))
	}
	if !hecho && !ev.SpecExists && !ev.Item.CreadoEn.IsZero() {
		if age := ev.Now.Sub(ev.Item.CreadoEn); age > DoctorSinSpecDias*24*time.Hour {
			d := int(age / (24 * time.Hour))
			out = append(out, problema(ProbSinSpec, Problem{
				What:   fmt.Sprintf("el item lleva %d dias sin spec (creado el %s)", d, ev.Item.CreadoEn.UTC().Format("2006-01-02")),
				Plain:  fmt.Sprintf("lleva %d dias esperando su spec", d),
				Action: c.Next,
				Ref:    item.RelPath(s)}))
		}
	}
	return out
}

// orphanAction is how the interrupted work goes on: resumed when the card
// can resume it, the review again, or the role again from scratch.
func orphanAction(c Card, in *Interrupted, spec string) string {
	// the role and the session id come from local telemetry: they go into a
	// command only when they are hoom's (finding f34bc3), and the role as
	// hoom writes it, not as the record does: Lookup forgives spaces around
	// it, and a newline would split the command in two (finding fffe28)
	r, err := agents.Lookup(in.Role)
	if err != nil {
		return fmt.Sprintf("el sobre dice un rol que no es de hoom (%q): relanza el trabajo desde la tarjeta", in.Role)
	}
	role := r.Slug
	withSpec := ""
	if !WritesSpecs(role) {
		withSpec = " --spec " + spec
	}
	for _, a := range c.Actions {
		if a.ID == ActReanudar && a.Enabled && sesionSegura.MatchString(a.ResumeID) && sesionSegura.MatchString(in.Provider) {
			return fmt.Sprintf("hoom agent --role %s --task %s%s --provider %s --resume %s %s",
				role, c.Slug, withSpec, in.Provider, a.ResumeID, pedidoHint)
		}
	}
	if role == "reviewer" {
		return "hoom review --task " + c.Slug + " --spec " + spec
	}
	return fmt.Sprintf("hoom agent --role %s --task %s%s %s", role, c.Slug, withSpec, pedidoHint)
}

// sesionSegura is a session id (or a provider name) that can go into a
// command as is.
var sesionSegura = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Doctor builds the board and lists the problems of every card, in board
// order, followed by the project's own. Read-only.
func Doctor(root, base, blockOn string, now time.Time) (DoctorReport, error) {
	g := newGatherer(root, base, blockOn, now)
	b, err := build(g)
	if err != nil {
		return DoctorReport{}, err
	}
	r := DoctorReport{Problems: []Problem{}}
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			r.Problems = append(r.Problems, c.Doctor...)
		}
	}
	// a directory it cannot read is an error, never "no problems": the
	// doctor would vouch for what it did not look at (finding e5d1e4)
	items, err := itemFiles(root)
	if err != nil {
		return DoctorReport{}, err
	}
	worktrees, err := taskWorktrees(root)
	if err != nil {
		return DoctorReport{}, err
	}
	r.Problems = append(r.Problems, orphanEnvelopes(g, items, worktrees)...)
	r.Problems = append(r.Problems, unverifiedWorktrees(g, items, worktrees)...)
	r.Problems = append(r.Problems, evidenceWithoutItem(g, items, worktrees)...)
	// the same for every other source it read: envelopes, runs, verdicts,
	// findings, reviews (finding d0d331)
	g.recogerFallas()
	if len(g.fallas) > 0 {
		return DoctorReport{}, fmt.Errorf("el doctor no pudo leer toda la evidencia y no certifica nada: %s", strings.Join(g.fallas, "; "))
	}
	return r, nil
}

// itemFiles is every item file of root, valid or not: an invalid one is
// still an item (the board warns about it). No items directory is no items;
// one it cannot read is an error.
func itemFiles(root string) (map[string]bool, error) {
	out := map[string]bool{}
	entries, err := os.ReadDir(item.Dir(root))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no pude leer %s: %w", item.Dir(root), err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			out[strings.TrimSuffix(e.Name(), ".yaml")] = true
		}
	}
	return out, nil
}

// taskWorktrees lists the task worktrees of root by slug, sorted. No
// worktrees directory is no worktrees; one it cannot read is an error.
func taskWorktrees(root string) ([]string, error) {
	var out []string
	dir := filepath.Join(root, ".hoom", "worktrees")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no pude leer %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// a name that is not a slug is not a task's, and never goes into
		// the command the doctor proposes (finding 7913f8)
		if !item.ValidSlug(e.Name()) {
			continue
		}
		_, err := runcmd.TaskDir(root, e.Name())
		if errors.Is(err, gitx.ErrSinComprobar) || errors.Is(err, runcmd.ErrOtraRama) {
			// git that cannot answer, or a worktree off its branch, is not
			// "no task": the doctor fails with the reason instead of
			// leaving it out of its report (findings 6aeb17, 966474)
			return nil, err
		}
		if err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// place is one tree the doctor reads telemetry or evidence from.
type place struct{ dir, rel string }

func places(root string, worktrees []string) []place {
	ps := []place{{dir: root, rel: "."}}
	for _, s := range worktrees {
		if dir, err := runcmd.TaskDir(root, s); err == nil {
			ps = append(ps, place{dir: dir, rel: ".hoom/worktrees/" + s})
		}
	}
	return ps
}

// orphanEnvelopes: open envelopes whose owner is gone and that belong to no
// card (no task, or a task without item), unless a later envelope of the
// same task relieved them.
func orphanEnvelopes(g *gatherer, items map[string]bool, worktrees []string) []Problem {
	type found struct {
		rec envelope.Record
		ref string
	}
	var all []found
	seen := map[string]bool{}
	metas := map[string]runcmd.Meta{}
	ps := places(g.root, worktrees)
	for _, p := range ps {
		for _, m := range g.metasOf(p.dir) {
			if _, ok := metas[m.ID]; !ok {
				metas[m.ID] = m
			}
		}
	}
	for _, p := range ps {
		for _, rec := range g.envelopesOf(p.dir) {
			if seen[rec.ID] {
				continue
			}
			seen[rec.ID] = true
			all = append(all, found{rec: rec, ref: path.Join(p.rel, ".hoom", envelope.DirName, rec.ID+".json")})
		}
	}
	out := []Problem{}
	for _, f := range all {
		rec := f.rec
		if rec.Done() || (rec.Task != "" && items[rec.Task]) || g.envelopeAlive(rec, metas) {
			continue
		}
		relieved := false
		for _, o := range all {
			if rec.Task != "" && o.rec.ID != rec.ID && o.rec.Task == rec.Task && o.rec.StartedAt.After(rec.UpdatedAt) {
				relieved = true
			}
		}
		if relieved {
			continue
		}
		role := rec.Role
		if role == "" {
			role = "rol"
		}
		out = append(out, Problem{ID: ProbSobreHuerfano, Slug: rec.Task,
			What:   fmt.Sprintf("el sobre %s del %s quedo abierto en el paso %s y no es de ninguna tarjeta", rec.ID, role, rec.Stage),
			Action: "es telemetria local: si ya no te sirve, borralo con rm " + f.ref, Ref: f.ref})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

// unverifiedWorktrees: task worktrees without item whose changes no complete
// verdict of the tree certifies.
func unverifiedWorktrees(g *gatherer, items map[string]bool, worktrees []string) []Problem {
	out := []Problem{}
	for _, s := range worktrees {
		if items[s] {
			continue
		}
		dir, err := runcmd.TaskDir(g.root, s)
		if err != nil {
			continue
		}
		t := g.tree(dir)
		fp := t.currentFingerprint(g.base)
		if len(t.candidate(g.base)) == 0 || t.certifies(fp) {
			continue
		}
		rel := ".hoom/worktrees/" + s
		action := "cd " + rel + " && hoom verify"
		spec := ".hoom/specs/" + s + ".md"
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(spec))); err == nil && !st.IsDir() {
			action += " --spec " + spec
		}
		out = append(out, Problem{ID: ProbSinVeredicto, Slug: s,
			What:   fmt.Sprintf("el espacio de trabajo %s tiene cambios que ningun veredicto certifica (huella %s)", rel, fp),
			Action: action, Ref: rel})
	}
	return out
}

// evidenceWithoutItem: tasks with verdicts or findings and no item file, one
// problem per task. A verdict or finding seen in several trees counts once.
func evidenceWithoutItem(g *gatherer, items map[string]bool, worktrees []string) []Problem {
	verdicts, findings := map[string]map[string]bool{}, map[string]map[string]bool{}
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
	tasks := map[string]bool{}
	for t := range verdicts {
		tasks[t] = true
	}
	for t := range findings {
		tasks[t] = true
	}
	var names []string
	for t := range tasks {
		if !items[t] {
			names = append(names, t)
		}
	}
	sort.Strings(names)
	out := []Problem{}
	for _, t := range names {
		var parts []string
		if n := len(verdicts[t]); n > 0 {
			parts = append(parts, count(n, "veredicto", "veredictos"))
		}
		if n := len(findings[t]); n > 0 {
			parts = append(parts, count(n, "hallazgo", "hallazgos"))
		}
		out = append(out, Problem{ID: ProbEvidenciaSinItem, Slug: t,
			What:   fmt.Sprintf("hay %s de la tarea %s y ningun item la representa", strings.Join(parts, " y "), t),
			Action: `hoom item add "<titulo>" --slug ` + t, Ref: ".hoom/specs/" + t + ".md"})
	}
	return out
}

// specTask is the task of a verdict: the slug of its spec when the spec is
// .hoom/specs/<slug>.md; "" otherwise.
func specTask(v *verdict.Verdict, dir string) string {
	sp := strings.TrimSpace(v.Spec)
	if sp == "" {
		return ""
	}
	if filepath.IsAbs(sp) {
		rel, err := filepath.Rel(dir, sp)
		if err != nil {
			return ""
		}
		sp = rel
	}
	sp = filepath.ToSlash(filepath.Clean(sp))
	if !strings.HasPrefix(sp, ".hoom/specs/") || !strings.HasSuffix(sp, ".md") {
		return ""
	}
	t := strings.TrimSuffix(strings.TrimPrefix(sp, ".hoom/specs/"), ".md")
	if !item.ValidSlug(t) {
		return ""
	}
	return t
}

// ParseDoctorArgs is pure and strict, like ParseArgs.
func ParseDoctorArgs(args []string) (DoctorOptions, error) {
	fs := flag.NewFlagSet("board doctor", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emitir los problemas como JSON en stdout")
	if err := cliargs.Strict(fs, args, "board doctor", DoctorUsageText); err != nil {
		return DoctorOptions{}, err
	}
	return DoctorOptions{JSON: *asJSON}, nil
}

// RenderDoctor prints the report as text.
func RenderDoctor(w io.Writer, r DoctorReport) {
	switch n := len(r.Problems); n {
	case 0:
		fmt.Fprintln(w, "hoom board doctor: sin problemas de coherencia")
		return
	default:
		fmt.Fprintf(w, "hoom board doctor: %s\n", count(n, "problema", "problemas"))
	}
	for _, p := range r.Problems {
		slug := p.Slug
		if slug == "" {
			slug = "-"
		}
		fmt.Fprintf(w, "  %s  %s\n    %s\n    Accion: %s\n", p.ID, slug, p.What, p.Action)
	}
}

// DoctorJSONBytes is the report as `hoom board doctor --json` prints it.
func DoctorJSONBytes(r DoctorReport) ([]byte, error) {
	if r.Problems == nil {
		r.Problems = []Problem{}
	}
	return json.MarshalIndent(r, "", "  ")
}
