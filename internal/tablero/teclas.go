package tablero

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
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
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			t, n := escape(b[i:])
			if t != "" {
				out = append(out, t)
			}
			i += n
			continue
		case c == 0x03:
			out = append(out, TeclaCtrlC)
		case c == 0x05:
			out = append(out, TeclaCtrlE)
		case c == 0x15:
			out = append(out, TeclaCtrlU)
		case c == 0x09:
			out = append(out, TeclaTab)
		case c == 0x0d || c == 0x0a:
			out = append(out, TeclaEnter)
		case c == 0x7f || c == 0x08:
			out = append(out, TeclaBorrar)
		case c < 0x20:
			// another control byte: no key of the table
		default:
			r, n := utf8.DecodeRune(b[i:])
			if r != utf8.RuneError || n > 1 {
				if unicode.IsPrint(r) {
					out = append(out, Tecla(string(r)))
				}
			}
			i += n
			continue
		}
		i++
	}
	return out
}

// escape decodes the sequence that starts at b[0] (an ESC): the key and
// how many bytes it took. An unknown CSI sequence is no key.
func escape(b []byte) (Tecla, int) {
	if len(b) < 2 {
		return TeclaEsc, 1
	}
	switch b[1] {
	case 'O':
		if len(b) >= 3 {
			if t, ok := flechas[b[2]]; ok {
				return t, 3
			}
		}
		return TeclaEsc, 1
	case '[':
		// CSI: parameters and intermediates (0x20-0x3f), then a final byte
		// (0x40-0x7e)
		j := 2
		for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3f {
			j++
		}
		if j >= len(b) || b[j] < 0x40 || b[j] > 0x7e {
			return TeclaEsc, 1 // not a whole sequence: the ESC alone
		}
		if j == 2 {
			if t, ok := flechas[b[j]]; ok {
				return t, j + 1
			}
			if b[j] == 'Z' {
				return TeclaShiftTab, j + 1
			}
		}
		return "", j + 1
	}
	return TeclaEsc, 1
}

// flechas are the final bytes of the arrows (CSI and SS3 alike).
var flechas = map[byte]Tecla{'A': TeclaArriba, 'B': TeclaAbajo, 'C': TeclaDer, 'D': TeclaIzq}

// incompleta says whether b ends inside an escape sequence Decodificar
// would read as more than an ESC, or inside a UTF-8 character (a read can
// cut a pasted text anywhere): Run waits a moment for the rest.
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

// vistaV1 is what the screen shows; keys mean what the atajosV1 of the current
// vistaV1 say.
type vistaV1 string

// The views. The open card is one view per tab.
const (
	vistaLista        vistaV1 = "lista"
	vistaBusqueda     vistaV1 = "busqueda" // the list while / is open
	vistaResumen      vistaV1 = "resumen"
	vistaSpec         vistaV1 = "spec"
	vistaSobre        vistaV1 = "sobre"
	vistaHallazgos    vistaV1 = "hallazgos"
	vistaHistoria     vistaV1 = "historia"
	vistaMenu         vistaV1 = "menu"
	vistaFormulario   vistaV1 = "formulario"
	vistaConfirmacion vistaV1 = "confirmacion"
	vistaAyuda        vistaV1 = "ayuda"
)

// pestanasV1 are the open card's views, in tab order (keys 1 to 5).
var pestanasV1 = []vistaV1{vistaResumen, vistaSpec, vistaSobre, vistaHallazgos, vistaHistoria}

// AccionSiguiente is the Accion of n: the card's next step (see the
// package doc, "The next step"). n opens the modal of that action and runs
// that action's verb.
const AccionSiguiente = "siguiente"

// AccionUsar is the Accion of u: hoom usar, which every card has. It is the
// only action key that runs without a modal.
const AccionUsar = "usar"

