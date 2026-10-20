// Tests adversariales del hallazgo 20261020T201503_28c350 (high, risk) de la
// review de los fixes de C1 (.hoom/specs/items-y-columna-derivada.md: CA-280,
// la carpeta de la tarea es el worktree .hoom/worktrees/<slug>; CA-290, task
// done --force), sobre los verbos que descartan y cierran (CA-346 de
// .hoom/specs/acciones-desde-la-tarjeta.md). Escritos desde el contrato
// "puerta completa", nunca desde la implementacion:
//
// runcmd.TaskDir es la unica puerta que decide la carpeta de una tarea, y
// Done (con y sin force), Discardable, Discard y RunDiscard pasan por ella.
// Con .hoom/worktrees/<slug> como symlink (a otro repo, a la raiz, a otra
// tarea), con .hoom o .hoom/worktrees como symlink, o con una carpeta que no
// es un worktree registrado de ESTE repo (carpeta comun, clon, worktree de
// otro repo), fallan y no tocan nada: el otro repo, la raiz y el worktree
// real conservan su trabajo sin commitear, las ramas y los registros de
// worktree quedan igual. Guarda: el worktree real de Start sigue andando.
//
// Cada slug hostil se prueba con un entorno NUEVO para descartar y otro para
// cerrar: un verbo que hoy rompe la victima no esconde lo que hace el otro.
// Repos git reales, todos bajo t.TempDir (las victimas tambien).
package taskcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

const (
	t28Legit         = "legit"
	t28NombreVictima = "victima-28c350"
)

// t28Contenidos son los archivos sin guardar que ningun verbo rechazado
// puede listar: los de la victima, los de la raiz y los de la tarea real.
var t28Contenidos = []string{"tracked.go", "untracked.go", "raiz-nuevo.go", "wip.txt"}

func t28Symlink(t *testing.T, destino, enlace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Fatal(err)
	}
}

// t28Arbol es el estado de un arbol: HEAD y status si es un repo, y el
// contenido de cada archivo (fuera de .git y de .hoom/cache, que guarda
// candados descartables; los symlinks se anotan con su destino, no se
// siguen).
func t28Arbol(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		b.WriteString(cbGit(t, dir, "rev-parse", "HEAD") + "\n")
		b.WriteString(cbGit(t, dir, "status", "--porcelain", "--untracked-files=all") + "\n--\n")
	}
	var lineas []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil // el .git de un worktree es un archivo
		}
		if d.IsDir() && filepath.ToSlash(rel) == ".hoom/cache" {
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			dest, _ := os.Readlink(p)
			lineas = append(lineas, fmt.Sprintf("%s -> %s", rel, dest))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		raw, _ := os.ReadFile(p)
		lineas = append(lineas, fmt.Sprintf("%s=%q", rel, raw))
		return nil
	})
	sort.Strings(lineas)
	b.WriteString(strings.Join(lineas, "\n"))
	return b.String()
}

// t28Entorno: un proyecto (root) con su tarea legit de Start y trabajo sin
// commitear en la raiz y en la tarea, y una victima con trabajo ajeno sin
// commitear. legit es la tarea real que hay que preservar (en los entornos
// de .hoom symlink, el worktree registrado al que se llega por el symlink).
// extra son otros arboles a fotografiar (clones, otros repos).
type t28Entorno struct {
	base, root, legit, victima string
	extra                      []string
}

func (e t28Entorno) wt(slug string) string { return filepath.Join(e.root, ".hoom", "worktrees", slug) }

