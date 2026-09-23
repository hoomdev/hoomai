package spec

// Criterion is one acceptance criterion as written: its id and the statement
// of its own bullet in the criteria section ("" when the id is only cited
// elsewhere in the spec).
type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Criteria returns one Criterion per id of Lint, in Lint's order.
func Criteria(path string) ([]Criterion, error) {
	return nil, nil
}

// TokenIndex is one pass over the test files of a tree: for every whole
// CA-n token, the files that cite it.
type TokenIndex struct {
	Scanned int // test files read
	files   map[string][]string
}

// IndexTokens reads the test files of root once (same filter as Trace).
func IndexTokens(root string) (TokenIndex, error) {
	return TokenIndex{}, nil
}

// Files returns the test files citing id, relative to the indexed root,
// slash-separated and sorted. Never nil.
func (x TokenIndex) Files(id string) []string {
	return []string{}
}
