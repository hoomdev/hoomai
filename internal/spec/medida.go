package spec

// proporcion-por-riesgo: the measure of a spec (its words and its criteria)
// against the tope of the declared riesgo of its card, and the citations per
// criterion of spec_trace. This package depends on neither item nor manifest:
// whoever measures builds the Calibre (approval.CalibreDe) and hands it to
// GatesCon.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// Medida is what Medir counts of one spec.
type Medida struct {
	// Palabras are the words of the WHOLE file: the sequences without
	// blanks that strings.Fields gives, headings, tables, code blocks,
	// verifica markers and every section included (what `wc -w` gives on a
	// text without control characters).
	Palabras int
	// Criterios are the items `- CA-n: ...` of the section of acceptance
	// criteria: the criteria Criteria returns with a non-empty Text. An id
	// only cited or re-expressed in another section does not count; a
	// criterion with a verifica marker counts.
	Criterios int
}

// Medir measures the spec at ruta: its words and its criteria (Medida). An
// empty file measures 0 and 0. A file it cannot read is the error of
// os.ReadFile. Lint and Criteria do not change (CA-311).
func Medir(ruta string) (Medida, error) {
	raw, err := os.ReadFile(ruta)
	if err != nil {
		return Medida{}, err
	}
	m := Medida{Palabras: len(strings.Fields(string(raw)))}
	if len(raw) == 0 {
		return m, nil
	}
	cs, err := Criteria(ruta)
	if err != nil {
		return Medida{}, err
	}
	for _, c := range cs {
		if c.Text != "" {
			m.Criterios++
		}
	}
	return m, nil
}

// riesgoDe is c's riesgo as the texts say it: normal for "".
func riesgoDe(c Calibre) string {
	if c.Riesgo == "" {
		return "normal"
	}
	return c.Riesgo
}

// medidaDe is one measure of DescribirMedida: `<p> <que> de <t>`,
// `<p> <que> de <t> de la tarjeta` or `<p> <que> sin tope`.
func medidaDe(p, t int, deLaTarjeta bool, que string) string {
	switch {
	case t == 0:
		return fmt.Sprintf("%d %s sin tope", p, que)
	case deLaTarjeta:
		return fmt.Sprintf("%d %s de %d de la tarjeta", p, que, t)
	default:
		return fmt.Sprintf("%d %s de %d", p, que, t)
	}
}

// lineaDelTope is the line of one measure that passes its tope.
func lineaDelTope(p, t int, deLaTarjeta bool, que, flag string, c Calibre) string {
	r := riesgoDe(c)
	tope := fmt.Sprintf("el tope con riesgo %s es %d", r, t)
	if deLaTarjeta {
		tope = fmt.Sprintf("el tope de la tarjeta es %d", t)
	}
	if c.Tarjeta == "" {
		return fmt.Sprintf("el spec tiene %d %s y %s: reduce el alcance o sube spec.topes.%s.%s en hoom.yaml", p, que, tope, r, que)
	}
	return fmt.Sprintf(`el spec tiene %d %s y %s: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set %s %s %d --motivo "<por que>"`, p, que, tope, c.Tarjeta, flag, p)
}

// Tope is the tope of a spec for one riesgo: the most words, the most
// criteria and the most citations per criterion it may have. Each value is
// an integer >= 0, and 0 is sin tope: a measure equal to its tope passes
// (800 words with tope 800), one more fails. CitasPorCriterio only warns
// (the aviso of GatesCon): it never fails a gate.
type Tope struct {
	Palabras         int
	Criterios        int
	CitasPorCriterio int
}

