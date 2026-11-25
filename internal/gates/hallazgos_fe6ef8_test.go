//go:build !windows

// Regresion del hallazgo 20261125T162645_fe6ef8 (medium): cuando CambioDe no
// puede saber si el cambio toca el hoom.yaml del proyecto, rige «ante la
// duda, el gate corre» y Todos queda en true.
//
//   - Si git falla en 'rev-parse --show-prefix', no se sabe donde esta el
//     hoom.yaml en el repositorio: Todos, toque lo que toque el cambio.
//   - Si manifest.Find no encuentra el hoom.yaml (por ejemplo, porque se
//     borro), cualquier archivo del cambio llamado hoom.yaml puede ser el
//     del proyecto: Todos. Sin ninguno, el hoom.yaml del proyecto (este
//     donde este) no esta en el cambio, y Todos sigue en false con los
//     archivos, como hasta ahora.
package gates_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// hfe6GitQueFallaEnElPrefijo deja primero en el PATH un git que falla solo
// en --show-prefix; para todo lo demas es el git de verdad.
func hfe6GitQueFallaEnElPrefijo(t *testing.T) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do\n  if [ \"$a\" = \"--show-prefix\" ]; then echo 'fatal: simulado' >&2; exit 128; fi\ndone\nexec " + gprQ(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestHallazgofe6ef8_CA812_SiGitFallaEnElPrefijoTodos(t *testing.T) {
	const ca = "CA-812"
	casos := []struct {
		nombre string
		cambio func(t *testing.T, root string)
	}{
		{"con el hoom.yaml en el cambio", func(t *testing.T, root string) {
			gprEscribir(t, root, manifest.FileName, "schema: hoom/v1\nproject: demo\n# tocado\n")
		}},
		{"sin el hoom.yaml en el cambio", func(t *testing.T, root string) {
			gprEscribir(t, root, "app/a.go", "package app\n")
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			root := gprRepo(t, "", nil)
			c.cambio(t, root)
			hfe6GitQueFallaEnElPrefijo(t)
			got := gates.CambioDe(root, "main")
			if !got.Todos {
				t.Fatalf("%s (%s): si git no dice donde esta el hoom.yaml, ante la duda Todos: %+v", ca, c.nombre, got)
			}
			if len(got.Archivos) == 0 {
				t.Fatalf("%s (%s): git si dijo el cambio, que viene en Archivos: %+v", ca, c.nombre, got)
			}
		})
	}
}

func TestHallazgofe6ef8_CA812_SinHoomYamlQueEncontrarYUnoEnElCambioTodos(t *testing.T) {
	const ca = "CA-812"
	// el hoom.yaml del proyecto, borrado sin commitear: Find no lo encuentra
	root := gprRepo(t, "", nil)
	if err := os.Remove(filepath.Join(root, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
	if got := gates.CambioDe(root, "main"); !got.Todos || !gprContiene(got.Archivos, manifest.FileName) {
		t.Fatalf("%s: con el hoom.yaml borrado, Todos y hoom.yaml en Archivos: %+v", ca, got)
	}

	// un repo sin hoom.yaml en la raiz y uno en un subdirectorio del cambio
	root = hfe6RepoSinHoomYaml(t)
	gprEscribir(t, root, "otro/"+manifest.FileName, "schema: hoom/v1\n")
	if got := gates.CambioDe(root, "main"); !got.Todos {
		t.Fatalf("%s: sin hoom.yaml que encontrar, un hoom.yaml en el cambio puede ser el del proyecto: Todos: %+v", ca, got)
	}
}

// El control: sin hoom.yaml que encontrar y sin ninguno en el cambio, Todos
// sigue en false con los archivos (lo que ya prometia el go doc).
func TestHallazgofe6ef8_CA811_SinHoomYamlNiEnElCambioTodosSigueEnFalse(t *testing.T) {
	const ca = "CA-811"
	root := hfe6RepoSinHoomYaml(t)
	gprEscribir(t, root, "app/a.go", "package app\n")
	if got := gates.CambioDe(root, "main"); got.Todos || !gprContiene(got.Archivos, "app/a.go") {
		t.Fatalf("%s: sin ningun hoom.yaml, Todos false y app/a.go en Archivos: %+v", ca, got)
	}
}

// hfe6RepoSinHoomYaml es un repo git sin hoom.yaml, con base main y parado en
// la rama feature.
func hfe6RepoSinHoomYaml(t *testing.T) string {
	t.Helper()
	gprAislar(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gprGit(t, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	gprGit(t, root, "config", "user.email", "test@hoom.dev")
	gprGit(t, root, "config", "user.name", "hoom test")
	gprEscribir(t, root, "README.md", "# demo\n")
	gprCommit(t, root, "inicial")
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	return root
}

// Sin distinguir mayusculas, como los patrones de aplica_en: en un sistema de
// archivos que no las distingue, HOOM.YAML es el hoom.yaml del proyecto.
func TestHallazgofe6ef8_CA812_ElHoomYamlSinDistinguirMayusculas(t *testing.T) {
	const ca = "CA-812"
	root := gprRepo(t, "", nil)
	gprEscribir(t, root, "HOOM.YAML", "schema: hoom/v1\n")
	if got := gates.CambioDe(root, "main"); !got.Todos {
		t.Fatalf("%s: HOOM.YAML en el cambio puede ser el hoom.yaml del proyecto: Todos: %+v", ca, got)
	}
	root = hfe6RepoSinHoomYaml(t)
	gprEscribir(t, root, "otro/Hoom.Yaml", "schema: hoom/v1\n")
	if got := gates.CambioDe(root, "main"); !got.Todos {
		t.Fatalf("%s: sin hoom.yaml que encontrar, otro/Hoom.Yaml en el cambio: Todos: %+v", ca, got)
	}
}
