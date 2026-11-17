// Los controles en los textos (CA-534 de la v1, que sigue igual: "Todo texto
// de la pantalla pasa por la neutralizacion de controles de la v1") y las
// rutas fuera del proyecto en el modelo (CA-522 y CA-524 de la v1,
// re-expresados por CA-557 y CA-559: e en el detalle de una tarjeta y en la
// vista Hallazgos, su lista y su detalle; los nombres de la v3, spec
// tablero-tu-siguiente-paso, "Los nombres de la v2"). Los hallazgos c82abf,
// f28d31 y e7f022 de la v1 siguen valiendo: aca se miran en los lugares de la
// v3.
package tablero_test

import (
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttHostil lleva lo que una tarjeta o un agente puede meter en un texto:
// OSC 52, CSI 2J, BEL, CR, un C1 (U+009B), U+202E, un tab, DEL, U+2066,
// U+200E y una secuencia con forma de SGR (\x1b[7m, que una pantalla con
// color no puede confundir con las suyas; CA-534, CA-576, hallazgo f2d7fe).
// ttNeutro es como lo muestra la pantalla: el tab es un espacio y cada otro
// control un "?".
const (
	ttHostil = "a\x1b]52;c;aGk=\ab\x1b[2Jc\x07d\re\u009bf\u202eg\th\x7fi\u2066j\u200ekl\x1b[7mm"
	ttNeutro = "a?]52;c;aGk=?b?[2Jc?d?e?f?g h?i?j?kl?[7mm"
)

func ttEsControl(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// ttSinControles exige que ninguna linea lleve un control (con color, fuera
// de las SGR; sin color, ni una SGR) y devuelve la pantalla sin las SGR
// (CA-534, CA-576, hallazgo f2d7fe: las SGR se quitan solo cuando la
// pantalla es con color).
func ttSinControles(t *testing.T, ca, donde string, m *tablero.Modelo, color bool) string {
	t.Helper()
	var ls []string
	for i, l := range m.Pantalla() {
		if color {
			l = ttSinSGR(l)
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
// el plain, el why, el rojo, el doctor, el pedido, un criterio, un hallazgo,
// un evento del sobre y la historia.
func ttTarjetaHostil() (boardcmd.Card, tablero.Detalle) {
	w := ttAccionWriter()
	w.Pedido = "pedido " + ttHostil
	c := ttTarjetaCon(w, ttAccionGuardar(false, "why "+ttHostil), ttAccionAprobar())
	c.Item.Titulo = "titulo " + ttHostil
	c.Plain = "plain " + ttHostil
	c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v-1", Reason: "reason " + ttHostil, Plain: "rojo " + ttHostil}
	c.Doctor = []boardcmd.Problem{{ID: boardcmd.ProbSpecEditado, Slug: "tarjeta", What: "spec editado", Plain: "doctor " + ttHostil, Action: "hoom aprobar tarjeta"}}
	ms := int64(61000)
	d := tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
		Spec:     boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "- CA-1: criterio " + ttHostil + "\n"},
		Criteria: []boardcmd.CriterionTrace{{ID: "CA-1", Text: "criterio " + ttHostil, TracedBy: boardcmd.TracedTest}},
		Findings: []finding.Item{ttHallazgo("f1", finding.StatusOpen, "high", "a.go", "desc "+ttHostil)},
		Work: []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "env-h", RunID: "run-h", Role: "writer", Status: "entregable",
			Stage: "run", Step: 6, Steps: 6, StartedAt: ttAhora.Add(-time.Hour), DurationMS: &ms}}},
		Historia: boardcmd.Timeline{Slug: c.Slug, Entries: []boardcmd.TimelineEntry{{At: ttAhora.Add(-time.Hour), Kind: boardcmd.KindItem,
			Who: "Henry Orellana", Plain: "historia " + ttHostil, Summary: "item tarjeta"}}},
		Sobre: tablero.Sobre{EnvelopeID: "env-h", RunID: "run-h", Eventos: []runcmd.Event{ttEvento(-5, "text", "evento "+ttHostil)}},
	}
	return c, d
}

