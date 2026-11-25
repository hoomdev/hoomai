//go:build !windows

// Regresion del hallazgo 20261125T175030_8c4016 (high), en CambioDe: un
// monorepo con hoom.yaml en la raiz y en sub/. Si sub/hoom.yaml ya no esta
// (borrado sin commitear, borrado en un commit o movido con git mv a
// sub/hoom.yml), CambioDe(sub) subia hasta el hoom.yaml de la raiz, lo
// comparaba solo con "hoom.yaml" y dejaba Todos en false. Cuando el
// hoom.yaml que se encuentra esta mas arriba que dir, cualquier archivo del
// cambio llamado hoom.yaml puede ser el del proyecto: Todos (CA-812,
// aclaracion 6). La busqueda hacia arriba se conserva (aclaraciones 6 y 7).
package gates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// h8c4Monorepo es un repo con hoom.yaml en la raiz y en sub/, y sub/app/a.go,
// en main, parado en feature.
func h8c4Monorepo(t *testing.T) string {
	t.Helper()
	return he25Repo(t, func(root string) {
		gprEscribir(t, root, manifest.FileName, he25Manifiesto)
		gprEscribir(t, root, "sub/"+manifest.FileName, he25Manifiesto)
		gprEscribir(t, root, "sub/app/a.go", "package app\n")
	})
}

func TestHallazgo8c4016_CA812_SinElHoomYamlDeSubNoSeSubeAlDeLaRaiz(t *testing.T) {
	const ca = "CA-812"
	casos := []struct {
		nombre string
		quita  func(t *testing.T, root string)
	}{
		{"borrado sin commitear", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "sub", manifest.FileName)); err != nil {
				t.Fatal(err)
			}
		}},
		{"borrado en un commit", func(t *testing.T, root string) {
			gprGit(t, root, "rm", "-q", "sub/"+manifest.FileName)
			gprGit(t, root, "commit", "-q", "-m", "borra sub/hoom.yaml")
		}},
		{"movido con git mv a sub/hoom.yml", func(t *testing.T, root string) {
			gprGit(t, root, "mv", "sub/"+manifest.FileName, "sub/hoom.yml")
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			root := h8c4Monorepo(t)
			c.quita(t, root)
			got := gates.CambioDe(filepath.Join(root, "sub"), "main")
			if !got.Todos || !gprContiene(got.Archivos, "sub/"+manifest.FileName) {
				t.Fatalf("%s (%s): sin sub/hoom.yaml, el de la raiz no es el del proyecto: Todos, y sub/hoom.yaml en Archivos: %+v", ca, c.nombre, got)
			}
		})
	}
}

// Los controles: en el monorepo, un cambio que no toca ningun hoom.yaml deja
// Todos en false, desde sub, desde un subdirectorio de sub y desde la raiz.
func TestHallazgo8c4016_CA811_UnMonorepoSinCambiosEnNingunHoomYaml(t *testing.T) {
	const ca = "CA-811"
	root := h8c4Monorepo(t)
	gprEscribir(t, root, "sub/app/a.go", "package app\n// cambio\n")
	gprEscribir(t, root, "README.md", "# demo 2\n")
	gprCommit(t, root, "cambio sin hoom.yaml")
	for _, desde := range []string{"sub", "sub/app", ""} {
		got := gates.CambioDe(filepath.Join(root, filepath.FromSlash(desde)), "main")
		if got.Todos || !gprContiene(got.Archivos, "sub/app/a.go") {
			t.Fatalf("%s (desde %q): sin ningun hoom.yaml en el cambio, Todos false con los archivos: %+v", ca, desde, got)
		}
	}
}
