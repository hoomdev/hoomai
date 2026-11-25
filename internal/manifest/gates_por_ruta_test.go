// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre la carga
// de hoom.yaml (CA-809): la clave gates.<g>.aplica_en se lee tal cual, la
// lista del proyecto reemplaza entera a la del perfil, los tres errores salen
// con su texto exacto y "hoom.yaml invalido: " delante (en Load y en
// LoadWithoutBaseCheck), un hoom.yaml sin aplica_en carga como hoy y los
// cuatro perfiles embebidos cargan. Que `hoom verify` no corra gates ni
// escriba veredicto con esos errores se prueba contra el binario
// (cmd/hoom/gates_por_ruta_test.go).
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package manifest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
)

const gprBase = "schema: hoom/v1\nproject: demo\n"

// gprPrefijo es lo que el spec pone delante de cada error de aplica_en.
const gprPrefijo = "hoom.yaml invalido: "

func gprPtr(s string) *string { return &s }

// gprDir escribe body como hoom.yaml de un directorio nuevo.
func gprDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// gprCargadores son los dos cargadores que el spec obliga.
var gprCargadores = []struct {
	nombre string
	cargar func(dir string, r func(string) (map[string]manifest.Gate, string, error)) (*manifest.Manifest, error)
}{
	{"Load", manifest.Load},
	{"LoadWithoutBaseCheck", manifest.LoadWithoutBaseCheck},
}

// gprPerfil es un perfil inyectado cuyos static y mutation traen aplica_en.
// Devuelve un mapa nuevo en cada llamada: nadie comparte sus listas.
func gprPerfil(name string) (map[string]manifest.Gate, string, error) {
	req := true
	return map[string]manifest.Gate{
		"mutation": {Required: &req, Cmd: gprPtr("vendor/bin/infection"), DiffCmd: gprPtr("vendor/bin/infection --git-diff-base={base}"),
			AplicaEn: []string{"src/**", "lib/**"}},
		"static": {Required: &req, Cmd: gprPtr("vendor/bin/phpstan"), AplicaEn: []string{"src/**"}},
		"test":   {Required: &req, Cmd: gprPtr("vendor/bin/phpunit")},
	}, "perfil de prueba", nil
}

// gprRechaza exige que los dos cargadores rechacen body con
// "hoom.yaml invalido: " + want al principio del error.
func gprRechaza(t *testing.T, ca, caso, body, want string) {
	t.Helper()
	dir := gprDir(t, body)
	for _, c := range gprCargadores {
		m, err := c.cargar(dir, nil)
		if err == nil {
			t.Fatalf("%s (%s): %s rechaza\n%s\ncon %q, y cargo: %+v", ca, caso, c.nombre, body, gprPrefijo+want, m)
		}
		if !strings.HasPrefix(err.Error(), gprPrefijo+want) {
			t.Fatalf("%s (%s): %s da exactamente\n  %q\nfue\n  %q", ca, caso, c.nombre, gprPrefijo+want, err.Error())
		}
	}
}

// ---------------------------------------------------------------- textos

// CA-809: las claves validas de un gate, en el orden de su error, y los tres
// textos del spec, exactos.
func TestCA809_GateKeysYLosTextosDeLosErrores(t *testing.T) {
	const ca = "CA-809"
	if want := []string{"required", "cmd", "diff_cmd", "notes", "partial", "aplica_en"}; !reflect.DeepEqual(manifest.GateKeys, want) {
		t.Fatalf("%s: GateKeys es %q, fue %q", ca, want, manifest.GateKeys)
	}
	casos := []struct{ got, want string }{
		{fmt.Sprintf(manifest.ErrAplicaEnLista, "mutation"), "gates.mutation.aplica_en: tiene que ser una lista no vacia de patrones"},
		{fmt.Sprintf(manifest.ErrAplicaEnPatron, "mutation", "a//b"), `gates.mutation.aplica_en: patron invalido "a//b"`},
		{fmt.Sprintf(manifest.ErrGateClaveDesconocida, "mutation", "rutas"),
			`gates.mutation: clave desconocida "rutas" (validas: required, cmd, diff_cmd, notes, partial, aplica_en)`},
	}
	for _, c := range casos {
		if c.got != c.want {
			t.Fatalf("%s: el texto del spec es\n  %q\nfue\n  %q", ca, c.want, c.got)
		}
	}
}

// ---------------------------------------------------------------- carga

