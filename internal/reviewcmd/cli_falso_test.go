// El fixture unico de los CLIs de IA falsos de este paquete, y sus pruebas
// (hallazgo 20261005T155443_dd5118, reliability).
//
// `hoom review` le manda el pedido al reviewer por STDIN (CA-418, enmienda 4
// de review-aislada-y-modelo-elegido), y un provider que sale sin que el
// pedido haya entrado entero es un run con error ("el provider no leyo el
// pedido entero por stdin ...: broken pipe"): la review queda NO ENTREGABLE.
// Un CLI real lee su stdin hasta EOF. Un falso que sale sin leerlo deja el
// resultado del test en manos del scheduler: si su sh sale antes de que hoom
// escriba, el pipe ya no tiene lector y el test falla; si hoom escribe antes,
// el pedido (chico) queda en el buffer del pipe y el test pasa. En linux
// perdia cada tanto, con un test distinto cada vez (el CI rojo desde el
// 2026-09-29); en macOS no perdia nunca.
//
// Por eso todo CLI de IA falso del paquete se arma con cliFalso, que le pone
// el drenaje: fakeProvider, cbFake y raInstalar pasan por ahi. Los demas
// scripts del paquete (un git, un hoom viejo, un driver de diff) no son CLIs
// de IA y nadie les manda el pedido: estan contados en cliFalsoOtrosScripts.
package reviewcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// cliFalsoDrenaje es la primera linea del cuerpo de todo CLI de IA falso:
// cuando el shell sale, por la salida que sea (el fin del script, un `exit N`
// temprano, un error del propio shell), lee lo que quede de su stdin hasta
// EOF, como un CLI real.
//
// Corre AL SALIR, despues de todo lo que hizo el script: al que lee el pedido
// por su cuenta no le saca nada (encuentra el stdin ya en EOF). El shell
// guarda su estado de salida antes del trap y `cat` no imprime nada, asi que
// el exit y la salida del script quedan como estaban. Un subshell o un
// `$(...)` no lo disparan: el trap EXIT es solo del shell del script.
const cliFalsoDrenaje = "trap 'cat >/dev/null 2>&1' EXIT\n"

// cliFalsoPierde reconoce, a simple vista (al principio de una linea), las
// dos cosas que dejarian a un cuerpo sin drenaje: reemplazar el shell por
// otro programa (`exec <programa>`: ya no queda shell que corra el trap; un
// `exec` de solo redirecciones no molesta) y un `trap` propio, que puede
// pisar el de EXIT.
var cliFalsoPierde = regexp.MustCompile(`(?m)^[ \t]*(?:exec[ \t]+[^ \t<>0-9]|trap[ \t]).*$`)

// cliFalsoSinDrenaje devuelve la linea del cuerpo que lo dejaria sin drenaje
// ("" si no hay ninguna).
func cliFalsoSinDrenaje(cuerpo string) string {
	return strings.TrimSpace(cliFalsoPierde.FindString(cuerpo))
}

// cliFalso arma el script de un CLI de IA falso (claude, codex, ...) con ese
// cuerpo de sh. Es el UNICO lugar del paquete que arma uno, y le pone el
// drenaje (cliFalsoDrenaje) porque el pedido de la review viaja por stdin
// (CA-418): el falso consume su stdin ENTERO en toda salida, y ningun test
// depende de quien llega primero, si el sh a salir o hoom a escribir.
//
// Un test que necesite un provider que NO lee su stdin (para probar ese
// error) no lo arma con cliFalso: escribe su script a mano, con un nombre y
// un comentario que lo digan, y lo suma a cliFalsoOtrosScripts.
func cliFalso(t testing.TB, cuerpo string) []byte {
	t.Helper()
	if l := cliFalsoSinDrenaje(cuerpo); l != "" {
		t.Fatalf("fixture: el cuerpo de un CLI de IA falso no puede traer %q: lo deja sin el drenaje de stdin (hallazgo dd5118)", l)
	}
	return []byte("#!/bin/sh\n" + cliFalsoDrenaje + cuerpo)
}

// cliFalsoOtrosScripts cuenta, por archivo, los scripts con shebang de los
// tests de este paquete que NO son CLIs de IA: ninguno recibe el pedido de la
// review por stdin, asi que no llevan drenaje.
var cliFalsoOtrosScripts = map[string]int{
	"base_de_la_review_ronda1_test.go":      1, // bdGitQueMueveLaRef: un git
	"binario_de_la_review_test.go":          2, // el hoom viejo al frente del PATH (dos tests): nadie lo corre con el pedido
	"review_aislada_evidencia_test.go":      1, // raGitRoto: un git
	"review_aislada_memoria_test.go":        2, // un driver de diff de git y un git
	"review_aislada_opciones_test.go":       1, // raGitSinHoomYaml: un git
	"review_por_diferencia_helpers_test.go": 1, // rdGitEspia: un git
	"review_por_diferencia_ronda1_test.go":  1, // rdGitQueMueveHEAD: un git
	"review_por_diferencia_ronda2_test.go":  1, // rd2GitIdaYVuelta: un git
}

