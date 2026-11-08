// Regresiones de la ronda 1 sobre el modelo (review completa
// 20261108T231004_195bc7 de tablero-en-la-terminal), escritas desde el
// contrato de hoy (doc.go y go doc del paquete), sin leer la implementacion:
// las confirmaciones atadas a lo que muestran (CA-529), los archivos fuera
// del proyecto (CA-522, CA-524), los controles en los textos (CA-534), las
// cajas que no entran (CA-527, CA-529, CA-533), el aviso de fin (CA-530) y
// la guarda de que lo que corre es lo que se muestra (CA-525, CA-532).
package tablero_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// ttBarraDice exige que la barra 1 diga msg: sin color, al final (" · " +
// msg); con color, en rojo si rojo.
func ttBarraDice(t *testing.T, ca, que string, m *tablero.Modelo, color, rojo bool, msg string) {
	t.Helper()
	b := ttBarra1(m)
	if !color {
		if !strings.HasSuffix(b, " · "+msg) {
			t.Fatalf("%s: %s: la barra dice %q: %q", ca, que, msg, b)
		}
		return
	}
	if !strings.Contains(b, msg) || (rojo && !ttConColor(ttRojo, b, msg)) {
		t.Fatalf("%s: %s: la barra dice %q (en rojo: %v): %q", ca, que, msg, rojo, b)
	}
}

// ---------------------------------------------------------------- CA-529: atadas a lo que muestran

// ttCambio es el mensaje de una confirmacion cuyos datos cambiaron.
func ttCambio(slug string) string {
	return "cambio lo que ibas a confirmar en " + slug + ": vuelve a abrirlo"
}

// ttIntegrable es una tarjeta en Tu aceptacion con integrar habilitada.
func ttIntegrable() boardcmd.Card {
	c := ttCard("lista", "Lista para integrar", boardcmd.ColTuAceptacion, "lista", ttSeg("s", "s", boardcmd.SegHecho, "d"))
	c.Actions = []boardcmd.Action{ttAccionIntegrar()}
	return c
}

// CA-529 (hallazgo ecb288): una firma esta atada a lo que muestra. Abierta A
// (o I) y elegido Firmar, una lectura del tablero donde la tarjeta cambia lo
// que la firma dice (su titulo, su cantidad de criterios, quien firma), donde
// la accion queda deshabilitada o donde desaparece, la cierra: la vista
// vuelve a la lista (de donde se abrio), la barra dice en rojo el why de la
// accion o "cambio lo que ibas a confirmar en <slug>: vuelve a abrirlo", y
// el enter que sigue no ordena nada.
func TestCA529_LaFirmaEstaAtadaALoQueMuestra(t *testing.T) {
	const whyA = "el spec cambio despues de abrir la confirmacion"
	const whyI = "falta el veredicto verde de la base"
	casos := []struct {
		nombre  string
		tecla   tablero.Tecla
		accion  string
		base    func() boardcmd.Card
		cambiar func(c *boardcmd.Card)
		why     string // "" = el mensaje de cambio
	}{
		{"A: el titulo", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Item.Titulo = "Tenancy del nucleo, otra" }, ""},
		{"A: los criterios", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Evidence.Criteria = 6 }, ""},
		{"A: quien firma", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Actions[0].Signer = "Otra Persona <otra@hoom.dev>" }, ""},
		{"A: deshabilitada", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Actions[0].Enabled, c.Actions[0].Why = false, whyA }, whyA},
		{"A: desaparece", "A", boardcmd.ActAprobar, ttNucTenancy, func(c *boardcmd.Card) { c.Actions = []boardcmd.Action{} }, ""},
		{"I: el titulo", "I", boardcmd.ActIntegrar, ttIntegrable, func(c *boardcmd.Card) { c.Item.Titulo = "Lista para integrar, otra" }, ""},
		{"I: deshabilitada", "I", boardcmd.ActIntegrar, ttIntegrable, func(c *boardcmd.Card) { c.Actions[0].Enabled, c.Actions[0].Why = false, whyI }, whyI},
	}
	for _, cs := range casos {
		for _, color := range []bool{false, true} {
			que := fmt.Sprintf("%s (color=%v)", cs.nombre, color)
			cfg := ttAncho()
			cfg.Color = color
			base := cs.base()
			nuevo := cs.base() // otra copia: nada se comparte con lo que tiene el modelo
			cs.cambiar(&nuevo)
			if cs.why == "" {
				// guarda: el cambio cambia de verdad lo que la firma dice
				a0, _ := base.Action(cs.accion)
				if a1, ok := nuevo.Action(cs.accion); ok && tarjetacmd.TextoFirma(base, a0) == tarjetacmd.TextoFirma(nuevo, a1) {
					t.Fatalf("CA-529 (guarda): %s no cambia el texto de la firma: %q", que, tarjetacmd.TextoFirma(nuevo, a1))
				}
			}
			m := ttModelo(cfg, ttTablero(base))
			ttSinOrdenes(t, "CA-529", que+": "+string(cs.tecla), m.Tecla(cs.tecla, ttAhora))
			if m.Vista() != tablero.VistaConfirmacion {
				t.Fatalf("CA-529: %s: %q abre la confirmacion, la vista es %q", que, cs.tecla, m.Vista())
			}
			ttSinOrdenes(t, "CA-529", que+": → elige Firmar", m.Tecla(tablero.TeclaDer, ttAhora))
			ords := m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(nuevo)})
			if l := ttLanzares(ords); len(l) != 0 {
				t.Fatalf("CA-529: %s: la lectura que cambia la firma no ordena nada: %#v", que, l)
			}
			if m.Vista() != tablero.VistaLista {
				t.Fatalf("CA-529: %s: la confirmacion se cierra y se vuelve a la lista (de donde se abrio), la vista es %q\n%s", que, m.Vista(), ttPantalla(m))
			}
			msg := cs.why
			if msg == "" {
				msg = ttCambio(base.Slug)
			}
			ttBarraDice(t, "CA-529", que, m, color, true, msg)
			if l := ttLanzares(m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second))); len(l) != 0 {
				t.Fatalf("CA-529: %s: el enter que sigue no firma nada: %#v", que, l)
			}
		}
	}
}

