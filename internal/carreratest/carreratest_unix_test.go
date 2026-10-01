//go:build !windows

// Tests de la fixture misma (hallazgo b08c53 de la tercera ronda de review
// de .hoom/specs/evidencia-en-disco.md): si el intercambiador no cambia de
// verdad la entrada, o no devuelve el directorio, las carreras de CA-441 y
// CA-442 que lo usan pasarian sin medir nada.
package carreratest

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ctReloj es cuanto se espera, como mucho, ver cada forma de la carrera.
const ctReloj = 5 * time.Second

// CA-442 (b08c53): Fifo crea un FIFO (y el directorio que falta arriba), y
// al final del test suelta una lectura que se quedo colgada en el.
func TestCA442_FifoCreaUnFIFOYSueltaLaLecturaColgadaAlFinal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "falta", "fifo")
	listo := make(chan error, 1)
	t.Run("lectura colgada", func(t *testing.T) {
		Fifo(t, p)
		fi, err := os.Lstat(p)
		if err != nil || fi.Mode().Type() != fs.ModeNamedPipe {
			t.Fatalf("CA-442: fixture: Fifo crea un FIFO en %s: %v %v", p, fi, err)
		}
		empezo := make(chan struct{})
		go func() {
			close(empezo)
			_, err := os.ReadFile(p) // se cuelga: nadie lo escribe
			listo <- err
		}()
		<-empezo
		time.Sleep(50 * time.Millisecond)
		select {
		case err := <-listo:
			t.Fatalf("CA-442: fixture: leer un FIFO que nadie escribe se cuelga, y volvio: %v", err)
		default:
		}
	})
	select {
	case <-listo:
	case <-time.After(ctReloj):
		t.Fatalf("CA-442: fixture: al final del test Fifo suelta la lectura colgada en %s", p)
	}
}

// CA-442 (b08c53): Intercambiar pone en el destino cada forma por turno, la
// ruta nunca falta, y despues de pararlo (dos veces: es idempotente) no
// cambia mas.
func TestCA442_IntercambiarPoneCadaFormaPorTurnoSinQueFalteLaRuta(t *testing.T) {
	dir := t.TempDir()
	destino := filepath.Join(dir, "20261001T060606_ca2e2a.json")
	a, b := []byte("forma a\n"), []byte("forma b, otra\n")
	if err := os.WriteFile(destino, a, 0o644); err != nil {
		t.Fatal(err)
	}
	afuera := filepath.Join(t.TempDir(), "afuera.json")
	if err := os.WriteFile(afuera, []byte("afuera\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parar := Intercambiar(t, destino, Regular(a), Symlink(afuera), Regular(b))
	vistas := map[string]bool{}
	fin := time.Now().Add(ctReloj)
	for len(vistas) < 3 && time.Now().Before(fin) {
		fi, err := os.Lstat(destino)
		if err != nil {
			parar()
			t.Fatalf("CA-442: fixture: con rename atomico %s nunca falta: %v", destino, err)
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			vistas["symlink"] = true
			continue
		}
		raw, err := os.ReadFile(destino)
		if err == nil && (bytes.Equal(raw, a) || bytes.Equal(raw, b)) {
			vistas[string(raw)] = true
		}
	}
	n := parar()
	if len(vistas) < 3 || n < 3 {
		t.Fatalf("CA-442: fixture: el intercambiador puso cada forma (vio %v en %d cambios)", vistas, n)
	}
	if m := parar(); m != n {
		t.Fatalf("CA-442: fixture: parado, el intercambiador no cambia mas: %d y despues %d", n, m)
	}
	quieto, err := os.Lstat(destino)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if despues, err := os.Lstat(destino); err != nil || !os.SameFile(quieto, despues) {
		t.Fatalf("CA-442: fixture: parado, %s no cambia: %v", destino, err)
	}
}

// CA-442 (b08c53, hallazgos 545ddc/a081cb): IntercambiarDir cambia el
// directorio por cada forma, por turno (se ven el directorio, el symlink, el
// archivo regular y la ruta que falta), y al pararlo deja en el destino el
// MISMO directorio, con lo que tenia adentro.
func TestCA442_IntercambiarDirCambiaElDirectorioPorCadaFormaYLoDevuelve(t *testing.T) {
	dir := t.TempDir()
	destino := filepath.Join(dir, "findings")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	marca := filepath.Join(destino, "20261001T060606_ca2e2a.json")
	if err := os.WriteFile(marca, []byte("{\"severity\":\"high\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original, err := os.Lstat(destino)
	if err != nil {
		t.Fatal(err)
	}
	otro := filepath.Join(dir, "otro")
	if err := os.Mkdir(otro, 0o755); err != nil {
		t.Fatal(err)
	}
	parar := IntercambiarDir(t, destino, Symlink(otro), Symlink("otro"), Regular([]byte("no soy un directorio\n")))
	vistas := map[string]bool{}
	fin := time.Now().Add(ctReloj)
	for len(vistas) < 4 && time.Now().Before(fin) {
		fi, err := os.Lstat(destino)
		switch {
		case err != nil:
			vistas["falta"] = true
		case fi.Mode()&fs.ModeSymlink != 0:
			vistas["symlink"] = true
		case fi.Mode().IsRegular():
			vistas["regular"] = true
		case fi.IsDir() && os.SameFile(fi, original):
			vistas["directorio"] = true
		}
	}
	n := parar()
	if len(vistas) < 4 || n < 1 {
		t.Fatalf("CA-442: fixture: el intercambiador cambio el directorio por cada forma (vio %v en %d vueltas)", vistas, n)
	}
	fi, err := os.Lstat(destino)
	if err != nil || !fi.IsDir() || !os.SameFile(fi, original) {
		t.Fatalf("CA-442: fixture: parado, el destino es el mismo directorio de antes: %v %v", fi, err)
	}
	if raw, err := os.ReadFile(marca); err != nil || !bytes.Equal(raw, []byte("{\"severity\":\"high\"}\n")) {
		t.Fatalf("CA-442: fixture: el directorio vuelve con lo que tenia adentro: %q %v", raw, err)
	}
}
