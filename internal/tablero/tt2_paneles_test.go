// Lo que muestra cada panel (spec tablero-con-paneles): CA-555 [2]
// Tarjetas, CA-554 la busqueda, CA-556 y CA-557 [0] de una tarjeta, CA-558
// [3] Corriendo, CA-559 [4] Hallazgos, CA-560 [1] Proyecto y CA-578 la
// tarjeta que desaparece. Las lineas esperadas salen de doc.go ("[2]
// Tarjetas", "[0] of a card", "[3] Corriendo", "[4] Hallazgos", "[1]
// Proyecto"), escritas aca a mano.
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

// ttFila es una fila de tarjeta de [2] con el slug relleno a s celdas.
func ttFila(elegida bool, slug string, s int, tira string) string {
	m := " "
	if elegida {
		m = ">"
	}
	return fmt.Sprintf(" %s %-*s %s", m, s, slug, tira)
}

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

// ---------------------------------------------------------------- CA-555: [2]

// CA-555: [2] muestra las secciones de la v1 en su orden (NECESITA TU
// DECISION, EN CURSO y una por columna en el orden del tablero, con su
// cantidad), cada tarjeta una sola vez, sin secciones vacias salvo Hecho
// cerrada con su cantidad; cada fila es la marca, el slug relleno y la tira
// pegada al borde derecho, con ! en la primera falta de una tarjeta que
// espera tu decision. Re-expresa CA-518.
func TestCA555_CA518_SeccionesYFilas(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	// L = 40: interior 38, room 35, T = 4: el slug va relleno a 30
	quiere := []string{
		" NECESITA TU DECISION",
		ttFila(true, "nuc-tenancy", 30, "#!-."),
		ttFila(false, "pagos-caidos", 30, "##!-"),
		" EN CURSO",
		ttFila(false, "infra-api", 30, "##-."),
		" BACKLOG 2",
		ttFila(false, "infra-ci-cd", 30, "----"),
		ttFila(false, "docs-guia", 30, "----"),
		" ARQUITECTO 1",
		ttFila(false, "precios", 30, "#---"),
		" HECHO 3 >",
	}
	c := ttCajaDe(t, "CA-555", m, "[2]")
	ttIgualesAlPrincipio(t, "CA-555", "[2] a 120x32", c.ttTexto(), quiere)
	for _, l := range c.ttTexto()[len(quiere):] {
		if l != "" {
			t.Fatalf("CA-555: despues de Hecho cerrada no hay nada: %q", c.ttTexto())
		}
	}
	for _, l := range c.Dentro {
		if ttCeldas(l) != 38 {
			t.Fatalf("CA-555: cada linea interior mide w-2 = 38 celdas: %q", l)
		}
	}
	if c.Titulo != "[2] Tarjetas" {
		t.Fatalf("CA-555: el titulo de [2] es '[2] Tarjetas': %q", c.Titulo)
	}
	// con UTF-8: ✋ ocupa dos celdas y la tira sigue pegada al borde
	u := ttModeloDorado(ttConfig(120, 32, true))
	qu := []string{
		" NECESITA TU DECISIÓN",
		" ▸ " + fmt.Sprintf("%-29s", "nuc-tenancy") + " ●✋○·",
		"   " + fmt.Sprintf("%-29s", "pagos-caidos") + " ●●✋○",
		" EN CURSO",
		"   " + fmt.Sprintf("%-30s", "infra-api") + " ●●○·",
	}
	ttIgualesAlPrincipio(t, "CA-555", "[2] con UTF-8", ttCajaDe(t, "CA-555", u, "[2]").ttTexto(), qu)
}

// CA-555: el corte del slug: se corta con el signo del corte para dejar la
// tira y un espacio; si al slug le quedan menos de 8 celdas, la fila va sin
// tira y el slug se corta al lugar entero.
func TestCA555_CorteDelSlugYSinTira(t *testing.T) {
	segs := func(n int) []boardcmd.Segment {
		var out []boardcmd.Segment
		for i := 0; i < n; i++ {
			out = append(out, ttSeg(fmt.Sprintf("s%d", i), "s", boardcmd.SegHecho, "d"))
		}
		return out
	}
	// 80 columnas: L = 26, interior 24, room = 21
	casos := []struct {
		nombre string
		slug   string
		n      int
		utf    bool
		quiere string
	}{
		{"slug cortado", "abcdefghijklmnopqrst", 4, false, " > abcdefghijklmno~ ####"},
		{"slug cortado con UTF-8", "abcdefghijklmnopqrst", 4, true, " ▸ abcdefghijklmno… ●●●●"},
		{"8 celdas justas", "corta", 12, false, " > corta    ############"},
		{"7 celdas: sin tira", "corta", 13, false, " > corta"},
		{"sin tira y slug largo", "un-slug-de-mas-de-veintiuna", 13, false, " > un-slug-de-mas-de-ve~"},
		{"slug de un caracter", "x", 4, false, ttFila(true, "x", 16, "####")},
	}
	for _, c := range casos {
		card := ttCard(c.slug, "T", boardcmd.ColBacklog, "p", segs(c.n)...)
		m := ttModelo(ttConfig(80, 24, c.utf), ttTablero(card))
		got := ttCajaDe(t, "CA-555", m, "[2]").ttTexto()
		if len(got) < 2 || got[1] != c.quiere {
			t.Fatalf("CA-555 (%s): la fila es\n %q\nse esperaba\n %q", c.nombre, got, c.quiere)
		}
	}
}

// CA-555: con color, los titulos de seccion van en el color de su estado
// (NECESITA TU DECISION y las columnas humanas en amarillo, EN CURSO en cian,
// Hecho en verde, las otras columnas sin estilo) y una tarjeta con red o con
// interrupted lleva el slug en rojo.
func TestCA555_ColoresDeSeccionYSlugEnRojo(t *testing.T) {
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	acepta := ttCard("acepta", "Acepta", boardcmd.ColTuAceptacion, "lista", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	acepta.NeedsDecision, acepta.WaitingHuman = false, false
	conRojo := ttBacklog("con-rojo", "Con rojo")
	conRojo.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v", Plain: "dio rojo"}
	m := ttModelo(cfg, ttTablero(ttNucTenancy(), ttInfraAPI(true), ttPagosCaidos(), conRojo, acepta, ttViejo(1)))
	m.Tecla("4", ttAhora) // sin la barra de la seleccion en [2]
	estilo := func(texto string) string {
		for _, l := range m.Pantalla() {
			if strings.Contains(ttSinSGR(l), texto) {
				return ttEstiloDe(l, texto)
			}
		}
		t.Fatalf("CA-555: la pantalla tiene %q:\n%s", texto, ttPantalla(m))
		return ""
	}
	for texto, quiere := range map[string]string{
		"NECESITA TU DECISION": "\x1b[33m",
		"EN CURSO":             "\x1b[36m",
		"TU ACEPTACION 1":      "\x1b[33m",
		"HECHO 1 >":            "\x1b[32m",
		"BACKLOG 1":            "",
		"pagos-caidos":         "\x1b[31m",
		"con-rojo":             "\x1b[31m",
		"nuc-tenancy":          "",
	} {
		if got := estilo(texto); got != quiere {
			t.Fatalf("CA-555: con color %q va con %q, va con %q", texto, quiere, got)
		}
	}
}

// CA-555, CA-578: antes de la primera lectura [2] dice "leyendo el
// tablero..."; un proyecto sin items, `no hay tarjetas: hoom item add
// "<titulo>"`, y [0] no muestra nada. Re-expresa CA-535.
func TestCA555_CA578_CA535_LeyendoYSinTarjetas(t *testing.T) {
	m := tablero.Nuevo(ttConfig(200, 40, false))
	if got := ttCajaDe(t, "CA-555", m, "[2]").ttTexto(); len(got) == 0 || got[0] != " leyendo el tablero..." {
		t.Fatalf("CA-555: antes de la primera lectura [2] dice ' leyendo el tablero...': %q", got)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero()})
	if got := ttCajaDe(t, "CA-578", m, "[2]").ttTexto(); len(got) == 0 || got[0] != ` no hay tarjetas: hoom item add "<titulo>"` {
		t.Fatalf("CA-578: sin items [2] dice `no hay tarjetas: hoom item add \"<titulo>\"`: %q", got)
	}
	if got := ttCajaDe(t, "CA-578", m, "[0]").Titulo; got != "[0]" || m.Elegida() != "" {
		t.Fatalf("CA-578: sin tarjetas [0] no muestra nada: %q, Elegida %q", got, m.Elegida())
	}
	ttForma(t, "CA-578", m, 200, 40, false)
}

