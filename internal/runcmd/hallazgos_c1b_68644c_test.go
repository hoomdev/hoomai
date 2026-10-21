// Tests adversariales del hallazgo 20261021T145451_68644c (high, risk) de la
// review 1 de fixes-cabina-c1b (.hoom/specs/items-y-columna-derivada.md,
// CA-280: la carpeta de la tarea es el worktree .hoom/worktrees/<slug>),
// con la ampliacion del contrato tras el refutador. Escritos desde el
// contrato, nunca desde la implementacion:
//
// runcmd.TaskDir(root, task) es la unica puerta de un slug a una carpeta.
// .hoom/worktrees/<slug> es la tarea solo si, ademas de lo de antes (slug
// valido, sin symlinks en la ruta):
//
//  1. su .git es un ARCHIVO regular (no un directorio ni un symlink) que
//     apunta a un gitdir <common-dir de la raiz>/worktrees/<id>;
//  2. el registro <ese gitdir>/gitdir existe y nombra exactamente
//     <la carpeta>/.git (el backpointer);
//  3. desde la carpeta, git responde esa carpeta como top y el common-dir
//     de la raiz;
//  4. su HEAD esta en refs/heads/hoom/<slug> (si no, un error que dice que
//     la tarea no esta en su rama hoom/<slug>: no "no existe");
//  5. ninguna variable GIT_* del entorno (GIT_DIR, GIT_WORK_TREE,
//     GIT_COMMON_DIR, GIT_INDEX_FILE, GIT_OBJECT_DIRECTORY) cambia la
//     respuesta.
//
// Lo que no cumple falla sin tocar nada: ni legit (su rama, su indice, sus
// archivos), ni la raiz, ni la carpeta. Un run con esa Task no arranca.
// Guarda: los worktrees registrados de verdad siguen andando.
//
// Paquete externo, como los de 28c350.
package runcmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

var t68Start = taskcmd.Start

// t68Entorno: el proyecto con legit (commit propio, app.go modificado y
// wip-legit.txt sin commitear), trabajo sin commitear en la raiz y, si
// todos, cada carpeta de t68Casos con trabajo propio.
func t68Entorno(t *testing.T, todos bool) (root, legit string) {
	t.Helper()
	root, legit = t68Proyecto(t)
	t68Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
	t68Escribir(t, legit, "wip-legit.txt", "sin guardar\n")
	t68Escribir(t, root, "app.go", "package app\n\n// trabajo sin guardar de la raiz\n")
	t68Escribir(t, root, "raiz-nuevo.go", "package app\n")
	if todos {
		for _, c := range t68Casos {
			t68Armar(t, root, c.slug, true)
		}
	}
	return root, legit
}

// t68Acepta exige que la puerta de la carpeta wt (rutas reales).
func t68Acepta(t *testing.T, caso, root, slug, wt string) {
	t.Helper()
	got, err := runcmd.TaskDir(root, slug)
	if err != nil {
		t.Errorf("CA-280: guarda 68644c (%s): TaskDir(%q) de un worktree registrado de verdad funciona: %v", caso, slug, err)
		return
	}
	g, _ := filepath.EvalSymlinks(got)
	w, _ := filepath.EvalSymlinks(wt)
	if g != w {
		t.Errorf("CA-280: guarda 68644c (%s): TaskDir(%q) = %q, la carpeta de la tarea es %q", caso, slug, got, wt)
	}
}

// t68OtroDiceLaRama exige que el error de la tarea otro (registrada, pero en
// la rama de legit) diga que no esta en su rama hoom/otro, y no "no existe".
func t68OtroDiceLaRama(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return // ya lo reporto quien llama
	}
	if m := err.Error(); !strings.Contains(m, "hoom/otro") || !strings.Contains(m, "rama") || strings.Contains(m, "no existe") {
		t.Errorf("CA-280: hallazgo 68644c: el error de otro dice que la tarea no esta en su rama hoom/otro (y no \"no existe\"): %v", err)
	}
}

