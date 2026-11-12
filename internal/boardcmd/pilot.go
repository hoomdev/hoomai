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

// PilotDecision is what the belt does after a job the Studio launched, and
// what Piloto decides (autonomia-entre-firmas).
type PilotDecision struct {
	// Launch is true only when Paso is PasoLanzar (Pilot: when it launches).
	Launch    bool
	Action    string // the action it launches: pedir-* (pedir-refutador included)
	Role      string
	Provider  string
	BudgetUSD float64 // the USD budget of the launch; 0 = launched without a USD budget
	Pedido    string
	Why       string // why it stops; "" when it does not stop

	// autonomia-entre-firmas (Piloto; Pilot leaves them empty).

	// Paso is PasoLanzar, PasoCerrarHallazgos or PasoDetenerse.
	Paso string
	// Regla is the rule of a PasoLanzar or PasoCerrarHallazgos (Regla*); ""
	// when it stops.
	Regla string
	// Intento is 2 in a launch with ReglaReintento (and in its relevo); 0
	// otherwise.
	Intento int
	// Ronda is n inside the round of findings n (the round's launches and
	// its PasoCerrarHallazgos); 0 outside a round.
	Ronda int
	// Resume is the provider's session id a relevo-por-tiempo resumes; ""
	// = a new session.
	Resume string
	// Delta asks the review to continue the card's chainable review
	// (lanzar.Request.Delta).
	Delta bool
	// Hallazgos are the finding ids the step works on: the ones the
	// refutador or the round's test-writer receives, or the ones
	// PasoCerrarHallazgos closes. nil when the step has none.
	Hallazgos []string
	// Bloqueantes is k, how many findings block, in the FIRST launch of a
	// round (the record's `bloqueantes`); 0 in any other decision.
	Bloqueantes int
	// Evidencias are, for PasoCerrarHallazgos, the literal evidence each
	// finding of Hallazgos is closed with, by id:
	// `ronda <n>: el test <ruta> cita el hallazgo; veredicto <id> rojo antes del arreglo; despues, veredicto <id> con todos los demas gates en pass`.
	// nil in any other decision.
	Evidencias map[string]string
	// Caso is the number of the case of the pilot's table that decided (1 to
	// 14); 0 when none held and it launches with ReglaCadena.
	Caso int
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
