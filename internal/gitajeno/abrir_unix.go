//go:build unix

package gitajeno

import (
	"os"
	"syscall"
)

// abrir opens p to read it without ever blocking on the open (a FIFO put in
// place of a ref does not hang the photograph), following a final symlink
// only when seguir.
func abrir(p string, seguir bool) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NONBLOCK
	if !seguir {
		flags |= syscall.O_NOFOLLOW
	}
	return os.OpenFile(p, flags, 0)
}
