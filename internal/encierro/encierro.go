package encierro

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/materialgit"
	"github.com/hoomdev/hoomai/internal/procesos"
	"github.com/hoomdev/hoomai/internal/profiles"
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
	return "landlock ABI " + strconv.Itoa(abi)
}

// MotivoSinEstado is the motive of a role whose provider does not implement
// providers.ConEstado: `<provider> no declara su estado`.
func MotivoSinEstado(provider string) string {
	return provider + " no declara su estado"
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
	sonda.Lock()
	defer sonda.Unlock()
	if sonda.reemplazo != nil {
		return *sonda.reemplazo
	}
	if !sonda.hecha {
		sonda.valor = sondear()
		sonda.hecha = true
	}
	return sonda.valor
}

// sonda is the probe of this process: done once, or replaced.
var sonda struct {
	sync.Mutex
	hecha     bool
	valor     Sonda
	reemplazo *Sonda
}

// ReemplazarSonda makes Sondear (and so Nivel) answer s in this process
// until restaurar is called, which gives back what was there before. It is
// for the tests: `parcial` and `ninguno` on a machine that has `completo`.
// On Linux, an enclosed launch with a level from it uses bwrap as that level
// says (with completo, with the step of Landlock; with parcial, without it;
// with ninguno, not at all).
func ReemplazarSonda(s Sonda) (restaurar func()) {
	sonda.Lock()
	antes := sonda.reemplazo
	sonda.reemplazo = &s
	sonda.Unlock()
	return func() {
		sonda.Lock()
		sonda.reemplazo = antes
		sonda.Unlock()
	}
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
	ninguno := func(motivo string) Info {
		return Info{Nivel: NivelNinguno, Falta: []string{FaltaProcesos, FaltaEscrituras, FaltaIPC}, Motivo: motivo}
	}
	switch {
	case !c.Tarea:
		return ninguno(MotivoSinTarea)
	case c.Provider != "" && !c.DeclaraEstado:
		return ninguno(MotivoSinEstado(c.Provider))
	}
	switch c.Sistema {
	case "linux":
		switch {
		case !c.Sonda.Bwrap:
			return ninguno(MotivoSinBwrap)
		case !c.Sonda.Espacio:
			motivo := strings.TrimSpace(c.Sonda.Salida)
			if motivo == "" {
				motivo = "bwrap no pudo crear el espacio de usuario"
			}
			return ninguno(motivo)
		case c.Sonda.LandlockABI < 6:
			return Info{Nivel: NivelParcial, Mecanismo: MecanismoBwrap, Falta: []string{FaltaIPC}, Motivo: MotivoLandlock(c.Sonda.LandlockABI)}
		}
		return Info{Nivel: NivelCompleto, Mecanismo: MecanismoBwrap, Falta: []string{}}
	case "windows":
		if !c.Job {
			return ninguno(MotivoSinJob)
		}
		return Info{Nivel: NivelParcial, Mecanismo: MecanismoJob, Falta: []string{FaltaEscrituras, FaltaIPC}, Motivo: MotivoSinEscrituras}
	}
	return ninguno(MotivoSinContencion)
}

// Nivel is NivelDe for this system: runtime.GOOS, Sondear() and tarea, with
// prov (nil = no provider: the gates of verify, the git of the save) and
// whether it implements providers.ConEstado. On Windows it assumes the job is
// created (Job true): the run knows at its start, with procesos.EnJob, whether
// procesos.Adoptar created it, and its sidecar keeps the level it really had.
func Nivel(tarea bool, prov providers.Provider) Info {
	c := Caso{Sistema: runtime.GOOS, Tarea: tarea, Job: true}
	if prov != nil {
		c.Provider = prov.Name()
		_, c.DeclaraEstado = prov.(providers.ConEstado)
	}
	if c.Sistema == "linux" && c.Tarea && (c.Provider == "" || c.DeclaraEstado) {
		c.Sonda = Sondear() // only when it decides: a role without task never probes
	}
	return NivelDe(c)
}

// Alcanza says whether the level nivel meets exigir (manifest.ExigirCompleto,
// manifest.ExigirParcial, manifest.ExigirNo): any level meets ExigirNo (and
// ""), ExigirParcial needs parcial or completo, ExigirCompleto needs
// completo.
func Alcanza(nivel, exigir string) bool {
	switch exigir {
	case manifest.ExigirCompleto:
		return nivel == NivelCompleto
	case manifest.ExigirParcial:
		return nivel == NivelCompleto || nivel == NivelParcial
	}
	return true
}

