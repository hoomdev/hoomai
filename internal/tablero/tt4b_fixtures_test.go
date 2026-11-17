// Fixtures de la adaptacion de los tests de la v2 (tt_*, tt2_* y tt3_*) a la
// v3 de hoom tablero (spec .hoom/specs/tablero-tu-siguiente-paso.md,
// CA-696..CA-721; el contrato de la pantalla es doc.go). Los tests de la v2
// leian los paneles [0]..[4] por sus titulos (ttCajas); en la v3 no hay
// paneles numerados, asi que estos helpers leen la cabecera, la caja TU
// SIGUIENTE PASO, la lista y el detalle por el lugar que les da el contrato
// ("The screen", "The layout of the body") y traducen los nombres de la v2
// con la tabla "Los nombres de la v2" del spec:
//
//	[2]                         la lista de tarjetas      (tv3bLista, VistaTarjetas)
//	[0]                         el detalle                (tv3bDetalle)
//	[0] de una tarjeta de [2]   el detalle de la elegida  (tv3bDetalle con Elegida)
//	[1]                         la cabecera               (tv3bCabecera)
//	[0] de [1]                  el detalle con v          (LugarVerProyecto)
//	[4] y su [0]                la vista Hallazgos        (H: VistaHallazgos)
//	[3]                         el grupo CORRIENDO        (la lista)
//	el foco en [0]              el foco en el detalle     (ZonaDetalle: tab o →)
//	"vuelve a [2]"              "vuelve a la lista"       (ZonaLista)
//
// Ningun helper mira la implementacion: todo sale de doc.go y del spec.
package tablero_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- las zonas de la pantalla

// tv3bCaja es una caja de la v3 tal como esta en la pantalla: la de la v2
// (ttCaja: Fila, Col, Ancho, Alto, Plegado, Foco, Arriba, Titulo, Dentro,
// Abajo) mas lo que la v3 pone a la derecha de sus bordes. Titulo es solo la
// parte de la izquierda del borde de arriba.
type tv3bCaja struct {
	ttCaja
	// Derecha es la parte derecha del borde de arriba (las cuentas de la
	// caja, el slug del detalle), "" sin ella.
	Derecha string
	// Pie es lo que dice el borde de abajo entre sus signos de linea ("1 de
	// 7"), "" sin nada.
	Pie string
}

// tv3bZonas es la pantalla v3 de 60x15 o mas partida en sus zonas (doc.go,
// "The screen"): la cabecera (linea 1), la caja TU SIGUIENTE PASO (lineas
// 2..5), el cuerpo con la lista y el detalle (lineas 6..A-3), el Registro
// plegado (A-2), la linea de mensajes (A-1) y la de teclas (A), todas sin
// SGR.
type tv3bZonas struct {
	Ancho, Alto int
	Cabecera    string
	Caja        tv3bCaja
	Lista       tv3bCaja
	Detalle     tv3bCaja
	Registro    string
	Mensajes    string
	Teclas      string
}

// tv3bRuns parte celdas en tramos de signos de linea y de lo demas.
func tv3bRuns(celdas []string) (runs []string, linea []bool) {
	for _, x := range celdas {
		l := ttEsLinea(x)
		if len(runs) == 0 || linea[len(linea)-1] != l {
			runs, linea = append(runs, ""), append(linea, l)
		}
		runs[len(runs)-1] += x
	}
	return runs, linea
}

// tv3bPartirArriba parte el borde de arriba (las celdas entre sus dos
// esquinas) en el titulo y la parte derecha (doc.go, "The boxes": la parte
// derecha va pegada a la esquina con un signo de linea entre, y despues del
// titulo con al menos uno). El titulo son los tramos hasta el primer tramo de
// dos o mas signos de linea; lo que sigue a ese tramo, sin los signos del
// final, es la parte derecha. (Cuando el titulo y la parte derecha quedan a
// un solo signo, en ASCII no se distinguen de un "-" del texto: los tests de
// ese borde lo comparan entero.)
func tv3bPartirArriba(celdas []string) (titulo, derecha string) {
	runs, linea := tv3bRuns(celdas)
	if n := len(runs); n > 0 && linea[n-1] {
		runs, linea = runs[:n-1], linea[:n-1]
	}
	i := 0
	for ; i < len(runs); i++ {
		if linea[i] && ttCeldas(runs[i]) >= 2 {
			break
		}
		titulo += runs[i]
	}
	if i < len(runs) {
		derecha = strings.Join(runs[i+1:], "")
	}
	return titulo, derecha
}

