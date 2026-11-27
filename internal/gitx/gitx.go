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
	"strconv"
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
	cmd := exec.Command("git", sinFsmonitor(args)...)
	cmd.Dir = dir
	cmd.Env = soloLectura()
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Rama is the branch of dir as Snapshot reads it (the one a verdict
// keeps), and whether dir is in a git repository: without computing the
// change set nor its fingerprint.
func Rama(dir string) (rama string, repo bool) {
	if _, err := run(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return "", false
	}
	rama, _ = run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	return rama, true
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
	rn := func(args ...string) (string, error) { return run(dir, args...) }
	prefix, _ := rn("rev-parse", "--show-prefix")
	local, _ := locales(rn)
	for f := range local {
		set[f] = true
	}
	for f := range set {
		// Generated artifacts are never part of the change candidate: the act
		// of recording a verdict must not alter the fingerprint it certifies.
		if excludedRel(f, prefix) {
			continue
		}
		info.ChangedFiles = append(info.ChangedFiles, f)
	}
	sort.Strings(info.ChangedFiles)
	info.ChangeFingerprint = fingerprint(dir, local)
	info.Insertions, info.Deletions = diffStats(dir, base, info.ChangedFiles)
	return info
}

// LineasDelCambio are the files of the change of dir and the lines of each
// (spec riesgo-efectivo-y-tamano, «Los archivos del cambio»): path relative
// to the root of the repository, with "/" -> insertions plus deletions. The
// files are those of `git diff --numstat --no-renames` from the merge-base
// of base and HEAD against the working tree, plus the untracked files git
// does not ignore; a deleted file is there with its deleted lines, a binary
// with 0, an untracked file with its lines (a last line without a newline
// counts; 0 when it is binary). What the base brought after the merge-base
// does not count. Nothing is left out: .hoom/ is there too (its class,
// riesgo.ClaseHoom, keeps it out of producto and pruebas). Unlike Snapshot,
// it does not touch Info.Insertions nor Info.Deletions, which stay as they
// are.
//
// Outside a git repository it is an error (`hoom verify` then writes no
// `tamano` and computes the riesgo without files); so is a git that cannot
// say the merge-base, the diff or the untracked files inside one.
//
// Its files include what the branch deleted in a commit, which Snapshot's
// ChangedFiles leave out (they are --diff-filter=ACMR): ArchivosConBorrados
// joins both.
func LineasDelCambio(dir, base string) (map[string]int, error) {
	raiz, err := gitOut(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse --show-toplevel: %v", err)
	}
	raiz = strings.TrimSpace(raiz)
	if raiz == "" {
		return nil, errors.New("git rev-parse --show-toplevel: sin arbol de trabajo")
	}
	mb, err := MergeBase(raiz, base)
	if err != nil {
		return nil, err
	}
	out, err := gitOut(raiz, "-c", "core.quotePath=false", "diff", "--numstat", "-z", "--no-renames",
		"--no-ext-diff", "--no-textconv", mb, "--")
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat %s: %v", mb, err)
	}
	lineas := lineasDelNumstat(out)
	otros, err := gitOut(raiz, "-c", "core.quotePath=false", "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files --others: %v", err)
	}
	for _, f := range strings.Split(otros, "\x00") {
		// a directory (a nested repository) is not a file of the change
		if f == "" || strings.HasSuffix(f, "/") {
			continue
		}
		n, err := lineasDeArchivo(filepath.Join(raiz, filepath.FromSlash(f)))
		if err != nil {
			return nil, err
		}
		lineas[f] = n
	}
	return lineas, nil
}

// lineasDelNumstat reads `git diff --numstat -z --no-renames`: each file
// with its insertions plus deletions; a binary ("-\t-") is 0.
func lineasDelNumstat(out string) map[string]int {
	lineas := map[string]int{}
	for _, entry := range strings.Split(out, "\x00") {
		parts := strings.SplitN(entry, "\t", 3)
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		d, _ := strconv.Atoi(parts[1])
		lineas[parts[2]] += a + d
	}
	return lineas
}

