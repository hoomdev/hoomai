package agentcmd

import "github.com/hoomdev/hoomai/internal/finding"

// RuleConfigDeAgentes (contencion-del-writer, "La config de los agentes"): a
// role that executes changed what makes code run in the next session of an
// agent's CLI (ConfigAgentes):
//   - in the tree: `.claude/`, `.codex/`, `.mcp.json` and what each adapter
//     declares in providers.Estado.EnArbol, created, changed or removed;
//   - outside the tree: what each adapter declares in providers.Estado.Corre
//     (claude: the key `hooks` of `~/.claude/settings.json` and of
//     `~/.claude/settings.local.json`, and `mcpServers`, global and of each
//     project, of `~/.claude.json`; codex: the whole `~/.codex/config.toml`).
//
// That the CLI changes other keys of those JSON files (its counters, its
// history) is not it. It holds for every role that executes, with encierro
// or without it, in any project, and agents.<rol>.write of hoom.yaml cannot
// lift it. In the envelope it is compared after the run, before any git of
// hoom on the tree, and cuts like RuleMaterialDeGit, without running git
// status: the envelope closes in scope, no-entregable, exit 1, with no
// verify, its note is NotaConfigDeAgentes(rol), its Violation.Detail
// DetalleConfigDeAgentes and its Violation.Path what changed (as
// CambiosConfigAgentes names it), so its finding (high, author
// AutorGateDeScope) names it. The review compares it around each pass
// (reviewcmd.PasadaConConfigDeAgentes).
const RuleConfigDeAgentes = "config-de-agentes"

// DetalleConfigDeAgentes is the Detail of a RuleConfigDeAgentes violation.
const DetalleConfigDeAgentes = "la config de los agentes hace correr codigo en la proxima sesion: ningun rol que ejecuta la cambia"

// NotaConfigDeAgentes is the note of an envelope of role rol cut by
// RuleConfigDeAgentes:
// `el <rol> cambio la config de los agentes: no se emite veredicto sobre este arbol`.
func NotaConfigDeAgentes(rol string) string {
	return ""
}

// DeConfigDeAgentes says whether f is the finding the scope gate registered
// for a RuleConfigDeAgentes violation in a run of role rol
// (DescripcionViolacion, author AutorGateDeScope, severity high), whatever its
// path. The pilot reads with it which envelopes changed the agents' config,
// and treats them as the ones that changed the git material
// (boardcmd.Sobre).
func DeConfigDeAgentes(f finding.Finding, rol string) bool {
	return false
}

// ConfigAgentes is the agents' config of a tree as RuleConfigDeAgentes
// compares it (FotoConfigAgentes): by path -> its Huella. A path inside the
// tree is relative to it, with "/" (each file under `.claude/`, `.codex/`
// and the directories of EnArbol is its own entry); a path outside it is
// absolute. For a file of providers.Estado.Corre with Claves, the Huella is
// that of the value of those keys only; without Claves, that of the whole
// file. ProtegidoAusente for what is not there, HuellaIlegible for what
// could not be read.
type ConfigAgentes map[string]string

// FotoConfigAgentes photographs the agents' config of the tree dir: in the
// tree, `.claude/`, `.codex/`, `.mcp.json` and the EnArbol of every provider
// of providers.Default that implements providers.ConEstado; outside it, the
// Corre of each of them ("~/" being this process's $HOME). It reads files
// only, never follows a symlink, and runs no git.
func FotoConfigAgentes(dir string) ConfigAgentes {
	return nil
}

// CambiosConfigAgentes lists, sorted, the paths whose entry is not the same
// in antes and despues: changed, appeared or gone. nil when none.
func CambiosConfigAgentes(antes, despues ConfigAgentes) []string {
	return nil
}
