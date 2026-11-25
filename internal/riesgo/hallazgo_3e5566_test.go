// Regresion del hallazgo 20261125T162554_3e5566 (high): casan probaba con
// backtracking cada reparto de segmentos entre los `**`, y un patron con k
// `**` contra una ruta de L segmentos tardaba del orden de L^k. Con k=16 y
// L=32, un aplica_en o una ruta de `rutas:` colgaba el verify mas de un
// minuto. La version nueva es programacion dinamica, O(|patron|·|ruta|), y
// da los mismos resultados: se compara con una copia del algoritmo de antes
// (h3e5CasanAntes), en los ejemplos de CA-777, con `**` al principio, en
// medio y al final, y en todas las combinaciones chicas de un alfabeto de
// segmentos.
package riesgo

import (
	"path"
	"strings"
	"testing"
	"time"
)

// h3e5CasanAntes es casan como era antes de la correccion: el oraculo de la
// semantica de hoy.
func h3e5CasanAntes(pat, seg []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(seg); i++ {
				if h3e5CasanAntes(pat[1:], seg[i:]) {
					return true
				}
			}
			return false
		}
		if len(seg) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], seg[0]); err != nil || !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}

// h3e5CoincideAntes es Coincide sobre h3e5CasanAntes.
func h3e5CoincideAntes(patron, ruta string) bool {
	if ruta == "" || !PatronValido(patron) {
		return false
	}
	pat := segmentos(strings.ToLower(patron))
	segs := strings.Split(strings.ToLower(ruta), "/")
	for k := len(segs); k >= 1; k-- {
		if h3e5CasanAntes(pat, segs[:k]) {
			return true
		}
	}
	return false
}

// h3e5EnTerritorioAntes es enTerritorio sobre h3e5CasanAntes.
func h3e5EnTerritorioAntes(patrones []string, ruta string) bool {
	segs := strings.Split(ruta, "/")
	for _, p := range patrones {
		if p = strings.TrimSpace(p); p != "" && h3e5CasanAntes(segmentos(p), segs) {
			return true
		}
	}
	return false
}

func TestHallazgo3e5566_CA777_ElMismoResultadoQueAntes(t *testing.T) {
	const ca = "CA-777"
	for _, k := range []struct {
		patron, ruta string
		want         bool
	}{
		// los ejemplos de CA-777
		{"**/*auth*", "internal/oauth.go", true},
		{"**/*auth*", "app/Http/Auth/Login.php", true},
		{"vendor", "vendor/a/b.go", true},
		{"vendor", "a/vendor/b.go", false},
		{"**/*auth*", "internal/app.go", false},
		// `**` al principio
		{"**/b", "b", true},
		{"**/b", "a/b", true},
		{"**/b", "a/x/b/c.go", true},
		{"**/b", "a/xb", false},
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "a/b/c.php", false},
		// `**` en medio
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/b/z", true},
		{"a/**/b", "x/a/b", false},
		{"a/**/b/**/c", "a/b/c", true},
		{"a/**/b/**/c", "a/1/b/2/3/c/d", true},
		{"a/**/b/**/c", "a/c/b", false},
		{"a/**/**/b", "a/b", true},
		// `**` al final
		{"a/**", "a", true},
		{"a/**", "a/b/c", true},
		{"a/**", "b/a", false},
		{"**", "a/b/c.go", true},
		{"**/a/**", "x/a", true},
		{"**/a/**", "x/y", false},
		// `*`, `?` y `[...]` dentro de un segmento, y mayusculas
		{"**/*Tenant*/**/?x", "src/MultiTenant/q/zx", true},
		{"**/[ab]*/**/c", "x/Bq/c", true},
		{"**/[ab]*/**/c", "x/cq/c", false},
		// un patron invalido no coincide con nada
		{"app/[", "app/[", false},
		{"a//b", "a//b", false},
	} {
		if got := Coincide(k.patron, k.ruta); got != k.want {
			t.Fatalf("%s: Coincide(%q, %q) = %v, quiero %v", ca, k.patron, k.ruta, got, k.want)
		}
		if antes := h3e5CoincideAntes(k.patron, k.ruta); antes != k.want {
			t.Fatalf("%s: el oraculo de antes da Coincide(%q, %q) = %v: la tabla esta mal", ca, k.patron, k.ruta, antes)
		}
	}
}

