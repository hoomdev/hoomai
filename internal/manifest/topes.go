package manifest

import (
	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/spec"
)

// SpecPolicy is the optional `spec:` section of hoom.yaml
// (proporcion-por-riesgo): the tope of a spec for each riesgo its card may
// declare, which spec_lint measures and the approval enforces.
//
//	spec:
//	  topes:
//	    bajo:   { palabras: 400,  criterios: 4,  citas_por_criterio: 10 }
//	    normal: { palabras: 800,  criterios: 8,  citas_por_criterio: 10 }
//	    alto:   { palabras: 2000, criterios: 15, citas_por_criterio: 0 }
//
// Those are the defaults (TopesPorDefecto): without the section, or without
// a riesgo or a key, the key takes its default. Each value is an integer
// >= 0, and 0 is sin tope. The section is strict; the rest of the top level
// stays as lenient as always. Load (and LoadWithoutBaseCheck) and LoadRoles
// reject an invalid section, wrapped as `hoom.yaml invalido: <error>`, and
// then `hoom verify` runs no gate and writes no verdict. The errors are
// exactly, with <k> the key and <v> the value as written:
//
//   - `spec: clave desconocida "<k>" (validas: topes)`;
//   - `spec.topes: clave desconocida "<k>" (validas: bajo, normal, alto)`;
//   - `spec.topes.<r>: clave desconocida "<k>" (validas: palabras, criterios, citas_por_criterio)`;
//   - `spec.topes.<r>.<k>: <v> no es un entero mayor o igual que 0 (0 = sin tope)`;
//   - `<donde> debe ser un mapa`, with <donde> spec, spec.topes or
//     spec.topes.<r>.
//
// A spec: or a spec.topes that is null declares nothing.
type SpecPolicy struct {
	// Topes are the topes per riesgo, one entry per riesgo.Vocabulario,
	// every key resolved: what the section declares, and TopesPorDefecto
	// for each riesgo or key it leaves out.
	Topes map[string]spec.Tope
}

// UnmarshalYAML decodes spec: with the rules of SpecPolicy.
func (p *SpecPolicy) UnmarshalYAML(n *yaml.Node) error {
	return nil
}

// SpecKeys are the keys `spec:` accepts, in the order its error names them.
var SpecKeys = []string{"topes"}

// TopeKeys are the keys `spec.topes.<r>` accepts, in the order its error
// names them; `spec.topes` accepts riesgo.Vocabulario.
var TopeKeys = []string{"palabras", "criterios", "citas_por_criterio"}

// TopesPorDefecto are hoom's topes per riesgo, keyed by riesgo
// (riesgo.Vocabulario): bajo 400 words, 4 criteria and 10 citations per
// criterion; normal 800, 8 and 10; alto 2000, 15 and 0 (sin tope of
// citations). The result is a copy: changing it changes nothing here.
func TopesPorDefecto() map[string]spec.Tope {
	return nil
}

// TopeDelSpec is the tope of hoom.yaml for a spec whose card declares
// riesgo r: m's spec.topes.<r>, with TopesPorDefecto for each key the
// section leaves out. A nil manifest, or one without the section, gives
// TopesPorDefecto()[r]; "" reads as riesgo.Normal. The card's own tope
// (item tope_palabras, tope_criterios) goes over this one in
// approval.CalibreDe, never here.
func (m *Manifest) TopeDelSpec(r string) spec.Tope {
	return spec.Tope{}
}
