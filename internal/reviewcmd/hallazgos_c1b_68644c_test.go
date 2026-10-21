// Tests adversariales del hallazgo 20261021T145451_68644c (high, risk) de la
// review 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md:
// CA-280, el trabajo de la tarea vive en su worktree .hoom/worktrees/<slug>;
// CA-293, la review con --task), sobre `hoom review --task`, con la
// ampliacion del contrato tras el refutador. Escritos desde el contrato,
// nunca desde la implementacion:
//
// .hoom/worktrees/<slug> es la tarea solo si su .git es un ARCHIVO regular
// hacia <common-dir de la raiz>/worktrees/<id>, el registro <ese
// gitdir>/gitdir nombra exactamente <la carpeta>/.git, git responde desde
// ahi esa carpeta como top y el common-dir de la raiz, su HEAD esta en
// refs/heads/hoom/<slug>, y ninguna variable GIT_* del entorno cambia la
// respuesta. Con cualquier carpeta de t68Casos (o una carpeta plana con
// GIT_DIR en el entorno), la review con --task falla (error, o un Result que
// no es "revisado" con exit distinto de 0), el reviewer no corre y legit
// (rama, indice, archivos), la raiz y la carpeta quedan igual. Guarda: la
// review de legit corre en legit.
//
// Cada carpeta queda LIMPIA vista por git desde adentro (una copia del arbol
// al que apunta): una carpeta sucia la protegeria por otra razon (la review
// revisa solo lo commiteado).
package reviewcmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

var t68Start = taskcmd.Start

// t68Entorno: el proyecto con legit (con su commit: algo que revisar), un
// reviewer falso que deja rastro en log, y la carpeta limpia del caso slug
// (o todas las de t68Casos con slug ""). La plana es una copia de legit sin
// .git, con GIT_DIR = el gitdir de legit en el entorno que devuelve.
func t68Entorno(t *testing.T, slug string) (root, log string, vars map[string]string) {
	t.Helper()
	root, legit := t68Proyecto(t)
	log = htReviewer(t, "")
	switch slug {
	case "":
		for _, c := range t68Casos {
			t68Armar(t, root, c.slug, false)
		}
	case "plana":
		t68Copiar(t, legit, filepath.Join(root, ".hoom", "worktrees", "plana"))
		vars = map[string]string{"GIT_DIR": filepath.Join(root, ".git", "worktrees", "legit")}
	default:
		t68Armar(t, root, slug, false)
		if slug != "evil-inexistente" {
			if st := t68Git(t, filepath.Join(root, ".hoom", "worktrees", slug), "status", "--porcelain", "--untracked-files=all"); st != "" {
				t.Fatalf("fixture: desde %s git ve el arbol limpio, ve:\n%s", slug, st)
			}
		}
	}
	return root, log, vars
}

// Hallazgo 68644c (high). CA-280 y CA-293: `hoom review --task <slug>` con
// cualquier carpeta de t68Casos, o con una carpeta plana y GIT_DIR en el
// entorno, falla, el reviewer no corre y no se toca legit, la raiz ni la
// carpeta.
func TestHallazgo68644c_CA293_ReviewDeLoQueNoEsLaTareaNoCorre(t *testing.T) {
	slugs := []string{}
	for _, c := range t68Casos {
		slugs = append(slugs, c.slug)
	}
	slugs = append(slugs, "plana")
	for _, slug := range slugs {
		slug := slug
		t.Run(slug, func(t *testing.T) {
			root, log, vars := t68Entorno(t, slug)
			antes := t68Foto(t, root)
			var out bytes.Buffer
			var res Result
			var err error
			t68ConEntorno(vars, func() {
				res, err = Run(root, "main", Options{Lens: "risk", Provider: "codex", Task: slug}, &out)
			})
			if err == nil && (res.Status == "revisado" || res.ExitCode == 0) {
				t.Errorf("CA-293: hallazgo 68644c: la review con Task %q debe fallar (la carpeta no es la tarea), termino %q exit %d:\n%s",
					slug, res.Status, res.ExitCode, out.String())
			}
			if raw, errLeer := os.ReadFile(log); errLeer == nil {
				t.Errorf("CA-280: hallazgo 68644c: con Task %q el reviewer no corre, corrio:\n%s", slug, raw)
			}
			if despues := t68Foto(t, root); despues != antes {
				t.Errorf("CA-280: hallazgo 68644c: la review con Task %q toco legit, la raiz o la carpeta:\n%s", slug, t68Cambios(antes, despues))
			}
		})
	}
}

