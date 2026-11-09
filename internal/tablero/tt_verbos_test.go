// Lo que corre es lo que se muestra (CA-525, CA-532): para cada fila de
// Atajos con Accion, los Args del Lanzar son las palabras de su Verbo, y el
// menu y la linea -> muestran ese mismo verbo. Regresion de la ronda 1
// (review completa 20261108T231004_195bc7 de tablero-en-la-terminal),
// escrita desde el contrato de hoy (doc.go y go doc del paquete), sin leer
// la implementacion.
package tablero_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- CA-525, CA-532: lo que corre es lo que se muestra

// CA-532, CA-525 (hallazgo e8c2ef, guarda): para cada fila de Atajos con
// Accion, lo que corre es lo que se muestra. Los Args del Lanzar empiezan con
// las palabras de su Verbo sin "hoom" y con <slug> reemplazado: u enseguida,
// A e I con Firmar, s con enter; a, t, w, r, R y n por su formulario sin
// cambiar nada, con "-m" <pedido> cuando el formulario tiene Pedido (y su
// linea -> empieza con el Verbo). El menu muestra ese mismo Verbo.
func TestCA532_CA525_LoQueCorreEsLoQueSeMuestra(t *testing.T) {
	acciones := map[string]boardcmd.Action{
		boardcmd.ActPedirArquitecto: ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec"),
		boardcmd.ActPedirTestWriter: ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests"),
		boardcmd.ActPedirWriter:     ttAccionWriter(),
		boardcmd.ActPedirReviewer:   ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""),
		boardcmd.ActReanudar:        ttPagosCaidos().Actions[0],
		boardcmd.ActGuardar:         ttAccionGuardar(true, "", "internal/a.go"),
		boardcmd.ActAprobar:         ttAccionAprobar(),
		boardcmd.ActIntegrar:        ttAccionIntegrar(),
		tablero.AccionSiguiente:     ttAccionWriter(), // n: la primera accion de la tarjeta, de rol
	}
	vistas := 0
	for _, fila := range tablero.Atajos {
		if fila.Accion == "" {
			continue
		}
		vistas++
		if len(fila.Teclas) == 0 {
			t.Fatalf("CA-532: la fila %q no tiene teclas", fila.Hace)
		}
		k := fila.Teclas[0]
		ca := fmt.Sprintf("CA-532 (%q, %s)", k, fila.Accion)
		verbo := strings.ReplaceAll(fila.Verbo, "<slug>", "tarjeta")
		pal := strings.Fields(verbo)
		if len(pal) < 2 || pal[0] != "hoom" {
			t.Fatalf("%s: el Verbo de la fila es un verbo de hoom: %q", ca, fila.Verbo)
		}
		quiere := append([]string{}, pal[1:]...)
		var card boardcmd.Card
		acc, conAccion := acciones[fila.Accion]
		switch {
		case fila.Accion == tablero.AccionUsar:
			card = ttTarjetaCon()
		case conAccion:
			card = ttTarjetaCon(acc)
		default:
			t.Fatalf("%s: la guarda no sabe armar la accion %q: agregala a la tabla del test", ca, fila.Accion)
		}
		if conAccion && fila.Accion != tablero.AccionSiguiente {
			// el menu muestra el mismo verbo
			m := ttModelo(ttAncho(), ttTablero(card))
			m.Tecla(tablero.TeclaEspacio, ttAhora)
			l, n := ttLineaCon(m, " "+string(k)+" "+acc.Label+"  ")
			if n == 0 || !strings.HasSuffix(l, "  "+verbo) {
				t.Fatalf("%s: el menu muestra la accion con su tecla y el verbo %q: %q\n%s", ca, verbo, l, ttPantalla(m))
			}
		}
		m := ttModelo(ttAncho(), ttTablero(card))
		ords := m.Tecla(k, ttAhora)
		switch m.Vista() {
		case tablero.VistaFormulario:
			ttSinOrdenes(t, ca, "abrir el formulario", ords)
			if got := ttFlecha(t, ca, m); !strings.HasPrefix(got, verbo) {
				t.Fatalf("%s: la linea -> empieza con el verbo de la tabla %q: %q", ca, verbo, got)
			}
			if tienePedido := ttCampo(m, "Pedido") != ""; tienePedido != (acc.Role != "reviewer") {
				t.Fatalf("%s: el formulario tiene Pedido salvo el del reviewer (tiene: %v)", ca, tienePedido)
			}
			if ttCampo(m, "Pedido") != "" {
				quiere = append(quiere, "-m", strings.TrimSpace(acc.Pedido))
			}
			ords = m.Tecla(tablero.TeclaEnter, ttAhora)
		case tablero.VistaConfirmacion:
			ttSinOrdenes(t, ca, "abrir la confirmacion", ords)
			if fila.Accion == boardcmd.ActAprobar || fila.Accion == boardcmd.ActIntegrar {
				m.Tecla(tablero.TeclaDer, ttAhora)
			}
			ords = m.Tecla(tablero.TeclaEnter, ttAhora)
		}
		l := ttUnaLanzar(t, ca, "la accion", ords)
		ttMismosArgs(t, ca, "lo que corre es lo que se muestra", l.Args, quiere)
		if l.Slug != "tarjeta" || l.Rol != acc.Role {
			t.Fatalf("%s: el Lanzar es de la tarjeta y del rol de la accion (%q): %#v", ca, acc.Role, l)
		}
	}
	if vistas < 10 {
		t.Fatalf("CA-532: la tabla tiene las diez teclas de accion del spec, tiene %d", vistas)
	}
}
