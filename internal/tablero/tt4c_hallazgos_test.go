// Tests de los tres defectos que encontro la review de 4 lentes en la v3 de
// hoom tablero, escritos desde el contrato (doc.go, "The signature with the
// pilot on (CA-717)", "Firma (A, I, ...; CA-574, CA-717)" y "A pilot's
// stop:" en Seguir solo) y desde las firmas publicas (Lectura.Pedida):
//
//  1. una firma con el piloto cuya linea "y despues: hoom piloto <slug>"
//     no entra entera a lo ancho nunca se firma; el comando de la accion
//     cortado, por si solo, no la frena (CA-717, CA-574);
//  2. un trabajo que el tablero lanzo sobre la tarjeta levanta la parada del
//     piloto al terminar, aunque ninguna lectura lo haya visto correr; el que
//     no pudo arrancar (su FinProceso con codigo -1) no la levanta (CA-715,
//     hallazgos 755290 y a0d940);
//  3. la parada toma la columna de la primera lectura empezada despues del
//     fin del piloto (CA-715, CA-717);
//  4. si el tablero se cierra mientras corre el hoom aprobar de una firma con
//     el piloto, el piloto no se lanza y hoom tablero lo dice al salir
//     (CA-717, decision de Henry sobre el hallazgo 98ba68):
//     Modelo.PilotosSinLanzar, MensajePilotos y la salida de Run.
package tablero_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ===========================================================================
// 1. CA-717 y CA-574: la firma que no entra entera a lo ancho
// ===========================================================================

// tv3cHMsgNoEntra es lo que dice enter sobre la accion de una firma que no
// entra entera (doc.go, Firma).
const tv3cHMsgNoEntra = "agranda la terminal: la firma no entra entera"

// tv3cHSlug es un slug de n caracteres.
func tv3cHSlug(n int) string {
	return "firma-" + strings.Repeat("x", n-6)
}

// tv3cHFirmaLarga es una tarjeta en Tu aprobacion con aprobar, con o sin el
// piloto encendido, de slug s.
func tv3cHFirmaLarga(s string, auto bool) boardcmd.Card {
	c := tv3cSola(s, "Larga", boardcmd.ColTuAprobacion)
	if !auto {
		c = tv3cSinAuto(c)
	}
	return c
}

// tv3cHAbreFirma abre la firma con k (A, o enter y n, cuyo paso es aprobar)
// y elige el boton de la accion.
func tv3cHAbreFirma(t *testing.T, ca, que string, m *tablero.Modelo, k tablero.Tecla) {
	t.Helper()
	ttSinOrdenes(t, ca, que+": "+string(k), ttTeclas(m, k))
	if m.Modal() != tablero.ModalFirma {
		t.Fatalf("%s: %s: %s abre la firma: modal %q, mensaje %q", ca, que, k, m.Modal(), ttMensaje(m))
	}
	ttSinOrdenes(t, ca, que+": ->", ttTeclas(m, tablero.TeclaDer))
}

// tv3cHTextoFirma es el ancho del texto del modal: Wm - 6, Wm = min(72, W-4).
func tv3cHTextoFirma(ancho int) int { return ttAnchoModal(ancho) - 6 }

// CA-717 y CA-574: con el piloto encendido, el comando y la linea del piloto
// que no entran en una linea se parten por palabras en dos (la segunda desde
// "  ") y, si las dos entran enteras, la firma se firma: enter sobre la
// accion da el Lanzar de hoom aprobar y su fin con 0 el del piloto. Los
// casos son el slug de 40 en 60 columnas (50 celdas de texto), el mas largo
// que entra en 60 (43: "  <slug> --si" son 50 celdas) y el mas largo que
// entra en el modal mas ancho (59, con 66 celdas de texto).
func TestV3C_CA717_CA574_LaFirmaPartidaQueEntraSeFirma(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-717"
	for _, c := range []struct {
		ancho, largo int
	}{{60, 40}, {60, 43}, {61, 43}, {200, 59}} {
		s := tv3cHSlug(c.largo)
		que := fmt.Sprintf("slug de %d a %d columnas", c.largo, c.ancho)
		if tv3cHTextoFirma(c.ancho) < len("  "+s+" --si") {
			t.Fatalf("%s: precondicion: %q entra en %d celdas", ca, "  "+s+" --si", tv3cHTextoFirma(c.ancho))
		}
		m := tv3cModelo(tv3cConfigColor(c.ancho, 40), tv3cHFirmaLarga(s, true))
		tv3cHAbreFirma(t, ca, que, m, "A")
		ls := tv3cLineasFirma(t, ca, m, c.ancho)
		quiero := []string{"Se va a ejecutar:", "hoom aprobar", "  " + s + " --si", "y despues: hoom piloto", "  " + s, ""}
		if len(ls) != len(quiero)+1 {
			t.Fatalf("%s: [%s] desde \"Se va a ejecutar:\" van %q y los botones, van %q", ca, que, quiero, ls)
		}
		for i, q := range quiero {
			if ls[i] != q {
				t.Fatalf("%s: [%s] la linea %d de la firma es %q, es %q (%q)", ca, que, i, q, ls[i], ls)
			}
		}
		if got := strings.TrimSpace(ls[len(ls)-1]); got != "Cancelar     [ Aprobar spec ]" {
			t.Fatalf("%s: [%s] el boton de la accion quedo elegido: %q", ca, que, got)
		}
		ap := ttUnaLanzar(t, ca, que+": enter sobre Aprobar spec", ttTeclas(m, tablero.TeclaEnter))
		ttMismosArgs(t, ca, que+": aprobar", ap.Args, []string{"aprobar", s, "--si"})
		pl := ttUnaLanzar(t, ca, que+": el fin con 0 de aprobar", tv3cFin(m, ap.ID, 0, ""))
		if pl.Rol != "piloto" || pl.Slug != s {
			t.Fatalf("%s: [%s] el fin con 0 encadena el piloto de %s: %+v", ca, que, s, pl)
		}
		ttMismosArgs(t, ca, que+": el piloto", pl.Args, []string{"piloto", s})
	}
}

