// Tests de punta a punta del hallazgo 20261021T163139_1e4b3e (high, risk) de
// la delta 1 de fixes-cabina-c1b, sobre el binario real de hoom
// (.hoom/specs/acciones-desde-la-tarjeta.md, CA-346: `hoom task discard`
// vuelve a HEAD las rutas de LA TAREA; CA-243: `hoom run --task` corre el
// provider en el worktree de la tarea). Escritos desde el contrato, nunca
// desde la implementacion:
//
// hoom no puede terminar operando sobre otro arbol porque el entorno traiga
// GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR, GIT_INDEX_FILE,
// GIT_OBJECT_DIRECTORY, GIT_ALTERNATE_OBJECT_DIRECTORIES o GIT_NAMESPACE. El
// binario, apenas arranca, quita esas variables de su proceso, asi que ni sus
// git ni los providers que lanza (CLI y Studio) las heredan:
//
//   - `hoom task discard <slug> --yes`, con esas variables apuntando a la
//     raiz, restaura SOLO la tarea (a su HEAD, que no es el de la raiz) y deja
//     intactos los cambios sin commitear de la raiz;
//   - `hoom run --task <slug>` con un provider falso: el provider no recibe
//     ninguna de esas variables (escribe su entorno a un archivo).
//
// Guardas: con el entorno limpio, discard y run hacen lo de siempre.
package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// e1VarsDeOtroRepo son las variables con las que git trabaja sobre otro
// repositorio, otro arbol u otro indice.
var e1VarsDeOtroRepo = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
}

// e1EntornoLimpio es el entorno del test sin ninguna de las variables de otro
// repositorio ni HOOM_ROLE/HOOM_RUN/HOOM_PROVIDER/HOOM_TASK heredadas.
func e1EntornoLimpio() []string {
	var out []string
	for _, kv := range os.Environ() {
		nombre := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			nombre = kv[:i]
		}
		saltar := false
		for _, v := range append([]string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK"}, e1VarsDeOtroRepo...) {
			if nombre == v {
				saltar = true
			}
		}
		if !saltar {
			out = append(out, kv)
		}
	}
	return out
}

// e1ConPATH reemplaza PATH en env por path.
func e1ConPATH(env []string, path string) []string {
	var out []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, "PATH=") {
			out = append(out, kv)
		}
	}
	return append(out, "PATH="+path)
}

type e1Salida struct {
	exit           int
	stdout, stderr string
}

func (s e1Salida) todo() string { return s.stdout + s.stderr }

// e1Correr ejecuta bin en dir con exactamente env, con un tope de 2 minutos.
func e1Correr(t *testing.T, bin, dir string, env []string, args ...string) e1Salida {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir, cmd.Env = dir, env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("hoom %v no termino en 2 minutos\nstdout: %s\nstderr: %s", args, out.String(), errb.String())
	}
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar %s %v: %v", bin, args, err)
	}
	return e1Salida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

