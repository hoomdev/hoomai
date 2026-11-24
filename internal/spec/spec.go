// Package spec implements deterministic spec quality checks: structural lint
// and spec→test traceability. This is hoomAI's adaptation of SwarmForge's
// Gherkin mutation idea: instead of mutating spec examples and hoping the
// pipeline notices, we bind every acceptance criterion (CA-n) to at least one
// test that references it, and verify that binding with the binary — no LLM,
// no trust, just grep with rules. An untraced criterion means the spec and
// the tests have drifted apart, which is exactly the circularity gap the
// adversarial test-writer exists to close.
package spec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// requiredSections are matched accent- and case-insensitively.
var requiredSections = []string{
	"objetivo",
	"no-goals",
	"contratos",
	"casos limite",
	"criterios de aceptacion",
	"decisiones",
	"riesgos",
}

var caRe = regexp.MustCompile(`CA-(\d+)`)

// verifRe matches the `[verifica: <comando>]` marker: a criterion verified
// by running a command (exit 0) instead of by a test mentioning its token.
// Made for tooling items whose criteria a test cannot assert without
// circularity (a suite cannot test that itself passes). The marker must sit
// on the SAME line as the CA-n token it verifies.
var verifRe = regexp.MustCompile(`\[verifica:\s*([^\]]*)\]`)

// cmdTimeout bounds each [verifica:] command run.
const cmdTimeout = 10 * time.Minute

var accentReplacer = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "ñ", "n", "Ñ", "n",
)

func normalize(s string) string {
	return accentReplacer.Replace(strings.ToLower(s))
}

// Lint validates the spec structure and returns its criterion IDs, the
// commands declared with [verifica: <comando>] per criterion, and any
// issues found. Empty issues = spec passes lint.
func Lint(path string) (ids []string, cmds map[string][]string, issues []string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	ids, cmds, issues = lintDe(raw)
	return ids, cmds, issues, nil
}

// lintDe is Lint of the bytes raw (one read: MedirContenido).
func lintDe(raw []byte) (ids []string, cmds map[string][]string, issues []string) {
	norm := normalize(string(raw))
	for _, sec := range requiredSections {
		if !strings.Contains(norm, sec) {
			issues = append(issues, fmt.Sprintf("falta la seccion %q", sec))
		}
	}
	seen := map[string]int{}
	for _, m := range caRe.FindAllStringSubmatch(string(raw), -1) {
		id := "CA-" + m[1]
		seen[id]++
		if seen[id] == 1 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		issues = append(issues, "no hay criterios de aceptacion identificados como CA-1, CA-2, ... (la trazabilidad spec->test los necesita)")
	}
	sort.Slice(ids, func(i, j int) bool {
		return len(ids[i]) < len(ids[j]) || (len(ids[i]) == len(ids[j]) && ids[i] < ids[j])
	})

	// [verifica: <comando>] binds by line: the marker verifies the CA-n
	// token on its own line; anywhere else it is an orphan (lint issue).
	cmds = map[string][]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		ms := verifRe.FindAllStringSubmatch(line, -1)
		if len(ms) == 0 {
			continue
		}
		ca := caRe.FindStringSubmatch(line)
		if ca == nil {
			issues = append(issues, fmt.Sprintf("marcador [verifica:] sin criterio CA-n en la misma linea: %q", strings.TrimSpace(line)))
			continue
		}
		id := "CA-" + ca[1]
		for _, m := range ms {
			cmd := strings.TrimSpace(m[1])
			if cmd == "" {
				issues = append(issues, fmt.Sprintf("marcador [verifica:] de %s sin comando", id))
				continue
			}
			cmds[id] = append(cmds[id], cmd)
		}
	}
	return ids, cmds, issues
}

var skipDirs = map[string]bool{
	".git": true, ".hoom": true, "vendor": true, "node_modules": true,
	"dist": true, "build": true, "storage": true, ".gradle": true,
	".idea": true, ".vscode": true, "bootstrap": true,
}

// isTestFile decides whether a path looks like a test artifact worth scanning.
func isTestFile(rel string) bool {
	l := strings.ToLower(rel)
	return strings.Contains(l, "test") || strings.Contains(l, "spec")
}

