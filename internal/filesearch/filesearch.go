// Package filesearch backs the Studio's @file autocomplete. It does not
// index the disk: git IS the index. `git ls-files` respects .gitignore by
// construction, so ignored trees (worktrees, runs, cache, node_modules)
// can never be suggested. A project without git degrades to zero
// suggestions, silently — never to a homemade indexer.
package filesearch

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// List returns the project's files: tracked plus untracked-but-not-ignored,
// deduplicated, in stable (sorted) order. Non-git roots yield an empty
// list and no error.
func List(root string) []string {
	set := map[string]bool{}
	for _, args := range [][]string{
		{"ls-files", "--cached"},
		{"ls-files", "--others", "--exclude-standard"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	files := make([]string, 0, len(set))
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// match ranks: filename prefix (0) > filename substring (1) > path
// substring (2); no match = -1. Query is treated as literal text.
func rank(path, q string) int {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case strings.HasPrefix(base, q):
		return 0
	case strings.Contains(base, q):
		return 1
	case strings.Contains(strings.ToLower(path), q):
		return 2
	}
	return -1
}

// Match filters files by a case-insensitive literal query with the ranking
// above; ties break alphabetically (the input's stable order). An empty
// query returns the first limit entries as-is.
func Match(files []string, query string, limit int) []string {
	if limit <= 0 {
		limit = 20
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		if len(files) > limit {
			return files[:limit]
		}
		return files
	}
	type scored struct {
		path string
		r    int
	}
	var hits []scored
	for _, f := range files {
		if r := rank(f, q); r >= 0 {
			hits = append(hits, scored{f, r})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].r < hits[j].r })
	out := make([]string, 0, limit)
	for _, h := range hits {
		out = append(out, h.path)
		if len(out) == limit {
			break
		}
	}
	return out
}
