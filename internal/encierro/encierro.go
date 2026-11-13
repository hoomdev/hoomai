package encierro

import (
	"os/exec"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
)

// The levels of the encierro (Info.Nivel).
const (
	NivelCompleto = "completo"
	NivelParcial  = "parcial"
	NivelNinguno  = "ninguno"
)

// The mechanisms of the encierro (Info.Mecanismo). A level ninguno has none
// ("").
const (
	MecanismoBwrap = "bwrap"
	MecanismoJob   = "job"
)

// What an encierro may lack (Info.Falta), always in this order.
const (
	FaltaProcesos   = "procesos"
	FaltaEscrituras = "escrituras"
	FaltaIPC        = "ipc"
)

// The motives the contract fixes (Info.Motivo). The others are
// MotivoLandlock, MotivoSinEstado and, for a bwrap that cannot create the user
// namespace, what bwrap said (Sonda.Salida).
const (
	// MotivoSinBwrap: Linux without bwrap in PATH.
	MotivoSinBwrap = "bwrap no esta"
	// MotivoSinTarea: a role that executes without --task, which runs in the
	// main checkout, in any system.
	MotivoSinTarea = "sin tarea"
)

// The motives of macOS and Windows. The contract fixes no text for them; these
// are the ones hoom writes.
const (
	// MotivoSinContencion: a system with no way to contain processes (macOS,
	// and any system that is neither Linux nor Windows).
	MotivoSinContencion = "este sistema no puede contener procesos"
	// MotivoSinJob: Windows did not let Adoptar create or assign the job.
	MotivoSinJob = "windows no dejo crear el job"
	// MotivoSinEscrituras: Windows with the job, which contains processes
	// and not writes.
	MotivoSinEscrituras = "windows no encierra escrituras"
)

// MotivoLandlock is the motive of a Linux with bwrap and user namespaces whose
// Landlock ABI is under 6 (0 = no Landlock): `landlock ABI <n>`.
func MotivoLandlock(abi int) string {
	return ""
}

// MotivoSinEstado is the motive of a role whose provider does not implement
// providers.ConEstado: `<provider> no declara su estado`.
func MotivoSinEstado(provider string) string {
	return ""
}

// Info is the level of one encierro as the records keep it under the key
// `encierro` (envelope.Record, reviewcmd.Record, runcmd.Meta):
// `{"nivel", "mecanismo", "falta", "motivo"}`. With completo, Falta is the
// empty list (`"falta": []`, never null) and Motivo is absent; with ninguno,
// Mecanismo is "".
type Info struct {
	Nivel     string   `json:"nivel"`
	Mecanismo string   `json:"mecanismo"`
	Falta     []string `json:"falta"`
	Motivo    string   `json:"motivo,omitempty"`
}

// Sonda is what the probe of a Linux found (Sondear).
type Sonda struct {
	// Bwrap: bwrap is in PATH.
	Bwrap bool
	// Espacio: bwrap created its user namespace with the minimal view and ran
	// a command that ends.
	Espacio bool
	// Salida is what bwrap said when it could not (Espacio false with Bwrap
	// true): its stderr, trimmed, or its exit status when it said nothing
	// (for example AppArmor's refusal in Ubuntu 24.04).
	Salida string
	// LandlockABI is the ABI Landlock answers; 0 = no Landlock.
	LandlockABI int
}

// Sondear is the probe of this Linux: it runs bwrap with the minimal view
// and a command that ends, and asks Landlock its ABI. It runs once per
// process; the next calls give the same Sonda. Outside Linux it is the zero
// Sonda. ReemplazarSonda replaces it.
func Sondear() Sonda {
	return Sonda{}
}

// ReemplazarSonda makes Sondear (and so Nivel) answer s in this process
// until restaurar is called, which gives back what was there before. It is
// for the tests: `parcial` and `ninguno` on a machine that has `completo`.
// On Linux, an enclosed launch with a level from it uses bwrap as that level
// says (with completo, with the step of Landlock; with parcial, without it;
// with ninguno, not at all).
func ReemplazarSonda(s Sonda) (restaurar func()) {
	return func() {}
}

