// Tests de regresion de la ronda 5 de ciclo-de-review-acotado: Guardar desde
// el Studio servido en el worktree de una tarea que quedo fuera de su rama
// (hallazgo 20261112T034738_13bba6; contrato del hallazgo
// 20261021T145451_68644c y CA-345). Los comandos que guardan en una tarea se
// niegan si su worktree no esta en hoom/<slug>: runcmd.TaskDir es la puerta
// y su error envuelve runcmd.ErrOtraRama. Con el worktree en otra rama, con
// HEAD separado, en main o en hoom/<slug>-viejo (worktreetest.RamasAjenas) y
// el spec cambiado, POST /api/board/{slug}/save con lo que el tablero mostro
// es 409 con el mismo error de rama que da runcmd.TaskDir, y no se crea
// ningun commit en ningun arbol ni en ninguna rama; el spec sigue cambiado.
// Guarda: de vuelta en hoom/<slug>, el ida y vuelta de la ronda 4 da 200.
//
// Reusa los fixtures de la ronda 4 (cr4Tarea, cr4Guardable, cr4Guardar, ...).
// Ningun CLI real: el PATH tiene solo git y sh (atPATH). Escritos desde el
// contrato, las firmas publicas y los tests existentes de la rama, sin leer
// la implementacion.
package servecmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

const cr5Hallazgos = "hallazgos 20261112T034738_13bba6 y 20261021T145451_68644c (CA-345)"

