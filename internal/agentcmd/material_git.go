package agentcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
)

// RuleMaterialDeGit (autonomia-entre-firmas, aclaracion 25): a role of shape
// codigo (the writer) changed the git material of its task, what the pilot's
// save runs (aclaracion 23): a file of the effective hooks directory of the
// task's tree (`git rev-parse --git-path hooks`: the common hooks/ or
// core.hooksPath's, inside or outside the tree), its content, its execute
// permission or a symlink's target, created or removed; the common config;
// the tree's config.worktree; or the common info/attributes (MaterialGit).
// A photograph of it that does not end within its bound counts as changed
// (FotoSinTerminar, aclaracion 28). It cuts like RuleTestsDelTestWriter: the envelope closes in scope,
// no-entregable, exit 1, with no verify, and its note is NotaMaterialDeGit
// (the tampering note when the same run also tampered with the evidence,
// NotaTestsDelTestWriter when it also touched a protected test). Its
// Violation.Detail is DetalleMaterialDeGit and its Violation.Path names what
// changed (relative to the tree when it is inside it, absolute otherwise),
// so its finding (high, author AutorGateDeScope) names it.
// agents.<rol>.write of hoom.yaml cannot lift it.
const RuleMaterialDeGit = "material-de-git"

// DetalleMaterialDeGit is the Detail of a RuleMaterialDeGit violation.
const DetalleMaterialDeGit = "los hooks, la config de git y info/attributes de la tarea los corre el guardado del piloto: el writer no los cambia"

// NotaMaterialDeGit is the note of an envelope cut by RuleMaterialDeGit.
const NotaMaterialDeGit = "el writer cambio el material de git de su tarea: no se emite veredicto sobre este arbol"

// MaterialGit is the git material of a task's tree as the envelope read it
// BEFORE the run (FotoMaterialGit): by path as RuleMaterialDeGit names it,
// the same Huellas the disk floor uses (a regular file's adds whether it can
// be executed; a large one's is HuellaGrande), or ProtegidoAusente for one
// that was not there; FotoSinTerminar alone when the photograph did not end.
type MaterialGit map[string]string

// maxLeidoMaterialGit is the largest regular file of the git material the
// photograph reads (aclaracion 28): a larger one is not read, its Huella is
// HuellaGrande.
const maxLeidoMaterialGit = 1 << 20

// HuellaGrande prefixes the Huella of a regular file of the git material
// larger than maxLeidoMaterialGit: its size, its mode and its modification
// time, never its content. Changing its size, mode or time changes it.
const HuellaGrande = "grande:"

// topeFotoMaterialGit bounds a whole FotoMaterialGit (aclaracion 28): a
// photograph that does not end by then is FotoSinTerminar. Tests shrink it.
var topeFotoMaterialGit = 5 * time.Second

// FotoSinTerminar is the one entry of the MaterialGit of a photograph that
// did not end within its bound: as a path it says so, and against any other
// photograph, itself included, it counts as changed git material.
const FotoSinTerminar = "el material de git (su foto no termino a tiempo)"

// FotoMaterialGit photographs the git material of the tree dir: the
// effective hooks directory and each entry right inside it (the ones git
// runs), the common config, the tree's config.worktree and the common
// info/attributes, read from the disk, never followed, a file larger than
// maxLeidoMaterialGit not read. Outside a git repository there is none: an
// empty MaterialGit. When git does not say where it is, the entry "git" is
// HuellaIlegible. A photograph that does not end within
// topeFotoMaterialGit is FotoSinTerminar alone (aclaracion 28).
func FotoMaterialGit(dir string) MaterialGit {
	limite := time.Now().Add(topeFotoMaterialGit)
	listo := make(chan MaterialGit, 1)
	go func() { listo <- fotoMaterialGit(dir, limite) }()
	t := time.NewTimer(time.Until(limite))
	defer t.Stop()
	select {
	case foto := <-listo:
		if foto != nil {
			return foto
		}
	case <-t.C:
	}
	return MaterialGit{FotoSinTerminar: HuellaIlegible}
}

