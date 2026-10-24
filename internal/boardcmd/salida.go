package boardcmd

import (
	"strconv"
	"strings"
	"unicode"
)

// conControl says whether s has a control character or one that changes
// the direction of the text (U+202E and the like).
func conControl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)
	}) >= 0
}

// textoSeguro is a value from the evidence or from git (a severity, an id, a
// file name, a branch, git's stderr) as it goes into text a terminal prints
// (a reason, a warning, the doctor's error): as it is when it is plain text,
// quoted with its control and direction characters escaped when it is not
// (findings 763d5f, feeb49, 49af1d, 769721). It is the board's one place
// for that: every such value goes through it.
func textoSeguro(s string) string {
	if conControl(s) {
		return strconv.Quote(s)
	}
	return s
}
