// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-395, CA-400..CA-407) sobre `hoom review`: provider, modelo, esfuerzo y
// same_provider se resuelven opcion > hoom.yaml > vacio; cada pasada corre
// aislada con el esfuerzo resuelto; hoom congela la evidencia (diff del
// candidato + spec) una vez, se la da entera a cada lente antes de la linea
// de la lente, y se niega a correr si pasa el tope; risk va primero; la
// salida y el registro dicen modelo, esfuerzo, aislamiento, evidencia y
// gasto por lente.
//
// Los CLIs de IA son falsos y viven en un PATH MINIMO (sistema + los falsos):
// ningun claude/codex real de esta maquina puede colarse. Cada invocacion
// guarda su argv (separado por NUL) y su stdin FUERA del arbol revisado, asi
// el gate de scope no los ve.
package reviewcmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/providers"
)

// ---------------------------------------------------------------- fixtures

// raPATH deja un PATH minimo: el directorio de los CLIs falsos y los del
// sistema (git, sh). Ningun CLI de IA real entra.
func raPATH(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return bin
}

// raCLI es un CLI de IA falso que deja rastro de cada invocacion.
type raCLI struct{ dir string }

// raInstalar pone en bin un CLI falso name. extra corre antes del exit 0 y
// ve $n, el numero de invocacion (1, 2, ...).
func raInstalar(t *testing.T, bin, name, extra string) *raCLI {
	t.Helper()
	dir := t.TempDir()
	s := "#!/bin/sh\nd='" + dir + "'\n" +
		"n=$(cat \"$d/n\" 2>/dev/null || echo 0); n=$((n+1)); echo $n > \"$d/n\"\n" +
		"printf '%s\\000' \"$@\" > \"$d/argv.$n\"\n" +
		"cat > \"$d/stdin.$n\"\n" +
		extra + "exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return &raCLI{dir: dir}
}

func (c *raCLI) veces() int {
	raw, err := os.ReadFile(filepath.Join(c.dir, "n"))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return n
}

func (c *raCLI) argv(t *testing.T, n int) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(c.dir, "argv."+strconv.Itoa(n)))
	if err != nil {
		t.Fatalf("la invocacion %d del CLI falso nunca corrio: %v", n, err)
	}
	args := strings.Split(string(raw), "\x00")
	if len(args) > 0 && args[len(args)-1] == "" {
		args = args[:len(args)-1]
	}
	return args
}

func (c *raCLI) stdin(t *testing.T, n int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(c.dir, "stdin."+strconv.Itoa(n)))
	if err != nil {
		t.Fatalf("la invocacion %d del CLI falso nunca corrio: %v", n, err)
	}
	return string(raw)
}

// pedido es el prompt que recibio la invocacion n: por stdin si el argv
// termina en "-" (codex) o si llego algo por stdin (claude sin prompt
// posicional); si no, el ultimo argumento.
func (c *raCLI) pedido(t *testing.T, n int) string {
	t.Helper()
	args, in := c.argv(t, n), c.stdin(t, n)
	if len(args) > 0 && args[len(args)-1] == "-" {
		return in
	}
	if in != "" {
		return in
	}
	if len(args) == 0 {
		t.Fatalf("la invocacion %d no trajo argumentos", n)
	}
	return args[len(args)-1]
}

// raUsoCodex: la invocacion n informa entrada n*1000, cache n*100, salida
// n*10 y un turno, como lo informa Codex (turn.completed).
const raUsoCodex = "printf '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":%d,\"cached_input_tokens\":%d,\"output_tokens\":%d}}\\n' $((n*1000)) $((n*100)) $((n*10))\n"

// raUsoPares: solo las invocaciones pares informan consumo.
const raUsoPares = "if [ $((n % 2)) -eq 0 ]; then " + "printf '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":%d,\"cached_input_tokens\":%d,\"output_tokens\":%d}}\\n' $((n*1000)) $((n*100)) $((n*10))" + "; fi\n"

