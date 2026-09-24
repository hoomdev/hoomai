// Tests de regresion de la review de la cabina (C2, C3 y C4) sobre el Studio:
//
//   - 20260924T051307_fa8070 (CA-317, CA-365): GET /api/board/{slug} y
//     GET /api/board/{slug}/timeline responden 404 solo cuando el item no
//     existe o es invalido por contenido (el mensaje de CardFor, sin cambios).
//     Un item que existe pero no se puede LEER (error de E/S) es un error
//     interno: 500 JSON con el error, no un 404 que lo esconde.
package servecmd

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// hcItemIlegible arma el proyecto de C2 con un item VALIDO "ilegible" (y
// "sana", legible) y le saca todos los permisos al archivo del item: existe,
// pero leerlo es un error de E/S. Saltea el test si corre como root (root lee
// igual un archivo 000).
func hcItemIlegible(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("como root un archivo con modo 000 se lee igual: no hay error de E/S que probar")
	}
	root := tbProyecto(t)
	tbItem(t, root, "ilegible", "")
	tbItem(t, root, "sana", "")
	p := filepath.Join(root, ".hoom", "items", "ilegible.yaml")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := os.ReadFile(p); err == nil {
		t.Skip("este sistema de archivos deja leer un archivo 000: no hay error de E/S que probar")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture: el item ilegible existe: %v", err)
	}
	return root
}

// hcNoExisteSigueIgual: el item que no existe sigue siendo 404 JSON con el
// mensaje de CardFor (CA-317 / CA-365 sin cambios).
func hcNoExisteSigueIgual(t *testing.T, ca string, s *Server, root, sufijo string) {
	t.Helper()
	msg := tbErrorJSON(t, ca+" (no-existe)", tbGET(t, s, "/api/board/no-existe"+sufijo), http.StatusNotFound)
	_, err := boardcmd.CardFor(root, "main", tbBlockOn, "no-existe", time.Now().UTC())
	if err == nil || msg != err.Error() {
		t.Fatalf("%s: el 404 del item que no existe sigue trayendo el mensaje de CardFor:\napi: %q\ncli: %v", ca, msg, err)
	}
}

// Hallazgo 20260924T051307_fa8070. CA-317: GET /api/board/{slug} de un item
// que existe pero no se puede leer (modo 000) no es "no existe": es un error
// interno, 500 JSON con el error. Tambien con ?diff=1. El item que no existe
// sigue en 404 con el mensaje de CardFor, y el vecino legible sigue en 200.
func TestHallazgo_fa8070_DetalleDeItemIlegibleEs500(t *testing.T) {
	root := hcItemIlegible(t)
	s := newServer(t, root)

	for _, path := range []string{"/api/board/ilegible", "/api/board/ilegible?diff=1"} {
		rec := tbGET(t, s, path)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("fa8070 CA-317: GET %s de un item que EXISTE pero no se puede leer no es 404 (eso es solo para el que no existe): %s",
				path, rec.Body.String())
		}
		tbErrorJSON(t, "fa8070 CA-317 (GET "+path+")", rec, http.StatusInternalServerError)
	}

	hcNoExisteSigueIgual(t, "fa8070 CA-317", s, root, "")
	if rec := tbGET(t, s, "/api/board/sana"); rec.Code != http.StatusOK {
		t.Fatalf("fa8070 CA-317: el item legible de al lado sigue respondiendo 200: %d %s", rec.Code, rec.Body.String())
	}
}

// Hallazgo 20260924T051307_fa8070. CA-365: lo mismo para
// GET /api/board/{slug}/timeline: 404 solo para el item que no existe (o es
// invalido por contenido); el que existe y no se puede leer es 500 JSON.
func TestHallazgo_fa8070_TimelineDeItemIlegibleEs500(t *testing.T) {
	root := hcItemIlegible(t)
	s := newServer(t, root)

	rec := tbGET(t, s, "/api/board/ilegible/timeline")
	if rec.Code == http.StatusNotFound {
		t.Fatalf("fa8070 CA-365: GET /api/board/ilegible/timeline de un item que EXISTE pero no se puede leer no es 404: %s",
			rec.Body.String())
	}
	tbErrorJSON(t, "fa8070 CA-365 (GET /api/board/ilegible/timeline)", rec, http.StatusInternalServerError)

	hcNoExisteSigueIgual(t, "fa8070 CA-365", s, root, "/timeline")
	if rec := tbGET(t, s, "/api/board/sana/timeline"); rec.Code != http.StatusOK {
		t.Fatalf("fa8070 CA-365: la historia del item legible de al lado sigue respondiendo 200: %d %s", rec.Code, rec.Body.String())
	}
}
