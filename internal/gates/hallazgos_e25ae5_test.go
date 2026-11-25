//go:build !windows

// Regresion del hallazgo 20261125T171417_e25ae5 (high): con un hoom.yaml que
// es un enlace simbolico (hoom.yaml -> config/proyecto.yaml), editar el
// destino cambia el manifiesto que carga verify sin que el cambio traiga
// hoom.yaml, y CambioDe dejaba Todos en false. La persona eligio: si el
// hoom.yaml del proyecto es un enlace simbolico, a cualquier destino, dentro
// o fuera del repo, todos los gates aplican siempre (CA-812, «ante la duda,
// el gate corre»).
package gates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/manifest"
)

const he25Manifiesto = "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

// he25Repo es un repo git con base main, parado en la rama feature recien
// creada. prepara arma, antes del commit de la base, lo que va en ella (el
// hoom.yaml incluido); la base lleva ademas app/a.go y README.md.
func he25Repo(t *testing.T, prepara func(root string)) string {
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
	gprEscribir(t, root, "app/a.go", "package app\n")
	prepara(root)
	gprCommit(t, root, "inicial")
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	return root
}

func he25Enlace(t *testing.T, destino, enlace string) {
	t.Helper()
	if err := os.Symlink(destino, enlace); err != nil {
		t.Fatal(err)
	}
}

func TestHallazgoe25ae5_CA812_UnHoomYamlQueEsUnEnlaceDentroDelRepo(t *testing.T) {
	const ca = "CA-812"
	for _, edita := range []bool{true, false} {
		root := he25Repo(t, func(root string) {
			gprEscribir(t, root, "config/proyecto.yaml", he25Manifiesto)
			he25Enlace(t, "config/proyecto.yaml", filepath.Join(root, manifest.FileName))
		})
		if _, err := manifest.Load(root, nil); err != nil {
			t.Fatalf("%s: el manifiesto se carga a traves del enlace: %v", ca, err)
		}
		gprEscribir(t, root, "app/a.go", "package app\n// cambio\n")
		if edita {
			// el destino del enlace: el manifiesto que carga verify
			gprEscribir(t, root, "config/proyecto.yaml", he25Manifiesto+"# tocado\n")
		}
		gprCommit(t, root, "cambio")
		for _, desde := range []string{"", "app"} {
			got := gates.CambioDe(filepath.Join(root, desde), "main")
			if !got.Todos || !gprContiene(got.Archivos, "app/a.go") {
				t.Fatalf("%s (edita el destino %v, desde %q): con hoom.yaml enlace simbolico, Todos siempre, y el cambio en Archivos: %+v", ca, edita, desde, got)
			}
		}
	}
}

func TestHallazgoe25ae5_CA812_UnHoomYamlQueEsUnEnlaceFueraDelRepo(t *testing.T) {
	const ca = "CA-812"
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	compartido := filepath.Join(fuera, "compartido.yaml")
	if err := os.WriteFile(compartido, []byte(he25Manifiesto), 0o644); err != nil {
		t.Fatal(err)
	}
	root := he25Repo(t, func(root string) {
		he25Enlace(t, compartido, filepath.Join(root, manifest.FileName))
	})
	gprEscribir(t, root, "app/a.go", "package app\n// cambio\n")
	gprCommit(t, root, "cambio")
	// el destino cambia fuera del repo: git no lo ve
	if err := os.WriteFile(compartido, []byte(he25Manifiesto+"# tocado\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := gates.CambioDe(root, "main"); !got.Todos || !gprContiene(got.Archivos, "app/a.go") {
		t.Fatalf("%s: con hoom.yaml enlace a un archivo fuera del repo, Todos siempre: %+v", ca, got)
	}
}

// El control: con un hoom.yaml de verdad que el cambio no toca, Todos sigue
// en false.
func TestHallazgoe25ae5_CA811_UnHoomYamlRealQueNoSeTocaDejaTodosEnFalse(t *testing.T) {
	const ca = "CA-811"
	root := he25Repo(t, func(root string) {
		gprEscribir(t, root, manifest.FileName, he25Manifiesto)
		gprEscribir(t, root, "config/proyecto.yaml", he25Manifiesto)
	})
	gprEscribir(t, root, "app/a.go", "package app\n// cambio\n")
	gprEscribir(t, root, "config/proyecto.yaml", he25Manifiesto+"# tocado\n")
	gprCommit(t, root, "cambio")
	if got := gates.CambioDe(root, "main"); got.Todos || !gprContiene(got.Archivos, "app/a.go") {
		t.Fatalf("%s: con un hoom.yaml real fuera del cambio, Todos false y app/a.go en Archivos: %+v", ca, got)
	}
}
