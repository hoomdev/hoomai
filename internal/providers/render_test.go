// Tests adversariales del spec .hoom/specs/providers-v2-interfaz-y-claude.md
// (CA-124): JSONBytes suma capabilities con los nueve booleanos por provider y
// conserva name/installed/bin y el orden; RenderText imprime estado y la linea
// de capacidades (o `texto plano` cuando no hay ninguna).
package providers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// CA-124: hoom providers --json y el render de texto. Se fija el PATH con un
// solo fake `claude` (como el CA-21 existente) para que installed sea
// determinista.
func TestCA124_JSONYRenderText(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "claude")) // SOLO claude existe

	raw, err := JSONBytes()
	if err != nil {
		t.Fatalf("CA-124: JSONBytes no debe fallar: %v", err)
	}
	var infos []Info
	if err := json.Unmarshal(raw, &infos); err != nil {
		t.Fatalf("CA-124: JSONBytes debe ser un array de Info: %v\n%s", err, raw)
	}

	want := []string{"claude", "opencode", "codex", "gemini"}
	if len(infos) != len(want) {
		t.Fatalf("CA-124: esperaba %d providers, hay %d", len(want), len(infos))
	}
	for i, n := range want {
		if infos[i].Name != n {
			t.Fatalf("CA-124: el orden debe ser %v; en %d hay %q", want, i, infos[i].Name)
		}
	}
	if !infos[0].Installed {
		t.Fatal("CA-124: claude esta en PATH y debe reportarse installed")
	}
	for _, in := range infos[1:] {
		if in.Installed {
			t.Fatalf("CA-124: %s ausente no puede reportarse installed", in.Name)
		}
	}
	// claude declara las nueve capacidades verdaderas.
	c := infos[0].Capabilities
	if !(c.Structured && c.Continue && c.Resume && c.SessionID && c.Model && c.SystemPrompt && c.Tools && c.MaxTurns && c.Budget) {
		t.Fatalf("CA-124: claude debe declarar las nueve capacidades: %+v", c)
	}
	// El JSON debe traer la clave capabilities con los nueve booleanos.
	for _, key := range []string{`"capabilities"`, `"structured"`, `"continue"`, `"resume"`, `"session_id"`, `"model"`, `"system_prompt"`, `"tools"`, `"max_turns"`, `"budget"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("CA-124: el JSON debe incluir %s:\n%s", key, raw)
		}
	}
	// claude installed trae bin; el campo se conserva.
	if infos[0].Bin == "" {
		t.Fatalf("CA-124: claude installed debe conservar bin: %+v", infos[0])
	}

	// RenderText: por provider su estado y la linea de capacidades; gemini, sin
	// ninguna, cae a `texto plano`.
	var buf bytes.Buffer
	RenderText(&buf, Detect())
	out := buf.String()
	for _, n := range want {
		if !strings.Contains(out, n) {
			t.Fatalf("CA-124: RenderText debe nombrar al provider %q:\n%s", n, out)
		}
	}
	if !strings.Contains(out, "capacidades:") {
		t.Fatalf("CA-124: RenderText debe imprimir la linea 'capacidades:':\n%s", out)
	}
	if !strings.Contains(out, "texto plano") {
		t.Fatalf("CA-124: un provider sin capacidades (gemini) debe mostrarse como 'texto plano':\n%s", out)
	}
}
