// Package gitx derives verification scope from Git, never from agent
// narration: commit, branch, dirty state and the changed-file set that
// diff-scoped gates (e.g. mutation) operate on.
package gitx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Info is the Git snapshot bound to a verdict.
type Info struct {
	IsRepo       bool     `json:"is_repo"`
	Commit       string   `json:"commit,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Dirty        bool     `json:"dirty"`
	Base         string   `json:"base,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	// ChangeFingerprint freezes the exact change candidate that was verified:
	// a hash over HEAD plus the content of every changed file. If the working
	// tree drifts after a verdict, `hoom check` detects the mismatch. This is
	// RDD's candidate freezing without the cryptographic ceremony: the threat
	// model is drift, not forgery.
	ChangeFingerprint string `json:"change_fingerprint,omitempty"`
	// Insertions/Deletions measure the size of the change candidate vs base,
	// feeding deterministic rules like "more than 400 changed lines demands
	// all 4 review lenses" and the correction line budget.
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Snapshot collects Git state for dir. ChangedFiles is the union of
// (base...HEAD) committed changes plus working-tree modifications plus
// untracked files: everything the current change touches.
func Snapshot(dir, base string) Info {
	info := Info{Base: base}
	if _, err := run(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return info
	}
	info.IsRepo = true
	info.Commit, _ = run(dir, "rev-parse", "--short=12", "HEAD")
	info.Branch, _ = run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if st, _ := run(dir, "status", "--porcelain"); st != "" {
		info.Dirty = true
	}

	set := map[string]bool{}
	local := map[string]bool{} // worktree divergence vs HEAD: uncommitted + untracked + deleted
	if out, err := run(dir, "diff", "--name-only", "--diff-filter=ACMR", base+"...HEAD"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	if out, err := run(dir, "diff", "--name-only", "HEAD"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				set[f] = true
				local[f] = true
			}
		}
	}
	if out, err := run(dir, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				set[f] = true
				local[f] = true
			}
		}
	}
	for f := range set {
		// Generated artifacts are never part of the change candidate: the act
		// of recording a verdict must not alter the fingerprint it certifies.
		if excludedFromCandidate(f) {
			continue
		}
		info.ChangedFiles = append(info.ChangedFiles, f)
	}
	sort.Strings(info.ChangedFiles)
	info.ChangeFingerprint = fingerprint(dir, local)
	info.Insertions, info.Deletions = diffStats(dir, base, info.ChangedFiles)
	return info
}

// fingerprint (v2) identifies the change candidate by CONTENT, not by git
// position: the blob listing of HEAD's tree overlaid with the working-tree
// version of every locally-divergent file. Committing identical content —
// including the verdict files themselves, which are excluded — preserves the
// fingerprint; changing one byte anywhere breaks it. Deterministic, local,
// zero tokens.
func fingerprint(dir string, local map[string]bool) string {
	entries := map[string]string{}
	if out, err := run(dir, "ls-tree", "-r", "HEAD"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			tab := strings.IndexByte(line, '\t')
			if tab < 0 {
				continue
			}
			meta := strings.Fields(line[:tab])
			path := line[tab+1:]
			if len(meta) >= 3 && !excludedFromCandidate(path) {
				entries[path] = "g:" + meta[2]
			}
		}
	}
	// Hash local files with git's own blob hasher so a file has the SAME
	// identity whether committed or sitting in the working tree: committing
	// identical content preserves the fingerprint by construction.
	var localPaths []string
	for f := range local {
		if excludedFromCandidate(f) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			delete(entries, f) // deleted locally
			continue
		}
		localPaths = append(localPaths, f)
	}
	sort.Strings(localPaths)
	if len(localPaths) > 0 {
		cmd := exec.Command("git", append([]string{"hash-object", "--"}, localPaths...)...)
		cmd.Dir = dir
		if out, err := cmd.Output(); err == nil {
			hashes := strings.Split(strings.TrimSpace(string(out)), "\n")
			for i, f := range localPaths {
				if i < len(hashes) && hashes[i] != "" {
					entries[f] = "g:" + hashes[i]
				}
			}
		}
	}
	paths := make([]string, 0, len(entries))
	for p := range entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	io.WriteString(h, "hoom.fingerprint/v2\n")
	for _, p := range paths {
		io.WriteString(h, p+"\n"+entries[p]+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// excludedFromCandidate: generated artifacts are never part of the candidate;
// recording or committing a verdict must not alter the fingerprint it
// certifies.
func excludedFromCandidate(path string) bool {
	return strings.HasPrefix(path, ".hoom/verdicts/") || strings.HasPrefix(path, ".hoom/cache/") || strings.HasPrefix(path, ".hoom/worktrees/")
}

// diffStats measures the change candidate size: numstat of base vs working
// tree, plus line counts of untracked files. Binary files are skipped.
func diffStats(dir, base string, changed []string) (ins, del int) {
	out, err := run(dir, "diff", "--numstat", base)
	if err != nil {
		out, err = run(dir, "diff", "--numstat", "HEAD")
	}
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.Fields(line)
			if len(parts) < 3 || parts[0] == "-" {
				continue
			}
			var a, d int
			if _, e := fmt.Sscanf(parts[0], "%d", &a); e == nil {
				ins += a
			}
			if _, e := fmt.Sscanf(parts[1], "%d", &d); e == nil {
				del += d
			}
		}
	}
	if out, err := run(dir, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f == "" || strings.HasPrefix(f, ".hoom/verdicts/") || strings.HasPrefix(f, ".hoom/cache/") {
				continue
			}
			if raw, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
				if len(raw) > 0 && bytes.IndexByte(raw, 0) < 0 { // skip binaries
					ins += bytes.Count(raw, []byte("\n"))
				}
			}
		}
	}
	return ins, del
}

// GoPackages derives ./dir style package paths from changed .go files,
// for diff-scoped `go test {packages}`.
func GoPackages(changed []string) []string {
	set := map[string]bool{}
	for _, f := range changed {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		// vendored dependencies are never a test target
		if strings.HasPrefix(f, "vendor/") || strings.Contains(f, "/vendor/") {
			continue
		}
		dir := "./."
		if i := strings.LastIndex(f, "/"); i >= 0 {
			dir = "./" + f[:i]
		}
		set[dir] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
