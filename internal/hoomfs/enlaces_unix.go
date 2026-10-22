//go:build !windows

package hoomfs

import (
	"errors"
	"syscall"
)

// sinEnlacesSO: Linux answers EPERM to a link on vfat and exfat; macOS
// answers ENOTSUP on exFAT and FAT, which errors.ErrUnsupported already
// names.
func sinEnlacesSO(err error) bool {
	return errors.Is(err, syscall.EPERM)
}
