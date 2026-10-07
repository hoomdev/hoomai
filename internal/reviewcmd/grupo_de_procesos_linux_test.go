// La espera sin recolectar de grupo_de_procesos_test.go, en linux.

//go:build linux

package reviewcmd

import (
	"syscall"
	"unsafe"
)

// grupoEsperarSinRecolectar espera a que termine el proceso pid, un hijo de
// este proceso todavia sin recolectar, y lo deja como esta: sin recolectar.
// Es waitid con WNOWAIT, lo que hace la biblioteca estandar antes del Wait de
// un proceso (os/wait_waitid.go), y como ahi una interrupcion (EINTR) es
// volver a esperar. Cualquier otro error es un error: no "ya termino".
func grupoEsperarSinRecolectar(pid int) error {
	const pPID = 1           // P_PID: esperar a ese proceso
	var info [128 / 8]uint64 // un siginfo_t, que aca no se mira: 128 bytes en todo linux
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, pPID, uintptr(pid),
			uintptr(unsafe.Pointer(&info[0])), syscall.WEXITED|syscall.WNOWAIT, 0, 0)
		switch errno {
		case 0:
			return nil
		case syscall.EINTR:
		default:
			return errno
		}
	}
}
