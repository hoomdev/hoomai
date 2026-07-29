// Package gitx derives verification scope from Git, never from agent
// narration: commit, branch, dirty state and the changed-file set that
// diff-scoped gates (e.g. mutation) operate on.
package gitx

import (
	"os/exec"
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
	if out, err := run(dir, "diff", "--name-only", "--diff-filter=ACMR", base+"...HEAD"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	if out, err := run(dir, "diff", "--name-only", "--diff-filter=ACMR", "HEAD"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	if out, err := run(dir, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	for f := range set {
		info.ChangedFiles = append(info.ChangedFiles, f)
	}
	sort.Strings(info.ChangedFiles)
	return info
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
