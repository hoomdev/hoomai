// Tests de regresion de la ronda 6 de ciclo-de-review-acotado: guardar OTRA
// tarjeta desde el worktree de una tarea que quedo fuera de su rama
// (hallazgo 20261112T040826_65fbb7; contrato del hallazgo
// 20261021T145451_68644c). La ronda 5 cubrio la tarjeta del propio worktree;
// aca el proyecto tiene dos tareas, precios (con worktree) y otro (item y
// spec commiteados en el principal), y en el worktree de precios, fuera de
// hoom/precios (worktreetest.RamasAjenas), esta cambiado el spec de otro.
// Save(<worktree de precios>, ..., "otro", expect) y `hoom item save otro`
// sin rutas desde ahi se niegan con el error de rama de la tarea precios (el
// texto de runcmd.TaskDir(principal, "precios"); Save envuelve
// runcmd.ErrOtraRama), sin commit en ningun arbol ni movimiento de ninguna
// ref, y el cambio sigue sin guardar. Que pasa al guardar otro con el
// worktree en su rama no se decide en esta ronda y no se fija.
//
// Reusa los fixtures de las rondas 4 y 5 (crR4PATH, crR5ErrorDeRama, ...).
// Ningun CLI de IA: el PATH tiene solo git y sh. Escritos desde el contrato,
// las firmas publicas y lo que el refutador observo, sin leer la
// implementacion.
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

const (
	crR6Otro          = "otro"
	crR6OtroItemRel   = ".hoom/items/" + crR6Otro + ".yaml"
	crR6OtroSpecRel   = ".hoom/specs/" + crR6Otro + ".md"
	crR6OtroSpec      = "# Spec: otro\n\nla otra tarea del proyecto\n"
	crR6OtroSpecSucio = crR6OtroSpec + "\nun cambio de otro sin guardar\n"
	crR6Hallazgos     = "hallazgos 20261112T040826_65fbb7 y 20261021T145451_68644c"
)

// crR6Tarea arma el proyecto con dos tareas: los items de precios y de otro y
// el spec de otro commiteados en el principal, y la tarea precios iniciada
// (worktree en .hoom/worktrees/precios, rama hoom/precios, su spec
// commiteado en la rama). .hoom/ ignora worktrees/ y cache/. Nada sin
// guardar en ningun arbol.
func crR6Tarea(t *testing.T) (root, wt string) {
	t.Helper()
	root = icRepo(t)
	icEscribir(t, root, ".hoom/.gitignore", crR4Ignorar)
	icEscribir(t, root, crR4ItemRel, icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, crR6OtroItemRel, icItemYAML("Otro", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, crR6OtroSpecRel, crR6OtroSpec)
	crCommitearItems(t, root)
	wt = filepath.Join(root, ".hoom", "worktrees", crR4Slug)
	icGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+crR4Slug, wt, "main")
	icEscribir(t, wt, crR4SpecRel, crR4Spec)
	crCommitearItems(t, wt)
	for _, dir := range []string{root, wt} {
		if st := crR3Git(t, dir, "status", "--porcelain", "--untracked-files=all"); strings.TrimSpace(st) != "" {
			t.Fatalf("fixture: %s arranca sin nada sin guardar: %q", dir, st)
		}
	}
	return root, wt
}

