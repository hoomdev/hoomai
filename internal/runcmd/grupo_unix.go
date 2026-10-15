//go:build !windows

package runcmd

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

// enGrupo starts the provider in a process group of its own and makes
// cancelling the run (Cancel, its timeout) kill the whole group: what the CLI
// launched (its tools) goes with it (finding b5e187).
func enGrupo(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return matarGrupo(cmd.Process.Pid) }
}

// matarGrupo kills the process group whose leader is pid.
func matarGrupo(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}

// grupos are the process groups of the runs alive in this process. A group of
// its own no longer gets the terminal's Ctrl-C nor the hang-up of a closed
// terminal, and a hoom serve stopped with SIGTERM would leave it working: so
// the first run installs a handler that, on SIGINT, SIGTERM or SIGHUP, kills
// every group and then lets the signal do what it always did.
var grupos = struct {
	sync.Mutex
	pids map[int]bool
	once sync.Once
}{pids: map[int]bool{}}

func registrarGrupo(pid int) {
	grupos.once.Do(func() {
		ch := make(chan os.Signal, 1)
		senales := []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}
		signal.Notify(ch, senales...)
		go func() {
			s := <-ch
			grupos.Lock()
			for p := range grupos.pids {
				matarGrupo(p)
			}
			grupos.Unlock()
			signal.Reset(senales...)
			if sys, ok := s.(syscall.Signal); ok {
				syscall.Kill(os.Getpid(), sys)
			}
		}()
	})
	grupos.Lock()
	grupos.pids[pid] = true
	grupos.Unlock()
}

func soltarGrupo(pid int) {
	grupos.Lock()
	delete(grupos.pids, pid)
	grupos.Unlock()
}
