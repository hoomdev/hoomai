// La pantalla de paneles (spec tablero-con-paneles: CA-550 el diseno y el
// pliegue, CA-551 el foco, CA-552 la seleccion, CA-553 lo que muestra [0],
// CA-577 los anchos): las cajas se leen de la pantalla por sus titulos
// (ttCajas) y se comparan con las reglas de doc.go ("The layout", "The
// panels"), calculadas aca desde el contrato.
package tablero_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- las alturas del contrato

// ttAlto es la altura de un panel de la izquierda (1 plegado).
type ttAlto struct {
	h       int
	plegado bool
}

// ttAlturasDosColumnas son las alturas de [1], [2], [3] y [4] con dos
// columnas (doc.go, "The layout"): h1 = 6; h3 = filas + 2 entre 3 y 7; h4 =
// max(3, (B - h1 - h3) / 3); h2 = el resto; mientras h2 < 3 se pliegan los
// que no tienen el foco, en el orden [1], [4], [3], con h = 1.
func ttAlturasDosColumnas(b, filas3 int, foco tablero.Panel) map[tablero.Panel]ttAlto {
	h3n := filas3 + 2
	if filas3 == 0 {
		h3n = 3
	}
	if h3n > 7 {
		h3n = 7
	}
	pleg := map[tablero.Panel]bool{}
	calc := func() (int, int, int, int) {
		h1, h3, h4 := 6, h3n, 0
		if pleg[tablero.PanelProyecto] {
			h1 = 1
		}
		if pleg[tablero.PanelCorriendo] {
			h3 = 1
		}
		if pleg[tablero.PanelHallazgos] {
			h4 = 1
		} else {
			h4 = (b - h1 - h3) / 3
			if h4 < 3 {
				h4 = 3
			}
		}
		return h1, b - h1 - h3 - h4, h3, h4
	}
	h1, h2, h3, h4 := calc()
	for _, p := range []tablero.Panel{tablero.PanelProyecto, tablero.PanelHallazgos, tablero.PanelCorriendo} {
		if h2 >= 3 {
			break
		}
		if p == foco {
			continue
		}
		pleg[p] = true
		h1, h2, h3, h4 = calc()
	}
	return map[tablero.Panel]ttAlto{
		tablero.PanelProyecto:  {h1, pleg[tablero.PanelProyecto]},
		tablero.PanelTarjetas:  {h2, false},
		tablero.PanelCorriendo: {h3, pleg[tablero.PanelCorriendo]},
		tablero.PanelHallazgos: {h4, pleg[tablero.PanelHallazgos]},
	}
}

// ttTableroConCorriendo es un tablero con n tarjetas en Writer con running y
// dos en Backlog.
func ttTableroConCorriendo(n int) boardcmd.Board {
	cards := []boardcmd.Card{ttBacklog("uno", "Uno"), ttBacklog("dos", "Dos")}
	for i := 0; i < n; i++ {
		c := ttCard(fmt.Sprintf("corre-%d", i), "Corre", boardcmd.ColWriter, "corre", ttSeg("s", "s", boardcmd.SegHecho, "d"))
		c.Running = &boardcmd.Running{EnvelopeID: fmt.Sprintf("env-%d", i), Role: "writer", Stage: "run", Step: 1, Steps: 2,
			StartedAt: ttAhora.Add(-time.Minute)}
		cards = append(cards, c)
	}
	return ttTablero(cards...)
}

// ttTeclaDe es la tecla que le da el foco a p.
func ttTeclaDe(p tablero.Panel) tablero.Tecla { return tablero.Tecla(fmt.Sprintf("%d", int(p))) }

// ttRevisarDosColumnas exige el diseno de dos columnas de doc.go en m.
func ttRevisarDosColumnas(t *testing.T, que string, m *tablero.Modelo, ancho, alto, filas3 int, foco tablero.Panel) {
	t.Helper()
	ttForma(t, "CA-550", m, ancho, alto, false)
	b := alto - 2
	l, r := ancho/3, ancho-ancho/3
	hs := ttAlturasDosColumnas(b, filas3, foco)
	cs := ttCajas(m)
	fila := 0
	for _, p := range []tablero.Panel{tablero.PanelProyecto, tablero.PanelTarjetas, tablero.PanelCorriendo, tablero.PanelHallazgos} {
		n := ttNombrePanel(p)
		c, ok := cs[n]
		if !ok {
			t.Fatalf("CA-550: %s: la pantalla tiene %s:\n%s", que, n, ttPantalla(m))
		}
		h := hs[p]
		if c.Fila != fila || c.Col != 0 || c.Ancho != l || c.Alto != h.h || c.Plegado != h.plegado {
			t.Fatalf("CA-550: %s: %s esta en la linea %d, celda 0, con %d celdas (Ancho/3) y %d lineas (plegado %v); esta en %d,%d con %d celdas y %d lineas (plegado %v):\n%s",
				que, n, fila+1, l, h.h, h.plegado, c.Fila+1, c.Col, c.Ancho, c.Alto, c.Plegado, ttPantalla(m))
		}
		fila += h.h
	}
	if fila != b {
		t.Fatalf("CA-550: %s: la columna de la izquierda ocupa las B=%d lineas, ocupa %d", que, b, fila)
	}
	hReg := 4
	if alto >= 24 {
		hReg = 6
	}
	c0, reg := cs["[0]"], cs["Registro"]
	if c0.Fila != 0 || c0.Col != l || c0.Ancho != r || c0.Alto != b-hReg || c0.Plegado {
		t.Fatalf("CA-550: %s: [0] esta arriba a la derecha: linea 1, celda %d, %d celdas, %d lineas; esta en %d,%d con %d celdas y %d lineas:\n%s",
			que, l, r, b-hReg, c0.Fila+1, c0.Col, c0.Ancho, c0.Alto, ttPantalla(m))
	}
	if reg.Fila != b-hReg || reg.Col != l || reg.Ancho != r || reg.Alto != hReg || reg.Plegado || reg.Foco {
		t.Fatalf("CA-550: %s: el Registro esta abajo a la derecha con %d lineas (sin foco): esta en %d,%d con %d celdas y %d lineas (foco %v):\n%s",
			que, hReg, reg.Fila+1, reg.Col, reg.Ancho, reg.Alto, reg.Foco, ttPantalla(m))
	}
}

