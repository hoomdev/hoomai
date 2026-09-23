package boardcmd

import (
	"errors"
	"time"
)

// Timeline sources: where an entry was read. The page labels them, so a
// person on another machine knows why the agents' work is missing.
const (
	SourceGit        = "git"
	SourceTelemetria = "telemetria"
)

// Timeline entry kinds, in their tie-break order.
const (
	KindItem         = "item"
	KindSpec         = "spec"
	KindAprobacion   = "aprobacion"
	KindCommit       = "commit"
	KindVeredicto    = "veredicto"
	KindHallazgo     = "hallazgo"
	KindResolucion   = "resolucion"
	KindReview       = "review"
	KindSobre        = "sobre"
	KindRun          = "run"
	KindSubagente    = "subagente"
	KindSubagenteFin = "subagente-fin"
)

// TimelinePatchMax caps the patch of the task's commits read to light the
// criteria.
const TimelinePatchMax = 8 << 20

// The notes of a timeline, verbatim.
const (
	NoteSinTelemetria = "sin telemetria en esta computadora: el trabajo de los agentes solo se ve donde corrio"
	NoteFastForward   = "no se pueden separar los commits de la tarea: se integro sin commit de merge"
	NoteSinEspacio    = "la tarjeta no tiene espacio de trabajo propio: la historia muestra solo su evidencia"
	NoteTestsCortados = "la historia de los tests se corto en 8 MiB: los criterios se encienden con el veredicto"
	NoteSinGitPrefijo = "sin historial de git: "
)

// Timeline is the story of one card: how it got where it is.
type Timeline struct {
	Slug      string          `json:"slug"`
	Card      Card            `json:"card"`  // the card now: the replay's last frame
	Meter     []MeterSlot     `json:"meter"` // the replay's meter skeleton (card.meter's ids and labels)
	Entries   []TimelineEntry `json:"entries"`
	Telemetry TelemetryCount  `json:"telemetry"`
	Notes     []string        `json:"notes"`
}

// MeterSlot is one segment of the replay's meter.
type MeterSlot struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// MeterEffect is what one entry does to one segment of the replay's meter.
type MeterEffect struct {
	ID    string `json:"id"`
	State string `json:"state"` // SegHecho | SegFalta | SegNoAplica
}

// TimelineEntry is one moment of the card's story.
type TimelineEntry struct {
	At       time.Time     `json:"at"`
	EndedAt  *time.Time    `json:"ended_at"`
	Source   string        `json:"source"` // SourceGit | SourceTelemetria
	Kind     string        `json:"kind"`
	Who      string        `json:"who"` // git identity, or "<rol> (<provider>)"
	Role     string        `json:"role"`
	Provider string        `json:"provider"`
	Artifact string        `json:"artifact"` // relative to root; "" for a code commit
	Ref      string        `json:"ref"`      // sha, envelope id, run id
	Summary  string        `json:"summary"`  // expert mode
	Plain    string        `json:"plain"`    // normal mode
	CostUSD  *float64      `json:"cost_usd"`
	Tokens   int           `json:"tokens"`
	Piloto   bool          `json:"piloto"`
	Meter    []MeterEffect `json:"meter"`
}

// TelemetryCount is how many envelopes and runs of the card this machine has.
type TelemetryCount struct {
	Sobres int `json:"sobres"`
	Runs   int `json:"runs"`
}

// TimelineFor reads the story of one card. It fails like CardFor and, like
// Gather, creates and modifies nothing: it only runs git reads.
func TimelineFor(root, base, blockOn, slug string, now time.Time) (Timeline, error) {
	return Timeline{}, errors.New("sin implementar")
}
