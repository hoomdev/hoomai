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
//
// Each group keeps its gracia: one started with ArrancarConGracia gets
// SIGTERM first and up to that long to end before the SIGKILL, as when its
// deadline passes, so a docker client can stop its container on a Ctrl-C too
// (finding 3f106e). A second signal meanwhile cuts the wait short.
//
// A group is only known by its number, which the system hands to a new
// group once the old one is empty. Two protections, and one window left:
//   - the delayed SIGKILL of TerminarGrupo goes only to the registration it
//     was armed for: each registration is an object of its own (grupo), and
//     a new group of hoom with the same number is another one (finding
//     bc1864);
//   - a group already killed, or seen empty, is never signalled again
//     (findings 67eb4f, f5984e);
//   - between looking at a group (does it still have members?) and
//     signalling it, the system could hand its number to someone else's
//     group. That takes the whole pid range going round in that instant, and
//     it is accepted (Henry, 2026-10-27; findings 710266, f6709d, 6d3179).
var grupos = struct {
	sync.Mutex
	pids map[int]*grupo // the leader's pid of each group -> its registration
	once sync.Once
}{pids: map[int]*grupo{}}

// grupo is one registration of a process group.
type grupo struct{ gracia time.Duration }

func prepararSenales() {
	grupos.once.Do(func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, Senales...)
		go func() {
			s := <-ch
			// the lock is never released: a command that another goroutine
			// was about to start never starts, and the process ends holding
			// it (finding e30013)
			grupos.Lock()
			// the groups without gracia (a run's provider) die at once and
			// are not signalled again: they never wait for a gate that
			// ignores SIGTERM (finding 1e64a4)
			var conGracia []int
			for p, g := range grupos.pids {
				if g.gracia > 0 {
					conGracia = append(conGracia, p)
				} else {
					MatarGrupo(p)
				}
			}
			// only the gates still there at the end of their gracia get
			// the SIGKILL
			for _, p := range terminarConGracia(conGracia, ch) {
				matarSiQueda(p)
			}
			Terminar(s)
		}()
	})
}

// terminarConGracia sends SIGTERM to the registered groups of pids and
// waits until they are gone, their longest gracia passes, or another signal
// arrives on otra. It returns the ones still there: a group seen empty is
// dropped and never signalled again. One that cannot get a SIGTERM (Windows
// has none for a tree) is killed right there, never left out (findings
// 01c41b, 648d31). The registry's lock is held.
func terminarConGracia(pids []int, otra <-chan os.Signal) []int {
	hasta := time.Now()
	var vivos []int
	for _, p := range pids {
		if !senalarTerm(p) {
			matarSiQueda(p)
			continue
		}
		vivos = append(vivos, p)
		if t := time.Now().Add(grupos.pids[p].gracia); t.After(hasta) {
			hasta = t
		}
	}
	for len(vivos) > 0 && time.Now().Before(hasta) {
		select {
		case <-otra:
			return vivos
		case <-time.After(20 * time.Millisecond):
		}
		quedan := vivos[:0]
		for _, p := range vivos {
			if grupoVivo(p) {
				quedan = append(quedan, p)
			}
		}
		vivos = quedan
	}
	return vivos
}

// EsperarGrupo waits until the group of pid is gone or hasta passes: the
// rest of a group's gracia once its leader already ended (finding 01d565).
// On Windows a tree has no gracia, and it returns at once.
func EsperarGrupo(pid int, hasta time.Time) {
	for grupoVivo(pid) && time.Now().Before(hasta) {
		time.Sleep(20 * time.Millisecond)
	}
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
	grupos.pids[cmd.Process.Pid] = &grupo{gracia: gracia}
	return nil
}

// Liberar is the end of a group started with Arrancar, once its leader has
// ended: whatever the leader left running in it (a tool in the background)
// dies too, as in Windows when the Job Object closes, and the group is
// forgotten. A process left behind would keep working for a command that
// already looks over, and no signal would reach it any more (finding
// e30013). On Unix only a group that still has members gets the signal: an
// empty one may have handed its number to someone else's group (findings
// 710266, 2f4af2).
func Liberar(pid int) {
	grupos.Lock()
	matarSiQueda(pid)
	delete(grupos.pids, pid)
	grupos.Unlock()
	Soltar(pid)
}

// matarTrasGracia is the SIGKILL that TerminarGrupo leaves for after gracia:
// only for the very registration pid has now (a group that Liberar forgot,
// or a new group of hoom that got the same number, is another object) and
// only if the group still has members, checked under the registry's lock
// (findings 2f4af2, f0dfd4, cd9f87, bc1864). A pid with no registration gets
// nothing later.
func matarTrasGracia(pid int, gracia time.Duration) {
	grupos.Lock()
	esta := grupos.pids[pid]
	grupos.Unlock()
	if esta == nil {
		return
	}
	go func() {
		time.Sleep(gracia)
		grupos.Lock()
		defer grupos.Unlock()
		if grupos.pids[pid] == esta {
			matarSiQueda(pid)
		}
	}()
}
