// Package worktreetest arma, SOLO para tests, el proyecto y la matriz
// adversarial de la puerta de la carpeta de una tarea (runcmd.TaskDir,
// hallazgo 20261021T145451_68644c de fixes-cabina-c1b;
// .hoom/specs/items-y-columna-derivada.md, CA-280).
//
// .hoom/worktrees/<slug> es la tarea solo si su .git es un ARCHIVO regular
// hacia <common-dir de la raiz>/worktrees/<id>, el registro <ese
// gitdir>/gitdir nombra exactamente <la carpeta>/.git, git responde desde
// ahi esa carpeta como top y el common-dir de la raiz, su HEAD esta en
// refs/heads/hoom/<slug>, y ninguna variable GIT_* del entorno cambia la
// respuesta. Casos son carpetas que fallan alguna de esas reglas; Armar las
// crea, Proyecto arma el proyecto con la tarea legit de verdad, y Foto junta
// lo que un verbo rechazado no puede tocar. RegistrosAjenos (hallazgo
// 880f0a: el registro nombra otro archivo que existe) y RamasAjenas
// (hallazgos 966474 y c284d0: el worktree registrado fuera de su rama) son
// las variantes de la delta 1 sobre un worktree registrado de verdad.
//
// Existe solo para tests, como net/http/httptest: ningun codigo de
// produccion lo importa. Es un paquete y no un _test.go porque la misma
// matriz la usan agentcmd, itemcmd, reviewcmd, runcmd y taskcmd: antes cada
// uno tenia una copia del bloque (hallazgo 20261021T174702_00e2fc), cinco
// fuentes de verdad que habia que mover juntas. Cada paquete conserva solo su
// operacion propia (el verbo que prueba con cada carpeta).
//
// No importa ninguno de esos paquetes: Proyecto recibe la funcion que crea la
// tarea (taskcmd.Start), asi los tests internos de taskcmd lo usan sin ciclo.
package worktreetest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// Caso es una carpeta .hoom/worktrees/<Slug> que NO es la tarea <Slug>; Que
// dice por que.
type Caso struct{ Slug, Que string }

// Casos: carpetas en .hoom/worktrees/<slug> que NO son la tarea <slug>.
// Cada una falla alguna regla de la puerta: .git archivo regular hacia
// <common-dir>/worktrees/<id>, backpointer <gitdir>/gitdir = <carpeta>/.git,
// top y common-dir de la raiz desde adentro, y HEAD en refs/heads/hoom/<slug>.
var Casos = []Caso{
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

// SlugsConPlana: los slugs de Casos y, al final, "plana" (la carpeta de
// Plana, que se prueba con su entorno GIT_*).
func SlugsConPlana() []string {
	slugs := []string{}
	for _, c := range Casos {
		slugs = append(slugs, c.Slug)
	}
	return append(slugs, "plana")
}

// Git corre git en dir y devuelve su salida sin espacios al borde; si falla,
// corta el test.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (en %s): %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Escribir escribe dir/rel con body, creando lo que falte del camino.
func Escribir(t testing.TB, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Copiar copia a dst los archivos rastreados de src, tal cual.
func Copiar(t testing.TB, src, dst string) {
	t.Helper()
	for _, rel := range strings.Split(Git(t, src, "ls-files"), "\n") {
		raw, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		Escribir(t, dst, rel, string(raw))
	}
}

// CpR corre cp -R src dst.
func CpR(t testing.TB, src, dst string) {
	t.Helper()
	if out, err := exec.Command("cp", "-R", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("cp -R %s %s: %v\n%s", src, dst, err, out)
	}
}

// Symlink crea el enlace hacia destino, creando lo que falte del camino.
func Symlink(t testing.TB, destino, enlace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Fatal(err)
	}
}

// Start crea la tarea slug desde base, como taskcmd.Start.
type Start func(root, slug, base string) error

// Proyecto: base/proyecto, un proyecto hoom (hoom.yaml, .hoom/.gitignore,
// app.go) con la tarea legit creada con start (rama hoom/legit) y un commit
// propio en ella.
func Proyecto(t testing.TB, start Start) (root, legit string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "proyecto")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	Git(t, root, "init", "-q", "-b", "main")
	Git(t, root, "config", "user.email", "test@hoom.dev")
	Git(t, root, "config", "user.name", "hoom test")
	Escribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	Escribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	Escribir(t, root, "app.go", "package app\n")
	Git(t, root, "add", "-A")
	Git(t, root, "commit", "-q", "-m", "inicial")
	if err := start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}
	legit = filepath.Join(root, ".hoom", "worktrees", "legit")
	Escribir(t, legit, "legit.go", "package app\n\nfunc DeLegit() {}\n")
	Git(t, legit, "add", "-A")
	Git(t, legit, "commit", "-q", "-m", "trabajo de legit")
	return root, legit
}

