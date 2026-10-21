// Tests de regresion del hallazgo 20261021T150011_3229f3 (medium,
// reliability) de la review de los fixes de C1: el filtro de desborde del
// gasto de la tarjeta (hallazgos 0dc2de y 4cab20, ver
// hallazgos_c1b_costo_desborda_test.go) dependia del orden en que se sumaban
// los costos. [1e308, 8e307, 7e307] dejaba 1.7e308 (sin costo el 8e307) y
// [8e307, 7e307, 1e308] dejaba 1.5e308 (sin costo el 1e308), y el orden lo
// deciden las fechas de los runs y los sobres que los referencian, no los
// costos.
//
// La regla (.hoom/specs/items-y-columna-derivada.md, CA-287;
// .hoom/specs/tablero-de-solo-lectura.md, CA-314;
// .hoom/specs/acciones-desde-la-tarjeta.md, CA-327), que mantiene lo que
// exigen los tests de 0dc2de y 4cab20: los costos de la tarjeta se ordenan de
// menor a mayor, con empate por el id del run, y se suman en ese orden; el
// run cuyo costo haria pasar la suma a infinito cuenta como sin costo, igual
// que todos los que vienen despues de el. Asi el total y que filas de work
// quedan sin costo son los mismos para cualquier orden de los runs y de los
// sobres:
//
//   - [1e308, 8e307, 7e307] en cualquier orden, con o sin sobres: cost_usd
//     es 7e307 + 8e307, queda sin costo solo la fila del 1e308, y las filas
//     suman el gasto;
//   - dos runs de 1e308 (empate): cost_usd 1e308 y queda sin costo el de id
//     mayor, sea cual sea el mas nuevo y aunque un sobre referencie a uno;
//   - guarda: si todo entra, todos los costos cuentan.
package boardcmd

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// o3Presupuesto es el presupuesto_usd de todas las tarjetas de estos tests.
const o3Presupuesto = 10.0

// o3Conjunto son tres costos validos, cada uno con el resultado que exige la
// regla: el total y cuales quedan sin costo (por indice).
type o3Conjunto struct {
	nombre   string
	costos   []float64
	total    float64
	sinCosto map[int]bool
}

func o3Conjuntos() []o3Conjunto {
	total := 7e307 // se suman de menor a mayor: 7e307, 8e307 y el 1e308 ya no entra
	total += 8e307
	pot := func(e int) float64 { return math.Ldexp(1, e) }
	return []o3Conjunto{
		{"el de 3229f3", []float64{1e308, 8e307, 7e307}, total, map[int]bool{0: true}},
		// guarda: todo entra (suma exacta en cualquier orden)
		{"guarda que entra", []float64{pot(1023), pot(1022), pot(1021)}, pot(1023) + pot(1022) + pot(1021), map[int]bool{}},
	}
}

