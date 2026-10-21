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
// Paquete externo, como los de 28c350. La matriz (worktreetest.Casos) y su
// armado viven en internal/worktreetest (hallazgo 20261021T174702_00e2fc):
// aca queda solo la operacion propia.
package runcmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// t68Entorno: el proyecto con legit (commit propio, app.go modificado y
// wip-legit.txt sin commitear), trabajo sin commitear en la raiz y, si
// todos, cada carpeta de worktreetest.Casos con trabajo propio.
func t68Entorno(t *testing.T, todos bool) (root, legit string) {
	t.Helper()
	root, legit = worktreetest.Proyecto(t, taskcmd.Start)
	worktreetest.Escribir(t, legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
	worktreetest.Escribir(t, legit, "wip-legit.txt", "sin guardar\n")
	worktreetest.Escribir(t, root, "app.go", "package app\n\n// trabajo sin guardar de la raiz\n")
	worktreetest.Escribir(t, root, "raiz-nuevo.go", "package app\n")
	if todos {
		for _, c := range worktreetest.Casos {
			worktreetest.Armar(t, root, c.Slug, true)
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

// Hallazgo 68644c (high). CA-280: ninguna de las carpetas de worktreetest.Casos es su
// tarea: TaskDir falla con cada una, tambien con la raiz alcanzada por un
// symlink, y no toca nada. otro dice que no esta en su rama. Guarda: legit
// sigue siendo su tarea.
func TestHallazgo68644c_CA280_TaskDirRechazaLoQueNoEsLaTarea(t *testing.T) {
	root, legit := t68Entorno(t, true)
	enlace := filepath.Join(filepath.Dir(root), "enlace-al-proyecto")
	worktreetest.Symlink(t, root, enlace)
	antes := worktreetest.Foto(t, root)
	for _, c := range worktreetest.Casos {
		for _, r := range []string{root, enlace} {
			got, err := runcmd.TaskDir(r, c.Slug)
			if err == nil {
				t.Errorf("CA-280: hallazgo 68644c: TaskDir(%q) por %s debe fallar (%s), devolvio %q", c.Slug, r, c.Que, got)
			}
			if c.Slug == "otro" {
				t68OtroDiceLaRama(t, err)
			}
		}
	}
	if despues := worktreetest.Foto(t, root); despues != antes {
		t.Errorf("CA-280: hallazgo 68644c: TaskDir no toca nada, y cambio:\n%s", worktreetest.Cambios(antes, despues))
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
	worktreetest.Plana(t, root)
	otro := filepath.Join(base, "otro")
	if err := os.MkdirAll(otro, 0o755); err != nil {
		t.Fatal(err)
	}
	worktreetest.Git(t, otro, "init", "-q", "-b", "main")
	worktreetest.Git(t, otro, "-c", "user.email=o@hoom.dev", "-c", "user.name=otro", "commit", "-q", "--allow-empty", "-m", "otro")
	worktreetest.Git(t, otro, "worktree", "add", "-q", "-b", "hoom/ajeno", filepath.Join(root, ".hoom", "worktrees", "ajeno"), "main")
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
	antes := worktreetest.Foto(t, root)
	for _, esc := range escenarios {
		worktreetest.ConEntorno(esc.vars, func() {
			for _, slug := range []string{"plana", "ajeno"} {
				if got, err := runcmd.TaskDir(root, slug); err == nil {
					t.Errorf("CA-280: hallazgo 68644c (%s): TaskDir(%q) debe fallar, el entorno no la vuelve una tarea; devolvio %q", esc.nombre, slug, got)
				}
			}
			t68Acepta(t, esc.nombre, root, "legit", legit)
		})
	}
	if despues := worktreetest.Foto(t, root); despues != antes {
		t.Errorf("CA-280: hallazgo 68644c: TaskDir no toca nada, y cambio:\n%s", worktreetest.Cambios(antes, despues))
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
// carpetas de worktreetest.Casos, o = una carpeta plana con GIT_DIR de la raiz en el
// entorno, no arranca: el provider (que commitea lo que ve) no corre y
// legit, la raiz y las carpetas quedan igual. Guarda: el run de legit corre
// en su worktree.
func TestHallazgo68644c_CA280_RunConCarpetaQueNoEsLaTareaNoArranca(t *testing.T) {
	marca := t68ProviderQueCommitea(t)
	root, legit := t68Entorno(t, true)
	entornoPlana := worktreetest.Plana(t, root)
	m := runcmd.NewManager(root)
	intentar := func(slug, que string, vars map[string]string) {
		t.Helper()
		antes := worktreetest.Foto(t, root)
		worktreetest.ConEntorno(vars, func() {
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
		if despues := worktreetest.Foto(t, root); despues != antes {
			t.Errorf("CA-280: hallazgo 68644c: un run con Task %q no toca legit, la raiz ni la carpeta, y cambio:\n%s", slug, worktreetest.Cambios(antes, despues))
		}
	}
	for _, c := range worktreetest.Casos {
		intentar(c.Slug, c.Que, nil)
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
	worktreetest.Symlink(t, root, enlace)
	t68Acepta(t, "raiz por symlink", enlace, "legit", legit)
	if err := taskcmd.Start(enlace, "por-el-enlace", "main"); err != nil {
		t.Fatalf("fixture: task start por el enlace: %v", err)
	}
	t68Acepta(t, "tarea creada por el enlace", enlace, "por-el-enlace", wt(enlace, "por-el-enlace"))
	t68Acepta(t, "tarea creada por el enlace, pedida por la ruta real", root, "por-el-enlace", wt(root, "por-el-enlace"))

	t.Run("rutas relativas", func(t *testing.T) {
		worktreetest.Git(t, root, "config", "worktree.useRelativePaths", "true")
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
