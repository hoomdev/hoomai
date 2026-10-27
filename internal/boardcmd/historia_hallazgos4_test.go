// Test del hallazgo 49af1d (delta 2 de la review de historia-en-el-tablero,
// ronda 4 del test-writer): cuando el espacio de trabajo de una tarea no se
// puede usar porque esta en otra rama, y esa rama se llama con un caracter
// de direccion o de control que git acepta en un nombre de rama, la salida
// humana del tablero (el texto de 'hoom board', el de la tarjeta de 'hoom
// item show' con su doctor) no trae ese caracter crudo, y la tarjeta sigue
// sin poder usarse (el fail-closed se conserva). Escrito desde el spec
// .hoom/specs/historia-en-el-tablero.md y el texto del hallazgo, sin leer la
// implementacion. Repos git reales en t.TempDir(). Todo identificador de este
// archivo lleva el prefijo h4.
package boardcmd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
)

// h4Ramas son, por tarea, ramas que git acepta (git check-ref-format) con un
// caracter hostil: la anulacion de direccion U+202E, el CSI de 8 bits
// U+009B (un control C1) y el aislamiento U+2067..U+2069.
var h4Ramas = map[string]string{
	"hist-rlo": "otra-‮dcba",
	"hist-csi": "otra-\u009b31m-roja",
	"hist-iso": "otra-⁧aislada⁩",
	"plana":    "otra-‮analp",
}

// h4Historicas son las tareas de h4Ramas con item historico.
var h4Historicas = []string{"hist-rlo", "hist-csi", "hist-iso"}

// h4EsCrudo: dentro de una linea, un control C0 (salvo \t), DEL, un control
// C1, o un caracter de embebido, anulacion o aislamiento de direccion.
func h4EsCrudo(r rune) bool {
	switch {
	case r < 0x20:
		return r != '\t'
	case r >= 0x7f && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// h4TextoLimpio exige que ninguna linea del texto traiga un caracter crudo.
func h4TextoLimpio(t *testing.T, ca, que, texto string) {
	t.Helper()
	for _, l := range strings.Split(texto, "\n") {
		for _, r := range l {
			if h4EsCrudo(r) {
				t.Fatalf("%s: %s no trae caracteres de control o de direccion crudos (U+%04X) en %+q", ca, que, r, l)
			}
		}
	}
}

// h4Campo exige lo mismo de un campo de una linea.
func h4Campo(t *testing.T, ca, que, v string) {
	t.Helper()
	for _, r := range v {
		if h4EsCrudo(r) || r == '\n' {
			t.Fatalf("%s: %s no trae caracteres de control o de direccion crudos (U+%04X): %+q", ca, que, r, v)
		}
	}
}

// CA-451, CA-452, hallazgo 49af1d: tres historicas (dos por hallazgos, una
// por veredicto) y una tarjeta comun, cada una con su espacio de trabajo en
// una rama de nombre hostil. El espacio no se puede usar (WorktreeInusable),
// la historica no se sostiene por lo de root y no queda en Hecho, la tarjeta
// sigue bloqueada (Missing no vacio), y nada de lo que se muestra a una
// persona trae el caracter hostil crudo: el texto de 'hoom board', el de la
// tarjeta ('hoom item show', con su doctor), y los campos de una linea de la
// tarjeta (Missing, Plain, Next y cada problema de su doctor).
func TestCA452_Hallazgo49af1d_RamaHostilSinCrudos(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	for _, s := range []string{"hist-rlo", "hist-csi"} {
		htCerrar(t, root, htHallazgo(t, root, s, "high"), finding.StatusCorrected)
		bdEscribir(t, root, ".hoom/items/"+s+".yaml", htItemYAML(s, "bug", item.HistoricoHallazgos))
	}
	htSpec(t, root, "hist-iso", "# Spec: hist-iso")
	bdAprobar(t, root, "hist-iso")
	bdCommitear(t, root, "hallazgos, items y spec")
	v := htVeredicto(t, root, root, "hist-iso", bdT0, false, bdGatesVerdes(), nil)
	htForma(t, v, true, false)
	bdEscribir(t, root, ".hoom/items/hist-iso.yaml", htItemYAML("hist-iso", "feature", item.HistoricoVeredicto(v.ID)))
	bdEscribir(t, root, ".hoom/items/plana.yaml", htItemYAML("plana", "feature", ""))
	bdCommitear(t, root, "verde e items")
	for s := range h4Ramas {
		bdWorktree(t, root, s)
	}
	for _, s := range h4Historicas {
		h1SeSostiene(t, "CA-451 (precondicion: en su rama)", root, s)
	}
	for s, rama := range h4Ramas {
		wt := filepath.Join(root, ".hoom", "worktrees", s)
		bdGit(t, wt, "checkout", "-q", "-b", rama)
	}

	b := h1Tablero(t, root)
	for s := range h4Ramas {
		ev := Gather(root, "main", "high", bdCard(t, b, s).Item, htAhora)
		if ev.WorktreeInusable == "" {
			t.Fatalf("precondicion: el espacio de %s, en la rama %+q, no se puede usar (WorktreeInusable)", s, h4Ramas[s])
		}
	}

	// el fail-closed se conserva
	for s := range h4Ramas {
		c := bdCard(t, b, s)
		if c.Column == ColHecho || c.Historica || len(c.Missing) == 0 {
			t.Fatalf("CA-452 (49af1d): con su espacio inusable, %s sigue bloqueada: fuera de Hecho, sin la marca y con lo que falta; fue %s historica=%v missing=%+q",
				s, c.Column, c.Historica, c.Missing)
		}
	}
	for _, s := range h4Historicas {
		c := bdCard(t, b, s)
		if ev := Gather(root, "main", "high", c.Item, htAhora); ev.HistoricaSostenida {
			t.Fatalf("CA-452 (49af1d): con su espacio inusable, la historica %s no se sostiene por lo de root", s)
		}
		n := 0
		for _, p := range c.Doctor {
			if p.ID == ProbHistoricaSinSosten {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("CA-452 (49af1d): el doctor de la tarjeta %s da UN historica-sin-sosten: %s", s, drFmt(c.Doctor))
		}
	}

	// la salida humana
	ca := "CA-451 (49af1d)"
	var buf bytes.Buffer
	Render(&buf, b)
	h4TextoLimpio(t, ca, "el texto de 'hoom board'", buf.String())
	for s := range h4Ramas {
		c := bdCard(t, b, s)
		buf.Reset()
		RenderCard(&buf, c)
		h4TextoLimpio(t, ca, "el texto de la tarjeta "+s+" ('hoom item show')", buf.String())
		for i, m := range c.Missing {
			h4Campo(t, ca, fmt.Sprintf("Missing[%d] de %s", i, s), m)
		}
		h4Campo(t, ca, "Plain de "+s, c.Plain)
		h4Campo(t, ca, "Next de "+s, c.Next)
		for _, p := range c.Doctor {
			for campo, v := range map[string]string{"What": p.What, "Plain": p.Plain, "Action": p.Action, "Ref": p.Ref} {
				h4Campo(t, ca, fmt.Sprintf("el %s de %s en el doctor de %s", campo, p.ID, s), v)
			}
		}
	}
}
