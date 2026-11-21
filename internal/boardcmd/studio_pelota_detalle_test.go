// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md en
// internal/boardcmd: AHORA, SIGUE y el camino de la pelota (CA-726, CA-727),
// mas una propiedad sobre todo el espacio de fixtures (CA-722..CA-727). Los
// textos son los de la tabla de AHORA y la de SIGUE del tablero v3 ("El
// detalle") y los de "El camino", con las diferencias del spec nuevo: sin los
// casos del piloto lanzado por un tablero, `ya no corre` para interrupted y
// red.reason para un rojo no esperado, y ningun texto nombra una tecla.
// Los helpers sp* estan en studio_pelota_test.go.
package boardcmd

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// ===========================================================================
// CA-726: AHORA, una prueba por fila de su tabla.
// ===========================================================================

func spAhoraEs(t *testing.T, nombre string, c Card, frase, gris string) {
	t.Helper()
	p := spPelotaDe(t, "CA-726", nombre, c)
	if p.Ahora.Frase != frase || p.Ahora.Gris != gris {
		t.Fatalf("CA-726: [%s] pelota.ahora es {frase %q, gris %q}, fue {frase %q, gris %q}", nombre, frase, gris, p.Ahora.Frase, p.Ahora.Gris)
	}
}

// CA-726: con running, la frase de su rol y `<provider> · empezo <HH:MM> ·
// <stage> <step>/<steps>`.
func TestCA726_AhoraConRunningPorRol(t *testing.T) {
	for _, cs := range []struct{ rol, frase string }{
		{"arquitecto", "El arquitecto está escribiendo el spec."},
		{"test-writer", "El test-writer está escribiendo los tests."},
		{"writer", "El writer está escribiendo el código."},
		{"reviewer", "El reviewer está revisando el cambio."},
		{"refutador", "El refutador está trabajando."},
	} {
		c := spCard(t, "corriendo "+cs.rol, spConRun(acWriter(), cs.rol), ColWriter)
		if c.Running == nil || c.Running.Provider != "claude" || c.Running.Stage != "run" || c.Running.Step != 3 || c.Running.Steps != 5 {
			t.Fatalf("fixture [corriendo %s]: running de claude en run 3/5: %+v", cs.rol, c.Running)
		}
		p := spPelotaDe(t, "CA-726", "corriendo "+cs.rol, c)
		if p.Ahora.Frase != cs.frase {
			t.Fatalf("CA-726: [corriendo %s] pelota.ahora.frase es %q, fue %q", cs.rol, cs.frase, p.Ahora.Frase)
		}
		if re := spGrisCorriendo(c.Running); !re.MatchString(p.Ahora.Gris) {
			t.Fatalf("CA-726: [corriendo %s] pelota.ahora.gris es `claude · empezo <HH:MM> · run 3/5`, fue %q", cs.rol, p.Ahora.Gris)
		}
	}
	// running gana sobre el rojo y sobre la columna
	c := Derive(spConRun(spRojoVeredicto(acEv()), "writer"))
	p := spPelotaDe(t, "CA-726", "corriendo y en rojo", c)
	if p.Ahora.Frase != "El writer está escribiendo el código." {
		t.Fatalf("CA-726: [corriendo y en rojo] AHORA es el del primer caso (running): %q", p.Ahora.Frase)
	}
	ev := spColumna(t, ColTuAprobacion)
	c = spCard(t, "corriendo en tu aprobacion", spConRun(ev, "arquitecto"), ColTuAprobacion)
	if p = spPelotaDe(t, "CA-726", "corriendo en tu aprobacion", c); p.Ahora.Frase != "El arquitecto está escribiendo el spec." {
		t.Fatalf("CA-726: [corriendo en tu aprobacion] AHORA es el del primer caso (running): %q", p.Ahora.Frase)
	}
}

