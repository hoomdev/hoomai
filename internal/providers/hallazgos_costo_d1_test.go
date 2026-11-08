// Tests del writer para los hallazgos de la delta 1 de costo-por-spec
// (registro 20261107T173245_7f0aa5) en la lectura de la sesion de Codex.
package providers

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Hallazgos 85877f y 168202 — CA-541: el tope de bytes y el plazo se miran
// en cada fragmento de una linea, no al terminarla: una sola linea sin fin
// (o sin salto de linea) corta la lectura dentro del tope.
func TestHallazgo_85877f_168202_CA541_UnaLineaSinFinCortaDentroDelTope(t *testing.T) {
	// an endless line: a reader that never gives a newline nor EOF
	sinFin := bufio.NewReaderSize(io.MultiReader(strings.NewReader(`{"type":"response_item","x":"`), infinito{}), 64<<10)
	b := &presupuesto{bytes: 1 << 20, entradas: 1, hasta: time.Now().Add(time.Minute)}
	if _, _, err := leerLineaAcotada(sinFin, maxLineaSesion, b); !errors.Is(err, errTopeSesion) {
		t.Fatalf("la linea sin fin pasa el tope de bytes: %v", err)
	}
	if b.bytes < -(64 << 10) {
		t.Fatalf("corta en el fragmento que pasa el tope, no despues: quedan %d bytes", b.bytes)
	}
	sinFin = bufio.NewReaderSize(infinito{}, 64<<10)
	b = &presupuesto{bytes: 1 << 40, entradas: 1, hasta: time.Now().Add(50 * time.Millisecond)}
	listo := make(chan error, 1)
	go func() { _, _, err := leerLineaAcotada(sinFin, maxLineaSesion, b); listo <- err }()
	select {
	case err := <-listo:
		if !errors.Is(err, errTopeSesion) {
			t.Fatalf("la linea sin fin pasa el plazo: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el plazo se mira dentro de la linea: la lectura no termino")
	}

	// the whole reading: one line bigger than the byte bound, no newline
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t0 := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	f := cpsEscribirSesion(t, home, t0, cpsThread, cpsSessionMeta(t0, cpsThread))
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fh.WriteString(`{"type":"response_item","x":"` + strings.Repeat("a", 1<<20)); err != nil {
		t.Fatal(err)
	}
	fh.Close()
	viejo := limitesSesion
	t.Cleanup(func() { limitesSesion = viejo })
	limitesSesion.bytes = 128 << 10
	if _, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0); !errors.Is(err, errTopeSesion) {
		t.Fatalf("una linea mayor que el tope es sin dato: %v", err)
	}
}

// infinito is a reader that never ends and never gives a newline.
type infinito struct{}

func (infinito) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// Hallazgo 843fbf — CA-541: buscar los archivos tambien cae dentro del
// presupuesto: un arbol de sesiones con mas entradas que el tope, o un plazo
// vencido, es sin dato antes de leer nada; y la busqueda encuentra lo mismo
// que el patron sessions/*/*/*/rollout-*-<thread>.jsonl.
func TestHallazgo_843fbf_CA541_LaBusquedaTieneTopes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t0 := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	cpsEscribirSesion(t, home, t0, cpsThread, cpsSessionMeta(t0, cpsThread),
		cpsTokenCount(t0.Add(time.Second), &cpsRL{used: 10, minutes: 300, resets: 1794053319}, nil, "plus"))
	dia := filepath.Join(home, "sessions", "2026", "11", "06")
	if err := os.MkdirAll(dia, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		if err := os.WriteFile(filepath.Join(dia, "otro-"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+".jsonl"), nil, 0o644); err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
	}
	viejo := limitesSesion
	t.Cleanup(func() { limitesSesion = viejo })

	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0)
	if err != nil || len(m.Muestras) != 1 {
		t.Fatalf("con los topes de fabrica la sesion se lee: %v %+v", err, m)
	}

	limitesSesion.entradas = 50
	if _, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0); !errors.Is(err, errTopeSesion) {
		t.Fatalf("mas entradas que el tope es sin dato: %v", err)
	}
	limitesSesion = viejo
	limitesSesion.plazo = -time.Second
	if _, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0); !errors.Is(err, errTopeSesion) {
		t.Fatalf("el plazo cuenta desde la busqueda: %v", err)
	}
	limitesSesion = viejo

	// the same files the glob found: a symlinked day directory is followed, a
	// name that only looks like it is not taken
	otroDia := filepath.Join(t.TempDir(), "dia")
	if err := os.MkdirAll(otroDia, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otroDia, filepath.Join(home, "sessions", "2026", "11", "05")); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"rollout--" + cpsThread + ".jsonl", "rollout-" + cpsThread + ".jsonl", "xrollout-a-" + cpsThread + ".jsonl"} {
		if err := os.WriteFile(filepath.Join(otroDia, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := buscarSesiones(home, cpsThread, nuevoPresupuesto())
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+cpsThread+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || len(got) != 2 {
		t.Fatalf("la busqueda encuentra lo que el patron:\n got %q\nwant %q", got, want)
	}
}
