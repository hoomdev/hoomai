// Tests de regresion de la ronda 5 de ciclo-de-review-acotado: Save desde el
// worktree de una tarea que quedo fuera de su rama (hallazgo
// 20261112T034738_13bba6; contrato del hallazgo 20261021T145451_68644c). Los
// comandos que guardan en una tarea se niegan si su worktree no esta en
// hoom/<slug>: runcmd.TaskDir es la puerta y su error envuelve
// runcmd.ErrOtraRama. Con el worktree en otra rama, con HEAD separado, en
// main o en hoom/<slug>-viejo (worktreetest.RamasAjenas) y el spec cambiado,
// Save(<worktree>, ..., expect) con lo que la tarjeta mostro se niega con el
// error de rama (errors.Is ErrOtraRama, el mismo texto que runcmd.TaskDir) y
// no se crea ningun commit en ningun arbol ni en ninguna rama; el spec sigue
// cambiado. Guardas: `hoom item save <slug>` sin rutas desde ese worktree
// tambien se niega sin commits, y de vuelta en hoom/<slug> el ida y vuelta de
// la ronda 4 guarda.
//
// Reusa los fixtures de la ronda 4 (crR4Tarea, crR4Vista, crR4Riesgo, ...).
// Ningun CLI de IA: el PATH tiene solo git y sh (crR4PATH). Escritos desde el
// contrato, las firmas publicas y los tests existentes de la rama, sin leer
// la implementacion.
package itemcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

const crR5Hallazgos = "hallazgos 20261112T034738_13bba6 y 20261021T145451_68644c"

func crR5Spec(t *testing.T, wt string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wt, filepath.FromSlash(crR4SpecRel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// crR5Foto junta lo que un save rechazado no puede tocar: todas las ramas, el
// registro de worktrees, y HEAD (con su rama), indice y estado del principal
// y del worktree, mas el spec del worktree.
func crR5Foto(t *testing.T, root, wt string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("ramas:\n" + crR3Git(t, root, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"))
	b.WriteString("worktrees:\n" + crR3Git(t, root, "worktree", "list", "--porcelain"))
	for _, d := range []string{root, wt} {
		b.WriteString(d + "\nHEAD " + strings.TrimSpace(crR3Git(t, d, "rev-parse", "--symbolic-full-name", "HEAD")) + " " + crHead(t, d) + "\n")
		b.WriteString("indice:\n" + crR3Git(t, d, "ls-files", "-s"))
		b.WriteString("estado:\n" + crR3Git(t, d, "status", "--porcelain", "--untracked-files=all"))
	}
	b.WriteString("spec del worktree:\n" + crR5Spec(t, wt))
	return b.String()
}

// crR5FueraDeSuRama deja el worktree de la tarea fuera de hoom/precios con v
// (el worktree tiene que estar limpio) y despues le cambia el spec.
func crR5FueraDeSuRama(t *testing.T, ca, wt string, v worktreetest.RamaAjena) {
	t.Helper()
	v.Armar(t, wt, crR4Slug)
	crR4EnsuciarSpec(t, wt)
	if rama := strings.TrimSpace(crR3Git(t, wt, "rev-parse", "--symbolic-full-name", "HEAD")); rama == "refs/heads/hoom/"+crR4Slug {
		t.Fatalf("%s: fixture: el worktree queda fuera de hoom/%s", ca, crR4Slug)
	}
}

// crR5LoMostrado es el expect que manda el Studio: el unsynced de la tarjeta
// vista desde el worktree ahora, o, si no muestra nada (o no se deriva), el
// que mostraba en su rama.
func crR5LoMostrado(wt string, antes []string) []string {
	c, err := boardcmd.CardFor(wt, "main", "high", crR4Slug, time.Now().UTC())
	if err == nil && len(c.Unsynced) > 0 {
		return append([]string{}, c.Unsynced...)
	}
	return antes
}

// crR5ErrorDeRama es el error de rama que dan los comandos de la tarea: el de
// runcmd.TaskDir desde el principal, que envuelve runcmd.ErrOtraRama.
func crR5ErrorDeRama(t *testing.T, ca, root string) string {
	t.Helper()
	got, err := runcmd.TaskDir(root, crR4Slug)
	if !errors.Is(err, runcmd.ErrOtraRama) {
		t.Fatalf("%s: fixture: runcmd.TaskDir dice que el worktree esta fuera de su rama (ErrOtraRama): %q %v", ca, got, err)
	}
	return err.Error()
}

// hallazgos 20261112T034738_13bba6 y 20261021T145451_68644c: con el worktree
// de la tarea fuera de hoom/precios (cada variante de
// worktreetest.RamasAjenas: HEAD separado, otra rama, main,
// hoom/precios-viejo) y el spec cambiado, Save(<worktree>, ..., expect) con
// lo que la tarjeta mostro se niega con el error de rama: envuelve
// runcmd.ErrOtraRama, nombra hoom/precios y la rama, trae el texto de
// runcmd.TaskDir y no es ErrChanged. No hay commit en ningun arbol ni se
// mueve ninguna rama (ni la equivocada, ni hoom/precios, ni main), el indice
// y el estado de los dos arboles quedan igual y el spec sigue cambiado.
func TestHallazgo_13bba6_68644c_SaveDesdeElWorktreeFueraDeSuRamaEsErrOtraRama(t *testing.T) {
	crR4PATH(t)
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			ca := crR5Hallazgos + ", " + v.Nombre
			root, wt := crR4Tarea(t, true)
			crR4EnsuciarSpec(t, wt)
			antes := crR4Vista(t, ca+" (fixture, en su rama)", wt)
			icEscribir(t, wt, crR4SpecRel, crR4Spec)
			crR5FueraDeSuRama(t, ca, wt, v)
			expect := crR5LoMostrado(wt, antes)
			esperado := crR5ErrorDeRama(t, ca, root)
			foto := crR5Foto(t, root, wt)

			res, err := Save(wt, "main", "high", crR4Slug, expect)
			if err == nil {
				t.Fatalf("%s: con el worktree fuera de su rama, Save con %v se niega con el error de rama: %+v", ca, expect, res)
			}
			if !errors.Is(err, runcmd.ErrOtraRama) {
				t.Fatalf("%s: el error de Save envuelve runcmd.ErrOtraRama: %v", ca, err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "hoom/"+crR4Slug) || !strings.Contains(msg, "rama") || strings.Contains(msg, "no existe") {
				t.Fatalf("%s: el error dice que el worktree no esta en su rama hoom/%s (y no \"no existe\"): %q", ca, crR4Slug, msg)
			}
			if strings.Contains(msg, ErrChanged) {
				t.Fatalf("%s: la negativa es por la rama, no ErrChanged: %q", ca, msg)
			}
			if !strings.Contains(msg, esperado) {
				t.Fatalf("%s: el error es el mismo que dan los comandos de la tarea (runcmd.TaskDir):\nquiero: %q\ndijo:   %q", ca, esperado, msg)
			}
			if len(res.Commits) != 0 {
				t.Fatalf("%s: la negativa no informa commits: %+v", ca, res.Commits)
			}
			if despues := crR5Foto(t, root, wt); despues != foto {
				t.Fatalf("%s: la negativa no commitea en ningun arbol ni mueve ninguna rama:\nantes:\n%s\ndespues:\n%s", ca, foto, despues)
			}
			if crR5Spec(t, wt) != crR4SpecSucio {
				t.Fatalf("%s: el spec del worktree sigue cambiado", ca)
			}
		})
	}
}

// Guarda de 13bba6 / 68644c: `hoom item save precios` sin rutas desde el
// worktree fuera de su rama (cada variante de worktreetest.RamasAjenas), con
// el spec cambiado en el worktree y el riesgo del item del principal cambiado
// (antes de salir de la rama), se niega y no commitea nada: ni el spec, ni
// el item del principal, ni en ninguna rama. Los dos cambios siguen sin
// guardar.
func TestHallazgo_13bba6_68644c_ItemSaveSinRutasFueraDeSuRamaSeNiega(t *testing.T) {
	crR4PATH(t)
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			ca := crR5Hallazgos + ", hoom item save sin rutas, " + v.Nombre
			root, wt := crR4Tarea(t, true)
			crR4Riesgo(t, root, wt, true)
			crR5FueraDeSuRama(t, ca, wt, v)
			foto := crR5Foto(t, root, wt)

			out, errb, err := cbExec(t, wt, "save", crR4Slug)
			if err == nil {
				t.Fatalf("%s: hoom item save %s desde el worktree fuera de su rama se niega:\n%s%s", ca, crR4Slug, out, errb)
			}
			if despues := crR5Foto(t, root, wt); despues != foto {
				t.Fatalf("%s: la negativa no commitea en ningun arbol ni mueve ninguna rama:\nantes:\n%s\ndespues:\n%s", ca, foto, despues)
			}
			if crR5Spec(t, wt) != crR4SpecSucio {
				t.Fatalf("%s: el spec del worktree sigue cambiado", ca)
			}
			if !strings.Contains(crR2Leer(t, root, crR4Slug), "riesgo: alto") {
				t.Fatalf("%s: el item del principal sigue con riesgo: alto sin guardar", ca)
			}
		})
	}
}

