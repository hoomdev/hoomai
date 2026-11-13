// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md
// (CA-640): la seccion `autonomia` de hoom.yaml. Opcional y estricta: tres
// claves, 0, 0 y ventana_max sin declarar cuando falta la seccion o una
// clave, y un hoom.yaml invalido con el texto del contrato para una clave
// desconocida, cada valor fuera de rango y rondas sin findings.block_on.
// Escrito desde el spec y el go doc de manifest.AutonomiaPolicy.
package manifest_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/manifest"
)

const aemBase = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

const aemBlockOn = "findings:\n  block_on: high\n"

// aemCargadores son las tres formas de leer el hoom.yaml del proyecto: el
// contrato vale para todas.
var aemCargadores = []struct {
	nombre string
	cargar func(dir string) (*manifest.Manifest, error)
}{
	{"Load", func(dir string) (*manifest.Manifest, error) { return manifest.Load(dir, nil) }},
	{"LoadWithoutBaseCheck", func(dir string) (*manifest.Manifest, error) { return manifest.LoadWithoutBaseCheck(dir, nil) }},
	{"LoadRoles", manifest.LoadRoles},
}

func aemDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(aemBase+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// aemPolitica carga body con cada cargador y exige que todos den la misma
// politica.
func aemPolitica(t *testing.T, body string) (*manifest.AutonomiaPolicy, manifest.AutonomiaPolicy) {
	t.Helper()
	var decl *manifest.AutonomiaPolicy
	var def manifest.AutonomiaPolicy
	for i, c := range aemCargadores {
		m, err := c.cargar(aemDir(t, body))
		if err != nil {
			t.Fatalf("CA-640: [%s] el hoom.yaml\n%s\ncarga: %v", c.nombre, body, err)
		}
		if i == 0 {
			decl, def = m.Autonomia, m.AutonomiaOrDefault()
			continue
		}
		if !reflect.DeepEqual(m.Autonomia, decl) || !reflect.DeepEqual(m.AutonomiaOrDefault(), def) {
			t.Fatalf("CA-640: [%s] lee la misma politica que Load: %+v / %+v vs %+v / %+v", c.nombre, m.Autonomia, m.AutonomiaOrDefault(), decl, def)
		}
	}
	return decl, def
}

// aemInvalido exige que cada cargador rechace body como hoom.yaml invalido
// con el texto del contrato.
func aemInvalido(t *testing.T, body, quiero string) {
	t.Helper()
	for _, c := range aemCargadores {
		_, err := c.cargar(aemDir(t, body))
		if err == nil {
			t.Fatalf("CA-640: [%s] el hoom.yaml\n%s\nes invalido", c.nombre, body)
		}
		if !strings.Contains(err.Error(), "hoom.yaml invalido") || !strings.Contains(err.Error(), quiero) {
			t.Fatalf("CA-640: [%s] el error es `hoom.yaml invalido: ...` con\n  %q\nfue\n  %q", c.nombre, quiero, err.Error())
		}
	}
}

func aemVentana(v *int) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprint(*v)
}

// CA-640: la seccion con sus tres claves se lee tal cual, en Load,
// LoadWithoutBaseCheck y LoadRoles, y AutonomiaOrDefault la devuelve.
func TestCA640_LaSeccionConSusTresClaves(t *testing.T) {
	decl, def := aemPolitica(t, aemBlockOn+"autonomia:\n  reintentos_ante_rojo: 1   # 0 o 1\n  rondas_de_hallazgos: 2    # de 0 a 5\n  ventana_max: 80           # de 1 a 100\n")
	if decl == nil || decl.ReintentosAnteRojo != 1 || decl.RondasDeHallazgos != 2 || decl.VentanaMax == nil || *decl.VentanaMax != 80 {
		t.Fatalf("CA-640: la seccion declarada es {1, 2, 80}: %+v (ventana %s)", decl, aemVentana(decl.VentanaMax))
	}
	if def.ReintentosAnteRojo != 1 || def.RondasDeHallazgos != 2 || def.VentanaMax == nil || *def.VentanaMax != 80 {
		t.Fatalf("CA-640: AutonomiaOrDefault devuelve la seccion declarada: %+v", def)
	}
	// los bordes de cada rango
	for _, c := range []struct {
		body          string
		r, n, ventana int
	}{
		{"autonomia:\n  reintentos_ante_rojo: 0\n  rondas_de_hallazgos: 0\n  ventana_max: 1\n", 0, 0, 1},
		{aemBlockOn + "autonomia:\n  reintentos_ante_rojo: 1\n  rondas_de_hallazgos: 5\n  ventana_max: 100\n", 1, 5, 100},
	} {
		_, def := aemPolitica(t, c.body)
		if def.ReintentosAnteRojo != c.r || def.RondasDeHallazgos != c.n || def.VentanaMax == nil || *def.VentanaMax != c.ventana {
			t.Fatalf("CA-640: los bordes de los rangos son validos (%d, %d, %d): %+v", c.r, c.n, c.ventana, def)
		}
	}
	if !reflect.DeepEqual(manifest.AutonomiaKeys, []string{"reintentos_ante_rojo", "rondas_de_hallazgos", "ventana_max"}) {
		t.Fatalf("CA-640: las claves de autonomia son las tres del contrato, en orden: %v", manifest.AutonomiaKeys)
	}
}

