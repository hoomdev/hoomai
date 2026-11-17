// Tests adversariales de la v3 de hoom tablero: el mouse en la lista, el
// detalle, la caja y los modales (CA-719, que re-expresa CA-575).
package tablero_test

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// tv3cClic hace un clic en la celda (x, y) y exige que no de ordenes.
func tv3cClic(t *testing.T, ca string, m *tablero.Modelo, x, y int) {
	t.Helper()
	ttSinOrdenes(t, ca, "un clic", m.Raton(tablero.Raton{Accion: tablero.RatonClic, X: x, Y: y}, ttAhora))
}

// tv3cRueda mueve la rueda una muesca en la celda (x, y) y exige que no de
// ordenes.
func tv3cRueda(t *testing.T, ca string, m *tablero.Modelo, abajo bool, x, y int) {
	t.Helper()
	a := tablero.RatonRuedaArriba
	if abajo {
		a = tablero.RatonRuedaAbajo
	}
	ttSinOrdenes(t, ca, "la rueda", m.Raton(tablero.Raton{Accion: a, X: x, Y: y}, ttAhora))
}

// tv3cLineaDe es el indice de pantalla de la primera linea interior de la
// lista (W >= 100) que, sin espacios a los lados, empieza con texto.
func tv3cLineaDe(t *testing.T, ca string, m *tablero.Modelo, ancho int, texto string) int {
	t.Helper()
	ls := ttLimpia(m)
	for y := 6; y <= len(ls)-5; y++ {
		if strings.HasPrefix(strings.TrimSpace(ttTramo(ls[y], 1, tv3cL(ancho)-1)), texto) {
			return y
		}
	}
	t.Fatalf("%s: la lista tiene una linea que empieza con %q:\n%s", ca, texto, ttPantalla(m))
	return -1
}

// CA-719 y CA-575: un clic en una tarjeta de la lista (en cualquiera de sus
// dos lineas) la elige y le da el foco a la lista; uno en una linea de grupo
// solo le da el foco; uno en la linea de HECHO la elige sin abrirla; uno en
// el borde de la lista le da el foco.
func TestV3C_CA719_CA575_ClicEnLaLista(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	const ancho = 100
	m := tv3cBase(tv3cConfig(ancho, 30))
	conFocoEnElDetalle := func() {
		t.Helper()
		if m.Foco() != tablero.ZonaDetalle {
			ttTeclas(m, tablero.TeclaTab)
		}
		if m.Foco() != tablero.ZonaDetalle {
			t.Fatalf("%s: precondicion del spec: tab le da el foco al detalle", ca)
		}
	}
	mira := func(que, elegida string) {
		t.Helper()
		if m.Elegida() != elegida || m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas {
			t.Fatalf("%s: %s: elegida=%q foco=%d lugar=%q, se esperaba elegida=%q con el foco en la lista", ca, que,
				m.Elegida(), m.Foco(), m.Lugar(), elegida)
		}
		tv3cSinModal(t, ca, que, m)
	}
	conFocoEnElDetalle()
	y := tv3cFilaY(t, ca, m, ancho, "Precios por region")
	tv3cClic(t, ca, m, 5, y)
	mira("clic en la primera linea de precios", "precios")
	if a, _ := tv3cBordeDetalle(m, ancho); !strings.Contains(a, "Precios por region") {
		t.Fatalf("%s: el detalle sigue a la tarjeta elegida con el clic: %q", ca, a)
	}
	conFocoEnElDetalle()
	y = tv3cFilaY(t, ca, m, ancho, "Convenciones de la API")
	tv3cClic(t, ca, m, 20, y+1)
	mira("clic en la segunda linea de infra-api", "infra-api")
	conFocoEnElDetalle()
	tv3cClic(t, ca, m, 3, tv3cLineaDe(t, ca, m, ancho, "CORRIENDO"))
	mira("clic en una linea de grupo", "infra-api")
	conFocoEnElDetalle()
	tv3cClic(t, ca, m, 0, 5)
	mira("clic en el borde de arriba de la lista", "infra-api")
	conFocoEnElDetalle()
	tv3cClic(t, ca, m, tv3cL(ancho)-1, 12)
	mira("clic en el borde de la derecha de la lista", "infra-api")
	conFocoEnElDetalle()
	tv3cClic(t, ca, m, 10, tv3cLineaDe(t, ca, m, ancho, "HECHO"))
	mira("clic en la linea de HECHO", "")
	if l, _ := ttLineaCon(m, "HECHO 3"); !strings.Contains(l, "HECHO 3 >") || ttTiene(m, "Viejo") {
		t.Fatalf("%s: el clic elige HECHO sin abrirla: %q", ca, l)
	}
	// en la vista Hallazgos un clic en un hallazgo lo elige
	ttTeclas(m, "H", tablero.TeclaTab)
	tv3cClic(t, ca, m, 5, tv3cFilaY(t, ca, m, ancho, "medio.go"))
	if r, _ := m.ArchivoElegido(); r != "internal/api/medio.go" || m.Foco() != tablero.ZonaLista {
		t.Fatalf("%s: en la vista Hallazgos el clic elige el hallazgo y le da el foco a la lista: %s foco=%d", ca, r, m.Foco())
	}
}

