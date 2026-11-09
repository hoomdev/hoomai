// Las cajas que no entran (CA-527, CA-529, CA-533): a 60x15 el formulario y
// las confirmaciones dejan afuera, en el orden del contrato, lo que no entra
// y la caja entra siempre entera en el cuerpo; guardar muestra sus primeras
// rutas e "y <n> rutas mas"; una firma que no entra entera no se firma.
// Regresiones de la ronda 1 (review completa 20261108T231004_195bc7 de
// tablero-en-la-terminal) y de la ronda 2 (review delta: hallazgo
// 20261109T003932_beb53d), escritas desde el contrato de hoy (doc.go y go doc
// del paquete), sin leer la implementacion.
package tablero_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- CA-527, CA-529, CA-533: cajas que no entran

// ttBordeAbajo dice si l es el borde de abajo de una caja ASCII a ese ancho.
func ttBordeAbajo(l string, ancho int) bool { return l == " +"+strings.Repeat("-", ancho-4)+"+" }

// ttCajaEntra exige la forma de la pantalla y que la caja entre en el
// cuerpo: empieza en la linea 2 y su borde de abajo esta a lo sumo en la
// linea A-3; el separador sigue en A-2.
func ttCajaEntra(t *testing.T, ca string, m *tablero.Modelo, ancho, alto int) {
	t.Helper()
	ttForma(t, ca, m, ancho, alto, false)
	p := m.Pantalla()
	if !strings.HasPrefix(p[1], " + ") {
		t.Fatalf("%s: la caja empieza en la linea 2: %q\n%s", ca, p[1], ttPantalla(m))
	}
	abajo := 0
	for i, l := range p {
		if ttBordeAbajo(l, ancho) {
			abajo = i + 1
		}
	}
	if abajo == 0 || abajo > alto-3 {
		t.Fatalf("%s: la caja entra en el cuerpo (lineas 2..%d), su borde de abajo esta en la linea %d:\n%s", ca, alto-3, abajo, ttPantalla(m))
	}
	if p[alto-3] != strings.Repeat("-", ancho) {
		t.Fatalf("%s: la linea A-2 sigue siendo el separador: %q", ca, p[alto-3])
	}
}

func ttTiene(ls []string, s string) bool {
	for _, l := range ls {
		if l == s {
			return true
		}
	}
	return false
}

// CA-527, CA-533 (hallazgo 9f25a9): a 60x15 el formulario del writer con un
// pedido de tres lineas deja afuera, en orden, las lineas vacias, la de las
// teclas y la primera del pedido, y conserva los campos, la linea del cursor
// y la linea ->. Enter lanza lo mismo que antes.
func TestCA527_CA533_FormularioChicoConservaLaFlecha(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = strings.Repeat("abcdefghij", 12) // 120 celdas: 54 + 54 + 12 en un interior de 54
	m := ttFormulario(t, ttConfig(60, 15, false), "w", a)
	ttCajaEntra(t, "CA-533", m, 60, 15)
	if got := ttFlecha(t, "CA-527", m); !strings.HasPrefix(got, "hoom pedir writer tarjeta -m ") {
		t.Fatalf("CA-527: la linea -> siempre queda: %q", got)
	}
	for _, et := range []string{"Provider", "Modelo", "Esfuerzo", "Presupuesto", "Tiempo limite", "Pedido"} {
		if ttCampo(m, et) == "" {
			t.Fatalf("CA-527: a 60x15 el campo %s queda:\n%s", et, ttPantalla(m))
		}
	}
	caja := ttCaja(m)
	for _, l := range caja {
		if l == "" {
			t.Fatalf("CA-527: lo primero que se va son las lineas vacias:\n%s", ttPantalla(m))
		}
		if strings.HasPrefix(l, "enter lanzar") {
			t.Fatalf("CA-527: la linea de las teclas se va antes que las del pedido:\n%s", ttPantalla(m))
		}
	}
	p := a.Pedido
	if !ttTiene(caja, p[108:]+"_") || !ttTiene(caja, p[54:108]) || ttTiene(caja, p[:54]) {
		t.Fatalf("CA-527: del pedido se van las lineas de arriba hasta la del cursor (queda lo que entra):\n%s", ttPantalla(m))
	}
	l := ttUnaLanzar(t, "CA-528", "enter a 60x15", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "a 60x15", l.Args, []string{"pedir", "writer", "tarjeta", "-m", p})
}

