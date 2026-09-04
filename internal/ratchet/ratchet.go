// Package ratchet implements quality that can only go up: a baseline of
// measurable metrics frozen in .hoom/ratchet.json (committed to Git) that
// verify --full compares against. Worse beyond tolerance = red with the
// exact figures; better = the baseline tightens itself and the team
// inherits the new floor. Metrics are DECLARED COMMANDS whose last output
// line is a number — the same agnostic contract as hoom.yaml: the core
// runs and compares; the project knows where the numbers come from.
// Loosening exists but only through an explicit command with a mandatory
// reason: the easy path is the audited path.
package ratchet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	Schema   = "hoom.ratchet/v1"
	FileName = "ratchet.json"

	// cmdTimeout bounds each metric command (mutation runs are slow).
	cmdTimeout = 10 * time.Minute
)

// Metric is one declared measurement and its frozen baseline.
type Metric struct {
	Cmd       string   `json:"cmd"`
	Direction string   `json:"direction"`            // up = more is better | down = less is better
	Tolerance float64  `json:"tolerance,omitempty"`  // absolute noise margin (default 0)
	Value     *float64 `json:"value,omitempty"`      // frozen baseline; nil = not frozen yet
	UpdatedAt string   `json:"updated_at,omitempty"` // last baseline movement
}

// Change is one baseline movement, appended to the file's history.
type Change struct {
	TS     time.Time `json:"ts"`
	Metric string    `json:"metric"`
	From   *float64  `json:"from,omitempty"`
	To     float64   `json:"to"`
	Kind   string    `json:"kind"` // frozen | tightened | loosened
	Reason string    `json:"reason,omitempty"`
}

// File mirrors .hoom/ratchet.json.
type File struct {
	Schema  string             `json:"schema"`
	Metrics map[string]*Metric `json:"metrics"`
	History []Change           `json:"history,omitempty"`
}

func path(root string) string { return filepath.Join(root, ".hoom", FileName) }

// Load reads the baseline. (nil, nil) means no ratchet is declared.
func Load(root string) (*File, error) {
	raw, err := os.ReadFile(path(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s invalido: %w", FileName, err)
	}
	if f.Schema != Schema {
		return nil, fmt.Errorf("%s: schema no soportado %q (esperado %q)", FileName, f.Schema, Schema)
	}
	return &f, nil
}

// Save persists the baseline (pretty JSON: the Git diff IS the audit).
func (f *File) Save(root string) error {
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(root), append(raw, '\n'), 0o644)
}

// Init scaffolds an empty baseline; an existing file is never overwritten.
func Init(root string) (string, error) {
	p := path(root)
	if _, err := os.Stat(p); err == nil {
		return "", fmt.Errorf("%s ya existe; edita el archivo o usa 'hoom ratchet lower' para moverlo", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	f := &File{Schema: Schema, Metrics: map[string]*Metric{}}
	if err := f.Save(root); err != nil {
		return "", err
	}
	return p, nil
}

// Lower moves one baseline toward WORSE, with a mandatory reason. Moving
// toward better is refused: tightening is verify's job, backed by a
// measurement, never by hand.
func Lower(root, name string, to float64, reason string) (*Change, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("aflojar la base exige --reason: nadie baja la vara sin decir por que")
	}
	f, err := Load(root)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("no existe %s (ejecuta 'hoom ratchet init')", FileName)
	}
	m, ok := f.Metrics[name]
	if !ok {
		return nil, fmt.Errorf("la metrica %q no esta declarada en %s", name, FileName)
	}
	if m.Value == nil {
		return nil, fmt.Errorf("la metrica %q no tiene base congelada todavia (corre 'hoom verify --full')", name)
	}
	worse := to < *m.Value // direction up: lower is worse
	if m.Direction == "down" {
		worse = to > *m.Value
	}
	if !worse {
		return nil, fmt.Errorf("%v no afloja la base %v de %q: eso es apretar, y apretar es trabajo de 'hoom verify --full'", to, *m.Value, name)
	}
	ch := Change{TS: time.Now().UTC(), Metric: name, From: m.Value, To: to, Kind: "loosened", Reason: reason}
	v := to
	m.Value = &v
	m.UpdatedAt = ch.TS.Format(time.RFC3339)
	f.History = append(f.History, ch)
	if err := f.Save(root); err != nil {
		return nil, err
	}
	return &ch, nil
}

