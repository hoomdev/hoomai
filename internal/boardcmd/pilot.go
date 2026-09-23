package boardcmd

import (
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/item"
)

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
// It stops at the first case that holds, in the spec's order; every launch
// needs the column to advance and the role to change, so the belt always
// ends.
func Pilot(before, after Card, closed envelope.Record) PilotDecision {
	stop := func(why string) PilotDecision { return PilotDecision{Why: why} }
	role := closed.Role
	if role == "" {
		role = "rol"
	}
	name := after.Column
	for _, c := range Columns {
		if c.ID == after.Column {
			name = c.Name
		}
	}

	if after.Item.Auto != item.AutoHastaHumano {
		return stop("el piloto automatico no esta activado en la tarjeta")
	}
	if closed.Status != envelope.StatusDeliverable {
		status := closed.Status
		if status == "" {
			status = "sin cerrar"
		}
		return stop("el trabajo del " + role + " no cerro entregable (" + status + "): el piloto se detiene ante un rojo")
	}
	if after.Red != nil {
		return stop("la tarjeta esta en rojo: " + after.Red.Plain)
	}
	if after.Running != nil || after.Interrupted != nil {
		return stop("la tarjeta tiene otro trabajo en curso o interrumpido")
	}
	if after.Column == ColHecho {
		return stop("la tarjeta llego a Hecho: no queda trabajo")
	}
	if isHumanColumn(after.Column) {
		return stop("la tarjeta llego a " + name + ": le toca a una persona")
	}
	if columnIndex(after.Column) <= columnIndex(before.Column) {
		return stop("la tarjeta no avanzo (sigue en " + name + "): el piloto no reintenta")
	}
	want, inTable := PilotTable[after.Column]
	if !inTable || len(after.Actions) == 0 || after.Actions[0].ID != want {
		return stop("lo que sigue en " + name + " no esta en la tabla del piloto: le toca a una persona")
	}
	a := after.Actions[0]
	if a.Role == closed.Role {
		return stop("seria volver a pedirle al " + role + ": el piloto no reintenta")
	}
	if !a.Enabled {
		return stop(a.Why)
	}
	if after.Item.PresupuestoUSD == nil || a.BudgetUSD == nil {
		return stop("la tarjeta no tiene presupuesto: el piloto automatico solo corre con un tope")
	}
	provider := ""
	for _, o := range a.Providers {
		if o.OK && o.Budget && o.Name == closed.Provider {
			provider = o.Name
		}
	}
	for _, o := range a.Providers {
		if provider == "" && o.OK && o.Budget {
			provider = o.Name
		}
	}
	if provider == "" {
		return stop("ningun proveedor con tope de presupuesto puede tomar este trabajo")
	}
	return PilotDecision{Launch: true, Action: a.ID, Role: a.Role, Provider: provider,
		BudgetUSD: *a.BudgetUSD, Pedido: a.Pedido}
}

func isHumanColumn(id string) bool {
	for _, c := range Columns {
		if c.ID == id {
			return c.Human
		}
	}
	return false
}
