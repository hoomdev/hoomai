//go:build !windows

// Regresion del hallazgo 20261125T172319_22b671 (medium): manifest.Find
// convierte cualquier error de os.Stat en «aqui no hay hoom.yaml» y sigue
// subiendo. Con un hoom.yaml que no se puede leer (un enlace en ELOOP, o
// cuyo destino quedo sin permisos), CambioDe tomaba el hoom.yaml de un padre,
// o ninguno, como si fuera el del proyecto, y dejaba Todos en false. Rige
// «ante la duda, el gate corre»: CambioDe busca el hoom.yaml con su propio
// recorrido, que no sigue el enlace, y da Todos ante un error que no sea
// fs.ErrNotExist.
package gates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// Un hoom.yaml que es un enlace a si mismo: os.Stat da ELOOP y Find dice que
// no hay hoom.yaml en ningun directorio.
func TestHallazgo22b671_CA812_UnHoomYamlEnELOOP(t *testing.T) {
	const ca = "CA-812"
	root := he25Repo(t, func(root string) {
		he25Enlace(t, manifest.FileName, filepath.Join(root, manifest.FileName))
	})
	if _, err := os.Stat(filepath.Join(root, manifest.FileName)); err == nil {
		t.Fatalf("%s: el enlace a si mismo no se puede seguir", ca)
	}
	gprEscribir(t, root, "app/a.go", "package app\n// cambio\n")
	gprCommit(t, root, "cambio")
	if got := gates.CambioDe(root, "main"); !got.Todos || !gprContiene(got.Archivos, "app/a.go") {
		t.Fatalf("%s: con el hoom.yaml en ELOOP no se sabe cual es: Todos: %+v", ca, got)
	}
}

// Un monorepo con hoom.yaml en la raiz y el proyecto en sub/, cuyo hoom.yaml
// es un enlace a un destino que queda sin permisos: os.Stat da EACCES, y
// Find sube y da el hoom.yaml de la raiz como si fuera el del proyecto.
func TestHallazgo22b671_CA812_UnHoomYamlSinPermisos(t *testing.T) {
	const ca = "CA-812"
	if os.Geteuid() == 0 {
		t.Skip("como root, los permisos no niegan el stat")
	}
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cerrado := filepath.Join(fuera, "cerrado")
	if err := os.MkdirAll(cerrado, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cerrado, "p.yaml"), []byte(he25Manifiesto), 0o644); err != nil {
		t.Fatal(err)
	}
	root := he25Repo(t, func(root string) {
		gprEscribir(t, root, manifest.FileName, he25Manifiesto)
		gprEscribir(t, root, "sub/app/a.go", "package app\n")
		he25Enlace(t, filepath.Join(cerrado, "p.yaml"), filepath.Join(root, "sub", manifest.FileName))
	})
	gprEscribir(t, root, "sub/app/a.go", "package app\n// cambio\n")
	gprCommit(t, root, "cambio")
	sub := filepath.Join(root, "sub")
	if p, err := manifest.Find(sub); err != nil || p != sub {
		t.Fatalf("%s: antes de quitar los permisos, Find(sub) da sub: %q %v", ca, p, err)
	}

	if err := os.Chmod(cerrado, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cerrado, 0o755) })
	if _, err := os.Stat(filepath.Join(sub, manifest.FileName)); !os.IsPermission(err) {
		t.Fatalf("%s: el stat del hoom.yaml de sub da EACCES: %v", ca, err)
	}
	if got := gates.CambioDe(sub, "main"); !got.Todos || !gprContiene(got.Archivos, "sub/app/a.go") {
		t.Fatalf("%s: con el hoom.yaml de sub sin permisos no se sabe cual es: Todos: %+v", ca, got)
	}
}
