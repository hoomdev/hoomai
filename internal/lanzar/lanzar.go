// Package lanzar is the ONE way a role is launched on a card, whoever asks:
// the Studio's POST /launch, its belt and the card verbs of the terminal
// (spec verbos-de-la-tarjeta). It takes the card's launch lock, derives the
// card, checks the action and its values in the contract's order, writes the
// envelope's log and creates the task of a card in Backlog. The skins differ
// only in Params: the Studio returns at the first record and runs its belt
// afterwards; the terminal waits for the end and copies the narration to its
// standard output.
package lanzar

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/valores"
)

// MsgLanzando is what the second launch on the same card gets while the
// first one is being set up, from any process.
const MsgLanzando = "ya se esta lanzando un trabajo en esta tarjeta"

// Request is one launch of a role action.
type Request struct {
	Action    string        // pedir-<rol>, reanudar or relanzar
	Provider  string        // "" = the action's default provider
	Model     string        // "" = the action's model (hoom.yaml) or the provider's
	Effort    string        // "" = the action's effort (hoom.yaml) or the provider's
	BudgetUSD *float64      // nil = the action's proposed budget
	Timeout   time.Duration // 0 = the action's timeout (timeout_s)
	Pedido    string        // what the role has to do; the reviewer builds its own
	Pilot     bool          // the belt launched it: its record says so
}

// Params are the skin of a launch.
type Params struct {
	Root     string             // the project: the main tree, the one the Studio serves
	Manifest *manifest.Manifest // the project's hoom.yaml
	// Runs is the Studio's own manager: a run it is starting counts as
	// active before its sidecar says so. nil = only the sidecars.
	Runs *runcmd.Manager
	// Eco also gets the job's narration (the terminal's stdout); the log of
	// the envelope always gets it. nil = only the log.
	Eco io.Writer
	// Candado is an extra in-process lock taken before the file lock (the
	// Studio's); nil = only the file lock. ok=false means taken.
	Candado func(slug string) (soltar func(), ok bool)
	// Despues runs in the job's goroutine once the job ended (the Studio's
	// belt), with the card as it was before the launch, the envelope's id and
	// the envelope's log. nil = nothing.
	Despues func(antes boardcmd.Card, envelopeID string, log io.Writer)
	// Grupo, when given, counts the job's goroutine (the Studio waits for its
	// launches with it).
	Grupo *sync.WaitGroup
}

// Rechazo is why a launch did not happen: the same motive for every skin, and
// the HTTP code the Studio answers with (the terminal exits with 1).
type Rechazo struct {
	Code   int
	Motivo string
}

func (r *Rechazo) Error() string { return r.Motivo }

// Trabajo is a launched job: its envelope already wrote its first record (or
// the job already ended and said why it did not run).
type Trabajo struct {
	EnvelopeID  string
	Role        string
	Provider    string
	TaskStarted bool
	Valores     valores.Elegidos // the values it was launched with
	fin         chan Fin
}

// Fin is how a job ended.
type Fin struct {
	Agent   *agentcmd.Result  // the envelope's result (roles of the envelope)
	Review  *reviewcmd.Result // the review's result (the reviewer)
	Err     error             // a broken setup
	Nota    string            // the note of a review decided without passes
	Arranco bool              // it reached its first record
}

// Wait blocks until the job ends.
func (t *Trabajo) Wait() Fin {
	panic("sin implementar")
}

// Candado is the path of the card's launch lock: a file lock in
// .hoom/cache/locks/ that the Studio and the terminal take.
func Candado(root, slug string) string {
	return hoomfs.LockPath(root, "lanzar-"+slug)
}

// Valores are the values a launch of action a of card c takes for req: what
// req asks, else what the action proposes (which already applied hoom.yaml),
// each with its origin. The card verbs print them before launching.
func Valores(c boardcmd.Card, a boardcmd.Action, req Request) valores.Elegidos {
	panic("sin implementar")
}

// Lanzar launches req on the card slug. It answers when the envelope's first
// record is on disk, with a *Rechazo when the launch did not happen, or when
// ctx is done (the job then goes on by itself).
func Lanzar(ctx context.Context, p Params, slug string, req Request) (*Trabajo, error) {
	panic("sin implementar")
}
