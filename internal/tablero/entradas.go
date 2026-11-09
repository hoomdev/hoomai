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