// e1Git corre git en dir con el entorno limpio y devuelve su salida.
func e1Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, e1EntornoLimpio()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v en %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func e1Escribir(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func e1Leer(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// e1Envenenado es una forma de apuntar git a la raiz desde el entorno.
type e1Envenenado struct {
	nombre string
	vars   func(root string) []string
}

func e1Envenenados() []e1Envenenado {
	return []e1Envenenado{
		{"GIT_WORK_TREE en la raiz", func(root string) []string {
			return []string{"GIT_WORK_TREE=" + root}
		}},
		{"GIT_DIR en el .git de la raiz", func(root string) []string {
			return []string{"GIT_DIR=" + filepath.Join(root, ".git")}
		}},
		{"las siete apuntando a la raiz", func(root string) []string {
			g := filepath.Join(root, ".git")
			return []string{
				"GIT_DIR=" + g, "GIT_WORK_TREE=" + root, "GIT_COMMON_DIR=" + g,
				"GIT_INDEX_FILE=" + filepath.Join(g, "index"), "GIT_OBJECT_DIRECTORY=" + filepath.Join(g, "objects"),
				"GIT_ALTERNATE_OBJECT_DIRECTORIES=" + filepath.Join(g, "objects"), "GIT_NAMESPACE=otro",
			}
		}},
	}
}

// Contenidos del fixture de discard.
const (
	e1AppInicial    = "package app\n"
	e1AppTarea      = "package app\n\n// commiteado en la tarea\n"
	e1SoloTarea     = "package app\n\n// solo existe en el HEAD de la tarea\n"
	e1AppTareaSucio = "package app\n\n// sin guardar en la tarea\n"
	e1AppRaizSucio  = "package app\n\n// sin guardar en la raiz\n"
	e1RaizNuevo     = "package app\n\n// nuevo y sin guardar en la raiz\n"
)

// e1Tarea arma un proyecto real con la tarea demo creada por `hoom task
// start` (entorno limpio). La rama de la tarea tiene un commit propio
// (app.go cambiado y solo_tarea.go nuevo), y despues quedan cambios sin
// commitear en los dos arboles: en la tarea, app.go y tarea_nuevo.go; en la
// raiz, app.go y raiz_nuevo.go.
func e1Tarea(t *testing.T, bin string) (root, wt string) {
	t.Helper()
	root = qcRepo(t)
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if s := e1Correr(t, bin, root, e1EntornoLimpio(), "task", "start", "demo"); s.exit != 0 {
		t.Fatalf("fixture: hoom task start demo: exit %d\n%s", s.exit, s.todo())
	}
	wt = filepath.Join(root, ".hoom", "worktrees", "demo")
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("fixture: task start crea el worktree en %s: %v", wt, err)
	}
	e1Escribir(t, filepath.Join(wt, "app.go"), e1AppTarea)
	e1Escribir(t, filepath.Join(wt, "solo_tarea.go"), e1SoloTarea)
	e1Git(t, wt, "add", "-A")
	e1Git(t, wt, "commit", "-q", "-m", "trabajo de la tarea")

	e1Escribir(t, filepath.Join(wt, "app.go"), e1AppTareaSucio)
	e1Escribir(t, filepath.Join(wt, "tarea_nuevo.go"), "package app\n")
	e1Escribir(t, filepath.Join(root, "app.go"), e1AppRaizSucio)
	e1Escribir(t, filepath.Join(root, "raiz_nuevo.go"), e1RaizNuevo)
	return root, wt
}

// e1FotoGit es lo que git dice de un arbol: su HEAD y su estado (indice y
// arbol), con el entorno limpio.
func e1FotoGit(t *testing.T, dir string) string {
	t.Helper()
	return e1Git(t, dir, "rev-parse", "HEAD") + "\n" + e1Git(t, dir, "status", "--porcelain", "--untracked-files=all")
}

// e1SinCambiosFueraDeHoom exige que el arbol dir no tenga cambios sin guardar
// fuera de .hoom/.
func e1SinCambiosFueraDeHoom(t *testing.T, ca, como, dir string) {
	t.Helper()
	for _, l := range strings.Split(e1Git(t, dir, "status", "--porcelain", "--untracked-files=all"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.Contains(l, ".hoom/") {
			continue
		}
		t.Errorf("%s: hallazgo 1e4b3e (%s): discard --yes deja la tarea sin cambios fuera de .hoom/; queda %q", ca, como, l)
	}
}

// e1RevisarDiscard exige el efecto de `hoom task discard demo --yes`: la
// tarea vuelve a SU HEAD (app.go como lo commiteo la tarea, solo_tarea.go
// sigue, tarea_nuevo.go no esta) y la raiz queda exactamente como estaba.
func e1RevisarDiscard(t *testing.T, como, root, wt, fotoRaiz, ramaTarea string, s e1Salida) {
	t.Helper()
	const ca = "CA-346"
	if s.exit != 0 {
		t.Errorf("%s: hallazgo 1e4b3e (%s): discard --yes descarta los cambios de la tarea y sale con 0, salio con %d\n%s", ca, como, s.exit, s.todo())
	}
	if got, ok := e1Leer(t, filepath.Join(wt, "app.go")); !ok || got != e1AppTarea {
		t.Errorf("%s: hallazgo 1e4b3e (%s): app.go de la tarea vuelve al HEAD de LA TAREA (%q), quedo %q (existe %v)", ca, como, e1AppTarea, got, ok)
	}
	if got, ok := e1Leer(t, filepath.Join(wt, "solo_tarea.go")); !ok || got != e1SoloTarea {
		t.Errorf("%s: hallazgo 1e4b3e (%s): solo_tarea.go esta en el HEAD de la tarea: discard no lo toca, quedo %q (existe %v)", ca, como, got, ok)
	}
	if _, ok := e1Leer(t, filepath.Join(wt, "tarea_nuevo.go")); ok {
		t.Errorf("%s: hallazgo 1e4b3e (%s): tarea_nuevo.go no esta en el HEAD de la tarea: discard --yes lo borra", ca, como)
	}
	e1SinCambiosFueraDeHoom(t, ca, como, wt)

	if got, ok := e1Leer(t, filepath.Join(root, "app.go")); !ok || got != e1AppRaizSucio {
		t.Errorf("%s: hallazgo 1e4b3e (%s): el app.go sin guardar de la RAIZ queda intacto (%q), quedo %q (existe %v)", ca, como, e1AppRaizSucio, got, ok)
	}
	if got, ok := e1Leer(t, filepath.Join(root, "raiz_nuevo.go")); !ok || got != e1RaizNuevo {
		t.Errorf("%s: hallazgo 1e4b3e (%s): raiz_nuevo.go, sin guardar en la RAIZ, queda intacto, quedo %q (existe %v)", ca, como, got, ok)
	}
	if got := e1FotoGit(t, root); got != fotoRaiz {
		t.Errorf("%s: hallazgo 1e4b3e (%s): la raiz (HEAD, indice y arbol) queda como estaba:\nantes:\n%s\ndespues:\n%s", ca, como, fotoRaiz, got)
	}
	if got := e1Git(t, root, "rev-parse", "hoom/demo"); got != ramaTarea {
		t.Errorf("%s: hallazgo 1e4b3e (%s): discard no mueve la rama de la tarea: %s -> %s", ca, como, ramaTarea, got)
	}
	if strings.Contains(s.todo(), "raiz_nuevo.go") {
		t.Errorf("%s: hallazgo 1e4b3e (%s): discard no nombra rutas de la raiz:\n%s", ca, como, s.todo())
	}
}

// Hallazgo 20261021T163139_1e4b3e. CA-346: con GIT_WORK_TREE, GIT_DIR o las
// siete variables apuntando a la raiz en el entorno del proceso, `hoom task
// discard demo --yes` (desde la raiz) restaura solo la tarea, a su propio
// HEAD, y deja intactos los cambios sin commitear de la raiz. Guarda: con el
// entorno limpio hace lo mismo.
func TestCA346_Hallazgo1e4b3e_DiscardConEntornoDeOtroArbolSoloTocaLaTarea(t *testing.T) {
	bin := qcHoom(t)

	t.Run("guarda: entorno limpio", func(t *testing.T) {
		root, wt := e1Tarea(t, bin)
		foto, rama := e1FotoGit(t, root), e1Git(t, root, "rev-parse", "hoom/demo")
		s := e1Correr(t, bin, root, e1EntornoLimpio(), "task", "discard", "demo", "--yes")
		e1RevisarDiscard(t, "entorno limpio", root, wt, foto, rama, s)
	})

	for _, v := range e1Envenenados() {
		v := v
		t.Run(v.nombre, func(t *testing.T) {
			root, wt := e1Tarea(t, bin)
			foto, rama := e1FotoGit(t, root), e1Git(t, root, "rev-parse", "hoom/demo")
			env := append(e1EntornoLimpio(), v.vars(root)...)
			s := e1Correr(t, bin, root, env, "task", "discard", "demo", "--yes")
			e1RevisarDiscard(t, v.nombre, root, wt, foto, rama, s)
		})
	}
}

// e1ProviderFalso instala en un directorio nuevo un `claude` falso que, en
// cada invocacion, deja su entorno (env-<pid>), sus argumentos (args-<pid>)
// y su directorio (pwd-<pid>) en salida, y termina con 0 sin decir nada.
// Devuelve el directorio del binario.
func e1ProviderFalso(t *testing.T, salida string) string {
	t.Helper()
	bin := t.TempDir()
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	s := "#!/bin/sh\n" +
		"env > " + q(filepath.Join(salida, ".tmp-env")) + ".$$\n" +
		"mv " + q(filepath.Join(salida, ".tmp-env")) + ".$$ " + q(filepath.Join(salida, "env")) + "-$$\n" +
		"printf '%s\\n' \"$@\" > " + q(filepath.Join(salida, "args")) + "-$$\n" +
		"pwd -P > " + q(filepath.Join(salida, "pwd")) + "-$$\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// e1Invocacion es lo que dejo una invocacion del provider falso.
type e1Invocacion struct {
	pid            string
	env, args, pwd string
}

func e1Invocaciones(t *testing.T, salida string) []e1Invocacion {
	t.Helper()
	envs, err := filepath.Glob(filepath.Join(salida, "env-*"))
	if err != nil {
		t.Fatal(err)
	}
	var out []e1Invocacion
	for _, e := range envs {
		pid := strings.TrimPrefix(filepath.Base(e), "env-")
		env, _ := e1Leer(t, e)
		args, _ := e1Leer(t, filepath.Join(salida, "args-"+pid))
		pwd, _ := e1Leer(t, filepath.Join(salida, "pwd-"+pid))
		out = append(out, e1Invocacion{pid: pid, env: env, args: args, pwd: strings.TrimSpace(pwd)})
	}
	return out
}

// e1RevisarRun exige que `hoom run --provider claude --task demo` haya
// lanzado el provider con el pedido, en el worktree de la tarea, y que
// ninguna invocacion del provider haya recibido una variable de otro
// repositorio.
func e1RevisarRun(t *testing.T, como, wt, pedido string, s e1Salida, invs []e1Invocacion) {
	t.Helper()
	if s.exit != 0 {
		t.Errorf("CA-243: hallazgo 1e4b3e (%s): hoom run --task con el provider falso sale con 0, salio con %d\n%s", como, s.exit, s.todo())
	}
	corrio := false
	for _, inv := range invs {
		if strings.Contains(inv.args, pedido) {
			corrio = true
			if inv.pwd != wt {
				t.Errorf("CA-243: hallazgo 1e4b3e (%s): el provider corre en el worktree de la tarea %s, corrio en %s", como, wt, inv.pwd)
			}
		}
		for _, l := range strings.Split(inv.env, "\n") {
			for _, v := range e1VarsDeOtroRepo {
				if strings.HasPrefix(l, v+"=") {
					t.Errorf("CA-243: hallazgo 1e4b3e (%s): el provider (pid %s, args %q) no hereda %s: recibio %q",
						como, inv.pid, strings.ReplaceAll(inv.args, "\n", " "), v, l)
				}
			}
		}
	}
	if !corrio {
		t.Errorf("CA-243: hallazgo 1e4b3e (%s): hoom run --task lanza el provider con el pedido %q; invocaciones: %d\n%s", como, pedido, len(invs), s.todo())
	}
}

// Hallazgo 20261021T163139_1e4b3e. CA-243: con GIT_WORK_TREE, GIT_DIR o las
// siete variables apuntando a la raiz en el entorno del proceso, `hoom run
// --provider claude --task demo` lanza el provider (falso) en el worktree de
// la tarea y el provider no recibe ninguna de esas variables. Guarda: con el
// entorno limpio el provider corre igual y tampoco las ve.
func TestCA243_Hallazgo1e4b3e_RunNoLePasaAlProviderElEntornoDeOtroArbol(t *testing.T) {
	bin := qcHoom(t)
	casos := append([]e1Envenenado{{"guarda: entorno limpio", func(string) []string { return nil }}}, e1Envenenados()...)
	for _, v := range casos {
		v := v
		t.Run(v.nombre, func(t *testing.T) {
			root := qcRepo(t)
			if r, err := filepath.EvalSymlinks(root); err == nil {
				root = r
			}
			if s := e1Correr(t, bin, root, e1EntornoLimpio(), "task", "start", "demo"); s.exit != 0 {
				t.Fatalf("fixture: hoom task start demo: exit %d\n%s", s.exit, s.todo())
			}
			wt := filepath.Join(root, ".hoom", "worktrees", "demo")
			salida := t.TempDir()
			fake := e1ProviderFalso(t, salida)
			env := e1ConPATH(e1EntornoLimpio(), fake+string(os.PathListSeparator)+os.Getenv("PATH"))
			env = append(env, v.vars(root)...)
			const pedido = "pedido del hallazgo 1e4b3e"
			s := e1Correr(t, bin, root, env, "run", "--provider", "claude", "--task", "demo", pedido)
			e1RevisarRun(t, v.nombre, wt, pedido, s, e1Invocaciones(t, salida))
		})
	}
}
