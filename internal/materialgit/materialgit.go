// Package materialgit photographs the git material of a tree: what git runs
// or reads to decide what runs when it works on that tree (the hooks, its
// config, its attributes and excludes, where its objects come from), and what
// the pilot's save would run (autonomia-entre-firmas, aclaraciones 23 a 29;
// contencion-del-writer, "Mas material"). agentcmd (the rule material-de-git
// of the writer's envelope), verifycmd (around the gates of a verify with
// Options.Sobre), itemcmd (around the pilot's save) and encierro (which keeps
// the pieces that exist read-only) share this one definition. It imports
// nothing of hoom but hoomfs, so any of them can use it.
package materialgit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// The Huellas that are not a content hash. They are the same strings the
// disk floor of agentcmd uses (agentcmd.ProtegidoAusente,
// agentcmd.HuellaNoRegular, agentcmd.HuellaIlegible, agentcmd.HuellaGrande,
// agentcmd.FotoSinTerminar).
const (
	// Ausente is the Huella of a piece that is not there.
	Ausente = "-"
	// NoRegular prefixes the type of an entry that is not a regular file:
	// never followed nor read. A symlink's adds ":" and the sha256 of its
	// target text.
	NoRegular = "no-regular:"
	// Ilegible marks an entry hoom could not open or read.
	Ilegible = "ilegible"
	// Grande prefixes the Huella of a regular file larger than MaxLeido: its
	// size, mode, mtime and ctime, never its content.
	Grande = "grande:"
	// SinTerminar is the one entry of a photograph that did not end within its
	// bound: against any other photograph it counts as changed.
	SinTerminar = "el material de git (su foto no termino a tiempo)"
)

// MaxLeido is the largest regular file of the git material a photograph
// reads; a larger one's Huella is Grande.
const MaxLeido = 1 << 20

// ArchivosGestores are the files of the hook managers at the root of a tree
// that always count as git material (pre-commit and lefthook), and
// DirGestor the directory that counts whole (lefthook's scripts).
var ArchivosGestores = []string{".pre-commit-config.yaml", "lefthook.yml", "lefthook.yaml", ".lefthook.yml", ".lefthook.yaml", "lefthook-local.yml"}

// DirGestor is lefthook's directory at the root of a tree: it counts whole.
const DirGestor = ".lefthook"

// Foto is the git material of a tree: by name (relative to the tree, with
// "/", when inside it; absolute otherwise) the Huella of each piece.
type Foto map[string]string

// Lugar is where a tree's git lives, as git says it.
type Lugar struct {
	// Dir is the tree.
	Dir string
	// Hooks is the effective hooks directory (`git rev-parse --git-path
	// hooks`), absolute.
	Hooks string
	// Comun is the git common directory, absolute.
	Comun string
	// GitDir is the tree's own git directory, absolute (Comun for the main
	// checkout, Comun/worktrees/<name> for a linked worktree).
	GitDir string
}

// ErrNoRepo is the error of Ubicar outside a git repository.
var ErrNoRepo = errors.New("no es un repositorio de git")

// Ubicar asks git where the git of the tree dir lives. ErrNoRepo outside a
// repository; another error when git does not say.
func Ubicar(dir string, limite time.Time) (Lugar, error) {
	out, err := git(dir, limite, "rev-parse", "--path-format=absolute", "--git-path", "hooks", "--git-common-dir", "--git-dir")
	lineas := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if err != nil || len(lineas) != 3 {
		var ee *exec.ExitError
		if errors.As(err, &ee) && strings.Contains(string(ee.Stderr), "not a git repository") {
			return Lugar{}, ErrNoRepo
		}
		if err == nil {
			err = fmt.Errorf("git rev-parse: salida inesperada %q", out)
		}
		return Lugar{}, err
	}
	return Lugar{Dir: dir, Hooks: lineas[0], Comun: lineas[1], GitDir: lineas[2]}, nil
}

