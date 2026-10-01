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
	"errors"
	"fmt"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"io"
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
	"github.com/hoomdev/hoomai/internal/verdict"
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
	// Real photographs the REAL tree around the blind run (nil = not taken):
	// its evidence is compared on disk, so a resolution written out of the
	// quarantine is a closing even when git does not list it.
	Real *Fotos
}

// Fotos are the two photographs of one tree around a run.
type Fotos struct{ Antes, Despues Snapshot }

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

// Delivered is what the run changed that counts as the role's work: Touched
// minus what hoom's own commands generate (a verdict from `hoom verify`, a
// finding from `hoom finding add`, a ratchet tightened by `verify --full`).
// Running hoom is not delivering; it is exactly how a green verdict of
// nothing used to be made. A method, not a field: the JSON of ScopeResult
// that Spec B fixed does not change.
func (s ScopeResult) Delivered() []string {
	var out []string
	for _, p := range s.Touched {
		if !hoomGenerated(p) {
			out = append(out, p)
		}
	}
	return out
}

func hoomGenerated(p string) bool {
	return strings.HasPrefix(p, ".hoom/verdicts/") || strings.HasPrefix(p, ".hoom/findings/") || p == ratchetPath
}

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
	// Huellas: what the disk floor compares for every entry under
	// .hoom/{verdicts,findings,approvals} on disk that is not a directory,
	// git-ignored ones included: the hex sha256 of a regular file's whole
	// content; HuellaNoRegular plus its type for anything else (a symlink, a
	// FIFO: never followed or read). And HuellaIlegible for an entry hoom
	// could not read or that changed while it was being opened — a directory
	// too, the one exception to "not a directory". nil in a hand-built
	// photograph (the disk floor then does not apply).
	Huellas map[string]string
	// Directorios: the directories under those three on disk (not the three
	// themselves), readable or not. hoom never creates one there, so the disk
	// floor marks the ones a run created; nil in a hand-built photograph.
	Directorios map[string]bool
	Manifest    string        // hash of hoom.yaml ("" = unreadable)
	Ratchet     *ratchet.File // nil = no baseline declared
}