// CA-555, CA-517 (propiedad): para tableros al azar, cada tarjeta aparece una
// sola vez (en la primera seccion que le toca: decision, en curso o su
// columna), las secciones van en el orden del contrato con la cantidad de
// tarjetas que tienen y el nombre de la columna que trae el tablero en
// mayusculas, ninguna seccion vacia se muestra salvo Hecho, y la tira tiene
// un signo por segmento con ! en la primera falta cuando espera tu decision.
// Re-expresa CA-518.
func TestCA555_CA518_PropiedadCadaTarjetaUnaVez(t *testing.T) {
	cols := []string{boardcmd.ColBacklog, boardcmd.ColArquitecto, boardcmd.ColTuAprobacion, boardcmd.ColTestWriter,
		boardcmd.ColWriter, boardcmd.ColReview, boardcmd.ColTuAceptacion, boardcmd.ColHecho}
	estados := []string{boardcmd.SegHecho, boardcmd.SegFalta, boardcmd.SegNoAplica}
	signo := map[string]string{boardcmd.SegHecho: "#", boardcmd.SegFalta: "-", boardcmd.SegNoAplica: "."}
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		n := 1 + r.Intn(12)
		var cards []boardcmd.Card
		tiraDe := map[string]string{}
		for i := 0; i < n; i++ {
			col := cols[r.Intn(len(cols))]
			var segs []boardcmd.Segment
			for k := 0; k < 1+r.Intn(6); k++ {
				segs = append(segs, ttSeg(fmt.Sprintf("s%d", k), "s", estados[r.Intn(3)], "d"))
			}
			c := ttCard(fmt.Sprintf("c-%02d", i), "T", col, "p", segs...)
			if col != boardcmd.ColHecho {
				switch r.Intn(4) {
				case 0:
					c.Running = &boardcmd.Running{Role: "writer", Stage: "run", Step: 1, Steps: 2, StartedAt: ttAhora.Add(-time.Minute)}
				case 1:
					c.Interrupted = &boardcmd.Interrupted{Role: "writer", Stage: "run", Step: 1, Steps: 2}
					c.NeedsDecision = true
				}
			}
			tira, puesto := "", false
			for _, s := range segs {
				if s.State == boardcmd.SegFalta && c.NeedsDecision && !puesto {
					tira, puesto = tira+"!", true
					continue
				}
				tira += signo[s.State]
			}
			tiraDe[c.Slug] = tira
			cards = append(cards, c)
		}
		tb := ttTablero(cards...)
		// los nombres que trae el tablero (no los de boardcmd.Columns)
		for i := range tb.Columns {
			tb.Columns[i].Name = "Col " + tb.Columns[i].Name
		}
		m := ttModelo(ttConfig(240, 80, false), tb)
		m.Tecla("G", ttAhora)
		m.Tecla(tablero.TeclaEnter, ttAhora) // abre Hecho

		type seccion struct {
			titulo string
			slugs  []string
		}
		var quiere []seccion
		usado := map[string]bool{}
		var dec, cur []string
		for _, col := range tb.Columns {
			for _, c := range col.Cards {
				if c.NeedsDecision {
					dec = append(dec, c.Slug)
					usado[c.Slug] = true
				}
			}
		}
		for _, col := range tb.Columns {
			for _, c := range col.Cards {
				if !usado[c.Slug] && c.Running != nil {
					cur = append(cur, c.Slug)
					usado[c.Slug] = true
				}
			}
		}
		if len(dec) > 0 {
			quiere = append(quiere, seccion{" NECESITA TU DECISION", dec})
		}
		if len(cur) > 0 {
			quiere = append(quiere, seccion{" EN CURSO", cur})
		}
		for _, col := range tb.Columns {
			var s []string
			for _, c := range col.Cards {
				if !usado[c.Slug] {
					s = append(s, c.Slug)
				}
			}
			if col.ID == boardcmd.ColHecho {
				quiere = append(quiere, seccion{fmt.Sprintf(" %s %d v", strings.ToUpper(col.Name), len(s)), s})
			} else if len(s) > 0 {
				quiere = append(quiere, seccion{fmt.Sprintf(" %s %d", strings.ToUpper(col.Name), len(s)), s})
			}
		}
		var hay []seccion
		for _, l := range ttCajaDe(t, "CA-555", m, "[2]").ttTexto() {
			if l == "" {
				continue
			}
			if len(l) > 3 && l[0] == ' ' && (l[1] == ' ' || l[1] == '>') && l[2] == ' ' && !strings.HasPrefix(l, " > COL ") {
				f := strings.Fields(l[3:])
				if len(hay) == 0 || len(f) < 2 || f[1] != tiraDe[f[0]] {
					t.Logf("semilla %d: la fila %q (tira esperada %q)", semilla, l, tiraDe[strings.Fields(l[3:])[0]])
					return false
				}
				hay[len(hay)-1].slugs = append(hay[len(hay)-1].slugs, f[0])
				continue
			}
			hay = append(hay, seccion{titulo: strings.Replace(l, " > ", " ", 1)})
		}
		if fmt.Sprint(hay) != fmt.Sprint(quiere) {
			t.Logf("semilla %d:\n hay    %v\n quiere %v\n%s", semilla, hay, quiere, ttPantalla(m))
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 120}); err != nil {
		t.Fatalf("CA-555: %v", err)
	}
}

// ---------------------------------------------------------------- CA-554: la busqueda

// CA-554: / pone el foco en [2] desde cualquier panel, la linea de mensajes
// muestra "/<texto>▌" ("_" en ASCII) y [2] se filtra mientras se escribe,
// por slug o titulo, con las secciones y sus cantidades de lo que queda
// (Hecho sigue con su linea); enter y esc cierran la busqueda, sueltan el
// filtro y dejan elegida la tarjeta que estaba elegida. Re-expresa CA-520.
func TestCA554_CA520_BuscarYCerrar(t *testing.T) {
	for _, cerrar := range []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaEsc} {
		m := ttModeloDorado(ttConfig(120, 32, false))
		m.Tecla("4", ttAhora)
		ttSinOrdenes(t, "CA-554", "/", m.Tecla("/", ttAhora))
		if m.Foco() != tablero.PanelTarjetas || m.Lugar() != tablero.LugarBusqueda {
			t.Fatalf("CA-554: / pone el foco en [2] y abre la busqueda: foco %d, lugar %q", int(m.Foco()), m.Lugar())
		}
		if got := ttMensaje(m); got != "/_" {
			t.Fatalf("CA-554: con la busqueda vacia la linea de mensajes es ' /_': %q", got)
		}
		ttSinOrdenes(t, "CA-554", "escribir", ttEscribir(m, "inf"))
		if got := ttMensaje(m); got != "/inf_" {
			t.Fatalf("CA-554: la linea de mensajes muestra '/inf_': %q", got)
		}
		quiere := []string{
			" EN CURSO",
			ttFila(true, "infra-api", 30, "##-."),
			" BACKLOG 1",
			ttFila(false, "infra-ci-cd", 30, "----"),
			" HECHO 0 >",
		}
		got := ttCajaDe(t, "CA-554", m, "[2]").ttTexto()
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
		if m.Elegida() != "infra-ci-cd" || ttSlugElegido(t, m) != "infra-ci-cd" {
			t.Fatalf("CA-554: %q suelta el filtro y deja elegida la tarjeta elegida: %q\n%s", cerrar, m.Elegida(), ttPantalla(m))
		}
		if !ttTiene(m, "nuc-tenancy") || !ttTiene(m, "pagos-caidos") {
			t.Fatalf("CA-554: sin el filtro vuelven todas las tarjetas:\n%s", ttPantalla(m))
		}
	}
}

