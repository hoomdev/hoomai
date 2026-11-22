// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// `hoom task start` (CA-771, enmendado: las rutas relativas de los worktrees
// son opcionales por maquina, con worktrees_relativos en .hoom/local.yaml):
//
//   - con worktrees_relativos: true en el .hoom/local.yaml del principal y
//     un git que acepta --relative-paths (2.48 o mas nuevo), el worktree de
//     la tarea guarda su gitdir con rutas relativas, asi sigue valido en un
//     contenedor que monta el proyecto en otra ruta; y `hoom task list`,
//     `hoom verify` y `hoom task done` lo reconocen como antes. Con un git
//     real mas viejo esos tests se saltan (gitx.AceptaRutasRelativas);
//   - sin la clave, o con false, nulo o vacio, el worktree es el de antes,
//     con cualquier git: un .git con un gitdir absoluto, y el .git/config
//     sin extensions.relativeWorktrees;
//   - con la clave y un git viejo (un git falso primero en el PATH que dice
//     2.47.0), Start se niega con entorno.ErrWorktreesRelativosGitViejo y no
//     crea nada: ni rama, ni carpeta, ni cambios en .hoom/.gitignore;
//   - con un .hoom/local.yaml que verify rechaza, Start se niega con ese
//     mismo error y no crea nada; un root sin hoom.yaml ni .hoom/local.yaml
//     arranca su tarea como antes.
//
// En proceso (taskcmd.Start, gitx.WorktreeDe, Snapshot) y por el binario.
// Escritos desde el spec, las aclaraciones de la ronda 0, el encargo de la
// ronda 0b y el go doc, sin leer la implementacion.
package taskcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	hentorno "github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	hmanifest "github.com/hoomdev/hoomai/internal/manifest"
	hprofiles "github.com/hoomdev/hoomai/internal/profiles"
)

// veeAislarGit deja la config de git del test en un archivo vacio.
func veeAislarGit(t *testing.T) {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// veeGitignore es el .hoom/.gitignore canonico con la linea local.yaml,
// aunque el canonico todavia no la traiga (eso es CA-770): el
// .hoom/local.yaml tiene que estar ignorado, o Leer da ErrNoIgnorado.
func veeGitignore() string {
	body := hoomfs.GitignoreBody()
	for _, l := range strings.Split(body, "\n") {
		if strings.TrimSpace(l) == "local.yaml" {
			return body
		}
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "local.yaml\n"
}

// veeHoomYAML es el hoom.yaml de estos proyectos: un solo gate, test.
const veeHoomYAML = "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

// veeRelativos es el .hoom/local.yaml que pide las rutas relativas.
const veeRelativos = "worktrees_relativos: true\n"

// veeSoloLocal es un .hoom/.gitignore que solo ignora local.yaml: un Start
// que siguiera de largo lo completaria con sus directorios.
const veeSoloLocal = "local.yaml\n"

// veeBase es un proyecto en main con hoomYAML ("" = sin hoom.yaml), el
// .hoom/.gitignore gitignore y app.go, todo commiteado, sin tareas y sin
// .hoom/local.yaml.
func veeBase(t *testing.T, hoomYAML, gitignore string) string {
	t.Helper()
	veeAislarGit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rdGit(t, root, "init", "-q", "-b", "main")
	rdGit(t, root, "config", "user.email", "test@hoom.dev")
	rdGit(t, root, "config", "user.name", "hoom test")
	if hoomYAML != "" {
		rdEscribir(t, root, "hoom.yaml", hoomYAML)
	}
	rdEscribir(t, root, ".hoom/.gitignore", gitignore)
	rdEscribir(t, root, "app.go", "package app\n")
	rdGit(t, root, "add", "-A")
	rdGit(t, root, "commit", "-q", "-m", "inicial")
	return root
}

// veeRepo es un proyecto hoom en main, todo commiteado, sin tareas, con
// worktrees_relativos: true en el .hoom/local.yaml del principal (ignorado
// por git). Se salta con un git que no acepta --relative-paths.
func veeRepo(t *testing.T) string {
	t.Helper()
	root := veeBase(t, veeHoomYAML, veeGitignore())
	rdEscribir(t, root, hentorno.Ruta, veeRelativos)
	if !gitx.AceptaRutasRelativas(root) {
		t.Skip("CA-771: el git de esta maquina no acepta --relative-paths (2.48 o mas nuevo)")
	}
	return root
}

// veeGitViejo pone primero en el PATH un git falso, un script de sh, que
// contesta "git version 2.47.0" a toda llamada que lleva el argumento
// version en cualquier posicion (la de gitx.AceptaRutasRelativas es
// `git -c core.fsmonitor=false version`) y pasa las demas al git real, que
// busca antes de cambiar el PATH. Usa t.Setenv: el test no puede ser
// paralelo.
func veeGitViejo(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("el git falso es un script de sh")
	}
	gitReal, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("fixture: sin git en el PATH: %v", err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		"for a do\n" +
		"  if [ \"$a\" = version ]; then\n" +
		"    echo 'git version 2.47.0'\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"exec '" + strings.ReplaceAll(gitReal, "'", `'\''`) + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if gitx.AceptaRutasRelativas(bin) {
		t.Fatal("fixture: con el git falso, gitx.AceptaRutasRelativas da false")
	}
}

// veeGitdir es la ruta que guarda el archivo p ("gitdir: <ruta>").
func veeGitdir(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("CA-771: leer %s: %v", p, err)
	}
	s := strings.TrimSpace(string(raw))
	return strings.TrimSpace(strings.TrimPrefix(s, "gitdir:"))
}

