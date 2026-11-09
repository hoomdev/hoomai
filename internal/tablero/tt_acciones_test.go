// Las acciones (CA-517, CA-525, CA-526, CA-527, CA-528, CA-529, CA-530,
// CA-531): cada tecla de accion sobre la tarjeta, el menu, el formulario, las
// firmas, lanzar y el fin de lo lanzado, y lo que dice la barra.
package tablero_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

var (
	ttRojo  = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*(?:31|91)(?:;[0-9]+)*m`)
	ttVerde = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*(?:32|92)(?:;[0-9]+)*m`)
)

// ttConColor dice si en la linea hay una secuencia del color re antes de
// texto.
func ttConColor(re *regexp.Regexp, linea, texto string) bool {
	i := strings.Index(linea, texto)
	if i < 0 {
		return false
	}
	loc := re.FindStringIndex(linea)
	return loc != nil && loc[0] < i
}

// ttAncho es una pantalla donde entran los mensajes y la linea -> enteros.
func ttAncho() tablero.Config { return ttConfig(200, 40, false) }

// ttTarjetaCon es una tarjeta en Writer con solo esas acciones.
func ttTarjetaCon(acciones ...boardcmd.Action) boardcmd.Card {
	c := ttCard("tarjeta", "Tarjeta de prueba", boardcmd.ColWriter, "el plain de la tarjeta",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"), ttSeg("verde", "verde", boardcmd.SegFalta, "sin verde"))
	c.Actions = append([]boardcmd.Action{}, acciones...)
	return c
}

// ttHaceDe es el Hace de la fila de Atajos que tiene la tecla k con una
// Accion (la tabla es la del despacho).
func ttHaceDe(t *testing.T, k tablero.Tecla) string {
	t.Helper()
	for _, a := range tablero.Atajos {
		for _, x := range a.Teclas {
			if x == k && a.Accion != "" {
				return a.Hace
			}
		}
	}
	t.Fatalf("la tecla %q no esta en ninguna fila de Atajos con Accion", k)
	return ""
}

// ttAccionDeTecla es la accion habilitada que la tecla abre.
func ttAccionDeTecla(k tablero.Tecla) (boardcmd.Action, tablero.Vista) {
	switch k {
	case "a":
		return ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec"), tablero.VistaFormulario
	case "t":
		return ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests"), tablero.VistaFormulario
	case "w":
		return ttAccionWriter(), tablero.VistaFormulario
	case "r":
		return ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""), tablero.VistaFormulario
	case "R":
		return ttPagosCaidos().Actions[0], tablero.VistaFormulario
	case "s":
		return ttAccionGuardar(true, "", "internal/a.go", ".hoom/specs/tarjeta.md"), tablero.VistaConfirmacion
	case "A":
		return ttAccionAprobar(), tablero.VistaConfirmacion
	case "I":
		return ttAccionIntegrar(), tablero.VistaConfirmacion
	}
	panic("tecla sin accion: " + string(k))
}

var ttTeclasDeAccion = []tablero.Tecla{"a", "t", "w", "r", "R", "s", "A", "I"}

// ttRelanzar es una accion sin verbo de la capa 1.
func ttRelanzar() boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActRelanzar, Role: "writer", Label: "Volver a lanzar", Enabled: true, Paths: []string{}}
}

// ttEnVista lleva el modelo de la lista a la vista desde donde se aprieta la
// tecla: la lista, una pestana de la tarjeta abierta o el menu.
func ttEnVista(t *testing.T, m *tablero.Modelo, v tablero.Vista) {
	t.Helper()
	switch v {
	case tablero.VistaLista:
	case tablero.VistaMenu:
		m.Tecla(tablero.TeclaEspacio, ttAhora)
	default:
		m.Tecla(tablero.TeclaEnter, ttAhora)
		for i, p := range tablero.Pestanas {
			if p == v {
				m.Tecla(tablero.Tecla(string(rune('1'+i))), ttAhora)
			}
		}
	}
	if m.Vista() != v {
		t.Fatalf("no pude llegar a la vista %q (estoy en %q)", v, m.Vista())
	}
}

// CA-525: cada tecla de accion, con la accion habilitada, abre su formulario
// o su confirmacion (desde la lista, una pestana de la tarjeta y el menu);
// sin la accion, la barra dice "en <Columna> no hay <hace>"; deshabilitada,
// su why. En ningun caso se ejecuta nada.
func TestCA525_TeclasDeAccion(t *testing.T) {
	for _, desde := range []tablero.Vista{tablero.VistaLista, tablero.VistaSpec, tablero.VistaMenu} {
		for _, k := range ttTeclasDeAccion {
			a, vista := ttAccionDeTecla(k)
			// habilitada
			m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(a)))
			ttEnVista(t, m, desde)
			ttSinOrdenes(t, "CA-525", string(k)+" (habilitada)", m.Tecla(k, ttAhora))
			if m.Vista() != vista {
				t.Fatalf("CA-525: desde %q, %q con %s habilitada abre %q, la vista es %q", desde, k, a.ID, vista, m.Vista())
			}
			// sin la accion (con otra, sin verbo, para que el menu tenga algo)
			m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttRelanzar())))
			ttEnVista(t, m, desde)
			ttSinOrdenes(t, "CA-525", string(k)+" (sin la accion)", m.Tecla(k, ttAhora))
			quiere := " · en Writer no hay " + ttHaceDe(t, k)
			if b := ttBarra1(m); !strings.HasSuffix(b, quiere) || m.Vista() != desde {
				t.Fatalf("CA-525: desde %q, %q sin %s: la barra termina en %q y la vista sigue: %q (%q)", desde, k, a.ID, quiere, b, m.Vista())
			}
			// deshabilitada
			a.Enabled, a.Why = false, "un motivo de la tarjeta para "+a.ID
			m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(a)))
			ttEnVista(t, m, desde)
			ttSinOrdenes(t, "CA-525", string(k)+" (deshabilitada)", m.Tecla(k, ttAhora))
			if b := ttBarra1(m); !strings.HasSuffix(b, " · "+a.Why) || m.Vista() != desde {
				t.Fatalf("CA-525: desde %q, %q con %s deshabilitada: la barra dice su why y la vista sigue: %q (%q)", desde, k, a.ID, b, m.Vista())
			}
			// el aviso dura DuracionAviso
			m.Tic(ttAhora.Add(tablero.DuracionAviso + time.Second))
			if b := ttBarra1(m); strings.Contains(b, a.Why) {
				t.Fatalf("CA-525: el why se va pasado DuracionAviso: %q", b)
			}
		}
	}
}

// CA-525, caso limite: t con el spec sin guardar. La barra dice el why de la
// capa 1 y no se abre el formulario.
func TestCA525_TConElSpecSinGuardar(t *testing.T) {
	a := ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests")
	a.Enabled, a.Why = false, boardcmd.WhySpecSinGuardar
	c := ttTarjetaCon(a)
	c.Column, c.ColumnName = boardcmd.ColTestWriter, "Test-writer"
	m := ttModelo(ttAncho(), ttTablero(c))
	ttSinOrdenes(t, "CA-525", "t", m.Tecla("t", ttAhora))
	if m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-525: t con el spec sin guardar no abre el formulario, la vista es %q", m.Vista())
	}
	if b := ttBarra1(m); !strings.HasSuffix(b, " · "+boardcmd.WhySpecSinGuardar) {
		t.Fatalf("CA-525: la barra dice el why de la capa 1: %q", b)
	}
}

