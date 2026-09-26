// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (enmienda 1: CA-395, CA-400, CA-415): provider, modelo, esfuerzo y
// same_provider se resuelven opcion > hoom.yaml > vacio (un
// --same-provider=false explicito tambien) y cada pasada corre aislada con
// el esfuerzo resuelto. Los fixtures estan en review_aislada_helpers_test.go.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// ---------------------------------------------------------------- CA-395

// CA-395: la opcion explicita gana a hoom.yaml en provider, modelo y
// esfuerzo; el Result y el registro dicen lo que se pidio.
func TestCA395_OpcionExplicitaGanaAHoomYaml(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  provider: claude\n  model: m-yaml\n  effort: e-yaml\n")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")
	cl := raInstalar(t, bin, "claude", "")

	res, out := raRevisar(t, "CA-395", root, Options{Provider: "codex", Model: "m-opt", Effort: "e-opt", Lens: "risk"})
	if res.Status != "revisado" || cx.veces() != 1 || cl.veces() != 0 {
		t.Fatalf("CA-395: --provider codex gana a review.provider claude: codex %d, claude %d: %+v\n%s", cx.veces(), cl.veces(), res, out)
	}
	args := cx.argv(t, 1)
	if !raPar(args, "-m", "m-opt") || !raPar(args, "-c", `model_reasoning_effort="e-opt"`) {
		t.Fatalf("CA-395: modelo y esfuerzo explicitos ganan a hoom.yaml: %v", args)
	}
	for _, a := range args {
		if strings.Contains(a, "m-yaml") || strings.Contains(a, "e-yaml") {
			t.Fatalf("CA-395: lo de hoom.yaml no se cuela cuando hay opcion: %v", args)
		}
	}
	if res.Provider != "codex" || res.Model != "m-opt" || res.Effort != "e-opt" {
		t.Fatalf("CA-395: el Result dice lo que se pidio: %+v", res)
	}
	recs := raRegistros(t, root)
	if len(recs) != 1 || recs[0].Model != "m-opt" || recs[0].Effort != "e-opt" || recs[0].Provider != "codex" {
		t.Fatalf("CA-395: el registro dice modelo y esfuerzo pedidos: %+v", recs)
	}
}

// CA-395: sin opciones (como el Studio) manda hoom.yaml, y cada campo se
// resuelve por separado: una opcion de modelo no borra el esfuerzo de
// hoom.yaml.
func TestCA395_SinOpcionesTomaHoomYaml(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  provider: codex\n  model: m-yaml\n  effort: e-yaml\n")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")
	cl := raInstalar(t, bin, "claude", "") // primero en el orden de deteccion: sin hoom.yaml ganaria

	res, out := raRevisar(t, "CA-395", root, Options{Lens: "risk"})
	if cx.veces() != 1 || cl.veces() != 0 || res.Provider != "codex" {
		t.Fatalf("CA-395: review.provider codex elige el reviewer sin --provider: codex %d, claude %d: %+v\n%s",
			cx.veces(), cl.veces(), res, out)
	}
	args := cx.argv(t, 1)
	if !raPar(args, "-m", "m-yaml") || !raPar(args, "-c", `model_reasoning_effort="e-yaml"`) {
		t.Fatalf("CA-395: modelo y esfuerzo de hoom.yaml: %v", args)
	}
	if res.Model != "m-yaml" || res.Effort != "e-yaml" {
		t.Fatalf("CA-395: el Result dice los de hoom.yaml: %+v", res)
	}

	res, out = raRevisar(t, "CA-395", root, Options{Lens: "risk", Model: "m-opt"})
	args = cx.argv(t, 2)
	if !raPar(args, "-m", "m-opt") || !raPar(args, "-c", `model_reasoning_effort="e-yaml"`) || res.Model != "m-opt" || res.Effort != "e-yaml" {
		t.Fatalf("CA-395: cada campo se resuelve solo: modelo de la opcion, esfuerzo de hoom.yaml: %v %+v\n%s", args, res, out)
	}
}

