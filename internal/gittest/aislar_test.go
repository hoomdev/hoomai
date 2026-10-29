// Tests de AislarGit (hallazgos 20261029T172636_8f2253 y
// 20261029T172811_c4957f): lo que los TestMain de gitx, reviewcmd y boardcmd
// le sacan y le fijan al entorno antes de m.Run. Si AislarGit dejara pasar
// una variable de git del padre, o la config global o la del sistema de quien
// corre los tests, los fixtures de esos paquetes plantarian otra cosa que la
// que dicen (un commit que no se hace, un repo sha256, un archivo ignorado) y
// sus tests fallarian, o pasarian, por eso y no por hoom. Es todo fixture: no
// hay un criterio de un spec detras.
//
// AislarGit cambia el entorno del proceso con os.Setenv y os.Unsetenv. Cada
// test pone antes con t.Setenv todo lo que AislarGit puede tocar (aisProteger),
// asi al terminar el entorno vuelve a ser el de antes para los demas tests del
// paquete.
package gittest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// aisQuitadas son las variables que AislarGit tiene que quitar del entorno,
// escritas aca y no leidas de quitaAislarGit, para que el test no se de la
// razon solo: el repo, la historia (reemplazos, grafts, shallow), la config
// por variables, los atributos, lo que planta git init, los pathspecs y las
// trazas. De las familias va mas de un miembro (GIT_CONFIG_KEY_<n>,
// GIT_TRACE_<x>, GIT_TRACE2_<x>).
var aisQuitadas = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE",
	"GIT_REPLACE_REF_BASE", "GIT_NO_REPLACE_OBJECTS", "GIT_GRAFT_FILE", "GIT_SHALLOW_FILE",
	"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
	"GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_CONFIG_KEY_17", "GIT_CONFIG_VALUE_17",
	"GIT_CONFIG_SYSTEM", "GIT_CONFIG",
	"GIT_ATTR_SOURCE",
	"GIT_DEFAULT_HASH", "GIT_TEMPLATE_DIR", "GIT_ALLOW_PROTOCOL", "GIT_PROTOCOL_FROM_USER",
	"GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS", "GIT_ICASE_PATHSPECS",
	"GIT_TRACE", "GIT_TRACE_PACKET", "GIT_TRACE_SETUP", "GIT_TRACE2", "GIT_TRACE2_EVENT", "GIT_TRACE2_PERF",
}

// aisIntactas son variables que AislarGit no toca: la identidad (los
// fixtures la ponen en la config del repo, y un test puede querer la suya) y
// las que no son de git (HOME y XDG_CONFIG_HOME tambien las leen go y hoom).
var aisIntactas = []string{
	"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_AUTHOR_DATE",
	"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE",
	"HOME", "XDG_CONFIG_HOME", "HOOM_AISLAR_TESTIGO",
}

// aisProteger pone con t.Setenv, a su valor de ahora, cada variable GIT_*
// del entorno y las de nombres: al terminar el test t.Setenv las devuelve a
// ese valor (o las quita, si no estaban), toque AislarGit lo que toque.
func aisProteger(t *testing.T, nombres ...string) {
	t.Helper()
	for _, kv := range os.Environ() {
		if k, v, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "GIT_") {
			t.Setenv(k, v)
		}
	}
	for _, k := range nombres {
		t.Setenv(k, os.Getenv(k))
	}
}

// aisAislar llama a AislarGit y deja para el final del test lo que devuelve.
func aisAislar(t *testing.T) {
	t.Helper()
	deshacer, err := AislarGit()
	if err != nil {
		t.Fatalf("AislarGit: %v", err)
	}
	t.Cleanup(deshacer)
}

// aisGit corre git en dir con el entorno del proceso y devuelve su stdout
// sin espacios alrededor, y su stderr en el error.
func aisGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return strings.TrimSpace(string(out)), &aisErrGit{err: err, stderr: stderr.String()}
	}
	return strings.TrimSpace(string(out)), nil
}

