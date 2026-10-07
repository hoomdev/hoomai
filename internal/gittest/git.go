package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// EntornoSolo es el entorno del proceso sin nada que le cambie a git su
// configuracion (ni GIT_*, ni el config global, ni el del sistema), con una
// identidad para commitear y con extra: lo que git hace por defecto.
func EntornoSolo(t testing.TB, extra ...string) []string {
	t.Helper()
	casa := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_") || k == "HOME" || k == "XDG_CONFIG_HOME" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+casa, "XDG_CONFIG_HOME="+filepath.Join(casa, "xdg"),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=hoom test", "GIT_AUTHOR_EMAIL=test@hoom.dev",
		"GIT_COMMITTER_NAME=hoom test", "GIT_COMMITTER_EMAIL=test@hoom.dev")
	return append(env, extra...)
}

// salida corre git en dir con env (nil: el del proceso) y devuelve su salida
// y su error, sin exigir nada.
func salida(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// RepoCrudo es un repo recien creado en dir por `git init` a secas (con
// EntornoSolo, y sin pasar por el git() de ningun paquete): con el
// mantenimiento automatico como git lo trae, prendido. Devuelve dir.
func RepoCrudo(t testing.TB, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("fixture: crear %s: %v", dir, err)
	}
	if out, err := salida(dir, EntornoSolo(t), "init", "-q", "-b", "main"); err != nil {
		t.Fatalf("fixture: git init en %s: %v\n%s", dir, err, out)
	}
	return dir
}

// CommitConTraza hace un commit vacio en dir con GIT_TRACE=1 y env (nil: el
// entorno del proceso) y devuelve, de la traza, las lineas en las que git
// lanza mantenimiento por su cuenta (`git maintenance run --auto`, o `git gc
// --auto` en un git viejo).
func CommitConTraza(t testing.TB, dir string, env []string) []string {
	t.Helper()
	if env == nil {
		env = os.Environ()
	}
	traza, err := salida(dir, append(append([]string{}, env...), "GIT_TRACE=1"),
		"commit", "-q", "--allow-empty", "-m", "otro")
	if err != nil {
		t.Fatalf("fixture: git commit con GIT_TRACE=1 en %s: %v\n%s", dir, err, traza)
	}
	if !strings.Contains(traza, "trace:") {
		t.Fatalf("fixture: GIT_TRACE=1 no dejo traza del commit en %s: sin traza no se ve lo que git lanza\n%s", dir, traza)
	}
	var lanza []string
	for _, l := range strings.Split(traza, "\n") {
		if strings.Contains(l, "maintenance") || strings.Contains(l, "gc --auto") {
			lanza = append(lanza, strings.TrimSpace(l))
		}
	}
	return lanza
}