// Todas las combinaciones de hasta 4 segmentos de patron y de ruta, de un
// alfabeto con `**`, comodines, un segmento malformado y uno vacio: Coincide
// y enTerritorio dan lo mismo que antes.
func TestHallazgo3e5566_CA777_ElMismoResultadoEnTodasLasCombinaciones(t *testing.T) {
	const ca = "CA-777"
	pats := h3e5Combinaciones([]string{"**", "a", "*", "?b", "[ab]*", "[a", ""}, 4)
	rutas := h3e5Combinaciones([]string{"a", "b", "ab", "B", ""}, 4)
	for _, p := range pats {
		for _, r := range rutas {
			if got, antes := Coincide(p, r), h3e5CoincideAntes(p, r); got != antes {
				t.Fatalf("%s: Coincide(%q, %q) = %v, antes %v", ca, p, r, got, antes)
			}
			if got, antes := enTerritorio([]string{p}, r), h3e5EnTerritorioAntes([]string{p}, r); got != antes {
				t.Fatalf("%s: enTerritorio(%q, %q) = %v, antes %v", ca, p, r, got, antes)
			}
		}
	}
}

// h3e5Combinaciones son las uniones con "/" de 1 a n segmentos de alfabeto.
func h3e5Combinaciones(alfabeto []string, n int) []string {
	var out []string
	nivel := []string{""}
	for i := 0; i < n; i++ {
		var sig []string
		for _, pre := range nivel {
			for _, s := range alfabeto {
				if i == 0 {
					sig = append(sig, s)
				} else {
					sig = append(sig, pre+"/"+s)
				}
			}
		}
		out = append(out, sig...)
		nivel = sig
	}
	return out
}

// h3e5AntesDe corre f y falla si no termina dentro de tope (el codigo de
// antes tardaba minutos en estos casos: el test no lo espera).
func h3e5AntesDe(t *testing.T, ca, caso string, tope time.Duration, f func() bool) bool {
	t.Helper()
	listo := make(chan bool, 1)
	t0 := time.Now()
	go func() { listo <- f() }()
	select {
	case got := <-listo:
		return got
	case <-time.After(tope):
		t.Fatalf("%s (%s): no termino en %v (lleva %v)", ca, caso, tope, time.Since(t0).Round(time.Millisecond))
		return false
	}
}

func TestHallazgo3e5566_CA777_MuchosDobleAsteriscoTerminanEnMenosDeUnSegundo(t *testing.T) {
	const ca = "CA-777"
	const tope = time.Second
	reales := []string{"app", "Http", "Controllers", "Admin", "data", "manager", "cache", "Facades",
		"Domain", "Costeo", "Factura", "Pagos", "Jobs", "Mail", "Traits", "Gateway"}
	var nombres []string
	for len(nombres) < 32 {
		nombres = append(nombres, reales...)
	}
	real32 := strings.Join(nombres[:32], "/")
	a32 := strings.TrimSuffix(strings.Repeat("a/", 32), "/")
	casos := []struct {
		nombre, patron, ruta string
		want                 bool
	}{
		// k=16 `**/a` terminados en z, contra 32 segmentos a: ninguno es z
		{"k=16 L=32", strings.Repeat("**/a/", 16) + "z", a32, false},
		// lo mismo con comodines contra nombres reales: ninguno termina en .zz
		{"*a* con nombres reales", strings.Repeat("**/*a*/", 12) + "*.zz", real32, false},
		// y uno que si coincide: 12 nombres con a, en orden, y lo que siga
		{"*a* con nombres reales que coincide", strings.Repeat("**/*a*/", 12) + "**", real32, true},
	}
	for _, c := range casos {
		if !PatronValido(c.patron) {
			t.Fatalf("%s (%s): el patron es valido (el spec no acota los **): %q", ca, c.nombre, c.patron)
		}
		if got := h3e5AntesDe(t, ca, c.nombre+", Coincide", tope, func() bool { return Coincide(c.patron, c.ruta) }); got != c.want {
			t.Fatalf("%s (%s): Coincide = %v, quiero %v", ca, c.nombre, got, c.want)
		}
		if got := h3e5AntesDe(t, ca, c.nombre+", enTerritorio", tope, func() bool { return enTerritorio([]string{c.patron}, c.ruta) }); got != c.want {
			t.Fatalf("%s (%s): enTerritorio = %v, quiero %v", ca, c.nombre, got, c.want)
		}
	}

	// en un tamano que el algoritmo de antes todavia resuelve, el mismo
	// resultado
	for _, k := range []int{2, 4, 6} {
		patron := strings.Repeat("**/a/", k) + "z"
		ruta := strings.TrimSuffix(strings.Repeat("a/", 12), "/") + "/z"
		for _, r := range []string{ruta, strings.TrimSuffix(ruta, "/z")} {
			if got, antes := Coincide(patron, r), h3e5CoincideAntes(patron, r); got != antes {
				t.Fatalf("%s (k=%d): Coincide(%q, %q) = %v, antes %v", ca, k, patron, r, got, antes)
			}
		}
	}
}