// Armar crea en root la carpeta del caso slug. conTrabajo le deja cambios
// propios sin commitear (app.go distinto y <slug>.go nuevo); sin trabajo
// queda LIMPIA vista por git desde adentro (una copia del arbol al que
// apunta), para que nada que no sea la puerta la proteja.
func Armar(t testing.TB, root, slug string, conTrabajo bool) {
	t.Helper()
	wts := filepath.Join(root, ".hoom", "worktrees")
	dir := filepath.Join(wts, slug)
	legit := filepath.Join(wts, "legit")
	gitdirLegit := filepath.Join(root, ".git", "worktrees", "legit")
	gitArchivo := func(destino string) { Escribir(t, dir, ".git", "gitdir: "+destino+"\n") }
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
		Git(t, root, "worktree", "add", "-q", "-b", "hoom/reg", dir, "main")
		gitArchivo(gitdirLegit)
		origen = legit
	case "copia":
		CpR(t, legit, dir)
	case "copia-rama":
		fuente := filepath.Join(filepath.Dir(root), "fuente-copia-rama")
		Git(t, root, "worktree", "add", "-q", "-b", "hoom/copia-rama", fuente, "main")
		CpR(t, fuente, dir)
	case "movido":
		antes := filepath.Join(wts, "antes-de-mover")
		Git(t, root, "worktree", "add", "-q", "-b", "hoom/movido", antes, "main")
		if err := os.Rename(antes, dir); err != nil {
			t.Fatal(err)
		}
	case "dirgit":
		Git(t, root, "branch", "hoom/dirgit", "main")
		Escribir(t, dir, ".git/commondir", filepath.Join(root, ".git")+"\n")
		Escribir(t, dir, ".git/HEAD", "ref: refs/heads/hoom/dirgit\n")
		Escribir(t, dir, ".git/gitdir", filepath.Join(dir, ".git")+"\n")
		Git(t, dir, "reset", "-q", "--hard")
		if top := Git(t, dir, "rev-parse", "--show-toplevel"); top != dir {
			t.Fatalf("fixture: desde dirgit git responde dirgit como top, responde %s", top)
		}
		if c := Git(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir"); c != filepath.Join(root, ".git") {
			t.Fatalf("fixture: desde dirgit git responde el common-dir de la raiz, responde %s", c)
		}
	case "gitlink":
		Symlink(t, filepath.Join(legit, ".git"), filepath.Join(dir, ".git"))
		origen = legit
	case "gitlink-dir":
		Symlink(t, gitdirLegit, filepath.Join(dir, ".git"))
		origen = legit
	case "otro":
		Git(t, root, "worktree", "add", "-q", "-b", "hoom/otro", dir, "main")
		Git(t, dir, "checkout", "-q", "--ignore-other-worktrees", "hoom/legit")
	default:
		t.Fatalf("fixture: caso desconocido %q", slug)
	}
	if conTrabajo {
		Escribir(t, dir, "app.go", "package app\n\n// escrito desde "+slug+"\n")
		Escribir(t, dir, slug+".go", "package app\n\n// archivo de "+slug+"\n")
	} else if origen != "" {
		Copiar(t, origen, dir)
	}
}

