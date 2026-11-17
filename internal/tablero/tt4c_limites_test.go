// Tests hostiles de la v3 de hoom tablero: los casos limite del spec
// (.hoom/specs/tablero-tu-siguiente-paso.md, "Casos limite y errores
// esperados") que tocan el paso, Seguir solo, el grupo CORRIENDO, la vista
// Hallazgos, el mouse y la pantalla en cualquier tamano.
package tablero_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-716 y CA-715, "Dos pilotos para la misma tarjeta": el segundo hoom
// piloto sale con "el piloto de <slug> ya corre en otro proceso"; el
// Registro lo muestra en rojo, la linea de mensajes tambien, no se lanza
// nada mas y Seguir solo no se ofrece de nuevo mientras la parada sigue en
// pie.
func TestV3C_CA716_CA715_DosPilotos(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-716"
	const ultima = "el piloto de sola ya corre en otro proceso"
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfigColor(200, 40), sola)
	l := tv3cLanzaSeguir(t, ca, m, "sola")
	ttSinOrdenes(t, ca, "el fin del segundo piloto", tv3cFin(m, l.ID, 1, ultima))
	if a := tv3cAsiento(t, ca, m, "Seguir solo"); a.Texto != "Seguir solo · sola · fallo: "+ultima || a.Comando != "hoom piloto sola" {
		t.Fatalf("%s: la entrada del segundo piloto es {Seguir solo · sola · fallo: %s, hoom piloto sola}: %+v", ca, ultima, a)
	}
	crudo := m.Pantalla()[tv3cAlto(m)-3]
	if got := ttEstiloDe(crudo, "fallo: "+ultima); got != "\x1b[31m" {
		t.Fatalf("%s: el Registro muestra el fallo en rojo: %q en %q", ca, got, crudo)
	}
	if ttMensaje(m) != ultima || ttEstiloDe(ttMensajeCrudo(m), ultima) != "\x1b[31m" {
		t.Fatalf("%s: la linea de mensajes dice la ultima linea del piloto en rojo: %q", ca, ttMensajeCrudo(m))
	}
	tv3cLeer(m, sola)
	if g := m.Grupo("sola"); g != tablero.GrupoTeToca {
		t.Fatalf("%s: con el piloto terminado y sin trabajo la tarjeta vuelve a TE TOCA: %q", ca, g)
	}
	// la parada sigue en pie: P no abre y dice por que
	ttSinOrdenes(t, ca, "P tras la parada", ttTeclas(m, "P"))
	tv3cSinModal(t, ca, "P tras la parada", m)
	if got := ttMensaje(m); got != "el piloto se detuvo: "+ultima {
		t.Fatalf("%s: P tras la parada dice \"el piloto se detuvo: %s\": %q", ca, ultima, got)
	}
	if n := tv3cCuentaAsientos(m, "Seguir solo"); n != 1 {
		t.Fatalf("%s: una sola entrada de Seguir solo, hay %d", ca, n)
	}
}

// CA-711 y CA-715, "Se cierra el tablero con un piloto corriendo": un
// tablero que abre de nuevo no sabe del piloto de antes; la tarjeta esta en
// CORRIENDO mientras haya un trabajo en curso, sin "el piloto la lleva", y
// entre trabajo y trabajo vuelve a TE TOCA con Seguir solo.
func TestV3C_CA711_CA715_ElTableroNuevoNoVeElPilotoDeAntes(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-711"
	const ancho = 200
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(ancho, 40), tv3cConRunning(sola, "env-de-antes", "writer"))
	if g := m.Grupo("sola"); g != tablero.GrupoCorriendo {
		t.Fatalf("%s: con un trabajo en curso la tarjeta esta en CORRIENDO: %q", ca, g)
	}
	if got := tv3cSegunda(t, ca, m, ancho, "Sola"); got != "   writer · 0:30 de 45:00" {
		t.Fatalf("%s: un piloto que no lanzo este tablero no lleva \"el piloto la lleva\": %q", ca, got)
	}
	ttTeclas(m, "P")
	tv3cSinModal(t, ca, "P con un trabajo en curso", m)
	if got := ttMensaje(m); got != "la tarjeta ya tiene un trabajo en curso o cortado" {
		t.Fatalf("%s: P con running dice \"la tarjeta ya tiene un trabajo en curso o cortado\": %q", ca, got)
	}
	// entre trabajo y trabajo
	tv3cLeer(m, sola)
	if g := m.Grupo("sola"); g != tablero.GrupoTeToca {
		t.Fatalf("%s: entre trabajo y trabajo la tarjeta esta en TE TOCA: %q", ca, g)
	}
	if m.Corriendo() != 0 {
		t.Fatalf("%s: este tablero no lanzo nada: Corriendo es 0, es %d", ca, m.Corriendo())
	}
	tv3cLanzaSeguir(t, ca, m, "sola")
}

