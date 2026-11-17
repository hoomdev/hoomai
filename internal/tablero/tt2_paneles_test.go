// Lo que muestra cada zona, adaptado de la v2 (spec tablero-con-paneles) a la
// v3 (spec tablero-tu-siguiente-paso). La lista de tarjetas (la v2: [2]
// Tarjetas; CA-555 re-expresado por CA-700 y CA-701), la busqueda (CA-554),
// el detalle de la tarjeta elegida (la v2: [0] de una tarjeta; CA-556
// re-expresado por CA-708, y CA-557), el grupo CORRIENDO y el detalle de lo
// que corre (la v2: [3] Corriendo; CA-558 re-expresado por CA-711), la vista
// Hallazgos (la v2: [4] y su [0]; CA-559), la cabecera y el detalle con v (la
// v2: [1] y su [0]; CA-560) y la tarjeta que desaparece (CA-578). Las lineas
// esperadas salen de doc.go ("The list", "The rows", "The groups", "The
// path", "The detail of a card", "The Hallazgos view", "The detail of the
// project", "The step of a card"), escritas aca a mano; los nombres de la v2
// se leen con la tabla "Los nombres de la v2" del spec.
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
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/statuscmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ttIgualesAlPrincipio exige que las primeras lineas de got sean quiere.
func ttIgualesAlPrincipio(t *testing.T, ca, que string, got, quiere []string) {
	t.Helper()
	if len(got) < len(quiere) {
		t.Fatalf("%s: %s: se esperaban al menos %d lineas, hay %d:\n%s", ca, que, len(quiere), len(got), strings.Join(got, "\n"))
	}
	for i, q := range quiere {
		if got[i] != q {
			t.Fatalf("%s: %s: la linea %d es\n %q\nse esperaba\n %q\n--- todas:\n%s", ca, que, i+1, got[i], q, strings.Join(got, "\n"))
		}
	}
}

// ---------------------------------------------------------------- helpers del detalle v3

// tv3bPanAnchoDetalle es el ancho del detalle con 100 columnas o mas: W -
// W*2/5 (doc.go, "The layout of the body").
func tv3bPanAnchoDetalle(ancho int) int { return ancho - ancho*2/5 }

// tv3bPanDerecha es una linea con parte derecha (AHORA, EL CAMINO, la linea
// de la columna de la tarjeta en el camino) en una caja de ancho celdas, como
// la da ttTexto (sin el relleno): izq, espacios y der terminando en la celda
// w-3, con un espacio antes del lado (doc.go, "The boxes" y "The detail of a
// card").
func tv3bPanDerecha(ancho int, izq, der string) string {
	return izq + strings.Repeat(" ", ancho-3-ttCeldas(izq)-ttCeldas(der)) + der
}

// tv3bPanCabecera es la cabecera (linea 1) de W celdas sin el espacio del
// final: " " + la parte izquierda, espacios y la parte derecha terminando en
// la celda W-2 (doc.go, "The header").
func tv3bPanCabecera(ancho int, izq, der string) string {
	return izq + strings.Repeat(" ", ancho-1-ttCeldas(izq)-ttCeldas(der)) + der
}

// tv3bPanPasos son los nombres de los pasos del camino en ASCII (sin los
// acentos de las palabras fijas; doc.go, "The path"), uno por columna de
// boardcmd.Columns.
var tv3bPanPasos = []string{"Pedido", "Spec", "Tu firma", "Tests", "Codigo", "Review", "Tu aceptacion", "Hecho"}

// tv3bPanCamino son las 9 lineas de EL CAMINO, en ASCII y en la forma ancha,
// de una tarjeta en la columna col y el grupo g, en un detalle de ancho
// celdas: la del titulo con "paso <i> de 8" y una por paso; la de su columna
// con el nombre relleno a 15, estado y "<- aca" terminando en la celda w-3.
func tv3bPanCamino(ancho int, col string, g tablero.Grupo, estado string) []string {
	signos := tv3bCamino(col, g, false)
	i := 0
	for k, c := range boardcmd.Columns {
		if c.ID == col {
			i = k
		}
	}
	out := []string{tv3bPanDerecha(ancho, " EL CAMINO", fmt.Sprintf("paso %d de 8", i+1))}
	for k, n := range tv3bPanPasos {
		l := "  " + signos[k:k+1] + " " + n
		if k == i {
			l = tv3bPanDerecha(ancho, "  "+signos[k:k+1]+" "+fmt.Sprintf("%-15s", n)+estado, "<- aca")
		}
		out = append(out, l)
	}
	return out
}

