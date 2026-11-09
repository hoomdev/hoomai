package tablero

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A path the evidence names (a finding's file, a card's spec) opens in the
// editor only inside the project. It is looked at twice, in two places:
//
//   - fueraPorLaForma, in the model (pure): its form, on every system,
//     before any order is given;
//   - fueraPorLosEnlaces, in Run, right before the editor: what the path
//     resolves to through the project's symlinks, inside os.Root.

// absoluta says whether p, a path of the evidence, is absolute on some
// system, whichever one reads it: rooted ("/x", "\x", a UNC "\\h\s") or
// with a drive ("C:x", "C:/x"). Evidence written on Windows is read on Unix
// too, and joined under a dir it would stop being absolute.
func absoluta(p string) bool {
	q := strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(q, "/") {
		return true
	}
	if len(q) >= 2 && q[1] == ':' && 'a' <= q[0]|0x20 && q[0]|0x20 <= 'z' {
		return true
	}
	return filepath.IsAbs(filepath.FromSlash(p)) || filepath.VolumeName(filepath.FromSlash(p)) != ""
}

// fueraPorLaForma says whether p, a path relative to the root, leaves the
// project by its form on some system: an absolute one (absoluta), or one
// whose ".." climbs out with / or \ as the separator.
func fueraPorLaForma(p string) bool {
	return absoluta(p) || !filepath.IsLocal(filepath.FromSlash(strings.ReplaceAll(p, `\`, "/")))
}

// fueraPorLosEnlaces says whether rel, followed through its symlinks inside
// os.Root, leaves root (fuera), or why it could not be told (err): the
// check fails closed, and only a path that os.Root resolves inside root,
// or one that is not there yet under components inside it, is fit to open.
// Nothing is looked at outside os.Root: a symlink of the project to
// another filesystem is not followed to find out.
func fueraPorLosEnlaces(root, rel string) (fuera bool, err error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return false, err
	}
	defer r.Close()
	_, err = r.Stat(rel)
	switch {
	case err == nil, errors.Is(err, fs.ErrNotExist):
		return false, nil
	case errors.Is(err, escapa(r)):
		return true, nil
	}
	return false, err
}

// escapa is the error os.Root gives for a path that leaves it, taken from
// the one path that always does (".."): os does not export it.
func escapa(r *os.Root) error {
	_, err := r.Stat("..")
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
