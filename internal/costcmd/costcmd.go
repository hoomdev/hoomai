// Package costcmd is `hoom costo` (spec costo-por-spec): what each task cost
// until it was accepted, by role and by round of review, in the units each
// provider really gives. It only reads: the run sidecars (.hoom/runs), the
// review records (.hoom/reviews), the items (.hoom/items), the findings
// (.hoom/findings) and the verdicts (.hoom/verdicts). What a provider did not
// report is said ("sin dato"), never filled in with a zero.
//
// Text output (the strings a reader and a test can rely on; layout and
// spacing around them are not part of the contract):
//
//   - state of a task: "aceptada <fecha>" (fecha = "2006-01-02 15:04 UTC"),
//     "cerrada sin aceptar" or "en curso"; "rondas <n>";
//   - token figures, per provider: "nuevos <n>", "cache leido <n>",
//     "salida <n>", "razonamiento <n>", plain integers; a null figure is
//     "<label> sin dato" (e.g. "razonamiento sin dato");
//   - USD: "<x> USD equivalente API" with x to 2 decimals; a provider whose
//     runs reported no cost at all says "<provider> no informa costo"; some
//     runs without cost add "costo: sin dato en <n> de <m> runs";
//   - window, per window name: "<nombre> <x> puntos" (x with at most one
//     decimal, "3" not "3.0"), "<nombre> <1 punto" when it moved less than
//     one point, and "al menos " before the figure when AlMenos; a provider
//     with no window data says "ventana: sin dato", and finished runs without
//     it add "ventana: sin dato en <n> de <m> runs"; any output that shows a
//     window figure also says "(ventana de la cuenta durante los runs)";
//   - runs still running: "<n> en curso";
//   - unreadable sidecars: "<n> runs ilegibles";
//   - a round of review: its id, "completa", "delta" or "parcial",
//     "lentes <a>,<b>" and "high <a> medium <b> low <c>";
//   - reviewer runs no record names: "reviewer (sin ronda)";
//   - a role-less run is role "sin rol".
//
// Window rule (CA-545): the finished runs of one provider that have a window
// are ordered by created_at and grouped in blocks — a run joins the block
// while its created_at is not after the latest ended_at of the block. A
// block moved fin(last run to end) - inicio(first run to start). A block
// "reset" when one of its runs has the window reiniciada, or two consecutive
// runs (by created_at) report reinicia_en more than 60 s apart: then the
// block contributes fin(last run to end) — what was spent after the reset is
// at least that — and the total is AlMenos. A block whose figure went down
// with no reset seen is a reset hoom did not see, and counts the same. The
// total adds the blocks.
package costcmd

import (
	"flag"
	"fmt"
	"time"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/item"
)

// UsageText is `hoom costo --help`.
const UsageText = `Uso: hoom costo [<slug>] [--json]

  sin <slug>   una linea por tarea con runs o reviews, de la mas reciente a la
               mas vieja: estado, rondas de review y, por provider, tokens,
               USD y ventana
  <slug>       el desglose de la tarea: por rol, por ronda de review y el total
  --json       el mismo reporte como JSON en stdout

El movimiento de ventana es el de la ventana de la cuenta durante los runs:
si la cuenta gasto en otra sesion al mismo tiempo, entra. El primer dato de
cada run llega despues de su primera llamada.`

// Options are the parsed arguments of `hoom costo`.
type Options struct {
	Slug string // "" = every task
	JSON bool
}

// ParseArgs is pure: at most one positional (the slug, which must be a valid
// item slug) and --json. Anything else — another flag, two positionals, a
// slug of another shape — is a *cliargs.UsageError (exit 2); -h/--help is
// cliargs.ErrHelp.
func ParseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("costo", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emitir el reporte como JSON en stdout")
	ops, err := cliargs.OperandsRango(fs, args, "costo", UsageText, 0, 1)
	if err != nil {
		return Options{}, err
	}
	opt := Options{JSON: *asJSON}
	if len(ops) == 1 {
		if !item.ValidSlug(ops[0]) {
			return Options{}, cliargs.NewUsageError("costo", fmt.Sprintf("slug invalido %q", ops[0]), UsageText)
		}
		opt.Slug = ops[0]
	}
	return opt, nil
}

// Estado values of a task.
const (
	EstadoAceptada          = "aceptada"            // the item has hecho_en
	EstadoEnCurso           = "en_curso"            // no hecho_en and its worktree exists
	EstadoCerradaSinAceptar = "cerrada_sin_aceptar" // no hecho_en and no worktree
)

