package boardcmd

import "github.com/hoomdev/hoomai/internal/envelope"

// PilotTable is the fixed order of C1 for the role columns a card can
// advance to: the ONLY actions the belt may launch.
var PilotTable = map[string]string{
	ColArquitecto: ActPedirArquitecto,
	ColTestWriter: ActPedirTestWriter,
	ColWriter:     ActPedirWriter,
	ColReview:     ActPedirReviewer,
}

// PilotDecision is what the belt does after a job the Studio launched.
type PilotDecision struct {
	Launch    bool
	Action    string // the table's action: pedir-*
	Role      string
	Provider  string
	BudgetUSD float64
	Pedido    string
	Why       string // why it stops; "" when it launches
}

// Pilot decides the belt's next step. It is a conveyor belt, not an
// orchestrator: pure, no judgment, no retries. before is the card the
// Studio derived to launch the job, closed the job's envelope record when
// its function returned, and after the card derived again at that moment.
func Pilot(before, after Card, closed envelope.Record) PilotDecision {
	return PilotDecision{Why: "sin implementar"}
}
