// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md
// (CA-491, CA-493, CA-513) sobre el sobre: `hoom agent` gana --effort y
// --timeout (Options.Effort y Options.Timeout) y, cuando no se pasan, toma
// provider, modelo, esfuerzo, presupuesto y tiempo limite de agents.<rol> del
// hoom.yaml del PROYECTO (su argumento root), nunca del de la rama de la
// tarea. El provider recibe el esfuerzo. Sin --spec, el sobre no exige spec,
// como hoy. Los CLIs son falsos (fakeProvider) y anotan sus argumentos.
package agentcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// vdtRepo es repo(t) con extra agregado a hoom.yaml y commiteado.
func vdtRepo(t *testing.T, extra string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(repo(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if extra == "" {
		return root
	}
	write(t, root, "hoom.yaml", string(raw)+extra)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "agents en hoom.yaml")
	return root
}

// vdtFalso instala un CLI falso que deja sus argumentos, separados por NUL,
// en un archivo propio de cada invocacion, y devuelve el directorio.
func vdtFalso(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	fakeProvider(t, name, "printf '%s\\0' \"$@\" > '"+dir+"/argv.'$$\nexit 0\n")
	return dir
}

// vdtArgv devuelve los argumentos de cada invocacion anotada en dir.
func vdtArgv(t *testing.T, dir string) [][]string {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var out [][]string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "argv.") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00"))
	}
	return out
}

func vdtSigue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func vdtTiene(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// vdtUnaInvocacion exige una sola invocacion del CLI y la devuelve.
func vdtUnaInvocacion(t *testing.T, ca, que, dir string) []string {
	t.Helper()
	ll := vdtArgv(t, dir)
	if len(ll) != 1 {
		t.Fatalf("%s: %s: el sobre corre el CLI una vez, fueron %d", ca, que, len(ll))
	}
	return ll[0]
}

// vdtTimeoutS es el timeout_s del unico sidecar de runs de root.
func vdtTimeoutS(t *testing.T, ca, root string) int {
	t.Helper()
	metas := runcmd.Metas(root)
	if len(metas) != 1 {
		t.Fatalf("%s: un run, un sidecar en .hoom/runs: %d", ca, len(metas))
	}
	return metas[0].TimeoutS
}

const vdtAgentsWriter = "agents:\n  writer:\n    model: m-yaml\n    effort: e-yaml\n    budget_usd: 3\n    timeout: 45m\n"

// CA-493: sin opciones, el sobre toma modelo, esfuerzo, presupuesto y tiempo
// limite de agents.writer; el provider recibe el esfuerzo.
func TestCA493_AgentUsaLosValoresDeAgentsDelRol(t *testing.T) {
	argv := vdtFalso(t, "claude")
	root := vdtRepo(t, vdtAgentsWriter)
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa el cambio"}, &buf); err != nil {
		t.Fatalf("CA-493: el sobre corre: %v\n%s", err, buf.String())
	}
	args := vdtUnaInvocacion(t, "CA-493", "sin opciones", argv)
	if v, _ := vdtSigue(args, "--model"); v != "m-yaml" {
		t.Fatalf("CA-493: sin --model, el de agents.writer.model (m-yaml): %q", args)
	}
	if v, _ := vdtSigue(args, "--effort"); v != "e-yaml" {
		t.Fatalf("CA-493: sin --effort, el de agents.writer.effort llega al provider (--effort e-yaml): %q", args)
	}
	if v, _ := vdtSigue(args, "--max-budget-usd"); v != "3" {
		t.Fatalf("CA-493: sin --budget-usd, el de agents.writer.budget_usd (3): %q", args)
	}
	if got := vdtTimeoutS(t, "CA-493", root); got != 45*60 {
		t.Fatalf("CA-493: sin --timeout, el de agents.writer.timeout (45m = 2700 s) en el sidecar: %d", got)
	}
}

// CA-493 y CA-491: lo que se pide gana a hoom.yaml: --model, --effort,
// --budget-usd y --timeout.
func TestCA493_LoPedidoGanaAHoomYaml(t *testing.T) {
	argv := vdtFalso(t, "claude")
	root := vdtRepo(t, vdtAgentsWriter)
	var buf bytes.Buffer
	_, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa el cambio", Model: "m-cmd", Effort: "e-cmd",
		BudgetUSD: 1, BudgetSet: true, Timeout: 10 * time.Minute}, &buf)
	if err != nil {
		t.Fatalf("CA-493: el sobre corre: %v\n%s", err, buf.String())
	}
	args := vdtUnaInvocacion(t, "CA-493", "con opciones", argv)
	if v, _ := vdtSigue(args, "--model"); v != "m-cmd" {
		t.Fatalf("CA-491: --model gana a hoom.yaml: %q", args)
	}
	if v, _ := vdtSigue(args, "--effort"); v != "e-cmd" {
		t.Fatalf("CA-493: --effort llega al provider: %q", args)
	}
	if v, _ := vdtSigue(args, "--max-budget-usd"); v != "1" {
		t.Fatalf("CA-491: --budget-usd gana a hoom.yaml: %q", args)
	}
	if got := vdtTimeoutS(t, "CA-493", root); got != 600 {
		t.Fatalf("CA-493: --timeout 10m gana a hoom.yaml en el sidecar (600 s): %d", got)
	}
}