// veeExigeRelativo exige que el worktree wt de root guarde rutas relativas
// en su .git y en el registro del repo.
func veeExigeRelativo(t *testing.T, root, wt string) {
	t.Helper()
	g := veeGitdir(t, filepath.Join(wt, ".git"))
	if g == "" || filepath.IsAbs(g) || strings.HasPrefix(g, root) {
		t.Fatalf("CA-771: el .git del worktree guarda un gitdir relativo: %q", g)
	}
	reg := filepath.Clean(filepath.Join(wt, g))
	if filepath.Dir(reg) != filepath.Join(root, ".git", "worktrees") {
		t.Fatalf("CA-771: el gitdir relativo apunta al registro del repo: %s", reg)
	}
	if back := veeGitdir(t, filepath.Join(reg, "gitdir")); filepath.IsAbs(back) {
		t.Fatalf("CA-771: el registro del repo tambien guarda una ruta relativa: %q", back)
	}
}

// veeExigeAbsoluto exige el worktree wt de root como antes de esta
// tarjeta: su .git guarda un gitdir absoluto hacia el registro del repo
// (<root>/.git/worktrees/<id>), el registro guarda una ruta absoluta, y el
// config del directorio comun de git no tiene extensions.relativeWorktrees
// en ninguna capitalizacion.
func veeExigeAbsoluto(t *testing.T, caso, root, wt string) {
	t.Helper()
	g := veeGitdir(t, filepath.Join(wt, ".git"))
	if !filepath.IsAbs(g) || filepath.Dir(filepath.Clean(g)) != filepath.Join(root, ".git", "worktrees") {
		t.Fatalf("CA-771: %s: el .git del worktree guarda un gitdir absoluto hacia %s: %q", caso, filepath.Join(root, ".git", "worktrees"), g)
	}
	if back := veeGitdir(t, filepath.Join(g, "gitdir")); !filepath.IsAbs(back) {
		t.Fatalf("CA-771: %s: el registro del repo guarda una ruta absoluta, como antes: %q", caso, back)
	}
	cfg, err := os.ReadFile(filepath.Join(root, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(cfg)), "relativeworktrees") {
		t.Fatalf("CA-771: %s: el .git/config no gana extensions.relativeWorktrees:\n%s", caso, cfg)
	}
}

// veeFoto junta lo que un Start que se niega no puede tocar: las ramas, los
// worktrees que git registra, lo que hay en el arbol (git status con los
// ignorados), el .hoom/.gitignore y el .git/config.
func veeFoto(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(rdGit(t, root, "for-each-ref", "--format=%(refname) %(objectname)"))
	b.WriteString("\n-- worktrees\n")
	b.WriteString(rdGit(t, root, "worktree", "list", "--porcelain"))
	b.WriteString("\n-- status\n")
	b.WriteString(rdGit(t, root, "status", "--porcelain", "--ignored", "--untracked-files=all"))
	for _, p := range []string{".hoom/.gitignore", ".git/config"} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		b.WriteString("\n-- " + p + "\n")
		b.Write(raw)
	}
	return b.String()
}