func t28Repo(t *testing.T, dir string, conHoom bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cbGit(t, dir, "init", "-q", "-b", "main")
	cbGit(t, dir, "config", "user.email", "test@hoom.dev")
	cbGit(t, dir, "config", "user.name", "hoom test")
	cbEscribir(t, dir, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	if conHoom {
		cbEscribir(t, dir, ".hoom/.gitignore", hoomfs.GitignoreBody())
	}
	cbEscribir(t, dir, "app.go", "package app\n")
	cbGit(t, dir, "add", "-A")
	cbGit(t, dir, "commit", "-q", "-m", "inicial")
}

// t28Ensuciar deja trabajo sin commitear en dir: app.go modificado y un
// archivo nuevo.
func t28Ensuciar(t *testing.T, dir, nuevo string) {
	t.Helper()
	cbEscribir(t, dir, "app.go", "package app\n\n// trabajo sin guardar en "+filepath.Base(dir)+"\n")
	cbEscribir(t, dir, nuevo, "trabajo nuevo sin guardar\n")
}

func t28Proyecto(t *testing.T) t28Entorno {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	t28Repo(t, root, true)
	if err := Start(root, t28Legit, "main"); err != nil {
		t.Fatalf("fixture: task start %s: %v", t28Legit, err)
	}
	e := t28Entorno{base: base, root: root, legit: filepath.Join(root, ".hoom", "worktrees", t28Legit),
		victima: filepath.Join(base, t28NombreVictima)}
	t28Ensuciar(t, e.legit, "wip.txt")
	t28Ensuciar(t, e.root, "raiz-nuevo.go")
	h1Victima(t, e.victima)
	return e
}

// foto junta todo lo que un verbo rechazado no puede tocar: la victima, la
// raiz (con lo que hay bajo .hoom/worktrees), la tarea real, los arboles
// extra, las ramas y los registros de worktree de los repos involucrados.
func (e t28Entorno) foto(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	seccion := func(nombre, foto string) {
		for _, l := range strings.Split(foto, "\n") {
			fmt.Fprintf(&b, "[%s] %s\n", nombre, l)
		}
	}
	seccion("victima", t28Arbol(t, e.victima))
	seccion("raiz", t28Arbol(t, e.root))
	if _, err := os.Lstat(e.legit); err == nil {
		seccion("legit", t28Arbol(t, e.legit))
	} else {
		seccion("legit", "NO EXISTE")
	}
	for _, x := range e.extra {
		seccion(x, t28Arbol(t, x))
		if _, err := os.Stat(filepath.Join(x, ".git")); err == nil {
			seccion(x+" ramas", cbGit(t, x, "branch", "--list", "--format=%(refname)"))
			seccion(x+" worktrees", cbGit(t, x, "worktree", "list", "--porcelain"))
		}
	}
	seccion("ramas", cbGit(t, e.root, "branch", "--list", "--format=%(refname)"))
	seccion("worktrees", cbGit(t, e.root, "worktree", "list", "--porcelain"))
	return b.String()
}

// t28Rechazado exige que el verbo haya fallado sin tocar nada (la foto de
// antes de ESTE verbo) y sin listar archivos ajenos. Devuelve false si el
// verbo rompio algo: los siguientes ya no tendrian una victima intacta.
func t28Rechazado(t *testing.T, ca, verbo, slug string, err error, antes, despues, salida string) bool {
	t.Helper()
	sano := true
	if despues != antes {
		t.Errorf("%s: hallazgo 28c350: %s con %q toco algo que no es la tarea:\n%s\nsalida:\n%s", ca, verbo, slug, h1Cambios(antes, despues), salida)
		sano = false
	}
	if err == nil {
		t.Errorf("%s: hallazgo 28c350: %s con %q debe fallar (la carpeta no es un worktree de la tarea), fue nil\nsalida:\n%s", ca, verbo, slug, salida)
	}
	for _, nombre := range t28Contenidos {
		if strings.Contains(salida, nombre) {
			t.Errorf("%s: hallazgo 28c350: %s con %q no lista archivos ajenos (%s):\n%s", ca, verbo, slug, nombre, salida)
			break
		}
	}
	return sano
}

// t28Descartar corre los verbos de descarte con slug sobre un entorno nuevo
// y exige que cada uno falle sin tocar nada; corta en el primero que rompe.
func t28Descartar(t *testing.T, e t28Entorno, slug string) {
	t.Helper()
	antes := e.foto(t)
	lista, err := Discardable(e.root, slug)
	if !t28Rechazado(t, "CA-346", "Discardable", slug, err, antes, e.foto(t), strings.Join(lista, "\n")) {
		return
	}

	var out bytes.Buffer
	err = RunDiscard(e.root, slug, false, false, &out)
	if err != nil {
		out.WriteString("\nerror: " + err.Error())
	}
	if !t28Rechazado(t, "CA-346", "RunDiscard sin --yes", slug, err, antes, e.foto(t), out.String()) {
		return
	}

	pref := ".hoom/worktrees/" + slug + "/"
	for _, expect := range [][]string{nil, lista, {pref + "tracked.go", pref + "untracked.go"}, {pref + "app.go", pref + "raiz-nuevo.go"}, {pref + "app.go", pref + "wip.txt"}} {
		res, err := Discard(e.root, slug, expect)
		if !t28Rechazado(t, "CA-346", fmt.Sprintf("Discard (expect %v)", expect), slug, err, antes, e.foto(t), fmt.Sprintf("%+v", res)) {
			return
		}
	}

	for _, asJSON := range []bool{false, true} {
		out.Reset()
		err = RunDiscard(e.root, slug, true, asJSON, &out)
		if !t28Rechazado(t, "CA-346", fmt.Sprintf("RunDiscard --yes (json=%v)", asJSON), slug, err, antes, e.foto(t), out.String()) {
			return
		}
	}
}

// t28Cerrar corre Done (con force primero: es el que quita sin mirar) sobre
// un entorno nuevo y exige que falle sin tocar nada.
func t28Cerrar(t *testing.T, e t28Entorno, slug string) {
	t.Helper()
	antes := e.foto(t)
	for _, force := range []bool{true, false} {
		err := Done(e.root, slug, "main", force)
		if !t28Rechazado(t, "CA-290", fmt.Sprintf("Done(force=%v)", force), slug, err, antes, e.foto(t), "") {
			return
		}
	}
}

// t28Hostiles prueba cada slug con un entorno nuevo para descartar y otro
// para cerrar.
func t28Hostiles(t *testing.T, armar func(t *testing.T) t28Entorno, slugs ...string) {
	t.Helper()
	for _, slug := range slugs {
		t.Run(slug+"/descartar", func(t *testing.T) { t28Descartar(t, armar(t), slug) })
		t.Run(slug+"/cerrar", func(t *testing.T) { t28Cerrar(t, armar(t), slug) })
	}
}

// t28Guarda: el worktree real de Start sigue andando aunque al lado haya
// symlinks o carpetas hostiles. Discardable lista su trabajo sin guardar,
// Discard lo vuelve a HEAD sin tocar la raiz ni la victima, y Done --force
// lo quita (la rama queda).
func t28Guarda(t *testing.T, e t28Entorno) {
	t.Helper()
	victima := t28Arbol(t, e.victima)
	antesRaiz := cbGit(t, e.root, "status", "--porcelain", "--untracked-files=all")
	want := []string{".hoom/worktrees/" + t28Legit + "/app.go", ".hoom/worktrees/" + t28Legit + "/wip.txt"}
	lista, err := Discardable(e.root, t28Legit)
	if err != nil || !cbMismos(lista, want) {
		t.Fatalf("CA-346: guarda 28c350: Discardable de la tarea real lista su trabajo %v, dio %v, %v", want, lista, err)
	}
	if _, err := Discard(e.root, t28Legit, lista); err != nil {
		t.Fatalf("CA-346: guarda 28c350: Discard de la tarea real funciona: %v", err)
	}
	if st := cbGit(t, e.legit, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("CA-346: guarda 28c350: despues de Discard la tarea real queda en HEAD:\n%s", st)
	}
	if ahora := cbGit(t, e.root, "status", "--porcelain", "--untracked-files=all"); ahora != antesRaiz {
		t.Fatalf("CA-346: guarda 28c350: Discard de la tarea real no toca la raiz:\nantes:\n%s\ndespues:\n%s", antesRaiz, ahora)
	}
	if t28Arbol(t, e.victima) != victima {
		t.Fatalf("CA-346: guarda 28c350: Discard de la tarea real no toca la victima")
	}
	if err := Done(e.root, t28Legit, "main", true); err != nil {
		t.Fatalf("CA-290: guarda 28c350: Done --force de la tarea real funciona: %v", err)
	}
	if _, err := os.Stat(e.legit); !os.IsNotExist(err) {
		t.Fatalf("CA-290: guarda 28c350: Done --force quita el worktree real: %v", err)
	}
	if strings.Contains(cbGit(t, e.root, "worktree", "list", "--porcelain"), e.legit) {
		t.Fatalf("CA-290: guarda 28c350: Done --force deja el worktree desregistrado")
	}
	if cbGit(t, e.root, "branch", "--list", "hoom/"+t28Legit) == "" {
		t.Fatalf("CA-290: guarda 28c350: Done --force deja la rama hoom/%s", t28Legit)
	}
}

// Hallazgo 28c350 (high). CA-346 y CA-290: .hoom/worktrees/<slug> como
// symlink a OTRO repo con trabajo sin commitear (por ruta absoluta y
// relativa): Discardable, RunDiscard, Discard y Done (con y sin force)
// fallan y el otro repo conserva su archivo modificado y su archivo nuevo.
func TestHallazgo28c350_CA346_SymlinkAOtroRepoNoSeDescartaNiSeCierra(t *testing.T) {
	armar := func(t *testing.T) t28Entorno {
		e := t28Proyecto(t)
		t28Symlink(t, e.victima, e.wt("a-otro-repo"))
		t28Symlink(t, "../../../"+t28NombreVictima, e.wt("a-otro-repo-rel"))
		return e
	}
	t28Hostiles(t, armar, "a-otro-repo", "a-otro-repo-rel")
	t28Guarda(t, armar(t))
}

// Hallazgo 28c350 (high). CA-346 y CA-290: .hoom/worktrees/<slug> como
// symlink a la RAIZ del proyecto (../.. y absoluto, como el que llega
// commiteado con un clon): ningun verbo revierte ni borra el trabajo sin
// commitear de la raiz.
func TestHallazgo28c350_CA346_SymlinkALaRaizNoSeDescartaNiSeCierra(t *testing.T) {
	armar := func(t *testing.T) t28Entorno {
		e := t28Proyecto(t)
		t28Symlink(t, "../..", e.wt("a-la-raiz"))
		t28Symlink(t, e.root, e.wt("a-la-raiz-abs"))
		return e
	}
	t28Hostiles(t, armar, "a-la-raiz", "a-la-raiz-abs")
	t28Guarda(t, armar(t))
}

// Hallazgo 28c350 (high). CA-290: `task done baz --force` con baz -> el
// worktree de OTRA tarea de este repo no quita ese worktree (ni su wip.txt
// sin commitear); Discard tampoco lo toca. Relativo y absoluto.
func TestHallazgo28c350_CA290_SymlinkAOtraTareaNoLaCierra(t *testing.T) {
	armar := func(t *testing.T) t28Entorno {
		e := t28Proyecto(t)
		t28Symlink(t, t28Legit, e.wt("baz"))
		t28Symlink(t, e.legit, e.wt("baz-abs"))
		return e
	}
	t28Hostiles(t, armar, "baz", "baz-abs")
	t28Guarda(t, armar(t))
}

// Hallazgo 28c350 (high). CA-346 y CA-290: una carpeta en
// .hoom/worktrees/<slug> que no es un worktree registrado de ESTE repo no es
// la tarea: una carpeta comun (git ahi adentro ve la raiz), un clon aparte
// con trabajo sin commitear, un worktree de OTRO repo con trabajo sin
// commitear, y una carpeta comun en el lugar de un worktree registrado que
// se borro a mano. Nada se descarta ni se cierra: ni ahi, ni en la raiz.
func TestHallazgo28c350_CA346_CarpetaQueNoEsWorktreeDeEsteRepo(t *testing.T) {
	armar := func(t *testing.T) t28Entorno {
		e := t28Proyecto(t)

		cbEscribir(t, e.wt("comun"), "x.go", "package x\n")

		cbGit(t, e.base, "clone", "-q", e.root, e.wt("clon"))
		cbGit(t, e.wt("clon"), "config", "user.email", "test@hoom.dev")
		cbGit(t, e.wt("clon"), "config", "user.name", "hoom test")
		t28Ensuciar(t, e.wt("clon"), "nuevo-del-clon.go")

		otro := filepath.Join(e.base, "otro")
		t28Repo(t, otro, false)
		cbGit(t, otro, "worktree", "add", "-q", "-b", "hoom/ajeno", e.wt("ajeno"), "main")
		t28Ensuciar(t, e.wt("ajeno"), "nuevo-del-ajeno.go")

		cbGit(t, e.root, "worktree", "add", "-q", "-b", "hoom/viejo", e.wt("viejo"), "main")
		if err := os.RemoveAll(e.wt("viejo")); err != nil {
			t.Fatal(err)
		}
		cbEscribir(t, e.wt("viejo"), "app.go", "package app\n\n// carpeta comun en el lugar del worktree\n")

		e.extra = []string{e.wt("clon"), otro, e.wt("ajeno")}
		return e
	}
	t28Hostiles(t, armar, "comun", "clon", "ajeno", "viejo")
	t28Guarda(t, armar(t))
}

// t28ConWorktreesSymlink arma, junto al entorno de la guarda, un proyecto2
// cuyo .hoom/worktrees es un symlink a base/externo, donde vive foo: un
// worktree REGISTRADO de proyecto2 (creado fuera con git worktree add) con
// trabajo sin commitear.
func t28ConWorktreesSymlink(t *testing.T) t28Entorno {
	e := t28Proyecto(t)
	root := filepath.Join(e.base, "proyecto2")
	t28Repo(t, root, true)
	externo := filepath.Join(e.base, "externo")
	if err := os.MkdirAll(externo, 0o755); err != nil {
		t.Fatal(err)
	}
	cbGit(t, root, "worktree", "add", "-q", "-b", "hoom/foo", filepath.Join(externo, "foo"), "main")
	t28Ensuciar(t, filepath.Join(externo, "foo"), "wip.txt")
	t28Symlink(t, externo, filepath.Join(root, ".hoom", "worktrees"))
	return t28Entorno{base: e.base, root: root, legit: filepath.Join(externo, "foo"), victima: e.victima}
}

// t28ConHoomSymlink: proyecto3 cuyo .hoom es un symlink a base/otro-hoom,
// con foo en otro-hoom/worktrees: un worktree REGISTRADO de proyecto3 con
// trabajo sin commitear.
func t28ConHoomSymlink(t *testing.T) t28Entorno {
	e := t28Proyecto(t)
	root := filepath.Join(e.base, "proyecto3")
	t28Repo(t, root, false)
	otro := filepath.Join(e.base, "otro-hoom")
	if err := os.MkdirAll(filepath.Join(otro, "worktrees"), 0o755); err != nil {
		t.Fatal(err)
	}
	cbGit(t, root, "worktree", "add", "-q", "-b", "hoom/foo", filepath.Join(otro, "worktrees", "foo"), "main")
	t28Ensuciar(t, filepath.Join(otro, "worktrees", "foo"), "wip.txt")
	t28Symlink(t, otro, filepath.Join(root, ".hoom"))
	return t28Entorno{base: e.base, root: root, legit: filepath.Join(otro, "worktrees", "foo"), victima: e.victima}
}

// Hallazgo 28c350 (high). CA-346 y CA-290: si .hoom/worktrees es un symlink,
// ni siquiera un worktree REGISTRADO de este repo al que se llega por el es
// la carpeta de la tarea: no se descarta ni se quita.
func TestHallazgo28c350_CA346_WorktreesSymlinkNoSeSigue(t *testing.T) {
	t28Hostiles(t, t28ConWorktreesSymlink, "foo")
	t28Guarda(t, t28Proyecto(t))
}

// Hallazgo 28c350 (high). CA-346 y CA-290: si .hoom es un symlink, igual: el
// worktree registrado al que se llega por el no se descarta ni se quita.
func TestHallazgo28c350_CA346_HoomSymlinkNoSeSigue(t *testing.T) {
	t28Hostiles(t, t28ConHoomSymlink, "foo")
	t28Guarda(t, t28Proyecto(t))
}
