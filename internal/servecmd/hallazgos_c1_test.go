// Tests de regresion de la review cruzada de la cabina sobre la UI del
// Studio:
//
//   - 20260924T194924_047f0e (low; C3 CA-354): acciones.js decide si una
//     accion abre el dialogo de pedir al rol con el role que trae la accion
//     del backend (a.role), no con una lista local de ids (ACC_ROL): "la
//     pagina no decide", muestra lo que trae actions. Chequeo estatico, como
//     los de CA-354 y CA-355.
package servecmd

import (
	"regexp"
	"strings"
	"testing"
)

// h2Nombre es el nombre de la declaracion d (un match de atDeclaracion).
func h2Nombre(js string, d []int) string {
	if d[2] >= 0 {
		return js[d[2]:d[3]]
	}
	return js[d[4]:d[5]]
}

// h2Cuerpo es el texto de la funcion nombre de js: desde su declaracion hasta
// la declaracion siguiente ("" si no esta declarada).
func h2Cuerpo(js, nombre string) string {
	for _, d := range atDeclaracion.FindAllStringSubmatchIndex(js, -1) {
		if h2Nombre(js, d) != nombre {
			continue
		}
		fin := len(js)
		if sig := atDeclaracion.FindStringIndex(js[d[1]:]); sig != nil {
			fin = d[1] + sig[0]
		}
		return js[d[0]:fin]
	}
	return ""
}

// h2QuienAbreElDialogoDeRol son las funciones de acciones.js que llaman a
// dialogoRol( (sin contar su propia declaracion): las que deciden si una
// accion abre el dialogo de pedir al rol.
func h2QuienAbreElDialogoDeRol(t *testing.T, js string) []string {
	t.Helper()
	llamada := regexp.MustCompile(`\bdialogoRol\(`)
	declaracion := regexp.MustCompile(`function\s*$`)
	var quienes []string
	visto := map[string]bool{}
	for _, m := range llamada.FindAllStringIndex(js, -1) {
		if declaracion.MatchString(js[:m[0]]) {
			continue
		}
		q := atFuncionQueEncierra(js, m[0])
		if q == "" {
			t.Fatal("CA-354: la llamada a dialogoRol( vive en una funcion con nombre de acciones.js")
		}
		if q == "dialogoRol" || visto[q] {
			continue
		}
		visto[q] = true
		quienes = append(quienes, q)
	}
	if len(quienes) == 0 {
		t.Fatal("CA-354: acciones.js abre el dialogo de rol (provider, modelo, presupuesto y pedido) con dialogoRol(")
	}
	return quienes
}

// CA-354 (C3). Hallazgo 20260924T194924_047f0e: la decision de abrir el
// dialogo de pedir al rol (pedir-*, reanudar, relanzar) sale del role que
// trae la accion del backend: acciones.js no usa ACC_ROL.has( y la funcion
// que llama a dialogoRol( mira a.role, sin una lista local de ids de accion
// de rol ("pedir-...").
func TestCA354_H2_DialogoDeRolDecidePorElRoleDeLaAccion(t *testing.T) {
	_, _, acc := atUI(t)
	if strings.Contains(acc, "ACC_ROL.has(") {
		t.Error("CA-354: acciones.js no decide el dialogo de rol con una lista local: no contiene ACC_ROL.has(")
	}
	role := regexp.MustCompile(`\ba\.role\b`)
	idDeRol := regexp.MustCompile("[\"'`](?:pedir-[a-z-]+|reanudar|relanzar)[\"'`]")
	for _, quien := range h2QuienAbreElDialogoDeRol(t, acc) {
		cuerpo := h2Cuerpo(acc, quien)
		if cuerpo == "" {
			t.Fatalf("CA-354: no encontre el cuerpo de %s en acciones.js", quien)
		}
		if !role.MatchString(cuerpo) {
			t.Fatalf("CA-354: %s decide si abre el dialogo de pedir al rol con a.role (el role que trae la accion), no con una lista local:\n%s",
				quien, cuerpo)
		}
		if id := idDeRol.FindString(cuerpo); id != "" {
			t.Fatalf("CA-354: %s no tiene una lista local de acciones de rol (contiene %s): la pagina no decide:\n%s",
				quien, id, cuerpo)
		}
	}
}
