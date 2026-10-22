//go:build windows

package hoomfs

import (
	"errors"
	"syscall"
)

// errorInvalidFunction is what CreateHardLinkW answers on exFAT and FAT:
// the volume cannot make the link. ERROR_NOT_SUPPORTED (50) is already
// errors.ErrUnsupported, and syscall.EPERM is an invented value here,
// never returned by the system (finding e45e0d).
const errorInvalidFunction syscall.Errno = 1

func sinEnlacesSO(err error) bool {
	return errors.Is(err, errorInvalidFunction)
}
