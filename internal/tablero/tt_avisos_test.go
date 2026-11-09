// Lo que dice la barra y el aviso de fin (CA-530): ttBarraDice, el aviso
// "<rol> termino en <slug>: <plain>" en rojo o en verde (en rojo tambien sin
// sobre: ronda 2b), varios fines en una lectura, y que ningun aviso de fin se
// pierda detras de otro mensaje. Regresiones de la ronda 1 (review completa
// 20261108T231004_195bc7 de tablero-en-la-terminal) y de la ronda 2 (review
// delta: hallazgos 20261109T003926_08c87c y 20261109T004627_48bfb7),
// escritas desde el contrato de hoy (doc.go y go doc del paquete), sin leer
// la implementacion.
package tablero_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttBarraDice exige que la barra 1 diga msg: sin color, al final (" · " +
// msg); con color, en rojo si rojo.
func ttBarraDice(t *testing.T, ca, que string, m *tablero.Modelo, color, rojo bool, msg string) {
	t.Helper()
	b := ttBarra1(m)
	if !color {
		if !strings.HasSuffix(b, " · "+msg) {
			t.Fatalf("%s: %s: la barra dice %q: %q", ca, que, msg, b)
		}
		return
	}
	if !strings.Contains(b, msg) || (rojo && !ttConColor(ttRojo, b, msg)) {
		t.Fatalf("%s: %s: la barra dice %q (en rojo: %v): %q", ca, que, msg, rojo, b)
	}
}

// ---------------------------------------------------------------- CA-530: el aviso de fin

// ttCorre es una tarjeta en Writer con un running del sobre env (sin sobre si
// env es "") y del run run.
func ttCorre(slug, rol, env, run string) boardcmd.Card {
	c := ttCard(slug, "Tarjeta "+slug, boardcmd.ColWriter, "corre "+slug, ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
	c.Running = &boardcmd.Running{EnvelopeID: env, RunID: run, Role: rol, Provider: "claude", Stage: "run", Step: 2, Steps: 6, StartedAt: ttAhora}
	return c
}

// ttParada es la misma tarjeta sin running, con su plain de ahora.
func ttParada(slug string) boardcmd.Card {
	return ttCard(slug, "Tarjeta "+slug, boardcmd.ColWriter, "termino "+slug, ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
}

// CA-530 (hallazgo 5f48e8): una tarjeta vista con running del sobre E, leida
// sin running y con interrupted de E, avisa en rojo (el trabajo murio, no
// cerro entregable); lo mismo un run sin sobre (Running{RunID: R}) que queda
// interrupted de R. La que solo dejo de correr, sin rojo ni interrupted,
// avisa en verde; tambien la que muestra interrupted de otro sobre.
func TestCA530_FinInterrumpidoEsRojo(t *testing.T) {
	interr := func(slug, env, run, rol string) boardcmd.Card {
		c := ttParada(slug)
		c.Plain, c.NeedsDecision = "el trabajo se corto", true
		c.Interrupted = &boardcmd.Interrupted{EnvelopeID: env, RunID: run, Role: rol, Provider: "claude", Stage: "run", Step: 2, Steps: 6, UpdatedAt: ttAhora}
		return c
	}
	casos := []struct {
		nombre string
		antes  boardcmd.Card
		despue boardcmd.Card
		msg    string
		rojo   bool
	}{
		{"interrumpida del sobre", ttCorre("tarjeta", "writer", "env-e", "run-e"), interr("tarjeta", "env-e", "run-e", "writer"),
			"writer termino en tarjeta: el trabajo se corto", true},
		{"interrumpida del run sin sobre", ttCorre("tarjeta", "reviewer", "", "run-r"), interr("tarjeta", "", "run-r", "reviewer"),
			"reviewer termino en tarjeta: el trabajo se corto", true},
		{"solo paro", ttCorre("tarjeta", "writer", "env-e", "run-e"), ttParada("tarjeta"),
			"writer termino en tarjeta: termino tarjeta", false},
		{"interrumpida de otro sobre", ttCorre("tarjeta", "writer", "env-e", "run-e"), interr("tarjeta", "env-otro", "run-otro", "writer"),
			"writer termino en tarjeta: el trabajo se corto", false},
	}
	for _, cs := range casos {
		cfg := ttAncho()
		cfg.Color = true
		m := ttModelo(cfg, ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		b := ttBarra1(m)
		color := ttVerde
		if cs.rojo {
			color = ttRojo
		}
		if !strings.Contains(b, cs.msg) || !ttConColor(color, b, cs.msg) {
			t.Fatalf("CA-530: %s: la barra dice %q en %s: %q", cs.nombre, cs.msg, map[bool]string{true: "rojo", false: "verde"}[cs.rojo], b)
		}
		// sin color, el mismo texto al final de la barra
		m = ttModelo(ttAncho(), ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		ttBarraDice(t, "CA-530", cs.nombre, m, false, false, cs.msg)
	}
}

// CA-530 (hallazgo 8a3a66): dos tarjetas que dejan de correr en la misma
// lectura avisan una tras otra, en el orden del tablero, cada una durante
// DuracionFin; despues, ninguna.
func TestCA530_DosFinesEnUnaLectura(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttCorre("zeta", "test-writer", "env-z", "run-z"), ttCorre("alfa", "writer", "env-a", "run-a")))
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttParada("zeta"), ttParada("alfa"))})
	primero, segundo := "test-writer termino en zeta: termino zeta", "writer termino en alfa: termino alfa"
	ttBarraDice(t, "CA-530", "al leer", m, false, false, primero)
	m.Tic(t0.Add(tablero.DuracionFin - time.Second))
	ttBarraDice(t, "CA-530", "antes de DuracionFin", m, false, false, primero)
	m.Tic(t0.Add(tablero.DuracionFin + 500*time.Millisecond))
	ttBarraDice(t, "CA-530", "pasado el primero", m, false, false, segundo)
	m.Tic(t0.Add(2*tablero.DuracionFin - time.Second))
	ttBarraDice(t, "CA-530", "el segundo dura DuracionFin", m, false, false, segundo)
	m.Tic(t0.Add(2*tablero.DuracionFin + time.Second))
	if b := ttBarra1(m); strings.Contains(b, "termino en") {
		t.Fatalf("CA-530: pasados los dos, la barra no dice ningun fin: %q", b)
	}
}

