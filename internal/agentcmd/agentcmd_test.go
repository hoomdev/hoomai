// Tests adversariales del spec .hoom/specs/hoom-agent-sobre-determinista.md:
// el sobre. Una pasada, cinco pasos, un exit code; la evidencia manda sobre
// la narracion y ningun paso se salta en silencio.
package agentcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// repoGate arma un proyecto real: repo git, hoom.yaml con UN gate y commit.
func repoGate(t *testing.T, cmd string) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \""+cmd+"\"\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	return root
}

func repo(t *testing.T) string { return repoGate(t, "true") }

// fakeProvider pone al frente del PATH un CLI de IA falso.
func fakeProvider(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

// specDemo escribe un spec valido (7 secciones, un CA verificado por comando).
func specDemo(t *testing.T, root string) string {
	t.Helper()
	write(t, root, ".hoom/specs/demo.md", `# Spec demo

## Objetivo
Demostrar el sobre.

## No-goals
Nada mas.

## Contratos
Ninguno.

## Casos limite
Ninguno.

## Criterios de aceptacion
- CA-1: el sobre corre. [verifica: true]

## Decisiones
Ninguna.

## Riesgos
Ninguno.
`)
	return ".hoom/specs/demo.md"
}

func runsCount(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".hoom", "runs"))
	if err != nil {
		return 0
	}
	return len(entries)
}

func filesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// providerMudo implementa Provider pero NO puede imponer un rol de solo
// lectura: no declara la capacidad read_only.
type providerMudo struct{}

func (providerMudo) Name() string { return "mudo" }
func (providerMudo) Bin() string  { return "mudo" }
func (providerMudo) Capabilities() providers.Capabilities {
	return providers.Capabilities{SystemPrompt: true}
}
func (providerMudo) Command(providers.Request) (providers.Invocation, error) {
	return providers.Invocation{}, nil
}
func (providerMudo) Normalize(string) []providers.Event { return nil }

// CA-129: el provider explicito manda; sin el, el primero instalado que pueda
// cargar el contrato; y si ninguno puede, se dice cual es la capacidad que
// falta en vez de degradar en silencio.
func TestCA129_EleccionDeProvider(t *testing.T) {
	fakeProvider(t, "claude", "exit 0\n")
	p, err := pickProvider("claude")
	if err != nil || p.Name() != "claude" {
		t.Fatalf("CA-129: el provider explicito manda: %v, %v", p, err)
	}
	p, err = pickProvider("")
	if err != nil || p.Name() != "claude" {
		t.Fatalf("CA-129: sin --provider se elige el primero instalado con system_prompt: %v, %v", p, err)
	}
	_, err = pickProvider("gemini")
	if err == nil || !strings.Contains(err.Error(), "system_prompt") ||
		!strings.Contains(err.Error(), "hoom providers") {
		t.Fatalf("CA-129: un provider sin system_prompt debe nombrar la capacidad y remitir a hoom providers: %v", err)
	}
	t.Setenv("PATH", t.TempDir()) // ninguna CLI instalada
	_, err = pickProvider("")
	if err == nil || !strings.Contains(err.Error(), "system_prompt") ||
		!strings.Contains(err.Error(), "hoom providers") {
		t.Fatalf("CA-129: sin providers instalados el error debe nombrar la capacidad faltante: %v", err)
	}
}

