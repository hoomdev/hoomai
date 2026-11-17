package tablero

import "github.com/hoomdev/hoomai/internal/boardcmd"

// Zona is one of the two zones that take the focus: the list or the
// detail (CA-706). There is always one with the focus; at the start it is
// the list.
type Zona int

// The zones.
const (
	ZonaLista   Zona = 0 // the list: of cards, or of findings in the Hallazgos view
	ZonaDetalle Zona = 1 // the detail: of the chosen card or finding, of the project, or nothing
)

// Vista is what the list shows (CA-712).
type Vista string

// The views.
const (
	VistaTarjetas  Vista = "tarjetas"  // the cards, grouped by who has the ball
	VistaHallazgos Vista = "hallazgos" // the project's open findings (H)
)

// Grupo is the group of a card in the list: who has the ball (CA-700).
type Grupo string

// The groups, in the list's order. Their values are their names on screen.
const (
	GrupoTeToca    Grupo = "TE TOCA"
	GrupoCorriendo Grupo = "CORRIENDO"
	GrupoDetenida  Grupo = "DETENIDA"
	GrupoEnCola    Grupo = "EN COLA"
	GrupoHecho     Grupo = "HECHO"
)

// Modal is the modal that is open over the screen ("" = none).
type Modal string

// The modals.
const (
	SinModal      Modal = ""
	ModalPedir    Modal = "pedir"    // a, t, w, r, R, and enter or n when the step launches a role
	ModalFirma    Modal = "firma"    // A, I, and enter or n when the step is a signature
	ModalGuardar  Modal = "guardar"  // s, and enter or n when the step is guardar
	ModalSeguir   Modal = "seguir"   // P, and enter or n when the step is Seguir solo
	ModalAcciones Modal = "acciones" // espacio
	ModalAyuda    Modal = "ayuda"    // ?
	ModalRegistro Modal = "registro" // L
)

// Lugar is where the keys are now: the zone with the focus and what it
// shows, the search, or the open modal and the part of it with the focus.
// Each row of Atajos says the Lugares where its keys work; a key with no
// row in the current Lugar does nothing (Modelo.Lugar says which Lugar is
// current).
type Lugar string

// The Lugares.
const (
	LugarTarjetas  Lugar = "tarjetas"  // the list of cards with the focus
	LugarHallazgos Lugar = "hallazgos" // the list of the Hallazgos view with the focus

	LugarVerProyecto Lugar = "ver-proyecto" // the detail with the focus, showing the project (v)
	LugarVerTarjeta  Lugar = "ver-tarjeta"  // the detail with the focus, showing the chosen card
	LugarVerHallazgo Lugar = "ver-hallazgo" // the detail with the focus, showing the chosen finding
	LugarVerNada     Lugar = "ver-nada"     // the detail with the focus, with nothing to show

	LugarBusqueda Lugar = "busqueda" // the list of cards while the search (/) is open

	LugarPedirSelector Lugar = "pedir-selector" // the Pedir modal, the focus on a selector
	LugarPedirPedido   Lugar = "pedir-pedido"   // the Pedir modal, the focus on the Pedido
	LugarPedirTexto    Lugar = "pedir-texto"    // the Pedir modal, a selector turned text by otro…
	LugarPedirLista    Lugar = "pedir-lista"    // the Pedir modal, a selector's list of options open
	LugarPedirBotones  Lugar = "pedir-botones"  // the Pedir modal, the focus on its buttons
	LugarFirma         Lugar = "firma"          // the Firma modal (only buttons)
	LugarGuardar       Lugar = "guardar"        // the Guardar modal (only buttons)
	LugarSeguir        Lugar = "seguir"         // the Seguir solo modal (only buttons)
	LugarAcciones      Lugar = "acciones"       // the Acciones modal
	LugarAyuda         Lugar = "ayuda"          // the Ayuda modal
	LugarRegistro      Lugar = "registro"       // the Registro modal
)

