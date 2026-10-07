// Hallazgo 20261006T204527_30abfc: el mantenimiento automatico de git corre
// contra los fixtures que copian un repo. Despues de cada commit (y de merge,
// rebase, fetch, am) git lanza por su cuenta `git maintenance run --auto`
// —desde git 2.47 suelto en segundo plano, con --detach; antes de git 2.29,
// `git gc --auto`—, que crea y borra .git/objects/maintenance.lock. El que
// copia el repo recien commiteado lista el lock y, cuando lo va a mirar, ya
// no esta: en el CI, una de cada ocho corridas.
//
// Lo que los repos de prueba llevan para que eso no pase (el mantenimiento
// apagado desde que nacen) y lo que la copia les exige viven en
// internal/gittest, con sus tests (hallazgo 20261007T142722_de562b: una sola
// copia de la fixture, que tambien usa reviewcmd). Aca quedan los tests de
// este paquete: que cada constructor arma su repo con el mantenimiento
// apagado, y que la copia se niega con un repo que no lo traiga asi. Es todo
// fixture: no hay un criterio de un spec detras.
package agentcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gittest"
)

// Hallazgo 30abfc: un repo armado por cada constructor del paquete que hace
// `git init` (los demas —repo, repoCiego, h3Repo, qcRepoConHallazgo, las
// plantillas— parten de uno de estos) no lanza mantenimiento despues de un
// commit (GIT_TRACE=1 lo mostraria), y su .git/config pasa lo que la copia
// exige. Un worktree del repo, tampoco: comparte su .git/config. El control
// es un repo de `git init` a secas, con git sin mas configuracion que la
// suya: ahi el commit SI lo lanza (si no, este test no distinguiria nada), y
// despues de gittest.ApagarMantenimiento deja de lanzarlo.
func TestHallazgo30abfc_LosReposDePruebaNoLanzanMantenimientoDeGit(t *testing.T) {
	control := gittest.RepoCrudo(t, t.TempDir())
	solo := gittest.EntornoSolo(t)
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
		armar  func(t *testing.T) string
		// limpia: el constructor limpia el entorno del proceso
		// (qcLimpiarEntorno), y por eso no corre en paralelo
		limpia bool
	}{
		{"repoGate", func(t *testing.T) string { return repoGate(t, "true") }, false},
		{"piRepo", piRepo, false},
		{"raRepoConReview", raRepoConReview, false},
		{"edRepo", func(t *testing.T) string { return edRepo(t, true, true) }, true},
		{"qcr1RepoBloqueante", func(t *testing.T) string { root, _ := qcr1RepoBloqueante(t); return root }, true},
		{"qcr2RepoCiegoBloqueante", func(t *testing.T) string { root, _ := qcr2RepoCiegoBloqueante(t); return root }, true},
	}
	for _, c := range constructores {
		t.Run(c.nombre, func(t *testing.T) {
			if !c.limpia {
				t.Parallel()
			}
			root := c.armar(t)
			if err := gittest.ExigirMantenimientoApagado(filepath.Join(root, ".git", "config")); err != nil {
				t.Fatalf("30abfc: %s: %v", c.nombre, err)
			}
			if lanza := gittest.CommitConTraza(t, root, nil); len(lanza) != 0 {
				t.Fatalf("30abfc: %s: un commit en el repo no lanza mantenimiento; lanzo:\n%s", c.nombre, strings.Join(lanza, "\n"))
			}
			if c.nombre != "repoGate" {
				return
			}
			// un worktree (asi arma hoom las tareas) comparte el config del repo
			wt := filepath.Join(t.TempDir(), "wt")
			git(t, root, "worktree", "add", "-q", "-b", "hoom/tarea", wt)
			if lanza := gittest.CommitConTraza(t, wt, nil); len(lanza) != 0 {
				t.Fatalf("30abfc: %s: un commit en un worktree del repo no lanza mantenimiento; lanzo:\n%s", c.nombre, strings.Join(lanza, "\n"))
			}
		})
	}
}

