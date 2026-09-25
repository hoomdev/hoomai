// Tests de regresion de la review cruzada de la cabina sobre boardcmd:
//
//   - 20260924T201558_d8bf06 (high; C3 CA-327, C1 CA-287): el gasto de la
//     tarjeta nunca cuenta un costo negativo, NaN o infinito de un sidecar de
//     .hoom/runs/<id>.meta.json ni del usage de un registro de sobre. Ese run
//     cuenta como run sin costo reportado (runs_without_cost) y el total no
//     lo suma, asi que spend.remaining_usd no crece plantando un costo
//     negativo. Guardas: con gasto real mayor al presupuesto remaining_usd
//     sigue pudiendo ser negativo, y sin presupuesto es null.
//   - 20260924T200412_092d43 (medium; C1 CA-275, CA-278, CA-288; C2 CA-306;
//     C3 CA-343): un hallazgo que no se puede leer en el arbol de evidencia
//     (truncado, JSON invalido o sin id) no deja que la tarjeta pase de
//     Review: la columna queda en review, missing lo nombra, integrar no esta
//     habilitada, plain cumple CA-306 y Board.Warnings trae el aviso del
//     archivo. Guarda: un hallazgo legible, abierto y high ya la deja en
//     review.
package boardcmd

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// ---------------------------------------------------------------------------
// 20260924T201558_d8bf06: costos invalidos en el gasto de la tarjeta
// ---------------------------------------------------------------------------

// h2Invalidos son los costos que ningun run puede reportar: el gasto de la
// tarjeta los trata como "sin costo reportado".
type h2Invalido struct {
	nombre string
	costo  float64
}

func h2Invalidos() []h2Invalido {
	return []h2Invalido{
		{"negativo -100", -100},
		{"negativo chico", -0.01},
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
		{"-Inf", math.Inf(-1)},
	}
}

func h2Casi(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// h2Run es un run cerrado de la tarjeta (sidecar) con su usage.
func h2Run(id, provider string, costo *float64, in, out int) RunState {
	return RunState{Meta: runcmd.Meta{ID: id, Provider: provider, Role: "writer", Task: bdSlug,
		Status: runcmd.StatusDone, CreatedAt: bdT0.Add(time.Minute), EndedAt: bdT0.Add(2 * time.Minute),
		Usage: &providers.Usage{CostUSD: costo, InputTokens: in, OutputTokens: out}}}
}

// h2SobreSinSidecar es un sobre cerrado de la tarjeta cuyo run no tiene
// sidecar: su propio usage es el que cuenta (CA-287).
func h2SobreSinSidecar(costo *float64) EnvelopeState {
	return EnvelopeState{Record: envelope.Record{ID: "e-sin-sidecar", RunID: "r-sin-sidecar", Role: "writer",
		Provider: "claude", Task: bdSlug, Status: envelope.StatusDeliverable, Stage: "ok", Step: 5, Steps: 5,
		StartedAt: bdT0.Add(3 * time.Minute), UpdatedAt: bdT0.Add(4 * time.Minute), EndedAt: bdT0.Add(4 * time.Minute),
		Usage: &providers.Usage{CostUSD: costo, InputTokens: 10, OutputTokens: 1}}}
}

func h2FmtF(p *float64) string {
	if p == nil {
		return "null"
	}
	return strconv.FormatFloat(*p, 'g', -1, 64)
}

func h2FmtSpend(s Spend) string {
	return "cost_usd=" + h2FmtF(s.CostUSD) + " remaining_usd=" + h2FmtF(s.RemainingUSD) +
		" budget_usd=" + h2FmtF(s.BudgetUSD) + " runs=" + h2Itoa(s.Runs) + " runs_without_cost=" + h2Itoa(s.RunsWithoutCost) +
		" tokens=" + h2Itoa(s.InputTokens) + "/" + h2Itoa(s.OutputTokens)
}

func h2Itoa(n int) string { return strconv.Itoa(n) }

func h2MismoF(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return h2Casi(*a, *b)
}

// h2MismoGasto: un costo invalido cuenta exactamente como un run que no
// reporto costo (su gemelo con cost_usd ausente).
func h2MismoGasto(t *testing.T, ca, nombre string, got, gemelo Spend) {
	t.Helper()
	if !h2MismoF(got.CostUSD, gemelo.CostUSD) || !h2MismoF(got.RemainingUSD, gemelo.RemainingUSD) ||
		!h2MismoF(got.BudgetUSD, gemelo.BudgetUSD) || got.Runs != gemelo.Runs ||
		got.RunsWithoutCost != gemelo.RunsWithoutCost || got.InputTokens != gemelo.InputTokens ||
		got.OutputTokens != gemelo.OutputTokens {
		t.Fatalf("%s: [%s] un costo invalido cuenta como run sin costo reportado:\n con el costo invalido: %s\n con el costo ausente:  %s",
			ca, nombre, h2FmtSpend(got), h2FmtSpend(gemelo))
	}
}

// h2GastoEs exige cost_usd, remaining_usd, runs y runs_without_cost.
func h2GastoEs(t *testing.T, ca, nombre string, s Spend, costo, queda *float64, runs, sinCosto int) {
	t.Helper()
	if !h2MismoF(s.CostUSD, costo) || !h2MismoF(s.RemainingUSD, queda) || s.Runs != runs || s.RunsWithoutCost != sinCosto {
		t.Fatalf("%s: [%s] el gasto debe ser cost_usd=%s remaining_usd=%s runs=%d runs_without_cost=%d, fue %s",
			ca, nombre, h2FmtF(costo), h2FmtF(queda), runs, sinCosto, h2FmtSpend(s))
	}
}

// h2JSONSano: la tarjeta se puede emitir en JSON (hoom board --json, /api/board)
// y no dice NaN ni Inf.
func h2JSONSano(t *testing.T, ca, nombre string, c Card) string {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("%s: [%s] la tarjeta se emite en JSON aunque un sidecar mienta: %v", ca, nombre, err)
	}
	return string(raw)
}

