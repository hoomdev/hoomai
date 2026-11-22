// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md sobre
// hoom.yaml: la clave findings.max_rondas (CA-747) y el tope que sale de
// ella o, sin ella, de autonomia.rondas_de_hallazgos (CA-748). Escrito desde
// el spec, ACLARACIONES.md (11: LoadRoles compara solo con un max_rondas
// valido) y el go doc de manifest.FindingsPolicy.MaxRondas y
// Manifest.FindingsMaxRondas. Reusa aemBase, aemDir y aemCargadores
// (autonomia_entre_firmas_test.go, mismo paquete).
package manifest_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/manifest"
)

const (
	cldErrRango   = "findings: max_rondas tiene que ser un entero de 1 a 5"
	cldInvalido   = "hoom.yaml invalido"
	cldBlockOnYML = "findings:\n  block_on: high\n"
)

// cldCargadoresCompletos son los dos cargadores que leen el archivo entero
// (con findings): los que dan el error de rango (aclaracion 11).
var cldCargadoresCompletos = []struct {
	nombre string
	cargar func(dir string) (*manifest.Manifest, error)
}{
	{"Load", func(dir string) (*manifest.Manifest, error) { return manifest.Load(dir, nil) }},
	{"LoadWithoutBaseCheck", func(dir string) (*manifest.Manifest, error) { return manifest.LoadWithoutBaseCheck(dir, nil) }},
}

func cldCruzado(a, m int) string {
	return fmt.Sprintf("autonomia: rondas_de_hallazgos (%d) pasa de findings.max_rondas (%d)", a, m)
}

func cldDesconocida(k string) string {
	return `findings: clave desconocida "` + k + `" (validas: block_on, max_pendientes, max_rondas)`
}

func cldConMaxRondas(v string) string {
	return "findings:\n  block_on: high\n  max_rondas: " + v + "\n"
}

func cldAutonomia(a int) string {
	return fmt.Sprintf("autonomia:\n  rondas_de_hallazgos: %d\n", a)
}

// cldValido carga body con los dos cargadores completos y exige que los dos
// lo acepten; devuelve lo que leyo Load.
func cldValido(t *testing.T, ca, body string) *manifest.Manifest {
	t.Helper()
	var primero *manifest.Manifest
	for _, c := range cldCargadoresCompletos {
		m, err := c.cargar(aemDir(t, body))
		if err != nil {
			t.Fatalf("%s: [%s] el hoom.yaml\n%s\nes valido, fue: %v", ca, c.nombre, body, err)
		}
		if primero == nil {
			primero = m
		}
	}
	return primero
}

// cldRechaza exige que cada cargador de cs rechace body como `hoom.yaml
// invalido: ...` con quiero.
func cldRechaza(t *testing.T, ca, body, quiero string, cs []struct {
	nombre string
	cargar func(dir string) (*manifest.Manifest, error)
}) {
	t.Helper()
	for _, c := range cs {
		_, err := c.cargar(aemDir(t, body))
		if err == nil {
			t.Fatalf("%s: [%s] el hoom.yaml\n%s\nes invalido", ca, c.nombre, body)
		}
		if !strings.Contains(err.Error(), cldInvalido) || !strings.Contains(err.Error(), quiero) {
			t.Fatalf("%s: [%s] el error es `hoom.yaml invalido: ...` con\n  %q\nfue\n  %q", ca, c.nombre, quiero, err.Error())
		}
	}
}

// CA-747: findings.max_rondas acepta un entero de 1 a 5 en Load,
// LoadWithoutBaseCheck y ParseFindings, se lee tal cual en
// FindingsPolicy.MaxRondas y es el tope de FindingsMaxRondas. LoadRoles
// tampoco lo rechaza.
func TestCA747_MaxRondasDeUnoACinco(t *testing.T) {
	for n := 1; n <= 5; n++ {
		body := cldConMaxRondas(fmt.Sprint(n))
		for _, c := range cldCargadoresCompletos {
			m, err := c.cargar(aemDir(t, body))
			if err != nil {
				t.Fatalf("CA-747: [%s] max_rondas: %d es valido: %v", c.nombre, n, err)
			}
			if m.Findings == nil || m.Findings.MaxRondas == nil || *m.Findings.MaxRondas != n {
				t.Fatalf("CA-747: [%s] FindingsPolicy.MaxRondas es %d: %+v", c.nombre, n, m.Findings)
			}
			if got := m.FindingsMaxRondas(); got != n {
				t.Fatalf("CA-747: [%s] FindingsMaxRondas con max_rondas: %d es %d, fue %d", c.nombre, n, n, got)
			}
		}
		if _, err := manifest.LoadRoles(aemDir(t, body)); err != nil {
			t.Fatalf("CA-747: LoadRoles acepta max_rondas: %d: %v", n, err)
		}
		p, err := manifest.ParseFindings([]byte(aemBase + body))
		if err != nil || p == nil || p.MaxRondas == nil || *p.MaxRondas != n {
			t.Fatalf("CA-747: ParseFindings lee max_rondas: %d: %+v %v", n, p, err)
		}
	}
}