// CA-719 y CA-575: un clic en el detalle le da el foco sin cambiar la
// tarjeta elegida.
func TestV3C_CA719_CA575_ClicEnElDetalle(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	const ancho = 100
	m := tv3cBase(tv3cConfig(ancho, 30))
	ttTeclas(m, "j")
	for _, p := range [][2]int{{60, 10}, {tv3cL(ancho) + 1, 6}, {ancho - 2, 25}} {
		tv3cClic(t, ca, m, p[0], p[1])
		if m.Foco() != tablero.ZonaDetalle || m.Elegida() != "precios" || m.Lugar() != tablero.LugarVerTarjeta {
			t.Fatalf("%s: un clic en (%d,%d) del detalle le da el foco: foco=%d elegida=%q lugar=%q", ca, p[0], p[1],
				m.Foco(), m.Elegida(), m.Lugar())
		}
		tv3cSinModal(t, ca, "clic en el detalle", m)
		ttTeclas(m, tablero.TeclaTab)
	}
}

// CA-719 y CA-575: un clic en la caja TU SIGUIENTE PASO elige en la lista la
// tarjeta de la caja y le da el foco a la lista, tambien desde la vista
// Hallazgos, el proyecto o la busqueda; sin tarjeta en la caja no hace nada.
func TestV3C_CA719_CA575_ClicEnLaCaja(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	const ancho = 100
	for _, prep := range [][]tablero.Tecla{{"j", "j"}, {"j", "j", tablero.TeclaTab}, {"H"}, {"v"}, {"/"}, {"G"}} {
		m := tv3cBase(tv3cConfig(ancho, 30))
		ttTeclas(m, prep...)
		caja := m.Caja()
		if caja != "nuc-tenancy" {
			t.Fatalf("%s: precondicion del spec: la caja tiene nuc-tenancy: %q", ca, caja)
		}
		for _, y := range []int{1, 2, 3, 4} {
			tv3cClic(t, ca, m, 30, y)
			if m.Elegida() != caja || m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas {
				t.Fatalf("%s: tras %q, un clic en la linea %d de la caja elige %s con el foco en la lista: elegida=%q foco=%d lugar=%q",
					ca, prep, y, caja, m.Elegida(), m.Foco(), m.Lugar())
			}
			tv3cSinModal(t, ca, "clic en la caja", m)
			ttTeclas(m, prep...)
		}
	}
	// sin tarjeta en la caja (solo EN COLA) no hace nada
	m := tv3cModelo(tv3cConfig(ancho, 30), ttBacklog("cola-a", "Cola A"), ttBacklog("cola-b", "Cola B"))
	ttTeclas(m, "j", tablero.TeclaTab)
	if m.Caja() != "" {
		t.Fatalf("%s: precondicion del spec: con solo EN COLA la caja no tiene tarjeta: %q", ca, m.Caja())
	}
	antes := tv3cFoto(m)
	tv3cClic(t, ca, m, 30, 2)
	if despues := tv3cFoto(m); despues != antes {
		t.Fatalf("%s: sin tarjeta en la caja el clic no hace nada:\nantes:\n%s\ndespues:\n%s", ca, antes, despues)
	}
}