// TraceResult is the outcome of one traceability pass.
type TraceResult struct {
	MissingTests []string // criteria that need a test token and have none
	CmdFailed    []string // formatted [verifica:] command failures
	Scanned      int      // test files scanned
	ByTest       int      // criteria traced via test token
	ByCmd        int      // criteria verified via command (all exit 0)
	// Citas (proporcion-por-riesgo) are the citations of each criterion
	// Trace looked for in the tests (every id without [verifica:]): the
	// number of LINES of the files spec_trace scans (IsTestPath, the same
	// files as Scanned) where its whole token appears. A line counts once
	// per criterion, also when it cites it twice; a line that cites two
	// criteria counts one for each; a token inside a longer one (CA-1 in
	// CA-12) is not a citation. One entry per id looked for, 0 when no
	// test cites it; a criterion with [verifica:] has no entry. nil when
	// Trace looked for none.
	Citas map[string]int
}

// Trace verifies each criterion: IDs with declared [verifica:] commands run
// them (every command must exit 0); the rest must have their exact token
// (e.g. "CA-3") in at least one test file.
func Trace(root string, ids []string, cmds map[string][]string) (TraceResult, error) {
	var res TraceResult
	var needTest []string
	for _, id := range ids {
		if len(cmds[id]) == 0 {
			needTest = append(needTest, id)
		}
	}

	if len(needTest) > 0 {
		// nosemgrep: trailofbits.go.invalid-usage-of-modified-variable.invalid-usage-of-modified-variable -- con error, IndexTokens igual cuenta los archivos que alcanzo a leer
		idx, err := IndexTokens(root)
		if err != nil {
			res.Scanned = idx.Scanned
			return res, err
		}
		res.Scanned = idx.Scanned
		res.MissingTests = idx.Missing(needTest)
		res.ByTest = len(needTest) - len(res.MissingTests)
		res.Citas = make(map[string]int, len(needTest))
		for _, id := range needTest {
			res.Citas[id] = idx.Citas(id)
		}
	}

	for _, id := range ids {
		cs := cmds[id]
		if len(cs) == 0 {
			continue
		}
		ok := true
		for _, c := range cs {
			if msg := runVerifCmd(root, id, c); msg != "" {
				res.CmdFailed = append(res.CmdFailed, msg)
				ok = false
			}
		}
		if ok {
			res.ByCmd++
		}
	}
	return res, nil
}

// runVerifCmd executes one declared verification command; "" means it
// passed, anything else is the formatted failure (criterion, exit code,
// command and output tail — evidence, not narration).
func runVerifCmd(root, id, c string) string {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", c) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- el marcador [verifica:] del spec aprobado por el humano
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		return ""
	}
	reason := err.Error()
	if ee, ok := err.(*exec.ExitError); ok {
		reason = fmt.Sprintf("exit %d", ee.ExitCode())
	}
	if ctx.Err() == context.DeadlineExceeded {
		reason = fmt.Sprintf("timeout tras %s", cmdTimeout)
	}
	msg := fmt.Sprintf("%s (%s): %s", id, reason, c)
	if tail := lastNonEmptyLine(string(out)); tail != "" {
		msg += "\n    | " + tail
	}
	return msg
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

// Gates runs lint + trace for a spec and returns them as synthetic gate
// results to embed in the verdict. Both are required: binding a spec to a
// verify run is opt-in, and once bound, its criteria must be enforced.
//
// proporcion-por-riesgo: Gates is GatesCon with the zero Calibre (a spec
// without card, riesgo normal, no tope and no current approval) without its
// aviso. `hoom verify` measures with GatesCon and approval.CalibreDe.
func Gates(projectDir, specPath string) []verdict.GateResult {
	gates, _ := GatesCon(projectDir, specPath, Calibre{})
	return gates
}

// Tokens is the half of Trace that only READS: which of ids have no test file
// mentioning their exact token, and how many test files were scanned. It
// never runs a verifica command — the board uses it to trace without effects.
func Tokens(root string, ids []string) (missing []string, scanned int, err error) {
	if len(ids) == 0 {
		return nil, 0, nil
	}
	// nosemgrep: trailofbits.go.invalid-usage-of-modified-variable.invalid-usage-of-modified-variable -- con error, IndexTokens igual cuenta los archivos que alcanzo a leer
	idx, err := IndexTokens(root)
	if err != nil {
		return nil, idx.Scanned, err
	}
	return idx.Missing(ids), idx.Scanned, nil
}
