// Package live is the ephemeral event stream of a verification run: what is
// happening RIGHT NOW, written by the hoom binary itself no matter which AI
// (or human) invoked it. Events cover only what has no artifact yet — the
// run in progress; verdicts, findings and tasks remain the only history.
// The file lives in .hoom/cache/ (gitignored, outside the fingerprint), is
// truncated on every verify start, and every write is best-effort: failing
// to narrate NEVER fails a gate.
package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	Schema   = "hoom.live/v1"
	FileName = "verify-live.jsonl"

	// Event kinds.
	KindVerifyStart = "verify_start"
	KindGateStart   = "gate_start"
	KindGateEnd     = "gate_end"
	KindVerifyEnd   = "verify_end"

	// OrphanAfter is the fixed inactivity threshold after which an unfinished
	// run is labeled a POSSIBLE orphan. A silent long gate can trip it; the
	// label says "posible" precisely because hoom cannot tell the difference.
	OrphanAfter = 15 * time.Minute
)

// Event is one line of the stream.
type Event struct {
	Schema     string    `json:"schema"`
	TS         time.Time `json:"ts"`
	Kind       string    `json:"kind"`
	Gates      int       `json:"gates,omitempty"`       // verify_start: declared total
	Spec       string    `json:"spec,omitempty"`        // verify_start
	Gate       string    `json:"gate,omitempty"`        // gate_start / gate_end
	Scope      string    `json:"scope,omitempty"`       // gate_start
	Status     string    `json:"status,omitempty"`      // gate_end
	DurationMS int64     `json:"duration_ms,omitempty"` // gate_end
	ExitCode   int       `json:"exit_code,omitempty"`   // gate_end
	Verdict    string    `json:"verdict,omitempty"`     // verify_end
	VerdictID  string    `json:"verdict_id,omitempty"`  // verify_end
	Partial    bool      `json:"partial,omitempty"`     // verify_end
}

func path(projectDir string) string {
	return filepath.Join(projectDir, ".hoom", "cache", FileName)
}

// Writer appends events best-effort. A Writer is always usable: when the
// cache is not writable it degrades to a silent no-op — the run simply is
// not narrated, never disturbed.
type Writer struct {
	f *os.File
}

// NewWriter truncates the live file and returns a writer (possibly no-op).
func NewWriter(projectDir string) *Writer {
	dir := filepath.Dir(path(projectDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &Writer{}
	}
	f, err := os.Create(path(projectDir)) // truncate: one run, one file
	if err != nil {
		return &Writer{}
	}
	return &Writer{f: f}
}

// Emit writes one event; errors are swallowed by design (best-effort).
func (w *Writer) Emit(ev Event) {
	if w == nil || w.f == nil {
		return
	}
	ev.Schema = Schema
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	w.f.Write(append(raw, '\n'))
}

// Close releases the file; safe on no-op writers.
func (w *Writer) Close() {
	if w != nil && w.f != nil {
		w.f.Close()
	}
}

// GateView is the live state of one gate as derived from events only.
type GateView struct {
	Name       string    `json:"name"`
	Scope      string    `json:"scope,omitempty"`
	Status     string    `json:"status,omitempty"` // empty = running
	DurationMS int64     `json:"duration_ms,omitempty"`
	StartedAt  time.Time `json:"started_at"`
}

// State is the parsed live file: the current (or last) run's progress.
type State struct {
	StartedAt time.Time  `json:"started_at"`
	Gates     int        `json:"gates"`
	Spec      string     `json:"spec,omitempty"`
	GateViews []GateView `json:"gate_views"`
	Done      bool       `json:"done"`
	Verdict   string     `json:"verdict,omitempty"`
	VerdictID string     `json:"verdict_id,omitempty"`
	Partial   bool       `json:"partial,omitempty"`
	LastEvent time.Time  `json:"last_event"`
}

// PossibleOrphan reports whether an unfinished run has gone silent past the
// threshold: it started, never ended, and shows no recent signals.
func (s *State) PossibleOrphan(now time.Time) bool {
	return s != nil && !s.Done && now.Sub(s.LastEvent) > OrphanAfter
}

// Read parses the live file. (nil, nil) means no run has been narrated.
// A corrupt or partially-written line is skipped, never fatal: the panel
// shows what it can prove and nothing else.
func Read(projectDir string) (*State, error) {
	raw, err := os.ReadFile(path(projectDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st *State
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var ev Event
		if line == "" || json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch ev.Kind {
		case KindVerifyStart:
			st = &State{StartedAt: ev.TS, Gates: ev.Gates, Spec: ev.Spec}
		case KindGateStart:
			if st == nil {
				continue
			}
			st.GateViews = append(st.GateViews, GateView{Name: ev.Gate, Scope: ev.Scope, StartedAt: ev.TS})
		case KindGateEnd:
			if st == nil {
				continue
			}
			for i := len(st.GateViews) - 1; i >= 0; i-- {
				if st.GateViews[i].Name == ev.Gate && st.GateViews[i].Status == "" {
					st.GateViews[i].Status = ev.Status
					st.GateViews[i].DurationMS = ev.DurationMS
					break
				}
			}
		case KindVerifyEnd:
			if st == nil {
				continue
			}
			st.Done = true
			st.Verdict = ev.Verdict
			st.VerdictID = ev.VerdictID
			st.Partial = ev.Partial
		}
		if st != nil && ev.TS.After(st.LastEvent) {
			st.LastEvent = ev.TS
		}
	}
	return st, nil
}
