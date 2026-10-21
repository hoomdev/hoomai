// Tests adversariales del hallazgo 20261021T145451_68644c (high, risk) de la
// review 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md:
// CA-280, la carpeta de la tarea es el worktree .hoom/worktrees/<slug>;
// CA-290, task done --force), sobre los verbos que descartan y cierran
// (CA-346 de .hoom/specs/acciones-desde-la-tarjeta.md), con la ampliacion
// del contrato tras el refutador. Escritos desde el contrato, nunca desde la
// implementacion:
//
// .hoom/worktrees/<slug> es la tarea solo si su .git es un ARCHIVO regular
// hacia <common-dir de la raiz>/worktrees/<id>, el registro <ese
// gitdir>/gitdir nombra exactamente <la carpeta>/.git, git responde desde
// ahi esa carpeta como top y el common-dir de la raiz, su HEAD esta en
// refs/heads/hoom/<slug>, y ninguna variable GIT_* del entorno cambia la
// respuesta. Con cualquier carpeta de t68Casos (o una carpeta plana con
// GIT_DIR de la raiz en el entorno), Discardable, RunDiscard (con y sin
// --yes), Discard y Done (con --force primero, y sin) fallan y no tocan
// nada: ni legit (su rama, su indice, sus archivos con su trabajo sin
// commitear), ni la raiz (main, su indice, su trabajo sin commitear), ni la
// carpeta. Guarda: legit sigue andando.
//
// Cada caso se prueba con un entorno NUEVO para descartar y otro para
// cerrar. Repos git reales bajo t.TempDir.
package taskcmd

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
)

var t68Start = Start

// t68Entorno: el proyecto con legit (commit propio, app.go modificado y
// wip-legit.txt sin commitear), trabajo sin commitear en la raiz, y la
// carpeta del caso slug (o la plana, que devuelve su entorno GIT_*).
func t68Entorno(t *testing.T, slug string) (root string, vars map[string]string) {
	t.Helper()
	root, legit := t68Proyecto(t)
	t68Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
	t68Escribir(t, legit, "wip-legit.txt", "sin guardar\n")
	t68Escribir(t, root, "app.go", "package app\n\n// trabajo sin guardar de la raiz\n")
	t68Escribir(t, root, "raiz-nuevo.go", "package app\n")
	if slug == "plana" {
		return root, t68Plana(t, root)
	}
	t68Armar(t, root, slug, true)
	return root, nil
}

// t68Rechazado exige que el verbo haya fallado sin tocar nada y sin listar
// el trabajo de legit ni el de la raiz. Devuelve false si el verbo rompio
// algo: los siguientes ya no tendrian un entorno intacto.
func t68Rechazado(t *testing.T, ca, verbo, slug string, err error, antes, despues, salida string) bool {
	t.Helper()
	sano := true
	if despues != antes {
		t.Errorf("%s: hallazgo 68644c: %s con %q toco legit, la raiz o la carpeta:\n%s\nsalida:\n%s", ca, verbo, slug, t68Cambios(antes, despues), salida)
		sano = false
	}
	if err == nil {
		t.Errorf("%s: hallazgo 68644c: %s con %q debe fallar (la carpeta no es la tarea), fue nil\nsalida:\n%s", ca, verbo, slug, salida)
	}
	for _, ajeno := range []string{"wip-legit.txt", "legit.go", "raiz-nuevo.go"} {
		if strings.Contains(salida, ajeno) {
			t.Errorf("%s: hallazgo 68644c: %s con %q no lista archivos de legit ni de la raiz (%s):\n%s", ca, verbo, slug, ajeno, salida)
			break
		}
	}
	return sano
}

func t68Descartar(t *testing.T, root, slug string, vars map[string]string) {
	t.Helper()
	antes := t68Foto(t, root)
	var lista []string
	var err error
	t68ConEntorno(vars, func() { lista, err = Discardable(root, slug) })
	if !t68Rechazado(t, "CA-346", "Discardable", slug, err, antes, t68Foto(t, root), strings.Join(lista, "\n")) {
		return
	}
	var out bytes.Buffer
	t68ConEntorno(vars, func() { err = RunDiscard(root, slug, false, false, &out) })
	if !t68Rechazado(t, "CA-346", "RunDiscard sin --yes", slug, err, antes, t68Foto(t, root), out.String()) {
		return
	}
	pref := ".hoom/worktrees/" + slug + "/"
	for _, expect := range [][]string{lista, nil, {pref + "app.go", pref + slug + ".go"}} {
		var res DiscardResult
		t68ConEntorno(vars, func() { res, err = Discard(root, slug, expect) })
		if !t68Rechazado(t, "CA-346", fmt.Sprintf("Discard (expect %v)", expect), slug, err, antes, t68Foto(t, root), fmt.Sprintf("%+v", res)) {
			return
		}
	}
	for _, asJSON := range []bool{false, true} {
		out.Reset()
		t68ConEntorno(vars, func() { err = RunDiscard(root, slug, true, asJSON, &out) })
		if !t68Rechazado(t, "CA-346", fmt.Sprintf("RunDiscard --yes (json=%v)", asJSON), slug, err, antes, t68Foto(t, root), out.String()) {
			return
		}
	}
}

