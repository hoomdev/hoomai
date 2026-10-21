// Tests del hallazgo 20261021T163139_1e4b3e (high, risk) de la delta 1 de
// fixes-cabina-c1b, sobre gitx.LimpiarEntorno
// (.hoom/specs/acciones-desde-la-tarjeta.md, CA-346: lo que hoom hace sobre
// una tarea lo hace sobre la tarea). Escritos desde el contrato, nunca desde
// la implementacion:
//
// LimpiarEntorno quita del proceso GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR,
// GIT_INDEX_FILE, GIT_OBJECT_DIRECTORY, GIT_ALTERNATE_OBJECT_DIRECTORIES y
// GIT_NAMESPACE (definidas con valor o vacias): despues ninguna esta en
// os.Environ ni la ve un proceso hijo lanzado sin Env propio. Las demas
// variables quedan como estaban.
//
// En af1c3ed gitx.LimpiarEntorno no existe y este archivo no compila: es lo
// esperado hasta el arreglo.
package gitx

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

var lmVarsDeOtroRepo = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
}

// lmEnEnviron dice si nombre esta definida en entorno (lista de k=v).
func lmEnEnviron(entorno []string, nombre string) (string, bool) {
	for _, kv := range entorno {
		if strings.HasPrefix(kv, nombre+"=") {
			return kv, true
		}
	}
	return "", false
}

// Hallazgo 20261021T163139_1e4b3e. CA-346: tras LimpiarEntorno ninguna de
// las siete variables queda en el proceso (ni en os.Environ, ni para
// os.LookupEnv, ni para un hijo que hereda el entorno), con valores reales,
// raros o vacios; llamarla dos veces da lo mismo. Guarda: PATH, HOME y una
// variable cualquiera siguen con su valor.
func TestCA346_Hallazgo1e4b3e_LimpiarEntornoQuitaLasVariablesDeOtroRepo(t *testing.T) {
	casos := []struct {
		nombre string
		valor  func(v string) string
	}{
		{"con valor", func(v string) string { return "/otro/repo/" + strings.ToLower(v) }},
		{"vacias", func(string) string { return "" }},
		{"con espacios, = y unicode", func(v string) string { return " /rarö dir/" + v + "=x " }},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nombre, func(t *testing.T) {
			for _, v := range lmVarsDeOtroRepo {
				t.Setenv(v, c.valor(v))
			}
			t.Setenv("HOOM_TEST_1E4B3E_QUEDA", "sigue aca")
			path, home := os.Getenv("PATH"), os.Getenv("HOME")

			for vuelta := 1; vuelta <= 2; vuelta++ {
				LimpiarEntorno()
				for _, v := range lmVarsDeOtroRepo {
					if val, ok := os.LookupEnv(v); ok {
						t.Errorf("CA-346: hallazgo 1e4b3e (%s, vuelta %d): tras LimpiarEntorno %s no esta definida, vale %q", c.nombre, vuelta, v, val)
					}
					if kv, ok := lmEnEnviron(os.Environ(), v); ok {
						t.Errorf("CA-346: hallazgo 1e4b3e (%s, vuelta %d): tras LimpiarEntorno os.Environ no trae %s: trae %q", c.nombre, vuelta, v, kv)
					}
				}
				if got := os.Getenv("HOOM_TEST_1E4B3E_QUEDA"); got != "sigue aca" {
					t.Errorf("CA-346: guarda 1e4b3e (%s): LimpiarEntorno no toca otras variables: HOOM_TEST_1E4B3E_QUEDA = %q", c.nombre, got)
				}
				if os.Getenv("PATH") != path || os.Getenv("HOME") != home {
					t.Errorf("CA-346: guarda 1e4b3e (%s): LimpiarEntorno no toca PATH ni HOME", c.nombre)
				}
			}

			envBin, err := exec.LookPath("env")
			if err != nil {
				t.Skip("sin env en el PATH: no se mira el entorno de un hijo")
			}
			out, err := exec.Command(envBin).Output()
			if err != nil {
				t.Fatalf("env: %v", err)
			}
			hijo := strings.Split(string(out), "\n")
			for _, v := range lmVarsDeOtroRepo {
				if kv, ok := lmEnEnviron(hijo, v); ok {
					t.Errorf("CA-346: hallazgo 1e4b3e (%s): un hijo lanzado tras LimpiarEntorno no hereda %s: recibio %q", c.nombre, v, kv)
				}
			}
			if _, ok := lmEnEnviron(hijo, "HOOM_TEST_1E4B3E_QUEDA"); !ok {
				t.Errorf("CA-346: guarda 1e4b3e (%s): el hijo hereda las demas variables (HOOM_TEST_1E4B3E_QUEDA)", c.nombre)
			}
		})
	}
}
