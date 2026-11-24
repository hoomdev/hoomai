package manifest

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/riesgo"
)

// Rutas is the section `rutas` of hoom.yaml (spec riesgo-efectivo-y-tamano):
//
//	rutas:
//	  de_riesgo: ["app/Domain/Costeo", "**/*tenant*"]
//	  de_bajo_riesgo: ["**/*.md", "lang"]
//	  generadas: ["composer.lock", "vendor"]
//
// Each of the three keys is a list of patterns (riesgo.Coincide). A nil
// field is a key that is absent or null: it takes the default of the
// project's profile (Resolver). A non-nil field, an empty list included,
// replaces that default whole: `de_riesgo: []` leaves no risk path.
//
// The section is decoded strictly (UnmarshalYAML) by Load and by ParseRutas,
// and any of its errors makes the command that reads hoom.yaml fail like
// any other error of the manifest (`hoom verify` then runs no gate and
// writes no verdict).
type Rutas struct {
	DeRiesgo     []string `yaml:"de_riesgo"`
	DeBajoRiesgo []string `yaml:"de_bajo_riesgo"`
	Generadas    []string `yaml:"generadas"`
}

// RutasKeys are the keys `rutas:` accepts, in the order its error lists
// them: de_riesgo, de_bajo_riesgo, generadas.
var RutasKeys = []string{"de_riesgo", "de_bajo_riesgo", "generadas"}

// UnmarshalYAML decodes `rutas:` strictly. Its errors are exactly, with the
// key and the pattern quoted as %q quotes them:
//
//   - a key outside RutasKeys:
//     `rutas: clave desconocida "<k>" (validas: de_riesgo, de_bajo_riesgo, generadas)`;
//   - a key whose value is not null and not a list of strings (a scalar, a
//     map, or a list with an element that is null, a list or a map):
//     `rutas.<clave>: tiene que ser una lista de patrones`;
//   - an element that is not a valid pattern (riesgo.PatronValido):
//     `rutas.<clave>: patron invalido "<p>"`.
//
// A key that is null is left nil, like an absent one. A section that is not
// a map is the error `rutas: tiene que ser un mapa (ej. rutas: { de_riesgo: [...] })`.
func (r *Rutas) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("rutas: tiene que ser un mapa (ej. rutas: { de_riesgo: [...] })")
	}
	var out Rutas
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, v := n.Content[i].Value, n.Content[i+1]
		if !containsKey(RutasKeys, key) {
			return fmt.Errorf("rutas: clave desconocida %q (validas: %s)", key, strings.Join(RutasKeys, ", "))
		}
		if esNulo(v) {
			continue // null = without the key: the profile's default
		}
		if v.Kind != yaml.SequenceNode {
			return fmt.Errorf("rutas.%s: tiene que ser una lista de patrones", key)
		}
		lista := []string{} // not nil: an empty list replaces the default
		for _, e := range v.Content {
			if e.Kind != yaml.ScalarNode || e.ShortTag() != "!!str" {
				return fmt.Errorf("rutas.%s: tiene que ser una lista de patrones", key)
			}
			if !riesgo.PatronValido(e.Value) {
				return fmt.Errorf("rutas.%s: patron invalido %q", key, e.Value)
			}
			lista = append(lista, e.Value)
		}
		switch key {
		case "de_riesgo":
			out.DeRiesgo = lista
		case "de_bajo_riesgo":
			out.DeBajoRiesgo = lista
		case "generadas":
			out.Generadas = lista
		}
	}
	*r = out
	return nil
}

// ParseRutas reads only the `rutas:` section of a hoom.yaml, with the same
// strictness Load applies to it: the one `hoom review` reads from its
// merge-base, never from the tree it reviews (CA-417, like ParseReview).
// nil = no section (or a null one).
func ParseRutas(raw []byte) (*Rutas, error) {
	var doc struct {
		Rutas *Rutas `yaml:"rutas"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	return doc.Rutas, nil
}

// Resolver is r over the defaults def (the project's profile:
// profiles.Rutas): each key r leaves nil takes def's list, and each key r
// gives (an empty list included) replaces it whole. A nil r is def. The
// lists returned are copies: changing them changes neither r nor def.
func (r *Rutas) Resolver(def riesgo.Rutas) riesgo.Rutas {
	out := riesgo.Rutas{
		DeRiesgo:     copiaLista(def.DeRiesgo),
		DeBajoRiesgo: copiaLista(def.DeBajoRiesgo),
		Generadas:    copiaLista(def.Generadas),
	}
	if r == nil {
		return out
	}
	if r.DeRiesgo != nil {
		out.DeRiesgo = copiaLista(r.DeRiesgo)
	}
	if r.DeBajoRiesgo != nil {
		out.DeBajoRiesgo = copiaLista(r.DeBajoRiesgo)
	}
	if r.Generadas != nil {
		out.Generadas = copiaLista(r.Generadas)
	}
	return out
}

// copiaLista is a copy of l that keeps nil apart from empty.
func copiaLista(l []string) []string {
	if l == nil {
		return nil
	}
	return append([]string{}, l...)
}