// CA-327 (C3), CA-287 (C1). Hallazgo 20260924T201558_d8bf06: un sidecar con
// cost_usd negativo, NaN o infinito al lado de un run legitimo de 0.9 con
// presupuesto 1 no cuenta: cost_usd 0.9, remaining_usd 0.1 (no 100.1), runs
// 2 y runs_without_cost 1, igual que si ese run no hubiera reportado costo.
func TestCA327_H2_CostoInvalidoDeUnSidecarNoCuenta(t *testing.T) {
	for _, inv := range h2Invalidos() {
		nombre, malo := inv.nombre, inv.costo
		ev := bdEv()
		ev.Item.PresupuestoUSD = bdF(1)
		ev.Runs = []RunState{
			h2Run("r-legit", "claude", bdF(0.9), 1000, 100),
			h2Run("r-malo", "claude", bdF(malo), 20, 2),
		}
		c := Derive(ev)
		h2GastoEs(t, "CA-327", nombre, c.Spend, bdF(0.9), bdF(0.1), 2, 1)
		if c.Spend.InputTokens != 1020 || c.Spend.OutputTokens != 102 {
			t.Fatalf("CA-287: [%s] los tokens del run con costo invalido siguen sumando: %s", nombre, h2FmtSpend(c.Spend))
		}

		gemelo := bdEv()
		gemelo.Item.PresupuestoUSD = bdF(1)
		gemelo.Runs = []RunState{
			h2Run("r-legit", "claude", bdF(0.9), 1000, 100),
			h2Run("r-malo", "claude", nil, 20, 2),
		}
		h2MismoGasto(t, "CA-327", nombre, c.Spend, Derive(gemelo).Spend)
		h2JSONSano(t, "CA-327", nombre, c)
	}
}