// CA-554: el filtro no distingue mayusculas ni acentos (de los dos lados) y
// mira el slug y el titulo; en la busqueda q es texto, no salir, y ctrl-c
// sale; en modo experto la linea es " EXPERTO · /<texto>▌".
func TestCA554_SinMayusculasNiAcentosYQEsTexto(t *testing.T) {
	seg := ttSeg("s", "s", boardcmd.SegHecho, "d")
	b := ttTablero(ttCard("pinata", "La Piñata Ácida", boardcmd.ColWriter, "p", seg), ttCard("zeta", "Zeta", boardcmd.ColWriter, "p", seg),
		ttCard("cafe", "Café con leche", boardcmd.ColWriter, "p", seg))
	for _, busca := range []string{"PIÑATA", "piñata", "pinata", "acida", "ÁCIDA", "Acída", "PINATA ACIDA"} {
		m := ttModelo(ttConfig(120, 32, false), b)
		m.Tecla("/", ttAhora)
		ttEscribir(m, busca)
		got := strings.Join(ttCajaDe(t, "CA-554", m, "[2]").ttTexto(), "\n")
		if !strings.Contains(got, "pinata") || strings.Contains(got, "zeta") || strings.Contains(got, "cafe") {
			t.Fatalf("CA-554: %q encuentra la piñata y nada mas:\n%s", busca, got)
		}
	}
	m := ttModelo(ttConfig(120, 32, false), b)
	m.Tecla("/", ttAhora)
	if os := ttEscribir(m, "CAFE"); len(os) != 0 {
		t.Fatalf("CA-554: escribir no da ordenes: %#v", os)
	}
	if got := strings.Join(ttCajaDe(t, "CA-554", m, "[2]").ttTexto(), "\n"); !strings.Contains(got, "cafe") || strings.Contains(got, "zeta") {
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

// ---------------------------------------------------------------- CA-556: [0] de una tarjeta

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

// CA-556: [0] de una tarjeta es un solo documento en el orden del contrato:
// lo que falta y el siguiente paso, la columna con el gasto (y el
// presupuesto), quien escribio y reviso, el rojo y el doctor, una linea por
// segmento de la tira, los criterios con +, x o v (✓ ✗ ⚑), los hallazgos de
// su tarea (los abiertos primero, el cerrado en gris), la historia y la
// narracion del ultimo sobre. Re-expresa CA-521, CA-523 y CA-531.
func TestCA556_CA521_CA523_ElDocumentoDeLaTarjeta(t *testing.T) {
	c, d := ttDetalleRico()
	m := ttModelo(ttConfig(200, 50, false), ttTablero(c))
	m.LeerDetalle(d)
	got := ttCajaDe(t, "CA-556", m, "[0]").ttTexto()
	quiere := []string{
		" Que falta?  el writer esta trabajando",
		" Siguiente paso  Pedir al writer  [w]",
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
	}
	ttIgualesAlPrincipio(t, "CA-556", "el documento de infra-api", got, quiere)
	sobre := got[len(quiere)]
	if !strings.HasPrefix(sobre, "  writer · run 6/6 · 1:01") || !strings.Contains(sobre, "entregable") || !strings.HasSuffix(sobre, "ultimo evento hace 5s") {
		t.Fatalf("CA-523: sin running, la narracion empieza con el rol, el paso, la duracion y el estado del ultimo sobre y 'ultimo evento hace 5s': %q", sobre)
	}
	if got[len(quiere)+1] != "  11:58:55 tool Read x.go" || got[len(quiere)+2] != "  11:59:55 text leyendo el spec" {
		t.Fatalf("CA-523: despues, un evento por linea con su hora UTC: %q", got[len(quiere):])
	}
	// el titulo
	if ti := ttCajaDe(t, "CA-556", m, "[0]").Titulo; ti != "[0] infra-api · Convenciones de la API" {
		t.Fatalf("CA-556: el titulo de [0] es '<slug> · <titulo>': %q", ti)
	}
}

// CA-556: con color, el rojo va en rojo, la severidad en su color (high
// rojo, medium amarillo, low gris) y un hallazgo cerrado todo en gris.
func TestCA556_ColoresDelDocumento(t *testing.T) {
	c, d := ttDetalleRico()
	cfg := ttConfig(200, 50, false)
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(c))
	m.LeerDetalle(d)
	m.Tecla("1", ttAhora) // sin barras de seleccion encima
	m.Tecla("2", ttAhora)
	linea := func(s string) string {
		for _, l := range m.Pantalla() {
			if strings.Contains(ttSinSGR(l), s) {
				return l
			}
		}
		t.Fatalf("CA-556: la pantalla tiene %q", s)
		return ""
	}
	if l := linea("rojo: la verificacion dio rojo"); !ttConColor(ttRojo, l, "rojo: la verificacion") {
		t.Fatalf("CA-556: 'rojo: <plain>' va en rojo: %q", l)
	}
	if l := linea("high x.go:12"); ttEstiloDe(l, "high") != "\x1b[31m" {
		t.Fatalf("CA-556: la severidad high va en rojo: %q", l)
	}
	if l := linea("medium c.go"); ttEstiloDe(l, "medium") != "\x1b[33m" {
		t.Fatalf("CA-556: la severidad medium va en amarillo: %q", l)
	}
	if l := linea("low - refutado ya"); ttEstiloDe(l, "refutado ya") != "\x1b[90m" || ttEstiloDe(l, "low") != "\x1b[90m" {
		t.Fatalf("CA-556: un hallazgo cerrado va todo en gris: %q", l)
	}
}

// CA-556, CA-531: x alterna el modo experto: la linea de mensajes empieza con
// "EXPERTO" y [0] suma "siguiente: <next>" y una linea "missing:" por cada
// falta, el rojo dice su reason, cada criterio lleva su id antes del texto y
// la historia dice su summary. x otra vez vuelve al modo normal.
func TestCA556_CA531_ModoExperto(t *testing.T) {
	c, d := ttDetalleRico()
	c.Missing = []string{"veredicto verde de la tarjeta", "la review"}
	d.Detalle.Card = c
	m := ttModelo(ttConfig(200, 50, false), ttTablero(c))
	m.LeerDetalle(d)
	ttSinOrdenes(t, "CA-556", "x", m.Tecla("x", ttAhora))
	if got := ttMensaje(m); got != "EXPERTO" {
		t.Fatalf("CA-556: en modo experto, sin mensaje, la linea de mensajes es ' EXPERTO': %q", got)
	}
	got := ttCajaDe(t, "CA-556", m, "[0]").ttTexto()
	ttIgualesAlPrincipio(t, "CA-556", "modo experto", got, []string{
		" Que falta?  el writer esta trabajando",
		" siguiente: hoom agent --role writer --task infra-api",
		" missing: veredicto verde de la tarjeta",
		" missing: la review",
		" Siguiente paso  Pedir al writer  [w]",
		"",
		" Writer · gasto 3.88 USD de 12.25 USD · escribio claude · reviso codex",
		" rojo: test: FAIL",
	})
	todo := strings.Join(got, "\n")
	for _, s := range []string{"  + CA-1 criterio uno", "  x CA-2 criterio dos", "  v CA-3 criterio tres", "  2026-11-08 09:00 Henry Orellana item infra-api"} {
		if !strings.Contains(todo, s) {
			t.Fatalf("CA-556: en modo experto [0] dice %q:\n%s", s, todo)
		}
	}
	m.Tecla("w", ttAhora) // un mensaje en modo experto
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("a", ttAhora) // sin la accion: un aviso
	if got := ttMensaje(m); !strings.HasPrefix(got, "EXPERTO · en Writer no hay ") {
		t.Fatalf("CA-556: en modo experto un mensaje va despues de 'EXPERTO · ': %q", got)
	}
	m.Tecla("x", ttAhora)
	if got := ttMensaje(m); strings.HasPrefix(got, "EXPERTO") {
		t.Fatalf("CA-556: x otra vez vuelve al modo normal: %q", got)
	}
	if strings.Contains(strings.Join(ttCajaDe(t, "CA-556", m, "[0]").ttTexto(), "\n"), "missing:") {
		t.Fatalf("CA-556: en modo normal no hay missing")
	}
}

// CA-556: las variantes del documento: sin presupuesto, sin costo y sin
// writer ni reviewer, sin rojo ni doctor; un criterio ("1 criterio") y un
// hallazgo abierto (" · 1 abierto"); sin spec (" sin spec: <ruta>"); sin
// hallazgos, sin historia y sin sobres; antes del detalle (" leyendo la
// tarjeta...") y con un error (" no se pudo leer la tarjeta: <Err>").
func TestCA556_Variantes(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	m := ttModelo(ttConfig(200, 50, false), ttTablero(c))
	got := ttCajaDe(t, "CA-556", m, "[0]").ttTexto()
	ttIgualesAlPrincipio(t, "CA-556", "antes del detalle", got, []string{
		" Que falta?  el plain de la tarjeta",
		" Siguiente paso  Pedir al writer  [w]",
		"",
		" Writer · gasto sin dato de costo",
		"",
		"  # spec: aprobado",
		"  - verde: sin verde",
		"",
		" leyendo la tarjeta...",
	})
	d := tablero.Detalle{Slug: "tarjeta", Detalle: boardcmd.Detail{Card: c,
		Spec:     boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "- CA-1: solo uno\n"},
		Criteria: []boardcmd.CriterionTrace{{ID: "CA-1", Text: "solo uno", TracedBy: boardcmd.TracedTest}},
		Findings: []finding.Item{ttHallazgo("f1", finding.StatusOpen, "low", "", "sin archivo")}}}
	m.LeerDetalle(d)
	todo := strings.Join(ttCajaDe(t, "CA-556", m, "[0]").ttTexto(), "\n")
	for _, s := range []string{" Spec · 1 criterio · 0 sin prueba\n  + solo uno", " Hallazgos · 1 abierto\n  low - sin archivo",
		" Historia\n  sin historia todavia", " Narracion\n  sin sobres todavia"} {
		if !strings.Contains(todo, s) {
			t.Fatalf("CA-556: el documento dice %q:\n%s", s, todo)
		}
	}
	d.Detalle.Findings = nil
	d.Detalle.Spec = boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md"}
	d.Detalle.Criteria = nil
	m.LeerDetalle(d)
	todo = strings.Join(ttCajaDe(t, "CA-556", m, "[0]").ttTexto(), "\n")
	for _, s := range []string{" sin spec: .hoom/specs/tarjeta.md", " Hallazgos\n  sin hallazgos"} {
		if !strings.Contains(todo, s) {
			t.Fatalf("CA-556: el documento dice %q:\n%s", s, todo)
		}
	}
	if strings.Contains(todo, " Spec · ") {
		t.Fatalf("CA-556: sin spec no hay ' Spec · ':\n%s", todo)
	}
	m.LeerDetalle(tablero.Detalle{Slug: "tarjeta", Err: errors.New("disco roto")})
	todo = strings.Join(ttCajaDe(t, "CA-556", m, "[0]").ttTexto(), "\n")
	if !strings.Contains(todo, " no se pudo leer la tarjeta: disco roto") || strings.Contains(todo, " Historia") {
		t.Fatalf("CA-556: con Detalle.Err, [0] lo dice y no muestra nada viejo:\n%s", todo)
	}
	// un detalle de otra tarjeta se ignora
	m.LeerDetalle(tablero.Detalle{Slug: "otra", Detalle: boardcmd.Detail{Spec: boardcmd.SpecDetail{Exists: true, Markdown: "x"}}})
	if !strings.Contains(strings.Join(ttCajaDe(t, "CA-556", m, "[0]").ttTexto(), "\n"), "no se pudo leer la tarjeta") {
		t.Fatalf("CA-556: un Detalle de otra tarjeta se ignora")
	}
}

