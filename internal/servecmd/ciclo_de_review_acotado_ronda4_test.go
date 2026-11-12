// Tests de regresion de la ronda 4 de ciclo-de-review-acotado: Guardar desde
// el tablero del Studio servido en el worktree de la tarea (CA-345;
// hallazgos 20261112T023834_d26397 y 20261112T024355_f4d55b). Contrato:
// POST /api/board/{slug}/save con las rutas de `unsynced` responde 200 con
// el SaveResult; con otras rutas, 409 con `los cambios de la tarjeta
// cambiaron desde que los viste: revisalos de nuevo` y sin commit.
//
// Decision de Henry (ronda 4b): con rutas esperadas (el Studio), guardar
// desde un arbol guarda exactamente lo que mostro el tablero de ESE arbol, y
// en ese arbol. El item del checkout principal cambiado con `hoom item set`
// desde el worktree no lo guarda el Studio del worktree: queda para `hoom
// item save <slug>` sin rutas, que desde el worktree lo commitea en el
// principal (ronda 3). El tablero no cambia.
//
// Las aserciones no fijan el espacio de rutas de `unsynced` (ni su prefijo),
// ni el source/dir de la evidencia, ni el dir de los commits: fijan la
// propiedad (lo mostrado, mandado a save, da 200) y el estado final en git,
// y reconocen cada commit por su sha. Los casos con SOLO el riesgo cambiado
// van en una funcion terminada en _SoloRiesgo. Ningun CLI real: el PATH
// tiene solo git y sh (atPATH). Escritos desde el spec, las firmas publicas
// y el texto de los hallazgos, sin leer la implementacion.
package servecmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/itemcmd"
)

const (
	cr4Slug      = "precios"
	cr4ItemRel   = ".hoom/items/" + cr4Slug + ".yaml"
	cr4SpecRel   = ".hoom/specs/" + cr4Slug + ".md"
	cr4Spec      = "# Spec: precios\n\nlo que el arquitecto escribio\n"
	cr4SpecSucio = cr4Spec + "\nuna linea nueva sin guardar\n"
	cr4Hallazgos = "hallazgos 20261112T023834_d26397 y 20261112T024355_f4d55b (CA-345)"
)

// cr4Accion y cr4Card son lo que la pagina lee de una tarjeta para guardar.
type cr4Accion struct {
	ID      string   `json:"id"`
	Enabled bool     `json:"enabled"`
	Why     string   `json:"why"`
	Paths   []string `json:"paths"`
}

type cr4Card struct {
	Slug     string      `json:"slug"`
	Unsynced []string    `json:"unsynced"`
	Actions  []cr4Accion `json:"actions"`
}

// cr4Tarea arma el proyecto (tbProyecto) con la tarea iniciada (worktree en
// .hoom/worktrees/precios, rama hoom/precios) y nada sin guardar: el item
// commiteado en el checkout principal (enPrincipal) o solo en la rama de la
// tarea, y el spec commiteado en la rama.
func cr4Tarea(t *testing.T, enPrincipal bool) (dir, wt string) {
	t.Helper()
	dir = tbProyecto(t)
	if enPrincipal {
		tbItem(t, dir, cr4Slug, "")
		gitRun(t, dir, "add", "-A")
		gitRun(t, dir, "commit", "-q", "-m", "item")
	}
	wt = filepath.Join(dir, ".hoom", "worktrees", cr4Slug)
	gitRun(t, dir, "worktree", "add", "-q", "-b", "hoom/"+cr4Slug, wt, "main")
	if !enPrincipal {
		tbItem(t, wt, cr4Slug, "")
	}
	tbEscribir(t, wt, cr4SpecRel, cr4Spec)
	gitRun(t, wt, "add", "-A")
	gitRun(t, wt, "commit", "-q", "-m", "spec")
	if st := atGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("fixture: el worktree arranca sin nada sin guardar: %q", st)
	}
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(cr4ItemRel)))
	if enPrincipal {
		if err != nil {
			t.Fatalf("fixture: el principal tiene el item: %v", err)
		}
		if st := atGit(t, dir, "status", "--porcelain", "--", cr4ItemRel); st != "" {
			t.Fatalf("fixture: el item del principal arranca guardado: %q", st)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("fixture: el principal no tiene el item: %v", err)
	}
	return dir, wt
}

