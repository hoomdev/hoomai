// Tests adversariales del spec .hoom/specs/codex-v2-y-review-cruzada.md
// (CA-168): E2E opcional contra el Codex real. Se omite salvo HOOM_E2E=1 y
// jamas es requisito de `go test`.
package providers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const contratoE2E = "# Rol de prueba\n\nContesta \"ok\" y nada mas.\n\tTab, backslash \\ y acentos: revisión.\n"

// CA-168: con HOOM_E2E=1 y `codex` real en PATH, el argv que devuelve Command
// corre con exit 0 y su stream da un start con SessionID y un end; y el mismo
// -c developer_instructions deja el contrato al principio del primer mensaje
// developer, lo que se comprueba con `codex debug prompt-input` SIN gastar una
// llamada al modelo.
func TestCA168_E2ECodexReal(t *testing.T) {
	if os.Getenv("HOOM_E2E") != "1" {
		t.Skip("CA-168: E2E opcional; exporta HOOM_E2E=1 y tene 'codex' en PATH para correrlo")
	}
	p, err := Lookup("codex")
	if err != nil {
		t.Fatalf("CA-168: %v", err)
	}
	if _, err := exec.LookPath(p.Bin()); err != nil {
		t.Skipf("CA-168: 'codex' no esta en PATH: %v", err)
	}

	// Codex se niega a correr fuera de un repo git: el E2E le da uno.
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"}, {"config", "user.email", "test@hoom.dev"},
		{"config", "user.name", "hoom test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("CA-168: git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(dir+"/a.txt", []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inv, err := p.Command(Request{Prompt: "Responde unicamente OK", ReadOnly: true})
	if err != nil {
		t.Fatalf("CA-168: Command fallo: %v", err)
	}
	cmd := exec.Command(inv.Bin, inv.Args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	exit := -1
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	if exit != 0 {
		t.Fatalf("CA-168: exit debe ser 0, fue %d (err=%v)\nstderr:\n%s", exit, runErr, stderr.String())
	}

	var startSID string
	var sawStart, sawEnd bool
	sc := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		for _, e := range p.Normalize(sc.Text()) {
			switch e.Kind {
			case "start":
				sawStart, startSID = true, e.SessionID
			case "end":
				sawEnd = true
			}
		}
	}
	if !sawStart || startSID == "" {
		t.Fatalf("CA-168: thread.started debe traer el id del hilo (visto=%v id=%q)\nstdout:\n%s", sawStart, startSID, stdout.String())
	}
	if !sawEnd {
		t.Fatalf("CA-168: el turno debe cerrar con un end\nstdout:\n%s", stdout.String())
	}

	// el contrato llega entero al principio del primer mensaje developer
	inv, err = p.Command(Request{Prompt: "hola", SystemPrompt: contratoE2E})
	if err != nil {
		t.Fatalf("CA-168: %v", err)
	}
	var flag string
	for i, a := range inv.Args {
		if a == "-c" && i+1 < len(inv.Args) && strings.HasPrefix(inv.Args[i+1], "developer_instructions=") {
			flag = inv.Args[i+1]
		}
	}
	debug := exec.Command(p.Bin(), "debug", "prompt-input", "-c", flag, "hola")
	debug.Dir = dir
	raw, err := debug.Output()
	if err != nil {
		t.Fatalf("CA-168: codex debug prompt-input fallo: %v", err)
	}
	var mensajes []struct {
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &mensajes); err != nil || len(mensajes) == 0 {
		t.Fatalf("CA-168: no se pudo leer el prompt del modelo: %v\n%s", err, raw)
	}
	var texto string
	for _, c := range mensajes[0].Content {
		texto += c.Text
	}
	if mensajes[0].Role != "developer" || !strings.HasPrefix(texto, contratoE2E) {
		t.Fatalf("CA-168: el contrato debe encabezar el primer mensaje developer (rol %q):\n%.400s", mensajes[0].Role, texto)
	}
	if len(texto) <= len(contratoE2E) {
		t.Fatal("CA-168: developer_instructions AGREGA; las instrucciones propias de Codex siguen ahi")
	}
}