// CA-556, CA-566: la linea "Siguiente paso" en cada caso del contrato (el de
// doc.go, "The next step"): la accion con su tecla, o el texto de hoom
// siguiente sin tecla.
func TestCA556_CA566_SiguientePaso(t *testing.T) {
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
		quiere string
	}{
		{"en Hecho", enHecho, "la tarjeta ya esta en Hecho"},
		{"con running", conRunning, "el writer esta trabajando: espera a que termine"},
		{"interrumpida", conInterr, "el trabajo se corto: R reanuda o pide de nuevo"},
		{"en Tu aprobacion", ttNucTenancy(), "Aprobar spec  [A]"},
		{"en Tu aceptacion", enAcept, "Integrar  [I]"},
		{"en Tu aprobacion sin la accion", aprobSin, "en Tu aprobacion no hay aprobar el spec"},
		{"sin acciones", ttTarjetaCon(), "en Writer no hay nada que hacer ahora: el plain de la tarjeta"},
		{"primera de rol deshabilitada", ttTarjetaCon(writerOff), "se agoto el presupuesto de la tarjeta"},
		{"primera guardar", ttTarjetaCon(ttAccionGuardar(true, "", "a.go"), writer), "Guardar  [s]"},
		{"primera sin tecla", ttTarjetaCon(ttRelanzar(), writer), "en Writer no hay nada que hacer ahora: el plain de la tarjeta"},
		{"reanudar", ttPagosCaidos(), "el trabajo se corto: R reanuda o pide de nuevo"},
	}
	for _, c := range casos {
		m := ttModelo(ttConfig(200, 50, false), ttTablero(c.card))
		if c.nombre == "en Hecho" {
			m.Tecla("G", ttAhora)
			m.Tecla(tablero.TeclaEnter, ttAhora)
			m.Tecla("j", ttAhora)
		}
		got := ttCajaDe(t, "CA-556", m, "[0]").ttTexto()
		if len(got) < 2 || got[1] != " Siguiente paso  "+c.quiere {
			t.Fatalf("CA-556 (%s): la segunda linea de [0] es ' Siguiente paso  %s': %q", c.nombre, c.quiere, got)
		}
	}
}

// CA-556: las lineas mas anchas que [0] se parten en los espacios (tantas
// palabras como entren; una palabra mas larga que la linea se corta), cada
// continuacion con los espacios del comienzo de la linea que sigue; las de
// los eventos de la narracion se cortan.
func TestCA556_LasLineasLargasSeParten(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	c.Plain = "uno dos tres cuatro cinco seis siete ocho nueve diez once doce trece catorce quince dieciseis"
	c.Meter = []boardcmd.Segment{ttSeg("s", "spec", boardcmd.SegHecho, "un detalle largo de segmento que no entra en una sola linea de cincuenta")}
	m := ttModelo(ttConfig(80, 40, false), ttTablero(c)) // [0]: 54 celdas, 52 interiores
	ev := strings.Repeat("palabra ", 20)
	m.LeerDetalle(tablero.Detalle{Slug: "tarjeta", Detalle: boardcmd.Detail{Card: c,
		Work: []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "e", Role: "writer", Status: "entregable", Stage: "run", StartedAt: ttAhora}}},
		Sobre: tablero.Sobre{EnvelopeID: "e", Eventos: []runcmd.Event{ttEvento(-1, "text", ev)}}})
	got := ttCajaDe(t, "CA-556", m, "[0]").ttTexto()
	// el plain: la primera linea corta en un espacio antes de las 52 celdas;
	// las que siguen empiezan con el espacio del comienzo; ninguna palabra se
	// pierde ni se corta
	if !strings.HasPrefix(got[0], " Que falta?  uno dos") || ttCeldas(got[0]) > 52 || strings.HasSuffix(got[0], " ") {
		t.Fatalf("CA-556: la primera linea del plain se parte en un espacio dentro de las 52 celdas: %q", got[0])
	}
	var palabras []string
	i := 0
	for ; i < len(got) && !strings.HasPrefix(got[i], " Siguiente paso"); i++ {
		if i > 0 && (!strings.HasPrefix(got[i], " ") || strings.HasPrefix(got[i], "  ")) {
			t.Fatalf("CA-556: cada continuacion empieza con el espacio del comienzo de su linea: %q", got[i])
		}
		if ttCeldas(got[i]) > 52 {
			t.Fatalf("CA-556: ninguna linea pasa de las 52 celdas: %q", got[i])
		}
		palabras = append(palabras, strings.Fields(got[i])...)
	}
	if strings.Join(palabras, " ") != "Que falta? "+c.Plain || i < 2 {
		t.Fatalf("CA-556: el plain partido tiene todas sus palabras, en orden: %q", got[:i])
	}
	todo := strings.Join(got, "\n")
	if !strings.Contains(todo, "  # spec: un detalle largo de segmento que no entra\n  en una sola linea de cincuenta") {
		t.Fatalf("CA-556: la continuacion lleva los espacios del comienzo (dos en la tira):\n%s", todo)
	}
	var evento string
	for _, l := range got {
		if strings.HasPrefix(l, "  11:59:59 text ") {
			evento = l
		}
	}
	if evento == "" || ttCeldas(evento) != 52 || !strings.HasSuffix(evento, "~") {
		t.Fatalf("CA-556: la linea de un evento se corta al ancho de [0] (52 celdas, con ~): %q\n%s", evento, todo)
	}
}

// ---------------------------------------------------------------- CA-557: desplazar [0], ]c [c, e

// ttTarjetaLarga es infra-api con el detalle rico y veinte hallazgos mas,
// para que [0] se pueda desplazar mas alla de los criterios.
func ttTarjetaLarga() (boardcmd.Card, tablero.Detalle) {
	c, d := ttDetalleRico()
	for i := 0; i < 20; i++ {
		d.Detalle.Findings = append(d.Detalle.Findings, ttHallazgo(fmt.Sprintf("x%02d", i), finding.StatusOpen, "low", "", fmt.Sprintf("relleno %02d", i)))
	}
	return c, d
}