// CA-717 y CA-574: el comando de la accion conserva la regla de la v2: un
// "hoom aprobar <slug> --si" que no entra entero ni en dos lineas (en 60
// columnas, un slug de 44 o mas: "  <slug> --si" pasa de 50 celdas; en el
// modal mas ancho, uno de 60 a 64) no frena la firma por si solo. Con la
// linea del piloto entera (partida en "y despues: hoom piloto" y
// "  <slug>"), o sin piloto, y abra la firma A, enter o n: enter sobre la
// accion da el Lanzar de hoom aprobar, la linea de mensajes no dice
// "agranda la terminal: la firma no entra entera", y con auto su fin con 0
// encadena el piloto. El de 48 en 60 columnas es el borde: "  <slug>" son
// justo 50 celdas.
func TestV3C_CA717_CA574_ElComandoCortadoSoloNoFrenaLaFirma(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-717"
	for _, c := range []struct {
		ancho, largo int
		auto         bool
	}{
		{60, 44, true}, {60, 44, false}, {60, 48, true}, {200, 60, true}, {200, 60, false}, {200, 64, true}, {200, 64, false},
	} {
		s := tv3cHSlug(c.largo)
		if len("  "+s+" --si") <= tv3cHTextoFirma(c.ancho) || len("  "+s) > tv3cHTextoFirma(c.ancho) {
			t.Fatalf("%s: precondicion: %q no entra en %d celdas y %q si", ca, "  "+s+" --si", tv3cHTextoFirma(c.ancho), "  "+s)
		}
		for _, k := range []tablero.Tecla{"A", tablero.TeclaEnter, "n"} {
			que := fmt.Sprintf("slug de %d a %d columnas, auto %v, %s", c.largo, c.ancho, c.auto, k)
			m := tv3cModelo(tv3cConfigColor(c.ancho, 40), tv3cHFirmaLarga(s, c.auto))
			tv3cHAbreFirma(t, ca, que, m, k)
			ls := tv3cLineasFirma(t, ca, m, c.ancho)
			piloto := -1
			for i, l := range ls {
				if strings.Contains(l, "hoom piloto") {
					piloto = i
					break
				}
			}
			switch {
			case c.auto && (piloto < 0 || ls[piloto] != "y despues: hoom piloto" || piloto+1 >= len(ls) || ls[piloto+1] != "  "+s):
				t.Fatalf("%s: [%s] la linea del piloto entra entera en dos: \"y despues: hoom piloto\" y %q: %q", ca, que, "  "+s, ls)
			case !c.auto && piloto >= 0:
				t.Fatalf("%s: [%s] sin auto la firma no tiene la linea del piloto: %q", ca, que, ls)
			}
			ap := ttUnaLanzar(t, ca, que+": enter sobre Aprobar spec", ttTeclas(m, tablero.TeclaEnter))
			ttMismosArgs(t, ca, que+": aprobar", ap.Args, []string{"aprobar", s, "--si"})
			if strings.Contains(ttMensaje(m), tv3cHMsgNoEntra) {
				t.Fatalf("%s: [%s] un comando de la accion cortado no frena la firma: %q", ca, que, ttMensaje(m))
			}
			if n := tv3cCuentaAsientos(m, "Aprobar spec"); n != 1 {
				t.Fatalf("%s: [%s] la firma tiene su entrada en el Registro: %+v", ca, que, m.Registro())
			}
			ords := tv3cFin(m, ap.ID, 0, "")
			if !c.auto {
				ttSinOrdenes(t, ca, que+": el fin con 0 de aprobar sin auto", ords)
				continue
			}
			pl := ttUnaLanzar(t, ca, que+": el fin con 0 de aprobar", ords)
			if pl.Rol != "piloto" || pl.Slug != s {
				t.Fatalf("%s: [%s] el fin con 0 encadena el piloto de %s: %+v", ca, que, s, pl)
			}
			ttMismosArgs(t, ca, que+": el piloto", pl.Args, []string{"piloto", s})
		}
	}
}

// CA-717 y CA-574: cuando la linea "y despues: hoom piloto <slug>" no entra
// ni partida (en 60 columnas, "  <slug>" de 51 celdas o mas: un slug de 49
// o mas, hasta 64), la firma no se firma: enter sobre la accion no lanza
// nada y dice en rojo "agranda la terminal: la firma no entra entera".
func TestV3C_CA717_CA574_LaLineaDelPilotoCortadaNoSeFirma(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-717"
	for _, largo := range []int{49, 50, 64} {
		s := tv3cHSlug(largo)
		if len("  "+s) <= tv3cHTextoFirma(60) {
			t.Fatalf("%s: precondicion: %q no entra en %d celdas", ca, "  "+s, tv3cHTextoFirma(60))
		}
		m := tv3cModelo(tv3cConfigColor(60, 40), tv3cHFirmaLarga(s, true))
		tv3cHAbreFirma(t, ca, s, m, "A")
		ttSinOrdenes(t, ca, s+": enter sobre Aprobar spec", ttTeclas(m, tablero.TeclaEnter))
		ttDiceMensaje(t, ca, s+": enter sobre Aprobar spec", m, true, tv3cHMsgNoEntra)
		if n := tv3cCuentaAsientos(m, "Seguir solo") + tv3cCuentaAsientos(m, "Aprobar spec"); n != 0 {
			t.Fatalf("%s: [%s] ni la firma ni el piloto tienen entrada en el Registro: %+v", ca, s, m.Registro())
		}
		if m.Corriendo() != 0 {
			t.Fatalf("%s: [%s] no corre nada: Corriendo es %d", ca, s, m.Corriendo())
		}
	}
}

// ===========================================================================
// 2. CA-715: un trabajo lanzado por el tablero levanta la parada
// ===========================================================================

// tv3cHParada lanza Seguir solo sobre sola (en Writer, con auto; otras
// tarjetas pueden ir detras) y lo detiene con codigo 1; la lectura que
// sigue la deja en la misma columna, sin running. Devuelve el mensaje de la
// parada.
func tv3cHParada(t *testing.T, m *tablero.Modelo, cards ...boardcmd.Card) string {
	t.Helper()
	l := tv3cLanzaSeguir(t, "CA-715", m, "sola")
	tv3cFin(m, l.ID, 1, tv3cPrefijoParada+"un rojo final")
	tv3cLeer(m, cards...)
	msg := "el piloto se detuvo: un rojo final"
	tv3cPNoAbre(t, "CA-715", "con la parada en pie", m, msg)
	return msg
}

