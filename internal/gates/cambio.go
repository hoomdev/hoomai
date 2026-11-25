package gates

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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
	// change (ante la duda, el gate corre), the change touches the
	// project's hoom.yaml (CA-812), or may touch it (a hoom.yaml of the
	// change when the one found is above the directory asked about), or that
	// hoom.yaml is a symbolic link. verifycmd.Run sets it too when the
	// hoom.yaml the manifest was loaded from is no longer there, or no longer
	// has the bytes it was loaded from (manifest.Manifest.HoomYAMLSigueIgual).
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
//     and when the change may touch the project's hoom.yaml (CA-812):
//     Archivos holds the project's hoom.yaml (the nearest one from dir up,
//     as manifest.Find walks), as a path relative to the root ("hoom.yaml"
//     when it is at the root), ignoring case; the file system cannot say
//     where that hoom.yaml is (an error other than "it does not exist",
//     finding 22b671); git cannot say where it is in the repository; or,
//     when dir has no hoom.yaml of its own (none is found, or the one found
//     is in a directory above dir), a file of Archivos is named hoom.yaml,
//     ignoring case, in any directory: it may be the project's, deleted or
//     moved (finding 8c4016). In these last cases Archivos still holds the
//     change: ante la duda, el gate corre. With no hoom.yaml in Archivos and
//     none found, the project's hoom.yaml, wherever it is, is not in the
//     change; with one found above dir, only that one counts.
//   - Todos is also true, always, when the project's hoom.yaml is a
//     symbolic link, to any target, inside the repository or outside it
//     (finding e25ae5): the manifest verify loads is its target, which the
//     change may touch under another name, or git may not see at all.
//
// It reads git and the file system (where the hoom.yaml is, and whether it
// is a symbolic link, without following it), and changes nothing.
func CambioDe(dir, base string) Cambio {
	lineas, err := gitx.LineasDelCambio(dir, base)
	if err != nil {
		// outside git, or git cannot say the change: ante la duda, el gate
		// corre
		return Cambio{Todos: true}
	}
	archivos := gitx.ArchivosConBorrados(nil, lineas)
	return Cambio{Archivos: archivos, Todos: len(archivos) == 0 || tocaElHoomYAML(dir, archivos)}
}

// tocaElHoomYAML says whether archivos (paths relative to the root of the
// repository, with "/") may hold the project's hoom.yaml, the one
// buscarHoomYAML finds from dir, ignoring case like the patterns of
// aplica_en. Ante la duda, yes: when buscarHoomYAML cannot tell (finding
// 22b671) or finds a symbolic link (finding e25ae5); when git cannot say
// where that hoom.yaml is in the repository (finding fe6ef8); and, when dir
// has no hoom.yaml of its own (there is none up to the root, or the one
// found is in a directory above dir), when any file of archivos is named
// hoom.yaml: dir's may have been deleted or moved, and the one above is
// then not the project's (finding 8c4016).
func tocaElHoomYAML(dir string, archivos []string) bool {
	proyecto, hay, dudoso := buscarHoomYAML(dir, os.Lstat)
	if dudoso {
		return true
	}
	// buscarHoomYAML already said dudoso if dir cannot be made absolute
	abs, _ := filepath.Abs(dir)
	if !hay || proyecto != abs {
		for _, f := range archivos {
			if strings.EqualFold(path.Base(f), manifest.FileName) {
				return true
			}
		}
		if !hay {
			return false
		}
	}
	prefijo, _, err := gitx.OutPrefix(proyecto, 0, "-c", "core.quotePath=false", "rev-parse", "--show-prefix")
	if err != nil {
		return true
	}
	yaml := filepath.ToSlash(sinFinDeLinea(prefijo)) + manifest.FileName
	for _, f := range archivos {
		if strings.EqualFold(f, yaml) {
			return true
		}
	}
	return false
}

// buscarHoomYAML walks up from dir the way manifest.Find does, to the first
// directory with an entry named hoom.yaml: proyecto is that directory, and
// hay false when no directory up to the root has one. Unlike Find, it reads
// each entry with lstat (os.Lstat), which does not follow a symbolic link,
// and only fs.ErrNotExist means "not here": dudoso is true, and the walk
// stops, on any other error of lstat, or when dir cannot be made absolute
// (finding 22b671: Find takes any error of os.Stat for "not here", and may
// give a parent's hoom.yaml as the project's), and when the entry found is
// a symbolic link (finding e25ae5). manifest.Find stays as it is: it has
// many callers.
func buscarHoomYAML(dir string, lstat func(string) (fs.FileInfo, error)) (proyecto string, hay, dudoso bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false, true
	}
	for {
		fi, err := lstat(filepath.Join(abs, manifest.FileName))
		switch {
		case err == nil:
			// a link: its target is the manifest verify loads, under any
			// name, inside the repository or outside it; it is not followed
			return abs, true, fi.Mode()&fs.ModeSymlink != 0
		case !errors.Is(err, fs.ErrNotExist):
			return "", false, true
		}
		padre := filepath.Dir(abs)
		if padre == abs {
			return "", false, false
		}
		abs = padre
	}
}

// sinFinDeLinea is git's output without the line end git puts after it, and
// nothing more: the bytes of the path, a leading or trailing space included,
// stay as they are (finding 09a40f).
func sinFinDeLinea(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
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
