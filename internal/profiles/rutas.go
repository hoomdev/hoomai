package profiles

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

// rolTestWriter is the role whose write territory is what counts as prueba.
const rolTestWriter = "test-writer"

// Rutas are the default rutas of profile name and of everything it extends
// (spec riesgo-efectivo-y-tamano, «Valores por defecto»): riesgo.RutasBase,
// and after it the row of each profile of the chain, parents first, each in
// its profile's order, with no repeats. "" (no profile) is
// riesgo.RutasBase alone; an unknown profile is the error of Resolve. The
// rows are:
//
//   - go: generadas go.sum, vendor;
//   - laravel: de_bajo_riesgo lang, resources/lang; generadas composer.lock,
//     package-lock.json, yarn.lock, pnpm-lock.yaml, vendor, node_modules,
//     public/build;
//   - kmp (and so kmp-compose, which extends it): generadas
//     kotlin-js-store/yarn.lock, **/gradle.lockfile.
//
// No profile adds to de_riesgo.
func Rutas(name string) (riesgo.Rutas, error) {
	out := riesgo.RutasBase()
	if name == "" {
		return out, nil
	}
	chain, err := chainOf(name)
	if err != nil {
		return riesgo.Rutas{}, err
	}
	for _, p := range chain {
		out.DeRiesgo = sumarSinRepetir(out.DeRiesgo, p.Rutas.DeRiesgo)
		out.DeBajoRiesgo = sumarSinRepetir(out.DeBajoRiesgo, p.Rutas.DeBajoRiesgo)
		out.Generadas = sumarSinRepetir(out.Generadas, p.Rutas.Generadas)
	}
	return out, nil
}

// sumarSinRepetir appends to l each pattern of mas that l does not have yet,
// in mas' order.
func sumarSinRepetir(l, mas []string) []string {
	for _, p := range mas {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		repetido := false
		for _, q := range l {
			if q == p {
				repetido = true
				break
			}
		}
		if !repetido {
			l = append(l, p)
		}
	}
	return l
}

// Clasificador is the classifier of the project whose hoom.yaml is m
// (riesgo.Clasificador): its rutas, m.Rutas over the defaults of m.Profile
// (manifest.Rutas.Resolver over Rutas(m.Profile)), and the test-writer's
// territory, its agents.test-writer.write.allow, or, when m does not declare
// it (or declares it empty), agents.AllowPorDefecto(agents.ScopeTests): the
// allow agentcmd.PolicyFor gives that role. A nil m is a project without
// hoom.yaml: riesgo.RutasBase and the default territory. An unknown profile
// is the error of Resolve.
//
// `hoom verify` uses the one of the hoom.yaml it verifies with (the tree's,
// as the gates read it), and the board the project's.
func Clasificador(m *manifest.Manifest) (riesgo.Clasificador, error) {
	if m == nil {
		return clasificador("", nil, nil)
	}
	return clasificador(m.Profile, m.Rutas, m.Agents)
}

// clasificador is the classifier of a hoom.yaml whose profile, section
// rutas and section agents are these.
func clasificador(perfil string, rutas *manifest.Rutas, roles map[string]manifest.AgentPolicy) (riesgo.Clasificador, error) {
	def, err := Rutas(perfil)
	if err != nil {
		return riesgo.Clasificador{}, err
	}
	pruebas := agents.AllowPorDefecto(agents.ScopeTests)
	if ap, ok := roles[rolTestWriter]; ok {
		// as agentcmd.PolicyFor: a declared allow (blanks dropped) replaces
		// the default; an empty one is no allow
		var allow []string
		for _, g := range ap.Write.Allow {
			if g = strings.TrimSpace(g); g != "" {
				allow = append(allow, g)
			}
		}
		if len(allow) > 0 {
			pruebas = allow
		}
	}
	return riesgo.Clasificador{Rutas: rutas.Resolver(def), Pruebas: pruebas}, nil
}

// ClasificadorDe is Clasificador for the bytes of a hoom.yaml that is not
// loaded: the merge-base's, which `hoom review` reads for its rutas and the
// test-writer's territory, never the reviewed tree's (CA-417, CA-784). It
// reads only the keys profile, rutas (with manifest.ParseRutas' strictness
// and errors) and agents (manifest.ParseAgents). nil or empty raw (no
// hoom.yaml in that commit) is Clasificador(nil).
func ClasificadorDe(raw []byte) (riesgo.Clasificador, error) {
	var doc struct {
		Profile string `yaml:"profile"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return riesgo.Clasificador{}, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	rutas, err := manifest.ParseRutas(raw)
	if err != nil {
		return riesgo.Clasificador{}, err
	}
	roles, err := manifest.ParseAgents(raw)
	if err != nil {
		return riesgo.Clasificador{}, err
	}
	return clasificador(doc.Profile, rutas, roles)
}
