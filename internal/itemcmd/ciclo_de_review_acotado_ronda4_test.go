// Tests de regresion de la ronda 4 de ciclo-de-review-acotado: guardar la
// tarjeta desde el worktree de la tarea con las rutas que esa misma tarjeta
// mostro (CA-345; hallazgos 20261112T023834_d26397 y
// 20261112T024355_f4d55b). Contrato (acciones-desde-la-tarjeta): Save
// commitea exactamente `unsynced` de la tarjeta, y un expect no nil tiene que
// ser igual a `unsynced` como conjunto; si no, se niega con ErrChanged.
//
// Decision de Henry (ronda 4b): con rutas esperadas (el Studio), guardar
// desde un arbol guarda exactamente lo que mostro la tarjeta vista desde ESE
// arbol, y en ese arbol. El item del checkout principal cambiado con `hoom
// item set` desde el worktree no lo guarda Save(<worktree>, expect): queda
// para `hoom item save <slug>` sin rutas, que desde el worktree lo commitea
// en el principal (ronda 3). La tarjeta no cambia.
//
// Las aserciones no fijan el espacio de rutas de `unsynced` (ni su prefijo)
// ni el dir de los commits: fijan la propiedad (lo mostrado guarda) y el
// estado final en git, y reconocen cada commit por su sha. Los casos con
// SOLO el riesgo cambiado van en una funcion terminada en _SoloRiesgo.
// Escritos desde el spec, las firmas publicas y el texto de los hallazgos,
// sin leer la implementacion. Ningun CLI de IA: el PATH tiene solo git y sh.
package itemcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
)

const (
	crR4Slug      = "precios"
	crR4ItemRel   = ".hoom/items/" + crR4Slug + ".yaml"
	crR4SpecRel   = ".hoom/specs/" + crR4Slug + ".md"
	crR4Spec      = "# Spec: precios\n\nlo que el arquitecto escribio\n"
	crR4SpecSucio = crR4Spec + "\nuna linea nueva sin guardar\n"
	crR4Hallazgos = "hallazgos 20261112T023834_d26397 y 20261112T024355_f4d55b (CA-345)"
	// crR4Ignorar es lo que un proyecto hoom deja fuera de git en .hoom/:
	// los espacios de trabajo y la cache (alli quedan los candados del item).
	crR4Ignorar = "worktrees/\ncache/\n"
)