func cr4Leer(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(cr4ItemRel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// cr4Riesgo es `hoom item set precios --riesgo alto` corrido desde el
// worktree: escribe el item del principal cuando lo tiene, si no el de la
// tarea, y no commitea.
func cr4Riesgo(t *testing.T, dir, wt string, enPrincipal bool) {
	t.Helper()
	if _, err := itemcmd.Set(wt, "main", tbBlockOn, cr4Slug, "alto", ""); err != nil {
		t.Fatalf("fixture: hoom item set desde el worktree: %v", err)
	}
	donde := wt
	if enPrincipal {
		donde = dir
	}
	if !strings.Contains(cr4Leer(t, donde), "riesgo: alto") {
		t.Fatalf("fixture: el item de %s queda con riesgo: alto:\n%s", donde, cr4Leer(t, donde))
	}
	if st := atGit(t, donde, "status", "--porcelain", "--", cr4ItemRel); st == "" {
		t.Fatalf("fixture: el item de %s queda sin guardar", donde)
	}
}

func cr4EnsuciarSpec(t *testing.T, wt string) {
	t.Helper()
	tbEscribir(t, wt, cr4SpecRel, cr4SpecSucio)
}

// cr4Detalle es la tarjeta de GET /api/board/{slug} en el tablero que sirve s.
func cr4Detalle(t *testing.T, ca string, s *Server) cr4Card {
	t.Helper()
	rec := tbGET(t, s, "/api/board/"+cr4Slug)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: GET /api/board/%s responde 200: %d %s", ca, cr4Slug, rec.Code, rec.Body.String())
	}
	var d struct {
		Card cr4Card `json:"card"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("%s: el detalle es JSON: %v", ca, err)
	}
	return d.Card
}

// cr4Vista es el unsynced de la tarjeta en el tablero que sirve s: el de
// GET /api/board, que tiene que ser el mismo de GET /api/board/{slug}. No le
// exige nada mas.
func cr4Vista(t *testing.T, ca string, s *Server) []string {
	t.Helper()
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: GET /api/board responde 200: %d %s", ca, rec.Code, rec.Body.String())
	}
	var b struct {
		Columns []struct {
			Cards []cr4Card `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("%s: GET /api/board es el tablero en JSON: %v", ca, err)
	}
	var enTablero *cr4Card
	for i := range b.Columns {
		for j := range b.Columns[i].Cards {
			if b.Columns[i].Cards[j].Slug == cr4Slug {
				c := b.Columns[i].Cards[j]
				enTablero = &c
			}
		}
	}
	if enTablero == nil {
		t.Fatalf("%s: el tablero trae la tarjeta %s: %s", ca, cr4Slug, rec.Body.String())
	}
	if d := cr4Detalle(t, ca, s); !atMismas(enTablero.Unsynced, d.Unsynced) {
		t.Fatalf("%s: el tablero y el detalle muestran lo mismo sin guardar: %v vs %v", ca, enTablero.Unsynced, d.Unsynced)
	}
	return append([]string{}, enTablero.Unsynced...)
}

// cr4Guardable es lo que el Studio manda al guardar cuando la tarjeta tiene
// algo de ese arbol que guardar: unsynced no vacio, y guardar habilitada con
// las rutas de unsynced.
func cr4Guardable(t *testing.T, ca string, s *Server) []string {
	t.Helper()
	unsynced := cr4Vista(t, ca, s)
	if len(unsynced) == 0 {
		t.Fatalf("%s: la tarjeta tiene cambios sin guardar en este arbol y el tablero los muestra en unsynced", ca)
	}
	acciones := cr4Detalle(t, ca, s).Actions
	for _, a := range acciones {
		if a.ID != boardcmd.ActGuardar {
			continue
		}
		if !a.Enabled || !atMismas(a.Paths, unsynced) {
			t.Fatalf("%s: guardar esta habilitada con las rutas de unsynced %v: %+v", ca, unsynced, a)
		}
		return append([]string{}, a.Paths...)
	}
	t.Fatalf("%s: con cambios sin guardar la tarjeta ofrece guardar: %+v", ca, acciones)
	return nil
}

func cr4Guardar(t *testing.T, ca string, s *Server, paths []string) *httptest.ResponseRecorder {
	t.Helper()
	return atPedir(t, ca, s, http.MethodPost, "/api/board/"+cr4Slug+"/save", s.Token(), atCuerpoPaths(paths))
}

// cr4Resultado exige 200 con el SaveResult de la tarjeta y exactamente un
// commit por cada sha de shas (los HEAD nuevos de los arboles que
// guardaron), reconocidos por su sha y no por su dir.
func cr4Resultado(t *testing.T, ca string, rec *httptest.ResponseRecorder, paths []string, shas ...string) itemcmd.SaveResult {
	t.Helper()
	if rec.Code == http.StatusConflict {
		t.Fatalf("%s: con las rutas que el tablero mostro (%v) no es 409: nada cambio entre ver y guardar: %s", ca, paths, rec.Body.String())
	}
	var res itemcmd.SaveResult
	claves := atOK(t, ca, rec, &res)
	atTieneClaves(t, ca, claves, "slug", "message", "commits")
	if res.Slug != cr4Slug || res.Message != itemcmd.SaveMessage(cr4Slug) {
		t.Fatalf("%s: el SaveResult nombra la tarjeta y el mensaje fijo: %s", ca, rec.Body.String())
	}
	if len(res.Commits) != len(shas) {
		t.Fatalf("%s: exactamente %d commit(s), uno por arbol que guardo: %s", ca, len(shas), rec.Body.String())
	}
	tiene := map[string]bool{}
	for _, c := range res.Commits {
		tiene[c.SHA] = true
	}
	for _, sha := range shas {
		if !tiene[sha] {
			t.Fatalf("%s: el SaveResult trae el commit %s (el HEAD nuevo de su arbol): %s", ca, sha, rec.Body.String())
		}
	}
	return res
}

// cr4Archivos son las rutas (relativas al arbol) que toco el commit HEAD de dir.
func cr4Archivos(t *testing.T, dir string) []string {
	t.Helper()
	return strings.Fields(atGit(t, dir, "show", "--name-only", "--format=", "HEAD"))
}

// cr4PrincipalGuardado exige un solo commit nuevo en el principal sobre
// headP, con el mensaje fijo y solo el item, que queda limpio y con riesgo:
// alto.
func cr4PrincipalGuardado(t *testing.T, ca, dir, headP string) {
	t.Helper()
	if atGit(t, dir, "rev-parse", "HEAD") == headP {
		t.Fatalf("%s: el principal gana un commit con el item", ca)
	}
	if p := atGit(t, dir, "rev-parse", "HEAD~1"); p != headP {
		t.Fatalf("%s: un solo commit nuevo en el principal: el padre es %s, no %s", ca, p, headP)
	}
	if got := cr4Archivos(t, dir); !atMismas(got, []string{cr4ItemRel}) {
		t.Fatalf("%s: el commit del principal lleva solo el item: %v", ca, got)
	}
	if st := atGit(t, dir, "status", "--porcelain", "--", cr4ItemRel); st != "" {
		t.Fatalf("%s: el item del principal queda limpio en git status: %q", ca, st)
	}
	if !strings.Contains(atGit(t, dir, "show", "HEAD:"+cr4ItemRel), "riesgo: alto") {
		t.Fatalf("%s: el commit nuevo del principal contiene riesgo: alto", ca)
	}
	if msg := atGit(t, dir, "log", "-1", "--format=%s"); msg != itemcmd.SaveMessage(cr4Slug) {
		t.Fatalf("%s: el commit del principal tiene el mensaje fijo: %q", ca, msg)
	}
	if autor := atGit(t, dir, "log", "-1", "--format=%an <%ae>"); autor != "hoom test <test@hoom.dev>" {
		t.Fatalf("%s: el commit del principal usa la identidad git configurada: %q", ca, autor)
	}
}

// cr4PrincipalSinGuardar exige que el principal no gane commit y que su item
// siga con riesgo: alto sin commitear: el Studio del worktree no lo guarda
// (decision de la ronda 4b).
func cr4PrincipalSinGuardar(t *testing.T, ca, dir, headP string) {
	t.Helper()
	if atGit(t, dir, "rev-parse", "HEAD") != headP {
		t.Fatalf("%s: el principal no gana commit: su item lo guarda `hoom item save` sin rutas, no el Studio del worktree", ca)
	}
	if !strings.Contains(cr4Leer(t, dir), "riesgo: alto") {
		t.Fatalf("%s: el item del principal sigue con riesgo: alto", ca)
	}
	if st := atGit(t, dir, "status", "--porcelain", "--", cr4ItemRel); st == "" {
		t.Fatalf("%s: el item del principal sigue sin commitear", ca)
	}
}

// cr4WorktreeGuardado exige un solo commit nuevo en el worktree, en la rama
// de la tarea, con el mensaje fijo y exactamente rutas (relativas al
// worktree); el worktree queda limpio y el spec commiteado con el cambio.
func cr4WorktreeGuardado(t *testing.T, ca, wt, headW string, rutas ...string) {
	t.Helper()
	nuevo := atGit(t, wt, "rev-parse", "HEAD")
	if nuevo == headW {
		t.Fatalf("%s: el worktree gana un commit con el spec", ca)
	}
	if p := atGit(t, wt, "rev-parse", "HEAD~1"); p != headW {
		t.Fatalf("%s: un solo commit nuevo en el worktree: el padre es %s, no %s", ca, p, headW)
	}
	if rama := atGit(t, wt, "rev-parse", "hoom/"+cr4Slug); rama != nuevo {
		t.Fatalf("%s: el commit queda en la rama de la tarea hoom/%s: %s vs %s", ca, cr4Slug, rama, nuevo)
	}
	if got := cr4Archivos(t, wt); !atMismas(got, rutas) {
		t.Fatalf("%s: el commit del worktree lleva exactamente %v: %v", ca, rutas, got)
	}
	if got := atGit(t, wt, "show", "HEAD:"+cr4SpecRel) + "\n"; got != cr4SpecSucio {
		t.Fatalf("%s: el spec commiteado en la rama es el cambiado en el worktree: %q", ca, got)
	}
	if st := atGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("%s: el worktree queda sin nada sin guardar: %q", ca, st)
	}
	if msg := atGit(t, wt, "log", "-1", "--format=%s"); msg != itemcmd.SaveMessage(cr4Slug) {
		t.Fatalf("%s: el commit del worktree tiene el mensaje fijo: %q", ca, msg)
	}
}

