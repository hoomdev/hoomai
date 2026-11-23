// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// los perfiles: las rutas por defecto de cada perfil (CA-778), el
// clasificador de un proyecto con su territorio del test-writer y sus
// lockfiles (CA-774) y una seccion `rutas` sobre el perfil (CA-776).
// Escritos desde el spec, las aclaraciones de la ronda 0 y las firmas
// publicas, sin leer la implementacion.
package profiles_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

var retyMarcas = []string{"auth", "login", "password", "passwd", "secret", "token", "cred",
	"crypt", "signin", "signature", "pago", "payment", "billing", "invoice", "dte",
	"permission", "permiso", "sandbox", "sudo"}

var retyBajoBase = []string{"**/*.md", "**/*.txt", "**/*.rst", "**/*.adoc", "**/docs", "**/LICENSE", "**/NOTICE"}

func retyDeRiesgoBase() []string {
	var out []string
	for _, m := range retyMarcas {
		out = append(out, "**/*"+m+"*")
	}
	return out
}

// retyLista compara dos listas de patrones; nil y vacia son iguales.
func retyLista(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func retyRutas(t *testing.T, ca, perfil string, r riesgo.Rutas, riesgos, bajos, generadas []string) {
	t.Helper()
	if !retyLista(r.DeRiesgo, riesgos) {
		t.Fatalf("%s: perfil %q: de_riesgo es %q, fue %q", ca, perfil, riesgos, r.DeRiesgo)
	}
	if !retyLista(r.DeBajoRiesgo, bajos) {
		t.Fatalf("%s: perfil %q: de_bajo_riesgo es %q, fue %q", ca, perfil, bajos, r.DeBajoRiesgo)
	}
	if !retyLista(r.Generadas, generadas) {
		t.Fatalf("%s: perfil %q: generadas es %q, fue %q", ca, perfil, generadas, r.Generadas)
	}
}

// ---------------------------------------------------------------- CA-778

// CA-778: las rutas por defecto son la fila base mas la de cada perfil, con
// sus valores exactos y en orden; ningun perfil suma a de_riesgo.
func TestCA778_LasRutasDeCadaPerfil(t *testing.T) {
	const ca = "CA-778"
	base := retyDeRiesgoBase()
	sinPerfil, err := profiles.Rutas("")
	if err != nil {
		t.Fatalf("%s: profiles.Rutas(\"\"): %v", ca, err)
	}
	retyRutas(t, ca, "", sinPerfil, base, retyBajoBase, nil)
	if !reflect.DeepEqual(sinPerfil, riesgo.RutasBase()) {
		t.Fatalf("%s: sin perfil valen las de RutasBase: %+v", ca, sinPerfil)
	}
	kmp := []string{"kotlin-js-store/yarn.lock", "**/gradle.lockfile"}
	for _, k := range []struct {
		perfil           string
		bajos, generadas []string
	}{
		{"go", retyBajoBase, []string{"go.sum", "vendor"}},
		{"laravel", append(append([]string(nil), retyBajoBase...), "lang", "resources/lang"),
			[]string{"composer.lock", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "vendor", "node_modules", "public/build"}},
		{"kmp", retyBajoBase, kmp},
		{"kmp-compose", retyBajoBase, kmp},
	} {
		r, err := profiles.Rutas(k.perfil)
		if err != nil {
			t.Fatalf("%s: profiles.Rutas(%q): %v", ca, k.perfil, err)
		}
		retyRutas(t, ca, k.perfil, r, base, k.bajos, k.generadas)
	}
	for _, n := range profiles.Names() {
		r, err := profiles.Rutas(n)
		if err != nil {
			t.Fatalf("%s: profiles.Rutas(%q) de un perfil que existe: %v", ca, n, err)
		}
		if !reflect.DeepEqual(r.DeRiesgo, base) {
			t.Fatalf("%s: el perfil %s no suma a de_riesgo: %q", ca, n, r.DeRiesgo)
		}
		for _, lista := range [][]string{r.DeRiesgo, r.DeBajoRiesgo, r.Generadas} {
			visto := map[string]bool{}
			for _, p := range lista {
				if !riesgo.PatronValido(p) || visto[p] {
					t.Fatalf("%s: el perfil %s trae un patron invalido o repetido: %q", ca, n, lista)
				}
				visto[p] = true
			}
		}
		for _, p := range r.DeRiesgo {
			if p == "**/*sign*" {
				t.Fatalf("%s: el perfil %s todavia trae la marca sign", ca, n)
			}
		}
	}
	if _, err := profiles.Rutas("no-existe"); err == nil {
		t.Fatalf("%s: un perfil desconocido es un error", ca)
	}
}

// CA-778: cada llamada a profiles.Rutas da listas nuevas.
func TestCA778_RutasDelPerfilSonCopias(t *testing.T) {
	const ca = "CA-778"
	a, err := profiles.Rutas("laravel")
	if err != nil {
		t.Fatal(err)
	}
	a.Generadas[0] = "cambiado"
	a.DeBajoRiesgo[0] = "cambiado"
	a.DeRiesgo[0] = "cambiado"
	b, err := profiles.Rutas("laravel")
	if err != nil {
		t.Fatal(err)
	}
	if b.Generadas[0] != "composer.lock" || b.DeBajoRiesgo[0] != "**/*.md" || b.DeRiesgo[0] != "**/*auth*" {
		t.Fatalf("%s: cambiar lo que dio profiles.Rutas cambio la siguiente llamada: %+v", ca, b)
	}
	if r := riesgo.RutasBase(); r.DeRiesgo[0] != "**/*auth*" || len(r.Generadas) != 0 {
		t.Fatalf("%s: cambiar lo que dio profiles.Rutas cambio RutasBase: %+v", ca, r)
	}
}

// CA-778: sin la seccion rutas, design.md, assign.go y signal.go no son rutas
// de riesgo, y signature.go, SignIn.php e internal/auth/x.go si, en cada
// perfil; un generado del perfil o un archivo de .hoom/ con una marca no sube
// el riesgo.
func TestCA778_SinRutasEnCadaPerfil(t *testing.T) {
	const ca = "CA-778"
	for _, n := range append([]string{""}, profiles.Names()...) {
		c, err := profiles.Clasificador(&manifest.Manifest{Profile: n})
		if err != nil {
			t.Fatalf("%s: profiles.Clasificador(perfil %q): %v", ca, n, err)
		}
		tj := riesgo.Tarjeta{Tarea: "pagos"}
		for _, f := range []string{"design.md", "assign.go", "signal.go"} {
			if e := c.Evaluar(tj, []string{f}); e.Efectivo != "normal" || e.Motivo != "declarado normal" {
				t.Fatalf("%s: perfil %q: %s no es ruta de riesgo: %+v", ca, n, f, e)
			}
		}
		for _, f := range []string{"signature.go", "SignIn.php", "internal/auth/x.go"} {
			want := riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: "el cambio toca " + f + ": riesgo alto"}
			if e := c.Evaluar(tj, []string{f}); e != want {
				t.Fatalf("%s: perfil %q: %s es ruta de riesgo: %+v", ca, n, f, e)
			}
		}
		if e := c.Evaluar(tj, []string{".hoom/specs/token.md", ".hoom/items/auth.yaml"}); e.Efectivo != "normal" {
			t.Fatalf("%s: perfil %q: .hoom/ no sube el riesgo: %+v", ca, n, e)
		}
	}
	for _, k := range []struct{ perfil, generado string }{
		{"go", "vendor/golang.org/x/crypto/a.go"},
		{"laravel", "vendor/laravel/sanctum/src/Token.php"},
		{"laravel", "node_modules/auth-lib/index.js"},
		{"laravel", "public/build/assets/login.js"},
		{"kmp", "app/auth/gradle.lockfile"},
	} {
		c, err := profiles.Clasificador(&manifest.Manifest{Profile: k.perfil})
		if err != nil {
			t.Fatal(err)
		}
		e := c.Evaluar(riesgo.Tarjeta{Tarea: "pagos", Riesgo: "bajo"}, []string{k.generado, "docs/a.md"})
		if e != (riesgo.Evaluacion{Declarado: "bajo", Efectivo: "bajo", Motivo: "declarado bajo"}) {
			t.Fatalf("%s: perfil %s: el generado %s no sube el riesgo: %+v", ca, k.perfil, k.generado, e)
		}
	}
	// laravel: los textos de traduccion son de bajo riesgo.
	c, err := profiles.Clasificador(&manifest.Manifest{Profile: "laravel"})
	if err != nil {
		t.Fatal(err)
	}
	if e := c.Evaluar(riesgo.Tarjeta{Tarea: "t", Riesgo: "bajo"}, []string{"lang/es/validation.php", "resources/lang/en.json"}); e.Efectivo != "bajo" {
		t.Fatalf("%s: laravel: lang y resources/lang son de bajo riesgo: %+v", ca, e)
	}
	g, err := profiles.Clasificador(&manifest.Manifest{Profile: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if e := g.Evaluar(riesgo.Tarjeta{Tarea: "t", Riesgo: "bajo"}, []string{"lang/es/validation.php"}); e.Efectivo != "normal" {
		t.Fatalf("%s: go: lang no es de bajo riesgo fuera de laravel: %+v", ca, e)
	}
}

// ---------------------------------------------------------------- CA-774

// CA-774: el territorio por defecto del test-writer, exacto, y cada llamada
// da una lista nueva.
func TestCA774_AllowPorDefectoDelTestWriter(t *testing.T) {
	const ca = "CA-774"
	want := []string{".hoom/**", "tests/**", "test/**", "spec/**", "src/test/**", "**/*_test.go", "**/*Test.php",
		"**/*Test.kt", "**/*Test.java", "**/*Spec.kt", "**/*.test.ts", "**/*.test.js"}
	got := agents.AllowPorDefecto(agents.ScopeTests)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: AllowPorDefecto(tests) es %q, fue %q", ca, want, got)
	}
	got[0] = "cambiado"
	if otra := agents.AllowPorDefecto(agents.ScopeTests); otra[0] != ".hoom/**" {
		t.Fatalf("%s: AllowPorDefecto da una lista nueva en cada llamada: %q", ca, otra)
	}
}

// CA-774: con el perfil laravel, composer.lock y package-lock.json no suman;
// con go, go.sum y vendor/ tampoco; 90 de producto y 1,500 de pruebas son 90
// y 1500; lo de .hoom/ suma a hoom, tambien un _test.go.
func TestCA774_LockfilesYVendorPorPerfil(t *testing.T) {
	const ca = "CA-774"
	for _, k := range []struct {
		perfil string
		lineas map[string]int
		want   riesgo.Tamano
	}{
		{"laravel", map[string]int{
			"composer.lock":                   3000,
			"package-lock.json":               5000,
			"yarn.lock":                       70,
			"pnpm-lock.yaml":                  80,
			"vendor/laravel/framework/a.php":  800,
			"node_modules/x/y.js":             100,
			"public/build/app.js":             50,
			"app/Http/Controllers/Precio.php": 90,
			"tests/Feature/PrecioTest.php":    1500,
			".hoom/verdicts/a.json":           10,
			".hoom/x_test.go":                 5,
		}, riesgo.Tamano{Producto: 90, Pruebas: 1500, Hoom: 15}},
		{"go", map[string]int{
			"go.sum":                       400,
			"vendor/golang.org/x/sys/a.go": 900,
			"internal/app.go":              80,
			"go.mod":                       10,
			"internal/app_test.go":         1500,
			".hoom/x_test.go":              7,
		}, riesgo.Tamano{Producto: 90, Pruebas: 1500, Hoom: 7}},
		// Lo que es generado en un perfil es producto en otro.
		{"go", map[string]int{"composer.lock": 40, "package-lock.json": 50}, riesgo.Tamano{Producto: 90}},
		{"laravel", map[string]int{"go.sum": 90}, riesgo.Tamano{Producto: 90}},
		{"kmp", map[string]int{"kotlin-js-store/yarn.lock": 900, "app/gradle.lockfile": 30, "gradle.lockfile": 20,
			"yarn.lock": 90, "src/test/kotlin/PrecioTest.kt": 1500}, riesgo.Tamano{Producto: 90, Pruebas: 1500}},
		{"kmp-compose", map[string]int{"kotlin-js-store/yarn.lock": 900, "shared/gradle.lockfile": 30, "app/Main.kt": 90},
			riesgo.Tamano{Producto: 90}},
		{"", map[string]int{"go.sum": 40, "vendor/a.go": 50, "composer.lock": 1}, riesgo.Tamano{Producto: 91}},
	} {
		c, err := profiles.Clasificador(&manifest.Manifest{Profile: k.perfil})
		if err != nil {
			t.Fatalf("%s: profiles.Clasificador(perfil %q): %v", ca, k.perfil, err)
		}
		if got := c.Medir(k.lineas); got != k.want {
			t.Fatalf("%s: perfil %q: Medir(%v) = %+v, quiero %+v", ca, k.perfil, k.lineas, got, k.want)
		}
	}
}

// CA-774 y aclaracion 4: sin allow, `prueba` es el territorio por defecto
// (tambien con un allow vacio y sin mirar el deny); con allow, es ese
// territorio; la ruta entera y con mayusculas.
func TestCA774_PruebaEsElTerritorioDelTestWriter(t *testing.T) {
	const ca = "CA-774"
	def := agents.AllowPorDefecto(agents.ScopeTests)
	for _, k := range []struct {
		nombre string
		m      *manifest.Manifest
	}{
		{"sin hoom.yaml", nil},
		{"sin agents", &manifest.Manifest{}},
		{"allow vacio", &manifest.Manifest{Agents: manifest.AgentPolicies{"test-writer": {Write: manifest.WriteScope{Allow: []string{}}}}}},
		{"solo deny", &manifest.Manifest{Agents: manifest.AgentPolicies{"test-writer": {Write: manifest.WriteScope{Deny: []string{"tests/x/**"}}}}}},
		{"allow de otro rol", &manifest.Manifest{Agents: manifest.AgentPolicies{"writer": {Write: manifest.WriteScope{Allow: []string{"pruebas/**"}}}}}},
	} {
		c, err := profiles.Clasificador(k.m)
		if err != nil {
			t.Fatalf("%s: %s: %v", ca, k.nombre, err)
		}
		if !reflect.DeepEqual(c.Pruebas, def) {
			t.Fatalf("%s: %s: el territorio es el de AllowPorDefecto: %q", ca, k.nombre, c.Pruebas)
		}
		for _, f := range []struct{ ruta, clase string }{
			{"internal/app_test.go", riesgo.ClasePrueba},
			{"tests/x/a.php", riesgo.ClasePrueba},
			{"src/test/kotlin/A.kt", riesgo.ClasePrueba},
			{"web/precio.test.ts", riesgo.ClasePrueba},
			{"latest.go", riesgo.ClaseProducto},
			{"internal/testutil/x.go", riesgo.ClaseProducto},
			{"Tests/a.php", riesgo.ClaseProducto},
			{"pruebas/a.go", riesgo.ClaseProducto},
			{".hoom/x_test.go", riesgo.ClaseHoom},
			{"hoom.yaml", riesgo.ClaseProducto},
		} {
			if got := c.Clase(f.ruta); got != f.clase {
				t.Fatalf("%s: %s: Clase(%q) = %q, quiero %q", ca, k.nombre, f.ruta, got, f.clase)
			}
		}
	}
	if c, err := profiles.Clasificador(nil); err != nil || !reflect.DeepEqual(c.Rutas, riesgo.RutasBase()) {
		t.Fatalf("%s: un m nil tiene las rutas de RutasBase: %+v %v", ca, c.Rutas, err)
	}
	propio := &manifest.Manifest{Profile: "go", Agents: manifest.AgentPolicies{"test-writer": {Write: manifest.WriteScope{Allow: []string{"pruebas/**"}}}}}
	c, err := profiles.Clasificador(propio)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Pruebas, []string{"pruebas/**"}) {
		t.Fatalf("%s: con allow, el territorio es ese: %q", ca, c.Pruebas)
	}
	lineas := map[string]int{"pruebas/a.go": 1500, "internal/app_test.go": 60, "internal/app.go": 30, ".hoom/y_test.go": 4, "go.sum": 9}
	if got := c.Medir(lineas); got != (riesgo.Tamano{Producto: 90, Pruebas: 1500, Hoom: 4}) {
		t.Fatalf("%s: con allow [pruebas/**], un _test.go fuera es producto: %+v", ca, got)
	}
	if _, err := profiles.Clasificador(&manifest.Manifest{Profile: "no-existe"}); err == nil {
		t.Fatalf("%s: un perfil desconocido es un error", ca)
	}
}

