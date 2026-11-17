// El mouse (spec tablero-con-paneles CA-575; en la v3, spec
// tablero-tu-siguiente-paso, CA-719 re-expresa donde hace efecto un clic):
// DecodificarEntradas y el formato SGR, lo que hace cada evento en el modelo
// (el foco de la lista o del detalle, la seleccion, la caja TU SIGUIENTE
// PASO, la rueda, el modal) y que ninguno lance ni abra un modal
// (propiedad), y Run: las secuencias que piden y devuelven el mouse,
// HOOM_MOUSE=0, y un clic que llega por la terminal falsa.
package tablero_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/tablero"
)

func ttClic(x, y int) tablero.Raton { return tablero.Raton{Accion: tablero.RatonClic, X: x, Y: y} }
func ttRuedaAbajo(x, y int) tablero.Raton {
	return tablero.Raton{Accion: tablero.RatonRuedaAbajo, X: x, Y: y}
}
func ttRuedaArriba(x, y int) tablero.Raton {
	return tablero.Raton{Accion: tablero.RatonRuedaArriba, X: x, Y: y}
}

// ttRaton manda r y exige que no de ninguna orden.
func ttRaton(t *testing.T, m *tablero.Modelo, r tablero.Raton) {
	t.Helper()
	ttSinOrdenes(t, "CA-575", fmt.Sprintf("el evento %+v", r), m.Raton(r, ttAhora))
}

// ---------------------------------------------------------------- DecodificarEntradas

// CA-575: DecodificarEntradas lee el formato SGR: ESC [ < b ; x ; y M
// (apretado), con X = x-1 e Y = y-1; b 0 es el clic izquierdo, 64 y 65 la
// rueda; los bits 4, 8 y 16 (shift, meta, control) no cambian lo que es;
// una liberacion (m), el boton del medio o el derecho, un movimiento, las
// ruedas 6 y 7, numeros que no se leen y el formato X10 se descartan
// enteros, sin teclas; las teclas siguen como Decodificar.
func TestCA575_DecodificarEntradas(t *testing.T) {
	clic := func(x, y int) tablero.Entrada { r := ttClic(x, y); return tablero.Entrada{Raton: &r} }
	arriba := func(x, y int) tablero.Entrada { r := ttRuedaArriba(x, y); return tablero.Entrada{Raton: &r} }
	abajo := func(x, y int) tablero.Entrada { r := ttRuedaAbajo(x, y); return tablero.Entrada{Raton: &r} }
	tecla := func(k tablero.Tecla) tablero.Entrada { return tablero.Entrada{Tecla: k} }
	casos := []struct {
		nombre string
		in     string
		quiere []tablero.Entrada
	}{
		{"clic", "\x1b[<0;10;5M", []tablero.Entrada{clic(9, 4)}},
		{"clic en la primera celda", "\x1b[<0;1;1M", []tablero.Entrada{clic(0, 0)}},
		{"rueda arriba", "\x1b[<64;3;4M", []tablero.Entrada{arriba(2, 3)}},
		{"rueda abajo", "\x1b[<65;3;4M", []tablero.Entrada{abajo(2, 3)}},
		{"clic con shift", "\x1b[<4;2;2M", []tablero.Entrada{clic(1, 1)}},
		{"clic con meta", "\x1b[<8;2;2M", []tablero.Entrada{clic(1, 1)}},
		{"clic con control", "\x1b[<16;2;2M", []tablero.Entrada{clic(1, 1)}},
		{"rueda con control", "\x1b[<81;5;6M", []tablero.Entrada{abajo(4, 5)}},
		{"coordenadas grandes", "\x1b[<0;300;200M", []tablero.Entrada{clic(299, 199)}},
		{"liberacion", "\x1b[<0;10;5m", nil},
		{"boton del medio", "\x1b[<1;10;5M", nil},
		{"boton derecho", "\x1b[<2;10;5M", nil},
		{"movimiento", "\x1b[<32;10;5M", nil},
		{"arrastre", "\x1b[<35;10;5M", nil},
		{"rueda 6", "\x1b[<66;10;5M", nil},
		{"rueda 7", "\x1b[<67;10;5M", nil},
		{"numeros que no se leen", "\x1b[<a;10;5M", nil},
		{"un numero de menos", "\x1b[<0;10M", nil},
		{"X10", "\x1b[M !!", nil},
		{"mezcla", "j\x1b[<0;2;3Mk\x1b[<65;1;1M\r", []tablero.Entrada{tecla("j"), clic(1, 2), tecla("k"), abajo(0, 0), tecla(tablero.TeclaEnter)}},
		{"una liberacion entre teclas", "a\x1b[<0;2;3mb", []tablero.Entrada{tecla("a"), tecla("b")}},
	}
	for _, c := range casos {
		got := tablero.DecodificarEntradas([]byte(c.in))
		if len(got) != len(c.quiere) {
			t.Fatalf("CA-575: DecodificarEntradas(%q) (%s) da %d entradas, se esperaban %d: %#v", c.in, c.nombre, len(got), len(c.quiere), got)
		}
		for i := range got {
			g, q := got[i], c.quiere[i]
			if g.Tecla != q.Tecla || (g.Raton == nil) != (q.Raton == nil) || (g.Raton != nil && *g.Raton != *q.Raton) {
				t.Fatalf("CA-575: DecodificarEntradas(%q) (%s) [%d] = %+v (raton %+v), se esperaba %+v (raton %+v)", c.in, c.nombre, i, g, g.Raton, q, q.Raton)
			}
			if (g.Tecla == "") == (g.Raton == nil) {
				t.Fatalf("CA-575: cada Entrada es una tecla o un evento, exactamente uno: %+v", g)
			}
		}
		// Decodificar da las mismas teclas, sin los eventos
		var teclas []tablero.Tecla
		for _, e := range c.quiere {
			if e.Raton == nil {
				teclas = append(teclas, e.Tecla)
			}
		}
		if got := tablero.Decodificar([]byte(c.in)); fmt.Sprint(got) != fmt.Sprint(teclas) {
			t.Fatalf("CA-575: Decodificar(%q) son las teclas sin los eventos: %q, se esperaba %q", c.in, got, teclas)
		}
	}
}

