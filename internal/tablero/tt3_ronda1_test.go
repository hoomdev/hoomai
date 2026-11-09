// Ronda 1 del test-writer (la review por tramos de tablero-con-paneles), en
// el modelo: los tests nuevos de la parte A (lo que el refutador rompio sin
// que ningun test lo viera) y las regresiones de la parte B, escritos desde
// doc.go y el spec. Cada uno cita su CA y el id corto de su hallazgo.
package tablero_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/statuscmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ttNoEntra es lo que la linea de mensajes dice, en rojo, cuando el modal
// Pedir o su comando no entran ni cediendo lo que cede (doc.go, ronda 1b).
const ttNoEntra = "agranda la terminal: el comando no entra entero"

// ---------------------------------------------------------------- A1 f4b30e: n sin siguiente paso

// CA-566 (v1 CA-525), hallazgo f4b30e: n sin siguiente paso no ordena nada,
// no abre ningun modal y la linea de mensajes dice lo que diria hoom
// siguiente: en Hecho, interrumpida, sin acciones, con la accion del
// siguiente paso deshabilitada, con running (y las otras reglas que no dan
// accion). Desde [2] y desde [0] de la tarjeta.
func TestCA566_CA525_NSinSiguientePaso(t *testing.T) {
	writer := ttAccionWriter()
	writerOff := ttAccionWriter()
	writerOff.Enabled, writerOff.Why = false, "se agoto el presupuesto de la tarjeta"
	conRunning := ttTarjetaCon(writer)
	conRunning.Running = &boardcmd.Running{EnvelopeID: "e", Role: "writer", Stage: "run", StartedAt: ttAhora}
	conInterr := ttTarjetaCon(writer)
	conInterr.Interrupted, conInterr.NeedsDecision = &boardcmd.Interrupted{EnvelopeID: "e", Role: "writer", Stage: "run", Step: 1, Steps: 5}, true
	enHecho := ttViejo(1)
	enHecho.Actions = []boardcmd.Action{writer}
	aprobSin := ttCard("sin-firma", "Sin firma", boardcmd.ColTuAprobacion, "falta", ttSeg("s", "s", boardcmd.SegFalta, "d"))
	aprobOff := ttNucTenancy()
	aprobOff.Actions = []boardcmd.Action{{ID: boardcmd.ActAprobar, Label: "Aprobar spec", Enabled: false, Why: "el spec no pasa el lint", Paths: []string{}}}
	casos := []struct {
		nombre string
		card   boardcmd.Card
		quiere string
	}{
		{"en Hecho", enHecho, "la tarjeta ya esta en Hecho"},
		{"con running", conRunning, "el writer esta trabajando: espera a que termine"},
		{"interrumpida", conInterr, "el trabajo se corto: R reanuda o pide de nuevo"},
		{"interrumpida con reanudar", ttPagosCaidos(), "el trabajo se corto: R reanuda o pide de nuevo"},
		{"sin acciones", ttTarjetaCon(), "en Writer no hay nada que hacer ahora: el plain de la tarjeta"},
		{"primera de rol deshabilitada", ttTarjetaCon(writerOff), "se agoto el presupuesto de la tarjeta"},
		{"aprobar deshabilitada", aprobOff, "el spec no pasa el lint"},
		{"en Tu aprobacion sin la accion", aprobSin, "en Tu aprobacion no hay aprobar el spec"},
		{"primera sin tecla", ttTarjetaCon(ttRelanzar(), writer), "en Writer no hay nada que hacer ahora: el plain de la tarjeta"},
	}
	for _, c := range casos {
		ca := "CA-566 (hallazgo f4b30e, " + c.nombre + ")"
		m := ttModelo(ttConfig(200, 50, false), ttTablero(c.card))
		if c.nombre == "en Hecho" {
			ttTeclas(m, "G", tablero.TeclaEnter, "j")
		}
		if m.Elegida() != c.card.Slug {
			t.Fatalf("%s (guarda): la tarjeta esta elegida: %q\n%s", ca, m.Elegida(), ttPantalla(m))
		}
		for _, desde := range []string{"[2]", "[0]"} {
			if desde == "[0]" {
				m.Tecla(tablero.TeclaEnter, ttAhora)
				if m.Lugar() != tablero.LugarVerTarjeta {
					t.Fatalf("%s (guarda): enter lleva a [0] de la tarjeta: %q", ca, m.Lugar())
				}
			}
			lugar := m.Lugar()
			ttSinOrdenes(t, ca, "n desde "+desde, m.Tecla("n", ttAhora))
			if m.Modal() != tablero.SinModal || m.Lugar() != lugar {
				t.Fatalf("%s: desde %s, n sin siguiente paso no abre nada: modal %q, lugar %q", ca, desde, m.Modal(), m.Lugar())
			}
			ttDiceMensaje(t, ca, "n desde "+desde, m, false, c.quiere)
		}
	}
}

// ---------------------------------------------------------------- A2 7a54a9: el provider que no puede

// CA-568 (v1 CA-527), hallazgo 7a54a9: un provider que no puede (OK false)
// puesto entre claude y codex: → y ← lo saltan; en la lista abierta va todo
// en gris con su why, y enter sobre el no lo elige (la lista sigue abierta y
// nada cambia).
func TestCA568_CA527_ProviderQueNoPuede(t *testing.T) {
	cfg := ttAncho()
	cfg.Color = true
	a := ttAccionWriter()
	a.Pedido = "hola"
	a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), ttProvOpencode(), ttProvCodex(false)}
	m := ttAbrirModal(t, cfg, "w", tablero.ModalPedir, a)
	valor := func() string { return ttMC(t, m).ttValor("Provider") }
	m.Tecla(tablero.TeclaTab, ttAhora)
	if v := valor(); v != "< claude >" {
		t.Fatalf("CA-568 (guarda): el Provider empieza en claude (el por defecto): %q", v)
	}
	for _, p := range []struct {
		k      tablero.Tecla
		quiere string
	}{{tablero.TeclaDer, "< codex >"}, {tablero.TeclaDer, "< claude >"}, {tablero.TeclaIzq, "< codex >"}} {
		m.Tecla(p.k, ttAhora)
		if v := valor(); v != p.quiere {
			t.Fatalf("CA-568 (hallazgo 7a54a9): %q salta opencode (no puede): %q, se esperaba %q", p.k, v, p.quiere)
		}
	}
	// la lista: opencode en gris con su why
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if m.Lugar() != tablero.LugarPedirLista {
		t.Fatalf("CA-568: enter en Provider abre su lista: %q", m.Lugar())
	}
	ops, _ := ttLista(t, m)
	quiere := []string{"   claude", "   opencode  " + ttWhyOpencode, " > codex"}
	if strings.Join(ops, "|") != strings.Join(quiere, "|") {
		t.Fatalf("CA-568 (hallazgo 7a54a9): las filas de la lista de Provider:\n got %q\nwant %q", ops, quiere)
	}
	grises := 0
	for _, x := range m.Pantalla() {
		s := ttSinSGR(x)
		if strings.Contains(s, "opencode  "+ttWhyOpencode) {
			grises++
			if ttEstiloDe(x, "opencode  "+ttWhyOpencode) != "\x1b[90m" {
				t.Fatalf("CA-568 (hallazgo 7a54a9): la fila de opencode va toda en gris: %q", x)
			}
		}
		if strings.Contains(s, "   claude") && ttEstiloDe(x, "claude") == "\x1b[90m" {
			t.Fatalf("CA-568: la fila de claude (puede) no va en gris: %q", x)
		}
	}
	if grises != 1 {
		t.Fatalf("CA-568 (hallazgo 7a54a9): la fila de opencode esta una vez: %d\n%s", grises, ttPantalla(m))
	}
	// ↑ la ilumina; enter sobre ella no hace nada
	m.Tecla(tablero.TeclaArriba, ttAhora)
	ops, _ = ttLista(t, m)
	if len(ops) != 3 || !strings.HasPrefix(ops[1], " > opencode") {
		t.Fatalf("CA-568: ↑ mueve la marca por todas las opciones, la gris incluida: %q", ops)
	}
	ttSinOrdenes(t, "CA-568", "enter sobre opencode", m.Tecla(tablero.TeclaEnter, ttAhora))
	if m.Lugar() != tablero.LugarPedirLista {
		t.Fatalf("CA-568 (hallazgo 7a54a9): enter sobre la gris no la elige ni cierra la lista: %q", m.Lugar())
	}
	if ops, _ = ttLista(t, m); !strings.HasPrefix(ops[1], " > opencode") {
		t.Fatalf("CA-568 (hallazgo 7a54a9): enter sobre la gris no hace nada: %q", ops)
	}
	m.Tecla(tablero.TeclaEsc, ttAhora)
	if v := valor(); v != "< codex >" || !strings.Contains(ttMC(t, m).ttComando(), "--provider codex") {
		t.Fatalf("CA-568 (hallazgo 7a54a9): opencode no quedo elegido: %q %q", v, ttMC(t, m).ttComando())
	}
}