// CA-719: un clic en la cabecera, en la linea del Registro o en las dos
// ultimas lineas no hace nada (el del Registro no abre el modal).
func TestV3C_CA719_ClicesSinEfecto(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	const ancho, alto = 100, 30
	m := tv3cBase(tv3cConfig(ancho, alto))
	tv3cFin(m, ttUnaLanzar(t, ca, "u", ttTeclas(m, "u")).ID, 0, "")
	ttTeclas(m, "j", tablero.TeclaTab)
	antes := tv3cFoto(m)
	for _, y := range []int{0, alto - 3, alto - 2, alto - 1} {
		for _, x := range []int{0, 1, 5, 40, 60, ancho - 1} {
			tv3cClic(t, ca, m, x, y)
			if despues := tv3cFoto(m); despues != antes {
				t.Fatalf("%s: un clic en (%d,%d) no hace nada:\nantes:\n%s\ndespues:\n%s", ca, x, y, antes, despues)
			}
		}
	}
}

// CA-719 y CA-575: la rueda sobre la lista mueve la seleccion una fila por
// muesca sin mover el foco; sobre el detalle lo desplaza 3 lineas; sobre lo
// demas (la lista plegada incluida) no hace nada.
func TestV3C_CA719_CA575_LaRueda(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	const ancho = 100
	for _, foco := range []tablero.Zona{tablero.ZonaLista, tablero.ZonaDetalle} {
		m := tv3cBase(tv3cConfig(ancho, 30))
		if foco == tablero.ZonaDetalle {
			ttTeclas(m, tablero.TeclaTab)
		}
		for _, p := range []struct {
			abajo   bool
			elegida string
		}{{true, "precios"}, {true, "infra-api"}, {false, "precios"}, {false, "nuc-tenancy"}, {false, "nuc-tenancy"}} {
			tv3cRueda(t, ca, m, p.abajo, 10, 15)
			if m.Elegida() != p.elegida || m.Foco() != foco {
				t.Fatalf("%s: la rueda sobre la lista elige %q sin mover el foco (%d): elegida=%q foco=%d", ca, p.elegida, foco,
					m.Elegida(), m.Foco())
			}
		}
	}
	// sobre el detalle, 3 lineas, con el foco en la lista o en el detalle
	for _, tab := range []bool{false, true} {
		a, b := tv3cBase(tv3cConfig(ancho, 30)), tv3cBase(tv3cConfig(ancho, 30))
		if tab {
			ttTeclas(a, tablero.TeclaTab)
		}
		ttTeclas(b, tablero.TeclaTab, "j", "j", "j")
		tv3cRueda(t, ca, a, true, 70, 15)
		if strings.Join(tv3cInteriorDetalle(a, ancho), "\n") != strings.Join(tv3cInteriorDetalle(b, ancho), "\n") {
			t.Fatalf("%s: la rueda sobre el detalle lo desplaza 3 lineas (como j j j)", ca)
		}
		if a.Elegida() != "nuc-tenancy" {
			t.Fatalf("%s: la rueda sobre el detalle no cambia la tarjeta elegida: %q", ca, a.Elegida())
		}
		tv3cRueda(t, ca, a, false, 70, 15)
		ttTeclas(b, "k", "k", "k")
		if strings.Join(tv3cInteriorDetalle(a, ancho), "\n") != strings.Join(tv3cInteriorDetalle(b, ancho), "\n") {
			t.Fatalf("%s: la rueda arriba sobre el detalle vuelve 3 lineas", ca)
		}
	}
	// sobre lo demas, nada
	m := tv3cBase(tv3cConfig(ancho, 30))
	ttTeclas(m, "j", tablero.TeclaTab, "j")
	antes := tv3cFoto(m)
	for _, y := range []int{0, 1, 2, 3, 4, 27, 28, 29} {
		for _, abajo := range []bool{true, false} {
			tv3cRueda(t, ca, m, abajo, 30, y)
			if despues := tv3cFoto(m); despues != antes {
				t.Fatalf("%s: la rueda en la linea %d no hace nada:\nantes:\n%s\ndespues:\n%s", ca, y, antes, despues)
			}
		}
	}
	// la lista plegada (60..99 columnas, foco en el detalle)
	m = tv3cBase(tv3cConfig(80, 24))
	ttTeclas(m, tablero.TeclaTab)
	ls := ttLimpia(m)
	y := -1
	for i, l := range ls {
		if strings.HasPrefix(l, "-Tarjetas · ") || strings.HasPrefix(l, "=Tarjetas · ") {
			y = i
			break
		}
	}
	if y < 0 {
		t.Fatalf("%s: a 80 columnas con el foco en el detalle la lista esta plegada (\"-Tarjetas · n de m\"):\n%s", ca, ttPantalla(m))
	}
	antes = tv3cFoto(m)
	for _, abajo := range []bool{true, false} {
		tv3cRueda(t, ca, m, abajo, 20, y)
		if despues := tv3cFoto(m); despues != antes {
			t.Fatalf("%s: la rueda sobre la lista plegada no hace nada:\nantes:\n%s\ndespues:\n%s", ca, antes, despues)
		}
	}
	// y un clic en la lista plegada le da el foco
	tv3cClic(t, ca, m, 20, y)
	if m.Foco() != tablero.ZonaLista || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("%s: un clic en la lista plegada le da el foco: foco=%d elegida=%q", ca, m.Foco(), m.Elegida())
	}
}