// tv3cHPedirWriter pide al writer sobre la tarjeta elegida desde su modal
// Pedir (w, enter al boton, enter) y devuelve el Lanzar.
func tv3cHPedirWriter(t *testing.T, m *tablero.Modelo, slug string) tablero.Lanzar {
	t.Helper()
	ttSinOrdenes(t, "CA-715", "w", ttTeclas(m, "w"))
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-715: w abre Pedir: modal %q, mensaje %q", m.Modal(), ttMensaje(m))
	}
	ttTeclas(m, tablero.TeclaEnter)
	l := ttUnaLanzar(t, "CA-715", "enter en el boton de Pedir", ttTeclas(m, tablero.TeclaEnter))
	if l.Rol != "writer" || l.Slug != slug {
		t.Fatalf("CA-715: el Lanzar del writer es {Rol: writer, Slug: %s}: %+v", slug, l)
	}
	return l
}

// CA-715: el piloto se detiene con 1; el tablero lanza el writer sobre la
// tarjeta y su FinProceso llega sin que ninguna lectura lo haya mostrado
// corriendo: con cualquier codigo, la parada se levanta y P vuelve a abrir
// Seguir solo (antes de otra lectura y despues de una sin cambios). Mientras
// el writer no termino, la parada sigue en pie.
func TestV3C_CA715_ElTrabajoQueTerminaLevantaLaParada(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-715"
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	for _, code := range []int{0, 1} {
		m := tv3cModelo(tv3cConfig(200, 40), sola)
		msg := tv3cHParada(t, m, sola)
		w := tv3cHPedirWriter(t, m, "sola")
		tv3cPNoAbre(t, ca, "con el writer lanzado y sin terminar", m, msg)
		tv3cLeer(m, sola)
		tv3cPNoAbre(t, ca, "con el writer sin terminar y una lectura sin running", m, msg)
		ttSinOrdenes(t, ca, "el fin del writer", tv3cFin(m, w.ID, code, "listo"))
		tv3cPAbre(t, ca, "tras el fin del writer, sin lectura", m)
		tv3cLeer(m, sola)
		tv3cPAbre(t, ca, "tras el fin del writer y una lectura sin running", m)
	}
}

// CA-715: un trabajo que el tablero lanzo antes de la parada y termina
// despues tambien la levanta ("no job this board launched on it has ended
// since").
func TestV3C_CA715_UnTrabajoDeAntesQueTerminaDespuesLaLevanta(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-715"
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(200, 40), sola)
	pl := tv3cLanzaSeguir(t, ca, m, "sola")
	w := tv3cHPedirWriter(t, m, "sola")
	tv3cFin(m, pl.ID, 1, tv3cPrefijoParada+"un rojo final")
	tv3cLeer(m, sola)
	tv3cPNoAbre(t, ca, "con la parada en pie y el writer sin terminar", m, "el piloto se detuvo: un rojo final")
	tv3cFin(m, w.ID, 0, "")
	tv3cPAbre(t, ca, "tras el fin del writer", m)
}

// CA-715: lo que no es un trabajo sobre la tarjeta no levanta la parada: un
// verbo sin rol (u, hoom usar: Rol "") que termina, ni el writer de otra
// tarjeta que termina.
func TestV3C_CA715_LoQueNoEsUnTrabajoNoLevantaLaParada(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-715"
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	otra := ttTarjetaCon(ttAccionWriter())
	m := tv3cModelo(tv3cConfig(200, 40), sola, otra)
	if m.Elegida() != "sola" {
		t.Fatalf("%s: precondicion del spec: en TE TOCA, en la misma columna, va el orden del tablero: elegida %q", ca, m.Elegida())
	}
	msg := tv3cHParada(t, m, sola, otra)
	u := ttUnaLanzar(t, ca, "u", ttTeclas(m, "u"))
	if u.Rol != "" || u.Slug != "sola" {
		t.Fatalf("%s: precondicion del spec: u lanza hoom usar sin rol sobre sola: %+v", ca, u)
	}
	ttSinOrdenes(t, ca, "el fin de hoom usar", tv3cFin(m, u.ID, 0, ""))
	tv3cPNoAbre(t, ca, "tras el fin de un verbo sin rol", m, msg)
	// el writer de la otra tarjeta
	ttTeclas(m, "j")
	if m.Elegida() != "tarjeta" {
		t.Fatalf("%s: precondicion del spec: j elige la otra tarjeta: %q", ca, m.Elegida())
	}
	w := tv3cHPedirWriter(t, m, "tarjeta")
	tv3cFin(m, w.ID, 0, "")
	tv3cLeer(m, sola, otra)
	ttTeclas(m, "k")
	if m.Elegida() != "sola" {
		t.Fatalf("%s: precondicion del spec: k vuelve a sola: %q", ca, m.Elegida())
	}
	tv3cPNoAbre(t, ca, "tras el fin del writer de otra tarjeta", m, msg)
}

// tv3cHAhoraDetenido es la frase de AHORA de una tarjeta cuyo piloto se
// detuvo con la parada en pie (doc.go, AHORA: "a pilot of this board that
// stopped, its stop still standing"; la linea gris es el motivo).
const tv3cHAhoraDetenido = "El piloto se detuvo."

// tv3cHAhoraDice dice si el detalle (en 200 columnas) tiene la frase de AHORA
// "El piloto se detuvo." seguida de la linea gris motivo.
func tv3cHAhoraDice(m *tablero.Modelo, motivo string) bool {
	ls := tv3cInteriorDetalle(m, 200)
	for i := 0; i+1 < len(ls); i++ {
		if strings.TrimSpace(ls[i]) == tv3cHAhoraDetenido && strings.TrimSpace(ls[i+1]) == motivo {
			return true
		}
	}
	return false
}

// tv3cHParadaEnPie exige la parada en pie, como la ven P y la pantalla: P no
// abre nada, no ordena nada y la linea de mensajes dice msg; el AHORA del
// detalle dice "El piloto se detuvo." y su motivo.
func tv3cHParadaEnPie(t *testing.T, ca, que string, m *tablero.Modelo, msg, motivo string) {
	t.Helper()
	tv3cPNoAbre(t, ca, que, m, msg)
	if !tv3cHAhoraDice(m, motivo) {
		t.Fatalf("%s: [%s] con la parada en pie el AHORA del detalle dice %q y %q:\n%s", ca, que,
			tv3cHAhoraDetenido, motivo, strings.Join(tv3cInteriorDetalle(m, 200), "\n"))
	}
}