// Caso is everything NivelDe decides from.
type Caso struct {
	// Sistema is the system, as runtime.GOOS names it.
	Sistema string
	// Sonda is the result of the probe; only Linux reads it.
	Sonda Sonda
	// Tarea: the role runs with --task.
	Tarea bool
	// Provider is the name of the role's provider, for MotivoSinEstado; ""
	// for what has no provider (the gates of verify, the git of the save).
	Provider string
	// DeclaraEstado: the provider implements providers.ConEstado. Ignored
	// with Provider "".
	DeclaraEstado bool
	// Job: on Windows, procesos.Adoptar created the job. Only Windows reads
	// it.
	Job bool
}

// NivelDe is the contract's table of levels, pure. The first that holds:
//
//   - without Tarea, in any system: ninguno, motive MotivoSinTarea;
//   - a Provider that does not declare its state: ninguno, motive
//     MotivoSinEstado(Provider);
//   - Linux without bwrap: ninguno, MotivoSinBwrap;
//   - Linux with bwrap that cannot create the user namespace: ninguno, motive
//     Sonda.Salida;
//   - Linux with bwrap and user namespaces, Landlock ABI under 6: parcial,
//     bwrap, missing ipc, motive MotivoLandlock(abi);
//   - Linux with bwrap, user namespaces and Landlock ABI 6 or more:
//     completo, bwrap, missing nothing (the empty list), no motive;
//   - Windows with the job: parcial, job, missing escrituras and ipc, motive
//     MotivoSinEscrituras;
//   - Windows without the job: ninguno, MotivoSinJob;
//   - macOS, and any other system: ninguno, MotivoSinContencion.
//
// Every ninguno has mechanism "" and misses procesos, escrituras and ipc.
func NivelDe(c Caso) Info {
	return Info{}
}

// Nivel is NivelDe for this system: runtime.GOOS, Sondear() and tarea, with
// prov (nil = no provider: the gates of verify, the git of the save) and
// whether it implements providers.ConEstado. On Windows it assumes the job is
// created (Job true): the run knows at its start, with procesos.EnJob, whether
// procesos.Adoptar created it, and its sidecar keeps the level it really had.
func Nivel(tarea bool, prov providers.Provider) Info {
	return Info{}
}

// Alcanza says whether the level nivel meets exigir (manifest.ExigirCompleto,
// manifest.ExigirParcial, manifest.ExigirNo): any level meets ExigirNo (and
// ""), ExigirParcial needs parcial or completo, ExigirCompleto needs
// completo.
func Alcanza(nivel, exigir string) bool {
	return false
}

// Linea is the line of the envelope's log, inside its step run, that says
// its encierro: `encierro <nivel> con <mecanismo>` (completo or parcial), or
// `sin encierro: <motivo>` (ninguno).
func Linea(i Info) string {
	return ""
}

// Aviso is the warning of a level under completo, for the envelope's log or
// the review's output:
// `aviso: encierro <nivel> (falta: <lista>): lo que el <rol> deje fuera de su arbol no se ve`,
// <lista> being Falta joined with ", ". "" with completo.
func Aviso(i Info, rol string) string {
	return ""
}

// NotaExigir is the note of an envelope (and the review) that refuses
// because the level is under encierro.exigir:
// `el encierro de este sistema es <nivel> (falta: <lista>) y hoom.yaml exige <exigir>: el <rol> no corre`,
// <lista> being Falta joined with ", ".
func NotaExigir(i Info, exigir, rol string) string {
	return ""
}

// Validar applies the two rules of the policy that need the tree and the
// repository, when the envelope or the review starts (CA-675): principal is
// the main checkout, comunGit the git common directory, and gates the names
// of the gates of the tree's hoom.yaml already merged with its profile (the
// ones verify runs, the keys of manifest.Load's Gates). It returns the first
// that breaks, the writable paths first, in their order, then the gates:
//
//   - `encierro: escribibles: <ruta> contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible`,
//     <ruta> as hoom.yaml writes it ("~/" is the $HOME of this process);
//   - `encierro: gates_sin_encierro: el gate <g> no esta en los gates de este arbol`.
//
// nil when neither breaks. It runs no git.
func Validar(pol manifest.EncierroPolicy, principal, comunGit string, gates []string) error {
	return nil
}