// ---------------------------------------------------------------- A8 09738a: las teclas de accion desde [3]

// CA-566, CA-558, hallazgo 09738a: las teclas de accion desde [3] y desde su
// [0] actuan sobre la tarjeta de la fila elegida de [3], no sobre la de [2]:
// con [2] en nuc-tenancy y [3] en pagos-caidos (interrumpida), R abre
// "Reanudar · pagos-caidos" y su boton lanza hoom reanudar pagos-caidos.
func TestCA566_CA558_TeclasDeAccionDesdeCorriendo(t *testing.T) {
	pedido := strings.TrimSpace(ttPagosCaidos().Actions[0].Pedido)
	for _, enCero := range []bool{false, true} {
		ca := fmt.Sprintf("CA-566 (hallazgo 09738a, desde [0]: %v)", enCero)
		m := ttModelo(ttAncho(), ttTableroPrincipal())
		if m.Elegida() != "nuc-tenancy" {
			t.Fatalf("%s (guarda): [2] empieza en nuc-tenancy: %q", ca, m.Elegida())
		}
		ttTeclas(m, "3", "j")
		if e := ttElegidaEn(ttCajaDe(t, ca, m, "[3]")); !strings.HasPrefix(e, "pagos-caidos ") {
			t.Fatalf("%s (guarda): j en [3] elige pagos-caidos: %q", ca, e)
		}
		if enCero {
			m.Tecla(tablero.TeclaEnter, ttAhora)
			if m.Lugar() != tablero.LugarVerCorriendo {
				t.Fatalf("%s (guarda): enter en [3] lleva a su [0]: %q", ca, m.Lugar())
			}
		}
		ttSinOrdenes(t, ca, "R", m.Tecla("R", ttAhora))
		if m.Modal() != tablero.ModalPedir {
			t.Fatalf("%s: R abre el modal de reanudar de la fila de [3]: %q\n%s", ca, m.Modal(), ttPantalla(m))
		}
		if mc := ttMC(t, m); !strings.HasPrefix(mc.Arriba, "# Reanudar · pagos-caidos ") {
			t.Fatalf("%s: el titulo es 'Reanudar · pagos-caidos': %q", ca, mc.Arriba)
		}
		m.Tecla(tablero.TeclaEnter, ttAhora) // al boton
		l := ttUnaLanzar(t, ca, "el boton de reanudar", m.Tecla(tablero.TeclaEnter, ttAhora))
		ttMismosArgs(t, ca, "reanudar desde [3]", l.Args, []string{"reanudar", "pagos-caidos", "-m", pedido})
		if l.Slug != "pagos-caidos" || l.Rol != "writer" {
			t.Fatalf("%s: el Lanzar es de pagos-caidos y del rol de la accion: %#v", ca, l)
		}
	}
}

// ---------------------------------------------------------------- A10 da8fe2: la Narracion del sobre en curso

// CA-556, CA-558, hallazgo da8fe2: la Narracion de [0] de una tarjeta con
// running es la del sobre en curso, con su linea exacta: "  <rol> · <paso> ·
// <tiempo de [3] con ' de <limite>'> · ultimo evento hace <n>s" ("último" y
// "Narración" con UTF-8, sin acento en ASCII) y un evento por linea; una
// segunda LeerDetalle la actualiza; sin eventos no dice "ultimo evento".
func TestCA556_CA558_LaNarracionDelSobreEnCurso(t *testing.T) {
	for _, utf := range []bool{false, true} {
		ca := fmt.Sprintf("CA-556 (hallazgo da8fe2, utf8=%v)", utf)
		c := ttInfraAPI(true)
		d := tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c},
			Sobre: tablero.Sobre{EnvelopeID: "env-api", RunID: "run-api",
				Eventos: []runcmd.Event{ttEvento(-65, "tool", "Read x.go"), ttEvento(-5, "text", "leyendo el spec")}}}
		m := ttModelo(ttConfig(200, 50, utf), ttTablero(c))
		m.LeerDetalle(d)
		titulo, ultimo := " Narracion", "ultimo"
		if utf {
			titulo, ultimo = " Narración", "último"
		}
		narr := func() []string {
			t.Helper()
			got := ttCajaDe(t, ca, m, "[0]").ttTexto()
			for i, l := range got {
				if l == titulo {
					return got[i+1:]
				}
			}
			t.Fatalf("%s: [0] tiene la parte %q:\n%s", ca, titulo, strings.Join(got, "\n"))
			return nil
		}
		base := "  writer · run 3/6 · 12:41 de 45:00"
		if g := narr(); len(g) < 3 || g[0] != base+" · "+ultimo+" evento hace 5s" || g[1] != "  11:58:55 tool Read x.go" || g[2] != "  11:59:55 text leyendo el spec" {
			t.Fatalf("%s: la Narracion del sobre en curso es %q y sus eventos: %q", ca, base+" · "+ultimo+" evento hace 5s", g)
		}
		// una segunda lectura la actualiza
		d.Sobre.Eventos = append(d.Sobre.Eventos, ttEvento(-1, "text", "escribiendo"))
		m.LeerDetalle(d)
		if g := narr(); len(g) < 4 || g[0] != base+" · "+ultimo+" evento hace 1s" || g[3] != "  11:59:59 text escribiendo" {
			t.Fatalf("%s: una segunda LeerDetalle actualiza la Narracion: %q", ca, g)
		}
		// sin eventos: sin "ultimo evento"
		d.Sobre.Eventos = nil
		m.LeerDetalle(d)
		g := narr()
		if len(g) < 1 || g[0] != base {
			t.Fatalf("%s: sin eventos, la Narracion es %q (sin 'ultimo evento'): %q", ca, base, g)
		}
		for _, l := range g {
			if strings.Contains(l, "evento hace") {
				t.Fatalf("%s: sin eventos no dice 'ultimo evento hace': %q", ca, g)
			}
		}
	}
}

// ---------------------------------------------------------------- A11 b5b345: e y los margenes del spec

