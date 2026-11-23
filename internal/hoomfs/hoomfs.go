// Package hoomfs is the single source of truth for what under .hoom is
// LOCAL — telemetry and scaffolding that never travel in Git nor enter the
// change candidate — and for the one way hoom writes those files: atomically.
// Five packages used to carry their own copy of the local list; a directory
// added in one and forgotten in another meant a run charged to the role, a
// fingerprint that moved, or an untracked file blocking a task.
package hoomfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Local names the directories under .hoom/ that stay on this machine: the
// live cache, the narration of runs, task worktrees, blind trees and
// envelope records. Evidence (verdicts, findings, approvals) is NOT here:
// it travels.
var Local = []string{"cache", "worktrees", "runs", "isolated", "envelopes"}

// IsLocal reports whether a slash path relative to the project root points
// inside a local directory.
func IsLocal(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, d := range Local {
		if rel == ".hoom/"+d || strings.HasPrefix(rel, ".hoom/"+d+"/") {
			return true
		}
	}
	return false
}

// LocalFiles names the files under .hoom/ that stay on this machine
// (verify-en-el-entorno): local.yaml, the machine's adjustment
// (entorno.Ruta). Unlike the directories of Local, each one's line in
// .hoom/.gitignore is its bare name, with no "/".
var LocalFiles = []string{"local.yaml"}

// GitignoreBody is the canonical content of .hoom/.gitignore: a line `<d>/`
// for each directory of Local, in its order, then a line `<f>` for each file
// of LocalFiles (`local.yaml`, verify-en-el-entorno), each line ending in
// "\n".
func GitignoreBody() string {
	var b strings.Builder
	for _, d := range Local {
		b.WriteString(d + "/\n")
	}
	for _, f := range LocalFiles {
		b.WriteString(f + "\n")
	}
	return b.String()
}

// EnsureIgnored completes .hoom/.gitignore with the named local directories
// it lacks (all of them when none is named) and returns what it added. A file
// that already has them is never rewritten, not even byte-identically:
// registering telemetry must not move the candidate. Callers that only need
// their own entry ask for it by name, so opening a quarantine never edits the
// file for a directory it does not use.
func EnsureIgnored(root string, names ...string) (added []string, err error) {
	if len(names) == 0 {
		names = Local
	}
	lineas := make([]string, len(names))
	for i, d := range names {
		lineas[i] = d + "/"
	}
	return completarGitignore(root, lineas)
}

// completarGitignore appends to .hoom/.gitignore every one of lineas it
// lacks (a line counts as there when, trimmed, it is that one), creating the
// file when there is none, and returns what it added, in order. A file that
// already has them all is never written.
//
// An existing file is completed in place, never replaced (findings 3faa18,
// 4583f7): the missing lines go in ONE append (O_APPEND, after a "\n" when
// the file does not end in one), so the file keeps its inode, mode and
// owner, and a line another hand appended in place between the reading and
// the writing stays (the append lands after it). After the write, the name
// has to still be the object written to and hold every line added; when it
// is not (an editor replaced the file by rename meanwhile, so the append
// went to the old one), it reads the file again and recomputes what is
// missing, up to intentosGitignore times, and then it is an error: never
// "added" for a line that is not there. A file that does not exist is
// created with O_EXCL, mode 0644, and written with O_APPEND too: one created
// at the same time by someone else is completed like any existing one, and
// what another hand appends to the one just created, before its lines are
// written, stays ahead of them (finding 814254). What it cannot see is another
// hand appending, between its reading and its write, a last line without
// "\n": the lines added then follow that one on the same line.
func completarGitignore(root string, lineas []string) (added []string, err error) {
	// .hoom is walked by descriptors, read AND written in that one directory:
	// a .hoom swapped for a symlink to another project between the reading
	// and the writing never gets that project's .gitignore overwritten
	// (findings c6c300, 0c9aec)
	r, err := AbrirOCrearRuta(root, ".hoom")
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for intento := 0; intento < intentosGitignore; intento++ {
		added, listo, err := completarUnaVez(r, root, lineas)
		if err != nil {
			return nil, err
		}
		if !listo {
			continue
		}
		// The .hoom just written into has to still be the one root/.hoom
		// leads to; if another process moved it, this is an error, never
		// "added" for a .gitignore left in a moved directory (findings
		// 6513a7, 46561c, f27c7d).
		if err := SigueEn(root, r, ".hoom"); err != nil {
			return nil, err
		}
		return added, nil
	}
	return nil, fmt.Errorf(".hoom/.gitignore cambio %d veces mientras hoom lo completaba: vuelve a intentarlo", intentosGitignore)
}

// intentosGitignore bounds how many times completarGitignore reads and
// completes a .hoom/.gitignore that keeps changing under it.
const intentosGitignore = 5