// CA-525: sin tarjeta elegida (la seleccion en la linea de Hecho) las teclas
// de accion no hacen nada.
func TestCA525_SinTarjetaElegida(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter(), ttAccionAprobar())))
	m.Tecla("G", ttAhora)
	antes := ttPantalla(m)
	for _, k := range append([]tablero.Tecla{"n", "u"}, ttTeclasDeAccion...) {
		ttSinOrdenes(t, "CA-525", string(k)+" sobre la linea de Hecho", m.Tecla(k, ttAhora))
		if ttPantalla(m) != antes || m.Vista() != tablero.VistaLista {
			t.Fatalf("CA-525: %q sin tarjeta elegida no hace nada:\n%s", k, ttPantalla(m))
		}
	}
}

// CA-525: n hace lo de hoom siguiente: cada caso con su mensaje, o el
// formulario de la primera accion (que ejecuta hoom siguiente).
func TestCA525_NSiguiente(t *testing.T) {
	writer := ttAccionWriter()
	writerOff := ttAccionWriter()
	writerOff.Enabled, writerOff.Why = false, "se agoto el presupuesto de la tarjeta"
	conRunning := ttTarjetaCon(writer)
	conRunning.Running = &boardcmd.Running{EnvelopeID: "e", Role: "writer", Stage: "run", StartedAt: ttAhora}
	conInterr := ttTarjetaCon(writer)
	conInterr.Interrupted, conInterr.NeedsDecision = &boardcmd.Interrupted{EnvelopeID: "e", Role: "writer", Stage: "run", Step: 1, Steps: 5}, true
	enHecho := ttViejo(1)
	enHecho.Actions = []boardcmd.Action{writer}
	enAprob := ttNucTenancy()
	enAcept := ttCard("acepta", "Acepta", boardcmd.ColTuAceptacion, "lista", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	enAcept.Actions = []boardcmd.Action{ttAccionIntegrar()}
	casos := []struct {
		nombre string
		card   boardcmd.Card
		quiere string
	}{
		{"en Hecho", enHecho, "la tarjeta ya esta en Hecho"},
		{"con running", conRunning, "el writer esta trabajando: espera a que termine"},
		{"interrumpida", conInterr, "el trabajo se corto: R reanuda o pide de nuevo"},
		{"en Tu aprobacion", enAprob, "te toca: A aprobar"},
		{"en Tu aceptacion", enAcept, "te toca: I integrar"},
		{"sin acciones", ttTarjetaCon(), "en Writer no hay nada que hacer ahora: el plain de la tarjeta"},
		{"primera de rol deshabilitada", ttTarjetaCon(writerOff), "se agoto el presupuesto de la tarjeta"},
		{"primera guardar", ttTarjetaCon(ttAccionGuardar(true, "", "a.go"), writer), "te toca: s guardar"},
	}
	for _, c := range casos {
		m := ttModelo(ttAncho(), ttTablero(c.card))
		if c.nombre == "en Hecho" {
			m.Tecla("G", ttAhora)
			m.Tecla(tablero.TeclaEnter, ttAhora) // abre Hecho
			m.Tecla("j", ttAhora)
		}
		ttSinOrdenes(t, "CA-525", "n ("+c.nombre+")", m.Tecla("n", ttAhora))
		if b := ttBarra1(m); !strings.HasSuffix(b, " · "+c.quiere) || m.Vista() != tablero.VistaLista {
			t.Fatalf("CA-525: n %s: la barra dice %q (vista %q): %q\n%s", c.nombre, c.quiere, m.Vista(), b, ttPantalla(m))
		}
	}
	// la primera accion es de rol y esta habilitada: su formulario, que
	// ejecuta hoom siguiente
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(writer)))
	ttSinOrdenes(t, "CA-525", "n", m.Tecla("n", ttAhora))
	if m.Vista() != tablero.VistaFormulario {
		t.Fatalf("CA-525: n con la primera accion de rol habilitada abre su formulario, la vista es %q", m.Vista())
	}
	if got, quiere := ttFlecha(t, "CA-527", m), "hoom siguiente tarjeta -m "+tarjetacmd.CitarPedido(ttPedidoWriter, false); got != quiere {
		t.Fatalf("CA-527: la linea -> de n es el verbo corto de hoom siguiente:\n got %q\nwant %q", got, quiere)
	}
	l := ttUnaLanzar(t, "CA-528", "enter en el formulario de n", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "n", l.Args, []string{"siguiente", "tarjeta", "-m", strings.TrimSpace(ttPedidoWriter)})
	if l.Rol != "writer" || l.Slug != "tarjeta" || l.ID != 1 {
		t.Fatalf("CA-528: el Lanzar de n lleva el rol de la accion, la tarjeta y el ID 1: %#v", l)
	}
	if b := ttBarra1(m); !strings.HasSuffix(b, " · lanzando hoom siguiente tarjeta") || m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-528: tras lanzar se vuelve a la lista con 'lanzando hoom siguiente tarjeta': %q (%q)", b, m.Vista())
	}
}

// CA-531: u ejecuta hoom usar <slug> (cualquier tarjeta, sin accion), desde
// la lista y desde la tarjeta abierta; los ID de las ordenes van de 1 en 1.
func TestCA531_UFijaLaTarjeta(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon()))
	l := ttUnaLanzar(t, "CA-531", "u", m.Tecla("u", ttAhora))
	ttMismosArgs(t, "CA-531", "u", l.Args, []string{"usar", "tarjeta"})
	if l.ID != 1 || l.Slug != "tarjeta" || l.Rol != "" {
		t.Fatalf("CA-531: el Lanzar de u es el ID 1, de la tarjeta y sin rol: %#v", l)
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	l = ttUnaLanzar(t, "CA-531", "u en la tarjeta abierta", m.Tecla("u", ttAhora))
	ttMismosArgs(t, "CA-531", "u", l.Args, []string{"usar", "tarjeta"})
	if l.ID != 2 {
		t.Fatalf("CA-528: cada orden tiene su numero, de 1 en 1: el segundo Lanzar es el 2, es %d", l.ID)
	}
	if n := m.Corriendo(); n != 0 {
		t.Fatalf("CA-528: usar no es un trabajo de rol: Corriendo = %d", n)
	}
	// el fin de usar pone su ultima linea en la barra
	m.Termino(tablero.FinProceso{ID: 2, Code: 0, Ultima: "tarjeta fijada para esta terminal: tarjeta"}, ttAhora)
	if b := ttBarra1(m); !strings.HasSuffix(b, " · tarjeta fijada para esta terminal: tarjeta") {
		t.Fatalf("CA-531: el fin de usar dice su ultima linea en la barra: %q", b)
	}
}

// CA-531: la barra 1 dice la tarjeta fijada con hoom usar, o "sin tarjeta
// fijada".
func TestCA531_BarraConLaFijada(t *testing.T) {
	m := tablero.Nuevo(ttAncho())
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttTarjetaCon()), Fijada: "x"})
	if b := ttBarra1(m); b != " NORMAL · tarjeta: x (hoom usar)" {
		t.Fatalf("CA-531: con una tarjeta fijada la barra 1 es ' NORMAL · tarjeta: x (hoom usar)': %q", b)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttTarjetaCon())})
	if b := ttBarra1(m); b != " NORMAL · sin tarjeta fijada" {
		t.Fatalf("CA-531: sin tarjeta fijada: %q", b)
	}
}