// CA-557, hallazgo b5b345: e en [0] de una tarjeta abre el spec en la linea
// del primer criterio con una linea a la vista: la primera del markdown que,
// sin sus espacios, empieza con "- " + id + ":" (CA-1 no es CA-10, una con
// sangria cuenta, "-CA-3:" sin espacio no), o 1 si ninguno esta a la vista.
func TestCA557_ELineaDelCriterioConMargenesHostiles(t *testing.T) {
	c, d := ttTarjetaLarga()
	for i := 0; i < 6; i++ { // la tira larga deja la parte D fuera de la vista de arriba
		c.Meter = append(c.Meter, ttSeg(fmt.Sprintf("x%d", i), fmt.Sprintf("extra %d", i), boardcmd.SegFalta, "relleno"))
	}
	c.Evidence.Untraced = []string{"CA-1", "CA-2", "CA-3"}
	d.Detalle.Card = c
	d.Detalle.Spec = boardcmd.SpecDetail{Path: ".hoom/specs/infra-api.md", Exists: true,
		Markdown: "# Spec\n\n## Criterios de aceptacion\n\n- CA-10: el diez\n- CA-1: el uno\n   - CA-2: el dos\n-CA-3: mal escrito\n- CA-3: el tres\n"}
	d.Detalle.Criteria = []boardcmd.CriterionTrace{{ID: "CA-1", Text: "el uno"}, {ID: "CA-2", Text: "el dos"}, {ID: "CA-3", Text: "el tres"},
		{ID: "CA-10", Text: "el diez", TracedBy: boardcmd.TracedTest}}
	m := ttModelo(ttConfig(80, 24, false), ttTablero(c))
	m.LeerDetalle(d)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	vista := func() []string { return ttCajaDe(t, "CA-557", m, "[0]").ttTexto() }
	for _, l := range vista() {
		if strings.Contains(l, "el uno") || strings.Contains(l, "el diez") {
			t.Fatalf("CA-557 (guarda): arriba de todo no hay criterios a la vista: %q", vista())
		}
	}
	abre := func(que string, linea int) {
		t.Helper()
		ttAbreLocal(t, "CA-557 (hallazgo b5b345)", que, m.Tecla("e", ttAhora), ".hoom/specs/infra-api.md", linea)
	}
	abre("e arriba de todo, sin criterios a la vista", 1)
	for _, x := range []struct {
		texto string
		linea int
	}{{"el uno", 6}, {"el dos", 7}, {"el tres", 9}} {
		ttSinOrdenes(t, "CA-557", "]c", ttTeclas(m, "]", "c"))
		if v := vista(); v[0] != "  x "+x.texto {
			t.Fatalf("CA-557 (guarda): ]c deja arriba la linea de %q: %q", x.texto, v)
		}
		abre("e con '"+x.texto+"' arriba", x.linea)
	}
}

// ---------------------------------------------------------------- A12 3c80d0: la seleccion sigue a su tarjeta

// CA-578 (v1 CA-535), hallazgo 3c80d0: la seleccion sigue a SU tarjeta cuando
// aparece otra antes: con a, b y c en Backlog y b elegida, una lectura que
// suma una tarjeta de backlog antes de a deja elegida b ([2] y [0]); lo mismo
// con la fila elegida de [3].
func TestCA578_CA535_LaSeleccionSigueASuTarjeta(t *testing.T) {
	b := ttTablero(ttBacklog("a", "A"), ttBacklog("b", "B"), ttBacklog("c", "C"))
	m := ttModelo(ttConfig(120, 32, false), b)
	m.Tecla("j", ttAhora)
	if m.Elegida() != "b" {
		t.Fatalf("CA-578 (guarda): j elige b: %q", m.Elegida())
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttBacklog("nueva", "Nueva"), ttBacklog("a", "A"), ttBacklog("b", "B"), ttBacklog("c", "C"))})
	if m.Elegida() != "b" || ttSlugElegido(t, m) != "b" || ttCajaDe(t, "CA-578", m, "[0]").Titulo != "[0] b · B" {
		t.Fatalf("CA-578 (hallazgo 3c80d0): con otra tarjeta antes, la seleccion sigue en b: %q\n%s", m.Elegida(), ttPantalla(m))
	}
	corre := func(s string) boardcmd.Card { return ttCorre(s, "writer", "env-"+s, "run-"+s) }
	r := ttModelo(ttConfig(120, 32, false), ttTablero(corre("a"), corre("b"), corre("c")))
	ttTeclas(r, "3", "j")
	if e := ttElegidaEn(ttCajaDe(t, "CA-578", r, "[3]")); !strings.HasPrefix(e, "b ") {
		t.Fatalf("CA-578 (guarda): j en [3] elige b: %q", e)
	}
	r.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(corre("nueva"), corre("a"), corre("b"), corre("c"))})
	if e := ttElegidaEn(ttCajaDe(t, "CA-578", r, "[3]")); !strings.HasPrefix(e, "b ") || r.Elegida() != "b" {
		t.Fatalf("CA-578 (hallazgo 3c80d0): con otra fila antes, la de [3] sigue en b: %q (elegida %q)\n%s", e, r.Elegida(), ttPantalla(r))
	}
}

// ---------------------------------------------------------------- B1, B2: nada a ciegas

// ttNadaACiegas: con el modal Pedir abierto, lleva el foco al boton de la
// accion y exige que el modal entre en las B lineas de los paneles y muestre
// "Se va a ejecutar", el comando y los botones con la accion elegida; enter
// ahi o lanza (y cada flag de sus Args, con su valor, esta entero en el
// comando que el modal mostro) o no ordena nada y la linea de mensajes dice
// ttNoEntra, en rojo con color (CA-565, CA-568, hallazgos f069bc, 0d34e9 y
// 48a4a2). Devuelve el Lanzar, o nil.
func ttNadaACiegas(t *testing.T, ca string, m *tablero.Modelo, cfg tablero.Config, etiqueta string) *tablero.Lanzar {
	t.Helper()
	for i := 0; i < 12 && !(m.Lugar() == tablero.LugarPedirBotones && ttFoco(ttModal(t, ca, m, cfg.Ancho)) == "botones"); i++ {
		m.Tecla(tablero.TeclaTab, ttAhora)
	}
	if m.Lugar() != tablero.LugarPedirBotones {
		t.Fatalf("%s (guarda): tab llega al boton de la accion: %q", ca, m.Lugar())
	}
	ttForma(t, ca, m, cfg.Ancho, cfg.Alto, cfg.Color)
	mc := ttModal(t, ca, m, cfg.Ancho)
	if mc.Fila < 0 || mc.Fila+mc.Alto > cfg.Alto-2 {
		t.Fatalf("%s: el modal entra en las %d lineas de los paneles: linea %d, alto %d", ca, cfg.Alto-2, mc.Fila+1, mc.Alto)
	}
	todo := strings.Join(mc.Textos, "\n")
	cmd := mc.ttComando()
	if !strings.Contains(todo, "Se va a ejecutar:") || !strings.HasPrefix(cmd, "hoom ") || !strings.Contains(todo, "[ "+etiqueta+" ]") {
		t.Fatalf("%s: el modal muestra 'Se va a ejecutar', el comando y los botones con '[ %s ]' elegido:\n%s", ca, etiqueta, todo)
	}
	ords := m.Tecla(tablero.TeclaEnter, ttAhora)
	if len(ttLanzares(ords)) == 0 {
		if len(ords) != 0 {
			t.Fatalf("%s: un comando que no entra no ordena nada: %#v", ca, ords)
		}
		ttDiceMensaje(t, ca, "enter sobre la accion sin que entre", m, cfg.Color, ttNoEntra)
		if m.Corriendo() != 0 || len(m.Registro()) != 0 {
			t.Fatalf("%s: lo que no entra no corre ni va al Registro: %d %#v", ca, m.Corriendo(), m.Registro())
		}
		return nil
	}
	l := ttUnaLanzar(t, ca, "enter sobre la accion", ords)
	for i, a := range l.Args {
		if !strings.HasPrefix(a, "--") {
			continue
		}
		if i+1 >= len(l.Args) {
			t.Fatalf("%s: el flag %q lleva su valor: %q", ca, a, l.Args)
		}
		v := l.Args[i+1]
		if !strings.Contains(cmd, a+" "+v) && !strings.Contains(cmd, a+" '"+v+"'") && !strings.Contains(cmd, a+` "`+v+`"`) {
			t.Fatalf("%s: nada a ciegas: el flag %s %s del Lanzar esta entero en el comando que se mostro: %q", ca, a, v, cmd)
		}
	}
	return &l
}