// tv3bPartirAbajo es lo que dice el borde de abajo (las celdas entre sus dos
// esquinas), sin los signos de linea de las puntas.
func tv3bPartirAbajo(celdas []string) string {
	runs, linea := tv3bRuns(celdas)
	if n := len(runs); n > 0 && linea[n-1] {
		runs, linea = runs[:n-1], linea[:n-1]
	}
	if len(runs) > 0 && linea[0] {
		runs = runs[1:]
	}
	return strings.Join(runs, "")
}

// tv3bCajaEn lee la caja de ancho celdas cuya esquina de arriba a la
// izquierda esta en la linea fila (desde 0), celda col: su borde de abajo es
// la primera linea mas abajo con una esquina en col seguida de un signo de
// linea, y entre los dos cada linea empieza con un lado. Una linea que en col
// empieza con un signo de linea es una caja plegada (doc.go, "A folded box is
// one line"). Devuelve el motivo cuando ahi no hay una caja.
func tv3bCajaEn(ls []string, nombre string, fila, col, ancho int) (tv3bCaja, string) {
	if fila < 0 || fila >= len(ls) {
		return tv3bCaja{}, fmt.Sprintf("no hay linea %d", fila+1)
	}
	c := ttCeldasDe(ls[fila])
	if col >= len(c) {
		return tv3bCaja{}, fmt.Sprintf("la linea %d no llega a la celda %d: %q", fila+1, col, ls[fila])
	}
	if ttEsLinea(c[col]) {
		tramo := ttCeldasDe(ttTramo(ls[fila], col, col+ancho))
		k := tv3bCaja{ttCaja: ttCaja{Nombre: nombre, Fila: fila, Col: col, Ancho: len(tramo), Alto: 1, Plegado: true,
			Foco: c[col] == "=" || c[col] == "═", Arriba: strings.Join(tramo, ""), Titulo: ttTituloDe(tramo[1:])}}
		return k, ""
	}
	if !ttEsEsquina(c[col]) {
		return tv3bCaja{}, fmt.Sprintf("la linea %d no tiene una esquina en la celda %d: %q", fila+1, col, ls[fila])
	}
	if col+ancho-1 >= len(c) || !ttEsEsquina(c[col+ancho-1]) {
		return tv3bCaja{}, fmt.Sprintf("la linea %d no tiene la esquina de la derecha en la celda %d: %q", fila+1, col+ancho-1, ls[fila])
	}
	abajo := -1
	for k := fila + 1; k < len(ls); k++ {
		ck := ttCeldasDe(ls[k])
		if col+1 < len(ck) && ttEsEsquina(ck[col]) && ttEsLinea(ck[col+1]) {
			abajo = k
			break
		}
		if col >= len(ck) || !ttEsLado(ck[col]) {
			return tv3bCaja{}, fmt.Sprintf("la linea %d no sigue la caja de la linea %d (sin lado en la celda %d): %q", k+1, fila+1, col, ls[k])
		}
	}
	if abajo < 0 {
		return tv3bCaja{}, fmt.Sprintf("la caja de la linea %d no tiene borde de abajo", fila+1)
	}
	cb := ttCeldasDe(ls[abajo])
	if col+ancho-1 >= len(cb) || !ttEsEsquina(cb[col+ancho-1]) {
		return tv3bCaja{}, fmt.Sprintf("el borde de abajo (linea %d) no tiene la esquina de la derecha en la celda %d: %q", abajo+1, col+ancho-1, ls[abajo])
	}
	k := tv3bCaja{ttCaja: ttCaja{Nombre: nombre, Fila: fila, Col: col, Ancho: ancho, Alto: abajo - fila + 1,
		Foco: c[col] == "#" || c[col] == "╔", Arriba: ttTramo(ls[fila], col, col+ancho), Abajo: ttTramo(ls[abajo], col, col+ancho)}}
	arriba := ttCeldasDe(k.Arriba)
	k.Titulo, k.Derecha = tv3bPartirArriba(arriba[1 : len(arriba)-1])
	ab := ttCeldasDe(k.Abajo)
	k.Pie = tv3bPartirAbajo(ab[1 : len(ab)-1])
	for f := fila + 1; f < abajo; f++ {
		k.Dentro = append(k.Dentro, ttTramo(ls[f], col+1, col+ancho-1))
	}
	return k, ""
}

