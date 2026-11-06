// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md en el
// tablero (CA-491, CA-492, CA-497, CA-513): las acciones de rol traen model,
// effort y timeout_s segun la regla de los valores (comando, hoom.yaml del
// PROYECTO, propuesta de la tarjeta, provider), el provider default es el de
// hoom.yaml cuando es una opcion con ok, el presupuesto de hoom.yaml nunca
// pasa lo que le queda a la tarjeta, el test-writer no se pide con el spec
// sin guardar, y Next y el texto del tablero no cambian.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/valores"
)

// vdtCon devuelve ev con los valores de hoom.yaml de un rol.
func vdtCon(ev Evidence, rol string, d manifest.RoleDefaults) Evidence {
	m := map[string]manifest.RoleDefaults{}
	for k, v := range ev.Defaults {
		m[k] = v
	}
	m[rol] = d
	ev.Defaults = m
	return ev
}

// vdtSpecDelEspacio es la ruta (relativa al proyecto) del spec de la tarjeta
// dentro de su espacio de trabajo, como la reportan las rutas sin guardar.
const vdtSpecDelEspacio = bdWtDir + "/" + bdSpec

// vdtDefault es el nombre de la opcion de provider con default: true.
func vdtDefault(t *testing.T, ca string, a Action) string {
	t.Helper()
	def := ""
	for _, o := range a.Providers {
		if o.Default {
			if def != "" {
				t.Fatalf("%s: %s tiene dos providers default: %+v", ca, a.ID, a.Providers)
			}
			if !o.OK {
				t.Fatalf("%s: %s: el default tiene que ser una opcion con ok: %+v", ca, a.ID, o)
			}
			def = o.Name
		}
	}
	return def
}

// vdtValores exige model, effort y timeout_s de una accion.
func vdtValores(t *testing.T, ca string, a Action, model, effort string, timeoutS int) {
	t.Helper()
	if a.Model != model || a.Effort != effort || a.TimeoutS != timeoutS {
		t.Fatalf("%s: %s trae model %q, effort %q, timeout_s %d; fue %q, %q, %d",
			ca, a.ID, model, effort, timeoutS, a.Model, a.Effort, a.TimeoutS)
	}
}

// CA-492: sin nada declarado, las acciones de rol traen model y effort
// vacios (los del provider) y timeout_s 1800, con origen provider.
func TestCA492_SinNadaDeclaradoLosDelProviderY1800(t *testing.T) {
	casos := []struct {
		nombre string
		ev     Evidence
		id     string
	}{
		{"arquitecto", func() Evidence { ev := acEv(); ev.SpecExists = false; return ev }(), ActPedirArquitecto},
		{"test-writer", acTestWriter(), ActPedirTestWriter},
		{"writer", acWriter(), ActPedirWriter},
		{"reviewer", acGrande(acEv()), ActPedirReviewer},
		{"reanudar", acInterrumpido(acWriter(), "writer", "claude"), ActReanudar},
		{"relanzar", acInterrumpido(acWriter(), "writer", "claude"), ActRelanzar},
	}
	for _, c := range casos {
		a := acAccion(t, "CA-492", Derive(c.ev), c.id)
		vdtValores(t, "CA-492 ("+c.nombre+")", a, "", "", 1800)
		if a.Origen.Model != valores.OrigenProvider || a.Origen.Effort != valores.OrigenProvider ||
			a.Origen.Timeout != valores.OrigenProvider {
			t.Fatalf("CA-492 (%s): sin nada declarado, modelo, esfuerzo y tiempo limite vienen del provider: %+v", c.nombre, a.Origen)
		}
	}
}