// o3Permutaciones da las 6 permutaciones de {0, 1, 2}.
func o3Permutaciones() [][]int {
	return [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
}

// o3Etiqueta son 3 letras [a-z] distintas para cada n < 26^3.
func o3Etiqueta(n int) string {
	return string([]byte{byte('a' + n/676%26), byte('a' + n/26%26), byte('a' + n%26)})
}

func o3CopiaUso(u *providers.Usage) *providers.Usage {
	c := *u
	if u.CostUSD != nil {
		c.CostUSD = bdF(*u.CostUSD)
	}
	return &c
}

// o3Plantada es una tarjeta en disco: que costo plantado tiene cada fila de
// work (por el id de la fila: el del run, o el del sobre), y sus tokens.
type o3Plantada struct {
	slug, forma, que string
	grupo            string // tarjetas del mismo grupo solo difieren en el orden
	costoDeFila      map[string]float64
	in, out          int
	filas            int
	runs             int // runs con sidecar
	total            float64
	sinCosto         map[string]bool // filas que quedan sin costo
	rwc              int             // runs_without_cost esperado; -1 = no se exige
}

// o3Plantar planta la tarjeta slug con los costos del conjunto. orden dice en
// que lugar del tiempo queda cada costo (orden[i] = lugar del costo i; lugar
// 0 = lo mas nuevo). forma: "runs" (tres runs), "huerfano" (el costo c lo
// trae un sobre cuyo run no tiene sidecar) o "del-run" (tres runs y un sobre
// que referencia el run del costo c, con su mismo usage).
func o3Plantar(t *testing.T, root, slug, etq string, now time.Time, cj o3Conjunto, forma string, c int, orden []int) o3Plantada {
	t.Helper()
	bdItemArchivo(t, root, slug, "Tarjeta "+slug, "presupuesto_usd: 10\n")
	ini := func(lugar int) time.Time { return now.Add(-time.Duration(10*(lugar+1)) * time.Minute) }
	p := o3Plantada{slug: slug, forma: forma, costoDeFila: map[string]float64{}, sinCosto: map[string]bool{},
		filas: 3, total: cj.total, rwc: len(cj.sinCosto)}
	p.que = fmt.Sprintf("%s, %s (con el costo %d), lugares %v", cj.nombre, forma, c, orden)
	if forma == "runs" {
		p.que = fmt.Sprintf("%s, %s, lugares %v", cj.nombre, forma, orden)
	}
	p.grupo = fmt.Sprintf("%s|%s|%d", cj.nombre, forma, c)
	idRun := map[int]string{}
	for i, costo := range cj.costos {
		lugar := orden[i]
		u := &providers.Usage{CostUSD: bdF(costo), InputTokens: 100 * (i + 1), OutputTokens: 10 * (i + 1)}
		p.in, p.out = p.in+u.InputTokens, p.out+u.OutputTokens
		if forma == "huerfano" && i == c {
			id := fmt.Sprintf("20261021T151%d00_%ss%02d", 5-lugar, etq, lugar)
			bdSobreDisco(t, root, envelope.Record{ID: id, Role: "writer", Provider: "claude", Task: slug,
				Dir: root, RunID: fmt.Sprintf("20261021T151%d00_%sp%02d", 5-lugar, etq, lugar), Stage: "ok", Step: 5, Steps: 5,
				Status: envelope.StatusDeliverable, Usage: u, StartedAt: ini(lugar), UpdatedAt: ini(lugar).Add(5 * time.Minute),
				EndedAt: ini(lugar).Add(5 * time.Minute)})
			p.costoDeFila[id] = costo
			if cj.sinCosto[i] {
				p.sinCosto[id] = true
				p.rwc = -1 // si el sobre sin sidecar cuenta como run no lo dice el contrato
			}
			continue
		}
		id := fmt.Sprintf("20261021T150%d00_%sr%02d", 5-lugar, etq, lugar)
		bdRunDisco(t, root, runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: slug, Dir: root,
			CreatedAt: ini(lugar), EndedAt: ini(lugar).Add(5 * time.Minute), Status: runcmd.StatusDone, Usage: u})
		p.runs++
		idRun[i] = id
		p.costoDeFila[id] = costo
		if cj.sinCosto[i] {
			p.sinCosto[id] = true
		}
	}
	if forma == "del-run" {
		id := fmt.Sprintf("20261021T152000_%ss99", etq)
		at := ini(orden[c]).Add(time.Minute)
		u := &providers.Usage{CostUSD: bdF(cj.costos[c]), InputTokens: 100 * (c + 1), OutputTokens: 10 * (c + 1)}
		bdSobreDisco(t, root, envelope.Record{ID: id, Role: "writer", Provider: "claude", Task: slug,
			Dir: root, RunID: idRun[c], Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable, Usage: u,
			StartedAt: at, UpdatedAt: at.Add(5 * time.Minute), EndedAt: at.Add(5 * time.Minute)})
		// la fila del run referenciado es la del sobre
		delete(p.costoDeFila, idRun[c])
		p.costoDeFila[id] = cj.costos[c]
		if p.sinCosto[idRun[c]] {
			delete(p.sinCosto, idRun[c])
			p.sinCosto[id] = true
		}
	}
	return p
}

