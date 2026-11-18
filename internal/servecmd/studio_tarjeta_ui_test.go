// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md
// (CA-741 y CA-742, la parte de Tarjeta): la pantalla Tarjeta, que tambien
// vive en ui/ahora.js. Son estaticos, como los de Ahora
// (studio_ahora_ui_test.go): afirman los textos del contrato y los campos
// del JSON que la pantalla lee. Los titulos fijos pueden estar en el marcado
// de index.html o en ahora.js; lo que depende de la tarjeta, en ahora.js.
package servecmd

import (
	"regexp"
	"testing"
)

// CA-741: Tarjeta tiene "← Ahora", el Camino con listo, falta y
// aria-current="step" en la entrada actual, AHORA — QUÉ ESTÁ HACIENDO,
// SIGUE — QUÉ PASA DESPUÉS, Historia — paso a paso (y su vacio), Topes —
// hasta dónde llega solo, Evidencia — lo último comprobado (con FAIL
// esperado y hallazgos que bloquean:) y el comando para apagar el piloto.
func TestCA741_PantallaTarjeta(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-741")
	cod := stCodigo(ahora)
	stTiene(t, "CA-741", "la UI (index.html o ahora.js)", html+cod,
		"← Ahora",
		"Camino — de tu firma a tu aceptación",
		"AHORA — QUÉ ESTÁ HACIENDO",
		"SIGUE — QUÉ PASA DESPUÉS",
		"Historia — paso a paso",
		"Topes — hasta dónde llega solo",
		"Evidencia — lo último comprobado")
	stTiene(t, "CA-741", "ahora.js", cod,
		"listo", "falta",
		"FAIL esperado", "hallazgos que bloquean: ",
		"para apagar el piloto: hoom item set ", " --auto no",
		"Todavía no hay historia para esta tarjeta.",
		"piloto encendido", "CORRIENDO", "DETENIDA")
	aria := regexp.MustCompile(`aria-current="step"|setAttribute\(\s*["']aria-current["']\s*,\s*["']step["']\s*\)|ariaCurrent\s*=\s*["']step["']`)
	if !aria.MatchString(cod) {
		t.Fatal(`CA-741: la entrada actual del camino lleva aria-current="step"`)
	}
	stUsa(t, "CA-741", "ahora.js", cod, "camino", "nombre", "estado", "texto", "grupo", "entries", "plain", "provider", "ref")
}

// CA-741: AHORA y SIGUE de la Tarjeta: la frase de ahora y su gris; con
// running, el badge CORRIENDO, "<provider> · empezó <HH:MM> · hace
// <tiempo>", la fila "tiempo de este trabajo" con "<min> de <limite> min" y
// su barra --hd-azul, y "sobre <envelope_id>"; SIGUE con su gris en --mono y
// --hd-gris; el borde izquierdo --hd-verde con running.
func TestCA741_AhoraYSigueDeLaTarjeta(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-741")
	cod := stCodigo(ahora)
	stTiene(t, "CA-741", "ahora.js", cod, "empezó ", "hace ", "tiempo de este trabajo", " min", "sobre ")
	stUsa(t, "CA-741", "ahora.js", cod, "ahora", "sigue", "frase", "gris", "running", "provider", "started_at", "timeout_s", "envelope_id")
	stVar(t, "CA-741", html, ahora, "--hd-verde", "--hd-azul", "--hd-gris", "--mono")
}

