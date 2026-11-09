// Los modales (spec tablero-con-paneles): CA-565 lo comun (centrado, el
// borde del foco, el fondo gris, las teclas solo al modal, esc, los
// botones, el cambio de tamano), CA-566 nada a ciegas, CA-567 Acciones,
// CA-568 Pedir, CA-569 y CA-570 Modelo y Esfuerzo, CA-571 Tope y Tiempo
// limite, CA-574 Firma y Guardar (con la regla de la v1 que ata la
// confirmacion a lo que muestra). Lanzar pasa por las ordenes del modelo,
// sin ejecutar nada.
//
// Correcciones del contrato de doc.go que el orquestador paso para esta
// ronda (no estan en la copia de doc.go del arbol ciego): con un modal
// abierto solo las lineas de los paneles (1 a Alto-2) van en gris, la linea
// de mensajes y la de teclas conservan su estilo; el why de un selector gris
// que no entra en la columna del origen va en su propia linea debajo del
// selector, desde la columna 15, en gris.
package tablero_test

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/modelos"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
	"github.com/hoomdev/hoomai/internal/valores"
)

// ttAbrirModal arma la tarjeta "tarjeta" con a, aprieta k y exige el modal.
func ttAbrirModal(t *testing.T, cfg tablero.Config, k tablero.Tecla, quiere tablero.Modal, a ...boardcmd.Action) *tablero.Modelo {
	t.Helper()
	m := ttModelo(cfg, ttTablero(ttTarjetaCon(a...)))
	ttSinOrdenes(t, "CA-566", string(k), m.Tecla(k, ttAhora))
	if m.Modal() != quiere {
		t.Fatalf("CA-566: %q abre el modal %q, el modal es %q\n%s", k, quiere, m.Modal(), ttPantalla(m))
	}
	return m
}

// ttPedir abre el modal Pedir de la tecla k con la accion a, a 200x40.
func ttPedir(t *testing.T, k tablero.Tecla, a boardcmd.Action) *tablero.Modelo {
	t.Helper()
	return ttAbrirModal(t, ttAncho(), k, tablero.ModalPedir, a)
}

// ttMC es el modal de m a 200 columnas.
func ttMC(t *testing.T, m *tablero.Modelo) ttModalCaja {
	t.Helper()
	return ttModal(t, "CA-565", m, 200)
}

// ttAyudaSel es la linea de ayuda de los selectores (la que esta dos lineas
// antes de "Se va a ejecutar:", con su linea vacia entre las dos), sin sus 15
// espacios.
func ttAyudaSel(mc ttModalCaja) string {
	_, i, _ := mc.ttLineaQueEmpieza("Se va a ejecutar:")
	if i < 2 || mc.Textos[i-1] != "" {
		return "?"
	}
	return strings.TrimSpace(mc.Textos[i-2])
}

// ttWhyDe es el why (o el origen) de un selector: en su linea, despues del
// valor; o, si no entra, en la linea de abajo desde la columna 15.
func ttWhyDe(mc ttModalCaja, etiqueta string) string {
	if o := mc.ttOrigen(etiqueta); o != "" {
		return o
	}
	_, i, _ := mc.ttLineaQueEmpieza(etiqueta)
	if i < 0 || i+1 >= len(mc.Textos) {
		return ""
	}
	sig := mc.Textos[i+1]
	if strings.HasPrefix(sig, strings.Repeat(" ", 15)) {
		return strings.TrimSpace(sig)
	}
	return ""
}

// ttLista son las opciones de la lista abierta del modal (su caja, desde la
// columna del valor, 15, hasta la ultima celda del texto), cada una sin el
// relleno; y su borde de abajo.
func ttLista(t *testing.T, mc ttModalCaja) ([]string, string) {
	t.Helper()
	ini := -1
	for i, l := range mc.Textos {
		c := ttCeldasDe(l)
		if len(c) > 16 && (c[15] == "+" || c[15] == "┌") && ttEsLinea(c[16]) {
			ini = i
			break
		}
	}
	if ini < 0 {
		t.Fatalf("CA-568: el modal tiene una lista abierta (una caja desde la columna 15):\n%s", strings.Join(mc.Textos, "\n"))
	}
	var out []string
	for i := ini + 1; i < len(mc.Textos); i++ {
		c := ttCeldasDe(mc.Textos[i])
		if len(c) > 16 && (c[15] == "+" || c[15] == "└") && ttEsLinea(c[16]) {
			return out, strings.Join(c[15:], "")
		}
		if len(c) < 17 {
			t.Fatalf("CA-568: una linea de la lista: %q", mc.Textos[i])
		}
		fin := len(c)
		if ttEsLado(c[fin-1]) {
			fin--
		}
		out = append(out, strings.TrimRight(strings.Join(c[16:fin], ""), " "))
	}
	t.Fatalf("CA-568: la lista tiene su borde de abajo:\n%s", strings.Join(mc.Textos, "\n"))
	return nil, ""
}

// ttOpcionElegida es la opcion de la lista con la marca (" > " o " ▸ ").
func ttOpcionElegida(ops []string) string {
	for _, o := range ops {
		if strings.HasPrefix(o, " > ") || strings.HasPrefix(o, " ▸ ") {
			return strings.Fields(o[3:])[0]
		}
	}
	return ""
}

// ttFoco es la etiqueta del campo del modal Pedir con la marca: Pedido,
// Provider, Modelo, Esfuerzo, Tope, Tiempo limite, o "botones".
func ttFoco(mc ttModalCaja) string {
	l := mc.ttMarcada()
	for _, et := range []string{"Pedido", "Provider", "Modelo", "Esfuerzo", "Tope", "Tiempo limite", "Tiempo límite"} {
		if strings.HasPrefix(l, et) {
			return et
		}
	}
	if strings.Contains(l, "[ ") {
		return "botones"
	}
	return l
}

// ---------------------------------------------------------------- CA-565: lo comun

// CA-565: el modal es un cuadro de Wm = min(72, W-4) celdas centrado sobre
// los paneles (su celda izquierda en (W-Wm)/2, su linea de arriba en
// (B-Hm)/2), con el borde del foco y su titulo en el borde de arriba; al
// cambiar el tamano se centra de nuevo.
func TestCA565_CentradoYTitulo(t *testing.T) {
	for _, tam := range [][2]int{{200, 40}, {80, 24}, {120, 32}, {76, 30}} {
		cfg := ttConfig(tam[0], tam[1], false)
		m := ttAbrirModal(t, cfg, "w", tablero.ModalPedir, ttAccionWriter())
		mc := ttModal(t, "CA-565", m, tam[0])
		b := tam[1] - 2
		if mc.Col != (tam[0]-ttAnchoModal(tam[0]))/2 || mc.Fila != (b-mc.Alto)/2 {
			t.Fatalf("CA-565: a %dx%d el modal esta centrado: celda %d, linea %d; esta en %d, %d (alto %d)",
				tam[0], tam[1], (tam[0]-ttAnchoModal(tam[0]))/2, (b-mc.Alto)/2, mc.Col, mc.Fila, mc.Alto)
		}
		if !strings.HasPrefix(mc.Arriba, "# Pedir al writer · tarjeta =") || !strings.HasSuffix(mc.Arriba, "=#") {
			t.Fatalf("CA-565: el borde de arriba es la esquina del foco, un espacio, el titulo, un espacio y signos del foco: %q", mc.Arriba)
		}
		if !strings.HasPrefix(mc.Abajo, "#=") || !strings.HasSuffix(mc.Abajo, "=#") {
			t.Fatalf("CA-565: el borde de abajo del modal lleva los signos del foco: %q", mc.Abajo)
		}
	}
	// el cambio de tamano con el modal abierto lo centra de nuevo
	m := ttAbrirModal(t, ttConfig(200, 40, false), "w", tablero.ModalPedir, ttAccionWriter())
	m.Tamano(120, 30)
	mc := ttModal(t, "CA-565", m, 120)
	if mc.Col != 24 || mc.Fila != (28-mc.Alto)/2 || m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-565: tras Tamano(120, 30) el modal sigue abierto y centrado (celda 24): %+v", mc)
	}
	ttForma(t, "CA-565", m, 120, 30, false)
	// con UTF-8, la linea doble
	u := ttAbrirModal(t, ttConfig(200, 40, true), "w", tablero.ModalPedir, ttAccionWriter())
	if mc := ttModal(t, "CA-565", u, 200); !strings.HasPrefix(mc.Arriba, "╔ Pedir al writer · tarjeta ═") || !strings.HasSuffix(mc.Abajo, "╝") {
		t.Fatalf("CA-565: con UTF-8 el modal lleva la linea doble: %q / %q", mc.Arriba, mc.Abajo)
	}
}

// CA-565: con color, el borde del modal va en 1;32 y todo lo de atras, en las
// lineas de los paneles, en gris solo (90); la linea de mensajes y la de
// teclas conservan su estilo (correccion del orquestador): un error del
// modal como "el pedido esta vacio" sigue en rojo.
func TestCA565_ElFondoGris(t *testing.T) {
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	m := ttModelo(cfg, ttTableroDorado())
	m.LeerProyecto(ttProyectoVerde())
	m.LeerHallazgos(ttHallazgosDorados())
	ttIrA(t, m, "infra-api")
	m.Tecla("w", ttAhora)
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-566: w abre Pedir: %q", m.Modal())
	}
	p := m.Pantalla()
	mc := ttModal(t, "CA-565", m, 120)
	for i := 0; i < mc.Fila; i++ {
		if quiere := "\x1b[90m" + ttSinSGR(p[i]) + "\x1b[0m"; p[i] != quiere {
			t.Fatalf("CA-565: la linea %d (encima del modal) va toda en gris solo, sin otro estilo:\n got %q\nwant %q", i+1, p[i], quiere)
		}
	}
	top := p[mc.Fila]
	if !strings.HasPrefix(top, "\x1b[90m") || !strings.Contains(top, "\x1b[1;32m# Pedir al writer · infra-api") {
		t.Fatalf("CA-565: a la izquierda del modal, gris; su borde, en 1;32: %q", top)
	}
	for i := mc.Fila + mc.Alto; i < len(p)-2; i++ {
		for _, sgr := range ttSGR.FindAllString(p[i], -1) {
			if sgr != "\x1b[90m" && sgr != "\x1b[0m" {
				t.Fatalf("CA-565: debajo del modal, las lineas de los paneles van solo en gris (linea %d): %q", i+1, p[i])
			}
		}
	}
	// el error del modal sigue en rojo en la linea de mensajes
	m.Tecla(tablero.TeclaCtrlU, ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora) // al boton
	ttSinOrdenes(t, "CA-568", "enter con el pedido vacio", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttDiceMensaje(t, "CA-565", "el error con el modal abierto", m, true, "el pedido esta vacio")
	if teclas := m.Pantalla()[len(p)-1]; strings.HasPrefix(teclas, "\x1b[90m") {
		t.Fatalf("CA-565: la linea de teclas no va en gris con el modal: %q", teclas)
	}
}

// CA-565: mientras hay un modal abierto las teclas van solo a el: las de los
// paneles (1..4, 0, j, k, /, x, q, las de accion) no mueven nada atras; esc
// lo cierra sin ejecutar nada (con una lista abierta, cierra solo la lista).
func TestCA565_LasTeclasVanAlModal(t *testing.T) {
	for _, abrir := range []struct {
		k     tablero.Tecla
		modal tablero.Modal
		a     []boardcmd.Action
	}{
		{"w", tablero.ModalPedir, []boardcmd.Action{ttAccionWriter()}},
		{"A", tablero.ModalFirma, []boardcmd.Action{ttAccionAprobar()}},
		{"s", tablero.ModalGuardar, []boardcmd.Action{ttAccionGuardar(true, "", "a.go")}},
		{tablero.TeclaEspacio, tablero.ModalAcciones, []boardcmd.Action{ttAccionWriter(), ttAccionAprobar()}},
		{"?", tablero.ModalAyuda, []boardcmd.Action{ttAccionWriter()}},
	} {
		m := ttAbrirModal(t, ttAncho(), abrir.k, abrir.modal, abrir.a...)
		foco, elegida := m.Foco(), m.Elegida()
		for _, k := range []tablero.Tecla{"1", "3", "4", "0", "/", "x", "q", "u", "n", "a", "R", "I"} {
			if abrir.modal == tablero.ModalPedir {
				break // en el Pedido, las letras son texto
			}
			os := m.Tecla(k, ttAhora)
			if len(ttLanzares(os)) != 0 && k != "u" {
				t.Fatalf("CA-565: con %q abierto %q no lanza nada: %#v", abrir.modal, k, os)
			}
			if k == "q" && len(os) != 0 {
				t.Fatalf("CA-565: con un modal abierto q no sale: %#v", os)
			}
		}
		if m.Foco() != foco || m.Elegida() != elegida {
			t.Fatalf("CA-565: con %q abierto nada se movio atras: foco %d, elegida %q", abrir.modal, int(m.Foco()), m.Elegida())
		}
		ttSinOrdenes(t, "CA-565", "esc", m.Tecla(tablero.TeclaEsc, ttAhora))
		if m.Modal() != tablero.SinModal || m.Corriendo() != 0 {
			t.Fatalf("CA-565: esc cierra %q sin ejecutar nada: %q", abrir.modal, m.Modal())
		}
		if os := m.Tecla(tablero.TeclaCtrlC, ttAhora); len(os) != 1 || os[0] != tablero.Orden(tablero.Salir{}) {
			t.Fatalf("CA-565: ctrl-c sale siempre: %#v", os)
		}
	}
	// ctrl-c tambien con un modal abierto y con una lista abierta
	m := ttPedir(t, "w", ttAccionWriter())
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if m.Lugar() != tablero.LugarPedirLista {
		t.Fatalf("CA-568: enter en Modelo abre su lista: %q", m.Lugar())
	}
	ttSinOrdenes(t, "CA-565", "esc con la lista abierta", m.Tecla(tablero.TeclaEsc, ttAhora))
	if m.Modal() != tablero.ModalPedir || m.Lugar() != tablero.LugarPedirSelector {
		t.Fatalf("CA-565: esc con una lista abierta cierra solo la lista: %q %q", m.Modal(), m.Lugar())
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if os := m.Tecla(tablero.TeclaCtrlC, ttAhora); len(os) != 1 || os[0] != tablero.Orden(tablero.Salir{}) {
		t.Fatalf("CA-565: ctrl-c con una lista abierta sale: %#v", os)
	}
}

// ---------------------------------------------------------------- CA-566: nada a ciegas

var ttTeclasDeAccion = []tablero.Tecla{"a", "t", "w", "r", "R", "s", "A", "I"}

// ttAccionDeTecla es la accion habilitada que la tecla abre y su modal.
func ttAccionDeTecla(k tablero.Tecla) (boardcmd.Action, tablero.Modal) {
	switch k {
	case "a":
		return ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec"), tablero.ModalPedir
	case "t":
		return ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests"), tablero.ModalPedir
	case "w":
		return ttAccionWriter(), tablero.ModalPedir
	case "r":
		return ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""), tablero.ModalPedir
	case "R":
		return ttPagosCaidos().Actions[0], tablero.ModalPedir
	case "s":
		return ttAccionGuardar(true, "", "internal/a.go", ".hoom/specs/tarjeta.md"), tablero.ModalGuardar
	case "A":
		return ttAccionAprobar(), tablero.ModalFirma
	case "I":
		return ttAccionIntegrar(), tablero.ModalFirma
	}
	panic("tecla sin accion: " + string(k))
}