// Guarda del hallazgo 68644c. CA-293: con todas las carpetas de t68Casos al
// lado, la review de legit corre en su worktree y termina revisado.
func TestHallazgo68644c_CA293_GuardaReviewDeLegitSigueAndando(t *testing.T) {
	root, log, _ := t68Entorno(t, "")
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	htRevisar(t, root, Options{Lens: "risk", Provider: "codex", Task: "legit"})
	htTodas(t, htPasadas(t, log), 1, "legit", htReal(t, legit))
}

// ---------------------------------------------------------------------------
// Comun a los archivos de 68644c (el mismo bloque en cada paquete).
// ---------------------------------------------------------------------------

// t68Casos: carpetas en .hoom/worktrees/<slug> que NO son la tarea <slug>.
// Cada una falla alguna regla de la puerta: .git archivo regular hacia
// <common-dir>/worktrees/<id>, backpointer <gitdir>/gitdir = <carpeta>/.git,
// top y common-dir de la raiz desde adentro, y HEAD en refs/heads/hoom/<slug>.
var t68Casos = []struct{ slug, que string }{
	{"evil", "carpeta comun con un archivo .git hacia el gitdir de legit"},
	{"evil-rel", "carpeta comun con un archivo .git hacia el gitdir de legit por ruta relativa"},
	{"evil-inexistente", "carpeta comun con un archivo .git hacia un gitdir que no existe"},
	{"evil-raiz", "carpeta comun con un archivo .git hacia el gitdir de la raiz"},
	{"reg", "worktree registrado cuyo .git se edito para apuntar al gitdir de legit"},
	{"copia", "cp -R del worktree de legit"},
	{"copia-rama", "cp -R de un worktree registrado en otra ruta, con la rama hoom/copia-rama"},
	{"movido", "worktree de la rama hoom/movido movido con mv, sin git worktree move"},
	{"dirgit", ".git directorio con commondir hacia la raiz y HEAD en hoom/dirgit"},
	{"gitlink", ".git symlink al archivo .git de legit"},
	{"gitlink-dir", ".git symlink al gitdir de legit"},
	{"otro", "worktree registrado con git checkout --ignore-other-worktrees hoom/legit"},
}

func t68Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (en %s): %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func t68Escribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// t68Copiar copia a dst los archivos rastreados de src, tal cual.
func t68Copiar(t *testing.T, src, dst string) {
	t.Helper()
	for _, rel := range strings.Split(t68Git(t, src, "ls-files"), "\n") {
		raw, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		t68Escribir(t, dst, rel, string(raw))
	}
}

func t68CpR(t *testing.T, src, dst string) {
	t.Helper()
	if out, err := exec.Command("cp", "-R", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("cp -R %s %s: %v\n%s", src, dst, err, out)
	}
}

func t68Symlink(t *testing.T, destino, enlace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Fatal(err)
	}
}

// t68Proyecto: base/proyecto, un proyecto hoom (hoom.yaml, .hoom/.gitignore,
// app.go) con la tarea legit creada con t68Start (rama hoom/legit) y un
// commit propio en ella.
func t68Proyecto(t *testing.T) (root, legit string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "proyecto")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t68Git(t, root, "init", "-q", "-b", "main")
	t68Git(t, root, "config", "user.email", "test@hoom.dev")
	t68Git(t, root, "config", "user.name", "hoom test")
	t68Escribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	t68Escribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	t68Escribir(t, root, "app.go", "package app\n")
	t68Git(t, root, "add", "-A")
	t68Git(t, root, "commit", "-q", "-m", "inicial")
	if err := t68Start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}
	legit = filepath.Join(root, ".hoom", "worktrees", "legit")
	t68Escribir(t, legit, "legit.go", "package app\n\nfunc DeLegit() {}\n")
	t68Git(t, legit, "add", "-A")
	t68Git(t, legit, "commit", "-q", "-m", "trabajo de legit")
	return root, legit
}

