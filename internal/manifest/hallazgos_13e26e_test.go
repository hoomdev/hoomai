// Regresion de los hallazgos 20261125T161655_13e26e (high) y
// 20261125T162224_dc272d (medium): ValidarGates dejaba de mirar pasados 8
// niveles de merges (<<) y devolvia nil. A partir de 9, Load cargaba sin
// error un aplica_en con un patron invalido, una clave desconocida y un
// aplica_en vacio (CA-809). Ahora recorre los merges a cualquier
// profundidad, ve cada mapa una sola vez y corta los ciclos.
package manifest_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
)

const h13eCabeza = "schema: hoom/v1\nproject: demo\nbase_branch: main\n"

// h13eEnLinea es gates.static con su cmd y un merge en linea de n niveles
// cuya hoja es hoja.
func h13eEnLinea(n int, hoja string) string {
	v := "{" + hoja + "}"
	for i := 1; i < n; i++ {
		v = "{<<: " + v + "}"
	}
	return h13eCabeza + "gates:\n  static:\n    cmd: \"true\"\n    <<: " + v + "\n"
}

// h13eAnclas es una cadena de anclas a1 <- a2 <- ... <- an fuera de gates:
// la hoja esta en a1, y gates.static hace <<: *an, n merges por encima.
func h13eAnclas(n int, hoja string) string {
	var b strings.Builder
	b.WriteString(h13eCabeza)
	b.WriteString("x-anclas:\n")
	fmt.Fprintf(&b, "  a1: &a1 {%s}\n", hoja)
	for i := 2; i <= n; i++ {
		fmt.Fprintf(&b, "  a%d: &a%d {<<: *a%d}\n", i, i, i-1)
	}
	fmt.Fprintf(&b, "gates:\n  static:\n    cmd: \"true\"\n    <<: *a%d\n", n)
	return b.String()
}

func TestHallazgo13e26e_CA809_LosErroresAlFondoDeMuchosMerges(t *testing.T) {
	const ca = "CA-809"
	errores := []struct {
		hoja, want string
	}{
		{`aplica_en: ["app/["]`, gprPrefijo + fmt.Sprintf(manifest.ErrAplicaEnPatron, "static", "app/[")},
		{`aplicaen: [app/**]`, gprPrefijo + fmt.Sprintf(manifest.ErrGateClaveDesconocida, "static", "aplicaen")},
		{`aplica_en: []`, gprPrefijo + fmt.Sprintf(manifest.ErrAplicaEnLista, "static")},
	}
	armados := []struct {
		nombre string
		armar  func(int, string) string
	}{
		{"en linea", h13eEnLinea},
		{"anclas", h13eAnclas},
	}
	for _, e := range errores {
		for _, a := range armados {
			for _, n := range []int{1, 8, 9, 10, 20, 64, 300} {
				dir := gprDir(t, a.armar(n, e.hoja))
				for _, c := range gprCargadores {
					_, err := c.cargar(dir, nil)
					if err == nil || err.Error() != e.want {
						t.Fatalf("%s (%s, %s, n=%d, %s): %q, no %v", ca, c.nombre, a.nombre, n, e.hoja, e.want, err)
					}
				}
			}
		}
	}
}

// El control: la misma cadena con una hoja valida carga, y static hereda el
// aplica_en del fondo.
func TestHallazgo13e26e_CA809_UnaCadenaValidaCarga(t *testing.T) {
	const ca = "CA-809"
	for _, n := range []int{9, 64, 300} {
		for _, doc := range []string{h13eEnLinea(n, "aplica_en: [app/**]"), h13eAnclas(n, "aplica_en: [app/**]")} {
			m, err := manifest.Load(gprDir(t, doc), nil)
			if err != nil {
				t.Fatalf("%s (n=%d): una cadena valida carga: %v", ca, n, err)
			}
			if got := m.Gates["static"].AplicaEn; len(got) != 1 || got[0] != "app/**" {
				t.Fatalf("%s (n=%d): static hereda aplica_en [app/**]: %q", ca, n, got)
			}
		}
	}
}

// Un ancla que se nombra a si misma no cuelga la validacion, y el hoom.yaml
// no carga.
func TestHallazgo13e26e_CA809_UnCicloNoCuelgaYNoCarga(t *testing.T) {
	const ca = "CA-809"
	doc := h13eCabeza + "x:\n  a: &a {cmd: \"true\", <<: *a}\ngates:\n  static:\n    <<: *a\n"
	if _, err := manifest.Load(gprDir(t, doc), nil); err == nil {
		t.Fatalf("%s: un ancla que se contiene a si misma no carga", ca)
	}
	doc = h13eCabeza + "x:\n  a: &a {aplicaen: [app/**], <<: *a}\ngates:\n  static:\n    <<: *a\n"
	want := gprPrefijo + fmt.Sprintf(manifest.ErrGateClaveDesconocida, "static", "aplicaen")
	if _, err := manifest.Load(gprDir(t, doc), nil); err == nil || err.Error() != want {
		t.Fatalf("%s: con un ciclo, la clave desconocida igual sale: %q, no %v", ca, want, err)
	}
}

// Cada mapa se ve una sola vez: un abanico de merges (f alias por nivel, 8
// niveles: f^8 caminos) se valida en lo que dura leer el archivo.
func TestHallazgo13e26e_CA809_UnAbanicoDeMergesEsLineal(t *testing.T) {
	const ca = "CA-809"
	var b strings.Builder
	b.WriteString("x:\n  a0: &a0 {cmd: \"true\"}\n")
	for i := 1; i <= 8; i++ {
		ref := strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*a%d, ", i-1), 12), ", ")
		fmt.Fprintf(&b, "  a%d: &a%d {<<: [%s]}\n", i, i, ref)
	}
	b.WriteString("gates:\n  static:\n    <<: *a8\n")
	t0 := time.Now()
	err := manifest.ValidarGates([]byte(b.String()))
	if d := time.Since(t0); d > time.Second {
		t.Fatalf("%s: 12^8 caminos de merges tardan %v: cada mapa se ve una vez", ca, d)
	}
	if err != nil {
		t.Fatalf("%s: un abanico valido no da error: %v", ca, err)
	}
}
