// Tests de regresion de los hallazgos 20261020T202026_0dc2de (medium,
// reliability) y 20261020T202440_4cab20 (medium, resilience) de la review de
// los fixes de C1, los dos el mismo defecto PRE-EXISTENTE: cada cost_usd se
// valida finito, pero la SUMA no. Dos runs de la tarjeta con cost_usd 1e308
// (JSON valido y finito cada uno) desbordan el total a +Inf, remaining_usd
// queda -Inf, y json.Marshal del tablero falla: `hoom board --json` sale con
// error y /api/board responde 500 por un solo item.
//
// El contrato: el gasto de la tarjeta (.hoom/specs/items-y-columna-derivada.md,
// CA-287) es finito; el run que haria desbordar la suma cuenta como sin costo
// reportado, igual que un costo ausente (runs_without_cost, con sus tokens), y
// la suma de cost_usd de las filas de work del detalle es igual a
// card.spend.cost_usd (.hoom/specs/tablero-de-solo-lectura.md, CA-314): la
// fila de ese run queda sin costo.
package boardcmd

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// c1bFinito dice si p es un numero finito (no nil).
func c1bFinito(p *float64) bool {
	return p != nil && !math.IsInf(*p, 0) && !math.IsNaN(*p)
}

// c1bRuns planta en root la tarjeta slug (con presupuesto 10) y un run
// cerrado por costo, con tokens distintos; el run i empezo 10*(i+1) minutos
// antes de now. etiqueta son 3 caracteres [a-z] que hacen unicos los ids
// dentro del repo. Devuelve los ids de los runs y la suma de sus tokens.
func c1bRuns(t *testing.T, root, slug, etiqueta string, now time.Time, costos ...float64) (ids []string, in, out int) {
	t.Helper()
	bdItemArchivo(t, root, slug, "Tarjeta "+slug, "presupuesto_usd: 10\n")
	for i, c := range costos {
		id := fmt.Sprintf("20261020T2020%02d_%s%03d", i, etiqueta, i)
		ini := now.Add(-time.Duration(10*(i+1)) * time.Minute)
		bdRunDisco(t, root, runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: slug, Dir: root,
			CreatedAt: ini, EndedAt: ini.Add(5 * time.Minute), Status: runcmd.StatusDone,
			Usage: &providers.Usage{CostUSD: bdF(c), InputTokens: 100 * (i + 1), OutputTokens: 10 * (i + 1)}})
		ids = append(ids, id)
		in, out = in+100*(i+1), out+10*(i+1)
	}
	return ids, in, out
}

// c1bWork suma las filas de work: el costo de las que traen costo (nil si
// ninguna) y cuantas no traen.
func c1bWork(d Detail) (costo *float64, sinCosto, in, out int) {
	for _, w := range d.Work {
		if w.Usage == nil {
			sinCosto++
			continue
		}
		if w.Usage.CostUSD != nil {
			if costo == nil {
				costo = bdF(0)
			}
			*costo += *w.Usage.CostUSD
		} else {
			sinCosto++
		}
		in += w.Usage.InputTokens
		out += w.Usage.OutputTokens
	}
	return costo, sinCosto, in, out
}

