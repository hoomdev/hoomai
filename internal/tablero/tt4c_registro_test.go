// Tests adversariales de la v3 de hoom tablero: el Registro plegado en una
// linea y el modal Registro (CA-714).
package tablero_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/tablero"
)

// tv3cLineaRegistro es la linea del Registro plegado esperada: "-Registro",
// " · " y el texto cuando hay, y signos de linea hasta W celdas.
func tv3cLineaRegistro(texto string, ancho int, signo string) string {
	s := signo + "Registro"
	if texto != "" {
		s += " · " + texto
	}
	if n := ancho - ttCeldas(s); n > 0 {
		s += strings.Repeat(signo, n)
	}
	return s
}

// tv3cFijar aprieta u (hoom usar) a la hora ahora y devuelve su Lanzar.
func tv3cFijar(t *testing.T, ca string, m *tablero.Modelo, ahora time.Time) tablero.Lanzar {
	t.Helper()
	return ttUnaLanzar(t, ca, "u", m.Tecla("u", ahora))
}

// CA-714: el Registro ocupa una linea, la A-2, con la primera linea de su
// entrada mas nueva ("-Registro" y signos de linea cuando esta vacio), como
// el Registro plegado de la v2.
func TestV3C_CA714_ElRegistroPlegado(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-714"
	for _, ancho := range []int{100, 80, 60} {
		m := tv3cBase(tv3cConfig(ancho, 30))
		if got, quiero := tv3cRegistroLinea(m), tv3cLineaRegistro("", ancho, "-"); got != quiero {
			t.Fatalf("%s: a %d columnas, vacio\n got: %q\nwant: %q", ca, ancho, got, quiero)
		}
		l := tv3cFijar(t, ca, m, ttAhora)
		quiero := tv3cLineaRegistro("12:00 Fijar para la terminal · nuc-tenancy · lanzando...", ancho, "-")
		if ttCeldas(quiero) > ancho {
			quiero = string([]rune(quiero)[:ancho])
		}
		if got := tv3cRegistroLinea(m); ancho == 100 && got != quiero {
			t.Fatalf("%s: a %d columnas, con una entrada\n got: %q\nwant: %q", ca, ancho, got, quiero)
		} else if ancho < 100 && !strings.HasPrefix(got, "-Registro · 12:00 Fijar") {
			t.Fatalf("%s: a %d columnas la linea muestra la entrada nueva: %q", ca, ancho, got)
		}
		if ttCeldas(tv3cRegistroLinea(m)) != ancho {
			t.Fatalf("%s: a %d columnas la linea del Registro tiene W celdas: %q", ca, ancho, tv3cRegistroLinea(m))
		}
		if strings.Contains(tv3cRegistroLinea(m), "hoom usar") {
			t.Fatalf("%s: la linea plegada es solo la primera linea de la entrada: %q", ca, tv3cRegistroLinea(m))
		}
		tv3cFin(m, l.ID, 0, "")
		if ancho == 100 {
			if got, quiero := tv3cRegistroLinea(m), tv3cLineaRegistro("12:00 Fijar para la terminal · nuc-tenancy · hecho", ancho, "-"); got != quiero {
				t.Fatalf("%s: al terminar con 0\n got: %q\nwant: %q", ca, got, quiero)
			}
		}
	}
	// la mas nueva es la de abajo; un fallo en rojo
	m := tv3cBase(tv3cConfigColor(100, 30))
	tv3cFin(m, tv3cFijar(t, ca, m, ttAhora).ID, 0, "")
	ttTeclas(m, "j")
	l := tv3cFijar(t, ca, m, ttAhora.Add(5*time.Minute))
	tv3cFin(m, l.ID, 1, "no se pudo fijar")
	quiero := tv3cLineaRegistro("12:05 Fijar para la terminal · precios · fallo: no se pudo fijar", 100, "-")
	if got := tv3cRegistroLinea(m); got != quiero {
		t.Fatalf("%s: la entrada mas nueva\n got: %q\nwant: %q", ca, got, quiero)
	}
	crudo := m.Pantalla()[tv3cAlto(m)-3]
	if got := ttEstiloDe(crudo, "fallo: no se pudo fijar"); got != "\x1b[31m" {
		t.Fatalf("%s: un fallo va en rojo en la linea del Registro: %q en %q", ca, got, crudo)
	}
	// con UTF-8
	u := tv3cConfig(100, 30)
	u.UTF8 = true
	m = tv3cBase(u)
	tv3cFijar(t, ca, m, ttAhora)
	if got, quiero := tv3cRegistroLinea(m), tv3cLineaRegistro("12:00 Fijar para la terminal · nuc-tenancy · lanzando…", 100, "─"); got != quiero {
		t.Fatalf("%s: con UTF-8\n got: %q\nwant: %q", ca, got, quiero)
	}
}

