package hoomfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
)

// AbrirRuta opens the directory root/partes... for reading, walking from
// root one component at a time with descriptors (AbrirDirEn): no component
// may be a symlink nor anything but a directory, and each one opened is the
// one looked at. A missing component comes back as fs.ErrNotExist, so the
// caller can tell "not there yet" from "unreadable". It is how hoom reads
// its own telemetry (.hoom/runs, .hoom/envelopes): a .hoom or a directory
// under it that is a symlink would take the reading to another project
// (findings 305cbc, 31a00d and 3826b3).
func AbrirRuta(root string, partes ...string) (*os.Root, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	for _, p := range partes {
		fi, err := r.Lstat(p)
		if err != nil {
			r.Close()
			return nil, err
		}
		if !fi.IsDir() {
			r.Close()
			return nil, fmt.Errorf("%s no es un directorio (%s)", p, fi.Mode().Type())
		}
		sub, err := AbrirDirEn(r, p, fi)
		r.Close()
		if err != nil {
			return nil, err
		}
		r = sub
	}
	return r, nil
}

// ListarEn lists the directory r.
func ListarEn(r *os.Root) ([]fs.DirEntry, error) {
	d, err := r.Open(".")
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.ReadDir(-1)
}

// LeerRegularEn reads the whole regular file name of r, never following a
// symlink nor blocking on a FIFO (AbrirRegularEn), and at most max bytes: a
// longer one is an error, never a cut.
func LeerRegularEn(r *os.Root, name string, max int64) ([]byte, error) {
	fi, err := r.Lstat(name)
	if err != nil {
		return nil, err
	}
	f, err := AbrirRegularEn(r, name, fi)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > max {
		return nil, fmt.Errorf("%s pasa de %d bytes", name, max)
	}
	return raw, nil
}

// AnexarRegularEn opens the regular file name of r to append to it, with the
// checks of AbrirRegularEn: what gets written is the file that was looked at,
// never the target of a symlink nor a FIFO.
func AnexarRegularEn(r *os.Root, name string, antes fs.FileInfo) (*os.File, error) {
	f, err := r.OpenFile(name, os.O_WRONLY|os.O_APPEND|sinBloquear, 0)
	if err != nil {
		return nil, err
	}
	if f, err = regular(f, name, antes); err != nil {
		return nil, err
	}
	if err := sigueSiendo(r, name, antes); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// Entrada is one file of a directory walked with RecorrerDirectorio: its
// name and either its content or why it could not be read. Contenido is only
// valid during the call that receives it: whoever keeps something copies it.
type Entrada struct {
	Nombre    string
	Contenido []byte
	Err       error
}

// RecorrerDirectorio walks the directory root/partes... the way hoom reads
// its evidence and its telemetry: down by descriptors (AbrirRuta), in name
// order (a listing in directory order would make equal evidence come out in
// another order on another file system, finding b36303), and every entry
// that is not a directory and whose name passes quiere read as a regular file
// of at most max bytes, never following a symlink nor blocking on a FIFO
// (LeerRegularEn), and handed to cada ONE AT A TIME: a directory of many
// files under the cap never sits whole in memory (findings c85840, 8c7270,
// b7bb41). A directory that does not exist is no entries and no error; one
// that cannot be opened or listed is an error. A file that cannot be read
// comes with its Err: whether that is a warning or a failure is cada's to
// say.
func RecorrerDirectorio(root string, partes []string, quiere func(nombre string) bool, max int64, cada func(Entrada)) error {
	r, err := AbrirRuta(root, partes...)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer r.Close()
	entries, err := ListarEn(r)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !quiere(e.Name()) {
			continue
		}
		raw, err := LeerRegularEn(r, e.Name(), max)
		cada(Entrada{Nombre: e.Name(), Contenido: raw, Err: err})
	}
	return nil
}
