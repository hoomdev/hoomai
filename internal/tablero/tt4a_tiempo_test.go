package tablero_test

// CA-708 (y CA-711 en la fila de CORRIENDO): la barra del tiempo de AHORA y
// el limite de un running. Se escriben desde doc.go: "rol is running.role
// ... tiempo the time since running.started_at and limite
// running.timeout_s (when it is above 0 and fits a duration: a negative or
// huge one, from a damaged sidecar, is no limit, and the board does not
// crash on it)" y "The bar (decision on the rounding) has 20 cells: f = 20
// * elapsed / limit (integer, from 0 to 20)".
//
// Cada caso se dibuja en la lista (la fila de CORRIENDO, ancha y angosta),
// en la caja TU SIGUIENTE PASO, y en el detalle de la tarjeta antes y
// despues de su Detalle (AHORA y la Narracion, la parte G). La pantalla
// tiene que salir sin panic y con lo que dice doc.go.

import (
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// tv3aTiempo dice si el texto del tiempo de una linea es el esperado.
type tv3aTiempo func(string) bool

// tv3aExacto es el tiempo que tiene que ser exactamente s.
func tv3aExacto(s string) tv3aTiempo { return func(x string) bool { return x == s } }

// tv3aRelojLargo es un tiempo de una hora o mas, como lo escribe la v1
// (h:mm:ss), con las horas que sean.
var tv3aRelojLargo tv3aTiempo = regexp.MustCompile(`^[0-9]+:[0-9]{2}:[0-9]{2}$`).MatchString

// tv3aUnaPalabra es un tiempo que doc.go no fija (un started_at en el
// futuro): un solo texto, sin espacios (ni " de <limite>" metido adentro).
func tv3aUnaPalabra(x string) bool { return x != "" && !strings.Contains(x, " ") }

// tv3aEncaja dice si got es antes + un tiempo que cumple tiempo + despues.
func tv3aEncaja(got, antes, despues string, tiempo tv3aTiempo) bool {
	if len(got) < len(antes)+len(despues) || !strings.HasPrefix(got, antes) || !strings.HasSuffix(got, despues) {
		return false
	}
	return tiempo(got[len(antes) : len(got)-len(despues)])
}

// tv3aCorridaCaso es un running con su started_at y su timeout_s, y lo que
// doc.go dice que se ve: f celdas hechas de la barra (-1: sin barra), el
// tiempo y el limite (""; sin limite).
type tv3aCorridaCaso struct {
	nombre     string
	inicio     time.Time
	limite     int
	f          int
	tiempo     tv3aTiempo
	tiempoDice string // para los mensajes
	limiteDice string
}

// tv3aCorrida es la tarjeta "corre" en Writer con el running del caso.
func tv3aCorrida(k tv3aCorridaCaso) boardcmd.Card {
	c := tv3aCorre(tv3aEn("corre", "Corre", boardcmd.ColWriter), "writer", 0, k.limite)
	c.Running.StartedAt = k.inicio
	return c
}

// tv3aDetalleCorrida es el Detalle de la tarjeta con running: su sobre en
// curso, sin eventos (la Narracion es solo su primera linea).
func tv3aDetalleCorrida(c boardcmd.Card) tablero.Detalle {
	return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c},
		Sobre: tablero.Sobre{EnvelopeID: c.Running.EnvelopeID, RunID: c.Running.RunID}}
}

// tv3aNarracionDe es la linea que sigue al titulo de la parte G del
// documento d (" Narración", " Narracion" en ASCII).
func tv3aNarracionDe(t *testing.T, ca string, utf bool, d []string) string {
	t.Helper()
	titulo := " Narracion"
	if utf {
		titulo = " Narración"
	}
	for i, l := range d {
		if l == titulo {
			return tv3aDe(d, i+1)
		}
	}
	t.Fatalf("%s: con el Detalle el documento tiene la parte %q a la vista:\n%s", ca, titulo, strings.Join(d, "\n"))
	return ""
}

