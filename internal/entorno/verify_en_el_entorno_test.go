// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre el
// paquete entorno, como consumidor externo (solo su API publica):
//
//   - CA-762: .hoom/local.yaml se lee del checkout principal, tambien desde
//     .hoom/worktrees/<slug>/; uno dentro del worktree no se lee; fuera de
//     git, el de la raiz; `ejecutar:` vacio, nulo o en blanco cuenta como
//     sin la clave (aclaraciones 4 y 7);
//   - CA-763: los cuatro errores con su texto exacto, en el orden del spec
//     (aclaracion 5), sin ErrNoIgnorado fuera de git (aclaracion 6), y el
//     resto de los defectos fallando cerrado con el prefijo del archivo
//     (aclaracion 4);
//   - CA-764: Ajuste.Linea cita cada marca como una palabra de sh entre
//     comillas simples (la comilla de adentro cerrada, escapada y
//     reabierta), en una pasada;
//     {rel} es "." en el principal y .hoom/worktrees/<slug> en una tarea;
//     ConPrefijo deja afuera a los de en_el_host;
//   - CA-771 (enmienda, ronda 0b): worktrees_relativos es un booleano de
//     YAML; false, sin valor, nulo, "" o blancos cuentan como sin la clave;
//     cualquier otro valor falla cerrado; Leer nunca devuelve el error de
//     git viejo, que es de `hoom task start`.
//
// Repos git temporales, con el git aislado de quien corre los tests
// (gittest.AislarGit). Escritos desde el spec, las aclaraciones de la ronda
// 0 y el go doc del paquete, sin leer la implementacion.
package entorno_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

func TestMain(m *testing.M) {
	borrarConfig, err := gittest.AislarGit()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture:", err)
		os.Exit(2)
	}
	code := m.Run()
	borrarConfig()
	os.Exit(code)
}

// ---------------------------------------------------------------- fixtures

// veeGates son los gates de hoom.yaml que los tests le pasan a Leer.
var veeGates = []string{"test", "build"}

func veeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (en %s): %v\n%s", args, dir, err, out)
	}
	if len(args) > 0 && args[0] == "init" {
		gittest.ApagarMantenimiento(t, dir)
	}
	return strings.TrimSpace(string(out))
}

func veeEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func veeDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// veeTieneLinea dice si body tiene la linea l (sin blancos al borde).
func veeTieneLinea(body, l string) bool {
	for _, x := range strings.Split(body, "\n") {
		if strings.TrimSpace(x) == l {
			return true
		}
	}
	return false
}

// veeGitignore es el .hoom/.gitignore canonico, con la linea local.yaml
// aunque el canonico todavia no la traiga: estos tests son de Leer, no de
// CA-770.
func veeGitignore() string {
	body := hoomfs.GitignoreBody()
	if !veeTieneLinea(body, "local.yaml") {
		body += "local.yaml\n"
	}
	return body
}

// veeRepo es un proyecto hoom en un repo git (rama main) cuyo .hoom/.gitignore
// ignora local.yaml, con todo commiteado. sub es la carpeta del proyecto
// dentro del repo ("" = la raiz).
func veeRepo(t *testing.T, sub string) string {
	t.Helper()
	root := veeDir(t)
	veeGit(t, root, "init", "-q", "-b", "main")
	veeGit(t, root, "config", "user.email", "test@hoom.dev")
	veeGit(t, root, "config", "user.name", "hoom test")
	proyecto := root
	if sub != "" {
		proyecto = filepath.Join(root, filepath.FromSlash(sub))
	}
	veeEscribir(t, proyecto, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n  build:\n    required: true\n    cmd: \"true\"\n")
	veeEscribir(t, root, ".hoom/.gitignore", veeGitignore())
	if sub != "" {
		veeEscribir(t, proyecto, ".hoom/.gitignore", veeGitignore())
	}
	veeEscribir(t, root, "app.go", "package app\n")
	veeGit(t, root, "add", "-A")
	veeGit(t, root, "commit", "-q", "-m", "inicial")
	return root
}

// veeTarea agrega el worktree de la tarea slug como lo deja `hoom task start`:
// rama hoom/<slug> en .hoom/worktrees/<slug>.
func veeTarea(t *testing.T, root, slug string) string {
	t.Helper()
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	veeGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	return wt
}

func veeLocal(t *testing.T, dir, body string) {
	t.Helper()
	veeEscribir(t, dir, entorno.Ruta, body)
}

func veeSha(body string) string {
	s := sha256.Sum256([]byte(body))
	return hex.EncodeToString(s[:])
}

