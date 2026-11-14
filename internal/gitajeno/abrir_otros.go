//go:build !unix

package gitajeno

import "os"

// abrir opens p to read it. Outside Unix there are no FIFOs to block on, and
// the open follows what the system follows.
func abrir(p string, _ bool) (*os.File, error) {
	return os.Open(p)
}