// CA-527, CA-533 (hallazgo 9f25a9): a 60x15, con providers que no pueden, sus
// lineas se van (despues de las vacias) y la de Provider termina en "  (<n>
// no pueden)"; la de las teclas entonces entra. A 80x24 la caja es la de
// antes: cada uno con su why y la de Provider sin la cuenta.
func TestCA527_CA533_ProvidersQueNoPuedenAChico(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola"
	a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), ttProvOpencode(),
		{Name: "gemini", OK: false, Why: "gemini no esta instalado"}, ttProvCodex(false)}
	m := ttFormulario(t, ttConfig(60, 15, false), "w", a)
	ttCajaEntra(t, "CA-533", m, 60, 15)
	if got := ttCampo(m, "Provider"); got != "  Provider       < claude >  (2 no pueden)" {
		t.Fatalf("CA-527: a 60x15 la linea de Provider cuenta los que no pueden: %q\n%s", got, ttPantalla(m))
	}
	for _, s := range []string{"opencode:", "gemini:"} {
		if l, n := ttLineaCon(m, s); n != 0 {
			t.Fatalf("CA-527: a 60x15 las lineas de los que no pueden se van: %q", l)
		}
	}
	caja := ttCaja(m)
	teclas := false
	for _, l := range caja {
		teclas = teclas || strings.HasPrefix(l, "enter lanzar")
	}
	if !teclas || !ttTiene(caja, "hola_") {
		t.Fatalf("CA-527: sin las vacias y sin los que no pueden la caja ya entra: quedan las teclas y el pedido:\n%s", ttPantalla(m))
	}
	if got := ttFlecha(t, "CA-527", m); !strings.HasPrefix(got, "hoom pedir writer tarjeta -m ") {
		t.Fatalf("CA-527: la linea -> queda: %q", got)
	}
	m = ttFormulario(t, ttConfig(80, 24, false), "w", a)
	ttCajaEntra(t, "CA-527", m, 80, 24)
	if got := ttCampo(m, "Provider"); got != "  Provider       < claude >" {
		t.Fatalf("CA-527: a 80x24 la linea de Provider es la de antes: %q", got)
	}
	for _, s := range []string{"opencode: " + ttWhyOpencode, "gemini: gemini no esta instalado"} {
		if _, n := ttLineaCon(m, s); n == 0 {
			t.Fatalf("CA-527: a 80x24 cada uno que no puede va con su why (%q):\n%s", s, ttPantalla(m))
		}
	}
}

