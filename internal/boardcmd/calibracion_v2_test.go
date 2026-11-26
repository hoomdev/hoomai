package boardcmd

import (
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

func TestPilotoOmiteRefutadorEnNormalYLoCorreEnAlto(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	for _, riesgo := range []string{"normal", "bajo"} {
		ev := piEv("tu-aceptacion", piConTope, piNada)
		ev.Item.Riesgo = riesgo
		ev.Findings = []finding.Item{aeF1, aeF2}
		ev.Reviews = []reviewcmd.Record{aeRev("r-nuevo", "gemini", 50)}
		e := aeEstado(ev, pol, aeSobres(aeBase()...))
		d := Piloto(e)
		if !d.Launch || d.Role != "test-writer" || d.Regla != ReglaRondaTestWriter {
			t.Fatalf("en riesgo %s el piloto omite refutador y va directo a test-writer: %+v", riesgo, d)
		}
	}

	// En alto si lanza refutador primero
	ev := piEv("tu-aceptacion", piConTope, piNada)
	ev.Item.Riesgo = "alto"
	ev.Findings = []finding.Item{aeF1, aeF2}
	ev.Reviews = []reviewcmd.Record{aeRev("r-nuevo", "gemini", 50)}
	e := aeEstado(ev, pol, aeSobres(aeBase()...))
	d := Piloto(e)
	if !d.Launch || d.Role != "refutador" || d.Regla != ReglaRondaRefutador {
		t.Fatalf("en riesgo alto el piloto corre refutador primero: %+v", d)
	}
}

func TestCadenaValidaDeltaAcotadaEnAlto(t *testing.T) {
	// Review completa de 4 lentes que solo encontro 1 high en "risk":
	// una delta que solo corre ["risk"] satisface cadenaValida.
	ev := crEv("alto", 50, 10, "internal/auth/login.go")
	fRisk := finding.Item{
		Finding: finding.Finding{ID: "f-risk", Severity: "high", Lens: "risk", File: "internal/auth/login.go", Task: bdSlug},
		Status:  finding.StatusCorrected,
	}
	ev.Findings = []finding.Item{fRisk}
	c1 := crRec("c1", reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes, "f-risk")
	c1.Hasta = "3333333333333333333333333333333333333333"
	d1 := crRec("d1", reviewcmd.CoberturaDelta, "c1", []string{"risk"})
	d1.Desde = c1.Hasta
	d1.CreatedAt = bdT0.Add(40 * time.Minute)
	ev.Reviews = []reviewcmd.Record{c1, d1}
	ev = bdColas(ev)
	ev.ReviewTails["c1"] = ReviewTail{Ancestro: true, Codigo: true}

	card := Derive(ev)
	bdCol(t, "delta acotada a risk en alto", card, ColTuAceptacion)
	if card.Evidence.ReviewID != "d1" {
		t.Fatalf("la delta acotada a risk debe contar: %+v", card.Evidence)
	}

	// Si la delta corrio reliability en vez de risk, NO satisface cadenaValida.
	ev.Reviews[1].Lenses = []string{"reliability"}
	card = Derive(ev)
	bdCol(t, "delta sin la lente bloqueante", card, ColReview)
	if card.Evidence.ReviewID != "" {
		t.Fatalf("una delta que omite la lente con bloqueante no cuenta: %+v", card.Evidence)
	}
	_ = envelope.StatusDeliverable
}