// CA-492: con agents.<rol> declarado, la accion de ese rol trae su model,
// effort y timeout_s, con origen hoom.yaml; otro rol no los toma.
func TestCA492_AccionTraeLosValoresDeSuRol(t *testing.T) {
	tw := manifest.RoleDefaults{Model: "opus", Effort: "high", Timeout: 45 * time.Minute}
	a := acAccion(t, "CA-492", Derive(vdtCon(acTestWriter(), "test-writer", tw)), ActPedirTestWriter)
	vdtValores(t, "CA-492", a, "opus", "high", 2700)
	if a.Origen.Model != valores.OrigenHoomYAML || a.Origen.Effort != valores.OrigenHoomYAML ||
		a.Origen.Timeout != valores.OrigenHoomYAML {
		t.Fatalf("CA-492: los valores de agents.test-writer tienen origen hoom.yaml: %+v", a.Origen)
	}

	// los del writer no se le pegan al test-writer
	a = acAccion(t, "CA-492", Derive(vdtCon(acTestWriter(), "writer", tw)), ActPedirTestWriter)
	vdtValores(t, "CA-492 (otro rol)", a, "", "", 1800)

	// el arquitecto, con sus propios valores
	ev := acEv()
	ev.SpecExists = false
	ar := manifest.RoleDefaults{Model: "m-arq", Effort: "low", Timeout: 90 * time.Minute}
	vdtValores(t, "CA-492 (arquitecto)", acAccion(t, "CA-492", Derive(vdtCon(ev, "arquitecto", ar)), ActPedirArquitecto),
		"m-arq", "low", 5400)

	// solo el tiempo limite declarado: model y effort siguen del provider
	a = acAccion(t, "CA-492", Derive(vdtCon(acWriter(), "writer", manifest.RoleDefaults{Timeout: 2 * time.Hour})), ActPedirWriter)
	vdtValores(t, "CA-492 (solo timeout)", a, "", "", 7200)
	if a.Origen.Model != valores.OrigenProvider || a.Origen.Timeout != valores.OrigenHoomYAML {
		t.Fatalf("CA-492: cada valor tiene su origen: %+v", a.Origen)
	}
}

// CA-492: el reviewer toma de review: (Defaults["reviewer"]) su model,
// effort y tiempo limite.
func TestCA492_ReviewerTomaLosDeReview(t *testing.T) {
	d := manifest.RoleDefaults{Model: "gpt-5", Effort: "high", Timeout: 60 * time.Minute}
	a := acAccion(t, "CA-492", Derive(vdtCon(acGrande(acEv()), "reviewer", d)), ActPedirReviewer)
	vdtValores(t, "CA-492 (reviewer)", a, "gpt-5", "high", 3600)
}

// CA-492: reanudar y relanzar usan los valores del rol interrumpido.
func TestCA492_ReanudarYRelanzarUsanElTiempoDelRol(t *testing.T) {
	d := manifest.RoleDefaults{Timeout: 45 * time.Minute}
	ev := vdtCon(acInterrumpido(acWriter(), "writer", "claude"), "writer", d)
	c := Derive(ev)
	if got := acAccion(t, "CA-492", c, ActReanudar).TimeoutS; got != 2700 {
		t.Fatalf("CA-492: reanudar al writer usa agents.writer.timeout (2700 s): %d", got)
	}
	if got := acAccion(t, "CA-492", c, ActRelanzar).TimeoutS; got != 2700 {
		t.Fatalf("CA-492: relanzar al writer usa agents.writer.timeout (2700 s): %d", got)
	}
}

// CA-492: el provider default es el de agents.<rol>.provider cuando es una
// opcion con ok; si no lo es (no recibe el contrato, no esta instalado, no
// puede revisar), el de hoy: el primero que puede.
func TestCA492_ProviderDefaultDeHoomYaml(t *testing.T) {
	casos := []struct {
		nombre, prov, want string
	}{
		{"codex declarado y con ok", "codex", "codex"},
		{"claude declarado y con ok", "claude", "claude"},
		{"opencode no recibe el contrato", "opencode", "claude"},
		{"gemini no esta instalado", "gemini", "claude"},
		{"sin declarar", "", "claude"},
	}
	for _, c := range casos {
		ev := vdtCon(acWriter(), "writer", manifest.RoleDefaults{Provider: c.prov})
		a := acAccion(t, "CA-492", Derive(ev), ActPedirWriter)
		if got := vdtDefault(t, "CA-492", a); got != c.want {
			t.Fatalf("CA-492 (%s): el default de pedir-writer es %q, fue %q: %+v", c.nombre, c.want, got, a.Providers)
		}
		if c.prov == c.want && c.prov != "" && a.Origen.Provider != valores.OrigenHoomYAML {
			t.Fatalf("CA-492 (%s): el provider de hoom.yaml tiene origen hoom.yaml: %+v", c.nombre, a.Origen)
		}
		if c.prov != c.want && a.Origen.Provider != valores.OrigenTarjeta {
			t.Fatalf("CA-492 (%s): el provider que propone la tarjeta tiene origen tarjeta: %+v", c.nombre, a.Origen)
		}
	}

	// el reviewer: review.provider codex gana al primero que puede (claude)
	a := acAccion(t, "CA-492", Derive(vdtCon(acGrande(acEv()), "reviewer", manifest.RoleDefaults{Provider: "codex"})), ActPedirReviewer)
	if got := vdtDefault(t, "CA-492", a); got != "codex" {
		t.Fatalf("CA-492: review.provider codex es el default de pedir-reviewer: %q %+v", got, a.Providers)
	}
	// review.provider claude, pero claude escribio el codigo: el de hoy
	ev := acGrande(acEv())
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
	a = acAccion(t, "CA-492", Derive(vdtCon(ev, "reviewer", manifest.RoleDefaults{Provider: "claude"})), ActPedirReviewer)
	if got := vdtDefault(t, "CA-492", a); got != "codex" {
		t.Fatalf("CA-492: un review.provider que no puede revisar no es default; el de hoy (codex): %q %+v", got, a.Providers)
	}
}

