package hoomfs

import (
	"path/filepath"
	"testing"
)

// Los casos de exclusion, soltar tarde o repetido y dueno que muere viven en
// lock_contrato_test.go (hallazgo 20261020T202736_a4af26: una sola suite por
// comportamiento). Aca queda solo lo que esa suite no cubre.

// LockPath vive bajo .hoom/cache/locks/: local e ignorado, nunca junto a la
// evidencia (.hoom/items/ queda con sus items y nada mas).
func TestLock_LockPathEnCache(t *testing.T) {
	got := filepath.ToSlash(LockPath("/r", "item-x"))
	if got != "/r/.hoom/cache/locks/item-x.lock" {
		t.Fatalf("LockPath = %s", got)
	}
}
