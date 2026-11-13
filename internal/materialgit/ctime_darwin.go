//go:build darwin

package materialgit

import (
	"io/fs"
	"syscall"
)

// CtimeDe is the status change time of fi in nanoseconds, from its stat.
func CtimeDe(fi fs.FileInfo) (int64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Ctimespec.Nano(), true
}

// InodoDe is the inode number of fi, from its stat; 0 when it has none.
func InodoDe(fi fs.FileInfo) uint64 {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(st.Ino)
}