// veeNadaCreado exige que Start no haya creado nada en root: ni la carpeta
// del worktree de slug, ni la rama hoom/<slug>, y la foto es la de antes.
func veeNadaCreado(t *testing.T, caso, root, slug, antes string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, ".hoom", "worktrees", slug)); err == nil {
		t.Fatalf("CA-771: %s: Start no crea la carpeta del worktree", caso)
	}
	if out := rdGit(t, root, "branch", "--list", "hoom/"+slug); out != "" {
		t.Fatalf("CA-771: %s: Start no crea la rama hoom/%s: %q", caso, slug, out)
	}
	if despues := veeFoto(t, root); despues != antes {
		t.Fatalf("CA-771: %s: Start no toca nada:\n--- antes ---\n%s\n--- despues ---\n%s", caso, antes, despues)
	}
}

// CA-771: con worktrees_relativos: true en el .hoom/local.yaml del
// principal (veeRepo), taskcmd.Start agrega el worktree con rutas
// relativas, y gitx.WorktreeDe y Snapshot lo reconocen.
func TestCA771_StartGuardaRutasRelativas(t *testing.T) {
	root := veeRepo(t)
	if err := Start(root, "demo", "main"); err != nil {
		t.Fatalf("CA-771: hoom task start: %v", err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", "demo")
	veeExigeRelativo(t, root, wt)
	rama, ok, err := gitx.WorktreeDe(root, wt)
	if err != nil || !ok || rama != "refs/heads/hoom/demo" {
		t.Fatalf("CA-771: WorktreeDe reconoce el worktree con rutas relativas: %q %v %v", rama, ok, err)
	}
	if top := rdGit(t, wt, "rev-parse", "--show-toplevel"); top != wt {
		t.Fatalf("CA-771: git responde el worktree como su raiz: %q", top)
	}
	tareas, err := Snapshot(root, "main")
	if err != nil || len(tareas) != 1 || tareas[0].Slug != "demo" || !strings.HasSuffix(tareas[0].Branch, "hoom/demo") {
		t.Fatalf("CA-771: la tarea aparece en la lista: %+v %v", tareas, err)
	}
}

// CA-771 y caso limite "el proyecto montado en otra ruta": con
// worktrees_relativos: true (veeRepo), una copia del proyecto en otra ruta
// (como lo ve un contenedor) tiene un worktree que git sigue reconociendo,
// con el repo de la copia como su comun.
func TestCA771_LaCopiaEnOtraRutaSigueValida(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp -a")
	}
	root := veeRepo(t)
	if err := Start(root, "demo", "main"); err != nil {
		t.Fatal(err)
	}
	otra, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	copia := filepath.Join(otra, "var", "www", "html")
	if err := os.MkdirAll(filepath.Dir(copia), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-a", root, copia).CombinedOutput(); err != nil {
		t.Fatalf("cp -a: %v\n%s", err, out)
	}
	wt := filepath.Join(copia, ".hoom", "worktrees", "demo")
	if top := rdGit(t, wt, "rev-parse", "--show-toplevel"); top != wt {
		t.Fatalf("CA-771: en la copia, git responde el worktree de la copia: %q", top)
	}
	if comun := rdGit(t, wt, "rev-parse", "--path-format=absolute", "--git-common-dir"); comun != filepath.Join(copia, ".git") {
		t.Fatalf("CA-771: en la copia, el comun es el de la copia: %q", comun)
	}
	if rama := rdGit(t, wt, "symbolic-ref", "HEAD"); rama != "refs/heads/hoom/demo" {
		t.Fatalf("CA-771: en la copia, el worktree sigue en su rama: %q", rama)
	}
	if _, ok, err := gitx.WorktreeDe(copia, wt); !ok || err != nil {
		t.Fatalf("CA-771: WorktreeDe reconoce el worktree de la copia: %v %v", ok, err)
	}
}

// veeTareaListaRelativa es tdTareaLista con worktrees_relativos: true en el
// .hoom/local.yaml del principal (ignorado por git) antes de `hoom task
// start`: la tarea creada por el binario, su codigo commiteado y un `hoom
// verify` verde commiteado.
func veeTareaListaRelativa(t *testing.T, bin, slug string) (root, wt string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rdGit(t, root, "init", "-b", "main")
	rdGit(t, root, "config", "user.email", "test@hoom.dev")
	rdGit(t, root, "config", "user.name", "hoom test")
	rdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	rdEscribir(t, root, ".hoom/.gitignore", veeGitignore())
	rdEscribir(t, root, "app.go", "package app\n")
	rdGit(t, root, "add", "-A")
	rdGit(t, root, "commit", "-q", "-m", "inicial")
	rdEscribir(t, root, hentorno.Ruta, veeRelativos)
	tdOK(t, "CA-771", tdCorrer(t, bin, root, "task", "start", slug), "task", "start", slug)
	wt = filepath.Join(root, ".hoom", "worktrees", slug)
	rdEscribir(t, wt, "feature.go", "package app\n\nfunc Feature() int { return 1 }\n")
	tdOK(t, "CA-771", tdCorrer(t, bin, wt, "verify"), "verify")
	rdGit(t, wt, "add", "-A")
	rdGit(t, wt, "commit", "-q", "-m", "feature y veredicto")
	return root, wt
}

// CA-771 por el binario: con worktrees_relativos: true, hoom task start,
// hoom verify (en el worktree), hoom task list --json y hoom task done con
// el worktree de rutas relativas, como antes.
func TestCA771_ElBinarioReconoceElWorktreeRelativo(t *testing.T) {
	if testing.Short() {
		t.Skip("compila el binario de hoom: se omite con -short")
	}
	veeAislarGit(t)
	if !gitx.AceptaRutasRelativas(t.TempDir()) {
		t.Skip("CA-771: el git de esta maquina no acepta --relative-paths (2.48 o mas nuevo)")
	}
	bin := tdBinario(t)
	root, wt := veeTareaListaRelativa(t, bin, "precios")
	veeExigeRelativo(t, root, wt)
	r := tdCorrer(t, bin, root, "task", "list", "--json")
	tdOK(t, "CA-771", r, "task", "list", "--json")
	var tareas []TaskInfo
	if err := json.Unmarshal([]byte(r.stdout), &tareas); err != nil {
		t.Fatalf("CA-771: task list --json es JSON: %v\n%s", err, r.stdout)
	}
	if len(tareas) != 1 || tareas[0].Slug != "precios" || tareas[0].State != StateGreen || tareas[0].Dirty {
		t.Fatalf("CA-771: task list la reconoce, verde y limpia: %+v", tareas)
	}
	tdOK(t, "CA-771", tdCorrer(t, bin, root, "task", "done", "precios"), "task", "done", "precios")
	if _, err := os.Stat(wt); err == nil {
		t.Fatalf("CA-771: task done quita el worktree")
	}
	if out := rdGit(t, root, "branch", "--list", "hoom/precios"); !strings.Contains(out, "hoom/precios") {
		t.Fatalf("CA-771: la rama queda para integrar: %q", out)
	}
}

// CA-771: sin la clave, o con false, sin valor, nulo, "" o blancos, Start
// agrega el worktree como antes, con cualquier git (tambien con uno que dice
// 2.47): el .git del worktree guarda un gitdir absoluto, y el .git/config
// no gana extensions.relativeWorktrees. WorktreeDe lo reconoce.
func TestCA771_SinLaClaveElWorktreeEsComoAntes(t *testing.T) {
	for _, c := range []struct {
		nombre, local string // local "" = sin .hoom/local.yaml
		viejo         bool
	}{
		{"sin el archivo", "", false},
		{"el archivo sin la clave", "en_el_host: [test]\n", false},
		{"con ejecutar y sin la clave", "ejecutar: \"ddev exec -d /var/www/html/{rel} sh -c {cmd}\"\n", false},
		{"false", "worktrees_relativos: false\n", false},
		{"sin valor", "worktrees_relativos:\n", false},
		{"nulo", "worktrees_relativos: ~\n", false},
		{"vacio", "worktrees_relativos: \"\"\n", false},
		{"solo blancos", "worktrees_relativos: \"  \"\n", false},
		{"sin el archivo y un git que dice 2.47", "", true},
		{"false y un git que dice 2.47", "worktrees_relativos: false\n", true},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := veeBase(t, veeHoomYAML, veeGitignore())
			if c.local != "" {
				rdEscribir(t, root, hentorno.Ruta, c.local)
			}
			if c.viejo {
				veeGitViejo(t)
			}
			if err := Start(root, "demo", "main"); err != nil {
				t.Fatalf("CA-771: %s: Start agrega el worktree como antes: %v", c.nombre, err)
			}
			wt := filepath.Join(root, ".hoom", "worktrees", "demo")
			veeExigeAbsoluto(t, c.nombre, root, wt)
			if rama, ok, err := gitx.WorktreeDe(root, wt); err != nil || !ok || rama != "refs/heads/hoom/demo" {
				t.Fatalf("CA-771: %s: WorktreeDe reconoce el worktree: %q %v %v", c.nombre, rama, ok, err)
			}
		})
	}
}

