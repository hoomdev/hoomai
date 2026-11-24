// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// manifest: la seccion spec.topes (CA-792) y los valores por riesgo de
// agents.<rol> (CA-803, que re-expresa CA-490). Escritos desde el spec, las
// aclaraciones de la ronda 0 y las firmas publicas del esqueleto, sin leer la
// implementacion. Paquete externo para poder nombrar agents.Roles.
package manifest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/spec"
)

const pprBase = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

// pprDir deja un hoom.yaml con la base y extra.
func pprDir(t *testing.T, extra string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(pprBase+extra), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func pprCargar(t *testing.T, extra string) (*manifest.Manifest, error) {
	t.Helper()
	return manifest.Load(pprDir(t, extra), nil)
}

// pprInvalido exige que Load, LoadWithoutBaseCheck y LoadRoles fallen como
// hoom.yaml invalido con el texto exacto adentro.
func pprInvalido(t *testing.T, ca, extra, quiero string) {
	t.Helper()
	dir := pprDir(t, extra)
	cargas := map[string]func() error{
		"Load":                 func() error { _, err := manifest.Load(dir, nil); return err },
		"LoadWithoutBaseCheck": func() error { _, err := manifest.LoadWithoutBaseCheck(dir, nil); return err },
		"LoadRoles":            func() error { _, err := manifest.LoadRoles(dir); return err },
	}
	for nombre, cargar := range cargas {
		err := cargar()
		if err == nil {
			t.Fatalf("%s: %s de\n%s\ndebe fallar", ca, nombre, extra)
		}
		if !strings.Contains(err.Error(), "hoom.yaml invalido: ") || !strings.Contains(err.Error(), quiero) {
			t.Fatalf("%s: %s dice «hoom.yaml invalido: » y\n  %q\nfue\n  %q", ca, nombre, quiero, err.Error())
		}
	}
}

func pprUSD(x float64) *float64 { return &x }

var pprDefecto = map[string]spec.Tope{
	"bajo":   {Palabras: 400, Criterios: 4, CitasPorCriterio: 10},
	"normal": {Palabras: 800, Criterios: 8, CitasPorCriterio: 10},
	"alto":   {Palabras: 2000, Criterios: 15, CitasPorCriterio: 0},
}

// CA-792: sin la seccion, los topes son los de la tabla; TopesPorDefecto es
// una copia; "" se lee como normal; un manifiesto nil da los de defecto.
func TestCA792_SinSeccionLosTopesPorDefecto(t *testing.T) {
	const ca = "CA-792"
	if got := manifest.TopesPorDefecto(); !reflect.DeepEqual(got, pprDefecto) {
		t.Fatalf("%s: TopesPorDefecto es la tabla del spec: %+v", ca, got)
	}
	cop := manifest.TopesPorDefecto()
	cop["normal"] = spec.Tope{Palabras: 1}
	delete(cop, "alto")
	if got := manifest.TopesPorDefecto(); !reflect.DeepEqual(got, pprDefecto) {
		t.Fatalf("%s: TopesPorDefecto devuelve una copia: %+v", ca, got)
	}
	m, err := pprCargar(t, "")
	if err != nil {
		t.Fatalf("%s: sin spec: carga: %v", ca, err)
	}
	if m.Spec != nil {
		t.Fatalf("%s: sin la seccion Spec es nil: %+v", ca, m.Spec)
	}
	var nilm *manifest.Manifest
	for r, want := range pprDefecto {
		if got := m.TopeDelSpec(r); got != want {
			t.Fatalf("%s: TopeDelSpec(%s) sin seccion es %+v: %+v", ca, r, want, got)
		}
		if got := nilm.TopeDelSpec(r); got != want {
			t.Fatalf("%s: un manifiesto nil da los de defecto para %s: %+v", ca, r, got)
		}
	}
	if got := m.TopeDelSpec(""); got != pprDefecto["normal"] {
		t.Fatalf("%s: \"\" se lee como normal: %+v", ca, got)
	}
	if !reflect.DeepEqual(manifest.SpecKeys, []string{"topes"}) ||
		!reflect.DeepEqual(manifest.TopeKeys, []string{"palabras", "criterios", "citas_por_criterio"}) {
		t.Fatalf("%s: SpecKeys %v TopeKeys %v", ca, manifest.SpecKeys, manifest.TopeKeys)
	}
}

// CA-792: cada clave que falta toma su valor por defecto; 0 es sin tope y se
// acepta; un spec: o un spec.topes nulo no declara nada.
func TestCA792_ClavesQueFaltanYCero(t *testing.T) {
	const ca = "CA-792"
	m, err := pprCargar(t, "spec:\n  topes:\n    normal: { palabras: 1200 }\n    alto:\n      citas_por_criterio: 5\n    bajo: { palabras: 0, criterios: 0, citas_por_criterio: 0 }\n")
	if err != nil {
		t.Fatalf("%s: carga: %v", ca, err)
	}
	want := map[string]spec.Tope{
		"bajo":   {},
		"normal": {Palabras: 1200, Criterios: 8, CitasPorCriterio: 10},
		"alto":   {Palabras: 2000, Criterios: 15, CitasPorCriterio: 5},
	}
	if m.Spec == nil || !reflect.DeepEqual(m.Spec.Topes, want) {
		t.Fatalf("%s: Spec.Topes resuelve cada clave: %+v", ca, m.Spec)
	}
	for r, w := range want {
		if got := m.TopeDelSpec(r); got != w {
			t.Fatalf("%s: TopeDelSpec(%s) = %+v, no %+v", ca, r, got, w)
		}
	}
	// solo un riesgo: los otros dos con sus valores por defecto
	m, err = pprCargar(t, "spec:\n  topes:\n    alto: { palabras: 3000 }\n")
	if err != nil || m.TopeDelSpec("normal") != pprDefecto["normal"] || m.TopeDelSpec("bajo") != pprDefecto["bajo"] ||
		m.TopeDelSpec("alto") != (spec.Tope{Palabras: 3000, Criterios: 15}) {
		t.Fatalf("%s: un riesgo que falta toma sus valores por defecto: %v", ca, err)
	}
	for _, extra := range []string{"spec:\n", "spec: ~\n", "spec:\n  topes:\n", "spec:\n  topes: ~\n", "spec: {}\n"} {
		m, err := pprCargar(t, extra)
		if err != nil {
			t.Fatalf("%s: %q no declara nada y carga: %v", ca, extra, err)
		}
		for r, w := range pprDefecto {
			if got := m.TopeDelSpec(r); got != w {
				t.Fatalf("%s: con %q, TopeDelSpec(%s) es el de defecto: %+v", ca, extra, r, got)
			}
		}
	}
	// LoadRoles tambien lee la seccion
	r, err := manifest.LoadRoles(pprDir(t, "spec:\n  topes:\n    normal: { criterios: 3 }\n"))
	if err != nil || r.TopeDelSpec("normal") != (spec.Tope{Palabras: 800, Criterios: 3, CitasPorCriterio: 10}) {
		t.Fatalf("%s: LoadRoles lee spec.topes: %v", ca, err)
	}
}

// CA-792: los cinco errores con su texto exacto, en Load, LoadWithoutBaseCheck
// y LoadRoles.
func TestCA792_LosCincoErrores(t *testing.T) {
	const ca = "CA-792"
	for _, c := range []struct{ extra, quiero string }{
		{"spec:\n  tope: {}\n", `spec: clave desconocida "tope" (validas: topes)`},
		{"spec:\n  topes:\n    medio: { palabras: 3 }\n", `spec.topes: clave desconocida "medio" (validas: bajo, normal, alto)`},
		{"spec:\n  topes:\n    Normal: { palabras: 3 }\n", `spec.topes: clave desconocida "Normal" (validas: bajo, normal, alto)`},
		{"spec:\n  topes:\n    normal: { palabra: 3 }\n", `spec.topes.normal: clave desconocida "palabra" (validas: palabras, criterios, citas_por_criterio)`},
		{"spec:\n  topes:\n    alto: { citas: 3 }\n", `spec.topes.alto: clave desconocida "citas" (validas: palabras, criterios, citas_por_criterio)`},
		{"spec:\n  topes:\n    normal: { palabras: -1 }\n", `spec.topes.normal.palabras: -1 no es un entero mayor o igual que 0 (0 = sin tope)`},
		{"spec:\n  topes:\n    bajo: { criterios: ocho }\n", `spec.topes.bajo.criterios: ocho no es un entero mayor o igual que 0 (0 = sin tope)`},
		{"spec:\n  topes:\n    alto: { citas_por_criterio: 1.5 }\n", `spec.topes.alto.citas_por_criterio: 1.5 no es un entero mayor o igual que 0 (0 = sin tope)`},
		{"spec: 3\n", `spec debe ser un mapa`},
		{"spec: [topes]\n", `spec debe ser un mapa`},
		{"spec:\n  topes: [1, 2]\n", `spec.topes debe ser un mapa`},
		{"spec:\n  topes: alto\n", `spec.topes debe ser un mapa`},
		{"spec:\n  topes:\n    alto: 5\n", `spec.topes.alto debe ser un mapa`},
		{"spec:\n  topes:\n    normal: [800, 8]\n", `spec.topes.normal debe ser un mapa`},
	} {
		pprInvalido(t, ca, c.extra, c.quiero)
	}
	// un valor que no es escalar: el lugar y la regla (aclaracion 9)
	for _, extra := range []string{
		"spec:\n  topes:\n    normal: { palabras: [1] }\n",
		"spec:\n  topes:\n    normal: { palabras: { a: 1 } }\n",
	} {
		pprInvalido(t, ca, extra, "spec.topes.normal.palabras: ")
		pprInvalido(t, ca, extra, " no es un entero mayor o igual que 0 (0 = sin tope)")
	}
}

// CA-803: agents.<rol> acepta bajo, normal y alto con model, effort,
// budget_usd y timeout; un mapa nulo o vacio no deja entrada; Provider es
// siempre ""; RoleDefaults los trae en PorRiesgo, como copia.
func TestCA803_ValoresPorRiesgoCargan(t *testing.T) {
	const ca = "CA-803"
	m, err := pprCargar(t, "agents:\n"+
		"  writer:\n"+
		"    provider: claude\n"+
		"    model: medio\n"+
		"    alto: { model: fuerte, effort: high, budget_usd: 9, timeout: 2h }\n"+
		"    bajo:\n"+
		"      model: barato\n"+
		"      budget_usd: 0\n"+
		"    normal: {}\n"+
		"  test-writer:\n"+
		"    alto: ~\n"+
		"    normal: { effort: low }\n")
	if err != nil {
		t.Fatalf("%s: carga: %v", ca, err)
	}
	w := m.RoleDefaults("writer")
	if w.Provider != "claude" || w.Model != "medio" {
		t.Fatalf("%s: agents.writer sigue igual: %+v", ca, w)
	}
	if len(w.PorRiesgo) != 2 {
		t.Fatalf("%s: alto y bajo, sin entrada para normal vacio: %+v", ca, w.PorRiesgo)
	}
	alto := w.PorRiesgo["alto"]
	if alto.Provider != "" || alto.Model != "fuerte" || alto.Effort != "high" || alto.BudgetUSD == nil || *alto.BudgetUSD != 9 || alto.Timeout != 2*time.Hour || alto.PorRiesgo != nil {
		t.Fatalf("%s: agents.writer.alto: %+v", ca, alto)
	}
	bajo := w.PorRiesgo["bajo"]
	if bajo.Model != "barato" || bajo.BudgetUSD == nil || *bajo.BudgetUSD != 0 || bajo.Effort != "" || bajo.Timeout != 0 {
		t.Fatalf("%s: agents.writer.bajo (budget 0 declarado sin tope): %+v", ca, bajo)
	}
	tw := m.RoleDefaults("test-writer")
	if len(tw.PorRiesgo) != 1 || tw.PorRiesgo["normal"].Effort != "low" {
		t.Fatalf("%s: alto nulo no deja entrada: %+v", ca, tw.PorRiesgo)
	}
	// copia
	w.PorRiesgo["alto"] = manifest.RoleDefaults{Model: "otro"}
	delete(w.PorRiesgo, "bajo")
	if again := m.RoleDefaults("writer"); again.PorRiesgo["alto"].Model != "fuerte" || len(again.PorRiesgo) != 2 {
		t.Fatalf("%s: RoleDefaults devuelve una copia de PorRiesgo: %+v", ca, again.PorRiesgo)
	}
	// AgentPolicy.PorRiesgo
	if p := m.Agents["writer"]; p.PorRiesgo["alto"].Model != "fuerte" || p.PorRiesgo["alto"].Provider != "" {
		t.Fatalf("%s: AgentPolicy.PorRiesgo: %+v", ca, p.PorRiesgo)
	}
	// el reviewer no tiene valores por riesgo
	if rv := m.RoleDefaults("reviewer"); rv.PorRiesgo != nil {
		t.Fatalf("%s: el reviewer no tiene PorRiesgo: %+v", ca, rv)
	}
	if !reflect.DeepEqual(manifest.AgentRiesgoKeys, []string{"model", "effort", "budget_usd", "timeout"}) {
		t.Fatalf("%s: AgentRiesgoKeys: %v", ca, manifest.AgentRiesgoKeys)
	}
	// LoadRoles los lee igual
	r, err := manifest.LoadRoles(pprDir(t, "agents:\n  arquitecto:\n    alto: { model: fuerte }\n"))
	if err != nil || r.RoleDefaults("arquitecto").PorRiesgo["alto"].Model != "fuerte" {
		t.Fatalf("%s: LoadRoles lee agents.<rol>.<r>: %v", ca, err)
	}
}

// CA-803: los errores exactos de los mapas por riesgo; provider no varia por
// riesgo; los valores siguen las reglas de agents.<rol> con su lugar.
func TestCA803_ErroresDeLosMapasPorRiesgo(t *testing.T) {
	const ca = "CA-803"
	claves := "(validas: model, effort, budget_usd, timeout)"
	for _, c := range []struct{ extra, quiero string }{
		{"agents:\n  writer:\n    alto: { provider: codex }\n", `agents.writer.alto: clave desconocida "provider" ` + claves},
		{"agents:\n  writer:\n    bajo: { write: { allow: [x] } }\n", `agents.writer.bajo: clave desconocida "write" ` + claves},
		{"agents:\n  test-writer:\n    normal: { efort: low }\n", `agents.test-writer.normal: clave desconocida "efort" ` + claves},
		{"agents:\n  writer:\n    alto: { alto: { model: x } }\n", `agents.writer.alto: clave desconocida "alto" ` + claves},
		{"agents:\n  writer:\n    alto: fuerte\n", `agents.writer.alto debe ser un mapa (ej. alto: { model: fuerte })`},
		{"agents:\n  arquitecto:\n    bajo: [model, fuerte]\n", `agents.arquitecto.bajo debe ser un mapa (ej. alto: { model: fuerte })`},
		{"agents:\n  writer:\n    alto: { timeout: 30 }\n", `agents.writer.alto.timeout: "30" no es una duracion entre 1m y 12h (ej. 45m, 1h30m)`},
		{"agents:\n  writer:\n    alto: { budget_usd: -1 }\n", `agents.writer.alto.budget_usd: `},
	} {
		pprInvalido(t, ca, c.extra, c.quiero)
	}
}

// CA-803: un rol desconocido falla al cargar con la lista de agents.Roles en
// su orden; agents.reviewer sigue rechazando toda clave de lanzamiento,
// tambien bajo, normal y alto.
func TestCA803_RolDesconocidoYReviewer(t *testing.T) {
	const ca = "CA-803"
	var slugs []string
	for _, r := range agents.Roles() {
		slugs = append(slugs, r.Slug)
	}
	validos := "(validos: " + strings.Join(slugs, ", ") + ")"
	for _, rol := range []string{"writr", "test_writer", "hoom-writer", "Writer"} {
		pprInvalido(t, ca, "agents:\n  "+rol+":\n    model: medio\n", `agents: rol "`+rol+`" desconocido `+validos)
	}
	for _, r := range []string{"bajo", "normal", "alto"} {
		pprInvalido(t, ca, "agents:\n  reviewer:\n    "+r+": { model: fuerte }\n",
			"agents.reviewer: la review se configura en review: (provider, model, effort, timeout)")
	}
	// los roles del bloque de hoom init se aceptan con su mapa alto
	var b strings.Builder
	b.WriteString("agents:\n")
	for _, s := range []string{"arquitecto", "designer", "scout", "writer", "test-writer", "characterizer", "analista", "refutador"} {
		b.WriteString("  " + s + ":\n    model: medio\n    alto: { model: fuerte }\n")
	}
	if _, err := pprCargar(t, b.String()); err != nil {
		t.Fatalf("%s: los roles de agents.Roles valen: %v", ca, err)
	}
}

// CA-803: los niveles valen donde vale un modelo (agents.<rol>.model,
// agents.<rol>.<r>.model, review.model); `Fuerte` pasa como nombre.
func TestCA803_NivelesDondeValeUnModelo(t *testing.T) {
	const ca = "CA-803"
	m, err := pprCargar(t, "agents:\n  writer:\n    model: Fuerte\n    alto: { model: barato }\nreview:\n  model: fuerte\n")
	if err != nil {
		t.Fatalf("%s: niveles y nombres cargan: %v", ca, err)
	}
	if w := m.RoleDefaults("writer"); w.Model != "Fuerte" || w.PorRiesgo["alto"].Model != "barato" {
		t.Fatalf("%s: el modelo se lee tal cual, sin traducir al cargar: %+v", ca, w)
	}
	if rv := m.RoleDefaults("reviewer"); rv.Model != "fuerte" {
		t.Fatalf("%s: review.model fuerte: %+v", ca, rv)
	}
}

// CA-490 (re-expresado por CA-803): agents.<rol> acepta las claves por
// riesgo, su lista de claves validas las suma, y un rol desconocido falla;
// un hoom.yaml sin estas claves carga igual que hoy, sin PorRiesgo.
func TestCA490_ClavesPorRiesgoEnLaListaYRolDesconocido(t *testing.T) {
	const ca = "CA-490"
	pprInvalido(t, ca, "agents:\n  writer:\n    efort: low\n",
		`agents.writer: clave desconocida "efort" (validas: provider, model, effort, budget_usd, timeout, write, bajo, normal, alto)`)
	pprInvalido(t, ca, "agents:\n  writr:\n    effort: low\n", `agents: rol "writr" desconocido (validos: `)
	if _, err := pprCargar(t, "agents:\n  writer:\n    bajo: { effort: low }\n    normal: { effort: medium }\n    alto: { effort: high }\n"); err != nil {
		t.Fatalf("%s: las claves por riesgo valen en agents.<rol>: %v", ca, err)
	}
	for _, extra := range []string{"", "agents:\n  test-writer:\n    write: { allow: [\"tests/**\"] }\n", "agents: {}\n", "agents:\n  writer:\n    model: m\n"} {
		m, err := pprCargar(t, extra)
		if err != nil {
			t.Fatalf("%s: %q carga como hoy: %v", ca, extra, err)
		}
		for _, rol := range []string{"arquitecto", "test-writer", "writer", "reviewer"} {
			if rd := m.RoleDefaults(rol); rd.PorRiesgo != nil {
				t.Fatalf("%s: %q: %s sin claves por riesgo no tiene PorRiesgo: %+v", ca, extra, rol, rd)
			}
		}
	}
}
