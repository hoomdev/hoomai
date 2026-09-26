// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (enmiendas 1 y 3: CA-394, CA-395, CA-400, CA-415, CA-417): provider,
// modelo, esfuerzo y same_provider se resuelven opcion > hoom.yaml de la
// base (el del merge-base, nunca el del candidato) > vacio (un
// --same-provider=false explicito tambien), cada pasada corre aislada con el
// esfuerzo resuelto, y un tope fuera de rango en la base es un error. Los
// fixtures estan en review_aislada_helpers_test.go.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
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

	_, ayuda, _ := raHoom(t, hoom, root, "help")
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

	code, out, errOut := raHoom(t, hoom, root, "review", "--provider", "codex", "--lens", "risk", "--effort", "high")
	if code != 0 || cx.veces() != 1 {
		t.Fatalf("CA-395: hoom review --effort high corre (exit %d, codex %d):\n%s\n%s", code, cx.veces(), out, errOut)
	}
	if !raPar(cx.argv(t, 1), "-c", `model_reasoning_effort="high"`) {
		t.Fatalf("CA-395: --effort high llega al provider: %v", cx.argv(t, 1))
	}
	if !strings.Contains(out, "  esfuerzo    high\n") {
		t.Fatalf("CA-395: la salida dice el esfuerzo elegido:\n%s", out)
	}

	code, out, errOut = raHoom(t, hoom, root, "review", "--provider", "codex", "--lens", "risk", "--effort", "low", "--json")
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

// ---------------------------------------------------------------- CA-394

// CA-394 (enmienda 3), de punta a punta: con max_evidence_kib 16385 o el
// maximo de int64 en el hoom.yaml de la base, `hoom review` (Run) es un error
// con el texto del contrato: no entra en panico, no desborda el tope a algo
// que deje correr, no lanza pasadas ni escribe registro. Tambien cuando la
// rama commitea un tope valido encima: la seccion review: es la de la base
// (CA-417).
func TestCA394_ReviewConTopeFueraDeRangoEnLaBaseEsError(t *testing.T) {
	for _, v := range []string{"16385", "9223372036854775807"} {
		for _, rama := range []bool{false, true} {
			nombre := v + "/sin-rama"
			if rama {
				nombre = v + "/la-rama-commitea-un-tope-valido"
			}
			t.Run(nombre, func(t *testing.T) {
				bin := raPATH(t)
				var root string
				if rama {
					root = raRamaConReview(t, "review:\n  max_evidence_kib: "+v+"\n", "review:\n  max_evidence_kib: 320\n", true)
				} else {
					root = raRepo(t, "review:\n  max_evidence_kib: "+v+"\n")
					raCambio(t, root)
				}
				cx := raInstalar(t, bin, "codex", "")

				var out bytes.Buffer
				var res Result
				var err error
				var panico any
				func() {
					defer func() { panico = recover() }()
					res, err = Run(root, "main", Options{Provider: "codex", Lens: "risk"}, &out)
				}()
				if panico != nil {
					t.Fatalf("CA-394: max_evidence_kib: %s en la base hace entrar en panico a la review: %v\n%s", v, panico, out.String())
				}
				if err == nil {
					t.Fatalf("CA-394: max_evidence_kib: %s en la base es un hoom.yaml invalido y la review es un error: %+v\n%s", v, res, out.String())
				}
				if msg := out.String() + "\n" + err.Error(); !strings.Contains(msg, "review: max_evidence_kib no puede pasar de 16384") {
					t.Fatalf("CA-394: la review dice \"review: max_evidence_kib no puede pasar de 16384\": %v\n%s", err, out.String())
				}
				if cx.veces() != 0 || len(res.Passes) != 0 {
					t.Fatalf("CA-394: no se lanza ninguna pasada (codex %d): %+v", cx.veces(), res)
				}
				if recs, _ := Records(root); len(recs) != 0 {
					t.Fatalf("CA-394: sin registro de review: %+v", recs)
				}
			})
		}
	}
}