// CA-530 (hallazgo 8a3a66): el aviso de fin le gana la barra enseguida a un
// mensaje de otra clase ("lanzando hoom ...", un why).
func TestCA530_ElAvisoDeFinTomaLaBarra(t *testing.T) {
	// "lanzando hoom pedir writer tarjeta"
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter()), ttCorre("dos", "reviewer", "env-dos", "run-dos")))
	ttIrA(t, m, "tarjeta")
	m.Tecla("w", ttAhora)
	ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttBarraDice(t, "CA-528", "lanzado", m, false, false, "lanzando hoom pedir writer tarjeta")
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Second), Tablero: ttTablero(ttTarjetaCon(ttAccionWriter()), ttParada("dos"))})
	ttBarraDice(t, "CA-530", "tras lanzar", m, false, false, "reviewer termino en dos: termino dos")
	// un why
	m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttCorre("dos", "reviewer", "env-dos", "run-dos")))
	ttIrA(t, m, "tarjeta")
	m.Tecla("s", ttAhora)
	ttBarraDice(t, "CA-525", "el why", m, false, false, ttWhyGuardar)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Second), Tablero: ttTablero(ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttParada("dos"))})
	ttBarraDice(t, "CA-530", "tras un why", m, false, false, "reviewer termino en dos: termino dos")
}

// ---------------------------------------------------------------- CA-530: ningun aviso de fin se pierde (ronda 2)

// ttFinVuelveEntero mueve el reloj con Tic, de a 250 ms desde desde, hasta que
// la barra dice el aviso de fin msg (a lo sumo hasta tope), y exige que lo
// diga sin cortes durante un DuracionFin entero desde que volvio y que pasado
// ese DuracionFin ya no lo diga. Sin color: el aviso al final de la barra.
func ttFinVuelveEntero(t *testing.T, ca, que string, m *tablero.Modelo, desde, tope time.Time, msg string) {
	t.Helper()
	const paso = 250 * time.Millisecond
	dice := func() bool { return strings.HasSuffix(ttBarra1(m), " · "+msg) }
	ahora := desde
	for !dice() {
		ahora = ahora.Add(paso)
		if ahora.After(tope) {
			t.Fatalf("%s: %s: el aviso de fin %q vuelve a la barra a lo sumo a los %v, la barra dice %q",
				ca, que, msg, tope.Sub(desde)+paso, ttBarra1(m))
		}
		m.Tic(ahora)
	}
	volvio := ahora
	for ahora.Before(volvio.Add(tablero.DuracionFin - 2*paso)) {
		ahora = ahora.Add(paso)
		m.Tic(ahora)
		if !dice() {
			t.Fatalf("%s: %s: el aviso de fin vuelve entero, DuracionFin desde que vuelve: volvio y %v despues la barra ya no lo dice: %q",
				ca, que, ahora.Sub(volvio), ttBarra1(m))
		}
	}
	m.Tic(volvio.Add(tablero.DuracionFin + 2*paso))
	if strings.Contains(ttBarra1(m), msg) {
		t.Fatalf("%s: %s: pasado DuracionFin desde que volvio, la barra ya no dice el aviso de fin: %q", ca, que, ttBarra1(m))
	}
}