// Linea is the line of the envelope's log, inside its step run, that says
// its encierro: `encierro <nivel> con <mecanismo>` (completo or parcial), or
// `sin encierro: <motivo>` (ninguno).
func Linea(i Info) string {
	if i.Nivel == NivelNinguno || i.Mecanismo == "" {
		return "sin encierro: " + i.Motivo
	}
	return "encierro " + i.Nivel + " con " + i.Mecanismo
}

// Aviso is the warning of a level under completo, for the envelope's log or
// the review's output:
// `aviso: encierro <nivel> (falta: <lista>): lo que el <rol> deje fuera de su arbol no se ve`,
// <lista> being Falta joined with ", ". "" with completo.
func Aviso(i Info, rol string) string {
	if i.Nivel == NivelCompleto {
		return ""
	}
	return "aviso: encierro " + i.Nivel + " (falta: " + strings.Join(i.Falta, ", ") + "): lo que el " + rol + " deje fuera de su arbol no se ve"
}

// NotaExigir is the note of an envelope (and the review) that refuses
// because the level is under encierro.exigir:
// `el encierro de este sistema es <nivel> (falta: <lista>) y hoom.yaml exige <exigir>: el <rol> no corre`,
// <lista> being Falta joined with ", ".
func NotaExigir(i Info, exigir, rol string) string {
	return "el encierro de este sistema es " + i.Nivel + " (falta: " + strings.Join(i.Falta, ", ") + ") y hoom.yaml exige " + exigir + ": el " + rol + " no corre"
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
	home, _ := os.UserHomeDir()
	for _, r := range pol.Escribibles {
		p := expandir(r, home)
		for _, d := range []string{principal, comunGit} {
			if d == "" || p == "" {
				continue
			}
			if cruza(p, d) {
				return fmt.Errorf("encierro: escribibles: %s contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible", r)
			}
		}
	}
	hay := map[string]bool{}
	for _, g := range gates {
		hay[g] = true
	}
	for _, g := range pol.GatesSinEncierro {
		if !hay[g] {
			return fmt.Errorf("encierro: gates_sin_encierro: el gate %s no esta en los gates de este arbol", g)
		}
	}
	return nil
}

// cruza says whether p contains d or is inside it, lexically and also once
// their symlinks are resolved (the one of the two that exists).
func cruza(p, d string) bool {
	pares := [][2]string{{filepath.Clean(p), filepath.Clean(d)}}
	rp, errp := filepath.EvalSymlinks(p)
	rd, errd := filepath.EvalSymlinks(d)
	if errp == nil || errd == nil {
		if errp != nil {
			rp = filepath.Clean(p)
		}
		if errd != nil {
			rd = filepath.Clean(d)
		}
		pares = append(pares, [2]string{rp, rd})
	}
	for _, par := range pares {
		if dentroDe(par[0], par[1]) || dentroDe(par[1], par[0]) {
			return true
		}
	}
	return false
}