// CA-557: con el foco en [0] de una tarjeta, ↓ j ↑ k desplazan una linea,
// pgdn pgup su alto interior, g y G el principio y el final (el final deja la
// ultima linea abajo de todo); fuera de esas puntas no se pasa. Re-expresa
// CA-522.
func TestCA557_CA522_DesplazarCero(t *testing.T) {
	c, d := ttTarjetaLarga()
	m := ttModelo(ttConfig(80, 24, false), ttTablero(c))
	m.LeerDetalle(d)
	todo := ttCajaDe(t, "CA-557", m, "[0]").ttTexto() // la vista de arriba
	ttSinOrdenes(t, "CA-557", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	vista := func() []string { return ttCajaDe(t, "CA-557", m, "[0]").ttTexto() }
	if v := vista(); strings.Join(v, "\n") != strings.Join(todo, "\n") {
		t.Fatalf("CA-557: [0] empieza en su primera linea: %q", v)
	}
	alto := len(todo) // 14
	if alto != 14 {
		t.Fatalf("CA-550: a 80x24 [0] tiene 14 lineas interiores: %d", alto)
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
	m.Tecla(tablero.TeclaEnter, ttAhora)
	vista := func() []string { return ttCajaDe(t, "CA-557", m, "[0]").ttTexto() }
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
	// arriba: a 80x24 [0] tiene 14 lineas de 52 celdas; la linea del gasto y
	// la del doctor se parten, y los criterios quedan debajo de la vista:
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

// CA-557: e en [0] de una tarjeta suma al Registro "Abrir en el editor ·
// <archivo>:<linea>" con el comando del editor (CA-563).
func TestCA557_CA563_EditorAlRegistro(t *testing.T) {
	c, d := ttDetalleRico()
	cfg := ttConfig(120, 32, false)
	cfg.Editor = "nvim"
	m := ttModelo(cfg, ttTablero(c))
	m.LeerDetalle(d)
	m.Tecla(tablero.TeclaEnter, ttAhora)
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

// ---------------------------------------------------------------- CA-558: [3] y lo que corre

// CA-558: [3] tiene una fila por tarjeta con running (slug, rol y tiempo, con
// " de <limite>" si entra) y una por tarjeta interrumpida ("interrumpido",
// en rojo), en el orden del tablero; vacio, "nada corriendo"; el titulo
// cuenta sus filas. Re-expresa CA-519.
func TestCA558_CA519_FilasDeCorriendo(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	c3 := ttCajaDe(t, "CA-558", m, "[3]")
	if c3.Titulo != "[3] Corriendo 2" {
		t.Fatalf("CA-558: el titulo de [3] cuenta sus filas: %q", c3.Titulo)
	}
	ttIgualesAlPrincipio(t, "CA-558", "[3] a 120x32", c3.ttTexto(), []string{
		" > infra-api writer 12:41 de 45:00",
		"   pagos-caidos writer interrumpido",
	})
	// a 80 columnas el limite no entra y el slug se corta
	a := ttModeloDorado(ttConfig(80, 24, false))
	ttIgualesAlPrincipio(t, "CA-558", "[3] a 80x24", ttCajaDe(t, "CA-558", a, "[3]").ttTexto(), []string{
		" > infra-a~ writer 12:41",
		"   ~ writer interrumpido", // "interrumpido" deja 1 celda al slug: solo el signo del corte
	})
	// el reloj mueve el tiempo
	m.Tic(ttAhora.Add(3 * time.Second))
	if got := ttCajaDe(t, "CA-558", m, "[3]").ttTexto()[0]; got != " > infra-api writer 12:44 de 45:00" {
		t.Fatalf("CA-558: Tic mueve el tiempo corrido: %q", got)
	}
	// una hora o mas: h:mm:ss; rol vacio: agente
	c := ttCard("larga", "Larga", boardcmd.ColWriter, "p", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	c.Running = &boardcmd.Running{Stage: "verify", StartedAt: ttAhora.Add(-(time.Hour + 2*time.Minute + 3*time.Second)), TimeoutS: 5400}
	l := ttModelo(ttConfig(120, 32, false), ttTablero(c))
	if got := ttCajaDe(t, "CA-558", l, "[3]").ttTexto()[0]; got != " > larga agente 1:02:03 de 1:30:00" {
		t.Fatalf("CA-558: con una hora o mas h:mm:ss, y rol vacio 'agente': %q", got)
	}
	// vacio
	v := ttModelo(ttConfig(120, 32, false), ttTablero(ttBacklog("uno", "Uno")))
	c3 = ttCajaDe(t, "CA-558", v, "[3]")
	if c3.Titulo != "[3] Corriendo" || c3.ttTexto()[0] != " nada corriendo" || c3.Alto != 3 {
		t.Fatalf("CA-558: sin filas, '[3] Corriendo', ' nada corriendo' y 3 lineas: %+v", c3)
	}
	// con color, la interrumpida en rojo
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	r := ttModeloDorado(cfg)
	for _, x := range r.Pantalla() {
		if strings.Contains(ttSinSGR(x), "pagos-caidos writer interrumpido") && !ttConColor(ttRojo, x, "pagos-caidos writer interrumpido") {
			t.Fatalf("CA-558: la fila interrumpida va en rojo: %q", x)
		}
	}
}

// CA-558: [0] de lo que corre: la linea del rol, el paso, el tiempo, "narro
// hace <n>s" y el gasto; despues un evento por linea; vacio, "sin eventos
// todavia"; antes del detalle, "leyendo la tarjeta..."; de una
// interrumpida, la linea roja de la v1. Re-expresa CA-519 y CA-523.
func TestCA558_CA519_CA523_CeroDeLoQueCorre(t *testing.T) {
	m := ttModeloDorado(ttConfig(200, 50, false))
	m.Tecla("3", ttAhora)
	got := ttCajaDe(t, "CA-558", m, "[0]").ttTexto()
	ttIgualesAlPrincipio(t, "CA-558", "antes del detalle", got, []string{
		" writer · run 3/6 · 12:41 de 45:00 · narro hace 2s · 3.88 USD",
		" leyendo la tarjeta...",
	})
	m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{EnvelopeID: "env-api"}})
	ttIgualesAlPrincipio(t, "CA-558", "sin eventos", ttCajaDe(t, "CA-558", m, "[0]").ttTexto(), []string{
		" writer · run 3/6 · 12:41 de 45:00 · narro hace 2s · 3.88 USD",
		" sin eventos todavia",
	})
	m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{EnvelopeID: "env-api",
		Eventos: []runcmd.Event{ttEvento(-3, "text", "uno\ndos"), ttEvento(-2, "tool", "Bash go test")}}})
	ttIgualesAlPrincipio(t, "CA-558", "con eventos", ttCajaDe(t, "CA-558", m, "[0]").ttTexto(), []string{
		" writer · run 3/6 · 12:41 de 45:00 · narro hace 2s · 3.88 USD",
		" 11:59:57 text uno dos",
		" 11:59:58 tool Bash go test",
	})
	m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{Err: errors.New("permiso denegado")}})
	if todo := strings.Join(ttCajaDe(t, "CA-558", m, "[0]").ttTexto(), "\n"); !strings.Contains(todo, " no se pudo leer el sobre: permiso denegado") {
		t.Fatalf("CA-558: un Sobre con Err lo dice: %s", todo)
	}
	// la interrumpida
	m.Tecla("j", ttAhora)
	if got := ttCajaDe(t, "CA-558", m, "[0]").ttTexto()[0]; got != " interrumpido en el paso 3 de 5 (run): el writer ya no corre" {
		t.Fatalf("CA-558: [0] de una interrumpida dice la linea de la v1: %q", got)
	}
	// sin narracion ni costo: sin "narro" y "sin dato de costo"
	c := ttCard("sola", "Sola", boardcmd.ColWriter, "p", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	c.Running = &boardcmd.Running{Role: "writer", Stage: "spec", Step: 1, Steps: 7, StartedAt: ttAhora.Add(-(9*time.Minute + 5*time.Second))}
	s := ttModelo(ttConfig(200, 50, false), ttTablero(c))
	s.Tecla("3", ttAhora)
	if got := ttCajaDe(t, "CA-558", s, "[0]").ttTexto()[0]; got != " writer · spec 1/7 · 9:05 · sin dato de costo" {
		t.Fatalf("CA-558: sin limite, sin narracion y sin costo: %q", got)
	}
	if ti := ttCajaDe(t, "CA-558", s, "[0]").Titulo; ti != "[0] sola · writer" {
		t.Fatalf("CA-558: el titulo de [0] es '<slug> · <rol>': %q", ti)
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

// CA-558: la vista de lo que corre empieza al final y lo sigue mientras
// llegan eventos, salvo que la persona haya subido; G la vuelve al final y
// desde ahi lo sigue de nuevo.
func TestCA558_SigueElFinal(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, false)) // [0]: 14 lineas
	m.Tecla("3", ttAhora)
	leer := func(n int) {
		m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{EnvelopeID: "env-api", Eventos: ttEventos(n)}})
	}
	ultima := func() string { v := ttCajaDe(t, "CA-558", m, "[0]").ttTexto(); return v[len(v)-1] }
	leer(30)
	if !strings.HasSuffix(ultima(), "evento 29") {
		t.Fatalf("CA-558: empieza al final: la ultima linea es el ultimo evento: %q", ultima())
	}
	leer(31)
	if !strings.HasSuffix(ultima(), "evento 30") {
		t.Fatalf("CA-558: sigue el final mientras llegan eventos: %q", ultima())
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if m.Lugar() != tablero.LugarVerCorriendo {
		t.Fatalf("CA-558: enter en [3] pasa el foco a [0] de lo que corre: %q", m.Lugar())
	}
	m.Tecla("k", ttAhora)
	if !strings.HasSuffix(ultima(), "evento 29") {
		t.Fatalf("CA-558: k sube una linea: %q", ultima())
	}
	leer(32)
	if !strings.HasSuffix(ultima(), "evento 29") {
		t.Fatalf("CA-558: si la persona subio, la vista no sigue el final: %q", ultima())
	}
	m.Tecla("G", ttAhora)
	if !strings.HasSuffix(ultima(), "evento 31") {
		t.Fatalf("CA-558: G vuelve al final: %q", ultima())
	}
	leer(33)
	if !strings.HasSuffix(ultima(), "evento 32") {
		t.Fatalf("CA-558: desde el final sigue de nuevo: %q", ultima())
	}
}