// veeLeer es Leer sin error, con el Ajuste nunca nil.
func veeLeer(t *testing.T, ca, dir string) *entorno.Ajuste {
	t.Helper()
	a, err := entorno.Leer(dir, veeGates)
	if err != nil {
		t.Fatalf("%s: Leer(%s): %v", ca, dir, err)
	}
	if a == nil {
		t.Fatalf("%s: con un error nil el Ajuste nunca es nil", ca)
	}
	return a
}

// veeError es el error de Leer en dir; corta si no hay.
func veeError(t *testing.T, ca, caso, dir string) string {
	t.Helper()
	a, err := entorno.Leer(dir, veeGates)
	if err == nil {
		t.Fatalf("%s: %s: Leer rechaza el archivo, devolvio %+v", ca, caso, a)
	}
	return err.Error()
}

// veeSinIgnorar deja el .hoom/.gitignore del principal sin local.yaml, y lo
// commitea.
func veeSinIgnorar(t *testing.T, root string) {
	t.Helper()
	veeEscribir(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\nenvelopes/\n")
	veeGit(t, root, "add", "-A")
	veeGit(t, root, "commit", "-q", "-m", "gitignore sin local.yaml")
}

const veePlantilla = "ddev exec -d /var/www/html/{rel} sh -c {cmd}"

// ---------------------------------------------------------------- CA-762

// CA-762: desde el principal y desde .hoom/worktrees/<slug>/ se lee el
// .hoom/local.yaml del principal, con su plantilla tal cual, en_el_host en
// el orden del archivo, el sha256 de sus bytes y {rel} "." o
// ".hoom/worktrees/<slug>"; el del worktree (aca invalido) no se lee.
func TestCA762_SeLeeElDelPrincipalDesdeElPrincipalYDesdeLaTarea(t *testing.T) {
	const ca = "CA-762"
	root := veeRepo(t, "")
	wt := veeTarea(t, root, "demo")
	body := "ejecutar: \"" + veePlantilla + "\"\nen_el_host: [build, test]\n"
	veeLocal(t, root, body)
	veeLocal(t, wt, "clave_que_no_existe: 1\nejecutar: \"otro-prefijo sin la marca\"\n")
	for _, c := range []struct{ dir, rel string }{{root, "."}, {wt, ".hoom/worktrees/demo"}} {
		a := veeLeer(t, ca, c.dir)
		if a.Principal != root {
			t.Fatalf("%s: desde %s el principal es %s: %q", ca, c.dir, root, a.Principal)
		}
		if !a.Existe || a.Ejecutar != veePlantilla || a.Rel != c.rel || a.SHA256 != veeSha(body) {
			t.Fatalf("%s: desde %s se lee el del principal (plantilla %q, rel %q, sha256 %s): %+v", ca, c.dir, veePlantilla, c.rel, veeSha(body), a)
		}
		if !reflect.DeepEqual(a.EnElHost, []string{"build", "test"}) {
			t.Fatalf("%s: en_el_host en el orden del archivo: %q", ca, a.EnElHost)
		}
	}
}

// CA-762: un .hoom/local.yaml dentro del worktree no se lee: sin el del
// principal, la tarea corre como hoy aunque el suyo tenga ejecutar.
func TestCA762_UnLocalYamlDentroDelWorktreeNoSeLee(t *testing.T) {
	const ca = "CA-762"
	root := veeRepo(t, "")
	wt := veeTarea(t, root, "demo")
	veeLocal(t, wt, "ejecutar: \"ddev exec sh -c {cmd}\"\n")
	a := veeLeer(t, ca, wt)
	if a.Existe || a.Ejecutar != "" || a.SHA256 != "" || a.ConPrefijo("test") {
		t.Fatalf("%s: el local.yaml del worktree no se lee: %+v", ca, a)
	}
	if a.Principal != root || a.Rel != ".hoom/worktrees/demo" {
		t.Fatalf("%s: el principal de la tarea sigue siendo %s y su {rel} .hoom/worktrees/demo: %+v", ca, root, a)
	}
	if got := a.Linea("go test ./...", wt); got != "go test ./..." {
		t.Fatalf("%s: sin plantilla, la linea es el comando: %q", ca, got)
	}
}

// CA-762 (regla de CA-747, aclaracion 4): `ejecutar:` sin valor, nulo, ""
// o de solo blancos cuenta como sin la clave: el archivo existe y se
// valida, pero ningun gate lleva el prefijo.
func TestCA762_EjecutarVacioONuloCuentaComoSinLaClave(t *testing.T) {
	const ca = "CA-762"
	for _, body := range []string{
		"ejecutar:\n",
		"ejecutar: ~\n",
		"ejecutar: null\n",
		"ejecutar: \"\"\n",
		"ejecutar: ''\n",
		"ejecutar: \"   \"\n",
		"ejecutar: \"\\t \\t\"\n",
		"ejecutar:\nen_el_host: [test]\n",
		"en_el_host: [test]\n",
	} {
		root := veeRepo(t, "")
		veeLocal(t, root, body)
		a := veeLeer(t, ca, root)
		if !a.Existe || a.Ejecutar != "" || a.SHA256 != veeSha(body) {
			t.Fatalf("%s: %q: existe, sin plantilla, con el sha256 de sus bytes: %+v", ca, body, a)
		}
		for _, g := range veeGates {
			if a.ConPrefijo(g) {
				t.Fatalf("%s: %q: sin plantilla ningun gate lleva prefijo (%s)", ca, body, g)
			}
		}
		if got := a.Linea("echo 'hola'", root); got != "echo 'hola'" {
			t.Fatalf("%s: %q: sin plantilla la linea es el comando tal cual: %q", ca, body, got)
		}
	}
}

// CA-762: sin el archivo no hay error: un Ajuste sin plantilla, Existe
// false, sha256 vacio.
func TestCA762_SinElArchivoComoHoy(t *testing.T) {
	const ca = "CA-762"
	root := veeRepo(t, "")
	a := veeLeer(t, ca, root)
	if a.Existe || a.Ejecutar != "" || a.SHA256 != "" || len(a.EnElHost) != 0 || a.Principal != root || a.Rel != "." {
		t.Fatalf("%s: sin .hoom/local.yaml: %+v", ca, a)
	}
}

// CA-762: fuera de un repo git se lee el de la raiz del proyecto (el
// directorio que recibe Leer), con {rel} ".".
func TestCA762_FueraDeGitSeLeeElDeLaRaiz(t *testing.T) {
	const ca = "CA-762"
	dir := veeDir(t)
	veeEscribir(t, dir, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    cmd: \"true\"\n")
	body := "ejecutar: \"" + veePlantilla + "\"\n"
	veeLocal(t, dir, body)
	a := veeLeer(t, ca, dir)
	if a.Principal != dir || !a.Existe || a.Ejecutar != veePlantilla || a.Rel != "." || a.SHA256 != veeSha(body) {
		t.Fatalf("%s: fuera de git se lee %s/.hoom/local.yaml con {rel} \".\": %+v", ca, dir, a)
	}
}

// CA-762 (aclaracion 7): con hoom.yaml en un subdirectorio del repo se lee
// <principal>/.hoom/local.yaml (no el del subdirectorio) y {rel} es la
// subruta; un worktree fuera del principal da un {rel} que empieza con
// "../".
func TestCA762_SubdirectorioYWorktreeFueraDelPrincipal(t *testing.T) {
	const ca = "CA-762"
	root := veeRepo(t, "sub/app")
	body := "ejecutar: \"" + veePlantilla + "\"\n"
	veeLocal(t, root, body)
	veeLocal(t, filepath.Join(root, "sub", "app"), "ejecutar: \"el-del-subdirectorio {cmd}\"\n")
	a := veeLeer(t, ca, filepath.Join(root, "sub", "app"))
	if a.Principal != root || a.Ejecutar != veePlantilla || a.Rel != "sub/app" {
		t.Fatalf("%s: proyecto en sub/app: el del principal y {rel} sub/app: %+v", ca, a)
	}

	root = veeRepo(t, "")
	veeLocal(t, root, body)
	fuera := filepath.Join(filepath.Dir(root), "vee-fuera")
	veeGit(t, root, "worktree", "add", "-q", "-b", "hoom/fuera", fuera, "main")
	a = veeLeer(t, ca, fuera)
	if a.Principal != root || a.Ejecutar != veePlantilla || a.Rel != "../vee-fuera" {
		t.Fatalf("%s: worktree fuera del principal: {rel} ../vee-fuera: %+v", ca, a)
	}
}

// ---------------------------------------------------------------- CA-763

// CA-763: el texto exacto de las cuatro constantes es el del spec.
func TestCA763_LosTextosDeLosErrores(t *testing.T) {
	const ca = "CA-763"
	for _, c := range []struct{ got, want string }{
		{fmt.Sprintf(entorno.ErrClaveDesconocida, "foo"), `.hoom/local.yaml: clave desconocida "foo" (validas: ejecutar, en_el_host, worktrees_relativos)`},
		{entorno.ErrSinCmd, ".hoom/local.yaml: ejecutar tiene que llevar {cmd}"},
		{fmt.Sprintf(entorno.ErrGateDesconocido, "semgrep"), `.hoom/local.yaml: en_el_host nombra "semgrep", que no es un gate de hoom.yaml`},
		{entorno.ErrNoIgnorado, ".hoom/local.yaml: git no lo ignora y es de esta maquina: agrega local.yaml al .hoom/.gitignore del checkout principal (y git rm --cached .hoom/local.yaml si esta en git)"},
	} {
		if c.got != c.want {
			t.Fatalf("%s: el texto es %q; es %q", ca, c.want, c.got)
		}
	}
	if !reflect.DeepEqual(entorno.Claves, []string{"ejecutar", "en_el_host", "worktrees_relativos"}) || entorno.Ruta != ".hoom/local.yaml" {
		t.Fatalf("%s: Claves y Ruta: %q %q", ca, entorno.Claves, entorno.Ruta)
	}
}

// CA-763 (aclaracion 5): cada error con su texto exacto, y con varios, el
// primero en el orden del spec: clave desconocida, {cmd}, en_el_host, el
// ignore. en_el_host se valida tambien sin ejecutar (aclaracion 3).
func TestCA763_LosCuatroErroresConSuTextoYSuOrden(t *testing.T) {
	const ca = "CA-763"
	clave := func(k string) string { return fmt.Sprintf(entorno.ErrClaveDesconocida, k) }
	gate := func(g string) string { return fmt.Sprintf(entorno.ErrGateDesconocido, g) }
	for _, c := range []struct {
		nombre, body string
		ignorado     bool
		want         string
	}{
		{"clave desconocida", "foo: 1\nejecutar: \"ddev {cmd}\"\n", true, clave("foo")},
		{"clave con mayuscula", "Ejecutar: \"ddev {cmd}\"\n", true, clave("Ejecutar")},
		{"clave con unicode", "ejecutár: \"ddev {cmd}\"\n", true, clave("ejecutár")},
		{"sin {cmd}", "ejecutar: \"ddev exec sh -c\"\n", true, entorno.ErrSinCmd},
		{"{CMD} no es {cmd}", "ejecutar: \"ddev exec sh -c {CMD}\"\n", true, entorno.ErrSinCmd},
		{"{ cmd } no es {cmd}", "ejecutar: \"ddev exec sh -c { cmd }\"\n", true, entorno.ErrSinCmd},
		{"solo {dir} y {rel}", "ejecutar: \"ddev exec -d {dir}/{rel} sh -c\"\n", true, entorno.ErrSinCmd},
		{"en_el_host con un gate que no es", "ejecutar: \"ddev {cmd}\"\nen_el_host: [test, semgrep]\n", true, gate("semgrep")},
		{"en_el_host sin ejecutar tambien se valida", "en_el_host: [semgrep]\n", true, gate("semgrep")},
		{"en_el_host distingue mayusculas", "en_el_host: [Test]\n", true, gate("Test")},
		{"no ignorado", "ejecutar: \"ddev {cmd}\"\n", false, entorno.ErrNoIgnorado},
		{"no ignorado y sin ejecutar", "en_el_host: [test]\n", false, entorno.ErrNoIgnorado},
		{"orden: la clave antes que todo", "foo: 1\nejecutar: \"ddev\"\nen_el_host: [semgrep]\n", false, clave("foo")},
		{"orden: {cmd} antes que en_el_host y el ignore", "ejecutar: \"ddev\"\nen_el_host: [semgrep]\n", false, entorno.ErrSinCmd},
		{"orden: en_el_host antes que el ignore", "ejecutar: \"ddev {cmd}\"\nen_el_host: [semgrep]\n", false, gate("semgrep")},
	} {
		root := veeRepo(t, "")
		if !c.ignorado {
			veeSinIgnorar(t, root)
		}
		veeLocal(t, root, c.body)
		if got := veeError(t, ca, c.nombre, root); got != c.want {
			t.Fatalf("%s: %s: el error es %q; fue %q", ca, c.nombre, c.want, got)
		}
		// desde la tarea, el mismo error (es el archivo del principal)
		wt := veeTarea(t, root, "demo")
		if got := veeError(t, ca, c.nombre+" desde la tarea", wt); got != c.want {
			t.Fatalf("%s: %s desde la tarea: el error es %q; fue %q", ca, c.nombre, c.want, got)
		}
	}
}

// CA-763: un .hoom/local.yaml que git ignora pero que esta en git
// (commiteado con -f) es ErrNoIgnorado.
func TestCA763_IgnoradoPeroRastreadoEsErrNoIgnorado(t *testing.T) {
	const ca = "CA-763"
	root := veeRepo(t, "")
	veeLocal(t, root, "ejecutar: \"ddev {cmd}\"\n")
	veeGit(t, root, "add", "-f", entorno.Ruta)
	veeGit(t, root, "commit", "-q", "-m", "el local.yaml en git")
	if got := veeError(t, ca, "rastreado", root); got != entorno.ErrNoIgnorado {
		t.Fatalf("%s: rastreado: %q; fue %q", ca, entorno.ErrNoIgnorado, got)
	}
}

// CA-763 (aclaracion 6): fuera de un repo git no hay ignore que comprobar,
// y ErrNoIgnorado nunca sale.
func TestCA763_FueraDeGitNoHayErrNoIgnorado(t *testing.T) {
	const ca = "CA-763"
	dir := veeDir(t)
	veeLocal(t, dir, "ejecutar: \"ddev {cmd}\"\nen_el_host: [build]\n")
	a := veeLeer(t, ca, dir)
	if a.Ejecutar != "ddev {cmd}" || !a.ConPrefijo("test") || a.ConPrefijo("build") {
		t.Fatalf("%s: fuera de git el archivo vale: %+v", ca, a)
	}
}

// CA-763 (aclaracion 4): lo que no se puede leer ni entender falla cerrado,
// con un error que empieza como los cuatro: ".hoom/local.yaml: ".
func TestCA763_OtrosDefectosFallanCerrados(t *testing.T) {
	const ca = "CA-763"
	const prefijo = ".hoom/local.yaml: "
	for _, c := range []struct{ nombre, body string }{
		{"YAML que no parsea", "ejecutar: \"ddev {cmd}\n"},
		{"YAML con tabs de indentacion rotos", "ejecutar:\n\t- [\n"},
		{"la raiz es una lista", "- ejecutar\n- en_el_host\n"},
		{"la raiz es un texto", "ddev exec sh -c {cmd}\n"},
		{"ejecutar es una lista", "ejecutar: [ddev, \"{cmd}\"]\n"},
		{"ejecutar es un mapa", "ejecutar: {cmd: ddev}\n"},
		{"en_el_host es un texto", "en_el_host: test\n"},
		{"en_el_host es un mapa", "en_el_host: {test: true}\n"},
		{"en_el_host tiene una lista adentro", "en_el_host: [[test]]\n"},
	} {
		root := veeRepo(t, "")
		veeLocal(t, root, c.body)
		if got := veeError(t, ca, c.nombre, root); !strings.HasPrefix(got, prefijo) {
			t.Fatalf("%s: %s: el error empieza con %q: %q", ca, c.nombre, prefijo, got)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	// un directorio, un symlink (aunque apunte a un archivo valido) y uno
	// que no se puede leer
	root := veeRepo(t, "")
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "local.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := veeError(t, ca, "un directorio", root); !strings.HasPrefix(got, prefijo) {
		t.Fatalf("%s: un directorio: %q", ca, got)
	}
	root = veeRepo(t, "")
	valido := filepath.Join(t.TempDir(), "valido.yaml")
	if err := os.WriteFile(valido, []byte("ejecutar: \"ddev {cmd}\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(valido, filepath.Join(root, ".hoom", "local.yaml")); err != nil {
		t.Fatal(err)
	}
	if got := veeError(t, ca, "un symlink", root); !strings.HasPrefix(got, prefijo) {
		t.Fatalf("%s: un symlink no es un archivo regular: %q", ca, got)
	}
	if os.Geteuid() != 0 {
		root = veeRepo(t, "")
		veeLocal(t, root, "ejecutar: \"ddev {cmd}\"\n")
		p := filepath.Join(root, ".hoom", "local.yaml")
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(p, 0o644) })
		if got := veeError(t, ca, "ilegible", root); !strings.HasPrefix(got, prefijo) {
			t.Fatalf("%s: uno que no se puede leer: %q", ca, got)
		}
	}
}

// ---------------------------------------------------------------- CA-764

// CA-764: Linea reemplaza {cmd}, {dir} y {rel}, cada una como una palabra
// de sh entre comillas simples, con la comilla de adentro cerrada,
// escapada y reabierta; cada
// aparicion; en una pasada (una marca dentro de un valor no se vuelve a
// reemplazar). Sin plantilla, o con un Ajuste nil, la linea es el comando.
func TestCA764_LineaCitaCadaMarcaComoUnaPalabra(t *testing.T) {
	const ca = "CA-764"
	en := func(ejecutar, rel string) *entorno.Ajuste {
		return &entorno.Ajuste{Existe: true, Ejecutar: ejecutar, Rel: rel}
	}
	for _, c := range []struct {
		nombre string
		a      *entorno.Ajuste
		cmd    string
		dir    string
		want   string
	}{
		{"el ejemplo del go doc", en("sh -c {cmd}", "."), "echo it's", "/p", `sh -c 'echo it'\''s'`},
		{"ddev en el principal", en(veePlantilla, "."), "go test ./...", "/p", `ddev exec -d /var/www/html/'.' sh -c 'go test ./...'`},
		{"ddev en una tarea", en(veePlantilla, ".hoom/worktrees/demo"), "php artisan test", "/p/.hoom/worktrees/demo",
			`ddev exec -d /var/www/html/'.hoom/worktrees/demo' sh -c 'php artisan test'`},
		{"las tres marcas", en("x {dir} {rel} {cmd}", "sub"), "a b", "/d i r", `x '/d i r' 'sub' 'a b'`},
		{"cada aparicion", en("{cmd} && {cmd} {dir}{dir}", "."), "c", "/d", `'c' && 'c' '/d''/d'`},
		{"una pasada: marcas dentro de cmd", en("run {cmd}", "."), "echo {dir} {rel} {cmd}", "/d", `run 'echo {dir} {rel} {cmd}'`},
		{"una pasada: marcas dentro de dir y rel", en("{dir} {rel} {cmd}", "{cmd}"), "c", "/x/{rel}", `'/x/{rel}' '{cmd}' 'c'`},
		{"comillas en dir y rel", en("{dir}|{rel}|{cmd}", "it's"), "''", "/a'b", `'/a'\''b'|'it'\''s'|''\'''\'''`},
		{"cmd vacio", en("sh -c {cmd}", "."), "", "/d", `sh -c ''`},
		{"sin plantilla", en("", "."), "echo it's", "/d", "echo it's"},
		{"Ajuste cero", &entorno.Ajuste{}, "echo it's", "/d", "echo it's"},
		{"Ajuste nil", nil, "echo it's", "/d", "echo it's"},
	} {
		if got := c.a.Linea(c.cmd, c.dir); got != c.want {
			t.Fatalf("%s: %s: Linea(%q, %q) es %q; fue %q", ca, c.nombre, c.cmd, c.dir, c.want, got)
		}
	}
}

// veeValor es un texto hostil para sh: comillas, $, backticks,
// sustituciones, saltos de linea, globs, marcas de la plantilla y unicode.
type veeValor string

func (veeValor) Generate(r *rand.Rand, size int) reflect.Value {
	piezas := []string{"a", "Z", "0", " ", "  ", "\t", "\n", "'", "''", "'\\''", "\"", "\\", "\\'", "$", "$HOME",
		"${x}", "`id`", "$(echo no)", ";", "&", "|", "<", ">", "*", "?", "[a]", "#", "~", "!", "%", "%s", "\\n",
		"{cmd}", "{dir}", "{rel}", "{", "}", "ñ", "日本", "🚀", "-n", "--", "=", ".", "/"}
	n := r.Intn(14)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(piezas[r.Intn(len(piezas))])
	}
	return reflect.ValueOf(veeValor(b.String()))
}

