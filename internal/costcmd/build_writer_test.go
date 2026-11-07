package costcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// CA-544: the runs a record names are its task's even when they were
// started without a task (hoom review --spec without --task), so the total
// by role and provider covers every round.
func TestCA544_RunsNombradosPorElRegistroSonDeLaTarea(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".hoom", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	m := runcmd.Meta{ID: "run-sin-tarea", Provider: "codex", Role: "reviewer", Status: runcmd.StatusDone,
		CreatedAt: at, EndedAt: at.Add(time.Minute)}
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, m.ID+".meta.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewcmd.WriteRecord(root, reviewcmd.Record{Task: "alfa", Provider: "codex", Cobertura: "completa",
		Lenses: []string{"risk"}, Runs: []reviewcmd.LensRun{{Lens: "risk", Run: m.ID}}, CreatedAt: at.Add(2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(root, "alfa")
	if err != nil {
		t.Fatal(err)
	}
	ta := rep.Tareas[0]
	if g, ok := ta.PorProvider["codex"]; !ok || g.Runs != 1 {
		t.Fatalf("por_provider codex = %+v, quiero 1 run", ta.PorProvider)
	}
	if len(ta.PorRol) != 1 || ta.PorRol[0].Rol != "reviewer" || ta.PorRol[0].Runs != 1 {
		t.Fatalf("por_rol = %+v", ta.PorRol)
	}
	for _, r := range ta.RondasReview {
		if r.SinRonda {
			t.Fatalf("un run nombrado no va a reviewer (sin ronda): %+v", r)
		}
	}
}
