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
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
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
	return filepath.ToSlash(specPath)
}

// gitUser identifies the approver from the project's git config; approvals
// are human acts, so the identity recorded is the repo's configured author.
func gitUser(root string) string {
	get := func(key string) string {
		cmd := exec.Command("git", "config", key)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	name, email := get("user.name"), get("user.email")
	switch {
	case name != "" && email != "":
		return name + " <" + email + ">"
	case name != "":
		return name
	case email != "":
		return email
	}
	return "desconocido"
}

// Approve records the approval of the spec's CURRENT content. Re-approving
// identical content is an informed no-op: already=true, no duplicate record.
func Approve(root, specPath string) (rec Record, already bool, err error) {
	sum, err := contentHash(specPath)
	if err != nil {
		return Record{}, false, err
	}
	target := filepath.Join(dir(root), specSlug(specPath)+"_"+sum[:8]+".json")
	if raw, rerr := os.ReadFile(target); rerr == nil {
		if jerr := json.Unmarshal(raw, &rec); jerr == nil && rec.SHA256 == sum {
			return rec, true, nil
		}
	}
	rec = Record{
		Spec:       relSpec(root, specPath),
		SHA256:     sum,
		ApprovedBy: gitUser(root),
		ApprovedAt: time.Now().UTC(),
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return Record{}, false, err
	}
	if err := os.MkdirAll(dir(root), 0o755); err != nil {
		return Record{}, false, err
	}
	if err := os.WriteFile(target, raw, 0o644); err != nil {
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
		if json.Unmarshal(raw, &rec) != nil || rec.Spec != spec {
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