// crR4PATH deja en el PATH solo git y sh de verdad: ningun CLI de IA de la
// maquina se ve mientras se deriva la tarjeta.
func crR4PATH(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	for _, h := range []string{"git", "sh"} {
		real, err := exec.LookPath(h)
		if err != nil {
			t.Fatalf("el test necesita %s: %v", h, err)
		}
		if err := os.Symlink(real, filepath.Join(bin, h)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

// crR4Tarea arma el proyecto con la tarea iniciada (worktree en
// .hoom/worktrees/precios, rama hoom/precios) y nada sin guardar: el item
// commiteado en el checkout principal (enPrincipal) o solo en la rama de la
// tarea, y el spec commiteado en la rama. .hoom/ ignora worktrees/ y cache/.
func crR4Tarea(t *testing.T, enPrincipal bool) (root, wt string) {
	t.Helper()
	root = icRepo(t)
	icEscribir(t, root, ".hoom/.gitignore", crR4Ignorar)
	if enPrincipal {
		icEscribir(t, root, crR4ItemRel, icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	}
	crCommitearItems(t, root)
	wt = filepath.Join(root, ".hoom", "worktrees", crR4Slug)
	icGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+crR4Slug, wt, "main")
	if !enPrincipal {
		icEscribir(t, wt, crR4ItemRel, icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	}
	icEscribir(t, wt, crR4SpecRel, crR4Spec)
	crCommitearItems(t, wt)
	for _, dir := range []string{root, wt} {
		if st := crR3Git(t, dir, "status", "--porcelain", "--untracked-files=all"); strings.TrimSpace(st) != "" {
			t.Fatalf("fixture: %s arranca sin nada sin guardar: %q", dir, st)
		}
	}
	if _, err := os.Stat(filepath.Join(root, crR4ItemRel)); enPrincipal == os.IsNotExist(err) {
		t.Fatalf("fixture: el principal tiene el item solo si enPrincipal (%v): %v", enPrincipal, err)
	}
	return root, wt
}

// crR4Riesgo es `hoom item set precios --riesgo alto` corrido desde el
// worktree: escribe el item del principal cuando lo tiene (ronda 1), si no
// el de la tarea, y no commitea.
func crR4Riesgo(t *testing.T, root, wt string, enPrincipal bool) {
	t.Helper()
	if _, err := Set(wt, "main", "high", crR4Slug, "alto", ""); err != nil {
		t.Fatalf("fixture: Set desde el worktree: %v", err)
	}
	donde := wt
	if enPrincipal {
		donde = root
	}
	if !strings.Contains(crR2Leer(t, donde, crR4Slug), "riesgo: alto") {
		t.Fatalf("fixture: el item de %s queda con riesgo: alto:\n%s", donde, crR2Leer(t, donde, crR4Slug))
	}
	if st := crR3Git(t, donde, "status", "--porcelain", "--", crR4ItemRel); strings.TrimSpace(st) == "" {
		t.Fatalf("fixture: el item de %s queda sin guardar", donde)
	}
}

// crR4EnsuciarSpec cambia el spec dentro del worktree, sin guardar.
func crR4EnsuciarSpec(t *testing.T, wt string) {
	t.Helper()
	icEscribir(t, wt, crR4SpecRel, crR4SpecSucio)
}

// crR4Mostrado es el unsynced de la tarjeta vista desde dir, sin exigirle
// nada.
func crR4Mostrado(t *testing.T, ca, dir string) []string {
	t.Helper()
	c, err := boardcmd.CardFor(dir, "main", "high", crR4Slug, time.Now().UTC())
	if err != nil {
		t.Fatalf("%s: la tarjeta vista desde %s se deriva: %v", ca, dir, err)
	}
	return append([]string{}, c.Unsynced...)
}

// crR4Vista es lo que la tarjeta, vista desde dir, muestra sin guardar
// cuando tiene algo de ese arbol que guardar: unsynced no vacio, y guardar
// habilitada (sin nadie trabajando) con esas mismas rutas, que son las que
// manda el Studio.
func crR4Vista(t *testing.T, ca, dir string) []string {
	t.Helper()
	c, err := boardcmd.CardFor(dir, "main", "high", crR4Slug, time.Now().UTC())
	if err != nil {
		t.Fatalf("%s: la tarjeta vista desde %s se deriva: %v", ca, dir, err)
	}
	if len(c.Unsynced) == 0 {
		t.Fatalf("%s: la tarjeta vista desde %s tiene cambios sin guardar y los muestra en unsynced", ca, dir)
	}
	a, ok := c.Action(boardcmd.ActGuardar)
	if !ok || !a.Enabled || !cbMismos(a.Paths, c.Unsynced) {
		t.Fatalf("%s: vista desde %s, la tarjeta ofrece guardar habilitada con las rutas de unsynced %v: %+v (esta %v)", ca, dir, c.Unsynced, a, ok)
	}
	return append([]string{}, c.Unsynced...)
}

// crR4SinNada exige que la tarjeta, vista desde cada dir, no tenga nada sin
// guardar.
func crR4SinNada(t *testing.T, ca string, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if u := crR4Mostrado(t, ca, dir); len(u) != 0 {
			t.Fatalf("%s: al final, la tarjeta vista desde %s no tiene nada sin guardar: %v", ca, dir, u)
		}
	}
}

// crR4Estado resume lo que un commit cambiaria en cada arbol: HEAD, el
// indice y lo que no esta commiteado.
func crR4Estado(t *testing.T, dirs ...string) string {
	t.Helper()
	var b strings.Builder
	for _, d := range dirs {
		fmt.Fprintf(&b, "%s\nHEAD %s\nindice:\n%s\nestado:\n%s\n", d,
			crHead(t, d),
			crR3Git(t, d, "diff", "--cached", "--name-only"),
			crR3Git(t, d, "status", "--porcelain", "--untracked-files=all"))
	}
	return b.String()
}

// crR4Resultado exige el SaveResult de la tarjeta con un commit por cada sha
// de shas (los HEAD nuevos de los arboles que guardaron), reconocidos por su
// sha y no por su dir.
func crR4Resultado(t *testing.T, ca string, res SaveResult, shas ...string) {
	t.Helper()
	if res.Slug != crR4Slug || res.Message != SaveMessage(crR4Slug) {
		t.Fatalf("%s: el SaveResult nombra la tarjeta y el mensaje fijo: %+v", ca, res)
	}
	if len(res.Commits) != len(shas) {
		t.Fatalf("%s: exactamente %d commit(s), uno por arbol que guardo: %+v", ca, len(shas), res.Commits)
	}
	tiene := map[string]bool{}
	for _, c := range res.Commits {
		tiene[c.SHA] = true
	}
	for _, sha := range shas {
		if !tiene[sha] {
			t.Fatalf("%s: el SaveResult trae el commit %s (el HEAD nuevo de su arbol): %+v", ca, sha, res.Commits)
		}
	}
}

// crR4PrincipalGuardado exige un solo commit nuevo en el principal sobre
// headP, con el mensaje fijo y solo el item, que queda limpio y con riesgo:
// alto.
func crR4PrincipalGuardado(t *testing.T, ca, root, headP string) {
	t.Helper()
	if crHead(t, root) == headP {
		t.Fatalf("%s: el principal gana un commit con el item", ca)
	}
	if p := strings.TrimSpace(crR3Git(t, root, "rev-parse", "HEAD~1")); p != headP {
		t.Fatalf("%s: un solo commit nuevo en el principal: el padre es %s, no %s", ca, p, headP)
	}
	if got := cbDelCommit(t, root); !cbMismos(got, []string{crR4ItemRel}) {
		t.Fatalf("%s: el commit del principal lleva solo el item: %v", ca, got)
	}
	if st := crR3Git(t, root, "status", "--porcelain", "--", crR4ItemRel); strings.TrimSpace(st) != "" {
		t.Fatalf("%s: el item del principal queda limpio en git status: %q", ca, st)
	}
	if !strings.Contains(crR3Git(t, root, "show", "HEAD:"+crR4ItemRel), "riesgo: alto") {
		t.Fatalf("%s: el commit nuevo del principal contiene riesgo: alto", ca)
	}
	if msg := strings.TrimSpace(crR3Git(t, root, "log", "-1", "--format=%B")); msg != SaveMessage(crR4Slug) {
		t.Fatalf("%s: el commit del principal tiene el mensaje fijo: %q", ca, msg)
	}
}

// crR4PrincipalSinGuardar exige que el principal no gane commit y que su
// item siga con riesgo: alto sin commitear: el Studio del worktree no lo
// guarda (decision de la ronda 4b).
func crR4PrincipalSinGuardar(t *testing.T, ca, root, headP string) {
	t.Helper()
	if crHead(t, root) != headP {
		t.Fatalf("%s: el principal no gana commit: su item lo guarda `hoom item save` sin rutas, no Save con lo que mostro el worktree", ca)
	}
	if !strings.Contains(crR2Leer(t, root, crR4Slug), "riesgo: alto") {
		t.Fatalf("%s: el item del principal sigue con riesgo: alto", ca)
	}
	if st := crR3Git(t, root, "status", "--porcelain", "--", crR4ItemRel); strings.TrimSpace(st) == "" {
		t.Fatalf("%s: el item del principal sigue sin commitear", ca)
	}
}

// crR4WorktreeGuardado exige un solo commit nuevo en el worktree, en la rama
// de la tarea, con el mensaje fijo y exactamente rutas (relativas al
// worktree); el worktree queda limpio y el spec commiteado con el cambio.
func crR4WorktreeGuardado(t *testing.T, ca, wt, headW string, rutas ...string) {
	t.Helper()
	nuevo := crHead(t, wt)
	if nuevo == headW {
		t.Fatalf("%s: el worktree gana un commit con el spec", ca)
	}
	if p := strings.TrimSpace(crR3Git(t, wt, "rev-parse", "HEAD~1")); p != headW {
		t.Fatalf("%s: un solo commit nuevo en el worktree: el padre es %s, no %s", ca, p, headW)
	}
	if rama := strings.TrimSpace(crR3Git(t, wt, "rev-parse", "hoom/"+crR4Slug)); rama != nuevo {
		t.Fatalf("%s: el commit queda en la rama de la tarea hoom/%s: %s vs %s", ca, crR4Slug, rama, nuevo)
	}
	if got := cbDelCommit(t, wt); !cbMismos(got, rutas) {
		t.Fatalf("%s: el commit del worktree lleva exactamente %v: %v", ca, rutas, got)
	}
	if got := crR3Git(t, wt, "show", "HEAD:"+crR4SpecRel); got != crR4SpecSucio {
		t.Fatalf("%s: el spec commiteado en la rama es el cambiado en el worktree: %q", ca, got)
	}
	if st := crR3Git(t, wt, "status", "--porcelain", "--untracked-files=all"); strings.TrimSpace(st) != "" {
		t.Fatalf("%s: el worktree queda sin nada sin guardar: %q", ca, st)
	}
	if msg := strings.TrimSpace(crR3Git(t, wt, "log", "-1", "--format=%B")); msg != SaveMessage(crR4Slug) {
		t.Fatalf("%s: el commit del worktree tiene el mensaje fijo: %q", ca, msg)
	}
}

// crR4EstadoFinal es el estado final de guardar los dos arboles: el item del
// principal commiteado alli, el spec del worktree commiteado en la rama, y la
// copia del item del worktree sin tocar.
func crR4EstadoFinal(t *testing.T, ca, root, wt, headP, headW, copia string) {
	t.Helper()
	crR4PrincipalGuardado(t, ca, root, headP)
	crR4WorktreeGuardado(t, ca, wt, headW, crR4SpecRel)
	if crR2Leer(t, wt, crR4Slug) != copia {
		t.Fatalf("%s: la copia del item del worktree no se toca", ca)
	}
}

// crR4ItemSave es `hoom item save precios` sin rutas corrido desde el
// worktree: commitea el item del principal (ronda 3), un commit con solo el
// item, sin commit nuevo en el worktree; despues los dos arboles quedan
// limpios y la tarjeta sin nada sin guardar.
func crR4ItemSave(t *testing.T, ca, root, wt, headP string) {
	t.Helper()
	headW := crHead(t, wt)
	out, errb, err := cbExec(t, wt, "save", crR4Slug)
	if err != nil || !strings.Contains(out, "tarjeta "+crR4Slug+" guardada") {
		t.Fatalf("%s: hoom item save %s sin rutas desde el worktree guarda: %v\n%s%s", ca, crR4Slug, err, out, errb)
	}
	crR4PrincipalGuardado(t, ca, root, headP)
	if crHead(t, wt) != headW {
		t.Fatalf("%s: hoom item save no hace commit nuevo en el worktree, que no tenia nada sin guardar", ca)
	}
	if st := crR3Git(t, wt, "status", "--porcelain", "--untracked-files=all"); strings.TrimSpace(st) != "" {
		t.Fatalf("%s: el worktree queda limpio: %q", ca, st)
	}
	crR4SinNada(t, ca, wt, root)
}

// crR4Niega exige que Save desde el worktree con expect se niegue con
// ErrChanged sin commitear ni tocar el indice de ningun arbol.
func crR4Niega(t *testing.T, ca, root, wt string, expect []string) {
	t.Helper()
	antes := crR4Estado(t, root, wt)
	_, err := Save(wt, "main", "high", crR4Slug, expect)
	if err == nil || !strings.Contains(err.Error(), ErrChanged) {
		t.Fatalf("%s: Save desde el worktree con expect %v se niega con %q: %v", ca, expect, ErrChanged, err)
	}
	if despues := crR4Estado(t, root, wt); despues != antes {
		t.Fatalf("%s: la negativa no commitea ni cambia el indice en ningun arbol:\nantes:\n%s\ndespues:\n%s", ca, antes, despues)
	}
}

func crR4Contiene(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}

// crR4NoMostrados son conjuntos no vacios distintos, como conjunto, de lo
// que la tarjeta mostro: la ruta del item, otra ruta, y lo mostrado mas la
// ruta del item.
func crR4NoMostrados(vista []string) [][]string {
	out := [][]string{}
	for _, c := range [][]string{{crR4ItemRel}, {"otro.go"}} {
		if !cbMismos(c, vista) {
			out = append(out, c)
		}
	}
	if len(vista) > 0 && !crR4Contiene(vista, crR4ItemRel) {
		out = append(out, append(append([]string{}, vista...), crR4ItemRel))
	}
	return out
}

// hallazgos 20261112T023834_d26397 y 20261112T024355_f4d55b (CA-345), por
// itemcmd, con la decision de la ronda 4b: el riesgo puesto con Set desde el
// worktree (item del principal) y el spec cambiado en el worktree.
// Save(<worktree>, expect = el unsynced de la tarjeta vista desde el
// worktree) no responde ErrChanged y hace exactamente un commit, en
// hoom/precios, solo con el spec; el principal no gana commit y su item
// sigue con riesgo: alto sin commitear. Despues `hoom item save precios` sin
// rutas desde el worktree commitea el item del principal, y recien ahi los
// dos arboles quedan limpios y la tarjeta sin nada sin guardar.
func TestHallazgo_d26397_f4d55b_SaveDesdeElWorktreeConLoQueLaTarjetaMostro(t *testing.T) {
	crR4PATH(t)
	ca := crR4Hallazgos + ", Save desde el worktree"
	root, wt := crR4Tarea(t, true)
	crR4Riesgo(t, root, wt, true)
	crR4EnsuciarSpec(t, wt)
	headP, headW, copia := crHead(t, root), crHead(t, wt), crR2Leer(t, wt, crR4Slug)

	vistas := crR4Vista(t, ca, wt)
	res, err := Save(wt, "main", "high", crR4Slug, vistas)
	if err != nil {
		if strings.Contains(err.Error(), ErrChanged) {
			t.Fatalf("%s: con las rutas que la tarjeta mostro (%v) no responde %q: nada cambio entre ver y guardar", ca, vistas, ErrChanged)
		}
		t.Fatalf("%s: Save con las rutas que la tarjeta mostro guarda: %v", ca, err)
	}
	crR4Resultado(t, ca, res, crHead(t, wt))
	crR4WorktreeGuardado(t, ca, wt, headW, crR4SpecRel)
	crR4PrincipalSinGuardar(t, ca, root, headP)
	if crR2Leer(t, wt, crR4Slug) != copia {
		t.Fatalf("%s: la copia del item del worktree no se toca", ca)
	}

	crR4ItemSave(t, ca+" (despues, hoom item save)", root, wt, headP)
}

// CA-345 (d26397 / f4d55b, decision de la ronda 4b): el riesgo cambiado
// desde el worktree DESPUES de ver la tarjeta con el spec sucio no cambia lo
// que guarda el worktree. Save con lo visto guarda solo el spec en
// hoom/precios, y el item del principal sigue sin commitear.
func TestHallazgo_d26397_f4d55b_RiesgoDespuesDeVerNoCambiaLoQueGuardaElWorktree(t *testing.T) {
	crR4PATH(t)
	ca := crR4Hallazgos + ", riesgo cambiado despues de ver"
	root, wt := crR4Tarea(t, true)
	crR4EnsuciarSpec(t, wt)
	vistas := crR4Vista(t, ca, wt)
	crR4Riesgo(t, root, wt, true)
	headP, headW, copia := crHead(t, root), crHead(t, wt), crR2Leer(t, wt, crR4Slug)

	res, err := Save(wt, "main", "high", crR4Slug, vistas)
	if err != nil {
		t.Fatalf("%s: Save con lo que la tarjeta mostro (%v) guarda: %v", ca, vistas, err)
	}
	crR4Resultado(t, ca, res, crHead(t, wt))
	crR4WorktreeGuardado(t, ca, wt, headW, crR4SpecRel)
	crR4PrincipalSinGuardar(t, ca, root, headP)
	if crR2Leer(t, wt, crR4Slug) != copia {
		t.Fatalf("%s: la copia del item del worktree no se toca", ca)
	}
}

// CA-345 (guarda de d26397 / f4d55b): desde el worktree el ErrChanged sigue
// vivo. (a) Entre ver y guardar aparece un cambio nuevo que la tarjeta vista
// desde el worktree si muestra: un hallazgo de la tarea sin commitear
// (CA-286). (b) Se manda un conjunto distinto del mostrado: le falta una
// ruta, le sobra una, le suma el item, otra ruta, vacio. En todos: ErrChanged
// y ningun commit ni cambio del indice en ningun arbol.
func TestHallazgo_d26397_f4d55b_SaveDesdeElWorktreeSigueNegandoLoQueCambio(t *testing.T) {
	crR4PATH(t)
	ca := crR4Hallazgos + ", guarda del ErrChanged"

	// (a) un cambio nuevo, a la vista del worktree, despues de ver
	root, wt := crR4Tarea(t, true)
	crR4Riesgo(t, root, wt, true)
	crR4EnsuciarSpec(t, wt)
	vistas := crR4Vista(t, ca+" (a)", wt)
	if _, err := finding.Register(wt, "main", finding.Draft{Severity: "low", Lens: "risk",
		Description: "aparecio entre ver y guardar", Author: "reviewer", Task: crR4Slug}); err != nil {
		t.Fatalf("fixture: registrar un hallazgo de la tarea en el worktree: %v", err)
	}
	if cbMismos(crR4Mostrado(t, ca, wt), vistas) {
		t.Fatalf("%s (a): fixture: el hallazgo nuevo de la tarea aparece sin guardar en la tarjeta vista desde el worktree (CA-286): %v", ca, vistas)
	}
	crR4Niega(t, ca+" (a: aparecio un hallazgo despues de ver)", root, wt, vistas)

	// (b) un conjunto distinto del mostrado
	root, wt = crR4Tarea(t, true)
	crR4Riesgo(t, root, wt, true)
	crR4EnsuciarSpec(t, wt)
	vistas = crR4Vista(t, ca+" (b)", wt)
	for i := range vistas {
		sin := append(append([]string{}, vistas[:i]...), vistas[i+1:]...)
		if len(sin) == 0 {
			continue
		}
		crR4Niega(t, ca+" (b: sin "+vistas[i]+")", root, wt, sin)
	}
	crR4Niega(t, ca+" (b: con una de mas)", root, wt, append(append([]string{}, vistas...), "otro.go"))
	if !crR4Contiene(vistas, crR4ItemRel) {
		crR4Niega(t, ca+" (b: con el item)", root, wt, append(append([]string{}, vistas...), crR4ItemRel))
	}
	crR4Niega(t, ca+" (b: otra ruta)", root, wt, []string{"otro.go"})
	crR4Niega(t, ca+" (b: vacio, no nil)", root, wt, []string{})
}

// hallazgos 20261112T023834_d26397 y 20261112T024355_f4d55b (CA-345), con
// SOLO el riesgo cambiado desde el worktree (decision de la ronda 4b): el
// worktree no tiene nada propio que guardar. (a) Save(<worktree>) con un
// conjunto no vacio que la tarjeta no mostro (la ruta del item, otra ruta)
// se niega con ErrChanged sin commits en ningun arbol. (b) `hoom item save
// precios` sin rutas desde el worktree commitea el item del principal. No se
// fija si unsynced sale vacio ni si guardar esta habilitada.
func TestHallazgo_d26397_f4d55b_SaveDesdeElWorktreeConSoloElRiesgo_SoloRiesgo(t *testing.T) {
	crR4PATH(t)
	ca := crR4Hallazgos + ", solo el riesgo cambiado desde el worktree"
	root, wt := crR4Tarea(t, true)
	crR4Riesgo(t, root, wt, true)
	headP, headW, copia := crHead(t, root), crHead(t, wt), crR2Leer(t, wt, crR4Slug)

	vista := crR4Mostrado(t, ca, wt)
	for _, otro := range crR4NoMostrados(vista) {
		crR4Niega(t, ca+" (a: "+strings.Join(otro, ",")+")", root, wt, otro)
	}

	crR4ItemSave(t, ca+" (b)", root, wt, headP)
	if crHead(t, wt) != headW || crR2Leer(t, wt, crR4Slug) != copia {
		t.Fatalf("%s (b): el worktree no gana commit y la copia de su item no se toca", ca)
	}
}

// CA-345 (guarda de d26397 / f4d55b): desde el checkout principal, el ida y
// vuelta tarjeta -> Save con lo que muestra sigue guardando como hoy: el
// item en el principal y el spec en la rama.
func TestHallazgo_d26397_f4d55b_GuardaDesdeElPrincipal(t *testing.T) {
	crR4PATH(t)
	ca := crR4Hallazgos + ", guarda desde el principal"
	root, wt := crR4Tarea(t, true)
	crR4Riesgo(t, root, wt, true)
	crR4EnsuciarSpec(t, wt)
	headP, headW, copia := crHead(t, root), crHead(t, wt), crR2Leer(t, wt, crR4Slug)

	vistas := crR4Vista(t, ca, root)
	res, err := Save(root, "main", "high", crR4Slug, vistas)
	if err != nil {
		t.Fatalf("%s: Save desde el principal con lo que la tarjeta muestra (%v): %v", ca, vistas, err)
	}
	crR4Resultado(t, ca, res, crHead(t, root), crHead(t, wt))
	crR4EstadoFinal(t, ca, root, wt, headP, headW, copia)
	crR4SinNada(t, ca, root, wt)
}

// CA-345 (guarda de d26397 / f4d55b): con el item solo en la rama de la
// tarea (el principal no lo tiene), el ida y vuelta desde el worktree guarda
// en el worktree (item y spec, un commit) y el principal no gana commit.
func TestHallazgo_d26397_f4d55b_ItemSoloEnLaTareaGuardaEnElWorktree(t *testing.T) {
	crR4PATH(t)
	ca := crR4Hallazgos + ", guarda con el item solo en la tarea"
	root, wt := crR4Tarea(t, false)
	crR4Riesgo(t, root, wt, false)
	crR4EnsuciarSpec(t, wt)
	headP, headW := crHead(t, root), crHead(t, wt)

	vistas := crR4Vista(t, ca, wt)
	res, err := Save(wt, "main", "high", crR4Slug, vistas)
	if err != nil {
		t.Fatalf("%s: Save desde el worktree con lo que la tarjeta muestra (%v): %v", ca, vistas, err)
	}
	crR4Resultado(t, ca, res, crHead(t, wt))
	crR4WorktreeGuardado(t, ca, wt, headW, crR4ItemRel, crR4SpecRel)
	if !strings.Contains(crR3Git(t, wt, "show", "HEAD:"+crR4ItemRel), "riesgo: alto") {
		t.Fatalf("%s: el item commiteado en la rama tiene riesgo: alto", ca)
	}
	if crHead(t, root) != headP {
		t.Fatalf("%s: el principal no gana commit", ca)
	}
	if _, err := os.Stat(filepath.Join(root, crR4ItemRel)); !os.IsNotExist(err) {
		t.Fatalf("%s: el principal sigue sin item: %v", ca, err)
	}
	crR4SinNada(t, ca, wt)
}

// CA-345 (guarda de d26397 / f4d55b): `hoom item save precios` por la CLI,
// sin rutas esperadas, desde el worktree sigue funcionando como hoy: guarda
// el item en el principal y el spec en la rama, y dice que commit hizo en
// cada arbol.
func TestHallazgo_d26397_f4d55b_ItemSaveSinRutasDesdeElWorktree(t *testing.T) {
	crR4PATH(t)
	ca := crR4Hallazgos + ", hoom item save desde el worktree"
	root, wt := crR4Tarea(t, true)
	crR4Riesgo(t, root, wt, true)
	crR4EnsuciarSpec(t, wt)
	headP, headW, copia := crHead(t, root), crHead(t, wt), crR2Leer(t, wt, crR4Slug)

	out, errb, err := cbExec(t, wt, "save", crR4Slug)
	if err != nil || !strings.Contains(out, "hoom item save: tarjeta "+crR4Slug+" guardada") {
		t.Fatalf("%s: sale bien con `hoom item save: tarjeta %s guardada`: %v\n%s%s", ca, crR4Slug, err, out, errb)
	}
	crR4EstadoFinal(t, ca, root, wt, headP, headW, copia)
	for _, dir := range []string{root, wt} {
		sha := crHead(t, dir)
		linea := ""
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, sha[:12]) {
				linea = l
			}
		}
		if !strings.Contains(linea, "(1 archivo)") {
			t.Fatalf("%s: el texto tiene una linea con el commit %s y `(1 archivo)`:\n%s", ca, sha[:12], out)
		}
	}
	crR4SinNada(t, ca, wt, root)
}

// CA-345 (guarda de d26397 / f4d55b), por el binario: `hoom item save
// precios --json` desde el worktree sale 0 con el SaveResult de los dos
// commits y deja el mismo estado final.
func TestHallazgo_d26397_f4d55b_ItemSaveSinRutasDesdeElWorktreePorElBinario(t *testing.T) {
	bin := ixBinario(t)
	crR4PATH(t)
	ca := crR4Hallazgos + ", hoom item save --json desde el worktree (binario)"
	root, wt := crR4Tarea(t, true)
	if r := crCorrer(t, bin, wt, "", "item", "set", crR4Slug, "--riesgo", "alto"); r.exit != 0 {
		t.Fatalf("fixture: hoom item set desde el worktree sale 0: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	if !strings.Contains(crR2Leer(t, root, crR4Slug), "riesgo: alto") {
		t.Fatal("fixture: el set desde el worktree escribe el item del principal")
	}
	crR4EnsuciarSpec(t, wt)
	headP, headW, copia := crHead(t, root), crHead(t, wt), crR2Leer(t, wt, crR4Slug)

	r := crCorrer(t, bin, wt, "", "item", "save", crR4Slug, "--json")
	if r.exit != 0 {
		t.Fatalf("%s: sale 0: %d\n%s%s", ca, r.exit, r.stdout, r.stderr)
	}
	var res SaveResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &res); err != nil {
		t.Fatalf("%s: --json emite el SaveResult: %v\n%s", ca, err, r.stdout)
	}
	crR4Resultado(t, ca, res, crHead(t, root), crHead(t, wt))
	crR4EstadoFinal(t, ca, root, wt, headP, headW, copia)
	crR4SinNada(t, ca, wt, root)
}
