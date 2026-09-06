// Tests adversariales del spec .hoom/specs/test-writer-en-arbol-ciego.md:
// los archivos con los que un perfil RECONOCE el stack son exactamente los
// que un rol ciego necesita para saber para que esta escribiendo tests.
package profiles

import (
	"strings"
	"testing"
)

// CA-171: Markers devuelve los archivos de deteccion del perfil y de todo lo
// que hereda, sin repetidos, y falla nombrando los perfiles disponibles.
func TestCA171_MarkersDelPerfil(t *testing.T) {
	goMarkers, err := Markers("go")
	if err != nil {
		t.Fatal(err)
	}
	if len(goMarkers) != 1 || goMarkers[0] != "go.mod" {
		t.Fatalf("CA-171: go se reconoce por go.mod: %v", goMarkers)
	}

	laravel, err := Markers("laravel")
	if err != nil {
		t.Fatal(err)
	}
	if !contiene(laravel, "composer.json") || !contiene(laravel, "artisan") {
		t.Fatalf("CA-171: laravel se reconoce por composer.json y artisan: %v", laravel)
	}

	// la herencia: kmp-compose declara los suyos y hereda los de kmp
	compose, err := Markers("kmp-compose")
	if err != nil {
		t.Fatal(err)
	}
	kmp, err := Markers("kmp")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range kmp {
		if !contiene(compose, m) {
			t.Fatalf("CA-171: kmp-compose hereda de kmp y le falta %q: %v", m, compose)
		}
	}
	visto := map[string]bool{}
	for _, m := range compose {
		if visto[m] {
			t.Fatalf("CA-171: marcador repetido %q en %v", m, compose)
		}
		visto[m] = true
	}

	if _, err := Markers("perl-6"); err == nil || !strings.Contains(err.Error(), "go") {
		t.Fatalf("CA-171: un perfil desconocido debe nombrar los disponibles: %v", err)
	}
}

func contiene(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
