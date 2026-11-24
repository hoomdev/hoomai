// Package tarjetas is how a package that cannot import internal/item reads
// a card (proporcion-por-riesgo). approval measures a spec against its
// card's tope (approval.CalibreDe), but item's own tests import approval,
// so approval importing item would be an import cycle in those tests. item
// registers its reader here when it is linked (item.TarjetaDe, in its
// init); approval asks this package.
package tarjetas

import "errors"

// Leida is what the measure of a spec reads of its card: the declared
// riesgo (never "": riesgo.Normal without the key) and the card's tope of
// its spec, nil when the card does not have it.
type Leida struct {
	Riesgo        string
	TopePalabras  *int
	TopeCriterios *int
}

// Lector reads the card slug as item.TarjetaDe does from the tree arbol: hay
// false, with no error, when there is no item; an item that is there and
// cannot be read is err (hay true).
type Lector func(arbol, slug string) (t Leida, hay bool, err error)

var lector Lector

// Registrar is item's registration of its reader (in its init).
func Registrar(l Lector) { lector = l }

// ErrSinLector is Leer's error when no reader was registered (a binary that
// does not link internal/item): the card counts as unreadable, so the
// measure fails closed instead of skipping the card.
var ErrSinLector = errors.New("el lector de tarjetas no esta registrado (internal/item no esta enlazado)")

// Leer reads the card slug from arbol with the registered reader.
func Leer(arbol, slug string) (Leida, bool, error) {
	if lector == nil {
		return Leida{}, true, ErrSinLector
	}
	return lector(arbol, slug)
}