// CA-492: model, effort y timeout_s estan en el JSON de TODA accion (aunque
// no lance un rol), y en las de rol timeout_s nunca es 0.
func TestCA492_ClavesEnElJSONDeTodaAccion(t *testing.T) {
	for _, c := range acColumnas() {
		card := Derive(c.ev)
		m, raw := acJSON(t, card)
		acts, _ := m["actions"].([]any)
		for _, x := range acts {
			a, _ := x.(map[string]any)
			for _, k := range []string{"model", "effort", "timeout_s"} {
				if _, ok := a[k]; !ok {
					t.Fatalf("CA-492 (%s): la accion %v no trae %q en el JSON:\n%s", c.nombre, a["id"], k, raw)
				}
			}
			if id, _ := a["id"].(string); acDeRol[id] {
				if v, _ := a["timeout_s"].(float64); v != 1800 {
					t.Fatalf("CA-492 (%s): %s sin nada declarado trae timeout_s 1800: %v", c.nombre, id, a["timeout_s"])
				}
			}
		}
	}
}

// CA-491: el presupuesto de hoom.yaml nunca pasa lo que le queda a la
// tarjeta; menos de lo que queda gana con origen hoom.yaml; sin presupuesto
// en la tarjeta es el de hoom.yaml.
func TestCA491_PresupuestoDeHoomYamlTopeadoPorLoQueQueda(t *testing.T) {
	// quedan 2 USD (5 - 3)
	base := acConGasto(acWriter(), 5, 3)
	a := acAccion(t, "CA-491", Derive(vdtCon(base, "writer", manifest.RoleDefaults{BudgetUSD: bdF(5)})), ActPedirWriter)
	acBudget(t, "CA-491 (5 declarados, quedan 2)", a, bdF(2))

	a = acAccion(t, "CA-491", Derive(vdtCon(base, "writer", manifest.RoleDefaults{BudgetUSD: bdF(1)})), ActPedirWriter)
	acBudget(t, "CA-491 (1 declarado, quedan 2)", a, bdF(1))
	if a.Origen.BudgetUSD != valores.OrigenHoomYAML {
		t.Fatalf("CA-491: un budget_usd menor que lo que queda gana, con origen hoom.yaml: %+v", a.Origen)
	}

	// sin declarar: lo que queda, propuesta de la tarjeta
	a = acAccion(t, "CA-491", Derive(base), ActPedirWriter)
	acBudget(t, "CA-491 (sin declarar)", a, bdF(2))
	if a.Origen.BudgetUSD != valores.OrigenTarjeta {
		t.Fatalf("CA-491: lo que queda es la propuesta de la tarjeta (origen tarjeta): %+v", a.Origen)
	}

	// sin presupuesto en la tarjeta: el de hoom.yaml
	a = acAccion(t, "CA-491", Derive(vdtCon(acWriter(), "writer", manifest.RoleDefaults{BudgetUSD: bdF(3)})), ActPedirWriter)
	acBudget(t, "CA-491 (sin presupuesto en la tarjeta)", a, bdF(3))
	if a.Origen.BudgetUSD != valores.OrigenHoomYAML {
		t.Fatalf("CA-491: el budget_usd de hoom.yaml tiene origen hoom.yaml: %+v", a.Origen)
	}
}

