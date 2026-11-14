package agentcmd

import (
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitajeno"
)

// RuleGitAjeno (contencion-del-writer, "Git ajeno"): a role that executes
// changed, in the git common directory of its task, what nobody changes with
// reason while a role runs:
//   - a ref under refs/replace/, created, moved or removed, loose or packed
//     (a replace ref changes what git shows of an object in every branch);
//   - a loose object, a pack or its index that already existed and still
//     exists with another content. Only the files that changed after the
//     photograph are looked at (by their status change time). Creating an
//     object or removing one (gc, prune) does not count: the name of an
//     object and of a pack come from their content.
//
// Moving another branch, refs/stash or any ref outside refs/replace/ is not
// it. It holds for every role that executes (agents.Role.Exec), with
// encierro or without it, in any project, and agents.<rol>.write of
// hoom.yaml cannot lift it. In the envelope it is compared after the run and
// cuts like RuleMaterialDeGit: the envelope closes in scope, no-entregable,
// exit 1, with no verify, its note is NotaGitAjeno(rol), its Violation.Detail
// DetalleGitAjeno and its Violation.Path what changed (as CambiosGitAjeno
// names it), so its finding (high, author AutorGateDeScope, the gate's usual
// format) names it. The review compares it around each pass
// (reviewcmd.PasadaConGitAjeno) and the pilot's save around itself
// (itemcmd.ErrGitAjeno).
const RuleGitAjeno = "git-ajeno"

// DetalleGitAjeno is the Detail of a RuleGitAjeno violation.
const DetalleGitAjeno = "un rol solo cambia la rama de su tarea: las refs de reemplazo y los objetos que ya existian no los toca"

// NotaGitAjeno is the note of an envelope of role rol cut by RuleGitAjeno:
// `el <rol> cambio git fuera de la rama de su tarea: no se emite veredicto sobre este arbol`.
func NotaGitAjeno(rol string) string {
	return "el " + rol + " cambio git fuera de la rama de su tarea: no se emite veredicto sobre este arbol"
}

// DeGitAjeno says whether f is the finding the scope gate registered for a
// RuleGitAjeno violation in a run of role rol (DescripcionViolacion, author
// AutorGateDeScope, severity high), whatever its path. The pilot reads with
// it which envelopes changed git outside their task's branch, and treats
// them as the ones that changed the git material (boardcmd.Sobre).
func DeGitAjeno(f finding.Finding, rol string) bool {
	return deRegla(f, rol, RuleGitAjeno, DetalleGitAjeno)
}

// GitAjeno is the photograph of what RuleGitAjeno compares in the git common
// directory of a tree (FotoGitAjeno): its refs under refs/replace/, loose and
// packed, and the object files that existed (gitajeno.Foto). The zero
// GitAjeno never changes.
type GitAjeno = gitajeno.Foto

// TopeFotoGitAjeno and MaxLeidoGitAjeno (ronda 1, decision 27) bound each
// photograph of git-ajeno (FotoGitAjeno) and each comparison
// (CambiosGitAjeno): its time, and the bytes it reads (refs, packed-refs,
// loose objects, packs and indexes). Besides, the walk of refs/replace/ reads
// 10000 directory entries at most, symlinked directories included, a batch
// at a time, and goes 32 levels down at most (ronda 3, aclaraciones 30 and
// 30b). A photograph that passes its time, or one of those bounds, or whose
// bytes do not reach a ref, is unfinished, and against it any comparison
// gives FotoSinTerminar; a loose object beyond the bytes of the photograph
// is kept without its hash (it counts as changed if it is touched); a
// comparison that passes its time or those bounds adds FotoSinTerminar, and
// a changed object beyond its bytes counts as changed, by its path. Never
// hangs.
const (
	TopeFotoGitAjeno = gitajeno.Tope
	MaxLeidoGitAjeno = gitajeno.MaxLeido
)

// FotoGitAjeno photographs, for the tree dir, its git common directory's
// refs under refs/replace/ (loose and in packed-refs) and the files under
// objects/ (loose objects, packs and their indexes), within TopeFotoGitAjeno
// and MaxLeidoGitAjeno, reading files only: it runs no git that reads the
// working tree (no status, no fsmonitor, no filter). Outside a git
// repository there is nothing to compare: the zero GitAjeno, which
// CambiosGitAjeno never finds changed.
func FotoGitAjeno(dir string) GitAjeno {
	return FotoGitAjenoConTope(dir, TopeFotoGitAjeno, MaxLeidoGitAjeno)
}

// FotoGitAjenoConTope is FotoGitAjeno within tope and maxLeido bytes.
func FotoGitAjenoConTope(dir string, tope time.Duration, maxLeido int64) GitAjeno {
	return gitajeno.Tomar(dir, tope, maxLeido)
}

// CambiosGitAjenoConTope is CambiosGitAjeno within tope and maxLeido bytes.
func CambiosGitAjenoConTope(antes GitAjeno, tope time.Duration, maxLeido int64) []string {
	return gitajeno.Cambios(antes, tope, maxLeido)
}

// CambiosGitAjeno looks again at the git common directory antes photographed
// and lists, sorted, what breaks RuleGitAjeno since then: each ref under
// refs/replace/ created, moved or removed, loose or packed, named
// `<comun>/refs/replace/<nombre>` (absolute, the same name whether it is
// loose or packed), and each loose object, pack or index that existed in antes
// and now has other bytes (only the files whose ctime, mtime, size or inode
// changed are read), by its absolute path. A file it cannot read counts as
// changed, by its path; past TopeFotoGitAjeno, FotoSinTerminar. nil when
// nothing changed.
func CambiosGitAjeno(antes GitAjeno) []string {
	return CambiosGitAjenoConTope(antes, TopeFotoGitAjeno, MaxLeidoGitAjeno)
}
