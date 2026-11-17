// Tests adversariales de la v3 de hoom tablero: el grupo CORRIENDO, DETENIDA
// por un corte y el detalle que sigue el final de la narracion (CA-711, que
// re-expresa CA-558).
package tablero_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// tv3cGrupoDe exige que la linea de grupo nombre este en la lista y que la
// tarjeta de titulo este debajo de ella (antes de la linea de grupo que
// sigue).
func tv3cGrupoDe(t *testing.T, ca string, m *tablero.Modelo, ancho int, titulo, grupo string) {
	t.Helper()
	ls := tv3cInteriorLista(m, ancho)
	esta := ""
	for _, l := range ls {
		f := strings.Fields(l)
		if len(f) >= 2 && (f[0] == "TE" || f[0] == "CORRIENDO" || f[0] == "DETENIDA" || f[0] == "EN" || f[0] == "HECHO") &&
			!strings.HasPrefix(l, "   ") {
			esta = strings.Join(f, " ")
			continue
		}
		if strings.Contains(l, titulo) {
			if !strings.HasPrefix(esta, grupo) {
				t.Fatalf("%s: %q esta en el grupo %s, esta debajo de %q:\n%s", ca, titulo, grupo, esta, strings.Join(ls, "\n"))
			}
			return
		}
	}
	t.Fatalf("%s: la lista tiene la tarjeta %q:\n%s", ca, titulo, strings.Join(ls, "\n"))
}

// CA-711 y CA-558: cada tarjeta con running esta en CORRIENDO con
// "<rol> · <tiempo>[ de <limite>]" en su segunda linea: los tiempos de la v1
// (m:ss, h:mm:ss), "agente" sin rol, sin " de" sin limite.
func TestV3C_CA711_CA558_LaSegundaLineaDeCorriendo(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-711"
	const ancho = 200
	cfg := tv3cConfig(ancho, 40)
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	conRol := func(rol string, hace time.Duration, limite int) boardcmd.Card {
		c := tv3cConRunning(sola, "env-1", rol)
		c.Running.StartedAt = ttAhora.Add(-hace)
		c.Running.TimeoutS = limite
		return c
	}
	casos := []struct {
		nombre string
		c      boardcmd.Card
		quiero string
	}{
		{"writer de 0:30 con limite", tv3cConRunning(sola, "env-1", "writer"), "   writer · 0:30 de 45:00"},
		{"sin limite", conRol("writer", 30*time.Second, 0), "   writer · 0:30"},
		{"sin rol", conRol("", 30*time.Second, 2700), "   agente · 0:30 de 45:00"},
		{"mas de una hora", conRol("reviewer", time.Hour+2*time.Minute+3*time.Second, 2700), "   reviewer · 1:02:03 de 45:00"},
		{"con un rojo no esperado", tv3cConRojo(tv3cConRunning(sola, "env-1", "writer"), false), "   writer · 0:30 de 45:00"},
		{"con running e interrupted", tv3cConInterrupted(tv3cConRunning(sola, "env-1", "writer"), "writer"), "   writer · 0:30 de 45:00"},
	}
	for _, c := range casos {
		m := tv3cModelo(cfg, c.c)
		tv3cGrupoDe(t, ca, m, ancho, "Sola", "CORRIENDO 1")
		if got := tv3cSegunda(t, ca, m, ancho, "Sola"); got != c.quiero {
			t.Fatalf("%s: [%s] la segunda linea de CORRIENDO\n got: %q\nwant: %q", ca, c.nombre, got, c.quiero)
		}
	}
	// la de los dorados: 12:41 de 45:00
	m := tv3cModelo(cfg, ttInfraAPI(true))
	tv3cGrupoDe(t, ca, m, ancho, "Convenciones de la API", "CORRIENDO 1")
	if got := tv3cSegunda(t, ca, m, ancho, "Convenciones de la API"); got != "   writer · 12:41 de 45:00" {
		t.Fatalf("%s: la segunda linea de infra-api es \"   writer · 12:41 de 45:00\": %q", ca, got)
	}
	// en cian
	m = tv3cModelo(tv3cConfigColor(ancho, 40), tv3cConRunning(sola, "env-1", "writer"))
	if got := ttEstiloDe(tv3cSegundaCruda(t, ca, m, ancho, "Sola"), "writer · 0:30 de 45:00"); got != "\x1b[36m" {
		t.Fatalf("%s: la segunda linea de CORRIENDO va en cian (36): %q", ca, got)
	}
}

