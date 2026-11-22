package entorno

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// Ruta is where the local adjustment lives, relative to the main checkout,
// with "/".
const Ruta = ".hoom/local.yaml"

// Claves are the keys .hoom/local.yaml accepts, in the order
// ErrClaveDesconocida lists them.
var Claves = []string{"ejecutar", "en_el_host"}

// The errors of Leer, exactly. The two with a %q take the key, or the gate,
// as the file writes it.
const (
	// ErrClaveDesconocida is a key of the file outside Claves.
	ErrClaveDesconocida = ".hoom/local.yaml: clave desconocida %q (validas: ejecutar, en_el_host)"
	// ErrSinCmd is an ejecutar (not empty) without the mark {cmd}.
	ErrSinCmd = ".hoom/local.yaml: ejecutar tiene que llevar {cmd}"
	// ErrGateDesconocido is an entry of en_el_host that is not among the
	// gates Leer gets (the gates of hoom.yaml).
	ErrGateDesconocido = ".hoom/local.yaml: en_el_host nombra %q, que no es un gate de hoom.yaml"
	// ErrNoIgnorado is a file that the main checkout's git does not ignore:
	// not covered by a .gitignore, or tracked.
	ErrNoIgnorado = ".hoom/local.yaml: git no lo ignora y es de esta maquina: agrega local.yaml a .hoom/.gitignore (y git rm --cached .hoom/local.yaml si esta en git)"
)

// Ajuste is the .hoom/local.yaml of one verified tree, read and validated by
// Leer. The zero Ajuste, and a nil *Ajuste, is "no adjustment": the gates run
// as today.
type Ajuste struct {
	// Principal is the main checkout whose .hoom/local.yaml Leer read: the
	// parent folder of the git common directory of the tree
	// (gitx.CheckoutPrincipal), or the directory Leer got when it is not in
	// a git repository.
	Principal string
	// Existe says that Principal/.hoom/local.yaml exists.
	Existe bool
	// Ejecutar is the template, as the file writes it; "" = without the key
	// (absent, with no value, null, "" or only blanks): the gates run as
	// today.
	Ejecutar string
	// EnElHost are the gates of en_el_host, in the file's order.
	EnElHost []string
	// SHA256 is the hex sha256 of the file's bytes; "" when it does not
	// exist.
	SHA256 string
	// Rel is what {rel} becomes: filepath.Rel(Principal, the tree) with "/":
	// "." when the tree is Principal (and outside a git repository),
	// ".hoom/worktrees/<slug>" in a task's tree, and as it comes out
	// otherwise, even when it starts with "../" or is the project's subpath
	// in the repository.
	Rel string
}

// Leer reads the .hoom/local.yaml of the verified tree dir (the manifest's
// Dir): the main checkout's (Ajuste.Principal), never one inside a worktree,
// and dir's own outside a git repository. gates are the names of the gates
// of hoom.yaml, already merged with its profile (the ones verify runs): every
// entry of en_el_host has to be one of them.
//
// Without the file it is an Ajuste with Existe false and no Ejecutar: as
// today, no error. With it, the file is validated whole, with ejecutar or
// without it, and its error is one of ErrClaveDesconocida, ErrSinCmd,
// ErrGateDesconocido (each with its %q filled) and ErrNoIgnorado, exactly,
// the first in that order when several apply (ErrNoIgnorado never outside a
// git repository), or another error that starts the same way (the path
// .hoom/local.yaml, a colon and a space) for a file it cannot read or
// understand (see the package doc); then verify
// runs no gate and writes no verdict. On a nil error the Ajuste is never
// nil.
func Leer(dir string, gates []string) (*Ajuste, error) {
	principal, enGit := gitx.CheckoutPrincipal(dir)
	if !enGit {
		principal = dir
	}
	a := &Ajuste{Principal: principal, Rel: rel(principal, dir)}
	raw, err := leerArchivo(principal)
	if errors.Is(err, fs.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	a.Existe = true
	sum := sha256.Sum256(raw)
	a.SHA256 = hex.EncodeToString(sum[:])
	if err := a.validar(raw, gates); err != nil {
		return nil, err
	}
	if enGit {
		switch ignorado, err := gitx.Ignora(principal, Ruta); {
		case err != nil:
			return nil, defecto("git no pudo decir si lo ignora: %v", err)
		case !ignorado:
			return nil, errors.New(ErrNoIgnorado)
		}
	}
	return a, nil
}

// maxArchivo is the largest .hoom/local.yaml Leer reads: two keys do not
// need more, and a larger one is refused, never cut.
const maxArchivo = 64 << 10

// defecto is a refusal of the file whose text is not one of the four fixed
// errors: it starts like them.
func defecto(format string, args ...any) error {
	return fmt.Errorf(Ruta+": "+format, args...)
}

// rel is {rel}: the tree dir relative to the main checkout, with "/", "."
// when Rel cannot say it (dir and principal are then the same project).
func rel(principal, dir string) string {
	r, err := filepath.Rel(principal, dir)
	if err != nil {
		return "."
	}
	return filepath.ToSlash(r)
}

// leerArchivo reads principal/.hoom/local.yaml from the disk, never
// following a symlink nor blocking on a FIFO (hoomfs.LeerRegularEn). An
// error that is fs.ErrNotExist is "there is no file"; any other one is
// already a defecto.
func leerArchivo(principal string) ([]byte, error) {
	r, err := os.OpenRoot(principal)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, defecto("no se puede leer: %v", err)
	}
	defer r.Close()
	name := filepath.FromSlash(Ruta)
	fi, err := r.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, err
	case err != nil:
		return nil, defecto("no se puede leer: %v", err)
	case !fi.Mode().IsRegular():
		return nil, defecto("tiene que ser un archivo regular y es %s", fi.Mode().Type())
	}
	raw, err := hoomfs.LeerRegularEn(r, name, maxArchivo)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// it went away between the look and the read: not "no file"
			return nil, defecto("cambio mientras se leia")
		}
		return nil, defecto("no se puede leer: %v", err)
	}
	return raw, nil
}

