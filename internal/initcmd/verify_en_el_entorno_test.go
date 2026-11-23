// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// `hoom init` (initcmd.Run):
//
//   - CA-770: un .hoom/.gitignore nuevo es hoomfs.GitignoreBody (con
//     local.yaml); a uno que ya existe se le agrega la linea local.yaml si le
//     falta, y nada mas cambia; si la tiene, queda byte a byte;
//   - CA-770 (enmienda 55b134e2, ronda 0d): con el proyecto en un
//     subdirectorio del repo, init deja local.yaml tambien en el
//     .hoom/.gitignore del checkout principal (lo crea con esa sola linea
//     si no existe) y despues Leer no da ErrNoIgnorado; en la raiz del repo
//     y fuera de git, como hoy; un fallo al escribir cualquiera de los dos
//     ignores es un error de init;
//   - CA-769: al final imprime la Sugerencia de cada entorno del perfil que
//     se cumple en el directorio (aclaracion 12: ddev y sail, en ese orden),
//     y nunca escribe .hoom/local.yaml.
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion. No se fija a que stream va la sugerencia: se lee
// stdout y stderr juntos.
package initcmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	hentorno "github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/profiles"
)

// veeCorrer corre Run capturando stdout y stderr juntos.
func veeCorrer(t *testing.T, dir, perfil string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	viejoOut, viejoErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	leido := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		leido <- string(b)
	}()
	runErr := Run(dir, "demo", perfil)
	os.Stdout, os.Stderr = viejoOut, viejoErr
	w.Close()
	out := <-leido
	r.Close()
	return out, runErr
}

// veeSinPHP deja un PATH con las herramientas del sistema que init pueda
// usar (git, sh...) y sin php.
func veeSinPHP(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	for _, n := range []string{"git", "sh", "bash", "env", "cat", "ls", "mkdir", "rm", "cp", "mv", "uname", "id", "tr", "sed", "grep", "head", "tail", "dirname", "basename", "printf", "true", "false", "test"} {
		if p, err := exec.LookPath(n); err == nil {
			_ = os.Symlink(p, filepath.Join(bin, n))
		}
	}
	t.Setenv("PATH", bin)
	return bin
}

func veeEscribir(t *testing.T, dir, rel, body string, modo os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), modo); err != nil {
		t.Fatal(err)
	}
}