// ---------------------------------------------------------------- CA-559: [4] y un hallazgo

// CA-559: [4] lista los hallazgos abiertos con los high primero, despues los
// medium, los low y cualquier otra severidad (cada grupo en el orden de la
// lectura); cada fila con la severidad rellena a 6, la tarea y
// <archivo>:<linea> sin carpetas; el titulo los cuenta y, con color, suma
// " · <k> high" en rojo. Re-expresa CA-524.
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
	c4 := ttCajaDe(t, "CA-559", m, "[4]")
	if c4.Titulo != "[4] Hallazgos 5" {
		t.Fatalf("CA-559: el titulo de [4] cuenta los abiertos: %q", c4.Titulo)
	}
	// interior 38: room = 28, tarea a 14, donde a 13
	fila := func(mk, sev, tarea, donde string) string {
		return fmt.Sprintf(" %s %-6s %-14s %s", mk, sev, tarea, donde)
	}
	quiere := []string{
		fila(">", "high", "infra-api", "h1.go:3"),
		fila(" ", "high", "infra-api", "h2.go:9"),
		fila(" ", "medium", "-", "-"),
		fila(" ", "low", "infra-api", "l1.go"),
	}
	got := c4.ttTexto()
	// h4 = 6 a 120x32: 4 lineas interiores (la quinta fila no entra)
	ttIgualesAlPrincipio(t, "CA-559", "[4] ordenado por severidad", got, quiere)
	if !strings.HasSuffix(c4.Abajo, "-1 de 5-+") {
		t.Fatalf("CA-552: [4] con 5 filas en 4 lineas dice '1 de 5': %q", c4.Abajo)
	}
	m.Tecla("4", ttAhora)
	m.Tecla("G", ttAhora)
	if e := ttElegidaEn(ttCajaDe(t, "CA-559", m, "[4]")); e != "critica infra-api      x.go" {
		t.Fatalf("CA-559: una severidad desconocida va despues de los low: %q", e)
	}
	// con color, la severidad en su color y " · 2 high" en rojo en el titulo
	cfg := ttConfig(120, 32, false)
	cfg.Color = true
	k := ttModelo(cfg, ttTableroPrincipal())
	k.LeerHallazgos(tablero.Hallazgos{Items: hs})
	var titulo string
	for _, l := range k.Pantalla() {
		if strings.Contains(ttSinSGR(l), "[4] Hallazgos 5 · 2 high") {
			titulo = l
		}
	}
	if titulo == "" || ttEstiloDe(titulo, " · 2 high") != "\x1b[31m" {
		t.Fatalf("CA-559: con color el titulo suma ' · 2 high' en rojo: %q", titulo)
	}
	// vacio y antes de la primera lectura
	v := ttModelo(ttConfig(120, 32, false), ttTableroPrincipal())
	if got := ttCajaDe(t, "CA-559", v, "[4]").ttTexto(); got[0] != " leyendo los hallazgos..." {
		t.Fatalf("CA-559: antes de la primera lectura [4] dice ' leyendo los hallazgos...': %q", got)
	}
	v.LeerHallazgos(tablero.Hallazgos{})
	if c := ttCajaDe(t, "CA-559", v, "[4]"); c.ttTexto()[0] != " sin hallazgos abiertos" || c.Titulo != "[4] Hallazgos" {
		t.Fatalf("CA-559: sin hallazgos, '[4] Hallazgos' y ' sin hallazgos abiertos': %+v", c)
	}
}

// CA-559: [0] de un hallazgo: la severidad, la lente y la tarea; la
// descripcion entera; el archivo con su linea; las lineas del archivo
// alrededor de la suya (K = min(ContextoArchivo, (H - F - 1) / 2)), con la
// del hallazgo marcada con > y los numeros alineados; y el comando para
// cerrarlo.
func TestCA559_CeroDeUnHallazgo(t *testing.T) {
	h := ttHallazgo("h-high", finding.StatusOpen, "high", "internal/api/router.go:41", "el router acepta rutas sin version")
	lineas := func(desde, hasta int) []string {
		var out []string
		for i := desde; i <= hasta; i++ {
			out = append(out, fmt.Sprintf("codigo %d", i))
		}
		return out
	}
	// 140x32: [0] de 94 celdas (92 interiores) y 22 lineas interiores; F = 6
	// lineas propias: K = min(10, (22 - 6 - 1) / 2) = 7
	m := ttModelo(ttConfig(140, 32, false), ttTableroPrincipal())
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h}})
	m.Tecla("4", ttAhora)
	if r, l := m.ArchivoElegido(); r != "internal/api/router.go" || l != 41 {
		t.Fatalf("CA-559: ArchivoElegido es la ruta y la linea: %q %d", r, l)
	}
	got := ttCajaDe(t, "CA-559", m, "[0]").ttTexto()
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
	got = ttCajaDe(t, "CA-559", m, "[0]").ttTexto()
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
	if !strings.Contains(strings.Join(ttCajaDe(t, "CA-559", m, "[0]").ttTexto(), "\n"), "\n > 41 codigo 41\n") {
		t.Fatalf("CA-559: una lectura que no es la de ArchivoElegido se ignora")
	}
	// los numeros se alinean a la derecha al mas ancho
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{ttHallazgo("h9", finding.StatusOpen, "low", "a.go:8", "d")}})
	m.LeerArchivo(tablero.Archivo{Ruta: "a.go", Linea: 8, Desde: 1, Lineas: lineas(1, 12)})
	todo := strings.Join(ttCajaDe(t, "CA-559", m, "[0]").ttTexto(), "\n")
	if !strings.Contains(todo, "\n    1 codigo 1\n") || !strings.Contains(todo, "\n >  8 codigo 8\n") || !strings.Contains(todo, "\n   12 codigo 12\n") {
		t.Fatalf("CA-559: los numeros van alineados a la derecha al mas ancho:\n%s", todo)
	}
}