// Gate measures every declared metric and applies the ratchet rule,
// mutating and persisting the baseline (freeze/tighten). Regression beyond
// tolerance = FAIL; broken command = ERROR fail-closed, baseline untouched.
func Gate(root string, f *File) verdict.GateResult {
	start := time.Now()
	res := verdict.GateResult{Name: "ratchet", Required: true, Scope: "full"}

	names := make([]string, 0, len(f.Metrics))
	for n := range f.Metrics {
		names = append(names, n)
	}
	sort.Strings(names)

	var lines []string
	var failed, broken, tightened, frozen int
	changed := false
	for _, name := range names {
		m := f.Metrics[name]
		if strings.TrimSpace(m.Cmd) == "" || (m.Direction != "up" && m.Direction != "down") {
			broken++
			lines = append(lines, fmt.Sprintf("%s: declaracion invalida (cmd vacio o direction != up|down)", name))
			continue
		}
		cur, err := measure(root, m.Cmd)
		if err != nil {
			broken++
			lines = append(lines, fmt.Sprintf("%s: %v (la base no se toca)", name, err))
			continue
		}
		now := time.Now().UTC()
		switch {
		case m.Value == nil:
			frozen++
			changed = true
			v := cur
			m.Value = &v
			m.UpdatedAt = now.Format(time.RFC3339)
			f.History = append(f.History, Change{TS: now, Metric: name, To: cur, Kind: "frozen",
				Reason: "primer verify --full: linea base congelada"})
			lines = append(lines, fmt.Sprintf("%s: linea base congelada en %s", name, num(cur)))
		case regressed(m, cur):
			failed++
			lines = append(lines, fmt.Sprintf("%s: %s empeora la base %s (delta %s, tolerancia %s) REGRESION",
				name, num(cur), num(*m.Value), num(delta(m, cur)), num(m.Tolerance)))
		case improved(m, cur):
			tightened++
			changed = true
			from := *m.Value
			v := cur
			m.Value = &v
			m.UpdatedAt = now.Format(time.RFC3339)
			f.History = append(f.History, Change{TS: now, Metric: name, From: &from, To: cur, Kind: "tightened",
				Reason: "verify --full: la mejora es el piso nuevo"})
			lines = append(lines, fmt.Sprintf("%s: apretada %s -> %s (el equipo hereda el piso nuevo)", name, num(from), num(cur)))
		default:
			lines = append(lines, fmt.Sprintf("%s: %s (base %s) OK", name, num(cur), num(*m.Value)))
		}
	}

	if changed {
		if err := f.Save(root); err != nil {
			broken++
			lines = append(lines, fmt.Sprintf("no se pudo persistir la base: %v (la base vieja sigue vigente)", err))
		}
	}

	res.DurationMS = time.Since(start).Milliseconds()
	res.OutputTail = strings.Join(lines, "\n")
	res.Notes = fmt.Sprintf("%d metrica(s): %d ok, %d congeladas, %d apretadas, %d regresiones, %d rotas",
		len(names), len(names)-failed-broken-tightened-frozen, frozen, tightened, failed, broken)
	switch {
	case broken > 0:
		res.Status = verdict.StatusError // configurado pero roto: fail-closed
	case failed > 0:
		res.Status = verdict.StatusFail
	default:
		res.Status = verdict.StatusPass
	}
	return res
}

func regressed(m *Metric, cur float64) bool {
	if m.Direction == "down" {
		return cur > *m.Value+m.Tolerance
	}
	return cur < *m.Value-m.Tolerance
}

func improved(m *Metric, cur float64) bool {
	if m.Direction == "down" {
		return cur < *m.Value-m.Tolerance
	}
	return cur > *m.Value+m.Tolerance
}

func delta(m *Metric, cur float64) float64 { return cur - *m.Value }

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// measure runs one metric command; its last non-empty output line must be
// a number.
func measure(root, cmdStr string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return 0, fmt.Errorf("timeout tras %s: %s", cmdTimeout, cmdStr)
	}
	if err != nil {
		reason := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			reason = fmt.Sprintf("exit %d", ee.ExitCode())
		}
		return 0, fmt.Errorf("comando fallo (%s): %s", reason, cmdStr)
	}
	line := lastNonEmptyLine(string(out))
	v, perr := strconv.ParseFloat(strings.TrimSpace(line), 64)
	if perr != nil {
		return 0, fmt.Errorf("la ultima linea de salida no es un numero (%q): %s", line, cmdStr)
	}
	return v, nil
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}
