package boardcmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/profiles"
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

// tarjetaDe is the card as the rules of the riesgo read it: its task, its
// tipo and its declared riesgo, and whether its item cannot be read (it then
// fails closed to alto, rule 1).
func tarjetaDe(ev Evidence) riesgo.Tarjeta {
	return riesgo.Tarjeta{Tarea: ev.Item.Slug, Tipo: ev.Item.Tipo, Riesgo: declaradoDe(ev.Item),
		Ilegible: ev.RiesgoIlegible != ""}
}

var (
	clasificadorSinYAML     riesgo.Clasificador
	clasificadorSinYAMLOnce sync.Once
)

// clasificadorDe is the classifier the card is measured with: the project's
// (Evidence.Clasificador), or the one of a project without hoom.yaml.
func clasificadorDe(ev Evidence) riesgo.Clasificador {
	if ev.Clasificador != nil {
		return *ev.Clasificador
	}
	clasificadorSinYAMLOnce.Do(func() {
		// without a profile it cannot fail: the base rutas and the default
		// territory of the test-writer
		clasificadorSinYAML, _ = profiles.Clasificador(nil)
	})
	return clasificadorSinYAML
}

// riesgoDe is the card's riesgo with v as its newest verdict
// (finding.RiesgoDeTarjeta): the higher of what v measured and what the item
// declares now; without v, the declared one.
func riesgoDe(ev Evidence, v *verdict.Verdict) riesgo.Evaluacion {
	return finding.RiesgoDeTarjeta(v, tarjetaDe(ev), clasificadorDe(ev))
}

// efectivoEv is the card's riesgo with its newest verdict.
func efectivoEv(ev Evidence) string { return riesgoDe(ev, ev.Verdict).Efectivo }

// DetalleNoAplica is the detail of the segment review of the meter when the
// card does not require one (riesgo-efectivo-y-tamano), exactly, with %d for
// the lines of producto of its newest verdict (its tamano.producto, or
// insertions + deletions for a verdict older than that spec) and %s for the
// lenses ("4 lentes", or "la lente reliability" on a bajo card):
// `el cambio es chico (<p> lineas de producto): no exige la revision de <lentes>`.
//
// The card requires its review (CardEvidence.ReviewRequired) when it has a
// verdict and: with riesgo alto (CardEvidence.Riesgo), always, unless its
// change (changed_files, plus the committed deletions of the current
// fingerprint: Evidence.ArchivosDelCambio) is empty or only documentation
// (reviewcmd.SoloDocs); with bajo and normal, when that producto passes
// reviewcmd.UmbralLineas and the change is not only documentation.
const DetalleNoAplica = "el cambio es chico (%d lineas de producto): no exige la revision de %s"

// reviewRequired is contract 06's rule read from the verdict itself, with
// the card's effective riesgo, over the lines of producto of the verdict
// (productoDe): an empty change or one of only documentation never requires
// it; otherwise alto always does, and bajo and normal with more than
// UmbralLineas lines of producto. The change's files are the verdict's
// changed_files plus, for the verdict of the card's current fingerprint,
// what the branch deleted in a commit (archivosDelVeredicto). Files hoom
// does not know are neither empty nor documentation (aclaracion 25): those
// of a verdict older than riesgo-efectivo-y-tamano with lines and no files,
// and those of the verdict of the current fingerprint when Gather could not
// read its deletions (Evidence.ArchivosDesconocidos, finding a4379f).
func reviewRequired(ev Evidence, v *verdict.Verdict, efectivo string) (int, bool) {
	if v == nil {
		return 0, false
	}
	p := productoDe(v)
	files, desconocidos := archivosDelVeredicto(ev, v)
	desconocidos = desconocidos || len(files) == 0 && v.Riesgo == nil && v.Git.Insertions+v.Git.Deletions > 0
	if !desconocidos && reviewcmd.SoloDocs(files) {
		// SoloDocs of no files is true: the empty change
		return p, false
	}
	if efectivo == riesgo.Alto {
		return p, true
	}
	return p, p > reviewcmd.UmbralLineas
}

// archivosDelVeredicto are the files of v's change: its changed_files and,
// when v certifies the card's current Fingerprint, Evidence.ArchivosDelCambio
// (the deletions changed_files leave out), sorted and without repeats.
// desconocidos is true when v certifies it and Gather could not read them
// (Evidence.ArchivosDesconocidos): its files are then not known.
func archivosDelVeredicto(ev Evidence, v *verdict.Verdict) (files []string, desconocidos bool) {
	if ev.Fingerprint == "" || v.Git.ChangeFingerprint != ev.Fingerprint {
		return v.Git.ChangedFiles, false
	}
	if ev.ArchivosDesconocidos {
		return v.Git.ChangedFiles, true
	}
	if len(ev.ArchivosDelCambio) == 0 {
		return v.Git.ChangedFiles, false
	}
	set := map[string]bool{}
	for _, f := range v.Git.ChangedFiles {
		set[f] = true
	}
	for _, f := range ev.ArchivosDelCambio {
		set[f] = true
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out, false
}

// productoDe is the lines of producto of v: its tamano.producto, or
// insertions + deletions for a verdict without tamano (older than
// riesgo-efectivo-y-tamano), a base harder than the new one, never softer.
func productoDe(v *verdict.Verdict) int {
	if v.Tamano != nil {
		return v.Tamano.Producto
	}
	return v.Git.Insertions + v.Git.Deletions
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
// the card's riesgo (efectivoEv), the rutas of the project's classifier and
// the completa records of its task.
func contextoDe(ev Evidence) finding.Contexto {
	rutas := clasificadorDe(ev).Rutas
	c := finding.Contexto{BlockOn: ev.BlockOn, Subidas: ev.Subidas,
		Riesgos: map[string]string{ev.Item.Slug: efectivoEv(ev)}, Rutas: &rutas}
	if ev.RiesgoIlegible != "" {
		c.Ilegibles = map[string]string{ev.Item.Slug: ev.RiesgoIlegible}
	}
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