// CA-726: con interrupted, `El trabajo del <rol> se cortó en el paso <step>
// de <steps>.` y la linea gris `ya no corre` (sin la tecla R), antes que el
// rojo.
func TestCA726_AhoraConInterrupted(t *testing.T) {
	c := spCard(t, "interrumpida", acInterrumpido(acWriter(), "writer", "claude"), ColWriter)
	spAhoraEs(t, "interrumpida", c, "El trabajo del writer se cortó en el paso 3 de 5.", "ya no corre")

	c = spCard(t, "interrumpida y en rojo", acInterrumpido(spRojoVeredicto(acEv()), "writer", "claude"), ColWriter)
	if c.Red == nil {
		t.Fatalf("fixture [interrumpida y en rojo]: tiene rojo")
	}
	spAhoraEs(t, "interrumpida y en rojo", c, "El trabajo del writer se cortó en el paso 3 de 5.", "ya no corre")

	ev := acEv()
	ev.SpecExists = false
	c = spCard(t, "interrumpida en backlog", acInterrumpido(ev, "arquitecto", "claude"), ColBacklog)
	spAhoraEs(t, "interrumpida en backlog", c, "El trabajo del arquitecto se cortó en el paso 3 de 5.", "ya no corre")
}

// CA-726: con un rojo no esperado, la frase es el plain del rojo y la linea
// gris su reason (en el tablero solo en modo experto; en la pelota siempre).
func TestCA726_AhoraConRojoNoEsperado(t *testing.T) {
	c := spCard(t, "rojo del veredicto", spRojoVeredicto(acEv()), ColWriter)
	if c.Red == nil || c.Red.Esperado || c.Red.Reason != "veredicto rojo: fallo el gate test" || c.Red.Plain == "" {
		t.Fatalf("fixture [rojo del veredicto]: un rojo no esperado con reason y plain: %+v", c.Red)
	}
	spAhoraEs(t, "rojo del veredicto", c, c.Red.Plain, "veredicto rojo: fallo el gate test")

	ev := acEv()
	ev.Approval = approval.StatusNotApproved
	c = spCard(t, "rojo del sobre en tu aprobacion", spRojoSobre(ev), ColTuAprobacion)
	if c.Red == nil || c.Red.Reason == "" {
		t.Fatalf("fixture [rojo del sobre en tu aprobacion]: tiene rojo con reason: %+v", c.Red)
	}
	spAhoraEs(t, "rojo del sobre en tu aprobacion (antes que la firma)", c, c.Red.Plain, c.Red.Reason)
}

// CA-726: con el rojo esperado, `Los tests están escritos y fallan, como se
// espera.` y `falta el código`.
func TestCA726_AhoraConRojoEsperado(t *testing.T) {
	for _, conPiloto := range []bool{true, false} {
		c := Derive(spEsperado(conPiloto))
		if c.Red == nil || !c.Red.Esperado {
			t.Fatalf("fixture [rojo esperado piloto=%v]: red.esperado: %+v", conPiloto, c.Red)
		}
		spAhoraEs(t, "rojo esperado", c, "Los tests están escritos y fallan, como se espera.", "falta el código")
	}
}

// CA-726: en Tu aprobacion, Tu aceptacion, Backlog y Hecho, las frases de la
// tabla con el plain en gris; en cualquier otra, el plain sin linea gris.
func TestCA726_AhoraPorColumna(t *testing.T) {
	casos := []struct {
		nombre, col, frase string
		conGris            bool
	}{
		{"tu aprobacion", ColTuAprobacion, "El spec está listo y espera tu firma.", true},
		{"tu aceptacion", ColTuAceptacion, "Está lista para que la pruebes y la aceptes.", true},
		{"backlog", ColBacklog, "Todavía no empezó.", true},
		{"hecho", ColHecho, "Terminada.", true},
		{"arquitecto", ColArquitecto, "", false},
		{"test-writer", ColTestWriter, "", false},
		{"writer", ColWriter, "", false},
		{"review", ColReview, "", false},
	}
	for _, cs := range casos {
		c := spCard(t, cs.nombre, spColumna(t, cs.col), cs.col)
		if c.Plain == "" || c.Red != nil || c.Running != nil || c.Interrupted != nil {
			t.Fatalf("fixture [%s]: con plain, sin rojo ni trabajo: %+v", cs.nombre, c)
		}
		if cs.conGris {
			spAhoraEs(t, cs.nombre, c, cs.frase, c.Plain)
		} else {
			spAhoraEs(t, cs.nombre+" (cualquier otro)", c, c.Plain, "")
		}
	}
}

// ===========================================================================
// CA-726: SIGUE, una prueba por celda de su tabla (columna x piloto).
// ===========================================================================