// atajoV1 is one row of the key table: the keys, how the help writes them,
// what they do, the verb they run and where they work.
type atajoV1 struct {
	Teclas  []Tecla
	Muestra string // the keys as the help writes them
	Hace    string // what they do, as the help says it
	Verbo   string // the capa-1 verb they run, with <slug> ("" = none)
	// Accion is the card action they run: a boardcmd.Act* id,
	// AccionSiguiente or AccionUsar ("" = none).
	Accion string
	Barra  string // its hint in bar 2 ("" = not in the bar)
	Vistas []vistaV1
}

// tarjetaVistas are the views where the card's action keys work: the list,
// every tab of the open card and the menu.
var tarjetaVistas = []vistaV1{vistaLista, vistaResumen, vistaSpec, vistaSobre, vistaHallazgos, vistaHistoria, vistaMenu}

// pestanaVistas are the open card's tabs.
var pestanaVistas = []vistaV1{vistaResumen, vistaSpec, vistaSobre, vistaHallazgos, vistaHistoria}

// atajosV1 is THE key table: the help lists it and the model dispatches from
// it, so no key works without being in the help, nor the other way round
// (CA-532). Its order is the help's and the bar's.
var atajosV1 = []atajoV1{
	// the list
	{Teclas: []Tecla{"j", TeclaAbajo, "k", TeclaArriba}, Muestra: "j k ↓ ↑", Hace: "moverse", Barra: "j/k moverse",
		Vistas: []vistaV1{vistaLista, vistaMenu}},
	{Teclas: []Tecla{"g", "G"}, Muestra: "g G", Hace: "primera, ultima", Vistas: []vistaV1{vistaLista}},
	{Teclas: []Tecla{TeclaEnter, "l", TeclaDer}, Muestra: "enter l →", Hace: "abrir la tarjeta", Barra: "enter abrir",
		Vistas: []vistaV1{vistaLista}},
	// the open card
	{Teclas: []Tecla{"1", "2", "3", "4", "5", TeclaTab}, Muestra: "1-5 tab", Hace: "elegir la pestana", Barra: "1-5 pestanas",
		Vistas: pestanaVistas},
	{Teclas: []Tecla{"j", TeclaAbajo, "k", TeclaArriba}, Muestra: "j k ↓ ↑", Hace: "desplazarse",
		Vistas: []vistaV1{vistaResumen, vistaSpec, vistaSobre, vistaHistoria}},
	{Teclas: []Tecla{"j", TeclaAbajo, "k", TeclaArriba}, Muestra: "j k ↓ ↑", Hace: "moverse entre los hallazgos",
		Vistas: []vistaV1{vistaHallazgos}},
	{Teclas: []Tecla{"g", "G"}, Muestra: "g G", Hace: "principio, final", Vistas: pestanaVistas},
	{Teclas: []Tecla{TeclaEsc, "h", TeclaIzq}, Muestra: "esc h ←", Hace: "volver a la lista", Barra: "esc volver",
		Vistas: pestanaVistas},
	// ]c is "]" and then "c": a key of two strokes, like in Vim
	{Teclas: []Tecla{"]c", "[c"}, Muestra: "]c [c", Hace: "siguiente y anterior criterio sin prueba", Barra: "]c [c sin prueba",
		Vistas: []vistaV1{vistaSpec}},
	{Teclas: []Tecla{"o"}, Muestra: "o", Hace: "abrir el spec en el editor", Barra: "o editor", Vistas: []vistaV1{vistaSpec}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "abrir el hallazgo en el editor", Barra: "enter editor",
		Vistas: []vistaV1{vistaHallazgos}},
	// the card's actions
	{Teclas: []Tecla{TeclaEspacio}, Muestra: "espacio", Hace: "menu de acciones de la tarjeta", Barra: "espacio acciones",
		Vistas: []vistaV1{vistaLista, vistaResumen, vistaSpec, vistaSobre, vistaHallazgos, vistaHistoria}},
	{Teclas: []Tecla{"n"}, Muestra: "n", Hace: "el siguiente paso", Verbo: "hoom siguiente <slug>", Accion: AccionSiguiente,
		Barra: "n siguiente", Vistas: tarjetaVistas},
	{Teclas: []Tecla{"a"}, Muestra: "a", Hace: "pedir al arquitecto", Verbo: "hoom pedir arquitecto <slug>",
		Accion: boardcmd.ActPedirArquitecto, Vistas: tarjetaVistas},
	{Teclas: []Tecla{"t"}, Muestra: "t", Hace: "pedir al test-writer", Verbo: "hoom pedir test-writer <slug>",
		Accion: boardcmd.ActPedirTestWriter, Vistas: tarjetaVistas},
	{Teclas: []Tecla{"w"}, Muestra: "w", Hace: "pedir al writer", Verbo: "hoom pedir writer <slug>",
		Accion: boardcmd.ActPedirWriter, Vistas: tarjetaVistas},
	{Teclas: []Tecla{"r"}, Muestra: "r", Hace: "pedir al reviewer", Verbo: "hoom pedir reviewer <slug>",
		Accion: boardcmd.ActPedirReviewer, Vistas: tarjetaVistas},
	{Teclas: []Tecla{"R"}, Muestra: "R", Hace: "reanudar", Verbo: "hoom reanudar <slug>", Accion: boardcmd.ActReanudar,
		Vistas: tarjetaVistas},
	{Teclas: []Tecla{"s"}, Muestra: "s", Hace: "guardar", Verbo: "hoom guardar <slug>", Accion: boardcmd.ActGuardar,
		Vistas: tarjetaVistas},
	{Teclas: []Tecla{"A"}, Muestra: "A", Hace: "aprobar el spec", Verbo: "hoom aprobar <slug> --si", Accion: boardcmd.ActAprobar,
		Vistas: tarjetaVistas},
	{Teclas: []Tecla{"I"}, Muestra: "I", Hace: "integrar", Verbo: "hoom integrar <slug> --si", Accion: boardcmd.ActIntegrar,
		Vistas: tarjetaVistas},
	{Teclas: []Tecla{"u"}, Muestra: "u", Hace: "fijar la tarjeta para la terminal", Verbo: "hoom usar <slug>",
		Accion: AccionUsar, Vistas: tarjetaVistas},
	// the menu
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "hacer la accion elegida", Barra: "enter hacer",
		Vistas: []vistaV1{vistaMenu}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "cerrar el menu", Barra: "esc cerrar", Vistas: []vistaV1{vistaMenu}},
	// the search
	{Teclas: []Tecla{"/"}, Muestra: "/", Hace: "buscar", Barra: "/ buscar", Vistas: []vistaV1{vistaLista}},
	{Teclas: []Tecla{TeclaTexto, TeclaBorrar}, Muestra: "texto", Hace: "filtrar por slug o titulo",
		Vistas: []vistaV1{vistaBusqueda}},
	{Teclas: []Tecla{TeclaAbajo, TeclaArriba}, Muestra: "↓ ↑", Hace: "moverse", Barra: "↓/↑ moverse",
		Vistas: []vistaV1{vistaBusqueda}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "abrir la tarjeta", Barra: "enter abrir",
		Vistas: []vistaV1{vistaBusqueda}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "cerrar la busqueda", Barra: "esc cerrar", Vistas: []vistaV1{vistaBusqueda}},
	// the launch form
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "lanzar", Barra: "enter lanzar", Vistas: []vistaV1{vistaFormulario}},
	{Teclas: []Tecla{TeclaTab, TeclaShiftTab}, Muestra: "tab shift-tab", Hace: "siguiente y anterior campo",
		Barra: "tab campos", Vistas: []vistaV1{vistaFormulario}},
	{Teclas: []Tecla{TeclaIzq, TeclaDer}, Muestra: "← →", Hace: "en Provider, cambiar la opcion; en un texto, mover el cursor",
		Vistas: []vistaV1{vistaFormulario}},
	{Teclas: []Tecla{TeclaTexto, TeclaBorrar}, Muestra: "texto", Hace: "escribir en el campo", Vistas: []vistaV1{vistaFormulario}},
	{Teclas: []Tecla{TeclaCtrlU}, Muestra: "ctrl-u", Hace: "vaciar el campo", Vistas: []vistaV1{vistaFormulario}},
	{Teclas: []Tecla{TeclaCtrlE}, Muestra: "ctrl-e", Hace: "abrir el pedido en el editor", Barra: "ctrl-e editor",
		Vistas: []vistaV1{vistaFormulario}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "cancelar sin lanzar", Barra: "esc cancelar",
		Vistas: []vistaV1{vistaFormulario}},
	// the confirmations
	{Teclas: []Tecla{TeclaIzq, TeclaDer, TeclaTab}, Muestra: "← → tab", Hace: "elegir Firmar o Cancelar", Barra: "←/→ elegir",
		Vistas: []vistaV1{vistaConfirmacion}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "confirmar", Barra: "enter confirmar", Vistas: []vistaV1{vistaConfirmacion}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "cancelar", Barra: "esc cancelar", Vistas: []vistaV1{vistaConfirmacion}},
	// the help
	{Teclas: []Tecla{TeclaEsc, "?"}, Muestra: "esc ?", Hace: "cerrar la ayuda", Barra: "esc cerrar", Vistas: []vistaV1{vistaAyuda}},
	// everywhere
	{Teclas: []Tecla{"x"}, Muestra: "x", Hace: "modo experto", Vistas: []vistaV1{vistaLista, vistaResumen, vistaSpec, vistaSobre,
		vistaHallazgos, vistaHistoria, vistaMenu}},
	{Teclas: []Tecla{"?"}, Muestra: "?", Hace: "ayuda", Barra: "? ayuda", Vistas: []vistaV1{vistaLista, vistaResumen, vistaSpec,
		vistaSobre, vistaHallazgos, vistaHistoria, vistaMenu}},
	{Teclas: []Tecla{"q", TeclaCtrlC}, Muestra: "q ctrl-c", Hace: "salir", Vistas: []vistaV1{vistaLista, vistaResumen, vistaSpec,
		vistaSobre, vistaHallazgos, vistaHistoria, vistaMenu, vistaAyuda}},
	{Teclas: []Tecla{TeclaCtrlC}, Muestra: "ctrl-c", Hace: "salir", Vistas: []vistaV1{vistaBusqueda, vistaFormulario, vistaConfirmacion}},
}

// atajosV1De are the rows of atajosV1 that work in v, in table order.
func atajosV1De(v vistaV1) []atajoV1 {
	var out []atajoV1
	for _, a := range atajosV1 {
		if contieneVista(a.Vistas, v) {
			out = append(out, a)
		}
	}
	return out
}

func contieneVista(vs []vistaV1, v vistaV1) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}

// atajo is the row of the table that t has in v: the one that names it or,
// for a printable key that no row names, the row of TeclaTexto.
func atajo(v vistaV1, t Tecla) (atajoV1, bool) {
	filas := atajosV1De(v)
	for _, a := range filas {
		for _, k := range a.Teclas {
			if k == t {
				return a, true
			}
		}
	}
	if !imprimible(t) {
		return atajoV1{}, false
	}
	for _, a := range filas {
		for _, k := range a.Teclas {
			if k == TeclaTexto {
				return a, true
			}
		}
	}
	return atajoV1{}, false
}

// imprimible: a key that is one printable rune (the space included).
func imprimible(t Tecla) bool {
	r, n := utf8.DecodeRuneInString(string(t))
	return n == len(t) && n > 0 && r != utf8.RuneError && unicode.IsPrint(r)
}
