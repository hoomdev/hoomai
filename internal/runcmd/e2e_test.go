// Test del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md (CA-216):
// E2E opcional contra el Claude real. Se omite salvo HOOM_E2E=1 y jamas es
// requisito de `go test`.
package runcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// CA-216: un run corto real deja costo > 0 y al menos un turno en su sidecar,
// y su jsonl no tiene ni una linea JSON cruda de "type":"system" — que es
// exactamente lo que se veia antes de esta spec (8 de 12 lineas de una
// corrida trivial).
func TestCA216_E2ECostoYRuidoReales(t *testing.T) {
	if os.Getenv("HOOM_E2E") != "1" {
		t.Skip("CA-216: E2E opcional; exporta HOOM_E2E=1 y tene 'claude' en PATH para correrlo")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skipf("CA-216: 'claude' no esta en PATH: %v", err)
	}
	root := t.TempDir()
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "responde unicamente OK",
		MaxTurns: 1, BudgetUSD: 1})
	if err != nil {
		t.Fatalf("CA-216: el run no arranco: %v", err)
	}
	fin := waitRun(t, m, info.ID)
	if fin.Status != StatusDone {
		t.Fatalf("CA-216: el run real debe cerrar bien: %+v", fin)
	}

	meta := Metas(root)
	if len(meta) != 1 || meta[0].Usage == nil {
		t.Fatalf("CA-216: el sidecar tiene que traer el gasto medido: %+v", meta)
	}
	u := meta[0].Usage
	if u.CostUSD == nil || *u.CostUSD <= 0 {
		t.Fatalf("CA-216: Claude reporta costo y tiene que quedar guardado: %+v", u)
	}
	if u.Turns < 1 {
		t.Fatalf("CA-216: al menos un turno: %+v", u)
	}

	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", info.ID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, `\"type\":\"system\"`) || strings.Contains(line, `"type":"system"`) {
			t.Fatalf("CA-216: ninguna linea system puede quedar como JSON crudo: %s", line)
		}
	}
	if !strings.Contains(string(raw), `"kind":"system"`) {
		t.Fatalf("CA-216: el ruido de la CLI tiene que estar, con su kind:\n%s", raw)
	}
}
