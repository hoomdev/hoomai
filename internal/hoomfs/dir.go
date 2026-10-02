package hoomfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrSinDescriptores: this system gives hoom no way to reopen a directory
// by its descriptor (unix without /dev/fd nor /proc), so a directory cannot
// be opened without following a symlink nor blocking on a FIFO.
var ErrSinDescriptores = errors.New("hoom necesita /dev/fd o /proc para abrir la evidencia sin seguir symlinks")

// AbrirDirEn opens name inside r as the directory antes (the Lstat of name
// the caller just took) describes, for a walk that never follows a
// symlink: an os.Root follows one that stays inside it, and the entry can
// be swapped between that Lstat and the open, so the directory opened must
// be that same one (os.SameFile) and the name must still be it after the
// open (sigueSiendo), or the open is refused. A FIFO swapped in does not
// block it.
func AbrirDirEn(r *os.Root, name string, antes fs.FileInfo) (*os.Root, error) {
	sub, err := abrirDirEn(r, name)
	if err != nil {
		return nil, err
	}
	if st, err := sub.Stat("."); err != nil || !os.SameFile(antes, st) {
		sub.Close()
		return nil, fmt.Errorf("%s cambio entre mirarlo y abrirlo", name)
	}
	if err := sigueSiendo(r, name, antes); err != nil {
		sub.Close()
		return nil, err
	}
	return sub, nil
}