// CA-640: sin la seccion, o sin una clave, los dos contadores valen 0 y
// ventana_max no esta declarada; un manifiesto nil no declara nada.
func TestCA640_SinSeccionOSinClaveValenCero(t *testing.T) {
	decl, def := aemPolitica(t, "")
	if decl != nil {
		t.Fatalf("CA-640: sin la seccion, Autonomia es nil: %+v", decl)
	}
	if def.ReintentosAnteRojo != 0 || def.RondasDeHallazgos != 0 || def.VentanaMax != nil {
		t.Fatalf("CA-640: sin la seccion, 0, 0 y ventana_max sin declarar: %+v", def)
	}
	casos := map[string]manifest.AutonomiaPolicy{
		"autonomia: {}\n":                                     {},
		"autonomia:\n  reintentos_ante_rojo: 1\n":             {ReintentosAnteRojo: 1},
		"autonomia:\n  ventana_max: 55\n":                     {VentanaMax: aemEnt(55)},
		aemBlockOn + "autonomia:\n  rondas_de_hallazgos: 3\n": {RondasDeHallazgos: 3},
	}
	for body, quiero := range casos {
		_, def := aemPolitica(t, body)
		if def.ReintentosAnteRojo != quiero.ReintentosAnteRojo || def.RondasDeHallazgos != quiero.RondasDeHallazgos ||
			aemVentana(def.VentanaMax) != aemVentana(quiero.VentanaMax) {
			t.Fatalf("CA-640: [%q] la clave que falta vale 0 (o sin declarar): quiero %+v (ventana %s), fue %+v (ventana %s)",
				body, quiero, aemVentana(quiero.VentanaMax), def, aemVentana(def.VentanaMax))
		}
	}
	var nada *manifest.Manifest
	if def := nada.AutonomiaOrDefault(); def.ReintentosAnteRojo != 0 || def.RondasDeHallazgos != 0 || def.VentanaMax != nil {
		t.Fatalf("CA-640: un manifiesto nil no declara nada: %+v", def)
	}
	// un hoom.yaml sin seccion en una raiz sin hoom.yaml: LoadRoles no declara nada
	m, err := manifest.LoadRoles(t.TempDir())
	if err != nil || m.AutonomiaOrDefault().RondasDeHallazgos != 0 || m.AutonomiaOrDefault().VentanaMax != nil {
		t.Fatalf("CA-640: una raiz sin hoom.yaml no declara autonomia: %+v %v", m, err)
	}
}

func aemEnt(n int) *int { return &n }

