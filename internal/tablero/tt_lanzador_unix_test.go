//go:build !windows

// El lanzador y el editor reales en Unix (CA-522, CA-524, CA-527, CA-528):
// LanzadorDe corre un hoom falso (un script sh, ninguna CLI de IA) desprendido
// en su propio grupo de procesos y su propia sesion, con stdin vacio y la
// salida a dos archivos; EditorDe corre un editor falso por el shell con
// ArgumentosEditor.
package tablero_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttScript escribe un script sh ejecutable.
func ttScript(t *testing.T, ruta, cuerpo string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte("#!/bin/sh\n"+cuerpo), 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// ttHoomFalso es un hoom que anota sus args, su directorio, su pid y su
// stdin, espera el archivo "sigue" (10s como mucho) y despues dice lo suyo y
// sale con codigo.
func ttHoomFalso(t *testing.T, dir, stdout, stderr string, codigo int) string {
	t.Helper()
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	cuerpo := "for a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + q(filepath.Join(dir, "args")) + "\n" +
		"pwd -P > " + q(filepath.Join(dir, "pwd")) + "\n" +
		"cat > " + q(filepath.Join(dir, "stdin")) + "\n" +
		"echo $$ > " + q(filepath.Join(dir, "pid.tmp")) + " && mv " + q(filepath.Join(dir, "pid.tmp")) + " " + q(filepath.Join(dir, "pid")) + "\n" +
		"i=0\nwhile [ ! -f " + q(filepath.Join(dir, "sigue")) + " ] && [ $i -lt 200 ]; do sleep 0.05; i=$((i+1)); done\n" +
		"printf '%s' " + q(stdout) + "\n" +
		"printf '%s' " + q(stderr) + " >&2\n" +
		"exit " + strconv.Itoa(codigo) + "\n"
	return ttScript(t, filepath.Join(dir, "bin", "hoom"), cuerpo)
}

func ttLeer(t *testing.T, ruta string) string {
	t.Helper()
	raw, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", ruta, err)
	}
	return string(raw)
}

func ttEsperarArchivo(t *testing.T, ca, ruta string) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ruta); err == nil {
			return
		}
		if time.Now().After(limite) {
			t.Fatalf("%s: el proceso lanzado no llego a escribir %s en 5s", ca, ruta)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ttFines junta las llamadas a fin.
type ttFines struct {
	mu    sync.Mutex
	fines []tablero.FinProceso
	llego chan struct{}
}

func ttNuevosFines() *ttFines { return &ttFines{llego: make(chan struct{}, 8)} }

func (f *ttFines) fin(p tablero.FinProceso) {
	f.mu.Lock()
	f.fines = append(f.fines, p)
	f.mu.Unlock()
	f.llego <- struct{}{}
}

func (f *ttFines) copia() []tablero.FinProceso {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tablero.FinProceso(nil), f.fines...)
}

