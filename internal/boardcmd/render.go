package boardcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// JSONBytes renders the board exactly as `hoom board --json` emits it.
func JSONBytes(b Board) ([]byte, error) {
	if b.Warnings == nil {
		b.Warnings = []string{}
	}
	return json.MarshalIndent(b, "", "  ")
}

// Render prints the board for humans: the eight columns always, in order,
// with the reason each card is where it is and the command that moves it.
func Render(w io.Writer, b Board) {
	total := 0
	for _, col := range b.Columns {
		total += len(col.Cards)
	}
	if total == 0 {
		fmt.Fprintln(w, `hoom board: sin items (crea uno con 'hoom item add "<titulo>"')`)
		renderWarnings(w, b.Warnings)
		return
	}
	fmt.Fprintf(w, "hoom board: %d %s (columna derivada de la evidencia; nada guardado)\n", total, plural(total, "tarjeta", "tarjetas"))
	for _, col := range b.Columns {
		head := fmt.Sprintf("%s (%d)", col.Name, len(col.Cards))
		if col.Human {
			head += " - esperando humano"
		}
		fmt.Fprintf(w, "\n%s\n", head)
		for _, c := range col.Cards {
			fmt.Fprintf(w, "  %-28s %s\n", c.Slug, c.Item.Titulo)
			if len(c.Missing) > 0 {
				fmt.Fprintf(w, "    %s\n", recortar(c.Missing[0]))
			}
			if c.Next != "" {
				fmt.Fprintf(w, "    siguiente: %s\n", c.Next)
			}
			for _, l := range substateLines(c) {
				fmt.Fprintf(w, "    %s\n", l)
			}
		}
	}
	renderWarnings(w, b.Warnings)
}

// RenderCard prints one card for humans (`hoom item show`): the item, where
// the card is and why, and every substate.
func RenderCard(w io.Writer, c Card) {
	it := c.Item
	fmt.Fprintf(w, "hoom item: %s - %s\n", c.Slug, it.Titulo)
	fmt.Fprintf(w, "  %s · prioridad %s · creado por %s el %s\n",
		it.Tipo, it.Prioridad, it.CreadoPor, it.CreadoEn.Format("2006-01-02 15:04 UTC"))
	if it.PresupuestoUSD != nil {
		fmt.Fprintf(w, "  presupuesto: %s USD\n", usd(*it.PresupuestoUSD))
	}
	if p := strings.TrimSpace(it.Pedido); p != "" {
		fmt.Fprintln(w, "  pedido:")
		for _, l := range strings.Split(p, "\n") {
			fmt.Fprintf(w, "    %s\n", l)
		}
	}
	col := c.ColumnName
	if c.WaitingHuman {
		col += " (esperando humano)"
	}
	fmt.Fprintf(w, "  columna: %s\n", col)
	for i, m := range c.Missing {
		if i == 0 {
			fmt.Fprintf(w, "  falta: %s\n", m)
			continue
		}
		fmt.Fprintf(w, "         %s\n", m)
	}
	if c.Next != "" {
		fmt.Fprintf(w, "  siguiente: %s\n", c.Next)
	}
	if it.HechoEn != nil {
		fmt.Fprintf(w, "  hecho: %s · commit_final %s\n", it.HechoEn.Format("2006-01-02 15:04 UTC"), it.CommitFinal)
	}
	e := c.Evidence
	fmt.Fprintf(w, "  evidencia: %s (%s)\n", e.Dir, e.Source)
	if e.SpecExists {
		lint := "pasa spec_lint"
		if len(e.LintIssues) > 0 {
			lint = fmt.Sprintf("%d issues de lint", len(e.LintIssues))
		}
		fmt.Fprintf(w, "    spec %s: %s · aprobacion %s · %d/%d criterios con test\n",
			e.Spec, lint, e.Approval, e.Traced, e.Criteria)
	} else {
		fmt.Fprintf(w, "    spec %s: no existe\n", e.Spec)
	}
	if e.VerdictID != "" {
		huella := "huella distinta"
		if e.FingerprintMatch {
			huella = "huella coincide"
		}
		fmt.Fprintf(w, "    veredicto %s: %s (%s)\n", e.VerdictID, e.Verdict, huella)
	}
	if e.ReviewRequired || e.ReviewID != "" {
		rev := "sin registro"
		if e.ReviewID != "" {
			rev = ".hoom/reviews/" + e.ReviewID + ".json"
		}
		fmt.Fprintf(w, "    review de 4 lentes: %s\n", rev)
	}
	if len(e.BlockingFindings) > 0 {
		fmt.Fprintf(w, "    hallazgos que bloquean: %s\n", strings.Join(e.BlockingFindings, ", "))
	}
	for _, l := range substateLines(c) {
		fmt.Fprintf(w, "  %s\n", l)
	}
}

