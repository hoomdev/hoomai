// Package agentcmd implements `hoom agent`: a deterministic ENVELOPE around
// ONE headless CLI session. It resolves the role (contract as system prompt,
// tools, write scope), runs the provider, and closes with evidence in a
// fixed order — scope, verify, check.
//
// The scope gate is the new answer to a question no prompt can answer: hoom
// photographs the tree before and after the run and asks whether the role
// wrote only where it belonged, and left the evidence intact. A contract that
// says "the scout never edits" becomes something the binary can prove.
package agentcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/ratchet"
)

// Violation rules. Tampering is the floor: it cuts the envelope before verify
// because certifying a tree where someone moved the demand itself would be
// certifying the trick. Out of scope is information, not forgery: everything
// still runs and the envelope closes red.
const (
	RuleTampering  = "manipulacion"
	RuleOutOfScope = "fuera-de-scope"
	// RuleIsolation: el arbol ciego dejo de serlo. Corta como la
	// manipulacion, y por la misma razon: certificar un arbol donde el rol
	// pudo mirar lo que no debia seria certificar la trampa.
	RuleIsolation = "aislamiento"
)

// Blind is the evidence of an isolated run: what the blind tree gave back and
// what the role wrote outside its quarantine.
type Blind struct {
	Restored []string // testigos que volvieron al disco del arbol ciego
	Leaked   []string // rutas que cambiaron en el arbol REAL mientras el rol corria confinado
}

const (
	detalleRestaurado = "el rol devolvio al arbol un archivo que el aislamiento habia quitado"
	detalleFuga       = "el rol escribio fuera de la cuarentena: el arbol real cambio durante el run ciego"
)

// Evidence directories the universal rules protect.
var evidenceDirs = []string{"verdicts", "findings", "approvals"}

const ratchetPath = ".hoom/" + ratchet.FileName

// Violation is one path the role should not have written.
type Violation struct {
	Path      string `json:"path"`
	Rule      string `json:"rule"` // manipulacion | fuera-de-scope
	Detail    string `json:"detail"`
	FindingID string `json:"finding_id,omitempty"`
}

// ScopeResult is the verdict of the scope gate.
type ScopeResult struct {
	Touched    []string    `json:"touched"`
	Violations []Violation `json:"violations"`
	Tampering  bool        `json:"tampering"`
	OK         bool        `json:"ok"`
}

// Cuts reports whether the tree does not deserve a verdict at all. Writing
// outside the role's territory is information; moving the demand itself, or
// undoing the blindfold, is not.
func (s ScopeResult) Cuts() bool { return s.Tampering || s.Broken() }

// Broken reports whether the isolation itself failed.
func (s ScopeResult) Broken() bool { return s.has(RuleIsolation) }

func (s ScopeResult) has(rule string) bool {
	for _, v := range s.Violations {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// Snapshot is the photograph of the tree the envelope takes before and after
// the run.
type Snapshot struct {
	Touched  map[string]string // gitx.Touched menos lo que escribe hoom: path -> content hash ("-" = gone)
	Evidence map[string]bool   // paths that EXIST under .hoom/{verdicts,findings,approvals}
	Manifest string            // hash of hoom.yaml ("" = unreadable)
	Ratchet  *ratchet.File     // nil = no baseline declared
}

// Take photographs the tree. It is cheap on purpose: two of these bracket the
// run, and the difference between them is what the role actually did.
func Take(root, base string) Snapshot {
	s := Snapshot{Touched: map[string]string{}, Evidence: map[string]bool{}}
	for p, h := range gitx.Touched(root, base) {
		if !hoomOwn(p) {
			s.Touched[p] = h
		}
	}
	for _, d := range evidenceDirs {
		dir := filepath.Join(root, ".hoom", d)
		filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil //nolint:nilerr // a missing evidence dir is a valid state
			}
			if rel, rerr := filepath.Rel(root, p); rerr == nil {
				s.Evidence[filepath.ToSlash(rel)] = true
			}
			return nil
		})
	}
	if raw, err := os.ReadFile(filepath.Join(root, manifest.FileName)); err == nil {
		sum := sha256.Sum256(raw)
		s.Manifest = hex.EncodeToString(sum[:])
	}
	rf, err := ratchet.Load(root)
	if err != nil {
		// A baseline that no longer parses has lost every demand it held.
		rf = &ratchet.File{}
	}
	s.Ratchet = rf
	return s
}

// hoomOwn marks the paths HOOM itself writes while the run happens: the
// narration of the run, the live cache, the worktrees of other tasks. They
// are local and outside Git by design, and charging them to the role would be
// blaming the referee for the game.
func hoomOwn(p string) bool {
	return strings.HasPrefix(p, ".hoom/runs/") ||
		strings.HasPrefix(p, ".hoom/cache/") ||
		strings.HasPrefix(p, ".hoom/worktrees/") ||
		strings.HasPrefix(p, ".hoom/isolated/")
}

