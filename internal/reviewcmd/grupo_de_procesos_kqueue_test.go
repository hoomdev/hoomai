// La espera sin recolectar de grupo_de_procesos_test.go, en macOS y los BSD.

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package reviewcmd

import "syscall"

// grupoEsperarSinRecolectar espera a que termine el proceso pid, un hijo de
// este proceso todavia sin recolectar, y lo deja como esta: sin recolectar.
// Aca waitid no sirve (el de macOS vuelve tambien cuando al proceso lo
// frenan, y el paquete syscall no lo trae): le pide el aviso a kqueue
// (EVFILT_PROC con NOTE_EXIT), que llega cuando el proceso termina.
//
// A un proceso que ya termino y sigue sin recolectar kqueue no lo encuentra
// (ESRCH): eso es "ya termino", siempre que el proceso siga ahi (kill con la
// senal 0, que no manda nada, lo encuentra). Si no esta, y cualquier otro
// error, es un error.
func grupoEsperarSinRecolectar(pid int) error {
	kq, err := syscall.Kqueue()
	if err != nil {
		return err
	}
	defer syscall.Close(kq)
	var pedido, aviso [1]syscall.Kevent_t
	syscall.SetKevent(&pedido[0], pid, syscall.EVFILT_PROC, syscall.EV_ADD|syscall.EV_ONESHOT)
	pedido[0].Fflags = syscall.NOTE_EXIT
	for {
		n, err := syscall.Kevent(kq, pedido[:], aviso[:], nil)
		if n > 0 && aviso[0].Flags&syscall.EV_ERROR != 0 {
			err = syscall.Errno(aviso[0].Data) // el pedido no entro: por que
		}
		switch {
		case err == syscall.EINTR, err == nil && n == 0:
			continue
		case err == syscall.ESRCH:
			return syscall.Kill(pid, 0)
		}
		return err
	}
}
