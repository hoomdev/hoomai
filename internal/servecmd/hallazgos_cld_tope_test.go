// Tests de regresion de los hallazgos c155a2, f58e6a, a3a4d8, 56c600,
// 75a10b, 6d82f5 y faf4f5 sobre .hoom/specs/ciclo-lo-decide-hoom.md (CA-750
// y CA-751): el piloto decide con el mismo tope que muestra la tarjeta, por
// el camino del Studio.
//
//   - c155a2 y f58e6a: el Studio carga su Manifest al arrancar; si una
//     persona sube (o baja) findings.max_rondas con el Studio abierto, el
//     piloto del Studio usa el tope que la tarjeta acaba de leer del
//     hoom.yaml, no el viejo.
//   - a3a4d8: sin findings.block_on la tarjeta juzga con "high" y queda al
//     tope; el piloto para con su razon, tambien con un writer cortado.
//   - 56c600: una delta que llega entre la decision del piloto y el candado
//     de lanzar.Lanzar gana: con Pilot, Lanzar no lanza al writer ni al
//     refutador de una tarjeta al tope.
//   - 75a10b, 6d82f5 y faf4f5: tampoco al test-writer ni al reviewer, que
//     la tarjeta al tope sigue ofreciendo: al tope el piloto no lanza
//     ningun rol.
//
// Fixtures de tablero_test.go (tbProyecto, tbEscribir, tbItem),
// lanzar_test.go (lnPATH, lnServidor, lnCommit, lnEsperarLanzamientos,
// lnBase), piloto_test.go (spAuto, spPresupuesto) y
// ciclo_lo_decide_hoom_test.go (cldCartaReview). El CLI de IA es falso.
package servecmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

const rgtSlug = "precios"

// rgtYAML reescribe el hoom.yaml del proyecto con findings (extra) y lo
// guarda.
func rgtYAML(t *testing.T, root, extra string) {
	t.Helper()
	tbEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+extra)
	lnCommit(t, root, "hoom.yaml: "+strings.ReplaceAll(extra, "\n", " "))
}

// rgtAuto le pone auto hasta-humano y presupuesto a la tarjeta.
func rgtAuto(t *testing.T, root string) {
	t.Helper()
	tbItem(t, root, rgtSlug, spAuto+spPresupuesto)
	lnCommit(t, root, "item con auto")
}

// rgtWriterCortado escribe en la raiz un sobre cerrado de writer cortado por
// tiempo, de hace hora y media: el piloto lo releva (caso 5) bajo el tope.
func rgtWriterCortado(t *testing.T, root, wt string) {
	t.Helper()
	ini := time.Now().UTC().Add(-90 * time.Minute).Truncate(time.Second)
	rec := envelope.Record{ID: "rgt-writer-cortado", Role: "writer", Provider: "claude", Task: rgtSlug, Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", rgtSlug+".md"), Stage: "run", Step: 3, Steps: 6,
		Status: envelope.StatusNotDeliverable, Corte: envelope.CorteTiempo, StartedAt: ini, UpdatedAt: ini.Add(5 * time.Minute),
		EndedAt: ini.Add(5 * time.Minute), Columna: boardcmd.ColReview, Pedido: "Corregi los hallazgos que bloquean la tarjeta."}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, root, ".hoom/"+envelope.DirName+"/"+rec.ID+".json", string(raw)+"\n")
}

// rgtDelta registra en el espacio de trabajo una review delta mas (la
// ronda i, despues de las de cldCartaReview) y la guarda.
func rgtDelta(t *testing.T, wt string, i int) {
	t.Helper()
	ini := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	if _, err := reviewcmd.WriteRecord(wt, reviewcmd.Record{CreatedAt: ini.Add(time.Duration(i) * time.Minute), Task: rgtSlug,
		Spec: ".hoom/specs/" + rgtSlug + ".md", Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes,
		Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}, Usage: []reviewcmd.LensUsage{},
		Cobertura: reviewcmd.CoberturaDelta, Base: lnBase, Desde: strings.Repeat("a", 40), Hasta: strings.Repeat("b", 40)}); err != nil {
		t.Fatal(err)
	}
	lnCommit(t, wt, fmt.Sprintf("delta %d", i))
}

