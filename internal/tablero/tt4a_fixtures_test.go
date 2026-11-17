// Fixtures de los tests de la pantalla de la v3 (spec
// .hoom/specs/tablero-tu-siguiente-paso.md, CA-696..CA-721; contrato en
// doc.go): el tablero de los dorados, con una tarjeta en cada grupo, y los
// helpers que leen la pantalla nueva (la cabecera, la caja TU SIGUIENTE
// PASO, la lista y el detalle) por su posicion, que el contrato fija, y no
// por sus titulos. Las cuentas de celdas usan la tabla propia de los tests
// (ttAnchoRuna), no la del paquete. Prefijo tv3a: los de este test-writer.
package tablero_test

import (
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- configuracion

// tv3aCfg es la configuracion de los tests de la v3.
func tv3aCfg(ancho, alto int, utf, color bool) tablero.Config {
	c := ttConfig(ancho, alto, utf)
	c.Color = color
	return c
}

// tv3aC es el alto del cuerpo: C = A - 8 (doc.go, "The screen").
func tv3aC(alto int) int { return alto - 8 }

// tv3aL es el ancho de la lista con dos zonas: W*2/5, division entera.
func tv3aL(ancho int) int { return ancho * 2 / 5 }

// ---------------------------------------------------------------- tarjetas

// tv3aSegs es una tira de cuatro segmentos.
func tv3aSegs() []boardcmd.Segment {
	return []boardcmd.Segment{
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"),
		ttSeg("pruebas", "pruebas", boardcmd.SegHecho, "4/4 con prueba"),
		ttSeg("verde", "verde", boardcmd.SegFalta, "sin verde"),
		ttSeg("review", "review", boardcmd.SegNoAplica, "todavia no"),
	}
}

// tv3aEn es una tarjeta en la columna col, con el plain "plain de <slug>",
// sin acciones.
func tv3aEn(slug, titulo, col string) boardcmd.Card {
	return ttCard(slug, titulo, col, "plain de "+slug, tv3aSegs()...)
}

// tv3aCon le pone a la tarjeta esas acciones.
func tv3aCon(c boardcmd.Card, acciones ...boardcmd.Action) boardcmd.Card {
	c.Actions = append([]boardcmd.Action{}, acciones...)
	return c
}

// tv3aCorre le pone un running del rol, empezado hace hace, con su limite
// en segundos (0 = sin limite).
func tv3aCorre(c boardcmd.Card, rol string, hace time.Duration, limite int) boardcmd.Card {
	c.Running = &boardcmd.Running{EnvelopeID: "env-" + c.Slug, RunID: "run-" + c.Slug, Role: rol, Provider: "claude",
		Stage: "run", Step: 3, Steps: 6, StartedAt: ttAhora.Add(-hace), TimeoutS: limite,
		LastEventAt: ttT(ttAhora.Add(-3 * time.Second))}
	return c
}

// tv3aRoja le pone un rojo de veredicto, esperado o no.
func tv3aRoja(c boardcmd.Card, plain string, esperado bool) boardcmd.Card {
	c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v-" + c.Slug, Reason: "test: FAIL", Plain: plain, Esperado: esperado}
	return c
}

// tv3aCortada le pone un interrupted del rol, cortado en el paso 3 de 5.
func tv3aCortada(c boardcmd.Card, rol string) boardcmd.Card {
	c.Interrupted = &boardcmd.Interrupted{EnvelopeID: "env-" + c.Slug, RunID: "run-" + c.Slug, Role: rol, Provider: "claude",
		Stage: "run", Step: 3, Steps: 5, UpdatedAt: ttAhora.Add(-time.Hour)}
	c.NeedsDecision = true
	return c
}

// tv3aConPiloto le enciende el piloto al item de la tarjeta.
func tv3aConPiloto(c boardcmd.Card) boardcmd.Card {
	c.Item.Auto = item.AutoHastaHumano
	return c
}

// Las acciones de rol de los fixtures, habilitadas.
func tv3aPedirArq() boardcmd.Action {
	return ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec")
}
func tv3aPedirTW() boardcmd.Action {
	return ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests")
}
func tv3aPedirRev() boardcmd.Action {
	return ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", "Revisa el cambio")
}
func tv3aReanudar() boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActReanudar, Role: "writer", Label: "Reanudar", Enabled: true, ResumeID: "sesion-1",
		Pedido: "Termina lo que se te pidio.", Providers: []boardcmd.ProviderOption{ttProvClaude(true)}, TimeoutS: 1800,
		Paths: []string{}, Origen: ttOrigenes()}
}