// t68Armar crea en root la carpeta del caso slug. conTrabajo le deja
// cambios propios sin commitear (app.go distinto y <slug>.go nuevo); sin
// trabajo queda LIMPIA vista por git desde adentro (una copia del arbol al
// que apunta), para que nada que no sea la puerta la proteja.
func t68Armar(t *testing.T, root, slug string, conTrabajo bool) {
	t.Helper()
	wts := filepath.Join(root, ".hoom", "worktrees")
	dir := filepath.Join(wts, slug)
	legit := filepath.Join(wts, "legit")
	gitdirLegit := filepath.Join(root, ".git", "worktrees", "legit")
	gitArchivo := func(destino string) { t68Escribir(t, dir, ".git", "gitdir: "+destino+"\n") }
	origen := "" // el arbol que la carpeta copia para quedar limpia
	switch slug {
	case "evil":
		gitArchivo(gitdirLegit)
		origen = legit
	case "evil-rel":
		gitArchivo("../../../.git/worktrees/legit")
		origen = legit
	case "evil-inexistente":
		gitArchivo(filepath.Join(root, ".git", "worktrees", "no-existe"))
	case "evil-raiz":
		gitArchivo(filepath.Join(root, ".git"))
		origen = root
	case "reg":
		t68Git(t, root, "worktree", "add", "-q", "-b", "hoom/reg", dir, "main")
		gitArchivo(gitdirLegit)
		origen = legit
	case "copia":
		t68CpR(t, legit, dir)
	case "copia-rama":
		fuente := filepath.Join(filepath.Dir(root), "fuente-copia-rama")
		t68Git(t, root, "worktree", "add", "-q", "-b", "hoom/copia-rama", fuente, "main")
		t68CpR(t, fuente, dir)
	case "movido":
		antes := filepath.Join(wts, "antes-de-mover")
		t68Git(t, root, "worktree", "add", "-q", "-b", "hoom/movido", antes, "main")
		if err := os.Rename(antes, dir); err != nil {
			t.Fatal(err)
		}
	case "dirgit":
		t68Git(t, root, "branch", "hoom/dirgit", "main")
		t68Escribir(t, dir, ".git/commondir", filepath.Join(root, ".git")+"\n")
		t68Escribir(t, dir, ".git/HEAD", "ref: refs/heads/hoom/dirgit\n")
		t68Escribir(t, dir, ".git/gitdir", filepath.Join(dir, ".git")+"\n")
		t68Git(t, dir, "reset", "-q", "--hard")
		if top := t68Git(t, dir, "rev-parse", "--show-toplevel"); top != dir {
			t.Fatalf("fixture: desde dirgit git responde dirgit como top, responde %s", top)
		}
		if c := t68Git(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir"); c != filepath.Join(root, ".git") {
			t.Fatalf("fixture: desde dirgit git responde el common-dir de la raiz, responde %s", c)
		}
	case "gitlink":
		t68Symlink(t, filepath.Join(legit, ".git"), filepath.Join(dir, ".git"))
		origen = legit
	case "gitlink-dir":
		t68Symlink(t, gitdirLegit, filepath.Join(dir, ".git"))
		origen = legit
	case "otro":
		t68Git(t, root, "worktree", "add", "-q", "-b", "hoom/otro", dir, "main")
		t68Git(t, dir, "checkout", "-q", "--ignore-other-worktrees", "hoom/legit")
	default:
		t.Fatalf("fixture: caso desconocido %q", slug)
	}
	if conTrabajo {
		t68Escribir(t, dir, "app.go", "package app\n\n// escrito desde "+slug+"\n")
		t68Escribir(t, dir, slug+".go", "package app\n\n// archivo de "+slug+"\n")
	} else if origen != "" {
		t68Copiar(t, origen, dir)
	}
}

