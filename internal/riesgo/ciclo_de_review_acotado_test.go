// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// el paquete riesgo (CA-615, CA-618): el vocabulario, la ruta de riesgo y el
// riesgo efectivo. Escritos desde el spec y las firmas publicas, sin leer la
// implementacion. Re-expresados por riesgo-efectivo-y-tamano (CA-618): Ruta y
// Efectivo desaparecen y la regla es Clasificador.Evaluar con las rutas base;
// un bajo con ruta de riesgo da alto, el motivo nunca queda vacio, la ruta
// que nombra es la primera en orden de bytes, y `sign` sale de las marcas.
package riesgo

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/quick"
)

// cr618 es el clasificador de un proyecto sin la seccion rutas.
func cr618() Clasificador { return Clasificador{Rutas: RutasBase()} }

// ruta618 es la ruta de riesgo que nombra la regla (Evaluar, regla 4) para
// files, o "" cuando ninguna sube el riesgo: lo que antes daba Ruta.
func ruta618(files []string) string {
	e := cr618().Evaluar(Tarjeta{Tarea: "t"}, files)
	const pre, suf = "el cambio toca ", ": riesgo alto"
	if e.Efectivo == Alto && strings.HasPrefix(e.Motivo, pre) && strings.HasSuffix(e.Motivo, suf) {
		return strings.TrimSuffix(strings.TrimPrefix(e.Motivo, pre), suf)
	}
	return ""
}

// CA-618: el vocabulario es bajo, normal, alto, en ese orden.
func TestCA618_Vocabulario(t *testing.T) {
	if !reflect.DeepEqual(Vocabulario, []string{"bajo", "normal", "alto"}) {
		t.Fatalf("CA-618: Vocabulario es bajo, normal, alto en ese orden: %v", Vocabulario)
	}
	if Bajo != "bajo" || Normal != "normal" || Alto != "alto" {
		t.Fatalf("CA-618: las constantes: %q %q %q", Bajo, Normal, Alto)
	}
}

// CA-615: Validar acepta el vocabulario y el vacio, y rechaza el resto con el
// texto exacto del contrato.
func TestCA615_Validar(t *testing.T) {
	for _, r := range []string{"", "bajo", "normal", "alto"} {
		if err := Validar(r); err != nil {
			t.Fatalf("CA-615: %q es un riesgo valido: %v", r, err)
		}
	}
	for _, r := range []string{"x", "Alto", "ALTO", " alto", "medio", "alto ", "altó", "bajo\n", "\x00"} {
		err := Validar(r)
		if err == nil {
			t.Fatalf("CA-615: %q no es del vocabulario", r)
		}
		want := `riesgo ` + quote(r) + ` fuera del vocabulario (bajo, normal, alto)`
		if err.Error() != want {
			t.Fatalf("CA-615: el texto para %q es %q, fue %q", r, want, err.Error())
		}
	}
}

// quote es %q de Go, como lo cita el contrato.
func quote(s string) string { return fmt.Sprintf("%q", s) }

// CA-618: la ruta de riesgo es el primer archivo, en orden de bytes, que
// coincide con rutas.de_riesgo (las marcas, sin mayusculas), tal como se
// dio; vacio cuando ninguno.
func TestCA618_Ruta(t *testing.T) {
	casos := []struct {
		files []string
		want  string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"main.go", "README.md"}, ""},
		{[]string{"main.go", "internal/auth/login.go", "pago.go"}, "internal/auth/login.go"},
		{[]string{"app/Pagos/Factura.php", "internal/auth/x.go"}, "app/Pagos/Factura.php"},
		{[]string{"src/DESIGN.kt"}, ""}, // `sign` ya no es marca: no atrapa `design`
		{[]string{"a/TOKEN.txt"}, "a/TOKEN.txt"},
		{[]string{"x/sudoers"}, "x/sudoers"},
		{[]string{"", "x.go"}, ""},
	}
	for _, c := range casos {
		if got := ruta618(c.files); got != c.want {
			t.Fatalf("CA-618: la ruta de riesgo de %q es %q, quiero %q", c.files, got, c.want)
		}
	}
	for _, m := range Marcas {
		f := "dir/x" + strings.ToUpper(m) + "y.go"
		if got := ruta618([]string{"limpio.go", f}); got != f {
			t.Fatalf("CA-618: la marca %q en mayusculas hace ruta de riesgo a %q: %q", m, f, got)
		}
	}
}

// CA-618: las marcas son las de la regla de lentes, salvo `sign`, que pasa a
// `signin` y `signature` (riesgo-efectivo-y-tamano).
func TestCA618_MarcasSinCambios(t *testing.T) {
	want := []string{"auth", "login", "password", "passwd", "secret", "token", "cred",
		"crypt", "signin", "signature", "pago", "payment", "billing", "invoice", "dte",
		"permission", "permiso", "sandbox", "sudo"}
	if !reflect.DeepEqual(Marcas, want) {
		t.Fatalf("CA-618: las marcas cambian solo sign por signin y signature: %v", Marcas)
	}
}

