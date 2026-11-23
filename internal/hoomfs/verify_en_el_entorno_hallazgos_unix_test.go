//go:build !windows

// Regresiones de la delta 2 de la review de verify-en-el-entorno (1617fc),
// escritas por el writer en la ronda 3, sobre completarGitignore, que solo
// valen donde el sistema cuenta los enlaces duros:
//
//   - 31f78c: un .hoom/.gitignore con otro enlace duro (a un archivo fuera
//     del proyecto, o hecho al que hoom acaba de crear) no se escribe: es un
//     *EnlacesDuros, sin agregar nada, y los dos nombres quedan como estaban.
package hoomfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// vrhEnlazar hace en otro un enlace duro a gi, o saltea el test si el
// sistema de archivos no los tiene.
func vrhEnlazar(t *testing.T, gi, otro string) {
	t.Helper()
	if err := os.Link(gi, otro); err != nil {
		t.Skipf("este sistema de archivos no hace enlaces duros: %v", err)
	}
}

// 31f78c: .hoom/.gitignore es un enlace duro a un archivo fuera del
// proyecto. EnsureIgnoredFiles(local.yaml) y EnsureIgnored(worktrees)
// devuelven un *EnlacesDuros, sin agregar nada, y los bytes de los dos
// nombres no cambian. Tambien cuando el enlace se hace al .hoom/.gitignore
// que EnsureIgnoredFiles acaba de crear, antes de escribirle su linea.
func TestHallazgo31f78c_UnGitignoreConOtroEnlaceDuroNoSeEscribe(t *testing.T) {
	for _, c := range []struct {
		nombre string
		llamar func(root string) ([]string, error)
	}{
		{"EnsureIgnoredFiles(local.yaml)", func(root string) ([]string, error) { return EnsureIgnoredFiles(root, "local.yaml") }},
		{"EnsureIgnored(worktrees)", func(root string) ([]string, error) { return EnsureIgnored(root, "worktrees") }},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "proyecto")
			gi := vrhGitignore(t, root, "", 0)
			otro := filepath.Join(base, "fuera-del-proyecto", "notas.txt")
			if err := os.MkdirAll(filepath.Dir(otro), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(otro, []byte("cache/\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			vrhEnlazar(t, otro, gi)
			added, err := c.llamar(root)
			vrhNoSeEscribio(t, c.nombre, added, err, map[string]string{gi: "cache/\n", otro: "cache/\n"})
		})
	}
	t.Run("al que acaba de crear", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "proyecto")
		gi := vrhGitignore(t, root, "", 0)
		otro := filepath.Join(base, "fuera-del-proyecto.txt")
		var corrio bool
		vrhArmar(t, &antesDeEscribirCreado, func(string) {
			corrio = true
			vrhEnlazar(t, gi, otro)
		})
		added, err := EnsureIgnoredFiles(root, "local.yaml")
		if !corrio {
			t.Fatal("fixture: el gancho entre la creacion y la escritura no corrio")
		}
		vrhNoSeEscribio(t, "EnsureIgnoredFiles(local.yaml) al que acaba de crear", added, err, map[string]string{gi: "", otro: ""})
	})
}

// vrhNoSeEscribio comprueba que la llamada devolvio un *EnlacesDuros sin
// agregar nada y que cada archivo de quedan sigue con sus bytes.
func vrhNoSeEscribio(t *testing.T, caso string, added []string, err error, quedan map[string]string) {
	t.Helper()
	var ed *EnlacesDuros
	if !errors.As(err, &ed) || ed.Enlaces != 2 {
		t.Errorf("31f78c: %s: devuelve un *EnlacesDuros con 2 enlaces: %v", caso, err)
	}
	if len(added) != 0 {
		t.Errorf("31f78c: %s: no agrega nada: %q", caso, added)
	}
	for path, want := range quedan {
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if string(raw) != want {
			t.Errorf("31f78c: %s: %s queda %q: %q", caso, path, want, raw)
		}
	}
}
