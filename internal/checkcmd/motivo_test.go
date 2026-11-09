// checkcmd.Motivo (spec tablero-con-paneles, CA-560: "check ROJO: <motivo>"
// con el texto de hoom check): por que un check que falla esta rojo, en las
// palabras de hoom check, una fuente para la CLI y para [1] de hoom tablero.
package checkcmd_test

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/checkcmd"
)

// CA-560: Motivo por cada Reason, y "" para un check OK o una Reason que no
// conoce.
func TestCA560_Motivo(t *testing.T) {
	casos := []struct {
		r      checkcmd.Result
		quiere string
	}{
		{checkcmd.Result{Reason: checkcmd.ReasonNoVerdict}, "no existe ningun veredicto"},
		{checkcmd.Result{Reason: checkcmd.ReasonOnlyPartial}, "solo hay veredictos PARCIALES (--gate), que no son referencia"},
		{checkcmd.Result{Reason: checkcmd.ReasonRedVerdict, VerdictID: "20261109T100000_abc123"}, "el ultimo veredicto (20261109T100000_abc123) es rojo"},
		{checkcmd.Result{Reason: checkcmd.ReasonDrift, VerdictID: "v-9"}, "el arbol cambio despues del ultimo veredicto verde (v-9)"},
		{checkcmd.Result{OK: true, VerdictID: "v"}, ""},
		{checkcmd.Result{Reason: "otra-cosa"}, ""},
	}
	for _, c := range casos {
		if got := checkcmd.Motivo(c.r); got != c.quiere {
			t.Fatalf("CA-560: Motivo(%+v) = %q, se esperaba %q", c.r, got, c.quiere)
		}
	}
}
