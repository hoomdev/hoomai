package gitx

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrSinComprobar: git, or the registry of worktrees, could not answer
// whether a folder is a worktree of root (git missing, root's repository or
// its registry unreadable). It is not a "no", and a reader that hides the
// folder for it must say so (finding 6aeb17).
var ErrSinComprobar = errors.New("no se pudo comprobar el worktree")

// WorktreeDe says whether dir is a worktree of root's repository, the one
// git registered at that path, and on which branch its HEAD is (the full
// ref, "" when detached). It holds when:
//
//   - dir/.git is a regular file whose gitdir is <root's common dir>/worktrees/<id>;
//   - that registry's gitdir file names dir/.git back;
//   - from dir, git answers dir as its top and root's common dir as its own.
//
// A plain folder, a copy or a moved worktree, a .git that is a directory or
// a symlink, and a registered worktree whose .git someone pointed at another
// task's all fail; nothing tracked can register a worktree, since the
// registry lives in root's git dir (findings 28c350, 68644c). git runs
// without the GIT_* variables a hook would pass down, which would answer
// for another repository. An error wraps ErrSinComprobar.
func WorktreeDe(root, dir string) (rama string, ok bool, err error) {
	sinComprobar := func(donde string, err error) (string, bool, error) {
		return "", false, fmt.Errorf("%w en %s: %v", ErrSinComprobar, donde, err)
	}
	comun, err := gitLimpio(root, "rev-parse", "--git-common-dir")
	if err != nil {
		return sinComprobar(root, err)
	}
	comun = rutaDeGit(root, comun)
	// the registry, read here: git worktree list skips one it cannot read
	st, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil || !st.Mode().IsRegular() {
		return "", false, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return sinComprobar(dir, err)
	}
	gitdir, found := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir: ")
	if !found {
		return "", false, nil
	}
	gitdir = rutaDeGit(dir, gitdir)
	if !mismaRuta(filepath.Dir(gitdir), filepath.Join(comun, "worktrees")) {
		return "", false, nil
	}
	back, err := os.ReadFile(filepath.Join(gitdir, "gitdir"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return sinComprobar(gitdir, err)
	}
	if !mismaRuta(dir, filepath.Dir(rutaDeGit(gitdir, strings.TrimSpace(string(back))))) {
		return "", false, nil
	}
	// registered and pointed back: from here a failure of git is git's
	out, err := gitLimpio(dir, "rev-parse", "--show-toplevel", "--git-common-dir")
	lineas := strings.Split(out, "\n")
	if err != nil || len(lineas) != 2 {
		return sinComprobar(dir, fmt.Errorf("git rev-parse: %v", err))
	}
	if !mismaRuta(dir, rutaDeGit(dir, lineas[0])) || !mismaRuta(comun, rutaDeGit(dir, lineas[1])) {
		return "", false, nil
	}
	rama, err = gitLimpio(dir, "symbolic-ref", "-q", "HEAD")
	var salida *exec.ExitError
	if errors.As(err, &salida) && salida.ExitCode() == 1 {
		return "", true, nil // HEAD separado
	}
	if err != nil {
		return sinComprobar(dir, err)
	}
	return rama, true, nil
}

// gitLimpio is gitOut, trimmed, without the variables that make git answer
// for a repository other than dir's: a hook exports GIT_DIR, and with it a
// plain folder would pass for a worktree.
func gitLimpio(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		switch k, _, _ := strings.Cut(kv, "="); k {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE":
		default:
			cmd.Env = append(cmd.Env, kv)
		}
	}
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// rutaDeGit is a path git printed in dir: relative to dir unless absolute.
func rutaDeGit(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

// mismaRuta says whether a and b name the same place, symlinks resolved.
func mismaRuta(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