// CA-550: con 80 columnas o mas, dos columnas: la izquierda de Ancho/3 celdas
// con [1], [2], [3] y [4] apilados (h1 = 6, h3 entre 3 y 7, h4 = un tercio
// de lo que queda con minimo 3, h2 el resto), la derecha con [0] arriba y el
// Registro abajo (6 lineas con 24 filas o mas, 4 por debajo: decision de la
// persona). En varios tamanos, con 0 a 7 trabajos en [3].
func TestCA550_DosColumnas(t *testing.T) {
	for _, tam := range [][2]int{{80, 24}, {81, 24}, {100, 30}, {120, 32}, {200, 50}, {80, 23}, {95, 18}, {80, 40}} {
		for _, n := range []int{0, 1, 3, 5, 9} {
			que := fmt.Sprintf("%dx%d con %d corriendo", tam[0], tam[1], n)
			m := ttModelo(ttConfig(tam[0], tam[1], false), ttTableroConCorriendo(n))
			ttRevisarDosColumnas(t, que, m, tam[0], tam[1], n, tablero.PanelTarjetas)
		}
	}
	// con UTF-8 las mismas cajas
	m := ttModelo(ttConfig(120, 32, true), ttTableroConCorriendo(2))
	ttRevisarDosColumnas(t, "120x32 con UTF-8", m, 120, 32, 2, tablero.PanelTarjetas)
}

// CA-550 (pliegue): con dos columnas y poca altura, los paneles de la
// izquierda sin el foco se pliegan a una linea en el orden [1], [4], [3]
// hasta que [2] tiene sus 3 lineas; el panel con el foco nunca se pliega y
// [2] no se pliega en dos columnas.
func TestCA550_PliegueEnDosColumnas(t *testing.T) {
	casos := []struct {
		ancho, alto, n int
		foco           tablero.Panel
	}{
		{80, 15, 0, tablero.PanelTarjetas},   // se pliega [1]
		{80, 15, 0, tablero.PanelProyecto},   // [1] tiene el foco: se pliega [4]
		{80, 15, 5, tablero.PanelTarjetas},   // [1] y [4]
		{80, 15, 5, tablero.PanelHallazgos},  // [1] y [3]
		{80, 15, 5, tablero.PanelCorriendo},  // [1] y [4]
		{80, 16, 9, tablero.PanelProyecto},   // [4] y [3]
		{120, 15, 9, tablero.PanelPrincipal}, // con el foco en [0], cualquiera
		{80, 17, 1, tablero.PanelTarjetas},
		{100, 19, 3, tablero.PanelHallazgos},
	}
	for _, c := range casos {
		que := fmt.Sprintf("%dx%d con %d corriendo y el foco en [%d]", c.ancho, c.alto, c.n, int(c.foco))
		m := ttModelo(ttConfig(c.ancho, c.alto, false), ttTableroConCorriendo(c.n))
		m.Tecla(ttTeclaDe(c.foco), ttAhora)
		if m.Foco() != c.foco {
			t.Fatalf("CA-551: %s: %q le da el foco a [%d], Foco = %d", que, ttTeclaDe(c.foco), int(c.foco), int(m.Foco()))
		}
		ttRevisarDosColumnas(t, que, m, c.ancho, c.alto, c.n, c.foco)
		if cs := ttCajas(m); cs[ttNombrePanel(c.foco)].Plegado {
			t.Fatalf("CA-550: %s: el panel con el foco nunca se pliega", que)
		}
	}
}

// CA-550: el panel plegado es una linea de su ancho: el signo de linea
// simple, su titulo con su cantidad y signos de linea hasta su ancho
// ("─[3] Corriendo 1──"); [1] plegado dice el check despues de su titulo
// (decision, del dibujo de 60 columnas del spec).
func TestCA550_ElPlegadoEsSuTitulo(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 15, false))
	cs := ttCajas(m)
	p1 := cs["[1]"]
	quiere := "-[1] Proyecto · check VERDE" + strings.Repeat("-", 120/3-ttCeldas("-[1] Proyecto · check VERDE"))
	if !p1.Plegado || p1.Arriba != quiere {
		t.Fatalf("CA-550: a 120x15 [1] va plegado como %q: %q (plegado %v)\n%s", quiere, p1.Arriba, p1.Plegado, ttPantalla(m))
	}
	// a 80x15 su titulo se corta a su ancho - 2 (24 celdas)
	m = ttModeloDorado(ttConfig(80, 15, false))
	if c := ttCajas(m)["[1]"]; !c.Plegado || c.Arriba != "-[1] Proyecto · check VE~-" {
		t.Fatalf("CA-550: a 80x15 [1] plegado corta su titulo a 24 celdas: %q", c.Arriba)
	}
	// a 60x15 (una columna) los plegados ocupan las 60 celdas
	u := ttModeloDorado(ttConfig(60, 15, false))
	cs = ttCajas(u)
	for nombre, titulo := range map[string]string{"[1]": "[1] Proyecto · check VERDE", "[3]": "[3] Corriendo 2", "[4]": "[4] Hallazgos 2"} {
		c := cs[nombre]
		if q := "-" + titulo + strings.Repeat("-", 60-1-ttCeldas(titulo)); !c.Plegado || c.Arriba != q {
			t.Fatalf("CA-550: a 60x15 %s va plegado como %q: %q\n%s", nombre, q, c.Arriba, ttPantalla(u))
		}
	}
	// ROJO y sin veredicto
	p := ttProyectoVerde()
	p.Check.OK, p.Check.Reason = false, "drift"
	u.LeerProyecto(p)
	if c := ttCajas(u)["[1]"]; !strings.HasPrefix(c.Arriba, "-[1] Proyecto · check ROJO-") {
		t.Fatalf("CA-550: con el check rojo, [1] plegado dice ' · check ROJO': %q", c.Arriba)
	}
	p.Check.Reason = "no-verdict"
	u.LeerProyecto(p)
	if c := ttCajas(u)["[1]"]; !strings.HasPrefix(c.Arriba, "-[1] Proyecto · sin veredicto-") {
		t.Fatalf("CA-550: sin veredicto, [1] plegado dice ' · sin veredicto': %q", c.Arriba)
	}
	// antes de la primera lectura del proyecto, nada despues del titulo
	n := ttModelo(ttConfig(60, 15, false), ttTableroPrincipal())
	if c := ttCajas(n)["[1]"]; !strings.HasPrefix(c.Arriba, "-[1] Proyecto--") {
		t.Fatalf("CA-550: antes de leer el proyecto, [1] plegado es solo su titulo: %q", c.Arriba)
	}
}

// ttAltoNatural es la altura natural de un panel de la izquierda en una
// columna (doc.go): [1] 6; [2] sus lineas + 2; [3] como h3; [4] sus filas +
// 2, al menos 3.
func ttAltoNatural(p tablero.Panel, lineas2, filas3, filas4 int) int {
	switch p {
	case tablero.PanelProyecto:
		return 6
	case tablero.PanelTarjetas:
		return lineas2 + 2
	case tablero.PanelCorriendo:
		h := filas3 + 2
		if filas3 == 0 {
			h = 3
		}
		if h > 7 {
			h = 7
		}
		return h
	}
	h := filas4 + 2
	if h < 3 {
		h = 3
	}
	return h
}