// CA-747: max_rondas no necesita block_on (ninguna regla del spec lo pide),
// y convive con max_pendientes.
func TestCA747_MaxRondasSinBlockOnYConMaxPendientes(t *testing.T) {
	m := cldValido(t, "CA-747", "findings:\n  max_rondas: 3\n")
	if m.FindingsMaxRondas() != 3 || m.FindingsBlockOn() != "" {
		t.Fatalf("CA-747: max_rondas sin block_on: tope 3 y gate apagado: %d %q", m.FindingsMaxRondas(), m.FindingsBlockOn())
	}
	m = cldValido(t, "CA-747", "findings:\n  block_on: medium\n  max_pendientes: 4\n  max_rondas: 5\n")
	if n, ok := m.FindingsMaxPendientes(); !ok || n != 4 || m.FindingsMaxRondas() != 5 || m.FindingsBlockOn() != "medium" {
		t.Fatalf("CA-747: las tres claves se leen juntas: pendientes (%d, %v), rondas %d, block_on %q",
			n, ok, m.FindingsMaxRondas(), m.FindingsBlockOn())
	}
}

// CA-747: 0, 6, un negativo y cualquier valor no entero (2.5, "dos", "3"
// entre comillas, true, una lista, un mapa) son el error de rango, en Load,
// LoadWithoutBaseCheck y ParseFindings.
func TestCA747_MaxRondasFueraDeRangoONoEntero(t *testing.T) {
	for _, v := range []string{"0", "6", "-1", "7", "100", "2.5", "1.5", "dos", `"3"`, "'2'", "true", "false", "[1]", "{n: 1}"} {
		body := cldConMaxRondas(v)
		cldRechaza(t, "CA-747 (max_rondas: "+v+")", body, cldErrRango, cldCargadoresCompletos)
		_, err := manifest.ParseFindings([]byte(aemBase + body))
		if err == nil || !strings.Contains(err.Error(), cldErrRango) {
			t.Fatalf("CA-747: ParseFindings rechaza max_rondas: %s con %q, fue %v", v, cldErrRango, err)
		}
	}
}

// CA-747: el error de rango es exactamente el texto del contrato (sin otro
// texto pegado despues del mensaje de la seccion).
func TestCA747_ElErrorDeRangoEsElDelContrato(t *testing.T) {
	_, err := manifest.Load(aemDir(t, cldConMaxRondas("0")), nil)
	if err == nil || !strings.HasSuffix(strings.TrimSpace(err.Error()), cldErrRango) {
		t.Fatalf("CA-747: el error termina en %q, fue %v", cldErrRango, err)
	}
}

// CA-747: max_rondas sin valor (null en todas sus grafias) es como no
// declararlo: MaxRondas nil y el tope por defecto.
func TestCA747_MaxRondasNuloEsSinClave(t *testing.T) {
	for _, v := range []string{"", "null", "~", "Null"} {
		m := cldValido(t, "CA-747", cldConMaxRondas(v))
		if m.Findings == nil || m.Findings.MaxRondas != nil {
			t.Fatalf("CA-747: max_rondas: %q es sin clave: %+v", v, m.Findings)
		}
		if m.FindingsMaxRondas() != manifest.DefaultMaxRondas {
			t.Fatalf("CA-747: max_rondas: %q da el tope por defecto, fue %d", v, m.FindingsMaxRondas())
		}
		p, err := manifest.ParseFindings([]byte(aemBase + cldConMaxRondas(v)))
		if err != nil || p == nil || p.MaxRondas != nil {
			t.Fatalf("CA-747: ParseFindings con max_rondas: %q es sin clave: %+v %v", v, p, err)
		}
	}
}