// raRepo arma un proyecto: repo en main con hoom.yaml (mas reviewYAML), la
// telemetria escondida en .hoom/.gitignore y app.go commiteado.
func raRepo(t *testing.T, reviewYAML string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n"+reviewYAML)
	write(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial")
	return root
}

// raCambio planta un cambio de codigo chico sin commitear (una lente).
func raCambio(t *testing.T, root string) {
	t.Helper()
	write(t, root, "app.go", "package app\n\nfunc Nuevo() {}\n")
}

// raCuatro planta un cambio en una ruta de riesgo: las 4 lentes.
func raCuatro(t *testing.T, root string) {
	t.Helper()
	raCambio(t, root)
	write(t, root, "internal/auth/token.go", "package auth\n\n// Valida revisa un token: ñandú €\nfunc Valida(s string) bool { return s != \"\" }\n")
}

func raKiB(n int) int { return (n + 1023) / 1024 }

func raSHA(diff, spec []byte) string {
	h := sha256.New()
	h.Write(diff)
	h.Write(spec)
	return hex.EncodeToString(h.Sum(nil))
}

func raRevisar(t *testing.T, ca, root string, opt Options) (Result, string) {
	t.Helper()
	var out bytes.Buffer
	res, err := Run(root, "main", opt, &out)
	if err != nil {
		t.Fatalf("%s: la review no se rompe: %v\n%s", ca, err, out.String())
	}
	return res, out.String()
}

func raRegistros(t *testing.T, dir string) []Record {
	t.Helper()
	recs, avisos := Records(dir)
	if len(avisos) != 0 {
		t.Fatalf("los registros de review se leen sin avisos: %v", avisos)
	}
	return recs
}

func raRegistroCrudo(t *testing.T, dir, id string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", RecordsDir, id+".json"))
	if err != nil {
		t.Fatalf("el registro %s existe: %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("el registro %s es JSON: %v", id, err)
	}
	return m
}

func raTiene(args []string, a string) bool {
	for _, x := range args {
		if x == a {
			return true
		}
	}
	return false
}

func raPar(args []string, flag, valor string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == valor {
			return true
		}
	}
	return false
}

func raLineas(s string) []string { return strings.Split(s, "\n") }

// raLinea devuelve el indice de la primera linea (desde from) con ese
// prefijo, o -1.
func raLinea(lineas []string, from int, prefijo string) int {
	for i := from; i < len(lineas); i++ {
		if strings.HasPrefix(lineas[i], prefijo) {
			return i
		}
	}
	return -1
}

func raEvidencia(t *testing.T, ca, root, spec string) Evidencia {
	t.Helper()
	ev, err := Evidence(root, "main", gitx.Snapshot(root, "main"), spec)
	if err != nil {
		t.Fatalf("%s: Evidence: %v", ca, err)
	}
	if len(ev.Diff) == 0 {
		t.Fatalf("%s: Evidence arma el diff del candidato: salio vacio", ca)
	}
	return ev
}

// raEvidenciaCruda arma la evidencia sin exigir nada de su contenido: los
// tests que la comparan con lo que vio el reviewer fallan por su propio
// criterio, no por el fixture.
func raEvidenciaCruda(t *testing.T, ca, root, spec string) Evidencia {
	t.Helper()
	ev, err := Evidence(root, "main", gitx.Snapshot(root, "main"), spec)
	if err != nil {
		t.Fatalf("%s: Evidence: %v", ca, err)
	}
	return ev
}

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

// ---------------------------------------------------------------- CA-401

// CA-401: Evidence arma el diff del candidato contra el merge-base y el arbol
// de trabajo: rastreado commiteado o no, no rastreado como archivo nuevo,
// binario sin contenido, nada de .hoom/; mas el spec; Bytes y SHA256 cierran.
func TestCA401_EvidenciaDelCandidato(t *testing.T) {
	root := raRepo(t, "")
	write(t, root, "committed.go", "package app\n\nvar Base = 1\n")
	write(t, root, "imagen.bin", "\x00\x01MARCA-BINARIA-BASE\x00")
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nversion base\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base")
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, root, "committed.go", "package app\n\nvar Base = 2 // commiteado en la rama\n")
	git(t, root, "commit", "-q", "-am", "rama")
	// sin commitear, rastreado
	write(t, root, "app.go", "package app\n\nfunc SinCommitear() {}\n")
	// no rastreados
	write(t, root, "nuevo.go", "package app\n\n// archivo nuevo sin rastrear ñandú\nfunc Nuevo() {}\n")
	write(t, root, "dir con espacio/ñu.go", "package dir\n\n// contenido-unico-ñu\n")
	write(t, root, "nuevo.bin", "\x00MARCA-BINARIA-NUEVO-ARCHIVO\x00\xff")
	// binario rastreado y modificado
	write(t, root, "imagen.bin", "\x00\x02MARCA-BINARIA-NUEVA\x00\xff")
	// lo de .hoom/ no va nunca, ni el spec cambiado ni un archivo suelto
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nversion rama: SPEC-NO-VA-EN-EL-DIFF\n")
	write(t, root, ".hoom/notas.txt", "NOTA-HOOM-FUERA-DEL-DIFF\n")

	g := gitx.Snapshot(root, "main")
	for _, f := range []string{"committed.go", "app.go", "nuevo.go", "imagen.bin", ".hoom/specs/x.md"} {
		if !raTiene(g.ChangedFiles, f) {
			t.Fatalf("CA-401: fixture: %s es parte del cambio: %v", f, g.ChangedFiles)
		}
	}
	ev, err := Evidence(root, "main", g, ".hoom/specs/x.md")
	if err != nil {
		t.Fatalf("CA-401: Evidence: %v", err)
	}
	diff := string(ev.Diff)
	for _, quiero := range []string{
		"diff --git a/committed.go b/committed.go", "-var Base = 1", "+var Base = 2 // commiteado en la rama",
		"diff --git a/app.go b/app.go", "+func SinCommitear() {}",
		"+++ b/nuevo.go", "+// archivo nuevo sin rastrear ñandú",
		"+// contenido-unico-ñu",
		"imagen.bin", "nuevo.bin", "Binary files",
	} {
		if !strings.Contains(diff, quiero) {
			t.Fatalf("CA-401: el diff del candidato trae %q:\n%s", quiero, diff)
		}
	}
	if !strings.Contains(diff, "new file mode") {
		t.Fatalf("CA-401: un no rastreado va como parche de archivo nuevo:\n%s", diff)
	}
	for _, nunca := range []string{"MARCA-BINARIA", "GIT binary patch", "SPEC-NO-VA-EN-EL-DIFF", "NOTA-HOOM-FUERA-DEL-DIFF", "a/.hoom/", "b/.hoom/"} {
		if strings.Contains(diff, nunca) {
			t.Fatalf("CA-401: el diff no trae %q (binarios sin contenido, nada de .hoom/):\n%s", nunca, diff)
		}
	}
	spec, _ := os.ReadFile(filepath.Join(root, ".hoom", "specs", "x.md"))
	if !bytes.Equal(ev.Spec, spec) {
		t.Fatalf("CA-401: Spec es el texto del spec tal cual esta en el arbol: %q", ev.Spec)
	}
	if ev.Bytes != len(ev.Diff)+len(ev.Spec) {
		t.Fatalf("CA-401: Bytes = len(Diff)+len(Spec): %d vs %d+%d", ev.Bytes, len(ev.Diff), len(ev.Spec))
	}
	if ev.SHA256 != raSHA(ev.Diff, ev.Spec) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(ev.SHA256) {
		t.Fatalf("CA-401: SHA256 es el hex de Diff seguido de Spec: %q", ev.SHA256)
	}

	// determinista: armarla dos veces da los mismos bytes
	otra, err := Evidence(root, "main", gitx.Snapshot(root, "main"), ".hoom/specs/x.md")
	if err != nil || !bytes.Equal(otra.Diff, ev.Diff) || otra.SHA256 != ev.SHA256 {
		t.Fatalf("CA-401: la evidencia del mismo arbol es la misma: %v", err)
	}

	// el diff es el de git.ChangedFiles: un archivo que no esta en la lista
	// no entra
	sin := g
	sin.ChangedFiles = nil
	for _, f := range g.ChangedFiles {
		if f != "nuevo.go" {
			sin.ChangedFiles = append(sin.ChangedFiles, f)
		}
	}
	parcial, err := Evidence(root, "main", sin, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(parcial.Diff), "archivo nuevo sin rastrear") || !strings.Contains(string(parcial.Diff), "SinCommitear") {
		t.Fatalf("CA-401: el diff cubre exactamente git.ChangedFiles fuera de .hoom/:\n%s", parcial.Diff)
	}
}

