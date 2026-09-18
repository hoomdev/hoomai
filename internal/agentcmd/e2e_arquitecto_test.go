// Test adversarial del spec .hoom/specs/arquitecto-bajo-el-sobre.md (CA-246):
// E2E opcional contra el Claude real. Es el dogfood del 2026-09-07 al reves:
// el arquitecto bajo el sobre ESCRIBE su spec y el sobre certifica esa
// entrega. Se omite salvo HOOM_E2E=1 y jamas es requisito de `go test`.
package agentcmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// CA-246: con HOOM_E2E=1 y `claude` real en PATH, `hoom agent --role
// arquitecto --max-turns 3 --budget-usd 1` sobre una copia de este repo, con
// un pedido que manda escribir un spec trivial, cierra entregable y
// Delivered() contiene ese spec.
func TestCA246_E2EArquitectoEscribeSuSpec(t *testing.T) {
	if os.Getenv("HOOM_E2E") != "1" {
		t.Skip("CA-246: E2E opcional; exporta HOOM_E2E=1 y tene 'claude' en PATH para correrlo")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skipf("CA-246: 'claude' no esta en PATH: %v", err)
	}
	root := copiaDeEsteRepo(t)
	const spec = ".hoom/specs/e2e-arquitecto-trivial.md"
	res, err := Run(root, "main", Options{
		Role: "arquitecto", Provider: "claude", MaxTurns: 3, BudgetUSD: 1,
		Prompt: "Escribi un spec trivial en el archivo " + spec + ": un titulo '# Spec trivial' y una " +
			"linea que diga 'Spec de prueba del E2E CA-246.'. No leas nada mas, no toques ningun otro archivo.",
	}, io.Discard)
	if err != nil {
		t.Fatalf("CA-246: el sobre no debio fallar en su armado: %v", err)
	}
	if res.RunStatus != "done" {
		t.Fatalf("CA-246: el run debe cerrar bien: status=%q stage=%q exit=%d",
			res.RunStatus, res.Stage, res.ExitCode)
	}
	var entregado bool
	for _, p := range res.Scope.Delivered() {
		if p == spec {
			entregado = true
		}
	}
	if !entregado {
		t.Fatalf("CA-246: Delivered() debe contener %s: %v (violaciones %+v)",
			spec, res.Scope.Delivered(), res.Scope.Violations)
	}
	if _, err := os.Stat(filepath.Join(root, spec)); err != nil {
		t.Fatalf("CA-246: el spec debe existir en el arbol: %v", err)
	}
	if res.Stage != "ok" || res.Status != "entregable" || res.ExitCode != 0 {
		t.Fatalf("CA-246: el arquitecto que entrega su spec cierra entregable: stage=%q status=%q exit=%d violaciones=%+v",
			res.Stage, res.Status, res.ExitCode, res.Scope.Violations)
	}
}