// CA-491: budget_usd 0 es sin tope declarado: con presupuesto en la tarjeta
// la propuesta sigue siendo lo que queda; sin el, sin tope.
func TestCA491_BudgetCeroEsSinTope(t *testing.T) {
	cero := manifest.RoleDefaults{BudgetUSD: bdF(0)}
	a := acAccion(t, "CA-491", Derive(vdtCon(acConGasto(acWriter(), 5, 3), "writer", cero)), ActPedirWriter)
	acBudget(t, "CA-491 (0 declarado, quedan 2)", a, bdF(2))
	a = acAccion(t, "CA-491", Derive(vdtCon(acWriter(), "writer", cero)), ActPedirWriter)
	acBudget(t, "CA-491 (0 declarado, sin presupuesto)", a, nil)
}

// CA-491: en la review la propuesta sigue siendo por lente, aunque review:
// declare provider, modelo o tiempo limite.
func TestCA491_ReviewSigueSiendoPorLente(t *testing.T) {
	ev := acGrande(acEv())
	ev.Item.PresupuestoUSD = bdF(5)
	ev.Runs = []RunState{acGasto("run-w", "writer", "codex", nil, 5*time.Minute),
		acGasto("run-s", "scout", "claude", bdF(3), 6*time.Minute)}
	d := manifest.RoleDefaults{Provider: "claude", Model: "opus", Timeout: time.Hour}
	a := acAccion(t, "CA-491", Derive(vdtCon(ev, "reviewer", d)), ActPedirReviewer)
	acBudget(t, "CA-491 (por lente)", a, bdF(0.5))
	acHabilitada(t, "CA-491", a)
}

// CA-497: pedir-test-writer esta deshabilitada con el why del contrato
// cuando el spec de la tarjeta tiene cambios sin guardar en su espacio de
// trabajo, y habilitada cuando no (aunque haya otras rutas sin guardar).
func TestCA497_TestWriterConElSpecSinGuardar(t *testing.T) {
	ev := acTestWriter()
	ev.Uncommitted = []string{vdtSpecDelEspacio}
	c := Derive(ev)
	acCol(t, "CA-497", "spec sin guardar", c, ColTestWriter)
	acDeshabilitada(t, "CA-497", acAccion(t, "CA-497", c, ActPedirTestWriter), WhySpecSinGuardar)

	// con otras rutas sin guardar, ademas
	ev = acTestWriter()
	ev.Uncommitted = append(acSucios(), vdtSpecDelEspacio)
	acDeshabilitada(t, "CA-497", acAccion(t, "CA-497", Derive(ev), ActPedirTestWriter), WhySpecSinGuardar)

	// el spec guardado y otras rutas sin guardar: habilitada
	ev = acTestWriter()
	ev.Uncommitted = acSucios()
	acHabilitada(t, "CA-497", acAccion(t, "CA-497", Derive(ev), ActPedirTestWriter))

	// todo guardado: habilitada
	acHabilitada(t, "CA-497", acAccion(t, "CA-497", Derive(acTestWriter()), ActPedirTestWriter))
}

// CA-497: relanzar con rol test-writer tambien; relanzar a otro rol no.
func TestCA497_RelanzarTestWriterConElSpecSinGuardar(t *testing.T) {
	ev := acInterrumpido(acTestWriter(), "test-writer", "claude")
	ev.Uncommitted = []string{vdtSpecDelEspacio}
	acDeshabilitada(t, "CA-497", acAccion(t, "CA-497", Derive(ev), ActRelanzar), WhySpecSinGuardar)

	ev = acInterrumpido(acTestWriter(), "test-writer", "claude")
	acHabilitada(t, "CA-497", acAccion(t, "CA-497", Derive(ev), ActRelanzar))

	ev = acInterrumpido(acWriter(), "writer", "claude")
	ev.Uncommitted = []string{vdtSpecDelEspacio}
	if a := acAccion(t, "CA-497", Derive(ev), ActRelanzar); a.Why == WhySpecSinGuardar {
		t.Fatalf("CA-497: relanzar al writer no depende del spec guardado: %+v", a)
	}
}

// CA-497: el why sigue la regla de los why: ninguna palabra de git.
func TestCA497_ElWhyNoTienePalabrasDeGit(t *testing.T) {
	if tbProhibidas.MatchString(WhySpecSinGuardar) {
		t.Fatalf("CA-497: el why no tiene palabras de git: %q", WhySpecSinGuardar)
	}
	if WhySpecSinGuardar != "el spec tiene cambios sin guardar y el test-writer trabaja con lo guardado: guarda primero" {
		t.Fatalf("CA-497: el texto del contrato: %q", WhySpecSinGuardar)
	}
}

