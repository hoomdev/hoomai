// La lista (CA-517, CA-518, CA-519, CA-520, CA-535): secciones, tira,
// segunda linea, seleccion, busqueda y tarjetas que desaparecen.
package tablero_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttEsFilaTarjeta dice si una linea del cuerpo (ASCII) es la fila de una
// tarjeta, y su slug: " " + M + " " + slug...
func ttEsFilaTarjeta(l string) (string, bool) {
	if len(l) < 4 || l[0] != ' ' || (l[1] != ' ' && l[1] != '>') || l[2] != ' ' || l[3] == ' ' {
		return "", false
	}
	f := strings.Fields(l[3:])
	if len(f) == 0 {
		return "", false
	}
	return f[0], true
}

// ttEsTituloSeccion dice si una linea es el titulo de una seccion: " " y una
// mayuscula.
func ttEsTituloSeccion(l string) bool {
	return len(l) > 2 && l[0] == ' ' && l[1] >= 'A' && l[1] <= 'Z'
}

// CA-517: el codigo del paquete (sin los tests) no contiene ningun id de
// columna como cadena. Guarda: el esqueleto ya lo cumple.
func TestCA517_SinIDsDeColumnaComoCadena(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", n))
		if err != nil {
			t.Fatal(err)
		}
		vistos++
		for _, id := range []string{`"backlog"`, `"tu-aprobacion"`, `"tu-aceptacion"`, `"hecho"`} {
			if strings.Contains(string(raw), id) {
				t.Fatalf("CA-517: %s contiene el id de columna %s como cadena: la TUI no tiene tabla de columnas", n, id)
			}
		}
	}
	if vistos == 0 {
		t.Fatal("CA-517: no encontre los archivos del paquete")
	}
}

// CA-517: las secciones de columna siguen el orden en que el tablero trae
// sus columnas (aqui uno que no es el de boardcmd.Columns) y sus titulos son
// los nombres que trae el tablero, en mayusculas.
func TestCA517_SeccionesEnElOrdenDelTablero(t *testing.T) {
	w := ttCard("w1", "Uno", boardcmd.ColWriter, "escribe", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	b := ttCard("b1", "Dos", boardcmd.ColBacklog, "falta", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	a := ttCard("a1", "Tres", boardcmd.ColArquitecto, "spec", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	tb := boardcmd.Board{Warnings: []string{}, Columns: []boardcmd.BoardColumn{
		{ID: boardcmd.ColWriter, Name: "Escritura", Cards: []boardcmd.Card{w}},
		{ID: boardcmd.ColReview, Name: "Review", Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColBacklog, Name: "Pendientes", Cards: []boardcmd.Card{b}},
		{ID: boardcmd.ColTestWriter, Name: "Test-writer", Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColArquitecto, Name: "Arquitecto", Cards: []boardcmd.Card{a}},
		{ID: boardcmd.ColTuAprobacion, Name: "Tu aprobacion", Human: true, Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColTuAceptacion, Name: "Tu aceptacion", Human: true, Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColHecho, Name: "Hecho", Cards: []boardcmd.Card{}},
	}}
	m := ttModelo(ttConfig(80, 24, false), tb)
	var titulos []string
	for _, l := range ttCuerpo(m) {
		if ttEsTituloSeccion(l) {
			titulos = append(titulos, l)
		}
	}
	quiere := []string{" ESCRITURA 1", " PENDIENTES 1", " ARQUITECTO 1", " HECHO 0 >"}
	if strings.Join(titulos, "|") != strings.Join(quiere, "|") {
		t.Fatalf("CA-517: las secciones van en el orden del tablero y con su nombre: %q, se esperaba %q\n%s", titulos, quiere, ttPantalla(m))
	}
}

// CA-518 (propiedad): para tableros al azar, cada tarjeta aparece una sola
// vez (en la primera seccion que le toca: decision, en curso o su columna),
// las secciones van en el orden del contrato con la cantidad de tarjetas
// que tienen, ninguna seccion vacia se muestra salvo Hecho, y la tira tiene
// un signo por segmento con ! en el primero que falta cuando espera tu
// decision.
func TestCA518_PropiedadCadaTarjetaUnaVez(t *testing.T) {
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
		m := ttModelo(ttConfig(80, 80, false), tb)
		// abre Hecho: G va a la ultima linea elegible (la de Hecho) y enter la abre
		m.Tecla("G", ttAhora)
		m.Tecla(tablero.TeclaEnter, ttAhora)

		// lo esperado
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
				quiere = append(quiere, seccion{fmt.Sprintf(" HECHO %d v", len(s)), s})
			} else if len(s) > 0 {
				quiere = append(quiere, seccion{fmt.Sprintf(" %s %d", strings.ToUpper(col.Name), len(s)), s})
			}
		}

		// lo que hay
		var hay []seccion
		for _, l := range ttCuerpo(m) {
			if ttEsTituloSeccion(l) {
				hay = append(hay, seccion{titulo: l})
				continue
			}
			if slug, ok := ttEsFilaTarjeta(l); ok && len(hay) > 0 {
				hay[len(hay)-1].slugs = append(hay[len(hay)-1].slugs, slug)
				campos := strings.Fields(l[3:])
				if len(campos) < 2 || campos[1] != tiraDe[slug] {
					t.Logf("semilla %d: la tira de %s es %q, se esperaba %q (linea %q)", semilla, slug, campos, tiraDe[slug], l)
					return false
				}
			}
		}
		if fmt.Sprint(hay) != fmt.Sprint(quiere) {
			t.Logf("semilla %d:\n hay    %v\n quiere %v\n%s", semilla, hay, quiere, ttPantalla(m))
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 150}); err != nil {
		t.Fatalf("CA-518: %v", err)
	}
}