// CA-529 (hallazgo ecb288): una firma cuyos datos no cambiaron firma como
// antes, aunque la tarjeta cambie en lo que la firma no muestra (el plain,
// el gasto) o el tablero traiga otra tarjeta.
func TestCA529_LaFirmaSinCambiosSigueValiendo(t *testing.T) {
	cambios := []struct {
		nombre  string
		cambiar func(c *boardcmd.Card) []boardcmd.Card
	}{
		{"igual", func(c *boardcmd.Card) []boardcmd.Card { return nil }},
		{"el plain", func(c *boardcmd.Card) []boardcmd.Card { c.Plain = "otro plain de la tarjeta"; return nil }},
		{"el gasto", func(c *boardcmd.Card) []boardcmd.Card {
			c.Spend = boardcmd.Spend{CostUSD: ttF(9.99), Runs: 3}
			return nil
		}},
		{"otra tarjeta", func(c *boardcmd.Card) []boardcmd.Card { return []boardcmd.Card{ttBacklog("nueva", "Tarjeta nueva")} }},
	}
	for _, cb := range cambios {
		for _, k := range []struct {
			tecla tablero.Tecla
			base  func() boardcmd.Card
			args  []string
		}{
			{"A", ttNucTenancy, []string{"aprobar", "nuc-tenancy", "--si"}},
			{"I", ttIntegrable, []string{"integrar", "lista", "--si"}},
		} {
			que := cb.nombre + " con " + string(k.tecla)
			m := ttModelo(ttAncho(), ttTablero(k.base()))
			m.Tecla(k.tecla, ttAhora)
			m.Tecla(tablero.TeclaDer, ttAhora)
			for i := 1; i <= 2; i++ {
				nuevo := k.base()
				otras := cb.cambiar(&nuevo)
				ords := m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Duration(2*i) * time.Second), Tablero: ttTablero(append(otras, nuevo)...)})
				if l := ttLanzares(ords); len(l) != 0 {
					t.Fatalf("CA-529: %s: leer no firma: %#v", que, l)
				}
			}
			if m.Vista() != tablero.VistaConfirmacion {
				t.Fatalf("CA-529: %s: sin cambios en lo que muestra, la confirmacion sigue abierta, la vista es %q (barra %q)", que, m.Vista(), ttBarra1(m))
			}
			l := ttUnaLanzar(t, "CA-529", que+": enter sobre Firmar", m.Tecla(tablero.TeclaEnter, ttAhora.Add(5*time.Second)))
			ttMismosArgs(t, "CA-529", que, l.Args, k.args)
		}
	}
}

// CA-529 (hallazgo ecb288): con la tarjeta abierta, la firma tambien esta
// atada al spec que mostro su detalle: un LeerDetalle con otro markdown la
// cierra (se vuelve a la pestana de donde se abrio, la barra dice el
// mensaje de cambio y enter no firma); con el mismo, firma.
func TestCA529_LaFirmaAtadaAlSpecDeLaTarjetaAbierta(t *testing.T) {
	c := ttNucTenancy()
	md1 := "# Spec: tenancy\n\n## Criterios de aceptacion\n\n- CA-1: uno.\n"
	md2 := md1 + "- CA-2: un criterio que el arquitecto agrego despues.\n"
	det := func(md string) tablero.Detalle {
		return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: ttNucTenancy(),
			Spec: boardcmd.SpecDetail{Path: ".hoom/specs/nuc-tenancy.md", Exists: true, Markdown: md}}}
	}
	abrir := func(color bool) *tablero.Modelo {
		t.Helper()
		cfg := ttAncho()
		cfg.Color = color
		m := ttModelo(cfg, ttTablero(c))
		m.Tecla(tablero.TeclaEnter, ttAhora)
		m.Tecla("2", ttAhora)
		m.LeerDetalle(det(md1))
		if m.Vista() != tablero.VistaSpec {
			t.Fatalf("CA-521: 2 es la pestana Spec, la vista es %q", m.Vista())
		}
		ttSinOrdenes(t, "CA-529", "A desde la pestana Spec", m.Tecla("A", ttAhora))
		if m.Vista() != tablero.VistaConfirmacion {
			t.Fatalf("CA-529: A desde la tarjeta abierta abre la confirmacion, la vista es %q", m.Vista())
		}
		m.Tecla(tablero.TeclaDer, ttAhora)
		return m
	}
	for _, color := range []bool{false, true} {
		m := abrir(color)
		m.LeerDetalle(det(md2))
		if m.Vista() != tablero.VistaSpec {
			t.Fatalf("CA-529: otro spec cierra la firma y vuelve a la pestana Spec, la vista es %q\n%s", m.Vista(), ttPantalla(m))
		}
		ttBarraDice(t, "CA-529", "otro spec", m, color, true, ttCambio(c.Slug))
		if l := ttLanzares(m.Tecla(tablero.TeclaEnter, ttAhora.Add(time.Second))); len(l) != 0 {
			t.Fatalf("CA-529: con otro spec el enter que sigue no firma: %#v", l)
		}
	}
	// el mismo spec (y el mismo tablero): firma
	m := abrir(false)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(ttNucTenancy())})
	m.LeerDetalle(det(md1))
	if m.Vista() != tablero.VistaConfirmacion {
		t.Fatalf("CA-529: con el mismo spec la confirmacion sigue, la vista es %q (barra %q)", m.Vista(), ttBarra1(m))
	}
	l := ttUnaLanzar(t, "CA-529", "enter sobre Firmar con el mismo spec", m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second)))
	ttMismosArgs(t, "CA-529", "aprobar", l.Args, []string{"aprobar", "nuc-tenancy", "--si"})
}