// CA-550: entre 60 y 79 columnas, una columna: de arriba, [1], [2], [3], [4],
// [0] y el Registro, todos de Ancho celdas; el Registro siempre plegado; el
// panel de la izquierda con el foco ocupa min(su altura natural, A/3) lineas
// y al menos 3, los otros tres plegados, y [0] el resto; con el foco en [0],
// los cuatro plegados.
func TestCA550_UnaColumna(t *testing.T) {
	// ttModeloDorado: [2] tiene 11 lineas, [3] 2 filas, [4] 2 filas
	for _, tam := range [][2]int{{60, 15}, {70, 24}, {79, 24}, {79, 40}, {65, 18}} {
		for _, foco := range []tablero.Panel{tablero.PanelTarjetas, tablero.PanelProyecto, tablero.PanelCorriendo, tablero.PanelHallazgos, tablero.PanelPrincipal} {
			ancho, alto := tam[0], tam[1]
			que := fmt.Sprintf("%dx%d con el foco en [%d]", ancho, alto, int(foco))
			m := ttModeloDorado(ttConfig(ancho, alto, false))
			m.Tecla(ttTeclaDe(foco), ttAhora)
			ttForma(t, "CA-550", m, ancho, alto, false)
			b := alto - 2
			cs := ttCajas(m)
			fila := 0
			for _, p := range []tablero.Panel{tablero.PanelProyecto, tablero.PanelTarjetas, tablero.PanelCorriendo, tablero.PanelHallazgos} {
				n := ttNombrePanel(p)
				c, ok := cs[n]
				if !ok {
					t.Fatalf("CA-550: %s: la pantalla tiene %s:\n%s", que, n, ttPantalla(m))
				}
				h, pleg := 1, true
				if p == foco {
					h = ttAltoNatural(p, 11, 2, 2)
					if h > alto/3 {
						h = alto / 3
					}
					if h < 3 {
						h = 3
					}
					pleg = false
				}
				if c.Fila != fila || c.Col != 0 || c.Ancho != ancho || c.Alto != h || c.Plegado != pleg {
					t.Fatalf("CA-550: %s: %s esta en la linea %d con %d celdas y %d lineas (plegado %v); esta en la %d con %d celdas y %d lineas (plegado %v):\n%s",
						que, n, fila+1, ancho, h, pleg, c.Fila+1, c.Ancho, c.Alto, c.Plegado, ttPantalla(m))
				}
				fila += h
			}
			c0, reg := cs["[0]"], cs["Registro"]
			if c0.Fila != fila || c0.Col != 0 || c0.Ancho != ancho || c0.Alto != b-fila-1 {
				t.Fatalf("CA-550: %s: [0] sigue a los paneles de la izquierda y ocupa el resto (%d lineas): esta en la %d con %d lineas:\n%s",
					que, b-fila-1, c0.Fila+1, c0.Alto, ttPantalla(m))
			}
			if !reg.Plegado || reg.Fila != b-1 || reg.Ancho != ancho {
				t.Fatalf("CA-550: %s: el Registro va plegado en la ultima linea de los paneles: %+v\n%s", que, reg, ttPantalla(m))
			}
		}
	}
}

// CA-550, casos limite: el borde exacto entre una y dos columnas (79 y 80) y
// el minimo (60x15 dibuja, 59x15 y 60x14 no).
func TestCA550_TamanosBorde(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, false))
	if c := ttCajas(m)["[0]"]; c.Col != 80/3 {
		t.Fatalf("CA-550: a 80 columnas hay dos columnas ([0] en la celda %d): %+v", 80/3, c)
	}
	m = ttModeloDorado(ttConfig(79, 24, false))
	if c := ttCajas(m)["[0]"]; c.Col != 0 || c.Ancho != 79 {
		t.Fatalf("CA-550: a 79 columnas hay una sola columna ([0] de 79 celdas en la celda 0): %+v", c)
	}
	m = ttModeloDorado(ttConfig(60, 15, false))
	if _, ok := ttCajas(m)["[2]"]; !ok {
		t.Fatalf("CA-550: 60x15 es el minimo y dibuja los paneles:\n%s", ttPantalla(m))
	}
	for _, tam := range [][2]int{{59, 15}, {60, 14}, {59, 14}, {50, 10}, {20, 5}, {1, 1}} {
		m := ttModeloDorado(ttConfig(tam[0], tam[1], false))
		ttPantallaChica(t, m, tam[0], tam[1], false)
	}
}

// ttPantallaChica exige la pantalla de por debajo de 60x15: una linea,
// "agranda la terminal (minimo 60x15)" cortado al ancho, y Alto-1 vacias.
func ttPantallaChica(t *testing.T, m *tablero.Modelo, ancho, alto int, utf bool) {
	t.Helper()
	const msg = "agranda la terminal (minimo 60x15)"
	quiere := msg
	if len(msg) > ancho {
		corte := "~"
		if utf {
			corte = "…"
		}
		quiere = msg[:ancho-1] + corte
	}
	p := m.Pantalla()
	if len(p) != alto || p[0] != quiere {
		t.Fatalf("CA-550: a %dx%d la pantalla es una linea %q y %d vacias: %q", ancho, alto, quiere, alto-1, p)
	}
	for i, l := range p[1:] {
		if l != "" {
			t.Fatalf("CA-550: a %dx%d la linea %d esta vacia: %q", ancho, alto, i+2, l)
		}
	}
}

// CA-550, caso limite: por debajo de 60x15 ninguna tecla salvo q y ctrl-c, y
// ningun evento del mouse, hace nada; al agrandar vuelve la misma pantalla de
// antes (la seleccion y el foco no se movieron).
func TestCA550_PorDebajoDelMinimoNadaHaceNada(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, false))
	m.Tecla("3", ttAhora)
	antes := ttPantalla(m)
	m.Tamano(59, 15)
	ttPantallaChica(t, m, 59, 15, false)
	for _, k := range []tablero.Tecla{"1", "j", tablero.TeclaEnter, "w", "A", "u", "/", "?", "x", tablero.TeclaTab, tablero.TeclaEspacio, "e", "n"} {
		ttSinOrdenes(t, "CA-550", fmt.Sprintf("%q a 59x15", k), m.Tecla(k, ttAhora))
		ttPantallaChica(t, m, 59, 15, false)
	}
	for _, r := range []tablero.Raton{{Accion: tablero.RatonClic, X: 0, Y: 0}, {Accion: tablero.RatonRuedaAbajo, X: 5, Y: 3}} {
		ttSinOrdenes(t, "CA-550", "el mouse a 59x15", m.Raton(r, ttAhora))
	}
	if m.Modal() != tablero.SinModal || m.Foco() != tablero.PanelCorriendo || len(m.Registro()) != 0 {
		t.Fatalf("CA-550: a 59x15 nada cambio: modal %q, foco %d, registro %v", m.Modal(), int(m.Foco()), m.Registro())
	}
	m.Tamano(80, 24)
	if got := ttPantalla(m); got != antes {
		t.Fatalf("CA-550: al agrandar vuelve la pantalla de antes:\n%s\nantes:\n%s", got, antes)
	}
	m.Tamano(30, 10)
	for _, k := range []tablero.Tecla{"q", tablero.TeclaCtrlC} {
		if os := m.Tecla(k, ttAhora); len(os) != 1 || os[0] != tablero.Orden(tablero.Salir{}) {
			t.Fatalf("CA-550: %q por debajo del minimo ordena Salir: %#v", k, os)
		}
	}
	// el corte del aviso con UTF-8
	u := ttModeloDorado(ttConfig(20, 5, true))
	ttPantallaChica(t, u, 20, 5, true)
}

