// Tests adversariales del spec .hoom/specs/contencion-del-writer.md (CA-675):
// la seccion `encierro` de hoom.yaml. Opcional y estricta: sin la seccion
// no hay encierro, sin exigir vale no, y cada error con el texto del
// contrato, sin correr git, en Load, LoadWithoutBaseCheck y LoadRoles.
// Escrito desde el spec y el go doc de manifest.EncierroPolicy.
package manifest_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/manifest"
)

var cdwCargadores = []struct {
	nombre string
	cargar func(dir string) (*manifest.Manifest, error)
}{
	{"Load", func(dir string) (*manifest.Manifest, error) { return manifest.Load(dir, nil) }},
	{"LoadWithoutBaseCheck", func(dir string) (*manifest.Manifest, error) { return manifest.LoadWithoutBaseCheck(dir, nil) }},
	{"LoadRoles", manifest.LoadRoles},
}

const cdwBase = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n  e2e:\n    required: false\n    cmd: \"true\"\n"

func cdwDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(cdwBase+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// cdwPolitica carga body con los tres cargadores (sin git: PATH vacio) y
// exige que den la misma seccion.
func cdwPolitica(t *testing.T, body string) *manifest.EncierroPolicy {
	t.Helper()
	t.Setenv("PATH", "")
	var primera *manifest.EncierroPolicy
	for i, c := range cdwCargadores {
		m, err := c.cargar(cdwDir(t, body))
		if err != nil {
			t.Fatalf("CA-675: [%s] el hoom.yaml\n%s\ncarga (sin git): %v", c.nombre, body, err)
		}
		if i == 0 {
			primera = m.Encierro
			continue
		}
		if !reflect.DeepEqual(m.Encierro, primera) {
			t.Fatalf("CA-675: [%s] lee la misma seccion que Load: %+v vs %+v", c.nombre, m.Encierro, primera)
		}
	}
	return primera
}

// cdwInvalido exige que los tres cargadores rechacen body con
// `hoom.yaml invalido: <quiero>`, sin correr git.
func cdwInvalido(t *testing.T, body, quiero string) {
	t.Helper()
	t.Setenv("PATH", "")
	for _, c := range cdwCargadores {
		_, err := c.cargar(cdwDir(t, body))
		if err == nil || !strings.Contains(err.Error(), "hoom.yaml invalido: "+quiero) {
			t.Fatalf("CA-675: [%s] el hoom.yaml\n%s\nes invalido con `hoom.yaml invalido: %s`; fue %v", c.nombre, body, quiero, err)
		}
	}
}

// CA-675: la seccion con sus tres claves se lee tal cual (las rutas como
// las escribe el archivo, sin expandir ~/).
func TestCA675_LaSeccionConSusTresClaves(t *testing.T) {
	p := cdwPolitica(t, "encierro:\n  exigir: completo          # completo, parcial o no\n  escribibles:\n    - ~/.semgrep\n    - /opt/cache con espacio\n  gates_sin_encierro:\n    - e2e\n")
	want := &manifest.EncierroPolicy{Exigir: manifest.ExigirCompleto, Escribibles: []string{"~/.semgrep", "/opt/cache con espacio"}, GatesSinEncierro: []string{"e2e"}}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("CA-675: la seccion se lee tal cual: %+v; quiero %+v", p, want)
	}
	for _, e := range []string{"completo", "parcial", "no"} {
		if p := cdwPolitica(t, "encierro:\n  exigir: "+e+"\n"); p == nil || p.Exigir != e {
			t.Fatalf("CA-675: exigir %s se lee: %+v", e, p)
		}
	}
	if !reflect.DeepEqual(manifest.EncierroKeys, []string{"exigir", "escribibles", "gates_sin_encierro"}) {
		t.Fatalf("CA-675: EncierroKeys en el orden del error: %q", manifest.EncierroKeys)
	}
}

// CA-675: sin la seccion no hay encierro (nil); con la seccion sin exigir
// (o exigir nulo), exigir vale no.
func TestCA675_SinSeccionYSinExigir(t *testing.T) {
	if p := cdwPolitica(t, ""); p != nil {
		t.Fatalf("CA-675: sin la seccion, Encierro es nil: %+v", p)
	}
	for _, body := range []string{
		"encierro:\n  escribibles:\n    - ~/.semgrep\n",
		"encierro:\n  gates_sin_encierro: [e2e]\n",
		"encierro:\n  exigir:\n",
		"encierro:\n  exigir: ~\n",
		"encierro: {}\n",
	} {
		p := cdwPolitica(t, body)
		if p == nil || p.Exigir != manifest.ExigirNo {
			t.Fatalf("CA-675: con la seccion y sin exigir, exigir vale no:\n%s\n%+v", body, p)
		}
	}
	t.Setenv("PATH", "")
	m, err := manifest.LoadRoles(t.TempDir())
	if err != nil || m == nil || m.Encierro != nil {
		t.Fatalf("CA-675: un root sin hoom.yaml no declara nada: %+v %v", m, err)
	}
}

