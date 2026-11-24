// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// `hoom piloto` (CA-806, que re-expresa CA-647): justo antes de su linea
// `piloto automatico: pide al ...`, que no cambia, el piloto imprime
// `piloto automatico: valores: <lo mismo>`, con los valores del riesgo
// declarado de la tarjeta y el nivel traducido; el sobre no la repite.
// Escritos desde el spec, las aclaraciones de la ronda 0 y las firmas
// publicas, sin leer la implementacion.
package pilotocmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// CA-806: hoom piloto imprime `piloto automatico: valores: ` justo antes de
// `pide al`, una vez por lanzamiento, con el modelo del nivel del riesgo
// alto de la tarjeta; la linea de pedir es la de hoy (CA-647) y el log del
// sobre no trae otra linea valores: sin el prefijo del piloto.
func TestCA806_PilotoImprimeValoresAntesDePideAl(t *testing.T) {
	const ca = "CA-806"
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "agents:\n  test-writer:\n    model: medio\n    alto: { model: fuerte }\n")
	const slug = "alta"
	apTestWriter(t, root, slug, apConAuto+"riesgo: alto\n")
	s := apRun(t, ca, root, slug)
	ls := apSoloLineasDelPiloto(t, ca, s)
	ip, nv, np := -1, 0, 0
	for i, l := range ls {
		if strings.HasPrefix(l, apPrefijo+"valores: ") {
			nv++
		}
		if strings.HasPrefix(l, apPrefijo+"pide al ") {
			np++
			if ip < 0 {
				ip = i
			}
		}
	}
	if ip < 1 || nv != np || np != 1 {
		t.Fatalf("%s: una linea valores: por lanzamiento (%d valores, %d pide):\n%s", ca, nv, np, s)
	}
	v := ls[ip-1]
	if !strings.HasPrefix(v, apPrefijo+"valores: provider claude (") {
		t.Fatalf("%s: justo antes de pide al va la linea valores: %q", ca, v)
	}
	if !strings.Contains(v, " · modelo opus (nivel fuerte, hoom.yaml, riesgo alto) · ") {
		t.Fatalf("%s: con la tarjeta alto, modelo opus (nivel fuerte, hoom.yaml, riesgo alto): %q", ca, v)
	}
	for _, n := range []string{"modelo fuerte", "modelo medio", "modelo barato"} {
		if strings.Contains(v, n) {
			t.Fatalf("%s: ningun nivel sin traducir en la pantalla: %q", ca, v)
		}
	}
	m := apPide.FindStringSubmatch(ls[ip])
	if m == nil {
		t.Fatalf("CA-647: la linea de pedir no cambia: %q", ls[ip])
	}
	rec := apRegistro(t, ca, root, m[4])
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, rec.ID+".log"))
	if err != nil {
		t.Fatalf("%s: el log del sobre: %v", ca, err)
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "valores: ") {
			t.Fatalf("%s: el sobre no repite la linea valores: (ValoresImpresos):\n%s", ca, raw)
		}
	}
}

// CA-806: una tarjeta normal con el mismo hoom.yaml lanza con medio (sonnet)
// y origen hoom.yaml; una parada sin lanzamiento sigue siendo una linea, sin
// valores: (CA-647).
func TestCA806_PilotoNormalYParadaSinValores(t *testing.T) {
	const ca = "CA-806"
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "agents:\n  test-writer:\n    model: medio\n    alto: { model: fuerte }\n")
	const slug = "normal"
	apTestWriter(t, root, slug, apConAuto)
	s := apRun(t, ca, root, slug)
	ls := apSoloLineasDelPiloto(t, ca, s)
	found := false
	for i, l := range ls {
		if strings.HasPrefix(l, apPrefijo+"pide al ") {
			if i == 0 || !strings.Contains(ls[i-1], " · modelo sonnet (nivel medio, hoom.yaml) · ") {
				t.Fatalf("%s: antes de pide al, valores: con sonnet (nivel medio, hoom.yaml): %q", ca, ls)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("%s: el piloto lanza al test-writer:\n%s", ca, s)
	}
	// sin auto: se detiene sin lanzar, en una linea y sin valores:
	f2 := apPATH(t)
	f2.cli(t, "claude")
	root2 := apProyecto(t, "")
	apTestWriter(t, root2, slug, apSinAuto)
	s = apRun(t, "CA-647", root2, slug)
	ls = apSoloLineasDelPiloto(t, "CA-647", s)
	if len(ls) != 1 || !strings.HasPrefix(ls[0], apDetiene) || strings.Contains(s.out, "valores: ") {
		t.Fatalf("CA-647: una parada sin lanzamiento es una linea, sin valores:\n%s", s)
	}
}