// CA-575 (propiedad): un texto imprimible con eventos SGR intercalados se
// decodifica en sus teclas y sus eventos, en orden; ningun byte de un evento
// sale como tecla.
func TestCA575_DecodificarEntradasPropiedad(t *testing.T) {
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		var b strings.Builder
		var quiere []string
		for i := 0; i < 1+r.Intn(20); i++ {
			if r.Intn(2) == 0 {
				x, y := 1+r.Intn(250), 1+r.Intn(80)
				cod := []int{0, 64, 65, 4, 69}[r.Intn(5)]
				fmt.Fprintf(&b, "\x1b[<%d;%d;%dM", cod, x, y)
				quiere = append(quiere, fmt.Sprintf("r%d:%d,%d", map[int]int{0: 1, 4: 1, 64: 2, 65: 3, 69: 3}[cod], x-1, y-1))
			} else {
				c := string(rune('a' + r.Intn(26)))
				b.WriteString(c)
				quiere = append(quiere, "t"+c)
			}
		}
		var got []string
		for _, e := range tablero.DecodificarEntradas([]byte(b.String())) {
			if e.Raton != nil {
				got = append(got, fmt.Sprintf("r%d:%d,%d", int(e.Raton.Accion), e.Raton.X, e.Raton.Y))
			} else {
				got = append(got, "t"+string(e.Tecla))
			}
		}
		if strings.Join(got, " ") != strings.Join(quiere, " ") {
			t.Logf("semilla %d: %q\n got %v\nwant %v", semilla, b.String(), got, quiere)
			return false
		}
		return true
	}
	if tablero.RatonClic != 1 || tablero.RatonRuedaArriba != 2 || tablero.RatonRuedaAbajo != 3 {
		t.Fatalf("CA-575 (guarda): RatonClic, RatonRuedaArriba y RatonRuedaAbajo son 1, 2 y 3 (iota + 1)")
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("CA-575: %v", err)
	}
}

// ---------------------------------------------------------------- el modelo

// ttFilaDe es la linea (desde 0) de la pantalla donde esta s, en las
// primeras 40 celdas (en la v3, la lista con 100 columnas o mas).
func ttFilaDe(t *testing.T, m *tablero.Modelo, s string) int {
	t.Helper()
	for i, l := range ttLimpia(m) {
		if strings.Contains(ttTramo(l, 0, 40), s) {
			return i
		}
	}
	t.Fatalf("CA-575: la columna de la izquierda tiene %q:\n%s", s, ttPantalla(m))
	return -1
}

// tv3bRatFila es la linea de la pantalla (desde 0) de la linea k (0 la
// primera, 1 la segunda) de la fila de la lista cuyo titulo es titulo.
func tv3bRatFila(t *testing.T, m *tablero.Modelo, titulo string, k int) int {
	t.Helper()
	c := tv3bLista(t, "CA-719", m)
	i := tv3bFilaDe(c, titulo)
	if i < 0 || i+k >= len(c.Dentro) {
		t.Fatalf("CA-719: la lista tiene la fila %q con su linea %d:\n%s", titulo, k+1, ttPantalla(m))
	}
	return c.Fila + 1 + i + k
}

// tv3bRatLinea es la linea de la pantalla (desde 0) de la linea de la lista
// que, sin el relleno, es s (la de un grupo, la de HECHO).
func tv3bRatLinea(t *testing.T, m *tablero.Modelo, s string) int {
	t.Helper()
	c := tv3bLista(t, "CA-719", m)
	for i, l := range c.Dentro {
		if strings.TrimRight(l, " ") == s {
			return c.Fila + 1 + i
		}
	}
	t.Fatalf("CA-719: la lista tiene la linea %q:\n%s", s, ttPantalla(m))
	return -1
}

// tv3bRatCon es la linea de la pantalla (desde 0) de la primera linea de la
// lista que tiene s.
func tv3bRatCon(t *testing.T, m *tablero.Modelo, s string) int {
	t.Helper()
	c := tv3bLista(t, "CA-719", m)
	for i, l := range c.Dentro {
		if strings.Contains(l, s) {
			return c.Fila + 1 + i
		}
	}
	t.Fatalf("CA-719: la lista tiene una linea con %q:\n%s", s, ttPantalla(m))
	return -1
}

