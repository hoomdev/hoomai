//go:build !windows

package procesos

import (
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// EnGrupo makes cmd start in a process group of its own, and ending it (exec's
// Cancel, when its context ends) kill the whole group.
func EnGrupo(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return MatarGrupo(cmd.Process.Pid) }
}

// Adoptar does nothing on Unix: the group was set when the process started.
func Adoptar(cmd *exec.Cmd) {}

// Soltar does nothing on Unix: a group has nothing to close.
func Soltar(pid int) {}

// MismoProceso says whether pid is still the process that started at
// arranco, by the start time ps reports (one-second resolution): a pid that
// another process reused started later. When ps cannot say, it answers true,
// the side that protects what the process may still be writing.
func MismoProceso(pid int, arranco time.Time) bool {
	if pid <= 0 || arranco.IsZero() {
		return false
	}
	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C") // the date is read: untranslated
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	inicio, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.TrimSpace(string(out)), time.Local)
	if err != nil {
		return true
	}
	diff := inicio.Sub(arranco)
	return diff > -3*time.Second && diff < 3*time.Second
}

// MatarGrupo kills the process group whose leader is pid. A process that is
// forking right when the signal reaches the group can leave a child that did
// not get it (macOS does not hold the fork back): so the group is signalled
// again a few times in the next milliseconds, until it is gone (finding
// e30013).
func MatarGrupo(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	for i := 0; i < 5 && err == nil; i++ {
		time.Sleep(10 * time.Millisecond)
		err = syscall.Kill(-pid, syscall.SIGKILL)
	}
	if err == syscall.ESRCH {
		return nil // nobody left in the group
	}
	return err
}

// TerminarGrupo ends the process group whose leader is pid the polite way
// first: SIGTERM to the whole group, which a docker client passes on to its
// container (so a containerized semgrep stops instead of running on, finding
// af0383) and a tool can catch to clean up; whoever is still in the group
// after gracia gets SIGKILL (MatarGrupo). It does not wait for the gracia:
// it is exec's Cancel, which must return at once.
func TerminarGrupo(pid int, gracia time.Duration) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGTERM)
	if err == syscall.ESRCH {
		return nil // nobody left in the group
	}
	go func() {
		time.Sleep(gracia)
		MatarGrupo(pid)
	}()
	return err
}

// senalarTerm sends SIGTERM to the group of pid and says whether it was
// there to get it.
func senalarTerm(pid int) bool {
	return pid > 0 && syscall.Kill(-pid, syscall.SIGTERM) == nil
}

// grupoVivo says whether anyone is left in the group of pid.
func grupoVivo(pid int) bool {
	return pid > 0 && syscall.Kill(-pid, 0) == nil
}

// Senales are the signals after which this process must not leave its
// groups behind: a group of its own gets neither the terminal's Ctrl-C nor
// the hang-up of a closed terminal.
var Senales = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// Terminar ends this process the way signal s would have, once its groups
// are dead.
func Terminar(s os.Signal) {
	signal.Reset(Senales...)
	if sys, ok := s.(syscall.Signal); ok {
		syscall.Kill(os.Getpid(), sys)
		time.Sleep(time.Second)
		os.Exit(128 + int(sys))
	}
	os.Exit(1)
}