// ttEnLugar lleva el modelo (foco en [2] sobre "tarjeta") al lugar desde
// donde se aprieta la tecla: [2], [0] de la tarjeta, o el modal Acciones.
func ttEnLugar(t *testing.T, m *tablero.Modelo, l tablero.Lugar) {
	t.Helper()
	switch l {
	case tablero.LugarVerTarjeta:
		m.Tecla(tablero.TeclaEnter, ttAhora)
	case tablero.LugarAcciones:
		m.Tecla(tablero.TeclaEspacio, ttAhora)
	}
	if m.Lugar() != l {
		t.Fatalf("no pude llegar a %q (estoy en %q)", l, m.Lugar())
	}
}

// CA-566, CA-525: cada tecla de accion, con la accion habilitada, abre su
// modal y no da ninguna orden (desde [2], [0] de la tarjeta y el modal de
// acciones); sin la accion, la linea de mensajes dice "en <columna> no hay
// <hace>"; deshabilitada, su why. En ningun caso se ejecuta nada. En la v1
// (CA-525) la tecla abria el formulario o la confirmacion; en la v2, su
// modal.
func TestCA566_CA525_TeclasDeAccionAbrenSuModal(t *testing.T) {
	for _, desde := range []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarVerTarjeta, tablero.LugarAcciones} {
		for _, k := range ttTeclasDeAccion {
			a, modal := ttAccionDeTecla(k)
			m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(a)))
			ttEnLugar(t, m, desde)
			ttSinOrdenes(t, "CA-566", string(k)+" (habilitada)", m.Tecla(k, ttAhora))
			if m.Modal() != modal {
				t.Fatalf("CA-566: desde %q, %q con %s habilitada abre %q, el modal es %q", desde, k, a.ID, modal, m.Modal())
			}
			// sin la accion (con otra sin verbo, para que Acciones tenga algo)
			m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttRelanzar())))
			ttEnLugar(t, m, desde)
			ttSinOrdenes(t, "CA-525", string(k)+" (sin la accion)", m.Tecla(k, ttAhora))
			ttDiceMensaje(t, "CA-525", fmt.Sprintf("desde %q, %q sin %s", desde, k, a.ID), m, false, "en Writer no hay "+ttHaceDe(t, k))
			if m.Lugar() != desde {
				t.Fatalf("CA-525: sin la accion el lugar sigue (%q): %q", desde, m.Lugar())
			}
			// deshabilitada
			a.Enabled, a.Why = false, "un motivo de la tarjeta para "+a.ID
			m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(a)))
			ttEnLugar(t, m, desde)
			ttSinOrdenes(t, "CA-525", string(k)+" (deshabilitada)", m.Tecla(k, ttAhora))
			ttDiceMensaje(t, "CA-525", fmt.Sprintf("desde %q, %q deshabilitada", desde, k), m, false, a.Why)
			m.Tic(ttAhora.Add(tablero.DuracionAviso + time.Second))
			if strings.Contains(ttMensaje(m), a.Why) {
				t.Fatalf("CA-525: el why se va pasado DuracionAviso: %q", ttMensaje(m))
			}
		}
	}
}

// CA-566, CA-525, caso limite de la v1: t con el spec sin guardar: la linea
// de mensajes dice el why de la capa 1 y no se abre el modal.
func TestCA566_CA525_TConElSpecSinGuardar(t *testing.T) {
	a := ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests")
	a.Enabled, a.Why = false, boardcmd.WhySpecSinGuardar
	c := ttTarjetaCon(a)
	c.Column, c.ColumnName = boardcmd.ColTestWriter, "Test-writer"
	m := ttModelo(ttAncho(), ttTablero(c))
	ttSinOrdenes(t, "CA-525", "t", m.Tecla("t", ttAhora))
	if m.Modal() != tablero.SinModal {
		t.Fatalf("CA-525: t con el spec sin guardar no abre el modal: %q", m.Modal())
	}
	ttDiceMensaje(t, "CA-525", "t", m, false, boardcmd.WhySpecSinGuardar)
}

// CA-566, CA-525: sin tarjeta (la linea de Hecho, [3] vacio, [1] o [4]) las
// teclas de accion, n y u no hacen nada.
func TestCA566_CA525_SinTarjeta(t *testing.T) {
	for _, lugar := range []tablero.Tecla{"G", "3", "1", "4"} {
		m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter(), ttAccionAprobar())))
		m.Tecla(lugar, ttAhora)
		antes := ttPantalla(m)
		for _, k := range append([]tablero.Tecla{"n", "u"}, ttTeclasDeAccion...) {
			ttSinOrdenes(t, "CA-525", fmt.Sprintf("%q tras %q", k, lugar), m.Tecla(k, ttAhora))
			if ttPantalla(m) != antes || m.Modal() != tablero.SinModal {
				t.Fatalf("CA-525: %q sin tarjeta (tras %q) no hace nada:\n%s", k, lugar, ttPantalla(m))
			}
		}
	}
}

// CA-566, CA-528: n abre el modal de la accion del siguiente paso, sea de
// pedir, de firma o de guardar, y su boton ejecuta el verbo de esa accion (no
// hoom siguiente); sin siguiente paso, la linea de mensajes dice lo que diria
// hoom siguiente y nada se abre. Re-expresa el test de la v1 de CA-528 que
// esperaba "lanzando hoom siguiente" (decision del orquestador).
func TestCA566_CA528_NAbreElModalDelSiguientePaso(t *testing.T) {
	casos := []struct {
		nombre string
		card   boardcmd.Card
		modal  tablero.Modal
		args   []string
		rol    string
	}{
		{"pedir", ttTarjetaCon(ttAccionWriter()), tablero.ModalPedir,
			[]string{"pedir", "writer", "tarjeta", "-m", strings.TrimSpace(ttPedidoWriter)}, "writer"},
		{"firma", ttNucTenancy(), tablero.ModalFirma, []string{"aprobar", "nuc-tenancy", "--si"}, ""},
		{"guardar", ttTarjetaCon(ttAccionGuardar(true, "", "a.go"), ttAccionWriter()), tablero.ModalGuardar, []string{"guardar", "tarjeta"}, ""},
	}
	for _, c := range casos {
		m := ttModelo(ttAncho(), ttTablero(c.card))
		ttSinOrdenes(t, "CA-566", "n ("+c.nombre+")", m.Tecla("n", ttAhora))
		if m.Modal() != c.modal {
			t.Fatalf("CA-566: n (%s) abre %q, abrio %q", c.nombre, c.modal, m.Modal())
		}
		if c.modal == tablero.ModalPedir {
			m.Tecla(tablero.TeclaEnter, ttAhora) // al boton
		}
		if c.modal == tablero.ModalFirma {
			m.Tecla(tablero.TeclaDer, ttAhora) // de Cancelar a la accion
		}
		l := ttUnaLanzar(t, "CA-566", "el boton de n ("+c.nombre+")", m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, "CA-566", "n ("+c.nombre+")", l.Args, c.args)
		if l.Rol != c.rol || l.ID != 1 {
			t.Fatalf("CA-528: el Lanzar de n lleva el rol de la accion y el ID 1: %#v", l)
		}
	}
	// sin siguiente paso
	conRunning := ttTarjetaCon(ttAccionWriter())
	conRunning.Running = &boardcmd.Running{EnvelopeID: "e", Role: "writer", Stage: "run", StartedAt: ttAhora}
	m := ttModelo(ttAncho(), ttTablero(conRunning))
	ttSinOrdenes(t, "CA-566", "n con running", m.Tecla("n", ttAhora))
	if m.Modal() != tablero.SinModal {
		t.Fatalf("CA-566: sin siguiente paso n no abre nada: %q", m.Modal())
	}
	ttDiceMensaje(t, "CA-566", "n con running", m, false, "el writer esta trabajando: espera a que termine")
}

// CA-566 (propiedad): ninguna tecla de accion salvo u produce un Lanzar como
// primera orden, desde cualquier panel, con cualquier seleccion: las ordenes
// de esa tecla nunca traen un Lanzar.
func TestCA566_PropiedadNingunaTeclaDeAccionLanza(t *testing.T) {
	moverse := []tablero.Tecla{"1", "2", "3", "4", "0", "j", "k", "G", "g", tablero.TeclaEnter, tablero.TeclaEsc, "x"}
	accion := append([]tablero.Tecla{"n"}, ttTeclasDeAccion...)
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		c := ttTarjetaCon(ttAccionWriter(), ttAccionAprobar(), ttAccionGuardar(true, "", "a.go"), ttAccionIntegrar(),
			ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "x"))
		m := ttModelo(ttAncho(), ttTablero(c, ttInfraAPI(true), ttNucTenancy(), ttPagosCaidos(), ttViejo(1)))
		m.LeerHallazgos(ttHallazgosDorados())
		for i := 0; i < 30; i++ {
			if m.Modal() != tablero.SinModal {
				m.Tecla(tablero.TeclaEsc, ttAhora)
			}
			for j := r.Intn(4); j > 0; j-- {
				m.Tecla(moverse[r.Intn(len(moverse))], ttAhora)
			}
			if m.Lugar() == tablero.LugarBusqueda {
				m.Tecla(tablero.TeclaEsc, ttAhora)
			}
			k := accion[r.Intn(len(accion))]
			if ls := ttLanzares(m.Tecla(k, ttAhora)); len(ls) != 0 {
				t.Logf("semilla %d: %q en %q dio un Lanzar: %#v", semilla, k, m.Lugar(), ls)
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatalf("CA-566: %v", err)
	}
}

// ---------------------------------------------------------------- CA-567: Acciones

// CA-567: espacio abre el modal de acciones de la tarjeta: una linea por
// accion en su orden (tecla, etiqueta y verbo), las deshabilitadas en gris
// con su why, las sin verbo en gris con "sin verbo en la terminal", las de
// experto solo en modo experto; la marca empieza en la primera habilitada
// con verbo y j k la mueven solo sobre esas; enter abre el modal de la
// elegida; la tecla de una accion hace lo de afuera. Re-expresa CA-526.
func TestCA567_CA526_ElModalDeAcciones(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttInfraAPI(false)))
	ttSinOrdenes(t, "CA-567", "espacio", m.Tecla(tablero.TeclaEspacio, ttAhora))
	if m.Modal() != tablero.ModalAcciones || m.Lugar() != tablero.LugarAcciones {
		t.Fatalf("CA-567: espacio abre Acciones: %q %q", m.Modal(), m.Lugar())
	}
	mc := ttMC(t, m)
	if !strings.HasPrefix(mc.Arriba, "# Acciones · infra-api =") {
		t.Fatalf("CA-567: el titulo es 'Acciones · <slug>': %q", mc.Arriba)
	}
	quiere := []string{
		"w Pedir al writer  hoom pedir writer infra-api",
		"s Guardar  hoom guardar infra-api  (" + ttWhyGuardar + ")",
		"  Volver a lanzar  sin verbo en la terminal",
		"A Aprobar spec  hoom aprobar infra-api --si",
	}
	ttIgualesAlPrincipio(t, "CA-567", "las lineas de Acciones", mc.Textos, quiere)
	if mc.ttTieneLinea("  Descartar cambios  sin verbo en la terminal") {
		t.Fatalf("CA-567: la de experto no aparece en modo normal")
	}
	if got := mc.ttMarcada(); got != quiere[0] {
		t.Fatalf("CA-567: la marca empieza en la primera habilitada con verbo: %q", got)
	}
	m.Tecla("j", ttAhora)
	if got := ttMC(t, m).ttMarcada(); got != quiere[3] {
		t.Fatalf("CA-567: j salta la deshabilitada y la sin verbo: %q", got)
	}
	m.Tecla(tablero.TeclaAbajo, ttAhora)
	if got := ttMC(t, m).ttMarcada(); got != quiere[3] {
		t.Fatalf("CA-567: ↓ en la ultima elegible no se mueve: %q", got)
	}
	m.Tecla("k", ttAhora)
	if got := ttMC(t, m).ttMarcada(); got != quiere[0] {
		t.Fatalf("CA-567: k vuelve a la anterior elegible: %q", got)
	}
	m.Tecla(tablero.TeclaArriba, ttAhora)
	m.Tecla("j", ttAhora)
	ttSinOrdenes(t, "CA-567", "enter sobre Aprobar spec", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Modal() != tablero.ModalFirma {
		t.Fatalf("CA-567: enter abre el modal de la elegida (Firma), es %q", m.Modal())
	}
	// la tecla de una accion desde Acciones
	m = ttModelo(ttAncho(), ttTablero(ttInfraAPI(false)))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	ttSinOrdenes(t, "CA-567", "s (deshabilitada) en Acciones", m.Tecla("s", ttAhora))
	ttDiceMensaje(t, "CA-567", "s deshabilitada", m, false, ttWhyGuardar)
	if m.Modal() != tablero.ModalAcciones {
		t.Fatalf("CA-567: con el why, Acciones sigue abierto: %q", m.Modal())
	}
	m.Tecla("w", ttAhora)
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-567: w desde Acciones abre Pedir: %q", m.Modal())
	}
	// u desde Acciones ejecuta hoom usar
	m = ttModelo(ttAncho(), ttTablero(ttInfraAPI(false)))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	l := ttUnaLanzar(t, "CA-567", "u en Acciones", m.Tecla("u", ttAhora))
	ttMismosArgs(t, "CA-567", "u en Acciones", l.Args, []string{"usar", "infra-api"})
	// en modo experto la experta aparece, gris y sin verbo
	m = ttModelo(ttAncho(), ttTablero(ttInfraAPI(false)))
	m.Tecla("x", ttAhora)
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	if !ttMC(t, m).ttTieneLinea("  Descartar cambios  sin verbo en la terminal") {
		t.Fatalf("CA-567: en modo experto la experta aparece:\n%s", ttPantalla(m))
	}
	// con color, las grises en gris
	cfg := ttAncho()
	cfg.Color = true
	g := ttModelo(cfg, ttTablero(ttInfraAPI(false)))
	g.Tecla(tablero.TeclaEspacio, ttAhora)
	for _, s := range []string{"s Guardar  hoom guardar", "  Volver a lanzar  sin verbo"} {
		for _, x := range g.Pantalla() {
			if strings.Contains(ttSinSGR(x), s) && ttEstiloDe(x, strings.TrimSpace(s)) != "\x1b[90m" {
				t.Fatalf("CA-567: %q va en gris: %q", s, x)
			}
		}
	}
}

