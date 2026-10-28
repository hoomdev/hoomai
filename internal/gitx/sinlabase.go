package gitx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Comparacion is what hasta is compared against to see only what a branch
// changed since desde (spec evidencia-de-la-review, finding 547228). When
// the base the branch has integrated at hasta was already in desde, it is
// desde itself. When the branch integrated the base afterwards, it is the
// tree of a synthetic merge of desde with that base: compared with hasta it
// leaves out what the base brought and keeps what the branch did, including
// what a merge commit added beyond the clean merge.
//
// The synthetic merge, and every git that reads its tree, runs in a scratch
// bare repository of its own, in a temporary directory Cerrar removes: the
// repository's objects are only read, as an alternate, and nothing touches
// its objects, refs, index or working tree; the board is read-only. That
// scratch repository has no configuration and no attributes of its own, the
// global and system ones are off, and no GIT_* variable hoom inherited
// reaches it: no .gitattributes of the branch, no $GIT_DIR/info/attributes,
// no merge.default nor merge.<driver> of the repository can pick a program,
// so the merge runs none (as the diff runs no textconv: finding 43bde9;
// findings be631b, fec6f2, 85d5b8).
type Comparacion struct {
	Antes     string // desde, or the synthetic merge's tree; "" when the merge does not come apart
	Integrada string // the base integrated after desde; "" when desde already had it
	// When the merge does not come apart from the change, one of these says
	// why: the first conflicted path, or the version of a git older than the
	// floor (gitMinimo).
	Conflicto string
	Version   string
	repo      string   // the scratch repository Antes lives in; "" = the caller's
	env       []string // the environment of a git that runs in repo
}

// Separada says whether Antes can be compared: the base did not need to be
// taken out, or it came apart cleanly.
func (c *Comparacion) Separada() bool { return c.Antes != "" }

// Cerrar removes the synthetic merge's objects. It is safe on any
// Comparacion, and more than once.
func (c *Comparacion) Cerrar() {
	if c != nil && c.repo != "" {
		os.RemoveAll(c.repo)
		c.repo = ""
	}
}

// donde is where a git that reads Antes runs: the scratch repository, or
// dir when Antes is a commit of dir.
func (c *Comparacion) donde(dir string) string {
	if c.repo != "" {
		return c.repo
	}
	return dir
}

// Cambios is CambiosEntre from Antes to hasta: the files and lines the
// branch changed since desde, without what the integrated base brought.
func (c *Comparacion) Cambios(dir, hasta string) (files []string, ins, del int, err error) {
	if !c.Separada() {
		return nil, 0, 0, errors.New("el merge de la base no se separo del cambio")
	}
	return cambiosEntre(c.donde(dir), c.env, c.Antes, hasta)
}

// gitMinimo is the oldest git hoom builds the synthetic merge with: the
// floor the spec sets (merge-tree --write-tree -z --name-only is 2.38).
var gitMinimo = [2]int{2, 40}

// SinLaBase is the Comparacion of desde..hasta for a branch whose integrated
// base (the merge-base of the base with hasta) is integrada; all three are
// full shas. When integrada is hasta, the base already contains the whole
// change (a range already merged, reviewed from the base: CA-424): the
// branch integrated nothing and desde is compared as it is. A conflict or a
// git too old is not an error: the Comparacion says it and the caller
// reviews the whole change. Any other git failure is an error, never a fall
// back to the raw range.
func SinLaBase(dir, desde, hasta, integrada string) (*Comparacion, error) {
	if integrada == hasta {
		return &Comparacion{Antes: desde}, nil
	}
	ya, err := EsAncestro(dir, integrada, desde)
	if err != nil {
		return nil, err
	}
	if ya {
		return &Comparacion{Antes: desde}, nil
	}
	c := &Comparacion{Integrada: integrada}
	v, err := versionDeGit(dir)
	if err != nil {
		return nil, err
	}
	if !alMenos(v, gitMinimo) {
		c.Version = v
		return c, nil
	}
	if c.repo, err = repoDeTrabajo(dir); err != nil {
		return nil, err
	}
	c.env = aislado(c.repo)
	arbol, conflicto, err := mergeSintetico(c.repo, c.env, desde, integrada)
	if err != nil {
		c.Cerrar()
		return nil, err
	}
	if conflicto != "" {
		c.Cerrar()
		c.Conflicto = conflicto
		return c, nil
	}
	c.Antes = arbol
	return c, nil
}