// o3Revisar exige el gasto y las filas de una tarjeta plantada: cost_usd es
// el total de la regla, quedan sin costo exactamente las filas de la regla
// (las demas traen su costo plantado), las filas suman el gasto y
// remaining_usd es el presupuesto menos el gasto.
func o3Revisar(t *testing.T, root string, now time.Time, b Board, p o3Plantada) Spend {
	t.Helper()
	s := bdCard(t, b, p.slug).Spend
	if s.CostUSD == nil || *s.CostUSD != p.total {
		t.Errorf("CA-287: hallazgo 3229f3: %s: cost_usd es %v en cualquier orden (se suman de menor a mayor), fue %s",
			p.que, p.total, h2FmtF(s.CostUSD))
	}
	if s.InputTokens != p.in || s.OutputTokens != p.out {
		t.Errorf("CA-287: %s: los tokens de todos suman, tambien los de las filas sin costo: %d/%d, esperaba %d/%d",
			p.que, s.InputTokens, s.OutputTokens, p.in, p.out)
	}
	if p.forma != "huerfano" && s.Runs != p.runs {
		t.Errorf("CA-287: %s: runs %d, esperaba %d (el sobre de un run con sidecar no suma otro run)", p.que, s.Runs, p.runs)
	}
	if p.rwc >= 0 && s.RunsWithoutCost != p.rwc {
		t.Errorf("CA-287: hallazgo 3229f3: %s: runs_without_cost %d, esperaba %d", p.que, s.RunsWithoutCost, p.rwc)
	}
	if s.CostUSD != nil && (s.RemainingUSD == nil || *s.RemainingUSD != o3Presupuesto-*s.CostUSD) {
		t.Errorf("CA-327: %s: remaining_usd es el presupuesto menos el gasto: %s", p.que, h2FmtSpend(s))
	}
	d, err := DetailFor(root, "main", "high", p.slug, now, false)
	if err != nil {
		t.Fatalf("CA-313: DetailFor(%s): %v", p.slug, err)
	}
	if _, err := json.Marshal(d); err != nil {
		t.Errorf("CA-313: %s: el detalle se serializa: %v", p.que, err)
	}
	if len(d.Work) != p.filas {
		t.Errorf("CA-314: %s: work trae %d filas, esperaba %d", p.que, len(d.Work), p.filas)
	}
	for _, w := range d.Work {
		plantado, ok := p.costoDeFila[w.ID]
		if !ok {
			t.Errorf("CA-314: %s: fila de work inesperada %s (%s, run %s)", p.que, w.ID, w.Kind, w.RunID)
			continue
		}
		var got *float64
		if w.Usage != nil {
			got = w.Usage.CostUSD
		}
		switch {
		case p.sinCosto[w.ID] && got != nil:
			t.Errorf("CA-314: hallazgo 3229f3: %s: la fila %s (costo %v) es la que haria pasar la suma a infinito: queda sin costo, trae %s",
				p.que, w.ID, plantado, h2FmtF(got))
		case !p.sinCosto[w.ID] && (got == nil || *got != plantado):
			t.Errorf("CA-314: hallazgo 3229f3: %s: la fila %s entra en la suma de menor a mayor: trae su costo %v, trae %s",
				p.que, w.ID, plantado, h2FmtF(got))
		}
	}
	costo, _, in, out := c1bWork(d)
	if !h2MismoF(costo, d.Card.Spend.CostUSD) {
		t.Errorf("CA-314: hallazgo 3229f3: %s: la suma de cost_usd de las filas (%s) es card.spend.cost_usd (%s)",
			p.que, h2FmtF(costo), h2FmtF(d.Card.Spend.CostUSD))
	}
	if in != d.Card.Spend.InputTokens || out != d.Card.Spend.OutputTokens {
		t.Errorf("CA-314: %s: la suma de tokens de las filas (%d/%d) es la de card.spend (%d/%d)", p.que, in, out, d.Card.Spend.InputTokens, d.Card.Spend.OutputTokens)
	}
	return s
}

