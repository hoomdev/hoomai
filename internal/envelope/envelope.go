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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
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

// idRe is the shape of an envelope id: one file name, no separators, no
// dots that climb, nothing a shell would read.
var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// NewID mints a record id with the same shape as a run's: sortable by name
// and unique per invocation.
func NewID() string {
	raw := make([]byte, 3)
	rand.Read(raw)
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(raw)
}

func dir(root string) string { return filepath.Join(root, ".hoom", DirName) }

// Write records the envelope's state. Best-effort by contract (CA-202): a
// record that cannot be written never breaks the envelope it describes, so
// callers may ignore the error. The one that cannot is the Started handshake
// (CA-335): it promises the first record is on disk.
func Write(root string, rec Record) error {
	if strings.TrimSpace(rec.ID) == "" {
		return nil
	}
	rec.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir(root), 0o755); err != nil {
		return err
	}
	// La regla queda escrita (CA-202) sin reescribir un archivo completo:
	// registrar telemetria no mueve el candidato (CA-203).
	hoomfs.EnsureIgnored(root, DirName)
	// Atomico: status y el Studio leen este archivo MIENTRAS se escribe, y un
	// sobre a medias leido como vacio desapareceria de la lista.
	return hoomfs.AtomicWrite(filepath.Join(dir(root), rec.ID+".json"), append(raw, '\n'), 0o644)
}

// List returns the project's envelope records, newest first. A file that is
// unreadable, of another shape or without an id is skipped, and a project
// that never ran an envelope gets an empty list instead of an error: broken
// telemetry never breaks a command.
func List(root string) []Record {
	out, _ := ListErr(root)
	return out
}

// maxRecord caps one record file: hoom writes a few hundred bytes.
const maxRecord = 1 << 20

// ListErr is List for whoever must not take "unreadable" for "none" (the
// doctor, finding d0d331): an envelopes directory that exists and cannot be
// listed, or that is a symlink or not a directory, is an error; one that
// does not exist yet is no envelopes. The directory and every record are
// read by descriptors, never following a symlink: a record linked to one of
// another project would publish that project's telemetry as this one's
// (finding 3826b3). A record that cannot be read is still skipped.
func ListErr(root string) ([]Record, error) {
	r, err := hoomfs.AbrirRuta(root, ".hoom", DirName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir .hoom/%s: %v", DirName, err)
	}
	defer r.Close()
	entries, err := hoomfs.ListarEn(r)
	if err != nil {
		return nil, fmt.Errorf("no se pudo listar .hoom/%s: %v", DirName, err)
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := hoomfs.LeerRegularEn(r, e.Name(), maxRecord)
		if err != nil {
			continue
		}
		var rec Record
		if json.Unmarshal(raw, &rec) != nil || strings.TrimSpace(rec.ID) == "" {
			continue
		}
		// the file name is the address and the content only describes it: a
		// record whose id is not its file's name, or is not an id at all, is
		// not hoom's and never reaches a command the doctor proposes
		// (finding f34bc3)
		if rec.ID+".json" != e.Name() || !idRe.MatchString(rec.ID) {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

// esperasCierre is how long a close that did not reach the disk waits before
// each new try; past the last one it keeps trying at that pace.
var esperasCierre = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second}

// Cerrar writes the closing record of an envelope. Its owner's PID keeps an
// open record alive (CA-333): a close that never reaches the disk would leave
// the envelope "en curso" for as long as that process lives, which for one
// the Studio launched is the whole life of the server (finding de4843). So a
// close that fails is not dropped: it is tried again in the background, at
// the pace of esperasCierre, until it lands or the process ends — and then
// the dead PID is what says the envelope stopped. Cerrar returns the first
// try's error; the record stays best-effort for the envelope (CA-202).
func Cerrar(root string, rec Record) error {
	err := Write(root, rec)
	if err != nil && strings.TrimSpace(rec.ID) != "" {
		go reintentarCierre(root, rec)
	}
	return err
}

func reintentarCierre(root string, rec Record) {
	path := filepath.Join(dir(root), rec.ID+".json")
	for i := 0; ; i++ {
		espera := esperasCierre[len(esperasCierre)-1]
		if i < len(esperasCierre) {
			espera = esperasCierre[i]
		}
		time.Sleep(espera)
		// a record that is gone has nothing left to close, and a retry never
		// creates the directories back: a project removed under it stays
		// removed
		if _, err := os.Lstat(path); err != nil {
			return
		}
		if Write(root, rec) == nil {
			return
		}
	}
}