// cliFalsoEsteArchivo es este archivo: el unico que puede escribir el shebang
// de un CLI de IA.
const cliFalsoEsteArchivo = "cli_falso_test.go"

// ---------------------------------------------------------------- fixtures

// cliFalsoHoomCitado es el hoom que cita la ultima linea de cliFalsoPedido.
const cliFalsoHoomCitado = "/opt/hoom de prueba/bin/hoom"

// cliFalsoPedido arma un pedido de mas de 2 MiB: no entra en el buffer de
// ningun pipe (64 KiB en linux y en macOS; 1 MiB es lo mas que puede pedir en
// linux un proceso sin privilegios), asi que quien lo escribe solo termina
// bien si del otro lado lo leen ENTERO. La ultima linea trae el comando de
// registro citado, como el pedido de verdad: quien lo encuentra leyo todo.
func cliFalsoPedido() []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < 2<<20; i++ {
		fmt.Fprintf(&b, "linea %06d del pedido de la review: evidencia congelada por hoom\n", i)
	}
	b.WriteString("Registra cada hallazgo con: '" + cliFalsoHoomCitado + "' finding add --sev low|medium|high --lens risk --file <ruta> --author reviewer@codex \"<descripcion>\"\n")
	return b.Bytes()
}

// cliFalsoCorrida es lo que se ve de una invocacion de un CLI falso.
type cliFalsoCorrida struct {
	escritos int   // bytes del pedido que entraron por su stdin
	errStdin error // el error de esa escritura (nil: entro entero)
	exit     int
	stdout   string
	stderr   string
}

// cliFalsoGuardar deja script como el ejecutable name de un directorio nuevo
// y devuelve su ruta.
func cliFalsoGuardar(t *testing.T, name string, script []byte) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(ruta, script, 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// cliFalsoCorrer corre el ejecutable de ruta como hoom corre al reviewer: le
// escribe el pedido entero por stdin y cierra. Lo corre en un directorio
// nuevo y con reloj: si no termina en 30 s lo mata y el test falla.
func cliFalsoCorrer(t *testing.T, ruta string, pedido []byte, args ...string) cliFalsoCorrida {
	t.Helper()
	cmd := exec.Command(ruta, args...)
	cmd.Dir = t.TempDir()
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("fixture: no pude correr %s: %v", ruta, err)
	}
	ch := make(chan cliFalsoCorrida, 1)
	go func() {
		var c cliFalsoCorrida
		c.escritos, c.errStdin = in.Write(pedido)
		_ = in.Close()
		_ = cmd.Wait()
		c.exit, c.stdout, c.stderr = cmd.ProcessState.ExitCode(), o.String(), e.String()
		ch <- c
	}()
	select {
	case c := <-ch:
		return c
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("fixture: %s no termino en 30 s con el pedido por stdin", ruta)
	}
	return cliFalsoCorrida{}
}

// cliFalsoEntero exige que el pedido haya entrado entero por el stdin del
// CLI: quien lo escribe (hoom) no ve ni un error ni una escritura corta.
func cliFalsoEntero(t *testing.T, caso string, c cliFalsoCorrida, pedido []byte) {
	t.Helper()
	if c.errStdin != nil || c.escritos != len(pedido) {
		t.Fatalf("CA-418: hallazgo dd5118: %s: el CLI falso lee su stdin entero antes de salir: entraron %d de %d bytes (%v)\nstderr:\n%s",
			caso, c.escritos, len(pedido), c.errStdin, c.stderr)
	}
}

// ---------------------------------------------------------------- tests