// CA-493: --budget-usd 0 pedido (BudgetSet) es sin tope aunque hoom.yaml
// declare uno.
func TestCA493_BudgetPedidoEnCeroEsSinTope(t *testing.T) {
	argv := vdtFalso(t, "claude")
	root := vdtRepo(t, vdtAgentsWriter)
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa", BudgetSet: true}, &buf); err != nil {
		t.Fatalf("CA-493: el sobre corre: %v", err)
	}
	if args := vdtUnaInvocacion(t, "CA-493", "budget 0 pedido", argv); vdtTiene(args, "--max-budget-usd") {
		t.Fatalf("CA-493: un presupuesto pedido de 0 no toma el de hoom.yaml: %q", args)
	}
}

// CA-493: el provider de agents.<rol> gana al primero instalado; con codex,
// que no acepta tope, el budget_usd de hoom.yaml no se pasa ni es un error.
func TestCA493_ProviderDeHoomYaml(t *testing.T) {
	claude := vdtFalso(t, "claude")
	codex := vdtFalso(t, "codex")
	root := vdtRepo(t, "agents:\n  writer:\n    provider: codex\n    effort: e-yaml\n    budget_usd: 3\n")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
	if err != nil {
		t.Fatalf("CA-493: con codex en hoom.yaml el presupuesto no se pasa y el sobre corre: %v\n%s", err, buf.String())
	}
	if res.Provider != "codex" || len(vdtArgv(t, claude)) != 0 {
		t.Fatalf("CA-493: sin --provider, el de agents.writer.provider (codex): %+v, claude corrio %d veces", res, len(vdtArgv(t, claude)))
	}
	args := vdtUnaInvocacion(t, "CA-493", "codex de hoom.yaml", codex)
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-c" && args[i+1] == `model_reasoning_effort="e-yaml"` {
			found = true
		}
	}
	if !found {
		t.Fatalf(`CA-493: codex recibe el esfuerzo de hoom.yaml (-c model_reasoning_effort="e-yaml"): %q`, args)
	}
}

// CA-493: un --provider pedido gana al de hoom.yaml.
func TestCA493_ProviderPedidoGana(t *testing.T) {
	claude := vdtFalso(t, "claude")
	codex := vdtFalso(t, "codex")
	root := vdtRepo(t, "agents:\n  writer:\n    provider: codex\n")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Provider: "claude", Prompt: "implementa"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != "claude" || len(vdtArgv(t, codex)) != 0 || len(vdtArgv(t, claude)) != 1 {
		t.Fatalf("CA-491: --provider claude gana a agents.writer.provider: %+v", res)
	}
}

// CA-491: los valores salen del hoom.yaml del proyecto (root), aunque la
// rama de la tarea traiga otros: la rama no se sube su propio presupuesto ni
// su tiempo limite.
func TestCA491_AgentTomaElHoomYamlDelProyectoNoElDeLaRama(t *testing.T) {
	argv := vdtFalso(t, "claude")
	root := vdtRepo(t, "agents:\n  writer:\n    model: m-proyecto\n    budget_usd: 1\n    timeout: 45m\n")
	if err := taskcmd.Start(root, "en-rama", "main"); err != nil {
		t.Fatalf("fixture: hoom task start: %v", err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", "en-rama")
	raw, err := os.ReadFile(filepath.Join(wt, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cambiado := strings.Replace(string(raw), "m-proyecto", "m-rama", 1)
	cambiado = strings.Replace(cambiado, "budget_usd: 1", "budget_usd: 50", 1)
	cambiado = strings.Replace(cambiado, "timeout: 45m", "timeout: 12h", 1)
	write(t, wt, "hoom.yaml", cambiado)
	git(t, wt, "add", "-A")
	git(t, wt, "commit", "-q", "-m", "la rama se sube su presupuesto")

	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "writer", Task: "en-rama", Prompt: "implementa"}, &buf); err != nil {
		t.Fatalf("CA-491: el sobre corre en la tarea: %v\n%s", err, buf.String())
	}
	args := vdtUnaInvocacion(t, "CA-491", "en la tarea", argv)
	if v, _ := vdtSigue(args, "--model"); v != "m-proyecto" {
		t.Fatalf("CA-491: el modelo es el del hoom.yaml del proyecto (m-proyecto), no el de la rama: %q", args)
	}
	if v, _ := vdtSigue(args, "--max-budget-usd"); v != "1" {
		t.Fatalf("CA-491: el presupuesto es el del proyecto (1), no el de la rama (50): %q", args)
	}
	if got := vdtTimeoutS(t, "CA-491", root); got != 45*60 {
		t.Fatalf("CA-491: el tiempo limite es el del proyecto (2700 s), no el de la rama: %d", got)
	}
}

// CA-513: sin --spec, `hoom agent` no exige spec, como hoy: corre el rol en
// el arbol, sin spec en el resultado.
func TestCA513_AgentSinSpecNoExigeSpec(t *testing.T) {
	argv := vdtFalso(t, "claude")
	root := vdtRepo(t, "")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa sin spec"}, &buf)
	if err != nil {
		t.Fatalf("CA-513: sin --spec el sobre corre: %v\n%s", err, buf.String())
	}
	if res.Spec != "" || len(vdtArgv(t, argv)) != 1 {
		t.Fatalf("CA-513: sin --spec no hay spec y el rol corre: %+v", res)
	}
}