// DirCache is where the caches of the encierro live, relative to the main
// checkout: `.hoom/cache/encierro`.
const DirCache = ".hoom/cache/encierro"

// The owners of a cache that are not a role (Lugar.Quien): the gates of the
// verify of an envelope (and of cerrar-hallazgos), and the git of the
// pilot's save.
const (
	QuienVerify   = "verify"
	QuienGuardado = "guardado"
)

// Cache is the cache of quien (a role's slug, QuienVerify or QuienGuardado)
// for the task slug, in the main checkout principal:
// `<principal>/.hoom/cache/encierro/<slug>/<quien>`. Nobody writes the cache of
// another: the one the writer's run fills is not the one its verify, the
// test-writer or the review use.
func Cache(principal, slug, quien string) string {
	return ""
}

// Lugar is where one enclosed thing runs, from which VistaDe builds its
// view.
type Lugar struct {
	// Principal is the project's main checkout.
	Principal string
	// Tarea is the task's slug.
	Tarea string
	// Arbol is its run tree, writable: the task's tree
	// (.hoom/worktrees/<slug>), the test-writer's blind tree inside it, or
	// the tree the gates of verify and the git of the save run in.
	Arbol string
	// Quien owns its cache: the role's slug, QuienVerify or QuienGuardado.
	Quien string
	// Estado is what its provider declares; its Escribe is writable. The
	// zero Estado for what has no provider.
	Estado providers.Estado
	// Escribibles is encierro.escribibles of the project's hoom.yaml, as
	// written (absolute or "~/").
	Escribibles []string
	// Perfil is the project's profile, whose cache variables it gets
	// (profiles.Cache).
	Perfil string
}

// Vista is what one enclosed thing may write, with absolute paths. Read is
// always the whole file system, read-only, with the known secrets and the
// sockets covered (see the package's doc): Envolver adds those.
type Vista struct {
	// Dir is the working directory of what runs.
	Dir string
	// Escribibles are writable inside (besides Cache). One that does not
	// exist is not mounted.
	Escribibles []string
	// SoloLectura stay read-only even when they are inside an Escribible:
	// the git material that exists, the agents' config in the run tree, the
	// tree's `.git` file, the admin directories of the other worktrees, and
	// for the test-writer the task's tree outside its blind tree. One that
	// does not exist is not mounted: creating it is not blocked.
	SoloLectura []string
	// Cache is its cache: writable, created when missing; TMPDIR,
	// XDG_CACHE_HOME and VarsCache point inside it.
	Cache string
	// VarsCache are the names of the profile's cache variables.
	VarsCache []string
}

// VistaDe builds the Vista of l: Arbol as Dir; writable Arbol, the git
// common directory of Arbol (minus the git material and the admin
// directories of the other worktrees, which go to SoloLectura), each path of
// l.Estado.Escribe and of l.Escribibles ("~/" expanded with this process's
// $HOME); read-only the git material that exists, the agents' config in
// Arbol (`.claude/`, `.codex/`, `.mcp.json` and l.Estado.EnArbol) and Arbol's
// `.git` file; Cache(l.Principal, l.Tarea, l.Quien) as Cache; and
// profiles.Cache(l.Perfil) as VarsCache. An error when git does not say where
// Arbol's repository is, or the profile is unknown.
func VistaDe(l Lugar) (Vista, error) {
	return Vista{}, nil
}

// Plan is one enclosed launch, decided before it starts: its level and, with
// mechanism bwrap, its view. A nil *Plan wherever one is taken (for example
// runcmd.StartOptions.Encierro) is "no encierro": as today, and no
// `encierro` key in the records.
type Plan struct {
	Info  Info
	Vista Vista
}

// VariablesQuitadas are the variables Entorno takes out of the environment:
// they would point at what the encierro covers.
var VariablesQuitadas = []string{"SSH_AUTH_SOCK", "GPG_AGENT_INFO", "DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR"}

// ConfigGit are the key and value pairs Entorno adds to git's environment
// config (GIT_CONFIG_COUNT), as the pilot's save already passes them: no
// automatic gc, no maintenance (git 2.47 launches it with --detach after a
// commit, and it would die half done when the encierro ends) and no
// fsmonitor.
var ConfigGit = [][2]string{{"gc.auto", "0"}, {"maintenance.auto", "false"}, {"core.fsmonitor", "false"}}