// ttSelectores son las etiquetas de los cinco selectores de Pedir (ASCII).
var ttSelectores = []string{"Provider", "Modelo", "Esfuerzo", "Tope", "Tiempo limite"}

// ttEsSelector dice si la linea del modal es la de un selector.
func ttEsSelector(l string) bool {
	for _, et := range append([]string{"Tiempo límite"}, ttSelectores...) {
		if strings.HasPrefix(l, et) {
			return true
		}
	}
	return false
}

// ttTextosGrises son los textos de la linea de ayuda y de los why de los
// selectores que el modal Pedir cede primero: las notas, los why grises, los
// origenes con texto propio y la linea de "por defecto del provider".
var ttTextosGrises = []string{"no esta en la lista de", "puede rechazarlo", "hoom no le pasa", "no acepta tope", "pide al menos",
	"la sesion es de", "de review.timeout", "hoom.yaml fija", "el que elige"}

// ttSoloLoQueNoCede exige que en el modal Pedir que no entra, despues de
// ceder la linea de ayuda, los why que bajan a su linea, las palabras de
// origen y las lineas vacias, queden solo el Pedido y su cuadro, los cinco
// selectores, "Se va a ejecutar", el comando y los botones (CA-565, CA-568,
// hallazgos f069bc, 0d34e9 y 28a616; la ayuda y los why se reconocen por su
// contenido, no por su sangria: ronda 1c).
func ttSoloLoQueNoCede(t *testing.T, ca string, mc ttModalCaja, etiqueta string) {
	t.Helper()
	todo := strings.Join(mc.Textos, "\n")
	for _, g := range ttTextosGrises {
		if strings.Contains(todo, g) {
			t.Fatalf("%s: la linea de ayuda, los why y los origenes ceden primero: queda %q\n%s", ca, g, todo)
		}
	}
	enComando := false
	for _, l := range mc.Textos {
		esBotones := strings.Contains(l, "[ ") && strings.Contains(l, "Cancelar") && strings.Contains(l, etiqueta)
		switch {
		case strings.HasPrefix(l, "+") || strings.HasPrefix(l, "┌") || strings.HasPrefix(l, "└") ||
			strings.HasPrefix(l, "| ") || strings.HasPrefix(l, "│ ") || l == "Pedido":
			// el Pedido y su cuadro
		case ttEsSelector(l):
		case strings.HasPrefix(l, "Se va a ejecutar:"):
			enComando = true
		case enComando && !esBotones && (strings.HasPrefix(l, "hoom ") || (strings.HasPrefix(l, "  ") && strings.TrimSpace(l) != "")):
			// el comando
		case esBotones:
			enComando = false
		default:
			t.Fatalf("%s: despues de ceder lo que cede, el modal tiene solo el Pedido, los selectores, el comando y los botones: sobra %q\n%s", ca, l, todo)
		}
	}
}

// ttLineaGris es el texto (Wm-6 = c celdas) de una linea gris de los
// selectores (la de ayuda, o un why en su linea): desde la columna 15 si
// entra ahi; si no, desde la primera columna del texto del modal; cortada
// solo si tampoco entra ahi (decision del orquestador, ronda 1c).
func ttLineaGris(texto string, c int) string {
	switch {
	case 15+ttCeldas(texto) <= c:
		return strings.Repeat(" ", 15) + texto
	case ttCeldas(texto) <= c:
		return texto
	}
	return ttCortaA(texto, c)
}

// CA-565, CA-568 ("Nada a ciegas"), hallazgos f069bc y 0d34e9: el modal
// Pedir que no entra cede, en orden, la linea de ayuda de los selectores, los
// why grises que bajan a su linea y las palabras de origen (despues, lo de
// siempre); en todos los casos muestra "Se va a ejecutar", el comando y los
// botones con la accion elegida, y enter sobre la accion lanza lo que mostro
// o, si aun asi no entra, no lanza y la linea de mensajes avisa en rojo.
func TestCA565_CA568_ElPedirQueNoEntra(t *testing.T) {
	conProv := func(p string) boardcmd.Action {
		a := ttAccionWriter()
		a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), {Name: p, OK: true}}
		return a
	}
	reanudar := ttPagosCaidos().Actions[0]
	reanudar.Pedido = strings.TrimSpace(strings.Repeat("palabra ", 40))
	derecha := []tablero.Tecla{tablero.TeclaTab, tablero.TeclaDer}
	casos := []struct {
		nombre        string
		ancho, alto   int
		k             tablero.Tecla
		a             boardcmd.Action
		teclas        []tablero.Tecla
		etiqueta, arg string // arg: el flag que el Lanzar lleva, si lanza
	}{
		{"60x15, codex (no acepta tope)", 60, 15, "w", ttAccionWriter(), derecha, "Pedir al writer", "codex"},
		{"60x15, gemini (no recibe modelo)", 60, 15, "w", conProv("gemini"), derecha, "Pedir al writer", "gemini"},
		{"60x15, opencode (no recibe modelo)", 60, 15, "w", conProv("opencode"), derecha, "Pedir al writer", "opencode"},
		{"100x15, gemini", 100, 15, "w", conProv("gemini"), derecha, "Pedir al writer", "gemini"},
		{"100x16, gemini", 100, 16, "w", conProv("gemini"), derecha, "Pedir al writer", "gemini"},
		{"60x15, reanudar con un pedido largo", 60, 15, "R", reanudar, nil, "Reanudar", ""},
	}
	for _, c := range casos {
		for _, color := range []bool{false, true} {
			ca := fmt.Sprintf("CA-565 (hallazgos f069bc y 0d34e9, %s, color=%v)", c.nombre, color)
			cfg := ttConfig(c.ancho, c.alto, false)
			cfg.Color = color
			m := ttAbrirModal(t, cfg, c.k, tablero.ModalPedir, c.a)
			ttTeclas(m, c.teclas...)
			ttForma(t, ca, m, c.ancho, c.alto, color)
			mc := ttModal(t, ca, m, c.ancho)
			todo := strings.Join(mc.Textos, "\n")
			if mc.Fila+mc.Alto > c.alto-2 || !strings.Contains(todo, "Se va a ejecutar:") || !strings.HasPrefix(mc.ttComando(), "hoom ") ||
				!strings.Contains(todo, "[ "+c.etiqueta+" ]") {
				t.Fatalf("%s: con el foco en un selector, el modal entra y muestra 'Se va a ejecutar', el comando y los botones:\n%s", ca, ttPantalla(m))
			}
			// lo primero que cede: la linea de ayuda y los why en su linea, y
			// las palabras de origen (ronda 1c: por su contenido y por lo que
			// queda, no por la sangria; la linea de botones va centrada)
			ttSoloLoQueNoCede(t, ca, mc, c.etiqueta)
			for _, et := range ttSelectores {
				if mc.ttCampo(et) == "" {
					t.Fatalf("%s: el selector %s queda:\n%s", ca, et, todo)
				}
				if o := mc.ttOrigen(et); o != "" {
					t.Fatalf("%s: las palabras de origen ceden antes que las lineas: %s dice %q\n%s", ca, et, o, todo)
				}
			}
			l := ttNadaACiegas(t, ca, m, cfg, c.etiqueta)
			if l != nil && c.arg != "" && !strings.Contains(strings.Join(l.Args, " "), "--provider "+c.arg) {
				t.Fatalf("%s: lo que se lanza lleva el provider elegido: %q", ca, l.Args)
			}
		}
	}
}

