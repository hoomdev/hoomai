// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre el
// cerrar-hallazgos del piloto, como consumidor externo (proyectos git
// reales, PATH minimo, ningun CLI de IA real, un `ddev` falso):
//
//   - CA-765: el verify de cerrar-hallazgos corre los gates con el
//     .hoom/local.yaml del principal: su veredicto lleva entorno y el gate
//     corre con el prefijo en el arbol de la tarea;
//   - CA-773: un gate de ese verify que crea el .hoom/local.yaml del
//     principal deja el veredicto rojo con entorno_local, y el piloto se
//     detiene con su texto de veredicto rojo, que lo nombra.
//
// Reusa los helpers de autonomia-entre-firmas (apPATH, apr1Ronda, apRun...).
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package pilotocmd_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// veeDdev instala en el bin del PATH minimo un `ddev` falso que anota su
// directorio y sus argumentos (separados por NUL) en una carpeta fuera del
// proyecto y corre `sh -c` con el ultimo, con VEE_DENTRO=1.
func veeDdev(t *testing.T, f *apFalsos) string {
	t.Helper()
	log := t.TempDir()
	q := "'" + strings.ReplaceAll(log, "'", `'\''`) + "'"
	script := "#!/bin/sh\n" +
		"log=" + q + "\n" +
		"{ printf '%s\\000' \"$(pwd -P)\"; for a do printf '%s\\000' \"$a\"; done; } > \"$log/.tmp.$$\"\n" +
		"mv \"$log/.tmp.$$\" \"$log/llamada.$$\"\n" +
		"for ultimo do :; done\n" +
		"VEE_DENTRO=1\nexport VEE_DENTRO\n" +
		"exec sh -c \"$ultimo\"\n"
	if err := os.WriteFile(filepath.Join(f.bin, "ddev"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
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

func veeSaltar(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("el ddev falso es un script de sh")
	}
	ignora := false
	for _, l := range strings.Split(hoomfs.GitignoreBody(), "\n") {
		ignora = ignora || strings.TrimSpace(l) == "local.yaml"
	}
	if !ignora {
		// apProyecto escribe hoomfs.GitignoreBody(): sin local.yaml (CA-770)
		// el .hoom/local.yaml del principal seria ErrNoIgnorado.
		t.Fatal("CA-770: hoomfs.GitignoreBody incluye local.yaml (aclaracion 15)")
	}
}

// veeUnicoNuevo es el unico veredicto nuevo de la tarjeta.
func veeUnicoNuevo(t *testing.T, ca string, fx apRondaFix, s apSalida) (string, verdict.Verdict) {
	t.Helper()
	nuevos := apNuevos(t, fx.wt, fx.veredictos)
	if len(nuevos) != 1 {
		t.Fatalf("%s: cerrar-hallazgos deja un veredicto nuevo: %d\n%s", ca, len(nuevos), s)
	}
	for id, v := range nuevos {
		return id, v
	}
	return "", verdict.Verdict{}
}

// CA-765: el verify de cerrar-hallazgos usa el .hoom/local.yaml del
// principal: el veredicto nuevo lleva la plantilla y el sha256, el gate test
// corre con el prefijo, en el arbol de la tarea y con su {rel}.
func TestCA765_CerrarHallazgosCorreConElPrefijo(t *testing.T) {
	const ca = "CA-765"
	veeSaltar(t)
	f := apPATH(t)
	log := veeDdev(t, f)
	const gate = "echo dentro=${VEE_DENTRO:-no}"
	fx := apr1Ronda(t, gate)
	tpl := "ddev exec -d /var/www/html/{rel} sh -c {cmd}"
	body := "ejecutar: \"" + tpl + "\"\n"
	apEscribir(t, fx.root, ".hoom/local.yaml", body)
	s := apRun(t, ca, fx.root, fx.slug)
	_, v := veeUnicoNuevo(t, ca, fx, s)
	sum := sha256.Sum256([]byte(body))
	if v.Entorno == nil || v.Entorno.Ejecutar != tpl || v.Entorno.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("%s: el veredicto de cerrar-hallazgos lleva entorno: %+v\n%s", ca, v.Entorno, s)
	}
	var g *verdict.GateResult
	for i := range v.Gates {
		if v.Gates[i].Name == "test" {
			g = &v.Gates[i]
		}
		if v.Gates[i].Entorno == "" {
			t.Fatalf("%s: con ejecutar todos los gates llevan entorno: %+v", ca, v.Gates[i])
		}
	}
	if g == nil || g.Entorno != verdict.EntornoPrefijo || g.Command != gate || !strings.Contains(g.OutputTail, "dentro=1") {
		t.Fatalf("%s: el gate test corre con el prefijo: %+v", ca, g)
	}
	ll := veeLlamadas(t, log)
	if len(ll) != 1 || ll[0][0] != fx.wt || len(ll[0]) < 4 || ll[0][3] != "/var/www/html/.hoom/worktrees/"+fx.slug {
		t.Fatalf("%s: el prefijo corre una vez, en %s, con {rel} .hoom/worktrees/%s: %q", ca, fx.wt, fx.slug, ll)
	}
	f.ninguna(t, ca, "cerrar-hallazgos")
}

// CA-773: un gate del verify de cerrar-hallazgos que crea el
// .hoom/local.yaml del principal deja entorno_local en fail; el veredicto
// es rojo y el piloto se detiene con su texto de rojo, que nombra
// entorno_local, exit 1, sin guardar.
func TestCA773_CerrarHallazgosConUnGateQueCreaElArchivo(t *testing.T) {
	const ca = "CA-773"
	veeSaltar(t)
	f := apPATH(t)
	fx := apr1Ronda(t, "c=$(git rev-parse --path-format=absolute --git-common-dir); echo x >> $c/../.hoom/local.yaml; true")
	head := apHead(t, fx.wt)
	s := apRun(t, ca, fx.root, fx.slug)
	if _, err := os.Stat(filepath.Join(fx.root, ".hoom", "local.yaml")); err != nil {
		t.Fatalf("%s: fixture: el gate creo el .hoom/local.yaml del principal: %v\n%s", ca, err, s)
	}
	id, v := veeUnicoNuevo(t, ca, fx, s)
	var g *verdict.GateResult
	for i := range v.Gates {
		if v.Gates[i].Name == "entorno_local" {
			g = &v.Gates[i]
		}
	}
	if g == nil || g.Status != verdict.StatusFail || !g.Required || g.OutputTail != ".hoom/local.yaml" || v.Verdict == "green" {
		t.Fatalf("%s: el veredicto nuevo %s trae entorno_local en fail y es rojo: %+v", ca, id, v.Gates)
	}
	ls := apSoloLineasDelPiloto(t, ca, s)
	pre := apDetiene + "despues de cerrar los hallazgos de la ronda 1, el veredicto " + id + " sigue rojo en "
	u := apUltima(ls)
	if s.code != 1 || !strings.HasPrefix(u, pre) || !strings.Contains(strings.TrimPrefix(u, pre), "entorno_local") {
		t.Fatalf("%s: se detiene con %q...entorno_local..., exit 1:\n%s", ca, pre, s)
	}
	for _, l := range ls {
		if apGuarda.MatchString(l) {
			t.Fatalf("%s: se detiene antes de guardar: %q\n%s", ca, l, s)
		}
	}
	if h := apHead(t, fx.wt); h != head {
		if msg := apGit(t, fx.wt, "log", "-1", "--format=%s"); msg == apGuardaMs+fx.slug {
			t.Fatalf("%s: no guarda despues de un veredicto con entorno_local: HEAD %s -> %s", ca, head, h)
		}
	}
	f.ninguna(t, ca, "cerrar-hallazgos")
}