// cr4SinNada exige que la tarjeta no tenga nada sin guardar: en el tablero
// que sirve s y en la tarjeta derivada desde cada dir.
func cr4SinNada(t *testing.T, ca string, s *Server, dirs ...string) {
	t.Helper()
	if u := cr4Vista(t, ca, s); len(u) != 0 {
		t.Fatalf("%s: al final, el tablero no muestra nada sin guardar: %v", ca, u)
	}
	for _, d := range dirs {
		if u := atTarjeta(t, d, cr4Slug).Unsynced; len(u) != 0 {
			t.Fatalf("%s: al final, la tarjeta vista desde %s no tiene nada sin guardar: %v", ca, d, u)
		}
	}
}

// cr4ItemSave es `hoom item save precios` sin rutas corrido desde el
// worktree (Parse y Execute, como la CLI): commitea el item del principal
// (ronda 3), un commit con solo el item, sin commit nuevo en el worktree;
// despues los dos arboles quedan limpios y la tarjeta sin nada sin guardar.
func cr4ItemSave(t *testing.T, ca string, s *Server, dir, wt, headP string) {
	t.Helper()
	headW := atGit(t, wt, "rev-parse", "HEAD")
	req, err := itemcmd.Parse([]string{"save", cr4Slug})
	if err != nil {
		t.Fatalf("%s: Parse(save %s): %v", ca, cr4Slug, err)
	}
	var out, errb bytes.Buffer
	if err := itemcmd.Execute(wt, "main", tbBlockOn, req, &out, &errb); err != nil || !strings.Contains(out.String(), "tarjeta "+cr4Slug+" guardada") {
		t.Fatalf("%s: hoom item save %s sin rutas desde el worktree guarda: %v\n%s%s", ca, cr4Slug, err, out.String(), errb.String())
	}
	cr4PrincipalGuardado(t, ca, dir, headP)
	if atGit(t, wt, "rev-parse", "HEAD") != headW {
		t.Fatalf("%s: hoom item save no hace commit nuevo en el worktree, que no tenia nada sin guardar", ca)
	}
	if st := atGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("%s: el worktree queda limpio: %q", ca, st)
	}
	cr4SinNada(t, ca, s, wt, dir)
}