// dentroDe says whether p is d or inside it, lexically.
func dentroDe(p, d string) bool {
	rel, err := filepath.Rel(d, p)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// expandir makes a path of hoom.yaml or of providers.Estado absolute: "~/"
// is home; "" for a relative one, or "~/" without a home.
func expandir(p, home string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if home == "" {
			return ""
		}
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	if !filepath.IsAbs(p) {
		return ""
	}
	return filepath.Clean(p)
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
	return filepath.Join(principal, filepath.FromSlash(DirCache), slug, quien)
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
	// the tree the gates of verify and the git of the save run in. "", or a
	// tree that does not exist, is no writable tree at all (the gates of the
	// test-writer's verify once its blind tree is gone): then only Cache,
	// Estado.Escribe and Escribibles are writable.
	Arbol string
	// Dir (ronda 1, a8ef36) is where what runs starts, when it is not Arbol:
	// the gates of the test-writer's verify run in the task's tree, which
	// stays read-only, with Arbol its blind tree. "" = Arbol.
	Dir string
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
	// SoloLectura stay read-only even when they are inside an Escribible
	// (and an Escribible deeper inside one of them is writable again: Envolver
	// mounts both lists by depth, the shallower first):
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

// VistaDe builds the Vista of l: l.Dir (or Arbol) as Dir; writable Arbol, the git
// common directory of Arbol (minus the git material and the admin
// directories of the other worktrees, which go to SoloLectura; when Arbol is
// a linked worktree, minus too the main checkout's administration that
// exists, AdministracionPrincipal, and the whole `worktrees/` but for
// Arbol's own administration, which is writable), each path of
// l.Estado.Escribe and of l.Escribibles ("~/" expanded with this process's
// $HOME); read-only the git material that exists, the agents' config in
// Arbol (`.claude/`, `.codex/`, `.mcp.json` and l.Estado.EnArbol) and Arbol's
// `.git` file; Cache(l.Principal, l.Tarea, l.Quien) as Cache; and
// profiles.Cache(l.Perfil) as VarsCache. With l.Dir and no Arbol (or one
// that is not there), nothing of a tree is writable: the cache, Escribe and
// Escribibles only. An error when git does not say where Arbol's repository
// is, or the profile is unknown.
func VistaDe(l Lugar) (Vista, error) {
	limite := time.Now().Add(topeVista)
	// without a writable tree (the gates of the test-writer's verify once
	// its blind tree is gone: Dir set and Arbol "" or not there), only the
	// cache, Escribe and Escribibles are writable (ronda 1, a8ef36)
	conArbol := l.Arbol != "" && (l.Dir == "" || esDirectorio(l.Arbol))
	var lg materialgit.Lugar
	if conArbol {
		var err error
		if lg, err = materialgit.Ubicar(l.Arbol, limite); err != nil {
			return Vista{}, fmt.Errorf("encierro: no se sabe donde esta el repositorio de %s: %w", l.Arbol, err)
		}
	} else if l.Dir == "" {
		return Vista{}, fmt.Errorf("encierro: la vista no tiene arbol")
	}
	var vars []string
	if strings.TrimSpace(l.Perfil) != "" {
		var err error
		if vars, err = profiles.Cache(l.Perfil); err != nil {
			return Vista{}, err
		}
	}
	home, _ := os.UserHomeDir()
	dir := l.Dir
	if dir == "" {
		dir = l.Arbol
	}
	v := Vista{Dir: dir, Cache: Cache(l.Principal, l.Tarea, l.Quien), VarsCache: vars}
	visto := map[string]bool{}
	escribible := func(p string) {
		if p != "" && !visto[p] {
			visto[p] = true
			v.Escribibles = append(v.Escribibles, p)
		}
	}
	if conArbol {
		escribible(filepath.Clean(l.Arbol))
		escribible(filepath.Clean(lg.Comun))
	}
	// whoever builds the view already refused this (ValidarEscribe, at the
	// start of the envelope or the review); never mounted all the same
	if err := ValidarEscribe("el provider", l.Estado.Escribe, l.Principal, lg.Comun); err != nil {
		return Vista{}, err
	}
	for _, p := range l.Estado.Escribe {
		escribible(expandir(p, home))
	}
	for _, p := range l.Escribibles {
		escribible(expandir(p, home))
	}
	if !conArbol {
		return v, nil
	}
	soloVisto := map[string]bool{}
	soloLectura := func(p string) {
		p = filepath.Clean(p)
		if soloVisto[p] {
			return
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return // what is not there is not mounted: creating it is detected
		}
		if !montable(p, fi) {
			return
		}
		soloVisto[p] = true
		v.SoloLectura = append(v.SoloLectura, p)
	}
	rutas, _ := lg.Rutas(limite) // a failing `git config --list` still leaves the rest
	for _, p := range rutas {
		soloLectura(p)
	}
	enArbol := append([]string{".claude/", ".codex/", ".mcp.json"}, l.Estado.EnArbol...)
	for _, rel := range enArbol {
		rel = strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(rel), "./"), "/")
		if rel != "" {
			soloLectura(filepath.Join(l.Arbol, filepath.FromSlash(rel)))
		}
	}
	comun, propio := filepath.Clean(lg.Comun), filepath.Clean(lg.GitDir)
	worktrees := filepath.Join(comun, "worktrees")
	if propio != comun && dentroDe(propio, worktrees) {
		// a linked worktree (ronda 1, fab30f): the main checkout's own
		// administration and the whole worktrees/ are read-only, so nothing
		// enclosed moves or blocks the main checkout nor creates another
		// worktree's administration; its own administration is writable
		// again (Envolver mounts the deeper path later)
		for _, rel := range AdministracionPrincipal {
			soloLectura(filepath.Join(comun, filepath.FromSlash(rel)))
		}
		soloLectura(worktrees)
		escribible(propio)
	}
	// each other worktree's administration, by name too
	if es, err := os.ReadDir(worktrees); err == nil {
		for _, e := range es {
			if p := filepath.Join(worktrees, e.Name()); filepath.Clean(p) != propio {
				soloLectura(p)
			}
		}
	}
	return v, nil
}