// CA-327 (C3), CA-287 (C1). Hallazgo 20260924T201558_d8bf06: lo mismo con el
// usage de un registro de sobre cuyo run no tiene sidecar (el que CA-287
// suma): un costo invalido ahi tampoco entra al total ni infla lo que queda.
func TestCA327_H2_CostoInvalidoDeUnSobreNoCuenta(t *testing.T) {
	for _, inv := range h2Invalidos() {
		nombre, malo := inv.nombre, inv.costo
		ev := bdEv()
		ev.Item.PresupuestoUSD = bdF(1)
		ev.Runs = []RunState{h2Run("r-legit", "claude", bdF(0.9), 1000, 100)}
		ev.Envelopes = []EnvelopeState{h2SobreSinSidecar(bdF(malo))}
		c := Derive(ev)
		if c.Spend.CostUSD == nil || !h2Casi(*c.Spend.CostUSD, 0.9) ||
			c.Spend.RemainingUSD == nil || !h2Casi(*c.Spend.RemainingUSD, 0.1) {
			t.Fatalf("CA-327: [%s] el costo invalido del sobre no suma: cost_usd 0.9 y remaining_usd 0.1, fue %s",
				nombre, h2FmtSpend(c.Spend))
		}

		gemelo := bdEv()
		gemelo.Item.PresupuestoUSD = bdF(1)
		gemelo.Runs = []RunState{h2Run("r-legit", "claude", bdF(0.9), 1000, 100)}
		gemelo.Envelopes = []EnvelopeState{h2SobreSinSidecar(nil)}
		h2MismoGasto(t, "CA-327", nombre, c.Spend, Derive(gemelo).Spend)
		h2JSONSano(t, "CA-327", nombre, c)
	}
}

// CA-287 (C1), CA-327 (C3). Hallazgo 20260924T201558_d8bf06: si el unico
// costo "reportado" es invalido, nadie reporto costo: cost_usd null (nunca un
// numero inventado) y remaining_usd es el presupuesto entero.
func TestCA287_H2_SoloCostosInvalidosEsSinCosto(t *testing.T) {
	for _, inv := range h2Invalidos() {
		nombre, malo := inv.nombre, inv.costo
		ev := bdEv()
		ev.Item.PresupuestoUSD = bdF(1)
		ev.Runs = []RunState{h2Run("r-malo", "claude", bdF(malo), 5, 5)}
		c := Derive(ev)
		h2GastoEs(t, "CA-287", nombre, c.Spend, nil, bdF(1), 1, 1)
		raw := h2JSONSano(t, "CA-287", nombre, c)
		if !strings.Contains(raw, `"cost_usd":null`) {
			t.Fatalf("CA-287: [%s] sin ningun costo valido, \"cost_usd\":null en JSON: %s", nombre, raw)
		}
	}
}

// CA-287 (C1), CA-327 (C3). Hallazgo 20260924T201558_d8bf06, como propiedad:
// para cualquier mezcla de runs (sin costo, NaN, +Inf, -Inf, negativos y
// validos), cost_usd es la suma de los costos finitos y no negativos (null si
// no hay ninguno), runs_without_cost cuenta todos los demas, y remaining_usd
// es el presupuesto menos esa suma.
func TestCA327_H2_PropiedadSoloSumanCostosValidos(t *testing.T) {
	prop := func(codigos []uint8, centavos []int16) bool {
		n := len(codigos)
		if len(centavos) < n {
			n = len(centavos)
		}
		ev := bdEv()
		ev.Item.PresupuestoUSD = bdF(10)
		suma, validos, sinCosto := 0.0, 0, 0
		for i := 0; i < n; i++ {
			var costo *float64
			switch codigos[i] % 6 {
			case 0: // no reporta costo
			case 1:
				costo = bdF(math.NaN())
			case 2:
				costo = bdF(math.Inf(1))
			case 3:
				costo = bdF(math.Inf(-1))
			default: // un monto cualquiera, negativo o no
				costo = bdF(float64(centavos[i]) / 100)
			}
			if costo != nil && !math.IsNaN(*costo) && !math.IsInf(*costo, 0) && *costo >= 0 {
				suma += *costo
				validos++
			} else {
				sinCosto++
			}
			ev.Runs = append(ev.Runs, h2Run("r-"+h2Itoa(i), "claude", costo, 1, 1))
		}
		s := Derive(ev).Spend
		if s.Runs != n || s.RunsWithoutCost != sinCosto {
			t.Logf("runs=%d sin costo=%d, fue %s", n, sinCosto, h2FmtSpend(s))
			return false
		}
		if validos == 0 {
			if s.CostUSD != nil {
				t.Logf("sin costos validos cost_usd es null, fue %s", h2FmtSpend(s))
				return false
			}
		} else if s.CostUSD == nil || math.Abs(*s.CostUSD-suma) > 1e-6 {
			t.Logf("cost_usd debe ser %v, fue %s", suma, h2FmtSpend(s))
			return false
		}
		if s.RemainingUSD == nil || math.Abs(*s.RemainingUSD-(10-suma)) > 1e-6 {
			t.Logf("remaining_usd debe ser %v, fue %s", 10-suma, h2FmtSpend(s))
			return false
		}
		if _, err := json.Marshal(s); err != nil {
			t.Logf("el gasto se emite en JSON: %v", err)
			return false
		}
		return true
	}
	cfg := &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(20260924))}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("CA-327: solo suman los costos finitos y no negativos: %v", err)
	}
}