// ---------------------------------------------------------------- el tablero dorado

// tv3aExportar: TE TOCA en Tu aprobacion, con su firma.
func tv3aExportar() boardcmd.Card {
	c := ttCard("exportar-pdf", "Exportar reporte a PDF", boardcmd.ColTuAprobacion, "te toca aprobar el spec",
		ttSeg("spec", "spec", boardcmd.SegHecho, "pasa el lint"),
		ttSeg("aprobacion", "aprobacion", boardcmd.SegFalta, "sin tu firma"))
	c.Actions = []boardcmd.Action{ttAccionAprobar()}
	return c
}

// tv3aListado: TE TOCA en Writer, con el rojo ESPERADO del test-writer (sus
// tests fallan porque todavia no hay codigo) y pedir-writer habilitado.
func tv3aListado() boardcmd.Card {
	c := ttCard("listado", "Listado de proyectos", boardcmd.ColWriter, "los tests fallan: falta el codigo",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"),
		ttSeg("pruebas", "pruebas", boardcmd.SegHecho, "4/4 con prueba"),
		ttSeg("verde", "verde", boardcmd.SegFalta, "rojo esperado"),
		ttSeg("review", "review", boardcmd.SegNoAplica, "todavia no"))
	c = tv3aRoja(c, "los tests del test-writer fallan", true)
	c.Actions = []boardcmd.Action{ttAccionWriter()}
	return c
}

// tv3aModulo: CORRIENDO, el writer en Writer hace 14:02 con 30:00 de limite.
func tv3aModulo() boardcmd.Card {
	c := ttCard("modulo-usuarios", "Módulo de usuarios", boardcmd.ColWriter, "el writer esta trabajando", tv3aSegs()...)
	return tv3aCorre(c, "writer", 14*time.Minute+2*time.Second, 1800)
}

// tv3aPagos: DETENIDA por un interrupted del writer, en Writer.
func tv3aPagos() boardcmd.Card {
	c := ttCard("pagos", "Pagos con tarjeta", boardcmd.ColWriter, "el trabajo se corto: decide como sigue", tv3aSegs()...)
	c = tv3aCortada(c, "writer")
	c.Actions = []boardcmd.Action{tv3aReanudar()}
	return c
}

// tv3aNotificaciones: DETENIDA por un rojo que no es el esperado, en Review.
func tv3aNotificaciones() boardcmd.Card {
	c := ttCard("notificaciones", "Notificaciones por correo", boardcmd.ColReview, "la verificacion dio rojo", tv3aSegs()...)
	c = tv3aRoja(c, "el veredicto quedo rojo en test", false)
	c.Actions = []boardcmd.Action{tv3aPedirRev()}
	return c
}

// tv3aImportar: EN COLA, en Backlog, con pedir-arquitecto habilitado.
func tv3aImportar() boardcmd.Card {
	c := ttCard("importar-excel", "Importar desde Excel", boardcmd.ColBacklog, "falta el spec de la tarjeta", tv3aSegs()...)
	c.Actions = []boardcmd.Action{tv3aPedirArq()}
	return c
}