// CA-715, hallazgos 755290 y a0d940: un rol que no llego a arrancar no corrio
// sobre la tarjeta, asi que su fin no levanta la parada del piloto (doc.go,
// "A pilot's stop": "A job that could not start (its FinProceso has code -1)
// did not run: its end does not lift the stop"). El piloto que el tablero
// lanzo sobre sola se detiene con 1 ("un rojo final"); el tablero lanza el
// writer sobre la misma tarjeta y su FinProceso llega con -1. La parada sigue
// en pie con el writer sin terminar, tras su fin con -1 sin otra lectura y
// tras una lectura sin cambios: P no abre nada, no ordena nada y dice "el
// piloto se detuvo: un rojo final"; el AHORA del detalle dice "El piloto se
// detuvo." y "un rojo final"; y enter no abre Seguir solo (no es el paso).
func TestV3C_CA715_UnRolQueNoArrancoNoLevantaLaParada(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-715"
	const motivo = "un rojo final"
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(200, 40), sola)
	msg := tv3cHParada(t, m, sola)
	tv3cHParadaEnPie(t, ca, "con la parada del piloto", m, msg, motivo)
	w := tv3cHPedirWriter(t, m, "sola")
	tv3cHParadaEnPie(t, ca, "con el writer lanzado y sin terminar", m, msg, motivo)
	ttSinOrdenes(t, ca, "el fin con -1 del writer", tv3cFin(m, w.ID, -1, "no se pudo arrancar el proceso"))
	tv3cHParadaEnPie(t, ca, "tras el fin con -1 del writer, sin lectura", m, msg, motivo)
	tv3cLeer(m, sola)
	tv3cHParadaEnPie(t, ca, "tras el fin con -1 del writer y una lectura sin running", m, msg, motivo)
	if got := ttTeclas(m, tablero.TeclaEnter); len(got) != 0 || m.Modal() == tablero.ModalSeguir {
		t.Fatalf("%s: con la parada en pie Seguir solo no es el paso: enter dio %#v, modal %q", ca, got, m.Modal())
	}
}

// CA-715, hallazgos 755290 y a0d940, el otro lado: un rol que arranco y fallo
// (codigo 1, o 2) si corrio sobre la tarjeta, y su fin levanta la parada;
// solo -1 dice que no arranco. Mismo armado que el anterior: con el writer
// sin terminar la parada sigue en pie; tras su fin, P abre Seguir solo
// (antes de otra lectura y despues de una sin cambios) y el AHORA del detalle
// ya no dice "El piloto se detuvo.".
func TestV3C_CA715_UnRolQueArrancoYFallaLevantaLaParada(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-715"
	const motivo = "un rojo final"
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	for _, code := range []int{1, 2} {
		que := fmt.Sprintf("tras el fin con %d del writer", code)
		m := tv3cModelo(tv3cConfig(200, 40), sola)
		msg := tv3cHParada(t, m, sola)
		w := tv3cHPedirWriter(t, m, "sola")
		tv3cHParadaEnPie(t, ca, "con el writer lanzado y sin terminar", m, msg, motivo)
		ttSinOrdenes(t, ca, "el fin del writer", tv3cFin(m, w.ID, code, "el writer fallo"))
		tv3cPAbre(t, ca, que+", sin lectura", m)
		if tv3cDetalleTiene(m, 200, tv3cHAhoraDetenido) {
			t.Fatalf("%s: [%s, sin lectura] sin la parada el AHORA no dice %q:\n%s", ca, que, tv3cHAhoraDetenido,
				strings.Join(tv3cInteriorDetalle(m, 200), "\n"))
		}
		tv3cLeer(m, sola)
		tv3cPAbre(t, ca, que+" y una lectura sin running", m)
		if tv3cDetalleTiene(m, 200, tv3cHAhoraDetenido) {
			t.Fatalf("%s: [%s y una lectura] sin la parada el AHORA no dice %q:\n%s", ca, que, tv3cHAhoraDetenido,
				strings.Join(tv3cInteriorDetalle(m, 200), "\n"))
		}
	}
}

// ===========================================================================
// 3. CA-715 y CA-717: la columna de la parada es la de la primera lectura
// empezada despues del fin del piloto
// ===========================================================================

// tv3cHLeerEn lee el tablero a la hora ahora, empezada a la hora pedida.
func tv3cHLeerEn(m *tablero.Modelo, ahora, pedida time.Time, cards ...boardcmd.Card) {
	m.Leer(tablero.Lectura{Ahora: ahora, Pedida: pedida, Tablero: ttTablero(cards...)})
}

// tv3cHPNoAbreEn aprieta P a la hora ahora y exige que no abra nada, no
// ordene nada y diga msg.
func tv3cHPNoAbreEn(t *testing.T, ca, que string, m *tablero.Modelo, ahora time.Time, msg string) {
	t.Helper()
	ttSinOrdenes(t, ca, "P "+que, m.Tecla("P", ahora))
	tv3cSinModal(t, ca, "P "+que, m)
	ttDiceMensaje(t, ca, "P "+que, m, false, msg)
}

// tv3cHPAbreEn aprieta P a la hora ahora y exige Seguir solo; lo cierra con
// esc.
func tv3cHPAbreEn(t *testing.T, ca, que string, m *tablero.Modelo, ahora time.Time) {
	t.Helper()
	ttSinOrdenes(t, ca, "P "+que, m.Tecla("P", ahora))
	if m.Modal() != tablero.ModalSeguir {
		t.Fatalf("%s: P %s abre Seguir solo: modal %q, mensaje %q", ca, que, m.Modal(), ttMensaje(m))
	}
	m.Tecla(tablero.TeclaEsc, ahora)
	tv3cSinModal(t, ca, "esc en Seguir solo", m)
}