// CA-529 (hallazgo ecb288): la confirmacion de s esta atada a sus rutas: si
// las rutas sin guardar cambian (otra, una de mas) se cierra con el mensaje de
// cambio; si guardar queda deshabilitada, con su why; en ningun caso el enter
// que sigue guarda. Con las mismas rutas, enter guarda.
func TestCA529_GuardarAtadoASusRutas(t *testing.T) {
	conRutas := func(rutas ...string) boardcmd.Card {
		c := ttTarjetaCon(ttAccionGuardar(true, "", rutas...))
		c.Unsynced = append([]string{}, rutas...)
		return c
	}
	cambios := []struct {
		nombre string
		nuevo  boardcmd.Card
		msg    string
	}{
		{"otra ruta", conRutas("internal/a.go", "internal/c.go"), ttCambio("tarjeta")},
		{"una de mas", conRutas("internal/a.go", "internal/b.go", "internal/c.go"), ttCambio("tarjeta")},
		{"deshabilitada", ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttWhyGuardar},
	}
	for _, cb := range cambios {
		for _, color := range []bool{false, true} {
			que := fmt.Sprintf("%s (color=%v)", cb.nombre, color)
			cfg := ttAncho()
			cfg.Color = color
			m := ttModelo(cfg, ttTablero(conRutas("internal/a.go", "internal/b.go")))
			ttSinOrdenes(t, "CA-529", "s", m.Tecla("s", ttAhora))
			if m.Vista() != tablero.VistaConfirmacion {
				t.Fatalf("CA-529: s abre su confirmacion, la vista es %q", m.Vista())
			}
			if l := ttLanzares(m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cb.nuevo)})); len(l) != 0 {
				t.Fatalf("CA-529: %s: leer no guarda: %#v", que, l)
			}
			if m.Vista() != tablero.VistaLista {
				t.Fatalf("CA-529: %s: la confirmacion de s se cierra y se vuelve a la lista, la vista es %q\n%s", que, m.Vista(), ttPantalla(m))
			}
			ttBarraDice(t, "CA-529", que, m, color, true, cb.msg)
			if l := ttLanzares(m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second))); len(l) != 0 {
				t.Fatalf("CA-529: %s: el enter que sigue no guarda: %#v", que, l)
			}
		}
	}
	m := ttModelo(ttAncho(), ttTablero(conRutas("internal/a.go", "internal/b.go")))
	m.Tecla("s", ttAhora)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(conRutas("internal/a.go", "internal/b.go"))})
	l := ttUnaLanzar(t, "CA-529", "enter con las mismas rutas", m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second)))
	ttMismosArgs(t, "CA-529", "guardar", l.Args, []string{"guardar", "tarjeta"})
}

// ---------------------------------------------------------------- CA-522, CA-524: fuera del proyecto

// ttFuera es el mensaje de un archivo que sale del proyecto.
func ttFuera(ruta string) string {
	return "el archivo " + strconv.Quote(ruta) + " esta fuera del proyecto"
}

// CA-524 (hallazgo f28d31): enter sobre un hallazgo cuyo archivo sale del
// proyecto (absoluto, o con ".." que sube) no ordena nada: la barra dice en
// rojo "el archivo <ruta con strconv.Quote> esta fuera del proyecto" durante
// DuracionAviso. Uno local con ".." adentro abre el editor.
func TestCA524_HallazgoFueraDelProyecto(t *testing.T) {
	casos := []struct{ file, ruta string }{
		{"/etc/passwd", "/etc/passwd"},
		{"../../x.go", "../../x.go"},
		{"../fuera.go:12", "../fuera.go"},
		{`../x "y".go`, `../x "y".go`},
		{"../\x1b[2J.go", "../\x1b[2J.go"},
	}
	var hs []finding.Item
	for i, cs := range casos {
		hs = append(hs, ttHallazgo(fmt.Sprintf("f%d", i), finding.StatusOpen, "high", cs.file, fmt.Sprintf("hallazgo %d", i)))
	}
	hs = append(hs, ttHallazgo("local", finding.StatusOpen, "low", "dentro/../a.go", "local con puntos"))
	for _, color := range []bool{false, true} {
		cfg := ttAncho()
		cfg.Color = color
		c := ttInfraAPI(false)
		c.Evidence.Dir = "."
		m := ttAbrir(t, cfg, c, tablero.Detalle{})
		m.Tecla("4", ttAhora)
		m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Findings: hs}})
		ahora := ttAhora
		for i, cs := range casos {
			ahora = ahora.Add(time.Second)
			que := fmt.Sprintf("enter sobre el hallazgo de %q (color=%v)", cs.file, color)
			ttSinOrdenes(t, "CA-524", que, m.Tecla(tablero.TeclaEnter, ahora))
			ttBarraDice(t, "CA-524", que, m, color, true, ttFuera(cs.ruta))
			if i == 0 {
				m.Tic(ahora.Add(tablero.DuracionAviso + time.Second))
				if strings.Contains(ttBarra1(m), "fuera del proyecto") {
					t.Fatalf("CA-524: el aviso dura DuracionAviso: %q", ttBarra1(m))
				}
				ahora = ahora.Add(tablero.DuracionAviso + time.Second)
			}
			m.Tecla("j", ahora)
		}
		ords := m.Tecla(tablero.TeclaEnter, ahora)
		if len(ords) != 1 {
			t.Fatalf("CA-524: un archivo local (con un .. que no sale) abre el editor: %#v", ords)
		}
		if _, ok := ords[0].(tablero.AbrirEditor); !ok {
			t.Fatalf("CA-524: un archivo local ordena AbrirEditor: %#v", ords[0])
		}
	}
}

// CA-522 (hallazgo f28d31): o en la pestana Spec con una ruta de spec que
// sale del proyecto no ordena nada y la barra dice en rojo "el archivo
// <ruta> esta fuera del proyecto".
func TestCA522_SpecFueraDelProyecto(t *testing.T) {
	for _, ruta := range []string{"../../fuera.md", "/etc/hoom/spec.md"} {
		for _, color := range []bool{false, true} {
			cfg := ttAncho()
			cfg.Color = color
			c := ttInfraAPI(false)
			spec := boardcmd.SpecDetail{Path: ruta, Exists: true, Markdown: "# Spec\n\nuna linea\n"}
			m := ttAbrir(t, cfg, c, tablero.Detalle{Detalle: boardcmd.Detail{Spec: spec}})
			m.Tecla("2", ttAhora)
			m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec}})
			que := fmt.Sprintf("o con el spec en %q (color=%v)", ruta, color)
			ttSinOrdenes(t, "CA-522", que, m.Tecla("o", ttAhora))
			ttBarraDice(t, "CA-522", que, m, color, true, ttFuera(ruta))
		}
	}
}

