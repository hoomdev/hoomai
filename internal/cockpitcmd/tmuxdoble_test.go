// Soporte compartido de los tests de Abrir sesion cuando el cierre falla
// (CA-347, CA-348), escrito desde .hoom/specs/acciones-desde-la-tarjeta.md y
// el contrato de los hallazgos 9c1816, 57bd29 y 03481c (decision de Henry).
// Aca no hay tests: lo usan hallazgos_db8541_f7efdc_test.go (hf7Tmux y el test
// de f7efdc) y hallazgos_b59c30_e0001e_test.go (hb5Tmux y el test de la
// matriz). Antes tmuxSinSesion vivia en hallazgos_b59c30_e0001e_test.go y lo
// usaba el doble de hallazgos_db8541_f7efdc_test.go (hallazgo 344b2b, de
// legibilidad): lo compartido vive aca.
//
//   - Las respuestas de tmux de verdad que dan los dobles: tmuxSinSesion (la
//     sesion no esta) y tmuxClienteRoto (falla el propio cliente de tmux).
//     Las dos salen con codigo 1: por eso el codigo de salida no prueba nada
//     (9c1816).
//   - incumplimientosDelCierreFallido: en que incumple el contrato el error
//     de Open cuando creo la sesion, algo fallo despues y el cierre `tmux
//     kill-session -t =<sesion>` TAMBIEN fallo. Un solo mensaje para todos
//     los casos, este la sesion viva, muerta o sin saberse. Lo comun con
//     servecmd (y lo de la guarda de C1, con kill-session sano:
//     DiceQueLaCerro) vive en internal/cierretmuxtest (hallazgo 9accd8); aca
//     queda lo propio de Open.
package cockpitcmd

import (
	"os/exec"
	"strings"

	"github.com/hoomdev/hoomai/internal/cierretmuxtest"
)

// tmuxSinSesion es el error con el que tmux de verdad responde has-session o
// kill-session de una sesion que no existe: sale con codigo 1 y dice "can't
// find session: <sesion>" por stderr. Es el *exec.ExitError (ExitCode() == 1,
// con Stderr) de un proceso de verdad, no un error de Go armado a mano
// (b59c30, e0001e). Lo usan hb5Tmux y hf7Tmux.
func tmuxSinSesion(sesion string) error {
	_, err := exec.Command("/bin/sh", "-c", `printf "can't find session: %s\n" "$1" >&2; exit 1`, "sh", sesion).Output()
	return err
}

// tmuxClienteRotoMsg es lo que dice por stderr el cliente de tmux cuando se
// rompe su conexion con el servidor.
const tmuxClienteRotoMsg = "lost server"

// tmuxClienteRoto es el error con el que sale el propio cliente de tmux
// cuando falla (perdio el servidor): tambien codigo 1, como el de una sesion
// que no esta, aunque la sesion siga viva. Es el caso del hallazgo 9c1816: un
// *exec.ExitError con ExitCode() == 1 y la sesion VIVA. Un proceso de verdad,
// no un error de Go armado a mano.
func tmuxClienteRoto() error {
	_, err := exec.Command("/bin/sh", "-c", `printf '%s\n' "$1" >&2; exit 1`, "sh", tmuxClienteRotoMsg).Output()
	return err
}

// incumplimientosDelCierreFallido devuelve en que incumple msg el contrato
// nuevo (hallazgos 9c1816, 57bd29 y 03481c, decision de Henry): msg es el
// error de Open cuando creo la sesion sesion, algo fallo despues y el cierre
// `tmux kill-session -t =<sesion>` TAMBIEN fallo: tmux sale con codigo 1
// tanto si la sesion no esta como si falla su propio cliente, asi que nada de
// lo que responda prueba si la sesion sigue abierta. Lo comun con servecmd
// vive en internal/cierretmuxtest (hallazgo 9accd8: no dice que la cerro, no
// afirma como hecho que la sesion ya no existe ni que quedo abierta, dice que
// no se pudo saber si sigue abierta y muestra el comando de cierre con el
// destino entre comillas). Lo propio de Open:
//
//   - conserva original, el error del paso que fallo (va primero);
//   - el comando de cierre `tmux kill-session -t '=<sesion>'` se busca tal
//     cual (strings.Contains); que corra tal cual en sh y en zsh lo exige
//     aparte cierreEjecutable.
//
// No es un predicado: devuelve la lista de incumplimientos, vacia si msg
// cumple todo.
func incumplimientosDelCierreFallido(msg, original, sesion string) []string {
	var mal []string
	if !strings.Contains(msg, original) {
		mal = append(mal, "conserva el error del paso que fallo ("+original+")")
	}
	return append(mal, cierretmuxtest.IncumplimientosDelCierreFallido(msg, sesion, strings.Contains)...)
}
