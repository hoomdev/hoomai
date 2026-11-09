// Las confirmaciones atadas a lo que muestran (CA-529): la firma de A e I y
// la confirmacion de s se cierran cuando cambia lo que muestran (la tarjeta,
// el texto de la firma, el spec de la tarjeta abierta, las rutas), con el why
// o "cambio lo que ibas a confirmar en <slug>: vuelve a abrirlo" en rojo, y
// nada corre. Regresiones de la ronda 1 (review completa
// 20261108T231004_195bc7 de tablero-en-la-terminal) y de la ronda 2 (review
// delta: hallazgos 20261109T003918_f0fbb9 y 20261109T003335_4581a6),
// escritas desde el contrato de hoy (doc.go y go doc del paquete), sin leer
// la implementacion.
package tablero_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

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

// ---------------------------------------------------------------- CA-529: la firma falla cerrada (ronda 2)

// CA-529 (hallazgos 20261109T003918_f0fbb9 y 20261109T003335_4581a6): una
// firma abierta desde la tarjeta abierta mientras su detalle mostraba el spec
// (LeerDetalle con Spec.Exists y su markdown) falla cerrada: una lectura de
// esa tarjeta que falla (Detalle.Err, aunque traiga el mismo spec) o que no
// trae spec (Spec.Exists false), con el tablero y el texto de la firma sin
// cambios, cuenta como un cambio. La confirmacion se cierra, la vista vuelve
// a la pestana Spec (de donde se abrio), la barra dice en rojo "cambio lo que
// ibas a confirmar en <slug>: vuelve a abrirlo" y el enter que sigue no
// ordena nada. Con A y con I.
func TestCA529_LaFirmaFallaCerradaSiElDetalleNoTraeElSpec(t *testing.T) {
	const md = "# Spec\n\n## Criterios de aceptacion\n\n- CA-1: uno.\n"
	firmas := []struct {
		tecla tablero.Tecla
		base  func() boardcmd.Card
	}{
		{"A", ttNucTenancy},
		{"I", ttIntegrable},
	}
	spec := func(c boardcmd.Card, existe bool) boardcmd.SpecDetail {
		s := boardcmd.SpecDetail{Path: ".hoom/specs/" + c.Slug + ".md", Exists: existe}
		if existe {
			s.Markdown = md
		}
		return s
	}
	lecturas := []struct {
		nombre string
		det    func(c boardcmd.Card) tablero.Detalle
	}{
		{"la lectura falla", func(c boardcmd.Card) tablero.Detalle {
			return tablero.Detalle{Slug: c.Slug, Err: errors.New("no se pudo leer la tarjeta: disco roto")}
		}},
		{"la lectura falla y trae el mismo spec", func(c boardcmd.Card) tablero.Detalle {
			return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec(c, true)},
				Err: errors.New("no se pudo leer la historia")}
		}},
		{"la lectura no trae spec", func(c boardcmd.Card) tablero.Detalle {
			return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec(c, false)}}
		}},
	}
	for _, f := range firmas {
		for _, lec := range lecturas {
			for _, color := range []bool{false, true} {
				c := f.base()
				que := fmt.Sprintf("%s: %s (color=%v)", f.tecla, lec.nombre, color)
				cfg := ttAncho()
				cfg.Color = color
				m := ttModelo(cfg, ttTablero(c))
				ttSinOrdenes(t, "CA-520", "enter abre la tarjeta", m.Tecla(tablero.TeclaEnter, ttAhora))
				if s, _ := m.Abierta(); s != c.Slug {
					t.Fatalf("CA-520: %s: enter abre %s, Abierta = %q", que, c.Slug, s)
				}
				m.Tecla("2", ttAhora)
				m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: f.base(), Spec: spec(c, true)}})
				if m.Vista() != tablero.VistaSpec {
					t.Fatalf("CA-521: %s: 2 es la pestana Spec, la vista es %q", que, m.Vista())
				}
				ttSinOrdenes(t, "CA-529", que+": "+string(f.tecla)+" desde la pestana Spec", m.Tecla(f.tecla, ttAhora))
				if m.Vista() != tablero.VistaConfirmacion {
					t.Fatalf("CA-529: %s: %q desde la tarjeta abierta abre la confirmacion, la vista es %q\n%s", que, f.tecla, m.Vista(), ttPantalla(m))
				}
				ttSinOrdenes(t, "CA-529", que+": → elige Firmar", m.Tecla(tablero.TeclaDer, ttAhora))
				// guarda: el mismo tablero (el mismo texto de la firma) no la cierra
				if l := ttLanzares(m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(f.base())})); len(l) != 0 {
					t.Fatalf("CA-529: %s: leer no firma: %#v", que, l)
				}
				if m.Vista() != tablero.VistaConfirmacion {
					t.Fatalf("CA-529 (guarda): %s: con el mismo tablero la confirmacion sigue abierta, la vista es %q (barra %q)", que, m.Vista(), ttBarra1(m))
				}
				m.LeerDetalle(lec.det(f.base()))
				if m.Vista() != tablero.VistaSpec {
					t.Fatalf("CA-529: %s: la confirmacion se cierra y la vista vuelve a la pestana Spec (de donde se abrio), la vista es %q\n%s", que, m.Vista(), ttPantalla(m))
				}
				ttBarraDice(t, "CA-529", que, m, color, true, ttCambio(c.Slug))
				if l := ttLanzares(m.Tecla(tablero.TeclaEnter, ttAhora.Add(3*time.Second))); len(l) != 0 {
					t.Fatalf("CA-529: %s: el enter que sigue no firma nada: %#v", que, l)
				}
			}
		}
	}
}
