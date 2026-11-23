// Regresiones de los hallazgos de la review completa de verify-en-el-entorno
// (6e455e) sobre verifycmd.Run, escritas por el writer en la ronda a mano:
//
//   - ef18d4, ab20c5 y dfb560 (CA-773): la foto del .hoom/local.yaml del
//     principal cubre tambien los gates propios de hoom que corren comandos:
//     los [verifica:] de spec_trace y las metricas del ratchet. Un comando de
//     cualquiera de los dos que crea, cambia o borra el archivo deja el gate
//     entorno_local en fail y el veredicto rojo, aunque salga 0.
package verifycmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// vrhTrampa escribe en el .hoom/local.yaml del principal (el directorio del
// comando) una plantilla que se traga el exit de todos los gates.
const vrhTrampa = `printf 'ejecutar: "true {cmd}"\n' > .hoom/local.yaml`

// vrhSpec escribe y aprueba en root un spec cuyo unico criterio verifica con
// el comando verifica.
func vrhSpec(t *testing.T, root, verifica string) string {
	t.Helper()
	dir := filepath.Join(root, ".hoom", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "item.md")
	body := "# Spec\n## Objetivo\nx\n## No-goals\nx\n## Contratos\nx\n## Casos limite\nx\n" +
		"## Criterios de aceptacion\n- CA-1: x. [verifica: " + verifica + "]\n## Decisiones\nx\n## Riesgos\nx\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := approval.Approve(root, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// vrhSoloEntornoLocal exige que el unico gate que no paso sea entorno_local:
// sin el, el veredicto habria sido verde.
func vrhSoloEntornoLocal(t *testing.T, caso string, v *verdict.Verdict) {
	t.Helper()
	for _, g := range v.Gates {
		if g.Name != GateEntornoLocal && g.Status != verdict.StatusPass {
			t.Fatalf("%s: fixture: todos los demas gates pasan: %+v", caso, g)
		}
	}
}

// ef18d4, ab20c5, dfb560: un [verifica:] de spec_trace que crea, cambia o
// borra el .hoom/local.yaml del principal y sale 0 deja entorno_local en
// fail y el veredicto rojo.
func TestHallazgoEf18d4_SpecTraceQueTocaElArchivoDejaEntornoLocal(t *testing.T) {
	veeSaltarSinSh(t)
	for _, c := range []struct {
		nombre, antes, verifica, entorno string
	}{
		{"lo crea", "", vrhTrampa, ""},
		{"lo cambia, con ejecutar", "ejecutar: \"sh -c {cmd}\"\n", vrhTrampa, verdict.EntornoHost},
		{"lo borra", "en_el_host: [test]\n", "rm .hoom/local.yaml", ""},
	} {
		caso := "spec_trace " + c.nombre
		root, _, m := veeProyecto(t, veeGate("test", "true"), "", false)
		if c.antes != "" {
			veeLocal(t, root, c.antes)
		}
		spec := vrhSpec(t, root, c.verifica)
		v, _ := veeRun(t, caso, m, Options{Full: true, Spec: spec})
		if g := gateNamed(v, "spec_trace"); g == nil || g.Status != verdict.StatusPass {
			t.Fatalf("%s: fixture: el [verifica:] corre y sale 0: %+v", caso, g)
		}
		vrhSoloEntornoLocal(t, caso, v)
		veeExigeEntornoLocal(t, "CA-773", caso, v, root, c.entorno)
	}
}

// ef18d4, ab20c5, dfb560: una metrica del ratchet que crea, cambia o borra el
// .hoom/local.yaml del principal e imprime un numero valido deja
// entorno_local en fail y el veredicto rojo.
func TestHallazgoEf18d4_RatchetQueTocaElArchivoDejaEntornoLocal(t *testing.T) {
	veeSaltarSinSh(t)
	for _, c := range []struct {
		nombre, antes, metrica, entorno string
	}{
		{"lo crea", "", vrhTrampa + "; echo 90", ""},
		{"lo cambia, con ejecutar", "ejecutar: \"sh -c {cmd}\"\n", vrhTrampa + "; echo 90", verdict.EntornoHost},
		{"lo borra", "en_el_host: [test]\n", "rm .hoom/local.yaml; echo 90", ""},
	} {
		caso := "ratchet " + c.nombre
		root, _, m := veeProyecto(t, veeGate("test", "true"), "", false)
		if c.antes != "" {
			veeLocal(t, root, c.antes)
		}
		base := 70.0
		writeRatchet(t, root, c.metrica, &base)
		v, _ := veeRun(t, caso, m, Options{Full: true})
		if g := gateNamed(v, "ratchet"); g == nil || g.Status != verdict.StatusPass {
			t.Fatalf("%s: fixture: la metrica corre y su numero vale: %+v", caso, g)
		}
		vrhSoloEntornoLocal(t, caso, v)
		veeExigeEntornoLocal(t, "CA-773", caso, v, root, c.entorno)
	}
}
