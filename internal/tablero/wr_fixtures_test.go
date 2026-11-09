// Fixtures del writer para sus propias pruebas de tablero-con-paneles
// (archivos wr_*): tableros armados a mano, sin providers detectados.
package tablero

import (
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/statuscmd"
	"github.com/hoomdev/hoomai/internal/valores"
	"github.com/hoomdev/hoomai/internal/verdict"
)

var wrAhora = time.Date(2026, 11, 9, 14, 2, 0, 0, time.UTC)

func wrF(v float64) *float64     { return &v }
func wrT(t time.Time) *time.Time { return &t }

func wrSeg(id, state, detail string) boardcmd.Segment {
	return boardcmd.Segment{ID: id, Label: id, State: state, Detail: detail}
}

func wrCard(slug, titulo, col, plain string, segs ...boardcmd.Segment) boardcmd.Card {
	c := boardcmd.Card{Slug: slug, Item: item.Item{Slug: slug, Titulo: titulo}, Column: col, Plain: plain, Meter: segs,
		Evidence: boardcmd.CardEvidence{Dir: ".", Spec: ".hoom/specs/" + slug + ".md"}, Missing: []string{}}
	for _, k := range boardcmd.Columns {
		if k.ID == col {
			c.ColumnName = k.Name
			if k.Human {
				c.WaitingHuman, c.NeedsDecision = true, true
			}
		}
	}
	return c
}

func wrTablero(cards ...boardcmd.Card) boardcmd.Board {
	var b boardcmd.Board
	for _, k := range boardcmd.Columns {
		col := boardcmd.BoardColumn{ID: k.ID, Name: k.Name, Human: k.Human}
		for _, c := range cards {
			if c.Column == k.ID {
				col.Cards = append(col.Cards, c)
			}
		}
		b.Columns = append(b.Columns, col)
	}
	return b
}

func wrAccionWriter() boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActPedirWriter, Role: "writer", Label: "Pedir al writer", Enabled: true,
		Pedido:    "Implementa .hoom/specs/infra-api-convenciones.md hasta que hoom verify quede VERDE",
		BudgetUSD: wrF(5),
		Providers: []boardcmd.ProviderOption{
			{Name: "claude", OK: true, Budget: true, MinBudgetUSD: 0.5, Default: true},
			{Name: "codex", OK: true},
			{Name: "gemini", OK: true},
			{Name: "opencode", OK: false, Why: "opencode no puede recibir el contrato del rol"},
		},
		Model: "sonnet", Effort: "", TimeoutS: 2700,
		Origen: valores.Origenes{Provider: "tarjeta", Model: "hoom.yaml", Effort: "provider", BudgetUSD: "tarjeta", Timeout: "hoom.yaml"}}
}

func wrTableroPrincipal() boardcmd.Board {
	nuc := wrCard("nuc-tenancy", "Aislar los datos por tenant", boardcmd.ColTuAprobacion, "el spec espera tu aprobacion",
		wrSeg("spec", boardcmd.SegHecho, "pasa el lint"), wrSeg("aprobacion", boardcmd.SegFalta, "sin tu firma"),
		wrSeg("pruebas", boardcmd.SegFalta, "0/14"), wrSeg("codigo", boardcmd.SegFalta, ""), wrSeg("review", boardcmd.SegFalta, ""))
	nuc.Evidence.Criteria = 14
	nuc.Spend = boardcmd.Spend{CostUSD: wrF(0.42), BudgetUSD: wrF(5)}
	nuc.Providers.Writer = "claude"
	nuc.Actions = []boardcmd.Action{{ID: boardcmd.ActAprobar, Label: "Aprobar spec", Enabled: true, Signer: "Henry <h@x>"}}
	api := wrCard("infra-api-convenciones", "Convenciones de la API", boardcmd.ColWriter, "el writer esta trabajando",
		wrSeg("spec", boardcmd.SegHecho, ""), wrSeg("aprobacion", boardcmd.SegHecho, ""), wrSeg("pruebas", boardcmd.SegFalta, ""),
		wrSeg("codigo", boardcmd.SegFalta, ""), wrSeg("review", boardcmd.SegFalta, ""))
	api.Running = &boardcmd.Running{EnvelopeID: "env-1", RunID: "run-1", Role: "writer", Stage: "run", Step: 3, Steps: 6,
		StartedAt: wrAhora.Add(-(12*time.Minute + 41*time.Second)), TimeoutS: 2700, LastEventAt: wrT(wrAhora.Add(-2 * time.Second))}
	api.Spend = boardcmd.Spend{CostUSD: wrF(3.88)}
	api.Actions = []boardcmd.Action{wrAccionWriter()}
	var cards = []boardcmd.Card{nuc, api}
	for _, s := range []string{"infra-ci-cd", "infra-observabilidad", "docs-guia"} {
		cards = append(cards, wrCard(s, "Titulo "+s, boardcmd.ColBacklog, "falta el spec",
			wrSeg("spec", boardcmd.SegFalta, ""), wrSeg("a", boardcmd.SegFalta, ""), wrSeg("b", boardcmd.SegFalta, ""),
			wrSeg("c", boardcmd.SegFalta, ""), wrSeg("d", boardcmd.SegNoAplica, "")))
	}
	for _, s := range []string{"viejo-1", "viejo-2", "viejo-3"} {
		cards = append(cards, wrCard(s, s, boardcmd.ColHecho, "terminada"))
	}
	return wrTablero(cards...)
}

func wrProyecto() Proyecto {
	return Proyecto{Git: true, Rama: "main",
		Check:     checkcmd.Result{OK: true, VerdictID: "v1"},
		Veredicto: &statuscmd.LastVerdict{ID: "v1", Verdict: "green", CreatedAt: wrAhora.Add(-2 * time.Hour), Summary: verdict.Summary{Pass: 5}},
		Hoy:       costcmd.Hoy{Runs: 4, CostoUSD: 3.88}}
}

func wrHallazgos() Hallazgos {
	return Hallazgos{Items: []finding.Item{
		{Finding: finding.Finding{ID: "f-low", Severity: "low", Lens: "claridad", File: "internal/tenant.go:7", Task: "nuc-tenancy",
			Description: "el nombre del tenant no se recorta"}, Status: "abierto"},
		{Finding: finding.Finding{ID: "f-high", Severity: "high", Lens: "seguridad", File: "router.go", Task: "infra-api-convenciones",
			Description: "router.go:41 no valida el tenant"}, Status: "abierto"},
	}}
}

func wrModelo(w, h int, utf8 bool) *Modelo {
	m := Nuevo(Config{Proyecto: "oceanbuild", Ancho: w, Alto: h, UTF8: utf8, Editor: "vi"})
	m.Leer(Lectura{Ahora: wrAhora, Tablero: wrTableroPrincipal(), Fijada: "nuc-tenancy"})
	m.LeerProyecto(wrProyecto())
	m.LeerHallazgos(wrHallazgos())
	return m
}

func wrPantalla(m *Modelo) string { return strings.Join(m.Pantalla(), "\n") }

func wrTeclas(m *Modelo, ts ...Tecla) []Orden {
	var out []Orden
	for _, t := range ts {
		out = append(out, m.Tecla(t, wrAhora)...)
	}
	return out
}
