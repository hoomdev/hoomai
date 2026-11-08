//go:build windows

package tablero

import (
	"os/exec"
	"syscall"
)

// lineaCmd is cmd running linea as it is written. The command line is set
// whole: Go's own quoting is for programs that parse it like the C runtime,
// and cmd does not (it reads \" as a backslash and a quote). /S takes off
// exactly the outer pair of quotes, whatever linea holds.
func lineaCmd(linea string) *exec.Cmd {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /S /C "` + linea + `"`}
	return cmd
}