// tv3bPanIndice es el indice de la primera linea igual a s, o -1.
func tv3bPanIndice(ls []string, s string) int {
	for i, l := range ls {
		if l == s {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------- CA-700, CA-701 (re-expresan CA-555): la lista

// CA-700, CA-701: la lista de tarjetas (la v2: [2]) muestra los grupos en su
// orden (TE TOCA, CORRIENDO, DETENIDA, EN COLA y HECHO cerrado con su
// cantidad), cada tarjeta una sola vez, sin grupos vacios salvo HECHO; TE
// TOCA va por columna de la ultima a la primera (nuc-tenancy en Tu
// aprobacion antes que precios en Arquitecto); cada fila es la marca, el
// titulo relleno y el camino de 8 signos pegado al borde derecho, y una
// segunda linea con el estado. Re-expresa CA-555 y CA-518.
func TestCA700_CA701_CA555_CA518_GruposYFilas(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	// L = 48: interior 46, room = 43, el titulo va relleno a 34
	cam := func(col string, g tablero.Grupo) string { return tv3bCamino(col, g, false) }
	precios := "   " + ttPlainPrecios
	precios = precios[:45] + "~" // la segunda linea se corta a w-2 = 46
	quiere := []string{
		" TE TOCA 2",
		tv3bFilaAncha(120, true, false, "Tenancy del nucleo", cam(boardcmd.ColTuAprobacion, tablero.GrupoTeToca)),
		"   te toca aprobar el spec",
		tv3bFilaAncha(120, false, false, "Precios por region", cam(boardcmd.ColArquitecto, tablero.GrupoTeToca)),
		precios,
		" CORRIENDO 1",
		tv3bFilaAncha(120, false, false, "Convenciones de la API", cam(boardcmd.ColWriter, tablero.GrupoCorriendo)),
		"   writer · 12:41 de 45:00",
		" DETENIDA 1",
		tv3bFilaAncha(120, false, false, "Pagos caidos", cam(boardcmd.ColWriter, tablero.GrupoDetenida)),
		"   x se corto el trabajo del writer",
		" EN COLA 2",
		tv3bFilaAncha(120, false, false, "CI y CD de la infra", cam(boardcmd.ColBacklog, tablero.GrupoEnCola)),
		"   falta el spec de la tarjeta",
		tv3bFilaAncha(120, false, false, "Guia de la documentacion", cam(boardcmd.ColBacklog, tablero.GrupoEnCola)),
		"   falta el spec de la tarjeta",
		" HECHO 3 >",
	}
	if quiere[1] != " > Tenancy del nucleo                 ##!-----" {
		t.Fatalf("CA-702: el camino de nuc-tenancy (Tu aprobacion, TE TOCA) es ##!-----: %q", quiere[1])
	}
	c := tv3bLista(t, "CA-701", m)
	ttIgualesAlPrincipio(t, "CA-701", "la lista a 120x32", c.ttTexto(), quiere)
	for _, l := range c.ttTexto()[len(quiere):] {
		if l != "" {
			t.Fatalf("CA-700: despues de HECHO cerrado no hay nada: %q", c.ttTexto())
		}
	}
	for _, l := range c.Dentro {
		if ttCeldas(l) != 46 {
			t.Fatalf("CA-701: cada linea interior mide w-2 = 46 celdas: %q", l)
		}
	}
	if c.Titulo != "Tarjetas" {
		t.Fatalf("CA-701: el titulo de la lista es 'Tarjetas': %q", c.Titulo)
	}
	// con UTF-8: la seleccion es ▸ y el camino ● ◆ ○, pegado al borde
	u := ttModeloDorado(ttConfig(120, 32, true))
	qu := []string{
		" TE TOCA 2",
		tv3bFilaAncha(120, true, true, "Tenancy del nucleo", "●●◆○○○○○"),
		"   te toca aprobar el spec",
		tv3bFilaAncha(120, false, true, "Precios por region", "●◆○○○○○○"),
	}
	ttIgualesAlPrincipio(t, "CA-701", "la lista con UTF-8", tv3bLista(t, "CA-701", u).ttTexto(), qu)
}

// CA-701: el corte del titulo: con 100 columnas o mas se corta con el signo
// del corte (o se rellena) a room - 9 para dejar el camino pegado al borde;
// sin titulo, la fila muestra el slug; el camino tiene siempre 8 signos,
// cualquiera sea el meter de la tarjeta. La v2 dejaba la fila sin tira
// cuando al slug le quedaban menos de 8 celdas: en la v3 eso pide una lista
// de menos de 22 celdas, que no existe desde 60 columnas, asi que se prueba
// lo que lo reemplaza: un titulo largo se corta y el camino queda, tambien
// en la forma angosta. Re-expresa CA-555.
func TestCA701_CA555_CorteDelTituloYElCamino(t *testing.T) {
	segs := func(n int) []boardcmd.Segment {
		var out []boardcmd.Segment
		for i := 0; i < n; i++ {
			out = append(out, ttSeg(fmt.Sprintf("s%d", i), "s", boardcmd.SegHecho, "d"))
		}
		return out
	}
	largo := "un titulo de mas de cuarenta celdas que no entra en ninguna lista"
	// 100 columnas: L = 40, interior 38, room = 35, el titulo a 26
	casos := []struct {
		nombre string
		ancho  int
		slug   string
		titulo string
		n      int
		utf    bool
		quiere string
	}{
		{"titulo cortado", 100, "t", "abcdefghijklmnopqrstuvwxyz0123", 4, false, " > abcdefghijklmnopqrstuvwxy~ --------"},
		{"titulo cortado con UTF-8", 100, "t", "abcdefghijklmnopqrstuvwxyz0123", 4, true, " ▸ abcdefghijklmnopqrstuvwxy… ○○○○○○○○"},
		{"26 celdas justas, 12 segmentos", 100, "t", "abcdefghijklmnopqrstuvwxyz", 12, false, " > abcdefghijklmnopqrstuvwxyz --------"},
		{"titulo de un caracter", 100, "t", "x", 4, false, tv3bFilaAncha(100, true, false, "x", "--------")},
		{"sin titulo: el slug", 100, "solo-el-slug", "", 4, false, tv3bFilaAncha(100, true, false, "solo-el-slug", "--------")},
		{"titulo largo: el camino queda", 100, "t", largo, 13, false, " > " + largo[:25] + "~ --------"},
		// 80 columnas (angosta): t = max(8, min(L, (78-12)/2 = 33)) = 33
		{"angosta: titulo largo, el camino queda", 80, "t", largo, 13, false, " > " + largo[:32] + "~ -------- p"},
	}
	for _, c := range casos {
		card := ttCard(c.slug, c.titulo, boardcmd.ColBacklog, "p", segs(c.n)...)
		m := ttModelo(ttConfig(c.ancho, 24, c.utf), ttTablero(card))
		got := tv3bLista(t, "CA-701", m).ttTexto()
		if len(got) < 2 || got[1] != c.quiere {
			t.Fatalf("CA-701 (%s): la fila es\n %q\nse esperaba\n %q", c.nombre, got, c.quiere)
		}
	}
}

// CA-700, CA-701: con color, el nombre de cada grupo va en su estilo (TE
// TOCA 7;33, CORRIENDO 7;36, DETENIDA 7;31, EN COLA y HECHO 90) y la segunda
// linea de cada fila en el color de su grupo (33, 36, 31, 90, 90); el titulo
// no lleva estilo. Una tarjeta con red no esperado (aunque este en Backlog)
// o con interrupted va a DETENIDA y su estado va en rojo: lo que la v2 decia
// con el slug en rojo. Una con red esperado no se detiene: TE TOCA.
// Re-expresa CA-555.
func TestCA700_CA701_CA555_ColoresDeGrupoYEstado(t *testing.T) {
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	acepta := ttCard("acepta", "Acepta", boardcmd.ColTuAceptacion, "lista", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	acepta.NeedsDecision, acepta.WaitingHuman = false, false
	conRojo := ttBacklog("con-rojo", "Con rojo")
	conRojo.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v", Plain: "dio rojo"}
	esperado := ttCard("esperado", "Esperado", boardcmd.ColTestWriter, "los tests fallan", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	esperado.Red = &boardcmd.Red{Source: boardcmd.RedEnvelope, ID: "e", Plain: "rojo del test-writer", Esperado: true}
	m := ttModelo(cfg, ttTablero(ttNucTenancy(), ttInfraAPI(true), ttPagosCaidos(), conRojo, acepta, esperado,
		ttBacklog("en-cola", "En cola"), ttViejo(1)))
	for slug, g := range map[string]tablero.Grupo{"con-rojo": tablero.GrupoDetenida, "esperado": tablero.GrupoTeToca,
		"pagos-caidos": tablero.GrupoDetenida, "infra-api": tablero.GrupoCorriendo, "en-cola": tablero.GrupoEnCola, "viejo-1": tablero.GrupoHecho} {
		if got := m.Grupo(slug); got != g {
			t.Fatalf("CA-700: %s cae en %q, cae en %q", slug, g, got)
		}
	}
	// HECHO abierto y el foco en el detalle: sin la barra de la seleccion en
	// la lista y sin nada en el detalle
	tv3bIrAHecho(t, m)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora)
	estilo := func(texto string) string {
		t.Helper()
		p := m.Pantalla()
		for _, l := range p[5:] { // el cuerpo: la caja de arriba repite el plain de su tarjeta
			if strings.Contains(ttSinSGR(l), texto) {
				return ttEstiloDe(l, texto)
			}
		}
		t.Fatalf("CA-701: el cuerpo tiene %q:\n%s", texto, ttPantalla(m))
		return ""
	}
	for texto, quiere := range map[string]string{
		"TE TOCA":                               "\x1b[7;33m",
		"CORRIENDO":                             "\x1b[7;36m",
		"DETENIDA":                              "\x1b[7;31m",
		"EN COLA":                               "\x1b[90m",
		"HECHO":                                 "\x1b[90m",
		"te toca aprobar el spec":               "\x1b[33m",
		"los tests fallan":                      "\x1b[33m",
		"writer · 12:41 de 45:00":               "\x1b[36m",
		"x se corto el trabajo del writer":      "\x1b[31m",
		"x dio rojo":                            "\x1b[31m",
		"falta el spec de la tarjeta":           "\x1b[90m",
		"terminada: su cierre quedo registrado": "\x1b[90m",
		"Tenancy del nucleo":                    "",
	} {
		if got := estilo(texto); got != quiere {
			t.Fatalf("CA-701: con color %q va con %q, va con %q", texto, quiere, got)
		}
	}
}

// CA-701, CA-578: antes de la primera lectura la lista dice "leyendo el
// tablero..."; un proyecto sin items, `no hay tarjetas: hoom item add
// "<titulo>"`, y el detalle no muestra nada. Re-expresa CA-555 y CA-535.
func TestCA701_CA555_CA578_CA535_LeyendoYSinTarjetas(t *testing.T) {
	m := tablero.Nuevo(ttConfig(200, 40, false))
	if got := tv3bLista(t, "CA-701", m).ttTexto(); len(got) == 0 || got[0] != " leyendo el tablero..." {
		t.Fatalf("CA-701: antes de la primera lectura la lista dice ' leyendo el tablero...': %q", got)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero()})
	if got := tv3bLista(t, "CA-578", m).ttTexto(); len(got) == 0 || got[0] != ` no hay tarjetas: hoom item add "<titulo>"` {
		t.Fatalf("CA-578: sin items la lista dice `no hay tarjetas: hoom item add \"<titulo>\"`: %q", got)
	}
	d := tv3bDetalle(t, "CA-578", m)
	if d.Titulo != "" || d.Derecha != "" || strings.TrimSpace(strings.Join(d.ttTexto(), "")) != "" || m.Elegida() != "" {
		t.Fatalf("CA-578: sin tarjetas el detalle no muestra nada (sin titulo): %+v, Elegida %q", d, m.Elegida())
	}
	ttForma(t, "CA-578", m, 200, 40, false)
}

// CA-700, CA-701 (propiedad): para tableros al azar, cada tarjeta cae en un
// solo grupo (el primero que se cumple: HECHO, CORRIENDO, DETENIDA, EN COLA,
// TE TOCA), que Modelo.Grupo dice; los grupos van en el orden del contrato
// con la cantidad de tarjetas que tienen, TE TOCA por columna de la ultima a
// la primera y los demas en el orden del tablero; ningun grupo vacio se
// muestra salvo HECHO; cada fila tiene el camino de su columna y su grupo y
// el estado de la tabla; y la caja cuenta TE TOCA, CORRIENDO y DETENIDA. Los
// nombres de los grupos no dependen de los nombres de columna que trae el
// tablero (la v2 los ponia en mayusculas). Re-expresa CA-555 y CA-518.
func TestCA700_CA701_CA555_CA518_PropiedadUnGrupoPorTarjeta(t *testing.T) {
	cols := []string{boardcmd.ColBacklog, boardcmd.ColArquitecto, boardcmd.ColTuAprobacion, boardcmd.ColTestWriter,
		boardcmd.ColWriter, boardcmd.ColReview, boardcmd.ColTuAceptacion, boardcmd.ColHecho}
	grupos := []string{"TE TOCA", "CORRIENDO", "DETENIDA", "EN COLA", "HECHO"}
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		n := 1 + r.Intn(12)
		var cards []boardcmd.Card
		grupoDe := map[string]tablero.Grupo{}
		estadoDe := map[string]string{}
		for i := 0; i < n; i++ {
			col := cols[r.Intn(len(cols))]
			slug := fmt.Sprintf("c-%02d", i)
			c := ttCard(slug, fmt.Sprintf("Tarjeta %02d", i), col, fmt.Sprintf("plain %02d", i), ttSeg("s", "s", boardcmd.SegHecho, "d"))
			caso := r.Intn(5)
			switch caso {
			case 0:
				c.Running = &boardcmd.Running{Role: "writer", Stage: "run", Step: 1, Steps: 2, StartedAt: ttAhora.Add(-time.Minute)}
			case 1:
				c.Interrupted = &boardcmd.Interrupted{Role: "writer", Stage: "run", Step: 1, Steps: 2}
				c.NeedsDecision = true
			case 2:
				c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v", Plain: fmt.Sprintf("rojo %02d", i)}
			case 3:
				c.Red = &boardcmd.Red{Source: boardcmd.RedEnvelope, ID: "e", Plain: fmt.Sprintf("rojo %02d", i), Esperado: true}
			}
			var g tablero.Grupo
			estado := c.Plain
			switch {
			case col == boardcmd.ColHecho:
				g = tablero.GrupoHecho
			case caso == 0:
				g, estado = tablero.GrupoCorriendo, "writer · 1:00"
			case caso == 1:
				g, estado = tablero.GrupoDetenida, "x se corto el trabajo del writer"
			case caso == 2:
				g, estado = tablero.GrupoDetenida, "x "+c.Red.Plain
			case col == boardcmd.ColBacklog:
				g = tablero.GrupoEnCola
			default:
				g = tablero.GrupoTeToca
			}
			grupoDe[slug], estadoDe[slug] = g, estado
			cards = append(cards, c)
		}
		tb := ttTablero(cards...)
		// los nombres que trae el tablero (no los de boardcmd.Columns)
		for i := range tb.Columns {
			tb.Columns[i].Name = "Col " + tb.Columns[i].Name
		}
		m := ttModelo(ttConfig(240, 80, false), tb)
		m.Tecla("G", ttAhora)                // la ultima fila: la linea de HECHO
		m.Tecla(tablero.TeclaEnter, ttAhora) // abre HECHO

		type seccion struct {
			titulo string
			filas  []string
		}
		fila := func(c boardcmd.Card) string {
			return c.Item.Titulo + "|" + tv3bCamino(c.Column, grupoDe[c.Slug], false) + "|" + estadoDe[c.Slug]
		}
		por := map[tablero.Grupo][]string{}
		cuenta := map[tablero.Grupo]int{}
		for k := len(tb.Columns) - 1; k >= 0; k-- { // TE TOCA: de la ultima columna a la primera
			for _, c := range tb.Columns[k].Cards {
				if grupoDe[c.Slug] == tablero.GrupoTeToca {
					por[tablero.GrupoTeToca] = append(por[tablero.GrupoTeToca], fila(c))
				}
			}
		}
		for _, col := range tb.Columns {
			for _, c := range col.Cards {
				cuenta[grupoDe[c.Slug]]++
				if g := grupoDe[c.Slug]; g != tablero.GrupoTeToca {
					por[g] = append(por[g], fila(c))
				}
			}
		}
		var quiere []seccion
		for _, g := range []tablero.Grupo{tablero.GrupoTeToca, tablero.GrupoCorriendo, tablero.GrupoDetenida, tablero.GrupoEnCola, tablero.GrupoHecho} {
			if g == tablero.GrupoHecho {
				quiere = append(quiere, seccion{fmt.Sprintf(" HECHO %d v", cuenta[g]), por[g]})
			} else if cuenta[g] > 0 {
				quiere = append(quiere, seccion{fmt.Sprintf(" %s %d", g, cuenta[g]), por[g]})
			}
		}
		esGrupo := func(l string) (string, bool) {
			s := l
			if strings.HasPrefix(s, " > ") { // la linea de HECHO elegida
				s = " " + s[3:]
			}
			for _, g := range grupos {
				if strings.HasPrefix(s, " "+g+" ") {
					return s, true
				}
			}
			return "", false
		}
		var hay []seccion
		ls := tv3bLista(t, "CA-701", m).ttTexto()
		for i := 0; i < len(ls); i++ {
			l := ls[i]
			if l == "" {
				continue
			}
			if s, ok := esGrupo(l); ok {
				hay = append(hay, seccion{titulo: s})
				continue
			}
			if len(hay) == 0 || i+1 >= len(ls) || !strings.HasPrefix(ls[i+1], "   ") || len(l) < 3 {
				t.Logf("semilla %d: la fila %q no esta bajo un grupo o no tiene segunda linea:\n%s", semilla, l, ttPantalla(m))
				return false
			}
			ti, cam, _ := tv3bPartirFila(l[3:])
			hay[len(hay)-1].filas = append(hay[len(hay)-1].filas, ti+"|"+cam+"|"+strings.TrimPrefix(ls[i+1], "   "))
			i++
		}
		if fmt.Sprint(hay) != fmt.Sprint(quiere) {
			t.Logf("semilla %d:\n hay    %v\n quiere %v\n%s", semilla, hay, quiere, ttPantalla(m))
			return false
		}
		for slug, g := range grupoDe {
			if m.Grupo(slug) != g {
				t.Logf("semilla %d: Grupo(%s) = %q, se esperaba %q", semilla, slug, m.Grupo(slug), g)
				return false
			}
		}
		caja := fmt.Sprintf("te toca %d · corriendo %d · detenida %d", cuenta[tablero.GrupoTeToca], cuenta[tablero.GrupoCorriendo], cuenta[tablero.GrupoDetenida])
		if got := tv3bCajaPaso(t, "CA-699", m).Derecha; got != caja {
			t.Logf("semilla %d: la caja cuenta %q, se esperaba %q", semilla, got, caja)
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 120}); err != nil {
		t.Fatalf("CA-700: %v", err)
	}
}

// ---------------------------------------------------------------- CA-554: la busqueda

// CA-554: / pone el foco en la lista de tarjetas desde cualquier Lugar (aca,
// la vista Hallazgos: la v2 lo probaba desde [4]), la linea de mensajes
// muestra "/<texto>▌" ("_" en ASCII) y la lista se filtra mientras se
// escribe, por slug o titulo, con los grupos y sus cantidades de lo que queda
// (HECHO sigue con su linea); enter y esc cierran la busqueda, sueltan el
// filtro y dejan elegida la tarjeta que estaba elegida. Re-expresa CA-520.
func TestCA554_CA520_BuscarYCerrar(t *testing.T) {
	for _, cerrar := range []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaEsc} {
		m := ttModeloDorado(ttConfig(120, 32, false))
		m.Tecla("H", ttAhora)
		ttSinOrdenes(t, "CA-554", "/", m.Tecla("/", ttAhora))
		if m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarBusqueda || m.Vista() != tablero.VistaTarjetas {
			t.Fatalf("CA-554: / vuelve a las tarjetas, pone el foco en la lista y abre la busqueda: foco %d, lugar %q, vista %q",
				int(m.Foco()), m.Lugar(), m.Vista())
		}
		if got := ttMensaje(m); got != "/_" {
			t.Fatalf("CA-554: con la busqueda vacia la linea de mensajes es ' /_': %q", got)
		}
		ttSinOrdenes(t, "CA-554", "escribir", ttEscribir(m, "inf"))
		if got := ttMensaje(m); got != "/inf_" {
			t.Fatalf("CA-554: la linea de mensajes muestra '/inf_': %q", got)
		}
		quiere := []string{
			" CORRIENDO 1",
			tv3bFilaAncha(120, true, false, "Convenciones de la API", "####*---"),
			"   writer · 12:41 de 45:00",
			" EN COLA 1",
			tv3bFilaAncha(120, false, false, "CI y CD de la infra", "--------"),
			"   falta el spec de la tarjeta",
			" HECHO 0 >",
		}
		got := tv3bLista(t, "CA-554", m).ttTexto()
		ttIgualesAlPrincipio(t, "CA-554", "filtrado por inf", got, quiere)
		if len(got) > len(quiere) && got[len(quiere)] != "" {
			t.Fatalf("CA-554: solo quedan las tarjetas cuyo slug o titulo tiene 'inf': %q", got)
		}
		// la elegida no quedo: la seleccion fue a la primera fila que queda
		if m.Elegida() != "infra-api" {
			t.Fatalf("CA-554: la elegida (nuc-tenancy) no queda: la seleccion va a la primera que queda, Elegida %q", m.Elegida())
		}
		m.Tecla(tablero.TeclaAbajo, ttAhora)
		if m.Elegida() != "infra-ci-cd" {
			t.Fatalf("CA-554: ↓ mueve la seleccion en la busqueda: %q", m.Elegida())
		}
		ttSinOrdenes(t, "CA-554", string(cerrar), m.Tecla(cerrar, ttAhora))
		if m.Lugar() != tablero.LugarTarjetas || ttMensaje(m) != "" {
			t.Fatalf("CA-554: %q cierra la busqueda: lugar %q, mensaje %q", cerrar, m.Lugar(), ttMensaje(m))
		}
		if m.Elegida() != "infra-ci-cd" || tv3bTituloElegido(t, "CA-554", m) != "CI y CD de la infra" {
			t.Fatalf("CA-554: %q suelta el filtro y deja elegida la tarjeta elegida: %q\n%s", cerrar, m.Elegida(), ttPantalla(m))
		}
		if !ttTiene(m, "Tenancy del nucleo") || !ttTiene(m, "Pagos caidos") {
			t.Fatalf("CA-554: sin el filtro vuelven todas las tarjetas:\n%s", ttPantalla(m))
		}
	}
}

// CA-554: el filtro no distingue mayusculas ni acentos (de los dos lados) y
// mira el slug y el titulo; en la busqueda q es texto, no salir, y ctrl-c
// sale; en modo experto la linea es " EXPERTO · /<texto>▌". La lista muestra
// los titulos (CA-701), asi que se mira por ellos.
func TestCA554_SinMayusculasNiAcentosYQEsTexto(t *testing.T) {
	seg := ttSeg("s", "s", boardcmd.SegHecho, "d")
	b := ttTablero(ttCard("pinata", "La Piñata Ácida", boardcmd.ColWriter, "p", seg), ttCard("zeta", "Zeta", boardcmd.ColWriter, "p", seg),
		ttCard("cafe", "Café con leche", boardcmd.ColWriter, "p", seg))
	for _, busca := range []string{"PIÑATA", "piñata", "pinata", "acida", "ÁCIDA", "Acída", "PINATA ACIDA"} {
		m := ttModelo(ttConfig(120, 32, false), b)
		m.Tecla("/", ttAhora)
		ttEscribir(m, busca)
		got := strings.Join(tv3bLista(t, "CA-554", m).ttTexto(), "\n")
		if !strings.Contains(got, "La Piñata Ácida") || strings.Contains(got, "Zeta") || strings.Contains(got, "Café con leche") {
			t.Fatalf("CA-554: %q encuentra la piñata y nada mas:\n%s", busca, got)
		}
	}
	m := ttModelo(ttConfig(120, 32, false), b)
	m.Tecla("/", ttAhora)
	if os := ttEscribir(m, "CAFE"); len(os) != 0 {
		t.Fatalf("CA-554: escribir no da ordenes: %#v", os)
	}
	if got := strings.Join(tv3bLista(t, "CA-554", m).ttTexto(), "\n"); !strings.Contains(got, "Café con leche") || strings.Contains(got, "Zeta") {
		t.Fatalf("CA-554: CAFE encuentra Café por el titulo:\n%s", got)
	}
	m.Tecla(tablero.TeclaBorrar, ttAhora)
	if ttMensaje(m) != "/CAF_" {
		t.Fatalf("CA-554: backspace borra en la busqueda: %q", ttMensaje(m))
	}
	if os := ttEscribir(m, "q"); len(os) != 0 || m.Lugar() != tablero.LugarBusqueda || ttMensaje(m) != "/CAFq_" {
		t.Fatalf("CA-554: q en la busqueda es texto, no salir: %#v, %q, %q", os, m.Lugar(), ttMensaje(m))
	}
	if os := m.Tecla(tablero.TeclaCtrlC, ttAhora); len(os) != 1 || os[0] != tablero.Orden(tablero.Salir{}) {
		t.Fatalf("CA-554: ctrl-c en la busqueda sale: %#v", os)
	}
	x := ttModelo(ttConfig(120, 32, false), b)
	x.Tecla("x", ttAhora)
	x.Tecla("/", ttAhora)
	ttEscribir(x, "ze")
	if got := ttMensaje(x); got != "EXPERTO · /ze_" {
		t.Fatalf("CA-554: en modo experto la linea es ' EXPERTO · /ze_': %q", got)
	}
	u := ttModelo(ttConfig(120, 32, true), b)
	u.Tecla("/", ttAhora)
	ttEscribir(u, "ze")
	if got := ttMensaje(u); got != "/ze▌" {
		t.Fatalf("CA-554: con UTF-8 el cursor es ▌: %q", got)
	}
}

// ---------------------------------------------------------------- CA-708 (re-expresa CA-556): el detalle de una tarjeta

// ttDetalleRico es el detalle de infra-api (sin running) con su spec de
// cuatro criterios (CA-2 y CA-4 sin prueba, CA-3 por comando), tres
// hallazgos (dos abiertos, uno cerrado), dos entradas de historia, un sobre
// entregable y dos eventos.
func ttDetalleRico() (boardcmd.Card, tablero.Detalle) {
	c := ttInfraAPI(false)
	ms := int64(61000)
	cerrado := ttHallazgo("f3", finding.StatusRefuted, "low", "", "refutado ya")
	d := tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
		Spec: boardcmd.SpecDetail{Path: ".hoom/specs/infra-api.md", Exists: true,
			Markdown: "# Spec\n\n## Criterios de aceptacion\n\n- CA-1: criterio uno\n- CA-2: criterio dos\n- CA-3: criterio tres\n- CA-4: criterio cuatro\n"},
		Criteria: []boardcmd.CriterionTrace{
			{ID: "CA-1", Text: "criterio uno", TracedBy: boardcmd.TracedTest},
			{ID: "CA-2", Text: "criterio dos"},
			{ID: "CA-3", Text: "criterio tres", TracedBy: boardcmd.TracedComando},
			{ID: "CA-4", Text: "criterio cuatro"},
		},
		Findings: []finding.Item{
			cerrado,
			ttHallazgo("f1", finding.StatusOpen, "high", "internal/api/x.go:12", "el router acepta todo"),
			ttHallazgo("f2", finding.StatusOpen, "medium", "a/b/c.go", "la descripcion\ncon un salto"),
		},
		Work: []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "env-h", RunID: "run-h", Role: "writer", Status: "entregable",
			Stage: "run", Step: 6, Steps: 6, StartedAt: ttAhora.Add(-time.Hour), DurationMS: &ms}},
	},
		Historia: boardcmd.Timeline{Slug: c.Slug, Entries: []boardcmd.TimelineEntry{
			{At: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC), Kind: boardcmd.KindItem, Who: "Henry Orellana", Plain: "creo la tarjeta", Summary: "item infra-api"},
			{At: time.Date(2026, 11, 8, 10, 30, 0, 0, time.UTC), Kind: boardcmd.KindCommit, Who: "writer (claude)", Plain: "escribio el codigo", Summary: "commit abc123"},
		}},
		Sobre: tablero.Sobre{EnvelopeID: "env-h", RunID: "run-h", Eventos: []runcmd.Event{ttEvento(-65, "tool", "Read x.go"), ttEvento(-5, "text", "leyendo el spec")}},
	}
	return c, d
}