// tv3bLeer parte la pantalla de m en sus zonas (60x15 o mas). El ancho es el
// del borde de arriba de la caja TU SIGUIENTE PASO, que va de la celda 0 a la
// W-1; con 100 columnas o mas la lista tiene W*2/5 celdas y el detalle el
// resto, y entre 60 y 99 las dos van una debajo de la otra, de todo el ancho
// (CA-696). Si las zonas no estan donde dice el contrato, el test falla con
// ca.
func tv3bLeer(t *testing.T, ca string, m *tablero.Modelo) tv3bZonas {
	t.Helper()
	ls := ttLimpia(m)
	if len(ls) < 15 || len(ls) < 2 {
		t.Fatalf("%s: la pantalla tiene al menos 15 lineas para leer sus zonas:\n%s", ca, ttPantalla(m))
	}
	z := tv3bZonas{Alto: len(ls), Ancho: ttCeldas(ls[1]), Cabecera: ls[0],
		Registro: ls[len(ls)-3], Mensajes: ls[len(ls)-2], Teclas: ls[len(ls)-1]}
	var mal string
	if z.Caja, mal = tv3bCajaEn(ls, "caja", 1, 0, z.Ancho); mal != "" {
		t.Fatalf("%s (CA-696): la caja TU SIGUIENTE PASO va en las lineas 2 a 5: %s\n%s", ca, mal, ttPantalla(m))
	}
	if z.Caja.Alto != 4 || z.Caja.Plegado {
		t.Fatalf("%s (CA-696): la caja TU SIGUIENTE PASO tiene 4 lineas, tiene %d:\n%s", ca, z.Caja.Alto, ttPantalla(m))
	}
	if z.Ancho >= 100 {
		l := z.Ancho * 2 / 5
		if z.Lista, mal = tv3bCajaEn(ls, "lista", 5, 0, l); mal != "" {
			t.Fatalf("%s (CA-696): la lista va en la linea 6, celdas 0..%d: %s\n%s", ca, l-1, mal, ttPantalla(m))
		}
		if z.Detalle, mal = tv3bCajaEn(ls, "detalle", 5, l, z.Ancho-l); mal != "" {
			t.Fatalf("%s (CA-696): el detalle va en la linea 6, celdas %d..%d: %s\n%s", ca, l, z.Ancho-1, mal, ttPantalla(m))
		}
		return z
	}
	if z.Lista, mal = tv3bCajaEn(ls, "lista", 5, 0, z.Ancho); mal != "" {
		t.Fatalf("%s (CA-696): la lista va en la linea 6, de todo el ancho: %s\n%s", ca, mal, ttPantalla(m))
	}
	if z.Detalle, mal = tv3bCajaEn(ls, "detalle", 5+z.Lista.Alto, 0, z.Ancho); mal != "" {
		t.Fatalf("%s (CA-696): el detalle va debajo de la lista, de todo el ancho: %s\n%s", ca, mal, ttPantalla(m))
	}
	return z
}

// tv3bLista es la caja de la lista (la v2: [2], o [4] en la vista
// Hallazgos).
func tv3bLista(t *testing.T, ca string, m *tablero.Modelo) tv3bCaja {
	t.Helper()
	return tv3bLeer(t, ca, m).Lista
}

