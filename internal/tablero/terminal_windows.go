//go:build windows

package tablero

import (
	"os"

	"golang.org/x/sys/windows"
)

// habilitarVT turns on the console's VT processing, which reads the ANSI
// sequences of the screen, and returns how to put the mode back.
func habilitarVT(f *os.File) func() {
	h := windows.Handle(f.Fd())
	var modo uint32
	if windows.GetConsoleMode(h, &modo) != nil {
		return func() {}
	}
	if windows.SetConsoleMode(h, modo|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
		return func() {}
	}
	return func() { _ = windows.SetConsoleMode(h, modo) }
}