// CA-771: con worktrees_relativos: true y un git que no acepta
// --relative-paths (uno falso primero en el PATH que dice 2.47.0), Start
// devuelve exactamente entorno.ErrWorktreesRelativosGitViejo y no crea
// nada: ni la rama, ni la carpeta del worktree, ni cambios en el
// .hoom/.gitignore (que solo ignora local.yaml: seguir de largo lo
// completaria) ni en el .git/config.
func TestCA771_ConLaClaveYUnGitViejoSeNiegaSinCrearNada(t *testing.T) {
	for _, c := range []struct{ nombre, local string }{
		{"solo la clave", veeRelativos},
		{"con ejecutar y en_el_host", "ejecutar: \"ddev exec -d /var/www/html/{rel} sh -c {cmd}\"\nen_el_host: [test]\nworktrees_relativos: true\n"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := veeBase(t, veeHoomYAML, veeSoloLocal)
			rdEscribir(t, root, hentorno.Ruta, c.local)
			veeGitViejo(t)
			antes := veeFoto(t, root)
			err := Start(root, "demo", "main")
			if err == nil || err.Error() != hentorno.ErrWorktreesRelativosGitViejo {
				t.Fatalf("CA-771: %s: con la clave y un git viejo, Start se niega con %q: %v", c.nombre, hentorno.ErrWorktreesRelativosGitViejo, err)
			}
			veeNadaCreado(t, c.nombre, root, "demo", antes)
		})
	}
}