// esDirectorio says whether p is a directory (followed).
func esDirectorio(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// montable says whether the path p (fi, not followed) can be re-bound
// read-only: a file, a directory or a symlink outside /dev and /proc. The
// encierro gives /dev and /proc fresh (bwrap's --dev and --proc), so a device
// the git material names, such as GIT_CONFIG_GLOBAL=/dev/null, is not
// re-bound: read-only, /dev/null would refuse git its writes.
func montable(p string, fi fs.FileInfo) bool {
	for _, d := range []string{"/dev", "/proc"} {
		if p == d || strings.HasPrefix(p, d+"/") {
			return false
		}
	}
	return fi.Mode()&(fs.ModeDevice|fs.ModeCharDevice|fs.ModeNamedPipe|fs.ModeSocket|fs.ModeIrregular) == 0
}

// topeVista bounds the git VistaDe runs.
const topeVista = 10 * time.Second

// Plan is one enclosed launch, decided before it starts: its level and, with
// mechanism bwrap, its view. A nil *Plan wherever one is taken (for example
// runcmd.StartOptions.Encierro) is "no encierro": as today, and no
// `encierro` key in the records.
type Plan struct {
	Info  Info
	Vista Vista
	// Exigir (ronda 1, decision 22) is encierro.exigir of the project's
	// hoom.yaml the launch runs under (manifest.ExigirCompleto,
	// ExigirParcial or ExigirNo; "" = ExigirNo): Comprobar reads it once the
	// process started.
	Exigir string
}

// AdministracionPrincipal (ronda 1, fab30f) are the entries of the main
// checkout's administration at the root of the git common directory, by
// their path relative to it: when the run tree is a linked worktree, VistaDe
// leaves read-only the ones that exist (and the whole `worktrees/`, but for
// the tree's own administration), so nothing enclosed moves or blocks the
// main checkout. refs/, objects/, the other logs/ and packed-refs stay
// writable: the writer still commits on its branch.
var AdministracionPrincipal = []string{
	"HEAD", "index", "ORIG_HEAD", "FETCH_HEAD", "AUTO_MERGE",
	"MERGE_HEAD", "MERGE_MSG", "MERGE_MODE", "MERGE_RR",
	"CHERRY_PICK_HEAD", "REVERT_HEAD", "REBASE_HEAD", "SQUASH_MSG", "COMMIT_EDITMSG", "TAG_EDITMSG",
	"BISECT_LOG", "BISECT_START", "BISECT_TERMS", "BISECT_NAMES", "BISECT_EXPECTED_REV", "BISECT_ANCESTORS_OK",
	"logs/HEAD", "rebase-merge", "rebase-apply", "sequencer", "info/sparse-checkout",
}

// TrasAdoptar (ronda 1, decision 22) is the decision once a process of an
// enclosed launch started on sistema (as runtime.GOOS names it) and
// procesos.Adoptar did (enJob) or did not put it in a Job Object, under
// exigir (encierro.exigir): the level the launch really has and whether its
// process tree dies at once and the launch closes as a refusal of exigir.
// On Windows the level is NivelDe's with Job enJob, and it kills when there
// is no job and exigir is not ExigirNo (nor ""). On any other system it
// returns the zero Info and false: there is nothing to correct. Pure.
func TrasAdoptar(sistema string, enJob bool, exigir string) (Info, bool) {
	if sistema != "windows" {
		return Info{}, false
	}
	info := NivelDe(Caso{Sistema: sistema, Tarea: true, Job: enJob})
	return info, !enJob && exigir != "" && exigir != manifest.ExigirNo
}

// SinJob (ronda 1, decision 22) is Comprobar's error when Windows did not
// let Adoptar create the job and exigir is not ExigirNo: Info is the real
// level (ninguno, MotivoSinJob). Its text:
// `el encierro de este sistema es <nivel> (falta: <lista>) y hoom.yaml exige <exigir>: <motivo>`.
// The envelope closes in `run` with NotaExigir, the review's pass with its
// refusal (NotaExigir, role reviewer), the gate in error with this text, and
// the save with itemcmd.ErrExigirDelGuardado of Info.Nivel.
type SinJob struct {
	Info   Info
	Exigir string
}

func (e *SinJob) Error() string {
	return "el encierro de este sistema es " + e.Info.Nivel + " (falta: " + strings.Join(e.Info.Falta, ", ") +
		") y hoom.yaml exige " + e.Exigir + ": " + e.Info.Motivo
}

// Comprobar (ronda 1, decision 22) looks at cmd, started with Arrancar under
// p, right after it started: it returns the level the launch really has
// (p.Info, or on Windows without the job, ninguno with MotivoSinJob) and,
// when TrasAdoptar says so, kills cmd's process tree at once and returns a
// *SinJob. Whoever calls it still waits for cmd and calls Liberar.
func Comprobar(cmd *exec.Cmd, p Plan) (Info, error) {
	if cmd == nil || cmd.Process == nil {
		return p.Info, nil
	}
	adopcion.Lock()
	sistema, enJob := runtime.GOOS, procesos.EnJob
	if adopcion.enJob != nil {
		sistema, enJob = adopcion.sistema, adopcion.enJob
	}
	adopcion.Unlock()
	info, matar := TrasAdoptar(sistema, enJob(cmd.Process.Pid), p.Exigir)
	if info.Nivel == "" {
		return p.Info, nil // not Windows: nothing to correct
	}
	if matar {
		// no job and the project demands one: its tree dies at once, and
		// whoever launched it closes as a refusal of exigir (decision 22)
		_ = procesos.MatarGrupo(cmd.Process.Pid)
		return info, &SinJob{Info: info, Exigir: p.Exigir}
	}
	return info, nil
}

// adopcion is ReemplazarAdopcion's replacement of the system and of
// procesos.EnJob for Comprobar; enJob nil = the real ones.
var adopcion struct {
	sync.Mutex
	sistema string
	enJob   func(pid int) bool
}

// ReemplazarAdopcion makes Comprobar take sistema as the system and enJob
// as procesos.EnJob until restaurar: the seam that lets the decision of
// Windows without the job be tried on Linux.
func ReemplazarAdopcion(sistema string, enJob func(pid int) bool) (restaurar func()) {
	adopcion.Lock()
	antesS, antesF := adopcion.sistema, adopcion.enJob
	adopcion.sistema, adopcion.enJob = sistema, enJob
	adopcion.Unlock()
	return func() {
		adopcion.Lock()
		adopcion.sistema, adopcion.enJob = antesS, antesF
		adopcion.Unlock()
	}
}

// ValidarEscribe (ronda 1, 425a75 and 03a36d) applies to the Escribe that
// provider declares (providers.Estado.Escribe, "~/" being this process's
// $HOME) the rules of encierro.escribibles, when the envelope or the review
// starts: principal is the main checkout and comunGit the git common
// directory. It returns the first that breaks, in Escribe's order:
//
//   - `encierro: <provider> escribe <ruta>: es todo el disco o todo el directorio personal: no puede ser escribible`,
//     for `/` or the $HOME, after filepath.Clean;
//   - `encierro: <provider> escribe <ruta>: contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible`,
//
// <ruta> as the provider declares it. The envelope then refuses in `run`
// (exit 1, without launching) and the review before its first pass (exit 1,
// no record), like Validar's errors.
func ValidarEscribe(provider string, escribe []string, principal, comunGit string) error {
	home, _ := os.UserHomeDir()
	for _, r := range escribe {
		p := expandir(r, home)
		if p == "" {
			continue // not absolute: VistaDe never mounts it
		}
		if p == string(filepath.Separator) || p == filepath.VolumeName(p)+string(filepath.Separator) ||
			(home != "" && p == filepath.Clean(home)) {
			return fmt.Errorf("encierro: %s escribe %s: es todo el disco o todo el directorio personal: no puede ser escribible", provider, r)
		}
		for _, d := range []string{principal, comunGit} {
			if d != "" && cruza(p, d) {
				return fmt.Errorf("encierro: %s escribe %s: contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible", provider, r)
			}
		}
	}
	return nil
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
	quitar := map[string]bool{}
	for _, k := range VariablesQuitadas {
		quitar[k] = true
	}
	poner := [][2]string{{"TMPDIR", filepath.Join(v.Cache, "TMPDIR")}, {"XDG_CACHE_HOME", filepath.Join(v.Cache, "XDG_CACHE_HOME")}}
	for _, k := range v.VarsCache {
		poner = append(poner, [2]string{k, filepath.Join(v.Cache, k)})
	}
	n := 0
	for _, kv := range base {
		if k, val, ok := strings.Cut(kv, "="); ok && k == "GIT_CONFIG_COUNT" {
			if c, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && c > 0 {
				n = c
			}
		}
	}
	for i, kv := range ConfigGit {
		poner = append(poner, [2]string{"GIT_CONFIG_KEY_" + strconv.Itoa(n+i), kv[0]}, [2]string{"GIT_CONFIG_VALUE_" + strconv.Itoa(n+i), kv[1]})
	}
	poner = append(poner, [2]string{"GIT_CONFIG_COUNT", strconv.Itoa(n + len(ConfigGit))})
	for _, kv := range poner {
		quitar[kv[0]] = true
	}
	out := make([]string, 0, len(base)+len(poner))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !quitar[k] {
			out = append(out, kv)
		}
	}
	for _, kv := range poner {
		out = append(out, kv[0]+"="+kv[1])
	}
	return out
}

