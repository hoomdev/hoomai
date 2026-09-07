// Package isolate builds BLIND trees: sparse git worktrees that hold exactly
// what a role may read, and not one file of implementation.
//
// It exists because a post-run gate can see writes and never reads, and no
// future gate will: a read leaves no trace on the tree. So the anti-circularity
// of the test-writer ("NEVER read the implementation") stops being verified
// and starts being impossible — the file is not on the disk where the role
// runs. What this package DOES verify is the isolation itself: it freezes the
// list of paths git left out (the witnesses) and looks for them on disk
// before and after the run.
//
// The check stats the filesystem instead of asking git on purpose. When a
// skipped file is written back by hand, git clears its SKIP_WORKTREE bit and
// `git status` stays silent as long as the content matches HEAD: the one case
// that matters is the one git does not report.
package isolate

import (
	"bytes"
	"fmt"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// evidenceThatQuotesCode: the two directories under .hoom/ whose artifacts
// carry implementation verbatim — a red verdict quotes it in the gate output,
// a finding quotes it in its description. Writing them from inside a blind
// tree is legitimate work; reading them would make the whole thing a joke.
var evidenceThatQuotesCode = []string{".hoom/verdicts/**", ".hoom/findings/**"}

// Tree is a blind worktree: a sparse checkout of ONE commit holding exactly
// what a role may read.
type Tree struct {
	Dir      string   // .hoom/isolated/<rol>_<id>
	Commit   string   // sha del commit desde el que se armo
	Patterns []string // los patrones que viajaron a git, en orden
	Hidden   []string // rutas rastreadas que git dejo FUERA del arbol: los testigos

	parent string // desde donde se creo el worktree (git lo necesita para cerrarlo)
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Ready reports whether dir can host a blind tree at all. The envelope asks
// BEFORE anything else so it can refuse with its own vocabulary.
func Ready(dir string) error {
	if _, err := run(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("%s no es un repositorio git y el arbol ciego se arma con git worktree. Accion: 'git init' en el proyecto", dir)
	}
	if _, err := run(dir, "rev-parse", "HEAD"); err != nil {
		return fmt.Errorf("el repositorio no tiene ningun commit y el arbol ciego se arma DESDE un commit. Accion: 'git add -A && git commit'")
	}
	return nil
}

// Patterns translates a role's write policy into git sparse-checkout
// patterns. allow and deny travel verbatim — hoom's glob syntax and git's
// agree on every form the roles use, so translating would only add a layer
// where the writer and the reader could disagree, and a disagreement there is
// a hole. markers are anchored with a leading slash: without it `go.mod` also
// matches vendor/<algo>/go.mod.
func Patterns(allow, deny, markers []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p = strings.TrimSpace(p); p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, g := range allow {
		add(g)
	}
	for _, m := range markers {
		m = strings.TrimSpace(m)
		if m != "" && !strings.HasPrefix(m, "/") {
			m = "/" + m
		}
		add(m)
	}
	// Sin las reglas de ignore del proyecto, el arbol ciego lista como
	// candidatos los artefactos de build y ensucia el delta del rol.
	add(".gitignore")
	// Los negativos van AL FINAL: en sparse-checkout el ultimo patron que
	// matchea manda, asi que una exclusion antes de su include no excluye.
	for _, g := range deny {
		if g = strings.TrimSpace(g); g != "" {
			add("!" + g)
		}
	}
	for _, g := range evidenceThatQuotesCode {
		add("!" + g)
	}
	return out
}

// Open builds the blind tree at parent/.hoom/isolated/<name> from parent's
// HEAD and freezes Hidden. It never touches the parent's checkout: the sparse
// configuration lives in .git/worktrees/<name>/info/sparse-checkout.
func Open(parent, name string, patterns []string) (*Tree, error) {
	if err := Ready(parent); err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, fmt.Errorf("un arbol ciego sin patrones no contendria nada: el rol se quedaria sin insumos")
	}
	commit, err := run(parent, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer HEAD de %s: %s", parent, commit)
	}
	if err := ensureIgnored(parent); err != nil {
		return nil, err
	}
	dir := filepath.Join(parent, ".hoom", "isolated", name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("la cuarentena %s ya existe (un run anterior la dejo en pie).\n  Accion: revisala y descartala con 'git worktree remove --force %s'", dir, dir)
	}
	if out, err := run(parent, "worktree", "add", "--no-checkout", "--detach", dir, commit); err != nil {
		return nil, fmt.Errorf("no se pudo crear el worktree del arbol ciego: %s", out)
	}
	t := &Tree{Dir: dir, Commit: commit, Patterns: patterns, parent: parent}
	if out, err := t.arm(); err != nil {
		t.Close()
		return nil, fmt.Errorf("no se pudo aplicar el sparse-checkout del arbol ciego: %s", out)
	}
	hidden, visible, err := t.indexTags()
	if err != nil {
		t.Close()
		return nil, err
	}
	if visible == 0 {
		t.Close()
		return nil, fmt.Errorf("los patrones del rol no dejaron ningun archivo en el arbol ciego (%s).\n  Accion: declara el layout del proyecto en hoom.yaml, en agents.<rol>.write.allow", strings.Join(patterns, ", "))
	}
	t.Hidden = hidden
	return t, nil
}

// ensureIgnored guarantees .hoom/.gitignore hides isolated/, the same way
// taskcmd does for worktrees/: a quarantine must never enter the change
// candidate nor the fingerprint that certifies the real tree.
func ensureIgnored(root string) error {
	_, err := hoomfs.EnsureIgnored(root, "isolated")
	return err
}

// arm writes the sparse patterns and materializes the tree. The patterns
// travel by stdin so one that starts with '-' or '!' is never read as a flag.
func (t *Tree) arm() (string, error) {
	cmd := exec.Command("git", "sparse-checkout", "set", "--no-cone", "--stdin")
	cmd.Dir = t.Dir
	cmd.Stdin = strings.NewReader(strings.Join(t.Patterns, "\n") + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return strings.TrimSpace(string(out)), err
	}
	return run(t.Dir, "checkout")
}

// indexTags splits the index into the paths git left OUT of the tree (tag S:
// skip-worktree) and the count of the ones it materialized. git computes the
// set with its own matcher, so hoom never has to re-implement globbing and
// cannot disagree with it.
func (t *Tree) indexTags() (hidden []string, visible int, err error) {
	cmd := exec.Command("git", "-c", "core.quotePath=false", "ls-files", "-t", "-z")
	cmd.Dir = t.Dir
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("no se pudo leer el indice del arbol ciego: %w", err)
	}
	for _, rec := range strings.Split(string(out), "\x00") {
		tag, path, ok := strings.Cut(rec, " ")
		if !ok || path == "" {
			continue
		}
		if tag == "S" {
			hidden = append(hidden, path)
			continue
		}
		visible++
	}
	sort.Strings(hidden)
	return hidden, visible, nil
}

