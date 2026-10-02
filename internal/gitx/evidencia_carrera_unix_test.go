//go:build !windows

// Carrera del hallazgo 98faf2 de la segunda ronda de review de
// .hoom/specs/evidencia-en-disco.md (CA-442: un symlink o un FIFO "nunca se
// sigue ni se lee") en la parte de la foto que pasa por git: Touched no se
// cuelga si un proceso que la corrida dejo vivo cambia una evidencia que git
// ve entre un archivo regular, un symlink a un FIFO y el FIFO mismo. La
// carrera es la de internal/carreratest (hallazgo b08c53: una sola copia).
package gitx

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// CA-442 (hallazgo 98faf2, propiedad de carrera): en un repo donde git ve
// .hoom/findings/<id>.json sin commitear, mientras otro proceso la cambia con
// rename atomico y sin parar entre un archivo regular, un symlink a un FIFO
// de afuera y el FIFO mismo (un enlace duro; nadie lo escribe), CADA Touched
// vuelve (con reloj por llamada: si una se cuelga, el test falla en vez de
// colgarse).
func TestCA442_TouchedNoSeCuelgaSiLaEvidenciaCambiaEntreRegularYFIFO(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial")
	const rel = ".hoom/findings/20261001T060606_ca2e2a.json"
	regular := []byte("{\"severity\":\"high\"}\n")
	write(t, root, rel, string(regular))
	if got := Touched(root, "main"); got[rel] == "" {
		t.Fatalf("CA-442: fixture: git ve %s sin commitear: %v", rel, got)
	}

	fifo := filepath.Join(t.TempDir(), "fifo")
	carreratest.Fifo(t, fifo) // al final suelta una lectura colgada (de un git hijo, por ejemplo)
	detener := carreratest.RegularYFifo(t, filepath.Join(root, rel), fifo, regular)

	const reloj = 5 * time.Second
	llamadas := 0
	fin := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(fin) {
		ch := make(chan map[string]string, 1)
		go func() { ch <- Touched(root, "main") }()
		select {
		case <-ch:
		case <-time.After(reloj):
			detener()
			t.Fatalf("CA-442: Touched no volvio en %s mientras %s cambiaba entre un archivo regular y un FIFO", reloj, rel)
		}
		llamadas++
	}
	cambios := detener()
	if cambios < 4 {
		t.Fatalf("CA-442: fixture: el intercambiador cambio %s %d veces", rel, cambios)
	}
	t.Logf("CA-442: %d llamadas a Touched, %d cambios", llamadas, cambios)
}