// CA-705 y CA-704, "La tarjeta de la caja desaparece en una lectura": la
// caja pasa a la siguiente con la regla de siempre y n abre el paso de esa.
func TestV3C_CA705_CA704_LaCajaDesaparece(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-705"
	// en TE TOCA, Writer va antes que Tu aprobacion
	a := tv3cSola("firma", "Firma", boardcmd.ColTuAprobacion)
	b := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(200, 40), a, b)
	if m.Caja() != "sola" {
		t.Fatalf("%s: la caja tiene la primera de TE TOCA (Writer antes que Tu aprobacion): %q", ca, m.Caja())
	}
	ttSinOrdenes(t, ca, "n", ttTeclas(m, "n"))
	if m.Modal() != tablero.ModalSeguir {
		t.Fatalf("%s: n abre el paso de sola (Seguir solo): %q, mensaje %q", ca, m.Modal(), ttMensaje(m))
	}
	ttTeclas(m, tablero.TeclaEsc)
	tv3cLeer(m, a)
	if m.Caja() != "firma" {
		t.Fatalf("%s: sola desaparece y la caja pasa a firma: %q", ca, m.Caja())
	}
	ttSinOrdenes(t, ca, "n", ttTeclas(m, "n"))
	if m.Modal() != tablero.ModalFirma {
		t.Fatalf("%s: n abre el paso de la nueva tarjeta de la caja (la firma): %q, mensaje %q", ca, m.Modal(), ttMensaje(m))
	}
	ttTeclas(m, tablero.TeclaEsc)
	tv3cLeer(m)
	if m.Caja() != "" {
		t.Fatalf("%s: sin tarjetas la caja no tiene tarjeta: %q", ca, m.Caja())
	}
	ttSinOrdenes(t, ca, "n sin tarjetas", ttTeclas(m, "n"))
	tv3cSinModal(t, ca, "n sin tarjetas", m)
	if got := ttMensaje(m); got != "no hay nada pendiente" {
		t.Fatalf("%s: n sin tarjetas dice \"no hay nada pendiente\": %q", ca, got)
	}
}

// CA-707, "La vista Hallazgos sin hallazgos": dice "sin hallazgos
// abiertos"; elegir, ir al detalle o al editor no da ordenes ni se rompe.
func TestV3C_CA707_LaVistaHallazgosSinHallazgos(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-707"
	m := ttModelo(tv3cConfig(100, 30), ttTableroDorado())
	m.LeerHallazgos(tablero.Hallazgos{})
	ttTeclas(m, "H")
	if m.Lugar() != tablero.LugarHallazgos || m.Vista() != tablero.VistaHallazgos {
		t.Fatalf("%s: H abre la vista Hallazgos aunque este vacia: %q %q", ca, m.Lugar(), m.Vista())
	}
	if !strings.Contains(strings.Join(tv3cInteriorLista(m, 100), "\n"), "sin hallazgos abiertos") {
		t.Fatalf("%s: la vista Hallazgos vacia dice \"sin hallazgos abiertos\":\n%s", ca, ttPantalla(m))
	}
	for _, k := range []tablero.Tecla{"j", "k", tablero.TeclaAbajo, tablero.TeclaArriba, "g", "G", "e", tablero.TeclaEnter, "e"} {
		ttSinOrdenes(t, ca, string(k)+" sin hallazgos", ttTeclas(m, k))
		if r, l := m.ArchivoElegido(); r != "" || l != 0 {
			t.Fatalf("%s: sin hallazgos no hay archivo elegido: %s:%d", ca, r, l)
		}
	}
	tv3cSinModal(t, ca, "las teclas sin hallazgos", m)
}