// CA-530 (hallazgo 20261109T003926_08c87c): ningun aviso de fin se pierde. En
// una lectura donde una tarjeta deja de correr y la tarjeta abierta ya no
// esta en el tablero, la vista vuelve a la lista y la barra dice primero "la
// tarjeta <slug> ya no existe" (durante su DuracionAviso) y despues el aviso
// "<rol> termino en <slug>: <plain>", que dura entonces un DuracionFin entero.
func TestCA530_ElFinVuelveDespuesDeYaNoExiste(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttBacklog("abierta", "La tarjeta abierta"), ttCorre("corre", "writer", "env-c", "run-c")))
	ttIrA(t, m, "abierta")
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if s, _ := m.Abierta(); s != "abierta" {
		t.Fatalf("CA-520: enter abre la tarjeta abierta, Abierta = %q", s)
	}
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttParada("corre"))})
	if m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-535: la tarjeta abierta ya no existe: la vista vuelve a la lista, es %q", m.Vista())
	}
	const ya = "la tarjeta abierta ya no existe"
	ttBarraDice(t, "CA-530", "al leer, primero lo de esta lectura", m, false, false, ya)
	m.Tic(t0.Add(tablero.DuracionAviso - time.Second))
	ttBarraDice(t, "CA-530", "ya no existe dura su DuracionAviso", m, false, false, ya)
	ttFinVuelveEntero(t, "CA-530", "despues de ya no existe", m, t0.Add(tablero.DuracionAviso-time.Second),
		t0.Add(tablero.DuracionAviso+time.Second), "writer termino en corre: termino corre")
}

// CA-530 (hallazgo 20261109T003926_08c87c): lo mismo con una confirmacion
// que esa lectura cierra (cambio el texto de la firma): la barra dice primero
// "cambio lo que ibas a confirmar en <slug>: vuelve a abrirlo" (durante
// DuracionAviso, como un why) y despues el aviso de fin, entero. Nada se
// firma.
func TestCA530_ElFinVuelveDespuesDeLaConfirmacionCerrada(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttNucTenancy(), ttCorre("corre", "reviewer", "env-c", "run-c")))
	ttIrA(t, m, "nuc-tenancy")
	ttSinOrdenes(t, "CA-529", "A", m.Tecla("A", ttAhora))
	if m.Vista() != tablero.VistaConfirmacion {
		t.Fatalf("CA-529: A abre la confirmacion, la vista es %q", m.Vista())
	}
	m.Tecla(tablero.TeclaDer, ttAhora)
	otra := ttNucTenancy()
	otra.Item.Titulo = "Tenancy del nucleo, otra"
	t0 := ttAhora.Add(2 * time.Second)
	if l := ttLanzares(m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(otra, ttParada("corre"))})); len(l) != 0 {
		t.Fatalf("CA-529: la lectura que cambia la firma no firma: %#v", l)
	}
	if m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-529: la confirmacion se cierra y se vuelve a la lista, la vista es %q", m.Vista())
	}
	cambio := "cambio lo que ibas a confirmar en nuc-tenancy: vuelve a abrirlo"
	ttBarraDice(t, "CA-530", "al leer, primero la confirmacion cerrada", m, false, false, cambio)
	m.Tic(t0.Add(tablero.DuracionAviso - time.Second))
	ttBarraDice(t, "CA-530", "el mensaje de la confirmacion dura su DuracionAviso", m, false, false, cambio)
	ttFinVuelveEntero(t, "CA-530", "despues de la confirmacion cerrada", m, t0.Add(tablero.DuracionAviso-time.Second),
		t0.Add(tablero.DuracionAviso+time.Second), "reviewer termino en corre: termino corre")
	if l := ttLanzares(m.Tecla(tablero.TeclaEnter, t0.Add(30*time.Second))); len(l) != 0 {
		t.Fatalf("CA-529: nada se firma: %#v", l)
	}
}