// CA-567, CA-526: un modal de acciones sin nada elegible: j, k y enter no
// hacen nada y ninguna linea lleva la marca; sin acciones dice "no hay
// acciones en esta tarjeta".
func TestCA567_CA526_SinNadaElegible(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar), ttRelanzar())))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	antes := ttPantalla(m)
	for _, k := range []tablero.Tecla{"j", "k", tablero.TeclaEnter} {
		ttSinOrdenes(t, "CA-567", string(k)+" sin nada elegible", m.Tecla(k, ttAhora))
	}
	if ttPantalla(m) != antes || m.Modal() != tablero.ModalAcciones || ttMC(t, m).ttMarcada() != "" {
		t.Fatalf("CA-567: sin nada elegible, j, k y enter no hacen nada y no hay marca:\n%s", ttPantalla(m))
	}
	v := ttModelo(ttAncho(), ttTablero(ttTarjetaCon()))
	v.Tecla(tablero.TeclaEspacio, ttAhora)
	if !ttMC(t, v).ttTieneLinea("no hay acciones en esta tarjeta") {
		t.Fatalf("CA-567: sin acciones: 'no hay acciones en esta tarjeta':\n%s", ttPantalla(v))
	}
}

// CA-567, CA-517: lo que muestran Acciones y Pedir (la etiqueta, el why) es
// de la tarjeta, aunque diga otra cosa que boardcmd.
func TestCA567_CA517_LoQueMuestraEsDeLaTarjeta(t *testing.T) {
	a := ttAccionWriter()
	a.Label = "Etiqueta inventada por la tarjeta"
	g := ttAccionGuardar(false, "un why inventado por la tarjeta")
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(a, g)))
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	mc := ttMC(t, m)
	if !mc.ttTieneLinea("w Etiqueta inventada por la tarjeta  hoom pedir writer tarjeta") ||
		!mc.ttTieneLinea("s Guardar  hoom guardar tarjeta  (un why inventado por la tarjeta)") {
		t.Fatalf("CA-517: Acciones muestra la etiqueta y el why de la tarjeta:\n%s", ttPantalla(m))
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("w", ttAhora)
	mc = ttMC(t, m)
	if !strings.HasPrefix(mc.Arriba, "# Etiqueta inventada por la tarjeta · tarjeta ") || !strings.Contains(strings.Join(mc.Textos, "\n"), "[ Etiqueta inventada por la tarjeta ]") {
		t.Fatalf("CA-517: el titulo y el boton de Pedir llevan la etiqueta de la accion: %q\n%s", mc.Arriba, ttPantalla(m))
	}
}

// ---------------------------------------------------------------- CA-568: Pedir

// CA-568, CA-527, CA-528: el modal Pedir trae los valores de la accion, el
// foco en el Pedido; enter en el Pedido lleva el foco al boton (sin lanzar);
// enter en el boton lanza el verbo con -m y el pedido recortado, sin flags,
// cierra el modal, dice "lanzando hoom pedir writer <slug>" y suma la
// entrada al Registro con el mismo comando que mostro. Re-expresa CA-527.
func TestCA568_CA527_CA528_LanzarSinCambios(t *testing.T) {
	m := ttPedir(t, "w", ttAccionWriter())
	mc := ttMC(t, m)
	if m.Lugar() != tablero.LugarPedirPedido || ttFoco(mc) != "Pedido" {
		t.Fatalf("CA-568: el foco empieza en el Pedido: %q, %q", m.Lugar(), ttFoco(mc))
	}
	for et, v := range map[string]string{"Provider": "< claude >", "Modelo": "< opus >", "Esfuerzo": "< high >",
		"Tope": "< 2.75 USD >", "Tiempo limite": "< 45m >"} {
		if got := mc.ttValor(et); got != v {
			t.Fatalf("CA-568: %s muestra %q, muestra %q\n%s", et, v, got, strings.Join(mc.Textos, "\n"))
		}
	}
	for et, o := range map[string]string{"Provider": "de la tarjeta", "Modelo": "de hoom.yaml", "Esfuerzo": "de hoom.yaml",
		"Tope": "de la tarjeta", "Tiempo limite": "de hoom.yaml"} {
		if got := mc.ttOrigen(et); got != o {
			t.Fatalf("CA-568: el origen de %s es %q, es %q", et, o, got)
		}
	}
	comando := "hoom pedir writer tarjeta -m " + tarjetacmd.CitarPedido(ttPedidoWriter, false)
	if got := mc.ttComando(); got != comando {
		t.Fatalf("CA-568: 'Se va a ejecutar:' sin cambios no lleva flags:\n got %q\nwant %q", got, comando)
	}
	if !mc.ttTieneLinea("Se va a ejecutar:") {
		t.Fatalf("CA-568: el modal dice 'Se va a ejecutar:' en su linea:\n%s", strings.Join(mc.Textos, "\n"))
	}
	if b := strings.Join(mc.Textos, "\n"); !strings.Contains(b, "[ Pedir al writer ]     Cancelar") {
		t.Fatalf("CA-568: los botones son '[ Pedir al writer ]' (elegido) y 'Cancelar':\n%s", b)
	}
	ttSinOrdenes(t, "CA-568", "enter en el Pedido", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Lugar() != tablero.LugarPedirBotones || ttFoco(ttMC(t, m)) != "botones" || m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-568: enter en el Pedido lleva el foco a los botones, sin lanzar: %q", m.Lugar())
	}
	l := ttUnaLanzar(t, "CA-528", "enter en el boton", m.Tecla(tablero.TeclaEnter, ttAhora.Add(time.Minute)))
	ttMismosArgs(t, "CA-528", "w sin cambios", l.Args, []string{"pedir", "writer", "tarjeta", "-m", strings.TrimSpace(ttPedidoWriter)})
	if l.Rol != "writer" || l.Slug != "tarjeta" || l.ID != 1 {
		t.Fatalf("CA-528: el Lanzar lleva Rol, Slug e ID: %#v", l)
	}
	if m.Modal() != tablero.SinModal || m.Corriendo() != 1 {
		t.Fatalf("CA-528: lanzar cierra el modal y el trabajo corre: %q, %d", m.Modal(), m.Corriendo())
	}
	ttDiceMensaje(t, "CA-528", "tras lanzar", m, false, "lanzando hoom pedir writer tarjeta")
	r := m.Registro()
	if len(r) != 1 || r[0].Texto != "Pedir al writer · tarjeta · lanzando..." || r[0].Comando != comando || !r[0].Hora.Equal(ttAhora.Add(time.Minute)) {
		t.Fatalf("CA-563: la entrada del Registro: '<etiqueta> · <slug> · lanzando...' con el comando que mostro: %#v", r)
	}
}

// CA-568, CA-527: tab y shift-tab pasan por Pedido, Provider, Modelo,
// Esfuerzo, Tope, Tiempo limite y los botones, en ciclo.
func TestCA568_CA527_FocoEnCiclo(t *testing.T) {
	m := ttPedir(t, "w", ttAccionWriter())
	orden := []string{"Provider", "Modelo", "Esfuerzo", "Tope", "Tiempo limite", "botones", "Pedido"}
	for i, q := range orden {
		ttSinOrdenes(t, "CA-568", "tab", m.Tecla(tablero.TeclaTab, ttAhora))
		if f := ttFoco(ttMC(t, m)); f != q {
			t.Fatalf("CA-568: el tab numero %d deja el foco en %q, esta en %q", i+1, q, f)
		}
	}
	for i := len(orden) - 2; i >= -1; i-- {
		m.Tecla(tablero.TeclaShiftTab, ttAhora)
		q := orden[(i+len(orden))%len(orden)]
		if f := ttFoco(ttMC(t, m)); f != q {
			t.Fatalf("CA-568: shift-tab deja el foco en %q, esta en %q", q, f)
		}
	}
	// los lugares del modal
	m = ttPedir(t, "w", ttAccionWriter())
	m.Tecla(tablero.TeclaTab, ttAhora)
	if m.Lugar() != tablero.LugarPedirSelector {
		t.Fatalf("CA-568: en un selector el Lugar es LugarPedirSelector: %q", m.Lugar())
	}
}

