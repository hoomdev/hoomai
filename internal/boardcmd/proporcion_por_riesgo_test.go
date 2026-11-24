// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre el
// tablero: un spec sin aprobacion vigente que pasa su tope da la columna
// arquitecto con las lineas de spec_lint en missing, y uno aprobado la de hoy
// (CA-798, que re-expresa CA-270 y CA-271); las acciones de rol traen los
// valores con el riesgo declarado de su tarjeta y el modelo traducido
// (CA-804 y CA-805, que re-expresan CA-491 y CA-492). Escritos desde el
// spec, las aclaraciones de la ronda 0 y las firmas publicas, sin leer la
// implementacion.
package boardcmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/valores"
)

// pprSpecN es el spec de bdSpecTexto (con Riesgos) con k criterios y
// exactamente n palabras (strings.Fields del archivo entero).
func pprSpecN(t *testing.T, n, k int) string {
	t.Helper()
	var crit strings.Builder
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&crit, "- CA-%d: criterio %d.\n", i, i)
	}
	s := bdSpecTexto(crit.String(), true)
	base := len(strings.Fields(s))
	if n < base {
		t.Fatalf("fixture: el spec ya tiene %d palabras", base)
	}
	s = strings.Replace(s, "## Contratos\nx", "## Contratos\nx"+strings.Repeat(" palabra", n-base), 1)
	if got := len(strings.Fields(s)); got != n {
		t.Fatalf("fixture: %d palabras, no %d", got, n)
	}
	return s
}

// pprTarjeta arma la tarjeta precios con espacio de trabajo y su spec de n
// palabras y k criterios (sin aprobar); itemExtra va en el item del
// proyecto.
func pprTarjeta(t *testing.T, yamlExtra, itemExtra string, n, k int) (root, wt string) {
	t.Helper()
	root = bdRepo(t, yamlExtra)
	bdItemArchivo(t, root, bdSlug, "Precios", itemExtra)
	bdCommitear(t, root, "item")
	wt = bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, pprSpecN(t, n, k))
	return root, wt
}

func pprLineaPal(p int, r string, tope int) string {
	return fmt.Sprintf(`el spec tiene %d palabras y el tope con riesgo %s es %d: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-palabras %d --motivo "<por que>"`, p, r, tope, p)
}

func pprTieneLinea(list []string, l string) bool {
	for _, x := range list {
		if x == l {
			return true
		}
	}
	return false
}

// CA-798: un spec sin aprobar que pasa su tope da arquitecto con la linea de
// spec_lint en missing (y en Evidence.LintIssues de Gather), no
// tu-aprobacion. Re-expresa CA-271.
func TestCA798_SinAprobarQuePasaSuTopeVaAlArquitecto(t *testing.T) {
	const ca = "CA-798"
	root, _ := pprTarjeta(t, "", "", 801, 2)
	c := vdtCardFor(t, ca, root)
	want := pprLineaPal(801, "normal", 800)
	if c.Column != ColArquitecto || !pprTieneLinea(c.Missing, want) {
		t.Fatalf("%s: columna arquitecto con\n%s\nen missing; fue %s %q", ca, want, c.Column, c.Missing)
	}
	it, err := item.Load(root, bdSlug)
	if err != nil {
		t.Fatal(err)
	}
	ev := Gather(root, "main", "high", it, time.Now().UTC())
	if !pprTieneLinea(ev.LintIssues, want) {
		t.Fatalf("%s: Gather agrega la linea a LintIssues: %q", ca, ev.LintIssues)
	}
	if d := Derive(ev); d.Column != ColArquitecto {
		t.Fatalf("%s: Derive de esa evidencia da arquitecto: %s", ca, d.Column)
	}
	// en el tope exacto: tu-aprobacion como hoy
	root, _ = pprTarjeta(t, "", "", 800, 8)
	if c := vdtCardFor(t, ca, root); c.Column != ColTuAprobacion {
		t.Fatalf("%s: 800 palabras y 8 criterios van a tu-aprobacion: %s %q", ca, c.Column, c.Missing)
	}
}

// CA-271 (re-expresado por CA-798): lint OK sin aprobacion vigente es
// tu-aprobacion solo dentro del tope; con 9 criterios es arquitecto.
func TestCA271_TuAprobacionSoloDentroDelTope(t *testing.T) {
	const ca = "CA-271"
	root, _ := pprTarjeta(t, "", "", 300, 9)
	c := vdtCardFor(t, ca, root)
	want := `el spec tiene 9 criterios y el tope con riesgo normal es 8: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-criterios 9 --motivo "<por que>"`
	if c.Column != ColArquitecto || !pprTieneLinea(c.Missing, want) {
		t.Fatalf("%s: un spec sin aprobar que pasa su tope no va a tu-aprobacion: %s %q", ca, c.Column, c.Missing)
	}
	root, _ = pprTarjeta(t, "", "", 300, 3)
	if c := vdtCardFor(t, ca, root); c.Column != ColTuAprobacion {
		t.Fatalf("%s: dentro del tope sigue tu-aprobacion: %s", ca, c.Column)
	}
}