// h2IDRun arma un id de run/sobre con la forma de los de hoom.
func h2IDRun(sufijo string) string { return "20260924T100000_" + sufijo }

// h2GastoDisco planta en el disco del proyecto, para la tarjeta slug (con
// presupuesto 1), un run legitimo de 0.9 y, segun donde, un sidecar o un
// sobre (cuyo run no tiene sidecar) con el costo dado.
func h2GastoDisco(t *testing.T, root, slug, sufijo, donde string, costo *float64, now time.Time) {
	t.Helper()
	bdItemArchivo(t, root, slug, "Gasto "+slug, "presupuesto_usd: 1\n")
	bdRunDisco(t, root, runcmd.Meta{ID: h2IDRun("leg" + sufijo), Provider: "claude", Role: "writer", Task: slug, Dir: root,
		CreatedAt: now.Add(-2 * time.Hour), EndedAt: now.Add(-110 * time.Minute), Status: runcmd.StatusDone,
		Usage: &providers.Usage{CostUSD: bdF(0.9), InputTokens: 1000, OutputTokens: 100}})
	switch donde {
	case "sidecar":
		bdRunDisco(t, root, runcmd.Meta{ID: h2IDRun("mal" + sufijo), Provider: "claude", Role: "writer", Task: slug, Dir: root,
			CreatedAt: now.Add(-time.Hour), EndedAt: now.Add(-50 * time.Minute), Status: runcmd.StatusDone,
			Usage: &providers.Usage{CostUSD: costo, InputTokens: 20, OutputTokens: 2}})
	case "sobre":
		bdSobreDisco(t, root, envelope.Record{ID: h2IDRun("sob" + sufijo), Role: "writer", Provider: "claude", Task: slug,
			Dir: root, RunID: h2IDRun("per" + sufijo), Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
			Usage:     &providers.Usage{CostUSD: costo, InputTokens: 20, OutputTokens: 2},
			StartedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-40 * time.Minute), EndedAt: now.Add(-40 * time.Minute)})
	default:
		t.Fatalf("h2GastoDisco: donde %q", donde)
	}
}

