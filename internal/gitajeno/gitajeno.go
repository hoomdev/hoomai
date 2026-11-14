// Package gitajeno photographs and compares what the rule git-ajeno of
// contencion-del-writer looks at in the git common directory of a tree: the
// refs under refs/replace/ (loose and packed) and the object files that
// already existed (loose objects, packs and their indexes). agentcmd
// (RuleGitAjeno, FotoGitAjeno, CambiosGitAjeno), verifycmd (around the gates
// of a verify with Sobre) and itemcmd (around the pilot's save) use it.
//
// Every photograph and every comparison has a bound of time, of the bytes it
// reads and of the entries it lists from directories, which it lists a batch
// at a time (aclaracion 30): passing a bound counts as changed (SinTerminar),
// and it never hangs nor holds a whole directory. It runs no git that reads
// the working tree (no status, no fsmonitor, no filter).
package gitajeno

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/materialgit"
)

// SinTerminar is the one path Cambios gives for a photograph, or a
// comparison, that did not end within its bound: it counts as changed. It is
// materialgit's mark (agentcmd.FotoSinTerminar).
const SinTerminar = materialgit.SinTerminar

// Tope and MaxLeido are the bounds of one photograph or one comparison
// that its callers use (agentcmd.TopeFotoGitAjeno and
// agentcmd.MaxLeidoGitAjeno): its time, and the bytes it hashes.
const (
	Tope           = 10 * time.Second
	MaxLeido int64 = 1 << 30
)

// Ilegible is materialgit's mark for what could not be read. A photograph of
// refs/replace/ no longer gives it to a loose ref: what cannot be read is
// photographed by its metadata (recorrido.entrada), so two states never
// share a value.
const Ilegible = materialgit.Ilegible

// Foto is the photograph of the git common directory of a tree (Tomar): its
// refs under refs/replace/, loose and packed, and the object files that
// existed. The zero Foto (outside a repository) never changes.
type Foto struct {
	// comun is the git common directory, absolute; "" = nothing to compare.
	comun string
	// sha256 says the repository's objects are named by sha256 (its config
	// says objectformat = sha256): the checksum of its packs.
	sha256 bool
	// refs are the refs under refs/replace/ by full name -> their value
	// (a loose one over a packed one of the same name).
	refs map[string]string
	// objetos are the loose objects, packs and indexes by absolute path.
	objetos map[string]objeto
	// sinTerminar says the photograph did not end within its bound.
	sinTerminar bool
}

// objeto is what the photograph keeps of one object file: enough to know
// whether it changed afterwards without reading it again.
type objeto struct {
	ctime, mtime, size int64
	ino                uint64
	// cola is a pack's or an index's trailer (the last 32 bytes, its
	// checksum included), hex.
	cola string
	// huella is a loose object's raw bytes' sha256, hex; "" when the
	// photograph's bytes did not reach it (it counts as changed if touched).
	huella string
}

// presupuesto is what is left of one photograph's or comparison's bound.
type presupuesto struct {
	limite time.Time
	bytes  int64
}

func (p *presupuesto) vencido() bool { return time.Now().After(p.limite) }

// errVencido is a reading cut by the time of its bound.
var errVencido = errors.New("gitajeno: el tope de tiempo vencio")

// errTope is a reading cut by one of its bounds of entries, depth or bytes
// (aclaracion 30): like errVencido, it leaves the photograph or the
// comparison unfinished (SinTerminar).
var errTope = errors.New("gitajeno: la foto paso su tope de entradas, profundidad o bytes")