// CA-395: review.same_provider: true equivale a --same-provider: con un
// writer de claude y solo claude instalado, la review corre y queda
// no-cruzada, con o sin --provider.
func TestCA395_SameProviderDeHoomYamlPermite(t *testing.T) {
	bin := raPATH(t)
	for _, opt := range []Options{{Lens: "risk"}, {Lens: "risk", Provider: "claude"}} {
		root := raRepo(t, "review:\n  same_provider: true\n")
		raCambio(t, root)
		metaDeRun(t, root, "20260925T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
		cl := raInstalar(t, bin, "claude", "")

		res, out := raRevisar(t, "CA-395", root, opt)
		if res.ExitCode != 0 || res.Status != "revisado" || cl.veces() != 1 {
			t.Fatalf("CA-395: same_provider: true deja revisar con el mismo provider (%+v): %+v\n%s", opt, res, out)
		}
		if res.Cross != CrossNo || res.Provider != "claude" {
			t.Fatalf("CA-395: la review queda marcada no-cruzada: %+v", res)
		}
		recs := raRegistros(t, root)
		if len(recs) != 1 || recs[0].Cross != CrossNo {
			t.Fatalf("CA-395: el registro dice no-cruzada: %+v", recs)
		}
	}
}

// CA-395 (caso limite): same_provider: true PERMITE, no fuerza: con otro
// provider instalado la review se hace con el otro y es cruzada. Guarda: el
// esqueleto ya elige al otro; la implementacion no puede volverla no cruzada.
func TestCA395_SameProviderNoFuerza(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  same_provider: true\n")
	raCambio(t, root)
	metaDeRun(t, root, "20260925T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	cl := raInstalar(t, bin, "claude", "")
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-395", root, Options{Lens: "risk"})
	if res.Provider != "codex" || res.Cross != CrossYes || cl.veces() != 0 || cx.veces() != 1 {
		t.Fatalf("CA-395: con otro provider instalado revisa el otro (cruzada): %+v\n%s", res, out)
	}
}

// CA-395: la negativa de una review no cruzada nombra tambien la clave de
// hoom.yaml, y no lanza nada.
func TestCA395_NegativaNombraReviewSameProvider(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	metaDeRun(t, root, "20260925T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	cl := raInstalar(t, bin, "claude", "")

	res, out := raRevisar(t, "CA-395", root, Options{Lens: "risk"})
	if res.ExitCode != 1 || res.Cross != CrossNo || len(res.Passes) != 0 || cl.veces() != 0 {
		t.Fatalf("CA-395: sin same_provider la review no cruzada se niega sin lanzar: %+v\n%s", res, out)
	}
	if !strings.Contains(out, "--same-provider") || !strings.Contains(out, "review.same_provider: true") {
		t.Fatalf("CA-395: la negativa nombra --same-provider y review.same_provider: true:\n%s", out)
	}
}

// CA-395 (caso limite): review.provider sin system_prompt da el error de hoy
// (el de --provider), sin lanzar nada.
func TestCA395_ProviderDeHoomYamlSinSystemPrompt(t *testing.T) {
	bin := raPATH(t)
	for _, prov := range []string{"gemini", "opencode"} {
		root := raRepo(t, "review:\n  provider: "+prov+"\n")
		raCambio(t, root)
		g := raInstalar(t, bin, prov, "")
		cx := raInstalar(t, bin, "codex", "")

		var out bytes.Buffer
		_, err := Run(root, "main", Options{Lens: "risk"}, &out)
		var eu providers.ErrUnsupported
		if !errors.As(err, &eu) || !raTiene(eu.Fields, providers.FieldSystemPrompt) || eu.Provider != prov {
			t.Fatalf("CA-395: review.provider %s sin system_prompt es el error de hoy: %v\n%s", prov, err, out.String())
		}
		if g.veces() != 0 || cx.veces() != 0 {
			t.Fatalf("CA-395: nada se lanza (%s %d, codex %d)", prov, g.veces(), cx.veces())
		}
	}
}

// CA-395: `hoom review --effort <e>` existe, esta en la ayuda y llega al
// provider; con --json el resultado trae effort e isolated.
func TestCA395_E2EFlagEffortYAyuda(t *testing.T) {
	hoom := hbHoomReal(t)
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")

	correr := func(args ...string) (int, string, string) {
		cmd := exec.Command(hoom, args...)
		cmd.Dir = root
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "HOOM_TASK=") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		_ = cmd.Run()
		if cmd.ProcessState == nil {
			t.Fatalf("CA-395: no pude correr hoom %v", args)
		}
		return cmd.ProcessState.ExitCode(), o.String(), e.String()
	}

	_, ayuda, _ := correr("help")
	i := strings.Index(ayuda, "Flags de review")
	if i < 0 {
		t.Fatalf("CA-395: la ayuda tiene el bloque de review:\n%s", ayuda)
	}
	bloque := ayuda[i:]
	if j := strings.Index(bloque, "\n\n"); j >= 0 {
		bloque = bloque[:j]
	}
	if !strings.Contains(bloque, "--effort") {
		t.Fatalf("CA-395: la ayuda de review nombra --effort:\n%s", bloque)
	}

	code, out, errOut := correr("review", "--provider", "codex", "--lens", "risk", "--effort", "high")
	if code != 0 || cx.veces() != 1 {
		t.Fatalf("CA-395: hoom review --effort high corre (exit %d, codex %d):\n%s\n%s", code, cx.veces(), out, errOut)
	}
	if !raPar(cx.argv(t, 1), "-c", `model_reasoning_effort="high"`) {
		t.Fatalf("CA-395: --effort high llega al provider: %v", cx.argv(t, 1))
	}
	if !strings.Contains(out, "  esfuerzo    high\n") {
		t.Fatalf("CA-395: la salida dice el esfuerzo elegido:\n%s", out)
	}

	code, out, errOut = correr("review", "--provider", "codex", "--lens", "risk", "--effort", "low", "--json")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil || code != 0 {
		t.Fatalf("CA-395: --json emite el resultado (exit %d): %v\n%s\n%s", code, err, out, errOut)
	}
	if m["effort"] != "low" || m["isolated"] != true || m["model"] != "" {
		t.Fatalf("CA-395: el JSON trae effort, isolated y model: %v", m)
	}
}

// ---------------------------------------------------------------- CA-400

// CA-400: cada pasada de hoom review corre aislada y con el esfuerzo
// resuelto (aca, de hoom.yaml), en codex y en claude.
func TestCA400_CadaPasadaAisladaYConEsfuerzo(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  effort: high\n")
	raCuatro(t, root)
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-400", root, Options{Provider: "codex"})
	if res.Status != "revisado" || len(res.Passes) != 4 || cx.veces() != 4 {
		t.Fatalf("CA-400: fixture: las 4 lentes corren: %+v\n%s", res, out)
	}
	for n := 1; n <= 4; n++ {
		args := cx.argv(t, n)
		if !raTiene(args, "--ignore-user-config") || !raPar(args, "-c", `model_reasoning_effort="high"`) {
			t.Fatalf("CA-400: la pasada %d corre aislada y con el esfuerzo de hoom.yaml: %v", n, args)
		}
	}
	if !res.Isolated || res.Effort != "high" {
		t.Fatalf("CA-400: el Result dice aislado y el esfuerzo: %+v", res)
	}
	if recs := raRegistros(t, root); len(recs) != 1 || !recs[0].Isolated || recs[0].Effort != "high" {
		t.Fatalf("CA-400: el registro dice aislado y el esfuerzo: %+v", recs)
	}

	root2 := raRepo(t, "")
	raCambio(t, root2)
	cl := raInstalar(t, bin, "claude", "")
	raRevisar(t, "CA-400", root2, Options{Provider: "claude", Lens: "risk", Effort: "max"})
	args := cl.argv(t, 1)
	if !raTiene(args, "--strict-mcp-config") || !raPar(args, "--setting-sources", "project") || !raPar(args, "--effort", "max") {
		t.Fatalf("CA-400: claude revisa aislado y con --effort: %v", args)
	}
}

// CA-400: review.isolated: false apaga el aislamiento (compatibilidad), y la
// salida lo dice.
func TestCA400_IsolatedFalseEnHoomYaml(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  isolated: false\n")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-400", root, Options{Provider: "codex", Lens: "risk"})
	if cx.veces() != 1 || raTiene(cx.argv(t, 1), "--ignore-user-config") {
		t.Fatalf("CA-400: con isolated: false el reviewer carga la config del usuario: %v", cx.argv(t, 1))
	}
	if res.Isolated {
		t.Fatalf("CA-400: el Result dice no aislado: %+v", res)
	}
	if !strings.Contains(out, "\n  aislado     no - review.isolated: false en hoom.yaml\n") {
		t.Fatalf("CA-400: la salida dice por que no esta aislado:\n%s", out)
	}
	if recs := raRegistros(t, root); len(recs) != 1 || recs[0].Isolated {
		t.Fatalf("CA-400: el registro dice no aislado: %+v", recs)
	}
}

// CA-400 (caso limite): --effort con un provider sin la capacidad es
// ErrUnsupported, sin pasadas. Guarda: los providers sin effort tampoco
// tienen system_prompt, y la negativa ya existe; no puede aparecer una
// pasada.
func TestCA400_EffortConProviderSinCapacidad(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	g := raInstalar(t, bin, "gemini", "")
	_, err := Run(root, "main", Options{Provider: "gemini", Effort: "high", Lens: "risk"}, &bytes.Buffer{})
	var eu providers.ErrUnsupported
	if !errors.As(err, &eu) || g.veces() != 0 {
		t.Fatalf("CA-400: ErrUnsupported y ninguna pasada: %v (gemini %d)", err, g.veces())
	}
	if recs := raRegistros(t, root); len(recs) != 0 {
		t.Fatalf("CA-400: sin pasadas no hay registro: %+v", recs)
	}
}

// ---------------------------------------------------------------- CA-415

// CA-415: SameProviderSet hace que un SameProvider false explicito venza a
// review.same_provider: true de hoom.yaml: con solo el provider del writer, la
// review se niega por no cruzada (no-entregable, exit 1, no-cruzada, sin
// pasadas ni registro); sin decirlo, hoom.yaml la sigue permitiendo. Y al
// reves: un true explicito vence a same_provider: false.
func TestCA415_SameProviderExplicitoVenceAHoomYaml(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  same_provider: true\n")
	raCambio(t, root)
	metaDeRun(t, root, "20260925T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	cl := raInstalar(t, bin, "claude", "")

	for _, opt := range []Options{
		{Lens: "risk", SameProvider: false, SameProviderSet: true},
		{Lens: "risk", Provider: "claude", SameProvider: false, SameProviderSet: true},
	} {
		res, out := raRevisar(t, "CA-415", root, opt)
		if res.Status != "no-entregable" || res.ExitCode != 1 || res.Cross != CrossNo || len(res.Passes) != 0 || cl.veces() != 0 {
			t.Fatalf("CA-415: SameProvider false explicito (%+v) vence a same_provider: true y la review se niega por no cruzada: %+v\n%s", opt, res, out)
		}
		if recs := raRegistros(t, root); len(recs) != 0 {
			t.Fatalf("CA-415: la negativa no escribe registro: %+v", recs)
		}
	}

	res, out := raRevisar(t, "CA-415", root, Options{Lens: "risk"})
	if res.Status != "revisado" || res.ExitCode != 0 || res.Cross != CrossNo || cl.veces() != 1 {
		t.Fatalf("CA-415: sin decirlo, same_provider: true de hoom.yaml sigue permitiendola: %+v\n%s", res, out)
	}

	root2 := raRepo(t, "review:\n  same_provider: false\n")
	raCambio(t, root2)
	metaDeRun(t, root2, "20260925T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	res, out = raRevisar(t, "CA-415", root2, Options{Lens: "risk", SameProvider: true, SameProviderSet: true})
	if res.Status != "revisado" || res.Cross != CrossNo || cl.veces() != 2 {
		t.Fatalf("CA-415: un SameProvider true explicito vence a same_provider: false: %+v\n%s", res, out)
	}
}

// raHoom corre el binario de hoom en root, sin HOOM_TASK, con el PATH del
// test (los CLIs falsos).
func raHoom(t *testing.T, hoom, root string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(hoom, args...)
	cmd.Dir = root
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOOM_TASK=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	_ = cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude correr hoom %v", args)
	}
	return cmd.ProcessState.ExitCode(), o.String(), e.String()
}

// CA-415, de punta a punta: `hoom review --same-provider=false` pone
// SameProviderSet y vence a same_provider: true de hoom.yaml (exit 1, sin
// pasadas ni registro); sin el flag, hoom.yaml la permite (exit 0).
func TestCA415_E2ESameProviderFalseEnLaLineaDeComando(t *testing.T) {
	hoom := hbHoomReal(t)
	bin := raPATH(t)
	root := raRepo(t, "review:\n  same_provider: true\n")
	raCambio(t, root)
	metaDeRun(t, root, "20260925T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	cl := raInstalar(t, bin, "claude", "")

	code, out, errOut := raHoom(t, hoom, root, "review", "--lens", "risk", "--same-provider=false")
	if code != 1 || cl.veces() != 0 {
		t.Fatalf("CA-415: --same-provider=false vence a hoom.yaml y la review se niega (exit %d, claude %d):\n%s\n%s", code, cl.veces(), out, errOut)
	}
	if recs, _ := Records(root); len(recs) != 0 {
		t.Fatalf("CA-415: la negativa no escribe registro: %+v", recs)
	}

	code, out, errOut = raHoom(t, hoom, root, "review", "--lens", "risk")
	if code != 0 || cl.veces() != 1 {
		t.Fatalf("CA-415: sin el flag, same_provider: true de hoom.yaml la permite (exit %d, claude %d):\n%s\n%s", code, cl.veces(), out, errOut)
	}
}