// rgtTarjeta es la tarjeta como la deriva el Studio, con su Manifest.
func rgtTarjeta(t *testing.T, s *Server) boardcmd.Card {
	t.Helper()
	c, err := boardcmd.CardFor(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), rgtSlug, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// rgtEstado es lo que ve el piloto del Studio (s.piloto) y lo que decide.
func rgtEstado(t *testing.T, s *Server) (boardcmd.Estado, boardcmd.PilotDecision) {
	t.Helper()
	e, err := pilotocmd.Armar(s.piloto(rgtSlug))
	if err != nil {
		t.Fatalf("fixture: pilotocmd.Armar: %v", err)
	}
	return e, boardcmd.Piloto(e)
}

// rgtNoLanza exige que una vuelta del piloto del Studio pare con razon sin
// llamar al CLI.
func rgtNoLanza(t *testing.T, ca string, s *Server, f *lnFakes, razon string) {
	t.Helper()
	o := s.piloto(rgtSlug)
	o.Lanzar.Despues = nil // una sola vuelta
	res, err := pilotocmd.Avanzar(context.Background(), o)
	if err != nil {
		t.Fatalf("%s: pilotocmd.Avanzar: %v", ca, err)
	}
	lnEsperarLanzamientos(t, ca, s)
	if n := len(f.llamadas(t, "claude")); n != 0 {
		t.Fatalf("%s: al tope el piloto del Studio no lanza nada; el CLI se llamo %d veces (lineas %q)", ca, n, res.Lineas)
	}
	quiero := "piloto automatico: se detiene: " + razon
	if len(res.Lineas) != 1 || res.Lineas[0] != quiero {
		t.Fatalf("%s: el piloto del Studio dice %q, fue %q", ca, quiero, res.Lineas)
	}
}

// c155a2 y f58e6a: con el Studio abierto, una persona sube
// findings.max_rondas de 2 (por defecto) a 3. La tarjeta sale del tope y el
// piloto del Studio decide con el mismo tope: no para por el viejo.
func TestHallazgoC155a2_ElPilotoDelStudioSubeConElTope(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t) // findings.block_on: high, sin max_rondas: tope 2
	cldCartaReview(t, "c155a2", root, rgtSlug, 2, 2)
	rgtAuto(t, root)
	s := lnServidor(t, root, f)
	razon := boardcmd.RazonTopeDeRondas(2, 2)
	if c := rgtTarjeta(t, s); !c.Rondas.AlTope || c.Rondas.Tope != 2 {
		t.Fatalf("c155a2: fixture: con dos rondas y tope 2 la tarjeta esta al tope: %+v", c.Rondas)
	}
	if e, d := rgtEstado(t, s); e.TopeDeRondas != 2 || d.Why != razon || d.Caso != 7 {
		t.Fatalf("c155a2: fixture: al tope el piloto para con %q: tope %d, %+v", razon, e.TopeDeRondas, d)
	}

	rgtYAML(t, root, "findings:\n  block_on: high\n  max_rondas: 3\n")
	c := rgtTarjeta(t, s)
	if c.Rondas.AlTope || c.Rondas.Tope != 3 {
		t.Fatalf("c155a2: subido el tope a 3, la tarjeta sale del tope: %+v", c.Rondas)
	}
	e, d := rgtEstado(t, s)
	if e.TopeDeRondas != 3 || e.TopeDeRondas != e.Tarjeta.Rondas.Tope {
		t.Fatalf("c155a2: el piloto del Studio decide con el tope de la tarjeta (3), fue %d (tarjeta %d)", e.TopeDeRondas, e.Tarjeta.Rondas.Tope)
	}
	if strings.HasPrefix(d.Why, "se hicieron ") {
		t.Fatalf("c155a2: subido el tope, el piloto del Studio no para por el viejo: %+v", d)
	}
}