// CA-401 (caso limite): sin --spec la evidencia no tiene bloque de spec:
// Spec vacio, Bytes = len(Diff), SHA256 del diff solo.
func TestCA401_SinSpecNoHayBloqueDeSpec(t *testing.T) {
	root := raRepo(t, "")
	raCambio(t, root)
	ev, err := Evidence(root, "main", gitx.Snapshot(root, "main"), "")
	if err != nil {
		t.Fatalf("CA-401: %v", err)
	}
	if len(ev.Spec) != 0 || ev.Bytes != len(ev.Diff) || len(ev.Diff) == 0 {
		t.Fatalf("CA-401: sin spec, Spec vacio y Bytes = len(Diff) > 0: %+v", ev)
	}
	if ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-401: sin spec el SHA256 es el del diff: %q", ev.SHA256)
	}
	if !strings.Contains(string(ev.Diff), "+func Nuevo() {}") {
		t.Fatalf("CA-401: el diff trae el cambio sin commitear:\n%s", ev.Diff)
	}
}

// CA-401: el diff sale del MERGE-BASE, no de la punta de la base: lo que la
// base hizo despues de abrir la rama no aparece (ni revertido).
func TestCA401_DiffDesdeElMergeBase(t *testing.T) {
	root := raRepo(t, "")
	write(t, root, "app.go", "package app\n\n// linea base\n")
	git(t, root, "commit", "-q", "-am", "base")
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, root, "rama.go", "package app\n\nvar EnLaRama = true\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "rama")
	git(t, root, "checkout", "-q", "main")
	write(t, root, "app.go", "package app\n\n// linea base\n// CAMBIO-EN-MAIN-DESPUES-DE-LA-RAMA\n")
	write(t, root, "solo_main.go", "package app\n\n// SOLO-EN-MAIN\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "main avanza")
	git(t, root, "checkout", "-q", "feature")
	write(t, root, "app.go", "package app\n\n// linea base\n// cambio de la rama sin commitear\n")

	ev := raEvidencia(t, "CA-401", root, "")
	diff := string(ev.Diff)
	if !strings.Contains(diff, "+var EnLaRama = true") || !strings.Contains(diff, "+// cambio de la rama sin commitear") {
		t.Fatalf("CA-401: el diff trae lo commiteado en la rama y lo sin commitear:\n%s", diff)
	}
	if strings.Contains(diff, "CAMBIO-EN-MAIN-DESPUES-DE-LA-RAMA") || strings.Contains(diff, "SOLO-EN-MAIN") || strings.Contains(diff, "solo_main.go") {
		t.Fatalf("CA-401: lo que la base hizo despues de la rama no es del candidato:\n%s", diff)
	}
}

// CA-401: las 4 pasadas reciben exactamente los mismos bytes de evidencia, y
// son los de Evidence; el Result y el registro dicen Bytes y SHA256.
func TestCA401_LasCuatroPasadasRecibenLosMismosBytes(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nCriterio con unicode: ñandú €\n")
	cx := raInstalar(t, bin, "codex", "")
	ev := raEvidenciaCruda(t, "CA-401", root, ".hoom/specs/x.md")

	res, out := raRevisar(t, "CA-401", root, Options{Provider: "codex", Spec: ".hoom/specs/x.md"})
	if res.Status != "revisado" || cx.veces() != 4 {
		t.Fatalf("CA-401: fixture: las 4 lentes corren: %+v\n%s", res, out)
	}
	const fin = "=== fin de la evidencia ==="
	var prefijo string
	for n := 1; n <= 4; n++ {
		p := cx.pedido(t, n)
		if !strings.Contains(p, "=== diff ===\n"+string(ev.Diff)) {
			t.Fatalf("CA-401: la pasada %d recibe el diff entero, byte a byte", n)
		}
		if !strings.Contains(p, "=== spec .hoom/specs/x.md ===\n"+string(ev.Spec)) {
			t.Fatalf("CA-401: la pasada %d recibe el spec entero", n)
		}
		i := strings.Index(p, fin)
		if i < 0 {
			t.Fatalf("CA-401: la pasada %d cierra la evidencia con %q", n, fin)
		}
		if n == 1 {
			prefijo = p[:i+len(fin)]
		} else if p[:i+len(fin)] != prefijo {
			t.Fatalf("CA-401: la pasada %d recibe otra evidencia que la 1", n)
		}
	}
	if res.EvidenceBytes != ev.Bytes || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-401: el Result dice la evidencia que recibieron las lentes: %d %q vs %d %q",
			res.EvidenceBytes, res.EvidenceSHA256, ev.Bytes, ev.SHA256)
	}
	recs := raRegistros(t, root)
	if len(recs) != 1 || recs[0].EvidenceBytes != ev.Bytes || recs[0].EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-401: el registro dice la evidencia: %+v", recs)
	}
}

// CA-401 (caso limite): --lens x recibe la misma evidencia, en una pasada.
func TestCA401_UnaLenteMismaEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	cx := raInstalar(t, bin, "codex", "")
	ev := raEvidenciaCruda(t, "CA-401", root, "")

	res, out := raRevisar(t, "CA-401", root, Options{Provider: "codex", Lens: "resilience"})
	if cx.veces() != 1 || len(res.Passes) != 1 {
		t.Fatalf("CA-401: --lens es una pasada: %+v\n%s", res, out)
	}
	if !strings.Contains(cx.pedido(t, 1), "=== diff ===\n"+string(ev.Diff)) || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-401: la pasada unica recibe la misma evidencia entera")
	}
}

// ---------------------------------------------------------------- CA-402

func raNoEntregable(n, m int) string {
	return fmt.Sprintf("hoom review: NO ENTREGABLE - la evidencia (%d KiB) pasa el tope (%d KiB): hoom no la corta; "+
		"parti el cambio o subi review.max_evidence_kib si el modelo del reviewer la aguanta", n, m)
}

func raHallazgos(t *testing.T, dir string) int {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(dir, ".hoom", "findings"))
	return len(entries)
}

// CA-402: con la evidencia sobre el tope de hoom.yaml, ninguna pasada, ni
// registro ni hallazgos, exit 1 y el texto del contrato; el mismo cambio con
// el tope por defecto corre (el tope de hoom.yaml lo cambia).
func TestCA402_EvidenciaSobreElTopeNoLanzaPasadas(t *testing.T) {
	bin := raPATH(t)
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "var Relleno%02d = \"cuarenta bytes de relleno por linea\"\n", i)
	}
	root := raRepo(t, "review:\n  max_evidence_kib: 1\n")
	write(t, root, "app.go", b.String())
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if cx.veces() != 0 {
		t.Fatalf("CA-402: sobre el tope no se lanza ninguna pasada (codex corrio %d veces)\n%s", cx.veces(), out)
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 || len(res.Passes) != 0 {
		t.Fatalf("CA-402: sobre el tope es no-entregable con exit 1 y sin pasadas: %+v\n%s", res, out)
	}
	if recs, _ := Records(root); len(recs) != 0 {
		t.Fatalf("CA-402: sin registro de review: %+v", recs)
	}
	if n := raHallazgos(t, root); n != 0 {
		t.Fatalf("CA-402: sin hallazgos: %d", n)
	}
	ev := raEvidencia(t, "CA-402", root, "")
	if !strings.Contains(out, raNoEntregable(raKiB(ev.Bytes), 1)+"\n") {
		t.Fatalf("CA-402: la salida dice %q:\n%s", raNoEntregable(raKiB(ev.Bytes), 1), out)
	}

	// el mismo cambio, sin tope en hoom.yaml (320 KiB), corre
	root2 := raRepo(t, "")
	write(t, root2, "app.go", b.String())
	res, out = raRevisar(t, "CA-402", root2, Options{Provider: "codex"})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-402: bajo el tope por defecto la misma evidencia corre: %+v\n%s", res, out)
	}
}

