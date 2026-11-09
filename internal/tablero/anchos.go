package tablero

// AnchoDe is how many cells of the terminal the rune r takes, by hoom's own
// width table (CA-577; no go-runewidth, no uniseg):
//
//   - 0 for a combining mark: the categories Mn and Me of Go's unicode
//     tables (U+0301, U+FE0F the emoji variation selector);
//   - 2 for East Asian Wide (W) and Fullwidth (F) in the EastAsianWidth.txt
//     of Unicode 16.0.0, which includes every emoji that presents as emoji
//     by default (Emoji_Presentation=Yes): 漢, 字, Ａ (U+FF21), 🚀, ✋;
//   - 1 for everything else, the Ambiguous and Neutral ones included: every
//     sign of the screen (● ○ · ✓ ✗ ⚑ ▸ ▾ ‹ › … ▌ and the box lines ─ │ ═ ║
//     and their corners) takes one cell; only ✋ among them takes two.
//
// The controls never get here: the screen neutralizes them first (see the
// package doc).
func AnchoDe(r rune) int {
	return 0
}

// Celdas is how many cells s takes: the sum of AnchoDe of its runes. Every
// width of the screen (the panels, the cuts, the paddings, the mouse's X)
// is counted with it.
func Celdas(s string) int {
	return 0
}