// antesDeCompletar, when set, runs in completarGitignore between the reading
// of .hoom/.gitignore (or seeing it is not there) and the write, with the
// root. It exists only for the tests: what another hand does in that window
// (findings 4583f7) needs one to be shown. Atomic, like envelope's.
var antesDeCompletar atomic.Pointer[func(root string)]

// antesDeEscribirCreado, when set, runs in crearGitignore between the O_EXCL
// that creates .hoom/.gitignore and the write of its lines, with the root.
// Only for the tests, like antesDeCompletar: another hand that appends to the
// file just created (finding 814254) needs that window to be shown.
var antesDeEscribirCreado atomic.Pointer[func(root string)]

// completarUnaVez is one try of completarGitignore in r, the .hoom of root:
// listo false when the file changed under it and the try has to be made
// again, from a fresh reading.
func completarUnaVez(r *os.Root, root string, lineas []string) (added []string, listo bool, err error) {
	const name = ".gitignore"
	antes, err := r.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return crearGitignore(r, root, lineas)
	}
	if err != nil {
		return nil, false, err
	}
	if !antes.Mode().IsRegular() {
		// never followed nor opened: a symlink would take the write
		// elsewhere, a FIFO would block it
		return nil, false, &NoRegular{Path: name, Mode: antes.Mode()}
	}
	f, err := r.OpenFile(name, os.O_RDWR|os.O_APPEND|sinBloquear, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil // gone since the Lstat: try again
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	switch {
	case err != nil:
		return nil, false, err
	case !mismo(antes, st) || sigueSiendo(r, name, antes) != nil:
		return nil, false, nil // another object took the name: try again
	case !st.Mode().IsRegular():
		return nil, false, &NoRegular{Path: name, Mode: st.Mode()}
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxGitignore+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(raw)) > maxGitignore {
		return nil, false, fmt.Errorf("%s pasa de %d bytes", name, maxGitignore)
	}
	added = faltan(raw, lineas)
	if len(added) == 0 {
		return nil, true, nil
	}
	data := strings.Join(added, "\n") + "\n"
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		data = "\n" + data
	}
	esperar(&antesDeCompletar, root)
	if _, err := f.Write([]byte(data)); err != nil {
		return nil, false, err
	}
	if !quedaron(r, name, antes, added) {
		return nil, false, nil
	}
	return added, true, nil
}

// crearGitignore creates the .hoom/.gitignore of r with lineas, mode 0644,
// only if there is still none (O_EXCL): listo false when someone else
// created it first, or replaced it right after, so the try is made again.
func crearGitignore(r *os.Root, root string, lineas []string) (added []string, listo bool, err error) {
	const name = ".gitignore"
	added = faltan(nil, lineas)
	esperar(&antesDeCompletar, root)
	// O_APPEND too (finding 814254): another hoom that finds the file just
	// created, empty, appends its lines to it before these are written; with
	// the offset at 0 they would land on top of its, and both would say
	// "added".
	f, err := r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	esperar(&antesDeEscribirCreado, root)
	creado, err := f.Stat()
	if err == nil {
		err = f.Chmod(0o644) // as before, whatever the umask
	}
	if err == nil {
		_, err = f.Write([]byte(strings.Join(added, "\n") + "\n"))
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, false, err
	}
	if !quedaron(r, name, creado, added) {
		return nil, false, nil
	}
	return added, true, nil
}

// quedaron says whether the name in r is still the object escrito describes
// and holds every one of lineas: what was written there is in place.
func quedaron(r *os.Root, name string, escrito fs.FileInfo, lineas []string) bool {
	if st, err := r.Lstat(name); err != nil || !mismo(escrito, st) {
		return false
	}
	raw, err := LeerRegularEn(r, name, maxGitignore)
	return err == nil && len(faltan(raw, lineas)) == 0
}

// faltan are the lines of lineas that raw lacks, in order and once each: a
// line counts as there when, trimmed, it is that one.
func faltan(raw []byte, lineas []string) []string {
	have := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var out []string
	for _, l := range lineas {
		if !have[l] {
			out = append(out, l)
			have[l] = true
		}
	}
	return out
}

// esperar runs the test hook in gancho (antesDeCompletar or
// antesDeEscribirCreado), if set.
func esperar(gancho *atomic.Pointer[func(root string)], root string) {
	if f := gancho.Load(); f != nil {
		(*f)(root)
	}
}

// EnsureIgnoredFiles is EnsureIgnored for the local files
// (verify-en-el-entorno): it completes .hoom/.gitignore with a line `<f>`
// for each named file it lacks (all of LocalFiles when none is named),
// creating .hoom/.gitignore when there is none, and returns what it added,
// in order. A line counts as there when, trimmed, it is `<f>`. It keeps
// EnsureIgnored's guarantees: a file that already has them is never
// rewritten, not even byte-identically; .hoom is walked by descriptors, read
// and written in that one directory. EnsureIgnored itself still adds
// directories only. `hoom init` calls it for local.yaml on the project's
// .hoom/.gitignore when that one already exists and, with the project in a
// subdirectory of its repository, on the main checkout's, which it then
// creates with that one line when there is none.
func EnsureIgnoredFiles(root string, names ...string) (added []string, err error) {
	if len(names) == 0 {
		names = LocalFiles
	}
	return completarGitignore(root, names)
}