// CA-270 (re-expresado por CA-798): con issues de lint y el tope pasado,
// arquitecto con los issues y luego las lineas del tope en missing.
func TestCA270_IssuesYLuegoLasLineasDelTope(t *testing.T) {
	const ca = "CA-270"
	root, wt := pprTarjeta(t, "", "", 900, 2)
	s := strings.Replace(pprSpecN(t, 900, 2), "\n## Riesgos\nx\n", "\n", 1) + " otra"
	bdEscribir(t, wt, bdSpec, s)
	c := vdtCardFor(t, ca, root)
	if c.Column != ColArquitecto || len(c.Missing) < 2 {
		t.Fatalf("%s: arquitecto con el issue y la linea del tope: %s %q", ca, c.Column, c.Missing)
	}
	last := c.Missing[len(c.Missing)-1]
	if !strings.HasPrefix(last, "el spec tiene ") || strings.HasPrefix(c.Missing[0], "el spec tiene ") {
		t.Fatalf("%s: los issues de lint van antes que las lineas del tope: %q", ca, c.Missing)
	}
}

// CA-798: un spec aprobado que pasa su tope tiene la columna de hoy (aqui
// test-writer: CA-1 sin prueba) y ninguna linea del tope; si cambia despues de
// aprobarse, arquitecto.
func TestCA798_AprobadoTieneLaColumnaDeHoy(t *testing.T) {
	const ca = "CA-798"
	root, wt := pprTarjeta(t, "", "", 1500, 2)
	bdAprobar(t, wt, bdSlug)
	c := vdtCardFor(t, ca, root)
	if c.Column == ColArquitecto || c.Column == ColTuAprobacion {
		t.Fatalf("%s: aprobado no se mide: columna de hoy, fue %s %q", ca, c.Column, c.Missing)
	}
	for _, m := range c.Missing {
		if strings.HasPrefix(m, "el spec tiene ") {
			t.Fatalf("%s: aprobado no lleva lineas del tope: %q", ca, c.Missing)
		}
	}
	bdEscribir(t, wt, bdSpec, pprSpecN(t, 1501, 2))
	if c := vdtCardFor(t, ca, root); c.Column != ColArquitecto || !pprTieneLinea(c.Missing, pprLineaPal(1501, "normal", 800)) {
		t.Fatalf("%s: cambiado despues de aprobarse se mide: %s %q", ca, c.Column, c.Missing)
	}
}

// CA-798: el riesgo declarado de la tarjeta elige el tope, y los topes salen
// del hoom.yaml del checkout principal (aclaracion 16), aunque la rama de la
// tarjeta diga otro.
func TestCA798_RiesgoYHoomYamlDelProyecto(t *testing.T) {
	const ca = "CA-798"
	root, _ := pprTarjeta(t, "", "riesgo: alto\n", 1500, 12)
	if c := vdtCardFor(t, ca, root); c.Column == ColArquitecto {
		t.Fatalf("%s: alto tiene tope 2000 y 15: %s %q", ca, c.Column, c.Missing)
	}
	root, _ = pprTarjeta(t, "", "riesgo: bajo\n", 401, 2)
	if c := vdtCardFor(t, ca, root); c.Column != ColArquitecto || !pprTieneLinea(c.Missing, pprLineaPal(401, "bajo", 400)) {
		t.Fatalf("%s: bajo tiene tope 400: %s %q", ca, c.Column, c.Missing)
	}
	// el hoom.yaml del proyecto, despues de crear la rama: manda el del proyecto
	root, _ = pprTarjeta(t, "", "", 300, 2)
	bdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nspec:\n  topes:\n    normal: { palabras: 100 }\n")
	if c := vdtCardFor(t, ca, root); c.Column != ColArquitecto || !pprTieneLinea(c.Missing, pprLineaPal(300, "normal", 100)) {
		t.Fatalf("%s: el tope sale del hoom.yaml del proyecto: %s %q", ca, c.Column, c.Missing)
	}
	// el tope de la tarjeta la saca de arquitecto
	root, _ = pprTarjeta(t, "", "tope_palabras: 1000\ntope_motivo: largo\n", 900, 2)
	if c := vdtCardFor(t, ca, root); c.Column != ColTuAprobacion {
		t.Fatalf("%s: con el tope de la tarjeta subido vuelve a tu-aprobacion: %s %q", ca, c.Column, c.Missing)
	}
}

// pprModeloDe es el modelo de nivel n en el provider p (la tabla del spec).
func pprModeloDe(p, n string) string {
	tabla := map[string]map[string]string{
		"claude": {"fuerte": "opus", "medio": "sonnet", "barato": "haiku"},
		"codex":  {"fuerte": "gpt-6-astra", "medio": "gpt-6.1-sol", "barato": "gpt-6-luna"},
	}
	return tabla[p][n]
}

