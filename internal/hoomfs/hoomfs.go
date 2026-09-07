// Package hoomfs is the single source of truth for what under .hoom is
// LOCAL — telemetry and scaffolding that never travel in Git nor enter the
// change candidate — and for the one way hoom writes those files: atomically.
// Five packages used to carry their own copy of the local list; a directory
// added in one and forgotten in another meant a run charged to the role, a
// fingerprint that moved, or an untracked file blocking a task.
package hoomfs

import (
	"os"
	"path/filepath"
	"strings"
)

// Local names the directories under .hoom/ that stay on this machine: the
// live cache, the narration of runs, task worktrees, blind trees and
// envelope records. Evidence (verdicts, findings, approvals) is NOT here:
// it travels.
var Local = []string{"cache", "worktrees", "runs", "isolated", "envelopes"}

// IsLocal reports whether a slash path relative to the project root points
// inside a local directory.
func IsLocal(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, d := range Local {
		if rel == ".hoom/"+d || strings.HasPrefix(rel, ".hoom/"+d+"/") {
			return true
		}
	}
	return false
}

// GitignoreBody is the canonical content of .hoom/.gitignore.
func GitignoreBody() string {
	var b strings.Builder
	for _, d := range Local {
		b.WriteString(d + "/\n")
	}
	return b.String()
}

// EnsureIgnored completes .hoom/.gitignore with the named local directories
// it lacks (all of them when none is named) and returns what it added. A file
// that already has them is never rewritten, not even byte-identically:
// registering telemetry must not move the candidate. Callers that only need
// their own entry ask for it by name, so opening a quarantine never edits the
// file for a directory it does not use.
func EnsureIgnored(root string, names ...string) (added []string, err error) {
	if len(names) == 0 {
		names = Local
	}
	gi := filepath.Join(root, ".hoom", ".gitignore")
	raw, _ := os.ReadFile(gi)
	have := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	for _, d := range names {
		if !have[d+"/"] {
			added = append(added, d+"/")
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		return nil, err
	}
	content := string(raw)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(added, "\n") + "\n"
	return added, AtomicWrite(gi, []byte(content), 0o644)
}

// AtomicWrite writes data to path through a temp file in the same directory
// and a rename: a concurrent reader sees the previous complete file or the
// new complete file, never a truncated one, and a crash mid-write leaves the
// previous content in place. The temp name never ends in the target's
// extension, so listings that filter by suffix never pick it up.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