// tv3cBoton es la celda del boton de texto en el modal abierto.
func tv3cBoton(t *testing.T, ca string, m *tablero.Modelo, ancho int, texto string) (int, int) {
	t.Helper()
	mc := ttModal(t, ca, m, ancho)
	ls := ttLimpia(m)
	for i := range mc.Textos {
		y := mc.Fila + 1 + i
		celdas := ttCeldasDe(ls[y])
		for x := mc.Col; x+len([]rune(texto)) <= mc.Col+mc.Ancho; x++ {
			if strings.Join(celdas[x:x+len([]rune(texto))], "") == texto {
				return x + 1, y
			}
		}
	}
	t.Fatalf("%s: el modal tiene el boton %q:\n%s", ca, texto, ttPantalla(m))
	return -1, -1
}

// CA-719 y CA-575: en un modal, un clic en un boton lo elige sin ejecutarlo
// y la linea de mensajes dice "enter para <boton>"; un clic fuera no hace
// nada.
func TestV3C_CA719_CA575_ElMouseEnLosModales(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	const ancho = 100
	m := tv3cModelo(tv3cConfig(ancho, 30), tv3cSola("sola", "Sola", boardcmd.ColWriter))
	ttTeclas(m, "P")
	x, y := tv3cBoton(t, ca, m, ancho, "Cancelar")
	tv3cClic(t, ca, m, x, y)
	if m.Modal() != tablero.ModalSeguir || ttMensaje(m) != "enter para cancelar" {
		t.Fatalf("%s: el clic en Cancelar lo elige y dice \"enter para cancelar\": modal=%q mensaje=%q", ca, m.Modal(), ttMensaje(m))
	}
	if !ttTiene(m, "[ Cancelar ]") {
		t.Fatalf("%s: Cancelar queda elegido:\n%s", ca, ttPantalla(m))
	}
	x, y = tv3cBoton(t, ca, m, ancho, "Seguir solo ")
	tv3cClic(t, ca, m, x, y)
	if ttMensaje(m) != "enter para seguir solo" || !ttTiene(m, "[ Seguir solo ]") {
		t.Fatalf("%s: el clic en Seguir solo lo elige y dice \"enter para seguir solo\": %q", ca, ttMensaje(m))
	}
	antes := tv3cFoto(m)
	for _, p := range [][2]int{{0, 0}, {1, 15}, {ancho - 1, 15}, {50, 29}} {
		tv3cClic(t, ca, m, p[0], p[1])
		if despues := tv3cFoto(m); despues != antes {
			t.Fatalf("%s: un clic fuera del modal en (%d,%d) no hace nada:\nantes:\n%s\ndespues:\n%s", ca, p[0], p[1], antes, despues)
		}
	}
	// la firma
	m = tv3cBase(tv3cConfig(ancho, 30))
	ttTeclas(m, "A")
	x, y = tv3cBoton(t, ca, m, ancho, "Cancelar")
	tv3cClic(t, ca, m, x, y)
	x, y = tv3cBoton(t, ca, m, ancho, "Aprobar spec")
	tv3cClic(t, ca, m, x, y)
	if m.Modal() != tablero.ModalFirma || ttMensaje(m) != "enter para aprobar spec" {
		t.Fatalf("%s: el clic en Aprobar spec lo elige y dice \"enter para aprobar spec\": %q %q", ca, m.Modal(), ttMensaje(m))
	}
}

