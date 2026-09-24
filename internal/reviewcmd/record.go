package reviewcmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RecordsDir is where the review records live, under .hoom/.
const RecordsDir = "reviews"

// Record is the trace a review leaves: "I reviewed this", never "this is
// fine" — that is verify's job. Append-only, versioned in Git like findings,
// outside the change candidate. It is what lets a CLEAN review exist on disk:
// a review that found nothing used to leave no trace at all.
type Record struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	Task        string    `json:"task"`
	Spec        string    `json:"spec"`
	Fingerprint string    `json:"fingerprint"` // change fingerprint of the reviewed tree
	VerdictID   string    `json:"verdict_id"`  // latest complete verdict when the review started
	Verdict     string    `json:"verdict"`     // its color
	Lenses      []string  `json:"lenses"`
	Provider    string    `json:"provider"` // the reviewer's
	Writer      string    `json:"writer"`   // provider of the last run that wrote
	Cross       string    `json:"cross"`    // cruzada | no-cruzada | cruzada-declarada | desconocida
	// WritersDeclared: the providers of the item's sessions (see Result).
	WritersDeclared []string `json:"writers_declared"`
	Findings        []string `json:"findings"` // ids hoom saw appear
	// Notes: what hoom saw go wrong with the evidence, e.g. findings that
	// do not carry the review's task.
	Notes []string `json:"notes,omitempty"`
}

func recordsDir(dir string) string { return filepath.Join(dir, ".hoom", RecordsDir) }

// WriteRecord persists r under dir/.hoom/reviews/<id>.json, stamping ID and
// CreatedAt when empty. It never overwrites an existing record.
func WriteRecord(dir string, r Record) (Record, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.ID == "" {
		raw := make([]byte, 3)
		rand.Read(raw)
		r.ID = r.CreatedAt.UTC().Format("20060102T150405") + "_" + hex.EncodeToString(raw)
	}
	if !validRecordID(r.ID) {
		return Record{}, fmt.Errorf("id de registro invalido %q", r.ID)
	}
	if r.Lenses == nil {
		r.Lenses = []string{}
	}
	if r.Findings == nil {
		r.Findings = []string{}
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(recordsDir(dir), 0o755); err != nil {
		return Record{}, err
	}
	// append-only: O_EXCL, a record is never rewritten
	f, err := os.OpenFile(filepath.Join(recordsDir(dir), r.ID+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Record{}, err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		return Record{}, err
	}
	return r, f.Close()
}

// Records reads every review record of dir, oldest first, plus one warning
// per unreadable file (never fatal). The file name is the address: a record
// whose id does not match its name is not trusted.
func Records(dir string) ([]Record, []string) {
	out, warnings := []Record{}, []string{}
	entries, err := os.ReadDir(recordsDir(dir))
	if err != nil {
		return out, warnings
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(recordsDir(dir), e.Name()))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("registro de review ilegible %s: %v", e.Name(), err))
			continue
		}
		var r Record
		if err := json.Unmarshal(raw, &r); err != nil || r.ID != strings.TrimSuffix(e.Name(), ".json") {
			warnings = append(warnings, fmt.Sprintf("registro de review ilegible %s", e.Name()))
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, warnings
}

func validRecordID(id string) bool {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return false
	}
	return true
}
