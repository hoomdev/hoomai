package boardcmd

import "github.com/hoomdev/hoomai/internal/envelope"

// ciclo-lo-decide-hoom: after a review, what is fixed, what the refutador
// sees and how many rounds there are is decided by hoom, by any path — the
// pilot, `hoom siguiente`, `hoom pedir` or the orchestrator. The rules live
// here once; Derive (the card, which the verbs read) and Piloto use them.

// BloqueantesSinRefutar are the ids of bloqueantes, in their order, that are
// not in the Hallazgos of any of sobres with Role "refutador" and a Stage
// other than "run": «los hallazgos que bloquean y que no estan en el campo
// hallazgos de ningun sobre de refutador de la tarjeta cuyo stage no es
// run». A refutador's envelope cut in `run` does not count as received: its
// findings stay without refuting. The envelopes of other roles never count.
// nil when there is none.
//
// It is the ONE rule, the pilot's of before this spec: Piloto gives it the
// ids of Estado.Bloqueantes and the records of Estado.Sobres (the round's
// refutador); Derive gives it CardEvidence.BlockingFindings and the records
// of Evidence.Envelopes (the card's pedir-refutador and its Hallazgos).
func BloqueantesSinRefutar(bloqueantes []string, sobres []envelope.Record) []string {
	return nil
}

// RazonTopeDeRondas is the reason of a card at the cap of its rounds of
// findings (RondasDeHallazgos.Razon, Piloto's stop), with n the rounds done
// (reviewcmd.Rondas) and k the findings that block:
//
//	se hicieron <n> rondas de hallazgos y quedan <k> que bloquean: te toca
func RazonTopeDeRondas(n, k int) string {
	return ""
}

// PedidoCorreccion is the writer's pedido in Review with blocking findings
// (the card's pedir-writer there), with ids the card's blocking findings
// (CardEvidence.BlockingFindings: all of them, whether a refutador saw them
// or not, in the order of `hoom finding list`) joined with ", ". The round
// by hand is the refutador and then ONE writer with every blocking finding:
//
//	Corregi los hallazgos que bloquean la tarjeta: <ids>. Solo esos: los pendientes quedan abiertos.
func PedidoCorreccion(ids []string) string {
	return ""
}