// ---------------------------------------------------------------- CA-551: el foco

// CA-551: al abrir el foco esta en [2]; siempre hay exactamente un panel con
// los signos del foco (= y # en ASCII) y es el de Foco(); 1, 2, 3, 4 y 0 lo
// llevan a ese panel; el Registro nunca lo tiene. Re-expresa CA-520.
func TestCA551_CA520_FocoConLasTeclasDeNumero(t *testing.T) {
	m := tablero.Nuevo(ttConfig(120, 32, false))
	if m.Foco() != tablero.PanelTarjetas {
		t.Fatalf("CA-551: antes de la primera lectura el foco esta en [2]: %d", int(m.Foco()))
	}
	m = ttModeloDorado(ttConfig(120, 32, false))
	if m.Foco() != tablero.PanelTarjetas || strings.Join(ttFocos(m), ",") != "[2]" {
		t.Fatalf("CA-551: al abrir el foco esta en [2] y solo [2] lleva sus signos: %d, %v\n%s", int(m.Foco()), ttFocos(m), ttPantalla(m))
	}
	for _, p := range []tablero.Panel{tablero.PanelProyecto, tablero.PanelCorriendo, tablero.PanelHallazgos, tablero.PanelPrincipal, tablero.PanelTarjetas, tablero.PanelPrincipal} {
		ttSinOrdenes(t, "CA-551", string(ttTeclaDe(p)), m.Tecla(ttTeclaDe(p), ttAhora))
		if m.Foco() != p {
			t.Fatalf("CA-551: %q le da el foco a [%d], Foco = %d", ttTeclaDe(p), int(p), int(m.Foco()))
		}
		if f := ttFocos(m); len(f) != 1 || f[0] != ttNombrePanel(p) {
			t.Fatalf("CA-551: tras %q solo %s lleva los signos del foco: %v\n%s", ttTeclaDe(p), ttNombrePanel(p), f, ttPantalla(m))
		}
	}
}

// CA-551: tab y shift-tab mueven el foco en el orden 1, 2, 3, 4, 0, en ciclo.
func TestCA551_TabYShiftTab(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	orden := []tablero.Panel{tablero.PanelCorriendo, tablero.PanelHallazgos, tablero.PanelPrincipal, tablero.PanelProyecto, tablero.PanelTarjetas,
		tablero.PanelCorriendo}
	for i, p := range orden {
		ttSinOrdenes(t, "CA-551", "tab", m.Tecla(tablero.TeclaTab, ttAhora))
		if m.Foco() != p {
			t.Fatalf("CA-551: el tab numero %d deja el foco en [%d], esta en [%d]", i+1, int(p), int(m.Foco()))
		}
	}
	atras := []tablero.Panel{tablero.PanelTarjetas, tablero.PanelProyecto, tablero.PanelPrincipal, tablero.PanelHallazgos, tablero.PanelCorriendo}
	for i, p := range atras {
		m.Tecla(tablero.TeclaShiftTab, ttAhora)
		if m.Foco() != p {
			t.Fatalf("CA-551: el shift-tab numero %d deja el foco en [%d], esta en [%d]", i+1, int(p), int(m.Foco()))
		}
	}
}

// CA-551: con color, el borde del panel con foco (el titulo incluido) va en
// verde y negrita (1;32); los otros bordes, sin estilo.
func TestCA551_ElFocoConColor(t *testing.T) {
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	m := ttModeloDorado(cfg)
	p := m.Pantalla()
	// [2] empieza en la linea 7 (h1 = 6), celda 0
	if !strings.HasPrefix(p[6], "\x1b[1;32m#[2] Tarjetas=") {
		t.Fatalf("CA-551: el borde de arriba de [2] con el foco, titulo incluido, va en 1;32: %q", p[6])
	}
	if strings.HasPrefix(p[0], "\x1b[") || !strings.HasPrefix(p[0], "+[1] Proyecto-") {
		t.Fatalf("CA-551: el borde de [1] sin foco no tiene estilo: %q", p[0])
	}
	for i := 7; i < 6+14-1; i++ {
		if !strings.HasPrefix(p[i], "\x1b[1;32m#\x1b[0m") {
			t.Fatalf("CA-551: el lado izquierdo de [2] con foco va en 1;32 (linea %d): %q", i+1, p[i])
		}
	}
	m.Tecla("0", ttAhora)
	p = m.Pantalla()
	if strings.HasPrefix(p[6], "\x1b[1;32m") || !strings.HasPrefix(p[6], "+[2] Tarjetas-") || !strings.Contains(p[0], "\x1b[1;32m#[0] nuc-tenancy") {
		t.Fatalf("CA-551: con el foco en [0], su borde va en 1;32 y el de [2] ya no:\n%q\n%q", p[0], p[6])
	}
}

// CA-551 (propiedad): ninguna secuencia de teclas de moverse deja la pantalla
// sin un panel con foco o con dos: siempre exactamente uno, el de Foco().
func TestCA551_PropiedadSiempreUnFoco(t *testing.T) {
	teclas := []tablero.Tecla{"1", "2", "3", "4", "0", tablero.TeclaTab, tablero.TeclaShiftTab, tablero.TeclaEnter, tablero.TeclaEsc,
		"j", "k", "g", "G", "h", "l", tablero.TeclaIzq, tablero.TeclaDer, tablero.TeclaArriba, tablero.TeclaAbajo, tablero.TeclaPgAbajo, "x"}
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		tams := [][2]int{{120, 32}, {80, 24}, {70, 20}, {60, 15}, {80, 15}}
		tam := tams[r.Intn(len(tams))]
		m := ttModeloDorado(ttConfig(tam[0], tam[1], r.Intn(2) == 0))
		for i := 0; i < 40; i++ {
			k := teclas[r.Intn(len(teclas))]
			m.Tecla(k, ttAhora)
			if m.Modal() != tablero.SinModal {
				t.Logf("semilla %d: %q abrio el modal %q", semilla, k, m.Modal())
				return false
			}
			f := ttFocos(m)
			if len(f) != 1 || f[0] != ttNombrePanel(m.Foco()) {
				t.Logf("semilla %d (%dx%d): tras %q los paneles con foco son %v y Foco es [%d]:\n%s", semilla, tam[0], tam[1], k, f, int(m.Foco()), ttPantalla(m))
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 80}); err != nil {
		t.Fatalf("CA-551: %v", err)
	}
}

// ---------------------------------------------------------------- CA-552: la seleccion

