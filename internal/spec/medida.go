package spec

// proporcion-por-riesgo: the measure of a spec (its words and its criteria)
// against the tope of the declared riesgo of its card, and the citations per
// criterion of spec_trace. This package depends on neither item nor manifest:
// whoever measures builds the Calibre (approval.CalibreDe) and hands it to
// GatesCon.

import (
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
	return Medida{}, nil
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
	return ""
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
	return nil
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
	return ""
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
	return nil, ""
}
