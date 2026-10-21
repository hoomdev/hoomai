package gitx

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrSinComprobar: git could not answer whether a folder is a worktree of
// root (git missing, root's repository unreadable). It is not a "no", and a
// reader that hides the folder for it must say so (finding 6aeb17).
var ErrSinComprobar = errors.New("git no pudo comprobar los worktrees")

// WorktreeDe says whether dir is a worktree of root's own repository, the
// one git registered at that path: root lists dir among its worktrees, and
// from dir git answers dir as its top and root's common git dir as its own.
// A plain folder inside root answers root's top, a clone or another
// repository's worktree answers another common dir, and a folder whose .git
// file points at another worktree's git dir is not a path root registered
// (findings 28c350, 68644c). Nothing tracked can register a worktree: the
// registry lives in root's git dir. An error wraps ErrSinComprobar.
func WorktreeDe(root, dir string) (bool, error) {
	comun, err := gitOut(root, "rev-parse", "--git-common-dir")
	if err != nil {
		return false, fmt.Errorf("%w en %s: %v", ErrSinComprobar, root, err)
	}
	lista, err := gitOut(root, "worktree", "list", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("%w en %s: %v", ErrSinComprobar, root, err)
	}
	registrada := false
	for _, linea := range strings.Split(lista, "\n") {
		if p, ok := strings.CutPrefix(linea, "worktree "); ok && mismaRuta(dir, p) {
			registrada = true
			break
		}
	}
	if !registrada {
		return false, nil
	}
	out, err := gitOut(dir, "rev-parse", "--show-toplevel", "--git-common-dir")
	lineas := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lineas) != 2 {
		return false, nil
	}
	return mismaRuta(dir, rutaDeGit(dir, lineas[0])) &&
		mismaRuta(rutaDeGit(root, strings.TrimSpace(comun)), rutaDeGit(dir, lineas[1])), nil
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