func TestCA726_SiguePorColumnaYPiloto(t *testing.T) {
	gris := "con el piloto seguiria sola: hoom item set precios --auto hasta-humano"
	casos := []struct {
		col                  string
		conPiloto, sinPiloto string
		grisSinPiloto        string
	}{
		{ColBacklog, "el arquitecto escribe el spec; despues te toca firmarlo", "el arquitecto escribe el spec; despues te toca firmarlo", ""},
		{ColArquitecto, "te toca firmar el spec", "te toca firmar el spec", ""},
		{ColTuAprobacion, "al firmar, el piloto pide los tests y sigue solo hasta tu aceptación", "al firmar, te toca pedir los tests al test-writer", gris},
		{ColTestWriter, "el piloto pide el código al writer y sigue hasta tu aceptación", "despues, te toca pedir el código al writer", gris},
		{ColWriter, "el piloto pide la review; si algo bloquea, hace una ronda de hallazgos", "despues, te toca pedir la review al reviewer", gris},
		{ColReview, "si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos", "si nada bloquea, te toca aceptar", gris},
		{ColTuAceptacion, "al aceptar, hoom integra y la tarjeta pasa a Hecho", "al aceptar, hoom integra y la tarjeta pasa a Hecho", ""},
		{ColHecho, "nada: la tarjeta esta terminada", "nada: la tarjeta esta terminada", ""},
	}
	for _, cs := range casos {
		ev := spColumna(t, cs.col)
		ev.Item.Auto = ""
		p := spPelotaDe(t, "CA-726", "sigue sin piloto en "+cs.col, spCard(t, "sin piloto "+cs.col, ev, cs.col))
		if p.Sigue.Frase != cs.sinPiloto || p.Sigue.Gris != cs.grisSinPiloto {
			t.Fatalf("CA-726: [%s sin piloto] pelota.sigue es {%q, %q}, fue {%q, %q}", cs.col, cs.sinPiloto, cs.grisSinPiloto, p.Sigue.Frase, p.Sigue.Gris)
		}
		ev.Item.Auto = item.AutoHastaHumano
		p = spPelotaDe(t, "CA-726", "sigue con piloto en "+cs.col, spCard(t, "con piloto "+cs.col, ev, cs.col))
		if p.Sigue.Frase != cs.conPiloto || p.Sigue.Gris != "" {
			t.Fatalf("CA-726: [%s con piloto] pelota.sigue es {%q, \"\"}, fue {%q, %q}", cs.col, cs.conPiloto, p.Sigue.Frase, p.Sigue.Gris)
		}
	}
	// la linea gris nombra el slug de la tarjeta
	ev := acWriter()
	ev.Item.Slug = "exportar-reporte-pdf"
	ev.Item.Auto = ""
	c := spCard(t, "sigue con otro slug", ev, ColWriter)
	if c.Slug != "exportar-reporte-pdf" {
		t.Fatalf("fixture [sigue con otro slug]: la tarjeta tiene el slug del item: %q", c.Slug)
	}
	p := spPelotaDe(t, "CA-726", "sigue con otro slug", c)
	if p.Sigue.Gris != "con el piloto seguiria sola: hoom item set exportar-reporte-pdf --auto hasta-humano" {
		t.Fatalf("CA-726: [otro slug] la linea gris de SIGUE nombra el slug de la tarjeta: %q", p.Sigue.Gris)
	}
}

// CA-726: ningun texto de pelota nombra una tecla del tablero (`R `,
// `enter`, ` n `, `P `), en ningun fixture; en particular, interrupted no dice
// `R reanuda` ni `R lo reanuda`.
func TestCA726_NingunTextoNombraUnaTecla(t *testing.T) {
	for _, f := range spTodas(t) {
		spMiraTeclas(t, "CA-726", f.nombre, spPelotaDe(t, "CA-726", f.nombre, f.c))
	}
}

// CA-726: AHORA y SIGUE en todos los fixtures son los de sus tablas.
func TestCA726_AhoraYSigueEnTodosLosFixtures(t *testing.T) {
	for _, f := range spTodas(t) {
		p := spPelotaDe(t, "CA-726", f.nombre, f.c)
		spMiraAhora(t, "CA-726", f.nombre, f.c, p)
		spMiraSigue(t, "CA-726", f.nombre, f.c, p)
	}
}

// ===========================================================================
// CA-727: el camino.
// ===========================================================================

