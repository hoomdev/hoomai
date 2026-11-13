package agentcmd

import (
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/materialgit"
)

// RuleMaterialDeGit (autonomia-entre-firmas, aclaracion 25): a role of shape
// codigo (the writer) changed the git material of its task, what the pilot's
// save runs (aclaracion 23): a file of the effective hooks directory of the
// task's tree (`git rev-parse --git-path hooks`: the common hooks/ or
// core.hooksPath's, inside or outside the tree), its content, its execute
// permission or a symlink's target, created or removed; the common config;
// the tree's config.worktree; or the common info/attributes (MaterialGit).
// contencion-del-writer adds the pieces FotoMaterialGit lists (the git
// config the tree reads outside the repository, info/exclude,
// objects/info/alternates, objects/info/grafts, shallow, the tree's .git
// file and the hook managers of the tree), which count as this same rule,
// and two more moments: around the gates of verifycmd.Run with
// verifycmd.Options.Sobre (the gate verifycmd.GateMaterialDeGit; in the
// writer's envelope also this rule's finding, and the envelope closes in
// verify) and around the pilot's save (itemcmd.ErrHookCambioMaterial).
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

// HuellaGrande prefixes the Huella of a regular file of the git material
// larger than materialgit.MaxLeido (aclaraciones 28 y 29): its size, its
// mode, its modification time and its status change time (ctime, from the
// system's stat), never its content. Changing its size, mode or mtime
// changes it, and so does any write or chmod, which moves its ctime: a user
// process cannot set the ctime back. So overwriting it with other bytes of
// the same length and restoring its mtime counts as changing it, and so does
// changing it and restoring it byte for byte: that is the price of not
// reading it (a small file is still compared by its content). Where the
// system gives no ctime (materialgit.CtimeDe), the Huella has none.
const HuellaGrande = materialgit.Grande

// topeFotoMaterialGit bounds a whole FotoMaterialGit (aclaracion 28): a
// photograph that does not end by then is FotoSinTerminar. Tests shrink it.
var topeFotoMaterialGit = 5 * time.Second

// FotoSinTerminar is the one entry of the MaterialGit of a photograph that
// did not end within its bound: as a path it says so, and against any other
// photograph, itself included, it counts as changed git material.
const FotoSinTerminar = materialgit.SinTerminar

// FotoMaterialGit photographs the git material of the tree dir: the
// effective hooks directory and each entry right inside it (the ones git
// runs), the common config, the tree's config.worktree and the common
// info/attributes, read from the disk, never followed, a file larger than
// materialgit.MaxLeido not read. Outside a git repository there is none: an
// empty MaterialGit. When git does not say where it is, the entry "git" is
// HuellaIlegible. A photograph that does not end within
// topeFotoMaterialGit is FotoSinTerminar alone (aclaracion 28).
//
// contencion-del-writer (CA-686) adds, with the same Huellas and the same
// bound:
//   - the git config the tree reads outside the repository: each file `git
//     config --list --show-origin` names as an origin in the tree, except
//     under /etc (~/.gitconfig, $XDG_CONFIG_HOME/git/config, GIT_CONFIG_GLOBAL,
//     and what an include.path or an includeIf reaches), the file of
//     core.attributesFile and $XDG_CONFIG_HOME/git/attributes;
//   - of the common directory: info/exclude, objects/info/alternates,
//     objects/info/grafts and shallow; never the rest of objects/info/
//     (commit-graphs, which git's maintenance writes after each commit);
//   - the tree's .git file;
//   - the hook managers of the tree: when the effective hooks directory is
//     inside the tree, the entries right inside its parent directory (husky
//     calls .husky/<hook> from .husky/_/); always .pre-commit-config.yaml,
//     lefthook.yml, lefthook.yaml, .lefthook.yml, .lefthook.yaml and
//     lefthook-local.yml of the tree's root, and the directory .lefthook/.
//
// Each piece is named as RuleMaterialDeGit names it (relative to the tree
// when inside it, absolute otherwise), and one that is not there is
// ProtegidoAusente, so creating it is a change. The global config files
// count whether they exist or not; the husky parent does not count when it
// is the tree's root or a git directory. The git it runs carries
// core.fsmonitor=false. It is materialgit.Tomar within its bound.
func FotoMaterialGit(dir string) MaterialGit {
	return MaterialGit(materialgit.TomarConTope(dir, topeFotoMaterialGit))
}

// CambiosMaterialGit lists, sorted, the paths whose entry is not the same in
// antes and despues (FotoMaterialGit): changed, appeared or gone. When either
// photograph is FotoSinTerminar it is FotoSinTerminar alone (a photograph that
// did not end proves nothing). nil when none.
func CambiosMaterialGit(antes, despues MaterialGit) []string {
	if antes == nil && despues == nil {
		return nil
	}
	return materialgit.Cambios(materialgit.Foto(antes), materialgit.Foto(despues))
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
	return conMaterialGit(sc, CambiosMaterialGit(antes, FotoMaterialGit(dir)))
}

// conMaterialGit adds to sc one RuleMaterialDeGit violation for each path of
// rutas (CambiosMaterialGit; a photograph that did not end is
// FotoSinTerminar, which fails closed). It replaces an out-of-scope violation
// of the same path and leaves any other one alone, like sinTocarProtegidos.
func conMaterialGit(sc ScopeResult, rutas []string) ScopeResult {
	return conRegla(sc, rutas, RuleMaterialDeGit, DetalleMaterialDeGit)
}

// conRegla adds to sc one violation of rule regla with detail detalle for
// each path of rutas, replacing an out-of-scope violation of the same path
// and leaving any other one alone. sc is unchanged when rutas is empty.
func conRegla(sc ScopeResult, rutas []string, regla, detalle string) ScopeResult {
	tocadas := map[string]bool{}
	for _, p := range rutas {
		tocadas[p] = true
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
	quedan := make([]string, 0, len(tocadas))
	for p := range tocadas {
		quedan = append(quedan, p)
	}
	sort.Strings(quedan)
	for _, p := range quedan {
		sc.Violations = append(sc.Violations, Violation{Path: p, Rule: regla, Detail: detalle})
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
