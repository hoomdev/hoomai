package runcmd

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"

	"github.com/hoomdev/hoomai/internal/procesos"
)

// grupos are the process groups (process trees on Windows) of the runs alive
// in this process. The first run installs a handler that, on one of
// procesos.Senales, kills every group and then ends the process as the signal
// would have: Ctrl-C, a closed terminal or a hoom serve stopped with SIGTERM
// leave no provider working alone (finding b5e187).
var grupos = struct {
	sync.Mutex
	pids map[int]bool
	once sync.Once
}{pids: map[int]bool{}}

func prepararSenales() {
	grupos.once.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, procesos.Senales...)
		go func() {
			s := <-ch
			// the lock is never released: a provider that another goroutine
			// was about to start never starts, and the process ends holding
			// it (finding e30013)
			grupos.Lock()
			for p := range grupos.pids {
				procesos.MatarGrupo(p)
			}
			procesos.Terminar(s)
		}()
	})
}

// arrancar starts cmd in a group of its own and registers the group in one
// step: a signal that arrives meanwhile waits for the registration, and the
// new group dies with the others (finding e30013).
func arrancar(cmd *exec.Cmd) error {
	prepararSenales()
	procesos.EnGrupo(cmd)
	grupos.Lock()
	defer grupos.Unlock()
	if err := cmd.Start(); err != nil {
		return err
	}
	procesos.Adoptar(cmd) // Windows: its Job Object (finding d24626)
	grupos.pids[cmd.Process.Pid] = true
	return nil
}

// soltarGrupo is the end of a run's group, once its provider has ended:
// whatever the provider left running in it (a tool in the background) dies
// too, as in Windows when the Job Object closes. A process left behind would
// keep writing in the tree of a run that already looks over, and no signal
// would reach it any more (finding e30013).
func soltarGrupo(pid int) {
	grupos.Lock()
	procesos.MatarGrupo(pid)
	delete(grupos.pids, pid)
	grupos.Unlock()
	procesos.Soltar(pid)
}