// tv3bRatEnDetalle es una celda dentro del detalle.
func tv3bRatEnDetalle(t *testing.T, m *tablero.Modelo) (int, int) {
	t.Helper()
	d := tv3bDetalle(t, "CA-719", m)
	return d.Col + 10, d.Fila + 3
}

// CA-719, CA-575: un clic en la lista le da el foco (sus bordes incluidos);
// en cualquiera de las dos lineas de una tarjeta ademas la elige (el detalle
// la sigue); en la linea de un grupo solo da el foco; en la linea de HECHO la
// elige sin abrirla. Un clic en el detalle le da el foco. Un clic en la caja
// TU SIGUIENTE PASO elige en la lista la tarjeta de la caja y le da el foco a
// la lista (desde la vista Hallazgos y desde el detalle del proyecto vuelve a
// las tarjetas); sin tarjeta en la caja no hace nada. En la cabecera, en la
// linea del Registro y en las dos ultimas no hace nada. Nunca una orden. (La
// v2 empezaba con un clic en [1], que mostraba el proyecto en [0]: en la v3
// la cabecera no hace nada con un clic y el proyecto se ve con v.)
func TestCA719_CA575_ClicDaElFocoYElige(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	z := tv3bLeer(t, "CA-719", m)
	ttRaton(t, m, ttClic(tv3bRatEnDetalle(t, m)))
	if m.Foco() != tablero.ZonaDetalle || m.Lugar() != tablero.LugarVerTarjeta || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("CA-719: un clic en el detalle le da el foco, con la tarjeta elegida: %d %q %q", int(m.Foco()), m.Lugar(), m.Elegida())
	}
	ttRaton(t, m, ttClic(3, tv3bRatFila(t, m, "CI y CD de la infra", 0)))
	if m.Foco() != tablero.ZonaLista || m.Elegida() != "infra-ci-cd" || tv3bTituloElegido(t, "CA-719", m) != "CI y CD de la infra" ||
		tv3bDetalle(t, "CA-719", m).Titulo != "CI y CD de la infra" {
		t.Fatalf("CA-719: un clic en la primera linea de una tarjeta le da el foco a la lista y la elige (el detalle la sigue): %d %q\n%s", int(m.Foco()), m.Elegida(), ttPantalla(m))
	}
	tv3bAlDetalle(t, m)
	ttRaton(t, m, ttClic(3, tv3bRatFila(t, m, "Guia de la documentacion", 1)))
	if m.Foco() != tablero.ZonaLista || m.Elegida() != "docs-guia" {
		t.Fatalf("CA-719: un clic en la segunda linea de una tarjeta tambien la elige y le da el foco a la lista: %d %q", int(m.Foco()), m.Elegida())
	}
	tv3bAlDetalle(t, m)
	ttRaton(t, m, ttClic(3, tv3bRatLinea(t, m, " EN COLA 2")))
	if m.Foco() != tablero.ZonaLista || m.Elegida() != "docs-guia" {
		t.Fatalf("CA-719: un clic en la linea de un grupo solo da el foco: %d %q", int(m.Foco()), m.Elegida())
	}
	ttRaton(t, m, ttClic(3, tv3bRatLinea(t, m, " HECHO 3 >")))
	if got := tv3bTituloElegido(t, "CA-719", m); got != "HECHO 3 >" || m.Elegida() != "" || ttTiene(m, "Viejo 1") {
		t.Fatalf("CA-719: un clic en la linea de HECHO la elige sin abrirla (%q):\n%s", got, ttPantalla(m))
	}
	// una fila del grupo CORRIENDO (la v2: [3]) y una de DETENIDA
	if g := m.Grupo("infra-api"); g != tablero.GrupoCorriendo {
		t.Fatalf("CA-700 (guarda): infra-api va en CORRIENDO: %q", g)
	}
	ttRaton(t, m, ttClic(10, tv3bRatFila(t, m, "Convenciones de la API", 0)))
	if m.Foco() != tablero.ZonaLista || m.Elegida() != "infra-api" {
		t.Fatalf("CA-719: un clic en una fila del grupo CORRIENDO la elige: %d %q", int(m.Foco()), m.Elegida())
	}
	ttRaton(t, m, ttClic(10, tv3bRatFila(t, m, "Pagos caidos", 1)))
	if m.Foco() != tablero.ZonaLista || m.Elegida() != "pagos-caidos" {
		t.Fatalf("CA-719: un clic en la segunda linea de una fila de DETENIDA la elige: %d %q", int(m.Foco()), m.Elegida())
	}
	// la vista Hallazgos (la v2: [4]): desde su detalle, un clic en una fila
	// de su lista le da el foco y la elige
	m.Tecla("H", ttAhora)
	tv3bAlDetalle(t, m)
	if m.Lugar() != tablero.LugarVerHallazgo {
		t.Fatalf("CA-712 (guarda): H y tab: el detalle de un hallazgo: %q", m.Lugar())
	}
	ttRaton(t, m, ttClic(10, tv3bRatCon(t, m, "low    nuc-tenancy")))
	if m.Lugar() != tablero.LugarHallazgos || tv3bDetalle(t, "CA-719", m).Titulo != "hallazgo h-low" {
		t.Fatalf("CA-719: un clic en una fila de la lista de la vista Hallazgos le da el foco y la elige: %q %q", m.Lugar(), tv3bDetalle(t, "CA-719", m).Titulo)
	}
	tv3bAlDetalle(t, m)
	ttRaton(t, m, ttClic(z.Lista.Col, z.Lista.Fila)) // la esquina de la lista
	if m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarHallazgos {
		t.Fatalf("CA-719: un clic en el borde de la lista le da el foco: %d %q", int(m.Foco()), m.Lugar())
	}
	// la caja TU SIGUIENTE PASO
	if m.Caja() != "nuc-tenancy" {
		t.Fatalf("CA-698 (guarda): la tarjeta de la caja es nuc-tenancy: %q", m.Caja())
	}
	tv3bAlDetalle(t, m)
	ttRaton(t, m, ttClic(5, z.Caja.Fila+1))
	if m.Vista() != tablero.VistaTarjetas || m.Lugar() != tablero.LugarTarjetas || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("CA-719: un clic en la caja desde la vista Hallazgos vuelve a las tarjetas, elige la de la caja y le da el foco a la lista: %q %q %q", m.Vista(), m.Lugar(), m.Elegida())
	}
	tv3bIrAHecho(t, m)
	ttRaton(t, m, ttClic(tv3bRatEnDetalle(t, m)))
	if m.Foco() != tablero.ZonaDetalle || m.Lugar() != tablero.LugarVerNada {
		t.Fatalf("CA-719: un clic en el detalle le da el foco (la lista estaba en HECHO: nada): %d %q", int(m.Foco()), m.Lugar())
	}
	ttRaton(t, m, ttClic(60, z.Caja.Fila+2))
	if m.Lugar() != tablero.LugarTarjetas || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("CA-719: un clic en la caja desde el detalle elige la tarjeta de la caja, con el foco en la lista: %q %q", m.Lugar(), m.Elegida())
	}
	m.Tecla("v", ttAhora)
	if m.Lugar() != tablero.LugarVerProyecto {
		t.Fatalf("CA-713 (guarda): v muestra el proyecto en el detalle: %q", m.Lugar())
	}
	ttRaton(t, m, ttClic(5, z.Caja.Fila+1))
	if m.Lugar() != tablero.LugarTarjetas || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("CA-719: un clic en la caja desde el detalle del proyecto vuelve a las tarjetas: %q %q", m.Lugar(), m.Elegida())
	}
	// la cabecera, la linea del Registro (la v2: el panel del Registro) y
	// las dos ultimas no hacen nada
	tv3bAlDetalle(t, m)
	antes := ttFoto(m)
	for _, y := range []int{0, z.Alto - 3, z.Alto - 2, z.Alto - 1} {
		ttRaton(t, m, ttClic(5, y))
		if ttFoto(m) != antes {
			t.Fatalf("CA-719: un clic en la linea %d (la cabecera, el Registro o las dos ultimas) no hace nada:\n%s", y+1, ttPantalla(m))
		}
	}
	// sin tarjeta en la caja (solo EN COLA), un clic en la caja no hace nada
	n := ttModelo(ttConfig(120, 32, false), ttTablero(ttBacklog("cola", "En la cola")))
	if n.Caja() != "" {
		t.Fatalf("CA-698 (guarda): con solo EN COLA la caja no tiene tarjeta: %q", n.Caja())
	}
	ttRaton(t, n, ttClic(tv3bRatEnDetalle(t, n)))
	if n.Foco() != tablero.ZonaDetalle {
		t.Fatalf("CA-719: un clic en el detalle le da el foco: %d", int(n.Foco()))
	}
	antes = ttFoto(n)
	ttRaton(t, n, ttClic(5, z.Caja.Fila+1))
	if ttFoto(n) != antes {
		t.Fatalf("CA-719: sin tarjeta en la caja, un clic en la caja no hace nada:\n%s", ttPantalla(n))
	}
}