// cr4Niega exige 409 con la negativa del contrato y ningun commit ni cambio
// del indice en ningun arbol.
func cr4Niega(t *testing.T, ca string, s *Server, dir, wt string, paths []string) {
	t.Helper()
	antes := atEstadoGit(t, dir, wt)
	msg := tbErrorJSON(t, ca, cr4Guardar(t, ca, s, paths), http.StatusConflict)
	if !strings.Contains(msg, atCambiaron) {
		t.Fatalf("%s: el 409 dice %q, dijo %q", ca, atCambiaron, msg)
	}
	atMismoGit(t, ca, antes, dir, wt)
}

func cr4Contiene(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}

// cr4NoMostrados son conjuntos no vacios distintos, como conjunto, de lo que
// el tablero mostro: la ruta del item, otra ruta, y lo mostrado mas la ruta
// del item.
func cr4NoMostrados(vista []string) [][]string {
	out := [][]string{}
	for _, c := range [][]string{{cr4ItemRel}, {"otro.go"}} {
		if !atMismas(c, vista) {
			out = append(out, c)
		}
	}
	if len(vista) > 0 && !cr4Contiene(vista, cr4ItemRel) {
		out = append(out, append(append([]string{}, vista...), cr4ItemRel))
	}
	return out
}

// hallazgos 20261112T023834_d26397 y 20261112T024355_f4d55b (CA-345), con
// la decision de la ronda 4b: con el Studio servido desde el worktree, el
// riesgo puesto con `hoom item set` desde el worktree (item del principal) y
// el spec cambiado en el worktree, GET del tablero -> POST
// /api/board/precios/save con exactamente lo que mostro responde 200 con
// exactamente un commit, en hoom/precios, solo con el spec. El principal no
// gana commit: su item sigue con riesgo: alto sin commitear y un archivo
// ajeno de su indice sigue ahi. Despues `hoom item save precios` sin rutas
// desde el worktree commitea el item del principal (solo el item), y recien
// ahi los dos arboles quedan limpios y la tarjeta sin nada sin guardar.
func TestHallazgo_d26397_f4d55b_GuardarDesdeElTableroDelWorktree(t *testing.T) {
	atPATH(t, nil)
	ca := cr4Hallazgos + ", tablero servido desde el worktree"
	dir, wt := cr4Tarea(t, true)
	cr4Riesgo(t, dir, wt, true)
	cr4EnsuciarSpec(t, wt)
	tbEscribir(t, dir, "ajeno.txt", "esto es de la persona\n")
	gitRun(t, dir, "add", "ajeno.txt")
	headP, headW := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")
	copia := cr4Leer(t, wt)

	s := newServer(t, wt)
	paths := cr4Guardable(t, ca, s)
	rec := cr4Guardar(t, ca, s, paths)
	cr4Resultado(t, ca, rec, paths, atGit(t, wt, "rev-parse", "HEAD"))

	cr4WorktreeGuardado(t, ca, wt, headW, cr4SpecRel)
	cr4PrincipalSinGuardar(t, ca, dir, headP)
	if cr4Leer(t, wt) != copia {
		t.Fatalf("%s: la copia del item del worktree no se toca", ca)
	}
	if idx := atGit(t, dir, "diff", "--cached", "--name-only"); idx != "ajeno.txt" {
		t.Fatalf("%s: el archivo ajeno sigue en el indice del principal sin commitear: %q", ca, idx)
	}

	cr4ItemSave(t, ca+" (despues, hoom item save)", s, dir, wt, headP)
	if idx := atGit(t, dir, "diff", "--cached", "--name-only"); idx != "ajeno.txt" {
		t.Fatalf("%s: despues de hoom item save, el archivo ajeno sigue en el indice del principal sin commitear: %q", ca, idx)
	}
}