// Hallazgo 20261021T150011_3229f3. CA-287, CA-314 y CA-327, en disco (Gather
// ordena por fechas y antepone los runs que referencian los sobres):
// [1e308, 8e307, 7e307] en cada uno de los 6 ordenes en el tiempo, sin
// sobres, con cada costo en un sobre sin sidecar y con un sobre que
// referencia el run de cada costo. Siempre cost_usd = 7e307 + 8e307, queda
// sin costo solo la fila del 1e308, las filas suman el gasto, y el gasto es
// el mismo en todos los ordenes de la misma forma. Guarda: con costos que
// entran, todos cuentan, en las mismas formas y ordenes.
func TestCA314_Hallazgo3229f3_PermutacionesEnDisco(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	var ps []o3Plantada
	for ci, cj := range o3Conjuntos() {
		for _, forma := range []string{"runs", "huerfano", "del-run"} {
			cs := []int{0, 1, 2}
			if forma == "runs" {
				cs = []int{-1}
			}
			for _, c := range cs {
				for pi, orden := range o3Permutaciones() {
					slug := fmt.Sprintf("o3-%d-%s-%d-%d", ci, forma, c+1, pi)
					ps = append(ps, o3Plantar(t, root, slug, o3Etiqueta(len(ps)), now, cj, forma, c, orden))
				}
			}
		}
	}
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-288: Build: %v", err)
	}
	if _, err := JSONBytes(b); err != nil {
		t.Errorf("CA-288: hallazgo 3229f3: el tablero se serializa: %v", err)
	}
	primero := map[string]o3Plantada{}
	gasto := map[string]Spend{}
	for _, p := range ps {
		s := o3Revisar(t, root, now, b, p)
		if q, ok := primero[p.grupo]; !ok {
			primero[p.grupo], gasto[p.grupo] = p, s
		} else if !reflect.DeepEqual(s, gasto[p.grupo]) {
			t.Errorf("CA-287: hallazgo 3229f3: el gasto no depende del orden:\n  %s: %s\n  %s: %s",
				q.que, h2FmtSpend(gasto[p.grupo]), p.que, h2FmtSpend(s))
		}
	}
}