// repoDeTrabajo creates the scratch bare repository for dir: a temporary
// directory with the same object format, no templates (no hooks, no
// info/attributes), whose objects/info/alternates is dir's object directory.
func repoDeTrabajo(dir string) (string, error) {
	objetos, err := gitOut(dir, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-path objects: %v", err)
	}
	objetos = strings.TrimRight(objetos, "\r\n")
	if strings.ContainsAny(objetos, "\r\n") {
		return "", fmt.Errorf("el directorio de objetos %q no cabe en objects/info/alternates", objetos)
	}
	formato, err := gitOut(dir, "rev-parse", "--show-object-format")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-object-format: %v", err)
	}
	repo, err := os.MkdirTemp("", "hoom-merge-")
	if err != nil {
		return "", err
	}
	if _, err := gitOutEnv(repo, ambienteAislado(), "-c", "init.defaultBranch=hoom", "init", "--bare", "--quiet",
		"--template=", "--object-format="+strings.TrimSpace(formato), repo); err != nil {
		os.RemoveAll(repo)
		return "", fmt.Errorf("git init del repositorio de trabajo: %v", err)
	}
	info := filepath.Join(repo, "objects", "info")
	err = os.MkdirAll(info, 0o700)
	if err == nil {
		err = os.WriteFile(filepath.Join(info, "alternates"), []byte(objetos+"\n"), 0o600)
	}
	if err != nil {
		os.RemoveAll(repo)
		return "", err
	}
	return repo, nil
}

// ambienteAislado is hoom's environment minus every GIT_* it inherited (a
// GIT_DIR, GIT_INDEX_FILE or GIT_CONFIG_PARAMETERS of a hook that runs hoom
// would point git elsewhere or add configuration), with the global and
// system configuration and the system attributes off.
func ambienteAislado() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_ATTR_NOSYSTEM=1",
		"GIT_OPTIONAL_LOCKS=0")
}

// aislado is ambienteAislado for a git that runs in the scratch repository.
func aislado(repo string) []string {
	return append(ambienteAislado(), "GIT_DIR="+repo)
}

// mergeSintetico runs `git merge-tree --write-tree` of a and b in the
// scratch repository, with no attributes file: the tree it wrote, or the
// first conflicted path when git says there are conflicts (exit 1).
func mergeSintetico(repo string, env []string, a, b string) (arbol, conflicto string, err error) {
	cmd := exec.Command("git", "-c", "core.attributesFile="+os.DevNull,
		"merge-tree", "--write-tree", "-z", "--name-only", "--no-messages", a, b)
	cmd.Dir = repo
	cmd.Env = env
	stderr := &capped{n: stderrMax}
	cmd.Stderr = stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	// "<tree>\0<conflicted path>\0...": only the first two fields are kept,
	// the rest is drained so git's exit code is the one that decides
	out, rerr := io.ReadAll(io.LimitReader(pipe, 64<<10))
	if rerr == nil {
		_, rerr = io.Copy(io.Discard, pipe)
	}
	werr := cmd.Wait()
	if rerr != nil {
		return "", "", rerr
	}
	campos := strings.SplitN(string(out), "\x00", 3)
	var exit *exec.ExitError
	switch {
	case werr == nil && len(campos) >= 1 && esOID(campos[0]):
		return campos[0], "", nil
	case errors.As(werr, &exit) && exit.ExitCode() == 1 && len(campos) >= 2 && campos[1] != "":
		return "", campos[1], nil
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return "", "", fmt.Errorf("git merge-tree %s %s: %s", a, b, msg)
	}
	if werr != nil {
		return "", "", fmt.Errorf("git merge-tree %s %s: %v", a, b, werr)
	}
	return "", "", fmt.Errorf("git merge-tree %s %s: salida inesperada", a, b)
}

// versionDeGit is the version git reports ("2.54.0" of "git version 2.54.0
// (Apple Git-157)").
func versionDeGit(dir string) (string, error) {
	out, err := gitOutEnv(dir, soloLectura(), "version")
	if err != nil {
		return "", fmt.Errorf("git version: %v", err)
	}
	campos := strings.Fields(out)
	if len(campos) < 3 || campos[0] != "git" || campos[1] != "version" {
		return "", fmt.Errorf("git version: salida inesperada %q", strings.TrimSpace(out))
	}
	return campos[2], nil
}

// alMenos says whether version v ("2.39.5", "2.45.1.windows.1") is at least
// min (major, minor). A version it cannot read is not.
func alMenos(v string, min [2]int) bool {
	partes := strings.SplitN(v, ".", 3)
	if len(partes) < 2 {
		return false
	}
	mayor, err1 := strconv.Atoi(partes[0])
	menor, err2 := strconv.Atoi(partes[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return mayor > min[0] || (mayor == min[0] && menor >= min[1])
}

// esOID says whether s is a full object id (sha1 or sha256), in hex.
func esOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// soloLectura is hoom's environment for a git that only reads: without
// optional locks, so a git status does not rewrite the index to refresh it.
func soloLectura() []string {
	return append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
}
