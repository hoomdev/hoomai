// Tests adversariales del spec .hoom/specs/codex-v2-y-review-cruzada.md:
// el limite de un rol deja de ser el dialecto de un CLI y pasa a ser una
// intencion del vocabulario comun.
package providers

import (
	"errors"
	"strings"
	"testing"
)

// CA-148: la capacidad existe y se nombra; la intencion viaja si el provider
// la declara, se ignora si no, y bajo Strict es ErrUnsupported. Exec sin
// ReadOnly no dice nada.
func TestCA148_IntencionDeSoloLectura(t *testing.T) {
	caps := Capabilities{Tools: true, ReadOnly: true, MaxTurns: true}
	names := strings.Join(caps.Names(), ",")
	if names != "tools,read_only,max_turns" {
		t.Fatalf("CA-148: read_only se nombra despues de tools: %q", names)
	}
	if !strings.Contains(Capabilities{ReadOnly: true}.Summary(), "read_only") {
		t.Fatal("CA-148: el resumen humano tambien la nombra")
	}

	// claude la declara: la intencion llega al plan
	claude, _ := Lookup("claude")
	inv, err := claude.Command(Request{Prompt: "hola", ReadOnly: true, Strict: true})
	if err != nil {
		t.Fatalf("CA-148: un provider que la declara no puede fallar: %v", err)
	}
	if len(inv.Ignored) != 0 {
		t.Fatalf("CA-148: nada que ignorar: %v", inv.Ignored)
	}

	// gemini no la declara: se ignora con su nombre canonico
	gemini, _ := Lookup("gemini")
	inv, err = gemini.Command(Request{Prompt: "hola", ReadOnly: true, Exec: true})
	if err != nil {
		t.Fatalf("CA-148: sin Strict se degrada, no se falla: %v", err)
	}
	if len(inv.Ignored) != 1 || inv.Ignored[0] != FieldReadOnly {
		t.Fatalf("CA-148: la degradacion se declara como read_only: %v", inv.Ignored)
	}

	// ...y con Strict es un ErrUnsupported que nombra el campo
	_, err = gemini.Command(Request{Prompt: "hola", ReadOnly: true, Strict: true})
	var eu ErrUnsupported
	if !errors.As(err, &eu) || len(eu.Fields) != 1 || eu.Fields[0] != FieldReadOnly {
		t.Fatalf("CA-148: con Strict se niega nombrando read_only: %v", err)
	}

	// Exec solo no significa nada: sin ReadOnly no hay limite que pedir
	inv, err = gemini.Command(Request{Prompt: "hola", Exec: true, Strict: true})
	if err != nil || len(inv.Ignored) != 0 {
		t.Fatalf("CA-148: Exec sin ReadOnly no pide nada: %v %v", inv.Ignored, err)
	}
}

// CA-149: claude traduce la intencion a SUS nombres de herramientas —el mismo
// vocabulario que ya escribe en .claude/agents/*.md— y conserva lo que el
// llamador haya pedido por su cuenta.
func TestCA149_ClaudeTraduceElLimite(t *testing.T) {
	claude, _ := Lookup("claude")
	arg := func(req Request) string {
		inv, err := claude.Command(req)
		if err != nil {
			t.Fatalf("CA-149: %v", err)
		}
		return strings.Join(inv.Args, " ")
	}

	sinExec := arg(Request{Prompt: "hola", ReadOnly: true})
	if !strings.Contains(sinExec, "--allowedTools Read,Grep,Glob ") {
		t.Fatalf("CA-149: allow de un rol que no ejecuta: %s", sinExec)
	}
	for _, want := range []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"} {
		if !strings.Contains(denyDe(t, sinExec), want) {
			t.Fatalf("CA-149: %s debe estar prohibido: %s", want, sinExec)
		}
	}

	conExec := arg(Request{Prompt: "hola", ReadOnly: true, Exec: true})
	if !strings.Contains(conExec, "--allowedTools Read,Grep,Glob,Bash ") {
		t.Fatalf("CA-149: un rol que ejecuta necesita Bash en allow: %s", conExec)
	}
	if strings.Contains(denyDe(t, conExec), "Bash") {
		t.Fatalf("CA-149: ...y Bash no puede estar tambien en deny: %s", conExec)
	}

	// un rol de escritura no declara ninguna bandera de herramientas
	if libre := arg(Request{Prompt: "hola"}); strings.Contains(libre, "Tools") {
		t.Fatalf("CA-149: sin ReadOnly no hay banderas de herramientas: %s", libre)
	}

	// lo que el llamador pidio por su cuenta se conserva y no se repite
	mixto := arg(Request{Prompt: "hola", ReadOnly: true, AllowTools: []string{"Read", "WebFetch"}})
	if !strings.Contains(mixto, "--allowedTools Read,WebFetch,Grep,Glob ") {
		t.Fatalf("CA-149: el allow del llamador se conserva, deduplicado y en orden: %s", mixto)
	}
}

// denyDe extrae el valor de --disallowedTools de un argv ya unido.
func denyDe(t *testing.T, argv string) string {
	t.Helper()
	i := strings.Index(argv, "--disallowedTools ")
	if i < 0 {
		return ""
	}
	rest := argv[i+len("--disallowedTools "):]
	if j := strings.Index(rest, " "); j >= 0 {
		return rest[:j]
	}
	return rest
}
