// models: en hoom.yaml (spec tablero-con-paneles, "models en hoom.yaml";
// CA-573): acepta models.<provider> con entradas name y description para los
// providers a los que hoom les pasa el modelo (claude, codex); un provider
// desconocido, gemini u opencode, una entrada sin name, un nombre que empieza
// con "-" o una clave desconocida son un error de carga con el texto del
// spec. Escrito desde el spec y el go doc de manifest.ModelCatalog.
package manifest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
)

const mmBase = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

func mmCargar(t *testing.T, extra string) (*manifest.Manifest, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(mmBase+extra), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest.Load(dir, profiles.Resolve)
}

// CA-573: las entradas validas se cargan en Manifest.Models, por provider,
// en el orden del archivo, con name y description recortados; sin
// description, vacia.
func TestCA573_ModelsValidos(t *testing.T) {
	m, err := mmCargar(t, "models:\n  codex:\n    - name: gpt-7-sol\n      description: el nuevo de todos los dias\n    - name: \"  gpt-8  \"\n"+
		"  claude:\n    - name: mi-claude\n      description: \"  con espacios  \"\n")
	if err != nil {
		t.Fatalf("CA-573: models valido carga: %v", err)
	}
	quiere := manifest.ModelCatalog{
		"codex":  {{Name: "gpt-7-sol", Description: "el nuevo de todos los dias"}, {Name: "gpt-8"}},
		"claude": {{Name: "mi-claude", Description: "con espacios"}},
	}
	if !reflect.DeepEqual(m.Models, quiere) {
		t.Fatalf("CA-573: Manifest.Models:\n got %#v\nwant %#v", m.Models, quiere)
	}
	for _, vacio := range []string{"", "models:\n", "models: {}\n", "models: null\n"} {
		m, err := mmCargar(t, vacio)
		if err != nil || len(m.Models) != 0 {
			t.Fatalf("CA-573: un models vacio o nulo no declara nada (%q): %v %#v", vacio, err, m)
		}
	}
	if !reflect.DeepEqual(manifest.ModelKeys, []string{"name", "description"}) {
		t.Fatalf("CA-573: ModelKeys son name y description: %q", manifest.ModelKeys)
	}
}

// CA-573: cada error de models: es un hoom.yaml invalido con el texto del
// spec, el primero en el orden del archivo (dentro de una entrada, sus claves
// antes que su name).
func TestCA573_ModelsInvalidos(t *testing.T) {
	casos := []struct {
		nombre, yaml, texto string
	}{
		{"provider desconocido", "models:\n  foo:\n    - name: x\n", `models: provider desconocido "foo" (validos: claude, codex)`},
		{"gemini", "models:\n  gemini:\n    - name: x\n", "models.gemini: hoom no le pasa el modelo a gemini"},
		{"opencode", "models:\n  opencode:\n    - name: x\n", "models.opencode: hoom no le pasa el modelo a opencode"},
		{"sin name", "models:\n  codex:\n    - name: ok\n    - description: y\n", "models.codex[1]: falta name"},
		{"name vacio", "models:\n  codex:\n    - name: \"   \"\n", "models.codex[0]: falta name"},
		{"name con guion", "models:\n  claude:\n    - name: -x\n", "models.claude[0]: el nombre no puede empezar con '-'"},
		{"name con guion tras espacios", "models:\n  claude:\n    - name: \"  -x\"\n", "models.claude[0]: el nombre no puede empezar con '-'"},
		{"clave desconocida", "models:\n  codex:\n    - name: x\n      desc: y\n", `models.codex[0]: clave desconocida "desc" (validas: name, description)`},
		{"la clave antes que el name", "models:\n  codex:\n    - desc: y\n", `models.codex[0]: clave desconocida "desc" (validas: name, description)`},
		{"el primero del archivo", "models:\n  codex:\n    - name: -a\n  foo:\n    - name: x\n", "models.codex[0]: el nombre no puede empezar con '-'"},
		{"no es un mapa", "models:\n  - codex\n", "models debe ser un mapa por provider (ej. models: { codex: [ { name: gpt-7-sol } ] })"},
		{"no es una lista", "models:\n  codex: gpt-7-sol\n", "models.codex debe ser una lista de modelos (ej. - name: gpt-7-sol)"},
		{"entrada que no es un mapa", "models:\n  codex:\n    - gpt-7-sol\n", "models.codex[0] debe ser un mapa con name y description"},
		{"provider repetido", "models:\n  codex:\n    - name: a\n  codex:\n    - name: b\n", `models: el provider "codex" esta repetido (linea `},
	}
	for _, c := range casos {
		_, err := mmCargar(t, c.yaml)
		if err == nil {
			t.Fatalf("CA-573 (%s): hoom.yaml es invalido", c.nombre)
		}
		if !strings.Contains(err.Error(), "hoom.yaml invalido: ") || !strings.Contains(err.Error(), c.texto) {
			t.Fatalf("CA-573 (%s): el error es 'hoom.yaml invalido: ' y %q, es %q", c.nombre, c.texto, err.Error())
		}
	}
}

// CA-573: LoadWithoutBaseCheck aplica las mismas reglas a models:.
func TestCA573_LoadWithoutBaseCheckTambien(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(mmBase+"models:\n  gemini:\n    - name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.LoadWithoutBaseCheck(dir, profiles.Resolve); err == nil || !strings.Contains(err.Error(), "models.gemini: hoom no le pasa el modelo a gemini") {
		t.Fatalf("CA-573: LoadWithoutBaseCheck rechaza models invalido: %v", err)
	}
}
