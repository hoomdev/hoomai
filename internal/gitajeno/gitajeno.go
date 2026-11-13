// Package gitajeno photographs and compares what the rule git-ajeno of
// contencion-del-writer looks at in the git common directory of a tree: the
// refs under refs/replace/ (loose and packed) and the object files that
// already existed (loose objects, packs and their indexes). agentcmd
// (RuleGitAjeno, FotoGitAjeno, CambiosGitAjeno), verifycmd (around the gates
// of a verify with Sobre) and itemcmd (around the pilot's save) use it.
//
// Every photograph and every comparison has a bound of time and of bytes it
// hashes: passing the time counts as changed (SinTerminar), and it never
// hangs. It runs no git that reads the working tree (no status, no
// fsmonitor, no filter).
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
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
// photographed by its metadata (valorDeRef), so two states never share a
// value.
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

// Tomar photographs, for the tree dir, its git common directory's refs under
// refs/replace/ and the files under objects/ (loose objects, packs and their
// indexes) within tope and maxLeido bytes hashed. Outside a git repository
// there is nothing to compare: the zero Foto. A photograph that passes tope
// is unfinished: Cambios against it gives [SinTerminar]. A loose object
// beyond maxLeido is kept without its hash.
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
// (absolute, the same name whether it is loose or packed), and each loose
// object, pack or index that existed in antes and now has other bytes, by its
// absolute path. Only the files whose ctime, mtime, size or inode changed are
// read. A file it cannot read, or that would pass maxLeido, counts as
// changed, by its path; an unfinished antes, or a comparison that passes
// tope, adds SinTerminar. nil when nothing changed.
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
	case errors.Is(err, errVencido):
		return []string{SinTerminar}
	case err != nil:
		out = append(out, filepath.Join(antes.comun, "refs", "replace")+string(filepath.Separator))
	default:
		for n, v := range antes.refs {
			if ahora[n] != v {
				out = append(out, filepath.Join(antes.comun, filepath.FromSlash(n)))
			}
		}
		for n := range ahora {
			if _, ok := antes.refs[n]; !ok {
				out = append(out, filepath.Join(antes.comun, filepath.FromSlash(n)))
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
// them; with the reftable backend, git for-each-ref (which reads no working
// tree). errVencido when pr's time passes; another error when they cannot be
// read.
func refsDeReemplazo(comun string, pr *presupuesto) (map[string]string, error) {
	refs := map[string]string{}
	if st, err := os.Stat(filepath.Join(comun, "reftable")); err == nil && st.IsDir() {
		out, err := gitEnComun(comun, pr.limite, "for-each-ref", "--format=%(refname) %(objectname)", "refs/replace/")
		if err != nil {
			if pr.vencido() {
				return nil, errVencido
			}
			return nil, err
		}
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if n, v, ok := strings.Cut(l, " "); ok {
				refs[n] = v
			}
		}
		return refs, nil
	}
	if err := packedDeReemplazo(comun, refs, pr); err != nil {
		return nil, err
	}
	base := filepath.Join(comun, "refs", "replace")
	err := filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
		if pr.vencido() {
			return errVencido
		}
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == base {
				return filepath.SkipAll
			}
			return err
		}
		if e.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(comun, p)
		if err != nil {
			return err
		}
		n := filepath.ToSlash(rel)
		if strings.HasSuffix(n, ".lock") {
			return nil // a ref being written: its result is what counts
		}
		fi, err := e.Info()
		if err != nil {
			fi, err = os.Lstat(p)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil // gone while it was walked: as if it never was
		}
		if err != nil {
			return err // fails closed: the whole refs/replace/ counts as changed
		}
		v, err := valorDeRef(p, fi, pr)
		if err != nil {
			return err
		}
		refs[n] = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// maxPackedRefs is the most of packed-refs a photograph reads, and
// maxLineaPacked the longest line: past either, the photograph fails closed
// (it never reads, nor holds, GBs).
const (
	maxPackedRefs  = 256 << 20
	maxLineaPacked = 64 << 10
)

// packedDeReemplazo adds to refs the refs under refs/replace/ of the
// packed-refs of the common directory comun, read line by line (ronda 2,
// d91180): its bytes come out of pr, it looks at pr's time at every line,
// and it holds one line at a time. errVencido when pr's time passes; another
// error (it fails closed) when packed-refs is not a regular file, passes
// maxPackedRefs or pr's bytes, has a line over maxLineaPacked, or cannot be
// read. No packed-refs adds nothing.
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
	tope := min(int64(maxPackedRefs), pr.bytes)
	if fi.Size() > tope {
		return errors.New("gitajeno: packed-refs pasa el tope de bytes")
	}
	r := bufio.NewReaderSize(io.LimitReader(f, tope+1), maxLineaPacked)
	var leidos int64
	defer func() { pr.bytes -= leidos }()
	for {
		if pr.vencido() {
			return errVencido
		}
		linea, err := r.ReadSlice('\n')
		leidos += int64(len(linea))
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			return errors.New("gitajeno: packed-refs tiene una linea que pasa el tope")
		case leidos > tope:
			return errors.New("gitajeno: packed-refs pasa el tope de bytes")
		case err != nil && !errors.Is(err, io.EOF):
			return err
		}
		l := strings.TrimRight(string(linea), "\r\n")
		if l != "" && l[0] != '#' && l[0] != '^' {
			if v, n, ok := strings.Cut(l, " "); ok && strings.HasPrefix(n, "refs/replace/") {
				refs[n] = v
			}
		}
		if err != nil {
			return nil // io.EOF: all of it
		}
	}
}

