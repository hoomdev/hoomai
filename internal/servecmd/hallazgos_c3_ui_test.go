// Tests de regresion de la mitad 2 de la review de 4 lentes de la cabina C3
// sobre .hoom/specs/acciones-desde-la-tarjeta.md, en la UI embebida. Son
// estaticos, como los de hallazgos_c2_ui_test.go: afirman lo que esta en
// tablero.js y acciones.js. Que se vea y funcione se comprueba a mano con hoom
// serve.
//
//   - 20261014T203733_48feb7 (medium; CA-353): el espejo de la terminal se
//     pide 1000 ms despues de cada respuesta MIENTRAS la seccion esta abierta.
//     Con la pagina oculta deja de pedirse; al volver visible, el manejador de
//     visibilitychange tiene que volver a pedirlo si el detalle y la seccion
//     Terminal estan abiertos. Hoy reinicia el tablero, el detalle y el run,
//     pero no la terminal.
//   - 20261014T203738_933c74 (low; CA-354): el enlace "Leer el spec" del
//     dialogo de aprobar deja el detalle en la pestana Que (data-pane="que"),
//     que es donde esta el spec, aunque el detalle haya quedado en otra.
package servecmd

import (
	"regexp"
	"strings"
	"testing"
)

// h3Bloque devuelve lo que hay entre la primera llave que abre en js desde
// desde y la que la cierra, saltando las cadenas (con comillas simples,
// dobles o invertidas) y los comentarios.
func h3Bloque(js string, desde int) (string, bool) {
	i := strings.IndexByte(js[desde:], '{')
	if i < 0 {
		return "", false
	}
	i += desde
	nivel := 0
	for j := i; j < len(js); j++ {
		switch c := js[j]; c {
		case '"', '\'', '`':
			for j++; j < len(js) && js[j] != c; j++ {
				if js[j] == '\\' {
					j++
				}
			}
		case '/':
			if j+1 < len(js) && js[j+1] == '/' {
				for j < len(js) && js[j] != '\n' {
					j++
				}
			} else if j+1 < len(js) && js[j+1] == '*' {
				if k := strings.Index(js[j+2:], "*/"); k >= 0 {
					j += k + 3
				} else {
					j = len(js)
				}
			}
		case '{':
			nivel++
		case '}':
			nivel--
			if nivel == 0 {
				return js[i+1 : j], true
			}
		}
	}
	return "", false
}

// h3CuerpoDe devuelve el cuerpo de la funcion declarada con ese nombre.
func h3CuerpoDe(js, nombre string) (string, bool) {
	for _, d := range atDeclaracion.FindAllStringSubmatchIndex(js, -1) {
		n := ""
		if d[2] >= 0 {
			n = js[d[2]:d[3]]
		} else {
			n = js[d[4]:d[5]]
		}
		if n == nombre {
			return h3Bloque(js, d[1])
		}
	}
	return "", false
}

// Hallazgo 48feb7 (CA-353): el manejador de visibilitychange de tablero.js
// vuelve a pedir el espejo de la terminal (llama a la funcion que pide
// /api/board/{slug}/terminal), y ese pedido depende de que la seccion Terminal
// (#tb-terminal) este abierta: la condicion esta en el manejador o en la
// funcion que pide.
func TestHallazgo_48feb7_AlVolverVisibleSigueElEspejoDeLaTerminal(t *testing.T) {
	_, tablero, _ := atUI(t)
	js := c2SinComentarios(tablero)

	i := strings.Index(js, "/terminal")
	if i < 0 {
		t.Fatal("48feb7 CA-353: fixture: tablero.js pide /api/board/{slug}/terminal")
	}
	pide := atFuncionQueEncierra(js, i)
	if pide == "" {
		t.Fatal("48feb7 CA-353: fixture: el pedido de /terminal esta dentro de una funcion con nombre")
	}
	cuerpoPide, _ := h3CuerpoDe(js, pide)

	k := regexp.MustCompile(`addEventListener\(\s*["']visibilitychange["']`).FindStringIndex(js)
	if k == nil {
		t.Fatal(`48feb7 CA-353: fixture: tablero.js escucha "visibilitychange"`)
	}
	manejador, ok := h3Bloque(js, k[1])
	if !ok {
		t.Fatal("48feb7 CA-353: fixture: el manejador de visibilitychange tiene cuerpo")
	}
	if !regexp.MustCompile(`\b` + regexp.QuoteMeta(pide) + `\s*\(`).MatchString(manejador) {
		t.Fatalf("48feb7 CA-353: al volver visible la pagina, el manejador de visibilitychange vuelve a pedir el espejo de la terminal (llama a %s):\n%s", pide, manejador)
	}
	abierta := regexp.MustCompile(`["']tb-terminal["']\s*\)\s*\.open\b`)
	if !abierta.MatchString(manejador) && !abierta.MatchString(cuerpoPide) {
		t.Fatalf("48feb7 CA-353: el espejo se vuelve a pedir solo con la seccion Terminal abierta ($(\"tb-terminal\").open en el manejador o en %s)", pide)
	}
}

// Hallazgo 933c74 (CA-354): el click del boton "Leer el spec" del dialogo de
// aprobar (acciones.js) deja el detalle en la pestana Que: su manejador nombra
// data-pane="que".
func TestHallazgo_933c74_LeerElSpecDejaElDetalleEnQue(t *testing.T) {
	_, _, acciones := atUI(t)
	js := c2SinComentarios(acciones)

	m := regexp.MustCompile(`id=["']([\w-]+)["'][^>]*>\s*Leer el spec\s*<`).FindStringSubmatch(js)
	if m == nil {
		t.Fatal(`933c74 CA-354: fixture: el dialogo de aprobar tiene un boton "Leer el spec" con id`)
	}
	k := regexp.MustCompile(`["']` + regexp.QuoteMeta(m[1]) + `["']\s*\)\s*\.addEventListener\(\s*["']click["']`).FindStringIndex(js)
	if k == nil {
		t.Fatalf("933c74 CA-354: fixture: el boton #%s tiene un manejador de click", m[1])
	}
	manejador, ok := h3Bloque(js, k[1])
	if !ok {
		t.Fatalf("933c74 CA-354: fixture: el manejador de click de #%s tiene cuerpo", m[1])
	}
	if !regexp.MustCompile(`data-pane=\\?["']?que\\?["']?\]`).MatchString(manejador) {
		t.Fatalf("933c74 CA-354: \"Leer el spec\" deja el detalle en la pestana Que (data-pane=\"que\"), aunque haya quedado en otra:\n%s", manejador)
	}
}