// CA-534 (hallazgo c82abf de la v1): ningun texto de la tarjeta, del proyecto
// o de los agentes llega a la terminal como una secuencia. En cada lugar de
// la v3 (la cabecera, la caja TU SIGUIENTE PASO, la lista y el detalle de la
// tarjeta, la linea de mensajes con el why, el modal de acciones, Pedir, la
// firma, el Registro, la vista Hallazgos con el detalle del hallazgo, el
// detalle del proyecto con v) ninguna linea lleva un control; con color, solo
// SGR. El tab se ve como un espacio y cada otro control como "?". El pedido
// que se lanza es el de la accion, sin tocar: neutralizar es solo de la
// pantalla. La tarjeta hostil tiene un rojo no esperado (esta en DETENIDA: su
// AHORA y su fila dicen el plain del rojo); el plain de la tarjeta se mira
// despues con la misma tarjeta sin el rojo (TE TOCA: su AHORA, su fila y la
// caja dicen el plain), porque en DETENIDA la v3 no lo muestra.
func TestCA534_ControlesEnLosTextos(t *testing.T) {
	for _, color := range []bool{false, true} {
		for _, utf := range []bool{false, true} {
			cfg := tablero.Config{Proyecto: "demo", Ancho: 240, Alto: 60, Color: color, UTF8: utf, Editor: "vi"}
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
			m.LeerDetalle(d)
			p := ttProyectoVerde()
			p.Rama = "rama " + ttHostil
			p.Problemas = []boardcmd.Problem{{ID: boardcmd.ProbSinSpec, Slug: "x", Plain: "problema " + ttHostil, Action: "accion " + ttHostil}}
			m.LeerProyecto(p)
			m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{ttHallazgo("h1", finding.StatusOpen, "high", "x.go:2", "hallazgo "+ttHostil)}})
			// la cabecera (rama), la fila y el borde del detalle (titulo), el
			// AHORA, la fila y la caja (el plain del rojo) y las partes B a G
			ver(m, "la cabecera, la caja, la lista y el detalle de la tarjeta", "titulo "+ttNeutro, "rojo "+ttNeutro,
				"doctor "+ttNeutro, "criterio "+ttNeutro, "desc "+ttNeutro, "historia "+ttNeutro, "evento "+ttNeutro, "rama "+ttNeutro)
			if z := tv3bLeer(t, ca, m); !strings.HasPrefix(z.Detalle.Titulo, "titulo "+ttNeutro) || !strings.Contains(z.Detalle.Derecha, "tarjeta") {
				t.Fatalf("%s: el borde del detalle dice el titulo neutralizado y, a la derecha, el slug: %q / %q", ca, z.Detalle.Titulo, z.Detalle.Derecha)
			}
			sinRojo := c
			sinRojo.Red = nil
			n := ttModelo(cfg, ttTablero(sinRojo))
			dn := d
			dn.Detalle.Card = sinRojo
			n.LeerDetalle(dn)
			ver(n, "la tarjeta sin rojo (TE TOCA)", "titulo "+ttNeutro, "plain "+ttNeutro)
			m.Tecla("s", ttAhora)
			ver(m, "la linea de mensajes con el why", "why "+ttNeutro)
			m.Tecla(tablero.TeclaEspacio, ttAhora)
			ver(m, "el modal de acciones", "why "+ttNeutro[:14])
			m.Tecla(tablero.TeclaEsc, ttAhora)
			m.Tecla("w", ttAhora)
			if m.Modal() != tablero.ModalPedir {
				t.Fatalf("%s: w abre Pedir: %q", ca, m.Modal())
			}
			ver(m, "el modal Pedir", "pedido "+ttNeutro)
			m.Tecla(tablero.TeclaEnter, ttAhora)
			l := ttUnaLanzar(t, ca, "enter en el boton", m.Tecla(tablero.TeclaEnter, ttAhora))
			ttMismosArgs(t, ca, "el pedido se lanza como es", l.Args, []string{"pedir", "writer", "tarjeta", "-m", "pedido " + ttHostil})
			ver(m, "el Registro")
			m.Tecla("A", ttAhora)
			if m.Modal() != tablero.ModalFirma {
				t.Fatalf("%s: A abre la firma: %q", ca, m.Modal())
			}
			ver(m, "la firma", "titulo "+ttNeutro[:10])
			m.Tecla(tablero.TeclaEsc, ttAhora)
			m.Tecla("H", ttAhora)
			if m.Vista() != tablero.VistaHallazgos {
				t.Fatalf("%s: H abre la vista Hallazgos: %q", ca, m.Vista())
			}
			m.LeerArchivo(tablero.Archivo{Ruta: "x.go", Linea: 2, Desde: 1, Lineas: []string{"uno " + ttHostil, "dos"}})
			ver(m, "la vista Hallazgos y el detalle de un hallazgo", "hallazgo "+ttNeutro, "uno "+ttNeutro)
			m.Tecla("H", ttAhora) // de vuelta a las tarjetas: v es de la lista de tarjetas y del detalle
			m.Tecla("v", ttAhora)
			if m.Lugar() != tablero.LugarVerProyecto {
				t.Fatalf("%s: v muestra el proyecto en el detalle: %q", ca, m.Lugar())
			}
			ver(m, "el detalle del proyecto (v)", "problema "+ttNeutro, "accion "+ttNeutro, "rama "+ttNeutro)
		}
	}
}

