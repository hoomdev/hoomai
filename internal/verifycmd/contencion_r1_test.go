// Ronda 1 del test-writer de contencion-del-writer: regresiones de la review
// de codex sobre verifycmd, en cualquier sistema. git-ajeno alrededor de los
// gates (id 318ca0; aclaracion 20): con Sobre, un gate que crea, mueve o borra
// una ref bajo refs/replace/ o reescribe un objeto suelto que existia deja el
// gate git_ajeno en fail, con la ruta, y el veredicto rojo, sin la seccion
// encierro y con el gate en gates_sin_encierro; uno que solo crea objetos no
// es git-ajeno; sin Sobre, como hoy.
package verifycmd

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// r1Sha es el id (sha1) del blob data.
func r1Sha(data string) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(data)) + "\x00" + data))
	return hex.EncodeToString(h.Sum(nil))
}

func r1GateDe(v *verdict.Verdict, nombre string) *verdict.GateResult {
	for i := range v.Gates {
		if v.Gates[i].Name == nombre {
			return &v.Gates[i]
		}
	}
	return nil
}

func r1Tiene(salida, ruta string) bool {
	for _, l := range strings.Split(strings.TrimSpace(salida), "\n") {
		if strings.TrimSpace(l) == ruta {
			return true
		}
	}
	return false
}

// r1Objeto escribe un objeto suelto nuevo y devuelve su ruta en el
// directorio comun.
func r1Objeto(t *testing.T, wt, comun string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "objeto")
	cdwEscribir(t, f, "r1 un objeto que ya existia "+strconv.FormatInt(time.Now().UnixNano(), 10)+"\n")
	sha := cdwGit(t, wt, "hash-object", "-w", f)
	p := filepath.Join(comun, "objects", sha[:2], sha[2:])
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture: el objeto suelto %s existe: %v", p, err)
	}
	return p
}

// r1ConGate arma cdwProyecto con un gate de relleno y lo cambia, en el
// hoom.yaml del principal y del arbol (commiteados), por el que devuelve
// arma, que ya puede usar el directorio comun. Devuelve el manifiesto del
// arbol recargado y la ruta que arma dice que el gate cambia.
func r1ConGate(t *testing.T, extra string, arma func(t *testing.T, wt, comun string) (gate, ruta string)) (root, wt string, m *manifest.Manifest, ruta string) {
	t.Helper()
	root, wt, _ = cdwProyecto(t, "true", extra)
	gate, ruta := arma(t, wt, cdwComun(t, wt))
	for _, d := range []string{root, wt} {
		p := filepath.Join(d, manifest.FileName)
		raw, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(raw), "cmd: \"true\"") {
			t.Fatalf("fixture: el hoom.yaml de %s tiene el gate de relleno: %v", d, err)
		}
		cdwEscribir(t, p, strings.Replace(string(raw), "cmd: \"true\"", "cmd: \""+gate+"\"", 1))
		cdwGit(t, d, "add", manifest.FileName)
		cdwGit(t, d, "commit", "-q", "-m", "el gate de la ronda 1")
	}
	m, err := manifest.Load(wt, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root, wt, m, ruta
}

// r1CasosGitAjeno son gates que rompen git-ajeno, con la ruta que nombran.
var r1CasosGitAjeno = []struct {
	nombre string
	arma   func(t *testing.T, wt, comun string) (gate, ruta string)
}{
	{"crea una ref de reemplazo", func(t *testing.T, wt, comun string) (string, string) {
		const a, b = "r1-verify-a", "r1-verify-b"
		return "x=$(printf " + a + " | git hash-object -w --stdin); y=$(printf " + b + " | git hash-object -w --stdin); git replace -f $x $y; true",
			filepath.Join(comun, "refs", "replace", r1Sha(a))
	}},
	{"borra una ref de reemplazo que existia", func(t *testing.T, wt, comun string) (string, string) {
		a := r1Objeto(t, wt, comun)
		b := r1Objeto(t, wt, comun)
		sa := filepath.Base(filepath.Dir(a)) + filepath.Base(a)
		sb := filepath.Base(filepath.Dir(b)) + filepath.Base(b)
		cdwGit(t, wt, "replace", "-f", sa, sb)
		return "git replace -d " + sa + "; true", filepath.Join(comun, "refs", "replace", sa)
	}},
	{"reescribe un objeto suelto que existia", func(t *testing.T, wt, comun string) (string, string) {
		p := r1Objeto(t, wt, comun)
		return "chmod u+w " + p + "; printf basura >> " + p + "; true", p
	}},
}

