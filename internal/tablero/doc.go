// Package tablero is `hoom tablero` (spec tablero-con-paneles, the v2 of
// tablero-en-la-terminal): the board of the Studio in the terminal, full
// screen, in panels, driven by the keyboard and the mouse. It is a skin,
// like the Studio: the column, the actions, their why, the proposed pedido
// and values come from the card (boardcmd), and every action runs a capa-1
// verb (hoom pedir, aprobar, guardar, integrar, reanudar, usar) as a
// detached process of its own, after a modal says "Se va a ejecutar:" and
// the exact command (all but u).
//
// The model (Modelo) is pure: it gets keys (Tecla), mouse events (Raton),
// reads (Lectura of the board, Detalle of a card, Proyecto, Hallazgos,
// Archivo of a finding's file), the end of the processes it launched
// (FinProceso), the clock and the size, and it returns a screen (Pantalla)
// and orders (Orden) that Run carries out. Tests drive it with no terminal.
//
// This comment is the contract of the screen. Where it says "decision" it
// fixes a detail the spec leaves open (the skeleton's decisions); every
// other rule is the spec's.
//
// # The screen
//
// The screen is Alto lines of at most Ancho cells. Pantalla returns exactly
// Alto strings with their trailing spaces trimmed; without color
// (Config.Color false) they carry no escape sequence.
//
// Cells are counted with hoom's width table (AnchoDe, Celdas; CA-577): a
// wide character (East Asian Wide, Fullwidth, an emoji that presents as
// emoji) takes two cells and a combining mark none. This replaces the v1's
// "every rune, one cell". Every text cut to a width keeps the longest start
// that fits in the width less one cell and then the cut sign ("…", "~" in
// ASCII): a wide character that would straddle the limit goes out whole,
// and the cell it leaves is a space when something follows (a border).
//
// Every text the screen shows passes, before it is measured and cut, the
// v1's neutralization of controls: a tab is a space, and any other control
// (C0, DEL, C1) and any bidi control (U+200E, U+200F, U+202A..U+202E,
// U+2066..U+2069) is "?". Where a text is said to be "on one line" its
// newlines ("\n", "\r\n") are spaces first (a lone "\r" is a control). The
// texts come from the project and from the agents: none of them reaches
// the terminal as a sequence. The only sequences of a line are the SGR of
// the color.
//
// Signs, with UTF-8 (Config.UTF8) and in ASCII (CA-576):
//
//	segment hecho / falta / no aplica      ● ○ ·      # - .
//	your decision                          ✋          !    (✋ takes two cells)
//	criterion with a test / without / by command   ✓ ✗ ⚑   + x v
//	selected row, a finding's line         ▸          >
//	Hecho closed / open                    ▸ ▾        > v
//	box, simple: corners, side, line       ┌ ┐ └ ┘ │ ─   + + + + | -
//	box, focus: corners, side, line        ╔ ╗ ╚ ╝ ║ ═   # # # # # =
//	selector arrows                        ‹ ›        < >
//	text cursor                            ▌          _
//	cut                                    …          ~
//	"…" of the TUI's own words             …          ...  (lanzando…, otro…: decision)
//	"…" of CitarPedido's cut in a command  …          ~    (decision of the orchestrator)
//	arrows of the keys line and the help   ↑ ↓ ← →    ^ v < >  (decision)
//
// The separator " · " is the same in both modes (as in the v1).
//
// Accents, as in the v1: the TUI's titles and labels carry their accents
// with UTF-8 and lose them in ASCII ("NECESITA TU DECISIÓN" / "NECESITA TU
// DECISION", "¿Qué falta?" / "Que falta?", "Tiempo límite" / "Tiempo
// limite", "Narración" / "Narracion", "escribió" / "escribio", "revisó" /
// "reviso", "narró" / "narro", "último" / "ultimo"); the messages, the
// keys line, the help, the Registro's own words and the catalog's lines
// carry none in either mode; the texts of the card and of the project go as
// they come.
//
// Below 60x15 the screen is one line, "agranda la terminal (minimo 60x15)"
// cut to Ancho, and Alto-1 empty lines; no key but q and ctrl-c and no
// mouse event does anything then.
//
// From 60x15 on, with A = Alto and W = Ancho:
//
//	lines 1..A-2   the panels (B = A-2 lines)
//	line  A-1      the message line
//	line  A        the keys line
//
// # The layout
//
// Two columns with W >= 80 (CA-550). The left column is the cells
// 0..L-1, L = W/3 (integer division); the right one the cells L..W-1, R =
// W-L cells. Both columns are B lines tall.
//
// The left column has, from the top, [1], [2], [3] and [4], with these
// heights (borders included):
//
//	h1 = 6
//	h3 = its rows + 2, between 3 and 7 ("nada corriendo" is one row)
//	h4 = max(3, (B - h1 - h3) / 3)
//	h2 = B - h1 - h3 - h4
//
// When h2 < 3 (its minimum, decision), the panels without the focus fold to
// one line, in this order: [1], [4], [3]; after each fold the heights are
// computed again with h = 1 for the folded ones, until h2 >= 3. [2] does not
// fold in two columns, and the panel with the focus never folds.
//
// The right column has [0] on top and the Registro under it: the Registro
// is 6 lines with Alto >= 24 and 4 lines below (confirmed by the person on
// 2026-11-09: the spec fixes 6 "con 24 filas o más"; the 100x25 sketch is
// not the rule), and [0] takes the rest (h0 = B - hRegistro). The Registro
// would fold when h0 < 3, which never happens from 15 lines on.
//
// One column with 60 <= W <= 79 (CA-550): every panel is W cells wide and
// they go, from the top: [1], [2], [3], [4], [0], the Registro. The
// Registro is always folded. The left panel with the focus takes
// min(its natural height, A/3) lines and at least 3 (A/3 is "un tercio del
// cuerpo" of the spec rounded up: ceil(B/3) = A/3); the other three are
// folded; [0] takes the rest. With the focus on [0], the four are folded.
// The natural heights: [1] 6; [2] its lines + 2; [3] as h3 above; [4] its
// rows + 2, at least 3.
//
// A folded panel is one line, its width long: the simple line sign, the
// title (see Titles) cut to its width - 2, and the line sign up to its
// width: "─[3] Corriendo 1──────". A click on it gives it the focus, and
// then it unfolds (see The mouse).
//
// # The panels
//
// A panel of w cells and h >= 3 lines is drawn:
//
//	top      corner + title + line signs + corner
//	middle   side + inner line (w-2 cells) + side      (h-2 lines)
//	bottom   corner + line signs [+ "<n> de <m>" + one line sign] + corner
//
// The title goes right after the corner, cut to w-2 cells:
// "┌[1] Proyecto─────┐". Each inner line is the panel's content line cut
// to w-2 cells and padded with spaces to w-2. Every content line starts
// with a margin of one space; the content has no right margin (the tira
// of [2] touches the right side). The text of an empty panel or of one that
// is reading (` no hay tarjetas: hoom item add "<titulo>"`, " leyendo el
// tablero...", " nada corriendo", " leyendo los hallazgos...", " sin
// hallazgos abiertos", the Registro's " todavia no se ejecuto nada") that
// does not fit in one line is wrapped at its words, each continuation with
// its margin, when its lines fit in the panel's inner height; only when they
// do not is it one line, cut (the orchestrator's decision, 2026-11-09). In
// one column, [2]'s natural height counts those lines.
//
// The panel with the focus has the focus signs (═ ║ ╔ ╗ ╚ ╝; = and # in
// ASCII) and, with color, its border (the title included) is green and
// bold; the others have the simple signs (CA-551). There is always a panel
// with the focus; at the start it is [2]. The Registro never has it.
//
// Rows that do not fit (CA-552). [2], [3] and [4] have rows that can be
// chosen, each with its own selection, kept while the focus is elsewhere.
// A panel scrolls the least needed to keep its chosen row in view: a
// selection that goes below the view makes it the last line in view, one
// that goes above makes it the first. When the panel's content lines do
// not all fit in its h-2 inner lines, its bottom border says "<n> de <m>":
// n is the chosen row's place among the rows that can be chosen (from 1)
// and m how many they are ([2]'s section titles are not rows; Hecho's line
// is one). It goes right before the corner with one line sign between:
// "╚═════════1 de 10═╝", and only when it fits (its cells + 1 <= w-2).
// [1] has no rows: when its four lines do not fit (one column), it shows
// the first ones, without "<n> de <m>".
//
// The selection (CA-552): the chosen row of each left panel starts with
// the selection sign after the margin (" ▸ ..."; " " + " " + " " on the
// other rows). In the panel with the focus, with color, the chosen row is
// also a bar in reverse video over the whole inner line (its w-2 cells,
// the padding included); in the panels without the focus only the ▸ stays.
//
// Titles, in the top border (and in the folded line):
//
//	[1]        "[1] Proyecto"; folded, with " · check VERDE", " · check ROJO"
//	           or " · sin veredicto" after it (decision, from the spec's
//	           60-column sketch; nothing before the first read)
//	[2]        "[2] Tarjetas"
//	[3]        "[3] Corriendo <n>", n its rows (running and interrupted);
//	           "[3] Corriendo" without rows
//	[4]        "[4] Hallazgos <n>", n the open findings, and, with color,
//	           " · <k> high" in red when k of them are high; "[4] Hallazgos"
//	           without findings
//	[0]        "[0] " and what it shows: "<slug> · <titulo>" (a card of [2]),
//	           "Proyecto", "<slug> · <rol>" (a card of [3]) or
//	           "hallazgo <id>"; "[0]" alone when it shows nothing
//	Registro   "Registro"; folded, "Registro · " and the first line of its
//	           newest entry without its margin ("Registro · 14:02 Pedir al
//	           arquitecto · nuc-tenancy · hecho"), or "Registro" when it is
//	           empty
//
// [0] shows what is chosen in the last left panel that had the focus, and
// follows every move of that selection, without enter (CA-553): Proyecto
// for [1], the chosen card for [2] (nothing on Hecho's line), the chosen
// card's job for [3], the chosen finding for [4]; nothing when that panel
// has no row. At the start it is [2]'s.
//
// # The two last lines
//
// The message line (A-1) is " " and the message: the why, the errors and
// the notices, with their times and colors of the v1 (DuracionAviso,
// DuracionFin); the read errors stay while the last read of their kind
// failed, under any timed message, the board's first, then the
// project's, then the findings'. In expert mode it is " EXPERTO · <mensaje>",
// or " EXPERTO" without a message (decision). While the search is open it
// is " /<texto>▌" (" EXPERTO · /<texto>▌" in expert mode), and no message
// shows.
//
// The keys line (A) is " " and the hints of the current Lugar joined by
// " · " (CA-564): the Barra of each row of AtajosDe(Lugar), in table
// order. Right after "n siguiente paso", when the card has a next-step
// action (see The next step), goes that action's hint: the Muestra of its
// key's row and its label in lower case ("A aprobar spec"). Without a card
// ([2] on Hecho's line, an empty [3], [0] showing nothing) the hints of the
// card's rows (espacio, n and the action keys) are left out. When the line
// does not fit in W cells: a last hint "? todas las teclas" becomes
// "? todas", and then the hints before the last one are left out, from the
// one right before it backward, until it fits (decision, from the spec's
// two sketches); the last one is cut if it alone does not fit. At 100
// columns, [2] on a card in Tu aprobacion, the line is a space and
//
//	↑↓ elegir · enter ver · espacio acciones · n siguiente paso · A aprobar spec · ? todas las teclas
//
// and at 60 columns " ↑↓ elegir · enter ver · espacio acciones · ? todas".
//
// # Colors
//
// The 16 colors of the terminal (CA-576: none with NO_COLOR). Each piece
// of a line with a style is written "\x1b[" + its codes joined by ";" +
// "m" + the text + "\x1b[0m", its codes in this order: 1 (bold), 7
// (reverse), then its color: 31 red, 32 green, 33 yellow, 36 cyan, 90
// gray. Adjacent pieces with the same style are one piece; a piece without
// style is written as it is. The styles:
//
//	the focus (a panel's border and title, a modal's border)   1;32
//	the chosen row of the panel with the focus                 7 (and its pieces' colors: 7;31)
//	your decision: NECESITA TU DECISIÓN and the titles of the
//	columns with human true (Tu aprobacion, Tu aceptacion)     33
//	en curso: EN CURSO                                         36
//	hecho: Hecho's title; VERDE; a good message; a green end    32
//	red: a slug of a card with red or interrupted, an interrupted row of
//	[3], ROJO, "rojo: <plain>", "fallo: <...>", an error message,
//	a red end                                                  31
//	a finding's severity: high 31, medium 33, low 90
//	secondary and disabled: the origins and the help line of the
//	selectors, a gray option, action or selector, a closed finding  90
//
// The other columns' titles have no style. The " · <k> high" of [4]'s
// title is 31 alone, inside the border's style. While a modal is open, every
// cell of the panels (lines 1..A-2) outside its box (and outside an open
// list) is drawn, with color, in gray alone (90), whatever its own style
// (CA-565); the message line and the keys line keep theirs (an error of
// the modal stays red).
//
// # [1] Proyecto
//
// Four lines (CA-560), each in one of its forms:
//
//	1  " <proyecto> · rama <Rama>"         with Git
//	   " <proyecto> · sin git"             without it
//	2  " check VERDE · verify hace <t>"    Check.OK, t since Veredicto.CreatedAt
//	   " check ROJO: <motivo>"             not OK, motivo checkcmd.Motivo(Check)
//	   " sin veredicto todavia"            Check.Reason is checkcmd.ReasonNoVerdict
//	3  " hoy <usd> USD en <n> trabajos"    Hoy.Runs = n > 0, usd Hoy.CostoUSD; and
//	                                       " · <k> sin dato" after it when Hoy.SinCosto = k > 0
//	   " hoy sin trabajos"                 Hoy.Runs = 0
//	4  " fijada: <slug>"                   Lectura.Fijada
//	   " sin tarjeta fijada"               without one
//
// VERDE is green and ROJO red. <proyecto> is Config.Proyecto. Before the
// first Proyecto the lines are " <proyecto>", " leyendo el proyecto...",
// "" and the fourth.
//
// Its [0] ("[0] Proyecto"):
//
//	" Veredicto"
//	"  <id> · hace <t> · VERDE"          (ROJO when verdict is not green; " · parcial" after it when Partial)
//	"  pass <p> · fail <f> · error <e> · absent <a>"
//	"  .hoom/verdicts/<id>.json"
//	""
//	" Check"
//	"  <the check line of [1], without its margin>"
//	""
//	" Doctor"
//	"  <slug> · <plain>: <action>"       (one per problem; What when Plain is ""; without "<slug> · " when Slug is "")
//
// " Veredicto" is followed by "  sin veredicto todavia" without one, and
// " Doctor" by "  sin problemas" without problems. Before the first
// Proyecto: " leyendo el proyecto...".
//
// # [2] Tarjetas
//
// The sections of the v1, in its order (CA-555): "NECESITA TU DECISIÓN"
// (the cards with needs_decision), "EN CURSO" (the ones with running), then
// one per column of the board, in the board's order; each card once, in
// the first section it belongs to, in the board's order of cards; no empty
// section but Hecho's, which is always there, closed until enter opens it.
// A section's line is " " + its title: the two first as they are, a
// column's its name in upper case, a space and its number of cards
// (" BACKLOG 7"), Hecho's adding " ▸" closed or " ▾" open (its cards follow
// only when open). The titles go in the color of their state (see Colors).
//
// A card's row (decision on the exact sums, from the spec's rules):
//
//	" " + M + " " + S + " " + tira
//
// M is the selection sign or a space; the tira is the v1's: one sign per
// segment of the card's meter, in order, without separators, the FIRST
// falta segment being ✋ when the card has needs_decision. With room =
// (w-2) - 3 and T = Celdas(tira): when room - T - 1 >= 8, S is the slug cut
// (with the cut sign) or padded to room - T - 1 cells, so the tira touches
// the right side; otherwise the row has no tira and S is the slug cut to
// room. A card with red or interrupted has its slug in red. The plain is
// not in the row.
//
// Hecho's line is a row too: " HECHO 3 ▸" when it is not chosen and
// " ▸ HECHO 3 ▸" when it is (decision); enter, → or l on it opens or
// closes Hecho.
//
// Before the first read: " leyendo el tablero..."; with no card at all:
// ` no hay tarjetas: hoom item add "<titulo>"` (CA-578). At the start the
// selection is on the first card row.
//
// The search (/, LugarBusqueda; CA-554): from any panel it puts the focus
// on [2] and filters while it is typed: only the cards whose slug or title
// contains the text stay, without telling upper from lower case (Unicode
// lower case) and with the accents folded (á à â ä ã → a, é è ê ë → e,
// í ì î ï → i, ó ò ô ö õ → o, ú ù û ü → u, ñ → n, ç → c, upper case too).
// The sections and their numbers are those of the cards that stay (Hecho
// keeps its line). When the chosen card does not stay, the selection goes
// to the first row that does. ↓ and ↑ move the selection; enter and esc
// close the search and drop the filter, and the card chosen then stays
// chosen.
//
// # [0] of a card
//
// One document (CA-556), its parts separated by one empty line:
//
//	A  " ¿Qué falta?  <plain>"
//	   " siguiente: <next>"                  (expert mode only)
//	   " missing: <missing[i]>"              (expert mode only, one each)
//	   " Siguiente paso  <label>  [<key>]"   (the next step's action and the Muestra of its key's row)
//	   " Siguiente paso  <text>"             (no next-step action: what n says)
//	B  " <column_name> · gasto <g>[ · escribió <writer>][ · revisó <reviewer>]"
//	   " rojo: <red.plain>"                  (only with red, in red; red.reason in expert mode)
//	   " doctor: <plain>: <action>"          (one per problem of the card; What when Plain is "")
//	C  "  <sign> <label>: <detail>"          (one per segment of the tira; "  <sign> <label>" without detail)
//	D  " Spec · <n> criterios · <k> sin prueba"
//	   "  <sign> <text>"                     (one per criterion; "  <sign> <id> <text>" in expert mode)
//	E  " Hallazgos[ · <k> abiertos]"
//	   "  <severity> <donde> <description>"  (one per finding of its task, the open ones first)
//	F  " Historia"
//	   "  <YYYY-MM-DD HH:MM> <who> <plain>"  (one per timeline entry; summary in expert mode)
//	G  " Narración"
//	   the narration (see below)
//
// A: the expert lines and the next step as in "The next step". B: g is
// the card's spend.cost_usd with two decimals and " USD", or "sin dato de
// costo", and then " de <budget_usd> USD" (two decimals) when the card has
// a budget, as the v1's Resumen; " · escribió <w>" and " · revisó <r>"
// only when providers.writer and providers.reviewer are not empty
// (decision). C: the tira's signs, ✋ included. D: n is the number of
// criteria of the detail ("1 criterio" for one) and k of those with ✗; a
// criterion is ✗ when its id is in the card's evidence.untraced (the
// board's or the detail's), ⚑ when its traced_by is
// boardcmd.TracedComando, ✓ otherwise; text is its text. Without a spec
// (the detail's spec does not exist and has no markdown) part D is
// " sin spec: <ruta>", ruta as the v1's (the detail's paths.spec, else
// spec.path, else the card's evidence.spec, under the evidence dir). E:
// " · 1 abierto" or " · <k> abiertos" only when k > 0; severity in its
// color; donde is the base name of the finding's file (the part after
// its last / or \) with ":<linea>" when it names a line, or "-" without a
// file; the description on one line; a finding that is not open goes all
// gray; without findings, "  sin hallazgos". F: in UTC, in the v1's order;
// without entries, "  sin historia todavia". G: the v1's Sobre, two
// spaces in: "  <rol> · <paso> · <tiempo> · último evento hace <n>s" (with
// running: rol, paso and tiempo as [3]'s; without it, those of the card's
// newest work row, its tiempo its duration_ms and its status after it;
// "último evento hace <n>s" left out without events), "  no se pudo leer
// el sobre: <err>" in red when Sobre.Err, and one line per event
// "  <HH:MM:SS> <kind> <detail>" (UTC, the detail on one line); without
// work rows, "  sin sobres todavia".
//
// Parts D to G need the Detalle: before it comes they are one part,
// " leyendo la tarjeta..."; with Detalle.Err, " no se pudo leer la
// tarjeta: <Err>" in red.
//
// The lines longer than [0]'s inner width (w-2) are wrapped at the spaces
// (greedy: as many words as fit; a word longer than the line is cut
// hard), each continuation line starting with the leading spaces of the
// line it continues; the event lines of G are cut instead (decision).
//
// [0] starts at its first line for each new card, and scrolls (with the
// focus on it: ↑ ↓ k j one line, pgup pgdn its inner height, g G the start
// and the end; the wheel 3 lines) between its first line and the one that
// leaves its last line at the bottom (CA-557). ]c and [c make the next and
// the previous ✗ line of part D the first line in view, as far as the
// scroll goes (the next one below the first line in view; the previous
// one above it); without one the message line says "no hay mas criterios
// sin prueba". e orders AbrirEditor with the spec's path (relative to the
// root, as in D) and the line, in the spec's markdown, of the first
// criterion that has a line in view (the first markdown line whose text,
// trimmed, starts with "- " + its id + ":"), or 1 when none is in view or
// found; without a spec the message line says "sin spec: <ruta>"
// (decision).
//
// # [3] Corriendo
//
// One row per card with running and per card with interrupted (and no
// running), in the board's order (CA-558):
//
//	" " + M + " " + S + " " + rol + " " + tiempo [+ " de " + limite]
//	" " + M + " " + S + " " + rol + " interrumpido"     (in red)
//
// rol is running.role (interrupted.role), "agente" when empty; tiempo is
// the time since running.started_at and limite running.timeout_s (when not
// 0), as the v1 writes them: m:ss below an hour, h:mm:ss from an hour on
// ("12:41 de 45:00", "1:02:03"). With room = (w-2) - 3 and U the cells of
// what follows S (with the space before it): " de <limite>" goes only when
// room - U >= 8; S is the slug cut to room - U cells (at least 1), not
// padded (decision). Without rows: " nada corriendo".
//
// Its [0] ("[0] <slug> · <rol>"): first the v1's second line without its
// margin of 28: " <rol> · <paso> · <tiempo> · narró hace <n>s · <gasto>"
// (paso "<stage> <step>/<steps>", or the stage alone when steps is 0;
// tiempo as the row's with " de <limite>"; "narró hace <n>s" left out when
// last_event_at is null; gasto the card's spend.cost_usd "<x> USD" or "sin
// dato de costo"), or, interrupted, " interrumpido en el paso <step> de
// <steps> (<stage>): el <rol> ya no corre" in red; then " no se pudo leer
// el sobre: <err>" in red when Sobre.Err; then one line per event of the
// Detalle's Sobre: " <HH:MM:SS> <kind> <detail>" (UTC, on one line, cut),
// or " sin eventos todavia". Before its Detalle: " leyendo la tarjeta...".
// It starts at its end and follows it while the reads bring events, unless
// the person scrolled up: it follows again when the view is back at its
// end, and G takes it there.
//
// # [4] Hallazgos
//
// The project's open findings (Hallazgos, the function of hoom finding
// list --open): the high ones first, then the medium ones, then the low
// ones, then any other severity, each group in the order of the read
// (CA-559). A row (decision on the columns):
//
//	" " + M + " " + severity padded to 6 + " " + tarea + " " + donde
//
// With room = (w-2) - 10: tarea is the finding's task ("-" without one)
// cut or padded to room/2 cells, and donde (as part E of a card's [0]) is
// cut to room - room/2 - 1 cells. The severity word is in its color.
// Without findings: " sin hallazgos abiertos"; before the first read:
// " leyendo los hallazgos...".
//
// Its [0] ("[0] hallazgo <id>"):
//
//	" <severity> · lente <lens> · tarea <task>"   (severity in its color; "sin tarea" without a task)
//	" <description>"                               (whole: its own lines, each wrapped)
//	""
//	" <file>[:<linea>]"                            (the path as the finding names it)
//	" " + M + " " + <number> + " " + <text>        (the context lines, see below)
//	""
//	" para cerrarlo: hoom finding resolve <id> --as corregido|refutado --evidence \"<por que>\""
//
// The context lines are the file's lines from linea-K to linea+K that the
// Archivo read brings (with linea 0, its first 2K+1, none marked), K =
// min(ContextoArchivo, max(0, (H - F - 1) / 2)), H being [0]'s inner
// height and F the number of the document's other lines, so it fits when
// it can (decision); M is the finding-line sign on the finding's line and
// a space on the others; the numbers are right-aligned to the widest one
// shown; the text is cut, not wrapped. Instead of them: " leyendo
// <file>..." before the read, " no se pudo leer <file>: <Err>" in red when
// it failed, ` el archivo "<file>" esta fuera del proyecto` in red when it
// leaves the project (by its form, or Archivo.Fuera). A finding without a
// file has neither the empty line before " <file>" nor these lines. The
// finding's line is the v1's: the number after a ":" that ends the file
// field ("x.go:12"), or else after the first "<file>:" in the description;
// 0 without one. The file is relative to the project root.
//
// e, in [4] and in its [0], orders AbrirEditor with that file and line;
// without a file the message line says "el hallazgo no nombra un
// archivo"; with a path that leaves the project the message line says in
// red "el archivo <path quoted as strconv.Quote> esta fuera del proyecto"
// and nothing is ordered (see Paths).
//
// # The Registro
//
// One entry per verb the TUI runs, per e, and per end of a job (CA-563),
// as Modelo.Registro gives them:
//
//	" <HH:MM> <etiqueta> · <slug> · <estado>"
//	"       <comando>"                          (seven spaces: the margin and six)
//
//	" <HH:MM> Abrir en el editor · <file>[:<linea>]"
//	"       <editor command>"
//
//	" <HH:MM> <rol> termino en <slug>: <plain>"  (one line, green or red as the v1's end notice)
//
// HH:MM is the model's clock when the entry was added, in that clock's
// location (decision). etiqueta is the action's label on the card
// ("Pedir al writer", "Aprobar spec", "Guardar"); u's is "Fijar para la
// terminal". estado is "lanzando…" until its launch is registered (a role
// whose card shows running with a new envelope: "corriendo") or its
// process ends: `hecho` with code 0, `fallo: <Ultima>` in red otherwise
// (FinProceso; `hoom <verb> salio con <code>` when Ultima is empty, the
// verb being the first word of the order's Args: "hoom guardar salio con
// 3"). The
// command is the one "Se va a ejecutar" showed, on one line (its -m with
// tarjetacmd.CitarPedido's cut to 60 characters, not shortened further;
// in ASCII with "~" for that cut's "…", as the modal shows it), cut to the
// panel. The editor's command is Config.Editor and
// ArgumentosEditor(Config.Editor, <file>, <linea>) joined by spaces, the
// file relative to the root (decision). Ctrl-e adds no entry.
//
// The panel shows the newest whole entries that fit in its inner lines,
// the oldest of them on top and the newest at the bottom, from its first
// inner line (decision); empty, " todavia no se ejecuto nada". Folded, it
// shows its newest entry's first line in its title.
//
// # The keys
//
// Atajos is the one table of the keys line, the help and the dispatch
// (CA-564); Modelo.Lugar says the current Lugar. What each row does:
//
// Focus (CA-551): 1, 2, 3, 4 and 0 give the focus to that panel; tab and
// shift-tab move it in the order 1, 2, 3, 4, 0 (wrapping around). A left
// panel that gets the focus makes [0] show its choice; when the focus
// goes to [0] from a left panel, [0] keeps showing that panel's choice,
// and esc, ← and h in [0] give the focus back to it.
//
// In [2], [3] and [4]: ↑ ↓ k j move the selection one row (it stops at
// the ends), g and G go to the first and the last row (CA-552); enter, →
// and l give the focus to [0], or, on Hecho's line, open and close Hecho.
// In [1]: enter, → and l give the focus to [0] (Proyecto).
//
// In [0]: ↑ ↓ k j, pgup pgdn, g G scroll it (see each [0]); esc ← h
// back. In [0] with nothing to show (LugarVerNada) only esc ← h work: the
// keys that scroll are not in its rows of Atajos, since there they would
// have no effect (LugaresVerAlgo; the orchestrator's answer, 2026-11-09). With a card (LugaresConTarjeta) the card's keys work on [2]'s
// chosen card, [3]'s row's card or the card [0] shows.
//
// The action keys (CA-566): a, t, w, r, R, s, A, I and n never order
// Lanzar. When the card has the row's action and it is enabled they open
// its modal: Pedir for a, t, w, r and R; Firma for A and I; Guardar for s.
// When the card does not have it the message line says for DuracionAviso
// "en <column_name> no hay <Hace>"; when it is disabled, its why (v1). u
// orders Lanzar{Args: {"usar", <slug>}} at once, without a modal, and adds
// its Registro entry; [1] shows the card fixed after the next read
// (CA-560). espacio opens the Acciones modal.
//
// The next step (what n opens and [0]'s "Siguiente paso" says; CA-566),
// the first rule that applies:
//
//  1. the card in Hecho: none, "la tarjeta ya esta en Hecho";
//  2. with running: none, "el <rol> esta trabajando: espera a que termine";
//  3. with interrupted: none, "el trabajo se corto: R reanuda o pide de nuevo";
//  4. in Tu aprobacion (boardcmd.ColTuAprobacion): its action aprobar;
//     in Tu aceptacion (boardcmd.ColTuAceptacion): its action integrar;
//  5. without actions: none, "en <column_name> no hay nada que hacer
//     ahora: <plain>";
//  6. its first action, when it is a role action with a key (pedir
//     arquitecto, test-writer, writer, reviewer, reanudar) or guardar;
//  7. otherwise none, "en <column_name> no hay nada que hacer ahora:
//     <plain>".
//
// An action chosen by 4 or 6 that the card does not have is none with
// "en <column_name> no hay <Hace of its key's row>", and one that is
// disabled is none with its why (decision). n opens the modal of the
// next step's action, whatever its kind, and its button runs that
// action's verb (not hoom siguiente); without one, the message line says
// the text for DuracionAviso and nothing opens.
//
// e and ]c [c: see [0] of a card and [4]. / opens the search (see [2]),
// x toggles the expert mode, ? opens the Ayuda modal, q and ctrl-c order
// Salir (ctrl-c also in the search and in every modal: confirmed by the
// person on 2026-11-09).
//
// A card that disappears from the board (CA-578): see Modelo.Leer.
//
// # The modals
//
// Common to all (CA-565). A box of Wm = min(72, W-4) cells (decision),
// centered over the panels: its left cell at (W-Wm)/2, its top line at
// (B-Hm)/2 (lines from 0), Hm its lines (at most B). Its border has the
// focus signs and, with color, the focus style; its top border is
// corner + " " + title + " " + line signs + corner (the title cut to
// Wm-4 cells); its bottom border can say "<n> de <m>" as a panel's. Each
// inner line is
//
//	side + " " + mark + text + " " + side
//
// mark being two cells ("▸ " on the line with the focus, two spaces on the
// others) and text Wm-6 cells (cut and padded). While a modal is open the
// keys go to it only (the Lugares of the modal), and the keys line shows
// its keys; esc closes it without running anything (with a list open, it
// closes the list only). The modals that run a verb (Pedir, Firma,
// Guardar) end with a line of two buttons, centered in the text: "<b1>   <b2>",
// the chosen one as "[ <label> ]" and the other as "  <label>  " (decision);
// ←, → (and tab in Firma and Guardar) choose the other one, and enter on
// the buttons runs the chosen one: the action (orders Lanzar, closes the
// modal, adds the Registro entry) or Cancelar (closes it). When the size
// changes the modal is centered again (CA-565).
//
// A modal taller than B gives up, in order and only until it fits: the
// selectors' help line; the empty lines; the Pedido box's lines, from 4
// down to 1; the "Pedido" line, which goes into the box's top border as
// "┌ Pedido ─...┐" (its mark on that line); the line "Se va a ejecutar:",
// which joins the command's first line ("Se va a ejecutar: hoom ...")
// (decision, the last two, so the modal fits at 60x15). "Se va a
// ejecutar" and the buttons never go. Guardar's paths that still do not
// fit show the first ones and "y <n> rutas mas" in a last line (v1). What
// still does not fit is drawn from the top line and cut at line B. In
// width, the origin words are the first thing to go (see Pedir).
//
// Pedir (a, t, w, r, R, and n when the next step launches a role; CA-568),
// title "<label> · <slug>", its text lines (c = Wm-6):
//
//	"Pedido"                                   (the reviewer has neither this line nor the box)
//	"┌" + c-2 line signs + "┐"                  (the box of the Pedido, simple signs)
//	"│ " + pedido line (c-4 cells) + " │"       (4 lines)
//	"└" + c-2 line signs + "┘"
//	"Provider       " + value [+ origin]       (the 5 selectors: the label padded to 15)
//	"Modelo         " + value [+ origin]
//	"Esfuerzo       " + value [+ origin]
//	"Tope           " + value [+ origin]
//	"Tiempo límite  " + value [+ origin]
//	15 spaces + the help line                  (gray)
//	""
//	"Se va a ejecutar:"
//	the command, 1 or 2 lines
//	""
//	"[ <label> ]     Cancelar  "               (the buttons: the action, Cancelar)
//
// The fields, in tab and shift-tab order (wrapping around): Pedido,
// Provider, Modelo, Esfuerzo, Tope, Tiempo límite and the buttons; a gray
// selector is skipped. At the start the focus is on the Pedido with its
// cursor at its end (on the first selector that is not gray for the
// reviewer) and the action's button is the chosen one (decision on the
// focus, as the v1). The mark "▸ " goes on the line of the field with the
// focus ("Pedido" for the Pedido, the buttons' line for the buttons).
// "Pedido" is red after an enter on the action button with the pedido
// empty, which runs nothing: the focus goes to the Pedido and the message
// line says "el pedido esta vacio" in red (v1).
//
// The Pedido is edited as in the v1 (←, → move the cursor, backspace
// deletes before it, ctrl-u empties it, ctrl-e orders EditarPedido with
// it, from any field of the modal: confirmed by the person on 2026-11-09)
// and enter on it gives the focus to the
// buttons, without running. Its box shows the pedido wrapped hard every
// c-4 cells (its newlines kept as line breaks): with the focus, the 4
// lines that end at the cursor's line, the cursor shown with the text
// cursor sign; without it, its first 4 lines (decision).
//
// A selector's value is "‹ <option> ›"; a gray selector's is its value in
// gray ("‹ del provider ›", "‹ sin tope ›", the reviewer's "‹ <timeout> ›",
// reanudar's provider name without arrows); a selector turned text by
// otro… shows the text and the text cursor, without arrows. The origin
// goes at the column 15 + V of the text, V = max(26, the widest value of
// the five + 2) (decision, from the spec's sketch): for an enabled
// selector, where its value came from, by the action's origen ("de
// hoom.yaml" for hoom.yaml, "de la tarjeta" for tarjeta, "del provider"
// for provider), or "elegido" when the chosen value is not the proposed
// one; for a gray one, its why (below). The origins of the enabled
// selectors are gray, and they are all left out when one of them does not
// fit in the line (decision). A gray selector's why goes at that column when
// it fits there, and otherwise on a line of its own right under the
// selector, from column 15, gray: "hoom no le pasa el esfuerzo a opencode"
// never fits beside its selector in the 66 cells of the modal, and CA-569
// asks for it (decision). That line is not given up when the modal does
// not fit, and a why that goes on its own line does not count for the rule
// that leaves all the origins out (the orchestrator's answer, 2026-11-09).
//
// The help line, at column 15 and gray (decision on its form): with the
// focus on a selector, "<option>: <line>" of its chosen option when that
// option has a line, the note alone when it has a note (below), its why
// alone when it is gray, empty otherwise; with the focus on Provider, the
// note of Modelo's chosen option, or else Esfuerzo's, when they have one;
// empty with the focus on the Pedido or the buttons.
//
// In a selector ← and → choose the previous and the next option, wrapping
// around and skipping the gray ones and otro…; enter and espacio open its
// list. The list (the orchestrator's decision, 2026-11-09, from the spec's
// "una linea por opcion con su nombre y su linea", "encima del modal" and
// "una lista de opciones mas larga que su modal: se desplaza") is a box of
// its own with the simple signs, drawn over the modal:
//
//   - its rows, one per option: " " + M + " " + name padded to the longest
//     name + "  " + its line (a gray one all gray, with its why as its
//     line; without a line, nothing after the name), M the selection sign
//     on the highlighted option;
//   - its width: its longest row + 1 (its margin on the right) + 2 (its
//     borders), at most Ancho-2; each inner line is the row padded (or, only
//     when the row does not fit even at Ancho-2, cut with the cut sign of
//     the mode) to width-3 cells, then the margin's space;
//   - its left border on the modal's first text cell (the modal's left
//     border + 4), moved left just enough when it would leave the screen;
//   - its height: its options + 2, and at most the modal's height (its text
//     lines + 2); its top border on the selector's line, moved up just
//     enough for its bottom border to be inside the modal's height;
//   - it scrolls, with "<n> de <m>" on its bottom border (n the highlighted
//     option's place), only when it has more options than the modal has
//     text lines.
//
// At the start the chosen option is the highlighted one; ↑ and ↓ move over
// every option, enter chooses the highlighted one and closes the list (on
// a gray one it does nothing), esc closes it without changing (CA-568).
//
// otro… turns the field into text, empty: enter with text makes it the
// chosen value (origin "elegido"; it stays among the options, before
// otro…); esc, or enter with the text empty, goes back to the option
// chosen before. The TUI does not validate what is written (CA-568). The
// text is a part of its selector, like an open list: ctrl-e does nothing
// in it (LugarPedirTexto has no row of ctrl-e; the orchestrator's answer,
// 2026-11-09).
//
// The options of each selector (CA-569, CA-570, CA-571), p being the chosen
// provider:
//
//   - Provider: the action's providers, in their order, by their name;
//     those that cannot (ok false) are gray with their why. At the start
//     the default one is chosen (the first that can when none is). In
//     reanudar the provider does not change: its selector is gray, without
//     arrows, with the origin "la sesion es de <p>".
//   - Modelo: "por defecto del provider" (line "el que elige <p>: su
//     configuracion o tu cuenta"), then modelos.Modelos(p, Config.Modelos[p])
//     with their lines, then the action's model when it is in neither (line
//     "de hoom.yaml"), then "otro…". At the start the action's model is
//     chosen ("por defecto del provider" when it is ""). When the action's
//     origen.model is hoom.yaml, "por defecto del provider" is gray with
//     "hoom.yaml fija <model> para el <rol>". A p without Model
//     (modelos.RecibeModelo false: gemini, opencode) makes the selector
//     gray, "‹ del provider ›", with "hoom no le pasa el modelo a <p>", and
//     it opens no list.
//   - Esfuerzo: "por defecto del provider" (the same line), the catalog's
//     efforts of p (modelos.De), the action's effort when it is not among
//     them (line "de hoom.yaml"), and "otro…"; origen.effort plays
//     origen.model's part; a p without Effort (modelos.RecibeEsfuerzo
//     false) makes it gray, "‹ del provider ›", with "hoom no le pasa el
//     esfuerzo a <p>".
//   - Changing the provider: when origen.model is hoom.yaml the chosen
//     model stays chosen, and when it is not in p's lists it is the option
//     before otro… with the note "<model> no esta en la lista de <p>:
//     puede rechazarlo"; without hoom.yaml's value, Modelo goes back to
//     "por defecto del provider", and back to the proposed provider it
//     goes back to the proposed model. Esfuerzo follows the same three
//     rules with origen.effort. Without hoom.yaml's value the proposal is
//     what the card brings ("" being "por defecto del provider"): back at
//     it, no --model nor --effort goes (the orchestrator's answer,
//     2026-11-09).
//   - Tope: when p takes no cap (its option's budget false) it is gray,
//     "‹ sin tope ›", with "<p> no acepta tope". Otherwise: first the
//     proposal (the action's budget_usd as "<x> USD", x with
//     strconv.FormatFloat(x, 'f', -1, 64), or "sin tope" when it brings
//     none), then 1, 2, 5, 10 and 20 USD without repeating the proposal,
//     then "otro…" (its text is the number of USD: "‹ <text> USD ›"). "sin
//     tope" is there only as the proposal. An option under p's
//     min_budget_usd is gray with "<p> pide al menos <min> USD".
//   - Tiempo límite: first the proposal (timeout_s as valores.Duracion),
//     then 15m, 30m, 45m, 1h and 2h without repeating it, then "otro…".
//     The reviewer's is gray with "de review.timeout": hoom pedir reviewer
//     takes it from review.timeout and has no --timeout.
//
// The command (CA-568) is "hoom" and the order's Args: the verb
// ({"pedir", <rol>, <slug>} or {"reanudar", <slug>}), "-m" and the
// trimmed pedido (not for the reviewer), then, in this order and only
// when the chosen value differs from the action's proposal: "--provider"
// <name> (never in reanudar), "--model" <name>, "--effort" <name>,
// "--budget-usd" <number>, "--timeout" <duration> (never for the
// reviewer); "por defecto del provider" is the empty value and a gray
// Modelo, Esfuerzo or Tope sends nothing. Each value is quoted as the v1's
// → line (tarjetacmd.Citar, the pedido with tarjetacmd.CitarPedido; on
// Windows with their markers). In ASCII the screen writes the "…" of
// CitarPedido's cut with the ASCII cut sign "~" (no screen in ASCII carries
// "…"; the orchestrator's decision, 2026-11-09); the order's Args do not
// change. It is laid out by words, an argument being
// one word: the first line from the text's first cell, each next word on
// the same line after a space when it fits in c cells and else on a new
// line that starts with two spaces. When that takes more than two lines,
// the argument of -m is shortened to its start and "…": the longest start
// (of the pedido on one line, trimmed) with which, quoted with
// CitarPedido, it takes at most two lines (decision); when not even one
// character does, the second line is cut. The pedido's newlines are
// spaces here and in the Registro (CitarPedido does it); the order's Args
// keep them.
//
// Firma (A, I, and n when the next step is a signature; CA-574): title
// "<label> · <slug>", the text of tarjetacmd.TextoFirma wrapped at the
// spaces, "", "Se va a ejecutar:", "hoom aprobar <slug> --si" (or "hoom
// integrar <slug> --si"), "", and the buttons Cancelar and the action's
// label, Cancelar chosen at the start. Only enter on the action's button
// orders Lanzar{Args: {"aprobar"|"integrar", <slug>, "--si"}}. As the v1:
// the modal is tied to what it shows (on every read of the board or of the
// card and on enter before anything runs, its card must still have the
// action enabled, the signature's text be the same, and the spec too when
// the card's Detalle showed it; when not, it closes and the message line
// says in red the action's why or "cambio lo que ibas a confirmar en
// <slug>: vuelve a abrirlo"), and a signature that does not fit whole is
// never signed: enter on the action's button says in red "agranda la
// terminal: la firma no entra entera" and runs nothing.
//
// Guardar (s, and n when the next step is guardar; CA-574): title
// "<label> · <slug>", "Se guarda en git:", one line per path it saves (the
// action's paths, else the card's unsynced ones), "", "Se va a ejecutar:",
// "hoom guardar <slug>", "", and the buttons Guardar (the action's label)
// and Cancelar, the action chosen at the start. enter on it orders
// Lanzar{Args: {"guardar", <slug>}}; it is tied to what it shows like
// Firma (its paths).
//
// Acciones (espacio; CA-567): title "Acciones · <slug>", one line per
// action of the card in its order (the expert ones only in expert mode):
// "<tecla> <label>  <verbo>", tecla the Muestra of the row with that
// Accion (a space when none) and verbo its Verbo with <slug> replaced; a
// disabled action is gray and ends with "  (<why>)"; one without a row
// (relanzar, descartar, sesion, terminal) is gray and its verbo is "sin
// verbo en la terminal"; without actions, "no hay acciones en esta
// tarjeta" in gray. The mark "▸ " is on the chosen action: ↑ ↓ k j move it
// over the enabled ones that have a row (the first of them at the start);
// enter opens the modal of the chosen one (Acciones closes), and the key of
// an action does what it does outside (opens its modal, runs u, or says
// its why with Acciones still open). u is not listed, since it is not an
// action of the card, but its key runs hoom usar there (the orchestrator's
// answer, 2026-11-09).
//
// Ayuda (?; CA-564): title "Todas las teclas", its text lines in groups,
// one empty line between groups, each group a line with its title and one
// line per row: "  " + Muestra padded to 18 cells + Hace, and "  " + Verbo
// after it when the row has one. A line that does not fit in the modal is
// wrapped at its words, each continuation from the column of Hace (20; the
// Mouse lines, from their own two spaces): no text is lost (the
// orchestrator's answer, 2026-11-09). The groups, in this order: the group of
// the Lugar it was opened from; "En todas partes" (the rows whose Lugares
// include all of LugaresPaneles); the other panels' groups, in the order
// [1] Proyecto, [2] Tarjetas, [3] Corriendo, [4] Hallazgos, [0] el
// proyecto, [0] una tarjeta, [0] lo que corre, [0] un hallazgo; "Modales"
// (the rows of LugaresModales, each once, in table order); "Mouse". A
// panel's group has the rows of AtajosDe of its Lugar that are not in "En
// todas partes" ([2]'s, then those of LugarBusqueda); LugarVerNada's group
// is "[0]", only when it is the first. "Mouse", with Config.Mouse:
//
//	"  clic              foco al panel; en una fila, la elige"
//	"  rueda             mueve la seleccion; en [0] y en la ayuda, desplaza"
//	"  clic en un modal  enfoca un campo, elige una opcion o un boton"
//	"  para seleccionar texto con el mouse, manten Option (macOS) o Shift"
//
// and without it "  apagado: HOOM_MOUSE=0" (decision). It is B lines tall
// when its lines do not fit (then its bottom border says "<n> de <m>", n
// the number of the last line in view and m of all of them; decision),
// starts at its first line, scrolls with ↑ ↓ k j (one line), pgup pgdn
// (its inner height) and the wheel (3 lines), and esc or ? close it.
//
// # The mouse
//
// Run asks for it unless HOOM_MOUSE is "0" (ConMouse, SecMouseSi). The
// model never orders Lanzar for a mouse event nor opens a modal (CA-575):
//
//   - A click in a panel's box (its borders included, a folded line too)
//     gives it the focus (the Registro takes none); on a row of a left
//     panel it also chooses that row ([0] follows); on a section title of
//     [2] it only gives the focus; on Hecho's line it chooses it, without
//     opening it. A click that takes the focus out of [2] closes the search
//     (as esc); one on a row of [2] keeps it open.
//   - The wheel over a left panel moves its selection one row per notch,
//     without moving the focus; over [0] it scrolls it 3 lines; over the
//     Registro or a folded line it does nothing.
//   - With a modal open: a click outside it does nothing; a click on a
//     field's line (a selector that is not gray, the "Pedido" line or its
//     box) gives it the focus; a click on a button chooses it without
//     running it, and the message line says "enter para <label in lower
//     case>" ("enter para pedir al writer", "enter para cancelar") for
//     DuracionAviso; with a list open, a click on an option that is not
//     gray chooses it and closes the list, and any other click does
//     nothing; in Acciones a click on an action that can be chosen marks
//     it; in Ayuda the wheel scrolls 3 lines; nothing else does anything.
//
// A terminal that sends no SGR events has no mouse; everything works with
// the keyboard.
//
// # Paths
//
// As in the v1: the paths are relative to the project root, and a path
// that leaves the project is neither read nor opened. The model looks at
// its form on every system (an absolute one on any of them: rooted as "/x",
// "\x" or a UNC, or with a drive as "C:x"; or one whose ".." climbs out
// with / or \ as the separator); Run and Datos.Archivo look at its
// symlinks inside os.Root.
//
// # Times and amounts
//
// "hace <t>": "<n>s" below a minute, else the biggest whole unit among
// "<n>m", "<n>h" and "<n>d" (90 s is "1m", 3600 s "1h", 86400 s "1d"); a
// time in the future is "0s". Amounts go with two decimals and " USD".
// The running times are the v1's m:ss and h:mm:ss.
//
// # What Run writes
//
// The sequences of Run (SecEntrar and SecMouseSi when it takes the
// terminal, SecMouseNo and SecSalir when it gives it back), then one frame
// per change: "\x1b[H" and the Pantalla lines, each followed by "\x1b[K",
// joined by "\r\n".
package tablero