func crR6OtroEnWorktree(t *testing.T, wt string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wt, filepath.FromSlash(crR6OtroSpecRel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// crR6Foto junta lo que un save rechazado no puede tocar: todas las ramas, el
// registro de worktrees, y HEAD (con su rama), indice y estado del principal
// y del worktree, mas el spec de otro en el worktree.
func crR6Foto(t *testing.T, root, wt string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("ramas:\n" + crR3Git(t, root, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"))
	b.WriteString("worktrees:\n" + crR3Git(t, root, "worktree", "list", "--porcelain"))
	for _, d := range []string{root, wt} {
		b.WriteString(d + "\nHEAD " + strings.TrimSpace(crR3Git(t, d, "rev-parse", "--symbolic-full-name", "HEAD")) + " " + crHead(t, d) + "\n")
		b.WriteString("indice:\n" + crR3Git(t, d, "ls-files", "-s"))
		b.WriteString("estado:\n" + crR3Git(t, d, "status", "--porcelain", "--untracked-files=all"))
	}
	b.WriteString("spec de otro en el worktree:\n" + crR6OtroEnWorktree(t, wt))
	return b.String()
}

// crR6Unsynced es el unsynced de la tarjeta otro vista desde wt, sin
// exigirle nada (nil si no se deriva).
func crR6Unsynced(wt string) []string {
	c, err := boardcmd.CardFor(wt, "main", "high", crR6Otro, time.Now().UTC())
	if err != nil {
		return nil
	}
	return append([]string{}, c.Unsynced...)
}

// crR6FueraDeSuRama arma el caso: el spec de otro cambiado en el worktree de
// precios y visto en su rama (antes: el unsynced de otro ahi, sin exigirle
// nada), y despues el worktree dejado fuera de hoom/precios por v, con el
// spec de otro cambiado otra vez (para cambiar de rama vuelve un momento a
// lo guardado).
func crR6FueraDeSuRama(t *testing.T, ca, wt string, v worktreetest.RamaAjena) (antes []string) {
	t.Helper()
	icEscribir(t, wt, crR6OtroSpecRel, crR6OtroSpecSucio)
	antes = crR6Unsynced(wt)
	icEscribir(t, wt, crR6OtroSpecRel, crR6OtroSpec)
	v.Armar(t, wt, crR4Slug)
	icEscribir(t, wt, crR6OtroSpecRel, crR6OtroSpecSucio)
	if rama := strings.TrimSpace(crR3Git(t, wt, "rev-parse", "--symbolic-full-name", "HEAD")); rama == "refs/heads/hoom/"+crR4Slug {
		t.Fatalf("%s: fixture: el worktree queda fuera de hoom/%s", ca, crR4Slug)
	}
	if st := crR3Git(t, wt, "status", "--porcelain", "--", crR6OtroSpecRel); strings.TrimSpace(st) == "" {
		t.Fatalf("%s: fixture: el spec de otro queda cambiado sin guardar en el worktree", ca)
	}
	return antes
}

// crR6LoMostrado es el expect que manda el Studio para otro: el unsynced de
// otro visto desde el worktree ahora; si no muestra nada, el que mostraba en
// su rama; y si tampoco, el spec de otro (lo que el refutador vio).
func crR6LoMostrado(wt string, antes []string) []string {
	if ahora := crR6Unsynced(wt); len(ahora) > 0 {
		return ahora
	}
	if len(antes) > 0 {
		return antes
	}
	return []string{crR6OtroSpecRel}
}

// crR6DiceLaRama exige que msg sea el error de rama de la tarea precios:
// nombra hoom/precios y la rama, trae el texto de runcmd.TaskDir(principal,
// "precios"), no dice "no existe" ni es ErrChanged.
func crR6DiceLaRama(t *testing.T, ca, msg, esperado string) {
	t.Helper()
	if !strings.Contains(msg, "hoom/"+crR4Slug) || !strings.Contains(msg, "rama") || strings.Contains(msg, "no existe") {
		t.Fatalf("%s: el error dice que el worktree de precios no esta en su rama hoom/%s (y no \"no existe\"): %q", ca, crR4Slug, msg)
	}
	if strings.Contains(msg, ErrChanged) {
		t.Fatalf("%s: la negativa es por la rama, no ErrChanged: %q", ca, msg)
	}
	if !strings.Contains(msg, esperado) {
		t.Fatalf("%s: el error es el de rama de la tarea precios (runcmd.TaskDir):\nquiero: %q\ndijo:   %q", ca, esperado, msg)
	}
}

// hallazgos 20261112T040826_65fbb7 y 20261021T145451_68644c: con el worktree
// de precios fuera de hoom/precios (cada variante de
// worktreetest.RamasAjenas) y el spec de otro cambiado alli,
// Save(<worktree de precios>, ..., "otro", expect) con lo que la tarjeta otro
// mostro se niega con el error de rama de la tarea precios: envuelve
// runcmd.ErrOtraRama y trae el texto de runcmd.TaskDir(principal,
// "precios"). No hay commit en ningun arbol ni se mueve ninguna ref, el
// indice y el estado de los dos arboles quedan igual y el spec de otro sigue
// cambiado.
func TestHallazgo_65fbb7_68644c_SaveOtraTarjetaDesdeElWorktreeFueraDeSuRamaEsErrOtraRama(t *testing.T) {
	crR4PATH(t)
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			ca := crR6Hallazgos + ", Save de otro, " + v.Nombre
			root, wt := crR6Tarea(t)
			antes := crR6FueraDeSuRama(t, ca, wt, v)
			expect := crR6LoMostrado(wt, antes)
			esperado := crR5ErrorDeRama(t, ca, root)
			foto := crR6Foto(t, root, wt)

			res, err := Save(wt, "main", "high", crR6Otro, expect)
			if err == nil {
				t.Fatalf("%s: con el worktree de precios fuera de su rama, Save de %s con %v se niega con el error de rama de precios: %+v", ca, crR6Otro, expect, res)
			}
			if !errors.Is(err, runcmd.ErrOtraRama) {
				t.Fatalf("%s: el error de Save envuelve runcmd.ErrOtraRama: %v", ca, err)
			}
			crR6DiceLaRama(t, ca, err.Error(), esperado)
			if len(res.Commits) != 0 {
				t.Fatalf("%s: la negativa no informa commits: %+v", ca, res.Commits)
			}
			if despues := crR6Foto(t, root, wt); despues != foto {
				t.Fatalf("%s: la negativa no commitea en ningun arbol ni mueve ninguna ref:\nantes:\n%s\ndespues:\n%s", ca, foto, despues)
			}
			if crR6OtroEnWorktree(t, wt) != crR6OtroSpecSucio {
				t.Fatalf("%s: el spec de otro sigue cambiado en el worktree", ca)
			}
		})
	}
}

// hallazgos 20261112T040826_65fbb7 y 20261021T145451_68644c: `hoom item
// save otro` sin rutas corrido desde el worktree de precios fuera de
// hoom/precios (cada variante de worktreetest.RamasAjenas), con el spec de
// otro cambiado alli, se niega: Execute devuelve error (exit distinto de 0)
// con el error de rama de la tarea precios (el texto de runcmd.TaskDir), y no
// "nada sin guardar" con exit 0. No hay commit en ningun arbol ni se mueve
// ninguna ref, y el spec de otro sigue cambiado.
func TestHallazgo_65fbb7_68644c_ItemSaveOtroSinRutasDesdeElWorktreeFueraDeSuRamaSeNiega(t *testing.T) {
	crR4PATH(t)
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			ca := crR6Hallazgos + ", hoom item save otro sin rutas, " + v.Nombre
			root, wt := crR6Tarea(t)
			crR6FueraDeSuRama(t, ca, wt, v)
			esperado := crR5ErrorDeRama(t, ca, root)
			foto := crR6Foto(t, root, wt)

			out, errb, err := cbExec(t, wt, "save", crR6Otro)
			if err == nil {
				t.Fatalf("%s: hoom item save %s desde el worktree de precios fuera de su rama se niega (exit distinto de 0) con el error de rama de precios:\n%s%s", ca, crR6Otro, out, errb)
			}
			crR6DiceLaRama(t, ca, err.Error()+"\n"+errb, esperado)
			if despues := crR6Foto(t, root, wt); despues != foto {
				t.Fatalf("%s: la negativa no commitea en ningun arbol ni mueve ninguna ref:\nantes:\n%s\ndespues:\n%s", ca, foto, despues)
			}
			if crR6OtroEnWorktree(t, wt) != crR6OtroSpecSucio {
				t.Fatalf("%s: el spec de otro sigue cambiado en el worktree", ca)
			}
		})
	}
}