// CA-711 y CA-558: una tarjeta cuyo piloto lanzo este tablero y sigue vivo
// esta en CORRIENDO: "el piloto la lleva · decidiendo" sin trabajo, "el
// piloto la lleva · " delante del trabajo con running; al terminar el
// piloto sale de CORRIENDO.
func TestV3C_CA711_CA558_ElPilotoLaLleva(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-711"
	const ancho = 200
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(ancho, 40), sola)
	tv3cGrupoDe(t, ca, m, ancho, "Sola", "TE TOCA 1")
	l := tv3cLanzaSeguir(t, ca, m, "sola")
	tv3cGrupoDe(t, ca, m, ancho, "Sola", "CORRIENDO 1")
	if got := tv3cSegunda(t, ca, m, ancho, "Sola"); got != "   el piloto la lleva · decidiendo" {
		t.Fatalf("%s: el piloto vivo sin trabajo: \"   el piloto la lleva · decidiendo\": %q", ca, got)
	}
	// una lectura sin running no lo saca de CORRIENDO mientras vive
	tv3cLeer(m, sola)
	tv3cGrupoDe(t, ca, m, ancho, "Sola", "CORRIENDO 1")
	tv3cLeer(m, tv3cConRunning(sola, "env-del-piloto", "writer"))
	if got := tv3cSegunda(t, ca, m, ancho, "Sola"); got != "   el piloto la lleva · writer · 0:30 de 45:00" {
		t.Fatalf("%s: el piloto vivo con running: \"   el piloto la lleva · writer · 0:30 de 45:00\": %q", ca, got)
	}
	// el trabajo del writer termina y el piloto sigue vivo: decidiendo
	tv3cLeer(m, sola)
	if got := tv3cSegunda(t, ca, m, ancho, "Sola"); got != "   el piloto la lleva · decidiendo" {
		t.Fatalf("%s: entre dos trabajos el piloto decide: %q", ca, got)
	}
	tv3cFin(m, l.ID, 0, tv3cPrefijoParada+"la tarjeta llego a tu aceptacion")
	tv3cLeer(m, sola)
	tv3cGrupoDe(t, ca, m, ancho, "Sola", "TE TOCA 1")
	if strings.Contains(strings.Join(tv3cInteriorLista(m, ancho), "\n"), "el piloto la lleva") {
		t.Fatalf("%s: con el piloto terminado ninguna fila dice \"el piloto la lleva\"", ca)
	}
}

// CA-711 y CA-558: una tarjeta con interrupted (sin running) va a DETENIDA,
// en rojo, con "✗ se corto el trabajo del <rol>" ("agente" sin rol); una con
// un rojo no esperado, con "✗ " y el plain de su rojo.
func TestV3C_CA711_CA558_ElCorteVaADetenida(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-711"
	const ancho = 200
	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	sinRol := tv3cConInterrupted(sola, "")
	casos := []struct {
		nombre string
		c      boardcmd.Card
		quiero string
	}{
		{"cortado del writer", tv3cConInterrupted(sola, "writer"), "   x se corto el trabajo del writer"},
		{"cortado sin rol", sinRol, "   x se corto el trabajo del agente"},
		{"cortado y rojo", tv3cConRojo(tv3cConInterrupted(sola, "reviewer"), false), "   x se corto el trabajo del reviewer"},
		{"rojo no esperado", tv3cConRojo(sola, false), "   x la verificacion de sola dio rojo"},
	}
	for _, c := range casos {
		m := tv3cModelo(tv3cConfig(ancho, 40), c.c)
		tv3cGrupoDe(t, ca, m, ancho, "Sola", "DETENIDA 1")
		if got := tv3cSegunda(t, ca, m, ancho, "Sola"); got != c.quiero {
			t.Fatalf("%s: [%s] la segunda linea de DETENIDA\n got: %q\nwant: %q", ca, c.nombre, got, c.quiero)
		}
		mc := tv3cModelo(tv3cConfigColor(ancho, 40), c.c)
		texto := strings.TrimPrefix(c.quiero, "   ")
		if got := ttEstiloDe(tv3cSegundaCruda(t, ca, mc, ancho, "Sola"), texto); got != "\x1b[31m" {
			t.Fatalf("%s: [%s] la segunda linea de DETENIDA va en rojo (31): %q", ca, c.nombre, got)
		}
	}
	// el rojo esperado no va a DETENIDA
	m := tv3cModelo(tv3cConfig(ancho, 40), tv3cConRojo(sola, true))
	tv3cGrupoDe(t, ca, m, ancho, "Sola", "TE TOCA 1")
}