// CA-708: el detalle de una tarjeta es un solo documento en el orden del
// contrato: AHORA (el grupo a la derecha y, para un rojo no esperado, su
// plain), TE TOCA (enter con el paso y su ayuda), SIGUE (la frase de Writer
// sin piloto y la linea gris del piloto), EL CAMINO (los 8 pasos, "paso 5 de
// 8" y "<- aca" en Codigo) y despues las partes B a G de la v2: la columna con
// el gasto (y el presupuesto), quien escribio y reviso, el rojo y el doctor,
// una linea por segmento de la tira, los criterios con +, x o v (✓ ✗ ⚑), los
// hallazgos de su tarea (los abiertos primero, el cerrado en gris), la
// historia y la narracion del ultimo sobre. La v2 empezaba con "Que falta?"
// y "Siguiente paso": los reemplazan AHORA y TE TOCA. Re-expresa CA-556,
// CA-521, CA-523 y CA-531.
func TestCA708_CA556_CA521_CA523_ElDocumentoDeLaTarjeta(t *testing.T) {
	c, d := ttDetalleRico()
	m := ttModelo(ttConfig(200, 70, false), ttTablero(c)) // el detalle: 120 celdas y 60 lineas interiores
	m.LeerDetalle(d)
	if m.Grupo("infra-api") != tablero.GrupoDetenida {
		t.Fatalf("CA-700: infra-api sin running y con un rojo no esperado va a DETENIDA: %q", m.Grupo("infra-api"))
	}
	got := tv3bDetalle(t, "CA-708", m).ttTexto()
	quiere := []string{
		tv3bPanDerecha(120, " AHORA", "DETENIDA"),
		" la verificacion dio rojo",
		"",
		" TE TOCA",
		" enter pedir al writer · se abre el pedido para revisarlo antes de lanzar",
		"",
		" SIGUE",
		" despues, te toca pedir la review al reviewer",
		" con el piloto seguiria sola: hoom item set infra-api --auto hasta-humano",
		"",
	}
	quiere = append(quiere, tv3bPanCamino(120, boardcmd.ColWriter, tablero.GrupoDetenida, "se detuvo")...)
	quiere = append(quiere,
		"",
		" Writer · gasto 3.88 USD de 12.25 USD · escribio claude · reviso codex",
		" rojo: la verificacion dio rojo",
		" doctor: el spec cambio despues de aprobarlo: hoom aprobar infra-api",
		"",
		"  # spec: aprobado",
		"  # pruebas: 27/27 con prueba",
		"  - verde: sin veredicto verde",
		"  . review: no hace falta todavia",
		"",
		" Spec · 4 criterios · 2 sin prueba",
		"  + criterio uno",
		"  x criterio dos",
		"  v criterio tres",
		"  x criterio cuatro",
		"",
		" Hallazgos · 2 abiertos",
		"  high x.go:12 el router acepta todo",
		"  medium c.go la descripcion con un salto",
		"  low - refutado ya",
		"",
		" Historia",
		"  2026-11-08 09:00 Henry Orellana creo la tarjeta",
		"  2026-11-08 10:30 writer (claude) escribio el codigo",
		"",
		" Narracion",
	)
	// quiere[10] es " EL CAMINO"; los pasos siguen en el orden de las
	// columnas: Pedido 11, Spec 12, Tu firma 13, Tests 14, Codigo (Writer) 15
	if quiere[15] != "  x Codigo         se detuvo"+strings.Repeat(" ", 83)+"<- aca" {
		t.Fatalf("CA-708: la linea de Codigo del camino: %q", quiere[15])
	}
	ttIgualesAlPrincipio(t, "CA-708", "el documento de infra-api", got, quiere)
	sobre := got[len(quiere)]
	if !strings.HasPrefix(sobre, "  writer · run 6/6 · 1:01") || !strings.Contains(sobre, "entregable") || !strings.HasSuffix(sobre, "ultimo evento hace 5s") {
		t.Fatalf("CA-523: sin running, la narracion empieza con el rol, el paso, la duracion y el estado del ultimo sobre y 'ultimo evento hace 5s': %q", sobre)
	}
	if got[len(quiere)+1] != "  11:58:55 tool Read x.go" || got[len(quiere)+2] != "  11:59:55 text leyendo el spec" {
		t.Fatalf("CA-523: despues, un evento por linea con su hora UTC: %q", got[len(quiere):])
	}
	// el titulo: el de la tarjeta, con el slug como parte derecha
	if k := tv3bDetalle(t, "CA-708", m); k.Titulo != "Convenciones de la API" || k.Derecha != "infra-api" {
		t.Fatalf("CA-708: el titulo del detalle es el de la tarjeta y su parte derecha el slug: %q / %q", k.Titulo, k.Derecha)
	}
}

// CA-708: con color, el rojo va en rojo, la severidad en su color (high
// rojo, medium amarillo, low gris) y un hallazgo cerrado todo en gris.
// Re-expresa CA-556.
func TestCA708_CA556_ColoresDelDocumento(t *testing.T) {
	c, d := ttDetalleRico()
	cfg := ttConfig(200, 70, false)
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(c))
	m.LeerDetalle(d)
	m.Tecla(tablero.TeclaTab, ttAhora) // el foco en el detalle: sin la barra de la seleccion en la lista
	linea := func(s string) string {
		for _, l := range m.Pantalla() {
			if strings.Contains(ttSinSGR(l), s) {
				return l
			}
		}
		t.Fatalf("CA-708: la pantalla tiene %q", s)
		return ""
	}
	if l := linea("rojo: la verificacion dio rojo"); !ttConColor(ttRojo, l, "rojo: la verificacion") {
		t.Fatalf("CA-708: 'rojo: <plain>' va en rojo: %q", l)
	}
	if l := linea("high x.go:12"); ttEstiloDe(l, "high") != "\x1b[31m" {
		t.Fatalf("CA-708: la severidad high va en rojo: %q", l)
	}
	if l := linea("medium c.go"); ttEstiloDe(l, "medium") != "\x1b[33m" {
		t.Fatalf("CA-708: la severidad medium va en amarillo: %q", l)
	}
	if l := linea("low - refutado ya"); ttEstiloDe(l, "refutado ya") != "\x1b[90m" || ttEstiloDe(l, "low") != "\x1b[90m" {
		t.Fatalf("CA-708: un hallazgo cerrado va todo en gris: %q", l)
	}
}

// CA-708, CA-531: x alterna el modo experto: la linea de mensajes empieza
// con "EXPERTO" y AHORA suma la linea gris del rojo (su reason),
// "siguiente: <next>" y una linea "missing:" por cada falta; el rojo de la
// parte B dice su reason, cada criterio lleva su id antes del texto y la
// historia dice su summary. x otra vez vuelve al modo normal. Re-expresa
// CA-556.
func TestCA708_CA556_CA531_ModoExperto(t *testing.T) {
	c, d := ttDetalleRico()
	c.Missing = []string{"veredicto verde de la tarjeta", "la review"}
	d.Detalle.Card = c
	m := ttModelo(ttConfig(200, 70, false), ttTablero(c))
	m.LeerDetalle(d)
	ttSinOrdenes(t, "CA-708", "x", m.Tecla("x", ttAhora))
	if got := ttMensaje(m); got != "EXPERTO" {
		t.Fatalf("CA-531: en modo experto, sin mensaje, la linea de mensajes es ' EXPERTO': %q", got)
	}
	got := tv3bDetalle(t, "CA-708", m).ttTexto()
	ttIgualesAlPrincipio(t, "CA-708", "modo experto", got, []string{
		tv3bPanDerecha(120, " AHORA", "DETENIDA"),
		" la verificacion dio rojo",
		" test: FAIL",
		" siguiente: hoom agent --role writer --task infra-api",
		" missing: veredicto verde de la tarjeta",
		" missing: la review",
		"",
		" TE TOCA",
	})
	todo := strings.Join(got, "\n")
	for _, s := range []string{"\n rojo: test: FAIL\n", "  + CA-1 criterio uno", "  x CA-2 criterio dos", "  v CA-3 criterio tres",
		"  2026-11-08 09:00 Henry Orellana item infra-api"} {
		if !strings.Contains(todo, s) {
			t.Fatalf("CA-708: en modo experto el detalle dice %q:\n%s", s, todo)
		}
	}
	m.Tecla("w", ttAhora) // un mensaje en modo experto
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("a", ttAhora) // sin la accion: un aviso
	if got := ttMensaje(m); !strings.HasPrefix(got, "EXPERTO · en Writer no hay ") {
		t.Fatalf("CA-531: en modo experto un mensaje va despues de 'EXPERTO · ': %q", got)
	}
	m.Tecla("x", ttAhora)
	if got := ttMensaje(m); strings.HasPrefix(got, "EXPERTO") {
		t.Fatalf("CA-531: x otra vez vuelve al modo normal: %q", got)
	}
	if todo := strings.Join(tv3bDetalle(t, "CA-708", m).ttTexto(), "\n"); strings.Contains(todo, "missing:") || strings.Contains(todo, "test: FAIL") {
		t.Fatalf("CA-708: en modo normal no hay missing ni reason:\n%s", todo)
	}
}

