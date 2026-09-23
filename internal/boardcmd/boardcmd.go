// Package boardcmd computes the cabin's board: the column of every item as a
// FUNCTION of the evidence on disk and in Git. Nobody writes a column. Gather
// collects the evidence of one card (read-only), Derive turns it into a Card
// (pure: no disk, no clock, no git), and Build does it for every item of the
// tree. The same evidence gives the same column on any machine, and nothing
// is stored — there is nothing to corrupt or to rebuild.
//
// A column is the station where the PENDING work is: a card in "Tu
// aprobacion" is waiting for a person to sign, and it sits there until the
// evidence of that signature exists.
package boardcmd

import (
	"flag"
	"time"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Column ids, fixed in the binary: the order is the method.
const (
	ColBacklog      = "backlog"
	ColArquitecto   = "arquitecto"
	ColTuAprobacion = "tu-aprobacion"
	ColTestWriter   = "test-writer"
	ColWriter       = "writer"
	ColReview       = "review"
	ColTuAceptacion = "tu-aceptacion"
	ColHecho        = "hecho"
)

// Column is one of the eight, as the board prints it.
type Column struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Human bool   `json:"human"` // the two purple columns: a person acts
}

// Columns is the fixed order 1..8.
var Columns = []Column{
	{ID: ColBacklog, Name: "Backlog"},
	{ID: ColArquitecto, Name: "Arquitecto"},
	{ID: ColTuAprobacion, Name: "Tu aprobacion", Human: true},
	{ID: ColTestWriter, Name: "Test-writer"},
	{ID: ColWriter, Name: "Writer"},
	{ID: ColReview, Name: "Review"},
	{ID: ColTuAceptacion, Name: "Tu aceptacion", Human: true},
	{ID: ColHecho, Name: "Hecho"},
}

// Evidence sources: where the card's evidence was read.
const (
	SourceWorktree = "worktree" // .hoom/worktrees/<slug>
	SourceArbol    = "arbol"    // the current tree
)

// Red sources.
const (
	RedVerdict  = "veredicto"
	RedEnvelope = "sobre"
)

// EnvelopeState is one envelope record of the card plus the only thing
// Derive cannot know by itself: whether its owner is alive. Gather resolves it
// (run sidecar running with a live PID, or a fresh heartbeat).
type EnvelopeState struct {
	Record envelope.Record
	Alive  bool
}

// RunState is one run sidecar of the card plus whether its owner process is
// alive (PID probe, resolved by Gather).
type RunState struct {
	Meta  runcmd.Meta
	Alive bool
}

// Evidence is everything Derive needs about one card, already read. Paths are
// relative to the project root.
type Evidence struct {
	Item item.Item
	Now  time.Time

	Source string // SourceWorktree | SourceArbol
	Dir    string // the evidence tree E, relative to root ("." = the current tree)

	SpecPath   string   // ".hoom/specs/<slug>.md"
	SpecExists bool     // the file exists in E
	LintIssues []string // spec.Lint issues (read error included); empty = passes
	Criteria   []string // CA-n ids of the spec
	Untraced   []string // criteria without test token nor verifica command
	ByCommand  []string // criteria covered by a verifica marker

	Approval string // approval.StatusApproved | StatusNotApproved | StatusInvalidated

	Verdict       *verdict.Verdict // the card's newest complete verdict (spec == SpecPath); nil = none
	GreenVerdicts []string         // ids of every complete GREEN verdict of the card
	Fingerprint   string           // current change fingerprint of E

	Reviews  []reviewcmd.Record // review records of E with task == slug
	Findings []finding.Item     // OPEN findings of E with task == slug
	BlockOn  string             // effective threshold: the project's block_on, or "high"

	ReadyErr    string   // taskcmd.Ready error message; "" = task done would close it
	ReadyKind   string   // taskcmd.ReadyError kind of ReadyErr ("" when unknown)
	Worktree    bool     // the task worktree exists
	Uncommitted []string // uncommitted evidence paths (see the spec)

	Envelopes []EnvelopeState // envelope records with task == slug, newest first
	Runs      []RunState      // run sidecars with task == slug, newest first

	// C3: what the card's actions need that Derive cannot find out alone.
	Providers []providers.Info // the providers of this machine (providers.Detect, once per Build)
	Signer    string           // git identity of the evidence tree: who an approval would name
}

