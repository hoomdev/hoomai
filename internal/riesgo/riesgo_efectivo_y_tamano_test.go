// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// el paquete riesgo: la clase de cada archivo y el tamano en tres numeros
// (CA-774), los patrones de `rutas` (CA-777), las rutas por defecto (CA-778)
// y las 8 reglas del riesgo efectivo (CA-779), con sus textos exactos.
// Escritos desde el spec, las aclaraciones de la ronda 0 y las firmas
// publicas del esqueleto, sin leer la implementacion.
//
// El paquete riesgo no importa ningun otro paquete de hoom, asi que aqui el
// territorio del test-writer va escrito a mano; los valores por defecto
// reales (agents.AllowPorDefecto, los perfiles) se prueban en profiles.
package riesgo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/quick"
)

// retyMarcas son las marcas del spec, escritas a mano: las de hoy con `sign`
// cambiada por `signin` y `signature`, en su lugar.
var retyMarcas = []string{"auth", "login", "password", "passwd", "secret", "token", "cred",
	"crypt", "signin", "signature", "pago", "payment", "billing", "invoice", "dte",
	"permission", "permiso", "sandbox", "sudo"}

// retyBajoBase es la columna de_bajo_riesgo de la fila base, en su orden.
var retyBajoBase = []string{"**/*.md", "**/*.txt", "**/*.rst", "**/*.adoc", "**/docs", "**/LICENSE", "**/NOTICE"}

// retyTerritorio es un territorio de test-writer escrito a mano, con
// `.hoom/**` como el valor por defecto (un archivo bajo .hoom/ es hoom antes
// que prueba).
var retyTerritorio = []string{".hoom/**", "tests/**", "**/*_test.go", "**/*Test.php"}

// retyClasificador es un clasificador con las rutas base, generadas `vendor`
// y `go.sum`, y retyTerritorio.
func retyClasificador() Clasificador {
	r := RutasBase()
	r.Generadas = []string{"vendor", "go.sum"}
	return Clasificador{Rutas: r, Pruebas: append([]string(nil), retyTerritorio...)}
}

func retyTarea(riesgo string) Tarjeta { return Tarjeta{Tarea: "pagos", Riesgo: riesgo} }

func retyEval(t *testing.T, ca string, c Clasificador, tj Tarjeta, files []string, decl, ef, mot string) {
	t.Helper()
	got := c.Evaluar(tj, files)
	want := Evaluacion{Declarado: decl, Efectivo: ef, Motivo: mot}
	if got != want {
		t.Fatalf("%s: Evaluar(%+v, %q) = %+v, quiero %+v", ca, tj, files, got, want)
	}
}

// ---------------------------------------------------------------- CA-774

// CA-774: la clase es la primera que toca, en orden: hoom (bajo .hoom/, un
// _test.go incluido), generado, prueba (el territorio, ruta entera y con
// mayusculas, sin regla de padre) y producto (lo demas, hoom.yaml incluido).
func TestCA774_LaClaseEsLaPrimeraQueToca(t *testing.T) {
	const ca = "CA-774"
	c := retyClasificador()
	for _, k := range []struct{ ruta, clase string }{
		{".hoom/x_test.go", ClaseHoom},
		{".hoom/verdicts/20261120T100000_abcdef.json", ClaseHoom},
		{".hoom/vendor/a.go", ClaseHoom},
		{".hoom/specs/auth.md", ClaseHoom},
		{"vendor/x_test.go", ClaseGenerado},
		{"vendor/a/b.go", ClaseGenerado},
		{"go.sum", ClaseGenerado},
		{"internal/app_test.go", ClasePrueba},
		{"a_test.go", ClasePrueba},
		{"tests/Feature/PagoTest.php", ClasePrueba},
		{"app/UserTest.php", ClasePrueba},
		{"latest.go", ClaseProducto},
		{"internal/testutil/x.go", ClaseProducto},
		{"Tests/a.php", ClaseProducto},
		{"x/tests/a.go", ClaseProducto},
		{"hoom.yaml", ClaseProducto},
		{".hoomx/a.go", ClaseProducto},
		{"vendors/a.go", ClaseProducto},
		{"internal/app.go", ClaseProducto},
		{"docs/a.md", ClaseProducto},
	} {
		if got := c.Clase(k.ruta); got != k.clase {
			t.Fatalf("%s: Clase(%q) = %q, quiero %q", ca, k.ruta, got, k.clase)
		}
	}
	// Con allow, `prueba` es ese territorio y nada mas: la ruta entera, sin
	// la regla del directorio padre de `rutas`.
	solo := Clasificador{Pruebas: []string{"pruebas/**", "spec"}}
	for _, k := range []struct{ ruta, clase string }{
		{"pruebas/a.go", ClasePrueba},
		{"pruebas/x/y_test.go", ClasePrueba},
		{"a_test.go", ClaseProducto},
		{"spec", ClasePrueba},
		{"spec/a.go", ClaseProducto},
		{"tests/a.go", ClaseProducto},
		{"Pruebas/a.go", ClaseProducto},
	} {
		if got := solo.Clase(k.ruta); got != k.clase {
			t.Fatalf("%s: con allow [pruebas/**, spec], Clase(%q) = %q, quiero %q", ca, k.ruta, got, k.clase)
		}
	}
	// El Clasificador cero: todo lo de fuera de .hoom/ es producto.
	var cero Clasificador
	for _, k := range []struct{ ruta, clase string }{
		{".hoom/a.json", ClaseHoom},
		{"vendor/a.go", ClaseProducto},
		{"a_test.go", ClaseProducto},
		{"go.sum", ClaseProducto},
	} {
		if got := cero.Clase(k.ruta); got != k.clase {
			t.Fatalf("%s: el Clasificador cero da Clase(%q) = %q, quiero %q", ca, k.ruta, got, k.clase)
		}
	}
	if ClaseHoom != "hoom" || ClaseGenerado != "generado" || ClasePrueba != "prueba" || ClaseProducto != "producto" {
		t.Fatalf("%s: las clases: %q %q %q %q", ca, ClaseHoom, ClaseGenerado, ClasePrueba, ClaseProducto)
	}
}

