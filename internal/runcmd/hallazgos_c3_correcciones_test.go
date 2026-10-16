// Tests de regresion del hallazgo 20261015T163244_7a04de (medium) de la
// review de las correcciones de C3 (.hoom/specs/acciones-desde-la-tarjeta.md),
// escritos desde el contrato (go doc LockArbol) y el hallazgo, sin leer la
// implementacion: LockArbol solo es un candado que no hace nada fuera de un
// repositorio de git. Si git falla por cualquier otra razon (por ejemplo
// "dubious ownership"), devuelve error: dos procesos que no coinciden en
// donde esta el candado no se excluirian.
//
// Reusa initRepo de runcmd_test.go.
package runcmd

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Hallazgo 20261015T163244_7a04de (medium): dentro de un repo donde git
// falla por otra razon que "no es un repositorio" (GIT_TEST_ASSUME_DIFFERENT_OWNER=1
// simula "dubious ownership"), LockArbol devuelve error y no un candado que
// no frena nada.
func TestHallazgo_7a04de_LockArbolConGitFallandoEsError(t *testing.T) {
	root := initRepo(t)
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "dubious ownership") {
		t.Skipf("este git no simula dubious ownership con GIT_TEST_ASSUME_DIFFERENT_OWNER: %v %s", err, out)
	}

	suelta, err := LockArbol(root, time.Second)
	if err == nil {
		if suelta != nil {
			suelta()
		}
		t.Fatal("hallazgo 7a04de: dentro de un repo donde git falla por dubious ownership, LockArbol devuelve error y no un candado que no frena nada")
	}
}

// Hallazgo 20261015T163244_7a04de (guarda): fuera de un repositorio,
// LockArbol sigue sin fallar, con o sin GIT_TEST_ASSUME_DIFFERENT_OWNER.
func TestHallazgo_7a04de_GuardaFueraDeUnRepoNoFalla(t *testing.T) {
	for _, conVariable := range []bool{false, true} {
		nombre := "sin-variable"
		if conVariable {
			nombre = "con-dubious-ownership"
		}
		t.Run(nombre, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command("git", "rev-parse", "--git-common-dir")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "not a git repository") {
				t.Skipf("el directorio temporal esta dentro de un repo: %v %s", err, out)
			}
			if conVariable {
				t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
			}
			suelta, err := LockArbol(dir, time.Second)
			if err != nil {
				t.Fatalf("hallazgo 7a04de: guarda: fuera de un repo LockArbol no falla: %v", err)
			}
			if suelta == nil {
				t.Fatal("hallazgo 7a04de: guarda: LockArbol devuelve como soltarlo")
			}
			suelta()
		})
	}
}
