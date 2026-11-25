// Package verdict defines hoomAI's core artifact: the verification verdict.
// Verdicts are append-only JSON files under .hoom/verdicts/ — plain,
// diffable, merge-conflict-free by construction (one file per run, unique
// name), versioned in Git so history travels with the project across
// machines and CI. Any local index (SQLite) is a regenerable cache derived
// from these files, never the source of truth.
package verdict

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

const SchemaID = "hoom.verdict/v1"

// Gate status values.
const (
	StatusPass    = "pass"    // gate ran, exit 0
	StatusFail    = "fail"    // gate ran, non-zero exit
	StatusError   = "error"   // configured but could not run (missing tool, timeout) — fail closed
	StatusAbsent  = "absent"  // no command configured — declared degradation, reported loudly
	StatusSkipped = "skipped" // excluded by --gate selection
	// StatusNoAplica (gates-por-ruta) is a gate of hoom.yaml with aplica_en
	// that no file of the change matches, in a verify de trabajo (without
	// --cierre, --full or --gate): its command did not run, its scope is
	// "none" and its notes say why (gates.MotivoNoAplica). It never turns
	// the verdict red nor makes it partial (Finalize, IsPartial), and it
	// counts in Summary.NoAplica. The cierre does not accept it:
	// taskcmd.Ready refuses a latest complete verdict with one.
	StatusNoAplica = "no_aplica"
)

