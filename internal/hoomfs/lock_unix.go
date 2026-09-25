//go:build !windows

package hoomfs

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes f's exclusive flock without waiting: false when another open
// description of the file holds it.
func tryLock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}
