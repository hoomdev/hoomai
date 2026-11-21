// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md sobre
// hoom tablero (CA-753, aclaracion 7): pedir-refutador con la tecla f,
// «pedir al refutador», verbo `hoom pedir refutador <slug>`, en los mismos
// lugares que las otras acciones de rol; sin la accion, el mensaje de hoy; con
// ella, el modal de pedir lanza `hoom pedir refutador <slug>` sin -m (el
// refutador arma su propio pedido, CA-755).
package tablero_test

import (
	"reflect"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

const (
	cldHace  = "pedir al refutador"
	cldVerbo = "hoom pedir refutador <slug>"
)

// cldFilaDe es la unica fila de Atajos con esa Accion.
func cldFilaDe(t *testing.T, ca, accion string) tablero.Atajo {
	t.Helper()
	var out []tablero.Atajo
	for _, a := range tablero.Atajos {
		if a.Accion == accion {
			out = append(out, a)
		}
	}
	if len(out) != 1 {
		t.Fatalf("%s: Atajos tiene una fila para %s, tiene %d", ca, accion, len(out))
	}
	return out[0]
}

func cldTiene(ks []tablero.Tecla, k tablero.Tecla) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

// CA-753 y aclaracion 7: la fila de pedir-refutador es la tecla f, «pedir
// al refutador», `hoom pedir refutador <slug>`, en los mismos Lugares que
// pedir-writer y, como ella, de la tarjeta; en cada uno de esos Lugares f no
// es de ninguna otra fila, y AtajosDe la trae.
func TestCA753_LaTeclaF(t *testing.T) {
	const ca = "CA-753"
	fila := cldFilaDe(t, ca, boardcmd.ActPedirRefutador)
	if len(fila.Teclas) == 0 || fila.Teclas[0] != tablero.Tecla("f") {
		t.Fatalf("%s: pedir-refutador es la tecla f: %q", ca, fila.Teclas)
	}
	if fila.Hace != cldHace || fila.Verbo != cldVerbo {
		t.Fatalf("%s: la fila dice %q y %q, dice %q y %q", ca, cldHace, cldVerbo, fila.Hace, fila.Verbo)
	}
	writer := cldFilaDe(t, ca, boardcmd.ActPedirWriter)
	if !reflect.DeepEqual(fila.Lugares, writer.Lugares) || fila.DeTarjeta != writer.DeTarjeta || len(fila.Lugares) == 0 {
		t.Fatalf("%s: f va en los mismos lugares que pedir-writer: %q (de tarjeta %v) vs %q (de tarjeta %v)",
			ca, fila.Lugares, fila.DeTarjeta, writer.Lugares, writer.DeTarjeta)
	}
	for _, l := range fila.Lugares {
		en := false
		for _, a := range tablero.AtajosDe(l) {
			if a.Accion == boardcmd.ActPedirRefutador {
				en = true
				continue
			}
			if cldTiene(a.Teclas, "f") {
				t.Fatalf("%s: en %q la tecla f es solo de pedir al refutador, tambien de %q", ca, l, a.Hace)
			}
		}
		if !en {
			t.Fatalf("%s: AtajosDe(%q) trae la fila de pedir al refutador", ca, l)
		}
	}
}

// CA-753: f en una tarjeta que no tiene pedir-refutador no ejecuta nada y
// dice `en <columna> no hay pedir al refutador`.
func TestCA753_FSinLaAccion(t *testing.T) {
	const ca = "CA-753"
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter())))
	ttSinOrdenes(t, ca, "f sin la accion", m.Tecla("f", ttAhora))
	ttDiceMensaje(t, ca, "f sin la accion", m, false, "en "+ttNombreColumna(boardcmd.ColWriter)+" no hay "+cldHace)
}

// CA-753 y CA-755: f en una tarjeta con pedir-refutador abre el modal de
// pedir, que dice el verbo de la tabla, y su boton lanza `hoom pedir
// refutador tarjeta` del rol refutador, sin -m: el pedido lo arma hoom.
func TestCA753_FLanzaAlRefutador(t *testing.T) {
	const ca = "CA-753"
	acc := ttAccionRol(boardcmd.ActPedirRefutador, "refutador", "Pedir al refutador",
		"Ronda 1 de hallazgos de tarjeta. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: f1, f2. No edites codigo.")
	acc.Hallazgos = []string{"f1", "f2"}
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(acc, ttAccionWriter())))
	ttSinOrdenes(t, ca, "abrir Pedir con f", m.Tecla("f", ttAhora))
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("%s: f abre el modal de pedir: %q", ca, m.Modal())
	}
	if got := ttModal(t, ca, m, 200).ttComando(); len(got) < len("hoom pedir refutador tarjeta") || got[:len("hoom pedir refutador tarjeta")] != "hoom pedir refutador tarjeta" {
		t.Fatalf("%s: 'Se va a ejecutar' empieza con hoom pedir refutador tarjeta: %q", ca, got)
	}
	for i := 0; m.Lugar() != tablero.LugarPedirBotones; i++ {
		if i > 20 {
			t.Fatalf("%s: el modal llega a sus botones con Tab", ca)
		}
		m.Tecla(tablero.TeclaTab, ttAhora)
	}
	l := ttUnaLanzar(t, ca, "el boton del modal", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, ca, "pedir al refutador", l.Args, []string{"pedir", "refutador", "tarjeta"})
	if l.Slug != "tarjeta" || l.Rol != "refutador" {
		t.Fatalf("%s: el Lanzar es de la tarjeta y del refutador: %#v", ca, l)
	}
}
