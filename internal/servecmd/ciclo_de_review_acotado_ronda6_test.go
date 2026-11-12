// Tests de regresion de la ronda 6 de ciclo-de-review-acotado: Guardar OTRA
// tarjeta desde el Studio servido en el worktree de una tarea que quedo fuera
// de su rama (hallazgo 20261112T040826_65fbb7; contrato del hallazgo
// 20261021T145451_68644c y CA-345). La ronda 5 cubrio guardar la tarjeta del
// propio worktree; aca el proyecto tiene dos tareas, precios (con worktree) y
// otro (item y spec commiteados en el principal), y en el worktree de precios,
// fuera de hoom/precios (worktreetest.RamasAjenas), esta cambiado el spec de
// otro. POST /api/board/otro/save con lo que el tablero mostro para otro es
// 409 con el error de rama del worktree donde corre el Studio, el de la
// tarea precios (el texto de runcmd.TaskDir(principal, "precios")), sin
// commit en ningun arbol ni movimiento de ninguna ref; el cambio sigue sin
// guardar. Guarda: de vuelta en hoom/precios, guardar la tarjeta precios
// desde ahi da 200. Que pasa al guardar otro con el worktree en su rama no
// se decide en esta ronda y no se fija.
//
// Reusa los fixtures de las rondas 4 y 5 (cr4Guardable, cr5ErrorDeRama,
// cr5DiceLaRama, ...). Ningun CLI real: el PATH tiene solo git y sh
// (atPATH). Escritos desde el contrato, las firmas publicas y lo que el
// refutador observo, sin leer la implementacion.
package servecmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/worktreetest"
)

const (
	cr6Otro          = "otro"
	cr6OtroSpecRel   = ".hoom/specs/" + cr6Otro + ".md"
	cr6OtroSpec      = "# Spec: otro\n\nla otra tarea del proyecto\n"
	cr6OtroSpecSucio = cr6OtroSpec + "\nun cambio de otro sin guardar\n"
	cr6Hallazgos     = "hallazgos 20261112T040826_65fbb7 y 20261021T145451_68644c (CA-345)"
)

// cr6Tarea arma el proyecto (tbProyecto) con dos tareas: los items de precios
// y de otro y el spec de otro commiteados en el principal, y la tarea precios
// iniciada (worktree en .hoom/worktrees/precios, rama hoom/precios, su spec
// commiteado en la rama). Nada sin guardar en ningun arbol.
func cr6Tarea(t *testing.T) (dir, wt string) {
	t.Helper()
	dir = tbProyecto(t)
	tbItem(t, dir, cr4Slug, "")
	tbItem(t, dir, cr6Otro, "")
	tbEscribir(t, dir, cr6OtroSpecRel, cr6OtroSpec)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "items")
	wt = filepath.Join(dir, ".hoom", "worktrees", cr4Slug)
	gitRun(t, dir, "worktree", "add", "-q", "-b", "hoom/"+cr4Slug, wt, "main")
	tbEscribir(t, wt, cr4SpecRel, cr4Spec)
	gitRun(t, wt, "add", "-A")
	gitRun(t, wt, "commit", "-q", "-m", "spec")
	if st := atGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("fixture: el worktree arranca sin nada sin guardar: %q", st)
	}
	if st := atGit(t, dir, "status", "--porcelain", "--untracked-files=all", "--", ".hoom/items", ".hoom/specs"); st != "" {
		t.Fatalf("fixture: el principal arranca con los items y el spec de otro guardados: %q", st)
	}
	return dir, wt
}