// CA-685 y CA-687 (id 318ca0; aclaracion 20): con Sobre, el gate que rompe
// git-ajeno deja git_ajeno requerido, en fail, de scope full, con la ruta en
// su salida (una por linea), el veredicto rojo y LatestComplete sin verde;
// sin la seccion encierro y con el gate en gates_sin_encierro.
func TestR1_318ca0_CA685_CA687_ConSobreUnGateQueRompeGitAjenoDejaRojo(t *testing.T) {
	const ca = "CA-685"
	for _, extra := range []string{"", "encierro:\n  exigir: no\n  gates_sin_encierro:\n    - test\n"} {
		for _, c := range r1CasosGitAjeno {
			caso := c.nombre
			if extra != "" {
				caso += " (gates_sin_encierro)"
			}
			root, wt, m, ruta := r1ConGate(t, extra, c.arma)
			v, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)})
			if err != nil {
				t.Fatalf("%s: %s: verify con Sobre corre: %v", ca, caso, err)
			}
			g := r1GateDe(v, GateGitAjeno)
			if g == nil || !g.Required || g.Status != verdict.StatusFail || g.Scope != "full" || !r1Tiene(g.OutputTail, ruta) {
				t.Fatalf("%s: %s: el veredicto lleva git_ajeno requerido, en fail, scope full, con %s en su salida: %+v (gates %+v)", ca, caso, ruta, g, v.Gates)
			}
			if v.Verdict != "red" {
				t.Fatalf("CA-687: %s: el veredicto queda rojo: %s", caso, v.Verdict)
			}
			if cdwUltimoCompletoVerde(t, wt) {
				t.Fatalf("CA-687: %s: verdict.LatestComplete no lo da como verde", caso)
			}
		}
	}
	if GateGitAjeno != "git_ajeno" {
		t.Fatalf("%s: el gate se llama git_ajeno: %q", ca, GateGitAjeno)
	}
}

// CA-685: un gate que solo crea objetos nuevos no es git-ajeno (el nombre
// de un objeto sale de su contenido): con Sobre no hay git_ajeno y el
// veredicto es verde.
func TestR1_318ca0_CA685_UnGateQueSoloCreaObjetosNoEsGitAjeno(t *testing.T) {
	const ca = "CA-685"
	for _, extra := range []string{"", "encierro:\n  exigir: no\n  gates_sin_encierro:\n    - test\n"} {
		root, _, m, _ := r1ConGate(t, extra, func(t *testing.T, wt, comun string) (string, string) {
			r1Objeto(t, wt, comun) // un objeto que existia y el gate no toca
			n := strconv.FormatInt(time.Now().UnixNano(), 10)
			return "printf r1-nuevo-" + n + " | git hash-object -w --stdin >/dev/null; printf r1-otro-" + n + " | git hash-object -w --stdin >/dev/null; true", ""
		})
		v, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)})
		if err != nil {
			t.Fatal(err)
		}
		if g := r1GateDe(v, GateGitAjeno); g != nil || v.Verdict != "green" {
			t.Fatalf("%s: extra %q: crear objetos nuevos no es git-ajeno: %+v %s (gates %+v)", ca, extra, g, v.Verdict, v.Gates)
		}
	}
}

// CA-683: sin Sobre (hoom verify de la persona) el gate que crea una ref de
// reemplazo no agrega git_ajeno: el veredicto es el de hoy.
func TestR1_318ca0_CA683_SinSobreNoHayGitAjeno(t *testing.T) {
	_, _, m, _ := r1ConGate(t, "", r1CasosGitAjeno[0].arma)
	v, _, err := Run(m, Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if g := r1GateDe(v, GateGitAjeno); g != nil || v.Verdict != "green" {
		t.Fatalf("CA-683: sin Sobre no hay git_ajeno y el veredicto es el de hoy: %+v %s", g, v.Verdict)
	}
}