// CA-402: igual al tope corre; un byte mas, no (y el KiB de la negativa se
// redondea hacia arriba).
func TestCA402_IgualAlTopeCorreYUnByteMasNo(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  max_evidence_kib: 2\n")
	cx := raInstalar(t, bin, "codex", "")
	relleno := func(k int) { write(t, root, "relleno.go", "// "+strings.Repeat("x", k)+"\n") }

	relleno(1000)
	ev := raEvidencia(t, "CA-402", root, "")
	if ev.Bytes > 2048 {
		t.Fatalf("CA-402: fixture: con 1000 bytes de relleno la evidencia no pasa 2 KiB: %d", ev.Bytes)
	}
	k := 1000 + 2048 - ev.Bytes
	relleno(k)
	if ev = raEvidencia(t, "CA-402", root, ""); ev.Bytes != 2048 {
		t.Fatalf("CA-402: fixture: la evidencia queda exactamente en 2048 bytes: %d", ev.Bytes)
	}
	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-402: igual al tope (2048 bytes, 2 KiB) corre: %+v\n%s", res, out)
	}

	relleno(k + 1) // 2049 bytes de evidencia
	if ev = raEvidencia(t, "CA-402", root, ""); ev.Bytes != 2049 {
		t.Fatalf("CA-402: fixture: un byte mas da 2049: %d", ev.Bytes)
	}
	antes := len(raRegistros(t, root))
	res, out = raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if res.Status != "no-entregable" || res.ExitCode != 1 || cx.veces() != 1 {
		t.Fatalf("CA-402: un byte sobre el tope no corre: %+v\n%s", res, out)
	}
	if len(raRegistros(t, root)) != antes {
		t.Fatal("CA-402: la negativa no escribe registro")
	}
	if !strings.Contains(out, raNoEntregable(3, 2)) {
		t.Fatalf("CA-402: 2049 bytes son 3 KiB (hacia arriba) y el tope 2:\n%s", out)
	}
}

// CA-402: sin tope en hoom.yaml el tope es 320 KiB: una evidencia mas grande
// se niega nombrandolo.
func TestCA402_TopePorDefecto320KiB(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; b.Len() < 330*1024; i++ {
		fmt.Fprintf(&b, "var Grande%05d = \"una linea de relleno para pasar el tope\"\n", i)
	}
	write(t, root, "grande.go", b.String())
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if cx.veces() != 0 || res.ExitCode != 1 || res.Status != "no-entregable" {
		t.Fatalf("CA-402: 330 KiB pasan el tope por defecto: codex %d, %+v\n%s", cx.veces(), res, out)
	}
	ev := raEvidencia(t, "CA-402", root, "")
	if !strings.Contains(out, raNoEntregable(raKiB(ev.Bytes), 320)) {
		t.Fatalf("CA-402: la negativa nombra el tope por defecto (320 KiB):\n%s", out)
	}
}

// CA-402 (caso limite): 0 lentes (solo documentacion) no arma evidencia y
// sigue SIN REVISAR, aunque la documentacion pase el tope. Guarda: el
// esqueleto ya es asi; el tope no puede adelantarse a la regla de 0 lentes.
func TestCA402_CeroLentesNoArmaEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  max_evidence_kib: 1\n")
	write(t, root, "README.md", strings.Repeat("documentacion larga\n", 400))
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if res.Status != "sin-revisar" || res.ExitCode != 0 || cx.veces() != 0 || len(res.Lenses) != 0 {
		t.Fatalf("CA-402: solo documentacion es SIN REVISAR como hoy: %+v\n%s", res, out)
	}
	if res.EvidenceBytes != 0 || res.EvidenceSHA256 != "" || strings.Contains(out, "NO ENTREGABLE") {
		t.Fatalf("CA-402: sin lentes no se arma evidencia: %+v\n%s", res, out)
	}
}

// ---------------------------------------------------------------- CA-403