// CA-345 (d26397 / f4d55b, decision de la ronda 4b): el riesgo cambiado
// desde el worktree DESPUES de ver el tablero con el spec sucio no cambia lo
// que guarda el Studio del worktree. POST save con lo visto da 200 con un
// commit, solo el spec en hoom/precios, y el item del principal sigue sin
// commitear.
func TestHallazgo_d26397_f4d55b_RiesgoDespuesDeVerNoCambiaLoQueGuardaElWorktree(t *testing.T) {
	atPATH(t, nil)
	ca := cr4Hallazgos + ", riesgo cambiado despues de ver"
	dir, wt := cr4Tarea(t, true)
	cr4EnsuciarSpec(t, wt)
	s := newServer(t, wt)
	paths := cr4Guardable(t, ca, s)
	cr4Riesgo(t, dir, wt, true)
	headP, headW := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")
	copia := cr4Leer(t, wt)

	rec := cr4Guardar(t, ca, s, paths)
	cr4Resultado(t, ca, rec, paths, atGit(t, wt, "rev-parse", "HEAD"))
	cr4WorktreeGuardado(t, ca, wt, headW, cr4SpecRel)
	cr4PrincipalSinGuardar(t, ca, dir, headP)
	if cr4Leer(t, wt) != copia {
		t.Fatalf("%s: la copia del item del worktree no se toca", ca)
	}
}

