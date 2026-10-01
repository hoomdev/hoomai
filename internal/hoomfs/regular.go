package hoomfs

import (
	"fmt"
	"io/fs"
	"os"
)

// NoRegular is AbrirRegular's error for an entry that is not a regular file:
// Mode says what it is.
type NoRegular struct {
	Path string
	Mode fs.FileMode
}

func (e *NoRegular) Error() string {
	return fmt.Sprintf("%s no es un archivo regular (%s)", e.Path, e.Mode.Type())
}

// AbrirRegular opens path for reading only if it is a regular file, and it
// decides on the descriptor it returns, never on an earlier look at the
// path: between a look and an open anything can swap the entry (a process
// the run left behind, say), and on one descriptor nothing can. A symlink is
// not followed and a FIFO does not block the open; either one, or anything
// else that is not a regular file, comes back as *NoRegular or as the
// error of the open.
func AbrirRegular(path string) (*os.File, error) {
	f, err := abrirSinSeguir(path)
	if err != nil {
		return nil, err
	}
	return regular(f, path, nil)
}

// AbrirRegularEn is AbrirRegular for name inside r, where antes is the
// Lstat of name the caller just took: an os.Root follows a symlink that
// stays inside it, and the entry can be swapped between that Lstat and the
// open, so the descriptor must show that same file (os.SameFile) or the open
// is refused. A FIFO does not block it either.
func AbrirRegularEn(r *os.Root, name string, antes fs.FileInfo) (*os.File, error) {
	f, err := r.OpenFile(name, os.O_RDONLY|sinBloquear, 0)
	if err != nil {
		return nil, err
	}
	return regular(f, name, antes)
}

// AbrirDirEn opens name inside r as the directory antes (the Lstat of name
// the caller just took) describes, for a walk that never follows a
// symlink: an os.Root follows one that stays inside it, and the entry can
// be swapped between that Lstat and the open, so the directory opened must
// be that same one (os.SameFile) or the open is refused.
func AbrirDirEn(r *os.Root, name string, antes fs.FileInfo) (*os.Root, error) {
	sub, err := r.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	if st, err := sub.Stat("."); err != nil || !os.SameFile(antes, st) {
		sub.Close()
		return nil, fmt.Errorf("%s cambio entre mirarlo y abrirlo", name)
	}
	return sub, nil
}

// regular keeps f only if its descriptor is a regular file — and, with
// antes, the very file antes describes.
func regular(f *os.File, path string, antes fs.FileInfo) (*os.File, error) {
	st, err := f.Stat()
	switch {
	case err != nil:
		f.Close()
		return nil, err
	case antes != nil && !os.SameFile(antes, st):
		f.Close()
		return nil, fmt.Errorf("%s cambio entre mirarlo y abrirlo", path)
	case !st.Mode().IsRegular():
		f.Close()
		return nil, &NoRegular{Path: path, Mode: st.Mode()}
	}
	return f, nil
}
