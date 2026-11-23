// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// POST /api/verify, que corre el mismo verifycmd.Run que `hoom verify`:
//
//   - CA-765: con .hoom/local.yaml, el verify del Studio corre los gates con
//     la plantilla y el veredicto lleva entorno;
//   - CA-763: un .hoom/local.yaml invalido no da 200, el error lleva el
//     texto exacto, ningun gate corre y no hay veredicto;
//   - CA-773: un gate que crea el .hoom/local.yaml deja entorno_local.
//
// Proyecto fuera de git (fuera de git el principal es la raiz, y no hay
// ErrNoIgnorado). Un `ddev` falso primero en el PATH. Escritos desde el
// spec, las aclaraciones de la ronda 0 y el go doc, sin leer la
// implementacion.
package servecmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	hentorno "github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// veeProyecto es un proyecto fuera de git con el gate test requerido cmd.
func veeProyecto(t *testing.T, cmd string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("el ddev falso es un script de sh")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: " + fmt.Sprintf("%q", cmd) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func veeLocal(t *testing.T, dir, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(hentorno.Ruta))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// veeDdev pone primero en el PATH un `ddev` falso que anota su directorio y
// sus argumentos fuera del proyecto y corre `sh -c` con el ultimo.
func veeDdev(t *testing.T) string {
	t.Helper()
	bin, log := t.TempDir(), t.TempDir()
	q := "'" + strings.ReplaceAll(log, "'", `'\''`) + "'"
	script := "#!/bin/sh\n" +
		"log=" + q + "\n" +
		"{ printf '%s\\000' \"$(pwd -P)\"; for a do printf '%s\\000' \"$a\"; done; } > \"$log/.tmp.$$\"\n" +
		"mv \"$log/.tmp.$$\" \"$log/llamada.$$\"\n" +
		"for ultimo do :; done\n" +
		"VEE_DENTRO=1\nexport VEE_DENTRO\n" +
		"exec sh -c \"$ultimo\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ddev"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func veeLlamadas(t *testing.T, log string) [][]string {
	t.Helper()
	ents, err := os.ReadDir(log)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "llamada.") {
			b, err := os.ReadFile(filepath.Join(log, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"))
		}
	}
	return out
}

// veeVerify hace POST /api/verify y devuelve el codigo, el cuerpo y el
// veredicto si el cuerpo lo es.
func veeVerify(t *testing.T, dir string) (int, string, *verdict.Verdict) {
	t.Helper()
	s := newServer(t, dir)
	rec := doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		return rec.Code, body, nil
	}
	var v verdict.Verdict
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("el cuerpo de un 200 es el veredicto: %v\n%s", err, body)
	}
	return rec.Code, body, &v
}

// veeDice dice si el cuerpo lleva el texto, crudo o como un valor de JSON.
func veeDice(body, texto string) bool {
	if strings.Contains(body, texto) {
		return true
	}
	var x any
	if json.Unmarshal([]byte(body), &x) != nil {
		return false
	}
	var buscar func(any) bool
	buscar = func(v any) bool {
		switch y := v.(type) {
		case string:
			return strings.Contains(y, texto)
		case map[string]any:
			for _, z := range y {
				if buscar(z) {
					return true
				}
			}
		case []any:
			for _, z := range y {
				if buscar(z) {
					return true
				}
			}
		}
		return false
	}
	return buscar(x)
}

func veeGateDe(v *verdict.Verdict, nombre string) *verdict.GateResult {
	for i := range v.Gates {
		if v.Gates[i].Name == nombre {
			return &v.Gates[i]
		}
	}
	return nil
}

// CA-765: POST /api/verify corre los gates con la plantilla de
// .hoom/local.yaml, en el directorio del proyecto, con {rel} "."; el
// veredicto lleva entorno y el gate el suyo.
func TestCA765_VerifyDelStudioUsaElPrefijo(t *testing.T) {
	const ca = "CA-765"
	dir := veeProyecto(t, "echo dentro=${VEE_DENTRO:-no}")
	log := veeDdev(t)
	tpl := "ddev exec -d /var/www/html/{rel} sh -c {cmd}"
	veeLocal(t, dir, "ejecutar: \""+tpl+"\"\n")
	code, body, v := veeVerify(t, dir)
	if code != http.StatusOK || v == nil {
		t.Fatalf("%s: 200 con el veredicto: %d %s", ca, code, body)
	}
	if v.Entorno == nil || v.Entorno.Ejecutar != tpl {
		t.Fatalf("%s: el veredicto lleva entorno: %s", ca, body)
	}
	g := veeGateDe(v, "test")
	if g == nil || g.Entorno != verdict.EntornoPrefijo || !strings.Contains(g.OutputTail, "dentro=1") {
		t.Fatalf("%s: el gate corre con el prefijo: %+v", ca, g)
	}
	ll := veeLlamadas(t, log)
	if len(ll) != 1 || ll[0][0] != dir || len(ll[0]) < 4 || ll[0][3] != "/var/www/html/." {
		t.Fatalf("%s: el prefijo corre una vez en %s con {rel} \".\": %q", ca, dir, ll)
	}
}

// CA-763: con un .hoom/local.yaml invalido, POST /api/verify no da 200, su
// respuesta lleva el error exacto, el gate no corre y no hay veredicto.
func TestCA763_VerifyDelStudioConArchivoInvalido(t *testing.T) {
	const ca = "CA-763"
	for _, c := range []struct{ body, want string }{
		{"ejecutar: \"ddev exec sh -c\"\n", hentorno.ErrSinCmd},
		{"ejecutar: \"ddev {cmd}\"\nfoo: 1\n", fmt.Sprintf(hentorno.ErrClaveDesconocida, "foo")},
		{"en_el_host: [semgrep]\n", fmt.Sprintf(hentorno.ErrGateDesconocido, "semgrep")},
	} {
		dir := veeProyecto(t, "touch vee-corrio")
		veeLocal(t, dir, c.body)
		code, body, _ := veeVerify(t, dir)
		if code == http.StatusOK || !veeDice(body, c.want) {
			t.Fatalf("%s: %q: no es 200 y lleva %q: %d %s", ca, c.body, c.want, code, body)
		}
		if _, err := os.Stat(filepath.Join(dir, "vee-corrio")); err == nil {
			t.Fatalf("%s: %q: ningun gate corre", ca, c.body)
		}
		if ents, _ := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts")); len(ents) != 0 {
			t.Fatalf("%s: %q: no hay veredicto: %d", ca, c.body, len(ents))
		}
	}
}

// CA-773: un gate del verify del Studio que crea el .hoom/local.yaml deja
// entorno_local en fail y el veredicto rojo.
func TestCA773_VerifyDelStudioConUnGateQueCreaElArchivo(t *testing.T) {
	const ca = "CA-773"
	dir := veeProyecto(t, "mkdir -p .hoom && echo 'en_el_host: [test]' > .hoom/local.yaml")
	code, body, v := veeVerify(t, dir)
	if code != http.StatusOK || v == nil {
		t.Fatalf("%s: 200 con el veredicto: %d %s", ca, code, body)
	}
	g := veeGateDe(v, "entorno_local")
	if g == nil || g.Status != verdict.StatusFail || !g.Required || g.OutputTail != hentorno.Ruta || v.Verdict != "red" {
		t.Fatalf("%s: entorno_local en fail y rojo: %s", ca, body)
	}
}