// CA-640: una clave desconocida, cada valor fuera de rango y las rondas sin
// findings.block_on son un hoom.yaml invalido con el texto exacto del
// contrato, en los tres cargadores.
func TestCA640_RechazaConElTextoDelContrato(t *testing.T) {
	desconocida := func(k string) string {
		return fmt.Sprintf("autonomia: clave desconocida %q (validas: reintentos_ante_rojo, rondas_de_hallazgos, ventana_max)", k)
	}
	const (
		reintentos = "autonomia: reintentos_ante_rojo tiene que ser 0 o 1"
		rondas     = "autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5"
		ventana    = "autonomia: ventana_max tiene que ser un entero de 1 a 100"
		sinUmbral  = "autonomia: rondas_de_hallazgos necesita findings.block_on"
	)
	casos := []struct{ body, quiero string }{
		{"autonomia:\n  reintentos: 1\n", desconocida("reintentos")},
		{"autonomia:\n  reintentos_ante_rojo: 1\n  Ventana_Max: 80\n", desconocida("Ventana_Max")},
		{aemBlockOn + "autonomia:\n  rondas_de_hallazgos: 1\n  nivel: 3\n", desconocida("nivel")},
		{"autonomia:\n  reintentos_ante_rojo: 2\n", reintentos},
		{"autonomia:\n  reintentos_ante_rojo: -1\n", reintentos},
		{"autonomia:\n  reintentos_ante_rojo: si\n", reintentos},
		{"autonomia:\n  reintentos_ante_rojo: 0.5\n", reintentos},
		{aemBlockOn + "autonomia:\n  rondas_de_hallazgos: 6\n", rondas},
		{aemBlockOn + "autonomia:\n  rondas_de_hallazgos: -1\n", rondas},
		{aemBlockOn + "autonomia:\n  rondas_de_hallazgos: 2.5\n", rondas},
		{aemBlockOn + "autonomia:\n  rondas_de_hallazgos: dos\n", rondas},
		{"autonomia:\n  ventana_max: 0\n", ventana},
		{"autonomia:\n  ventana_max: 101\n", ventana},
		{"autonomia:\n  ventana_max: -80\n", ventana},
		{"autonomia:\n  ventana_max: 80.5\n", ventana},
		{"autonomia:\n  ventana_max: 80%\n", ventana},
		{"autonomia:\n  rondas_de_hallazgos: 1\n", sinUmbral},
		{"findings: {}\nautonomia:\n  rondas_de_hallazgos: 5\n", sinUmbral},
		{"findings:\n  block_on:\nautonomia:\n  rondas_de_hallazgos: 2\n", sinUmbral},
	}
	for _, c := range casos {
		aemInvalido(t, c.body, c.quiero)
	}
	// sin rondas no hace falta el umbral
	if _, def := aemPolitica(t, "autonomia:\n  rondas_de_hallazgos: 0\n  reintentos_ante_rojo: 1\n"); def.ReintentosAnteRojo != 1 {
		t.Fatalf("CA-640: rondas_de_hallazgos: 0 no necesita findings.block_on: %+v", def)
	}
}

// CA-640: para cualquier entero, cada clave carga si y solo si esta en su
// rango, y si no, falla con el texto de esa clave.
func TestCA640_RangosParaCualquierEntero(t *testing.T) {
	claves := []struct {
		clave     string
		min, max  int
		texto     string
		leer      func(manifest.AutonomiaPolicy) int
		conUmbral bool
	}{
		{"reintentos_ante_rojo", 0, 1, "autonomia: reintentos_ante_rojo tiene que ser 0 o 1",
			func(p manifest.AutonomiaPolicy) int { return p.ReintentosAnteRojo }, false},
		{"rondas_de_hallazgos", 0, 5, "autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5",
			func(p manifest.AutonomiaPolicy) int { return p.RondasDeHallazgos }, true},
		{"ventana_max", 1, 100, "autonomia: ventana_max tiene que ser un entero de 1 a 100",
			func(p manifest.AutonomiaPolicy) int {
				if p.VentanaMax == nil {
					return -999999
				}
				return *p.VentanaMax
			}, false},
	}
	f := func(n int16) bool {
		v := int(n) % 140
		for _, c := range claves {
			body := fmt.Sprintf("autonomia:\n  %s: %d\n", c.clave, v)
			if c.conUmbral {
				body = aemBlockOn + body
			}
			m, err := manifest.Load(aemDir(t, body), nil)
			valido := v >= c.min && v <= c.max
			if valido && (err != nil || c.leer(m.AutonomiaOrDefault()) != v) {
				t.Errorf("CA-640: %s: %d esta en rango y carga tal cual: %v", c.clave, v, err)
				return false
			}
			if !valido && (err == nil || !strings.Contains(err.Error(), c.texto) || !strings.Contains(err.Error(), "hoom.yaml invalido")) {
				t.Errorf("CA-640: %s: %d esta fuera de rango: %v", c.clave, v, err)
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 120, Rand: rand.New(rand.NewSource(640))}); err != nil {
		t.Fatal(err)
	}
}
