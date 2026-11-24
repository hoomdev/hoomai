package approval

import (
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/spec"
)

// CalibreDe (proporcion-por-riesgo) is how hoom measures the spec at
// specPath against its tope, the same for `hoom verify` (spec.GatesCon), the
// approval (Aprobar), `hoom spec status` and the board:
//
//   - its card: the spec .hoom/specs/<slug>.md (under root or under any
//     tree) has the card .hoom/items/<slug>.yaml, read with item.TarjetaDe
//     from root (the main checkout's item first). A spec in another path,
//     or without item, has no card (Calibre.Tarjeta ""): riesgo normal. An
//     item that cannot be read is Calibre.Ilegible (with riesgo alto);
//   - its riesgo: the card's DECLARED one, riesgo.Normal without the key;
//   - its tope: per measure, the card's tope_palabras and tope_criterios
//     when it has them (Calibre.PalabrasDeLaTarjeta,
//     CriteriosDeLaTarjeta), else m.TopeDelSpec(riesgo); the citations
//     always m's. A nil m is the defaults (manifest.TopesPorDefecto);
//   - Aprobado: Status(root, specPath) is StatusApproved.
//
// specPath is absolute or relative to root.
func CalibreDe(m *manifest.Manifest, root, specPath string) spec.Calibre {
	return spec.Calibre{}
}

// Aprobar (proporcion-por-riesgo) is a person's approval of the spec at
// specPath, measured: the ONE function `hoom spec approve`, `hoom aprobar`
// and the approvals of the Studio and the board call. With
// CalibreDe(m, root, specPath):
//
//   - a spec that is not Aprobado and passes its tope (spec.LineasDelTope
//     not empty) is refused: no record is written, and the error's text is
//     those lines joined by "\n";
//   - a card that cannot be read is refused the same way, with
//     `no se puede leer la tarjeta <slug>: <error>`
//     (spec.FormatoTarjetaIlegible);
//   - anything else is Approve(root, specPath), exactly: the record, or
//     already when this content was approved (an approved spec is not
//     measured).
//
// Raising the card's tope (`hoom item set <slug> --tope-palabras`) and
// approving again approves it. Approve stays the raw writer of records.
func Aprobar(m *manifest.Manifest, root, specPath string) (rec Record, already bool, err error) {
	return Record{}, false, nil
}
