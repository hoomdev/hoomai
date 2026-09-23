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
	"io"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
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

	Approval string // approval.StatusApproved | StatusNotApproved | StatusInvalidated

	Verdict       *verdict.Verdict // the card's newest complete verdict (spec == SpecPath); nil = none
	GreenVerdicts []string         // ids of every complete GREEN verdict of the card
	Fingerprint   string           // current change fingerprint of E

	Reviews  []reviewcmd.Record // review records of E with task == slug
	Findings []finding.Item     // OPEN findings of E with task == slug
	BlockOn  string             // effective threshold: the project's block_on, or "high"

	ReadyErr    string   // taskcmd.Ready error message; "" = task done would close it
	Worktree    bool     // the task worktree exists
	Uncommitted []string // uncommitted evidence paths (see the spec)

	Envelopes []EnvelopeState // envelope records with task == slug, newest first
	Runs      []RunState      // run sidecars with task == slug, newest first
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

// Derive computes the card from its evidence. Pure.
func Derive(ev Evidence) Card {
	return Card{}
}

// Gather reads the evidence of one item. Read-only.
func Gather(root, base, blockOn string, it item.Item, now time.Time) Evidence {
	return Evidence{}
}

// Build derives the card of every valid item of root.
func Build(root, base, blockOn string, now time.Time) (Board, error) {
	return Board{}, nil
}

// CardFor derives the card of one item.
func CardFor(root, base, blockOn, slug string, now time.Time) (Card, error) {
	return Card{}, nil
}

// JSONBytes renders the board exactly as `hoom board --json` emits it.
func JSONBytes(b Board) ([]byte, error) {
	return nil, nil
}

// Render prints the board for humans.
func Render(w io.Writer, b Board) {}

// RenderCard prints one card for humans (`hoom item show`).
func RenderCard(w io.Writer, c Card) {}

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
	return Options{}, nil
}
