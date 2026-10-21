// Tests del hallazgo 20261021T163600_1ab3b0 (medium, reliability) de la delta
// 1 de fixes-cabina-c1b, sobre el gasto de la tarjeta
// (.hoom/specs/tablero-de-solo-lectura.md, CA-314;
// .hoom/specs/items-y-columna-derivada.md, CA-287;
// .hoom/specs/acciones-desde-la-tarjeta.md, CA-327). Escritos desde el
// contrato, nunca desde la implementacion:
//
// El gasto de una tarjeta es el mismo en cualquier orden de sus runs y de sus
// sobres, tambien cuando la suma en float64 de sus costos depende del orden
// aunque no desborde: [1e16, 1, 1] (1e16+1+1 da 1e16 y 1+1+1e16 da
// 10000000000000002) y [0.1, 0.2, 0.3] (0.1+0.2+0.3 y 0.3+0.2+0.1 difieren
// en el ultimo bit). card.spend.cost_usd y remaining_usd son identicos, bit
// a bit, en todas las permutaciones, y siguen siendo la suma de los costos
// (salvo el redondeo de float64) y el presupuesto menos esa suma.
//
// Reusa o3Conjunto, o3Plantar, o3Permutaciones, o3Etiqueta, o3RunEv,
// o3SobreEv y o3Presupuesto de hallazgos_c1b_3229f3_test.go.
package boardcmd

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// abConjuntos son costos validos, finitos y lejos del desborde, cuya suma en
// float64 depende del orden.
func abConjuntos() [][]float64 {
	return [][]float64{
		{1e16, 1, 1},
		{0.1, 0.2, 0.3},
		{1, 1e16, 1},
		{0.3, 0.1, 0.2},
		{1e16, 1, 1, 1},
		{0.1, 0.2, 0.3, 0.4},
		{9007199254740992, 1, 1, 0.5},
	}
}

// abPermutaciones da todas las permutaciones de {0..n-1}, en orden
// lexicografico.
func abPermutaciones(n int) [][]int {
	if n == 3 {
		return o3Permutaciones()
	}
	var out [][]int
	var rec func(p []int, usado []bool)
	rec = func(p []int, usado []bool) {
		if len(p) == n {
			out = append(out, append([]int(nil), p...))
			return
		}
		for i := 0; i < n; i++ {
			if !usado[i] {
				usado[i] = true
				rec(append(p, i), usado)
				usado[i] = false
			}
		}
	}
	rec(nil, make([]bool, n))
	return out
}

// abExacta es la suma exacta de los costos, redondeada una sola vez.
func abExacta(costos []float64) float64 {
	s := new(big.Float).SetPrec(2000)
	for _, c := range costos {
		s.Add(s, new(big.Float).SetPrec(2000).SetFloat64(c))
	}
	f, _ := s.Float64()
	return f
}

// abCerca dice si got es la suma de los costos salvo el redondeo de float64
// (unas pocas unidades del ultimo lugar).
func abCerca(got float64, costos []float64) bool {
	ex := abExacta(costos)
	tol := 0.0
	for _, c := range costos {
		tol += math.Abs(c)
	}
	return math.Abs(got-ex) <= tol*8*0x1p-52
}

// abRevisar exige que s sea el gasto de los costos: cost_usd es la suma
// (salvo redondeo), remaining_usd el presupuesto menos cost_usd.
func abRevisar(t *testing.T, que string, s Spend, costos []float64) {
	t.Helper()
	if s.CostUSD == nil || !abCerca(*s.CostUSD, costos) {
		t.Errorf("CA-287: hallazgo 1ab3b0: %s: cost_usd es la suma de %v (%v salvo redondeo), fue %s",
			que, costos, abExacta(costos), h2FmtF(s.CostUSD))
	}
	if s.CostUSD != nil && (s.RemainingUSD == nil || *s.RemainingUSD != o3Presupuesto-*s.CostUSD) {
		t.Errorf("CA-327: hallazgo 1ab3b0: %s: remaining_usd es el presupuesto menos el gasto: %s", que, h2FmtSpend(s))
	}
}

// abForma arma la Evidence de la tarjeta (presupuesto o3Presupuesto) con los
// costos en el orden orden (orden[i] = lugar del costo i en la lista; lugar
// 0 = lo mas nuevo, con las fechas de acuerdo).
type abForma struct {
	nombre string
	armar  func(costos []float64, orden []int) Evidence
}

func abEv() Evidence {
	e := bdEv()
	e.Item.PresupuestoUSD = bdF(o3Presupuesto)
	return e
}

