package gitx

import (
	"bytes"
	"io"
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
	patch, truncated, err := gitHead(dir, maxBytes, "diff", "--no-color", "--no-ext-diff", rng, "--")
	if err != nil {
		d.Note = "git diff " + rng + " fallo: " + err.Error()
		return d, nil
	}
	d.Patch, d.Truncated = patch, truncated
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

// gitHead is gitOut for an output that may not fit: it reads at most max
// bytes of git's stdout, cut on a line end, and never holds more than that.
// When git has more to say it is stopped and truncated is true; the rest
// stays in the pipe. max <= 0 reads everything.
func gitHead(dir string, max int, args ...string) (out string, truncated bool, err error) {
	if max <= 0 {
		out, err = gitOut(dir, args...)
		return out, false, err
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", false, err
	}
	if err := cmd.Start(); err != nil {
		return "", false, err
	}
	var buf bytes.Buffer
	_, rerr := io.CopyN(&buf, pipe, int64(max)+1)
	if rerr != io.EOF {
		// more than max (rerr nil) or a broken pipe: git is not read to
		// the end, so it is stopped before Wait
		cmd.Process.Kill()
		cmd.Wait()
		if rerr != nil {
			return "", false, rerr
		}
		head := buf.Bytes()[:max]
		return string(head[:bytes.LastIndexByte(head, '\n')+1]), true, nil
	}
	if err := cmd.Wait(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", false, errorString(msg)
		}
		return "", false, err
	}
	return buf.String(), false, nil
}

type errorString string

func (e errorString) Error() string { return string(e) }
