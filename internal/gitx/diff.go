package gitx

import (
	"bufio"
	"bytes"
	"fmt"
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

// EndOfOptions goes before every revision that comes from outside (the
// base_branch of a hoom.yaml, a --base, the commit_final of an item): after
// it git reads the next argument as a revision, never as an option, so a
// value like `--output=<path>` is a bad revision instead of a file a read
// creates or truncates (finding bb986a). manifest.Load already refuses such a
// base_branch; this is the same guarantee for whoever calls git with a
// revision of its own.
const EndOfOptions = "--end-of-options"

// BranchDiff runs `git diff <base>...HEAD` in dir: only what the branch
// committed, never the working tree. It is a read, so git runs no program of
// the local configuration for it: no external diff (--no-ext-diff) and no
// textconv driver (--no-textconv, finding 43bde9). The patch is cut at
// maxBytes on a line end (Truncated). When git fails, Available is false and
// Note carries git's own words; the error is reserved for nothing else, so a
// reader never has to tell the two apart.
func BranchDiff(dir, base string, maxBytes int) (Diff, error) {
	d := Diff{Base: base, Files: []DiffFile{}}
	rng := base + "...HEAD"
	numstat, err := gitOut(dir, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--numstat", EndOfOptions, rng, "--")
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
	patch, truncated, err := gitOutPrefix(dir, maxBytes, "diff", "--no-color", "--no-ext-diff", "--no-textconv", EndOfOptions, rng, "--")
	if err != nil {
		d.Note = "git diff " + rng + " fallo: " + err.Error()
		return d, nil
	}
	d.Patch, d.Truncated = patch, truncated
	d.Head, _ = run(dir, "rev-parse", "--short=12", "HEAD")
	d.Available = true
	return d, nil
}

// CandidatePatch appends to dst the unified patch of the COMMITTED change:
// `git diff --find-renames` of the merge-base of base against HEAD, of
// everything outside .hoom/ plus .hoom/agents/ (the roles' contracts are part
// of the change), with deletions and both sides of a rename as git writes
// them. Nothing comes from the working tree: with anything uncommitted
// outside .hoom/ it returns ArbolSucio, before looking at the base, and
// reads no patch. A binary carries
// git's own binary line and no content, and neither textconv nor clean
// filters run (two commits are compared). dst holds at most max bytes of
// evidence: the moment it would hold max+1 it stops git and returns over, and
// the caller discards dst. A failing git (no merge-base, status, a diff) is
// an error, never a shorter patch.
func CandidatePatch(dir, base string, dst *bytes.Buffer, max int) (over bool, err error) {
	// the dirty tree goes first, as in hoom review: a dirty tree over a
	// broken base is refused for the dirt, not for the base
	ruta, err := CambioSinCommitear(dir)
	if err != nil {
		return false, err
	}
	if ruta != "" {
		return false, ArbolSucio{Ruta: ruta}
	}
	mb, err := MergeBase(dir, base)
	if err != nil {
		return false, err
	}
	return patchDesde(dir, mb, "HEAD", dst, max)
}

// RangePatch is CandidatePatch with explicit ends: the patch of desde..hasta,
// with the same paths, the same dirty-tree refusal (first) and the same cap.
// hasta is a frozen sha (the HEAD the review resolved once), so a HEAD that
// moves meanwhile never changes what the patch says; desde must be its
// ancestor.
func RangePatch(dir, desde, hasta string, dst *bytes.Buffer, max int) (over bool, err error) {
	ruta, err := CambioSinCommitear(dir)
	if err != nil {
		return false, err
	}
	if ruta != "" {
		return false, ArbolSucio{Ruta: ruta}
	}
	ok, err := EsAncestro(dir, desde, hasta)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("%s no es un ancestro de %s", desde, hasta)
	}
	return patchDesde(dir, desde, hasta, dst, max)
}

// evidencePaths are the paths the evidence covers: everything outside .hoom/
// plus .hoom/agents/ (the roles' contracts are part of the change).
var evidencePaths = [][]string{{".", ":(exclude).hoom"}, {".hoom/agents"}}

// patchDesde appends the patch of from..to over the evidence's paths.
func patchDesde(dir, from, to string, dst *bytes.Buffer, max int) (over bool, err error) {
	// quotePath=false: the patch names a file as it is on disk (ñ, not
	// \303\261); safecrlf=false: a CRLF warning on stderr is not a failure
	diff := []string{"-c", "core.quotePath=false", "-c", "core.safecrlf=false", "diff", "--no-color", "--no-ext-diff",
		"--no-textconv", "--find-renames", from, to, "--"}
	for _, paths := range evidencePaths {
		over, err = gitOutBounded(dir, dst, max, false, append(append([]string{}, diff...), paths...)...)
		if err != nil {
			return false, fmt.Errorf("git diff: %v", err)
		}
		if over {
			return true, nil
		}
	}
	return false, nil
}

// EsAncestro says whether commit a is an ancestor of b (a commit is its own
// ancestor). A git failure (an unknown object) is an error, never a no.
func EsAncestro(dir, a, b string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", a, b)
	cmd.Dir = dir
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return false, nil
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %s", a, b, msg)
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %v", a, b, err)
}

// ResolverCommit is the full sha of the commit rev names in dir; ok is false
// when rev names no commit there. rev must not start with "-" (the caller
// rejects it: it would be read as an option).
func ResolverCommit(dir, rev string) (sha string, ok bool, err error) {
	if rev == "" || strings.HasPrefix(rev, "-") {
		return "", false, nil
	}
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	cmd.Dir = dir
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if exit, isExit := err.(*exec.ExitError); isExit && exit.ExitCode() == 1 {
		return "", false, nil // --quiet: not a commit
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", false, fmt.Errorf("git rev-parse %s: %s", rev, msg)
		}
		return "", false, fmt.Errorf("git rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out)), true, nil
}

// Head is the full sha of HEAD in dir.
func Head(dir string) (string, error) {
	out, err := gitOut(dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(out), nil
}

// CambiosEntre are the files that changed from desde to hasta in the
// evidence's paths, with their added and deleted lines (a binary adds 0).
// Renames count as a deletion and an addition.
func CambiosEntre(dir, desde, hasta string) (files []string, ins, del int, err error) {
	for _, paths := range evidencePaths {
		args := append([]string{"-c", "core.quotePath=false", "diff", "--numstat", "-z", "--no-renames",
			"--no-ext-diff", "--no-textconv", desde, hasta, "--"}, paths...)
		out, err := gitOut(dir, args...)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("git diff --numstat %s %s: %v", desde, hasta, err)
		}
		// "<added>\t<deleted>\t<path>\0"; a binary is "-\t-\t<path>\0"
		for _, entry := range strings.Split(out, "\x00") {
			parts := strings.SplitN(entry, "\t", 3)
			if len(parts) != 3 {
				continue
			}
			a, _ := strconv.Atoi(parts[0])
			d, _ := strconv.Atoi(parts[1])
			ins, del = ins+a, del+d
			files = append(files, parts[2])
		}
	}
	return files, ins, del, nil
}

// CambioSinCommitear is the first path outside .hoom/ with anything not
// committed (modified, staged, deleted, or untracked and not ignored); ""
// when the tree is clean. Only that first entry is read: the listing is
// never held, and git status opens no file (a symlink to a FIFO is listed,
// never read). A bare FIFO is not a file for git: it is not listed, cannot
// be committed, and so never enters the evidence.
func CambioSinCommitear(dir string) (string, error) {
	cmd := exec.Command("git", "-c", "core.quotePath=false", "status", "--porcelain=v1", "-z",
		"--untracked-files=normal", "--", ".", ":(exclude).hoom")
	cmd.Dir = dir
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	entrada, rerr := bufio.NewReader(io.LimitReader(pipe, 64<<10)).ReadString(0)
	if entrada = strings.TrimSuffix(entrada, "\x00"); rerr == nil && entrada != "" {
		cmd.Process.Kill()
		cmd.Wait()
		// "XY ruta": the two status letters, a space, the path
		if len(entrada) > 3 {
			return entrada[3:], nil
		}
		return entrada, nil
	}
	if err := cmd.Wait(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git status: %s", msg)
		}
		return "", fmt.Errorf("git status: %v", err)
	}
	return "", nil
}

// ArbolSucio: the working tree has something uncommitted outside .hoom/, and
// the review only reviews what is committed.
type ArbolSucio struct{ Ruta string }

func (e ArbolSucio) Error() string {
	return fmt.Sprintf("la review revisa solo lo commiteado y hay cambios sin commitear (%s): commitealos antes de revisar", e.Ruta)
}

// stderrMax is how much of a git's stderr hoom keeps: enough for any real
// message, never in proportion to what a repository makes git print.
const stderrMax = 64 << 10

// capped keeps the first n bytes written to it and drops the rest.
type capped struct {
	b strings.Builder
	n int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.n - c.b.Len(); room > 0 {
		if len(p) > room {
			c.b.Write(p[:room])
		} else {
			c.b.Write(p)
		}
	}
	return len(p), nil
}

func (c *capped) String() string { return c.b.String() }

// VerificarBase fails when base and HEAD have no merge-base or git cannot
// diff them (a shallow clone, a base that does not exist, a missing tree): it
// runs before anything is measured, so a broken base never looks like "no
// changes" (CA-414).
func VerificarBase(dir, base string) error {
	mb, err := MergeBase(dir, base)
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "diff", "--quiet", mb, "HEAD", "--")
	cmd.Dir = dir
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
	err = cmd.Run()
	if exit, ok := err.(*exec.ExitError); err == nil || (ok && exit.ExitCode() == 1) {
		return nil // 1 = hay diferencias: git pudo compararlas
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return fmt.Errorf("git diff %s HEAD: %s", mb, msg)
	}
	return fmt.Errorf("git diff %s HEAD: %v", mb, err)
}

