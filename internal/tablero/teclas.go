package tablero

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tecla is one key as the model gets it: a printable key is its own text
// (Tecla("j"), Tecla("A"), Tecla(" "), Tecla("ñ")), and the others are the
// constants below.
type Tecla string

// The keys that are not text.
const (
	TeclaEnter    Tecla = "enter"
	TeclaEsc      Tecla = "esc"
	TeclaTab      Tecla = "tab"
	TeclaShiftTab Tecla = "shift-tab"
	TeclaArriba   Tecla = "up"
	TeclaAbajo    Tecla = "down"
	TeclaIzq      Tecla = "left"
	TeclaDer      Tecla = "right"
	TeclaBorrar   Tecla = "backspace"
	TeclaCtrlC    Tecla = "ctrl-c"
	TeclaCtrlU    Tecla = "ctrl-u"
	TeclaCtrlE    Tecla = "ctrl-e"
	// TeclaPgArriba and TeclaPgAbajo are page up and page down.
	TeclaPgArriba Tecla = "pgup"
	TeclaPgAbajo  Tecla = "pgdn"
	// TeclaEspacio is the space bar: a printable key like any other.
	TeclaEspacio Tecla = " "
	// TeclaTexto stands, in Atajos only, for any printable key that a text
	// field takes (the search, the Pedido, a selector turned text). The
	// model never gets it.
	TeclaTexto Tecla = "texto"
)

// Decodificar turns the bytes a terminal in raw mode sends into keys: the
// keys of DecodificarEntradas, in order, without its mouse events. It
// knows: 0x03 ctrl-c, 0x05 ctrl-e, 0x15 ctrl-u, 0x09 tab, 0x0d and 0x0a
// enter, 0x7f and 0x08 backspace; ESC [ A/B/C/D and ESC O A/B/C/D the
// arrows, ESC [ Z shift-tab, ESC [ 5 ~ pgup and ESC [ 6 ~ pgdn; an ESC
// that starts no sequence it knows is TeclaEsc (a lone ESC at the end
// included), and the rest of an unknown CSI sequence (ESC [ ... final
// byte) is dropped; a mouse event in the SGR format (ESC [ < ... M or m,
// see DecodificarEntradas) is no key; ESC [ M and the three bytes after it
// (a mouse event in the old X10 format, from a terminal that does not speak
// SGR) are dropped whole, so a click there never reads as keys; any other
// control byte is dropped; valid UTF-8 runes are themselves; an invalid
// byte is dropped. The buffer is taken as complete: Run waits for the rest
// of a sequence before it decodes (an ESC [ M with fewer than three bytes
// after it, or an ESC [ < sequence without its final M or m, is waited
// for like any other).
func Decodificar(b []byte) []Tecla {
	var out []Tecla
	for _, e := range DecodificarEntradas(b) {
		if e.Raton == nil {
			out = append(out, e.Tecla)
		}
	}
	return out
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
	var out []Entrada
	tecla := func(t Tecla) { out = append(out, Entrada{Tecla: t}) }
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			t, r, n := escape(b[i:])
			switch {
			case r != nil:
				out = append(out, Entrada{Raton: r})
			case t != "":
				tecla(t)
			}
			i += n
			continue
		case c == 0x03:
			tecla(TeclaCtrlC)
		case c == 0x05:
			tecla(TeclaCtrlE)
		case c == 0x15:
			tecla(TeclaCtrlU)
		case c == 0x09:
			tecla(TeclaTab)
		case c == 0x0d || c == 0x0a:
			tecla(TeclaEnter)
		case c == 0x7f || c == 0x08:
			tecla(TeclaBorrar)
		case c < 0x20:
			// another control byte: no key of the table
		default:
			r, n := utf8.DecodeRune(b[i:])
			if r != utf8.RuneError || n > 1 {
				if unicode.IsPrint(r) {
					tecla(Tecla(string(r)))
				}
			}
			i += n
			continue
		}
		i++
	}
	return out
}