// Hallazgo 20261021T150011_3229f3. CA-287 y CA-314: empate. Dos runs de
// 1e308 con ids X < Y, en los dos ordenes en el tiempo, sin sobres y con un
// sobre que referencia a X o a Y. Se ordenan de menor a mayor con empate por
// el id del run: entra el de X y queda sin costo el de id mayor (Y). cost_usd
// es 1e308 y runs_without_cost 1 en todos los casos.
func TestCA314_Hallazgo3229f3_EmpateQuedaSinCostoElDeIdMayor(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	ini := func(lugar int) time.Time { return now.Add(-time.Duration(10*(lugar+1)) * time.Minute) }
	type caso struct {
		slug, que string
		filaX     string // la fila que trae el run X (su id, o el del sobre que lo referencia)
		filaY     string
	}
	var casos []caso
	n := 0
	for _, xMasNuevo := range []bool{true, false} {
		for _, sobre := range []string{"", "X", "Y"} {
			etq := o3Etiqueta(500 + n)
			n++
			slug := fmt.Sprintf("empate-%d", n)
			bdItemArchivo(t, root, slug, "Tarjeta "+slug, "presupuesto_usd: 10\n")
			idX := fmt.Sprintf("20261021T160000_%sa01", etq)
			idY := fmt.Sprintf("20261021T160000_%sa02", etq)
			lugarX, lugarY := 0, 1
			if !xMasNuevo {
				lugarX, lugarY = 1, 0
			}
			for _, r := range []struct {
				id    string
				lugar int
				in    int
			}{{idX, lugarX, 100}, {idY, lugarY, 200}} {
				bdRunDisco(t, root, runcmd.Meta{ID: r.id, Provider: "claude", Role: "writer", Task: slug, Dir: root,
					CreatedAt: ini(r.lugar), EndedAt: ini(r.lugar).Add(5 * time.Minute), Status: runcmd.StatusDone,
					Usage: &providers.Usage{CostUSD: bdF(1e308), InputTokens: r.in, OutputTokens: r.in / 10}})
			}
			c := caso{slug: slug, filaX: idX, filaY: idY,
				que: fmt.Sprintf("X mas nuevo %v, sobre de %q", xMasNuevo, sobre)}
			if sobre != "" {
				ref, lugar, in := idX, lugarX, 100
				if sobre == "Y" {
					ref, lugar, in = idY, lugarY, 200
				}
				idS := fmt.Sprintf("20261021T160500_%ss01", etq)
				at := ini(lugar).Add(time.Minute)
				bdSobreDisco(t, root, envelope.Record{ID: idS, Role: "writer", Provider: "claude", Task: slug,
					Dir: root, RunID: ref, Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
					Usage:     &providers.Usage{CostUSD: bdF(1e308), InputTokens: in, OutputTokens: in / 10},
					StartedAt: at, UpdatedAt: at.Add(5 * time.Minute), EndedAt: at.Add(5 * time.Minute)})
				if sobre == "X" {
					c.filaX = idS
				} else {
					c.filaY = idS
				}
			}
			casos = append(casos, c)
		}
	}
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-288: Build: %v", err)
	}
	if _, err := JSONBytes(b); err != nil {
		t.Errorf("CA-288: hallazgo 3229f3: el tablero se serializa: %v", err)
	}
	for _, c := range casos {
		s := bdCard(t, b, c.slug).Spend
		if s.CostUSD == nil || *s.CostUSD != 1e308 || s.Runs != 2 || s.RunsWithoutCost != 1 || s.InputTokens != 300 || s.OutputTokens != 30 {
			t.Errorf("CA-287: hallazgo 3229f3: empate (%s): cost_usd 1e308, runs 2, sin costo 1, tokens 300/30: %s", c.que, h2FmtSpend(s))
		}
		d, err := DetailFor(root, "main", "high", c.slug, now, false)
		if err != nil {
			t.Fatalf("CA-313: DetailFor(%s): %v", c.slug, err)
		}
		if len(d.Work) != 2 {
			t.Errorf("CA-314: empate (%s): dos filas de work, hay %d", c.que, len(d.Work))
		}
		costos := map[string]*float64{}
		for _, w := range d.Work {
			if w.Usage != nil {
				costos[w.ID] = w.Usage.CostUSD
			} else {
				costos[w.ID] = nil
			}
		}
		x, okX := costos[c.filaX]
		y, okY := costos[c.filaY]
		if !okX || !okY || x == nil || *x != 1e308 || y != nil {
			t.Errorf("CA-314: hallazgo 3229f3: empate (%s): se ordenan por id: la fila de X (%s) trae 1e308 y la de Y, id mayor (%s), queda sin costo: X %s, Y %s (filas %v)",
				c.que, c.filaX, c.filaY, h2FmtF(x), h2FmtF(y), costos)
		}
		costo, _, _, _ := c1bWork(d)
		if !h2MismoF(costo, d.Card.Spend.CostUSD) {
			t.Errorf("CA-314: empate (%s): las filas suman card.spend.cost_usd: %s vs %s", c.que, h2FmtF(costo), h2FmtF(d.Card.Spend.CostUSD))
		}
	}
}

// o3RunEv es un run cerrado de la tarjeta en memoria, empezado min minutos
// despues de bdT0.
func o3RunEv(id string, costo float64, in, min int) RunState {
	at := bdT0.Add(time.Duration(min) * time.Minute)
	return RunState{Meta: runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: bdSlug,
		Status: runcmd.StatusDone, CreatedAt: at, EndedAt: at.Add(2 * time.Minute),
		Usage: &providers.Usage{CostUSD: bdF(costo), InputTokens: in, OutputTokens: in / 10}}}
}