// CA-719, CA-575: un clic en la lista plegada (una columna, con el foco en el
// detalle) le da el foco a la lista, que se despliega, sin cambiar la
// elegida; la rueda sobre la lista plegada no hace nada. (La v2 plegaba [4]
// y [1] a 60x15.)
func TestCA719_CA575_ClicEnUnPlegado(t *testing.T) {
	m := ttModeloDorado(ttConfig(60, 15, false))
	tv3bAlDetalle(t, m)
	l := tv3bLista(t, "CA-719", m)
	if !l.Plegado {
		t.Fatalf("CA-696 (guarda): a 60x15 con el foco en el detalle la lista va plegada: %+v", l)
	}
	elegida := m.Elegida()
	ttRaton(t, m, ttClic(30, l.Fila))
	if m.Foco() != tablero.ZonaLista || tv3bLista(t, "CA-719", m).Plegado || m.Elegida() != elegida {
		t.Fatalf("CA-719: un clic en la lista plegada le da el foco y la despliega (%q, era %q):\n%s", m.Elegida(), elegida, ttPantalla(m))
	}
	// la rueda sobre la lista plegada no hace nada
	tv3bAlDetalle(t, m)
	l = tv3bLista(t, "CA-719", m)
	antes := ttFoto(m)
	ttRaton(t, m, ttRuedaAbajo(5, l.Fila))
	ttRaton(t, m, ttRuedaArriba(5, l.Fila))
	if ttFoto(m) != antes {
		t.Fatalf("CA-719: la rueda sobre la lista plegada no hace nada")
	}
}