// Hallazgo dd5118 (CA-418): un CLI de IA falso armado con cliFalso consume su
// stdin entero en TODA salida — el fin del script, un `exit N` temprano, un
// error — sin que cambie lo que el script hace: el mismo exit y la misma
// salida que tendria sin el drenaje.
func TestHallazgo_dd5118_ElCLIFalsoLeeTodoElPedidoEnCadaSalida(t *testing.T) {
	pedido := cliFalsoPedido()
	fuera := t.TempDir()
	contador := filepath.Join(fuera, "n.txt")
	hallazgo := filepath.Join(fuera, "findings", "h.json")
	const cuerpoDelHallazgo = `{"id":"20261005T155443_dd5118","description":"exec y trap, aca, son texto"}` + "\n"
	for _, c := range []struct {
		nombre, cuerpo string
		exit           int // -1: cualquiera distinto de 0
		stdout         string
	}{
		{"cuerpo vacio", "", 0, ""},
		{"exit 0", "exit 0\n", 0, ""},
		{"exit temprano con error", "echo fallando\nexit 3\n", 3, "fallando\n"},
		{"exit dentro de un if, con un $(...) antes", "n=$(cat '" + contador + "' 2>/dev/null || echo 0)\n" +
			"n=$((n+1)); echo $n > '" + contador + "'\n" +
			"if [ $n -eq 1 ]; then exit 7; fi\nexit 0\n", 7, ""},
		{"termina sin exit y con su ultimo comando fallado", "false\n", 1, ""},
		{"set -e corta el script", "set -e\nfalse\necho no-llega\nexit 0\n", 1, ""},
		{"error de sintaxis", "if then\nexit 0\n", -1, ""},
		{"una redireccion que falla y el script sigue", "cat > /no/existe/dd5118/pedido.txt\nexit 9\n", 9, ""},
		{"un hijo que hereda el stdin y falla", "sh -c 'exit 4' || exit 8\nexit 0\n", 8, ""},
		{"escribe un hallazgo con un heredoc", "mkdir -p '" + filepath.Dir(hallazgo) + "'\n" +
			"cat > '" + hallazgo + "' <<'EOF'\n" + cuerpoDelHallazgo + "EOF\nexit 0\n", 0, ""},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			got := cliFalsoCorrer(t, cliFalsoGuardar(t, "codex", cliFalso(t, c.cuerpo)), pedido)
			cliFalsoEntero(t, c.nombre, got, pedido)
			if (c.exit >= 0 && got.exit != c.exit) || (c.exit < 0 && got.exit == 0) {
				t.Fatalf("hallazgo dd5118: %s: el drenaje no cambia el exit del script: quiere %d (-1: distinto de 0), fue %d\nstderr:\n%s",
					c.nombre, c.exit, got.exit, got.stderr)
			}
			if got.stdout != c.stdout {
				t.Fatalf("hallazgo dd5118: %s: el drenaje no agrega ni saca nada de la salida del script: quiere %q, fue %q", c.nombre, c.stdout, got.stdout)
			}
		})
	}
	if raw, err := os.ReadFile(hallazgo); err != nil || string(raw) != cuerpoDelHallazgo {
		t.Fatalf("hallazgo dd5118: el heredoc del script escribe lo suyo, no el pedido: %q (%v)", raw, err)
	}
}

// Hallazgo dd5118 (CA-418): el drenaje no le roba el pedido al script que lo
// lee por su cuenta. Con las formas en que lo leen los tests de este paquete
// (guardarlo, guardarlo detras del argv, buscarle el comando citado con una
// tuberia), el script recibe el pedido byte a byte; y el que lee solo una
// parte se queda con esa parte, y el resto se consume igual.
func TestHallazgo_dd5118_ElDrenajeNoLeRobaElPedidoAlQueLoLee(t *testing.T) {
	pedido := cliFalsoPedido()
	for _, c := range []struct {
		nombre string
		cuerpo func(f string) string
		args   []string
		exit   int
		quiere string
	}{
		{"lo guarda entero", func(f string) string { return "cat > '" + f + "'\nexit 0\n" },
			nil, 0, string(pedido)},
		{"lo guarda entero y sale con error", func(f string) string { return "cat > '" + f + "'\nexit 5\n" },
			nil, 5, string(pedido)},
		{"lo guarda detras de su argv", func(f string) string {
			return "{ echo \"argv: $@\"; echo 'stdin:'; cat; } > '" + f + "'\nexit 0\n"
		}, []string{"exec", "-"}, 0, "argv: exec -\nstdin:\n" + string(pedido)},
		{"lo lee despues de un $(...), un subshell y una tuberia", func(f string) string {
			return "n=$(cat '" + f + ".no-existe' 2>/dev/null || echo 0)\n" +
				"( exit 4 )\n" +
				"x=$( (exit 5); echo sub )\n" +
				"{ echo a; echo b; } | sed -n 1p > /dev/null\n" +
				"cat > '" + f + "'\nexit 0\n"
		}, nil, 0, string(pedido)},
		{"le busca el comando citado con una tuberia", func(f string) string {
			return "hb=$({ printf '%s\\n' \"$@\"; cat; } | sed -n \"s/.*'\\([^']*\\)' finding add --sev.*/\\1/p\" | head -n 1)\n" +
				"echo \"$hb\" > '" + f + "'\nexit 0\n"
		}, []string{"exec", "-"}, 0, cliFalsoHoomCitado + "\n"},
		{"lee solo los primeros 100 bytes", func(f string) string { return "head -c 100 > '" + f + "'\nexit 0\n" },
			nil, 0, string(pedido[:100])},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "visto.txt")
			got := cliFalsoCorrer(t, cliFalsoGuardar(t, "codex", cliFalso(t, c.cuerpo(f))), pedido, c.args...)
			cliFalsoEntero(t, c.nombre, got, pedido)
			if got.exit != c.exit {
				t.Fatalf("hallazgo dd5118: %s: el script sale con %d, salio con %d\nstderr:\n%s", c.nombre, c.exit, got.exit, got.stderr)
			}
			visto, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("hallazgo dd5118: %s: el script dejo lo que leyo: %v", c.nombre, err)
			}
			if string(visto) != c.quiere {
				t.Fatalf("hallazgo dd5118: %s: el script ve su pedido tal cual, el drenaje no le saca nada: quiere %d bytes, vio %d\nempieza: %q",
					c.nombre, len(c.quiere), len(visto), visto[:min(len(visto), 120)])
			}
		})
	}
}

