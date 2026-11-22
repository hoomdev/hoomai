// Regresiones de los hallazgos de la review completa de verify-en-el-entorno
// (6e455e) sobre el sobre de `hoom agent`, escritas por el writer en la ronda
// a mano:
//
//   - bd3025 (CA-768): en un proyecto que vive en un subdirectorio de su
//     repo (hoom.yaml en sub/app), el sobre fotografia el .hoom/local.yaml
//     del checkout principal, el que lee verify (entorno.Principal), no uno
//     en sub/app: un rol que cambia <repo>/.hoom/local.yaml corta en scope
//     por manipulacion.
package agentcmd

import (
	"bytes"
	"path/filepath"
	"testing"

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
