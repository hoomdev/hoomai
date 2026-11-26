package reviewcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
)

func TestLentesDeDeltaAcotaABloqueantes(t *testing.T) {
	ctx := finding.Contexto{BlockOn: "high"}
	prev := Record{ID: "r1", Task: "t", Cobertura: CoberturaCompleta, Findings: []string{"f1", "f2", "f3", "f4"}}
	items := []finding.Item{
		{Finding: finding.Finding{ID: "f1", Severity: "high", Lens: "risk", Task: "t"}, Status: finding.StatusCorrected},
		{Finding: finding.Finding{ID: "f2", Severity: "high", Lens: "resilience", Task: "t"}, Status: finding.StatusRefuted},
		{Finding: finding.Finding{ID: "f3", Severity: "low", Lens: "reliability", Task: "t"}, Status: finding.StatusOpen},
		{Finding: finding.Finding{ID: "f4", Severity: "high", Lens: "readability", Task: "t"}, Status: finding.StatusCorrected},
	}
	got := LentesDeDelta(prev, items, ctx)
	if !reflect.DeepEqual(got, []string{"risk"}) {
		t.Fatalf("LentesDeDelta debe acotar solo a risk (ignora refutado, low y readability): %v", got)
	}

	// Sin bloqueantes no refutados -> fallback a LentesDeUnaDelta
	prevClean := Record{ID: "r2", Task: "t", Cobertura: CoberturaCompleta, Findings: []string{}}
	got = LentesDeDelta(prevClean, items, ctx)
	if !reflect.DeepEqual(got, LentesDeUnaDelta) {
		t.Fatalf("LentesDeDelta sin bloqueantes cae a LentesDeUnaDelta: %v", got)
	}
}

func TestEvidenciaOmiteCuerpoDePruebasYAgregaIndice(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	wt := cbTarea(t, root, "precios")
	// Archivo de producto chico + archivo de pruebas grande con CA-1 y TestPrecioGrande
	var bigTest strings.Builder
	bigTest.WriteString("package app\n\nimport \"testing\"\n\n// CA-1: valida precio grande\nfunc TestPrecioGrande(t *testing.T) {\n")
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&bigTest, "\t_ = %d // MARCA_CUERPO_TEST_QUE_NO_ENTRA\n", i)
	}
	bigTest.WriteString("}\n")
	rdCommit(t, wt, "producto y prueba grande", map[string]string{
		"internal/precio.go":      "package app\n\nfunc Precio() int { return 42 }\n",
		"internal/precio_test.go": bigTest.String(),
	})
	crItem(t, root, "precios", "normal")
	cx := raInstalar(t, bin, "codex", "")

	res, out := rdRevisar(t, "evidencia sin tests", root, Options{Provider: "codex", Task: "precios"})
	if res.Status != "revisado" || len(res.Lenses) != 1 {
		t.Fatalf("review esperada revisado con 1 lente: %+v\n%s", res, out)
	}
	if !strings.Contains(out, "1 archivo de pruebas en indice") {
		t.Fatalf("encabezado debe informar archivo de pruebas en indice:\n%s", out)
	}
	prompt := cx.pedido(t, 1)
	if strings.Contains(prompt, "MARCA_CUERPO_TEST_QUE_NO_ENTRA") {
		t.Fatalf("el cuerpo del archivo de pruebas no debe entrar en el diff por defecto")
	}
	if !strings.Contains(prompt, "=== indice de pruebas (1 archivos omitidos del diff") ||
		!strings.Contains(prompt, "internal/precio_test.go") ||
		!strings.Contains(prompt, "TestPrecioGrande") ||
		!strings.Contains(prompt, "CA-1") {
		t.Fatalf("el prompt debe traer el indice de pruebas con archivo, simbolo y CA:\n%s", prompt)
	}
	if !strings.Contains(prompt, "func Precio() int { return 42 }") {
		t.Fatalf("el diff de producto debe seguir completo en el prompt")
	}

	// Con review.incluir_tests: true en la base, el cuerpo de pruebas si entra
	rootFull := raRepo(t, "review:\n  incluir_tests: true\n")
	wtFull := cbTarea(t, rootFull, "precios")
	rdCommit(t, wtFull, "producto y prueba grande", map[string]string{
		"internal/precio.go":      "package app\n\nfunc Precio() int { return 42 }\n",
		"internal/precio_test.go": bigTest.String(),
	})
	crItem(t, rootFull, "precios", "normal")
	cxFull := raInstalar(t, bin, "codex", "")
	resFull, outFull := rdRevisar(t, "evidencia con tests", rootFull, Options{Provider: "codex", Task: "precios"})
	if resFull.Status != "revisado" {
		t.Fatalf("review con incluir_tests=true: %+v\n%s", resFull, outFull)
	}
	if !strings.Contains(cxFull.pedido(t, 1), "MARCA_CUERPO_TEST_QUE_NO_ENTRA") {
		t.Fatalf("con incluir_tests: true el diff completo de pruebas si entra")
	}
	if res.EvidenceBytes >= resFull.EvidenceBytes {
		t.Fatalf("sin cuerpo de pruebas la evidencia debe ser mucho menor: sin=%d con=%d", res.EvidenceBytes, resFull.EvidenceBytes)
	}
}

func TestLentesEnParaleloRiesgoAlto(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  concurrencia: 4\n")
	wt := cbTarea(t, root, "pagos")
	rdCommit(t, wt, "cambio alto", map[string]string{
		"internal/auth/login.go": "package auth\n\nfunc Login() bool { return true }\n",
	})
	crItem(t, root, "pagos", "alto")
	callsDir := t.TempDir()
	script := "f=$(mktemp '" + callsDir + "/call.XXXXXX')\ncat > \"$f\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), cliFalso(script), 0o755); err != nil {
		t.Fatal(err)
	}

	res, out := rdRevisar(t, "paralelo alto", root, Options{Provider: "codex", Task: "pagos"})
	calls, err := filepath.Glob(filepath.Join(callsDir, "call.*"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, Lentes) || len(res.Passes) != 4 || len(calls) != 4 {
		t.Fatalf("review paralela en riesgo alto debe correr las 4 lentes en orden: %+v (%d pasadas)\n%s", res, len(calls), out)
	}
	for i, want := range Lentes {
		if res.Passes[i].Lens != want {
			t.Fatalf("orden de pasadas en paralelo: pos %d quiero %s, fue %s", i, want, res.Passes[i].Lens)
		}
	}
}
