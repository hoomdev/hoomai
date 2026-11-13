//go:build !linux

package encierro

import (
	"os/exec"
	"time"

	"github.com/hoomdev/hoomai/internal/procesos"
)

// sondear outside Linux is the zero Sonda: there is no bwrap nor Landlock.
func sondear() Sonda { return Sonda{} }

// envuelto outside Linux is never made: Envolver leaves the command as it is.
type envuelto struct{}

func (*envuelto) terminar()              {}
func (*envuelto) cerrar()                {}
func (*envuelto) esperarFin(_ time.Time) {}

// envolver outside Linux: there is no bwrap to wrap the command with.
func envolver(*exec.Cmd, Plan) error { return errSinBwrap }

// arrancarEnvuelto is never reached outside Linux (nothing is envuelto); it
// starts cmd as procesos does.
func arrancarEnvuelto(cmd *exec.Cmd, gracia time.Duration, _ *envuelto, alCancelar func()) error {
	return procesos.ArrancarCon(cmd, gracia, procesos.Terminacion{AlCancelar: alCancelar})
}