// ---------------------------------------------------------------- CA-534: controles en los textos

// ttHostil lleva lo que una tarjeta o un agente puede meter en un texto:
// OSC 52 (escribir el portapapeles), CSI 2J (borrar la pantalla), BEL, CR,
// un C1 (U+009B, CSI de 8 bits), U+202E (invierte el texto), un tab, DEL,
// U+2066 y U+200E. ttNeutro es como lo muestra la pantalla: el tab es un
// espacio y cada otro control un "?".
const (
	ttHostil = "a\x1b]52;c;aGk=\ab\x1b[2Jc\x07d\re\u009bf\u202eg\th\x7fi\u2066j\u200ek"
	ttNeutro = "a?]52;c;aGk=?b?[2Jc?d?e?f?g h?i?j?k"
)

// ttEsControl dice si r es un control que ninguna linea puede llevar: C0,
// DEL, C1 o uno bidi del contrato.
func ttEsControl(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// ttSinControles exige que ninguna linea de la pantalla lleve un control (con
// color, fuera de las SGR \x1b[...m) y devuelve la pantalla sin las SGR.
func ttSinControles(t *testing.T, ca, donde string, m *tablero.Modelo, color bool) string {
	t.Helper()
	var ls []string
	for i, l := range m.Pantalla() {
		if color {
			l = ttSGR.ReplaceAllString(l, "")
		}
		for _, r := range l {
			if ttEsControl(r) {
				t.Fatalf("%s: %s: la linea %d lleva el control %U (solo puede haber SGR): %q", ca, donde, i+1, r, l)
			}
		}
		ls = append(ls, l)
	}
	return strings.Join(ls, "\n")
}

// ttTarjetaHostil es una tarjeta (y su detalle) con ttHostil en el titulo,
// el plain, el why, el rojo, el doctor, el pedido, el spec, un hallazgo, un
// evento del sobre y la historia.
func ttTarjetaHostil() (boardcmd.Card, tablero.Detalle) {
	w := ttAccionWriter()
	w.Pedido = "pedido " + ttHostil
	c := ttTarjetaCon(w, ttAccionGuardar(false, "why "+ttHostil), ttAccionAprobar())
	c.Item.Titulo = "titulo " + ttHostil
	c.Plain = "plain " + ttHostil
	c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v-1", Reason: "reason " + ttHostil, Plain: "rojo " + ttHostil}
	c.Doctor = []boardcmd.Problem{{ID: boardcmd.ProbSpecEditado, Slug: "tarjeta", What: "spec editado",
		Plain: "doctor " + ttHostil, Action: "hoom aprobar tarjeta"}}
	ms := int64(61000)
	d := tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
		Spec: boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true,
			Markdown: "# Spec\n\nlinea " + ttHostil + "\n\n- CA-1: criterio " + ttHostil + "\n"},
		Criteria: []boardcmd.CriterionTrace{{ID: "CA-1", TracedBy: boardcmd.TracedTest}},
		Findings: []finding.Item{ttHallazgo("f1", finding.StatusOpen, "high", "a.go", "desc "+ttHostil)},
		Work: []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "env-h", RunID: "run-h", Role: "writer", Status: "entregable",
			Stage: "run", Step: 6, Steps: 6, StartedAt: ttAhora.Add(-time.Hour), DurationMS: &ms}},
	},
		Historia: boardcmd.Timeline{Slug: c.Slug, Entries: []boardcmd.TimelineEntry{{At: ttAhora.Add(-time.Hour), Kind: boardcmd.KindItem,
			Who: "Henry Orellana", Plain: "historia " + ttHostil, Summary: "item tarjeta"}}},
		Sobre: tablero.Sobre{EnvelopeID: "env-h", RunID: "run-h", Eventos: []runcmd.Event{ttEvento(-5, "text", "evento "+ttHostil)}},
	}
	return c, d
}

// CA-534 (hallazgo c82abf): ningun texto de la tarjeta o de los agentes llega
// a la terminal como una secuencia. En cada pantalla (la lista, la barra, el
// menu, las cinco pestanas, el formulario y la firma), sin color ninguna
// linea lleva un "\x1b" ni otro control; con color, solo SGR. El tab se ve
// como un espacio y cada otro control como "?". El pedido que se lanza es el
// de la accion, sin tocar: neutralizar es solo de la pantalla.
func TestCA534_ControlesEnLosTextos(t *testing.T) {
	for _, color := range []bool{false, true} {
		for _, utf := range []bool{false, true} {
			cfg := tablero.Config{Proyecto: "demo", Ancho: 200, Alto: 40, Color: color, UTF8: utf}
			ca := fmt.Sprintf("CA-534 (color=%v, utf8=%v)", color, utf)
			ver := func(m *tablero.Modelo, donde string, quiere ...string) {
				t.Helper()
				s := ttSinControles(t, ca, donde, m, color)
				for _, q := range quiere {
					if !strings.Contains(s, q) {
						t.Fatalf("%s: %s muestra %q (el tab como espacio, los controles como ?):\n%s", ca, donde, q, s)
					}
				}
			}
			c, d := ttTarjetaHostil()
			m := ttModelo(cfg, ttTablero(c))
			ver(m, "la lista", "plain "+ttNeutro)
			m.Tecla("s", ttAhora)
			ver(m, "la barra con el why", "why "+ttNeutro)
			m.Tecla(tablero.TeclaEspacio, ttAhora)
			ver(m, "el menu", "why "+ttNeutro)
			m.Tecla(tablero.TeclaEsc, ttAhora)
			m.Tecla(tablero.TeclaEnter, ttAhora)
			m.LeerDetalle(d)
			ver(m, "Resumen", " tarjeta · titulo "+ttNeutro, "plain "+ttNeutro, "rojo "+ttNeutro, "doctor "+ttNeutro)
			for _, p := range []struct {
				tecla  tablero.Tecla
				nombre string
				quiere []string
			}{
				{"2", "Spec", []string{"linea " + ttNeutro, "criterio " + ttNeutro}},
				{"3", "Sobre", []string{"evento " + ttNeutro}},
				{"4", "Hallazgos", []string{"desc " + ttNeutro}},
				{"5", "Historia", []string{"historia " + ttNeutro}},
			} {
				m.Tecla(p.tecla, ttAhora)
				m.LeerDetalle(d)
				ver(m, "la pestana "+p.nombre, p.quiere...)
			}
			m.Tecla(tablero.TeclaEsc, ttAhora)
			m.Tecla("w", ttAhora)
			if m.Vista() != tablero.VistaFormulario {
				t.Fatalf("%s: w abre el formulario, la vista es %q", ca, m.Vista())
			}
			ver(m, "el formulario", "pedido "+ttNeutro)
			l := ttUnaLanzar(t, ca, "enter en el formulario", m.Tecla(tablero.TeclaEnter, ttAhora))
			ttMismosArgs(t, ca, "el pedido se lanza como es", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "pedido " + ttHostil})
			m.Tecla("A", ttAhora)
			if m.Vista() != tablero.VistaConfirmacion {
				t.Fatalf("%s: A abre la firma, la vista es %q", ca, m.Vista())
			}
			ver(m, "la firma")
		}
	}
}

