//go:build windows

package tablero

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// desprender gives the process a group of its own and no console: leaving
// the TUI, or closing its console, does not cut it.
func desprender(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
}