// CA-716 y CA-534: la ultima linea del piloto con controles (secuencias de
// escape, BEL, bidi) llega al Registro y a la linea de mensajes sin ningun
// control: la pantalla solo lleva las SGR del tablero.
func TestV3C_CA716_CA534_LaUltimaLineaHostilDelPiloto(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-716"
	for _, code := range []int{0, 1} {
		for _, ultima := range []string{tv3cPrefijoParada + ttHostil, ttHostil} {
			m := tv3cModelo(tv3cConfigColor(200, 40), tv3cSola("sola", "Sola", boardcmd.ColWriter))
			l := tv3cLanzaSeguir(t, ca, m, "sola")
			tv3cFin(m, l.ID, code, ultima)
			pantalla := ttSinControles(t, ca, fmt.Sprintf("fin %d con %q", code, ultima), m, true)
			if !strings.Contains(pantalla, ttNeutro) {
				t.Fatalf("%s: la ultima linea se muestra neutralizada (%q):\n%s", ca, ttNeutro, pantalla)
			}
			ttTeclas(m, "L")
			ttSinControles(t, ca, "el modal Registro", m, true)
		}
	}
}

// CA-719: un evento del mouse fuera de la pantalla (coordenadas negativas o
// mas alla de W y A) no da ordenes ni abre o cierra un modal.
func TestV3C_CA719_EventosFueraDeLaPantalla(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	for _, l := range []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarVerTarjeta, tablero.LugarHallazgos,
		tablero.LugarSeguir, tablero.LugarAyuda, tablero.LugarRegistro, tablero.LugarPedirSelector} {
		m := tv3cEstado(t, ca, l)
		modal := m.Modal()
		for _, p := range [][2]int{{-1, -1}, {-1, 5}, {5, -1}, {100, 10}, {10, 30}, {1000, 1000}, {-1000, 15}} {
			for _, a := range []tablero.AccionRaton{tablero.RatonClic, tablero.RatonRuedaAbajo, tablero.RatonRuedaArriba, 0, 99} {
				if ords := m.Raton(tablero.Raton{Accion: a, X: p[0], Y: p[1]}, ttAhora); len(ords) != 0 {
					t.Fatalf("%s: en %s el evento %d en (%d,%d) dio las ordenes %#v", ca, l, a, p[0], p[1], ords)
				}
				if m.Modal() != modal {
					t.Fatalf("%s: en %s el evento %d en (%d,%d) cambio el modal de %q a %q", ca, l, a, p[0], p[1], modal, m.Modal())
				}
			}
		}
	}
}

// CA-718 y CA-714: en cada Lugar y en cada tamano (de 60x15 en adelante) la
// pantalla tiene A lineas de a lo sumo W celdas, la linea de teclas entra en
// W y el Registro plegado ocupa su linea entera; cambiar el tamano no cambia
// el Lugar ni el modal.
func TestV3C_CA718_CA714_CadaTamanoEntra(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-718"
	tamanos := [][2]int{{60, 15}, {61, 16}, {79, 24}, {80, 24}, {99, 30}, {100, 30}, {101, 31}, {120, 36}, {137, 41}, {200, 60}}
	for _, l := range tv3cLugaresProbados {
		for _, tm := range tamanos {
			m := tv3cEstado(t, ca, l)
			modal := m.Modal()
			m.Tamano(tm[0], tm[1])
			if m.Lugar() != l || m.Modal() != modal {
				t.Fatalf("%s: en %s, el tamano %dx%d cambio el Lugar o el modal: %q %q", ca, l, tm[0], tm[1], m.Lugar(), m.Modal())
			}
			ls := ttLimpia(m)
			if len(ls) != tm[1] {
				t.Fatalf("%s: en %s a %dx%d la pantalla tiene %d lineas, tiene %d", ca, l, tm[0], tm[1], tm[1], len(ls))
			}
			for i, s := range ls {
				if ttCeldas(s) > tm[0] {
					t.Fatalf("%s: en %s a %dx%d la linea %d tiene %d celdas (mas de %d): %q", ca, l, tm[0], tm[1], i+1, ttCeldas(s), tm[0], s)
				}
			}
			if modal == tablero.SinModal {
				if r := ls[tm[1]-3]; ttCeldas(r) != tm[0] || !strings.HasPrefix(r, "-Registro") {
					t.Fatalf("%s: en %s a %dx%d el Registro plegado ocupa su linea: %q", ca, l, tm[0], tm[1], r)
				}
			}
		}
	}
}