// ---------------------------------------------------------------- CA-527, CA-529, CA-533: cajas que no entran

// ttBordeAbajo dice si l es el borde de abajo de una caja ASCII a ese ancho.
func ttBordeAbajo(l string, ancho int) bool { return l == " +"+strings.Repeat("-", ancho-4)+"+" }

// ttCajaEntra exige la forma de la pantalla y que la caja entre en el
// cuerpo: empieza en la linea 2 y su borde de abajo esta a lo sumo en la
// linea A-3; el separador sigue en A-2.
func ttCajaEntra(t *testing.T, ca string, m *tablero.Modelo, ancho, alto int) {
	t.Helper()
	ttForma(t, ca, m, ancho, alto, false)
	p := m.Pantalla()
	if !strings.HasPrefix(p[1], " + ") {
		t.Fatalf("%s: la caja empieza en la linea 2: %q\n%s", ca, p[1], ttPantalla(m))
	}
	abajo := 0
	for i, l := range p {
		if ttBordeAbajo(l, ancho) {
			abajo = i + 1
		}
	}
	if abajo == 0 || abajo > alto-3 {
		t.Fatalf("%s: la caja entra en el cuerpo (lineas 2..%d), su borde de abajo esta en la linea %d:\n%s", ca, alto-3, abajo, ttPantalla(m))
	}
	if p[alto-3] != strings.Repeat("-", ancho) {
		t.Fatalf("%s: la linea A-2 sigue siendo el separador: %q", ca, p[alto-3])
	}
}

func ttTiene(ls []string, s string) bool {
	for _, l := range ls {
		if l == s {
			return true
		}
	}
	return false
}

