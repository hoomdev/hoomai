//go:build !windows

// Regresion de los hallazgos 20261125T161655_09a40f (high) y
// 20261125T162230_e8575e (medium): CambioDe tiene que reconocer el hoom.yaml
// del proyecto en el cambio aunque el directorio del proyecto empiece con un
// espacio o un tab. Recortarle espacios al --show-prefix de git daba
// "web/hoom.yaml" en vez de " web/hoom.yaml", y entonces Todos quedaba en
// false (CA-812, aclaracion 6).
package gates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/manifest"
)

func TestHallazgo09a40f_CA812_ElHoomYamlDeUnProyectoQueEmpiezaConEspacio(t *testing.T) {
	const ca = "CA-812"
	for _, proyecto := range []string{" web", "\tweb", "  web/ api"} {
		for _, desde := range []string{proyecto, proyecto + "/resources"} {
			for _, commit := range []bool{true, false} {
				js := filepath.ToSlash(filepath.Join(proyecto, "resources/js/app.js"))
				root := gprRepo(t, proyecto, map[string]string{js: "x\n"})
				yaml := filepath.Join(root, filepath.FromSlash(proyecto), manifest.FileName)
				raw, err := os.ReadFile(yaml)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(yaml, append(raw, []byte("# un cambio del hoom.yaml\n")...), 0o644); err != nil {
					t.Fatal(err)
				}
				gprEscribir(t, root, js, "y\n")
				if commit {
					gprCommit(t, root, "toca el hoom.yaml")
				}
				got := gates.CambioDe(filepath.Join(root, filepath.FromSlash(desde)), "main")
				if !got.Todos {
					t.Fatalf("%s (proyecto %q, desde %q, commit %v): con el hoom.yaml del proyecto en el cambio, Todos: %+v", ca, proyecto, desde, commit, got)
				}
			}
		}
	}
}

// El control: con el mismo proyecto y sin tocar su hoom.yaml, Todos sigue
// en false y el cambio trae la ruta con su espacio.
func TestHallazgo09a40f_CA811_SinTocarElHoomYamlTodosSigueEnFalse(t *testing.T) {
	const ca = "CA-811"
	const proyecto = " web"
	js := proyecto + "/resources/js/app.js"
	root := gprRepo(t, proyecto, map[string]string{js: "x\n"})
	gprEscribir(t, root, js, "y\n")
	gprCommit(t, root, "toca solo el js")
	got := gates.CambioDe(filepath.Join(root, proyecto), "main")
	if got.Todos || !gprContiene(got.Archivos, js) {
		t.Fatalf("%s: sin el hoom.yaml en el cambio, Todos false y %q en Archivos: %+v", ca, js, got)
	}
}