// raRevisarPedido comprueba la forma del pedido de una lente.
func raRevisarPedido(t *testing.T, ped, lens string, g gitx.Info, ev Evidencia, spec, lineaVeredicto string) {
	t.Helper()
	lineas := raLineas(ped)
	re := regexp.MustCompile(`^Revisa el cambio de esta rama\. La evidencia completa esta abajo, congelada por hoom \(sha256 ([0-9a-f]{64}), (\d+) KiB\): no vuelvas a sacar el diff; lee otros archivos solo por rangos y solo si hace falta\.$`)
	m := re.FindStringSubmatch(lineas[0])
	if m == nil {
		t.Fatalf("CA-403 (%s): la primera linea del pedido es la del contrato, fue %q", lens, lineas[0])
	}
	if m[1] != ev.SHA256 || m[2] != strconv.Itoa(raKiB(ev.Bytes)) {
		t.Fatalf("CA-403 (%s): la primera linea dice sha256 %s y %d KiB: %q", lens, ev.SHA256, raKiB(ev.Bytes), lineas[0])
	}
	base := fmt.Sprintf("Base: main. Tamano: %d archivos, +%d/-%d lineas.", len(g.ChangedFiles), g.Insertions, g.Deletions)
	if lineas[1] != base {
		t.Fatalf("CA-403 (%s): la segunda linea es %q, fue %q", lens, base, lineas[1])
	}
	if lineas[2] != lineaVeredicto {
		t.Fatalf("CA-403 (%s): la tercera linea es la del veredicto de hoy %q, fue %q", lens, lineaVeredicto, lineas[2])
	}
	if spec != "" {
		if lineas[3] != "Spec: "+spec || lineas[4] != "=== spec "+spec+" ===" {
			t.Fatalf("CA-403 (%s): con spec siguen 'Spec: %s' y '=== spec %s ===': %q %q", lens, spec, spec, lineas[3], lineas[4])
		}
		if !strings.Contains(ped, "=== spec "+spec+" ===\n"+string(ev.Spec)) {
			t.Fatalf("CA-403 (%s): el bloque del spec trae el texto entero", lens)
		}
	} else if strings.Contains(ped, "=== spec ") || raLinea(lineas, 0, "Spec:") >= 0 {
		t.Fatalf("CA-403 (%s): sin spec no hay linea Spec: ni bloque de spec:\n%s", lens, ped)
	}
	iDiff := strings.Index(ped, "=== diff ===\n"+string(ev.Diff))
	const fin = "=== fin de la evidencia ==="
	iFin := strings.Index(ped, fin)
	if iDiff < 0 || iFin < iDiff {
		t.Fatalf("CA-403 (%s): el diff entero va en su bloque, antes de %q", lens, fin)
	}
	if spec != "" && strings.Index(ped, "=== spec "+spec+" ===") > iDiff {
		t.Fatalf("CA-403 (%s): el spec va antes del diff", lens)
	}
	resto := ped[iFin+len(fin):]
	lente := "\nRevisalo con la lente " + lens + ". Solo esa lente.\nRegistra cada hallazgo"
	if !strings.HasPrefix(resto, lente) {
		t.Fatalf("CA-403 (%s): despues de la evidencia van la linea de la lente y 'Registra cada hallazgo': %q", lens, resto[:min(len(resto), 200)])
	}
	if !strings.Contains(resto, " finding add --sev low|medium|high --lens "+lens+" --file <ruta> --author reviewer@codex ") {
		t.Fatalf("CA-403 (%s): se conserva la linea de hoom finding add de CA-162:\n%s", lens, resto)
	}
	if !strings.Contains(resto, "El chat no es registro: lo que no quede como hallazgo, no paso.\n") ||
		!strings.HasSuffix(strings.TrimRight(resto, "\n"), "No edites codigo: este arbol es de solo lectura para vos.") {
		t.Fatalf("CA-403 (%s): el pedido cierra con las lineas de hoy:\n%s", lens, resto)
	}
	if strings.Contains(ped, "El diff lo sacas vos") || raLinea(lineas, 0, "Archivos:") >= 0 {
		t.Fatalf("CA-403 (%s): desaparecen 'El diff lo sacas vos' y la lista Archivos:", lens)
	}
}

// CA-403: el pedido empieza con la evidencia congelada, es identico entre
// lentes hasta '=== fin de la evidencia ===' inclusive, sigue con la linea
// de la lente y conserva la de hoom finding add (re-expresa el prefijo de
// CA-337).
func TestCA403_PedidoConLaEvidenciaAntesDeLaLente(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\n- CA-1: algo.\n")
	v := rcVeredicto(t, root)
	cx := raInstalar(t, bin, "codex", "")
	g := gitx.Snapshot(root, "main")
	ev := raEvidenciaCruda(t, "CA-403", root, ".hoom/specs/x.md")

	res, out := raRevisar(t, "CA-403", root, Options{Provider: "codex", Spec: ".hoom/specs/x.md"})
	if res.Status != "revisado" || cx.veces() != 4 {
		t.Fatalf("CA-403: fixture: las 4 lentes corren: %+v\n%s", res, out)
	}
	lineaV := fmt.Sprintf("Veredicto vigente: %s (%s).", v.ID, v.Verdict)
	const fin = "=== fin de la evidencia ==="
	var prefijo string
	for n, lens := range res.Lenses {
		p := cx.pedido(t, n+1)
		raRevisarPedido(t, p, lens, g, ev, ".hoom/specs/x.md", lineaV)
		pre := p[:strings.Index(p, fin)+len(fin)]
		if n == 0 {
			prefijo = pre
		} else if pre != prefijo {
			t.Fatalf("CA-403: el pedido de %s difiere del de %s antes de %q", lens, res.Lenses[0], fin)
		}
	}
}

// CA-403 (caso limite): sin --spec el pedido no tiene bloque de spec, y sin
// veredicto dice la linea de hoy.
func TestCA403_PedidoSinSpecNiVeredicto(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")
	g := gitx.Snapshot(root, "main")
	ev := raEvidenciaCruda(t, "CA-403", root, "")

	res, _ := raRevisar(t, "CA-403", root, Options{Provider: "codex", Lens: "risk"})
	if cx.veces() != 1 || res.Status != "revisado" {
		t.Fatalf("CA-403: fixture: una pasada: %+v", res)
	}
	raRevisarPedido(t, cx.pedido(t, 1), "risk", g, ev, "",
		"No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
}

// ---------------------------------------------------------------- CA-404

// CA-404: las lentes son risk, reliability, resilience, readability: en
// Lentes, en la regla, en el error de una lente desconocida, en el orden de
// ejecucion y en Record.Lenses (re-expresa el orden de CA-160).
func TestCA404_RiskPrimero(t *testing.T) {
	want := []string{"risk", "reliability", "resilience", "readability"}
	if !reflect.DeepEqual(Lentes, want) {
		t.Fatalf("CA-404: Lentes es %v, fue %v", want, Lentes)
	}
	lentes, _, err := Lenses(gitx.Info{ChangedFiles: []string{"internal/auth/login.go"}, Insertions: 3}, "")
	if err != nil || !reflect.DeepEqual(lentes, want) {
		t.Fatalf("CA-404: la regla del contrato 06 da las 4 en ese orden: %v %v", lentes, err)
	}
	lentes, _, _ = Lenses(gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 500}, "")
	if !reflect.DeepEqual(lentes, want) {
		t.Fatalf("CA-404: por tamano tambien, en ese orden: %v", lentes)
	}
	if _, _, err := Lenses(gitx.Info{ChangedFiles: []string{"a.go"}}, "profundidad"); err == nil ||
		!strings.Contains(err.Error(), "risk, reliability, resilience, readability") {
		t.Fatalf("CA-404: una lente invalida lista las 4 en el orden nuevo: %v", err)
	}

	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	cx := raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-404", root, Options{Provider: "codex"})
	if !reflect.DeepEqual(res.Lenses, want) || len(res.Passes) != 4 {
		t.Fatalf("CA-404: el Result lista las lentes en orden: %+v\n%s", res, out)
	}
	for i, lens := range want {
		if res.Passes[i].Lens != lens {
			t.Fatalf("CA-404: la pasada %d es %s: %+v", i+1, lens, res.Passes[i])
		}
		if !strings.Contains(cx.pedido(t, i+1), "Revisalo con la lente "+lens+". Solo esa lente.") {
			t.Fatalf("CA-404: la invocacion %d es la de %s", i+1, lens)
		}
		if raLinea(raLineas(out), 0, fmt.Sprintf("  [%d/4] %s", i+1, lens)) < 0 {
			t.Fatalf("CA-404: la salida corre [%d/4] %s:\n%s", i+1, lens, out)
		}
	}
	recs := raRegistros(t, root)
	if len(recs) != 1 || !reflect.DeepEqual(recs[0].Lenses, want) {
		t.Fatalf("CA-404: Record.Lenses conserva el orden: %+v", recs)
	}
}

