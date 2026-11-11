// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// hoom.yaml (CA-632): findings.max_pendientes, un entero mayor o igual a 0;
// sin la clave no hay tope; cualquier otro valor invalida el manifiesto con
// el texto del contrato, y la clave desconocida nombra las dos validas.
// Escritos desde el spec y las firmas publicas, sin leer la implementacion.
package manifest

import (
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

const (
	crMaxInvalido = "findings: max_pendientes tiene que ser un entero mayor o igual a 0"
	crValidas     = "(validas: block_on, max_pendientes)"
)

// CA-632: acepta un entero mayor o igual a 0, con o sin block_on.
func TestCA632_MaxPendientesValido(t *testing.T) {
	for _, c := range []struct {
		yml string
		n   int
	}{
		{"findings:\n  block_on: high\n  max_pendientes: 0\n", 0},
		{"findings:\n  block_on: high\n  max_pendientes: 3\n", 3},
		{"findings:\n  max_pendientes: 12\n", 12},
		{"findings: {block_on: medium, max_pendientes: 1}\n", 1},
	} {
		m, err := foCargar(t, c.yml)
		if err != nil {
			t.Fatalf("CA-632: %q es valido: %v", c.yml, err)
		}
		n, ok := m.FindingsMaxPendientes()
		if !ok || n != c.n {
			t.Fatalf("CA-632: %q da (%d, true), fue (%d, %v)", c.yml, c.n, n, ok)
		}
		if m.Findings == nil || m.Findings.MaxPendientes == nil || *m.Findings.MaxPendientes != c.n {
			t.Fatalf("CA-632: FindingsPolicy.MaxPendientes: %+v", m.Findings)
		}
		p, err := ParseFindings([]byte(foBody(c.yml)))
		if err != nil || p == nil || p.MaxPendientes == nil || *p.MaxPendientes != c.n {
			t.Fatalf("CA-632: ParseFindings lee lo mismo: %+v %v", p, err)
		}
	}
}

func foBody(findingsYAML string) string {
	return "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" + findingsYAML
}

// CA-632: sin la clave no hay tope.
func TestCA632_SinLaClaveNoHayTope(t *testing.T) {
	for _, yml := range []string{"", "findings:\n  block_on: high\n", "findings: {}\n"} {
		m, err := foCargar(t, yml)
		if err != nil {
			t.Fatalf("CA-632: %q es valido: %v", yml, err)
		}
		if n, ok := m.FindingsMaxPendientes(); ok || n != 0 {
			t.Fatalf("CA-632: %q no declara tope: (%d, %v)", yml, n, ok)
		}
	}
	if p, err := ParseFindings([]byte(foBody(""))); err != nil || p != nil {
		t.Fatalf("CA-632: sin seccion ParseFindings es nil: %+v %v", p, err)
	}
}

// CA-632: un negativo o un valor que no es entero invalida el manifiesto con
// el texto del contrato, en Load y en ParseFindings.
func TestCA632_MaxPendientesInvalido(t *testing.T) {
	for _, v := range []string{"-1", "-100", "1.5", "tres", "\"3\"", "true", "[1]", "{a: 1}", "1e3", "99999999999999999999999"} {
		yml := "findings:\n  block_on: high\n  max_pendientes: " + v + "\n"
		_, err := foCargar(t, yml)
		if err == nil || !strings.Contains(err.Error(), crMaxInvalido) {
			t.Fatalf("CA-632: max_pendientes %s es invalido con %q: %v", v, crMaxInvalido, err)
		}
		if _, err := ParseFindings([]byte(foBody(yml))); err == nil || !strings.Contains(err.Error(), crMaxInvalido) {
			t.Fatalf("CA-632: ParseFindings rechaza max_pendientes %s con el texto del contrato: %v", v, err)
		}
	}
}

// CA-632: la clave desconocida nombra las dos validas.
func TestCA632_ClaveDesconocidaNombraLasDos(t *testing.T) {
	for _, clave := range []string{"max_pendiente", "maxPendientes", "tope", "blockon"} {
		yml := "findings:\n  block_on: high\n  " + clave + ": 1\n"
		_, err := foCargar(t, yml)
		want := `findings: clave desconocida "` + clave + `" ` + crValidas
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("CA-632: %q falla con %q: %v", clave, want, err)
		}
	}
}

type crEntero int

func (crEntero) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(crEntero(r.Intn(2000) - 1000))
}

// CA-632 (propiedad): para todo entero n, max_pendientes n es valido si y
// solo si n >= 0, y entonces FindingsMaxPendientes da (n, true).
func TestCA632_PropiedadEnteros(t *testing.T) {
	prop := func(n crEntero) bool {
		m, err := foCargar(t, "findings:\n  block_on: high\n  max_pendientes: "+strconv.Itoa(int(n))+"\n")
		if n < 0 {
			return err != nil && strings.Contains(err.Error(), crMaxInvalido)
		}
		if err != nil {
			return false
		}
		got, ok := m.FindingsMaxPendientes()
		return ok && got == int(n)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-632: %v", err)
	}
}
