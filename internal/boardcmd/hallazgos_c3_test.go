// Tests de regresion de la review de 4 lentes de la cabina C3 sobre
// .hoom/specs/acciones-desde-la-tarjeta.md, en boardcmd (CA-327: en la review
// el budget_usd propuesto es remaining_usd dividido por las lentes, porque
// `hoom review` le pasa --budget-usd a cada pasada):
//
//   - 20261014T181136_285834 (readability): ReviewLenses repite a mano la
//     cantidad de lentes que corre `hoom review`. Guarda: tiene que ser igual
//     a len(reviewcmd.Lentes), para que el presupuesto por lente no se
//     desfase si la lista cambia. Pasa hoy.
package boardcmd

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

// CA-327 (hallazgo 285834), guarda: el divisor del presupuesto de la review
// es la cantidad de lentes que `hoom review` corre.
func TestHallazgo_285834_GuardaReviewLensesEsLaCantidadDeLentes(t *testing.T) {
	if len(reviewcmd.Lentes) == 0 {
		t.Fatal("CA-327: fixture: hoom review corre al menos una lente")
	}
	if ReviewLenses != len(reviewcmd.Lentes) {
		t.Fatalf("CA-327 (hallazgo 285834): ReviewLenses (%d) es la cantidad de lentes que corre hoom review, len(reviewcmd.Lentes) = %d (%v)",
			ReviewLenses, len(reviewcmd.Lentes), reviewcmd.Lentes)
	}
}