// CA-559, casos limite: un hallazgo sin archivo (su [0] va sin lineas y e
// dice "el hallazgo no nombra un archivo"); el archivo que no se puede leer
// ([0] dice en rojo "no se pudo leer <archivo>: <error>"); una ruta fuera
// del proyecto por su forma o por un enlace (Fuera) (el mensaje de la v1, en
// rojo, sin leer ni abrir nada); la linea en la descripcion; linea 0.
func TestCA559_CasosLimiteDelHallazgo(t *testing.T) {
	cfg := ttConfig(140, 32, false)
	cfg.Color = true
	abrir := func(h finding.Item) *tablero.Modelo {
		m := ttModelo(cfg, ttTableroPrincipal())
		m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h}})
		m.Tecla("4", ttAhora)
		return m
	}
	// sin archivo
	m := abrir(ttHallazgo("s", finding.StatusOpen, "low", "", "no dice donde"))
	got := ttCajaDe(t, "CA-559", m, "[0]").ttTexto()
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
	// fuera por un enlace (Archivo.Fuera)
	m.LeerArchivo(tablero.Archivo{Ruta: "x/y.go", Linea: 3, Fuera: true})
	if !ttTiene(m, ` el archivo "x/y.go" esta fuera del proyecto`) {
		t.Fatalf("CA-559: Archivo.Fuera: [0] dice que esta fuera del proyecto:\n%s", ttPantalla(m))
	}
	// fuera por su forma: no se lee (ArchivoElegido "") ni se abre
	for _, file := range []string{"../fuera.go:3", "/etc/passwd", `C:\x.go`, `..\..\x.go`, `\\srv\r\x.go`} {
		m = abrir(ttHallazgo("f", finding.StatusOpen, "high", file, "d"))
		if r, _ := m.ArchivoElegido(); r != "" {
			t.Fatalf("CA-559: %q sale del proyecto por su forma: ArchivoElegido es \"\": %q", file, r)
		}
		ruta := strings.TrimSuffix(file, ":3")
		if !ttTiene(m, ` el archivo "`+strings.ReplaceAll(ruta, `\`, `\\`)+`" esta fuera del proyecto`) && !ttTiene(m, "esta fuera del proyecto") {
			t.Fatalf("CA-559: [0] de %q dice que esta fuera del proyecto:\n%s", file, ttPantalla(m))
		}
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
	todo := strings.Join(ttCajaDe(t, "CA-559", m, "[0]").ttTexto(), "\n")
	if !strings.Contains(todo, " b.go\n   1 uno\n   2 dos\n   3 tres") || strings.Contains(todo, " > ") {
		t.Fatalf("CA-559: con linea 0, las primeras lineas sin marca:\n%s", todo)
	}
}

// CA-559: e en [4] y en su [0] ordena AbrirEditor con el archivo relativo a
// la raiz y su linea (y suma la entrada del editor al Registro).
func TestCA559_EAbreElArchivoDelHallazgo(t *testing.T) {
	for _, enCero := range []bool{false, true} {
		m := ttModeloDorado(ttConfig(120, 32, false))
		m.Tecla("4", ttAhora)
		if enCero {
			m.Tecla(tablero.TeclaEnter, ttAhora)
		}
		ords := m.Tecla("e", ttAhora)
		if len(ords) != 1 {
			t.Fatalf("CA-559: e (en [0]: %v) da una orden: %#v", enCero, ords)
		}
		if a, ok := ords[0].(tablero.AbrirEditor); !ok || a.Archivo != "internal/api/router.go" || a.Linea != 41 {
			t.Fatalf("CA-559: e abre el archivo del hallazgo en su linea: %#v", ords[0])
		}
		if r := m.Registro(); len(r) != 1 || r[0].Texto != "Abrir en el editor · internal/api/router.go:41" || r[0].Comando != "vi +41 internal/api/router.go" {
			t.Fatalf("CA-563: la entrada del editor: %#v", r)
		}
	}
}

// ---------------------------------------------------------------- CA-560: [1] y su [0]

// CA-560: [1] muestra el proyecto y la rama, el check, el gasto de hoy y la
// tarjeta fijada, cada una en sus formas. Re-expresa CA-531.
func TestCA560_CA531_LasLineasDeProyecto(t *testing.T) {
	m := tablero.Nuevo(ttConfig(120, 32, false))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTableroPrincipal()})
	if got := ttCajaDe(t, "CA-560", m, "[1]").ttTexto(); strings.Join(got, "|") != " demo| leyendo el proyecto...|| sin tarjeta fijada" {
		t.Fatalf("CA-560: antes del primer Proyecto, ' demo', ' leyendo el proyecto...', '' y la cuarta: %q", got)
	}
	casos := []struct {
		nombre string
		p      func() tablero.Proyecto
		fijada string
		quiere []string
	}{
		{"verde", ttProyectoVerde, "nuc-tenancy", []string{" demo · rama main", " check VERDE · verify hace 2h", " hoy 3.88 USD en 4 trabajos", " fijada: nuc-tenancy"}},
		{"sin git, sin veredicto, sin trabajos", func() tablero.Proyecto {
			return tablero.Proyecto{Check: checkcmd.Result{Reason: checkcmd.ReasonNoVerdict}}
		}, "", []string{" demo · sin git", " sin veredicto todavia", " hoy sin trabajos", " sin tarjeta fijada"}},
		{"rojo por drift, con sin dato", func() tablero.Proyecto {
			p := ttProyectoVerde()
			p.Check = checkcmd.Result{OK: false, Reason: checkcmd.ReasonDrift, VerdictID: "v9"}
			p.Hoy = costcmd.Hoy{Runs: 1, CostoUSD: 0.5, SinCosto: 2}
			return p
		}, "", []string{" demo · rama main", " check ROJO: " + checkcmd.Motivo(checkcmd.Result{Reason: checkcmd.ReasonDrift, VerdictID: "v9"}),
			" hoy 0.50 USD en 1 trabajos · 2 sin dato", " sin tarjeta fijada"}},
	}
	for _, c := range casos {
		m := tablero.Nuevo(ttConfig(200, 32, false))
		m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTableroPrincipal(), Fijada: c.fijada})
		m.LeerProyecto(c.p())
		if got := ttCajaDe(t, "CA-560", m, "[1]").ttTexto(); strings.Join(got, "|") != strings.Join(c.quiere, "|") {
			t.Fatalf("CA-560 (%s): [1] dice\n %q\nse esperaba\n %q", c.nombre, got, c.quiere)
		}
	}
	// hace <t>: s por debajo del minuto, despues la unidad entera mas grande
	for d, q := range map[time.Duration]string{30 * time.Second: "30s", 90 * time.Second: "1m", 3599 * time.Second: "59m",
		time.Hour: "1h", 47 * time.Hour: "1d", 72 * time.Hour: "3d", -time.Minute: "0s"} {
		p := ttProyectoVerde()
		p.Veredicto.CreatedAt = ttAhora.Add(-d)
		m := ttModelo(ttConfig(200, 32, false), ttTableroPrincipal())
		m.LeerProyecto(p)
		if got := ttCajaDe(t, "CA-560", m, "[1]").ttTexto()[1]; got != " check VERDE · verify hace "+q {
			t.Fatalf("CA-560: un veredicto de hace %v dice 'hace %s': %q", d, q, got)
		}
	}
	// con color VERDE en verde y ROJO en rojo
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

