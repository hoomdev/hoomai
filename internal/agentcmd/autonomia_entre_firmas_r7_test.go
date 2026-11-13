//go:build unix

// Tests de regresion de la ronda 7 de autonomia-entre-firmas sobre la foto
// del material de git (aclaracion 28):
//
//   - hallazgo 20261113T051843_192f73 (CA-638): con un archivo disperso de
//     64 GiB en el directorio de hooks efectivo (core.hooksPath fuera del
//     worktree), el sobre de un writer que solo cambia codigo termina en
//     segundos y no corta por el material de git; uno que le cambia el
//     tamano o la fecha a ese archivo corta en scope como en la aclaracion
//     25.
//
// El archivo es disperso (os.Truncate, nada se escribe en disco); si el
// sistema de archivos de los temporales no lo soporta, el test salta. Usa
// syscall.Stat_t para ver los bloques ocupados, por eso es solo unix.
// Reusa los helpers de las rondas anteriores (aeaProyecto, ar6Cortado,
// ar6DelGate, ar6Ruta). CLIs falsos (fakeProvider).
package agentcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// ar7Tamano es el tamano aparente del archivo disperso: 64 GiB.
const ar7Tamano = 64 << 30

// ar7Enorme arma la tarea demo con core.hooksPath apuntando a un directorio
// fuera del worktree que tiene un archivo disperso de 64 GiB (enorme) y un
// pre-commit chico. Salta si el sistema de archivos no soporta dispersos.
func ar7Enorme(t *testing.T) (root, wt, hooks, enorme string) {
	t.Helper()
	root, wt = aeaProyecto(t, "true", "")
	hooks = t.TempDir()
	ar6Escribir(t, filepath.Join(hooks, "pre-commit"), "#!/bin/sh\nexit 0\n", 0o755)
	enorme = filepath.Join(hooks, "enorme")
	if err := os.WriteFile(enorme, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(enorme, ar7Tamano); err != nil {
		t.Skipf("192f73/CA-638: el sistema de archivos de los temporales no deja un archivo disperso de 64 GiB: %v", err)
	}
	fi, err := os.Stat(enorme)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || fi.Size() != ar7Tamano || int64(st.Blocks)*512 > 1<<20 {
		_ = os.Remove(enorme)
		t.Skipf("192f73/CA-638: el sistema de archivos de los temporales no soporta archivos dispersos (tamano %d, bloques %v)", fi.Size(), st)
	}
	git(t, wt, "config", "core.hooksPath", hooks)
	if h := ar6Ruta(t, wt, "--git-path", "hooks"); h != hooks {
		t.Fatalf("fixture: el directorio de hooks efectivo es %s: %s", hooks, h)
	}
	return root, wt, hooks, enorme
}

// ar7Acotado corre el writer con el CLI falso script y exige que su sobre
// termine en menos de 20 s. Si no, espera hasta 3 minutos a que termine (para
// que los temporales se puedan borrar) y falla.
func ar7Acotado(t *testing.T, ca, root, script string) (Result, string) {
	t.Helper()
	fakeProvider(t, "claude", script)
	type fin struct {
		res Result
		err error
	}
	var buf bytes.Buffer
	listo := make(chan fin, 1)
	desde := time.Now()
	go func() {
		res, err := Run(root, "main", Options{Role: "writer", Task: aeaTarea, Prompt: "trabaja en la tarea demo"}, &buf)
		listo <- fin{res, err}
	}()
	var f fin
	select {
	case f = <-listo:
	case <-time.After(20 * time.Second):
		select {
		case <-listo:
		case <-time.After(3 * time.Minute):
		}
		t.Fatalf("%s: con un archivo de 64 GiB en el directorio de hooks efectivo, el sobre del writer termina en menos de 20 s (aclaracion 28); tardo %s", ca, time.Since(desde))
	}
	if f.err != nil || f.res.EnvelopeID == "" {
		t.Fatalf("%s: fixture: el sobre del writer corre: %v %+v\n%s", ca, f.err, f.res, buf.String())
	}
	return f.res, buf.String()
}

// Hallazgo 20261113T051843_192f73 (CA-638, aclaracion 28): con un archivo
// disperso de 64 GiB en el core.hooksPath (fuera del worktree), un writer
// que solo cambia codigo cierra su sobre en menos de 20 s, el sobre no
// corta, verify corre y no queda ningun hallazgo abierto del gate de scope.
func TestHallazgo192f73_CA638_UnArchivoEnormeEnLosHooksNoCuelgaElSobre(t *testing.T) {
	const ca = "192f73/CA-638 (solo codigo)"
	root, wt, _, enorme := ar7Enorme(t)
	res, out := ar7Acotado(t, ca, root, ar1Codigo+"exit 0\n")
	if fi, err := os.Stat(enorme); err != nil || fi.Size() != ar7Tamano {
		t.Fatalf("%s: fixture: el archivo enorme queda como estaba: %v %v", ca, fi, err)
	}
	if res.Scope.Cuts() || res.Stage == "scope" || res.VerdictID == "" {
		t.Fatalf("%s: un writer que solo cambia codigo no corta por el material de git y verify corre: %+v\n%s", ca, res, out)
	}
	if hs := ar6DelGate(t, wt, root); len(hs) != 0 {
		t.Fatalf("%s: ningun hallazgo abierto del gate de scope: %+v", ca, hs)
	}
}

// Hallazgo 20261113T051843_192f73 (CA-638, aclaraciones 25 y 28): con el
// mismo archivo disperso de 64 GiB en el core.hooksPath, un writer que
// cambia codigo y ademas le agrega un byte al final (cambia su tamano), o le
// cambia la fecha de modificacion, corta su sobre como en la aclaracion 25:
// scope, no-entregable, exit 1, sin veredicto, con un hallazgo high abierto
// del gate de scope que nombra el archivo. Tambien termina en menos de 20 s.
func TestHallazgo192f73_CA638_CambiarElArchivoEnormeCorta(t *testing.T) {
	for _, c := range []struct{ nombre, script string }{
		{"le agrega un byte al final", "printf 'x' >> \"$(git rev-parse --git-path hooks)/enorme\"\n"},
		{"le cambia la fecha de modificacion", "touch -m -d '2001-02-03 04:05:06' \"$(git rev-parse --git-path hooks)/enorme\"\n"},
	} {
		ca := "192f73/CA-638 (" + c.nombre + ")"
		root, wt, _, enorme := ar7Enorme(t)
		antes, err := os.Stat(enorme)
		if err != nil {
			t.Fatal(err)
		}
		veredictos := aeaVeredictos(root, wt)
		res, out := ar7Acotado(t, ca, root, ar1Codigo+c.script+"exit 0\n")
		if fi, err := os.Stat(enorme); err != nil || (fi.Size() == antes.Size() && fi.ModTime().Equal(antes.ModTime())) {
			t.Fatalf("%s: fixture: el writer cambio el archivo enorme: %v %v", ca, fi, err)
		}
		ar6Cortado(t, ca, c.nombre, root, wt, res, out, veredictos, "enorme")
	}
}