// Hallazgo 68644c (high). CA-280: ninguna de las carpetas de t68Casos es su
// tarea: TaskDir falla con cada una, tambien con la raiz alcanzada por un
// symlink, y no toca nada. otro dice que no esta en su rama. Guarda: legit
// sigue siendo su tarea.
func TestHallazgo68644c_CA280_TaskDirRechazaLoQueNoEsLaTarea(t *testing.T) {
	root, legit := t68Entorno(t, true)
	enlace := filepath.Join(filepath.Dir(root), "enlace-al-proyecto")
	t68Symlink(t, root, enlace)
	antes := t68Foto(t, root)
	for _, c := range t68Casos {
		for _, r := range []string{root, enlace} {
			got, err := runcmd.TaskDir(r, c.slug)
			if err == nil {
				t.Errorf("CA-280: hallazgo 68644c: TaskDir(%q) por %s debe fallar (%s), devolvio %q", c.slug, r, c.que, got)
			}
			if c.slug == "otro" {
				t68OtroDiceLaRama(t, err)
			}
		}
	}
	if despues := t68Foto(t, root); despues != antes {
		t.Errorf("CA-280: hallazgo 68644c: TaskDir no toca nada, y cambio:\n%s", t68Cambios(antes, despues))
	}
	t68Acepta(t, "legit junto a las carpetas falsas", root, "legit", legit)
}

// Hallazgo 68644c (high), regla 5. CA-280: el entorno no cambia la
// respuesta. Con GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR, GIT_INDEX_FILE o
// GIT_OBJECT_DIRECTORY puestos, una carpeta plana y el worktree de otro repo
// siguen sin ser tareas, y legit sigue siendo la suya.
func TestHallazgo68644c_CA280_TaskDirNoLeeElEntornoGit(t *testing.T) {
	root, legit := t68Entorno(t, false)
	base := filepath.Dir(root)
	t68Plana(t, root)
	otro := filepath.Join(base, "otro")
	if err := os.MkdirAll(otro, 0o755); err != nil {
		t.Fatal(err)
	}
	t68Git(t, otro, "init", "-q", "-b", "main")
	t68Git(t, otro, "-c", "user.email=o@hoom.dev", "-c", "user.name=otro", "commit", "-q", "--allow-empty", "-m", "otro")
	t68Git(t, otro, "worktree", "add", "-q", "-b", "hoom/ajeno", filepath.Join(root, ".hoom", "worktrees", "ajeno"), "main")
	plana := filepath.Join(root, ".hoom", "worktrees", "plana")
	escenarios := []struct {
		nombre string
		vars   map[string]string
	}{
		{"GIT_DIR=.git de la raiz", map[string]string{"GIT_DIR": filepath.Join(root, ".git")}},
		{"GIT_DIR=gitdir de legit", map[string]string{"GIT_DIR": filepath.Join(root, ".git", "worktrees", "legit")}},
		{"GIT_DIR y GIT_WORK_TREE", map[string]string{"GIT_DIR": filepath.Join(root, ".git"), "GIT_WORK_TREE": plana}},
		{"GIT_COMMON_DIR=.git de la raiz", map[string]string{"GIT_COMMON_DIR": filepath.Join(root, ".git")}},
		{"GIT_INDEX_FILE ajeno", map[string]string{"GIT_INDEX_FILE": filepath.Join(base, "otro-indice")}},
		{"GIT_OBJECT_DIRECTORY ajeno", map[string]string{"GIT_OBJECT_DIRECTORY": filepath.Join(base, "otros-objetos")}},
	}
	antes := t68Foto(t, root)
	for _, esc := range escenarios {
		t68ConEntorno(esc.vars, func() {
			for _, slug := range []string{"plana", "ajeno"} {
				if got, err := runcmd.TaskDir(root, slug); err == nil {
					t.Errorf("CA-280: hallazgo 68644c (%s): TaskDir(%q) debe fallar, el entorno no la vuelve una tarea; devolvio %q", esc.nombre, slug, got)
				}
			}
			t68Acepta(t, esc.nombre, root, "legit", legit)
		})
	}
	if despues := t68Foto(t, root); despues != antes {
		t.Errorf("CA-280: hallazgo 68644c: TaskDir no toca nada, y cambio:\n%s", t68Cambios(antes, despues))
	}
}

// t68ProviderQueCommitea pone un `claude` falso que anota su cwd fisico en
// la marca y commitea todo lo que ve: si corre en una carpeta falsa, mueve
// la rama de legit (o main).
func t68ProviderQueCommitea(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	marca := filepath.Join(t.TempDir(), "corrio-en.txt")
	script := "#!/bin/sh\npwd -P >> '" + marca + "'\n" +
		"git add -A >/dev/null 2>&1\n" +
		"git -c user.email=p@hoom.dev -c user.name=provider commit -q -m 'del provider' >/dev/null 2>&1\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return marca
}