// CA-568, CA-527: en el Pedido, ← y → mueven el cursor, backspace borra
// antes del cursor, ctrl-u lo vacia; el cursor se ve con el signo del
// cursor; un pedido vacio (o de espacios) no lanza: el foco vuelve al
// Pedido, "Pedido" se marca en rojo y la linea de mensajes dice "el pedido
// esta vacio" en rojo.
func TestCA568_CA527_EditarElPedidoYVacio(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola"
	m := ttPedir(t, "w", a)
	if !strings.Contains(strings.Join(ttMC(t, m).Textos, "\n"), "| hola_") {
		t.Fatalf("CA-568: el Pedido muestra el texto con el cursor al final:\n%s", ttPantalla(m))
	}
	ttTeclas(m, tablero.TeclaIzq, tablero.TeclaIzq)
	ttEscribir(m, "Z")
	ttTeclas(m, tablero.TeclaDer, tablero.TeclaBorrar)
	ttEscribir(m, "X")
	m.Tecla(tablero.TeclaEnter, ttAhora)
	l := ttUnaLanzar(t, "CA-568", "enter en el boton", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-568", "pedido editado", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "hoZXa"})

	for _, vacio := range []string{"", "   "} {
		cfg := ttAncho()
		cfg.Color = true
		m = ttAbrirModal(t, cfg, "w", tablero.ModalPedir, a)
		ttSinOrdenes(t, "CA-568", "ctrl-u", m.Tecla(tablero.TeclaCtrlU, ttAhora))
		ttEscribir(m, vacio)
		m.Tecla(tablero.TeclaEnter, ttAhora)
		ttSinOrdenes(t, "CA-568", "enter en el boton con el pedido vacio", m.Tecla(tablero.TeclaEnter, ttAhora))
		if m.Modal() != tablero.ModalPedir || m.Lugar() != tablero.LugarPedirPedido || m.Corriendo() != 0 {
			t.Fatalf("CA-568: un pedido vacio no lanza y el foco vuelve al Pedido: %q %q", m.Modal(), m.Lugar())
		}
		ttDiceMensaje(t, "CA-568", "pedido vacio", m, true, "el pedido esta vacio")
		var pedido string
		for _, x := range m.Pantalla() {
			if strings.Contains(ttSinSGR(x), "> Pedido") {
				pedido = x
			}
		}
		if !ttConColor(ttRojo, pedido, "Pedido") {
			t.Fatalf("CA-568: 'Pedido' se marca en rojo: %q", pedido)
		}
	}
}

// CA-568, CA-527: ctrl-e (desde cualquier campo: decision de la persona)
// ordena EditarPedido con el pedido; PedidoEditado lo reemplaza con el cursor
// al final; un editor con error deja el pedido y la linea de mensajes dice
// "el editor salio con error" en rojo; esc cierra sin lanzar.
func TestCA568_CA527_CtrlEYEsc(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "pedido original"
	for _, campo := range []int{0, 1, 3, 6} { // Pedido, Provider, Esfuerzo, botones
		m := ttPedir(t, "w", a)
		for i := 0; i < campo; i++ {
			m.Tecla(tablero.TeclaTab, ttAhora)
		}
		os := m.Tecla(tablero.TeclaCtrlE, ttAhora)
		if len(os) != 1 {
			t.Fatalf("CA-568: ctrl-e (tras %d tabs) da una orden EditarPedido, dio %#v", campo, os)
		}
		if ep, ok := os[0].(tablero.EditarPedido); !ok || ep.Texto != "pedido original" {
			t.Fatalf("CA-568: ctrl-e abre el pedido en el editor: %#v", os[0])
		}
	}
	m := ttPedir(t, "w", a)
	m.Tecla(tablero.TeclaCtrlE, ttAhora)
	m.PedidoEditado("", errors.New("exit status 1"))
	ttDiceMensaje(t, "CA-568", "el editor con error", m, false, "el editor salio con error")
	if got := ttMC(t, m).ttComando(); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("pedido original", false) {
		t.Fatalf("CA-568: con el editor en error el pedido queda como estaba: %q", got)
	}
	m.PedidoEditado("pedido del editor\n", nil)
	if got := ttMC(t, m).ttComando(); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("pedido del editor", false) {
		t.Fatalf("CA-568: lo que dio el editor es el pedido: %q", got)
	}
	ttEscribir(m, "!") // el cursor queda al final
	m.Tecla(tablero.TeclaEnter, ttAhora)
	l := ttUnaLanzar(t, "CA-568", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-568", "pedido del editor", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "pedido del editor\n!"})
	// esc y Cancelar
	m = ttPedir(t, "w", a)
	ttSinOrdenes(t, "CA-568", "esc", m.Tecla(tablero.TeclaEsc, ttAhora))
	if m.Modal() != tablero.SinModal || m.Corriendo() != 0 || len(m.Registro()) != 0 {
		t.Fatalf("CA-568: esc cierra sin lanzar: %q", m.Modal())
	}
	m = ttPedir(t, "w", a)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora)
	if b := strings.Join(ttMC(t, m).Textos, "\n"); !strings.Contains(b, "  Pedir al writer     [ Cancelar ]") {
		t.Fatalf("CA-565: → elige Cancelar: %q", b)
	}
	ttSinOrdenes(t, "CA-565", "enter en Cancelar", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Modal() != tablero.SinModal || m.Corriendo() != 0 {
		t.Fatalf("CA-565: enter en Cancelar cierra sin lanzar: %q", m.Modal())
	}
}

// CA-568, CA-527: "Se va a ejecutar:" y los Args llevan solo los flags cuyo
// valor difiere de lo propuesto, en el orden del contrato, y cambian con
// cada cambio; ← y → cambian la opcion de un selector en su lugar, en ciclo,
// saltando las grises y otro…; la opcion cambiada dice "elegido".
func TestCA568_CA527_SoloLosFlagsCambiados(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola mundo"
	m := ttPedir(t, "w", a)
	base := "hoom pedir writer tarjeta -m " + tarjetacmd.CitarPedido("hola mundo", false)
	cmd := func(quiere string) {
		t.Helper()
		if got := ttMC(t, m).ttComando(); got != quiere {
			t.Fatalf("CA-568: 'Se va a ejecutar' es\n %q\nse esperaba\n %q", got, quiere)
		}
	}
	cmd(base)
	m.Tecla(tablero.TeclaTab, ttAhora) // Provider
	m.Tecla(tablero.TeclaTab, ttAhora) // Modelo
	m.Tecla(tablero.TeclaDer, ttAhora)
	mc := ttMC(t, m)
	if mc.ttValor("Modelo") != "< sonnet >" || mc.ttOrigen("Modelo") != "elegido" {
		t.Fatalf("CA-568: → en Modelo elige la siguiente (sonnet) y su origen dice 'elegido': %q %q", mc.ttValor("Modelo"), mc.ttOrigen("Modelo"))
	}
	cmd(base + " --model sonnet")
	m.Tecla(tablero.TeclaIzq, ttAhora)
	if mc := ttMC(t, m); mc.ttValor("Modelo") != "< opus >" || mc.ttOrigen("Modelo") != "de hoom.yaml" {
		t.Fatalf("CA-568: ← vuelve a lo propuesto, con su origen: %q %q", mc.ttValor("Modelo"), mc.ttOrigen("Modelo"))
	}
	cmd(base)
	m.Tecla(tablero.TeclaIzq, ttAhora) // fable
	m.Tecla(tablero.TeclaIzq, ttAhora) // salta "por defecto del provider" (gris: hoom.yaml fija opus) y otro…: sonnet[1m]
	if v := ttMC(t, m).ttValor("Modelo"); v != "< sonnet[1m] >" {
		t.Fatalf("CA-568: ← da la vuelta saltando las grises y otro…: %q", v)
	}
	m.Tecla(tablero.TeclaDer, ttAhora) // de vuelta a fable, saltando otro… y la gris
	if v := ttMC(t, m).ttValor("Modelo"); v != "< fable >" {
		t.Fatalf("CA-568: → da la vuelta saltando otro… y la gris: %q", v)
	}
	cmd(base + " --model fable")
	m.Tecla(tablero.TeclaTab, ttAhora) // Esfuerzo
	m.Tecla(tablero.TeclaIzq, ttAhora) // medium
	cmd(base + " --model fable --effort medium")
	m.Tecla(tablero.TeclaTab, ttAhora) // Tope
	m.Tecla(tablero.TeclaDer, ttAhora) // 1 USD
	m.Tecla(tablero.TeclaDer, ttAhora) // 2 USD
	m.Tecla(tablero.TeclaDer, ttAhora) // 5 USD
	cmd(base + " --model fable --effort medium --budget-usd 5")
	m.Tecla(tablero.TeclaTab, ttAhora) // Tiempo limite
	m.Tecla(tablero.TeclaDer, ttAhora) // 15m
	cmd(base + " --model fable --effort medium --budget-usd 5 --timeout 15m")
	m.Tecla(tablero.TeclaShiftTab, ttAhora)
	m.Tecla(tablero.TeclaShiftTab, ttAhora)
	m.Tecla(tablero.TeclaShiftTab, ttAhora)
	m.Tecla(tablero.TeclaShiftTab, ttAhora) // Provider
	m.Tecla(tablero.TeclaDer, ttAhora)      // codex
	// codex no acepta tope: Tope gris no manda nada; el modelo y el
	// esfuerzo elegidos siguen (fable no esta en la lista de codex)
	cmd(base + " --provider codex --model fable --effort medium --timeout 15m")
	// shift-tab al Pedido, enter al boton, enter lanza
	l := ttUnaLanzar(t, "CA-568", "enter en el boton", ttTeclas(m, tablero.TeclaShiftTab, tablero.TeclaEnter, tablero.TeclaEnter))
	ttMismosArgs(t, "CA-568", "con flags", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "hola mundo",
		"--provider", "codex", "--model", "fable", "--effort", "medium", "--timeout", "15m"})
}

// CA-568: enter o espacio en un selector abren su lista encima del modal,
// una linea por opcion con su nombre y su linea (las grises con su why);
// la elegida empieza marcada; ↑ ↓ se mueven por todas, enter elige y cierra,
// esc cierra sin cambiar, enter sobre una gris no hace nada.
func TestCA568_LaListaDeOpciones(t *testing.T) {
	for _, abrir := range []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaEspacio} {
		m := ttPedir(t, "w", ttAccionWriter())
		m.Tecla(tablero.TeclaTab, ttAhora)
		m.Tecla(tablero.TeclaTab, ttAhora)
		m.Tecla(tablero.TeclaTab, ttAhora) // Esfuerzo
		ttSinOrdenes(t, "CA-568", "abrir la lista", m.Tecla(abrir, ttAhora))
		if m.Lugar() != tablero.LugarPedirLista {
			t.Fatalf("CA-568: %q en un selector abre su lista: %q", abrir, m.Lugar())
		}
		ops, _ := ttLista(t, ttMC(t, m))
		op := func(mk, nombre, l string) string {
			return strings.TrimRight(fmt.Sprintf(" %s %-24s  %s", mk, nombre, l), " ")
		}
		quiere := []string{
			op(" ", "por defecto del provider", "hoom.yaml fija high para el writer"),
			op(" ", "low", "rapido, razona poco"),
			op(" ", "medium", "equilibra velocidad y profundidad"),
			op(">", "high", "mas profundidad para problemas complejos"),
			op(" ", "xhigh", "profundidad extra"),
			op(" ", "max", "la maxima profundidad, el mas lento"),
			op(" ", "otro...", ""),
		}
		if strings.Join(ops, "\n") != strings.Join(quiere, "\n") {
			t.Fatalf("CA-568: la lista de Esfuerzo:\n got %q\nwant %q", ops, quiere)
		}
		m.Tecla(tablero.TeclaArriba, ttAhora)
		m.Tecla(tablero.TeclaArriba, ttAhora)
		m.Tecla(tablero.TeclaArriba, ttAhora) // la gris
		if o, _ := ttLista(t, ttMC(t, m)); ttOpcionElegida(o) != "por" {
			t.Fatalf("CA-568: ↑ se mueve por todas, las grises tambien: %q", o)
		}
		ttSinOrdenes(t, "CA-568", "enter sobre la gris", m.Tecla(tablero.TeclaEnter, ttAhora))
		if m.Lugar() != tablero.LugarPedirLista {
			t.Fatalf("CA-568: enter sobre una gris no hace nada: %q", m.Lugar())
		}
		m.Tecla(tablero.TeclaAbajo, ttAhora) // low
		m.Tecla(tablero.TeclaEnter, ttAhora)
		mc := ttMC(t, m)
		if m.Lugar() != tablero.LugarPedirSelector || mc.ttValor("Esfuerzo") != "< low >" {
			t.Fatalf("CA-568: enter elige y cierra la lista: %q %q", m.Lugar(), mc.ttValor("Esfuerzo"))
		}
		if got := ttAyudaSel(mc); got != "low: rapido, razona poco" {
			t.Fatalf("CA-568: la linea gris de abajo dice la linea de la opcion elegida: %q", got)
		}
		m.Tecla(tablero.TeclaEnter, ttAhora)
		m.Tecla(tablero.TeclaAbajo, ttAhora)
		m.Tecla(tablero.TeclaEsc, ttAhora)
		if mc := ttMC(t, m); mc.ttValor("Esfuerzo") != "< low >" || m.Lugar() != tablero.LugarPedirSelector {
			t.Fatalf("CA-568: esc cierra la lista sin cambiar: %q", mc.ttValor("Esfuerzo"))
		}
	}
}

// CA-568, caso limite: una lista de opciones mas larga que el modal se
// desplaza y su borde dice "<n> de <m>".
func TestCA568_ListaMasLargaQueElModal(t *testing.T) {
	cfg := ttConfig(80, 24, false)
	a := ttAccionWriter()
	cfg.Modelos = map[string][]modelos.Propio{"claude": {}}
	for i := 0; i < 15; i++ {
		cfg.Modelos["claude"] = append(cfg.Modelos["claude"], modelos.Propio{Nombre: fmt.Sprintf("propio-%02d", i)})
	}
	m := ttModelo(cfg, ttTablero(ttTarjetaCon(a)))
	m.Tecla("w", ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	mc := ttModal(t, "CA-568", m, 80)
	ops, abajo := ttLista(t, mc)
	total := 1 + 8 + 15 + 1
	if len(ops) >= total || !strings.Contains(abajo, fmt.Sprintf("3 de %d", total)) {
		t.Fatalf("CA-568: la lista de %d opciones no entra: se desplaza y su borde dice '3 de %d' (opus elegido): %d lineas, %q", total, total, len(ops), abajo)
	}
	for i := 0; i < 30; i++ {
		m.Tecla(tablero.TeclaAbajo, ttAhora)
	}
	ops, abajo = ttLista(t, ttModal(t, "CA-568", m, 80))
	if !strings.Contains(abajo, fmt.Sprintf("%d de %d", total, total)) || !strings.HasPrefix(ops[len(ops)-1], " > otro...") {
		t.Fatalf("CA-568: abajo de todo la ultima (otro…) a la vista: %q %q", ops, abajo)
	}
	ttForma(t, "CA-568", m, 80, 24, false)
}

// CA-568, CA-527: otro… convierte el campo en texto (vacio); enter con texto
// lo hace el valor elegido ("elegido", y queda entre las opciones antes de
// otro…); esc, o enter con el texto vacio, vuelve a la opcion de antes; la
// TUI no valida lo escrito (el Tope "abc" y el Tiempo limite "5s" se mandan
// tal cual: decision del orquestador, el mensaje de la v1 "el presupuesto no
// es un numero" ya no existe).
func TestCA568_CA527_OtroAceptaTexto(t *testing.T) {
	irAOtro := func(m *tablero.Modelo) {
		t.Helper()
		m.Tecla(tablero.TeclaEnter, ttAhora)
		for i := 0; i < 40; i++ {
			m.Tecla(tablero.TeclaAbajo, ttAhora)
		}
		m.Tecla(tablero.TeclaEnter, ttAhora)
		if m.Lugar() != tablero.LugarPedirTexto {
			t.Fatalf("CA-568: enter sobre otro… convierte el campo en texto: %q", m.Lugar())
		}
	}
	a := ttAccionWriter()
	a.Pedido = "hola"
	m := ttPedir(t, "w", a)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora) // Modelo
	irAOtro(m)
	if v := ttMC(t, m).ttCampo("Modelo"); v != "Modelo         _" {
		t.Fatalf("CA-568: el texto empieza vacio, con el cursor y sin flechas: %q", v)
	}
	ttEscribir(m, "gpt-x q")
	if m.Lugar() != tablero.LugarPedirTexto || !strings.HasPrefix(ttMC(t, m).ttCampo("Modelo"), "Modelo         gpt-x q_") {
		t.Fatalf("CA-568: en el texto las letras se escriben (la q tambien): %q", ttMC(t, m).ttCampo("Modelo"))
	}
	m.Tecla(tablero.TeclaBorrar, ttAhora)
	m.Tecla(tablero.TeclaBorrar, ttAhora)
	ttSinOrdenes(t, "CA-568", "enter con texto", m.Tecla(tablero.TeclaEnter, ttAhora))
	mc := ttMC(t, m)
	if mc.ttValor("Modelo") != "< gpt-x >" || mc.ttOrigen("Modelo") != "elegido" || m.Lugar() != tablero.LugarPedirSelector {
		t.Fatalf("CA-568: enter confirma el texto como el valor elegido: %q %q", mc.ttValor("Modelo"), mc.ttOrigen("Modelo"))
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	ops, _ := ttLista(t, ttMC(t, m))
	if len(ops) < 2 || !strings.HasPrefix(ops[len(ops)-2], " > gpt-x") || !strings.HasPrefix(ops[len(ops)-1], "   otro...") {
		t.Fatalf("CA-568: el texto queda entre las opciones, antes de otro…: %q", ops)
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	// esc vuelve a la de antes
	m.Tecla(tablero.TeclaTab, ttAhora) // Esfuerzo
	irAOtro(m)
	ttEscribir(m, "algo")
	m.Tecla(tablero.TeclaEsc, ttAhora)
	if v := ttMC(t, m).ttValor("Esfuerzo"); v != "< high >" || m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-568: esc en el texto vuelve a la opcion de antes (y no cierra el modal): %q %q", v, m.Modal())
	}
	// enter con el texto vacio, igual
	irAOtro(m)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if v := ttMC(t, m).ttValor("Esfuerzo"); v != "< high >" {
		t.Fatalf("CA-568: enter con el texto vacio vuelve a la opcion de antes: %q", v)
	}
	// Tope y Tiempo limite sin validar
	m.Tecla(tablero.TeclaTab, ttAhora) // Tope
	irAOtro(m)
	ttEscribir(m, "abc")
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if v := ttMC(t, m).ttValor("Tope"); v != "< abc USD >" {
		t.Fatalf("CA-571: el texto de otro… en Tope es el numero de USD: '< abc USD >': %q", v)
	}
	m.Tecla(tablero.TeclaTab, ttAhora) // Tiempo limite
	irAOtro(m)
	ttEscribir(m, "5s")
	m.Tecla(tablero.TeclaEnter, ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora) // los botones
	l := ttUnaLanzar(t, "CA-568", "enter en el boton", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-568", "lo escrito, sin validar", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "hola",
		"--model", "gpt-x", "--budget-usd", "abc", "--timeout", "5s"})
}