// CA-764 y caso limite "una comilla simple en el comando": para cualquier
// valor, el sh que corre la linea recibe {cmd}, {dir} y {rel} intactos,
// cada uno como un solo argumento. Propiedad, no ejemplos.
func TestCA764_ShRecibeCadaValorIntacto(t *testing.T) {
	const ca = "CA-764"
	if runtime.GOOS == "windows" {
		t.Skip("la propiedad se mide con sh")
	}
	f := func(cmd, dir, rel veeValor) bool {
		a := &entorno.Ajuste{Existe: true, Ejecutar: `printf '%s\000%s\000%s' {cmd} {dir} {rel}`, Rel: string(rel)}
		linea := a.Linea(string(cmd), string(dir))
		out, err := exec.Command("sh", "-c", linea).Output()
		want := string(cmd) + "\x00" + string(dir) + "\x00" + string(rel)
		if err != nil || string(out) != want {
			t.Logf("cmd %q dir %q rel %q\nlinea %q\nsalida %q (%v)", cmd, dir, rel, linea, out, err)
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}

// CA-764: ConPrefijo es "tiene plantilla y el gate no esta en en_el_host";
// false con un Ajuste nil o sin plantilla.
func TestCA764_ConPrefijoDejaAfueraAEnElHost(t *testing.T) {
	const ca = "CA-764"
	var nada *entorno.Ajuste
	if nada.ConPrefijo("test") || (&entorno.Ajuste{}).ConPrefijo("test") ||
		(&entorno.Ajuste{Existe: true, EnElHost: []string{"build"}}).ConPrefijo("test") {
		t.Fatalf("%s: sin plantilla ningun gate lleva prefijo", ca)
	}
	a := &entorno.Ajuste{Existe: true, Ejecutar: "ddev {cmd}", EnElHost: []string{"semgrep", "build"}}
	for g, want := range map[string]bool{"test": true, "lint": true, "semgrep": false, "build": false, "Semgrep": true} {
		if a.ConPrefijo(g) != want {
			t.Fatalf("%s: ConPrefijo(%q) es %v", ca, g, want)
		}
	}
}

// CA-764: el {rel} que da Leer es el que entra en la linea: el principal da
// '.', la tarea '.hoom/worktrees/<slug>'.
func TestCA764_ElRelDeLeerEntraEnLaLinea(t *testing.T) {
	const ca = "CA-764"
	root := veeRepo(t, "")
	wt := veeTarea(t, root, "precios")
	veeLocal(t, root, "ejecutar: \""+veePlantilla+"\"\n")
	if got, want := veeLeer(t, ca, root).Linea("echo x", root), `ddev exec -d /var/www/html/'.' sh -c 'echo x'`; got != want {
		t.Fatalf("%s: en el principal: %q; fue %q", ca, want, got)
	}
	if got, want := veeLeer(t, ca, wt).Linea("echo x", wt), `ddev exec -d /var/www/html/'.hoom/worktrees/precios' sh -c 'echo x'`; got != want {
		t.Fatalf("%s: en la tarea: %q; fue %q", ca, want, got)
	}
}

// ---------------------------------------------------------------- CA-771

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

// CA-771: el error de `hoom task start` con la clave y un git viejo es, al
// pie de la letra, el del spec.
func TestCA771_ElErrorDeGitViejoEsElDelSpec(t *testing.T) {
	const ca = "CA-771"
	if want := ".hoom/local.yaml: worktrees_relativos pide git 2.48 o mas nuevo"; entorno.ErrWorktreesRelativosGitViejo != want {
		t.Fatalf("%s: el texto es %q; es %q", ca, want, entorno.ErrWorktreesRelativosGitViejo)
	}
}

// CA-771 (go doc de Ajuste.WorktreesRelativos): worktrees_relativos: true,
// un booleano de YAML (true, True o TRUE), deja WorktreesRelativos en true,
// sin error y con el sha256 de los bytes del archivo. Desde la tarea es el
// del principal; el del worktree (con false) no se lee. No toca verify: sin
// ejecutar ningun gate lleva prefijo, y con ejecutar la plantilla es la de
// siempre. Leer nunca devuelve ErrWorktreesRelativosGitViejo, tampoco con un
// git que dice 2.47: el archivo es valido, y ese error es de `hoom task
// start`.
func TestCA771_WorktreesRelativosTrueEsTrue(t *testing.T) {
	const ca = "CA-771"
	for _, viejo := range []bool{false, true} {
		nombre := "con el git real"
		if viejo {
			nombre = "con un git que dice 2.47"
		}
		t.Run(nombre, func(t *testing.T) {
			if viejo {
				veeGitViejo(t)
			}
			for _, c := range []struct{ body, ejecutar string }{
				{"worktrees_relativos: true\n", ""},
				{"worktrees_relativos: True\n", ""},
				{"worktrees_relativos: TRUE\n", ""},
				{"en_el_host: [build]\nworktrees_relativos: true\n", ""},
				{"ejecutar: \"" + veePlantilla + "\"\nworktrees_relativos: true\n", veePlantilla},
			} {
				root := veeRepo(t, "")
				wt := veeTarea(t, root, "demo")
				veeLocal(t, wt, "worktrees_relativos: false\n")
				veeLocal(t, root, c.body)
				for _, dir := range []string{root, wt} {
					a := veeLeer(t, ca, dir)
					if !a.WorktreesRelativos || !a.Existe || a.Principal != root || a.SHA256 != veeSha(c.body) {
						t.Fatalf("%s: %q desde %s: WorktreesRelativos true, del principal, con su sha256: %+v", ca, c.body, dir, a)
					}
					if a.Ejecutar != c.ejecutar || a.ConPrefijo("test") != (c.ejecutar != "") {
						t.Fatalf("%s: %q desde %s: worktrees_relativos no cambia la plantilla (%q): %+v", ca, c.body, dir, c.ejecutar, a)
					}
				}
			}
		})
	}
}

// CA-771: false, sin valor, nulo, "" o solo blancos cuentan como sin la
// clave: WorktreesRelativos false, sin error, y el archivo vale (Existe,
// con su sha256). Sin el archivo tambien es false, y el .hoom/local.yaml
// del worktree (con true) no se lee.
func TestCA771_WorktreesRelativosFalsoVacioONuloEsFalse(t *testing.T) {
	const ca = "CA-771"
	for _, body := range []string{
		"worktrees_relativos: false\n",
		"worktrees_relativos:\n",
		"worktrees_relativos: ~\n",
		"worktrees_relativos: null\n",
		"worktrees_relativos: \"\"\n",
		"worktrees_relativos: ''\n",
		"worktrees_relativos: \"   \"\n",
		"worktrees_relativos: \"\\t \\t\"\n",
		"ejecutar: \"" + veePlantilla + "\"\nworktrees_relativos: false\n",
		"ejecutar: \"" + veePlantilla + "\"\n",
		"en_el_host: [test]\n",
	} {
		root := veeRepo(t, "")
		veeLocal(t, root, body)
		a := veeLeer(t, ca, root)
		if a.WorktreesRelativos || !a.Existe || a.SHA256 != veeSha(body) {
			t.Fatalf("%s: %q: el archivo vale y WorktreesRelativos es false: %+v", ca, body, a)
		}
	}
	root := veeRepo(t, "")
	wt := veeTarea(t, root, "demo")
	veeLocal(t, wt, "worktrees_relativos: true\n")
	for _, dir := range []string{root, wt} {
		a := veeLeer(t, ca, dir)
		if a.WorktreesRelativos || a.Existe {
			t.Fatalf("%s: sin el archivo del principal (desde %s), WorktreesRelativos es false aunque el worktree tenga uno con true: %+v", ca, dir, a)
		}
	}
}

// CA-771 (go doc de Ajuste.WorktreesRelativos, aclaracion 4): cualquier
// otro valor falla cerrado, con un error que empieza con
// ".hoom/local.yaml: " y que no es el de clave desconocida (la clave es
// valida) ni el de git viejo: un "true" entre comillas, yes, on, no, off,
// y, un numero, una lista o un mapa. Desde el principal, desde la tarea y
// fuera de git.
func TestCA771_WorktreesRelativosQueNoEsBooleanoFallaCerrado(t *testing.T) {
	const ca = "CA-771"
	const prefijo = ".hoom/local.yaml: "
	for _, body := range []string{
		"worktrees_relativos: \"true\"\n",
		"worktrees_relativos: 'true'\n",
		"worktrees_relativos: \"false\"\n",
		"worktrees_relativos: yes\n",
		"worktrees_relativos: on\n",
		"worktrees_relativos: no\n",
		"worktrees_relativos: off\n",
		"worktrees_relativos: y\n",
		"worktrees_relativos: 1\n",
		"worktrees_relativos: 0\n",
		"worktrees_relativos: 2.48\n",
		"worktrees_relativos: [true]\n",
		"worktrees_relativos:\n  - true\n",
		"worktrees_relativos: {valor: true}\n",
		"worktrees_relativos:\n  valor: true\n",
		"ejecutar: \"" + veePlantilla + "\"\nworktrees_relativos: si\n",
	} {
		root := veeRepo(t, "")
		wt := veeTarea(t, root, "demo")
		veeLocal(t, root, body)
		fuera := veeDir(t)
		veeLocal(t, fuera, body)
		for _, dir := range []string{root, wt, fuera} {
			got := veeError(t, ca, fmt.Sprintf("%q desde %s", body, dir), dir)
			if !strings.HasPrefix(got, prefijo) || got == fmt.Sprintf(entorno.ErrClaveDesconocida, "worktrees_relativos") ||
				got == entorno.ErrWorktreesRelativosGitViejo {
				t.Fatalf("%s: %q desde %s: falla cerrado con un error que empieza con %q, sin ser el de la clave ni el de git viejo: %q", ca, body, dir, prefijo, got)
			}
		}
	}
}

// CA-771 y CA-763: la clave se escribe como en el spec y distingue
// mayusculas: otra grafia es una clave desconocida, con el texto exacto que
// ya nombra las tres validas.
func TestCA771_LaClaveSeEscribeComoEnElSpec(t *testing.T) {
	const ca = "CA-771"
	for _, k := range []string{"Worktrees_relativos", "WORKTREES_RELATIVOS", "worktrees-relativos", "worktree_relativos", "worktrees_relativo", "relative_paths"} {
		root := veeRepo(t, "")
		veeLocal(t, root, k+": true\n")
		want := fmt.Sprintf(entorno.ErrClaveDesconocida, k)
		if got := veeError(t, ca, k, root); got != want {
			t.Fatalf("%s: %s: el error es %q; fue %q", ca, k, want, got)
		}
	}
	if got, want := fmt.Sprintf(entorno.ErrClaveDesconocida, "Worktrees_relativos"),
		`.hoom/local.yaml: clave desconocida "Worktrees_relativos" (validas: ejecutar, en_el_host, worktrees_relativos)`; got != want {
		t.Fatalf("%s: el texto es %q; es %q", ca, want, got)
	}
}
