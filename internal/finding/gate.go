package finding

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// GateName is the synthetic gate that lets open findings block verify.
const GateName = "findings_open"

// severityRank orders the three severities; a threshold blocks its own rank
// and every rank above it.
var severityRank = map[string]int{"low": 0, "medium": 1, "high": 2}

// Blocks reports whether an open finding of this severity blocks under the
// blockOn threshold (low | medium | high: that severity or worse). An unknown
// severity or an empty threshold blocks nothing: the gate itself fails closed
// on unknown severities before it ever asks.
func Blocks(severity, blockOn string) bool {
	sev, okS := severityRank[strings.ToLower(strings.TrimSpace(severity))]
	min, okB := severityRank[strings.ToLower(strings.TrimSpace(blockOn))]
	return okS && okB && sev >= min
}

// TaskOfSpec is the task a spec binds a verify to: its file name without .md
// (.hoom/specs/<slug>.md -> <slug>), the same slug `hoom task start` takes.
func TaskOfSpec(specPath string) string {
	return strings.TrimSuffix(filepath.Base(filepath.ToSlash(specPath)), ".md")
}

// descMax bounds each finding's description in the gate's output tail.
const descMax = 160

// Gate evaluates the open findings of the tree at root against blockOn.
// specPath "" counts every finding (scope full); otherwise it counts the
// findings of TaskOfSpec(specPath) plus the ones without a task (scope spec):
// a finding without a task may be anyone's, so only one that provably
// belongs to ANOTHER task stays out.
//
// It fails closed: a finding file it cannot read, or an in-scope open finding
// with a severity hoom does not know, is an ERROR — the gate cannot tell
// whether it blocks, so it does not assume it does not. base is taken for
// parity with List; counting states needs no tree fingerprint.
func Gate(root, base, blockOn, specPath string) (res verdict.GateResult) {
	start := time.Now()
	res = verdict.GateResult{Name: GateName, Required: true, Scope: "full"}
	task := ""
	if specPath != "" {
		res.Scope = "spec"
		task = TaskOfSpec(specPath)
	}
	defer func() { res.DurationMS = time.Since(start).Milliseconds() }()

	sc, err := scan(root)
	if err != nil {
		res.Status = verdict.StatusError
		res.Notes = err.Error()
		return res
	}

	var blocking []Item
	quiet := map[string]int{} // abiertos que no bloquean, por severidad
	others := 0
	broken := append([]string{}, sc.broken...)
	for _, it := range sc.items {
		if it.Status != StatusOpen {
			continue
		}
		if task != "" && it.Task != "" && it.Task != task {
			others++
			continue
		}
		sev := strings.ToLower(strings.TrimSpace(it.Severity))
		if _, ok := severityRank[sev]; !ok {
			broken = append(broken, fmt.Sprintf("%s.json: severidad desconocida %q (low|medium|high)", it.ID, it.Severity))
			continue
		}
		if Blocks(sev, blockOn) {
			blocking = append(blocking, it)
		} else {
			quiet[sev]++
		}
	}

	if len(broken) > 0 {
		res.Status = verdict.StatusError
		res.Notes = fmt.Sprintf("hallazgos que no se pueden evaluar (no se sabe si bloquean): %s. Accion: reparalos a mano; son evidencia versionada y el cambio queda en el diff de Git",
			strings.Join(broken, "; "))
		res.OutputTail = strings.Join(sc.warnings, "\n")
		return res
	}

	alcance := "todos los hallazgos"
	if task != "" {
		alcance = "tarea " + task + " + sin tarea"
		if others > 0 {
			alcance += fmt.Sprintf(" (%d de otras tareas no cuentan)", others)
		}
	}
	var notes string
	if len(blocking) == 0 {
		res.Status = verdict.StatusPass
		notes = fmt.Sprintf("0 abiertos que bloquean (block_on: %s)", blockOn)
	} else {
		res.Status = verdict.StatusFail
		ids := make([]string, 0, len(blocking))
		for _, it := range blocking {
			ids = append(ids, it.ID+" ["+lensOf(it)+"]")
		}
		notes = fmt.Sprintf("%d abiertos que bloquean (block_on: %s): %s", len(blocking), blockOn, strings.Join(ids, ", "))
	}
	notes += " · alcance: " + alcance
	var silent []string
	for _, sev := range []string{"high", "medium", "low"} {
		if quiet[sev] > 0 {
			silent = append(silent, fmt.Sprintf("%d %s", quiet[sev], sev))
		}
	}
	if len(silent) > 0 {
		notes += " · no bloquean: " + strings.Join(silent, ", ")
	}
	res.Notes = notes

	var tail []string
	if len(blocking) > 0 {
		for _, it := range blocking {
			where := ""
			if it.File != "" {
				where = " " + it.File
			}
			tail = append(tail, fmt.Sprintf("%s %s [%s]%s: %s", it.ID, it.Severity, lensOf(it), where, recortar(it.Description)))
		}
		tail = append(tail, "Accion: corrige y cierra cada uno con evidencia, o refutalo con la evidencia que lo tumba:")
		for _, it := range blocking {
			tail = append(tail, fmt.Sprintf("  hoom finding resolve %s --as corregido|refutado --evidence \"...\"", it.ID))
		}
		tail = append(tail, "despues vuelve a correr 'hoom verify'")
	}
	tail = append(tail, sc.warnings...)
	res.OutputTail = strings.Join(tail, "\n")
	return res
}

func lensOf(it Item) string {
	if strings.TrimSpace(it.Lens) == "" {
		return "sin lente"
	}
	return it.Lens
}

func recortar(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > descMax {
		return string(r[:descMax]) + "..."
	}
	return s
}