// Card is one item on the board.
type Card struct {
	Slug         string       `json:"slug"`
	Item         item.Item    `json:"item"`
	Column       string       `json:"column"`
	ColumnName   string       `json:"column_name"`
	Missing      []string     `json:"missing"`
	Next         string       `json:"next"`
	Evidence     CardEvidence `json:"evidence"`
	Running      *Running     `json:"running"`
	Interrupted  *Interrupted `json:"interrupted"`
	WaitingHuman bool         `json:"waiting_human"`
	Red          *Red         `json:"red"`
	Unsynced     []string     `json:"unsynced"`
	Spend        Spend        `json:"spend"`

	// C2: what the Studio paints, derived here so the page only paints.
	Plain         string    `json:"plain"`          // the main reason, in the normal mode's words
	NeedsDecision bool      `json:"needs_decision"` // waiting_human or interrupted
	Meter         []Segment `json:"meter"`          // the evidence meter, segment by segment
	Providers     Providers `json:"providers"`      // who wrote and who reviewed

	// C3: what can be done with the card. The page shows it; every action
	// endpoint derives the card again before acting.
	Actions []Action `json:"actions"` // valid for its column, in order: the primary first
	Drops   []Drop   `json:"drops"`   // one per other column: what dropping there does, or why not
	Ghost   *Ghost   `json:"ghost"`   // the card as it will be, in the next column, while a role works
}

// Action ids.
const (
	ActPedirArquitecto = "pedir-arquitecto"
	ActPedirTestWriter = "pedir-test-writer"
	ActPedirWriter     = "pedir-writer"
	ActPedirReviewer   = "pedir-reviewer"
	ActAprobar         = "aprobar"
	ActIntegrar        = "integrar"
	ActReanudar        = "reanudar"
	ActRelanzar        = "relanzar"
	ActDescartar       = "descartar"
	ActGuardar         = "guardar"
	ActSesion          = "sesion"
	ActTerminal        = "terminal"
)

// Action is one thing a person can do with the card now.
type Action struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"` // the role it launches ("" when it launches none)
	Label     string           `json:"label"`
	Expert    bool             `json:"expert"` // shown only in expert mode
	Enabled   bool             `json:"enabled"`
	Why       string           `json:"why"`        // why not, in the normal mode's words ("" when enabled)
	Pedido    string           `json:"pedido"`     // proposed request, editable
	BudgetUSD *float64         `json:"budget_usd"` // proposed budget; nil = no cap
	Providers []ProviderOption `json:"providers"`
	Paths     []string         `json:"paths"`     // guardar: unsynced; descartar: what goes back
	ResumeID  string           `json:"resume_id"` // reanudar: the provider session to resume
	Signer    string           `json:"signer"`    // aprobar: the identity that signs
}

// ProviderOption is one installed provider as a role action sees it.
type ProviderOption struct {
	Name         string  `json:"name"`
	OK           bool    `json:"ok"`
	Why          string  `json:"why"`
	Budget       bool    `json:"budget"` // takes a USD cap
	MinBudgetUSD float64 `json:"min_budget_usd"`
	Default      bool    `json:"default"`
}

// Drop is what dropping the card on one column does: the action it opens,
// or why it opens nothing.
type Drop struct {
	Column string `json:"column"`
	Action string `json:"action"`
	Why    string `json:"why"`
}

// Ghost is the card in the column its running role would win.
type Ghost struct {
	Column     string `json:"column"`
	Role       string `json:"role"`
	Provider   string `json:"provider"`
	EnvelopeID string `json:"envelope_id"`
	RunID      string `json:"run_id"`
	Stage      string `json:"stage"`
	Step       int    `json:"step"`
	Steps      int    `json:"steps"`
}

// Action returns the card's action with that id, if it has it.
func (c Card) Action(id string) (Action, bool) {
	return Action{}, false // esqueleto
}

// Segment states: a segment fills only with a fact that exists and holds.
const (
	SegHecho    = "hecho"
	SegFalta    = "falta"
	SegNoAplica = "no-aplica"
)

