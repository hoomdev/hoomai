// Tests adversariales del spec .hoom/specs/base-de-la-review.md (CA-434): la
// ayuda de `hoom review` documenta --base. El README lo verifica el comando
// declarado en el spec; la ayuda, este test. El E2E que corre el binario con
// --base vive en internal/reviewcmd; este no compila nada y no se omite con
// -short.
package main

import (
	"strings"
	"testing"
)

// CA-434: el bloque "Flags de review" de la ayuda nombra --base.
func TestCA434_AyudaDeReviewNombraBase(t *testing.T) {
	i := strings.Index(usage, "Flags de review")
	if i < 0 {
		t.Fatal("CA-434: la ayuda tiene el bloque 'Flags de review'")
	}
	bloque := usage[i:]
	if j := strings.Index(bloque, "\n\n"); j >= 0 {
		bloque = bloque[:j]
	}
	if !strings.Contains(bloque, "--base") {
		t.Fatalf("CA-434: la ayuda de review nombra --base:\n%s", bloque)
	}
}