// Take photographs the tree: two of these bracket the run, and the
// difference between them is what the role actually did. Its cost is linear
// in the evidence, read whole to hash it.
func Take(root, base string) Snapshot {
	s := Snapshot{Touched: map[string]string{}, Evidence: map[string]bool{}, Huellas: map[string]string{}, Directorios: map[string]bool{}}
	for p, h := range gitx.Touched(root, base) {
		if !hoomOwn(p) {
			s.Touched[p] = h
		}
	}
	switch r, err := os.OpenRoot(root); {
	case err == nil:
		s.fotoEvidencia(r)
		r.Close()
	case !errors.Is(err, fs.ErrNotExist):
		s.sinEvidenciaLegible()
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

// fotoEvidencia photographs .hoom/{verdicts,findings,approvals} under the
// project root r without ever following a symlink. Every component, .hoom
// included, is looked at (Lstat) and then opened relative to its parent's
// descriptor as that same object (hoomfs.AbrirDirEn, AbrirRegularEn): a
// process still alive during the photograph can swap a directory for a
// symlink between a look and an open, and a walk by name would follow it —
// outside the repo, or to a clean copy of the evidence. hoom never makes
// .hoom anything but a directory.
func (s *Snapshot) fotoEvidencia(r *os.Root) {
	fi, err := r.Lstat(".hoom")
	if errors.Is(err, fs.ErrNotExist) {
		return // no evidence yet is a valid state
	}
	if err == nil && fi.IsDir() {
		if hoom, err := hoomfs.AbrirDirEn(r, ".hoom", fi); err == nil {
			defer hoom.Close()
			for _, d := range evidenceDirs {
				s.fotoEntrada(hoom, d, ".hoom/"+d, true)
			}
			return
		}
	}
	s.sinEvidenciaLegible()
}

// sinEvidenciaLegible is the photograph of evidence hoom cannot reach: what
// it cannot read it cannot vouch for, so the floor fails closed on it.
func (s *Snapshot) sinEvidenciaLegible() {
	for _, d := range evidenceDirs {
		s.Huellas[".hoom/"+d] = HuellaIlegible
	}
}

// fotoEntrada photographs the entry name of the directory r as rel, and
// everything under it if it is a directory. raiz marks one of the three
// evidence directories: missing is a valid state, and it is not one of the
// Directorios.
func (s *Snapshot) fotoEntrada(r *os.Root, name, rel string, raiz bool) {
	fi, err := r.Lstat(name)
	switch {
	case raiz && errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		s.Huellas[rel] = HuellaIlegible
	case fi.IsDir():
		if !raiz {
			s.Directorios[rel] = true
		}
		s.fotoDirectorio(r, name, rel, fi)
	default:
		s.Evidence[rel] = true
		s.Huellas[rel] = huella(r, name, fi)
	}
}

// fotoDirectorio photographs what the directory name of r (the one fi
// describes) holds. What it could list before an error is still
// photographed; the directory itself is then HuellaIlegible.
func (s *Snapshot) fotoDirectorio(r *os.Root, name, rel string, fi fs.FileInfo) {
	sub, err := hoomfs.AbrirDirEn(r, name, fi)
	if err != nil {
		s.Huellas[rel] = HuellaIlegible
		return
	}
	defer sub.Close()
	d, err := sub.Open(".")
	if err != nil {
		s.Huellas[rel] = HuellaIlegible
		return
	}
	entradas, err := d.ReadDir(-1)
	d.Close()
	if err != nil {
		s.Huellas[rel] = HuellaIlegible
	}
	for _, e := range entradas {
		s.fotoEntrada(sub, e.Name(), rel+"/"+e.Name(), false)
	}
}

// hoomOwn marks the paths HOOM itself writes while the run happens: the
// narration of the run, the live cache, the worktrees of other tasks. They
// are local and outside Git by design, and charging them to the role would be
// blaming the referee for the game.
func hoomOwn(p string) bool { return hoomfs.IsLocal(p) }

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
	case agents.ScopeSpecs:
		// the spec authors' territory is the spec, not the rest of .hoom/:
		// role contracts, intake documents and findings stay out
		return []string{".hoom/specs/**"}
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
// and a finding that cannot be written never hides the violation. task ties
// those findings to the task the run belongs to ("" = none).
func Gate(dir, base, task string, role agents.Role, before, after Snapshot, pol Policy, blind *Blind) ScopeResult {
	sc := CheckScope(before, after, pol)
	if blind != nil {
		sc = withIsolation(sc, *blind)
	}
	// The disk floor and the closings go last, over every tree the run could
	// write to (the one it ran in and, blind, the real one): withIsolation
	// replaces the earlier violations of a leaked path, and these must
	// survive it.
	arboles := []Fotos{{Antes: before, Despues: after}}
	if blind != nil && blind.Real != nil {
		arboles = append(arboles, *blind.Real)
	}
	// closings first: a resolution a role created keeps the one violation
	// that says what it did, and the disk floor does not add a second
	if role.Slug != finding.RolQueRefuta {
		sc = sinCierres(sc, role, arboles...)
	}
	sc = pisoEnDisco(sc, arboles...)
	for i, v := range sc.Violations {
		desc := fmt.Sprintf("%s: el rol %s escribio %s - %s", v.Rule, role.Slug, v.Path, v.Detail)
		if f, err := finding.Register(dir, base, finding.Draft{Severity: "high", Lens: "risk", File: v.Path,
			Description: desc, Author: "hoom gate de scope", Task: task}); err == nil {
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

// The two Huellas that are not a content hash.
const (
	// HuellaNoRegular prefixes the type of an entry that is not a regular
	// file: it is never followed nor read (a FIFO would hang the photograph,
	// a symlink would point it elsewhere), and no evidence hoom writes is one.
	// A symlink's adds ":" and the sha256 of its target text (readlink does
	// not follow it), so pointing it elsewhere changes it.
	HuellaNoRegular = "no-regular:"
	// HuellaIlegible marks an entry hoom could not open or read.
	HuellaIlegible = "ilegible"
)

// huella is what the disk floor compares for the entry name of r, which fi
// (its Lstat) describes: the sha256 of a regular file's whole content (the
// cost is linear in the evidence, and a hash of less would leave bytes
// unwatched), read only if the descriptor shows that same regular file —
// the entry can change between the Lstat and the open.
func huella(r *os.Root, name string, fi fs.FileInfo) string {
	switch t := fi.Mode().Type(); {
	case t&fs.ModeSymlink != 0:
		destino, err := r.Readlink(name)
		if err != nil {
			return HuellaIlegible
		}
		sum := sha256.Sum256([]byte(destino))
		return HuellaNoRegular + t.String() + ":" + hex.EncodeToString(sum[:])
	case !t.IsRegular():
		return HuellaNoRegular + t.String()
	}
	f, err := hoomfs.AbrirRegularEn(r, name, fi)
	if err != nil {
		return HuellaIlegible
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return HuellaIlegible
	}
	return hex.EncodeToString(h.Sum(nil))
}

// conForma says whether a file a run created under the evidence has the
// shape hoom writes: a finding or a resolution under .hoom/findings/, a
// verdict under .hoom/verdicts/ (finding.EsNombre and verdict.EsNombre are
// the one definition of each). Nothing created under .hoom/approvals/ has
// it: the human approval is never a role's.
func conForma(p string) bool {
	switch dir, name := path.Split(p); dir {
	case ".hoom/findings/":
		return finding.EsNombre(name)
	case ".hoom/verdicts/":
		return verdict.EsNombre(name)
	}
	return false
}

// noRegular is the tampering of an entry created under the evidence that is
// not a regular file, tipo being what it is.
func noRegular(p, tipo string) string {
	return "la evidencia es un archivo regular: " + p + " no lo es (" + tipo + ")"
}

// pisoEnDisco is the append-only floor measured on disk, for every role
// and every tree the run could write to: an evidence file that existed and
// changed or disappeared is tampering, and a created one is legitimate only
// with the shape hoom writes (a created directory never is) — whatever git
// lists or ignores. Photographs
// without Huellas (built by hand) do not apply it. A path the git-based floor
// already marked keeps its one violation.
func pisoEnDisco(sc ScopeResult, arboles ...Fotos) ScopeResult {
	marcadas := map[string]bool{}
	for _, v := range sc.Violations {
		if v.Rule == RuleTampering {
			marcadas[v.Path] = true
		}
	}
	nuevas := map[string]string{}
	for _, f := range arboles {
		if f.Antes.Huellas == nil || f.Despues.Huellas == nil {
			continue
		}
		for p, h := range f.Antes.Huellas {
			if h == HuellaIlegible {
				// evidence hoom could not read BEFORE the run: whatever it
				// holds now cannot be compared, so nothing after it is vouched
				// for either (a dir made unreadable in one run would make its
				// files look new in the next)
				nuevas[p] = "la evidencia no se puede leer: " + p
				continue
			}
			switch d, ok := f.Despues.Huellas[p]; {
			case !ok:
				nuevas[p] = "la evidencia es append-only: " + p + " desaparecio durante el run"
			case d != h:
				nuevas[p] = "la evidencia es append-only: " + p + " cambio durante el run"
			}
		}
		for p, d := range f.Despues.Huellas {
			if d == HuellaIlegible {
				// what hoom cannot read it cannot vouch for: fail closed
				nuevas[p] = "la evidencia no se puede leer: " + p
				continue
			}
			if _, ok := f.Antes.Huellas[p]; ok {
				continue
			}
			if strings.HasPrefix(d, HuellaNoRegular) {
				tipo, _, _ := strings.Cut(strings.TrimPrefix(d, HuellaNoRegular), ":")
				nuevas[p] = noRegular(p, tipo)
				continue
			}
			if conForma(p) {
				continue
			}
			if strings.HasPrefix(p, ".hoom/approvals/") {
				nuevas[p] = "la aprobacion humana no la escribe un agente (usa 'hoom spec approve')"
				continue
			}
			dir := strings.SplitN(strings.TrimPrefix(p, ".hoom/"), "/", 2)[0]
			nuevas[p] = "bajo .hoom/" + dir + " solo se crean archivos con la forma que escribe hoom: " + p
		}
		for p := range f.Despues.Directorios {
			// a directory is an entry that is not a regular file too: empty,
			// git does not even list it, and named like a resolution or an
			// approval it blocks the one hoom would write there
			if _, ya := nuevas[p]; !ya && !f.Antes.Directorios[p] {
				nuevas[p] = noRegular(p, fs.ModeDir.String())
			}
		}
	}
	rutas := make([]string, 0, len(nuevas))
	for p := range nuevas {
		if !marcadas[p] {
			rutas = append(rutas, p)
		}
	}
	sort.Strings(rutas)
	for _, p := range rutas {
		sc.Violations = append(sc.Violations, Violation{Path: p, Rule: RuleTampering, Detail: nuevas[p]})
		if !contains(sc.Touched, p) {
			sc.Touched = append(sc.Touched, p)
		}
		sc.Tampering, sc.OK = true, false
	}
	sort.Strings(sc.Touched)
	sortViolations(sc.Violations)
	return sc
}

// sinCierres marks every finding resolution the run created, in any of the
// trees it could write to: only the refutador closes findings inside a run
// (contract 09), and a role that writes the .res.json by hand, without the
// CLI, is caught all the same. It compares the photographs of the disk
// (Evidence), not what git lists: a resolution git ignores (.gitignore,
// .git/info/exclude) still closes its finding, so it is still a closing. A
// path created in more than one tree is one closing. An existing resolution
// that changed is already tampering (universal, append-only).
func sinCierres(sc ScopeResult, role agents.Role, arboles ...Fotos) ScopeResult {
	creadas := map[string]bool{}
	for _, f := range arboles {
		for p := range f.Despues.Evidence {
			if strings.HasPrefix(p, ".hoom/findings/") && strings.HasSuffix(p, ".res.json") && !f.Antes.Evidence[p] {
				creadas[p] = true
			}
		}
	}
	rutas := make([]string, 0, len(creadas))
	for p := range creadas {
		rutas = append(rutas, p)
	}
	sort.Strings(rutas)
	for _, p := range rutas {
		sc.Violations = append(sc.Violations, Violation{Path: p, Rule: RuleTampering,
			Detail: fmt.Sprintf("un %s no cierra hallazgos: los cierra el refutador (refutado) o una persona", role.Slug)})
		if !contains(sc.Touched, p) {
			sc.Touched = append(sc.Touched, p)
		}
		sc.Tampering, sc.OK = true, false
	}
	sort.Strings(sc.Touched)
	sortViolations(sc.Violations)
	return sc
}

// universal applies the append-only floor to one path.
func universal(p string, before Snapshot, loosened []string) (Violation, bool) {
	switch {
	case p == manifest.FileName:
		return Violation{Path: p, Rule: RuleTampering,
			Detail: "hoom.yaml define la exigencia: un rol no la cambia"}, true
	case p == ".hoom/.gitignore":
		return Violation{Path: p, Rule: RuleTampering,
			Detail: "que queda fuera de Git lo decide hoom, no un rol (.hoom/.gitignore)"}, true
	case strings.HasPrefix(p, ".hoom/approvals/"):
		return Violation{Path: p, Rule: RuleTampering,
			Detail: "la aprobacion humana no la escribe un agente (usa 'hoom spec approve')"}, true
	case strings.HasPrefix(p, ".hoom/items/"):
		return Violation{Path: p, Rule: RuleTampering,
			Detail: "el item lo escribe una persona (hoom item add) y su cierre hoom task done; un rol no mueve su tarjeta ni afloja su presupuesto"}, true
	case strings.HasPrefix(p, ".hoom/reviews/"):
		return Violation{Path: p, Rule: RuleTampering,
			Detail: "el registro de review lo escribe hoom review, no un rol"}, true
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
