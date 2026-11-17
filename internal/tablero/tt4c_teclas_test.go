// Tests adversariales de la v3 de hoom tablero: las zonas y el foco (CA-706,
// que re-expresa CA-551 y CA-553), elegir y desplazar (CA-707, que
// re-expresa CA-552 y CA-553), y la linea de teclas, la ayuda y la tabla unica
// (CA-718, que re-expresa CA-564).
package tablero_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ===========================================================================
// CA-706: las zonas
// ===========================================================================

// tv3cFocoEn exige la zona con el foco, el Lugar y los signos de los bordes
// (W >= 100, ASCII): la zona con el foco tiene # y =, la otra + y -; la caja
// TU SIGUIENTE PASO nunca tiene el foco.
func tv3cFocoEn(t *testing.T, ca, que string, m *tablero.Modelo, ancho int, z tablero.Zona, l tablero.Lugar) {
	t.Helper()
	if m.Foco() != z || m.Lugar() != l {
		t.Fatalf("%s: %s: foco=%d lugar=%q, se esperaba foco=%d lugar=%q", ca, que, m.Foco(), m.Lugar(), z, l)
	}
	la, _ := tv3cBordeLista(m, ancho)
	da, _ := tv3cBordeDetalle(m, ancho)
	foco, simple := da, la
	if z == tablero.ZonaLista {
		foco, simple = la, da
	}
	fc, sc := ttCeldasDe(foco), ttCeldasDe(simple)
	if len(fc) < 2 || fc[0] != "#" || fc[len(fc)-1] != "#" || !strings.Contains(foco, "=") {
		t.Fatalf("%s: %s: la zona con el foco tiene los signos del foco: %q", ca, que, foco)
	}
	if len(sc) < 2 || sc[0] != "+" || sc[len(sc)-1] != "+" || strings.Contains(simple, "=") {
		t.Fatalf("%s: %s: la otra zona tiene los signos simples: %q", ca, que, simple)
	}
	caja := ttCeldasDe(ttLimpia(m)[1])
	if len(caja) == 0 || caja[0] != "+" {
		t.Fatalf("%s: %s: la caja TU SIGUIENTE PASO nunca tiene el foco: %q", ca, que, ttLimpia(m)[1])
	}
}

