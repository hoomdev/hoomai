//go:build unix

// Tests de regresion de la ronda 8 de autonomia-entre-firmas, como
// consumidor externo (proyectos git reales, CLIs de IA falsos en un PATH
// minimo, nunca uno real), sobre la aclaracion 30 (el guardado reconoce su
// commit por su propia actualizacion de la rama):
//
//   - hallazgos 20261113T055727_c3e30e, 20261113T060147_401b6d y
//     20261113T060451_59db0e (CA-644): una actualizacion de hoom/<slug> que
//     hace otro proceso mientras el guardado corre no es del guardado; si el
//     guardado falla, no la retira.
//
// La carrera se arma con dos FIFOs (internal/carreratest): por uno el hook
// avisa que esta corriendo, por el otro espera la senal del test. Por eso es
// solo unix. Reusa los helpers de las rondas anteriores (apTuAprobacion,
// apr4Hook, apr5Mirar, apr5Fallo, apr1Arrancar).
package pilotocmd_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
	"github.com/hoomdev/hoomai/internal/itemcmd"
)

// apr8Soltar abre fifo para escribir sin bloquear y lo cierra (una lectura
// colgada en el recibe fin de archivo), reintentando hasta d mientras nadie
// lo este leyendo.
func apr8Soltar(fifo string, d time.Duration) {
	hasta := time.Now().Add(d)
	for {
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
			return
		}
		if time.Now().After(hasta) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Hallazgos 20261113T055727_c3e30e, 20261113T060147_401b6d y
// 20261113T060451_59db0e (CA-644, aclaracion 30): en Tu aprobacion con un
// cambio sin guardar, el pre-commit del guardado avisa por un FIFO que esta
// corriendo, espera la senal del test por otro y despues sale 1 (rechaza el
// commit). Mientras el hook espera, el test (que no es hijo del hook) crea
// un commit X sobre la punta de antes (`git commit-tree` y `git update-ref
// refs/heads/hoom/<slug> X <antes>`) y suelta al hook. `hoom piloto` sale con
// exit 1 y `no pudo guardar el trabajo de la tarjeta: <error>`, sin linea de
// guardado y con el candado libre, y la rama de la tarea queda en X: el
// guardado no retira la actualizacion ajena. TopeGit va en 10 s, de sobra
// para la carrera.
func TestHallazgosC3e30e_401b6d_59db0e_CA644_ElGuardadoNoRetiraUnaActualizacionAjena(t *testing.T) {
	const ca = "c3e30e/401b6d/59db0e/CA-644"
	tope := itemcmd.TopeGit
	itemcmd.TopeGit = 10 * time.Second
	t.Cleanup(func() { itemcmd.TopeGit = tope })

	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "rama-ajena"
	wt := apTuAprobacion(t, root, slug, apConAuto, true)
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	antes := apr5Mirar(t, root, wt, slug)

	aparte := t.TempDir()
	listo, senal := filepath.Join(aparte, "listo"), filepath.Join(aparte, "senal")
	carreratest.Fifo(t, listo)
	carreratest.Fifo(t, senal)
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\nprintf 'corriendo\\n' > '"+listo+"'\nread x < '"+senal+"'\nexit 1\n")

	p := apr1Arrancar(root, slug)
	terminar := func() { // suelta al hook y espera al piloto, para que los temporales se puedan borrar
		apr8Soltar(senal, 2*time.Second)
		p.esperar(time.Minute)
	}
	aviso := make(chan error, 1)
	go func() {
		_, err := os.ReadFile(listo)
		aviso <- err
	}()
	select {
	case err := <-aviso:
		if err != nil {
			terminar()
			t.Fatalf("%s: fixture: el aviso del pre-commit se lee: %v", ca, err)
		}
	case <-time.After(30 * time.Second):
		terminar()
		t.Fatalf("%s: fixture: el guardado del piloto corre el pre-commit (aclaracion 23), que avisa por el FIFO; a los 30 s no aviso:\n%s", ca, p.salida())
	}

	// el hook esta esperando: otro proceso mueve la rama de la tarea desde antes
	arbol := apGit(t, root, "rev-parse", antes.rama+"^{tree}")
	x := apGit(t, root, "commit-tree", arbol, "-p", antes.rama, "-m", "un commit ajeno al guardado")
	apGit(t, root, "update-ref", "refs/heads/hoom/"+slug, x, antes.rama)
	if r := apGit(t, root, "rev-parse", "refs/heads/hoom/"+slug); r != x || x == antes.rama {
		terminar()
		t.Fatalf("%s: fixture: la rama de la tarea paso a X (%s): %s", ca, x, r)
	}

	escrita := make(chan error, 1)
	go func() { escrita <- os.WriteFile(senal, []byte("ya\n"), 0o644) }()
	select {
	case err := <-escrita:
		if err != nil {
			terminar()
			t.Fatalf("%s: fixture: la senal al hook se escribe: %v", ca, err)
		}
	case <-time.After(10 * time.Second):
		terminar()
		t.Fatalf("%s: fixture: el hook toma la senal", ca)
	}
	if !p.esperar(30 * time.Second) {
		terminar()
		t.Fatalf("%s: con el hook suelto, el piloto termina en un tiempo acotado; a los 30 s seguia:\n%s", ca, p.salida())
	}
	s := p.salida()

	apr5Fallo(t, ca, s, root, slug)
	if r := apGit(t, root, "rev-parse", "refs/heads/hoom/"+slug); r != x {
		t.Fatalf("%s: el guardado no retira la actualizacion ajena: hoom/%s es %s, X %s (antes %s)", ca, slug, r, x, antes.rama)
	}
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}
