// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-421, CA-422, CA-423) de punta a punta, por el binario real de hoom
// (compilado en un directorio temporal; se omiten con -short): --desde y
// --delta juntos son un error de uso con exit 2 y sin efectos; --json trae
// desde, hasta, cobertura y desde_review; un --desde invalido sale con su
// error. Los fixtures estan en review_por_diferencia_helpers_test.go.
package reviewcmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// rdFoto fotografia el arbol (sin .git/): cada archivo con el sha256 de su
// contenido.
func rdFoto(t *testing.T, root string) map[string]string {
	t.Helper()
	foto := map[string]string{}
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if fi.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if !fi.Mode().IsRegular() {
			foto[rel] = fi.Mode().String()
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		foto[rel] = fmt.Sprintf("%x", sha256.Sum256(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return foto
}

// CA-422: "--desde y --delta juntos: error de uso `hoom review: --desde y
// --delta no van juntos`, exit 2, sin efectos". En cualquier orden, con
// --desde=<c>, con --json: exit 2, el mensaje, el provider no corre, ningun
// registro, y el arbol queda byte a byte igual (ni sobres ni runs).
func TestCA422_E2EDesdeYDeltaJuntosEsErrorDeUso(t *testing.T) {
	hoom := hbHoomReal(t)
	bin := raPATH(t)
	root, _, a, _ := rdRamaDos(t, "", ".hoom/specs/x.md")
	cx := raInstalar(t, bin, "codex", "")
	raLimpio(t, "CA-422", root)
	antes := rdFoto(t, root)
	for _, args := range [][]string{
		{"review", "--provider", "codex", "--desde", a, "--delta"},
		{"review", "--provider", "codex", "--delta", "--desde", a},
		{"review", "--provider", "codex", "--desde=" + a[:12], "--delta", "--json"},
		{"review", "--spec", ".hoom/specs/x.md", "--delta", "--desde", "main"},
	} {
		code, out, errOut := raHoom(t, hoom, root, args...)
		if code != 2 {
			t.Fatalf("CA-422: %v: --desde con --delta sale con 2, salio con %d:\n%s\n%s", args, code, out, errOut)
		}
		if !strings.Contains(out+errOut, "hoom review: --desde y --delta no van juntos") {
			t.Fatalf("CA-422: %v: el error de uso dice 'hoom review: --desde y --delta no van juntos':\n%s\n%s", args, out, errOut)
		}
		if cx.veces() != 0 {
			t.Fatalf("CA-422: %v: sin efectos: el provider no corre (%d)", args, cx.veces())
		}
		if despues := rdFoto(t, root); !reflect.DeepEqual(antes, despues) {
			t.Fatalf("CA-422: %v: sin efectos: el arbol queda igual:\nantes   %v\ndespues %v", args, antes, despues)
		}
	}
}

// CA-423: "el registro, el Result y --json llevan desde, hasta (40 hex),
// cobertura y desde_review", de punta a punta: `hoom review --json` sin
// rango (completa, del merge-base a HEAD), con --desde a mano (parcial, sin
// desde_review) y con --delta (delta, con desde_review).
func TestCA423_E2EJSONConDesdeHastaYCobertura(t *testing.T) {
	hoom := hbHoomReal(t)
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root, mb, a, b := rdRamaDos(t, "", spec)
	raInstalar(t, bin, "codex", "")

	correr := func(args ...string) map[string]any {
		t.Helper()
		raLimpio(t, "CA-423", root)
		code, out, errOut := raHoom(t, hoom, root, args...)
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil || code != 0 {
			t.Fatalf("CA-423: %v: exit 0 y JSON en stdout (exit %d, %v):\n%s\n%s", args, code, err, out, errOut)
		}
		return m
	}
	m := correr("review", "--provider", "codex", "--spec", spec, "--json")
	if m["desde"] != mb || m["hasta"] != b || m["cobertura"] != CoberturaCompleta {
		t.Fatalf("CA-423: --json sin rango: desde %s, hasta %s, completa: %v", mb[:12], b[:12], m)
	}
	if v, ok := m["desde_review"]; ok && v != "" {
		t.Fatalf("CA-423: --json sin rango no nombra desde_review: %v", v)
	}
	completa, _ := m["record_id"].(string)

	m = correr("review", "--provider", "codex", "--spec", spec, "--json", "--desde", a)
	if m["desde"] != a || m["hasta"] != b || m["cobertura"] != CoberturaParcial {
		t.Fatalf("CA-423: --json --desde a mano: desde %s, hasta %s, parcial: %v", a[:12], b[:12], m)
	}
	if v, ok := m["desde_review"]; ok && v != "" {
		t.Fatalf("CA-423: un parcial no nombra desde_review: %v", v)
	}

	rdEsperarSegundo()
	c := rdCommit(t, root, "C", map[string]string{"c.go": "package app\n\nfunc C() {}\n"})
	m = correr("review", "--provider", "codex", "--spec", spec, "--json", "--delta")
	if m["desde"] != b || m["hasta"] != c || m["cobertura"] != CoberturaDelta || m["desde_review"] != completa || completa == "" {
		t.Fatalf("CA-423: --json --delta: desde %s, hasta %s, delta, desde_review %s: %v", b[:12], c[:12], completa, m)
	}
}

// CA-421, de punta a punta: `hoom review --desde <c>` con un <c> que no es
// un commit, que no es ancestro de HEAD o que empieza con '-' (tomado como
// valor del flag) sale con un codigo distinto de 0 y su error, sin pasadas
// ni registro, y un valor --output=<ruta> no crea el archivo.
func TestCA421_E2EDesdeInvalidoPorLaCLI(t *testing.T) {
	hoom := hbHoomReal(t)
	bin := raPATH(t)
	root, _, _, _ := rdRamaDos(t, "", "")
	git(t, root, "checkout", "-q", "main")
	write(t, root, "main.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "main avanza")
	deMain := rdSha(t, root, "HEAD")
	git(t, root, "checkout", "-q", "feature")
	cx := raInstalar(t, bin, "codex", "")
	pwned := filepath.Join(t.TempDir(), "pwned-cli-ca421")
	for _, c := range []struct{ desde, dice string }{
		{"no-existe-rd", rdErrNoCommit("no-existe-rd")},
		{deMain, rdErrNoAncestro(deMain)},
		{"--output=" + pwned, "--desde --output=" + pwned + ": "},
	} {
		raLimpio(t, "CA-421", root)
		code, out, errOut := raHoom(t, hoom, root, "review", "--provider", "codex", "--desde", c.desde)
		if code == 0 || !strings.Contains(out+errOut, c.dice) {
			t.Fatalf("CA-421: --desde %s sale con != 0 y dice %q (exit %d):\n%s\n%s", c.desde, c.dice, code, out, errOut)
		}
		if cx.veces() != 0 || rdRegistrosEn(root) != 0 {
			t.Fatalf("CA-421: --desde %s: sin pasadas (%d) ni registro (%d)", c.desde, cx.veces(), rdRegistrosEn(root))
		}
		if _, err := os.Stat(pwned); err == nil {
			t.Fatalf("CA-421: --desde %s no llega a git: se creo %s", c.desde, pwned)
		}
	}
}
