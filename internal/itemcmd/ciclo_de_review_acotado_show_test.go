// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// `hoom item show --json` (CA-629): la tarjeta trae `pendientes` con medium,
// low, ids (nunca null) y texto. Escritos desde el spec y las firmas
// publicas, sin leer la implementacion.
package itemcmd

import (
	"encoding/json"
	"testing"
)

func crBuscarPendientes(v any) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		if p, ok := x["pendientes"].(map[string]any); ok {
			return p, true
		}
		for _, e := range x {
			if p, ok := crBuscarPendientes(e); ok {
				return p, true
			}
		}
	case []any:
		for _, e := range x {
			if p, ok := crBuscarPendientes(e); ok {
				return p, true
			}
		}
	}
	return nil, false
}

// CA-629: `hoom item show --json` trae pendientes, vacio sin hallazgos.
func TestCA629_ItemShowJSONTraePendientes(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	r := ixCorrer(t, bin, root, "item", "show", "precios", "--json")
	if r.exit != 0 {
		t.Fatalf("CA-629: item show --json sale 0: %d\n%s", r.exit, r.stderr)
	}
	var v any
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("CA-629: item show --json es JSON: %v\n%s", err, r.stdout)
	}
	p, ok := crBuscarPendientes(v)
	if !ok {
		t.Fatalf("CA-629: item show --json trae pendientes:\n%s", r.stdout)
	}
	ids, esLista := p["ids"].([]any)
	if p["medium"] != float64(0) || p["low"] != float64(0) || !esLista || len(ids) != 0 || p["texto"] != "" {
		t.Fatalf("CA-629: sin hallazgos, pendientes es {0, 0, [], \"\"}: %v", p)
	}
}