// spCaminoEs exige el camino con la entrada actual en idx (-1 = ninguna) y su
// texto.
func spCaminoEs(t *testing.T, nombre string, c Card, idx int, texto string) {
	t.Helper()
	p := spPelotaDe(t, "CA-727", nombre, c)
	if len(p.Camino) != 8 {
		t.Fatalf("CA-727: [%s] el camino tiene 8 entradas, tuvo %d", nombre, len(p.Camino))
	}
	for i, e := range p.Camino {
		if e.Nombre != spNombres[i] {
			t.Fatalf("CA-727: [%s] camino[%d].nombre es %q, fue %q", nombre, i, spNombres[i], e.Nombre)
		}
		estado, txt := "falta", ""
		switch {
		case idx < 0 || i < idx:
			estado = "hecho"
		case i == idx:
			estado, txt = "actual", texto
		}
		if e.Estado != estado || e.Texto != txt {
			t.Fatalf("CA-727: [%s] camino[%d] (%s) es {%s, %q}, fue {%s, %q}", nombre, i, spNombres[i], estado, txt, e.Estado, e.Texto)
		}
	}
}

// CA-727: TE TOCA en cada columna del medio: `te toca a vos` en la suya.
func TestCA727_CaminoTeToca(t *testing.T) {
	for i, col := range []string{ColArquitecto, ColTuAprobacion, ColTestWriter, ColWriter, ColReview, ColTuAceptacion} {
		c := spCard(t, "te toca en "+col, spColumna(t, col), col)
		spCaminoEs(t, "te toca en "+col, c, i+1, "te toca a vos")
	}
	spCaminoEs(t, "rojo esperado", spCard(t, "rojo esperado", spEsperado(false), ColWriter), 4, "te toca a vos")
}

// CA-727: CORRIENDO: `<rol>, ahora` en su columna.
func TestCA727_CaminoCorriendo(t *testing.T) {
	spCaminoEs(t, "corriendo writer", spCard(t, "corriendo writer", spConRun(acWriter(), "writer"), ColWriter), 4, "writer, ahora")
	spCaminoEs(t, "corriendo reviewer en writer", spCard(t, "corriendo reviewer", spConRun(acWriter(), "reviewer"), ColWriter), 4, "reviewer, ahora")
	ev := acEv()
	ev.SpecExists = false
	spCaminoEs(t, "corriendo en backlog", spCard(t, "corriendo en backlog", spConRun(ev, "arquitecto"), ColBacklog), 0, "arquitecto, ahora")
}

// CA-727: DETENIDA: `se detuvo` en su columna.
func TestCA727_CaminoDetenida(t *testing.T) {
	spCaminoEs(t, "interrumpida", spCard(t, "interrumpida", acInterrumpido(acWriter(), "writer", "claude"), ColWriter), 4, "se detuvo")
	ev := acEv()
	ev.Approval = approval.StatusNotApproved
	spCaminoEs(t, "rojo en tu aprobacion", spCard(t, "rojo en tu aprobacion", spRojoSobre(ev), ColTuAprobacion), 2, "se detuvo")
	ev = acEv()
	ev.SpecExists = false
	spCaminoEs(t, "rojo en backlog", spCard(t, "rojo en backlog", spRojoSobre(ev), ColBacklog), 0, "se detuvo")
}

// CA-727: EN COLA: `sin empezar` en Backlog, el resto falta.
func TestCA727_CaminoEnCola(t *testing.T) {
	spCaminoEs(t, "en cola", spCard(t, "en cola", spColumna(t, ColBacklog), ColBacklog), 0, "sin empezar")
}

// CA-727: HECHO: las 8 entradas son hecho, sin texto.
func TestCA727_CaminoHecho(t *testing.T) {
	spCaminoEs(t, "hecho", spCard(t, "hecho", spColumna(t, ColHecho), ColHecho), -1, "")
	ev := spColumna(t, ColHecho)
	ev.Uncommitted = acSucios()
	spCaminoEs(t, "hecho con cambios", spCard(t, "hecho con cambios", ev, ColHecho), -1, "")
}