// CA-568, caso limite: un pedido con saltos de linea: en "Se va a ejecutar"
// los saltos se ven como espacios (CitarPedido); el argumento del verbo los
// conserva.
func TestCA568_PedidoConSaltos(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "uno\ndos\r\ntres"
	m := ttPedir(t, "w", a)
	if got, quiere := ttMC(t, m).ttComando(), "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido(a.Pedido, false); got != quiere ||
		strings.Contains(got, "\n") {
		t.Fatalf("CA-568: los saltos se ven como espacios: %q, se esperaba %q", got, quiere)
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	l := ttUnaLanzar(t, "CA-568", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-568", "los saltos en el argumento", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "uno\ndos\r\ntres"})
	if r := m.Registro(); len(r) != 1 || strings.ContainsAny(r[0].Comando, "\n\r") {
		t.Fatalf("CA-563: en el Registro los saltos se ven como espacios: %#v", r)
	}
}

// CA-568: "Se va a ejecutar:" ocupa hasta dos lineas; si no entra, el
// argumento de -m se acorta a su comienzo y "…" ("..." en ASCII: los signos
// del corte de palabras propias de la TUI), y la segunda linea empieza con
// dos espacios.
func TestCA568_ElComandoEnDosLineas(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = strings.Repeat("palabra ", 30)
	m := ttPedir(t, "w", a)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora) // codex: --provider codex
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora) // otro modelo
	mc := ttMC(t, m)
	_, i, _ := mc.ttLineaQueEmpieza("Se va a ejecutar:")
	l1, l2, l3 := mc.Textos[i+1], mc.Textos[i+2], mc.Textos[i+3]
	if !strings.HasPrefix(l1, "hoom pedir writer tarjeta -m") || !strings.HasPrefix(l2, "  ") || l3 != "" {
		t.Fatalf("CA-568: el comando ocupa dos lineas, la segunda con dos espacios, y sigue una vacia:\n%q\n%q\n%q", l1, l2, l3)
	}
	if ttCeldas(l1) > 66 || ttCeldas(l2) > 66 || !strings.Contains(l1+l2, "--provider codex") || !strings.Contains(l1+l2, "--model") {
		t.Fatalf("CA-568: las dos lineas entran en 66 celdas y llevan los flags: %q %q", l1, l2)
	}
	if !strings.Contains(l1+l2, "...") && !strings.Contains(l1+l2, "~") {
		t.Fatalf("CA-568: el -m acortado termina en un signo de corte ASCII: %q %q", l1, l2)
	}
	// el Registro lleva el comando en una linea, con el -m de CitarPedido
	// (no acortado de mas)
	for i := 0; i < 8 && ttFoco(ttMC(t, m)) != "botones"; i++ {
		m.Tecla(tablero.TeclaTab, ttAhora)
	}
	ttUnaLanzar(t, "CA-568", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	if r := m.Registro(); len(r) != 1 || !strings.Contains(r[0].Comando, tarjetacmd.CitarPedido(a.Pedido, false)) {
		t.Fatalf("CA-563: el Registro lleva el -m de CitarPedido sin acortar mas: %#v", r)
	}
}

// CA-568, CA-527: el reviewer no tiene Pedido; el foco empieza en el primer
// selector que no es gris (Provider); su Tiempo limite va en gris con "de
// review.timeout", tab lo saltea, y nunca se manda -m ni --timeout.
func TestCA568_CA527_ElReviewerNoTienePedido(t *testing.T) {
	a := ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", "un pedido que no se usa")
	m := ttPedir(t, "r", a)
	mc := ttMC(t, m)
	if mc.ttCampo("Pedido") != "" || strings.Contains(strings.Join(mc.Textos, "\n"), "| un pedido") {
		t.Fatalf("CA-568: el reviewer no tiene Pedido ni su cuadro:\n%s", strings.Join(mc.Textos, "\n"))
	}
	if ttFoco(mc) != "Provider" || m.Lugar() != tablero.LugarPedirSelector {
		t.Fatalf("CA-568: en el reviewer el foco empieza en Provider: %q", ttFoco(mc))
	}
	if mc.ttValor("Tiempo limite") != "< 45m >" || ttWhyDe(mc, "Tiempo limite") != "de review.timeout" {
		t.Fatalf("CA-571: el Tiempo limite del reviewer va gris con 'de review.timeout': %q %q", mc.ttValor("Tiempo limite"), ttWhyDe(mc, "Tiempo limite"))
	}
	if got := mc.ttComando(); got != "hoom pedir reviewer tarjeta" {
		t.Fatalf("CA-568: el reviewer no lleva -m ni --timeout: %q", got)
	}
	for _, q := range []string{"Modelo", "Esfuerzo", "Tope", "botones", "Provider"} {
		m.Tecla(tablero.TeclaTab, ttAhora)
		if f := ttFoco(ttMC(t, m)); f != q {
			t.Fatalf("CA-568: en el reviewer tab deja el foco en %q (el Tiempo limite gris se saltea): %q", q, f)
		}
	}
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora) // Modelo: sonnet
	m.Tecla(tablero.TeclaShiftTab, ttAhora)
	m.Tecla(tablero.TeclaShiftTab, ttAhora) // los botones
	l := ttUnaLanzar(t, "CA-568", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-568", "reviewer", l.Args, []string{"pedir", "reviewer", "tarjeta", "--model", "sonnet"})
	if l.Rol != "reviewer" {
		t.Fatalf("CA-528: el Lanzar del reviewer lleva su rol: %#v", l)
	}
}

// CA-568, CA-527: reanudar no cambia el provider: su selector va gris, sin
// flechas, con "la sesion es de <p>"; ← y → no lo cambian; ejecuta hoom
// reanudar <slug> -m <pedido> y nunca --provider.
func TestCA568_CA527_Reanudar(t *testing.T) {
	a := ttPagosCaidos().Actions[0]
	a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), ttProvCodex(false)}
	m := ttPedir(t, "R", a)
	mc := ttMC(t, m)
	if mc.ttValor("Provider") != "claude" || ttWhyDe(mc, "Provider") != "la sesion es de claude" {
		t.Fatalf("CA-568: en reanudar el Provider va sin flechas y dice 'la sesion es de claude': %q %q", mc.ttValor("Provider"), ttWhyDe(mc, "Provider"))
	}
	if !strings.HasPrefix(mc.Arriba, "# Reanudar · tarjeta ") {
		t.Fatalf("CA-568: el titulo es '<etiqueta> · <slug>': %q", mc.Arriba)
	}
	m.Tecla(tablero.TeclaTab, ttAhora)
	if f := ttFoco(ttMC(t, m)); f != "Modelo" {
		t.Fatalf("CA-568: tab saltea el Provider gris de reanudar: %q", f)
	}
	ttTeclas(m, tablero.TeclaShiftTab, tablero.TeclaIzq, tablero.TeclaDer) // al Pedido: el cursor va y vuelve
	quiere := "hoom reanudar tarjeta -m " + tarjetacmd.CitarPedido(a.Pedido, false)
	if got := ttMC(t, m).ttComando(); got != quiere {
		t.Fatalf("CA-568: en reanudar el provider no cambia y no hay --provider: %q", got)
	}
	m.Tecla(tablero.TeclaEnter, ttAhora) // al boton
	l := ttUnaLanzar(t, "CA-568", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-568", "reanudar", l.Args, []string{"reanudar", "tarjeta", "-m", a.Pedido})
	if l.Rol != "writer" {
		t.Fatalf("CA-528: reanudar lanza el rol de la accion: %#v", l)
	}
}

// CA-568, CA-527: en Windows el comando cita como los verbos de la capa 1
// ahi (tarjetacmd.CitarPedido y Citar con windows); fuera de Windows, como
// sh. Los Args van sin citar.
func TestCA568_CA527_CitaSegunElSistema(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "usa $HOME y 100%"
	for _, win := range []bool{false, true} {
		cfg := ttAncho()
		cfg.Windows = win
		m := ttAbrirModal(t, cfg, "w", tablero.ModalPedir, a)
		m.Tecla(tablero.TeclaTab, ttAhora)
		m.Tecla(tablero.TeclaTab, ttAhora)
		m.Tecla(tablero.TeclaTab, ttAhora) // Esfuerzo
		m.Tecla(tablero.TeclaEnter, ttAhora)
		for i := 0; i < 10; i++ {
			m.Tecla(tablero.TeclaAbajo, ttAhora)
		}
		m.Tecla(tablero.TeclaEnter, ttAhora) // otro…
		ttEscribir(m, "muy alto")
		m.Tecla(tablero.TeclaEnter, ttAhora)
		quiere := "hoom pedir writer tarjeta -m " + tarjetacmd.CitarPedido(a.Pedido, win) + " --effort " + tarjetacmd.Citar("muy alto", "<esfuerzo>", win)
		if got := ttMC(t, m).ttComando(); got != quiere {
			t.Fatalf("CA-568 (windows=%v): el comando cita como la capa 1:\n got %q\nwant %q", win, got, quiere)
		}
		ttTeclas(m, tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaTab)
		l := ttUnaLanzar(t, "CA-568", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, "CA-568", "valores sin citar", l.Args, []string{"pedir", "writer", "tarjeta", "-m", a.Pedido, "--effort", "muy alto"})
	}
}

// ---------------------------------------------------------------- CA-569: Modelo y Esfuerzo

