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
	patch, truncated, err := gitOutPrefix(dir, maxBytes, "diff", "--no-color", "--no-ext-diff", rng, "--")
	if err != nil {
		d.Note = "git diff " + rng + " fallo: " + err.Error()
		return d, nil
	}
	d.Patch, d.Truncated = patch, truncated
	d.Head, _ = run(dir, "rev-parse", "--short=12", "HEAD")
	d.Available = true
	return d, nil
}

// CandidatePatch is the unified patch of files against the merge-base of
// base and HEAD, working tree included (committed or not): the tracked files
// in one `git diff`, then each untracked file as a new-file patch. A binary
// file carries git's own binary line and no content. Never cut.
func CandidatePatch(dir, base string, files []string) ([]byte, error) {
	if len(files) == 0 {
		return nil, nil
	}
	from := base
	if mb, err := run(dir, "merge-base", base, "HEAD"); err == nil && mb != "" {
		from = mb
	}
	untracked := map[string]bool{}
	if out, err := run(dir, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				untracked[f] = true
			}
		}
	}
	var tracked, nuevos []string
	for _, f := range files {
		if untracked[f] {
			nuevos = append(nuevos, f)
		} else {
			tracked = append(tracked, f)
		}
	}
	var out bytes.Buffer
	if len(tracked) > 0 {
		args := append([]string{"diff", "--no-color", "--no-ext-diff", from, "--"}, tracked...)
		patch, err := gitOut(dir, args...)
		if err != nil {
			return nil, err
		}
		out.WriteString(patch)
	}
	for _, f := range nuevos {
		// --no-index exits 1 when the files differ: that is the patch, not a failure
		cmd := exec.Command("git", "diff", "--no-color", "--no-ext-diff", "--no-index", "--", "/dev/null", f)
		cmd.Dir = dir
		var stderr strings.Builder
		cmd.Stderr = &stderr
		patch, err := cmd.Output()
		if exit, ok := err.(*exec.ExitError); err != nil && !(ok && exit.ExitCode() == 1) {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return nil, errorString(msg)
			}
			return nil, err
		}
		out.Write(patch)
	}
	return out.Bytes(), nil
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

// gitOutPrefix is gitOut for an output that may not fit: it returns at most
// max bytes of git's stdout, cut on a line end, and truncated when there was
// more. It reads one byte past max to know that, drains the rest without
// keeping it and waits for git, so a failure after the cut is still git's
// error. max <= 0 reads everything.
func gitOutPrefix(dir string, max int, args ...string) (out string, truncated bool, err error) {
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
	switch rerr {
	case nil:
		truncated = true
		_, rerr = io.Copy(io.Discard, pipe)
	case io.EOF:
		rerr = nil
	}
	if rerr != nil {
		// a broken pipe: git is not read to the end, so it is stopped
		// before Wait
		cmd.Process.Kill()
		cmd.Wait()
		return "", false, rerr
	}
	if err := cmd.Wait(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", false, errorString(msg)
		}
		return "", false, err
	}
	if truncated {
		head := buf.Bytes()[:max]
		return string(head[:bytes.LastIndexByte(head, '\n')+1]), true, nil
	}
	return buf.String(), false, nil
}

type errorString string

func (e errorString) Error() string { return string(e) }
