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

// grupo is one registration of a process group. term and matar, when set
// (ArrancarCon), are how its polite signal and its end reach what it runs
// besides the group itself.
type grupo struct {
	gracia   time.Duration
	term     func() bool
	diferida func(hecho func(entregado bool)) bool
	matar    func()
}

// Terminacion is how ArrancarCon ends a command whose work does not live in
// its own process group: an encierro of bubblewrap, whose main process inside
// is in another group (encierro.Arrancar).
type Terminacion struct {
	// Term sends the polite signal (SIGTERM) to what must get it, and says
	// whether someone was there to get it; false makes the end come at once.
	// It must not block: it is called with the registry's lock held.
	Term func() bool
	// Diferida, when set, is used instead of Term, for a polite signal that
	// may have to wait until what must get it is there (an encierro whose
	// command has not started yet). It only starts it, and returns at once:
	// it is called with the registry's lock held, so it never waits there
	// and never holds up another group nor a signal to hoom. It says whether
	// someone is, or will be, there to get it (false makes the end come at
	// once), and then calls hecho, once and in a bounded time, with true when
	// the signal really reached it, or with false when nothing will get it.
	// The SIGKILL of the gracia is armed from that call, never before, and
	// false brings it at once (ronda 3, aacb4e and 75396e; aclaracion 31).
	// hecho never blocks, and it may be called before Diferida returns.
	Diferida func(hecho func(entregado bool)) bool
	// Matar ends everything at once, besides the SIGKILL to the group. It
	// must not block, and it is called with the registry's lock held.
	Matar func()
	// AlCancelar, when set, is called first thing when cmd's context ends
	// (its Cancel): the caller learns when the gracia started.
	AlCancelar func()
}

// ArrancarCon is ArrancarConGracia (gracia 0: Arrancar) for a command that
// is ended through t: where today SIGTERM goes to the group, t.Diferida or
// t.Term is called (on cmd's Cancel, and on a signal to hoom); wherever the
// group gets its SIGKILL (its gracia passed, Liberar, a signal to hoom, a
// group without gracia), t.Matar is called too.
func ArrancarCon(cmd *exec.Cmd, gracia time.Duration, t Terminacion) error {
	return arrancarCon(cmd, gracia, t)
}

// matarRegistrado kills the group of pid if someone is still in it, and
// whatever its registration g ends besides it. The registry's lock is held.
func matarRegistrado(pid int, g *grupo) {
	matarSiQueda(pid)
	if g != nil && g.matar != nil {
		g.matar()
	}
}

