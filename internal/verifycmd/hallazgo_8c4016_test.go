//go:build !windows

// Regresion del hallazgo 20261125T175030_8c4016 (high), en Run: un monorepo
// con hoom.yaml en la raiz y en sub/. Se carga el manifiesto de sub
// (manifest.Load, como hace el Studio al arrancar) y despues sub/hoom.yaml
// deja de estar: borrado, borrado en un commit, movido con git mv, o (caso
// 4b) sin seguimiento y borrado con otro archivo cambiado. CambioDe subia
// hasta el hoom.yaml de la raiz y static, con aplica_en: [sub/app/**],
// quedaba no_aplica. Si el hoom.yaml con el que se juzga ya no esta donde se
// cargo, ante la duda el gate corre (CA-812).
package verifycmd_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

// h8c4Sub es el hoom.yaml de sub: test sin aplica_en, y static con
// aplica_en: [sub/app/**] que deja marca/static-cmd.
func h8c4Sub(marca string) string {
	return "schema: hoom/v1\nproject: sub\nbase_branch: main\ngates:\n" +
		"  test:\n    required: true\n    cmd: \"true\"\n" +
		"  static:\n    required: true\n    cmd: " + strconv.Quote(gprToque(marca, "static-cmd", false)) + "\n" +
		"    aplica_en: [\"sub/app/**\"]\n"
}

// h8c4Repo es el monorepo: hoom.yaml en la raiz, sub/app/a.go, README.md y
// un .gitignore de .hoom/, en main, parado en la rama feature. Con
// subEnLaBase, sub/hoom.yaml va en la base; sin, queda sin seguimiento en
// feature.
func h8c4Repo(t *testing.T, marca string, subEnLaBase bool) string {
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
	gprEscribir(t, root, manifest.FileName, "schema: hoom/v1\nproject: raiz\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	gprEscribir(t, root, ".gitignore", ".hoom/\nsub/.hoom/\n")
	gprEscribir(t, root, "README.md", "# demo\n")
	gprEscribir(t, root, "sub/app/a.go", "package app\n")
	if subEnLaBase {
		gprEscribir(t, root, "sub/"+manifest.FileName, h8c4Sub(marca))
	}
	gprCommit(t, root, "inicial")
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	if !subEnLaBase {
		gprEscribir(t, root, "sub/"+manifest.FileName, h8c4Sub(marca))
	}
	return root
}

func TestHallazgo8c4016_CA812_SinElHoomYamlQueSeCargoElGateCorre(t *testing.T) {
	const ca = "CA-812"
	casos := []struct {
		nombre      string
		subEnLaBase bool
		despues     func(t *testing.T, root string)
	}{
		{"borrado sin commitear", true, func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "sub", manifest.FileName)); err != nil {
				t.Fatal(err)
			}
		}},
		{"borrado en un commit", true, func(t *testing.T, root string) {
			gprGit(t, root, "rm", "-q", "sub/"+manifest.FileName)
			gprGit(t, root, "commit", "-q", "-m", "borra sub/hoom.yaml")
		}},
		{"movido con git mv a sub/hoom.yml", true, func(t *testing.T, root string) {
			gprGit(t, root, "mv", "sub/"+manifest.FileName, "sub/hoom.yml")
		}},
		{"4b: sin seguimiento, borrado, y README.md cambiado", false, func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "sub", manifest.FileName)); err != nil {
				t.Fatal(err)
			}
			gprEscribir(t, root, "README.md", "# demo 2\n")
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			marca := t.TempDir()
			root := h8c4Repo(t, marca, c.subEnLaBase)
			m, err := manifest.Load(filepath.Join(root, "sub"), nil)
			if err != nil {
				t.Fatalf("%s: fixture: Load(sub): %v", ca, err)
			}
			c.despues(t, root)
			v, _, err := verifycmd.Run(m, verifycmd.Options{})
			if err != nil {
				t.Fatalf("%s (%s): verify corre: %v", ca, c.nombre, err)
			}
			g := gprGate(t, ca, v, "static")
			if g.Status != verdict.StatusPass || !gprHay(marca, "static-cmd") || v.Summary.NoAplica != 0 {
				t.Fatalf("%s (%s): sin el hoom.yaml que se cargo, static corre, no queda no_aplica: %+v %+v", ca, c.nombre, g, v.Summary)
			}
		})
	}
}

// El control: con sub/hoom.yaml en su lugar y un cambio fuera de
// sub/app/**, static sigue en no_aplica.
func TestHallazgo8c4016_CA810_ConElHoomYamlEnSuLugarStaticNoAplica(t *testing.T) {
	const ca = "CA-810"
	marca := t.TempDir()
	root := h8c4Repo(t, marca, true)
	m, err := manifest.Load(filepath.Join(root, "sub"), nil)
	if err != nil {
		t.Fatalf("%s: fixture: Load(sub): %v", ca, err)
	}
	gprEscribir(t, root, "README.md", "# demo 2\n")
	gprCommit(t, root, "solo el README")
	v, _, err := verifycmd.Run(m, verifycmd.Options{})
	if err != nil {
		t.Fatalf("%s: verify corre: %v", ca, err)
	}
	if g := gprGate(t, ca, v, "static"); g.Status != verdict.StatusNoAplica || gprHay(marca, "static-cmd") {
		t.Fatalf("%s: con el hoom.yaml en su lugar y solo README.md cambiado, static no aplica: %+v", ca, g)
	}
}
