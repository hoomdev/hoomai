// Package statuscmd composes the arbiter's window: one read-only snapshot of
// everything the harness can PROVE right now — check state, last verdict,
// the verify in progress (from live events), active runs with their role,
// tasks and open findings. One brain, three skins: text, --json and --watch.
// It shows what it knows, labels what it cannot know, and never invents:
// an unfinished silent run is a "posible huerfano", a run without provider
// delegation data reads "sin delegacion visible", jamas un rol adivinado.
package statuscmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	cReset  = "\033[0m"
	cGreen  = "\033[32m"
	cRed    = "\033[31m"
	cYellow = "\033[33m"
	cGray   = "\033[90m"
	cBold   = "\033[1m"
)

// RunView is what the disk can prove about one hoom run.
type RunView struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider,omitempty"` // parsed from the start event; empty = unknown
	Role      string    `json:"role,omitempty"`     // last delegated agent; empty = no delegation visible
	Active    bool      `json:"active"`
	StartedAt time.Time `json:"started_at"`
	LastEvent time.Time `json:"last_event"`
	Events    int       `json:"events"`
}

// LastVerdict summarizes the newest verdict on disk.
type LastVerdict struct {
	ID        string          `json:"id"`
	Verdict   string          `json:"verdict"`
	Partial   bool            `json:"partial"`
	CreatedAt time.Time       `json:"created_at"`
	Summary   verdict.Summary `json:"summary"`
}

// FindingsSummary counts open findings.
type FindingsSummary struct {
	Open     int `json:"open"`
	OpenHigh int `json:"open_high"`
}

// Snapshot is the full status: one read, three renders.
type Snapshot struct {
	Root       string             `json:"root"`
	Now        time.Time          `json:"now"`
	Check      checkcmd.Result    `json:"check"`
	Last       *LastVerdict       `json:"last_verdict,omitempty"`
	Live       *live.State        `json:"live,omitempty"` // in-progress (or orphaned) verify
	LiveOrphan bool               `json:"live_orphan,omitempty"`
	Runs       []RunView          `json:"runs"` // active runs only
	Tasks      []taskcmd.TaskInfo `json:"tasks"`
	Findings   FindingsSummary    `json:"findings"`
}

// Build composes the snapshot. Strictly read-only: nothing under .hoom is
// created or modified by looking at it.
func Build(root, base string) (*Snapshot, error) {
	now := time.Now().UTC()
	s := &Snapshot{Root: root, Now: now, Runs: []RunView{}, Tasks: []taskcmd.TaskInfo{}}

	check, err := checkcmd.Run(root, base)
	if err != nil {
		return nil, err
	}
	s.Check = check

	all, err := verdict.LoadAll(root)
	if err != nil {
		return nil, err
	}
	if len(all) > 0 {
		v := all[len(all)-1]
		s.Last = &LastVerdict{ID: v.ID, Verdict: v.Verdict, Partial: v.IsPartial(),
			CreatedAt: v.CreatedAt, Summary: v.Summary}
	}

	st, err := live.Read(root)
	if err == nil && st != nil && !st.Done {
		s.Live = st
		s.LiveOrphan = st.PossibleOrphan(now)
	}

	s.Runs = activeRuns(root)

	tasks, err := taskcmd.Snapshot(root, base)
	if err == nil {
		s.Tasks = tasks
	}

	items, _, err := finding.List(root, base, true)
	if err == nil {
		s.Findings.Open = len(items)
		for _, it := range items {
			if it.Severity == "high" {
				s.Findings.OpenHigh++
			}
		}
	}
	return s, nil
}

var runStartRe = regexp.MustCompile(`^run \S+: (\S+) en `)

