package gates

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

// MotivoNoAplica (gates-por-ruta) is the note of a gate in
// verdict.StatusNoAplica, exactly: %s is the gate's aplica_en
// (manifest.Gate.AplicaEn), its patterns as written and in their order,
// joined by ", ". With aplica_en: [app/**, src/**] it is
// `ningun archivo del cambio coincide con aplica_en (app/**, src/**)`.
const MotivoNoAplica = "ningun archivo del cambio coincide con aplica_en (%s)"

// Cambio (gates-por-ruta) is the change a verify judges aplica_en against
// (Options.Cambio): what CambioDe gives.
type Cambio struct {
	// Archivos are the files of the change, the ones of
	// gitx.ArchivosConBorrados: the commits since the merge-base with the
	// base, the local changes, the untracked files and the deleted ones
	// (deleted in a commit or in the working tree). Each one is relative to
	// the root of the repository, with "/", whatever directory hoom runs
	// in (the untracked ones too); a non-ASCII name comes as it is, without
	// git's quotes; the exclusions of the fingerprint (.hoom/verdicts/,
	// .hoom/findings/, ...) are left out. Sorted, without repeats.
	Archivos []string
	// Todos says every gate applies, whatever its aplica_en: the change is
	// empty, the directory is not in a git repository, git cannot say the
	// change (ante la duda, el gate corre), or the change touches the
	// project's hoom.yaml (CA-812).
	Todos bool
}

// CambioDe (gates-por-ruta) is the change of dir against the branch base:
// the verify passes manifest.Manifest.Dir and BaseBranch. dir may be any
// directory of the repository: the paths are always relative to its root.
//
//   - Archivos is what Cambio.Archivos says.
//   - Todos is true when Archivos is empty; when dir is not in a git
//     repository (Archivos nil); when git cannot say the change (no
//     merge-base with base, a diff or a listing that fails: Archivos nil);
//     and when Archivos holds the hoom.yaml that manifest.Find(dir) finds,
//     as a path relative to the root ("hoom.yaml" when it is at the root;
//     with no hoom.yaml found, this last rule says nothing).
//
// It reads git and nothing else, and changes nothing.
func CambioDe(dir, base string) Cambio {
	lineas, err := gitx.LineasDelCambio(dir, base)
	if err != nil {
		// outside git, or git cannot say the change: ante la duda, el gate
		// corre
		return Cambio{Todos: true}
	}
	archivos := gitx.ArchivosConBorrados(nil, lineas)
	c := Cambio{Archivos: archivos, Todos: len(archivos) == 0}
	if yaml, ok := hoomYAMLDe(dir); ok {
		for _, f := range archivos {
			if f == yaml {
				c.Todos = true
				break
			}
		}
	}
	return c
}

// hoomYAMLDe is the path, relative to the root of the repository, of the
// hoom.yaml manifest.Find(dir) finds; false when there is none, or when git
// cannot say where its directory is in the repository.
func hoomYAMLDe(dir string) (string, bool) {
	proyecto, err := manifest.Find(dir)
	if err != nil {
		return "", false
	}
	prefijo, _, err := gitx.OutPrefix(proyecto, 0, "-c", "core.quotePath=false", "rev-parse", "--show-prefix")
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(strings.TrimSpace(prefijo)) + manifest.FileName, true
}

// Aplica (gates-por-ruta) says whether gate g applies in a verify de trabajo
// whose change is c, and, when it does not, why. It applies when g has no
// aplica_en (len(g.AplicaEn) == 0), when c.Todos, when c.Archivos is empty,
// or when a file of c.Archivos matches one of the patterns of g.AplicaEn
// (riesgo.CoincideAlguno: the path itself or one of its parent
// directories, ignoring case). motivo is "" when it applies, and
// fmt.Sprintf(MotivoNoAplica, strings.Join(g.AplicaEn, ", ")) when it does
// not.
//
// It looks only at the patterns and the files: a gate whose cmd is empty is
// absent whatever Aplica says (Run), and Full, Cierre and Only are Run's.
func Aplica(g manifest.Gate, c Cambio) (aplica bool, motivo string) {
	if len(g.AplicaEn) == 0 || c.Todos || len(c.Archivos) == 0 {
		return true, ""
	}
	for _, f := range c.Archivos {
		if riesgo.CoincideAlguno(g.AplicaEn, f) {
			return true, ""
		}
	}
	return false, fmt.Sprintf(MotivoNoAplica, strings.Join(g.AplicaEn, ", "))
}