// CA-526: el menu: las acciones en su orden; j/k (y las flechas) mueven la
// marca solo sobre las habilitadas con verbo; enter o la tecla de la accion
// la ejecuta; esc cierra; las expertas solo en modo experto.
func TestCA526_MenuMoverElegirExperto(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTablero(ttInfraAPI(false)))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	marcada := func() string {
		for _, l := range ttCuerpo(m) {
			if strings.HasPrefix(l, " > ") {
				return l
			}
		}
		return ""
	}
	if got := marcada(); !strings.HasPrefix(got, " > w Pedir al writer") {
		t.Fatalf("CA-526: la marca empieza en la primera accion habilitada con verbo: %q\n%s", got, ttPantalla(m))
	}
	m.Tecla("j", ttAhora)
	if got := marcada(); !strings.HasPrefix(got, " > A Aprobar spec") {
		t.Fatalf("CA-526: j salta la deshabilitada y la que no tiene verbo: %q\n%s", got, ttPantalla(m))
	}
	m.Tecla("j", ttAhora)
	if got := marcada(); !strings.HasPrefix(got, " > A Aprobar spec") {
		t.Fatalf("CA-526: j en la ultima elegible no se mueve: %q", got)
	}
	m.Tecla(tablero.TeclaArriba, ttAhora)
	if got := marcada(); !strings.HasPrefix(got, " > w Pedir al writer") {
		t.Fatalf("CA-526: ↑ vuelve a la anterior elegible: %q", got)
	}
	m.Tecla(tablero.TeclaAbajo, ttAhora)
	ttSinOrdenes(t, "CA-526", "enter sobre Aprobar spec", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Vista() != tablero.VistaConfirmacion {
		t.Fatalf("CA-526: enter ejecuta la accion elegida (aprobar abre su confirmacion), la vista es %q", m.Vista())
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	// la tecla de la accion desde el menu
	m = ttModelo(ttConfig(80, 24, false), ttTablero(ttInfraAPI(false)))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	ttSinOrdenes(t, "CA-526", "s (deshabilitada) en el menu", m.Tecla("s", ttAhora))
	if b := ttBarra1(m); !strings.HasSuffix(b, " · "+ttWhyGuardar) {
		t.Fatalf("CA-526: la tecla de una accion deshabilitada dice su why: %q", b)
	}
	m.Tecla("w", ttAhora)
	if m.Vista() != tablero.VistaFormulario {
		t.Fatalf("CA-526: la tecla de la accion la ejecuta desde el menu (w abre su formulario), la vista es %q", m.Vista())
	}
	// esc cierra
	m = ttModelo(ttConfig(80, 24, false), ttTablero(ttInfraAPI(false)))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	ttSinOrdenes(t, "CA-526", "esc", m.Tecla(tablero.TeclaEsc, ttAhora))
	if m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-526: esc cierra el menu, la vista es %q", m.Vista())
	}
	if _, n := ttLineaCon(m, "Descartar cambios"); n != 0 {
		t.Fatalf("CA-526: la experta no aparece en modo normal")
	}
	// modo experto: la experta aparece, gris y sin verbo
	m.Tecla("x", ttAhora)
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	if l, _ := ttLineaCon(m, "Descartar cambios"); l != "     Descartar cambios  sin verbo en la terminal" {
		t.Fatalf("CA-526: en modo experto la experta aparece (sin verbo en la terminal): %q\n%s", l, ttPantalla(m))
	}
}

// CA-526: un menu sin nada elegible: enter no hace nada.
func TestCA526_MenuSinNadaElegible(t *testing.T) {
	c := ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar), boardcmd.Action{ID: boardcmd.ActRelanzar, Role: "writer",
		Label: "Volver a lanzar", Enabled: true, Paths: []string{}})
	m := ttModelo(ttConfig(80, 24, false), ttTablero(c))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	antes := ttPantalla(m)
	for _, k := range []tablero.Tecla{"j", "k", tablero.TeclaEnter} {
		ttSinOrdenes(t, "CA-526", string(k)+" sin nada elegible", m.Tecla(k, ttAhora))
	}
	if ttPantalla(m) != antes || m.Vista() != tablero.VistaMenu {
		t.Fatalf("CA-526: sin nada elegible, j, k y enter no hacen nada:\n%s", ttPantalla(m))
	}
	for _, l := range ttCuerpo(m) {
		if strings.HasPrefix(l, " > ") {
			t.Fatalf("CA-526: ninguna linea lleva la marca: %q", l)
		}
	}
}

// CA-517: lo que muestran el menu y la barra (la etiqueta, el why) es lo de
// la tarjeta, aunque la tarjeta diga otra cosa que boardcmd.
func TestCA517_LoQueMuestraEsDeLaTarjeta(t *testing.T) {
	a := ttAccionWriter()
	a.Label = "Etiqueta inventada por la tarjeta"
	g := ttAccionGuardar(false, "un why inventado por la tarjeta")
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(a, g)))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	if _, n := ttLineaCon(m, " w Etiqueta inventada por la tarjeta  hoom pedir writer tarjeta"); n == 0 {
		t.Fatalf("CA-517: el menu muestra la etiqueta de la tarjeta:\n%s", ttPantalla(m))
	}
	if _, n := ttLineaCon(m, "  (un why inventado por la tarjeta)"); n == 0 {
		t.Fatalf("CA-517: el menu muestra el why de la tarjeta:\n%s", ttPantalla(m))
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("w", ttAhora)
	if _, n := ttLineaCon(m, "Etiqueta inventada por la tarjeta · tarjeta"); n == 0 {
		t.Fatalf("CA-517: la caja del formulario lleva la etiqueta de la accion:\n%s", ttPantalla(m))
	}
}

// ---------------------------------------------------------------- el formulario

// ttFormulario abre el formulario de la tecla k sobre una tarjeta con a.
func ttFormulario(t *testing.T, cfg tablero.Config, k tablero.Tecla, a boardcmd.Action) *tablero.Modelo {
	t.Helper()
	m := ttModelo(cfg, ttTablero(ttTarjetaCon(a)))
	ttSinOrdenes(t, "CA-527", string(k), m.Tecla(k, ttAhora))
	if m.Vista() != tablero.VistaFormulario {
		t.Fatalf("CA-527: %q abre el formulario, la vista es %q", k, m.Vista())
	}
	return m
}

func ttCampo(m *tablero.Modelo, etiqueta string) string {
	for _, l := range ttCaja(m) {
		if strings.HasPrefix(l[min(2, len(l)):], etiqueta) {
			return l
		}
	}
	return ""
}

