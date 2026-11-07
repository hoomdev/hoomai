// Package costcmd is `hoom costo` (spec costo-por-spec): what each task cost
// until it was accepted, by role and by round of review, in the units each
// provider really gives. It only reads: the run sidecars (.hoom/runs), the
// review records (.hoom/reviews), the items (.hoom/items), the findings
// (.hoom/findings) and the verdicts (.hoom/verdicts). What a provider did not
// report is said ("sin dato"), never filled in with a zero.
//
// Text output (the strings a reader and a test can rely on):
//
//   - state of a task: "aceptada <fecha>" (fecha = "2006-01-02 15:04 UTC"),
//     "cerrada sin aceptar" or "en curso";
//   - token columns, per provider: "nuevos", "cache leido", "salida",
//     "razonamiento";
//   - USD: the figure followed by "USD equivalente API"; a total that does not
//     cover a provider adds "<provider> no informa costo";
//   - window movement, per window and provider: "<x> puntos", "<1 punto"
//     when it moved less than one point, "al menos <x> puntos" when a stretch
//     could not be measured because the window reset; every window line ends
//     with "(ventana de la cuenta durante los runs)";
//   - missing data: "sin dato", and when some runs lack it
//     "<que>: sin dato en <n> de <m> runs";
//   - unreadable sidecars: "<n> runs ilegibles";
//   - a round of review: its id, "completa", "delta" or "parcial", its lenses
//     and "high <a> medium <b> low <c>";
//   - reviewer runs no round names: "reviewer (sin ronda)".
package costcmd

import (
	"errors"
	"io"
	"time"
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

// ParseArgs is pure: a request hoom does not understand is a
// *cliargs.UsageError (exit 2); -h/--help is cliargs.ErrHelp.
func ParseArgs(args []string) (Options, error) {
	return Options{}, errors.New("costcmd: esqueleto sin comportamiento")
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
	// Ventanas is the movement of each plan window ("5h", "semana", ...).
	Ventanas map[string]Movimiento `json:"ventanas"`
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

// Movimiento is how far one window moved over a set of runs: overlapping
// runs form one block (from its first run's inicio to its last run's fin)
// and the blocks are added.
type Movimiento struct {
	// Puntos: percent points moved; null when no run of the set has this
	// window.
	Puntos *float64 `json:"puntos"`
	// AlMenos: some stretch could not be measured (a run with the window
	// reiniciada, or a reset time that moved more than 60 s inside a block),
	// so Puntos is a lower bound.
	AlMenos bool `json:"al_menos"`
	// SinDato counts the finished runs of the set without this window.
	SinDato int `json:"sin_dato"`
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

// Build reads the project at root and returns the report: every task when
// slug is "", or that task alone. A slug with no runs and no reviews is an
// error whose text is fmt.Sprintf(ErrSinNada, slug).
func Build(root, slug string) (Reporte, error) {
	return Reporte{}, errors.New("costcmd: esqueleto sin comportamiento")
}

// JSONBytes renders the report as indented JSON.
func JSONBytes(r Reporte) ([]byte, error) {
	return nil, errors.New("costcmd: esqueleto sin comportamiento")
}

// Render writes the report for humans: one line per task, or the breakdown
// when the report was built for one slug (detalle).
func Render(w io.Writer, r Reporte, detalle bool) {}