type aisErrGit struct {
	err    error
	stderr string
}

func (e *aisErrGit) Error() string { return e.err.Error() + "\n" + e.stderr }

// aisEsVerdad dice si v es un si de git para una variable booleana.
func aisEsVerdad(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// aisEscribir escribe contenido en p (creando sus directorios) o falla el
// test.
func aisEscribir(t *testing.T, p, contenido string, modo os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contenido), modo); err != nil {
		t.Fatal(err)
	}
}

// Hallazgos 688441, 43f79f, d88bae y 20261029T172636_8f2253: AislarGit
// quita cada variable de git que cambia lo que un fixture planta o lee (con
// valor o vacia, y todos los miembros de las familias), FIJA la config global
// y la del sistema aunque el padre traiga otra (GIT_CONFIG_GLOBAL a un
// archivo que no dice nada de lo de quien corre los tests, y
// GIT_CONFIG_NOSYSTEM y GIT_ATTR_NOSYSTEM en si), no toca la identidad ni
// las variables que no son de git, y lo que devuelve borra lo que creo.
func TestHallazgo8f2253_AislarGitQuitaFijaYNoTocaLoDemas(t *testing.T) {
	casa := t.TempDir()
	hostil := filepath.Join(casa, "gitconfig-hostil")
	aisEscribir(t, hostil, "[commit]\n\tgpgsign = true\n[user]\n\tname = del padre\n", 0o644)

	aisProteger(t, append(append([]string{}, aisIntactas...), "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "GIT_ATTR_NOSYSTEM")...)
	for i, k := range aisQuitadas {
		v := "hostil-" + k
		if i%3 == 0 { // con valor o vacia: AislarGit la quita igual
			v = ""
		}
		t.Setenv(k, v)
	}
	intactas := map[string]string{}
	for _, k := range aisIntactas {
		intactas[k] = "del padre: " + k
		t.Setenv(k, intactas[k])
	}
	t.Setenv("GIT_CONFIG_GLOBAL", hostil)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	t.Setenv("GIT_ATTR_NOSYSTEM", "")

	deshacer, err := AislarGit()
	if err != nil {
		t.Fatalf("AislarGit: %v", err)
	}
	borrado := false
	t.Cleanup(func() {
		if !borrado {
			deshacer()
		}
	})

	for _, k := range aisQuitadas {
		if v, ok := os.LookupEnv(k); ok {
			t.Errorf("8f2253: AislarGit quita %s del entorno; quedo %s=%q", k, k, v)
		}
	}
	for _, k := range aisIntactas {
		if v := os.Getenv(k); v != intactas[k] {
			t.Errorf("8f2253: AislarGit no toca %s; el padre traia %q y quedo %q", k, intactas[k], v)
		}
	}
	for _, k := range []string{"GIT_CONFIG_NOSYSTEM", "GIT_ATTR_NOSYSTEM"} {
		if v := os.Getenv(k); !aisEsVerdad(v) {
			t.Errorf("8f2253: AislarGit fija %s en si, traiga lo que traiga el padre; quedo %q", k, v)
		}
	}
	global := os.Getenv("GIT_CONFIG_GLOBAL")
	if global == "" || global == hostil {
		t.Fatalf("8f2253: AislarGit fija GIT_CONFIG_GLOBAL aunque el padre traiga otra (%s); quedo %q", hostil, global)
	}
	// lo que git lee como config global: nada de quien corre los tests, a lo
	// sumo el apagado del ignore y los attributes de $XDG_CONFIG_HOME/git
	lista, err := aisGit(t.TempDir(), "config", "--global", "--name-only", "--list")
	if err != nil {
		t.Fatalf("8f2253: git config --global --list con el entorno aislado: %v", err)
	}
	for _, clave := range strings.Fields(lista) {
		if c := strings.ToLower(clave); c != "core.excludesfile" && c != "core.attributesfile" {
			t.Errorf("8f2253: la config global que deja AislarGit no dice nada mas que apagar el ignore y los "+
				"attributes de quien corre los tests; dice %s (toda: %q)", clave, lista)
		}
	}

	if global != os.DevNull {
		if _, err := os.Stat(global); err != nil {
			t.Fatalf("8f2253: la config global que fija AislarGit (%s) existe mientras corren los tests: %v", global, err)
		}
		deshacer()
		borrado = true
		if _, err := os.Stat(global); !os.IsNotExist(err) {
			t.Fatalf("8f2253: lo que devuelve AislarGit borra la config global que creo (%s); sigue ahi (%v)", global, err)
		}
	}
}

// Hallazgo 20261029T172636_8f2253, de punta a punta: con un HOME de quien
// corre los tests que firma los commits (commit.gpgsign), los pasa por un
// pre-commit que rechaza (core.hooksPath), arma los repos en sha256
// (init.defaultObjectFormat) e ignora y marca *.go (~/.config/git/ignore y
// attributes, o los de $XDG_CONFIG_HOME/git), un fixture con el entorno que
// deja AislarGit planta lo que dice: `git init` en sha1, el a.go entra, sin
// atributos, y el commit se hace. Venga el padre sin GIT_CONFIG_GLOBAL, con
// GIT_CONFIG_GLOBAL=/dev/null (el caso del hallazgo: la ronda 7 la quitaba
// y volvia el ~/.gitconfig) o con GIT_CONFIG_GLOBAL apuntando a la config
// hostil, y con o sin XDG_CONFIG_HOME.
func TestHallazgo8f2253_AislarGitDejaAfueraLaConfigDeQuienCorreLosTests(t *testing.T) {
	for _, globalDelPadre := range []string{"sin", "devnull", "hostil"} {
		for _, conXDG := range []bool{false, true} {
			nombre := "GIT_CONFIG_GLOBAL-" + globalDelPadre
			if conXDG {
				nombre += "/XDG_CONFIG_HOME"
			}
			t.Run(nombre, func(t *testing.T) { aisDePuntaAPunta(t, globalDelPadre, conXDG) })
		}
	}
}

func aisDePuntaAPunta(t *testing.T, globalDelPadre string, conXDG bool) {
	aisProteger(t, "HOME", "XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "GIT_ATTR_NOSYSTEM")
	casa := t.TempDir()
	ganchos := filepath.Join(casa, "ganchos")
	aisEscribir(t, filepath.Join(ganchos, "pre-commit"), "#!/bin/sh\necho 'pre-commit de quien corre los tests' >&2\nexit 1\n", 0o755)
	gitconfig := "[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = " + filepath.Join(casa, "no-hay-gpg") + "\n" +
		"[core]\n\thooksPath = " + ganchos + "\n[init]\n\tdefaultObjectFormat = sha256\n"
	aisEscribir(t, filepath.Join(casa, ".gitconfig"), gitconfig, 0o644)
	xdg := filepath.Join(casa, ".config")
	t.Setenv("HOME", casa)
	if conXDG {
		xdg = filepath.Join(casa, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)
	} else {
		os.Unsetenv("XDG_CONFIG_HOME") // t.Setenv de aisProteger la devuelve
	}
	aisEscribir(t, filepath.Join(xdg, "git", "ignore"), "*.go\n", 0o644)
	aisEscribir(t, filepath.Join(xdg, "git", "attributes"), "*.go hostil\n", 0o644)
	switch globalDelPadre {
	case "sin":
		os.Unsetenv("GIT_CONFIG_GLOBAL")
	case "devnull":
		t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	case "hostil":
		t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(casa, ".gitconfig"))
	}
	os.Unsetenv("GIT_CONFIG_NOSYSTEM")
	os.Unsetenv("GIT_ATTR_NOSYSTEM")

	// controles: lo plantado es lo que git lee sin aislar. Sin GIT_CONFIG_GLOBAL
	// (lo que dejaba la ronda 7) git lee el ~/.gitconfig, y el ignore y los
	// attributes los lee aun con GIT_CONFIG_GLOBAL=/dev/null.
	sinGlobal := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_CONFIG_GLOBAL=") {
			sinGlobal = append(sinGlobal, kv)
		}
	}
	vacio := t.TempDir()
	cmd := exec.Command("git", "config", "--global", "--get", "commit.gpgsign")
	cmd.Dir, cmd.Env = vacio, sinGlobal
	if out, err := cmd.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		t.Fatalf("8f2253: control: sin GIT_CONFIG_GLOBAL git lee el ~/.gitconfig plantado en %s (commit.gpgsign); leyo %q (%v)", casa, out, err)
	}
	control := filepath.Join(t.TempDir(), "control")
	if out, err := aisGit("", "init", "-q", control); err != nil {
		t.Fatalf("fixture: git init %s: %v\n%s", control, err, out)
	}
	ApagarMantenimiento(t, control)
	if _, err := aisGit(control, "check-ignore", "-q", "a.go"); err != nil {
		t.Fatalf("8f2253: control: sin aislar, git ignora a.go por el %s/git/ignore plantado: %v", xdg, err)
	}
	if attr, err := aisGit(control, "check-attr", "hostil", "--", "a.go"); err != nil || !strings.HasSuffix(attr, ": set") {
		t.Fatalf("8f2253: control: sin aislar, a.go tiene el atributo hostil del %s/git/attributes plantado; "+
			"git check-attr: %q (%v)", xdg, attr, err)
	}

	aisAislar(t)

	root := filepath.Join(t.TempDir(), "repo")
	if out, err := aisGit("", "init", "-q", "-b", "main", root); err != nil {
		t.Fatalf("8f2253: con el entorno de AislarGit `git init` anda: %v\n%s", err, out)
	}
	ApagarMantenimiento(t, root)
	for _, kv := range [][2]string{{"user.name", "hoom test"}, {"user.email", "test@hoom.dev"}} {
		if out, err := aisGit(root, "config", kv[0], kv[1]); err != nil {
			t.Fatalf("fixture: git config %s: %v\n%s", kv[0], err, out)
		}
	}
	aisEscribir(t, filepath.Join(root, "a.go"), "package a\n", 0o644)
	if out, err := aisGit(root, "add", "-A"); err != nil {
		t.Fatalf("8f2253: con el entorno de AislarGit `git add -A` anda: %v\n%s", err, out)
	}
	archivos, err := aisGit(root, "ls-files")
	if err != nil {
		t.Fatalf("fixture: git ls-files: %v", err)
	}
	if got := strings.Fields(archivos); !(len(got) == 1 && got[0] == "a.go") {
		sort.Strings(got)
		t.Fatalf("8f2253: con el entorno de AislarGit el a.go entra (el ignore de quien corre los tests no cuenta); "+
			"en el indice: %q", got)
	}
	if attr, err := aisGit(root, "check-attr", "hostil", "--", "a.go"); err != nil || !strings.HasSuffix(attr, ": unspecified") {
		t.Errorf("8f2253: con el entorno de AislarGit a.go no tiene los attributes de quien corre los tests; "+
			"git check-attr: %q (%v)", attr, err)
	}
	if out, err := aisGit(root, "commit", "-q", "-m", "uno"); err != nil {
		t.Fatalf("8f2253: con el entorno de AislarGit el commit del fixture se hace, sin firmar y sin el pre-commit "+
			"de quien corre los tests: %v\n%s", err, out)
	}
	sha, err := aisGit(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("fixture: git rev-parse HEAD: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("8f2253: con el entorno de AislarGit `git init` arma un repo sha1, no el init.defaultObjectFormat "+
			"de quien corre los tests; HEAD es %q", sha)
	}
}