// ---------------------------------------------------------------- CA-405

// raCabecera devuelve las 4 lineas de la cabecera, exigiendo que vayan
// juntas, despues de la linea reviewer y antes de [1/n].
func raCabecera(t *testing.T, out string) []string {
	t.Helper()
	lineas := raLineas(out)
	r := raLinea(lineas, 0, "  reviewer    ")
	m := raLinea(lineas, 0, "  modelo      ")
	p := raLinea(lineas, 0, "  [1/")
	if r < 0 || m < 0 || p < 0 || !(r < m && m+3 < p) {
		t.Fatalf("CA-405: modelo, esfuerzo, aislado y evidencia van despues de reviewer y antes de la primera lente:\n%s", out)
	}
	cab := lineas[m : m+4]
	for i, pre := range []string{"  modelo      ", "  esfuerzo    ", "  aislado     ", "  evidencia   "} {
		if !strings.HasPrefix(cab[i], pre) {
			t.Fatalf("CA-405: la linea %d de la cabecera empieza con %q: %q\n%s", i+1, pre, cab[i], out)
		}
	}
	return cab
}

// CA-405: con modelo y esfuerzo elegidos la cabecera los dice, dice que el
// reviewer esta aislado y la evidencia con sus KiB (hacia arriba), el tope y
// los 12 primeros hex del sha256.
func TestCA405_CabeceraConModeloEsfuerzoYEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  model: gpt-5.6-sol\n  effort: xhigh\n  max_evidence_kib: 64\n")
	raCambio(t, root)
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\n"+strings.Repeat("texto del spec\n", 90))
	raInstalar(t, bin, "codex", "")
	ev := raEvidenciaCruda(t, "CA-405", root, ".hoom/specs/x.md")

	res, out := raRevisar(t, "CA-405", root, Options{Provider: "codex", Lens: "risk", Spec: ".hoom/specs/x.md"})
	if res.Status != "revisado" {
		t.Fatalf("CA-405: fixture: la review corre: %+v\n%s", res, out)
	}
	cab := raCabecera(t, out)
	quiero := []string{
		"  modelo      gpt-5.6-sol",
		"  esfuerzo    xhigh",
		"  aislado     si - sin la config personal del provider",
		fmt.Sprintf("  evidencia   %d KiB (diff %d + spec %d), tope 64 KiB - sha256 %s",
			raKiB(ev.Bytes), raKiB(len(ev.Diff)), raKiB(len(ev.Spec)), ev.SHA256[:12]),
	}
	if !reflect.DeepEqual(cab, quiero) {
		t.Fatalf("CA-405: la cabecera es\n%s\nfue\n%s", strings.Join(quiero, "\n"), strings.Join(cab, "\n"))
	}
	if raKiB(len(ev.Spec)) < 2 {
		t.Fatalf("CA-405: fixture: el spec pasa 1 KiB para probar el redondeo: %d bytes", len(ev.Spec))
	}
}

// CA-405 y CA-395: sin modelo ni esfuerzo (ni opcion ni hoom.yaml) la
// cabecera dice 'por defecto del provider (no elegido)', el argv no lleva -m
// ni esfuerzo y el Result y el registro dicen "" (no elegido); sin spec, la
// evidencia es 'spec 0'.
func TestCA405_CabeceraPorDefectoNoElegido(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")
	ev := raEvidenciaCruda(t, "CA-405", root, "")

	res, out := raRevisar(t, "CA-405", root, Options{Provider: "codex", Lens: "risk"})
	cab := raCabecera(t, out)
	quiero := []string{
		"  modelo      por defecto del provider (no elegido)",
		"  esfuerzo    por defecto del provider (no elegido)",
		"  aislado     si - sin la config personal del provider",
		fmt.Sprintf("  evidencia   %d KiB (diff %d + spec 0), tope 320 KiB - sha256 %s", raKiB(ev.Bytes), raKiB(len(ev.Diff)), ev.SHA256[:12]),
	}
	if !reflect.DeepEqual(cab, quiero) {
		t.Fatalf("CA-405: la cabecera por defecto es\n%s\nfue\n%s", strings.Join(quiero, "\n"), strings.Join(cab, "\n"))
	}
	for _, a := range cx.argv(t, 1) {
		if a == "-m" || strings.HasPrefix(a, "model_reasoning_effort=") {
			t.Fatalf("CA-395: vacio = el del provider: ni -m ni esfuerzo en el argv: %v", cx.argv(t, 1))
		}
	}
	if res.Model != "" || res.Effort != "" {
		t.Fatalf("CA-395: el Result registra \"\" (no elegido): %+v", res)
	}
	m := raRegistroCrudo(t, root, res.RecordID)
	if v, ok := m["model"]; !ok || v != "" {
		t.Fatalf("CA-407: el registro trae model \"\": %v", m)
	}
	if v, ok := m["effort"]; !ok || v != "" {
		t.Fatalf("CA-407: el registro trae effort \"\": %v", m)
	}
}

// ---------------------------------------------------------------- CA-406