// CA-708: las variantes del documento: una tarjeta de TE TOCA (AHORA es su
// plain, sin linea gris; "te toca a vos" en el camino); sin presupuesto, sin
// costo y sin writer ni reviewer, sin rojo ni doctor; un criterio ("1
// criterio") y un hallazgo abierto (" · 1 abierto"); sin spec (" sin spec:
// <ruta>"); sin hallazgos, sin historia y sin sobres; antes del detalle ("
// leyendo la tarjeta...") y con un error (" no se pudo leer la tarjeta:
// <Err>"). Re-expresa CA-556.
func TestCA708_CA556_Variantes(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	m := ttModelo(ttConfig(200, 70, false), ttTablero(c))
	got := tv3bDetalle(t, "CA-708", m).ttTexto()
	quiere := []string{
		tv3bPanDerecha(120, " AHORA", "TE TOCA"),
		" el plain de la tarjeta",
		"",
		" TE TOCA",
		" enter pedir al writer · se abre el pedido para revisarlo antes de lanzar",
		"",
		" SIGUE",
		" despues, te toca pedir la review al reviewer",
		" con el piloto seguiria sola: hoom item set tarjeta --auto hasta-humano",
		"",
	}
	quiere = append(quiere, tv3bPanCamino(120, boardcmd.ColWriter, tablero.GrupoTeToca, "te toca a vos")...)
	quiere = append(quiere,
		"",
		" Writer · gasto sin dato de costo",
		"",
		"  # spec: aprobado",
		"  - verde: sin verde",
		"",
		" leyendo la tarjeta...",
	)
	ttIgualesAlPrincipio(t, "CA-708", "antes del detalle", got, quiere)
	d := tablero.Detalle{Slug: "tarjeta", Detalle: boardcmd.Detail{Card: c,
		Spec:     boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "- CA-1: solo uno\n"},
		Criteria: []boardcmd.CriterionTrace{{ID: "CA-1", Text: "solo uno", TracedBy: boardcmd.TracedTest}},
		Findings: []finding.Item{ttHallazgo("f1", finding.StatusOpen, "low", "", "sin archivo")}}}
	m.LeerDetalle(d)
	todo := strings.Join(tv3bDetalle(t, "CA-708", m).ttTexto(), "\n")
	for _, s := range []string{" Spec · 1 criterio · 0 sin prueba\n  + solo uno", " Hallazgos · 1 abierto\n  low - sin archivo",
		" Historia\n  sin historia todavia", " Narracion\n  sin sobres todavia"} {
		if !strings.Contains(todo, s) {
			t.Fatalf("CA-708: el documento dice %q:\n%s", s, todo)
		}
	}
	d.Detalle.Findings = nil
	d.Detalle.Spec = boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md"}
	d.Detalle.Criteria = nil
	m.LeerDetalle(d)
	todo = strings.Join(tv3bDetalle(t, "CA-708", m).ttTexto(), "\n")
	for _, s := range []string{" sin spec: .hoom/specs/tarjeta.md", " Hallazgos\n  sin hallazgos"} {
		if !strings.Contains(todo, s) {
			t.Fatalf("CA-708: el documento dice %q:\n%s", s, todo)
		}
	}
	if strings.Contains(todo, " Spec · ") {
		t.Fatalf("CA-708: sin spec no hay ' Spec · ':\n%s", todo)
	}
	m.LeerDetalle(tablero.Detalle{Slug: "tarjeta", Err: errors.New("disco roto")})
	todo = strings.Join(tv3bDetalle(t, "CA-708", m).ttTexto(), "\n")
	if !strings.Contains(todo, " no se pudo leer la tarjeta: disco roto") || strings.Contains(todo, " Historia") {
		t.Fatalf("CA-708: con Detalle.Err, el detalle lo dice y no muestra nada viejo:\n%s", todo)
	}
	// un detalle de otra tarjeta se ignora
	m.LeerDetalle(tablero.Detalle{Slug: "otra", Detalle: boardcmd.Detail{Spec: boardcmd.SpecDetail{Exists: true, Markdown: "x"}}})
	if !strings.Contains(strings.Join(tv3bDetalle(t, "CA-708", m).ttTexto(), "\n"), "no se pudo leer la tarjeta") {
		t.Fatalf("CA-708: un Detalle de otra tarjeta se ignora")
	}
}

// CA-708, CA-704, CA-705: TE TOCA en cada caso de la regla del paso (doc.go,
// "The step of a card"): "enter <etiqueta> · <ayuda>" con un paso, o "nada:
// <texto>" sin el; y enter sobre la tarjeta abre el modal del paso sin
// lanzar, o, sin paso, la linea de mensajes dice ese texto y nada se abre. La
// v2 lo decia en la linea "Siguiente paso" con la tecla de la accion.
// Re-expresa CA-556 y CA-566.
func TestCA708_CA704_CA705_CA556_CA566_TeTocaEnCadaCaso(t *testing.T) {
	writer := ttAccionWriter()
	writerOff := ttAccionWriter()
	writerOff.Enabled, writerOff.Why = false, "se agoto el presupuesto de la tarjeta"
	conRunning := ttTarjetaCon(writer)
	conRunning.Running = &boardcmd.Running{EnvelopeID: "e", Role: "writer", Stage: "run", StartedAt: ttAhora}
	conInterr := ttTarjetaCon(writer)
	conInterr.Interrupted, conInterr.NeedsDecision = &boardcmd.Interrupted{EnvelopeID: "e", Role: "writer", Stage: "run", Step: 1, Steps: 5}, true
	enHecho := ttViejo(1)
	enHecho.Actions = []boardcmd.Action{writer}
	enAcept := ttCard("acepta", "Acepta", boardcmd.ColTuAceptacion, "lista", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	enAcept.Actions = []boardcmd.Action{ttAccionIntegrar()}
	aprobSin := ttCard("sin-firma", "Sin firma", boardcmd.ColTuAprobacion, "falta", ttSeg("s", "s", boardcmd.SegFalta, "d"))
	casos := []struct {
		nombre string
		card   boardcmd.Card
		paso   bool // con paso: "enter ..."; sin paso: "nada: ..."
		quiere string
	}{
		{"en Hecho", enHecho, false, "la tarjeta ya esta en Hecho"},
		{"con running", conRunning, false, "el writer esta trabajando: espera a que termine"},
		{"interrumpida", conInterr, false, "el trabajo se corto: R reanuda o pide de nuevo"},
		{"en Tu aprobacion", ttNucTenancy(), true, "aprobar spec · hoom no firma por vos"},
		{"en Tu aceptacion", enAcept, true, "integrar · hoom no acepta por vos"},
		{"en Tu aprobacion sin la accion", aprobSin, false, "en Tu aprobacion no hay aprobar el spec"},
		{"sin acciones", ttTarjetaCon(), false, "en Writer no hay nada que hacer ahora: el plain de la tarjeta"},
		{"primera de rol deshabilitada", ttTarjetaCon(writerOff), false, "se agoto el presupuesto de la tarjeta"},
		{"primera de rol", ttTarjetaCon(writer), true, "pedir al writer · se abre el pedido para revisarlo antes de lanzar"},
		{"primera guardar", ttTarjetaCon(ttAccionGuardar(true, "", "a.go"), writer), true, "guardar · guarda el trabajo de la tarjeta en su rama"},
		{"primera sin tecla", ttTarjetaCon(ttRelanzar(), writer), false, "en Writer no hay nada que hacer ahora: el plain de la tarjeta"},
		{"reanudar", ttPagosCaidos(), false, "el trabajo se corto: R reanuda o pide de nuevo"},
	}
	for _, c := range casos {
		m := ttModelo(ttConfig(200, 50, false), ttTablero(c.card))
		if c.nombre == "en Hecho" {
			tv3bIrAHecho(t, m)
			m.Tecla(tablero.TeclaEnter, ttAhora) // abre HECHO
			m.Tecla("j", ttAhora)
		}
		if m.Elegida() != c.card.Slug {
			t.Fatalf("CA-708 (%s): la elegida es %s: %q\n%s", c.nombre, c.card.Slug, m.Elegida(), ttPantalla(m))
		}
		got := tv3bDetalle(t, "CA-708", m).ttTexto()
		i := tv3bPanIndice(got, " TE TOCA")
		linea := " nada: " + c.quiere
		if c.paso {
			linea = " enter " + c.quiere
		}
		if i < 0 || i+1 >= len(got) || got[i+1] != linea {
			t.Fatalf("CA-708 (%s): debajo de ' TE TOCA' el detalle dice %q:\n%s", c.nombre, linea, strings.Join(got, "\n"))
		}
		ords := m.Tecla(tablero.TeclaEnter, ttAhora)
		if len(ttLanzares(ords)) != 0 {
			t.Fatalf("CA-705 (%s): enter nunca lanza: %#v", c.nombre, ords)
		}
		if c.paso && m.Modal() == tablero.SinModal {
			t.Fatalf("CA-705 (%s): enter abre el modal del paso", c.nombre)
		}
		if !c.paso {
			if m.Modal() != tablero.SinModal {
				t.Fatalf("CA-705 (%s): sin paso enter no abre nada: %q", c.nombre, m.Modal())
			}
			ttDiceMensaje(t, "CA-705 ("+c.nombre+")", "enter sin paso", m, false, c.quiere)
		}
	}
}

// CA-708: las lineas mas anchas que el detalle se parten en los espacios
// (tantas palabras como entren; una palabra mas larga que la linea se
// corta), cada continuacion con los espacios del comienzo de la linea que
// sigue; las de los eventos de la narracion se cortan. La frase de AHORA de
// una tarjeta de TE TOCA es su plain (la v2: "Que falta?"). Re-expresa
// CA-556.
func TestCA708_CA556_LasLineasLargasSeParten(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	c.Plain = "uno dos tres cuatro cinco seis siete ocho nueve diez once doce trece catorce quince dieciseis"
	c.Meter = []boardcmd.Segment{ttSeg("s", "spec", boardcmd.SegHecho, "un detalle largo de segmento que no entra en una sola linea de cincuenta")}
	m := ttModelo(ttConfig(100, 60, false), ttTablero(c)) // el detalle: 60 celdas, 58 interiores
	ev := strings.Repeat("palabra ", 20)
	m.LeerDetalle(tablero.Detalle{Slug: "tarjeta", Detalle: boardcmd.Detail{Card: c,
		Work: []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "e", Role: "writer", Status: "entregable", Stage: "run", StartedAt: ttAhora}}},
		Sobre: tablero.Sobre{EnvelopeID: "e", Eventos: []runcmd.Event{ttEvento(-1, "text", ev)}}})
	got := tv3bDetalle(t, "CA-708", m).ttTexto()
	if !strings.HasPrefix(got[0], " AHORA ") {
		t.Fatalf("CA-708: el detalle empieza con AHORA: %q", got[0])
	}
	// el plain: la primera linea corta en un espacio antes de las 58 celdas;
	// las que siguen empiezan con el espacio del comienzo; ninguna palabra se
	// pierde ni se corta
	var palabras []string
	i := 1
	for ; i < len(got) && got[i] != ""; i++ {
		if !strings.HasPrefix(got[i], " ") || strings.HasPrefix(got[i], "  ") {
			t.Fatalf("CA-708: cada linea del plain (y cada continuacion) empieza con el espacio del comienzo: %q", got[i])
		}
		if ttCeldas(got[i]) > 58 || strings.HasSuffix(got[i], " ") {
			t.Fatalf("CA-708: ninguna linea pasa de las 58 celdas ni se parte despues de un espacio: %q", got[i])
		}
		palabras = append(palabras, strings.Fields(got[i])...)
	}
	if strings.Join(palabras, " ") != c.Plain || i < 3 {
		t.Fatalf("CA-708: el plain partido tiene todas sus palabras, en orden, en mas de una linea: %q", got[:i])
	}
	todo := strings.Join(got, "\n")
	if !strings.Contains(todo, "  # spec: un detalle largo de segmento que no entra en una\n  sola linea de cincuenta") {
		t.Fatalf("CA-708: la continuacion lleva los espacios del comienzo (dos en la tira):\n%s", todo)
	}
	var evento string
	for _, l := range got {
		if strings.HasPrefix(l, "  11:59:59 text ") {
			evento = l
		}
	}
	if evento == "" || ttCeldas(evento) != 58 || !strings.HasSuffix(evento, "~") {
		t.Fatalf("CA-708: la linea de un evento se corta al ancho del detalle (58 celdas, con ~): %q\n%s", evento, todo)
	}
}

// ---------------------------------------------------------------- CA-557: desplazar el detalle, ]c [c, e

// ttTarjetaLarga es infra-api con el detalle rico y veinte hallazgos mas,
// para que el detalle se pueda desplazar mas alla de los criterios.
func ttTarjetaLarga() (boardcmd.Card, tablero.Detalle) {
	c, d := ttDetalleRico()
	for i := 0; i < 20; i++ {
		d.Detalle.Findings = append(d.Detalle.Findings, ttHallazgo(fmt.Sprintf("x%02d", i), finding.StatusOpen, "low", "", fmt.Sprintf("relleno %02d", i)))
	}
	return c, d
}

