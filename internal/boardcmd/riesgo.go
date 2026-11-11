package boardcmd

import (
	"fmt"
	"strings"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// declaradoDe is the riesgo the item declares: normal without the key.
func declaradoDe(it item.Item) string {
	if it.Riesgo == "" {
		return riesgo.Normal
	}
	return it.Riesgo
}

// efectivoDe is the riesgo the board applies to the card: the item's, over
// the changed_files of the verdict v (the whole change against the base).
func efectivoDe(it item.Item, v *verdict.Verdict) string {
	var files []string
	if v != nil {
		files = v.Git.ChangedFiles
	}
	ef, _ := riesgo.Efectivo(declaradoDe(it), files)
	return ef
}

// efectivoEv is efectivoDe for the card's evidence.
func efectivoEv(ev Evidence) string { return efectivoDe(ev.Item, ev.Verdict) }

// reviewRequired is contract 06's rule read from the verdict itself, with
// the card's effective riesgo: alto requires the review whenever the change
// has files and is not only documentation; bajo and normal, with more than
// UmbralLineas changed lines (exactly when the verdict prints "la review
// exige las 4 lentes").
func reviewRequired(v *verdict.Verdict, efectivo string) (int, bool) {
	if v == nil {
		return 0, false
	}
	n := v.Git.Insertions + v.Git.Deletions
	if efectivo == riesgo.Alto {
		return n, len(v.Git.ChangedFiles) > 0 && !reviewcmd.SoloDocs(v.Git.ChangedFiles)
	}
	return n, n > reviewcmd.UmbralLineas
}

// lentesPedidas are the lenses the rule asks a record of the chain for:
// with bajo, reliability in a completa and in a delta; with normal and alto,
// the 4 in a completa and all but readability in a delta.
func lentesPedidas(efectivo, cobertura string) []string {
	switch {
	case efectivo == riesgo.Bajo:
		return []string{reviewcmd.LenteDominante}
	case cobertura == reviewcmd.CoberturaDelta:
		return reviewcmd.LentesDeUnaDelta
	}
	return reviewcmd.Lentes
}

// textoLentes is what the card's texts call the review the rule asks for:
// "la lente reliability" on a bajo card, "4 lentes" otherwise.
func textoLentes(efectivo string) string {
	if efectivo == riesgo.Bajo {
		return "la lente " + reviewcmd.LenteDominante
	}
	return "4 lentes"
}

// faltaReview is the reason and the plain text of a review the card
// requires and does not have, for its effective riesgo.
func faltaReview(efectivo string, lines int) (motivo, plain string) {
	switch efectivo {
	case riesgo.Alto:
		return "la review exige las 4 lentes (riesgo alto) y no hay registro de review",
			"falta la revision de 4 lentes (riesgo alto)"
	case riesgo.Bajo:
		return fmt.Sprintf("la review exige la lente reliability (%d lineas > %d, riesgo bajo) y no hay registro de review", lines, reviewcmd.UmbralLineas),
			fmt.Sprintf("falta la revision de la lente reliability (%d lineas)", lines)
	}
	return fmt.Sprintf("la review exige las 4 lentes (%d lineas > %d) y no hay registro de review", lines, reviewcmd.UmbralLineas),
		fmt.Sprintf("falta la revision de 4 lentes (%d lineas)", lines)
}

// contextoDe is the Contexto the card's findings are judged with
// (finding.Bloquea): the board's threshold, the valid raises of its tree,
// the riesgo its item declares and the completa records of its task.
func contextoDe(ev Evidence) finding.Contexto {
	c := finding.Contexto{BlockOn: ev.BlockOn, Subidas: ev.Subidas,
		Riesgos: map[string]string{ev.Item.Slug: declaradoDe(ev.Item)}}
	for _, r := range ev.Reviews {
		if r.Cobertura == reviewcmd.CoberturaCompleta {
			c.Completas = append(c.Completas, finding.RevisionCompleta{ID: r.ID, Task: r.Task, Findings: r.Findings})
		}
	}
	return c
}

// juzgadas are the card's open findings with what Bloquea says of each.
func juzgadas(ev Evidence) []finding.Item {
	c := contextoDe(ev)
	var out []finding.Item
	for _, f := range ev.Findings {
		if f.Status != finding.StatusOpen || f.Task != ev.Item.Slug {
			continue
		}
		f.Bloquea, f.Motivo = finding.Bloquea(f.Finding, c)
		out = append(out, f)
	}
	return out
}

// conMotivo is a blocking finding as the card's reason names it: its id,
// and its motivo when it blocks by a raise, a risk path or riesgo alto.
func conMotivo(f finding.Item) string {
	if f.Motivo == "" || strings.HasPrefix(f.Motivo, "severidad ") {
		return f.ID
	}
	return f.ID + " (" + f.Motivo + ")"
}

// pendientesDe are the card's open findings that do not block.
func pendientesDe(ev Evidence) Pendientes {
	p := Pendientes{IDs: []string{}}
	for _, f := range juzgadas(ev) {
		if f.Bloquea {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(f.Severity)) {
		case "medium":
			p.Medium++
		case "low":
			p.Low++
		default:
			continue
		}
		p.IDs = append(p.IDs, f.ID)
	}
	if c := finding.CuentaPendientes(p.Medium, p.Low); c != "" {
		p.Texto = "pendientes: " + c + " (no bloquean)"
	}
	return p
}
