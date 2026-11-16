// Package tablero is `hoom tablero` (spec tablero-tu-siguiente-paso, the
// v3; it re-expresses part of tablero-con-paneles, the v2, which itself was
// the v2 of tablero-en-la-terminal): the board of the Studio in the
// terminal, full screen, ordered around one question, who has the ball. On
// top, the box TU SIGUIENTE PASO says what the person has to do now; under
// it, on the left, one list of the cards grouped by who has the ball; on the
// right, the detail of the chosen card, always in the same order (AHORA,
// TE TOCA, SIGUE, EL CAMINO). It is a skin, like the Studio: the column,
// the actions, their why, the proposed pedido and values come from the card
// (boardcmd), and every action runs a capa-1 verb (hoom pedir, aprobar,
// guardar, integrar, reanudar, usar, piloto) as a detached process of its
// own, after a modal says "Se va a ejecutar:" and the exact command (all
// but u).
//
// The model (Modelo) is pure: it gets keys (Tecla), mouse events (Raton),
// reads (Lectura of the board, Detalle of a card, Proyecto, Hallazgos,
// Archivo of a finding's file), the end of the processes it launched
// (FinProceso), the clock and the size, and it returns a screen (Pantalla)
// and orders (Orden) that Run carries out. Tests drive it with no terminal.
//
// This comment is the contract of the screen. Where it says "decision" it
// fixes a detail the spec leaves open (the skeleton's decisions); every
// other rule is the spec's. "The v2" below is the package as
// tablero-con-paneles left it: what this comment does not change keeps its
// v2 rule, and the v2's names of its panels read as the spec's table "Los
// nombres de la v2" says ([2] the list of cards, [0] the detail, [1] the
// header, the [0] of [1] the detail with v, [4] and its [0] the Hallazgos
// view, [3] the group CORRIENDO).
//
// # The screen
//
// The screen is Alto lines of at most Ancho cells. Pantalla returns exactly
// Alto strings with their trailing spaces trimmed; without color
// (Config.Color false) they carry no escape sequence.
//
// Cells are counted with hoom's width table (AnchoDe, Celdas; CA-577): a
// wide character (East Asian Wide, Fullwidth, an emoji that presents as
// emoji) takes two cells and a combining mark none: a mark is drawn with
// the cell before it, its base, and is not lost ("Cafe" + U+0301 shows
// "Café"); a variation selector (U+FE00..U+FE0F) is dropped, since a
// terminal can turn its base two cells wide. Every text cut to a width keeps
// the longest start that fits in the width less one cell and then the cut
// sign ("…", "~" in ASCII): a wide character that would straddle the limit
// goes out whole, and the cell it leaves is a space when something follows
// (a border).
//
// Every text the screen shows passes, before it is measured and cut, the
// v1's neutralization of controls: a tab is a space, and any other control
// (C0, DEL, C1), any bidi control (U+200E, U+200F, U+202A..U+202E,
// U+2066..U+2069) and any other format character (the category Cf: a zero
// width space, a joiner, a BOM, the tags) is "?", so the width table and
// the terminal agree. Where a text is said to be "on one line" its
// newlines ("\n", "\r\n") are spaces first (a lone "\r" is a control). The
// texts come from the project and from the agents: none of them reaches
// the terminal as a sequence. The only sequences of a line are the SGR of
// the color.
//
// Signs, with UTF-8 (Config.UTF8) and in ASCII (CA-576, CA-720):
//
//	segment hecho / falta / no aplica      ● ○ ·      # - .
//	your decision                          ✋          !    (✋ takes two cells)
//	criterion with a test / without / by command   ✓ ✗ ⚑   + x v
//	the path: before / TE TOCA / CORRIENDO /
//	  DETENIDA / EN COLA and after          ● ◆ ◉ ✗ ○  # ! * x -
//	the "here" of the path                 ← acá      <- aca
//	the time bar: run / left               █ ░        # -
//	the quotes of a title                  « »        " "
//	selected row, a finding's line         ▸          >
//	HECHO closed / open                    ▸ ▾        > v
//	box, simple: corners, side, line       ┌ ┐ └ ┘ │ ─   + + + + | -
//	box, focus: corners, side, line        ╔ ╗ ╚ ╝ ║ ═   # # # # # =
//	selector arrows                        ‹ ›        < >
//	text cursor                            ▌          _
//	cut                                    …          ~
//	"…" of the TUI's own words             …          ...  (lanzando…, otro…)
//	"…" of CitarPedido's cut in a command  …          ~
//	arrows of the keys line and the help   ↑ ↓ ← →    ^ v < >
//	the DETENIDA mark of a row             ✗          x
//
// The separator " · " is the same in both modes.
//
// Accents. The TUI's own fixed words carry their accents with UTF-8 and
// lose them in ASCII (á → a, é → e, í → i, ó → o, ú → u, and the upper
// case ones): the v2's titles and labels ("¿Qué falta?" / "Que falta?",
// "Tiempo límite" / "Tiempo limite", "Narración" / "Narracion", "escribió" /
// "escribio", "revisó" / "reviso", "narró" / "narro", "último" /
// "ultimo"), and the new fixed texts of this spec: the phrases of the box
// and of AHORA, SIGUE and the path's names, "después:", "más", "← acá"
// (CA-720), each written as the spec writes it ("despues te toca firmarlo"
// has no accent in either mode). The messages, the keys line, the help, the
// Registro's own words, the modals' fixed texts and the catalog's lines
// carry none in either mode; the texts of the card and of the project go as
// they come.
//
// Below 60x15 the screen is one line, "agranda la terminal (minimo 60x15)"
// cut to Ancho, and Alto-1 empty lines; no key but q and ctrl-c and no
// mouse event does anything then.
//
// From 60x15 on, with A = Alto and W = Ancho (lines counted from 1, as the
// spec does; Pantalla's index is the line - 1):
//
//	line  1        the header
//	lines 2..5     the box TU SIGUIENTE PASO
//	lines 6..A-3   the body: the list and the detail (C = A-8 lines)
//	line  A-2      the Registro, folded on one line
//	line  A-1      the message line
//	line  A        the keys line
//
// # The layout of the body
//
// Two zones with W >= 100 (CA-696): the list is the cells 0..L-1, L = W*2/5
// (integer division), and the detail the cells L..W-1; both are C lines
// tall.
//
// One column with 60 <= W <= 99: both are W cells wide, the list on top and
// the detail under it. With the focus on the list, the list takes
// max(3, min(N, (C+1)/2)) lines, N being its natural height (its content
// lines + 2), and the detail the rest. With the focus on the detail the list
// is folded to one line and the detail takes C-1 lines.
//
// A folded box is one line, its width long: the simple line sign, its title
// cut to its width - 2, and the line sign up to its width. The folded
// list's title is the list's title and, when it has a chosen row,
// " · <n> de <m>" (n and m as in its bottom border, below):
// "─Tarjetas · 1 de 7──────". A click on it gives the focus to the list,
// which then unfolds.
//
// # The boxes
//
// The list and the detail are boxes; the box TU SIGUIENTE PASO is one too.
// A box of w cells and h >= 3 lines is drawn:
//
//	top      corner + title + line signs [+ right part + one line sign] + corner
//	middle   side + inner line (w-2 cells) + side      (h-2 lines)
//	bottom   corner + line signs [+ "<n> de <m>" + one line sign] + corner
//
// The title goes right after the corner, cut to w-2 cells. A right part of
// the top border (the box's counts, the detail's slug) goes right before
// the corner with one line sign between, only when it fits after the title
// with at least one line sign between them. Each inner line is the box's
// content line cut to w-2 cells and padded with spaces to w-2. Every content
// line starts with a margin of one space; a content line with a right part
// (the box's second line, AHORA, EL CAMINO) has a margin of one space on
// the right too, and the list's paths touch the right side. The text of an
// empty box or of one that is reading (` no hay tarjetas: hoom item add
// "<titulo>"`, " leyendo el tablero...", " leyendo los hallazgos...", " sin
// hallazgos abiertos") that does not fit in one line is wrapped at its
// words, each continuation with its margin, when its lines fit in the box's
// inner height; only when they do not is it one line, cut. In one column,
// the list's natural height counts those lines.
//
// The zone with the focus, the list or the detail, has the focus signs
// (═ ║ ╔ ╗ ╚ ╝; = and # in ASCII) and, with color, its border (the title
// and the right part included) is green and bold; the other has the simple
// signs (CA-706). There is always a zone with the focus; at the start it is
// the list. The box TU SIGUIENTE PASO never has it, and the Registro is no
// box.
//
// Rows that do not fit (CA-707). The list keeps its chosen row in view,
// the whole card (its two lines with W >= 100): a selection that goes below
// the view makes the chosen card's last line the last line in view, one
// that goes above makes its first line the first in view (the least
// scroll). When the list's content lines do not all fit in its h-2 inner
// lines, its bottom border says "<n> de <m>": n is the chosen row's place
// among the rows that can be chosen (from 1) and m how many they are (the
// group lines are not rows; HECHO's line is one). It goes right before the
// corner with one line sign between: "╚═════════1 de 10═╝", and only when
// it fits (its cells + 1 <= w-2).
//
// # The header (line 1)
//
// One line (CA-697): " " + the left part, then spaces, then the right part
// and one space, so the right part ends at the cell W-2.
//
//	left   "hoom tablero · <proyecto> · rama <Rama>"   with Git
//	       "hoom tablero · <proyecto> · sin git"       without it
//	       "hoom tablero · <proyecto>"                 before the first Proyecto
//	right  the check, then " · hoy <usd> USD" (Hoy.Runs > 0, usd Hoy.CostoUSD with two
//	       decimals) or " · hoy sin trabajos", then " · fijada: <slug>" (Lectura.Fijada,
//	       only when there is one); before the first Proyecto, "leyendo el proyecto..."
//
// The check is the v2's [1] second line without its margin: "check VERDE ·
// verify hace <t>" (Check.OK, t since the CreatedAt of the verdict the
// check refers to: VeredictoDelCheck, or Veredicto when it is that one),
// "check ROJO: <motivo>" (checkcmd.Motivo) or "sin veredicto todavia"
// (Check.Reason is checkcmd.ReasonNoVerdict). <proyecto> is Config.Proyecto.
//
// What does not fit (decision on the gap): the line fits when 1 +
// Celdas(left) + 2 + Celdas(right) + 1 <= W. While it does not, the right
// part drops " · fijada: <slug>", then the hoy part; then the check is cut
// to W - 4 - Celdas(left) cells when that is at least 1; when it is not,
// there is no right part and the left part is cut to W-2 cells. With color
// "hoom tablero" is green and bold, VERDE green and ROJO red.
//
// # The box TU SIGUIENTE PASO (lines 2..5)
//
// A box of W cells and 4 lines, always with the simple signs (CA-699). Its
// title is "TU SIGUIENTE PASO" and the right part of its top border
// "te toca <a> · corriendo <b> · detenida <c>", a, b and c the number of
// cards of the groups TE TOCA, CORRIENDO and DETENIDA (see The groups),
// always the three; before the first read there is no right part. With
// color its whole border (corners, lines, sides, title and right part) is
// yellow when TE TOCA or DETENIDA has a card and gray when not (gray before
// the first read).
//
// The box counts and chooses from every card of the board: the search's
// filter does not change it (decision).
//
// The card of the box (Modelo.Caja; CA-698) is the first card of TE TOCA in
// the list's order, else the first of DETENIDA. Its two inner lines:
//
//	1  " " + phrase + "  " + plain     the phrase bold, the plain gray; cut to w-2
//	2  " " + left [+ spaces + right + " "]
//
// The phrase comes from the card's step (see The step of a card) and its
// title T (the item's titulo; the slug when the titulo is empty), between
// the quotes of the mode:
//
//	Aprobar spec (aprobar)                 Firmá el spec de «T».
//	Integrar (integrar)                    Aceptá «T».
//	Pedir al arquitecto                    Pedile el spec al arquitecto para «T».
//	Pedir al test-writer                   Pedile los tests al test-writer para «T».
//	Pedir al writer                        Pedile el código al writer para «T».
//	Pedir al reviewer                      Pedile la review al reviewer para «T».
//	Reanudar                               Reanudá el trabajo de «T».
//	Guardar                                Guardá el trabajo de «T».
//	Seguir solo                            Dejá que «T» siga sola.
//	no step, in TE TOCA                    Mirá «T».
//	a card of DETENIDA (with a step or not)  Decidí cómo sigue «T».
//
// The step's action is known by its id (boardcmd.ActAprobar,
// ActIntegrar, ActPedirArquitecto, ActPedirTestWriter, ActPedirWriter,
// ActPedirReviewer, ActReanudar, ActGuardar; Seguir solo is AccionSeguir).
// The plain is the card's plain; for a card of DETENIDA it is its stop's:
// "se corto el trabajo del <rol>" with interrupted (rol as the v2's:
// interrupted.role, "agente" when empty), else red.plain.
//
// The left part of line 2 is "n " + the step's label in lower case
// ("n aprobar spec", "n seguir solo"), or, without a step, the text the
// rule of the step gives. The right part, gray, is "después: " and the
// titles of the cards that follow the box's in the same order (the rest of
// TE TOCA, then DETENIDA), each between the quotes and joined by " · ", as
// many as fit (decision on the room): room = (w-2) - 1 - Celdas(left) - 2 -
// 1; the titles are added one by one; one that does not fit whole is cut
// to the room it leaves (its quotes kept) when that is at least 2 cells,
// and then no more are added; when not even the first one fits that way,
// there is no right part. Without other cards there is no right part.
//
// Without a card in TE TOCA nor in DETENIDA (CA-699) the box has no card
// (Caja is "") and says:
//
//   - with a card in CORRIENDO: "Nada te espera. hoom trabaja en <n>
//     tarjeta(s)." ("1 tarjeta", "<n> tarjetas"; bold) and "corriendo: «T»
//     (<rol>, <tiempo>)" of the first card of CORRIENDO, " y <k> más" after
//     it when k more run; rol and tiempo as its row's (see The rows), and
//     "(piloto, decidiendo)" for a card whose board's pilot is alive without
//     running (decision). n says "nada te espera: hoom esta trabajando".
//   - with none running and a card in EN COLA: "Nada en curso. Podés
//     empezar «T»." (bold) with the first card of EN COLA, and line 2 as
//     above for that card ("n pedir al arquitecto"); n opens that card's
//     step (it is n's card, though Caja is "").
//   - with none of the three: "No hay nada pendiente." (bold) and
//     `para sumar trabajo: hoom item add "<titulo>"`. n says
//     "no hay nada pendiente" (decision).
//
// Before the first read of the board the box says " leyendo el
// tablero..." on its first inner line, and nothing on the second.
//
// # The groups
//
// Each card falls in one group (Modelo.Grupo; CA-700), the first of these
// that holds:
//
//	1  HECHO       its column is Hecho
//	2  CORRIENDO   it has running, or a pilot this board launched is alive for it
//	3  DETENIDA    it has interrupted, or it has red with red.esperado false
//	4  EN COLA     its column is Backlog
//	5  TE TOCA     any other
//
// A pilot "this board launched is alive" from its Lanzar (see Seguir solo)
// until its FinProceso.
//
// # The list
//
// The cards view (Vista VistaTarjetas; the Hallazgos view is below). Its
// title is "Tarjetas". The groups go in this order: TE TOCA, CORRIENDO,
// DETENIDA, EN COLA, HECHO. In TE TOCA the cards go by column from the
// board's last column to its first (Tu aceptacion, Review, Writer,
// Test-writer, Tu aprobacion, Arquitecto), and inside a column, as in every
// other group, in the board's order of cards. No group is shown empty but
// HECHO, which is always there and starts closed.
//
// A group's line is " " + its name + " " + its number of cards (" TE TOCA
// 2"); HECHO's adds " ▸" closed or " ▾" open, and its cards follow only when
// it is open. With color the name of TE TOCA is in reverse yellow (7;33),
// CORRIENDO's reverse cyan (7;36), DETENIDA's reverse red (7;31), and EN
// COLA's and HECHO's gray (90); the number and HECHO's sign have no style.
// The group lines are not rows, but HECHO's is: " HECHO 4 ▸" when it is not
// chosen and " ▸ HECHO 4 ▸" when it is; enter on it opens or closes HECHO.
//
// Before the first read: " leyendo el tablero..."; with no card at all:
// ` no hay tarjetas: hoom item add "<titulo>"` (CA-578). At the start the
// selection is on the first card row.
//
// # The rows
//
// A card's first line (CA-701), with room = (w-2) - 3:
//
//	" " + M + " " + T + " " + path        when room - 9 >= 8
//	" " + M + " " + T                     otherwise (no path)
//
// M is the selection sign or a space, the path is the card's 8 signs (see
// The path) and T its title (the slug when the titulo is empty; never the
// slug otherwise): with W >= 100, cut or padded to room - 9 cells, so the
// path touches the right side; without the path, cut to room.
//
// With W >= 100 every card has a second line: "   " (three spaces) + its
// state, cut to w-2, in its group's color (TE TOCA 33, CORRIENDO 36,
// DETENIDA 31, EN COLA 90, HECHO 90):
//
//	TE TOCA     the plain
//	CORRIENDO   "<rol> · <tiempo>[ de <limite>]"; with a pilot of this board alive it starts
//	            with "el piloto la lleva · "; without running it is "el piloto la lleva · decidiendo"
//	DETENIDA    "✗ " + red.plain, or "✗ se corto el trabajo del <rol>" (interrupted)
//	EN COLA     the plain
//	HECHO       the plain
//
// rol is running.role ("agente" when empty), tiempo the time since
// running.started_at and limite running.timeout_s (when not 0), as the v1
// writes them: m:ss below an hour, h:mm:ss from an hour on ("14:02 de
// 30:00", "1:02:03").
//
// With 60 <= W <= 99 a card is one line (decision on T's width): " " + M +
// " " + T + " " + path + " " + state (the state of the table above, in its
// color), the whole line cut to w-2. T is cut or padded to
// t = max(8, min(L, (w-2-12)/2)) cells, L being the widest title among the
// list's card rows (those shown: the filter's, and HECHO's only while it is
// open), so the path is at the same column in every row.
//
// The selection (CA-701): the chosen row's first line has the selection
// sign; in the zone with the focus, with color, that line (only the first)
// is also a bar in reverse video over the whole inner line (its w-2 cells,
// the padding included), its pieces keeping their colors (7;33, 7;32...).
//
// The search (/, LugarBusqueda; CA-554): from any Lugar of the panels it
// goes back to the cards view, puts the focus on the list and filters while
// it is typed: only the cards whose slug or title contains the text stay,
// without telling upper from lower case (Unicode lower case) and with the
// accents folded (á à â ä ã → a, é è ê ë → e, í ì î ï → i, ó ò ô ö õ → o,
// ú ù û ü → u, ñ → n, ç → c, upper case too). The groups and their numbers
// are those of the cards that stay (HECHO keeps its line). When the chosen
// card does not stay, the selection goes to the first row that does. ↓ and
// ↑ move the selection; enter and esc close the search and drop the
// filter, and the card chosen then stays chosen.
//
// # The path
//
// One sign per column of boardcmd.Columns, in its order, 8 in all (CA-702):
// ● (green, 32) for the columns before the card's; in the card's column ◆
// (yellow, 33) in TE TOCA, ◉ (cyan, 36) in CORRIENDO, ✗ (red, 31) in
// DETENIDA, ○ (gray, 90) in EN COLA; ○ (gray) for the columns after it. In
// HECHO the 8 are ● (green). The detail names the steps of the path, one per
// column:
//
//	Backlog Pedido · Arquitecto Spec · Tu aprobacion Tu firma · Test-writer Tests ·
//	Writer Código · Review Review · Tu aceptacion Tu aceptación · Hecho Hecho
//
// # The Hallazgos view (H; CA-712)
//
// The list shows the project's open findings (Hallazgos, the function of
// hoom finding list --open): the high ones first, then the medium ones,
// then the low ones, then any other severity, each group in the order of
// the read (CA-559). Its title is "Hallazgos <n>" (n the open findings),
// and, with color, " · <k> high" in red when k of them are high;
// "Hallazgos" without findings. A row, as the v2's:
//
//	" " + M + " " + severity padded to 6 + " " + tarea + " " + donde
//
// With room = (w-2) - 10: tarea is the finding's task ("-" without one) cut
// or padded to room/2 cells, and donde (the base name of the finding's file
// with ":<linea>" when it names a line, or "-" without a file) is cut to
// room - room/2 - 1 cells. The severity word is in its color (high 31,
// medium 33, low 90). Without findings: " sin hallazgos abiertos"; before
// the first read: " leyendo los hallazgos...". The detail shows the chosen
// finding (see The detail of a finding). esc in the list and H anywhere in
// the view go back to the cards view, with the card that was chosen, the
// focus on the list; / goes back too and opens the search.
//
// # The detail
//
// It shows the card chosen in the list (the cards view), the finding
// chosen in the list (the Hallazgos view), the project (after v, until the
// focus leaves the detail; CA-713) or nothing (HECHO's line, an empty
// list). It follows every move of the list's selection, without enter
// (CA-707).
//
// Its top border (CA-708): the title of what it shows and, for a card, a
// right part. For a card the title is its title (the slug when the titulo
// is empty) and the right part "<slug>", then " · riesgo <r>" when the
// item's riesgo is neither "" nor "normal", then " · piloto encendido" when
// the item's auto is item.AutoHastaHumano. What does not fit drops from the
// end (piloto, then riesgo); when the slug alone does not fit, the title is
// cut to (w-2) - Celdas(slug) - 2 cells when that is at least 2, and else
// there is no right part and the title is cut to w-2. For a finding the
// title is "hallazgo <id>"; for the project, "Proyecto"; with nothing, no
// title.
//
// Lines longer than the detail's inner width (w-2) are wrapped at the
// spaces (greedy: as many words as fit; a word longer than the line is cut
// hard), each continuation line starting with the leading spaces of the
// line it continues; the lines with a right part, the path's lines and the
// event lines of the Narración are not wrapped but built or cut to w-2.
//
// It scrolls (with the focus on it: ↑ ↓ k j one line, pgup pgdn its inner
// height, g G the start and the end; the wheel 3 lines) between its first
// line and the one that leaves its last line at the bottom (CA-557). It
// starts at its first line for each new thing it shows, and follows its
// end only when the document is longer than the view and the person
// scrolled to its end: then a read that makes it longer keeps the end in
// view. A card with running starts at its end instead, and follows it
// while the reads bring events, unless the person scrolled up; it follows
// again when the view is back at its end, and G takes it there (CA-711).
// The project and a finding never follow.
//
// # The detail of a card
//
// One document, its parts separated by one empty line (CA-708). With W >=
// 100 (the wide form):
//
//	AHORA    " AHORA" + spaces + G + " "          (AHORA bold gray, 1;90; G the card's group's
//	                                               name, in the style of its group line)
//	         " " + phrase                          (bold)
//	         " " + gray line                       (gray; only when there is one)
//	         " tiempo " + bar + " " + tiempo + " de " + limite   (only with running and a limit)
//	         " siguiente: <next>"                  (expert mode only)
//	         " missing: <missing[i]>"              (expert mode only, one each)
//	TE TOCA  " TE TOCA"                            (bold; yellow 1;33 when the card has a step,
//	                                               gray 1;90 when not)
//	         " enter " + label + " · " + help      (the step's label in lower case; " · " and the
//	                                               help gray)
//	         " nada: " + text                      (without a step: the rule's text)
//	SIGUE    " SIGUE"                              (1;90)
//	         " " + phrase
//	         " con el piloto seguiria sola: hoom item set <slug> --auto hasta-humano"
//	                                               (gray; see SIGUE)
//	CAMINO   " EL CAMINO" + spaces + "paso <i> de 8" + " "   (EL CAMINO 1;90)
//	         "  " + sign + " " + name [+ pad + state] [+ spaces + "← acá" + " "]   (8 lines)
//	B to G   the v2's parts B to G (below)
//
// AHORA's phrase and gray line, by the first case that holds (CA-709):
//
//	with running                 by its role (below)          "<provider> · empezo <HH:MM> · <paso>"
//	a pilot of this board alive  El piloto está decidiendo el siguiente paso.   "hoom piloto <slug>"
//	with interrupted             El trabajo del <rol> se cortó en el paso <step> de <steps>.
//	                                                          "ya no corre: R lo reanuda"
//	a pilot of this board that stopped, its stop still standing (see Seguir solo)
//	                             El piloto se detuvo.         its stop's motivo
//	with red not esperado        red.plain                    red.reason in expert mode, none otherwise
//	with red esperado            Los tests están escritos y fallan, como se espera.   "falta el código"
//	in Tu aprobacion             El spec está listo y espera tu firma.     the plain
//	in Tu aceptacion             Está lista para que la pruebes y la aceptes.   the plain
//	in Backlog                   Todavía no empezó.           the plain
//	in Hecho                     Terminada.                   the plain
//	any other                    the plain                    none
//
// By role, with running: arquitecto "El arquitecto está escribiendo el
// spec.", test-writer "El test-writer está escribiendo los tests.", writer
// "El writer está escribiendo el código.", reviewer "El reviewer está
// revisando el cambio.", any other "El <rol> está trabajando." (rol
// "agente" when empty). <provider> is running.provider, HH:MM its
// started_at in the model clock's location, and <paso> the v2's
// "<stage> <step>/<steps>" (the stage alone when steps is 0); the parts
// that are empty are left out with their " · ". The bar (decision on the
// rounding) has 20 cells: f = 20 * elapsed / limit (integer, at most 20)
// are the run sign, cyan (36), and 20-f the left sign, gray (90); tiempo
// and limite as the rows'. "ya no corre: R lo reanuda" and "falta el
// código" are gray lines too.
//
// TE TOCA's help, by the step:
//
//	Aprobar spec          hoom no firma por vos
//	Integrar              hoom no acepta por vos
//	Pedir a un rol        se abre el pedido para revisarlo antes de lanzar
//	Reanudar              retoma la sesion que se corto
//	Guardar               guarda el trabajo de la tarjeta en su rama
//	Seguir solo           el piloto la lleva hasta tu aceptacion o hasta que algo lo frene
//
// SIGUE's phrase, by the card's column, with the pilot on (the item's auto
// is item.AutoHastaHumano) or without it (CA-710):
//
//	Backlog        both: el arquitecto escribe el spec; despues te toca firmarlo
//	Arquitecto     both: te toca firmar el spec
//	Tu aprobacion  on:  al firmar, el piloto pide los tests y sigue solo hasta tu aceptación
//	               off: al firmar, te toca pedir los tests al test-writer
//	Test-writer    on:  el piloto pide el código al writer y sigue hasta tu aceptación
//	               off: despues, te toca pedir el código al writer
//	Writer         on:  el piloto pide la review; si algo bloquea, hace una ronda de hallazgos
//	               off: despues, te toca pedir la review al reviewer
//	Review         on:  si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos
//	               off: si nada bloquea, te toca aceptar
//	Tu aceptacion  both: al aceptar, hoom integra y la tarjeta pasa a Hecho
//	Hecho          both: nada: la tarjeta esta terminada
//
// Without the pilot, in Tu aprobacion, Test-writer, Writer and Review, the
// gray line "con el piloto seguiria sola: hoom item set <slug> --auto
// hasta-humano" follows the phrase.
//
// EL CAMINO: i is the place (1..8) of the card's column in
// boardcmd.Columns. Its 8 lines, one per column in order: "  " + the sign
// of the path for that column (in its color) + " " + the step's name; the
// card's column's line pads the name to 15 cells (decision) and adds, in
// its group's color, "te toca a vos" (TE TOCA), "<rol>, ahora" (CORRIENDO;
// "piloto, ahora" without running, decision), "se detuvo" (DETENIDA) or
// "sin empezar" (EN COLA), nothing in HECHO; and, on every group, "← acá" in
// its group's color ending at the cell w-3, so one space is left before the
// side. The other lines have nothing after the name.
//
// The rest are the v2's parts B to G, in their order and with their
// content, each after an empty line:
//
//	B  " <column_name> · gasto <g>[ · escribió <writer>][ · revisó <reviewer>]"
//	   " rojo: <red.plain>"                  (only with red, in red; red.reason in expert mode)
//	   " doctor: <plain>: <action>"          (one per problem of the card; What when Plain is "")
//	   " <Pendientes.Texto>"                 (only when the card's Pendientes.Texto is not "": the
//	                                         spec puts the line of pendientes in the rest, after the
//	                                         doctor and before the meter)
//	C  "  <sign> <label>: <detail>"          (one per segment of the meter; "  <sign> <label>"
//	                                         without detail; only when the card has a meter)
//	D  " Spec · <n> criterios · <k> sin prueba"
//	   "  <sign> <text>"                     (one per criterion; "  <sign> <id> <text>" in expert mode)
//	E  " Hallazgos[ · <k> abiertos]"
//	   "  <severity> <donde> <description>"  (one per finding of its task, the open ones first)
//	F  " Historia"
//	   "  <YYYY-MM-DD HH:MM> <who> <plain>"  (one per timeline entry; summary in expert mode)
//	G  " Narración"
//	   the narration (see below)
//
// B: g is the card's spend.cost_usd with two decimals and " USD", or "sin
// dato de costo", and then " de <budget_usd> USD" (two decimals) when the
// card has a budget; " · escribió <w>" and " · revisó <r>" only when
// providers.writer and providers.reviewer are not empty. C: the meter's
// signs (● ○ · and ✋ on the first falta segment when the card has
// needs_decision). D: n is the number of criteria of the detail ("1
// criterio" for one) and k of those with ✗; a criterion is ✗ when its id is
// in the card's evidence.untraced (the board's or the detail's), ⚑ when its
// traced_by is boardcmd.TracedComando, ✓ otherwise; text is its text.
// Without a spec (the detail's spec does not exist and has no markdown)
// part D is " sin spec: <ruta>", ruta the detail's paths.spec, else
// spec.path, else the card's evidence.spec, under the evidence dir. E:
// " · 1 abierto" or " · <k> abiertos" only when k > 0; severity in its
// color; donde as the Hallazgos view's rows; the description on one line; a
// finding that is not open goes all gray; without findings, "  sin
// hallazgos". F: in UTC, in the v1's order; without entries, "  sin historia
// todavia". G: two spaces in, "  <rol> · <paso> · <tiempo> · último evento
// hace <n>s" (with running: rol, paso and tiempo with " de <limite>"; without
// it, those of the card's newest work row, its tiempo its duration_ms and
// its status after it; "último evento hace <n>s" left out without events),
// "  no se pudo leer el sobre: <err>" in red when Sobre.Err, and one line
// per event "  <HH:MM:SS> <kind> <detail>" (UTC, the detail on one line,
// cut); without work rows, "  sin sobres todavia".
//
// Parts D to G need the Detalle: before it comes they are one part,
// " leyendo la tarjeta..."; with Detalle.Err, " no se pudo leer la
// tarjeta: <Err>" in red.
//
// With 60 <= W <= 99 (the narrow form) AHORA, TE TOCA, SIGUE and EL CAMINO
// are one line each, with no empty line between them, each cut to w-2
// instead of wrapped (the labels padded to 8 cells; their styles as in the
// wide form):
//
//	" AHORA   " + phrase + spaces + G + " "     (the phrase cut so G fits with 2 spaces before it)
//	" TE TOCA " + "enter <label> · <help>"      (or "nada: <text>")
//	" SIGUE   " + phrase
//	" CAMINO  " + the 8 signs + " paso <i> de 8: <name>"
//
// then the empty line and parts B to G. AHORA carries only its phrase: no
// gray line, no bar, no expert lines; SIGUE has no gray line.
//
// ]c and [c (the detail of a card with the focus) make the next and the
// previous ✗ line of part D the first line in view, as far as the scroll
// goes; without one the message line says "no hay mas criterios sin
// prueba" (CA-557). e orders AbrirEditor with the spec's path (relative to
// the root, as in D) and the line, in the spec's markdown, of the first
// criterion that has a line in view (the first markdown line whose text,
// trimmed, starts with "- " + its id + ":"), or 1; without a spec the
// message line says "sin spec: <ruta>".
//
// # The detail of the project (v)
//
// Title "Proyecto" (CA-713). First the four lines of the v2's [1], which
// the header shortens (decision: the v2's [1] keeps its content here), then
// the v2's [0] of [1]:
//
//	" <proyecto> · rama <Rama>"           (" <proyecto> · sin git" without Git)
//	" <the check, as the header's>"
//	" hoy <usd> USD en <n> trabajos"      (" · <k> sin dato" after it when Hoy.SinCosto = k > 0;
//	                                       " hoy sin trabajos" when Hoy.Runs = 0)
//	" fijada: <slug>"                     (" sin tarjeta fijada" without one)
//	""
//	" Veredicto"
//	"  <id> · hace <t> · VERDE"           (ROJO when the verdict is not green; " · parcial" when Partial)
//	"  pass <p> · fail <f> · error <e> · absent <a>"
//	"  .hoom/verdicts/<id>.json"
//	""
//	" Check"
//	"  <the check>"
//	""
//	" Avisos"                             (only with Proyecto.Avisos, one line each, neutralized)
//	"  <aviso>"
//	""
//	" Doctor"
//	"  <slug> · <plain>: <action>"        (one per problem; What when Plain is ""; without
//	                                       "<slug> · " when Slug is "")
//
// " Veredicto" is followed by "  sin veredicto todavia" without one, and
// " Doctor" by "  sin problemas" without problems. Before the first
// Proyecto the document is " leyendo el proyecto...".
//
// # The detail of a finding
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
// min(ContextoArchivo, max(0, (H - F - 1) / 2)), H being the detail's inner
// height and F the number of the document's other lines; M is the
// finding-line sign on the finding's line and a space on the others; the
// numbers are right-aligned to the widest one shown; the text is cut, not
// wrapped. Instead of them: " leyendo <file>..." before the read, " no se
// pudo leer <file>: <Err>" in red when it failed, ` el archivo "<file>" esta
// fuera del proyecto` in red when it leaves the project (by its form, or
// Archivo.Fuera). A finding without a file has neither the empty line
// before " <file>" nor these lines. The finding's line is the v1's: the
// number after a ":" that ends the file field ("x.go:12"), or else after
// the first "<file>:" in the description; 0 without one. The file is
// relative to the evidence dir of the card of its task:
// "<evidence.dir>/<file>" when that card is on the board and its dir is not
// "" nor ".", the file alone otherwise (and an absolute one as it is, for
// the rules of the paths to judge); that is the path the context is read
// from and e opens. Datos.Archivo reads only a regular file. e, in the
// Hallazgos view's list and detail, orders AbrirEditor with that file and
// line; without a file the message line says "el hallazgo no nombra un
// archivo"; with a path that leaves the project it says in red "el archivo
// <path quoted as strconv.Quote> esta fuera del proyecto" and nothing is
// ordered.
//
// # The step of a card
//
// What enter on a card and n on the box's card open (CA-704), the first
// rule that applies:
//
//  1. the card in Hecho: none, "la tarjeta ya esta en Hecho";
//  2. with a pilot of this board alive: none, "el piloto lleva la tarjeta:
//     espera a que se detenga"; with running: none, "el <rol> esta
//     trabajando: espera a que termine";
//  3. with interrupted: none, "el trabajo se corto: R reanuda o pide de nuevo";
//  4. Seguir solo, when it is available for the card (see Seguir solo);
//  5. in Tu aprobacion (boardcmd.ColTuAprobacion): its action aprobar;
//     in Tu aceptacion (boardcmd.ColTuAceptacion): its action integrar;
//  6. without actions: none, "en <column_name> no hay nada que hacer
//     ahora: <plain>";
//  7. its first action, when it is a role action with a key (pedir
//     arquitecto, test-writer, writer, reviewer, reanudar) or guardar;
//  8. otherwise none, "en <column_name> no hay nada que hacer ahora:
//     <plain>".
//
// An action chosen by 5 or 7 that the card does not have is none with
// "en <column_name> no hay <Hace of its key's row>", and one that is
// disabled is none with its why. The step's label is the action's Label
// ("Aprobar spec", "Pedir al writer"), or "Seguir solo".
//
// enter on a card of the list (and in the detail of a card) opens the modal
// of its step (Pedir, Firma, Guardar or Seguir solo), whatever its kind, and
// never launches; its button runs that step's verb. Without a step the
// message line says the rule's text for DuracionAviso and nothing opens
// (CA-705). n does the same with the box's card (the first card of EN COLA
// when the box offers it), chosen or not; without one it says the box's
// text for its case.
//
// # Seguir solo
//
// A new action of the board, not among the card's actions: P, label
// "Seguir solo", verb "hoom piloto <slug>" (CA-715). It is available when
// all of this holds, and when it is not, P opens nothing and the message
// line says, for DuracionAviso, the message of the first reason:
//
//	the item's auto is not item.AutoHastaHumano   el piloto no esta encendido en la tarjeta: hoom item set <slug> --auto hasta-humano
//	its column is not Test-writer, Writer or Review   en <column_name> el piloto no tiene nada que hacer
//	it has running or interrupted                 la tarjeta ya tiene un trabajo en curso o cortado
//	it has red with red.esperado false            la tarjeta esta detenida: mira que fallo antes de seguir
//	a pilot of this board is alive for it         el piloto ya lleva la tarjeta
//	its pilot's stop stands, with a code that is not 0   el piloto se detuvo: <motivo>
//
// A pilot's stop: when a pilot this board launched ends (its FinProceso),
// the model keeps its code, its motivo (its Ultima without the prefix
// "piloto automatico: se detiene: " when it has it, else its Ultima, else
// "hoom piloto salio con <code>"), the card's column then and the running
// id the card had then. The stop stands while the card is in that column
// and has not shown a running with another id (another job ran on it).
//
// The modal Seguir solo (P, and enter and n when the step is Seguir solo;
// CA-716), with the modals' common rules, title "Seguir solo · <T>" (T the
// card's title, the slug when it is empty), its text lines:
//
//	the text, wrapped at the spaces: "El piloto lleva la tarjeta sola: tests, codigo, verify,
//	  review y rondas de hallazgos, hasta tu aceptacion o hasta que algo lo frene. Nunca firma
//	  ni acepta."
//	""
//	"Se va a ejecutar:"
//	"hoom piloto <slug>"
//	""
//	the buttons Seguir solo and Cancelar, Seguir solo chosen at the start
//
// What does not fit goes as in Guardar: the empty lines first, then the
// label joins the command ("Se va a ejecutar: hoom piloto <slug>"). Only
// enter on Seguir solo orders Lanzar{Rol: "piloto", Args: {"piloto",
// <slug>}}, detached as every verb (the pilot goes on when the board
// closes), closes the modal and adds its Registro entry with the label
// "Seguir solo" and the command "hoom piloto <slug>". It is tied to what it
// shows, like Firma: on every read of the board and on enter, Seguir solo
// must still be available for its card; when it is not, the modal closes
// and the message line says in red that reason's message.
//
// A Seguir solo entry's estado: "lanzando…" until the card shows a running
// whose envelope id (or run id without one) it did not have when the order
// was given; "corriendo" after; at its end, when its Ultima starts with
// "piloto automatico: se detiene: ", "se detuvo: <the rest>" (green with
// code 0, red with another), else the v2's rule (`hecho` with 0, `fallo:
// <Ultima>` in red). Its end also puts its Ultima in the message line for
// DuracionAviso, green with code 0 and red otherwise (decision). While its
// process lives (from the Lanzar to its FinProceso) the card is in
// CORRIENDO.
//
// The signature with the pilot on (CA-717): when the card of the Firma modal
// of aprobar has auto item.AutoHastaHumano, the modal adds, right after
// the line "hoom aprobar <slug> --si", the line "y despues: hoom piloto
// <slug>" (and the modal is tied to that too: a change of the card's auto
// closes it as a change of what it shows). When that aprobar ends with code
// 0 (Termino), the model orders Lanzar{Rol: "piloto", Args: {"piloto",
// <slug>}}, with its own Registro entry as above; with another code it
// launches nothing. Without auto the Firma modal is the v2's.
//
// The board does not look at the pilot's lock (.hoom/cache/locks): it only
// knows the pilots it launched.
//
// # The Registro
//
// One entry per verb the TUI runs, per e, and per end of a job (CA-563),
// as Modelo.Registro gives them:
//
//	"<HH:MM> <etiqueta> · <slug> · <estado>"
//	"      <comando>"                          (six spaces)
//
//	"<HH:MM> Abrir en el editor · <file>[:<linea>]"
//	"      <editor command>"
//
//	"<HH:MM> <rol> termino en <slug>: <plain>"  (one line, green or red as the v1's end notice)
//
// HH:MM is the model's clock when the entry was added, in that clock's
// location. etiqueta is the action's label on the card ("Pedir al writer",
// "Aprobar spec", "Guardar"), "Seguir solo", or, for u, "Fijar para la
// terminal". A verb's estado is "lanzando…" until its launch is registered
// (a role or the pilot, whose card shows running with a new envelope:
// "corriendo") or its process ends: `hecho` with code 0, `fallo: <Ultima>`
// in red otherwise (`hoom <verb> salio con <code>` when Ultima is empty),
// and the pilot's as in Seguir solo. The command is the one "Se va a
// ejecutar" showed, on one line (its -m with tarjetacmd.CitarPedido's cut
// to 60 characters; in ASCII with "~" for that cut's "…"). The editor's
// command is Config.Editor and ArgumentosEditor(Config.Editor, <file>,
// <linea>) joined by spaces. Ctrl-e adds no entry.
//
// On screen it is line A-2 (CA-714): the v2's folded Registro,
// "─Registro · " + the first line of its newest entry + line signs up to W
// ("─Registro · 09:58 Pedir al arquitecto · nuc-tenancy · hecho────"), or
// "─Registro" and line signs when it is empty; the end of a job's text in
// its color, a fallo in red, a green or red "se detuvo".
//
// L opens the modal Registro: title "Registro", one text line per line of
// every entry, the oldest on top and the newest at the bottom, each line cut
// to the text's width, or "todavia no se ejecuto nada" when it is empty. It
// is B lines tall when its lines do not fit (then its bottom border says
// "<n> de <m>", n the number of the last line in view and m of all of
// them), opens at its end, scrolls like the Ayuda (↑ ↓ k j one line, pgup
// pgdn its inner height, the wheel 3 lines), and esc or L close it. It runs
// nothing.
//
// # The two last lines
//
// The message line (A-1) is " " and the message: the why, the errors and
// the notices, with their times and colors of the v1 (DuracionAviso,
// DuracionFin); the read errors stay while the last read of their kind
// failed, under any timed message, the board's first, then the
// project's, then the findings'. In expert mode it is " EXPERTO · <mensaje>",
// or " EXPERTO" without a message. While the search is open it is
// " /<texto>▌" (" EXPERTO · /<texto>▌" in expert mode), and no message
// shows.
//
// The keys line (A) is " " and the hints of the current Lugar joined by
// " · " (CA-718): the Barra of each row of AtajosDe(Lugar), in table order,
// but the row with Accion AccionPaso, whose hint is "enter " + the step's
// label in lower case on a card with a step ("enter aprobar spec"), "enter
// abrir" on HECHO's line closed and "enter cerrar" open, and none on a card
// without a step or with nothing chosen. The hint of enter goes first (the
// table puts its rows first) and, with color, in bold (1). Without a card
// (HECHO's line, an empty list, the detail of the project or of a finding,
// the Acciones of a card gone) the hints of the card's rows (DeTarjeta) are
// left out. In the reviewer's Pedir modal, which has no Pedido, the row of
// ctrl-e is neither on the keys line nor dispatched (CA-564). When the line
// does not fit in W cells: a last hint "? todas las teclas" becomes "?
// todas", and then the hints before the last one are left out, from the
// one right before it backward, never the first one when it is enter's,
// until it fits; the last one is cut if it alone does not fit. At 100
// columns, the list on a card in Tu aprobacion, the line is a space and
//
//	enter aprobar spec · n tu siguiente paso · ↑↓ elegir · tab detalle · espacio acciones · ? todas
//
// # Colors
//
// The 16 colors of the terminal (CA-576, CA-720: none with NO_COLOR). Each
// piece of a line with a style is written "\x1b[" + its codes joined by ";"
// + "m" + the text + "\x1b[0m", its codes in this order: 1 (bold), 7
// (reverse), then its color: 31 red, 32 green, 33 yellow, 36 cyan, 90
// gray. Adjacent pieces with the same style are one piece; a piece without
// style is written as it is. The styles:
//
//	the focus (a zone's border, title and right part; a modal's border)   1;32
//	"hoom tablero" in the header                               1;32
//	the box's border: TE TOCA or DETENIDA with a card / not     33 / 90
//	the box's phrase / its plain and its "después:"             1 / 90
//	a group's name: TE TOCA, CORRIENDO, DETENIDA / EN COLA, HECHO   7;33, 7;36, 7;31 / 90
//	a row's second line (and a narrow row's state) and the path's state words,
//	  by group: TE TOCA, CORRIENDO, DETENIDA, EN COLA, HECHO   33, 36, 31, 90, 90
//	the path's signs ● ◆ ◉ ✗ ○                                 32, 33, 36, 31, 90
//	the time bar: run / left                                   36 / 90
//	AHORA, SIGUE, EL CAMINO; TE TOCA without a step / with one   1;90; 1;90 / 1;33
//	AHORA's phrase / its gray line                              1 / 90
//	the hint of enter on the keys line                          1
//	the chosen row of the zone with the focus                  7 (and its pieces' colors: 7;33)
//	the v2's: VERDE; a good message; a green end; HECHO's
//	  meter segments and the end notices                       32
//	red: ROJO, "rojo: <plain>", "fallo: <...>", an error message,
//	  a red end                                                31
//	a finding's severity: high 31, medium 33, low 90
//	secondary and disabled: the origins and the help line of the
//	  selectors, a gray option, action or selector, a closed finding  90
//
// The " · <k> high" of the Hallazgos title is 31 alone, inside the
// border's style. While a modal is open, every cell of lines 1..A-2 outside
// its box (and outside an open list) is drawn, with color, in gray alone
// (90), whatever its own style (CA-565); the message line and the keys line
// keep theirs.
//
// # The keys
//
// Atajos is the one table of the keys line, the help and the dispatch
// (CA-718); Modelo.Lugar says the current Lugar. What each row does:
//
// The zones (CA-706): tab and shift-tab move the focus to the other zone;
// → and l move it from the list to the detail, and esc, ← and h from the
// detail to the list. 1, 2, 3, 4 and 0 do nothing and are not in the help.
// When the focus leaves the detail of the project, the detail shows the
// list's choice again.
//
// In the list (and in the Hallazgos view's list): ↑ ↓ k j move the
// selection one row (it stops at the ends), g and G go to the first and the
// last row, the group lines skipped but HECHO's (CA-707). In the detail:
// ↑ ↓ k j, pgup pgdn, g G scroll it. In the detail with nothing to show
// (LugarVerNada) the keys that scroll are not in its rows.
//
// enter: on a card of the list or in the detail of a card, the modal of its
// step (see The step of a card); on HECHO's line, it opens or closes HECHO;
// in the Hallazgos view's list, the focus to the detail. n: in every Lugar
// of the panels, the modal of the step of the box's card.
//
// The action keys (CA-705): a, t, w, r, R, s, A, I, n, P and enter never
// order Lanzar. a, t, w, r, R, s, A and I, when the card has the row's
// action and it is enabled, open its modal: Pedir for a, t, w, r and R;
// Firma for A and I; Guardar for s. When the card does not have it the
// message line says for DuracionAviso "en <column_name> no hay <Hace>";
// when it is disabled, its why. P opens Seguir solo when it is available
// (see Seguir solo). u orders Lanzar{Args: {"usar", <slug>}} at once,
// without a modal, and adds its Registro entry (CA-560). espacio opens the
// Acciones modal. They act on the list's chosen card, or on the card the
// detail shows (LugaresConTarjeta), or, in Acciones, its card.
//
// H opens and closes the Hallazgos view (CA-712); v shows the project in
// the detail, with the focus on it (CA-713); L opens the modal Registro
// (CA-714). e and ]c [c: see the detail. / opens the search, x toggles the
// expert mode, ? opens the Ayuda modal, q and ctrl-c order Salir (ctrl-c
// also in the search and in every modal).
//
// A card that disappears from the board (CA-578): see Modelo.Leer.
//
// # The modals
//
// Common to all (CA-565). A box of Wm = min(72, W-4) cells, centered over
// lines 1..A-2: its left cell at (W-Wm)/2, its top line at (B-Hm)/2 (lines
// from 0), B = A-2, Hm its lines (at most B). Its border has the focus
// signs and, with color, the focus style; its top border is corner + " " +
// title + " " + line signs + corner (the title cut to Wm-4 cells); its
// bottom border can say "<n> de <m>" as a box's. Each inner line is
//
//	side + " " + mark + text + " " + side
//
// mark being two cells ("▸ " on the line with the focus, two spaces on the
// others) and text Wm-6 cells (cut and padded). While a modal is open the
// keys go to it only (the Lugares of the modal), and the keys line shows
// its keys; esc closes it without running anything (with a list open, it
// closes the list only). The modals that run a verb (Pedir, Firma,
// Guardar, Seguir solo) end with a line of two buttons, centered in the
// text: "<b1>   <b2>", the chosen one as "[ <label> ]" and the other as
// "  <label>  "; ←, → and tab choose the other one (in Pedir, the action and
// Cancelar are two stops of tab and shift-tab), and enter on the buttons
// runs the chosen one: the action (orders Lanzar, closes the modal, adds
// the Registro entry) or Cancelar (closes it). When the size changes the
// modal is centered again.
//
// Pedir, Firma, Guardar, Acciones and Ayuda are the v2's, with their
// content (the spec's no-goals), and with their rules of what does not fit,
// of the command and of being tied to what they show; the only changes are
// the line "y despues: hoom piloto <slug>" of Firma (see Seguir solo) and
// the Ayuda's groups (below). In short:
//
// Pedir (a, t, w, r, R, and enter or n when the step launches a role;
// CA-568), title "<label> · <slug>": "Pedido", the Pedido's box (4 lines),
// the selectors Provider, Modelo, Esfuerzo, Tope and Tiempo límite (the
// label padded to 15, the value "‹ <option> ›", the origin at column 15 +
// V, V = max(26, the widest value + 2)), the help line (gray, at column 15),
// "", "Se va a ejecutar:", the command (1 or 2 lines), "", and the buttons
// (the action, Cancelar). The fields in tab order: Pedido, Provider,
// Modelo, Esfuerzo, Tope, Tiempo límite, the action's button and Cancelar;
// the reviewer has no Pedido. The options of each selector, their gray
// whys, the origins, the list over the modal, otro…, the command's flags
// (only what differs from the proposal), its layout by words in two lines
// with the -m shortened, the "agranda la terminal: el comando no entra
// entero" and what gives up first when it does not fit are the v2's
// (CA-568 to CA-571).
//
// Firma (A, I, and enter or n when the step is a signature; CA-574,
// CA-717): title "<label> · <slug>", the text of tarjetacmd.TextoFirma
// wrapped at the spaces, "", "Se va a ejecutar:", "hoom aprobar <slug>
// --si" (or "hoom integrar <slug> --si"), ["y despues: hoom piloto <slug>"],
// "", and the buttons Cancelar and the action's label, Cancelar chosen at
// the start. Only enter on the action's button orders Lanzar{Args:
// {"aprobar"|"integrar", <slug>, "--si"}}. It is tied to what it shows (its
// card must still have the action enabled, the signature's text be the
// same, and the spec too when the card's Detalle showed it; when not, it
// closes and the message line says in red the action's why or "cambio lo
// que ibas a confirmar en <slug>: vuelve a abrirlo"), and a signature that
// does not fit whole is never signed: enter on the action's button says in
// red "agranda la terminal: la firma no entra entera" and runs nothing.
//
// Guardar (s, and enter or n when the step is guardar): title "<label> ·
// <slug>", "Se guarda en git:", one line per path, "", "Se va a
// ejecutar:", "hoom guardar <slug>", "", and the buttons Guardar and
// Cancelar, the action chosen at the start; it is tied to its paths.
//
// Acciones (espacio; CA-567): title "Acciones · <slug>", one line per
// action of the card in its order (the expert ones only in expert mode):
// "<tecla> <label>  <verbo>"; a disabled action is gray and ends with
// "  (<why>)"; one without a row is gray with "sin verbo en la terminal";
// without actions, "no hay acciones en esta tarjeta" in gray. ↑ ↓ k j move
// the mark over the enabled ones that have a row, enter opens the chosen
// one's modal, and the key of an action does what it does outside. Seguir
// solo is not listed: it is not an action of the card, and P does not work
// in Acciones (decision).
//
// Ayuda (?; CA-718): title "Todas las teclas", its text lines in groups,
// one empty line between groups, each group a line with its title and one
// line per row: "  " + Muestra padded to 18 cells + Hace, and "  " + Verbo
// after it when the row has one (a line that does not fit is wrapped at its
// words, each continuation from the column of Hace, 20). The groups:
// "Tarjetas" (the rows of LugarTarjetas and LugarBusqueda), "Detalle" (the
// rows of LugaresVer), "Hallazgos" (the rows of LugarHallazgos), "Modales"
// (the rows of LugaresModales) and "Mouse". The first is the group of where
// the focus is (Tarjetas with the focus on the cards' list or the search,
// Detalle with it on the detail, Hallazgos on the Hallazgos view's list),
// then the other two of the first three in that order, then Modales and
// Mouse; each row is listed once, in the first group (in that order) whose
// Lugares it works in (decision). "Mouse", with Config.Mouse:
//
//	"  clic              foco a la lista o al detalle; en una tarjeta, la elige"
//	"  clic en la caja   elige la tarjeta de tu siguiente paso"
//	"  rueda             mueve la seleccion de la lista; en el detalle y en la ayuda, desplaza"
//	"  clic en un modal  enfoca un campo, elige una opcion o un boton"
//	"  para seleccionar texto con el mouse, manten Option (macOS) o Shift"
//
// and without it "  apagado: HOOM_MOUSE=0". It is B lines tall when its
// lines do not fit (its bottom border says "<n> de <m>", n the number of
// the last line in view and m of all of them), starts at its first line,
// scrolls with ↑ ↓ k j (one line), pgup pgdn (its inner height) and the
// wheel (3 lines), and esc or ? close it.
//
// Registro (L): see The Registro. Seguir solo: see Seguir solo.
//
// # The mouse
//
// Run asks for it unless HOOM_MOUSE is "0" (ConMouse, SecMouseSi). The
// model never orders Lanzar for a mouse event nor opens a modal (CA-719):
//
//   - A click in the list's box (its borders included, its folded line too)
//     gives it the focus; on a card's line (either of its two lines) it also
//     chooses that card (the detail follows), on HECHO's line it chooses it
//     without opening it, and on a group line it only gives the focus. A
//     click in the detail's box gives it the focus. A click in the box TU
//     SIGUIENTE PASO chooses in the list the box's card (from the Hallazgos
//     view and from the detail of the project it goes back to the cards; it
//     closes the search) and gives the focus to the list; without a card in
//     the box it does nothing. A click that takes the focus out of the list
//     closes the search (as esc); one on a row of the list keeps it open.
//     A click on the header, the Registro's line or the two last lines does
//     nothing.
//   - The wheel over the list moves its selection one row per notch,
//     without moving the focus; over the detail it scrolls it 3 lines; over
//     anything else (the folded list included) it does nothing.
//   - With a modal open: a click outside it does nothing; a click on a
//     field's line (a selector that is not gray, the "Pedido" line or its
//     box) gives it the focus; a click on a button chooses it without
//     running it, and the message line says "enter para <label in lower
//     case>" for DuracionAviso; with a list open, a click on an option that
//     is not gray chooses it and closes the list, and any other click does
//     nothing; in Acciones a click on an action that can be chosen marks
//     it; in Ayuda and Registro the wheel scrolls 3 lines; nothing else
//     does anything.
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
// per change: "\x1b[H" and the Pantalla lines joined by "\r\n", each line
// followed by "\x1b[K" only when it takes fewer than Ancho cells (without
// its SGR): after a line that reaches the last column the cursor stays on
// that cell, and "\x1b[K" would clear it. Run carries out the orders that
// Tecla, Raton, Leer and Termino return.
package tablero
