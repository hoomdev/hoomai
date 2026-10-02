//go:build windows

package hoomfs

import "os"

// abrirDirEn opens name inside r as an os.Root: no entry of the file system
// blocks an open on Windows.
func abrirDirEn(r *os.Root, name string) (*os.Root, error) {
	return r.OpenRoot(name)
}

// DescriptoresDisponibles is always nil on Windows: abrirDirEn needs no
// descriptor reopened by name there.
func DescriptoresDisponibles() error { return nil }