// CA-747: una clave desconocida en findings nombra las tres validas, en
// Load, LoadWithoutBaseCheck y ParseFindings. Las variantes de max_rondas mal
// escritas son las que un tope en silencio no puede tragarse.
func TestCA747_ClaveDesconocidaNombraLasTres(t *testing.T) {
	for _, k := range []string{"maxrondas", "max_ronda", "max-rondas", "rondas", "MaxRondas", "blockon"} {
		body := "findings:\n  block_on: high\n  " + k + ": 2\n"
		cldRechaza(t, "CA-747 (clave "+k+")", body, cldDesconocida(k), cldCargadoresCompletos)
		_, err := manifest.ParseFindings([]byte(aemBase + body))
		if err == nil || !strings.Contains(err.Error(), cldDesconocida(k)) {
			t.Fatalf("CA-747: ParseFindings nombra las tres claves validas para %q: %v", k, err)
		}
		if strings.Contains(err.Error(), "(validas: block_on, max_pendientes)") {
			t.Fatalf("CA-747: el texto viejo (dos claves) ya no vale: %v", err)
		}
	}
}

// CA-748: sin findings.max_rondas el tope es manifest.DefaultMaxRondas, que
// es 2; un manifiesto nil tampoco declara nada.
func TestCA748_SinMaxRondasElTopeEsDos(t *testing.T) {
	if manifest.DefaultMaxRondas != 2 {
		t.Fatalf("CA-748: el tope por defecto es 2, fue %d", manifest.DefaultMaxRondas)
	}
	for _, body := range []string{"", cldBlockOnYML, "findings: {}\n", "findings:\n  block_on: high\n  max_pendientes: 0\n"} {
		m := cldValido(t, "CA-748", body)
		if got := m.FindingsMaxRondas(); got != 2 {
			t.Fatalf("CA-748: sin max_rondas (%q) el tope es 2, fue %d", body, got)
		}
		r, err := manifest.LoadRoles(aemDir(t, body))
		if err != nil || r.FindingsMaxRondas() != 2 {
			t.Fatalf("CA-748: LoadRoles sin max_rondas (%q): tope 2: %v", body, err)
		}
	}
	var nada *manifest.Manifest
	if got := nada.FindingsMaxRondas(); got != 2 {
		t.Fatalf("CA-748: un manifiesto nil no declara nada: tope 2, fue %d", got)
	}
}

// CA-748 (caso limite): sin findings.max_rondas, un
// autonomia.rondas_de_hallazgos mayor que 2 sube el tope (3 da 3, 5 da 5);
// uno de 0, 1 o 2 lo deja en 2. Lo mismo por LoadRoles, que lee autonomia.
func TestCA748_AutonomiaMayorSubeElTope(t *testing.T) {
	for a := 0; a <= 5; a++ {
		body := cldBlockOnYML + cldAutonomia(a)
		quiero := 2
		if a > 2 {
			quiero = a
		}
		m := cldValido(t, "CA-748", body)
		if got := m.FindingsMaxRondas(); got != quiero {
			t.Fatalf("CA-748: autonomia.rondas_de_hallazgos: %d sin max_rondas: tope %d, fue %d", a, quiero, got)
		}
		r, err := manifest.LoadRoles(aemDir(t, body))
		if err != nil {
			t.Fatalf("CA-748: LoadRoles acepta rondas_de_hallazgos: %d: %v", a, err)
		}
		if got := r.FindingsMaxRondas(); got != quiero {
			t.Fatalf("CA-748: LoadRoles con rondas_de_hallazgos: %d: tope %d, fue %d", a, quiero, got)
		}
	}
	// el orden de las secciones en el archivo no cambia nada
	m := cldValido(t, "CA-748", cldAutonomia(3)+cldBlockOnYML)
	if m.FindingsMaxRondas() != 3 {
		t.Fatalf("CA-748: autonomia antes que findings: tope 3, fue %d", m.FindingsMaxRondas())
	}
}