// ---------------------------------------------------------------- CA-417
//
// Enmienda 3: la seccion review: sale del hoom.yaml del MERGE-BASE (la base
// que el equipo aprobo), no del candidato: un cambio no elige su propio
// reviewer ni afloja su propia review. Las opciones explicitas siguen
// mandando. Todos estos tests corren en una rama feature sobre main.

// raRamaConReview arma la base (main) con baseReview en su hoom.yaml y abre
// la rama feature, cuyo hoom.yaml trae candReview: commiteado en la rama o,
// sin commitear, solo en el arbol de trabajo. Planta ademas un cambio de
// codigo sin commitear (una lente).
func raRamaConReview(t *testing.T, baseReview, candReview string, commitear bool) string {
	t.Helper()
	root := raRepo(t, baseReview)
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, root, "hoom.yaml", raYAML(candReview))
	if commitear {
		git(t, root, "commit", "-q", "-am", "la rama elige su propia review")
	}
	raCambio(t, root)
	return root
}

// raRamaSinHoomYamlEnLaBase: la base no tiene hoom.yaml; la rama lo agrega,
// con candReview, en un commit. Sin hoom.yaml en la base valen los valores
// por defecto.
func raRamaSinHoomYamlEnLaBase(t *testing.T, candReview string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial sin hoom.yaml")
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, root, "hoom.yaml", raYAML(candReview))
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "la rama agrega hoom.yaml con su review")
	raCambio(t, root)
	return root
}

// raValores devuelve cada valor que sigue a flag en el argv. Los chequeos de
// modelo y esfuerzo miran solo los valores de sus flags: el pedido lleva el
// diff, y el diff trae el hoom.yaml que la rama cambio.
func raValores(args []string, flag string) []string {
	var v []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			v = append(v, args[i+1])
		}
	}
	return v
}

// raEsfuerzosCodex son los -c model_reasoning_effort=... del argv de codex.
func raEsfuerzosCodex(args []string) []string {
	var v []string
	for _, c := range raValores(args, "-c") {
		if strings.HasPrefix(c, "model_reasoning_effort=") {
			v = append(v, c)
		}
	}
	return v
}

// raSolo dice si got es exactamente [want] (want "" = ninguno).
func raSolo(got []string, want string) bool {
	if want == "" {
		return len(got) == 0
	}
	return len(got) == 1 && got[0] == want
}

// raAflojada es la review que una rama se quiere dar a si misma.
const raAflojada = "review: {same_provider: true, isolated: false}\n"

// CA-417: una rama que se da review: {same_provider: true, isolated: false}
// sobre una base sin esa seccion no elige su reviewer: con solo el provider
// que escribio instalado, la review se niega por no cruzada (no-entregable,
// exit 1, no-cruzada, sin pasadas ni registro), con o sin --provider. Igual
// si el cambio de hoom.yaml esta sin commitear o si la base ni tiene
// hoom.yaml.
func TestCA417_LaRamaNoSeDaSameProvider(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T) string
	}{
		{"commiteado-en-la-rama", func(t *testing.T) string { return raRamaConReview(t, "", raAflojada, true) }},
		{"sin-commitear", func(t *testing.T) string { return raRamaConReview(t, "", raAflojada, false) }},
		{"base-sin-hoom-yaml", func(t *testing.T) string { return raRamaSinHoomYamlEnLaBase(t, raAflojada) }},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := c.armar(t)
			metaDeRun(t, root, "20260926T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
			cl := raInstalar(t, bin, "claude", "")

			for _, opt := range []Options{{Lens: "risk"}, {Lens: "risk", Provider: "claude"}} {
				res, out := raRevisar(t, "CA-417", root, opt)
				if res.Status != "no-entregable" || res.ExitCode != 1 || res.Cross != CrossNo || len(res.Passes) != 0 || cl.veces() != 0 {
					t.Fatalf("CA-417: same_provider: true de la rama no deja revisar con el provider que escribio (%+v): claude %d, %+v\n%s",
						opt, cl.veces(), res, out)
				}
				if !strings.Contains(out, "--same-provider") {
					t.Fatalf("CA-417: la negativa es la de siempre y nombra --same-provider:\n%s", out)
				}
				if recs := raRegistros(t, root); len(recs) != 0 {
					t.Fatalf("CA-417: la negativa no escribe registro: %+v", recs)
				}
			}
		})
	}
}