// spCaminoBienFormado son las propiedades del camino que valen para toda
// tarjeta: 8 entradas en el orden de boardcmd.Columns con sus nombres, a lo
// sumo una actual, hecho antes de ella y falta despues, y texto vacio fuera de
// la actual (en la actual, el texto de su grupo).
func spCaminoBienFormado(t *testing.T, nombre string, c Card, p spPelota) {
	t.Helper()
	if len(p.Camino) != len(Columns) {
		t.Fatalf("CA-727: [%s] el camino tiene una entrada por columna (8), tuvo %d", nombre, len(p.Camino))
	}
	actuales := 0
	visto := false
	for i, e := range p.Camino {
		if e.Nombre != spNombres[i] {
			t.Fatalf("CA-727: [%s] camino[%d] es %q (orden de boardcmd.Columns), fue %q", nombre, i, spNombres[i], e.Nombre)
		}
		switch e.Estado {
		case "hecho":
			if visto {
				t.Fatalf("CA-727: [%s] despues de la actual todo falta: %+v", nombre, p.Camino)
			}
		case "actual":
			actuales++
			visto = true
		case "falta":
			if !visto && p.Grupo != spHecho {
				t.Fatalf("CA-727: [%s] falta solo despues de la actual: %+v", nombre, p.Camino)
			}
		default:
			t.Fatalf("CA-727: [%s] camino[%d].estado es hecho, actual o falta, fue %q", nombre, i, e.Estado)
		}
		if e.Estado != "actual" && e.Texto != "" {
			t.Fatalf("CA-727: [%s] texto solo en la entrada actual: camino[%d] = %+v", nombre, i, e)
		}
	}
	if actuales > 1 {
		t.Fatalf("CA-727: [%s] a lo sumo una entrada actual, hubo %d: %+v", nombre, actuales, p.Camino)
	}
	if (actuales == 0) != (c.Column == ColHecho) {
		t.Fatalf("CA-727: [%s] hay una actual salvo en Hecho, donde las 8 son hecho: %+v", nombre, p.Camino)
	}
}

// CA-727: en todos los fixtures el camino es el del contrato y esta bien
// formado.
func TestCA727_CaminoEnTodosLosFixtures(t *testing.T) {
	for _, f := range spTodas(t) {
		p := spPelotaDe(t, "CA-727", f.nombre, f.c)
		spCaminoBienFormado(t, f.nombre, f.c, p)
		spMiraCamino(t, "CA-727", f.nombre, f.c, p)
	}
}

// ===========================================================================
// Propiedad (testing/quick): para cualquier combinacion de forma, tope,
// extra, piloto, titulo, cambios sin guardar y proveedores, la pelota es la
// del contrato (CA-722..CA-727) y su camino esta bien formado.
// ===========================================================================

// TestCA722_CA727_PropiedadPelotaDelContrato compara la pelota de cada
// combinacion con el oraculo sp*.
// Re-expresado por CA-753 (ciclo-lo-decide-hoom): con la forma
// review-hallazgos el paso es pedir-refutador y el oraculo (spFraseDe) da su
// frase.
func TestCA722_CA727_PropiedadPelotaDelContrato(t *testing.T) {
	prop := func(forma, tope, extra uint8, piloto, sinTitulo, cambios, sinProv, arbol bool) bool {
		ev := piEv(piFormas[int(forma)%len(piFormas)], piTopes[int(tope)%len(piTopes)], piExtras[int(extra)%len(piExtras)])
		if !piloto {
			ev.Item.Auto = ""
		}
		if sinTitulo {
			ev.Item.Titulo = ""
		}
		if cambios {
			ev.Uncommitted = acSucios()
		}
		if sinProv {
			ev.Providers = nil
		}
		if arbol {
			ev = bdArbol(ev)
		}
		c := Derive(ev)
		nombre := "propiedad " + c.Column
		p := spPelotaDe(t, "CA-722", nombre, c)
		spMiraTodo(t, nombre, c, p)
		spCaminoBienFormado(t, nombre, c, p)
		return true
	}
	cfg := &quick.Config{MaxCount: 400, Rand: rand.New(rand.NewSource(722))}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("CA-722..CA-727: %v", err)
	}
}

// CA-724: la propiedad de la review sin accion: cualquier combinacion de
// cambios y espacio propio da guardar (si hay cambios) o nada que hacer.
func TestCA724_PropiedadReviewSinAccion(t *testing.T) {
	prop := func(cambios, arbol bool) bool {
		ev := tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar)
		if cambios {
			ev.Uncommitted = acSucios()
		}
		if arbol {
			ev = bdArbol(ev)
		}
		c := Derive(ev)
		spMiraPaso(t, "CA-724", "review sin accion", c, spPelotaDe(t, "CA-724", "review sin accion", c))
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 8, Rand: rand.New(rand.NewSource(724))}); err != nil {
		t.Fatalf("CA-724: %v", err)
	}
}
