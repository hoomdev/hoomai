package agentcmd

import (
	"bufio"
	"bytes"
	"compress/zlib"
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
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/materialgit"
)

// RuleGitAjeno (contencion-del-writer, "Git ajeno"): a role that executes
// changed, in the git common directory of its task, what nobody changes with
// reason while a role runs:
//   - a ref under refs/replace/, created, moved or removed, loose or packed
//     (a replace ref changes what git shows of an object in every branch);
//   - a loose object, a pack or its index that already existed and still
//     exists with another content. Only the files that changed after the
//     photograph are looked at (by their status change time). Creating an
//     object or removing one (gc, prune) does not count: the name of an
//     object and of a pack come from their content.
//
// Moving another branch, refs/stash or any ref outside refs/replace/ is not
// it. It holds for every role that executes (agents.Role.Exec), with
// encierro or without it, in any project, and agents.<rol>.write of
// hoom.yaml cannot lift it. In the envelope it is compared after the run and
// cuts like RuleMaterialDeGit: the envelope closes in scope, no-entregable,
// exit 1, with no verify, its note is NotaGitAjeno(rol), its Violation.Detail
// DetalleGitAjeno and its Violation.Path what changed (as CambiosGitAjeno
// names it), so its finding (high, author AutorGateDeScope, the gate's usual
// format) names it. The review compares it around each pass
// (reviewcmd.PasadaConGitAjeno) and the pilot's save around itself
// (itemcmd.ErrGitAjeno).
const RuleGitAjeno = "git-ajeno"

// DetalleGitAjeno is the Detail of a RuleGitAjeno violation.
const DetalleGitAjeno = "un rol solo cambia la rama de su tarea: las refs de reemplazo y los objetos que ya existian no los toca"

// NotaGitAjeno is the note of an envelope of role rol cut by RuleGitAjeno:
// `el <rol> cambio git fuera de la rama de su tarea: no se emite veredicto sobre este arbol`.
func NotaGitAjeno(rol string) string {
	return "el " + rol + " cambio git fuera de la rama de su tarea: no se emite veredicto sobre este arbol"
}

// DeGitAjeno says whether f is the finding the scope gate registered for a
// RuleGitAjeno violation in a run of role rol (DescripcionViolacion, author
// AutorGateDeScope, severity high), whatever its path. The pilot reads with
// it which envelopes changed git outside their task's branch, and treats
// them as the ones that changed the git material (boardcmd.Sobre).
func DeGitAjeno(f finding.Finding, rol string) bool {
	return deRegla(f, rol, RuleGitAjeno, DetalleGitAjeno)
}

// GitAjeno is the photograph of what RuleGitAjeno compares in the git common
// directory of a tree (FotoGitAjeno): its refs under refs/replace/, loose and
// packed, the object files that existed, and when it was taken.
type GitAjeno struct {
	// comun is the git common directory, absolute; "" = nothing to compare.
	comun string
	// refs are the refs under refs/replace/ by full name -> their value
	// (a loose one over a packed one of the same name).
	refs map[string]string
	// objetos are the loose objects, packs and indexes by absolute path.
	objetos map[string]objetoGit
}

// objetoGit is what the photograph keeps of one object file: enough to know
// whether it changed afterwards without reading it again.
type objetoGit struct {
	ctime, mtime, size int64
	ino                uint64
	// cola is a pack's or an index's trailer (its checksum), hex.
	cola string
}

// topeGitAjeno bounds the git FotoGitAjeno runs (to find the common
// directory, and for-each-ref with reftable).
const topeGitAjeno = 10 * time.Second

// FotoGitAjeno photographs, for the tree dir, its git common directory's
// refs under refs/replace/ (loose and in packed-refs) and the files under
// objects/ (loose objects, packs and their indexes), reading files only: it
// runs no git that reads the working tree (no status, no fsmonitor, no
// filter). Outside a git repository there is nothing to compare: the zero
// GitAjeno, which CambiosGitAjeno never finds changed.
func FotoGitAjeno(dir string) GitAjeno {
	l, err := materialgit.Ubicar(dir, time.Now().Add(topeGitAjeno))
	if err != nil {
		return GitAjeno{}
	}
	foto := GitAjeno{comun: l.Comun, objetos: map[string]objetoGit{}}
	foto.refs, _ = refsDeReemplazo(l.Comun)
	recorrerObjetos(l.Comun, func(p string, fi fs.FileInfo) {
		o := objetoDe(fi)
		if esPackOIdx(p) {
			o.cola = colaDe(p)
		}
		foto.objetos[p] = o
	})
	return foto
}