// validar fills a from the file's bytes raw and checks them, in the order
// of Leer: what cannot be understood, then the four fixed errors but the
// ignore.
func (a *Ajuste) validar(raw []byte, gates []string) error {
	raiz, err := documento(raw)
	if err != nil {
		return err
	}
	var ejecutar, enElHost *yaml.Node
	var desconocida string
	for i := 0; raiz != nil && i+1 < len(raiz.Content); i += 2 {
		k, v := raiz.Content[i], alias(raiz.Content[i+1])
		if k.Kind != yaml.ScalarNode {
			return defecto("cada clave tiene que ser un texto")
		}
		switch k.Value {
		case "ejecutar":
			if ejecutar != nil {
				return defecto("la clave %q esta repetida", k.Value)
			}
			ejecutar = v
		case "en_el_host":
			if enElHost != nil {
				return defecto("la clave %q esta repetida", k.Value)
			}
			enElHost = v
		default:
			if desconocida == "" {
				desconocida = k.Value
			}
		}
	}
	if ejecutar != nil && !nulo(ejecutar) {
		if ejecutar.Kind != yaml.ScalarNode {
			return defecto("ejecutar tiene que ser un texto")
		}
		if strings.TrimSpace(ejecutar.Value) != "" {
			a.Ejecutar = ejecutar.Value
		}
	}
	if enElHost != nil && !nulo(enElHost) {
		if enElHost.Kind != yaml.SequenceNode {
			return defecto("en_el_host tiene que ser una lista de gates")
		}
		for _, g := range enElHost.Content {
			if g = alias(g); g.Kind != yaml.ScalarNode || nulo(g) {
				return defecto("en_el_host tiene que ser una lista de gates")
			}
			a.EnElHost = append(a.EnElHost, g.Value)
		}
	}
	if desconocida != "" {
		return fmt.Errorf(ErrClaveDesconocida, desconocida)
	}
	if a.Ejecutar != "" && !strings.Contains(a.Ejecutar, "{cmd}") {
		return errors.New(ErrSinCmd)
	}
	for _, g := range a.EnElHost {
		if !slices.Contains(gates, g) {
			return fmt.Errorf(ErrGateDesconocido, g)
		}
	}
	return nil
}

// documento is the top-level map of the file's one YAML document; nil for
// an empty file, one with only comments or a null top level.
func documento(raw []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	switch err := dec.Decode(&doc); {
	case errors.Is(err, io.EOF):
		return nil, nil
	case err != nil:
		return nil, defecto("no es YAML valido: %v", err)
	}
	var otro yaml.Node
	if err := dec.Decode(&otro); !errors.Is(err, io.EOF) {
		return nil, defecto("tiene que ser un solo documento YAML")
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	raiz := alias(doc.Content[0])
	switch {
	case nulo(raiz):
		return nil, nil
	case raiz.Kind != yaml.MappingNode:
		return nil, defecto("tiene que ser un mapa de claves (ejecutar, en_el_host)")
	}
	return raiz, nil
}

// alias is the node an alias stands for, or n itself.
func alias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

// nulo says whether n is YAML's null: no value, ~ or null.
func nulo(n *yaml.Node) bool {
	return n == nil || (n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null")
}

// ConPrefijo says whether gate runs with the template: a has Ejecutar and
// gate is not in EnElHost. false with a nil Ajuste.
func (a *Ajuste) ConPrefijo(gate string) bool {
	return a != nil && a.Ejecutar != "" && !slices.Contains(a.EnElHost, gate)
}

// Linea is the line `sh -c` runs for a gate with the template: Ejecutar with
// {cmd} replaced by cmd, {dir} by dir and {rel} by a.Rel, each as one sh word
// between single quotes, with each single quote inside the value written as
// these four characters:
//
//	'\''
//
// For example, with Ejecutar "sh -c {cmd}" and cmd "echo it's", the line is:
//
//	sh -c 'echo it'\''s'
//
// The marks are replaced in one pass: a mark inside cmd, dir or Rel is not
// replaced again. Every occurrence of a mark is replaced. With a nil Ajuste,
// or one without Ejecutar, it is cmd itself: as today.
func (a *Ajuste) Linea(cmd, dir string) string {
	if a == nil || a.Ejecutar == "" {
		return cmd
	}
	return strings.NewReplacer("{cmd}", palabra(cmd), "{dir}", palabra(dir), "{rel}", palabra(a.Rel)).Replace(a.Ejecutar)
}

// palabra is s as one sh word between single quotes.
func palabra(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