// CA-417: la rama tampoco se saca el aislamiento. Con isolated: false solo
// en la rama, cada pasada corre aislada: codex con --ignore-user-config;
// claude con --strict-mcp-config y --setting-sources project. El Result y el
// registro dicen aislado y la salida no culpa a hoom.yaml. Aun con el mismo
// provider que escribio (solo porque el operador lo pide con la opcion
// explicita), la review corre aislada.
func TestCA417_LaRamaNoSeSacaElAislamiento(t *testing.T) {
	casos := []struct {
		nombre, writer, reviewer string
		opt                      Options
		commitear                bool
	}{
		{"cruzada-con-codex", "claude", "codex", Options{Lens: "risk"}, true},
		{"cruzada-con-claude", "codex", "claude", Options{Lens: "risk"}, true},
		{"cruzada-con-codex-sin-commitear", "claude", "codex", Options{Lens: "risk"}, false},
		{"mismo-provider-por-opcion-explicita", "claude", "claude", Options{Lens: "risk", SameProvider: true, SameProviderSet: true}, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRamaConReview(t, "", raAflojada, c.commitear)
			metaDeRun(t, root, "20260926T090000_wr1ter", c.writer, "writer", time.Now().Add(-time.Minute))
			cli := raInstalar(t, bin, c.reviewer, "")
			if c.writer != c.reviewer {
				raInstalar(t, bin, c.writer, "")
			}

			res, out := raRevisar(t, "CA-417", root, c.opt)
			if res.Status != "revisado" || res.Provider != c.reviewer || cli.veces() != 1 {
				t.Fatalf("CA-417: fixture: la review corre con %s: %+v\n%s", c.reviewer, res, out)
			}
			args := cli.argv(t, 1)
			if c.reviewer == "codex" && !raTiene(args, "--ignore-user-config") {
				t.Fatalf("CA-417: isolated: false de la rama no vale: codex corre con --ignore-user-config: %v", args)
			}
			if c.reviewer == "claude" && (!raTiene(args, "--strict-mcp-config") || !raPar(args, "--setting-sources", "project")) {
				t.Fatalf("CA-417: isolated: false de la rama no vale: claude corre con --strict-mcp-config y --setting-sources project: %v", args)
			}
			if !res.Isolated {
				t.Fatalf("CA-417: el Result dice aislado: %+v", res)
			}
			if !strings.Contains(out, "\n  aislado     si - sin la config personal del provider\n") || strings.Contains(out, "review.isolated: false") {
				t.Fatalf("CA-417: la salida dice aislado, sin culpar al hoom.yaml de la rama:\n%s", out)
			}
			if recs := raRegistros(t, root); len(recs) != 1 || !recs[0].Isolated {
				t.Fatalf("CA-417: el registro dice aislado: %+v", recs)
			}
			if c.writer == c.reviewer && res.Cross != CrossNo {
				t.Fatalf("CA-417: con el mismo provider la review queda no-cruzada: %+v", res)
			}
		})
	}
}