// tv3cHFirmaYParada firma "pila" (Tu aprobacion, con auto) a la hora
// ttAhora, termina aprobar con 0 a t1 (se encadena el piloto) y el piloto
// con 1 a t2, antes de ninguna lectura. Devuelve t2 y el mensaje de la
// parada.
func tv3cHFirmaYParada(t *testing.T, m *tablero.Modelo) (time.Time, string) {
	t.Helper()
	const ca = "CA-717"
	t1, t2 := ttAhora.Add(time.Minute), ttAhora.Add(2*time.Minute)
	tv3cHAbreFirma(t, ca, "pila", m, "A")
	ap := ttUnaLanzar(t, ca, "enter sobre Aprobar spec", ttTeclas(m, tablero.TeclaEnter))
	ttMismosArgs(t, ca, "aprobar", ap.Args, []string{"aprobar", "pila", "--si"})
	pl := ttUnaLanzar(t, ca, "el fin con 0 de aprobar", m.Termino(tablero.FinProceso{ID: ap.ID, Code: 0}, t1))
	if pl.Rol != "piloto" || pl.Slug != "pila" {
		t.Fatalf("%s: el fin con 0 de aprobar encadena el piloto de pila: %+v", ca, pl)
	}
	ttSinOrdenes(t, ca, "el fin del piloto", m.Termino(tablero.FinProceso{ID: pl.ID, Code: 1,
		Ultima: tv3cPrefijoParada + "tope de presupuesto"}, t2))
	return t2, "el piloto se detuvo: tope de presupuesto"
}

// CA-715 y CA-717, el caso real: A sobre una tarjeta en Tu aprobacion con
// auto, aprobar termina con 0, se encadena el piloto y el piloto se detiene
// con 1 antes de la lectura siguiente; esa lectura (empezada despues del
// fin, a la misma hora, o con Pedida cero) muestra la tarjeta ya en
// Test-writer y la parada sigue en pie; una lectura posterior en otra
// columna la levanta.
func TestV3C_CA715_CA717_LaParadaTomaLaColumnaDeLaLecturaSiguiente(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-715"
	apr := tv3cSola("pila", "Pila", boardcmd.ColTuAprobacion)
	tw := tv3cSola("pila", "Pila", boardcmd.ColTestWriter)
	wr := tv3cSola("pila", "Pila", boardcmd.ColWriter)
	for _, c := range []struct {
		nombre string
		pedida func(t2 time.Time) time.Time
	}{
		{"empezada despues del fin", func(t2 time.Time) time.Time { return t2.Add(time.Second) }},
		{"empezada a la hora del fin", func(t2 time.Time) time.Time { return t2 }},
		{"con Pedida cero", func(time.Time) time.Time { return time.Time{} }},
	} {
		m := tv3cModelo(tv3cConfig(200, 40), apr)
		t2, msg := tv3cHFirmaYParada(t, m)
		t3 := t2.Add(time.Minute)
		tv3cHLeerEn(m, t3, c.pedida(t2), tw)
		tv3cHPNoAbreEn(t, ca, "["+c.nombre+"] en Test-writer, la columna de la primera lectura tras la parada", m, t3, msg)
		t4 := t3.Add(time.Minute)
		tv3cHLeerEn(m, t4, t3, tw)
		tv3cHPNoAbreEn(t, ca, "["+c.nombre+"] tras otra lectura en Test-writer", m, t4, msg)
		t5 := t4.Add(time.Minute)
		tv3cHLeerEn(m, t5, t4, wr)
		tv3cHPAbreEn(t, ca, "["+c.nombre+"] tras pasar a Writer", m, t5)
	}
}

// CA-715 y CA-717: una lectura empezada antes del fin del piloto (una
// lectura vieja que estaba en vuelo) no le fija la columna a la parada: la
// fija la primera empezada despues. Con la vieja en Tu aprobacion y la
// nueva en Test-writer, la parada queda en Test-writer; con la vieja en
// Test-writer y la nueva en Writer, queda en Writer.
func TestV3C_CA715_CA717_UnaLecturaViejaNoFijaLaColumna(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-715"
	apr := tv3cSola("pila", "Pila", boardcmd.ColTuAprobacion)
	tw := tv3cSola("pila", "Pila", boardcmd.ColTestWriter)
	wr := tv3cSola("pila", "Pila", boardcmd.ColWriter)
	rv := tv3cSola("pila", "Pila", boardcmd.ColReview)
	for _, c := range []struct {
		nombre             string
		vieja, nueva, otra boardcmd.Card
	}{
		{"vieja en Tu aprobacion, nueva en Test-writer", apr, tw, wr},
		{"vieja en Test-writer, nueva en Writer", tw, wr, rv},
	} {
		m := tv3cModelo(tv3cConfig(200, 40), apr)
		t2, msg := tv3cHFirmaYParada(t, m)
		t3 := t2.Add(time.Minute)
		tv3cHLeerEn(m, t3, t2.Add(-time.Second), c.vieja)
		t4 := t3.Add(time.Minute)
		tv3cHLeerEn(m, t4, t3, c.nueva)
		tv3cHPNoAbreEn(t, ca, "["+c.nombre+"] en la columna de la primera lectura empezada tras la parada", m, t4, msg)
		t5 := t4.Add(time.Minute)
		tv3cHLeerEn(m, t5, t4, c.otra)
		tv3cHPAbreEn(t, ca, "["+c.nombre+"] tras pasar a otra columna", m, t5)
	}
}

// ===========================================================================
// 4. CA-717: el piloto que no se lanza porque el tablero se cerro
// ===========================================================================

// tv3cHMsgPiloto es la linea que hoom tablero escribe al salir por la tarjeta
// slug (doc.go, CA-717; MensajePilotos).
func tv3cHMsgPiloto(slug string) string {
	return "el piloto de " + slug + " no se lanza solo: cuando termine la firma, hoom piloto " + slug
}

// tv3cHPilotos exige que PilotosSinLanzar sean justo esos slugs, en ese orden
// (ninguno: vacio o nil).
func tv3cHPilotos(t *testing.T, que string, m *tablero.Modelo, quiero ...string) {
	t.Helper()
	if got := m.PilotosSinLanzar(); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", quiero) {
		t.Fatalf("CA-717: [%s] PilotosSinLanzar es %q, es %q", que, quiero, got)
	}
}

// tv3cHElegir lleva la seleccion de la lista a la tarjeta slug con k y j.
func tv3cHElegir(t *testing.T, m *tablero.Modelo, slug string) {
	t.Helper()
	for i := 0; i < 10 && m.Elegida() != slug; i++ {
		ttTeclas(m, "k")
	}
	for i := 0; i < 20 && m.Elegida() != slug; i++ {
		ttTeclas(m, "j")
	}
	if m.Elegida() != slug {
		t.Fatalf("CA-717: j y k llevan la seleccion a %s; elegida %q", slug, m.Elegida())
	}
}

