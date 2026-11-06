// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// en el Studio: la tarjeta muestra modelo y esfuerzo del reviewer y marca la
// cruzada sin confirmar (CA-408); el dialogo de pedir-reviewer gana el campo
// esfuerzo y POST /launch lo pasa a la review (CA-409; desde
// verbos-de-la-tarjeta, CA-496, tambien al sobre en las otras acciones de
// rol); sin opciones, la review del Studio toma modelo y esfuerzo de
// hoom.yaml (CA-395); y los roles que escriben siguen sin aislamiento
// (CA-400). Los CLIs son los falsos de lanzar_test.go, en un PATH minimo.
package servecmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

// raProyectoConReview es tbProyecto con una seccion review: commiteada en
// main, antes de que nazca cualquier espacio de trabajo.
func raProyectoConReview(t *testing.T, review string) string {
	t.Helper()
	root := tbProyecto(t)
	raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, root, "hoom.yaml", string(raw)+review)
	lnCommit(t, root, "review en hoom.yaml")
	return root
}

func raCuerpo(action, provider, model, pedido, effort string) map[string]any {
	c := lnCuerpo(action, provider, model, nil, pedido)
	c["effort"] = effort
	return c
}

// ---------------------------------------------------------------- CA-408

// CA-408: tablero.js pinta el modelo y el esfuerzo del reviewer, con 'sin
// registrar' por cada vacio, marca 'cruzada sin confirmar' la cruzada
// desconocida y conserva 'misma CLI'; la regla del vocabulario normal
// (CA-321) sigue en pie.
func TestCA408_TableroJSMuestraModeloEsfuerzoYCruzadaSinConfirmar(t *testing.T) {
	html, js := tbUI(t)
	for _, quiero := range []string{"reviewer_model", "reviewer_effort", "sin registrar", "cruzada sin confirmar", "misma CLI", "desconocida"} {
		if !strings.Contains(js, quiero) {
			t.Fatalf("CA-408: tablero.js contiene %q", quiero)
		}
	}
	tbRevisarModo(t, html, js)
}

// ---------------------------------------------------------------- CA-409

// CA-409: el dialogo de pedir-reviewer tiene provider, modelo, esfuerzo,
// presupuesto y pedido (re-expresa CA-354), y el POST lleva effort.
func TestCA409_DialogoConCampoEsfuerzo(t *testing.T) {
	_, _, acc := atUI(t)
	if !strings.Contains(acc, `<label for="acc-effort">Esfuerzo</label>`) {
		t.Fatal(`CA-409: el dialogo tiene <label for="acc-effort">Esfuerzo</label>`)
	}
	if !regexp.MustCompile(`id="acc-effort"`).MatchString(acc) {
		t.Fatal(`CA-409: el dialogo tiene el campo id="acc-effort"`)
	}
	if !regexp.MustCompile(`\beffort\s*:`).MatchString(acc) {
		t.Fatal("CA-409: el cuerpo del POST de /launch lleva effort")
	}
	bajo := strings.ToLower(acc)
	for _, campo := range []string{"provider", "modelo", "esfuerzo", "presupuesto", "pedido"} {
		if !strings.Contains(bajo, campo) {
			t.Fatalf("CA-409: el dialogo de rol tiene %s", campo)
		}
	}
}

// CA-409 y CA-408: POST /launch de pedir-reviewer con effort "high" corre la
// review con ese esfuerzo (y aislada) en las 4 pasadas; la tarjeta despues
// dice el esfuerzo registrado y, sin modelo elegido, un modelo vacio.
func TestCA409_LaunchPasaEffortALaReview(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "codex", false)
	root := tbProyecto(t)
	wt := lnCartaReview(t, "CA-409", root, "con-esfuerzo")
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-409", lnLaunch(t, s, "con-esfuerzo", s.Token(),
		raCuerpo("pedir-reviewer", "codex", "", "", "high")))
	if r.Role != "reviewer" || r.Provider != "codex" {
		t.Fatalf("CA-409: pedir-reviewer responde rol reviewer con codex: %+v", r)
	}
	lnEsperarLanzamientos(t, "CA-409", s)
	ll := f.llamadas(t, "codex")
	if len(ll) != len(reviewcmd.Lentes) {
		t.Fatalf("CA-409: una pasada por lente (%d), hubo %d", len(reviewcmd.Lentes), len(ll))
	}
	for _, args := range ll {
		if !raHayPar(args, "-c", `model_reasoning_effort="high"`) {
			t.Fatalf(`CA-409: cada pasada lleva -c model_reasoning_effort="high": %v`, args)
		}
		if !lnTiene(args, "--ignore-user-config") {
			t.Fatalf("CA-400: la review del Studio corre aislada: %v", args)
		}
	}
	recs, _ := reviewcmd.Records(wt)
	if len(recs) != 1 || recs[0].Effort != "high" || recs[0].Model != "" || !recs[0].Isolated {
		t.Fatalf("CA-409: el registro dice el esfuerzo pedido desde el Studio: %+v", recs)
	}
	c := lnCarta(t, root, "con-esfuerzo")
	if c.Providers.ReviewerEffort != "high" || c.Providers.ReviewerModel != "" || c.Providers.Reviewer != "codex" {
		t.Fatalf("CA-408: la tarjeta trae reviewer_effort del registro de la review: %+v", c.Providers)
	}
}

func raHayPar(args []string, flag, valor string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == valor {
			return true
		}
	}
	return false
}

