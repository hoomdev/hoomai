// Regresion del hallazgo c9ffe4 (ronda 1 de proporcion-por-riesgo), sobre el
// binario: `hoom spec approve` y `hoom spec status` leen la ruta del spec
// igual desde cualquier directorio. Tras aprobar desde un subdirectorio,
// status desde ese subdirectorio y desde la raiz (con la ruta limpia o con
// ./) encuentran el mismo registro: APROBADO y exit 0.
//
// Compila el binario: se omite con -short.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHallazgo_c9ffe4_StatusDesdeUnSubdirectorioVeLaAprobacion(t *testing.T) {
	const h = "hallazgo c9ffe4"
	veeSaltar(t)
	bin := qcHoom(t)
	root := pprProyecto(t, "", "-", 100, 2, 1)
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	desdeSub := "../" + pprRuta
	if s := qcCorrer(t, bin, sub, qcSinCorrida, "spec", "approve", desdeSub); s.exit != 0 {
		t.Fatalf("%s: fixture: el spec se aprueba desde sub: exit %d\n%s", h, s.exit, s.todo())
	}
	for _, c := range []struct{ dir, ruta string }{{sub, desdeSub}, {root, pprRuta}, {root, "./" + pprRuta}} {
		st := qcCorrer(t, bin, c.dir, qcSinCorrida, "spec", "status", c.ruta)
		if st.exit != 0 || !strings.Contains(st.stdout, "hoom spec: APROBADO (vigente)") {
			t.Fatalf("%s: `hoom spec status %s` desde %s ve la aprobacion (exit 0): exit %d\n%s", h, c.ruta, c.dir, st.exit, st.todo())
		}
	}
	if n := pprRegistros(root); n != 1 {
		t.Fatalf("%s: una aprobacion, un registro: %d", h, n)
	}
}