// RunUser is the directory the encierro covers as /run/user/<uid> of this
// process (the systemd user bus, the graphical session, the ssh and gpg
// agents): `/run/user/<uid>`, or the one ReemplazarRunUser set.
func RunUser() string {
	runUser.Lock()
	defer runUser.Unlock()
	if runUser.reemplazo != nil {
		return *runUser.reemplazo
	}
	return "/run/user/" + strconv.Itoa(os.Getuid())
}

// runUser is the replacement of RunUser, when a test set one.
var runUser struct {
	sync.Mutex
	reemplazo *string
}

// ReemplazarRunUser makes RunUser answer dir in this process until
// restaurar is called. It is for the tests: a runner without a session has no
// /run/user/<uid>, so the test creates a directory that plays it and checks
// that inside it is seen empty.
func ReemplazarRunUser(dir string) (restaurar func()) {
	runUser.Lock()
	antes := runUser.reemplazo
	runUser.reemplazo = &dir
	runUser.Unlock()
	return func() {
		runUser.Lock()
		runUser.reemplazo = antes
		runUser.Unlock()
	}
}

// Envolver prepares cmd, not started yet, to run under p. With
// p.Info.Mecanismo MecanismoBwrap, cmd ends up running bwrap with p.Vista
// around its original command (Path, Args and Dir), in its own PID, user and
// mount namespaces, a new session, no_new_privs, --die-with-parent and
// --info-fd, with hoom's step (Paso, through os.Executable) before the
// command: the step makes the command the leader of a session of its own and,
// with level completo, applies Landlock; its environment is
// Entorno(cmd.Environ(), p.Vista); the known secrets and the sockets are
// covered; it creates p.Vista.Cache and the directories Entorno points at.
// With any other mechanism (MecanismoJob, or "" with ninguno) it leaves cmd
// as it is. An error (bwrap not in PATH, the cache cannot be created) leaves
// cmd unchanged. Start cmd with Arrancar.
func Envolver(cmd *exec.Cmd, p Plan) error {
	if p.Info.Mecanismo != MecanismoBwrap {
		return nil
	}
	return envolver(cmd, p)
}

