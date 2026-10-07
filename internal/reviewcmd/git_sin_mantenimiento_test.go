// Hallazgo 20261006T204527_30abfc: el mantenimiento automatico de git corre
// contra los fixtures. Despues de cada commit (y de merge, rebase, fetch, am)
// git lanza por su cuenta `git maintenance run --auto` —desde git 2.47 suelto
// en segundo plano, con --detach; antes de git 2.29, `git gc --auto`—, que
// crea y borra .git/objects/maintenance.lock mientras el test sigue: un
// proceso de mas por commit, y un archivo que aparece y desaparece debajo del
// que copie o fotografie el repo.
//
// Aca vive lo que los repos de prueba de este paquete llevan para que eso no
// pase (el mantenimiento apagado desde que nacen) y el test que lo prueba. Es
// todo fixture: no hay un criterio de un spec detras. (La copia de plantillas
// que lo exige, y el test de que git lee esta configuracion como se espera,
// estan en internal/agentcmd/git_sin_mantenimiento_test.go.)
package reviewcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitSinMantenimiento es lo que el .git/config de un repo de prueba lleva
// para que git no lance nada por su cuenta. maintenance.auto=false apaga el
// lanzamiento (git 2.29 en adelante); gc.auto=0 es lo mismo para un git
// anterior, que lanza `git gc --auto`, y deja sin nada que hacer al gc que
// alguien lance a mano con --auto. Los worktrees comparten el .git/config de
// su repo, asi que lo heredan; un `git clone` NO: al clon hay que apagarselo
// (gitApagarMantenimiento).
const gitSinMantenimiento = "[maintenance]\n\tauto = false\n[gc]\n\tauto = 0\n"

// gitApagarMantenimiento anexa gitSinMantenimiento al .git/config de root.
// Es un archivo, no un proceso: no cuesta un git mas por repo (este paquete
// corre con -race cerca del limite de tiempo de go test). git(t, dir, "init",
// ...) lo llama solo.
func gitApagarMantenimiento(t *testing.T, root string) {
	t.Helper()
	cfg := filepath.Join(root, ".git", "config")
	// sin O_CREATE: si git no dejo su config, esto no es un repo
	f, err := os.OpenFile(cfg, os.O_WRONLY|os.O_APPEND, 0)
	if err == nil {
		_, err = f.WriteString(gitSinMantenimiento)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		t.Fatalf("fixture: apagar el mantenimiento automatico de git en %s: %v", root, err)
	}
}

// gitEntornoSolo es el entorno del proceso sin nada que le cambie a git su
// configuracion (ni GIT_*, ni el config global, ni el del sistema) y con una
// identidad para commitear: lo que git hace por defecto.
func gitEntornoSolo(t *testing.T) []string {
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
	return append(env, "HOME="+casa, "XDG_CONFIG_HOME="+filepath.Join(casa, "xdg"),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=hoom test", "GIT_AUTHOR_EMAIL=test@hoom.dev",
		"GIT_COMMITTER_NAME=hoom test", "GIT_COMMITTER_EMAIL=test@hoom.dev")
}

// gitCommitConTraza hace un commit vacio en dir con GIT_TRACE=1 y env (nil:
// el entorno del proceso) y devuelve, de la traza, las lineas en las que git
// lanza mantenimiento por su cuenta (`git maintenance run --auto`, o `git gc
// --auto` en un git viejo).
func gitCommitConTraza(t *testing.T, dir string, env []string) []string {
	t.Helper()
	if env == nil {
		env = os.Environ()
	}
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "otro")
	cmd.Dir = dir
	cmd.Env = append(append([]string{}, env...), "GIT_TRACE=1")
	raw, err := cmd.CombinedOutput()
	traza := string(raw)
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

// Hallazgo 30abfc: un repo armado por cada constructor del paquete —los
// clones incluidos, que no heredan el .git/config de su origen— no lanza
// mantenimiento despues de un commit (GIT_TRACE=1 lo mostraria). El control
// es un repo de `git init` a secas, con git sin mas configuracion que la
// suya: ahi el commit SI lo lanza (si no, este test no distinguiria nada), y
// despues de gitApagarMantenimiento deja de lanzarlo.
func TestHallazgo30abfc_LosReposDePruebaNoLanzanMantenimientoDeGit(t *testing.T) {
	solo := gitEntornoSolo(t)
	control := t.TempDir()
	// no pasa por git(): este `init` NO apaga el mantenimiento
	if out, err := rdGitCrudo(control, "init", "-q", "-b", "main"); err != nil {
		t.Fatalf("fixture: git init en %s: %v\n%s", control, err, out)
	}
	if lanza := gitCommitConTraza(t, control, solo); len(lanza) == 0 {
		t.Fatalf("30abfc: control: en un repo de `git init` a secas el commit lanza el mantenimiento automatico " +
			"(git maintenance run --auto, o git gc --auto): este git no lanzo nada, y asi este test no puede ver si los fixtures lo apagan")
	}
	gitApagarMantenimiento(t, control)
	if lanza := gitCommitConTraza(t, control, solo); len(lanza) != 0 {
		t.Fatalf("30abfc: despues de gitApagarMantenimiento el commit no lanza nada; lanzo:\n%s", strings.Join(lanza, "\n"))
	}

	constructores := []struct {
		nombre string
		armar  func(t *testing.T) []string
	}{
		{"repo", func(t *testing.T) []string { return []string{repo(t)} }},
		{"raRepo", func(t *testing.T) []string { return []string{raRepo(t, "")} }},
		{"raRepoSinHoomYaml", func(t *testing.T) []string { return []string{raRepoSinHoomYaml(t)} }},
		{"raRamaSinHoomYamlEnLaBase", func(t *testing.T) []string { return []string{raRamaSinHoomYamlEnLaBase(t, "")} }},
		{"cbRepo", func(t *testing.T) []string { return []string{cbRepo(t)} }},
		{"htRepoLimpio", func(t *testing.T) []string { return []string{htRepoLimpio(t)} }},
		{"raClonShallow", func(t *testing.T) []string { return []string{raClonShallow(t)} }},
		{"raClonShallowDe", func(t *testing.T) []string {
			origen, clon := raClonShallowDe(t, raSoloDocs)
			return []string{origen, clon}
		}},
	}
	for _, c := range constructores {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			for _, root := range c.armar(t) {
				if lanza := gitCommitConTraza(t, root, nil); len(lanza) != 0 {
					t.Fatalf("30abfc: %s: un commit en %s no lanza mantenimiento; lanzo:\n%s", c.nombre, root, strings.Join(lanza, "\n"))
				}
			}
		})
	}
}
