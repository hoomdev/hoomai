// Tests adversariales del spec .hoom/specs/gate-findings-open.md
// (CA-249, CA-253, CA-256, CA-258, CA-259, CA-261) contra el BINARIO: que
// hoom.yaml opte por el gate, que verify, status y finding lo lean, y el ciclo
// completo de un hallazgo high de la tarea del spec (rojo, resolver, verde)
// con exit codes reales.
package verifycmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// foCorrer ejecuta el binario sin HOOM_TASK: la tarea de un hallazgo la dice
// el test con --task, no el entorno de quien corre la suite.
func foCorrer(t *testing.T, bin, dir string, args ...string) salidaCLI {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOOM_TASK=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	return salidaCLI{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

// foRepoCLI crea un repo git con hoom.yaml (un gate barato + el bloque
// findings dado) y un commit inicial.
func foRepoCLI(t *testing.T, findingsYAML string) string {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" + findingsYAML
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@hoom.dev"},
		{"config", "user.name", "hoom test"},
		{"add", "-A"},
		{"commit", "-m", "inicial"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func foSpecMinimo(t *testing.T, dir, slug string) string {
	t.Helper()
	rel := filepath.Join(".hoom", "specs", slug+".md")
	if err := os.MkdirAll(filepath.Join(dir, ".hoom", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Spec: " + slug + "\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n- CA-1: el ciclo del hallazgo cierra. [verifica: true]\n\n## Decisiones\nx\n\n## Riesgos\nx\n"
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return rel
}

var foIDRe = regexp.MustCompile(`registrado (\S+) \(`)

func foAltaCLI(t *testing.T, bin, dir string, args ...string) string {
	t.Helper()
	res := foCorrer(t, bin, dir, append([]string{"finding", "add"}, args...)...)
	if res.exit != 0 {
		t.Fatalf("hoom finding add %v salio %d\nstdout: %s\nstderr: %s", args, res.exit, res.stdout, res.stderr)
	}
	m := foIDRe.FindStringSubmatch(res.stdout)
	if m == nil {
		t.Fatalf("no encontre el id del hallazgo en la salida de finding add:\n%s", res.stdout)
	}
	return m[1]
}

func foUltimo(t *testing.T, dir string) *verdict.Verdict {
	t.Helper()
	all, err := verdict.LoadAll(dir)
	if err != nil || len(all) == 0 {
		t.Fatalf("no hay veredictos en %s: %v", dir, err)
	}
	return all[len(all)-1]
}

func foGate(v *verdict.Verdict) *verdict.GateResult {
	for i := range v.Gates {
		if v.Gates[i].Name == "findings_open" {
			return &v.Gates[i]
		}
	}
	return nil
}

// CA-261: E2E con el binario real. block_on high y un hallazgo high de la
// tarea del spec: 'hoom verify --spec' sale 1 y ROJO; tras 'hoom finding
// resolve <id> --as refutado --evidence', el mismo verify sale 0 y VERDE.
func TestCA261_E2ERojoResolverVerde(t *testing.T) {
	bin := construirHoom(t)
	dir := foRepoCLI(t, "findings:\n  block_on: high\n")
	spec := foSpecMinimo(t, dir, "tarea-e2e")
	if res := foCorrer(t, bin, dir, "spec", "approve", spec); res.exit != 0 {
		t.Fatalf("CA-261: hoom spec approve salio %d\n%s%s", res.exit, res.stdout, res.stderr)
	}
	id := foAltaCLI(t, bin, dir, "--sev", "high", "--lens", "risk", "--task", "tarea-e2e",
		"el gate no mira la tarea del spec")

	rojo := foCorrer(t, bin, dir, "verify", "--spec", spec)
	if rojo.exit != 1 {
		t.Fatalf("CA-261: con un high abierto de la tarea, verify --spec sale 1, salio %d\nstdout: %s\nstderr: %s",
			rojo.exit, rojo.stdout, rojo.stderr)
	}
	v := foUltimo(t, dir)
	g := foGate(v)
	if v.Verdict != "red" || g == nil || g.Status != verdict.StatusFail || g.Scope != "spec" {
		t.Fatalf("CA-261: veredicto ROJO por findings_open FAIL (scope spec): %s %+v", v.Verdict, g)
	}
	if !strings.Contains(g.Notes, id) || !strings.Contains(g.OutputTail, "hoom finding resolve "+id) {
		t.Fatalf("CA-261: el gate nombra el hallazgo y la accion: notes=%q tail=%q", g.Notes, g.OutputTail)
	}
	for _, otro := range v.Gates {
		if otro.Name != "findings_open" && otro.Status != verdict.StatusPass {
			t.Fatalf("CA-261: el rojo lo pone SOLO findings_open; %s quedo %s (%s %s)", otro.Name, otro.Status, otro.Notes, otro.OutputTail)
		}
	}

	if res := foCorrer(t, bin, dir, "finding", "resolve", id, "--as", "refutado",
		"--evidence", "TestTareaE2E demuestra que el gate si mira la tarea"); res.exit != 0 {
		t.Fatalf("CA-261: resolve salio %d\n%s%s", res.exit, res.stdout, res.stderr)
	}

	verde := foCorrer(t, bin, dir, "verify", "--spec", spec)
	if verde.exit != 0 {
		t.Fatalf("CA-261: tras refutar, verify --spec sale 0, salio %d\nstdout: %s\nstderr: %s",
			verde.exit, verde.stdout, verde.stderr)
	}
	v = foUltimo(t, dir)
	g = foGate(v)
	if v.Verdict != "green" || g == nil || g.Status != verdict.StatusPass || !strings.HasPrefix(g.Notes, "0 abiertos") {
		t.Fatalf("CA-261: veredicto VERDE con findings_open PASS '0 abiertos': %s %+v", v.Verdict, g)
	}

	// --json: el mismo veredicto parseable por un agente, con el gate adentro
	js := foCorrer(t, bin, dir, "verify", "--spec", spec, "--json")
	if js.exit != 0 {
		t.Fatalf("CA-261: verify --json sale 0 en verde, salio %d\n%s", js.exit, js.stderr)
	}
	var parsed verdict.Verdict
	if err := json.Unmarshal([]byte(js.stdout), &parsed); err != nil {
		t.Fatalf("CA-261: stdout de --json es el veredicto: %v\n%s", err, js.stdout)
	}
	if foGate(&parsed) == nil {
		t.Fatalf("CA-261: el veredicto JSON trae findings_open: %+v", parsed.Gates)
	}
}

// CA-259: el binario lee findings.block_on para 'hoom status': --json expone
// block_on y blocking, y el texto muestra la linea del gate. Sin block_on,
// block_on va vacio y el texto no menciona el gate.
func TestCA259_E2EStatusLeeElUmbral(t *testing.T) {
	bin := construirHoom(t)
	dir := foRepoCLI(t, "findings:\n  block_on: high\n")
	foAltaCLI(t, bin, dir, "--sev", "high", "--lens", "risk", "un high sin tarea")
	foAltaCLI(t, bin, dir, "--sev", "medium", "--lens", "risk", "un medium")

	js := foCorrer(t, bin, dir, "status", "--json")
	if js.exit != 0 {
		t.Fatalf("CA-259: status --json salio %d\n%s", js.exit, js.stderr)
	}
	var snap struct {
		Findings map[string]any `json:"findings"`
	}
	if err := json.Unmarshal([]byte(js.stdout), &snap); err != nil {
		t.Fatalf("CA-259: status --json es JSON: %v\n%s", err, js.stdout)
	}
	if snap.Findings["block_on"] != "high" || snap.Findings["blocking"] != float64(1) {
		t.Fatalf("CA-259: findings.block_on=high y blocking=1 esperados: %v", snap.Findings)
	}
	if snap.Findings["open"] != float64(2) || snap.Findings["open_high"] != float64(1) {
		t.Fatalf("CA-259: open/open_high con la definicion unica: %v", snap.Findings)
	}
	txt := foCorrer(t, bin, dir, "status")
	if txt.exit != 0 || !strings.Contains(txt.stdout, "gate findings_open: 1 bloquean (block_on: high)") {
		t.Fatalf("CA-259: el texto muestra 'gate findings_open: 1 bloquean (block_on: high)' (exit %d):\n%s", txt.exit, txt.stdout)
	}

	apagado := foRepoCLI(t, "")
	foAltaCLI(t, bin, apagado, "--sev", "high", "--lens", "risk", "un high con el gate apagado")
	js = foCorrer(t, bin, apagado, "status", "--json")
	snap.Findings = nil
	if err := json.Unmarshal([]byte(js.stdout), &snap); err != nil {
		t.Fatalf("CA-259: status --json es JSON: %v\n%s", err, js.stdout)
	}
	if v, ok := snap.Findings["block_on"]; !ok || v != "" {
		t.Fatalf("CA-259: sin block_on la clave block_on va vacia (no ausente): %v", snap.Findings)
	}
	txt = foCorrer(t, bin, apagado, "status")
	if strings.Contains(txt.stdout, "findings_open") {
		t.Fatalf("CA-259: con el gate apagado el texto no menciona findings_open:\n%s", txt.stdout)
	}
}

// CA-249: un hoom.yaml con block_on invalido o una clave desconocida en
// findings no deja correr ningun verbo: verify no escribe veredicto ni narra,
// y el error cita el valor (o la clave) y los valores validos.
func TestCA249_E2EManifiestoInvalidoNoVerifica(t *testing.T) {
	bin := construirHoom(t)
	for _, c := range []struct {
		yml    string
		nombra []string
	}{
		{"findings:\n  block_on: critical\n", []string{`"critical"`, "low|medium|high"}},
		{"findings:\n  block_on: \"\"\n", []string{`""`, "low|medium|high"}},
		{"findings:\n  blockon: high\n", []string{"blockon"}},
	} {
		dir := foRepoCLI(t, c.yml)
		for _, args := range [][]string{{"verify"}, {"verify", "--full"}, {"status", "--json"}} {
			res := foCorrer(t, bin, dir, args...)
			if res.exit == 0 {
				t.Fatalf("CA-249 %v con %q: un hoom.yaml invalido no deja correr el verbo (exit 0)\n%s", args, c.yml, res.stdout)
			}
			for _, s := range c.nombra {
				if !strings.Contains(res.stderr, s) {
					t.Fatalf("CA-249 %v con %q: el error debe nombrar %s:\n%s", args, c.yml, s, res.stderr)
				}
			}
		}
		if n := cuentaVeredictos(t, dir); n != 0 {
			t.Fatalf("CA-249 con %q: verify no escribe veredicto, dejo %d", c.yml, n)
		}
		if _, err := os.Stat(filepath.Join(dir, ".hoom", "cache", live.FileName)); !os.IsNotExist(err) {
			t.Fatalf("CA-249 con %q: no se abre la narracion viva (err=%v)", c.yml, err)
		}
	}
}

// CA-256: 'hoom verify --gate findings_open' sale 2 aunque hoom.yaml declare
// un gate con ese nombre, sin veredicto; y '--gate test' con block_on no trae
// findings_open en su veredicto PARCIAL.
func TestCA256_E2EGateFindingsOpenEsUso(t *testing.T) {
	bin := construirHoom(t)
	dir := foRepoCLI(t, "findings:\n  block_on: high\n")
	yml, err := os.ReadFile(filepath.Join(dir, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	conGate := strings.Replace(string(yml), "gates:\n", "gates:\n  findings_open:\n    required: true\n    cmd: \"true\"\n", 1)
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(conGate), 0o644); err != nil {
		t.Fatal(err)
	}
	foAltaCLI(t, bin, dir, "--sev", "high", "--lens", "risk", "un high abierto")

	res := foCorrer(t, bin, dir, "verify", "--gate", "findings_open")
	if res.exit != 2 {
		t.Fatalf("CA-256: --gate findings_open es error de uso (exit 2), salio %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	if n := cuentaVeredictos(t, dir); n != 0 {
		t.Fatalf("CA-256: el rechazo no deja veredicto, dejo %d", n)
	}

	// Otro repo sin el gate homonimo: el declarado apareceria como 'skipped'
	// en el parcial y confundiria el aserto sobre el sintetico.
	limpio := foRepoCLI(t, "findings:\n  block_on: high\n")
	foAltaCLI(t, bin, limpio, "--sev", "high", "--lens", "risk", "un high abierto")
	res = foCorrer(t, bin, limpio, "verify", "--gate", "test")
	if res.exit != 0 {
		t.Fatalf("CA-256: --gate test sin findings_open sale 0 aunque haya un high abierto, salio %d\n%s%s", res.exit, res.stdout, res.stderr)
	}
	v := foUltimo(t, limpio)
	if !v.Partial || foGate(v) != nil {
		t.Fatalf("CA-256: el veredicto PARCIAL no trae findings_open: partial=%v %+v", v.Partial, v.Gates)
	}
}

// CA-258: 'hoom finding resolve' sobre un hallazgo con un .res.json invalido
// se niega (exit != 0) diciendo que sigue ABIERTO y nombrando el archivo, que
// queda byte a byte igual. Y 'hoom finding list --open' lo sigue listando
// (definicion unica, CA-253) con el aviso por stderr.
func TestCA258_E2EResolveNoPisaLaResolucionInvalida(t *testing.T) {
	bin := construirHoom(t)
	dir := foRepoCLI(t, "findings:\n  block_on: high\n")
	id := foAltaCLI(t, bin, dir, "--sev", "high", "--lens", "risk", "un high mal cerrado a mano")
	ruta := filepath.Join(dir, ".hoom", "findings", id+".res.json")
	invalida := []byte(`{"finding_id": "` + id + `", "as": "wontfix", "evidence": "no lo vamos a arreglar"}`)
	if err := os.WriteFile(ruta, invalida, 0o644); err != nil {
		t.Fatal(err)
	}

	lista := foCorrer(t, bin, dir, "finding", "list", "--open")
	if lista.exit != 0 || !strings.Contains(lista.stdout, id) || !strings.Contains(lista.stdout, "ABIERTO") {
		t.Fatalf("CA-253: 'finding list --open' lista el mal cerrado como ABIERTO (exit %d):\n%s", lista.exit, lista.stdout)
	}
	if !strings.Contains(lista.stderr, "resolucion invalida") || !strings.Contains(lista.stderr, id+".res.json") {
		t.Fatalf("CA-253: el aviso de la resolucion invalida sale por stderr:\n%s", lista.stderr)
	}

	res := foCorrer(t, bin, dir, "finding", "resolve", id, "--as", "refutado", "--evidence", "evidencia legitima")
	if res.exit == 0 {
		t.Fatalf("CA-258: resolve sobre una resolucion invalida se niega (exit 0)\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "ABIERTO") || !strings.Contains(res.stderr, id+".res.json") {
		t.Fatalf("CA-258: el error dice que sigue ABIERTO y nombra el archivo:\n%s", res.stderr)
	}
	despues, err := os.ReadFile(ruta)
	if err != nil || !bytes.Equal(despues, invalida) {
		t.Fatalf("CA-258: el .res.json queda byte a byte igual (err=%v)\n  antes: %q\n  ahora: %q", err, invalida, despues)
	}

	// y verify lo ve abierto: ROJO por findings_open
	if v := foCorrer(t, bin, dir, "verify"); v.exit != 1 {
		t.Fatalf("CA-253: con el high mal cerrado verify sale 1, salio %d\n%s%s", v.exit, v.stdout, v.stderr)
	}
}