// Policy is where a role may write, already resolved: the shape's defaults
// re-aimed by the project's manifest.
type Policy struct{ Allow, Deny []string }

// defaultAllow / defaultDeny per scope shape. The test globs cover the usual
// layouts of the four profiles; a project with another one declares its own
// in hoom.yaml instead of living with noise.
func defaultAllow(scope string) []string {
	switch scope {
	case agents.ScopeTests:
		return []string{".hoom/**", "tests/**", "test/**", "spec/**", "src/test/**",
			"**/*_test.go", "**/*Test.php", "**/*Test.kt", "**/*Test.java",
			"**/*Spec.kt", "**/*.test.ts", "**/*.test.js"}
	case agents.ScopeCodigo:
		return []string{"**"}
	default: // evidencia
		return []string{".hoom/**"}
	}
}

func defaultDeny(scope string) []string {
	if scope == agents.ScopeCodigo {
		// El spec es del arquitecto y lo aprueba el humano: reescribirlo
		// desde el writer invalidaria la aprobacion que lo autorizo.
		return []string{".hoom/specs/**"}
	}
	return nil
}

// PolicyFor resolves the role's write policy for this project. A declared
// allow REPLACES the defaults; deny is added to them.
func PolicyFor(m *manifest.Manifest, r agents.Role) Policy {
	p := Policy{Allow: defaultAllow(r.Scope), Deny: defaultDeny(r.Scope)}
	if m == nil {
		return p
	}
	ap, ok := m.Agents[r.Slug]
	if !ok {
		return p
	}
	if allow := clean(ap.Write.Allow); len(allow) > 0 {
		p.Allow = allow
	}
	p.Deny = append(p.Deny, clean(ap.Write.Deny)...)
	return p
}

func clean(list []string) []string {
	var out []string
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// matchGlob reports whether path matches pattern. It supports ** (zero or
// more path segments), * and ? (inside one segment). Own matcher on purpose:
// the project carries no dependency beyond yaml.
func matchGlob(pattern, p string) bool {
	// Colapsar ** consecutivos: "**/**/**" significa lo mismo que "**" y, sin
	// colapsar, cada uno multiplica el backtracking del siguiente.
	var pat []string
	for _, seg := range strings.Split(pattern, "/") {
		if seg == "**" && len(pat) > 0 && pat[len(pat)-1] == "**" {
			continue
		}
		pat = append(pat, seg)
	}
	return matchSegments(pat, strings.Split(p, "/"))
}

func matchSegments(pat, seg []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(seg); i++ {
				if matchSegments(pat[1:], seg[i:]) {
					return true
				}
			}
			return false
		}
		if len(seg) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], seg[0]); err != nil || !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}

// allowedBy evaluates the role policy for one path: deny wins over allow.
func allowedBy(p string, pol Policy) (bool, string) {
	for _, g := range pol.Deny {
		if matchGlob(g, p) {
			return false, g
		}
	}
	for _, g := range pol.Allow {
		if matchGlob(g, p) {
			return true, ""
		}
	}
	return false, ""
}

// Gate is the envelope's third step, and it belongs to whoever runs a role:
// `hoom agent` and `hoom review` judge with ONE implementation of "the role
// wrote where it belonged". Every violation becomes an append-only artifact —
// a terminal message is lost, a finding demands a resolution with evidence —
// and a finding that cannot be written never hides the violation.
func Gate(dir, base string, role agents.Role, before, after Snapshot, pol Policy, blind *Blind) ScopeResult {
	sc := CheckScope(before, after, pol)
	if blind != nil {
		sc = withIsolation(sc, *blind)
	}
	for i, v := range sc.Violations {
		desc := fmt.Sprintf("%s: el rol %s escribio %s - %s", v.Rule, role.Slug, v.Path, v.Detail)
		if f, err := finding.Add(dir, base, "high", "risk", v.Path, desc, "hoom gate de scope"); err == nil {
			sc.Violations[i].FindingID = f.ID
		}
	}
	return sc
}

// withIsolation folds the isolation rules into a scope result. They win over
// whatever the scope said about the same path: a witness back on disk is not
// "the role wrote outside its territory", it is "the role took off the
// blindfold", and only the second one cuts.
func withIsolation(sc ScopeResult, b Blind) ScopeResult {
	broken := map[string]string{}
	for _, p := range b.Restored {
		broken[p] = detalleRestaurado
	}
	for _, p := range b.Leaked {
		if _, ok := broken[p]; !ok {
			broken[p] = detalleFuga
		}
	}
	if len(broken) == 0 {
		return sc
	}
	kept := sc.Violations[:0]
	for _, v := range sc.Violations {
		if _, ok := broken[v.Path]; !ok {
			kept = append(kept, v)
		}
	}
	sc.Violations = kept
	paths := make([]string, 0, len(broken))
	for p := range broken {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		sc.Violations = append(sc.Violations, Violation{Path: p, Rule: RuleIsolation, Detail: broken[p]})
	}
	sortViolations(sc.Violations)
	sc.OK = false
	return sc
}

