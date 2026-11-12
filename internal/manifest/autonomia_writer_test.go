// Tests propios del writer (autonomia-entre-firmas): la seccion autonomia
// de hoom.yaml, leida por LoadRoles.
package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func awLoad(t *testing.T, yml string) (*Manifest, error) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	return LoadRoles(root)
}

func TestWriterAutonomiaValores(t *testing.T) {
	m, err := awLoad(t, "autonomia:\n  reintentos_ante_rojo: 1\n  rondas_de_hallazgos: 2\n  ventana_max: 80\nfindings:\n  block_on: high\n")
	if err != nil {
		t.Fatal(err)
	}
	a := m.AutonomiaOrDefault()
	if a.ReintentosAnteRojo != 1 || a.RondasDeHallazgos != 2 || a.VentanaMax == nil || *a.VentanaMax != 80 {
		t.Fatalf("%+v", a)
	}
	m, err = awLoad(t, "project: x\n")
	if err != nil || m.AutonomiaOrDefault() != (AutonomiaPolicy{}) {
		t.Fatalf("sin seccion: %+v %v", m, err)
	}
}

func TestWriterAutonomiaErrores(t *testing.T) {
	for yml, want := range map[string]string{
		"autonomia:\n  otra: 1\n":                  `autonomia: clave desconocida "otra" (validas: reintentos_ante_rojo, rondas_de_hallazgos, ventana_max)`,
		"autonomia:\n  reintentos_ante_rojo: 2\n":  "autonomia: reintentos_ante_rojo tiene que ser 0 o 1",
		"autonomia:\n  rondas_de_hallazgos: 6\n":   "autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5",
		"autonomia:\n  rondas_de_hallazgos: 1.5\n": "autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5",
		"autonomia:\n  ventana_max: 0\n":           "autonomia: ventana_max tiene que ser un entero de 1 a 100",
		"autonomia:\n  ventana_max: 101\n":         "autonomia: ventana_max tiene que ser un entero de 1 a 100",
		"autonomia:\n  rondas_de_hallazgos: 1\n":   "autonomia: rondas_de_hallazgos necesita findings.block_on",
	} {
		_, err := awLoad(t, yml)
		if err == nil || !strings.HasPrefix(err.Error(), "hoom.yaml invalido: ") || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: quiero %q, fue %v", yml, want, err)
		}
	}
}
