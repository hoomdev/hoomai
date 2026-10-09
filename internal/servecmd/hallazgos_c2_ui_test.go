// Tests de regresion de la review de 4 lentes de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md, en la UI embebida. Son estaticos,
// como los de C2, C3 y C4 (tablero_ui_test.go): afirman lo que esta y lo que
// no esta en tablero.js, con los nombres del CONTRATO (las claves del JSON
// del detalle), nunca con nombres internos del archivo. No pintan: que se vea
// bien, y que el modo normal no lo muestre, se comprueba a mano con hoom
// serve.
//
//   - 20261009T192554_05f9c5 (medium, reliability; CA-313 y CA-319): en modo
//     experto el detalle suma las rutas de paths (item, approval, verdict,
//     reviews y findings, ademas de las que ya mostraba) y los ids de los
//     registros de review (reviews). Hoy tablero.js no nombra reviews en
//     ningun lado, y lo unico que lee de paths no incluye verdict, reviews ni
//     findings.
package servecmd

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// c2SinComentarios devuelve js con los comentarios en blanco: los bloques
// /* ... */ y lo que sigue a un // que no esta dentro de una cadena (comillas
// pares antes del //, la regla de huFueraDeComentarios). Un nombre que solo
// aparece en un comentario no es codigo que lo lea.
func c2SinComentarios(js string) string {
	js = regexp.MustCompile(`/\*[\s\S]*?\*/`).ReplaceAllStringFunc(js, func(c string) string {
		return regexp.MustCompile(`[^\n]`).ReplaceAllString(c, " ")
	})
	lineas := strings.Split(js, "\n")
	for n, linea := range lineas {
		for i := strings.Index(linea, "//"); i >= 0; {
			antes := linea[:i]
			if strings.Count(antes, `"`)%2 == 0 && strings.Count(antes, "'")%2 == 0 && strings.Count(antes, "`")%2 == 0 {
				lineas[n] = antes
				break
			}
			sig := strings.Index(linea[i+2:], "//")
			if sig < 0 {
				break
			}
			i += 2 + sig
		}
	}
	return strings.Join(lineas, "\n")
}

// Hallazgo 20261009T192554_05f9c5. CA-313 y CA-319: el detalle trae los
// registros de review de la tarjeta en reviews y sus archivos en
// paths.reviews, y el modo experto suma sus ids y sus rutas. Para mostrarlos
// tablero.js tiene que leer esa clave: la nombra en su codigo (no solo en un
// comentario). Hoy no la nombra en ningun lado.
func TestHallazgo_05f9c5_TableroLeeLasReviewsDelDetalle(t *testing.T) {
	_, js := tbUI(t)
	if !huPalabra(c2SinComentarios(js), "reviews") {
		t.Fatal(`05f9c5 CA-313: tablero.js lee la clave "reviews" del detalle (los registros de review y paths.reviews) para sumar sus ids y sus rutas en modo experto: no la nombra en su codigo`)
	}
}

// Hallazgo 20261009T192554_05f9c5. CA-313 y CA-319: el codigo de tablero.js
// que pinta las rutas (las funciones que nombran paths, o un ayudante al que
// llaman, con la regla de huUsa) nombra cada ruta del contrato que faltaba:
// item, approval, verdict, reviews y findings. Tambien vale recorrer paths
// entero (Object.entries, Object.keys o for...in): asi salen todas sin
// nombrarlas. Hoy ese codigo no nombra verdict, reviews ni findings (item y
// approval ya los nombra por otros datos del detalle: para esos dos este test
// no distingue).
func TestHallazgo_05f9c5_ElPintorDeRutasNombraCadaRutaDelContrato(t *testing.T) {
	_, js := tbUI(t)
	sin := c2SinComentarios(js)
	cuerpos := huCuerpos(sin)
	var pintores []string
	recorre := false
	for nombre, cuerpo := range cuerpos {
		if !huPalabra(cuerpo, "paths") {
			continue
		}
		pintores = append(pintores, nombre)
		if regexp.MustCompile(`Object\.(entries|keys|values)\(|\bfor\s*\([^)]*\bin\b`).MatchString(cuerpo) {
			recorre = true
		}
	}
	if len(pintores) == 0 {
		t.Fatal(`05f9c5 CA-313: tablero.js lee "paths" del detalle en una funcion con nombre`)
	}
	if recorre {
		return // recorre paths entero: todas las rutas salen
	}
	var faltan []string
	for _, clave := range []string{"item", "approval", "verdict", "reviews", "findings"} {
		nombrada := false
		for _, f := range pintores {
			nombrada = nombrada || huUsa(sin, f, clave)
		}
		if !nombrada {
			faltan = append(faltan, clave)
		}
	}
	sort.Strings(faltan)
	if len(faltan) != 0 {
		t.Fatalf("05f9c5 CA-313: en modo experto el detalle suma las rutas paths.item, paths.approval, paths.verdict, paths.reviews y paths.findings: el codigo de tablero.js que lee paths (ni un ayudante al que llama) no nombra %s",
			strings.Join(faltan, ", "))
	}
}