// The sets of Lugares the rows of Atajos use.
var (
	// LugaresPaneles are the Lugares without a modal and without the
	// search: the list (of cards or of findings) and the detail.
	LugaresPaneles = []Lugar{LugarTarjetas, LugarHallazgos,
		LugarVerProyecto, LugarVerTarjeta, LugarVerHallazgo, LugarVerNada}
	// LugaresVer are the detail's.
	LugaresVer = []Lugar{LugarVerProyecto, LugarVerTarjeta, LugarVerHallazgo, LugarVerNada}
	// LugaresVerAlgo are the detail's with something to show: the keys that
	// scroll it work there (in LugarVerNada they would have no effect).
	LugaresVerAlgo = []Lugar{LugarVerProyecto, LugarVerTarjeta, LugarVerHallazgo}
	// LugaresConTarjeta are "con tarjeta" in the spec: the list of cards on
	// a card and the detail showing a card. On HECHO's line and in an empty
	// list there is no card: the rows of the card do nothing there.
	LugaresConTarjeta = []Lugar{LugarTarjetas, LugarVerTarjeta}
	// LugaresAccion are where the action keys work: LugaresConTarjeta and
	// the Acciones modal.
	LugaresAccion = []Lugar{LugarTarjetas, LugarVerTarjeta, LugarAcciones}
	// LugaresPedir are the Pedir modal's.
	LugaresPedir = []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirTexto, LugarPedirLista, LugarPedirBotones}
	// LugaresModales are the Lugares of every modal.
	LugaresModales = []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirTexto, LugarPedirLista, LugarPedirBotones,
		LugarFirma, LugarGuardar, LugarSeguir, LugarAcciones, LugarAyuda, LugarRegistro}
	// LugaresTodos are every Lugar: the zones, the search and the modals.
	LugaresTodos = []Lugar{LugarTarjetas, LugarHallazgos,
		LugarVerProyecto, LugarVerTarjeta, LugarVerHallazgo, LugarVerNada, LugarBusqueda,
		LugarPedirSelector, LugarPedirPedido, LugarPedirTexto, LugarPedirLista, LugarPedirBotones,
		LugarFirma, LugarGuardar, LugarSeguir, LugarAcciones, LugarAyuda, LugarRegistro}
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
	// AccionPaso (enter: the chosen card's step), AccionSiguiente (n: the
	// box's card's step), AccionSeguir (P) or AccionUsar ("" = none).
	Accion string
	// Barra is their hint on the keys line ("" = not there), with UTF-8
	// arrows like Muestra. The row with AccionPaso has a hint that depends
	// on what is chosen (see the package doc, "The two last lines"): its
	// Barra only says that it has one.
	Barra string
	// DeTarjeta marks the rows of the card (espacio, P and the action
	// keys): without a card (HECHO's line, an empty list, the detail of the
	// project or of a finding, the Acciones of a card gone) their hints are
	// not on the keys line.
	DeTarjeta bool
	Lugares   []Lugar
}

