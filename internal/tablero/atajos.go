package tablero

import "github.com/hoomdev/hoomai/internal/boardcmd"

// Panel is one of the five panels that take the focus, by its number: [0]
// the main panel, [1] Proyecto, [2] Tarjetas, [3] Corriendo and [4]
// Hallazgos. The Registro is a panel too, but it never takes the focus.
type Panel int

// The panels, with their number.
const (
	PanelPrincipal Panel = 0 // [0]: what was chosen in the last left panel with the focus
	PanelProyecto  Panel = 1 // [1] Proyecto
	PanelTarjetas  Panel = 2 // [2] Tarjetas
	PanelCorriendo Panel = 3 // [3] Corriendo
	PanelHallazgos Panel = 4 // [4] Hallazgos
)

// Modal is the modal that is open over the panels ("" = none).
type Modal string

// The modals.
const (
	SinModal      Modal = ""
	ModalPedir    Modal = "pedir"    // a, t, w, r, R, and n when the next step launches a role
	ModalFirma    Modal = "firma"    // A, I, and n when the next step is a signature
	ModalGuardar  Modal = "guardar"  // s, and n when the next step is guardar
	ModalAcciones Modal = "acciones" // espacio
	ModalAyuda    Modal = "ayuda"    // ?
)

// Lugar is where the keys are now: the panel with the focus (and, for [0],
// what it shows), the search, or the open modal and the part of it with the
// focus. Each row of Atajos says the Lugares where its keys work; a key
// with no row in the current Lugar does nothing (Modelo.Lugar says which
// Lugar is current).
type Lugar string

// The Lugares.
const (
	LugarProyecto  Lugar = "proyecto"  // [1] with the focus
	LugarTarjetas  Lugar = "tarjetas"  // [2] with the focus
	LugarCorriendo Lugar = "corriendo" // [3] with the focus
	LugarHallazgos Lugar = "hallazgos" // [4] with the focus

	LugarVerProyecto  Lugar = "ver-proyecto"  // [0] with the focus, showing Proyecto
	LugarVerTarjeta   Lugar = "ver-tarjeta"   // [0] with the focus, showing the card chosen in [2]
	LugarVerCorriendo Lugar = "ver-corriendo" // [0] with the focus, showing the card chosen in [3]
	LugarVerHallazgo  Lugar = "ver-hallazgo"  // [0] with the focus, showing the finding chosen in [4]
	LugarVerNada      Lugar = "ver-nada"      // [0] with the focus, with nothing chosen to show

	LugarBusqueda Lugar = "busqueda" // [2] while the search (/) is open

	LugarPedirSelector Lugar = "pedir-selector" // the Pedir modal, the focus on a selector
	LugarPedirPedido   Lugar = "pedir-pedido"   // the Pedir modal, the focus on the Pedido
	LugarPedirTexto    Lugar = "pedir-texto"    // the Pedir modal, a selector turned text by otro…
	LugarPedirLista    Lugar = "pedir-lista"    // the Pedir modal, a selector's list of options open
	LugarPedirBotones  Lugar = "pedir-botones"  // the Pedir modal, the focus on its buttons
	LugarFirma         Lugar = "firma"          // the Firma modal (only buttons)
	LugarGuardar       Lugar = "guardar"        // the Guardar modal (only buttons)
	LugarAcciones      Lugar = "acciones"       // the Acciones modal
	LugarAyuda         Lugar = "ayuda"          // the Ayuda modal
)

