// Tests del hallazgo 20261021T163600_e7ab69 (medium, reliability) de la delta
// 1 de fixes-cabina-c1b, sobre las acciones de la tarjeta
// (.hoom/specs/acciones-desde-la-tarjeta.md, CA-325 a CA-346) cuando su
// espacio de trabajo no se pudo comprobar (hallazgo 6aeb17: git no pudo decir
// si la carpeta de la tarea es su worktree, Evidence.WorktreeSinComprobar).
// Escritos desde el contrato, nunca desde la implementacion:
//
// Una tarjeta cuyo espacio de trabajo no se pudo comprobar no ofrece ninguna
// accion, ni habilitada ni deshabilitada: card.actions es la lista vacia
// (`[]` en JSON, no null) y Card.Action no encuentra ninguna. Vale en todas
// las columnas, con o sin espacio propio, interrumpida, con cambios sin
// guardar o con algo en curso, y en disco con git fuera del PATH aunque haya
// un provider instalado.
//
// Guardas: sin esa marca, las mismas tarjetas ofrecen sus acciones de
// siempre; en disco, con git sano, la misma tarjeta ofrece las suyas.
//
// Reusa los fixtures ac* de acciones_test.go y b6Tarjeta y
// b6MotivoSinComprobar de hallazgos_c1b_6aeb17_test.go.
package boardcmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// e7SinComprobar es el motivo con que Gather marca la tarjeta cuando git no
// pudo comprobar su espacio de trabajo.
const e7SinComprobar = "no se pudo comprobar el espacio de trabajo de la tarea: git no respondio"

// e7TodasLasAcciones son los ids de todas las acciones del contrato.
var e7TodasLasAcciones = []string{ActPedirArquitecto, ActPedirTestWriter, ActPedirWriter, ActPedirReviewer,
	ActAprobar, ActIntegrar, ActReanudar, ActRelanzar, ActDescartar, ActGuardar, ActSesion, ActTerminal}

// e7Fixture es una tarjeta en memoria y las acciones que ofrece sin la marca
// (nil = no se exige la lista, solo que no este vacia).
type e7Fixture struct {
	nombre string
	ev     Evidence
	ids    []string
}

// e7Fixtures recorre las columnas con y sin espacio propio, y variantes
// interrumpidas, con cambios sin guardar, en curso y con el presupuesto
// agotado.
func e7Fixtures() []e7Fixture {
	var fs []e7Fixture
	for _, c := range append(acColumnas(), acColumnasArbol()...) {
		fs = append(fs, e7Fixture{c.nombre, c.ev, c.ids})
	}
	fs = append(fs,
		e7Fixture{"writer interrumpido", acInterrumpido(acWriter(), "writer", "claude"), nil},
		e7Fixture{"arquitecto interrumpido en backlog", func() Evidence {
			ev := acEv()
			ev.SpecExists = false
			return acInterrumpido(ev, "arquitecto", "claude")
		}(), nil},
		e7Fixture{"writer con cambios sin guardar", func() Evidence {
			ev := acWriter()
			ev.Uncommitted = acSucios()
			return ev
		}(), nil},
		e7Fixture{"backlog sin espacio propio con cambios sin guardar", func() Evidence {
			ev := bdArbol(acEv())
			ev.SpecExists = false
			ev.Uncommitted = []string{".hoom/items/precios.yaml"}
			return ev
		}(), nil},
		e7Fixture{"writer en curso", acConEnCurso(acWriter(), "writer"), nil},
		e7Fixture{"writer con el presupuesto agotado", acConGasto(acWriter(), 1, 2), nil},
		e7Fixture{"tu-aceptacion interrumpida y con cambios sin guardar", func() Evidence {
			ev := acInterrumpido(acEv(), "writer", "claude")
			ev.Uncommitted = acSucios()
			return ev
		}(), nil},
	)
	return fs
}