// CA-557: con el foco en el detalle de una tarjeta (tab: la v2 lo pasaba a
// [0] con enter), ↓ j ↑ k desplazan una linea, pgdn pgup su alto interior, g
// y G el principio y el final (el final deja la ultima linea abajo de todo);
// fuera de esas puntas no se pasa. Re-expresa CA-522.
func TestCA557_CA522_DesplazarCero(t *testing.T) {
	c, d := ttTarjetaLarga()
	m := ttModelo(ttConfig(80, 24, false), ttTablero(c))
	m.LeerDetalle(d)
	ttSinOrdenes(t, "CA-557", "tab", m.Tecla(tablero.TeclaTab, ttAhora))
	vista := func() []string { return tv3bDetalle(t, "CA-557", m).ttTexto() }
	todo := vista() // la vista de arriba, con el foco en el detalle
	if !strings.HasPrefix(todo[0], " AHORA ") {
		t.Fatalf("CA-557: el detalle empieza en su primera linea: %q", todo)
	}
	alto := len(todo)
	if alto != 13 {
		t.Fatalf("CA-696: a 80x24, con el foco en el detalle (la lista plegada), el detalle tiene 13 lineas interiores: %d", alto)
	}
	m.Tecla("j", ttAhora)
	if v := vista(); v[0] != todo[1] {
		t.Fatalf("CA-557: j baja una linea: %q", v[0])
	}
	m.Tecla(tablero.TeclaAbajo, ttAhora)
	m.Tecla("k", ttAhora)
	if v := vista(); v[0] != todo[1] {
		t.Fatalf("CA-557: ↓ y k: una linea abajo de la primera: %q", v[0])
	}
	m.Tecla(tablero.TeclaArriba, ttAhora)
	m.Tecla(tablero.TeclaArriba, ttAhora)
	if v := vista(); v[0] != todo[0] {
		t.Fatalf("CA-557: ↑ en la primera linea no sube mas: %q", v[0])
	}
	// pgdn baja exactamente el alto interior: lo mismo que alto veces j
	m.Tecla(tablero.TeclaPgAbajo, ttAhora)
	pag := vista()
	m.Tecla("g", ttAhora)
	for i := 0; i < alto; i++ {
		m.Tecla("j", ttAhora)
	}
	if v := vista(); strings.Join(v, "\n") != strings.Join(pag, "\n") || pag[0] == todo[0] {
		t.Fatalf("CA-557: pgdn baja el alto interior (%d lineas), como %d veces j:\n pgdn %q\n j    %q", alto, alto, pag, v)
	}
	m.Tecla(tablero.TeclaPgArriba, ttAhora)
	if v := vista(); strings.Join(v, "\n") != strings.Join(todo, "\n") {
		t.Fatalf("CA-557: pgup sube una pagina (al principio): %q", v)
	}
	m.Tecla("G", ttAhora)
	fin := vista()
	if fin[len(fin)-1] != "  11:59:55 text leyendo el spec" {
		t.Fatalf("CA-557: G deja la ultima linea del documento abajo de todo: %q", fin)
	}
	m.Tecla("j", ttAhora)
	m.Tecla(tablero.TeclaPgAbajo, ttAhora)
	if v := vista(); strings.Join(v, "\n") != strings.Join(fin, "\n") {
		t.Fatalf("CA-557: en el final j y pgdn no bajan mas: %q", v)
	}
	m.Tecla("g", ttAhora)
	if v := vista(); v[0] != todo[0] {
		t.Fatalf("CA-557: g vuelve al principio: %q", v[0])
	}
}

// CA-557: ]c y [c llevan la vista al siguiente y al anterior criterio con x
// (la linea queda primera en la vista), y sin otro la linea de mensajes dice
// "no hay mas criterios sin prueba"; e ordena AbrirEditor con el spec y la
// linea del markdown del primer criterio a la vista, o 1 sin ninguno.
func TestCA557_SaltosEntreCriteriosYEditor(t *testing.T) {
	c, d := ttTarjetaLarga()
	m := ttModelo(ttConfig(80, 24, false), ttTablero(c))
	m.LeerDetalle(d)
	m.Tecla(tablero.TeclaTab, ttAhora)
	vista := func() []string { return tv3bDetalle(t, "CA-557", m).ttTexto() }
	abrir := func(que string, linea int) {
		t.Helper()
		ords := m.Tecla("e", ttAhora)
		if len(ords) != 1 {
			t.Fatalf("CA-557: e (%s) da una orden AbrirEditor: %#v", que, ords)
		}
		a, ok := ords[0].(tablero.AbrirEditor)
		if !ok || a.Archivo != ".hoom/specs/infra-api.md" || a.Linea != linea {
			t.Fatalf("CA-557: e (%s) abre el spec en la linea %d: %#v", que, linea, ords[0])
		}
	}
	// arriba: a 80x24, con el foco en el detalle, el detalle tiene 13 lineas
	// de 78 celdas; en la forma angosta AHORA, TE TOCA, SIGUE y EL CAMINO son
	// una linea cada una, y la parte D (los criterios) empieza en la linea 15:
	// ningun criterio a la vista, la linea 1
	abrir("arriba", 1)
	ttSinOrdenes(t, "CA-557", "]c", ttEscribir(m, "]c"))
	if v := vista(); v[0] != "  x criterio dos" {
		t.Fatalf("CA-557: ]c deja el siguiente criterio sin prueba primero en la vista: %q", v[0])
	}
	abrir("en CA-2", 6)
	ttEscribir(m, "]c")
	if v := vista(); v[0] != "  x criterio cuatro" {
		t.Fatalf("CA-557: ]c otra vez, CA-4 (salta el de comando): %q", v[0])
	}
	abrir("en CA-4", 8)
	ttEscribir(m, "]c")
	ttDiceMensaje(t, "CA-557", "]c sin otro", m, false, "no hay mas criterios sin prueba")
	if v := vista(); v[0] != "  x criterio cuatro" {
		t.Fatalf("CA-557: sin otro, la vista no se mueve: %q", v[0])
	}
	ttEscribir(m, "[c")
	if v := vista(); v[0] != "  x criterio dos" {
		t.Fatalf("CA-557: [c vuelve al anterior sin prueba: %q", v[0])
	}
	ttEscribir(m, "[c")
	ttDiceMensaje(t, "CA-557", "[c sin otro", m, false, "no hay mas criterios sin prueba")
	m.Tecla("G", ttAhora)
	abrir("en el final, ningun criterio a la vista", 1)
	// e sin spec: la linea de mensajes lo dice, sin orden
	d.Detalle.Spec = boardcmd.SpecDetail{Path: ".hoom/specs/infra-api.md"}
	d.Detalle.Criteria = nil
	m.LeerDetalle(d)
	ttSinOrdenes(t, "CA-557", "e sin spec", m.Tecla("e", ttAhora))
	ttDiceMensaje(t, "CA-557", "e sin spec", m, false, "sin spec: .hoom/specs/infra-api.md")
}

// CA-557: e en el detalle de una tarjeta suma al Registro "Abrir en el
// editor · <archivo>:<linea>" con el comando del editor (CA-563).
func TestCA557_CA563_EditorAlRegistro(t *testing.T) {
	c, d := ttDetalleRico()
	cfg := ttConfig(120, 32, false)
	cfg.Editor = "nvim"
	m := ttModelo(cfg, ttTablero(c))
	m.LeerDetalle(d)
	m.Tecla(tablero.TeclaTab, ttAhora)
	m.Tecla("G", ttAhora)
	m.Tecla("g", ttAhora)
	ords := m.Tecla("e", ttAhora.Add(90*time.Second))
	if len(ords) != 1 {
		t.Fatalf("CA-557: e da una orden: %#v", ords)
	}
	r := m.Registro()
	if len(r) != 1 {
		t.Fatalf("CA-563: e suma una entrada al Registro: %#v", r)
	}
	a := ords[0].(tablero.AbrirEditor)
	if r[0].Texto != fmt.Sprintf("Abrir en el editor · %s:%d", a.Archivo, a.Linea) {
		t.Fatalf("CA-563: la entrada del editor es 'Abrir en el editor · <archivo>:<linea>': %q", r[0].Texto)
	}
	if quiere := "nvim " + strings.Join(tablero.ArgumentosEditor("nvim", a.Archivo, a.Linea), " "); r[0].Comando != quiere {
		t.Fatalf("CA-563: su segunda linea es el comando del editor %q: %q", quiere, r[0].Comando)
	}
	if !r[0].Hora.Equal(ttAhora.Add(90 * time.Second)) {
		t.Fatalf("CA-563: la hora de la entrada es el reloj del modelo al sumarla: %v", r[0].Hora)
	}
}

// ---------------------------------------------------------------- CA-711 (re-expresa CA-558): CORRIENDO y lo que corre

// CA-711: el grupo CORRIENDO (la v2: [3]) tiene una fila por tarjeta con
// running, con "<rol> · <tiempo> de <limite>" en su segunda linea (o, entre
// 60 y 99 columnas, en la misma linea despues del camino), y la caja lo
// cuenta; una tarjeta interrumpida va a DETENIDA con "x se corto el trabajo
// del <rol>" en rojo (la v2 la ponia en [3] como "interrumpido"). Sin nada
// corriendo no hay grupo CORRIENDO y la caja dice "corriendo 0" (la v2: "nada
// corriendo"). Re-expresa CA-558 y CA-519.
func TestCA711_CA558_CA519_FilasDeCorriendo(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	lista := tv3bLista(t, "CA-711", m)
	if i := tv3bFilaDe(lista, "Convenciones de la API"); i < 1 || lista.ttTexto()[i-1] != " CORRIENDO 1" {
		t.Fatalf("CA-711: infra-api es la fila de CORRIENDO 1:\n%s", strings.Join(lista.ttTexto(), "\n"))
	}
	if got := tv3bSegunda(lista, "Convenciones de la API"); got != "   writer · 12:41 de 45:00" {
		t.Fatalf("CA-711: la segunda linea de lo que corre es '<rol> · <tiempo> de <limite>': %q", got)
	}
	if i := tv3bFilaDe(lista, "Pagos caidos"); i < 1 || lista.ttTexto()[i-1] != " DETENIDA 1" {
		t.Fatalf("CA-711: la interrumpida va a DETENIDA:\n%s", strings.Join(lista.ttTexto(), "\n"))
	}
	if got := tv3bSegunda(lista, "Pagos caidos"); got != "   x se corto el trabajo del writer" {
		t.Fatalf("CA-711: la interrumpida dice 'x se corto el trabajo del <rol>': %q", got)
	}
	if got := tv3bCajaPaso(t, "CA-711", m).Derecha; got != "te toca 2 · corriendo 1 · detenida 1" {
		t.Fatalf("CA-699: la caja cuenta lo que corre: %q", got)
	}
	// a 80 columnas (angosta) el estado va en la misma linea, despues del
	// camino; el titulo se rellena al mas ancho de la lista (24)
	a := ttModeloDorado(ttConfig(80, 24, false))
	la := tv3bLista(t, "CA-711", a)
	if i := tv3bFilaDe(la, "Convenciones de la API"); i < 0 || la.ttTexto()[i] != "   Convenciones de la API   ####*--- writer · 12:41 de 45:00" {
		t.Fatalf("CA-711: a 80x24 la fila es una linea con el estado despues del camino:\n%s", strings.Join(la.ttTexto(), "\n"))
	}
	// el reloj mueve el tiempo
	m.Tic(ttAhora.Add(3 * time.Second))
	if got := tv3bSegunda(tv3bLista(t, "CA-711", m), "Convenciones de la API"); got != "   writer · 12:44 de 45:00" {
		t.Fatalf("CA-711: Tic mueve el tiempo corrido: %q", got)
	}
	// una hora o mas: h:mm:ss; rol vacio: agente
	c := ttCard("larga", "Larga", boardcmd.ColWriter, "p", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	c.Running = &boardcmd.Running{Stage: "verify", StartedAt: ttAhora.Add(-(time.Hour + 2*time.Minute + 3*time.Second)), TimeoutS: 5400}
	l := ttModelo(ttConfig(120, 32, false), ttTablero(c))
	if got := tv3bSegunda(tv3bLista(t, "CA-711", l), "Larga"); got != "   agente · 1:02:03 de 1:30:00" {
		t.Fatalf("CA-711: con una hora o mas h:mm:ss, y rol vacio 'agente': %q", got)
	}
	// vacio
	v := ttModelo(ttConfig(120, 32, false), ttTablero(ttBacklog("uno", "Uno")))
	for _, x := range tv3bLista(t, "CA-711", v).ttTexto() {
		if strings.HasPrefix(x, " CORRIENDO") {
			t.Fatalf("CA-700: sin nada corriendo no hay grupo CORRIENDO: %q", x)
		}
	}
	if got := tv3bCajaPaso(t, "CA-711", v).Derecha; got != "te toca 0 · corriendo 0 · detenida 0" {
		t.Fatalf("CA-699: sin nada corriendo la caja dice 'corriendo 0': %q", got)
	}
	// con color, la interrumpida en rojo y lo que corre en cian
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	r := ttModeloDorado(cfg)
	rojo, cian := false, false
	for _, x := range r.Pantalla() {
		if strings.Contains(ttSinSGR(x), "x se corto el trabajo del writer") {
			rojo = ttConColor(ttRojo, x, "x se corto el trabajo del writer")
		}
		if strings.Contains(ttSinSGR(x), "writer · 12:41 de 45:00") {
			cian = ttEstiloDe(x, "writer · 12:41 de 45:00") == "\x1b[36m"
		}
	}
	if !rojo || !cian {
		t.Fatalf("CA-711: la interrumpida va en rojo (%v) y lo que corre en cian (%v):\n%s", rojo, cian, strings.Join(r.Pantalla(), "\n"))
	}
}

// CA-711, CA-709: el detalle de lo que corre (la v2: [0] de [3]) es el
// detalle de su tarjeta: AHORA dice la frase de su rol, "<provider> · empezo
// <HH:MM> · <paso>" y la barra del tiempo; TE TOCA, que el rol esta
// trabajando; el camino, "<rol>, ahora". Su narracion (la parte G) empieza
// con la linea del rol, el paso y el tiempo (y "ultimo evento hace <n>s" con
// eventos), un evento por linea, y "no se pudo leer el sobre" con Sobre.Err;
// antes del detalle, "leyendo la tarjeta...". De una interrumpida, AHORA
// dice donde se corto. Sin limite, sin barra. Re-expresa CA-558, CA-519 y
// CA-523.
func TestCA711_CA709_CA558_CA519_CA523_ElDetalleDeLoQueCorre(t *testing.T) {
	m := ttModeloDorado(ttConfig(200, 70, false))
	tv3bIrA(t, m, "infra-api")
	got := tv3bDetalle(t, "CA-711", m).ttTexto()
	quiere := []string{
		tv3bPanDerecha(120, " AHORA", "CORRIENDO"),
		" El writer esta escribiendo el codigo.",
		" claude · empezo 11:47 · run 3/6",
		" tiempo #####--------------- 12:41 de 45:00",
		"",
		" TE TOCA",
		" nada: el writer esta trabajando: espera a que termine",
		"",
		" SIGUE",
		" despues, te toca pedir la review al reviewer",
		" con el piloto seguiria sola: hoom item set infra-api --auto hasta-humano",
		"",
	}
	quiere = append(quiere, tv3bPanCamino(120, boardcmd.ColWriter, tablero.GrupoCorriendo, "writer, ahora")...)
	quiere = append(quiere,
		"",
		" Writer · gasto 3.88 USD de 12.25 USD · escribio claude · reviso codex",
		" rojo: la verificacion dio rojo",
		" doctor: el spec cambio despues de aprobarlo: hoom aprobar infra-api",
		"",
		"  # spec: aprobado",
		"  # pruebas: 27/27 con prueba",
		"  - verde: sin veredicto verde",
		"  . review: no hace falta todavia",
		"",
		" leyendo la tarjeta...",
	)
	ttIgualesAlPrincipio(t, "CA-709", "antes del detalle", got, quiere)
	narracion := func() []string {
		t.Helper()
		g := tv3bDetalle(t, "CA-711", m).ttTexto()
		i := tv3bPanIndice(g, " Narracion")
		if i < 0 {
			t.Fatalf("CA-711: con el Detalle el documento tiene ' Narracion':\n%s", strings.Join(g, "\n"))
		}
		return g[i+1:]
	}
	m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{EnvelopeID: "env-api"}})
	if n := narracion(); !strings.HasPrefix(n[0], "  writer · run 3/6 · 12:41 de 45:00") {
		t.Fatalf("CA-711: con running la narracion empieza con el rol, el paso y el tiempo con su limite: %q", n)
	}
	m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{EnvelopeID: "env-api",
		Eventos: []runcmd.Event{ttEvento(-3, "text", "uno\ndos"), ttEvento(-2, "tool", "Bash go test")}}})
	ttIgualesAlPrincipio(t, "CA-711", "con eventos", narracion(), []string{
		"  writer · run 3/6 · 12:41 de 45:00 · ultimo evento hace 2s",
		"  11:59:57 text uno dos",
		"  11:59:58 tool Bash go test",
	})
	m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{Err: errors.New("permiso denegado")}})
	if todo := strings.Join(tv3bDetalle(t, "CA-711", m).ttTexto(), "\n"); !strings.Contains(todo, "  no se pudo leer el sobre: permiso denegado") {
		t.Fatalf("CA-711: un Sobre con Err lo dice: %s", todo)
	}
	// la interrumpida
	tv3bIrA(t, m, "pagos-caidos")
	ttIgualesAlPrincipio(t, "CA-709", "la interrumpida", tv3bDetalle(t, "CA-709", m).ttTexto(), []string{
		tv3bPanDerecha(120, " AHORA", "DETENIDA"),
		" El trabajo del writer se corto en el paso 3 de 5.",
		" ya no corre: R lo reanuda",
		"",
	})
	// sin provider ni limite: la linea gris sin el provider, sin barra; la
	// narracion sin limite ni ultimo evento
	c := ttCard("sola", "Sola", boardcmd.ColWriter, "p", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	c.Running = &boardcmd.Running{Role: "writer", Stage: "spec", Step: 1, Steps: 7, StartedAt: ttAhora.Add(-(9*time.Minute + 5*time.Second))}
	s := ttModelo(ttConfig(200, 70, false), ttTablero(c))
	ttIgualesAlPrincipio(t, "CA-709", "sin provider ni limite", tv3bDetalle(t, "CA-709", s).ttTexto(), []string{
		tv3bPanDerecha(120, " AHORA", "CORRIENDO"),
		" El writer esta escribiendo el codigo.",
		" empezo 11:50 · spec 1/7",
		"",
		" TE TOCA",
	})
	if k := tv3bDetalle(t, "CA-708", s); k.Titulo != "Sola" || k.Derecha != "sola" {
		t.Fatalf("CA-708: el titulo del detalle es el de la tarjeta y su parte derecha el slug: %q / %q", k.Titulo, k.Derecha)
	}
	s.LeerDetalle(tablero.Detalle{Slug: "sola", Sobre: tablero.Sobre{EnvelopeID: "e"}})
	g := tv3bDetalle(t, "CA-711", s).ttTexto()
	if i := tv3bPanIndice(g, " Narracion"); i < 0 || i+1 >= len(g) || g[i+1] != "  writer · spec 1/7 · 9:05" {
		t.Fatalf("CA-711: sin limite ni eventos la narracion dice '  writer · spec 1/7 · 9:05':\n%s", strings.Join(g, "\n"))
	}
}

