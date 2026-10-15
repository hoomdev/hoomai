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
			grupos.Lock()
			for p := range grupos.pids {
				procesos.MatarGrupo(p)
			}
			grupos.Unlock()
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

func soltarGrupo(pid int) {
	grupos.Lock()
	delete(grupos.pids, pid)
	grupos.Unlock()
	procesos.Soltar(pid)
}