// Hallazgo 68644c (high). CA-280: un run con Task = cualquiera de las
// carpetas de t68Casos, o = una carpeta plana con GIT_DIR de la raiz en el
// entorno, no arranca: el provider (que commitea lo que ve) no corre y
// legit, la raiz y las carpetas quedan igual. Guarda: el run de legit corre
// en su worktree.
func TestHallazgo68644c_CA280_RunConCarpetaQueNoEsLaTareaNoArranca(t *testing.T) {
	marca := t68ProviderQueCommitea(t)
	root, legit := t68Entorno(t, true)
	entornoPlana := t68Plana(t, root)
	m := runcmd.NewManager(root)
	intentar := func(slug, que string, vars map[string]string) {
		t.Helper()
		antes := t68Foto(t, root)
		t68ConEntorno(vars, func() {
			run, err := m.Start(runcmd.StartOptions{Provider: "claude", Prompt: "hola", Task: slug})
			if err == nil {
				m.Wait(run.ID)
				t.Errorf("CA-280: hallazgo 68644c: un run con Task %q (%s) no debe arrancar (arranco %s)", slug, que, run.ID)
			}
		})
		if raw, errLeer := os.ReadFile(marca); errLeer == nil {
			t.Errorf("CA-280: hallazgo 68644c: con Task %q el provider no debe correr, corrio en:\n%s", slug, raw)
			os.Remove(marca)
		}
		if despues := t68Foto(t, root); despues != antes {
			t.Errorf("CA-280: hallazgo 68644c: un run con Task %q no toca legit, la raiz ni la carpeta, y cambio:\n%s", slug, t68Cambios(antes, despues))
		}
	}
	for _, c := range t68Casos {
		intentar(c.slug, c.que, nil)
	}
	intentar("plana", "carpeta plana con GIT_DIR de la raiz en el entorno", entornoPlana)

	run, err := m.Start(runcmd.StartOptions{Provider: "claude", Prompt: "hola", Task: "legit"})
	if err != nil {
		t.Fatalf("CA-280: guarda 68644c: el run de legit arranca: %v", err)
	}
	m.Wait(run.ID)
	raw, err := os.ReadFile(marca)
	if err != nil {
		t.Fatalf("CA-280: guarda 68644c: el provider de legit corrio: %v", err)
	}
	real, _ := filepath.EvalSymlinks(legit)
	if got := strings.TrimSpace(string(raw)); got != real {
		t.Fatalf("CA-280: guarda 68644c: el run de legit corre en su worktree (%s), corrio en %s", real, got)
	}
}

// Guarda del hallazgo 68644c. CA-280: con las carpetas falsas al lado, la
// puerta da la carpeta de cada worktree registrado de verdad: legit, una
// tarea nueva, la raiz alcanzada por un symlink (git lista rutas reales: la
// puerta compara rutas, no textos), una tarea creada por el enlace, y una
// creada con worktree.useRelativePaths (git escribe el .git y el registro
// con rutas relativas: siguen nombrando la carpeta).
func TestHallazgo68644c_CA280_GuardaLosRegistradosSiguenAndando(t *testing.T) {
	root, legit := t68Entorno(t, true)
	wt := func(r, slug string) string { return filepath.Join(r, ".hoom", "worktrees", slug) }
	t68Acepta(t, "legit", root, "legit", legit)
	if err := taskcmd.Start(root, "otra", "main"); err != nil {
		t.Fatalf("fixture: task start otra: %v", err)
	}
	t68Acepta(t, "tarea nueva junto a las carpetas falsas", root, "otra", wt(root, "otra"))
	enlace := filepath.Join(filepath.Dir(root), "enlace-al-proyecto")
	t68Symlink(t, root, enlace)
	t68Acepta(t, "raiz por symlink", enlace, "legit", legit)
	if err := taskcmd.Start(enlace, "por-el-enlace", "main"); err != nil {
		t.Fatalf("fixture: task start por el enlace: %v", err)
	}
	t68Acepta(t, "tarea creada por el enlace", enlace, "por-el-enlace", wt(enlace, "por-el-enlace"))
	t68Acepta(t, "tarea creada por el enlace, pedida por la ruta real", root, "por-el-enlace", wt(root, "por-el-enlace"))

	t.Run("rutas relativas", func(t *testing.T) {
		t68Git(t, root, "config", "worktree.useRelativePaths", "true")
		if err := taskcmd.Start(root, "relativa", "main"); err != nil {
			t.Fatalf("fixture: task start relativa: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(root, ".git", "worktrees", "relativa", "gitdir"))
		if err != nil {
			t.Fatalf("fixture: el registro de relativa: %v", err)
		}
		if filepath.IsAbs(strings.TrimSpace(string(raw))) {
			t.Skip("este git no escribe rutas relativas en el registro (worktree.useRelativePaths es de git 2.48)")
		}
		t68Acepta(t, "worktree con rutas relativas", root, "relativa", wt(root, "relativa"))
	})
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