// ---------------------------------------------------------------- CA-776

// CA-776: una lista en rutas.<clave> reemplaza el valor por defecto del
// perfil, [] lo deja vacio, y una clave nula o ausente usa el del perfil.
func TestCA776_LaSeccionSobreElPerfil(t *testing.T) {
	const ca = "CA-776"
	m := &manifest.Manifest{Profile: "laravel", Rutas: &manifest.Rutas{DeRiesgo: []string{}, Generadas: []string{"composer.lock"}}}
	c, err := profiles.Clasificador(m)
	if err != nil {
		t.Fatal(err)
	}
	lar, _ := profiles.Rutas("laravel")
	if len(c.Rutas.DeRiesgo) != 0 {
		t.Fatalf("%s: de_riesgo: [] lo deja vacio: %q", ca, c.Rutas.DeRiesgo)
	}
	if !reflect.DeepEqual(c.Rutas.DeBajoRiesgo, lar.DeBajoRiesgo) {
		t.Fatalf("%s: la clave ausente usa la del perfil: %q", ca, c.Rutas.DeBajoRiesgo)
	}
	if !reflect.DeepEqual(c.Rutas.Generadas, []string{"composer.lock"}) {
		t.Fatalf("%s: una lista reemplaza el valor por defecto entero: %q", ca, c.Rutas.Generadas)
	}
	// Con de_riesgo vacio, auth no sube; package-lock.json ya no es generado.
	if e := c.Evaluar(riesgo.Tarjeta{Tarea: "t"}, []string{"app/Http/Auth/Login.php"}); e.Efectivo != "normal" || e.Motivo != "declarado normal" {
		t.Fatalf("%s: con de_riesgo: [] ninguna ruta sube el riesgo: %+v", ca, e)
	}
	if got := c.Medir(map[string]int{"package-lock.json": 10, "composer.lock": 99}); got != (riesgo.Tamano{Producto: 10}) {
		t.Fatalf("%s: generadas propia reemplaza la del perfil: %+v", ca, got)
	}
	if !reflect.DeepEqual(m.Rutas.Generadas, []string{"composer.lock"}) || m.Rutas.DeBajoRiesgo != nil {
		t.Fatalf("%s: Clasificador no cambia la seccion del manifiesto: %+v", ca, m.Rutas)
	}
}

