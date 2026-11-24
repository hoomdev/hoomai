package approval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/tarjetas"
)

// CalibreDe (proporcion-por-riesgo) is how hoom measures the spec at
// specPath against its tope, the same for `hoom verify` (spec.GatesCon), the
// approval (Aprobar), `hoom spec status` and the board:
//
//   - its card: the spec <root>/.hoom/specs/<slug>.md has the card
//     .hoom/items/<slug>.yaml, read with item.TarjetaDe from root (the main
//     checkout's item first): root is the spec's tree. A spec in another
//     path (another tree's .hoom/specs included: finding 647f9a), or
//     without item, has no card (Calibre.Tarjeta ""): riesgo normal. An
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
	ruta := rutaEn(root, specPath)
	c := calibre(m, root, ruta)
	if st, _, err := Status(root, ruta); err == nil && st == StatusApproved {
		c.Aprobado = true
	}
	return c
}

// rutaEn is specPath when absolute, else relative to root.
func rutaEn(root, specPath string) string {
	if filepath.IsAbs(specPath) {
		return specPath
	}
	return filepath.Join(root, specPath)
}

// calibre is CalibreDe of the spec at ruta without Aprobado: its card, its
// riesgo and its tope.
func calibre(m *manifest.Manifest, root, ruta string) spec.Calibre {
	c := spec.Calibre{Riesgo: riesgo.Normal}
	var tarjeta tarjetas.Leida
	if slug, ok := tarjetaDelSpec(root, ruta); ok {
		// item.TarjetaDe, through tarjetas: item's tests import approval
		it, hay, err := tarjetas.Leer(root, slug)
		switch {
		case err != nil:
			c.Tarjeta, c.Ilegible, c.Riesgo = slug, err.Error(), riesgo.Alto
		case hay:
			c.Tarjeta, c.Riesgo, tarjeta = slug, it.Riesgo, it
			if c.Riesgo == "" {
				c.Riesgo = riesgo.Normal
			}
		}
	}
	c.Tope = m.TopeDelSpec(c.Riesgo)
	if tarjeta.TopePalabras != nil {
		c.Tope.Palabras, c.PalabrasDeLaTarjeta = *tarjeta.TopePalabras, true
	}
	if tarjeta.TopeCriterios != nil {
		c.Tope.Criterios, c.CriteriosDeLaTarjeta = *tarjeta.TopeCriterios, true
	}
	return c
}

// tarjetaDelSpec is the slug of the card of the spec at ruta: ruta is
// <root>/.hoom/specs/<slug>.md, the spec of root's own tree, as relSpec
// names it: the card that measures and the record that signs come from the
// same path (finding 1b8a48: on Windows, Rel ignores case).
func tarjetaDelSpec(root, ruta string) (string, bool) {
	if filepath.Ext(ruta) != ".md" {
		return "", false
	}
	slug := strings.TrimSuffix(filepath.Base(ruta), ".md")
	// a name that is not a slug has no item: item.TarjetaDe says hay false
	return slug, slug != "" && relSpec(root, ruta) == ".hoom/specs/"+slug+".md"
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
// specPath is absolute or relative to root, as in CalibreDe, and is read
// ONCE: what is measured, what decides Aprobado and what the record signs
// are the same bytes (findings c75c44, b2072a and 6d7433).
//
// Raising the card's tope (`hoom item set <slug> --tope-palabras`) and
// approving again approves it. Approve stays the raw writer of records.
func Aprobar(m *manifest.Manifest, root, specPath string) (rec Record, already bool, err error) {
	ruta := rutaEn(root, specPath)
	raw, err := os.ReadFile(ruta)
	if err != nil {
		return Record{}, false, err
	}
	sum := hashDe(raw)
	c := calibre(m, root, ruta)
	if c.Ilegible != "" {
		return Record{}, false, &Negativa{texto: fmt.Sprintf(spec.FormatoTarjetaIlegible, c.Tarjeta, c.Ilegible)}
	}
	if st, _, err := statusSum(root, ruta, sum); err != nil || st != StatusApproved {
		if lineas := spec.LineasDelTope(spec.MedirContenido(raw), c); len(lineas) > 0 {
			return Record{}, false, &Negativa{texto: strings.Join(lineas, "\n")}
		}
	}
	return approveSum(root, ruta, sum)
}

// Negativa is Aprobar's refusal of a spec by its measure (the lines of its
// tope, or its card that cannot be read): a person's decision to take, not
// a failure of hoom. The Studio answers it with a 409, as its other refused
// approvals; its text is the refusal's, as it is.
type Negativa struct{ texto string }

func (n *Negativa) Error() string { return n.texto }
