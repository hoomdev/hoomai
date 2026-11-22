// Regresiones de los hallazgos de la review completa de verify-en-el-entorno
// (6e455e) sobre `hoom task start`, escritas por el writer en la ronda a
// mano:
//
//   - 053a71 (CA-771): sin worktrees_relativos (sin el archivo, sin la clave
//     o con false), un worktree.useRelativePaths=true en la config del repo
//     o en la global no le gana a Start: el worktree guarda un gitdir
//     absoluto y el .git/config no cambia (no gana
//     extensions.relativeWorktrees).
package taskcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	hentorno "github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/gitx"
)

// 053a71: sin la clave, Start agrega el worktree con rutas absolutas aunque
// la config de git pida rutas relativas, en el repo o en la global.
func TestHallazgo053a71_SinLaClaveLaConfigDeGitNoPideRutasRelativas(t *testing.T) {
	for _, c := range []struct {
		nombre, donde, local string // donde: "repo", "global" o "" (sin config)
	}{
		{"repo, sin el archivo", "repo", ""},
		{"global, sin el archivo", "global", ""},
		{"repo, el archivo sin la clave", "repo", "en_el_host: [test]\n"},
		{"repo, worktrees_relativos: false", "repo", "worktrees_relativos: false\n"},
		{"global, worktrees_relativos: false", "global", "worktrees_relativos: false\n"},
		{"control: sin la config de git", "", ""},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := veeBase(t, veeHoomYAML, veeGitignore())
			switch c.donde {
			case "repo":
				rdGit(t, root, "config", "worktree.useRelativePaths", "true")
			case "global":
				// el archivo vacio de veeAislarGit
				out, err := exec.Command("git", "config", "--file", os.Getenv("GIT_CONFIG_GLOBAL"), "worktree.useRelativePaths", "true").CombinedOutput()
				if err != nil {
					t.Fatalf("fixture: la config global: %v %s", err, out)
				}
			}
			if c.local != "" {
				rdEscribir(t, root, hentorno.Ruta, c.local)
			}
			cfg := filepath.Join(root, ".git", "config")
			antes, err := os.ReadFile(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := Start(root, "demo", "main"); err != nil {
				t.Fatalf("053a71: %s: Start agrega el worktree: %v", c.nombre, err)
			}
			wt := filepath.Join(root, ".hoom", "worktrees", "demo")
			veeExigeAbsoluto(t, c.nombre, root, wt)
			despues, err := os.ReadFile(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if string(despues) != string(antes) {
				t.Fatalf("053a71: %s: el .git/config no cambia:\n--- antes ---\n%s\n--- despues ---\n%s", c.nombre, antes, despues)
			}
			if rama, ok, err := gitx.WorktreeDe(root, wt); err != nil || !ok || rama != "refs/heads/hoom/demo" {
				t.Fatalf("053a71: %s: WorktreeDe reconoce el worktree: %q %v %v", c.nombre, rama, ok, err)
			}
		})
	}
}
