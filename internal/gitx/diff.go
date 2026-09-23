package gitx

import (
	"os/exec"
	"strconv"
	"strings"
)

// Diff is base...HEAD of a tree: what its branch changed, committed.
type Diff struct {
	Available  bool       `json:"available"`
	Base       string     `json:"base"`
	Head       string     `json:"head"`
	Files      []DiffFile `json:"files"`
	Insertions int        `json:"insertions"`
	Deletions  int        `json:"deletions"`
	Patch      string     `json:"patch"`
	Truncated  bool       `json:"truncated"`
	Note       string     `json:"note"`
}

// DiffFile is one file of the numstat.
type DiffFile struct {
	Path       string `json:"path"`
	Insertions int    `json:"insertions"`
	Deletions  int    `json:"deletions"`
}

// BranchDiff runs `git diff <base>...HEAD` in dir: only what the branch
// committed, never the working tree. The patch is cut at maxBytes on a line
// end (Truncated). When git fails, Available is false and Note carries git's
// own words; the error is reserved for nothing else, so a reader never has to
// tell the two apart.
func BranchDiff(dir, base string, maxBytes int) (Diff, error) {
	d := Diff{Base: base, Files: []DiffFile{}}
	rng := base + "...HEAD"
	numstat, err := gitOut(dir, "diff", "--no-color", "--no-ext-diff", "--numstat", rng, "--")
	if err != nil {
		d.Note = "git diff " + rng + " fallo: " + err.Error()
		return d, nil
	}
	for _, line := range strings.Split(strings.TrimRight(numstat, "\n"), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		f := DiffFile{Path: parts[2]}
		f.Insertions, _ = strconv.Atoi(parts[0]) // "-" (binary) stays 0
		f.Deletions, _ = strconv.Atoi(parts[1])
		d.Files = append(d.Files, f)
		d.Insertions += f.Insertions
		d.Deletions += f.Deletions
	}
	patch, err := gitOut(dir, "diff", "--no-color", "--no-ext-diff", rng, "--")
	if err != nil {
		d.Note = "git diff " + rng + " fallo: " + err.Error()
		return d, nil
	}
	if maxBytes > 0 && len(patch) > maxBytes {
		cut := strings.LastIndexByte(patch[:maxBytes], '\n')
		patch, d.Truncated = patch[:cut+1], true
	}
	d.Patch = patch
	d.Head, _ = run(dir, "rev-parse", "--short=12", "HEAD")
	d.Available = true
	return d, nil
}

// gitOut runs git and returns its stdout untrimmed; on failure the error
// carries git's stderr, which is what a person needs to read.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errorString(msg)
		}
		return "", err
	}
	return string(out), nil
}

type errorString string

func (e errorString) Error() string { return string(e) }