// substateLines are the orthogonal states, one line each, only when present.
func substateLines(c Card) []string {
	var out []string
	if r := c.Running; r != nil {
		who := r.Role
		if r.Provider != "" {
			who += " (" + r.Provider + ")"
		}
		switch {
		case r.EnvelopeID != "":
			out = append(out, fmt.Sprintf("en curso: sobre %s - %s, paso %d de %d (%s)", r.EnvelopeID, who, r.Step, r.Steps, r.Stage))
		default:
			out = append(out, fmt.Sprintf("en curso: run %s - %s", r.RunID, strings.TrimSpace(who)))
		}
	}
	if i := c.Interrupted; i != nil {
		out = append(out, fmt.Sprintf("interrumpido en el paso %d de %d (%s): sobre %s de %s, sin dueno vivo",
			i.Step, i.Steps, i.Stage, i.EnvelopeID, i.Role))
	}
	if c.Red != nil {
		out = append(out, "rojo: "+c.Red.Reason)
	}
	if n := len(c.Unsynced); n > 0 {
		out = append(out, fmt.Sprintf("sin sincronizar: %d %s sin commitear", n, plural(n, "archivo", "archivos")))
	}
	if l := spendLine(c.Spend); l != "" {
		out = append(out, l)
	}
	return out
}

// spendLine says what the card cost here. What no run reported is said, never
// printed as a zero.
func spendLine(s Spend) string {
	if s.Runs == 0 && s.BudgetUSD == nil {
		return ""
	}
	cost := "sin dato de costo"
	if s.CostUSD != nil {
		cost = usd(*s.CostUSD) + " USD"
	}
	if s.BudgetUSD != nil {
		cost += " de " + usd(*s.BudgetUSD) + " USD"
	}
	parts := []string{cost, plural2(s.Runs, "run", "runs")}
	if s.RunsWithoutCost > 0 && s.CostUSD != nil {
		parts = append(parts, fmt.Sprintf("%d sin dato de costo", s.RunsWithoutCost))
	}
	if tok := s.InputTokens + s.OutputTokens; tok > 0 {
		parts = append(parts, fmt.Sprintf("%d tokens", tok))
	}
	return "gasto: " + strings.Join(parts, " · ") + " (telemetria local)"
}

// lineMax bounds a reason on the board; `hoom item show` and the JSON keep it
// whole.
const lineMax = 160

func recortar(s string) string {
	if r := []rune(s); len(r) > lineMax {
		return string(r[:lineMax-3]) + "..."
	}
	return s
}

func renderWarnings(w io.Writer, warnings []string) {
	for _, m := range warnings {
		fmt.Fprintf(w, "aviso: %s\n", m)
	}
}

func usd(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func plural(n int, uno, varios string) string {
	if n == 1 {
		return uno
	}
	return varios
}

func plural2(n int, uno, varios string) string {
	return fmt.Sprintf("%d %s", n, plural(n, uno, varios))
}