// ---------------------------------------------------------------- las rutas fuera del proyecto

// ttConHallazgos es el modelo con esos hallazgos y la vista Hallazgos abierta
// con el foco en su lista (la v2: el foco en [4]; la v3: H).
func ttConHallazgos(cfg tablero.Config, hs []finding.Item) *tablero.Modelo {
	m := ttModelo(cfg, ttTableroPrincipal())
	m.LeerHallazgos(tablero.Hallazgos{Items: hs})
	m.Tecla("H", ttAhora)
	return m
}

// ttAbreLocal exige que ords sea un solo AbrirEditor de ruta (tal cual o
// limpia con path.Clean) en la linea linea.
func ttAbreLocal(t *testing.T, ca, que string, ords []tablero.Orden, ruta string, linea int) {
	t.Helper()
	if len(ords) != 1 {
		t.Fatalf("%s: %s da una orden AbrirEditor: %#v", ca, que, ords)
	}
	a, ok := ords[0].(tablero.AbrirEditor)
	if !ok || (a.Archivo != ruta && a.Archivo != path.Clean(ruta)) || a.Linea != linea {
		t.Fatalf("%s: %s abre %q en la linea %d: %#v", ca, que, ruta, linea, ords[0])
	}
}

// CA-559, CA-524 (hallazgos f28d31 y e7f022 de la v1): e sobre un hallazgo
// cuyo archivo sale del proyecto en algun sistema (absoluto en cualquiera, con
// una unidad, un UNC, una raiz "\" o un ".." que sube con / o \) no ordena
// nada y la linea de mensajes dice en rojo "el archivo <ruta con
// strconv.Quote> esta fuera del proyecto" durante DuracionAviso; uno local
// con un ".." que no sale abre el editor. En la lista de la vista Hallazgos y
// en su detalle (enter, en la vista Hallazgos, le da el foco al detalle).
func TestCA559_CA524_HallazgoFueraDelProyecto(t *testing.T) {
	casos := []struct{ file, ruta string }{
		{"/etc/passwd", "/etc/passwd"},
		{"../../x.go", "../../x.go"},
		{"../fuera.go:12", "../fuera.go"},
		{`../x "y".go`, `../x "y".go`},
		{"../\x1b[2J.go", "../\x1b[2J.go"},
		{"C:/fuera/x.go", "C:/fuera/x.go"},
		{"C:/fuera/x.go:12", "C:/fuera/x.go"},
		{"C:x.go", "C:x.go"},
		{`\\servidor\recurso\x.go`, `\\servidor\recurso\x.go`},
		{`\x.go`, `\x.go`},
		{`..\..\x.go`, `..\..\x.go`},
		{`sub\..\..\x.go`, `sub\..\..\x.go`},
	}
	for _, enCero := range []bool{false, true} {
		for _, color := range []bool{false, true} {
			cfg := ttAncho()
			cfg.Color = color
			ahora := ttAhora
			for _, cs := range casos {
				m := ttConHallazgos(cfg, []finding.Item{ttHallazgo("f", finding.StatusOpen, "high", cs.file, "d")})
				tv3bChicoControlesEnVista(t, m, enCero)
				ahora = ahora.Add(time.Second)
				que := fmt.Sprintf("e sobre %q (en el detalle: %v, color=%v)", cs.file, enCero, color)
				ttSinOrdenes(t, "CA-559", que, m.Tecla("e", ahora))
				ttDiceMensaje(t, "CA-559", que, m, color, ttFuera(cs.ruta))
				if len(m.Registro()) != 0 {
					t.Fatalf("CA-559: %s: nada se abre, nada va al Registro: %#v", que, m.Registro())
				}
				m.Tic(ahora.Add(tablero.DuracionAviso + time.Second))
				if strings.Contains(ttMensaje(m), "fuera del proyecto") {
					t.Fatalf("CA-559: %s: el aviso dura DuracionAviso: %q", que, ttMensaje(m))
				}
			}
			for _, local := range []string{"dentro/../a.go", "sub/../x.go", `sub\..\x.go`} {
				m := ttConHallazgos(cfg, []finding.Item{ttHallazgo("l", finding.StatusOpen, "low", local, "local")})
				tv3bChicoControlesEnVista(t, m, enCero)
				ords := m.Tecla("e", ttAhora)
				if local == `sub\..\x.go` {
					// CA-559, CA-534, hallazgo d31956: la unica orden es un
					// AbrirEditor (la ruta, tal cual o limpia segun el sistema)
					if len(ords) != 1 {
						t.Fatalf("CA-559: %q es local: e lo abre: %#v", local, ords)
					}
					if a, ok := ords[0].(tablero.AbrirEditor); !ok || (a.Archivo != local && a.Archivo != "x.go") || a.Linea != 0 {
						t.Fatalf("CA-559 (hallazgo d31956): %q es local: la unica orden es AbrirEditor en la linea 0: %#v", local, ords[0])
					}
					continue
				}
				ttAbreLocal(t, "CA-559", "e sobre "+local, ords, local, 0)
			}
		}
	}
}

