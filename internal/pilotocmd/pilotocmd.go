package pilotocmd

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
)

// UsageText is the usage block of `hoom piloto`.
const UsageText = `Uso: hoom piloto <slug> [--json]

  Corre el piloto automatico de la tarjeta en primer plano hasta que se
  detiene: guarda, decide y lanza (o cierra hallazgos con su test), una linea
  por decision. Sale con 0 si se detuvo en una columna tuya o en Hecho, y con
  1 en cualquier otra detencion. Nunca aprueba ni integra.
  --json   Emite el resultado como JSON en stdout`

// Candado is the path of the lock `hoom piloto` holds while it runs on the
// card slug of the project root: .hoom/cache/locks/piloto-<slug>.lock.
func Candado(root, slug string) string {
	return ""
}

// MsgYaCorre is what a pilot gets when another process already holds the
// card's lock: `el piloto de <slug> ya corre en otro proceso`.
func MsgYaCorre(slug string) string {
	return ""
}

// AvisoDeFirma is the line `hoom aprobar <slug>` and `hoom spec approve
// <ruta>` add after signing, when the card of spec (the card whose spec path
// is spec, .hoom/specs/<slug>.md) has `auto: hasta-humano` in its item (read
// like the riesgo: the main checkout's item when it has it):
// `la tarjeta tiene piloto automatico: sigue con: hoom piloto <slug>`. ""
// when the spec is no card's or its card has no pilot. root is the project;
// spec is relative to it or absolute. It launches nothing.
func AvisoDeFirma(root, spec string) string {
	return ""
}

// Opciones are one run of the pilot on a card.
type Opciones struct {
	// Root is the project: the main checkout, the tree the Studio serves.
	Root string
	// Manifest is the project's hoom.yaml (its base, block_on and the
	// autonomia policy of LoadRoles); nil = read from Root.
	Manifest *manifest.Manifest
	Slug     string
	// Lanzar is the skin every launch goes through (lanzar.Params): the
	// Studio's own (its Runs, Candado, Grupo and a Despues that runs the next
	// turn), or the terminal's. Root and Manifest are taken from Opciones
	// when empty.
	Lanzar lanzar.Params
	// Salida also gets every line of the pilot (the output of `hoom
	// piloto`); the log of the card's newest closed envelope always gets
	// them. nil = only the log.
	Salida io.Writer
	// Ahora is the clock the turns read (Estado.Hora); nil = time.Now.
	Ahora func() time.Time
	// Proveedores are the providers of the machine (Estado.Proveedores); nil
	// = providers.Detect.
	Proveedores func() []providers.Info
}

// Resultado is what a run of the pilot did.
type Resultado struct {
	Slug string `json:"slug"`
	// Decisiones are the decisions of every turn, in order: the last one is
	// the stop (or, for Avanzar, the launch it returned on).
	Decisiones []boardcmd.PilotDecision `json:"-"`
	// Lineas are the pilot's lines, in the order they were written.
	Lineas []string `json:"lineas"`
	// Trabajo is the job the last turn launched, when Avanzar returned on a
	// launch; nil otherwise.
	Trabajo *lanzar.Trabajo `json:"-"`
	// ExitCode is `hoom piloto`'s: 0 when it stopped in a human column or in
	// Hecho (the table's case 3), 1 on any other stop.
	ExitCode int `json:"exit_code"`
}

// Avanzar runs turns on the card until one stops or launches a job, and
// returns then, without waiting for that job (the Studio's skin: its
// lanzar.Params.Despues runs Avanzar again when the job ends). It holds
// Candado(root, slug) while it decides and acts, never while the job runs;
// with the lock taken by another process it stops with MsgYaCorre(slug).
// The error is a broken setup (the project or the card cannot be read); a
// stop is a Resultado.
func Avanzar(ctx context.Context, o Opciones) (Resultado, error) {
	return Resultado{}, nil
}

// Correr runs turns on the card until one stops, waiting for every job it
// launches (`hoom piloto`), holding Candado(root, slug) the whole time. With
// the lock taken by another process it does not run a turn: it returns an
// error with the text MsgYaCorre(slug).
func Correr(ctx context.Context, o Opciones) (Resultado, error) {
	return Resultado{}, nil
}

// Armar reads the card's boardcmd.Estado from the disk: the card derived
// with o.Manifest's base and block_on, its closed envelopes (oldest first)
// with the verdict each one emitted and the session id of its run's
// sidecar, the card's newest complete verdict, its review records, the open
// findings of its task that block (finding.Bloquea) with the protected tests
// (agentcmd.TestsProtegidos) whose text contains each id, the project's
// autonomia policy, the newest "5h" datum per provider of the sidecars of
// the card's runs, the machine's providers and o.Ahora.
func Armar(o Opciones) (boardcmd.Estado, error) {
	return boardcmd.Estado{}, nil
}

// Entorno is the process boundary of `hoom piloto`: where it was called
// from and its standard streams. Tests replace it whole.
type Entorno struct {
	Dir string    // the working directory
	Out io.Writer // the pilot's lines, or the JSON
	Err io.Writer // errors, as `hoom: <error>`
}

// Real is the Entorno of the hoom process.
func Real() Entorno {
	return Entorno{Dir: ".", Out: os.Stdout, Err: os.Stderr}
}

// Run is `hoom piloto <slug> [--json]`, and it owns its exit code. The
// project is the one e.Dir belongs to (from inside a task's worktree, the
// project that contains it). It runs Correr on the card and prints each
// line of the pilot as it is written; with --json it prints the Resultado
// as JSON (`{"slug": ..., "lineas": [...], "exit_code": n}`) instead. It
// returns Resultado.ExitCode: 0 when the pilot stopped in a human column or
// in Hecho, 1 on any other stop. With the card's lock held by another
// process it prints `hoom: el piloto de <slug> ya corre en otro proceso` to
// e.Err and returns 1; a request it does not understand (no slug, an unknown
// flag) is a usage error with UsageText and returns 2.
func Run(args []string, e Entorno) int {
	return 0
}
