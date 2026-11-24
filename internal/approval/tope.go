package approval

import (
	"fmt"
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
	ruta := specPath
	if !filepath.IsAbs(ruta) {
		ruta = filepath.Join(root, ruta)
	}
	c := spec.Calibre{Riesgo: riesgo.Normal}
	var tarjeta tarjetas.Leida
	if slug, ok := tarjetaDelSpec(ruta); ok {
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
	if st, _, err := Status(root, ruta); err == nil && st == StatusApproved {
		c.Aprobado = true
	}
	return c
}

// tarjetaDelSpec is the slug of the card of the spec at ruta: ruta is
// .hoom/specs/<slug>.md, under any tree.
func tarjetaDelSpec(ruta string) (string, bool) {
	dir := filepath.Dir(ruta)
	if filepath.Base(dir) != "specs" || filepath.Base(filepath.Dir(dir)) != ".hoom" || filepath.Ext(ruta) != ".md" {
		return "", false
	}
	slug := strings.TrimSuffix(filepath.Base(ruta), ".md")
	// a name that is not a slug has no item: item.TarjetaDe says hay false
	return slug, slug != ""
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
	c := CalibreDe(m, root, specPath)
	if c.Ilegible != "" {
		return Record{}, false, &Negativa{texto: fmt.Sprintf(spec.FormatoTarjetaIlegible, c.Tarjeta, c.Ilegible)}
	}
	if !c.Aprobado {
		ruta := specPath
		if !filepath.IsAbs(ruta) {
			ruta = filepath.Join(root, ruta)
		}
		med, err := spec.Medir(ruta)
		if err != nil {
			return Record{}, false, err
		}
		if lineas := spec.LineasDelTope(med, c); len(lineas) > 0 {
			return Record{}, false, &Negativa{texto: strings.Join(lineas, "\n")}
		}
	}
	return Approve(root, specPath)
}

// Negativa is Aprobar's refusal of a spec by its measure (the lines of its
// tope, or its card that cannot be read): a person's decision to take, not
// a failure of hoom. The Studio answers it with a 409, as its other refused
// approvals; its text is the refusal's, as it is.
type Negativa struct{ texto string }

func (n *Negativa) Error() string { return n.texto }