// CA-518: una tira mas larga que el lugar despues del slug se corta: los
// signos que entran menos uno, "~" y sin plain; el plain va solo si le
// quedan al menos 8 celdas; un slug de mas de 24 va entero, sin relleno.
func TestCA518_CorteDeLaTira(t *testing.T) {
	segs := func(n int) []boardcmd.Segment {
		var out []boardcmd.Segment
		for i := 0; i < n; i++ {
			out = append(out, ttSeg(fmt.Sprintf("s%d", i), "s", boardcmd.SegHecho, "d"))
		}
		return out
	}
	casos := []struct {
		nombre string
		slug   string
		n      int
		quiere string
	}{
		// 80 - 28 = 52 celdas para la tira: 60 signos no entran
		{"tira cortada", "larga", 60, " > larga" + strings.Repeat(" ", 19) + " " + strings.Repeat("#", 51) + "~"},
		// 42 signos: quedan 80-28-42-2 = 8 celdas para el plain (cortado)
		{"plain con 8 celdas", "justa", 42, " > " + "justa" + strings.Repeat(" ", 19) + " " + strings.Repeat("#", 42) + "  el plai~"},
		// 43 signos: quedan 7 celdas, sin plain
		{"sin lugar para el plain", "corta", 43, " > " + "corta" + strings.Repeat(" ", 19) + " " + strings.Repeat("#", 43)},
		// un slug de 29 va entero, sin relleno
		{"slug largo", "un-slug-que-pasa-de-24-celdas", 2, " > un-slug-que-pasa-de-24-celdas ##  el plain de la tarjeta"},
	}
	for _, c := range casos {
		card := ttCard(c.slug, "T", boardcmd.ColBacklog, "el plain de la tarjeta", segs(c.n)...)
		m := ttModelo(ttConfig(80, 24, false), ttTablero(card))
		if got := ttLinea(m, 3); got != c.quiere {
			t.Fatalf("CA-518 (%s): la fila es\n %q\nse esperaba\n %q", c.nombre, got, c.quiere)
		}
	}
}

