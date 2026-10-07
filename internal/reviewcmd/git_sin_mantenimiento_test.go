// Hallazgo 20261006T204527_30abfc: el mantenimiento automatico de git corre
// contra los fixtures. Despues de cada commit (y de merge, rebase, fetch, am)
// git lanza por su cuenta `git maintenance run --auto` —desde git 2.47 suelto
// en segundo plano, con --detach; antes de git 2.29, `git gc --auto`—, que
// crea y borra .git/objects/maintenance.lock mientras el test sigue: un
// proceso de mas por commit, y un archivo que aparece y desaparece debajo del
// que copie o fotografie el repo.
//
// Lo que los repos de prueba llevan para que eso no pase (el mantenimiento
// apagado desde que nacen) vive en internal/gittest, con sus tests (hallazgo
// 20261007T142722_de562b: una sola copia de la fixture, que tambien usa
// agentcmd). Aca queda el test de este paquete: que cada constructor, los
// clones incluidos, arma su repo con el mantenimiento apagado. Es todo
// fixture: no hay un criterio de un spec detras.
package reviewcmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gittest"
)

// Hallazgo 30abfc: un repo armado por cada constructor del paquete —los
// clones incluidos, que no heredan el .git/config de su origen— no lanza
// mantenimiento despues de un commit (GIT_TRACE=1 lo mostraria), y su
// .git/config lo trae apagado por escrito (lo mismo que agentcmd le exige al
// repo que copia: no depende de la configuracion de git del que corre los
// tests). El control es un repo de `git init` a secas, con git sin mas
// configuracion que la suya: ahi el commit SI lo lanza (si no, este test no
// distinguiria nada), y despues de gittest.ApagarMantenimiento deja de
// lanzarlo.
func TestHallazgo30abfc_LosReposDePruebaNoLanzanMantenimientoDeGit(t *testing.T) {
	solo := gittest.EntornoSolo(t)
	// no pasa por git(): este `init` NO apaga el mantenimiento
	control := gittest.RepoCrudo(t, t.TempDir())
	if lanza := gittest.CommitConTraza(t, control, solo); len(lanza) == 0 {
		t.Fatalf("30abfc: control: en un repo de `git init` a secas el commit lanza el mantenimiento automatico " +
			"(git maintenance run --auto, o git gc --auto): este git no lanzo nada, y asi este test no puede ver si los fixtures lo apagan")
	}
	gittest.ApagarMantenimiento(t, control)
	if lanza := gittest.CommitConTraza(t, control, solo); len(lanza) != 0 {
		t.Fatalf("30abfc: despues de gittest.ApagarMantenimiento el commit no lanza nada; lanzo:\n%s", strings.Join(lanza, "\n"))
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
				if err := gittest.ExigirMantenimientoApagado(filepath.Join(root, ".git", "config")); err != nil {
					t.Fatalf("30abfc: %s: %v", c.nombre, err)
				}
				if lanza := gittest.CommitConTraza(t, root, nil); len(lanza) != 0 {
					t.Fatalf("30abfc: %s: un commit en %s no lanza mantenimiento; lanzo:\n%s", c.nombre, root, strings.Join(lanza, "\n"))
				}
			}
		})
	}
}
