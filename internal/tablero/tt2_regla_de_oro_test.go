// La regla de oro (CA-517 de la v1, que sigue igual en la v2: "la TUI es una
// piel y la tarjeta decide"): el codigo del paquete no tiene tabla de
// columnas, y las secciones de [2] siguen el orden y los nombres que trae el
// tablero (CA-555 las re-expresa). De tt_lista_test.go de la v1: las
// aserciones son las mismas, mirando [2].
package tablero_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// CA-517: el codigo del paquete (sin los tests) no contiene ningun id de
// columna como cadena.
func TestCA517_SinIDsDeColumnaComoCadena(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", n))
		if err != nil {
			t.Fatal(err)
		}
		vistos++
		for _, id := range []string{`"backlog"`, `"tu-aprobacion"`, `"tu-aceptacion"`, `"hecho"`} {
			if strings.Contains(string(raw), id) {
				t.Fatalf("CA-517: %s contiene el id de columna %s como cadena: la TUI no tiene tabla de columnas", n, id)
			}
		}
	}
	if vistos == 0 {
		t.Fatal("CA-517: no encontre los archivos del paquete")
	}
}

// CA-517, CA-555: las secciones de columna de [2] siguen el orden en que el
// tablero trae sus columnas (aca uno que no es el de boardcmd.Columns) y sus
// titulos son los nombres que trae el tablero, en mayusculas.
func TestCA517_CA555_SeccionesEnElOrdenDelTablero(t *testing.T) {
	w := ttCard("w1", "Uno", boardcmd.ColWriter, "escribe", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	b := ttCard("b1", "Dos", boardcmd.ColBacklog, "falta", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	a := ttCard("a1", "Tres", boardcmd.ColArquitecto, "spec", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	tb := boardcmd.Board{Warnings: []string{}, Columns: []boardcmd.BoardColumn{
		{ID: boardcmd.ColWriter, Name: "Escritura", Cards: []boardcmd.Card{w}},
		{ID: boardcmd.ColReview, Name: "Review", Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColBacklog, Name: "Pendientes", Cards: []boardcmd.Card{b}},
		{ID: boardcmd.ColTestWriter, Name: "Test-writer", Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColArquitecto, Name: "Arquitecto", Cards: []boardcmd.Card{a}},
		{ID: boardcmd.ColTuAprobacion, Name: "Tu aprobacion", Human: true, Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColTuAceptacion, Name: "Tu aceptacion", Human: true, Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColHecho, Name: "Hecho", Cards: []boardcmd.Card{}},
	}}
	m := ttModelo(ttConfig(120, 32, false), tb)
	var titulos []string
	for _, l := range ttCajaDe(t, "CA-517", m, "[2]").ttTexto() {
		if len(l) > 2 && l[0] == ' ' && l[1] >= 'A' && l[1] <= 'Z' {
			titulos = append(titulos, l)
		}
	}
	quiere := []string{" ESCRITURA 1", " PENDIENTES 1", " ARQUITECTO 1", " HECHO 0 >"}
	if strings.Join(titulos, "|") != strings.Join(quiere, "|") {
		t.Fatalf("CA-517: las secciones van en el orden del tablero y con su nombre: %q, se esperaba %q\n%s", titulos, quiere, ttPantalla(m))
	}
}