// CA-771: Start lee el .hoom/local.yaml con las mismas reglas que verify:
// con uno de sus errores (los cuatro del spec con su texto exacto, o uno
// que empieza con ".hoom/local.yaml: ", como un worktrees_relativos que no
// es booleano o un YAML roto), Start devuelve ese error y no crea nada.
// Con el git de la maquina, sea cual sea su version.
func TestCA771_StartConUnLocalYamlInvalidoSeNiegaConSuError(t *testing.T) {
	const noIgnora = "cache/\nworktrees/\n"
	for _, c := range []struct {
		nombre, gitignore, local string
		want                     string // "" = solo el prefijo
	}{
		{"clave desconocida", veeSoloLocal, "foo: 1\nworktrees_relativos: true\n", fmt.Sprintf(hentorno.ErrClaveDesconocida, "foo")},
		{"sin {cmd}", veeSoloLocal, "ejecutar: \"ddev exec sh -c\"\nworktrees_relativos: true\n", hentorno.ErrSinCmd},
		{"gate que no es", veeSoloLocal, "en_el_host: [semgrep]\n", fmt.Sprintf(hentorno.ErrGateDesconocido, "semgrep")},
		{"no ignorado", noIgnora, "worktrees_relativos: true\n", hentorno.ErrNoIgnorado},
		{"no ignorado y false", noIgnora, "worktrees_relativos: false\n", hentorno.ErrNoIgnorado},
		{"worktrees_relativos entre comillas", veeSoloLocal, "worktrees_relativos: \"true\"\n", ""},
		{"worktrees_relativos yes", veeSoloLocal, "worktrees_relativos: yes\n", ""},
		{"worktrees_relativos una lista", veeSoloLocal, "worktrees_relativos: [true]\n", ""},
		{"YAML roto", veeSoloLocal, "worktrees_relativos: [\n", ""},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := veeBase(t, veeHoomYAML, c.gitignore)
			rdEscribir(t, root, hentorno.Ruta, c.local)
			antes := veeFoto(t, root)
			err := Start(root, "demo", "main")
			if err == nil {
				t.Fatalf("CA-771: %s: Start se niega con el error del archivo", c.nombre)
			}
			if c.want != "" && err.Error() != c.want {
				t.Fatalf("CA-771: %s: el error es %q; fue %q", c.nombre, c.want, err.Error())
			}
			if !strings.HasPrefix(err.Error(), ".hoom/local.yaml: ") || err.Error() == hentorno.ErrWorktreesRelativosGitViejo {
				t.Fatalf("CA-771: %s: el error es el de Leer, que empieza con el archivo y no es el de git viejo: %q", c.nombre, err.Error())
			}
			veeNadaCreado(t, c.nombre, root, "demo", antes)
		})
	}
}