// CA-618: el riesgo efectivo (Evaluar) con cada combinacion del contrato.
func TestCA618_Efectivo(t *testing.T) {
	riesgosa := []string{"main.go", "internal/auth/x.go"}
	limpia := []string{"main.go", "docs/a.md"}
	casos := []struct {
		decl  string
		files []string
		ef    string
		mot   string
	}{
		{"bajo", riesgosa, "alto", "el cambio toca internal/auth/x.go: riesgo alto"},
		{"bajo", limpia, "normal", "el cambio toca main.go, fuera de las rutas de bajo riesgo: riesgo normal"},
		{"bajo", nil, "bajo", "declarado bajo"},
		{"normal", riesgosa, "alto", "el cambio toca internal/auth/x.go: riesgo alto"},
		{"normal", limpia, "normal", "declarado normal"},
		{"alto", riesgosa, "alto", "declarado alto"},
		{"alto", limpia, "alto", "declarado alto"},
		{"", riesgosa, "alto", "el cambio toca internal/auth/x.go: riesgo alto"},
		{"", limpia, "normal", "declarado normal"},
		{"", nil, "normal", "declarado normal"},
	}
	for _, c := range casos {
		e := cr618().Evaluar(Tarjeta{Tarea: "t", Riesgo: c.decl}, c.files)
		ef, mot := e.Efectivo, e.Motivo
		if ef != c.ef || mot != c.mot {
			t.Fatalf("CA-618: Evaluar(%q, %q) = (%q, %q), quiero (%q, %q)", c.decl, c.files, ef, mot, c.ef, c.mot)
		}
	}
}

// CA-618 (caso limite: "Riesgo bajo y el cambio toca internal/auth/"): el
// motivo nombra la PRIMERA ruta de riesgo en orden de bytes, y el riesgo es
// alto.
func TestCA618_EfectivoNombraLaPrimeraRuta(t *testing.T) {
	files := []string{"a.go", "x/billing.go", "internal/auth/y.go"}
	e := cr618().Evaluar(Tarjeta{Tarea: "t", Riesgo: "bajo"}, files)
	ef, mot := e.Efectivo, e.Motivo
	if ef != "alto" || mot != "el cambio toca internal/auth/y.go: riesgo alto" {
		t.Fatalf("CA-618: (%q, %q)", ef, mot)
	}
}

func nivel(r string) int {
	switch r {
	case "bajo":
		return 0
	case "", "normal":
		return 1
	case "alto":
		return 2
	}
	return -1
}

type entrada struct {
	Decl  string
	Files []string
}

func (entrada) Generate(r *rand.Rand, _ int) reflect.Value {
	decls := []string{"", "bajo", "normal", "alto"}
	piezas := []string{"main.go", "docs/a.md", "internal/auth/x.go", "pagos/f.php", "x/DESIGN.kt",
		"ñandú/árbol.go", "", "README.md", "a/b/c/d.go", "Token.TXT"}
	n := r.Intn(6)
	var fs []string
	for i := 0; i < n; i++ {
		fs = append(fs, piezas[r.Intn(len(piezas))])
	}
	return reflect.ValueOf(entrada{Decl: decls[r.Intn(len(decls))], Files: fs})
}

// CA-618 (propiedad): el riesgo sube solo y nunca baja solo; el efectivo es
// siempre del vocabulario; fuera de un declarado alto, una ruta de riesgo lo
// sube a alto con un motivo que nombra la primera en orden de bytes; un bajo
// con producto fuera de las rutas de bajo riesgo sube a normal; el motivo
// nunca es vacio.
func TestCA618_PropiedadEfectivoNuncaBaja(t *testing.T) {
	base := RutasBase()
	prop := func(e entrada) bool {
		ev := cr618().Evaluar(Tarjeta{Tarea: "t", Riesgo: e.Decl}, e.Files)
		ef, mot := ev.Efectivo, ev.Motivo
		if nivel(ef) < nivel(e.Decl) || nivel(ef) < 0 || ef == "" || mot == "" {
			return false
		}
		orden := append([]string(nil), e.Files...)
		sort.Strings(orden)
		ruta, fuera := "", ""
		for _, f := range orden {
			if f == "" {
				continue
			}
			if ruta == "" && CoincideAlguno(base.DeRiesgo, f) {
				ruta = f
			}
			if fuera == "" && !CoincideAlguno(base.DeBajoRiesgo, f) {
				fuera = f
			}
		}
		switch {
		case e.Decl == "alto":
			return ef == "alto" && mot == "declarado alto"
		case ruta != "":
			return ef == "alto" && mot == "el cambio toca "+ruta+": riesgo alto"
		case e.Decl == "bajo" && fuera != "":
			return ef == "normal" && mot == "el cambio toca "+fuera+", fuera de las rutas de bajo riesgo: riesgo normal"
		}
		want := e.Decl
		if want == "" {
			want = "normal"
		}
		return ef == want && mot == "declarado "+want
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatalf("CA-618: %v", err)
	}
}

// CA-618 (propiedad): la ruta de riesgo es "" o un elemento de files que
// contiene una marca, y ningun elemento anterior en orden de bytes la
// contiene.
func TestCA618_PropiedadRutaEsLaPrimera(t *testing.T) {
	tiene := func(f string) bool {
		l := strings.ToLower(f)
		for _, m := range Marcas {
			if strings.Contains(l, m) {
				return true
			}
		}
		return false
	}
	prop := func(e entrada) bool {
		got := ruta618(e.Files)
		orden := append([]string(nil), e.Files...)
		sort.Strings(orden)
		for _, f := range orden {
			if tiene(f) {
				return got == f
			}
		}
		return got == ""
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatalf("CA-618: %v", err)
	}
}

// CA-618: Evaluar no muta la lista que recibe.
func TestCA618_EfectivoNoMutaFiles(t *testing.T) {
	files := []string{"b.go", "internal/auth/a.go"}
	copia := append([]string(nil), files...)
	cr618().Evaluar(Tarjeta{Tarea: "t", Riesgo: "bajo"}, files)
	ruta618(files)
	if !reflect.DeepEqual(files, copia) {
		t.Fatalf("CA-618: files cambio: %v", files)
	}
}