// vdtTarjetaTestWriter arma, con git real, una tarjeta con espacio de
// trabajo cuyo spec esta aprobado en disco y sin prueba para CA-1.
func vdtTarjetaTestWriter(t *testing.T, extraYAML string) (root, wt string) {
	t.Helper()
	root = bdRepo(t, extraYAML)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt = bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo que probar.", true))
	bdAprobar(t, wt, bdSlug)
	return root, wt
}

func vdtCardFor(t *testing.T, ca, root string) Card {
	t.Helper()
	c, err := CardFor(root, "main", "high", bdSlug, time.Now().UTC())
	if err != nil {
		t.Fatalf("%s: CardFor: %v", ca, err)
	}
	return c
}

// CA-497 (git real): el spec del espacio de trabajo sin guardar nunca
// deshabilita; guardado, habilita; y lo guardado distinto de lo que hay en
// disco (aprobado en disco) vuelve a deshabilitar.
func TestCA497_ConGitReal(t *testing.T) {
	// un provider (claude, falso) instalado al frente del PATH, con git
	// todavia alcanzable: pedir-test-writer necesita un provider que tome el
	// trabajo y el runner de CI no tiene ninguna CLI de IA (hallazgo: CI rojo
	// en PR 61). Sin el, la accion queda deshabilitada por otro motivo.
	provs := t.TempDir()
	e7ClaudeFalso(t, provs)
	t.Setenv("PATH", provs+string(os.PathListSeparator)+os.Getenv("PATH"))

	root, wt := vdtTarjetaTestWriter(t, "")
	c := vdtCardFor(t, "CA-497", root)
	acCol(t, "CA-497", "spec nunca guardado", c, ColTestWriter)
	acDeshabilitada(t, "CA-497 (spec nunca guardado)", acAccion(t, "CA-497", c, ActPedirTestWriter), WhySpecSinGuardar)

	bdCommitear(t, wt, "spec y aprobacion")
	c = vdtCardFor(t, "CA-497", root)
	acCol(t, "CA-497", "spec guardado", c, ColTestWriter)
	acHabilitada(t, "CA-497 (spec guardado)", acAccion(t, "CA-497", c, ActPedirTestWriter))

	// otra version en disco, aprobada; la guardada es la vieja
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo que probar, distinto.", true))
	bdAprobar(t, wt, bdSlug)
	c = vdtCardFor(t, "CA-497", root)
	acCol(t, "CA-497", "guardado distinto del disco", c, ColTestWriter)
	acDeshabilitada(t, "CA-497 (guardado distinto del disco)", acAccion(t, "CA-497", c, ActPedirTestWriter), WhySpecSinGuardar)

	// codigo sin guardar con el spec guardado: habilitada
	bdCommitear(t, wt, "spec nuevo")
	bdEscribir(t, wt, "app.go", "package app // cambio\n")
	c = vdtCardFor(t, "CA-497", root)
	acHabilitada(t, "CA-497 (solo codigo sin guardar)", acAccion(t, "CA-497", c, ActPedirTestWriter))
}