func cr5Spec(t *testing.T, wt string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wt, filepath.FromSlash(cr4SpecRel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// cr5Foto junta lo que un save rechazado no puede tocar: todas las ramas, el
// registro de worktrees, y HEAD (con su rama), indice y estado del principal
// y del worktree, mas el spec del worktree.
func cr5Foto(t *testing.T, dir, wt string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("ramas:\n" + atGit(t, dir, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads") + "\n")
	b.WriteString("worktrees:\n" + atGit(t, dir, "worktree", "list", "--porcelain") + "\n")
	for _, d := range []string{dir, wt} {
		b.WriteString(d + "\nHEAD " + atGit(t, d, "rev-parse", "--symbolic-full-name", "HEAD") + " " + atGit(t, d, "rev-parse", "HEAD") + "\n")
		b.WriteString("indice:\n" + atGit(t, d, "ls-files", "-s") + "\n")
		b.WriteString("estado:\n" + atGit(t, d, "status", "--porcelain", "--untracked-files=all") + "\n")
	}
	b.WriteString("spec del worktree:\n" + cr5Spec(t, wt))
	return b.String()
}

// cr5FueraDeSuRama arma el caso: la tarea iniciada con el item en el
// principal (cr4Tarea), el Studio servido desde el worktree, el spec
// cambiado y visto en su rama (antes: lo que el tablero mostraba ahi), y
// despues el worktree dejado fuera de hoom/precios por v, con el spec
// cambiado otra vez (para cambiar de rama vuelve un momento a lo guardado).
func cr5FueraDeSuRama(t *testing.T, ca string, v worktreetest.RamaAjena) (dir, wt string, s *Server, antes []string) {
	t.Helper()
	dir, wt = cr4Tarea(t, true)
	s = newServer(t, wt)
	cr4EnsuciarSpec(t, wt)
	antes = cr4Guardable(t, ca+" (fixture, en su rama)", s)
	tbEscribir(t, wt, cr4SpecRel, cr4Spec)
	v.Armar(t, wt, cr4Slug)
	cr4EnsuciarSpec(t, wt)
	if rama := atGit(t, wt, "rev-parse", "--symbolic-full-name", "HEAD"); rama == "refs/heads/hoom/"+cr4Slug {
		t.Fatalf("%s: fixture: el worktree queda fuera de hoom/%s", ca, cr4Slug)
	}
	return dir, wt, s, antes
}

// cr5LoMostrado es lo que se manda a save: el unsynced que el tablero del
// worktree muestra ahora, o, si no muestra nada (o no responde), el que
// mostraba en su rama.
func cr5LoMostrado(t *testing.T, s *Server, antes []string) []string {
	t.Helper()
	rec := tbGET(t, s, "/api/board/"+cr4Slug)
	var d struct {
		Card cr4Card `json:"card"`
	}
	if rec.Code == http.StatusOK && json.Unmarshal(rec.Body.Bytes(), &d) == nil && len(d.Card.Unsynced) > 0 {
		return append([]string{}, d.Card.Unsynced...)
	}
	return antes
}

// cr5ErrorDeRama es el error de rama que dan los comandos de la tarea: el de
// runcmd.TaskDir desde el principal, que envuelve runcmd.ErrOtraRama.
func cr5ErrorDeRama(t *testing.T, ca, dir string) string {
	t.Helper()
	got, err := runcmd.TaskDir(dir, cr4Slug)
	if !errors.Is(err, runcmd.ErrOtraRama) {
		t.Fatalf("%s: fixture: runcmd.TaskDir dice que el worktree esta fuera de su rama (ErrOtraRama): %q %v", ca, got, err)
	}
	return err.Error()
}

// cr5DiceLaRama exige que el mensaje sea el error de rama de los comandos de
// la tarea: nombra hoom/precios y la rama, trae el texto de runcmd.TaskDir,
// no dice "no existe" ni es la negativa de rutas que cambiaron.
func cr5DiceLaRama(t *testing.T, ca, msg, esperado string) {
	t.Helper()
	if !strings.Contains(msg, "hoom/"+cr4Slug) || !strings.Contains(msg, "rama") || strings.Contains(msg, "no existe") {
		t.Fatalf("%s: el error dice que el worktree no esta en su rama hoom/%s (y no \"no existe\"): %q", ca, cr4Slug, msg)
	}
	if strings.Contains(msg, atCambiaron) {
		t.Fatalf("%s: la negativa es por la rama, no por rutas que cambiaron: %q", ca, msg)
	}
	if !strings.Contains(msg, esperado) {
		t.Fatalf("%s: el error es el mismo que dan los comandos de la tarea (runcmd.TaskDir):\nquiero: %q\ndijo:   %q", ca, esperado, msg)
	}
}

// hallazgos 20261112T034738_13bba6 y 20261021T145451_68644c (CA-345): con
// el Studio servido desde el worktree de la tarea y ese worktree fuera de
// hoom/precios (cada variante de worktreetest.RamasAjenas: HEAD separado,
// otra rama, main, hoom/precios-viejo) con el spec cambiado, POST
// /api/board/precios/save con lo que el tablero mostro es 409 con el error
// de rama de runcmd.TaskDir. No se crea ningun commit en ningun arbol ni se
// mueve ninguna rama (ni la equivocada, ni hoom/precios, ni main), el indice
// y el estado de los dos arboles quedan igual, y el spec sigue cambiado.
func TestHallazgo_13bba6_68644c_GuardarDesdeElWorktreeFueraDeSuRamaDa409(t *testing.T) {
	atPATH(t, nil)
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			ca := cr5Hallazgos + ", " + v.Nombre
			dir, wt, s, antes := cr5FueraDeSuRama(t, ca, v)
			paths := cr5LoMostrado(t, s, antes)
			esperado := cr5ErrorDeRama(t, ca, dir)
			foto := cr5Foto(t, dir, wt)

			rec := cr4Guardar(t, ca, s, paths)
			if rec.Code != http.StatusConflict {
				t.Fatalf("%s: con el worktree fuera de su rama, POST save con %v es 409 con el error de rama, fue %d: %s", ca, paths, rec.Code, rec.Body.String())
			}
			cr5DiceLaRama(t, ca, tbErrorJSON(t, ca, rec, http.StatusConflict), esperado)
			if despues := cr5Foto(t, dir, wt); despues != foto {
				t.Fatalf("%s: la negativa no commitea en ningun arbol ni mueve ninguna rama:\nantes:\n%s\ndespues:\n%s", ca, foto, despues)
			}
			if cr5Spec(t, wt) != cr4SpecSucio {
				t.Fatalf("%s: el spec del worktree sigue cambiado", ca)
			}
		})
	}
}

// CA-345 (guarda de 13bba6 / 68644c): el worktree paso por otra rama con el
// spec cambiado y volvio a hoom/precios: el ida y vuelta de la ronda 4 desde
// el Studio del worktree sigue dando 200, con exactamente un commit, en
// hoom/precios, solo con el spec. El principal no gana commit y la rama por
// la que paso no se mueve.
func TestHallazgo_13bba6_68644c_DeVueltaEnSuRamaGuardarDa200(t *testing.T) {
	atPATH(t, nil)
	ca := cr5Hallazgos + ", guarda de vuelta en hoom/precios"
	dir, wt := cr4Tarea(t, true)
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
}
