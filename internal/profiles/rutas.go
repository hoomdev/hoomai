package profiles

import (
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

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
	return riesgo.Rutas{}, nil
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
	return riesgo.Clasificador{}, nil
}

// ClasificadorDe is Clasificador for the bytes of a hoom.yaml that is not
// loaded: the merge-base's, which `hoom review` reads for its rutas and the
// test-writer's territory, never the reviewed tree's (CA-417, CA-784). It
// reads only the keys profile, rutas (with manifest.ParseRutas' strictness
// and errors) and agents (manifest.ParseAgents). nil or empty raw (no
// hoom.yaml in that commit) is Clasificador(nil).
func ClasificadorDe(raw []byte) (riesgo.Clasificador, error) {
	return riesgo.Clasificador{}, nil
}