// CA-527: el formulario trae los valores de la accion; enter sin cambios
// lanza el verbo con -m y el pedido recortado, sin flags; vuelve a la lista
// con "lanzando hoom pedir writer <slug>".
func TestCA527_CA528_LanzarSinCambios(t *testing.T) {
	m := ttFormulario(t, ttAncho(), "w", ttAccionWriter())
	if got := ttCampo(m, "Provider"); got != "  Provider       < claude >" {
		t.Fatalf("CA-527: Provider con la opcion por defecto: %q", got)
	}
	for et, quiere := range map[string]string{"Modelo": "  Modelo         opus", "Esfuerzo": "  Esfuerzo       high",
		"Presupuesto": "  Presupuesto    2.75 USD", "Tiempo limite": "  Tiempo limite  45m"} {
		if got := ttCampo(m, et); got != quiere {
			t.Fatalf("CA-527: el campo %s es %q, se esperaba %q", et, got, quiere)
		}
	}
	if got := ttCampo(m, "Pedido"); got != "> Pedido" {
		t.Fatalf("CA-527: el foco empieza en Pedido: %q", got)
	}
	if got := ttFlecha(t, "CA-527", m); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido(ttPedidoWriter, false) {
		t.Fatalf("CA-527: la linea -> sin cambios no lleva flags: %q", got)
	}
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "w sin cambios", l.Args, []string{"pedir", "writer", "tarjeta", "-m", strings.TrimSpace(ttPedidoWriter)})
	if l.Rol != "writer" || l.Slug != "tarjeta" || l.ID != 1 {
		t.Fatalf("CA-528: el Lanzar lleva Rol, Slug e ID: %#v", l)
	}
	if b := ttBarra1(m); !strings.HasSuffix(b, " · lanzando hoom pedir writer tarjeta") || m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-528: se vuelve a la lista con 'lanzando hoom pedir writer tarjeta': %q (%q)", b, m.Vista())
	}
	if n := m.Corriendo(); n != 1 {
		t.Fatalf("CA-528: un trabajo de rol lanzado corre hasta su Termino: Corriendo = %d", n)
	}
}

// CA-527, CA-528: la linea -> y los Args llevan solo los flags que cambiaron,
// en el orden del contrato, y se actualizan con cada cambio.
func TestCA527_CA528_SoloLosFlagsCambiados(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola mundo"
	a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), ttProvOpencode(), ttProvCodex(false)}
	m := ttFormulario(t, ttAncho(), "w", a)
	base := "hoom pedir writer tarjeta -m " + tarjetacmd.CitarPedido("hola mundo", false)
	flecha := func(quiere string) {
		t.Helper()
		if got := ttFlecha(t, "CA-527", m); got != quiere {
			t.Fatalf("CA-527: la linea -> es\n %q\nse esperaba\n %q", got, quiere)
		}
	}
	flecha(base)
	// el que no puede, en su linea con su why
	if l, n := ttLineaCon(m, "opencode: "+ttWhyOpencode); n == 0 || strings.Contains(l, "<") {
		t.Fatalf("CA-527: el provider que no puede va con su why: %q\n%s", l, ttPantalla(m))
	}
	// tab desde Pedido da la vuelta a Provider; → salta al que no puede
	ttSinOrdenes(t, "CA-527", "tab", m.Tecla(tablero.TeclaTab, ttAhora))
	if got := ttCampo(m, "Provider"); got != "> Provider       < claude >" {
		t.Fatalf("CA-527: tab desde Pedido pasa a Provider (en ciclo): %q", got)
	}
	m.Tecla(tablero.TeclaDer, ttAhora)
	if got := ttCampo(m, "Provider"); got != "> Provider       < codex >" {
		t.Fatalf("CA-527: → elige la siguiente opcion que puede (salta opencode): %q", got)
	}
	flecha(base + " --provider codex")
	m.Tecla(tablero.TeclaDer, ttAhora)
	if got := ttCampo(m, "Provider"); got != "> Provider       < claude >" {
		t.Fatalf("CA-527: → da la vuelta: %q", got)
	}
	flecha(base)
	m.Tecla(tablero.TeclaIzq, ttAhora)
	flecha(base + " --provider codex")
	// Modelo
	m.Tecla(tablero.TeclaTab, ttAhora)
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "sonnet")
	flecha(base + " --provider codex --model sonnet")
	// Esfuerzo
	m.Tecla(tablero.TeclaTab, ttAhora)
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "low")
	flecha(base + " --provider codex --model sonnet --effort low")
	// Presupuesto vacio es 0, sin tope
	m.Tecla(tablero.TeclaTab, ttAhora)
	ttTeclas(m, tablero.TeclaCtrlU)
	flecha(base + " --provider codex --model sonnet --effort low --budget-usd 0")
	ttEscribir(m, "9")
	// Tiempo limite
	m.Tecla(tablero.TeclaTab, ttAhora)
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "1h")
	todo := base + " --provider codex --model sonnet --effort low --budget-usd 9 --timeout 1h"
	flecha(todo)
	// shift-tab vuelve al Presupuesto; un valor igual al propuesto no es flag
	m.Tecla(tablero.TeclaShiftTab, ttAhora)
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "2.75")
	flecha(base + " --provider codex --model sonnet --effort low --timeout 1h")
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "con flags", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "hola mundo",
		"--provider", "codex", "--model", "sonnet", "--effort", "low", "--timeout", "1h"})
}

// CA-527: la marca del foco sigue a tab y shift-tab en el orden de los
// campos, en ciclo.
func TestCA527_FocoEnCiclo(t *testing.T) {
	m := ttFormulario(t, ttAncho(), "w", ttAccionWriter())
	orden := []string{"Provider", "Modelo", "Esfuerzo", "Presupuesto", "Tiempo limite", "Pedido"}
	enfocado := func() string {
		for _, l := range ttCaja(m) {
			if strings.HasPrefix(l, "> ") {
				for _, et := range orden {
					if strings.HasPrefix(l[2:], et) {
						return et
					}
				}
			}
		}
		return ""
	}
	if f := enfocado(); f != "Pedido" {
		t.Fatalf("CA-527: el foco empieza en Pedido, esta en %q", f)
	}
	for i := 0; i < 6; i++ {
		m.Tecla(tablero.TeclaTab, ttAhora)
		if f := enfocado(); f != orden[i] {
			t.Fatalf("CA-527: tab numero %d deja el foco en %q, se esperaba %q", i+1, f, orden[i])
		}
	}
	for i := 4; i >= -1; i-- {
		m.Tecla(tablero.TeclaShiftTab, ttAhora)
		quiere := orden[(i+6)%6]
		if f := enfocado(); f != quiere {
			t.Fatalf("CA-527: shift-tab deja el foco en %q, se esperaba %q", f, quiere)
		}
	}
}

// CA-527: en un texto, ← y → mueven el cursor y backspace borra antes del
// cursor; ctrl-u lo vacia.
func TestCA527_EditarElPedido(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola"
	m := ttFormulario(t, ttAncho(), "w", a)
	ttTeclas(m, tablero.TeclaIzq, tablero.TeclaIzq)
	ttEscribir(m, "Z")
	ttTeclas(m, tablero.TeclaDer, tablero.TeclaBorrar)
	ttEscribir(m, "X")
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-527", "pedido editado", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "hoZXa"})
}

