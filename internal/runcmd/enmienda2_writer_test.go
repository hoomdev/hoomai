// Test del writer para la enmienda 2 de costo-por-spec (hallazgo 54db05):
// lo que se lee de un sidecar respeta la misma regla que lo que se escribe.
package runcmd

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// CA-549: un sidecar con task no tiene finding_task, y un finding_task que no
// es un slug no llega a quien lee: se lee como sin tarea, igual que task.
func TestEnmienda2_CA549_FindingTaskLeidoSigueLaRegla(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(runsDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		id, task, ft, quiere string
	}{
		{"r-solo", "", "costo-por-spec", "costo-por-spec"},
		{"r-con-task", "otra", "costo-por-spec", ""},
		{"r-ruta", "", "../fuera", ""},
		{"r-control", "", "a\x1b[31m", ""},
	}
	for _, c := range casos {
		raw, err := json.Marshal(Meta{ID: c.id, Provider: "codex", Task: c.task, FindingTask: c.ft,
			Status: StatusDone, CreatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metaPath(root, c.id), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	metas, ilegibles, err := MetasConIlegibles(root)
	if err != nil || len(ilegibles) != 0 {
		t.Fatalf("MetasConIlegibles: %v %v", err, ilegibles)
	}
	got := map[string]string{}
	for _, m := range metas {
		got[m.ID] = m.FindingTask
	}
	for _, c := range casos {
		if got[c.id] != c.quiere {
			t.Errorf("%s: finding_task leido %q, quiere %q", c.id, got[c.id], c.quiere)
		}
	}
}
