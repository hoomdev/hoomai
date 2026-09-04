// Tests adversariales del spec .hoom/specs/hoom-agent-sobre-determinista.md
// (CA-147): E2E opcional contra el Claude real. Se omite salvo HOOM_E2E=1 y
// jamas es requisito de `go test`.
package agentcmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// copiaDeEsteRepo arma un proyecto temporal con el contenido REAL de este
// repo (mismo hoom.yaml, mismos contratos, mismo codigo) y su propio historial
// git. Trabajar sobre la copia y no sobre el original evita dos daños: un
// veredicto de prueba dentro de la evidencia versionada, y la recursion de
// `go test` llamandose a si mismo desde el gate de test.
func copiaDeEsteRepo(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if out, err := exec.Command("cp", "-R", src+"/.", dst).CombinedOutput(); err != nil {
		t.Fatalf("CA-147: no se pudo copiar el repo: %v\n%s", err, out)
	}
	if out, err := exec.Command("rm", "-rf", filepath.Join(dst, ".git")).CombinedOutput(); err != nil {
		t.Fatalf("CA-147: %v\n%s", err, out)
	}
	// Gates baratos: el E2E mide EL SOBRE, no la suite del proyecto.
	write(t, dst, "hoom.yaml", "schema: hoom/v1\nproject: hoomai-e2e\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	git(t, dst, "init", "-b", "main")
	git(t, dst, "config", "user.email", "test@hoom.dev")
	git(t, dst, "config", "user.name", "hoom test")
	git(t, dst, "add", "-A")
	git(t, dst, "commit", "-m", "copia para el E2E")
	return dst
}

// CA-147: con HOOM_E2E=1 y `claude` real en PATH, el sobre corre el rol scout
// con un turno y un dolar de tope sobre una copia de este repo: el run cierra
// en 0, la sesion del provider queda capturada y el scout —que es de solo
// lectura— no deja NINGUNA violacion de scope.
func TestCA147_E2ESobreEsteRepo(t *testing.T) {
	if os.Getenv("HOOM_E2E") != "1" {
		t.Skip("CA-147: E2E opcional; exporta HOOM_E2E=1 y tene 'claude' en PATH para correrlo")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skipf("CA-147: 'claude' no esta en PATH: %v", err)
	}
	root := copiaDeEsteRepo(t)
	res, err := Run(root, "main", Options{
		Role: "scout", Provider: "claude", MaxTurns: 1, BudgetUSD: 1,
		Prompt: "responde unicamente OK",
	}, io.Discard)
	if err != nil {
		t.Fatalf("CA-147: el sobre no debio fallar en su armado: %v", err)
	}
	if res.RunStatus != "done" {
		t.Fatalf("CA-147: el run debe cerrar bien: status=%q stage=%q exit=%d",
			res.RunStatus, res.Stage, res.ExitCode)
	}
	if res.SessionID == "" {
		t.Fatalf("CA-147: el sobre debe capturar la sesion del provider: %+v", res)
	}
	if !res.Scope.OK {
		t.Fatalf("CA-147: un scout no escribe: %+v", res.Scope.Violations)
	}
}
