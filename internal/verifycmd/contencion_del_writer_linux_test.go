//go:build linux

// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre
// verifycmd en Linux con encierro: con Sobre y la seccion, cada gate corre
// encerrado con la vista del rol y la cache verify (CA-683, CA-682, CA-694,
// CA-679), con espacios de usuario anidados; el de gates_sin_encierro y el
// verify sin Sobre corren como hoy, y el material se fotografia igual
// (CA-687). Sin bwrap fallan (aclaracion 1). Lo que el gate cuenta lo dice
// por su salida (OutputTail), saliendo con 1.
package verifycmd

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func cdwlEncierro(t *testing.T, ca string) {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Fatalf("%s: bwrap no esta en el PATH: los tests del encierro fallan sin el (%v)", ca, err)
	}
	if info := encierro.Nivel(true, nil); info.Nivel == encierro.NivelNinguno {
		t.Fatalf("%s: este sistema no tiene encierro (%s)", ca, info.Motivo)
	}
}

func cdwlToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	tok := "cdwvtok" + hex.EncodeToString(b)
	t.Cleanup(func() {
		for _, pid := range cdwlVivos(tok) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return tok
}

func cdwlVivos(tok string) []int {
	var out []int
	es, _ := os.ReadDir("/proc")
	for _, e := range es {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err == nil && strings.Contains(string(raw), tok) {
			out = append(out, pid)
		}
	}
	return out
}

// cdwlSalida es la salida del gate test del veredicto.
func cdwlSalida(t *testing.T, v *verdict.Verdict) string {
	t.Helper()
	for _, g := range v.Gates {
		if g.Name == "test" {
			return g.OutputTail
		}
	}
	t.Fatalf("el veredicto trae el gate test: %+v", v.Gates)
	return ""
}

func cdwlSecretos(t *testing.T, home string) {
	t.Helper()
	for rel, body := range map[string]string{".ssh/id_ed25519": "clave", ".gnupg/pubring.kbx": "anillo",
		".config/gh/hosts.yml": "token", ".aws/credentials": "secreto", ".docker/config.json": `{"auths":{}}`,
		"no-es-secreto/leeme.txt": "se lee"} {
		cdwEscribir(t, filepath.Join(home, rel), body)
	}
}

// el gate que cuenta lo que ve (una linea por dato) y sale con 1.
const cdwlGateQueCuenta = "printf x > $HOME/gate-escribio 2>/dev/null; " +
	"echo tmpdir=$TMPDIR; echo xdg=$XDG_CACHE_HOME; " +
	"for d in .ssh .gnupg .config/gh .aws; do echo $d=$(ls -A $HOME/$d 2>/dev/null | wc -l); done; " +
	"echo docker=$(cat $HOME/.docker/config.json 2>/dev/null | wc -c); " +
	"echo leeme=$(cat $HOME/no-es-secreto/leeme.txt 2>/dev/null); " +
	"if bwrap --ro-bind / / --dev /dev true 2>/dev/null; then echo anidado=si; else echo anidado=no; fi; " +
	"exit 1"

func cdwlValor(salida, clave string) (string, bool) {
	for _, l := range strings.Split(salida, "\n") {
		if strings.HasPrefix(l, clave+"=") {
			return strings.TrimPrefix(l, clave+"="), true
		}
	}
	return "", false
}

// CA-683, CA-682 y CA-694: con Sobre y la seccion, el gate corre encerrado:
// no deja nada en $HOME, recibe TMPDIR y XDG_CACHE_HOME en la cache verify,
// ve vacios los secretos y lee el resto del $HOME, y puede anidar espacios
// de usuario.
func TestCA683_ConSobreElGateCorreEncerrado(t *testing.T) {
	const ca = "CA-683"
	cdwlEncierro(t, ca)
	root, _, m := cdwProyecto(t, cdwlGateQueCuenta, "encierro:\n  exigir: no\n")
	home := os.Getenv("HOME")
	cdwlSecretos(t, home)
	v, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)})
	if err != nil {
		t.Fatal(err)
	}
	salida := cdwlSalida(t, v)
	if _, err := os.Stat(filepath.Join(home, "gate-escribio")); err == nil {
		t.Fatalf("%s: el gate encerrado no deja nada en $HOME:\n%s", ca, salida)
	}
	cache := encierro.Cache(root, cdwTarea, encierro.QuienVerify)
	for _, k := range []string{"tmpdir", "xdg"} {
		val, _ := cdwlValor(salida, k)
		if val == "" || !(val == cache || strings.HasPrefix(val, cache+"/")) {
			t.Errorf("CA-682: el gate recibe %s dentro de la cache verify %s: %q\n%s", k, cache, val, salida)
		}
	}
	for _, k := range []string{".ssh", ".gnupg", ".config/gh", ".aws", "docker"} {
		if val, _ := cdwlValor(salida, k); val != "0" {
			t.Errorf("CA-694: el gate encerrado ve vacio %s: %q\n%s", k, val, salida)
		}
	}
	if val, _ := cdwlValor(salida, "leeme"); val != "se lee" {
		t.Errorf("CA-694: el gate encerrado lee el resto del $HOME: %q\n%s", val, salida)
	}
	if val, _ := cdwlValor(salida, "anidado"); val != "si" {
		t.Errorf("%s: el gate encerrado puede anidar espacios de usuario (bwrap adentro): %q\n%s", ca, val, salida)
	}
}

