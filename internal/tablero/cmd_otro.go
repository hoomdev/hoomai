//go:build !windows

package tablero

import "os/exec"

// lineaCmd is cmd running linea, for an Entorno that says windows on
// another system: there is no cmd to run there, and the error says so.
func lineaCmd(linea string) *exec.Cmd {
	return exec.Command("cmd", "/S", "/C", linea) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- solo fuera de Windows con un GOOS falso: no hay cmd que correr
}
