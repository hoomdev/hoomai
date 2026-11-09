// Package tablero is `hoom tablero` (spec tablero-en-la-terminal, capa 2):
// the board of the Studio in the terminal, full screen, driven by the
// keyboard. It is a skin, like the Studio: the column, the actions, their
// why, the proposed pedido and values come from the card (boardcmd), and
// every action runs a capa-1 verb (hoom pedir, siguiente, aprobar, guardar,
// integrar, reanudar, usar) as a detached process of its own.
//
// The model (Modelo) is pure: it gets keys (Tecla), reads of the board
// (Lectura), reads of the open card (Detalle), the end of the processes it
// launched (FinProceso) and the clock, and it returns a screen (Pantalla)
// and orders (Orden) that Run carries out. Tests drive it with no terminal.
//
// # The screen contract
//
// The screen is Alto lines of at most Ancho cells. Every rune counts as one
// cell (there is no Unicode width table: spec, Riesgos). Pantalla returns
// exactly Alto strings with their trailing spaces trimmed; without color
// (Config.Color false) they carry no escape sequence. Every text that does
// not fit in its line is cut to the line's width, its last cell being "…"
// ("~" in ASCII).
//
// Signs (Signos): with UTF-8 (Config.UTF8) and in ASCII:
//
//	segment hecho            ●  #
//	segment falta            ○  -
//	segment no aplica        ·  .
//	your decision            ✋  !
//	criterion with a test    ✓  +
//	criterion without test   ✗  x
//	criterion by command     ⚑  v
//	selected row             ▸  >
//	Hecho closed / open      ▸ ▾  > v
//	separator line           ─  -
//	box corners and sides    ┌ ┐ └ ┘ │ ─   + + + + | -
//	form arrows              ‹ ›  < >
//	text cursor              ▌  _
//	the → of the verb line   →  ->
//	cut                      …  ~
//
// In ASCII the TUI's own words lose their accents too: "NECESITA TU
// DECISIÓN" is "NECESITA TU DECISION", "narró" is "narro", "último" is
// "ultimo". Text that comes from the card (titles, plain, why) is printed
// as it is.
//
// Below 60x15 the screen is one line, "agranda la terminal (minimo 60x15)",
// and Alto-1 empty lines.
//
// Every screen of 60x15 or more has the same frame:
//
//	line 1        the title (see each screen)
//	lines 2..A-3  the body (A-4 lines)
//	line A-2      the separator: Ancho times ─
//	line A-1      bar 1: " <MODO> · <fijada>" and, while there is one,
//	              " · <mensaje>"
//	line A        bar 2: " " + the hints of the current screen
//
// where A is Alto. MODO is NORMAL, or EXPERTO in expert mode, or, while the
// search is open, "/" followed by what was typed and the text cursor (e.g.
// "/nuc▌"). <fijada> is "tarjeta: <slug> (hoom usar)" when hoom usar fixed
// a card and "sin tarjeta fijada" otherwise. The hints are the Barra texts
// of the Atajos of the current Vista, in table order, joined by " · ".
//
// # The list (VistaLista)
//
// Title: " hoom · <proyecto> · <n> tarjetas" (" · 1 tarjeta" for one).
// Before the first read of the board the body's first line is
// " leyendo el tablero..."; with no card at all it is
// ` no hay tarjetas: hoom item add "<titulo>"`.
//
// The body is the sections, in this order: "NECESITA TU DECISIÓN" (cards
// with needs_decision), "EN CURSO" (cards with running), then one per
// column of the board, in the board's order. A card appears once, in the
// first section it belongs to, in the board's order of cards. An empty
// section is not shown, except the one of the column Hecho
// (boardcmd.ColHecho), always shown and closed until enter opens it.
//
// A section's line is " " + its title. The two first titles go as they are;
// a column's title is its name in upper case and the number of cards in the
// section: " BACKLOG 7", " TU APROBACION 1". Hecho's adds its state:
// " HECHO 3 ▸" closed, " HECHO 3 ▾" open (its cards follow only when open).
//
// A card is one line:
//
//	" " + M + " " + slug + pad + " " + tira [+ "  " + plain]
//
// M is ▸ on the selected row and a space elsewhere. The slug is padded with
// spaces to 24 cells (a longer slug goes whole, with no pad). The tira is
// one sign per segment of the card's meter, in order, with no separators:
// hecho, falta, no aplica; when the card has needs_decision, the FIRST
// falta segment is ✋ instead. The plain follows the tira after two spaces
// when at least 8 cells are left for it, cut to the line. A tira longer than
// the room after the slug is cut: the signs that fit in the room minus one,
// then "…", and no plain.
//
// In expert mode the tira shows every segment as "<sign> <label>: <detail>",
// joined by "  ", cut to the line, and no plain.
//
// A card with running has a second line: 28 spaces and
//
//	<rol> · <paso> · <tiempo> · narró hace <n>s · <gasto>
//
// rol is running.role ("agente" when empty); paso is "<stage>
// <step>/<steps>", or the stage alone when steps is 0; tiempo is the time
// since running.started_at, and " de " and running.timeout_s when it is not
// 0, both as m:ss below an hour and h:mm:ss from an hour on ("12:41 de
// 45:00", "1:02:03"); "narró hace <n>s" (whole seconds since
// running.last_event_at) is left out when last_event_at is null; gasto is
// the card's spend.cost_usd with two decimals and " USD" ("3.88 USD"), or
// "sin dato de costo". The pieces are joined by " · ".
//
// A card with interrupted (and no running) has a second line in red: 28
// spaces and "interrumpido en el paso <step> de <steps> (<stage>): el
// <rol> ya no corre".
//
// The selection moves over the card lines and Hecho's line (enter, l or →
// on it opens or closes Hecho). At the start it is on the first card line.
// The body scrolls the least needed to keep the selected line (and its
// second line) in view.
//
// The search (/, VistaBusqueda) filters while it is typed: only the cards
// whose slug or title contains the text stay, case-insensitive (Unicode
// lower case) and with the accents folded (á à â ä ã → a, é è ê ë → e,
// í ì î ï → i, ó ò ô ö õ → o, ú ù û ü → u, ñ → n, ç → c, upper case too).
// The sections and their counts are those of the cards that stay (Hecho
// keeps its line). ↓ and ↑ move the selection; enter opens the selected
// card; enter and esc close the search and drop the filter.
//
// # The open card (VistaTarjeta)
//
// Title: " <slug> · <titulo>". The body's first line is the tabs:
// " " + the five "N Nombre" (1 Resumen, 2 Spec, 3 Sobre, 4 Hallazgos,
// 5 Historia) joined by "  ", the active one as "[N Nombre]". The tab's
// content follows on the body's next lines, scrolled with j/k.
//
// Resumen, one item per line, each starting with " ":
//
//	columna: <column_name>
//	<plain>
//	siguiente: <next>              (expert mode only)
//	missing: <missing[i]>          (expert mode only, one line each)
//	falta: <label>: <detail>       (one line per falta segment; "falta: nada" without one)
//	tira: <tira>                   (the list's signs, in both modes)
//	  <sign> <label>: <detail>     (one line per segment)
//	gasto: <cost> de <budget>      (see Gasto)
//	escribio: <writer> · reviso: <reviewer>   ("sin dato" for an empty one)
//	rojo: <red.plain>              (only with red; red.reason in expert mode)
//	doctor: <plain>: <action>      (one line per problem; what instead of an empty plain)
//
// Gasto is "gasto: " + the cost (as in the list, "sin dato de costo"
// without one) + " de <budget> USD" when spend.budget_usd is set.
//
// Spec: the lines of the spec as Detalle carries it, each after a margin of
// two cells: a criterion line (one whose text, trimmed, starts with "- " +
// the id of a criterion of the card followed by ":") has its sign and a
// space (✗ when the id is in evidence.untraced, ⚑ when its trace is
// boardcmd.TracedComando, ✓ otherwise); every other line has two spaces.
// Without a spec the content is " sin spec: <path>". ]c and [c make the next
// and previous ✗ line the first line in view ("no hay mas criterios sin
// prueba" in the bar when there is none). o orders AbrirEditor with the
// spec's path and the line number (1-based) of the first line in view.
//
// Sobre: its first line is " <rol> · <paso> · <tiempo> · último evento
// hace <n>s": with running, rol, paso and tiempo as in the list's second
// line; without it, those of the card's newest work row (Detalle.Work[0]),
// its tiempo being its duration_ms (m:ss, h:mm:ss) and its status added
// after it (" · <status>"). "último evento hace <n>s" counts from the last
// event of Detalle.Sobre and is left out without one. Then one line per
// event of Detalle.Sobre: " <HH:MM:SS> <kind> <detail>" (UTC, the detail's
// newlines as spaces). Without work rows: " sin sobres todavia". The
// content starts scrolled to its end.
//
// Hallazgos: the task's findings, the open ones (status abierto) first, each
// group in the order Detalle carries them, one line each:
// " " + M + " " + "<status> <severity> <file>[:<line>] <description>", cut.
// M marks the selected one (j/k). <file> is "-" for a finding without a
// file. Its line is the number after a ":" that ends the file field
// ("x.go:12") or, failing that, after the first "<file>:" in the
// description; 0 without one. enter orders AbrirEditor with the file
// relative to the project root (evidence.dir + "/" + file, or just file
// when dir is ".") and that line; on a finding without a file the bar says
// "el hallazgo no nombra un archivo". A path that leaves the project on
// some system (an absolute one on any of them: rooted as "/x", "\x" or a
// UNC, or with a drive as "C:x"; or one whose ".." climbs out, with / or \
// as the separator: filepath.IsLocal says no) orders nothing, here and with
// the Spec tab's o: the bar says in red
// "el archivo <path quoted as strconv.Quote> esta fuera del proyecto" for
// DuracionAviso. Without findings: " sin hallazgos".
//
// Historia: one line per timeline entry: " <YYYY-MM-DD HH:MM> <who>
// <plain>" (summary instead of plain in expert mode). Without entries:
// " sin historia todavia".
//
// # The action menu (VistaMenu)
//
// It takes the body: its first line is " Acciones · <slug>", then one line
// per action of the card in its order (the expert ones only in expert mode):
// " " + M + " " + <tecla> + " " + <label> + "  " + <verbo>, where tecla is the
// action's key (or a space) and verbo is "hoom ..." with <slug> replaced.
// A disabled action is gray and ends with "  (" + why + ")"; an action with
// no capa-1 verb (relanzar, descartar, sesion, terminal) is gray and its
// verbo is "sin verbo en la terminal". j/k (and the arrows) move M over the
// enabled actions that have a verb; enter or the action's key runs it; esc
// closes.
//
// # The launch form (VistaFormulario)
//
// A box over the list, from line 2: each box line is " " + a line of
// Ancho-2 cells; the list's body lines below the box stay as they were.
//
//	┌ <label> · <slug> ─...─┐
//	│ <fields>              │
//	└─...───────────────────┘
//
// The inner lines are "│ " + text padded to Ancho-6 + " │". The fields,
// one per line, each label padded to 15 cells after a two-cell mark (▸ and
// a space on the focused field, two spaces elsewhere):
//
//	Provider       ‹ <name> ›
//	                 <name>: <why>   (one line per provider that cannot, gray)
//	Modelo         <text>       ("del provider" when empty and not focused)
//	Esfuerzo       <text>       ("del provider" when empty and not focused)
//	Presupuesto    <text> USD   ("sin tope" when empty and not focused)
//	Tiempo limite  <text>
//	(empty line)
//	Pedido
//	<the pedido, wrapped to the inner width, its last 4 lines>
//	(empty line)
//	→ <the verb line>
//	enter lanzar · tab campos · ctrl-e editor · esc cancelar
//
// The focused text field shows the text cursor at its cursor (the pedido:
// its wrapped lines with the cursor inserted, the 4 that end at the
// cursor's line). The reviewer has no Pedido (nor its empty line), and its
// Tiempo limite is not a field: hoom pedir reviewer takes it from
// review.timeout and has no --timeout, so the line shows it gray, without
// the focus mark, as "Tiempo limite  <text> (de review.timeout)" ("de
// review.timeout" for <text> when the action brings none), tab skips it and
// no --timeout is ever sent; reanudar shows Provider without the arrows
// (the session is its provider's: ← and → do nothing there).
//
// A box taller than the body (Alto-4 lines, the box's two borders
// included) leaves out, in this order and only until it fits: the empty
// lines; the lines of the providers that cannot (the Provider line then
// ends with "  (<n> no pueden)", gray); the keys line; and the pedido's
// lines, down to the one with the cursor. The → line always stays. The
// confirmations follow the same rule: their empty line goes first;
// guardar's that still does not fit shows its first paths and "y <n>
// rutas mas" in a last line, its header and keys line kept; a signature
// that does not fit whole is never signed: enter on Firmar says "agranda
// la terminal: la firma no entra entera" in red and runs nothing.
//
// tab and shift-tab move the focus over the fields in that order, wrapping
// around; ← and → on Provider choose the previous or next option that can
// (wrapping around), and in a text field move its cursor; backspace
// deletes before the cursor; ctrl-u empties the focused text field.
//
// # The confirmations (VistaConfirmacion)
//
// A box like the form's. aprobar and integrar: the text of the capa-1
// signature (tarjetacmd.TextoFirma), wrapped, an empty line, and the
// buttons "[ Cancelar ]   Firmar " or "  Cancelar   [ Firmar ]" (the chosen
// one between brackets; Cancelar at the start). guardar: "Se guarda en
// git:", one line per unsynced path, an empty line and
// "enter guardar · esc cancelar".
//
// A confirmation is tied to what it shows. On every read of the board or of
// the open card, and on enter before anything runs, it is compared with the
// data of now: its card must still have the action enabled, the signature's
// text be the same (and the spec, when the open card's detail showed it: a
// read of the card that fails or brings no spec counts as a change),
// guardar's paths be the same. When they are not, it closes and the bar
// says in red the action's why or "cambio lo que ibas a confirmar en
// <slug>: vuelve a abrirlo", and nothing runs.
//
// # The help (VistaAyuda)
//
// ? opens it from the list, every tab of the open card and the menu, for the
// view it was opened from. It takes the body: " Teclas de <vista>" (the
// Vista's name: lista, resumen, spec, ..., menu) and one line per Atajo of
// that Vista (AtajosDe): " " + Muestra padded to 14 cells + Hace, and "  " +
// Verbo when it has one. esc or ? close it. When the rows do not fit under
// the header line but fit in the whole body, the header gives way; when
// they do not fit at all, they flow down and then across in as many
// balanced columns as they need, (Ancho-1)/columns cells each, Muestra
// padded to its longest + 2 and every cell cut with the cut sign: every
// key stays on the screen.
//
// # Wrapping
//
// The texts of the boxes are wrapped to the box's inner width: the pedido
// hard, every inner-width cells (its newlines kept as line breaks); the
// other texts at the spaces (greedy: as many words as fit; a word longer
// than the line is cut hard).
//
// # Controls in the texts
//
// Every text the screen shows passes, before it is cut to the width, with
// its controls neutralized: a tab is a space, and any other control (C0,
// DEL, C1) and any bidi control (U+200E, U+200F, U+202A..U+202E,
// U+2066..U+2069) is "?". The texts come from the project and from the
// agents (titles, plains, specs, findings, events): none of them reaches
// the terminal as a sequence. The only sequences of a line are the SGR of
// the color.
//
// # What Run writes
//
// SecEntrar once, then one frame per change: "\x1b[H" and the Pantalla
// lines, each followed by "\x1b[K", joined by "\r\n"; with color, the
// lines carry SGR sequences (bold titles, red and green messages, gray
// disabled items, reverse on nothing). SecSalir once at the end.
package tablero