func abFormas(n int) []abForma {
	fs := []abForma{
		{"solo runs", func(costos []float64, orden []int) Evidence {
			e := abEv()
			e.Runs = make([]RunState, len(costos))
			for i, lugar := range orden {
				e.Runs[lugar] = o3RunEv(fmt.Sprintf("r-%d", i), costos[i], 100*(i+1), 10*len(costos)-10*lugar)
			}
			return e
		}},
		{"un sobre de cada run, los sobres en otro orden", func(costos []float64, orden []int) Evidence {
			e := abEv()
			for i := range costos {
				e.Runs = append(e.Runs, o3RunEv(fmt.Sprintf("r-%d", i), costos[i], 100*(i+1), 10*len(costos)-10*i))
			}
			e.Envelopes = make([]EnvelopeState, len(costos))
			for i, lugar := range orden {
				e.Envelopes[lugar] = o3SobreEv(fmt.Sprintf("e-%d", i), fmt.Sprintf("r-%d", i),
					o3CopiaUso(e.Runs[i].Meta.Usage), 100-10*lugar)
			}
			return e
		}},
	}
	for k := 0; k < n; k++ {
		k := k
		fs = append(fs, abForma{fmt.Sprintf("el costo %d en un sobre sin sidecar", k), func(costos []float64, orden []int) Evidence {
			e := abEv()
			lugarDe := map[int]int{}
			for i, lugar := range orden {
				lugarDe[i] = lugar
			}
			// los runs (todos menos k), lo mas nuevo primero
			var runs []int
			for l := 0; l < len(costos); l++ {
				for i := range costos {
					if i != k && lugarDe[i] == l {
						runs = append(runs, i)
					}
				}
			}
			for _, i := range runs {
				e.Runs = append(e.Runs, o3RunEv(fmt.Sprintf("r-%d", i), costos[i], 100*(i+1), 10*len(costos)-10*lugarDe[i]))
			}
			e.Envelopes = []EnvelopeState{o3SobreEv("e-huerfano", "r-sin-sidecar",
				&providers.Usage{CostUSD: bdF(costos[k]), InputTokens: 100 * (k + 1), OutputTokens: 10 * (k + 1)},
				10*len(costos)-10*lugarDe[k])}
			return e
		}})
	}
	return fs
}

// abMismoEnTodos deriva la tarjeta en cada orden y exige el mismo gasto, bit
// a bit, en todos, y que sea el de los costos.
func abMismoEnTodos(t *testing.T, que string, costos []float64, f abForma) {
	t.Helper()
	var primero *Spend
	var primerOrden []int
	for _, orden := range abPermutaciones(len(costos)) {
		s := Derive(f.armar(costos, orden)).Spend
		if primero == nil {
			primero, primerOrden = &s, orden
			abRevisar(t, fmt.Sprintf("%s, orden %v", que, orden), s, costos)
			continue
		}
		if !reflect.DeepEqual(s, *primero) {
			t.Errorf("CA-314: hallazgo 1ab3b0: %s: el gasto no depende del orden de los runs y los sobres:\n  orden %v: %s\n  orden %v: %s",
				que, primerOrden, h2FmtSpend(*primero), orden, h2FmtSpend(s))
			return
		}
	}
}

// Hallazgo 20261021T163600_1ab3b0. CA-314, CA-287 y CA-327, en memoria
// (Derive, con el orden de las listas bajo control): [1e16, 1, 1],
// [0.1, 0.2, 0.3] y otros costos cuya suma en float64 depende del orden, en
// cada permutacion, solo con runs, con un sobre de cada run en otro orden y
// con cada costo en un sobre sin sidecar. cost_usd y remaining_usd son
// identicos en todas las permutaciones.
func TestCA314_Hallazgo1ab3b0_GastoIgualEnCualquierOrdenEnDerive(t *testing.T) {
	for _, costos := range abConjuntos() {
		for _, f := range abFormas(len(costos)) {
			abMismoEnTodos(t, fmt.Sprintf("%v, %s", costos, f.nombre), costos, f)
		}
	}
}