// o3SobreEv es un sobre cerrado de la tarjeta en memoria.
func o3SobreEv(id, runID string, u *providers.Usage, min int) EnvelopeState {
	at := bdT0.Add(time.Duration(min) * time.Minute)
	return EnvelopeState{Record: envelope.Record{ID: id, RunID: runID, Role: "writer", Provider: "claude", Task: bdSlug,
		Status: envelope.StatusDeliverable, Stage: "ok", Step: 5, Steps: 5,
		StartedAt: at, UpdatedAt: at.Add(2 * time.Minute), EndedAt: at.Add(2 * time.Minute), Usage: u}}
}

// Hallazgo 20261021T150011_3229f3. CA-287 y CA-327, en memoria (Derive, con
// el orden de las listas bajo control): los runs y sobres de la tarjeta en
// cada orden de la lista (lo mas nuevo primero, con las fechas de acuerdo),
// sin sobres, con un sobre sin sidecar y con sobres que referencian runs.
// cost_usd y runs_without_cost son los de la regla en todos los ordenes, y
// el gasto es el mismo. Empate: dos runs de 1e308 en los dos ordenes dan
// cost_usd 1e308 y un run sin costo.
func TestCA287_Hallazgo3229f3_PermutacionesEnDerive(t *testing.T) {
	ev := func() Evidence {
		e := bdEv()
		e.Item.PresupuestoUSD = bdF(o3Presupuesto)
		return e
	}
	for _, cj := range o3Conjuntos() {
		formas := []struct {
			nombre string
			armar  func(orden []int) Evidence
			rwc    int
		}{
			{"solo runs", func(orden []int) Evidence {
				e := ev()
				e.Runs = make([]RunState, 3)
				for i, lugar := range orden {
					e.Runs[lugar] = o3RunEv(fmt.Sprintf("r-%d", i), cj.costos[i], 100*(i+1), 30-10*lugar)
				}
				return e
			}, len(cj.sinCosto)},
			{"un sobre de cada run, en otro orden", func(orden []int) Evidence {
				e := ev()
				for i := 0; i < 3; i++ {
					e.Runs = append(e.Runs, o3RunEv(fmt.Sprintf("r-%d", i), cj.costos[i], 100*(i+1), 30-10*i))
				}
				e.Envelopes = make([]EnvelopeState, 3)
				for i, lugar := range orden {
					e.Envelopes[lugar] = o3SobreEv(fmt.Sprintf("e-%d", i), fmt.Sprintf("r-%d", i),
						o3CopiaUso(e.Runs[i].Meta.Usage), 60-10*lugar)
				}
				return e
			}, len(cj.sinCosto)},
		}
		for k := 0; k < 3; k++ {
			k := k
			formas = append(formas, struct {
				nombre string
				armar  func(orden []int) Evidence
				rwc    int
			}{fmt.Sprintf("un sobre del run %d", k), func(orden []int) Evidence {
				e := ev()
				e.Runs = make([]RunState, 3)
				for i, lugar := range orden {
					e.Runs[lugar] = o3RunEv(fmt.Sprintf("r-%d", i), cj.costos[i], 100*(i+1), 30-10*lugar)
				}
				e.Envelopes = []EnvelopeState{o3SobreEv("e-del-run", fmt.Sprintf("r-%d", k),
					&providers.Usage{CostUSD: bdF(cj.costos[k]), InputTokens: 100 * (k + 1), OutputTokens: 10 * (k + 1)}, 50)}
				return e
			}, len(cj.sinCosto)})
		}
		for k := 0; k < 3; k++ {
			k := k
			// el costo k lo trae un sobre sin sidecar; si es el que no entra,
			// el contrato no dice si ese sobre cuenta en runs: no se exige rwc
			rwc := len(cj.sinCosto)
			if cj.sinCosto[k] {
				rwc = -1
			}
			formas = append(formas, struct {
				nombre string
				armar  func(orden []int) Evidence
				rwc    int
			}{fmt.Sprintf("el costo %d en un sobre sin sidecar", k), func(orden []int) Evidence {
				e := ev()
				lugares := map[int]int{}
				for i, lugar := range orden {
					lugares[i] = lugar
				}
				var runs []int
				for i := 0; i < 3; i++ {
					if i != k {
						runs = append(runs, i)
					}
				}
				// los dos runs, lo mas nuevo primero
				if lugares[runs[0]] > lugares[runs[1]] {
					runs[0], runs[1] = runs[1], runs[0]
				}
				for _, i := range runs {
					e.Runs = append(e.Runs, o3RunEv(fmt.Sprintf("r-%d", i), cj.costos[i], 100*(i+1), 30-10*lugares[i]))
				}
				e.Envelopes = []EnvelopeState{o3SobreEv("e-huerfano", "r-sin-sidecar",
					&providers.Usage{CostUSD: bdF(cj.costos[k]), InputTokens: 100 * (k + 1), OutputTokens: 10 * (k + 1)}, 30-10*lugares[k])}
				return e
			}, rwc})
		}
		for _, f := range formas {
			t.Run(cj.nombre+"/"+f.nombre, func(t *testing.T) {
				var primero *Spend
				var primeroOrden []int
				for _, orden := range o3Permutaciones() {
					card := Derive(f.armar(orden))
					s := card.Spend
					if _, err := json.Marshal(card); err != nil {
						t.Fatalf("CA-288: hallazgo 3229f3: orden %v: la tarjeta se serializa: %v", orden, err)
					}
					if s.CostUSD == nil || *s.CostUSD != cj.total || (f.rwc >= 0 && s.RunsWithoutCost != f.rwc) ||
						s.RemainingUSD == nil || *s.RemainingUSD != o3Presupuesto-cj.total {
						t.Fatalf("CA-287: hallazgo 3229f3: orden %v: se suman de menor a mayor: cost_usd %v, sin costo %d, remaining %v; fue %s",
							orden, cj.total, f.rwc, o3Presupuesto-cj.total, h2FmtSpend(s))
					}
					if primero == nil {
						primero, primeroOrden = &s, orden
					} else if !reflect.DeepEqual(s, *primero) {
						t.Fatalf("CA-287: hallazgo 3229f3: el gasto no depende del orden:\n  orden %v: %s\n  orden %v: %s",
							primeroOrden, h2FmtSpend(*primero), orden, h2FmtSpend(s))
					}
				}
			})
		}
	}

	t.Run("empate de dos 1e308", func(t *testing.T) {
		x := o3RunEv("20261021T160000_empa01", 1e308, 100, 20)
		y := o3RunEv("20261021T160000_empa02", 1e308, 200, 10)
		for _, c := range []struct {
			nombre string
			runs   []RunState
		}{
			{"X mas nuevo", []RunState{x, y}},
			{"Y mas nuevo", func() []RunState {
				x2, y2 := x, y
				x2.Meta.CreatedAt, y2.Meta.CreatedAt = y.Meta.CreatedAt, x.Meta.CreatedAt
				x2.Meta.EndedAt, y2.Meta.EndedAt = y.Meta.EndedAt, x.Meta.EndedAt
				return []RunState{y2, x2}
			}()},
		} {
			e := ev()
			e.Runs = c.runs
			s := Derive(e).Spend
			if s.CostUSD == nil || *s.CostUSD != 1e308 || s.Runs != 2 || s.RunsWithoutCost != 1 || s.RemainingUSD == nil || *s.RemainingUSD != o3Presupuesto-1e308 {
				t.Fatalf("CA-287: hallazgo 3229f3: empate (%s): cost_usd 1e308, runs 2, sin costo 1: %s", c.nombre, h2FmtSpend(s))
			}
		}
	})
}