// CA-560: [0] de [1] muestra el ultimo veredicto (id, hace cuanto, VERDE o
// ROJO, parcial, sus cuentas y su ruta), la linea del check y los problemas
// de hoom board doctor con su accion.
func TestCA560_CeroDelProyecto(t *testing.T) {
	p := ttProyectoVerde()
	p.Problemas = []boardcmd.Problem{
		{ID: boardcmd.ProbSpecEditado, Slug: "infra-api", What: "spec editado", Plain: "el spec cambio despues de aprobarlo", Action: "hoom aprobar infra-api"},
		{ID: boardcmd.ProbEvidenciaSinItem, Slug: "", What: "evidencia sin item", Plain: "", Action: "hoom item backfill"},
		{ID: boardcmd.ProbSinSpec, Slug: "viejo", What: "sin spec", Plain: "", Action: "hoom pedir arquitecto viejo"},
	}
	m := ttModelo(ttConfig(200, 50, false), ttTableroPrincipal())
	m.LeerProyecto(p)
	m.Tecla("1", ttAhora)
	c0 := ttCajaDe(t, "CA-560", m, "[0]")
	if c0.Titulo != "[0] Proyecto" {
		t.Fatalf("CA-560: el titulo de [0] es '[0] Proyecto': %q", c0.Titulo)
	}
	ttIgualesAlPrincipio(t, "CA-560", "[0] Proyecto", c0.ttTexto(), []string{
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
	ttIgualesAlPrincipio(t, "CA-560", "[0] sin veredicto ni problemas", ttCajaDe(t, "CA-560", m, "[0]").ttTexto(), []string{
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
	got := ttCajaDe(t, "CA-560", m, "[0]").ttTexto()
	if got[1] != "  v-r · hace 30s · ROJO · parcial" || got[2] != "  pass 1 · fail 2 · error 3 · absent 4" || got[3] != "  .hoom/verdicts/v-r.json" {
		t.Fatalf("CA-560: un veredicto rojo y parcial: %q", got)
	}
	// antes de la primera lectura
	n := ttModelo(ttConfig(200, 50, false), ttTableroPrincipal())
	n.Tecla("1", ttAhora)
	if got := ttCajaDe(t, "CA-560", n, "[0]").ttTexto(); got[0] != " leyendo el proyecto..." {
		t.Fatalf("CA-560: antes del primer Proyecto, [0] dice ' leyendo el proyecto...': %q", got)
	}
}

// CA-560: u ejecuta hoom usar <slug> enseguida, sin modal, y suma su entrada
// al Registro ("Fijar para la terminal"); [1] muestra la tarjeta fijada tras
// la siguiente lectura. Re-expresa CA-531.
func TestCA560_CA531_UFijaSinModal(t *testing.T) {
	m := ttModeloDorado(ttConfig(200, 40, false))
	m.Tecla("j", ttAhora) // pagos-caidos
	l := ttUnaLanzar(t, "CA-560", "u", m.Tecla("u", ttAhora.Add(time.Minute)))
	ttMismosArgs(t, "CA-560", "u", l.Args, []string{"usar", "pagos-caidos"})
	if l.ID != 1 || l.Slug != "pagos-caidos" || l.Rol != "" || m.Modal() != tablero.SinModal {
		t.Fatalf("CA-560: u es el Lanzar 1, de la tarjeta, sin rol, sin modal: %#v, modal %q", l, m.Modal())
	}
	r := m.Registro()
	if len(r) != 1 || r[0].Texto != "Fijar para la terminal · pagos-caidos · lanzando..." || r[0].Comando != "hoom usar pagos-caidos" {
		t.Fatalf("CA-563: u va al Registro como 'Fijar para la terminal' con su comando: %#v", r)
	}
	if got := ttCajaDe(t, "CA-560", m, "[1]").ttTexto()[3]; got != " fijada: nuc-tenancy" {
		t.Fatalf("CA-560: [1] cambia con la siguiente lectura, no antes: %q", got)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Minute), Tablero: ttTableroDorado(), Fijada: "pagos-caidos"})
	if got := ttCajaDe(t, "CA-560", m, "[1]").ttTexto()[3]; got != " fijada: pagos-caidos" {
		t.Fatalf("CA-560: tras la siguiente lectura [1] muestra la tarjeta fijada: %q", got)
	}
	// u desde [3] y desde [0] de una tarjeta, de 1 en 1
	m.Tecla("3", ttAhora)
	l = ttUnaLanzar(t, "CA-560", "u en [3]", m.Tecla("u", ttAhora))
	ttMismosArgs(t, "CA-560", "u en [3]", l.Args, []string{"usar", "infra-api"})
	m.Tecla(tablero.TeclaEnter, ttAhora)
	l = ttUnaLanzar(t, "CA-560", "u en [0]", m.Tecla("u", ttAhora))
	if l.ID != 3 {
		t.Fatalf("CA-528: cada orden tiene su numero, de 1 en 1: %d", l.ID)
	}
	if m.Corriendo() != 0 {
		t.Fatalf("CA-528: usar no es un trabajo de rol: Corriendo = %d", m.Corriendo())
	}
	// el fin de usar pone su ultima linea en la linea de mensajes
	m.Termino(tablero.FinProceso{ID: 3, Code: 0, Ultima: "tarjeta fijada para esta terminal: infra-api"}, ttAhora)
	ttDiceMensaje(t, "CA-560", "el fin de usar", m, false, "tarjeta fijada para esta terminal: infra-api")
	// sin tarjeta (Hecho, [1], [4]) u no hace nada
	for _, k := range []tablero.Tecla{"1", "4"} {
		m.Tecla(k, ttAhora)
		ttSinOrdenes(t, "CA-560", "u sin tarjeta en "+string(k), m.Tecla("u", ttAhora))
	}
}

// ---------------------------------------------------------------- CA-578: la tarjeta que desaparece

// CA-578: si la tarjeta elegida desaparece en una lectura, la seleccion pasa
// a la fila que ahora tiene su indice y [0] la muestra; si el foco estaba en
// [0] mostrandola, vuelve a [2] y la linea de mensajes dice "la tarjeta
// <slug> ya no existe" durante DuracionAviso. Re-expresa CA-535.
func TestCA578_CA535_LaElegidaDesaparece(t *testing.T) {
	b := ttTablero(ttBacklog("a", "A"), ttBacklog("b", "B"), ttBacklog("c", "C"))
	m := ttModelo(ttConfig(120, 32, false), b)
	m.Tecla("j", ttAhora) // b
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttBacklog("a", "A"), ttBacklog("c", "C"))})
	if m.Elegida() != "c" || ttSlugElegido(t, m) != "c" || ttCajaDe(t, "CA-578", m, "[0]").Titulo != "[0] c · C" {
		t.Fatalf("CA-578: la elegida desaparece: la seleccion pasa a la siguiente y [0] la muestra: %q\n%s", m.Elegida(), ttPantalla(m))
	}
	if ttMensaje(m) != "" {
		t.Fatalf("CA-578: con el foco en [2] no hay mensaje: %q", ttMensaje(m))
	}
	// la ultima tarjeta: la fila que ahora tiene su indice es la linea de
	// Hecho (una fila tambien), y [0] no muestra nada
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttBacklog("a", "A"))})
	if m.Elegida() != "" || ttSlugElegido(t, m) != "HECHO" {
		t.Fatalf("CA-578: la seleccion pasa a la fila siguiente (la de Hecho): %q\n%s", m.Elegida(), ttPantalla(m))
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero()})
	if m.Elegida() != "" || !ttTiene(m, `no hay tarjetas: hoom item add "<titulo>"`) {
		t.Fatalf("CA-578: sin ninguna tarjeta, [2] lo dice y nada esta elegido:\n%s", ttPantalla(m))
	}
	// con el foco en [0]
	m = ttModelo(ttConfig(120, 32, false), b)
	m.Tecla("j", ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttBacklog("a", "A"), ttBacklog("c", "C"))})
	if m.Foco() != tablero.PanelTarjetas {
		t.Fatalf("CA-578: con el foco en [0] mostrandola, el foco vuelve a [2]: %d", int(m.Foco()))
	}
	ttDiceMensaje(t, "CA-578", "la tarjeta que ya no existe", m, false, "la tarjeta b ya no existe")
	m.Tic(t0.Add(tablero.DuracionAviso + time.Second))
	if strings.Contains(ttMensaje(m), "ya no existe") {
		t.Fatalf("CA-578: el aviso dura DuracionAviso: %q", ttMensaje(m))
	}
	// un modal de esa tarjeta se cierra
	w := ttTarjetaCon(ttAccionWriter())
	m = ttModelo(ttConfig(120, 32, false), ttTablero(w, ttBacklog("otra", "Otra")))
	ttIrA(t, m, "tarjeta")
	m.Tecla("w", ttAhora)
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-566: w abre el modal Pedir: %q", m.Modal())
	}
	ords := m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttBacklog("otra", "Otra"))})
	if len(ttLanzares(ords)) != 0 || m.Modal() != tablero.SinModal || m.Foco() != tablero.PanelTarjetas {
		t.Fatalf("CA-578: un modal de la tarjeta que desaparece se cierra sin lanzar: %#v, modal %q", ords, m.Modal())
	}
	ttDiceMensaje(t, "CA-578", "el modal cerrado", m, false, "la tarjeta tarjeta ya no existe")
	// [3]: la que deja de correr sale de [3], sin mensaje
	r := ttModeloDorado(ttConfig(120, 32, false))
	r.Tecla("3", ttAhora)
	r.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttPagosCaidos(), ttNucConUntraced())})
	if e := ttElegidaEn(ttCajaDe(t, "CA-578", r, "[3]")); !strings.HasPrefix(e, "pagos-caidos ") {
		t.Fatalf("CA-578: la que deja de correr sale de [3] y la seleccion pasa a la de su indice: %q", e)
	}
}
