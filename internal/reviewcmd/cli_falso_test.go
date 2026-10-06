// El fixture unico de los CLIs de IA falsos de este paquete, y sus pruebas
// (hallazgo 20261005T155443_dd5118, reliability; y los de su review:
// 20261005T182810_eb7cde, 20261005T183300_d88b8d y 20261005T183447_b56921).
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
// Por eso todo CLI de IA falso del paquete se arma con cliFalso, que envuelve
// el cuerpo en un script que lee su stdin entero haga lo que haga el cuerpo:
// fakeProvider, cbFake y raInstalar pasan por ahi. Los demas scripts del
// paquete (un git, un hoom viejo, un driver de diff) no son CLIs de IA y
// nadie les manda el pedido: estan contados en cliFalsoOtrosScripts.
package reviewcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// cliFalsoAntes y cliFalsoDespues son el script de todo CLI de IA falso, a
// los dos lados del cuerpo:
//
//	#!/bin/sh
//	exec 9<&0
//	trap 'cat <&9 >/dev/null 2>&1' EXIT
//	(
//	:
//	<cuerpo>
//	)
//	exit $?
//
// El cuerpo corre en un SUBSHELL. Lo que le haga a su shell — reemplazarlo
// por otro programa (`exec <programa>`), cambiarle o cerrarle el stdin
// (`exec </dev/null`, `exec 0<&-`), ponerle o sacarle un trap de EXIT, salir
// antes, cortarlo con `set -e` — se lo hace al subshell. El shell del script
// queda afuera con su trap, y cuando sale lee hasta EOF lo que quede del
// pedido, como un CLI real. Lo lee del descriptor 9, la copia del stdin que
// guardo antes de que corriera el cuerpo, y lo lee despues de todo lo que
// hizo el cuerpo: al que lee el pedido por su cuenta no le saca nada.
//
// `exit $?` sale con el estado del subshell, que es el del cuerpo, y `cat` no
// imprime nada: el exit y la salida del falso son los del cuerpo. El `:` esta
// porque un `( )` vacio es un error de sintaxis, y el salto de linea antes
// del `)` porque el cuerpo puede terminar sin el suyo o en un comentario, que
// se comeria el parentesis. Un error de sintaxis del cuerpo, aunque se trague
// el `)` y el `exit` (una comilla o un heredoc sin cerrar), lo encuentra el
// shell del script con el trap ya puesto.
const (
	cliFalsoAntes = "#!/bin/sh\n" +
		"exec 9<&0\n" +
		"trap 'cat <&9 >/dev/null 2>&1' EXIT\n" +
		"(\n" +
		":\n"
	cliFalsoDespues = "\n)\n" +
		"exit $?\n"
)