// CA-714: L abre el modal Registro: "Registro", "todavia no se ejecuto nada"
// vacio, cada entrada con su comando y la mas nueva abajo; esc o L lo
// cierran y no ejecuta nada.
func TestV3C_CA714_ElModalRegistro(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-714"
	const ancho = 100
	m := tv3cBase(tv3cConfig(ancho, 30))
	for _, cierra := range []tablero.Tecla{tablero.TeclaEsc, "L"} {
		ttSinOrdenes(t, ca, "L", ttTeclas(m, "L"))
		if m.Modal() != tablero.ModalRegistro || m.Lugar() != tablero.LugarRegistro {
			t.Fatalf("%s: L abre el modal Registro: %q %q", ca, m.Modal(), m.Lugar())
		}
		mc := ttModal(t, ca, m, ancho)
		if !strings.Contains(mc.Arriba, " Registro ") {
			t.Fatalf("%s: el modal se titula Registro: %q", ca, mc.Arriba)
		}
		if strings.Join(mc.Textos, "|") != "todavia no se ejecuto nada" {
			t.Fatalf("%s: vacio dice \"todavia no se ejecuto nada\": %q", ca, mc.Textos)
		}
		ttSinOrdenes(t, ca, string(cierra), ttTeclas(m, cierra))
		tv3cSinModal(t, ca, string(cierra)+" en el Registro", m)
	}
	tv3cFin(m, tv3cFijar(t, ca, m, ttAhora).ID, 0, "")
	ttTeclas(m, "j")
	tv3cFijar(t, ca, m, ttAhora.Add(5*time.Minute))
	ttTeclas(m, "L")
	mc := ttModal(t, ca, m, ancho)
	quiero := []string{
		"12:00 Fijar para la terminal · nuc-tenancy · hecho",
		"      hoom usar nuc-tenancy",
		"12:05 Fijar para la terminal · precios · lanzando...",
		"      hoom usar precios",
	}
	if strings.Join(mc.Textos, "\n") != strings.Join(quiero, "\n") {
		t.Fatalf("%s: el modal Registro\n got: %q\nwant: %q", ca, mc.Textos, quiero)
	}
	for _, k := range []tablero.Tecla{"k", "j", tablero.TeclaPgArriba, tablero.TeclaPgAbajo, tablero.TeclaEnter, "u"} {
		ttSinOrdenes(t, ca, string(k)+" en el Registro", ttTeclas(m, k))
	}
	if m.Modal() != tablero.ModalRegistro {
		t.Fatalf("%s: solo esc o L cierran el Registro: %q", ca, m.Modal())
	}
}

var tv3cNdeM = regexp.MustCompile(`(\d+) de (\d+)`)

// tv3cVistaRegistro es el n y el m de "<n> de <m>" del borde de abajo del
// modal Registro.
func tv3cVistaRegistro(t *testing.T, ca string, m *tablero.Modelo, ancho int) (int, int, ttModalCaja) {
	t.Helper()
	mc := ttModal(t, ca, m, ancho)
	s := tv3cNdeM.FindStringSubmatch(mc.Abajo)
	if s == nil {
		t.Fatalf("%s: con lineas que no entran el borde de abajo dice \"<n> de <m>\": %q", ca, mc.Abajo)
	}
	n, _ := strconv.Atoi(s[1])
	tot, _ := strconv.Atoi(s[2])
	return n, tot, mc
}

// CA-714: con mas lineas de las que entran, el modal Registro abre en su
// final (la entrada mas nueva a la vista) y se desplaza como la Ayuda: ↑ ↓ k
// j una linea, pgup pgdn su alto interior, la rueda 3 lineas.
func TestV3C_CA714_ElRegistroLargo(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-714"
	const ancho = 100
	m := tv3cModelo(tv3cConfig(ancho, 30), tv3cMuchas(15)...)
	for i := 0; i < 15; i++ {
		tv3cFin(m, tv3cFijar(t, ca, m, ttAhora.Add(time.Duration(i)*time.Minute)).ID, 0, "")
		if i < 14 {
			ttTeclas(m, "j")
		}
	}
	ttTeclas(m, "L")
	n, tot, mc := tv3cVistaRegistro(t, ca, m, ancho)
	if tot != 30 || n != 30 {
		t.Fatalf("%s: 15 entradas son 30 lineas y abre en el final (30 de 30): %d de %d", ca, n, tot)
	}
	h := len(mc.Textos)
	if mc.Textos[h-1] != "      hoom usar t15" || mc.Textos[h-2] != fmt.Sprintf("12:14 Fijar para la terminal · t15 · hecho") {
		t.Fatalf("%s: abre con la entrada mas nueva abajo: %q", ca, mc.Textos)
	}
	paso := func(que string, quiero int, accion func()) {
		t.Helper()
		accion()
		if n, _, _ := tv3cVistaRegistro(t, ca, m, ancho); n != quiero {
			t.Fatalf("%s: %s deja la ultima linea a la vista en %d, esta en %d", ca, que, quiero, n)
		}
	}
	paso("k", 29, func() { ttTeclas(m, "k") })
	paso("↑", 28, func() { ttTeclas(m, tablero.TeclaArriba) })
	paso("j", 29, func() { ttTeclas(m, "j") })
	paso("↓", 30, func() { ttTeclas(m, tablero.TeclaAbajo) })
	paso("j en el final", 30, func() { ttTeclas(m, "j") })
	// pgup sube el alto interior, pero la vista no empieza antes de la
	// primera linea: la ultima a la vista queda en max(h, 30-h)
	paso("pgup", max(h, 30-h), func() { ttTeclas(m, tablero.TeclaPgArriba) })
	paso("pgdn", 30, func() { ttTeclas(m, tablero.TeclaPgAbajo) })
	dentro := tablero.Raton{X: mc.Col + 6, Y: mc.Fila + 2}
	paso("la rueda arriba", 27, func() {
		dentro.Accion = tablero.RatonRuedaArriba
		ttSinOrdenes(t, ca, "rueda", m.Raton(dentro, ttAhora))
	})
	paso("la rueda abajo", 30, func() {
		dentro.Accion = tablero.RatonRuedaAbajo
		ttSinOrdenes(t, ca, "rueda", m.Raton(dentro, ttAhora))
	})
	for i := 0; i < 40; i++ {
		ttTeclas(m, "k")
	}
	paso("k hasta el principio", h, func() {})
	if got := ttModal(t, ca, m, ancho).Textos[0]; got != "12:00 Fijar para la terminal · t01 · hecho" {
		t.Fatalf("%s: arriba de todo, la entrada mas vieja: %q", ca, got)
	}
}