func t68Cerrar(t *testing.T, root, slug string, vars map[string]string) {
	t.Helper()
	antes := t68Foto(t, root)
	for _, force := range []bool{true, false} {
		var err error
		t68ConEntorno(vars, func() { err = Done(root, slug, "main", force) })
		if !t68Rechazado(t, "CA-290", fmt.Sprintf("Done(force=%v)", force), slug, err, antes, t68Foto(t, root), "") {
			return
		}
	}
}

// Hallazgo 68644c (high). CA-346 y CA-290: con cualquier carpeta de
// t68Casos, y con una carpeta plana con GIT_DIR de la raiz en el entorno,
// task discard y task done (--force incluido) fallan sin tocar legit, la
// raiz ni la carpeta.
func TestHallazgo68644c_CA346_LoQueNoEsLaTareaNoSeDescartaNiSeCierra(t *testing.T) {
	slugs := []string{}
	for _, c := range t68Casos {
		slugs = append(slugs, c.slug)
	}
	slugs = append(slugs, "plana")
	for _, slug := range slugs {
		slug := slug
		t.Run(slug+"/descartar", func(t *testing.T) {
			root, vars := t68Entorno(t, slug)
			t68Descartar(t, root, slug, vars)
		})
		t.Run(slug+"/cerrar", func(t *testing.T) {
			root, vars := t68Entorno(t, slug)
			t68Cerrar(t, root, slug, vars)
		})
	}
}

// Guarda del hallazgo 68644c. CA-346 y CA-290: con todas las carpetas de
// t68Casos al lado, legit sigue andando: Discardable lista su trabajo sin
// guardar, Discard lo vuelve a HEAD sin tocar la raiz ni las carpetas de al
// lado, y Done --force lo quita (la rama queda, con su commit).
func TestHallazgo68644c_CA346_GuardaLegitSigueAndando(t *testing.T) {
	root, _ := t68Entorno(t, "evil")
	for _, c := range t68Casos[1:] {
		t68Armar(t, root, c.slug, true)
	}
	legit := filepath.Join(root, ".hoom", "worktrees", "legit")
	raiz := t68Git(t, root, "status", "--porcelain", "--untracked-files=all")
	vecinas := func() string {
		var b strings.Builder
		for _, c := range t68Casos {
			b.WriteString(t68Archivos(t, filepath.Join(root, ".hoom", "worktrees", c.slug)) + "\n")
		}
		return b.String()
	}
	antesVecinas := vecinas()
	want := []string{".hoom/worktrees/legit/app.go", ".hoom/worktrees/legit/wip-legit.txt"}
	lista, err := Discardable(root, "legit")
	if err != nil || !cbMismos(lista, want) {
		t.Fatalf("CA-346: guarda 68644c: Discardable de legit lista su trabajo %v, dio %v, %v", want, lista, err)
	}
	if _, err := Discard(root, "legit", lista); err != nil {
		t.Fatalf("CA-346: guarda 68644c: Discard de legit funciona: %v", err)
	}
	if st := t68Git(t, legit, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("CA-346: guarda 68644c: despues de Discard legit queda en HEAD:\n%s", st)
	}
	if ahora := t68Git(t, root, "status", "--porcelain", "--untracked-files=all"); ahora != raiz {
		t.Fatalf("CA-346: guarda 68644c: Discard de legit no toca la raiz:\nantes:\n%s\ndespues:\n%s", raiz, ahora)
	}
	if vecinas() != antesVecinas {
		t.Fatalf("CA-346: guarda 68644c: Discard de legit no toca las carpetas de al lado")
	}
	head := t68Git(t, legit, "rev-parse", "HEAD")
	if err := Done(root, "legit", "main", true); err != nil {
		t.Fatalf("CA-290: guarda 68644c: Done --force de legit funciona: %v", err)
	}
	if _, err := os.Stat(legit); !os.IsNotExist(err) {
		t.Fatalf("CA-290: guarda 68644c: Done --force quita el worktree de legit: %v", err)
	}
	if got := t68Git(t, root, "rev-parse", "refs/heads/hoom/legit"); got != head {
		t.Fatalf("CA-290: guarda 68644c: Done --force deja la rama hoom/legit en su commit %s, esta en %s", head, got)
	}
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