// Archivos lista los archivos bajo dir con su contenido, sin entrar a un
// directorio .git ni a los de saltar; un .git archivo se lista y un symlink
// se anota con su destino.
func Archivos(t testing.TB, dir string, saltar ...string) string {
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

// Foto junta lo que un verbo rechazado no puede tocar: todas las ramas, el
// registro de worktrees, HEAD, indice y status de la raiz (fuera de .hoom,
// donde viven runs, sobres y candados) y de legit, los archivos de la raiz
// fuera de .hoom y los de cada carpeta de .hoom/worktrees.
func Foto(t testing.TB, root string) string {
	t.Helper()
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	var b strings.Builder
	sec := func(nombre, s string) {
		for _, l := range strings.Split(s, "\n") {
			fmt.Fprintf(&b, "[%s] %s\n", nombre, l)
		}
	}
	sec("ramas", Git(t, root, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"))
	sec("worktrees", Git(t, root, "worktree", "list", "--porcelain"))
	sec("raiz HEAD", Git(t, root, "symbolic-ref", "HEAD")+" "+Git(t, root, "rev-parse", "HEAD"))
	sec("raiz indice", Git(t, root, "ls-files", "-s"))
	sec("raiz status", Git(t, root, "status", "--porcelain", "--untracked-files=all", "--", ".", ":(exclude).hoom"))
	if _, err := os.Lstat(legit); err == nil {
		sec("legit HEAD", Git(t, legit, "symbolic-ref", "HEAD")+" "+Git(t, legit, "rev-parse", "HEAD"))
		sec("legit indice", Git(t, legit, "ls-files", "-s"))
		sec("legit status", Git(t, legit, "status", "--porcelain", "--untracked-files=all"))
	} else {
		sec("legit", "NO EXISTE")
	}
	sec("raiz archivos", Archivos(t, root, ".hoom"))
	sec("worktrees archivos", Archivos(t, filepath.Join(root, ".hoom", "worktrees")))
	return b.String()
}

// Cambios: las lineas que se fueron (-) y las que aparecieron (+).
func Cambios(antes, despues string) string {
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

// ConEntorno corre f con las variables vars puestas y las deja como estaban
// (tambien si f corta el test).
func ConEntorno(vars map[string]string, f func()) {
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

// Plana crea .hoom/worktrees/plana, una carpeta comun sin .git, y devuelve
// el entorno que la haria pasar por un worktree de la raiz si la puerta lo
// leyera: GIT_DIR = el .git de la raiz.
func Plana(t testing.TB, root string) map[string]string {
	t.Helper()
	dir := filepath.Join(root, ".hoom", "worktrees", "plana")
	Escribir(t, dir, "app.go", "package app\n\n// escrito desde plana\n")
	Escribir(t, dir, "plana.go", "package app\n")
	return map[string]string{"GIT_DIR": filepath.Join(root, ".git")}
}

// Registro devuelve la ruta del registro (el backpointer) de la carpeta
// dir: <gitdir>/gitdir, con el gitdir que nombra su archivo .git.
func Registro(t testing.TB, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		t.Fatalf("fixture: el archivo .git de %s: %v", dir, err)
	}
	linea := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(linea, "gitdir:") {
		t.Fatalf("fixture: el .git de %s no es un archivo gitdir: %q", dir, linea)
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(linea, "gitdir:"))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}
	return filepath.Join(gitdir, "gitdir")
}

// RegistroAjeno es un registro editado que nombra un archivo que EXISTE y
// no es <carpeta>/.git (hallazgo 20261021T173532_880f0a). Armar lo deja
// listo en la carpeta dir y devuelve la ruta que va al registro.
type RegistroAjeno struct {
	Nombre string
	Armar  func(t testing.TB, dir string) string
}

// RegistrosAjenos: otro nombre en la misma carpeta, el .git de una
// subcarpeta, .GIT y <carpeta>/s/../.git con s un symlink hacia afuera.
// Cada archivo lleva el mismo contenido que el .git de la carpeta. En un
// disco que no distingue mayusculas (macOS), .GIT ya existe: es el mismo
// .git, y aun asi no es el nombre que el registro debe tener.
var RegistrosAjenos = []RegistroAjeno{
	{"otro nombre en la carpeta", func(t testing.TB, dir string) string {
		return copiaDelGit(t, dir, "no-es-dot-git")
	}},
	{".git de una subcarpeta", func(t testing.TB, dir string) string {
		return copiaDelGit(t, dir, filepath.Join("sub", ".git"))
	}},
	{".GIT", func(t testing.TB, dir string) string {
		return copiaDelGit(t, dir, ".GIT")
	}},
	{"un registro que sube por un symlink hacia afuera", registroPorSymlink},
}

// copiaDelGit deja dir/rel con el contenido del .git de dir (si rel ya
// existe, lo deja como esta) y devuelve su ruta.
func copiaDelGit(t testing.TB, dir, rel string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if _, err := os.Lstat(p); err == nil {
		return p
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	Escribir(t, dir, rel, string(raw))
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture: %s existe: %v", p, err)
	}
	return p
}

// RamaAjena deja el worktree registrado dir de la tarea slug fuera de su
// rama hoom/<slug> (hallazgos 20261021T174140_966474 y _c284d0): sigue
// registrado en su lugar, pero su HEAD no esta en refs/heads/hoom/<slug>.
type RamaAjena struct {
	Nombre string
	Armar  func(t testing.TB, dir, slug string)
}

// RamasAjenas: HEAD separado, una rama nueva cualquiera, main (la rama de la
// raiz) y hoom/<slug>-viejo (empieza como la suya y no es la suya).
var RamasAjenas = []RamaAjena{
	{"HEAD separado", func(t testing.TB, dir, _ string) {
		Git(t, dir, "checkout", "-q", "--detach")
	}},
	{"otra rama nueva", func(t testing.TB, dir, _ string) {
		Git(t, dir, "checkout", "-q", "-b", "otra-cosa")
	}},
	{"main, la rama de la raiz", func(t testing.TB, dir, _ string) {
		Git(t, dir, "checkout", "-q", "--ignore-other-worktrees", "main")
	}},
	{"una rama que empieza como la suya", func(t testing.TB, dir, slug string) {
		Git(t, dir, "checkout", "-q", "-b", "hoom/"+slug+"-viejo")
	}},
}

// VolverASuRama devuelve el worktree dir de la tarea slug a hoom/<slug>.
func VolverASuRama(t testing.TB, dir, slug string) {
	t.Helper()
	Git(t, dir, "checkout", "-q", "--ignore-other-worktrees", "hoom/"+slug)
}

// registroPorSymlink deja en dir el symlink s hacia afuera/dentro, con
// afuera/.git (copia del .git de dir) al lado, y devuelve <dir>/s/../.git
// tal cual, sin limpiar: como texto, filepath.Clean lo vuelve <dir>/.git;
// en el disco, s/.. es afuera y el archivo es afuera/.git (hallazgo
// 880f0a, el camino que vio el refutador en f12cbb0).
func registroPorSymlink(t testing.TB, dir string) string {
	t.Helper()
	afuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(afuera, "dentro"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	Escribir(t, afuera, ".git", string(raw))
	Symlink(t, filepath.Join(afuera, "dentro"), filepath.Join(dir, "s"))
	sep := string(filepath.Separator)
	p := dir + sep + "s" + sep + ".." + sep + ".git"
	if filepath.Clean(p) != filepath.Join(dir, ".git") {
		t.Fatalf("fixture: como texto %s es %s", p, filepath.Join(dir, ".git"))
	}
	if real, err := filepath.EvalSymlinks(p); err != nil || real != filepath.Join(afuera, ".git") {
		t.Fatalf("fixture: en el disco %s es %s, resolvio %q (%v)", p, filepath.Join(afuera, ".git"), real, err)
	}
	return p
}

// ModosCerrados: los permisos con que se cierra una carpeta para que no se
// pueda entrar a lo que tiene. 0600 deja listar los nombres pero no
// alcanzarlos (sin permiso de busqueda); 0000 no deja ni listarlos.
var ModosCerrados = []os.FileMode{0o600, 0o000}

// Cerrar deja dir con modo y devuelve con que abrirla otra vez (0755); al
// terminar el test se abre sola. Saltea si corre como root o si el sistema
// deja alcanzar dir/hijo igual: el test no probaria nada (hallazgos 48eeed
// y 44ed99).
func Cerrar(t testing.TB, dir, hijo string, modo os.FileMode) (abrir func()) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita el acceso, el test no probaria nada")
	}
	if err := os.Chmod(dir, modo); err != nil {
		t.Fatal(err)
	}
	abrir = func() {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Errorf("no se pudo abrir %s otra vez: %v", dir, err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.Lstat(filepath.Join(dir, hijo)); err == nil {
		t.Skip("este sistema deja alcanzar lo que hay en una carpeta sin permiso de busqueda")
	}
	return abrir
}
