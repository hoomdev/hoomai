// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// el paquete riesgo (CA-615, CA-618): el vocabulario, la ruta de riesgo y el
// riesgo efectivo. Escritos desde el spec y las firmas publicas, sin leer la
// implementacion.
package riesgo

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

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

// CA-618: Ruta devuelve el primer archivo, en su orden, que contiene una
// marca en minusculas, tal como se dio; vacio cuando ninguno.
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
		{[]string{"src/DESIGN.kt"}, "src/DESIGN.kt"}, // `sign` atrapa `design`: gruesa a proposito
		{[]string{"a/TOKEN.txt"}, "a/TOKEN.txt"},
		{[]string{"x/sudoers"}, "x/sudoers"},
		{[]string{"", "x.go"}, ""},
	}
	for _, c := range casos {
		if got := Ruta(c.files); got != c.want {
			t.Fatalf("CA-618: Ruta(%q) = %q, quiero %q", c.files, got, c.want)
		}
	}
	for _, m := range Marcas {
		f := "dir/x" + strings.ToUpper(m) + "y.go"
		if got := Ruta([]string{"limpio.go", f}); got != f {
			t.Fatalf("CA-618: la marca %q en mayusculas hace ruta de riesgo a %q: %q", m, f, got)
		}
	}
}

// CA-618: las marcas son las de la regla de lentes, sin cambios.
func TestCA618_MarcasSinCambios(t *testing.T) {
	want := []string{"auth", "login", "password", "passwd", "secret", "token", "cred",
		"crypt", "sign", "pago", "payment", "billing", "invoice", "dte",
		"permission", "permiso", "sandbox", "sudo"}
	if !reflect.DeepEqual(Marcas, want) {
		t.Fatalf("CA-618: la lista de marcas no cambia (no-goal): %v", Marcas)
	}
}

// CA-618: Efectivo con cada combinacion del contrato.
func TestCA618_Efectivo(t *testing.T) {
	riesgosa := []string{"main.go", "internal/auth/x.go"}
	limpia := []string{"main.go", "docs/a.md"}
	casos := []struct {
		decl  string
		files []string
		ef    string
		mot   string
	}{
		{"bajo", riesgosa, "normal", "el cambio toca internal/auth/x.go: riesgo normal"},
		{"bajo", limpia, "bajo", ""},
		{"bajo", nil, "bajo", ""},
		{"normal", riesgosa, "normal", ""},
		{"normal", limpia, "normal", ""},
		{"alto", riesgosa, "alto", ""},
		{"alto", limpia, "alto", ""},
		{"", riesgosa, "normal", ""},
		{"", limpia, "normal", ""},
		{"", nil, "normal", ""},
	}
	for _, c := range casos {
		ef, mot := Efectivo(c.decl, c.files)
		if ef != c.ef || mot != c.mot {
			t.Fatalf("CA-618: Efectivo(%q, %q) = (%q, %q), quiero (%q, %q)", c.decl, c.files, ef, mot, c.ef, c.mot)
		}
	}
}

// CA-618 (caso limite: "Riesgo bajo y el cambio toca internal/auth/"): el
// motivo nombra la PRIMERA ruta de riesgo, la misma que Ruta.
func TestCA618_EfectivoNombraLaPrimeraRuta(t *testing.T) {
	files := []string{"a.go", "x/billing.go", "internal/auth/y.go"}
	ef, mot := Efectivo("bajo", files)
	if ef != "normal" || mot != "el cambio toca x/billing.go: riesgo normal" {
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
// siempre del vocabulario; solo un bajo con ruta de riesgo cambia, y entonces
// a normal con un motivo que nombra Ruta(files); en otro caso el motivo es
// vacio.
func TestCA618_PropiedadEfectivoNuncaBaja(t *testing.T) {
	prop := func(e entrada) bool {
		ef, mot := Efectivo(e.Decl, e.Files)
		if nivel(ef) < nivel(e.Decl) || nivel(ef) < 0 || ef == "" {
			return false
		}
		ruta := Ruta(e.Files)
		if e.Decl == "bajo" && ruta != "" {
			return ef == "normal" && mot == "el cambio toca "+ruta+": riesgo normal"
		}
		want := e.Decl
		if want == "" {
			want = "normal"
		}
		return ef == want && mot == ""
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatalf("CA-618: %v", err)
	}
}

// CA-618 (propiedad): Ruta devuelve "" o un elemento de files que contiene
// una marca, y ningun elemento anterior la contiene.
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
		got := Ruta(e.Files)
		for _, f := range e.Files {
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

// CA-618: Efectivo no muta la lista que recibe.
func TestCA618_EfectivoNoMutaFiles(t *testing.T) {
	files := []string{"b.go", "internal/auth/a.go"}
	copia := append([]string(nil), files...)
	Efectivo("bajo", files)
	Ruta(files)
	if !reflect.DeepEqual(files, copia) {
		t.Fatalf("CA-618: files cambio: %v", files)
	}
}