// CA-569: Modelo ofrece "por defecto del provider", los modelos del catalogo
// del provider elegido, los models.<provider> de hoom.yaml (Config.Modelos,
// sin repetir un nombre del catalogo: su descripcion le cambia la linea) y
// otro…; con el model de la accion elegido al abrir, sumado antes de otro…
// con la linea "de hoom.yaml" cuando no esta en ninguna lista.
func TestCA569_LasOpcionesDeModelo(t *testing.T) {
	cfg := ttAncho()
	cfg.Modelos = map[string][]modelos.Propio{"claude": {{Nombre: "opus", Descripcion: "el de la casa"}, {Nombre: "mi-claude", Descripcion: "uno propio"},
		{Nombre: "sin-desc"}}, "codex": {{Nombre: "gpt-7-sol", Descripcion: "el nuevo de todos los dias"}}}
	a := ttAccionWriter()
	a.Origen.Model = valores.OrigenTarjeta
	m := ttModelo(cfg, ttTablero(ttTarjetaCon(a)))
	m.Tecla("w", ttAhora)
	ttTeclas(m, tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaEnter)
	ops, _ := ttLista(t, ttMC(t, m))
	cat := modelos.Modelos("claude", cfg.Modelos["claude"])
	largo := len("por defecto del provider")
	for _, o := range cat {
		if len(o.Nombre) > largo {
			largo = len(o.Nombre)
		}
	}
	linea := func(mk, nombre, l string) string {
		return strings.TrimRight(fmt.Sprintf(" %s %-*s  %s", mk, largo, nombre, l), " ")
	}
	quiere := []string{linea(" ", "por defecto del provider", "el que elige claude: su configuracion o tu cuenta")}
	for _, o := range cat {
		mk := " "
		if o.Nombre == "opus" {
			mk = ">"
		}
		quiere = append(quiere, linea(mk, o.Nombre, o.Linea))
	}
	quiere = append(quiere, linea(" ", "otro...", ""))
	if strings.Join(ops, "\n") != strings.Join(quiere, "\n") {
		t.Fatalf("CA-569: la lista de Modelo:\n got %q\nwant %q", ops, quiere)
	}
	if strings.Count(strings.Join(ops, "\n"), " opus ") != 1 || !strings.Contains(strings.Join(ops, "\n"), "el de la casa") {
		t.Fatalf("CA-569: opus de hoom.yaml no se repite y su descripcion es su linea: %q", ops)
	}
	// el model de la accion que no esta en ninguna lista
	b := ttAccionWriter()
	b.Model = "un-modelo-raro"
	m = ttModelo(cfg, ttTablero(ttTarjetaCon(b)))
	m.Tecla("w", ttAhora)
	ttTeclas(m, tablero.TeclaTab, tablero.TeclaTab)
	if v := ttMC(t, m).ttValor("Modelo"); v != "< un-modelo-raro >" {
		t.Fatalf("CA-569: el model de la accion esta elegido al abrir: %q", v)
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	ops, _ = ttLista(t, ttMC(t, m))
	if len(ops) < 2 || !strings.HasPrefix(ops[len(ops)-2], " > un-modelo-raro") || !strings.HasSuffix(ops[len(ops)-2], "  de hoom.yaml") ||
		!strings.HasPrefix(ops[len(ops)-1], "   otro...") {
		t.Fatalf("CA-569: un modelo que no esta en ninguna lista va elegido antes de otro…, con 'de hoom.yaml': %q", ops)
	}
	// sin model en la accion: "por defecto del provider" elegido
	c := ttAccionWriter()
	c.Model, c.Origen.Model = "", valores.OrigenProvider
	m = ttModelo(cfg, ttTablero(ttTarjetaCon(c)))
	m.Tecla("w", ttAhora)
	mc := ttMC(t, m)
	if mc.ttValor("Modelo") != "< por defecto del provider >" || mc.ttOrigen("Modelo") != "del provider" {
		t.Fatalf("CA-569: sin model, 'por defecto del provider', con origen 'del provider': %q %q", mc.ttValor("Modelo"), mc.ttOrigen("Modelo"))
	}
}

// CA-569: Esfuerzo ofrece "por defecto del provider", los esfuerzos del
// catalogo del provider (codex suma ultra) y otro….
func TestCA569_LasOpcionesDeEsfuerzo(t *testing.T) {
	a := ttAccionWriter()
	a.Providers = []boardcmd.ProviderOption{ttProvCodex(true), ttProvClaude(false)}
	a.Model, a.Origen.Model, a.Origen.Effort = "", valores.OrigenProvider, valores.OrigenTarjeta
	m := ttPedir(t, "w", a)
	ttTeclas(m, tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaEnter)
	ops, _ := ttLista(t, ttMC(t, m))
	var nombres []string
	for _, o := range ops {
		nombres = append(nombres, strings.Fields(o[3:])[0])
	}
	if strings.Join(nombres, ",") != "por,low,medium,high,xhigh,max,ultra,otro..." {
		t.Fatalf("CA-569: las opciones de Esfuerzo de codex: %q", ops)
	}
	cat, _ := modelos.De("codex")
	if last := ops[6]; !strings.HasSuffix(last, cat.Esfuerzos[5].Linea) {
		t.Fatalf("CA-569: cada esfuerzo con su linea del catalogo: %q", last)
	}
}

// CA-569: un provider sin Model en sus Capabilities (opencode, gemini):
// Modelo y Esfuerzo van en gris, "‹ del provider ›", con "hoom no le pasa el
// modelo a <p>" y "hoom no le pasa el esfuerzo a <p>" (en su linea o, si no
// entran, en la de abajo: correccion del orquestador); no abren lista, tab
// los saltea y no mandan nada.
func TestCA569_ProviderQueNoRecibeModelo(t *testing.T) {
	for _, p := range []string{"opencode", "gemini"} {
		if modelos.RecibeModelo(p) || modelos.RecibeEsfuerzo(p) {
			t.Fatalf("CA-569 (guarda): hoom no le pasa modelo ni esfuerzo a %s", p)
		}
		a := ttAccionWriter()
		a.Pedido = "hola"
		a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), {Name: p, OK: true}}
		m := ttPedir(t, "w", a)
		m.Tecla(tablero.TeclaTab, ttAhora)
		m.Tecla(tablero.TeclaDer, ttAhora) // p
		mc := ttMC(t, m)
		if mc.ttValor("Modelo") != "< del provider >" || ttWhyDe(mc, "Modelo") != "hoom no le pasa el modelo a "+p {
			t.Fatalf("CA-569: con %s, Modelo gris con su why: %q %q\n%s", p, mc.ttValor("Modelo"), ttWhyDe(mc, "Modelo"), strings.Join(mc.Textos, "\n"))
		}
		if mc.ttValor("Esfuerzo") != "< del provider >" || ttWhyDe(mc, "Esfuerzo") != "hoom no le pasa el esfuerzo a "+p {
			t.Fatalf("CA-569: con %s, Esfuerzo gris con su why: %q %q", p, mc.ttValor("Esfuerzo"), ttWhyDe(mc, "Esfuerzo"))
		}
		if mc.ttValor("Tope") != "< sin tope >" || ttWhyDe(mc, "Tope") != p+" no acepta tope" {
			t.Fatalf("CA-571: con %s (sin budget), Tope gris: %q %q", p, mc.ttValor("Tope"), ttWhyDe(mc, "Tope"))
		}
		// correccion del orquestador: el why que no entra en la columna del
		// origen (15 + 26 = 41, y el texto tiene 66 celdas) va en su propia
		// linea, debajo del selector, desde la columna 15
		for _, et := range []string{"Modelo", "Esfuerzo"} {
			_, i, _ := mc.ttLineaQueEmpieza(et)
			why := "hoom no le pasa el modelo a " + p
			if et == "Esfuerzo" {
				why = "hoom no le pasa el esfuerzo a " + p
			}
			if mc.ttOrigen(et) != "" || i+1 >= len(mc.Textos) || mc.Textos[i+1] != strings.Repeat(" ", 15)+why {
				t.Fatalf("CA-569: el why de %s gris no entra en su linea: va en la de abajo desde la columna 15:\n%s", et, strings.Join(mc.Textos, "\n"))
			}
		}
		m.Tecla(tablero.TeclaTab, ttAhora)
		if f := ttFoco(ttMC(t, m)); f != "Tiempo limite" {
			t.Fatalf("CA-569: tab saltea Modelo, Esfuerzo y Tope grises: %q", f)
		}
		m.Tecla(tablero.TeclaShiftTab, ttAhora)
		if got := ttMC(t, m).ttComando(); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("hola", false)+" --provider "+p {
			t.Fatalf("CA-569: Modelo, Esfuerzo y Tope grises no mandan nada: %q", got)
		}
	}
}

// CA-569: con color, un selector gris va en gris, y los origenes y la
// linea de ayuda tambien.
func TestCA569_GrisesConColor(t *testing.T) {
	cfg := ttAncho()
	cfg.Color = true
	a := ttAccionWriter()
	a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), {Name: "opencode", OK: true}}
	m := ttAbrirModal(t, cfg, "w", tablero.ModalPedir, a)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora)
	for _, x := range m.Pantalla() {
		s := ttSinSGR(x)
		if strings.Contains(s, "Modelo         < del provider >") && ttEstiloDe(x, "< del provider >") != "\x1b[90m" {
			t.Fatalf("CA-569: el selector gris va en gris: %q", x)
		}
		if strings.Contains(s, "Tope           < sin tope >") && ttEstiloDe(x, "< sin tope >") != "\x1b[90m" {
			t.Fatalf("CA-571: el Tope gris va en gris: %q", x)
		}
	}
}

// ---------------------------------------------------------------- CA-570: lo que fija hoom.yaml