// CA-774: 90 lineas de producto y 1,500 de pruebas dan producto 90 y pruebas
// 1500; lo de .hoom/ suma a hoom, tambien un _test.go; un generado no suma a
// ninguno; una ruta vacia se deja fuera; Medir no cambia el mapa.
func TestCA774_Medir90De1500(t *testing.T) {
	const ca = "CA-774"
	c := retyClasificador()
	lineas := map[string]int{
		"internal/app.go":            60,
		"hoom.yaml":                  30,
		"internal/app_test.go":       1400,
		"tests/Feature/PagoTest.php": 100,
		".hoom/verdicts/a.json":      25,
		".hoom/x_test.go":            7,
		"vendor/lib/a.go":            999,
		"go.sum":                     50,
		"":                           13,
	}
	copia := map[string]int{}
	for k, v := range lineas {
		copia[k] = v
	}
	got := c.Medir(lineas)
	if got != (Tamano{Producto: 90, Pruebas: 1500, Hoom: 32}) {
		t.Fatalf("%s: Medir = %+v, quiero producto 90, pruebas 1500, hoom 32", ca, got)
	}
	if !reflect.DeepEqual(lineas, copia) {
		t.Fatalf("%s: Medir cambio el mapa: %v", ca, lineas)
	}
	if l := got.Linea(); l != "tamano: producto 90 · pruebas 1500 · hoom 32" {
		t.Fatalf("%s: la linea del tamano: %q", ca, l)
	}
	if got := c.Medir(nil); got != (Tamano{}) {
		t.Fatalf("%s: Medir(nil) es cero: %+v", ca, got)
	}
	// Un archivo de 0 lineas (un binario) no suma.
	if got := c.Medir(map[string]int{"logo.png": 0, "a.go": 3}); got != (Tamano{Producto: 3}) {
		t.Fatalf("%s: un binario suma 0: %+v", ca, got)
	}
}

// CA-774: el JSON de Tamano y de Evaluacion usa las claves del spec.
func TestCA774_JSONDeTamanoYEvaluacion(t *testing.T) {
	const ca = "CA-774"
	b, err := json.Marshal(Tamano{Producto: 90, Pruebas: 1500, Hoom: 3})
	if err != nil || string(b) != `{"producto":90,"pruebas":1500,"hoom":3}` {
		t.Fatalf("%s: el JSON de tamano: %s %v", ca, b, err)
	}
	b, err = json.Marshal(Evaluacion{Declarado: "bajo", Efectivo: "alto", Motivo: "declarado alto"})
	if err != nil || string(b) != `{"declarado":"bajo","efectivo":"alto","motivo":"declarado alto"}` {
		t.Fatalf("%s: el JSON de riesgo: %s %v", ca, b, err)
	}
}

// retyPieza es una ruta con su clase y sus marcas, decididas a mano: el
// oraculo de las propiedades no llama a la funcion bajo prueba.
type retyPieza struct {
	ruta   string
	clase  string // "" = ruta vacia
	riesgo bool   // coincide con RutasBase().DeRiesgo
	bajo   bool   // coincide con RutasBase().DeBajoRiesgo
}

// retyPiezas, para retyClasificador.
var retyPiezas = []retyPieza{
	{"main.go", ClaseProducto, false, false},
	{"docs/a.md", ClaseProducto, false, true},
	{"docs/sub/x.go", ClaseProducto, false, true},
	{"README.md", ClaseProducto, false, true},
	{"LICENSE", ClaseProducto, false, true},
	{"hoom.yaml", ClaseProducto, false, false},
	{"x/DESIGN.kt", ClaseProducto, false, false},
	{"assign.go", ClaseProducto, false, false},
	{"ñandú/árbol.go", ClaseProducto, false, false},
	{"internal/auth/x.go", ClaseProducto, true, false},
	{"app/Http/Auth/Login.php", ClaseProducto, true, false},
	{"B/sudo.go", ClaseProducto, true, false},
	{"signature.go", ClaseProducto, true, false},
	{"docs/secret.md", ClaseProducto, true, true},
	{"Token.TXT", ClaseProducto, true, true},
	{"internal/spec/tokens_test.go", ClasePrueba, true, false},
	{"a_test.go", ClasePrueba, false, false},
	{"tests/Pago.php", ClasePrueba, true, false},
	{".hoom/specs/auth.md", ClaseHoom, true, true},
	{".hoom/x_test.go", ClaseHoom, false, false},
	{"vendor/token.go", ClaseGenerado, true, false},
	{"go.sum", ClaseGenerado, false, false},
	{"", "", false, false},
}

func retyPiezaDe(ruta string) retyPieza {
	for _, p := range retyPiezas {
		if p.ruta == ruta {
			return p
		}
	}
	panic("pieza desconocida " + ruta)
}

type retyCambio struct {
	Lineas map[string]int
}