// Hallazgo dd5118 (CA-418): los tres instaladores de CLIs de IA falsos del
// paquete (fakeProvider, cbFake, raInstalar) arman CLIs que consumen el
// pedido entero, y raInstalar sigue guardando su stdin y su argv tal cual.
func TestHallazgo_dd5118_LosInstaladoresArmanCLIsQueLeenTodoElPedido(t *testing.T) {
	pedido := cliFalsoPedido()

	// fakeProvider deja el falso en el primer directorio del PATH
	fakeProvider(t, "codex", "echo fallando\nexit 3\n")
	ruta := filepath.Join(strings.SplitN(os.Getenv("PATH"), ":", 2)[0], "codex")
	if _, err := os.Stat(ruta); err != nil {
		t.Fatalf("fixture: fakeProvider deja su codex al frente del PATH: %v", err)
	}
	got := cliFalsoCorrer(t, ruta, pedido)
	cliFalsoEntero(t, "fakeProvider", got, pedido)
	if got.exit != 3 || got.stdout != "fallando\n" {
		t.Fatalf("hallazgo dd5118: fakeProvider: el script hace lo suyo (fallando, exit 3): %+v", got)
	}

	bin := cbPathAislado(t)
	cbFake(t, bin, "claude", "exit 4\n")
	got = cliFalsoCorrer(t, filepath.Join(bin, "claude"), pedido)
	cliFalsoEntero(t, "cbFake", got, pedido)
	if got.exit != 4 || got.stdout != "" {
		t.Fatalf("hallazgo dd5118: cbFake: el script hace lo suyo (exit 4, sin salida): %+v", got)
	}

	bin = raPATH(t)
	cx := raInstalar(t, bin, "codex", "exit 5\n")
	got = cliFalsoCorrer(t, filepath.Join(bin, "codex"), pedido, "exec", "-")
	cliFalsoEntero(t, "raInstalar", got, pedido)
	if got.exit != 5 || cx.veces() != 1 {
		t.Fatalf("hallazgo dd5118: raInstalar: una invocacion, y extra corre antes del exit 0 (exit 5): veces %d, %+v", cx.veces(), got)
	}
	if visto := cx.stdin(t, 1); visto != string(pedido) {
		t.Fatalf("hallazgo dd5118: raInstalar guarda el pedido byte a byte: quiere %d bytes, guardo %d", len(pedido), len(visto))
	}
	if args := cx.argv(t, 1); len(args) != 2 || args[0] != "exec" || args[1] != "-" {
		t.Fatalf("hallazgo dd5118: raInstalar guarda su argv tal cual: %q", args)
	}
}