// Calibre is how GatesCon measures one spec: its card, the card's declared
// riesgo, the tope of each measure and where it came from, and whether the
// spec has a current approval. approval.CalibreDe builds it from the spec's
// path, the card's item and hoom.yaml.
//
// The zero Calibre is a spec without card, riesgo normal, no tope (every
// measure passes) and without a current approval.
type Calibre struct {
	// Tarjeta is the slug of the spec's card: .hoom/specs/<slug>.md has the
	// card .hoom/items/<slug>.yaml when that item exists (item.TarjetaDe:
	// the main checkout's first). "" = the spec has no card (another path,
	// or no item): it is measured with riesgo normal and its lines name
	// spec.topes instead of `hoom item set`.
	Tarjeta string
	// Ilegible is why the card's item cannot be read (the item's read or
	// parse error, as text); "" = it can. With it the measure is not made:
	// spec_lint is in error with FormatoTarjetaIlegible.
	Ilegible string
	// Riesgo is the card's DECLARED riesgo (riesgo.Vocabulario):
	// riesgo.Normal without card, riesgo.Alto with Ilegible. "" reads as
	// riesgo.Normal.
	Riesgo string
	// Tope is the tope of each measure: the card's own (tope_palabras,
	// tope_criterios) when it has it, else spec.topes.<Riesgo> of hoom.yaml
	// (manifest.Manifest.TopeDelSpec). CitasPorCriterio is always
	// hoom.yaml's: a card has no tope of citations of its own.
	Tope Tope
	// PalabrasDeLaTarjeta says Tope.Palabras is the card's tope_palabras;
	// CriteriosDeLaTarjeta, that Tope.Criterios is its tope_criterios. A
	// card's 0 is sin tope even when hoom.yaml has one, and a card's tope
	// below hoom.yaml's is the card's too.
	PalabrasDeLaTarjeta  bool
	CriteriosDeLaTarjeta bool
	// Aprobado says the spec has a CURRENT approval (approval.Status is
	// approval.StatusApproved): it is not measured against its tope (P7). A
	// spec that changed after its approval is not Aprobado and is measured
	// again.
	Aprobado bool
}

// The exact texts of the measure (proporcion-por-riesgo, «spec_lint, la
// aprobación y hoom spec status»), as format strings.
const (
	// FormatoTarjetaIlegible is spec_lint's note in error, Aprobar's
	// refusal and the end of the line of `hoom spec status` when the card
	// cannot be read: `no se puede leer la tarjeta <slug>: <error>`, with
	// Calibre.Tarjeta and Calibre.Ilegible.
	FormatoTarjetaIlegible = "no se puede leer la tarjeta %s: %s"
	// NotaAprobado ends spec_lint's note in pass and the line of
	// `hoom spec status` for a spec with a current approval.
	NotaAprobado = " · aprobado: el tope no se mide"
	// FormatoAvisoCitas is the aviso GatesCon gives and `hoom verify`
	// prints: `! <k> criterios pasan el tope de <t> citas en pruebas
	// (riesgo <r>): <id> <n>, ...` (the word is criterios also for k 1).
	FormatoAvisoCitas = "! %d criterios pasan el tope de %d citas en pruebas (riesgo %s): %s"
	// FormatoNotaCitas is what spec_trace's note in pass adds:
	// ` · citas por criterio: <id> <n>, ...`.
	FormatoNotaCitas = " · citas por criterio: %s"
	// FormatoNotaPasanCitas ends spec_trace's note in pass when k criteria
	// pass the tope t of citations: ` · <k> pasan el tope de <t> citas`.
	FormatoNotaPasanCitas = " · %d pasan el tope de %d citas"
)

// DescribirMedida is the two measures of m against c, joined by " · ":
// `<medida de palabras> · <medida de criterios>`, each one written
//
//   - `<p> palabras de <t>` with the tope of hoom.yaml,
//   - `<p> palabras de <t> de la tarjeta` with the card's
//     (Calibre.PalabrasDeLaTarjeta),
//   - `<p> palabras sin tope` when the tope is 0 (the card's or hoom.yaml's),
//
// and the same with `criterios`. spec_lint's note in pass and the line of
// `hoom spec status` start with it.
func DescribirMedida(m Medida, c Calibre) string {
	return medidaDe(m.Palabras, c.Tope.Palabras, c.PalabrasDeLaTarjeta, "palabras") + " · " +
		medidaDe(m.Criterios, c.Tope.Criterios, c.CriteriosDeLaTarjeta, "criterios")
}