// tv3bDetalle es la caja del detalle (la v2: [0]).
func tv3bDetalle(t *testing.T, ca string, m *tablero.Modelo) tv3bCaja {
	t.Helper()
	return tv3bLeer(t, ca, m).Detalle
}

// tv3bCajaPaso es la caja TU SIGUIENTE PASO.
func tv3bCajaPaso(t *testing.T, ca string, m *tablero.Modelo) tv3bCaja {
	t.Helper()
	return tv3bLeer(t, ca, m).Caja
}

// tv3bCabecera es la linea 1 sin SGR (la v2: [1]).
func tv3bCabecera(m *tablero.Modelo) string {
	ls := ttLimpia(m)
	if len(ls) == 0 {
		return ""
	}
	return ls[0]
}

// tv3bRegistro es la linea A-2 sin SGR: el Registro plegado.
func tv3bRegistro(m *tablero.Modelo) string {
	ls := ttLimpia(m)
	if len(ls) < 3 {
		return ""
	}
	return ls[len(ls)-3]
}

// tv3bFocos son las zonas ("caja", "lista", "detalle") que llevan los signos
// del foco (doc.go: siempre una, la lista o el detalle; la caja nunca).
func tv3bFocos(t *testing.T, ca string, m *tablero.Modelo) []string {
	t.Helper()
	z := tv3bLeer(t, ca, m)
	var out []string
	for _, c := range []tv3bCaja{z.Caja, z.Lista, z.Detalle} {
		if c.Foco {
			out = append(out, c.Nombre)
		}
	}
	return out
}

// tv3bZonaDe es el nombre en pantalla de una Zona ("lista" o "detalle").
func tv3bZonaDe(z tablero.Zona) string {
	if z == tablero.ZonaDetalle {
		return "detalle"
	}
	return "lista"
}

// ---------------------------------------------------------------- las filas de la lista

// tv3bCaminoRe es el camino de 8 signos (UTF-8 o ASCII) de una fila de
// tarjeta, y lo que le sigue en la forma angosta.
var tv3bCaminoRe = regexp.MustCompile(`^(.*?) ([●◆◉✗○]{8}|[#!*x\-]{8})(?: (.*))?$`)

// tv3bPartirFila parte la primera linea de una fila de tarjeta (sin la marca
// ni su margen: lo que devuelve ttElegidaEn, o la linea sin sus tres primeras
// celdas) en el titulo, el camino y el estado de la forma angosta. Sin
// camino, todo es el titulo.
func tv3bPartirFila(s string) (titulo, camino, estado string) {
	if g := tv3bCaminoRe.FindStringSubmatch(s); g != nil {
		return strings.TrimRight(g[1], " "), g[2], g[3]
	}
	return strings.TrimRight(s, " "), "", ""
}

// tv3bElegida es la primera linea de la fila elegida de la lista, sin la
// marca ni el relleno ("" si ninguna): ttElegidaEn de la v2 sobre la caja de
// la lista.
func tv3bElegida(t *testing.T, ca string, m *tablero.Modelo) string {
	t.Helper()
	return ttElegidaEn(tv3bLista(t, ca, m).ttCaja)
}

// tv3bTituloElegido es el titulo de la fila elegida de la lista (la v2:
// ttSlugElegido; en la v3 la fila muestra el titulo, nunca el slug), o la
// linea de HECHO entera ("HECHO 3 >") cuando es la elegida.
func tv3bTituloElegido(t *testing.T, ca string, m *tablero.Modelo) string {
	t.Helper()
	e := tv3bElegida(t, ca, m)
	if strings.HasPrefix(e, "HECHO ") {
		return e
	}
	ti, _, _ := tv3bPartirFila(e)
	return ti
}

