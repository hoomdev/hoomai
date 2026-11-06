// Tests adversariales de la ronda 1 de la review de verbos-de-la-tarjeta en
// el Studio: hallazgos 896eff (el piso del provider con el presupuesto de
// hoom.yaml, CA-504), faff7c (POST /launch con la vista de lo que el dialogo
// mostro, CA-504) y 4a1c25, 810954 y 42550b (el origen de cada valor en
// /api/board y lo que acciones.js hace con el, CA-492 y CA-496). Los CLIs son
// los falsos de lanzar_test.go, en un PATH minimo.
package servecmd

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// ===========================================================================
// hallazgo 896eff (CA-504): el piso del provider con el presupuesto de hoom.yaml
// ===========================================================================

// hallazgo 896eff, CA-504: un presupuesto que sale de hoom.yaml
// (agents.writer.budget_usd: 0.1) por debajo del piso de claude se rechaza en
// POST /launch con 409 y el mismo motivo que la terminal, sin lanzar nada.
func TestHallazgo_896eff_CA504_LaunchConPresupuestoDeHoomYamlBajoElPiso(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := raProyectoConReview(t, "agents:\n  writer:\n    budget_usd: 0.1\n")
	lnCartaWriter(t, "hallazgo 896eff", root, "precios", "")
	a, ok := lnAccion(lnCarta(t, root, "precios"), boardcmd.ActPedirWriter)
	if !ok || a.BudgetUSD == nil || *a.BudgetUSD != 0.1 {
		t.Fatalf("hallazgo 896eff: fixture: pedir-writer propone el presupuesto de hoom.yaml (0.1): %+v", a)
	}
	s := lnServidor(t, root, f)
	antes := lnHuellas(t, root)
	rec := lnLaunch(t, s, "precios", s.Token(), lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec hasta que verify de verde."))
	lnEsperarLanzamientos(t, "hallazgo 896eff", s)
	if rec.Code != http.StatusConflict {
		t.Fatalf("hallazgo 896eff, CA-504: el presupuesto de hoom.yaml bajo el piso es un 409, fue %d: %s", rec.Code, rec.Body.String())
	}
	studio := vdtMotivoStudio(t, "hallazgo 896eff", rec)
	if !strings.Contains(studio, "claude necesita al menos 0.5 USD por run") {
		t.Fatalf("hallazgo 896eff, CA-504: el motivo dice %q: %q", "claude necesita al menos 0.5 USD por run", studio)
	}
	lnSinEfectos(t, "hallazgo 896eff", "un presupuesto de hoom.yaml bajo el piso", antes, lnHuellas(t, root), false)

	code, _, errT := vdtTerminal(t, "hallazgo 896eff", root, "pedir", "writer", "precios", "-m", "Implementa el spec hasta que verify de verde.")
	if code != 1 || !strings.Contains(errT, "hoom: "+studio) {
		t.Fatalf("hallazgo 896eff, CA-504: la terminal sale con 1 y el mismo motivo que el Studio (%q): %d\n%s", studio, code, errT)
	}
	lnSinEfectos(t, "hallazgo 896eff", "un presupuesto de hoom.yaml bajo el piso (terminal)", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "hallazgo 896eff", "un presupuesto de hoom.yaml bajo el piso", f)
}

// ===========================================================================
// hallazgo faff7c (CA-504): POST /launch con la vista del dialogo
// ===========================================================================

func r1Cambio(lista string) string {
	return "la tarjeta cambio mientras escribias el pedido (" + lista + "): no se lanzo nada, vuelve a pedirlo"
}

// r1Vista es lo que el dialogo mostro de la accion a: su rol, su provider
// default, modelo, esfuerzo, presupuesto, tiempo limite y la sesion que
// retoma, con las siete claves del contrato (el rol, hallazgo a8e103).
func r1Vista(a boardcmd.Action) map[string]any {
	prov := ""
	for _, p := range a.Providers {
		if p.Default {
			prov = p.Name
		}
	}
	var b any
	if a.BudgetUSD != nil {
		b = *a.BudgetUSD
	}
	return map[string]any{"role": a.Role, "provider": prov, "model": a.Model, "effort": a.Effort, "budget_usd": b,
		"timeout_s": a.TimeoutS, "resume_id": a.ResumeID}
}

func r1ConVista(cuerpo, vista map[string]any) map[string]any {
	cuerpo["vista"] = vista
	return cuerpo
}

func r1Copia(v map[string]any) map[string]any {
	out := map[string]any{}
	for k, x := range v {
		out[k] = x
	}
	return out
}

// hallazgo faff7c, CA-504: POST /launch con una vista distinta de lo que la
// accion propone ahora es un 409 que nombra lo que cambio (provider, modelo,
// esfuerzo, presupuesto, tiempo limite, sesion, en ese orden), sin efectos;
// con la vista igual, o sin vista, lanza como antes.
func TestHallazgo_faff7c_CA504_LaunchConVistaDistintaEs409(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "codex", false)
	root := raProyectoConReview(t, "agents:\n  writer:\n    model: m-yaml\n    effort: high\n    timeout: 45m\n")
	lnCartaWriter(t, "hallazgo faff7c", root, "precios", "presupuesto_usd: 5\n")
	a, ok := lnAccion(lnCarta(t, root, "precios"), boardcmd.ActPedirWriter)
	if !ok || !a.Enabled {
		t.Fatalf("hallazgo faff7c: fixture: pedir-writer habilitada: %+v", a)
	}
	base := r1Vista(a)
	if base["provider"] != "claude" || base["timeout_s"] != 2700 {
		t.Fatalf("hallazgo faff7c: fixture: la accion propone claude y 45m: %v", base)
	}
	s := lnServidor(t, root, f)
	cambia := map[string]func(v map[string]any){
		"provider":      func(v map[string]any) { v["provider"] = "codex" },
		"modelo":        func(v map[string]any) { v["model"] = "m-otro" },
		"esfuerzo":      func(v map[string]any) { v["effort"] = "low" },
		"presupuesto":   func(v map[string]any) { v["budget_usd"] = 4.5 },
		"tiempo limite": func(v map[string]any) { v["timeout_s"] = 3600 },
		"sesion":        func(v map[string]any) { v["resume_id"] = "otra-sesion" },
	}
	casos := [][]string{{"provider"}, {"modelo"}, {"esfuerzo"}, {"presupuesto"}, {"tiempo limite"}, {"sesion"},
		{"esfuerzo", "tiempo limite"}, {"provider", "modelo", "esfuerzo", "presupuesto", "tiempo limite", "sesion"}}
	for _, campos := range casos {
		v := r1Copia(base)
		for i := len(campos) - 1; i >= 0; i-- {
			cambia[campos[i]](v)
		}
		antes := lnHuellas(t, root)
		rec := lnLaunch(t, s, "precios", s.Token(), r1ConVista(lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec."), v))
		lnEsperarLanzamientos(t, "hallazgo faff7c", s)
		want := r1Cambio(strings.Join(campos, ", "))
		if rec.Code != http.StatusConflict {
			t.Fatalf("hallazgo faff7c, CA-504 (%v): una vista distinta es un 409, fue %d: %s", campos, rec.Code, rec.Body.String())
		}
		if got := vdtMotivoStudio(t, "hallazgo faff7c", rec); got != want {
			t.Fatalf("hallazgo faff7c, CA-504 (%v): el motivo es\n  %q\nfue\n  %q", campos, want, got)
		}
		lnSinEfectos(t, "hallazgo faff7c", "una vista distinta", antes, lnHuellas(t, root), false)
	}
	// una vista sin presupuesto cuando la accion propone uno tambien es distinta
	v := r1Copia(base)
	v["budget_usd"] = nil
	rec := lnLaunch(t, s, "precios", s.Token(), r1ConVista(lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec."), v))
	lnEsperarLanzamientos(t, "hallazgo faff7c", s)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "(presupuesto)") {
		t.Fatalf("hallazgo faff7c, CA-504: una vista sin presupuesto frente a 5 USD es un 409 (presupuesto): %d %s", rec.Code, rec.Body.String())
	}
	lnSinLlamadas(t, "hallazgo faff7c", "una vista distinta", f)

	// la vista igual lanza, con lo que se mostro
	lnAceptado(t, "hallazgo faff7c (vista igual)", lnLaunch(t, s, "precios", s.Token(),
		r1ConVista(lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec."), r1Copia(base))))
	lnEsperarLanzamientos(t, "hallazgo faff7c", s)
	ll := f.llamadas(t, "claude")
	if len(ll) != 1 {
		t.Fatalf("hallazgo faff7c: la vista igual lanza una vez: %d", len(ll))
	}
	if v, _ := lnSigue(ll[0], "--model"); v != "m-yaml" {
		t.Fatalf("hallazgo faff7c: se lanza lo que se mostro (m-yaml): %q", ll[0])
	}
	// sin vista, como antes
	lnAceptado(t, "hallazgo faff7c (sin vista)", lnLaunch(t, s, "precios", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec, otra vez.")))
	lnEsperarLanzamientos(t, "hallazgo faff7c", s)
	if n := len(f.llamadas(t, "claude")); n != 2 {
		t.Fatalf("hallazgo faff7c: sin vista se lanza como antes: %d llamadas", n)
	}
}

// hallazgo faff7c, CA-504: la carrera real del dialogo: la persona abre el
// dialogo con 5 USD restantes y, mientras escribe, otro run gasta 2; el POST
// con lo que se le mostro es un 409 (presupuesto) y no lanza nada.
func TestHallazgo_faff7c_CA504_ElPresupuestoCambioMientrasEscribias(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wt := lnCartaWriter(t, "hallazgo faff7c", root, "precios", "presupuesto_usd: 5\n")
	a, _ := lnAccion(lnCarta(t, root, "precios"), boardcmd.ActPedirWriter)
	if a.BudgetUSD == nil || *a.BudgetUSD != 5 {
		t.Fatalf("hallazgo faff7c: fixture: la accion propone 5 USD: %+v", a.BudgetUSD)
	}
	vista := r1Vista(a)
	s := lnServidor(t, root, f)
	lnGasto(t, "hallazgo faff7c", root, "precios", wt, 2)
	antes := lnHuellas(t, root)
	rec := lnLaunch(t, s, "precios", s.Token(), r1ConVista(lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec."), vista))
	lnEsperarLanzamientos(t, "hallazgo faff7c", s)
	if rec.Code != http.StatusConflict {
		t.Fatalf("hallazgo faff7c, CA-504: el presupuesto cambio mientras se escribia el pedido: 409, fue %d: %s", rec.Code, rec.Body.String())
	}
	if got := vdtMotivoStudio(t, "hallazgo faff7c", rec); got != r1Cambio("presupuesto") {
		t.Fatalf("hallazgo faff7c, CA-504: el motivo es %q: %q", r1Cambio("presupuesto"), got)
	}
	lnSinEfectos(t, "hallazgo faff7c", "un presupuesto que cambio", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "hallazgo faff7c", "un presupuesto que cambio", f)
}

// hallazgo faff7c, CA-504: el POST del dialogo de rol manda la vista
// (vista: vistaDe(a)), y vistaDe arma las seis claves del contrato.
func TestHallazgo_faff7c_AccionesJSMandaLaVista(t *testing.T) {
	_, _, acc := atUI(t)
	js := c2SinComentarios(acc)
	if !regexp.MustCompile(`\bvista\s*:\s*vistaDe\(`).MatchString(js) {
		t.Fatal("hallazgo faff7c: el POST de launch de acciones.js lleva vista: vistaDe(a)")
	}
	cuerpo := h2Cuerpo(js, "vistaDe")
	if cuerpo == "" {
		t.Fatal("hallazgo faff7c: acciones.js declara la funcion vistaDe")
	}
	for _, k := range []string{"provider", "model", "effort", "budget_usd", "timeout_s", "resume_id"} {
		if !regexp.MustCompile(`\b` + k + `\b`).MatchString(cuerpo) {
			t.Fatalf("hallazgo faff7c: vistaDe arma la clave %q:\n%s", k, cuerpo)
		}
	}
}

// ===========================================================================
// hallazgos 4a1c25, 810954 y 42550b (CA-492, CA-496): el origen de cada valor
// ===========================================================================

var r1Origenes = map[string]bool{"comando": true, "hoom.yaml": true, "tarjeta": true, "provider": true}

// r1AccionDelTablero devuelve la accion id de la tarjeta slug tal como la
// emite /api/board, y exige que toda accion de rol del tablero traiga origen
// con sus cinco claves en el vocabulario del contrato.
func r1AccionDelTablero(t *testing.T, ca string, s *Server, slug, id string) map[string]any {
	t.Helper()
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: GET /api/board es 200: %d %s", ca, rec.Code, rec.Body.String())
	}
	var b struct {
		Columns []struct {
			Cards []map[string]any `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("%s: /api/board es JSON: %v", ca, err)
	}
	var vista map[string]any
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			acts, _ := c["actions"].([]any)
			for _, x := range acts {
				a, _ := x.(map[string]any)
				if r, _ := a["role"].(string); r != "" {
					o, ok := a["origen"].(map[string]any)
					if !ok {
						t.Fatalf("%s: la accion de rol %v de %v trae origen en /api/board: %v", ca, a["id"], c["slug"], a)
					}
					for _, k := range []string{"provider", "model", "effort", "budget_usd", "timeout"} {
						if v, _ := o[k].(string); !r1Origenes[v] {
							t.Fatalf("%s: origen.%s de %v es comando, hoom.yaml, tarjeta o provider: %v", ca, k, a["id"], o)
						}
					}
				}
				if c["slug"] == slug && a["id"] == id {
					vista = a
				}
			}
		}
	}
	if vista == nil {
		t.Fatalf("%s: fixture: la tarjeta %s tiene %s en /api/board", ca, slug, id)
	}
	return vista
}

// hallazgos 4a1c25 y 42550b, CA-492 y CA-496: /api/board emite el origen:
// con agents.writer.timeout: 30m, timeout_s 1800 con origen.timeout
// hoom.yaml (no "el de hoom"); sin nada declarado, 1800 con origen provider.
// El modelo de hoom.yaml dice hoom.yaml y el esfuerzo sin declarar, provider.
func TestHallazgo_42550b_CA496_ApiBoardEmiteElOrigen(t *testing.T) {
	root := raProyectoConReview(t, "agents:\n  writer:\n    model: m-yaml\n    timeout: 30m\n")
	lnCartaWriter(t, "hallazgo 42550b", root, "precios", "")
	lnCartaTestWriter(t, "hallazgo 42550b", root, "faltan-tests")
	a := r1AccionDelTablero(t, "hallazgo 42550b", newServer(t, root), "precios", boardcmd.ActPedirWriter)
	o, _ := a["origen"].(map[string]any)
	if a["timeout_s"] != float64(1800) || o["timeout"] != "hoom.yaml" || o["model"] != "hoom.yaml" || o["effort"] != "provider" {
		t.Fatalf("hallazgo 42550b, CA-496: agents.writer.timeout: 30m da 1800 con origen hoom.yaml, model de hoom.yaml y effort del provider: %v", a)
	}
	tw := r1AccionDelTablero(t, "hallazgo 42550b", newServer(t, root), "faltan-tests", boardcmd.ActPedirTestWriter)
	if o, _ := tw["origen"].(map[string]any); tw["timeout_s"] != float64(1800) || o["timeout"] != "provider" || o["model"] != "provider" {
		t.Fatalf("hallazgo 42550b, CA-496: el test-writer sin nada declarado: 1800 con origen provider: %v", tw)
	}
}

// hallazgo 810954, CA-496: los campos vacios del dialogo nombran el valor de
// hoom.yaml cuando el valor de la accion sale de ahi, y si no, el del CLI;
// la linea del tiempo limite mira origen.timeout para decir "(el de hoom)".
// Es una comprobacion sobre el fuente de acciones.js (como CA-496).
func TestHallazgo_810954_CA496_AccionesJSUsaElOrigen(t *testing.T) {
	_, _, acc := atUI(t)
	js := c2SinComentarios(acc)
	for _, quiero := range []string{"vacío: el de hoom.yaml (", "vacío: el que usa el CLI por defecto", " (el de hoom)", "tiempo limite: "} {
		if !strings.Contains(js, quiero) {
			t.Fatalf("hallazgo 810954, CA-496: acciones.js (fuera de los comentarios) contiene %q", quiero)
		}
	}
	if !regexp.MustCompile(`\borigen\b`).MatchString(js) || !regexp.MustCompile(`\btimeout\b`).MatchString(js) {
		t.Fatal("hallazgo 42550b, CA-496: acciones.js lee origen.timeout de la accion")
	}
	if !regexp.MustCompile(`['"]provider['"]`).MatchString(js) || !regexp.MustCompile(`['"]hoom\.yaml['"]`).MatchString(js) {
		t.Fatal("hallazgo 810954, CA-496: acciones.js compara el origen con provider y hoom.yaml")
	}
}
