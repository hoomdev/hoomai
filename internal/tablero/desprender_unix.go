//go:build unix

package tablero

import (
	"os/exec"
	"syscall"
)

// desprender gives the process a session of its own (and with it a process
// group): leaving the TUI, or closing its terminal, does not cut it.
func desprender(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
