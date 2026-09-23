// Package envelope keeps the durable record of a `hoom agent` run. The
// envelope is what actually runs a role — five steps, six when the role runs
// blind — and until now it left no trace anybody could look at while it
// worked: its Result was printed and gone. From another terminal `hoom status`
// saw an anonymous run, and the Studio did not even see that.
//
// The record answers, at any moment and from any process, the question the
// terminal output can only answer to whoever is watching it: WHO is running,
// with WHICH provider, over WHICH spec, at WHICH step, and how it ended.
//
// It is telemetry, not evidence: it lives in .hoom/envelopes/, outside Git,
// outside the change candidate and outside the fingerprint — same contract as
// .hoom/runs/. Writing it can never break the envelope it describes, so every
// write is best-effort by design.
package envelope

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// Status values of a record. Same vocabulary the envelope already prints.
const (
	StatusRunning        = "en curso"
	StatusDeliverable    = "entregable"
	StatusNotDeliverable = "no-entregable"
	// StatusNoDelivery: a role that writes finished its run without changing
	// anything but the evidence hoom itself generates. There is no new tree
	// to certify, so there is no verdict — and that is not the same as a red
	// one.
	StatusNoDelivery = "sin-entrega"
)

// DirName is where the records live, under .hoom/.
const DirName = "envelopes"

// Record is one envelope invocation, as far as it got.
type Record struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Provider string `json:"provider"`
	Task     string `json:"task,omitempty"`
	Dir      string `json:"dir"`
	Spec     string `json:"spec,omitempty"`
	Approval string `json:"approval,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	// Isolated says the role ran blind, and from which commit: the guarantee
	// the Spec D built is also part of the envelope's identity.
	Isolated     bool             `json:"isolated,omitempty"`
	IsolatedFrom string           `json:"isolated_from,omitempty"`
	VerdictID    string           `json:"verdict_id,omitempty"`
	Verdict      string           `json:"verdict,omitempty"`
	Usage        *providers.Usage `json:"usage,omitempty"`
	// Stage is the step the envelope is in or stopped at, with the same names
	// it prints: spec | aislar | run | scope | verify | check | ok.
	Stage     string    `json:"stage"`
	Step      int       `json:"step"`
	Steps     int       `json:"steps"`
	Status    string    `json:"status"`
	ExitCode  int       `json:"exit_code"`
	Note      string    `json:"note,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// PID is the process that owns the envelope (brecha 4 of the cabin RFC):
	// with it, "interrupted" is a fact the moment that process dies instead
	// of an inference from a silent heartbeat. 0 in records of older hoom.
	PID int `json:"pid,omitempty"`
	// Pilot says the cabin's belt (auto: hasta-humano) launched this
	// envelope, not a person.
	Pilot   bool      `json:"piloto,omitempty"`
	EndedAt time.Time `json:"ended_at,omitempty"`
}

// Done reports a record that reached an end. Anything else is still "en
// curso" — which does NOT mean alive: whoever reads it decides how much to
// trust an old UpdatedAt, and hoom never closes what it did not see close.
func (r Record) Done() bool { return r.Status != "" && r.Status != StatusRunning }

// NewID mints a record id with the same shape as a run's: sortable by name
// and unique per invocation.
func NewID() string {
	raw := make([]byte, 3)
	rand.Read(raw)
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(raw)
}

func dir(root string) string { return filepath.Join(root, ".hoom", DirName) }

// Write records the envelope's state. Best-effort by contract: a record that
// cannot be written never breaks the envelope it describes, so nothing here
// returns an error.
func Write(root string, rec Record) {
	if strings.TrimSpace(rec.ID) == "" {
		return
	}
	rec.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(dir(root), 0o755) != nil {
		return
	}
	// La regla queda escrita (CA-202) sin reescribir un archivo completo:
	// registrar telemetria no mueve el candidato (CA-203).
	hoomfs.EnsureIgnored(root, DirName)
	// Atomico: status y el Studio leen este archivo MIENTRAS se escribe, y un
	// sobre a medias leido como vacio desapareceria de la lista.
	hoomfs.AtomicWrite(filepath.Join(dir(root), rec.ID+".json"), append(raw, '\n'), 0o644)
}

// List returns the project's envelope records, newest first. A file that is
// unreadable, of another shape or without an id is skipped, and a project
// that never ran an envelope gets an empty list instead of an error: broken
// telemetry never breaks a command.
func List(root string) []Record {
	entries, err := os.ReadDir(dir(root))
	if err != nil {
		return nil
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir(root), e.Name()))
		if err != nil {
			continue
		}
		var rec Record
		if json.Unmarshal(raw, &rec) != nil || strings.TrimSpace(rec.ID) == "" {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}
