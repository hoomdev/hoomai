package boardcmd

import (
	"time"

	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Pulso is what the pilot and its save read of a card before they touch its
// working tree: CardFor's Item (auto and presupuesto_usd from the main
// checkout's item, as on the card), Evidence.Source, Evidence.Dir and
// Running, read the same way but without any git command that reads the
// working tree: no status, no diff, no fingerprint. Whatever such a git runs
// (a clean filter that never ends, aclaracion 21) then only runs inside the
// save, bounded by itemcmd.TopeGit and in its own group (aclaracion 19).
type Pulso struct {
	Item    item.Item
	Source  string   // SourceWorktree with the task's worktree, SourceArbol otherwise
	Dir     string   // ".hoom/worktrees/<slug>" with the task's worktree, "." otherwise
	Running *Running // who is working on the card now, nil when nobody
}

// PulsoDe is the Pulso of the card slug of root, with CardFor's error for
// an item that is missing or cannot be read.
func PulsoDe(root, slug string, now time.Time) (Pulso, error) {
	it, err := loadItem(root, slug)
	if err != nil {
		return Pulso{}, err
	}
	g := newGatherer(root, "", "", now)
	g.pilotoDelPrincipal(&it)
	p := Pulso{Item: it, Source: SourceArbol, Dir: "."}
	dir := root
	if wt, err := runcmd.TaskDir(root, slug); err == nil && slug != "" {
		dir, p.Source, p.Dir = wt, SourceWorktree, ".hoom/worktrees/"+slug
	}
	var ev Evidence
	ev.Envelopes, ev.Runs = g.telemetry(slug, dir)
	p.Running, _ = liveness(ev)
	return p, nil
}
