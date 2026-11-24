// Package approval records human spec approvals bound to the SHA-256 of the
// spec's CONTENT — the fingerprint philosophy applied to the approval gate:
// what gets approved is an exact text, not a filename. Records are
// append-only JSON files under .hoom/approvals/ (one per approved content,
// zero merge conflicts by construction), versioned in Git like verdicts.
// Editing an approved spec invalidates the approval automatically: the
// current hash no longer matches any record.
package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// Status values for a spec's approval state.
const (
	StatusApproved    = "aprobado"
	StatusNotApproved = "no-aprobado"
	StatusInvalidated = "invalidado" // approvals exist, none match the current content
)

// Record is one immutable approval.
type Record struct {
	Spec       string    `json:"spec"` // path as given, project-relative when possible
	SHA256     string    `json:"sha256"`
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
}

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

func dir(root string) string { return filepath.Join(root, ".hoom", "approvals") }

func contentHash(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashDe(raw), nil
}

// hashDe is the SHA-256 of a content already read (Aprobar signs the bytes
// it measured).
func hashDe(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func specSlug(specPath string) string {
	base := strings.TrimSuffix(filepath.Base(specPath), filepath.Ext(specPath))
	slug := slugRe.ReplaceAllString(base, "-")
	if slug == "" {
		slug = "spec"
	}
	return slug
}

func relSpec(root, specPath string) string {
	if rel, err := filepath.Rel(root, specPath); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return canon(specPath)
}

// canon is the spelling a record names its spec by, the same on both sides
// of Status (findings 7bd189 and aeb764): `./x.md` is x.md and
// `/repo/../otro/x.md` is /otro/x.md.
func canon(s string) string { return filepath.ToSlash(filepath.Clean(filepath.FromSlash(s))) }

// Approve records the approval of the spec's CURRENT content. Re-approving
// identical content is an informed no-op: already=true, no duplicate record.
func Approve(root, specPath string) (rec Record, already bool, err error) {
	sum, err := contentHash(specPath)
	if err != nil {
		return Record{}, false, err
	}
	return approveSum(root, specPath, sum)
}

// approveSum is Approve of the content whose hash is sum.
func approveSum(root, specPath, sum string) (rec Record, already bool, err error) {
	// already is exactly what Status says: aprobado, by this spec's record
	// (finding 6dcf5b: not another spec's with the same name and content)
	if st, r, serr := statusSum(root, specPath, sum); serr == nil && st == StatusApproved {
		return *r, true, nil
	}
	name := specSlug(specPath) + "_" + sum[:8]
	target := filepath.Join(dir(root), name+".json")
	if _, serr := os.Stat(target); serr == nil {
		// the name is another spec's record: this one gets its own
		h := sha256.Sum256([]byte(relSpec(root, specPath)))
		target = filepath.Join(dir(root), name+"_"+hex.EncodeToString(h[:4])+".json")
	}
	rec = Record{
		Spec:       relSpec(root, specPath),
		SHA256:     sum,
		ApprovedBy: gitx.Identity(root),
		ApprovedAt: time.Now().UTC(),
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return Record{}, false, err
	}
	if err := os.MkdirAll(dir(root), 0o755); err != nil {
		return Record{}, false, err
	}
	// a new file, never over another record
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Record{}, false, err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return Record{}, false, err
	}
	if err := f.Close(); err != nil {
		return Record{}, false, err
	}
	return rec, false, nil
}

// Status reports the spec's approval state against its CURRENT content:
// aprobado (a record matches the hash), invalidado (records exist for this
// spec but the content changed), or no-aprobado (no record at all).
func Status(root, specPath string) (string, *Record, error) {
	sum, err := contentHash(specPath)
	if err != nil {
		return "", nil, err
	}
	return statusSum(root, specPath, sum)
}

// statusSum is Status of the content whose hash is sum.
func statusSum(root, specPath, sum string) (string, *Record, error) {
	entries, err := os.ReadDir(dir(root))
	if os.IsNotExist(err) {
		return StatusNotApproved, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	spec := relSpec(root, specPath)
	var newest *Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir(root), e.Name()))
		if rerr != nil {
			continue
		}
		var rec Record
		// a record names its spec as it was given: `./.hoom/specs/x.md` is
		// .hoom/specs/x.md (finding 7bd189)
		if json.Unmarshal(raw, &rec) != nil || canon(rec.Spec) != spec {
			continue
		}
		if rec.SHA256 == sum {
			r := rec
			return StatusApproved, &r, nil
		}
		if newest == nil || rec.ApprovedAt.After(newest.ApprovedAt) {
			r := rec
			newest = &r
		}
	}
	if newest != nil {
		return StatusInvalidated, newest, nil
	}
	return StatusNotApproved, nil, nil
}

// Describe renders the status line the CLI prints for `hoom spec status`.
func Describe(state string, rec *Record) string {
	switch state {
	case StatusApproved:
		return fmt.Sprintf("APROBADO (vigente) por %s el %s - sha256 %s",
			rec.ApprovedBy, rec.ApprovedAt.Format("2006-01-02 15:04 UTC"), rec.SHA256[:8])
	case StatusInvalidated:
		return fmt.Sprintf("INVALIDADO - el contenido cambio despues de la aprobacion de %s (%s).\n  Accion: revisa el spec y re-aprueba con 'hoom spec approve'",
			rec.ApprovedBy, rec.ApprovedAt.Format("2006-01-02 15:04 UTC"))
	default:
		return "NO-APROBADO - sin registro de aprobacion.\n  Accion: aprueba con 'hoom spec approve <ruta>'"
	}
}
