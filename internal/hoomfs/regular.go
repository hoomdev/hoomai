package hoomfs

import (
	"fmt"
	"io/fs"
	"os"
)

// NoRegular is AbrirRegular's error for an entry that is not a regular file:
// Mode says what it is.
type NoRegular struct {
	Path string
	Mode fs.FileMode
}

func (e *NoRegular) Error() string {
	return fmt.Sprintf("%s no es un archivo regular (%s)", e.Path, e.Mode.Type())
}

// AbrirRegular opens path for reading only if it is a regular file, and it
// decides on the descriptor it returns, never on an earlier look at the
// path: between a look and an open anything can swap the entry (a process
// the run left behind, say), and on one descriptor nothing can. A symlink is
// not followed and a FIFO does not block the open; either one, or anything
// else that is not a regular file, comes back as *NoRegular or as the
// error of the open.
func AbrirRegular(path string) (*os.File, error) {
	f, err := abrirSinSeguir(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, &NoRegular{Path: path, Mode: st.Mode()}
	}
	return f, nil
}