// Tomar photographs the git material of the tree dir, read from the disk,
// never followed, a file larger than MaxLeido not read:
//   - the effective hooks directory and each entry right inside it;
//   - the common config, the tree's config.worktree and, of the common
//     directory, info/attributes, info/exclude, objects/info/alternates,
//     objects/info/grafts and shallow (never the rest of objects/info/, where
//     git's maintenance writes commit-graphs);
//   - the tree's .git;
//   - the git config the tree reads outside the repository: each file `git
//     config --list --show-origin` names, except the system's (scope system,
//     or under /etc); always $GIT_CONFIG_GLOBAL, or ~/.gitconfig and
//     $XDG_CONFIG_HOME/git/config, so creating one is a change; the file of
//     core.attributesFile and $XDG_CONFIG_HOME/git/attributes;
//   - the hook managers of the tree: when the effective hooks directory is
//     inside the tree (and not in a git directory nor the tree's root itself),
//     the entries right inside its parent (husky calls .husky/<hook> from
//     .husky/_/); always ArchivosGestores at the tree's root, and DirGestor
//     with everything under it.
//
// A piece that is not there is Ausente. Outside a git repository there is
// none: an empty Foto. When git does not say where it is, the entry "git" is
// Ilegible; when `git config --list` fails, the entry "git config" is. nil
// once limite passes (the caller makes it SinTerminar). Every git it runs
// carries core.fsmonitor=false.
func Tomar(dir string, limite time.Time) Foto {
	foto := Foto{}
	l, err := Ubicar(dir, limite)
	if errors.Is(err, ErrNoRepo) {
		return foto
	}
	if err != nil {
		foto["git"] = Ilegible
		return foto
	}
	rutas, err := l.piezas(limite, true)
	if err != nil {
		foto["git config"] = Ilegible
	}
	for _, p := range rutas {
		if time.Now().After(limite) {
			return nil
		}
		if strings.HasSuffix(p, string(filepath.Separator)+"\x00ilegible") {
			foto[l.Nombre(strings.TrimSuffix(p, "\x00ilegible"))+"/"] = Ilegible
			continue
		}
		foto[l.Nombre(p)] = Huella(p)
	}
	if time.Now().After(limite) {
		return nil
	}
	return foto
}

// Rutas are the absolute paths of the pieces of the git material of l that
// encierro keeps read-only when they exist: the effective hooks directory
// and DirGestor whole, the husky parent directory whole when it counts, and
// each file Tomar photographs (whether it exists or not: the caller mounts
// only the ones that do).
func (l Lugar) Rutas(limite time.Time) ([]string, error) {
	return l.piezas(limite, false)
}

// piezas lists the absolute paths of the pieces of l. With entradas, the
// entries of the hooks directory, of the husky parent and of DirGestor are
// listed one by one (for the photograph); without it, each such directory is
// one path (for a read-only mount). A directory that cannot be read is
// listed as its path plus "/\x00ilegible". The error is the one of `git
// config --list`; the rest is still listed.
func (l Lugar) piezas(limite time.Time, entradas bool) ([]string, error) {
	var rutas []string
	visto := map[string]bool{}
	sumar := func(p string) {
		p = filepath.Clean(p)
		if !visto[p] {
			visto[p] = true
			rutas = append(rutas, p)
		}
	}
	dirEntero := func(d string, recursivo bool) {
		sumar(d)
		if !entradas {
			return
		}
		if !recursivo {
			// followed, as git follows it (a hooks directory may be a symlink)
			if st, err := os.Stat(d); err != nil || !st.IsDir() {
				return
			}
			es, err := os.ReadDir(d)
			if err != nil {
				rutas = append(rutas, d+string(filepath.Separator)+"\x00ilegible")
			}
			for _, e := range es {
				sumar(filepath.Join(d, e.Name()))
			}
			return
		}
		if st, err := os.Lstat(d); err != nil || !st.IsDir() {
			return
		}
		filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				rutas = append(rutas, p+string(filepath.Separator)+"\x00ilegible")
				return nil
			}
			if time.Now().After(limite) {
				return filepath.SkipAll
			}
			sumar(p)
			return nil
		})
	}
	dirEntero(l.Hooks, false)
	sumar(filepath.Join(l.Comun, "config"))
	sumar(filepath.Join(l.GitDir, "config.worktree"))
	for _, p := range []string{"info/attributes", "info/exclude", "objects/info/alternates", "objects/info/grafts", "shallow"} {
		sumar(filepath.Join(l.Comun, filepath.FromSlash(p)))
	}
	sumar(filepath.Join(l.Dir, ".git"))
	if padre := filepath.Dir(l.Hooks); dentro(l.Hooks, l.Dir) && filepath.Clean(padre) != filepath.Clean(l.Dir) &&
		!dentro(l.Hooks, l.Comun) && !dentro(l.Hooks, l.GitDir) && dentro(padre, l.Dir) {
		dirEntero(padre, false)
	}
	for _, f := range ArchivosGestores {
		sumar(filepath.Join(l.Dir, f))
	}
	dirEntero(filepath.Join(l.Dir, DirGestor), true)
	fuera, err := l.configDeFuera(limite)
	for _, p := range fuera {
		sumar(p)
	}
	return rutas, err
}

