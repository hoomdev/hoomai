//go:build windows

package tablero

import (
	"os"
	"time"
	"unsafe"

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

var (
	kernel32      = windows.NewLazySystemDLL("kernel32.dll")
	procPeekInput = kernel32.NewProc("PeekConsoleInputW")
	procReadInput = kernel32.NewProc("ReadConsoleInputW")
)

// registroEntrada is a console INPUT_RECORD read as a KEY_EVENT_RECORD.
type registroEntrada struct {
	tipo     uint16
	_        uint16
	abajo    int32
	repetir  uint16
	tecla    uint16
	scan     uint16
	caracter uint16
	control  uint32
}

// hayEntrada waits up to d for a key that gives text: the console's other
// events (focus, the mouse, a key up) are taken out, or the Read that
// follows would block on them.
func hayEntrada(f *os.File, d time.Duration) (bool, error) {
	h := windows.Handle(f.Fd())
	ev, err := windows.WaitForSingleObject(h, uint32(d/time.Millisecond))
	if err != nil {
		return false, err
	}
	if ev != windows.WAIT_OBJECT_0 {
		return false, nil
	}
	var recs [32]registroEntrada
	var n uint32
	r, _, err := procPeekInput.Call(uintptr(h), uintptr(unsafe.Pointer(&recs[0])), uintptr(len(recs)), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return false, err
	}
	for _, rec := range recs[:n] {
		if rec.tipo == windows.KEY_EVENT && rec.abajo != 0 && rec.caracter != 0 {
			return true, nil
		}
	}
	if n > 0 {
		// only events without text: take exactly those out
		var leidos uint32
		procReadInput.Call(uintptr(h), uintptr(unsafe.Pointer(&recs[0])), uintptr(n), uintptr(unsafe.Pointer(&leidos)))
	}
	return false, nil
}

// avisosDeTamano: Windows has no SIGWINCH; the size is asked every 250 ms.
func avisosDeTamano() (<-chan os.Signal, func()) { return nil, func() {} }