// CA-409, re-expresado por CA-496 (verbos-de-la-tarjeta): effort ya no es
// solo de pedir-reviewer. Con pedir-writer y pedir-test-writer el launch se
// acepta y el esfuerzo llega al sobre (claude --effort high); el error
// 'effort solo aplica a pedir-reviewer' no aparece nunca. Vacio, la accion
// sigue como hoy.
func TestCA409_EffortConOtraAccionLlegaAlSobre(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-409", root, "a-implementar", "")
	lnCartaTestWriter(t, "CA-409", root, "faltan-tests")
	s := lnServidor(t, root, f)

	for _, k := range []struct{ slug, action, pedido string }{
		{"a-implementar", "pedir-writer", "Implementa el spec hasta que verify de verde."},
		{"faltan-tests", "pedir-test-writer", "Escribi los tests del criterio que falta."},
	} {
		rec := lnLaunch(t, s, k.slug, s.Token(), raCuerpo(k.action, "claude", "", k.pedido, "high"))
		if strings.Contains(rec.Body.String(), "effort solo aplica a pedir-reviewer") {
			t.Fatalf("CA-409/CA-496: %s con effort ya no responde 'effort solo aplica a pedir-reviewer': %s", k.action, rec.Body.String())
		}
		if r := lnAceptado(t, "CA-409/CA-496 ("+k.action+")", rec); r.Provider != "claude" {
			t.Fatalf("CA-409/CA-496: %s con effort se lanza con claude: %+v", k.action, r)
		}
		lnEsperarLanzamientos(t, "CA-409", s)
	}
	ll := f.llamadas(t, "claude")
	if len(ll) != 2 {
		t.Fatalf("CA-409/CA-496: dos lanzamientos, dos llamadas a claude: %d", len(ll))
	}
	for _, args := range ll {
		if v, ok := lnSigue(args, "--effort"); !ok || v != "high" {
			t.Fatalf("CA-409/CA-496: el esfuerzo pedido llega al sobre (--effort high): %q", args)
		}
	}

	// con effort vacio, la misma accion se acepta como hoy
	lnAceptado(t, "CA-409", lnLaunch(t, s, "a-implementar", s.Token(),
		raCuerpo("pedir-writer", "claude", "", "Implementa el spec hasta que verify de verde.", "")))
	lnEsperarLanzamientos(t, "CA-409", s)
}

// ---------------------------------------------------------------- CA-395

// CA-395: la review que lanza el Studio sin modelo ni esfuerzo toma los de
// hoom.yaml; un modelo del dialogo gana al de hoom.yaml.
func TestCA395_StudioTomaModeloYEsfuerzoDeHoomYaml(t *testing.T) {
	for _, k := range []struct{ model, quiero string }{{"", "m-yaml"}, {"m-dialogo", "m-dialogo"}} {
		f := lnPATH(t)
		f.cli(t, "codex", false)
		root := raProyectoConReview(t, "review:\n  model: m-yaml\n  effort: e-yaml\n")
		wt := lnCartaReview(t, "CA-395", root, "desde-yaml")
		s := lnServidor(t, root, f)

		lnAceptado(t, "CA-395", lnLaunch(t, s, "desde-yaml", s.Token(),
			raCuerpo("pedir-reviewer", "codex", k.model, "", "")))
		lnEsperarLanzamientos(t, "CA-395", s)
		ll := f.llamadas(t, "codex")
		if len(ll) == 0 {
			t.Fatal("CA-395: la review del Studio corrio")
		}
		for _, args := range ll {
			if m, _ := lnSigue(args, "-m"); m != k.quiero || !raHayPar(args, "-c", `model_reasoning_effort="e-yaml"`) {
				t.Fatalf("CA-395: modelo %q y esfuerzo de hoom.yaml en la review del Studio: %v", k.quiero, args)
			}
		}
		recs, _ := reviewcmd.Records(wt)
		if len(recs) != 1 || recs[0].Model != k.quiero || recs[0].Effort != "e-yaml" {
			t.Fatalf("CA-395: el registro dice modelo %q y esfuerzo e-yaml: %+v", k.quiero, recs)
		}
	}
}

// ---------------------------------------------------------------- CA-400

// CA-400: desde el Studio, pedir-writer con codex no aisla ni pone esfuerzo
// aunque hoom.yaml los pida para la review. Guarda: el esqueleto ya es asi;
// la implementacion no puede derramar review: sobre los roles que escriben.
func TestCA400_StudioNoAislaALosRolesDeEscritura(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "codex", false)
	root := raProyectoConReview(t, "review:\n  effort: xhigh\n  isolated: true\n")
	lnCartaWriter(t, "CA-400", root, "a-implementar", "")
	s := lnServidor(t, root, f)

	lnAceptado(t, "CA-400", lnLaunch(t, s, "a-implementar", s.Token(),
		raCuerpo("pedir-writer", "codex", "", "Implementa el spec hasta que verify de verde.", "")))
	lnEsperarLanzamientos(t, "CA-400", s)
	ll := f.llamadas(t, "codex")
	if len(ll) != 1 {
		t.Fatalf("CA-400: fixture: el writer corrio una vez: %d", len(ll))
	}
	for _, a := range ll[0] {
		if a == "--ignore-user-config" || strings.HasPrefix(a, "model_reasoning_effort=") {
			t.Fatalf("CA-400: el writer no lleva aislamiento ni esfuerzo: %v", ll[0])
		}
	}
}