// CA-776 y CA-784: ClasificadorDe lee profile, rutas y agents de los bytes de
// un hoom.yaml (el del merge-base), con los errores de rutas; nil es sin
// hoom.yaml.
func TestCA776_ClasificadorDeLosBytes(t *testing.T) {
	const ca = "CA-776"
	for _, raw := range [][]byte{nil, {}} {
		c, err := profiles.ClasificadorDe(raw)
		if err != nil {
			t.Fatalf("%s: ClasificadorDe(%q): %v", ca, raw, err)
		}
		cero, _ := profiles.Clasificador(nil)
		if !reflect.DeepEqual(c, cero) {
			t.Fatalf("%s: ClasificadorDe sin bytes es Clasificador(nil): %+v", ca, c)
		}
	}
	raw := []byte("schema: hoom/v1\nproject: demo\nprofile: laravel\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" +
		"rutas:\n  generadas: [\"a.lock\"]\n  de_riesgo: null\n" +
		"agents:\n  test-writer:\n    write:\n      allow: [\"pruebas/**\"]\n")
	c, err := profiles.ClasificadorDe(raw)
	if err != nil {
		t.Fatalf("%s: ClasificadorDe: %v", ca, err)
	}
	lar, _ := profiles.Rutas("laravel")
	if !reflect.DeepEqual(c.Rutas.Generadas, []string{"a.lock"}) || !reflect.DeepEqual(c.Rutas.DeBajoRiesgo, lar.DeBajoRiesgo) ||
		!reflect.DeepEqual(c.Rutas.DeRiesgo, lar.DeRiesgo) || !reflect.DeepEqual(c.Pruebas, []string{"pruebas/**"}) {
		t.Fatalf("%s: ClasificadorDe lee profile, rutas y agents: %+v", ca, c)
	}
	for _, k := range []struct{ rutas, want string }{
		{"rutas:\n  de_riesgos: []\n", `rutas: clave desconocida "de_riesgos" (validas: de_riesgo, de_bajo_riesgo, generadas)`},
		{"rutas:\n  generadas: vendor\n", "rutas.generadas: tiene que ser una lista de patrones"},
		{"rutas:\n  de_bajo_riesgo: [\"a//b\"]\n", `rutas.de_bajo_riesgo: patron invalido "a//b"`},
	} {
		_, err := profiles.ClasificadorDe([]byte("profile: go\n" + k.rutas))
		if err == nil || !strings.Contains(err.Error(), k.want) {
			t.Fatalf("%s: ClasificadorDe con\n%s\nda %q, fue %v", ca, k.rutas, k.want, err)
		}
	}
}
