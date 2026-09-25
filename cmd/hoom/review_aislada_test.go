// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-395): `--effort <e>` en `hoom review` y en su ayuda. El E2E que corre
// el binario con --effort vive en internal/reviewcmd; este no compila nada y
// no se omite con -short.
package main

import (
	"strings"
	"testing"
)

// CA-395: el bloque "Flags de review" de la ayuda nombra --effort.
func TestCA395_AyudaDeReviewNombraEffort(t *testing.T) {
	i := strings.Index(usage, "Flags de review")
	if i < 0 {
		t.Fatal("CA-395: la ayuda tiene el bloque 'Flags de review'")
	}
	bloque := usage[i:]
	if j := strings.Index(bloque, "\n\n"); j >= 0 {
		bloque = bloque[:j]
	}
	if !strings.Contains(bloque, "--effort") {
		t.Fatalf("CA-395: la ayuda de review nombra --effort:\n%s", bloque)
	}
}