// CA-406: cada pasada imprime su gasto con los numeros de providers.Usage,
// despues de su linea de hallazgos; el resumen imprime el total de las 4
// lentes antes de 'registro'; Result.passes[].usage los trae; el sobre de la
// review sigue sin gasto (CA-334).
func TestCA406_GastoPorPasadaYTotal(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	raInstalar(t, bin, "codex", raUsoCodex)

	res, out := raRevisar(t, "CA-406", root, Options{Provider: "codex"})
	if res.Status != "revisado" || len(res.Passes) != 4 {
		t.Fatalf("CA-406: fixture: las 4 lentes corren: %+v\n%s", res, out)
	}
	lineas := raLineas(out)
	for i := 1; i <= 4; i++ {
		ini := raLinea(lineas, 0, fmt.Sprintf("  [%d/4] ", i))
		fin := raLinea(lineas, ini+1, "  [")
		if fin < 0 {
			fin = len(lineas)
		}
		h := raLinea(lineas, ini, "    hallazgos ")
		g := raLinea(lineas, ini, "    gasto     ")
		quiero := fmt.Sprintf("    gasto     entrada %d - cache %d - salida %d - turnos 1", i*1000, i*100, i*10)
		if ini < 0 || h < 0 || g < 0 || !(h < g && g < fin) || lineas[g] != quiero {
			t.Fatalf("CA-406: la pasada %d imprime %q despues de sus hallazgos:\n%s", i, quiero, out)
		}
		u := res.Passes[i-1].Usage
		if u == nil || u.InputTokens != i*1000 || u.CachedTokens != i*100 || u.OutputTokens != i*10 || u.Turns != 1 {
			t.Fatalf("CA-406: Result.passes[%d].usage trae lo que informo el provider: %+v", i-1, u)
		}
	}
	total := "  gasto       4 lentes: entrada 10000 - cache 1000 - salida 100"
	gt := raLinea(lineas, 0, "  gasto       ")
	reg := raLinea(lineas, 0, "  registro    ")
	if gt < 0 || lineas[gt] != total || reg < 0 || gt > reg || gt < raLinea(lineas, 0, "  [4/4] ") {
		t.Fatalf("CA-406: el resumen imprime %q antes de registro:\n%s", total, out)
	}
	raw, _ := json.Marshal(res)
	var m struct {
		Passes []map[string]json.RawMessage `json:"passes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || len(m.Passes) != 4 {
		t.Fatal(err)
	}
	for i, p := range m.Passes {
		if _, ok := p["usage"]; !ok {
			t.Fatalf("CA-406: el JSON de la pasada %d trae usage: %s", i+1, raw)
		}
	}
	recs := envelope.List(root)
	if len(recs) != 1 || !recs[0].Usage.Empty() {
		t.Fatalf("CA-406/CA-334: el sobre de la review sigue sin gasto: %+v", recs)
	}
}

// CA-406: con una lente el resumen dice '1 lente:'; una pasada sin consumo
// informado lo dice, y su usage queda omitido.
func TestCA406_UnaLenteYSinConsumo(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	raInstalar(t, bin, "codex", raUsoCodex)
	res, out := raRevisar(t, "CA-406", root, Options{Provider: "codex", Lens: "risk"})
	if !strings.Contains(out, "\n    gasto     entrada 1000 - cache 100 - salida 10 - turnos 1\n") ||
		!strings.Contains(out, "\n  gasto       1 lente: entrada 1000 - cache 100 - salida 10\n") {
		t.Fatalf("CA-406: una lente: gasto de la pasada y '1 lente:' en el resumen:\n%s", out)
	}
	if res.Passes[0].Usage == nil {
		t.Fatalf("CA-406: la pasada trae su usage: %+v", res.Passes[0])
	}

	root2 := raRepo(t, "")
	raCambio(t, root2)
	bin2 := raPATH(t)
	raInstalar(t, bin2, "codex", "")
	res, out = raRevisar(t, "CA-406", root2, Options{Provider: "codex", Lens: "risk"})
	if !strings.Contains(out, "\n    gasto     el provider no informo consumo\n") {
		t.Fatalf("CA-406: sin consumo informado la pasada lo dice:\n%s", out)
	}
	if res.Passes[0].Usage != nil {
		t.Fatalf("CA-406: sin datos Pass.Usage es nil: %+v", res.Passes[0].Usage)
	}
	raw, _ := json.Marshal(res.Passes[0])
	if strings.Contains(string(raw), `"usage"`) {
		t.Fatalf("CA-406: sin datos el JSON de la pasada omite usage: %s", raw)
	}
}

// ---------------------------------------------------------------- CA-407

// CA-407: el registro trae model, effort, isolated, evidence_bytes,
// evidence_sha256 y usage con una entrada por pasada CON datos (aca, las
// pares: reliability y readability); el total solo suma lo informado.
func TestCA407_RegistroConModeloEsfuerzoEvidenciaYGasto(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	raInstalar(t, bin, "codex", raUsoPares)
	ev := raEvidenciaCruda(t, "CA-407", root, "")

	res, out := raRevisar(t, "CA-407", root, Options{Provider: "codex", Model: "m1", Effort: "e1"})
	if res.Status != "revisado" || res.RecordID == "" {
		t.Fatalf("CA-407: fixture: la review termina revisada: %+v\n%s", res, out)
	}
	m := raRegistroCrudo(t, root, res.RecordID)
	if m["model"] != "m1" || m["effort"] != "e1" || m["isolated"] != true {
		t.Fatalf("CA-407: el registro trae model, effort e isolated: %v", m)
	}
	if m["evidence_bytes"] != float64(ev.Bytes) || m["evidence_sha256"] != ev.SHA256 {
		t.Fatalf("CA-407: el registro trae evidence_bytes %d y evidence_sha256 %s: %v", ev.Bytes, ev.SHA256, m)
	}
	usos, ok := m["usage"].([]any)
	if !ok || len(usos) != 2 {
		t.Fatalf("CA-407: usage es una lista con una entrada por pasada con datos (2): %v", m["usage"])
	}
	quiero := []map[string]any{
		{"lens": "reliability", "input_tokens": 2000.0, "cached_input_tokens": 200.0, "output_tokens": 20.0, "turns": 1.0},
		{"lens": "readability", "input_tokens": 4000.0, "cached_input_tokens": 400.0, "output_tokens": 40.0, "turns": 1.0},
	}
	for i, u := range usos {
		e, _ := u.(map[string]any)
		if !reflect.DeepEqual(e, quiero[i]) {
			t.Fatalf("CA-407: la entrada %d de usage es %v, fue %v", i, quiero[i], e)
		}
	}
	recs := raRegistros(t, root)
	if len(recs) != 1 || len(recs[0].Usage) != 2 || recs[0].Usage[0].Lens != "reliability" || recs[0].Usage[1].InputTokens != 4000 {
		t.Fatalf("CA-407: Records lee el gasto por lente: %+v", recs)
	}
	// CA-406 (caso hostil): el total suma solo las pasadas que informaron, y
	// las que no, lo dicen
	if !strings.Contains(out, "\n  gasto       4 lentes: entrada 6000 - cache 600 - salida 60\n") ||
		strings.Count(out, "\n    gasto     el provider no informo consumo\n") != 2 {
		t.Fatalf("CA-406: el total suma solo las pasadas con datos (2 y 4) y las otras dos dicen que no informaron:\n%s", out)
	}
}

// CA-407: sin consumo informado usage es [] (nunca null), y el JSON del
// Result trae model, effort, isolated, evidence_bytes y evidence_sha256.
func TestCA407_SinConsumoUsageEsListaVacia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-407", root, Options{Provider: "codex", Lens: "risk"})
	if res.RecordID == "" {
		t.Fatalf("CA-407: fixture: la review deja registro: %+v\n%s", res, out)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", RecordsDir, res.RecordID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if u, ok := m["usage"]; !ok || strings.TrimSpace(string(u)) != "[]" {
		t.Fatalf("CA-407: sin datos usage es [] y no null: %s", raw)
	}
	for _, k := range []string{"model", "effort", "isolated", "evidence_bytes", "evidence_sha256"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("CA-407: al registro le falta %q: %s", k, raw)
		}
	}
	rj, _ := json.Marshal(res)
	var r map[string]json.RawMessage
	json.Unmarshal(rj, &r)
	for _, k := range []string{"model", "effort", "isolated", "evidence_bytes", "evidence_sha256"} {
		if _, ok := r[k]; !ok {
			t.Fatalf("CA-407: al JSON del Result le falta %q: %s", k, rj)
		}
	}
	if string(r["isolated"]) != "true" || string(r["evidence_bytes"]) == "0" {
		t.Fatalf("CA-407: el Result dice isolated true y los bytes de la evidencia: %s", rj)
	}
}

// CA-407: un registro viejo, sin model/effort/isolated/evidence/usage, se
// sigue leyendo sin avisos y con los campos en cero. Guarda: los lectores de
// hoy ya lo leen; la implementacion no puede volverlos estrictos.
func TestCA407_RegistroViejoSeSigueLeyendo(t *testing.T) {
	dir := t.TempDir()
	viejo := `{
  "id": "20260920T100000_v1ej0a",
  "created_at": "2026-09-20T10:00:00Z",
  "task": "precios",
  "spec": ".hoom/specs/precios.md",
  "fingerprint": "abc",
  "verdict_id": "v1",
  "verdict": "green",
  "lenses": ["readability", "reliability", "resilience", "risk"],
  "provider": "codex",
  "writer": "claude",
  "cross": "cruzada",
  "writers_declared": [],
  "findings": []
}
`
	write(t, dir, filepath.Join(".hoom", RecordsDir, "20260920T100000_v1ej0a.json"), viejo)
	recs, avisos := Records(dir)
	if len(avisos) != 0 || len(recs) != 1 {
		t.Fatalf("CA-407: un registro viejo se lee sin avisos: %v %+v", avisos, recs)
	}
	r := recs[0]
	if r.Model != "" || r.Effort != "" || r.Isolated || r.EvidenceBytes != 0 || r.EvidenceSHA256 != "" || len(r.Usage) != 0 {
		t.Fatalf("CA-407: lo que el registro viejo no dice queda en cero: %+v", r)
	}
	if r.Provider != "codex" || r.Cross != CrossYes || len(r.Lenses) != 4 {
		t.Fatalf("CA-407: lo que el registro viejo dice se conserva: %+v", r)
	}
}

// ---------------------------------------------------------------- CA-399 / CA-402 en la review

// CA-399: una evidencia que deja el pedido por encima de 16 KiB (y bajo el
// tope) viaja entera por stdin: codex termina su argv en "-" y claude no
// lleva prompt posicional; el reviewer recibe el diff entero.
func TestCA399_ReviewConEvidenciaGrandeViajaPorStdin(t *testing.T) {
	bin := raPATH(t)
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; b.Len() < 40*1024; i++ {
		fmt.Fprintf(&b, "// linea %04d de un cambio grande con \"comillas\", $HOME y ñandú\n", i)
	}
	for _, prov := range []string{"codex", "claude"} {
		root := raRepo(t, "")
		write(t, root, "grande.go", b.String())
		cli := raInstalar(t, bin, prov, "")

		res, out := raRevisar(t, "CA-399", root, Options{Provider: prov, Lens: "risk"})
		if res.Status != "revisado" || cli.veces() != 1 {
			t.Fatalf("CA-399: %s: la review corre: %+v\n%s", prov, res, out)
		}
		args, in := cli.argv(t, 1), cli.stdin(t, 1)
		if len(in) <= providers.StdinPromptBytes {
			t.Fatalf("CA-399: %s: el pedido con la evidencia grande llega por stdin (llegaron %d bytes)", prov, len(in))
		}
		if prov == "codex" && args[len(args)-1] != "-" {
			t.Fatalf("CA-399: codex termina su argv en '-': %v", args[len(args)-1])
		}
		for _, a := range args {
			if strings.Contains(a, "=== diff ===") || strings.Contains(a, "linea 0001 de un cambio grande") {
				t.Fatalf("CA-399: %s: el argv no lleva el pedido", prov)
			}
		}
		ev := raEvidencia(t, "CA-399", root, "")
		if !strings.Contains(in, "=== diff ===\n"+string(ev.Diff)) || !strings.HasPrefix(in, "Revisa el cambio de esta rama.") {
			t.Fatalf("CA-399: %s: el reviewer recibe por stdin el pedido entero, con el diff entero", prov)
		}
	}
}

// CA-402 (caso limite): un modelo que no aguanta la evidencia aunque este
// bajo el tope: el run falla y la review es NO ENTREGABLE, como cualquier run
// fallido, sin registro. El CLI falso falla solo si el pedido trae mas de
// 2500 bytes: con la evidencia adentro (unos 3 KiB) no la aguanta.
func TestCA402_ModeloQueNoAguantaLaEvidenciaEsNoEntregable(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "var Relleno%02d = \"cuarenta bytes de relleno por linea\"\n", i)
	}
	write(t, root, "app.go", b.String())
	cx := raInstalar(t, bin, "codex",
		"for last; do :; done\n"+
			"if [ \"$last\" = \"-\" ]; then size=$(wc -c < \"$d/stdin.$n\"); else size=${#last}; fi\n"+
			"if [ \"$size\" -gt 2500 ]; then echo 'context window exceeded' >&2; exit 3; fi\n")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if cx.veces() != 1 {
		t.Fatalf("CA-402: bajo el tope la pasada se lanza: codex %d\n%s", cx.veces(), out)
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 || len(res.Passes) != 1 || res.Passes[0].RunStatus == "done" {
		t.Fatalf("CA-402: el run que no aguanta la evidencia falla y la review es NO ENTREGABLE: %+v\n%s", res, out)
	}
	if recs, _ := Records(root); len(recs) != 0 {
		t.Fatalf("CA-402: sin registro de review: %+v", recs)
	}
}
