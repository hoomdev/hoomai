// Regresiones de la ronda 1 sobre Run (review completa
// 20261108T231004_195bc7): la terminal que no acepta SecEntrar (CA-533), el
// editor que no abre (CA-522) y las teclas que una lectura corta por la
// mitad (CA-520, CA-527). Con la terminal, los datos, el lanzador y el
// editor falsos de tt_run_test.go; cada espera con su tiempo maximo.
package tablero_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttTermMuda es la terminal falsa cuyo Write falla siempre: cada intento se
// anota en el registro como "intento" (lo que se quiso escribir).
type ttTermMuda struct {
	*ttTerm
	err error
}

func (f ttTermMuda) Write(p []byte) (int, error) {
	f.r.anotar("intento", string(p))
	return 0, f.err
}

// CA-533 (hallazgos af2e4b, 96ce0b): una terminal que se pone en crudo pero
// no acepta la primera escritura (SecEntrar): Run la devuelve (intenta
// SecSalir y llama a restaurar), escribe "hoom: <error>" en Err despues de
// devolverla y sale con 1, sin quedarse esperando teclas.
func TestCA533_RunFallaAlEscribirSecEntrar(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	falla := errors.New("la terminal no acepta escrituras")
	w.e.Terminal = ttTermMuda{ttTerm: w.term, err: falla}
	code, pan := w.correr(nil)(t)
	if code != 1 || pan != nil {
		t.Fatalf("CA-533: si escribir SecEntrar falla, Run sale con 1: %d (panic %v)", code, pan)
	}
	if got := strings.TrimRight(w.r.texto("err"), "\n"); got != "hoom: "+falla.Error() {
		t.Fatalf("CA-533: Err dice 'hoom: <error>': %q", got)
	}
	es := w.r.copia()
	crudo := w.r.indice(0, func(e ttSuceso) bool { return e.que == "crudo" })
	primero := w.r.indice(0, func(e ttSuceso) bool { return e.que == "intento" })
	if crudo < 0 || primero < 0 || primero < crudo {
		t.Fatalf("CA-533: primero Crudo, despues SecEntrar: %v", es)
	}
	if !strings.HasPrefix(es[primero].dato, tablero.SecEntrar) {
		t.Fatalf("CA-533: lo primero que se escribe es SecEntrar: %q", es[primero].dato)
	}
	salir := w.r.indice(primero+1, func(e ttSuceso) bool { return e.que == "intento" && strings.Contains(e.dato, tablero.SecSalir) })
	if salir < 0 {
		t.Fatalf("CA-533: al devolver la terminal se intenta SecSalir: %v", es)
	}
	rest := w.r.indice(0, func(e ttSuceso) bool { return e.que == "restaurar" })
	if rest < 0 || rest < salir {
		t.Fatalf("CA-533: la terminal se devuelve: SecSalir y despues restaurar: %v", es)
	}
	if errI := w.r.indice(0, func(e ttSuceso) bool { return e.que == "err" }); errI < rest {
		t.Fatalf("CA-533: el error se escribe despues de devolver la terminal: %v", es)
	}
	if got := w.r.texto("out"); got != "" {
		t.Fatalf("CA-533: nada en Out: %q", got)
	}
}

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

// CA-520 (hallazgo ca8887): una lectura que termina dentro de un caracter
// UTF-8 espera a la siguiente: "/" con el primer byte de "ñ" y despues el
// segundo, en dos lecturas seguidas, es la busqueda de "ñ" (que filtra como
// n: queda la de la piñata, se va zeta).
func TestCA520_RunRunaPartidaEnDosLecturas(t *testing.T) {
	seg := ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado")
	w := ttNuevoMundo(t, ttTablero(ttCard("pinata", "La piñata", boardcmd.ColWriter, "con ene", seg),
		ttCard("zeta", "Zeta", boardcmd.ColWriter, "sin ene", seg)))
	fin := w.correr(nil)
	ttEsperar(t, "CA-520", "la lista", w, func() bool { return w.marcoCon("zeta") && w.marcoCon("pinata") })
	w.term.mandar("/\xc3")
	w.term.mandar("\xb1")
	ttEsperar(t, "CA-520", "la busqueda de ñ", w, func() bool {
		return w.marcoCon("/ñ_") && w.marcoCon("pinata") && !w.marcoCon("zeta")
	})
	w.term.mandar("\x03")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-520: ctrl-c en la busqueda sale con 0, salio con %d", code)
	}
}

// CA-527 (hallazgo ca8887): lo mismo en el pedido del formulario: los dos
// bytes de "ñ" en dos lecturas seguidas son una "ñ" en el pedido que se
// lanza.
func TestCA527_RunRunaPartidaEnElPedido(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "hola"
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(a)))
	fin := w.correr(nil)
	ttEsperar(t, "CA-527", "la tarjeta", w, func() bool { return w.marcoCon("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, "CA-527", "el formulario", w, func() bool { return w.marcoCon("-> hoom pedir writer tarjeta") })
	w.term.mandar("\xc3")
	w.term.mandar("\xb1")
	ttEsperar(t, "CA-527", "la ñ en el pedido", w, func() bool { return w.marcoCon("holañ_") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-527", "la orden", w, func() bool { return len(w.lanz.lanzadas()) == 1 })
	ttMismosArgs(t, "CA-527", "el pedido con la ñ", w.lanz.lanzadas()[0].Args, []string{"pedir", "writer", "tarjeta", "-m", "holañ"})
	w.term.mandar("q")
	fin(t)
}

// CA-520 (hallazgo ca8887): una lectura que termina dentro de una secuencia
// de escape tambien espera a la siguiente: ESC y despues "[B", en dos
// lecturas seguidas, es la flecha abajo (no esc, "[" y "B").
func TestCA520_RunFlechaPartidaEnDosLecturas(t *testing.T) {
	seg := ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado")
	w := ttNuevoMundo(t, ttTablero(ttCard("uno", "Uno", boardcmd.ColWriter, "la primera", seg),
		ttCard("dos", "Dos", boardcmd.ColWriter, "la segunda", seg)))
	fin := w.correr(nil)
	ttEsperar(t, "CA-520", "la lista con uno elegida", w, func() bool { return w.marcoCon(" > uno ") })
	w.term.mandar("\x1b")
	w.term.mandar("[B")
	ttEsperar(t, "CA-520", "la flecha abajo elige dos", w, func() bool { return w.marcoCon(" > dos ") })
	w.term.mandar("q")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-520: q sale con 0, salio con %d", code)
	}
}