// Segment is one cell of the evidence meter.
type Segment struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// Providers names the CLI that wrote the card's code and the one that
// reviewed it ("" when nothing says so).
type Providers struct {
	Writer   string `json:"writer"`
	Reviewer string `json:"reviewer"`
	Cross    string `json:"cross"`
	// Declared: the providers of the item's interactive sessions, distinct,
	// in order of appearance.
	Declared []string `json:"declared"`
}

// CardEvidence is the evidence meter: every segment is something that can be
// opened and checked.
type CardEvidence struct {
	Source           string   `json:"source"`
	Dir              string   `json:"dir"`
	Spec             string   `json:"spec"`
	SpecExists       bool     `json:"spec_exists"`
	LintIssues       []string `json:"lint_issues"`
	Criteria         int      `json:"criteria"`
	Traced           int      `json:"traced"`
	Untraced         []string `json:"untraced"`
	Approval         string   `json:"approval"`
	VerdictID        string   `json:"verdict_id"`
	Verdict          string   `json:"verdict"`
	FingerprintMatch bool     `json:"fingerprint_match"`
	ReviewRequired   bool     `json:"review_required"`
	ReviewID         string   `json:"review_id"`
	BlockingFindings []string `json:"blocking_findings"`
}

// Running: an envelope or run of the card whose owner is alive.
type Running struct {
	EnvelopeID string    `json:"envelope_id,omitempty"`
	RunID      string    `json:"run_id,omitempty"`
	Role       string    `json:"role,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Stage      string    `json:"stage"`
	Step       int       `json:"step"`
	Steps      int       `json:"steps"`
	StartedAt  time.Time `json:"started_at"`
}

// Interrupted: an open envelope whose owner is not alive. hoom labels it; it
// never closes what it did not see close.
type Interrupted struct {
	EnvelopeID string    `json:"envelope_id"`
	RunID      string    `json:"run_id,omitempty"`
	Role       string    `json:"role,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Stage      string    `json:"stage"`
	Step       int       `json:"step"`
	Steps      int       `json:"steps"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Red is the reason, in one sentence, of the newest failure of the card.
type Red struct {
	Source string `json:"source"` // RedVerdict | RedEnvelope
	ID     string `json:"id"`
	Reason string `json:"reason"`
	Plain  string `json:"plain"` // the same failure in the normal mode's words, without the note
}

// Spend is what the card's runs cost on THIS machine (local telemetry).
// CostUSD is nil when no run reported a cost: "no data" is not 0.
type Spend struct {
	CostUSD         *float64 `json:"cost_usd"`
	Runs            int      `json:"runs"`
	RunsWithoutCost int      `json:"runs_without_cost"`
	InputTokens     int      `json:"input_tokens"`
	OutputTokens    int      `json:"output_tokens"`
	BudgetUSD       *float64 `json:"budget_usd"`
	// RemainingUSD is the budget minus the reported cost (may be negative);
	// nil without a budget.
	RemainingUSD *float64 `json:"remaining_usd"`
}

// BoardColumn is a column with its cards.
type BoardColumn struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Human bool   `json:"human"`
	Cards []Card `json:"cards"`
}

// Board is the whole projection: the eight columns always, in order.
type Board struct {
	Columns  []BoardColumn `json:"columns"`
	Warnings []string      `json:"warnings"`
}

// UsageText is the exact usage block of `hoom board`.
const UsageText = `Uso: hoom board [--json]

  --json   Emite el tablero como JSON en stdout

La columna de cada item sale de su evidencia (spec, aprobacion, tests,
veredicto, review, hallazgos, cierre); nada se guarda y nadie la escribe.
'hoom board' no tiene argumentos posicionales.`

// Options mirror the board verb's flags.
type Options struct {
	JSON bool
}

// ParseArgs is pure: a request hoom does not understand is a
// *cliargs.UsageError; -h/--help is cliargs.ErrHelp.
func ParseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("board", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emitir el tablero como JSON en stdout")
	if err := cliargs.Strict(fs, args, "board", UsageText); err != nil {
		return Options{}, err
	}
	return Options{JSON: *asJSON}, nil
}