// fotoMaterialGit is FotoMaterialGit without its bound; nil once it passes
// limite.
func fotoMaterialGit(dir string, limite time.Time) MaterialGit {
	foto := MaterialGit{}
	out, err := gitEn(dir, "rev-parse", "--path-format=absolute", "--git-path", "hooks", "--git-common-dir", "--git-dir")
	lineas := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if err != nil || len(lineas) != 3 {
		var ee *exec.ExitError
		if errors.As(err, &ee) && strings.Contains(string(ee.Stderr), "not a git repository") {
			return foto
		}
		foto["git"] = HuellaIlegible
		return foto
	}
	hooks, comun, gitDir := lineas[0], lineas[1], lineas[2]
	nombre := func(p string) string {
		if rel, err := filepath.Rel(dir, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(p)
	}
	rutas := []string{hooks}
	if st, err := os.Stat(hooks); err == nil && st.IsDir() {
		entradas, err := os.ReadDir(hooks)
		if err != nil {
			foto[nombre(hooks)+"/"] = HuellaIlegible
		}
		for _, e := range entradas {
			rutas = append(rutas, filepath.Join(hooks, e.Name()))
		}
	}
	rutas = append(rutas, filepath.Join(comun, "config"), filepath.Join(gitDir, "config.worktree"),
		filepath.Join(comun, "info", "attributes"))
	for _, p := range rutas {
		if time.Now().After(limite) {
			return nil
		}
		foto[nombre(p)] = huellaMaterial(p)
	}
	return foto
}

// huellaMaterial is the Huella of the entry at p: ProtegidoAusente when it
// is not there, HuellaGrande for a regular file larger than
// maxLeidoMaterialGit, a regular file's content hash with ":x" when someone
// can execute it, the disk floor's for the rest.
func huellaMaterial(p string) string {
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return ProtegidoAusente
	}
	if err != nil {
		return HuellaIlegible
	}
	if fi.Mode().IsRegular() && fi.Size() > maxLeidoMaterialGit {
		return fmt.Sprintf("%s%d:%s:%d", HuellaGrande, fi.Size(), fi.Mode(), fi.ModTime().UnixNano())
	}
	r, err := os.OpenRoot(filepath.Dir(p))
	if err != nil {
		return HuellaIlegible
	}
	defer r.Close()
	h := huella(r, filepath.Base(p), fi)
	if fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
		h += ":x"
	}
	return h
}

// sinTocarMaterialGit applies RuleMaterialDeGit: for a role of shape
// codigo, every entry of the git material of dir that is not the one
// photographed before the run (antes) is a violation, whatever the role's
// Allow says. It replaces an out-of-scope violation of the same path and
// leaves any other one alone, like sinTocarProtegidos.
func sinTocarMaterialGit(sc ScopeResult, dir string, role agents.Role, antes MaterialGit) ScopeResult {
	if role.Scope != agents.ScopeCodigo || antes == nil {
		return sc
	}
	ahora := FotoMaterialGit(dir)
	tocadas := map[string]bool{}
	_, antesSin := antes[FotoSinTerminar]
	_, ahoraSin := ahora[FotoSinTerminar]
	switch {
	case antesSin || ahoraSin:
		// a photograph that did not end proves nothing: fail closed
		tocadas[FotoSinTerminar] = true
	default:
		for p, h := range antes {
			if ahora[p] != h {
				tocadas[p] = true
			}
		}
		for p := range ahora {
			if _, ok := antes[p]; !ok {
				tocadas[p] = true
			}
		}
	}
	if len(tocadas) == 0 {
		return sc
	}
	kept := sc.Violations[:0]
	for _, v := range sc.Violations {
		if tocadas[v.Path] && v.Rule == RuleOutOfScope {
			continue
		}
		if tocadas[v.Path] {
			delete(tocadas, v.Path)
		}
		kept = append(kept, v)
	}
	sc.Violations = kept
	rutas := make([]string, 0, len(tocadas))
	for p := range tocadas {
		rutas = append(rutas, p)
	}
	sort.Strings(rutas)
	for _, p := range rutas {
		sc.Violations = append(sc.Violations, Violation{Path: p, Rule: RuleMaterialDeGit, Detail: DetalleMaterialDeGit})
	}
	sortViolations(sc.Violations)
	sc.OK = false
	return sc
}

// DeMaterialDeGit says whether f is the finding the scope gate registered
// for a RuleMaterialDeGit violation in a run of role rol
// (DescripcionViolacion, author AutorGateDeScope, severity high), whatever
// its path. The pilot reads with it which envelopes changed the git material
// of the task (aclaracion 25).
func DeMaterialDeGit(f finding.Finding, rol string) bool {
	return deRegla(f, rol, RuleMaterialDeGit, DetalleMaterialDeGit)
}

// deRegla says whether f is the finding of the scope gate for a violation
// of rule regla with detail detalle in a run of role rol.
func deRegla(f finding.Finding, rol, regla, detalle string) bool {
	pre, suf, _ := strings.Cut(DescripcionViolacion(Violation{Rule: regla, Path: "\x00", Detail: detalle}, rol), "\x00")
	return f.Author == AutorGateDeScope && f.Severity == "high" &&
		len(f.Description) > len(pre)+len(suf) && strings.HasPrefix(f.Description, pre) && strings.HasSuffix(f.Description, suf)
}
