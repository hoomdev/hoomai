package gitx

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sort"
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

// CandidatePatch is the unified patch of the WHOLE change against the
// merge-base of base and HEAD, working tree included: one `git diff` of
// everything tracked outside .hoom/ (deletions and both sides of a rename, as
// git writes them), then each untracked file outside .hoom/ as a new-file
// patch. A binary carries git's own binary line and no content. It reads at
// most max bytes: past that it stops git and returns over with nothing kept
// (read says how much it read). A failing git — no merge-base, the untracked
// listing, a diff — is an error, never a shorter patch.
func CandidatePatch(dir, base string, max int) (patch []byte, read int, over bool, err error) {
	mb, err := gitOut(dir, "merge-base", base, "HEAD")
	if err != nil {
		return nil, 0, false, fmt.Errorf("git merge-base %s HEAD: %v", base, err)
	}
	if mb = strings.TrimSpace(mb); mb == "" {
		return nil, 0, false, fmt.Errorf("git merge-base %s HEAD: sin ancestro comun", base)
	}
	// -z: names as they are on disk, never C-quoted
	lista, err := gitOut(dir, "ls-files", "-z", "--others", "--exclude-standard", "--", ".", ":(exclude).hoom")
	if err != nil {
		return nil, 0, false, fmt.Errorf("git ls-files: %v", err)
	}
	var out bytes.Buffer
	// quotePath=false: the patch names a file as it is on disk (ñ, not \303\261)
	chunk, over, err := gitOutBounded(dir, max, false, "-c", "core.quotePath=false",
		"diff", "--no-color", "--no-ext-diff", "--find-renames", mb, "--", ".", ":(exclude).hoom")
	if err != nil {
		return nil, 0, false, fmt.Errorf("git diff: %v", err)
	}
	if over {
		return nil, max + 1, true, nil
	}
	out.Write(chunk)
	var nuevos []string
	for _, f := range strings.Split(lista, "\x00") {
		if f != "" {
			nuevos = append(nuevos, f)
		}
	}
	sort.Strings(nuevos)
	for _, f := range nuevos {
		// --no-index exits 1 when the files differ: that is the patch
		chunk, over, err := gitOutBounded(dir, max-out.Len(), true, "-c", "core.quotePath=false",
			"diff", "--no-color", "--no-ext-diff", "--no-index", "--", "/dev/null", f)
		if err != nil {
			return nil, out.Len(), false, fmt.Errorf("git diff --no-index %s: %v", f, err)
		}
		if over {
			return nil, max + 1, true, nil
		}
		out.Write(chunk)
	}
	return out.Bytes(), out.Len(), false, nil
}

// gitOutBounded runs git and keeps at most max bytes of its stdout. Past max
// it stops git — the rest is never read — and reports over with nothing
// kept. exit1OK accepts exit code 1 with a silent stderr (git diff
// --no-index says "the files differ" that way); anything else that fails is
// an error carrying git's stderr.
func gitOutBounded(dir string, max int, exit1OK bool, args ...string) (out []byte, over bool, err error) {
	if max < 0 {
		return nil, true, nil
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	var buf bytes.Buffer
	_, rerr := io.CopyN(&buf, pipe, int64(max)+1)
	if rerr != io.EOF {
		// max+1 bytes read (over), or a broken pipe: git is not read to the
		// end, so it is stopped before Wait
		cmd.Process.Kill()
		cmd.Wait()
		if rerr == nil {
			return nil, true, nil
		}
		return nil, false, rerr
	}
	if werr := cmd.Wait(); werr != nil {
		msg := strings.TrimSpace(stderr.String())
		exit, ok := werr.(*exec.ExitError)
		if !(exit1OK && ok && exit.ExitCode() == 1 && msg == "") {
			if msg != "" {
				return nil, false, errorString(msg)
			}
			return nil, false, werr
		}
	}
	return buf.Bytes(), false, nil
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
