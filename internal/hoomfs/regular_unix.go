//go:build !windows

package hoomfs

import (
	"fmt"
	"os"
	"syscall"
)

// sinBloquear keeps a FIFO from blocking an open.
const sinBloquear = syscall.O_NONBLOCK

// abrirSinSeguir opens path read-only on the entry itself: O_NOFOLLOW makes
// the open of a symlink fail instead of following it, and sinBloquear keeps
// a FIFO from blocking it.
func abrirSinSeguir(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|sinBloquear, 0)
}

// abrirDirEn opens name inside r as an os.Root without ever blocking:
// Root.OpenRoot opens with neither O_DIRECTORY nor O_NONBLOCK, so a FIFO put
// in the directory's place would hang it. The directory is opened as a
// file with both instead (a FIFO fails at once), and that descriptor itself
// becomes the Root through /dev/fd/N — darwin dups it, linux reopens it
// through /proc/self/fd — with no name resolved again.
func abrirDirEn(r *os.Root, name string) (*os.Root, error) {
	d, err := r.OpenFile(name, os.O_RDONLY|syscall.O_DIRECTORY|sinBloquear, 0)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return os.OpenRoot(fmt.Sprintf("/dev/fd/%d", d.Fd()))
}