// Hallazgos 20261020T202026_0dc2de y 20261020T202440_4cab20. CA-287 y CA-314:
// con costos que desbordan la suma, el tablero se serializa, el gasto de la
// tarjeta es finito (el run que desbordaria cuenta como sin costo, con sus
// tokens), remaining_usd es finito, y la suma de las filas de work es el
// gasto de la tarjeta. Pasa igual sin importar en que orden se sumen los
// runs.
func TestCA287_Hallazgo0dc2de_LaSumaDeCostosNoDesborda(t *testing.T) {
	casos := []struct {
		slug, etq  string
		costos     []float64
		total      float64 // el gasto que queda
		sinCosto   int     // los runs que cuentan como sin costo
		filasNulas int     // las filas de work sin costo
	}{
		{"dos-1e308", "dua", []float64{1e308, 1e308}, 1e308, 1, 1},
		{"dos-maximos", "dmx", []float64{math.MaxFloat64, math.MaxFloat64}, math.MaxFloat64, 1, 1},
		{"tres-1e308", "tre", []float64{1e308, 1e308, 1e308}, 1e308, 2, 2},
		// cualquiera sea el orden, entran 1e308 y 7e307 y el otro 1e308 no
		{"dos-entran-uno-no", "den", []float64{1e308, 7e307, 1e308}, 1e308 + 7e307, 1, 1},
	}
	root := bdRepo(t, "")
	now := time.Now().UTC()
	type plantado struct {
		ids     []string
		in, out int
	}
	plantados := map[string]plantado{}
	for _, c := range casos {
		ids, in, out := c1bRuns(t, root, c.slug, c.etq, now, c.costos...)
		plantados[c.slug] = plantado{ids, in, out}
	}

	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-288: Build: %v", err)
	}
	if _, err := JSONBytes(b); err != nil {
		t.Errorf("CA-288: hallazgo 0dc2de: el tablero se serializa aunque la suma de costos desborde (hoom board --json y /api/board): %v", err)
	}
	for _, c := range casos {
		t.Run(c.slug, func(t *testing.T) {
			p := plantados[c.slug]
			s := bdCard(t, b, c.slug).Spend
			if !c1bFinito(s.CostUSD) || *s.CostUSD != c.total {
				t.Errorf("CA-287: hallazgo 0dc2de: el gasto de la tarjeta es finito y suma solo los runs que entran: cost_usd %s, esperaba %v", h2FmtF(s.CostUSD), c.total)
			}
			if s.Runs != len(c.costos) || s.RunsWithoutCost != c.sinCosto {
				t.Errorf("CA-287: hallazgo 0dc2de: el run que haria desbordar la suma cuenta como sin costo reportado: runs %d sin costo %d, esperaba %d y %d",
					s.Runs, s.RunsWithoutCost, len(c.costos), c.sinCosto)
			}
			if s.InputTokens != p.in || s.OutputTokens != p.out {
				t.Errorf("CA-287: los tokens de todos los runs suman, tambien los del run sin costo: %d/%d, esperaba %d/%d", s.InputTokens, s.OutputTokens, p.in, p.out)
			}
			if !c1bFinito(s.RemainingUSD) || s.BudgetUSD == nil || *s.RemainingUSD != *s.BudgetUSD-c.total {
				t.Errorf("CA-287: hallazgo 0dc2de: remaining_usd es el presupuesto menos el gasto, finito: %s (budget %s)", h2FmtF(s.RemainingUSD), h2FmtF(s.BudgetUSD))
			}
			if _, err := json.Marshal(s); err != nil {
				t.Errorf("CA-287: hallazgo 0dc2de: el gasto de la tarjeta se serializa: %v", err)
			}

			d, err := DetailFor(root, "main", "high", c.slug, now, false)
			if err != nil {
				t.Fatalf("CA-313: DetailFor: %v", err)
			}
			if _, err := json.Marshal(d); err != nil {
				t.Errorf("CA-313: hallazgo 0dc2de: el detalle se serializa (GET /api/board/<slug>): %v", err)
			}
			if len(d.Work) != len(c.costos) {
				t.Fatalf("CA-314: work trae una fila por run de la tarjeta: %d filas, %d runs", len(d.Work), len(c.costos))
			}
			costo, filasNulas, in, out := c1bWork(d)
			if !h2MismoF(costo, d.Card.Spend.CostUSD) || !c1bFinito(costo) {
				t.Errorf("CA-314: hallazgo 0dc2de: la suma de cost_usd de las filas de work (%s) es igual a card.spend.cost_usd (%s) y finita",
					h2FmtF(costo), h2FmtF(d.Card.Spend.CostUSD))
			}
			if filasNulas != c.filasNulas {
				t.Errorf("CA-314: hallazgo 0dc2de: la fila del run que haria desbordar la suma queda sin costo, como un costo ausente: %d filas sin costo, esperaba %d",
					filasNulas, c.filasNulas)
			}
			if in != d.Card.Spend.InputTokens || out != d.Card.Spend.OutputTokens {
				t.Errorf("CA-314: la suma de tokens de las filas (%d/%d) es la de card.spend (%d/%d)", in, out, d.Card.Spend.InputTokens, d.Card.Spend.OutputTokens)
			}
		})
	}
}