// CA-327 (C3), CA-287 (C1). Hallazgo 20260924T201558_d8bf06, en disco: plantar
// .hoom/runs/<id>.meta.json con usage.cost_usd -100 (o un sobre con ese
// usage) al lado de un run legitimo de 0.9, con presupuesto 1, deja
// remaining_usd 0.1 y no 100.1, en CardFor y en Build (lo que emiten
// hoom item show --json, hoom board --json y /api/board).
func TestCA327_H2_CostoNegativoPlantadoEnDisco(t *testing.T) {
	for _, donde := range []string{"sidecar", "sobre"} {
		t.Run(donde, func(t *testing.T) {
			root := bdRepo(t, "")
			now := time.Now().UTC()
			h2GastoDisco(t, root, "plantado", "001", donde, bdF(-100), now)
			h2GastoDisco(t, root, "gemelo", "002", donde, nil, now)

			c, err := CardFor(root, "main", "high", "plantado", now)
			if err != nil {
				t.Fatal(err)
			}
			if c.Spend.RemainingUSD == nil || !h2Casi(*c.Spend.RemainingUSD, 0.1) ||
				c.Spend.CostUSD == nil || !h2Casi(*c.Spend.CostUSD, 0.9) {
				t.Fatalf("CA-327: [%s] con cost_usd -100 plantado, presupuesto 1 y un run de 0.9: remaining_usd 0.1 (no 100.1) y cost_usd 0.9, fue %s",
					donde, h2FmtSpend(c.Spend))
			}
			if donde == "sidecar" && (c.Spend.Runs != 2 || c.Spend.RunsWithoutCost != 1) {
				t.Fatalf("CA-287: [%s] el run con costo negativo es un run sin costo reportado: runs 2, runs_without_cost 1, fue %s",
					donde, h2FmtSpend(c.Spend))
			}
			g, err := CardFor(root, "main", "high", "gemelo", now)
			if err != nil {
				t.Fatal(err)
			}
			h2MismoGasto(t, "CA-327", donde, c.Spend, g.Spend)

			b, err := Build(root, "main", "high", now)
			if err != nil {
				t.Fatal(err)
			}
			bc := bdCard(t, b, "plantado")
			if bc.Spend.RemainingUSD == nil || !h2Casi(*bc.Spend.RemainingUSD, 0.1) {
				t.Fatalf("CA-327: [%s] Build da el mismo remaining_usd 0.1: %s", donde, h2FmtSpend(bc.Spend))
			}
			if raw := h2JSONSano(t, "CA-327", donde, bc); strings.Contains(raw, "100.1") || strings.Contains(raw, "-99.1") {
				t.Fatalf("CA-327: [%s] el JSON de la tarjeta no refleja el costo plantado: %s", donde, raw)
			}
		})
	}
}

// GUARDA (verde hoy). CA-327 (C3): con gasto REAL mayor al presupuesto,
// remaining_usd sigue siendo negativo; sin presupuesto es null; y un costo
// reportado de 0 es un costo reportado (cuenta, no es "sin costo").
func TestCA327_H2_GuardaGastoRealNegativoYSinPresupuesto(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	bdItemArchivo(t, root, "de-mas", "De mas", "presupuesto_usd: 1\n")
	bdItemArchivo(t, root, "sin-tope", "Sin tope", "")
	for i, r := range []struct {
		slug  string
		costo float64
	}{{"de-mas", 0.9}, {"de-mas", 0.6}, {"sin-tope", 0.9}} {
		bdRunDisco(t, root, runcmd.Meta{ID: h2IDRun("gua00" + h2Itoa(i)), Provider: "claude", Role: "writer", Task: r.slug,
			Dir: root, CreatedAt: now.Add(-time.Hour), EndedAt: now.Add(-50 * time.Minute), Status: runcmd.StatusDone,
			Usage: &providers.Usage{CostUSD: bdF(r.costo), InputTokens: 1, OutputTokens: 1}})
	}
	c, err := CardFor(root, "main", "high", "de-mas", now)
	if err != nil {
		t.Fatal(err)
	}
	h2GastoEs(t, "CA-327", "gastado de mas", c.Spend, bdF(1.5), bdF(-0.5), 2, 0)
	c, err = CardFor(root, "main", "high", "sin-tope", now)
	if err != nil {
		t.Fatal(err)
	}
	h2GastoEs(t, "CA-327", "sin presupuesto", c.Spend, bdF(0.9), nil, 1, 0)

	// un 0 reportado es un dato, no la falta de dato (CA-287)
	ev := bdEv()
	ev.Item.PresupuestoUSD = bdF(1)
	ev.Runs = []RunState{h2Run("r-cero", "claude", bdF(0), 1, 1)}
	h2GastoEs(t, "CA-287", "costo 0 reportado", Derive(ev).Spend, bdF(0), bdF(1), 1, 0)
}

