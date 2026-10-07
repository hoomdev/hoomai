// La espera sin recolectar de grupo_de_procesos_test.go, donde no la hay.

//go:build !(linux || darwin || dragonfly || freebsd || netbsd || openbsd)

package reviewcmd

import (
	"errors"
	"runtime"
)

// grupoEsperarSinRecolectar no sabe esperar sin recolectar en este sistema, y
// lo dice: el que la llama falla, que es mejor que seguir sin haber esperado.
func grupoEsperarSinRecolectar(int) error {
	return errors.New("no se como esperar a un proceso sin recolectarlo en " + runtime.GOOS)
}
