// Tests de los hallazgos db8541 y f7efdc sobre cockpitcmd.Open (CA-347,
// CA-348), escritos desde .hoom/specs/acciones-desde-la-tarjeta.md:
//
//   - db8541, ENMIENDA 5: "el comando que los errores proponen para cerrar una
//     sesion a mano lleva el destino entre comillas, `tmux kill-session -t
//     '=<session>'`, porque zsh expande una palabra que empieza con `=` como
//     la ruta de un comando". En zsh, `tmux kill-session -t =hoom-demo-x` sin
//     comillas ni llega a tmux ("zsh: hoom-demo-x not found"). cierreEjecutable
//     extrae del mensaje el comando que propone y lo corre, tal cual, con sh -c
//     y con zsh -c contra un tmux falso que anota sus argumentos: tmux tiene
//     que recibir `-t` con exactamente `=<session>`. Lo usan los tests de
//     363285 en hallazgos_c1c_test.go y los de abajo.
//   - f7efdc, con el contrato nuevo de los hallazgos 9c1816, 57bd29 y 03481c
//     (decision de Henry): Open crea la sesion, el split-window falla y
//     cerrarla tambien falla. Este la sesion muerta (la CLI de la IA salio y
//     tmux dice "can't find session", con codigo 1) o viva (kill-session
//     roto, el caso de 363285), el error es UNO SOLO: conserva el del
//     split-window, no afirma que la sesion ya no existe ni que quedo
//     abierta, dice que no se pudo saber si sigue abierta y muestra el
//     comando de cierre `tmux kill-session -t '=<sesion>'` entre comillas.
//     Con la sesion muerta, el reintento la crea y la registra.
//
// Fixtures de open_capture_test.go (cbProyecto, cbLeerItem),
// hallazgos_c1_test.go (h3Tmux, h3Sesion, h3Reintento), hallazgos_c1c_test.go
// (hc3Tmux) y tmuxdoble_test.go (tmuxSinSesion,
// incumplimientosDelCierreFallido).
// Ningun tmux real.
package cockpitcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// dbPATH es el PATH con el que arranco el binario de tests, antes de que un
// test lo cambie: ahi se buscan sh y zsh.
var dbPATH = os.Getenv("PATH")

