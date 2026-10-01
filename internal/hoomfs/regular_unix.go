//go:build !windows

package hoomfs

import (
	"os"
	"syscall"
)

// abrirSinSeguir opens path read-only on the entry itself: O_NOFOLLOW makes
// the open of a symlink fail instead of following it, and O_NONBLOCK keeps a
// FIFO from blocking it.
func abrirSinSeguir(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
