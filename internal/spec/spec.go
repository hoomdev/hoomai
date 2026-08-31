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
	"path/filepath"
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
	return ids, cmds, issues, nil
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

	found := map[string]bool{}
	if len(needTest) > 0 {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
			if werr != nil {
				return nil
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if !isTestFile(rel) {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil || info.Size() > 2<<20 {
				return nil
			}
			raw, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			res.Scanned++
			s := string(raw)
			for _, id := range needTest {
				if !found[id] && containsToken(s, id) {
					found[id] = true
				}
			}
			return nil
		})
		if err != nil {
			return res, err
		}
	}
	for _, id := range needTest {
		if found[id] {
			res.ByTest++
		} else {
			res.MissingTests = append(res.MissingTests, id)
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
	cmd := exec.CommandContext(ctx, "sh", "-c", c)
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

// containsToken matches the ID as a whole token so CA-1 does not match CA-12.
func containsToken(s, id string) bool {
	idx := 0
	for {
		i := strings.Index(s[idx:], id)
		if i < 0 {
			return false
		}
		pos := idx + i
		end := pos + len(id)
		if end >= len(s) || !isDigit(s[end]) {
			return true
		}
		idx = end
	}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// Gates runs lint + trace for a spec and returns them as synthetic gate
// results to embed in the verdict. Both are required: binding a spec to a
// verify run is opt-in, and once bound, its criteria must be enforced.
func Gates(projectDir, specPath string) []verdict.GateResult {
	start := time.Now()
	lintRes := verdict.GateResult{Name: "spec_lint", Required: true, Scope: "spec"}
	traceRes := verdict.GateResult{Name: "spec_trace", Required: true, Scope: "spec"}

	ids, cmds, issues, err := Lint(specPath)
	lintRes.DurationMS = time.Since(start).Milliseconds()
	switch {
	case err != nil:
		lintRes.Status = verdict.StatusError
		lintRes.Notes = err.Error()
		traceRes.Status = verdict.StatusSkipped
		return []verdict.GateResult{lintRes, traceRes}
	case len(issues) > 0:
		lintRes.Status = verdict.StatusFail
		lintRes.OutputTail = strings.Join(issues, "\n")
	default:
		lintRes.Status = verdict.StatusPass
		lintRes.Notes = fmt.Sprintf("%d criterios (CA-*)", len(ids))
		if len(cmds) > 0 {
			lintRes.Notes += fmt.Sprintf(", %d con [verifica:]", len(cmds))
		}
	}

	t := time.Now()
	tr, terr := Trace(projectDir, ids, cmds)
	traceRes.DurationMS = time.Since(t).Milliseconds()
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
	}
	return []verdict.GateResult{lintRes, traceRes}
}