// LineasDelTope are the lines spec_lint puts in OutputTail for a spec that
// passes its tope, one per measure that passes it, words first: nil when
// every measure is within its tope (the exact tope passes), when the tope
// is 0, when c is Aprobado and when c is Ilegible. With p the measure, t the
// tope, r the riesgo (Calibre.Riesgo, normal for "") and slug the card:
//
//   - with the tope of hoom.yaml:
//     `el spec tiene <p> palabras y el tope con riesgo <r> es <t>: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set <slug> --tope-palabras <p> --motivo "<por que>"`
//   - with the card's tope (Calibre.PalabrasDeLaTarjeta), `el tope de la
//     tarjeta es <t>` in place of `el tope con riesgo <r> es <t>`;
//   - without card (Calibre.Tarjeta ""):
//     `el spec tiene <p> palabras y el tope con riesgo <r> es <t>: reduce el alcance o sube spec.topes.<r>.palabras en hoom.yaml`
//
// and the same with `criterios` and `--tope-criterios <c>`
// (`spec.topes.<r>.criterios`). `<por que>` is literal. The approval
// refuses with these same lines (approval.Aprobar), and the board puts them
// in the card's missing.
func LineasDelTope(m Medida, c Calibre) []string {
	if c.Aprobado || c.Ilegible != "" {
		return nil
	}
	var out []string
	if t := c.Tope.Palabras; t > 0 && m.Palabras > t {
		out = append(out, lineaDelTope(m.Palabras, t, c.PalabrasDeLaTarjeta, "palabras", "--tope-palabras", c))
	}
	if t := c.Tope.Criterios; t > 0 && m.Criterios > t {
		out = append(out, lineaDelTope(m.Criterios, t, c.CriteriosDeLaTarjeta, "criterios", "--tope-criterios", c))
	}
	return out
}

// LineaDeStatus is the line `hoom spec status <ruta>` prints after today's:
//
//   - `hoom spec: <DescribirMedida> (riesgo <r>)`;
//   - without card: `hoom spec: <DescribirMedida> (riesgo normal, sin tarjeta)`;
//   - with a current approval, NotaAprobado at the end:
//     `hoom spec: <DescribirMedida> (riesgo <r>) · aprobado: el tope no se mide`;
//   - with the card Ilegible:
//     `hoom spec: no se puede medir el tope: no se puede leer la tarjeta <slug>: <error>`.
//
// It does not change the exit codes of `hoom spec status` (CA-12).
func LineaDeStatus(m Medida, c Calibre) string {
	if c.Ilegible != "" {
		return "hoom spec: no se puede medir el tope: " + fmt.Sprintf(FormatoTarjetaIlegible, c.Tarjeta, c.Ilegible)
	}
	riesgo := "riesgo " + riesgoDe(c)
	if c.Tarjeta == "" {
		riesgo += ", sin tarjeta"
	}
	l := "hoom spec: " + DescribirMedida(m, c) + " (" + riesgo + ")"
	if c.Aprobado {
		l += NotaAprobado
	}
	return l
}

