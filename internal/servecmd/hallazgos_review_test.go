// Tests de regresion de la review cruzada (Codex, 2026-09-06) sobre Spec D + Spec E.
package servecmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Hallazgo 20260907T001833_0190fb: runRows copia Meta.Status de los sidecars
// ajenos al JSON de /api/runs sin validar, y en la UI badge() interpola ese
// valor como clase SIN esc() dentro de innerHTML (runBadge(r.status)): un meta
// plantado en .hoom/runs con status `x"><img src=x onerror=...>` ejecuta JS en
// el Studio y desde ahi lee el token de acciones. Del lado del servidor, un
// status fuera de {running, done, error, canceled} no puede llegar al JSON:
// o se normaliza a uno del conjunto, o el sidecar se omite.
func TestHallazgo_0190fb_ElStatusDeUnSidecarAjenoNoViajaSinValidar(t *testing.T) {
	dir := newGitProject(t)
	srv := newServer(t, dir)
	const id = "20260906T230000_0f0f0f"
	const veneno = `x"><img src=x onerror=1>`
	now := time.Now().UTC()
	runAjeno(t, dir, id,
		runcmd.Meta{ID: id, Provider: "codex", Role: "reviewer", Dir: dir, CreatedAt: now, Status: veneno, ExitCode: -1},
		[]providers.Event{{TS: now, Kind: "start", Detail: "run"}})

	validos := map[string]bool{
		runcmd.StatusRunning: true, runcmd.StatusDone: true, runcmd.StatusError: true, runcmd.StatusCanceled: true,
	}
	var runs []RunRow
	get(t, srv, "/api/runs", &runs)
	for _, r := range runs {
		if r.ID == id && !validos[r.Status] {
			t.Fatalf("0190fb: /api/runs sirve el status del sidecar ajeno tal cual, fuera de {running, done, error, canceled}: %q", r.Status)
		}
	}

	// el detalle del run, mismo criterio (si el sidecar se omite, 404 esta bien)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/runs/"+id, nil))
	switch rec.Code {
	case http.StatusOK:
		var detalle struct {
			Run RunRow `json:"run"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &detalle); err != nil {
			t.Fatal(err)
		}
		if !validos[detalle.Run.Status] {
			t.Fatalf("0190fb: /api/runs/{id} tambien sirve el status sin validar: %q", detalle.Run.Status)
		}
	case http.StatusNotFound:
	default:
		t.Fatalf("0190fb: /api/runs/{id} respondio %d: %s", rec.Code, rec.Body)
	}
}