// tv3aTableroDorado: una tarjeta (o mas) en cada grupo. La lista esperada:
//
//	TE TOCA 2    listado (Writer, rojo esperado), exportar-pdf (Tu aprobacion)
//	CORRIENDO 1  modulo-usuarios
//	DETENIDA 2   pagos (Writer, interrupted), notificaciones (Review, rojo)
//	EN COLA 1    importar-excel
//	HECHO 2      viejo-1, viejo-2 (cerrado)
func tv3aTableroDorado() boardcmd.Board {
	return ttTablero(tv3aImportar(), tv3aExportar(), tv3aListado(), tv3aModulo(), tv3aPagos(), tv3aNotificaciones(),
		ttViejo(1), ttViejo(2))
}

// tv3aModeloDorado es el modelo de los dorados: el tablero (exportar-pdf
// fijada) y el proyecto verde de los fixtures de la v2; sin Detalle (las
// partes D a G dicen "leyendo la tarjeta...").
func tv3aModeloDorado(c tablero.Config) *tablero.Modelo {
	m := tablero.Nuevo(c)
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: tv3aTableroDorado(), Fijada: "exportar-pdf"})
	m.LeerProyecto(ttProyectoVerde())
	return m
}

// ---------------------------------------------------------------- textos

// tv3aFijo es un texto fijo de la TUI como lo escribe el modo: con UTF-8
// tal cual; en ASCII sin acentos, con " por « y », y <- por ← (CA-720).
func tv3aFijo(utf bool, s string) string {
	if utf {
		return s
	}
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "«", "\"", "»", "\"", "←", "<-", "…", "...")
	return r.Replace(s)
}

// tv3aQ es un titulo entre las comillas del modo.
func tv3aQ(utf bool, t string) string {
	if utf {
		return "«" + t + "»"
	}
	return "\"" + t + "\""
}

// tv3aCorte es el signo del corte del modo.
func tv3aCorte(utf bool) string {
	if utf {
		return "…"
	}
	return "~"
}

// tv3aCortar corta s a n celdas con la regla de doc.go: lo mas largo del
// principio que entra en n-1 celdas y el signo del corte (una runa ancha que
// no entra sale entera). s que entra queda igual.
func tv3aCortar(s string, n int, utf bool) string {
	if ttCeldas(s) <= n {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		a := ttAnchoRuna(r)
		if w+a > n-1 {
			break
		}
		b.WriteRune(r)
		w += a
	}
	return b.String() + tv3aCorte(utf)
}

// tv3aRelleno rellena s con espacios hasta n celdas.
func tv3aRelleno(s string, n int) string {
	if k := n - ttCeldas(s); k > 0 {
		return s + strings.Repeat(" ", k)
	}
	return s
}

// tv3aConDerecha es una linea de w celdas con izq a la izquierda y der
// pegado a la derecha, con un espacio de margen al final.
func tv3aConDerecha(izq, der string, w int) string {
	k := w - ttCeldas(izq) - ttCeldas(der) - 1
	if k < 0 {
		k = 0
	}
	return izq + strings.Repeat(" ", k) + der + " "
}

// tv3aSignos son los signos de un modo.
type tv3aSignos struct {
	Antes, Toca, Corre, Det, Desp       string // el camino
	Sel, HechoCerrado, HechoAbierto     string
	Lin, Lado, ESI, ESD, EII, EID       string // la caja simple
	FLin, FLado, FESI, FESD, FEII, FEID string // la caja del foco
	MarcaDet                            string // la marca de DETENIDA de una fila
	BarraLlena, BarraVacia              string
}

func tv3aSig(utf bool) tv3aSignos {
	if utf {
		return tv3aSignos{"●", "◆", "◉", "✗", "○", "▸", "▸", "▾", "─", "│", "┌", "┐", "└", "┘", "═", "║", "╔", "╗", "╚", "╝", "✗", "█", "░"}
	}
	return tv3aSignos{"#", "!", "*", "x", "-", ">", ">", "v", "-", "|", "+", "+", "+", "+", "=", "#", "#", "#", "#", "#", "x", "#", "-"}
}