// tv3aRevisarCorrida dibuja el caso a 200x80 (con color) y a 80x60, en
// UTF-8 y en ASCII, y revisa la barra, la fila, la caja y la Narracion.
func tv3aRevisarCorrida(t *testing.T, k tv3aCorridaCaso) {
	t.Helper()
	de := ""
	if k.limiteDice != "" {
		de = " de " + k.limiteDice
	}
	for _, utf := range []bool{true, false} {
		ca := "CA-708 (" + k.nombre + ", utf=" + map[bool]string{true: "si", false: "no"}[utf] + ")"
		s := tv3aSig(utf)
		card := tv3aCorrida(k)

		// ---- con dos zonas: 200x80, la lista de 80, el detalle de 120
		m := tv3aGrande(utf, true, ttTablero(card))
		if m.Grupo("corre") != tablero.GrupoCorriendo {
			t.Fatalf("%s (guarda): con running la tarjeta esta en CORRIENDO, esta en %q", ca, m.Grupo("corre"))
		}
		barra := func(cuando string) {
			t.Helper()
			d := tv3aDoc(t, ca, m, 200)
			if k.f < 0 {
				if tv3aDe(d, 3) != "" {
					t.Fatalf("%s, %s: sin limite AHORA no tiene barra: despues de su linea gris viene la linea vacia: %q\n%s",
						ca, cuando, tv3aDe(d, 3), tv3aLineas(m))
				}
				for _, l := range d {
					if strings.HasPrefix(l, " tiempo ") {
						t.Fatalf("%s, %s: sin limite no hay barra del tiempo: %q", ca, cuando, l)
					}
				}
				return
			}
			antes := " tiempo " + strings.Repeat(s.BarraLlena, k.f) + strings.Repeat(s.BarraVacia, 20-k.f) + " "
			if !tv3aEncaja(tv3aDe(d, 3), antes, de, k.tiempo) {
				t.Fatalf("%s, %s: la cuarta linea de AHORA es la barra con %d de 20 hechas, %q + %s + %q; es %q\n%s",
					ca, cuando, k.f, antes, k.tiempoDice, de, tv3aDe(d, 3), tv3aLineas(m))
			}
			// la barra en la pantalla: la linea 10, celdas 89 a 108
			if k.f > 0 {
				tv3aSgrTramo(t, ca, cuando+": lo corrido de la barra (cian)", m, 10, 89, 89+k.f, "36")
			}
			if k.f < 20 {
				tv3aSgrTramo(t, ca, cuando+": lo que queda de la barra (gris)", m, 10, 89+k.f, 109, "90")
			}
		}
		barra("antes del Detalle")
		li := tv3aLista(t, ca, m, 200).tv3aTexto()
		if !tv3aEncaja(tv3aDe(li, 2), "   writer · ", de, k.tiempo) {
			t.Fatalf("%s: la segunda linea de la fila de CORRIENDO es \"   writer · \" + %s + %q; es %q\n%s",
				ca, k.tiempoDice, de, tv3aDe(li, 2), tv3aLineas(m))
		}
		caja := tv3aCajaPaso(t, ca, m, 200).tv3aTexto()
		if antes := " corriendo: " + tv3aQ(utf, "Corre") + " (writer, "; !tv3aEncaja(tv3aDe(caja, 1), antes, ")", k.tiempo) {
			t.Fatalf("%s: la caja dice %q + %s + \")\" (el tiempo sin limite); dice %q", ca, antes, k.tiempoDice, tv3aDe(caja, 1))
		}
		m.LeerDetalle(tv3aDetalleCorrida(card))
		barra("con el Detalle")
		if g := tv3aNarracionDe(t, ca, utf, tv3aDoc(t, ca, m, 200)); !tv3aEncaja(g, "  writer · run 3/6 · ", de, k.tiempo) {
			t.Fatalf("%s: la Narracion empieza con \"  writer · run 3/6 · \" + %s + %q; es %q", ca, k.tiempoDice, de, g)
		}

		// ---- en una columna: 80x60, la fila es una linea y AHORA no tiene barra
		m = ttModelo(tv3aCfg(80, 60, utf, false), ttTablero(card))
		li = tv3aLista(t, ca, m, 80).tv3aTexto()
		if got := ttTramo(tv3aDe(li, 1), 21, 1<<20); !tv3aEncaja(got, "writer · ", de, k.tiempo) {
			t.Fatalf("%s: a 80 la fila es \" ▸ \" + titulo (8) + \" \" + camino + \" \" + \"writer · \" + %s + %q; desde la celda 21 dice %q: %q",
				ca, k.tiempoDice, de, got, tv3aDe(li, 1))
		}
		m.LeerDetalle(tv3aDetalleCorrida(card))
		d := tv3aDoc(t, ca, m, 80)
		for _, l := range d {
			if strings.HasPrefix(l, " tiempo ") {
				t.Fatalf("%s: en la forma angosta AHORA no tiene barra: %q", ca, l)
			}
		}
		if g := tv3aNarracionDe(t, ca, utf, d); !tv3aEncaja(g, "  writer · run 3/6 · ", de, k.tiempo) {
			t.Fatalf("%s: a 80 la Narracion empieza con \"  writer · run 3/6 · \" + %s + %q; es %q", ca, k.tiempoDice, de, g)
		}
	}
}

