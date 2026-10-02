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
// by reopening its descriptor through each of fdDirs until one works — one
// that opens and gives back d itself (os.SameFile): whatever made one fail
// (missing, unreadable, another directory under that name), the next may
// still work. With none, ErrSinDescriptores and every cause.
func rootDe(d *os.File) (*os.Root, error) {
	propio, err := d.Stat()
	if err != nil {
		return nil, err
	}
	causas := make([]string, 0, len(fdDirs))
	for _, fds := range fdDirs {
		p := fmt.Sprintf("%s/%d", fds, d.Fd())
		sub, err := os.OpenRoot(p)
		if err != nil {
			causas = append(causas, err.Error())
			continue
		}
		st, err := sub.Stat(".")
		switch {
		case err != nil:
			sub.Close()
			causas = append(causas, p+": "+err.Error())
		case !os.SameFile(propio, st):
			sub.Close()
			causas = append(causas, p+" no reabre el mismo directorio")
		default:
			return sub, nil
		}
	}
	return nil, fmt.Errorf("%w (%s)", ErrSinDescriptores, strings.Join(causas, "; "))
}

var sondeo struct {
	once sync.Once
	err  error
}

// DescriptoresDisponibles says whether this system lets hoom reopen a
// directory by its descriptor: nil when it does, ErrSinDescriptores with its
// causes when it does not, ErrSinSonda when hoom could not even look. Never
// nil without having looked. It looks once (sondear).
func DescriptoresDisponibles() error {
	sondeo.once.Do(func() { sondeo.err = sondear() })
	return sondeo.err
}

// dirsSonda are the directories sondear tries to probe with, in order: the
// first one that opens as a directory. A test of this package may point it
// elsewhere.
var dirsSonda = []string{"/", ".", os.TempDir()}

// sondear reopens a directory by its descriptor, the way the photograph
// reopens each evidence directory (opened with O_DIRECTORY|O_NONBLOCK: a
// file or a FIFO in dirsSonda is no directory to probe with, and never
// blocks it): a failure there is the system's, never the evidence's. With no
// directory of dirsSonda to open, it has not looked, and says so
// (ErrSinSonda) instead of vouching for the system.
func sondear() error {
	var d *os.File
	causas := make([]string, 0, len(dirsSonda))
	for _, dir := range dirsSonda {
		f, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_DIRECTORY|sinBloquear, 0)
		if err == nil {
			d = f
			break
		}
		causas = append(causas, err.Error())
	}
	if d == nil {
		return fmt.Errorf("%w (%s)", ErrSinSonda, strings.Join(causas, "; "))
	}
	defer d.Close()
	sub, err := rootDe(d)
	if err != nil {
		return err
	}
	sub.Close()
	return nil
}
