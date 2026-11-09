// Run y el editor que no abre (CA-522, CA-557): la linea de mensajes lo dice, la terminal se
// toma de nuevo y la TUI sigue. Regresion de la ronda 1 sobre Run (review
// completa 20261108T231004_195bc7). Con la terminal, los datos, el lanzador
// y el editor falsos de tt_run_test.go; cada espera con su tiempo maximo.
// Los enlaces que salen del proyecto: tt_run_enlaces_unix_test.go.
package tablero_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-522, CA-557 (hallazgo f28d31): un Editor cuyo Abrir falla: la linea de
// mensajes dice "no se pudo abrir el editor: <error>", la terminal se toma
// de nuevo y la TUI sigue (otra tecla redibuja, q sale con 0, nada en Err).
// Re-expresado por CA-557: el editor se pide con e en [0] de la tarjeta (en
// la v1, o en la pestana Spec), y "la TUI sigue" se ve con 2, que le
// devuelve el foco a [2] (en la v1, 1 volvia a Resumen).
func TestCA522_CA557_RunEditorQueNoAbreSigueLaTUI(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	w := ttNuevoMundo(t, ttTablero(c))
	w.ed.err = errors.New("vi: no existe")
	w.datos.detalleDe = func(slug string) tablero.Detalle {
		return tablero.Detalle{Slug: slug, Detalle: boardcmd.Detail{Card: c,
			Spec: boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "# Spec\n\nuna linea\n"}}}
	}
	fin := w.correr(nil)
	ttEsperar(t, "CA-522", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-553", "el foco en [0] con el spec de la tarjeta", w, func() bool {
		return w.marcoCon("#[0] tarjeta · Tarjeta de prueba") && w.marcoCon("Spec · 0 criterios")
	})
	desde := len(w.r.copia())
	w.term.mandar("e")
	ttEsperar(t, "CA-522", "el editor", w, func() bool {
		return w.r.indice(desde, func(e ttSuceso) bool { return e.que == "abrir" }) >= 0
	})
	abrir := w.r.indice(desde, func(e ttSuceso) bool { return e.que == "abrir" })
	ttEsperar(t, "CA-522", "el error del editor en la linea de mensajes", w, func() bool {
		return w.marcoCon("no se pudo abrir el editor: vi: no existe")
	})
	ttEsperar(t, "CA-522", "la terminal tomada de nuevo", w, func() bool {
		cr := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "crudo" })
		en := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "escribe" && strings.Contains(e.dato, tablero.SecEntrar) })
		return cr > abrir && en > cr
	})
	w.term.mandar("2")
	ttEsperar(t, "CA-551", "la TUI sigue: 2 le da el foco a [2]", w, func() bool { return w.marcoCon("#[2] Tarjetas") })
	w.term.mandar("q")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-522: tras un editor que no abre, q sale con 0, salio con %d", code)
	}
	if got := w.r.texto("err"); got != "" {
		t.Fatalf("CA-522: el editor que no abre va a la linea de mensajes, no a Err: %q", got)
	}
}