// ruleRank orders the violations by how badly they end the run: the
// blindfold first, then the demand, then the territory.
func ruleRank(rule string) int {
	switch rule {
	case RuleIsolation:
		return 0
	case RuleTampering:
		return 1
	default:
		return 2
	}
}

func sortViolations(vs []Violation) {
	sort.SliceStable(vs, func(i, j int) bool {
		if ri, rj := ruleRank(vs[i].Rule), ruleRank(vs[j].Rule); ri != rj {
			return ri < rj
		}
		return vs[i].Path < vs[j].Path
	})
}

// CheckScope compares the two photographs and judges every path the run
// changed. Universal rules run first: they are the floor no manifest can
// lower.
func CheckScope(before, after Snapshot, pol Policy) ScopeResult {
	res := ScopeResult{Touched: delta(before.Touched, after.Touched)}
	// hoom.yaml may be untracked or ignored; its content settles the question.
	if before.Manifest != after.Manifest && !contains(res.Touched, manifest.FileName) {
		res.Touched = append(res.Touched, manifest.FileName)
		sort.Strings(res.Touched)
	}
	loosened := ratchet.Loosened(before.Ratchet, after.Ratchet)
	seenRatchet := false
	for _, p := range res.Touched {
		if p == ratchetPath {
			seenRatchet = true
		}
		if v, ok := universal(p, before, loosened); ok {
			res.Violations = append(res.Violations, v)
			continue
		}
		if ok, deny := allowedBy(p, pol); !ok {
			res.Violations = append(res.Violations, Violation{
				Path: p, Rule: RuleOutOfScope, Detail: outOfScopeDetail(deny, pol),
			})
		}
	}
	// A loosened baseline is a violation even if the file itself never showed
	// up in the delta (ignored, moved, rewritten in place).
	if len(loosened) > 0 && !seenRatchet {
		res.Violations = append(res.Violations, ratchetViolation(loosened))
		res.Touched = append(res.Touched, ratchetPath)
		sort.Strings(res.Touched)
	}
	sortViolations(res.Violations)
	for _, v := range res.Violations {
		if v.Rule == RuleTampering {
			res.Tampering = true
		}
	}
	res.OK = len(res.Violations) == 0
	return res
}

// universal applies the append-only floor to one path.
func universal(p string, before Snapshot, loosened []string) (Violation, bool) {
	switch {
	case p == manifest.FileName:
		return Violation{Path: p, Rule: RuleTampering,
			Detail: "hoom.yaml define la exigencia: un rol no la cambia"}, true
	case strings.HasPrefix(p, ".hoom/approvals/"):
		return Violation{Path: p, Rule: RuleTampering,
			Detail: "la aprobacion humana no la escribe un agente (usa 'hoom spec approve')"}, true
	case strings.HasPrefix(p, ".hoom/verdicts/"), strings.HasPrefix(p, ".hoom/findings/"):
		if before.Evidence[p] {
			return Violation{Path: p, Rule: RuleTampering,
				Detail: "la evidencia es append-only: este artefacto ya existia antes del run"}, true
		}
		return Violation{}, false // creado: trabajo legitimo (hoom verify / hoom finding add)
	case p == ratchetPath && len(loosened) > 0:
		return ratchetViolation(loosened), true
	}
	return Violation{}, false
}

func ratchetViolation(loosened []string) Violation {
	return Violation{Path: ratchetPath, Rule: RuleTampering,
		Detail: fmt.Sprintf("el trinquete solo puede subir; aflojo %s (aflojar se hace con 'hoom ratchet lower --reason')",
			strings.Join(loosened, ", "))}
}

// detalleFueraDelAllow prefixes the violation whose cause is a territory the
// role simply does not own — the only kind a wider allow would fix.
const detalleFueraDelAllow = "fuera del scope de escritura del rol"

func outOfScopeDetail(deny string, pol Policy) string {
	if deny != "" {
		return fmt.Sprintf("ruta prohibida para el rol (deny %s)", deny)
	}
	return fmt.Sprintf("%s (permitido: %s)", detalleFueraDelAllow, strings.Join(pol.Allow, ", "))
}

// delta is the symmetric difference by CONTENT: a file edited and returned to
// its original bytes never appears, and a file already dirty before the run
// does appear when the run changed it again. The envelope measures the tree,
// not the history of edits.
func delta(before, after map[string]string) []string {
	var out []string
	for p, h := range after {
		if before[p] != h {
			out = append(out, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
