// Tests adversariales del spec .hoom/specs/codex-v2-y-review-cruzada.md
// (CA-150): el sobre resuelve el limite por CAPACIDAD declarada, no por quien
// es el provider — y por eso corre con un segundo provider sin tocar el sobre.
package agentcmd

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

func TestCA150_LimitePorCapacidad(t *testing.T) {
	claude, _ := providers.Lookup("claude")
	codex, _ := providers.Lookup("codex")
	scout, _ := agents.Lookup("scout")
	reviewer, _ := agents.Lookup("reviewer")
	writer, _ := agents.Lookup("writer")

	// el que la declara la recibe, con el Exec del rol
	so, warn := startOptions(claude, scout, "C", Options{Prompt: "explora"})
	if !so.ReadOnly || so.Exec || warn {
		t.Fatalf("CA-150: claude declara read_only: %+v warn=%v", so, warn)
	}
	so, warn = startOptions(codex, reviewer, "C", Options{Prompt: "revisa"})
	if !so.ReadOnly || !so.Exec || warn {
		t.Fatalf("CA-150: codex tambien la declara, con exec: %+v warn=%v", so, warn)
	}
	// un rol de escritura nunca pide limite
	if so, _ = startOptions(codex, writer, "C", Options{Prompt: "implementa"}); so.ReadOnly || so.Exec {
		t.Fatalf("CA-150: un rol de escritura no se limita: %+v", so)
	}
	// el que no la declara avisa, y el sobre NO se niega: devuelve opciones
	// validas para arrancar igual (el gate de scope es la red)
	so, warn = startOptions(providerMudo{}, scout, "C", Options{Prompt: "explora"})
	if !warn || so.ReadOnly || !so.Strict || so.SystemPrompt != "C" {
		t.Fatalf("CA-150: sin la capacidad se avisa y el run igual arranca: %+v warn=%v", so, warn)
	}

	// y el sobre entero corre con el SEGUNDO provider, que es de lo que se
	// trataba: la abstraccion no estaba acoplada a Claude
	root := repo(t)
	fakeProvider(t, "codex", "exit 0\n")
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Role: "scout", Provider: "codex", Prompt: "explora"}, &out)
	if err != nil {
		t.Fatalf("CA-150: %v\n%s", err, out.String())
	}
	if res.Provider != "codex" || res.Stage != "ok" || res.ExitCode != 0 {
		t.Fatalf("CA-150: el sobre cierra verde con codex: %+v\n%s", res, out.String())
	}
	if res.RunID == "" || !strings.Contains(out.String(), "rol scout (codex)") {
		t.Fatalf("CA-150: el sobre nombra al provider que uso: %s", out.String())
	}
	// y el run recuerda que rol encarno, para que la review sepa quien escribio
	metas := runcmd.Metas(root)
	if len(metas) == 0 || metas[0].Provider != "codex" || metas[0].Role != "scout" {
		t.Fatalf("CA-150: el meta del run guarda provider y rol: %+v", metas)
	}
	_ = io.Discard
}