// CA-529, CA-533 (hallazgo 9f25a9): a 60x15 la confirmacion de s con veinte
// rutas muestra las primeras, "y <n> rutas mas" en una ultima linea y su
// "enter guardar · esc cancelar"; enter guarda igual.
func TestCA529_CA533_GuardarConMuchasRutasAChico(t *testing.T) {
	var rutas []string
	for i := 0; i < 20; i++ {
		rutas = append(rutas, fmt.Sprintf("internal/archivo-%02d.go", i))
	}
	c := ttTarjetaCon(ttAccionGuardar(true, "", rutas...))
	c.Unsynced = append([]string{}, rutas...)
	m := ttModelo(ttConfig(60, 15, false), ttTablero(c))
	ttSinOrdenes(t, "CA-529", "s", m.Tecla("s", ttAhora))
	if m.Vista() != tablero.VistaConfirmacion {
		t.Fatalf("CA-529: s abre su confirmacion, la vista es %q", m.Vista())
	}
	ttCajaEntra(t, "CA-533", m, 60, 15)
	caja := ttCaja(m)
	if len(caja) < 3 || caja[0] != "Se guarda en git:" {
		t.Fatalf("CA-529: la confirmacion empieza con 'Se guarda en git:':\n%s", ttPantalla(m))
	}
	j := 0
	for j < len(rutas) && 1+j < len(caja) && caja[1+j] == rutas[j] {
		j++
	}
	if j == 0 || 1+j >= len(caja) || caja[1+j] != fmt.Sprintf("y %d rutas mas", len(rutas)-j) {
		t.Fatalf("CA-529: muestra las primeras rutas (%d) y 'y %d rutas mas' en una ultima linea:\n%s", j, len(rutas)-j, ttPantalla(m))
	}
	resto := caja[2+j:]
	if len(resto) > 0 && resto[0] == "" {
		resto = resto[1:]
	}
	if len(resto) != 1 || resto[0] != "enter guardar · esc cancelar" {
		t.Fatalf("CA-529: despues de las rutas queda 'enter guardar · esc cancelar' (y nada mas): %q\n%s", resto, ttPantalla(m))
	}
	l := ttUnaLanzar(t, "CA-529", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-529", "guardar a 60x15", l.Args, []string{"guardar", "tarjeta"})
}

// CA-529, CA-533 (hallazgo 9f25a9): una firma cuyo texto no entra entero
// nunca se firma: enter sobre Firmar no ordena nada y la barra dice en rojo
// "agranda la terminal: la firma no entra entera". Tambien si la terminal se
// achica con la firma abierta. Donde entra, firma.
func TestCA529_CA533_FirmaQueNoEntraNoSeFirma(t *testing.T) {
	const msg = "agranda la terminal: la firma no entra entera"
	c := ttNucTenancy()
	c.Item.Titulo = strings.TrimSpace(strings.Repeat("palabra ", 380)) // unas 3000 celdas: 17 lineas a 194
	firmar := func(m *tablero.Modelo) []tablero.Orden {
		t.Helper()
		if m.Vista() != tablero.VistaConfirmacion {
			t.Fatalf("CA-529: A abre la confirmacion, la vista es %q", m.Vista())
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
		ttSinOrdenes(t, "CA-529", "enter sobre Firmar "+que, firmar(m))
		if tam.ancho == 60 {
			// la barra se corta al ancho
			if b := ttBarra1(m); !strings.HasPrefix(b, " NORMAL · sin tarjeta fijada · agranda la terminal") {
				t.Fatalf("CA-529: %s la barra dice %q: %q", que, msg, b)
			}
			continue
		}
		ttBarraDice(t, "CA-529", que, m, tam.color, true, msg)
	}
	// abierta grande y achicada despues
	m := ttModelo(ttConfig(200, 40, false), ttTablero(c))
	m.Tecla("A", ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora)
	m.Tamano(200, 15)
	ttSinOrdenes(t, "CA-529", "enter sobre Firmar tras achicar", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttBarraDice(t, "CA-529", "tras achicar", m, false, true, msg)
	// donde entra, firma
	m = ttModelo(ttConfig(200, 40, false), ttTablero(c))
	m.Tecla("A", ttAhora)
	l := ttUnaLanzar(t, "CA-529", "enter sobre Firmar a 200x40", firmar(m))
	ttMismosArgs(t, "CA-529", "aprobar", l.Args, []string{"aprobar", "nuc-tenancy", "--si"})
}

// ---------------------------------------------------------------- CA-529, CA-533: guardar en el borde (ronda 2)

// ttCajaContigua lee la caja ASCII de la pantalla linea por linea: su borde de
// arriba en la linea 2, sus lineas interiores (" | <texto> |") seguidas y su
// borde de abajo, que esta en el cuerpo (a lo sumo en la linea A-3). Devuelve
// los textos interiores sin el relleno y el numero de linea del borde de
// abajo; una linea que no es de la caja antes del borde de abajo es una linea
// perdida.
func ttCajaContigua(t *testing.T, ca, que string, m *tablero.Modelo, ancho, alto int) ([]string, int) {
	t.Helper()
	ttForma(t, ca, m, ancho, alto, false)
	p := m.Pantalla()
	if !strings.HasPrefix(p[1], " +") || !strings.HasSuffix(p[1], "+") {
		t.Fatalf("%s: %s: la caja empieza en la linea 2 con su borde de arriba: %q\n%s", ca, que, p[1], ttPantalla(m))
	}
	var dentro []string
	for i := 2; i < len(p); i++ {
		l := p[i]
		if ttBordeAbajo(l, ancho) {
			if i+1 > alto-3 {
				t.Fatalf("%s: %s: la caja entra en el cuerpo (lineas 2..%d), su borde de abajo esta en la linea %d:\n%s", ca, que, alto-3, i+1, ttPantalla(m))
			}
			if p[alto-3] != strings.Repeat("-", ancho) {
				t.Fatalf("%s: %s: la linea A-2 sigue siendo el separador: %q", ca, que, p[alto-3])
			}
			return dentro, i + 1
		}
		if len(l) < 5 || !strings.HasPrefix(l, " | ") || !strings.HasSuffix(l, " |") {
			t.Fatalf("%s: %s: la linea %d es de la caja (o su borde de abajo): ninguna linea de la caja se pierde: %q\n%s", ca, que, i+1, l, ttPantalla(m))
		}
		dentro = append(dentro, strings.TrimRight(l[3:len(l)-2], " "))
	}
	t.Fatalf("%s: %s: el borde de abajo de la caja esta en pantalla:\n%s", ca, que, ttPantalla(m))
	return nil, 0
}

// CA-529, CA-533 (hallazgo 20261109T003932_beb53d): el borde exacto de la
// confirmacion de s a 60x15. El cuerpo tiene Alto-4 = 11 lineas, los dos
// bordes de la caja incluidos: la caja entera es su borde, "Se guarda en
// git:", una linea por ruta, una vacia, "enter guardar · esc cancelar" y su
// borde, n+5 lineas. Hasta 6 rutas entra entera. Con 7 se va la linea vacia
// (lo primero que se deja afuera, y solo hasta que entra) y entran las 7: es
// la cantidad exacta de rutas que entran. Con una mas (8) y con dos mas (9),
// y con 20, muestra las 6 primeras y "y <n> rutas mas" en una ultima linea (n
// = las que no muestra), sin la vacia. En todos los casos la caja entra
// entera en el cuerpo: su borde de abajo y su "enter guardar · esc cancelar"
// en pantalla, ninguna linea de la caja perdida; enter guarda.
func TestCA529_CA533_GuardarAChicoEnElBordeExacto(t *testing.T) {
	const teclas = "enter guardar · esc cancelar"
	for _, n := range []int{5, 6, 7, 8, 9, 20} {
		que := fmt.Sprintf("%d rutas a 60x15", n)
		var rutas []string
		for i := 0; i < n; i++ {
			rutas = append(rutas, fmt.Sprintf("internal/archivo-%02d.go", i))
		}
		quiere := []string{"Se guarda en git:"}
		switch {
		case n <= 6:
			quiere = append(append(quiere, rutas...), "", teclas)
		case n == 7:
			quiere = append(append(quiere, rutas...), teclas)
		default:
			quiere = append(append(quiere, rutas[:6]...), fmt.Sprintf("y %d rutas mas", n-6), teclas)
		}
		c := ttTarjetaCon(ttAccionGuardar(true, "", rutas...))
		c.Unsynced = append([]string{}, rutas...)
		m := ttModelo(ttConfig(60, 15, false), ttTablero(c))
		ttSinOrdenes(t, "CA-529", que+": s", m.Tecla("s", ttAhora))
		if m.Vista() != tablero.VistaConfirmacion {
			t.Fatalf("CA-529: %s: s abre su confirmacion, la vista es %q", que, m.Vista())
		}
		got, abajo := ttCajaContigua(t, "CA-533", que, m, 60, 15)
		if strings.Join(got, "\n") != strings.Join(quiere, "\n") {
			t.Fatalf("CA-529: %s: la caja de s dice\n got: %q\nwant: %q\n%s", que, got, quiere, ttPantalla(m))
		}
		if abajo != len(quiere)+3 {
			t.Fatalf("CA-533: %s: el borde de abajo sigue a la ultima linea de la caja (linea %d), esta en la %d", que, len(quiere)+3, abajo)
		}
		l := ttUnaLanzar(t, "CA-529", que+": enter", m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, "CA-529", que, l.Args, []string{"guardar", "tarjeta"})
	}
}
