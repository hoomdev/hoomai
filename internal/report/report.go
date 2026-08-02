// Package report renders verdicts for the human who has no time to read
// diffs: one screen, evidence per gate, degradation loud, trend over time.
package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	cReset  = "\033[0m"
	cGreen  = "\033[32m"
	cRed    = "\033[31m"
	cYellow = "\033[33m"
	cGray   = "\033[90m"
	cBold   = "\033[1m"
)

func statusBadge(s string) string {
	switch s {
	case verdict.StatusPass:
		return cGreen + "PASS " + cReset
	case verdict.StatusFail:
		return cRed + "FAIL " + cReset
	case verdict.StatusError:
		return cRed + "ERROR" + cReset
	case verdict.StatusAbsent:
		return cYellow + "AUSEN" + cReset
	case verdict.StatusSkipped:
		return cGray + "SKIP " + cReset
	}
	return s
}

// Render prints one verdict to w.
func Render(w io.Writer, v *verdict.Verdict, path string) {
	fmt.Fprintf(w, "\n%shoomAI - veredicto de verificacion%s\n", cBold, cReset)
	fmt.Fprintf(w, "proyecto: %s  perfil: %s  politica: %s\n", v.Project, v.Profile, v.Policy)
	if v.Git.IsRepo {
		dirty := ""
		if v.Git.Dirty {
			dirty = " (working tree con cambios)"
		}
		fmt.Fprintf(w, "git: %s @ %s vs %s%s - %d archivo(s) en el cambio\n",
			v.Git.Branch, v.Git.Commit, v.Git.Base, dirty, len(v.Git.ChangedFiles))
	} else {
		fmt.Fprintf(w, "git: no es un repositorio (sin scoping por diff)\n")
	}
	if v.Git.IsRepo && (v.Git.Insertions+v.Git.Deletions) > 0 {
		fmt.Fprintf(w, "cambio: +%d -%d lineas - huella %s\n", v.Git.Insertions, v.Git.Deletions, v.Git.ChangeFingerprint)
		if v.Git.Insertions+v.Git.Deletions > 400 {
			fmt.Fprintf(w, "%s! cambio grande (>400 lineas): la review exige las 4 lentes (readability/reliability/resilience/risk)%s\n", cYellow, cReset)
		}
	}
	fmt.Fprintln(w, strings.Repeat("-", 72))
	for _, g := range v.Gates {
		scope := ""
		if g.Scope == "diff" {
			scope = cGray + " [diff]" + cReset
		}
		req := " "
		if g.Required {
			req = "*"
		}
		fmt.Fprintf(w, " %s %s %-14s %7dms%s\n", statusBadge(g.Status), req, g.Name, g.DurationMS, scope)
		if g.Partial != "" {
			fmt.Fprintf(w, "         %s! cobertura parcial: %s%s\n", cYellow, g.Partial, cReset)
		}
		if g.Status == verdict.StatusAbsent {
			fmt.Fprintf(w, "         %s! gate declarado ausente - sin comando configurado%s\n", cYellow, cReset)
		}
		if g.Status == verdict.StatusFail || g.Status == verdict.StatusError {
			fmt.Fprintf(w, "         %scmd: %s%s\n", cGray, g.Command, cReset)
			if g.Notes != "" {
				fmt.Fprintf(w, "         %s%s%s\n", cGray, g.Notes, cReset)
			}
			for _, line := range lastLines(g.OutputTail, 6) {
				fmt.Fprintf(w, "         %s| %s%s\n", cGray, line, cReset)
			}
		}
	}
	fmt.Fprintln(w, strings.Repeat("-", 72))
	color, label := cGreen, "VERDE"
	if v.Verdict == "red" {
		color, label = cRed, "ROJO"
	}
	fmt.Fprintf(w, " veredicto: %s%s%s%s  (pass:%d fail:%d error:%d ausentes:%d skip:%d)\n",
		cBold, color, label, cReset,
		v.Summary.Pass, v.Summary.Fail, v.Summary.Error, v.Summary.Absent, v.Summary.Skipped)
	if v.Summary.Absent > 0 {
		fmt.Fprintf(w, " %s! %d capacidad(es) sin gate: la red de seguridad esta incompleta y lo sabes.%s\n",
			cYellow, v.Summary.Absent, cReset)
	}
	if path != "" {
		fmt.Fprintf(w, " artefacto: %s\n", path)
	}
	fmt.Fprintln(w)
}

// History prints the last n verdicts and a per-gate trend across all runs.
func History(w io.Writer, all []*verdict.Verdict, n int) {
	if len(all) == 0 {
		fmt.Fprintln(w, "hoom: sin veredictos todavia (ejecuta 'hoom verify')")
		return
	}
	if n > len(all) {
		n = len(all)
	}
	recent := all[len(all)-n:]
	fmt.Fprintf(w, "\n%shoomAI - historial (%d de %d veredictos)%s\n", cBold, n, len(all), cReset)
	fmt.Fprintln(w, strings.Repeat("-", 72))
	for _, v := range recent {
		color, mark := cGreen, "OK "
		if v.Verdict == "red" {
			color, mark = cRed, "RED"
		}
		fmt.Fprintf(w, " %s%s%s %s %s@%s  pass:%d fail:%d err:%d aus:%d\n",
			color, mark, cReset,
			v.CreatedAt.Format("2006-01-02 15:04"),
			v.Git.Branch, v.Git.Commit,
			v.Summary.Pass, v.Summary.Fail, v.Summary.Error, v.Summary.Absent)
	}
	fmt.Fprintln(w, strings.Repeat("-", 72))

	type stat struct{ pass, total int }
	stats := map[string]*stat{}
	var order []string
	for _, v := range all {
		for _, g := range v.Gates {
			if g.Status == verdict.StatusSkipped || g.Status == verdict.StatusAbsent {
				continue
			}
			s, ok := stats[g.Name]
			if !ok {
				s = &stat{}
				stats[g.Name] = s
				order = append(order, g.Name)
			}
			s.total++
			if g.Status == verdict.StatusPass {
				s.pass++
			}
		}
	}
	fmt.Fprintf(w, " tendencia por gate (historial completo):\n")
	for _, name := range order {
		s := stats[name]
		rate := 0
		if s.total > 0 {
			rate = 100 * s.pass / s.total
		}
		color := cGreen
		if rate < 80 {
			color = cYellow
		}
		if rate < 50 {
			color = cRed
		}
		fmt.Fprintf(w, "   %-14s %s%3d%%%s (%d/%d)\n", name, color, rate, cReset, s.pass, s.total)
	}
	fmt.Fprintln(w)
}

func lastLines(s string, n int) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