// GateResult is the outcome of one gate execution.
type GateResult struct {
	Name       string `json:"name"`
	Required   bool   `json:"required"`
	Status     string `json:"status"`
	Scope      string `json:"scope"` // full | diff | none
	Command    string `json:"command,omitempty"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	OutputTail string `json:"output_tail,omitempty"`
	Partial    string `json:"partial,omitempty"`
	Notes      string `json:"notes,omitempty"`
	// Entorno (verify-en-el-entorno) is where the gate ran when
	// .hoom/local.yaml has ejecutar (Verdict.Entorno not nil): EntornoPrefijo
	// for a gate of hoom.yaml that runs with the template, EntornoHost for
	// one of en_el_host and for the gates hoom itself computes (spec_lint,
	// spec_trace, spec_approved, findings_open, ratchet, material_de_git,
	// git_ajeno, entorno_local). A gate of hoom.yaml that is absent or
	// skipped carries where it would have run. "" (no key in the JSON)
	// without ejecutar.
	Entorno string `json:"entorno,omitempty"`
}

// The values of GateResult.Entorno (verify-en-el-entorno).
const (
	EntornoPrefijo = "prefijo" // ran with the template of .hoom/local.yaml
	EntornoHost    = "host"    // ran as today, on the host
)

// Entorno (verify-en-el-entorno) is the key `entorno` at the top of a verdict
// whose gates ran with the template of .hoom/local.yaml:
// `{"ejecutar": "<plantilla>", "sha256": "<de .hoom/local.yaml>"}`.
type Entorno struct {
	// Ejecutar is the template as the file writes it (entorno.Ajuste.Ejecutar).
	Ejecutar string `json:"ejecutar"`
	// SHA256 is the hex sha256 of the bytes of .hoom/local.yaml
	// (entorno.Ajuste.SHA256).
	SHA256 string `json:"sha256"`
}

// Verdict is one immutable verification run.
type Verdict struct {
	Schema    string       `json:"schema"`
	ID        string       `json:"id"`
	CreatedAt time.Time    `json:"created_at"`
	Project   string       `json:"project"`
	Profile   string       `json:"profile"`
	Policy    string       `json:"policy"`
	Git       gitx.Info    `json:"git"`
	Gates     []GateResult `json:"gates"`
	Summary   Summary      `json:"summary"`
	Verdict   string       `json:"verdict"` // green | red
	// Partial marks a --gate scoped run: a diagnostic artifact that must
	// NEVER serve as reference for check/task done (rule 4b by construction).
	Partial bool     `json:"partial,omitempty"`
	Spec    string   `json:"spec,omitempty"`
	Notes   []string `json:"notes,omitempty"`
	// Entorno (verify-en-el-entorno) is where the gates ran: set when
	// .hoom/local.yaml has ejecutar, and then every GateResult carries its
	// Entorno. nil (no key `entorno` in the JSON) without ejecutar: the JSON
	// is today's. It is not part of the fingerprint, which is the tree's.
	Entorno *Entorno `json:"entorno,omitempty"`
	// Riesgo (riesgo-efectivo-y-tamano) is the riesgo of the change as this
	// verify computed it (riesgo.Clasificador.Evaluar): the card of the
	// verify's task (Tarea) and the files of the change, with the rutas and
	// the test-writer's territory of the hoom.yaml it verifies with. Every
	// verify writes it: {"declarado", "efectivo", "motivo"}, with declarado
	// normal when the card does not say it or there is no task. nil (no key)
	// only in a verdict older than that spec.
	Riesgo *riesgo.Evaluacion `json:"riesgo,omitempty"`
	// Tamano (riesgo-efectivo-y-tamano) is the size of the change
	// (riesgo.Clasificador.Medir over gitx.LineasDelCambio):
	// {"producto", "pruebas", "hoom"}. nil (no key) outside a git
	// repository and in a verdict older than that spec. Git's Insertions and
	// Deletions stay as they were.
	//
	// Neither Riesgo nor Tamano is part of the fingerprint, which is the
	// tree's (I2): the same tree gives the same Git.ChangeFingerprint with
	// these keys as without them.
	Tamano *riesgo.Tamano `json:"tamano,omitempty"`
	// Tarea is the verify's task (riesgo-efectivo-y-tamano): the one of the
	// envelope of `hoom agent` (verifycmd.Sobre.Task), or the one of the
	// worktree .hoom/worktrees/<slug> where `hoom verify` runs
	// (runcmd.TareaDe); "" in any other tree. Never in the JSON (the spec
	// adds only riesgo and tamano): a verdict read back has it empty. The
	// text of `hoom verify` (report.Render) names it in
	// riesgo.AvisoMayorQueDeclarado.
	Tarea string `json:"-"`
}

// Summary counts the gates of a verdict by status.
type Summary struct {
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Error   int `json:"error"`
	Absent  int `json:"absent"`
	Skipped int `json:"skipped"`
	// NoAplica (gates-por-ruta) counts the gates in StatusNoAplica. The key
	// no_aplica is left out of the JSON when it is 0, so a verdict without
	// such gates serializes as today.
	NoAplica int `json:"no_aplica,omitempty"`
}

// Finalize computes the summary and the strict verdict: any fail or error
// on a required gate turns it red. Declared absence is never silent (it is
// counted and rendered loudly) but is not red; configured-but-broken is.
//
// gates-por-ruta: a gate in StatusNoAplica, required or not, counts in
// Summary.NoAplica and never turns the verdict red.
func (v *Verdict) Finalize() {
	s := Summary{}
	red := false
	for _, g := range v.Gates {
		switch g.Status {
		case StatusPass:
			s.Pass++
		case StatusFail:
			s.Fail++
			if g.Required {
				red = true
			}
		case StatusError:
			s.Error++
			if g.Required {
				red = true
			}
		case StatusAbsent:
			s.Absent++
		case StatusSkipped:
			s.Skipped++
		case StatusNoAplica:
			s.NoAplica++
		}
	}
	v.Summary = s
	v.Verdict = "green"
	if red {
		v.Verdict = "red"
	}
}

// IsPartial reports whether this verdict came from a --gate scoped run.
// Verdicts written before the field existed are detected by heuristic:
// the gate runner only ever emits skipped for --gate exclusion, so any
// skipped gate other than spec_lint/spec_trace (which complete runs can
// legitimately skip) implies a partial run.
//
// gates-por-ruta: StatusNoAplica is not skipped: a verdict whose gates are
// no_aplica is complete, never partial, and LatestComplete may return it.
func (v *Verdict) IsPartial() bool {
	if v.Partial {
		return true
	}
	for _, g := range v.Gates {
		if g.Status == StatusSkipped && g.Name != "spec_lint" && g.Name != "spec_trace" {
			return true
		}
	}
	return false
}

// LatestComplete returns the newest non-partial verdict, or nil when none
// exists. This is THE reference selector for check, task done and the
// Studio: partial (--gate) runs are diagnostics and never count as
// reference, in either direction (no false green, no false red).
func LatestComplete(all []*Verdict) *Verdict {
	for i := len(all) - 1; i >= 0; i-- {
		if !all[i].IsPartial() {
			return all[i]
		}
	}
	return nil
}

// The id Write gives a verdict: its second written in idFecha, "_", and the
// first idHex hex digits of the sha256 of its content. EsNombre checks a
// name against the same two, so the name hoom writes and the name the scope
// gate accepts have one definition.
const (
	idFecha = "2006-01-02T15-04-05Z"
	idHex   = 8
)

// EsNombre says whether name is the file name Write gives a verdict (its id
// plus ".json"): the one definition the scope gate uses to tell a verdict
// from anything else.
func EsNombre(name string) bool {
	id, ok := strings.CutSuffix(name, ".json")
	if !ok {
		return false
	}
	fecha, hash, ok := strings.Cut(id, "_")
	t, err := time.Parse(idFecha, fecha)
	// Format gives the same text back only for a second written as Write
	// writes it
	return ok && err == nil && t.Format(idFecha) == fecha &&
		len(hash) == idHex && strings.Trim(hash, "0123456789abcdef") == ""
}

// Write persists the verdict as .hoom/verdicts/<timestamp>_<hash>.json.
// Timestamp + content hash guarantees unique, collision-free, append-only
// files even across machines and CI running concurrently.
func Write(projectDir string, v *Verdict) (string, error) {
	v.Schema = SchemaID
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	v.ID = v.CreatedAt.Format(idFecha) + "_" + hex.EncodeToString(sum[:])[:idHex]
	raw, _ = json.MarshalIndent(v, "", "  ")

	dir := filepath.Join(projectDir, ".hoom", "verdicts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, v.ID+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// LoadAll reads every verdict in the project, oldest first. This scan IS
// the regenerable index for v1; a SQLite cache can be layered later
// without changing the source of truth. Unreadable files are skipped and
// reported to stderr, never fatal.
func LoadAll(projectDir string) ([]*Verdict, error) {
	out, warnings, err := LoadAllWithWarnings(projectDir)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "hoom:", w)
	}
	return out, err
}

// maxVerdict caps one verdict file. It is evidence, read whole, so the cap
// only stops a file no hoom would write from filling the memory; one past it
// is unreadable and comes back as a warning.
const maxVerdict = 256 << 20

// LoadAllWithWarnings is LoadAll with the skipped-file diagnostics returned
// in-band instead of printed, so callers with a UI (hoom serve) can surface
// them without touching stderr.
func LoadAllWithWarnings(projectDir string) ([]*Verdict, []string, error) {
	// down by descriptors and as regular files: a verdict that is a FIFO or
	// a symlink to another project is not read (finding 53c2af)
	var out []*Verdict
	var warnings []string
	err := hoomfs.RecorrerDirectorio(projectDir, []string{".hoom", "verdicts"},
		func(n string) bool { return strings.HasSuffix(n, ".json") }, maxVerdict,
		func(e hoomfs.Entrada) {
			if e.Err != nil {
				warnings = append(warnings, fmt.Sprintf("veredicto ilegible %s: %v", e.Nombre, e.Err))
				return
			}
			var v Verdict
			if err := json.Unmarshal(e.Contenido, &v); err != nil {
				warnings = append(warnings, fmt.Sprintf("veredicto ilegible %s: %v", e.Nombre, err))
				return
			}
			out = append(out, &v)
		})
	if err != nil {
		return nil, nil, err
	}
	// stable: ties keep the name order they were read in (finding 3cb01b)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, warnings, nil
}
