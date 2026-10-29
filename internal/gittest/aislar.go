package gittest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// quitaAislarGit son las variables de git que AislarGit quita del entorno:
// las que, heredadas por el go test, cambian lo que un fixture planta o lee
// (hallazgos 688441, 43f79f y d88bae, de la review de
// evidencia-de-la-review).
// Un nombre que termina en "_" es una familia (GIT_CONFIG_KEY_<n>,
// GIT_TRACE2_EVENT, ...).
var quitaAislarGit = []string{
	// el repositorio, el arbol, el indice, los objetos y las refs
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE",
	// la historia: reemplazos, grafts y la frontera de un clon shallow
	"GIT_REPLACE_REF_BASE", "GIT_NO_REPLACE_OBJECTS", "GIT_GRAFT_FILE", "GIT_SHALLOW_FILE",
	// la config que no es la global ni la del sistema (esas las fija AislarGit)
	"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_",
	"GIT_CONFIG_SYSTEM", "GIT_CONFIG",
	// de donde lee los atributos
	"GIT_ATTR_SOURCE",
	// lo que planta git init y con que protocolo clona
	"GIT_DEFAULT_HASH", "GIT_TEMPLATE_DIR", "GIT_ALLOW_PROTOCOL", "GIT_PROTOCOL_FROM_USER",
	// como lee los pathspecs
	"GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS", "GIT_ICASE_PATHSPECS",
	// lo que agrega por stderr (los git(t, ...) de los paquetes leen stdout y
	// stderr juntos)
	"GIT_TRACE", "GIT_TRACE_", "GIT_TRACE2", "GIT_TRACE2_",
}

// configGlobalAislada es el contenido del archivo que AislarGit deja como
// config global: nada mas que apagar los dos archivos que git lee por su
// cuenta fuera de toda config, $XDG_CONFIG_HOME/git/ignore y
// $XDG_CONFIG_HOME/git/attributes (o ~/.config/git/...), que tambien son del
// que corre los tests.
const configGlobalAislada = "[core]\n\texcludesFile = " + os.DevNull + "\n\tattributesFile = " + os.DevNull + "\n"

// AislarGit deja el entorno de ESTE proceso, el de un binario de tests, con
// git aislado de lo que trae quien lo corre: la llaman, antes de m.Run, los
// TestMain de los paquetes que plantan repos de prueba y corren hoom en el
// mismo proceso (gitx, reviewcmd y boardcmd). Asi el fixture, sus guardas y
// hoom ven el mismo git, venga lo que venga del padre:
//
//   - quita las variables de git que cambian lo que un fixture planta o lee
//     (quitaAislarGit: el repo, la historia, la config por variables, los
//     atributos, lo que planta git init, los pathspecs y las trazas), con
//     valor o vacias;
//   - FIJA la config global y la del sistema: GIT_CONFIG_GLOBAL a un archivo
//     propio con configGlobalAislada, GIT_CONFIG_NOSYSTEM=1 y
//     GIT_ATTR_NOSYSTEM=1. Quitarlas no alcanza (hallazgo
//     20261029T172636_8f2253): git volveria a leer el ~/.gitconfig y el
//     /etc/gitconfig de quien corre los tests (commit.gpgsign,
//     core.hooksPath, init.templateDir, init.defaultObjectFormat,
//     url.<x>.insteadOf...), y un GIT_CONFIG_GLOBAL del padre le ganaria. El
//     archivo propio, y no os.DevNull como en EntornoSolo, es porque con
//     GIT_CONFIG_GLOBAL=/dev/null git igual lee el ignore y los attributes
//     de $XDG_CONFIG_HOME/git; asi no hay que tocar HOME ni XDG_CONFIG_HOME,
//     que tambien leen go y hoom.
//
// No toca la identidad (GIT_AUTHOR_*, GIT_COMMITTER_*: los fixtures la ponen
// en la config del repo) ni lo que no cambia lo que git planta o lee
// (GIT_EDITOR, GIT_PAGER, GIT_SSH*, GIT_EXEC_PATH, ...). Un test que prueba
// que hoom resiste una de estas variables, o que necesita otra config
// global, la pone con t.Setenv: le gana a esto durante ese test.
//
// Devuelve borrarConfig, para despues de m.Run: borra el archivo de la
// config global y nada mas; no le devuelve al entorno lo que AislarGit le
// saco o le fijo (hallazgo 20261029T193927_c4ebe3). Es una funcion de
// proceso y no recibe un testing.TB porque corre antes que todo test; los
// tres TestMain la comparten desde aca, y no con una copia cada uno
// (hallazgo 20261029T172811_c4957f, como 20261007T142722_de562b).
func AislarGit() (borrarConfig func(), err error) {
	for _, kv := range os.Environ() {
		if nombre, _, _ := strings.Cut(kv, "="); seQuitaEnAislarGit(nombre) {
			if err := os.Unsetenv(nombre); err != nil {
				return nil, fmt.Errorf("aislar git: quitar %s: %w", nombre, err)
			}
		}
	}
	dir, err := os.MkdirTemp("", "gittest-aislargit-")
	if err != nil {
		return nil, fmt.Errorf("aislar git: %w", err)
	}
	borrarConfig = func() { os.RemoveAll(dir) }
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, []byte(configGlobalAislada), 0o644); err != nil {
		borrarConfig()
		return nil, fmt.Errorf("aislar git: %w", err)
	}
	for _, kv := range [][2]string{{"GIT_CONFIG_GLOBAL", global}, {"GIT_CONFIG_NOSYSTEM", "1"}, {"GIT_ATTR_NOSYSTEM", "1"}} {
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			borrarConfig()
			return nil, fmt.Errorf("aislar git: fijar %s: %w", kv[0], err)
		}
	}
	return borrarConfig, nil
}

// seQuitaEnAislarGit dice si nombre es una de quitaAislarGit o de sus
// familias.
func seQuitaEnAislarGit(nombre string) bool {
	if runtime.GOOS == "windows" { // ahi el entorno no distingue mayusculas
		nombre = strings.ToUpper(nombre)
	}
	for _, v := range quitaAislarGit {
		if nombre == v || (strings.HasSuffix(v, "_") && strings.HasPrefix(nombre, v)) {
			return true
		}
	}
	return false
}