// CA-748: un findings.max_rondas declarado manda, tambien por debajo de 2 y
// por encima de autonomia; igual a autonomia es valido.
func TestCA748_ElDeclaradoMandaSobreElDefecto(t *testing.T) {
	casos := []struct {
		a, m int
	}{{0, 1}, {1, 1}, {0, 5}, {3, 4}, {3, 3}, {5, 5}, {2, 2}, {1, 3}}
	for _, c := range casos {
		body := cldConMaxRondas(fmt.Sprint(c.m)) + cldAutonomia(c.a)
		m := cldValido(t, "CA-748", body)
		if got := m.FindingsMaxRondas(); got != c.m {
			t.Fatalf("CA-748: max_rondas %d con rondas_de_hallazgos %d: el tope es el declarado, fue %d", c.m, c.a, got)
		}
		if _, err := manifest.LoadRoles(aemDir(t, body)); err != nil {
			t.Fatalf("CA-748: LoadRoles acepta max_rondas %d con rondas_de_hallazgos %d: %v", c.m, c.a, err)
		}
	}
}

// CA-748: autonomia.rondas_de_hallazgos mayor que un max_rondas declarado es
// el error cruzado del contrato, en Load, LoadWithoutBaseCheck y LoadRoles,
// con las secciones en cualquier orden.
func TestCA748_AutonomiaPasaDeMaxRondasEsError(t *testing.T) {
	for _, c := range []struct{ a, m int }{{3, 2}, {5, 4}, {2, 1}, {5, 1}, {4, 3}} {
		for _, body := range []string{
			cldConMaxRondas(fmt.Sprint(c.m)) + cldAutonomia(c.a),
			cldAutonomia(c.a) + cldConMaxRondas(fmt.Sprint(c.m)),
		} {
			cldRechaza(t, fmt.Sprintf("CA-748 (%d > %d)", c.a, c.m), body, cldCruzado(c.a, c.m), aemCargadores)
		}
	}
}

// CA-748 y aclaracion 11: con un max_rondas invalido, LoadRoles nunca da el
// error cruzado (compara solo contra un entero valido), y Load da el de
// rango, no el cruzado.
func TestCA748_UnMaxRondasInvalidoNoSeCruza(t *testing.T) {
	for _, c := range []struct {
		v string
		a int
	}{{"0", 1}, {"0", 3}, {"dos", 3}, {"-2", 1}, {"2.5", 3}, {"true", 1}} {
		body := cldConMaxRondas(c.v) + cldAutonomia(c.a)
		if _, err := manifest.LoadRoles(aemDir(t, body)); err != nil && strings.Contains(err.Error(), "pasa de findings.max_rondas") {
			t.Fatalf("CA-748: LoadRoles no cruza rondas_de_hallazgos %d con un max_rondas invalido (%s): %v", c.a, c.v, err)
		}
		for _, l := range cldCargadoresCompletos {
			_, err := l.cargar(aemDir(t, body))
			if err == nil || !strings.Contains(err.Error(), cldErrRango) || strings.Contains(err.Error(), "pasa de findings.max_rondas") {
				t.Fatalf("CA-748: [%s] max_rondas %s con rondas_de_hallazgos %d es el error de rango, no el cruzado: %v", l.nombre, c.v, c.a, err)
			}
		}
	}
}

// CA-748 (propiedad): para todo a de 0 a 5 y m de 1 a 5 declarado, Load
// acepta si y solo si a <= m, con tope m; si no, el error cruzado con esos
// dos numeros. Sin max_rondas el tope es max(2, a).
func TestCA748_PropiedadTopeYCruce(t *testing.T) {
	prop := func(ra, rm uint8, declarar bool) bool {
		a, m := int(ra%6), int(rm%5)+1
		body := cldBlockOnYML + cldAutonomia(a)
		if declarar {
			body = cldConMaxRondas(fmt.Sprint(m)) + cldAutonomia(a)
		}
		man, err := manifest.Load(aemDir(t, body), nil)
		if !declarar {
			quiero := 2
			if a > 2 {
				quiero = a
			}
			return err == nil && man.FindingsMaxRondas() == quiero
		}
		if a <= m {
			return err == nil && man.FindingsMaxRondas() == m
		}
		return err != nil && strings.Contains(err.Error(), cldInvalido) && strings.Contains(err.Error(), cldCruzado(a, m))
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 60}); err != nil {
		t.Fatalf("CA-748: %v", err)
	}
}
