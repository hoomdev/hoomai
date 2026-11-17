// La pantalla de la v3 (spec tablero-tu-siguiente-paso): CA-696 el diseno y
// el pliegue (re-expresa CA-550), CA-706 el foco (re-expresa CA-551 y la
// parte del foco de CA-553), CA-707 la seleccion y el detalle que la sigue
// (re-expresa CA-552 y CA-553) y CA-577 los anchos, sin cambios. Las zonas se
// leen de la pantalla por el lugar que les da doc.go ("The screen", "The
// layout of the body", "The boxes") con tv3bLeer (tt4b_fixtures_test.go) y se
// comparan con esas reglas, calculadas aca desde el contrato. Lo que la v2
// leia de un panel numerado se lee donde lo pone la tabla "Los nombres de la
// v2" del spec: [2] la lista, [0] el detalle, [1] la cabecera (y el detalle
// con v), [3] el grupo CORRIENDO de la lista, [4] la vista Hallazgos (H).
package tablero_test

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- las alturas del contrato

// tv3bPtaCuerpo es C = A-8, las lineas del cuerpo (doc.go, "The screen": las
// lineas 6..A-3).
func tv3bPtaCuerpo(alto int) int { return alto - 8 }

// tv3bPtaAltoLista es la altura de la lista en una columna con el foco en la
// lista (doc.go, "The layout of the body"): max(3, min(N, (C+1)/2)), N su
// altura natural (sus lineas + 2).
func tv3bPtaAltoLista(alto, lineas int) int {
	return max(3, min(lineas+2, (tv3bPtaCuerpo(alto)+1)/2))
}

// tv3bPtaLineasCorriendo son las lineas de la lista de
// ttTableroConCorriendo(n) entre 60 y 99 columnas (una linea por tarjeta):
// " CORRIENDO n" y sus n filas cuando n > 0, " EN COLA 2" y sus dos filas
// (uno y dos, en Backlog), y la linea de HECHO, que siempre esta.
func tv3bPtaLineasCorriendo(n int) int {
	l := 1 + 2 + 1
	if n > 0 {
		l += 1 + n
	}
	return l
}

// tv3bPtaLineasDorado son las lineas de la lista de ttModeloDorado entre 60
// y 99 columnas: cinco de grupo (TE TOCA 2, CORRIENDO 1, DETENIDA 1, EN COLA
// 2 y HECHO 3, cerrada) y seis filas.
const tv3bPtaLineasDorado = 11

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

// tv3bPtaLugar es donde esta una caja en la pantalla.
type tv3bPtaLugar struct {
	Fila, Col, Ancho, Alto int
	Plegado, Foco          bool
}

func tv3bPtaEn(c tv3bCaja) tv3bPtaLugar {
	return tv3bPtaLugar{c.Fila, c.Col, c.Ancho, c.Alto, c.Plegado, c.Foco}
}

// tv3bPtaRevisar exige el diseno de doc.go en m, de ancho x alto, con la
// lista de lineas lineas (en una columna) y el foco en foco: la cabecera en la
// linea 1, la caja TU SIGUIENTE PASO de Ancho celdas y 4 lineas sin foco en
// las lineas 2..5, el cuerpo en las C = A-8 lineas que siguen (dos zonas con
// 100 columnas o mas; una columna entre 60 y 99, la lista plegada con el foco
// en el detalle), el Registro plegado en la linea A-2 y la de teclas en la A.
func tv3bPtaRevisar(t *testing.T, que string, m *tablero.Modelo, ancho, alto, lineas int, foco tablero.Zona) {
	t.Helper()
	ttForma(t, "CA-696", m, ancho, alto, false)
	z := tv3bLeer(t, "CA-696: "+que, m)
	c := tv3bPtaCuerpo(alto)
	if z.Ancho != ancho || z.Caja.Ancho != ancho || z.Caja.Alto != 4 || z.Caja.Foco || z.Caja.Plegado {
		t.Fatalf("CA-696: %s: la caja TU SIGUIENTE PASO va en las lineas 2..5 con %d celdas, 4 lineas y sin foco: %+v\n%s",
			que, ancho, z.Caja.ttCaja, ttPantalla(m))
	}
	if !strings.HasPrefix(z.Cabecera, " hoom tablero · demo") {
		t.Fatalf("CA-696: %s: la linea 1 es la cabecera: %q", que, z.Cabecera)
	}
	var lista, detalle tv3bPtaLugar
	switch {
	case ancho >= 100:
		l := ancho * 2 / 5
		lista = tv3bPtaLugar{5, 0, l, c, false, foco == tablero.ZonaLista}
		detalle = tv3bPtaLugar{5, l, ancho - l, c, false, foco == tablero.ZonaDetalle}
	case foco == tablero.ZonaLista:
		h := tv3bPtaAltoLista(alto, lineas)
		lista = tv3bPtaLugar{5, 0, ancho, h, false, true}
		detalle = tv3bPtaLugar{5 + h, 0, ancho, c - h, false, false}
	default:
		lista = tv3bPtaLugar{5, 0, ancho, 1, true, false}
		detalle = tv3bPtaLugar{6, 0, ancho, c - 1, false, true}
	}
	if got := tv3bPtaEn(z.Lista); got != lista {
		t.Fatalf("CA-696: %s: la lista esta en la linea %d, celda %d, con %d celdas y %d lineas (plegada %v, foco %v); esta en %d,%d con %d celdas y %d lineas (plegada %v, foco %v):\n%s",
			que, lista.Fila+1, lista.Col, lista.Ancho, lista.Alto, lista.Plegado, lista.Foco,
			got.Fila+1, got.Col, got.Ancho, got.Alto, got.Plegado, got.Foco, ttPantalla(m))
	}
	if got := tv3bPtaEn(z.Detalle); got != detalle {
		t.Fatalf("CA-696: %s: el detalle esta en la linea %d, celda %d, con %d celdas y %d lineas (foco %v); esta en %d,%d con %d celdas y %d lineas (plegado %v, foco %v):\n%s",
			que, detalle.Fila+1, detalle.Col, detalle.Ancho, detalle.Alto, detalle.Foco,
			got.Fila+1, got.Col, got.Ancho, got.Alto, got.Plegado, got.Foco, ttPantalla(m))
	}
	if z.Detalle.Fila+z.Detalle.Alto != 5+c {
		t.Fatalf("CA-696: %s: el cuerpo ocupa las C=%d lineas 6..%d", que, c, alto-3)
	}
	reg := ttCeldasDe(z.Registro)
	if len(reg) != ancho || (reg[0] != "-" && reg[0] != "─") || !strings.HasPrefix(strings.Join(reg[1:], ""), "Registro") {
		t.Fatalf("CA-696: %s: el Registro va plegado en la linea A-2 (%d), de %d celdas y con el signo de linea simple: %q\n%s",
			que, alto-2, ancho, z.Registro, ttPantalla(m))
	}
	if z.Teclas == "" {
		t.Fatalf("CA-696: %s: la linea A es la de teclas:\n%s", que, ttPantalla(m))
	}
}

// ---------------------------------------------------------------- CA-696: el diseno

// CA-696 (re-expresa CA-550): con 100 columnas o mas, de arriba hacia abajo,
// la cabecera, la caja TU SIGUIENTE PASO de 4 lineas, el cuerpo con la lista
// a la izquierda en W*2/5 celdas y el detalle a la derecha, las dos de C =
// A-8 lineas, el Registro en una linea, la de mensajes y la de teclas. En
// varios tamanos, con 0 a 9 trabajos corriendo, y con el foco en cada zona
// (en dos zonas nada se pliega).
func TestCA696_CA550_DosZonas(t *testing.T) {
	for _, tam := range [][2]int{{100, 30}, {101, 24}, {120, 32}, {120, 36}, {150, 20}, {200, 50}, {100, 15}, {139, 17}} {
		for _, n := range []int{0, 1, 3, 5, 9} {
			que := fmt.Sprintf("%dx%d con %d corriendo", tam[0], tam[1], n)
			m := ttModelo(ttConfig(tam[0], tam[1], false), ttTableroConCorriendo(n))
			tv3bPtaRevisar(t, que, m, tam[0], tam[1], tv3bPtaLineasCorriendo(n), tablero.ZonaLista)
			m.Tecla(tablero.TeclaTab, ttAhora)
			tv3bPtaRevisar(t, que+" y el foco en el detalle", m, tam[0], tam[1], tv3bPtaLineasCorriendo(n), tablero.ZonaDetalle)
		}
	}
	// con UTF-8 las mismas cajas
	m := ttModeloDorado(ttConfig(120, 32, true))
	tv3bPtaRevisar(t, "120x32 con UTF-8", m, 120, 32, tv3bPtaLineasDorado, tablero.ZonaLista)
}