// activeRuns scans .hoom/runs/*.jsonl and keeps what the events prove: a run
// whose last event is not terminal is active; its role is the last delegated
// agent the provider reported — absent data stays absent.
func activeRuns(root string) []RunView {
	dir := filepath.Join(root, ".hoom", "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []RunView{}
	}
	out := []RunView{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		rv := RunView{ID: strings.TrimSuffix(e.Name(), ".jsonl")}
		var last providers.Event
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var ev providers.Event
			if line == "" || json.Unmarshal([]byte(line), &ev) != nil {
				continue
			}
			rv.Events++
			if rv.Events == 1 {
				rv.StartedAt = ev.TS
				if m := runStartRe.FindStringSubmatch(ev.Detail); m != nil {
					rv.Provider = m[1]
				}
			}
			if ev.Agent != "" {
				rv.Role = ev.Agent
			}
			last = ev
		}
		if rv.Events == 0 {
			continue
		}
		rv.LastEvent = last.TS
		rv.Active = last.Kind != "end" && last.Kind != "error"
		if rv.Active {
			out = append(out, rv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// JSONBytes renders the snapshot for agents and the Studio: same data,
// machine skin.
func JSONBytes(s *Snapshot) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// paint wraps s in an ANSI color only when color is on.
func paint(color bool, code, s string) string {
	if !color {
		return s
	}
	return code + s + cReset
}

func ago(now, t time.Time) string {
	d := now.Sub(t).Round(time.Second)
	if d < 0 {
		d = 0
	}
	return d.String()
}

// Render prints the snapshot. With color=false the output carries zero ANSI
// escapes (CI logs, redirections).
func Render(w io.Writer, s *Snapshot, color bool) {
	fmt.Fprintf(w, "%s\n", paint(color, cBold, "hoomAI - status de "+filepath.Base(s.Root)))
	fmt.Fprintln(w, strings.Repeat("-", 72))

	// check
	if s.Check.OK {
		fmt.Fprintf(w, " check:     %s huella %s\n", paint(color, cGreen, "VERDE"), s.Check.FingerprintNow)
	} else {
		fmt.Fprintf(w, " check:     %s %s - %s\n", paint(color, cRed, "ROJO"), s.Check.Reason, s.Check.Action)
	}

	// ultimo veredicto
	if s.Last == nil {
		fmt.Fprintf(w, " veredicto: %s\n", paint(color, cGray, "ninguno todavia (ejecuta 'hoom verify')"))
	} else {
		badge := paint(color, cGreen, "VERDE")
		if s.Last.Verdict != "green" {
			badge = paint(color, cRed, "ROJO")
		}
		partial := ""
		if s.Last.Partial {
			partial = " " + paint(color, cYellow, "[PARCIAL]")
		}
		fmt.Fprintf(w, " veredicto: %s %s%s  pass:%d fail:%d err:%d aus:%d skip:%d\n",
			badge, s.Last.ID, partial, s.Last.Summary.Pass, s.Last.Summary.Fail,
			s.Last.Summary.Error, s.Last.Summary.Absent, s.Last.Summary.Skipped)
	}

	// verify en curso
	if s.Live == nil {
		fmt.Fprintf(w, " verify:    %s\n", paint(color, cGray, "sin corrida en curso"))
	} else {
		done := 0
		for _, g := range s.Live.GateViews {
			if g.Status != "" {
				done++
			}
		}
		fmt.Fprintf(w, " verify:    corriendo (%d/%d gates)\n", done, s.Live.Gates)
		if s.LiveOrphan {
			fmt.Fprintf(w, "            %s\n", paint(color, cYellow,
				fmt.Sprintf("posible huerfano: sin actividad hace %s (arranco y no termino)", ago(s.Now, s.Live.LastEvent))))
		}
		for _, g := range s.Live.GateViews {
			scope := ""
			if g.Scope == "diff" {
				scope = " [diff]"
			}
			if g.Status == "" {
				fmt.Fprintf(w, "   %s %-14s %s\n", paint(color, cYellow, "..."), g.Name,
					paint(color, cYellow, "corriendo "+ago(s.Now, g.StartedAt))+scope)
			} else {
				badge := map[string]string{
					verdict.StatusPass:    paint(color, cGreen, "OK "),
					verdict.StatusFail:    paint(color, cRed, "FAI"),
					verdict.StatusError:   paint(color, cRed, "ERR"),
					verdict.StatusAbsent:  paint(color, cYellow, "AUS"),
					verdict.StatusSkipped: paint(color, cGray, "SKP"),
				}[g.Status]
				fmt.Fprintf(w, "   %s %-14s %dms%s\n", badge, g.Name, g.DurationMS, scope)
			}
		}
	}

	// runs activos con rol
	if len(s.Runs) == 0 {
		fmt.Fprintf(w, " runs:      %s\n", paint(color, cGray, "sin runs activos"))
	} else {
		fmt.Fprintf(w, " runs:      %d activo(s)\n", len(s.Runs))
		for _, r := range s.Runs {
			provider := r.Provider
			if provider == "" {
				provider = "?"
			}
			role := paint(color, cGray, "sin delegacion visible")
			if r.Role != "" {
				role = paint(color, cBold, "rol: "+r.Role)
			}
			fmt.Fprintf(w, "   %s %s %s · ultimo evento hace %s\n", r.ID, provider, role, ago(s.Now, r.LastEvent))
		}
	}

	// tareas
	if len(s.Tasks) == 0 {
		fmt.Fprintf(w, " tareas:    %s\n", paint(color, cGray, "ninguna activa"))
	} else {
		fmt.Fprintf(w, " tareas:    %d activa(s)\n", len(s.Tasks))
		for _, t := range s.Tasks {
			fmt.Fprintf(w, "   %-24s %s\n", t.Slug, t.State)
		}
	}

	// hallazgos
	if s.Findings.Open == 0 {
		fmt.Fprintf(w, " hallazgos: %s\n", paint(color, cGray, "sin abiertos"))
	} else {
		line := fmt.Sprintf("%d abierto(s)", s.Findings.Open)
		if s.Findings.OpenHigh > 0 {
			line += fmt.Sprintf(", %d high", s.Findings.OpenHigh)
		}
		fmt.Fprintf(w, " hallazgos: %s\n", paint(color, cYellow, line))
	}
	fmt.Fprintln(w, strings.Repeat("-", 72))
}

// Options mirror the status verb's flags plus the terminal reality.
type Options struct {
	JSON     bool
	Watch    bool
	TTY      bool
	Interval time.Duration // watch refresh (0 = 1s)
}

// Run executes the verb: one-shot text, --json, or --watch (TTY refresh).
// Without a TTY, --watch prints the snapshot ONCE with zero ANSI and exits 0.
func Run(root, base string, out io.Writer, opt Options) error {
	if opt.Interval == 0 {
		opt.Interval = time.Second
	}
	s, err := Build(root, base)
	if err != nil {
		return err
	}
	if opt.JSON {
		raw, err := JSONBytes(s)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(raw))
		return nil
	}
	if opt.Watch && !opt.TTY {
		Render(out, s, false)
		fmt.Fprintln(out, "hoom: --watch necesita una terminal (TTY); snapshot unico emitido")
		return nil
	}
	if !opt.Watch {
		Render(out, s, opt.TTY)
		return nil
	}
	for {
		fmt.Fprint(out, "\033[H\033[2J")
		Render(out, s, true)
		fmt.Fprintf(out, " %s\n", cGray+"refresco cada "+opt.Interval.String()+" · ctrl+c para salir"+cReset)
		time.Sleep(opt.Interval)
		if s, err = Build(root, base); err != nil {
			return err
		}
	}
}
