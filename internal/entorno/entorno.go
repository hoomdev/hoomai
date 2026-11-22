package entorno

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
	return &Ajuste{}, nil
}

// ConPrefijo says whether gate runs with the template: a has Ejecutar and
// gate is not in EnElHost. false with a nil Ajuste.
func (a *Ajuste) ConPrefijo(gate string) bool {
	return false
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
	return ""
}
