// Run y el editor que no abre (CA-522): la barra lo dice, la terminal se
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

// CA-522 (hallazgo f28d31): un Editor cuyo Abrir falla: la barra dice "no se
// pudo abrir el editor: <error>", la terminal se toma de nuevo y la TUI
// sigue (otra tecla redibuja, q sale con 0, nada en Err).
func TestCA522_RunEditorQueNoAbreSigueLaTUI(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	w := ttNuevoMundo(t, ttTablero(c))
	w.ed.err = errors.New("vi: no existe")
	w.datos.detalleDe = func(slug string) tablero.Detalle {
		return tablero.Detalle{Slug: slug, Detalle: boardcmd.Detail{Card: c,
			Spec: boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "# Spec\n\nuna linea\n"}}}
	}
	fin := w.correr(nil)
	ttEsperar(t, "CA-522", "la tarjeta", w, func() bool { return w.marcoCon("tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-522", "la tarjeta abierta", w, func() bool { return w.marcoCon("[1 Resumen]") })
	w.term.mandar("2")
	ttEsperar(t, "CA-522", "la pestana Spec", w, func() bool { return w.marcoCon("una linea") })
	desde := len(w.r.copia())
	w.term.mandar("o")
	ttEsperar(t, "CA-522", "el editor", w, func() bool {
		return w.r.indice(desde, func(e ttSuceso) bool { return e.que == "abrir" }) >= 0
	})
	abrir := w.r.indice(desde, func(e ttSuceso) bool { return e.que == "abrir" })
	ttEsperar(t, "CA-522", "el error del editor en la barra", w, func() bool {
		return w.marcoCon("no se pudo abrir el editor: vi: no existe")
	})
	ttEsperar(t, "CA-522", "la terminal tomada de nuevo", w, func() bool {
		cr := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "crudo" })
		en := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "escribe" && strings.Contains(e.dato, tablero.SecEntrar) })
		return cr > abrir && en > cr
	})
	w.term.mandar("1")
	ttEsperar(t, "CA-522", "la TUI sigue: 1 vuelve a Resumen", w, func() bool { return w.marcoCon("[1 Resumen]") })
	w.term.mandar("q")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-522: tras un editor que no abre, q sale con 0, salio con %d", code)
	}
	if got := w.r.texto("err"); got != "" {
		t.Fatalf("CA-522: el editor que no abre va a la barra, no a Err: %q", got)
	}
}