// CA-708, CA-711: un timeout_s que no es un limite (negativo, cero, o tan
// grande que no entra en una duracion: el de un sidecar danado) es "sin
// limite": AHORA no tiene barra, ni la fila de CORRIENDO, ni la caja, ni la
// Narracion dicen " de <limite>", el tiempo sigue siendo el transcurrido
// (14:02) y el tablero no se cae, tampoco cuando el reloj avanza (Tic).
func TestV3A_CA708_LimiteDanadoEsSinLimite(t *testing.T) {
	defer tv3aSinPanico(t)
	inicio := ttAhora.Add(-(14*time.Minute + 2*time.Second))
	for _, k := range []tv3aCorridaCaso{
		{nombre: "timeout_s -1", inicio: inicio, limite: -1, f: -1, tiempo: tv3aExacto("14:02"), tiempoDice: "14:02"},
		{nombre: "timeout_s 1<<62", inicio: inicio, limite: 1 << 62, f: -1, tiempo: tv3aExacto("14:02"), tiempoDice: "14:02"},
		{nombre: "timeout_s MinInt", inicio: inicio, limite: math.MinInt, f: -1, tiempo: tv3aExacto("14:02"), tiempoDice: "14:02"},
		{nombre: "timeout_s MaxInt", inicio: inicio, limite: math.MaxInt, f: -1, tiempo: tv3aExacto("14:02"), tiempoDice: "14:02"},
		{nombre: "timeout_s 0", inicio: inicio, limite: 0, f: -1, tiempo: tv3aExacto("14:02"), tiempoDice: "14:02"},
	} {
		tv3aRevisarCorrida(t, k)
		// ninguna linea de la pantalla pone un limite despues del tiempo
		for _, w := range []int{200, 80} {
			card := tv3aCorrida(k)
			m := ttModelo(tv3aCfg(w, 60, true, false), ttTablero(card))
			m.LeerDetalle(tv3aDetalleCorrida(card))
			for i, l := range ttLimpia(m) {
				if strings.Contains(l, "14:02 de") {
					t.Fatalf("CA-708 (%s, %d): sin limite ninguna linea dice \"14:02 de <limite>\"; la %d dice %q", k.nombre, w, i+1, l)
				}
			}
			m.Tic(ttAhora.Add(time.Second))
			if n := tv3aLineaCon(m, "writer · 14:03", 3); n == 0 {
				t.Fatalf("CA-708 (%s, %d): el reloj avanza sin que se caiga: la fila dice \"writer · 14:03\":\n%s", k.nombre, w, tv3aLineas(m))
			}
			for i, l := range ttLimpia(m) {
				if strings.Contains(l, "14:03 de") {
					t.Fatalf("CA-708 (%s, %d): despues del Tic sigue sin limite; la linea %d dice %q", k.nombre, w, i+1, l)
				}
			}
		}
	}
}

