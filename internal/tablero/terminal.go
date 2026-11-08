package tablero

import (
	"os"

	"golang.org/x/term"
)

// terminalReal is the process's stdin and stdout when both are a terminal.
type terminalReal struct {
	in, out *os.File
}

// terminalDelProceso is the process's terminal, or nil when stdin or stdout
// is not one (a pipe, a file).
func terminalDelProceso() Terminal {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	return &terminalReal{in: os.Stdin, out: os.Stdout}
}

func (t *terminalReal) Read(p []byte) (int, error)  { return t.in.Read(p) }
func (t *terminalReal) Write(p []byte) (int, error) { return t.out.Write(p) }

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
