package manifest

import (
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
// a map is the error `rutas debe ser un mapa (ej. rutas: { de_riesgo: [...] })`.
func (r *Rutas) UnmarshalYAML(n *yaml.Node) error {
	return nil
}

// ParseRutas reads only the `rutas:` section of a hoom.yaml, with the same
// strictness Load applies to it: the one `hoom review` reads from its
// merge-base, never from the tree it reviews (CA-417, like ParseReview).
// nil = no section (or a null one).
func ParseRutas(raw []byte) (*Rutas, error) {
	return nil, nil
}

// Resolver is r over the defaults def (the project's profile:
// profiles.Rutas): each key r leaves nil takes def's list, and each key r
// gives (an empty list included) replaces it whole. A nil r is def. The
// lists returned are copies: changing them changes neither r nor def.
func (r *Rutas) Resolver(def riesgo.Rutas) riesgo.Rutas {
	return riesgo.Rutas{}
}
