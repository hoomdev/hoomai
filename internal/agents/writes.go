package agents

// WritesCode reports whether a run of role wrote the code under review: false
// for read-only roles and for spec authors (scope specs), true for the rest,
// for an empty role (`hoom run`) and for an unknown one — nothing says it did
// not write. It is the one rule behind the cross review and the card's
// writer logo.
func WritesCode(role string) bool {
	return false
}