// CA-527, CA-533 (hallazgo 9f25a9): a 60x15 el formulario del writer con un
// pedido de tres lineas deja afuera, en orden, las lineas vacias, la de las
// teclas y la primera del pedido, y conserva los campos, la linea del cursor
// y la linea ->. Enter lanza lo mismo que antes.
func TestCA527_CA533_FormularioChicoConservaLaFlecha(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = strings.Repeat("abcdefghij", 12) // 120 celdas: 54 + 54 + 12 en un interior de 54
	m := ttFormulario(t, ttConfig(60, 15, false), "w", a)
	ttCajaEntra(t, "CA-533", m, 60, 15)
	if got := ttFlecha(t, "CA-527", m); !strings.HasPrefix(got, "hoom pedir writer tarjeta -m ") {
		t.Fatalf("CA-527: la linea -> siempre queda: %q", got)
	}
	for _, et := range []string{"Provider", "Modelo", "Esfuerzo", "Presupuesto", "Tiempo limite", "Pedido"} {
		if ttCampo(m, et) == "" {
			t.Fatalf("CA-527: a 60x15 el campo %s queda:\n%s", et, ttPantalla(m))
		}
	}
	caja := ttCaja(m)
	for _, l := range caja {
		if l == "" {
			t.Fatalf("CA-527: lo primero que se va son las lineas vacias:\n%s", ttPantalla(m))
		}
		if strings.HasPrefix(l, "enter lanzar") {
			t.Fatalf("CA-527: la linea de las teclas se va antes que las del pedido:\n%s", ttPantalla(m))
		}
	}
	p := a.Pedido
	if !ttTiene(caja, p[108:]+"_") || !ttTiene(caja, p[54:108]) || ttTiene(caja, p[:54]) {
		t.Fatalf("CA-527: del pedido se van las lineas de arriba hasta la del cursor (queda lo que entra):\n%s", ttPantalla(m))
	}
	l := ttUnaLanzar(t, "CA-528", "enter a 60x15", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-528", "a 60x15", l.Args, []string{"pedir", "writer", "tarjeta", "-m", p})
}

// CA-527, CA-533 (hallazgo 9f25a9): a 60x15, con providers que no pueden, sus
// lineas se van (despues de las vacias) y la de Provider termina en "  (<n>
// no pueden)"; la de las teclas entonces entra. A 80x24 la caja es la de
// antes: cada uno con su why y la de Provider sin la cuenta.
func TestCA527_CA533_ProvidersQueNoPuedenAChico(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola"
	a.Providers = []boardcmd.ProviderOption{ttProvClaude(true), ttProvOpencode(),
		{Name: "gemini", OK: false, Why: "gemini no esta instalado"}, ttProvCodex(false)}
	m := ttFormulario(t, ttConfig(60, 15, false), "w", a)
	ttCajaEntra(t, "CA-533", m, 60, 15)
	if got := ttCampo(m, "Provider"); got != "  Provider       < claude >  (2 no pueden)" {
		t.Fatalf("CA-527: a 60x15 la linea de Provider cuenta los que no pueden: %q\n%s", got, ttPantalla(m))
	}
	for _, s := range []string{"opencode:", "gemini:"} {
		if l, n := ttLineaCon(m, s); n != 0 {
			t.Fatalf("CA-527: a 60x15 las lineas de los que no pueden se van: %q", l)
		}
	}
	caja := ttCaja(m)
	teclas := false
	for _, l := range caja {
		teclas = teclas || strings.HasPrefix(l, "enter lanzar")
	}
	if !teclas || !ttTiene(caja, "hola_") {
		t.Fatalf("CA-527: sin las vacias y sin los que no pueden la caja ya entra: quedan las teclas y el pedido:\n%s", ttPantalla(m))
	}
	if got := ttFlecha(t, "CA-527", m); !strings.HasPrefix(got, "hoom pedir writer tarjeta -m ") {
		t.Fatalf("CA-527: la linea -> queda: %q", got)
	}
	m = ttFormulario(t, ttConfig(80, 24, false), "w", a)
	ttCajaEntra(t, "CA-527", m, 80, 24)
	if got := ttCampo(m, "Provider"); got != "  Provider       < claude >" {
		t.Fatalf("CA-527: a 80x24 la linea de Provider es la de antes: %q", got)
	}
	for _, s := range []string{"opencode: " + ttWhyOpencode, "gemini: gemini no esta instalado"} {
		if _, n := ttLineaCon(m, s); n == 0 {
			t.Fatalf("CA-527: a 80x24 cada uno que no puede va con su why (%q):\n%s", s, ttPantalla(m))
		}
	}
}

// CA-529, CA-533 (hallazgo 9f25a9): a 60x15 la confirmacion de s con veinte
// rutas muestra las primeras, "y <n> rutas mas" en una ultima linea y su
// "enter guardar · esc cancelar"; enter guarda igual.
func TestCA529_CA533_GuardarConMuchasRutasAChico(t *testing.T) {
	var rutas []string
	for i := 0; i < 20; i++ {
		rutas = append(rutas, fmt.Sprintf("internal/archivo-%02d.go", i))
	}
	c := ttTarjetaCon(ttAccionGuardar(true, "", rutas...))
	c.Unsynced = append([]string{}, rutas...)
	m := ttModelo(ttConfig(60, 15, false), ttTablero(c))
	ttSinOrdenes(t, "CA-529", "s", m.Tecla("s", ttAhora))
	if m.Vista() != tablero.VistaConfirmacion {
		t.Fatalf("CA-529: s abre su confirmacion, la vista es %q", m.Vista())
	}
	ttCajaEntra(t, "CA-533", m, 60, 15)
	caja := ttCaja(m)
	if len(caja) < 3 || caja[0] != "Se guarda en git:" {
		t.Fatalf("CA-529: la confirmacion empieza con 'Se guarda en git:':\n%s", ttPantalla(m))
	}
	j := 0
	for j < len(rutas) && 1+j < len(caja) && caja[1+j] == rutas[j] {
		j++
	}
	if j == 0 || 1+j >= len(caja) || caja[1+j] != fmt.Sprintf("y %d rutas mas", len(rutas)-j) {
		t.Fatalf("CA-529: muestra las primeras rutas (%d) y 'y %d rutas mas' en una ultima linea:\n%s", j, len(rutas)-j, ttPantalla(m))
	}
	resto := caja[2+j:]
	if len(resto) > 0 && resto[0] == "" {
		resto = resto[1:]
	}
	if len(resto) != 1 || resto[0] != "enter guardar · esc cancelar" {
		t.Fatalf("CA-529: despues de las rutas queda 'enter guardar · esc cancelar' (y nada mas): %q\n%s", resto, ttPantalla(m))
	}
	l := ttUnaLanzar(t, "CA-529", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-529", "guardar a 60x15", l.Args, []string{"guardar", "tarjeta"})
}

// CA-529, CA-533 (hallazgo 9f25a9): una firma cuyo texto no entra entero
// nunca se firma: enter sobre Firmar no ordena nada y la barra dice en rojo
// "agranda la terminal: la firma no entra entera". Tambien si la terminal se
// achica con la firma abierta. Donde entra, firma.
func TestCA529_CA533_FirmaQueNoEntraNoSeFirma(t *testing.T) {
	const msg = "agranda la terminal: la firma no entra entera"
	c := ttNucTenancy()
	c.Item.Titulo = strings.TrimSpace(strings.Repeat("palabra ", 380)) // unas 3000 celdas: 17 lineas a 194
	firmar := func(m *tablero.Modelo) []tablero.Orden {
		t.Helper()
		if m.Vista() != tablero.VistaConfirmacion {
			t.Fatalf("CA-529: A abre la confirmacion, la vista es %q", m.Vista())
		}
		m.Tecla(tablero.TeclaDer, ttAhora)
		return m.Tecla(tablero.TeclaEnter, ttAhora)
	}
	for _, tam := range []struct {
		ancho, alto int
		color       bool
	}{{200, 15, false}, {200, 15, true}, {60, 15, false}} {
		cfg := ttConfig(tam.ancho, tam.alto, false)
		cfg.Color = tam.color
		que := fmt.Sprintf("a %dx%d (color=%v)", tam.ancho, tam.alto, tam.color)
		m := ttModelo(cfg, ttTablero(c))
		m.Tecla("A", ttAhora)
		ttSinOrdenes(t, "CA-529", "enter sobre Firmar "+que, firmar(m))
		if tam.ancho == 60 {
			// la barra se corta al ancho
			if b := ttBarra1(m); !strings.HasPrefix(b, " NORMAL · sin tarjeta fijada · agranda la terminal") {
				t.Fatalf("CA-529: %s la barra dice %q: %q", que, msg, b)
			}
			continue
		}
		ttBarraDice(t, "CA-529", que, m, tam.color, true, msg)
	}
	// abierta grande y achicada despues
	m := ttModelo(ttConfig(200, 40, false), ttTablero(c))
	m.Tecla("A", ttAhora)
	m.Tecla(tablero.TeclaDer, ttAhora)
	m.Tamano(200, 15)
	ttSinOrdenes(t, "CA-529", "enter sobre Firmar tras achicar", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttBarraDice(t, "CA-529", "tras achicar", m, false, true, msg)
	// donde entra, firma
	m = ttModelo(ttConfig(200, 40, false), ttTablero(c))
	m.Tecla("A", ttAhora)
	l := ttUnaLanzar(t, "CA-529", "enter sobre Firmar a 200x40", firmar(m))
	ttMismosArgs(t, "CA-529", "aprobar", l.Args, []string{"aprobar", "nuc-tenancy", "--si"})
}

// ---------------------------------------------------------------- CA-530: el aviso de fin

// ttCorre es una tarjeta en Writer con un running del sobre env (sin sobre si
// env es "") y del run run.
func ttCorre(slug, rol, env, run string) boardcmd.Card {
	c := ttCard(slug, "Tarjeta "+slug, boardcmd.ColWriter, "corre "+slug, ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
	c.Running = &boardcmd.Running{EnvelopeID: env, RunID: run, Role: rol, Provider: "claude", Stage: "run", Step: 2, Steps: 6, StartedAt: ttAhora}
	return c
}

// ttParada es la misma tarjeta sin running, con su plain de ahora.
func ttParada(slug string) boardcmd.Card {
	return ttCard(slug, "Tarjeta "+slug, boardcmd.ColWriter, "termino "+slug, ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
}

// CA-530 (hallazgo 5f48e8): una tarjeta vista con running del sobre E, leida
// sin running y con interrupted de E, avisa en rojo (el trabajo murio, no
// cerro entregable); lo mismo un run sin sobre (Running{RunID: R}) que queda
// interrupted de R. La que solo dejo de correr, sin rojo ni interrupted,
// avisa en verde; tambien la que muestra interrupted de otro sobre.
func TestCA530_FinInterrumpidoEsRojo(t *testing.T) {
	interr := func(slug, env, run, rol string) boardcmd.Card {
		c := ttParada(slug)
		c.Plain, c.NeedsDecision = "el trabajo se corto", true
		c.Interrupted = &boardcmd.Interrupted{EnvelopeID: env, RunID: run, Role: rol, Provider: "claude", Stage: "run", Step: 2, Steps: 6, UpdatedAt: ttAhora}
		return c
	}
	casos := []struct {
		nombre string
		antes  boardcmd.Card
		despue boardcmd.Card
		msg    string
		rojo   bool
	}{
		{"interrumpida del sobre", ttCorre("tarjeta", "writer", "env-e", "run-e"), interr("tarjeta", "env-e", "run-e", "writer"),
			"writer termino en tarjeta: el trabajo se corto", true},
		{"interrumpida del run sin sobre", ttCorre("tarjeta", "reviewer", "", "run-r"), interr("tarjeta", "", "run-r", "reviewer"),
			"reviewer termino en tarjeta: el trabajo se corto", true},
		{"solo paro", ttCorre("tarjeta", "writer", "env-e", "run-e"), ttParada("tarjeta"),
			"writer termino en tarjeta: termino tarjeta", false},
		{"interrumpida de otro sobre", ttCorre("tarjeta", "writer", "env-e", "run-e"), interr("tarjeta", "env-otro", "run-otro", "writer"),
			"writer termino en tarjeta: el trabajo se corto", false},
	}
	for _, cs := range casos {
		cfg := ttAncho()
		cfg.Color = true
		m := ttModelo(cfg, ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		b := ttBarra1(m)
		color := ttVerde
		if cs.rojo {
			color = ttRojo
		}
		if !strings.Contains(b, cs.msg) || !ttConColor(color, b, cs.msg) {
			t.Fatalf("CA-530: %s: la barra dice %q en %s: %q", cs.nombre, cs.msg, map[bool]string{true: "rojo", false: "verde"}[cs.rojo], b)
		}
		// sin color, el mismo texto al final de la barra
		m = ttModelo(ttAncho(), ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		ttBarraDice(t, "CA-530", cs.nombre, m, false, false, cs.msg)
	}
}

// CA-530 (hallazgo 8a3a66): dos tarjetas que dejan de correr en la misma
// lectura avisan una tras otra, en el orden del tablero, cada una durante
// DuracionFin; despues, ninguna.
func TestCA530_DosFinesEnUnaLectura(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttCorre("zeta", "test-writer", "env-z", "run-z"), ttCorre("alfa", "writer", "env-a", "run-a")))
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttParada("zeta"), ttParada("alfa"))})
	primero, segundo := "test-writer termino en zeta: termino zeta", "writer termino en alfa: termino alfa"
	ttBarraDice(t, "CA-530", "al leer", m, false, false, primero)
	m.Tic(t0.Add(tablero.DuracionFin - time.Second))
	ttBarraDice(t, "CA-530", "antes de DuracionFin", m, false, false, primero)
	m.Tic(t0.Add(tablero.DuracionFin + 500*time.Millisecond))
	ttBarraDice(t, "CA-530", "pasado el primero", m, false, false, segundo)
	m.Tic(t0.Add(2*tablero.DuracionFin - time.Second))
	ttBarraDice(t, "CA-530", "el segundo dura DuracionFin", m, false, false, segundo)
	m.Tic(t0.Add(2*tablero.DuracionFin + time.Second))
	if b := ttBarra1(m); strings.Contains(b, "termino en") {
		t.Fatalf("CA-530: pasados los dos, la barra no dice ningun fin: %q", b)
	}
}

// CA-530 (hallazgo 8a3a66): el aviso de fin le gana la barra enseguida a un
// mensaje de otra clase ("lanzando hoom ...", un why).
func TestCA530_ElAvisoDeFinTomaLaBarra(t *testing.T) {
	// "lanzando hoom pedir writer tarjeta"
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter()), ttCorre("dos", "reviewer", "env-dos", "run-dos")))
	ttIrA(t, m, "tarjeta")
	m.Tecla("w", ttAhora)
	ttUnaLanzar(t, "CA-528", "enter", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttBarraDice(t, "CA-528", "lanzado", m, false, false, "lanzando hoom pedir writer tarjeta")
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Second), Tablero: ttTablero(ttTarjetaCon(ttAccionWriter()), ttParada("dos"))})
	ttBarraDice(t, "CA-530", "tras lanzar", m, false, false, "reviewer termino en dos: termino dos")
	// un why
	m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttCorre("dos", "reviewer", "env-dos", "run-dos")))
	ttIrA(t, m, "tarjeta")
	m.Tecla("s", ttAhora)
	ttBarraDice(t, "CA-525", "el why", m, false, false, ttWhyGuardar)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Second), Tablero: ttTablero(ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttParada("dos"))})
	ttBarraDice(t, "CA-530", "tras un why", m, false, false, "reviewer termino en dos: termino dos")
}