// CA-804: la accion del test-writer en una tarjeta alto trae los valores de
// agents.test-writer.alto con origen `hoom.yaml, riesgo alto`; en una
// normal, los de agents.test-writer. Re-expresa CA-492.
func TestCA804_AccionConElRiesgoDeSuTarjeta(t *testing.T) {
	const ca = "CA-804"
	tw := manifest.RoleDefaults{Model: "medio", Effort: "low", Timeout: 45 * time.Minute,
		PorRiesgo: map[string]manifest.RoleDefaults{"alto": {Model: "fuerte", Effort: "high", Timeout: 2 * time.Hour}}}
	ev := vdtCon(acTestWriter(), "test-writer", tw)
	ev.Item.Riesgo = "alto"
	a := acAccion(t, ca, Derive(ev), ActPedirTestWriter)
	def := vdtDefault(t, ca, a)
	if a.Model != pprModeloDe(def, "fuerte") || a.Nivel != "fuerte" || a.Origen.Model != "hoom.yaml, riesgo alto" ||
		a.Effort != "high" || a.Origen.Effort != "hoom.yaml, riesgo alto" || a.TimeoutS != 7200 {
		t.Fatalf("%s: alto (provider %s): %+v", ca, def, a)
	}
	ev.Item.Riesgo = "normal"
	a = acAccion(t, ca, Derive(ev), ActPedirTestWriter)
	def = vdtDefault(t, ca, a)
	if a.Model != pprModeloDe(def, "medio") || a.Nivel != "medio" || a.Origen.Model != valores.OrigenHoomYAML ||
		a.Effort != "low" || a.TimeoutS != 2700 {
		t.Fatalf("%s: normal (provider %s): %+v", ca, def, a)
	}
	// el writer en una tarjeta alto con un nombre por riesgo
	w := manifest.RoleDefaults{Model: "m-rol", PorRiesgo: map[string]manifest.RoleDefaults{"alto": {Model: "m-alto"}}}
	ev = vdtCon(acWriter(), "writer", w)
	ev.Item.Riesgo = "alto"
	a = acAccion(t, ca, Derive(ev), ActPedirWriter)
	if a.Model != "m-alto" || a.Nivel != "" || a.Origen.Model != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: el writer alto: %+v", ca, a)
	}
	ev.Item.Riesgo = "bajo"
	if a := acAccion(t, ca, Derive(ev), ActPedirWriter); a.Model != "m-rol" || a.Origen.Model != valores.OrigenHoomYAML {
		t.Fatalf("%s: el writer bajo sin mapa bajo: %+v", ca, a)
	}
}

// CA-804: el presupuesto por riesgo de la accion no pasa lo que le queda a
// la tarjeta. Re-expresa CA-491.
func TestCA491_PresupuestoPorRiesgoTopeadoPorLoQueQueda(t *testing.T) {
	const ca = "CA-491"
	base := acConGasto(acWriter(), 5, 3) // quedan 2
	base.Item.Riesgo = "alto"
	w := manifest.RoleDefaults{BudgetUSD: bdF(1), PorRiesgo: map[string]manifest.RoleDefaults{"alto": {BudgetUSD: bdF(9)}}}
	a := acAccion(t, ca, Derive(vdtCon(base, "writer", w)), ActPedirWriter)
	acBudget(t, ca+" (9 del riesgo, quedan 2)", a, bdF(2))
	w.PorRiesgo["alto"] = manifest.RoleDefaults{BudgetUSD: bdF(1.5)}
	a = acAccion(t, ca, Derive(vdtCon(base, "writer", w)), ActPedirWriter)
	acBudget(t, ca+" (1.5 del riesgo, quedan 2)", a, bdF(1.5))
	if a.Origen.BudgetUSD != "hoom.yaml, riesgo alto" {
		t.Fatalf("%s: el presupuesto del riesgo tiene su origen: %+v", ca, a.Origen)
	}
}

// CA-805: Action.model trae el modelo traducido (nunca un nivel) y nivel en
// el JSON; un nombre no trae nivel en el JSON. Re-expresa CA-492.
func TestCA492_ActionTraeElModeloTraducidoYNivel(t *testing.T) {
	const ca = "CA-492"
	ev := vdtCon(acTestWriter(), "test-writer", manifest.RoleDefaults{Model: "barato"})
	a := acAccion(t, ca, Derive(ev), ActPedirTestWriter)
	def := vdtDefault(t, ca, a)
	if a.Model != pprModeloDe(def, "barato") || a.Nivel != "barato" {
		t.Fatalf("%s: model traducido para %s y nivel barato: %+v", ca, def, a)
	}
	for _, n := range []string{"fuerte", "medio", "barato"} {
		if a.Model == n {
			t.Fatalf("%s: ningun nivel llega sin traducir: %+v", ca, a)
		}
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"nivel":"barato"`) || !strings.Contains(string(raw), `"model":"`+a.Model+`"`) {
		t.Fatalf("%s: el JSON trae model y nivel: %s", ca, raw)
	}
	ev = vdtCon(acTestWriter(), "test-writer", manifest.RoleDefaults{Model: "Fuerte"})
	a = acAccion(t, ca, Derive(ev), ActPedirTestWriter)
	raw, _ = json.Marshal(a)
	if a.Model != "Fuerte" || a.Nivel != "" || strings.Contains(string(raw), `"nivel"`) {
		t.Fatalf("%s: Fuerte es un nombre: sin nivel, ni en el JSON: %s", ca, raw)
	}
}