// CA-696 (re-expresa el pliegue de CA-550): entre 60 y 99 columnas, con el
// foco en la lista, la lista ocupa max(3, min(N, (C+1)/2)) lineas (N su
// altura natural) y el detalle el resto; con el foco en el detalle la lista
// se pliega a una linea y el detalle ocupa C-1. La zona con el foco nunca se
// pliega (la v2: "el panel con el foco nunca se pliega") y el detalle no se
// pliega nunca; con 100 columnas o mas no se pliega nada.
func TestCA696_CA550_ElPliegueEnUnaColumna(t *testing.T) {
	casos := []struct {
		ancho, alto, n int
	}{
		{60, 15, 0},  // N = 6, (C+1)/2 = 4: la mitad
		{80, 15, 5},  // N = 12
		{80, 24, 9},  // N = 16, (C+1)/2 = 8
		{99, 30, 1},  // N = 8 < 11: su altura natural
		{70, 20, 3},  // N = 10, 6
		{60, 16, 0},  // C = 8, (C+1)/2 = 4
		{79, 40, 0},  // N = 6, entra
		{79, 40, 9},  // N = 16 = la mitad
		{100, 15, 9}, // dos zonas: nada se pliega
		{120, 15, 0},
	}
	for _, c := range casos {
		for _, foco := range []tablero.Zona{tablero.ZonaLista, tablero.ZonaDetalle} {
			que := fmt.Sprintf("%dx%d con %d corriendo y el foco en %s", c.ancho, c.alto, c.n, tv3bZonaDe(foco))
			m := ttModelo(ttConfig(c.ancho, c.alto, false), ttTableroConCorriendo(c.n))
			if foco == tablero.ZonaDetalle {
				m.Tecla(tablero.TeclaTab, ttAhora)
			}
			if m.Foco() != foco {
				t.Fatalf("CA-706: %s: el foco esta en %s, Foco = %d", que, tv3bZonaDe(foco), int(m.Foco()))
			}
			tv3bPtaRevisar(t, que, m, c.ancho, c.alto, tv3bPtaLineasCorriendo(c.n), foco)
			z := tv3bLeer(t, "CA-696", m)
			if (foco == tablero.ZonaLista && z.Lista.Plegado) || z.Detalle.Plegado {
				t.Fatalf("CA-696: %s: la zona con el foco nunca se pliega, y el detalle nunca:\n%s", que, ttPantalla(m))
			}
		}
	}
}

// CA-696 (re-expresa CA-550): la caja plegada es una linea de su ancho: el
// signo de linea simple, su titulo y signos de linea hasta su ancho; el
// titulo de la lista plegada es el de la lista y " · <n> de <m>" cuando hay
// una fila elegida ("─Tarjetas · 1 de 7──"). El Registro plegado de la linea
// A-2 corta su texto a su ancho - 2 con el signo de corte (la v2: "-[1]
// Proyecto · check VE~-"). Lo que la v2 leia en [1] plegado (el check despues
// del titulo) esta en la v3 en la cabecera (CA-697): check VERDE, ROJO con su
// motivo, sin veredicto, y antes de la primera lectura del proyecto, nada del
// check.
func TestCA696_CA550_ElPlegadoEsSuTitulo(t *testing.T) {
	plegada := func(m *tablero.Modelo) string { return tv3bLista(t, "CA-696", m).Arriba }
	m := ttModeloDorado(ttConfig(80, 24, false))
	m.Tecla(tablero.TeclaTab, ttAhora)
	quiere := "-Tarjetas · 1 de 7" + strings.Repeat("-", 80-ttCeldas("-Tarjetas · 1 de 7"))
	if got := plegada(m); got != quiere {
		t.Fatalf("CA-696: a 80x24 con el foco en el detalle la lista va plegada como %q: %q\n%s", quiere, got, ttPantalla(m))
	}
	// el n de m sigue a la eleccion
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("j", ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora)
	if got := plegada(m); !strings.HasPrefix(got, "-Tarjetas · 2 de 7--") {
		t.Fatalf("CA-696: con la segunda fila elegida, la lista plegada dice ' · 2 de 7': %q", got)
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("G", ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora)
	if got := plegada(m); !strings.HasPrefix(got, "-Tarjetas · 7 de 7--") {
		t.Fatalf("CA-696: con la linea de HECHO elegida, la lista plegada dice ' · 7 de 7': %q", got)
	}
	// a 60x15 con UTF-8 ocupa las 60 celdas
	u := ttModeloDorado(ttConfig(60, 15, true))
	u.Tecla(tablero.TeclaTab, ttAhora)
	if q := "─Tarjetas · 1 de 7" + strings.Repeat("─", 60-ttCeldas("─Tarjetas · 1 de 7")); plegada(u) != q {
		t.Fatalf("CA-696: a 60x15 la lista va plegada como %q: %q\n%s", q, plegada(u), ttPantalla(u))
	}
	// en la vista Hallazgos, su titulo
	h := ttModeloDorado(ttConfig(80, 24, false))
	h.Tecla("H", ttAhora)
	h.Tecla(tablero.TeclaTab, ttAhora)
	if q := "-Hallazgos 2 · 1 de 2" + strings.Repeat("-", 80-ttCeldas("-Hallazgos 2 · 1 de 2")); plegada(h) != q {
		t.Fatalf("CA-696, CA-712: en la vista Hallazgos la lista plegada es %q: %q", q, plegada(h))
	}
	// el Registro: vacio, "─Registro" y signos de linea; con una entrada
	// larga, su texto cortado a Ancho - 2 con el signo de corte y un signo de
	// linea al final
	if r := tv3bRegistro(u); r != "─Registro"+strings.Repeat("─", 60-9) {
		t.Fatalf("CA-696, CA-714: el Registro vacio es '─Registro' y signos de linea hasta el ancho: %q", r)
	}
	largo := "una-tarjeta-con-un-slug-tan-largo-que-no-entra-en-la-linea"
	r := ttModelo(ttConfig(60, 15, false), ttTablero(ttBacklog(largo, "Larga")))
	ttUnaLanzar(t, "CA-560", "u", r.Tecla("u", ttAhora))
	cs := ttCeldasDe(tv3bRegistro(r))
	if len(cs) != 60 || cs[0] != "-" || !strings.HasPrefix(strings.Join(cs, ""), "-Registro · ") ||
		!strings.Contains(strings.Join(cs, ""), "Fijar para la terminal") || cs[58] != "~" || cs[59] != "-" {
		t.Fatalf("CA-696, CA-714: el Registro plegado mide 60 celdas y corta su texto a 58 con ~, seguido de un signo de linea: %q", strings.Join(cs, ""))
	}
	// la cabecera dice el check (lo que decia [1] plegado)
	p := ttModeloDorado(ttConfig(200, 15, false))
	if c := tv3bCabecera(p); !strings.Contains(c, "check VERDE") {
		t.Fatalf("CA-696, CA-697: la cabecera dice 'check VERDE' (lo que decia [1] plegado): %q", c)
	}
	pr := ttProyectoVerde()
	pr.Check.OK, pr.Check.Reason = false, checkcmd.ReasonDrift
	p.LeerProyecto(pr)
	if c, q := tv3bCabecera(p), "check ROJO: "+checkcmd.Motivo(pr.Check); !strings.Contains(c, q) {
		t.Fatalf("CA-696, CA-697: con el check rojo, la cabecera dice %q: %q", q, c)
	}
	pr.Check.Reason = checkcmd.ReasonNoVerdict
	p.LeerProyecto(pr)
	if c := tv3bCabecera(p); !strings.Contains(c, "sin veredicto todavia") || strings.Contains(c, "check ") {
		t.Fatalf("CA-696, CA-697: sin veredicto, la cabecera dice 'sin veredicto todavia': %q", c)
	}
	// antes de la primera lectura del proyecto, nada del check
	n := ttModelo(ttConfig(200, 15, false), ttTableroPrincipal())
	if c := tv3bCabecera(n); !strings.HasSuffix(c, "leyendo el proyecto...") || strings.Contains(c, "check") || strings.Contains(c, "veredicto") {
		t.Fatalf("CA-696, CA-697: antes de leer el proyecto la cabecera no dice el check: %q", c)
	}
}

// CA-696 (re-expresa CA-550): entre 60 y 99 columnas, una columna: la lista
// arriba y el detalle abajo, los dos de Ancho celdas; con el foco en la lista
// la lista ocupa max(3, min(N, (C+1)/2)) lineas y el detalle el resto; con el
// foco en el detalle (tab, → o l) la lista va plegada y el detalle ocupa C-1;
// esc la despliega de nuevo. El Registro siempre va plegado en la linea A-2.
func TestCA696_CA550_UnaColumna(t *testing.T) {
	for i, tam := range [][2]int{{60, 15}, {70, 24}, {79, 24}, {99, 24}, {79, 40}, {65, 18}, {99, 30}, {60, 16}} {
		ancho, alto := tam[0], tam[1]
		utf := i%2 == 0
		for _, entrar := range []tablero.Tecla{tablero.TeclaTab, tablero.TeclaDer, "l"} {
			que := fmt.Sprintf("%dx%d (utf8=%v)", ancho, alto, utf)
			m := ttModeloDorado(ttConfig(ancho, alto, utf))
			tv3bPtaRevisar(t, que, m, ancho, alto, tv3bPtaLineasDorado, tablero.ZonaLista)
			m.Tecla(entrar, ttAhora)
			tv3bPtaRevisar(t, fmt.Sprintf("%s tras %q", que, entrar), m, ancho, alto, tv3bPtaLineasDorado, tablero.ZonaDetalle)
			m.Tecla(tablero.TeclaEsc, ttAhora)
			tv3bPtaRevisar(t, que+" tras esc", m, ancho, alto, tv3bPtaLineasDorado, tablero.ZonaLista)
		}
	}
	// la vista Hallazgos (dos filas) se mide igual
	m := ttModeloDorado(ttConfig(80, 24, false))
	m.Tecla("H", ttAhora)
	tv3bPtaRevisar(t, "80x24 en la vista Hallazgos", m, 80, 24, 2, tablero.ZonaLista)
	m.Tecla(tablero.TeclaTab, ttAhora)
	tv3bPtaRevisar(t, "80x24 en el detalle de un hallazgo", m, 80, 24, 2, tablero.ZonaDetalle)
}

// CA-696, casos limite: el borde exacto entre una y dos zonas (99 y 100) y el
// minimo (60x15 dibuja, 59x15 y 60x14 no: la regla de la v2).
func TestCA696_CA550_TamanosBorde(t *testing.T) {
	for _, alto := range []int{15, 24} {
		m := ttModeloDorado(ttConfig(100, alto, false))
		if z := tv3bLeer(t, "CA-696", m); z.Lista.Ancho != 40 || z.Detalle.Col != 40 || z.Detalle.Ancho != 60 || z.Detalle.Fila != 5 {
			t.Fatalf("CA-696: a 100 columnas hay dos zonas (la lista de 40 celdas, el detalle en la celda 40): %+v %+v", z.Lista.ttCaja, z.Detalle.ttCaja)
		}
		m = ttModeloDorado(ttConfig(99, alto, false))
		if z := tv3bLeer(t, "CA-696", m); z.Lista.Ancho != 99 || z.Detalle.Col != 0 || z.Detalle.Ancho != 99 || z.Detalle.Fila != 5+z.Lista.Alto {
			t.Fatalf("CA-696: a 99 columnas hay una sola columna (el detalle de 99 celdas debajo de la lista): %+v %+v", z.Lista.ttCaja, z.Detalle.ttCaja)
		}
	}
	m := ttModeloDorado(ttConfig(60, 15, false))
	tv3bPtaRevisar(t, "60x15, el minimo", m, 60, 15, tv3bPtaLineasDorado, tablero.ZonaLista)
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
		t.Fatalf("CA-696 (CA-550): a %dx%d la pantalla es una linea %q y %d vacias: %q", ancho, alto, quiere, alto-1, p)
	}
	for i, l := range p[1:] {
		if l != "" {
			t.Fatalf("CA-696 (CA-550): a %dx%d la linea %d esta vacia: %q", ancho, alto, i+2, l)
		}
	}
}

