//go:build !windows

package hoomfs

import (
	"io/fs"
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

// enlaces is the count of hard links of st, from the system's stat, and
// true; false when st does not carry one.
func enlaces(st fs.FileInfo) (uint64, bool) {
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(sys.Nlink), true // uint16 on darwin, uint32 on linux/386
}
