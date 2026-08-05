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
	"fmt"
	"os"
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

var accentReplacer = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "ñ", "n", "Ñ", "n",
)

func normalize(s string) string {
	return accentReplacer.Replace(strings.ToLower(s))
}

// Lint validates the spec structure and returns its criterion IDs plus any
// issues found. Empty issues = spec passes lint.
func Lint(path string) (ids []string, issues []string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
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
	return ids, issues, nil
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

// Trace scans the project's test files for references to each criterion ID.
// A criterion is traced when its exact token (e.g. "CA-3") appears in at
// least one test file. Returns the untraced IDs and how many files were
// scanned.
func Trace(root string, ids []string) (missing []string, scanned int, err error) {
	found := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
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
		scanned++
		s := string(raw)
		for _, id := range ids {
			if !found[id] && containsToken(s, id) {
				found[id] = true
			}
		}
		return nil
	})
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	return missing, scanned, err
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

	ids, issues, err := Lint(specPath)
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
	}

	t := time.Now()
	missing, scanned, terr := Trace(projectDir, ids)
	traceRes.DurationMS = time.Since(t).Milliseconds()
	switch {
	case terr != nil:
		traceRes.Status = verdict.StatusError
		traceRes.Notes = terr.Error()
	case len(ids) == 0:
		traceRes.Status = verdict.StatusSkipped
		traceRes.Notes = "sin criterios CA-* que trazar"
	case len(missing) > 0:
		traceRes.Status = verdict.StatusFail
		traceRes.OutputTail = fmt.Sprintf("criterios SIN test que los referencie: %s\nAccion: el test-writer debe crear tests que mencionen cada CA-n (en el nombre o un comentario).", strings.Join(missing, ", "))
		traceRes.Notes = fmt.Sprintf("%d archivos de test escaneados", scanned)
	default:
		traceRes.Status = verdict.StatusPass
		traceRes.Notes = fmt.Sprintf("%d/%d criterios trazados en %d archivos de test", len(ids), len(ids), scanned)
	}
	return []verdict.GateResult{lintRes, traceRes}
}