// CA-719, CA-575: la rueda sobre la lista mueve su seleccion una fila por
// muesca sin mover el foco; sobre el detalle lo desplaza 3 lineas; sobre lo
// demas (la cabecera, la caja, la linea del Registro) no hace nada.
func TestCA719_CA575_LaRueda(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	tv3bAlDetalle(t, m)
	fila := tv3bRatFila(t, m, "Tenancy del nucleo", 0)
	ttRaton(t, m, ttRuedaAbajo(5, fila))
	if m.Foco() != tablero.ZonaDetalle || m.Elegida() != "precios" || tv3bTituloElegido(t, "CA-719", m) != "Precios por region" {
		t.Fatalf("CA-719: la rueda sobre la lista mueve su seleccion sin mover el foco: %d %q", int(m.Foco()), m.Elegida())
	}
	ttRaton(t, m, ttRuedaArriba(5, fila))
	if m.Foco() != tablero.ZonaDetalle || m.Elegida() != "nuc-tenancy" {
		t.Fatalf("CA-719: la rueda arriba la sube: %d %q", int(m.Foco()), m.Elegida())
	}
	// el detalle (la v2: [0]): tres lineas por muesca
	c, d := ttTarjetaLarga()
	n := ttModelo(ttConfig(80, 24, false), ttTablero(c))
	n.LeerDetalle(d)
	det := tv3bDetalle(t, "CA-719", n)
	todo := det.ttTexto()
	ttRaton(t, n, ttRuedaAbajo(50, det.Fila+2))
	if v := tv3bDetalle(t, "CA-719", n).ttTexto(); v[0] != todo[3] || n.Foco() != tablero.ZonaLista {
		t.Fatalf("CA-719: la rueda sobre el detalle lo desplaza 3 lineas (sin mover el foco): %q", v[0])
	}
	ttRaton(t, n, ttRuedaArriba(50, det.Fila+2))
	if v := tv3bDetalle(t, "CA-719", n).ttTexto(); v[0] != todo[0] {
		t.Fatalf("CA-719: la rueda arriba vuelve: %q", v[0])
	}
	antes := ttFoto(n)
	for _, y := range []int{0, 2, 21} { // la cabecera, la caja, la linea del Registro (A-2)
		ttRaton(t, n, ttRuedaAbajo(5, y))
		if ttFoto(n) != antes {
			t.Fatalf("CA-719: la rueda sobre la linea %d (fuera de la lista y del detalle) no hace nada", y+1)
		}
	}
}

// CA-719, CA-575: un clic que saca el foco de la lista cierra la busqueda
// (como esc) y suelta el filtro; uno en una fila de la lista la deja
// abierta; uno en la caja tambien la cierra y elige la tarjeta de la caja.
func TestCA719_CA575_ClicYLaBusqueda(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	m.Tecla("/", ttAhora)
	ttEscribir(m, "inf")
	ttRaton(t, m, ttClic(3, tv3bRatFila(t, m, "CI y CD de la infra", 0)))
	if m.Lugar() != tablero.LugarBusqueda || m.Elegida() != "infra-ci-cd" {
		t.Fatalf("CA-719: un clic en una fila de la lista deja la busqueda abierta y elige: %q %q", m.Lugar(), m.Elegida())
	}
	if tv3bFilaDe(tv3bLista(t, "CA-719", m), "Tenancy del nucleo") >= 0 {
		t.Fatalf("CA-554 (guarda): con la busqueda \"inf\" nuc-tenancy no esta en la lista:\n%s", ttPantalla(m))
	}
	ttRaton(t, m, ttClic(tv3bRatEnDetalle(t, m)))
	if m.Lugar() != tablero.LugarVerTarjeta || m.Elegida() != "infra-ci-cd" || ttMensaje(m) != "" ||
		tv3bFilaDe(tv3bLista(t, "CA-719", m), "Tenancy del nucleo") < 0 {
		t.Fatalf("CA-719: un clic en el detalle cierra la busqueda y suelta el filtro: %q %q %q", m.Lugar(), m.Elegida(), ttMensaje(m))
	}
	m.Tecla("/", ttAhora)
	ttEscribir(m, "inf")
	ttRaton(t, m, ttClic(5, 2)) // la caja TU SIGUIENTE PASO
	if m.Lugar() != tablero.LugarTarjetas || m.Elegida() != "nuc-tenancy" || ttMensaje(m) != "" ||
		tv3bFilaDe(tv3bLista(t, "CA-719", m), "Precios por region") < 0 {
		t.Fatalf("CA-719: un clic en la caja cierra la busqueda, suelta el filtro y elige la tarjeta de la caja: %q %q %q", m.Lugar(), m.Elegida(), ttMensaje(m))
	}
}

