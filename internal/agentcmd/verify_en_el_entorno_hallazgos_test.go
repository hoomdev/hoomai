// Regresiones de los hallazgos de la review completa de verify-en-el-entorno
// (6e455e) sobre el sobre de `hoom agent`, escritas por el writer en la ronda
// a mano:
//
//   - bd3025 (CA-768): en un proyecto que vive en un subdirectorio de su
//     repo (hoom.yaml en sub/app), el sobre fotografia el .hoom/local.yaml
//     del checkout principal, el que lee verify (entorno.Principal), no uno
//     en sub/app: un rol que cambia <repo>/.hoom/local.yaml corta en scope
//     por manipulacion;
//   - acc76d (CA-768): FotoLocal no lee entero un .hoom/local.yaml enorme
//     (un disperso de 1 EiB, lo que deja `truncate -s 1E`): vuelve enseguida
//     con una huella distinta de la del archivo chico, y el sobre de un rol
//     que lo agranda asi vuelve y corta en scope por manipulacion.
package agentcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/envelope"
)

// vrhSubProyecto es un repo con el proyecto hoom en sub (hoom.yaml con el
// gate test en true y su .hoom/.gitignore), el .hoom/.gitignore del
// principal que ignora local.yaml, todo commiteado en main, y un
// .hoom/local.yaml valido en el principal.
func vrhSubProyecto(t *testing.T, sub string) (repo, proyecto string) {
	t.Helper()
	cdwHome(t)
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proyecto = filepath.Join(repo, filepath.FromSlash(sub))
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "config", "user.email", "test@hoom.dev")
	git(t, repo, "config", "user.name", "hoom test")
	write(t, proyecto, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, proyecto, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\nlocal.yaml\n")
	write(t, repo, ".hoom/.gitignore", "local.yaml\n")
	write(t, proyecto, "app.go", "package app\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "inicial")
	write(t, repo, entorno.Ruta, "en_el_host: [test]\n")
	if got := entorno.Principal(proyecto); got != repo {
		t.Fatalf("fixture: el principal de %s es el repo %s: %s", proyecto, repo, got)
	}
	return repo, proyecto
}

// bd3025: con el proyecto en sub/app, el writer que cambia o borra el
// .hoom/local.yaml del principal del repo corta en scope, no-entregable,
// exit 1, sin verify, con la nota de manipulacion y la violacion
// RuleTampering en .hoom/local.yaml.
func TestHallazgoBd3025_ProyectoEnUnSubdirectorioCortaEnScope(t *testing.T) {
	veeSaltarSinSh(t)
	notaManip := veeNotaDeManipulacion(t)
	for _, c := range []struct {
		nombre string
		script func(local string) string
	}{
		{"lo cambia", func(l string) string { return "printf 'ejecutar: \"true {cmd}\"\\n' > " + veeQ(l) + "\n" }},
		{"lo borra", func(l string) string { return "rm " + veeQ(l) + "\n" }},
	} {
		repo, proyecto := vrhSubProyecto(t, "sub/app")
		local := filepath.Join(repo, filepath.FromSlash(entorno.Ruta))
		fakeProvider(t, "claude", "printf 'package app // implementado\\n' > app.go\n"+c.script(local)+"exit 0\n")
		var buf bytes.Buffer
		res, err := Run(proyecto, "main", Options{Role: "writer", Prompt: "trabaja en la app"}, &buf)
		if err != nil {
			t.Fatalf("bd3025: %s: el sobre corre: %v\n%s", c.nombre, err, buf.String())
		}
		if res.Stage != "scope" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || !res.Scope.Cuts() || !res.Scope.Tampering {
			t.Fatalf("bd3025: %s: corta en scope, no-entregable, exit 1: %+v\n%s", c.nombre, res, buf.String())
		}
		if res.VerdictID != "" || res.Check != nil {
			t.Fatalf("bd3025: %s: sin verify ni veredicto: %+v", c.nombre, res)
		}
		if res.Note != notaManip {
			t.Fatalf("bd3025: %s: la nota es la de manipulacion %q: %q", c.nombre, notaManip, res.Note)
		}
		if v, ok := aeaViolacion(res.Scope, RuleTampering, entorno.Ruta); !ok || v.Detail != DetalleLocal {
			t.Fatalf("bd3025: %s: violacion %s en %s con %q: %+v", c.nombre, RuleTampering, entorno.Ruta, DetalleLocal, res.Scope.Violations)
		}
	}
}

// vrhExabyte es 1 EiB, el tamano que deja `truncate -s 1E`.
const vrhExabyte = int64(1) << 60

// vrhSaltarSinDisperso salta el test cuando el sistema de archivos de dir no
// deja crear un archivo disperso de 1 EiB.
func vrhSaltarSinDisperso(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, "vrh-disperso")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	if err := os.Truncate(p, vrhExabyte); err != nil {
		t.Skipf("este sistema de archivos no deja crear un archivo disperso de 1 EiB: %v", err)
	}
}

