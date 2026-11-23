package riesgo

import (
	"fmt"
	"strings"
)

// The three riesgos a card may declare.
const (
	// Bajo: one lens (reliability) whatever the size of the change. It is
	// an allow-list that fails closed: a file of class producto outside
	// rutas.de_bajo_riesgo makes the effective riesgo Normal, and one that
	// matches rutas.de_riesgo makes it Alto (Clasificador.Evaluar).
	Bajo = "bajo"
	// Normal: the 4 lenses only past UmbralProducto lines of producto, the
	// one dominant lens otherwise. It is the absence of the key: no verb of
	// hoom ever writes `riesgo: normal`.
	Normal = "normal"
	// Alto: the 4 lenses always, a review always required (unless the
	// change is empty or only documentation), and the mediums of a complete
	// review block.
	Alto = "alto"
)

// Vocabulario is every riesgo a card may declare, in this order: Bajo,
// Normal, Alto.
var Vocabulario = []string{Bajo, Normal, Alto}

// Marcas are the marks of the default risk paths: RutasBase().DeRiesgo is
// `**/*<m>*` for each m, in this order. With the implementation of
// riesgo-efectivo-y-tamano they are auth, login, password, passwd, secret,
// token, cred, crypt, signin, signature, pago, payment, billing, invoice,
// dte, permission, permiso, sandbox, sudo: today's, with `sign` (which also
// caught design, assign and signal) replaced by signin and signature, in its
// place. The skeleton still has `sign`.
var Marcas = []string{
	"auth", "login", "password", "passwd", "secret", "token", "cred",
	"crypt", "sign", "pago", "payment", "billing", "invoice", "dte",
	"permission", "permiso", "sandbox", "sudo",
}

// Ruta is the first of files, in their order, whose path in lower case
// contains one of Marcas, returned as it was given; "" when none does (an
// empty or nil files included).
//
// The implementation of riesgo-efectivo-y-tamano removes it: a risk path is
// a file that matches rutas.de_riesgo (Coincide, CoincideAlguno), and the
// tests of CA-618 are re-expressed over Clasificador.Evaluar. It stays in
// the skeleton only so that today's tests still compile.
func Ruta(files []string) string {
	for _, f := range files {
		l := strings.ToLower(f)
		for _, m := range Marcas {
			if strings.Contains(l, m) {
				return f
			}
		}
	}
	return ""
}

// Efectivo is the riesgo hoom applies for a card that declares declarado and
// whose whole change is files:
//
//   - declarado Bajo and Ruta(files) not empty: Normal, with the motivo
//     `el cambio toca <ruta>: riesgo normal` (<ruta> is Ruta(files));
//   - any other case: declarado ("" gives Normal) and an empty motivo.
//
// The riesgo goes up by itself and never down by itself: Normal and Alto are
// returned as they are, with or without a risk path. A declarado outside
// Vocabulario is returned as it is too: validating it is Validar's job.
//
// The implementation of riesgo-efectivo-y-tamano removes it: the effective
// riesgo is Clasificador.Evaluar (a risk path gives Alto, also with Bajo,
// and the motivo is never empty), and the tests of CA-618 are re-expressed
// over it. It stays in the skeleton only so that today's tests still compile.
func Efectivo(declarado string, files []string) (efectivo, motivo string) {
	if declarado == "" {
		return Normal, ""
	}
	if declarado == Bajo {
		if p := Ruta(files); p != "" {
			return Normal, fmt.Sprintf("el cambio toca %s: riesgo normal", p)
		}
	}
	return declarado, ""
}

// Validar says whether r is a riesgo a card may declare: nil for "" (the
// missing key, Normal) and for each value of Vocabulario; for anything else
// the error whose text is exactly
// `riesgo "<r>" fuera del vocabulario (bajo, normal, alto)` (r quoted as %q
// quotes it), the text item.Parse gives.
func Validar(r string) error {
	if r == "" {
		return nil
	}
	for _, v := range Vocabulario {
		if r == v {
			return nil
		}
	}
	return fmt.Errorf("riesgo %q fuera del vocabulario (%s)", r, strings.Join(Vocabulario, ", "))
}