// CA-719, CA-575: los eventos fuera de la pantalla (mas alla del ancho o del
// alto, en la linea de mensajes o en la de teclas) no hacen nada.
func TestCA719_CA575_FueraDeLaPantalla(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	antes := ttFoto(m)
	for _, r := range []tablero.Raton{ttClic(120, 5), ttClic(500, 500), ttClic(5, 32), ttClic(-1, 3), ttClic(3, -1),
		ttClic(5, 30), ttClic(5, 31), ttRuedaAbajo(119, 40), ttRuedaArriba(-5, -5), {Accion: 0, X: 5, Y: 5}, {Accion: 99, X: 5, Y: 8}} {
		ttRaton(t, m, r)
		if ttFoto(m) != antes {
			t.Fatalf("CA-719: el evento %+v fuera de la lista, del detalle y de la caja no hace nada:\n%s", r, ttPantalla(m))
		}
	}
}

// CA-719, CA-575: con un modal abierto, un clic fuera no hace nada; uno en la
// linea de un selector (que no es gris) le da el foco; uno en el Pedido, al
// Pedido; uno en un boton lo elige sin ejecutarlo y la linea de mensajes dice
// "enter para <accion>"; con una lista abierta, un clic en una opcion la
// elige y cierra la lista; en Acciones marca la accion; en la ayuda y en el
// Registro (L, nuevo en la v3) la rueda desplaza 3 lineas.
func TestCA719_CA575_ElMouseEnUnModal(t *testing.T) {
	m := ttAbrirModal(t, ttConfig(120, 32, false), "w", tablero.ModalPedir, ttAccionWriter())
	mc := ttModal(t, "CA-575", m, 120)
	x := mc.Col + 6 // dentro del texto (que empieza en Col + 4)
	fila := func(etiqueta string) int {
		_, i, _ := mc.ttLineaQueEmpieza(etiqueta)
		if i < 0 {
			t.Fatalf("CA-575: el modal tiene %q", etiqueta)
		}
		return mc.Fila + 1 + i
	}
	antes := ttFoto(m)
	ttRaton(t, m, ttClic(2, 2)) // fuera del modal
	ttRaton(t, m, ttClic(mc.Col+mc.Ancho+3, mc.Fila+3))
	if ttFoto(m) != antes {
		t.Fatalf("CA-575: un clic fuera del modal no hace nada")
	}
	ttRaton(t, m, ttClic(x, fila("Tope")))
	if f := ttFoco(ttModal(t, "CA-575", m, 120)); f != "Tope" || m.Lugar() != tablero.LugarPedirSelector {
		t.Fatalf("CA-575: un clic en un selector le da el foco: %q", f)
	}
	ttRaton(t, m, ttClic(x+3, fila("Pedido")+2)) // dentro del cuadro del Pedido
	if m.Lugar() != tablero.LugarPedirPedido {
		t.Fatalf("CA-575: un clic en el cuadro del Pedido le da el foco: %q", m.Lugar())
	}
	// los botones
	_, ib, _ := mc.ttLineaQueEmpieza("")
	for i, l := range mc.Textos {
		if strings.Contains(l, "Cancelar") {
			ib = i
		}
	}
	lb := mc.Textos[ib]
	cancelar := mc.Col + 4 + ttCeldas(lb[:strings.Index(lb, "Cancelar")]) + 2
	ttRaton(t, m, ttClic(cancelar, mc.Fila+1+ib))
	if m.Modal() != tablero.ModalPedir || m.Corriendo() != 0 || !strings.Contains(strings.Join(ttModal(t, "CA-575", m, 120).Textos, "\n"), "[ Cancelar ]") {
		t.Fatalf("CA-575: un clic en Cancelar lo elige sin ejecutarlo:\n%s", ttPantalla(m))
	}
	ttDiceMensaje(t, "CA-575", "un clic en un boton", m, false, "enter para cancelar")
	lb = ttModal(t, "CA-575", m, 120).Textos[ib]
	accion := mc.Col + 4 + ttCeldas(lb[:strings.Index(lb, "Pedir al writer")]) + 2
	ttRaton(t, m, ttClic(accion, mc.Fila+1+ib))
	ttDiceMensaje(t, "CA-575", "un clic en el boton de la accion", m, false, "enter para pedir al writer")
	if m.Lugar() != tablero.LugarPedirBotones || m.Corriendo() != 0 {
		t.Fatalf("CA-575: un clic en el boton no lanza: %q", m.Lugar())
	}
	// una lista abierta
	ttRaton(t, m, ttClic(x, fila("Esfuerzo")))
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if m.Lugar() != tablero.LugarPedirLista {
		t.Fatalf("CA-568: enter abre la lista: %q", m.Lugar())
	}
	lista := ttListaDe(t, m)
	low := -1
	for i, l := range lista.Ops {
		if strings.HasPrefix(l, "   low ") {
			low = i
		}
	}
	if low < 0 {
		t.Fatalf("CA-568: la lista de Esfuerzo tiene low: %q", lista.Ops)
	}
	antes = ttFoto(m)
	ttRaton(t, m, ttClic(2, 2))
	if ttFoto(m) != antes {
		t.Fatalf("CA-575: con una lista abierta, un clic que no es en una opcion no hace nada")
	}
	ttRaton(t, m, ttClic(lista.Col+4, lista.Fila+1+low))
	if m.Lugar() != tablero.LugarPedirSelector || ttModal(t, "CA-575", m, 120).ttValor("Esfuerzo") != "< low >" {
		t.Fatalf("CA-575: un clic en una opcion la elige y cierra la lista: %q %q", m.Lugar(), ttModal(t, "CA-575", m, 120).ttValor("Esfuerzo"))
	}
	// Acciones
	a := ttModelo(ttConfig(120, 32, false), ttTablero(ttInfraAPI(false)))
	a.Tecla(tablero.TeclaEspacio, ttAhora)
	ma := ttModal(t, "CA-575", a, 120)
	_, ia, _ := ma.ttLineaQueEmpieza("A Aprobar spec")
	ttRaton(t, a, ttClic(ma.Col+6, ma.Fila+1+ia))
	if got := ttModal(t, "CA-575", a, 120).ttMarcada(); !strings.HasPrefix(got, "A Aprobar spec") || a.Modal() != tablero.ModalAcciones {
		t.Fatalf("CA-575: en Acciones un clic marca la accion, sin abrirla: %q %q", got, a.Modal())
	}
	// la ayuda: la rueda, 3 lineas
	h := ttModeloDorado(ttConfig(80, 24, false))
	h.Tecla("?", ttAhora)
	primero := ttModal(t, "CA-575", h, 80).Textos
	ttRaton(t, h, ttRuedaAbajo(40, 10))
	if got := ttModal(t, "CA-575", h, 80).Textos; got[0] != primero[3] {
		t.Fatalf("CA-575: en la ayuda la rueda desplaza 3 lineas: %q", got[0])
	}
	// el Registro (L): abre en su final y la rueda lo desplaza 3 lineas; 15
	// entradas de horas distintas son 30 lineas, mas que las 20 interiores
	// de un modal de B = 22 lineas a 80x24
	g := ttModelo(ttConfig(80, 24, false), ttTablero(ttTarjetaCon(ttAccionWriter())))
	for i := 0; i < 15; i++ {
		g.Tecla("u", ttAhora.Add(time.Duration(i)*time.Minute))
	}
	g.Tecla("L", ttAhora.Add(time.Hour))
	if g.Modal() != tablero.ModalRegistro {
		t.Fatalf("CA-714 (guarda): L abre el modal Registro: %q", g.Modal())
	}
	fin := ttModal(t, "CA-719", g, 80).Textos
	ttRaton(t, g, ttRuedaArriba(40, 10))
	arriba := ttModal(t, "CA-719", g, 80).Textos
	if len(arriba) != len(fin) || len(fin) < 4 || strings.Join(arriba[3:], "\n") != strings.Join(fin[:len(fin)-3], "\n") {
		t.Fatalf("CA-719: en el Registro la rueda arriba desplaza 3 lineas:\n antes %q\ndespues %q", fin, arriba)
	}
	ttRaton(t, g, ttRuedaAbajo(40, 10))
	if got := ttModal(t, "CA-719", g, 80).Textos; strings.Join(got, "\n") != strings.Join(fin, "\n") || g.Modal() != tablero.ModalRegistro {
		t.Fatalf("CA-719: en el Registro la rueda abajo vuelve al final: %q", got)
	}
}

