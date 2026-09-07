// Tests del writer para el bloqueo del dogfood del 2026-09-07 (run
// 20260907T045901_04107d, rol test-writer bajo Claude): el sobre lanzaba un
// rol que ESCRIBE sin ninguna herramienta preaprobada, y en modo headless
// Claude Code niega en silencio cada Write. La intencion Unattended viaja en
// el Request y cada adapter la traduce a lo suyo.
package providers

import (
	"errors"
	"strings"
	"testing"
)

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// Claude: un rol que escribe y al que nadie atiende recibe lectura,
// escritura y shell por nombre, y el prompt sigue siendo el ultimo argumento.
func TestUnattended_ClaudeConcedeEscrituraPorNombre(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := p.Command(Request{Prompt: "implementa", Unattended: true, Strict: true})
	if err != nil {
		t.Fatalf("unattended: claude lo soporta y no debe fallar: %v", err)
	}
	allow := flagValue(inv.Args, "--allowedTools")
	for _, want := range []string{"Read", "Edit", "Write", "Bash"} {
		if !strings.Contains(allow, want) {
			t.Fatalf("unattended: falta %s en --allowedTools %q (args %v)", want, allow, inv.Args)
		}
	}
	if flagValue(inv.Args, "--disallowedTools") != "" {
		t.Fatalf("unattended: un rol que escribe no lleva herramientas prohibidas: %v", inv.Args)
	}
	if inv.Args[len(inv.Args)-1] != "implementa" {
		t.Fatalf("unattended: el prompt debe seguir ultimo: %v", inv.Args)
	}
	if len(inv.Ignored) != 0 {
		t.Fatalf("unattended: nada ignorado en claude: %v", inv.Ignored)
	}
}

// Claude: con solo lectura, Unattended NO agrega escritura: el conjunto de
// lectura sigue siendo el limite y Edit/Write quedan prohibidos.
func TestUnattended_ConSoloLecturaNoAbreLaEscritura(t *testing.T) {
	p, _ := Lookup("claude")
	inv, err := p.Command(Request{Prompt: "revisa", ReadOnly: true, Exec: true, Unattended: true})
	if err != nil {
		t.Fatal(err)
	}
	allow, deny := flagValue(inv.Args, "--allowedTools"), flagValue(inv.Args, "--disallowedTools")
	if strings.Contains(allow, "Edit") || strings.Contains(allow, "Write") {
		t.Fatalf("unattended+read_only: no debe permitir escritura: %q", allow)
	}
	if !strings.Contains(deny, "Edit") || !strings.Contains(deny, "Write") {
		t.Fatalf("unattended+read_only: la escritura sigue prohibida: %q", deny)
	}
	if !strings.Contains(allow, "Bash") {
		t.Fatalf("unattended+read_only+exec: el shell sigue permitido: %q", allow)
	}
}

// Codex: nunca pregunta, asi que la intencion se traduce en el sandbox: un
// rol que escribe recibe workspace-write explicito; el de solo lectura no
// cambia.
func TestUnattended_CodexFijaElSandbox(t *testing.T) {
	p, _ := Lookup("codex")
	inv, err := p.Command(Request{Prompt: "implementa", Unattended: true, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(inv.Args, " "), `sandbox_mode="workspace-write"`) {
		t.Fatalf("unattended: codex debe fijar workspace-write: %v", inv.Args)
	}
	ro, _ := p.Command(Request{Prompt: "lee", ReadOnly: true, Unattended: true})
	if !strings.Contains(strings.Join(ro.Args, " "), `sandbox_mode="read-only"`) {
		t.Fatalf("unattended+read_only: codex sigue en read-only: %v", ro.Args)
	}
}

// Sin la capacidad se declara: gemini lo ignora con nombre canonico y con
// Strict se niega; la capacidad se nombra en la lista de hoom providers.
func TestUnattended_SinCapacidadSeDeclara(t *testing.T) {
	g, _ := Lookup("gemini")
	inv, err := g.Command(Request{Prompt: "hola", Unattended: true})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(inv.Ignored, FieldUnattended) {
		t.Fatalf("unattended: gemini debe declararlo ignorado: %v", inv.Ignored)
	}
	_, err = g.Command(Request{Prompt: "hola", Unattended: true, Strict: true})
	var eu ErrUnsupported
	if !errors.As(err, &eu) || !contains(eu.Fields, FieldUnattended) {
		t.Fatalf("unattended+strict: gemini debe negarse con ErrUnsupported: %v", err)
	}
	c, _ := Lookup("claude")
	if !contains(c.Capabilities().Names(), "unattended") {
		t.Fatalf("unattended: claude debe nombrar la capacidad: %v", c.Capabilities().Names())
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