// escape decodes the sequence that starts at b[0] (an ESC): the key or the
// mouse event, and how many bytes it took. An unknown CSI sequence is
// neither.
func escape(b []byte) (Tecla, *Raton, int) {
	if len(b) < 2 {
		return TeclaEsc, nil, 1
	}
	switch b[1] {
	case 'O':
		if len(b) >= 3 {
			if t, ok := flechas[b[2]]; ok {
				return t, nil, 3
			}
		}
		return TeclaEsc, nil, 1
	case '[':
		// the X10 mouse: ESC [ M and three bytes, dropped whole
		if len(b) >= 3 && b[2] == 'M' {
			if len(b) >= 6 {
				return "", nil, 6
			}
			return "", nil, len(b)
		}
		// the SGR mouse: ESC [ < and its numbers up to M or m, whole (a
		// number that does not read drops it too)
		if len(b) >= 3 && b[2] == '<' {
			j := 3
			for j < len(b) && b[j] != 'M' && b[j] != 'm' && b[j] >= 0x20 && b[j] < 0x7f {
				j++
			}
			if j < len(b) && (b[j] == 'M' || b[j] == 'm') {
				return "", raton(string(b[3:j]), b[j]), j + 1
			}
			if j >= len(b) {
				return "", nil, len(b)
			}
			return "", nil, j
		}
		// CSI: parameters and intermediates (0x20-0x3f), then a final byte
		// (0x40-0x7e)
		j := 2
		for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3f {
			j++
		}
		if j >= len(b) || b[j] < 0x40 || b[j] > 0x7e {
			return TeclaEsc, nil, 1 // not a whole sequence: the ESC alone
		}
		params := string(b[2:j])
		final := b[j]
		if j == 2 {
			if t, ok := flechas[final]; ok {
				return t, nil, j + 1
			}
			if final == 'Z' {
				return TeclaShiftTab, nil, j + 1
			}
		}
		if final == '~' {
			switch params {
			case "5":
				return TeclaPgArriba, nil, j + 1
			case "6":
				return TeclaPgAbajo, nil, j + 1
			}
		}
		return "", nil, j + 1
	}
	return TeclaEsc, nil, 1
}

// raton is the mouse event of an SGR sequence's "b;x;y" and its final
// byte, or nil for one the model does not take.
func raton(params string, final byte) *Raton {
	f := strings.Split(params, ";")
	if len(f) != 3 || final != 'M' {
		return nil
	}
	var n [3]int
	for i, s := range f {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 || (i > 0 && v < 1) {
			return nil
		}
		n[i] = v
	}
	b, x, y := n[0], n[1]-1, n[2]-1
	if b&32 != 0 {
		return nil
	}
	switch {
	case b&64 == 0 && b&3 == 0:
		return &Raton{Accion: RatonClic, X: x, Y: y}
	case b&64 != 0 && b&128 == 0 && b&3 == 0:
		return &Raton{Accion: RatonRuedaArriba, X: x, Y: y}
	case b&64 != 0 && b&128 == 0 && b&3 == 1:
		return &Raton{Accion: RatonRuedaAbajo, X: x, Y: y}
	}
	return nil
}

// flechas are the final bytes of the arrows (CSI and SS3 alike).
var flechas = map[byte]Tecla{'A': TeclaArriba, 'B': TeclaAbajo, 'C': TeclaDer, 'D': TeclaIzq}

// incompleta says whether b ends inside an escape sequence the decoder
// would read as more than an ESC (a key's, a mouse event's, an X10 one's),
// or inside a UTF-8 character (a read can cut a pasted text anywhere): Run
// waits a moment for the rest.
func incompleta(b []byte) bool {
	// the last character's start: at most 3 continuation bytes back
	for j := len(b) - 1; j >= 0 && j >= len(b)-utf8.UTFMax; j-- {
		if utf8.RuneStart(b[j]) {
			if !utf8.FullRune(b[j:]) {
				return true
			}
			break
		}
	}
	i := bytes.LastIndexByte(b, 0x1b)
	if i < 0 {
		return false
	}
	resto := b[i:]
	switch {
	case len(resto) == 1:
		return true
	case resto[1] == 'O':
		return len(resto) == 2
	case resto[1] == '[':
		if len(resto) >= 3 && resto[2] == 'M' {
			return len(resto) < 6
		}
		if len(resto) >= 3 && resto[2] == '<' {
			for _, c := range resto[3:] {
				if c == 'M' || c == 'm' || c < 0x20 || c >= 0x7f {
					return false
				}
			}
			return true
		}
		for _, c := range resto[2:] {
			if c >= 0x40 && c <= 0x7e {
				return false
			}
			if c < 0x20 || c > 0x3f {
				return false
			}
		}
		return true
	}
	return false
}

// AccionSiguiente is the Accion of n: the card's next step (see the
// package doc, "The next step"). n opens the modal of that action and runs
// that action's verb.
const AccionSiguiente = "siguiente"

// AccionUsar is the Accion of u: hoom usar, which every card has. It is the
// only action key that runs without a modal.
const AccionUsar = "usar"
