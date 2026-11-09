// hoom costo y el gasto de hoy (spec tablero-con-paneles, CA-561), por el
// binario real: sin slug la primera linea es "hoy: ...", con los runs que
// empezaron hoy en la hora local; --json suma "hoy"; con slug no cambia.
package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// CA-561: hoom costo sin slug imprime primero la linea de hoy, y --json
// trae hoy con runs, costo_usd y sin_costo; hoom costo <slug> no la tiene.
func TestCA561_HoomCostoHoy(t *testing.T) {
	ahora := time.Now()
	if fin := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 23, 59, 0, 0, ahora.Location()); ahora.After(fin) {
		t.Skip("a un minuto de medianoche el dia puede cambiar durante el test")
	}
	bin := qcHoom(t)
	dir := vdtRepo(t, "")
	costo := 0.5
	hoy := ahora // CA-561, hallazgo 27e287: un segundo antes, entre las 00:00:00 y las 00:00:01, caeria ayer
	cpsRun(t, dir, hoy.UTC().Format("20060102T150405")+"_c0f001", "alfa", "writer", "claude", hoy,
		&providers.Usage{InputTokens: 10, OutputTokens: 1, CostUSD: &costo, Turns: 1})
	cpsRun(t, dir, hoy.UTC().Format("20060102T150405")+"_c0f002", "alfa", "reviewer", "codex", hoy,
		&providers.Usage{InputTokens: 10, OutputTokens: 1, Turns: 1})
	ayer := ahora.Add(-48 * time.Hour)
	cpsRun(t, dir, ayer.UTC().Format("20060102T150405")+"_c0f003", "alfa", "writer", "claude", ayer,
		&providers.Usage{InputTokens: 10, OutputTokens: 1, CostUSD: &costo, Turns: 1})
	s := qcCorrer(t, bin, dir, nil, "costo")
	if s.exit != 0 {
		t.Fatalf("CA-561: hoom costo sale con 0: %d\n%s", s.exit, s.todo())
	}
	if primera := strings.SplitN(s.stdout, "\n", 2)[0]; primera != "hoy: 0.50 USD en 2 trabajos, 1 sin dato de costo" {
		t.Fatalf("CA-561: la primera linea de hoom costo es la de hoy: %q\n%s", primera, s.stdout)
	}
	j := qcCorrer(t, bin, dir, nil, "costo", "--json")
	var m struct {
		Hoy *struct {
			Runs     int      `json:"runs"`
			CostoUSD *float64 `json:"costo_usd"`
			SinCosto int      `json:"sin_costo"`
		} `json:"hoy"`
	}
	if err := json.Unmarshal([]byte(j.stdout), &m); err != nil || m.Hoy == nil || m.Hoy.Runs != 2 || m.Hoy.SinCosto != 1 ||
		m.Hoy.CostoUSD == nil || *m.Hoy.CostoUSD < 0.499 || *m.Hoy.CostoUSD > 0.501 {
		t.Fatalf("CA-561: hoom costo --json trae hoy {runs 2, costo_usd 0.5, sin_costo 1}: %v\n%s", err, j.stdout)
	}
	u := qcCorrer(t, bin, dir, nil, "costo", "alfa")
	if u.exit != 0 || strings.Contains(u.stdout, "hoy:") {
		t.Fatalf("CA-561: hoom costo <slug> no cambia (sin la linea de hoy): %d\n%s", u.exit, u.todo())
	}
	v := qcCorrer(t, bin, vdtRepo(t, ""), nil, "costo")
	if v.exit != 0 || !strings.HasPrefix(v.stdout, "hoy: sin trabajos\n") {
		t.Fatalf("CA-561: sin runs, la primera linea es 'hoy: sin trabajos': %d\n%s", v.exit, v.todo())
	}
}