// termRegistrado sends the polite signal to the group of pid as its
// registration says, and says whether someone was there to get it. The
// registry's lock is held.
func termRegistrado(pid int) bool {
	if g := grupos.pids[pid]; g != nil && g.term != nil {
		return g.term()
	}
	return senalarTerm(pid)
}

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
					if g.matar != nil {
						g.matar()
					}
				}
			}
			// only the gates still there at the end of their gracia get
			// the SIGKILL
			for _, p := range terminarConGracia(conGracia, ch) {
				matarRegistrado(p, grupos.pids[p])
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
// 01c41b, 648d31). A group whose polite signal is Diferida is started with
// the others, without waiting for it, and its gracia counts from when it
// says it was delivered (or it dies then, if nothing got it). The registry's
// lock is held.
func terminarConGracia(pids []int, otra <-chan os.Signal) []int {
	hasta := time.Now()
	var vivos []int
	type pendiente struct {
		pid   int
		hecho <-chan bool
	}
	var pendientes []pendiente
	desde := func(p int) {
		vivos = append(vivos, p)
		if t := time.Now().Add(grupos.pids[p].gracia); t.After(hasta) {
			hasta = t
		}
	}
	for _, p := range pids {
		if g := grupos.pids[p]; g != nil && g.diferida != nil {
			hecho, ok := diferir(g)
			if !ok {
				matarRegistrado(p, g)
				continue
			}
			pendientes = append(pendientes, pendiente{p, hecho})
			continue
		}
		if !termRegistrado(p) {
			matarRegistrado(p, grupos.pids[p])
			continue
		}
		desde(p)
	}
	for len(pendientes) > 0 || (len(vivos) > 0 && time.Now().Before(hasta)) {
		select {
		case <-otra:
			for _, x := range pendientes {
				vivos = append(vivos, x.pid)
			}
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
		siguen := pendientes[:0]
		for _, x := range pendientes {
			select {
			case entregado := <-x.hecho:
				if entregado {
					desde(x.pid)
				} else {
					matarRegistrado(x.pid, grupos.pids[x.pid])
				}
			default:
				siguen = append(siguen, x)
			}
		}
		pendientes = siguen
	}
	return vivos
}

// diferir starts the polite signal of the registration g (its Diferida) and
// gives where its hecho arrives, and whether someone is there to get it. The
// registry's lock is held; Diferida never waits there.
func diferir(g *grupo) (<-chan bool, bool) {
	hecho := make(chan bool, 1)
	ok := g.diferida(func(entregado bool) {
		select {
		case hecho <- entregado:
		default:
		}
	})
	return hecho, ok
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
	return arrancarCon(cmd, gracia, Terminacion{})
}

func arrancarCon(cmd *exec.Cmd, gracia time.Duration, t Terminacion) error {
	term, matar := t.Term, t.Matar
	prepararSenales()
	EnGrupo(cmd)
	switch {
	case t.Diferida != nil && gracia > 0:
		// the polite signal is only started under the lock; the SIGKILL of
		// the gracia is armed when it says it was delivered, for this same
		// registration (ronda 3, aacb4e and 75396e)
		cmd.Cancel = func() error {
			pid := cmd.Process.Pid
			grupos.Lock()
			g := grupos.pids[pid]
			if g == nil {
				grupos.Unlock()
				return nil
			}
			hecho, ok := diferir(g)
			if !ok {
				matarRegistrado(pid, g)
			}
			grupos.Unlock()
			if ok {
				go func() {
					if <-hecho {
						armarSigkill(pid, g, gracia)
						return
					}
					grupos.Lock()
					defer grupos.Unlock()
					if grupos.pids[pid] == g {
						matarRegistrado(pid, g)
					}
				}()
			}
			return nil
		}
	case term != nil && gracia > 0:
		cmd.Cancel = func() error {
			pid := cmd.Process.Pid
			grupos.Lock()
			g := grupos.pids[pid]
			ok := g != nil && termRegistrado(pid)
			if g != nil && !ok {
				matarRegistrado(pid, g)
			}
			grupos.Unlock()
			if ok {
				matarTrasGracia(pid, gracia)
			}
			return nil
		}
	case matar != nil:
		cmd.Cancel = func() error {
			err := MatarGrupo(cmd.Process.Pid)
			matar()
			return err
		}
	case gracia > 0:
		cmd.Cancel = func() error { return TerminarGrupo(cmd.Process.Pid, gracia) }
	}
	if al, cancel := t.AlCancelar, cmd.Cancel; al != nil && cancel != nil {
		cmd.Cancel = func() error {
			al()
			return cancel()
		}
	}
	grupos.Lock()
	defer grupos.Unlock()
	if err := cmd.Start(); err != nil {
		return err
	}
	Adoptar(cmd) // Windows: its Job Object (finding d24626)
	grupos.pids[cmd.Process.Pid] = &grupo{gracia: gracia, term: term, diferida: t.Diferida, matar: matar}
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
	matarRegistrado(pid, grupos.pids[pid])
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
	armarSigkill(pid, esta, gracia)
}

// armarSigkill leaves the SIGKILL of matarTrasGracia for after gracia, for
// the registration esta of pid only (nil: nothing).
func armarSigkill(pid int, esta *grupo, gracia time.Duration) {
	if esta == nil {
		return
	}
	go func() {
		time.Sleep(gracia)
		grupos.Lock()
		defer grupos.Unlock()
		if grupos.pids[pid] == esta {
			matarRegistrado(pid, esta)
		}
	}()
}
