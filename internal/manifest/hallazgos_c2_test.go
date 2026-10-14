// Tests de regresion de la review de 4 lentes de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md:
//
//   - 20261009T191843_bb986a (high, risk; CA-318): base_branch de hoom.yaml
//     termina como revision en las invocaciones de git que hacen las lecturas
//     del tablero. Un valor que empieza con "-" lo leeria git como una opcion
//     (--output=<ruta> crea o trunca un archivo), asi que cargar un hoom.yaml
//     con ese valor es un error que nombra base_branch. Una rama con guiones
//     en el medio sigue siendo una base valida.
//
// Y de la review delta 4 de la tarea fixes-cabina-c2:
//
//   - 20261014T171507_461d05 (low, readability): la carga que no valida la
//     base es una funcion explicita, LoadWithoutBaseCheck, para un hoom.yaml
//     cuyo base_branch hoom solo muestra y nunca le pasa a git (la rama de la
//     tarea que lee la review, CA-430 de .hoom/specs/base-de-la-review.md).
//     Aplica TODAS las reglas de Load salvo la de base_branch: nada mas se
//     afloja, y con una base valida las dos dan lo mismo.
package manifest

import (
	"fmt"
	"math/rand"
	"os"
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

// ---------------------------------------------------------------------------
// 20261014T171507_461d05: LoadWithoutBaseCheck es Load salvo base_branch
// ---------------------------------------------------------------------------

const c2Gates = "gates:\n  test:\n    required: true\n    cmd: \"true\"\n"

// c2Escribir deja body como el hoom.yaml de dir.
func c2Escribir(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// c2ConBase es un hoom.yaml con schema, project, base_branch base y resto.
func c2ConBase(base, resto string) string {
	return "schema: hoom/v1\nproject: demo\nbase_branch: " + strconv.Quote(base) + "\n" + resto
}

// c2SinPerfiles es un resolveProfile que no conoce ningun perfil.
func c2SinPerfiles(name string) (map[string]Gate, string, error) {
	return nil, "", fmt.Errorf("no existe el perfil %q", name)
}

// Hallazgo 20261014T171507_461d05. CA-430 (base-de-la-review: el base_branch
// de la rama se muestra y nunca se obedece): con base_branch que empieza con
// "-" y el resto valido, LoadWithoutBaseCheck carga y deja BaseBranch tal
// cual, y Load sobre el MISMO archivo falla nombrando base_branch. Lo que
// carga es lo mismo que Load carga de ese archivo con base_branch main, salvo
// la base: no se afloja ni se pierde nada mas.
func TestHallazgo_461d05_SinElChequeoDeBaseCargaLoQueLoadRechazaPorLaBase(t *testing.T) {
	for _, base := range []string{"--x", "-x", "-", "--output=" + filepath.Join(t.TempDir(), "x")} {
		dir := t.TempDir()
		c2Escribir(t, dir, c2ConBase(base, c2Gates))
		m, err := LoadWithoutBaseCheck(dir, nil)
		if err != nil {
			t.Fatalf("461d05 CA-430: con base_branch %q y el resto valido LoadWithoutBaseCheck carga, fallo: %v", base, err)
		}
		if m.BaseBranch != base {
			t.Fatalf("461d05 CA-430: LoadWithoutBaseCheck deja base_branch tal cual (%q), fue %q", base, m.BaseBranch)
		}
		if _, err := Load(dir, nil); err == nil || !strings.Contains(err.Error(), "base_branch") {
			t.Fatalf("461d05: Load del mismo archivo (base_branch %q) falla nombrando base_branch, fue: %v", base, err)
		}

		c2Escribir(t, dir, c2ConBase("main", c2Gates))
		conMain, err := Load(dir, nil)
		if err != nil {
			t.Fatalf("fixture: el mismo hoom.yaml con base_branch main carga: %v", err)
		}
		copia := *m
		copia.BaseBranch = "main"
		if !reflect.DeepEqual(&copia, conMain) {
			t.Fatalf("461d05 CA-430: salvo la base, LoadWithoutBaseCheck da lo mismo que Load con base_branch main:\nsin chequeo (base %q): %+v\nLoad (main):          %+v", base, m, conMain)
		}
	}
}

// Hallazgo 20261014T171507_461d05. CA-430 y CA-414: LoadWithoutBaseCheck no
// perdona nada mas que la base. Con base_branch "--x" y otra cosa invalida
// (un perfil que no existe, un hoom.yaml sin gates ni perfil, una clave
// desconocida en review:, un schema distinto) falla, y con el MISMO error que
// da con base_branch main, que es el de Load con base_branch main.
func TestHallazgo_461d05_SinElChequeoDeBaseLoDemasSeValidaIgual(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   func(base string) string
		perfiles func(string) (map[string]Gate, string, error)
	}{
		{"perfil que no existe", func(b string) string {
			return "schema: hoom/v1\nproject: demo\nprofile: no-existe-461d05\nbase_branch: " + strconv.Quote(b) + "\n"
		}, c2SinPerfiles},
		{"sin gates ni perfil", func(b string) string { return c2ConBase(b, "") }, nil},
		{"clave desconocida en review", func(b string) string { return c2ConBase(b, c2Gates+"review:\n  inventada: 1\n") }, nil},
		{"schema distinto", func(b string) string {
			return "schema: hoom/v2\nproject: demo\nbase_branch: " + strconv.Quote(b) + "\n" + c2Gates
		}, nil},
	}
	for _, c := range casos {
		dir := t.TempDir()
		c2Escribir(t, dir, c.cuerpo("--x"))
		_, errGuion := LoadWithoutBaseCheck(dir, c.perfiles)
		c2Escribir(t, dir, c.cuerpo("main"))
		_, errMain := LoadWithoutBaseCheck(dir, c.perfiles)
		_, errLoad := Load(dir, c.perfiles)
		if errMain == nil || errLoad == nil {
			t.Errorf("fixture [%s]: el hoom.yaml con base_branch main es invalido para Load y para LoadWithoutBaseCheck: Load=%v, sin chequeo=%v", c.nombre, errLoad, errMain)
			continue
		}
		if errGuion == nil {
			t.Errorf("461d05 CA-430 [%s]: con base_branch \"--x\" LoadWithoutBaseCheck falla igual que con base_branch main (%v); cargo", c.nombre, errMain)
			continue
		}
		if errGuion.Error() != errMain.Error() {
			t.Errorf("461d05 CA-430 [%s]: el error de LoadWithoutBaseCheck no depende de la base:\ncon \"--x\": %v\ncon main:  %v", c.nombre, errGuion, errMain)
		}
		if errMain.Error() != errLoad.Error() {
			t.Errorf("461d05 CA-430 [%s]: con base_branch main, LoadWithoutBaseCheck falla con el error de Load:\nsin chequeo: %v\nLoad:        %v", c.nombre, errMain, errLoad)
		}
	}
}

// c2BaseValida genera bases que no empiezan con "-": segmentos de letras y
// digitos unidos por "/", "-" o ".", con una letra al principio.
type c2BaseValida string

func (c2BaseValida) Generate(rnd *rand.Rand, size int) reflect.Value {
	const letras, todo, juntas = "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz0123456789", "/-."
	var b strings.Builder
	b.WriteByte(letras[rnd.Intn(len(letras))])
	for seg := rnd.Intn(3); seg >= 0; seg-- {
		for i := rnd.Intn(5); i >= 0; i-- {
			b.WriteByte(todo[rnd.Intn(len(todo))])
		}
		if seg > 0 {
			b.WriteByte(juntas[rnd.Intn(len(juntas))])
		}
	}
	return reflect.ValueOf(c2BaseValida(b.String()))
}

// Hallazgo 20261014T171507_461d05, como propiedad. CA-430: con una base que
// no empieza con "-" (main, develop, release/1.0, feature-x y generadas), en
// un hoom.yaml valido con gates propios o heredados de un perfil, Load y
// LoadWithoutBaseCheck cargan los dos y devuelven Manifest iguales
// (reflect.DeepEqual).
func TestHallazgo_461d05_PropiedadConUnaBaseValidaLasDosCarganLoMismo(t *testing.T) {
	// control: dos Load del mismo archivo son iguales (si no, DeepEqual no dice nada)
	ctrl := t.TempDir()
	c2Escribir(t, ctrl, c2ConBase("main", c2Gates))
	a, errA := Load(ctrl, nil)
	b, errB := Load(ctrl, nil)
	if errA != nil || errB != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("fixture: dos Load del mismo hoom.yaml dan Manifest iguales: %v %v", errA, errB)
	}

	revisar := func(base string) bool {
		for _, v := range []struct {
			resto    string
			perfiles func(string) (map[string]Gate, string, error)
		}{
			{c2Gates, nil},
			{"profile: fake\n" + c2Gates + "  mutation:\n    cmd: \"otro\"\n", laravelLikeResolver},
		} {
			dir := t.TempDir()
			c2Escribir(t, dir, c2ConBase(base, v.resto))
			mL, errL := Load(dir, v.perfiles)
			mW, errW := LoadWithoutBaseCheck(dir, v.perfiles)
			if errL != nil || errW != nil {
				t.Logf("con base_branch %q (%q) las dos cargan: Load=%v, sin chequeo=%v", base, strings.SplitN(v.resto, "\n", 2)[0], errL, errW)
				return false
			}
			if !reflect.DeepEqual(mL, mW) {
				t.Logf("con base_branch %q las dos dan el mismo Manifest:\nLoad:        %+v\nsin chequeo: %+v", base, mL, mW)
				return false
			}
		}
		return true
	}
	for _, base := range []string{"main", "develop", "release/1.0", "feature-x", "hoom/fixes-cabina-c2", "x-"} {
		if !revisar(base) {
			t.Fatalf("461d05 CA-430: con base_branch %q Load y LoadWithoutBaseCheck cargan lo mismo", base)
		}
	}
	prop := func(b c2BaseValida) bool { return revisar(string(b)) }
	if err := quick.Check(prop, &quick.Config{MaxCount: 60, Rand: rand.New(rand.NewSource(461))}); err != nil {
		t.Fatalf("461d05 CA-430: con una base que no empieza con guion Load y LoadWithoutBaseCheck cargan lo mismo: %v", err)
	}
}