// CA-552: en [2], ↓ j ↑ k mueven la seleccion una fila (los titulos de
// seccion no son filas; se detiene en las puntas), g y G van a la primera y
// a la ultima (la linea de Hecho). Re-expresa CA-520.
func TestCA552_CA520_MoverEnTarjetas(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	orden := []string{"nuc-tenancy", "pagos-caidos", "infra-api", "infra-ci-cd", "docs-guia", "precios", "HECHO"}
	if got := ttSlugElegido(t, m); got != orden[0] {
		t.Fatalf("CA-552: al abrir la seleccion esta en la primera fila de tarjeta: %q", got)
	}
	for i, k := range []tablero.Tecla{"j", tablero.TeclaAbajo, "j", "j", tablero.TeclaAbajo, "j", "j"} {
		m.Tecla(k, ttAhora)
		quiere := orden[min(i+1, len(orden)-1)]
		if got := ttSlugElegido(t, m); got != quiere {
			t.Fatalf("CA-552: el %q numero %d elige %q, eligio %q\n%s", k, i+1, quiere, got, ttPantalla(m))
		}
	}
	m.Tecla("g", ttAhora)
	if got := ttSlugElegido(t, m); got != "nuc-tenancy" {
		t.Fatalf("CA-552: g va a la primera fila: %q", got)
	}
	m.Tecla("k", ttAhora)
	if got := ttSlugElegido(t, m); got != "nuc-tenancy" {
		t.Fatalf("CA-552: k en la primera no se mueve: %q", got)
	}
	m.Tecla("G", ttAhora)
	if got := ttSlugElegido(t, m); got != "HECHO" {
		t.Fatalf("CA-552: G va a la ultima fila (la linea de Hecho): %q", got)
	}
	m.Tecla(tablero.TeclaArriba, ttAhora)
	if got := ttSlugElegido(t, m); got != "precios" {
		t.Fatalf("CA-552: ↑ sube una fila: %q", got)
	}
}

// CA-552: [3] y [4] tienen su propia seleccion, que queda mientras el foco
// esta en otro panel, con su ▸ ("> " en ASCII); se mueven con j k g G.
func TestCA552_CadaPanelSuSeleccion(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	c3 := ttCajaDe(t, "CA-552", m, "[3]")
	if got := ttElegidaEn(c3); !strings.HasPrefix(got, "infra-api ") {
		t.Fatalf("CA-552: [3] sin foco muestra su fila elegida con '>': %q\n%s", got, ttPantalla(m))
	}
	m.Tecla("3", ttAhora)
	m.Tecla("j", ttAhora)
	if got := ttElegidaEn(ttCajaDe(t, "CA-552", m, "[3]")); !strings.HasPrefix(got, "pagos-caidos ") {
		t.Fatalf("CA-552: j en [3] elige la segunda fila: %q", got)
	}
	m.Tecla("4", ttAhora)
	if got := ttElegidaEn(ttCajaDe(t, "CA-552", m, "[3]")); !strings.HasPrefix(got, "pagos-caidos ") {
		t.Fatalf("CA-552: la seleccion de [3] queda con el foco en [4]: %q", got)
	}
	if got := ttElegidaEn(ttCajaDe(t, "CA-552", m, "[4]")); !strings.HasPrefix(got, "high ") {
		t.Fatalf("CA-552: [4] empieza en su primera fila (el high): %q", got)
	}
	m.Tecla("G", ttAhora)
	if got := ttElegidaEn(ttCajaDe(t, "CA-552", m, "[4]")); !strings.HasPrefix(got, "low ") {
		t.Fatalf("CA-552: G en [4] va a la ultima: %q", got)
	}
	m.Tecla("g", ttAhora)
	if got := ttElegidaEn(ttCajaDe(t, "CA-552", m, "[4]")); !strings.HasPrefix(got, "high ") {
		t.Fatalf("CA-552: g en [4] va a la primera: %q", got)
	}
	// y la de [2] quedo en su tarjeta
	if got := ttSlugElegido(t, m); got != "nuc-tenancy" {
		t.Fatalf("CA-552: la seleccion de [2] queda en su fila: %q", got)
	}
}

// CA-552: con color, la fila elegida del panel con foco es una barra en video
// inverso sobre su linea interior entera (sus w-2 celdas, el relleno
// incluido); un slug en rojo va en 7;31 dentro de la barra; en los paneles
// sin foco queda solo el ▸.
func TestCA552_LaBarraEnVideoInverso(t *testing.T) {
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	m := ttModeloDorado(cfg)
	l, _ := ttLineaCon(m, "# > nuc-tenancy")
	var cruda string
	for _, x := range m.Pantalla() {
		if strings.Contains(ttSinSGR(x), "# > nuc-tenancy") {
			cruda = x
		}
	}
	interior := ttTramo(l, 1, 39)
	if !strings.Contains(cruda, "\x1b[7m"+interior+"\x1b[0m") {
		t.Fatalf("CA-552: la fila elegida de [2] con foco es 7 sobre sus 38 celdas interiores (%q): %q", interior, cruda)
	}
	// la fila de [3] elegida, sin foco: sin video inverso
	for _, x := range m.Pantalla() {
		if strings.Contains(ttSinSGR(x), "| > infra-api writer") && strings.Contains(x[:strings.Index(x, "infra-api")], "\x1b[7") {
			t.Fatalf("CA-552: en un panel sin foco la fila elegida no lleva video inverso: %q", x)
		}
	}
	// el slug en rojo dentro de la barra
	m.Tecla("j", ttAhora)
	var pagos string
	for _, x := range m.Pantalla() {
		if strings.Contains(ttSinSGR(x), "# > pagos-caidos") {
			pagos = x
		}
	}
	if !strings.Contains(pagos, "\x1b[7;31mpagos-caidos") {
		t.Fatalf("CA-552, CA-555: el slug de la interrumpida, elegido con foco, va en 7;31: %q", pagos)
	}
	// sin color, el ▸ y nada mas
	s := ttModeloDorado(ttConfig(120, 32, false))
	if !ttTiene(s, "# > nuc-tenancy") || !ttTiene(s, "| > infra-api writer") {
		t.Fatalf("CA-552: sin color, la seleccion es el > de cada panel:\n%s", ttPantalla(s))
	}
}

// ttTableroLargo son n tarjetas en Backlog (b-00..), sin otras.
func ttTableroLargo(n int) boardcmd.Board {
	var cs []boardcmd.Card
	for i := 0; i < n; i++ {
		cs = append(cs, ttBacklog(fmt.Sprintf("b-%02d", i), fmt.Sprintf("Tarjeta %02d", i)))
	}
	return ttTablero(cs...)
}