// The sets of Lugares the rows of Atajos use.
var (
	// LugaresPaneles are the Lugares without a modal and without the
	// search: "siempre" in the spec's key table.
	LugaresPaneles = []Lugar{LugarProyecto, LugarTarjetas, LugarCorriendo, LugarHallazgos,
		LugarVerProyecto, LugarVerTarjeta, LugarVerCorriendo, LugarVerHallazgo, LugarVerNada}
	// LugaresVer are [0]'s.
	LugaresVer = []Lugar{LugarVerProyecto, LugarVerTarjeta, LugarVerCorriendo, LugarVerHallazgo, LugarVerNada}
	// LugaresVerAlgo are [0]'s with something to show: the keys that
	// scroll it work there (in LugarVerNada they would have no effect).
	LugaresVerAlgo = []Lugar{LugarVerProyecto, LugarVerTarjeta, LugarVerCorriendo, LugarVerHallazgo}
	// LugaresConTarjeta are "con tarjeta" in the spec: [2] on a card, [3]
	// (the card of the row) and [0] showing a card (chosen in [2] or in
	// [3]). In [2] on Hecho's line and in an empty [3] there is no card:
	// the rows of the card do nothing there.
	LugaresConTarjeta = []Lugar{LugarTarjetas, LugarCorriendo, LugarVerTarjeta, LugarVerCorriendo}
	// LugaresAccion are where the action keys work: LugaresConTarjeta and
	// the Acciones modal.
	LugaresAccion = []Lugar{LugarTarjetas, LugarCorriendo, LugarVerTarjeta, LugarVerCorriendo, LugarAcciones}
	// LugaresPedir are the Pedir modal's.
	LugaresPedir = []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirTexto, LugarPedirLista, LugarPedirBotones}
	// LugaresModales are the Lugares of every modal.
	LugaresModales = []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirTexto, LugarPedirLista, LugarPedirBotones,
		LugarFirma, LugarGuardar, LugarAcciones, LugarAyuda}
	// LugaresTodos are every Lugar: the panels, the search and the modals.
	LugaresTodos = []Lugar{LugarProyecto, LugarTarjetas, LugarCorriendo, LugarHallazgos,
		LugarVerProyecto, LugarVerTarjeta, LugarVerCorriendo, LugarVerHallazgo, LugarVerNada, LugarBusqueda,
		LugarPedirSelector, LugarPedirPedido, LugarPedirTexto, LugarPedirLista, LugarPedirBotones,
		LugarFirma, LugarGuardar, LugarAcciones, LugarAyuda}
)

// Atajo is one row of the key table: the keys, how the help writes them,
// what they do, the verb they run, the card action they run, their hint on
// the keys line and the Lugares where they work.
type Atajo struct {
	Teclas []Tecla
	// Muestra is the keys as the help writes them, with UTF-8 arrows (in
	// ASCII the help writes ↑ ↓ ← → as ^ v < >).
	Muestra string
	// Hace is what they do, as the help says it (no accents, in both modes).
	Hace string
	// Verbo is the capa-1 verb they run, with <slug> for the card ("" =
	// none): what the help shows, and what runs.
	Verbo string
	// Accion is the card action they run: a boardcmd.Act* id,
	// AccionSiguiente or AccionUsar ("" = none).
	Accion string
	// Barra is their hint on the keys line ("" = not there), with UTF-8
	// arrows like Muestra.
	Barra string
	// DeTarjeta marks the rows of the card (espacio, n and the action
	// keys): without a card ([2] on Hecho's line, an empty [3], the Acciones
	// of a card gone) their hints are not on the keys line.
	DeTarjeta bool
	Lugares   []Lugar
}

