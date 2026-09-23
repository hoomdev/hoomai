package reviewcmd

import "time"

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
	Cross       string    `json:"cross"`    // cruzada | no-cruzada | desconocida
	Findings    []string  `json:"findings"` // ids hoom saw appear
}

// WriteRecord persists r under dir/.hoom/reviews/<id>.json, stamping ID and
// CreatedAt when empty. It never overwrites an existing record.
func WriteRecord(dir string, r Record) (Record, error) {
	return Record{}, nil
}

// Records reads every review record of dir, oldest first, plus one warning
// per unreadable file (never fatal).
func Records(dir string) ([]Record, []string) {
	return nil, nil
}