// tv3cHFirmar firma la tarjeta slug, ya elegida: A abre la firma, -> elige el
// boton de la accion y enter da el Lanzar de hoom aprobar <slug> --si, sin
// rol (doc.go, Firma).
func tv3cHFirmar(t *testing.T, m *tablero.Modelo, slug string) tablero.Lanzar {
	t.Helper()
	ttSinOrdenes(t, "CA-717", "A sobre "+slug, ttTeclas(m, "A"))
	if m.Modal() != tablero.ModalFirma {
		t.Fatalf("CA-717: A sobre %s abre la firma: modal %q, mensaje %q", slug, m.Modal(), ttMensaje(m))
	}
	ttSinOrdenes(t, "CA-717", "-> en la firma de "+slug, ttTeclas(m, tablero.TeclaDer))
	ap := ttUnaLanzar(t, "CA-717", "enter sobre Aprobar spec de "+slug, ttTeclas(m, tablero.TeclaEnter))
	ttMismosArgs(t, "CA-717", "la firma de "+slug, ap.Args, []string{"aprobar", slug, "--si"})
	if ap.Rol != "" || ap.Slug != slug {
		t.Fatalf("CA-717: hoom aprobar no lanza un rol y es de %s: %+v", slug, ap)
	}
	return ap
}

// CA-717: PilotosSinLanzar tiene la tarjeta de una firma con el piloto desde
// el Lanzar de su hoom aprobar hasta su FinProceso: no mientras el modal esta
// abierto ni cuando se cierra sin firmar (esc, o enter sobre Cancelar), y la
// pierde con el fin de su aprobar, con 0 (el piloto ya se lanzo) y con otro
// codigo (no hay piloto que lanzar). El piloto lo encadena lo que dijo el
// modal al firmar: una lectura de despues sin auto no saca la tarjeta.
func TestV3C_CA717_PilotosSinLanzarMientrasCorreLaFirma(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-717"
	m := tv3cModelo(tv3cConfig(200, 40), tv3cNucAuto())
	tv3cHPilotos(t, "al empezar", m)
	for _, cerrar := range [][]tablero.Tecla{{tablero.TeclaEsc}, {tablero.TeclaEnter}} {
		ttSinOrdenes(t, ca, "A", ttTeclas(m, "A"))
		if m.Modal() != tablero.ModalFirma {
			t.Fatalf("%s: A abre la firma: modal %q, mensaje %q", ca, m.Modal(), ttMensaje(m))
		}
		tv3cHPilotos(t, "con la firma abierta", m)
		ttSinOrdenes(t, ca, "cerrar la firma sin firmar", ttTeclas(m, cerrar...))
		tv3cSinModal(t, ca, "tras cerrar la firma sin firmar", m)
		tv3cHPilotos(t, "tras cerrar la firma sin firmar", m)
	}
	ap := tv3cHFirmar(t, m, "nuc-tenancy")
	tv3cHPilotos(t, "con hoom aprobar corriendo", m, "nuc-tenancy")
	tv3cLeer(m, ttNucTenancy())
	tv3cHPilotos(t, "tras una lectura sin auto con hoom aprobar corriendo", m, "nuc-tenancy")
	p := ttUnaLanzar(t, ca, "el fin con 0 de hoom aprobar", tv3cFin(m, ap.ID, 0, "spec aprobado"))
	if p.Rol != "piloto" || p.Slug != "nuc-tenancy" {
		t.Fatalf("%s: el fin con 0 lanza el piloto de nuc-tenancy: %+v", ca, p)
	}
	tv3cHPilotos(t, "tras el fin con 0 de hoom aprobar", m)
	tv3cFin(m, p.ID, 0, "")
	tv3cHPilotos(t, "tras el fin del piloto", m)

	for _, code := range []int{1, 2, -1} {
		que := fmt.Sprintf("tras el fin con %d de hoom aprobar", code)
		m := tv3cModelo(tv3cConfig(200, 40), tv3cNucAuto())
		ap := tv3cHFirmar(t, m, "nuc-tenancy")
		tv3cHPilotos(t, "con hoom aprobar corriendo", m, "nuc-tenancy")
		ttSinOrdenes(t, ca, que, tv3cFin(m, ap.ID, code, "no se pudo firmar"))
		tv3cHPilotos(t, que, m)
	}
}

// CA-717: sin "y despues: hoom piloto <slug>" en la firma no hay piloto sin
// lanzar: una firma sin auto, un Integrar con auto (solo aprobar encadena el
// piloto), el piloto de Seguir solo (ya se lanzo) y una firma con el piloto
// que no entra a lo ancho (enter no firma).
func TestV3C_CA717_SinPilotoEncadenadoNoHayPilotoSinLanzar(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-717"
	m := tv3cModelo(tv3cConfig(200, 40), ttNucTenancy())
	ap := tv3cHFirmar(t, m, "nuc-tenancy")
	tv3cHPilotos(t, "firma sin auto con hoom aprobar corriendo", m)
	ttSinOrdenes(t, ca, "el fin con 0 de aprobar sin auto", tv3cFin(m, ap.ID, 0, ""))
	tv3cHPilotos(t, "firma sin auto tras su fin", m)

	m = tv3cModelo(tv3cConfig(200, 40), tv3cSola("acep", "Acep", boardcmd.ColTuAceptacion))
	ttTeclas(m, "I", tablero.TeclaDer)
	in := ttUnaLanzar(t, ca, "enter sobre Integrar", ttTeclas(m, tablero.TeclaEnter))
	ttMismosArgs(t, ca, "integrar con auto", in.Args, []string{"integrar", "acep", "--si"})
	tv3cHPilotos(t, "Integrar con auto corriendo", m)

	m = tv3cModelo(tv3cConfig(200, 40), tv3cSola("sola", "Sola", boardcmd.ColWriter))
	tv3cLanzaSeguir(t, ca, m, "sola")
	tv3cHPilotos(t, "con el piloto de Seguir solo corriendo", m)

	s := tv3cHSlug(50)
	m = tv3cModelo(tv3cConfigColor(60, 40), tv3cHFirmaLarga(s, true))
	tv3cHAbreFirma(t, ca, "firma que no entra", m, "A")
	ttSinOrdenes(t, ca, "enter sobre la firma que no entra", ttTeclas(m, tablero.TeclaEnter))
	tv3cHPilotos(t, "tras enter sobre una firma que no entra", m)
}