// ttConvenciones es infra-api-convenciones con sus acciones de test-writer
// y de reviewer, con claude (por defecto), codex con tope y los providers
// extra.
func ttConvenciones(extra ...boardcmd.ProviderOption) boardcmd.Card {
	c := ttCard("infra-api-convenciones", "Convenciones de la API", boardcmd.ColWriter, "faltan los tests",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
	provs := append([]boardcmd.ProviderOption{ttProvClaude(true), {Name: "codex", OK: true, Budget: true}}, extra...)
	tw := ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests de las convenciones de la API")
	rv := ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", "")
	tw.Providers, rv.Providers = provs, provs
	c.Actions = []boardcmd.Action{tw, rv}
	return c
}

// CA-568, hallazgo 48a4a2: un comando que no entra en dos lineas ni
// acortando -m no se lanza: a 60x24, t en infra-api-convenciones con el
// provider (codex), el modelo, el tope (10 USD) y el tiempo limite (2h)
// cambiados, enter sobre la accion no ordena nada y la linea de mensajes
// dice en rojo "agranda la terminal: el comando no entra entero".
func TestCA568_ElComandoQueNoEntraNoSeLanza(t *testing.T) {
	cfg := ttConfig(60, 24, false)
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(ttConvenciones()))
	ttSinOrdenes(t, "CA-568", "t", m.Tecla("t", ttAhora))
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-568 (guarda): t abre Pedir: %q", m.Modal())
	}
	der4 := []tablero.Tecla{tablero.TeclaDer, tablero.TeclaDer, tablero.TeclaDer, tablero.TeclaDer}
	ttTeclas(m, tablero.TeclaTab, tablero.TeclaDer)                                      // Provider: codex
	ttTeclas(m, tablero.TeclaTab, tablero.TeclaDer)                                      // Modelo: el siguiente
	ttTeclas(m, append([]tablero.Tecla{tablero.TeclaTab, tablero.TeclaTab}, der4...)...) // Esfuerzo; Tope: 1, 2, 5, 10 USD
	ttTeclas(m, append([]tablero.Tecla{tablero.TeclaTab}, der4...)...)                   // Tiempo limite: 15m, 30m, 1h, 2h
	mc := ttModal(t, "CA-568", m, 60)
	if mc.ttValor("Provider") != "< codex >" || mc.ttValor("Modelo") == "< opus >" || mc.ttValor("Tope") != "< 10 USD >" || mc.ttValor("Tiempo limite") != "< 2h >" {
		t.Fatalf("CA-568 (guarda): los cuatro selectores cambiados: %q %q %q %q\n%s", mc.ttValor("Provider"), mc.ttValor("Modelo"),
			mc.ttValor("Tope"), mc.ttValor("Tiempo limite"), strings.Join(mc.Textos, "\n"))
	}
	if l := ttNadaACiegas(t, "CA-568 (hallazgo 48a4a2)", m, cfg, "Pedir al test-writer"); l != nil {
		t.Fatalf("CA-568 (hallazgo 48a4a2): un comando de cuatro flags cambiados no entra en dos lineas de 50 celdas y no se lanza: %#v", l)
	}
}

// CA-568, hallazgo 48a4a2 (propiedad): a cualquier tamano, en el Pedir del
// test-writer y en el del reviewer, y con cualquier cambio de los
// selectores, cuando enter sobre la accion da un Lanzar, cada flag de sus
// Args esta entero, con su valor, en el comando que el modal mostro; cuando
// no, no hay ninguna orden y la linea de mensajes avisa.
func TestCA568_PropiedadLoQueSeLanzaSeMostroEntero(t *testing.T) {
	tamanos := [][2]int{{60, 15}, {60, 24}, {70, 16}, {80, 24}, {100, 15}, {120, 30}, {200, 40}}
	rnd := rand.New(rand.NewSource(48))
	lanzados := 0
	for i := 0; i < 120; i++ {
		tm := tamanos[rnd.Intn(len(tamanos))]
		cfg := ttConfig(tm[0], tm[1], i%2 == 0)
		ca := fmt.Sprintf("CA-568 (hallazgo 48a4a2, caso %d, %dx%d)", i, tm[0], tm[1])
		m := ttModelo(cfg, ttTablero(ttConvenciones(boardcmd.ProviderOption{Name: "gemini", OK: true})))
		k, etiqueta := tablero.Tecla("t"), "Pedir al test-writer"
		if rnd.Intn(3) == 0 {
			k, etiqueta = "r", "Pedir al reviewer"
		}
		m.Tecla(k, ttAhora)
		if m.Modal() != tablero.ModalPedir {
			t.Fatalf("%s (guarda): t abre Pedir: %q", ca, m.Modal())
		}
		for j := 0; j < 4; j++ {
			for k := 1 + rnd.Intn(3); k > 0; k-- {
				m.Tecla(tablero.TeclaTab, ttAhora)
			}
			if m.Lugar() == tablero.LugarPedirBotones {
				continue // ← y → ahi eligen el boton, no un valor
			}
			for k := rnd.Intn(5); k > 0; k-- {
				m.Tecla(tablero.TeclaDer, ttAhora)
			}
		}
		if l := ttNadaACiegas(t, ca, m, cfg, etiqueta); l != nil {
			lanzados++
		}
	}
	if lanzados == 0 {
		t.Fatalf("CA-568 (guarda): en 120 casos, con pantallas anchas, alguno lanza")
	}
}

// ---------------------------------------------------------------- B4 fac86a: el modelo elegido que no esta en la lista