// CA-417: con la seccion en la base, vale: same_provider: true deja revisar
// con el provider que escribio (no-cruzada, y el registro lo dice) e
// isolated: false apaga el aislamiento. La rama no la puede quitar ni
// endurecer por su cuenta: aunque su hoom.yaml ya no traiga review: (o
// traiga otra), rige la de la base.
func TestCA417_LaSeccionDeLaBaseVale(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T) string
	}{
		{"la-rama-no-toca-hoom-yaml", func(t *testing.T) string {
			root := raRepo(t, raAflojada)
			git(t, root, "checkout", "-q", "-b", "feature")
			write(t, root, "rama.go", "package app\n\nvar EnLaRama = true\n")
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "rama")
			raCambio(t, root)
			return root
		}},
		{"la-rama-commitea-hoom-yaml-sin-review", func(t *testing.T) string { return raRamaConReview(t, raAflojada, "", true) }},
		{"la-rama-quita-review-sin-commitear", func(t *testing.T) string { return raRamaConReview(t, raAflojada, "", false) }},
		{"la-rama-commitea-una-review-mas-estricta", func(t *testing.T) string {
			return raRamaConReview(t, raAflojada, "review: {same_provider: false, isolated: true}\n", true)
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := c.armar(t)
			metaDeRun(t, root, "20260926T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
			cl := raInstalar(t, bin, "claude", "")

			res, out := raRevisar(t, "CA-417", root, Options{Lens: "risk"})
			if res.Status != "revisado" || res.ExitCode != 0 || res.Cross != CrossNo || res.Provider != "claude" || cl.veces() != 1 {
				t.Fatalf("CA-417: same_provider: true de la base deja revisar con el mismo provider: claude %d, %+v\n%s", cl.veces(), res, out)
			}
			args := cl.argv(t, 1)
			if raTiene(args, "--strict-mcp-config") || raTiene(args, "--setting-sources") || res.Isolated {
				t.Fatalf("CA-417: isolated: false de la base apaga el aislamiento: %v %+v", args, res)
			}
			if !strings.Contains(out, "\n  aislado     no - review.isolated: false en hoom.yaml\n") {
				t.Fatalf("CA-417: la salida dice por que no esta aislado:\n%s", out)
			}
			if recs := raRegistros(t, root); len(recs) != 1 || recs[0].Cross != CrossNo || recs[0].Isolated {
				t.Fatalf("CA-417: el registro dice no-cruzada y no aislado: %+v", recs)
			}
		})
	}
}

// CA-417: provider, modelo y esfuerzo salen de la base aunque la rama los
// cambie: base {provider: codex, model: base-model, effort: low}, rama
// {provider: claude, model: cand-model, effort: high} -> codex con
// base-model y low. Y si la base no los elige y la rama si, quedan los del
// provider (no elegidos).
func TestCA417_ModeloYEsfuerzoDeLaBase(t *testing.T) {
	const base = "review:\n  provider: codex\n  model: base-model\n  effort: low\n"
	const cand = "review:\n  provider: claude\n  model: cand-model\n  effort: high\n"
	for _, commitear := range []bool{true, false} {
		nombre := "sin-commitear"
		if commitear {
			nombre = "commiteado-en-la-rama"
		}
		t.Run(nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRamaConReview(t, base, cand, commitear)
			cl := raInstalar(t, bin, "claude", "") // primero en el orden de deteccion
			cx := raInstalar(t, bin, "codex", "")

			res, out := raRevisar(t, "CA-417", root, Options{Lens: "risk"})
			if res.Status != "revisado" || res.Provider != "codex" || cx.veces() != 1 || cl.veces() != 0 {
				t.Fatalf("CA-417: review.provider de la base (codex) elige el reviewer, no el de la rama: codex %d, claude %d, %+v\n%s",
					cx.veces(), cl.veces(), res, out)
			}
			args := cx.argv(t, 1)
			if !raSolo(raValores(args, "-m"), "base-model") || !raSolo(raEsfuerzosCodex(args), `model_reasoning_effort="low"`) {
				t.Fatalf("CA-417: modelo y esfuerzo de la base, y nada de lo que eligio la rama: -m %v, esfuerzo %v",
					raValores(args, "-m"), raEsfuerzosCodex(args))
			}
			if res.Model != "base-model" || res.Effort != "low" {
				t.Fatalf("CA-417: el Result dice los de la base: %+v", res)
			}
			if !strings.Contains(out, "\n  modelo      base-model\n") || !strings.Contains(out, "\n  esfuerzo    low\n") {
				t.Fatalf("CA-417: la salida dice modelo y esfuerzo de la base:\n%s", out)
			}
			if recs := raRegistros(t, root); len(recs) != 1 || recs[0].Model != "base-model" || recs[0].Effort != "low" || recs[0].Provider != "codex" {
				t.Fatalf("CA-417: el registro dice provider, modelo y esfuerzo de la base: %+v", recs)
			}
		})
	}

	// la base no elige; la rama si: quedan los del provider
	bin := raPATH(t)
	root := raRamaConReview(t, "", "review:\n  model: cand-model\n  effort: high\n", true)
	cx := raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-417", root, Options{Provider: "codex", Lens: "risk"})
	if cx.veces() != 1 || res.Model != "" || res.Effort != "" {
		t.Fatalf("CA-417: sin modelo ni esfuerzo en la base, no se eligen (los de la rama no valen): %+v\n%s", res, out)
	}
	if args := cx.argv(t, 1); !raSolo(raValores(args, "-m"), "") || !raSolo(raEsfuerzosCodex(args), "") {
		t.Fatalf("CA-417: el argv no lleva modelo ni esfuerzo: -m %v, esfuerzo %v", raValores(args, "-m"), raEsfuerzosCodex(args))
	}
	if !strings.Contains(out, "\n  modelo      por defecto del provider (no elegido)\n") ||
		!strings.Contains(out, "\n  esfuerzo    por defecto del provider (no elegido)\n") {
		t.Fatalf("CA-417: la salida dice que no se eligieron:\n%s", out)
	}
}