// maxGitignore caps .hoom/.gitignore: hoom writes a few lines.
const maxGitignore = 1 << 20

// crearTemporal creates, next to path, the hidden temp file a whole write
// goes through: "." + the name of path + ".tmp-" + the digits os.CreateTemp
// puts in place of *, the one form TemporalDe recognizes (finding 6b1f72).
// It never ends in the target's extension, so listings that filter by
// suffix never pick it up.
func crearTemporal(path string) (*os.File, error) {
	return os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
}

// TemporalDe says whether name is a temp file of AtomicWrite or
// PublicarNuevo, exactly as they write it, and the name of the file it will
// become. Anything else is not one of hoom's writes in progress.
func TemporalDe(name string) (base string, ok bool) {
	rest, ok := strings.CutPrefix(name, ".")
	i := strings.LastIndex(rest, ".tmp-")
	if !ok || i <= 0 {
		return "", false
	}
	digitos := rest[i+len(".tmp-"):]
	if digitos == "" || strings.Trim(digitos, "0123456789") != "" {
		return "", false
	}
	return rest[:i], true
}

// AtomicWrite writes data to path through a temp file in the same directory
// and a rename: a concurrent reader sees the previous complete file or the
// new complete file, never a truncated one, and a crash mid-write leaves the
// previous content in place.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := crearTemporal(path)
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// PublicarNuevo creates path with data, whole or not at all, and never
// replaces a file already there: then it fails with an error that is
// os.ErrExist. On a filesystem without hard links it fails with
// ErrSinEnlaces; with path already taken it may fail with os.ErrExist
// first, since macOS and Linux answer that before the missing link, so a
// caller treats both (finding e581eb). The data goes to a temp file that
// is closed first and then,
// under candado, linked to path: one operation that never replaces,
// whoever else writes there, so a reader sees nothing or all of it and a
// crash leaves at most the temp file (TemporalDe names it). A filesystem
// without hard links (exFAT, FAT) has no portable operation that is whole
// and never replaces at once: there it fails with ErrSinEnlaces and writes
// nothing (enmienda 2 of items-y-columna-derivada; findings c6ddec, d2c506,
// 9e2e0d, c380a2, 589ad5, 25172f). candado serializes the hoom publishers
// of a directory.
func PublicarNuevo(path string, data []byte, mode os.FileMode, candado string) error {
	tmp, err := crearTemporal(path)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err != nil {
		return err
	}
	unlock, err := Lock(candado, esperaPublicar)
	if err != nil {
		return err
	}
	defer unlock()
	switch err := enlazar(tmp.Name(), path); {
	case err == nil:
		return nil
	case sinEnlaces(err):
		return fmt.Errorf("%w: %s (%v)", ErrSinEnlaces, filepath.Dir(path), err)
	default:
		return err
	}
}

// ErrSinEnlaces: the filesystem of .hoom has no hard links, and a new item
// or review record cannot be published whole and without replacing.
var ErrSinEnlaces = errors.New(".hoom necesita un sistema de archivos con hard links (exFAT y FAT no los tienen)")

// enlazar is os.Link; a test of this package replaces it to stand for a
// filesystem without hard links, where PublicarNuevo fails.
var enlazar = os.Link

// sinEnlaces says that a link failed because the filesystem has no hard
// links, in each system's words (sinEnlacesSO). Any other failure, an
// existing path among them, is the publication's answer.
func sinEnlaces(err error) bool {
	return err != nil && (errors.Is(err, errors.ErrUnsupported) || sinEnlacesSO(err))
}

// esperaPublicar is how long PublicarNuevo waits for another publisher of
// the same directory: a link, never a long hold.
const esperaPublicar = 10 * time.Second

// ErrLocked: another process holds the lock and did not let go in time.
var ErrLocked = errors.New("otro proceso tiene tomado")

// Lock takes an exclusive lock on the file at path, across processes (the
// CLI and the Studio) and goroutines, waiting up to wait. It is the kernel's
// lock (flock, LockFileEx): a holder that dies releases it with its process,
// so there is no stale lock to guess, and releasing (the returned func) only
// closes this holder's descriptor. The file stays; it lives under
// .hoom/cache/locks/ (LockPath), local and ignored.
func Lock(path string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		ok, err := tryLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if ok {
			return func() { f.Close() }, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("%w %s", ErrLocked, path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// LockPath is where the lock called name of the project at root lives.
func LockPath(root, name string) string {
	return filepath.Join(root, ".hoom", "cache", "locks", name+".lock")
}
