// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre el
// veredicto: no_aplica no pone rojo ni hace parcial al veredicto y cuenta en
// summary.no_aplica (CA-810); un veredicto sin no_aplica se serializa igual
// que hoy, sin la clave no_aplica (CA-815, aclaracion 11).
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package verdict_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

var gprT0 = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

const gprMotivo = "ningun archivo del cambio coincide con aplica_en (app/**)"

// gprNoAplica es un gate requerido en no_aplica, como lo deja un verify de
// trabajo.
func gprNoAplica(nombre string) verdict.GateResult {
	return verdict.GateResult{Name: nombre, Required: true, Status: verdict.StatusNoAplica, Scope: "none", Notes: gprMotivo}
}

func gprPass(nombre string) verdict.GateResult {
	return verdict.GateResult{Name: nombre, Required: true, Status: verdict.StatusPass, Scope: "full", Command: "true"}
}

// gprClavesDelSummary es el conjunto de claves de summary en el JSON de v.
func gprClavesDelSummary(t *testing.T, v *verdict.Verdict) (map[string]int, []string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Summary map[string]int `json:"summary"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("el veredicto es JSON: %v\n%s", err, raw)
	}
	var claves []string
	for k := range doc.Summary {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	return doc.Summary, claves
}

// CA-810: dos gates requeridos en no_aplica no ponen rojo al veredicto ni lo
// hacen parcial: verde, summary.no_aplica 2 y los demas contadores en lo
// suyo; LatestComplete lo da aunque haya un parcial mas nuevo.
func TestCA810_NoAplicaNoPoneRojoNiParcial(t *testing.T) {
	const ca = "CA-810"
	v := &verdict.Verdict{ID: "v-trabajo", CreatedAt: gprT0,
		Gates: []verdict.GateResult{gprPass("test"), gprNoAplica("static"), gprNoAplica("mutation")}}
	v.Finalize()
	if v.Verdict != "green" {
		t.Fatalf("%s: no_aplica, requerido o no, no pone rojo: %s %+v", ca, v.Verdict, v.Summary)
	}
	if want := (verdict.Summary{Pass: 1, NoAplica: 2}); v.Summary != want {
		t.Fatalf("%s: summary %+v, fue %+v", ca, want, v.Summary)
	}
	if v.Partial || v.IsPartial() {
		t.Fatalf("%s: no_aplica no es skipped: el veredicto es completo", ca)
	}
	parcial := &verdict.Verdict{ID: "v-parcial", CreatedAt: gprT0.Add(time.Minute), Partial: true,
		Gates: []verdict.GateResult{gprPass("test"), {Name: "static", Required: true, Status: verdict.StatusSkipped}}}
	parcial.Finalize()
	if got := verdict.LatestComplete([]*verdict.Verdict{v, parcial}); got != v {
		t.Fatalf("%s: LatestComplete da el veredicto con no_aplica: %+v", ca, got)
	}
}

// CA-815 y aclaracion 11: un veredicto sin no_aplica se serializa igual que
// hoy: summary sin la clave no_aplica, con las cinco de siempre. Con dos
// no_aplica la clave va con 2.
func TestCA815_SinNoAplicaSeSerializaComoHoy(t *testing.T) {
	const ca = "CA-815"
	hoy := &verdict.Verdict{ID: "v-hoy", CreatedAt: gprT0, Schema: verdict.SchemaID,
		Gates: []verdict.GateResult{gprPass("test"), {Name: "lint", Status: verdict.StatusAbsent, Scope: "none"}}}
	hoy.Finalize()
	_, claves := gprClavesDelSummary(t, hoy)
	if want := []string{"absent", "error", "fail", "pass", "skipped"}; !reflect.DeepEqual(claves, want) {
		t.Fatalf("%s: sin no_aplica, summary tiene las claves de hoy %q, fue %q", ca, want, claves)
	}

	con := &verdict.Verdict{ID: "v-con", CreatedAt: gprT0, Schema: verdict.SchemaID,
		Gates: []verdict.GateResult{gprPass("test"), gprNoAplica("static"), gprNoAplica("mutation")}}
	con.Finalize()
	summary, _ := gprClavesDelSummary(t, con)
	if n, ok := summary["no_aplica"]; !ok || n != 2 {
		t.Fatalf("%s: con dos no_aplica, summary.no_aplica es 2: %+v", ca, summary)
	}
}