// cliFalso arma el script de un CLI de IA falso (claude, codex, ...) con ese
// cuerpo de sh. Es el UNICO lugar del paquete que arma uno, y lo envuelve
// (cliFalsoAntes, cliFalsoDespues) porque el pedido de la review viaja por
// stdin (CA-418): el falso consume su stdin ENTERO haga lo que haga el
// cuerpo, y ningun test depende de quien llega primero, si el sh a salir o
// hoom a escribir.
//
// No mira el cuerpo: no hay forma que rechazar. (Antes el drenaje era un trap
// en el shell del cuerpo y una expresion regular rechazaba los cuerpos que lo
// perdian; dejaba pasar los diez primeros de cliFalsoFormasHostiles:
// hallazgos eb7cde y d88b8d.) Por eso su primer parametro ya no se usa: queda
// porque asi lo llaman los tres instaladores.
//
// Lo que NO cierra (lo muestra TestHallazgo_dd5118_LoQueElCLIFalsoNoCierra):
//   - que el cuerpo mate al shell del script (`kill -9 $$`; en el subshell
//     `$$` sigue siendo el shell del script): muere sin correr nada, tampoco
//     el trap, y el pedido queda afuera. Esto no lo cierra ningun script: no
//     hay shell que atienda un SIGKILL;
//   - un cuerpo que cierra el parentesis A PROPOSITO (`)` ... `(`): el cuerpo
//     va pegado dentro del script, y lo que ponga en el medio corre en el
//     shell del script; ahi un `exec <programa>` o un `trap - EXIT` lo dejan
//     sin drenaje. (Si solo le cambia el stdin, no: el drenaje lee del
//     descriptor 9.);
//   - un falso al que nadie le cierra el stdin no sale: el drenaje lee hasta
//     EOF, como un CLI real;
//   - que maten al proceso del falso desde afuera: la senal le llega al shell
//     del script, y el cuerpo, que corre en el subshell, sigue hasta terminar.
//     Un test con un provider colgado al que hoom mata tiene que contar con
//     eso.
//
// Un test que necesite un provider que NO lee su stdin (para probar ese
// error) no lo arma con cliFalso: escribe su script a mano, con un nombre y
// un comentario que lo digan, y lo suma a cliFalsoOtrosScripts.
func cliFalso(_ testing.TB, cuerpo string) []byte {
	return []byte(cliFalsoAntes + cuerpo + cliFalsoDespues)
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

// cliFalsoShell es un shell con el que se corre el script de un CLI falso.
type cliFalsoShell struct {
	nombre string // el que lleva el subtest
	// interprete es el shell al que se le pasa el script (`<interprete>
	// <script>`). "" es correr el script solo, por su shebang: como lo corre
	// hoom.
	interprete string
}

// cliFalsoShells son los shells con los que corren las tablas de formas:
// siempre el del shebang (/bin/sh) y, cuando existe y es OTRO archivo,
// /bin/dash. En ubuntu (el CI) /bin/sh ES dash y alcanza con el primero; en
// macOS /bin/sh es bash 3.2, y sin la segunda vuelta una forma que solo
// pierde con dash recien se veria en el CI.
func cliFalsoShells(t *testing.T) []cliFalsoShell {
	t.Helper()
	shells := []cliFalsoShell{{nombre: "sh"}}
	sh, errSh := os.Stat("/bin/sh")
	dash, errDash := os.Stat("/bin/dash")
	switch {
	case errSh != nil || errDash != nil:
		t.Logf("las formas corren solo con /bin/sh: no hay con que comparar /bin/dash (%v, %v)", errSh, errDash)
	case os.SameFile(sh, dash):
		t.Log("las formas corren solo con /bin/sh: es /bin/dash")
	default:
		shells = append(shells, cliFalsoShell{nombre: "dash", interprete: "/bin/dash"})
	}
	return shells
}

// comando arma la invocacion del script de ruta con este shell.
func (s cliFalsoShell) comando(ruta string, args ...string) *exec.Cmd {
	if s.interprete == "" {
		return exec.Command(ruta, args...)
	}
	return exec.Command(s.interprete, append([]string{ruta}, args...)...)
}

// cliFalsoForma es una forma de cuerpo de un CLI de IA falso, con lo que ese
// cuerpo hace por si solo: con que sale y que imprime. Corre en un directorio
// nuevo, asi que sus rutas relativas caen ahi.
type cliFalsoForma struct {
	nombre string
	cuerpo string
	exit   int // cliFalsoExitConError: cualquiera distinto de 0
	stdout string
	// archivo, cuando no es "", es un archivo que el cuerpo escribe (relativo
	// al directorio en que corre), y contenido lo que le escribe.
	archivo   string
	contenido string
}

// cliFalsoExitConError es, como exit de una forma, "cualquiera distinto de 0".
const cliFalsoExitConError = -1

// cliFalsoCuerpoDelHallazgo es lo que escribe la forma del heredoc.
const cliFalsoCuerpoDelHallazgo = `{"id":"20261005T155443_dd5118","description":"exec y trap, aca, son texto"}` + "\n"

// cliFalsoFormasDeSalida son las salidas de un cuerpo: el fin del script, un
// `exit N` temprano, un error. Las de abajo son las que apuntan al envoltorio
// de cliFalso: lo que el cuerpo le deja pegado al `)` que lo cierra, los
// errores de sintaxis que se tragan ese `)` y lo que toca del descriptor 9.
var cliFalsoFormasDeSalida = []cliFalsoForma{
	{nombre: "cuerpo vacio", cuerpo: "", exit: 0},
	{nombre: "exit 0", cuerpo: "exit 0\n", exit: 0},
	{nombre: "exit temprano con error", cuerpo: "echo fallando\nexit 3\n", exit: 3, stdout: "fallando\n"},
	{
		nombre: "exit dentro de un if, con un $(...) antes",
		cuerpo: "n=$(cat n.txt 2>/dev/null || echo 0)\n" +
			"n=$((n+1)); echo $n > n.txt\n" +
			"if [ $n -eq 1 ]; then exit 7; fi\nexit 0\n",
		exit: 7,
	},
	{nombre: "termina sin exit y con su ultimo comando fallado", cuerpo: "false\n", exit: 1},
	{nombre: "set -e corta el script", cuerpo: "set -e\nfalse\necho no-llega\nexit 0\n", exit: 1},
	{nombre: "error de sintaxis", cuerpo: "if then\nexit 0\n", exit: cliFalsoExitConError},
	{nombre: "una redireccion que falla y el script sigue", cuerpo: "cat > /no/existe/dd5118/pedido.txt\nexit 9\n", exit: 9},
	{nombre: "un hijo que hereda el stdin y falla", cuerpo: "sh -c 'exit 4' || exit 8\nexit 0\n", exit: 8},
	{
		nombre: "escribe un hallazgo con un heredoc",
		cuerpo: "mkdir -p findings\n" +
			"cat > findings/h.json <<'EOF'\n" + cliFalsoCuerpoDelHallazgo + "EOF\nexit 0\n",
		exit:      0,
		archivo:   "findings/h.json",
		contenido: cliFalsoCuerpoDelHallazgo,
	},

	{nombre: "termina sin salto de linea", cuerpo: "echo hola\nexit 3", exit: 3, stdout: "hola\n"},
	{nombre: "termina en un comentario, sin salto de linea", cuerpo: "echo hola\n# y nada mas", exit: 0, stdout: "hola\n"},
	{nombre: "termina en una barra de continuacion", cuerpo: "echo hola \\", exit: 0, stdout: "hola\n"},
	{nombre: "error de sintaxis: una comilla sin cerrar", cuerpo: "echo 'sin cerrar\nexit 0\n", exit: cliFalsoExitConError},
	{nombre: "error de sintaxis: un heredoc sin cerrar", cuerpo: "cat <<EOF\nhola\n", exit: cliFalsoExitConError},
	{nombre: "error de sintaxis: un if sin cerrar", cuerpo: "if true; then\n  echo adentro\n", exit: cliFalsoExitConError},
	{nombre: "cierra el descriptor 9", cuerpo: "exec 9<&-\nexit 4\n", exit: 4},
	{nombre: "cierra el parentesis y le cambia el stdin al shell del script", cuerpo: ")\nexec </dev/null\n(\n:\n", exit: 0},
}

// cliFalsoFormasHostiles son los cuerpos que se quedan sin drenaje cuando el
// drenaje es solo un trap de EXIT en su mismo shell (cliFalsoSoloElTrap; lo
// prueba TestHallazgo_dd5118_ControlSinDrenajeElPedidoNoEntra): reemplazan el
// shell, le cambian o le cierran el stdin, o le tocan el trap. Los diez
// primeros son los de los hallazgos eb7cde y d88b8d, tal cual: los que la
// expresion regular de antes dejaba pasar.
var cliFalsoFormasHostiles = []cliFalsoForma{
	{nombre: "exec con una redireccion y un programa", cuerpo: "exec >/dev/null true\n", exit: 0},
	{nombre: "exec despues de otro comando en la linea", cuerpo: "echo antes; exec true\n", exit: 0, stdout: "antes\n"},
	{nombre: "trap - EXIT despues de otro comando en la linea", cuerpo: "echo antes; trap - EXIT\n", exit: 0, stdout: "antes\n"},
	{nombre: "exec despues de un &&", cuerpo: "true && exec true\n", exit: 0},
	{nombre: "exec con una asignacion delante", cuerpo: "FOO=1 exec true\n", exit: 0},
	{nombre: "exec detras de command", cuerpo: "command exec true\n", exit: 0},
	{nombre: "exec dentro de un eval", cuerpo: "eval \"exec true\"\n", exit: 0},
	{nombre: "exec que le pone otro stdin", cuerpo: "exec </dev/null\n", exit: 0},
	{nombre: "exec que cierra el descriptor 0", cuerpo: "exec 0<&-\n", exit: 0},
	{nombre: "exec que cierra el stdin", cuerpo: "exec <&-\n", exit: 0},

	{nombre: "exec de otro programa", cuerpo: "exec true\n", exit: 0},
	{nombre: "exec de un programa que imprime y sale con error", cuerpo: "exec sh -c 'echo del programa; exit 6'\n", exit: 6, stdout: "del programa\n"},
	{nombre: "se pone otro stdin, lo lee vacio y sale con error", cuerpo: "exec </dev/null\ncat\nexit 5\n", exit: 5},
	{nombre: "su propio trap de EXIT", cuerpo: "trap 'echo chau' EXIT\necho hola\nexit 3\n", exit: 3, stdout: "hola\nchau\n"},
	{nombre: "trap - EXIT", cuerpo: "trap - EXIT\n", exit: 0},
}

// cliFalsoSoloElTrap es el principio de un script que drena su stdin SOLO con
// un trap de EXIT en el shell del cuerpo: asi armaba cliFalso los falsos
// antes de los hallazgos eb7cde y d88b8d. Queda a mano y A PROPOSITO, para el
// control.
const cliFalsoSoloElTrap = "#!/bin/sh\n" +
	"trap 'cat >/dev/null 2>&1' EXIT\n"

// cliFalsoScript es un script entero, con su shebang, para los controles.
type cliFalsoScript struct {
	nombre string
	script string
}

// cliFalsoCorrida es lo que se ve de una invocacion de un CLI falso.
type cliFalsoCorrida struct {
	dir      string // el directorio, nuevo, en que corrio
	escritos int    // bytes del pedido que entraron por su stdin
	errStdin error  // el error de esa escritura (nil: entro entero)
	exit     int
	stdout   string
	stderr   string
}

// cliFalsoGuardar deja script como el ejecutable name de un directorio nuevo
// y devuelve su ruta.
//
// En un subtest paralelo se llama ANTES de t.Parallel(): hasta ahi los
// subtests corren de a uno, y cuando arrancan los procesos ya no queda ningun
// script a medio escribir. En linux, un fork que cae mientras otra goroutine
// escribe un script se lleva una copia de ese descriptor, y ejecutar el
// script mientras alguien lo tiene abierto para escribir falla con ETXTBSY
// ("text file busy").
func cliFalsoGuardar(t *testing.T, name string, script []byte) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(ruta, script, 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// cliFalsoCorrer corre el ejecutable de ruta como hoom corre al reviewer: por
// su shebang, y le escribe el pedido entero por stdin y cierra. Lo corre en
// un directorio nuevo y con reloj: si no termina en 30 s lo mata y el test
// falla.
func cliFalsoCorrer(t *testing.T, ruta string, pedido []byte, args ...string) cliFalsoCorrida {
	t.Helper()
	return cliFalsoCorrerCon(t, cliFalsoShell{}, ruta, pedido, args...)
}

// cliFalsoCorrerCon es cliFalsoCorrer con el shell sh.
func cliFalsoCorrerCon(t *testing.T, sh cliFalsoShell, ruta string, pedido []byte, args ...string) cliFalsoCorrida {
	t.Helper()
	cmd := sh.comando(ruta, args...)
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
		c := cliFalsoCorrida{dir: cmd.Dir}
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

// cliFalsoAparece espera, hasta 30 s, a que exista el archivo ruta.
func cliFalsoAparece(ruta string) bool {
	for fin := time.Now().Add(30 * time.Second); time.Now().Before(fin); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(ruta); err == nil {
			return true
		}
	}
	return false
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

// cliFalsoAfuera exige lo contrario: el pedido NO entro entero y quien lo
// escribe vio EPIPE, el "broken pipe" de hoom.
func cliFalsoAfuera(t *testing.T, caso string, c cliFalsoCorrida, pedido []byte) {
	t.Helper()
	if !errors.Is(c.errStdin, syscall.EPIPE) || c.escritos >= len(pedido) {
		t.Fatalf("hallazgo dd5118: %s: sin drenaje el pedido no entra entero y quien escribe ve EPIPE: entraron %d de %d bytes (%v)",
			caso, c.escritos, len(pedido), c.errStdin)
	}
}

// ---------------------------------------------------------------- tests

// Hallazgo dd5118 (CA-418): un CLI de IA falso armado con cliFalso consume su
// stdin entero en TODA salida — el fin del script, un `exit N` temprano, un
// error — sin que cambie lo que el script hace: el mismo exit y la misma
// salida que tendria sin el drenaje.
//
// Hallazgos eb7cde y d88b8d: tambien con los cuerpos que reemplazan su shell,
// le cambian o le cierran el stdin o le tocan el trap de EXIT
// (cliFalsoFormasHostiles), y con /bin/sh y con /bin/dash (cliFalsoShells).
func TestHallazgo_dd5118_ElCLIFalsoLeeTodoElPedidoEnCadaSalida(t *testing.T) {
	pedido := cliFalsoPedido()
	for _, sh := range cliFalsoShells(t) {
		for _, c := range slices.Concat(cliFalsoFormasDeSalida, cliFalsoFormasHostiles) {
			caso := sh.nombre + ": " + c.nombre
			t.Run(caso, func(t *testing.T) {
				ruta := cliFalsoGuardar(t, "codex", cliFalso(t, c.cuerpo))
				t.Parallel()
				got := cliFalsoCorrerCon(t, sh, ruta, pedido)
				cliFalsoEntero(t, caso, got, pedido)
				if (c.exit >= 0 && got.exit != c.exit) || (c.exit < 0 && got.exit == 0) {
					t.Fatalf("hallazgo dd5118: %s: el drenaje no cambia el exit del script: quiere %d (-1: distinto de 0), fue %d\nstderr:\n%s",
						caso, c.exit, got.exit, got.stderr)
				}
				if got.stdout != c.stdout {
					t.Fatalf("hallazgo dd5118: %s: el drenaje no agrega ni saca nada de la salida del script: quiere %q, fue %q", caso, c.stdout, got.stdout)
				}
				if c.archivo == "" {
					return
				}
				if raw, err := os.ReadFile(filepath.Join(got.dir, c.archivo)); err != nil || string(raw) != c.contenido {
					t.Fatalf("hallazgo dd5118: %s: el heredoc del script escribe lo suyo, no el pedido: %q (%v)", caso, raw, err)
				}
			})
		}
	}
}

// cliFalsoVisto es el archivo, en el directorio en que corre, donde un cuerpo
// de cliFalsoLectura deja lo que leyo.
const cliFalsoVisto = "visto.txt"

// cliFalsoLectura es un cuerpo que lee el pedido por su cuenta.
type cliFalsoLectura struct {
	nombre string
	cuerpo string // deja lo que leyo en cliFalsoVisto
	args   []string
	exit   int
	quiere string // lo que tiene que quedar en cliFalsoVisto
}

// Hallazgo dd5118 (CA-418): el drenaje no le roba el pedido al script que lo
// lee por su cuenta. Con las formas en que lo leen los tests de este paquete
// (guardarlo, guardarlo detras del argv, buscarle el comando citado con una
// tuberia), el script recibe el pedido byte a byte; y el que lee solo una
// parte se queda con esa parte, y el resto se consume igual.
//
// Hallazgos eb7cde y d88b8d: tampoco se lo roba al programa que reemplaza al
// shell del cuerpo ni al trap de EXIT del propio cuerpo.
func TestHallazgo_dd5118_ElDrenajeNoLeRobaElPedidoAlQueLoLee(t *testing.T) {
	pedido := cliFalsoPedido()
	casos := []cliFalsoLectura{
		{nombre: "lo guarda entero", cuerpo: "cat > " + cliFalsoVisto + "\nexit 0\n", exit: 0, quiere: string(pedido)},
		{nombre: "lo guarda entero y sale con error", cuerpo: "cat > " + cliFalsoVisto + "\nexit 5\n", exit: 5, quiere: string(pedido)},
		{
			nombre: "lo guarda detras de su argv",
			cuerpo: "{ echo \"argv: $@\"; echo 'stdin:'; cat; } > " + cliFalsoVisto + "\nexit 0\n",
			args:   []string{"exec", "-"},
			exit:   0,
			quiere: "argv: exec -\nstdin:\n" + string(pedido),
		},
		{
			nombre: "lo lee despues de un $(...), un subshell y una tuberia",
			cuerpo: "n=$(cat " + cliFalsoVisto + ".no-existe 2>/dev/null || echo 0)\n" +
				"( exit 4 )\n" +
				"x=$( (exit 5); echo sub )\n" +
				"{ echo a; echo b; } | sed -n 1p > /dev/null\n" +
				"cat > " + cliFalsoVisto + "\nexit 0\n",
			exit:   0,
			quiere: string(pedido),
		},
		{
			nombre: "le busca el comando citado con una tuberia",
			cuerpo: "hb=$({ printf '%s\\n' \"$@\"; cat; } | sed -n \"s/.*'\\([^']*\\)' finding add --sev.*/\\1/p\" | head -n 1)\n" +
				"echo \"$hb\" > " + cliFalsoVisto + "\nexit 0\n",
			args:   []string{"exec", "-"},
			exit:   0,
			quiere: cliFalsoHoomCitado + "\n",
		},
		{nombre: "lee solo los primeros 100 bytes", cuerpo: "head -c 100 > " + cliFalsoVisto + "\nexit 0\n", exit: 0, quiere: string(pedido[:100])},

		{nombre: "lo lee el programa que reemplaza a su shell", cuerpo: "exec cat > " + cliFalsoVisto + "\n", exit: 0, quiere: string(pedido)},
		{nombre: "lo lee su propio trap de EXIT", cuerpo: "trap 'cat > " + cliFalsoVisto + "' EXIT\nexit 5\n", exit: 5, quiere: string(pedido)},
	}
	for _, sh := range cliFalsoShells(t) {
		for _, c := range casos {
			caso := sh.nombre + ": " + c.nombre
			t.Run(caso, func(t *testing.T) {
				ruta := cliFalsoGuardar(t, "codex", cliFalso(t, c.cuerpo))
				t.Parallel()
				got := cliFalsoCorrerCon(t, sh, ruta, pedido, c.args...)
				cliFalsoEntero(t, caso, got, pedido)
				if got.exit != c.exit {
					t.Fatalf("hallazgo dd5118: %s: el script sale con %d, salio con %d\nstderr:\n%s", caso, c.exit, got.exit, got.stderr)
				}
				visto, err := os.ReadFile(filepath.Join(got.dir, cliFalsoVisto))
				if err != nil {
					t.Fatalf("hallazgo dd5118: %s: el script dejo lo que leyo: %v", caso, err)
				}
				if string(visto) != c.quiere {
					t.Fatalf("hallazgo dd5118: %s: el script ve su pedido tal cual, el drenaje no le saca nada: quiere %d bytes, vio %d\nempieza: %q",
						caso, len(c.quiere), len(visto), visto[:min(len(visto), 120)])
				}
			})
		}
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
// quien le escribe el pedido (EPIPE, el "broken pipe" de hoom), siempre.
//
// Hallazgos eb7cde y d88b8d, la forma de los dos: lo mismo le pasa al script
// que drena solo con un trap de EXIT (cliFalsoSoloElTrap) con cada cuerpo de
// cliFalsoFormasHostiles. Esos mismos cuerpos, armados con cliFalso, dejan
// entrar el pedido entero: TestHallazgo_dd5118_ElCLIFalsoLeeTodoElPedidoEnCadaSalida.
func TestHallazgo_dd5118_ControlSinDrenajeElPedidoNoEntra(t *testing.T) {
	pedido := cliFalsoPedido()
	casos := []cliFalsoScript{
		// a mano y sin drenaje A PROPOSITO: es lo que eran los falsos antes
		{nombre: "sale sin leer", script: "#!/bin/sh\nexit 0\n"},
		{nombre: "sale con error sin leer", script: "#!/bin/sh\necho fallando\nexit 3\n"},
	}
	for _, f := range cliFalsoFormasHostiles {
		casos = append(casos, cliFalsoScript{nombre: "solo con el trap de EXIT: " + f.nombre, script: cliFalsoSoloElTrap + f.cuerpo})
	}
	for _, sh := range cliFalsoShells(t) {
		for _, c := range casos {
			caso := sh.nombre + ": " + c.nombre
			t.Run(caso, func(t *testing.T) {
				ruta := cliFalsoGuardar(t, "codex", []byte(c.script))
				t.Parallel()
				cliFalsoAfuera(t, caso, cliFalsoCorrerCon(t, sh, ruta, pedido), pedido)
			})
		}
	}
}

// cliFalsoEsperaConElStdinAbierto es cuanto se le da a un CLI falso para que
// salga (mal) con su stdin todavia abierto.
const cliFalsoEsperaConElStdinAbierto = 200 * time.Millisecond

// cliFalsoCuerpoQueSigue avisa que ya corre (deja `corre`), espera a que le
// dejen `seguir` y recien entonces deja `siguio`. Espera hasta 30 s: si el
// test se cae antes, no queda girando.
const cliFalsoCuerpoQueSigue = ": > corre\n" +
	"i=0\n" +
	"while [ ! -e seguir ] && [ $i -lt 3000 ]; do sleep 0.01; i=$((i+1)); done\n" +
	"[ -e seguir ] && : > siguio\n"

// Hallazgos eb7cde y d88b8d (CA-418): lo que el comentario de cliFalso dice
// que cliFalso NO cierra, visto. Un cuerpo que mata al shell del script o que
// se sale del parentesis para reemplazarlo deja el pedido afuera (EPIPE); un
// falso al que nadie le cierra el stdin no sale hasta que se lo cierran; y el
// cuerpo de un falso al que matan desde afuera sigue corriendo. Si alguno de
// estos casos falla, el limite se movio: hay que corregir ese comentario.
func TestHallazgo_dd5118_LoQueElCLIFalsoNoCierra(t *testing.T) {
	pedido := cliFalsoPedido()
	casos := []cliFalsoScript{
		{nombre: "el cuerpo mata al shell del script", script: string(cliFalso(t, "kill -9 $$\n"))},
		{nombre: "el cuerpo cierra el parentesis y reemplaza el shell del script", script: string(cliFalso(t, ")\nexec true\n(\n:\n"))},
	}
	for _, sh := range cliFalsoShells(t) {
		for _, c := range casos {
			caso := sh.nombre + ": " + c.nombre
			t.Run(caso, func(t *testing.T) {
				ruta := cliFalsoGuardar(t, "codex", []byte(c.script))
				t.Parallel()
				cliFalsoAfuera(t, caso, cliFalsoCorrerCon(t, sh, ruta, pedido), pedido)
			})
		}

		caso := sh.nombre + ": nadie le cierra el stdin"
		t.Run(caso, func(t *testing.T) {
			ruta := cliFalsoGuardar(t, "codex", cliFalso(t, "exit 0\n"))
			t.Parallel()
			cmd := sh.comando(ruta)
			cmd.Dir = t.TempDir()
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatalf("fixture: no pude correr %s: %v", ruta, err)
			}
			salio := make(chan error, 1)
			go func() { salio <- cmd.Wait() }()
			select {
			case err := <-salio:
				t.Fatalf("hallazgo dd5118: %s: el CLI falso lee su stdin hasta EOF y salio (%v) con el stdin todavia abierto", caso, err)
			case <-time.After(cliFalsoEsperaConElStdinAbierto):
			}
			_ = in.Close()
			select {
			case err := <-salio:
				if err != nil {
					t.Fatalf("hallazgo dd5118: %s: con el stdin cerrado el CLI falso sale con el exit de su cuerpo (0): %v", caso, err)
				}
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("fixture: %s no termino en 30 s con el stdin cerrado", ruta)
			}
		})

		caso = sh.nombre + ": lo matan desde afuera"
		t.Run(caso, func(t *testing.T) {
			ruta := cliFalsoGuardar(t, "codex", cliFalso(t, cliFalsoCuerpoQueSigue))
			t.Parallel()
			cmd := sh.comando(ruta)
			cmd.Dir = t.TempDir()
			if err := cmd.Start(); err != nil {
				t.Fatalf("fixture: no pude correr %s: %v", ruta, err)
			}
			corre := cliFalsoAparece(filepath.Join(cmd.Dir, "corre"))
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			if !corre {
				t.Fatalf("fixture: el cuerpo de %s no arranco en 30 s", ruta)
			}
			// el shell del script ya no esta: recien ahora el cuerpo puede seguir
			if err := os.WriteFile(filepath.Join(cmd.Dir, "seguir"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if !cliFalsoAparece(filepath.Join(cmd.Dir, "siguio")) {
				t.Fatalf("hallazgo dd5118: %s: el cuerpo corre en un subshell y sigue cuando matan al shell del script: no dejo `siguio`", caso)
			}
		})
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