// tv3cUltimaDelDetalle es la ultima linea interior del detalle (W >= 100).
func tv3cUltimaDelDetalle(m *tablero.Modelo, ancho int) string {
	ls := tv3cInteriorDetalle(m, ancho)
	return ls[len(ls)-1]
}

// CA-711 y CA-558: el detalle de una tarjeta con running abre mostrando el
// final de su narracion y lo sigue mientras llegan eventos; si la persona
// sube deja de seguirlo, vuelve a seguirlo cuando la vista vuelve al final,
// y G lo lleva al final.
func TestV3C_CA711_CA558_ElDetalleSigueElFinal(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-711"
	const ancho = 200
	c := tv3cConRunning(tv3cSola("sola", "Sola", boardcmd.ColWriter), "env-1", "writer")
	m := tv3cModelo(tv3cConfig(ancho, 40), c)
	leer := func(n int) {
		t.Helper()
		m.LeerDetalle(tv3cDetalleCon(c, tv3cEventos(n)))
	}
	ultima := func(que, evento string) {
		t.Helper()
		if got := tv3cUltimaDelDetalle(m, ancho); !strings.Contains(got, evento) {
			t.Fatalf("%s: %s: la ultima linea a la vista del detalle es la de %s, es %q:\n%s", ca, que, evento, got,
				strings.Join(tv3cInteriorDetalle(m, ancho), "\n"))
		}
	}
	leer(80)
	ultima("al abrir", "evento-079")
	if strings.HasPrefix(tv3cInteriorDetalle(m, ancho)[0], " AHORA") {
		t.Fatalf("%s: con running el detalle no abre en su primera linea", ca)
	}
	leer(81)
	ultima("llega un evento", "evento-080")
	// la persona sube una linea: deja de seguirlo
	ttTeclas(m, tablero.TeclaTab, "k")
	ultima("subio una linea", "evento-079")
	leer(82)
	ultima("subio y llega un evento", "evento-079")
	if tv3cDetalleTiene(m, ancho, "evento-081") {
		t.Fatalf("%s: tras subir, un evento nuevo no mueve la vista", ca)
	}
	// G vuelve al final y lo sigue de nuevo
	ttTeclas(m, "G")
	ultima("G", "evento-081")
	leer(83)
	ultima("G y llega un evento", "evento-082")
	// volver al final con j tambien lo sigue
	ttTeclas(m, "k", "j")
	leer(84)
	ultima("k, j y llega un evento", "evento-083")
}

// CA-711: una tarjeta sin running abre su detalle en la primera linea, y un
// evento nuevo no lo mueve.
func TestV3C_CA711_SinRunningElDetalleAbreArriba(t *testing.T) {
	defer tv3cSinPanico(t)
	const ca = "CA-711"
	const ancho = 200
	c := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(ancho, 40), c)
	m.LeerDetalle(tv3cDetalleCon(c, tv3cEventos(80)))
	primera := tv3cInteriorDetalle(m, ancho)[0]
	if !strings.HasPrefix(primera, " AHORA") {
		t.Fatalf("%s: sin running el detalle abre en su primera linea (AHORA): %q", ca, primera)
	}
	m.LeerDetalle(tv3cDetalleCon(c, tv3cEventos(81)))
	if got := tv3cInteriorDetalle(m, ancho)[0]; got != primera {
		t.Fatalf("%s: sin running un evento nuevo no mueve el detalle: %q", ca, got)
	}
}
