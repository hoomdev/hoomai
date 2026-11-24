// Regresion del hallazgo cc7db0 (proporcion-por-riesgo, decision de la
// persona): el contrato 00 dice la regla de la nota de riesgo que hoom agent
// aplica (spec, CA-799): con la tarjeta ilegible el riesgo es alto; sin
// tarjeta o sin la clave, normal. Como el marcador verifica de CA-800, sobre
// la plantilla embebida.
package agents

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestHallazgo_cc7db0_ContratoOrquestadorNombraLaTarjetaIlegible(t *testing.T) {
	raw, err := fs.ReadFile(assetsFS, "assets/agents/00-orquestador.md")
	if err != nil {
		t.Fatal(err)
	}
	// la regla puede partirse en lineas: se compara con los blancos colapsados
	contrato := regexp.MustCompile(`\s+`).ReplaceAllString(string(raw), " ")
	for _, regla := range []string{
		"Riesgo declarado de la tarjeta",
		"con la tarjeta ilegible, alto",
		"sin tarjeta o sin la clave, normal",
	} {
		if !strings.Contains(contrato, regla) {
			t.Fatalf("hallazgo cc7db0: el contrato 00 embebido dice %q", regla)
		}
	}
}
