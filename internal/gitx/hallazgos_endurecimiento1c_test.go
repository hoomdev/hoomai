// Tests de regresion de la tercera ronda de la tarea endurecimiento-1 en
// gitx, sobre .hoom/specs/evidencia-en-disco.md (CA-442). Escritos desde el
// spec, el texto del hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Reusan git, write (touched_test.go) e idAislarGit
// (identity_test.go); los helpers de este archivo llevan el prefijo e1c.
//
//   - 20261023T123045_cb8107 y 20261023T123540_b6d4ca (reliability,
//     resilience): el listado de lo local conservaba solo el PRIMER error de
//     los git que corre. Si `git diff HEAD` fallaba rapido y despues
//     `git ls-files --others` no respondia dentro del tope, el error que
//     quedaba era el rapido; HuellaConTope solo reconoce el de "no respondio"
//     y seguia con un listado parcial: devolvia (y grababa) una huella aunque
//     un git no respondio. Ahora HuellaConTope devuelve error y ninguna
//     huella: un git que no responde prevalece sobre un error anterior.
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// e1cGitFalso pone al frente del PATH un git que, segun el modo, falla
// enseguida (exit 128, "fatal:") en `git diff ... HEAD` y/o se queda sin
// responder (exec sleep 30) en `git ls-files --others`; cualquier otra
// invocacion es el git de verdad. Devuelve los archivos que marcan que cada
// caso se disparo.
func e1cGitFalso(t *testing.T, fallaDiff, cuelgaLs bool) (marcaDiff, marcaLs string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("el git falso es un script de sh: el caso se prueba en Linux y macOS")
	}
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("sin git en el PATH")
	}
	real, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	bin, marcas := t.TempDir(), t.TempDir()
	marcaDiff, marcaLs = filepath.Join(marcas, "diff"), filepath.Join(marcas, "ls-files")
	si := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	s := "#!/bin/sh\n" +
		"diff=0; head=0; ls=0; others=0\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in\n" +
		"    diff) diff=1 ;;\n" +
		"    HEAD) head=1 ;;\n" +
		"    ls-files) ls=1 ;;\n" +
		"    --others) others=1 ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ " + si(fallaDiff) + " = 1 ] && [ $diff = 1 ] && [ $head = 1 ]; then\n" +
		"  : > '" + marcaDiff + "'\n" +
		"  echo 'fatal: git diff roto por la prueba (endurecimiento-1c)' >&2\n" +
		"  exit 128\n" +
		"fi\n" +
		"if [ " + si(cuelgaLs) + " = 1 ] && [ $ls = 1 ] && [ $others = 1 ]; then\n" +
		"  : > '" + marcaLs + "'\n" +
		"  exec sleep 30\n" +
		"fi\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marcaDiff, marcaLs
}

// e1cRepo arma un repo con un commit, un archivo modificado y uno sin
// seguimiento.
func e1cRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@hoom.dev")
	git(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "app.go", "package app\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "inicial")
	write(t, dir, "app.go", "package app // tocado\n")
	write(t, dir, "nuevo.txt", "sin seguimiento\n")
	return dir
}

// e1cHuella corre HuellaConTope con reloj propio (60 s) y dice cuanto tardo.
func e1cHuella(t *testing.T, ca, dir string, tope time.Duration) (string, error, time.Duration) {
	t.Helper()
	type res struct {
		h   string
		err error
	}
	listo := make(chan res, 1)
	t0 := time.Now()
	go func() {
		h, err := HuellaConTope(dir, tope)
		listo <- res{h, err}
	}()
	select {
	case r := <-listo:
		return r.h, r.err, time.Since(t0)
	case <-time.After(60 * time.Second):
		t.Fatalf("%s: HuellaConTope con tope %s no volvio en 60 s", ca, tope)
		return "", nil, 0
	}
}

// CA-442 (hallazgos cb8107 y b6d4ca): con gitx.TopeListado y el tope de
// HuellaConTope en 1 s, si `git diff HEAD` falla enseguida y `git ls-files
// --others` no responde, HuellaConTope devuelve error y ninguna huella (no
// una huella del listado parcial), y vuelve acotado por el tope (menos de
// 15 s, no los 30 s del git colgado). Guardas: con el git de verdad hay
// huella sin error; con solo ls-files colgado (diff sano), error y ninguna
// huella; y los dos casos del git falso se disparan.
func TestHallazgo_cb8107_b6d4ca_CA442_GitQueNoRespondeDespuesDeUnFalloRapidoEsUnErrorDeHuellaConTope(t *testing.T) {
	const ca = "CA-442 (hallazgos cb8107 y b6d4ca, timeout despues de un fallo rapido)"
	idAislarGit(t)
	viejo := TopeListado
	TopeListado = time.Second
	t.Cleanup(func() { TopeListado = viejo })
	const tope = time.Second
	dir := e1cRepo(t)

	if h, err, _ := e1cHuella(t, ca, dir, 10*time.Second); err != nil || h == "" {
		t.Fatalf("%s: guarda: con el git de verdad HuellaConTope da una huella sin error: %q %v", ca, h, err)
	}

	t.Run("guarda: solo ls-files no responde", func(t *testing.T) {
		_, marcaLs := e1cGitFalso(t, false, true)
		h, err, tardo := e1cHuella(t, ca, dir, tope)
		if _, e := os.Stat(marcaLs); e != nil {
			t.Fatalf("%s: fixture: HuellaConTope corre git ls-files --others (el git falso no se disparo)", ca)
		}
		if err == nil || h != "" {
			t.Fatalf("%s: guarda: si git ls-files no responde, HuellaConTope da error y ninguna huella: %q %v (tardo %s)", ca, h, err, tardo.Round(time.Millisecond))
		}
	})

	t.Run("diff HEAD falla rapido y ls-files no responde", func(t *testing.T) {
		marcaDiff, marcaLs := e1cGitFalso(t, true, true)
		h, err, tardo := e1cHuella(t, ca, dir, tope)
		_, eDiff := os.Stat(marcaDiff)
		_, eLs := os.Stat(marcaLs)
		if eDiff != nil || eLs != nil {
			t.Fatalf("%s: fixture: HuellaConTope corre git diff HEAD y git ls-files --others (diff disparado: %v, ls-files disparado: %v)",
				ca, eDiff == nil, eLs == nil)
		}
		if err == nil || h != "" {
			t.Errorf("%s: git diff HEAD fallo rapido y git ls-files --others no respondio dentro del tope: HuellaConTope devuelve error y ninguna huella, no la huella %q del listado parcial (err %v, tardo %s)",
				ca, h, err, tardo.Round(time.Millisecond))
		}
		if tardo > 15*time.Second {
			t.Errorf("%s: con tope de %s, HuellaConTope vuelve acotado: tardo %s", ca, tope, tardo.Round(time.Millisecond))
		}
	})
}
