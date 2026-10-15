//go:build windows

package runcmd

import "os/exec"

// On Windows the provider keeps the default: cancelling kills the process
// it started (Process.Kill). There is no process group to kill with it.
func enGrupo(cmd *exec.Cmd) {}

func registrarGrupo(pid int) {}

func soltarGrupo(pid int) {}
