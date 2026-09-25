// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-400) sobre `hoom agent`: el aislamiento y el esfuerzo son solo de
// `hoom review`. Ningun rol del sobre —tampoco `hoom agent --role reviewer`—
// lleva Isolated ni Effort, aunque hoom.yaml tenga una seccion review: que
// los pida (CA-155: un rol de escritura no pisa la config del usuario).
//
// Guardas: el esqueleto ya las cumple (no hay aislamiento en ningun lado); lo
// que prueban es que la implementacion no lo derrame fuera de la review.
package agentcmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/providers"
)

// raRepoConReview es repo(t) con una seccion review: que pide aislamiento y
// esfuerzo: si el sobre la leyera, se veria en el argv.
func raRepoConReview(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n"+
		"review:\n  model: gpt-5.6-sol\n  effort: xhigh\n  isolated: true\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	return root
}

func raArgvDe(t *testing.T, dump string) []string {
	t.Helper()
	raw, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("CA-400: el CLI nunca corrio: %v", err)
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
}

func raSinAislamiento(t *testing.T, que string, args []string) {
	t.Helper()
	for _, a := range args {
		switch {
		case a == "--ignore-user-config", a == "--strict-mcp-config", a == "--setting-sources", a == "--effort",
			strings.HasPrefix(a, "model_reasoning_effort="), a == "xhigh":
			t.Fatalf("CA-400: %s no lleva aislamiento ni esfuerzo (%q): %v", que, a, args)
		}
	}
}

// CA-400: startOptions nunca pide Isolated ni Effort, para ningun rol y con
// los dos providers que los soportan.
func TestCA400_ElSobreNuncaPideAislamientoNiEsfuerzo(t *testing.T) {
	for _, name := range []string{"codex", "claude"} {
		prov, err := providers.Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, role := range agents.Roles() {
			so, _ := startOptions(prov, role, "C", Options{Prompt: "trabaja", Model: "m"})
			if so.Isolated || so.Effort != "" {
				t.Fatalf("CA-400: el rol %s en %s no lleva Isolated ni Effort: %+v", role.Slug, name, so)
			}
		}
	}
}

// CA-400: de punta a punta, `hoom agent --role reviewer` (codex) y el writer
// (claude) corren sin aislamiento ni esfuerzo aunque hoom.yaml los pida para
// la review.
func TestCA400_HoomAgentNoAislaAunqueHoomYamlLoPidaParaLaReview(t *testing.T) {
	root := raRepoConReview(t)
	dump := filepath.Join(t.TempDir(), "argv.txt")
	fakeProvider(t, "codex", "printf '%s\\n' \"$@\" > '"+dump+"'\nexit 0\n")
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Role: "reviewer", Provider: "codex", Prompt: "revisa el cambio"}, &out)
	if err != nil || res.RunID == "" {
		t.Fatalf("CA-400: hoom agent --role reviewer corre: %v %+v\n%s", err, res, out.String())
	}
	raSinAislamiento(t, "hoom agent --role reviewer", raArgvDe(t, dump))

	root2 := raRepoConReview(t)
	dump2 := filepath.Join(t.TempDir(), "argv.txt")
	fakeProvider(t, "claude", "printf '%s\\n' \"$@\" > '"+dump2+"'\nprintf 'package app // hecho\\n' > app.go\nexit 0\n")
	if _, err := Run(root2, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	raSinAislamiento(t, "el writer", raArgvDe(t, dump2))
}