// CA-717: varias firmas con el piloto corriendo a la vez salen en orden,
// cada una hasta el fin de su aprobar; la firma sin auto no sale. Las tres
// tarjetas estan en Tu aprobacion en el orden alfa, beta, gamma y se firman
// en ese orden, asi el orden del tablero y el de las firmas coinciden.
// MensajePilotos de esa lista es una linea por tarjeta, en el mismo orden.
func TestV3C_CA717_LosPilotosSinLanzarVanEnOrden(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-717"
	alfa := tv3cSola("alfa", "Alfa", boardcmd.ColTuAprobacion)
	beta := tv3cSinAuto(tv3cSola("beta", "Beta", boardcmd.ColTuAprobacion))
	gamma := tv3cSola("gamma", "Gamma", boardcmd.ColTuAprobacion)
	m := tv3cModelo(tv3cConfig(200, 40), alfa, beta, gamma)
	ids := map[string]int{}
	for _, slug := range []string{"alfa", "beta", "gamma"} {
		tv3cHElegir(t, m, slug)
		ids[slug] = tv3cHFirmar(t, m, slug).ID
	}
	tv3cHPilotos(t, "con las tres firmas corriendo", m, "alfa", "gamma")
	quiero := []string{tv3cHMsgPiloto("alfa"), tv3cHMsgPiloto("gamma")}
	if got := tablero.MensajePilotos(m.PilotosSinLanzar()); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", quiero) {
		t.Fatalf("%s: MensajePilotos de los pilotos sin lanzar es %q, es %q", ca, quiero, got)
	}

	p := ttUnaLanzar(t, ca, "el fin con 0 de la firma de alfa", tv3cFin(m, ids["alfa"], 0, ""))
	if p.Rol != "piloto" || p.Slug != "alfa" {
		t.Fatalf("%s: el fin con 0 de alfa lanza su piloto: %+v", ca, p)
	}
	tv3cHPilotos(t, "tras el fin de alfa", m, "gamma")
	ttSinOrdenes(t, ca, "el fin con 0 de la firma de beta, sin auto", tv3cFin(m, ids["beta"], 0, ""))
	tv3cHPilotos(t, "tras el fin de beta", m, "gamma")
	ttSinOrdenes(t, ca, "el fin con 1 de la firma de gamma", tv3cFin(m, ids["gamma"], 1, "no se pudo firmar"))
	tv3cHPilotos(t, "tras el fin de gamma", m)
}

// CA-717: MensajePilotos es una linea por tarjeta, en el orden que se le da:
// "el piloto de <slug> no se lanza solo: cuando termine la firma, hoom
// piloto <slug>"; sin tarjetas, ninguna linea.
func TestV3C_CA717_MensajePilotosEsUnaLineaPorTarjeta(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-717"
	for _, sin := range [][]string{nil, {}} {
		if got := tablero.MensajePilotos(sin); len(got) != 0 {
			t.Fatalf("%s: MensajePilotos(%q) no tiene lineas: %q", ca, sin, got)
		}
	}
	for _, c := range []struct {
		slugs  []string
		quiero []string
	}{
		{[]string{"nuc-tenancy"}, []string{"el piloto de nuc-tenancy no se lanza solo: cuando termine la firma, hoom piloto nuc-tenancy"}},
		{[]string{"zeta", "alfa"}, []string{
			"el piloto de zeta no se lanza solo: cuando termine la firma, hoom piloto zeta",
			"el piloto de alfa no se lanza solo: cuando termine la firma, hoom piloto alfa",
		}},
	} {
		if got := tablero.MensajePilotos(c.slugs); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", c.quiero) {
			t.Fatalf("%s: MensajePilotos(%q) es %q, es %q", ca, c.slugs, c.quiero, got)
		}
	}
}

// tv3cHFirmaEnRun corre hoom tablero en 120x40 sobre la tarjeta del writer
// (Writer) y pila (Tu aprobacion, con auto); en TE TOCA el Writer va antes que
// Tu aprobacion, asi que la seleccion empieza en la del writer. Lanza su
// writer (w, enter, enter: CA-528), llama a antes (si no es nil), baja a pila
// con j y la firma con el piloto (A, ->, enter). Exige las dos ordenes y
// devuelve la espera del fin de Run.
func tv3cHFirmaEnRun(t *testing.T, w *ttMundo, antes func()) func(t *testing.T) (int, any) {
	t.Helper()
	const ca = "CA-717"
	w.term.cambiar(120, 40)
	fin := w.correr(nil)
	ttEsperar(t, ca, "la tarjeta del writer elegida", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, ca, "el modal del writer", w, func() bool {
		return w.marcoCon("Se va a ejecutar:") && w.marcoCon("hoom pedir writer tarjeta -m")
	})
	w.term.mandar("\r\r")
	ttEsperar(t, ca, "el writer lanzado y su modal cerrado", w, func() bool {
		return len(w.lanz.lanzadas()) >= 1 && !w.marcoCon("Se va a ejecutar:")
	})
	if antes != nil {
		antes()
	}
	w.term.mandar("j")
	ttEsperar(t, ca, "pila elegida", w, func() bool { return w.filaElegida("pila") })
	w.term.mandar("A")
	ttEsperar(t, ca, "la firma con el piloto", w, func() bool {
		return w.marcoCon("hoom aprobar pila --si") && w.marcoCon("y despues: hoom piloto pila")
	})
	w.term.mandar("\x1b[C")
	ttEsperar(t, ca, "el boton de la accion elegido", w, func() bool { return w.marcoCon("[ Aprobar spec ]") })
	w.term.mandar("\r")
	ttEsperar(t, ca, "la orden de hoom aprobar y la firma cerrada", w, func() bool {
		return len(w.lanz.lanzadas()) >= 2 && !w.marcoCon("Se va a ejecutar:")
	})
	ls := w.lanz.lanzadas()
	ttMismosArgs(t, ca, "Run: el writer", ls[0].Args, []string{"pedir", "writer", "tarjeta", "-m", strings.TrimSpace(ttPedidoWriter)})
	ttMismosArgs(t, ca, "Run: la firma", ls[1].Args, []string{"aprobar", "pila", "--si"})
	if ls[0].Rol != "writer" || ls[1].Rol != "" {
		t.Fatalf("%s: el writer lleva su rol y hoom aprobar ninguno: %+v", ca, ls)
	}
	return fin
}

