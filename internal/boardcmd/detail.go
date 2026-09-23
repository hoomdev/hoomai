package boardcmd

import (
	"errors"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// DiffMaxBytes caps the patch of the detail's diff.
const DiffMaxBytes = 256 << 10

// How a criterion is covered.
const (
	TracedTest    = "test"
	TracedComando = "comando"
)

// Kinds of WorkRow.
const (
	WorkSobre = "sobre"
	WorkRun   = "run"
)

// Detail is everything the Studio shows about one card, read-only.
type Detail struct {
	Card        Card               `json:"card"`
	Spec        SpecDetail         `json:"spec"`
	Criteria    []CriterionTrace   `json:"criteria"`
	Verdict     *verdict.Verdict   `json:"verdict"`
	Fingerprint string             `json:"fingerprint"`
	Findings    []finding.Item     `json:"findings"`
	Reviews     []reviewcmd.Record `json:"reviews"`
	Work        []WorkRow          `json:"work"`
	Paths       Paths              `json:"paths"`
	Diff        *gitx.Diff         `json:"diff,omitempty"`
}

// SpecDetail is the card's spec as it is in the evidence tree.
type SpecDetail struct {
	Path     string           `json:"path"`
	Exists   bool             `json:"exists"`
	Markdown string           `json:"markdown"`
	Approval *approval.Record `json:"approval"`
}

// CriterionTrace is one criterion with how it is covered.
type CriterionTrace struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	TracedBy string   `json:"traced_by"` // TracedTest | TracedComando | ""
	Files    []string `json:"files"`
}

// WorkRow is one envelope of the card, or one run of the card that no
// envelope references.
type WorkRow struct {
	Kind       string           `json:"kind"` // WorkSobre | WorkRun
	ID         string           `json:"id"`
	RunID      string           `json:"run_id,omitempty"`
	Role       string           `json:"role"`
	Provider   string           `json:"provider"`
	Status     string           `json:"status"`
	Stage      string           `json:"stage,omitempty"`
	Step       int              `json:"step,omitempty"`
	Steps      int              `json:"steps,omitempty"`
	Isolated   bool             `json:"isolated"`
	StartedAt  time.Time        `json:"started_at"`
	EndedAt    *time.Time       `json:"ended_at,omitempty"`
	DurationMS *int64           `json:"duration_ms"`
	Alive      bool             `json:"alive"`
	Usage      *providers.Usage `json:"usage"`
	Note       string           `json:"note,omitempty"`
}

// Paths are the card's artifacts, relative to root ("" / empty when absent).
type Paths struct {
	Item     string   `json:"item"`
	Dir      string   `json:"dir"`
	Spec     string   `json:"spec"`
	Approval string   `json:"approval"`
	Verdict  string   `json:"verdict"`
	Reviews  []string `json:"reviews"`
	Findings []string `json:"findings"`
}

// DetailFor reads the detail of one card. It fails like CardFor (missing or
// invalid item) and, like Gather, it creates and modifies nothing.
func DetailFor(root, base, blockOn, slug string, now time.Time, withDiff bool) (Detail, error) {
	return Detail{}, errors.New("sin implementar")
}