// Guarda de 13bba6 / 68644c: el worktree paso por otra rama con el spec
// cambiado y volvio a hoom/precios: el ida y vuelta de la ronda 4 (Save desde
// el worktree con lo que la tarjeta mostro) guarda, con exactamente un
// commit, en hoom/precios, solo con el spec. El principal no gana commit y
// la rama por la que paso no se mueve.
func TestHallazgo_13bba6_68644c_DeVueltaEnSuRamaGuarda(t *testing.T) {
	crR4PATH(t)
	ca := crR5Hallazgos + ", guarda de vuelta en hoom/precios"
	root, wt := crR4Tarea(t, true)
	crR4EnsuciarSpec(t, wt)
	icGit(t, wt, "checkout", "-q", "-b", "otra")
	worktreetest.VolverASuRama(t, wt, crR4Slug)
	headP, headW := crHead(t, root), crHead(t, wt)
	otra := strings.TrimSpace(crR3Git(t, root, "rev-parse", "refs/heads/otra"))

	expect := crR4Vista(t, ca, wt)
	res, err := Save(wt, "main", "high", crR4Slug, expect)
	if err != nil {
		t.Fatalf("%s: de vuelta en su rama, Save desde el worktree con %v guarda: %v", ca, expect, err)
	}
	crR4Resultado(t, ca, res, crHead(t, wt))
	crR4WorktreeGuardado(t, ca, wt, headW, crR4SpecRel)
	if crHead(t, root) != headP {
		t.Fatalf("%s: el principal no gana commit", ca)
	}
	if got := strings.TrimSpace(crR3Git(t, root, "rev-parse", "refs/heads/otra")); got != otra {
		t.Fatalf("%s: la rama otra no se mueve: %s -> %s", ca, otra, got)
	}
}
