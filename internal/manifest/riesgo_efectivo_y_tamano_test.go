// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// la seccion `rutas` de hoom.yaml (CA-776): las tres claves, `[]`, nula y
// ausente, Resolver sobre los valores por defecto y cada error con su texto
// exacto, en ParseRutas, en el UnmarshalYAML y en Load. Escritos desde el
// spec, las aclaraciones de la ronda 0 y las firmas publicas, sin leer la
// implementacion.
package manifest_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

const retyBase = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

func retyCargar(t *testing.T, body string) (*manifest.Manifest, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(retyBase+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest.Load(dir, nil)
}

func retyCargarSinBase(t *testing.T, body string) (*manifest.Manifest, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(retyBase+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest.LoadWithoutBaseCheck(dir, nil)
}

// retyDecodificar decodifica body con el UnmarshalYAML de Rutas, sin Load.
func retyDecodificar(body string) (*manifest.Rutas, error) {
	var doc struct {
		Rutas *manifest.Rutas `yaml:"rutas"`
	}
	err := yaml.Unmarshal([]byte(body), &doc)
	return doc.Rutas, err
}

// CA-776: las claves validas, en el orden de su error.
func TestCA776_RutasKeys(t *testing.T) {
	if !reflect.DeepEqual(manifest.RutasKeys, []string{"de_riesgo", "de_bajo_riesgo", "generadas"}) {
		t.Fatalf("CA-776: RutasKeys: %q", manifest.RutasKeys)
	}
}

// CA-776 y aclaracion 6: sin la seccion, o con `rutas: null`, no hay
// seccion; una clave nula o ausente queda nil; `[]` queda vacia y no nil; una
// lista queda tal cual.
func TestCA776_LaSeccionSeLeeTalCual(t *testing.T) {
	const ca = "CA-776"
	for _, body := range []string{"", "rutas: null\n", "rutas: ~\n", "rutas:\n"} {
		r, err := manifest.ParseRutas([]byte(retyBase + body))
		if err != nil || r != nil {
			t.Fatalf("%s: ParseRutas sin seccion (%q) es nil: %+v %v", ca, body, r, err)
		}
		m, err := retyCargar(t, body)
		if err != nil || m.Rutas != nil {
			t.Fatalf("%s: Load sin seccion (%q) deja Rutas nil: %+v %v", ca, body, m, err)
		}
	}
	if r, err := manifest.ParseRutas(nil); err != nil || r != nil {
		t.Fatalf("%s: ParseRutas(nil) es nil: %+v %v", ca, r, err)
	}
	ejemplo := "rutas:\n  de_riesgo: [\"app/Domain/Costeo\", \"**/*tenant*\"]\n  de_bajo_riesgo: [\"**/*.md\", \"lang\"]\n  generadas: [\"composer.lock\", \"vendor\"]\n"
	want := &manifest.Rutas{DeRiesgo: []string{"app/Domain/Costeo", "**/*tenant*"}, DeBajoRiesgo: []string{"**/*.md", "lang"},
		Generadas: []string{"composer.lock", "vendor"}}
	r, err := manifest.ParseRutas([]byte(retyBase + ejemplo))
	if err != nil || !reflect.DeepEqual(r, want) {
		t.Fatalf("%s: ParseRutas del ejemplo del spec: %+v %v", ca, r, err)
	}
	m, err := retyCargar(t, ejemplo)
	if err != nil || !reflect.DeepEqual(m.Rutas, want) {
		t.Fatalf("%s: Load del ejemplo del spec: %+v %v", ca, m, err)
	}
	r, err = manifest.ParseRutas([]byte("rutas:\n  de_riesgo: []\n  de_bajo_riesgo: null\n"))
	if err != nil || r == nil {
		t.Fatalf("%s: ParseRutas con [] y null: %+v %v", ca, r, err)
	}
	if r.DeRiesgo == nil || len(r.DeRiesgo) != 0 {
		t.Fatalf("%s: de_riesgo: [] es una lista vacia, no nil: %#v", ca, r.DeRiesgo)
	}
	if r.DeBajoRiesgo != nil || r.Generadas != nil {
		t.Fatalf("%s: una clave nula o ausente queda nil: %#v %#v", ca, r.DeBajoRiesgo, r.Generadas)
	}
	m, err = retyCargar(t, "rutas:\n  generadas: []\n  de_riesgo: ~\n")
	if err != nil || m.Rutas == nil || m.Rutas.Generadas == nil || len(m.Rutas.Generadas) != 0 || m.Rutas.DeRiesgo != nil {
		t.Fatalf("%s: Load con generadas: [] y de_riesgo: ~: %+v %v", ca, m, err)
	}
}

// CA-776: Resolver toma la del perfil para cada clave nil, y una lista (vacia
// incluida) la reemplaza entera; un r nil es def; las listas son copias.
func TestCA776_Resolver(t *testing.T) {
	const ca = "CA-776"
	def := riesgo.RutasBase()
	def.Generadas = []string{"go.sum", "vendor"}
	var nada *manifest.Rutas
	if got := nada.Resolver(def); !reflect.DeepEqual(got, def) {
		t.Fatalf("%s: un r nil es def: %+v", ca, got)
	}
	r := &manifest.Rutas{DeRiesgo: []string{}, DeBajoRiesgo: []string{"lang"}}
	got := r.Resolver(def)
	if len(got.DeRiesgo) != 0 || !reflect.DeepEqual(got.DeBajoRiesgo, []string{"lang"}) || !reflect.DeepEqual(got.Generadas, []string{"go.sum", "vendor"}) {
		t.Fatalf("%s: Resolver: %+v", ca, got)
	}
	got.DeBajoRiesgo[0] = "cambiado"
	got.Generadas[0] = "cambiado"
	if r.DeBajoRiesgo[0] != "lang" || def.Generadas[0] != "go.sum" {
		t.Fatalf("%s: las listas de Resolver son copias: r %+v def %+v", ca, r, def)
	}
	otro := nada.Resolver(def)
	otro.DeRiesgo[0] = "cambiado"
	if def.DeRiesgo[0] != "**/*auth*" {
		t.Fatalf("%s: con r nil, Resolver tambien copia: %q", ca, def.DeRiesgo[0])
	}
}

type retyClaves struct {
	Modo [3]int // 0 ausente/nil, 1 vacia, 2 una lista
}

func (retyClaves) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(retyClaves{Modo: [3]int{r.Intn(3), r.Intn(3), r.Intn(3)}})
}

// CA-776 (propiedad): cada clave de Resolver es la del perfil si la seccion
// no la da, y la de la seccion si la da.
func TestCA776_PropiedadResolver(t *testing.T) {
	const ca = "CA-776"
	def := riesgo.Rutas{DeRiesgo: []string{"**/*auth*"}, DeBajoRiesgo: []string{"**/*.md"}, Generadas: []string{"vendor"}}
	propia := [3][]string{{"app/Domain/Costeo"}, {"lang"}, {"composer.lock"}}
	prop := func(e retyClaves) bool {
		var listas [3][]string
		for i, modo := range e.Modo {
			switch modo {
			case 1:
				listas[i] = []string{}
			case 2:
				listas[i] = append([]string(nil), propia[i]...)
			}
		}
		got := (&manifest.Rutas{DeRiesgo: listas[0], DeBajoRiesgo: listas[1], Generadas: listas[2]}).Resolver(def)
		defs := [3][]string{def.DeRiesgo, def.DeBajoRiesgo, def.Generadas}
		gots := [3][]string{got.DeRiesgo, got.DeBajoRiesgo, got.Generadas}
		for i, modo := range e.Modo {
			switch modo {
			case 0:
				if !reflect.DeepEqual(gots[i], defs[i]) {
					return false
				}
			case 1:
				if len(gots[i]) != 0 {
					return false
				}
			case 2:
				if !reflect.DeepEqual(gots[i], propia[i]) {
					return false
				}
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}

// retyErrores son los tres errores de `rutas`, con su texto exacto (claves y
// patrones citados con %q).
var retyErrores = []struct {
	rutas, want string
}{
	{"rutas:\n  de_riesgos: []\n", `rutas: clave desconocida "de_riesgos" (validas: de_riesgo, de_bajo_riesgo, generadas)`},
	{"rutas:\n  De_Riesgo: []\n", `rutas: clave desconocida "De_Riesgo" (validas: de_riesgo, de_bajo_riesgo, generadas)`},
	{"rutas:\n  \"a b\": [x]\n", `rutas: clave desconocida "a b" (validas: de_riesgo, de_bajo_riesgo, generadas)`},
	{"rutas:\n  de_riesgo: \"**/*auth*\"\n", "rutas.de_riesgo: tiene que ser una lista de patrones"},
	{"rutas:\n  de_bajo_riesgo: {a: b}\n", "rutas.de_bajo_riesgo: tiene que ser una lista de patrones"},
	{"rutas:\n  generadas: vendor\n", "rutas.generadas: tiene que ser una lista de patrones"},
	{"rutas:\n  generadas: [vendor, null]\n", "rutas.generadas: tiene que ser una lista de patrones"},
	{"rutas:\n  generadas:\n    - vendor\n    -\n", "rutas.generadas: tiene que ser una lista de patrones"},
	{"rutas:\n  de_riesgo: [[a]]\n", "rutas.de_riesgo: tiene que ser una lista de patrones"},
	{"rutas:\n  de_riesgo: [{a: b}]\n", "rutas.de_riesgo: tiene que ser una lista de patrones"},
	{"rutas:\n  de_riesgo: [\"a//b\"]\n", `rutas.de_riesgo: patron invalido "a//b"`},
	{"rutas:\n  generadas: [\"[\"]\n", `rutas.generadas: patron invalido "["`},
	{"rutas:\n  de_bajo_riesgo: [\"**/*.md\", \"\"]\n", `rutas.de_bajo_riesgo: patron invalido ""`},
	{"rutas:\n  de_bajo_riesgo: [\"  \"]\n", `rutas.de_bajo_riesgo: patron invalido "  "`},
	{"rutas:\n  de_riesgo: [\"/app\"]\n", `rutas.de_riesgo: patron invalido "/app"`},
	{"rutas:\n  generadas: [\"vendor/\"]\n", `rutas.generadas: patron invalido "vendor/"`},
	{"rutas:\n  de_riesgo: ['a\"b[']\n", `rutas.de_riesgo: patron invalido "a\"b["`},
	{"rutas:\n  de_riesgo: ['x\\']\n", `rutas.de_riesgo: patron invalido "x\\"`},
}

// CA-776: cada error de `rutas` sale con su texto exacto en el
// UnmarshalYAML, y lo llevan ParseRutas, Load y LoadWithoutBaseCheck.
func TestCA776_LosErroresDeRutas(t *testing.T) {
	const ca = "CA-776"
	for _, k := range retyErrores {
		if r, err := retyDecodificar(k.rutas); err == nil || err.Error() != k.want {
			t.Fatalf("%s: el UnmarshalYAML de\n%s\nda exactamente %q, fue %v (%+v)", ca, k.rutas, k.want, err, r)
		}
		if _, err := manifest.ParseRutas([]byte(retyBase + k.rutas)); err == nil || !strings.Contains(err.Error(), k.want) {
			t.Fatalf("%s: ParseRutas de\n%s\nda %q, fue %v", ca, k.rutas, k.want, err)
		}
		if _, err := retyCargar(t, k.rutas); err == nil || !strings.Contains(err.Error(), k.want) {
			t.Fatalf("%s: Load de\n%s\nda %q, fue %v", ca, k.rutas, k.want, err)
		}
		if _, err := retyCargarSinBase(t, k.rutas); err == nil || !strings.Contains(err.Error(), k.want) {
			t.Fatalf("%s: LoadWithoutBaseCheck de\n%s\nda %q, fue %v", ca, k.rutas, k.want, err)
		}
	}
}

// CA-776 y aclaracion 7: un elemento que no es texto (un numero, un
// booleano) da `tiene que ser una lista de patrones`.
func TestCA776_UnElementoQueNoEsTexto(t *testing.T) {
	const ca = "CA-776"
	for _, k := range []struct{ rutas, want string }{
		{"rutas:\n  de_riesgo: [1]\n", "rutas.de_riesgo: tiene que ser una lista de patrones"},
		{"rutas:\n  generadas: [vendor, true]\n", "rutas.generadas: tiene que ser una lista de patrones"},
	} {
		if _, err := manifest.ParseRutas([]byte(retyBase + k.rutas)); err == nil || !strings.Contains(err.Error(), k.want) {
			t.Fatalf("%s: ParseRutas de\n%s\nda %q, fue %v", ca, k.rutas, k.want, err)
		}
		if _, err := retyCargar(t, k.rutas); err == nil || !strings.Contains(err.Error(), k.want) {
			t.Fatalf("%s: Load de\n%s\nda %q, fue %v", ca, k.rutas, k.want, err)
		}
	}
}

// CA-776 y aclaracion 6: una seccion que no es un mapa falla con un error
// que empieza con `rutas: ` (el resto no esta fijado).
func TestCA776_UnaSeccionQueNoEsMapa(t *testing.T) {
	const ca = "CA-776"
	for _, body := range []string{"rutas: vendor\n", "rutas: [de_riesgo]\n", "rutas: 3\n"} {
		if _, err := retyDecodificar(body); err == nil || !strings.HasPrefix(err.Error(), "rutas: ") {
			t.Fatalf("%s: %q: el error empieza con `rutas: `: %v", ca, body, err)
		}
		if _, err := manifest.ParseRutas([]byte(body)); err == nil {
			t.Fatalf("%s: ParseRutas de %q es un error", ca, body)
		}
		if _, err := retyCargar(t, body); err == nil {
			t.Fatalf("%s: Load de %q es un error", ca, body)
		}
	}
}
