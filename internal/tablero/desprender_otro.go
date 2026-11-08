//go:build !unix && !windows

package tablero

import "os/exec"

// desprender has nothing to do where hoom has no process groups.
func desprender(*exec.Cmd) {}
