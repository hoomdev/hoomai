//go:build windows

package hoomfs

import (
	"io/fs"
	"os"
	"syscall"
)

// sinBloquear is nothing on Windows: no entry of the file system blocks an
// open there.
const sinBloquear = 0

// abrirSinSeguir opens path read-only on the entry itself: with
// FILE_FLAG_OPEN_REPARSE_POINT a symlink or a junction is opened as what it
// is, not as what it points to, and Stat on the handle says so. No entry of
// the file system blocks an open on Windows.
func abrirSinSeguir(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// enlaces is not looked at on Windows (finding 31f78c): the FileInfo of a
// handle does not carry the count of hard links, so a .hoom/.gitignore with
// more than one is written there like any other.
func enlaces(fs.FileInfo) (uint64, bool) {
	return 0, false
}