// CA-809: aplica_en se lee con sus patrones tal como estan escritos y en su
// orden, en flujo y en bloque; un gate sin la clave queda con AplicaEn nil.
func TestCA809_AplicaEnSeCargaTalCual(t *testing.T) {
	const ca = "CA-809"
	body := gprBase + "gates:\n" +
		"  mutation:\n    required: true\n    cmd: \"true\"\n    aplica_en: [app/**, src/**]\n" +
		"  static:\n    required: true\n    cmd: \"true\"\n    aplica_en:\n      - \"App/Models/**\"\n      - lib\n" +
		"  test:\n    required: true\n    cmd: \"true\"\n"
	dir := gprDir(t, body)
	for _, c := range gprCargadores {
		m, err := c.cargar(dir, nil)
		if err != nil {
			t.Fatalf("%s: %s carga un hoom.yaml con aplica_en: %v", ca, c.nombre, err)
		}
		if got := m.Gates["mutation"].AplicaEn; !reflect.DeepEqual(got, []string{"app/**", "src/**"}) {
			t.Fatalf("%s: %s: mutation.aplica_en tal cual y en su orden: %q", ca, c.nombre, got)
		}
		if got := m.Gates["static"].AplicaEn; !reflect.DeepEqual(got, []string{"App/Models/**", "lib"}) {
			t.Fatalf("%s: %s: static.aplica_en tal como esta escrito: %q", ca, c.nombre, got)
		}
		if got := m.Gates["test"].AplicaEn; got != nil {
			t.Fatalf("%s: %s: un gate sin aplica_en queda con AplicaEn nil (aplica siempre): %q", ca, c.nombre, got)
		}
		if m.Gates["mutation"].CmdStr() != "true" || !m.Gates["mutation"].IsRequired() {
			t.Fatalf("%s: %s: aplica_en no cambia las otras claves del gate: %+v", ca, c.nombre, m.Gates["mutation"])
		}
	}
}

// CA-809: la lista del proyecto reemplaza ENTERA a la del perfil; un gate del
// proyecto sin la clave conserva la del perfil; un gate del perfil sin
// aplica_en sigue sin ella.
func TestCA809_ElDelProyectoReemplazaAlDelPerfil(t *testing.T) {
	const ca = "CA-809"
	body := gprBase + "profile: falso\ngates:\n" +
		"  mutation:\n    aplica_en: [app/**]\n" +
		"  static:\n    cmd: \"otro\"\n"
	dir := gprDir(t, body)
	for _, c := range gprCargadores {
		m, err := c.cargar(dir, gprPerfil)
		if err != nil {
			t.Fatalf("%s: %s carga con el perfil inyectado: %v", ca, c.nombre, err)
		}
		mut := m.Gates["mutation"]
		if !reflect.DeepEqual(mut.AplicaEn, []string{"app/**"}) {
			t.Fatalf("%s: %s: el aplica_en del proyecto reemplaza entero al del perfil ([src/**, lib/**]): %q", ca, c.nombre, mut.AplicaEn)
		}
		if mut.CmdStr() != "vendor/bin/infection" || mut.DiffCmdStr() != "vendor/bin/infection --git-diff-base={base}" || !mut.IsRequired() {
			t.Fatalf("%s: %s: lo demas del gate se hereda como hoy: %+v", ca, c.nombre, mut)
		}
		st := m.Gates["static"]
		if !reflect.DeepEqual(st.AplicaEn, []string{"src/**"}) || st.CmdStr() != "otro" {
			t.Fatalf("%s: %s: un gate del proyecto sin aplica_en conserva el del perfil: %+v", ca, c.nombre, st)
		}
		if got := m.Gates["test"].AplicaEn; got != nil {
			t.Fatalf("%s: %s: un gate del perfil sin aplica_en sigue sin ella: %q", ca, c.nombre, got)
		}
	}
}

// ---------------------------------------------------------------- errores

// CA-809 y aclaracion 4: una aplica_en que no es una lista no vacia de
// patrones (vacia, nula, sin valor, un escalar, un mapa, o con un elemento
// que no es string) es ErrAplicaEnLista.
func TestCA809_AplicaEnQueNoEsUnaLista(t *testing.T) {
	const ca = "CA-809"
	want := fmt.Sprintf(manifest.ErrAplicaEnLista, "mutation")
	for _, valor := range []string{
		" []",
		" null",
		" ~",
		"",
		" \"app/**\"",
		" app",
		" {app: x}",
		" [app/**, ~]",
		" [[app]]",
		" [{a: b}]",
		" [1]",
		" [app/**, true]",
	} {
		body := gprBase + "gates:\n  test:\n    required: true\n    cmd: \"true\"\n" +
			"  mutation:\n    required: true\n    cmd: \"true\"\n    aplica_en:" + valor + "\n"
		gprRechaza(t, ca, "aplica_en:"+valor, body, want)
	}
}