// CA-527: un pedido vacio (ctrl-u, o solo espacios) no lanza: la barra dice
// "el pedido esta vacio" y el formulario sigue.
func TestCA527_PedidoVacioNoLanza(t *testing.T) {
	m := ttFormulario(t, ttAncho(), "w", ttAccionWriter())
	ttSinOrdenes(t, "CA-527", "ctrl-u", m.Tecla(tablero.TeclaCtrlU, ttAhora))
	ttSinOrdenes(t, "CA-527", "enter con el pedido vacio", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Vista() != tablero.VistaFormulario {
		t.Fatalf("CA-527: un pedido vacio no lanza: el formulario sigue, la vista es %q", m.Vista())
	}
	if b := ttBarra1(m); !strings.HasSuffix(b, " · el pedido esta vacio") {
		t.Fatalf("CA-527: la barra dice 'el pedido esta vacio': %q", b)
	}
	ttEscribir(m, "   ")
	ttSinOrdenes(t, "CA-527", "enter con un pedido de espacios", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Corriendo() != 0 {
		t.Fatalf("CA-527: nada corre")
	}
}

// CA-527: un Tiempo limite que no es una duracion de 1m a 12h dice el error
// de manifest.ParseTimeout; un Presupuesto que no es numero lo dice. Nada se
// lanza.
func TestCA527_ValoresInvalidos(t *testing.T) {
	m := ttFormulario(t, ttAncho(), "w", ttAccionWriter())
	m.Tecla(tablero.TeclaShiftTab, ttAhora) // Tiempo limite
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "5s")
	ttSinOrdenes(t, "CA-527", "enter con 5s", m.Tecla(tablero.TeclaEnter, ttAhora))
	if b := ttBarra1(m); !strings.Contains(b, `"5s" no es una duracion entre 1m y 12h`) {
		t.Fatalf("CA-527: el tiempo limite invalido dice el error de manifest.ParseTimeout: %q", b)
	}
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "45m")
	m.Tecla(tablero.TeclaShiftTab, ttAhora) // Presupuesto
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "abc")
	ttSinOrdenes(t, "CA-527", "enter con abc", m.Tecla(tablero.TeclaEnter, ttAhora))
	if b := ttBarra1(m); !strings.HasSuffix(b, " · el presupuesto no es un numero") || m.Vista() != tablero.VistaFormulario {
		t.Fatalf("CA-527: el presupuesto invalido lo dice y el formulario sigue: %q (%q)", b, m.Vista())
	}
}

// CA-527: los vacios no enfocados dicen "del provider" y "sin tope"; un
// presupuesto que la accion no propone y se escribe es flag.
func TestCA527_CamposVacios(t *testing.T) {
	a := ttAccionWriter()
	a.Model, a.Effort, a.BudgetUSD = "", "", nil
	a.Pedido = "hola"
	m := ttFormulario(t, ttAncho(), "w", a)
	if got := ttCampo(m, "Modelo"); got != "  Modelo         del provider" {
		t.Fatalf("CA-527: Modelo vacio sin foco dice 'del provider': %q", got)
	}
	if got := ttCampo(m, "Esfuerzo"); got != "  Esfuerzo       del provider" {
		t.Fatalf("CA-527: Esfuerzo vacio sin foco dice 'del provider': %q", got)
	}
	if got := ttCampo(m, "Presupuesto"); !strings.HasPrefix(got, "  Presupuesto    sin tope") {
		t.Fatalf("CA-527: Presupuesto vacio sin foco dice 'sin tope': %q", got)
	}
	if got := ttFlecha(t, "CA-527", m); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("hola", false) {
		t.Fatalf("CA-527: sin cambios no hay flags: %q", got)
	}
	ttTeclas(m, tablero.TeclaShiftTab, tablero.TeclaShiftTab)
	ttEscribir(m, "3")
	if got := ttFlecha(t, "CA-527", m); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("hola", false)+" --budget-usd 3" {
		t.Fatalf("CA-527: un presupuesto escrito es flag: %q", got)
	}
}

// CA-527: ctrl-e pide EditarPedido con el pedido; PedidoEditado lo
// reemplaza; un editor con error deja el pedido y la barra dice "el editor
// salio con error" (caso limite); esc cancela sin lanzar y vuelve a la lista.
func TestCA527_CtrlEYEsc(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "pedido original"
	m := ttFormulario(t, ttAncho(), "w", a)
	os := m.Tecla(tablero.TeclaCtrlE, ttAhora)
	if len(os) != 1 {
		t.Fatalf("CA-527: ctrl-e da una orden EditarPedido, dio %#v", os)
	}
	if ep, ok := os[0].(tablero.EditarPedido); !ok || ep.Texto != "pedido original" {
		t.Fatalf("CA-527: ctrl-e abre el pedido en el editor: %#v", os[0])
	}
	m.PedidoEditado("", errors.New("exit status 1"))
	if b := ttBarra1(m); !strings.HasSuffix(b, " · el editor salio con error") {
		t.Fatalf("CA-527: un editor con error lo dice: %q", b)
	}
	if got := ttFlecha(t, "CA-527", m); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("pedido original", false) {
		t.Fatalf("CA-527: con el editor en error el pedido queda como estaba: %q", got)
	}
	m.PedidoEditado("pedido del editor\n", nil)
	if got := ttFlecha(t, "CA-527", m); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("pedido del editor", false) {
		t.Fatalf("CA-527: lo que dio el editor es el pedido: %q", got)
	}
	ttEscribir(m, "!") // el cursor queda al final
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "pedido del editor", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "pedido del editor\n!"})

	m = ttFormulario(t, ttAncho(), "w", a)
	ttSinOrdenes(t, "CA-527", "esc", m.Tecla(tablero.TeclaEsc, ttAhora))
	if m.Vista() != tablero.VistaLista || m.Corriendo() != 0 {
		t.Fatalf("CA-527: esc cancela sin lanzar y vuelve a la lista: vista %q", m.Vista())
	}
}

// ttEnfocado es la etiqueta del campo del formulario que lleva la marca del
// foco ("> " en ASCII), o "" si ninguno la lleva.
func ttEnfocado(m *tablero.Modelo) string {
	for _, l := range ttCaja(m) {
		if !strings.HasPrefix(l, "> ") {
			continue
		}
		for _, et := range []string{"Provider", "Modelo", "Esfuerzo", "Presupuesto", "Tiempo limite", "Pedido"} {
			if strings.HasPrefix(l[2:], et) {
				return et
			}
		}
	}
	return ""
}