// CA-708, CA-711: con un limite normal (30:00) y un started_at muy viejo el
// transcurrido es enorme: la barra esta llena (20 de 20, nunca mas), el
// tiempo va en h:mm:ss con las horas que sean, " de 30:00" sigue, y el
// tablero no se cae (20 * transcurrido no entra en un int64 de
// nanosegundos). Hace 200 anios son 73049 dias justos: 1753176:00:00.
func TestV3A_CA708_TranscurridoEnormeBarraLlena(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, k := range []tv3aCorridaCaso{
		{nombre: "hace 200 anios", inicio: ttAhora.AddDate(-200, 0, 0), limite: 1800, f: 20,
			tiempo: tv3aExacto("1753176:00:00"), tiempoDice: "1753176:00:00", limiteDice: "30:00"},
		{nombre: "el 2 de enero del anio 1", inicio: time.Date(1, 1, 2, 0, 0, 0, 0, time.UTC), limite: 1800, f: 20,
			tiempo: tv3aRelojLargo, tiempoDice: "<h:mm:ss>", limiteDice: "30:00"},
		{nombre: "hace 31 minutos", inicio: ttAhora.Add(-31 * time.Minute), limite: 1800, f: 20,
			tiempo: tv3aExacto("31:00"), tiempoDice: "31:00", limiteDice: "30:00"},
	} {
		tv3aRevisarCorrida(t, k)
	}
}

// CA-708, CA-711: con un started_at en el futuro (un reloj adelantado) la
// barra tiene 0 hechas (las 20 que quedan, en gris), " de 30:00" sigue y el
// tablero no se cae, tambien 200 anios adelante (20 * transcurrido, negativo
// y enorme, tampoco entra). El tiempo en si doc.go no lo fija: es un solo
// texto, sin espacios.
func TestV3A_CA708_EmpezadoEnElFuturoBarraVacia(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, k := range []tv3aCorridaCaso{
		{nombre: "en 10 minutos", inicio: ttAhora.Add(10 * time.Minute), limite: 1800, f: 0,
			tiempo: tv3aUnaPalabra, tiempoDice: "<el tiempo>", limiteDice: "30:00"},
		{nombre: "en 200 anios", inicio: ttAhora.AddDate(200, 0, 0), limite: 1800, f: 0,
			tiempo: tv3aUnaPalabra, tiempoDice: "<el tiempo>", limiteDice: "30:00"},
	} {
		tv3aRevisarCorrida(t, k)
	}
}

// CA-708: la frontera de "fits a duration". 9223372036 s es el timeout_s
// mas grande que entra en un time.Duration (math.MaxInt64 ns son
// 9223372036.85 s): es un limite, 2562047:47:16, y con 14:02 corridos la
// barra tiene 0 hechas; con 200 anios (6311433600 s) tiene 20 * 6311433600
// / 9223372036 = 13 (entero, hacia abajo). Uno mas, 9223372037, ya no entra:
// sin limite.
func TestV3A_CA708_LimiteEnLaFronteraDeUnaDuracion(t *testing.T) {
	defer tv3aSinPanico(t)
	mayor := int(math.MaxInt64 / int64(time.Second))
	if mayor != 9223372036 {
		t.Fatalf("CA-708 (guarda): el timeout_s mas grande que entra es 9223372036, es %d", mayor)
	}
	for _, k := range []tv3aCorridaCaso{
		{nombre: "timeout_s 9223372036, 14:02", inicio: ttAhora.Add(-(14*time.Minute + 2*time.Second)), limite: mayor, f: 0,
			tiempo: tv3aExacto("14:02"), tiempoDice: "14:02", limiteDice: "2562047:47:16"},
		{nombre: "timeout_s 9223372036, 200 anios", inicio: ttAhora.AddDate(-200, 0, 0), limite: mayor, f: 13,
			tiempo: tv3aExacto("1753176:00:00"), tiempoDice: "1753176:00:00", limiteDice: "2562047:47:16"},
		{nombre: "timeout_s 9223372037", inicio: ttAhora.Add(-(14*time.Minute + 2*time.Second)), limite: mayor + 1, f: -1,
			tiempo: tv3aExacto("14:02"), tiempoDice: "14:02"},
	} {
		tv3aRevisarCorrida(t, k)
	}
}