// CA-345 (guarda de d26397 / f4d55b): desde el worktree el 409 sigue vivo.
// (a) Entre ver y guardar aparece un cambio nuevo que el tablero del
// worktree si muestra: un hallazgo de la tarea sin commitear (CA-286).
// (b) Se manda un conjunto distinto del mostrado: le falta una ruta, le
// sobra una, le suma el item, u otra ruta. En todos: 409 con la negativa del
// contrato y ningun commit ni cambio del indice en ningun arbol.
func TestHallazgo_d26397_f4d55b_GuardarDesdeElWorktreeSigueDando409(t *testing.T) {
	atPATH(t, nil)
	ca := cr4Hallazgos + ", guarda del 409 desde el worktree"

	// (a) un cambio nuevo, a la vista del worktree, despues de ver
	dir, wt := cr4Tarea(t, true)
	cr4Riesgo(t, dir, wt, true)
	cr4EnsuciarSpec(t, wt)
	s := newServer(t, wt)
	vistas := cr4Guardable(t, ca+" (a)", s)
	if _, err := finding.Register(wt, "main", finding.Draft{Severity: "low", Lens: "risk",
		Description: "aparecio entre ver y guardar", Author: "reviewer", Task: cr4Slug}); err != nil {
		t.Fatalf("fixture: registrar un hallazgo de la tarea en el worktree: %v", err)
	}
	if atMismas(cr4Vista(t, ca+" (a)", s), vistas) {
		t.Fatalf("%s (a): fixture: el hallazgo nuevo de la tarea aparece sin guardar en el tablero del worktree (CA-286): %v", ca, vistas)
	}
	cr4Niega(t, ca+" (a: aparecio un hallazgo despues de ver)", s, dir, wt, vistas)

	// (b) un conjunto distinto del mostrado
	dir, wt = cr4Tarea(t, true)
	cr4Riesgo(t, dir, wt, true)
	cr4EnsuciarSpec(t, wt)
	s = newServer(t, wt)
	vistas = cr4Guardable(t, ca+" (b)", s)
	for i := range vistas {
		sin := append(append([]string{}, vistas[:i]...), vistas[i+1:]...)
		if len(sin) == 0 {
			continue
		}
		cr4Niega(t, ca+" (b: sin "+vistas[i]+")", s, dir, wt, sin)
	}
	cr4Niega(t, ca+" (b: con una de mas)", s, dir, wt, append(append([]string{}, vistas...), "otro.go"))
	if !cr4Contiene(vistas, cr4ItemRel) {
		cr4Niega(t, ca+" (b: con el item)", s, dir, wt, append(append([]string{}, vistas...), cr4ItemRel))
	}
	cr4Niega(t, ca+" (b: otra ruta)", s, dir, wt, []string{"otro.go"})
}