// CA-491 (git real): la tarjeta toma los valores del hoom.yaml del
// proyecto aunque la rama de la tarjeta traiga otro.
func TestCA491_TarjetaTomaElHoomYamlDelProyecto(t *testing.T) {
	root, wt := vdtTarjetaTestWriter(t, "agents:\n  test-writer:\n    model: m-proyecto\n    effort: e-proyecto\n    timeout: 45m\n")
	raw, err := os.ReadFile(filepath.Join(wt, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rama := strings.NewReplacer("m-proyecto", "m-rama", "e-proyecto", "e-rama", "timeout: 45m", "timeout: 12h").Replace(string(raw))
	bdEscribir(t, wt, "hoom.yaml", rama)
	bdCommitear(t, wt, "la rama se sube sus valores")
	a := acAccion(t, "CA-491", vdtCardFor(t, "CA-491", root), ActPedirTestWriter)
	vdtValores(t, "CA-491 (hoom.yaml del proyecto)", a, "m-proyecto", "e-proyecto", 2700)
}

// CA-513: Next y el texto de la tarjeta no cambian con valores declarados
// que no tocan provider ni presupuesto.
func TestCA513_NextYTextoNoCambianConLosValores(t *testing.T) {
	d := manifest.RoleDefaults{Model: "opus", Effort: "high", Timeout: 45 * time.Minute}
	for _, c := range acColumnas() {
		ev := c.ev
		for _, rol := range []string{"arquitecto", "test-writer", "writer", "reviewer"} {
			ev = vdtCon(ev, rol, d)
		}
		sin, con := Derive(c.ev), Derive(ev)
		if sin.Next != con.Next {
			t.Fatalf("CA-513 (%s): next no cambia con hoom.yaml: %q vs %q", c.nombre, sin.Next, con.Next)
		}
		var a, b bytes.Buffer
		RenderCard(&a, sin)
		RenderCard(&b, con)
		if a.String() != b.String() {
			t.Fatalf("CA-513 (%s): el texto de la tarjeta no cambia con hoom.yaml:\n%s\nvs\n%s", c.nombre, a.String(), b.String())
		}
	}
}

// vdtAcciones devuelve las acciones (genericas) de la tarjeta slug en un
// JSON de 'hoom board --json'.
func vdtAccionesDelTablero(t *testing.T, raw, slug string) []map[string]any {
	t.Helper()
	var b struct {
		Columns []struct {
			Cards []map[string]any `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatalf("CA-492: 'hoom board --json' es JSON: %v", err)
	}
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if c["slug"] == slug {
				return vdtAccionesDe(c)
			}
		}
	}
	t.Fatalf("CA-492: el tablero no tiene la tarjeta %s:\n%s", slug, raw)
	return nil
}

func vdtAccionesDe(card map[string]any) []map[string]any {
	var out []map[string]any
	acts, _ := card["actions"].([]any)
	for _, x := range acts {
		if a, ok := x.(map[string]any); ok {
			out = append(out, a)
		}
	}
	return out
}

// vdtAccionesBien exige las tres claves en toda accion y los valores de
// agents.test-writer en pedir-test-writer.
func vdtAccionesBien(t *testing.T, ca, que string, acts []map[string]any) {
	t.Helper()
	vista := false
	for _, a := range acts {
		for _, k := range []string{"model", "effort", "timeout_s"} {
			if _, ok := a[k]; !ok {
				t.Fatalf("%s: %s: la accion %v no trae %q: %v", ca, que, a["id"], k, a)
			}
		}
		if a["id"] == ActPedirTestWriter {
			vista = true
			if a["model"] != "opus" || a["effort"] != "high" || a["timeout_s"] != float64(2700) {
				t.Fatalf("%s: %s: pedir-test-writer trae model opus, effort high, timeout_s 2700: %v", ca, que, a)
			}
		}
	}
	if !vista {
		t.Fatalf("%s: %s: la tarjeta en Test-writer tiene pedir-test-writer: %v", ca, que, acts)
	}
}

// CA-492 (binario): 'hoom board --json' y 'hoom item show --json' emiten
// model, effort y timeout_s.
func TestCA492_BoardEItemShowLosEmiten(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "agents:\n  test-writer:\n    model: opus\n    effort: high\n    timeout: 45m\n")
	const slug = "demo-valores"
	spec := ".hoom/specs/" + slug + ".md"
	bxOK(t, "CA-492", bxCorrer(t, bin, root, "item", "add", "Demo valores", "--pedido", "que la tarjeta proponga valores"), "item", "add")
	bxOK(t, "CA-492", bxCorrer(t, bin, root, "task", "start", slug), "task", "start")
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	bdEscribir(t, wt, spec, bdSpecTexto("- CA-1: la tarjeta propone valores.", true))
	bxOK(t, "CA-492", bxCorrer(t, bin, wt, "spec", "approve", spec), "spec", "approve")
	bdCommitear(t, wt, "spec y aprobacion")
	bxColumna(t, "CA-492", bin, root, slug, ColTestWriter)

	_, raw := bxBoard(t, bin, root)
	vdtAccionesBien(t, "CA-492", "hoom board --json", vdtAccionesDelTablero(t, raw, slug))

	r := bxCorrer(t, bin, root, "item", "show", slug, "--json")
	bxOK(t, "CA-492", r, "item", "show", slug, "--json")
	var show struct {
		Card map[string]any `json:"card"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &show); err != nil {
		t.Fatalf("CA-492: 'hoom item show --json' es JSON: %v\n%s", err, r.stdout)
	}
	vdtAccionesBien(t, "CA-492", "hoom item show --json", vdtAccionesDe(show.Card))
}
