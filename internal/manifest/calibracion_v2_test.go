package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewCalibracionV2(t *testing.T) {
	cargar := func(extra string) (*Manifest, error) {
		dir := t.TempDir()
		raw := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" + extra
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		return LoadWithoutBaseCheck(dir, nil)
	}

	m, err := cargar("")
	if err != nil {
		t.Fatalf("sin review: %v", err)
	}
	if m.Review.IncluirTestsOrDefault() != false || m.Review.ConcurrenciaOrDefault() != 1 {
		t.Fatalf("defaults sin review: incluir_tests=%v concurrencia=%d", m.Review.IncluirTestsOrDefault(), m.Review.ConcurrenciaOrDefault())
	}

	m, err = cargar("review:\n  incluir_tests: true\n  concurrencia: 4\n")
	if err != nil {
		t.Fatalf("con incluir_tests y concurrencia validos: %v", err)
	}
	if !m.Review.IncluirTestsOrDefault() || m.Review.ConcurrenciaOrDefault() != 4 {
		t.Fatalf("valores cargados: incluir_tests=%v concurrencia=%d", m.Review.IncluirTestsOrDefault(), m.Review.ConcurrenciaOrDefault())
	}

	for _, bad := range []string{"0", "5", "-1"} {
		if _, err := cargar("review:\n  concurrencia: " + bad + "\n"); err == nil || !strings.Contains(err.Error(), "concurrencia tiene que ser un entero de 1 a 4") {
			t.Fatalf("concurrencia=%s debe fallar: %v", bad, err)
		}
	}
}
