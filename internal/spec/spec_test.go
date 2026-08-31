// Tests adversariales del spec .hoom/specs/gates-ausentes-parciales-verifica.md
// (CA-68, CA-69, CA-70): criterios verificados por comando via el marcador
// "verifica: <comando>" entre corchetes, en la misma linea del CA.
package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/verdict"
)

const specSkeleton = `# Spec de prueba
## Objetivo
x
## No-goals
x
## Contratos
x
## Casos limite
x
## Criterios de aceptacion
%s
## Decisiones
x
## Riesgos
x
`

// writeSpec deja el spec en .hoom/specs (excluido del escaneo de tests) y
// un archivo de test en la raiz que referencia CA-2.
func writeSpec(t *testing.T, criteria string) (root, path string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, ".hoom", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "item.md")
	body := strings.Replace(specSkeleton, "%s", criteria, 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "item_test.txt"), []byte("cubre CA-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func gateByName(t *testing.T, gates []verdict.GateResult, name string) verdict.GateResult {
	t.Helper()
	for _, g := range gates {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("no existe el gate %q", name)
	return verdict.GateResult{}
}

// CA-68: un criterio con comando declarado (exit 0) queda trazado sin test;
// el resto sigue exigiendo su token en un archivo de test.
func TestCA68_ComandoExitCeroTraza(t *testing.T) {
	root, path := writeSpec(t, "- CA-1: tooling. [verifica: true]\n- CA-2: codigo.\n")
	gates := Gates(root, path)
	lint := gateByName(t, gates, "spec_lint")
	if lint.Status != verdict.StatusPass {
		t.Fatalf("CA-68: spec_lint debia pasar: %+v", lint)
	}
	trace := gateByName(t, gates, "spec_trace")
	if trace.Status != verdict.StatusPass {
		t.Fatalf("CA-68: spec_trace debia pasar: %+v", trace)
	}
	if !strings.Contains(trace.Notes, "verificados por comando") {
		t.Fatalf("CA-68: la nota debe declarar la verificacion por comando: %q", trace.Notes)
	}
}

// CA-69: comando fallido => spec_trace FAIL nombrando CA, comando y exit code.
func TestCA69_ComandoFallidoEsRojo(t *testing.T) {
	root, path := writeSpec(t, "- CA-1: tooling roto. [verifica: exit 3]\n- CA-2: codigo.\n")
	trace := gateByName(t, Gates(root, path), "spec_trace")
	if trace.Status != verdict.StatusFail {
		t.Fatalf("CA-69: esperaba FAIL: %+v", trace)
	}
	for _, want := range []string{"CA-1", "exit 3"} {
		if !strings.Contains(trace.OutputTail, want) {
			t.Fatalf("CA-69: la evidencia debe incluir %q: %q", want, trace.OutputTail)
		}
	}
}

// CA-69: comando inexistente => exit 127 visible (caso limite del spec).
func TestCA69_ComandoInexistente(t *testing.T) {
	root, path := writeSpec(t, "- CA-1: x. [verifica: comando-que-no-existe-hoom]\n- CA-2: y.\n")
	trace := gateByName(t, Gates(root, path), "spec_trace")
	if trace.Status != verdict.StatusFail || !strings.Contains(trace.OutputTail, "exit 127") {
		t.Fatalf("CA-69: esperaba FAIL con exit 127: %+v", trace)
	}
}

// CA-70: marcador huerfano (linea sin CA-n) => issue de spec_lint.
func TestCA70_MarcadorHuerfano(t *testing.T) {
	root, path := writeSpec(t, "- CA-1: x. [verifica: true]\n- CA-2: y.\nnota suelta [verifica: false]\n")
	lint := gateByName(t, Gates(root, path), "spec_lint")
	if lint.Status != verdict.StatusFail {
		t.Fatalf("CA-70: el marcador huerfano debe fallar spec_lint: %+v", lint)
	}
	if !strings.Contains(lint.OutputTail, "sin criterio CA-n") {
		t.Fatalf("CA-70: el issue debe explicar el huerfano: %q", lint.OutputTail)
	}
}

// CA-68 (control): un spec sin marcadores se comporta exactamente como antes.
func TestCA68_SinMarcadoresComoAntes(t *testing.T) {
	root, path := writeSpec(t, "- CA-1: a.\n- CA-2: b.\n")
	trace := gateByName(t, Gates(root, path), "spec_trace")
	if trace.Status != verdict.StatusFail {
		t.Fatalf("CA-68: CA-1 sin test ni comando debe fallar: %+v", trace)
	}
	if !strings.Contains(trace.OutputTail, "CA-1") || strings.Contains(trace.OutputTail, "CA-2") {
		t.Fatalf("CA-68: solo CA-1 esta sin trazar: %q", trace.OutputTail)
	}
}
