package manifest

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/riesgo"
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
	topes := TopesPorDefecto()
	*p = SpecPolicy{Topes: topes}
	if esNulo(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("spec debe ser un mapa")
	}
	visto := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1]
		if !containsKey(SpecKeys, key) {
			return fmt.Errorf("spec: clave desconocida %q (validas: %s)", key, strings.Join(SpecKeys, ", "))
		}
		if visto[key] {
			return fmt.Errorf("spec: la clave %q esta repetida (linea %d)", key, n.Content[i].Line)
		}
		visto[key] = true
		if err := decodeTopes(val, topes); err != nil {
			return err
		}
	}
	return nil
}

// decodeTopes is spec.topes over topes, which holds the defaults.
func decodeTopes(n *yaml.Node, topes map[string]spec.Tope) error {
	if esNulo(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("spec.topes debe ser un mapa")
	}
	visto := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		r, val := n.Content[i].Value, n.Content[i+1]
		if !containsKey(riesgo.Vocabulario, r) {
			return fmt.Errorf("spec.topes: clave desconocida %q (validas: %s)", r, strings.Join(riesgo.Vocabulario, ", "))
		}
		if visto[r] {
			return fmt.Errorf("spec.topes: el riesgo %q esta repetido (linea %d)", r, n.Content[i].Line)
		}
		visto[r] = true
		t, err := decodeTope("spec.topes."+r, val, topes[r])
		if err != nil {
			return err
		}
		topes[r] = t
	}
	return nil
}

// decodeTope is spec.topes.<r> (donde) over t, the riesgo's defaults.
func decodeTope(donde string, n *yaml.Node, t spec.Tope) (spec.Tope, error) {
	if esNulo(n) {
		return t, nil
	}
	if n.Kind != yaml.MappingNode {
		return t, fmt.Errorf("%s debe ser un mapa", donde)
	}
	visto := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i].Value, n.Content[i+1]
		if !containsKey(TopeKeys, k) {
			return t, fmt.Errorf("%s: clave desconocida %q (validas: %s)", donde, k, strings.Join(TopeKeys, ", "))
		}
		if visto[k] {
			return t, fmt.Errorf("%s: la clave %q esta repetida (linea %d)", donde, k, n.Content[i].Line)
		}
		visto[k] = true
		v, err := enteroDelTope(donde+"."+k, val)
		if err != nil {
			return t, err
		}
		switch k {
		case "palabras":
			t.Palabras = v
		case "criterios":
			t.Criterios = v
		case "citas_por_criterio":
			t.CitasPorCriterio = v
		}
	}
	return t, nil
}

// enteroDelTope is one value of spec.topes.<r>: an integer >= 0. A scalar
// that is not one is named as written; any other node by its YAML type.
func enteroDelTope(donde string, n *yaml.Node) (int, error) {
	mal := func(v string) error {
		return fmt.Errorf("%s: %s no es un entero mayor o igual que 0 (0 = sin tope)", donde, v)
	}
	if n.Kind != yaml.ScalarNode {
		return 0, mal(n.ShortTag())
	}
	var v int
	if n.ShortTag() != "!!int" || n.Decode(&v) != nil || v < 0 {
		return 0, mal(n.Value)
	}
	return v, nil
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
	return map[string]spec.Tope{
		riesgo.Bajo:   {Palabras: 400, Criterios: 4, CitasPorCriterio: 10},
		riesgo.Normal: {Palabras: 800, Criterios: 8, CitasPorCriterio: 10},
		riesgo.Alto:   {Palabras: 2000, Criterios: 15, CitasPorCriterio: 0},
	}
}

// TopeDelSpec is the tope of hoom.yaml for a spec whose card declares
// riesgo r: m's spec.topes.<r>, with TopesPorDefecto for each key the
// section leaves out. A nil manifest, or one without the section, gives
// TopesPorDefecto()[r]; "" reads as riesgo.Normal. The card's own tope
// (item tope_palabras, tope_criterios) goes over this one in
// approval.CalibreDe, never here.
func (m *Manifest) TopeDelSpec(r string) spec.Tope {
	if r == "" {
		r = riesgo.Normal
	}
	if m == nil || m.Spec == nil || m.Spec.Topes == nil {
		return TopesPorDefecto()[r]
	}
	if t, ok := m.Spec.Topes[r]; ok {
		return t
	}
	return TopesPorDefecto()[r]
}