// f58e6a: con el Studio abierto, una persona baja findings.max_rondas de 3
// a 2 con un writer cortado. La tarjeta queda al tope y el piloto del Studio
// decide con ese tope: no releva al writer.
func TestHallazgoF58e6a_ElPilotoDelStudioBajaConElTope(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	rgtYAML(t, root, "findings:\n  block_on: high\n  max_rondas: 3\n")
	wt, _ := cldCartaReview(t, "f58e6a", root, rgtSlug, 2, 2)
	rgtAuto(t, root)
	rgtWriterCortado(t, root, wt)
	s := lnServidor(t, root, f)
	if _, d := rgtEstado(t, s); !d.Launch || d.Action != boardcmd.ActPedirWriter || d.Regla != boardcmd.ReglaRelevoPorTiempo {
		t.Fatalf("f58e6a: fixture: bajo el tope 3 el piloto releva al writer cortado: %+v", d)
	}

	rgtYAML(t, root, "findings:\n  block_on: high\n  max_rondas: 2\n")
	razon := boardcmd.RazonTopeDeRondas(2, 2)
	if c := rgtTarjeta(t, s); !c.Rondas.AlTope || c.Rondas.Razon != razon {
		t.Fatalf("f58e6a: bajado el tope a 2, la tarjeta queda al tope: %+v", c.Rondas)
	}
	e, d := rgtEstado(t, s)
	if e.TopeDeRondas != 2 || e.TopeDeRondas != e.Tarjeta.Rondas.Tope {
		t.Fatalf("f58e6a: el piloto del Studio decide con el tope de la tarjeta (2), fue %d (tarjeta %d)", e.TopeDeRondas, e.Tarjeta.Rondas.Tope)
	}
	if d.Launch || d.Why != razon || d.Caso != 7 {
		t.Fatalf("f58e6a: al tope el piloto del Studio para con %q, fue %+v", razon, d)
	}
	rgtNoLanza(t, "f58e6a", s, f, razon)
}

// a3a4d8: sin findings.block_on (max_rondas no lo exige) la tarjeta juzga
// con "high", y con dos rondas y dos high abiertos queda al tope. El piloto
// para con la razon de la tarjeta, tambien con un writer cortado que sin el
// tope relevaria.
func TestHallazgoA3a4d8_SinBlockOnElPilotoParaAlTope(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	rgtYAML(t, root, "")
	wt, _ := cldCartaReview(t, "a3a4d8", root, rgtSlug, 2, 2)
	rgtAuto(t, root)
	rgtWriterCortado(t, root, wt)
	s := lnServidor(t, root, f)
	if s.m.FindingsBlockOn() != "" {
		t.Fatalf("a3a4d8: fixture: el hoom.yaml no declara findings.block_on, fue %q", s.m.FindingsBlockOn())
	}
	razon := boardcmd.RazonTopeDeRondas(2, 2)
	if c := rgtTarjeta(t, s); !c.Rondas.AlTope || c.Rondas.Razon != razon {
		t.Fatalf("a3a4d8: fixture: sin block_on la tarjeta juzga con high y queda al tope: %+v", c.Rondas)
	}
	if _, d := rgtEstado(t, s); d.Launch || d.Why != razon || d.Caso != 7 {
		t.Fatalf("a3a4d8: al tope el piloto para con la razon de la tarjeta %q, fue %+v", razon, d)
	}
	rgtNoLanza(t, "a3a4d8", s, f, razon)
}