// CA-575 (propiedad): ninguna secuencia de eventos del mouse produce una
// orden (ni Lanzar) ni abre un modal; con un modal abierto (tambien el
// Registro de la v3, L), ninguno lo cambia por otro ni lo cierra.
func TestCA575_PropiedadElMouseNuncaEjecuta(t *testing.T) {
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		tams := [][2]int{{120, 32}, {80, 24}, {70, 20}, {60, 15}}
		tam := tams[r.Intn(len(tams))]
		m := ttModeloDorado(ttConfig(tam[0], tam[1], r.Intn(2) == 0))
		abierto := tablero.SinModal
		switch r.Intn(5) {
		case 1:
			m.Tecla("A", ttAhora)
		case 2:
			m.Tecla(tablero.TeclaEspacio, ttAhora)
		case 3:
			m.Tecla("?", ttAhora)
		case 4:
			m.Tecla("L", ttAhora)
		}
		abierto = m.Modal()
		for i := 0; i < 60; i++ {
			ev := tablero.Raton{Accion: tablero.AccionRaton(1 + r.Intn(3)), X: r.Intn(tam[0]+4) - 2, Y: r.Intn(tam[1]+4) - 2}
			if os := m.Raton(ev, ttAhora); len(os) != 0 {
				t.Logf("semilla %d: %+v dio las ordenes %#v", semilla, ev, os)
				return false
			}
			if m.Modal() != abierto {
				t.Logf("semilla %d: %+v cambio el modal de %q a %q", semilla, ev, abierto, m.Modal())
				return false
			}
			if m.Corriendo() != 0 || len(m.Registro()) != 0 {
				t.Logf("semilla %d: %+v ejecuto algo", semilla, ev)
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 150}); err != nil {
		t.Fatalf("CA-575: %v", err)
	}
}

// ---------------------------------------------------------------- Run