// SecretosDirs are the known secrets that are directories ("~/" is the $HOME
// of the enclosed process): inside, when they exist, each one is an empty
// directory, read-only.
var SecretosDirs = []string{"~/.ssh", "~/.gnupg", "~/.config/gh", "~/.aws"}

// SecretosArchivos are the known secrets that are files: inside, when they
// exist, each one is an empty file, read-only.
var SecretosArchivos = []string{"~/.docker/config.json"}

// Entorno is the environment of what runs under v, from base (the one it
// would have today): without VariablesQuitadas; with TMPDIR, XDG_CACHE_HOME
// and each of v.VarsCache set to a directory of its own inside v.Cache
// (filepath.Join(v.Cache, <name>)); and with ConfigGit added to git's
// environment config after the entries base already carries
// (GIT_CONFIG_COUNT, GIT_CONFIG_KEY_<n>, GIT_CONFIG_VALUE_<n>). A variable it
// sets replaces the one base had. Pure: it creates nothing (Envolver
// creates the directories).
func Entorno(base []string, v Vista) []string {
	return nil
}

// RunUser is the directory the encierro covers as /run/user/<uid> of this
// process (the systemd user bus, the graphical session, the ssh and gpg
// agents): `/run/user/<uid>`, or the one ReemplazarRunUser set.
func RunUser() string {
	return ""
}

// ReemplazarRunUser makes RunUser answer dir in this process until
// restaurar is called. It is for the tests: a runner without a session has no
// /run/user/<uid>, so the test creates a directory that plays it and checks
// that inside it is seen empty.
func ReemplazarRunUser(dir string) (restaurar func()) {
	return func() {}
}

// Envolver prepares cmd, not started yet, to run under p. With
// p.Info.Mecanismo MecanismoBwrap, cmd ends up running bwrap with p.Vista
// around its original command (Path, Args and Dir), in its own PID, user and
// mount namespaces, a new session, no_new_privs, --die-with-parent and
// --info-fd, and with level completo the step of Landlock (Paso, through
// os.Executable) before the command; its environment is
// Entorno(cmd.Environ(), p.Vista); the known secrets and the sockets are
// covered; it creates p.Vista.Cache and the directories Entorno points at.
// With any other mechanism (MecanismoJob, or "" with ninguno) it leaves cmd
// as it is. An error (bwrap not in PATH, the cache cannot be created) leaves
// cmd unchanged. Start cmd with Arrancar.
func Envolver(cmd *exec.Cmd, p Plan) error {
	return nil
}

// Arrancar starts cmd in a process group of its own, registered for hoom's
// signal handler, like procesos.ArrancarConGracia (gracia 0: like
// procesos.Arrancar). For a cmd Envolver prepared with bwrap it keeps
// today's gracia: when cmd's context ends (its deadline, a cancel), and on a
// signal to hoom, SIGTERM goes to the group of the main process inside the
// encierro (the outside pid bwrap reports with --info-fd), and only after
// gracia does the whole encierro end (SIGKILL to its pid 1), with everything
// inside it. Whoever calls it calls Liberar once cmd's Wait returned.
func Arrancar(cmd *exec.Cmd, gracia time.Duration) error {
	return nil
}

// Liberar is the end of cmd started with Arrancar, once its Wait returned:
// after the rest of its gracia when its context ended, whatever is left in
// its group, or inside its encierro, dies, and the group is forgotten (like
// procesos.EsperarGrupo and procesos.Liberar).
func Liberar(cmd *exec.Cmd) {}

// Paso is the step of hoom inside the encierro, before the command: when this
// process is that step (Envolver launched it, through os.Executable, inside
// bwrap) it forbids itself, with Landlock, to connect to an abstract socket
// outside its domain and executes the command in its place: it never
// returns. Otherwise it returns at once. This package's init calls it, so any
// binary that links the package does the step; calling it again, first thing
// in main or in a TestMain, is harmless (see the package's doc).
func Paso() {
	paso()
}

func init() { Paso() }