// maxRef is the size up to which a loose ref is read as its value: a ref is
// one line.
const maxRef = 4096

// topeHuellaRef is how many bytes of a loose ref over maxRef (or of the file
// a symlink there resolves to) its photograph hashes.
const topeHuellaRef = 1 << 20

// valorDeRef is the photograph of the loose ref at p, whose Lstat is fi: what
// git reads of it and, for whatever cannot be read as one line, enough that
// any touch changes it (aclaracion 28: a photograph never joins two
// different states in one value; ronda 2, 61babd and a48bb4):
//   - a regular file of up to maxRef bytes: its content, trimmed (what git
//     takes from it);
//   - a symlink, which git follows: its target, read with readlink, and the
//     photograph of what it resolves to, as follows;
//   - a bigger regular file: its size, inode, mtime and ctime, and the sha256
//     of its first topeHuellaRef bytes (beyond them, any write changes its
//     ctime);
//   - a directory a symlink resolves to: its metadata and that of every
//     entry under it;
//   - what is not a regular file, or cannot be opened or read: its kind and
//     its metadata (ctime included: any touch changes it).
//
// Every value but the first kind's starts with a newline (especial), which a
// trimmed content never does, and carries the lengths of its free parts: no
// two of them, nor one of them and a ref's content, are ever the same string.
// errVencido when pr's time passes.
func valorDeRef(p string, fi fs.FileInfo, pr *presupuesto) (string, error) {
	if fi.Mode()&fs.ModeSymlink == 0 {
		return contenidoDeRef(p, false, pr)
	}
	destino, err := os.Readlink(p)
	if err != nil {
		return especial("enlace-ilegible", metadatos(fi)), nil
	}
	resuelto, err := contenidoDeRef(p, true, pr)
	if err != nil {
		return "", err
	}
	return especial("enlace", destino, resuelto), nil
}

// especial is a value of valorDeRef that is not a ref's content: a newline,
// its kind, and each part with its length.
func especial(tipo string, partes ...string) string {
	var b strings.Builder
	b.WriteString("\n" + tipo)
	for _, x := range partes {
		fmt.Fprintf(&b, ":%d:%s", len(x), x)
	}
	return b.String()
}

// contenidoDeRef is the photograph of the file at p (followed when seguir) as
// valorDeRef describes it. It never blocks on the open (abrir) and reads at
// most topeHuellaRef bytes, out of pr. errVencido when pr's time passes.
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
		case fi.IsDir() && seguir:
			return directorioDeRef(p, fi, pr)
		}
		return especial("ilegible", metadatos(fi)), nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return especial("ilegible", err.Error()), nil
	}
	if fi.IsDir() {
		if seguir {
			return directorioDeRef(p, fi, pr)
		}
		return especial("no-regular", metadatos(fi)), nil
	}
	if !fi.Mode().IsRegular() {
		return especial("no-regular", metadatos(fi)), nil
	}
	if fi.Size() <= maxRef {
		raw, err := io.ReadAll(io.LimitReader(f, maxRef+1))
		if err != nil {
			return especial("ilegible", metadatos(fi)), nil
		}
		if len(raw) <= maxRef {
			return strings.TrimSpace(string(raw)), nil
		}
		// it grew while it was read: a big one
	}
	n := min(fi.Size(), topeHuellaRef)
	huella := "sin-huella" // beyond pr's bytes: its metadata is all there is
	if n <= pr.bytes {
		h := sha256.New()
		switch err := copiar(h, io.NewSectionReader(f, 0, n), n, pr); {
		case errors.Is(err, errVencido):
			return "", err
		case err == nil:
			huella = hex.EncodeToString(h.Sum(nil))
		default:
			huella = "corta" // it shrank while it was read
		}
	}
	return especial("grande", metadatos(fi), huella), nil
}