// CA-527: el reviewer no tiene Pedido (ni su linea vacia), el foco empieza en
// Provider y sus Args no llevan -m. Su Tiempo limite no es un campo: la
// linea lo muestra (sin la marca del foco) como "Tiempo limite  <texto> (de
// review.timeout)", tab lo saltea y nunca se manda --timeout.
//
// Hallazgo 20261108T230035_bc6939: la version de la ronda 0 exigia que el
// reviewer NO tuviera la linea Tiempo limite (lo decia el doc viejo del
// esqueleto); CA-527 dice que el formulario trae timeout_s y solo exime al
// reviewer del Pedido. Reescrito al contrato de hoy (doc.go, "The launch
// form"); lo demas que el test revisaba sigue igual.
func TestCA527_ElReviewerNoTienePedido(t *testing.T) {
	a := ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", "un pedido que no se usa")
	m := ttFormulario(t, ttAncho(), "r", a)
	if got := ttCampo(m, "Provider"); got != "> Provider       < claude >" {
		t.Fatalf("CA-527: en el reviewer el foco empieza en Provider: %q", got)
	}
	if got := ttCampo(m, "Pedido"); got != "" {
		t.Fatalf("CA-527: el reviewer no tiene campo Pedido: %q", got)
	}
	tiempo := "  Tiempo limite  45m (de review.timeout)"
	if got := ttCampo(m, "Tiempo limite"); got != tiempo {
		t.Fatalf("CA-527: el Tiempo limite del reviewer se muestra sin la marca del foco y con su origen:\n got %q\nwant %q", got, tiempo)
	}
	if got := ttFlecha(t, "CA-527", m); got != "hoom pedir reviewer tarjeta" {
		t.Fatalf("CA-527: la linea -> del reviewer no lleva -m ni --timeout: %q", got)
	}
	// tab y shift-tab saltean el Tiempo limite (y no hay Pedido): el ciclo
	// es Provider, Modelo, Esfuerzo, Presupuesto
	for _, quiere := range []string{"Modelo", "Esfuerzo", "Presupuesto", "Provider"} {
		ttSinOrdenes(t, "CA-527", "tab", m.Tecla(tablero.TeclaTab, ttAhora))
		if got := ttEnfocado(m); got != quiere {
			t.Fatalf("CA-527: en el reviewer tab deja el foco en %q, se esperaba %q (el Tiempo limite no es un campo)\n%s", got, quiere, ttPantalla(m))
		}
		if got := ttCampo(m, "Tiempo limite"); got != tiempo {
			t.Fatalf("CA-527: el Tiempo limite del reviewer nunca lleva el foco: %q", got)
		}
	}
	for _, quiere := range []string{"Presupuesto", "Esfuerzo", "Modelo", "Provider"} {
		ttSinOrdenes(t, "CA-527", "shift-tab", m.Tecla(tablero.TeclaShiftTab, ttAhora))
		if got := ttEnfocado(m); got != quiere {
			t.Fatalf("CA-527: en el reviewer shift-tab deja el foco en %q, se esperaba %q\n%s", got, quiere, ttPantalla(m))
		}
	}
	m.Tecla(tablero.TeclaTab, ttAhora)
	ttTeclas(m, tablero.TeclaCtrlU)
	ttEscribir(m, "sonnet")
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "reviewer", l.Args, []string{"pedir", "reviewer", "tarjeta", "--model", "sonnet"})
	if l.Rol != "reviewer" {
		t.Fatalf("CA-528: el Lanzar del reviewer lleva su rol: %#v", l)
	}
	// una accion sin tiempo limite: la linea dice que sale de review.timeout
	a.TimeoutS = 0
	m = ttFormulario(t, ttAncho(), "r", a)
	if got := ttCampo(m, "Tiempo limite"); !strings.HasPrefix(got, "  Tiempo limite  de review.timeout") {
		t.Fatalf("CA-527: sin tiempo limite en la accion, el del reviewer dice 'de review.timeout': %q", got)
	}
	l = ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "reviewer sin tiempo limite", l.Args, []string{"pedir", "reviewer", "tarjeta"})
}

// CA-527: reanudar muestra Provider sin flechas (← y → no hacen nada ahi) y
// ejecuta hoom reanudar <slug> -m <pedido>.
func TestCA527_Reanudar(t *testing.T) {
	a := ttPagosCaidos().Actions[0]
	a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), ttProvCodex(false)}
	m := ttFormulario(t, ttAncho(), "R", a)
	m.Tecla(tablero.TeclaTab, ttAhora) // Provider
	if got := ttCampo(m, "Provider"); got != "> Provider       claude" {
		t.Fatalf("CA-527: reanudar muestra el provider sin flechas: %q", got)
	}
	antes := ttFlecha(t, "CA-527", m)
	m.Tecla(tablero.TeclaDer, ttAhora)
	m.Tecla(tablero.TeclaIzq, ttAhora)
	if got := ttFlecha(t, "CA-527", m); got != antes || got != "hoom reanudar tarjeta -m "+tarjetacmd.CitarPedido(a.Pedido, false) {
		t.Fatalf("CA-527: en reanudar ← y → no cambian el provider: %q", got)
	}
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "reanudar", l.Args, []string{"reanudar", "tarjeta", "-m", a.Pedido})
	if l.Rol != "writer" {
		t.Fatalf("CA-528: reanudar lanza el rol de la accion: %#v", l)
	}
}

// CA-527: en Windows la linea -> cita como los verbos de la capa 1 ahi
// (tarjetacmd.CitarPedido y Citar con windows); fuera de Windows, como sh.
func TestCA527_CitaSegunElSistema(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "usa $HOME y 100%"
	for _, win := range []bool{false, true} {
		cfg := ttAncho()
		cfg.Windows = win
		m := ttFormulario(t, cfg, "w", a)
		m.Tecla(tablero.TeclaShiftTab, ttAhora)
		m.Tecla(tablero.TeclaShiftTab, ttAhora)
		m.Tecla(tablero.TeclaShiftTab, ttAhora) // Esfuerzo
		ttTeclas(m, tablero.TeclaCtrlU)
		ttEscribir(m, "muy alto")
		quiere := "hoom pedir writer tarjeta -m " + tarjetacmd.CitarPedido(a.Pedido, win) + " --effort " + tarjetacmd.Citar("muy alto", "<esfuerzo>", win)
		if got := ttFlecha(t, "CA-527", m); got != quiere {
			t.Fatalf("CA-527 (windows=%v): la linea -> cita como la capa 1:\n got %q\nwant %q", win, got, quiere)
		}
		l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, "CA-528", "valores sin citar", l.Args, []string{"pedir", "writer", "tarjeta", "-m", a.Pedido, "--effort", "muy alto"})
	}
}

// ---------------------------------------------------------------- las firmas

// CA-529: A muestra el texto de la firma con Cancelar elegido; enter enseguida
// no ejecuta nada; → (o tab) y enter ejecuta hoom aprobar <slug> --si; ←
// vuelve a Cancelar; esc cierra.
func TestCA529_AprobarSoloConFirmar(t *testing.T) {
	c := ttNucTenancy()
	a, _ := c.Action(boardcmd.ActAprobar)
	abrir := func() *tablero.Modelo {
		m := ttModelo(ttAncho(), ttTablero(c))
		m.Tecla("A", ttAhora)
		if m.Vista() != tablero.VistaConfirmacion {
			t.Fatalf("CA-529: A abre la confirmacion, la vista es %q", m.Vista())
		}
		return m
	}
	m := abrir()
	caja := strings.Join(ttCaja(m), " ")
	if !strings.Contains(caja, tarjetacmd.TextoFirma(c, a)) {
		t.Fatalf("CA-529: la confirmacion dice el texto de la firma de la capa 1:\n%s\nse esperaba %q", caja, tarjetacmd.TextoFirma(c, a))
	}
	if !strings.Contains(caja, "[ Cancelar ]   Firmar") {
		t.Fatalf("CA-529: Cancelar empieza elegido: %q", caja)
	}
	ttSinOrdenes(t, "CA-529", "enter sobre Cancelar", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-529: enter sobre Cancelar cierra, la vista es %q", m.Vista())
	}
	for _, elegir := range []tablero.Tecla{tablero.TeclaDer, tablero.TeclaTab} {
		m = abrir()
		ttSinOrdenes(t, "CA-529", string(elegir), m.Tecla(elegir, ttAhora))
		if caja := strings.Join(ttCaja(m), " "); !strings.Contains(caja, "  Cancelar   [ Firmar ]") {
			t.Fatalf("CA-529: %q elige Firmar: %q", elegir, caja)
		}
		l := ttUnaLanzar(t, "CA-529", "enter sobre Firmar", m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, "CA-529", "aprobar", l.Args, []string{"aprobar", "nuc-tenancy", "--si"})
		if l.Rol != "" {
			t.Fatalf("CA-529: aprobar no es un trabajo de rol: %#v", l)
		}
	}
	m = abrir()
	ttSinOrdenes(t, "CA-529", "→ ←", ttTeclas(m, tablero.TeclaDer, tablero.TeclaIzq))
	ttSinOrdenes(t, "CA-529", "enter tras volver a Cancelar", m.Tecla(tablero.TeclaEnter, ttAhora))
	m = abrir()
	ttSinOrdenes(t, "CA-529", "→ esc", ttTeclas(m, tablero.TeclaDer, tablero.TeclaEsc))
	if m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-529: esc cierra la confirmacion, la vista es %q", m.Vista())
	}
}