// ErrSinNada is the text of `hoom costo <slug>` for a slug with no runs and
// no reviews: the verb prints it to stderr and exits 1.
const ErrSinNada = "hoom costo: no hay runs ni reviews de la tarea %s"

// Reporte is what `hoom costo` shows and `--json` emits, with this exact
// schema. Every figure nobody reported is null, never 0.
type Reporte struct {
	Tareas []Tarea `json:"tareas"`
	// Ilegibles counts the sidecars of .hoom/runs that could not be read:
	// an unreadable sidecar names no task, so it is counted for the whole
	// report ("<n> runs ilegibles") and the command goes on.
	Ilegibles int `json:"ilegibles"`
}

// Tarea is one task's cost. Sin slug the report carries every task (newest
// activity first) with PorRol and RondasReview filled too; text output only
// prints the breakdown for a single slug.
type Tarea struct {
	Slug        string     `json:"slug"`
	Estado      string     `json:"estado"`       // EstadoAceptada | EstadoEnCurso | EstadoCerradaSinAceptar
	AceptadaEn  *time.Time `json:"aceptada_en"`  // the item's hecho_en; null if not accepted
	Veredicto   *string    `json:"veredicto"`    // id of the latest green verdict of .hoom/specs/<slug>.md, if accepted
	CommitFinal *string    `json:"commit_final"` // the item's commit_final, if accepted
	Rondas      int        `json:"rondas"`       // review records of the task
	// PorProvider is the total per provider, keyed by provider name. Tokens
	// of two providers are never added together anywhere.
	PorProvider map[string]Gasto `json:"por_provider"`
	// PorRol has one row per (role, provider), roles in alphabetical order;
	// a run without role is role "".
	PorRol []GastoRol `json:"por_rol"`
	// RondasReview has one row per review record, oldest first, plus, last,
	// one row with ID null and SinRonda true holding the reviewer runs no
	// record names (only when there are such runs).
	RondasReview []Ronda `json:"rondas_review"`
	// UltimaActividad is the newest of its runs' start and its records'
	// creation: the order of the list. Not part of the JSON.
	UltimaActividad time.Time `json:"-"`
}

// Gasto is what a set of runs of ONE provider cost.
type Gasto struct {
	Runs    int    `json:"runs"`
	EnCurso int    `json:"en_curso"` // runs still running: they add no window
	Tokens  Tokens `json:"tokens"`
	// CostoUSD is the sum of what the provider reported (Claude's
	// total_cost_usd: the API equivalent); null when no run reported a cost.
	CostoUSD *float64 `json:"costo_usd"`
	// SinCosto counts the runs that reported no cost.
	SinCosto int `json:"sin_costo"`
	// Ventanas is the movement of each plan window ("5h", "semana", ...);
	// null when no finished run of the set has a window.
	Ventanas map[string]Movimiento `json:"ventanas"`
	// SinVentana counts the finished runs of the set without any window.
	SinVentana int `json:"sin_ventana"`
	// DuracionMS is the sum of the finished runs' wall time
	// (ended_at - created_at); null when none finished.
	DuracionMS *int64 `json:"duracion_ms"`
}

// Tokens are provider-neutral columns: Nuevos is Claude input_tokens and
// Codex input_tokens - cached_input_tokens; CacheLeido is
// cached_input_tokens. Each is null when no run reported it.
type Tokens struct {
	Nuevos       *int `json:"nuevos"`
	CacheLeido   *int `json:"cache_leido"`
	Salida       *int `json:"salida"`
	Razonamiento *int `json:"razonamiento"`
}

// Movimiento is how far one window moved over a set of runs (see the window
// rule in the package doc).
type Movimiento struct {
	// Puntos: percent points moved, the sum of the blocks.
	Puntos *float64 `json:"puntos"`
	// AlMenos: some block reset, so Puntos is a lower bound.
	AlMenos bool `json:"al_menos"`
}

// GastoRol is the cost of one role on one provider.
type GastoRol struct {
	Rol      string `json:"rol"`
	Provider string `json:"provider"`
	Gasto
}

// Ronda is one round of review.
type Ronda struct {
	ID        *string        `json:"id"`        // the record id; null for the SinRonda row
	SinRonda  bool           `json:"sin_ronda"` // reviewer runs no record names
	Cobertura string         `json:"cobertura"` // completa | delta | parcial ("" for SinRonda)
	Lentes    []string       `json:"lentes"`
	Hallazgos map[string]int `json:"hallazgos"` // "high", "medium", "low" -> count of the record's findings
	Provider  string         `json:"provider"`
	// Gasto adds up the runs the record names; null when the record names
	// none (records older than costo-por-spec).
	Gasto *Gasto `json:"gasto"`
}
