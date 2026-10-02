//go:build !windows

package hoomfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"syscall"
)

// abrirDirEn opens name inside r as an os.Root without ever blocking:
// Root.OpenRoot opens with neither O_DIRECTORY nor O_NONBLOCK, so a FIFO put
// in the directory's place would hang it. The directory is opened as a
// file with both instead (a FIFO fails at once), and that descriptor itself
// becomes the Root (rootDe), with no name resolved again.
func abrirDirEn(r *os.Root, name string) (*os.Root, error) {
	d, err := r.OpenFile(name, os.O_RDONLY|syscall.O_DIRECTORY|sinBloquear, 0)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return rootDe(d)
}

// rootDe turns the open directory d into an os.Root on that very directory
// by reopening its descriptor: /dev/fd/N (darwin dups it; on linux it is a
// link to /proc/self/fd), then /proc/self/fd/N. With neither,
// ErrSinDescriptores.
func rootDe(d *os.File) (*os.Root, error) {
	var err error
	for _, fds := range []string{"/dev/fd", "/proc/self/fd"} {
		var sub *os.Root
		if sub, err = os.OpenRoot(fmt.Sprintf("%s/%d", fds, d.Fd())); err == nil {
			return sub, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w (%v)", ErrSinDescriptores, err)
}

var sondeo struct {
	once sync.Once
	err  error
}

// DescriptoresDisponibles says whether this system lets hoom reopen a
// directory by its descriptor: nil, or ErrSinDescriptores. It looks once.
func DescriptoresDisponibles() error {
	sondeo.once.Do(func() {
		d, err := os.Open(os.TempDir())
		if err != nil {
			return // nothing to probe with: no verdict on the system
		}
		defer d.Close()
		if sub, err := rootDe(d); err == nil {
			sub.Close()
		} else if errors.Is(err, ErrSinDescriptores) {
			sondeo.err = err
		}
	})
	return sondeo.err
}