// ttEventos son n eventos "evento <i>" de los ultimos n segundos.
func ttEventos(n int) []runcmd.Event {
	var out []runcmd.Event
	for i := 0; i < n; i++ {
		out = append(out, ttEvento(i-n, "text", fmt.Sprintf("evento %02d", i)))
	}
	return out
}

// CA-711: el detalle de una tarjeta con running empieza al final y lo sigue
// mientras llegan eventos, salvo que la persona haya subido; G lo vuelve al
// final y desde ahi lo sigue de nuevo. (A 120x32: el detalle no cambia de
// alto al pasarle el foco, como si en la forma angosta.) Re-expresa CA-558.
func TestCA711_CA558_SigueElFinal(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false)) // el detalle: 22 lineas interiores
	tv3bIrA(t, m, "infra-api")
	leer := func(n int) {
		m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{EnvelopeID: "env-api", Eventos: ttEventos(n)}})
	}
	ultima := func() string { v := tv3bDetalle(t, "CA-711", m).ttTexto(); return v[len(v)-1] }
	leer(30)
	if !strings.HasSuffix(ultima(), "evento 29") {
		t.Fatalf("CA-711: empieza al final: la ultima linea es el ultimo evento: %q", ultima())
	}
	leer(31)
	if !strings.HasSuffix(ultima(), "evento 30") {
		t.Fatalf("CA-711: sigue el final mientras llegan eventos: %q", ultima())
	}
	m.Tecla(tablero.TeclaTab, ttAhora)
	if m.Lugar() != tablero.LugarVerTarjeta {
		t.Fatalf("CA-706: tab pasa el foco al detalle de lo que corre: %q", m.Lugar())
	}
	if !strings.HasSuffix(ultima(), "evento 30") {
		t.Fatalf("CA-711: pasarle el foco no mueve la vista: %q", ultima())
	}
	m.Tecla("k", ttAhora)
	if !strings.HasSuffix(ultima(), "evento 29") {
		t.Fatalf("CA-557: k sube una linea: %q", ultima())
	}
	leer(32)
	if !strings.HasSuffix(ultima(), "evento 29") {
		t.Fatalf("CA-711: si la persona subio, la vista no sigue el final: %q", ultima())
	}
	m.Tecla("G", ttAhora)
	if !strings.HasSuffix(ultima(), "evento 31") {
		t.Fatalf("CA-711: G vuelve al final: %q", ultima())
	}
	leer(33)
	if !strings.HasSuffix(ultima(), "evento 32") {
		t.Fatalf("CA-711: desde el final sigue de nuevo: %q", ultima())
	}
}

// ---------------------------------------------------------------- CA-559: la vista Hallazgos (la v2: [4] y su [0])

// CA-559: la vista Hallazgos (H) lista los hallazgos abiertos con los high
// primero, despues los medium, los low y cualquier otra severidad (cada
// grupo en el orden de la lectura); cada fila con la severidad rellena a 6,
// la tarea y <archivo>:<linea> sin carpetas; el titulo los cuenta y, con
// color, suma " · <k> high" en rojo. Re-expresa CA-524.
func TestCA559_CA524_FilasDeHallazgos(t *testing.T) {
	hs := []finding.Item{
		ttHallazgo("l1", finding.StatusOpen, "low", "a/l1.go", "d"),
		ttHallazgo("x1", finding.StatusOpen, "critica", "x.go", "d"),
		ttHallazgo("h1", finding.StatusOpen, "high", `c:\w\h1.go:3`, "d"),
		ttHallazgo("m1", finding.StatusOpen, "medium", "", "d"),
		ttHallazgo("h2", finding.StatusOpen, "high", "b/h2.go:9", "d"),
	}
	hs[3].Task = ""
	m := ttModelo(ttConfig(120, 32, false), ttTableroPrincipal())
	m.LeerHallazgos(tablero.Hallazgos{Items: hs})
	m.Tecla("H", ttAhora)
	c4 := tv3bLista(t, "CA-559", m)
	if c4.Titulo != "Hallazgos 5" {
		t.Fatalf("CA-712: el titulo de la vista Hallazgos cuenta los abiertos: %q", c4.Titulo)
	}
	// L = 48, interior 46: room = 36, tarea a 18, donde a 17
	fila := func(mk, sev, tarea, donde string) string {
		return fmt.Sprintf(" %s %-6s %-18s %s", mk, sev, tarea, donde)
	}
	quiere := []string{
		fila(">", "high", "infra-api", "h1.go:3"),
		fila(" ", "high", "infra-api", "h2.go:9"),
		fila(" ", "medium", "-", "-"),
		fila(" ", "low", "infra-api", "l1.go"),
		fila(" ", "critica", "infra-api", "x.go"),
	}
	ttIgualesAlPrincipio(t, "CA-559", "la vista Hallazgos ordenada por severidad", c4.ttTexto(), quiere)
	m.Tecla("G", ttAhora)
	if e := ttElegidaEn(tv3bLista(t, "CA-559", m).ttCaja); e != "critica infra-api          x.go" {
		t.Fatalf("CA-559: una severidad desconocida va despues de los low: %q", e)
	}
	// las filas que no entran: a 60x15 la lista tiene 2 lineas interiores y su
	// borde de abajo dice "1 de 5" (la v2 lo veia en [4] a 120x32)
	p := ttModelo(ttConfig(60, 15, false), ttTableroPrincipal())
	p.LeerHallazgos(tablero.Hallazgos{Items: hs})
	p.Tecla("H", ttAhora)
	if c := tv3bLista(t, "CA-707", p); c.Pie != "1 de 5" {
		t.Fatalf("CA-707: la vista Hallazgos con 5 filas en %d lineas dice '1 de 5': %q", len(c.Dentro), c.Abajo)
	}
	// con color, la severidad en su color y " · 2 high" en rojo en el titulo
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	k := ttModelo(cfg, ttTableroPrincipal())
	k.LeerHallazgos(tablero.Hallazgos{Items: hs})
	k.Tecla("H", ttAhora)
	var titulo string
	for _, l := range k.Pantalla() {
		if strings.Contains(ttSinSGR(l), "Hallazgos 5 · 2 high") {
			titulo = l
		}
	}
	if titulo == "" || ttEstiloDe(titulo, " · 2 high") != "\x1b[31m" {
		t.Fatalf("CA-712: con color el titulo suma ' · 2 high' en rojo: %q", titulo)
	}
	// vacio y antes de la primera lectura
	v := ttModelo(ttConfig(120, 32, false), ttTableroPrincipal())
	v.Tecla("H", ttAhora)
	if got := tv3bLista(t, "CA-559", v).ttTexto(); got[0] != " leyendo los hallazgos..." {
		t.Fatalf("CA-559: antes de la primera lectura la vista Hallazgos dice ' leyendo los hallazgos...': %q", got)
	}
	v.LeerHallazgos(tablero.Hallazgos{})
	if c := tv3bLista(t, "CA-559", v); c.ttTexto()[0] != " sin hallazgos abiertos" || c.Titulo != "Hallazgos" {
		t.Fatalf("CA-559: sin hallazgos, 'Hallazgos' y ' sin hallazgos abiertos': %+v", c)
	}
}

// CA-559: el detalle de un hallazgo (la v2: [0] de [4]): la severidad, la
// lente y la tarea; la descripcion entera; el archivo con su linea; las
// lineas del archivo alrededor de la suya (K = min(ContextoArchivo, (H - F -
// 1) / 2)), con la del hallazgo marcada con > y los numeros alineados; y el
// comando para cerrarlo.
func TestCA559_CeroDeUnHallazgo(t *testing.T) {
	h := ttHallazgo("h-high", finding.StatusOpen, "high", "internal/api/router.go:41", "el router acepta rutas sin version")
	lineas := func(desde, hasta int) []string {
		var out []string
		for i := desde; i <= hasta; i++ {
			out = append(out, fmt.Sprintf("codigo %d", i))
		}
		return out
	}
	// 160x32: el detalle de 96 celdas (94 interiores: la linea de cerrarlo, 90
	// celdas, entra entera) y 22 lineas interiores; F = 6 lineas propias:
	// K = min(10, (22 - 6 - 1) / 2) = 7
	m := ttModelo(ttConfig(160, 32, false), ttTableroPrincipal())
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h}})
	m.Tecla("H", ttAhora)
	if r, l := m.ArchivoElegido(); r != "internal/api/router.go" || l != 41 {
		t.Fatalf("CA-559: ArchivoElegido es la ruta y la linea: %q %d", r, l)
	}
	if ti := tv3bDetalle(t, "CA-708", m).Titulo; ti != "hallazgo h-high" {
		t.Fatalf("CA-708: el titulo del detalle de un hallazgo es 'hallazgo <id>': %q", ti)
	}
	got := tv3bDetalle(t, "CA-559", m).ttTexto()
	ttIgualesAlPrincipio(t, "CA-559", "antes de leer el archivo", got, []string{
		" high · lente risk · tarea infra-api",
		" el router acepta rutas sin version",
		"",
		" internal/api/router.go:41",
		" leyendo internal/api/router.go...",
		"",
		` para cerrarlo: hoom finding resolve h-high --as corregido|refutado --evidence "<por que>"`,
	})
	m.LeerArchivo(tablero.Archivo{Ruta: "internal/api/router.go", Linea: 41, Desde: 31, Lineas: lineas(31, 51)})
	got = tv3bDetalle(t, "CA-559", m).ttTexto()
	quiere := []string{" high · lente risk · tarea infra-api", " el router acepta rutas sin version", "", " internal/api/router.go:41"}
	for i := 34; i <= 48; i++ {
		mk := " "
		if i == 41 {
			mk = ">"
		}
		quiere = append(quiere, fmt.Sprintf(" %s %d codigo %d", mk, i, i))
	}
	quiere = append(quiere, "", ` para cerrarlo: hoom finding resolve h-high --as corregido|refutado --evidence "<por que>"`)
	ttIgualesAlPrincipio(t, "CA-559", "con las lineas del archivo", got, quiere)
	// una lectura de otra ruta se ignora
	m.LeerArchivo(tablero.Archivo{Ruta: "otra.go", Linea: 41, Desde: 1, Lineas: []string{"no"}})
	if !strings.Contains(strings.Join(tv3bDetalle(t, "CA-559", m).ttTexto(), "\n"), "\n > 41 codigo 41\n") {
		t.Fatalf("CA-559: una lectura que no es la de ArchivoElegido se ignora")
	}
	// los numeros se alinean a la derecha al mas ancho
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{ttHallazgo("h9", finding.StatusOpen, "low", "a.go:8", "d")}})
	m.LeerArchivo(tablero.Archivo{Ruta: "a.go", Linea: 8, Desde: 1, Lineas: lineas(1, 12)})
	todo := strings.Join(tv3bDetalle(t, "CA-559", m).ttTexto(), "\n")
	if !strings.Contains(todo, "\n    1 codigo 1\n") || !strings.Contains(todo, "\n >  8 codigo 8\n") || !strings.Contains(todo, "\n   12 codigo 12\n") {
		t.Fatalf("CA-559: los numeros van alineados a la derecha al mas ancho:\n%s", todo)
	}
}

