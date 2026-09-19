// Tests adversariales del spec .hoom/specs/gate-findings-open.md
// (CA-248, CA-249): el bloque findings de hoom.yaml es opt-in, y un typo que
// apagaria en silencio un gate que bloquea hace fallar manifest.Load.
package manifest

import (
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

// foManifiesto escribe un hoom.yaml sin perfil, con un gate barato y el bloque
// findings tal cual se le pasa (vacio = sin bloque).
func foManifiesto(t *testing.T, findingsYAML string) string {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" + findingsYAML
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func foCargar(t *testing.T, findingsYAML string) (*Manifest, error) {
	t.Helper()
	return Load(foManifiesto(t, findingsYAML), nil)
}

// CA-248: sin findings.block_on el gate no existe: bloque ausente,
// findings: {} y block_on sin valor (null en YAML, en todas sus grafias).
func TestCA248_SinBlockOnElGateQuedaApagado(t *testing.T) {
	casos := map[string]string{
		"bloque ausente":     "",
		"findings: {}":       "findings: {}\n",
		"findings vacio":     "findings:\n",
		"block_on sin valor": "findings:\n  block_on:\n",
		"block_on: null":     "findings:\n  block_on: null\n",
		"block_on: ~":        "findings:\n  block_on: ~\n",
	}
	for nombre, yml := range casos {
		m, err := foCargar(t, yml)
		if err != nil {
			t.Fatalf("CA-248 (%s): no optar por el gate no es un hoom.yaml invalido: %v", nombre, err)
		}
		if got := m.FindingsBlockOn(); got != "" {
			t.Fatalf("CA-248 (%s): sin block_on el umbral debe ser \"\" (gate apagado), fue %q", nombre, got)
		}
	}
	// Control: el mismo manifiesto con block_on SI opta; sin esto el aserto de
	// arriba pasaria con un FindingsBlockOn que siempre devuelve "".
	m, err := foCargar(t, "findings:\n  block_on: high\n")
	if err != nil || m.FindingsBlockOn() != "high" {
		t.Fatalf("CA-248 (control): block_on high enciende el gate: err=%v", err)
	}
}

// CA-249: block_on acepta low|medium|high normalizado como 'hoom finding
// add' normaliza la severidad (minusculas y sin espacios): HIGH = high.
func TestCA249_BlockOnValidoSeNormaliza(t *testing.T) {
	casos := []struct{ yml, quiero string }{
		{"findings:\n  block_on: high\n", "high"},
		{"findings:\n  block_on: HIGH\n", "high"},
		{"findings:\n  block_on: Medium\n", "medium"},
		{"findings:\n  block_on: low\n", "low"},
		{"findings:\n  block_on: \" high \"\n", "high"},
		{"findings:\n  block_on: \"\\tLOW\\n\"\n", "low"},
		{"findings: {block_on: medium}\n", "medium"},
	}
	for _, c := range casos {
		m, err := foCargar(t, c.yml)
		if err != nil {
			t.Fatalf("CA-249: %q es un umbral valido y fue rechazado: %v", c.yml, err)
		}
		if got := m.FindingsBlockOn(); got != c.quiero {
			t.Fatalf("CA-249: %q debe quedar como %q, quedo %q", c.yml, c.quiero, got)
		}
	}
}

// CA-249: cualquier otro valor, incluido "", hace fallar manifest.Load con un
// error que cita el valor y los valores validos. Un typo no apaga el gate.
func TestCA249_BlockOnInvalidoFallaElLoad(t *testing.T) {
	for _, valor := range []string{"", "critical", "alto", "hgh", "highest", "hi gh", "none", "off", "3"} {
		yml := "findings:\n  block_on: " + strconv.Quote(valor) + "\n"
		m, err := foCargar(t, yml)
		if err == nil {
			t.Fatalf("CA-249: block_on %q debe hacer fallar manifest.Load (umbral=%q)", valor, m.FindingsBlockOn())
		}
		msg := err.Error()
		if !strings.Contains(msg, strconv.Quote(valor)) {
			t.Fatalf("CA-249: el error debe citar el valor %s: %v", strconv.Quote(valor), err)
		}
		if !strings.Contains(msg, "low|medium|high") {
			t.Fatalf("CA-249: el error debe nombrar los valores validos low|medium|high: %v", err)
		}
	}
}

// CA-249 (borde): solo espacios normaliza a "" y "" es invalido; un escalar
// que no es texto (numero, booleano, lista) tampoco es un umbral.
func TestCA249_BlockOnEnBlancoOTipoRaroFalla(t *testing.T) {
	for _, yml := range []string{
		"findings:\n  block_on: \"   \"\n",
		"findings:\n  block_on: 3\n",
		"findings:\n  block_on: true\n",
		"findings:\n  block_on: [high]\n",
		"findings:\n  block_on: {nivel: high}\n",
	} {
		if m, err := foCargar(t, yml); err == nil {
			t.Fatalf("CA-249: %q no es un umbral y debe fallar el Load (umbral=%q)", yml, m.FindingsBlockOn())
		}
	}
}

// CA-249: una clave desconocida dentro de findings (typo de block_on) hace
// fallar manifest.Load nombrando la clave.
func TestCA249_ClaveDesconocidaEnFindingsFalla(t *testing.T) {
	casos := map[string]string{
		"blockon":  "findings:\n  blockon: high\n",
		"block-on": "findings:\n  block-on: high\n",
		"blockOn":  "findings:\n  blockOn: high\n",
		"extra":    "findings:\n  block_on: high\n  extra: 1\n",
	}
	for clave, yml := range casos {
		_, err := foCargar(t, yml)
		if err == nil {
			t.Fatalf("CA-249: la clave desconocida %q dentro de findings debe fallar el Load", clave)
		}
		if !strings.Contains(err.Error(), clave) {
			t.Fatalf("CA-249: el error debe nombrar la clave %q: %v", clave, err)
		}
	}
}

// umbralHostil genera valores de block_on: los validos con mayusculas y
// espacios alrededor, y ruido que se les parece.
type umbralHostil string

func (umbralHostil) Generate(rnd *rand.Rand, size int) reflect.Value {
	bases := []string{"low", "medium", "high", "critical", "", "lo w", "hig", "mediumm", "ninguno", "HiGh"}
	s := bases[rnd.Intn(len(bases))]
	var b strings.Builder
	for _, r := range s {
		if rnd.Intn(2) == 0 {
			b.WriteString(strings.ToUpper(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	blancos := []string{"", " ", "  ", "\t"}
	out := blancos[rnd.Intn(len(blancos))] + b.String() + blancos[rnd.Intn(len(blancos))]
	return reflect.ValueOf(umbralHostil(out))
}

// CA-249 (propiedad): Load acepta un block_on si y solo si, normalizado
// (minusculas, sin espacios alrededor), es low|medium|high, y en ese caso
// FindingsBlockOn devuelve exactamente la forma normalizada.
func TestCA249_PropiedadSoloTresUmbrales(t *testing.T) {
	validos := map[string]bool{"low": true, "medium": true, "high": true}
	prop := func(u umbralHostil) bool {
		valor := string(u)
		norm := strings.ToLower(strings.TrimSpace(valor))
		m, err := foCargar(t, "findings:\n  block_on: "+strconv.Quote(valor)+"\n")
		if validos[norm] {
			return err == nil && m.FindingsBlockOn() == norm
		}
		return err != nil && strings.Contains(err.Error(), "low|medium|high")
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 60}); err != nil {
		t.Fatalf("CA-249: la normalizacion de block_on no es la del contrato: %v", err)
	}
}
