// Test de regresion del hallazgo a3a4d8 sobre
// .hoom/specs/ciclo-lo-decide-hoom.md (CA-751): sin findings.block_on la
// tarjeta juzga sus hallazgos con "high" y puede quedar al tope, pero
// pilotocmd.Armar no carga ningun bloqueante (Estado.Bloqueantes vacio). El
// piloto para con la razon de la tarjeta igual: no releva ni reintenta lo
// que la tarjeta ya quito.
//
// Fixtures de autonomia_entre_firmas_test.go (aeF1, aeF2, aeCortado,
// aeBase, aePol0, aeDetiene, aeCol) y ciclo_lo_decide_hoom_test.go
// (cldTrasWriter, cldEstado, cldEvRonda, cldRazon).
package boardcmd

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/manifest"
)

func TestHallazgoA3a4d8_ElPilotoParaConLaTarjetaAlTope(t *testing.T) {
	bloq := []finding.Item{aeF1, aeF2}
	// sin block_on, autonomia no puede declarar rondas_de_hallazgos
	pol := manifest.AutonomiaPolicy{ReintentosAnteRojo: 1}

	cortado := aeCortado("writer", "claude", "ses-w-1")
	e := cldTrasWriter(pol, bloq, 2, cortado)
	e.Bloqueantes = nil
	aeCol(t, "a3a4d8", e, ColWriter)
	if !e.Tarjeta.Rondas.AlTope {
		t.Fatalf("a3a4d8: fixture: con dos rondas y dos que bloquean la tarjeta esta al tope: %+v", e.Tarjeta.Rondas)
	}
	aeDetiene(t, "a3a4d8", "writer cortado, sin bloqueantes en el Estado", Piloto(e), cldRazon(2, 2), 7)

	e = cldEstado(cldEvRonda(bloq), aePol0, 3, 2, 2, aeBase()...)
	e.Bloqueantes = nil
	aeCol(t, "a3a4d8", e, ColReview)
	aeDetiene(t, "a3a4d8", "en Review, sin bloqueantes en el Estado", Piloto(e), cldRazon(3, 2), 7)

	// control: bajo el tope de la tarjeta, el writer cortado se releva
	e = cldTrasWriter(pol, bloq, 1, cortado)
	e.Bloqueantes = nil
	if d := Piloto(e); !d.Launch || d.Regla != ReglaRelevoPorTiempo {
		t.Fatalf("a3a4d8: bajo el tope el writer cortado se releva: %+v", d)
	}
}