// directorioDeRef is the photograph of the directory at p (fi its stat),
// where a symlink under refs/replace/ resolves: its metadata and, sorted,
// each entry under it (not followed) with its own. errVencido when pr's time
// passes.
func directorioDeRef(p string, fi fs.FileInfo, pr *presupuesto) (string, error) {
	raiz, err := filepath.EvalSymlinks(p)
	if err != nil {
		return especial("dir", metadatos(fi), "ilegible"), nil
	}
	var entradas []string
	err = filepath.WalkDir(raiz, func(q string, e fs.DirEntry, err error) error {
		if pr.vencido() {
			return errVencido
		}
		if q == raiz {
			return err
		}
		rel, _ := filepath.Rel(raiz, q)
		meta := "ilegible"
		if err == nil {
			if ei, ierr := e.Info(); ierr == nil {
				meta = metadatos(ei)
			}
		}
		entradas = append(entradas, especial("entrada", rel, meta))
		return nil
	})
	if errors.Is(err, errVencido) {
		return "", err
	}
	if err != nil {
		entradas = append(entradas, especial("ilegible", err.Error()))
	}
	sort.Strings(entradas)
	return especial("dir", append([]string{metadatos(fi)}, entradas...)...), nil
}

// metadatos is fi's kind and permissions, size, inode, mtime and ctime (in
// nanoseconds; inode and ctime where the system gives them).
func metadatos(fi fs.FileInfo) string {
	ctime, _ := materialgit.CtimeDe(fi)
	return fmt.Sprintf("%s:%d:%d:%d:%d", fi.Mode(), fi.Size(), materialgit.InodoDe(fi), fi.ModTime().UnixNano(), ctime)
}

// gitEnComun runs git on the common directory comun (GIT_DIR), with
// core.fsmonitor=false, untranslated, until limite.
func gitEnComun(comun string, limite time.Time, args ...string) ([]byte, error) {
	ctx, cancel := context.WithDeadline(context.Background(), limite)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false"}, args...)...)
	cmd.WaitDelay = time.Second
	cmd.Env = append(cmd.Environ(), "GIT_DIR="+comun, "LC_ALL=C", "LANGUAGE=C")
	cmd.Dir = comun
	return cmd.Output()
}

var (
	// objetoSuelto matches objects/<xx>/<rest> of a loose object (sha1 or
	// sha256).
	objetoSuelto = regexp.MustCompile(`^[0-9a-f]{2}/([0-9a-f]{38}|[0-9a-f]{62})$`)
	// packOIdx matches objects/pack/pack-<hash>.pack and .idx.
	packOIdx = regexp.MustCompile(`^pack/pack-[0-9a-f]+\.(pack|idx)$`)
)

// recorrerObjetos calls f for each loose object, pack and index under the
// objects directory of comun (never objects/info/, nor the other files of
// pack/), while pr's time lasts and f says to go on. It says whether it went
// through all of them.
func recorrerObjetos(comun string, pr *presupuesto, f func(p string, fi fs.FileInfo) bool) bool {
	base := filepath.Join(comun, "objects")
	completo := true
	_ = filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
		if pr.vencido() {
			completo = false
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if e.IsDir() {
			if rel == "info" {
				return filepath.SkipDir
			}
			return nil
		}
		if !objetoSuelto.MatchString(rel) && !packOIdx.MatchString(rel) {
			return nil
		}
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			return nil
		}
		if !f(p, fi) {
			completo = false
			return filepath.SkipAll
		}
		return nil
	})
	return completo
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
