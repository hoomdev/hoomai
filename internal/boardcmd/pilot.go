package boardcmd

import "github.com/hoomdev/hoomai/internal/item"

// PilotoNoSigue is the stop of the pilot on the card c, read again under its
// locks after Piloto decided, when it must not act on it: its item no longer
// has auto hasta-humano (caso 1, with case 1's Why), or it is in Hecho or in
// a human column (caso 3, with case 3's Why: `hoom piloto` exits 0)
// (aclaraciones 13, 20 and 22). caso 0 and "" when it may. lanzar.Lanzar
// checks it for a launch with Pilot, and the pilot's cerrar-hallazgos before
// its first resolution.
func PilotoNoSigue(c Card) (caso int, why string) {
	if c.Item.Auto != item.AutoHastaHumano {
		return 1, "el piloto automatico no esta activado en la tarjeta"
	}
	if c.Column == ColHecho {
		return 3, "la tarjeta llego a Hecho: no queda trabajo"
	}
	for _, col := range Columns {
		if col.ID == c.Column && col.Human {
			return 3, "la tarjeta llego a " + col.Name + ": le toca a una persona"
		}
	}
	return 0, ""
}

// PilotTable is the fixed order of C1 for the role columns a card can
// advance to: the ONLY column actions the pilot may launch. Arquitecto is not
// in it: a spec that goes back to that column is fixed by a person
// (autonomia-entre-firmas).
var PilotTable = map[string]string{
	ColTestWriter: ActPedirTestWriter,
	ColWriter:     ActPedirWriter,
	ColReview:     ActPedirReviewer,
}

// PilotDecision is what Piloto decides (autonomia-entre-firmas).
type PilotDecision struct {
	// Launch is true only when Paso is PasoLanzar.
	Launch    bool
	Action    string // the action it launches: pedir-* (pedir-refutador included)
	Role      string
	Provider  string
	BudgetUSD float64 // the USD budget of the launch; 0 = launched without a USD budget
	Pedido    string
	Why       string // why it stops; "" when it does not stop

	// autonomia-entre-firmas.

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

func isHumanColumn(id string) bool {
	for _, c := range Columns {
		if c.ID == id {
			return c.Human
		}
	}
	return false
}