// tv3bFilaDe es el indice (en Dentro) de la primera linea de la lista cuya
// fila tiene ese titulo (con la marca o sin ella), y -1 si ninguna.
func tv3bFilaDe(c tv3bCaja, titulo string) int {
	for i, l := range c.Dentro {
		if len(l) < 3 {
			continue
		}
		cs := ttCeldasDe(l)
		if len(cs) < 3 || cs[0] != " " || cs[2] != " " {
			continue
		}
		if ti, _, _ := tv3bPartirFila(strings.Join(cs[3:], "")); ti == titulo {
			return i
		}
	}
	return -1
}

// tv3bSegunda es la segunda linea de la fila de la tarjeta con ese titulo
// (con 100 columnas o mas: tres espacios y su estado), sin el relleno; "" si
// la fila no esta o no tiene segunda linea.
func tv3bSegunda(c tv3bCaja, titulo string) string {
	i := tv3bFilaDe(c, titulo)
	if i < 0 || i+1 >= len(c.Dentro) {
		return ""
	}
	return strings.TrimRight(c.Dentro[i+1], " ")
}

// ---------------------------------------------------------------- moverse

// tv3bIrA mueve la seleccion de la lista de tarjetas (con ↓, y despues con
// ↑) hasta la tarjeta slug: la v2 ttIrA, que leia el slug de la fila de [2].
// Sirve con el foco en la lista y en la busqueda.
func tv3bIrA(t *testing.T, m *tablero.Modelo, slug string) {
	t.Helper()
	for i := 0; i < 80; i++ {
		if m.Elegida() == slug {
			return
		}
		m.Tecla(tablero.TeclaAbajo, ttAhora)
	}
	for i := 0; i < 160; i++ {
		if m.Elegida() == slug {
			return
		}
		m.Tecla(tablero.TeclaArriba, ttAhora)
	}
	t.Fatalf("no se llego a elegir %s con ↓ y ↑ en la lista; pantalla:\n%s", slug, ttPantalla(m))
}

// tv3bIrAHecho mueve la seleccion de la lista de tarjetas hasta la linea de
// HECHO (la v2: G en [2], con Hecho cerrada).
func tv3bIrAHecho(t *testing.T, m *tablero.Modelo) {
	t.Helper()
	for i := 0; i < 80; i++ {
		if strings.HasPrefix(tv3bElegida(t, "CA-707", m), "HECHO ") {
			return
		}
		m.Tecla(tablero.TeclaAbajo, ttAhora)
	}
	t.Fatalf("no se llego a la linea de HECHO con ↓; pantalla:\n%s", ttPantalla(m))
}

// tv3bAlDetalle pasa el foco de la lista al detalle con tab (la v2: enter,
// → o l en un panel, o 0; CA-706).
func tv3bAlDetalle(t *testing.T, m *tablero.Modelo) {
	t.Helper()
	m.Tecla(tablero.TeclaTab, ttAhora)
	if m.Foco() != tablero.ZonaDetalle {
		t.Fatalf("CA-706: tab desde la lista pasa el foco al detalle: Foco = %d, Lugar %q", int(m.Foco()), m.Lugar())
	}
}

// tv3bEnLugar lleva el modelo, con el foco en la lista de tarjetas sobre una
// tarjeta, al Lugar l de los paneles: LugarVerTarjeta con tab, LugarHallazgos
// con H, LugarVerHallazgo con H y tab, LugarVerProyecto con v, LugarVerNada
// con la linea de HECHO y tab, LugarBusqueda con /.
func tv3bEnLugar(t *testing.T, m *tablero.Modelo, l tablero.Lugar) {
	t.Helper()
	switch l {
	case tablero.LugarTarjetas:
	case tablero.LugarVerTarjeta:
		m.Tecla(tablero.TeclaTab, ttAhora)
	case tablero.LugarHallazgos:
		m.Tecla("H", ttAhora)
	case tablero.LugarVerHallazgo:
		m.Tecla("H", ttAhora)
		m.Tecla(tablero.TeclaTab, ttAhora)
	case tablero.LugarVerProyecto:
		m.Tecla("v", ttAhora)
	case tablero.LugarVerNada:
		tv3bIrAHecho(t, m)
		m.Tecla(tablero.TeclaTab, ttAhora)
	case tablero.LugarBusqueda:
		m.Tecla("/", ttAhora)
	}
	if m.Lugar() != l {
		t.Fatalf("CA-706: no se llego al Lugar %q, se quedo en %q:\n%s", l, m.Lugar(), ttPantalla(m))
	}
}