// CA-552: cuando las filas no entran, el panel se desplaza lo minimo para
// que la elegida se vea (al bajar de la vista queda en la ultima linea, al
// subir en la primera) y su borde de abajo dice "<n> de <m>" (n el lugar de
// la elegida entre las filas, m cuantas son: los titulos de seccion no
// cuentan, la linea de Hecho si).
func TestCA552_DesplazamientoMinimoYNDeM(t *testing.T) {
	// 80x24: B = 22, h1 = 6, h3 = 3 (nada corriendo), h4 = max(3, 13/3) = 4,
	// h2 = 9 (7 interiores); [2] tiene el titulo BACKLOG 30, 30 filas y la
	// de Hecho: 32 lineas, 31 filas.
	m := ttModelo(ttConfig(80, 24, false), ttTableroLargo(30))
	vista := func() []string { return ttCajaDe(t, "CA-552", m, "[2]").ttTexto() }
	abajo := func() string { return ttCajaDe(t, "CA-552", m, "[2]").Abajo }
	if v := vista(); len(v) != 7 {
		t.Fatalf("CA-550: a 80x24 sin trabajos [2] tiene 7 lineas interiores: %q", v)
	}
	if a := abajo(); !strings.HasSuffix(a, "=1 de 31=#") {
		t.Fatalf("CA-552: el borde de abajo dice '1 de 31' antes de la esquina, con un signo de linea entre: %q", a)
	}
	if v := vista(); v[0] != " BACKLOG 30" || !strings.HasPrefix(v[1], " > b-00") {
		t.Fatalf("CA-552: la vista empieza arriba: %q", v)
	}
	for i := 0; i < 5; i++ {
		m.Tecla("j", ttAhora)
	}
	if v := vista(); !strings.HasPrefix(v[6], " > b-05") || v[0] != " BACKLOG 30" {
		t.Fatalf("CA-552: dentro de la vista, moverse no la desplaza: %q", v)
	}
	m.Tecla("j", ttAhora)
	// b-06 es la linea 8 de 32: no entraba, ahora es la ultima de la vista
	v := vista()
	if !strings.HasPrefix(v[6], " > b-06") || !strings.HasPrefix(v[0], "   b-00") {
		t.Fatalf("CA-552: al bajar de la vista, la elegida queda en su ultima linea (desplazamiento minimo): %q", v)
	}
	if a := abajo(); !strings.HasSuffix(a, "=7 de 31=#") {
		t.Fatalf("CA-552: el borde dice '7 de 31': %q", a)
	}
	for i := 0; i < 3; i++ {
		m.Tecla("k", ttAhora)
	}
	if v := vista(); !strings.HasPrefix(v[3], " > b-03") || !strings.HasPrefix(v[0], "   b-00") {
		t.Fatalf("CA-552: subir dentro de la vista no la desplaza: %q", v)
	}
	m.Tecla("G", ttAhora)
	if v := vista(); v[6] != " > HECHO 0 >" {
		t.Fatalf("CA-552: G lleva a la ultima fila (Hecho), al final de la vista: %q", v)
	}
	if a := abajo(); !strings.HasSuffix(a, "=31 de 31=#") {
		t.Fatalf("CA-552: el borde dice '31 de 31': %q", a)
	}
	m.Tecla("g", ttAhora)
	if v := vista(); !strings.HasPrefix(v[0], " > b-00") {
		t.Fatalf("CA-552: g vuelve a la primera fila, que al subir de la vista queda en su primera linea: %q", v)
	}
	// cuando todo entra, no hay "<n> de <m>"
	c := ttModelo(ttConfig(80, 24, false), ttTableroLargo(2))
	if a := ttCajaDe(t, "CA-552", c, "[2]").Abajo; strings.Contains(a, " de ") {
		t.Fatalf("CA-552: si las filas entran el borde no dice '<n> de <m>': %q", a)
	}
}

// CA-552, caso limite: mas filas que alto en [3] y en [4]: se desplazan y su
// borde dice "<n> de <m>".
func TestCA552_TresYCuatroConMasFilasQueAlto(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTableroConCorriendo(9))
	var hs []finding.Item
	for i := 0; i < 8; i++ {
		hs = append(hs, ttHallazgo(fmt.Sprintf("h%d", i), finding.StatusOpen, "medium", fmt.Sprintf("a%d.go:%d", i, i+1), "d"))
	}
	m.LeerHallazgos(tablero.Hallazgos{Items: hs})
	c3 := ttCajaDe(t, "CA-552", m, "[3]")
	if c3.Alto != 7 || !strings.HasSuffix(c3.Abajo, "-1 de 9-+") {
		t.Fatalf("CA-552: [3] con 9 trabajos ocupa 7 lineas y su borde dice '1 de 9': %+v", c3)
	}
	m.Tecla("3", ttAhora)
	m.Tecla("G", ttAhora)
	c3 = ttCajaDe(t, "CA-552", m, "[3]")
	if !strings.HasSuffix(c3.Abajo, "=9 de 9=#") || !strings.HasPrefix(c3.Dentro[len(c3.Dentro)-1], " > corre-8") {
		t.Fatalf("CA-552: G en [3] lleva a la ultima, a la vista: %+v", c3)
	}
	m.Tecla("4", ttAhora)
	m.Tecla("j", ttAhora)
	m.Tecla("j", ttAhora)
	m.Tecla("j", ttAhora)
	c4 := ttCajaDe(t, "CA-552", m, "[4]")
	if !strings.HasSuffix(c4.Abajo, "=4 de 8=#") || ttElegidaEn(c4) == "" {
		t.Fatalf("CA-552: [4] con 8 hallazgos se desplaza y dice '4 de 8': %+v", c4)
	}
}

// ---------------------------------------------------------------- CA-553: lo que muestra [0]

// CA-553: [0] muestra lo elegido en el ultimo panel de la izquierda que tuvo
// el foco y cambia con cada movimiento, sin enter: la tarjeta de [2] ("[0]
// <slug> · <titulo>"), el proyecto de [1], el trabajo de [3] ("[0] <slug> ·
// <rol>"), el hallazgo de [4] ("[0] hallazgo <id>"), nada en la linea de
// Hecho ("[0]"). Elegida y ArchivoElegido dicen que leer. Re-expresa CA-520
// y CA-521.
func TestCA553_CA520_CA521_CeroSigueALaSeleccion(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	titulo := func() string { return ttCajaDe(t, "CA-553", m, "[0]").Titulo }
	if got := titulo(); got != "[0] nuc-tenancy · Tenancy del nucleo" || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("CA-553: al abrir [0] muestra la tarjeta elegida en [2]: %q, Elegida %q", got, m.Elegida())
	}
	m.Tecla("j", ttAhora)
	if got := titulo(); got != "[0] pagos-caidos · Pagos caidos" || m.Elegida() != "pagos-caidos" {
		t.Fatalf("CA-553: [0] cambia con la seleccion, sin enter: %q, Elegida %q", got, m.Elegida())
	}
	m.Tecla("G", ttAhora)
	if got := titulo(); got != "[0]" || m.Elegida() != "" {
		t.Fatalf("CA-553: en la linea de Hecho [0] no muestra nada: %q, Elegida %q", got, m.Elegida())
	}
	m.Tecla("1", ttAhora)
	if got := titulo(); got != "[0] Proyecto" || m.Elegida() != "" {
		t.Fatalf("CA-553: con el foco en [1], [0] muestra el proyecto: %q, Elegida %q", got, m.Elegida())
	}
	m.Tecla("3", ttAhora)
	if got := titulo(); got != "[0] infra-api · writer" || m.Elegida() != "infra-api" {
		t.Fatalf("CA-553: con el foco en [3], [0] muestra su trabajo: %q, Elegida %q", got, m.Elegida())
	}
	m.Tecla("j", ttAhora)
	if got := titulo(); got != "[0] pagos-caidos · writer" || m.Elegida() != "pagos-caidos" {
		t.Fatalf("CA-553: [0] sigue a la seleccion de [3]: %q", got)
	}
	m.Tecla("4", ttAhora)
	if got := titulo(); got != "[0] hallazgo h-high" || m.Elegida() != "" {
		t.Fatalf("CA-553: con el foco en [4], [0] muestra el hallazgo elegido (el high primero): %q", got)
	}
	if r, l := m.ArchivoElegido(); r != "internal/api/router.go" || l != 41 {
		t.Fatalf("CA-559: ArchivoElegido es el archivo del hallazgo y su linea: %q %d", r, l)
	}
	m.Tecla("j", ttAhora)
	if got := titulo(); got != "[0] hallazgo h-low" {
		t.Fatalf("CA-553: [0] sigue a la seleccion de [4]: %q", got)
	}
	// [2] tiene el foco de nuevo: su tarjeta (la seleccion quedo en Hecho)
	m.Tecla("2", ttAhora)
	if got := titulo(); got != "[0]" {
		t.Fatalf("CA-553: de vuelta en [2], [0] muestra lo de [2]: %q", got)
	}
	if r, _ := m.ArchivoElegido(); r != "" {
		t.Fatalf("CA-559: sin hallazgo en [0], ArchivoElegido es \"\": %q", r)
	}
}

