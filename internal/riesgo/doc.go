// Package riesgo is the riesgo of a card: how much review its change gets
// and which of its findings block (specs ciclo-de-review-acotado and
// riesgo-efectivo-y-tamano). A person declares it in the item (the optional
// key `riesgo` of .hoom/items/<slug>.yaml); a missing key is Normal.
//
// The vocabulary is Vocabulario: Bajo, Normal, Alto, in that order.
//
// # The files of a change and their class
//
// The files of a change are those of `git diff --numstat --no-renames` from
// the merge-base of the base and HEAD against the working tree, plus the
// untracked files git does not ignore (gitx.LineasDelCambio), each with its
// lines: insertions plus deletions (a binary 0, an untracked file its
// lines). Each file has one class (Clasificador.Clase), the first that
// holds: ClaseHoom (under .hoom/), ClaseGenerado (rutas.generadas),
// ClasePrueba (the test-writer's territory) and ClaseProducto (the rest,
// hoom.yaml included). The size of a change (Tamano) is three numbers,
// producto, pruebas and hoom; a generado adds to none. Every threshold uses
// producto alone (UmbralProducto).
//
// # Rutas
//
// The three lists of patterns of the section `rutas` of hoom.yaml (Rutas):
// de_riesgo, de_bajo_riesgo and generadas. A project without the section
// gets the defaults of its profile over RutasBase (profiles.Rutas); the
// manifest decodes the section (manifest.Rutas) and profiles.Clasificador
// resolves it. A pattern (Coincide) has the syntax of
// agents.<rol>.write.allow and matches a path or one of its parent
// directories, case-insensitively.
//
// # The effective riesgo
//
// The riesgo hoom applies is the effective one (Clasificador.Evaluar): fixed
// rules over the card (Tarjeta: whether it can be read, its tipo and its
// declared riesgo) and the files of the change. hoom only raises it: no rule
// gives an effective riesgo below the declared one, and lowering it is a
// person's job (`hoom item set`). A file of class hoom or generado never
// raises it. The verdict keeps it (Evaluacion, under `riesgo`), with its
// motivo.
//
// The package imports no other package of hoom, so the item, the findings,
// the verdict, the review and the board all read the same definition: the
// lists arrive as parameters.
package riesgo