// dbBuscar busca el ejecutable nombre en dbPATH ("" si no esta).
func dbBuscar(nombre string) string {
	for _, d := range filepath.SplitList(dbPATH) {
		if d == "" {
			continue
		}
		p := filepath.Join(d, nombre)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// comandoDeCierre extrae de msg el comando que propone para cerrar la sesion
// a mano, como lo copiaria una persona: desde `tmux kill-session` hasta el fin
// de la palabra del destino de -t. La palabra es un tramo entre comillas
// simples o dobles, o un tramo suelto de caracteres de nombre (letras,
// digitos, `-`, `_`, `=`, `%` y escapes con `\`). "" si msg no propone ninguno.
func comandoDeCierre(msg string) string {
	const verbo = "tmux kill-session"
	i := strings.Index(msg, verbo)
	if i < 0 {
		return ""
	}
	pos := i + len(verbo)
	espacios := func() int {
		n := 0
		for pos < len(msg) && msg[pos] == ' ' {
			pos++
			n++
		}
		return n
	}
	if espacios() == 0 || !strings.HasPrefix(msg[pos:], "-t") {
		return ""
	}
	pos += len("-t")
	if espacios() == 0 || pos >= len(msg) {
		return ""
	}
	inicio := pos
	if c := msg[pos]; c == '\'' || c == '"' {
		j := strings.IndexByte(msg[pos+1:], c)
		if j < 0 {
			return ""
		}
		pos += j + 2
	} else {
	suelta:
		for pos < len(msg) {
			c := msg[pos]
			switch {
			case c == '\\' && pos+1 < len(msg):
				pos += 2
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '=', c == '%':
				pos++
			default:
				break suelta
			}
		}
	}
	if pos == inicio {
		return ""
	}
	return msg[i:pos]
}

// cierreEjecutable exige el contrato de db8541 (enmienda 5): el comando que
// msg propone para cerrar la sesion a mano, copiado tal cual, corre en sh y en
// zsh (si esta instalado; si no, esa parte se salta) y tmux recibe
// `kill-session -t =<sesion>`, con `=<sesion>` exacto como argumento de -t.
// El tmux es un falso que anota sus argumentos, uno por linea.
func cierreEjecutable(t *testing.T, ca, msg, sesion string) {
	t.Helper()
	cmd := comandoDeCierre(msg)
	if cmd == "" {
		t.Errorf("%s: hallazgo db8541 (enmienda 5): el error propone un comando `tmux kill-session -t '=%s'` para cerrar la sesion a mano: %q", ca, sesion, msg)
		return
	}
	quiere := []string{"kill-session", "-t", "=" + sesion}
	for _, sh := range []struct{ nombre, flags string }{{"sh", ""}, {"zsh", "-f"}} {
		t.Run(sh.nombre, func(t *testing.T) {
			bin := dbBuscar(sh.nombre)
			if bin == "" {
				t.Skipf("%s: %s no esta instalado: el comando propuesto no se prueba en %s", ca, sh.nombre, sh.nombre)
			}
			dir := t.TempDir()
			args := filepath.Join(dir, "args")
			falso := "#!/bin/sh\n: > '" + args + "'\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> '" + args + "'; done\nexit 0\n"
			if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(falso), 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			argv := []string{"-c", cmd}
			if sh.flags != "" {
				argv = append([]string{sh.flags}, argv...)
			}
			c := exec.CommandContext(ctx, bin, argv...)
			var env []string
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "PATH=") {
					env = append(env, kv)
				}
			}
			c.Env = append(env, "PATH="+dir+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
			c.Dir = dir
			out, err := c.CombinedOutput()
			raw, lerr := os.ReadFile(args)
			if lerr != nil {
				t.Fatalf("%s: hallazgo db8541 (enmienda 5): el comando que propone el error, `%s`, copiado tal cual en %s, ni llega a tmux (zsh expande una palabra que empieza con `=`; el destino va entre comillas): %v\n%s", ca, cmd, sh.nombre, err, out)
			}
			got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
			if err != nil || !reflect.DeepEqual(got, quiere) {
				t.Fatalf("%s: hallazgo db8541 (enmienda 5): el comando que propone el error, `%s`, en %s le pasa a tmux %q (quiere %q), salida %v:\n%s", ca, cmd, sh.nombre, got, quiere, err, out)
			}
		})
	}
}

// Guarda del extractor (db8541): comandoDeCierre lee la palabra del destino
// como un shell, con o sin comillas y entre otras comillas o backticks, y no
// encuentra comando donde no lo hay. Sin esto, cierreEjecutable podria pasar o
// fallar por el extractor y no por el mensaje.
func TestCA347_HallazgoDb8541_ElExtractorDelComandoDeCierre(t *testing.T) {
	const s = "hoom-demo-facturas"
	for _, c := range []struct{ msg, quiere string }{
		{"cerrala con tmux kill-session -t '=" + s + "' y reintenta", "tmux kill-session -t '=" + s + "'"},
		{"cerrala con `tmux kill-session -t '=" + s + "'` antes de reintentar", "tmux kill-session -t '=" + s + "'"},
		{"cerrala con 'tmux kill-session -t =" + s + "' y reintenta", "tmux kill-session -t =" + s},
		{"cerrala con tmux kill-session -t =" + s + ".", "tmux kill-session -t =" + s},
		{"cerrala con tmux kill-session -t \"=" + s + "\"", "tmux kill-session -t \"=" + s + "\""},
		{"cerrala con tmux kill-session -t \\=" + s + " y listo", "tmux kill-session -t \\=" + s},
		{"cerre la sesion " + s + " para que el proximo intento la arme entera", ""},
		{"tmux kill-session sin destino", ""},
	} {
		if got := comandoDeCierre(c.msg); got != c.quiere {
			t.Errorf("db8541: comandoDeCierre(%q) = %q, quiere %q", c.msg, got, c.quiere)
		}
	}
}

// hf7Tmux es el tmux falso con estado h3Tmux en el que la sesion muere justo
// cuando Open le agrega el pane de vigilancia (la CLI de la IA salio al
// arrancar): split-window se registra, la sesion deja de existir y devuelve
// el error de tmux sin la sesion ("can't find session: <sesion>"). Como tmux
// de verdad, kill-session y has-session de una sesion que no vive salen con
// codigo 1 (un *exec.ExitError con ExitCode() == 1, de tmuxSinSesion en
// tmuxdoble_test.go), no con un error de Go. Ese codigo 1 no prueba que la
// sesion no este: el cliente de tmux sale igual cuando el que falla es el
// (hallazgo 9c1816). list-panes y el resto los atiende h3Tmux.
type hf7Tmux struct {
	*h3Tmux
}

func (f *hf7Tmux) atender(name string, args []string) ([]byte, error) {
	if name == "tmux" && len(args) > 0 {
		switch args[0] {
		case "has-session":
			if d := h3Destino(args, "-t"); !f.viva(d) {
				f.mu.Lock()
				f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
				f.mu.Unlock()
				return nil, tmuxSinSesion(d)
			}
		case "split-window":
			f.mu.Lock()
			f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
			delete(f.vivas, h3Sesion)
			f.mu.Unlock()
			return nil, fmt.Errorf("can't find session: %s", h3Sesion)
		case "kill-session":
			d := h3Destino(args, "-t")
			if !f.viva(d) {
				f.mu.Lock()
				f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
				f.mu.Unlock()
				return nil, tmuxSinSesion(d)
			}
		}
	}
	return f.h3Tmux.atender(name, args)
}

func (f *hf7Tmux) deps() Deps {
	d := f.h3Tmux.deps()
	run := func(dir, name string, args ...string) error {
		_, err := f.atender(name, args)
		return err
	}
	d.RunCmd, d.QuietCmd = run, run
	d.Output = func(dir, name string, args ...string) ([]byte, error) {
		return f.atender(name, args)
	}
	return d
}

// Hallazgo f7efdc (CA-347, CA-348), con el contrato nuevo. Open crea la sesion
// (new-session funciona), el split-window falla y el cierre (`tmux
// kill-session -t =<sesion>`) TAMBIEN falla:
//
//   - la sesion murio: el split-window falla porque la sesion ya no esta, y
//     kill-session y has-session salen con codigo 1, como tmux de verdad
//     ("can't find session").
//   - la sesion sigue viva: el split-window falla y kill-session esta roto
//     (el caso de 363285).
//
// En los dos el error cumple el mismo contrato (lo verifica
// incumplimientosDelCierreFallido): conserva el del split-window, no afirma
// que la sesion ya no existe ni que quedo abierta, dice que no se pudo saber
// si sigue abierta y muestra `tmux kill-session -t '=<sesion>'`, que corre
// tal cual en sh y en zsh (cierreEjecutable). El item no declara nada, y con
// la sesion muerta el reintento con tmux sano la crea (created: true) y la
// registra.
//
// El contrato de tres estados (A: "ya no existe" si has-session salia con
// codigo 1; B: "quedo abierta" si salia bien; C: "no se sabe" si no
// confirmaba nada) quedo REEMPLAZADO por decision de Henry tras los hallazgos
// 9c1816, 57bd29 y 03481c: tmux sale con codigo 1 tanto si la sesion no esta
// como si falla su propio cliente, asi que el codigo de salida no prueba nada.
// No es debilitar el test: el mensaje nuevo es verdadero en todos los casos,
// cosa que el de tres estados no lograba (decia "ya no existe" con la sesion
// viva).
func TestCA347_CA348_HallazgoF7efdc_SiNoPudoCerrarlaElErrorNoSabeSiLaSesionSigueAbierta(t *testing.T) {
	t.Run("la sesion murio", func(t *testing.T) {
		const ca = "f7efdc (CA-347, CA-348): la sesion murio"
		root, _ := cbProyecto(t, true)
		crudo := cbLeerItem(t, root)
		f := &hf7Tmux{h3Tmux: &h3Tmux{}}

		_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
		if f.indice("tmux new-session", 0) < 0 || f.indice("tmux split-window", 0) < 0 {
			t.Fatalf("%s: fixture: Open creo la sesion y llego a split-window:\n%s", ca, f.todas())
		}
		if f.viva(h3Sesion) {
			t.Fatalf("%s: fixture: la sesion %s murio en el split-window:\n%s", ca, h3Sesion, f.todas())
		}
		if err == nil {
			t.Fatalf("%s: con el split-window roto Open devuelve el error:\n%s", ca, f.todas())
		}
		msg := err.Error()
		if mal := incumplimientosDelCierreFallido(msg, "can't find session: "+h3Sesion, h3Sesion); len(mal) > 0 {
			t.Errorf("%s: el cierre fallo y tmux no prueba si la sesion sigue abierta; el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, f.todas())
		}
		cierreEjecutable(t, ca, msg, h3Sesion)
		if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
			t.Fatalf("CA-348: f7efdc: una sesion que no quedo armada no se declara en el item:\n%s", ahora)
		}
		h3Reintento(t, "f7efdc: la sesion murio", root, f.h3Tmux)
	})

	t.Run("la sesion sigue viva y kill-session falla", func(t *testing.T) {
		const ca = "f7efdc (CA-347, CA-348): la sesion sigue viva (363285)"
		root, _ := cbProyecto(t, true)
		crudo := cbLeerItem(t, root)
		f := &hc3Tmux{h3Tmux: &h3Tmux{fallar: "split-window"}, killRoto: true}

		_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
		if !f.viva(h3Sesion) {
			t.Fatalf("%s: fixture: con kill-session roto la sesion %s sigue viva:\n%s", ca, h3Sesion, f.todas())
		}
		if err == nil {
			t.Fatalf("%s: con el split-window roto Open devuelve el error:\n%s", ca, f.todas())
		}
		msg := err.Error()
		if mal := incumplimientosDelCierreFallido(msg, h3ErrPaso.Error(), h3Sesion); len(mal) > 0 {
			t.Errorf("%s: el cierre fallo y tmux no prueba si la sesion sigue abierta; el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, f.todas())
		}
		cierreEjecutable(t, ca, msg, h3Sesion)
		if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
			t.Fatalf("CA-348: f7efdc: una sesion que no quedo armada no se declara en el item:\n%s", ahora)
		}
	})
}