// CA-570: si el origen.model de la accion es hoom.yaml, "por defecto del
// provider" va en gris con "hoom.yaml fija <modelo> para el <rol>"; al
// cambiar de provider ese modelo sigue elegido y, si no esta en la lista del
// provider nuevo, la linea gris dice "<modelo> no esta en la lista de
// <provider>: puede rechazarlo"; el esfuerzo sigue las mismas reglas.
func TestCA570_LoQueFijaHoomYAML(t *testing.T) {
	m := ttPedir(t, "w", ttAccionWriter())
	ttTeclas(m, tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaEnter)
	ops, _ := ttLista(t, ttMC(t, m))
	if !strings.HasPrefix(ops[0], "   por defecto del provider") || !strings.HasSuffix(ops[0], "hoom.yaml fija opus para el writer") {
		t.Fatalf("CA-570: con hoom.yaml, 'por defecto del provider' va gris con 'hoom.yaml fija opus para el writer': %q", ops[0])
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla(tablero.TeclaShiftTab, ttAhora) // Provider
	m.Tecla(tablero.TeclaDer, ttAhora)      // codex
	mc := ttMC(t, m)
	if mc.ttValor("Modelo") != "< opus >" || mc.ttValor("Esfuerzo") != "< high >" {
		t.Fatalf("CA-570: con hoom.yaml el modelo y el esfuerzo siguen elegidos al cambiar de provider: %q %q", mc.ttValor("Modelo"), mc.ttValor("Esfuerzo"))
	}
	if got := ttAyudaSel(mc); got != "opus no esta en la lista de codex: puede rechazarlo" {
		t.Fatalf("CA-570: con el foco en Provider la linea gris dice la nota de Modelo: %q", got)
	}
	if got := mc.ttComando(); !strings.HasSuffix(got, " --provider codex") || strings.Contains(got, "--model") || strings.Contains(got, "--effort") {
		t.Fatalf("CA-570: solo cambio el provider: %q", got)
	}
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	ops, _ = ttLista(t, ttMC(t, m))
	if len(ops) < 2 || !strings.HasPrefix(ops[len(ops)-2], " > opus") || !strings.HasPrefix(ops[len(ops)-1], "   otro...") {
		t.Fatalf("CA-570: opus, que no esta en la lista de codex, va antes de otro…: %q", ops)
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	if got := ttAyudaSel(ttMC(t, m)); got != "opus no esta en la lista de codex: puede rechazarlo" {
		t.Fatalf("CA-570: con el foco en Modelo, la nota: %q", got)
	}
}

// CA-570: sin valor de hoom.yaml, cambiar de provider vuelve Modelo y
// Esfuerzo a "por defecto del provider", y volver al provider por defecto
// devuelve lo propuesto (y el comando sin flags).
func TestCA570_SinHoomYAMLCambiarDeProvider(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola"
	a.Model, a.Effort = "sonnet", "low"
	a.Origen.Model, a.Origen.Effort = valores.OrigenTarjeta, valores.OrigenTarjeta
	m := ttPedir(t, "w", a)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora) // codex
	mc := ttMC(t, m)
	if mc.ttValor("Modelo") != "< por defecto del provider >" || mc.ttValor("Esfuerzo") != "< por defecto del provider >" {
		t.Fatalf("CA-570: sin hoom.yaml, otro provider vuelve Modelo y Esfuerzo a 'por defecto del provider': %q %q", mc.ttValor("Modelo"), mc.ttValor("Esfuerzo"))
	}
	m.Tecla(tablero.TeclaIzq, ttAhora) // claude
	mc = ttMC(t, m)
	if mc.ttValor("Modelo") != "< sonnet >" || mc.ttValor("Esfuerzo") != "< low >" || mc.ttOrigen("Modelo") != "de la tarjeta" {
		t.Fatalf("CA-570: volver al provider por defecto devuelve lo propuesto: %q %q %q", mc.ttValor("Modelo"), mc.ttValor("Esfuerzo"), mc.ttOrigen("Modelo"))
	}
	if got := mc.ttComando(); got != "hoom pedir writer tarjeta -m "+tarjetacmd.CitarPedido("hola", false) {
		t.Fatalf("CA-570: de vuelta a lo propuesto, sin flags: %q", got)
	}
}

// ---------------------------------------------------------------- CA-571: Tope y Tiempo limite

// ttOpciones abre la lista del selector n (1 = Provider) y da sus nombres
// (sin la marca ni la linea), la elegida y la cierra.
func ttOpciones(t *testing.T, m *tablero.Modelo, n int) []string {
	t.Helper()
	for m.Lugar() != tablero.LugarPedirPedido {
		m.Tecla(tablero.TeclaShiftTab, ttAhora)
	}
	for i := 0; i < n; i++ {
		m.Tecla(tablero.TeclaTab, ttAhora)
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	ops, _ := ttLista(t, ttMC(t, m))
	m.Tecla(tablero.TeclaEsc, ttAhora)
	var out []string
	for _, o := range ops {
		rest := strings.TrimSpace(o[3:])
		if i := strings.Index(rest, "  "); i >= 0 {
			rest = rest[:i]
		}
		out = append(out, rest)
	}
	return out
}

// CA-571: Tope ofrece primero lo propuesto (el budget_usd de la accion, o
// "sin tope" si no trae), despues 1, 2, 5, 10 y 20 USD y otro… sin repetir,
// nunca "sin tope" cuando lo propuesto es un tope; las que quedan por debajo
// de min_budget_usd, en gris con "<p> pide al menos <min> USD"; Tiempo
// limite ofrece primero lo propuesto y despues 15m, 30m, 45m, 1h y 2h y
// otro… sin repetir.
func TestCA571_ToposYTiempos(t *testing.T) {
	casos := []struct {
		budget  *float64
		timeout int
		tope    string
		tiempo  string
	}{
		{ttF(2.75), 2700, "2.75 USD,1 USD,2 USD,5 USD,10 USD,20 USD,otro...", "45m,15m,30m,1h,2h,otro..."},
		{nil, 1800, "sin tope,1 USD,2 USD,5 USD,10 USD,20 USD,otro...", "30m,15m,45m,1h,2h,otro..."},
		{ttF(5), 5400, "5 USD,1 USD,2 USD,10 USD,20 USD,otro...", "1h30m,15m,30m,45m,1h,2h,otro..."},
		{ttF(20), 7200, "20 USD,1 USD,2 USD,5 USD,10 USD,otro...", "2h,15m,30m,45m,1h,otro..."},
	}
	for _, c := range casos {
		a := ttAccionWriter()
		a.BudgetUSD, a.TimeoutS = c.budget, c.timeout
		m := ttPedir(t, "w", a)
		if got := strings.Join(ttOpciones(t, m, 4), ","); got != c.tope {
			t.Fatalf("CA-571: con budget %v, Tope ofrece %q, ofrece %q", c.budget, c.tope, got)
		}
		if got := strings.Join(ttOpciones(t, m, 5), ","); got != c.tiempo {
			t.Fatalf("CA-571: con timeout %ds, Tiempo limite ofrece %q, ofrece %q", c.timeout, c.tiempo, got)
		}
	}
	// el minimo del provider
	a := ttAccionWriter()
	a.Providers = []boardcmd.ProviderOption{{Name: "claude", OK: true, Budget: true, MinBudgetUSD: 2, Default: true}}
	m := ttPedir(t, "w", a)
	for m.Lugar() != tablero.LugarPedirSelector || ttFoco(ttMC(t, m)) != "Tope" {
		m.Tecla(tablero.TeclaTab, ttAhora)
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	ops, _ := ttLista(t, ttMC(t, m))
	var uno string
	for _, o := range ops {
		if strings.HasPrefix(strings.TrimSpace(o[3:]), "1 USD") {
			uno = o
		}
	}
	if !strings.Contains(uno, "claude pide al menos 2") || !strings.HasSuffix(uno, " USD") {
		t.Fatalf("CA-571: una opcion por debajo de min_budget_usd va gris con 'claude pide al menos <min> USD': %q", uno)
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora) // de 2.75 salta 1 USD (gris)
	if v := ttMC(t, m).ttValor("Tope"); v != "< 2 USD >" {
		t.Fatalf("CA-571: → salta la opcion gris: %q", v)
	}
	if !strings.Contains(ttMC(t, m).ttComando(), " --budget-usd 2") {
		t.Fatalf("CA-571: el tope elegido va como --budget-usd: %q", ttMC(t, m).ttComando())
	}
}

// ---------------------------------------------------------------- CA-574: Firma y Guardar

// CA-574, CA-529: A abre el modal de firma con el texto de la capa 1, "Se va
// a ejecutar: hoom aprobar <slug> --si" y los botones Cancelar (elegido) y
// la etiqueta de la accion; enter enseguida cierra sin nada; → (o tab) y
// enter ejecutan hoom aprobar <slug> --si; ← vuelve a Cancelar; esc cierra.
// Re-expresa CA-529 (en la v1 el boton era "Firmar").
func TestCA574_CA529_AprobarSoloConElBoton(t *testing.T) {
	c := ttNucTenancy()
	a, _ := c.Action(boardcmd.ActAprobar)
	abrir := func() *tablero.Modelo {
		m := ttModelo(ttAncho(), ttTablero(c))
		ttSinOrdenes(t, "CA-574", "A", m.Tecla("A", ttAhora))
		if m.Modal() != tablero.ModalFirma || m.Lugar() != tablero.LugarFirma {
			t.Fatalf("CA-574: A abre el modal de firma: %q %q", m.Modal(), m.Lugar())
		}
		return m
	}
	m := abrir()
	mc := ttMC(t, m)
	texto := strings.Join(mc.Textos, " ")
	if !strings.Contains(strings.Join(strings.Fields(texto), " "), strings.Join(strings.Fields(tarjetacmd.TextoFirma(c, a)), " ")) {
		t.Fatalf("CA-574: el modal dice el texto de la firma de la capa 1:\n%s\nse esperaba %q", texto, tarjetacmd.TextoFirma(c, a))
	}
	if !strings.HasPrefix(mc.Arriba, "# Aprobar spec · nuc-tenancy ") {
		t.Fatalf("CA-574: el titulo es '<etiqueta> · <slug>': %q", mc.Arriba)
	}
	if mc.ttComando() != "hoom aprobar nuc-tenancy --si" || !strings.Contains(texto, "[ Cancelar ]     Aprobar spec") {
		t.Fatalf("CA-574: 'Se va a ejecutar: hoom aprobar nuc-tenancy --si' y Cancelar elegido: %q / %q", mc.ttComando(), texto)
	}
	ttSinOrdenes(t, "CA-574", "enter sobre Cancelar", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Modal() != tablero.SinModal {
		t.Fatalf("CA-574: enter sobre Cancelar cierra: %q", m.Modal())
	}
	for _, elegir := range []tablero.Tecla{tablero.TeclaDer, tablero.TeclaTab} {
		m = abrir()
		ttSinOrdenes(t, "CA-574", string(elegir), m.Tecla(elegir, ttAhora))
		if b := strings.Join(ttMC(t, m).Textos, " "); !strings.Contains(b, "  Cancelar     [ Aprobar spec ]") {
			t.Fatalf("CA-574: %q elige la accion: %q", elegir, b)
		}
		l := ttUnaLanzar(t, "CA-574", "enter sobre la accion", m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, "CA-574", "aprobar", l.Args, []string{"aprobar", "nuc-tenancy", "--si"})
		if l.Rol != "" || m.Modal() != tablero.SinModal {
			t.Fatalf("CA-574: aprobar no es un trabajo de rol y el modal se cierra: %#v %q", l, m.Modal())
		}
		if r := m.Registro(); len(r) != 1 || r[0].Texto != "Aprobar spec · nuc-tenancy · lanzando..." || r[0].Comando != "hoom aprobar nuc-tenancy --si" {
			t.Fatalf("CA-563: la firma va al Registro: %#v", r)
		}
	}
	m = abrir()
	ttSinOrdenes(t, "CA-574", "→ ←", ttTeclas(m, tablero.TeclaDer, tablero.TeclaIzq))
	ttSinOrdenes(t, "CA-574", "enter tras volver a Cancelar", m.Tecla(tablero.TeclaEnter, ttAhora))
	m = abrir()
	ttSinOrdenes(t, "CA-574", "shift-tab esc", ttTeclas(m, tablero.TeclaShiftTab, tablero.TeclaEsc))
	if m.Modal() != tablero.SinModal {
		t.Fatalf("CA-574: esc cierra la firma: %q", m.Modal())
	}
}

// CA-574, CA-529: I igual, con hoom integrar <slug> --si y el texto de
// integrar.
func TestCA574_CA529_IntegrarSoloConElBoton(t *testing.T) {
	c := ttIntegrable()
	a := c.Actions[0]
	m := ttModelo(ttAncho(), ttTablero(c))
	ttSinOrdenes(t, "CA-574", "I", m.Tecla("I", ttAhora))
	mc := ttMC(t, m)
	texto := strings.Join(strings.Fields(strings.Join(mc.Textos, " ")), " ")
	if !strings.Contains(texto, strings.Join(strings.Fields(tarjetacmd.TextoFirma(c, a)), " ")) || !strings.Contains(texto, "[ Cancelar ] Integrar") ||
		mc.ttComando() != "hoom integrar lista --si" {
		t.Fatalf("CA-574: I muestra el texto de integrar, su comando y Cancelar elegido: %q", texto)
	}
	ttSinOrdenes(t, "CA-574", "enter sobre Cancelar", m.Tecla(tablero.TeclaEnter, ttAhora))
	m.Tecla("I", ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora)
	l := ttUnaLanzar(t, "CA-574", "enter sobre Integrar", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-574", "integrar", l.Args, []string{"integrar", "lista", "--si"})
}

// CA-574, CA-529: s abre el modal de guardar con "Se guarda en git:" y las
// rutas, "Se va a ejecutar: hoom guardar <slug>" y los botones Guardar
// (elegido) y Cancelar; enter guarda; esc no ejecuta nada.
func TestCA574_CA529_Guardar(t *testing.T) {
	c := ttTarjetaCon(ttAccionGuardar(true, "", "internal/a.go", ".hoom/specs/tarjeta.md"))
	m := ttModelo(ttAncho(), ttTablero(c))
	m.Tecla("s", ttAhora)
	if m.Modal() != tablero.ModalGuardar || m.Lugar() != tablero.LugarGuardar {
		t.Fatalf("CA-574: s abre Guardar: %q", m.Modal())
	}
	mc := ttMC(t, m)
	ttIgualesAlPrincipio(t, "CA-574", "Guardar", mc.Textos, []string{"Se guarda en git:", "internal/a.go", ".hoom/specs/tarjeta.md", "",
		"Se va a ejecutar:", "hoom guardar tarjeta", ""})
	if b := strings.Join(mc.Textos, " "); !strings.Contains(b, "[ Guardar ]     Cancelar") || !strings.HasPrefix(mc.Arriba, "# Guardar · tarjeta ") {
		t.Fatalf("CA-574: los botones Guardar (elegido) y Cancelar, y el titulo: %q %q", b, mc.Arriba)
	}
	ttSinOrdenes(t, "CA-574", "esc", m.Tecla(tablero.TeclaEsc, ttAhora))
	m.Tecla("s", ttAhora)
	l := ttUnaLanzar(t, "CA-574", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-574", "guardar", l.Args, []string{"guardar", "tarjeta"})
	if l.Rol != "" {
		t.Fatalf("CA-574: guardar no es un trabajo de rol: %#v", l)
	}
	// sin rutas en la accion: las sin guardar de la tarjeta
	c = ttTarjetaCon(ttAccionGuardar(true, ""))
	c.Unsynced = []string{"x.go"}
	m = ttModelo(ttAncho(), ttTablero(c))
	m.Tecla("s", ttAhora)
	if mc := ttMC(t, m); len(mc.Textos) < 2 || mc.Textos[1] != "x.go" {
		t.Fatalf("CA-574: sin rutas en la accion, las sin guardar de la tarjeta: %q", mc.Textos)
	}
}

// ttIntegrable es una tarjeta en Tu aceptacion con integrar habilitada.
func ttIntegrable() boardcmd.Card {
	c := ttCard("lista", "Lista para integrar", boardcmd.ColTuAceptacion, "lista", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	c.Actions = []boardcmd.Action{ttAccionIntegrar()}
	return c
}

// ttCambio es el mensaje de una confirmacion cuyos datos cambiaron.
func ttCambio(slug string) string {
	return "cambio lo que ibas a confirmar en " + slug + ": vuelve a abrirlo"
}

// CA-574, CA-529 (hallazgo ecb288 de la v1): una firma esta atada a lo que
// muestra. Abierta A (o I) y elegida la accion, una lectura del tablero donde
// la tarjeta cambia lo que la firma dice (su titulo, sus criterios, quien
// firma), donde la accion queda deshabilitada o donde desaparece, la cierra:
// la linea de mensajes dice en rojo el why o "cambio lo que ibas a confirmar
// en <slug>: vuelve a abrirlo", y el enter que sigue no ordena nada.
func TestCA574_CA529_LaFirmaEstaAtadaALoQueMuestra(t *testing.T) {
	const whyA = "el spec cambio despues de abrir la confirmacion"
	const whyI = "falta el veredicto verde de la base"
	casos := []struct {
		nombre  string
		tecla   tablero.Tecla
		accion  string
		base    func() boardcmd.Card
		cambiar func(c *boardcmd.Card)
		why     string
	}{
		{"A: el titulo", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Item.Titulo = "Tenancy del nucleo, otra" }, ""},
		{"A: los criterios", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Evidence.Criteria = 6 }, ""},
		{"A: quien firma", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Actions[0].Signer = "Otra Persona <otra@hoom.dev>" }, ""},
		{"A: deshabilitada", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Actions[0].Enabled, c.Actions[0].Why = false, whyA }, whyA},
		{"A: desaparece", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Actions = []boardcmd.Action{} }, ""},
		{"I: el titulo", "I", boardcmd.ActIntegrar, ttIntegrable, func(c *boardcmd.Card) { c.Item.Titulo = "Lista para integrar, otra" }, ""},
		{"I: deshabilitada", "I", boardcmd.ActIntegrar, ttIntegrable, func(c *boardcmd.Card) { c.Actions[0].Enabled, c.Actions[0].Why = false, whyI }, whyI},
	}
	for _, cs := range casos {
		for _, color := range []bool{false, true} {
			que := fmt.Sprintf("%s (color=%v)", cs.nombre, color)
			cfg := ttAncho()
			cfg.Color = color
			base, nuevo := cs.base(), cs.base()
			cs.cambiar(&nuevo)
			if cs.why == "" {
				a0, _ := base.Action(cs.accion)
				if a1, ok := nuevo.Action(cs.accion); ok && tarjetacmd.TextoFirma(base, a0) == tarjetacmd.TextoFirma(nuevo, a1) {
					t.Fatalf("CA-574 (guarda): %s no cambia el texto de la firma", que)
				}
			}
			m := ttModelo(cfg, ttTablero(base))
			ttSinOrdenes(t, "CA-574", que+": "+string(cs.tecla), m.Tecla(cs.tecla, ttAhora))
			ttSinOrdenes(t, "CA-574", que+": → elige la accion", m.Tecla(tablero.TeclaDer, ttAhora))
			if l := ttLanzares(m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(nuevo)})); len(l) != 0 {
				t.Fatalf("CA-574: %s: la lectura que cambia la firma no ordena nada: %#v", que, l)
			}
			if m.Modal() != tablero.SinModal {
				t.Fatalf("CA-574: %s: la firma se cierra: %q", que, m.Modal())
			}
			msg := cs.why
			if msg == "" {
				msg = ttCambio(base.Slug)
			}
			ttDiceMensaje(t, "CA-574", que, m, color, msg)
			if l := ttLanzares(m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second))); len(l) != 0 {
				t.Fatalf("CA-574: %s: el enter que sigue no firma nada: %#v", que, l)
			}
		}
	}
}

// CA-574, CA-529 (hallazgo ecb288): una firma cuyos datos no cambiaron
// firma como antes, aunque la tarjeta cambie en lo que la firma no muestra
// (el plain, el gasto) o el tablero traiga otra tarjeta.
func TestCA574_CA529_LaFirmaSinCambiosSigueValiendo(t *testing.T) {
	cambios := []struct {
		nombre  string
		cambiar func(c *boardcmd.Card) []boardcmd.Card
	}{
		{"igual", func(c *boardcmd.Card) []boardcmd.Card { return nil }},
		{"el plain", func(c *boardcmd.Card) []boardcmd.Card { c.Plain = "otro plain de la tarjeta"; return nil }},
		{"el gasto", func(c *boardcmd.Card) []boardcmd.Card {
			c.Spend = boardcmd.Spend{CostUSD: ttF(9.99), Runs: 3}
			return nil
		}},
		{"otra tarjeta", func(c *boardcmd.Card) []boardcmd.Card { return []boardcmd.Card{ttBacklog("nueva", "Tarjeta nueva")} }},
	}
	for _, cb := range cambios {
		for _, k := range []struct {
			tecla tablero.Tecla
			base  func() boardcmd.Card
			args  []string
		}{
			{"A", ttNucTenancy, []string{"aprobar", "nuc-tenancy", "--si"}},
			{"I", ttIntegrable, []string{"integrar", "lista", "--si"}},
		} {
			que := cb.nombre + " con " + string(k.tecla)
			m := ttModelo(ttAncho(), ttTablero(k.base()))
			m.Tecla(k.tecla, ttAhora)
			m.Tecla(tablero.TeclaDer, ttAhora)
			for i := 1; i <= 2; i++ {
				nuevo := k.base()
				otras := cb.cambiar(&nuevo)
				if l := ttLanzares(m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Duration(2*i) * time.Second), Tablero: ttTablero(append(otras, nuevo)...)})); len(l) != 0 {
					t.Fatalf("CA-574: %s: leer no firma: %#v", que, l)
				}
			}
			if m.Modal() != tablero.ModalFirma {
				t.Fatalf("CA-574: %s: sin cambios en lo que muestra la firma sigue abierta: %q (%q)", que, m.Modal(), ttMensaje(m))
			}
			l := ttUnaLanzar(t, "CA-574", que+": enter", m.Tecla(tablero.TeclaEnter, ttAhora.Add(5*time.Second)))
			ttMismosArgs(t, "CA-574", que, l.Args, k.args)
		}
	}
}

