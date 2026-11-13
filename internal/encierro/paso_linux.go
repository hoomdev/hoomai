//go:build linux

package encierro

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

// paso is the step of the encierro: when bwrap launched this binary as the
// step (marcaPaso), it makes itself the leader of a new session (no
// controlling terminal), with level completo forbids itself to connect to an
// abstract socket outside its domain (Landlock), and executes the command in
// its place (os.Args[1] the path, os.Args[2:] its argv). A failure ends the
// step: a level completo that could not apply Landlock does not run the
// command without it.
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
	// a session of its own, with no controlling terminal: the command leads
	// it, and the SIGTERM of the gracia goes to its group
	_, _ = syscall.Setsid()
	runtime.LockOSThread() // Landlock restricts this thread; execve keeps it
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
	err := syscall.Exec(path, os.Args[2:], os.Environ()) // nosemgrep: go.lang.security.audit.dangerous-syscall-exec.dangerous-syscall-exec -- el paso del encierro ejecuta el comando que Envolver puso detras de el: su argv lo arma hoom, como el exec.Cmd que envuelve
	fmt.Fprintf(os.Stderr, "hoom: encierro: %s: %v\n", os.Args[1], err)
	os.Exit(127)
}