// CA-519: la segunda linea de una tarjeta en curso con cada variante del
// contrato: rol vacio es "agente", paso sin steps es el stage solo, tiempo de
// una hora o mas en h:mm:ss, sin tiempo limite no hay " de ", sin
// last_event_at no hay "narro", sin costo "sin dato de costo".
func TestCA519_SegundaLineaVariantes(t *testing.T) {
	casos := []struct {
		nombre  string
		running boardcmd.Running
		costo   *float64
		quiere  string
	}{
		{"la del contrato", boardcmd.Running{Role: "test-writer", Stage: "run", Step: 3, Steps: 6,
			StartedAt: ttAhora.Add(-(12*time.Minute + 41*time.Second)), TimeoutS: 2700, LastEventAt: ttT(ttAhora.Add(-2 * time.Second))},
			ttF(3.88), "test-writer · run 3/6 · 12:41 de 45:00 · narro hace 2s · 3.88 USD"},
		{"agente, stage solo, h:mm:ss, sin narracion ni costo", boardcmd.Running{Stage: "verify",
			StartedAt: ttAhora.Add(-(time.Hour + 2*time.Minute + 3*time.Second)), TimeoutS: 5400},
			nil, "agente · verify · 1:02:03 de 1:30:00 · sin dato de costo"},
		{"sin tiempo limite, narro hace 75s, dos decimales", boardcmd.Running{Role: "writer", Stage: "spec", Step: 1, Steps: 7,
			StartedAt: ttAhora.Add(-(9*time.Minute + 5*time.Second)), LastEventAt: ttT(ttAhora.Add(-75 * time.Second))},
			ttF(0.5), "writer · spec 1/7 · 9:05 · narro hace 75s · 0.50 USD"},
	}
	for _, c := range casos {
		card := ttCard("corre", "T", boardcmd.ColWriter, "p", ttSeg("s", "s", boardcmd.SegHecho, "d"))
		r := c.running
		card.Running = &r
		card.Spend.CostUSD = c.costo
		m := ttModelo(ttConfig(120, 24, false), ttTablero(card))
		_, n := ttLineaCon(m, " corre ")
		if n == 0 {
			t.Fatalf("CA-519 (%s): la tarjeta en curso esta en la lista:\n%s", c.nombre, ttPantalla(m))
		}
		if got := ttLinea(m, n+1); got != strings.Repeat(" ", 28)+c.quiere {
			t.Fatalf("CA-519 (%s): la segunda linea es\n %q\nse esperaba\n %q", c.nombre, got, strings.Repeat(" ", 28)+c.quiere)
		}
	}
}

// CA-519: los tiempos en pantalla se mueven con el reloj (Tic).
func TestCA519_TicMueveLosTiempos(t *testing.T) {
	m := ttModelo(ttConfig(100, 24, false), ttTablero(ttInfraAPI(true)))
	m.Tic(ttAhora.Add(3 * time.Second))
	if l, _ := ttLineaCon(m, "writer · run 3/6 · "); l != strings.Repeat(" ", 28)+"writer · run 3/6 · 12:44 de 45:00 · narro hace 5s · 3.88 USD" {
		t.Fatalf("CA-519: tres segundos despues la segunda linea dice 12:44 y narro hace 5s: %q\n%s", l, ttPantalla(m))
	}
}

// CA-518: Hecho aparece cerrada con su cantidad y enter sobre su linea la
// abre (sus tarjetas la siguen) y la cierra.
func TestCA518_HechoSeAbreYSeCierra(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTablero(ttBacklog("uno", "Uno"), ttViejo(1), ttViejo(2), ttViejo(3)))
	if _, n := ttLineaCon(m, " HECHO 3 >"); n == 0 {
		t.Fatalf("CA-518: Hecho esta cerrada con su cantidad:\n%s", ttPantalla(m))
	}
	if _, n := ttLineaCon(m, "viejo-1"); n != 0 {
		t.Fatalf("CA-518: las tarjetas de Hecho cerrada no se muestran:\n%s", ttPantalla(m))
	}
	ttSinOrdenes(t, "CA-518", "G y enter sobre Hecho", ttTeclas(m, "G", tablero.TeclaEnter))
	if m.Vista() != tablero.VistaLista {
		t.Fatalf("CA-518: enter sobre la linea de Hecho no abre ninguna tarjeta, la vista es %q", m.Vista())
	}
	_, n := ttLineaCon(m, " HECHO 3 v")
	if n == 0 {
		t.Fatalf("CA-518: enter abre Hecho (' HECHO 3 v'):\n%s", ttPantalla(m))
	}
	for i, s := range []string{"viejo-1", "viejo-2", "viejo-3"} {
		if slug, ok := ttEsFilaTarjeta(ttLinea(m, n+1+i)); !ok || slug != s {
			t.Fatalf("CA-518: abierta, Hecho sigue con sus tarjetas en orden; linea %d = %q\n%s", n+1+i, ttLinea(m, n+1+i), ttPantalla(m))
		}
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if _, n := ttLineaCon(m, " HECHO 3 >"); n == 0 {
		t.Fatalf("CA-518: enter otra vez cierra Hecho:\n%s", ttPantalla(m))
	}
}

