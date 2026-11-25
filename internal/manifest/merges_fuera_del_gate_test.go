// Regresion de un defecto que encontro el writer al corregir 13e26e, sin
// hallazgo registrado todavia: ValidarGates seguia los merges (<<) solo
// dentro de un gate, y leia las claves sin resolver sus alias. El decoder de
// yaml.v3 resuelve las dos cosas en todos los niveles, asi que:
//
//   - un <<: en la raiz que trae gates:, o una clave *k que es "gates",
//     cargaban sin validar un aplica_en invalido o una clave desconocida
//     (CA-809, fallaba abierto);
//   - un <<: dentro de gates: (gates tomados de un ancla) se rechazaba como
//     un gate llamado "<<", y un alias en una clave de un gate como la clave
//     desconocida "k" (fallaba cerrado, con el error equivocado).
package manifest_test

import (
	"fmt"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
)

func TestMergesFueraDelGate_CA809_LosErroresQueTraeUnMergeDeLaRaiz(t *testing.T) {
	const ca = "CA-809"
	casos := []struct {
		nombre, doc, want string
	}{
		{"<<: en la raiz, patron invalido",
			h13eCabeza + "x: &base\n  gates:\n    static: {cmd: \"true\", aplica_en: [\"app/[\"]}\n<<: *base\n",
			gprPrefijo + fmt.Sprintf(manifest.ErrAplicaEnPatron, "static", "app/[")},
		{"<<: en la raiz, clave desconocida",
			h13eCabeza + "x: &base\n  gates:\n    static: {cmd: \"true\", aplicaen: [app/**]}\n<<: *base\n",
			gprPrefijo + fmt.Sprintf(manifest.ErrGateClaveDesconocida, "static", "aplicaen")},
		{"<<: en la raiz, lista vacia, dos merges abajo",
			h13eCabeza + "x: &a {gates: {static: {cmd: \"true\", aplica_en: []}}}\ny: &b {<<: *a}\n<<: [*b]\n",
			gprPrefijo + fmt.Sprintf(manifest.ErrAplicaEnLista, "static")},
		{"clave *k que es gates",
			h13eCabeza + "k: &k gates\n*k : {static: {cmd: \"true\", aplica_en: [\"app/[\"]}}\n",
			gprPrefijo + fmt.Sprintf(manifest.ErrAplicaEnPatron, "static", "app/[")},
		{"<<: dentro de gates, patron invalido",
			h13eCabeza + "x: &gs\n  static: {cmd: \"true\", aplica_en: [\"app/[\"]}\ngates:\n  <<: *gs\n",
			gprPrefijo + fmt.Sprintf(manifest.ErrAplicaEnPatron, "static", "app/[")},
		{"clave de un gate *k que es aplica_en, patron invalido",
			h13eCabeza + "k: &k aplica_en\ngates:\n  static: {cmd: \"true\", *k : [\"app/[\"]}\n",
			gprPrefijo + fmt.Sprintf(manifest.ErrAplicaEnPatron, "static", "app/[")},
		{"nombre de gate *g",
			h13eCabeza + "g: &g static\ngates:\n  *g : {cmd: \"true\", aplicaen: [app/**]}\n",
			gprPrefijo + fmt.Sprintf(manifest.ErrGateClaveDesconocida, "static", "aplicaen")},
	}
	for _, c := range casos {
		dir := gprDir(t, c.doc)
		for _, l := range gprCargadores {
			if _, err := l.cargar(dir, nil); err == nil || err.Error() != c.want {
				t.Fatalf("%s (%s, %s): %q, no %v", ca, l.nombre, c.nombre, c.want, err)
			}
		}
	}
}

// Lo valido carga igual que lo lee el decoder: gates tomados de un ancla con
// <<:, y un alias en la clave aplica_en.
func TestMergesFueraDelGate_CA809_LoValidoCarga(t *testing.T) {
	const ca = "CA-809"
	casos := []struct {
		nombre, doc string
	}{
		{"<<: dentro de gates", h13eCabeza + "x: &gs\n  static: {cmd: \"true\", aplica_en: [app/**]}\ngates:\n  <<: *gs\n"},
		{"<<: en la raiz", h13eCabeza + "x: &base\n  gates:\n    static: {cmd: \"true\", aplica_en: [app/**]}\n<<: *base\n"},
		{"clave *k que es aplica_en", h13eCabeza + "k: &k aplica_en\ngates:\n  static: {cmd: \"true\", *k : [app/**]}\n"},
	}
	for _, c := range casos {
		m, err := manifest.Load(gprDir(t, c.doc), nil)
		if err != nil {
			t.Fatalf("%s (%s): carga: %v", ca, c.nombre, err)
		}
		if got := m.Gates["static"].AplicaEn; len(got) != 1 || got[0] != "app/**" {
			t.Fatalf("%s (%s): static con aplica_en [app/**]: %q", ca, c.nombre, got)
		}
	}
}