// CA-706, CA-551 y CA-553: al abrir el foco esta en la lista; tab y
// shift-tab lo pasan a la otra zona, → y l de la lista al detalle, esc, ← y h
// del detalle a la lista.
func TestV3C_CA706_CA551_CA553_LasDosZonas(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-706"
	m := tv3cBase(tv3cConfig(100, 30))
	tv3cFocoEn(t, ca, "al abrir", m, 100, tablero.ZonaLista, tablero.LugarTarjetas)
	if a, _ := tv3cBordeLista(m, 100); !strings.HasPrefix(a, "#Tarjetas") {
		t.Fatalf("%s: el titulo de la lista es Tarjetas: %q", ca, a)
	}
	pasos := []struct {
		k tablero.Tecla
		z tablero.Zona
		l tablero.Lugar
	}{
		{tablero.TeclaTab, tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{tablero.TeclaTab, tablero.ZonaLista, tablero.LugarTarjetas},
		{tablero.TeclaShiftTab, tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{tablero.TeclaShiftTab, tablero.ZonaLista, tablero.LugarTarjetas},
		{tablero.TeclaDer, tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{tablero.TeclaEsc, tablero.ZonaLista, tablero.LugarTarjetas},
		{"l", tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{tablero.TeclaIzq, tablero.ZonaLista, tablero.LugarTarjetas},
		{"l", tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{"h", tablero.ZonaLista, tablero.LugarTarjetas},
	}
	for _, p := range pasos {
		ttSinOrdenes(t, ca, string(p.k), ttTeclas(m, p.k))
		tv3cFocoEn(t, ca, "tras "+string(p.k), m, 100, p.z, p.l)
	}
	// esc, ← y h en la lista no mueven el foco
	for _, k := range []tablero.Tecla{tablero.TeclaEsc, tablero.TeclaIzq, "h"} {
		ttTeclas(m, k)
		tv3cFocoEn(t, ca, string(k)+" en la lista", m, 100, tablero.ZonaLista, tablero.LugarTarjetas)
	}
	// en la vista Hallazgos tambien
	ttTeclas(m, "H")
	tv3cFocoEn(t, ca, "H", m, 100, tablero.ZonaLista, tablero.LugarHallazgos)
	for _, p := range []struct {
		k tablero.Tecla
		z tablero.Zona
		l tablero.Lugar
	}{
		{tablero.TeclaDer, tablero.ZonaDetalle, tablero.LugarVerHallazgo},
		{"h", tablero.ZonaLista, tablero.LugarHallazgos},
		{tablero.TeclaTab, tablero.ZonaDetalle, tablero.LugarVerHallazgo},
		{tablero.TeclaShiftTab, tablero.ZonaLista, tablero.LugarHallazgos},
	} {
		ttTeclas(m, p.k)
		tv3cFocoEn(t, ca, "Hallazgos "+string(p.k), m, 100, p.z, p.l)
	}
}

// CA-706: 1, 2, 3, 4 y 0 no hacen nada en ningun Lugar y no estan en la
// tabla.
func TestV3C_CA706_LosNumerosNoHacenNada(t *testing.T) {
	defer tv3cSinPanico(t)
	numeros := []tablero.Tecla{"1", "2", "3", "4", "0"}
	for _, a := range tablero.Atajos {
		for _, k := range a.Teclas {
			for _, n := range numeros {
				if k == n {
					t.Fatalf("CA-706: %q no esta en la tabla: fila %+v", n, a)
				}
			}
		}
	}
	for _, l := range []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarHallazgos, tablero.LugarVerProyecto,
		tablero.LugarVerTarjeta, tablero.LugarVerHallazgo, tablero.LugarVerNada, tablero.LugarSeguir, tablero.LugarAyuda} {
		for _, n := range numeros {
			m := tv3cEstado(t, "CA-706", l)
			antes := tv3cFoto(m)
			ttSinOrdenes(t, "CA-706", string(n)+" en "+string(l), ttTeclas(m, n))
			if despues := tv3cFoto(m); despues != antes {
				t.Fatalf("CA-706: %q en %s no hace nada:\nantes:\n%s\ndespues:\n%s", n, l, antes, despues)
			}
		}
	}
}

// ===========================================================================
// CA-707: elegir y desplazar
// ===========================================================================

// CA-707 y CA-552: en la lista ↑ ↓ k j mueven la seleccion una fila, saltando
// las lineas de grupo salvo la de HECHO, y se detienen en los extremos; g y G
// van a la primera y a la ultima; el detalle cambia con cada movimiento.
func TestV3C_CA707_CA552_ElegirEnLaLista(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-707"
	m := ttModelo(tv3cConfig(200, 40), ttTableroPrincipal())
	// TE TOCA: nuc-tenancy (Tu aprobacion) y precios (Arquitecto); CORRIENDO:
	// infra-api; DETENIDA: pagos-caidos; EN COLA: infra-ci-cd y docs-guia;
	// HECHO cerrado ("" elegida)
	orden := []string{"nuc-tenancy", "precios", "infra-api", "pagos-caidos", "infra-ci-cd", "docs-guia", ""}
	titulos := map[string]string{"nuc-tenancy": "Tenancy del nucleo", "precios": "Precios por region",
		"infra-api": "Convenciones de la API", "pagos-caidos": "Pagos caidos", "infra-ci-cd": "CI y CD de la infra",
		"docs-guia": "Guia de la documentacion"}
	mira := func(que, slug string) {
		t.Helper()
		if m.Elegida() != slug {
			t.Fatalf("%s: %s: elegida %q, se esperaba %q", ca, que, m.Elegida(), slug)
		}
		if slug != "" {
			if a, _ := tv3cBordeDetalle(m, 200); !strings.Contains(a, titulos[slug]) {
				t.Fatalf("%s: %s: el detalle sigue a la seleccion sin enter (%s): %q", ca, que, titulos[slug], a)
			}
		}
		if m.Lugar() != tablero.LugarTarjetas || m.Modal() != tablero.SinModal {
			t.Fatalf("%s: %s: elegir no cambia el Lugar ni abre nada: %q %q", ca, que, m.Lugar(), m.Modal())
		}
	}
	mira("al abrir", orden[0])
	for i := 1; i < len(orden); i++ {
		k := []tablero.Tecla{"j", tablero.TeclaAbajo}[i%2]
		ttSinOrdenes(t, ca, string(k), ttTeclas(m, k))
		mira(fmt.Sprintf("%s numero %d", k, i), orden[i])
	}
	if l, _ := ttLineaCon(m, "HECHO 3"); !strings.Contains(l, "> HECHO 3 >") {
		t.Fatalf("%s: la linea de HECHO elegida es \" > HECHO 3 >\": %q", ca, l)
	}
	ttTeclas(m, "j", tablero.TeclaAbajo)
	mira("j al final", "")
	for i := len(orden) - 2; i >= 0; i-- {
		k := []tablero.Tecla{"k", tablero.TeclaArriba}[i%2]
		ttTeclas(m, k)
		mira(fmt.Sprintf("%s hasta %d", k, i), orden[i])
	}
	ttTeclas(m, "k", tablero.TeclaArriba)
	mira("k al principio", orden[0])
	ttTeclas(m, "G")
	mira("G", "")
	ttTeclas(m, "g")
	mira("g", orden[0])
	// con HECHO abierta G va a su ultima tarjeta
	ttTeclas(m, "G", tablero.TeclaEnter, "g", "G")
	if m.Elegida() != "viejo-3" {
		t.Fatalf("%s: con HECHO abierta G va a su ultima tarjeta: %q", ca, m.Elegida())
	}
}

// CA-707 y CA-553: con el foco en el detalle ↑ ↓ k j lo desplazan una linea,
// pgup y pgdn su alto interior, g y G al principio y al final.
func TestV3C_CA707_CA553_DesplazarElDetalle(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-707"
	nuevo := func() *tablero.Modelo {
		m := tv3cBase(tv3cConfig(100, 30))
		ttTeclas(m, tablero.TeclaTab)
		return m
	}
	m := nuevo()
	inicio := tv3cInteriorDetalle(m, 100)
	h := len(inicio)
	for _, k := range []tablero.Tecla{"j", tablero.TeclaAbajo} {
		m = nuevo()
		ttSinOrdenes(t, ca, string(k), ttTeclas(m, k))
		uno := tv3cInteriorDetalle(m, 100)
		for i := 0; i+1 < h; i++ {
			if uno[i] != inicio[i+1] {
				t.Fatalf("%s: %q desplaza el detalle una linea: linea %d %q, se esperaba %q", ca, k, i, uno[i], inicio[i+1])
			}
		}
		if m.Elegida() != "nuc-tenancy" {
			t.Fatalf("%s: en el detalle %q no mueve la seleccion: %q", ca, k, m.Elegida())
		}
		for _, v := range []tablero.Tecla{"k", tablero.TeclaArriba} {
			ttTeclas(m, v)
			if got := tv3cInteriorDetalle(m, 100); strings.Join(got, "\n") != strings.Join(inicio, "\n") {
				t.Fatalf("%s: %q vuelve una linea", ca, v)
			}
			ttTeclas(m, k)
		}
	}
	// pgdn es el alto interior en lineas
	a, b := nuevo(), nuevo()
	ttTeclas(a, tablero.TeclaPgAbajo)
	for i := 0; i < h; i++ {
		ttTeclas(b, "j")
	}
	if strings.Join(tv3cInteriorDetalle(a, 100), "\n") != strings.Join(tv3cInteriorDetalle(b, 100), "\n") {
		t.Fatalf("%s: pgdn desplaza el alto interior del detalle (%d lineas)", ca, h)
	}
	ttTeclas(a, tablero.TeclaPgArriba)
	if strings.Join(tv3cInteriorDetalle(a, 100), "\n") != strings.Join(inicio, "\n") {
		t.Fatalf("%s: pgup vuelve el alto interior", ca)
	}
	// G al final, g al principio
	ttTeclas(a, "G")
	fin := tv3cInteriorDetalle(a, 100)
	if strings.Join(fin, "\n") == strings.Join(inicio, "\n") {
		t.Fatalf("%s: G va al final del detalle (el documento es mas largo que la vista)", ca)
	}
	ttTeclas(a, "j")
	if strings.Join(tv3cInteriorDetalle(a, 100), "\n") != strings.Join(fin, "\n") {
		t.Fatalf("%s: despues del final j no desplaza", ca)
	}
	// la ultima linea del documento es la del evento mas nuevo de la
	// narracion (parte G, con la fila de trabajo de tv3cBase)
	if !strings.Contains(fin[len(fin)-1], "evento-099") {
		t.Fatalf("%s: G deja la ultima linea del documento (evento-099) abajo: %q", ca, fin)
	}
	ttTeclas(a, "g")
	if strings.Join(tv3cInteriorDetalle(a, 100), "\n") != strings.Join(inicio, "\n") {
		t.Fatalf("%s: g vuelve al principio", ca)
	}
}

// CA-707: la lista se desplaza lo menos posible para que la tarjeta elegida
// se vea entera (sus dos lineas con W >= 100), y su borde de abajo dice "<n>
// de <m>" (las filas que se eligen: las tarjetas y la linea de HECHO).
func TestV3C_CA707_LaListaMuestraLaElegidaEntera(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-707"
	m := tv3cModelo(tv3cConfig(100, 24), tv3cMuchas(15)...)
	nDeM := func(n int) {
		t.Helper()
		_, abajo := tv3cBordeLista(m, 100)
		if !strings.HasSuffix(abajo, fmt.Sprintf("%d de 16=#", n)) {
			t.Fatalf("%s: el borde de abajo de la lista dice \"%d de 16\" antes del ultimo signo: %q", ca, n, abajo)
		}
	}
	nDeM(1)
	for i := 0; i < 6; i++ {
		ttTeclas(m, "j")
	}
	nDeM(7)
	ls := tv3cInteriorLista(m, 100)
	if len(ls) != 14 {
		t.Fatalf("%s: precondicion del spec: a 100x24 la lista tiene 14 lineas interiores, tiene %d", ca, len(ls))
	}
	if !strings.HasPrefix(ls[13-1], " > Tarjeta 07") || ls[13] != "   plain 07" {
		t.Fatalf("%s: al bajar, la ultima linea de la elegida es la ultima a la vista: %q", ca, ls)
	}
	if !strings.HasPrefix(ls[0], "   Tarjeta 01") {
		t.Fatalf("%s: al bajar la lista se desplaza lo menos posible (una linea): %q", ca, ls)
	}
	ttTeclas(m, "G")
	nDeM(16)
	ls = tv3cInteriorLista(m, 100)
	if ls[13] != " > HECHO 0 >" {
		t.Fatalf("%s: G elige la linea de HECHO, la ultima a la vista: %q", ca, ls)
	}
	for i := 0; i < 8; i++ {
		ttTeclas(m, "k")
	}
	nDeM(8)
	ls = tv3cInteriorLista(m, 100)
	if !strings.HasPrefix(ls[0], " > Tarjeta 08") || ls[1] != "   plain 08" {
		t.Fatalf("%s: al subir, la primera linea de la elegida es la primera a la vista: %q", ca, ls)
	}
	// cuando todo entra no hay "n de m"
	m = tv3cModelo(tv3cConfig(100, 24), tv3cMuchas(2)...)
	if _, abajo := tv3cBordeLista(m, 100); strings.Contains(abajo, " de ") || strings.Contains(abajo, "1 de") {
		t.Fatalf("%s: con todo a la vista el borde no dice n de m: %q", ca, abajo)
	}
}

// CA-707: en la vista Hallazgos ↑ ↓ k j eligen el hallazgo y g G van al
// primero y al ultimo; el detalle sigue a la seleccion.
func TestV3C_CA707_ElegirEnLaVistaHallazgos(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-707"
	m := tv3cBase(tv3cConfig(200, 40))
	ttTeclas(m, "H")
	archivo := func(que, ruta string, linea int) {
		t.Helper()
		r, l := m.ArchivoElegido()
		if r != ruta || l != linea {
			t.Fatalf("%s: %s: el detalle muestra el hallazgo de %s:%d, muestra %s:%d", ca, que, ruta, linea, r, l)
		}
	}
	// high, medium, low
	archivo("al abrir", "internal/api/router.go", 41)
	ttTeclas(m, "j")
	archivo("j", "internal/api/medio.go", 3)
	ttTeclas(m, tablero.TeclaAbajo)
	archivo("↓", "internal/tenant/tenant.go", 7)
	ttTeclas(m, "j")
	archivo("j al final", "internal/tenant/tenant.go", 7)
	ttTeclas(m, "k")
	archivo("k", "internal/api/medio.go", 3)
	ttTeclas(m, tablero.TeclaArriba, tablero.TeclaArriba)
	archivo("↑ al principio", "internal/api/router.go", 41)
	ttTeclas(m, "G")
	archivo("G", "internal/tenant/tenant.go", 7)
	ttTeclas(m, "g")
	archivo("g", "internal/api/router.go", 41)
	if m.Lugar() != tablero.LugarHallazgos || m.Vista() != tablero.VistaHallazgos {
		t.Fatalf("%s: elegir no sale de la vista: %q %q", ca, m.Lugar(), m.Vista())
	}
}

// ===========================================================================
// CA-718: la linea de teclas
// ===========================================================================

// tv3cTeclasDe es la linea de teclas esperada: un espacio y las pistas unidas
// por " · ", con las flechas en ASCII.
func tv3cTeclasDe(pistas ...string) string {
	return tv3cAscii(" " + strings.Join(pistas, " · "))
}

var (
	tv3cPistasLista   = []string{"n tu siguiente paso", "↑↓ elegir", "tab detalle", "espacio acciones", "H hallazgos", "v proyecto", "? todas las teclas"}
	tv3cPistasDetalle = []string{"n tu siguiente paso", "↑↓ desplazar", "esc volver", "espacio acciones", "? todas las teclas"}
)

func tv3cCon(primera string, resto []string) []string {
	return append([]string{primera}, resto...)
}

func tv3cSin(quitar string, resto []string) []string {
	var out []string
	for _, p := range resto {
		if p != quitar {
			out = append(out, p)
		}
	}
	return out
}

// CA-718 y CA-564: la linea de teclas empieza con la pista de enter para lo
// elegido y sigue con las pistas del spec para la lista, el detalle o la vista
// Hallazgos; sin tarjeta se quitan las de la tarjeta.
func TestV3C_CA718_CA564_LaLineaDeTeclas(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-718"
	cfg := tv3cConfig(200, 40)
	casos := []struct {
		nombre string
		m      func() *tablero.Modelo
		quiero string
	}{
		{"lista en Tu aprobacion", func() *tablero.Modelo { return tv3cBase(cfg) },
			tv3cTeclasDe(tv3cCon("enter aprobar spec", tv3cPistasLista)...)},
		{"detalle en Tu aprobacion", func() *tablero.Modelo { m := tv3cBase(cfg); ttTeclas(m, tablero.TeclaTab); return m },
			tv3cTeclasDe(tv3cCon("enter aprobar spec", tv3cPistasDetalle)...)},
		{"vista Hallazgos", func() *tablero.Modelo { m := tv3cBase(cfg); ttTeclas(m, "H"); return m },
			tv3cTeclasDe("enter ver", "↑↓ elegir", "e editor", "esc volver", "? todas las teclas")},
		{"HECHO cerrada", func() *tablero.Modelo { m := tv3cBase(cfg); ttTeclas(m, "G"); return m },
			tv3cTeclasDe(tv3cCon("enter abrir", tv3cSin("espacio acciones", tv3cPistasLista))...)},
		{"HECHO abierta", func() *tablero.Modelo { m := tv3cBase(cfg); ttTeclas(m, "G", tablero.TeclaEnter); return m },
			tv3cTeclasDe(tv3cCon("enter cerrar", tv3cSin("espacio acciones", tv3cPistasLista))...)},
		{"tarjeta sin paso (corriendo)", func() *tablero.Modelo { return tv3cModelo(cfg, ttInfraAPI(true)) },
			tv3cTeclasDe(tv3cPistasLista...)},
		{"tarjeta con Seguir solo", func() *tablero.Modelo { return tv3cModelo(cfg, tv3cSola("sola", "Sola", boardcmd.ColWriter)) },
			tv3cTeclasDe(tv3cCon("enter seguir solo", tv3cPistasLista)...)},
		{"tarjeta con pedir", func() *tablero.Modelo { return tv3cModelo(cfg, ttTarjetaCon(ttAccionWriter())) },
			tv3cTeclasDe(tv3cCon("enter pedir al writer", tv3cPistasLista)...)},
		{"lista vacia", func() *tablero.Modelo { return tv3cModelo(cfg) },
			tv3cTeclasDe(tv3cSin("espacio acciones", tv3cPistasLista)...)},
		{"detalle del proyecto", func() *tablero.Modelo { m := tv3cBase(cfg); ttTeclas(m, "v"); return m },
			tv3cTeclasDe("n tu siguiente paso", "↑↓ desplazar", "esc volver", "? todas las teclas")},
		{"detalle sin nada", func() *tablero.Modelo { m := tv3cBase(cfg); ttTeclas(m, "G", tablero.TeclaTab); return m },
			tv3cTeclasDe("n tu siguiente paso", "esc volver", "? todas las teclas")},
		{"modal Seguir solo", func() *tablero.Modelo {
			m := tv3cModelo(cfg, tv3cSola("sola", "Sola", boardcmd.ColWriter))
			ttTeclas(m, "P")
			return m
		}, tv3cTeclasDe("←→ boton", "enter ejecutar", "esc cerrar")},
		{"modal Registro", func() *tablero.Modelo { m := tv3cBase(cfg); ttTeclas(m, "L"); return m },
			tv3cTeclasDe("↑↓ desplazar", "esc cerrar")},
	}
	for _, c := range casos {
		if got := ttLineaTeclas(c.m()); got != c.quiero {
			t.Fatalf("%s: [%s] la linea de teclas\n got: %q\nwant: %q", ca, c.nombre, got, c.quiero)
		}
	}
	// con UTF-8 las flechas son las de la tabla
	u := ttConfig(200, 40, true)
	m := ttModelo(u, ttTableroDorado())
	if got, quiero := ttLineaTeclas(m), " "+strings.Join(tv3cCon("enter aprobar spec", tv3cPistasLista), " · "); got != quiero {
		t.Fatalf("%s: con UTF-8\n got: %q\nwant: %q", ca, got, quiero)
	}
}

// CA-718: con color la pista de enter va en negrita (1) y es la primera.
func TestV3C_CA718_LaPistaDeEnterEnNegrita(t *testing.T) {
	defer tv3cSinPanico(t)
	m := tv3cBase(tv3cConfigColor(200, 40))
	raw := m.Pantalla()[39]
	if !strings.HasPrefix(ttSinSGR(raw), " enter aprobar spec · ") {
		t.Fatalf("CA-718: la pista de enter es la primera: %q", ttSinSGR(raw))
	}
	if got := ttEstiloDe(raw, "enter aprobar spec"); got != "\x1b[1m" {
		t.Fatalf("CA-718: la pista de enter va en negrita (\\x1b[1m), va en %q: %q", got, raw)
	}
	if got := ttEstiloDe(raw, "n tu siguiente paso"); got != "" {
		t.Fatalf("CA-718: las demas pistas no llevan estilo: %q", got)
	}
}

// tv3cAcortar es la regla de la v2 para la linea de teclas que no entra: la
// ultima "? todas las teclas" pasa a "? todas", y despues se quitan las
// pistas de antes de la ultima, de atras para adelante, nunca la primera si
// es la de enter.
func tv3cAcortar(pistas []string, ancho int, conEnter bool) string {
	linea := func(p []string) string { return " " + strings.Join(p, " · ") }
	p := append([]string(nil), pistas...)
	if ttCeldas(linea(p)) <= ancho {
		return linea(p)
	}
	if p[len(p)-1] == "? todas las teclas" {
		p[len(p)-1] = "? todas"
	}
	for ttCeldas(linea(p)) > ancho {
		i := len(p) - 2
		if i < 0 || (i == 0 && conEnter) {
			break
		}
		p = append(p[:i], p[i+1:]...)
	}
	return linea(p)
}

// CA-718 y CA-564: la linea que no entra se acorta con la regla de la v2 sin
// quitar nunca la pista de enter, en todos los anchos de 60 a 200, en la
// lista y en el detalle, con y sin enter.
func TestV3C_CA718_CA564_LaLineaSeAcorta(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-718"
	// el ejemplo de doc.go a 100 columnas
	m := ttModelo(tv3cConfig(100, 30), ttTableroDorado())
	if got, quiero := ttLineaTeclas(m), " enter aprobar spec · n tu siguiente paso · ^v elegir · tab detalle · espacio acciones · ? todas"; got != quiero {
		t.Fatalf("%s: a 100 columnas\n got: %q\nwant: %q", ca, got, quiero)
	}
	for ancho := 60; ancho <= 200; ancho++ {
		for _, c := range []struct {
			nombre   string
			m        func() *tablero.Modelo
			pistas   []string
			conEnter bool
		}{
			{"lista con enter", func() *tablero.Modelo { return ttModelo(tv3cConfig(ancho, 30), ttTableroDorado()) },
				tv3cCon("enter aprobar spec", tv3cPistasLista), true},
			{"detalle con enter", func() *tablero.Modelo {
				m := ttModelo(tv3cConfig(ancho, 30), ttTableroDorado())
				ttTeclas(m, tablero.TeclaTab)
				return m
			}, tv3cCon("enter aprobar spec", tv3cPistasDetalle), true},
			{"lista sin enter", func() *tablero.Modelo { return tv3cModelo(tv3cConfig(ancho, 30), ttInfraAPI(true)) },
				tv3cPistasLista, false},
		} {
			var ascii []string
			for _, p := range c.pistas {
				ascii = append(ascii, tv3cAscii(p))
			}
			quiero := tv3cAcortar(ascii, ancho, c.conEnter)
			got := ttLineaTeclas(c.m())
			if got != quiero {
				t.Fatalf("%s: [%s a %d columnas] la linea de teclas\n got: %q\nwant: %q", ca, c.nombre, ancho, got, quiero)
			}
			if ttCeldas(got) > ancho {
				t.Fatalf("%s: [%s a %d] la linea entra en W celdas: %q", ca, c.nombre, ancho, got)
			}
		}
	}
}

// ===========================================================================
// CA-718: la ayuda
// ===========================================================================

// tv3cGrupos son los grupos de la ayuda con los Lugares de sus filas.
var tv3cGrupos = map[string][]tablero.Lugar{
	"Tarjetas":  {tablero.LugarTarjetas, tablero.LugarBusqueda},
	"Detalle":   tablero.LugaresVer,
	"Hallazgos": {tablero.LugarHallazgos},
	"Modales":   tablero.LugaresModales,
}

// tv3cAyuda abre la ayuda (en una pantalla donde entra entera) y devuelve los
// titulos de sus grupos en orden y, por grupo, sus lineas logicas (una fila
// partida en varias lineas va unida) y la primera linea fisica de cada una.
func tv3cAyuda(t *testing.T, ca string, m *tablero.Modelo, ancho int) ([]string, map[string][]string, map[string][]string, ttModalCaja) {
	t.Helper()
	ttTeclas(m, "?")
	if m.Modal() != tablero.ModalAyuda {
		t.Fatalf("%s: ? abre la ayuda: %q", ca, m.Modal())
	}
	mc := ttModal(t, ca, m, ancho)
	if !strings.Contains(mc.Arriba, " Todas las teclas ") {
		t.Fatalf("%s: la ayuda se titula \"Todas las teclas\": %q", ca, mc.Arriba)
	}
	if strings.Contains(mc.Abajo, " de ") {
		t.Fatalf("%s: precondicion del spec: la ayuda entra entera en la pantalla: %q", ca, mc.Abajo)
	}
	var titulos []string
	logicas, primeras := map[string][]string{}, map[string][]string{}
	grupo := ""
	for i, l := range mc.Textos {
		switch {
		case l == "":
			if i+1 < len(mc.Textos) && mc.Textos[i+1] == "" {
				t.Fatalf("%s: una sola linea vacia entre grupos: %q", ca, mc.Textos)
			}
		case !strings.HasPrefix(l, " "):
			grupo = l
			titulos = append(titulos, l)
			if i > 0 && mc.Textos[i-1] != "" {
				t.Fatalf("%s: el grupo %q va despues de una linea vacia: %q", ca, l, mc.Textos)
			}
		case strings.HasPrefix(l, "   "):
			ls := logicas[grupo]
			if len(ls) == 0 {
				t.Fatalf("%s: una continuacion sin fila: %q", ca, l)
			}
			ls[len(ls)-1] += " " + strings.TrimSpace(l)
		default:
			logicas[grupo] = append(logicas[grupo], l)
			primeras[grupo] = append(primeras[grupo], l)
		}
		if ttCeldas(l) > mc.Ancho-6 {
			t.Fatalf("%s: cada linea de la ayuda entra en el modal: %q", ca, l)
		}
	}
	return titulos, logicas, primeras, mc
}

func tv3cRelleno(s string, n int) string {
	if c := ttCeldas(s); c < n {
		return s + strings.Repeat(" ", n-c)
	}
	return s
}

// CA-718 y CA-564: ? abre la ayuda con los grupos Tarjetas, Detalle,
// Hallazgos, Modales y Mouse, primero el de donde esta el foco; cada fila de
// la tabla esta una vez, en un grupo donde funciona, con su Muestra rellena a
// 18 y su Hace (y su Verbo); los numeros no estan.
func TestV3C_CA718_CA564_LaAyuda(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-718"
	const ancho, alto = 200, 400
	casos := []struct {
		nombre string
		prep   []tablero.Tecla
		orden  []string
	}{
		{"foco en la lista", nil, []string{"Tarjetas", "Detalle", "Hallazgos", "Modales", "Mouse"}},
		{"foco en el detalle", []tablero.Tecla{tablero.TeclaTab}, []string{"Detalle", "Tarjetas", "Hallazgos", "Modales", "Mouse"}},
		{"vista Hallazgos", []tablero.Tecla{"H"}, []string{"Hallazgos", "Tarjetas", "Detalle", "Modales", "Mouse"}},
		{"detalle del proyecto", []tablero.Tecla{"v"}, []string{"Detalle", "Tarjetas", "Hallazgos", "Modales", "Mouse"}},
	}
	for _, c := range casos {
		m := tv3cBase(tv3cConfig(ancho, alto))
		ttTeclas(m, c.prep...)
		titulos, logicas, primeras, _ := tv3cAyuda(t, ca, m, ancho)
		if strings.Join(titulos, ",") != strings.Join(c.orden, ",") {
			t.Fatalf("%s: [%s] los grupos de la ayuda en orden %q, fueron %q", ca, c.nombre, c.orden, titulos)
		}
		// cada fila de la tabla, en un grupo donde funciona
		filas := 0
		for g := range tv3cGrupos {
			filas += len(logicas[g])
		}
		if filas != len(tablero.Atajos) {
			t.Fatalf("%s: [%s] una linea por fila de la tabla (%d), hay %d", ca, c.nombre, len(tablero.Atajos), filas)
		}
		for _, a := range tablero.Atajos {
			quiero := strings.Fields(tv3cAscii(a.Muestra) + " " + a.Hace + " " + a.Verbo)
			prefijo := "  " + tv3cRelleno(tv3cAscii(a.Muestra), 18)
			esta := false
			for g, lugares := range tv3cGrupos {
				funciona := false
				for _, l := range lugares {
					for _, x := range a.Lugares {
						funciona = funciona || x == l
					}
				}
				if !funciona {
					continue
				}
				for i, l := range logicas[g] {
					if strings.Join(strings.Fields(l), " ") == strings.Join(quiero, " ") && strings.HasPrefix(primeras[g][i], prefijo) {
						esta = true
					}
				}
			}
			if !esta {
				t.Fatalf("%s: [%s] la fila %q (%s) esta en la ayuda como %q, en un grupo donde funciona:\n%q",
					ca, c.nombre, a.Muestra, a.Hace, prefijo+a.Hace, logicas)
			}
		}
		for g := range tv3cGrupos {
			for _, l := range logicas[g] {
				if f := strings.Fields(l); len(f) > 0 && strings.ContainsAny(f[0], "01234") && len(f[0]) == 1 {
					t.Fatalf("%s: los numeros no estan en la ayuda: %q", ca, l)
				}
			}
		}
		// el grupo Mouse
		quiero := []string{
			"  clic              foco a la lista o al detalle; en una tarjeta, la elige",
			"  clic en la caja   elige la tarjeta de tu siguiente paso",
			"  rueda             mueve la seleccion de la lista; en el detalle y en la ayuda, desplaza",
			"  clic en un modal  enfoca un campo, elige una opcion o un boton",
			"  para seleccionar texto con el mouse, manten Option (macOS) o Shift",
		}
		mouse := logicas["Mouse"]
		if len(mouse) != len(quiero) {
			t.Fatalf("%s: [%s] el grupo Mouse con el mouse pedido tiene %d lineas: %q", ca, c.nombre, len(quiero), mouse)
		}
		for i, q := range quiero {
			if strings.Join(strings.Fields(mouse[i]), " ") != strings.Join(strings.Fields(q), " ") ||
				!strings.HasPrefix(primeras["Mouse"][i], q[:20]) {
				t.Fatalf("%s: [%s] la linea %d del grupo Mouse es %q, fue %q", ca, c.nombre, i, q, primeras["Mouse"][i])
			}
		}
	}
	// sin el mouse
	cfg := tv3cConfig(ancho, alto)
	cfg.Mouse = false
	_, logicas, _, _ := tv3cAyuda(t, ca, tv3cBase(cfg), ancho)
	if strings.Join(logicas["Mouse"], "|") != "  apagado: HOOM_MOUSE=0" {
		t.Fatalf("%s: sin el mouse el grupo Mouse dice \"  apagado: HOOM_MOUSE=0\": %q", ca, logicas["Mouse"])
	}
}

// ===========================================================================
// CA-718: una sola tabla para la linea, la ayuda y el despacho
// ===========================================================================

// tv3cTeclasCandidatas son las teclas que se prueban: las que no son texto y
// las letras, numeros y signos de un teclado (ctrl-c no: esta en todos).
var tv3cTeclasCandidatas = func() []tablero.Tecla {
	ks := []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaEsc, tablero.TeclaTab, tablero.TeclaShiftTab, tablero.TeclaArriba,
		tablero.TeclaAbajo, tablero.TeclaIzq, tablero.TeclaDer, tablero.TeclaBorrar, tablero.TeclaCtrlU, tablero.TeclaCtrlE,
		tablero.TeclaPgArriba, tablero.TeclaPgAbajo, tablero.TeclaEspacio}
	for _, r := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&()*+,-.:;<=>@[]^_{|}~ñé?/\"'`\\" {
		ks = append(ks, tablero.Tecla(string(r)))
	}
	return ks
}()

// tv3cEnLaTabla dice si la tecla tiene una fila en el Lugar l (una tecla de
// texto, si la fila tiene TeclaTexto).
func tv3cEnLaTabla(l tablero.Lugar, k tablero.Tecla) bool {
	texto := len([]rune(string(k))) == 1 && k != tablero.TeclaEspacio || k == tablero.TeclaEspacio
	for _, a := range tablero.AtajosDe(l) {
		for _, x := range a.Teclas {
			if x == k || (x == tablero.TeclaTexto && texto) {
				return true
			}
		}
	}
	return false
}

// CA-718 y CA-564, propiedad negativa: en cada Lugar, una tecla sin fila en
// la tabla no hace nada (ni la pantalla, ni el Lugar, ni el Registro, ni
// ordenes).
func TestV3C_CA718_CA564_NingunaTeclaFueraDeLaTabla(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-718"
	for _, l := range tv3cLugaresProbados {
		for _, k := range tv3cTeclasCandidatas {
			if tv3cEnLaTabla(l, k) {
				continue
			}
			m := tv3cEstado(t, ca, l)
			antes := tv3cFoto(m)
			ords := ttTeclas(m, k)
			if len(ords) != 0 {
				t.Fatalf("%s: %q no tiene fila en %s y dio las ordenes %#v", ca, k, l, ords)
			}
			if despues := tv3cFoto(m); despues != antes {
				t.Fatalf("%s: %q no tiene fila en %s y cambio el modelo:\nantes:\n%s\ndespues:\n%s", ca, k, l, antes, despues)
			}
		}
	}
}

// tv3cPrepara deja el estado de l donde cada tecla de sus filas puede tener
// efecto: la seleccion o el desplazamiento lejos de los extremos.
func tv3cPrepara(m *tablero.Modelo, l tablero.Lugar) {
	switch l {
	case tablero.LugarTarjetas, tablero.LugarHallazgos, tablero.LugarVerTarjeta, tablero.LugarVerProyecto,
		tablero.LugarVerHallazgo, tablero.LugarAcciones, tablero.LugarAyuda:
		ttTeclas(m, "j")
	case tablero.LugarRegistro:
		ttTeclas(m, "k")
	case tablero.LugarBusqueda:
		ttTeclas(m, "e", tablero.TeclaAbajo)
	}
}

// CA-718 y CA-564, propiedad: en cada Lugar, cada tecla de cada fila de la
// tabla tiene efecto (cambia lo que se ve o da una orden). No se prueban los
// Lugares del modal Pedir, que siguen siendo los de la v2, ni v en el detalle
// del proyecto (ya lo muestra).
func TestV3C_CA718_CA564_TodaTeclaDeLaTablaTieneEfecto(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-718"
	for _, l := range []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarHallazgos, tablero.LugarVerProyecto,
		tablero.LugarVerTarjeta, tablero.LugarVerHallazgo, tablero.LugarVerNada, tablero.LugarBusqueda,
		tablero.LugarFirma, tablero.LugarGuardar, tablero.LugarSeguir, tablero.LugarAcciones, tablero.LugarAyuda, tablero.LugarRegistro} {
		for _, a := range tablero.AtajosDe(l) {
			for _, k := range a.Teclas {
				if l == tablero.LugarVerProyecto && k == "v" {
					continue
				}
				m := tv3cEstado(t, ca, l)
				tv3cPrepara(m, l)
				antes := tv3cFoto(m)
				ords := tv3cApretar(m, k)
				if len(ords) == 0 && tv3cFoto(m) == antes {
					t.Fatalf("%s: %q (%s) esta en la tabla en %s y no tuvo efecto:\n%s", ca, k, a.Hace, l, antes)
				}
			}
		}
	}
}