// tv3aColumna es el indice (0..7) de una columna en boardcmd.Columns.
func tv3aColumna(id string) int {
	for i, k := range boardcmd.Columns {
		if k.ID == id {
			return i
		}
	}
	return -1
}

// tv3aCamino son los 8 signos del camino de una tarjeta de la columna col
// en el grupo g (CA-702).
func tv3aCamino(utf bool, col string, g tablero.Grupo) string {
	s := tv3aSig(utf)
	if g == tablero.GrupoHecho {
		return strings.Repeat(s.Antes, 8)
	}
	i := tv3aColumna(col)
	sig := map[tablero.Grupo]string{tablero.GrupoTeToca: s.Toca, tablero.GrupoCorriendo: s.Corre,
		tablero.GrupoDetenida: s.Det, tablero.GrupoEnCola: s.Desp}[g]
	return strings.Repeat(s.Antes, i) + sig + strings.Repeat(s.Desp, 7-i)
}

// tv3aArriba es el borde de arriba de una caja de w celdas (doc.go, "The
// boxes"): esquina, titulo cortado a w-2, signos de linea y, si entra
// despues del titulo con al menos un signo entre los dos, der con un signo
// de linea y la esquina.
func tv3aArriba(utf, foco bool, w int, titulo, der string) string {
	s := tv3aSig(utf)
	ei, ed, lin := s.ESI, s.ESD, s.Lin
	if foco {
		ei, ed, lin = s.FESI, s.FESD, s.FLin
	}
	t := tv3aCortar(titulo, w-2, utf)
	if der != "" && ttCeldas(t)+1+ttCeldas(der)+1 <= w-2 {
		return ei + t + strings.Repeat(lin, w-2-ttCeldas(t)-ttCeldas(der)-1) + der + lin + ed
	}
	return ei + t + strings.Repeat(lin, w-2-ttCeldas(t)) + ed
}

// tv3aAbajoDe es el borde de abajo, con "<n> de <m>" cuando der no es "".
func tv3aAbajoDe(utf, foco bool, w int, der string) string {
	s := tv3aSig(utf)
	ei, ed, lin := s.EII, s.EID, s.Lin
	if foco {
		ei, ed, lin = s.FEII, s.FEID, s.FLin
	}
	if der != "" && ttCeldas(der)+1 <= w-2 {
		return ei + strings.Repeat(lin, w-2-ttCeldas(der)-1) + der + lin + ed
	}
	return ei + strings.Repeat(lin, w-2) + ed
}

// ---------------------------------------------------------------- las cajas en la pantalla

// tv3aCaja es una caja leida de la pantalla en una posicion.
type tv3aCaja struct {
	Fila, Col, Ancho, Alto int
	Foco                   bool
	Arriba, Abajo          string
	Dentro                 []string // las lineas interiores, Ancho-2 celdas cada una
}