// t68Archivos lista los archivos bajo dir con su contenido, sin entrar a un
// directorio .git ni a los de saltar; un .git archivo se lista y un symlink
// se anota con su destino.
func t68Archivos(t *testing.T, dir string, saltar ...string) string {
	t.Helper()
	var lineas []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if d.Type()&os.ModeSymlink != 0 {
			dest, _ := os.Readlink(p)
			lineas = append(lineas, fmt.Sprintf("%s -> %s", rel, dest))
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			for _, s := range saltar {
				if filepath.ToSlash(rel) == s {
					return filepath.SkipDir
				}
			}
			return nil
		}
		raw, _ := os.ReadFile(p)
		lineas = append(lineas, fmt.Sprintf("%s=%q", rel, raw))
		return nil
	})
	sort.Strings(lineas)
	return strings.Join(lineas, "\n")
}

// t68Foto junta lo que un verbo rechazado no puede tocar: todas las ramas,
// el registro de worktrees, HEAD, indice y status de la raiz (fuera de
// .hoom, donde viven runs, sobres y candados) y de legit, los archivos de la
// raiz fuera de .hoom y los de cada carpeta de .hoom/worktrees.
func t68Foto(t *testing.T, root string) string {
	t.Helper()
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	var b strings.Builder
	sec := func(nombre, s string) {
		for _, l := range strings.Split(s, "\n") {
			fmt.Fprintf(&b, "[%s] %s\n", nombre, l)
		}
	}
	sec("ramas", t68Git(t, root, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"))
	sec("worktrees", t68Git(t, root, "worktree", "list", "--porcelain"))
	sec("raiz HEAD", t68Git(t, root, "symbolic-ref", "HEAD")+" "+t68Git(t, root, "rev-parse", "HEAD"))
	sec("raiz indice", t68Git(t, root, "ls-files", "-s"))
	sec("raiz status", t68Git(t, root, "status", "--porcelain", "--untracked-files=all", "--", ".", ":(exclude).hoom"))
	if _, err := os.Lstat(legit); err == nil {
		sec("legit HEAD", t68Git(t, legit, "symbolic-ref", "HEAD")+" "+t68Git(t, legit, "rev-parse", "HEAD"))
		sec("legit indice", t68Git(t, legit, "ls-files", "-s"))
		sec("legit status", t68Git(t, legit, "status", "--porcelain", "--untracked-files=all"))
	} else {
		sec("legit", "NO EXISTE")
	}
	sec("raiz archivos", t68Archivos(t, root, ".hoom"))
	sec("worktrees archivos", t68Archivos(t, filepath.Join(root, ".hoom", "worktrees")))
	return b.String()
}

// t68Cambios: las lineas que se fueron (-) y las que aparecieron (+).
func t68Cambios(antes, despues string) string {
	a, d := map[string]bool{}, map[string]bool{}
	for _, l := range strings.Split(antes, "\n") {
		a[l] = true
	}
	for _, l := range strings.Split(despues, "\n") {
		d[l] = true
	}
	var out []string
	for _, l := range strings.Split(antes, "\n") {
		if !d[l] {
			out = append(out, "- "+l)
		}
	}
	for _, l := range strings.Split(despues, "\n") {
		if !a[l] {
			out = append(out, "+ "+l)
		}
	}
	return strings.Join(out, "\n")
}

// t68ConEntorno corre f con las variables vars puestas y las deja como
// estaban (tambien si f corta el test).
func t68ConEntorno(vars map[string]string, f func()) {
	type previo struct {
		v  string
		ok bool
	}
	antes := map[string]previo{}
	for k, v := range vars {
		pv, ok := os.LookupEnv(k)
		antes[k] = previo{pv, ok}
		os.Setenv(k, v)
	}
	defer func() {
		for k, p := range antes {
			if p.ok {
				os.Setenv(k, p.v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()
	f()
}

// t68Plana crea .hoom/worktrees/plana, una carpeta comun sin .git, y
// devuelve el entorno que la haria pasar por un worktree de la raiz si la
// puerta lo leyera: GIT_DIR = el .git de la raiz.
func t68Plana(t *testing.T, root string) map[string]string {
	t.Helper()
	dir := filepath.Join(root, ".hoom", "worktrees", "plana")
	t68Escribir(t, dir, "app.go", "package app\n\n// escrito desde plana\n")
	t68Escribir(t, dir, "plana.go", "package app\n")
	return map[string]string{"GIT_DIR": filepath.Join(root, ".git")}
}
