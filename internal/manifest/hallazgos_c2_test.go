// Tests de regresion de la review de 4 lentes de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md:
//
//   - 20261009T191843_bb986a (high, risk; CA-318): base_branch de hoom.yaml
//     termina como revision en las invocaciones de git que hacen las lecturas
//     del tablero. Un valor que empieza con "-" lo leeria git como una opcion
//     (--output=<ruta> crea o trunca un archivo), asi que cargar un hoom.yaml
//     con ese valor es un error que nombra base_branch. Una rama con guiones
//     en el medio sigue siendo una base valida.
package manifest

import (
	"math/rand"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

// c2CargarBase carga un hoom.yaml valido con la linea base_branch dada (tal
// cual, con su fin de linea).
func c2CargarBase(t *testing.T, linea string) (*Manifest, error) {
	t.Helper()
	return Load(foManifiesto(t, linea), nil)
}

// Hallazgo 20261009T191843_bb986a. CA-318: un base_branch que empieza con
// "-" hace fallar manifest.Load, y el error nombra base_branch. Con comillas
// o sin ellas en el YAML, y con cualquier cosa despues del guion.
func TestHallazgo_bb986a_BaseBranchQueEmpiezaConGuionEsError(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "x") // nunca se crea: Load no corre git
	valores := []string{
		"--output=" + ruta,
		"--output=x",
		"--end-of-options",
		"--no-index",
		"--",
		"-",
		"-x",
		"-main",
		"--upload-pack=x",
	}
	var lineas []string
	for _, v := range valores {
		lineas = append(lineas, "base_branch: "+strconv.Quote(v)+"\n")
	}
	// sin comillas tambien es un texto de YAML que empieza con guion
	lineas = append(lineas, "base_branch: --output="+ruta+"\n", "base_branch: -x\n", "base_branch: '--output=x'\n")

	for _, linea := range lineas {
		m, err := c2CargarBase(t, linea)
		if err == nil {
			t.Errorf("bb986a CA-318: %q debe hacer fallar manifest.Load (git leeria la base como una opcion); cargo con BaseBranch=%q",
				strings.TrimSpace(linea), m.BaseBranch)
			continue
		}
		if !strings.Contains(err.Error(), "base_branch") {
			t.Errorf("bb986a CA-318: el error de %q debe nombrar base_branch, fue: %v", strings.TrimSpace(linea), err)
		}
	}
}

// Guarda del hallazgo 20261009T191843_bb986a. CA-318: la validacion no se
// lleva puestas las bases de verdad: una rama con guiones en el medio, con
// barras o con puntos sigue cargando y llega tal cual a BaseBranch; y un
// hoom.yaml sin base_branch sigue siendo valido.
func TestHallazgo_bb986a_GuardaLasRamasConGuionEnElMedioSiguenValiendo(t *testing.T) {
	for _, v := range []string{"main", "develop", "feature-x", "hoom/fixes-cabina-c2", "release/1.x", "a--b", "v1.0-rc1", "x-"} {
		m, err := c2CargarBase(t, "base_branch: "+strconv.Quote(v)+"\n")
		if err != nil {
			t.Fatalf("bb986a CA-318: la base %q no empieza con guion y es valida, Load fallo: %v", v, err)
		}
		if m.BaseBranch != v {
			t.Fatalf("bb986a CA-318: la base %q llega tal cual a BaseBranch, fue %q", v, m.BaseBranch)
		}
	}
	if _, err := c2CargarBase(t, ""); err != nil {
		t.Fatalf("bb986a CA-318: un hoom.yaml sin base_branch sigue siendo valido: %v", err)
	}
}

// c2Base genera valores de base_branch con guiones en cualquier lugar,
// tambien al principio y detras de espacios.
type c2Base string

func (c2Base) Generate(rnd *rand.Rand, size int) reflect.Value {
	const alfabeto = "--- \tab/=.x1"
	n := rnd.Intn(9)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(alfabeto[rnd.Intn(len(alfabeto))])
	}
	return reflect.ValueOf(c2Base(b.String()))
}

// Hallazgo 20261009T191843_bb986a, como propiedad. CA-318: para cualquier
// valor, (1) si empieza con "-", Load falla nombrando base_branch; y (2) lo
// que Load deja en BaseBranch nunca empieza con "-" (tampoco si recorta los
// espacios de un valor como " -x"): a git nunca le llega una opcion.
func TestHallazgo_bb986a_PropiedadBaseBranchNuncaEsUnaOpcion(t *testing.T) {
	prop := func(b c2Base) bool {
		valor := string(b)
		m, err := c2CargarBase(t, "base_branch: "+strconv.Quote(valor)+"\n")
		if strings.HasPrefix(valor, "-") {
			if err == nil {
				t.Logf("base_branch %q empieza con guion y cargo (BaseBranch=%q)", valor, m.BaseBranch)
				return false
			}
			if !strings.Contains(err.Error(), "base_branch") {
				t.Logf("el error de base_branch %q no nombra base_branch: %v", valor, err)
				return false
			}
			return true
		}
		if err == nil && strings.HasPrefix(m.BaseBranch, "-") {
			t.Logf("base_branch %q quedo en BaseBranch como %q, que git leeria como opcion", valor, m.BaseBranch)
			return false
		}
		return true
	}
	cfg := &quick.Config{MaxCount: 200, Rand: rand.New(rand.NewSource(20261009))}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("bb986a CA-318: base_branch nunca le llega a git como una opcion: %v", err)
	}
}
