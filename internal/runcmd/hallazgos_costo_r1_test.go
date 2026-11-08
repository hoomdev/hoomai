// Test del writer para el hallazgo 610e19 de la review 1 de costo-por-spec
// (registro 20261107T163525_52e14a).
package runcmd

import (
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// Hallazgo 610e19 — CA-540: reinicia_en es el del ultimo reporte tal como
// vino; un ultimo reporte sin hora de reinicio no hereda la de uno anterior,
// y el reinicio se sigue viendo contra la ultima hora conocida.
func TestHallazgo_610e19_CA540_ReiniciaEnEsElDelUltimoReporte(t *testing.T) {
	t1 := time.Date(2026, 11, 7, 15, 0, 0, 0, time.UTC)
	snap := func(usado float64, r time.Time) []providers.Ventana {
		return []providers.Ventana{{Nombre: "5h", Usado: usado, ReiniciaEn: r}}
	}
	vs := ventanasDe([][]providers.Ventana{snap(10, t1), snap(20, time.Time{})})
	if len(vs) != 1 || !vs[0].ReiniciaEn.IsZero() || vs[0].Reiniciada || vs[0].Fin != 20 {
		t.Fatalf("el ultimo reporte no trae hora: sin reinicia_en y sin reinicio: %+v", vs)
	}
	vs = ventanasDe([][]providers.Ventana{snap(10, t1), snap(20, time.Time{}), snap(3, t1.Add(5*time.Hour))})
	if len(vs) != 1 || !vs[0].Reiniciada || !vs[0].ReiniciaEn.Equal(t1.Add(5*time.Hour)) {
		t.Fatalf("el reinicio se ve contra la ultima hora conocida: %+v", vs)
	}
	vs = ventanasDe([][]providers.Ventana{snap(10, t1), snap(20, time.Time{}), snap(30, t1.Add(time.Second))})
	if len(vs) != 1 || vs[0].Reiniciada {
		t.Fatalf("un segundo no es reinicio, aunque haya un reporte sin hora en medio: %+v", vs)
	}
}
