// Regresion del hallazgo 7bd189 (ronda 2 de proporcion-por-riesgo), sobre el
// binario: un registro guardado como `./.hoom/specs/precios.md` (lo que
// escribia `hoom spec approve ./.hoom/specs/precios.md` antes) sigue
// aprobando ese contenido para `hoom spec status`, con las dos grafias desde
// la raiz y desde un subdirectorio: APROBADO y exit 0.
//
// Compila el binario: se omite con -short.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHallazgo_7bd189_SpecStatusVeElRegistroConPuntoBarra(t *testing.T) {
	const h = "hallazgo 7bd189"
	veeSaltar(t)
	bin := qcHoom(t)
	root := pprProyecto(t, "", "-", 100, 2, 1)
	raw, err := os.ReadFile(filepath.Join(root, pprRuta))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	rec := fmt.Sprintf(`{"spec": "./%s", "sha256": %q, "approved_by": "hoom test <test@hoom.dev>", "approved_at": "2026-11-20T10:00:00Z"}`, pprRuta, sha)
	veeEscribir(t, root, ".hoom/approvals/precios_"+sha[:8]+".json", rec, 0o644)
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ dir, ruta string }{{root, "./" + pprRuta}, {root, pprRuta}, {sub, "../" + pprRuta}} {
		st := qcCorrer(t, bin, c.dir, qcSinCorrida, "spec", "status", c.ruta)
		if st.exit != 0 || !strings.Contains(st.stdout, "hoom spec: APROBADO (vigente)") {
			t.Fatalf("%s: `hoom spec status %s` desde %s ve el registro ./%s (exit 0): exit %d\n%s", h, c.ruta, c.dir, pprRuta, st.exit, st.todo())
		}
	}
}