// CA-130: la StartOptions del sobre lleva Strict, el contrato como system
// prompt y las herramientas del rol; un provider que no puede cargar el
// contrato no deja ni run ni log.
func TestCA130_StartOptionsDelSobre(t *testing.T) {
	claude, err := providers.Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	scout, err := agents.Lookup("scout")
	if err != nil {
		t.Fatal(err)
	}
	so, warn := startOptions(claude, scout, "CONTRATO DEL SCOUT", Options{
		Prompt: "explora", Model: "sonnet", MaxTurns: 3, BudgetUSD: 1, ResumeID: "s1", Task: "t1",
	})
	if !so.Strict {
		t.Fatal("CA-130: el sobre invoca SIEMPRE con Strict")
	}
	if so.SystemPrompt != "CONTRATO DEL SCOUT" {
		t.Fatalf("CA-130: el contrato es el system prompt: %q", so.SystemPrompt)
	}
	// El limite del rol viaja como INTENCION desde la Spec C: el vocabulario
	// concreto (herramientas en Claude, sandbox en Codex) es del adapter.
	if !so.ReadOnly || so.Exec || warn {
		t.Fatalf("CA-130: un rol de solo lectura debe llevar su limite acotado: %+v", so)
	}
	if so.Role != "scout" {
		t.Fatalf("CA-130: el run recuerda que rol encarna: %q", so.Role)
	}
	if so.Model != "sonnet" || so.MaxTurns != 3 || so.BudgetUSD != 1 || so.ResumeID != "s1" || so.Task != "t1" {
		t.Fatalf("CA-130: las opciones del sobre pasan tal cual: %+v", so)
	}
	writer, _ := agents.Lookup("writer")
	sow, _ := startOptions(claude, writer, "C", Options{Prompt: "implementa"})
	if sow.ReadOnly || sow.Exec || !sow.Strict {
		t.Fatalf("CA-130: un rol de escritura no se limita: %+v", sow)
	}

	root := repo(t)
	fakeProvider(t, "gemini", "exit 0\n")
	_, err = Run(root, "main", Options{Role: "scout", Provider: "gemini", Prompt: "hola"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "system_prompt") {
		t.Fatalf("CA-130: sin system_prompt el sobre se niega: %v", err)
	}
	if n := runsCount(t, root); n != 0 {
		t.Fatalf("CA-130: una negativa no deja run ni log: %d archivos en .hoom/runs", n)
	}
}

// CA-131: el limite del rol lo impone el provider con lo que DECLARA. La
// Spec C reemplazo el type assertion sobre ToolNamer por la capacidad
// read_only: el sobre pide la intencion y cada adapter la traduce (Claude a
// nombres de herramientas, Codex a un sandbox). El que no puede imponerla
// produce aviso y el run igual arranca: el gate de scope es la red.
func TestCA131_LimiteDelRol(t *testing.T) {
	claude, _ := providers.Lookup("claude")
	codex, _ := providers.Lookup("codex")
	scout, _ := agents.Lookup("scout")       // solo lectura, no ejecuta
	reviewer, _ := agents.Lookup("reviewer") // solo lectura, ejecuta
	writer, _ := agents.Lookup("writer")

	if ro, exec, warn := ReadOnlyFor(claude, scout); !ro || exec || warn {
		t.Fatalf("CA-131: el scout es solo lectura sin exec: ro=%v exec=%v warn=%v", ro, exec, warn)
	}
	if ro, exec, warn := ReadOnlyFor(codex, reviewer); !ro || !exec || warn {
		t.Fatalf("CA-131: el reviewer es solo lectura CON exec, y codex lo declara: ro=%v exec=%v warn=%v", ro, exec, warn)
	}
	if ro, exec, warn := ReadOnlyFor(claude, writer); ro || exec || warn {
		t.Fatalf("CA-131: un rol de escritura no se limita: ro=%v exec=%v warn=%v", ro, exec, warn)
	}
	if ro, exec, warn := ReadOnlyFor(providerMudo{}, scout); ro || exec || !warn {
		t.Fatalf("CA-131: un provider que no puede imponerlo avisa y no finge: ro=%v exec=%v warn=%v", ro, exec, warn)
	}

	// y el vocabulario concreto sigue siendo el de siempre, adentro del adapter
	inv, err := claude.Command(providers.Request{Prompt: "hola", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(inv.Args, " ")
	if !strings.Contains(argv, "--allowedTools Read,Grep,Glob ") {
		t.Fatalf("CA-131: claude sigue acotando por nombre: %v", inv.Args)
	}
	for _, want := range []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"} {
		if !strings.Contains(argv, want) {
			t.Fatalf("CA-131: el scout no ejecuta: %s deberia estar prohibido (%v)", want, inv.Args)
		}
	}
	inv, _ = codex.Command(providers.Request{Prompt: "hola", ReadOnly: true, Exec: true})
	if !strings.Contains(strings.Join(inv.Args, " "), `sandbox_mode="workspace-write"`) {
		t.Fatalf("CA-131: codex impone el limite con su sandbox: %v", inv.Args)
	}
}

// CA-132: sin aprobacion vigente, el sobre no gasta un token.
func TestCA132_GatePrevioDeAprobacion(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	spec := specDemo(t, root)

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Spec: spec, Prompt: "implementa"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "spec" || res.ExitCode != 1 || res.Status != "no-entregable" {
		t.Fatalf("CA-132: un spec sin aprobar corta antes del run: %+v", res)
	}
	if res.RunID != "" || runsCount(t, root) != 0 {
		t.Fatalf("CA-132: no debe existir run: %+v", res)
	}
	if res.Approval != approval.StatusNotApproved || !strings.Contains(buf.String(), "hoom spec approve") {
		t.Fatalf("CA-132: la salida debe traer la accion exacta:\n%s", buf.String())
	}

	// un rol de solo lectura no exige aprobacion: el arquitecto escribe el
	// spec que todavia no esta aprobado
	res, err = Run(root, "main", Options{Role: "scout", Spec: spec, Prompt: "explora"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.RunID == "" || res.Stage == "spec" {
		t.Fatalf("CA-132: un rol de solo lectura pasa el gate del spec: %+v", res)
	}

	// aprobado vigente: sigue de largo
	if _, _, err := approval.Approve(root, filepath.Join(root, spec)); err != nil {
		t.Fatal(err)
	}
	res, err = Run(root, "main", Options{Role: "writer", Spec: spec, Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.Approval != approval.StatusApproved || res.Stage == "spec" || res.RunID == "" {
		t.Fatalf("CA-132: con aprobacion vigente el sobre sigue: %+v", res)
	}
}

// CA-140: cada violacion se vuelve un artefacto append-only; si el artefacto
// no se puede escribir, la violacion se reporta igual.
func TestCA140_ViolacionEsHallazgo(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "printf 'package nuevo\\n' > nuevo.go\nexit 0\n")

	res, err := Run(root, "main", Options{Role: "scout", Prompt: "explora"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Scope.Violations) != 1 || res.Scope.Violations[0].Path != "nuevo.go" {
		t.Fatalf("CA-140: un scout que escribe codigo esta fuera de scope: %+v", res.Scope)
	}
	id := res.Scope.Violations[0].FindingID
	if id == "" {
		t.Fatalf("CA-140: la violacion debe traer el id de su hallazgo: %+v", res.Scope.Violations[0])
	}
	items, _, err := finding.List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range items {
		if it.Finding.ID != id {
			continue
		}
		found = true
		if it.Finding.Severity != "high" || it.Finding.Lens != "risk" || it.Finding.File != "nuevo.go" {
			t.Fatalf("CA-140: el hallazgo debe ser high/risk con la ruta: %+v", it.Finding)
		}
		if !strings.Contains(it.Finding.Description, "scout") {
			t.Fatalf("CA-140: el hallazgo debe nombrar el rol: %q", it.Finding.Description)
		}
	}
	if !found {
		t.Fatalf("CA-140: el hallazgo %s no quedo en .hoom/findings/", id)
	}

	// el artefacto no se puede escribir: la violacion no desaparece
	root2 := repo(t)
	write(t, root2, ".hoom/findings", "no soy un directorio\n")
	res, err = Run(root2, "main", Options{Role: "scout", Prompt: "explora"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Scope.Violations) != 1 || res.Scope.Violations[0].FindingID != "" {
		t.Fatalf("CA-140: sin poder registrar el hallazgo, la violacion sigue en pie: %+v", res.Scope)
	}
	if res.ExitCode != 1 {
		t.Fatalf("CA-140: el exit no depende del artefacto: %+v", res)
	}
}

// CA-141: la manipulacion corta antes de verify; escribir fuera de scope, no.
func TestCA141_CorteDelSobre(t *testing.T) {
	root := repo(t)
	write(t, root, ".hoom/verdicts/2026-01-01T00-00-00Z_viejo.json", "{\"verdict\":\"green\"}\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "veredicto previo")
	fakeProvider(t, "claude", "printf 'x' >> .hoom/verdicts/2026-01-01T00-00-00Z_viejo.json\nexit 0\n")

	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.Tampering || res.Stage != "scope" || res.ExitCode != 1 {
		t.Fatalf("CA-141: reescribir evidencia corta en scope: %+v", res)
	}
	if res.VerdictID != "" || res.Check != nil {
		t.Fatalf("CA-141: no se emite veredicto sobre un arbol manipulado: %+v", res)
	}
	if n := len(filesIn(t, filepath.Join(root, ".hoom", "verdicts"))); n != 1 {
		t.Fatalf("CA-141: verify no debe haber corrido: %d veredictos", n)
	}

	// fuera de scope SIN manipulacion: el cuadro completo igual se arma
	root2 := repo(t)
	fakeProvider(t, "claude", "printf 'package nuevo\\n' > nuevo.go\nexit 0\n")
	res, err = Run(root2, "main", Options{Role: "scout", Prompt: "explora"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scope.Tampering || res.Scope.OK {
		t.Fatalf("CA-141: escribir fuera de scope no es manipulacion: %+v", res.Scope)
	}
	if res.VerdictID == "" || res.Check == nil {
		t.Fatalf("CA-141: verify y check igual corren: %+v", res)
	}
	if res.Stage != "scope" || res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-141: el cierre sigue siendo rojo, y el paso culpable es scope: %+v", res)
	}
}

// CA-142: el camino verde completo, con el spec atado al veredicto.
func TestCA142_CaminoVerdeCompleto(t *testing.T) {
	root := repo(t)
	spec := specDemo(t, root)
	if _, _, err := approval.Approve(root, filepath.Join(root, spec)); err != nil {
		t.Fatal(err)
	}
	fakeProvider(t, "claude", "printf 'package app // implementado\\n' > app.go\nexit 0\n")

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Spec: spec, Prompt: "implementa"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "ok" || res.Status != "entregable" || res.ExitCode != 0 {
		t.Fatalf("CA-142: los cinco pasos verdes cierran entregable: %+v\n%s", res, buf.String())
	}
	if res.VerdictID == "" || res.Verdict != "green" || res.Check == nil || !res.Check.OK {
		t.Fatalf("CA-142: veredicto y check deben venir verdes: %+v", res)
	}
	if !res.Scope.OK || len(res.Scope.Touched) == 0 {
		t.Fatalf("CA-142: el writer toco app.go dentro de su scope: %+v", res.Scope)
	}
	all, err := verdict.LoadAll(root)
	if err != nil || len(all) == 0 {
		t.Fatalf("CA-142: falta el artefacto del veredicto: %v", err)
	}
	last := all[len(all)-1]
	if !strings.Contains(last.Spec, "demo.md") {
		t.Fatalf("CA-142: verify debe haberse invocado con el --spec recibido: %q", last.Spec)
	}
	var conSpecGates bool
	for _, g := range last.Gates {
		if g.Name == "spec_approved" && g.Status == "pass" {
			conSpecGates = true
		}
	}
	if !conSpecGates {
		t.Fatalf("CA-142: el veredicto debe traer los gates del spec: %+v", last.Gates)
	}
	if !strings.Contains(buf.String(), "ENTREGABLE") {
		t.Fatalf("CA-142: la salida humana debe cerrar el cuadro:\n%s", buf.String())
	}
}

// CA-143: el exit es el del primer paso que falla.
func TestCA143_ExitCodes(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "echo fallando\nexit 3\n")
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || res.Stage != "run" {
		t.Fatalf("CA-143: el exit del provider se propaga: %+v", res)
	}
	if res.VerdictID != "" || len(res.Scope.Touched) != 0 {
		t.Fatalf("CA-143: un run fallido no deja arbol que medir: %+v", res)
	}

	// veredicto rojo: exit 1
	rojo := repoGate(t, "false")
	fakeProvider(t, "claude", "exit 0\n")
	res, err = Run(rojo, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "red" || res.Stage != "verify" || res.ExitCode != 1 {
		t.Fatalf("CA-143: un veredicto rojo cierra en 1 nombrando el paso: %+v", res)
	}

	// todo verde: exit 0
	verde := repo(t)
	res, err = Run(verde, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || res.Stage != "ok" {
		t.Fatalf("CA-143: los cinco pasos verdes salen 0: %+v", res)
	}
}

// CA-144: el JSON es el MISMO dato que el texto, con el mismo exit.
func TestCA144_SalidaJSON(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "printf 'package nuevo\\n' > nuevo.go\nexit 0\n")
	res, err := Run(root, "main", Options{Role: "scout", Prompt: "explora"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"role", "provider", "dir", "run_id", "scope", "verdict_id",
		"verdict", "check", "stage", "status", "exit_code"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("CA-144: falta %q en el JSON del sobre: %s", k, raw)
		}
	}
	sc, ok := got["scope"].(map[string]any)
	if !ok {
		t.Fatalf("CA-144: scope debe ser un objeto: %s", raw)
	}
	for _, k := range []string{"touched", "violations", "tampering", "ok"} {
		if _, ok := sc[k]; !ok {
			t.Fatalf("CA-144: falta scope.%s: %s", k, raw)
		}
	}
	if int(got["exit_code"].(float64)) != res.ExitCode || res.ExitCode == 0 {
		t.Fatalf("CA-144: el JSON respeta el mismo exit que el modo texto: %s", raw)
	}
}