// tv3aTexto son las lineas interiores sin el relleno de la derecha.
func (c tv3aCaja) tv3aTexto() []string {
	var out []string
	for _, l := range c.Dentro {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

func tv3aEsqArriba(x string) bool { return x == "┌" || x == "╔" || x == "+" || x == "#" }
func tv3aEsqAbajo(x string) bool  { return x == "└" || x == "╚" || x == "+" || x == "#" }

// tv3aEsqArribaDer es la esquina de arriba a la derecha.
func tv3aEsqArribaDer(x string) bool { return x == "┐" || x == "╗" || x == "+" || x == "#" }

// tv3aCajaEn exige una caja con su esquina de arriba en (fila, col) y ancho
// celdas, y la lee: su borde de abajo es la primera linea mas abajo con una
// esquina en col seguida de un signo de linea; entre las dos, los lados.
func tv3aCajaEn(t *testing.T, ca string, m *tablero.Modelo, fila, col, ancho int) tv3aCaja {
	t.Helper()
	return tv3aCajaEnLineas(t, ca, ttLimpia(m), ttPantalla(m), fila, col, ancho)
}

// tv3aCajaEnLineas es tv3aCajaEn sobre las lineas ls de una pantalla (sin
// SGR); volcado es lo que se muestra si falla.
func tv3aCajaEnLineas(t *testing.T, ca string, ls []string, volcado string, fila, col, ancho int) tv3aCaja {
	t.Helper()
	if fila >= len(ls) {
		t.Fatalf("%s: la pantalla tiene una caja en la linea %d:\n%s", ca, fila+1, volcado)
	}
	c0 := ttCeldasDe(ls[fila])
	if len(c0) < col+ancho || !tv3aEsqArriba(c0[col]) || !tv3aEsqArribaDer(c0[col+ancho-1]) {
		t.Fatalf("%s: la pantalla tiene una caja de %d celdas con su esquina de arriba en la linea %d, celda %d (y la otra en la celda %d):\n%s",
			ca, ancho, fila+1, col, col+ancho-1, volcado)
	}
	caja := tv3aCaja{Fila: fila, Col: col, Ancho: ancho, Arriba: ttTramo(ls[fila], col, col+ancho),
		Foco: c0[col] == "╔" || c0[col] == "#"}
	for k := fila + 1; k < len(ls); k++ {
		ck := ttCeldasDe(ls[k])
		if len(ck) < col+ancho {
			break
		}
		if tv3aEsqAbajo(ck[col]) && ttEsLinea(ck[col+1]) {
			caja.Abajo = ttTramo(ls[k], col, col+ancho)
			caja.Alto = k - fila + 1
			return caja
		}
		if !ttEsLado(ck[col]) || !ttEsLado(ck[col+ancho-1]) {
			break
		}
		caja.Dentro = append(caja.Dentro, ttTramo(ls[k], col+1, col+ancho-1))
	}
	t.Fatalf("%s: la caja de la linea %d, celda %d (%d celdas), tiene sus lados y su borde de abajo:\n%s",
		ca, fila+1, col, ancho, volcado)
	return caja
}

// tv3aCajaPaso es la caja TU SIGUIENTE PASO: lineas 2 a 5, todo el ancho.
func tv3aCajaPaso(t *testing.T, ca string, m *tablero.Modelo, ancho int) tv3aCaja {
	t.Helper()
	c := tv3aCajaEn(t, ca, m, 1, 0, ancho)
	if c.Alto != 4 {
		t.Fatalf("%s: la caja TU SIGUIENTE PASO tiene 4 lineas (2 a 5), tiene %d:\n%s", ca, c.Alto, ttPantalla(m))
	}
	return c
}

// tv3aListaPlegada dice si la lista esta plegada (una linea que empieza con
// un signo de linea en la linea 6).
func tv3aListaPlegada(m *tablero.Modelo) bool {
	ls := ttLimpia(m)
	if len(ls) < 6 {
		return false
	}
	c := ttCeldasDe(ls[5])
	return len(c) > 0 && ttEsLinea(c[0])
}

// tv3aLista es la caja de la lista desplegada: con dos zonas, en (5, 0) con
// W*2/5 celdas; en una columna, en (5, 0) con W celdas.
func tv3aLista(t *testing.T, ca string, m *tablero.Modelo, ancho int) tv3aCaja {
	t.Helper()
	w := ancho
	if ancho >= 100 {
		w = tv3aL(ancho)
	}
	return tv3aCajaEn(t, ca, m, 5, 0, w)
}

// tv3aDetalle es la caja del detalle: con dos zonas, en (5, L); en una
// columna, debajo de la lista (o de su linea plegada).
func tv3aDetalle(t *testing.T, ca string, m *tablero.Modelo, ancho int) tv3aCaja {
	t.Helper()
	if ancho >= 100 {
		l := tv3aL(ancho)
		return tv3aCajaEn(t, ca, m, 5, l, ancho-l)
	}
	if tv3aListaPlegada(m) {
		return tv3aCajaEn(t, ca, m, 6, 0, ancho)
	}
	li := tv3aLista(t, ca, m, ancho)
	return tv3aCajaEn(t, ca, m, 5+li.Alto, 0, ancho)
}

// tv3aDetalleTexto son las lineas interiores del detalle, sin relleno.
func tv3aDetalleTexto(t *testing.T, ca string, m *tablero.Modelo, ancho int) []string {
	t.Helper()
	return tv3aDetalle(t, ca, m, ancho).tv3aTexto()
}

// tv3aDe es la linea i de ls o, si no la hay, un texto que no es ninguna
// linea de la pantalla (para comparar sin salirse del slice).
func tv3aDe(ls []string, i int) string {
	if i < 0 || i >= len(ls) {
		return fmt.Sprintf("<no hay linea %d: hay %d>", i+1, len(ls))
	}
	return ls[i]
}

// tv3aIndice es el indice de la primera linea igual a s, o -1.
func tv3aIndice(ls []string, s string) int {
	for i, l := range ls {
		if l == s {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------- la seleccion

// tv3aElegir lleva la seleccion de la lista a la tarjeta slug: g y despues
// j hasta que el detalle la muestra (el foco tiene que estar en la lista).
func tv3aElegir(t *testing.T, m *tablero.Modelo, slug string) {
	t.Helper()
	m.Tecla("g", ttAhora)
	for i := 0; i < 80; i++ {
		if m.Elegida() == slug {
			return
		}
		m.Tecla("j", ttAhora)
	}
	t.Fatalf("no se llego a elegir %s con g y j en la lista; pantalla:\n%s", slug, ttPantalla(m))
}

// tv3aAbrirHecho elige la linea de HECHO (G, la ultima fila) y la abre con
// enter.
func tv3aAbrirHecho(t *testing.T, m *tablero.Modelo) {
	t.Helper()
	m.Tecla("G", ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if !ttTiene(m, "HECHO") || !(ttTiene(m, " ▾") || ttTiene(m, " v")) {
		t.Fatalf("CA-700: G y enter abren HECHO (su linea termina en ▾ o v):\n%s", ttPantalla(m))
	}
}

// ---------------------------------------------------------------- el color

// tv3aCelda es una celda de una linea con color: su texto y los codigos de
// la SGR que la rige ("" sin estilo; "33", "7;33", "1;32"...).
type tv3aCelda struct{ Txt, Sgr string }

// tv3aCeldasCrudas parte una linea con sus SGR en celdas (con la tabla de
// anchos de los tests): una runa ancha ocupa dos celdas con el mismo
// estilo, una marca combinante se pega a la anterior, "\x1b[0m" vuelve a
// "sin estilo".
func tv3aCeldasCrudas(l string) []tv3aCelda {
	var out []tv3aCelda
	cur := ""
	for i := 0; i < len(l); {
		if strings.HasPrefix(l[i:], "\x1b[") {
			j := strings.IndexByte(l[i:], 'm')
			if j < 0 {
				break
			}
			codes := l[i+2 : i+j]
			if codes == "0" || codes == "" {
				cur = ""
			} else {
				cur = codes
			}
			i += j + 1
			continue
		}
		r, n := utf8.DecodeRuneInString(l[i:])
		i += n
		switch ttAnchoRuna(r) {
		case 0:
			if len(out) > 0 {
				out[len(out)-1].Txt += string(r)
			}
		case 2:
			out = append(out, tv3aCelda{string(r), cur}, tv3aCelda{"", cur})
		default:
			out = append(out, tv3aCelda{string(r), cur})
		}
	}
	return out
}

// tv3aCon7 dice si los codigos de una SGR incluyen el 7 (video inverso).
func tv3aCon7(sgr string) bool { return strings.Contains(";"+sgr+";", ";7;") }

// tv3aSgrEn son los codigos de la SGR de la celda x de la linea n (desde 1)
// de la pantalla; "?" si la linea no llega.
func tv3aSgrEn(m *tablero.Modelo, n, x int) string {
	p := m.Pantalla()
	if n < 1 || n > len(p) {
		return "?"
	}
	c := tv3aCeldasCrudas(p[n-1])
	if x < 0 || x >= len(c) {
		return "?"
	}
	return c[x].Sgr
}

// tv3aSgrTramo exige que las celdas [desde, hasta) de la linea n tengan
// todas los codigos sgr.
func tv3aSgrTramo(t *testing.T, ca, que string, m *tablero.Modelo, n, desde, hasta int, sgr string) {
	t.Helper()
	p := m.Pantalla()
	if n < 1 || n > len(p) {
		t.Fatalf("%s: %s: la pantalla tiene la linea %d", ca, que, n)
	}
	c := tv3aCeldasCrudas(p[n-1])
	if hasta > len(c) {
		t.Fatalf("%s: %s: la linea %d tiene %d celdas, se miran hasta la %d: %q", ca, que, n, len(c), hasta, p[n-1])
	}
	for x := desde; x < hasta; x++ {
		if c[x].Sgr != sgr {
			t.Fatalf("%s: %s: la celda %d (%q) de la linea %d va con la SGR %q, va con %q: %q",
				ca, que, x, c[x].Txt, n, sgr, c[x].Sgr, p[n-1])
		}
	}
}

// tv3aPosEn es la celda donde empieza s en la linea n (sin SGR), desde la
// celda desde; -1 si no esta.
func tv3aPosEn(m *tablero.Modelo, n int, s string, desde int) int {
	ls := ttLimpia(m)
	if n < 1 || n > len(ls) {
		return -1
	}
	c := ttCeldasDe(ls[n-1])
	k := ttCeldasDe(s)
	for x := desde; x+len(k) <= len(c); x++ {
		if strings.Join(c[x:x+len(k)], "") == s {
			return x
		}
	}
	return -1
}

// tv3aSgrDe exige el texto s en la linea n (desde la celda desde) y que
// todas sus celdas tengan los codigos sgr; devuelve su celda.
func tv3aSgrDe(t *testing.T, ca, que string, m *tablero.Modelo, n int, s string, desde int, sgr string) int {
	t.Helper()
	x := tv3aPosEn(m, n, s, desde)
	if x < 0 {
		t.Fatalf("%s: %s: la linea %d dice %q: %q", ca, que, n, s, ttLinea(m, n))
	}
	tv3aSgrTramo(t, ca, que, m, n, x, x+ttCeldas(s), sgr)
	return x
}

// tv3aLineaCon es el numero (desde 1) de la primera linea (sin SGR) que
// contiene s desde la celda desde; 0 si ninguna.
func tv3aLineaCon(m *tablero.Modelo, s string, desde int) int {
	for n := 1; n <= len(m.Pantalla()); n++ {
		if tv3aPosEn(m, n, s, desde) >= 0 {
			return n
		}
	}
	return 0
}

// tv3aDebeLinea exige una linea con s (desde la celda desde) y da su numero.
func tv3aDebeLinea(t *testing.T, ca string, m *tablero.Modelo, s string, desde int) int {
	t.Helper()
	n := tv3aLineaCon(m, s, desde)
	if n == 0 {
		t.Fatalf("%s: la pantalla dice %q (desde la celda %d):\n%s", ca, s, desde, ttPantalla(m))
	}
	return n
}

// tv3aLineas es la pantalla sin SGR como un texto numerado (para los
// mensajes de error).
func tv3aLineas(m *tablero.Modelo) string {
	var b strings.Builder
	for i, l := range ttLimpia(m) {
		fmt.Fprintf(&b, "%2d|%s\n", i+1, l)
	}
	return b.String()
}

// tv3aSinPanico convierte un panic en el fallo de ese test (con su pila),
// para que un panic del modelo no corte la corrida de los demas: cada test
// TestV3A_ lo difiere al empezar.
func tv3aSinPanico(t *testing.T) {
	if r := recover(); r != nil {
		t.Fatalf("panic: %v (la pila dice donde)\n%s", r, debug.Stack())
	}
}
