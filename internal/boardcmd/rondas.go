package boardcmd

import (
	"fmt"
	"strings"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

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
	recibidos := map[string]bool{}
	for _, s := range sobres {
		if s.Role != "refutador" || s.Stage == "run" {
			continue
		}
		for _, id := range s.Hallazgos {
			recibidos[id] = true
		}
	}
	var out []string
	for _, id := range bloqueantes {
		if !recibidos[id] {
			out = append(out, id)
		}
	}
	return out
}

// RazonTopeDeRondas is the reason of a card at the cap of its rounds of
// findings (RondasDeHallazgos.Razon, Piloto's stop), with n the rounds done
// (reviewcmd.Rondas) and k the findings that block:
//
//	se hicieron <n> rondas de hallazgos y quedan <k> que bloquean: te toca
func RazonTopeDeRondas(n, k int) string {
	return fmt.Sprintf("se hicieron %d rondas de hallazgos y quedan %d que bloquean: te toca", n, k)
}

// PedidoCorreccion is the writer's pedido in Review with blocking findings
// (the card's pedir-writer there), with ids the card's blocking findings
// (CardEvidence.BlockingFindings: all of them, whether a refutador saw them
// or not, in the order of `hoom finding list`) joined with ", ". The round
// by hand is the refutador and then ONE writer with every blocking finding:
//
//	Corregi los hallazgos que bloquean la tarjeta: <ids>. Solo esos: los pendientes quedan abiertos.
func PedidoCorreccion(ids []string) string {
	return "Corregi los hallazgos que bloquean la tarjeta: " + strings.Join(ids, ", ") + ". Solo esos: los pendientes quedan abiertos."
}

// sobresCerrados are the records of the card's CLOSED envelopes
// (envelope.Record.Done), the ones Piloto sees too.
func sobresCerrados(ev Evidence) []envelope.Record {
	var out []envelope.Record
	for _, e := range ev.Envelopes {
		if e.Record.Done() {
			out = append(out, e.Record)
		}
	}
	return out
}

// sinRefutar are the card's blocking findings that no refutador received:
// BloqueantesSinRefutar over its closed envelopes.
func sinRefutar(ev Evidence) []string {
	return BloqueantesSinRefutar(blocking(ev), sobresCerrados(ev))
}

// rondasDe is Card.Rondas: the rounds of the card's task, its cap and
// whether it is at it. A card in Hecho, or whose workspace cannot be used
// (nothing else of it can be read), is never at the cap.
func rondasDe(ev Evidence, column string, bloqueantes []string) RondasDeHallazgos {
	r := RondasDeHallazgos{Hechas: reviewcmd.Rondas(ev.Reviews, ev.Item.Slug), Tope: ev.TopeDeRondas}
	if r.Tope == 0 {
		r.Tope = manifest.DefaultMaxRondas
	}
	k := len(bloqueantes)
	r.AlTope = r.Hechas >= r.Tope && k > 0 && column != ColHecho && ev.WorktreeInusable == ""
	if r.AlTope {
		r.Razon = RazonTopeDeRondas(r.Hechas, k)
	}
	return r
}

// alTopeQuita says whether the cap takes the action id away: pedir-writer
// and pedir-refutador, and the reanudar and relanzar of a writer's or
// refutador's interrupted job. pedir-reviewer stays: it is how a card leaves
// the cap after fixing by hand.
func alTopeQuita(c Card, id string) bool {
	if !c.Rondas.AlTope {
		return false
	}
	switch id {
	case ActPedirWriter, ActPedirRefutador:
		return true
	case ActReanudar, ActRelanzar:
		return c.Interrupted != nil && (c.Interrupted.Role == "writer" || c.Interrupted.Role == "refutador")
	}
	return false
}
