// Tests de regresion de la ronda 1 de ciclo-de-review-acotado sobre `hoom
// item set` desde el worktree de la tarea. Contrato (decision de Henry): el
// riesgo de una tarjeta se lee del item del CHECKOUT PRINCIPAL, tambien
// desde el worktree; si no esta ahi, del arbol de la tarea; si no, normal.
// Escritos desde el spec, las firmas publicas y el texto de los hallazgos,
// sin leer la implementacion.
package itemcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
)

// crR1Tarea arma el proyecto con el item slug sin riesgo commiteado en main
// y la tarea iniciada: su worktree en .hoom/worktrees/<slug>.
func crR1Tarea(t *testing.T, slug string) (root, wt string) {
	t.Helper()
	root = icRepo(t)
	icEscribir(t, root, ".hoom/items/"+slug+".yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, ".hoom/.gitignore", "worktrees/\n")
	crCommitearItems(t, root)
	wt = filepath.Join(root, ".hoom", "worktrees", slug)
	icGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	return root, wt
}

// crR1LoMismo exige que todos lean alto despues del set desde el worktree:
// el item escrito es el del principal, RiesgoDe desde el worktree y desde el
// principal, y la tarjeta del tablero.
func crR1LoMismo(t *testing.T, id, root, wt, slug string) {
	t.Helper()
	it, err := item.Load(root, slug)
	if err != nil || it.Riesgo != "alto" {
		t.Fatalf("hallazgo %s (CA-617): el item escrito es el del checkout principal, con riesgo alto: %q %v", id, it.Riesgo, err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", slug+".yaml"))
	if !strings.Contains(string(raw), "riesgo: alto") {
		t.Fatalf("hallazgo %s (CA-617): el archivo del principal dice riesgo: alto:\n%s", id, raw)
	}
	for _, dir := range []string{wt, root} {
		if r, err := item.RiesgoDe(dir, slug); err != nil || r != "alto" {
			t.Fatalf("hallazgo %s (CA-617): lo que se escribe es lo que se lee: RiesgoDe(%s) = %q %v", id, dir, r, err)
		}
	}
	c, err := boardcmd.CardFor(root, "main", "high", slug, time.Now().UTC())
	if err != nil || c.Item.Riesgo != "alto" {
		t.Fatalf("hallazgo %s (CA-617): la tarjeta del tablero dice alto: %q %v", id, c.Item.Riesgo, err)
	}
}

// hallazgo 20261111T203520_aee796 (CA-617): itemcmd.Set corrido desde el
// worktree de una tarea iniciada anuncia normal -> alto; despues todos (el
// item del principal, RiesgoDe desde el worktree, la tarjeta) dicen alto.
func TestHallazgo_aee796_SetDesdeElWorktreeEscribeLoQueSeLee(t *testing.T) {
	root, wt := crR1Tarea(t, "precios")
	res, err := Set(wt, "main", "high", "precios", "alto", "")
	if err != nil {
		t.Fatalf("aee796: Set desde el worktree: %v", err)
	}
	if res.Antes != "normal" || res.Riesgo != "alto" || !res.Cambio {
		t.Fatalf("aee796 (fixture): anuncia normal -> alto: %+v", res)
	}
	crR1LoMismo(t, "20261111T203520_aee796", root, wt, "precios")
}

// hallazgo 20261111T204019_f91dcf (CA-617), el mismo defecto por el
// binario: `hoom item set precios --riesgo alto` desde el worktree, y
// despues `hoom item show precios --json` (desde el worktree y desde el
// principal) dice alto, igual que la tarjeta.
func TestHallazgo_f91dcf_ItemSetPorElBinarioDesdeElWorktree(t *testing.T) {
	bin := ixBinario(t)
	root, wt := crR1Tarea(t, "precios")
	r := crCorrer(t, bin, wt, "", "item", "set", "precios", "--riesgo", "alto")
	if r.exit != 0 || !strings.Contains(r.stdout, "riesgo de precios: normal -> alto") {
		t.Fatalf("f91dcf (fixture): set sale 0 con normal -> alto: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	for _, dir := range []string{wt, root} {
		s := crCorrer(t, bin, dir, "", "item", "show", "precios", "--json")
		var v map[string]any
		if s.exit != 0 || json.Unmarshal([]byte(s.stdout), &v) != nil {
			t.Fatalf("f91dcf: item show --json desde %s: %d\n%s%s", dir, s.exit, s.stdout, s.stderr)
		}
		if !strings.Contains(s.stdout, `"riesgo":"alto"`) && !strings.Contains(s.stdout, `"riesgo": "alto"`) {
			t.Fatalf("hallazgo 20261111T204019_f91dcf (CA-617): item show --json desde %s dice riesgo alto:\n%s", dir, s.stdout)
		}
	}
	crR1LoMismo(t, "20261111T204019_f91dcf", root, wt, "precios")
}
