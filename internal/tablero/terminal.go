package tablero

import (
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// terminalReal is the process's stdin and stdout when both are a terminal.
// Its Read waits for input a moment at a time, so Pausar can stop it while
// the editor has the terminal: a Read blocked on the terminal would take
// the editor's first key.
type terminalReal struct {
	in, out *os.File

	mu      sync.Mutex
	pausada bool
}

// terminalDelProceso is the process's terminal, or nil when stdin or stdout
// is not one (a pipe, a file).
func terminalDelProceso() Terminal {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	return &terminalReal{in: os.Stdin, out: os.Stdout}
}

// esperaEntrada is how long one wait for input lasts.
const esperaEntrada = 50 * time.Millisecond

func (t *terminalReal) Read(p []byte) (int, error) {
	for {
		t.mu.Lock()
		if t.pausada {
			t.mu.Unlock()
			time.Sleep(esperaEntrada)
			continue
		}
		listo, err := hayEntrada(t.in, esperaEntrada)
		if err != nil {
			t.mu.Unlock()
			return 0, err
		}
		if !listo {
			t.mu.Unlock()
			continue
		}
		n, err := t.in.Read(p)
		t.mu.Unlock()
		return n, err
	}
}

func (t *terminalReal) Write(p []byte) (int, error) { return t.out.Write(p) }

// Pausar stops reading: when it returns, no Read is reading.
func (t *terminalReal) Pausar() {
	t.mu.Lock()
	t.pausada = true
	t.mu.Unlock()
}

// Reanudar reads again.
func (t *terminalReal) Reanudar() {
	t.mu.Lock()
	t.pausada = false
	t.mu.Unlock()
}

// Crudo puts stdin in raw mode (and, on Windows, stdout in VT mode for the
// ANSI sequences) and returns how to put both back.
func (t *terminalReal) Crudo() (func() error, error) {
	fd := int(t.in.Fd())
	estado, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	devolverVT := habilitarVT(t.out)
	return func() error {
		devolverVT()
		return term.Restore(fd, estado)
	}, nil
}

func (t *terminalReal) Tamano() (int, int, error) { return term.GetSize(int(t.out.Fd())) }