// CA-771 (go doc de Start): un root sin hoom.yaml y sin .hoom/local.yaml
// arranca su tarea como antes: sin el archivo, el hoom.yaml no se lee para
// el. El worktree es el de antes, con rutas absolutas.
func TestCA771_UnRootSinHoomYamlNiLocalYamlArrancaComoAntes(t *testing.T) {
	root := veeBase(t, "", veeGitignore())
	if err := Start(root, "demo", "main"); err != nil {
		t.Fatalf("CA-771: sin hoom.yaml ni .hoom/local.yaml, Start arranca la tarea como antes: %v", err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", "demo")
	veeExigeAbsoluto(t, "sin hoom.yaml", root, wt)
	if rama, ok, err := gitx.WorktreeDe(root, wt); err != nil || !ok || rama != "refs/heads/hoom/demo" {
		t.Fatalf("CA-771: sin hoom.yaml: WorktreeDe reconoce el worktree: %q %v %v", rama, ok, err)
	}
}

// CA-771 (go doc de Start): con el .hoom/local.yaml, Start toma los gates
// del hoom.yaml del root como verify (manifest.Load con profiles.Resolve);
// un hoom.yaml que falta o que no carga es su error, el mismo de
// manifest.Load, y no crea nada.
func TestCA771_ConElArchivoUnHoomYamlQueNoCargaEsElError(t *testing.T) {
	for _, c := range []struct{ nombre, hoomYAML string }{
		{"sin hoom.yaml", ""},
		{"hoom.yaml roto", "schema: hoom/v1\nproject: demo\ngates: [\n"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := veeBase(t, c.hoomYAML, veeSoloLocal)
			rdEscribir(t, root, hentorno.Ruta, "worktrees_relativos: false\n")
			_, lerr := hmanifest.Load(root, hprofiles.Resolve)
			if lerr == nil {
				t.Fatalf("fixture: %s: el hoom.yaml no carga", c.nombre)
			}
			antes := veeFoto(t, root)
			err := Start(root, "demo", "main")
			if err == nil || err.Error() != lerr.Error() {
				t.Fatalf("CA-771: %s: con el archivo, Start se niega con el error de cargar el hoom.yaml (%q): %v", c.nombre, lerr.Error(), err)
			}
			veeNadaCreado(t, c.nombre, root, "demo", antes)
		})
	}
}

// CA-771 por el binario: con worktrees_relativos: true y un git que dice
// 2.47, `hoom task start` sale distinto de 0 con el texto del spec y no
// crea nada.
func TestCA771_ElBinarioConUnGitViejoSeNiega(t *testing.T) {
	if testing.Short() {
		t.Skip("compila el binario de hoom: se omite con -short")
	}
	bin := tdBinario(t)
	root := veeBase(t, veeHoomYAML, veeSoloLocal)
	rdEscribir(t, root, hentorno.Ruta, veeRelativos)
	veeGitViejo(t)
	antes := veeFoto(t, root)
	r := tdCorrer(t, bin, root, "task", "start", "demo")
	if r.exit == 0 || !strings.Contains(r.stdout+r.stderr, hentorno.ErrWorktreesRelativosGitViejo) {
		t.Fatalf("CA-771: hoom task start se niega con %q: exit %d\nstdout: %s\nstderr: %s", hentorno.ErrWorktreesRelativosGitViejo, r.exit, r.stdout, r.stderr)
	}
	veeNadaCreado(t, "el binario", root, "demo", antes)
}
