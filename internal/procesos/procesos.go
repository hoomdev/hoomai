// Package procesos keeps what a command launches together, so that ending it
// ends everything it started: the provider of a run and its tools, a gate of
// hoom verify and whatever it spawned, or a tmux probe (findings b5e187,
// d24626, f8d179 and 862449).
package procesos

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"
)

// grupos are the process groups (process trees on Windows) started with
// Arrancar and alive in this process. The first one installs a handler that,
// on one of Senales, kills every group and then ends the process as the
// signal would have: Ctrl-C, a closed terminal or a hoom serve stopped with
// SIGTERM leave nothing working alone, whether it is a run's provider or a
// gate of verify (findings b5e187, 862449). A group of its own gets neither
// the terminal's Ctrl-C nor its hang-up, so without this it would go on.
var grupos = struct {
	sync.Mutex
	pids map[int]bool
	once sync.Once
}{pids: map[int]bool{}}

func prepararSenales() {
	grupos.once.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, Senales...)
		go func() {
			s := <-ch
			// the lock is never released: a command that another goroutine
			// was about to start never starts, and the process ends holding
			// it (finding e30013)
			grupos.Lock()
			for p := range grupos.pids {
				MatarGrupo(p)
			}
			Terminar(s)
		}()
	})
}

// Arrancar starts cmd in a group of its own (EnGrupo) and registers the
// group in one step: a signal that arrives meanwhile waits for the
// registration, and the new group dies with the others (finding e30013).
// Whoever calls it ends the group with Liberar once cmd's process ended.
func Arrancar(cmd *exec.Cmd) error { return arrancar(cmd, 0) }

// ArrancarConGracia is Arrancar where ending cmd's context ends its group
// with TerminarGrupo and gracia (SIGTERM, then SIGKILL) instead of killing
// it at once: what is set before the start, as exec requires.
func ArrancarConGracia(cmd *exec.Cmd, gracia time.Duration) error {
	return arrancar(cmd, gracia)
}

func arrancar(cmd *exec.Cmd, gracia time.Duration) error {
	prepararSenales()
	EnGrupo(cmd)
	if gracia > 0 {
		cmd.Cancel = func() error { return TerminarGrupo(cmd.Process.Pid, gracia) }
	}
	grupos.Lock()
	defer grupos.Unlock()
	if err := cmd.Start(); err != nil {
		return err
	}
	Adoptar(cmd) // Windows: its Job Object (finding d24626)
	grupos.pids[cmd.Process.Pid] = true
	return nil
}

// Liberar is the end of a group started with Arrancar, once its leader has
// ended: whatever the leader left running in it (a tool in the background)
// dies too, as in Windows when the Job Object closes, and the group is
// forgotten. A process left behind would keep working for a command that
// already looks over, and no signal would reach it any more (finding
// e30013).
func Liberar(pid int) {
	grupos.Lock()
	MatarGrupo(pid)
	delete(grupos.pids, pid)
	grupos.Unlock()
	Soltar(pid)
}