// CA-575: salvo con HOOM_MOUSE=0, Run pide el mouse en la misma escritura que
// SecEntrar (SecEntrar y despues SecMouseSi) y lo devuelve en la misma que
// SecSalir (SecMouseNo y despues SecSalir) al salir con q y con ctrl-c; con
// HOOM_MOUSE=0 no escribe ninguna de las dos.
func TestCA575_RunPideYDevuelveElMouse(t *testing.T) {
	for _, tecla := range []string{"q", "\x03"} {
		for _, apagado := range []bool{false, true} {
			w := ttNuevoMundo(t, ttTableroPrincipal())
			if apagado {
				env := map[string]string{"NO_COLOR": "1", "HOOM_ASCII": "1", "LANG": "C", "HOOM_MOUSE": "0"}
				w.e.Getenv = func(k string) string { return env[k] }
			}
			fin := w.correr(nil)
			ttEsperar(t, "CA-575", "el primer cuadro", w, func() bool { return w.ultimoMarco() != nil })
			w.term.mandar(tecla)
			if code, pan := fin(t); code != 0 || pan != nil {
				t.Fatalf("CA-575: %q sale con 0: %d (%v)", tecla, code, pan)
			}
			ttRevisarDevuelta(t, "CA-575", tecla, w)
			salida := w.r.texto("escribe")
			if apagado {
				if strings.Contains(salida, tablero.SecMouseSi) || strings.Contains(salida, tablero.SecMouseNo) {
					t.Fatalf("CA-575: con HOOM_MOUSE=0 no se pide ni se devuelve el mouse: %q", ttCola(salida, 300))
				}
				continue
			}
			if !strings.HasPrefix(salida, tablero.SecEntrar+tablero.SecMouseSi) || strings.Count(salida, tablero.SecMouseSi) != 1 {
				t.Fatalf("CA-575: el mouse se pide una vez, junto con SecEntrar: %q", ttCola(salida, 300))
			}
			if !strings.HasSuffix(salida, tablero.SecMouseNo+tablero.SecSalir) || strings.Count(salida, tablero.SecMouseNo) != 1 {
				t.Fatalf("CA-575: el mouse se devuelve una vez, junto con SecSalir: %q", ttCola(salida, 300))
			}
			hay := false
			for _, e := range w.r.copia() {
				if e.que == "escribe" && strings.Contains(e.dato, tablero.SecMouseNo) {
					hay = strings.Contains(e.dato, tablero.SecMouseNo+tablero.SecSalir)
				}
			}
			if !hay {
				t.Fatalf("CA-575: SecMouseNo y SecSalir van en una sola escritura")
			}
		}
	}
}

// CA-575: con un error de la terminal (y su fin de archivo) y con un panic,
// Run tambien devuelve el mouse antes de SecSalir.
func TestCA575_RunDevuelveElMouseConErrorYPanic(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	fin := w.correr(nil)
	ttEsperar(t, "CA-575", "el primer cuadro", w, func() bool { return w.ultimoMarco() != nil })
	w.term.cortar()
	if code, _ := fin(t); code != 1 {
		t.Fatalf("CA-533: con EOF Run sale con 1: %d", code)
	}
	if s := w.r.texto("escribe"); !strings.HasSuffix(s, tablero.SecMouseNo+tablero.SecSalir) {
		t.Fatalf("CA-575: con un error de la terminal el mouse se devuelve: %q", ttCola(s, 200))
	}
	p := ttNuevoMundo(t, ttTableroPrincipal())
	p.datos.panico = true
	if _, pan := p.correr(nil)(t); pan == nil {
		t.Fatalf("CA-533: el panic sigue en la goroutine de Run")
	}
	if s := p.r.texto("escribe"); !strings.HasSuffix(s, tablero.SecMouseNo+tablero.SecSalir) {
		t.Fatalf("CA-575: con un panic el mouse se devuelve: %q", ttCola(s, 200))
	}
}

// CA-719, CA-575: un clic que llega por la terminal (en el formato SGR,
// cortado en dos lecturas) le da el foco a la zona bajo el clic: a 80x24 con
// el tablero principal y el foco en la lista (una columna, C = 16), la lista
// ocupa las lineas 6 a 13 (desde 1) y el detalle las 14 a 21; el clic va a
// la linea 18, en el detalle, que toma los signos del foco (su borde de
// arriba "#" + el titulo de nuc-tenancy); la lista, sin el foco, se pliega a
// una linea con el signo simple (CA-696: entre 60 y 99 columnas, con el foco
// en el detalle): "-Tarjetas · 1 de 7---".
func TestCA719_CA575_RunUnClicPorLaTerminal(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	w.datos.hallazgos = ttHallazgosDorados()
	fin := w.correr(nil)
	ttEsperar(t, "CA-719", "el tablero", w, func() bool { return w.filaElegida("nuc-tenancy") })
	w.term.mandar("\x1b[<0;3;")
	w.term.mandar("18M")
	ttEsperar(t, "CA-719", "el foco en el detalle", w, func() bool {
		return w.marcoCon("\n#Tenancy del nucleo=") && w.marcoCon("\n-Tarjetas · 1 de 7-")
	})
	w.term.mandar("q")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-575: q sale con 0: %d", code)
	}
}
