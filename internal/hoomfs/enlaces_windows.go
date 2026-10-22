//go:build windows

package hoomfs

import (
	"errors"
	"syscall"
)

// Windows' own codes for a link the volume cannot make: CreateHardLinkW
// answers ERROR_INVALID_FUNCTION on exFAT and FAT, and some volumes
// ERROR_NOT_SUPPORTED. syscall.EPERM is an invented value here, never
// returned by the system (finding e45e0d).
const (
	errorInvalidFunction syscall.Errno = 1
	errorNotSupported    syscall.Errno = 50
)

func sinEnlacesSO(err error) bool {
	return errors.Is(err, errorInvalidFunction) || errors.Is(err, errorNotSupported)
}
