// Package entorno is the local adjustment of one machine
// (verify-en-el-entorno): the file .hoom/local.yaml (Ruta) of the project's
// main checkout, outside git, with the template that says where the gates of
// verify run. It is for the machines where the project's runtime does not
// live on the host (DDEV, Sail, Docker Compose, devcontainers, Lando): with
// it, each gate runs inside that environment, and the verdict says where
// each one ran.
//
// hoom never decides alone where a gate runs: nothing here detects the
// environment. The person writes the file (a profile only suggests it:
// profiles.Sugerencias), hoom reads it (Leer) and applies it (Ajuste.Linea)
// wherever it runs the gates: `hoom verify`, the verify of the envelope of
// `hoom agent`, the pilot's cerrar-hallazgos and POST /api/verify, all of
// them through verifycmd.Run.
//
// # Where it lives
//
// In the main checkout: the parent folder of the git common directory of the
// verified tree (gitx.CheckoutPrincipal), and the file read is always
// <main checkout>/.hoom/local.yaml, also for a project whose hoom.yaml lives
// in a subdirectory of the repository. From a task's tree,
// .hoom/worktrees/<slug>/, the main checkout's file is read; a
// .hoom/local.yaml inside a worktree is never read. Outside a git
// repository, the one of the project root (the directory Leer gets) is read.
//
// # The keys
//
//	ejecutar: "ddev exec -d /var/www/html/{rel} sh -c {cmd}"
//	en_el_host: [semgrep]   # gates que corren como hoy
//
// Claves are the only keys the file accepts. ejecutar is the template; with
// no value, null, "" or only blanks it counts as without the key (the rule
// of CA-747), and then the gates run as today. en_el_host names gates of
// hoom.yaml that run as today even with ejecutar; without ejecutar it
// changes nothing in the commands, the states or the JSON, but the file is
// still validated whole.
//
// # The marks
//
// In ejecutar, hoom replaces each of these marks by one sh word between
// single quotes, so the sh at the other end gets the value intact:
//
//   - {cmd}: the gate's command already built, with {base}, {files} and
//     {packages} resolved;
//   - {dir}: the absolute path of the verified tree;
//   - {rel}: the tree's path relative to the main checkout
//     (filepath.Rel(main checkout, tree)), with "/": "." in the main
//     checkout and ".hoom/worktrees/<slug>" in a task; as it comes out
//     otherwise, even when it starts with "../" (a worktree outside the main
//     checkout) or is the project's subpath (a hoom.yaml in a subdirectory
//     of the repository).
//
// A gate with the template runs `sh -c <ejecutar with the marks replaced>`,
// with the same directory, timeout and gracia as today. A single quote
// inside a value is written as these four characters (Ajuste.Linea):
//
//	'\''
//
// # Errors
//
// Leer refuses the file, and then `hoom verify` runs no gate and writes no
// verdict, as with an invalid hoom.yaml, with exactly:
//
//   - ErrClaveDesconocida: `.hoom/local.yaml: clave desconocida "<k>" (validas: ejecutar, en_el_host)`;
//   - ErrSinCmd: `.hoom/local.yaml: ejecutar tiene que llevar {cmd}`;
//   - ErrGateDesconocido: `.hoom/local.yaml: en_el_host nombra "<g>", que no es un gate de hoom.yaml`;
//   - ErrNoIgnorado: `.hoom/local.yaml: git no lo ignora y es de esta maquina: agrega local.yaml a .hoom/.gitignore (y git rm --cached .hoom/local.yaml si esta en git)`.
//
// When several apply, the first in that order. Outside a git repository
// there is no git to ignore the file, and so no ErrNoIgnorado. Any other
// defect also refuses the file, fail closed, with an error that starts like
// the four above (the path .hoom/local.yaml, a colon and a space), and whose
// text is not fixed: a file that is not regular or cannot be read, YAML
// that does not parse, a top level that is not a map, an ejecutar that is
// not text, an en_el_host that is not a list of names (all of these before
// the four above), and a git that does not answer whether it ignores the
// file (in the place of ErrNoIgnorado).
//
// # The verdict
//
// With ejecutar, the verdict carries verdict.Verdict.Entorno (the template
// and the sha256 of the file) and each gate its verdict.GateResult.Entorno
// (verdict.EntornoPrefijo or verdict.EntornoHost); without it, none of those
// keys appears and the JSON is today's. The fingerprint does not change: it
// is the tree's, and .hoom/local.yaml is ignored.
package entorno
