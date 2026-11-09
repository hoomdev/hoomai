// Run y las teclas que una lectura corta por la mitad (CA-520, CA-527): un
// caracter UTF-8 o una secuencia de escape partidos en dos lecturas esperan
// a la siguiente. Regresiones de la ronda 1 sobre Run (review completa
// 20261108T231004_195bc7). Con la terminal, los datos, el lanzador y el
// editor falsos de tt_run_test.go; cada espera con su tiempo maximo.
package tablero_test

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// CA-520, CA-554 (hallazgo ca8887): una lectura que termina dentro de un
// caracter UTF-8 espera a la siguiente: "/" con el primer byte de "ñ" y
// despues el segundo, en dos lecturas seguidas, es la busqueda de "ñ" (que
// filtra como n: queda la de la piñata, se va zeta). La linea de mensajes
// muestra "/ñ_" (CA-554: "/<texto>▌", "_" en ASCII).
func TestCA520_CA554_RunRunaPartidaEnDosLecturas(t *testing.T) {
	seg := ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado")
	w := ttNuevoMundo(t, ttTablero(ttCard("pinata", "La piñata", boardcmd.ColWriter, "con ene", seg),
		ttCard("zeta", "Zeta", boardcmd.ColWriter, "sin ene", seg)))
	fin := w.correr(nil)
	ttEsperar(t, "CA-520", "la lista", w, func() bool { return w.marcoCon("zeta") && w.marcoCon("pinata") })
	w.term.mandar("/\xc3")
	w.term.mandar("\xb1")
	ttEsperar(t, "CA-520", "la busqueda de ñ", w, func() bool {
		return w.marcoCon("/ñ_") && w.marcoCon("pinata") && !w.marcoCon("zeta")
	})
	w.term.mandar("\x03")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-520: ctrl-c en la busqueda sale con 0, salio con %d", code)
	}
}

// CA-527, CA-568 (hallazgo ca8887): lo mismo en el Pedido del modal: los
// dos bytes de "ñ" en dos lecturas seguidas son una "ñ" en el pedido que se
// lanza (desde el boton: enter en el Pedido va al boton, CA-568).
func TestCA527_CA568_RunRunaPartidaEnElPedido(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola"
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(a)))
	fin := w.correr(nil)
	ttEsperar(t, "CA-527", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, "CA-527", "el modal", w, func() bool { return w.marcoCon("Se va a ejecutar:") && w.marcoCon("hoom pedir writer tarjeta -m") })
	w.term.mandar("\xc3")
	w.term.mandar("\xb1")
	ttEsperar(t, "CA-527", "la ñ en el pedido", w, func() bool { return w.marcoCon("holañ_") })
	w.term.mandar("\r\r")
	ttEsperar(t, "CA-527", "la orden", w, func() bool { return len(w.lanz.lanzadas()) == 1 })
	ttMismosArgs(t, "CA-527", "el pedido con la ñ", w.lanz.lanzadas()[0].Args, []string{"pedir", "writer", "tarjeta", "-m", "holañ"})
	w.term.mandar("q")
	fin(t)
}

// CA-520, CA-552 (hallazgo ca8887): una lectura que termina dentro de una
// secuencia de escape tambien espera a la siguiente: ESC y despues "[B", en
// dos lecturas seguidas, es la flecha abajo (no esc, "[" y "B"), que en [2]
// elige la fila de abajo.
func TestCA520_CA552_RunFlechaPartidaEnDosLecturas(t *testing.T) {
	seg := ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado")
	w := ttNuevoMundo(t, ttTablero(ttCard("uno", "Uno", boardcmd.ColWriter, "la primera", seg),
		ttCard("dos", "Dos", boardcmd.ColWriter, "la segunda", seg)))
	fin := w.correr(nil)
	ttEsperar(t, "CA-552", "[2] con uno elegida", w, func() bool { return w.filaElegida("uno") })
	w.term.mandar("\x1b")
	w.term.mandar("[B")
	ttEsperar(t, "CA-552", "la flecha abajo elige dos", w, func() bool { return w.filaElegida("dos") })
	w.term.mandar("q")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-520: q sale con 0, salio con %d", code)
	}
}
