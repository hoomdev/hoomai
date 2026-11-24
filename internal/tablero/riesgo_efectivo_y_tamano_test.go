// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// el detalle de `hoom tablero` (CA-786): el borde de arriba dice ` · riesgo
// <efectivo>` con el riesgo de la tarjeta (CardEvidence.Riesgo), no con el
// declarado del item; y la linea `tamano:` del ultimo veredicto va en la
// parte B, despues de los pendientes (aclaracion 17), solo cuando la tarjeta
// tiene tamano. Escritos desde el spec, las aclaraciones de la ronda 0 y las
// firmas publicas, sin leer la implementacion.
package tablero_test

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-786: el borde del detalle usa el efectivo: un item que declara alto con
// efectivo normal no lo dice; uno que no declara nada con efectivo alto si.
func TestCA786_BordeDelDetalleConElEfectivo(t *testing.T) {
	defer tv3aSinPanico(t)
	con := func(declarado, efectivo string) boardcmd.Card {
		c := tv3aListado()
		c.Item.Riesgo = declarado
		c.Evidence.Riesgo = efectivo
		return c
	}
	for _, k := range []struct {
		que                 string
		declarado, efectivo string
		der                 string
	}{
		{"efectivo alto sin declarado", "", riesgo.Alto, "listado · riesgo alto"},
		{"efectivo alto sobre un bajo", riesgo.Bajo, riesgo.Alto, "listado · riesgo alto"},
		{"declarado alto, efectivo normal", riesgo.Alto, riesgo.Normal, "listado"},
		{"declarado bajo, efectivo normal", riesgo.Bajo, riesgo.Normal, "listado"},
		{"efectivo bajo", riesgo.Bajo, riesgo.Bajo, "listado · riesgo bajo"},
	} {
		m := ttModelo(tv3aCfg(120, 40, true, false), ttTablero(con(k.declarado, k.efectivo)))
		w := 120 - tv3aL(120)
		if quiere := tv3aArriba(true, false, w, "Listado de proyectos", k.der); tv3aDetalle(t, "CA-786", m, 120).Arriba != quiere {
			t.Fatalf("CA-786: %s: el borde de arriba del detalle es\n%q\nfue\n%q", k.que, quiere, tv3aDetalle(t, "CA-786", m, 120).Arriba)
		}
	}
}

// CA-786 y aclaracion 17: la linea `tamano:` va en la parte B, despues de los
// pendientes, en la forma ancha y en la angosta; sin tamano no hay linea.
func TestCA786_TamanoEnLaParteB(t *testing.T) {
	defer tv3aSinPanico(t)
	c := tv3aCon(ttCard("pend", "Con pendientes", boardcmd.ColTuAceptacion, "te toca aceptar",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"), ttSeg("review", "review", boardcmd.SegHecho, "sin bloqueos")), ttAccionIntegrar())
	c.Doctor = []boardcmd.Problem{{ID: "p1", Slug: "pend", What: "spec cambiado", Plain: "el spec cambio despues de tu firma",
		Action: "hoom aprobar pend --si"}}
	c.Pendientes = boardcmd.Pendientes{Medium: 1, Low: 2, IDs: []string{"a1", "b2", "c3"}, Texto: "pendientes: 1 medium, 2 low (no bloquean)"}
	c.Evidence.Tamano = &riesgo.Tamano{Producto: 90, Pruebas: 1500, Hoom: 32}
	h, _, _, _ := tv3aMedidor(true)
	b := []string{
		"",
		" Tu aceptacion · gasto sin dato de costo",
		" doctor: el spec cambio despues de tu firma: hoom aprobar pend --si",
		" pendientes: 1 medium, 2 low (no bloquean)",
		" tamano: producto 90 · pruebas 1500 · hoom 32",
		"",
		"  " + h + " spec: aprobado",
		"  " + h + " review: sin bloqueos",
	}
	m := tv3aGrande(true, false, ttTablero(c))
	d := tv3aDoc(t, "CA-786", m, 200)
	i := tv3aIndice(d, tv3aDer(" EL CAMINO", "paso 7 de 8", 118))
	if i < 0 || i+9+len(b) > len(d) {
		t.Fatalf("CA-786: el detalle tiene EL CAMINO y despues B y C:\n%s", strings.Join(d, "\n"))
	}
	tv3aIgualesDoc(t, "CA-786", "tamano en B", m, d[i+9:i+9+len(b)], b)
	for _, l := range d[:i] {
		if strings.Contains(l, "tamano") {
			t.Fatalf("CA-786: la linea tamano no va en AHORA, TE TOCA ni SIGUE: %q", l)
		}
	}
	m = ttModelo(tv3aCfg(99, 80, true, false), ttTablero(c))
	m.Tecla(tablero.TeclaTab, ttAhora)
	d = tv3aDoc(t, "CA-786", m, 99)
	if len(d) < 4+len(b) {
		t.Fatalf("CA-786: la forma angosta tiene sus cuatro lineas y despues B:\n%s", strings.Join(d, "\n"))
	}
	tv3aIgualesDoc(t, "CA-786", "tamano en B de la forma angosta", m, d[4:4+len(b)], b)

	// sin pendientes: despues del doctor
	c.Pendientes = boardcmd.Pendientes{IDs: []string{}}
	sinPend := append(append([]string{}, b[:3]...), b[4:]...)
	m = tv3aGrande(true, false, ttTablero(c))
	d = tv3aDoc(t, "CA-786", m, 200)
	i = tv3aIndice(d, tv3aDer(" EL CAMINO", "paso 7 de 8", 118))
	if i < 0 || i+9+len(sinPend) > len(d) {
		t.Fatalf("CA-786: el detalle tiene EL CAMINO y despues B y C:\n%s", strings.Join(d, "\n"))
	}
	tv3aIgualesDoc(t, "CA-786", "tamano en B sin pendientes", m, d[i+9:i+9+len(sinPend)], sinPend)

	// sin tamano (sin veredicto o uno anterior al spec): no hay linea
	c.Evidence.Tamano = nil
	m = tv3aGrande(true, false, ttTablero(c))
	for _, l := range tv3aDoc(t, "CA-786", m, 200) {
		if strings.Contains(l, "tamano:") {
			t.Fatalf("CA-786: sin tamano no hay linea tamano: %q", l)
		}
	}
	// un tamano en cero tambien se dice
	c.Evidence.Tamano = &riesgo.Tamano{}
	m = tv3aGrande(true, false, ttTablero(c))
	if tv3aIndice(tv3aDoc(t, "CA-786", m, 200), " tamano: producto 0 · pruebas 0 · hoom 0") < 0 {
		t.Fatalf("CA-786: un tamano en cero tambien es una linea:\n%s", strings.Join(tv3aDoc(t, "CA-786", m, 200), "\n"))
	}
}