// Breaches lists the witnesses that are back on disk. Empty = the isolation
// held. The set is the one frozen at Open: widening the patterns afterwards
// does not shrink it, because a proof the subject can rewrite is not a proof.
func (t *Tree) Breaches() []string {
	var out []string
	for _, p := range t.Hidden {
		// Lstat y no Stat: un symlink que apunta al archivo real tambien
		// devuelve la implementacion al alcance de una lectura.
		if _, err := os.Lstat(filepath.Join(t.Dir, filepath.FromSlash(p))); err == nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Apply transplants paths from the blind tree into dest: it creates,
// overwrites and deletes exactly the given list and nothing else. Two phases,
// so a refusal or a failure never leaves dest half-changed: every path is
// validated and every destination inspected BEFORE the first write, and a
// write that fails midway undoes the ones already made.
//
// A destination is only ever a file the blind tree started from: one that
// still matches the commit the tree was built from, or one that does not
// exist there nor on disk. Anything else — a local edit, an untracked file, a
// local deletion, a symlink anywhere in the path — is a conflict the
// transplant refuses by name, because overwriting it would destroy work hoom
// never certified.
func (t *Tree) Apply(dest string, paths []string) (applied, removed []string, err error) {
	type move struct {
		rel, src, dst string
		raw           []byte
		mode          os.FileMode
		delete        bool   // absent in the blind tree: the role deleted it
		existed       bool   // dst present before the move
		prev          []byte // dst content before the move, to undo
		prevMode      os.FileMode
	}
	var moves []move
	for _, p := range paths {
		src, serr := safeJoin(t.Dir, p)
		if serr != nil {
			return nil, nil, serr
		}
		dst, derr := safeJoin(dest, p)
		if derr != nil {
			return nil, nil, derr
		}
		if st, lerr := os.Lstat(src); lerr == nil && st.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("el trasplante no mueve enlaces simbolicos (%s): un enlace puede apuntar fuera del arbol de trabajo", p)
		}
		if err := noSymlinkUnder(dest, p); err != nil {
			return nil, nil, err
		}
		m := move{rel: p, src: src, dst: dst, mode: 0o644}
		raw, rerr := os.ReadFile(src)
		switch {
		case rerr == nil:
			m.raw = raw
			if st, serr := os.Stat(src); serr == nil {
				m.mode = st.Mode().Perm()
			}
		case os.IsNotExist(rerr):
			m.delete = true
		default:
			return nil, nil, rerr
		}
		base, inBase := t.baseContent(dest, p)
		if st, lerr := os.Lstat(dst); lerr == nil {
			if st.IsDir() {
				return nil, nil, fmt.Errorf("el trasplante no pisa un directorio (%s)", p)
			}
			prev, perr := os.ReadFile(dst)
			if perr != nil {
				return nil, nil, perr
			}
			m.existed, m.prev, m.prevMode = true, prev, st.Mode().Perm()
			if !inBase || !bytes.Equal(prev, base) {
				return nil, nil, conflict(p, t.Commit, "tiene una version distinta de la del commit")
			}
		} else if inBase {
			return nil, nil, conflict(p, t.Commit, "fue borrado localmente respecto del commit")
		}
		if m.delete && !m.existed {
			continue // nada que borrar: ni en el arbol ciego ni en el real
		}
		moves = append(moves, m)
	}

	var done []move
	undo := func() {
		for i := len(done) - 1; i >= 0; i-- {
			m := done[i]
			if m.existed {
				os.WriteFile(m.dst, m.prev, m.prevMode)
			} else {
				os.Remove(m.dst)
			}
		}
	}
	fail := func(m move, err error) error {
		n := len(done)
		undo()
		return fmt.Errorf("el trasplante fallo en %s: %v (se deshicieron los %d cambios anteriores; el arbol real quedo como antes)", m.rel, err, n)
	}
	for _, m := range moves {
		if m.delete {
			if err := os.Remove(m.dst); err != nil {
				return nil, nil, fail(m, err)
			}
			done = append(done, m)
			removed = append(removed, m.rel)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(m.dst), 0o755); err != nil {
			return nil, nil, fail(m, err)
		}
		if err := os.WriteFile(m.dst, m.raw, m.mode); err != nil {
			return nil, nil, fail(m, err)
		}
		done = append(done, m)
		applied = append(applied, m.rel)
	}
	sort.Strings(applied)
	sort.Strings(removed)
	return applied, removed, nil
}

// baseContent returns rel as it was in the commit the blind tree was built
// from, and whether it existed there. Exact bytes: this is a comparison, not
// a display.
func (t *Tree) baseContent(dest, rel string) ([]byte, bool) {
	cmd := exec.Command("git", "show", t.Commit+":"+filepath.ToSlash(rel))
	cmd.Dir = dest
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	return out, true
}

func conflict(rel, commit, why string) error {
	c := commit
	if len(c) > 12 {
		c = c[:12]
	}
	return fmt.Errorf("conflicto en %s: el arbol real %s %s desde el que se armo el arbol ciego (cambio local sin commitear o archivo sin trackear); el trasplante no lo pisa.\n  Accion: commitea o descarta ese cambio y repeti el run", rel, why, c)
}

// noSymlinkUnder refuses a destination that sits at or under a symlink:
// safeJoin is lexical, and a link already present in the real tree would
// carry the write anywhere it points.
func noSymlinkUnder(base, rel string) error {
	cur := base
	for _, c := range strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/") {
		cur = filepath.Join(cur, c)
		st, err := os.Lstat(cur)
		if err != nil {
			return nil // lo que sigue no existe todavia: no hay enlace que seguir
		}
		if st.Mode()&os.ModeSymlink != 0 {
			r, _ := filepath.Rel(base, cur)
			return fmt.Errorf("el trasplante no escribe a traves de un enlace simbolico (%s -> fuera del control del arbol ciego)", r)
		}
	}
	return nil
}

// safeJoin resolves rel under base and refuses anything that would land
// outside it. Copying out of the work tree would be exactly the hole the
// quarantine exists to close.
func safeJoin(base, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("ruta vacia en el trasplante")
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("el trasplante no acepta rutas absolutas (%s)", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("el trasplante no sale del arbol de trabajo (%s)", rel)
	}
	return filepath.Join(base, clean), nil
}

// Close removes the worktree. Always --force: a blind tree that did its job
// is full of untracked tests, and git refuses to remove one without it.
func (t *Tree) Close() error {
	if t == nil || t.Dir == "" {
		return nil
	}
	if out, err := run(t.parent, "worktree", "remove", "--force", t.Dir); err != nil {
		return fmt.Errorf("no se pudo cerrar la cuarentena %s: %s", t.Dir, out)
	}
	return nil
}
