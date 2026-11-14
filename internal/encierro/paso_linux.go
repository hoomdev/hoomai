//go:build linux

package encierro

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// paso is the step of the encierro: when bwrap launched this binary as the
// step (marcaPaso, at any level), it leads a session of its own (bwrap's
// --new-session; the SIGTERM of the gracia comes to its group), with level
// completo forbids the thread that starts the command to connect to an
// abstract socket outside its domain (Landlock), and starts the command
// (os.Args[1] the path, os.Args[2:] its argv) as its child, in a session of
// its own with no controlling terminal. It stays as a child subreaper
// (PR_SET_CHILD_SUBREAPER, ronda 1, a5c166): what the command leaves
// orphaned (a hook's child once git ended) becomes its child.
//
// Without a SIGTERM, when the command ends the step ends at once with its
// status, and bwrap ends whatever else is inside, as before (CA-679). A
// SIGTERM goes to the command's group, as before (held while the command is
// still starting: pasarTerm), and from then on the step
// waits for every descendant: when they all end it ends with the command's
// status, and otherwise the SIGKILL of the gracia from outside ends it all
// (CA-684). A command killed by a signal ends the step with 128 plus the
// signal, the status bwrap gives for it.
//
// A failure before the command starts ends the step: a level completo that
// could not apply Landlock does not run the command without it.
func paso() {
	nivel, ok := os.LookupEnv(marcaPaso)
	if !ok || nivel == "" {
		return
	}
	os.Unsetenv(marcaPaso)
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "hoom: encierro: el paso del encierro no recibio el comando")
		os.Exit(126)
	}
	// from here on a SIGTERM is the step's to pass on, never its death
	terms := make(chan os.Signal, 8)
	signal.Notify(terms, syscall.SIGTERM)
	// bwrap's --new-session already made it the leader of a session
	_, _ = syscall.Setsid()
	// only the thread that starts the command is under Landlock, and the
	// command (its child, the same user, no capabilities) must not ptrace
	// the step nor read its memory
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "hoom: encierro: el paso no pudo protegerse: %v\n", err)
		os.Exit(126)
	}
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "hoom: encierro: el paso no pudo quedar de subreaper: %v\n", err)
		os.Exit(126)
	}
	runtime.LockOSThread() // Landlock restricts this thread; the command is cloned from it
	if nivel == NivelCompleto {
		if err := restringir(); err != nil {
			fmt.Fprintf(os.Stderr, "hoom: encierro: %v\n", err)
			os.Exit(126)
		}
	}
	path := os.Args[1]
	if !strings.Contains(path, "/") {
		if p, err := exec.LookPath(path); err == nil {
			path = p
		}
	}
	// its own session, with no controlling terminal: the command leads it
	// (CA-680), and the SIGTERM the step passes on goes to its group
	pid, err := syscall.ForkExec(path, os.Args[2:], &syscall.ProcAttr{ // nosemgrep: go.lang.security.audit.dangerous-syscall-exec.dangerous-syscall-exec -- el paso del encierro ejecuta el comando que Envolver puso detras de el: su argv lo arma hoom, como el exec.Cmd que envuelve
		Env:   os.Environ(),
		Files: []uintptr{0, 1, 2},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hoom: encierro: %s: %v\n", os.Args[1], err)
		os.Exit(127)
	}
	soltarEntradaSalida()
	// syscall.Exit and not os.Exit: the step ends right away with the
	// command, with none of a Go program's exit work (a binary built with
	// -race, a test binary, sleeps a second at a clean os.Exit)
	syscall.Exit(esperarComando(pid, terms))
}

// soltarEntradaSalida points the step's stdin, stdout and stderr at
// /dev/null once the command has its own: the step does not keep the
// command's pipes open while it waits.
func soltarEntradaSalida() {
	fd, err := unix.Open("/dev/null", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return
	}
	for i := 0; i <= 2; i++ {
		_ = unix.Dup3(fd, i, 0)
	}
	if fd > 2 {
		_ = unix.Close(fd)
	}
}

// finDeHijo is one child the step reaped (wait4), or the error that ends
// the reaping (ECHILD: nobody left).
type finDeHijo struct {
	pid    int
	estado syscall.WaitStatus
	err    error
}

// esperarComando is the step's life once the command, pid, started: it reaps
// its children (the command and what it left orphaned), passes each SIGTERM
// of terms on to the command's group, and returns the status the step ends
// with: the command's as soon as it ends without a SIGTERM; after a SIGTERM,
// the command's once no descendant is left.
func esperarComando(pid int, terms <-chan os.Signal) int {
	fines := make(chan finDeHijo, 16)
	go func() {
		for {
			var st syscall.WaitStatus
			p, err := syscall.Wait4(-1, &st, 0, nil)
			if err == syscall.EINTR {
				continue
			}
			fines <- finDeHijo{pid: p, estado: st, err: err}
			if err != nil {
				return
			}
		}
	}()
	inicio := time.Now()
	terminando := false
	codigo := -1
	pasar := func() {
		terminando = true
		go pasarTerm(pid, inicio)
	}
	for {
		select {
		case <-terms:
			pasar()
		case f := <-fines:
			if f.err != nil {
				// nobody left (ECHILD): every descendant ended
				if codigo < 0 {
					codigo = 1
				}
				return codigo
			}
			if f.pid != pid {
				continue // an orphan the step adopted
			}
			codigo = codigoDe(f.estado)
			// a SIGTERM that came with the end still gives the rest its
			// gracia
			select {
			case <-terms:
				pasar()
			default:
			}
			if !terminando {
				return codigo
			}
		}
	}
}

// arranqueDelComando is how long the step holds a SIGTERM for a command that
// started less than that ago and does not handle SIGTERM yet: a cancel right
// at the start reaches the command once it can clean up (a trap, git's
// handler that drops index.lock), not while it is still starting.
const arranqueDelComando = 200 * time.Millisecond

// pasarTerm passes a SIGTERM on to the group of the command pid, started at
// inicio: at once when the command handles SIGTERM or started at least
// arranqueDelComando ago; otherwise as soon as one of the two is true.
func pasarTerm(pid int, inicio time.Time) {
	for !manejaTerm(pid) && time.Since(inicio) < arranqueDelComando {
		time.Sleep(2 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

// manejaTerm says whether the process pid has a handler of its own for
// SIGTERM (SigCgt in /proc/<pid>/status). false when it cannot say.
func manejaTerm(pid int) bool {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if hex, ok := strings.CutPrefix(l, "SigCgt:"); ok {
			m, err := strconv.ParseUint(strings.TrimSpace(hex), 16, 64)
			return err == nil && m&(1<<(uint(syscall.SIGTERM)-1)) != 0
		}
	}
	return false
}

// codigoDe is the exit status the step gives for the command's end st: its
// exit code, or 128 plus the signal that killed it (what bwrap gives).
func codigoDe(st syscall.WaitStatus) int {
	switch {
	case st.Exited():
		return st.ExitStatus()
	case st.Signaled():
		return 128 + int(st.Signal())
	}
	return 1
}
