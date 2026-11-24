// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// `hoom init` (CA-807): despues de los gates escribe el bloque exacto del
// spec (agents, review y spec), igual para los cuatro perfiles, y el
// hoom.yaml que escribe carga con manifest.Load con esos valores. Escritos
// desde el spec, las aclaraciones de la ronda 0 y las firmas publicas, sin
// leer la implementacion.
package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/spec"
)

// pprBloque es el bloque del spec, copiado del spec tal cual.
const pprBloque = `# Modelo por rol en niveles (fuerte, medio, barato): hoom los traduce al
# modelo de cada provider; un nombre de modelo tambien vale. Cada rol puede
# cambiar sus valores segun el riesgo de la tarjeta (bajo, normal, alto).
agents:
  arquitecto: { model: fuerte }
  designer: { model: medio }
  scout: { model: barato }
  writer: { model: medio, alto: { model: fuerte } }
  test-writer: { model: medio, alto: { model: fuerte } }
  characterizer: { model: medio, alto: { model: fuerte } }
  analista: { model: medio }
  refutador: { model: medio, alto: { model: fuerte } }
review:
  model: fuerte
# Topes del spec segun el riesgo declarado de la tarjeta (0 = sin tope).
spec:
  topes:
    bajo: { palabras: 400, criterios: 4, citas_por_criterio: 10 }
    normal: { palabras: 800, criterios: 8, citas_por_criterio: 10 }
    alto: { palabras: 2000, criterios: 15, citas_por_criterio: 0 }
`

// CA-807: BloquePorRiesgo es el bloque del spec.
func TestCA807_BloquePorRiesgoEsElDelSpec(t *testing.T) {
	const ca = "CA-807"
	if BloquePorRiesgo != pprBloque {
		t.Fatalf("%s: BloquePorRiesgo es el bloque del spec:\n%s", ca, BloquePorRiesgo)
	}
}

// CA-807: en los cuatro perfiles, hoom.yaml trae el bloque despues de los
// gates, con el mismo texto, y carga con manifest.Load con sus valores.
func TestCA807_InitEscribeElBloqueEnLosCuatroPerfiles(t *testing.T) {
	const ca = "CA-807"
	var colas []string
	for _, perfil := range []string{"go", "laravel", "kmp", "kmp-compose"} {
		dir := t.TempDir()
		if out, err := veeCorrer(t, dir, perfil); err != nil {
			t.Fatalf("%s: hoom init --profile %s: %v\n%s", ca, perfil, err, out)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "hoom.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		y := string(raw)
		ig, ib := strings.Index(y, "\ngates:"), strings.Index(y, pprBloque)
		if ib < 0 {
			t.Fatalf("%s: %s: hoom.yaml trae el bloque exacto:\n%s", ca, perfil, y)
		}
		if ig < 0 || ig > ib {
			t.Fatalf("%s: %s: el bloque va despues de los gates (%d, %d):\n%s", ca, perfil, ig, ib, y)
		}
		if strings.Count(y, "\nagents:") != 1 || strings.Count(y, "\nspec:") != 1 || strings.Count(y, "\nreview:") != 1 {
			t.Fatalf("%s: %s: una sola seccion agents, review y spec:\n%s", ca, perfil, y)
		}
		colas = append(colas, y[ib:])
		m, err := manifest.Load(dir, profiles.Resolve)
		if err != nil {
			t.Fatalf("%s: %s: el hoom.yaml de init carga: %v\n%s", ca, perfil, err, y)
		}
		if w := m.RoleDefaults("writer"); w.Model != "medio" || w.PorRiesgo["alto"].Model != "fuerte" {
			t.Fatalf("%s: %s: agents.writer: %+v", ca, perfil, w)
		}
		for rol, want := range map[string]string{"arquitecto": "fuerte", "designer": "medio", "scout": "barato", "test-writer": "medio",
			"characterizer": "medio", "analista": "medio", "refutador": "medio", "reviewer": "fuerte"} {
			if got := m.RoleDefaults(rol).Model; got != want {
				t.Fatalf("%s: %s: %s es %s: %q", ca, perfil, rol, want, got)
			}
		}
		if m.TopeDelSpec("normal") != (spec.Tope{Palabras: 800, Criterios: 8, CitasPorCriterio: 10}) ||
			m.TopeDelSpec("alto") != (spec.Tope{Palabras: 2000, Criterios: 15}) ||
			m.TopeDelSpec("bajo") != (spec.Tope{Palabras: 400, Criterios: 4, CitasPorCriterio: 10}) {
			t.Fatalf("%s: %s: los topes del bloque: %+v", ca, perfil, m.Spec)
		}
	}
	for i := 1; i < len(colas); i++ {
		if !strings.HasPrefix(colas[i], pprBloque) || !strings.HasPrefix(colas[0], pprBloque) {
			t.Fatalf("%s: el mismo texto en los cuatro perfiles", ca)
		}
	}
}