// ttLineaFueraEnCero exige que el detalle del hallazgo tenga la linea exacta
// del mensaje de la v1 para una ruta fuera del proyecto, " " +
// ttFuera(ruta) (strconv.Quote), y que vaya en rojo (CA-559, hallazgo
// 00f2e0).
func ttLineaFueraEnCero(t *testing.T, m *tablero.Modelo, ruta string) {
	t.Helper()
	quiere := " " + ttFuera(ruta)
	vistas := 0
	for _, l := range tv3bDetalle(t, "CA-559", m).ttTexto() {
		if strings.Contains(l, "fuera del proyecto") {
			vistas++
			if l != quiere {
				t.Fatalf("CA-559 (hallazgo 00f2e0): el detalle dice %q, dice %q", quiere, l)
			}
		}
	}
	if vistas != 1 {
		t.Fatalf("CA-559 (hallazgo 00f2e0): el detalle tiene una linea %q:\n%s", quiere, ttPantalla(m))
	}
	rojo := false
	for _, x := range m.Pantalla() {
		if strings.Contains(ttSinSGR(x), quiere) && ttConColor(ttRojo, x, ttFuera(ruta)) {
			rojo = true
		}
	}
	if !rojo {
		t.Fatalf("CA-559 (hallazgo 00f2e0): la linea %q va en rojo:\n%s", quiere, strings.Join(m.Pantalla(), "\n"))
	}
}

// CA-559, casos limite: un hallazgo sin archivo (su detalle va sin lineas y
// e dice "el hallazgo no nombra un archivo"); el archivo que no se puede leer
// (el detalle dice en rojo "no se pudo leer <archivo>: <error>"); una ruta
// fuera del proyecto por su forma o por un enlace (Fuera) (el mensaje de la
// v1, en rojo, sin leer ni abrir nada); la linea en la descripcion; linea 0.
func TestCA559_CasosLimiteDelHallazgo(t *testing.T) {
	cfg := ttConfig(160, 32, false)
	cfg.Color = true
	abrir := func(h finding.Item) *tablero.Modelo {
		m := ttModelo(cfg, ttTableroPrincipal())
		m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h}})
		m.Tecla("H", ttAhora)
		return m
	}
	// sin archivo
	m := abrir(ttHallazgo("s", finding.StatusOpen, "low", "", "no dice donde"))
	got := tv3bDetalle(t, "CA-559", m).ttTexto()
	ttIgualesAlPrincipio(t, "CA-559", "sin archivo", got, []string{
		" low · lente risk · tarea infra-api",
		" no dice donde",
		"",
		` para cerrarlo: hoom finding resolve s --as corregido|refutado --evidence "<por que>"`,
	})
	if r, _ := m.ArchivoElegido(); r != "" {
		t.Fatalf("CA-559: sin archivo, ArchivoElegido es \"\": %q", r)
	}
	ttSinOrdenes(t, "CA-559", "e sin archivo", m.Tecla("e", ttAhora))
	ttDiceMensaje(t, "CA-559", "e sin archivo", m, false, "el hallazgo no nombra un archivo")
	// no se puede leer
	m = abrir(ttHallazgo("r", finding.StatusOpen, "high", "x/y.go:3", "d"))
	m.LeerArchivo(tablero.Archivo{Ruta: "x/y.go", Linea: 3, Err: errors.New("permiso denegado")})
	var l string
	for _, x := range m.Pantalla() {
		if strings.Contains(ttSinSGR(x), "no se pudo leer x/y.go: permiso denegado") {
			l = x
		}
	}
	if l == "" || !ttConColor(ttRojo, l, "no se pudo leer x/y.go") {
		t.Fatalf("CA-559: un archivo que no se lee: ' no se pudo leer <archivo>: <error>' en rojo: %q\n%s", l, ttPantalla(m))
	}
	// fuera por un enlace (Archivo.Fuera): la linea exacta, en rojo (CA-559,
	// hallazgo 00f2e0)
	m.LeerArchivo(tablero.Archivo{Ruta: "x/y.go", Linea: 3, Fuera: true})
	ttLineaFueraEnCero(t, m, "x/y.go")
	// fuera por su forma: no se lee (ArchivoElegido "") ni se abre
	for _, file := range []string{"../fuera.go:3", "/etc/passwd", `C:\x.go`, `..\..\x.go`, `\\srv\r\x.go`} {
		m = abrir(ttHallazgo("f", finding.StatusOpen, "high", file, "d"))
		if r, _ := m.ArchivoElegido(); r != "" {
			t.Fatalf("CA-559: %q sale del proyecto por su forma: ArchivoElegido es \"\": %q", file, r)
		}
		ruta := strings.TrimSuffix(file, ":3")
		ttLineaFueraEnCero(t, m, ruta) // CA-559, hallazgo 00f2e0: sin la alternativa que dejaba muerta a la primera
		ttSinOrdenes(t, "CA-559", "e en "+file, m.Tecla("e", ttAhora))
		ttDiceMensaje(t, "CA-559", "e en "+file, m, true, ttFuera(ruta))
	}
	// la linea en la descripcion
	m = abrir(ttHallazgo("d", finding.StatusOpen, "high", "a.go", "en a.go:17 falla el parseo"))
	if r, l := m.ArchivoElegido(); r != "a.go" || l != 17 {
		t.Fatalf("CA-559: la linea sale de '<file>:' en la descripcion: %q %d", r, l)
	}
	// linea 0: las primeras lineas, ninguna marcada
	m = abrir(ttHallazgo("z", finding.StatusOpen, "high", "b.go", "sin linea"))
	m.LeerArchivo(tablero.Archivo{Ruta: "b.go", Linea: 0, Desde: 1, Lineas: []string{"uno", "dos", "tres"}})
	todo := strings.Join(tv3bDetalle(t, "CA-559", m).ttTexto(), "\n")
	if !strings.Contains(todo, " b.go\n   1 uno\n   2 dos\n   3 tres") || strings.Contains(todo, " > ") {
		t.Fatalf("CA-559: con linea 0, las primeras lineas sin marca:\n%s", todo)
	}
}

// CA-559: e en la lista de la vista Hallazgos y en su detalle (enter lleva el
// foco al detalle) ordena AbrirEditor con el archivo relativo a la raiz y su
// linea (y suma la entrada del editor al Registro).
func TestCA559_EAbreElArchivoDelHallazgo(t *testing.T) {
	for _, enCero := range []bool{false, true} {
		m := ttModeloDorado(ttConfig(120, 32, false))
		m.Tecla("H", ttAhora)
		if enCero {
			m.Tecla(tablero.TeclaEnter, ttAhora)
			if m.Lugar() != tablero.LugarVerHallazgo {
				t.Fatalf("CA-712: enter en la lista de la vista Hallazgos pasa el foco al detalle: %q", m.Lugar())
			}
		}
		ords := m.Tecla("e", ttAhora)
		if len(ords) != 1 {
			t.Fatalf("CA-559: e (en el detalle: %v) da una orden: %#v", enCero, ords)
		}
		if a, ok := ords[0].(tablero.AbrirEditor); !ok || a.Archivo != "internal/api/router.go" || a.Linea != 41 {
			t.Fatalf("CA-559: e abre el archivo del hallazgo en su linea: %#v", ords[0])
		}
		if r := m.Registro(); len(r) != 1 || r[0].Texto != "Abrir en el editor · internal/api/router.go:41" || r[0].Comando != "vi +41 internal/api/router.go" {
			t.Fatalf("CA-563: la entrada del editor: %#v", r)
		}
	}
}

// ---------------------------------------------------------------- CA-560: la cabecera y el detalle con v (la v2: [1] y su [0])

// CA-560: el proyecto y la rama, el check, el gasto de hoy y la tarjeta
// fijada, cada una en sus formas: la v2 los mostraba en las cuatro lineas de
// [1]; en la v3 la cabecera los acorta en una linea y el detalle con v
// empieza con esas cuatro lineas enteras (doc.go, "The detail of the
// project"). Re-expresa CA-531.
func TestCA560_CA531_LasLineasDeProyecto(t *testing.T) {
	m := tablero.Nuevo(ttConfig(120, 32, false))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTableroPrincipal()})
	if got := tv3bCabecera(m); got != tv3bPanCabecera(120, " hoom tablero · demo", "leyendo el proyecto...") {
		t.Fatalf("CA-560: antes del primer Proyecto la cabecera dice ' hoom tablero · demo' y 'leyendo el proyecto...': %q", got)
	}
	m.Tecla("v", ttAhora)
	if got := tv3bDetalle(t, "CA-560", m).ttTexto(); got[0] != " leyendo el proyecto..." {
		t.Fatalf("CA-560: antes del primer Proyecto el detalle con v dice ' leyendo el proyecto...': %q", got)
	}
	casos := []struct {
		nombre   string
		p        func() tablero.Proyecto
		fijada   string
		izq, der string // la cabecera
		quiere   []string
	}{
		{"verde", ttProyectoVerde, "nuc-tenancy", " hoom tablero · demo · rama main", "check VERDE · verify hace 2h · hoy 3.88 USD · fijada: nuc-tenancy",
			[]string{" demo · rama main", " check VERDE · verify hace 2h", " hoy 3.88 USD en 4 trabajos", " fijada: nuc-tenancy"}},
		{"sin git, sin veredicto, sin trabajos", func() tablero.Proyecto {
			return tablero.Proyecto{Check: checkcmd.Result{Reason: checkcmd.ReasonNoVerdict}}
		}, "", " hoom tablero · demo · sin git", "sin veredicto todavia · hoy sin trabajos",
			[]string{" demo · sin git", " sin veredicto todavia", " hoy sin trabajos", " sin tarjeta fijada"}},
		{"rojo por drift, con sin dato", func() tablero.Proyecto {
			p := ttProyectoVerde()
			p.Check = checkcmd.Result{OK: false, Reason: checkcmd.ReasonDrift, VerdictID: "v9"}
			p.Hoy = costcmd.Hoy{Runs: 1, CostoUSD: 0.5, SinCosto: 2}
			return p
		}, "", " hoom tablero · demo · rama main", "check ROJO: " + checkcmd.Motivo(checkcmd.Result{Reason: checkcmd.ReasonDrift, VerdictID: "v9"}) + " · hoy 0.50 USD",
			[]string{" demo · rama main", " check ROJO: " + checkcmd.Motivo(checkcmd.Result{Reason: checkcmd.ReasonDrift, VerdictID: "v9"}),
				" hoy 0.50 USD en 1 trabajos · 2 sin dato", " sin tarjeta fijada"}},
	}
	// a 240 columnas el detalle tiene 144 celdas: la linea del check rojo
	// entra entera, y la cabecera entera tambien
	for _, c := range casos {
		m := tablero.Nuevo(ttConfig(240, 32, false))
		m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTableroPrincipal(), Fijada: c.fijada})
		m.LeerProyecto(c.p())
		if got := tv3bCabecera(m); got != tv3bPanCabecera(240, c.izq, c.der) {
			t.Fatalf("CA-560 (%s): la cabecera dice\n %q\nse esperaba\n %q", c.nombre, got, tv3bPanCabecera(240, c.izq, c.der))
		}
		m.Tecla("v", ttAhora)
		got := tv3bDetalle(t, "CA-560", m).ttTexto()
		if len(got) < 4 || strings.Join(got[:4], "|") != strings.Join(c.quiere, "|") {
			t.Fatalf("CA-560 (%s): el detalle con v empieza con\n %q\nse esperaba\n %q", c.nombre, got, c.quiere)
		}
	}
	// hace <t>: s por debajo del minuto, despues la unidad entera mas grande
	for d, q := range map[time.Duration]string{30 * time.Second: "30s", 90 * time.Second: "1m", 3599 * time.Second: "59m",
		time.Hour: "1h", 47 * time.Hour: "1d", 72 * time.Hour: "3d", -time.Minute: "0s"} {
		p := ttProyectoVerde()
		p.Veredicto.CreatedAt = ttAhora.Add(-d)
		m := ttModelo(ttConfig(200, 32, false), ttTableroPrincipal())
		m.LeerProyecto(p)
		m.Tecla("v", ttAhora)
		if got := tv3bDetalle(t, "CA-560", m).ttTexto()[1]; got != " check VERDE · verify hace "+q {
			t.Fatalf("CA-560: un veredicto de hace %v dice 'hace %s': %q", d, q, got)
		}
	}
	// con color VERDE en verde
	cfg := ttConfig(200, 32, false)
	cfg.Color = true
	k := ttModelo(cfg, ttTableroPrincipal())
	k.LeerProyecto(ttProyectoVerde())
	for _, l := range k.Pantalla() {
		if strings.Contains(ttSinSGR(l), "check VERDE") && ttEstiloDe(l, "VERDE") != "\x1b[32m" {
			t.Fatalf("CA-560: VERDE va en verde: %q", l)
		}
	}
}