// e7SinAcciones exige que la tarjeta no ofrezca ninguna accion: actions vacio
// (y `[]` en JSON) y Card.Action no encuentra ninguna.
func e7SinAcciones(t *testing.T, como string, c Card) {
	t.Helper()
	if len(c.Actions) != 0 {
		t.Errorf("CA-325: hallazgo e7ab69 (%s): sin poder comprobar el espacio de trabajo la tarjeta no ofrece ninguna accion, ni habilitada ni deshabilitada; ofrece %s",
			como, e7Describir(c.Actions))
		return
	}
	for _, id := range e7TodasLasAcciones {
		if a, ok := c.Action(id); ok {
			t.Errorf("CA-338: hallazgo e7ab69 (%s): Card.Action(%q) no encuentra la accion en una tarjeta sin espacio comprobado: encontro %+v", como, id, a)
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("CA-325: hallazgo e7ab69 (%s): la tarjeta se serializa: %v", como, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(m["actions"])); got != "[]" {
		t.Errorf("CA-325: hallazgo e7ab69 (%s): actions es la lista vacia en JSON (`[]`, sin null), es %s", como, got)
	}
}

func e7Describir(as []Action) string {
	var out []string
	for _, a := range as {
		s := a.ID
		if a.Enabled {
			s += " (habilitada)"
		} else {
			s += " (deshabilitada: " + a.Why + ")"
		}
		out = append(out, s)
	}
	return "[" + strings.Join(out, ", ") + "]"
}

// Hallazgo 20261021T163600_e7ab69. CA-325 y CA-326, en memoria (Derive): en
// todas las columnas, con y sin espacio propio, interrumpida, con cambios sin
// guardar, en curso y con el presupuesto agotado, una tarjeta con
// WorktreeSinComprobar no ofrece ninguna accion. Guarda: sin la marca, la
// misma tarjeta ofrece sus acciones de siempre (las de la tabla, o al menos
// una).
func TestCA325_Hallazgoe7ab69_SinComprobarElEspacioNoHayAcciones(t *testing.T) {
	for _, f := range e7Fixtures() {
		guarda := Derive(f.ev)
		if f.ids != nil {
			if got := acIDs(guarda); !reflect.DeepEqual(got, f.ids) {
				t.Errorf("CA-325: guarda e7ab69 (%s): sin la marca la tarjeta ofrece %v, ofrece %v", f.nombre, f.ids, got)
			}
		} else if len(guarda.Actions) == 0 {
			t.Errorf("CA-325: guarda e7ab69 (%s): sin la marca la tarjeta ofrece alguna accion", f.nombre)
		}

		ev := f.ev
		ev.WorktreeSinComprobar = e7SinComprobar
		e7SinAcciones(t, f.nombre, Derive(ev))

		// como la deja Gather: la evidencia se lee del arbol raiz
		ev = bdArbol(f.ev)
		ev.WorktreeSinComprobar = e7SinComprobar
		e7SinAcciones(t, f.nombre+", leida del arbol raiz", Derive(ev))
	}
}

// e7ClaudeFalso deja en dir un `claude` falso que no hace nada.
func e7ClaudeFalso(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Hallazgo 20261021T163600_e7ab69. CA-325, CA-336 y CA-346, en disco: la
// tarjeta precios con su worktree y su spec commiteado ahi (b6Tarjeta), con
// git fuera del PATH y un provider (claude, falso) instalado. Build y
// CardFor la dan bloqueada por no poder comprobar su espacio de trabajo y
// sin ninguna accion (ni pedir-arquitecto, ni guardar, ni descartar).
// Guarda: con git sano y el mismo provider, la misma tarjeta lee su worktree
// y ofrece sus acciones (al menos sesion y terminal, que tiene espacio
// propio).
func TestCA325_Hallazgoe7ab69_SinGitLaTarjetaNoOfreceAcciones(t *testing.T) {
	root := b6Tarjeta(t)
	// cambios sin guardar en el arbol raiz y en el worktree: sin la marca
	// darian guardar y descartar
	bdEscribir(t, root, ".hoom/items/"+bdSlug+".yaml", bdItemYAML("Precios", "pedido: el pedido de precios, editado\n"))
	bdEscribir(t, filepath.Join(root, bdWtDir), "precios.go", "package precios\n")

	provs := t.TempDir()
	e7ClaudeFalso(t, provs)

	t.Run("guarda: git sano", func(t *testing.T) {
		t.Setenv("PATH", provs+string(os.PathListSeparator)+os.Getenv("PATH"))
		c, err := CardFor(root, "main", "high", bdSlug, b6Now)
		if err != nil {
			t.Fatalf("CA-280: guarda e7ab69: CardFor con git sano: %v", err)
		}
		if c.Evidence.Source != SourceWorktree || b6MotivoSinComprobar(c) != "" {
			t.Fatalf("CA-280: guarda e7ab69: con git sano la tarjeta lee su worktree: source=%s missing=%q", c.Evidence.Source, c.Missing)
		}
		ids := acIDs(c)
		if len(ids) == 0 || !strings.Contains(strings.Join(ids, ","), ActSesion) || !strings.Contains(strings.Join(ids, ","), ActTerminal) {
			t.Errorf("CA-325: guarda e7ab69: con git sano la tarjeta (columna %s) ofrece sus acciones, con sesion y terminal: %s",
				c.Column, e7Describir(c.Actions))
		}
	})

	t.Run("git fuera del PATH", func(t *testing.T) {
		t.Setenv("PATH", provs)
		if _, err := exec.LookPath("git"); err == nil {
			t.Fatal("fixture: git no esta en el PATH")
		}
		if _, err := exec.LookPath("claude"); err != nil {
			t.Fatalf("fixture: el claude falso esta en el PATH: %v", err)
		}
		b, err := Build(root, "main", "high", b6Now)
		if err != nil {
			t.Fatalf("CA-280: hallazgo e7ab69: sin git el tablero se arma igual: %v", err)
		}
		c := bdCard(t, b, bdSlug)
		if b6MotivoSinComprobar(c) == "" {
			t.Fatalf("precondicion (hallazgo 6aeb17): sin git la tarjeta queda bloqueada por no poder comprobar su espacio de trabajo: missing=%q", c.Missing)
		}
		e7SinAcciones(t, "Build sin git", c)

		c, err = CardFor(root, "main", "high", bdSlug, b6Now)
		if err != nil {
			t.Fatalf("CA-280: hallazgo e7ab69: sin git CardFor da la tarjeta bloqueada: %v", err)
		}
		e7SinAcciones(t, "CardFor sin git", c)
	})
}
