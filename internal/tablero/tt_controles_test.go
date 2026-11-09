// Los controles en los textos (CA-534): ningun texto de la tarjeta o de los
// agentes llega a la terminal como una secuencia; el tab es un espacio y
// cada otro control (C0, DEL, C1, bidi) un "?". Regresion de la ronda 1
// (review completa 20261108T231004_195bc7 de tablero-en-la-terminal),
// escrita desde el contrato de hoy (doc.go y go doc del paquete), sin leer
// la implementacion.
package tablero_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

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