// HeadEntry is the tree entry of path at HEAD: its mode and object id. ok is
// false when HEAD does not have it; a failing git is an error.
func HeadEntry(dir, path string) (mode, oid string, ok bool, err error) {
	return EntradaEn(dir, "HEAD", path)
}

// EntradaEn is HeadEntry at revision rev.
func EntradaEn(dir, rev, path string) (mode, oid string, ok bool, err error) {
	out, err := gitOut(dir, "ls-tree", "-z", rev, "--", path)
	if err != nil {
		return "", "", false, fmt.Errorf("git ls-tree %s %s: %v", rev, path, err)
	}
	entrada := strings.TrimSuffix(out, "\x00")
	tab := strings.IndexByte(entrada, '\t')
	if entrada == "" || tab < 0 {
		return "", "", false, nil
	}
	// "<mode> <type> <oid>\t<path>"
	meta := strings.Fields(entrada[:tab])
	if len(meta) < 3 {
		return "", "", false, fmt.Errorf("git ls-tree %s %s: entrada inesperada %q", rev, path, entrada)
	}
	return meta[0], meta[2], true, nil
}

// AppendBlob appends the content of object oid to dst, within max as
// gitOutBounded does.
func AppendBlob(dir, oid string, dst *bytes.Buffer, max int) (over bool, err error) {
	return gitOutBounded(dir, dst, max, false, "cat-file", "blob", oid)
}

