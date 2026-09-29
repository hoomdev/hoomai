// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-429): la ayuda de `hoom review` documenta --desde y --delta. El README
// lo verifica el comando declarado en el spec; la ayuda, este test. No
// compila nada y no se omite con -short.
package main

import (
	"strings"
	"testing"
)

// CA-429: el bloque "Flags de review" de la ayuda nombra --desde y --delta.
func TestCA429_AyudaDeReviewNombraDesdeYDelta(t *testing.T) {
	i := strings.Index(usage, "Flags de review")
	if i < 0 {
		t.Fatal("CA-429: la ayuda tiene el bloque 'Flags de review'")
	}
	bloque := usage[i:]
	if j := strings.Index(bloque, "\n\n"); j >= 0 {
		bloque = bloque[:j]
	}
	for _, flag := range []string{"--desde", "--delta"} {
		if !strings.Contains(bloque, flag) {
			t.Fatalf("CA-429: la ayuda de review nombra %s:\n%s", flag, bloque)
		}
	}
}
