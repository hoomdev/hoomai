//go:build !windows

package runcmd

import (
	"errors"
	"syscall"
)

// alive reports whether a process with this pid exists. Signal 0 probes
// without touching the process; EPERM means it exists but is not ours.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
