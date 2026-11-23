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

// EnlacesDuros is a regular file hoom does not write because it has more
// than one hard link (finding 31f78c): the write would land in every name of
// that file, and the others can be outside the project. The count comes from
// the descriptor (enlaces); where the system does not give it (Windows), it
// is not looked at.
type EnlacesDuros struct {
	Path    string
	Enlaces uint64
}

func (e *EnlacesDuros) Error() string {
	return fmt.Sprintf("%s tiene %d enlaces duros y hoom no escribe en un archivo que comparte con otro nombre: rompe el enlace copiandolo sobre si mismo (cp %s %s.copia && mv %s.copia %s) y vuelve a intentarlo",
		e.Path, e.Enlaces, e.Path, e.Path, e.Path, e.Path)
}

// unSoloEnlace is nil for st with one hard link, or whose count the system
// does not give (Windows), and *EnlacesDuros, with path, for one with more.
func unSoloEnlace(path string, st fs.FileInfo) error {
	if n, ok := enlaces(st); ok && n > 1 {
		return &EnlacesDuros{Path: path, Enlaces: n}
	}
	return nil
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
// open, so the descriptor must show that same object (mismo) and the
// name must still be it after the open (sigueSiendo), or the open is
// refused. A FIFO does not block it either.
func AbrirRegularEn(r *os.Root, name string, antes fs.FileInfo) (*os.File, error) {
	f, err := r.OpenFile(name, os.O_RDONLY|sinBloquear, 0)
	if err != nil {
		return nil, err
	}
	if f, err = regular(f, name, antes); err != nil {
		return nil, err
	}
	if err := sigueSiendo(r, name, antes); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// sigueSiendo says whether name in r is, by Lstat, still the object antes
// describes once it has been opened: os.Root follows a symlink that stays
// inside it, so a name moved aside and replaced by a symlink to itself
// passes os.SameFile on the descriptor. Before and after the open, the name
// is that object; what changes it later is a write after the photograph.
func sigueSiendo(r *os.Root, name string, antes fs.FileInfo) error {
	if st, err := r.Lstat(name); err != nil || !mismo(antes, st) {
		return fmt.Errorf("%s cambio entre mirarlo y abrirlo", name)
	}
	return nil
}

// mismo says whether antes and ahora can be the same object: the same
// device and inode number (os.SameFile) AND the same kind. The number alone
// is not an identity: a file system may hand a freed one to the next object
// it creates (linux does at once), so a name looked at as a symlink and the
// regular file put there afterwards can share it. What mismo cannot tell
// apart is an object of the SAME kind that inherited the number of a
// deleted one: stat exposes no generation, so it looks exactly like the
// old object rewritten in place, which nothing here detects either. For
// both it says yes, and the caller gets what is at the name now.
func mismo(antes, ahora fs.FileInfo) bool {
	return os.SameFile(antes, ahora) && antes.Mode().Type() == ahora.Mode().Type()
}

// regular keeps f only if its descriptor is a regular file — and, with
// antes, the very file antes describes.
func regular(f *os.File, path string, antes fs.FileInfo) (*os.File, error) {
	st, err := f.Stat()
	switch {
	case err != nil:
		f.Close()
		return nil, err
	case antes != nil && !mismo(antes, st):
		f.Close()
		return nil, fmt.Errorf("%s cambio entre mirarlo y abrirlo", path)
	case !st.Mode().IsRegular():
		f.Close()
		return nil, &NoRegular{Path: path, Mode: st.Mode()}
	}
	return f, nil
}
