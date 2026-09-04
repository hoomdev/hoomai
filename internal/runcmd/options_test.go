// Tests adversariales del spec .hoom/specs/providers-v2-interfaz-y-claude.md
// (CA-118, CA-119, CA-120, CA-121, CA-122): Manager.Start traduce StartOptions
// a la invocacion del provider, captura la sesion, reanuda re-aplicando las
// opciones originales, avisa e ignora (o niega con Strict), y ResolveSystemPrompt.
package runcmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// installFakeNamed pone al frente del PATH un binario falso con OTRO nombre
// (installFake ya cubre `claude`). Sirve para providers como `gemini`.
func installFakeNamed(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

// waitRun espera hasta que el run deja de estar en curso y devuelve su estado
// final (sirve tanto para el primer lanzamiento como para el de Input).
func waitRun(t *testing.T, m *Manager, id string) Run {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, _, err := m.Events(id, 0)
		if err != nil {
			t.Fatalf("Events(%s): %v", id, err)
		}
		if r.Status != StatusRunning {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("el run nunca termino")
	return Run{}
}

// CA-118: Start traduce las opciones a la invocacion: con un fake que imprime
// argv aparecen --model, --append-system-prompt, --allowedTools, --max-turns y
// --max-budget-usd con los valores dados.
func TestCA118_StartTraduceOpciones(t *testing.T) {
	installFake(t, "echo \"args: $@\"\nexit 0\n")
	m := NewManager(t.TempDir())

	run, err := m.Start(StartOptions{
		Provider:     "claude",
		Prompt:       "hola",
		Model:        "sonnet",
		SystemPrompt: "sos un rol",
		AllowTools:   []string{"Read", "Bash"},
		MaxTurns:     3,
		BudgetUSD:    0.05,
	})
	if err != nil {
		t.Fatalf("CA-118: Start no debe fallar: %v", err)
	}
	waitRun(t, m, run.ID)

	_, evs, err := m.Events(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := allDetails(evs)
	for _, want := range []string{"--model sonnet", "--append-system-prompt", "sos un rol", "--allowedTools Read,Bash", "--max-turns 3", "--max-budget-usd 0.05"} {
		if !strings.Contains(out, want) {
			t.Fatalf("CA-118: falta %q en la invocacion:\n%s", want, out)
		}
	}
}

// CA-119: sesion capturada. Un fake que emite system/init con session_id deja
// Run.ProviderSessionID con ese id (tambien en su JSON); Input invoca con
// --resume <id> y sin --continue, re-aplicando las opciones originales (el
// mismo --append-system-prompt).
func TestCA119_SesionCapturadaYResume(t *testing.T) {
	installFake(t, `echo "args: $@"
echo '{"type":"system","subtype":"init","session_id":"sess-123"}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"listo","session_id":"sess-123"}'
exit 0
`)
	m := NewManager(t.TempDir())

	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "primero", SystemPrompt: "sos un rol"})
	if err != nil {
		t.Fatalf("CA-119: Start fallo: %v", err)
	}
	final := waitRun(t, m, run.ID)
	if final.ProviderSessionID != "sess-123" {
		t.Fatalf("CA-119: ProviderSessionID debe capturar el id del stream, es %q", final.ProviderSessionID)
	}
	raw, err := json.Marshal(final)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"provider_session_id":"sess-123"`) {
		t.Fatalf("CA-119: el JSON del run debe exponer provider_session_id: %s", raw)
	}

	if _, err := m.Input(run.ID, "segundo prompt"); err != nil {
		t.Fatalf("CA-119: Input fallo: %v", err)
	}
	waitRun(t, m, run.ID)

	_, evs, err := m.Events(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := allDetails(evs)
	if !strings.Contains(out, "--resume sess-123") {
		t.Fatalf("CA-119: Input debe reanudar con --resume <id>:\n%s", out)
	}
	if strings.Contains(out, "--continue") {
		t.Fatalf("CA-119: con sesion capturada NO debe caer a --continue:\n%s", out)
	}
	if !strings.Contains(out, "--append-system-prompt") || !strings.Contains(out, "sos un rol") {
		t.Fatalf("CA-119: Input debe re-aplicar el system prompt original:\n%s", out)
	}
}

// CA-120: sin sesion capturada, Input cae a --continue (la continuacion actual
// del Studio, intacta) y no inventa ids.
func TestCA120_ContinuacionSinSesion(t *testing.T) {
	installFake(t, "echo \"args: $@\"\nexit 0\n") // no emite session_id
	m := NewManager(t.TempDir())

	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "uno"})
	if err != nil {
		t.Fatalf("CA-120: Start fallo: %v", err)
	}
	final := waitRun(t, m, run.ID)
	if final.ProviderSessionID != "" {
		t.Fatalf("CA-120: sin session_id en el stream, ProviderSessionID debe quedar vacio, es %q", final.ProviderSessionID)
	}

	if _, err := m.Input(run.ID, "dos"); err != nil {
		t.Fatalf("CA-120: Input fallo: %v", err)
	}
	waitRun(t, m, run.ID)

	_, evs, _ := m.Events(run.ID, 0)
	out := allDetails(evs)
	if !strings.Contains(out, "--continue") {
		t.Fatalf("CA-120: sin sesion capturada, Input cae a --continue:\n%s", out)
	}
	if strings.Contains(out, "--resume") {
		t.Fatalf("CA-120: sin id capturado el log no inventa --resume:\n%s", out)
	}
}

// CA-120: un provider sin continue ni resume (gemini) invoca de nuevo con el
// aviso actual en el log.
func TestCA120_ProviderSinContinuacion(t *testing.T) {
	installFakeNamed(t, "gemini", "echo \"args: $@\"\nexit 0\n")
	m := NewManager(t.TempDir())

	run, err := m.Start(StartOptions{Provider: "gemini", Prompt: "uno"})
	if err != nil {
		t.Fatalf("CA-120: Start gemini fallo: %v", err)
	}
	waitRun(t, m, run.ID)

	if _, err := m.Input(run.ID, "dos"); err != nil {
		t.Fatalf("CA-120: Input gemini fallo: %v", err)
	}
	waitRun(t, m, run.ID)

	_, evs, _ := m.Events(run.ID, 0)
	out := allDetails(evs)
	if !strings.Contains(out, "dos") {
		t.Fatalf("CA-120: un provider sin continuacion re-invoca con el nuevo prompt:\n%s", out)
	}
	if strings.Contains(out, "--continue") || strings.Contains(out, "--resume") {
		t.Fatalf("CA-120: gemini no soporta continue ni resume:\n%s", out)
	}
	// El aviso actual para providers sin continuacion se conserva (texto exacto
	// no fijado por la spec; se exige que exista un aviso en el log).
	low := strings.ToLower(out)
	if !strings.Contains(low, "aviso") && !strings.Contains(low, "no soporta") && !strings.Contains(low, "continu") {
		t.Fatalf("CA-120: debe conservarse el aviso para providers sin continuacion:\n%s", out)
	}
}

// CA-120: StartOptions.ResumeID en un run nuevo produce --resume <id> en la
// primera invocacion.
func TestCA120_ResumeIDEnRunNuevo(t *testing.T) {
	installFake(t, "echo \"args: $@\"\nexit 0\n")
	m := NewManager(t.TempDir())

	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "uno", ResumeID: "sess-abc"})
	if err != nil {
		t.Fatalf("CA-120: Start con ResumeID fallo: %v", err)
	}
	waitRun(t, m, run.ID)

	_, evs, _ := m.Events(run.ID, 0)
	out := allDetails(evs)
	if !strings.Contains(out, "--resume sess-abc") {
		t.Fatalf("CA-120: StartOptions.ResumeID debe dar --resume <id> en la primera invocacion:\n%s", out)
	}
}

// CA-121: cada campo ignorado produce un evento text
// `aviso: <provider> no soporta <campo>; se ignora` en el log del run.
func TestCA121_CamposIgnoradosEnLog(t *testing.T) {
	installFakeNamed(t, "gemini", "echo hola\nexit 0\n")
	m := NewManager(t.TempDir())

	run, err := m.Start(StartOptions{Provider: "gemini", Prompt: "hola", Model: "x", MaxTurns: 2})
	if err != nil {
		t.Fatalf("CA-121: Start fallo: %v", err)
	}
	waitRun(t, m, run.ID)

	_, evs, _ := m.Events(run.ID, 0)
	out := allDetails(evs)
	for _, want := range []string{"aviso: gemini no soporta model; se ignora", "aviso: gemini no soporta max_turns; se ignora"} {
		if !strings.Contains(out, want) {
			t.Fatalf("CA-121: falta el aviso %q en el log:\n%s", want, out)
		}
	}
}

// CA-121: con Strict, Start devuelve el ErrUnsupported del adapter y no crea
// run ni archivo de log.
func TestCA121_StrictNiegaYNoCreaRun(t *testing.T) {
	installFakeNamed(t, "gemini", "echo hola\nexit 0\n")
	root := t.TempDir()
	m := NewManager(root)

	_, err := m.Start(StartOptions{Provider: "gemini", Prompt: "hola", Model: "x", Strict: true})
	if err == nil {
		t.Fatal("CA-121: con Strict y campo no soportado, Start debe fallar")
	}
	var e providers.ErrUnsupported
	if !errors.As(err, &e) {
		t.Fatalf("CA-121: Start debe devolver el ErrUnsupported del adapter: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(root, ".hoom", "runs"))
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".jsonl") {
			t.Fatalf("CA-121: con Strict no debe crearse log de run: %s", en.Name())
		}
	}
}

// CA-122: ResolveSystemPrompt: @ruta devuelve el contenido del archivo; ruta
// inexistente => error que la nombra; texto sin @ inicial se devuelve literal;
// `@` solo es literal.
func TestCA122_ResolveSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rol.md")
	if err := os.WriteFile(p, []byte("contrato del rol"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSystemPrompt("@" + p)
	if err != nil || got != "contrato del rol" {
		t.Fatalf("CA-122: @ruta debe devolver el contenido del archivo: got=%q err=%v", got, err)
	}

	missing := filepath.Join(dir, "no-existe.md")
	if _, err := ResolveSystemPrompt("@" + missing); err == nil || !strings.Contains(err.Error(), "no-existe.md") {
		t.Fatalf("CA-122: una ruta inexistente debe fallar nombrandola: %v", err)
	}

	got, err = ResolveSystemPrompt("texto literal sin arroba")
	if err != nil || got != "texto literal sin arroba" {
		t.Fatalf("CA-122: el texto sin @ inicial se devuelve literal: got=%q err=%v", got, err)
	}

	got, err = ResolveSystemPrompt("@")
	if err != nil || got != "@" {
		t.Fatalf("CA-122: `@` solo (sin ruta) es literal: got=%q err=%v", got, err)
	}
}