// hallazgos 20261112T023834_d26397 y 20261112T024355_f4d55b (CA-345), con
// SOLO el riesgo cambiado desde el worktree (decision de la ronda 4b): el
// Studio del worktree no tiene que guardar eso. (a) Un POST save desde el
// worktree con un conjunto no vacio que el tablero no mostro (la ruta del
// item, otra ruta) da 409 sin commits en ningun arbol. (b) `hoom item save
// precios` sin rutas desde el worktree commitea el item del principal. No se
// fija si unsynced sale vacio ni si guardar esta habilitada.
func TestHallazgo_d26397_f4d55b_GuardarDesdeElWorktreeConSoloElRiesgo_SoloRiesgo(t *testing.T) {
	atPATH(t, nil)
	ca := cr4Hallazgos + ", solo el riesgo cambiado desde el worktree"
	dir, wt := cr4Tarea(t, true)
	cr4Riesgo(t, dir, wt, true)
	headP, headW := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")
	copia := cr4Leer(t, wt)
	s := newServer(t, wt)

	vista := cr4Vista(t, ca, s)
	for _, otro := range cr4NoMostrados(vista) {
		cr4Niega(t, ca+" (a: "+strings.Join(otro, ",")+")", s, dir, wt, otro)
	}

	cr4ItemSave(t, ca+" (b)", s, dir, wt, headP)
	if atGit(t, wt, "rev-parse", "HEAD") != headW || cr4Leer(t, wt) != copia {
		t.Fatalf("%s (b): el worktree no gana commit y la copia de su item no se toca", ca)
	}
}

// CA-345 (guarda de d26397 / f4d55b): con el Studio servido desde el
// checkout principal, el ida y vuelta tablero -> save con lo que muestra
// sigue dando 200 como hoy: el item en el principal y el spec en la rama.
func TestHallazgo_d26397_f4d55b_GuardarDesdeElTableroDelPrincipal(t *testing.T) {
	atPATH(t, nil)
	ca := cr4Hallazgos + ", guarda desde el principal"
	dir, wt := cr4Tarea(t, true)
	cr4Riesgo(t, dir, wt, true)
	cr4EnsuciarSpec(t, wt)
	headP, headW := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")
	copia := cr4Leer(t, wt)

	s := newServer(t, dir)
	paths := cr4Guardable(t, ca, s)
	rec := cr4Guardar(t, ca, s, paths)
	cr4Resultado(t, ca, rec, paths, atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD"))

	cr4PrincipalGuardado(t, ca, dir, headP)
	cr4WorktreeGuardado(t, ca, wt, headW, cr4SpecRel)
	if cr4Leer(t, wt) != copia {
		t.Fatalf("%s: la copia del item del worktree no se toca", ca)
	}
	cr4SinNada(t, ca, s, dir, wt)
}

// CA-345 (guarda de d26397 / f4d55b): con el item solo en la rama de la
// tarea (el principal no lo tiene), el ida y vuelta desde el tablero servido
// en el worktree da 200 y commitea en el worktree (item y spec); el
// principal no gana commit.
func TestHallazgo_d26397_f4d55b_ItemSoloEnLaTareaGuardaEnElWorktree(t *testing.T) {
	atPATH(t, nil)
	ca := cr4Hallazgos + ", guarda con el item solo en la tarea"
	dir, wt := cr4Tarea(t, false)
	cr4Riesgo(t, dir, wt, false)
	cr4EnsuciarSpec(t, wt)
	headP, headW := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")

	s := newServer(t, wt)
	paths := cr4Guardable(t, ca, s)
	rec := cr4Guardar(t, ca, s, paths)
	cr4Resultado(t, ca, rec, paths, atGit(t, wt, "rev-parse", "HEAD"))

	cr4WorktreeGuardado(t, ca, wt, headW, cr4ItemRel, cr4SpecRel)
	if !strings.Contains(atGit(t, wt, "show", "HEAD:"+cr4ItemRel), "riesgo: alto") {
		t.Fatalf("%s: el item commiteado en la rama tiene riesgo: alto", ca)
	}
	if atGit(t, dir, "rev-parse", "HEAD") != headP {
		t.Fatalf("%s: el principal no gana commit", ca)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(cr4ItemRel))); !os.IsNotExist(err) {
		t.Fatalf("%s: el principal sigue sin item: %v", ca, err)
	}
	cr4SinNada(t, ca, s, wt)
}
