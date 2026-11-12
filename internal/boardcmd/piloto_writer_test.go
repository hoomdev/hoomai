package boardcmd

import (
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Pruebas propias del writer (autonomia-entre-firmas): Piloto con Estados
// armados a mano, sin disco.

func wpAccion(id, rol string, provs ...ProviderOption) Action {
	b := 4.0
	return Action{ID: id, Role: rol, Enabled: true, Pedido: "pedido de " + rol, BudgetUSD: &b, Providers: provs}
}

var (
	wpClaude = ProviderOption{Name: "claude", OK: true, Budget: true}
	wpCodex  = ProviderOption{Name: "codex", OK: true}
)

func wpTarjeta(col string, acciones ...Action) Card {
	b := 20.0
	todas := []Action{
		wpAccion(ActPedirTestWriter, "test-writer", wpClaude, wpCodex),
		wpAccion(ActPedirWriter, "writer", wpClaude, wpCodex),
		wpAccion(ActPedirReviewer, "reviewer", wpCodex),
		wpAccion(ActPedirRefutador, "refutador", wpClaude, wpCodex),
	}
	return Card{Slug: "t", Column: col, Item: item.Item{Slug: "t", Auto: item.AutoHastaHumano, PresupuestoUSD: &b},
		Evidence: CardEvidence{Source: SourceWorktree, Dir: ".hoom/worktrees/t"}, Actions: acciones, PilotActions: todas}
}

func wpVeredicto(id string, gates map[string]string) *verdict.Verdict {
	v := &verdict.Verdict{ID: id}
	for _, n := range []string{"spec_lint", "spec_trace", "spec_approved", "test", finding.GateName} {
		st, ok := gates[n]
		if !ok {
			st = verdict.StatusPass
		}
		v.Gates = append(v.Gates, verdict.GateResult{Name: n, Required: true, Status: st})
	}
	v.Finalize()
	return v
}

func wpSobre(rol, prov, stage, status, col string, v *verdict.Verdict) Sobre {
	rec := envelope.Record{ID: "s-" + rol + "-" + stage, Role: rol, Provider: prov, Stage: stage, Status: status, Columna: col}
	if v != nil {
		rec.VerdictID, rec.Verdict = v.ID, v.Verdict
	}
	return Sobre{Record: rec, Veredicto: v}
}

func TestWriterPilotoCadenaDesdeLaFirma(t *testing.T) {
	c := wpTarjeta(ColTestWriter, wpAccion(ActPedirTestWriter, "test-writer", wpClaude))
	d := Piloto(Estado{Tarjeta: c})
	if !d.Launch || d.Paso != PasoLanzar || d.Role != "test-writer" || d.Provider != "claude" || d.Regla != ReglaCadena || d.BudgetUSD != 4 {
		t.Fatalf("la firma lanza al test-writer por la cadena: %+v", d)
	}
}

func TestWriterPilotoRojoEsperadoDejaPasarAlWriter(t *testing.T) {
	c := wpTarjeta(ColWriter, wpAccion(ActPedirWriter, "writer", wpClaude))
	c.Red = &Red{Plain: "rojo"}
	rojo := wpVeredicto("v1", map[string]string{"test": verdict.StatusFail})
	e := Estado{Tarjeta: c, Sobres: []Sobre{wpSobre("test-writer", "claude", "verify", envelope.StatusNotDeliverable, ColTestWriter, rojo)}}
	if d := Piloto(e); !d.Launch || d.Role != "writer" || d.Regla != ReglaCadena {
		t.Fatalf("el rojo esperado deja pasar al writer: %+v", d)
	}
	e.Sobres[0].Veredicto = wpVeredicto("v1", map[string]string{"test": verdict.StatusFail, "spec_trace": verdict.StatusFail})
	if d := Piloto(e); d.Launch || d.Caso != 8 || !strings.Contains(d.Why, "no cerro entregable") {
		t.Fatalf("sin spec_trace en pass se detiene en el caso 8: %+v", d)
	}
}

func TestWriterPilotoReintento(t *testing.T) {
	c := wpTarjeta(ColWriter, wpAccion(ActPedirWriter, "writer", wpClaude))
	rojo := wpVeredicto("v2", map[string]string{"test": verdict.StatusFail})
	e := Estado{Tarjeta: c, Politica: manifest.AutonomiaPolicy{ReintentosAnteRojo: 1},
		Sobres: []Sobre{wpSobre("writer", "claude", "verify", envelope.StatusNotDeliverable, ColWriter, rojo)}}
	d := Piloto(e)
	if !d.Launch || d.Regla != ReglaReintento || d.Intento != 2 || d.Resume != "" || d.Provider != "claude" || d.Caso != 6 {
		t.Fatalf("reintento: %+v", d)
	}
	if want := PedidoReintento("t", "v2", []string{"test"}); d.Pedido != want {
		t.Fatalf("pedido %q, quiero %q", d.Pedido, want)
	}
	e.Sobres = append(e.Sobres, e.Sobres[0])
	if d := Piloto(e); d.Launch || !strings.HasPrefix(d.Why, "rojo final: 2 intentos del writer y el veredicto v2 sigue rojo en test") {
		t.Fatalf("segundo rojo: %+v", d)
	}
	e.Sobres = e.Sobres[:1]
	e.Sobres[0].Veredicto = wpVeredicto("v2", map[string]string{"test": verdict.StatusError})
	if d := Piloto(e); d.Why != "entorno: el gate test no pudo correr: no se reintenta con modelos" {
		t.Fatalf("gate en error: %+v", d)
	}
}

func TestWriterPilotoRelevo(t *testing.T) {
	c := wpTarjeta(ColWriter, wpAccion(ActPedirWriter, "writer", wpClaude))
	s := wpSobre("writer", "claude", "run", envelope.StatusNotDeliverable, ColWriter, nil)
	s.Record.Corte, s.Record.Pedido, s.SesionID = envelope.CorteTiempo, "implementa", "ses-1"
	e := Estado{Tarjeta: c, Sobres: []Sobre{s},
		Proveedores: []providers.Info{{Name: "claude", Capabilities: providers.Capabilities{Resume: true}}}}
	if d := Piloto(e); !d.Launch || d.Resume != "ses-1" || d.Pedido != PedidoRelevo || d.Regla != ReglaRelevoPorTiempo {
		t.Fatalf("relevo con Resume: %+v", d)
	}
	e.Proveedores = nil
	if d := Piloto(e); !d.Launch || d.Resume != "" || d.Pedido != "implementa" {
		t.Fatalf("relevo sin Resume repite el pedido: %+v", d)
	}
	e.Sobres = append(e.Sobres, s)
	if d := Piloto(e); d.Why != "el writer se corto por tiempo dos veces: sube agents.writer.timeout o parte la tarea" {
		t.Fatalf("dos cortes: %+v", d)
	}
}

func TestWriterPilotoRonda(t *testing.T) {
	c := wpTarjeta(ColReview)
	verde := wpVeredicto("v3", nil)
	f := finding.Item{Finding: finding.Finding{ID: "f1", Severity: "high", File: "a.go", Description: "roto", Task: "t"}}
	max := 80
	e := Estado{Tarjeta: c, Politica: manifest.AutonomiaPolicy{RondasDeHallazgos: 2, VentanaMax: &max},
		Sobres:      []Sobre{wpSobre("reviewer", "codex", "ok", envelope.StatusDeliverable, ColReview, nil)},
		Reviews:     []reviewcmd.Record{{Provider: "codex"}},
		Bloqueantes: []finding.Item{f}, Veredicto: verde}
	d := Piloto(e)
	if !d.Launch || d.Action != ActPedirRefutador || d.Ronda != 1 || d.Bloqueantes != 1 || d.Provider != "codex" || d.BudgetUSD != 0 {
		t.Fatalf("la ronda empieza por el refutador: %+v", d)
	}
	e.Politica.VentanaMax = nil
	if d := Piloto(e); d.Caso != 13 {
		t.Fatalf("sin ventana_max codex no tiene tope: %+v", d)
	}
	e.Politica.VentanaMax = &max
	tw := wpSobre("test-writer", "claude", "ok", envelope.StatusDeliverable, ColWriter, nil)
	tw.Record.Ronda = 1
	ref := wpSobre("refutador", "codex", "ok", envelope.StatusDeliverable, ColReview, nil)
	ref.Record.Ronda, ref.Record.Hallazgos, ref.Record.Bloqueantes = 1, []string{"f1"}, 1
	e.Sobres = []Sobre{tw, e.Sobres[0], ref}
	if d := Piloto(e); !d.Launch || d.Role != "test-writer" || d.Regla != ReglaRondaTestWriter || !strings.Contains(d.Pedido, "f1 [high] a.go: roto") {
		t.Fatalf("despues del refutador va el test-writer: %+v", d)
	}
	// la ventana llena de codex detiene la ronda
	e.Sobres = e.Sobres[1:2]
	e.Hora = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	e.Ventanas = map[string]runcmd.VentanaRun{"codex": {Nombre: "5h", Fin: 85.4, ReiniciaEn: e.Hora.Add(time.Hour)}}
	if d := Piloto(e); d.Why != "la ventana de 5h de codex va en 85% (tope 80%): el piloto espera; reinicia 2026-11-12T11:00:00Z" {
		t.Fatalf("ventana llena: %+v", d)
	}
}
