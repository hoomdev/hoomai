// Test del writer para el bloqueo del dogfood del 2026-09-07: el sobre es
// desatendido por definicion, asi que TODO rol viaja con Unattended y el
// provider recibe sus herramientas de entrada; el gate de scope es el limite.
package agentcmd

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/providers"
)

func TestUnattended_ElSobreSiempreLoPide(t *testing.T) {
	prov, err := providers.Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"writer", "test-writer", "scout", "reviewer"} {
		role, err := agents.Lookup(slug)
		if err != nil {
			t.Fatal(err)
		}
		so, _ := startOptions(prov, role, "C", Options{Prompt: "trabaja"})
		if !so.Unattended {
			t.Fatalf("unattended: el sobre del rol %s debe pedirlo", slug)
		}
		if !so.Strict {
			t.Fatalf("unattended: el sobre sigue siendo estricto para %s", slug)
		}
	}
	// y la traduccion final de un rol que escribe concede la escritura
	w, _ := agents.Lookup("writer")
	so, _ := startOptions(prov, w, "C", Options{Prompt: "implementa"})
	inv, err := prov.Command(providers.Request{Prompt: so.Prompt, ReadOnly: so.ReadOnly, Exec: so.Exec, Unattended: so.Unattended, Strict: so.Strict, SystemPrompt: so.SystemPrompt})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for i, a := range inv.Args {
		if a == "--allowedTools" && i+1 < len(inv.Args) {
			joined = inv.Args[i+1]
		}
	}
	for _, want := range []string{"Edit", "Write", "Bash"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("unattended: el writer bajo claude debe recibir %s: %v", want, inv.Args)
		}
	}
}