// acc76d: FotoLocal de un .hoom/local.yaml disperso de 1 EiB vuelve en menos
// de un segundo, con una huella distinta de la del archivo chico que era y
// de la de otro tamano enorme.
func TestHallazgoAcc76d_FotoLocalDeUnArchivoEnormeVuelveEnseguida(t *testing.T) {
	principal := t.TempDir()
	vrhSaltarSinDisperso(t, principal)
	write(t, principal, entorno.Ruta, "en_el_host: [test]\n")
	chico := FotoLocal(principal)
	if chico != veeSha("en_el_host: [test]\n") {
		t.Fatalf("acc76d: fixture: la huella del archivo chico es su sha256: %q", chico)
	}
	p := filepath.Join(principal, filepath.FromSlash(entorno.Ruta))
	vistas := map[string]bool{chico: true}
	for _, tam := range []int64{vrhExabyte, vrhExabyte / 2} {
		if err := os.Truncate(p, tam); err != nil {
			t.Fatal(err)
		}
		hecho := make(chan string, 1)
		t0 := time.Now()
		go func() { hecho <- FotoLocal(principal) }()
		select {
		case h := <-hecho:
			if d := time.Since(t0); d >= time.Second {
				t.Fatalf("acc76d: FotoLocal de un archivo de %d bytes vuelve en menos de 1 s: %v", tam, d)
			}
			if h == "" || vistas[h] {
				t.Fatalf("acc76d: la huella del archivo de %d bytes es otra (%q); ya vistas: %v", tam, h, vistas)
			}
			vistas[h] = true
		case <-time.After(10 * time.Second):
			t.Fatalf("acc76d: FotoLocal de un archivo de %d bytes no volvio en 10 s", tam)
		}
	}
}

// acc76d: el sobre de un writer que hace truncate -s 1E sobre el
// .hoom/local.yaml del principal vuelve y corta en scope, no-entregable,
// exit 1, sin verify, con la nota de manipulacion y la violacion
// RuleTampering en .hoom/local.yaml.
func TestHallazgoAcc76d_ElSobreDeUnRolQueLoAgrandaCortaEnScope(t *testing.T) {
	veeSaltarSinSh(t)
	notaManip := veeNotaDeManipulacion(t)
	root, _ := veeProyecto(t, "true")
	vrhSaltarSinDisperso(t, root)
	write(t, root, entorno.Ruta, "en_el_host: [test]\n")
	local := filepath.Join(root, filepath.FromSlash(entorno.Ruta))
	script := "printf 'package app // implementado\\n' > app.go\ntruncate -s 1E " + veeQ(local) + " || exit 9\nexit 0\n"
	fakeProvider(t, "claude", script)
	type fin struct {
		res Result
		err error
		out string
	}
	hecho := make(chan fin, 1)
	go func() { // sin t: un Fatal solo vale desde la goroutine del test
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: "writer", Task: aeaTarea, Prompt: "trabaja en la tarea demo"}, &buf)
		hecho <- fin{res, err, buf.String()}
	}()
	var f fin
	select {
	case f = <-hecho:
	case <-time.After(90 * time.Second):
		t.Fatalf("acc76d: el sobre del writer que hace truncate -s 1E no volvio en 90 s")
	}
	if f.err != nil {
		t.Fatalf("acc76d: el sobre corre: %v\n%s", f.err, f.out)
	}
	if fi, err := os.Stat(local); err != nil || fi.Size() != vrhExabyte {
		t.Fatalf("acc76d: fixture: el writer agrando el archivo a 1 EiB: %v %v", fi, err)
	}
	res := f.res
	if res.Stage != "scope" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || !res.Scope.Cuts() || !res.Scope.Tampering {
		t.Fatalf("acc76d: corta en scope, no-entregable, exit 1: %+v\n%s", res, f.out)
	}
	if res.VerdictID != "" || res.Check != nil {
		t.Fatalf("acc76d: sin verify ni veredicto: %+v", res)
	}
	if res.Note != notaManip {
		t.Fatalf("acc76d: la nota es la de manipulacion %q: %q", notaManip, res.Note)
	}
	if v, ok := aeaViolacion(res.Scope, RuleTampering, entorno.Ruta); !ok || v.Detail != DetalleLocal {
		t.Fatalf("acc76d: violacion %s en %s con %q: %+v", RuleTampering, entorno.Ruta, DetalleLocal, res.Scope.Violations)
	}
}
