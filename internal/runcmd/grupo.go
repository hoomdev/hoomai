package runcmd

import (
	"os/exec"

	"github.com/hoomdev/hoomai/internal/procesos"
)

// arrancar starts a run's provider in a group of its own, registered for
// the signal handler that kills every group before hoom ends
// (procesos.Arrancar, shared with the gates of verify).
func arrancar(cmd *exec.Cmd) error { return procesos.Arrancar(cmd) }

// soltarGrupo is the end of a run's group, once its provider has ended:
// whatever the provider left running in it dies too (procesos.Liberar).
func soltarGrupo(pid int) { procesos.Liberar(pid) }
