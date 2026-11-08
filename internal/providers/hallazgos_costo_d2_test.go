// Tests del writer para los hallazgos de la delta 2 de costo-por-spec
// (registro 20261107T180547_fe87f0) en la busqueda de la sesion de Codex.
package providers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Hallazgos 2e32a9 y 62deb0 — CA-541: el tope de archivos es parte del
// presupuesto y corta la busqueda en el candidato que lo pasa, sin recorrer
// ni guardar el resto del directorio.
func TestHallazgo_2e32a9_62deb0_CA541_ElTopeDeArchivosCortaEnElCandidato(t *testing.T) {
	home := t.TempDir()
	dia := filepath.Join(home, "sessions", "2026", "11", "07")
	if err := os.MkdirAll(dia, 0o755); err != nil {
		t.Fatal(err)
	}
	const total = 3000
	for i := 0; i < total; i++ {
		n := fmt.Sprintf("rollout-2026-11-07T10-00-%04d-%s.jsonl", i, cpsThread)
		if err := os.WriteFile(filepath.Join(dia, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	viejo := limitesSesion
	t.Cleanup(func() { limitesSesion = viejo })
	limitesSesion.archivos = 2
	b := nuevoPresupuesto()
	antes := b.entradas
	if _, err := buscarSesiones(home, cpsThread, b); !errors.Is(err, errTopeSesion) {
		t.Fatalf("mas candidatos que el tope es sin dato: %v", err)
	}
	if mirados := antes - b.entradas; mirados > 512 {
		t.Fatalf("corta en el candidato que pasa el tope: miro %d de %d entradas", mirados, total)
	}
	if b.archivos >= 0 {
		t.Fatalf("el cupo de archivos se gasta en el presupuesto: queda %d", b.archivos)
	}
	t.Setenv("CODEX_HOME", home)
	limitesSesion = viejo
	limitesSesion.archivos = 2
	start := time.Now()
	if _, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, start); !errors.Is(err, errTopeSesion) {
		t.Fatalf("MedirSesion corta igual: %v", err)
	}
}