// CA-741: Topes y Evidencia de la Tarjeta: "presupuesto" con "<cost_usd> de
// <budget_usd> USD" a dos decimales y su barra (--hd-verde y --hd-amarillo),
// "sin dato de costo · tope <budget_usd> USD" con cost_usd null; el
// veredicto con su id corto y VERDE/ROJO, una fila por gate con su status en
// MAYUSCULA (PASS --hd-verde-gate, FAIL y ERROR --hd-rojo, FAIL esperado
// --hd-amarillo, los demas --hd-gris), "hallazgos que bloquean: <n>" con
// evidence.blocking_findings, pendientes.texto o "sin pendientes", y
// "todavía no hay veredicto".
func TestCA741_TopesYEvidencia(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-741")
	cod := stCodigo(ahora)
	stTiene(t, "CA-741", "ahora.js", cod,
		"presupuesto", " USD", "sin dato de costo · tope ",
		"veredicto ", "toUpperCase(", "FAIL esperado",
		"sin pendientes", "todavía no hay veredicto")
	if !regexp.MustCompile(`toFixed\(\s*2\s*\)|FractionDigits\s*:\s*2\b`).MatchString(cod) {
		t.Fatal("CA-741: el presupuesto va a dos decimales")
	}
	stUsa(t, "CA-741", "ahora.js", cod,
		"spend", "budget_usd", "cost_usd", "verdict", "gates", "name", "status",
		"red", "evidence", "blocking_findings", "pendientes", "texto")
	stVar(t, "CA-741", html, ahora, "--hd-verde-gate", "--hd-rojo", "--hd-amarillo", "--hd-gris", "--hd-verde")
}

// CA-741 (caso limite "un running sin timeout_s"): sin timeout_s no hay fila
// de tiempo ni " · tope": los dos textos dependen de timeout_s (se leen junto
// a el).
func TestCA741_RunningSinTimeoutNoTieneTope(t *testing.T) {
	_, _, ahora := stLeerUI(t, "CA-741")
	cod := stCodigo(ahora)
	usos := regexp.MustCompile(`\btimeout_s\b`).FindAllStringIndex(cod, -1)
	for _, texto := range []string{" · tope ", "tiempo de este trabajo"} {
		pos := regexp.MustCompile(regexp.QuoteMeta(texto)).FindAllStringIndex(cod, -1)
		if len(pos) == 0 {
			t.Fatalf("CA-741: ahora.js tiene %q", texto)
		}
		cerca := false
		for _, p := range pos {
			for _, u := range usos {
				if u[0] < p[0] && p[0]-u[0] <= 600 {
					cerca = true
				}
			}
		}
		if !cerca {
			t.Fatalf("CA-741: %q solo se muestra con running.timeout_s (sin timeout_s no hay tope)", texto)
		}
	}
}

// CA-741 (caso limite "una pagina muy angosta"): el camino de la Tarjeta es
// una grilla de 8 columnas que hace scroll horizontal.
func TestCA741_CaminoEnGrillaConScroll(t *testing.T) {
	html, _, ahora := stLeerUI(t, "CA-741")
	if !regexp.MustCompile(`repeat\(\s*8\s*,`).MatchString(html + ahora) {
		t.Fatal("CA-741: el camino es una grilla de 8 columnas (grid-template-columns: repeat(8, ...))")
	}
	if !regexp.MustCompile(`overflow-x\s*:\s*(?:auto|scroll)`).MatchString(stEstilo(t, html)) {
		t.Fatal("CA-741: el camino hace scroll horizontal en una pantalla angosta (overflow-x)")
	}
}

// CA-742: Tarjeta muestra "no existe la tarjeta «<slug>»" ante un 404 de
// /api/board/{slug} (j de index.html deja el status en el error) y "no se
// pudo leer la tarjeta: <error>" ante otro error.
func TestCA742_ErroresDeTarjeta(t *testing.T) {
	_, _, ahora := stLeerUI(t, "CA-742")
	cod := stCodigo(ahora)
	stTiene(t, "CA-742", "ahora.js", cod, "no existe la tarjeta «", "no se pudo leer la tarjeta: ")
	if !regexp.MustCompile(`\bstatus\b[^;\n]{0,20}\b404\b|\b404\b[^;\n]{0,20}\bstatus\b`).MatchString(cod) {
		t.Fatal("CA-742: la Tarjeta distingue el 404 por el status del error (e.status === 404)")
	}
}
