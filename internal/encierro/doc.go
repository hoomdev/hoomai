// Package encierro runs enclosed what runs code of the writer under an
// envelope of a role that executes, the review or the pilot
// (contencion-del-writer). Enclosed means: it writes only in its task, it
// neither sees nor signals processes outside, it does not read the person's
// known secrets, and nothing of it stays alive once it ends. Where the system
// does not let hoom enclose, the level says so (Info), and the person decides
// with encierro.exigir of hoom.yaml whether the role runs anyway.
//
// # When
//
// Only in a project whose hoom.yaml (the main checkout's, read by
// manifest.LoadRoles) has the section `encierro` (manifest.EncierroPolicy),
// and only for a role with a task (--task) that executes (agents.Role.Exec:
// writer, test-writer, reviewer, characterizer, refutador, and the
// orquestador when an envelope launches it). Whoever runs it decides it and
// hands this package a Plan: agentcmd.Run (the run of the role), reviewcmd.Run
// (each pass of the reviewer), verifycmd.Run with verifycmd.Options.Sobre (the
// gates of the verify of such an envelope and of the pilot's
// cerrar-hallazgos) and itemcmd.SaveTarea (each git of the pilot's save).
// `hoom verify`, POST /api/verify, `hoom item save` and `hoom run` never run
// enclosed. Without the section nothing here applies: no Plan, no `encierro`
// key in any record.
//
// # The level
//
// NivelDe is the contract's table, pure: from the system, the result of the
// probe (Sonda), whether the role has a task, whether its provider declares
// its state (providers.ConEstado) and, on Windows, whether the job was
// created, it gives the level (NivelCompleto, NivelParcial, NivelNinguno),
// the mechanism (MecanismoBwrap, MecanismoJob, or "" with ninguno), what is
// missing (FaltaProcesos, FaltaEscrituras, FaltaIPC; the empty list with
// completo) and the motive (absent with completo). Nivel is NivelDe for this
// system. On Linux the probe (Sondear) runs once per process: bwrap with the
// minimal view and a command that ends, and Landlock asked for its ABI.
// ReemplazarSonda replaces it in the tests, so `parcial` and `ninguno` can be
// tested on a machine that has `completo`.
//
// The envelope's record (envelope.Record), the review's record
// (reviewcmd.Record) and the sidecar of each enclosed run (runcmd.Meta) keep
// the Info under the key `encierro`. With a level under completo the
// envelope's log, or the review's output, carries Aviso; the envelope's log
// carries Linea inside its step run. With a level under exigir (Alcanza
// false) the envelope refuses inside its step run with NotaExigir, without
// launching.
//
// # The view, in Linux
//
// VistaDe builds, from a Lugar, the Vista of one enclosed thing:
//   - read: the whole file system as the person sees it, read-only, with its
//     own /proc and /dev. /tmp is seen too (CLIs, repos and the t.TempDir()
//     of the tests live there), read-only;
//   - known secrets covered, only when they exist: SecretosDirs are seen as an
//     empty directory and SecretosArchivos as an empty file, read-only. The
//     credentials of the CLIs (~/.claude, ~/.codex) are not covered;
//   - sockets that launch processes covered with an empty directory, only
//     when they exist: RunUser() (/run/user/<uid>, replaceable in the tests
//     with ReemplazarRunUser), /run/docker.sock, /run/podman/, and in /tmp
//     tmux-<uid>/, .X11-unix/, .ICE-unix/ and the person's ssh-*;
//   - writable, only: its run tree (the task's tree, or the test-writer's
//     blind tree, which then does not write the rest of the task's tree);
//     the git common directory minus the git material and the admin
//     directories of the other worktrees; its cache, Cache(principal, slug,
//     quien) = .hoom/cache/encierro/<slug>/<quien>/ in the main checkout,
//     which hoom creates when missing and nobody else writes; what the
//     provider declares (providers.Estado.Escribe); and encierro.escribibles;
//   - read-only inside its run tree: the git material, the agents' config
//     (`.claude/`, `.codex/`, `.mcp.json` and providers.Estado.EnArbol) and the
//     tree's `.git` file. What does not exist when it starts is not mounted:
//     creating it is not blocked, the gate detects it.
//
// # The environment
//
// Entorno: today's, without VariablesQuitadas, with TMPDIR, XDG_CACHE_HOME and
// the profile's cache variables (profiles.Cache: GOCACHE, GOMODCACHE and
// GOTMPDIR in the go profile) inside its cache, and with ConfigGit in git's
// environment config (GIT_CONFIG_COUNT): a `git commit` inside leaves no
// maintenance behind.
//
// # Processes
//
// Envolver prepares a command to run in its own PID, user and mount
// namespaces, armed with bubblewrap: inside, nothing of outside is seen or
// signalled (not hoom, not the Studio, not the person's terminal); it ends
// with its main process, and the kernel kills whatever was left inside, after
// setsid or a double fork; it dies with the hoom that launched it, also by
// SIGKILL (--die-with-parent); it runs in a new session without a
// controlling terminal; with no_new_privs; with nested user namespaces
// allowed (the gate `test` of hoomai and each CLI's own sandbox need them).
// Arrancar keeps today's gracia, which bwrap does not give by itself: when a
// deadline passes or hoom cancels, SIGTERM goes to the group of the main
// process inside (with the outside pid bwrap reports with --info-fd), today's
// gracia passes (graciaGit in the save, graciaGate in a gate), and only then
// the encierro ends. Liberar is the end once the command ended. Time bounds
// do not change.
//
// # The step
//
// A step of hoom inside the encierro, before the command, makes the command
// the leader of a session of its own, without a controlling terminal, and,
// with Landlock ABI 6 or more (level completo), forbids it to connect to an
// abstract socket outside its domain (X11's way to the graphical session).
// The step is the same binary
// (os.Executable), which is seen inside because the root is mounted: hoom
// itself, or, in the tests, the test binary. Paso is that step: when the
// process is the step it applies Landlock and executes the command, and never
// returns; otherwise it returns at once. This package calls it from its own
// init, so every binary that links it (hoom, and the test binary of any
// package whose code reaches it) already does the step, and an existing
// TestMain needs no change. main and a TestMain may also call it first
// thing, explicitly:
//
//	func TestMain(m *testing.M) {
//		encierro.Paso() // the step of the encierro: does not return when it is
//		os.Exit(m.Run())
//	}
//
// # Other systems
//
// Windows: procesos.Adoptar's Job Object already contains a child and kills
// it when it closes; the level is parcial with mechanism job when the job
// was created (procesos.EnJob), ninguno when not. Writes are not enclosed.
// macOS: nothing contains processes; the level is ninguno. Envolver leaves
// the command as it is outside Linux. The files of this package that need
// Linux carry a build tag, and the other systems get their counterpart.
package encierro