// Hallazgo 30abfc: edCopiarArbol se niega SIEMPRE a copiar un repo con el
// mantenimiento automatico prendido —el de la raiz de la plantilla o uno de
// mas adentro—, antes de copiar su .git, y nombra lo que falta. Con el
// mantenimiento apagado copia, la copia lo trae apagado (se puede volver a
// copiar) y un commit en la copia no lanza nada. Un arbol sin repo adentro
// se copia sin pedirle nada.
func TestHallazgo30abfc_LaCopiaSeNiegaConElMantenimientoPrendido(t *testing.T) {
	negada := func(t *testing.T, caso, plantilla, repoPrendido string) {
		t.Helper()
		destino := t.TempDir()
		err := edCopiarArbolEn(plantilla, destino)
		if err == nil {
			t.Fatalf("30abfc: %s: la copia de un repo con el mantenimiento automatico prendido se niega; copio", caso)
		}
		for _, pide := range []string{"maintenance.auto", "gc.auto", filepath.Join(repoPrendido, ".git", "config")} {
			if !strings.Contains(err.Error(), pide) {
				t.Fatalf("30abfc: %s: la negativa nombra %q: %v", caso, pide, err)
			}
		}
		rel, rerr := filepath.Rel(plantilla, repoPrendido)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if _, serr := os.Lstat(filepath.Join(destino, rel, ".git")); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("30abfc: %s: la copia se niega antes de copiar el .git del repo: %v", caso, serr)
		}
	}

	// el repo de la raiz de la plantilla: recien creado, y con cada forma de
	// config que git lee prendida
	prendido := gittest.RepoCrudo(t, t.TempDir())
	write(t, prendido, "app.go", "package app\n")
	negada(t, "git init a secas", prendido, prendido)
	cfg := filepath.Join(prendido, ".git", "config")
	base, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range gittest.ConfigsDeMantenimiento() {
		if c.ApagadoParaGit {
			continue
		}
		if err := os.WriteFile(cfg, append(append([]byte{}, base...), c.Anexo...), 0o644); err != nil {
			t.Fatal(err)
		}
		negada(t, c.Caso, prendido, prendido)
	}
	if err := os.WriteFile(cfg, base, 0o644); err != nil {
		t.Fatal(err)
	}

	// un repo de mas adentro, con el de la raiz bien armado
	anidada := repo(t)
	adentro := gittest.RepoCrudo(t, filepath.Join(anidada, "sub", "otro"))
	negada(t, "un repo anidado con el mantenimiento prendido", anidada, adentro)
	gittest.ApagarMantenimiento(t, adentro)
	if err := edCopiarArbolEn(anidada, t.TempDir()); err != nil {
		t.Fatalf("30abfc: con los dos repos apagados la copia sale: %v", err)
	}

	// apagado: copia, y la copia se puede volver a copiar y commitear
	gittest.ApagarMantenimiento(t, prendido)
	copia := edCopiarArbol(t, prendido)
	if raw, err := os.ReadFile(filepath.Join(copia, "app.go")); err != nil || string(raw) != "package app\n" {
		t.Fatalf("30abfc: la copia trae el arbol de la plantilla: %q %v", raw, err)
	}
	if err := gittest.ExigirMantenimientoApagado(filepath.Join(copia, ".git", "config")); err != nil {
		t.Fatalf("30abfc: la copia trae el mantenimiento apagado como su plantilla: %v", err)
	}
	copiaDeLaCopia := edCopiarArbol(t, copia)
	if lanza := gittest.CommitConTraza(t, copiaDeLaCopia, gittest.EntornoSolo(t)); len(lanza) != 0 {
		t.Fatalf("30abfc: un commit en la copia no lanza mantenimiento; lanzo:\n%s", strings.Join(lanza, "\n"))
	}

	// un arbol sin repo (un pedazo de .hoom) no tiene nada que apagar
	suelto := t.TempDir()
	write(t, suelto, "findings/a.json", "{}\n")
	if err := edCopiarArbolEn(suelto, t.TempDir()); err != nil {
		t.Fatalf("30abfc: un arbol sin repo adentro se copia sin pedirle nada: %v", err)
	}
}