// Tomar photographs, for the tree dir, its git common directory's refs under
// refs/replace/ and the files under objects/ (loose objects, packs and their
// indexes) within tope and maxLeido bytes hashed. Outside a git repository
// there is nothing to compare: the zero Foto. A photograph that passes tope,
// or a bound of the walk of refs/replace/ or of objects/ (maxEntradasRefs,
// maxProfundidad, maxEntradasObjetos), is unfinished: Cambios against it
// gives [SinTerminar]. A loose object beyond maxLeido is kept without its
// hash.
func Tomar(dir string, tope time.Duration, maxLeido int64) Foto {
	pr := &presupuesto{limite: time.Now().Add(tope), bytes: maxLeido}
	l, err := materialgit.Ubicar(dir, pr.limite)
	if err != nil {
		if pr.vencido() {
			return Foto{sinTerminar: true}
		}
		return Foto{}
	}
	foto := Foto{comun: l.Comun, sha256: formatoSha256(l.Comun), objetos: map[string]objeto{}}
	refs, err := refsDeReemplazo(l.Comun, pr)
	if err != nil {
		return Foto{comun: l.Comun, sinTerminar: true}
	}
	foto.refs = refs
	completa := recorrerObjetos(l.Comun, pr, func(p string, fi fs.FileInfo) bool {
		o := objetoDe(fi)
		if esPackOIdx(p) {
			o.cola = colaDe(p)
		} else if fi.Size() <= pr.bytes {
			h, err := huellaCruda(p, fi.Size(), pr)
			if errors.Is(err, errVencido) {
				return false
			}
			o.huella = h
		}
		foto.objetos[p] = o
		return true
	})
	if !completa {
		foto.sinTerminar = true
	}
	return foto
}