// Atajos is THE key table: the keys line, the help and the dispatch come
// from it, so no key does something without being in the help, nor the
// other way round (CA-564). Its order is the keys line's and the help's.
var Atajos = []Atajo{
	// enter: the step of what is chosen (first on the keys line)
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "abrir el modal del paso de la tarjeta (en HECHO, abrirlo o cerrarlo)",
		Accion: AccionPaso, Barra: "enter <paso>", Lugares: []Lugar{LugarTarjetas, LugarVerTarjeta}},
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "foco al detalle del hallazgo", Barra: "enter ver",
		Lugares: []Lugar{LugarHallazgos}},
	// n: the step of the box's card
	{Teclas: []Tecla{"n"}, Muestra: "n", Hace: "abrir el modal del paso de la tarjeta de la caja", Accion: AccionSiguiente,
		Barra: "n tu siguiente paso", Lugares: []Lugar{LugarTarjetas, LugarVerProyecto, LugarVerTarjeta, LugarVerHallazgo, LugarVerNada}},
	{Teclas: []Tecla{"n"}, Muestra: "n", Hace: "abrir el modal del paso de la tarjeta de la caja", Accion: AccionSiguiente,
		Lugares: []Lugar{LugarHallazgos}},
	// moving in the list and in the detail
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo, "k", "j"}, Muestra: "↑ ↓ k j", Hace: "elegir", Barra: "↑↓ elegir",
		Lugares: []Lugar{LugarTarjetas, LugarHallazgos}},
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo, "k", "j"}, Muestra: "↑ ↓ k j", Hace: "desplazar", Barra: "↑↓ desplazar",
		Lugares: LugaresVerAlgo},
	{Teclas: []Tecla{TeclaPgArriba, TeclaPgAbajo}, Muestra: "pgup pgdn", Hace: "desplazar una pagina", Lugares: LugaresVerAlgo},
	{Teclas: []Tecla{"g", "G"}, Muestra: "g G", Hace: "primera o ultima",
		Lugares: []Lugar{LugarTarjetas, LugarHallazgos}},
	{Teclas: []Tecla{"g", "G"}, Muestra: "g G", Hace: "principio o final", Lugares: LugaresVerAlgo},
	// the zones
	{Teclas: []Tecla{TeclaTab, TeclaShiftTab}, Muestra: "tab shift-tab", Hace: "foco a la otra zona", Barra: "tab detalle",
		Lugares: []Lugar{LugarTarjetas}},
	{Teclas: []Tecla{TeclaTab, TeclaShiftTab}, Muestra: "tab shift-tab", Hace: "foco a la otra zona",
		Lugares: []Lugar{LugarHallazgos, LugarVerProyecto, LugarVerTarjeta, LugarVerHallazgo, LugarVerNada}},
	{Teclas: []Tecla{TeclaDer, "l"}, Muestra: "→ l", Hace: "foco al detalle", Lugares: []Lugar{LugarTarjetas, LugarHallazgos}},
	{Teclas: []Tecla{"e"}, Muestra: "e", Hace: "abrir en el editor", Barra: "e editor", Lugares: []Lugar{LugarHallazgos}},
	{Teclas: []Tecla{"e"}, Muestra: "e", Hace: "abrir en el editor", Lugares: []Lugar{LugarVerTarjeta, LugarVerHallazgo}},
	{Teclas: []Tecla{TeclaEsc, TeclaIzq, "h"}, Muestra: "esc ← h", Hace: "foco a la lista", Barra: "esc volver",
		Lugares: LugaresVer},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "volver a las tarjetas", Barra: "esc volver",
		Lugares: []Lugar{LugarHallazgos}},
	// the card
	{Teclas: []Tecla{TeclaEspacio}, Muestra: "espacio", Hace: "acciones de la tarjeta", Barra: "espacio acciones",
		DeTarjeta: true, Lugares: LugaresConTarjeta},
	// the views
	{Teclas: []Tecla{"H"}, Muestra: "H", Hace: "abrir o cerrar la vista Hallazgos", Barra: "H hallazgos",
		Lugares: []Lugar{LugarTarjetas}},
	{Teclas: []Tecla{"H"}, Muestra: "H", Hace: "abrir o cerrar la vista Hallazgos",
		Lugares: []Lugar{LugarHallazgos, LugarVerProyecto, LugarVerTarjeta, LugarVerHallazgo, LugarVerNada}},
	{Teclas: []Tecla{"v"}, Muestra: "v", Hace: "el proyecto en el detalle", Barra: "v proyecto",
		Lugares: []Lugar{LugarTarjetas}},
	{Teclas: []Tecla{"v"}, Muestra: "v", Hace: "el proyecto en el detalle", Lugares: LugaresVer},
	// ]c is "]" and then "c": a key of two strokes, like in Vim
	{Teclas: []Tecla{"]c", "[c"}, Muestra: "]c [c", Hace: "criterio sin prueba siguiente o anterior",
		Lugares: []Lugar{LugarVerTarjeta}},
	// Seguir solo: an action of the board, not of the card
	{Teclas: []Tecla{"P"}, Muestra: "P", Hace: "seguir solo", Verbo: "hoom piloto <slug>", Accion: AccionSeguir,
		DeTarjeta: true, Lugares: LugaresConTarjeta},
	// the card's actions
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
	// the Firma, Guardar and Seguir solo modals
	{Teclas: []Tecla{TeclaIzq, TeclaDer, TeclaTab, TeclaShiftTab}, Muestra: "← → tab shift-tab",
		Hace: "boton anterior o siguiente", Barra: "←→ boton", Lugares: []Lugar{LugarFirma, LugarGuardar, LugarSeguir}},
	// the modals that run a verb
	{Teclas: []Tecla{TeclaEnter}, Muestra: "enter", Hace: "ejecutar la accion o cancelar, segun el boton elegido",
		Barra: "enter ejecutar", Lugares: []Lugar{LugarPedirBotones, LugarFirma, LugarGuardar, LugarSeguir}},
	{Teclas: []Tecla{TeclaCtrlE}, Muestra: "ctrl-e", Hace: "abrir el pedido en el editor", Barra: "ctrl-e editor",
		Lugares: []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirBotones}},
	{Teclas: []Tecla{TeclaEsc}, Muestra: "esc", Hace: "cerrar sin ejecutar", Barra: "esc cerrar",
		Lugares: []Lugar{LugarPedirSelector, LugarPedirPedido, LugarPedirBotones, LugarFirma, LugarGuardar, LugarSeguir, LugarAcciones}},
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
	// the Registro modal
	{Teclas: []Tecla{TeclaArriba, TeclaAbajo, "k", "j", TeclaPgArriba, TeclaPgAbajo}, Muestra: "↑ ↓ k j pgup pgdn",
		Hace: "desplazar", Barra: "↑↓ desplazar", Lugares: []Lugar{LugarRegistro}},
	{Teclas: []Tecla{TeclaEsc, "L"}, Muestra: "esc L", Hace: "cerrar el registro", Barra: "esc cerrar",
		Lugares: []Lugar{LugarRegistro}},
	// everywhere
	{Teclas: []Tecla{"L"}, Muestra: "L", Hace: "el registro de lo que se ejecuto", Lugares: LugaresPaneles},
	{Teclas: []Tecla{"/"}, Muestra: "/", Hace: "buscar tarjetas", Lugares: LugaresPaneles},
	{Teclas: []Tecla{"x"}, Muestra: "x", Hace: "modo experto", Lugares: LugaresPaneles},
	{Teclas: []Tecla{"?"}, Muestra: "?", Hace: "todas las teclas", Barra: "? todas las teclas", Lugares: LugaresPaneles},
	{Teclas: []Tecla{"q"}, Muestra: "q", Hace: "salir", Lugares: LugaresPaneles},
	{Teclas: []Tecla{TeclaCtrlC}, Muestra: "ctrl-c", Hace: "salir", Lugares: LugaresTodos},
}