// tv3cEstadosDelMouse son los Lugares donde se prueba la propiedad del mouse.
var tv3cEstadosDelMouse = []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarHallazgos, tablero.LugarVerProyecto,
	tablero.LugarVerTarjeta, tablero.LugarVerHallazgo, tablero.LugarVerNada, tablero.LugarBusqueda, tablero.LugarSeguir,
	tablero.LugarFirma, tablero.LugarGuardar, tablero.LugarAcciones, tablero.LugarAyuda, tablero.LugarRegistro,
	tablero.LugarPedirPedido, tablero.LugarPedirSelector}

// CA-719 y CA-575, propiedad: en cada estado, ningun evento del mouse en
// ninguna celda da una orden, abre un modal ni cierra el que esta abierto.
func TestV3C_CA719_CA575_NingunEventoEjecutaNiAbre(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-719"
	for _, l := range tv3cEstadosDelMouse {
		m := tv3cEstado(t, ca, l)
		modal := m.Modal()
		ancho, alto := 100, tv3cAlto(m)
		for _, a := range []tablero.AccionRaton{tablero.RatonRuedaAbajo, tablero.RatonRuedaArriba, tablero.RatonClic} {
			for y := 0; y < alto; y++ {
				for x := 0; x < ancho; x++ {
					ords := m.Raton(tablero.Raton{Accion: a, X: x, Y: y}, ttAhora)
					if len(ords) != 0 {
						t.Fatalf("%s: en %s, el evento %d en (%d,%d) dio las ordenes %#v", ca, l, a, x, y, ords)
					}
					if m.Modal() != modal {
						t.Fatalf("%s: en %s, el evento %d en (%d,%d) cambio el modal de %q a %q", ca, l, a, x, y, modal, m.Modal())
					}
				}
			}
		}
	}
}

// CA-719 y CA-575: lo que el tablero pide y suelta no cambia, ni HOOM_MOUSE=0.
func TestV3C_CA719_CA575_LoQueSePideNoCambia(t *testing.T) {
	defer tv3cSinPanico(t)
	if tablero.SecMouseSi != "\x1b[?1000h\x1b[?1006h" || tablero.SecMouseNo != "\x1b[?1006l\x1b[?1000l" {
		t.Fatalf("CA-719: las secuencias del mouse son las de la v2: %q %q", tablero.SecMouseSi, tablero.SecMouseNo)
	}
	for v, quiero := range map[string]bool{"0": false, "": true, "1": true, "no": true} {
		if got := tablero.ConMouse(ttEnv(map[string]string{"HOOM_MOUSE": v})); got != quiero {
			t.Fatalf("CA-719: ConMouse con HOOM_MOUSE=%q es %v", v, got)
		}
	}
	if !tablero.ConMouse(ttEnv(nil)) {
		t.Fatalf("CA-719: sin HOOM_MOUSE el tablero pide el mouse")
	}
}
