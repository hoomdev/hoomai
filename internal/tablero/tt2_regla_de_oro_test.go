// La regla de oro (CA-517 de la v1, que sigue igual en la v2 y en la v3: "la
// TUI es una piel y la tarjeta decide"): el codigo del paquete no tiene tabla
// de columnas, y el orden y los nombres de las columnas son los que trae el
// tablero. En la v2 las secciones de [2] eran las columnas (CA-555); en la v3
// la lista va por grupos (CA-700, que con CA-701 re-expresa CA-555), en TE
// TOCA por columna desde la ultima del tablero hasta la primera, y el nombre
// de una columna se ve donde la v3 lo dice (aca, en la caja TU SIGUIENTE
// PASO: "en <column_name> no hay nada que hacer ahora: <plain>"). De
// tt_lista_test.go de la v1.
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

// CA-517, CA-700, CA-701, CA-555: la lista sigue el orden en que el tablero
// trae sus columnas (aca uno que no es el de boardcmd.Columns): en la v3 los
// grupos van en su orden (TE TOCA, CORRIENDO, DETENIDA, EN COLA, HECHO, sin
// grupos vacios salvo HECHO) y en TE TOCA las tarjetas van por columna desde
// la ultima columna del tablero hasta la primera (doc.go, "The list"): con
// este tablero Arquitecto (Tres) va antes que Writer (Uno), al reves que en
// boardcmd.Columns. Los nombres son los que trae el tablero: la caja dice el
// de la columna de su tarjeta (Tres, sin acciones: la regla 6 del paso), que
// ninguna tabla de la TUI podria dar. Re-expresado: en la v2 las secciones de
// [2] eran las columnas, con su nombre en mayusculas.
func TestCA517_CA700_CA701_CA555_SeccionesEnElOrdenDelTablero(t *testing.T) {
	w := ttCard("w1", "Uno", boardcmd.ColWriter, "escribe", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	w.ColumnName = "Escritura"
	b := ttCard("b1", "Dos", boardcmd.ColBacklog, "falta", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	b.ColumnName = "Pendientes"
	a := ttCard("a1", "Tres", boardcmd.ColArquitecto, "spec", ttSeg("s", "s", boardcmd.SegFalta, "x"))
	a.ColumnName = "Diseno"
	tb := boardcmd.Board{Warnings: []string{}, Columns: []boardcmd.BoardColumn{
		{ID: boardcmd.ColWriter, Name: "Escritura", Cards: []boardcmd.Card{w}},
		{ID: boardcmd.ColReview, Name: "Review", Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColBacklog, Name: "Pendientes", Cards: []boardcmd.Card{b}},
		{ID: boardcmd.ColTestWriter, Name: "Test-writer", Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColArquitecto, Name: "Diseno", Cards: []boardcmd.Card{a}},
		{ID: boardcmd.ColTuAprobacion, Name: "Tu aprobacion", Human: true, Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColTuAceptacion, Name: "Tu aceptacion", Human: true, Cards: []boardcmd.Card{}},
		{ID: boardcmd.ColHecho, Name: "Hecho", Cards: []boardcmd.Card{}},
	}}
	m := ttModelo(ttConfig(120, 32, false), tb)
	lista := tv3bLista(t, "CA-700", m)
	var titulos []string
	for _, l := range lista.ttTexto() {
		if len(l) > 2 && l[0] == ' ' && l[1] >= 'A' && l[1] <= 'Z' {
			titulos = append(titulos, l)
		}
	}
	quiere := []string{" TE TOCA 2", " EN COLA 1", " HECHO 0 >"}
	if strings.Join(titulos, "|") != strings.Join(quiere, "|") {
		t.Fatalf("CA-700: los grupos van en su orden, sin los vacios salvo HECHO: %q, se esperaba %q\n%s", titulos, quiere, ttPantalla(m))
	}
	tres, uno, dos := tv3bFilaDe(lista, "Tres"), tv3bFilaDe(lista, "Uno"), tv3bFilaDe(lista, "Dos")
	if tres < 0 || uno < 0 || dos < 0 || !(tres < uno && uno < dos) {
		t.Fatalf("CA-517, CA-700, CA-701: en TE TOCA Tres (Diseno) va antes que Uno (Escritura), desde la ultima columna del tablero, y despues EN COLA con Dos: filas %d %d %d\n%s", tres, uno, dos, ttPantalla(m))
	}
	if m.Elegida() != "a1" || m.Caja() != "a1" {
		t.Fatalf("CA-700: la primera fila de la lista, y la tarjeta de la caja, es Tres (a1): elegida %q, caja %q", m.Elegida(), m.Caja())
	}
	if !strings.Contains(strings.Join(tv3bCajaPaso(t, "CA-517", m).ttTexto(), "\n"), "en Diseno no hay nada que hacer ahora: spec") {
		t.Fatalf("CA-517: el nombre de la columna es el que trae el tablero (en Diseno ...):\n%s", ttPantalla(m))
	}
}