// ---------------------------------------------------------------- el tablero dorado en la v3

// tv3bOrdenDorado son las filas que se eligen de la lista de ttTableroDorado
// (y de ttTableroPrincipal), en orden (doc.go, "The groups" y "The list"):
// TE TOCA nuc-tenancy (Tu aprobacion) y precios (Arquitecto), CORRIENDO
// infra-api (running), DETENIDA pagos-caidos (interrupted), EN COLA
// infra-ci-cd y docs-guia (Backlog), y la linea de HECHO 3, cerrada ("").
var tv3bOrdenDorado = []string{"nuc-tenancy", "precios", "infra-api", "pagos-caidos", "infra-ci-cd", "docs-guia", ""}

// tv3bTitulos son los titulos de las tarjetas de los fixtures de la v2: la
// fila de la lista muestra el titulo, no el slug.
var tv3bTitulos = map[string]string{
	"nuc-tenancy": "Tenancy del nucleo", "precios": "Precios por region", "infra-api": "Convenciones de la API",
	"pagos-caidos": "Pagos caidos", "infra-ci-cd": "CI y CD de la infra", "docs-guia": "Guia de la documentacion",
	"viejo-1": "Viejo 1", "viejo-2": "Viejo 2", "viejo-3": "Viejo 3", "tarjeta": "Tarjeta de prueba",
}

// tv3bCamino es el camino de una tarjeta en la columna col y el grupo g
// (CA-702): ● (#) antes de su columna, el signo de su grupo en la suya (◆ !
// TE TOCA, ◉ * CORRIENDO, ✗ x DETENIDA, ○ - EN COLA), ○ (-) despues; en
// HECHO los 8 son ● (#).
func tv3bCamino(col string, g tablero.Grupo, utf bool) string {
	antes, despues := "#", "-"
	suyo := map[tablero.Grupo]string{tablero.GrupoTeToca: "!", tablero.GrupoCorriendo: "*", tablero.GrupoDetenida: "x",
		tablero.GrupoEnCola: "-", tablero.GrupoHecho: "#"}
	if utf {
		antes, despues = "●", "○"
		suyo = map[tablero.Grupo]string{tablero.GrupoTeToca: "◆", tablero.GrupoCorriendo: "◉", tablero.GrupoDetenida: "✗",
			tablero.GrupoEnCola: "○", tablero.GrupoHecho: "●"}
	}
	i := 0
	for k, c := range boardcmd.Columns {
		if c.ID == col {
			i = k
		}
	}
	var b strings.Builder
	for k := range boardcmd.Columns {
		switch {
		case g == tablero.GrupoHecho || k < i:
			b.WriteString(antes)
		case k == i:
			b.WriteString(suyo[g])
		default:
			b.WriteString(despues)
		}
	}
	return b.String()
}

// tv3bAnchoTitulo es el ancho del titulo de una fila con 100 columnas o mas
// (doc.go, "The rows"): room - 9, con room = (L-2) - 3 y L = W*2/5.
func tv3bAnchoTitulo(ancho int) int { return (ancho*2/5 - 2) - 3 - 9 }

// tv3bFilaAncha es la primera linea de una fila de tarjeta con 100 columnas
// o mas: " " + marca + " " + el titulo relleno a tv3bAnchoTitulo + " " + el
// camino (sin cortar: el titulo tiene que entrar).
func tv3bFilaAncha(ancho int, elegida, utf bool, titulo, camino string) string {
	m := " "
	if elegida {
		m = ">"
		if utf {
			m = "▸"
		}
	}
	return " " + m + " " + titulo + strings.Repeat(" ", tv3bAnchoTitulo(ancho)-ttCeldas(titulo)) + " " + camino
}