// CA-560: el detalle con v (la v2: [0] de [1]) muestra, despues de las cuatro
// lineas del proyecto, el ultimo veredicto (id, hace cuanto, VERDE o ROJO,
// parcial, sus cuentas y su ruta), la linea del check y los problemas de
// hoom board doctor con su accion.
func TestCA560_CeroDelProyecto(t *testing.T) {
	p := ttProyectoVerde()
	p.Problemas = []boardcmd.Problem{
		{ID: boardcmd.ProbSpecEditado, Slug: "infra-api", What: "spec editado", Plain: "el spec cambio despues de aprobarlo", Action: "hoom aprobar infra-api"},
		{ID: boardcmd.ProbEvidenciaSinItem, Slug: "", What: "evidencia sin item", Plain: "", Action: "hoom item backfill"},
		{ID: boardcmd.ProbSinSpec, Slug: "viejo", What: "sin spec", Plain: "", Action: "hoom pedir arquitecto viejo"},
	}
	m := ttModelo(ttConfig(200, 50, false), ttTableroPrincipal())
	m.LeerProyecto(p)
	m.Tecla("v", ttAhora)
	c0 := tv3bDetalle(t, "CA-560", m)
	if c0.Titulo != "Proyecto" {
		t.Fatalf("CA-713: el titulo del detalle con v es 'Proyecto': %q", c0.Titulo)
	}
	ttIgualesAlPrincipio(t, "CA-560", "el detalle del proyecto", c0.ttTexto(), []string{
		" demo · rama main",
		" check VERDE · verify hace 2h",
		" hoy 3.88 USD en 4 trabajos",
		" sin tarjeta fijada",
		"",
		" Veredicto",
		"  20261108T100000_abc123 · hace 2h · VERDE",
		"  pass 7 · fail 0 · error 0 · absent 1",
		"  .hoom/verdicts/20261108T100000_abc123.json",
		"",
		" Check",
		"  check VERDE · verify hace 2h",
		"",
		" Doctor",
		"  infra-api · el spec cambio despues de aprobarlo: hoom aprobar infra-api",
		"  evidencia sin item: hoom item backfill",
		"  viejo · sin spec: hoom pedir arquitecto viejo",
	})
	p = tablero.Proyecto{Git: true, Rama: "main", Check: checkcmd.Result{Reason: checkcmd.ReasonNoVerdict}}
	m.LeerProyecto(p)
	got := tv3bDetalle(t, "CA-560", m).ttTexto()
	ttIgualesAlPrincipio(t, "CA-560", "el detalle del proyecto sin veredicto ni problemas", got[5:], []string{
		" Veredicto",
		"  sin veredicto todavia",
		"",
		" Check",
		"  sin veredicto todavia",
		"",
		" Doctor",
		"  sin problemas",
	})
	rojo := &statuscmd.LastVerdict{ID: "v-r", Verdict: "red", Partial: true, CreatedAt: ttAhora.Add(-30 * time.Second),
		Summary: verdict.Summary{Pass: 1, Fail: 2, Error: 3, Absent: 4}}
	m.LeerProyecto(tablero.Proyecto{Git: true, Rama: "main", Veredicto: rojo, Check: checkcmd.Result{Reason: checkcmd.ReasonRedVerdict, VerdictID: "v-r"}})
	got = tv3bDetalle(t, "CA-560", m).ttTexto()
	if got[6] != "  v-r · hace 30s · ROJO · parcial" || got[7] != "  pass 1 · fail 2 · error 3 · absent 4" || got[8] != "  .hoom/verdicts/v-r.json" {
		t.Fatalf("CA-560: un veredicto rojo y parcial: %q", got)
	}
	// antes de la primera lectura
	n := ttModelo(ttConfig(200, 50, false), ttTableroPrincipal())
	n.Tecla("v", ttAhora)
	if got := tv3bDetalle(t, "CA-560", n).ttTexto(); got[0] != " leyendo el proyecto..." {
		t.Fatalf("CA-560: antes del primer Proyecto, el detalle con v dice ' leyendo el proyecto...': %q", got)
	}
}

// CA-560: u ejecuta hoom usar <slug> enseguida, sin modal, y suma su entrada
// al Registro ("Fijar para la terminal"); la cabecera (la v2: [1]) muestra la
// tarjeta fijada tras la siguiente lectura. Re-expresa CA-531.
func TestCA560_CA531_UFijaSinModal(t *testing.T) {
	m := ttModeloDorado(ttConfig(200, 40, false))
	tv3bIrA(t, m, "pagos-caidos")
	l := ttUnaLanzar(t, "CA-560", "u", m.Tecla("u", ttAhora.Add(time.Minute)))
	ttMismosArgs(t, "CA-560", "u", l.Args, []string{"usar", "pagos-caidos"})
	if l.ID != 1 || l.Slug != "pagos-caidos" || l.Rol != "" || m.Modal() != tablero.SinModal {
		t.Fatalf("CA-560: u es el Lanzar 1, de la tarjeta, sin rol, sin modal: %#v, modal %q", l, m.Modal())
	}
	r := m.Registro()
	if len(r) != 1 || r[0].Texto != "Fijar para la terminal · pagos-caidos · lanzando..." || r[0].Comando != "hoom usar pagos-caidos" {
		t.Fatalf("CA-563: u va al Registro como 'Fijar para la terminal' con su comando: %#v", r)
	}
	if got := tv3bCabecera(m); !strings.HasSuffix(got, " · fijada: nuc-tenancy") {
		t.Fatalf("CA-560: la cabecera cambia con la siguiente lectura, no antes: %q", got)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Minute), Tablero: ttTableroDorado(), Fijada: "pagos-caidos"})
	if got := tv3bCabecera(m); !strings.HasSuffix(got, " · fijada: pagos-caidos") {
		t.Fatalf("CA-560: tras la siguiente lectura la cabecera muestra la tarjeta fijada: %q", got)
	}
	// u desde la fila de CORRIENDO (la v2: [3]) y desde el detalle de una
	// tarjeta, de 1 en 1
	tv3bIrA(t, m, "infra-api")
	l = ttUnaLanzar(t, "CA-560", "u en CORRIENDO", m.Tecla("u", ttAhora))
	ttMismosArgs(t, "CA-560", "u en CORRIENDO", l.Args, []string{"usar", "infra-api"})
	tv3bAlDetalle(t, m)
	l = ttUnaLanzar(t, "CA-560", "u en el detalle", m.Tecla("u", ttAhora))
	ttMismosArgs(t, "CA-560", "u en el detalle", l.Args, []string{"usar", "infra-api"})
	if l.ID != 3 {
		t.Fatalf("CA-528: cada orden tiene su numero, de 1 en 1: %d", l.ID)
	}
	if m.Corriendo() != 0 {
		t.Fatalf("CA-528: usar no es un trabajo de rol: Corriendo = %d", m.Corriendo())
	}
	// el fin de usar pone su ultima linea en la linea de mensajes
	m.Termino(tablero.FinProceso{ID: 3, Code: 0, Ultima: "tarjeta fijada para esta terminal: infra-api"}, ttAhora)
	ttDiceMensaje(t, "CA-560", "el fin de usar", m, false, "tarjeta fijada para esta terminal: infra-api")
	// sin tarjeta (el detalle con v, la vista Hallazgos, la linea de HECHO; la
	// v2: [1], [4] y Hecho) u no hace nada
	m.Tecla(tablero.TeclaEsc, ttAhora) // vuelve a la lista
	m.Tecla("v", ttAhora)
	ttSinOrdenes(t, "CA-560", "u en el detalle con v", m.Tecla("u", ttAhora))
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("H", ttAhora)
	ttSinOrdenes(t, "CA-560", "u en la vista Hallazgos", m.Tecla("u", ttAhora))
	m.Tecla("H", ttAhora)
	tv3bIrAHecho(t, m)
	ttSinOrdenes(t, "CA-560", "u en la linea de HECHO", m.Tecla("u", ttAhora))
}

// ---------------------------------------------------------------- CA-578: la tarjeta que desaparece

// CA-578: si la tarjeta elegida desaparece en una lectura, la seleccion pasa
// a la fila que ahora tiene su indice y el detalle la muestra; si el foco
// estaba en el detalle mostrandola, vuelve a la lista y la linea de mensajes
// dice "la tarjeta <slug> ya no existe" durante DuracionAviso. Re-expresa
// CA-535.
func TestCA578_CA535_LaElegidaDesaparece(t *testing.T) {
	b := ttTablero(ttBacklog("a", "A"), ttBacklog("b", "B"), ttBacklog("c", "C"))
	m := ttModelo(ttConfig(100, 30, false), b)
	m.Tecla("j", ttAhora) // b
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttBacklog("a", "A"), ttBacklog("c", "C"))})
	if d := tv3bDetalle(t, "CA-578", m); m.Elegida() != "c" || tv3bTituloElegido(t, "CA-578", m) != "C" || d.Titulo != "C" || d.Derecha != "c" {
		t.Fatalf("CA-578: la elegida desaparece: la seleccion pasa a la siguiente y el detalle la muestra: %q\n%s", m.Elegida(), ttPantalla(m))
	}
	if ttMensaje(m) != "" {
		t.Fatalf("CA-578: con el foco en la lista no hay mensaje: %q", ttMensaje(m))
	}
	// la ultima tarjeta: la fila que ahora tiene su indice es la linea de
	// HECHO (una fila tambien), y el detalle no muestra nada
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttBacklog("a", "A"))})
	if m.Elegida() != "" || !strings.HasPrefix(tv3bTituloElegido(t, "CA-578", m), "HECHO ") {
		t.Fatalf("CA-578: la seleccion pasa a la fila siguiente (la de HECHO): %q\n%s", m.Elegida(), ttPantalla(m))
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero()})
	// D2: a 100x30 la lista tiene 38 celdas interiores y el texto (42 con su
	// margen) se parte por palabras
	if got := tv3bLista(t, "CA-578", m).ttTexto(); m.Elegida() != "" || len(got) < 2 ||
		got[0] != " no hay tarjetas: hoom item add" || got[1] != ` "<titulo>"` {
		t.Fatalf("CA-578: sin ninguna tarjeta, la lista lo dice (partido por palabras) y nada esta elegido: %q\n%s", got, ttPantalla(m))
	}
	// con el foco en el detalle
	m = ttModelo(ttConfig(120, 32, false), b)
	m.Tecla("j", ttAhora)
	tv3bAlDetalle(t, m)
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttBacklog("a", "A"), ttBacklog("c", "C"))})
	if m.Foco() != tablero.ZonaLista {
		t.Fatalf("CA-578: con el foco en el detalle mostrandola, el foco vuelve a la lista: %d", int(m.Foco()))
	}
	ttDiceMensaje(t, "CA-578", "la tarjeta que ya no existe", m, false, "la tarjeta b ya no existe")
	m.Tic(t0.Add(tablero.DuracionAviso + time.Second))
	if strings.Contains(ttMensaje(m), "ya no existe") {
		t.Fatalf("CA-578: el aviso dura DuracionAviso: %q", ttMensaje(m))
	}
	// un modal de esa tarjeta se cierra
	w := ttTarjetaCon(ttAccionWriter())
	m = ttModelo(ttConfig(120, 32, false), ttTablero(w, ttBacklog("otra", "Otra")))
	tv3bIrA(t, m, "tarjeta")
	m.Tecla("w", ttAhora)
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-705: w abre el modal Pedir: %q", m.Modal())
	}
	ords := m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttBacklog("otra", "Otra"))})
	if len(ttLanzares(ords)) != 0 || m.Modal() != tablero.SinModal || m.Foco() != tablero.ZonaLista {
		t.Fatalf("CA-578: un modal de la tarjeta que desaparece se cierra sin lanzar: %#v, modal %q", ords, m.Modal())
	}
	ttDiceMensaje(t, "CA-578", "el modal cerrado", m, false, "la tarjeta tarjeta ya no existe")
	// el grupo CORRIENDO (la v2: [3]): la que deja de correr no desaparece, sale
	// de CORRIENDO (aca a DETENIDA, por su rojo) y sigue elegida, sin el aviso
	// de que ya no existe
	r := ttModeloDorado(ttConfig(120, 32, false))
	tv3bIrA(t, r, "infra-api")
	r.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttBacklog("infra-ci-cd", "CI y CD de la infra"), ttBacklog("docs-guia", "Guia de la documentacion"),
		ttPrecios(), ttNucConUntraced(), ttInfraAPI(false), ttPagosCaidos(), ttViejo(1), ttViejo(2), ttViejo(3))})
	if r.Elegida() != "infra-api" || r.Grupo("infra-api") != tablero.GrupoDetenida || tv3bTituloElegido(t, "CA-578", r) != "Convenciones de la API" {
		t.Fatalf("CA-578: la que deja de correr sigue elegida, ahora en DETENIDA: %q, %q\n%s", r.Elegida(), r.Grupo("infra-api"), ttPantalla(r))
	}
	for _, x := range tv3bLista(t, "CA-578", r).ttTexto() {
		if strings.HasPrefix(x, " CORRIENDO") {
			t.Fatalf("CA-711: sin nada corriendo, la lista no tiene el grupo CORRIENDO: %q", x)
		}
	}
	if strings.Contains(ttMensaje(r), "ya no existe") {
		t.Fatalf("CA-578: la que deja de correr no desaparece: %q", ttMensaje(r))
	}
}
