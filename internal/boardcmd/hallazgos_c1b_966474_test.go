// Tests adversariales del hallazgo 20261021T174140_966474 (medium,
// reliability) de la delta 1 de fixes-cabina-c1b, sobre el tablero
// (.hoom/specs/items-y-columna-derivada.md: CA-280, la evidencia sale del
// worktree de la tarea si existe; CA-269, el siguiente paso). Escritos desde
// el contrato, nunca desde la implementacion:
//
// Una tarjeta cuyo worktree esta registrado en .hoom/worktrees/<slug> pero
// quedo fuera de su rama hoom/<slug> (HEAD separado, otra rama, main, o una
// rama que empieza como la suya) no es una tarjeta sin espacio de trabajo:
// en Build y en CardFor queda bloqueada con un motivo (Card.Missing) que
// nombra su rama hoom/<slug>, y su siguiente paso no es `hoom task start`
// (que fallaria: la carpeta ya existe). Guarda: de vuelta en su rama, la
// tarjeta lee su worktree sin ese motivo.
//
// Reusa b6Tarjeta de hallazgos_c1b_6aeb17_test.go, las utilidades bd* de
// gather_test.go y worktreetest.RamasAjenas.
package boardcmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// b96MotivoDeRama devuelve el motivo de c que nombra la rama hoom/<slug> de
// la tarjeta, o "".
func b96MotivoDeRama(c Card) string {
	for _, m := range c.Missing {
		l := strings.ToLower(m)
		if strings.Contains(l, "hoom/"+c.Slug) && strings.Contains(l, "rama") {
			return m
		}
	}
	return ""
}

// b96Bloqueada exige que la tarjeta este bloqueada con el motivo de su rama
// y que su siguiente paso no sea crear la tarea.
func b96Bloqueada(t *testing.T, como string, c Card) {
	t.Helper()
	if b96MotivoDeRama(c) == "" {
		t.Errorf("CA-280: hallazgo 966474 (%s): con el worktree fuera de su rama la tarjeta queda bloqueada con un motivo que nombra hoom/%s; missing=%q column=%s next=%q source=%s",
			como, c.Slug, c.Missing, c.Column, c.Next, c.Evidence.Source)
	}
	if strings.Contains(c.Next, "hoom task start") {
		t.Errorf("CA-269: hallazgo 966474 (%s): el worktree ya existe: el siguiente paso no es %q", como, c.Next)
	}
}

// Hallazgo 966474. CA-280 y CA-269: con el worktree de precios fuera de su
// rama, Build y CardFor dejan la tarjeta bloqueada con el motivo de su rama
// y sin `hoom task start`.
func TestHallazgo966474_CA280_TableroConElWorktreeFueraDeSuRama(t *testing.T) {
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			root := b6Tarjeta(t)
			wt := filepath.Join(root, ".hoom", "worktrees", bdSlug)
			c, err := CardFor(root, "main", "high", bdSlug, b6Now)
			if err != nil || c.Evidence.Source != SourceWorktree || b96MotivoDeRama(c) != "" {
				t.Fatalf("precondicion en su rama: la tarjeta lee su worktree sin motivo de rama: err=%v source=%s missing=%q", err, c.Evidence.Source, c.Missing)
			}

			v.Armar(t, wt, bdSlug)
			b, err := Build(root, "main", "high", b6Now)
			if err != nil {
				t.Fatalf("CA-280: hallazgo 966474 (%s): con el worktree fuera de su rama el tablero se arma igual: %v", v.Nombre, err)
			}
			b96Bloqueada(t, v.Nombre+", Build", bdCard(t, b, bdSlug))
			c, err = CardFor(root, "main", "high", bdSlug, b6Now)
			if err != nil {
				t.Fatalf("CA-280: hallazgo 966474 (%s): CardFor da la tarjeta bloqueada, no un error: %v", v.Nombre, err)
			}
			b96Bloqueada(t, v.Nombre+", CardFor", c)

			worktreetest.VolverASuRama(t, wt, bdSlug)
			c, err = CardFor(root, "main", "high", bdSlug, b6Now)
			if err != nil || c.Evidence.Source != SourceWorktree || b96MotivoDeRama(c) != "" {
				t.Errorf("CA-280: guarda 966474 (%s): de vuelta en su rama la tarjeta lee su worktree sin motivo de rama: err=%v source=%s missing=%q",
					v.Nombre, err, c.Evidence.Source, c.Missing)
			}
		})
	}
}
