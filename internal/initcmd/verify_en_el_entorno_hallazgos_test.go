// Regresiones de los hallazgos de la delta 1 de la review de
// verify-en-el-entorno (ad7fac), escritas por el writer en la ronda a mano:
//
//   - 3faa18: `hoom init` completa un .hoom/.gitignore que existe sin
//     cambiarle el modo: con el proyecto en sub/app, el del checkout
//     principal en 0600 con cache/ queda en 0600 con cache/ y local.yaml; con
//     el proyecto en la raiz del repo, lo mismo con el del proyecto.
package initcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 3faa18: el .hoom/.gitignore que init completa (el del principal con el
// proyecto en sub/app, el del proyecto en la raiz del repo) conserva su modo
// 0600 y su inodo, y queda con sus lineas y local.yaml.
func TestHallazgo3faa18_InitConservaElModoDelGitignore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los permisos 0600 son de unix")
	}
	for _, sub := range []string{"sub/app", ""} {
		caso := "proyecto en " + sub
		if sub == "" {
			caso = "proyecto en la raiz del repo"
		}
		_, repo, proyecto := veeRepo(t, sub, "cache/\n")
		gi := filepath.Join(repo, ".hoom", ".gitignore")
		if err := os.Chmod(gi, 0o600); err != nil {
			t.Fatal(err)
		}
		antes, err := os.Lstat(gi)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := veeCorrer(t, proyecto, "go"); err != nil {
			t.Fatalf("3faa18: %s: hoom init: %v\n%s", caso, err, out)
		}
		despues, err := os.Lstat(gi)
		if err != nil {
			t.Fatal(err)
		}
		if despues.Mode().Perm() != 0o600 {
			t.Fatalf("3faa18: %s: el .hoom/.gitignore completado sigue en 0600: %v", caso, despues.Mode().Perm())
		}
		if !os.SameFile(antes, despues) {
			t.Fatalf("3faa18: %s: el .hoom/.gitignore completado es el mismo archivo (inodo)", caso)
		}
		if got := veeLeer(t, gi); !veeMismas(veeLineas(got), []string{"cache/", "local.yaml"}) {
			t.Fatalf("3faa18: %s: el .hoom/.gitignore queda con cache/ y local.yaml: %q", caso, got)
		}
	}
}