// configDeFuera lists the files of git config and attributes the tree reads
// that are not the system's: each file origin of `git config --list
// --show-origin --show-scope` (relative ones made absolute against the
// tree), the global config files whether they exist or not, the file of
// core.attributesFile and $XDG_CONFIG_HOME/git/attributes.
func (l Lugar) configDeFuera(limite time.Time) ([]string, error) {
	var out []string
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" && home != "" {
		xdg = filepath.Join(home, ".config")
	}
	if g := os.Getenv("GIT_CONFIG_GLOBAL"); g != "" {
		out = append(out, g)
	} else {
		if home != "" {
			out = append(out, filepath.Join(home, ".gitconfig"))
		}
		if xdg != "" {
			out = append(out, filepath.Join(xdg, "git", "config"))
		}
	}
	if xdg != "" {
		out = append(out, filepath.Join(xdg, "git", "attributes"))
	}
	if b, err := git(l.Dir, limite, "config", "--path", "--get", "core.attributesFile"); err == nil {
		if a := strings.TrimSpace(string(b)); a != "" {
			if !filepath.IsAbs(a) {
				a = filepath.Join(l.Dir, a)
			}
			out = append(out, a)
		}
	}
	lista, err := git(l.Dir, limite, "config", "--list", "--show-origin", "--show-scope", "-z")
	if err != nil {
		return out, err
	}
	campos := bytes.Split(lista, []byte{0})
	for i := 0; i+2 < len(campos); i += 3 {
		scope, origen := string(campos[i]), string(campos[i+1])
		p, ok := strings.CutPrefix(origen, "file:")
		if !ok || scope == "system" || p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(l.Dir, p)
		}
		if p == "/etc" || strings.HasPrefix(filepath.Clean(p), "/etc/") {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// Nombre is how the rule material-de-git names the piece at the absolute
// path p: relative to the tree, with "/", when inside it; absolute
// otherwise.
func (l Lugar) Nombre(p string) string {
	if rel, err := filepath.Rel(l.Dir, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}

// dentro says whether p is d or inside it, lexically.
func dentro(p, d string) bool {
	rel, err := filepath.Rel(d, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Huella is the Huella of the entry at p: Ausente when it is not there,
// Grande for a regular file larger than MaxLeido, a regular file's content
// hash with ":x" when someone can execute it, HuellaEn's for the rest.
func Huella(p string) string {
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Ausente
	}
	if err != nil {
		return Ilegible
	}
	if fi.Mode().IsRegular() && fi.Size() > MaxLeido {
		h := fmt.Sprintf("%s%d:%s:%d", Grande, fi.Size(), fi.Mode(), fi.ModTime().UnixNano())
		if c, ok := CtimeDe(fi); ok {
			h += ":" + strconv.FormatInt(c, 10)
		}
		return h
	}
	r, err := os.OpenRoot(filepath.Dir(p))
	if err != nil {
		return Ilegible
	}
	defer r.Close()
	h := HuellaEn(r, filepath.Base(p), fi)
	if fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
		h += ":x"
	}
	return h
}

// HuellaEn is the Huella of the entry name of r, which fi (its Lstat)
// describes: the sha256 of a regular file's whole content, read only if the
// descriptor shows that same regular file (hoomfs.AbrirRegularEn); for a
// symlink, NoRegular, its type and the sha256 of its target text; for any
// other type, NoRegular and its type; Ilegible when it cannot be read.
func HuellaEn(r *os.Root, name string, fi fs.FileInfo) string {
	switch t := fi.Mode().Type(); {
	case t&fs.ModeSymlink != 0:
		destino, err := r.Readlink(name)
		if err != nil {
			return Ilegible
		}
		sum := sha256.Sum256([]byte(destino))
		return NoRegular + t.String() + ":" + hex.EncodeToString(sum[:])
	case !t.IsRegular():
		return NoRegular + t.String()
	}
	f, err := hoomfs.AbrirRegularEn(r, name, fi)
	if err != nil {
		return Ilegible
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return Ilegible
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Cambios lists, sorted, the names whose entry is not the same in antes and
// despues: changed, appeared or gone. When either photograph is SinTerminar
// (or nil: it did not end) it is SinTerminar alone. nil when none.
func Cambios(antes, despues Foto) []string {
	_, antesSin := antes[SinTerminar]
	_, despuesSin := despues[SinTerminar]
	if antes == nil || despues == nil || antesSin || despuesSin {
		return []string{SinTerminar}
	}
	var out []string
	for p, h := range antes {
		if d, ok := despues[p]; !ok || d != h {
			out = append(out, p)
		}
	}
	for p := range despues {
		if _, ok := antes[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// git runs git in dir with core.fsmonitor=false, untranslated, until limite.
func git(dir string, limite time.Time, args ...string) ([]byte, error) {
	ctx, cancel := context.WithDeadline(context.Background(), limite)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false"}, args...)...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "LC_ALL=C", "LANGUAGE=C")
	return cmd.Output()
}
