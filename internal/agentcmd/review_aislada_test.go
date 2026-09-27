// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-400) sobre `hoom agent`: el aislamiento y el esfuerzo son solo de
// `hoom review`. Ningun rol del sobre —tampoco `hoom agent --role reviewer`—
// lleva Isolated ni Effort, aunque hoom.yaml tenga una seccion review: que
// los pida (CA-155: un rol de escritura no pisa la config del usuario).
//
// Guardas: el esqueleto ya las cumple (no hay aislamiento en ningun lado); lo
// que prueban es que la implementacion no lo derrame fuera de la review.
//
// Enmienda 4 (CA-418): el pedido por stdin a cualquier tamano es solo de
// `hoom review`; los roles de `hoom agent` siguen como en CA-109 (un prompt
// chico va ultimo en argv, con el stdin vacio).
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

// CA-418: startOptions nunca pide PromptStdin, para ningun rol y con los dos
// providers que lo soportan: el prompt de `hoom agent` sigue la regla de
// CA-109 (stdin solo por encima de 16 KiB).
func TestCA418_ElSobreNuncaPidePromptStdin(t *testing.T) {
	for _, name := range []string{"codex", "claude"} {
		prov, err := providers.Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, role := range agents.Roles() {
			so, _ := startOptions(prov, role, "C", Options{Prompt: "trabaja", Model: "m"})
			if so.PromptStdin {
				t.Fatalf("CA-418: el rol %s en %s no manda su prompt por stdin (CA-109): %+v", role.Slug, name, so)
			}
		}
	}
}

// CA-418 (CA-109 se conserva), de punta a punta: `hoom agent --role reviewer`
// con codex y el writer con claude, con un prompt chico y aunque hoom.yaml
// tenga una seccion review:, llevan el prompt ULTIMO en argv y el stdin del
// proceso llega vacio.
func TestCA418_HoomAgentConservaElPromptChicoEnArgv(t *testing.T) {
	casos := []struct {
		provider, role, prompt, extra string
	}{
		{"codex", "reviewer", "revisa el cambio con cuidado", ""},
		{"claude", "writer", "implementa el spec hasta que verify de verde", "printf 'package app // hecho\\n' > app.go\n"},
	}
	for _, c := range casos {
		t.Run(c.provider+"/"+c.role, func(t *testing.T) {
			root := raRepoConReview(t)
			dir := t.TempDir()
			argv, in := filepath.Join(dir, "argv.txt"), filepath.Join(dir, "stdin.txt")
			fakeProvider(t, c.provider, "printf '%s\\n' \"$@\" > '"+argv+"'\ncat > '"+in+"'\n"+c.extra+"exit 0\n")
			opt := Options{Role: c.role, Provider: c.provider, Prompt: c.prompt}
			if _, err := Run(root, "main", opt, io.Discard); err != nil {
				t.Fatalf("CA-418: hoom agent --role %s corre: %v", c.role, err)
			}
			args := raArgvDe(t, argv)
			if args[len(args)-1] != c.prompt {
				t.Fatalf("CA-418/CA-109: el prompt chico de hoom agent --role %s sigue ultimo en argv: %q", c.role, args)
			}
			raw, err := os.ReadFile(in)
			if err != nil || len(raw) != 0 {
				t.Fatalf("CA-418/CA-109: hoom agent --role %s no manda su prompt chico por stdin: %v %q", c.role, err, raw)
			}
		})
	}
}