// Atajos is THE key table: the keys line, the help and the dispatch come
// from it, so no key does something without being in the help, nor the
// other way round (CA-564). Its order is the keys line's and the help's.
var Atajos = []Atajo{
	// [1] to [4]
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo, "k", "j"}, Muestra: "↑ ↓ k j", Hace: "elegir", Barra: "↑↓ elegir",
		Lugares: []Lugar{LugarTarjetas, LugarCorriendo, LugarHallazgos}},
	{Teclas: []Tecla{"g", "G"}, Muestra: "g G", Hace: "primera o ultima",
		Lugares: []Lugar{LugarTarjetas, LugarCorriendo, LugarHallazgos}},
	{Teclas: []Tecla{TeclaEnter, TeclaDer, "l"}, Muestra: "enter → l", Hace: "ver en grande (en Hecho, abrir o cerrar)",
		Barra: "enter ver", Lugares: []Lugar{LugarProyecto, LugarTarjetas, LugarCorriendo, LugarHallazgos}},
	// [0]
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo, "k", "j"}, Muestra: "↑ ↓ k j", Hace: "desplazar", Barra: "↑↓ desplazar",
		Lugares: LugaresVerAlgo},
	{Teclas: []Tecla{"g", "G"}, Muestra: "g G", Hace: "principio o final", Lugares: LugaresVerAlgo},
	{Teclas: []Tecla{TeclaPgArriba, TeclaPgAbajo}, Muestra: "pgup pgdn", Hace: "desplazar una pagina", Lugares: LugaresVerAlgo},
	{Teclas: []Tecla{TeclaEsc, TeclaIzq, "h"}, Muestra: "esc ← h", Hace: "volver al panel de donde vino", Barra: "esc volver",
		Lugares: LugaresVer},
	// the card
	{Teclas: []Tecla{TeclaEspacio}, Muestra: "espacio", Hace: "acciones de la tarjeta", Barra: "espacio acciones",
		DeTarjeta: true, Lugares: LugaresConTarjeta},
	{Teclas: []Tecla{"n"}, Muestra: "n", Hace: "el siguiente paso", Accion: AccionSiguiente, Barra: "n siguiente paso",
		DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"a"}, Muestra: "a", Hace: "pedir al arquitecto", Verbo: "hoom pedir arquitecto <slug>",
		Accion: boardcmd.ActPedirArquitecto, DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"t"}, Muestra: "t", Hace: "pedir al test-writer", Verbo: "hoom pedir test-writer <slug>",
		Accion: boardcmd.ActPedirTestWriter, DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"w"}, Muestra: "w", Hace: "pedir al writer", Verbo: "hoom pedir writer <slug>",
		Accion: boardcmd.ActPedirWriter, DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"r"}, Muestra: "r", Hace: "pedir al reviewer", Verbo: "hoom pedir reviewer <slug>",
		Accion: boardcmd.ActPedirReviewer, DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"R"}, Muestra: "R", Hace: "reanudar", Verbo: "hoom reanudar <slug>", Accion: boardcmd.ActReanudar,
		DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"s"}, Muestra: "s", Hace: "guardar", Verbo: "hoom guardar <slug>", Accion: boardcmd.ActGuardar,
		DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"A"}, Muestra: "A", Hace: "aprobar el spec", Verbo: "hoom aprobar <slug> --si", Accion: boardcmd.ActAprobar,
		DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"I"}, Muestra: "I", Hace: "integrar", Verbo: "hoom integrar <slug> --si", Accion: boardcmd.ActIntegrar,
		DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"u"}, Muestra: "u", Hace: "fijar la tarjeta para la terminal", Verbo: "hoom usar <slug>",
		Accion: AccionUsar, DeTarjeta: true, Lugares: LugaresAccion},
	{Teclas: []Tecla{"e"}, Muestra: "e", Hace: "abrir en el editor", Barra: "e editor",
		Lugares: []Lugar{LugarVerTarjeta, LugarHallazgos, LugarVerHallazgo}},
	// ]c is "]" and then "c": a key of two strokes, like in Vim
	{Teclas: []Tecla{"]c", "[c"}, Muestra: "]c [c", Hace: "criterio sin prueba siguiente o anterior",
		Barra: "]c [c sin prueba", Lugares: []Lugar{LugarVerTarjeta}},
	// the search
	{Teclas: []Tecla{TeclaTexto, TeclaBorrar}, Muestra: "texto", Hace: "filtrar por slug o titulo",
		Lugares: []Lugar{LugarBusqueda}},
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo}, Muestra: "↑ ↓", Hace: "elegir", Barra: "↑↓ elegir",
		Lugares: []Lugar{LugarBusqueda}},
	{Teclas: []Tecla{TeclaEnter, TeclaEsc}, Muestra: "enter esc", Hace: "cerrar la busqueda", Barra: "enter esc cerrar",
		Lugares: []Lugar{LugarBusqueda}},
	// the Pedir modal
	{Teclas: []Tecla{TeclaTab, TeclaShiftTab}, Muestra: "tab shift-tab", Hace: "campo o boton siguiente o anterior",
		Barra: "tab campo", Lugares: []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirBotones}},
	{Teclas: []Tecla{TeclaIzq, TeclaDer}, Muestra: "← →", Hace: "opcion anterior o siguiente, saltando las grises",
		Barra: "←→ cambiar", Lugares: []Lugar{LugarPedirSelector}},
	{Teclas: []Tecla{TeclaEnter, TeclaEspacio}, Muestra: "enter espacio", Hace: "abrir la lista de opciones",
		Barra: "enter ver opciones", Lugares: []Lugar{LugarPedirSelector}},
	{Teclas: []Tecla{TeclaIzq, TeclaDer}, Muestra: "← →", Hace: "mover el cursor",
		Lugares: []Lugar{LugarPedirPedido, LugarPedirTexto}},
	{Teclas: []Tecla{TeclaTexto, TeclaBorrar}, Muestra: "texto", Hace: "escribir, borrar",
		Lugares: []Lugar{LugarPedirPedido, LugarPedirTexto}},
	{Teclas: []Tecla{TeclaCtrlU}, Muestra: "ctrl-u", Hace: "vaciar", Lugares: []Lugar{LugarPedirPedido, LugarPedirTexto}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "ir al boton de la accion", Barra: "enter al boton",
		Lugares: []Lugar{LugarPedirPedido}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "confirmar el texto", Barra: "enter confirmar",
		Lugares: []Lugar{LugarPedirTexto}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "volver a la opcion de antes", Barra: "esc volver",
		Lugares: []Lugar{LugarPedirTexto}},
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo}, Muestra: "↑ ↓", Hace: "moverse por las opciones", Barra: "↑↓ elegir",
		Lugares: []Lugar{LugarPedirLista}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "elegir la opcion", Barra: "enter elegir",
		Lugares: []Lugar{LugarPedirLista}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "cerrar la lista sin cambiar", Barra: "esc cerrar",
		Lugares: []Lugar{LugarPedirLista}},
	{Teclas: []Tecla{TeclaIzq, TeclaDer}, Muestra: "← →", Hace: "boton anterior o siguiente", Barra: "←→ boton",
		Lugares: []Lugar{LugarPedirBotones}},
	// the Firma and Guardar modals
	{Teclas: []Tecla{TeclaIzq, TeclaDer, TeclaTab, TeclaShiftTab}, Muestra: "← → tab shift-tab",
		Hace: "boton anterior o siguiente", Barra: "←→ boton", Lugares: []Lugar{LugarFirma, LugarGuardar}},
	// the modals that run a verb
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "ejecutar la accion o cancelar, segun el boton elegido",
		Barra: "enter ejecutar", Lugares: []Lugar{LugarPedirBotones, LugarFirma, LugarGuardar}},
	{Teclas: []Tecla{TeclaCtrlE}, Muestra: "ctrl-e", Hace: "abrir el pedido en el editor", Barra: "ctrl-e editor",
		Lugares: []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirBotones}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "cerrar sin ejecutar", Barra: "esc cerrar",
		Lugares: []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirBotones, LugarFirma, LugarGuardar, LugarAcciones}},
	// the Acciones modal (and the action keys above)
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo, "k", "j"}, Muestra: "↑ ↓ k j", Hace: "moverse por las acciones que se pueden elegir",
		Barra: "↑↓ elegir", Lugares: []Lugar{LugarAcciones}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "abrir el modal de la accion elegida, o ejecutar u",
		Barra: "enter abrir", Lugares: []Lugar{LugarAcciones}},
	// the Ayuda modal
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo, "k", "j", TeclaPgArriba, TeclaPgAbajo}, Muestra: "↑ ↓ k j pgup pgdn",
		Hace: "desplazar", Barra: "↑↓ desplazar", Lugares: []Lugar{LugarAyuda}},
	{Teclas: []Tecla{TeclaEsc, "?"}, Muestra: "esc ?", Hace: "cerrar la ayuda", Barra: "esc cerrar",
		Lugares: []Lugar{LugarAyuda}},
	// everywhere
	{Teclas: []Tecla{"1", "2", "3", "4", "0"}, Muestra: "1 2 3 4 0", Hace: "foco a ese panel", Lugares: LugaresPaneles},
	{Teclas: []Tecla{TeclaTab, TeclaShiftTab}, Muestra: "tab shift-tab", Hace: "panel siguiente o anterior (1, 2, 3, 4, 0)",
		Lugares: LugaresPaneles},
	{Teclas: []Tecla{"/"}, Muestra: "/", Hace: "buscar tarjetas", Lugares: LugaresPaneles},
	{Teclas: []Tecla{"x"}, Muestra: "x", Hace: "modo experto", Lugares: LugaresPaneles},
	{Teclas: []Tecla{"?"}, Muestra: "?", Hace: "todas las teclas", Barra: "? todas las teclas", Lugares: LugaresPaneles},
	{Teclas: []Tecla{"q"}, Muestra: "q", Hace: "salir", Lugares: LugaresPaneles},
	{Teclas: []Tecla{TeclaCtrlC}, Muestra: "ctrl-c", Hace: "salir", Lugares: LugaresTodos},
}