// CA-557, CA-522 (hallazgos f28d31 y e7f022 de la v1): e en el detalle de
// una tarjeta (el foco ahi con tab: en la v3 enter abre el modal de su paso)
// con un spec cuya ruta sale del proyecto (relativa con "..", absoluta,
// con unidad, UNC, raiz "\" o "..\") no ordena nada y la linea de mensajes
// dice en rojo "el archivo <ruta> esta fuera del proyecto"; uno local con un
// ".." que no sale abre el editor en la linea 1.
func TestCA557_CA522_SpecFueraDelProyecto(t *testing.T) {
	abrir := func(cfg tablero.Config, ruta string) *tablero.Modelo {
		c := ttInfraAPI(false)
		m := ttModelo(cfg, ttTablero(c))
		m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
			Spec: boardcmd.SpecDetail{Path: ruta, Exists: true, Markdown: "# Spec\n\nuna linea\n"}}})
		m.Tecla(tablero.TeclaTab, ttAhora)
		if m.Lugar() != tablero.LugarVerTarjeta {
			t.Fatalf("CA-557: tab le da el foco al detalle de la tarjeta: %q", m.Lugar())
		}
		return m
	}
	for _, ruta := range []string{"../../fuera.md", "/etc/hoom/spec.md", "C:/fuera/spec.md", "C:spec.md", `\\srv\r\spec.md`, `\spec.md`, `..\spec.md`} {
		for _, color := range []bool{false, true} {
			cfg := ttAncho()
			cfg.Color = color
			m := abrir(cfg, ruta)
			que := fmt.Sprintf("e con el spec en %q (color=%v)", ruta, color)
			ttSinOrdenes(t, "CA-557", que, m.Tecla("e", ttAhora))
			ttDiceMensaje(t, "CA-557", que, m, color, ttFuera(ruta))
		}
	}
	ttAbreLocal(t, "CA-557", "e con el spec en sub/../spec.md", abrir(ttAncho(), "sub/../spec.md").Tecla("e", ttAhora), "sub/../spec.md", 1)
}

// tv3bChicoControlesEnVista exige que m este en la lista de la vista
// Hallazgos y, con enCero, le da el foco al detalle del hallazgo con enter
// (la v2: enter en [4] pasaba el foco a su [0]).
func tv3bChicoControlesEnVista(t *testing.T, m *tablero.Modelo, enCero bool) {
	t.Helper()
	if m.Vista() != tablero.VistaHallazgos || m.Lugar() != tablero.LugarHallazgos {
		t.Fatalf("CA-559: H abre la vista Hallazgos con el foco en su lista: vista %q, lugar %q", m.Vista(), m.Lugar())
	}
	if enCero {
		m.Tecla(tablero.TeclaEnter, ttAhora)
		if m.Lugar() != tablero.LugarVerHallazgo || m.Foco() != tablero.ZonaDetalle {
			t.Fatalf("CA-559: enter en la vista Hallazgos le da el foco al detalle del hallazgo: lugar %q, foco %d", m.Lugar(), int(m.Foco()))
		}
	}
}
