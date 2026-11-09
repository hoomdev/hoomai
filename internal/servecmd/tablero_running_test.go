// Running gana timeout_s y last_event_at (spec tablero-en-la-terminal,
// CA-519): /api/board y /api/board/{slug} los emiten. El proyecto
// (con-sobre, suelto, sin-run) y lo que se exige del JSON son los de
// runningtest, los mismos de hoom board --json y hoom item show --json
// (hallazgo 20261108T230948_50d437).
package servecmd_test

import (
	"net/http/httptest"
	"testing"

	"github.com/hoomdev/hoomai/internal/runningtest"
	"github.com/hoomdev/hoomai/internal/servecmd"
)

// CA-519: /api/board emite timeout_s y last_event_at en cada running, y
// /api/board/{slug} tambien.
func TestCA519_ApiBoardTraeLasClaves(t *testing.T) {
	root, now := runningtest.Proyecto(t)
	s, err := servecmd.New(root)
	if err != nil {
		t.Fatalf("CA-519: servecmd.New: %v", err)
	}
	h := s.Handler()
	for _, ruta := range []string{"/api/board", "/api/board/con-sobre"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "http://127.0.0.1"+ruta, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("CA-519: GET %s da 200, dio %d: %s", ruta, rec.Code, rec.Body.String())
		}
		slugs := []string{"con-sobre", "suelto", "sin-run"}
		if ruta != "/api/board" {
			slugs = []string{"con-sobre"}
		}
		runningtest.RevisarJSON(t, "CA-519", "GET "+ruta, rec.Body.Bytes(), now, slugs...)
	}
}