// ---------------------------------------------------------------------------
// 20260924T200412_092d43: un hallazgo ilegible no deja pasar de Review
// ---------------------------------------------------------------------------

// h2Aceptable arma el escenario de CA-277: la tarea con su worktree limpio y
// verde, con la tarjeta en Tu aceptacion, y el item en el arbol raiz.
func h2Aceptable(t *testing.T) (root, wt string) {
	t.Helper()
	root = bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt = bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 1 }\n")
	bdAprobar(t, wt, bdSlug)
	bdVeredictoDisco(t, wt, bdSpec, bdT0.Add(time.Minute), false, bdGatesVerdes())
	bdCommitear(t, wt, "spec, codigo y veredicto")
	return root, wt
}

// h2Limpio exige que el worktree no tenga nada sin commitear (la condicion
// de cierre de taskcmd.Ready se cumple).
func h2Limpio(t *testing.T, wt string) {
	t.Helper()
	if st := bdGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("fixture: el worktree debe quedar limpio: %s", st)
	}
}

// h2HallazgoDeLaTarea registra en el worktree un hallazgo de la tarjeta y lo
// commitea. Devuelve la ruta del archivo.
func h2HallazgoDeLaTarea(t *testing.T, wt, sev string) (string, finding.Finding) {
	t.Helper()
	f, err := finding.Register(wt, "main", finding.Draft{Severity: sev, Lens: "risk", Description: "hallazgo de la tarjeta",
		Author: "reviewer", Task: bdSlug})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(wt, ".hoom", "findings", f.ID+".json")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture: el hallazgo vive en .hoom/findings/<id>.json: %v", err)
	}
	bdCommitear(t, wt, "hallazgo "+f.ID)
	return p, f
}

// h2Ilegible: la tarjeta no pasa de Review y lo dice.
func h2Ilegible(t *testing.T, ca, nombre string, c Card, archivo string) {
	t.Helper()
	bdCol(t, ca+" ["+nombre+"]", c, ColReview)
	nombra := false
	for _, m := range c.Missing {
		bajo := strings.ToLower(m)
		if strings.Contains(bajo, "hallazgo") &&
			(strings.Contains(bajo, "leer") || strings.Contains(bajo, "legible") || strings.Contains(bajo, "corrupt") ||
				strings.Contains(m, archivo)) {
			nombra = true
		}
	}
	if !nombra {
		t.Fatalf("%s: [%s] missing nombra que hay hallazgos que no se pueden leer: %q", ca, nombre, c.Missing)
	}
	if a, ok := c.Action("integrar"); ok && a.Enabled {
		t.Fatalf("CA-343: [%s] integrar (la accion de Tu aceptacion) no esta habilitada con un hallazgo ilegible: %+v", nombre, a)
	}
	for _, a := range c.Actions {
		if a.ID == "integrar" && a.Enabled {
			t.Fatalf("CA-343: [%s] ninguna accion integrar habilitada: %+v", nombre, c.Actions)
		}
	}
	if c.WaitingHuman {
		t.Fatalf("CA-284: [%s] en Review la tarjeta no espera la aceptacion humana", nombre)
	}
	tbNormal(t, "CA-306 ["+nombre+"]", "plain", c.Plain)
	if c.Plain == "espera tu aceptacion para integrar" {
		t.Fatalf("CA-306: [%s] plain no dice que espera la aceptacion: %q", nombre, c.Plain)
	}
}

