// Los controles en los textos (CA-534 de la v1, que sigue igual: "Todo texto
// de la pantalla pasa por la neutralizacion de controles de la v1") y las
// rutas fuera del proyecto en el modelo (CA-522 y CA-524 de la v1,
// re-expresados por CA-557 y CA-559: e en [0] de una tarjeta y en [4]). Los
// hallazgos c82abf, f28d31 y e7f022 de la v1 siguen valiendo: aca se miran
// en los lugares de la v2.
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
// OSC 52, CSI 2J, BEL, CR, un C1 (U+009B), U+202E, un tab, DEL, U+2066 y
// U+200E. ttNeutro es como lo muestra la pantalla: el tab es un espacio y
// cada otro control un "?".
const (
	ttHostil = "a\x1b]52;c;aGk=\ab\x1b[2Jc\x07d\re\u009bf\u202eg\th\x7fi\u2066j\u200ek"
	ttNeutro = "a?]52;c;aGk=?b?[2Jc?d?e?f?g h?i?j?k"
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
// de las SGR) y devuelve la pantalla sin las SGR.
func ttSinControles(t *testing.T, ca, donde string, m *tablero.Modelo) string {
	t.Helper()
	var ls []string
	for i, l := range m.Pantalla() {
		l = ttSinSGR(l)
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
// la v2 (los paneles, [0] de la tarjeta, la linea de mensajes con el why, el
// modal de acciones, Pedir, la firma, el Registro, [4] y su [0], [1] y su
// [0]) ninguna linea lleva un control; con color, solo SGR. El tab se ve
// como un espacio y cada otro control como "?". El pedido que se lanza es el
// de la accion, sin tocar: neutralizar es solo de la pantalla.
func TestCA534_ControlesEnLosTextos(t *testing.T) {
	for _, color := range []bool{false, true} {
		for _, utf := range []bool{false, true} {
			cfg := tablero.Config{Proyecto: "demo", Ancho: 240, Alto: 60, Color: color, UTF8: utf, Editor: "vi"}
			ca := fmt.Sprintf("CA-534 (color=%v, utf8=%v)", color, utf)
			ver := func(m *tablero.Modelo, donde string, quiere ...string) {
				t.Helper()
				s := ttSinControles(t, ca, donde, m)
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
			ver(m, "los paneles y [0] de la tarjeta", " tarjeta · titulo "+ttNeutro, "plain "+ttNeutro, "rojo "+ttNeutro,
				"doctor "+ttNeutro, "criterio "+ttNeutro, "desc "+ttNeutro, "historia "+ttNeutro, "evento "+ttNeutro, "rama "+ttNeutro)
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
			m.Tecla("4", ttAhora)
			m.LeerArchivo(tablero.Archivo{Ruta: "x.go", Linea: 2, Desde: 1, Lineas: []string{"uno " + ttHostil, "dos"}})
			ver(m, "[4] y [0] de un hallazgo", "hallazgo "+ttNeutro, "uno "+ttNeutro)
			m.Tecla("1", ttAhora)
			ver(m, "[0] del proyecto", "problema "+ttNeutro, "accion "+ttNeutro)
		}
	}
}

// ---------------------------------------------------------------- las rutas fuera del proyecto

// ttConHallazgos es el modelo con el foco en [4] y esos hallazgos.
func ttConHallazgos(cfg tablero.Config, hs []finding.Item) *tablero.Modelo {
	m := ttModelo(cfg, ttTableroPrincipal())
	m.LeerHallazgos(tablero.Hallazgos{Items: hs})
	m.Tecla("4", ttAhora)
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
// con un ".." que no sale abre el editor. En [4] y en su [0].
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
				if enCero {
					m.Tecla(tablero.TeclaEnter, ttAhora)
				}
				ahora = ahora.Add(time.Second)
				que := fmt.Sprintf("e sobre %q (en [0]: %v, color=%v)", cs.file, enCero, color)
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
				if enCero {
					m.Tecla(tablero.TeclaEnter, ttAhora)
				}
				ords := m.Tecla("e", ttAhora)
				if local == `sub\..\x.go` {
					if len(ords) != 1 {
						t.Fatalf("CA-559: %q es local: e lo abre: %#v", local, ords)
					}
					continue
				}
				ttAbreLocal(t, "CA-559", "e sobre "+local, ords, local, 0)
			}
		}
	}
}

// CA-557, CA-522 (hallazgos f28d31 y e7f022 de la v1): e en [0] de una
// tarjeta con un spec cuya ruta sale del proyecto (relativa con "..", absoluta,
// con unidad, UNC, raiz "\" o "..\") no ordena nada y la linea de mensajes
// dice en rojo "el archivo <ruta> esta fuera del proyecto"; uno local con un
// ".." que no sale abre el editor en la linea 1.
func TestCA557_CA522_SpecFueraDelProyecto(t *testing.T) {
	abrir := func(cfg tablero.Config, ruta string) *tablero.Modelo {
		c := ttInfraAPI(false)
		m := ttModelo(cfg, ttTablero(c))
		m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
			Spec: boardcmd.SpecDetail{Path: ruta, Exists: true, Markdown: "# Spec\n\nuna linea\n"}}})
		m.Tecla(tablero.TeclaEnter, ttAhora)
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
