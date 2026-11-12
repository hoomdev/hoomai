// Tests de regresion de la ronda 1 de autonomia-entre-firmas sobre la regla
// tests-del-test-writer (CA-638, aclaracion 14): "compara el contenido"
// cubre el disco, el indice y HEAD del worktree de la tarea, cada uno contra
// su estado antes del run; marcar un test protegido skip-worktree o
// assume-unchanged cuenta como cambiarlo; y un renombre incluye el que solo
// cambia mayusculas. Reusa los helpers de autonomia_entre_firmas_test.go
// (aeaProtegida, aeaCorrer, aeaCortado). CLIs falsos (fakeProvider).
package agentcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ar1Codigo es el cambio de codigo con el que el writer entrega algo
// ademas de lo que hace con el test protegido.
const ar1Codigo = "printf 'package app // implementado\\n' > app.go\n"

// ar1Esconde commitea otra version del protegido y deja en disco sus bytes
// originales (los saca del blob de HEAD de antes del run, sin archivos
// temporales); despues, si marca no es "", marca la ruta.
func ar1Esconde(marca string) string {
	s := "orig=$(git rev-parse HEAD:" + aeaProtegido + ")\n" +
		"printf 'package app\\n\\n// CA-1 precios, la version del writer\\n' > " + aeaProtegido + "\n" +
		"git add -A && git commit -qm 'el writer arregla el test'\n" +
		"git cat-file blob \"$orig\" > " + aeaProtegido + "\n"
	if marca != "" {
		s += "git update-index " + marca + " " + aeaProtegido + "\n"
	}
	return s + "exit 0\n"
}

func ar1GitOut(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// Hallazgo 20261112T172318_123262 (CA-638, aclaracion 14): un writer que
// commitea otra version de un test protegido y deja en disco los bytes
// originales (marcando o no la ruta skip-worktree/assume-unchanged), o que
// solo marca la ruta, corta como cualquier cambio: scope, no-entregable, exit
// 1, sin veredicto nuevo, la nota del contrato y el hallazgo high
// `tests-del-test-writer: el rol writer escribio <ruta>`.
func TestHallazgo123262_CA638_ElWriterQueEscondeElCambioDeUnProtegidoCorta(t *testing.T) {
	for _, c := range []struct {
		nombre   string
		script   string
		commitea bool
	}{
		{"commitea otra version, deja el original en disco y marca skip-worktree", ar1Esconde("--skip-worktree"), true},
		{"commitea otra version, deja el original en disco y marca assume-unchanged", ar1Esconde("--assume-unchanged"), true},
		{"commitea otra version y deja el original en disco sin marcar", ar1Esconde(""), true},
		{"solo marca skip-worktree", "git update-index --skip-worktree " + aeaProtegido + "\nexit 0\n", false},
		{"solo marca assume-unchanged", "git update-index --assume-unchanged " + aeaProtegido + "\nexit 0\n", false},
	} {
		root, wt := aeaProtegida(t, true, "")
		antes := aeaVeredictos(root, wt)
		res, out := aeaCorrer(t, root, "writer", ar1Codigo+c.script, Options{})
		if raw, err := os.ReadFile(filepath.Join(wt, aeaProtegido)); err != nil || string(raw) != aeaTestTW {
			t.Fatalf("123262/CA-638: %s: fixture: el disco conserva los bytes originales del protegido: %q %v", c.nombre, raw, err)
		}
		if c.commitea {
			if enHead, err := ar1GitOut(t, wt, "show", "HEAD:"+aeaProtegido); err != nil || enHead == aeaTestTW {
				t.Fatalf("123262/CA-638: %s: fixture: HEAD tiene otra version del protegido: %q %v", c.nombre, enHead, err)
			}
		} else if marcas, err := ar1GitOut(t, wt, "ls-files", "-v", "--", aeaProtegido); err != nil ||
			strings.HasPrefix(marcas, "H ") || strings.TrimSpace(marcas) == "" {
			t.Fatalf("123262/CA-638: %s: fixture: la ruta quedo marcada en el indice: %q %v", c.nombre, marcas, err)
		}
		aeaCortado(t, "123262/CA-638", c.nombre, root, wt, res, out, antes, aeaProtegido)
	}
}

// ar1SinMayusculas dice si el sistema de archivos de dir no distingue
// mayusculas.
func ar1SinMayusculas(t *testing.T, dir string) bool {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "Sonda-Mayusculas.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := os.Stat(filepath.Join(dir, "sONDA-mAYUSCULAS.TXT"))
	return err == nil
}

// Hallazgo 20261112T172503_11bfc2 (CA-638, aclaracion 14): en un sistema de
// archivos que no distingue mayusculas, renombrar un test protegido
// cambiando solo mayusculas (precios_test.GO) corta como cualquier renombre:
// el archivo tiene que conservar su nombre exacto.
func TestHallazgo11bfc2_CA638_RenombrarSoloConMayusculasCorta(t *testing.T) {
	if !ar1SinMayusculas(t, t.TempDir()) {
		t.Skip("11bfc2/CA-638: el sistema de archivos de los temporales distingue mayusculas: un renombre que solo cambia mayusculas es aca un renombre comun (cubierto por TestCA638_ElWriterQueTocaUnTestProtegidoCorta); este caso necesita uno que no las distinga")
	}
	renombrado := strings.TrimSuffix(aeaProtegido, ".go") + ".GO"
	for _, c := range []struct {
		nombre, script string
	}{
		{"sin commitear", "mv " + aeaProtegido + " hoom-renombre-intermedio && mv hoom-renombre-intermedio " + renombrado + "\nexit 0\n"},
		{"y lo commitea", "git mv " + aeaProtegido + " " + renombrado + " && git commit -qm 'renombre'\nexit 0\n"},
	} {
		root, wt := aeaProtegida(t, true, "")
		antes := aeaVeredictos(root, wt)
		res, out := aeaCorrer(t, root, "writer", ar1Codigo+c.script, Options{})
		ents, err := os.ReadDir(wt)
		if err != nil {
			t.Fatal(err)
		}
		nombres := []string{}
		for _, e := range ents {
			nombres = append(nombres, e.Name())
		}
		if !strings.Contains(" "+strings.Join(nombres, " ")+" ", " "+renombrado+" ") {
			t.Fatalf("11bfc2/CA-638: %s: fixture: el archivo quedo como %s: %q", c.nombre, renombrado, nombres)
		}
		aeaCortado(t, "11bfc2/CA-638", c.nombre, root, wt, res, out, antes, aeaProtegido)
	}
}
