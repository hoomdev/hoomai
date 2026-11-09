//go:build unix

package tablero

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// habilitarVT does nothing outside Windows: terminals read ANSI already.
func habilitarVT(*os.File) func() { return func() {} }

// hayEntrada waits up to d for f to have something to read. It uses select:
// poll does not work on terminals on macOS.
func hayEntrada(f *os.File, d time.Duration) (bool, error) {
	fd := int(f.Fd())
	var r unix.FdSet
	r.Zero()
	r.Set(fd)
	tv := unix.NsecToTimeval(d.Nanoseconds())
	n, err := unix.Select(fd+1, &r, nil, nil, &tv)
	if errors.Is(err, unix.EINTR) {
		return false, nil // SIGWINCH, for one
	}
	if err != nil {
		return false, err
	}
	return n > 0 && r.IsSet(fd), nil
}

// avisosDeTamano are the terminal's SIGWINCH.
func avisosDeTamano() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, unix.SIGWINCH)
	return ch, func() { signal.Stop(ch) }
}

// avisosDeSalida are the signals that end the TUI as Salir does: SIGTERM,
// SIGHUP and SIGINT (in raw mode ctrl-c is a key; a SIGINT comes from kill).
func avisosDeSalida() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, unix.SIGTERM, unix.SIGHUP, unix.SIGINT)
	return ch, func() { signal.Stop(ch) }
}

// codigoDeSenal is the exit code of a TUI ended by the signal s: 128+n.
func codigoDeSenal(s os.Signal) int {
	if n, ok := s.(syscall.Signal); ok {
		return 128 + int(n)
	}
	return 1
}