// lineasDeArchivo counts the lines of an untracked file the way git would
// show them added: one per newline, plus a last one without it; 0 for an
// empty file and for a binary (a NUL byte). A symbolic link is its target,
// one line.
func lineasDeArchivo(p string) (int, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return 0, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return 1, nil
	}
	if !fi.Mode().IsRegular() {
		return 0, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var (
		n      int
		ultimo byte
		buf    = make([]byte, 64<<10)
		leido  bool
	)
	for {
		k, err := f.Read(buf)
		if k > 0 {
			if bytes.IndexByte(buf[:k], 0) >= 0 {
				return 0, nil
			}
			n += bytes.Count(buf[:k], []byte("\n"))
			ultimo, leido = buf[k-1], true
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if leido && ultimo != '\n' {
		n++
	}
	return n, nil
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
//
// When dir is a subdirectory of the Git repository (e.g. a monorepo or
// nested project), every git listing is normalized to the repository root
// (--full-tree, --full-name, and -C top for hash-object) so that tracked
// modifications from `git diff --name-only HEAD` match the entries of
// `ls-tree` and exist on disk at filepath.Join(top, f). At the repository
// root, top == dir and prefix == "", producing byte-for-byte identical output.
func fingerprintCon(dir string, local map[string]bool, rn func(args ...string) (string, error)) (string, error) {
	top, err := rn("rev-parse", "--show-toplevel")
	if errors.Is(err, errSinRespuesta) {
		return "", err
	}
	if err != nil || top == "" {
		top = dir
	}
	prefix, err := rn("rev-parse", "--show-prefix")
	if errors.Is(err, errSinRespuesta) {
		return "", err
	}
	if err != nil {
		prefix = ""
	}

	entries := map[string]string{}
	out, err := rn("ls-tree", "-r", "--full-tree", "HEAD")
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
			if len(meta) >= 3 && !excludedRel(path, prefix) {
				entries[path] = "g:" + meta[2]
			}
		}
	}
	// Hash local files with git's own blob hasher so a file has the SAME
	// identity whether committed or sitting in the working tree: committing
	// identical content preserves the fingerprint by construction.
	var localPaths []string
	for f := range local {
		if excludedRel(f, prefix) {
			continue
		}
		if _, err := os.Stat(filepath.Join(top, filepath.FromSlash(f))); err != nil {
			delete(entries, f) // deleted locally
			continue
		}
		localPaths = append(localPaths, f)
	}
	sort.Strings(localPaths)
	if len(localPaths) > 0 {
		args := append([]string{"-C", top, "hash-object", "--"}, localPaths...)
		out, err := rn(args...)
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

// dirReal is dir as git runs in it: every symlink resolved left to right,
// before any "..", as the kernel does on a chdir (filepath.EvalSymlinks; Abs
// first would clean alias/.. away before seeing the link, finding 7adfcb),
// and then absolute. The strict listings resolve it ONCE and both run git in
// it and look for .git above it: a symlink retargeted between the two
// changes neither (finding 0738fe), and the parents looked at are the ones
// git walks up from, not those of a link's name (findings a5cd1a, 90c86c,
// 6f858b).
func dirReal(dir string) (string, error) {
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return filepath.Abs(d)
}

// fueraDeRepo says whether a git that failed in the real directory d
// (dirReal) did so because d is in no repository: neither d nor any
// directory above it has a .git (directory or file), and GIT_DIR does not
// point git anywhere. A tree that has one and where git fails is a broken
// repo, not "outside": the strict listings fail closed on it instead of
// answering with nothing (findings 9e4f40, e25ad1 -> 9ad39d).
func fueraDeRepo(d string) bool {
	if os.Getenv("GIT_DIR") != "" {
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
	dir, err := dirReal(dir)
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver el directorio: %w", err)
	}
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
		{"ls-files", "--full-name", "--others", "--exclude-standard"},
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

// ArchivosConBorrados is what the change touches, deletions included:
// changed (Snapshot's ChangedFiles) plus the files of lineas
// (LineasDelCambio) the candidate does not leave out, sorted, without
// repeats. Snapshot lists the committed changes with --diff-filter=ACMR, so
// a file the branch deleted in a commit is only in lineas; `hoom review` and
// the board decide «no changes» and «only documentation» over this list
// (findings 85c48e, d4c535). Info.ChangedFiles stays as it is: the
// verdict's changed_files and its fingerprint do not move.
func ArchivosConBorrados(changed []string, lineas map[string]int) []string {
	set := make(map[string]bool, len(changed)+len(lineas))
	for _, f := range changed {
		set[f] = true
	}
	for f := range lineas {
		if f != "" && !excludedFromCandidate(f) {
			set[f] = true
		}
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
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

// excludedRel checks excludedFromCandidate both at the repository root and
// under prefix (the subdir prefix returned by `git rev-parse --show-prefix`,
// e.g. "electronic_invoices/").
func excludedRel(path, prefix string) bool {
	if excludedFromCandidate(path) {
		return true
	}
	if prefix != "" && strings.HasPrefix(path, prefix) {
		return excludedFromCandidate(strings.TrimPrefix(path, prefix))
	}
	return false
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
	out, _, err := TouchedRelErr(dir, base)
	return out, err
}

// TouchedRelErr is TouchedErr that also returns the subdirectory prefix of dir
// inside its Git repository (e.g. "electronic_invoices/", or "" at the repo
// root / outside a repo), derived from the same `rev-parse --show-toplevel`
// without spawning an extra git process.
func TouchedRelErr(dir, base string) (map[string]string, string, error) {
	out := map[string]string{}
	dir, err := dirReal(dir)
	if err != nil {
		return out, "", fmt.Errorf("no se pudo resolver el directorio: %w", err)
	}
	top, err := runTope(dir, TopeListado, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, errSinRespuesta) || !fueraDeRepo(dir) {
			return out, "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
		}
		return out, "", nil
	}
	if top == "" {
		top = dir
	} else if tr, rerr := dirReal(top); rerr == nil {
		top = tr
	}
	prefix := ""
	if top != dir {
		if rel, rerr := filepath.Rel(top, dir); rerr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			prefix = filepath.ToSlash(rel) + "/"
		}
	}
	set := map[string]bool{}
	// No --diff-filter here: a path DELETED in the branch is a change too.
	lsArgs := []string{"ls-files", "--full-name", "--others", "--exclude-standard"}
	if prefix != "" {
		lsArgs = append([]string{"-C", top}, lsArgs...)
	}
	for _, args := range [][]string{
		{"diff", "--name-only", EndOfOptions, base + "...HEAD"},
		{"diff", "--name-only", "HEAD"},
		lsArgs,
	} {
		o, err := runTope(dir, TopeListado, args...)
		if err != nil {
			return out, "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		for _, f := range strings.Split(o, "\n") {
			if f != "" {
				set[f] = true
			}
		}
	}
	for f := range set {
		h, err := contentHash(filepath.Join(top, filepath.FromSlash(f)))
		if err != nil {
			out[f] = "-" // ya no existe (borrado, o un directorio que quedo en el listado)
			continue
		}
		out[f] = h
	}
	return out, prefix, nil
}

// Prefix returns the subdirectory path of dir inside its Git repository
// (from `git rev-parse --show-prefix`, using "/" and ending in "/", e.g.
// "electronic_invoices/"), or "" when dir is the repository root or outside
// a Git repository. A broken repository or a git that does not answer within
// TopeListado fails closed with an error.
func Prefix(dir string) (string, error) {
	dir, err := dirReal(dir)
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver el directorio: %w", err)
	}
	out, err := runTopeRaw(dir, TopeListado, "-c", "core.quotePath=false", "rev-parse", "--show-prefix")
	if err != nil {
		if errors.Is(err, errSinRespuesta) || !fueraDeRepo(dir) {
			return "", fmt.Errorf("git rev-parse --show-prefix: %w", err)
		}
		return "", nil
	}
	out = strings.TrimSuffix(strings.TrimSuffix(out, "\n"), "\r")
	return filepath.ToSlash(out), nil
}

// RelAlProyecto normalizes a repository-root-relative path (using "/") against
// the project's subdirectory prefix (from Prefix, e.g. "electronic_invoices/").
// At the repository root (prefix == ""), path is returned unchanged. Inside
// prefix, the prefix is stripped; outside prefix, "../" segments are prepended
// so a path outside the project never collides with project-relative globs.
func RelAlProyecto(path, prefix string) string {
	if prefix == "" {
		return path
	}
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return strings.Repeat("../", strings.Count(prefix, "/")) + path
}

// errSinRespuesta is a git that did not answer within its bound.
var errSinRespuesta = errors.New("git no respondio")

// runTope is run with a bound: past tope the git is killed and the error
// wraps errSinRespuesta.
func runTope(dir string, tope time.Duration, args ...string) (string, error) {
	out, err := runTopeRaw(dir, tope, args...)
	return strings.TrimSpace(out), err
}

// runTopeRaw is runTope without trimming spaces (needed for --show-prefix so
// leading or trailing spaces in directory names are preserved).
func runTopeRaw(dir string, tope time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tope)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", sinFsmonitor(args)...)
	cmd.Dir = dir
	cmd.Env = soloLectura()
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%w en %s: git %s", errSinRespuesta, tope, strings.Join(args, " "))
	}
	return string(out), err
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

// sinFsmonitor puts core.fsmonitor=false before args: no git of hoom over a
// tree runs the fsmonitor a role may have left in a config
// (contencion-del-writer, CA-693).
func sinFsmonitor(args []string) []string {
	return append([]string{"-c", "core.fsmonitor=false"}, args...)
}