// Hallazgo dd5118 (CA-418), CONTROL de los tests de arriba, y la forma del
// hallazgo: un script que sale sin leer y SIN el drenaje le rompe el pipe a
// quien le escribe el pedido (EPIPE, el "broken pipe" de hoom), siempre. Lo
// mismo le pasa a uno que reemplaza el shell con exec: por eso cliFalso no
// acepta ese cuerpo.
func TestHallazgo_dd5118_ControlSinDrenajeElPedidoNoEntra(t *testing.T) {
	pedido := cliFalsoPedido()
	for _, c := range []struct{ nombre, script string }{
		// a mano y sin drenaje A PROPOSITO: es lo que eran los falsos antes
		{"sale sin leer", "#!/bin/sh\nexit 0\n"},
		{"sale con error sin leer", "#!/bin/sh\necho fallando\nexit 3\n"},
		{"con el drenaje pero con exec de otro programa", "#!/bin/sh\n" + cliFalsoDrenaje + "exec true\n"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			got := cliFalsoCorrer(t, cliFalsoGuardar(t, "codex", []byte(c.script)), pedido)
			if !errors.Is(got.errStdin, syscall.EPIPE) || got.escritos >= len(pedido) {
				t.Fatalf("hallazgo dd5118: %s: sin drenaje el pedido no entra entero y quien escribe ve EPIPE: entraron %d de %d bytes (%v)",
					c.nombre, got.escritos, len(pedido), got.errStdin)
			}
		})
	}
}

// Hallazgo dd5118 (CA-418): cliFalso no acepta el cuerpo que se quedaria sin
// drenaje (exec de otro programa, un trap propio) y no se confunde con lo que
// no lo es: un exec de solo redirecciones, las palabras en un texto, los
// cuerpos de los fixtures del paquete.
func TestHallazgo_dd5118_CliFalsoReconoceElCuerpoQueSeQuedaSinDrenaje(t *testing.T) {
	for _, cuerpo := range []string{
		"exec '/usr/bin/git' \"$@\"\n",
		"echo antes\n\texec true\n",
		"if [ -n \"$1\" ]; then\n  exec \"$real\" \"$@\"\nfi\nexit 0\n",
		"trap 'echo chau' EXIT\nexit 0\n",
		"trap - EXIT\n",
		"trap '' TERM\n",
	} {
		if cliFalsoSinDrenaje(cuerpo) == "" {
			t.Errorf("hallazgo dd5118: este cuerpo se queda sin el drenaje y cliFalso no lo ve:\n%s", cuerpo)
		}
	}
	for _, cuerpo := range []string{
		"",
		"exit 0\n",
		"echo fallando\nexit 3\n",
		"exec 3>&1\nexec > /dev/null\nexec 2>&1\nexit 0\n",
		"echo 'un exec y un trap en un texto'\nprintf 'trap %s\\n' x\nexit 0\n",
		hallazgoFalso("/proyecto", "20260904T210000_abcdef", "reviewer@codex") + "exit 0\n",
		cbFoto("/proyecto", "/fotos"),
		raUsoCodex + raUsoPares,
	} {
		if l := cliFalsoSinDrenaje(cuerpo); l != "" {
			t.Errorf("hallazgo dd5118: este cuerpo conserva el drenaje y cliFalso lo rechaza por %q:\n%s", l, cuerpo)
		}
	}
}

// Hallazgo dd5118 (CA-418): ningun test del paquete arma a mano el script de
// un CLI de IA. Todo shebang fuera de este archivo es uno de los scripts que
// no son CLIs de IA, contados en cliFalsoOtrosScripts: un script nuevo o se
// arma con cliFalso o se suma ahi con su motivo.
func TestHallazgo_dd5118_NingunCLIDeIAFalsoSeArmaAMano(t *testing.T) {
	archivos, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	vistos := map[string]int{}
	esta := false
	for _, a := range archivos {
		if a == cliFalsoEsteArchivo {
			esta = true
			continue
		}
		raw, err := os.ReadFile(a)
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(raw), "#!/"); n != 0 {
			vistos[a] = n
		}
	}
	if !esta {
		t.Skipf("los fuentes de los tests no estan en el directorio de trabajo: no se puede mirar quien arma scripts (falta %s)", cliFalsoEsteArchivo)
	}
	for a, n := range vistos {
		if n != cliFalsoOtrosScripts[a] {
			t.Errorf("hallazgo dd5118: %s arma %d scripts con shebang y cliFalsoOtrosScripts le cuenta %d: "+
				"un CLI de IA falso se arma con cliFalso (el pedido de la review viaja por stdin y el falso tiene que leerlo entero); "+
				"si el script nuevo no es un CLI de IA, sumalo a cliFalsoOtrosScripts con su motivo", a, n, cliFalsoOtrosScripts[a])
		}
	}
	for a, n := range cliFalsoOtrosScripts {
		if _, hay := vistos[a]; !hay {
			t.Errorf("hallazgo dd5118: cliFalsoOtrosScripts cuenta %d scripts en %s y ya no arma ninguno: la cuenta quedo vieja", n, a)
		}
	}
}