// CA-570, hallazgo fac86a: con claude, Modelo en fable (elegido a mano) y
// despues Provider codex (con origen.model de hoom.yaml): Modelo sigue en
// fable, la linea gris dice "fable no esta en la lista de codex: puede
// rechazarlo" (con el foco en Provider y en Modelo) y el comando lleva
// --model fable.
func TestCA570_ElModeloElegidoQueNoEstaEnLaLista(t *testing.T) {
	cfg := ttAncho()
	cfg.Color = true
	a := ttAccionWriter()
	a.Pedido = "hola"
	m := ttAbrirModal(t, cfg, "w", tablero.ModalPedir, a)
	ttTeclas(m, tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaIzq) // Modelo: fable
	if v := ttMC(t, m).ttValor("Modelo"); v != "< fable >" {
		t.Fatalf("CA-570 (guarda): ← desde opus elige fable: %q", v)
	}
	ttTeclas(m, tablero.TeclaShiftTab, tablero.TeclaDer) // Provider: codex
	mc := ttMC(t, m)
	if mc.ttValor("Provider") != "< codex >" || mc.ttValor("Modelo") != "< fable >" {
		t.Fatalf("CA-570 (hallazgo fac86a): con codex, Modelo sigue en fable: %q %q", mc.ttValor("Provider"), mc.ttValor("Modelo"))
	}
	const nota = "fable no esta en la lista de codex: puede rechazarlo"
	if got := ttAyudaSel(mc); got != nota {
		t.Fatalf("CA-570 (hallazgo fac86a): con el foco en Provider la linea gris es la nota de Modelo %q: %q\n%s", nota, got, strings.Join(mc.Textos, "\n"))
	}
	m.Tecla(tablero.TeclaTab, ttAhora) // Modelo
	if got := ttAyudaSel(ttMC(t, m)); got != nota {
		t.Fatalf("CA-570 (hallazgo fac86a): con el foco en Modelo la linea gris es la nota sola: %q", got)
	}
	if mc := ttMC(t, m); !mc.ttTieneLinea(ttLineaGris(nota, 66)) {
		t.Fatalf("CA-570 (hallazgo fac86a): la nota entera, desde la columna 15 o (si no entra) desde la primera del texto: %q\n%s",
			ttLineaGris(nota, 66), strings.Join(mc.Textos, "\n"))
	}
	gris := 0
	for _, x := range m.Pantalla() {
		if strings.Contains(ttSinSGR(x), ttLineaGris(nota, 66)) {
			gris++
			if ttEstiloDe(x, nota) != "\x1b[90m" {
				t.Fatalf("CA-570 (hallazgo fac86a): la nota va en gris: %q", x)
			}
		}
	}
	if gris != 1 {
		t.Fatalf("CA-570 (hallazgo fac86a): la nota esta una vez, en la linea de ayuda: %d", gris)
	}
	if cmd := ttMC(t, m).ttComando(); !strings.Contains(cmd, "--provider codex --model fable") {
		t.Fatalf("CA-570 (hallazgo fac86a): el comando lleva --provider codex --model fable: %q", cmd)
	}
	l := ttNadaACiegas(t, "CA-570 (hallazgo fac86a)", m, cfg, "Pedir al writer")
	if l == nil {
		t.Fatalf("CA-570 (hallazgo fac86a): a 200x40 el comando entra y se lanza")
	}
	ttMismosArgs(t, "CA-570 (hallazgo fac86a)", "fable con codex", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "hola", "--provider", "codex", "--model", "fable"})
}

// ---------------------------------------------------------------- B5 a28cdb: seguir el final

// CA-557, CA-558, hallazgo a28cdb: [0] sigue el final solo de un documento
// mas largo que su vista y llevado a su final (donde esta la Narracion): el
// de una tarjeta que entra entero, con enter y j, no se va al final cuando
// una lectura lo alarga ("Que falta?" sigue a la vista); el de una tarjeta
// llevada al final con G lo sigue; el del proyecto nunca.
func TestCA557_CA558_SeguirElFinal(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	corto := tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c}}
	m := ttModelo(ttConfig(200, 50, false), ttTablero(c))
	m.LeerDetalle(corto)
	ttTeclas(m, tablero.TeclaEnter, "j")
	if v := ttCajaDe(t, "CA-557", m, "[0]").ttTexto(); !strings.HasPrefix(v[0], " Que falta?") {
		t.Fatalf("CA-557 (guarda): el documento entra entero y empieza en 'Que falta?': %q", v)
	}
	largo := corto
	largo.Detalle.Spec = boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "# Spec\n"}
	for i := 0; i < 80; i++ {
		largo.Detalle.Criteria = append(largo.Detalle.Criteria, boardcmd.CriterionTrace{ID: fmt.Sprintf("CA-%d", i+1),
			Text: fmt.Sprintf("criterio %02d", i+1), TracedBy: boardcmd.TracedTest})
	}
	m.LeerDetalle(largo)
	if v := ttCajaDe(t, "CA-557", m, "[0]").ttTexto(); !strings.HasPrefix(v[0], " Que falta?") {
		t.Fatalf("CA-557 (hallazgo a28cdb): un documento que entraba entero no se va al final cuando una lectura lo alarga: %q", v)
	}
	// llevado al final con G, la lectura que lo alarga deja el final a la vista
	c2, d2 := ttTarjetaLarga()
	n := ttModelo(ttConfig(80, 24, false), ttTablero(c2))
	n.LeerDetalle(d2)
	ttTeclas(n, tablero.TeclaEnter, "G")
	antes := ttCajaDe(t, "CA-557", n, "[0]").ttTexto()
	const ultima = "  11:59:55 text leyendo el spec"
	if antes[len(antes)-1] != ultima {
		t.Fatalf("CA-557 (guarda): G deja la ultima linea (el ultimo evento) abajo de todo: %q", antes)
	}
	for i := 0; i < 10; i++ {
		d2.Detalle.Findings = append(d2.Detalle.Findings, ttHallazgo(fmt.Sprintf("y%02d", i), finding.StatusOpen, "low", "", fmt.Sprintf("mas %02d", i)))
	}
	n.LeerDetalle(d2)
	if despues := ttCajaDe(t, "CA-557", n, "[0]").ttTexto(); despues[len(despues)-1] != ultima {
		t.Fatalf("CA-558 (hallazgo a28cdb): al final, una lectura que alarga el documento deja su final a la vista: %q", despues)
	}
	// el del proyecto nunca sigue el final
	p := ttModelo(ttConfig(80, 24, false), ttTableroPrincipal())
	pr := ttProyectoVerde()
	problemas := func(n int) {
		for i := len(pr.Problemas); i < n; i++ {
			pr.Problemas = append(pr.Problemas, boardcmd.Problem{ID: boardcmd.ProbSinSpec, Slug: fmt.Sprintf("p%02d", i), What: "sin spec", Action: "hoom pedir arquitecto x"})
		}
	}
	problemas(30)
	p.LeerProyecto(pr)
	ttTeclas(p, "1", tablero.TeclaEnter, "G")
	antes = ttCajaDe(t, "CA-557", p, "[0]").ttTexto()
	problemas(40)
	p.LeerProyecto(pr)
	if despues := ttCajaDe(t, "CA-557", p, "[0]").ttTexto(); despues[0] != antes[0] {
		t.Fatalf("CA-557 (hallazgo a28cdb): [0] del proyecto no sigue el final: %q, era %q", despues[0], antes[0])
	}
}

// ---------------------------------------------------------------- B6 02b9e8: el archivo, del dir de su tarjeta