// CA-696, caso limite (la regla de la v2, CA-550): por debajo de 60x15
// ninguna tecla salvo q y ctrl-c, y ningun evento del mouse, hace nada; al
// agrandar vuelve la misma pantalla de antes (la seleccion, la vista y el
// foco no se movieron).
func TestCA696_CA550_PorDebajoDelMinimoNadaHaceNada(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, false))
	m.Tecla(tablero.TeclaTab, ttAhora)
	antes := ttPantalla(m)
	m.Tamano(59, 15)
	ttPantallaChica(t, m, 59, 15, false)
	for _, k := range []tablero.Tecla{"1", "j", tablero.TeclaEnter, "w", "A", "u", "/", "?", "x", tablero.TeclaTab, tablero.TeclaEspacio,
		"e", "n", "H", "v", "L", "P", tablero.TeclaEsc, "G"} {
		ttSinOrdenes(t, "CA-696", fmt.Sprintf("%q a 59x15", k), m.Tecla(k, ttAhora))
		ttPantallaChica(t, m, 59, 15, false)
	}
	for _, r := range []tablero.Raton{{Accion: tablero.RatonClic, X: 0, Y: 0}, {Accion: tablero.RatonRuedaAbajo, X: 5, Y: 3}} {
		ttSinOrdenes(t, "CA-696", "el mouse a 59x15", m.Raton(r, ttAhora))
	}
	if m.Modal() != tablero.SinModal || m.Foco() != tablero.ZonaDetalle || m.Vista() != tablero.VistaTarjetas ||
		m.Lugar() != tablero.LugarVerTarjeta || len(m.Registro()) != 0 {
		t.Fatalf("CA-696: a 59x15 nada cambio: modal %q, foco %d, vista %q, lugar %q, registro %v", m.Modal(), int(m.Foco()), m.Vista(), m.Lugar(), m.Registro())
	}
	m.Tamano(80, 24)
	if got := ttPantalla(m); got != antes {
		t.Fatalf("CA-696: al agrandar vuelve la pantalla de antes:\n%s\nantes:\n%s", got, antes)
	}
	m.Tamano(30, 10)
	for _, k := range []tablero.Tecla{"q", tablero.TeclaCtrlC} {
		if os := m.Tecla(k, ttAhora); len(os) != 1 || os[0] != tablero.Orden(tablero.Salir{}) {
			t.Fatalf("CA-696: %q por debajo del minimo ordena Salir: %#v", k, os)
		}
	}
	// el corte del aviso con UTF-8
	u := ttModeloDorado(ttConfig(20, 5, true))
	ttPantallaChica(t, u, 20, 5, true)
}

// ---------------------------------------------------------------- CA-706: el foco

// CA-706 (re-expresa CA-551 y CA-520): al abrir el foco esta en la lista;
// siempre hay exactamente una zona con los signos del foco (= y # en ASCII)
// y es la de Foco(); la caja TU SIGUIENTE PASO nunca lo tiene. 1, 2, 3, 4 y
// 0, que en la v2 llevaban el foco a un panel, no hacen nada: ni el foco, ni
// la pantalla, ni ordenes, en la lista y en el detalle.
func TestCA706_CA551_CA520_LasTeclasDeNumeroNoHacenNada(t *testing.T) {
	m := tablero.Nuevo(ttConfig(120, 32, false))
	if m.Foco() != tablero.ZonaLista {
		t.Fatalf("CA-706: antes de la primera lectura el foco esta en la lista: %d", int(m.Foco()))
	}
	m = ttModeloDorado(ttConfig(120, 32, false))
	if m.Foco() != tablero.ZonaLista || strings.Join(tv3bFocos(t, "CA-706", m), ",") != "lista" {
		t.Fatalf("CA-706: al abrir el foco esta en la lista y solo la lista lleva sus signos: %d, %v\n%s",
			int(m.Foco()), tv3bFocos(t, "CA-706", m), ttPantalla(m))
	}
	for _, zona := range []tablero.Zona{tablero.ZonaLista, tablero.ZonaDetalle} {
		if zona == tablero.ZonaDetalle {
			m.Tecla(tablero.TeclaTab, ttAhora)
		}
		antes, lugar := ttPantalla(m), m.Lugar()
		for _, k := range []tablero.Tecla{"1", "2", "3", "4", "0"} {
			ttSinOrdenes(t, "CA-706", string(k), m.Tecla(k, ttAhora))
			if m.Foco() != zona || m.Lugar() != lugar || ttPantalla(m) != antes {
				t.Fatalf("CA-706: %q no hace nada (el foco en %s, Lugar %q): foco %d, lugar %q\n%s\nantes:\n%s",
					k, tv3bZonaDe(zona), lugar, int(m.Foco()), m.Lugar(), ttPantalla(m), antes)
			}
			if f := tv3bFocos(t, "CA-706", m); len(f) != 1 || f[0] != tv3bZonaDe(zona) {
				t.Fatalf("CA-706: tras %q solo %s lleva los signos del foco: %v", k, tv3bZonaDe(zona), f)
			}
		}
	}
}