// CA-553: enter, → y l pasan el foco a [0] (que sigue mostrando lo de ese
// panel), y esc, ← y h lo devuelven al panel de donde vino; en [0] el Lugar
// dice que muestra. En la linea de Hecho, enter (y → y l) la abren y la
// cierran sin mover el foco.
func TestCA553_EnterYEscEntreUnPanelYCero(t *testing.T) {
	casos := []struct {
		desde tablero.Tecla
		panel tablero.Panel
		lugar tablero.Lugar
		ver   tablero.Lugar
	}{
		{"2", tablero.PanelTarjetas, tablero.LugarTarjetas, tablero.LugarVerTarjeta},
		{"1", tablero.PanelProyecto, tablero.LugarProyecto, tablero.LugarVerProyecto},
		{"3", tablero.PanelCorriendo, tablero.LugarCorriendo, tablero.LugarVerCorriendo},
		{"4", tablero.PanelHallazgos, tablero.LugarHallazgos, tablero.LugarVerHallazgo},
	}
	for _, c := range casos {
		for _, entrar := range []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaDer, "l"} {
			for _, volver := range []tablero.Tecla{tablero.TeclaEsc, tablero.TeclaIzq, "h"} {
				m := ttModeloDorado(ttConfig(120, 32, false))
				m.Tecla(c.desde, ttAhora)
				antes := ttCajaDe(t, "CA-553", m, "[0]").Titulo
				if m.Lugar() != c.lugar {
					t.Fatalf("CA-553: con el foco en [%d] el Lugar es %q, es %q", int(c.panel), c.lugar, m.Lugar())
				}
				ttSinOrdenes(t, "CA-553", string(entrar), m.Tecla(entrar, ttAhora))
				if m.Foco() != tablero.PanelPrincipal || m.Lugar() != c.ver {
					t.Fatalf("CA-553: %q en [%d] pasa el foco a [0] (%q): foco %d, lugar %q", entrar, int(c.panel), c.ver, int(m.Foco()), m.Lugar())
				}
				if got := ttCajaDe(t, "CA-553", m, "[0]").Titulo; got != antes {
					t.Fatalf("CA-553: con el foco en [0] sigue mostrando lo de [%d]: %q, se esperaba %q", int(c.panel), got, antes)
				}
				ttSinOrdenes(t, "CA-553", string(volver), m.Tecla(volver, ttAhora))
				if m.Foco() != c.panel || m.Lugar() != c.lugar {
					t.Fatalf("CA-553: %q en [0] devuelve el foco a [%d]: foco %d, lugar %q", volver, int(c.panel), int(m.Foco()), m.Lugar())
				}
			}
		}
	}
	// Hecho
	for _, k := range []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaDer, "l"} {
		m := ttModeloDorado(ttConfig(120, 32, false))
		m.Tecla("G", ttAhora)
		ttSinOrdenes(t, "CA-553", string(k)+" en Hecho", m.Tecla(k, ttAhora))
		if m.Foco() != tablero.PanelTarjetas || !ttTiene(m, "# > HECHO 3 v") || !ttTiene(m, "   viejo-1 ") {
			t.Fatalf("CA-553: %q sobre la linea de Hecho la abre (' HECHO 3 v', con sus tarjetas) sin mover el foco:\n%s", k, ttPantalla(m))
		}
		m.Tecla(k, ttAhora)
		if !ttTiene(m, "# > HECHO 3 >") || ttTiene(m, "viejo-1") {
			t.Fatalf("CA-553: %q otra vez la cierra:\n%s", k, ttPantalla(m))
		}
	}
	// [0] sin nada que mostrar: LugarVerNada, y esc vuelve
	m := ttModeloDorado(ttConfig(120, 32, false))
	m.Tecla("G", ttAhora)
	m.Tecla("0", ttAhora)
	if m.Lugar() != tablero.LugarVerNada {
		t.Fatalf("CA-553: [0] sin nada que mostrar es LugarVerNada, es %q", m.Lugar())
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	if m.Foco() != tablero.PanelTarjetas {
		t.Fatalf("CA-553: esc desde [0] vuelve a [2]: %d", int(m.Foco()))
	}
}

// ---------------------------------------------------------------- CA-577: los anchos

// CA-577: la tabla de anchos de hoom: dos celdas los de ancho doble (East
// Asian Wide, Fullwidth, los emojis que se presentan como emoji, ✋), cero
// las marcas combinantes (y el selector de variacion), una el resto, todos
// los signos de la pantalla incluidos.
func TestCA577_AnchoDe(t *testing.T) {
	for _, r := range []rune{'漢', '字', 'Ａ', '🚀', '✋', 'あ', '한', '😀'} {
		if got := tablero.AnchoDe(r); got != 2 {
			t.Fatalf("CA-577: AnchoDe(%q) = %d, es de ancho doble", r, got)
		}
	}
	for _, r := range []rune{'\u0301', '\u0303', '\uFE0F', '\u20DD'} {
		if got := tablero.AnchoDe(r); got != 0 {
			t.Fatalf("CA-577: AnchoDe(%U) = %d, una marca combinante no ocupa celdas", r, got)
		}
	}
	for _, r := range []rune("a~ñé●○·✓✗⚑▸▾‹›…▌─│┌┐└┘═║╔╗╚╝↑↓←→¿Ó#=+|") {
		if got := tablero.AnchoDe(r); got != 1 {
			t.Fatalf("CA-577: AnchoDe(%q) = %d, ocupa una celda", r, got)
		}
	}
	casos := map[string]int{"": 0, "abc": 3, "漢字": 4, "🚀 go": 5, "e\u0301": 1, "man\u0303ana": 6, "●✋○·": 5, "Ａｂ": 4}
	for s, n := range casos {
		if got := tablero.Celdas(s); got != n {
			t.Fatalf("CA-577: Celdas(%q) = %d, se esperaba %d", s, got, n)
		}
	}
}