// 56c600: el piloto decide relevar al writer con dos rondas y tope 3; antes
// de que lance llega la tercera delta. Bajo el candado lanzar.Lanzar relee
// la tarjeta, que ya esta al tope: con Pilot no lanza al writer ni al
// refutador (409 con la razon) y el CLI no se llama.
func TestHallazgo56c600_ElCandadoNoLanzaPorEncimaDelTope(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	rgtYAML(t, root, "findings:\n  block_on: high\n  max_rondas: 3\n")
	wt, ids := cldCartaReview(t, "56c600", root, rgtSlug, 2, 2)
	rgtAuto(t, root)
	rgtWriterCortado(t, root, wt)
	e, err := pilotocmd.Armar(pilotocmd.Opciones{Root: root, Slug: rgtSlug})
	if err != nil {
		t.Fatal(err)
	}
	d := boardcmd.Piloto(e)
	if !d.Launch || d.Action != boardcmd.ActPedirWriter || d.Regla != boardcmd.ReglaRelevoPorTiempo {
		t.Fatalf("56c600: fixture: con dos rondas y tope 3 el piloto releva al writer: %+v", d)
	}

	rgtDelta(t, wt, 3) // la tercera ronda: 3 >= tope 3
	s := lnServidor(t, root, f)
	razon := boardcmd.RazonTopeDeRondas(3, 2)
	if c := rgtTarjeta(t, s); !c.Rondas.AlTope || c.Rondas.Razon != razon {
		t.Fatalf("56c600: fixture: tras la delta la tarjeta esta al tope: %+v", c.Rondas)
	}
	p := s.piloto(rgtSlug).Lanzar
	p.Despues = nil
	writer := lanzar.Request{Action: d.Action, Provider: d.Provider, Pedido: d.Pedido, Pilot: true, Resume: d.Resume,
		Delta: d.Delta, Regla: d.Regla, Intento: d.Intento, Ronda: d.Ronda, Hallazgos: d.Hallazgos, Bloqueantes: d.Bloqueantes}
	if d.BudgetUSD > 0 {
		b := d.BudgetUSD
		writer.BudgetUSD = &b
	}
	refutador := lanzar.Request{Action: boardcmd.ActPedirRefutador, Provider: "claude", Pilot: true,
		Pedido: boardcmd.PedidoRondaRefutador(3, rgtSlug, ids), Regla: boardcmd.ReglaRondaRefutador, Ronda: 3,
		Hallazgos: ids, Bloqueantes: len(ids)}
	quiero := razon + ": el piloto no lanza nada"
	for _, req := range []lanzar.Request{writer, refutador} {
		tr, err := lanzar.Lanzar(context.Background(), p, rgtSlug, req)
		var rech *lanzar.Rechazo
		if tr != nil || !errors.As(err, &rech) || rech.Code != http.StatusConflict || rech.Motivo != quiero {
			t.Fatalf("56c600: al tope, bajo el candado, el piloto no lanza %s: 409 %q; fue trabajo=%v err=%v", req.Action, quiero, tr != nil, err)
		}
	}
	lnEsperarLanzamientos(t, "56c600", s)
	if n := len(f.llamadas(t, "claude")); n != 0 {
		t.Fatalf("56c600: al tope el CLI no se llama, se llamo %d veces", n)
	}
}

// rgtCortado escribe en la raiz un sobre cerrado del rol role, de provider,
// cortado por tiempo hace hora y media: el piloto lo releva (caso 5) bajo el
// tope.
func rgtCortado(t *testing.T, root, wt, role, provider, pedido string) {
	t.Helper()
	ini := time.Now().UTC().Add(-90 * time.Minute).Truncate(time.Second)
	rec := envelope.Record{ID: "rgt-" + role + "-cortado", Role: role, Provider: provider, Task: rgtSlug, Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", rgtSlug+".md"), Stage: "run", Step: 3, Steps: 6,
		Status: envelope.StatusNotDeliverable, Corte: envelope.CorteTiempo, StartedAt: ini, UpdatedAt: ini.Add(5 * time.Minute),
		EndedAt: ini.Add(5 * time.Minute), Columna: boardcmd.ColReview, Pedido: pedido}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, root, ".hoom/"+envelope.DirName+"/"+rec.ID+".json", string(raw)+"\n")
}