// CA-574, CA-529 (hallazgos ecb288, f0fbb9 y 4581a6 de la v1): la firma
// tambien esta atada al spec que mostro el detalle de su tarjeta en [0]: un
// LeerDetalle con otro markdown, uno que falla (Detalle.Err, aunque traiga
// el mismo spec) o uno sin spec la cierra con el mensaje de cambio en rojo,
// y el enter que sigue no firma; con el mismo, firma. Con A y con I.
func TestCA574_CA529_LaFirmaAtadaAlSpecDeSuDetalle(t *testing.T) {
	const md1 = "# Spec: tenancy\n\n## Criterios de aceptacion\n\n- CA-1: uno.\n"
	const md2 = md1 + "- CA-2: un criterio que el arquitecto agrego despues.\n"
	spec := func(c boardcmd.Card, md string) boardcmd.SpecDetail {
		return boardcmd.SpecDetail{Path: ".hoom/specs/" + c.Slug + ".md", Exists: md != "", Markdown: md}
	}
	for _, f := range []struct {
		tecla tablero.Tecla
		base  func() boardcmd.Card
		args  []string
	}{{"A", ttNucTenancy, []string{"aprobar", "nuc-tenancy", "--si"}}, {"I", ttIntegrable, []string{"integrar", "lista", "--si"}}} {
		lecturas := []struct {
			nombre string
			det    func(c boardcmd.Card) tablero.Detalle
			cierra bool
		}{
			{"otro spec", func(c boardcmd.Card) tablero.Detalle {
				return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec(c, md2)}}
			}, true},
			{"la lectura falla", func(c boardcmd.Card) tablero.Detalle {
				return tablero.Detalle{Slug: c.Slug, Err: errors.New("disco roto")}
			}, true},
			{"falla con el mismo spec", func(c boardcmd.Card) tablero.Detalle {
				return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec(c, md1)}, Err: errors.New("no se pudo leer la historia")}
			}, true},
			{"sin spec", func(c boardcmd.Card) tablero.Detalle {
				return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec(c, "")}}
			}, true},
			{"el mismo spec", func(c boardcmd.Card) tablero.Detalle {
				return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec(c, md1)}}
			}, false},
		}
		for _, lec := range lecturas {
			for _, color := range []bool{false, true} {
				c := f.base()
				que := fmt.Sprintf("%s: %s (color=%v)", f.tecla, lec.nombre, color)
				cfg := ttAncho()
				cfg.Color = color
				m := ttModelo(cfg, ttTablero(c))
				m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: f.base(), Spec: spec(c, md1)}})
				m.Tecla(tablero.TeclaEnter, ttAhora) // [0] de la tarjeta, con su spec
				ttSinOrdenes(t, "CA-574", que+": "+string(f.tecla), m.Tecla(f.tecla, ttAhora))
				m.Tecla(tablero.TeclaDer, ttAhora)
				// guarda: el mismo tablero no la cierra
				m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(f.base())})
				if m.Modal() != tablero.ModalFirma {
					t.Fatalf("CA-574 (guarda): %s: con el mismo tablero la firma sigue: %q (%q)", que, m.Modal(), ttMensaje(m))
				}
				m.LeerDetalle(lec.det(f.base()))
				if !lec.cierra {
					l := ttUnaLanzar(t, "CA-574", que+": enter", m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second)))
					ttMismosArgs(t, "CA-574", que, l.Args, f.args)
					continue
				}
				if m.Modal() != tablero.SinModal {
					t.Fatalf("CA-574: %s: la firma se cierra: %q", que, m.Modal())
				}
				ttDiceMensaje(t, "CA-574", que, m, color, ttCambio(c.Slug))
				if l := ttLanzares(m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second))); len(l) != 0 {
					t.Fatalf("CA-574: %s: el enter que sigue no firma: %#v", que, l)
				}
			}
		}
	}
}

// CA-574, CA-529 (hallazgo ecb288): Guardar esta atado a sus rutas: otras
// rutas (o una de mas) lo cierran con el mensaje de cambio; guardar
// deshabilitada, con su why; el enter que sigue no guarda. Con las mismas,
// guarda.
func TestCA574_CA529_GuardarAtadoASusRutas(t *testing.T) {
	conRutas := func(rutas ...string) boardcmd.Card {
		c := ttTarjetaCon(ttAccionGuardar(true, "", rutas...))
		c.Unsynced = append([]string{}, rutas...)
		return c
	}
	cambios := []struct {
		nombre string
		nuevo  boardcmd.Card
		msg    string
	}{
		{"otra ruta", conRutas("internal/a.go", "internal/c.go"), ttCambio("tarjeta")},
		{"una de mas", conRutas("internal/a.go", "internal/b.go", "internal/c.go"), ttCambio("tarjeta")},
		{"deshabilitada", ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttWhyGuardar},
	}
	for _, cb := range cambios {
		for _, color := range []bool{false, true} {
			que := fmt.Sprintf("%s (color=%v)", cb.nombre, color)
			cfg := ttAncho()
			cfg.Color = color
			m := ttModelo(cfg, ttTablero(conRutas("internal/a.go", "internal/b.go")))
			m.Tecla("s", ttAhora)
			if l := ttLanzares(m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cb.nuevo)})); len(l) != 0 {
				t.Fatalf("CA-574: %s: leer no guarda: %#v", que, l)
			}
			if m.Modal() != tablero.SinModal {
				t.Fatalf("CA-574: %s: Guardar se cierra: %q", que, m.Modal())
			}
			ttDiceMensaje(t, "CA-574", que, m, color, cb.msg)
			if l := ttLanzares(m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second))); len(l) != 0 {
				t.Fatalf("CA-574: %s: el enter que sigue no guarda: %#v", que, l)
			}
		}
	}
	m := ttModelo(ttAncho(), ttTablero(conRutas("internal/a.go", "internal/b.go")))
	m.Tecla("s", ttAhora)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(conRutas("internal/a.go", "internal/b.go"))})
	l := ttUnaLanzar(t, "CA-574", "enter con las mismas rutas", m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second)))
	ttMismosArgs(t, "CA-574", "guardar", l.Args, []string{"guardar", "tarjeta"})
}

// ---------------------------------------------------------------- lo que no entra (CA-565, CA-568, CA-574)

// CA-565, CA-568, CA-527, CA-533 (hallazgo 9f25a9 de la v1): a 60x15 el
// modal Pedir del writer entra en las B = 13 lineas dejando afuera, en el
// orden del contrato, lo que no entra; "Se va a ejecutar" y los botones
// nunca se van; enter lanza lo mismo que antes.
func TestCA565_CA568_PedirChicoConservaElComandoYLosBotones(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = strings.Repeat("abcdefghij", 12)
	m := ttAbrirModal(t, ttConfig(60, 15, false), "w", tablero.ModalPedir, a)
	ttForma(t, "CA-565", m, 60, 15, false)
	mc := ttModal(t, "CA-565", m, 60)
	if mc.Fila < 0 || mc.Fila+mc.Alto > 13 {
		t.Fatalf("CA-565: el modal entra en las lineas de los paneles (1..13): linea %d, alto %d", mc.Fila+1, mc.Alto)
	}
	todo := strings.Join(mc.Textos, "\n")
	if !strings.Contains(todo, "Se va a ejecutar:") || !strings.Contains(todo, "hoom pedir writer tarjeta") || !strings.Contains(todo, "[ Pedir al writer ]") {
		t.Fatalf("CA-565: a 60x15 'Se va a ejecutar', el comando y los botones nunca se van:\n%s", todo)
	}
	for _, et := range []string{"Provider", "Modelo", "Esfuerzo", "Tope", "Tiempo limite"} {
		if mc.ttCampo(et) == "" {
			t.Fatalf("CA-568: a 60x15 el selector %s queda:\n%s", et, todo)
		}
	}
	for _, l := range mc.Textos {
		if l == "" {
			t.Fatalf("CA-565: las lineas vacias se van antes que el Pedido y los selectores:\n%s", todo)
		}
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	l := ttUnaLanzar(t, "CA-568", "enter a 60x15", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-568", "a 60x15", l.Args, []string{"pedir", "writer", "tarjeta", "-m", a.Pedido})
}

// CA-574, CA-533 (hallazgos 9f25a9 y beb53d de la v1): a 60x15 Guardar con
// muchas rutas muestra las primeras y "y <n> rutas mas" en una ultima linea,
// con "Se va a ejecutar", el comando y los botones; enter guarda. En el borde
// exacto, las que entran: ninguna linea perdida.
func TestCA574_CA533_GuardarConMuchasRutasAChico(t *testing.T) {
	for _, n := range []int{3, 5, 6, 7, 8, 9, 20} {
		var rutas []string
		for i := 0; i < n; i++ {
			rutas = append(rutas, fmt.Sprintf("internal/archivo-%02d.go", i))
		}
		c := ttTarjetaCon(ttAccionGuardar(true, "", rutas...))
		c.Unsynced = append([]string{}, rutas...)
		m := ttModelo(ttConfig(60, 15, false), ttTablero(c))
		m.Tecla("s", ttAhora)
		ttForma(t, "CA-574", m, 60, 15, false)
		mc := ttModal(t, "CA-574", m, 60)
		if mc.Fila+mc.Alto > 13 {
			t.Fatalf("CA-574: %d rutas: el modal entra en las 13 lineas: linea %d, alto %d", n, mc.Fila+1, mc.Alto)
		}
		if mc.Textos[0] != "Se guarda en git:" {
			t.Fatalf("CA-574: %d rutas: empieza con 'Se guarda en git:': %q", n, mc.Textos)
		}
		j := 0
		for j < n && 1+j < len(mc.Textos) && mc.Textos[1+j] == rutas[j] {
			j++
		}
		resto := mc.Textos[1+j:]
		if j < n {
			if len(resto) == 0 || resto[0] != fmt.Sprintf("y %d rutas mas", n-j) {
				t.Fatalf("CA-574: %d rutas: muestra las primeras (%d) y 'y %d rutas mas': %q", n, j, n-j, mc.Textos)
			}
			resto = resto[1:]
		}
		texto := strings.Join(resto, "\n")
		if !strings.Contains(texto, "hoom guardar tarjeta") || !strings.Contains(texto, "Se va a ejecutar:") || !strings.Contains(texto, "[ Guardar ]") {
			t.Fatalf("CA-574: %d rutas: despues de las rutas, 'Se va a ejecutar', el comando y los botones: %q", n, resto)
		}
		if n <= 3 && j != n {
			t.Fatalf("CA-574: %d rutas entran enteras: %q", n, mc.Textos)
		}
		l := ttUnaLanzar(t, "CA-574", fmt.Sprintf("%d rutas: enter", n), m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, "CA-574", "guardar a 60x15", l.Args, []string{"guardar", "tarjeta"})
	}
}

// CA-574, CA-529, CA-533 (hallazgo 9f25a9 de la v1): una firma cuyo texto no
// entra entero nunca se firma: enter sobre la accion no ordena nada y la
// linea de mensajes dice en rojo "agranda la terminal: la firma no entra
// entera"; tambien si la terminal se achica con la firma abierta. Donde
// entra, firma.
func TestCA574_CA529_LaFirmaQueNoEntraNoSeFirma(t *testing.T) {
	const msg = "agranda la terminal: la firma no entra entera"
	c := ttNucTenancy()
	c.Item.Titulo = strings.TrimSpace(strings.Repeat("palabra ", 380))
	firmar := func(m *tablero.Modelo) []tablero.Orden {
		t.Helper()
		if m.Modal() != tablero.ModalFirma {
			t.Fatalf("CA-574: A abre la firma: %q", m.Modal())
		}
		m.Tecla(tablero.TeclaDer, ttAhora)
		return m.Tecla(tablero.TeclaEnter, ttAhora)
	}
	for _, tam := range []struct {
		ancho, alto int
		color       bool
	}{{200, 15, false}, {200, 15, true}, {60, 15, false}} {
		cfg := ttConfig(tam.ancho, tam.alto, false)
		cfg.Color = tam.color
		que := fmt.Sprintf("a %dx%d (color=%v)", tam.ancho, tam.alto, tam.color)
		m := ttModelo(cfg, ttTablero(c))
		m.Tecla("A", ttAhora)
		ttSinOrdenes(t, "CA-574", "enter sobre la accion "+que, firmar(m))
		if tam.ancho == 60 {
			if got := ttMensaje(m); !strings.HasPrefix(got, "agranda la terminal") {
				t.Fatalf("CA-574: %s la linea de mensajes dice %q (cortado al ancho): %q", que, msg, got)
			}
			continue
		}
		ttDiceMensaje(t, "CA-574", que, m, tam.color, msg)
	}
	m := ttModelo(ttConfig(200, 40, false), ttTablero(c))
	m.Tecla("A", ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora)
	m.Tamano(200, 15)
	ttSinOrdenes(t, "CA-574", "enter tras achicar", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttDiceMensaje(t, "CA-574", "tras achicar", m, false, msg)
	m = ttModelo(ttConfig(200, 40, false), ttTablero(c))
	m.Tecla("A", ttAhora)
	l := ttUnaLanzar(t, "CA-574", "enter sobre la accion a 200x40", firmar(m))
	ttMismosArgs(t, "CA-574", "aprobar", l.Args, []string{"aprobar", "nuc-tenancy", "--si"})
}