// CA-530 (hallazgo 20261109T003926_08c87c): un mensaje que una tecla da
// mientras se ve el aviso de fin (a sobre una tarjeta sin la accion: "en
// <columna> no hay <hace>") toma la barra su DuracionAviso y despues el
// aviso de fin vuelve entero: un DuracionFin desde que vuelve, no lo que le
// quedaba.
func TestCA530_ElFinVuelveDespuesDelMensajeDeUnaTecla(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttCorre("corre", "writer", "env-c", "run-c")))
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttParada("corre"))})
	const fin = "writer termino en corre: termino corre"
	ttBarraDice(t, "CA-530", "al leer", m, false, false, fin)
	t1 := t0.Add(2 * time.Second)
	m.Tic(t1)
	ttBarraDice(t, "CA-530", "a los 2s", m, false, false, fin)
	if got := ttSeleccionada(m); got != "corre" {
		t.Fatalf("CA-520: la seleccion sigue en su tarjeta (corre), esta en %q\n%s", got, ttPantalla(m))
	}
	msg := "en " + ttNombreColumna(boardcmd.ColWriter) + " no hay " + ttHaceDe(t, "a")
	ttSinOrdenes(t, "CA-525", "a sin la accion", m.Tecla("a", t1))
	ttBarraDice(t, "CA-530", "la tecla toma la barra", m, false, false, msg)
	m.Tic(t1.Add(tablero.DuracionAviso - time.Second))
	ttBarraDice(t, "CA-530", "el mensaje de la tecla dura su DuracionAviso", m, false, false, msg)
	ttFinVuelveEntero(t, "CA-530", "despues del mensaje de la tecla", m, t1.Add(tablero.DuracionAviso-time.Second),
		t1.Add(tablero.DuracionAviso+time.Second), fin)
}

// CA-530 (hallazgo 20261109T004627_48bfb7; ronda 2b): el spec dice que el
// aviso de fin va "en verde si el sobre cerró `entregable` y en rojo si no"
// (spec tablero-en-la-terminal, lineas 222-225). Un trabajo sin sobre (un run
// de hoom run a mano: Running{RunID: R}, sin EnvelopeID) que deja de correr
// no cerro ningun sobre entregable: el aviso "<rol> termino en <slug>:
// <plain>" va en rojo (con Color, el rojo antes del texto y ningun verde).
// Tambien con el rojo de un veredicto o con un interrupted de otro run. Guarda:
// con sobre, sin rojo ni interrupted, el mismo caso es verde.
func TestCA530_ElFinDeUnRunSinSobreEsRojo(t *testing.T) {
	conRojo := func() boardcmd.Card {
		c := ttParada("tarjeta")
		c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v-9", Reason: "test: FAIL", Plain: "la verificacion dio rojo"}
		return c
	}
	conOtro := func() boardcmd.Card {
		c := ttParada("tarjeta")
		c.Interrupted = &boardcmd.Interrupted{RunID: "run-otro", Role: "writer", Provider: "claude", Stage: "run", Step: 2, Steps: 6, UpdatedAt: ttAhora}
		return c
	}
	casos := []struct {
		nombre string
		antes  boardcmd.Card
		despue boardcmd.Card
		verde  bool
	}{
		{"run sin sobre que paro", ttCorre("tarjeta", "writer", "", "run-r"), ttParada("tarjeta"), false},
		{"run sin sobre con el rojo de un veredicto", ttCorre("tarjeta", "writer", "", "run-r"), conRojo(), false},
		{"run sin sobre con un interrupted de otro run", ttCorre("tarjeta", "writer", "", "run-r"), conOtro(), false},
		{"con sobre (guarda: verde)", ttCorre("tarjeta", "writer", "env-e", "run-e"), ttParada("tarjeta"), true},
	}
	const msg = "writer termino en tarjeta: termino tarjeta"
	for _, cs := range casos {
		cfg := ttAncho()
		cfg.Color = true
		m := ttModelo(cfg, ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		b := ttBarra1(m)
		if !strings.Contains(b, msg) {
			t.Fatalf("CA-530: %s: la barra dice %q: %q", cs.nombre, msg, b)
		}
		verde, rojo := ttConColor(ttVerde, b, msg), ttConColor(ttRojo, b, msg)
		if cs.verde {
			if !verde || rojo {
				t.Fatalf("CA-530: %s: con sobre que solo paro el aviso es verde: %q", cs.nombre, b)
			}
			continue
		}
		if !rojo || verde {
			t.Fatalf("CA-530: %s: sin sobre nada cerro entregable, el aviso va en rojo (rojo %v, verde %v): %q", cs.nombre, rojo, verde, b)
		}
		// sin color, el mismo texto al final de la barra
		m = ttModelo(ttAncho(), ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		ttBarraDice(t, "CA-530", cs.nombre, m, false, false, msg)
	}
}