// CA-528: LanzadorDe corre hoom con los Args en dir, desprendido (su propio
// grupo de procesos y su propia sesion), con stdin vacio; Lanzar no espera;
// fin llega una vez con el codigo y la ultima linea de stderr sin "hoom: ";
// la salida queda en dos archivos nuevos de logs (que se crea).
func TestCA528_LanzadorDeDesprendido(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	trabajo := filepath.Join(base, "proyecto")
	if err := os.MkdirAll(trabajo, 0o755); err != nil {
		t.Fatal(err)
	}
	marcas := filepath.Join(base, "marcas")
	hoom := ttHoomFalso(t, marcas, "una linea de stdout\n", "algo antes\nhoom: ya se esta lanzando un trabajo en esta tarjeta\n\n", 1)
	t.Cleanup(func() { os.WriteFile(filepath.Join(marcas, "sigue"), nil, 0o644) })
	logs := filepath.Join(base, "no", "existe", "logs")
	l := ttLanzadorDe(t, hoom, trabajo, logs)
	fines := ttNuevosFines()
	orden := tablero.Lanzar{ID: 7, Slug: "tarjeta", Rol: "writer",
		Args: []string{"pedir", "writer", "tarjeta", "-m", "un pedido con espacios, 'comillas' y $HOME"}}
	empezo := time.Now()
	if err := l.Lanzar(orden, fines.fin); err != nil {
		t.Fatalf("CA-528: Lanzar arranca el proceso: %v", err)
	}
	if d := time.Since(empezo); d > 2*time.Second {
		t.Fatalf("CA-528: Lanzar no espera al proceso (tardo %v)", d)
	}
	ttEsperarArchivo(t, "CA-528", filepath.Join(marcas, "pid"))
	if len(fines.copia()) != 0 {
		t.Fatalf("CA-528: fin no llega antes de que el proceso termine")
	}
	if got := ttLeer(t, filepath.Join(marcas, "args")); got != strings.Join(orden.Args, "\n")+"\n" {
		t.Fatalf("CA-528: hoom recibe los Args exactos, uno por argumento:\n%q", got)
	}
	if got := strings.TrimSpace(ttLeer(t, filepath.Join(marcas, "pwd"))); got != trabajo {
		t.Fatalf("CA-528: hoom corre en dir %q, corrio en %q", trabajo, got)
	}
	ttEsperarArchivo(t, "CA-528", filepath.Join(marcas, "stdin"))
	if got := ttLeer(t, filepath.Join(marcas, "stdin")); got != "" {
		t.Fatalf("CA-528: stdin vacio: %q", got)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(ttLeer(t, filepath.Join(marcas, "pid"))))
	if err != nil {
		t.Fatal(err)
	}
	pgid, err := unix.Getpgid(pid)
	if err != nil {
		t.Fatalf("CA-528: el proceso sigue vivo mientras espera: %v", err)
	}
	if pgid != pid || pgid == unix.Getpgrp() {
		t.Fatalf("CA-528: el proceso tiene su propio grupo (pgid %d, pid %d, el del test %d)", pgid, pid, unix.Getpgrp())
	}
	if sid, err := unix.Getsid(pid); err != nil || sid != pid {
		t.Fatalf("CA-528: el proceso tiene su propia sesion: sid %d (%v), pid %d", sid, err, pid)
	}
	if err := os.WriteFile(filepath.Join(marcas, "sigue"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fines.llego:
	case <-time.After(10 * time.Second):
		t.Fatalf("CA-528: fin llega cuando el proceso termina")
	}
	time.Sleep(200 * time.Millisecond)
	fs := fines.copia()
	if len(fs) != 1 {
		t.Fatalf("CA-528: fin llega una sola vez: %#v", fs)
	}
	if fs[0].ID != 7 || fs[0].Code != 1 || fs[0].Ultima != "ya se esta lanzando un trabajo en esta tarjeta" {
		t.Fatalf("CA-528: fin trae el ID, el codigo y la ultima linea no vacia de stderr sin 'hoom: ': %#v", fs[0])
	}
	entradas, err := os.ReadDir(logs)
	if err != nil {
		t.Fatalf("CA-528: el directorio logs se crea: %v", err)
	}
	if len(entradas) != 2 {
		t.Fatalf("CA-528: la salida va a dos archivos nuevos de logs: %v", entradas)
	}
	var contenidos []string
	for _, e := range entradas {
		contenidos = append(contenidos, ttLeer(t, filepath.Join(logs, e.Name())))
	}
	junto := strings.Join(contenidos, "\x00")
	if !strings.Contains(junto, "una linea de stdout\n") || !strings.Contains(junto, "hoom: ya se esta lanzando un trabajo en esta tarjeta\n") {
		t.Fatalf("CA-528: stdout y stderr quedan en sus archivos: %q", contenidos)
	}
	for _, c := range contenidos {
		if strings.Contains(c, "una linea de stdout") && strings.Contains(c, "ya se esta lanzando") {
			t.Fatalf("CA-528: stdout y stderr van a dos archivos distintos: %q", c)
		}
	}
}

// CA-528: con stderr vacio, Ultima es la ultima linea no vacia de stdout
// (sin "hoom: "); un segundo lanzamiento escribe archivos nuevos.
func TestCA528_LanzadorDeUltimaDeStdout(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marcas := filepath.Join(base, "marcas")
	hoom := ttHoomFalso(t, marcas, "primera\nhoom: tarjeta fijada para esta terminal: x\n\n", "", 0)
	if err := os.WriteFile(filepath.Join(marcas, "sigue"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(base, "logs")
	l := ttLanzadorDe(t, hoom, base, logs)
	for i := 1; i <= 2; i++ {
		fines := ttNuevosFines()
		if err := l.Lanzar(tablero.Lanzar{ID: i, Slug: "x", Args: []string{"usar", "x"}}, fines.fin); err != nil {
			t.Fatalf("CA-528: Lanzar: %v", err)
		}
		select {
		case <-fines.llego:
		case <-time.After(10 * time.Second):
			t.Fatalf("CA-528: fin llega cuando el proceso termina")
		}
		if f := fines.copia()[0]; f.ID != i || f.Code != 0 || f.Ultima != "tarjeta fijada para esta terminal: x" {
			t.Fatalf("CA-528: sin stderr, Ultima es la ultima linea de stdout: %#v", f)
		}
	}
	if entradas, _ := os.ReadDir(logs); len(entradas) != 4 {
		t.Fatalf("CA-528: cada lanzamiento escribe dos archivos nuevos: %v", entradas)
	}
}

// CA-528: un binario que no existe: Lanzar da error y fin no se llama.
func TestCA528_LanzadorDeSinBinario(t *testing.T) {
	base := t.TempDir()
	l := ttLanzadorDe(t, filepath.Join(base, "no-existe", "hoom"), base, filepath.Join(base, "logs"))
	fines := ttNuevosFines()
	if err := l.Lanzar(tablero.Lanzar{ID: 1, Slug: "x", Args: []string{"usar", "x"}}, fines.fin); err == nil {
		t.Fatalf("CA-528: sin binario Lanzar da error")
	}
	time.Sleep(300 * time.Millisecond)
	if fs := fines.copia(); len(fs) != 0 {
		t.Fatalf("CA-528: si no arranco, fin no se llama: %#v", fs)
	}
}

// ttEditorDe es EditorDe, que tiene que dar un Editor.
func ttEditorDe(t *testing.T, comando string) tablero.Editor {
	t.Helper()
	e := tablero.EditorDe(comando, "linux")
	if e == nil {
		t.Fatalf("CA-524: EditorDe da un Editor")
	}
	return e
}

// ttLanzadorDe es LanzadorDe, que tiene que dar un Lanzador.
func ttLanzadorDe(t *testing.T, hoom, dir, logs string) tablero.Lanzador {
	t.Helper()
	l := tablero.LanzadorDe(hoom, dir, logs)
	if l == nil {
		t.Fatalf("CA-528: LanzadorDe da un Lanzador")
	}
	return l
}

// CA-522, CA-524: EditorDe abre el archivo con ArgumentosEditor por el shell
// (una ruta con espacios llega como un solo argumento); un editor que falla
// es un error.
func TestCA524_EditorDeAbre(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marcas := filepath.Join(base, "marcas")
	cuerpo := "for a in \"$@\"; do printf '%s\\n' \"$a\"; done > '" + filepath.Join(marcas, "args") + "'\n"
	for nombre, quiere := range map[string]func(f string) string{
		"vi":   func(f string) string { return "+12\n" + f + "\n" },
		"code": func(f string) string { return "-g\n" + f + ":12\n" },
		"subl": func(f string) string { return f + "\n" },
	} {
		ed := ttScript(t, filepath.Join(base, "bin-"+nombre, nombre), cuerpo)
		archivo := filepath.Join(base, "un dir con espacios", "a.go")
		os.Remove(filepath.Join(marcas, "args"))
		os.MkdirAll(marcas, 0o755)
		e := ttEditorDe(t, ed)
		if err := e.Abrir(archivo, 12); err != nil {
			t.Fatalf("CA-524: Abrir con %s: %v", nombre, err)
		}
		if got := ttLeer(t, filepath.Join(marcas, "args")); got != quiere(archivo) {
			t.Fatalf("CA-524: %s recibe %q, se esperaba %q", nombre, got, quiere(archivo))
		}
	}
	falla := ttScript(t, filepath.Join(base, "bin-falla", "vi"), "exit 3\n")
	if err := ttEditorDe(t, falla).Abrir(filepath.Join(base, "a.go"), 1); err == nil {
		t.Fatalf("CA-524: un editor que sale con 3 es un error")
	}
}

// CA-527: EditorDe.Editar abre el texto en un archivo temporal y devuelve lo
// que queda en el cuando el editor termina; un editor que falla es un error.
func TestCA527_EditorDeEdita(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	recibido := filepath.Join(base, "recibido")
	ed := ttScript(t, filepath.Join(base, "bin", "nano"),
		"cat \"$1\" > '"+recibido+"'\nprintf 'pedido editado\\ncon dos lineas\\n' > \"$1\"\n")
	got, err := ttEditorDe(t, ed).Editar("pedido original\n")
	if err != nil {
		t.Fatalf("CA-527: Editar: %v", err)
	}
	if got != "pedido editado\ncon dos lineas\n" {
		t.Fatalf("CA-527: Editar devuelve lo que dejo el editor: %q", got)
	}
	if r := ttLeer(t, recibido); r != "pedido original\n" {
		t.Fatalf("CA-527: el editor recibe el pedido en el archivo: %q", r)
	}
	falla := ttScript(t, filepath.Join(base, "bin-falla", "nano"), "exit 1\n")
	if _, err := ttEditorDe(t, falla).Editar("x"); err == nil {
		t.Fatalf("CA-527: un editor que falla es un error")
	}
}