// CA-675: cada rechazo con el texto exacto del contrato.
func TestCA675_LosRechazosDelContrato(t *testing.T) {
	const relativa = " no es una ruta absoluta ni empieza con ~/"
	const todo = " es todo el disco o todo el directorio personal"
	for _, c := range []struct{ body, quiero string }{
		{"encierro:\n  exigir: no\n  otra: 1\n", `encierro: clave desconocida "otra" (validas: exigir, escribibles, gates_sin_encierro)`},
		{"encierro:\n  Exigir: completo\n", `encierro: clave desconocida "Exigir" (validas: exigir, escribibles, gates_sin_encierro)`},
		{"encierro:\n  gates_sin_encierro: [e2e]\n  escribible: [/opt]\n", `encierro: clave desconocida "escribible" (validas: exigir, escribibles, gates_sin_encierro)`},
		{"encierro:\n  exigir: total\n", "encierro: exigir tiene que ser completo, parcial o no"},
		{"encierro:\n  exigir: Completo\n", "encierro: exigir tiene que ser completo, parcial o no"},
		{"encierro:\n  exigir: \"\"\n", "encierro: exigir tiene que ser completo, parcial o no"},
		{"encierro:\n  exigir: true\n", "encierro: exigir tiene que ser completo, parcial o no"},
		{"encierro:\n  exigir: [completo]\n", "encierro: exigir tiene que ser completo, parcial o no"},
		{"encierro:\n  escribibles:\n    - .semgrep\n", `encierro: escribibles: ".semgrep"` + relativa},
		{"encierro:\n  escribibles:\n    - semgrep/cache\n", `encierro: escribibles: "semgrep/cache"` + relativa},
		{"encierro:\n  escribibles:\n    - ../fuera\n", `encierro: escribibles: "../fuera"` + relativa},
		{"encierro:\n  escribibles:\n    - ~semgrep\n", `encierro: escribibles: "~semgrep"` + relativa},
		{"encierro:\n  escribibles:\n    - \"\"\n", `encierro: escribibles: ""` + relativa},
		{"encierro:\n  escribibles:\n    - /opt/ok\n    - relativa\n", `encierro: escribibles: "relativa"` + relativa},
		{"encierro:\n  escribibles:\n    - /\n", `encierro: escribibles: "/"` + todo},
		{"encierro:\n  escribibles:\n    - ~/\n", `encierro: escribibles: "~/"` + todo},
		{"encierro:\n  escribibles:\n    - /opt/ok\n    - ~/\n", `encierro: escribibles: "~/"` + todo},
	} {
		cdwInvalido(t, c.body, c.quiero)
	}
	// `- ~` es nulo en YAML: cae en el rechazo de la ruta relativa o vacia
	t.Setenv("PATH", "")
	for _, cg := range cdwCargadores {
		_, err := cg.cargar(cdwDir(t, "encierro:\n  escribibles:\n    - ~\n"))
		if err == nil || !strings.Contains(err.Error(), "hoom.yaml invalido: encierro: escribibles: ") || !strings.HasSuffix(err.Error(), relativa) {
			t.Fatalf("CA-675: [%s] `- ~` (nulo) se rechaza con el texto de la ruta relativa o vacia: %v", cg.nombre, err)
		}
	}
}

// cdwRelativa genera una ruta que no es absoluta ni empieza con "~"
// (la de "~" sola no la fija la aclaracion 16).
type cdwRelativa string

func (cdwRelativa) Generate(r *rand.Rand, _ int) reflect.Value {
	alfa := []rune("abcxyz019._-/ ñ")
	n := 1 + r.Intn(12)
	var b strings.Builder
	for b.Len() == 0 || len([]rune(b.String())) < n {
		b.WriteRune(alfa[r.Intn(len(alfa))])
	}
	s := b.String()
	for strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || strings.TrimSpace(s) != s || s == "" {
		s = "x" + strings.TrimLeft(strings.TrimSpace(s), "/~")
	}
	return reflect.ValueOf(cdwRelativa(s))
}

// CA-675 (propiedad): toda ruta relativa en escribibles se rechaza con su
// texto, nombrando la ruta como la escribe el archivo.
func TestCA675_PropiedadTodaRutaRelativaSeRechaza(t *testing.T) {
	t.Setenv("PATH", "")
	f := func(r cdwRelativa) bool {
		body := "encierro:\n  escribibles:\n    - " + strconv.Quote(string(r)) + "\n"
		_, err := manifest.LoadRoles(cdwDir(t, body))
		want := "hoom.yaml invalido: encierro: escribibles: " + strconv.Quote(string(r)) + " no es una ruta absoluta ni empieza con ~/"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Logf("%q: %v", string(r), err)
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("CA-675: %v", err)
	}
}

// CA-675: lo que no se juzga al cargar: una ruta escribible dentro del
// checkout principal y un gate que no esta en los gates se juzgan al empezar
// el sobre (encierro.Validar), no aca.
func TestCA675_LoQueNoSeJuzgaAlCargar(t *testing.T) {
	dir := t.TempDir()
	body := "encierro:\n  escribibles:\n    - " + dir + "\n  gates_sin_encierro:\n    - docker\n"
	t.Setenv("PATH", "")
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(cdwBase+body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range cdwCargadores {
		m, err := c.cargar(dir)
		if err != nil || m.Encierro == nil || !reflect.DeepEqual(m.Encierro.GatesSinEncierro, []string{"docker"}) {
			t.Fatalf("CA-675: [%s] las dos reglas que necesitan el arbol se juzgan al empezar el sobre, no al cargar: %+v %v", c.nombre, m, err)
		}
	}
}
