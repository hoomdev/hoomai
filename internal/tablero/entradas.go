package tablero

// AccionRaton is what a mouse event did.
type AccionRaton int

// The mouse events the model knows. Any other (the middle and right
// buttons, a release, a move) is never decoded.
const (
	// RatonClic is the left button pressed.
	RatonClic AccionRaton = iota + 1
	// RatonRuedaArriba and RatonRuedaAbajo are one notch of the wheel.
	RatonRuedaArriba
	RatonRuedaAbajo
)

// Raton is one mouse event: what it did and on which cell of the screen.
// X is the cell's column and Y its line, both from 0: Y is the index of
// the line in Pantalla, and X counts cells as the package doc's width table
// does (a wide character takes two columns).
type Raton struct {
	Accion AccionRaton
	X, Y   int
}

// Entrada is one thing the terminal sent: a key or a mouse event. Exactly
// one of the two is set: Tecla is "" for a mouse event, and Raton is nil for
// a key.
type Entrada struct {
	Tecla Tecla
	Raton *Raton
}

// DecodificarEntradas turns the bytes a terminal in raw mode sends into
// keys and mouse events, in order. The keys are those of Decodificar. A
// mouse event is a sequence of the SGR format (the one Run asks for with
// SecMouseSi): ESC [ < b ; x ; y and a final M (pressed) or m (released),
// b, x and y decimal numbers, x and y counted from 1. It decodes:
//
//   - final M, b & 64 == 0, b & 32 == 0 and b & 3 == 0: RatonClic (the left
//     button);
//   - final M, b & 64 != 0, b & 32 == 0: RatonRuedaArriba when b & 3 == 0,
//     RatonRuedaAbajo when b & 3 == 1;
//
// with X = x - 1 and Y = y - 1. The bits 4, 8 and 16 of b (shift, meta and
// control held) do not change what it is. Any other SGR mouse sequence (a
// release, the middle or the right button, a move, wheel buttons 6 and 7)
// and one whose numbers do not read is dropped whole: no key and no
// event. So is ESC [ M and the three bytes after it (the X10 format).
func DecodificarEntradas(b []byte) []Entrada {
	return nil
}