// envueltos are the commands Envolver prepared with bwrap and not yet
// forgotten by Liberar, with what Arrancar and Liberar need of each.
var envueltos = struct {
	sync.Mutex
	por map[*exec.Cmd]*envuelto
}{por: map[*exec.Cmd]*envuelto{}}

// lanzados are the commands Arrancar started (enclosed or not) and Liberar
// has not ended: their gracia, and when their context ended.
var lanzados = struct {
	sync.Mutex
	por map[*exec.Cmd]*lanzado
}{por: map[*exec.Cmd]*lanzado{}}

// lanzado is one command Arrancar started.
type lanzado struct {
	gracia time.Duration
	// cancelado is when cmd's Cancel ran (its context ended); zero if not.
	cancelado time.Time
	mu        sync.Mutex
}

// Arrancar starts cmd (made with exec.CommandContext, as procesos requires)
// in a process group of its own, registered for hoom's signal handler, like
// procesos.ArrancarConGracia (gracia 0: like procesos.Arrancar). For a cmd Envolver prepared with bwrap it keeps
// today's gracia: when cmd's context ends (its deadline, a cancel), and on a
// signal to hoom, SIGTERM goes to the group of the main process inside the
// encierro (found from the outside pid of its init, which bwrap reports with
// --info-fd), and only after
// gracia does the whole encierro end (SIGKILL to its pid 1), with everything
// inside it. Whoever calls it calls Liberar once cmd's Wait returned.
func Arrancar(cmd *exec.Cmd, gracia time.Duration) error {
	l := &lanzado{gracia: gracia}
	envueltos.Lock()
	e := envueltos.por[cmd]
	envueltos.Unlock()
	alCancelar := func() {
		l.mu.Lock()
		if l.cancelado.IsZero() {
			l.cancelado = time.Now()
		}
		l.mu.Unlock()
	}
	// the gracia of an encierro whose SIGTERM had to wait for bwrap to say
	// its init counts from that SIGTERM (ronda 1, 69b8a2)
	alTerm := func() {
		l.mu.Lock()
		if !l.cancelado.IsZero() {
			l.cancelado = time.Now()
		}
		l.mu.Unlock()
	}
	var err error
	if e != nil {
		err = arrancarEnvuelto(cmd, gracia, e, alCancelar, alTerm)
	} else {
		err = procesos.ArrancarCon(cmd, gracia, procesos.Terminacion{AlCancelar: alCancelar})
	}
	if err != nil {
		envueltos.Lock()
		delete(envueltos.por, cmd)
		envueltos.Unlock()
		if e != nil {
			e.cerrar()
		}
		return err
	}
	lanzados.Lock()
	lanzados.por[cmd] = l
	lanzados.Unlock()
	return nil
}