// CA-559, hallazgo 02b9e8: el archivo de un hallazgo es relativo al dir de
// evidencia de la tarjeta de su tarea (como en la v1): con infra-api en
// .hoom/worktrees/infra-api, e en [4] (y en su [0]) abre
// ".hoom/worktrees/infra-api/internal/api/router.go" en la linea 41 y
// ArchivoElegido da lo mismo; sin la tarjeta en el tablero o con dir ".", el
// archivo solo; las rutas que salen siguen sin leerse ni abrirse.
func TestCA559_ElArchivoEsDelDirDeLaTarjeta(t *testing.T) {
	c := ttInfraAPI(false)
	c.Evidence.Dir = ".hoom/worktrees/infra-api"
	b := ttTablero(c, ttNucTenancy())
	con := func(cfg tablero.Config, h finding.Item) *tablero.Modelo {
		m := ttModelo(cfg, b)
		m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h}})
		m.Tecla("4", ttAhora)
		return m
	}
	h := ttHallazgo("h-high", finding.StatusOpen, "high", "internal/api/router.go:41", "el router acepta todo")
	const ruta = ".hoom/worktrees/infra-api/internal/api/router.go"
	for _, enCero := range []bool{false, true} {
		m := con(ttAncho(), h)
		if enCero {
			m.Tecla(tablero.TeclaEnter, ttAhora)
		}
		if r, l := m.ArchivoElegido(); r != ruta || l != 41 {
			t.Fatalf("CA-559 (hallazgo 02b9e8): ArchivoElegido es %q:41 (desde [0]: %v): %q:%d", ruta, enCero, r, l)
		}
		ords := m.Tecla("e", ttAhora)
		if len(ords) != 1 {
			t.Fatalf("CA-559 (hallazgo 02b9e8): e da una orden: %#v", ords)
		}
		if a, ok := ords[0].(tablero.AbrirEditor); !ok || a.Archivo != ruta || a.Linea != 41 {
			t.Fatalf("CA-559 (hallazgo 02b9e8): e abre %q en la linea 41: %#v", ruta, ords[0])
		}
	}
	// sin su tarjeta en el tablero, o con dir ".", el archivo solo
	otro := h
	otro.Task = "no-esta"
	for _, x := range []struct {
		h     finding.Item
		ruta  string
		linea int
	}{{otro, "internal/api/router.go", 41}, {ttHallazgoNuc(), "internal/tenant/tenant.go", 7}} {
		if r, l := con(ttAncho(), x.h).ArchivoElegido(); r != x.ruta || l != x.linea {
			t.Fatalf("CA-559 (hallazgo 02b9e8): el hallazgo de %q es %s:%d: %q:%d", x.h.Task, x.ruta, x.linea, r, l)
		}
	}
	// las que salen siguen rechazadas
	cfg := ttAncho()
	cfg.Color = true
	for _, file := range []string{"/etc/passwd", "../../../../x.go"} {
		m := con(cfg, ttHallazgo("f", finding.StatusOpen, "high", file, "d"))
		if r, _ := m.ArchivoElegido(); r != "" {
			t.Fatalf("CA-559 (hallazgo 02b9e8): %q sale del proyecto aun desde el dir de la tarjeta: ArchivoElegido es \"\": %q", file, r)
		}
		ttSinOrdenes(t, "CA-559 (hallazgo 02b9e8)", "e en "+file, m.Tecla("e", ttAhora))
		if file == "/etc/passwd" {
			ttDiceMensaje(t, "CA-559 (hallazgo 02b9e8)", "e en "+file, m, true, ttFuera(file))
			continue
		}
		if msg := ttMensaje(m); !strings.HasSuffix(msg, " esta fuera del proyecto") || !ttConColor(ttRojo, ttMensajeCrudo(m), "el archivo") {
			t.Fatalf("CA-559 (hallazgo 02b9e8): e en %q dice en rojo que esta fuera del proyecto: %q", file, ttMensajeCrudo(m))
		}
	}
}

// ---------------------------------------------------------------- B7 9c1e0c, B8 aae463: el ancho de los textos

// CA-577, hallazgo 9c1e0c: una marca combinante se dibuja pegada a su letra
// (NFD: "Cafe" + U+0301, "man" + U+0303 + "ana": "Café y mañana") y cada
// linea de los paneles sigue midiendo Ancho celdas, contadas con la tabla de
// los tests.
func TestCA577_LasMarcasCombinantesVanConSuLetra(t *testing.T) {
	const titulo = "Cafe\u0301 y man\u0303ana"
	c := ttTarjetaCon(ttAccionWriter())
	c.Item.Titulo = titulo
	// el selector de variacion (U+FE0F) se deja afuera: la terminal puede
	// hacer ancha a su base (doc.go, decision de la review del 2026-11-09)
	v := ttTarjetaCon(ttAccionWriter())
	v.Item.Titulo = "te \u2764\ufe0f quiero"
	for _, utf := range []bool{true, false} {
		mv := ttModelo(ttConfig(80, 24, utf), ttTablero(v))
		if !ttTiene(mv, "[0] tarjeta · te \u2764 quiero") || strings.ContainsRune(ttPantalla(mv), 0xfe0f) {
			t.Fatalf("CA-577 (hallazgo 9c1e0c, utf8=%v): el selector de variacion U+FE0F no llega a la pantalla:\n%s", utf, ttPantalla(mv))
		}
		ttForma(t, "CA-577", mv, 80, 24, false)
	}
	for _, utf := range []bool{true, false} {
		ca := fmt.Sprintf("CA-577 (hallazgo 9c1e0c, utf8=%v)", utf)
		m := ttModelo(ttConfig(80, 24, utf), ttTablero(c))
		ttForma(t, ca, m, 80, 24, false)
		vistas := 0
		for i, l := range m.Pantalla() {
			if strings.Contains(l, "[0] tarjeta") {
				vistas++
				if !strings.Contains(l, "[0] tarjeta · "+titulo) {
					t.Fatalf("%s: el titulo de [0] lleva cada marca pegada a su letra (%q): %q", ca, titulo, l)
				}
			}
			if i < 22 && ttCeldas(l) != 80 {
				t.Fatalf("%s: la linea %d de los paneles mide 80 celdas (la marca no ocupa una): %d %q", ca, i+1, ttCeldas(l), l)
			}
		}
		if vistas != 1 {
			t.Fatalf("%s: el titulo de [0] esta una vez: %d\n%s", ca, vistas, ttPantalla(m))
		}
	}
}

// CA-577, CA-534, hallazgo aae463: los caracteres de formato (ZWSP U+200B,
// ZWJ U+200D, U+2060, el BOM U+FEFF y la etiqueta U+E0020) se neutralizan
// como los otros controles ("?"), en un titulo y en una linea del archivo de
// un hallazgo; y ninguna linea pasa de Ancho, medida con la tabla de los
// tests.
func TestCA577_CA534_LosCaracteresDeFormato(t *testing.T) {
	const crudo = "a\u200bb\u200dc\u2060d\ufeffe\U000E0020f"
	const neutro = "a?b?c?d?e?f"
	formato := func(r rune) bool { return r == 0x200b || r == 0x200d || r == 0x2060 || r == 0xfeff || r == 0xe0020 }
	for _, utf := range []bool{true, false} {
		ca := fmt.Sprintf("CA-577 (hallazgo aae463, utf8=%v)", utf)
		ver := func(m *tablero.Modelo, donde, quiere string) {
			t.Helper()
			if !ttTiene(m, quiere) {
				t.Fatalf("%s: %s muestra %q:\n%s", ca, donde, quiere, ttPantalla(m))
			}
			for i, l := range m.Pantalla() {
				for _, r := range l {
					if formato(r) {
						t.Fatalf("%s: %s: la linea %d no lleva el caracter de formato %U: %q", ca, donde, i+1, r, l)
					}
				}
				if ttCeldas(l) > 120 {
					t.Fatalf("%s: %s: la linea %d mide a lo sumo 120 celdas: %d %q", ca, donde, i+1, ttCeldas(l), l)
				}
			}
		}
		c := ttInfraAPI(false)
		c.Item.Titulo = "titulo " + crudo
		m := ttModelo(ttConfig(120, 32, utf), ttTablero(c))
		m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{ttHallazgo("h1", finding.StatusOpen, "high", "x.go:2", "desc")}})
		ver(m, "el titulo de [0]", "[0] infra-api · titulo "+neutro)
		m.Tecla("4", ttAhora)
		m.LeerArchivo(tablero.Archivo{Ruta: "x.go", Linea: 2, Desde: 1, Lineas: []string{"uno " + crudo, "dos"}})
		ver(m, "una linea del archivo", "uno "+neutro)
	}
}

