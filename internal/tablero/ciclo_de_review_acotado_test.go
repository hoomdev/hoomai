// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// `hoom tablero` (CA-630): el detalle de la tarjeta muestra la linea de
// pendientes (Card.Pendientes.Texto) cuando la tarjeta tiene alguno, como
// texto (pasa por la neutralizacion de controles), y no la muestra cuando no
// tiene. Escritos desde el spec y las firmas publicas, sin leer la
// implementacion.
package tablero_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

const crPendientes = "pendientes: 1 medium, 2 low (no bloquean)"

// crModeloConPendientes arma el modelo con la tarjeta (con p) elegida y su
// detalle leido, en una pantalla donde entra todo.
func crModeloConPendientes(p boardcmd.Pendientes, color, utf bool) *tablero.Modelo {
	c := ttTarjetaCon(ttAccionWriter())
	c.Pendientes = p
	d := tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
		Spec: boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "- CA-1: criterio\n"}}}
	cfg := tablero.Config{Proyecto: "demo", Ancho: 240, Alto: 60, Color: color, UTF8: utf, Editor: "vi"}
	m := ttModelo(cfg, ttTablero(c))
	m.LeerDetalle(d)
	return m
}

// CA-630: con pendientes, el detalle de la tarjeta muestra la linea tal cual.
func TestCA630_ElDetalleMuestraLosPendientes(t *testing.T) {
	for _, utf := range []bool{false, true} {
		m := crModeloConPendientes(boardcmd.Pendientes{Medium: 1, Low: 2, IDs: []string{"f1", "f2", "f3"}, Texto: crPendientes}, false, utf)
		if !ttTiene(m, crPendientes) {
			t.Fatalf("CA-630 (utf8=%v): el detalle muestra %q:\n%s", utf, crPendientes, ttPantalla(m))
		}
	}
}

// CA-630: sin pendientes (Texto vacio) no hay linea de pendientes.
func TestCA630_SinPendientesNoHayLinea(t *testing.T) {
	for _, p := range []boardcmd.Pendientes{{}, {IDs: []string{}}} {
		m := crModeloConPendientes(p, false, false)
		if s := ttPantalla(m); strings.Contains(s, "pendientes") || strings.Contains(s, "no bloquean") {
			t.Fatalf("CA-630: sin pendientes la pantalla no dice pendientes:\n%s", s)
		}
	}
}

// CA-630: la linea es texto: un Texto con controles se ve neutralizado, sin
// una secuencia que llegue a la terminal (como CA-534 para todo texto).
func TestCA630_LaLineaEsTexto(t *testing.T) {
	for _, color := range []bool{false, true} {
		for _, utf := range []bool{false, true} {
			ca := fmt.Sprintf("CA-630 (color=%v, utf8=%v)", color, utf)
			m := crModeloConPendientes(boardcmd.Pendientes{Low: 1, IDs: []string{"f1"}, Texto: "pendientes: 1 low " + ttHostil}, color, utf)
			s := ttSinControles(t, ca, "el detalle con pendientes", m, color)
			if !strings.Contains(s, "pendientes: 1 low "+ttNeutro) {
				t.Fatalf("%s: la linea se muestra como texto neutralizado:\n%s", ca, s)
			}
		}
	}
}