// CA-520: j, k, las flechas, g y G mueven la seleccion; la seleccion pasa por
// la linea de Hecho; enter, l y → abren la tarjeta; esc, h y ← vuelven.
func TestCA520_MoverAbrirVolver(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTablero(ttBacklog("a", "A"), ttBacklog("b", "B"), ttBacklog("c", "C"), ttViejo(1)))
	paso := func(k tablero.Tecla, quiere string) {
		t.Helper()
		ttSinOrdenes(t, "CA-520", string(k), m.Tecla(k, ttAhora))
		if got := ttSeleccionada(m); got != quiere {
			t.Fatalf("CA-520: tras %q la seleccion esta en %q, se esperaba %q\n%s", k, got, quiere, ttPantalla(m))
		}
	}
	if got := ttSeleccionada(m); got != "a" {
		t.Fatalf("CA-520: al empezar la seleccion esta en la primera tarjeta, esta en %q\n%s", got, ttPantalla(m))
	}
	paso("j", "b")
	paso(tablero.TeclaAbajo, "c")
	paso("j", "") // la linea de Hecho
	paso("k", "c")
	paso(tablero.TeclaArriba, "b")
	paso("G", "")
	paso("g", "a")
	paso("k", "a") // ya en la primera
	for _, abrir := range []tablero.Tecla{tablero.TeclaEnter, "l", tablero.TeclaDer} {
		for _, volver := range []tablero.Tecla{tablero.TeclaEsc, "h", tablero.TeclaIzq} {
			ttSinOrdenes(t, "CA-520", string(abrir), m.Tecla(abrir, ttAhora))
			if v := m.Vista(); v != tablero.VistaResumen {
				t.Fatalf("CA-520: %q abre la tarjeta en Resumen, la vista es %q", abrir, v)
			}
			if s, h := m.Abierta(); s != "a" || h {
				t.Fatalf("CA-520: la tarjeta abierta es a (sin Historia), es %q %v", s, h)
			}
			if got := ttLinea(m, 1); got != " a · A" {
				t.Fatalf("CA-521: el titulo de la tarjeta abierta es ' <slug> · <titulo>', es %q", got)
			}
			ttSinOrdenes(t, "CA-520", string(volver), m.Tecla(volver, ttAhora))
			if v := m.Vista(); v != tablero.VistaLista {
				t.Fatalf("CA-520: %q vuelve a la lista, la vista es %q", volver, v)
			}
			if s, _ := m.Abierta(); s != "" {
				t.Fatalf("CA-520: en la lista no hay tarjeta abierta, Abierta = %q", s)
			}
			if got := ttSeleccionada(m); got != "a" {
				t.Fatalf("CA-520: al volver la seleccion sigue en a, esta en %q", got)
			}
		}
	}
}

// CA-520: el cuerpo se desplaza lo minimo necesario para que se vea la
// linea elegida.
func TestCA520_DesplazamientoMinimo(t *testing.T) {
	var cards []boardcmd.Card
	for i := 0; i < 30; i++ {
		cards = append(cards, ttCard(fmt.Sprintf("c-%02d", i), "T", boardcmd.ColBacklog, "p", ttSeg("s", "s", boardcmd.SegFalta, "d")))
	}
	m := ttModelo(ttConfig(80, 15, false), ttTablero(cards...))
	for i := 0; i < 20; i++ {
		m.Tecla("j", ttAhora)
	}
	cuerpo := ttCuerpo(m) // 11 lineas
	if len(cuerpo) != 11 || !strings.HasPrefix(cuerpo[10], " > c-20 ") || !strings.HasPrefix(cuerpo[0], "   c-10 ") {
		t.Fatalf("CA-520: con c-20 elegida la ultima linea del cuerpo es la suya y la primera c-10:\n%s", strings.Join(cuerpo, "\n"))
	}
	for i := 0; i < 5; i++ {
		m.Tecla("k", ttAhora)
	}
	cuerpo = ttCuerpo(m)
	if !strings.HasPrefix(cuerpo[0], "   c-10 ") || !strings.HasPrefix(cuerpo[5], " > c-15 ") {
		t.Fatalf("CA-520: c-15 ya se ve: el cuerpo no se mueve:\n%s", strings.Join(cuerpo, "\n"))
	}
	for i := 0; i < 10; i++ {
		m.Tecla("k", ttAhora)
	}
	cuerpo = ttCuerpo(m)
	if !strings.HasPrefix(cuerpo[0], " > c-05 ") {
		t.Fatalf("CA-520: con c-05 elegida arriba, el cuerpo empieza en ella:\n%s", strings.Join(cuerpo, "\n"))
	}
}

func ttTableroBusqueda() boardcmd.Board {
	seg := ttSeg("s", "s", boardcmd.SegFalta, "d")
	return ttTablero(
		ttCard("accion-rapida", "Rapida", boardcmd.ColBacklog, "p", seg),
		ttCard("lote", "Acción en lote", boardcmd.ColBacklog, "p", seg),
		ttCard("otra", "Nada que ver", boardcmd.ColBacklog, "p", seg),
		ttCard("config", "Configuración del año", boardcmd.ColArquitecto, "p", seg),
	)
}

