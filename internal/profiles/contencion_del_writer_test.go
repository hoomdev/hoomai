// Tests adversariales del spec .hoom/specs/contencion-del-writer.md (CA-682):
// las variables de cache de un perfil (profiles.Cache), que lo encerrado
// recibe dentro de su cache. Solo se fijan las del perfil go (aclaracion 18).
package profiles_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/hoomdev/hoomai/internal/profiles"
)

// CA-682: el perfil go declara GOCACHE, GOMODCACHE y GOTMPDIR, sin repetidos.
func TestCA682_LasVariablesDeCacheDelPerfilGo(t *testing.T) {
	vars, err := profiles.Cache("go")
	if err != nil {
		t.Fatalf("CA-682: profiles.Cache(go): %v", err)
	}
	got := append([]string(nil), vars...)
	sort.Strings(got)
	if want := []string{"GOCACHE", "GOMODCACHE", "GOTMPDIR"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-682: el perfil go declara %q; fue %q", want, vars)
	}
}

// CA-682: ningun perfil repite una variable (los padres primero, sin
// repetidos), y un perfil desconocido es un error.
func TestCA682_SinRepetidosYPerfilDesconocido(t *testing.T) {
	for _, n := range profiles.Names() {
		vars, err := profiles.Cache(n)
		if err != nil {
			t.Fatalf("CA-682: profiles.Cache(%q) de un perfil que existe: %v", n, err)
		}
		visto := map[string]bool{}
		for _, v := range vars {
			if v == "" || visto[v] {
				t.Fatalf("CA-682: el perfil %s repite o deja vacia una variable: %q", n, vars)
			}
			visto[v] = true
		}
	}
	if _, err := profiles.Cache("perfil-que-no-existe"); err == nil {
		t.Fatal("CA-682: un perfil desconocido es el error de Resolve")
	}
}
