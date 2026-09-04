// Tests adversariales del spec .hoom/specs/codex-v2-y-review-cruzada.md
// (CA-169): E2E opcional de la review cruzada contra el Codex real. Se omite
// salvo HOOM_E2E=1 y jamas es requisito de `go test`.
package reviewcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// copiaDeEsteRepo arma un proyecto temporal con el contenido REAL de este repo
// y su propio historial git. Trabajar sobre la copia y no sobre el original
// evita ensuciar la evidencia versionada con una review de prueba.
func copiaDeEsteRepo(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if out, err := exec.Command("cp", "-R", src+"/.", dst).CombinedOutput(); err != nil {
		t.Fatalf("CA-169: no se pudo copiar el repo: %v\n%s", err, out)
	}
	if out, err := exec.Command("rm", "-rf", filepath.Join(dst, ".git")).CombinedOutput(); err != nil {
		t.Fatalf("CA-169: %v\n%s", err, out)
	}
	git(t, dst, "init", "-b", "main")
	git(t, dst, "config", "user.email", "test@hoom.dev")
	git(t, dst, "config", "user.name", "hoom test")
	git(t, dst, "add", "-A")
	git(t, dst, "commit", "-m", "copia para el E2E")
	return dst
}

// CA-169: con HOOM_E2E=1, `codex` en PATH y un cambio plantado sobre una copia
// de este repo, la review cierra con exit 0, sin violaciones de scope, sin
// veredicto nuevo, y reporta la lista —posiblemente vacia— de hallazgos que
// hoom vio aparecer.
func TestCA169_E2EReviewCruzada(t *testing.T) {
	if os.Getenv("HOOM_E2E") != "1" {
		t.Skip("CA-169: E2E opcional; exporta HOOM_E2E=1 y tene 'codex' en PATH para correrlo")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skipf("CA-169: 'codex' no esta en PATH: %v", err)
	}

	root := copiaDeEsteRepo(t)
	// el cambio plantado: chico a proposito, y con algo que decir para la
	// lente risk
	write(t, root, "internal/demo/token.go", `package demo

// TokenFijo es una credencial hardcodeada: el reviewer deberia verla.
const TokenFijo = "sk-demo-1234567890"

func Autoriza(dado string) bool { return dado == TokenFijo }
`)
	antes := verdictosEn(t, root)

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Provider: "codex", Lens: "risk"}, &out)
	if err != nil {
		t.Fatalf("CA-169: %v\n%s", err, out.String())
	}
	t.Logf("CA-169: salida de la review:\n%s", out.String())

	if res.Provider != "codex" || len(res.Lenses) != 1 || res.Lenses[0] != "risk" {
		t.Fatalf("CA-169: una pasada con la lente pedida: %+v", res)
	}
	if len(res.Passes) != 1 || res.Passes[0].RunStatus != "done" {
		t.Fatalf("CA-169: el run del reviewer debe cerrar bien: %+v", res.Passes)
	}
	if !res.Passes[0].Scope.OK {
		t.Fatalf("CA-169: el reviewer no escribe fuera de su territorio: %+v", res.Passes[0].Scope)
	}
	if res.ExitCode != 0 || res.Status != "revisado" {
		t.Fatalf("CA-169: la review cierra en verde: %+v", res)
	}
	if despues := verdictosEn(t, root); despues != antes {
		t.Fatalf("CA-169: la review no emite veredicto: %d -> %d", antes, despues)
	}
	t.Logf("CA-169: hallazgos nuevos observados por hoom: %v", res.Findings)
}