// Cambios looks again, within tope and maxLeido bytes hashed, at the git
// common directory antes photographed and lists, sorted, what breaks
// git-ajeno since then: each ref under refs/replace/ created, moved or
// removed, loose or packed, named `<comun>/refs/replace/<nombre>`
// (absolute, the same name whether it is loose or packed), a line of
// packed-refs too long to be a ref that changed, appeared or went as
// `<comun>/packed-refs`, and each loose
// object, pack or index that existed in antes and now has other bytes, by its
// absolute path. Only the files whose ctime, mtime, size or inode changed are
// read. A file it cannot read, or that would pass maxLeido, counts as
// changed, by its path; an unfinished antes, or a comparison that passes
// tope or a bound of the walk of refs/replace/, adds SinTerminar. nil when
// nothing changed.
func Cambios(antes Foto, tope time.Duration, maxLeido int64) []string {
	if antes.sinTerminar {
		return []string{SinTerminar}
	}
	if antes.comun == "" {
		return nil
	}
	pr := &presupuesto{limite: time.Now().Add(tope), bytes: maxLeido}
	var out []string
	ahora, err := refsDeReemplazo(antes.comun, pr)
	switch {
	case errors.Is(err, errVencido), errors.Is(err, errTope):
		return []string{SinTerminar}
	case err != nil:
		out = append(out, filepath.Join(antes.comun, "refs", "replace")+string(filepath.Separator))
	default:
		vistas := map[string]bool{}
		nombrar := func(n string) {
			p := filepath.Join(antes.comun, filepath.FromSlash(n))
			if strings.HasPrefix(n, clavePackedLarga) {
				p = filepath.Join(antes.comun, "packed-refs") // a long line of it
			}
			if !vistas[p] {
				vistas[p] = true
				out = append(out, p)
			}
		}
		for n, v := range antes.refs {
			if w, ok := ahora[n]; !ok || w != v {
				nombrar(n)
			}
		}
		for n := range ahora {
			if _, ok := antes.refs[n]; !ok {
				nombrar(n)
			}
		}
	}
	rutas := make([]string, 0, len(antes.objetos))
	for p := range antes.objetos {
		rutas = append(rutas, p)
	}
	sort.Strings(rutas)
	for _, p := range rutas {
		if pr.vencido() {
			out = append(out, SinTerminar)
			break
		}
		o := antes.objetos[p]
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed (gc, prune): not a rewrite
		}
		if err != nil || !fi.Mode().IsRegular() {
			out = append(out, p)
			continue
		}
		if objetoDe(fi) == (objeto{ctime: o.ctime, mtime: o.mtime, size: o.size, ino: o.ino}) {
			continue // untouched since the photograph
		}
		mismo, err := mismoContenido(p, o, fi, antes.sha256, pr)
		if errors.Is(err, errVencido) {
			out = append(out, SinTerminar)
			break
		}
		if !mismo {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// formatoSha256 says whether the config of the common directory comun names
// sha256 as the object format (extensions.objectformat), read whole only
// when it is no larger than materialgit.MaxLeido.
func formatoSha256(comun string) bool {
	p := filepath.Join(comun, "config")
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > materialgit.MaxLeido {
		return false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(strings.ToLower(strings.TrimSpace(l)), "=")
		if ok && strings.TrimSpace(k) == "objectformat" && strings.TrimSpace(v) == "sha256" {
			return true
		}
	}
	return false
}

// refsDeReemplazo reads the refs under refs/replace/ of the common directory
// comun within pr: packed-refs (packedDeReemplazo), then the loose ones over
// them (recorrido); with the reftable backend, git for-each-ref
// (refsDeReftable, which reads no working tree). errVencido when pr's time
// passes, errTope when it passes a bound of entries, depth or bytes
// (aclaracion 30); another error when they cannot be read.
func refsDeReemplazo(comun string, pr *presupuesto) (map[string]string, error) {
	if st, err := os.Stat(filepath.Join(comun, "reftable")); err == nil && st.IsDir() {
		return refsDeReftable(comun, pr)
	}
	refs := map[string]string{}
	if err := packedDeReemplazo(comun, refs, pr); err != nil {
		return nil, err
	}
	base := filepath.Join(comun, "refs", "replace")
	fi, err := os.Lstat(base)
	if errors.Is(err, fs.ErrNotExist) {
		return refs, nil
	}
	if err != nil {
		return nil, err // fails closed: the whole refs/replace/ counts as changed
	}
	r := &recorrido{pr: pr, refs: refs, quedan: maxEntradasRefs}
	if err := r.entrada(base, "refs/replace", fi, 0); err != nil {
		return nil, err
	}
	return refs, nil
}

// maxEntradasRefs, maxProfundidad and lote bound the walk of refs/replace/
// (recorrido; aclaraciones 30 and 30b): the entries it reads in all, from
// every directory it walks (those its symlinks resolve to included); how many
// levels it goes down under refs/replace/; and how many entries it lists
// from a directory at a time. Passing the first two is errTope, whatever the
// time left.
const (
	maxEntradasRefs = 10000
	maxProfundidad  = 32
	lote            = 256
)

// recorrido is one walk of the loose refs under refs/replace/
// (refsDeReemplazo) within pr and its own bounds (ronda 3, d204f2, bea100 and
// 6ce684; aclaraciones 30 and 30b). It lists each directory lote entries at
// a time (never a whole directory in one read), looking at pr's time at
// every batch and counting each entry out of quedan and its name's bytes out
// of pr's, and only once the directory is listed within them does it look at
// its entries: one with more entries than are left ends the walk before any
// of them is read. It goes down at most maxProfundidad levels, and follows a
// symlink to a directory as git does (its entries are refs too, under the
// symlink's name: git takes refs/replace/<objeto>/<algo> as a replacement of
// <objeto>), except into a directory it is already inside (a loop).
type recorrido struct {
	pr     *presupuesto
	refs   map[string]string
	quedan int
	// dentro are the directories of the current path, the outermost first:
	// a symlink back to one of them is a loop.
	dentro []fs.FileInfo
	// sumas are the hashes of the symlinked directories being walked, the
	// outermost first: every ref noted under one goes into its hash.
	sumas []hash.Hash
}

// anotar notes the ref n with the value v, also in the hash of every
// symlinked directory it is under.
func (r *recorrido) anotar(n, v string) {
	r.refs[n] = v
	for _, h := range r.sumas {
		_, _ = io.WriteString(h, especial("ref", n, v))
	}
}

// entrada photographs the entry at p, named n (refs/replace/...), whose Lstat
// is fi, prof levels under refs/replace/: what git reads of it and, for
// whatever cannot be read as one line, enough that any touch changes it
// (aclaracion 28: a photograph never joins two different states in one
// value; ronda 2, 61babd and a48bb4):
//   - a directory: each entry in it, by its own name (directorio);
//   - a lock file of git (`.lock`): nothing, its result is what counts;
//   - a symlink, which git follows: its target, read with readlink, and the
//     photograph of what it resolves to; to a directory, its metadata and
//     the hash of every ref under it, which are noted by their own names
//     too (directorio);
//   - anything else, its content (contenidoDeRef).
//
// errVencido when pr's time passes; errTope past a bound of the walk.
func (r *recorrido) entrada(p, n string, fi fs.FileInfo, prof int) error {
	if fi.IsDir() {
		v, err := r.directorio(p, n, false, prof)
		if err != nil {
			return err
		}
		if v != "" {
			r.anotar(n, v) // one it could not walk
		}
		return nil
	}
	if strings.HasSuffix(n, ".lock") {
		return nil // a ref being written: its result is what counts
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		v, err := contenidoDeRef(p, false, r.pr)
		if err != nil {
			return err
		}
		r.anotar(n, v)
		return nil
	}
	destino, err := os.Readlink(p)
	if err != nil {
		r.anotar(n, especial("enlace-ilegible", metadatos(fi)))
		return nil
	}
	var v string
	if st, serr := os.Stat(p); serr == nil && st.IsDir() {
		v, err = r.directorio(p, n, true, prof)
	} else {
		v, err = contenidoDeRef(p, true, r.pr)
	}
	if err != nil {
		return err
	}
	r.anotar(n, especial("enlace", destino, v))
	return nil
}

// directorio notes, as refs named n/<entry>, each entry of the directory at
// p (followed when enlace: a symlink n resolves to it), prof levels under
// refs/replace/, in the order of their names, once it is listed whole (listar). For
// a symlinked one it gives its value: its metadata and the sha256 of every
// ref noted under it, so that any change inside it also changes the
// symlink's; "" for the others. A directory it cannot open is given by why
// (git, run by the same user, cannot read it either), one it is already
// inside is a loop, given as such and not walked again, and one that stopped
// being a directory is given by its metadata. errVencido when pr's time
// passes; errTope past maxProfundidad, maxEntradasRefs or pr's bytes; another
// error when it cannot be listed (it fails closed).
func (r *recorrido) directorio(p, n string, enlace bool, prof int) (string, error) {
	if prof >= maxProfundidad {
		return "", errTope
	}
	f, err := abrir(p, enlace)
	if err != nil {
		return especial("dir-ilegible", err.Error()), nil
	}
	st, err := f.Stat()
	switch {
	case err != nil:
		f.Close()
		return especial("dir-ilegible", err.Error()), nil
	case !st.IsDir():
		f.Close()
		return especial("no-dir", metadatos(st)), nil // it changed meanwhile
	}
	for _, d := range r.dentro {
		if os.SameFile(d, st) {
			f.Close()
			return especial("bucle", metadatos(st)), nil
		}
	}
	ents, err := r.listar(f)
	f.Close()
	if err != nil {
		return "", err
	}
	r.dentro = append(r.dentro, st)
	defer func() { r.dentro = r.dentro[:len(r.dentro)-1] }()
	var suma hash.Hash
	if enlace {
		suma = sha256.New()
		r.sumas = append(r.sumas, suma)
		defer func() { r.sumas = r.sumas[:len(r.sumas)-1] }()
	}
	for _, e := range ents {
		if r.pr.vencido() {
			return "", errVencido
		}
		m := n + "/" + e.Name()
		fi, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue // gone while it was walked: as if it never was
		}
		if err != nil {
			r.anotar(m, especial("ilegible", err.Error()))
			continue
		}
		if err := r.entrada(filepath.Join(p, e.Name()), m, fi, prof+1); err != nil {
			return "", err
		}
	}
	if !enlace {
		return "", nil
	}
	return especial("dir", metadatos(st), hex.EncodeToString(suma.Sum(nil))), nil
}

// listar lists the directory f whole, lote entries at a time, within pr and
// quedan, and gives its entries sorted by name. errVencido when pr's time
// passes, errTope past quedan or pr's bytes, another error when it cannot be
// listed.
func (r *recorrido) listar(f *os.File) ([]fs.DirEntry, error) {
	var ents []fs.DirEntry
	for {
		if r.pr.vencido() {
			return nil, errVencido
		}
		l, err := f.ReadDir(lote)
		if r.quedan -= len(l); r.quedan < 0 {
			return nil, errTope
		}
		for _, e := range l {
			if r.pr.bytes -= int64(len(e.Name())); r.pr.bytes < 0 {
				return nil, errTope
			}
		}
		ents = append(ents, l...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	return ents, nil
}

// maxLineaRef is the longest line of packed-refs that can be a ref: a hash
// (64 hex digits at most), a space, a name of up to 4096 bytes and its end of
// line. bufPacked is the reading buffer: all the reading of packed-refs
// holds at a time, whatever its lines (ronda 2, d91180).
const (
	maxLineaRef = 64 + 1 + 4096 + 2
	bufPacked   = 64 << 10
)

// clavePackedLarga starts the name, in a photograph's refs, of a line of
// packed-refs longer than maxLineaRef, followed by its offset: it is not a
// ref, and it is not dropped either.
const clavePackedLarga = "packed-refs:linea-larga@"

// packedDeReemplazo adds to refs the refs under refs/replace/ of the
// packed-refs of the common directory comun, read as a stream (ronda 2,
// d91180): its bytes come out of pr, it looks at pr's time at every line and
// every bufPacked of a long one, and it never holds more than bufPacked of
// it. A line longer than maxLineaRef cannot be a ref: it is photographed by
// its length and the sha256 of its bytes, under clavePackedLarga and its
// offset, so that any change to it counts and never shares a value with
// another state, and the lines after it are still read. errVencido when pr's
// time passes; another error (it fails closed) when packed-refs is not a
// regular file, passes pr's bytes, or cannot be read. No packed-refs adds
// nothing.
func packedDeReemplazo(comun string, refs map[string]string, pr *presupuesto) error {
	f, err := abrir(filepath.Join(comun, "packed-refs"), true)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("gitajeno: packed-refs no es un archivo regular")
	}
	tope := pr.bytes
	if fi.Size() > tope {
		return errTope
	}
	r := bufio.NewReaderSize(io.LimitReader(f, tope+1), bufPacked)
	var leidos int64
	defer func() { pr.bytes -= leidos }()
	leer := func() ([]byte, error) {
		if pr.vencido() {
			return nil, errVencido
		}
		linea, err := r.ReadSlice('\n')
		leidos += int64(len(linea))
		if leidos > tope {
			return nil, errTope
		}
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
		return linea, err
	}
	reemplazo := []byte("refs/replace/")
	for {
		inicio := leidos
		linea, err := leer()
		if errors.Is(err, errVencido) || errors.Is(err, errTope) {
			return err
		}
		if errors.Is(err, bufio.ErrBufferFull) || len(linea) > maxLineaRef {
			h := sha256.New()
			h.Write(linea)
			for errors.Is(err, bufio.ErrBufferFull) {
				if linea, err = leer(); errors.Is(err, errVencido) || errors.Is(err, errTope) {
					return err
				}
				h.Write(linea)
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			refs[clavePackedLarga+strconv.FormatInt(inicio, 10)] = strconv.FormatInt(leidos-inicio, 10) + ":" + hex.EncodeToString(h.Sum(nil))
		} else if err != nil && !errors.Is(err, io.EOF) {
			return err
		} else if l := bytes.TrimRight(linea, "\r\n"); len(l) > 0 && l[0] != '#' && l[0] != '^' {
			if v, n, ok := bytes.Cut(l, []byte(" ")); ok && bytes.HasPrefix(n, reemplazo) {
				refs[string(n)] = string(v)
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

// maxRef is the size up to which a loose ref is read as its value: a ref is
// one line.
const maxRef = 4096

// especial is a value of a loose ref's photograph that is not its content (recorrido.entrada): a newline,
// its kind, and each part with its length. A trimmed content never starts
// with a newline, and the lengths of the free parts keep any two of these
// values, or one of them and a ref's content, from being the same string.
func especial(tipo string, partes ...string) string {
	var b strings.Builder
	b.WriteString("\n" + tipo)
	for _, x := range partes {
		fmt.Fprintf(&b, ":%d:%s", len(x), x)
	}
	return b.String()
}

// contenidoDeRef is the photograph of the file at p (followed when seguir), a
// loose ref or what a symlink there resolves to:
//   - a regular file of up to maxRef bytes: its content, trimmed (what git
//     takes from it);
//   - a bigger regular file: its size, inode, mtime and ctime, and the sha256
//     of all its bytes;
//   - what is not a regular file, or cannot be opened or read: its kind and
//     its metadata (ctime included where the system gives it).
//
// It never blocks on the open (abrir), and every byte it reads comes out of
// pr's: a regular file pr's bytes do not reach is errTope, an unfinished
// photograph on every system, never its metadata without its hash (ronda 3,
// 4064a8; aclaracion 30), and one that shrinks while it is hashed is an error
// (it fails closed). errVencido when pr's time passes.
func contenidoDeRef(p string, seguir bool, pr *presupuesto) (string, error) {
	f, err := abrir(p, seguir)
	if err != nil {
		fi, serr := os.Lstat(p)
		if seguir {
			fi, serr = os.Stat(p)
		}
		switch {
		case errors.Is(serr, fs.ErrNotExist):
			return especial("ausente"), nil
		case serr != nil:
			return especial("ilegible", serr.Error()), nil
		}
		return especial("ilegible", metadatos(fi)), nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return especial("ilegible", err.Error()), nil
	}
	if !fi.Mode().IsRegular() {
		return especial("no-regular", metadatos(fi)), nil
	}
	if fi.Size() <= maxRef {
		raw, err := io.ReadAll(io.LimitReader(f, maxRef+1))
		if pr.bytes -= int64(len(raw)); pr.bytes < 0 {
			return "", errTope
		}
		if err != nil {
			return especial("ilegible", metadatos(fi)), nil
		}
		if len(raw) <= maxRef {
			return strings.TrimSpace(string(raw)), nil
		}
		// it grew while it was read: a big one, as it is now
		if fi, err = f.Stat(); err != nil {
			return especial("ilegible", err.Error()), nil
		}
	}
	n := fi.Size()
	if n > pr.bytes {
		return "", errTope
	}
	h := sha256.New()
	if err := copiar(h, io.NewSectionReader(f, 0, n), n, pr); err != nil {
		if errors.Is(err, errVencido) {
			return "", err
		}
		return "", fmt.Errorf("gitajeno: %s cambio mientras se leia: %w", p, err)
	}
	return especial("grande", metadatos(fi), hex.EncodeToString(h.Sum(nil))), nil
}

// metadatos is fi's kind and permissions, size, inode, mtime and ctime (in
// nanoseconds; inode and ctime where the system gives them).
func metadatos(fi fs.FileInfo) string {
	ctime, _ := materialgit.CtimeDe(fi)
	return fmt.Sprintf("%s:%d:%d:%d:%d", fi.Mode(), fi.Size(), materialgit.InodoDe(fi), fi.ModTime().UnixNano(), ctime)
}

// refsDeReftable reads the refs under refs/replace/ of the common directory
// comun, with the reftable backend, from git for-each-ref (GIT_DIR, with
// core.fsmonitor=false, untranslated: it reads no working tree), as a stream
// within pr (aclaracion 30): its output comes out of pr's bytes and each of
// its lines out of maxEntradasRefs, and no line longer than maxLineaRef is
// kept; passing any of them is errTope (git stops at its next write), and
// pr's time errVencido.
func refsDeReftable(comun string, pr *presupuesto) (map[string]string, error) {
	ctx, cancel := context.WithDeadline(context.Background(), pr.limite)
	defer cancel()
	l := &lineasDeRefs{pr: pr, refs: map[string]string{}, quedan: maxEntradasRefs}
	cmd := exec.CommandContext(ctx, "git", "-c", "core.fsmonitor=false", "for-each-ref", "--format=%(refname) %(objectname)", "refs/replace/")
	cmd.WaitDelay = time.Second
	cmd.Env = append(cmd.Environ(), "GIT_DIR="+comun, "LC_ALL=C", "LANGUAGE=C")
	cmd.Dir = comun
	cmd.Stdout = l
	err := cmd.Run()
	if l.err == nil && err == nil {
		l.linea(l.resto) // a last line without its newline
	}
	switch {
	case l.err != nil:
		return nil, l.err
	case err != nil && pr.vencido():
		return nil, errVencido
	case err != nil:
		return nil, err
	}
	return l.refs, nil
}

// lineasDeRefs takes the output of git for-each-ref (refsDeReftable) as it
// comes, a line `<refname> <objectname>` at a time, within its bounds.
type lineasDeRefs struct {
	pr     *presupuesto
	refs   map[string]string
	quedan int
	// resto is the start of a line whose end has not come yet.
	resto []byte
	// err is the bound passed (errTope): nothing more is taken.
	err error
}

// Write takes b: its bytes out of pr's, each whole line into refs.
func (l *lineasDeRefs) Write(b []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	if l.pr.bytes -= int64(len(b)); l.pr.bytes < 0 {
		l.err = errTope
		return 0, l.err
	}
	n := len(b)
	for len(b) > 0 && l.err == nil {
		i := bytes.IndexByte(b, '\n')
		fin := i
		if i < 0 {
			fin = len(b)
		}
		if len(l.resto)+fin > maxLineaRef {
			l.err = errTope
			break
		}
		l.resto = append(l.resto, b[:fin]...)
		if i < 0 {
			break
		}
		l.linea(l.resto)
		l.resto, b = l.resto[:0], b[i+1:]
	}
	if l.err != nil {
		return 0, l.err
	}
	return n, nil
}

// linea takes one line of the output: an empty one is nothing, and each other
// counts out of quedan (errTope past it).
func (l *lineasDeRefs) linea(s []byte) {
	s = bytes.TrimSpace(s)
	if len(s) == 0 {
		return
	}
	if l.quedan--; l.quedan < 0 {
		l.err = errTope
		return
	}
	if n, v, ok := bytes.Cut(s, []byte(" ")); ok {
		l.refs[string(n)] = string(v)
	}
}

var (
	// objetoSuelto matches objects/<xx>/<rest> of a loose object (sha1 or
	// sha256).
	objetoSuelto = regexp.MustCompile(`^[0-9a-f]{2}/([0-9a-f]{38}|[0-9a-f]{62})$`)
	// packOIdx matches objects/pack/pack-<hash>.pack and .idx.
	packOIdx = regexp.MustCompile(`^pack/pack-[0-9a-f]+\.(pack|idx)$`)
	// dosHex matches the name of a directory of loose objects, objects/<xx>.
	dosHex = regexp.MustCompile(`^[0-9a-f]{2}$`)
)

// maxEntradasObjetos bounds the entries the walk of objects/ reads in all
// (recorrerObjetos; aclaracion 30). It is not maxEntradasRefs: the loose
// objects of a repository are many more than its replacement refs (git gc
// --auto packs them past some 6700, but only when a command runs it), and
// passing it leaves an unfinished photograph, which no save gets past.
const maxEntradasObjetos = 1 << 17

// recorrerObjetos calls f for each loose object, pack and index under the
// objects directory of comun (only in objects/<xx>/ and objects/pack/: never
// objects/info/, nor the other files of pack/), while pr's time lasts, the
// entries it reads stay within maxEntradasObjetos and f says to go on. Each
// directory is listed lote entries at a time (listarObjetos), never whole in
// one read. It says whether it went through all of them.
func recorrerObjetos(comun string, pr *presupuesto, f func(p string, fi fs.FileInfo) bool) bool {
	base := filepath.Join(comun, "objects")
	quedan := maxEntradasObjetos
	var dirs []string
	if !listarObjetos(base, pr, &quedan, func(e fs.DirEntry) bool {
		if e.IsDir() && (e.Name() == "pack" || dosHex.MatchString(e.Name())) {
			dirs = append(dirs, e.Name())
		}
		return true
	}) {
		return false
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		dir := filepath.Join(base, d)
		if !listarObjetos(dir, pr, &quedan, func(e fs.DirEntry) bool {
			rel := d + "/" + e.Name()
			if !objetoSuelto.MatchString(rel) && !packOIdx.MatchString(rel) {
				return true
			}
			fi, err := e.Info()
			if err != nil || !fi.Mode().IsRegular() {
				return true
			}
			return f(filepath.Join(dir, e.Name()), fi)
		}) {
			return false
		}
	}
	return true
}

// listarObjetos calls f for each entry of the directory dir (not followed),
// lote at a time, looking at pr's time at every batch and counting each
// entry out of quedan, while f says to go on. It says whether it went
// through all of them; a directory it cannot open or list has none left.
func listarObjetos(dir string, pr *presupuesto, quedan *int, f func(fs.DirEntry) bool) bool {
	d, err := abrir(dir, false)
	if err != nil {
		return true
	}
	defer d.Close()
	for {
		if pr.vencido() {
			return false
		}
		ents, err := d.ReadDir(lote)
		if *quedan -= len(ents); *quedan < 0 {
			return false
		}
		for _, e := range ents {
			if !f(e) {
				return false
			}
		}
		if err != nil {
			return true // its end (io.EOF), or what cannot be listed
		}
	}
}

// esPackOIdx says whether the object file p is a pack or an index.
func esPackOIdx(p string) bool {
	return strings.HasSuffix(p, ".pack") || strings.HasSuffix(p, ".idx")
}

// objetoDe is what changes in fi when its file is written, replaced or
// chmodded: its ctime (which a user process cannot set back), mtime, size
// and inode.
func objetoDe(fi fs.FileInfo) objeto {
	o := objeto{mtime: fi.ModTime().UnixNano(), size: fi.Size(), ino: materialgit.InodoDe(fi)}
	if c, ok := materialgit.CtimeDe(fi); ok {
		o.ctime = c
	}
	return o
}

// colaDe is the end of the pack or index at p: its last 32 bytes (the
// checksum, 20 bytes with sha1 and 32 with sha256, inside them), hex; ""
// when it cannot be read.
func colaDe(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() < 32 {
		return ""
	}
	b := make([]byte, 32)
	if _, err := f.ReadAt(b, fi.Size()-32); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// mismoContenido says whether the object file p, which changed on disk since
// o was photographed and is now fi, still holds the same bytes: the same size
// and, for a loose object, the same hash of its raw bytes (any byte counts,
// also after the zlib stream; nothing is inflated); for a pack or index, the
// end photographed, which is still the checksum (sha256 when the repository
// says so, else sha1) of everything before it. What it cannot read, or what
// would pass pr's bytes, has not the same bytes. errVencido when pr's time
// passes.
func mismoContenido(p string, o objeto, fi fs.FileInfo, sha256Repo bool, pr *presupuesto) (bool, error) {
	if fi.Size() != o.size {
		return false, nil
	}
	if fi.Size() > pr.bytes {
		return false, nil
	}
	if esPackOIdx(p) {
		if o.cola == "" || colaDe(p) != o.cola {
			return false, nil
		}
		return sumaCorrecta(p, fi.Size(), sha256Repo, pr)
	}
	if o.huella == "" {
		return false, nil
	}
	h, err := huellaCruda(p, fi.Size(), pr)
	if err != nil {
		return false, err
	}
	return h != "" && h == o.huella, nil
}

// sumaCorrecta says whether the trailer of the pack or index at p (size
// bytes) is the checksum of everything before it, sha256 when sha256Repo,
// else sha1. Its bytes come out of pr; errVencido when pr's time passes.
func sumaCorrecta(p string, size int64, sha256Repo bool, pr *presupuesto) (bool, error) {
	h := sha1.New() // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1 -- el checksum de un pack de un repositorio sha1 es su sha1: se compara con el, no se firma nada
	if sha256Repo {
		h = sha256.New()
	}
	n := int64(h.Size())
	if size < n {
		return false, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return false, nil
	}
	defer f.Close()
	if err := copiar(h, f, size-n, pr); err != nil {
		if errors.Is(err, errVencido) {
			return false, err
		}
		return false, nil
	}
	cola := make([]byte, n)
	if _, err := io.ReadFull(f, cola); err != nil {
		return false, nil
	}
	return bytes.Equal(h.Sum(nil), cola), nil
}

// huellaCruda is the sha256 of the raw bytes of the file at p (size bytes,
// out of pr), hex; "" when it cannot be read or it is not size bytes long.
// errVencido when pr's time passes.
func huellaCruda(p string, size int64, pr *presupuesto) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", nil
	}
	defer f.Close()
	h := sha256.New()
	if err := copiar(h, f, size, pr); err != nil {
		if errors.Is(err, errVencido) {
			return "", err
		}
		return "", nil
	}
	if n, _ := f.Read(make([]byte, 1)); n > 0 {
		return "", nil // it grew while it was read
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// trozo is how much copiar reads between two looks at the clock.
const trozo = 1 << 20

// copiar copies exactly n bytes of r into w, out of pr's bytes, looking at
// pr's time every trozo: errVencido when it passes; another error when r
// has fewer bytes or fails.
func copiar(w io.Writer, r io.Reader, n int64, pr *presupuesto) error {
	pr.bytes -= n
	for n > 0 {
		if pr.vencido() {
			return errVencido
		}
		c := min(int64(trozo), n)
		if _, err := io.CopyN(w, r, c); err != nil {
			return err
		}
		n -= c
	}
	return nil
}
