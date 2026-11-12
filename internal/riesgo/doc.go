// Package riesgo is the riesgo of a card: how much review its change gets
// and which of its findings block (spec ciclo-de-review-acotado). A person
// declares it in the item (the optional key `riesgo` of
// .hoom/items/<slug>.yaml); a missing key is Normal, and Normal is exactly
// what hoom did before the riesgo existed.
//
// The vocabulary is Vocabulario: Bajo, Normal, Alto, in that order.
//
// The riesgo hoom applies is the effective one (Efectivo): the declared one,
// except that a Bajo whose change touches a risk path (Ruta) is Normal. The
// riesgo goes up by itself and never down by itself: Normal and Alto stay
// what they are, with or without a risk path.
//
// A risk path is a file whose path, in lower case, contains one of Marcas as
// a substring: the same list the lens rule of `hoom review` has always used
// (coarse on purpose: `sign` also catches `design`). The files are always
// the files of the whole change against the base: in `hoom review`, those of
// the whole change also in a delta; on the board, the changed_files of the
// card's verdict.
//
// The package imports no other package of hoom, so the item, the findings,
// the review and the board all read the same definition; finding cannot
// import reviewcmd, which already imports finding.
package riesgo