// ---------------------------------------------------------------- CA-525, CA-532: lo que corre es lo que se muestra

// CA-532, CA-525 (hallazgo e8c2ef, guarda): para cada fila de Atajos con
// Accion, lo que corre es lo que se muestra. Los Args del Lanzar empiezan con
// las palabras de su Verbo sin "hoom" y con <slug> reemplazado: u enseguida,
// A e I con Firmar, s con enter; a, t, w, r, R y n por su formulario sin
// cambiar nada, con "-m" <pedido> cuando el formulario tiene Pedido (y su
// linea -> empieza con el Verbo). El menu muestra ese mismo Verbo.
func TestCA532_CA525_LoQueCorreEsLoQueSeMuestra(t *testing.T) {
	acciones := map[string]boardcmd.Action{
		boardcmd.ActPedirArquitecto: ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec"),
		boardcmd.ActPedirTestWriter: ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests"),
		boardcmd.ActPedirWriter:     ttAccionWriter(),
		boardcmd.ActPedirReviewer:   ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""),
		boardcmd.ActReanudar:        ttPagosCaidos().Actions[0],
		boardcmd.ActGuardar:         ttAccionGuardar(true, "", "internal/a.go"),
		boardcmd.ActAprobar:         ttAccionAprobar(),
		boardcmd.ActIntegrar:        ttAccionIntegrar(),
		tablero.AccionSiguiente:     ttAccionWriter(), // n: la primera accion de la tarjeta, de rol
	}
	vistas := 0
	for _, fila := range tablero.Atajos {
		if fila.Accion == "" {
			continue
		}
		vistas++
		if len(fila.Teclas) == 0 {
			t.Fatalf("CA-532: la fila %q no tiene teclas", fila.Hace)
		}
		k := fila.Teclas[0]
		ca := fmt.Sprintf("CA-532 (%q, %s)", k, fila.Accion)
		verbo := strings.ReplaceAll(fila.Verbo, "<slug>", "tarjeta")
		pal := strings.Fields(verbo)
		if len(pal) < 2 || pal[0] != "hoom" {
			t.Fatalf("%s: el Verbo de la fila es un verbo de hoom: %q", ca, fila.Verbo)
		}
		quiere := append([]string{}, pal[1:]...)
		var card boardcmd.Card
		acc, conAccion := acciones[fila.Accion]
		switch {
		case fila.Accion == tablero.AccionUsar:
			card = ttTarjetaCon()
		case conAccion:
			card = ttTarjetaCon(acc)
		default:
			t.Fatalf("%s: la guarda no sabe armar la accion %q: agregala a la tabla del test", ca, fila.Accion)
		}
		if conAccion && fila.Accion != tablero.AccionSiguiente {
			// el menu muestra el mismo verbo
			m := ttModelo(ttAncho(), ttTablero(card))
			m.Tecla(tablero.TeclaEspacio, ttAhora)
			l, n := ttLineaCon(m, " "+string(k)+" "+acc.Label+"  ")
			if n == 0 || !strings.HasSuffix(l, "  "+verbo) {
				t.Fatalf("%s: el menu muestra la accion con su tecla y el verbo %q: %q\n%s", ca, verbo, l, ttPantalla(m))
			}
		}
		m := ttModelo(ttAncho(), ttTablero(card))
		ords := m.Tecla(k, ttAhora)
		switch m.Vista() {
		case tablero.VistaFormulario:
			ttSinOrdenes(t, ca, "abrir el formulario", ords)
			if got := ttFlecha(t, ca, m); !strings.HasPrefix(got, verbo) {
				t.Fatalf("%s: la linea -> empieza con el verbo de la tabla %q: %q", ca, verbo, got)
			}
			if tienePedido := ttCampo(m, "Pedido") != ""; tienePedido != (acc.Role != "reviewer") {
				t.Fatalf("%s: el formulario tiene Pedido salvo el del reviewer (tiene: %v)", ca, tienePedido)
			}
			if ttCampo(m, "Pedido") != "" {
				quiere = append(quiere, "-m", strings.TrimSpace(acc.Pedido))
			}
			ords = m.Tecla(tablero.TeclaEnter, ttAhora)
		case tablero.VistaConfirmacion:
			ttSinOrdenes(t, ca, "abrir la confirmacion", ords)
			if fila.Accion == boardcmd.ActAprobar || fila.Accion == boardcmd.ActIntegrar {
				m.Tecla(tablero.TeclaDer, ttAhora)
			}
			ords = m.Tecla(tablero.TeclaEnter, ttAhora)
		}
		l := ttUnaLanzar(t, ca, "la accion", ords)
		ttMismosArgs(t, ca, "lo que corre es lo que se muestra", l.Args, quiere)
		if l.Slug != "tarjeta" || l.Rol != acc.Role {
			t.Fatalf("%s: el Lanzar es de la tarjeta y del rol de la accion (%q): %#v", ca, acc.Role, l)
		}
	}
	if vistas < 10 {
		t.Fatalf("CA-532: la tabla tiene las diez teclas de accion del spec, tiene %d", vistas)
	}
}