// CA-275, CA-278 (C1), CA-288 (C1), CA-306 (C2), CA-343 (C3). Hallazgo
// 20260924T200412_092d43: en el escenario de CA-277 (worktree limpio y verde,
// tarjeta en Tu aceptacion), un hallazgo de la tarea que queda truncado, con
// JSON invalido o sin id, y commiteado, deja la tarjeta en Review: missing lo
// nombra, integrar no esta habilitada, plain cumple CA-306 y Board.Warnings
// (Build) trae el aviso de ese archivo. El severity del hallazgo no importa:
// ilegible es ilegible (low tambien bloquea).
func TestCA275_H2_HallazgoIlegibleNoPasaDeReview(t *testing.T) {
	type rotura struct {
		nombre string
		sev    string
		romper func(t *testing.T, p string)
	}
	truncar := func(t *testing.T, p string) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, raw[:len(raw)/2], 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []rotura{
		{"truncado high", "high", truncar},
		{"truncado low", "low", truncar},
		{"json invalido", "low", func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte("{roto"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"sin id", "low", func(t *testing.T, p string) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("fixture: el hallazgo recien registrado es JSON: %v", err)
			}
			if _, ok := m["id"]; !ok {
				t.Fatalf("fixture: el hallazgo trae la clave id: %s", raw)
			}
			delete(m, "id")
			out, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(out, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(r.nombre, func(t *testing.T) {
			root, wt := h2Aceptable(t)
			now := time.Now().UTC()
			p, f := h2HallazgoDeLaTarea(t, wt, r.sev)
			archivo := f.ID + ".json"

			if r.sev == "low" {
				// legible y low: no bloquea con block_on high, la tarjeta
				// esta en Tu aceptacion (el fixture es el de CA-277)
				bdCol(t, "CA-278 (fixture legible)", Derive(Gather(root, "main", "high", bdItem(bdSlug), now)), ColTuAceptacion)
			}

			r.romper(t, p)
			bdCommitear(t, wt, "hallazgo roto")
			h2Limpio(t, wt)

			c := Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
			h2Ilegible(t, "CA-275", r.nombre, c, archivo)

			c, err := CardFor(root, "main", "high", bdSlug, now)
			if err != nil {
				t.Fatal(err)
			}
			h2Ilegible(t, "CA-275 (CardFor)", r.nombre, c, archivo)

			b, err := Build(root, "main", "high", now)
			if err != nil {
				t.Fatal(err)
			}
			h2Ilegible(t, "CA-275 (Build)", r.nombre, bdCard(t, b, bdSlug), archivo)
			avisa := false
			for _, w := range b.Warnings {
				if strings.Contains(w, archivo) {
					avisa = true
				}
			}
			if !avisa {
				t.Fatalf("CA-288: [%s] Board.Warnings trae el aviso del hallazgo ilegible %s: %q", r.nombre, archivo, b.Warnings)
			}
		})
	}
}

// GUARDA (verde hoy). CA-275 (C1): en el mismo escenario, un hallazgo
// LEGIBLE, abierto y high de la tarea ya deja la tarjeta en Review, sin
// avisos de archivos ilegibles.
func TestCA275_H2_GuardaHallazgoLegibleHighYaEsReview(t *testing.T) {
	root, wt := h2Aceptable(t)
	now := time.Now().UTC()
	bdCol(t, "CA-278 (fixture)", Derive(Gather(root, "main", "high", bdItem(bdSlug), now)), ColTuAceptacion)
	_, f := h2HallazgoDeLaTarea(t, wt, "high")
	h2Limpio(t, wt)
	c := Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
	bdCol(t, "CA-275", c, ColReview)
	bdTiene(t, "CA-275", c.Missing, f.ID)
	if a, ok := c.Action("integrar"); ok && a.Enabled {
		t.Fatalf("CA-343: integrar no esta habilitada con un hallazgo que bloquea: %+v", a)
	}
	tbNormal(t, "CA-306", "plain", c.Plain)
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatal(err)
	}
	bdCol(t, "CA-275 (Build)", bdCard(t, b, bdSlug), ColReview)
	for _, w := range b.Warnings {
		if strings.Contains(w, f.ID) {
			t.Fatalf("CA-288: un hallazgo legible no deja aviso: %q", b.Warnings)
		}
	}
}