func (retyCambio) Generate(r *rand.Rand, _ int) reflect.Value {
	m := map[string]int{}
	for i, n := 0, r.Intn(len(retyPiezas)+1); i < n; i++ {
		m[retyPiezas[r.Intn(len(retyPiezas))].ruta] = r.Intn(2000)
	}
	return reflect.ValueOf(retyCambio{Lineas: m})
}

// CA-774 (propiedad): Medir suma las lineas de cada archivo al numero de su
// clase, y las de un generado o una ruta vacia a ninguno; la suma de los tres
// es el total menos los generados.
func TestCA774_PropiedadMedirSumaPorClase(t *testing.T) {
	const ca = "CA-774"
	c := retyClasificador()
	prop := func(e retyCambio) bool {
		var want Tamano
		for ruta, n := range e.Lineas {
			switch retyPiezaDe(ruta).clase {
			case ClaseProducto:
				want.Producto += n
			case ClasePrueba:
				want.Pruebas += n
			case ClaseHoom:
				want.Hoom += n
			}
		}
		got := c.Medir(e.Lineas)
		if got != want {
			t.Logf("%v: %+v, quiero %+v", e.Lineas, got, want)
			return false
		}
		for ruta := range e.Lineas {
			if ruta != "" && c.Clase(ruta) != retyPiezaDe(ruta).clase {
				t.Logf("Clase(%q) = %q", ruta, c.Clase(ruta))
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 1000}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}

// ---------------------------------------------------------------- CA-777

// CA-777: un patron coincide con la ruta o con un directorio padre, sin
// distinguir mayusculas; `*`, `?` y `[...]` dentro de un segmento, `**` cero
// o mas segmentos.
func TestCA777_CoincideConLaRutaOUnPadre(t *testing.T) {
	const ca = "CA-777"
	for _, k := range []struct {
		patron, ruta string
		want         bool
	}{
		// los ejemplos del spec
		{"**/*auth*", "internal/oauth.go", true},
		{"**/*auth*", "app/Http/Auth/Login.php", true},
		{"vendor", "vendor/a/b.go", true},
		// el directorio padre
		{"vendor", "vendor", true},
		{"**/docs", "docs/a.md", true},
		{"**/docs", "x/docs/y/z.md", true},
		{"app/Domain/Costeo", "app/domain/costeo/Factura.php", true},
		{"**/*.md", "notas.md/x.go", true},
		{"lang", "lang/es/mensajes.php", true},
		{"resources/lang", "resources/lang/es.json", true},
		// sin mayusculas, en el patron y en la ruta
		{"VENDOR", "vendor/a.go", true},
		{"**/*.MD", "x/readme.md", true},
		{"**/*tenant*", "src/MultiTenant/a.go", true},
		{"**/LICENSE", "license", true},
		// `**` con cero segmentos, y varios seguidos son uno
		{"**/*.md", "README.md", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/**/b", "a/b", true},
		{"**/gradle.lockfile", "gradle.lockfile", true},
		{"**/gradle.lockfile", "app/gradle.lockfile", true},
		{"**", "a/b/c.go", true},
		// `?` y `[...]` dentro de un segmento
		{"a?c", "abc", true},
		{"[ab]x", "bx/y.go", true},
		{"kotlin-js-store/yarn.lock", "kotlin-js-store/yarn.lock", true},
		// lo que no coincide
		{"vendor", "a/vendor/b.go", false},
		{"vendor", "vendors/a.go", false},
		{"vendor", "myvendor", false},
		{"*.md", "docs/a.md", false},
		{"a/*/c", "a/b/x/c", false},
		{"**/docs", "docsx/a.md", false},
		{"**/docs", "mydocs/a.md", false},
		{"internal/auth", "internal/authx/a.go", false},
		{"a?c", "ac", false},
		{"a?c", "a/c", false},
		{"[ab]x", "cx", false},
		{"composer.lock", "a/composer.lock", false},
		{"kotlin-js-store/yarn.lock", "yarn.lock", false},
		{"**/*auth*", "internal/app.go", false},
	} {
		if got := Coincide(k.patron, k.ruta); got != k.want {
			t.Fatalf("%s: Coincide(%q, %q) = %v, quiero %v", ca, k.patron, k.ruta, got, k.want)
		}
	}
}

// CA-777: un patron invalido no coincide con nada, ni consigo mismo.
func TestCA777_UnPatronInvalidoNoCoincide(t *testing.T) {
	const ca = "CA-777"
	for _, k := range []struct{ patron, ruta string }{
		{"", ""}, {"", "a.go"}, {"  ", "  "}, {"[", "["}, {"[", "[/a.go"},
		{"a//b", "a//b"}, {"a//b", "a/b"}, {"/a", "/a"}, {"/a", "a"}, {"a/", "a/"}, {"a/", "a/b"},
		{`a\`, `a\`}, {"a/[b", "a/[b"},
	} {
		if Coincide(k.patron, k.ruta) {
			t.Fatalf("%s: el patron invalido %q no coincide con %q", ca, k.patron, k.ruta)
		}
	}
	if CoincideAlguno(nil, "a.go") || CoincideAlguno([]string{}, "a.go") || CoincideAlguno([]string{"["}, "[") {
		t.Fatalf("%s: CoincideAlguno es falso sin patrones o con uno invalido", ca)
	}
	if !CoincideAlguno([]string{"[", "", "vendor"}, "vendor/a.go") {
		t.Fatalf("%s: un patron invalido no tapa a uno valido de la lista", ca)
	}
}

// CA-777 y aclaracion 5: PatronValido es falso para un patron vacio o en
// blanco, uno con un segmento mal formado y uno con un segmento vacio.
func TestCA777_PatronValido(t *testing.T) {
	const ca = "CA-777"
	for _, p := range []string{"", " ", "\t", "  \n", "[", "a/[b", "[a-", `a\`, `a/b\`, "a//b", "/a", "a/", "/", "//"} {
		if PatronValido(p) {
			t.Fatalf("%s: %q no es un patron valido", ca, p)
		}
	}
	for _, p := range []string{"**", "*", "vendor", "**/*auth*", "a/**/b", "[ab]x", "a?c", "app/Domain/Costeo",
		"**/*.md", "kotlin-js-store/yarn.lock", "**/gradle.lockfile", "**/*tenant*"} {
		if !PatronValido(p) {
			t.Fatalf("%s: %q es un patron valido", ca, p)
		}
	}
	for _, p := range append(append([]string(nil), retyBajoBase...), "go.sum", "public/build", "node_modules") {
		if !PatronValido(p) {
			t.Fatalf("%s: el patron por defecto %q es valido", ca, p)
		}
	}
}

type retyPar struct {
	Patron, Dir string
}

func (retyPar) Generate(r *rand.Rand, _ int) reflect.Value {
	segs := []string{"a", "B", "docs", "Auth", "vendor", "x.md", "ñ", "*", "?", "**", "*auth*", "*.md"}
	dirs := []string{"a", "B", "docs", "Auth", "vendor", "x.md", "ñ", "oauth", "lang", "README.md"}
	n := 1 + r.Intn(3)
	var ps, ds []string
	for i := 0; i < n; i++ {
		ps = append(ps, segs[r.Intn(len(segs))])
	}
	for i, m := 0, 1+r.Intn(3); i < m; i++ {
		ds = append(ds, dirs[r.Intn(len(dirs))])
	}
	return reflect.ValueOf(retyPar{Patron: strings.Join(ps, "/"), Dir: strings.Join(ds, "/")})
}

// CA-777 (propiedad): si un patron coincide con una ruta, coincide con todo
// lo que hay debajo (la regla del padre); y la respuesta no cambia con las
// mayusculas de la ruta ni del patron.
func TestCA777_PropiedadPadreYMayusculas(t *testing.T) {
	const ca = "CA-777"
	prop := func(e retyPar) bool {
		got := Coincide(e.Patron, e.Dir)
		if got && !Coincide(e.Patron, e.Dir+"/hijo.go") {
			t.Logf("%q cubre %q y no %q", e.Patron, e.Dir, e.Dir+"/hijo.go")
			return false
		}
		if Coincide(strings.ToUpper(e.Patron), e.Dir) != got || Coincide(e.Patron, strings.ToUpper(e.Dir)) != got ||
			Coincide(strings.ToLower(e.Patron), strings.ToUpper(e.Dir)) != got {
			t.Logf("%q contra %q cambia con las mayusculas", e.Patron, e.Dir)
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}

// ---------------------------------------------------------------- CA-778

// CA-778: las marcas cambian `sign` por `signin` y `signature`, en su lugar.
func TestCA778_Marcas(t *testing.T) {
	if !reflect.DeepEqual(Marcas, retyMarcas) {
		t.Fatalf("CA-778: las marcas son %v, fueron %v", retyMarcas, Marcas)
	}
}

// CA-778: RutasBase es la fila base de la tabla, y cada llamada da listas
// nuevas.
func TestCA778_RutasBase(t *testing.T) {
	const ca = "CA-778"
	r := RutasBase()
	var riesgo []string
	for _, m := range retyMarcas {
		riesgo = append(riesgo, "**/*"+m+"*")
	}
	if !reflect.DeepEqual(r.DeRiesgo, riesgo) {
		t.Fatalf("%s: de_riesgo base es **/*<m>* por cada marca, en orden: %q", ca, r.DeRiesgo)
	}
	if !reflect.DeepEqual(r.DeBajoRiesgo, retyBajoBase) {
		t.Fatalf("%s: de_bajo_riesgo base: %q", ca, r.DeBajoRiesgo)
	}
	if len(r.Generadas) != 0 {
		t.Fatalf("%s: la base no tiene generadas: %q", ca, r.Generadas)
	}
	r.DeRiesgo[0] = "cambiado"
	r.DeBajoRiesgo[0] = "cambiado"
	r.Generadas = append(r.Generadas, "cambiado")
	otra := RutasBase()
	if otra.DeRiesgo[0] != "**/*auth*" || otra.DeBajoRiesgo[0] != "**/*.md" || len(otra.Generadas) != 0 {
		t.Fatalf("%s: cambiar lo que dio RutasBase cambio la siguiente llamada: %+v", ca, otra)
	}
}

// CA-778: sin la seccion rutas, design.md, assign.go y signal.go no son rutas
// de riesgo, y signature.go, SignIn.php e internal/auth/x.go si.
func TestCA778_SinRutasLasMarcasNuevas(t *testing.T) {
	const ca = "CA-778"
	base := RutasBase().DeRiesgo
	for _, f := range []string{"design.md", "assign.go", "signal.go", "src/DESIGN.kt", "internal/assignment/a.go",
		"signals/a.go", "docs/design/x.go", "main.go", "README.md"} {
		if CoincideAlguno(base, f) {
			t.Fatalf("%s: %q no es ruta de riesgo sin la seccion rutas", ca, f)
		}
	}
	for _, f := range []string{"signature.go", "SignIn.php", "internal/auth/x.go", "app/Http/Auth/Login.php",
		"internal/oauth.go", "x/sudoers", "Token.TXT", "internal/spec/tokens_test.go", "app/Pagos/Factura.php",
		"src/SIGNATURE/a.kt", "lib/Crypto.kt", "sandbox/a.go", "dte/x.php"} {
		if !CoincideAlguno(base, f) {
			t.Fatalf("%s: %q es ruta de riesgo sin la seccion rutas", ca, f)
		}
	}
	c := Clasificador{Rutas: RutasBase()}
	retyEval(t, ca, c, retyTarea(""), []string{"design.md", "assign.go", "signal.go"}, "normal", "normal", "declarado normal")
	retyEval(t, ca, c, retyTarea(""), []string{"design.md", "signature.go"}, "normal", "alto", "el cambio toca signature.go: riesgo alto")
	retyEval(t, ca, c, retyTarea(""), []string{"SignIn.php"}, "normal", "alto", "el cambio toca SignIn.php: riesgo alto")
}

// CA-778: un archivo hoom o generado con una marca no sube el riesgo, ni con
// normal ni con bajo.
func TestCA778_HoomYGeneradoNoSuben(t *testing.T) {
	const ca = "CA-778"
	c := retyClasificador()
	files := []string{".hoom/specs/auth.md", ".hoom/items/password.yaml", "vendor/x/token.go", "vendor/auth.go"}
	retyEval(t, ca, c, retyTarea("normal"), files, "normal", "normal", "declarado normal")
	retyEval(t, ca, c, retyTarea("bajo"), files, "bajo", "bajo", "declarado bajo")
	retyEval(t, ca, c, Tarjeta{}, files, "normal", "normal", "sin tarjeta: riesgo normal")
	// Un .hoom/ que tambien esta en el territorio (.hoom/**) sigue siendo hoom.
	retyEval(t, ca, c, retyTarea("bajo"), []string{".hoom/auth_test.go"}, "bajo", "bajo", "declarado bajo")
}

// ---------------------------------------------------------------- CA-779

// CA-779: los motivos y las constantes, con su texto exacto.
func TestCA779_ConstantesExactas(t *testing.T) {
	const ca = "CA-779"
	for _, k := range [][2]string{
		{MotivoIlegible, "no se pudo leer la tarjeta: riesgo alto"},
		{MotivoSeguridad, "tarjeta de tipo seguridad: riesgo alto"},
		{MotivoDeclarado, "declarado %s"},
		{MotivoRutaDeRiesgo, "el cambio toca %s: riesgo alto"},
		{MotivoFueraDeBajo, "el cambio toca %s, fuera de las rutas de bajo riesgo: riesgo normal"},
		{MotivoSinTarjeta, "sin tarjeta: riesgo normal"},
		{TipoSeguridad, "seguridad"},
		{LineaRiesgo, "riesgo: %s (%s)"},
		{LineaTamano, "tamano: producto %d · pruebas %d · hoom %d"},
		{AvisoMayorQueDeclarado, "! el riesgo efectivo (%s) es mayor que el declarado (%s): hoom item set %s --riesgo %s lo sube"},
		{AvisoProductoGrande, "! mas de 400 lineas de producto: la review exige 2 lentes (risk y reliability)"},
	} {
		if k[0] != k[1] {
			t.Fatalf("%s: la constante es %q, fue %q", ca, k[1], k[0])
		}
	}
	if UmbralProducto != 400 {
		t.Fatalf("%s: UmbralProducto es 400: %d", ca, UmbralProducto)
	}
	if got := fmt.Sprintf(AvisoMayorQueDeclarado, "alto", "normal", "pagos", "alto"); got !=
		"! el riesgo efectivo (alto) es mayor que el declarado (normal): hoom item set pagos --riesgo alto lo sube" {
		t.Fatalf("%s: el aviso: %q", ca, got)
	}
	if got := (Evaluacion{Declarado: "bajo", Efectivo: "alto", Motivo: "el cambio toca a/auth.go: riesgo alto"}).Linea(); got !=
		"riesgo: alto (el cambio toca a/auth.go: riesgo alto)" {
		t.Fatalf("%s: la linea del riesgo: %q", ca, got)
	}
}

// CA-779: cada una de las 8 reglas, con su motivo exacto y su declarado.
func TestCA779_LasOchoReglas(t *testing.T) {
	const ca = "CA-779"
	c := retyClasificador()
	riesgosa := []string{"main.go", "internal/auth/x.go"}
	for _, k := range []struct {
		nombre        string
		tj            Tarjeta
		files         []string
		decl, ef, mot string
	}{
		// 1: hay tarea y la tarjeta no se puede leer; gana sobre todo lo demas.
		{"1 ilegible", Tarjeta{Tarea: "t", Ilegible: true}, nil, "normal", "alto", MotivoIlegible},
		{"1 ilegible bajo", Tarjeta{Tarea: "t", Ilegible: true, Riesgo: "bajo"}, []string{"docs/a.md"}, "normal", "alto", MotivoIlegible},
		{"1 ilegible sobre seguridad y alto", Tarjeta{Tarea: "t", Ilegible: true, Tipo: "seguridad", Riesgo: "alto"}, riesgosa, "normal", "alto", MotivoIlegible},
		{"1 riesgo fuera del vocabulario", Tarjeta{Tarea: "t", Riesgo: "medio"}, nil, "normal", "alto", MotivoIlegible},
		{"1 riesgo Alto con mayuscula", Tarjeta{Tarea: "t", Riesgo: "Alto"}, []string{"docs/a.md"}, "normal", "alto", MotivoIlegible},
		// sin tarea, Ilegible no se lee
		{"7 ilegible sin tarea", Tarjeta{Ilegible: true}, []string{"main.go"}, "normal", "normal", MotivoSinTarjeta},
		// 2: tipo seguridad
		{"2 seguridad bajo", Tarjeta{Tarea: "t", Tipo: "seguridad", Riesgo: "bajo"}, []string{"docs/a.md"}, "bajo", "alto", MotivoSeguridad},
		{"2 seguridad sin archivos", Tarjeta{Tarea: "t", Tipo: TipoSeguridad}, nil, "normal", "alto", MotivoSeguridad},
		{"2 seguridad sobre alto y ruta", Tarjeta{Tarea: "t", Tipo: "seguridad", Riesgo: "alto"}, riesgosa, "alto", "alto", MotivoSeguridad},
		{"7 seguridad sin tarea", Tarjeta{Tipo: "seguridad"}, nil, "normal", "normal", MotivoSinTarjeta},
		// 3: declara alto, gana sobre la ruta
		{"3 alto con ruta", retyTarea("alto"), riesgosa, "alto", "alto", "declarado alto"},
		{"3 alto sin archivos", retyTarea("alto"), nil, "alto", "alto", "declarado alto"},
		// 4: una ruta de riesgo en producto o prueba
		{"4 normal", retyTarea("normal"), riesgosa, "normal", "alto", "el cambio toca internal/auth/x.go: riesgo alto"},
		{"4 vacio", retyTarea(""), riesgosa, "normal", "alto", "el cambio toca internal/auth/x.go: riesgo alto"},
		{"4 bajo", retyTarea("bajo"), riesgosa, "bajo", "alto", "el cambio toca internal/auth/x.go: riesgo alto"},
		{"4 sin tarea", Tarjeta{}, riesgosa, "normal", "alto", "el cambio toca internal/auth/x.go: riesgo alto"},
		{"4 solo una prueba", retyTarea("normal"), []string{"internal/spec/tokens_test.go"}, "normal", "alto", "el cambio toca internal/spec/tokens_test.go: riesgo alto"},
		{"4 prueba con bajo", retyTarea("bajo"), []string{"docs/a.md", "internal/spec/tokens_test.go"}, "bajo", "alto", "el cambio toca internal/spec/tokens_test.go: riesgo alto"},
		{"4 doc de riesgo con bajo", retyTarea("bajo"), []string{"docs/secret.md"}, "bajo", "alto", "el cambio toca docs/secret.md: riesgo alto"},
		{"4 gana sobre 5", retyTarea("bajo"), []string{"a.go", "z/auth.go"}, "bajo", "alto", "el cambio toca z/auth.go: riesgo alto"},
		// 5: bajo con un producto fuera de de_bajo_riesgo
		{"5", retyTarea("bajo"), []string{"main.go", "docs/a.md"}, "bajo", "normal", "el cambio toca main.go, fuera de las rutas de bajo riesgo: riesgo normal"},
		{"5 hoom.yaml es producto", retyTarea("bajo"), []string{"hoom.yaml"}, "bajo", "normal", "el cambio toca hoom.yaml, fuera de las rutas de bajo riesgo: riesgo normal"},
		{"5 solo mira producto", retyTarea("bajo"), []string{"a_test.go", "b.go"}, "bajo", "normal", "el cambio toca b.go, fuera de las rutas de bajo riesgo: riesgo normal"},
		{"5 design ya no es riesgo", retyTarea("bajo"), []string{"x/DESIGN.kt"}, "bajo", "normal", "el cambio toca x/DESIGN.kt, fuera de las rutas de bajo riesgo: riesgo normal"},
		// 6: bajo que solo toca documentacion, pruebas, .hoom/ o generados
		{"6 docs", retyTarea("bajo"), []string{"docs/a.md", "README.md", "LICENSE", "docs/sub/x.go", "notas.txt"}, "bajo", "bajo", "declarado bajo"},
		{"6 pruebas y hoom", retyTarea("bajo"), []string{"a_test.go", ".hoom/specs/x.md", ".hoom/verdicts/a.json", "vendor/y.go", "go.sum"}, "bajo", "bajo", "declarado bajo"},
		{"6 sin archivos", retyTarea("bajo"), nil, "bajo", "bajo", "declarado bajo"},
		{"6 ruta vacia fuera", retyTarea("bajo"), []string{"", "docs/a.md"}, "bajo", "bajo", "declarado bajo"},
		// 7: sin tarea; el Riesgo de la tarjeta no se lee
		{"7", Tarjeta{}, []string{"main.go"}, "normal", "normal", MotivoSinTarjeta},
		{"7 sin archivos", Tarjeta{}, nil, "normal", "normal", MotivoSinTarjeta},
		{"7 bajo sin tarea", Tarjeta{Riesgo: "bajo"}, []string{"docs/a.md"}, "normal", "normal", MotivoSinTarjeta},
		{"7 ruta vacia", Tarjeta{}, []string{""}, "normal", "normal", MotivoSinTarjeta},
		// 8: lo demas
		{"8 normal", retyTarea("normal"), []string{"main.go", "docs/a.md"}, "normal", "normal", "declarado normal"},
		{"8 vacio", retyTarea(""), []string{"main.go"}, "normal", "normal", "declarado normal"},
		{"8 sin archivos", retyTarea(""), nil, "normal", "normal", "declarado normal"},
		{"8 generado y hoom con marca", retyTarea(""), []string{"vendor/token.go", ".hoom/specs/auth.md"}, "normal", "normal", "declarado normal"},
	} {
		got := c.Evaluar(k.tj, k.files)
		if got != (Evaluacion{Declarado: k.decl, Efectivo: k.ef, Motivo: k.mot}) {
			t.Fatalf("%s: regla %s: Evaluar(%+v, %q) = %+v, quiero {%s %s %q}", ca, k.nombre, k.tj, k.files, got, k.decl, k.ef, k.mot)
		}
	}
}

// CA-779: <ruta> es el primer archivo que cumple la condicion en orden de
// bytes, tal como se dio, y solo entre los de su clase.
func TestCA779_LaRutaEsLaPrimeraEnOrdenDeBytes(t *testing.T) {
	const ca = "CA-779"
	c := retyClasificador()
	for _, files := range [][]string{{"a/auth.go", "B/Sudo.go"}, {"B/Sudo.go", "a/auth.go"}} {
		retyEval(t, ca, c, retyTarea("bajo"), files, "bajo", "alto", "el cambio toca B/Sudo.go: riesgo alto")
	}
	for _, files := range [][]string{{"z.go", "Y.go", "docs/a.md"}, {"docs/a.md", "Y.go", "z.go"}} {
		retyEval(t, ca, c, retyTarea("bajo"), files, "bajo", "normal", "el cambio toca Y.go, fuera de las rutas de bajo riesgo: riesgo normal")
	}
	// .hoom/ y vendor/ van antes en bytes, pero no cuentan para la regla 4.
	retyEval(t, ca, c, retyTarea("normal"), []string{"x/token.go", "vendor/auth.go", ".hoom/auth.md"}, "normal", "alto",
		"el cambio toca x/token.go: riesgo alto")
	// Una prueba de riesgo antes en bytes que un producto de riesgo.
	retyEval(t, ca, c, retyTarea("normal"), []string{"z/auth.go", "a/auth_test.go"}, "normal", "alto",
		"el cambio toca a/auth_test.go: riesgo alto")
	// Evaluar no cambia archivos.
	files := []string{"z.go", "a/auth.go", "B/sudo.go", ""}
	copia := append([]string(nil), files...)
	c.Evaluar(retyTarea("bajo"), files)
	c.Evaluar(Tarjeta{}, files)
	if !reflect.DeepEqual(files, copia) {
		t.Fatalf("%s: Evaluar cambio archivos: %q", ca, files)
	}
}

// CA-779: `rutas.de_riesgo: []` no sube nada y el veredicto sigue diciendo el
// motivo; `de_bajo_riesgo: []` hace de cualquier producto uno de fuera; el
// Clasificador cero no tiene rutas.
func TestCA779_ListasVacias(t *testing.T) {
	const ca = "CA-779"
	sinRiesgo := Clasificador{Rutas: Rutas{DeRiesgo: []string{}, DeBajoRiesgo: RutasBase().DeBajoRiesgo}}
	retyEval(t, ca, sinRiesgo, retyTarea("normal"), []string{"internal/auth/x.go"}, "normal", "normal", "declarado normal")
	retyEval(t, ca, sinRiesgo, retyTarea("bajo"), []string{"internal/auth/x.go"}, "bajo", "normal",
		"el cambio toca internal/auth/x.go, fuera de las rutas de bajo riesgo: riesgo normal")
	retyEval(t, ca, sinRiesgo, retyTarea("bajo"), []string{"docs/secret.md"}, "bajo", "bajo", "declarado bajo")
	sinBajo := Clasificador{Rutas: Rutas{DeRiesgo: RutasBase().DeRiesgo, DeBajoRiesgo: []string{}}}
	retyEval(t, ca, sinBajo, retyTarea("bajo"), []string{"docs/a.md"}, "bajo", "normal",
		"el cambio toca docs/a.md, fuera de las rutas de bajo riesgo: riesgo normal")
	var cero Clasificador
	retyEval(t, ca, cero, retyTarea("bajo"), []string{"internal/auth/x.go"}, "bajo", "normal",
		"el cambio toca internal/auth/x.go, fuera de las rutas de bajo riesgo: riesgo normal")
	retyEval(t, ca, cero, retyTarea("bajo"), []string{".hoom/a.md"}, "bajo", "bajo", "declarado bajo")
	// Rutas propias del proyecto, como las del ejemplo del spec.
	propio := Clasificador{Rutas: Rutas{DeRiesgo: []string{"app/Domain/Costeo", "**/*tenant*"}, DeBajoRiesgo: []string{"**/*.md", "lang"},
		Generadas: []string{"composer.lock", "vendor"}}}
	retyEval(t, ca, propio, retyTarea("normal"), []string{"app/Domain/Costeo/Lote.php"}, "normal", "alto",
		"el cambio toca app/Domain/Costeo/Lote.php: riesgo alto")
	retyEval(t, ca, propio, retyTarea("normal"), []string{"app/Http/Auth/Login.php"}, "normal", "normal", "declarado normal")
	retyEval(t, ca, propio, retyTarea("bajo"), []string{"lang/es/a.php", "composer.lock", "x.md"}, "bajo", "bajo", "declarado bajo")
}

type retyEntrada struct {
	Tj    Tarjeta
	Files []string
}

func (retyEntrada) Generate(r *rand.Rand, _ int) reflect.Value {
	tareas := []string{"", "pagos"}
	tipos := []string{"", "feature", "bug", "seguridad", "chore"}
	riesgos := []string{"", "bajo", "normal", "alto", "medio", "Alto"}
	tj := Tarjeta{Tarea: tareas[r.Intn(2)], Ilegible: r.Intn(6) == 0, Tipo: tipos[r.Intn(len(tipos))], Riesgo: riesgos[r.Intn(len(riesgos))]}
	var fs []string
	for i, n := 0, r.Intn(7); i < n; i++ {
		fs = append(fs, retyPiezas[r.Intn(len(retyPiezas))].ruta)
	}
	return reflect.ValueOf(retyEntrada{Tj: tj, Files: fs})
}

// retyOraculo es la tabla de 8 reglas del spec sobre las piezas decididas a
// mano.
func retyOraculo(tj Tarjeta, files []string) Evaluacion {
	tarea := tj.Tarea != ""
	vocab := tj.Riesgo == "" || tj.Riesgo == "bajo" || tj.Riesgo == "normal" || tj.Riesgo == "alto"
	ilegible := tarea && (tj.Ilegible || !vocab)
	decl := "normal"
	if tarea && !ilegible && tj.Riesgo != "" {
		decl = tj.Riesgo
	}
	orden := append([]string(nil), files...)
	sort.Strings(orden)
	switch {
	case ilegible:
		return Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: "no se pudo leer la tarjeta: riesgo alto"}
	case tarea && tj.Tipo == "seguridad":
		return Evaluacion{Declarado: decl, Efectivo: "alto", Motivo: "tarjeta de tipo seguridad: riesgo alto"}
	case decl == "alto":
		return Evaluacion{Declarado: decl, Efectivo: "alto", Motivo: "declarado alto"}
	}
	for _, f := range orden {
		p := retyPiezaDe(f)
		if (p.clase == ClaseProducto || p.clase == ClasePrueba) && p.riesgo {
			return Evaluacion{Declarado: decl, Efectivo: "alto", Motivo: "el cambio toca " + f + ": riesgo alto"}
		}
	}
	if decl == "bajo" {
		for _, f := range orden {
			p := retyPiezaDe(f)
			if p.clase == ClaseProducto && !p.bajo {
				return Evaluacion{Declarado: decl, Efectivo: "normal", Motivo: "el cambio toca " + f + ", fuera de las rutas de bajo riesgo: riesgo normal"}
			}
		}
		return Evaluacion{Declarado: decl, Efectivo: "bajo", Motivo: "declarado bajo"}
	}
	if !tarea {
		return Evaluacion{Declarado: "normal", Efectivo: "normal", Motivo: "sin tarjeta: riesgo normal"}
	}
	return Evaluacion{Declarado: decl, Efectivo: "normal", Motivo: "declarado normal"}
}

func retyNivel(r string) int {
	switch r {
	case "bajo":
		return 0
	case "normal":
		return 1
	case "alto":
		return 2
	}
	return -1
}

// CA-779 (propiedad): Evaluar es la tabla de 8 reglas; el efectivo es del
// vocabulario y nunca menor que el declarado; el motivo nunca queda vacio; la
// linea es `riesgo: <efectivo> (<motivo>)`; el orden de archivos no importa.
func TestCA779_PropiedadLaTabla(t *testing.T) {
	const ca = "CA-779"
	c := retyClasificador()
	prop := func(e retyEntrada) bool {
		got := c.Evaluar(e.Tj, e.Files)
		if want := retyOraculo(e.Tj, e.Files); got != want {
			t.Logf("Evaluar(%+v, %q) = %+v, quiero %+v", e.Tj, e.Files, got, want)
			return false
		}
		if retyNivel(got.Efectivo) < 0 || retyNivel(got.Declarado) < 0 || retyNivel(got.Efectivo) < retyNivel(got.Declarado) || got.Motivo == "" {
			return false
		}
		if got.Linea() != "riesgo: "+got.Efectivo+" ("+got.Motivo+")" {
			return false
		}
		alreves := append([]string(nil), e.Files...)
		for i, j := 0, len(alreves)-1; i < j; i, j = i+1, j-1 {
			alreves[i], alreves[j] = alreves[j], alreves[i]
		}
		return c.Evaluar(e.Tj, alreves) == got
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 3000}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}

// CA-779: Mayor es el mas alto en el orden del vocabulario; "" es normal, uno
// de fuera cuenta como alto, y nunca devuelve "".
func TestCA779_Mayor(t *testing.T) {
	const ca = "CA-779"
	for _, k := range [][3]string{
		{"bajo", "normal", "normal"}, {"normal", "bajo", "normal"}, {"alto", "bajo", "alto"}, {"bajo", "alto", "alto"},
		{"bajo", "bajo", "bajo"}, {"", "", "normal"}, {"", "bajo", "normal"}, {"bajo", "", "normal"}, {"", "alto", "alto"},
		{"medio", "bajo", "alto"}, {"bajo", "Alto", "alto"}, {"normal", "x", "alto"},
	} {
		if got := Mayor(k[0], k[1]); got != k[2] {
			t.Fatalf("%s: Mayor(%q, %q) = %q, quiero %q", ca, k[0], k[1], got, k[2])
		}
	}
	nivelMayor := func(r string) int {
		switch r {
		case "bajo":
			return 0
		case "", "normal":
			return 1
		}
		return 2
	}
	prop := func(a, b uint8) bool {
		vals := []string{"", "bajo", "normal", "alto", "medio", "BAJO", " normal"}
		x, y := vals[int(a)%len(vals)], vals[int(b)%len(vals)]
		got := Mayor(x, y)
		want := nivelMayor(x)
		if nivelMayor(y) > want {
			want = nivelMayor(y)
		}
		return got != "" && retyNivel(got) == want && Mayor(y, x) == got
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}