// CA-683: el gate de gates_sin_encierro, y el verify sin Sobre aunque haya
// seccion, corren como hoy: escriben en $HOME y leen los secretos.
func TestCA683_SinEncierroElGateCorreComoHoy(t *testing.T) {
	const ca = "CA-683"
	cdwlEncierro(t, ca)
	for _, c := range []struct {
		nombre, extra string
		sobre         bool
	}{
		{"gates_sin_encierro", "encierro:\n  exigir: no\n  gates_sin_encierro:\n    - test\n", true},
		{"verify sin Sobre", "encierro:\n  exigir: completo\n", false},
	} {
		root, _, m := cdwProyecto(t, cdwlGateQueCuenta, c.extra)
		home := os.Getenv("HOME")
		cdwlSecretos(t, home)
		opt := Options{Full: true}
		if c.sobre {
			opt.Sobre = cdwSobre(root)
		}
		v, _, err := Run(m, opt)
		if err != nil {
			t.Fatal(err)
		}
		salida := cdwlSalida(t, v)
		if _, err := os.Stat(filepath.Join(home, "gate-escribio")); err != nil {
			t.Fatalf("%s: %s: el gate corre como hoy y escribe en $HOME:\n%s", ca, c.nombre, salida)
		}
		if val, _ := cdwlValor(salida, ".ssh"); val != "1" {
			t.Fatalf("%s: %s: el gate corre como hoy y lee ~/.ssh: %q\n%s", ca, c.nombre, val, salida)
		}
	}
}

// CA-683 y CA-679: un gate encerrado que deja un proceso con setsid no lo
// deja vivo cuando termina el verify.
func TestCA683_ElGateEncerradoNoDejaProcesosVivos(t *testing.T) {
	const ca = "CA-683"
	cdwlEncierro(t, ca)
	tok := cdwlToken(t)
	gate := "setsid sh -c 'while :; do sleep 1; done' " + tok + " </dev/null >/dev/null 2>&1 & sleep 0.3; true"
	root, _, m := cdwProyecto(t, gate, "encierro:\n  exigir: no\n")
	if _, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)}); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(3 * time.Second)
	for len(cdwlVivos(tok)) > 0 {
		if time.Now().After(limite) {
			t.Fatalf("%s: el proceso que dejo el gate encerrado no queda vivo: %v", ca, cdwlVivos(tok))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// CA-687 y CA-681 con la seccion: el gate encerrado no puede cambiar el
// info/attributes que existe (no hay material_de_git), y el de
// gates_sin_encierro si lo crea, con el gate en fail.
func TestCA687_ConLaSeccionElMaterialSeFotografiaIgual(t *testing.T) {
	const ca = "CA-687"
	cdwlEncierro(t, ca)
	gate := "c=$(git rev-parse --git-common-dir); mkdir -p $c/info 2>/dev/null; printf x > $c/info/attributes 2>/dev/null; true"
	for _, sinEncierro := range []bool{false, true} {
		extra := "encierro:\n  exigir: no\n"
		if sinEncierro {
			extra += "  gates_sin_encierro:\n    - test\n"
		}
		root, wt, m := cdwProyecto(t, gate, extra)
		ruta := filepath.Join(cdwComun(t, wt), "info", "attributes")
		if !sinEncierro {
			cdwEscribir(t, ruta, "*.go diff\n")
		}
		v, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)})
		if err != nil {
			t.Fatal(err)
		}
		g := cdwGateMaterial(v)
		if sinEncierro {
			if g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, ruta) || v.Verdict != "red" {
				t.Fatalf("%s: el gate de gates_sin_encierro que cambia el material deja material_de_git en fail: %+v %s", ca, g, v.Verdict)
			}
			continue
		}
		if raw, _ := os.ReadFile(ruta); string(raw) != "*.go diff\n" || g != nil {
			t.Fatalf("%s: el gate encerrado no puede cambiar %s, que existe: %q %+v", ca, ruta, raw, g)
		}
	}
}