// CA-417: las opciones explicitas siguen mandando sobre el hoom.yaml de la
// base: --provider, --model y --effort le ganan a la base (y lo de la rama no
// aparece); una opcion sola no borra los demas campos de la base; y
// --same-provider explicito (true o false) le gana a same_provider de la
// base, digan lo que digan la base y la rama.
func TestCA417_LasOpcionesExplicitasSiguenMandando(t *testing.T) {
	const base = "review:\n  provider: claude\n  model: base-model\n  effort: low\n"
	const cand = "review:\n  provider: codex\n  model: cand-model\n  effort: high\n"
	bin := raPATH(t)
	root := raRamaConReview(t, base, cand, true)
	cl := raInstalar(t, bin, "claude", "")
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-417", root, Options{Provider: "codex", Model: "m-opt", Effort: "e-opt", Lens: "risk"})
	if res.Provider != "codex" || cx.veces() != 1 || cl.veces() != 0 {
		t.Fatalf("CA-417: --provider codex gana a review.provider de la base: codex %d, claude %d, %+v\n%s", cx.veces(), cl.veces(), res, out)
	}
	args := cx.argv(t, 1)
	if !raSolo(raValores(args, "-m"), "m-opt") || !raSolo(raEsfuerzosCodex(args), `model_reasoning_effort="e-opt"`) ||
		res.Model != "m-opt" || res.Effort != "e-opt" {
		t.Fatalf("CA-417: modelo y esfuerzo explicitos ganan a la base, y ni lo de la base ni lo de la rama se cuela: -m %v, esfuerzo %v, %+v",
			raValores(args, "-m"), raEsfuerzosCodex(args), res)
	}

	// solo --model: provider y esfuerzo siguen siendo los de la base
	res, out = raRevisar(t, "CA-417", root, Options{Model: "m-opt", Lens: "risk"})
	if res.Provider != "claude" || cl.veces() != 1 || cx.veces() != 1 {
		t.Fatalf("CA-417: sin --provider manda review.provider de la base (claude), no el de la rama (codex): claude %d, codex %d, %+v\n%s",
			cl.veces(), cx.veces(), res, out)
	}
	args = cl.argv(t, 1)
	if !raSolo(raValores(args, "--model"), "m-opt") || !raSolo(raValores(args, "--effort"), "low") || res.Model != "m-opt" || res.Effort != "low" {
		t.Fatalf("CA-417: modelo de la opcion y esfuerzo de la base: --model %v, --effort %v, %+v",
			raValores(args, "--model"), raValores(args, "--effort"), res)
	}

	// --same-provider=false explicito gana a same_provider: true de la base
	// (y de la rama): con solo el provider que escribio, se niega
	bin = raPATH(t)
	// la rama repite la seccion de la base; el comentario solo hace que haya
	// algo que commitear
	root = raRamaConReview(t, "review:\n  same_provider: true\n", "review:\n  same_provider: true\n# la rama repite la seccion de la base\n", true)
	metaDeRun(t, root, "20260926T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	cl = raInstalar(t, bin, "claude", "")
	res, out = raRevisar(t, "CA-417", root, Options{Lens: "risk", SameProvider: false, SameProviderSet: true})
	if res.Status != "no-entregable" || res.ExitCode != 1 || res.Cross != CrossNo || cl.veces() != 0 {
		t.Fatalf("CA-417: --same-provider=false gana a same_provider: true de la base: claude %d, %+v\n%s", cl.veces(), res, out)
	}
	res, out = raRevisar(t, "CA-417", root, Options{Lens: "risk"})
	if res.Status != "revisado" || res.Cross != CrossNo || cl.veces() != 1 {
		t.Fatalf("CA-417: sin la opcion, same_provider: true de la base la permite: %+v\n%s", res, out)
	}

	// --same-provider explicito gana a una base que no lo permite, aunque la
	// rama si lo diga: corre, no-cruzada
	bin = raPATH(t)
	root = raRamaConReview(t, "review:\n  same_provider: false\n", "review:\n  same_provider: true\n", true)
	metaDeRun(t, root, "20260926T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	cl = raInstalar(t, bin, "claude", "")
	res, out = raRevisar(t, "CA-417", root, Options{Lens: "risk"})
	if res.Status != "no-entregable" || cl.veces() != 0 {
		t.Fatalf("CA-417: same_provider: false de la base manda sobre el true de la rama: %+v\n%s", res, out)
	}
	res, out = raRevisar(t, "CA-417", root, Options{Lens: "risk", SameProvider: true, SameProviderSet: true})
	if res.Status != "revisado" || res.Cross != CrossNo || cl.veces() != 1 {
		t.Fatalf("CA-417: --same-provider explicito gana a same_provider: false de la base: %+v\n%s", res, out)
	}
}

// CA-417: el tope de la evidencia es el de la base. Una rama que sube
// max_evidence_kib no se agranda el tope: con la base en 1 KiB y la rama en
// 16384, una evidencia de mas de 1 KiB se niega nombrando 1 KiB, sin
// pasadas. Y una rama que lo baja tampoco cambia nada: rige el de la base
// (320 KiB por defecto).
func TestCA417_ElTopeEsElDeLaBase(t *testing.T) {
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "var Relleno%02d = \"cuarenta bytes de relleno por linea\"\n", i)
	}

	bin := raPATH(t)
	root := raRamaConReview(t, "review:\n  max_evidence_kib: 1\n", "review:\n  max_evidence_kib: 16384\n", true)
	write(t, root, "app.go", b.String())
	cx := raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-417", root, Options{Provider: "codex", Lens: "risk"})
	if cx.veces() != 0 || res.Status != "no-entregable" || res.ExitCode != 1 || len(res.Passes) != 0 {
		t.Fatalf("CA-417: la rama no sube el tope de la base (1 KiB): codex %d, %+v\n%s", cx.veces(), res, out)
	}
	if !strings.Contains(out, raNoEntregable(1)) {
		t.Fatalf("CA-417: la negativa nombra el tope de la base (1 KiB):\n%s", out)
	}
	if recs, _ := Records(root); len(recs) != 0 {
		t.Fatalf("CA-417: sin registro: %+v", recs)
	}

	root = raRamaConReview(t, "", "review:\n  max_evidence_kib: 1\n", true)
	write(t, root, "app.go", b.String())
	res, out = raRevisar(t, "CA-417", root, Options{Provider: "codex", Lens: "risk"})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-417: el tope de 1 KiB de la rama no vale; rige el de la base (320 KiB) y corre: %+v\n%s", res, out)
	}
	if !strings.Contains(out, ", tope 320 KiB - sha256 ") {
		t.Fatalf("CA-417: la linea evidencia dice el tope de la base (320 KiB):\n%s", out)
	}
}