// 75a10b, 6d82f5 y faf4f5: como en 56c600, el piloto decide con dos rondas
// y tope 3 relevar a un rol cortado, y antes de que lance llega la tercera
// delta. Al tope el piloto para (CA-751): bajo el candado lanzar.Lanzar
// tampoco lanza al test-writer ni al reviewer, que no son writer ni
// refutador (409 con la razon), y el CLI no se llama.
func TestHallazgo75a10b_ElCandadoNoLanzaNingunRolAlTope(t *testing.T) {
	casos := []struct {
		role, provider, pedido, accion, extra string
	}{
		{"test-writer", "claude", "Escribi los tests de CA-970.", boardcmd.ActPedirTestWriter, ""},
		// codex, sin tope en USD, toma la review con ventana_max declarada
		// (CA-661); el reviewer cortado era una review completa
		{"reviewer", "codex", "", boardcmd.ActPedirReviewer, "autonomia:\n  ventana_max: 80\n"},
	}
	for _, k := range casos {
		t.Run(k.role, func(t *testing.T) {
			f := lnPATH(t)
			f.cli(t, "claude", false)
			f.cli(t, "codex", false)
			root := tbProyecto(t)
			rgtYAML(t, root, "findings:\n  block_on: high\n  max_rondas: 3\n"+k.extra)
			wt, _ := cldCartaReview(t, "75a10b", root, rgtSlug, 2, 2)
			rgtAuto(t, root)
			rgtCortado(t, root, wt, k.role, k.provider, k.pedido)
			e, err := pilotocmd.Armar(pilotocmd.Opciones{Root: root, Slug: rgtSlug})
			if err != nil {
				t.Fatal(err)
			}
			d := boardcmd.Piloto(e)
			if !d.Launch || d.Action != k.accion || d.Regla != boardcmd.ReglaRelevoPorTiempo {
				t.Fatalf("75a10b: fixture: con dos rondas y tope 3 el piloto releva al %s: %+v", k.role, d)
			}

			rgtDelta(t, wt, 3) // la tercera ronda: 3 >= tope 3
			s := lnServidor(t, root, f)
			razon := boardcmd.RazonTopeDeRondas(3, 2)
			if c := rgtTarjeta(t, s); !c.Rondas.AlTope || c.Rondas.Razon != razon {
				t.Fatalf("75a10b: fixture: tras la delta la tarjeta esta al tope: %+v", c.Rondas)
			}
			p := s.piloto(rgtSlug).Lanzar
			p.Despues = nil
			// el pedido que arma el piloto con su decision (corrida.lanzar)
			req := lanzar.Request{Action: d.Action, Provider: d.Provider, Pedido: d.Pedido, Pilot: true, Resume: d.Resume,
				Delta: d.Delta, Regla: d.Regla, Intento: d.Intento, Ronda: d.Ronda, Hallazgos: d.Hallazgos, Bloqueantes: d.Bloqueantes}
			if d.BudgetUSD > 0 {
				b := d.BudgetUSD
				req.BudgetUSD = &b
			}
			quiero := razon + ": el piloto no lanza nada"
			tr, err := lanzar.Lanzar(context.Background(), p, rgtSlug, req)
			var rech *lanzar.Rechazo
			if tr != nil || !errors.As(err, &rech) || rech.Code != http.StatusConflict || rech.Motivo != quiero {
				t.Errorf("75a10b: al tope, bajo el candado, el piloto no lanza %s: 409 %q; fue trabajo=%v err=%v", req.Action, quiero, tr != nil, err)
			}
			lnEsperarLanzamientos(t, "75a10b", s)
			if n := len(f.llamadas(t, "claude")) + len(f.llamadas(t, "codex")); n != 0 {
				t.Fatalf("75a10b: al tope el CLI no se llama para el %s, se llamo %d veces", k.role, n)
			}
		})
	}
}