// Hallazgo 20261021T163600_1ab3b0. CA-314 y CA-287, propiedad: para costos
// al azar (semilla fija) de magnitudes mezcladas, de 1e-6 a 1e17, sin
// desborde, el gasto derivado es el mismo en todas las permutaciones de 3 a
// 5 runs, con y sin sobres.
func TestCA314_Hallazgo1ab3b0_PropiedadGastoIndependienteDelOrden(t *testing.T) {
	rnd := rand.New(rand.NewSource(0x1ab3b0))
	casos := 150
	if testing.Short() {
		casos = 30
	}
	for c := 0; c < casos; c++ {
		n := 3 + rnd.Intn(3)
		costos := make([]float64, n)
		for i := range costos {
			switch rnd.Intn(4) {
			case 0:
				costos[i] = math.Pow(10, float64(rnd.Intn(18))) // potencias de 10 hasta 1e17
			case 1:
				costos[i] = float64(rnd.Intn(1000)) / 10 // decimales como 0.1, 12.3
			case 2:
				costos[i] = rnd.Float64() * math.Pow(10, float64(rnd.Intn(24)-6))
			default:
				costos[i] = costos[rnd.Intn(i+1)] // repetidos: empates
			}
		}
		formas := abFormas(n)
		for _, f := range []abForma{formas[0], formas[1], formas[2+rnd.Intn(n)]} {
			abMismoEnTodos(t, fmt.Sprintf("caso %d %v, %s", c, costos, f.nombre), costos, f)
		}
		if t.Failed() {
			return
		}
	}
}

// Hallazgo 20261021T163600_1ab3b0. CA-314, CA-287 y CA-327, en disco (Gather
// ordena por fechas y antepone los runs que referencian los sobres): las
// tarjetas con [1e16, 1, 1] y [0.1, 0.2, 0.3] en cada uno de los 6 ordenes en
// el tiempo, solo con runs, con cada costo en un sobre sin sidecar y con un
// sobre que referencia el run de cada costo. Para cada forma, el gasto de la
// tarjeta (Build, CardFor y DetailFor) es identico en los 6 ordenes.
func TestCA314_Hallazgo1ab3b0_GastoIgualEnCualquierOrdenEnDisco(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	conjuntos := [][]float64{{1e16, 1, 1}, {0.1, 0.2, 0.3}}
	type plantada struct {
		o3Plantada
		costos []float64
	}
	var ps []plantada
	for ci, costos := range conjuntos {
		cj := o3Conjunto{nombre: fmt.Sprint(costos), costos: costos, sinCosto: map[int]bool{}}
		for _, forma := range []string{"runs", "huerfano", "del-run"} {
			cs := []int{0, 1, 2}
			if forma == "runs" {
				cs = []int{-1}
			}
			for _, c := range cs {
				for pi, orden := range o3Permutaciones() {
					slug := fmt.Sprintf("ab-%d-%s-%d-%d", ci, forma, c+1, pi)
					p := o3Plantar(t, root, slug, o3Etiqueta(1000+len(ps)), now, cj, forma, c, orden)
					ps = append(ps, plantada{p, costos})
				}
			}
		}
	}
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-288: Build: %v", err)
	}
	primero := map[string]plantada{}
	gasto := map[string]Spend{}
	for _, p := range ps {
		s := bdCard(t, b, p.slug).Spend
		abRevisar(t, p.que, s, p.costos)
		c, err := CardFor(root, "main", "high", p.slug, now)
		if err != nil {
			t.Fatalf("CA-313: CardFor(%s): %v", p.slug, err)
		}
		if !reflect.DeepEqual(c.Spend, s) {
			t.Errorf("CA-314: hallazgo 1ab3b0: %s: CardFor da el mismo gasto que Build:\n  Build:   %s\n  CardFor: %s", p.que, h2FmtSpend(s), h2FmtSpend(c.Spend))
		}
		d, err := DetailFor(root, "main", "high", p.slug, now, false)
		if err != nil {
			t.Fatalf("CA-313: DetailFor(%s): %v", p.slug, err)
		}
		if !reflect.DeepEqual(d.Card.Spend, s) {
			t.Errorf("CA-314: hallazgo 1ab3b0: %s: el detalle da el mismo gasto que el tablero:\n  Build:   %s\n  detalle: %s", p.que, h2FmtSpend(s), h2FmtSpend(d.Card.Spend))
		}
		if q, ok := primero[p.grupo]; !ok {
			primero[p.grupo], gasto[p.grupo] = p, s
		} else if !reflect.DeepEqual(s, gasto[p.grupo]) {
			t.Errorf("CA-314: hallazgo 1ab3b0: el gasto no depende del orden en el tiempo:\n  %s: %s\n  %s: %s",
				q.que, h2FmtSpend(gasto[p.grupo]), p.que, h2FmtSpend(s))
		}
	}
}