func ttVisibles(m *tablero.Modelo) []string {
	var out []string
	for _, l := range ttCuerpo(m) {
		if s, ok := ttEsFilaTarjeta(l); ok {
			out = append(out, s)
		}
	}
	return out
}

// CA-520: / filtra por slug o titulo sin distinguir mayusculas ni acentos;
// las secciones cuentan lo que queda; esc lo cierra y vuelve la lista entera.
func TestCA520_BusquedaSinMayusculasNiAcentos(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTableroBusqueda())
	ttSinOrdenes(t, "CA-520", "/", ttTeclas(m, "/"))
	if v := m.Vista(); v != tablero.VistaBusqueda {
		t.Fatalf("CA-520: / abre la busqueda, la vista es %q", v)
	}
	ttSinOrdenes(t, "CA-520", "ACCIÓN", ttEscribir(m, "ACCIÓN"))
	if got := strings.Join(ttVisibles(m), ","); got != "accion-rapida,lote" {
		t.Fatalf("CA-520: 'ACCIÓN' encuentra accion-rapida (slug) y lote (titulo 'Acción en lote'), se ven %q\n%s", got, ttPantalla(m))
	}
	if _, n := ttLineaCon(m, " BACKLOG 2"); n == 0 {
		t.Fatalf("CA-520: la seccion cuenta las tarjetas que quedan (BACKLOG 2):\n%s", ttPantalla(m))
	}
	if _, n := ttLineaCon(m, " ARQUITECTO"); n != 0 {
		t.Fatalf("CA-520: una seccion sin tarjetas que queden no se muestra:\n%s", ttPantalla(m))
	}
	if _, n := ttLineaCon(m, " HECHO 0 >"); n == 0 {
		t.Fatalf("CA-520: Hecho conserva su linea en la busqueda:\n%s", ttPantalla(m))
	}
	if b := ttBarra1(m); !strings.HasPrefix(b, " /ACCIÓN_") {
		t.Fatalf("CA-520: con la busqueda abierta el modo es '/' + lo escrito + el cursor: %q", b)
	}
	ttSinOrdenes(t, "CA-520", "esc", ttTeclas(m, tablero.TeclaEsc))
	if v := m.Vista(); v != tablero.VistaLista {
		t.Fatalf("CA-520: esc cierra la busqueda, la vista es %q", v)
	}
	if got := strings.Join(ttVisibles(m), ","); got != "accion-rapida,lote,otra,config" {
		t.Fatalf("CA-520: esc vuelve la lista entera, se ven %q", got)
	}
	if b := ttBarra1(m); !strings.HasPrefix(b, " NORMAL · ") {
		t.Fatalf("CA-520: cerrada la busqueda el modo vuelve a NORMAL: %q", b)
	}
	// la ñ y la ó del titulo, buscadas sin acento
	m.Tecla("/", ttAhora)
	ttEscribir(m, "configuracion del ano")
	if got := strings.Join(ttVisibles(m), ","); got != "config" {
		t.Fatalf("CA-520: 'configuracion del ano' encuentra 'Configuración del año', se ven %q", got)
	}
	// borrar: backspace deja "configuracion del an"
	m.Tecla(tablero.TeclaBorrar, ttAhora)
	if b := ttBarra1(m); !strings.HasPrefix(b, " /configuracion del an_") {
		t.Fatalf("CA-520: backspace borra lo ultimo escrito: %q", b)
	}
}

// CA-520: en la busqueda las letras se escriben (j no mueve), ↓/↑ mueven y
// enter abre la tarjeta elegida y cierra la busqueda.
func TestCA520_BusquedaMueveYAbre(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTableroBusqueda())
	ttTeclas(m, "/", "j")
	if b := ttBarra1(m); !strings.HasPrefix(b, " /j_") {
		t.Fatalf("CA-520: en la busqueda j es texto: %q", b)
	}
	m.Tecla(tablero.TeclaBorrar, ttAhora)
	ttEscribir(m, "o")
	if got := strings.Join(ttVisibles(m), ","); got != "accion-rapida,lote,otra,config" {
		t.Fatalf("CA-520: 'o' esta en las cuatro, se ven %q", got)
	}
	ttTeclas(m, tablero.TeclaAbajo, tablero.TeclaAbajo, tablero.TeclaArriba)
	if got := ttSeleccionada(m); got != "lote" {
		t.Fatalf("CA-520: ↓ ↓ ↑ deja la seleccion en la segunda, esta en %q\n%s", got, ttPantalla(m))
	}
	ttSinOrdenes(t, "CA-520", "enter", ttTeclas(m, tablero.TeclaEnter))
	if s, _ := m.Abierta(); s != "lote" || m.Vista() != tablero.VistaResumen {
		t.Fatalf("CA-520: enter abre la tarjeta elegida: abierta %q, vista %q", s, m.Vista())
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	if got := strings.Join(ttVisibles(m), ","); m.Vista() != tablero.VistaLista || got != "accion-rapida,lote,otra,config" {
		t.Fatalf("CA-520: enter cerro la busqueda y dejo el filtro: al volver se ve la lista entera, se ven %q", got)
	}
}