// Hallazgos 20261020T202026_0dc2de y 20261020T202440_4cab20. CA-287 y CA-314:
// el desborde tambien puede venir del usage de un sobre cuyo run no dejo
// sidecar (ese usage suma al gasto). El gasto queda finito y la suma de las
// filas de work es el gasto de la tarjeta.
func TestCA314_Hallazgo4cab20_DesbordeConUnSobreSinSidecar(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	ids, _, _ := c1bRuns(t, root, "con-sobre", "cso", now, 1e308)
	ini := now.Add(-time.Hour)
	bdSobreDisco(t, root, envelope.Record{ID: "20261020T202440_c1bsob", Role: "writer", Provider: "claude", Task: "con-sobre",
		Dir: root, RunID: "20261020T202440_perdid", Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
		Usage:     &providers.Usage{CostUSD: bdF(1e308), InputTokens: 7, OutputTokens: 7},
		StartedAt: ini, UpdatedAt: ini.Add(5 * time.Minute), EndedAt: ini.Add(5 * time.Minute)})

	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-288: Build: %v", err)
	}
	if _, err := JSONBytes(b); err != nil {
		t.Errorf("CA-288: hallazgo 4cab20: el tablero se serializa aunque el sobre sin sidecar desborde la suma: %v", err)
	}
	s := bdCard(t, b, "con-sobre").Spend
	if !c1bFinito(s.CostUSD) || *s.CostUSD != 1e308 || !c1bFinito(s.RemainingUSD) {
		t.Errorf("CA-287: hallazgo 4cab20: el gasto y lo que queda del presupuesto son finitos: cost_usd %s remaining %s", h2FmtF(s.CostUSD), h2FmtF(s.RemainingUSD))
	}
	if s.InputTokens != 107 || s.OutputTokens != 17 {
		t.Errorf("CA-287: los tokens del run y del sobre sin sidecar suman: %d/%d", s.InputTokens, s.OutputTokens)
	}
	d, err := DetailFor(root, "main", "high", "con-sobre", now, false)
	if err != nil {
		t.Fatalf("CA-313: DetailFor: %v", err)
	}
	if _, err := json.Marshal(d); err != nil {
		t.Errorf("CA-313: hallazgo 4cab20: el detalle se serializa: %v", err)
	}
	costo, filasNulas, _, _ := c1bWork(d)
	if len(d.Work) != 2 || filasNulas != 1 || !h2MismoF(costo, d.Card.Spend.CostUSD) {
		t.Errorf("CA-314: hallazgo 4cab20: una fila por sobre y por run (%d, run %s), la que desbordaria sin costo (%d sin costo), y la suma de work (%s) es card.spend (%s)",
			len(d.Work), ids[0], filasNulas, h2FmtF(costo), h2FmtF(d.Card.Spend.CostUSD))
	}
}

// Hallazgos 20261020T202026_0dc2de y 20261020T202440_4cab20. CA-288: con el
// binario real, `hoom board --json` no falla por dos runs de 1e308: sale 0,
// el gasto de la tarjeta es finito y un run cuenta como sin costo.
func TestCA288_Hallazgo0dc2de_BoardJSONConCostosQueDesbordan(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "")
	c1bRuns(t, root, "costo", "bin", time.Now().UTC(), 1e308, 1e308)
	r := bxCorrer(t, bin, root, "board", "--json")
	if r.exit != 0 {
		t.Fatalf("CA-288: hallazgo 0dc2de: 'hoom board --json' sale 0 aunque la suma de costos desborde; salio %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	}
	c := bxCard(t, bin, root, "costo")
	if !c1bFinito(c.Spend.CostUSD) || *c.Spend.CostUSD != 1e308 || c.Spend.Runs != 2 || c.Spend.RunsWithoutCost != 1 || !c1bFinito(c.Spend.RemainingUSD) {
		t.Fatalf("CA-287: hallazgo 0dc2de: el gasto de la tarjeta en 'hoom board --json': cost_usd %s, runs %d, sin costo %d, remaining %s",
			h2FmtF(c.Spend.CostUSD), c.Spend.Runs, c.Spend.RunsWithoutCost, h2FmtF(c.Spend.RemainingUSD))
	}
}