// ---------------------------------------------------------------- B11 140c81 (en el modelo): los avisos

// CA-562, hallazgo 140c81: los veredictos que no se pudieron leer llegan en
// Proyecto.Avisos y [0] de Proyecto los muestra, uno por linea y
// neutralizados, bajo " Avisos" (despues de Check, antes de Doctor, con una
// linea vacia antes); sin avisos esa parte no esta.
func TestCA562_LosAvisosDeLosVeredictos(t *testing.T) {
	const aviso = "no se pudo leer .hoom/verdicts/20261108T090000_bad111.json: "
	p := ttProyectoVerde()
	p.Avisos = []string{aviso + ttHostil}
	m := ttModelo(ttConfig(200, 50, false), ttTableroPrincipal())
	m.LeerProyecto(p)
	ttTeclas(m, "1", tablero.TeclaEnter)
	cero := ttCajaDe(t, "CA-562", m, "[0]").ttTexto()
	donde := func(s string) int {
		for i, l := range cero {
			if l == s {
				return i
			}
		}
		return -1
	}
	iCheck, iAvisos, iDoctor := donde(" Check"), donde(" Avisos"), donde(" Doctor")
	if iCheck < 0 || iAvisos < iCheck || iDoctor < iAvisos || cero[iAvisos-1] != "" || cero[iAvisos+1] != "  "+aviso+ttNeutro || cero[iAvisos+2] != "" {
		t.Fatalf("CA-562 (hallazgo 140c81): [0] del proyecto: ' Avisos' entre Check y Doctor con el aviso neutralizado:\n%s", strings.Join(cero, "\n"))
	}
	ttSinControles(t, "CA-562 (hallazgo 140c81)", "[0] del proyecto con avisos", m, false)
	m.LeerProyecto(ttProyectoVerde())
	for _, l := range ttCajaDe(t, "CA-562", m, "[0]").ttTexto() { // releida despues de la lectura
		if l == " Avisos" {
			t.Fatalf("CA-562 (hallazgo 140c81): sin Proyecto.Avisos, [0] no tiene ' Avisos'")
		}
	}
}

// ---------------------------------------------------------------- B14 db18ee: el check mide desde su veredicto

// CA-560, hallazgo db18ee (decision del orquestador): la linea 2 de [1] mide
// desde el veredicto al que se refiere el check (VeredictoDelCheck; sin el,
// el ultimo cuando es el del check), no desde el mas nuevo; el [0] de
// Proyecto sigue mostrando el ultimo. Caso: un completo viejo y un parcial
// nuevo.
func TestCA560_ElCheckMideDesdeSuVeredicto(t *testing.T) {
	viejo := ttVeredicto() // completo, verde, de hace dos horas: el del check
	nuevo := &statuscmd.LastVerdict{ID: "20261108T115500_def456", Verdict: "green", Partial: true, CreatedAt: ttAhora.Add(-5 * time.Minute),
		Summary: verdict.Summary{Pass: 1}}
	linea2 := func(p tablero.Proyecto) (*tablero.Modelo, string) {
		m := ttModelo(ttConfig(120, 32, false), ttTableroPrincipal())
		m.LeerProyecto(p)
		got := ttCajaDe(t, "CA-560", m, "[1]").ttTexto()
		if len(got) < 2 {
			t.Fatalf("CA-560: [1] tiene sus lineas: %q", got)
		}
		return m, got[1]
	}
	p := ttProyectoVerde()
	p.Veredicto, p.VeredictoDelCheck = nuevo, viejo
	m, l := linea2(p)
	if l != " check VERDE · verify hace 2h" {
		t.Fatalf("CA-560 (hallazgo db18ee): la linea 2 de [1] mide desde el veredicto del check (hace 2h), no desde el parcial nuevo: %q", l)
	}
	ttTeclas(m, "1", tablero.TeclaEnter)
	ttIgualesAlPrincipio(t, "CA-560 (hallazgo db18ee)", "[0] del proyecto", ttCajaDe(t, "CA-560", m, "[0]").ttTexto(), []string{
		" Veredicto",
		"  20261108T115500_def456 · hace 5m · VERDE · parcial",
		"  pass 1 · fail 0 · error 0 · absent 0",
		"  .hoom/verdicts/20261108T115500_def456.json",
		"",
		" Check",
		"  check VERDE · verify hace 2h",
	})
	// sin VeredictoDelCheck, el ultimo cuando es el del check (mismo id); y
	// con los dos siendo el mismo
	for _, del := range []*statuscmd.LastVerdict{nil, viejo} {
		q := ttProyectoVerde()
		q.VeredictoDelCheck = del
		if _, l := linea2(q); l != " check VERDE · verify hace 2h" {
			t.Fatalf("CA-560 (hallazgo db18ee): con el ultimo siendo el del check (VeredictoDelCheck nil: %v), mide desde el: %q", del == nil, l)
		}
	}
}

// ---------------------------------------------------------------- B15 dceff5: el reviewer y ctrl-e

// CA-564, hallazgo dceff5: en el modal del reviewer (sin Pedido) la fila de
// ctrl-e no esta en la linea de teclas y ctrl-e no hace nada, en un selector
// y en los botones; el writer si la tiene.
func TestCA564_ElReviewerNoTieneCtrlE(t *testing.T) {
	w := ttPedir(t, "w", ttAccionWriter())
	w.Tecla(tablero.TeclaTab, ttAhora)
	const conE = " tab campo · <> cambiar · enter ver opciones · ctrl-e editor · esc cerrar"
	if got := ttLineaTeclas(w); got != conE {
		t.Fatalf("CA-564 (guarda): la linea de teclas de un selector del writer lleva ctrl-e: %q", got)
	}
	m := ttPedir(t, "r", ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""))
	if m.Lugar() != tablero.LugarPedirSelector {
		t.Fatalf("CA-568 (guarda): el reviewer empieza en un selector: %q", m.Lugar())
	}
	if got := ttLineaTeclas(m); got != strings.Replace(conE, " · ctrl-e editor", "", 1) {
		t.Fatalf("CA-564 (hallazgo dceff5): en el reviewer la linea de teclas no lleva ctrl-e: %q", got)
	}
	for _, donde := range []string{"un selector", "los botones"} {
		if donde == "los botones" {
			for i := 0; i < 8 && m.Lugar() != tablero.LugarPedirBotones; i++ {
				m.Tecla(tablero.TeclaTab, ttAhora)
			}
			if strings.Contains(ttLineaTeclas(m), "ctrl-e") {
				t.Fatalf("CA-564 (hallazgo dceff5): en los botones del reviewer la linea de teclas no lleva ctrl-e: %q", ttLineaTeclas(m))
			}
		}
		antes := ttFoto(m)
		ttSinOrdenes(t, "CA-564 (hallazgo dceff5)", "ctrl-e en "+donde+" del reviewer", m.Tecla(tablero.TeclaCtrlE, ttAhora))
		if ttFoto(m) != antes {
			t.Fatalf("CA-564 (hallazgo dceff5): ctrl-e en %s del reviewer no hace nada", donde)
		}
	}
}