// CambiosGitAjeno looks again at the git common directory antes photographed
// and lists, sorted, what breaks RuleGitAjeno since then: each ref under
// refs/replace/ created, moved or removed, loose or packed, named
// `<comun>/refs/replace/<nombre>` (absolute, the same name whether it is
// loose or packed), and each loose object, pack or index that existed in antes
// and now has another content, by its absolute path. A file it cannot read
// counts as changed, by its path. nil when nothing changed.
func CambiosGitAjeno(antes GitAjeno) []string {
	if antes.comun == "" {
		return nil
	}
	var out []string
	ahora, err := refsDeReemplazo(antes.comun)
	if err != nil {
		out = append(out, filepath.Join(antes.comun, "refs", "replace")+string(filepath.Separator))
	} else {
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
	for p, o := range antes.objetos {
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed (gc, prune): not a rewrite
		}
		if err != nil || !fi.Mode().IsRegular() {
			out = append(out, p)
			continue
		}
		if objetoDe(fi) == (objetoGit{ctime: o.ctime, mtime: o.mtime, size: o.size, ino: o.ino}) {
			continue // untouched since the photograph
		}
		if !mismoContenido(p, o) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// refsDeReemplazo reads the refs under refs/replace/ of the common directory
// comun: packed-refs, then the loose ones over them; with the reftable
// backend, git for-each-ref (which reads no working tree). An error when they
// cannot be read.
func refsDeReemplazo(comun string) (map[string]string, error) {
	refs := map[string]string{}
	if st, err := os.Stat(filepath.Join(comun, "reftable")); err == nil && st.IsDir() {
		out, err := gitEnComun(comun, "for-each-ref", "--format=%(refname) %(objectname)", "refs/replace/")
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if n, v, ok := strings.Cut(l, " "); ok {
				refs[n] = v
			}
		}
		return refs, nil
	}
	if raw, err := os.ReadFile(filepath.Join(comun, "packed-refs")); err == nil {
		sc := bufio.NewScanner(bytes.NewReader(raw))
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			l := sc.Text()
			if l == "" || l[0] == '#' || l[0] == '^' {
				continue
			}
			if v, n, ok := strings.Cut(l, " "); ok && strings.HasPrefix(n, "refs/replace/") {
				refs[n] = v
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	base := filepath.Join(comun, "refs", "replace")
	err := filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
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
		raw, err := os.ReadFile(p)
		if err != nil {
			refs[n] = HuellaIlegible
			return nil
		}
		refs[n] = strings.TrimSpace(string(raw))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// gitEnComun runs git on the common directory comun (GIT_DIR), with
// core.fsmonitor=false, untranslated, within topeGitAjeno.
func gitEnComun(comun string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), topeGitAjeno)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", sinFsmonitor(args)...)
	cmd.WaitDelay = 5 * time.Second
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
// pack/).
func recorrerObjetos(comun string, f func(p string, fi fs.FileInfo)) {
	base := filepath.Join(comun, "objects")
	_ = filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
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
		f(p, fi)
		return nil
	})
}

// esPackOIdx says whether the object file p is a pack or an index.
func esPackOIdx(p string) bool {
	return strings.HasSuffix(p, ".pack") || strings.HasSuffix(p, ".idx")
}

// objetoDe is what changes in fi when its file is written, replaced or
// chmodded: its ctime (which a user process cannot set back), mtime, size
// and inode.
func objetoDe(fi fs.FileInfo) objetoGit {
	o := objetoGit{mtime: fi.ModTime().UnixNano(), size: fi.Size(), ino: materialgit.InodoDe(fi)}
	if c, ok := materialgit.CtimeDe(fi); ok {
		o.ctime = c
	}
	return o
}

// colaDe is the trailer of the pack or index at p: its last checksum (20
// bytes with sha1, 32 with sha256, told by the file's size and git's
// layout), hex; "" when it cannot be read.
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
// o was photographed, still holds the same content: a loose object whose
// inflated content still hashes to its name; a pack or index whose trailer is
// the one photographed and is the checksum of everything before it.
func mismoContenido(p string, o objetoGit) bool {
	if esPackOIdx(p) {
		return o.cola != "" && colaDe(p) == o.cola && sumaCorrecta(p)
	}
	return sueltoCorrecto(p)
}

// sumaCorrecta says whether the trailer of the pack or index at p is the
// checksum (sha1, or sha256) of everything before it.
func sumaCorrecta(p string) bool {
	for _, h := range []func() hash.Hash{sha1.New, sha256.New} {
		f, err := os.Open(p)
		if err != nil {
			return false
		}
		fi, err := f.Stat()
		n := int64(h().Size())
		if err != nil || fi.Size() < n {
			f.Close()
			return false
		}
		sum := h()
		_, err = io.CopyN(sum, f, fi.Size()-n)
		cola := make([]byte, n)
		if err == nil {
			_, err = io.ReadFull(f, cola)
		}
		f.Close()
		if err == nil && bytes.Equal(sum.Sum(nil), cola) {
			return true
		}
	}
	return false
}

// sueltoCorrecto says whether the loose object at p inflates to content
// whose hash (sha1 for a 40-hex name, sha256 for a 64-hex one) is its name.
func sueltoCorrecto(p string) bool {
	nombre := filepath.Base(filepath.Dir(p)) + filepath.Base(p)
	var h hash.Hash
	switch len(nombre) {
	case 40:
		h = sha1.New()
	case 64:
		h = sha256.New()
	default:
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	z, err := zlib.NewReader(f)
	if err != nil {
		return false
	}
	defer z.Close()
	if _, err := io.Copy(h, z); err != nil {
		return false
	}
	return fmt.Sprintf("%x", h.Sum(nil)) == nombre
}
