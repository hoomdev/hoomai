// Regresion del hallazgo 6dcf5b (ronda 3 de proporcion-por-riesgo), sobre el
// binario: docs/a/precios.md y docs/b/precios.md, con el mismo contenido, se
// aprueban cada uno con su registro: tras `hoom spec approve` de los dos,
// `hoom spec status` de cada uno dice APROBADO y sale con 0.
//
// Compila el binario: se omite con -short.
package main

import (
	"strings"
	"testing"
)

func TestHallazgo_6dcf5b_DosSpecsConElMismoNombreSeApruebanCadaUno(t *testing.T) {
	const h = "hallazgo 6dcf5b"
	veeSaltar(t)
	bin := qcHoom(t)
	root := pprProyecto(t, "", "-", 100, 2, 1)
	texto := pprSpec(t, 120, 2)
	specs := []string{"docs/a/precios.md", "docs/b/precios.md"}
	for _, s := range specs {
		veeEscribir(t, root, s, texto, 0o644)
		if ap := qcCorrer(t, bin, root, qcSinCorrida, "spec", "approve", s); ap.exit != 0 || !strings.Contains(ap.stdout, "hoom spec: APROBADO "+s) {
			t.Fatalf("%s: `hoom spec approve %s` lo aprueba con su registro: exit %d\n%s", h, s, ap.exit, ap.todo())
		}
	}
	for _, s := range specs {
		if st := qcCorrer(t, bin, root, qcSinCorrida, "spec", "status", s); st.exit != 0 || !strings.Contains(st.stdout, "hoom spec: APROBADO (vigente)") {
			t.Fatalf("%s: `hoom spec status %s` dice APROBADO (exit 0): exit %d\n%s", h, s, st.exit, st.todo())
		}
	}
	if n := pprRegistros(root); n != 2 {
		t.Fatalf("%s: dos specs, dos registros: %d", h, n)
	}
}