// MergeBase is the merge-base of base and HEAD in dir. No common ancestor (a
// shallow clone, a base that does not exist) is an error, never a fallback.
func MergeBase(dir, base string) (string, error) {
	return MergeBaseDe(dir, base, "HEAD")
}

// MergeBaseDe is MergeBase against revision rev instead of HEAD (a frozen
// sha, so a HEAD that moves meanwhile does not change it).
func MergeBaseDe(dir, base, rev string) (string, error) {
	mb, err := gitOut(dir, "merge-base", base, rev)
	if err != nil {
		return "", fmt.Errorf("git merge-base %s %s: %v", base, rev, err)
	}
	if mb = strings.TrimSpace(mb); mb == "" {
		return "", fmt.Errorf("git merge-base %s %s: sin ancestro comun", base, rev)
	}
	return mb, nil
}

// ShowFile is the content of path at revision rev. exists is false when the
// file is not in that revision; any other git failure is an error.
func ShowFile(dir, rev, path string) (content []byte, exists bool, err error) {
	// ls-tree says "absent" with an empty listing and fails for anything
	// else (a missing object, a broken repo): the two never mix
	entry, err := gitOut(dir, "ls-tree", "-z", rev, "--", path)
	if err != nil {
		return nil, false, fmt.Errorf("git ls-tree %s %s: %v", rev, path, err)
	}
	if strings.Trim(entry, "\x00\n ") == "" {
		return nil, false, nil
	}
	out, err := gitOut(dir, "show", rev+":"+path)
	if err != nil {
		return nil, false, fmt.Errorf("git show %s:%s: %v", rev, path, err)
	}
	return []byte(out), true, nil
}

// gitOutBounded runs git and appends its stdout to dst while dst holds at
// most max bytes: when it would hold max+1 it stops git — the rest is never
// read — and reports over (dst then has max+1 bytes the caller discards). exit1OK accepts exit code 1 with a silent stderr (git diff
// --no-index says "the files differ" that way); anything else that fails is
// an error carrying git's stderr.
func gitOutBounded(dir string, dst *bytes.Buffer, max int, exit1OK bool, args ...string) (over bool, err error) {
	room := max - dst.Len()
	if room < 0 {
		return true, nil
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	_, rerr := io.CopyN(dst, pipe, int64(room)+1)
	if rerr != io.EOF {
		// room+1 bytes read (over), or a broken pipe: git is not read to the
		// end, so it is stopped before Wait
		cmd.Process.Kill()
		cmd.Wait()
		if rerr == nil {
			return true, nil
		}
		return false, rerr
	}
	if werr := cmd.Wait(); werr != nil {
		msg := strings.TrimSpace(stderr.String())
		exit, ok := werr.(*exec.ExitError)
		if !(exit1OK && ok && exit.ExitCode() == 1 && msg == "") {
			if msg != "" {
				return false, errorString(msg)
			}
			return false, werr
		}
	}
	return false, nil
}

// gitOut runs git and returns its stdout untrimmed; on failure the error
// carries git's stderr, which is what a person needs to read.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
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
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
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