// Liberar is the end of cmd started with Arrancar, once its Wait returned:
// after the rest of its gracia when its context ended, whatever is left in
// its group, or inside its encierro, dies, and the group is forgotten (like
// procesos.EsperarGrupo and procesos.Liberar).
func Liberar(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	lanzados.Lock()
	l := lanzados.por[cmd]
	delete(lanzados.por, cmd)
	lanzados.Unlock()
	envueltos.Lock()
	e := envueltos.por[cmd]
	delete(envueltos.por, cmd)
	envueltos.Unlock()
	if l != nil && l.gracia > 0 {
		l.mu.Lock()
		cancelado := l.cancelado
		l.mu.Unlock()
		if !cancelado.IsZero() && e != nil {
			e.esperarFin(cancelado.Add(l.gracia)) // the encierro ends with its main process
		} else if !cancelado.IsZero() {
			procesos.EsperarGrupo(cmd.Process.Pid, cancelado.Add(l.gracia))
		}
	}
	procesos.Liberar(cmd.Process.Pid)
	if e != nil {
		e.terminar()
		e.cerrar()
	}
}

// errSinBwrap is the error of Envolver when bwrap is not in PATH.
var errSinBwrap = errors.New("encierro: bwrap no esta en el PATH")

// Paso is the step of hoom inside the encierro, before the command: when this
// process is that step (Envolver launched it, through os.Executable, inside
// bwrap) it becomes the leader of a new session, with level completo forbids
// itself, with Landlock, to connect to an abstract socket outside its domain,
// and executes the command in its place: it never returns. Otherwise it returns at once. This package's init calls it, so any
// binary that links the package does the step; calling it again, first thing
// in main or in a TestMain, is harmless (see the package's doc).
func Paso() {
	paso()
}

func init() { Paso() }
