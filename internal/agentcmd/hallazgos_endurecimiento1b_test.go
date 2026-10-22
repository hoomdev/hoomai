// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// agentcmd, sobre .hoom/specs/evidencia-en-disco.md (CA-442) y el gate de
// scope de .hoom/specs/hoom-agent-sobre-determinista.md (CA-133, CA-141).
// Escritos desde el spec, el texto del hallazgo y el contrato de la
// correccion, SIN LEER LA IMPLEMENTACION. Reusan repo, git, write y
// qcLimpiarEntorno de los tests del paquete; los helpers de este archivo
// llevan el prefijo e1b. El caso con un FIFO esta en
// hallazgos_endurecimiento1b_unix_test.go.
//
//   - 20261022T202056_6d4918 (high, risk): con un .git/index corrupto,
//     `git diff HEAD` y `git ls-files` fallan enseguida mientras base...HEAD
//     sigue listando un cambio permitido; gitx.TouchedErr devolvia esa lista
//     parcial sin error, Take dejaba Snapshot.Listado vacio y el gate no
//     cortaba aunque el rol hubiera dejado archivos fuera de scope. La
//     frontera de scope falla cerrada ante cualquier fallo de los git que
//     necesita para listar: Listado no vacio y el gate corta.
package agentcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
)

// CA-442, CA-133, CA-141 (hallazgo 6d4918): el rol writer (scope codigo)
// trabaja en la rama hoom/tarea: commitea un cambio permitido
// (internal/permitido.go, que main...HEAD lista), deja un archivo fuera de
// su scope (.hoom/specs/fuera.md) y corrompe el .git/index, asi `git diff
// HEAD` y `git ls-files` fallan enseguida. La foto de despues no toma la
// lista parcial por todo lo que cambio: Snapshot.Listado dice por que git
// no pudo listar (no vacio), y el gate de scope con esa foto corta. Guarda:
// la foto de antes (indice sano) no tiene Listado.
func TestHallazgo_6d4918_CA442_IndiceCorruptoNoEscondeLoQueElRolToco(t *testing.T) {
	const ca = "CA-442 (hallazgo 6d4918, .git/index corrupto)"
	qcLimpiarEntorno(t)
	root := repo(t)
	git(t, root, "checkout", "-q", "-b", "hoom/tarea")
	before := Take(root, "main")
	if before.Listado != "" {
		t.Fatalf("%s: guarda: con el indice sano, git lista y la foto no dice que no pudo: %q", ca, before.Listado)
	}

	write(t, root, "internal/permitido.go", "package internal\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "cambio permitido")
	write(t, root, ".hoom/specs/fuera.md", "# fuera del scope del writer\n")
	if err := os.WriteFile(filepath.Join(root, ".git", "index"), []byte("basura de la prueba e1b"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"diff", "--name-only", "HEAD"}, {"ls-files", "--others", "--exclude-standard"}} {
		c := exec.Command("git", args...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err == nil {
			t.Skipf("fixture: con el .git/index corrupto, git %v no falla en este sistema: %s", args, out)
		}
	}

	after := Take(root, "main")
	if after.Listado == "" {
		t.Errorf("%s: con el .git/index corrupto git no pudo listar lo que cambio sin commitear, y la foto lo dice: Snapshot.Listado no vacio; Touched=%v",
			ca, after.Touched)
	}
	rol, err := agents.Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
	if !res.Cuts() {
		t.Errorf("%s: con la foto de despues sin listado completo, el gate de scope corta (falla cerrado), aunque lo que llego a listar este en scope: %+v",
			ca, res)
	}
}