func cr6OtroEnWorktree(t *testing.T, wt string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wt, filepath.FromSlash(cr6OtroSpecRel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// cr6Foto junta lo que un save rechazado no puede tocar: todas las ramas, el
// registro de worktrees, y HEAD (con su rama), indice y estado del principal
// y del worktree, mas el spec de otro en el worktree.
func cr6Foto(t *testing.T, dir, wt string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("ramas:\n" + atGit(t, dir, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads") + "\n")
	b.WriteString("worktrees:\n" + atGit(t, dir, "worktree", "list", "--porcelain") + "\n")
	for _, d := range []string{dir, wt} {
		b.WriteString(d + "\nHEAD " + atGit(t, d, "rev-parse", "--symbolic-full-name", "HEAD") + " " + atGit(t, d, "rev-parse", "HEAD") + "\n")
		b.WriteString("indice:\n" + atGit(t, d, "ls-files", "-s") + "\n")
		b.WriteString("estado:\n" + atGit(t, d, "status", "--porcelain", "--untracked-files=all") + "\n")
	}
	b.WriteString("spec de otro en el worktree:\n" + cr6OtroEnWorktree(t, wt))
	return b.String()
}

// cr6Unsynced es el unsynced de la tarjeta slug en GET /api/board/{slug} del
// tablero que sirve s, sin exigirle nada (nil si no responde 200).
func cr6Unsynced(t *testing.T, s *Server, slug string) []string {
	t.Helper()
	rec := tbGET(t, s, "/api/board/"+slug)
	var d struct {
		Card cr4Card `json:"card"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &d) != nil {
		return nil
	}
	return append([]string{}, d.Card.Unsynced...)
}

// cr6FueraDeSuRama arma el caso: el spec de otro cambiado en el worktree de
// precios y visto en su rama (antes: lo que el tablero mostraba ahi para
// otro, sin exigirle nada), y despues el worktree dejado fuera de
// hoom/precios por v con el spec de otro cambiado otra vez (para cambiar de
// rama vuelve un momento a lo guardado).
func cr6FueraDeSuRama(t *testing.T, ca, wt string, s *Server, v worktreetest.RamaAjena) (antes []string) {
	t.Helper()
	tbEscribir(t, wt, cr6OtroSpecRel, cr6OtroSpecSucio)
	antes = cr6Unsynced(t, s, cr6Otro)
	tbEscribir(t, wt, cr6OtroSpecRel, cr6OtroSpec)
	v.Armar(t, wt, cr4Slug)
	tbEscribir(t, wt, cr6OtroSpecRel, cr6OtroSpecSucio)
	if rama := atGit(t, wt, "rev-parse", "--symbolic-full-name", "HEAD"); rama == "refs/heads/hoom/"+cr4Slug {
		t.Fatalf("%s: fixture: el worktree queda fuera de hoom/%s", ca, cr4Slug)
	}
	if st := atGit(t, wt, "status", "--porcelain", "--", cr6OtroSpecRel); st == "" {
		t.Fatalf("%s: fixture: el spec de otro queda cambiado sin guardar en el worktree", ca)
	}
	return antes
}

// cr6LoMostrado es lo que se manda a save para otro: lo que el tablero del
// worktree muestra ahora; si no muestra nada, lo que mostraba en su rama; y
// si tampoco, el spec de otro (lo que el refutador vio en unsynced).
func cr6LoMostrado(t *testing.T, s *Server, antes []string) []string {
	t.Helper()
	if ahora := cr6Unsynced(t, s, cr6Otro); len(ahora) > 0 {
		return ahora
	}
	if len(antes) > 0 {
		return antes
	}
	return []string{cr6OtroSpecRel}
}

// hallazgos 20261112T040826_65fbb7 y 20261021T145451_68644c (CA-345): con
// el Studio servido desde el worktree de precios y ese worktree fuera de
// hoom/precios (cada variante de worktreetest.RamasAjenas) con el spec de
// otro cambiado alli, POST /api/board/otro/save con lo que el tablero mostro
// para otro es 409 con el error de rama de la tarea precios (el de
// runcmd.TaskDir(principal, "precios"), que nombra hoom/precios). No se crea
// ningun commit en ningun arbol ni se mueve ninguna ref (ni la equivocada,
// ni hoom/precios, ni main), el indice y el estado de los dos arboles quedan
// igual, y el spec de otro sigue cambiado.
func TestHallazgo_65fbb7_68644c_GuardarOtraTarjetaDesdeElWorktreeFueraDeSuRamaDa409(t *testing.T) {
	atPATH(t, nil)
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			ca := cr6Hallazgos + ", tarjeta otro, " + v.Nombre
			dir, wt := cr6Tarea(t)
			s := newServer(t, wt)
			antes := cr6FueraDeSuRama(t, ca, wt, s, v)
			paths := cr6LoMostrado(t, s, antes)
			esperado := cr5ErrorDeRama(t, ca, dir)
			foto := cr6Foto(t, dir, wt)

			rec := atPedir(t, ca, s, http.MethodPost, "/api/board/"+cr6Otro+"/save", s.Token(), atCuerpoPaths(paths))
			if rec.Code != http.StatusConflict {
				t.Fatalf("%s: con el worktree de precios fuera de su rama, POST /api/board/%s/save con %v es 409 con el error de rama de precios, fue %d: %s", ca, cr6Otro, paths, rec.Code, rec.Body.String())
			}
			cr5DiceLaRama(t, ca, tbErrorJSON(t, ca, rec, http.StatusConflict), esperado)
			if despues := cr6Foto(t, dir, wt); despues != foto {
				t.Fatalf("%s: la negativa no commitea en ningun arbol ni mueve ninguna ref:\nantes:\n%s\ndespues:\n%s", ca, foto, despues)
			}
			if cr6OtroEnWorktree(t, wt) != cr6OtroSpecSucio {
				t.Fatalf("%s: el spec de otro sigue cambiado en el worktree", ca)
			}
		})
	}
}

// CA-345 (guarda de 65fbb7 / 68644c): en el proyecto con dos tareas, el
// worktree de precios paso por otra rama con su spec cambiado y volvio a
// hoom/precios: guardar la tarjeta precios desde el Studio de ese worktree
// sigue dando 200, con exactamente un commit, en hoom/precios, solo con el
// spec de precios. El principal no gana commit, la rama por la que paso no
// se mueve y el spec de otro no se toca. No fija nada de guardar la tarjeta
// otro desde aca (fuera de esta ronda).
func TestHallazgo_65fbb7_68644c_DeVueltaEnSuRamaGuardarPreciosDa200(t *testing.T) {
	atPATH(t, nil)
	ca := cr6Hallazgos + ", guarda: tarjeta precios de vuelta en hoom/precios"
	dir, wt := cr6Tarea(t)
	s := newServer(t, wt)
	cr4EnsuciarSpec(t, wt)
	gitRun(t, wt, "checkout", "-q", "-b", "otra")
	worktreetest.VolverASuRama(t, wt, cr4Slug)
	headP, headW := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")
	otra := atGit(t, dir, "rev-parse", "refs/heads/otra")

	paths := cr4Guardable(t, ca, s)
	rec := cr4Guardar(t, ca, s, paths)
	cr4Resultado(t, ca, rec, paths, atGit(t, wt, "rev-parse", "HEAD"))
	cr4WorktreeGuardado(t, ca, wt, headW, cr4SpecRel)
	if atGit(t, dir, "rev-parse", "HEAD") != headP {
		t.Fatalf("%s: el principal no gana commit", ca)
	}
	if got := atGit(t, dir, "rev-parse", "refs/heads/otra"); got != otra {
		t.Fatalf("%s: la rama otra no se mueve: %s -> %s", ca, otra, got)
	}
	if cr6OtroEnWorktree(t, wt) != cr6OtroSpec {
		t.Fatalf("%s: el spec de otro no se toca", ca)
	}
}
