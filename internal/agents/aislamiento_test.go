// Tests adversariales del spec .hoom/specs/test-writer-en-arbol-ciego.md:
// el aislamiento es un campo del rol y no una consecuencia de su forma de
// scope, porque dos roles comparten la forma `tests` y solo uno escribe sin
// mirar.
package agents

import "testing"

// CA-170: solo el test-writer corre ciego. El characterizer comparte la forma
// `tests` y NO se aisla: su trabajo es fijar el comportamiento del codigo
// legacy, asi que cegarlo seria romperlo.
func TestCA170_SoloElTestWriterCorreCiego(t *testing.T) {
	all := Roles()
	if len(all) != 10 {
		t.Fatalf("CA-170: la tabla debe seguir teniendo 10 roles, tiene %d", len(all))
	}
	var ciegos []string
	for _, r := range all {
		if r.Isolated {
			ciegos = append(ciegos, r.Slug)
		}
	}
	if len(ciegos) != 1 || ciegos[0] != "test-writer" {
		t.Fatalf("CA-170: el unico rol ciego es el test-writer, hay %v", ciegos)
	}

	ch, err := Lookup("characterizer")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Scope != ScopeTests {
		t.Fatalf("CA-170: el characterizer debe seguir en la forma tests: %q", ch.Scope)
	}
	if ch.Isolated {
		t.Fatal("CA-170: el characterizer necesita LEER el legacy que caracteriza; cegarlo lo rompe")
	}

	// el orden, los flags y los scopes de la tabla no se movieron
	tw, err := Lookup("hoom-test-writer")
	if err != nil {
		t.Fatal(err)
	}
	if !tw.Isolated || tw.Scope != ScopeTests || tw.ReadOnly || tw.Exec || tw.Primary {
		t.Fatalf("CA-170: el test-writer escribe tests, corre ciego y nada mas: %+v", tw)
	}
	if all[0].Slug != "orquestador" || all[len(all)-1].Slug != "refutador" {
		t.Fatalf("CA-170: el orden de la tabla cambio: %s..%s", all[0].Slug, all[len(all)-1].Slug)
	}

	// mutar la copia no puede tocar la tabla
	all[5].Isolated = false
	if tw2, _ := Lookup("test-writer"); !tw2.Isolated {
		t.Fatal("CA-170: Roles() debe devolver una copia; la tabla es la fuente unica")
	}
}
