// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// `hoom task start` (CA-771): con un git que acepta --relative-paths (2.48 o
// mas nuevo), el worktree de la tarea guarda su gitdir con rutas relativas,
// asi sigue valido en un contenedor que monta el proyecto en otra ruta; y
// `hoom task list`, `hoom verify` y `hoom task done` lo reconocen como
// antes. Con un git viejo el test se salta (gitx.AceptaRutasRelativas).
//
// En proceso (taskcmd.Start, gitx.WorktreeDe, Snapshot) y por el binario.
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package taskcmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
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

// veeRepo es un proyecto hoom en main, todo commiteado, sin tareas.
func veeRepo(t *testing.T) string {
	t.Helper()
	veeAislarGit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rdGit(t, root, "init", "-q", "-b", "main")
	rdGit(t, root, "config", "user.email", "test@hoom.dev")
	rdGit(t, root, "config", "user.name", "hoom test")
	rdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	rdEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	rdEscribir(t, root, "app.go", "package app\n")
	rdGit(t, root, "add", "-A")
	rdGit(t, root, "commit", "-q", "-m", "inicial")
	if !gitx.AceptaRutasRelativas(root) {
		t.Skip("CA-771: el git de esta maquina no acepta --relative-paths (2.48 o mas nuevo)")
	}
	return root
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

// CA-771: taskcmd.Start agrega el worktree con rutas relativas, y
// gitx.WorktreeDe y Snapshot lo reconocen.
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

// CA-771 y caso limite "el proyecto montado en otra ruta": una copia del
// proyecto en otra ruta (como lo ve un contenedor) tiene un worktree que git
// sigue reconociendo, con el repo de la copia como su comun.
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

// CA-771 por el binario: hoom task start, hoom verify (en el worktree),
// hoom task list --json y hoom task done con el worktree de rutas
// relativas, como antes.
func TestCA771_ElBinarioReconoceElWorktreeRelativo(t *testing.T) {
	if testing.Short() {
		t.Skip("compila el binario de hoom: se omite con -short")
	}
	veeAislarGit(t)
	if !gitx.AceptaRutasRelativas(t.TempDir()) {
		t.Skip("CA-771: el git de esta maquina no acepta --relative-paths (2.48 o mas nuevo)")
	}
	bin := tdBinario(t)
	root, wt := tdTareaLista(t, bin, "precios")
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