// CA-529: I igual, con hoom integrar <slug> --si y el texto de integrar.
func TestCA529_IntegrarSoloConFirmar(t *testing.T) {
	c := ttCard("lista", "Lista para integrar", boardcmd.ColTuAceptacion, "lista", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	c.Actions = []boardcmd.Action{ttAccionIntegrar()}
	a := c.Actions[0]
	m := ttModelo(ttAncho(), ttTablero(c))
	ttSinOrdenes(t, "CA-529", "I", m.Tecla("I", ttAhora))
	if caja := strings.Join(ttCaja(m), " "); !strings.Contains(caja, tarjetacmd.TextoFirma(c, a)) || !strings.Contains(caja, "[ Cancelar ]   Firmar") {
		t.Fatalf("CA-529: I muestra el texto de integrar con Cancelar elegido: %q", caja)
	}
	ttSinOrdenes(t, "CA-529", "enter sobre Cancelar", m.Tecla(tablero.TeclaEnter, ttAhora))
	m.Tecla("I", ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora)
	l := ttUnaLanzar(t, "CA-529", "enter sobre Firmar", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-529", "integrar", l.Args, []string{"integrar", "lista", "--si"})
}

// CA-529: s muestra las rutas sin guardar y ejecuta hoom guardar <slug> con
// enter; esc no ejecuta nada.
func TestCA529_Guardar(t *testing.T) {
	c := ttTarjetaCon(ttAccionGuardar(true, "", "internal/a.go", ".hoom/specs/tarjeta.md"))
	m := ttModelo(ttAncho(), ttTablero(c))
	m.Tecla("s", ttAhora)
	caja := strings.Join(ttCaja(m), "\n")
	if !strings.Contains(caja, "Se guarda en git:\ninternal/a.go\n.hoom/specs/tarjeta.md\n\nenter guardar · esc cancelar") {
		t.Fatalf("CA-529: s muestra las rutas sin guardar:\n%s", caja)
	}
	ttSinOrdenes(t, "CA-529", "esc", m.Tecla(tablero.TeclaEsc, ttAhora))
	m.Tecla("s", ttAhora)
	l := ttUnaLanzar(t, "CA-529", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-529", "guardar", l.Args, []string{"guardar", "tarjeta"})
	if l.Rol != "" {
		t.Fatalf("CA-529: guardar no es un trabajo de rol: %#v", l)
	}
}

// ---------------------------------------------------------------- lanzar y su fin

// CA-528, caso limite: dos TUI lanzan sobre la misma tarjeta. La que llega
// tarde termina antes de su primer registro con el error del candado: la
// barra lo dice, en rojo, durante DuracionAviso; ya no corre.
func TestCA528_TerminaAntesDelRegistro(t *testing.T) {
	cfg := ttAncho()
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(ttTarjetaCon(ttAccionWriter())))
	m.Tecla("w", ttAhora)
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Corriendo() != 1 {
		t.Fatalf("CA-528: lanzado, Corriendo es 1, es %d", m.Corriendo())
	}
	m.Termino(tablero.FinProceso{ID: l.ID, Code: 1, Ultima: "ya se esta lanzando un trabajo en esta tarjeta"}, ttAhora.Add(time.Second))
	if m.Corriendo() != 0 {
		t.Fatalf("CA-528: terminado, Corriendo es 0, es %d", m.Corriendo())
	}
	b := ttBarra1(m)
	if !strings.Contains(b, "ya se esta lanzando un trabajo en esta tarjeta") || !ttConColor(ttRojo, b, "ya se esta lanzando") {
		t.Fatalf("CA-528: la barra dice en rojo la ultima linea del error: %q", b)
	}
	m.Tic(ttAhora.Add(time.Second + tablero.DuracionAviso + time.Second))
	if strings.Contains(ttBarra1(m), "ya se esta lanzando") {
		t.Fatalf("CA-528: el aviso dura DuracionAviso: %q", ttBarra1(m))
	}
	// sin color, el mismo texto
	m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter())))
	m.Tecla("w", ttAhora)
	l = ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	m.Termino(tablero.FinProceso{ID: l.ID, Code: 1, Ultima: "ya se esta lanzando un trabajo en esta tarjeta"}, ttAhora)
	if b := ttBarra1(m); !strings.HasSuffix(b, " · ya se esta lanzando un trabajo en esta tarjeta") {
		t.Fatalf("CA-528: la barra dice la ultima linea del error: %q", b)
	}
}

// ttCorriendoCon es la tarjeta de prueba con un running del sobre env.
func ttCorriendoCon(env string) boardcmd.Card {
	c := ttTarjetaCon(ttAccionWriter())
	c.Running = &boardcmd.Running{EnvelopeID: env, RunID: "run-" + env, Role: "writer", Provider: "claude", Stage: "run",
		Step: 2, Steps: 6, StartedAt: ttAhora}
	return c
}

// CA-528, CA-530: lanzado y registrado (la tarjeta muestra running con un
// sobre nuevo), su fin deja la barra al aviso de Leer: "<rol> termino en
// <slug>: <plain>" durante DuracionFin, en rojo si el rojo de la tarjeta es
// el de ese sobre.
func TestCA528_CA530_RegistradoYTermino(t *testing.T) {
	cfg := ttAncho()
	cfg.Color = true
	viejo := ttCorriendoCon("env-viejo")
	m := ttModelo(cfg, ttTablero(viejo))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttTarjetaCon(ttAccionWriter()))}) // termino el viejo
	m.Tic(ttAhora.Add(time.Minute))                                                             // y su aviso se fue
	m.Tecla("w", ttAhora.Add(time.Minute))
	l := ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora.Add(time.Minute)))
	t1 := ttAhora.Add(time.Minute + 2*time.Second)
	m.Leer(tablero.Lectura{Ahora: t1, Tablero: ttTablero(ttCorriendoCon("env-nuevo"))})
	m.Termino(tablero.FinProceso{ID: l.ID, Code: 1, Ultima: "algo fallo en el writer"}, t1.Add(time.Second))
	if b := ttBarra1(m); strings.Contains(b, "algo fallo en el writer") {
		t.Fatalf("CA-528: registrado, el fin del proceso deja la barra al aviso de Leer: %q", b)
	}
	fin := ttTarjetaCon(ttAccionWriter())
	fin.Plain = "la verificacion dio rojo"
	fin.Red = &boardcmd.Red{Source: boardcmd.RedEnvelope, ID: "env-nuevo", Plain: "la verificacion dio rojo"}
	t2 := t1.Add(10 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t2, Tablero: ttTablero(fin)})
	b := ttBarra1(m)
	if !strings.Contains(b, "writer termino en tarjeta: la verificacion dio rojo") || !ttConColor(ttRojo, b, "writer termino en tarjeta") {
		t.Fatalf("CA-530: el aviso de fin, en rojo porque el rojo es de su sobre: %q", b)
	}
	m.Tic(t2.Add(tablero.DuracionFin - time.Second))
	if !strings.Contains(ttBarra1(m), "writer termino en tarjeta") {
		t.Fatalf("CA-530: el aviso dura DuracionFin (8s): a los 7s sigue: %q", ttBarra1(m))
	}
	m.Tic(t2.Add(tablero.DuracionFin + time.Second))
	if strings.Contains(ttBarra1(m), "termino en") {
		t.Fatalf("CA-530: pasado DuracionFin el aviso se va: %q", ttBarra1(m))
	}
}

