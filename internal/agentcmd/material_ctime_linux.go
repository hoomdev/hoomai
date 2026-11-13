//go:build linux

package agentcmd

import (
	"io/fs"
	"syscall"
)

// ctimeDe is the status change time of fi in nanoseconds, from its stat.
func ctimeDe(fi fs.FileInfo) (int64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Ctim.Nano(), true
}