// GatesCon is Gates measured with c (proporcion-por-riesgo): the same two
// required gates, spec_lint and spec_trace, in that order, plus the aviso of
// citations ("" when there is none). What changes from Gates:
//
// spec_lint:
//   - Calibre.Ilegible: status error, Notes
//     `no se puede leer la tarjeta <slug>: <error>` (FormatoTarjetaIlegible);
//     the measure is not made;
//   - a spec that is not Aprobado and passes its tope: status fail, with
//     one line per measure that passes in OutputTail (LineasDelTope), after
//     the lint issues of today when it also has them;
//   - in pass, Notes is `<DescribirMedida> · riesgo <r> · <n> ids CA-n`, n
//     the ids Lint gives, plus today's `, <k> con [verifica:]` when the spec
//     has verifica commands, and NotaAprobado at the end when c is Aprobado
//     (an approved spec that passes its tope stays in pass).
//
// spec_trace:
//   - its note in pass adds ` · citas por criterio: <id> <n>, ...`
//     (FormatoNotaCitas) with the criteria traced by test only (not those
//     verified by command), in the order of the ids, n from
//     TraceResult.Citas;
//   - when Tope.CitasPorCriterio is above 0 and k criteria have more
//     citations than it, the note ends in ` · <k> pasan el tope de <t> citas`
//     (FormatoNotaPasanCitas) and aviso is
//     `! <k> criterios pasan el tope de <t> citas en pruebas (riesgo <r>): <id> <n>, ...`
//     (FormatoAvisoCitas) with those criteria in the order of the ids. The
//     aviso changes no status: it is not a gate;
//   - a criterion without test leaves spec_trace in fail with any riesgo,
//     as today.
func GatesCon(projectDir, specPath string, c Calibre) (gates []verdict.GateResult, aviso string) {
	start := time.Now()
	lintRes := verdict.GateResult{Name: "spec_lint", Required: true, Scope: "spec"}
	traceRes := verdict.GateResult{Name: "spec_trace", Required: true, Scope: "spec"}

	ids, cmds, issues, err := Lint(specPath)
	var m Medida
	if err == nil && c.Ilegible == "" {
		m, err = Medir(specPath)
	}
	lintRes.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		lintRes.Status = verdict.StatusError
		lintRes.Notes = err.Error()
		traceRes.Status = verdict.StatusSkipped
		return []verdict.GateResult{lintRes, traceRes}, ""
	}
	lineas := LineasDelTope(m, c)
	switch {
	case c.Ilegible != "":
		lintRes.Status = verdict.StatusError
		lintRes.Notes = fmt.Sprintf(FormatoTarjetaIlegible, c.Tarjeta, c.Ilegible)
	case len(issues) > 0 || len(lineas) > 0:
		lintRes.Status = verdict.StatusFail
		lintRes.OutputTail = strings.Join(append(append([]string{}, issues...), lineas...), "\n")
	default:
		lintRes.Status = verdict.StatusPass
		lintRes.Notes = fmt.Sprintf("%s · riesgo %s · %d ids CA-n", DescribirMedida(m, c), riesgoDe(c), len(ids))
		if len(cmds) > 0 {
			lintRes.Notes += fmt.Sprintf(", %d con [verifica:]", len(cmds))
		}
		if c.Aprobado {
			lintRes.Notes += NotaAprobado
		}
	}

	t := time.Now()
	tr, terr := Trace(projectDir, ids, cmds)
	traceRes.DurationMS = time.Since(t).Milliseconds()
	var citas, pasan []string
	if terr == nil {
		tope := c.Tope.CitasPorCriterio
		for _, id := range ids {
			n, ok := tr.Citas[id]
			if !ok || len(cmds[id]) > 0 {
				continue
			}
			cita := fmt.Sprintf("%s %d", id, n)
			citas = append(citas, cita)
			if c.Ilegible == "" && tope > 0 && n > tope {
				pasan = append(pasan, cita)
			}
		}
		if len(pasan) > 0 {
			aviso = fmt.Sprintf(FormatoAvisoCitas, len(pasan), tope, riesgoDe(c), strings.Join(pasan, ", "))
		}
	}
	switch {
	case terr != nil:
		traceRes.Status = verdict.StatusError
		traceRes.Notes = terr.Error()
	case len(ids) == 0:
		traceRes.Status = verdict.StatusSkipped
		traceRes.Notes = "sin criterios CA-* que trazar"
	case len(tr.MissingTests) > 0 || len(tr.CmdFailed) > 0:
		traceRes.Status = verdict.StatusFail
		var parts []string
		if len(tr.MissingTests) > 0 {
			parts = append(parts, fmt.Sprintf("criterios SIN test que los referencie: %s\nAccion: el test-writer debe crear tests que mencionen cada CA-n (en el nombre o un comentario), o el spec declarar [verifica: <comando>] si el criterio se verifica por comando.", strings.Join(tr.MissingTests, ", ")))
		}
		if len(tr.CmdFailed) > 0 {
			parts = append(parts, fmt.Sprintf("criterios con comando [verifica:] FALLIDO:\n%s\nAccion: corrige el comando o el entorno; exit 0 = criterio trazado.", strings.Join(tr.CmdFailed, "\n")))
		}
		traceRes.OutputTail = strings.Join(parts, "\n")
		traceRes.Notes = fmt.Sprintf("%d archivos de test escaneados", tr.Scanned)
	default:
		traceRes.Status = verdict.StatusPass
		if tr.ByCmd > 0 {
			traceRes.Notes = fmt.Sprintf("%d/%d criterios: %d trazados por test (%d archivos), %d verificados por comando", len(ids), len(ids), tr.ByTest, tr.Scanned, tr.ByCmd)
		} else {
			traceRes.Notes = fmt.Sprintf("%d/%d criterios trazados en %d archivos de test", len(ids), len(ids), tr.Scanned)
		}
		if len(citas) > 0 {
			traceRes.Notes += fmt.Sprintf(FormatoNotaCitas, strings.Join(citas, ", "))
		}
	}
	if len(pasan) > 0 && (traceRes.Status == verdict.StatusPass || traceRes.Status == verdict.StatusFail) {
		traceRes.Notes += fmt.Sprintf(FormatoNotaPasanCitas, len(pasan), c.Tope.CitasPorCriterio)
	}
	return []verdict.GateResult{lintRes, traceRes}, aviso
}
