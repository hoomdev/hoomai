package boardcmd

import "time"

// Candidata is one task with evidence on the board's tree, as `hoom item
// backfill` sees it: the item it would create, or why not.
type Candidata struct {
	Slug   string `json:"slug"`
	Titulo string `json:"titulo"`
	Tipo   string `json:"tipo"`
	// Historico is what the item would carry; "" when it does not qualify.
	Historico string `json:"historico,omitempty"`
	// Motivo is why it does not qualify; "" when it does.
	Motivo string `json:"motivo,omitempty"`
	// ConItem: the task already has an item, which backfill never touches.
	ConItem bool `json:"con_item,omitempty"`
}

// Candidatas lists every task with verdicts or findings on root's trees
// (the ones the doctor's evidencia-sin-item looks at, plus those that
// already have an item), in slug order, each with what backfill would do.
// Read-only.
func Candidatas(root, base, blockOn string, now time.Time) ([]Candidata, error) {
	return nil, nil
}