// CA-809: un elemento que no es un patron valido (la sintaxis de rutas:
// vacio, solo blancos, un segmento vacio, un [ sin cerrar, un \ al final) es
// ErrAplicaEnPatron, con el elemento citado como lo cita %q.
func TestCA809_AplicaEnConUnPatronInvalido(t *testing.T) {
	const ca = "CA-809"
	for _, c := range []struct{ valor, patron string }{
		{`["a//b"]`, "a//b"},
		{`["["]`, "["},
		{`[app/**, ""]`, ""},
		{`["  "]`, "  "},
		{`["/app"]`, "/app"},
		{`["app/"]`, "app/"},
		{`['x\']`, `x\`},
		{`['a"b[']`, `a"b[`},
	} {
		body := gprBase + "gates:\n  mutation:\n    required: true\n    cmd: \"true\"\n    aplica_en: " + c.valor + "\n"
		gprRechaza(t, ca, "aplica_en: "+c.valor, body, fmt.Sprintf(manifest.ErrAplicaEnPatron, "mutation", c.patron))
	}
}

// CA-809: una clave de gates.<g> fuera de GateKeys es ErrGateClaveDesconocida:
// tambien `rutas` (la clave es aplica_en, decision del spec) y una mal
// escrita; el gate se nombra como esta escrito.
func TestCA809_ClaveDesconocidaDeUnGate(t *testing.T) {
	const ca = "CA-809"
	for _, c := range []struct{ gate, linea, clave string }{
		{"mutation", "    aplicaen: [app/**]\n", "aplicaen"},
		{"mutation", "    Aplica_En: [app/**]\n", "Aplica_En"},
		{"mutation", "    rutas: [app/**]\n", "rutas"},
		{"static", "    description: el analizador\n", "description"},
	} {
		body := gprBase + "gates:\n  " + c.gate + ":\n    required: true\n    cmd: \"true\"\n" + c.linea
		gprRechaza(t, ca, c.clave, body, fmt.Sprintf(manifest.ErrGateClaveDesconocida, c.gate, c.clave))
	}
}

// ---------------------------------------------------------------- como hoy

// CA-809: un hoom.yaml sin aplica_en, con las cinco claves de siempre, carga
// igual que hoy: cada clave como esta escrita y AplicaEn nil en todos.
func TestCA809_SinAplicaEnCargaComoHoy(t *testing.T) {
	const ca = "CA-809"
	body := gprBase + "gates:\n" +
		"  test:\n    required: true\n    cmd: \"go test ./...\"\n" +
		"  mutation:\n    required: false\n    cmd: \"infection\"\n    diff_cmd: \"infection --git-diff-base={base}\"\n" +
		"    notes: \"lento\"\n    partial: \"solo app/\"\n" +
		"  lint:\n    cmd: \"\"\n"
	dir := gprDir(t, body)
	for _, c := range gprCargadores {
		m, err := c.cargar(dir, nil)
		if err != nil {
			t.Fatalf("%s: %s carga un hoom.yaml sin aplica_en: %v", ca, c.nombre, err)
		}
		if len(m.Gates) != 3 {
			t.Fatalf("%s: %s: tres gates: %+v", ca, c.nombre, m.Gates)
		}
		mut := m.Gates["mutation"]
		if mut.IsRequired() || mut.CmdStr() != "infection" || mut.DiffCmdStr() != "infection --git-diff-base={base}" ||
			mut.Notes != "lento" || mut.Partial != "solo app/" {
			t.Fatalf("%s: %s: las claves de siempre se leen como hoy: %+v", ca, c.nombre, mut)
		}
		if g := m.Gates["lint"]; g.Cmd == nil || g.CmdStr() != "" {
			t.Fatalf("%s: %s: cmd \"\" sigue declarando el gate ausente: %+v", ca, c.nombre, g)
		}
		for n, g := range m.Gates {
			if g.AplicaEn != nil {
				t.Fatalf("%s: %s: sin aplica_en, %s queda con AplicaEn nil: %q", ca, c.nombre, n, g.AplicaEn)
			}
		}
	}
}

// CA-809: los cuatro perfiles embebidos (go, kmp, kmp-compose, laravel)
// cargan: Resolve sin error y un hoom.yaml que solo los nombra carga con sus
// gates, ninguno con aplica_en (sin aplica_en, como hoy).
func TestCA809_LosCuatroPerfilesEmbebidosCargan(t *testing.T) {
	const ca = "CA-809"
	nombres := profiles.Names()
	hay := map[string]bool{}
	for _, n := range nombres {
		hay[n] = true
	}
	for _, n := range []string{"go", "kmp", "kmp-compose", "laravel"} {
		if !hay[n] {
			t.Fatalf("%s: el perfil embebido %q esta en profiles.Names(): %q", ca, n, nombres)
		}
	}
	for _, n := range nombres {
		gates, _, err := profiles.Resolve(n)
		if err != nil {
			t.Fatalf("%s: el perfil embebido %s carga: %v", ca, n, err)
		}
		if len(gates) == 0 {
			t.Fatalf("%s: el perfil %s trae gates", ca, n)
		}
		dir := gprDir(t, gprBase+"profile: "+n+"\n")
		for _, c := range gprCargadores {
			m, err := c.cargar(dir, profiles.Resolve)
			if err != nil {
				t.Fatalf("%s: %s de un hoom.yaml con profile: %s: %v", ca, c.nombre, n, err)
			}
			if len(m.Gates) == 0 {
				t.Fatalf("%s: %s: con profile: %s hay gates: %+v", ca, c.nombre, n, m)
			}
			for g, gate := range m.Gates {
				if gate.AplicaEn != nil {
					t.Fatalf("%s: %s: el gate %s del perfil %s carga sin aplica_en: %q", ca, c.nombre, g, n, gate.AplicaEn)
				}
			}
		}
	}
}