// CA-535: la tarjeta elegida desaparece en una lectura: la seleccion pasa a
// la linea que ahora tiene su indice (la ultima si hay menos). La seleccion
// sigue a su tarjeta cuando otra aparece antes.
func TestCA535_LaElegidaDesaparece(t *testing.T) {
	a, b, c := ttBacklog("a", "A"), ttBacklog("b", "B"), ttBacklog("c", "C")
	m := ttModelo(ttConfig(80, 24, false), ttTablero(a, b, c))
	m.Tecla("j", ttAhora)
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttBacklog("x", "X"), a, b, c)})
	if got := ttSeleccionada(m); got != "b" {
		t.Fatalf("CA-535: la seleccion se queda en su tarjeta aunque otra aparezca antes, esta en %q", got)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(a, b, c)})
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(a, c)})
	if got := ttSeleccionada(m); got != "c" {
		t.Fatalf("CA-535: sin b la seleccion pasa a la que ahora tiene su indice (c), esta en %q\n%s", got, ttPantalla(m))
	}
	if strings.Contains(ttBarra1(m), "ya no existe") {
		t.Fatalf("CA-535: el aviso es solo para una tarjeta abierta: %q", ttBarra1(m))
	}
	// sin c, el indice 1 es la linea de Hecho (o, si el indice cuenta solo
	// tarjetas, la ultima: a); nunca una tarjeta que ya no esta
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(a)})
	if got := ttSeleccionada(m); got != "a" && got != "" {
		t.Fatalf("CA-535: con menos lineas la seleccion pasa a la ultima, esta en %q\n%s", got, ttPantalla(m))
	}
	if _, n := ttLineaCon(m, " > c "); n != 0 {
		t.Fatalf("CA-535: c ya no esta en la lista:\n%s", ttPantalla(m))
	}
}

// CA-535: la tarjeta abierta desaparece: se vuelve a la lista con "la
// tarjeta <slug> ya no existe" durante DuracionAviso.
func TestCA535_LaAbiertaDesaparece(t *testing.T) {
	a, b := ttBacklog("a", "A"), ttBacklog("b", "B")
	m := ttModelo(ttConfig(80, 24, false), ttTablero(a, b))
	m.Tecla("j", ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if s, _ := m.Abierta(); s != "b" {
		t.Fatalf("CA-535: b esta abierta, Abierta = %q", s)
	}
	m.Tecla("2", ttAhora) // en otra pestana, lo mismo
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(a)})
	if v := m.Vista(); v != tablero.VistaLista {
		t.Fatalf("CA-535: la tarjeta abierta ya no esta: se vuelve a la lista, la vista es %q", v)
	}
	if s, _ := m.Abierta(); s != "" {
		t.Fatalf("CA-535: ya no hay tarjeta abierta, Abierta = %q", s)
	}
	if b := ttBarra1(m); !strings.HasSuffix(b, " · la tarjeta b ya no existe") {
		t.Fatalf("CA-535: la barra dice 'la tarjeta b ya no existe': %q", b)
	}
	m.Tic(ttAhora.Add(tablero.DuracionAviso - time.Second))
	if b := ttBarra1(m); !strings.Contains(b, "la tarjeta b ya no existe") {
		t.Fatalf("CA-535: el aviso dura DuracionAviso: a los 4s sigue: %q", b)
	}
	m.Tic(ttAhora.Add(tablero.DuracionAviso + time.Second))
	if b := ttBarra1(m); strings.Contains(b, "ya no existe") || b != " NORMAL · sin tarjeta fijada" {
		t.Fatalf("CA-535: pasado DuracionAviso el aviso se va: %q", b)
	}
}
