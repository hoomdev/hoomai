// Run con una terminal que no acepta escrituras (CA-533): si escribir
// SecEntrar falla, la terminal se devuelve y Run sale con 1. Regresion de la
// ronda 1 sobre Run (review completa 20261108T231004_195bc7). Con la
// terminal, los datos, el lanzador y el editor falsos de tt_run_test.go;
// cada espera con su tiempo maximo.
package tablero_test

import (
	"errors"
	"strings"
	"testing"

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
