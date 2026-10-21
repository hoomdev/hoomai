// Soporte compartido de los tests de Abrir sesion cuando algo falla despues
// de crear la sesion (CA-347, CA-348), escrito desde
// .hoom/specs/acciones-desde-la-tarjeta.md. Aca no hay tests: lo usan
// hallazgos_db8541_f7efdc_test.go (hf7Tmux) y hallazgos_b59c30_e0001e_test.go
// (hb5Tmux y la matriz). Antes tmuxSinSesion vivia en
// hallazgos_b59c30_e0001e_test.go y lo usaba el doble de
// hallazgos_db8541_f7efdc_test.go (hallazgo 344b2b, de legibilidad): lo
// compartido vive aca.
//
// Son las respuestas de tmux de verdad que dan los dobles: tmuxSinSesion (la
// sesion no esta) y tmuxClienteRoto (falla el propio cliente de tmux). Las dos
// salen con codigo 1: por eso el codigo de salida no prueba nada (9c1816). El
// contrato del error de Open cuando algo falla despues de crear la sesion lo
// verifica incumplimientosEnmienda6 (hallazgos_c1b_bde7e4_test.go): con la
// ENMIENDA 6 (hallazgo bde7e4) Open ya no corre ningun cierre.
package cockpitcmd

import "os/exec"

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