// tv3cHTableroRun es el tablero de tv3cHFirmaEnRun.
func tv3cHTableroRun() boardcmd.Board {
	return ttTablero(ttTarjetaCon(ttAccionWriter()), tv3cSola("pila", "Pila", boardcmd.ColTuAprobacion))
}

// CA-717 (decision de Henry, hallazgo 98ba68): si hoom tablero se cierra
// mientras corre el hoom aprobar de una firma con el piloto, con q, con
// ctrl-c o con el fin de archivo de la terminal (un error), el piloto no se
// lanza, y Run lo dice despues de devolver la terminal y despues de
// MensajeSalida: Out es "queda 1 trabajo corriendo: hoom status" (el writer;
// hoom aprobar no tiene rol y no cuenta) y despues "el piloto de pila no se
// lanza solo: cuando termine la firma, hoom piloto pila". q y ctrl-c salen
// con 0; el fin de archivo con 1 y "hoom: EOF" en Err (CA-533).
func TestV3C_CA717_RunAlCerrarDiceElPilotoSinLanzar(t *testing.T) {
	const ca = "CA-717"
	for _, c := range []struct {
		nombre string
		cerrar func(w *ttMundo)
		code   int
		err    string
	}{
		{"q", func(w *ttMundo) { w.term.mandar("q") }, 0, ""},
		{"ctrl-c", func(w *ttMundo) { w.term.mandar("\x03") }, 0, ""},
		{"fin de archivo", func(w *ttMundo) { w.term.cortar() }, 1, "hoom: EOF"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			w := ttNuevoMundo(t, tv3cHTableroRun())
			fin := tv3cHFirmaEnRun(t, w, nil)
			c.cerrar(w)
			code, pan := fin(t)
			if code != c.code || pan != nil {
				t.Fatalf("%s: [%s] Run sale con %d: %d (%v)", ca, c.nombre, c.code, code, pan)
			}
			if got := strings.TrimRight(w.r.texto("err"), "\n"); got != c.err {
				t.Fatalf("%s: [%s] Err es %q, es %q", ca, c.nombre, c.err, got)
			}
			quiero := "queda 1 trabajo corriendo: hoom status\n" + tv3cHMsgPiloto("pila")
			if got := strings.TrimRight(w.r.texto("out"), "\n"); got != quiero {
				t.Fatalf("%s: [%s] al salir Out dice MensajeSalida y despues el piloto sin lanzar:\n%q\nes:\n%q", ca, c.nombre, quiero, got)
			}
			rest := w.r.indice(0, func(e ttSuceso) bool { return e.que == "restaurar" })
			outI := w.r.indice(0, func(e ttSuceso) bool { return e.que == "out" })
			if rest < 0 || outI < rest {
				t.Fatalf("%s: [%s] Out se escribe despues de devolver la terminal: %v", ca, c.nombre, w.r.copia())
			}
			ls := w.lanz.lanzadas()
			if len(ls) != 2 {
				t.Fatalf("%s: [%s] cerrar el tablero no lanza el piloto: %+v", ca, c.nombre, ls)
			}
		})
	}
}

// CA-717: cuando el hoom aprobar de la firma termina antes de cerrar el
// tablero no queda piloto sin lanzar: con 0 el piloto se lanzo (Lanzar{Rol:
// piloto, Args: {piloto, pila}}), con otro codigo no hay piloto; en los dos
// casos Out no dice "el piloto de". Desde que el writer quedo corriendo, el
// Lanzador falso termina cada orden enseguida con ese codigo (el writer sigue
// vivo). Con 1, el fin de hoom aprobar se ve en la linea de mensajes (doc de
// Termino: aprobar pone su Ultima ahi) y Out es solo MensajeSalida del
// writer.
func TestV3C_CA717_RunSinPilotoPendienteNoLoDice(t *testing.T) {
	const ca = "CA-717"
	for _, code := range []int{0, 1} {
		t.Run(fmt.Sprintf("fin con %d", code), func(t *testing.T) {
			w := ttNuevoMundo(t, tv3cHTableroRun())
			fin := tv3cHFirmaEnRun(t, w, func() {
				w.lanz.mu.Lock()
				w.lanz.fin = &tablero.FinProceso{Code: code, Ultima: "termino el proceso"}
				w.lanz.mu.Unlock()
			})
			if code == 0 {
				ttEsperar(t, ca, "el piloto lanzado al terminar hoom aprobar con 0", w, func() bool { return len(w.lanz.lanzadas()) >= 3 })
				p := w.lanz.lanzadas()[2]
				if p.Rol != "piloto" || p.Slug != "pila" {
					t.Fatalf("%s: el fin con 0 de hoom aprobar lanza el piloto de pila: %+v", ca, p)
				}
				ttMismosArgs(t, ca, "Run: el piloto", p.Args, []string{"piloto", "pila"})
			} else {
				ttEsperar(t, ca, "el fin con 1 de hoom aprobar en el tablero", w, func() bool {
					return w.marcoCon("termino el proceso") && !w.marcoCon("Se va a ejecutar:")
				})
			}
			w.term.mandar("q")
			if got, _ := fin(t); got != 0 {
				t.Fatalf("%s: q sale con 0, salio con %d", ca, got)
			}
			if out := w.r.texto("out"); strings.Contains(out, "el piloto de") {
				t.Fatalf("%s: [fin con %d] sin piloto pendiente Out no dice el piloto: %q", ca, code, out)
			}
			if code != 0 {
				if ls := w.lanz.lanzadas(); len(ls) != 2 {
					t.Fatalf("%s: [fin con %d] sin 0 no se lanza el piloto: %+v", ca, code, ls)
				}
				if got := strings.TrimRight(w.r.texto("out"), "\n"); got != "queda 1 trabajo corriendo: hoom status" {
					t.Fatalf("%s: [fin con %d] Out es solo MensajeSalida del writer: %q", ca, code, got)
				}
			}
		})
	}
}