func veeLeer(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// CA-770: hoom init en un proyecto nuevo deja .hoom/.gitignore igual a
// hoomfs.GitignoreBody, con la linea local.yaml.
func TestCA770_InitNuevoDejaElGitignoreCanonico(t *testing.T) {
	const ca = "CA-770"
	dir := t.TempDir()
	if _, err := veeCorrer(t, dir, "go"); err != nil {
		t.Fatalf("%s: hoom init: %v", ca, err)
	}
	got := veeLeer(t, filepath.Join(dir, ".hoom", ".gitignore"))
	if got != hoomfs.GitignoreBody() || !hrTieneRegla(got, "local.yaml") {
		t.Fatalf("%s: el .gitignore nuevo es GitignoreBody con local.yaml: %q", ca, got)
	}
}

// CA-770: a un .hoom/.gitignore que ya existe, hoom init le agrega la linea
// local.yaml si le falta y nada mas cambia; si la tiene, queda igual.
func TestCA770_InitConGitignoreExistente(t *testing.T) {
	const ca = "CA-770"
	for _, c := range []struct{ antes, despues string }{
		{"cache/\nmi-cosa.log\n", "cache/\nmi-cosa.log\nlocal.yaml\n"},
		{"worktrees/\n# de esta maquina\n", "worktrees/\n# de esta maquina\nlocal.yaml\n"},
		{"cache/\n  local.yaml\nmi-cosa\n", "cache/\n  local.yaml\nmi-cosa\n"},
		{hoomfs.GitignoreBody(), hoomfs.GitignoreBody()},
	} {
		dir := t.TempDir()
		veeEscribir(t, dir, ".hoom/.gitignore", c.antes, 0o644)
		if _, err := veeCorrer(t, dir, "go"); err != nil {
			t.Fatalf("%s: hoom init con .gitignore %q: %v", ca, c.antes, err)
		}
		if got := veeLeer(t, filepath.Join(dir, ".hoom", ".gitignore")); got != c.despues {
			t.Fatalf("%s: con %q el .gitignore queda %q; fue %q", ca, c.antes, c.despues, got)
		}
	}
}

// CA-769 y aclaracion 12: hoom init de un proyecto laravel sin php en el
// PATH imprime al final la sugerencia de cada entorno que se cumple, en el
// orden del perfil, una vez cada una, y no escribe .hoom/local.yaml; sin
// los archivos, o con php en el PATH, no sugiere nada.
func TestCA769_InitImprimeLasSugerenciasAlFinal(t *testing.T) {
	const ca = "CA-769"
	if runtime.GOOS == "windows" {
		t.Skip("el php falso es un archivo con permiso de ejecucion")
	}
	ents, err := profiles.Entornos("laravel")
	if err != nil || len(ents) != 2 || ents[0].Nombre != "ddev" || ents[1].Nombre != "sail" {
		t.Fatalf("%s: fixture: laravel declara ddev y sail: %+v %v", ca, ents, err)
	}
	ddev, sail := ents[0].Sugerencia(), ents[1].Sugerencia()
	bin := veeSinPHP(t)
	for _, c := range []struct {
		nombre   string
		archivos []string
		want     []string
	}{
		{"ddev", []string{".ddev/config.yaml"}, []string{ddev}},
		{"sail", []string{"vendor/bin/sail"}, []string{sail}},
		{"los dos", []string{".ddev/config.yaml", "vendor/bin/sail"}, []string{ddev, sail}},
		{"ninguno", []string{"composer.json"}, nil},
	} {
		dir := t.TempDir()
		veeEscribir(t, dir, "composer.json", "{}\n", 0o644)
		for _, a := range c.archivos {
			veeEscribir(t, dir, a, "x\n", 0o755)
		}
		out, err := veeCorrer(t, dir, "laravel")
		if err != nil {
			t.Fatalf("%s: %s: hoom init: %v\n%s", ca, c.nombre, err, out)
		}
		if n := strings.Count(out, "sugerencia: "); n != len(c.want) {
			t.Fatalf("%s: %s: %d sugerencias, una por entorno que se cumple: %d\n%s", ca, c.nombre, len(c.want), n, out)
		}
		fin := 0
		for _, s := range c.want {
			i := strings.Index(out, s)
			if i < fin {
				t.Fatalf("%s: %s: la sugerencia, en el orden del perfil:\n%s\n--- salida ---\n%s", ca, c.nombre, s, out)
			}
			fin = i + len(s)
		}
		if len(c.want) > 0 && strings.TrimSpace(out[fin:]) != "" {
			t.Fatalf("%s: %s: las sugerencias van al final: sobra %q", ca, c.nombre, out[fin:])
		}
		if _, err := os.Lstat(filepath.Join(dir, ".hoom", "local.yaml")); err == nil {
			t.Fatalf("%s: %s: hoom init nunca escribe .hoom/local.yaml", ca, c.nombre)
		}
	}

	veeEscribir(t, bin, "php", "#!/bin/sh\nexit 0\n", 0o755)
	dir := t.TempDir()
	veeEscribir(t, dir, "composer.json", "{}\n", 0o644)
	veeEscribir(t, dir, ".ddev/config.yaml", "x\n", 0o644)
	out, err := veeCorrer(t, dir, "laravel")
	if err != nil || strings.Contains(out, "sugerencia: ") {
		t.Fatalf("%s: con php en el PATH no hay sugerencia: %v\n%s", ca, err, out)
	}
}

// ---------------------------------------------------------------- CA-770, subdirectorio

// veeLocalValido es un .hoom/local.yaml valido con cualquier hoom.yaml: sin
// en_el_host, con {cmd}.
const veeLocalValido = "ejecutar: \"ddev exec -d /var/www/html/{rel} sh -c {cmd}\"\n"

// veeAislarGit deja a git, el de los tests y el que corren init y Leer, sin
// la config global ni la del sistema, y sin el excludes de quien corre los
// tests ($XDG_CONFIG_HOME/git/ignore). Usa t.Setenv: el test no es paralelo.
func veeAislarGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func veeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (en %s): %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// veeDirReal es un t.TempDir sin symlinks en su ruta.
func veeDirReal(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// veeRepo es un repo git (rama main) en una carpeta repo dentro de base, con
// un commit inicial; si gitignorePrincipal no es "", el commit trae ese
// <repo>/.hoom/.gitignore. Devuelve base, repo y la carpeta del proyecto,
// <repo>/<sub> (sub "" = la raiz del repo), creada y vacia.
func veeRepo(t *testing.T, sub, gitignorePrincipal string) (base, repo, proyecto string) {
	t.Helper()
	veeAislarGit(t)
	base = veeDirReal(t)
	repo = filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	veeGit(t, repo, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, repo)
	veeGit(t, repo, "config", "user.email", "test@hoom.dev")
	veeGit(t, repo, "config", "user.name", "hoom test")
	veeEscribir(t, repo, "README.md", "# repo\n", 0o644)
	if gitignorePrincipal != "" {
		veeEscribir(t, repo, ".hoom/.gitignore", gitignorePrincipal, 0o644)
	}
	veeGit(t, repo, "add", "-A")
	veeGit(t, repo, "commit", "-q", "-m", "inicial")
	proyecto = repo
	if sub != "" {
		proyecto = filepath.Join(repo, filepath.FromSlash(sub))
		if err := os.MkdirAll(proyecto, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return base, repo, proyecto
}

// veeIgnora dice si el git de repo ignora rel (`git check-ignore -q`): exit
// 0 si, 1 no; cualquier otra cosa corta el test.
func veeIgnora(t *testing.T, repo, rel string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", rel)
	cmd.Dir = repo
	err := cmd.Run()
	if err == nil {
		return true
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false
	}
	t.Fatalf("git check-ignore %s (en %s): %v", rel, repo, err)
	return false
}

// veeLineas son las lineas no vacias de s, sin el \r del final.
func veeLineas(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func veeMismas(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// veeExigeLocalIgnorado exige que, con un <repo>/.hoom/local.yaml valido sin
// commitear, entorno.Leer desde el proyecto lo lea sin error (sin
// ErrNoIgnorado) y que git lo ignore.
func veeExigeLocalIgnorado(t *testing.T, ca, caso, repo, proyecto string) {
	t.Helper()
	veeEscribir(t, repo, hentorno.Ruta, veeLocalValido, 0o644)
	a, err := hentorno.Leer(proyecto, nil)
	if err != nil {
		t.Fatalf("%s: %s: despues de init, el .hoom/local.yaml del principal no da error en Leer (ErrNoIgnorado es %q): %v", ca, caso, hentorno.ErrNoIgnorado, err)
	}
	if a == nil || !a.Existe || a.Principal != repo {
		t.Fatalf("%s: %s: Leer lee el .hoom/local.yaml de %s: %+v", ca, caso, repo, a)
	}
	if !veeIgnora(t, repo, hentorno.Ruta) {
		t.Fatalf("%s: %s: despues de init, git check-ignore ignora %s", ca, caso, hentorno.Ruta)
	}
}

// CA-770 (enmienda): con el proyecto en <repo>/sub/app y sin
// <repo>/.hoom/.gitignore, init deja el .hoom/.gitignore del proyecto como
// hoy (GitignoreBody, con local.yaml) y crea el del principal con UNA sola
// linea no vacia, local.yaml; no crea ninguno en <repo>/sub. Despues, un
// <repo>/.hoom/local.yaml valido sin commitear no da ErrNoIgnorado y git lo
// ignora.
func TestCA770_SubdirectorioCreaElGitignoreDelPrincipal(t *testing.T) {
	const ca = "CA-770"
	_, repo, proyecto := veeRepo(t, "sub/app", "")
	if veeIgnora(t, repo, hentorno.Ruta) {
		t.Fatalf("fixture: antes de init, git no ignora %s en %s", hentorno.Ruta, repo)
	}
	if out, err := veeCorrer(t, proyecto, "go"); err != nil {
		t.Fatalf("%s: hoom init en sub/app: %v\n%s", ca, err, out)
	}
	if got := veeLeer(t, filepath.Join(proyecto, ".hoom", ".gitignore")); got != hoomfs.GitignoreBody() || !hrTieneRegla(got, "local.yaml") {
		t.Fatalf("%s: el .hoom/.gitignore del proyecto es el de hoy, GitignoreBody con local.yaml: %q", ca, got)
	}
	got := veeLeer(t, filepath.Join(repo, ".hoom", ".gitignore"))
	if ls := veeLineas(got); !veeMismas(ls, []string{"local.yaml"}) {
		t.Fatalf("%s: el .hoom/.gitignore del principal se crea con una sola linea, local.yaml: %q", ca, got)
	}
	if _, err := os.Lstat(filepath.Join(repo, "sub", ".hoom")); err == nil {
		t.Fatalf("%s: init no crea nada en %s: el ignore va en el checkout principal", ca, filepath.Join(repo, "sub", ".hoom"))
	}
	veeExigeLocalIgnorado(t, ca, "sub/app sin ignore en el principal", repo, proyecto)
}

// CA-770 (enmienda): con el proyecto en <repo>/sub/app y un
// <repo>/.hoom/.gitignore que existe sin local.yaml, init lo deja con sus
// lineas de antes y ademas local.yaml (tambien sin salto de linea al
// final); si ya tiene la linea, sus bytes no cambian. En todos, despues, un
// <repo>/.hoom/local.yaml valido no da ErrNoIgnorado y git lo ignora.
func TestCA770_SubdirectorioCompletaElGitignoreDelPrincipal(t *testing.T) {
	const ca = "CA-770"
	for _, c := range []struct {
		nombre, antes string
		yaLaTiene     bool
	}{
		{"sin la linea", "cache/\nmi-cosa.log\n", false},
		{"sin la linea ni salto final", "worktrees/\n# de esta maquina", false},
		{"un comentario que la nombra no cuenta", "# local.yaml\nruns/\n", false},
		{"ya la tiene", "cache/\nlocal.yaml\nmi-cosa\n", true},
		{"es el canonico", hoomfs.GitignoreBody(), true},
	} {
		_, repo, proyecto := veeRepo(t, "sub/app", c.antes)
		if out, err := veeCorrer(t, proyecto, "go"); err != nil {
			t.Fatalf("%s: %s: hoom init en sub/app: %v\n%s", ca, c.nombre, err, out)
		}
		if got := veeLeer(t, filepath.Join(proyecto, ".hoom", ".gitignore")); !hrTieneRegla(got, "local.yaml") {
			t.Fatalf("%s: %s: el .hoom/.gitignore del proyecto tiene local.yaml, como hoy: %q", ca, c.nombre, got)
		}
		got := veeLeer(t, filepath.Join(repo, ".hoom", ".gitignore"))
		if c.yaLaTiene {
			if got != c.antes {
				t.Fatalf("%s: %s: con la linea, el .hoom/.gitignore del principal queda byte a byte: %q; fue %q", ca, c.nombre, c.antes, got)
			}
		} else if want := append(veeLineas(c.antes), "local.yaml"); !veeMismas(veeLineas(got), want) {
			t.Fatalf("%s: %s: el .hoom/.gitignore del principal queda con sus lineas y local.yaml %q: %q", ca, c.nombre, want, got)
		}
		veeExigeLocalIgnorado(t, ca, c.nombre, repo, proyecto)
	}
}

// CA-770: con el proyecto en la raiz del repo, todo queda como hoy: un solo
// .hoom/.gitignore, el del proyecto (nuevo: GitignoreBody; existente: sus
// lineas y local.yaml), y nada fuera del repo. Despues, un .hoom/local.yaml
// valido no da ErrNoIgnorado.
func TestCA770_EnLaRaizDelRepoUnSoloGitignore(t *testing.T) {
	const ca = "CA-770"
	for _, antes := range []string{"", "cache/\nmi-cosa.log\n"} {
		base, repo, proyecto := veeRepo(t, "", antes)
		if out, err := veeCorrer(t, proyecto, "go"); err != nil {
			t.Fatalf("%s: hoom init en la raiz del repo (%q): %v\n%s", ca, antes, err, out)
		}
		got := veeLeer(t, filepath.Join(repo, ".hoom", ".gitignore"))
		switch {
		case antes == "" && got != hoomfs.GitignoreBody():
			t.Fatalf("%s: en la raiz, el .hoom/.gitignore nuevo es GitignoreBody, como hoy: %q", ca, got)
		case antes != "" && !veeMismas(veeLineas(got), append(veeLineas(antes), "local.yaml")):
			t.Fatalf("%s: en la raiz, el .hoom/.gitignore existente queda con sus lineas y local.yaml: %q", ca, got)
		}
		ents, err := os.ReadDir(base)
		if err != nil {
			t.Fatal(err)
		}
		if len(ents) != 1 || ents[0].Name() != "repo" {
			t.Fatalf("%s: en la raiz, init no escribe nada fuera del repo: %v", ca, ents)
		}
		veeExigeLocalIgnorado(t, ca, fmt.Sprintf("en la raiz (%q)", antes), repo, proyecto)
	}
}

// CA-770: fuera de git, init no escribe nada fuera de la carpeta del
// proyecto: ni en <base>/.hoom ni en <base>/sub/.hoom. El .hoom/.gitignore
// del proyecto es el de hoy.
func TestCA770_FueraDeGitNoEscribeFueraDelProyecto(t *testing.T) {
	const ca = "CA-770"
	veeAislarGit(t)
	base := veeDirReal(t)
	if err := exec.Command("git", "-C", base, "rev-parse", "--is-inside-work-tree").Run(); err == nil {
		t.Skip("fixture: el directorio temporal esta dentro de un repo git")
	}
	proyecto := filepath.Join(base, "sub", "app")
	if err := os.MkdirAll(proyecto, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := veeCorrer(t, proyecto, "go"); err != nil {
		t.Fatalf("%s: hoom init fuera de git: %v\n%s", ca, err, out)
	}
	if got := veeLeer(t, filepath.Join(proyecto, ".hoom", ".gitignore")); got != hoomfs.GitignoreBody() {
		t.Fatalf("%s: fuera de git, el .hoom/.gitignore del proyecto es GitignoreBody: %q", ca, got)
	}
	var fuera []string
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == proyecto {
			return filepath.SkipDir
		}
		if rel, _ := filepath.Rel(base, p); rel != "." && rel != "sub" {
			fuera = append(fuera, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fuera) != 0 {
		t.Fatalf("%s: fuera de git, init no escribe nada fuera de %s: %q", ca, proyecto, fuera)
	}
}

// CA-770 (enmienda: «un fallo al escribirlo es un error de hoom init»): con
// el proyecto en <repo>/sub/app, si el .hoom/.gitignore del principal no se
// puede escribir (<repo>/.hoom es un archivo, o <repo>/.hoom/.gitignore es
// una carpeta), initcmd.Run devuelve un error, no un aviso con exito. No se
// fija el texto.
func TestCA770_FalloAlEscribirElIgnoreDelPrincipalEsError(t *testing.T) {
	const ca = "CA-770"
	for _, c := range []struct {
		nombre string
		armar  func(t *testing.T, repo string)
	}{
		{"<repo>/.hoom es un archivo", func(t *testing.T, repo string) {
			veeEscribir(t, repo, ".hoom", "no soy una carpeta\n", 0o644)
		}},
		{"<repo>/.hoom/.gitignore es una carpeta", func(t *testing.T, repo string) {
			if err := os.MkdirAll(filepath.Join(repo, ".hoom", ".gitignore"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		_, repo, proyecto := veeRepo(t, "sub/app", "")
		c.armar(t, repo)
		if out, err := veeCorrer(t, proyecto, "go"); err == nil {
			t.Fatalf("%s: %s: init en sub/app falla, no avisa con exito:\n%s", ca, c.nombre, out)
		}
	}
}

// CA-770 (enmienda; antes era un aviso, hallazgo 55b774): si el
// .hoom/.gitignore del proyecto no se puede escribir (es una carpeta),
// initcmd.Run devuelve un error, no un aviso con exito: fuera de git, en la
// raiz del repo y en un subdirectorio. No se fija el texto.
func TestCA770_FalloAlEscribirElIgnoreDelProyectoEsError(t *testing.T) {
	const ca = "CA-770"
	for _, donde := range []string{"fuera de git", "raiz del repo", "subdirectorio"} {
		var proyecto string
		switch donde {
		case "fuera de git":
			veeAislarGit(t)
			proyecto = veeDirReal(t)
		case "raiz del repo":
			_, _, proyecto = veeRepo(t, "", "")
		default:
			_, _, proyecto = veeRepo(t, "sub/app", "")
		}
		if err := os.MkdirAll(filepath.Join(proyecto, ".hoom", ".gitignore"), 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := veeCorrer(t, proyecto, "go"); err == nil {
			t.Fatalf("%s: %s: con <proyecto>/.hoom/.gitignore una carpeta, init falla, no avisa con exito:\n%s", ca, donde, out)
		}
	}
}