// CA-577 (propiedad): Celdas es la suma de AnchoDe de las runas.
func TestCA577_CeldasEsLaSuma(t *testing.T) {
	prop := func(rs []rune) bool {
		n := 0
		for _, r := range rs {
			n += tablero.AnchoDe(r)
		}
		return tablero.Celdas(string(rs)) == n
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-577: %v", err)
	}
}

// ttCardAncha es una tarjeta de Backlog con un titulo y un plain dados.
func ttCardAncha(slug, titulo, plain string) boardcmd.Card {
	c := ttCard(slug, titulo, boardcmd.ColBacklog, plain, ttSeg("spec", "spec", boardcmd.SegFalta, "no hay spec"))
	c.Actions = []boardcmd.Action{ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec")}
	return c
}

// CA-577: una tarjeta con 漢字, 🚀, un Fullwidth y marcas combinantes en su
// titulo y en su plain no corre el borde derecho de [0]: cada linea de la
// pantalla mide exactamente Ancho celdas donde termina en un borde, y la
// esquina de la derecha de [0] esta en la ultima celda.
func TestCA577_ElBordeNoSeCorre(t *testing.T) {
	titulos := []string{"Despegue 🚀 del 漢字", "漢字漢字漢字漢字", "🚀🚀🚀", "Ｆｕｌｌ ｗｉｄｔｈ", "Cafe\u0301 y man\u0303ana", "mezcla 漢 a 🚀 b é\u0301"}
	for _, utf := range []bool{true, false} {
		for _, ti := range titulos {
			for _, tam := range [][2]int{{80, 24}, {120, 32}, {70, 24}} {
				m := ttModelo(ttConfig(tam[0], tam[1], utf), ttTablero(ttCardAncha("cohete", ti, "lanzar el "+ti)))
				ttForma(t, "CA-577", m, tam[0], tam[1], false)
				c0 := ttCajaDe(t, "CA-577", m, "[0]")
				if c0.Col+c0.Ancho != tam[0] {
					t.Fatalf("CA-577: con %q (utf8=%v, %dx%d) [0] termina en la ultima celda: empieza en %d con %d celdas:\n%s",
						ti, utf, tam[0], tam[1], c0.Col, c0.Ancho, ttPantalla(m))
				}
				for i, l := range ttLimpia(m) {
					if i >= tam[1]-2 {
						break
					}
					if n := ttCeldas(l); n != tam[0] {
						t.Fatalf("CA-577: con %q (utf8=%v, %dx%d) la linea %d de los paneles mide %d celdas, no %d: %q",
							ti, utf, tam[0], tam[1], i+1, n, tam[0], l)
					}
				}
			}
		}
	}
}

// CA-577: un caracter ancho que quedaria a caballo del limite de un corte se
// va entero, y la celda que deja es un espacio antes del borde (doc.go, "The
// screen"). [0] a 80 columnas tiene 54 celdas: su titulo se corta a 52, con
// "[0] cohet · " (12 celdas) y diecinueve 漢 (38) caben 50 celdas mas "…":
// 51, y la celda 52 es un espacio.
func TestCA577_UnAnchoAlBordeDelCorte(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, true), ttTablero(ttCardAncha("cohet", strings.Repeat("漢", 30), "p")))
	c0 := ttCajaDe(t, "CA-577", m, "[0]")
	quiere := "┌[0] cohet · " + strings.Repeat("漢", 19) + "… ┐"
	if c0.Arriba != quiere {
		t.Fatalf("CA-577: el titulo cortado deja un espacio donde no entra el ancho:\n got %q\nwant %q", c0.Arriba, quiere)
	}
	// en ASCII el corte es ~
	a := ttModelo(ttConfig(80, 24, false), ttTablero(ttCardAncha("cohet", strings.Repeat("漢", 30), "p")))
	if c := ttCajaDe(t, "CA-577", a, "[0]"); c.Arriba != "+[0] cohet · "+strings.Repeat("漢", 19)+"~ +" {
		t.Fatalf("CA-577: en ASCII el corte es ~: %q", c.Arriba)
	}
}

// CA-577 (propiedad): con titulos y plains al azar de runas anchas,
// combinantes, controles y ASCII, toda linea de Pantalla mide a lo sumo
// Ancho celdas (con la tabla de los tests, que no es la del paquete) y las
// de los paneles que terminan en un borde miden exactamente Ancho.
func TestCA577_PropiedadCadaLineaMideAncho(t *testing.T) {
	piezas := []string{"a", "Z", "漢", "字", "🚀", "Ａ", "\u0301", "é", "ñ", " ", "\t", "\n", "\x1b[2J", "\u202e", "✋", "-", "x"}
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		txt := func() string {
			var b strings.Builder
			for i := 0; i < 1+r.Intn(90); i++ {
				b.WriteString(piezas[r.Intn(len(piezas))])
			}
			return b.String()
		}
		tams := [][2]int{{60, 15}, {79, 24}, {80, 24}, {80, 23}, {100, 30}, {120, 32}, {61, 16}}
		tam := tams[r.Intn(len(tams))]
		utf := r.Intn(2) == 0
		cards := []boardcmd.Card{ttCardAncha("c-"+fmt.Sprint(semilla%97), txt(), txt()), ttInfraAPI(true), ttNucTenancy()}
		cards[1].Item.Titulo = txt()
		m := ttModelo(ttConfig(tam[0], tam[1], utf), ttTablero(cards...))
		h := ttHallazgo("h", finding.StatusOpen, "high", txt()+".go:3", txt())
		m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h}})
		for _, k := range []tablero.Tecla{"", "j", "4", "3", tablero.TeclaEnter} {
			if k != "" {
				m.Tecla(k, ttAhora)
			}
			for i, l := range ttLimpia(m) {
				n := ttCeldas(l)
				if n > tam[0] || tablero.Celdas(l) > tam[0] {
					t.Logf("semilla %d (%dx%d utf8=%v): la linea %d mide %d celdas: %q", semilla, tam[0], tam[1], utf, i+1, n, l)
					return false
				}
				c := ttCeldasDe(l)
				if i < tam[1]-2 && len(c) > 0 && ttEsEsquina(c[len(c)-1]) && n != tam[0] {
					t.Logf("semilla %d (%dx%d): la linea %d termina en una esquina y mide %d: %q", semilla, tam[0], tam[1], i+1, n, l)
					return false
				}
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 120}); err != nil {
		t.Fatalf("CA-577: %v", err)
	}
}
