// Tests adversariales del spec .hoom/specs/providers-v2-interfaz-y-claude.md
// (CA-126): E2E opcional contra el Claude real. Se omite salvo HOOM_E2E=1 y
// jamas es requisito de `go test`.
package providers

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"testing"
)

// CA-126: con HOOM_E2E=1 y `claude` real en PATH, correr el comando que
// devuelve Command (prompt corto, MaxTurns 1, BudgetUSD 1) en un directorio
// temporal debe salir 0 y, normalizando cada linea de stdout, exigir SessionID
// no vacio en el evento start y en el evento de cierre (end o error).
func TestCA126_E2EClaudeReal(t *testing.T) {
	if os.Getenv("HOOM_E2E") != "1" {
		t.Skip("CA-126: E2E opcional; exporta HOOM_E2E=1 y tene 'claude' en PATH para correrlo")
	}
	p, err := Lookup("claude")
	if err != nil {
		t.Fatalf("CA-126: %v", err)
	}
	if _, err := exec.LookPath(p.Bin()); err != nil {
		t.Skipf("CA-126: 'claude' no esta en PATH: %v", err)
	}

	inv, err := p.Command(Request{Prompt: "responde unicamente OK", MaxTurns: 1, BudgetUSD: 1})
	if err != nil {
		t.Fatalf("CA-126: Command fallo: %v", err)
	}

	cmd := exec.Command(inv.Bin, inv.Args...)
	cmd.Dir = t.TempDir()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	exit := -1
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	if exit != 0 {
		t.Fatalf("CA-126: exit debe ser 0, fue %d (err=%v)\nstderr:\n%s", exit, runErr, stderr.String())
	}

	var startSID, closeSID string
	var sawStart, sawClose bool
	sc := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		for _, e := range p.Normalize(sc.Text()) {
			switch e.Kind {
			case "start":
				sawStart = true
				startSID = e.SessionID
			case "end", "error":
				sawClose = true
				closeSID = e.SessionID
			}
		}
	}

	if !sawStart || startSID == "" {
		t.Fatalf("CA-126: el evento start debe traer SessionID no vacio (visto=%v id=%q)\nstdout:\n%s", sawStart, startSID, stdout.String())
	}
	if !sawClose || closeSID == "" {
		t.Fatalf("CA-126: el evento de cierre (end/error) debe traer SessionID no vacio (visto=%v id=%q)\nstdout:\n%s", sawClose, closeSID, stdout.String())
	}
}
