// Package gitx derives verification scope from Git, never from agent
// narration: commit, branch, dirty state and the changed-file set that
// diff-scoped gates (e.g. mutation) operate on.
package gitx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	if out, err := run(dir, "diff", "--name-only", "--diff-filter=ACMR", EndOfOptions, base+"...HEAD"); err == nil {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	// worktree divergence vs HEAD: uncommitted + untracked + deleted. A
	// listing that fails contributes nothing, as it always did here.
	local, _ := locales(func(args ...string) (string, error) { return run(dir, args...) })
	for f := range local {
		set[f] = true
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
	h, _ := fingerprintCon(dir, local, func(args ...string) (string, error) { return run(dir, args...) })
	return h
}

// fingerprintCon is fingerprint with the git runner rn: the only error it
// returns is a git that did not answer (errSinRespuesta); any other failure
// is what fingerprint always did with it.
func fingerprintCon(dir string, local map[string]bool, rn func(args ...string) (string, error)) (string, error) {
	entries := map[string]string{}
	out, err := rn("ls-tree", "-r", "HEAD")
	if errors.Is(err, errSinRespuesta) {
		return "", err
	}
	if err == nil {
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
		out, err := rn(append([]string{"hash-object", "--"}, localPaths...)...)
		if errors.Is(err, errSinRespuesta) {
			return "", err
		}
		if err == nil {
			hashes := strings.Split(out, "\n")
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
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// fueraDeRepo says whether a git that failed in dir did so because dir is
// in no repository: neither dir nor any directory above it has a .git
// (directory or file), and GIT_DIR does not point git anywhere. A tree that
// has one and where git fails is a broken repo, not "outside": the strict
// listings fail closed on it instead of answering with nothing (findings
// 9e4f40, e25ad1 -> 9ad39d).
func fueraDeRepo(dir string) bool {
	if os.Getenv("GIT_DIR") != "" {
		return false
	}
	d, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for {
		if _, err := os.Lstat(filepath.Join(d, ".git")); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
		arriba := filepath.Dir(d)
		if arriba == d {
			return true
		}
		d = arriba
	}
}

// HuellaConTope is Snapshot's ChangeFingerprint with every git bounded by
// tope: what stamps the moment of a finding needs it, and it must never hang
// on a git that blocks — a FIFO where git reads a .gitignore hung the scope
// gate while it registered its findings (finding 5c5342). A git that does
// not answer is an error and no fingerprint, and so is a first git that
// fails inside a repo (fueraDeRepo); outside a repo there is none and no
// error, as in Snapshot.
func HuellaConTope(dir string, tope time.Duration) (string, error) {
	rn := func(args ...string) (string, error) { return runTope(dir, tope, args...) }
	if _, err := rn("rev-parse", "--is-inside-work-tree"); err != nil {
		if errors.Is(err, errSinRespuesta) || !fueraDeRepo(dir) {
			return "", fmt.Errorf("git rev-parse: %w", err)
		}
		return "", nil
	}
	// the same divergence Snapshot finds, from the same function: only a git
	// that did not answer stops it
	local, err := locales(rn)
	if errors.Is(err, errSinRespuesta) {
		return "", err
	}
	return fingerprintCon(dir, local, rn)
}

// locales is what diverges from HEAD in the working tree — uncommitted,
// untracked (not ignored) and deleted paths — the half of the candidate the
// fingerprint hashes from disk. It runs both listings even if one fails and
// returns the paths they gave, with an error: a git that did not answer
// wins over any other failure, whichever came first, so a bounded caller
// never takes a partial listing for a whole one (findings cb8107, b6d4ca);
// Snapshot keeps the paths either way.
func locales(rn func(args ...string) (string, error)) (map[string]bool, error) {
	local := map[string]bool{}
	var falla error
	for _, args := range [][]string{
		{"diff", "--name-only", "HEAD"},
		{"ls-files", "--others", "--exclude-standard"},
	} {
		out, err := rn(args...)
		if err != nil {
			if falla == nil || (errors.Is(err, errSinRespuesta) && !errors.Is(falla, errSinRespuesta)) {
				falla = err
			}
			continue
		}
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				local[f] = true
			}
		}
	}
	return local, falla
}

// excludedFromCandidate: generated artifacts are never part of the candidate;
// recording or committing a verdict must not alter the fingerprint it
// certifies.
func excludedFromCandidate(path string) bool {
	return hoomfs.IsLocal(path) ||
		strings.HasPrefix(path, ".hoom/verdicts/") ||
		strings.HasPrefix(path, ".hoom/findings/") ||
		// records ABOUT the work, not the work: a card created or closed, or
		// a review recorded, never moves the fingerprint it talks about
		strings.HasPrefix(path, ".hoom/items/") ||
		strings.HasPrefix(path, ".hoom/reviews/") ||
		// harness state, not code under verification: a baseline tightened
		// DURING verify must not break the very check it just earned, and the
		// ignore file hoom completes on its own must not either (a role that
		// edits it is caught by the scope gate, which does not use this list)
		path == ".hoom/ratchet.json" || path == ".hoom/.gitignore"
}

// diffStats measures the change candidate size: numstat of base vs working
// tree, plus line counts of untracked files. Binary files are skipped.
func diffStats(dir, base string, changed []string) (ins, del int) {
	out, err := run(dir, "diff", "--numstat", EndOfOptions, base)
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
			if f == "" || excludedFromCandidate(f) {
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

// Touched lists EVERYTHING that differs from base — commits of the branch,
// working tree and untracked files — WITHOUT the candidate exclusions: the
// envelope's scope gate needs to see exactly what the fingerprint hides
// (.hoom/verdicts/, .hoom/findings/, .hoom/ratchet.json). The value is the
// content hash (git's own blob hasher, so committed and working-tree copies
// of the same bytes share an identity); "-" means the path is gone.
func Touched(dir, base string) map[string]string {
	out, _ := TouchedErr(dir, base)
	return out
}

// TopeListado bounds each git that Touched runs. Listing what changed reads
// every .gitignore of the tree, and one that is a FIFO blocks git forever:
// the scope gate's photograph would never finish, and with it the envelope
// (finding 5c5342).
var TopeListado = 2 * time.Minute

// TouchedErr is Touched that says when it could not list: a git that fails
// or does not answer within TopeListado is an error, never a shorter list —
// the scope gate fails closed on it (findings 5c5342 and 6d4918). Outside a
// repo there is nothing to list and no error, as in Touched; a first git
// that fails inside one is an error too (fueraDeRepo).
func TouchedErr(dir, base string) (map[string]string, error) {
	out := map[string]string{}
	if _, err := runTope(dir, TopeListado, "rev-parse", "--is-inside-work-tree"); err != nil {
		if errors.Is(err, errSinRespuesta) || !fueraDeRepo(dir) {
			return out, fmt.Errorf("git rev-parse: %w", err)
		}
		return out, nil
	}
	set := map[string]bool{}
	// No --diff-filter here: a path DELETED in the branch is a change too.
	for _, args := range [][]string{
		{"diff", "--name-only", EndOfOptions, base + "...HEAD"},
		{"diff", "--name-only", "HEAD"},
		{"ls-files", "--others", "--exclude-standard"},
	} {
		o, err := runTope(dir, TopeListado, args...)
		if err != nil {
			return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		for _, f := range strings.Split(o, "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	for f := range set {
		h, err := contentHash(filepath.Join(dir, f))
		if err != nil {
			out[f] = "-" // ya no existe (borrado, o un directorio que quedo en el listado)
			continue
		}
		out[f] = h
	}
	return out, nil
}

// errSinRespuesta is a git that did not answer within its bound.
var errSinRespuesta = errors.New("git no respondio")

// runTope is run with a bound: past tope the git is killed and the error
// wraps errSinRespuesta.
func runTope(dir string, tope time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tope)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%w en %s: git %s", errSinRespuesta, tope, strings.Join(args, " "))
	}
	return strings.TrimSpace(string(out)), err
}

// contentHash hashes a file's bytes. Deliberately NOT git hash-object: this
// is the primitive that detects tampering, so it must never depend on a
// subprocess that can fail and leave two snapshots agreeing on "unknown".
func contentHash(path string) (string, error) {
	// Never follow a symlink nor open anything but a regular file: a link
	// to a FIFO, or a FIFO itself, would block the open and hang the
	// photograph. A symlink is its target text, as git stores it.
	st, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	switch {
	case st.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256([]byte(target))
		return "symlink:" + hex.EncodeToString(sum[:]), nil
	case st.IsDir():
		return "", fmt.Errorf("%s no es un archivo regular", path)
	case !st.Mode().IsRegular():
		return "no-regular:" + st.Mode().Type().String(), nil
	}
	// the Lstat above can be stale by now: open what is there without
	// following it, and decide on that descriptor
	fh, err := hoomfs.AbrirRegular(path)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Identity is the project's git identity — "Name <email>", whichever of the
// two exists, or "desconocido" — the one human acts are recorded with (spec
// approvals, findings, items). One definition: three copies of this rule is
// how an approval and an item end up signed by two different people.
func Identity(dir string) string {
	get := func(key string) string {
		out, err := run(dir, "config", key)
		if err != nil {
			return ""
		}
		return out
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
