// Regresion del hallazgo 647f9a (ronda 1 de proporcion-por-riesgo): solo el
// spec <root>/.hoom/specs/<slug>.md tiene la tarjeta <slug>. Un spec en otra
// ruta que tambien termina en .hoom/specs/<slug>.md (fuera del proyecto, o
// anidado en otro directorio) es un spec en otra ruta: riesgo normal, sin
// tarjeta. El arbol de una tarea (su worktree, pasado como root) sigue
// encontrando la suya.
package approval_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/spec"
)

func TestHallazgo_647f9a_SpecDeOtraRutaNoHeredaLaTarjeta(t *testing.T) {
	// la tarjeta precios es de riesgo bajo: tope 400 palabras
	root, sp, m := pprProyecto(t, "", 600, 2, "riesgo: bajo\n")
	if c := approval.CalibreDe(m, root, sp); c.Tarjeta != "precios" || c.Riesgo != riesgo.Bajo {
		t.Fatalf("fixture: el spec del proyecto tiene su tarjeta de riesgo bajo: %+v", c)
	}
	normal := spec.Tope{Palabras: 800, Criterios: 8, CitasPorCriterio: 10}

	fuera := pprEscribir(t, t.TempDir(), ".hoom/specs/precios.md", pprTexto(t, 600, 2))
	anidado := pprEscribir(t, root, "docs/.hoom/specs/precios.md", pprTexto(t, 600, 2))
	for _, ruta := range []string{fuera, anidado} {
		c := approval.CalibreDe(m, root, ruta)
		if c.Tarjeta != "" || c.Riesgo != riesgo.Normal || c.Ilegible != "" || c.Tope != normal {
			t.Fatalf("hallazgo 647f9a: %s no es el spec de la tarjeta precios: riesgo normal, sin tarjeta, tope %+v; fue %+v", ruta, normal, c)
		}
		// 600 palabras pasan el tope normal (800): se aprueba
		if _, _, err := approval.Aprobar(m, root, ruta); err != nil {
			t.Fatalf("hallazgo 647f9a: %s se mide con riesgo normal y se aprueba: %v", ruta, err)
		}
	}
	// el del proyecto sigue negandose con el tope de su tarjeta (400)
	var neg *approval.Negativa
	if _, _, err := approval.Aprobar(m, root, sp); !errors.As(err, &neg) {
		t.Fatalf("hallazgo 647f9a: el spec del proyecto pasa el tope de su tarjeta (400): *Negativa, fue %v", err)
	}

	// el worktree de la tarea, como root, encuentra la tarjeta en su arbol
	wt := filepath.Join(root, ".hoom", "worktrees", "precios")
	pprGit(t, root, "worktree", "add", "-q", "-b", "hoom/precios", wt)
	if c := approval.CalibreDe(m, wt, filepath.Join(wt, ".hoom", "specs", "precios.md")); c.Tarjeta != "precios" || c.Riesgo != riesgo.Bajo {
		t.Fatalf("hallazgo 647f9a: el spec del worktree de la tarea tiene su tarjeta: %+v", c)
	}
	if c := approval.CalibreDe(m, wt, ".hoom/specs/precios.md"); c.Tarjeta != "precios" || c.Tope != (spec.Tope{Palabras: 400, Criterios: 4, CitasPorCriterio: 10}) {
		t.Fatalf("hallazgo 647f9a: una ruta relativa es del root (el worktree): %+v", c)
	}
}
