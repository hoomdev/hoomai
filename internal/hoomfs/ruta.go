package hoomfs

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
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
	return bajar(root, false, partes)
}

// AbrirOCrearRuta is AbrirRuta that creates the components that are not
// there yet, one at a time inside the one above: what hoom writes into its
// own directories (.hoom/envelopes) never goes through a symlink to another
// tree, nor through anything that is not a directory, whether it was there
// or not (finding 4f418f).
func AbrirOCrearRuta(root string, partes ...string) (*os.Root, error) {
	return bajar(root, true, partes)
}

// bajar is the one walk by descriptors both share (finding 579060): from
// root, each component looked at with Lstat, required to be a directory and
// opened as the object looked at (AbrirDirEn); with crear, a component that
// is not there is made first.
func bajar(root string, crear bool, partes []string) (*os.Root, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	for _, p := range partes {
		fi, err := r.Lstat(p)
		if crear && errors.Is(err, fs.ErrNotExist) {
			if err := r.Mkdir(p, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				r.Close()
				return nil, err
			}
			fi, err = r.Lstat(p)
		}
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

// ErrCambio is ReemplazarSiSigue finding that the name it was to replace is
// no longer the object that was looked at: it is gone, or another object
// took its place. Nothing was written.
var ErrCambio = errors.New("cambio entre mirarlo y reemplazarlo")

// PublicarEn writes data as the file name of r, whatever was there before:
// through a temp file in r and a rename within r, so the write never leaves
// r and a reader sees the old file or the new one. name is one component of
// r.
func PublicarEn(r *os.Root, name string, data []byte, mode os.FileMode) error {
	tmp, err := temporalEn(r, name, data, mode)
	if err != nil {
		return err
	}
	if err := r.Rename(tmp, name); err != nil {
		r.Remove(tmp)
		return err
	}
	return nil
}

// SigueEn says, as an error, whether r is no longer the directory
// root/partes... leads to: a directory hoom opened and wrote into that
// another process moved away (and maybe replaced) in the meantime. A writer
// asks after publishing, so it never reports as in place what it left in a
// moved directory (findings f89080, 6513a7, 46561c).
//
// Moving or renaming hoom's own directories while hoom works in them is out
// of hoom's threat model (Henry, 2026-10-27), like rewriting evidence after
// its photograph (spec evidencia-en-disco): this check is the cheap part,
// and what moves the directory after it, or where a moved directory ends
// up, is not hoom's to see.
func SigueEn(root string, r *os.Root, partes ...string) error {
	ahora, err := AbrirRuta(root, partes...)
	if err != nil {
		return fmt.Errorf("%s cambio mientras se escribia: %w", strings.Join(partes, "/"), err)
	}
	defer ahora.Close()
	a, err1 := r.Stat(".")
	b, err2 := ahora.Stat(".")
	if err1 != nil || err2 != nil || !os.SameFile(a, b) {
		return fmt.Errorf("%s cambio mientras se escribia", strings.Join(partes, "/"))
	}
	return nil
}

// ReemplazarSiSigue replaces the regular file name of r, which antes
// describes, with data, the way PublicarEn writes it. Right before the
// rename it checks that name is still that object; gone, or another one in
// its place, the temp is removed and the result is ErrCambio: it never
// recreates what was deleted nor overwrites what replaced it while it wrote.
// The check and the rename are two steps, with nothing that can block
// between them, and what changes the name in that window is not seen: there
// is no portable compare-and-rename. That residual window is accepted
// (Henry, 2026-10-26; findings 1e984e, d2e5b6, ba78b9). name is one
// component of r.
func ReemplazarSiSigue(r *os.Root, name string, data []byte, mode os.FileMode, antes fs.FileInfo) error {
	tmp, err := temporalEn(r, name, data, mode)
	if err != nil {
		return err
	}
	st, err := r.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist) || (err == nil && !mismo(antes, st)):
		r.Remove(tmp)
		return ErrCambio
	case err != nil:
		r.Remove(tmp)
		return err
	}
	if err := r.Rename(tmp, name); err != nil {
		r.Remove(tmp)
		return err
	}
	return nil
}

// temporalEn writes data to a new temp file next to name in r and
// returns its name: the name AtomicWrite gives its temps (TemporalDe), so a
// listing skips it like any other write in progress.
func temporalEn(r *os.Root, name string, data []byte, mode os.FileMode) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("%q no es un nombre dentro del directorio", name)
	}
	var tmp string
	var f *os.File
	for intento := 0; ; intento++ {
		var b [4]byte
		rand.Read(b[:]) // never fails (crypto/rand, Go 1.24+)
		tmp = "." + name + ".tmp-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b[:])), 10)
		var err error
		f, err = r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) || intento >= 10 {
			return "", err
		}
	}
	fallar := func(err error) (string, error) {
		f.Close()
		r.Remove(tmp)
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		return fallar(err)
	}
	if err := f.Chmod(mode); err != nil {
		return fallar(err)
	}
	// no fsync, as AtomicWrite: hoom's records are telemetry, and a sync on
	// every update made each one ~25 times slower (finding b6156c)
	if err := f.Close(); err != nil {
		r.Remove(tmp)
		return "", err
	}
	return tmp, nil
}
