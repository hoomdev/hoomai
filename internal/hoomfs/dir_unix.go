//go:build !windows

package hoomfs

import (
	"fmt"
	"os"
	"strings"
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

// fdDirs are where rootDe reopens a descriptor by its number, in order:
// /dev/fd (darwin dups it; on linux it links to /proc/self/fd), then
// /proc/self/fd. A test of this package may point it elsewhere.
var fdDirs = []string{"/dev/fd", "/proc/self/fd"}

// rootDe turns the open directory d into an os.Root on that very directory
// by reopening its descriptor through each of fdDirs until one works:
// whatever made one fail (missing, unreadable, not what it should be), the
// next may still work. With none, ErrSinDescriptores and every cause.
func rootDe(d *os.File) (*os.Root, error) {
	causas := make([]string, 0, len(fdDirs))
	for _, fds := range fdDirs {
		sub, err := os.OpenRoot(fmt.Sprintf("%s/%d", fds, d.Fd()))
		if err == nil {
			return sub, nil
		}
		causas = append(causas, err.Error())
	}
	return nil, fmt.Errorf("%w (%s)", ErrSinDescriptores, strings.Join(causas, "; "))
}

var sondeo struct {
	once sync.Once
	err  error
}

// DescriptoresDisponibles says whether this system lets hoom reopen a
// directory by its descriptor: nil, or ErrSinDescriptores with its causes.
// It looks once (sondear).
func DescriptoresDisponibles() error {
	sondeo.once.Do(func() { sondeo.err = sondear() })
	return sondeo.err
}

// sondear reopens "/" by its descriptor, the way the photograph reopens
// each evidence directory: a failure there is the system's, never the
// evidence's. Only when even "/" cannot be opened is there nothing to try.
func sondear() error {
	d, err := os.Open("/")
	if err != nil {
		return nil
	}
	defer d.Close()
	sub, err := rootDe(d)
	if err != nil {
		return err
	}
	sub.Close()
	return nil
}