// CA-530, caso limite: un rol lanzado desde otra terminal (la TUI nunca lo
// lanzo) aparece igual y su fin avisa; en verde cuando el rojo no es el de
// su sobre (o no hay rojo).
func TestCA530_FinDeUnRolLanzadoDesdeOtraTerminal(t *testing.T) {
	cfg := ttConfig(120, 24, false)
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(ttCorriendoCon("env-ajeno")))
	if l, _ := ttLineaCon(m, "writer · run 2/6"); l == "" {
		t.Fatalf("CA-530: el running ajeno se ve en la lista:\n%s", ttPantalla(m))
	}
	fin := ttTarjetaCon(ttAccionWriter())
	fin.Plain = "espera tu aceptacion"
	fin.Red = &boardcmd.Red{Source: boardcmd.RedEnvelope, ID: "env-otro-mas-viejo", Plain: "x"}
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(fin)})
	b := ttBarra1(m)
	if !strings.Contains(b, "writer termino en tarjeta: espera tu aceptacion") || !ttConColor(ttVerde, b, "writer termino en tarjeta") {
		t.Fatalf("CA-530: el fin de un rol ajeno avisa en verde: %q", b)
	}
	// sin rojo, tambien verde
	m = ttModelo(cfg, ttTablero(ttCorriendoCon("env-ajeno")))
	fin.Red = nil
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(fin)})
	if b := ttBarra1(m); !ttConColor(ttVerde, b, "writer termino en tarjeta") {
		t.Fatalf("CA-530: sin rojo el aviso es verde: %q", b)
	}
}

// CA-530: una lectura que falla deja la ultima pantalla y la barra dice "no
// se pudo leer el tablero: <error>" hasta que una lectura funcione.
func TestCA530_LecturaQueFalla(t *testing.T) {
	m := ttModelo(ttConfig(120, 24, false), ttTableroPrincipal())
	antes := ttCuerpo(m)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Err: errors.New("disco roto")})
	if b := ttBarra1(m); !strings.HasSuffix(b, " · no se pudo leer el tablero: disco roto") {
		t.Fatalf("CA-530: la barra dice 'no se pudo leer el tablero: disco roto': %q", b)
	}
	if strings.Join(ttCuerpo(m), "\n") != strings.Join(antes, "\n") || ttLinea(m, 1) != " hoom · demo · 9 tarjetas" {
		t.Fatalf("CA-530: la pantalla se queda con el ultimo tablero:\n%s", ttPantalla(m))
	}
	m.Tic(ttAhora.Add(time.Minute))
	if b := ttBarra1(m); !strings.Contains(b, "no se pudo leer el tablero") {
		t.Fatalf("CA-530: el error de lectura sigue hasta que una lectura funcione: %q", b)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Minute + 2*time.Second), Tablero: ttTableroPrincipal()})
	if b := ttBarra1(m); strings.Contains(b, "no se pudo leer") {
		t.Fatalf("CA-530: una lectura que funciona quita el error: %q", b)
	}
	// antes de la primera lectura buena tambien lo dice
	m = tablero.Nuevo(ttConfig(120, 24, false))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Err: errors.New("sin permiso")})
	if b := ttBarra1(m); !strings.HasSuffix(b, " · no se pudo leer el tablero: sin permiso") {
		t.Fatalf("CA-530: una primera lectura que falla lo dice: %q", b)
	}
}

// CA-528: MensajeSalida dice cuantos trabajos de rol quedan corriendo.
func TestCA528_MensajeSalida(t *testing.T) {
	if got := tablero.MensajeSalida(0); got != "" {
		t.Fatalf("CA-528: sin trabajos no hay mensaje: %q", got)
	}
	if got := tablero.MensajeSalida(1); got != "queda 1 trabajo corriendo: hoom status" {
		t.Fatalf("CA-528: con uno: %q", got)
	}
	for _, n := range []int{2, 3, 17} {
		quiere := "quedan " + itoa(n) + " trabajos corriendo: hoom status"
		if got := tablero.MensajeSalida(n); got != quiere {
			t.Fatalf("CA-528: con %d: %q, se esperaba %q", n, got, quiere)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// CA-528: Corriendo cuenta los trabajos de rol lanzados que no terminaron;
// el fin de uno lo descuenta, el de un ID desconocido no.
func TestCA528_Corriendo(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter())))
	var ids []int
	for i := 0; i < 3; i++ {
		m.Tecla("w", ttAhora)
		ids = append(ids, ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora)).ID)
	}
	if ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Fatalf("CA-528: los ID van de 1 en 1: %v", ids)
	}
	if m.Corriendo() != 3 {
		t.Fatalf("CA-528: tres lanzados, Corriendo = %d", m.Corriendo())
	}
	m.Termino(tablero.FinProceso{ID: 99, Code: 0}, ttAhora)
	m.Termino(tablero.FinProceso{ID: 2, Code: 0, Ultima: "ok"}, ttAhora)
	if m.Corriendo() != 2 {
		t.Fatalf("CA-528: termino uno (y uno desconocido no cuenta), Corriendo = %d", m.Corriendo())
	}
	if got := tablero.MensajeSalida(m.Corriendo()); got != "quedan 2 trabajos corriendo: hoom status" {
		t.Fatalf("CA-528: al salir: %q", got)
	}
}

// q y ctrl-c ordenan Salir desde las vistas de la lista y la tarjeta;
// ctrl-c tambien desde la busqueda, el formulario y la confirmacion.
func TestCA533_QYCtrlCOrdenanSalir(t *testing.T) {
	salir := func(os []tablero.Orden) bool {
		return len(os) == 1 && os[0] == tablero.Orden(tablero.Salir{})
	}
	for _, k := range []tablero.Tecla{"q", tablero.TeclaCtrlC} {
		m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter())))
		if os := m.Tecla(k, ttAhora); !salir(os) {
			t.Fatalf("CA-533: %q en la lista ordena Salir: %#v", k, os)
		}
		m.Tecla(tablero.TeclaEnter, ttAhora)
		if os := m.Tecla(k, ttAhora); !salir(os) {
			t.Fatalf("CA-533: %q en la tarjeta abierta ordena Salir: %#v", k, os)
		}
	}
	m := ttFormulario(t, ttAncho(), "w", ttAccionWriter())
	ttEscribir(m, "q") // q en el formulario es texto
	if os := m.Tecla(tablero.TeclaCtrlC, ttAhora); !salir(os) {
		t.Fatalf("CA-533: ctrl-c en el formulario ordena Salir: %#v", os)
	}
}