// CA-706 (re-expresa CA-551): tab y shift-tab pasan el foco a la otra zona,
// en las dos vistas y con el detalle mostrando una tarjeta, un hallazgo, el
// proyecto o nada; al dejar el detalle del proyecto, el detalle vuelve a
// mostrar la eleccion de la lista.
func TestCA706_CA551_TabYShiftTab(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	pasos := []struct {
		k     tablero.Tecla
		zona  tablero.Zona
		lugar tablero.Lugar
	}{
		{tablero.TeclaTab, tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{tablero.TeclaTab, tablero.ZonaLista, tablero.LugarTarjetas},
		{tablero.TeclaShiftTab, tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{tablero.TeclaShiftTab, tablero.ZonaLista, tablero.LugarTarjetas},
		{tablero.TeclaTab, tablero.ZonaDetalle, tablero.LugarVerTarjeta},
		{tablero.TeclaShiftTab, tablero.ZonaLista, tablero.LugarTarjetas},
		{"H", tablero.ZonaLista, tablero.LugarHallazgos},
		{tablero.TeclaTab, tablero.ZonaDetalle, tablero.LugarVerHallazgo},
		{tablero.TeclaShiftTab, tablero.ZonaLista, tablero.LugarHallazgos},
		{tablero.TeclaShiftTab, tablero.ZonaDetalle, tablero.LugarVerHallazgo},
		{tablero.TeclaTab, tablero.ZonaLista, tablero.LugarHallazgos},
		{"H", tablero.ZonaLista, tablero.LugarTarjetas},
		{"v", tablero.ZonaDetalle, tablero.LugarVerProyecto},
		{tablero.TeclaTab, tablero.ZonaLista, tablero.LugarTarjetas},
	}
	for i, p := range pasos {
		ttSinOrdenes(t, "CA-706", string(p.k), m.Tecla(p.k, ttAhora))
		if m.Foco() != p.zona || m.Lugar() != p.lugar {
			t.Fatalf("CA-706: la tecla numero %d (%q) deja el foco en %s (%q): esta en %d (%q)", i+1, p.k, tv3bZonaDe(p.zona), p.lugar, int(m.Foco()), m.Lugar())
		}
		if f := tv3bFocos(t, "CA-706", m); len(f) != 1 || f[0] != tv3bZonaDe(p.zona) {
			t.Fatalf("CA-706: tras la tecla numero %d (%q) solo %s lleva los signos del foco: %v\n%s", i+1, p.k, tv3bZonaDe(p.zona), f, ttPantalla(m))
		}
	}
	if d := tv3bDetalle(t, "CA-706", m); d.Titulo != "Tenancy del nucleo" || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("CA-706, CA-713: al dejar el detalle del proyecto, el detalle vuelve a la eleccion de la lista: %q, Elegida %q", d.Titulo, m.Elegida())
	}
	// el detalle sin nada que mostrar
	tv3bEnLugar(t, m, tablero.LugarVerNada)
	m.Tecla(tablero.TeclaShiftTab, ttAhora)
	if m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas {
		t.Fatalf("CA-706: shift-tab en el detalle sin nada vuelve a la lista: %d %q", int(m.Foco()), m.Lugar())
	}
}

// CA-706 (re-expresa CA-551): con color, el borde de la zona con foco (el
// titulo incluido) va en verde y negrita (1;32); el de la otra zona y el de
// la caja TU SIGUIENTE PASO, no.
func TestCA706_CA551_ElFocoConColor(t *testing.T) {
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	m := ttModeloDorado(cfg)
	p := m.Pantalla()
	// la lista empieza en la linea 6, celda 0, y ocupa C = 24 lineas
	if !strings.HasPrefix(p[5], "\x1b[1;32m#Tarjetas=") {
		t.Fatalf("CA-706: el borde de arriba de la lista con el foco, titulo incluido, va en 1;32: %q", p[5])
	}
	if strings.HasPrefix(p[1], "\x1b[1;32m") || !strings.HasPrefix(ttSinSGR(p[1]), "+TU SIGUIENTE PASO-") {
		t.Fatalf("CA-706: la caja TU SIGUIENTE PASO nunca tiene el foco: %q", p[1])
	}
	for i := 6; i < 5+24-1; i++ {
		if !strings.HasPrefix(p[i], "\x1b[1;32m#\x1b[0m") {
			t.Fatalf("CA-706: el lado izquierdo de la lista con foco va en 1;32 (linea %d): %q", i+1, p[i])
		}
	}
	if !strings.Contains(p[5], "+Tenancy del nucleo-") || strings.Contains(p[5], "\x1b[1;32m+") {
		t.Fatalf("CA-706: el detalle sin foco tiene los signos simples, sin el verde: %q", p[5])
	}
	m.Tecla(tablero.TeclaTab, ttAhora)
	p = m.Pantalla()
	if strings.HasPrefix(p[5], "\x1b[") || !strings.HasPrefix(p[5], "+Tarjetas-") || !strings.Contains(p[5], "\x1b[1;32m#Tenancy del nucleo") {
		t.Fatalf("CA-706: con el foco en el detalle, su borde va en 1;32 y el de la lista ya no: %q", p[5])
	}
}

// CA-706 (propiedad; re-expresa CA-551): ninguna secuencia de teclas de
// moverse deja la pantalla sin una zona con foco o con dos: siempre
// exactamente una, la de Foco(), y la caja nunca. (enter ya no es una tecla
// de moverse: abre el modal del paso; H y v si lo son.)
func TestCA706_CA551_PropiedadSiempreUnFoco(t *testing.T) {
	teclas := []tablero.Tecla{"1", "2", "3", "4", "0", tablero.TeclaTab, tablero.TeclaShiftTab, tablero.TeclaEsc,
		"j", "k", "g", "G", "h", "l", tablero.TeclaIzq, tablero.TeclaDer, tablero.TeclaArriba, tablero.TeclaAbajo, tablero.TeclaPgAbajo, "x",
		"H", "v"}
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		tams := [][2]int{{120, 32}, {100, 30}, {80, 24}, {70, 20}, {60, 15}, {99, 15}}
		tam := tams[r.Intn(len(tams))]
		m := ttModeloDorado(ttConfig(tam[0], tam[1], r.Intn(2) == 0))
		for i := 0; i < 40; i++ {
			k := teclas[r.Intn(len(teclas))]
			m.Tecla(k, ttAhora)
			if m.Modal() != tablero.SinModal {
				t.Logf("semilla %d: %q abrio el modal %q", semilla, k, m.Modal())
				return false
			}
			f := tv3bFocos(t, "CA-706", m)
			if len(f) != 1 || f[0] != tv3bZonaDe(m.Foco()) {
				t.Logf("semilla %d (%dx%d): tras %q las zonas con foco son %v y Foco es %d:\n%s", semilla, tam[0], tam[1], k, f, int(m.Foco()), ttPantalla(m))
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 80}); err != nil {
		t.Fatalf("CA-706: %v", err)
	}
}

// ---------------------------------------------------------------- CA-707: la seleccion

// CA-707 (re-expresa CA-552 y CA-520): en la lista, ↓ j ↑ k mueven la
// seleccion una fila (las lineas de grupo no son filas; se detiene en las
// puntas), g y G van a la primera y a la ultima (la linea de HECHO). La fila
// muestra el titulo de la tarjeta; Elegida dice cual es.
func TestCA707_CA552_CA520_MoverEnTarjetas(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	orden := tv3bOrdenDorado
	quiere := func(slug string) string {
		if slug == "" {
			return "HECHO 3 >"
		}
		return tv3bTitulos[slug]
	}
	revisar := func(que, slug string) {
		t.Helper()
		if got := tv3bTituloElegido(t, "CA-707", m); got != quiere(slug) || m.Elegida() != slug {
			t.Fatalf("CA-707: %s elige %q (%q): la fila elegida es %q, Elegida %q\n%s", que, slug, quiere(slug), got, m.Elegida(), ttPantalla(m))
		}
	}
	revisar("al abrir", orden[0])
	for i, k := range []tablero.Tecla{"j", tablero.TeclaAbajo, "j", "j", tablero.TeclaAbajo, "j", "j"} {
		m.Tecla(k, ttAhora)
		revisar(fmt.Sprintf("el %q numero %d", k, i+1), orden[min(i+1, len(orden)-1)])
	}
	m.Tecla("g", ttAhora)
	revisar("g", "nuc-tenancy")
	m.Tecla("k", ttAhora)
	revisar("k en la primera", "nuc-tenancy")
	m.Tecla("G", ttAhora)
	revisar("G (la linea de HECHO)", "")
	m.Tecla(tablero.TeclaArriba, ttAhora)
	revisar("↑ desde HECHO", "docs-guia")
}

// CA-707 (re-expresa CA-552): la vista Hallazgos tiene su propia seleccion,
// que queda mientras el foco esta en el detalle (con su ▸, "> " en ASCII) y
// se mueve con j k g G; la de la lista de tarjetas queda en su tarjeta al
// abrir y cerrar la vista. [3] ya no tiene una seleccion propia: sus
// trabajos son las filas del grupo CORRIENDO de la misma lista.
func TestCA707_CA552_CadaListaSuSeleccion(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	tv3bIrA(t, m, "infra-api")
	if got := tv3bTituloElegido(t, "CA-707", m); got != "Convenciones de la API" {
		t.Fatalf("CA-707: el trabajo que corre es una fila de la lista (CORRIENDO): %q", got)
	}
	m.Tecla("j", ttAhora)
	if got := tv3bTituloElegido(t, "CA-707", m); got != "Pagos caidos" || m.Elegida() != "pagos-caidos" {
		t.Fatalf("CA-707: j desde CORRIENDO elige la de DETENIDA: %q", got)
	}
	elegidaH := func() string { return tv3bElegida(t, "CA-707", m) }
	m.Tecla("H", ttAhora)
	if got := elegidaH(); !strings.HasPrefix(got, "high ") {
		t.Fatalf("CA-707: la vista Hallazgos empieza en su primera fila (el high): %q\n%s", got, ttPantalla(m))
	}
	m.Tecla("j", ttAhora)
	if got := elegidaH(); !strings.HasPrefix(got, "low ") {
		t.Fatalf("CA-707: j en la vista Hallazgos elige la segunda fila: %q", got)
	}
	m.Tecla(tablero.TeclaTab, ttAhora)
	if got := elegidaH(); !strings.HasPrefix(got, "low ") || m.Foco() != tablero.ZonaDetalle {
		t.Fatalf("CA-707: la seleccion de la vista Hallazgos queda con el foco en el detalle: %q", got)
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("G", ttAhora)
	if got := elegidaH(); !strings.HasPrefix(got, "low ") {
		t.Fatalf("CA-707: G en la vista Hallazgos va a la ultima: %q", got)
	}
	m.Tecla("g", ttAhora)
	if got := elegidaH(); !strings.HasPrefix(got, "high ") {
		t.Fatalf("CA-707: g en la vista Hallazgos va a la primera: %q", got)
	}
	// y la de la lista de tarjetas quedo en su tarjeta
	m.Tecla("H", ttAhora)
	if got := tv3bTituloElegido(t, "CA-707", m); got != "Pagos caidos" || m.Elegida() != "pagos-caidos" {
		t.Fatalf("CA-707, CA-712: al cerrar la vista Hallazgos la seleccion de la lista sigue en su tarjeta: %q, Elegida %q", got, m.Elegida())
	}
}

// tv3bPtaCelda es una celda de una linea con color tal como la deja la
// terminal: su texto, si va en video inverso y su color de letra (30..37,
// 90..97; 0 sin color). Los SGR se acumulan hasta un 0: 7 prende el inverso,
// 27 lo apaga, 39 vuelve al color de letra de siempre.
type tv3bPtaCelda struct {
	Texto   string
	Inverso bool
	Letra   int
}

// tv3bPtaCeldasSGR son las celdas de una linea de Pantalla con su estilo.
func tv3bPtaCeldasSGR(l string) []tv3bPtaCelda {
	var out []tv3bPtaCelda
	inv, letra := false, 0
	for {
		i := strings.Index(l, "\x1b[")
		texto := l
		if i >= 0 {
			texto = l[:i]
		}
		for _, c := range ttCeldasDe(texto) {
			out = append(out, tv3bPtaCelda{c, inv, letra})
		}
		if i < 0 {
			return out
		}
		l = l[i+2:]
		j := strings.IndexByte(l, 'm')
		if j < 0 {
			return out
		}
		for _, p := range strings.Split(l[:j], ";") {
			n, _ := strconv.Atoi(p) // "" es 0
			switch {
			case n == 0:
				inv, letra = false, 0
			case n == 7:
				inv = true
			case n == 27:
				inv = false
			case n == 39:
				letra = 0
			case n >= 30 && n <= 37, n >= 90 && n <= 97:
				letra = n
			}
		}
		l = l[j+1:]
	}
}

// tv3bPtaLista son las l primeras celdas de una linea con color (las de la
// lista con 100 columnas o mas); el test falla si la linea es mas corta.
func tv3bPtaLista(t *testing.T, linea string, l int) []tv3bPtaCelda {
	t.Helper()
	cs := tv3bPtaCeldasSGR(linea)
	if len(cs) < l {
		t.Fatalf("CA-696: la linea llega al menos hasta el lado derecho de la lista (celda %d): %q", l-1, linea)
	}
	return cs[:l]
}

// tv3bPtaLineaDe es el indice de la primera linea de la pantalla que, sin
// SGR, empieza con s, y la linea cruda; -1 si ninguna.
func tv3bPtaLineaDe(m *tablero.Modelo, s string) (int, string) {
	for i, l := range m.Pantalla() {
		if strings.HasPrefix(ttSinSGR(l), s) {
			return i, l
		}
	}
	return -1, ""
}

// CA-707 (re-expresa CA-552; CA-701): con color, la primera linea de la fila
// elegida de la zona con foco es una barra en video inverso sobre su linea
// interior entera (sus w-2 celdas, el relleno incluido) y sus piezas guardan
// sus colores (7;33, 7;32...): el signo rojo de DETENIDA va en 7;31 dentro de
// la barra (la v2: el slug de la interrumpida en 7;31). La segunda linea de
// la fila no lleva la barra, ni los lados; sin foco queda solo el ▸.
func TestCA707_CA552_LaBarraEnVideoInverso(t *testing.T) {
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	m := ttModeloDorado(cfg)
	const l = 120 * 2 / 5 // la lista: celdas 0..47, el camino en las 39..46
	i, cruda := tv3bPtaLineaDe(m, "# > Tenancy del nucleo")
	if i < 0 {
		t.Fatalf("CA-707: la fila elegida de la lista con foco es '# > Tenancy del nucleo':\n%s", ttPantalla(m))
	}
	cs := tv3bPtaCeldasSGR(cruda)
	if len(cs) < l || cs[0].Inverso || cs[l-1].Inverso {
		t.Fatalf("CA-707: los lados de la lista no van en video inverso: %q", cruda)
	}
	for k := 1; k < l-1; k++ {
		if !cs[k].Inverso {
			t.Fatalf("CA-707: la fila elegida con foco es una barra en video inverso sobre sus %d celdas interiores (la celda %d no): %q", l-2, k, cruda)
		}
	}
	tuAprobacion := 39 + 2 // Tu aprobacion es la tercera columna
	if c := cs[tuAprobacion]; c.Texto != "!" || c.Letra != 33 || cs[39].Letra != 32 || cs[46].Letra != 90 {
		t.Fatalf("CA-707, CA-702: dentro de la barra el camino guarda sus colores (32, 33, 90): %+v %+v %+v", cs[39], c, cs[46])
	}
	for k, c := range tv3bPtaLista(t, m.Pantalla()[i+1], l) {
		if c.Inverso {
			t.Fatalf("CA-707: solo la primera linea de la fila elegida lleva la barra (la segunda, celda %d): %q", k, m.Pantalla()[i+1])
		}
	}
	// el signo rojo de la detenida dentro de la barra
	tv3bIrA(t, m, "pagos-caidos")
	_, pagos := tv3bPtaLineaDe(m, "# > Pagos caidos")
	writer := 39 + 4 // Writer es la quinta columna
	if c := tv3bPtaCeldasSGR(pagos); len(c) < l || c[writer].Texto != "x" || !c[writer].Inverso || c[writer].Letra != 31 {
		t.Fatalf("CA-707, CA-702: el signo de DETENIDA, elegido con foco, va en 7;31: %q", pagos)
	}
	// sin foco, la fila elegida no lleva video inverso
	m.Tecla(tablero.TeclaTab, ttAhora)
	_, sin := tv3bPtaLineaDe(m, "| > Pagos caidos")
	if sin == "" {
		t.Fatalf("CA-707: sin foco la fila elegida sigue con su >:\n%s", ttPantalla(m))
	}
	for k, c := range tv3bPtaLista(t, sin, l) {
		if c.Inverso {
			t.Fatalf("CA-707: en la zona sin foco la fila elegida no lleva video inverso (celda %d): %q", k, sin)
		}
	}
	// sin color, el ▸ y nada mas
	s := ttModeloDorado(ttConfig(120, 32, false))
	if !ttTiene(s, "# > Tenancy del nucleo") {
		t.Fatalf("CA-707: sin color, la seleccion es el >:\n%s", ttPantalla(s))
	}
	s.Tecla(tablero.TeclaTab, ttAhora)
	if !ttTiene(s, "| > Tenancy del nucleo") {
		t.Fatalf("CA-707: sin color y sin foco, la seleccion es el >:\n%s", ttPantalla(s))
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

// CA-707 (re-expresa CA-552): cuando las filas no entran, la lista se
// desplaza lo minimo para que la tarjeta elegida se vea entera (al bajar de
// la vista su ultima linea queda en la ultima linea, al subir su primera en
// la primera) y su borde de abajo dice "<n> de <m>" (n el lugar de la elegida
// entre las filas, m cuantas son: las lineas de grupo no cuentan, la de HECHO
// si), antes de la esquina con un signo de linea entre.
func TestCA707_CA552_DesplazamientoMinimoYNDeM(t *testing.T) {
	// 80x24: una columna, C = 16; la lista tiene " EN COLA 30", 30 filas y
	// la de HECHO: 32 lineas (N = 34), ocupa (C+1)/2 = 8 (6 interiores) y
	// tiene 31 filas.
	m := ttModelo(ttConfig(80, 24, false), ttTableroLargo(30))
	lista := func() tv3bCaja { return tv3bLista(t, "CA-707", m) }
	vista := func() []string { return lista().ttTexto() }
	if v := vista(); len(v) != 6 {
		t.Fatalf("CA-696: a 80x24 la lista de 32 lineas tiene 6 lineas interiores: %q", v)
	}
	if c := lista(); !strings.HasSuffix(c.Abajo, "=1 de 31=#") || c.Pie != "1 de 31" {
		t.Fatalf("CA-707: el borde de abajo dice '1 de 31' antes de la esquina, con un signo de linea entre: %q", c.Abajo)
	}
	if v := vista(); v[0] != " EN COLA 30" || !strings.HasPrefix(v[1], " > Tarjeta 00 ") {
		t.Fatalf("CA-707: la vista empieza arriba: %q", v)
	}
	for i := 0; i < 4; i++ {
		m.Tecla("j", ttAhora)
	}
	if v := vista(); !strings.HasPrefix(v[5], " > Tarjeta 04 ") || v[0] != " EN COLA 30" {
		t.Fatalf("CA-707: dentro de la vista, moverse no la desplaza: %q", v)
	}
	m.Tecla("j", ttAhora)
	// Tarjeta 05 es la linea 7 de 32: no entraba, ahora es la ultima de la vista
	v := vista()
	if !strings.HasPrefix(v[5], " > Tarjeta 05 ") || !strings.HasPrefix(v[0], "   Tarjeta 00 ") {
		t.Fatalf("CA-707: al bajar de la vista, la elegida queda en su ultima linea (desplazamiento minimo): %q", v)
	}
	if c := lista(); !strings.HasSuffix(c.Abajo, "=6 de 31=#") {
		t.Fatalf("CA-707: el borde dice '6 de 31': %q", c.Abajo)
	}
	for i := 0; i < 3; i++ {
		m.Tecla("k", ttAhora)
	}
	if v := vista(); !strings.HasPrefix(v[2], " > Tarjeta 02 ") || !strings.HasPrefix(v[0], "   Tarjeta 00 ") {
		t.Fatalf("CA-707: subir dentro de la vista no la desplaza: %q", v)
	}
	m.Tecla("G", ttAhora)
	if v := vista(); v[5] != " > HECHO 0 >" {
		t.Fatalf("CA-707: G lleva a la ultima fila (HECHO), al final de la vista: %q", v)
	}
	if c := lista(); !strings.HasSuffix(c.Abajo, "=31 de 31=#") {
		t.Fatalf("CA-707: el borde dice '31 de 31': %q", c.Abajo)
	}
	m.Tecla("g", ttAhora)
	if v := vista(); !strings.HasPrefix(v[0], " > Tarjeta 00 ") {
		t.Fatalf("CA-707: g vuelve a la primera fila, que al subir de la vista queda en su primera linea: %q", v)
	}
	// cuando todo entra, no hay "<n> de <m>"
	c := ttModelo(ttConfig(80, 24, false), ttTableroLargo(2))
	if a := tv3bLista(t, "CA-707", c); strings.Contains(a.Abajo, " de ") || a.Pie != "" {
		t.Fatalf("CA-707: si las filas entran el borde no dice '<n> de <m>': %q", a.Abajo)
	}
	// con 100 columnas o mas cada tarjeta tiene dos lineas y se ve entera: a
	// 120x15 la lista tiene 5 lineas interiores; Tarjeta 02 (lineas 6 y 7 de
	// la lista) baja de la vista y su segunda linea queda en la ultima
	a := ttModelo(ttConfig(120, 15, false), ttTableroLargo(30))
	a.Tecla("j", ttAhora)
	a.Tecla("j", ttAhora)
	if v := tv3bLista(t, "CA-707", a).ttTexto(); len(v) != 5 || !strings.HasPrefix(v[3], " > Tarjeta 02 ") || v[4] != "   falta el spec de la tarjeta" {
		t.Fatalf("CA-707: la tarjeta elegida se ve entera, sus dos lineas al final de la vista: %q", v)
	}
	if p := tv3bLista(t, "CA-707", a).Pie; p != "3 de 31" {
		t.Fatalf("CA-707: el borde dice '3 de 31': %q", p)
	}
}

// CA-707 (re-expresa CA-552), caso limite: mas filas que alto en el grupo
// CORRIENDO de la lista (la v2: [3]) y en la vista Hallazgos (la v2: [4]): se
// desplazan y el borde de abajo dice "<n> de <m>".
func TestCA707_CA552_CorriendoYHallazgosConMasFilasQueAlto(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTableroConCorriendo(9))
	var hs []finding.Item
	for i := 0; i < 8; i++ {
		hs = append(hs, ttHallazgo(fmt.Sprintf("h%d", i), finding.StatusOpen, "medium", fmt.Sprintf("a%d.go:%d", i, i+1), "d"))
	}
	m.LeerHallazgos(tablero.Hallazgos{Items: hs})
	// la lista: " CORRIENDO 9" y 9 filas, " EN COLA 2" y 2, HECHO: 14 lineas
	// (N = 16), 8 lineas (6 interiores), 12 filas
	c := tv3bLista(t, "CA-707", m)
	if c.Alto != 8 || !strings.HasSuffix(c.Abajo, "=1 de 12=#") {
		t.Fatalf("CA-707: la lista con 9 trabajos ocupa 8 lineas y su borde dice '1 de 12': %+v", c.ttCaja)
	}
	tv3bIrA(t, m, "corre-8")
	c = tv3bLista(t, "CA-707", m)
	if !strings.HasSuffix(c.Abajo, "=9 de 12=#") || !strings.HasPrefix(c.Dentro[len(c.Dentro)-1], " > Corre ") {
		t.Fatalf("CA-707: el ultimo trabajo, elegido, queda a la vista: %+v", c.ttCaja)
	}
	m.Tecla("G", ttAhora)
	if c = tv3bLista(t, "CA-707", m); !strings.HasSuffix(c.Abajo, "=12 de 12=#") || strings.TrimRight(c.Dentro[len(c.Dentro)-1], " ") != " > HECHO 0 >" {
		t.Fatalf("CA-707: G lleva a la linea de HECHO, a la vista: %+v", c.ttCaja)
	}
	m.Tecla("H", ttAhora)
	m.Tecla("j", ttAhora)
	m.Tecla("j", ttAhora)
	m.Tecla("j", ttAhora)
	h := tv3bLista(t, "CA-707", m)
	if !strings.HasSuffix(h.Abajo, "=4 de 8=#") || ttElegidaEn(h.ttCaja) == "" {
		t.Fatalf("CA-707: la vista Hallazgos con 8 hallazgos se desplaza y dice '4 de 8': %+v", h.ttCaja)
	}
	m.Tecla("G", ttAhora)
	if h = tv3bLista(t, "CA-707", m); !strings.HasSuffix(h.Abajo, "=8 de 8=#") || !strings.HasPrefix(h.Dentro[len(h.Dentro)-1], " > medium ") {
		t.Fatalf("CA-707: G en la vista Hallazgos lleva a la ultima, a la vista: %+v", h.ttCaja)
	}
}

// ---------------------------------------------------------------- CA-706 y CA-707: lo que muestra el detalle

// CA-707 (re-expresa CA-553, CA-520 y CA-521): el detalle muestra lo elegido
// en la lista y cambia con cada movimiento, sin enter: la tarjeta (su titulo
// y, a la derecha del borde, su slug), nada en la linea de HECHO, el
// proyecto con v, el trabajo del grupo CORRIENDO (la v2: [3]), el hallazgo
// de la vista Hallazgos ("hallazgo <id>"; la v2: [4]). Elegida y
// ArchivoElegido dicen que leer.
func TestCA707_CA706_CA553_CA520_CA521_ElDetalleSigueALaSeleccion(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	revisar := func(que, titulo, derecha, elegida string) {
		t.Helper()
		d := tv3bDetalle(t, "CA-707", m)
		if d.Titulo != titulo || d.Derecha != derecha || m.Elegida() != elegida {
			t.Fatalf("CA-707: %s, el detalle muestra %q (%q a la derecha), Elegida %q: muestra %q (%q), Elegida %q\n%s",
				que, titulo, derecha, elegida, d.Titulo, d.Derecha, m.Elegida(), ttPantalla(m))
		}
	}
	revisar("al abrir", "Tenancy del nucleo", "nuc-tenancy", "nuc-tenancy")
	m.Tecla("j", ttAhora)
	revisar("tras j, sin enter", "Precios por region", "precios", "precios")
	m.Tecla("G", ttAhora)
	revisar("en la linea de HECHO", "", "", "")
	m.Tecla("v", ttAhora)
	revisar("con v", "Proyecto", "", "")
	m.Tecla(tablero.TeclaEsc, ttAhora)
	revisar("de vuelta en la lista, sobre HECHO", "", "", "")
	tv3bIrA(t, m, "infra-api")
	revisar("en el grupo CORRIENDO", "Convenciones de la API", "infra-api", "infra-api")
	m.Tecla("j", ttAhora)
	revisar("en DETENIDA", "Pagos caidos", "pagos-caidos", "pagos-caidos")
	m.Tecla("H", ttAhora)
	revisar("en la vista Hallazgos (el high primero)", "hallazgo h-high", "", "")
	if r, l := m.ArchivoElegido(); r != "internal/api/router.go" || l != 41 {
		t.Fatalf("CA-559: ArchivoElegido es el archivo del hallazgo y su linea: %q %d", r, l)
	}
	m.Tecla("j", ttAhora)
	revisar("tras j en la vista Hallazgos", "hallazgo h-low", "", "")
	if r, l := m.ArchivoElegido(); r != "internal/tenant/tenant.go" || l != 7 {
		t.Fatalf("CA-559: ArchivoElegido sigue al hallazgo elegido: %q %d", r, l)
	}
	// de vuelta en las tarjetas: su tarjeta
	m.Tecla("H", ttAhora)
	revisar("de vuelta en las tarjetas", "Pagos caidos", "pagos-caidos", "pagos-caidos")
	if r, _ := m.ArchivoElegido(); r != "" {
		t.Fatalf("CA-559: sin hallazgo en el detalle, ArchivoElegido es \"\": %q", r)
	}
}

// CA-706 (re-expresa CA-553): →, l y tab pasan el foco de la lista al detalle
// (que sigue mostrando lo mismo; en la vista Hallazgos tambien enter), y esc,
// ← y h lo devuelven a la lista; el Lugar dice que muestra. En la linea de
// HECHO enter la abre y la cierra sin mover el foco; → y l pasan el foco al
// detalle sin nada (en la v2 la abrian: ahora solo enter). 0 no hace nada.
func TestCA706_CA553_EntrarYVolverDelDetalle(t *testing.T) {
	casos := []struct {
		que     string
		ir      func(m *tablero.Modelo)
		lugar   tablero.Lugar
		ver     tablero.Lugar
		entrars []tablero.Tecla
	}{
		{"una tarjeta", func(*tablero.Modelo) {}, tablero.LugarTarjetas, tablero.LugarVerTarjeta,
			[]tablero.Tecla{tablero.TeclaDer, "l", tablero.TeclaTab}},
		{"el grupo CORRIENDO", func(m *tablero.Modelo) { tv3bIrA(t, m, "infra-api") }, tablero.LugarTarjetas, tablero.LugarVerTarjeta,
			[]tablero.Tecla{tablero.TeclaDer, "l", tablero.TeclaTab}},
		{"la vista Hallazgos", func(m *tablero.Modelo) { m.Tecla("H", ttAhora) }, tablero.LugarHallazgos, tablero.LugarVerHallazgo,
			[]tablero.Tecla{tablero.TeclaDer, "l", tablero.TeclaTab, tablero.TeclaEnter}},
	}
	for _, c := range casos {
		for _, entrar := range c.entrars {
			for _, volver := range []tablero.Tecla{tablero.TeclaEsc, tablero.TeclaIzq, "h"} {
				m := ttModeloDorado(ttConfig(120, 32, false))
				c.ir(m)
				antes := tv3bDetalle(t, "CA-706", m).Titulo
				if m.Lugar() != c.lugar || m.Foco() != tablero.ZonaLista {
					t.Fatalf("CA-706: en %s con el foco en la lista el Lugar es %q, es %q", c.que, c.lugar, m.Lugar())
				}
				ttSinOrdenes(t, "CA-706", string(entrar), m.Tecla(entrar, ttAhora))
				if m.Foco() != tablero.ZonaDetalle || m.Lugar() != c.ver {
					t.Fatalf("CA-706: %q en %s pasa el foco al detalle (%q): foco %d, lugar %q", entrar, c.que, c.ver, int(m.Foco()), m.Lugar())
				}
				if got := tv3bDetalle(t, "CA-706", m).Titulo; got != antes {
					t.Fatalf("CA-706: con el foco en el detalle sigue mostrando lo de %s: %q, se esperaba %q", c.que, got, antes)
				}
				ttSinOrdenes(t, "CA-706", string(volver), m.Tecla(volver, ttAhora))
				if m.Foco() != tablero.ZonaLista || m.Lugar() != c.lugar {
					t.Fatalf("CA-706: %q en el detalle devuelve el foco a la lista de %s: foco %d, lugar %q", volver, c.que, int(m.Foco()), m.Lugar())
				}
			}
		}
	}
	// el proyecto: v le da el foco al detalle; esc, ← y h lo devuelven y el
	// detalle vuelve a la eleccion de la lista
	for _, volver := range []tablero.Tecla{tablero.TeclaEsc, tablero.TeclaIzq, "h"} {
		m := ttModeloDorado(ttConfig(120, 32, false))
		ttSinOrdenes(t, "CA-706", "v", m.Tecla("v", ttAhora))
		if m.Foco() != tablero.ZonaDetalle || m.Lugar() != tablero.LugarVerProyecto || tv3bDetalle(t, "CA-713", m).Titulo != "Proyecto" {
			t.Fatalf("CA-706, CA-713: v muestra el proyecto en el detalle, con el foco: foco %d, lugar %q", int(m.Foco()), m.Lugar())
		}
		m.Tecla(volver, ttAhora)
		if m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas || tv3bDetalle(t, "CA-706", m).Titulo != "Tenancy del nucleo" {
			t.Fatalf("CA-706: %q desde el proyecto vuelve a la lista y el detalle a su tarjeta: foco %d, lugar %q, %q",
				volver, int(m.Foco()), m.Lugar(), tv3bDetalle(t, "CA-706", m).Titulo)
		}
	}
	// HECHO: enter la abre y la cierra sin mover el foco
	m := ttModeloDorado(ttConfig(120, 32, false))
	tv3bIrAHecho(t, m)
	ttSinOrdenes(t, "CA-707", "enter en HECHO", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Foco() != tablero.ZonaLista || m.Modal() != tablero.SinModal || !ttTiene(m, "# > HECHO 3 v") || !ttTiene(m, "#   Viejo 1 ") {
		t.Fatalf("CA-707: enter sobre la linea de HECHO la abre (' HECHO 3 v', con sus tarjetas) sin mover el foco:\n%s", ttPantalla(m))
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if !ttTiene(m, "# > HECHO 3 >") || ttTiene(m, "Viejo 1") {
		t.Fatalf("CA-707: enter otra vez la cierra:\n%s", ttPantalla(m))
	}
	// 0 no hace nada; → y l pasan el foco al detalle sin nada, sin abrir HECHO
	m.Tecla("0", ttAhora)
	if m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas {
		t.Fatalf("CA-706: 0 no hace nada: foco %d, lugar %q", int(m.Foco()), m.Lugar())
	}
	for _, k := range []tablero.Tecla{tablero.TeclaDer, "l", tablero.TeclaTab} {
		ttSinOrdenes(t, "CA-706", string(k)+" en HECHO", m.Tecla(k, ttAhora))
		if m.Lugar() != tablero.LugarVerNada || m.Foco() != tablero.ZonaDetalle || ttTiene(m, "Viejo 1") || !ttTiene(m, "| > HECHO 3 >") {
			t.Fatalf("CA-706: %q sobre la linea de HECHO pasa el foco al detalle sin nada (LugarVerNada), con HECHO cerrada: %q\n%s", k, m.Lugar(), ttPantalla(m))
		}
		m.Tecla(tablero.TeclaEsc, ttAhora)
		if m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas {
			t.Fatalf("CA-706: esc desde el detalle sin nada vuelve a la lista: %d %q", int(m.Foco()), m.Lugar())
		}
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
// titulo y en su plain no corre el borde derecho del detalle (la v2: [0]):
// cada linea de la caja TU SIGUIENTE PASO, del cuerpo y del Registro mide
// exactamente Ancho celdas, y la esquina de la derecha del detalle esta en la
// ultima celda. (La cabecera tiene su propia regla, CA-697: su parte derecha
// termina en la celda W-2.)
func TestCA577_ElBordeNoSeCorre(t *testing.T) {
	titulos := []string{"Despegue 🚀 del 漢字", "漢字漢字漢字漢字", "🚀🚀🚀", "Ｆｕｌｌ ｗｉｄｔｈ", "Cafe\u0301 y man\u0303ana", "mezcla 漢 a 🚀 b é\u0301"}
	for _, utf := range []bool{true, false} {
		for _, ti := range titulos {
			for _, tam := range [][2]int{{80, 24}, {120, 32}, {70, 24}, {100, 30}} {
				m := ttModelo(ttConfig(tam[0], tam[1], utf), ttTablero(ttCardAncha("cohete", ti, "lanzar el "+ti)))
				ttForma(t, "CA-577", m, tam[0], tam[1], false)
				d := tv3bDetalle(t, "CA-577", m)
				if d.Col+d.Ancho != tam[0] {
					t.Fatalf("CA-577: con %q (utf8=%v, %dx%d) el detalle termina en la ultima celda: empieza en %d con %d celdas:\n%s",
						ti, utf, tam[0], tam[1], d.Col, d.Ancho, ttPantalla(m))
				}
				for i, l := range ttLimpia(m) {
					if i == 0 {
						continue
					}
					if i >= tam[1]-2 {
						break
					}
					if n := ttCeldas(l); n != tam[0] {
						t.Fatalf("CA-577: con %q (utf8=%v, %dx%d) la linea %d mide %d celdas, no %d: %q",
							ti, utf, tam[0], tam[1], i+1, n, tam[0], l)
					}
				}
			}
		}
	}
}

// CA-577: un caracter ancho que quedaria a caballo del limite de un corte se
// va entero, y la celda que deja es un espacio antes del borde (doc.go, "The
// screen"). El titulo del detalle (la v2: el de [0]) cuando no entra con su
// slug se corta a (w-2) - Celdas(slug) - 2 celdas (doc.go, "The detail"): a
// 80 columnas el detalle tiene 80 celdas y con el slug "cohete" el titulo se
// corta a 70: treinta y cuatro 漢 (68) mas "…" son 69, y la celda 70 es un
// espacio; a 120 columnas, 72 celdas y un corte a 62: treinta 漢, "…" y el
// espacio.
func TestCA577_UnAnchoAlBordeDelCorte(t *testing.T) {
	tarjeta := ttTablero(ttCardAncha("cohete", strings.Repeat("漢", 50), "p"))
	casos := []struct {
		ancho, alto, han int
	}{
		{80, 24, 34},
		{120, 32, 30},
	}
	for _, c := range casos {
		m := ttModelo(ttConfig(c.ancho, c.alto, true), tarjeta)
		quiere := "┌" + strings.Repeat("漢", c.han) + "… ─cohete─┐"
		if d := tv3bDetalle(t, "CA-577", m); d.Arriba != quiere {
			t.Fatalf("CA-577: a %dx%d el titulo cortado deja un espacio donde no entra el ancho:\n got %q\nwant %q", c.ancho, c.alto, d.Arriba, quiere)
		}
		// en ASCII el corte es ~
		a := ttModelo(ttConfig(c.ancho, c.alto, false), tarjeta)
		if d := tv3bDetalle(t, "CA-577", a); d.Arriba != "+"+strings.Repeat("漢", c.han)+"~ -cohete-+" {
			t.Fatalf("CA-577: a %dx%d en ASCII el corte es ~: %q", c.ancho, c.alto, d.Arriba)
		}
	}
}

// CA-577 (propiedad): con titulos y plains al azar de runas anchas,
// combinantes, controles y ASCII, toda linea de Pantalla mide a lo sumo
// Ancho celdas (con la tabla de los tests, que no es la del paquete) y las
// que terminan en una esquina miden exactamente Ancho. Las teclas son las de
// la v2 traducidas: j, H (la v2: 4), j en la vista Hallazgos (la v2: 3, que
// eran otras filas), tab (la v2: enter, el foco a [0]) y v.
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
		tams := [][2]int{{60, 15}, {79, 24}, {80, 24}, {80, 23}, {100, 30}, {120, 32}, {61, 16}, {99, 20}}
		tam := tams[r.Intn(len(tams))]
		utf := r.Intn(2) == 0
		cards := []boardcmd.Card{ttCardAncha("c-"+fmt.Sprint(semilla%97), txt(), txt()), ttInfraAPI(true), ttNucTenancy()}
		cards[1].Item.Titulo = txt()
		m := ttModelo(ttConfig(tam[0], tam[1], utf), ttTablero(cards...))
		h := ttHallazgo("h", finding.StatusOpen, "high", txt()+".go:3", txt())
		m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h}})
		for _, k := range []tablero.Tecla{"", "j", "H", "j", tablero.TeclaTab, "H", "v"} {
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
